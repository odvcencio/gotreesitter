//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"fmt"
	"reflect"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

type fakeGoScanner struct{}

func (fakeGoScanner) Create() any                                        { return nil }
func (fakeGoScanner) Destroy(any)                                        {}
func (fakeGoScanner) Serialize(any, []byte) int                          { return 0 }
func (fakeGoScanner) Deserialize(any, []byte)                            {}
func (fakeGoScanner) Scan(any, *gotreesitter.ExternalLexer, []bool) bool { return false }

type wrappedGoScanner struct{ grammars.GoExternalScanner }

func TestDiagnosticParserCoreRewritePrefixUsesExactProductionElection(t *testing.T) {
	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		t.Fatal(err)
	}
	var rewrite benchfixtures.LoadedFixture
	for _, fixture := range fixtures {
		if fixture.Fixture.ID == "rewrite" {
			rewrite = fixture
			break
		}
	}
	if len(rewrite.Source) == 0 {
		t.Fatal("frozen rewrite fixture not found")
	}
	scanner := grammars.GoExternalScanner{}
	result, prefixErr := gotreesitter.DiagnosticParseParserCorePrefix(scanner, rewrite.Source, gotreesitter.DiagnosticParserCorePrefixOptions{})
	if result.Grammar != "go" || !result.ExactRootDFA || result.Materialized {
		t.Fatalf("route identity = grammar %q exact=%t materialized=%t err=%v", result.Grammar, result.ExactRootDFA, result.Materialized, prefixErr)
	}
	if result.SourceSHA256 == [32]byte{} || result.GrammarBlobSHA256 == [32]byte{} || result.Tokens == 0 || len(result.Elections) == 0 {
		t.Fatalf("prefix did not reach an authenticated election: %+v", result)
	}
	if got := fmt.Sprintf("%x", result.GrammarBlobSHA256); got != "9cf914d26d962d1a62e7954f8b20b302337a44cb7d4a07218eec482c45a57a08" {
		t.Fatalf("embedded grammar receipt = %s", got)
	}
	if prefixErr != nil && result.Boundary == "" {
		t.Fatalf("untyped prefix error: %v", prefixErr)
	}
	if result.Boundary != gotreesitter.DiagnosticParserCoreFirstFork || result.Tokens != 68 || result.Dispatches != 117 || result.State != 186 || result.Lookahead.Symbol != 20 || prefixErr != nil {
		t.Fatalf("rewrite prefix identity drifted: boundary=%s tokens=%d dispatches=%d state=%d lookahead=%d fork_actions=%+v fork_boundaries=%d fork_paths=%d extras=%+v reductions=%+v", result.Boundary, result.Tokens, result.Dispatches, result.State, result.Lookahead.Symbol, result.ForkActions, result.ForkBoundaries, result.ForkLogicalPaths, result.ExtraShifts, result.ReductionAttempts)
	}
	wantForkActions := []gotreesitter.DiagnosticParserCoreForkAction{
		{Ordinal: 1, Action: gotreesitter.ParseAction{Type: gotreesitter.ParseActionShift, State: 193}, BranchOrder: 1},
		{Ordinal: 0, Action: gotreesitter.ParseAction{Type: gotreesitter.ParseActionReduce, Symbol: 171, ChildCount: 1}},
	}
	wantForkBoundaries := []gotreesitter.DiagnosticParserCoreForkBoundary{
		{State: 193, ByteOffset: 580, ExactPaths: 1},
		{State: 285, ByteOffset: 579, ExactPaths: 1},
	}
	if !reflect.DeepEqual(result.ForkActions, wantForkActions) || !reflect.DeepEqual(result.ForkBoundaryReceipts, wantForkBoundaries) || result.ForkBoundaries != 2 || result.ForkLogicalPaths != 2 {
		t.Fatalf("first fork receipt: actions=%+v boundary_receipts=%+v boundaries=%d paths=%d, want actions=%+v boundary_receipts=%+v boundaries=2 paths=2", result.ForkActions, result.ForkBoundaryReceipts, result.ForkBoundaries, result.ForkLogicalPaths, wantForkActions, wantForkBoundaries)
	}
	wantExtraSpans := [][2]uint32{{49, 116}, {117, 183}, {184, 266}, {267, 310}, {457, 517}}
	wantExtraStates := []gotreesitter.StateID{542, 542, 542, 542, 349}
	if len(result.ExtraShifts) != len(wantExtraSpans) {
		t.Fatalf("plain extra shifts=%d, want %d", len(result.ExtraShifts), len(wantExtraSpans))
	}
	for index, extra := range result.ExtraShifts {
		wantAction := gotreesitter.ParseAction{Type: gotreesitter.ParseActionShift, Extra: true}
		if extra.State != wantExtraStates[index] || extra.EffectiveState != wantExtraStates[index] || extra.Token.Symbol != 92 ||
			extra.Token.StartByte != wantExtraSpans[index][0] || extra.Token.EndByte != wantExtraSpans[index][1] ||
			extra.Token.NoLookahead || extra.Token.ExternalScannerToken || !reflect.DeepEqual(extra.Action, wantAction) {
			t.Fatalf("plain extra %d=%+v, want state-preserving DFA comment span=%v action=%+v", index, extra, wantExtraSpans[index], wantAction)
		}
		var election *gotreesitter.DiagnosticParserCoreElection
		for electionIndex := range result.Elections {
			candidate := &result.Elections[electionIndex]
			if candidate.Token.StartByte == extra.Token.StartByte && candidate.Token.EndByte == extra.Token.EndByte {
				election = candidate
				break
			}
		}
		if election == nil || !reflect.DeepEqual(election.Token, extra.Token) || election.ScannerBefore.Length != 0 || election.ScannerAfter.Length != 0 || election.CurrentCheckpointValid {
			t.Fatalf("plain extra election %d=%+v, extra=%+v", index, election, extra)
		}
	}
	wantTrailingReduction := gotreesitter.ParseAction{Type: gotreesitter.ParseActionReduce, Symbol: 197, ChildCount: 4}
	var foundTrailing bool
	for _, reduction := range result.ReductionAttempts {
		if reduction.State == 542 && reduction.Lookahead.Symbol == 16 {
			foundTrailing = reduction.Applied && reflect.DeepEqual(reduction.Action, wantTrailingReduction)
			break
		}
	}
	if !foundTrailing {
		t.Fatalf("state 542/lookahead 16 trailing-extra reduction missing: reductions=%+v", result.ReductionAttempts)
	}
	wantInterstitialReduction := gotreesitter.ParseAction{Type: gotreesitter.ParseActionReduce, Symbol: 197, ChildCount: 3}
	var foundInterstitial bool
	for _, attempt := range result.ReductionAttempts {
		if !attempt.Applied {
			t.Fatalf("pre-fork reduction was not applied: %+v", attempt)
		}
		if attempt.State == 349 && attempt.Lookahead.Symbol == 16 && reflect.DeepEqual(attempt.Action, wantInterstitialReduction) {
			foundInterstitial = true
		}
	}
	if !foundInterstitial {
		t.Fatalf("applied state 349/lookahead 16 interstitial-extra reduction missing")
	}

	// The comparison parse is evidence only. It does not feed tokens, actions,
	// or stack state into the compact scheduler.
	profile := gotreesitter.NewAmbiguityProfile()
	parser := gotreesitter.NewParser(grammars.GoLanguage())
	parser.SetAmbiguityProfile(profile)
	type tokenIdentity struct {
		symbol     gotreesitter.Symbol
		start, end uint32
	}
	var productionTokens []tokenIdentity
	parser.SetLogger(func(kind gotreesitter.ParserLogType, message string) {
		if kind != gotreesitter.ParserLogLex {
			return
		}
		var symbol uint16
		var start, end uint32
		if _, scanErr := fmt.Sscanf(message, "token sym=%d start=%d end=%d", &symbol, &start, &end); scanErr == nil {
			productionTokens = append(productionTokens, tokenIdentity{symbol: gotreesitter.Symbol(symbol), start: start, end: end})
		}
	})
	tree, err := parser.Parse(rewrite.Source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	if tree.ParseStoppedEarly() || tree.RootNode() == nil || tree.RootNode().EndByte() != uint32(len(rewrite.Source)) {
		t.Fatalf("production comparison did not accept frozen rewrite: runtime=%+v", tree.ParseRuntime())
	}
	if len(productionTokens) < len(result.Elections) {
		t.Fatalf("production token trace has %d tokens, prefix has %d", len(productionTokens), len(result.Elections))
	}
	for index, election := range result.Elections {
		want := productionTokens[index]
		if election.Token.Symbol != want.symbol || election.Token.StartByte != want.start || election.Token.EndByte != want.end {
			t.Fatalf("election %d token=(%d,%d,%d) production=(%d,%d,%d)", index, election.Token.Symbol, election.Token.StartByte, election.Token.EndByte, want.symbol, want.start, want.end)
		}
	}
	if result.Boundary == gotreesitter.DiagnosticParserCoreFirstFork {
		var matched bool
		for _, stat := range profile.SnapshotTop(-1) {
			if stat.State == result.State && stat.Lookahead == result.Lookahead.Symbol {
				want := make([]gotreesitter.ParseAction, 0, len(result.ForkActions))
				for ordinal := 0; ordinal < len(result.ForkActions); ordinal++ {
					for _, fork := range result.ForkActions {
						if fork.Ordinal == ordinal {
							want = append(want, fork.Action)
						}
					}
				}
				if reflect.DeepEqual(stat.Actions, want) {
					matched = true
					break
				}
			}
		}
		if !matched {
			t.Fatalf("independent first-fork boundary (%d,%d) absent from production ambiguity profile", result.State, result.Lookahead.Symbol)
		}
	} else if prefixErr == nil || result.Boundary == "" {
		t.Fatalf("prefix stopped without a typed boundary: result=%+v err=%v", result, prefixErr)
	}
	t.Logf("parser-core rewrite prefix boundary=%s detail=%q tokens=%d dispatches=%d state=%d lookahead=%d fork_boundaries=%+v", result.Boundary, result.Detail, result.Tokens, result.Dispatches, result.State, result.Lookahead.Symbol, result.ForkBoundaryReceipts)
}

func TestDiagnosticParserCoreUsesEmbeddedGrammarAndExactScanner(t *testing.T) {
	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		t.Fatal(err)
	}
	var source []byte
	for _, fixture := range fixtures {
		if fixture.Fixture.ID == "rewrite" {
			source = fixture.Source
			break
		}
	}
	if len(source) == 0 {
		t.Fatal("rewrite fixture missing")
	}
	canonical, canonicalErr := gotreesitter.DiagnosticParseParserCorePrefix(grammars.GoExternalScanner{}, source, gotreesitter.DiagnosticParserCorePrefixOptions{})
	if canonicalErr != nil || canonical.Boundary != gotreesitter.DiagnosticParserCoreFirstFork || canonical.Tokens != 68 || canonical.Dispatches != 117 || canonical.State != 186 || canonical.Lookahead.Symbol != 20 || canonical.ForkBoundaries != 2 || canonical.ForkLogicalPaths != 2 || !canonical.ExactRootDFA {
		t.Fatalf("exact scanner did not reach authenticated boundary: result=%+v err=%v", canonical, canonicalErr)
	}
	wrongScanners := []struct {
		name    string
		scanner gotreesitter.ExternalScanner
	}{
		{name: "nil"},
		{name: "different", scanner: grammars.JavaScriptExternalScanner{}},
		{name: "pointer", scanner: &grammars.GoExternalScanner{}},
		{name: "wrapper", scanner: wrappedGoScanner{}},
		{name: "fake", scanner: fakeGoScanner{}},
	}
	for _, test := range wrongScanners {
		t.Run(test.name, func(t *testing.T) {
			result, err := gotreesitter.DiagnosticParseParserCorePrefix(test.scanner, source, gotreesitter.DiagnosticParserCorePrefixOptions{})
			if err == nil || result.ExactRootDFA || result.Boundary != gotreesitter.DiagnosticParserCoreIdentity {
				t.Fatalf("wrong scanner admitted: result=%+v err=%v", result, err)
			}
		})
	}
}
