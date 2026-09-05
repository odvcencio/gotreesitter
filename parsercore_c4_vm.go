//go:build !gts_no_parsercorephase0

package gotreesitter

// C4 stage 2 — the parser bytecode corridor: the threaded interpreter.
//
// This file implements spec.c4-bytecode-isa.v1 section 6. One function, one
// dense switch over a validated []uint32 with an integer pc, no per-op
// function values, no interface dispatch, and no map lookups on the corridor.
// Threading lives in the operands: every SHIFT target is a direct block
// offset, so corridor control flow never consults the state-to-offset index.
//
// The corridor is an accelerator inside the current scheduler, not a
// replacement. It runs only on the deterministic single-header lane
// (spec section 4.2, scheduler-frontier-shape: the eligibility test is exactly
// len(headers) == 1 plus checkpoint continuity). Every boundary hands off to
// the existing generic dispatch pass mid-parse with full state fidelity: the
// lane operates on the same scheduler struct and the same header
// representation, so handoff is field identity, not translation (spec section
// 5, R4), and write-back is nil by design.
//
// Counter discipline (spec section 5, R1): the corridor emits the identical
// abstract event sequence the generic pass emits, or it emits nothing at all.
// It reads the dispatch table first — a pure table read with no counter
// effect — and only commits counters once it has proven the disposition is
// corridor-executable. Every non-executable disposition returns to the generic
// pass, which then owns the whole pass including its Passes increment, its
// ClassifyBoundary lookup, and its decline text. That is why no corridor exit
// ever produces a decline of its own.

import (
	"os"
	"sync"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// parserCoreCorridorEnabled gates the corridor lane. Stage 2 ships it behind
// an env gate, default off (spec section 8, stage 2 item 2d). Stage 3 flips
// the default once the retain gates are green.
var (
	parserCoreCorridorEnabledOnce sync.Once
	parserCoreCorridorEnabledVal  bool
)

func parserCoreCorridorEnabled() bool {
	parserCoreCorridorEnabledOnce.Do(func() {
		switch os.Getenv("GTS_C4_CORRIDOR") {
		case "1", "true", "TRUE", "True", "on", "ON", "yes", "YES":
			parserCoreCorridorEnabledVal = true
		}
	})
	return parserCoreCorridorEnabledVal
}

// SetParserCoreCorridorEnabledForTest overrides the corridor gate for one test
// process. Restore the previous value (the returned func) when done. It
// mirrors the override pattern the compact core already uses for its own
// gates.
func SetParserCoreCorridorEnabledForTest(on bool) func() {
	previous := parserCoreCorridorEnabled()
	parserCoreCorridorEnabledVal = on
	return func() { parserCoreCorridorEnabledVal = previous }
}

// acquireParserCoreCorridorProgram compiles and memoizes the corridor program
// for a language. It never fails the caller: a grammar whose compile hits an
// unsupported construct simply keeps the generic lane (fail-closed, no
// allowlists).
func acquireParserCoreCorridorProgram(lang *Language) *ParserCoreCorridorProgram {
	if lang == nil {
		return nil
	}
	lang.corridorProgramOnce.Do(func() {
		program, err := CompileParserCoreCorridorProgram(lang)
		if err != nil {
			lang.corridorProgramErr = err
			return
		}
		lang.corridorProgram = program
	})
	program, _ := lang.corridorProgram.(*ParserCoreCorridorProgram)
	return program
}

// corridorEligible is the entry test of spec section 4.2: exactly one header,
// checkpoint continuity, a runnable header, and a token class the corridor
// owns.
//
// Cost note, stated plainly: this runs once per generic run-loop iteration,
// not once per parse, and it runs on every parse whose language compiled a
// program — including the multi-header iterations the corridor can never
// take. It is a handful of predicted branches over fields already in cache,
// which is inside the section 6.3 re-entry budget, but it is not free and it
// is not hoisted.
func (s *diagnosticParserCoreGenericScheduler) corridorEligible() bool {
	if s.corridor == nil || s.corridorRows == nil || len(s.headers) != 1 {
		return false
	}
	header := &s.headers[0]
	if header.accepted || header.paused || header.recoveryRegion() != nil || header.shifted {
		return false
	}
	// Checkpoint continuity: the header must sit on the election's own
	// interned scanner checkpoint. This is the same identity gate elect runs.
	if header.checkpoint != s.checkpointID {
		return false
	}
	// Token classes the corridor does not own hand back to the generic pass,
	// which already owns the no-lookahead reduce context, the missing-token
	// decline, and the post-root EOF requirement (spec section 4.2,
	// mode-lex-feature).
	if s.token.Missing || s.token.NoLookahead || s.requireEOFPostNoLookaheadRoot {
		return false
	}
	if uint32(s.token.Symbol) >= s.corridor.tokenCount {
		return false
	}
	return true
}

// corridorCanLoop reports whether the corridor may run its own election and
// stay on the lane (EXPECT_LOOP, spec section 3.3). It refuses whenever the
// generic run loop owns extra per-election work the corridor does not model:
// the closed-byte completion probe and the seed observer's election hooks.
func (s *diagnosticParserCoreGenericScheduler) corridorCanLoop() bool {
	return s.options.GenericStopAtClosedByte == nil &&
		s.observer.beforeElection == nil && s.observer.afterElection == nil
}

// dispatchCorridor is the VM. It executes election-to-election corridors over
// the validated instruction stream and returns to the generic loop on any
// boundary opcode.
//
// It reports whether it executed at least one dispatch pass. The corridor
// never produces a decline: on every handoff the generic pass re-runs the full
// classification for the pass the corridor refused and owns the boundary
// verbatim, so decline text, boundary kind, header index, and census class are
// unchanged by construction. Returning progressed=false is what lets the
// caller fall straight through to the generic pass in the same run-loop
// iteration, so a refusal costs one return and no lost work.
func (s *diagnosticParserCoreGenericScheduler) dispatchCorridor() (progressed bool, err error) {
	program := s.corridor
	prog := program.prog
	state, err := s.corridorHeaderState()
	if err != nil {
		return false, err
	}
	if int(state) >= len(program.stateBlockOffset) {
		return false, nil
	}
	pc := program.stateBlockOffset[state]

	for {
		// One corridor iteration equals one generic run-loop iteration, so it
		// polls stop control exactly once, at the same point.
		if err := s.pollStopControl(); err != nil {
			return progressed, err
		}
		if uint64(len(s.headers)) > s.work.PeakHeaders {
			s.work.PeakHeaders = uint64(len(s.headers))
		}

		// Dispatch: a pure table read with no counter effect. Membership
		// failure is a no-action cell, which the generic pass owns (drop,
		// native recovery, or decline).
		body := corridorDispatch(prog, pc, s.token.Symbol)
		if body == 0 {
			return progressed, nil
		}

		word := prog[body]
		op := parserCoreCorridorOpcode(word & corridorOpcodeMask)
		// Token-dependent gates run before any counter is spent, so refusing
		// here leaves the pass entirely to the generic lane.
		if !s.corridorTokenGateOpen(op) {
			return progressed, nil
		}
		// body != 0 means this exact (state, elected-token) pair carries a
		// real compiled action -- the corridor's compiled equivalent of a
		// non-empty generic action row for the shared token. Apply the same
		// shared-token deferral dispatchPassActive applies at that point
		// (issue #983): a header whose own lex mode reads a wider close-angle
		// operator with a real action here must not act on the narrower
		// elected token either. Refusing before any counter is spent hands
		// the untouched election back to the generic pass, which re-runs full
		// classification and reaches the identical no-action outcome.
		//
		// tokenMaybeContextualCloseAngle is a cheap, state-free shape check
		// on the elected token (symbol name and width) that rules out the
		// overwhelming majority of tokens before paying for
		// corridorHeaderState()'s resolve below; deferContextualCloseAngleAction
		// still runs its own full guard, including this same check, as the
		// single source of truth. A refusal here when progressed is already
		// true (finding F13) costs exactly one extra run-loop iteration and
		// cannot livelock: progressed is a fresh local for this call, so the
		// next dispatchCorridor call always starts it over at false.
		if s.tokenSource != nil && s.tokenSource.lexer != nil &&
			tokenMaybeContextualCloseAngle(s.tokenSource.language, s.token) {
			corridorState, stateErr := s.corridorHeaderState()
			if stateErr != nil {
				return progressed, stateErr
			}
			probe := s.tokenSource.relexProbeLexer
			if probe == nil {
				probe = &Lexer{}
				s.tokenSource.relexProbeLexer = probe
			}
			if deferContextualCloseAngleAction(
				s.tokenSource.language, s.tokenSource.lexer.source, corridorState, s.token, nil, probe,
				&s.tokenSource.tokenInvariantMaxReadSpan,
			) {
				return progressed, nil
			}
		}
		switch op {
		case corridorOpShift:
			handled, err := s.corridorShift(prog[body+1])
			if err != nil {
				return progressed, err
			}
			progressed = true
			if !handled {
				return progressed, nil
			}
			// EXPECT_LOOP: fall directly into the target block's EXPECT so an
			// election-to-election corridor never re-enters the outer run
			// loop's all-closed scan or its per-pass scratch begin/finish.
			if !s.corridorCanLoop() {
				return progressed, nil
			}
			if err := s.pollStopControl(); err != nil {
				return progressed, err
			}
			if err := s.elect(false); err != nil {
				return progressed, err
			}
			if s.stoppedAfterElection || !s.corridorEligible() {
				return progressed, nil
			}
			pc = uint32(word >> 6)

		case corridorOpShiftExtra:
			here, byteOffset, boundaryErr := s.compact.Boundary(s.headers[0].head)
			if boundaryErr != nil {
				return progressed, boundaryErr
			}
			if s.corridorZeroWidthExtraShiftBlocked(word, StateID(here), byteOffset) {
				return progressed, nil
			}
			handled, err := s.corridorShiftExtra(prog[body+1])
			if err != nil {
				return progressed, err
			}
			progressed = true
			if !handled {
				return progressed, nil
			}
			if !s.corridorCanLoop() {
				return progressed, nil
			}
			if err := s.pollStopControl(); err != nil {
				return progressed, err
			}
			if err := s.elect(false); err != nil {
				return progressed, err
			}
			if s.stoppedAfterElection || !s.corridorEligible() {
				return progressed, nil
			}
			if word>>6&1 == 1 {
				// Self-loop extra shift: the target is this same block.
				continue
			}
			pc = uint32(word >> 7)

		case corridorOpReduce:
			handled, err := s.corridorReduce(prog[body+2])
			if err != nil {
				return progressed, err
			}
			progressed = true
			if !handled {
				return progressed, nil
			}
			// Reduce-before-shift: the same elected token is re-dispatched at
			// the post-goto state. Goto mode indexed resolves the landing
			// state from the header, which is the graph-stack fact the table
			// cannot carry, then re-enters through one slice index.
			if !s.corridorEligible() {
				return progressed, nil
			}
			next, err := s.corridorHeaderState()
			if err != nil {
				return progressed, err
			}
			if int(next) >= len(program.stateBlockOffset) {
				return progressed, nil
			}
			pc = program.stateBlockOffset[next]

		case corridorOpReduceChain:
			// The compiler proves that every predecessor edge lands in the same
			// sole-unary-reduce row. Apply the first reduction, preserve the
			// scheduler poll boundary, then apply the second row without another
			// dispatch-table walk.
			handled, err := s.corridorReduce(prog[body+1])
			if err != nil {
				return progressed, err
			}
			progressed = true
			if !handled || !s.corridorEligible() {
				return progressed, nil
			}
			if err := s.pollStopControl(); err != nil {
				return progressed, err
			}
			handled, err = s.corridorReduce(prog[body+2])
			if err != nil {
				return progressed, err
			}
			if !handled || !s.corridorEligible() {
				return progressed, nil
			}
			next, err := s.corridorHeaderState()
			if err != nil {
				return progressed, err
			}
			if int(next) >= len(program.stateBlockOffset) {
				return progressed, nil
			}
			pc = program.stateBlockOffset[next]

		case corridorOpReduceShift:
			// Keep the reduction on the generic apply seam. The fused part
			// removes only the second EXPECT and dispatch, after the reduction
			// has produced the same authenticated header state.
			handled, err := s.corridorReduce(prog[body+1])
			if err != nil {
				return progressed, err
			}
			progressed = true
			if !handled || !s.corridorEligible() {
				return progressed, nil
			}
			reduceTarget, err := s.corridorHeaderState()
			if err != nil {
				return progressed, err
			}
			if int(reduceTarget) >= len(program.stateBlockOffset) {
				return progressed, nil
			}
			shifted, err := s.corridorDirectShift(prog[body+2], false)
			if err != nil {
				return progressed, err
			}
			if !shifted {
				// Runtime gates may refuse the direct shift. Resume at the
				// ordinary post-reduce block, with no fused counter spent.
				pc = program.stateBlockOffset[reduceTarget]
				continue
			}
			if !s.corridorCanLoop() {
				return progressed, nil
			}
			if err := s.pollStopControl(); err != nil {
				return progressed, err
			}
			if err := s.elect(false); err != nil {
				return progressed, err
			}
			if s.stoppedAfterElection || !s.corridorEligible() {
				return progressed, nil
			}
			pc = uint32(word >> 6)

		case corridorOpAccept:
			if err := s.corridorAccept(prog[body+1]); err != nil {
				return progressed, err
			}
			progressed = true
			// Acceptance completion stays with the generic loop's own
			// all-closed branch, which owns completeAcceptance and the
			// sole-exact gate unchanged.
			return progressed, nil

		default:
			// FORK, EXIT_GENERIC, EXIT_UNSUPPORTED, and every reserved
			// stage-3 opcode hand the pass back untouched.
			return progressed, nil
		}
	}
}

// corridorHeaderState resolves the sole header's LR state.
func (s *diagnosticParserCoreGenericScheduler) corridorHeaderState() (StateID, error) {
	state, _, err := s.compact.Boundary(s.headers[0].head)
	if err != nil {
		return 0, err
	}
	return StateID(state), nil
}

// corridorTokenGateOpen evaluates, from the compiled opcode alone, the
// token-dependent gates the generic pass runs before it executes a cell of
// that kind. It reads only the elected token, so it costs nothing and — this
// is the load-bearing property — it never touches the parse tables. A closed
// gate hands the pass back before the corridor has emitted a single counter
// event, so the generic pass owns the whole pass, its ActionLookups
// increment, and its decline text.
//
// It mirrors diagnosticParserCoreGenericUnsupportedCellDescriptor's
// ExtraShift and Accept arms exactly. Shift and Reduce rows have no
// token-dependent gate there (DispatchSupported is true for both).
func (s *diagnosticParserCoreGenericScheduler) corridorTokenGateOpen(op parserCoreCorridorOpcode) bool {
	token := s.token
	switch op {
	case corridorOpShiftExtra:
		return token.EndByte >= token.StartByte &&
			(token.EndByte != token.StartByte || token.ExternalScannerToken)
	case corridorOpAccept:
		return token.Symbol == 0 && token.StartByte == token.EndByte &&
			!token.Missing && !token.NoLookahead && !token.ExternalScannerToken
	default:
		return true
	}
}

// corridorZeroWidthExtraShiftBlocked is the fused SHIFT_EXTRA no-progress
// guard, evaluated from the bytecode operand and the header's own boundary
// rather than from a decoded action row. It reproduces
// zeroWidthExtraShiftWithoutProgress for the sole-header cohort without a
// table lookup, so refusing here still costs no counter event.
func (s *diagnosticParserCoreGenericScheduler) corridorZeroWidthExtraShiftBlocked(
	word uint32, state StateID, byteOffset uint32,
) bool {
	if s.currentElection.ScannerAfter != s.currentElection.ScannerBefore {
		return false
	}
	if s.token.EndByte != s.token.StartByte {
		return false
	}
	target := state
	if word>>6&1 == 0 {
		target = StateID(s.corridor.blockState(word >> 7))
	}
	return target == state && s.token.EndByte <= byteOffset
}

// corridorClassify performs the one action lookup the pass is entitled to and
// builds the single dispatch cell the apply* seams consume. It emits exactly
// the abstract events the generic pass emits at this point: one Passes
// increment, one ActionLookups increment, and one resolved-cell record.
//
// A disagreement between the compiled disposition and the freshly decoded row
// is a stage-2 bug gate (spec section 4.2, other-with-detail): the compiled
// stream and the compact core read the same immutable language tables, so
// they cannot legitimately differ. It fails the parse closed rather than
// silently re-entering the generic lane with a counter already spent.
func (s *diagnosticParserCoreGenericScheduler) corridorClassify(
	rowIndex uint32, want core.ActionRowKind,
) (cell diagnosticParserCoreGenericCell, before []DiagnosticParserCoreHeaderReceipt, err error) {
	if s.fullReceipts() {
		before, err = s.headerReceipts(s.headers)
		if err != nil {
			return cell, nil, err
		}
	}
	// This is the indirection the corridor exists to delete: the compiled body
	// already names the action row, so the lane reads it straight out of the
	// shared language table instead of walking
	// ClassifyBoundary -> lookupActionIndex -> ActionRow -> descriptor again
	// (spec section 6.1). The row index is validated once, at build (S3), and
	// proven equal to the table's own action index for every (state, symbol)
	// by the decode-back test (S2).
	actions := s.corridorRows[rowIndex]
	// The corridor still performs one table lookup for this cell — it reads
	// its own validated dispatch table instead of walking the parse table —
	// so it records the same raw table-lookup event Parser.lookupActionIndex
	// records. Without this the published TableLookupsProxy counter
	// (contract gts-work-count/v2) would fall by exactly one per corridor
	// pass, which spec section 5 R1 classes as a semantics change rather
	// than an optimization. The hook compiles to an empty function outside
	// gts_workcount builds, so the corridor pays nothing for it by default.
	//
	// Semantic count, not physical count: this line records one event per
	// corridor pass that reaches classification. It does not record one
	// event per corridorDispatch probe. dispatchCorridor's run loop calls
	// corridorDispatch once per iteration — a real read of the compiled
	// dispatch table — before this function ever runs, and a probe that
	// resolves to a FORK/EXIT_GENERIC/EXIT_UNSUPPORTED body, or to a body
	// whose corridorTokenGateOpen gate is closed, returns straight to the
	// generic pass without calling corridorClassify at all. The generic pass
	// then re-walks the parse table for that same cell and records its own
	// lookup, so the semantic event still lands exactly once — just on the
	// other lane. Measured on the four canonical fixtures, physical
	// corridorDispatch probes exceed recorded corridor passes by 10.7-15.0%
	// (rewrite +251, query_compile +1,049, language +840, grammargen_lr
	// +11,455 probes beyond recorded passes). This is the safe direction:
	// the counter under-reports physical corridor work and never fabricates
	// or double-counts an event. Stage 3 must not read CorridorPasses or the
	// TableLookupsProxy delta as the corridor's physical probe count — they
	// are semantic-event counters, and the refused-pass surplus above is the
	// gap between "semantic events recorded" and "table reads physically
	// performed."
	s.corridorRecordClassification(actions.Len())
	boundary, err := s.compact.ClassifyBoundaryWithRow(s.headers[0].head, core.Symbol(s.token.Symbol), actions)
	if err != nil {
		return cell, nil, err
	}
	if actions.Descriptor().Kind() != want {
		return cell, before, &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreRoute,
			detail:   "parser-core corridor: compiled disposition disagrees with the decoded action row",
		}
	}
	return diagnosticParserCoreGenericCell{
		headerIndex: 0, boundary: boundary,
		corridorTrustedReduction: want == core.ActionRowReduce,
	}, before, nil
}

func (s *diagnosticParserCoreGenericScheduler) corridorRecordClassification(actionCount int) {
	s.work.Passes++
	s.work.SingleHeaderPasses++
	s.work.CorridorPasses++
	workCountRecordTableLookup()
	s.work.ActionLookups++
	workCountRecordResolvedActionCell(actionCount)
}

func (s *diagnosticParserCoreGenericScheduler) corridorDirectApplyEligible(extra bool) bool {
	return s != nil && s.freshSessionOwner != nil && !s.fullReceipts() &&
		!s.token.ExternalScannerToken && s.corridorCanLoop() &&
		(!extra || s.extraPostExecutionFault == nil)
}

func (s *diagnosticParserCoreGenericScheduler) corridorDirectShift(rowIndex uint32, extra bool) (bool, error) {
	if !s.corridorDirectApplyEligible(extra) {
		return false, nil
	}
	s.corridorRecordClassification(1)
	if err := s.reserveDispatches(1); err != nil {
		return false, err
	}
	token := s.token
	action := s.corridorRows[rowIndex].At(0)
	head, err := s.compact.ShiftDirectWithLiveCondenseCandidatesOwned(
		*s.freshSessionOwner,
		nil,
		s.headers[0].head,
		core.Symbol(token.Symbol),
		core.StateID(action.State),
		extra,
		core.Token{
			Symbol:    core.Symbol(token.Symbol),
			StartByte: token.StartByte,
			EndByte:   token.EndByte,
			Extra:     extra,
			External:  token.ExternalScannerToken,
		},
		core.ForkOrder{},
	)
	if err != nil {
		return false, err
	}
	s.headers[0].head = head
	s.headers[0].shifted = true
	markDiagnosticParserCoreExternalLineage(&s.headers[0], token)
	s.epochProgress = true
	if extra {
		s.work.ExtraShifts++
	} else {
		s.work.OrdinaryShifts++
	}
	s.work.Dispatches++
	if err := s.canonicalize(); err != nil {
		return false, err
	}
	if err := s.persistHeaderLineageOwned(*s.freshSessionOwner); err != nil {
		return false, err
	}
	return len(s.headers) == 1, nil
}

func (s *diagnosticParserCoreGenericScheduler) corridorShift(rowIndex uint32) (bool, error) {
	if handled, err := s.corridorDirectShift(rowIndex, false); handled || err != nil {
		return handled, err
	}
	cell, before, err := s.corridorClassify(rowIndex, core.ActionRowShift)
	if err != nil {
		return false, err
	}
	cells := s.corridorCells[:1]
	cells[0] = cell
	if err := s.applyGenericShifts(before, cells); err != nil {
		return false, err
	}
	return len(s.headers) == 1, nil
}

func (s *diagnosticParserCoreGenericScheduler) corridorShiftExtra(rowIndex uint32) (bool, error) {
	if handled, err := s.corridorDirectShift(rowIndex, true); handled || err != nil {
		return handled, err
	}
	cell, before, err := s.corridorClassify(rowIndex, core.ActionRowExtraShift)
	if err != nil {
		return false, err
	}
	cells := s.corridorCells[:1]
	cells[0] = cell
	if err := s.applyGenericExtraShifts(before, cells); err != nil {
		return false, err
	}
	return len(s.headers) == 1, nil
}

func (s *diagnosticParserCoreGenericScheduler) corridorReduce(rowIndex uint32) (bool, error) {
	cell, before, err := s.corridorClassify(rowIndex, core.ActionRowReduce)
	if err != nil {
		return false, err
	}
	if err := s.applyGenericReduction(before, cell); err != nil {
		return false, err
	}
	return len(s.headers) == 1, nil
}

func (s *diagnosticParserCoreGenericScheduler) corridorAccept(rowIndex uint32) error {
	cell, before, err := s.corridorClassify(rowIndex, core.ActionRowAccept)
	if err != nil {
		return err
	}
	return s.applyGenericAccept(before, cell)
}
