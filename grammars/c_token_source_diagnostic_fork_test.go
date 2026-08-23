//go:build gts_workcount && (!grammar_subset || grammar_subset_c || grammar_subset_cpp)

package grammars

import (
	"bytes"
	"crypto/sha256"
	"reflect"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

type cDiagnosticMutableState struct {
	cursor                  sourceCursor
	done                    bool
	pending                 []gotreesitter.Token
	preprocState            int
	parserState             gotreesitter.StateID
	glrStates               []gotreesitter.StateID
	lastSyntheticOffset     int
	preprocDefineNameEnd    int
	preprocOpaqueArgPending bool
	preprocOpaqueArgActive  bool
}

func captureCDiagnosticMutableState(source *CTokenSource) cDiagnosticMutableState {
	return cDiagnosticMutableState{
		cursor:                  source.cur,
		done:                    source.done,
		pending:                 append([]gotreesitter.Token(nil), source.pending...),
		preprocState:            source.preprocState,
		parserState:             source.parserState,
		glrStates:               append([]gotreesitter.StateID(nil), source.glrStates...),
		lastSyntheticOffset:     source.lastSyntheticOffset,
		preprocDefineNameEnd:    source.preprocDefineNameEnd,
		preprocOpaqueArgPending: source.preprocOpaqueArgPending,
		preprocOpaqueArgActive:  source.preprocOpaqueArgActive,
	}
}

func assertCDiagnosticStateEqual(t *testing.T, got, want cDiagnosticMutableState) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("C token-source state changed:\n got: %#v\nwant: %#v", got, want)
	}
}

func assertCDiagnosticForkStateEqual(t *testing.T, fork, live *CTokenSource) {
	t.Helper()
	if !reflect.DeepEqual(fork, live) {
		t.Fatalf("C fork state differs before advance:\n got: %#v\nwant: %#v", fork, live)
	}
}

func advanceCDiagnosticTokens(source *CTokenSource, count int) []gotreesitter.Token {
	out := make([]gotreesitter.Token, count)
	for index := range out {
		out[index] = source.Next()
	}
	return out
}

func requireCDiagnosticTokenSequenceEqual(t *testing.T, got, want []gotreesitter.Token) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("token sequence differs:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestCTokenSourceDiagnosticForkPreservesPendingTokens(t *testing.T) {
	lang := CLanguage()
	sourceBytes := []byte("const char *value = \"a\\n\"; int tail;\n")
	live, err := NewCTokenSource(sourceBytes, lang)
	if err != nil {
		t.Fatalf("NewCTokenSource failed: %v", err)
	}
	t.Cleanup(live.Close)
	live.SetParserState(101)
	live.SetGLRStates([]gotreesitter.StateID{4, 8, 15})

	foundOpener := false
	for index := 0; index < 12; index++ {
		if token := live.Next(); token.Text == "\"" {
			foundOpener = true
			break
		}
	}
	if !foundOpener {
		t.Fatal("setup did not reach a string opener")
	}
	if len(live.pending) < 2 {
		t.Fatalf("pending token count = %d, want at least 2", len(live.pending))
	}
	liveBefore := captureCDiagnosticMutableState(live)
	sourceBefore := append([]byte(nil), live.src...)

	forkSource, receipt, err := live.ForkTokenSourceForDiagnostic()
	if err != nil {
		t.Fatalf("ForkTokenSourceForDiagnostic failed: %v", err)
	}
	fork, ok := forkSource.(*CTokenSource)
	if !ok {
		t.Fatalf("fork type = %T, want *CTokenSource", forkSource)
	}
	t.Cleanup(fork.Close)
	assertCDiagnosticForkStateEqual(t, fork, live)
	if fork == live {
		t.Fatal("fork shares the live C token-source object")
	}
	if &fork.pending[0] == &live.pending[0] {
		t.Fatal("fork shares the pending-token backing array")
	}
	if &fork.glrStates[0] == &live.glrStates[0] {
		t.Fatal("fork shares the GLR-state backing array")
	}
	if &receipt.ActiveGLRStates[0] == &live.glrStates[0] || &receipt.ActiveGLRStates[0] == &fork.glrStates[0] {
		t.Fatal("receipt shares a mutable GLR-state backing array")
	}
	if got, want := receipt.TokenSourceKind, "c"; got != want {
		t.Fatalf("receipt kind = %q, want %q", got, want)
	}
	if receipt.CursorByte != uint32(live.cur.offset) || receipt.CursorPoint != live.cur.point() {
		t.Fatalf("receipt cursor = %d/%+v, want %d/%+v", receipt.CursorByte, receipt.CursorPoint, live.cur.offset, live.cur.point())
	}
	if receipt.ParserState != live.parserState {
		t.Fatalf("receipt parser state = %d, want %d", receipt.ParserState, live.parserState)
	}
	if !reflect.DeepEqual(receipt.ActiveGLRStates, live.glrStates) {
		t.Fatalf("receipt GLR states = %v, want %v", receipt.ActiveGLRStates, live.glrStates)
	}
	if receipt.PendingTokenCount != len(live.pending) || receipt.PendingTokenDigest != diagnosticCTokenDigest(live.pending) {
		t.Fatalf("receipt pending state = %d/%x, want %d/%x", receipt.PendingTokenCount, receipt.PendingTokenDigest, len(live.pending), diagnosticCTokenDigest(live.pending))
	}
	if receipt.ExternalScannerCheckpointPresent || receipt.ExternalScannerCheckpointDigest != [sha256.Size]byte{} {
		t.Fatalf("C receipt reports external scanner state: present=%v digest=%x", receipt.ExternalScannerCheckpointPresent, receipt.ExternalScannerCheckpointDigest)
	}

	forkTokens := advanceCDiagnosticTokens(fork, 7)
	assertCDiagnosticStateEqual(t, captureCDiagnosticMutableState(live), liveBefore)
	if !bytes.Equal(live.src, sourceBefore) {
		t.Fatalf("fork advance changed live source bytes: got %q, want %q", live.src, sourceBefore)
	}
	liveTokens := advanceCDiagnosticTokens(live, 7)
	requireCDiagnosticTokenSequenceEqual(t, forkTokens, liveTokens)

	fork.glrStates[0] = 99
	receipt.ActiveGLRStates[0] = 100
	if live.glrStates[0] != 4 {
		t.Fatalf("fork or receipt mutation changed live GLR state to %d", live.glrStates[0])
	}
}

func TestCTokenSourceDiagnosticForkPreservesPreprocessorState(t *testing.T) {
	lang := CLanguage()
	sourceBytes := []byte("#if __has_include(<x>)\nint enabled;\n")
	live, err := NewCTokenSource(sourceBytes, lang)
	if err != nil {
		t.Fatalf("NewCTokenSource failed: %v", err)
	}
	t.Cleanup(live.Close)

	setup := advanceCDiagnosticTokens(live, 3)
	if got, want := setup[2].Text, "("; got != want {
		t.Fatalf("third setup token text = %q, want %q", got, want)
	}
	if live.preprocState != cPreprocConditionalExpr || !live.preprocOpaqueArgActive || live.preprocOpaqueArgPending {
		t.Fatalf("setup preprocessor state = %d/%v/%v", live.preprocState, live.preprocOpaqueArgPending, live.preprocOpaqueArgActive)
	}
	liveBefore := captureCDiagnosticMutableState(live)

	forkSource, _, err := live.ForkTokenSourceForDiagnostic()
	if err != nil {
		t.Fatalf("ForkTokenSourceForDiagnostic failed: %v", err)
	}
	fork := forkSource.(*CTokenSource)
	t.Cleanup(fork.Close)
	assertCDiagnosticForkStateEqual(t, fork, live)
	if fork.preprocState != live.preprocState || fork.preprocOpaqueArgPending != live.preprocOpaqueArgPending || fork.preprocOpaqueArgActive != live.preprocOpaqueArgActive {
		t.Fatalf("fork preprocessor state = %d/%v/%v, want %d/%v/%v", fork.preprocState, fork.preprocOpaqueArgPending, fork.preprocOpaqueArgActive, live.preprocState, live.preprocOpaqueArgPending, live.preprocOpaqueArgActive)
	}

	forkTokens := advanceCDiagnosticTokens(fork, 6)
	assertCDiagnosticStateEqual(t, captureCDiagnosticMutableState(live), liveBefore)
	liveTokens := advanceCDiagnosticTokens(live, 6)
	requireCDiagnosticTokenSequenceEqual(t, forkTokens, liveTokens)
}

func TestCTokenSourceDiagnosticForkPreservesDefinitionBoundary(t *testing.T) {
	lang := CLanguage()
	sourceBytes := []byte("#define FLAG(x) x\nint tail;\n")
	live, err := NewCTokenSource(sourceBytes, lang)
	if err != nil {
		t.Fatalf("NewCTokenSource failed: %v", err)
	}
	t.Cleanup(live.Close)

	setup := advanceCDiagnosticTokens(live, 2)
	if got, want := setup[1].Text, "FLAG"; got != want {
		t.Fatalf("second setup token text = %q, want %q", got, want)
	}
	if live.preprocState != cPreprocAfterDefineName || live.preprocDefineNameEnd != live.cur.offset {
		t.Fatalf("definition boundary state = %d/%d, want %d/%d", live.preprocState, live.preprocDefineNameEnd, cPreprocAfterDefineName, live.cur.offset)
	}
	liveBefore := captureCDiagnosticMutableState(live)

	forkSource, _, err := live.ForkTokenSourceForDiagnostic()
	if err != nil {
		t.Fatalf("ForkTokenSourceForDiagnostic failed: %v", err)
	}
	fork := forkSource.(*CTokenSource)
	t.Cleanup(fork.Close)
	assertCDiagnosticForkStateEqual(t, fork, live)
	if fork.preprocDefineNameEnd != live.preprocDefineNameEnd {
		t.Fatalf("fork definition boundary = %d, want %d", fork.preprocDefineNameEnd, live.preprocDefineNameEnd)
	}

	forkTokens := advanceCDiagnosticTokens(fork, 6)
	assertCDiagnosticStateEqual(t, captureCDiagnosticMutableState(live), liveBefore)
	liveTokens := advanceCDiagnosticTokens(live, 6)
	requireCDiagnosticTokenSequenceEqual(t, forkTokens, liveTokens)
}

func TestCTokenSourceDiagnosticForkPreservesZeroWidthState(t *testing.T) {
	lang := CLanguage()
	sourceBytes := []byte("#ifdef __cplusplus\nextern \"C\" {\n#endif\n\nint x;\n\n#ifdef __cplusplus\n}\n#endif\n")
	live, err := NewCTokenSource(sourceBytes, lang)
	if err != nil {
		t.Fatalf("NewCTokenSource failed: %v", err)
	}
	t.Cleanup(live.Close)
	live.cur.advanceBytes(66)
	live.SetParserState(10)
	liveBefore := captureCDiagnosticMutableState(live)

	forkSource, receipt, err := live.ForkTokenSourceForDiagnostic()
	if err != nil {
		t.Fatalf("ForkTokenSourceForDiagnostic failed: %v", err)
	}
	fork := forkSource.(*CTokenSource)
	t.Cleanup(fork.Close)
	assertCDiagnosticForkStateEqual(t, fork, live)
	if got, want := receipt.ZeroWidthOffset, int64(-1); got != want {
		t.Fatalf("receipt zero-width offset = %d, want %d", got, want)
	}

	forkSynthetic := fork.Next()
	if forkSynthetic.Symbol != live.endifSymbol || forkSynthetic.StartByte != 66 || forkSynthetic.EndByte != 66 || !forkSynthetic.Missing {
		t.Fatalf("fork synthetic token = %+v, want a missing #endif at byte 66", forkSynthetic)
	}
	assertCDiagnosticStateEqual(t, captureCDiagnosticMutableState(live), liveBefore)
	liveSynthetic := live.Next()
	if !reflect.DeepEqual(liveSynthetic, forkSynthetic) {
		t.Fatalf("live synthetic token = %+v, want %+v", liveSynthetic, forkSynthetic)
	}

	guardedForkSource, guardedReceipt, err := live.ForkTokenSourceForDiagnostic()
	if err != nil {
		t.Fatalf("guarded ForkTokenSourceForDiagnostic failed: %v", err)
	}
	guardedFork := guardedForkSource.(*CTokenSource)
	t.Cleanup(guardedFork.Close)
	assertCDiagnosticForkStateEqual(t, guardedFork, live)
	if got, want := guardedReceipt.ZeroWidthOffset, int64(66); got != want {
		t.Fatalf("guarded receipt zero-width offset = %d, want %d", got, want)
	}
	liveBeforeGuardedAdvance := captureCDiagnosticMutableState(live)
	forkNext := guardedFork.Next()
	assertCDiagnosticStateEqual(t, captureCDiagnosticMutableState(live), liveBeforeGuardedAdvance)
	if liveNext := live.Next(); !reflect.DeepEqual(liveNext, forkNext) {
		t.Fatalf("guarded live token = %+v, want fork token %+v", liveNext, forkNext)
	}
}

func TestCTokenSourceDiagnosticForkPreservesDoneState(t *testing.T) {
	lang := CLanguage()
	live, err := NewCTokenSource([]byte("int"), lang)
	if err != nil {
		t.Fatalf("NewCTokenSource failed: %v", err)
	}
	t.Cleanup(live.Close)
	_ = live.Next()
	_ = live.Next()
	if !live.done {
		t.Fatal("setup source is not done")
	}
	liveBefore := captureCDiagnosticMutableState(live)

	forkSource, receipt, err := live.ForkTokenSourceForDiagnostic()
	if err != nil {
		t.Fatalf("ForkTokenSourceForDiagnostic failed: %v", err)
	}
	fork := forkSource.(*CTokenSource)
	t.Cleanup(fork.Close)
	assertCDiagnosticForkStateEqual(t, fork, live)
	if !fork.done {
		t.Fatal("fork did not preserve the done state")
	}
	if receipt.CursorByte != uint32(len(live.src)) || receipt.CursorPoint != (gotreesitter.Point{Column: 3}) {
		t.Fatalf("done receipt cursor = %d/%+v", receipt.CursorByte, receipt.CursorPoint)
	}

	forkToken := fork.Next()
	assertCDiagnosticStateEqual(t, captureCDiagnosticMutableState(live), liveBefore)
	if liveToken := live.Next(); !reflect.DeepEqual(liveToken, forkToken) {
		t.Fatalf("done live token = %+v, want fork token %+v", liveToken, forkToken)
	}
}
