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
// for the whole frontier under the union of every head state's row. A scanner
// that reads valid_symbols non-monotonically could therefore offer the
// no-action head a token under its own row that the union election never saw.
// The head would not be dead in C, and the compact accept would publish the
// wrong tree.
//
// compactEOFScannerQuiescenceProof closes that gap by measurement instead of
// assumption. It re-runs the external scanner once per head state, in
// isolation, from the authenticated election-start checkpoint, and requires
// every run to return the same authenticated end-of-input token and to leave
// the serialized scanner state byte-identical. Under that proof each head sees
// exactly the lookahead a live C stack version would see, so the frontier
// shape the compact scheduler observed is C's own frontier shape and the error
// cost rule above decides it.
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
	// stateless records that the language declared a stateless external
	// scanner, so the payload comparison was vacuous by contract.
	stateless bool
	// probedStates counts the head states this proof measured. It always
	// equals the frontier width when proved is true.
	probedStates uint8
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
)

// compactEOFScannerQuiescenceMaxStates caps the per-state probe. The admission
// itself admits exactly two heads; the cap keeps the bound explicit and keeps
// a later frontier widening from silently paying an unbounded scanner cost.
const compactEOFScannerQuiescenceMaxStates = 2

// proveCompactEOFScannerQuiescence measures the external scanner at end of
// input, once per head state, and reports whether every head sees the shared
// authenticated end-of-input token under its own lex mode.
//
// The probe is read-only with respect to the parse: it snapshots the token
// source, restores the election-start scanner payload before every run, and
// restores the shared post-election cursor and scanner state on every exit
// path. It publishes no record and mutates no header.
func (s *diagnosticParserCoreGenericScheduler) proveCompactEOFScannerQuiescence(
	language *Language,
	sourceLength uint32,
) (compactEOFScannerQuiescenceProof, string) {
	var proof compactEOFScannerQuiescenceProof
	if s == nil || s.compact == nil || s.tokenSource == nil ||
		s.tokenSource.lexer == nil || language == nil {
		return proof, compactEOFScannerQuiescenceDeclineContext
	}
	if language.ExternalScanner == nil {
		// A language that declares external tokens without a scanner
		// synthesizes them from the parse tables. There is no scanner to
		// re-run, so there is nothing to prove. Keep declining.
		return proof, compactEOFScannerQuiescenceDeclineNoScanner
	}
	// Step one. The shared end-of-input election must have left the
	// serialized scanner state unchanged. startElection authenticates
	// checkpointBeforeID against the frontier's own checkpoint and then
	// publishes checkpointID from the post-lex payload, so equality here is
	// a byte-exact statement that the end-of-input lex added nothing.
	if s.checkpointBeforeID == 0 || s.checkpointBeforeID != s.checkpointID {
		return proof, compactEOFScannerQuiescenceDeclineElectionChanged
	}
	// Step two. Every head entered that election under the same checkpoint,
	// so both C-equivalent versions carry the same last external token and
	// the same scanner state. Only the parse state differs.
	for _, header := range s.headers {
		if header.checkpoint != s.checkpointID {
			return proof, compactEOFScannerQuiescenceDeclineHeaderCheckpoint
		}
	}
	// Step three. The shared election's state vector must name every head, so
	// the union lex already saw each head's own external row. The per-state
	// probes below then remove the union assumption itself.
	states := s.currentElection.States
	if len(states) != len(s.headers) || len(states) == 0 {
		return proof, compactEOFScannerQuiescenceDeclineElectionStates
	}
	if len(states) > compactEOFScannerQuiescenceMaxStates {
		return proof, compactEOFScannerQuiescenceDeclineWidth
	}
	// Step four. The scanner must serialize into an authenticated checkpoint,
	// or declare itself stateless, so the probe can restore the exact
	// election-start state before every run.
	contract, contractErr := s.versionLexerScannerContract(language)
	if contractErr != nil || !contract.present {
		return compactEOFScannerQuiescenceProof{}, compactEOFScannerQuiescenceDeclineContext
	}
	if !contract.usesCheckpoints && !contract.stateless {
		return compactEOFScannerQuiescenceProof{}, compactEOFScannerQuiescenceDeclineContract
	}
	proof.stateless = contract.stateless
	if !s.versionLexerBeforeValid || s.versionLexerBeforeElection != s.electionIndex ||
		!s.versionLexerBefore.externalScannerPresent {
		return compactEOFScannerQuiescenceProof{}, compactEOFScannerQuiescenceDeclineSnapshot
	}
	if !contract.stateless {
		if len(s.versionLexerBefore.externalPayload) == 0 {
			return compactEOFScannerQuiescenceProof{}, compactEOFScannerQuiescenceDeclineSnapshot
		}
		identity, identityOK := s.checkpointIdentityForLanguage(language)
		if !identityOK || !identity.complete() || !s.versionLexerBeforeIdentityValid ||
			s.identityFingerprint.fingerprintFor(identity) != s.versionLexerBeforeIdentity {
			return compactEOFScannerQuiescenceProof{}, compactEOFScannerQuiescenceDeclineIdentity
		}
		if !s.compact.CheckpointMatches(s.checkpointBeforeID, s.versionLexerBefore.externalPayload) {
			return compactEOFScannerQuiescenceProof{}, compactEOFScannerQuiescenceDeclinePayload
		}
	}

	// Step five. Re-run the scanner once per head state, in isolation.
	d := s.tokenSource
	prior := d.snapshotRelexStateWithScratch(&s.relexPriorScratch)
	priorState := d.state
	priorGLRStates := d.glrStates
	defer func() {
		prior.restore(d)
		d.SetParserState(priorState)
		d.SetGLRStates(priorGLRStates)
	}()
	for _, state := range states {
		if int(state) >= len(language.LexModes) {
			return compactEOFScannerQuiescenceProof{}, compactEOFScannerQuiescenceDeclineLexMode
		}
		s.versionLexerBefore.restore(d)
		d.SetParserState(state)
		// SetGLRStates(nil) is the necessary step: it forces
		// nextExternalToken onto this one state's external_lex_state row,
		// exactly as C derives valid_external_tokens for one stack version.
		d.SetGLRStates(nil)
		candidate := d.Next()
		if candidate.Symbol != 0 || candidate.ExternalScannerToken || candidate.Missing ||
			candidate.NoLookahead || candidate.StartByte != sourceLength ||
			candidate.EndByte != sourceLength {
			return compactEOFScannerQuiescenceProof{}, compactEOFScannerQuiescenceDeclineStateToken
		}
		if !contract.stateless {
			after := d.snapshotRelexStateWithScratch(&s.relexAfterScratch)
			if !s.compact.CheckpointMatches(s.checkpointBeforeID, after.externalPayload) {
				return compactEOFScannerQuiescenceProof{}, compactEOFScannerQuiescenceDeclineStatePayload
			}
		}
		proof.probedStates++
	}
	proof.proved = true
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
