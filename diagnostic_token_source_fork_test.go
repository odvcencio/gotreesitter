//go:build gts_workcount

package gotreesitter

import (
	"crypto/sha256"
	"reflect"
	"testing"
)

type diagnosticForkStub struct {
	tokens    []Token
	index     int
	state     StateID
	glrStates []StateID
}

func (s *diagnosticForkStub) Next() Token {
	if s.index >= len(s.tokens) {
		return Token{}
	}
	token := s.tokens[s.index]
	s.index++
	return token
}

func (s *diagnosticForkStub) SetParserState(state StateID) {
	s.state = state
}

func (s *diagnosticForkStub) SetGLRStates(states []StateID) {
	s.glrStates = append(s.glrStates[:0], states...)
}

func (s *diagnosticForkStub) ForkTokenSourceForDiagnostic() (TokenSource, DiagnosticTokenSourceForkReceipt, error) {
	fork := *s
	fork.tokens = append([]Token(nil), s.tokens...)
	fork.glrStates = append([]StateID(nil), s.glrStates...)
	return &fork, DiagnosticTokenSourceForkReceipt{
		TokenSourceKind:                  "stub",
		ParserState:                      s.state,
		ActiveGLRStates:                  append([]StateID(nil), s.glrStates...),
		IncludedRangeIndex:               -1,
		ExternalScannerCheckpointPresent: true,
		ExternalScannerCheckpointDigest:  sha256.Sum256([]byte("stub scanner")),
	}, nil
}

func TestDiagnosticIncludedRangeForkIsIndependent(t *testing.T) {
	base := &diagnosticForkStub{
		tokens: []Token{
			{Symbol: 1, Text: "outside", StartByte: 0, EndByte: 7},
			{Symbol: 2, Text: "first", StartByte: 12, EndByte: 17},
			{Symbol: 3, Text: "second", StartByte: 31, EndByte: 37},
			{Symbol: 4, Text: "third", StartByte: 38, EndByte: 39},
			{},
		},
		state:     77,
		glrStates: []StateID{7, 11},
	}
	ranges := []Range{
		{StartByte: 10, EndByte: 20, StartPoint: Point{Column: 10}, EndPoint: Point{Column: 20}},
		{StartByte: 30, EndByte: 40, StartPoint: Point{Row: 1}, EndPoint: Point{Row: 1, Column: 10}},
	}
	live := newIncludedRangeTokenSource(base, ranges).(*includedRangeTokenSource)
	if token := live.Next(); token.Symbol != 2 {
		t.Fatalf("first included token symbol = %d, want 2", token.Symbol)
	}
	if live.idx != 0 {
		t.Fatalf("live range index = %d, want 0", live.idx)
	}

	liveBaseIndex := base.index
	liveRanges := append([]Range(nil), live.ranges...)
	forkSource, receipt, err := forkTokenSourceForDiagnostic(live)
	if err != nil {
		t.Fatalf("forkTokenSourceForDiagnostic failed: %v", err)
	}
	fork, ok := forkSource.(*includedRangeTokenSource)
	if !ok {
		t.Fatalf("fork type = %T, want *includedRangeTokenSource", forkSource)
	}
	forkBase, ok := fork.base.(*diagnosticForkStub)
	if !ok {
		t.Fatalf("fork base type = %T, want *diagnosticForkStub", fork.base)
	}
	if !reflect.DeepEqual(fork, live) {
		t.Fatalf("fork state differs before advance:\n got: %#v\nwant: %#v", fork, live)
	}
	if fork == live || forkBase == base {
		t.Fatal("fork shares a mutable source object with the live source")
	}
	if &fork.ranges[0] == &live.ranges[0] {
		t.Fatal("fork shares the included-range backing array")
	}
	if &forkBase.tokens[0] == &base.tokens[0] {
		t.Fatal("fork shares the base token backing array")
	}
	if &forkBase.glrStates[0] == &base.glrStates[0] {
		t.Fatal("fork shares the base GLR-state backing array")
	}
	if got, want := receipt.TokenSourceKind, "included-range/stub"; got != want {
		t.Fatalf("receipt kind = %q, want %q", got, want)
	}
	if receipt.IncludedRangeIndex != live.idx || receipt.IncludedRangeCount != len(live.ranges) {
		t.Fatalf("receipt range state = %d/%d, want %d/%d", receipt.IncludedRangeIndex, receipt.IncludedRangeCount, live.idx, len(live.ranges))
	}
	if receipt.ParserState != base.state {
		t.Fatalf("receipt parser state = %d, want %d", receipt.ParserState, base.state)
	}
	if !reflect.DeepEqual(receipt.ActiveGLRStates, base.glrStates) {
		t.Fatalf("receipt GLR states = %v, want %v", receipt.ActiveGLRStates, base.glrStates)
	}
	if got, want := receipt.IncludedRangeDigest, diagnosticIncludedRangeDigest(live.ranges); got != want {
		t.Fatalf("receipt range digest = %x, want %x", got, want)
	}
	if !receipt.ExternalScannerCheckpointPresent {
		t.Fatal("wrapper discarded the base scanner checkpoint state")
	}

	fork.SetParserState(99)
	fork.SetGLRStates([]StateID{13, 17})
	if base.state != 77 || !reflect.DeepEqual(base.glrStates, []StateID{7, 11}) {
		t.Fatalf("fork state update changed live base: state=%d glr=%v", base.state, base.glrStates)
	}
	forkToken := fork.Next()
	if forkToken.Symbol != 3 || fork.idx != 1 {
		t.Fatalf("fork next state = symbol %d at range %d, want symbol 3 at range 1", forkToken.Symbol, fork.idx)
	}
	if base.index != liveBaseIndex || live.idx != 0 || !reflect.DeepEqual(live.ranges, liveRanges) {
		t.Fatalf("fork advance changed live state: base index=%d range index=%d ranges=%v", base.index, live.idx, live.ranges)
	}
	if liveToken := live.Next(); !reflect.DeepEqual(liveToken, forkToken) {
		t.Fatalf("live next token = %+v, want fork token %+v", liveToken, forkToken)
	}

	liveBaseIndex = base.index
	forkToken = fork.Next()
	if forkToken.Symbol != 4 {
		t.Fatalf("fork next symbol = %d, want 4", forkToken.Symbol)
	}
	if base.index != liveBaseIndex || live.idx != 1 {
		t.Fatalf("second fork advance changed live state: base index=%d range index=%d", base.index, live.idx)
	}
	if liveToken := live.Next(); !reflect.DeepEqual(liveToken, forkToken) {
		t.Fatalf("second live token = %+v, want fork token %+v", liveToken, forkToken)
	}

	fork.ranges[0].EndByte++
	if reflect.DeepEqual(fork.ranges, live.ranges) {
		t.Fatal("fork range mutation reached the live wrapper")
	}
}

func TestDiagnosticIncludedRangeForkRejectsUnsupportedBase(t *testing.T) {
	base := &stubTokenSource{}
	live := newIncludedRangeTokenSource(base, []Range{{StartByte: 1, EndByte: 2}}).(*includedRangeTokenSource)
	indexBefore := live.idx
	if _, _, err := forkTokenSourceForDiagnostic(live); err == nil {
		t.Fatal("fork with an unsupported base succeeded")
	}
	if base.nextCalls != 0 || base.skipCalls != 0 || live.idx != indexBefore {
		t.Fatalf("rejected fork changed live source: next=%d skip=%d range index=%d, want 0/0/%d", base.nextCalls, base.skipCalls, live.idx, indexBefore)
	}
}
