//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"reflect"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func parserCoreGenericRewriteSource(t *testing.T) []byte {
	t.Helper()
	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		if fixture.Fixture.ID == "rewrite" {
			return fixture.Source
		}
	}
	t.Fatal("rewrite fixture missing")
	return nil
}

func TestDiagnosticParserCoreGenericSchedulerFindsFirstUnsupportedSemantic(t *testing.T) {
	source := parserCoreGenericRewriteSource(t)
	var first *gotreesitter.DiagnosticParserCoreGenericScheduler
	for run := 0; run < 3; run++ {
		result, routeErr := gotreesitter.DiagnosticParseParserCorePrefix(
			grammars.GoExternalScanner{}, source,
			gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true},
		)
		if routeErr == nil || result.GenericScheduler == nil {
			t.Fatalf("run %d generic scheduler result=%+v err=%v", run, result.GenericScheduler, routeErr)
		}
		generic := result.GenericScheduler
		if generic.StartElectionIndex != 102 || generic.StartToken.Symbol != 86 || generic.StartToken.StartByte != 742 || generic.StartToken.EndByte != 747 ||
			generic.Stop.Boundary != gotreesitter.DiagnosticParserCoreNoAction || generic.Stop.ElectionIndex != 103 || generic.Stop.State != 101 || generic.Stop.ByteOffset != 747 ||
			generic.Stop.Token.Symbol != 9 || generic.Stop.Token.StartByte != 747 || generic.Stop.Token.EndByte != 748 ||
			generic.Stop.Token.Missing || generic.Stop.Token.NoLookahead || generic.Stop.Token.ExternalScannerToken ||
			generic.Tokens != 104 || generic.Dispatches != 206 || generic.GlobalBranchOrder != 4 || generic.NextCreationSeq != 7 ||
			len(generic.StartHeaders) != 2 || len(generic.Elections) != 1 || len(generic.Rounds) != 6 || len(generic.Stop.Headers) != 3 ||
			generic.Stop.Stats.Nodes != 220 || generic.Stop.Stats.Links != 219 || generic.Stop.Stats.Subtrees != 204 || generic.Stop.Stats.Children != 192 || generic.Stop.Stats.CurrentExactPaths != 2 {
			t.Fatalf("run %d generic scheduler boundary drifted: %+v", run, generic)
		}
		if generic.Stop.Work != (gotreesitter.DiagnosticParserCoreGenericWork{
			Passes: 7, ActionLookups: 16, Dispatches: 8, Reductions: 4,
			OrdinaryShifts: 4, OrdinaryCohorts: 2, Elections: 1,
			Canonicalizations: 6, PeakHeaders: 3,
		}) {
			t.Fatalf("run %d generic scheduler work=%+v", run, generic.Stop.Work)
		}
		wantHeaders := []gotreesitter.DiagnosticParserCoreHeaderReceipt{
			{CreationSeq: 1, State: 826, ByteOffset: 748, Shifted: true, ExactPaths: 1, Checkpoint: generic.Elections[0].ScannerAfter.SHA256},
			{CreationSeq: 6, State: 716, ByteOffset: 748, Shifted: true, ExactPaths: 1, Checkpoint: generic.Elections[0].ScannerAfter.SHA256},
			{CreationSeq: 4, State: 101, ByteOffset: 747, ExactPaths: 2, Checkpoint: generic.Elections[0].ScannerAfter.SHA256},
		}
		for index, want := range wantHeaders {
			if !reflect.DeepEqual(generic.Stop.Headers[index].Header, want) {
				t.Fatalf("run %d final header %d=%+v want=%+v", run, index, generic.Stop.Headers[index].Header, want)
			}
		}
		if result.Boundary != generic.Stop.Boundary || result.State != generic.Stop.State || !reflect.DeepEqual(result.Lookahead, generic.Stop.Token) ||
			result.Tokens != generic.Tokens || result.Dispatches != generic.Dispatches {
			t.Fatalf("run %d top-level publication drifted: result=%+v stop=%+v", run, result, generic.Stop)
		}
		if first == nil {
			first = generic
		} else if !reflect.DeepEqual(first, generic) {
			t.Fatalf("run %d receipt is nondeterministic\nfirst=%+v\nnext=%+v", run, first, generic)
		}
	}
}

func TestDiagnosticParserCoreGenericSchedulerCapsPublishNothing(t *testing.T) {
	source := parserCoreGenericRewriteSource(t)
	tests := []struct {
		name    string
		options gotreesitter.DiagnosticParserCorePrefixOptions
	}{
		{name: "first-shift", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, MaxDispatches: 199}},
		{name: "after-election-dispatch", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, MaxDispatches: 200}},
		{name: "next-election", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, MaxTokens: 103}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, routeErr := gotreesitter.DiagnosticParseParserCorePrefix(grammars.GoExternalScanner{}, source, test.options)
			if routeErr == nil || result.Boundary != gotreesitter.DiagnosticParserCoreCap {
				t.Fatalf("cap route result=%+v err=%v", result, routeErr)
			}
			if result.GenericScheduler != nil || result.CachedDotClosure == nil || result.Tokens != 103 || result.Dispatches != 198 ||
				len(result.Elections) != 103 || result.State != 164 || result.Lookahead.Symbol != 86 || result.Lookahead.StartByte != 742 || result.Lookahead.EndByte != 747 {
				t.Fatalf("failed generic transaction leaked publication: result=%+v", result)
			}
		})
	}
}
