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
	if result.Boundary != gotreesitter.DiagnosticParserCoreExtra || result.Tokens != 16 || result.Dispatches != 23 || result.State != 542 || result.Lookahead.Symbol != 92 {
		t.Fatalf("rewrite prefix identity drifted: boundary=%s tokens=%d dispatches=%d state=%d lookahead=%d", result.Boundary, result.Tokens, result.Dispatches, result.State, result.Lookahead.Symbol)
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
	t.Logf("parser-core rewrite prefix boundary=%s detail=%q tokens=%d dispatches=%d state=%d lookahead=%d", result.Boundary, result.Detail, result.Tokens, result.Dispatches, result.State, result.Lookahead.Symbol)
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
	if canonicalErr == nil || canonical.Boundary != gotreesitter.DiagnosticParserCoreExtra || !canonical.ExactRootDFA {
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
