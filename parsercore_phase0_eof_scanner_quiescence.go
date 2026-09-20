//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// Scanner quiescence at the compact end-of-file admission.
//
// produceCompactEOFRecoveryAdmission (parsercore_phase0_driver.go) admits one
// exact frontier shape: at authenticated end of input, one head holds a sole
// Accept row and one sibling head holds an empty action row. C tree-sitter
// resolves that shape by error cost. ts_parser__advance accepts the first
// version (ts_parser__accept, parser.c), pauses the second one
// ("detect_error"), and ts_parser__condense_stack then resumes the paused
// version into ts_parser__handle_error. Every tree the resumed version can
// still build carries a recovery cost above zero, so ts_parser__select_tree
// keeps the accepted cost-zero tree ("select_smaller_error"). The admission
// models that rule directly: the accepting head prices at zero and the
// no-action head prices at RecoveryCostPerRecovery or more.
//
// That argument holds for a scanner-free language without further proof,
// because the internal lexer is a function of the byte position alone. An
// external scanner breaks the assumption in one specific way: C runs
// ts_parser__lex once per stack version, with that version's own
// external_lex_state row, while the compact scheduler elects one shared token
// for the whole frontier. That shared election is not one row. For a
// multi-state frontier the token source first scores each distinct
// external_lex_state separately (nextGLRScoredExternalToken,
// parser_dfa_token_source.go) and falls back to the union of the rows only
// when that scoring declines. Either way the scanner sees a set of valid
// symbols no single head owns, so a scanner that reads valid_symbols
// non-monotonically could offer the no-action head a token under its own row
// that the shared election never produced. The head would not be dead in C,
// and the compact accept would publish the wrong tree.
//
// compactEOFScannerQuiescenceProof closes that gap by measurement instead of
// assumption. It re-runs the external scanner once per head state, in
// isolation, from the authenticated election-start checkpoint, and requires
// three facts from every run:
//
//   - the run returns the same authenticated zero-width end-of-input token;
//   - the token source accepted no scanner token at all during the run
//     (dfaTokenSource.externalTokensProduced), which is stronger than reading
//     the returned token, because Next discards an unusable zero-width
//     external and returns end of input in its place;
//   - the serialized scanner state stays byte-identical.
//
// Together those three say the scanner is quiescent for that head's own row,
// not merely that the lexer handed back end of input. Under that proof each
// head sees exactly the lookahead a live C stack version would see, so the
// frontier shape the compact scheduler observed is C's own frontier shape and
// the error cost rule above decides it.
//
// The proof never widens materiality. selectCompactAcceptanceDerivation keeps
// its own gate, and the admission keeps every other decline.

// compactEOFScannerQuiescenceProof records one completed quiescence proof.
// It is a plain comparable value so the admission receipt can seal it.
type compactEOFScannerQuiescenceProof struct {
	// proved reports that every head state was probed and every probe
	// returned the authenticated end-of-input token with an unchanged
	// serialized scanner state.
	proved bool
	// stateless records that the proof took the stateless arm: the language
	// declared a stateless external scanner and carries no checkpoint, so the
	// payload comparison was vacuous and ExternalScannerIsStateless was
	// trusted. A language that offers a checkpoint always takes the stronger
	// payload comparison instead, whatever it declares (finding F5).
	stateless bool
	// probedStates counts the head states this proof measured. It always
	// equals the frontier width when proved is true.
	probedStates uint8
	// checkpointBefore names the interned scanner checkpoint every probe
	// restored and re-authenticated against. identityFingerprint names the
	// scanner-and-grammar checkpoint identity that authenticated it, or zero
	// on the stateless arm. Both ride the admission receipt's seal so a
	// replayed proof cannot claim a checkpoint it never measured (finding F6).
	checkpointBefore    core.CheckpointID
	identityFingerprint [32]byte
}

// Decline reasons. Every one keeps the historical
// "EOF recovery admission requires scanner quiescence" prefix so the census
// classifier and any operator grep still find this family, and each names the
// step that failed so a later widening starts from a measured cause.
const (
	compactEOFScannerQuiescencePrefix = "EOF recovery admission requires scanner quiescence"

	compactEOFScannerQuiescenceDeclineContext = compactEOFScannerQuiescencePrefix +
		": the probe context is incomplete"
	compactEOFScannerQuiescenceDeclineNoScanner = compactEOFScannerQuiescencePrefix +
		": the language declares external tokens without a probeable scanner"
	compactEOFScannerQuiescenceDeclineElectionChanged = compactEOFScannerQuiescencePrefix +
		": the end-of-input election changed the serialized scanner state"
	compactEOFScannerQuiescenceDeclineHeaderCheckpoint = compactEOFScannerQuiescencePrefix +
		": a head left the shared scanner checkpoint"
	compactEOFScannerQuiescenceDeclineElectionStates = compactEOFScannerQuiescencePrefix +
		": the shared election did not cover every head state"
	compactEOFScannerQuiescenceDeclineContract = compactEOFScannerQuiescencePrefix +
		": the external scanner is neither checkpointed nor stateless"
	compactEOFScannerQuiescenceDeclineSnapshot = compactEOFScannerQuiescencePrefix +
		": the election-start scanner snapshot is unavailable"
	compactEOFScannerQuiescenceDeclineIdentity = compactEOFScannerQuiescencePrefix +
		": the scanner checkpoint identity is incomplete"
	compactEOFScannerQuiescenceDeclinePayload = compactEOFScannerQuiescencePrefix +
		": the election-start scanner payload is unauthenticated"
	compactEOFScannerQuiescenceDeclineLexMode = compactEOFScannerQuiescencePrefix +
		": a head state is outside the lex mode table"
	compactEOFScannerQuiescenceDeclineStateToken = compactEOFScannerQuiescencePrefix +
		": a head state lexes a token of its own at end of input"
	compactEOFScannerQuiescenceDeclineStatePayload = compactEOFScannerQuiescencePrefix +
		": a head state changed the serialized scanner state at end of input"
	compactEOFScannerQuiescenceDeclineWidth = compactEOFScannerQuiescencePrefix +
		": the frontier width exceeds the probe cap"
	compactEOFScannerQuiescenceDeclineStateOffer = compactEOFScannerQuiescencePrefix +
		": the scanner offered a head state a token at end of input"
	compactEOFScannerQuiescenceDeclineWork = compactEOFScannerQuiescencePrefix +
		": the probe exceeded its work budget"
)

// compactEOFScannerQuiescenceMaxStates caps the per-state probe. It is the
// admission's own frontier width, so a later widening must raise both together
// (finding F7).
const compactEOFScannerQuiescenceMaxStates = compactEOFRecoveryAdmissionFrontierWidth

// compactEOFScannerQuiescenceProbeFaults is the test-only probe seam. It is
// nil in every shipped build and is read once per probe run behind one nil
// check, exactly like compactEOFRecoveryAdmissionFaultHook
// (parsercore_phase0_driver.go). It lets a test reach each per-state decline
// reason on a real two-head frontier instead of a hand-built fake one: every
// field rewrites one measurement the probe just took.
type compactEOFScannerQuiescenceProbeFaults struct {
	token   func(StateID, Token) Token
	offered func(StateID, uint32) uint32
	payload func(StateID, bool) bool
}

var compactEOFScannerQuiescenceProbeFaultHook *compactEOFScannerQuiescenceProbeFaults

// compactEOFScannerQuiescenceLastDecline records the most recent decline while
// a probe fault is installed. A shipped build never installs one, so this stays
// empty and the recorder never runs. It lets a fault test read the exact reason
// without depending on the GTS_ADMISSION_CENSUS opt-in, whose cached read can
// already be resolved by an earlier test in the same process.
var compactEOFScannerQuiescenceLastDecline string

func compactEOFScannerQuiescenceRecordDecline(reason string) string {
	if compactEOFScannerQuiescenceProbeFaultHook != nil {
		compactEOFScannerQuiescenceLastDecline = reason
	}
	return reason
}

// compactEOFScannerQuiescenceProbeWindowHook is the test-only perf-boundary
// seam for finding F8. It is nil in every shipped build and is read once per
// probe attempt behind one nil check, exactly like
// compactEOFScannerQuiescenceProbeFaultHook above. proveCompactEOFScannerQuiescence
// calls it with "before" immediately before its per-state Next() loop starts
// and "after" on every exit from that loop, so a test can snapshot perf
// counters at both boundaries and read the delta the loop alone produced,
// unconfounded by the rest of the parse.
var compactEOFScannerQuiescenceProbeWindowHook func(phase string)

// proveCompactEOFScannerQuiescence measures the external scanner at end of
// input, once per head state, and reports whether every head sees the shared
// authenticated end-of-input token under its own lex mode.
//
// The probe is read-only with respect to the parse: it snapshots the token
// source, restores the election-start scanner payload before every run, and
// restores the shared post-election cursor and scanner state on every exit
// path. It publishes no record and mutates no header.
//
// Cost. produceCompactEOFRecoveryAdmission calls this once, after its cheap
// table gates, on the one frontier shape it admits. That frontier collapses to
// a single head as soon as the accept applies, so one parse attempt runs the
// prover at most once and the scanner at most twice. Measured counts, one
// parse each: Scala smoke 1 call and 2 probes; a 16400-byte Scala source with
// 400 object definitions also 1 call and 2 probes; Go, Python, Kotlin, Ruby,
// Bash, and C# smoke samples 0 calls, because none of them reaches this
// frontier. TestEOFRecoveryAdmissionCensusRecordsScannerQuiescenceMechanism
// pins the once-per-parse bound through the admission census, and each probe
// run is accounted in receipt.work.scannerProbes (finding F8).
//
// Every decline returns the zero proof value, so a caller can never read a
// partially populated proof (finding F10).
func (s *diagnosticParserCoreGenericScheduler) proveCompactEOFScannerQuiescence(
	receipt *compactEOFRecoveryAdmissionReceipt,
	language *Language,
	sourceLength uint32,
) (compactEOFScannerQuiescenceProof, string) {
	var zero compactEOFScannerQuiescenceProof
	if s == nil || s.compact == nil || s.tokenSource == nil ||
		s.tokenSource.lexer == nil || language == nil || receipt == nil {
		return zero, compactEOFScannerQuiescenceDeclineContext
	}
	if language.ExternalScanner == nil {
		// A language that declares external tokens without a scanner
		// synthesizes them from the parse tables. There is no scanner to
		// re-run, so there is nothing to prove. Keep declining.
		return zero, compactEOFScannerQuiescenceDeclineNoScanner
	}
	// Step one. The shared end-of-input election must have left the
	// serialized scanner state unchanged. startElection authenticates
	// checkpointBeforeID against the frontier's own checkpoint and then
	// publishes checkpointID from the post-lex payload, so equality here is
	// a byte-exact statement that the end-of-input lex added nothing.
	if s.checkpointBeforeID == 0 || s.checkpointBeforeID != s.checkpointID {
		return zero, compactEOFScannerQuiescenceDeclineElectionChanged
	}
	// Step two. Every head entered that election under the same checkpoint,
	// so both C-equivalent versions carry the same last external token and
	// the same scanner state. Only the parse state differs.
	for _, header := range s.headers {
		if header.checkpoint != s.checkpointID {
			return zero, compactEOFScannerQuiescenceDeclineHeaderCheckpoint
		}
	}
	// Step three. The shared election's state vector must name every head, so
	// the shared lex already saw each head's own external row. The per-state
	// probes below then remove the shared-lex assumption itself.
	states := s.currentElection.States
	if len(states) != len(s.headers) || len(states) == 0 {
		return zero, compactEOFScannerQuiescenceDeclineElectionStates
	}
	if len(states) > compactEOFScannerQuiescenceMaxStates {
		return zero, compactEOFScannerQuiescenceDeclineWidth
	}
	// Step four. The scanner must serialize into an authenticated checkpoint,
	// or declare itself stateless, so the probe can restore the exact
	// election-start state before every run.
	//
	// A checkpoint is the stronger evidence, so take it whenever the language
	// offers one. The stateless arm applies only to a scanner that declares
	// itself stateless AND carries no checkpoint; it then trusts
	// ExternalScannerIsStateless and compares no payload (finding F5).
	contract, contractErr := s.versionLexerScannerContract(language)
	if contractErr != nil || !contract.present {
		return zero, compactEOFScannerQuiescenceDeclineContext
	}
	if !contract.usesCheckpoints && !contract.stateless {
		return zero, compactEOFScannerQuiescenceDeclineContract
	}
	comparePayload := contract.usesCheckpoints
	if !s.versionLexerBeforeValid || s.versionLexerBeforeElection != s.electionIndex ||
		!s.versionLexerBefore.externalScannerPresent {
		return zero, compactEOFScannerQuiescenceDeclineSnapshot
	}
	var identityFingerprint [32]byte
	if comparePayload {
		if len(s.versionLexerBefore.externalPayload) == 0 {
			return zero, compactEOFScannerQuiescenceDeclineSnapshot
		}
		identity, identityOK := s.checkpointIdentityForLanguage(language)
		if !identityOK || !identity.complete() || !s.versionLexerBeforeIdentityValid {
			return zero, compactEOFScannerQuiescenceDeclineIdentity
		}
		identityFingerprint = s.identityFingerprint.fingerprintFor(identity)
		if identityFingerprint != s.versionLexerBeforeIdentity {
			return zero, compactEOFScannerQuiescenceDeclineIdentity
		}
		if !s.compact.CheckpointMatches(s.checkpointBeforeID, s.versionLexerBefore.externalPayload) {
			return zero, compactEOFScannerQuiescenceDeclinePayload
		}
	}

	// Step five. Re-run the scanner once per head state, in isolation.
	d := s.tokenSource
	prior := d.snapshotRelexStateWithScratch(&s.relexPriorScratch)
	priorState := d.state
	priorGLRStates := d.glrStates
	// Route every Next() call below to the probe's own perf counters, not the
	// parse's (finding F8): this loop reruns the scanner outside the parse, so
	// a perf-instrumented build must not bill it to the parse's lexed count.
	d.quiescenceProbing = true
	if compactEOFScannerQuiescenceProbeWindowHook != nil {
		compactEOFScannerQuiescenceProbeWindowHook("before")
	}
	defer func() {
		d.quiescenceProbing = false
		prior.restore(d)
		d.SetParserState(priorState)
		d.SetGLRStates(priorGLRStates)
		if compactEOFScannerQuiescenceProbeWindowHook != nil {
			compactEOFScannerQuiescenceProbeWindowHook("after")
		}
	}()
	var proof compactEOFScannerQuiescenceProof
	proof.stateless = !comparePayload
	for _, state := range states {
		if int(state) >= len(language.LexModes) {
			return zero, compactEOFScannerQuiescenceRecordDecline(compactEOFScannerQuiescenceDeclineLexMode)
		}
		s.versionLexerBefore.restore(d)
		// Clear the same-position zero-width external mask before the run
		// (finding F1). nextExternalToken drops every masked symbol from the
		// row when the cursor and the parse state match the recorded pair, and
		// only reset, Close, and beginRelexAt clear that mask. An earlier
		// election can therefore have masked a symbol this head's own row
		// still contains, and the probe would never ask the scanner for it
		// while C would. Clearing widens the row the scanner sees, so the
		// probe can only decline more often, never less. The deferred restore
		// and the per-run restore above both put the mask back.
		d.extZeroPos = -1
		d.extZeroState = 0
		d.extZeroTried = d.extZeroTried[:0]
		d.zeroWidthPos = -1
		d.zeroWidthCount = 0
		d.SetParserState(state)
		// SetGLRStates(nil) is the necessary step: it forces the token source
		// onto this one state's external_lex_state row, exactly as C derives
		// valid_external_tokens for one stack version.
		d.SetGLRStates(nil)
		if err := compactEOFRecoveryAdmissionAddWork(s, receipt, &receipt.work.scannerProbes, 1); err != nil {
			return zero, compactEOFScannerQuiescenceDeclineWork
		}
		candidate := d.Next()
		offered := d.externalTokensProduced
		faults := compactEOFScannerQuiescenceProbeFaultHook
		if faults != nil {
			if faults.token != nil {
				candidate = faults.token(state, candidate)
			}
			if faults.offered != nil {
				offered = faults.offered(state, offered)
			}
		}
		if candidate.Symbol != 0 || candidate.ExternalScannerToken || candidate.Missing ||
			candidate.NoLookahead || candidate.StartByte != sourceLength ||
			candidate.EndByte != sourceLength {
			return zero, compactEOFScannerQuiescenceRecordDecline(compactEOFScannerQuiescenceDeclineStateToken)
		}
		// The returned token alone is not enough (finding F2). Next discards an
		// unusable zero-width external and returns end of input in its place,
		// so a head could be offered a token the probe never sees. Require the
		// token source's own count of accepted scanner tokens to be zero: that
		// is the direct statement "the scanner produced nothing for this row".
		if offered != 0 {
			return zero, compactEOFScannerQuiescenceRecordDecline(compactEOFScannerQuiescenceDeclineStateOffer)
		}
		if comparePayload {
			after := d.snapshotRelexStateWithScratch(&s.relexAfterScratch)
			payloadMatches := s.compact.CheckpointMatches(s.checkpointBeforeID, after.externalPayload)
			if faults != nil && faults.payload != nil {
				payloadMatches = faults.payload(state, payloadMatches)
			}
			if !payloadMatches {
				return zero, compactEOFScannerQuiescenceRecordDecline(compactEOFScannerQuiescenceDeclineStatePayload)
			}
		}
		proof.probedStates++
	}
	proof.proved = true
	proof.checkpointBefore = s.checkpointBeforeID
	proof.identityFingerprint = identityFingerprint
	return proof, ""
}

// compactEOFScannerQuiescenceExternalAdmitted reports whether one subtree on
// an admitted head's exact path is a zero-width external token.
//
// This is the only external payload the admission path walk accepts, and only
// under a completed quiescence proof. Zero width is the necessary
// condition: such a token owns no source byte, so it can add nothing to the
// recovery span cost, hide no unconsumed byte from the accepted-tail proof,
// and reconstruct no text the metadata walk cannot already read. A
// positive-width external token owns bytes whose shape depends on the scanner
// itself, so it stays rejected. Extras stay rejected, missing subtrees stay
// rejected, and every non-terminal external payload stays rejected.
//
// Admitting the token is only half the rule. produceCompactEOFRecoveryAdmission
// also requires both heads to present the identical ordered sequence of these
// tokens (compactEOFRecoveryAdmissionExternalDigest below), which proves the
// two heads shifted the same external tokens at the same offsets and therefore
// forked on an ordinary grammar conflict rather than on the scanner.
func compactEOFScannerQuiescenceExternalAdmitted(view core.EOFAdmissionSubtreeView) bool {
	return view.External && view.Terminal && !view.Extra && !view.Missing &&
		view.StartByte == view.EndByte &&
		len(view.Children) == 0 && len(view.Fields) == 0 && len(view.Aliases) == 0
}
