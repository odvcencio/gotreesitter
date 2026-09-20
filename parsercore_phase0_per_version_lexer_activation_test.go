//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter_test

import (
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

const versionLexerProductionSource = "a<(A<#/x/#)"

// TestDiagnosticParserCoreVersionLexerProductionActivation proves that the
// fresh-full scheduler owns each lexer cursor after a width disagreement. The
// shared pass consumes '<#' at byte 4. Both retained versions consume '<' from
// their own cursors. One lineage then shifts '#' and reaches EOF acceptance.
func TestDiagnosticParserCoreVersionLexerProductionActivation(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entry := grammars.DetectLanguageByName("swift")
	if entry == nil || entry.Language() == nil {
		t.Fatal("swift grammar is unavailable")
	}
	lang := entry.Language()
	// Resolve by name, not by number: a grammar bump renumbers concrete
	// symbol IDs whenever it adds internal grammar symbols ahead of these in
	// the table, even for tokens this test does not otherwise touch.
	hashSym, ok := lang.SymbolByName("#")
	if !ok {
		t.Fatal("swift grammar has no \"#\" symbol")
	}
	ltSym, ok := lang.SymbolByName("<")
	if !ok {
		t.Fatal("swift grammar has no \"<\" symbol")
	}
	receipt, err := gts.DiagnosticParserCoreVersionLexerProductionActivationForTest(
		lang, []byte(versionLexerProductionSource),
	)
	if err != nil {
		t.Fatalf("production scheduler: %v", err)
	}
	if receipt.Acceptance == nil {
		t.Fatalf("owned lexer route did not accept: stop=%+v", receipt.Stop)
	}
	if receipt.Stop.Detail != "" {
		t.Fatalf("accepted scheduler retained stop detail %q", receipt.Stop.Detail)
	}
	work := receipt.Acceptance.Work
	if receipt.PerVersionLexRequests != 4 || receipt.PerVersionLexRestores != 4 ||
		receipt.PerVersionLexPublications != 14 || receipt.PerVersionLexAcceptedRaggedSpans != 3 ||
		receipt.PerVersionLexViabilityDrops != 1 {
		t.Fatalf("published lexer counters=%d/%d/%d/%d/%d, want 4/4/14/3/1",
			receipt.PerVersionLexRequests, receipt.PerVersionLexRestores,
			receipt.PerVersionLexPublications, receipt.PerVersionLexAcceptedRaggedSpans,
			receipt.PerVersionLexViabilityDrops)
	}
	if work.PerVersionLexRequests != 4 || work.PerVersionLexRestores != 4 ||
		work.PerVersionLexPublications != 14 || work.PerVersionLexAcceptedRaggedSpans != 3 ||
		work.PerVersionLexViabilityDrops != 1 {
		t.Fatalf("acceptance lexer counters=%d/%d/%d/%d/%d, want 4/4/14/3/1",
			work.PerVersionLexRequests, work.PerVersionLexRestores,
			work.PerVersionLexPublications, work.PerVersionLexAcceptedRaggedSpans,
			work.PerVersionLexViabilityDrops)
	}
	if receipt.PeakLiveVersions != 2 || work.PeakLiveVersions != 2 {
		t.Fatalf("peak live versions=%d/%d, want 2/2", receipt.PeakLiveVersions, work.PeakLiveVersions)
	}
	if work.NoActionDrops != 1 || work.ConvergedReductionSplitDrops != 0 || work.ConvergedCoverageDrops != 0 {
		t.Fatalf("drop classes=%d/%d/%d, want viability-only 1/0/0",
			work.NoActionDrops, work.ConvergedReductionSplitDrops, work.ConvergedCoverageDrops)
	}

	if len(receipt.Elections) < 5 {
		t.Fatalf("elections=%d, want activation election", len(receipt.Elections))
	}
	activation := receipt.Elections[4]
	if len(activation.States) != 1 || activation.States[0] != 10 {
		t.Fatalf("activation states=%v, want [10]", activation.States)
	}
	if activation.Token.Symbol != hashSym || activation.Token.StartByte != 4 ||
		activation.Token.EndByte != 6 || !activation.Token.ExternalScannerToken {
		t.Fatalf("activation token=%+v, want external %d (\"#\") over bytes 4..6", activation.Token, hashSym)
	}
	assertNineByteCheckpoint := func(label string, checkpoint gts.DiagnosticParserCoreScannerCheckpoint) {
		t.Helper()
		if checkpoint.Length != 9 || checkpoint.SHA256 == [32]byte{} {
			t.Fatalf("%s checkpoint=%+v, want authenticated Swift state", label, checkpoint)
		}
	}
	assertNineByteCheckpoint("activation before", activation.ScannerBefore)
	assertNineByteCheckpoint("activation after", activation.ScannerAfter)

	// State IDs are pinned by number: they come from this fixture's GLR
	// table, not from grammar-symbol enumeration order, and there is no
	// name to resolve them by. Re-derived from receipt.VersionLexerRequests
	// against the current swift tables after the tree-sitter-swift
	// 00bbb0a2550f bump added internal grammar symbols and renumbered the
	// state graph; a state-ID probe against this same fixture is the only
	// way to recover them (see repro: run this test with -v and diff the
	// "unexpected owned request state" failure's states against this map).
	want := map[gts.StateID]struct {
		election      int
		symbol        gts.Symbol
		start, end    uint32
		external, dfa bool
	}{
		437:  {election: 4, symbol: ltSym, start: 4, end: 5, dfa: true},
		1525: {election: 4, symbol: ltSym, start: 4, end: 5, dfa: true},
		135:  {election: 5, symbol: hashSym, start: 5, end: 6, external: true},
		397:  {election: 5, symbol: gts.Symbol(65535), start: 5, end: 6},
	}
	if len(receipt.VersionLexerRequests) != len(want) {
		t.Fatalf("owned lexer requests=%d, want %d", len(receipt.VersionLexerRequests), len(want))
	}
	for _, request := range receipt.VersionLexerRequests {
		expect, ok := want[request.State]
		if !ok {
			t.Fatalf("unexpected owned request state=%d: requests=%+v", request.State, receipt.VersionLexerRequests)
		}
		if request.ElectionIndex != expect.election || request.Token.Symbol != expect.symbol ||
			request.Token.StartByte != expect.start || request.Token.EndByte != expect.end ||
			request.Token.ExternalScannerToken != expect.external || request.InternalDFAToken != expect.dfa {
			t.Fatalf("state %d request=%+v, want election=%d symbol=%d span=%d..%d external=%t dfa=%t",
				request.State, request, expect.election, expect.symbol, expect.start, expect.end,
				expect.external, expect.dfa)
		}
		assertNineByteCheckpoint(fmt.Sprintf("state %d before", request.State), request.ScannerBefore)
		assertNineByteCheckpoint(fmt.Sprintf("state %d after", request.State), request.ScannerAfter)
	}
	if len(receipt.NoActionDrops) != 1 {
		t.Fatalf("owned viability drops=%d, want 1", len(receipt.NoActionDrops))
	}
	if got := receipt.NoActionDrops[0].Token; got.Symbol != gts.Symbol(65535) || got.StartByte != 5 || got.EndByte != 6 {
		t.Fatalf("viability drop token=%+v, want error symbol at 5..6", got)
	}

	acceptance := receipt.Acceptance
	wantEOF := uint32(len(versionLexerProductionSource))
	if acceptance.Token.Symbol != 0 || acceptance.Token.StartByte != wantEOF || acceptance.Token.EndByte != wantEOF {
		t.Fatalf("accepted token=%+v, want EOF at byte %d", acceptance.Token, wantEOF)
	}
	if !acceptance.Header.Header.Accepted || acceptance.Header.Header.ByteOffset != wantEOF ||
		acceptance.Header.Header.Shifted || acceptance.Header.Header.Paused {
		t.Fatalf("accepted header=%+v, want sole closed EOF header", acceptance.Header.Header)
	}
}
