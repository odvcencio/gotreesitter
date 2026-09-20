//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// phase0PerVersionLexerSwiftWidthSource is the smallest checked-in Swift lexer
// witness for two parser versions that disagree about token width.
//
// At byte 3, parser state 528 consumes _hash_symbol_custom (external "#") over
// "<#". Parser state 49 consumes the one-byte '<' operator. C keeps both
// lexer views on their owning versions before it chooses a tree. The two
// symbols are resolved by name in the test body (lang.SymbolByName), not
// pinned by number: a grammar bump renumbers them whenever it adds internal
// grammar symbols ahead of these in the table. The two state IDs (49, 528;
// formerly 47, 524) are pinned by number since state IDs have no name to
// resolve by; re-derived from widthReceipt.VersionLexerRequests against the
// current swift tables after the tree-sitter-swift 00bbb0a2550f bump
// renumbered the state graph (run this test with -v to re-probe them).
const phase0PerVersionLexerSwiftWidthSource = "a<A<#/x/#"

// phase0PerVersionLexerSwiftOracleSource adds one tuple wrapper. The wrapper
// removes an unrelated comparison-associativity difference from the generated
// Go table. It keeps the production width disagreement and owned dispatch.
const phase0PerVersionLexerSwiftOracleSource = "a<(A<#/x/#)"

// TestPhase0PerVersionLexerVersionsOwnWidths is the stage-1
// falsifier. The direct probe pins both exact token widths. The scheduler
// assertion then requires the two parser versions to continue independently
// instead of declining because one shared cursor cannot represent both ends.
//
// The locked C comparison remains in the cgo_harness package. The root
// internal test package cannot call it.
// Incremental edits belong to the stage-5 integration gate and stay out of
// this stage-1 lexer-ownership falsifier.
func TestPhase0PerVersionLexerVersionsOwnWidths(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entry := grammars.DetectLanguageByName("swift")
	if entry == nil {
		t.Fatal("swift is absent from the language registry")
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
	widthSource := []byte(phase0PerVersionLexerSwiftWidthSource)

	shared := gts.Token{
		Symbol:                   hashSym,
		StartByte:                3,
		EndByte:                  5,
		StartPoint:               gts.Point{Column: 3},
		EndPoint:                 gts.Point{Column: 5},
		ExternalScannerToken:     true,
		ExternalScannerStartByte: 3,
	}
	const narrowState = gts.StateID(49)
	narrow, ok := gts.RelexTokenForStateForTest(
		lang,
		widthSource,
		1,
		gts.DiagnosticParserCorePrefixOptions{},
		narrowState,
		shared,
	)
	if !ok {
		t.Fatal("narrow parser version did not produce its own DFA token")
	}
	if narrow.Symbol != ltSym || narrow.StartByte != 3 || narrow.EndByte != 4 {
		t.Fatalf("narrow parser-version token=%+v, want symbol %d (\"<\") over bytes 3..4", narrow, ltSym)
	}
	if narrow.StartPoint != (gts.Point{Column: 3}) || narrow.EndPoint != (gts.Point{Column: 4}) || narrow.ExternalScannerToken {
		t.Fatalf("narrow parser-version token=%+v, want DFA token points 3..4", narrow)
	}
	widthReceipt, err := gts.RunStateDependentRelexSchedulerForTest(lang, widthSource)
	if err != nil {
		t.Fatalf("width scheduler: %v", err)
	}
	var narrowRequest, wideRequest *gts.DiagnosticParserCoreVersionLexerRequest
	for index := range widthReceipt.VersionLexerRequests {
		request := &widthReceipt.VersionLexerRequests[index]
		switch request.State {
		case 49:
			narrowRequest = request
		case 528:
			wideRequest = request
		}
	}
	if narrowRequest == nil || wideRequest == nil {
		t.Fatalf("width requests=%+v, want states 49 and 528", widthReceipt.VersionLexerRequests)
	}
	if narrowRequest.Token.Symbol != ltSym || narrowRequest.Token.StartByte != 3 || narrowRequest.Token.EndByte != 4 || !narrowRequest.InternalDFAToken {
		t.Fatalf("narrow request=%+v, want DFA symbol %d (\"<\") over bytes 3..4", *narrowRequest, ltSym)
	}
	if wideRequest.Token.Symbol != hashSym || wideRequest.Token.StartByte != 3 || wideRequest.Token.EndByte != 5 || !wideRequest.Token.ExternalScannerToken {
		t.Fatalf("wide request=%+v, want external symbol %d (\"#\") over bytes 3..5", *wideRequest, hashSym)
	}
	if wideRequest.Token.EndByte-wideRequest.Token.StartByte != 2 || narrowRequest.Token.EndByte-narrowRequest.Token.StartByte != 1 {
		t.Fatalf("version token widths=wide:%d narrow:%d, want 2:1", wideRequest.Token.EndByte-wideRequest.Token.StartByte, narrowRequest.Token.EndByte-narrowRequest.Token.StartByte)
	}

	source := []byte(phase0PerVersionLexerSwiftOracleSource)
	receipt, err := gts.RunStateDependentRelexSchedulerForTest(lang, source)
	if err != nil {
		t.Fatalf("compact scheduler: %v", err)
	}
	if receipt.Tokens == 0 || len(receipt.Elections) == 0 {
		t.Fatalf("scheduler telemetry did not prove an executed path: tokens=%d elections=%d", receipt.Tokens, len(receipt.Elections))
	}
	if strings.HasPrefix(receipt.Stop.Detail, gts.DiagnosticParserCoreOwnedDispatchPendingDetailForTest()) {
		t.Fatalf("owned lexer requests activated but did not dispatch: stop=%+v", receipt.Stop)
	}
	assertPhase0PerVersionLexerReceipt(t, lang, receipt, source)
	peakHeaders := receipt.Stop.Work.PeakHeaders
	if receipt.Acceptance != nil && receipt.Acceptance.Work.PeakHeaders > peakHeaders {
		peakHeaders = receipt.Acceptance.Work.PeakHeaders
	}
	if peakHeaders < 2 {
		t.Fatalf("scheduler telemetry did not prove two live parser versions: peak_headers=%d, want at least 2", peakHeaders)
	}
	hasMultiVersionElection := false
	for _, election := range receipt.Elections {
		if len(election.States) >= 2 {
			hasMultiVersionElection = true
			break
		}
	}
	if !hasMultiVersionElection {
		t.Fatalf("scheduler telemetry did not prove a multi-version election: elections=%+v", receipt.Elections)
	}
	wantRaggedDetail := gts.DiagnosticParserCoreRaggedRelexDeclineDetailFormatForTest(
		gts.Token{Symbol: ltSym, StartByte: 4, EndByte: 5},
		gts.Token{Symbol: hashSym, StartByte: 4, EndByte: 6},
	)
	if receipt.Stop.Detail == wantRaggedDetail || strings.HasPrefix(receipt.Stop.Detail, gts.DiagnosticParserCoreRaggedRelexDeclineDetailForTest()) {
		t.Fatalf("shared-cursor decline prevented C-equivalent parser-version selection: stop boundary=%q detail=%q; C views are symbol %d (\"#\") bytes 4..6 and symbol %d (\"<\") bytes 4..5", receipt.Stop.Boundary, receipt.Stop.Detail, hashSym, ltSym)
	}
	if receipt.Acceptance == nil {
		t.Fatalf("per-version lexer route did not select an accepted C-equivalent tree: stop=%+v", receipt.Stop)
	}
	if receipt.Acceptance.Work.PerVersionLexViabilityDrops != 1 ||
		receipt.Acceptance.Work.PerVersionLexAcceptedRaggedSpans != 3 {
		t.Fatalf("owned path telemetry=%d/%d, want 1 viability drop and 3 ragged shifts",
			receipt.Acceptance.Work.PerVersionLexViabilityDrops,
			receipt.Acceptance.Work.PerVersionLexAcceptedRaggedSpans)
	}
}

// TestPhase0PerVersionLexerScalaOracleWitness pins the smallest clean witness
// used by the locked C comparison. Both owned requests start at byte 3. One
// consumes "->" and the other consumes "-". The compact route must accept.
func TestPhase0PerVersionLexerScalaOracleWitness(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entry := grammars.DetectLanguageByName("scala")
	if entry == nil || entry.Language() == nil {
		t.Fatal("scala grammar is unavailable")
	}
	const source = "(y)->"
	receipt, err := gts.RunStateDependentRelexSchedulerForTest(entry.Language(), []byte(source))
	if err != nil {
		t.Fatalf("compact scheduler: %v", err)
	}
	if receipt.Acceptance == nil || receipt.Stop.Detail != "" {
		t.Fatalf("owned lexer route did not accept cleanly: stop=%+v", receipt.Stop)
	}
	if receipt.PeakLiveVersions != 2 || receipt.PerVersionLexRequests != 2 || receipt.PerVersionLexViabilityDrops != 1 {
		t.Fatalf("owned path telemetry peak/requests/drops=%d/%d/%d, want 2/2/1",
			receipt.PeakLiveVersions, receipt.PerVersionLexRequests, receipt.PerVersionLexViabilityDrops)
	}
	want := map[gts.StateID]struct {
		symbol     gts.Symbol
		start, end uint32
	}{
		8195:  {symbol: 79, start: 3, end: 5},
		18766: {symbol: 23, start: 3, end: 4},
	}
	if len(receipt.VersionLexerRequests) != len(want) {
		t.Fatalf("owned requests=%+v, want two width competitors", receipt.VersionLexerRequests)
	}
	for _, request := range receipt.VersionLexerRequests {
		expect, ok := want[request.State]
		if !ok {
			t.Fatalf("unexpected owned request=%+v; all=%+v", request, receipt.VersionLexerRequests)
		}
		if request.ElectionIndex != 3 || request.Token.Symbol != expect.symbol ||
			request.Token.StartByte != expect.start || request.Token.EndByte != expect.end {
			t.Fatalf("state %d request=%+v, want election 3 symbol %d span %d..%d",
				request.State, request, expect.symbol, expect.start, expect.end)
		}
	}
	if got := receipt.Acceptance.Header.Header.ByteOffset; got != uint32(len(source)) {
		t.Fatalf("accepted byte offset=%d, want %d", got, len(source))
	}
}

// assertPhase0PerVersionLexerReceipt checks the exact shared and owned lexer
// evidence that the generic scheduler publishes for this witness.
func assertPhase0PerVersionLexerReceipt(t *testing.T, lang *gts.Language, receipt gts.DiagnosticParserCoreGenericScheduler, source []byte) {
	t.Helper()
	hashSym, ok := lang.SymbolByName("#")
	if !ok {
		t.Fatal("swift grammar has no \"#\" symbol")
	}
	ltSym, ok := lang.SymbolByName("<")
	if !ok {
		t.Fatal("swift grammar has no \"<\" symbol")
	}
	if receipt.StartCheckpoint != receipt.Elections[0].ScannerBefore {
		t.Fatalf("start checkpoint=%+v, want first election's scanner-before=%+v", receipt.StartCheckpoint, receipt.Elections[0].ScannerBefore)
	}

	widthTokens := make([]gts.Token, 0, 7)
	cursor := uint32(0)
	var widthElection gts.DiagnosticParserCoreElection
	foundWidthElection := false
	for index, election := range receipt.Elections {
		token := election.Token
		if token.StartByte > token.EndByte || token.EndByte > uint32(len(source)) {
			t.Fatalf("election %d token span=%d..%d escapes source length %d", index, token.StartByte, token.EndByte, len(source))
		}
		if election.ScannerBefore.Length != 9 || election.ScannerAfter.Length != 9 {
			t.Fatalf("election %d scanner checkpoint lengths=%d/%d, want Swift's exact nine-byte contract", index, election.ScannerBefore.Length, election.ScannerAfter.Length)
		}
		if token.Symbol == 0 {
			if election.CurrentCheckpointValid {
				t.Fatalf("EOF election %d retained external-token checkpoint geometry: %+v", index, election)
			}
		} else {
			if !election.CurrentCheckpointValid || election.CurrentCheckpointStart != election.ScannerBefore || election.CurrentCheckpointEnd != election.ScannerAfter {
				t.Fatalf("election %d lost its scanner checkpoint sidecar: %+v", index, election)
			}
			if election.CurrentCheckpointBytes != [2]uint32{token.StartByte, token.EndByte} {
				t.Fatalf("election %d checkpoint bytes=%v, want token span [%d %d]", index, election.CurrentCheckpointBytes, token.StartByte, token.EndByte)
			}
		}
		if index > 0 && election.ScannerBefore != receipt.Elections[index-1].ScannerAfter {
			t.Fatalf("election %d scanner-before=%+v does not follow election %d scanner-after=%+v", index, election.ScannerBefore, index-1, receipt.Elections[index-1].ScannerAfter)
		}
		if token.EndByte == token.StartByte {
			if token.Symbol != 0 || token.StartByte != cursor {
				t.Fatalf("zero-width non-EOF or cursor-discontinuous token at election %d: %+v cursor=%d", index, token, cursor)
			}
			continue
		}
		if token.StartByte != cursor {
			t.Fatalf("lexer cursor gap or overlap at election %d: token=%d..%d cursor=%d", index, token.StartByte, token.EndByte, cursor)
		}
		cursor = token.EndByte
		widthTokens = append(widthTokens, token)
		if token.Symbol == hashSym && token.StartByte == 4 && token.EndByte == 6 && token.ExternalScannerToken {
			widthElection = election
			foundWidthElection = true
		}
	}
	if cursor != uint32(len(source)) {
		t.Fatalf("lexer cursor ended at %d, want source length %d", cursor, len(source))
	}
	wantTokens := []struct {
		symbol     gts.Symbol
		start, end uint32
		external   bool
	}{
		{symbol: 163, start: 0, end: 1},
		{symbol: ltSym, start: 1, end: 2},
		{symbol: 20, start: 2, end: 3},
		{symbol: 163, start: 3, end: 4},
		{symbol: hashSym, start: 4, end: 6, external: true},
		{symbol: 167, start: 6, end: 10},
		{symbol: 16, start: 10, end: 11},
	}
	if len(widthTokens) != len(wantTokens) {
		t.Fatalf("scheduler emitted %d non-zero-width tokens=%+v, want the exact seven-token witness", len(widthTokens), widthTokens)
	}
	for index, want := range wantTokens {
		got := widthTokens[index]
		if got.Symbol != want.symbol || got.StartByte != want.start || got.EndByte != want.end || got.ExternalScannerToken != want.external {
			t.Fatalf("scheduler token %d=%+v, want symbol=%d span=%d..%d external=%t", index, got, want.symbol, want.start, want.end, want.external)
		}
	}
	if !foundWidthElection {
		t.Fatal("scheduler telemetry did not retain the two-byte external token election")
	}

	// Keep two distinct headers alive at the shared-token boundary. A receipt
	// that merges them before this point cannot represent the narrower re-lex.
	hasTwoVersionsAtWidthStart := false
	for _, round := range receipt.Rounds {
		if len(round.Before) < 2 {
			continue
		}
		distinctStates := false
		firstState := round.Before[0].State
		allAtWidthStart := true
		for _, header := range round.Before {
			if header.ByteOffset != 4 || header.Checkpoint != widthElection.ScannerBefore.SHA256 {
				allAtWidthStart = false
				break
			}
			if header.State != firstState {
				distinctStates = true
			}
		}
		if allAtWidthStart && distinctStates {
			hasTwoVersionsAtWidthStart = true
			break
		}
	}
	if !hasTwoVersionsAtWidthStart {
		t.Fatalf("scheduler telemetry did not retain two distinct parser versions at byte 4 before the wide token: rounds=%+v", receipt.Rounds)
	}

	if receipt.Acceptance == nil {
		return
	}
	acceptance := receipt.Acceptance
	if acceptance.Token.Symbol != 0 || acceptance.Token.StartByte != uint32(len(source)) || acceptance.Token.EndByte != uint32(len(source)) {
		t.Fatalf("accepted token=%+v, want zero-width EOF at byte %d", acceptance.Token, len(source))
	}
	if acceptance.Header.Header.ByteOffset != uint32(len(source)) || !acceptance.Header.Header.Accepted || acceptance.Header.Header.Shifted || acceptance.Header.Header.Paused {
		t.Fatalf("accepted header=%+v, want one unshifted accepted EOF header at byte %d", acceptance.Header.Header, len(source))
	}
	foundEOF := false
	for _, election := range receipt.Elections {
		if election.Token.Symbol == 0 && election.Token.StartByte == uint32(len(source)) && election.Token.EndByte == uint32(len(source)) {
			if acceptance.Header.Header.Checkpoint != election.ScannerAfter.SHA256 {
				t.Fatalf("accepted header checkpoint=%x, want EOF scanner-after=%x", acceptance.Header.Header.Checkpoint, election.ScannerAfter.SHA256)
			}
			foundEOF = true
		}
	}
	if !foundEOF {
		t.Fatal("scheduler reported acceptance without an authenticated EOF election")
	}

	foundExternalShift := false
	for _, shift := range receipt.ExternalShifts {
		if shift.Token.StartByte == 4 && shift.Token.EndByte == 6 {
			t.Fatalf("wide shared token shifted, want its owned version dropped: %+v", shift)
		}
		if shift.Token.StartByte != 5 || shift.Token.EndByte != 6 {
			continue
		}
		if shift.Token.Symbol != hashSym || !shift.Token.ExternalScannerToken ||
			shift.ScannerBefore.Length != 9 || shift.ScannerAfter.Length != 9 {
			t.Fatalf("selected external shift lost its owning scanner contract: %+v", shift)
		}
		for _, payload := range shift.Payloads {
			if payload.Symbol == hashSym && payload.StartByte == 5 && payload.EndByte == 6 && payload.External && payload.Terminal {
				foundExternalShift = true
			}
		}
	}
	if !foundExternalShift {
		t.Fatal("accepted scheduler receipt has no scanner-owned terminal for the selected '#' token")
	}
}
