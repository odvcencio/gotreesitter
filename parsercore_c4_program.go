//go:build !gts_no_parsercorephase0

package gotreesitter

// C4 stage 2 — the parser bytecode corridor: instruction set and compiler.
//
// This file implements spec.c4-bytecode-isa.v1 sections 3 (the instruction
// set), 4 (the closed-domain contract), and 5 S1-S3 (the static equivalence
// obligations). The interpreter lives in parsercore_c4_vm.go.
//
// Design rule (spec section 3.1): the program is the table. Every instruction
// is a projection of one decoded parse-table cell or of one scheduler
// invariant. The compiler consumes only the decoded *Language table set — the
// same input buildParserCoreLanguageTables consumes — and nothing else. There
// is one compiler function, CompileParserCoreCorridorProgram, so equivalence
// with the table interpreter is by construction and is also checked by the
// per-state decode-back test (S2) and the cell exhaustiveness test
// (contract 4.3).

import (
	"errors"
	"fmt"
	"sort"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// parserCoreCorridorOpcode is the 6-bit opcode field of one corridor
// instruction word (spec section 3.6: fixed 32-bit words, opcode 6 bits).
type parserCoreCorridorOpcode uint32

const (
	// corridorOpHalt is the guard at word 0. A body offset of 0 means "no
	// cell", so word 0 must never decode as a real instruction.
	corridorOpHalt parserCoreCorridorOpcode = iota
	// corridorOpExpect executes one election for the sole header and then
	// dispatches on the elected token through this block's dispatch table
	// (spec section 3.4).
	corridorOpExpect
	// corridorOpShift applies one generic shift for the sole header and
	// threads to the target state's block (spec section 3.2).
	corridorOpShift
	// corridorOpShiftExtra applies one extra-token shift with the fused
	// zero-width no-progress guard.
	corridorOpShiftExtra
	// corridorOpReduce applies one reduction and re-dispatches the same
	// token at the post-goto state.
	corridorOpReduce
	// corridorOpAccept is the sole-accept path with fused EOF authentication.
	corridorOpAccept
	// corridorOpFork exits to the generic dispatch pass, which executes the
	// conflict through the existing conflict executor (spec section 3.5).
	corridorOpFork
	// corridorOpExitGeneric re-enters the generic pass without declining.
	corridorOpExitGeneric
	// corridorOpExitUnsupported exits to the scheduler's existing decline.
	corridorOpExitUnsupported
	// corridorOpCheckpoint is a provenance marker. It is fused into the EXT
	// form of EXPECT and is never emitted as a stream word; the decoder
	// produces it so checkpoint obligations stay provable per state
	// (spec section 3.5, CHECKPOINT row).
	corridorOpCheckpoint
	// corridorOpReduceChain and corridorOpReduceShift are the stage-3 fused
	// superinstructions (spec section 3.3). Stage 2 reserves the encodings and
	// never emits them: the analyzer receipts justify them first.
	corridorOpReduceChain
	corridorOpReduceShift

	corridorOpcodeCount
)

func (op parserCoreCorridorOpcode) String() string {
	switch op {
	case corridorOpHalt:
		return "HALT"
	case corridorOpExpect:
		return "EXPECT"
	case corridorOpShift:
		return "SHIFT"
	case corridorOpShiftExtra:
		return "SHIFT_EXTRA"
	case corridorOpReduce:
		return "REDUCE"
	case corridorOpAccept:
		return "ACCEPT"
	case corridorOpFork:
		return "FORK"
	case corridorOpExitGeneric:
		return "EXIT_GENERIC"
	case corridorOpExitUnsupported:
		return "EXIT_UNSUPPORTED"
	case corridorOpCheckpoint:
		return "CHECKPOINT"
	case corridorOpReduceChain:
		return "REDUCE_CHAIN"
	case corridorOpReduceShift:
		return "REDUCE_SHIFT"
	default:
		return fmt.Sprintf("OPCODE(%d)", uint32(op))
	}
}

// corridorOpcodeMask extracts the opcode from an instruction word.
const corridorOpcodeMask uint32 = 0x3F

// EXPECT flags (spec section 3.4). EXT marks a state whose external-token row
// is non-empty, so the full capture+intern checkpoint protocol must run. KW
// marks a state with a reserved-word set; it is a documentation bit for the
// decoder. BUDGET marks the election as a dispatch/token cap poll point.
const (
	corridorExpectFlagEXT uint32 = 1 << iota
	corridorExpectFlagKW
	corridorExpectFlagBUDGET
)

// Dispatch-table shapes (spec section 3.6), mirroring the existing dense/small
// parse-table split.
const (
	// corridorShapeDense indexes the table directly by terminal symbol.
	corridorShapeDense uint32 = 0
	// corridorShapeSparse holds sorted (symbol, body offset) pairs and is
	// searched with a branchless binary search.
	corridorShapeSparse uint32 = 1
)

// corridorGotoModeIndexed resolves the reduction's goto through the state's
// goto row at run time; corridorGotoModeStatic fuses an edge-unique target.
// Stage 2 emits only the indexed mode: the reduction's landing predecessor is
// a run-time property of the graph stack, so a static target needs the
// edge-uniqueness proof the analyzer receipt supplies for stage 3.
const (
	corridorGotoModeIndexed uint32 = 0
	corridorGotoModeStatic  uint32 = 1
)

// Exit reasons. Every enumerated boundary class of spec section 4.2 maps to
// exactly one of these, so a corridor exit is always named.
type parserCoreCorridorExitReason uint32

const (
	corridorExitNoCell parserCoreCorridorExitReason = iota
	corridorExitRepetitionSelectable
	corridorExitConflictRow
	corridorExitUnsupportedRow
	corridorExitZeroWidthShift
	corridorExitExtraChain
	corridorExitRecoverAction
	corridorExitReasonCount
)

func (r parserCoreCorridorExitReason) String() string {
	switch r {
	case corridorExitNoCell:
		return "no-cell"
	case corridorExitRepetitionSelectable:
		return "repetition-selectable"
	case corridorExitConflictRow:
		return "conflict-row"
	case corridorExitUnsupportedRow:
		return "unsupported-row"
	case corridorExitZeroWidthShift:
		return "zero-width-shift"
	case corridorExitExtraChain:
		return "extra-chain"
	case corridorExitRecoverAction:
		return "recover-action"
	default:
		return fmt.Sprintf("reason(%d)", uint32(r))
	}
}

// Instruction word counts. Fixed widths keep decode branchless and the
// validator simple (spec section 3.6, open question Q4 default).
const (
	corridorExpectWords = 2
	// Executable bodies carry the source action-row index in their trailing
	// word. The interpreter reads the decoded row straight out of the shared
	// language table with it, so the corridor never re-walks the parse table
	// (spec section 6.1). FORK carries the same index in-word, so the fork
	// lane sees the exact ActionRow the table interpreter would see.
	corridorShiftWords      = 2
	corridorShiftExtraWords = 2
	corridorReduceWords     = 3
	corridorAcceptWords     = 2
	corridorForkWords       = 1
	corridorExitWords       = 1
	corridorSparsePairWords = 2
	// corridorMaxBlockOffset is the narrowest block-offset operand field in
	// the encoding, not the widest. SHIFT packs 26 bits at bit 6, but
	// SHIFT_EXTRA spends bit 6 on its self-loop flag and packs only 25 bits at
	// bit 7, so a 26-bit ceiling would admit a stream whose SHIFT_EXTRA
	// targets silently truncate. The compiler checks every block offset
	// against this narrowest field.
	corridorMaxBlockOffset = 1 << 25
)

// ParserCoreCorridorProgram is one grammar's compiled corridor program: a
// single validated []uint32 instruction stream plus the entry index.
//
// Threading (spec section 3.6): every SHIFT target is a word offset into the
// stream, so corridor control flow never consults a state-to-offset index. The
// index exists only for corridor entry and for re-entry after a boundary
// opcode.
type ParserCoreCorridorProgram struct {
	// prog is the validated instruction stream. Word 0 is the halt guard.
	prog []uint32
	// stateBlockOffset maps an LR state to the word offset of its EXPECT
	// header. Zero means the state has no block (unreachable or out of range).
	stateBlockOffset []uint32
	// keySets are the interned populated-terminal key sets, indexed by set id.
	// Set 0 is always empty. They drive EXPECT membership and validation only;
	// they never feed the DFA (spec section 3.4).
	keySets [][]Symbol
	// stateSetID maps a state to its interned key-set id.
	stateSetID []uint16
	// stateFlags mirrors the EXPECT flags per state, for the decoder.
	stateFlags []uint8
	// states is the compiled state count.
	states int
	// tokenCount is the terminal-symbol ceiling this program was compiled
	// against. Symbols at or above it are nonterminals and never dispatch.
	tokenCount uint32
	// rowCount is len(Language.ParseActions), the addressable action-row
	// range every executable body's row index must fall inside.
	rowCount int
	// grammar names the source language, for receipts.
	grammar string
	// census records the compiled disposition of every populated
	// (state, terminal) cell (spec section 4.3).
	census ParserCoreCorridorCensus
}

// ParserCoreCorridorCensus counts compiled dispositions. Every populated
// (state, terminal) cell contributes to exactly one field, which is what the
// exhaustiveness test asserts.
type ParserCoreCorridorCensus struct {
	Shift            uint64
	ShiftExtra       uint64
	Reduce           uint64
	Accept           uint64
	Fork             uint64
	ExitGeneric      uint64
	ExitUnsupported  uint64
	PopulatedCells   uint64
	States           uint64
	BlockWords       uint64
	BodyWords        uint64
	InternedBodies   uint64
	DenseBlocks      uint64
	SparseBlocks     uint64
	ExternalStates   uint64
	ReservedWordSets uint64
}

// Words reports the compiled stream length in 32-bit words.
func (p *ParserCoreCorridorProgram) Words() int {
	if p == nil {
		return 0
	}
	return len(p.prog)
}

// Bytes reports the retained stream footprint.
func (p *ParserCoreCorridorProgram) Bytes() int { return p.Words() * 4 }

// corridorBodyKind is the symbolic form of one compiled cell body, before the
// stream offsets exist. Interning happens on this value (spec section 3.6:
// cell bodies are interned stream-wide, like ParseActionEntry dedup).
type corridorBodyKind struct {
	op          parserCoreCorridorOpcode
	targetState uint32
	selfLoop    bool
	prod        uint16
	lhs         uint16
	childCount  uint8
	gotoMode    uint32
	rowIndex    uint32
	reason      parserCoreCorridorExitReason
}

func (b corridorBodyKind) words() int {
	switch b.op {
	case corridorOpShift:
		return corridorShiftWords
	case corridorOpShiftExtra:
		return corridorShiftExtraWords
	case corridorOpReduce:
		return corridorReduceWords
	case corridorOpAccept:
		return corridorAcceptWords
	case corridorOpFork:
		return corridorForkWords
	case corridorOpExitGeneric, corridorOpExitUnsupported:
		return corridorExitWords
	default:
		return 0
	}
}

// corridorStateBlock is one state's compiled block plan.
type corridorStateBlock struct {
	keys       []Symbol
	bodies     []int // parallel to keys: index into the interned body pool
	shape      uint32
	tableWords int
	setID      uint16
	flags      uint8
	offset     uint32
}

func (b *corridorStateBlock) words() int { return corridorExpectWords + b.tableWords }

// corridorTables is the read-only decoded table view the compiler consumes. It
// holds exactly the inputs spec section 4 contract item 1 admits: ParseActions,
// ParseTable, SmallParseTable(+Map), goto data, LexModes/ExternalLexStates
// emptiness, ConflictPolicies, and repetition-fold shape. Nothing else.
type corridorTables struct {
	lang       *Language
	denseLimit int
	smallBase  int
	tokenCount uint32
	stateCount int
}

func newCorridorTables(lang *Language) (*corridorTables, error) {
	if lang == nil {
		return nil, errors.New("parser-core corridor: nil language")
	}
	t := &corridorTables{lang: lang, tokenCount: lang.TokenCount}
	if lang.LargeStateCount > 0 {
		t.denseLimit = int(lang.LargeStateCount)
	} else {
		t.denseLimit = len(lang.ParseTable)
	}
	t.smallBase = int(lang.LargeStateCount)
	states := int(lang.StateCount)
	if states == 0 {
		states = len(lang.ParseTable)
		if small := t.smallBase + len(lang.SmallParseTableMap); small > states {
			states = small
		}
		if len(lang.LexModes) > states {
			states = len(lang.LexModes)
		}
	}
	t.stateCount = states
	if t.stateCount <= 0 {
		return nil, errors.New("parser-core corridor: language has no parser states")
	}
	if t.tokenCount == 0 {
		return nil, errors.New("parser-core corridor: language has no terminal symbol ceiling")
	}
	return t, nil
}

// actionIndex mirrors Parser.lookupActionIndex without needing a *Parser. It
// reads only the language's own dense and small parse tables.
func (t *corridorTables) actionIndex(state StateID, sym Symbol) uint16 {
	if int(state) < t.denseLimit {
		if int(state) >= len(t.lang.ParseTable) {
			return 0
		}
		row := t.lang.ParseTable[state]
		if int(sym) >= len(row) {
			return 0
		}
		return row[sym]
	}
	smallIdx := int(state) - t.smallBase
	if smallIdx < 0 || smallIdx >= len(t.lang.SmallParseTableMap) {
		return 0
	}
	offset := t.lang.SmallParseTableMap[smallIdx]
	table := t.lang.SmallParseTable
	if int(offset) >= len(table) {
		return 0
	}
	groupCount := table[offset]
	pos := int(offset) + 1
	for i := uint16(0); i < groupCount; i++ {
		if pos+1 >= len(table) {
			break
		}
		sectionValue := table[pos]
		symbolCount := table[pos+1]
		pos += 2
		for j := uint16(0); j < symbolCount; j++ {
			if pos >= len(table) {
				break
			}
			if table[pos] == uint16(sym) {
				return sectionValue
			}
			pos++
		}
	}
	return 0
}

// forEachKey visits every populated (state, symbol) key in ascending symbol
// order. It mirrors Parser.forEachActionIndexInState over language data only.
func (t *corridorTables) forEachKey(state StateID, visit func(sym Symbol, idx uint16)) {
	if int(state) < t.denseLimit {
		if int(state) >= len(t.lang.ParseTable) {
			return
		}
		for sym, idx := range t.lang.ParseTable[state] {
			if idx == 0 {
				continue
			}
			visit(Symbol(sym), idx)
		}
		return
	}
	smallIdx := int(state) - t.smallBase
	if smallIdx < 0 || smallIdx >= len(t.lang.SmallParseTableMap) {
		return
	}
	offset := t.lang.SmallParseTableMap[smallIdx]
	table := t.lang.SmallParseTable
	if int(offset) >= len(table) {
		return
	}
	type pair struct {
		sym Symbol
		val uint16
	}
	var pairs []pair
	groupCount := table[offset]
	pos := int(offset) + 1
	for i := uint16(0); i < groupCount; i++ {
		if pos+1 >= len(table) {
			break
		}
		sectionValue := table[pos]
		symbolCount := table[pos+1]
		pos += 2
		for j := uint16(0); j < symbolCount; j++ {
			if pos >= len(table) {
				break
			}
			pairs = append(pairs, pair{sym: Symbol(table[pos]), val: sectionValue})
			pos++
		}
	}
	sort.Slice(pairs, func(a, b int) bool { return pairs[a].sym < pairs[b].sym })
	for _, p := range pairs {
		visit(p.sym, p.val)
	}
}

// row decodes one (state, symbol) cell into the same core.ActionRow the
// compact core's TableView produces. This is the single decode path the
// compiler and the decode-back test share (S2).
func (t *corridorTables) row(state StateID, sym Symbol) (core.ActionRow, uint16, error) {
	idx := t.actionIndex(state, sym)
	if idx == 0 || int(idx) >= len(t.lang.ParseActions) {
		return core.ActionRow{}, idx, nil
	}
	entry := t.lang.ParseActions[idx]
	if len(entry.Actions) == 0 {
		return core.ActionRow{}, idx, nil
	}
	converted := make([]core.Action, len(entry.Actions))
	for ordinal, action := range entry.Actions {
		var err error
		converted[ordinal], err = parserCoreAction(action)
		if err != nil {
			return core.ActionRow{}, idx, err
		}
	}
	return core.NewActionRow(converted), idx, nil
}

// externalState reports whether the state's external-token row is non-empty,
// which sets the EXT flag (spec section 3.4). It mirrors the runtime's own two
// sources: the ExternalLexStates table when present, the parse-action probe
// otherwise.
func (t *corridorTables) externalState(state StateID) bool {
	lang := t.lang
	if len(lang.ExternalSymbols) == 0 {
		return false
	}
	if len(lang.ExternalLexStates) > 0 {
		if int(state) >= len(lang.LexModes) {
			return false
		}
		external := int(lang.LexModes[state].ExternalLexState)
		if external <= 0 || external >= len(lang.ExternalLexStates) {
			return false
		}
		for _, valid := range lang.ExternalLexStates[external] {
			if valid {
				return true
			}
		}
		return false
	}
	for _, sym := range lang.ExternalSymbols {
		if t.actionIndex(state, sym) != 0 {
			return true
		}
	}
	return false
}

// reservedWordState reports whether the state carries a reserved-word set,
// which sets the KW documentation flag.
func (t *corridorTables) reservedWordState(state StateID) bool {
	lang := t.lang
	if len(lang.ReservedWords) == 0 || lang.MaxReservedWordSetSize == 0 {
		return false
	}
	if int(state) >= len(lang.LexModes) {
		return false
	}
	return lang.LexModes[state].ReservedWordSetID != 0
}

// classifyCell maps one decoded cell to its corridor disposition. This is the
// dispositions table of spec section 4.2 made executable: every enumerated
// behavior class returns exactly one body kind, and there is no default arm
// that invents behavior — the final branch is the named unsupported class.
func (t *corridorTables) classifyCell(state StateID, sym Symbol, row core.ActionRow, rowIndex uint16) corridorBodyKind {
	descriptor := row.Descriptor()
	switch descriptor.Kind() {
	case core.ActionRowEmpty:
		// Empty rows never enter a dispatch table; the caller filters them.
		return corridorBodyKind{op: corridorOpExitGeneric, reason: corridorExitNoCell}

	case core.ActionRowShift:
		// Sole ordinary shift. A sole-action row can never carry a conflict
		// policy (diagnosticParserCoreConflictPolicyOrdinal requires
		// actions.Len() >= 2) and can never carry a repetition selection
		// (both repetition paths require an ActionRowUnsupported descriptor),
		// so the disposition is statically total.
		action := row.At(0)
		return corridorBodyKind{op: corridorOpShift, targetState: uint32(action.State), rowIndex: uint32(rowIndex)}

	case core.ActionRowExtraShift:
		// Single-header extra cohort (spec section 4.2, extra-shift-shape).
		// The zero-width no-progress guard is fused into the opcode and
		// evaluated at run time, because it reads the elected token's span.
		action := row.At(0)
		target := uint32(action.State)
		selfLoop := target == 0 || target == uint32(state)
		return corridorBodyKind{op: corridorOpShiftExtra, targetState: target, selfLoop: selfLoop, rowIndex: uint32(rowIndex)}

	case core.ActionRowReduce:
		action := row.At(0)
		return corridorBodyKind{
			op: corridorOpReduce, prod: action.ProductionID, lhs: uint16(action.Symbol),
			childCount: action.ChildCount, gotoMode: corridorGotoModeIndexed,
			rowIndex: uint32(rowIndex),
		}

	case core.ActionRowAccept:
		// Acceptance-side classes are unchanged: ACCEPT exits into the same
		// completeAcceptance and the same sole-exact gate.
		return corridorBodyKind{op: corridorOpAccept, rowIndex: uint32(rowIndex)}

	case core.ActionRowConflict:
		// Q3 default: stage 2 forks all conflicts, the smallest sound
		// corridor. The operand carries the original action-row index so the
		// fork lane sees the exact ActionRow the table interpreter sees.
		return corridorBodyKind{op: corridorOpFork, rowIndex: uint32(rowIndex), reason: corridorExitConflictRow}

	case core.ActionRowUnsupported:
		// Unsupported rows split by whether the generic lane can still execute
		// them through a static selection. Repetition-fold and the
		// single-reduce repetition-shift opt-out are generic-lane executable,
		// so they exit without declining; everything else is the same
		// fail-closed decline the scheduler produces today.
		if t.repetitionSelectable(row) {
			return corridorBodyKind{op: corridorOpExitGeneric, reason: corridorExitRepetitionSelectable}
		}
		return corridorBodyKind{
			op: corridorOpExitUnsupported, rowIndex: uint32(rowIndex),
			reason: t.unsupportedReason(row),
		}
	}
	// Unreachable: ActionRowKind is a closed enumeration and every member is
	// handled above. Fail closed rather than invent a disposition.
	return corridorBodyKind{op: corridorOpExitUnsupported, reason: corridorExitUnsupportedRow}
}

// repetitionSelectable reports whether the generic lane owns a static
// selection for this unsupported row (spec section 4.2,
// repetition-shift-class). It mirrors the two selection probes the dispatch
// pass runs, in the same order.
func (t *corridorTables) repetitionSelectable(row core.ActionRow) bool {
	if _, ok := diagnosticParserCoreRepetitionFoldOrdinal(t.lang, row); ok {
		return true
	}
	if _, ok := diagnosticParserCoreSingleReduceRepetitionShiftOrdinal(row); ok &&
		cRepetitionSkipOptOut[t.lang.Name] {
		return true
	}
	return false
}

// unsupportedReason names why a row decodes unsupported, so no corridor exit
// is anonymous (spec section 4.2, other-with-detail is a bug gate).
func (t *corridorTables) unsupportedReason(row core.ActionRow) parserCoreCorridorExitReason {
	for index := 0; index < row.Len(); index++ {
		action := row.At(index)
		switch {
		case action.ExtraChain:
			return corridorExitExtraChain
		case action.Type == core.ActionRecover:
			return corridorExitRecoverAction
		case action.Repetition:
			return corridorExitRepetitionSelectable
		}
	}
	return corridorExitUnsupportedRow
}

// CompileParserCoreCorridorProgram is the one compiler (spec section 5, S1).
// grammargen's assemble step and the runtime load path both feed the same
// decoded table set into this function; there is no second implementation.
func CompileParserCoreCorridorProgram(lang *Language) (*ParserCoreCorridorProgram, error) {
	tables, err := newCorridorTables(lang)
	if err != nil {
		return nil, err
	}

	// Pass A: classify every populated terminal cell per state, intern cell
	// bodies and key sets, and choose each block's dispatch shape.
	bodyIndex := make(map[corridorBodyKind]int)
	var bodyPool []corridorBodyKind
	internBody := func(kind corridorBodyKind) int {
		if idx, ok := bodyIndex[kind]; ok {
			return idx
		}
		idx := len(bodyPool)
		bodyPool = append(bodyPool, kind)
		bodyIndex[kind] = idx
		return idx
	}

	keySetIndex := make(map[string]uint16)
	keySets := [][]Symbol{nil}
	keySetIndex[""] = 0

	blocks := make([]corridorStateBlock, tables.stateCount)
	program := &ParserCoreCorridorProgram{
		states:     tables.stateCount,
		tokenCount: tables.tokenCount,
		rowCount:   len(lang.ParseActions),
		grammar:    lang.Name,
	}
	census := &program.census
	census.States = uint64(tables.stateCount)

	for state := 0; state < tables.stateCount; state++ {
		block := &blocks[state]
		var visitErr error
		tables.forEachKey(StateID(state), func(sym Symbol, idx uint16) {
			if visitErr != nil {
				return
			}
			if uint32(sym) >= tables.tokenCount {
				// Nonterminal goto entries share the parse table but never
				// dispatch on an elected token. Goto rows stay outside the
				// corridor dispatch table (spec section 3.6).
				return
			}
			row, rowIndex, err := tables.row(StateID(state), sym)
			if err != nil {
				visitErr = err
				return
			}
			if row.Len() == 0 {
				return
			}
			kind := tables.classifyCell(StateID(state), sym, row, rowIndex)
			block.keys = append(block.keys, sym)
			block.bodies = append(block.bodies, internBody(kind))
			census.PopulatedCells++
			switch kind.op {
			case corridorOpShift:
				census.Shift++
			case corridorOpShiftExtra:
				census.ShiftExtra++
			case corridorOpReduce:
				census.Reduce++
			case corridorOpAccept:
				census.Accept++
			case corridorOpFork:
				census.Fork++
			case corridorOpExitGeneric:
				census.ExitGeneric++
			case corridorOpExitUnsupported:
				census.ExitUnsupported++
			default:
				visitErr = fmt.Errorf(
					"parser-core corridor: state %d symbol %d compiled to unenumerated disposition %s",
					state, sym, kind.op,
				)
			}
		})
		if visitErr != nil {
			return nil, visitErr
		}

		block.setID = internCorridorKeySet(keySetIndex, &keySets, block.keys)
		if tables.externalState(StateID(state)) {
			block.flags |= uint8(corridorExpectFlagEXT)
			census.ExternalStates++
		}
		if tables.reservedWordState(StateID(state)) {
			block.flags |= uint8(corridorExpectFlagKW)
			census.ReservedWordSets++
		}
		// Every corridor election is a cap poll point (spec section 3.4,
		// flag BUDGET), aligned with the stop-control boundaries.
		block.flags |= uint8(corridorExpectFlagBUDGET)
		block.shape, block.tableWords = corridorChooseShape(block.keys)
		if block.shape == corridorShapeDense {
			census.DenseBlocks++
		} else {
			census.SparseBlocks++
		}
	}

	// Pass B: lay out the stream. Word 0 is the halt guard, the interned body
	// pool follows, then one block per state in state order.
	bodyOffsets := make([]uint32, len(bodyPool))
	offset := uint32(1)
	for index, body := range bodyPool {
		bodyOffsets[index] = offset
		offset += uint32(body.words())
	}
	census.BodyWords = uint64(offset - 1)
	census.InternedBodies = uint64(len(bodyPool))

	program.stateBlockOffset = make([]uint32, tables.stateCount)
	for state := range blocks {
		block := &blocks[state]
		block.offset = offset
		program.stateBlockOffset[state] = offset
		offset += uint32(block.words())
		if offset >= corridorMaxBlockOffset {
			return nil, fmt.Errorf("parser-core corridor: program exceeds the %d-word block-offset ceiling", corridorMaxBlockOffset)
		}
	}
	census.BlockWords = uint64(offset) - census.BodyWords - 1

	// Pass C: emit.
	prog := make([]uint32, offset)
	prog[0] = uint32(corridorOpHalt)
	for index, body := range bodyPool {
		if err := corridorEmitBody(prog, bodyOffsets[index], body, program.stateBlockOffset); err != nil {
			return nil, err
		}
	}
	for state := range blocks {
		block := &blocks[state]
		base := block.offset
		prog[base] = uint32(corridorOpExpect) | uint32(block.flags)<<6 | block.shape<<14
		prog[base+1] = uint32(block.setID) | uint32(block.tableWords)<<16
		table := prog[base+corridorExpectWords : base+uint32(block.words())]
		switch block.shape {
		case corridorShapeDense:
			for i, sym := range block.keys {
				table[sym] = bodyOffsets[block.bodies[i]]
			}
		default:
			for i, sym := range block.keys {
				table[i*corridorSparsePairWords] = uint32(sym)
				table[i*corridorSparsePairWords+1] = bodyOffsets[block.bodies[i]]
			}
		}
	}

	program.prog = prog
	program.keySets = keySets
	program.stateSetID = make([]uint16, tables.stateCount)
	program.stateFlags = make([]uint8, tables.stateCount)
	for state := range blocks {
		program.stateSetID[state] = blocks[state].setID
		program.stateFlags[state] = blocks[state].flags
	}

	// S3: validate the stream once, at build. The interpreter then trusts
	// every offset it reads (spec section 6.1).
	if err := program.validate(); err != nil {
		return nil, err
	}
	return program, nil
}

func internCorridorKeySet(index map[string]uint16, sets *[][]Symbol, keys []Symbol) uint16 {
	if len(keys) == 0 {
		return 0
	}
	buf := make([]byte, 0, len(keys)*2)
	for _, sym := range keys {
		buf = append(buf, byte(sym), byte(sym>>8))
	}
	if id, ok := index[string(buf)]; ok {
		return id
	}
	id := uint16(len(*sets))
	snapshot := make([]Symbol, len(keys))
	copy(snapshot, keys)
	*sets = append(*sets, snapshot)
	index[string(buf)] = id
	return id
}

// corridorChooseShape mirrors the existing dense/small parse-table split: a
// direct-index table when the key span is dense enough to pay for itself, a
// sorted pair table otherwise. Both shapes decode to the same dispositions,
// so the choice is a footprint decision only.
func corridorChooseShape(keys []Symbol) (shape uint32, words int) {
	if len(keys) == 0 {
		return corridorShapeSparse, 0
	}
	span := int(keys[len(keys)-1]) + 1
	sparseWords := len(keys) * corridorSparsePairWords
	if span <= sparseWords {
		return corridorShapeDense, span
	}
	return corridorShapeSparse, sparseWords
}

func corridorEmitBody(prog []uint32, at uint32, body corridorBodyKind, blockOffsets []uint32) error {
	blockOffsetFor := func(state uint32) (uint32, error) {
		if int(state) >= len(blockOffsets) {
			return 0, fmt.Errorf("parser-core corridor: shift target state %d is outside the compiled state range", state)
		}
		return blockOffsets[state], nil
	}
	switch body.op {
	case corridorOpShift:
		target, err := blockOffsetFor(body.targetState)
		if err != nil {
			return err
		}
		prog[at] = uint32(corridorOpShift) | target<<6
		prog[at+1] = body.rowIndex
	case corridorOpShiftExtra:
		target := uint32(0)
		if !body.selfLoop {
			resolved, err := blockOffsetFor(body.targetState)
			if err != nil {
				return err
			}
			target = resolved
		}
		selfLoop := uint32(0)
		if body.selfLoop {
			selfLoop = 1
		}
		prog[at] = uint32(corridorOpShiftExtra) | selfLoop<<6 | target<<7
		prog[at+1] = body.rowIndex
	case corridorOpReduce:
		prog[at] = uint32(corridorOpReduce) | body.gotoMode<<6 | uint32(body.childCount)<<8
		prog[at+1] = uint32(body.prod) | uint32(body.lhs)<<16
		prog[at+2] = body.rowIndex
	case corridorOpAccept:
		prog[at] = uint32(corridorOpAccept)
		prog[at+1] = body.rowIndex
	case corridorOpFork:
		prog[at] = uint32(corridorOpFork) | body.rowIndex<<6
	case corridorOpExitGeneric:
		prog[at] = uint32(corridorOpExitGeneric) | uint32(body.reason)<<6
	case corridorOpExitUnsupported:
		prog[at] = uint32(corridorOpExitUnsupported) | uint32(body.reason)<<6
	default:
		return fmt.Errorf("parser-core corridor: cannot emit body opcode %s", body.op)
	}
	return nil
}

// validate is S3: every offset in range, every set id valid, every block
// well-formed. It runs once at build so the interpreter can trust the stream.
func (p *ParserCoreCorridorProgram) validate() error {
	if len(p.prog) == 0 || p.prog[0] != uint32(corridorOpHalt) {
		return errors.New("parser-core corridor: stream is missing its halt guard")
	}
	if len(p.stateBlockOffset) != p.states {
		return errors.New("parser-core corridor: state block index width mismatch")
	}
	seenBody := make(map[uint32]bool)
	for state := 0; state < p.states; state++ {
		base := p.stateBlockOffset[state]
		if int(base)+corridorExpectWords > len(p.prog) {
			return fmt.Errorf("parser-core corridor: state %d block header is out of range", state)
		}
		header := p.prog[base]
		if header&corridorOpcodeMask != uint32(corridorOpExpect) {
			return fmt.Errorf("parser-core corridor: state %d block does not start with EXPECT", state)
		}
		shape := (header >> 14) & 0x3
		if shape != corridorShapeDense && shape != corridorShapeSparse {
			return fmt.Errorf("parser-core corridor: state %d has unknown dispatch shape %d", state, shape)
		}
		setID := p.prog[base+1] & 0xFFFF
		if int(setID) >= len(p.keySets) {
			return fmt.Errorf("parser-core corridor: state %d references unknown key set %d", state, setID)
		}
		tableWords := int(p.prog[base+1] >> 16)
		if int(base)+corridorExpectWords+tableWords > len(p.prog) {
			return fmt.Errorf("parser-core corridor: state %d dispatch table overruns the stream", state)
		}
		table := p.prog[int(base)+corridorExpectWords : int(base)+corridorExpectWords+tableWords]
		keys := p.keySets[setID]
		populated := 0
		if shape == corridorShapeDense {
			for sym, body := range table {
				if body == 0 {
					continue
				}
				populated++
				if err := p.validateBody(body, state, Symbol(sym)); err != nil {
					return err
				}
				seenBody[body] = true
			}
		} else {
			if tableWords%corridorSparsePairWords != 0 {
				return fmt.Errorf("parser-core corridor: state %d sparse table is not pair aligned", state)
			}
			previous := int64(-1)
			for i := 0; i < tableWords; i += corridorSparsePairWords {
				sym := int64(table[i])
				if sym <= previous {
					return fmt.Errorf("parser-core corridor: state %d sparse table is not strictly ascending", state)
				}
				previous = sym
				body := table[i+1]
				if body == 0 {
					return fmt.Errorf("parser-core corridor: state %d sparse table has an empty body offset", state)
				}
				populated++
				if err := p.validateBody(body, state, Symbol(sym)); err != nil {
					return err
				}
				seenBody[body] = true
			}
		}
		if populated != len(keys) {
			return fmt.Errorf("parser-core corridor: state %d has %d table entries but key set %d holds %d keys", state, populated, setID, len(keys))
		}
	}
	return nil
}

// validateBlockTarget is part of S3: a threading operand must name a real
// state block, not merely a word whose low six bits happen to read as EXPECT.
// Sniffing the opcode bits is not sufficient — a target pointing one word into
// a block header decodes its set-id field, and any set id congruent to the
// EXPECT opcode modulo 64 would pass. It resolves the offset through the
// state-block index instead, which admits exactly the compiled block starts.
func (p *ParserCoreCorridorProgram) validateBlockTarget(
	target uint32, state int, sym Symbol, op parserCoreCorridorOpcode,
) error {
	if !p.isBlockOffset(target) {
		return fmt.Errorf("parser-core corridor: state %d symbol %d %s target %d is not a compiled block start", state, sym, op, target)
	}
	if int(target)+corridorExpectWords > len(p.prog) || p.prog[target]&corridorOpcodeMask != uint32(corridorOpExpect) {
		return fmt.Errorf("parser-core corridor: state %d symbol %d %s target %d is not a block header", state, sym, op, target)
	}
	return nil
}

// isBlockOffset reports whether offset is exactly one state's block start.
// stateBlockOffset is strictly ascending by construction, so this is the same
// binary search blockState performs.
func (p *ParserCoreCorridorProgram) isBlockOffset(offset uint32) bool {
	lo, hi := 0, len(p.stateBlockOffset)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if p.stateBlockOffset[mid] < offset {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo < len(p.stateBlockOffset) && p.stateBlockOffset[lo] == offset
}

// validateRowIndex is part of S3: an executable body's action-row index must
// address a real row of the shared language table the interpreter reads.
func (p *ParserCoreCorridorProgram) validateRowIndex(index uint32, state int, sym Symbol) error {
	if index == 0 || int(index) >= p.rowCount {
		return fmt.Errorf("parser-core corridor: state %d symbol %d body carries out-of-range action-row index %d", state, sym, index)
	}
	return nil
}

func (p *ParserCoreCorridorProgram) validateBody(at uint32, state int, sym Symbol) error {
	if int(at) >= len(p.prog) {
		return fmt.Errorf("parser-core corridor: state %d symbol %d body offset %d is out of range", state, sym, at)
	}
	word := p.prog[at]
	op := parserCoreCorridorOpcode(word & corridorOpcodeMask)
	switch op {
	case corridorOpShift:
		if int(at)+corridorShiftWords > len(p.prog) {
			return fmt.Errorf("parser-core corridor: state %d symbol %d SHIFT overruns the stream", state, sym)
		}
		if err := p.validateBlockTarget(word>>6, state, sym, corridorOpShift); err != nil {
			return err
		}
		if err := p.validateRowIndex(p.prog[at+1], state, sym); err != nil {
			return err
		}
	case corridorOpShiftExtra:
		if int(at)+corridorShiftExtraWords > len(p.prog) {
			return fmt.Errorf("parser-core corridor: state %d symbol %d SHIFT_EXTRA overruns the stream", state, sym)
		}
		if err := p.validateRowIndex(p.prog[at+1], state, sym); err != nil {
			return err
		}
		if word>>6&1 == 0 {
			if err := p.validateBlockTarget(word>>7, state, sym, corridorOpShiftExtra); err != nil {
				return err
			}
		}
	case corridorOpReduce:
		if int(at)+corridorReduceWords > len(p.prog) {
			return fmt.Errorf("parser-core corridor: state %d symbol %d REDUCE overruns the stream", state, sym)
		}
		if mode := word >> 6 & 0x3; mode != corridorGotoModeIndexed && mode != corridorGotoModeStatic {
			return fmt.Errorf("parser-core corridor: state %d symbol %d REDUCE has unknown goto mode %d", state, sym, mode)
		}
		if err := p.validateRowIndex(p.prog[at+2], state, sym); err != nil {
			return err
		}
	case corridorOpAccept:
		if int(at)+corridorAcceptWords > len(p.prog) {
			return fmt.Errorf("parser-core corridor: state %d symbol %d ACCEPT overruns the stream", state, sym)
		}
		if err := p.validateRowIndex(p.prog[at+1], state, sym); err != nil {
			return err
		}
	case corridorOpFork:
		// FORK hands the fork lane the original action-row index, so the same
		// range obligation applies to it as to an executable body.
		if err := p.validateRowIndex(word>>6, state, sym); err != nil {
			return err
		}
	case corridorOpExitGeneric, corridorOpExitUnsupported:
		// Single-word forms with in-word operands only.
	default:
		return fmt.Errorf("parser-core corridor: state %d symbol %d body decodes to non-body opcode %s", state, sym, op)
	}
	return nil
}

// corridorDispatch resolves one (state block, terminal) pair to its cell body
// offset, or zero when the state has no action for that symbol. It is the
// exact lookup the interpreter performs, factored out so the decode-back test
// walks the same path the VM walks.
func corridorDispatch(prog []uint32, base uint32, sym Symbol) uint32 {
	header := prog[base]
	tableWords := int(prog[base+1] >> 16)
	table := prog[int(base)+corridorExpectWords : int(base)+corridorExpectWords+tableWords]
	if (header>>14)&0x3 == corridorShapeDense {
		if int(sym) >= len(table) {
			return 0
		}
		return table[sym]
	}
	lo, hi := 0, len(table)/corridorSparsePairWords
	target := uint32(sym)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if table[mid*corridorSparsePairWords] < target {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(table)/corridorSparsePairWords && table[lo*corridorSparsePairWords] == target {
		return table[lo*corridorSparsePairWords+1]
	}
	return 0
}

// ParserCoreCorridorDecodedCell is the decode-back form of one compiled cell
// (spec section 5, S2). It names the disposition and carries enough operand
// detail to reconstruct the constituent table cell.
type ParserCoreCorridorDecodedCell struct {
	State        StateID
	Symbol       Symbol
	Opcode       string
	TargetState  StateID
	SelfLoop     bool
	ProductionID uint16
	LHS          Symbol
	ChildCount   uint8
	GotoMode     string
	RowIndex     uint32
	Reason       string
}

// DecodeCell decodes the compiled disposition of one (state, terminal) cell.
// It reports ok=false when the state has no compiled action for the symbol.
func (p *ParserCoreCorridorProgram) DecodeCell(state StateID, sym Symbol) (ParserCoreCorridorDecodedCell, bool) {
	if p == nil || int(state) >= len(p.stateBlockOffset) {
		return ParserCoreCorridorDecodedCell{}, false
	}
	base := p.stateBlockOffset[state]
	body := corridorDispatch(p.prog, base, sym)
	if body == 0 {
		return ParserCoreCorridorDecodedCell{}, false
	}
	word := p.prog[body]
	op := parserCoreCorridorOpcode(word & corridorOpcodeMask)
	decoded := ParserCoreCorridorDecodedCell{State: state, Symbol: sym, Opcode: op.String()}
	switch op {
	case corridorOpShift:
		decoded.TargetState = StateID(p.blockState(word >> 6))
		decoded.RowIndex = p.prog[body+1]
	case corridorOpShiftExtra:
		decoded.SelfLoop = word>>6&1 == 1
		if !decoded.SelfLoop {
			decoded.TargetState = StateID(p.blockState(word >> 7))
		} else {
			decoded.TargetState = state
		}
		decoded.RowIndex = p.prog[body+1]
	case corridorOpAccept:
		decoded.RowIndex = p.prog[body+1]
	case corridorOpReduce:
		decoded.ProductionID = uint16(p.prog[body+1] & 0xFFFF)
		decoded.LHS = Symbol(p.prog[body+1] >> 16)
		decoded.ChildCount = uint8(word >> 8 & 0x3F)
		decoded.RowIndex = p.prog[body+2]
		if word>>6&0x3 == corridorGotoModeStatic {
			decoded.GotoMode = "static"
		} else {
			decoded.GotoMode = "indexed"
		}
	case corridorOpFork:
		decoded.RowIndex = word >> 6
		decoded.Reason = corridorExitConflictRow.String()
	case corridorOpExitGeneric, corridorOpExitUnsupported:
		decoded.Reason = parserCoreCorridorExitReason(word >> 6).String()
	}
	return decoded, true
}

// DecodeCheckpointObligation reports the provenance marker for a state: the
// CHECKPOINT form when the state's EXPECT carries EXT, and the plain EXPECT
// form otherwise (spec section 3.5, CHECKPOINT row).
func (p *ParserCoreCorridorProgram) DecodeCheckpointObligation(state StateID) string {
	if p == nil || int(state) >= len(p.stateFlags) {
		return corridorOpHalt.String()
	}
	if uint32(p.stateFlags[state])&corridorExpectFlagEXT != 0 {
		return corridorOpCheckpoint.String()
	}
	return corridorOpExpect.String()
}

// blockState maps a block word offset back to its state, for decode-back.
func (p *ParserCoreCorridorProgram) blockState(offset uint32) uint32 {
	lo, hi := 0, len(p.stateBlockOffset)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if p.stateBlockOffset[mid] < offset {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(p.stateBlockOffset) && p.stateBlockOffset[lo] == offset {
		return uint32(lo)
	}
	return 0
}

// KeySet returns the interned populated-terminal key set for a state.
func (p *ParserCoreCorridorProgram) KeySet(state StateID) []Symbol {
	if p == nil || int(state) >= len(p.stateSetID) {
		return nil
	}
	return p.keySets[p.stateSetID[state]]
}

// StateCount reports the compiled state count.
func (p *ParserCoreCorridorProgram) StateCount() int {
	if p == nil {
		return 0
	}
	return p.states
}

// Census reports the compiled disposition counts.
func (p *ParserCoreCorridorProgram) Census() ParserCoreCorridorCensus {
	if p == nil {
		return ParserCoreCorridorCensus{}
	}
	return p.census
}
