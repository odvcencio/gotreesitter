//go:build gts_workcount && (!grammar_subset || grammar_subset_c || grammar_subset_cpp)

package grammars

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"

	"github.com/odvcencio/gotreesitter"
)

// ForkTokenSourceForDiagnostic copies the current C token-source state.
// The fork shares only immutable source bytes, language data, and lexer tables.
func (ts *CTokenSource) ForkTokenSourceForDiagnostic() (gotreesitter.TokenSource, gotreesitter.DiagnosticTokenSourceForkReceipt, error) {
	if ts == nil {
		return nil, gotreesitter.DiagnosticTokenSourceForkReceipt{}, fmt.Errorf("diagnostic C token fork: source is nil")
	}
	if ts.cur.offset < 0 || ts.cur.offset > len(ts.cur.src) {
		return nil, gotreesitter.DiagnosticTokenSourceForkReceipt{}, fmt.Errorf("diagnostic C token fork: cursor %d is outside source length %d", ts.cur.offset, len(ts.cur.src))
	}
	if uint64(ts.cur.offset) > uint64(^uint32(0)) {
		return nil, gotreesitter.DiagnosticTokenSourceForkReceipt{}, fmt.Errorf("diagnostic C token fork: cursor %d exceeds the token offset limit", ts.cur.offset)
	}
	if len(ts.src) != len(ts.cur.src) || (len(ts.src) > 0 && &ts.src[0] != &ts.cur.src[0]) {
		return nil, gotreesitter.DiagnosticTokenSourceForkReceipt{}, fmt.Errorf("diagnostic C token fork: source cursor does not share the immutable source")
	}

	fork := *ts
	fork.pending = diagnosticCloneCTokens(ts.pending)
	fork.glrStates = diagnosticCloneCStates(ts.glrStates)

	receipt := gotreesitter.DiagnosticTokenSourceForkReceipt{
		TokenSourceKind:    "c",
		CursorByte:         uint32(ts.cur.offset),
		CursorPoint:        ts.cur.point(),
		ParserState:        ts.parserState,
		ActiveGLRStates:    diagnosticCloneCStates(ts.glrStates),
		PendingTokenCount:  len(ts.pending),
		PendingTokenDigest: diagnosticCTokenDigest(ts.pending),
		ZeroWidthOffset:    int64(ts.lastSyntheticOffset),
		IncludedRangeIndex: -1,
	}
	return &fork, receipt, nil
}

func diagnosticCloneCTokens(tokens []gotreesitter.Token) []gotreesitter.Token {
	if tokens == nil {
		return nil
	}
	clone := make([]gotreesitter.Token, len(tokens))
	copy(clone, tokens)
	return clone
}

func diagnosticCloneCStates(states []gotreesitter.StateID) []gotreesitter.StateID {
	if states == nil {
		return nil
	}
	clone := make([]gotreesitter.StateID, len(states))
	copy(clone, states)
	return clone
}

func diagnosticCTokenDigest(tokens []gotreesitter.Token) [sha256.Size]byte {
	digest := sha256.New()
	diagnosticWriteUint64(digest, uint64(len(tokens)))
	for _, token := range tokens {
		diagnosticWriteUint64(digest, uint64(token.Symbol))
		diagnosticWriteUint64(digest, uint64(len(token.Text)))
		_, _ = digest.Write([]byte(token.Text))
		diagnosticWriteUint64(digest, uint64(token.StartByte))
		diagnosticWriteUint64(digest, uint64(token.EndByte))
		diagnosticWriteUint64(digest, uint64(token.StartPoint.Row))
		diagnosticWriteUint64(digest, uint64(token.StartPoint.Column))
		diagnosticWriteUint64(digest, uint64(token.EndPoint.Row))
		diagnosticWriteUint64(digest, uint64(token.EndPoint.Column))
		diagnosticWriteBool(digest, token.Missing)
		diagnosticWriteBool(digest, token.NoLookahead)
		diagnosticWriteBool(digest, token.ExternalScannerToken)
		diagnosticWriteUint64(digest, uint64(token.ExternalScannerStartByte))
	}
	var out [sha256.Size]byte
	copy(out[:], digest.Sum(nil))
	return out
}

func diagnosticWriteUint64(digest hash.Hash, value uint64) {
	var encoded [8]byte
	binary.LittleEndian.PutUint64(encoded[:], value)
	_, _ = digest.Write(encoded[:])
}

func diagnosticWriteBool(digest hash.Hash, value bool) {
	if value {
		_, _ = digest.Write([]byte{1})
		return
	}
	_, _ = digest.Write([]byte{0})
}
