package incr

import "bytes"

// EOFExtraTokenSource authenticates an existing extra terminal ending at EOF.
// Its scan must be independent of parser state, and appending an ASCII word
// byte to its body must preserve every preceding token at every parser state.
// This excludes lexers whose earlier lookahead reads the terminal's body.
type EOFExtraTokenSource interface {
	// Nonzero identities distinguish lexical policies within a language.
	// Wrappers that change token election must change identity or decline the
	// capability. The opaque language must identify the lexer's exact grammar;
	// a different language or zero identity supplies no proof.
	EOFExtraTokenProofID(language any) uint8
	EOFExtraTokenAppendInvariant(oldSource, source []byte, symbol uint16, start uint32) bool
}

func EOFExtraProofID(ts, language any) uint8 {
	if proof, ok := ts.(EOFExtraTokenSource); ok {
		return proof.EOFExtraTokenProofID(language)
	}
	return 0
}

// EOFExtraAppend proves the source changed only by one ASCII word byte at EOF.
// A surviving terminal and its lexer-owned dependency proof are still needed.
// Limiting growth to word bytes also keeps delimiter and newline rules outside
// this proof; deleting or replacing text must use ordinary verification.
func EOFExtraAppend(ts, language any, oldProof uint8, oldSource, source []byte, e TokenEdit, symbol uint16, start uint32) bool {
	proof, ok := ts.(EOFExtraTokenSource)
	if !ok || oldProof == 0 || oldProof != proof.EOFExtraTokenProofID(language) ||
		uint64(len(oldSource)) >= uint64(^uint32(0)) || len(source) != len(oldSource)+1 ||
		e.Start != uint32(len(oldSource)) || e.OldEnd != e.Start || e.NewEnd != uint32(len(source)) ||
		start >= e.Start || !bytes.Equal(oldSource, source[:len(oldSource)]) {
		return false
	}
	b := source[len(oldSource)]
	if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_') {
		return false
	}
	return proof.EOFExtraTokenAppendInvariant(oldSource, source, symbol, start)
}
