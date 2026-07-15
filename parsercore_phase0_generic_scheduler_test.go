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
			generic.Stop.Boundary != gotreesitter.DiagnosticParserCoreSubsequentConflictBoundary || generic.Stop.ElectionIndex != 105 || generic.Stop.State != 186 || generic.Stop.ByteOffset != 760 ||
			generic.Stop.Token.Symbol != 20 || generic.Stop.Token.Text != "{" || generic.Stop.Token.StartByte != 760 || generic.Stop.Token.EndByte != 761 ||
			generic.Stop.Token.Missing || generic.Stop.Token.NoLookahead || generic.Stop.Token.ExternalScannerToken ||
			generic.Tokens != 106 || generic.Dispatches != 207 || generic.GlobalBranchOrder != 4 || generic.NextCreationSeq != 7 ||
			len(generic.StartHeaders) != 2 || len(generic.Elections) != 3 || len(generic.Rounds) != 7 || len(generic.NoActionDrops) != 2 || len(generic.Stop.Headers) != 1 ||
			generic.Stop.Stats.Nodes != 221 || generic.Stop.Stats.Links != 220 || generic.Stop.Stats.Subtrees != 205 || generic.Stop.Stats.Children != 192 || generic.Stop.Stats.CurrentExactPaths != 1 {
			t.Fatalf("run %d generic scheduler boundary drifted: %+v", run, generic)
		}
		if generic.Stop.Work != (gotreesitter.DiagnosticParserCoreGenericWork{
			Passes: 10, ActionLookups: 20, Dispatches: 9, Reductions: 4,
			OrdinaryShifts: 5, OrdinaryCohorts: 2, NoActionDrops: 2,
			Elections: 3, Canonicalizations: 7, PeakHeaders: 3,
		}) {
			t.Fatalf("run %d generic scheduler work=%+v", run, generic.Stop.Work)
		}
		wantHeaders := []gotreesitter.DiagnosticParserCoreHeaderReceipt{{
			CreationSeq: 6, State: 186, ByteOffset: 760, ExactPaths: 1, Checkpoint: generic.Elections[2].ScannerAfter.SHA256,
		}}
		for index, want := range wantHeaders {
			if !reflect.DeepEqual(generic.Stop.Headers[index].Header, want) {
				t.Fatalf("run %d final header %d=%+v want=%+v", run, index, generic.Stop.Headers[index].Header, want)
			}
		}
		wantDrops := []gotreesitter.DiagnosticParserCoreHeaderReceipt{
			{CreationSeq: 4, State: 101, ByteOffset: 747, ExactPaths: 2, Checkpoint: generic.Elections[0].ScannerAfter.SHA256},
			{CreationSeq: 1, State: 826, ByteOffset: 748, ExactPaths: 1, Checkpoint: generic.Elections[1].ScannerAfter.SHA256},
		}
		for index, want := range wantDrops {
			if !reflect.DeepEqual(generic.NoActionDrops[index].Header.Header, want) {
				t.Fatalf("run %d no-action drop %d=%+v want=%+v", run, index, generic.NoActionDrops[index], want)
			}
		}
		if generic.NoActionDrops[0].ElectionIndex != 103 || generic.NoActionDrops[0].Token.Symbol != 9 ||
			generic.NoActionDrops[1].ElectionIndex != 104 || generic.NoActionDrops[1].Token.Symbol != 86 || generic.NoActionDrops[1].Token.Text != "rewriteEdit" {
			t.Fatalf("run %d no-action drop epochs drifted: %+v", run, generic.NoActionDrops)
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
		{name: "after-first-drop-token", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, MaxTokens: 104}},
		{name: "after-second-election-dispatch", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, MaxDispatches: 206}},
		{name: "after-second-drop-token", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, MaxTokens: 105}},
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
