package gotreesitter

// This file opens the D2 first-leaf token-cache lane
// (spec.derivation-set-equivalence.v1, section 3) with a literal port of
// C's ts_parser__can_reuse_first_leaf (parser.c:472-505 at C revision
// f5afe475, 0.25.1; the function is byte-identical in the locked 0.27.1
// oracle, 10eb5ac1, where it sits at parser.c:470). The predicate decides whether a token lexed under one
// parse state's lexer mode is reusable under another state without
// re-lexing. Nothing consumes it yet; the per-epoch token cell and the
// scheduler election land in later tranches of the same lane.

// tokenReuseLeaf carries the leaf facts C reads from a Subtree, split
// across two different accessors exactly as
// ts_parser__can_reuse_first_leaf (parser.c:472-505) reads them: Symbol
// and LeafState come from the first leaf (ts_subtree_leaf_symbol and
// ts_subtree_leaf_parse_state, which both read through the subtree's
// leftmost leaf); ParseState, SizeBytes, and IsKeyword come from the root
// subtree itself (ts_subtree_parse_state, ts_subtree_size, and
// ts_subtree_is_keyword). A bare token cell sets LeafState and ParseState
// equal; a reused subtree keeps them distinct (the first leaf's lex state
// against the root's parse state, per ts_subtree_leaf_parse_state and
// ts_subtree_parse_state).
type tokenReuseLeaf struct {
	Symbol     Symbol
	LeafState  StateID
	ParseState StateID
	SizeBytes  uint32
	IsKeyword  bool
}

// lexerModeTriple starts from C's TSLexerMode (parser.h:89-93), the three
// fields the C reuse test compares with memcmp, but widens and extends it
// for this engine's own table layout rather than reading Go's LexMode raw
// fields directly:
//
//   - lexState uses LexStateIndex() (uint32), not the raw uint16 LexState
//     field, because grammargen tables can exceed 64K lexer states and the
//     widened index is the only field that stays collision-free at that
//     size (Language.LexStateID / LexMode.LexStateIndex's own doc comment).
//   - afterWhitespaceLexState (via AfterWhitespaceLexStateIndex()) is
//     included even though C's TSLexerMode has no equivalent field: in this
//     engine, after-whitespace is an independent table entry populated by a
//     separate pass over the DFA (grammargen/assemble.go:138-142), so two
//     states with an identical C triple can still lex differently here if
//     their after-whitespace entries differ. Leaving it out of the identity
//     would let the reuse rule below fire on a match C never has to prove
//     safe. Including it keeps the comparison aligned with actual lexer
//     behavior and errs fail-closed relative to C rather than risking an
//     unsound reuse.
type lexerModeTriple struct {
	lexState                uint32
	externalLexState        uint16
	reservedWordSetID       uint16
	afterWhitespaceLexState uint32
}

func lexerModeTripleForState(lang *Language, state StateID) (lexerModeTriple, bool) {
	if lang == nil || int(state) >= len(lang.LexModes) {
		return lexerModeTriple{}, false
	}
	mode := lang.LexModes[state]
	return lexerModeTriple{
		lexState:                mode.LexStateIndex(),
		externalLexState:        mode.ExternalLexState,
		reservedWordSetID:       mode.ReservedWordSetID,
		afterWhitespaceLexState: mode.AfterWhitespaceLexStateIndex(),
	}, true
}

// canReuseFirstLeaf ports ts_parser__can_reuse_first_leaf clause for
// clause; the rule order and every short-circuit match the C source. The
// only addition is a fail-closed false when either state has no lexer-mode
// row, a case C's complete tables cannot present.
func canReuseFirstLeaf(lang *Language, state StateID, leaf tokenReuseLeaf, entry ParseActionEntry) bool {
	currentMode, ok := lexerModeTripleForState(lang, state)
	if !ok {
		return false
	}
	leafMode, ok := lexerModeTripleForState(lang, leaf.LeafState)
	if !ok {
		return false
	}

	// parser.c:487 -- never reuse at the end of a non-terminal extra. C's
	// sentinel is (uint16_t)-1 on the raw LexState field; this engine's
	// package-wide noLookaheadLexState sentinel is the same value widened
	// through LexStateIndex() (parser_dfa_token_source.go), so comparing
	// against it here follows the same convention every other no-lookahead
	// check in this package uses instead of duplicating a private sentinel.
	if currentMode.lexState == noLookaheadLexState {
		return false
	}

	// parser.c:490-497 -- a token created under the same lookahead set is
	// reusable, unless it is the keyword-capture token and either was
	// adopted as a keyword or came from a different parse state.
	if len(entry.Actions) > 0 && leafMode == currentMode &&
		(leaf.Symbol != lang.KeywordCaptureToken ||
			(!leaf.IsKeyword && leaf.ParseState == state)) {
		return true
	}

	// parser.c:500 -- zero-width tokens other than end-of-file never cross
	// into a different lookahead set.
	if leaf.SizeBytes == 0 && leaf.Symbol != 0 {
		return false
	}

	// parser.c:504 -- otherwise reuse requires a lex context with no
	// external scanner and the table entry's reusable bit.
	return currentMode.externalLexState == 0 && entry.Reusable
}
