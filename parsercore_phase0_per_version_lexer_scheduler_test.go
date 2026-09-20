//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestDiagnosticParserCoreVersionLexerRequestsOwnRaggedSpans(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entry := grammars.DetectLanguageByName("swift")
	if entry == nil || entry.Language() == nil {
		t.Fatal("swift grammar is unavailable")
	}
	lang := entry.Language()
	// Resolve by name, not by number: a grammar bump renumbers concrete
	// symbol IDs whenever it adds internal grammar symbols ahead of these in
	// the table, even for tokens this test does not otherwise touch.
	ltSym, ok := lang.SymbolByName("<")
	if !ok {
		t.Fatal("swift grammar has no \"<\" symbol")
	}
	hashSym, ok := lang.SymbolByName("#")
	if !ok {
		t.Fatal("swift grammar has no \"#\" symbol")
	}
	requests, err := gts.DiagnosticParserCoreVersionLexerRequestWitnessForTest(
		lang, []byte("a<A<#"),
	)
	if err != nil {
		t.Fatalf("version lexer request witness: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("version lexer requests=%d, want 2", len(requests))
	}
	// The two state IDs (49, 528; formerly 47, 524) are pinned by number:
	// state IDs have no name to resolve by. Re-derived by probing this same
	// witness after the tree-sitter-swift 00bbb0a2550f bump renumbered the
	// state graph.
	want := map[gts.StateID]struct {
		symbol   gts.Symbol
		endByte  uint32
		external bool
		internal bool
	}{
		49:  {symbol: ltSym, endByte: 4, internal: true},
		528: {symbol: hashSym, endByte: 5, external: true},
	}
	for _, request := range requests {
		expect, ok := want[request.State]
		if !ok {
			t.Fatalf("unexpected request state=%d", request.State)
		}
		if request.Token.Symbol != expect.symbol || request.Token.StartByte != 3 || request.Token.EndByte != expect.endByte {
			t.Fatalf("state %d token=%+v, want symbol=%d span=3..%d", request.State, request.Token, expect.symbol, expect.endByte)
		}
		if request.Token.ExternalScannerToken != expect.external {
			t.Fatalf("state %d external=%t, want %t", request.State, request.Token.ExternalScannerToken, expect.external)
		}
		if request.InternalDFAToken != expect.internal {
			t.Fatalf("state %d internal DFA=%t, want %t", request.State, request.InternalDFAToken, expect.internal)
		}
		if request.ScannerBefore.Length != 9 || request.ScannerAfter.Length != 9 {
			t.Fatalf("state %d lost scanner checkpoint pair: before=%+v after=%+v", request.State, request.ScannerBefore, request.ScannerAfter)
		}
	}
}
