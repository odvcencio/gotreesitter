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
			generic.Stop.Boundary != gotreesitter.DiagnosticParserCoreRoute || generic.Stop.Detail != "generic scheduler does not yet carry external-token checkpoint identity" ||
			generic.Stop.ElectionIndex != 128 || generic.Stop.State != 1189 || generic.Stop.ByteOffset != 847 ||
			generic.Stop.Token.Symbol != 94 || generic.Stop.Token.StartByte != 847 || generic.Stop.Token.EndByte != 848 ||
			generic.Stop.Token.Missing || generic.Stop.Token.NoLookahead || !generic.Stop.Token.ExternalScannerToken ||
			generic.Tokens != 129 || generic.Dispatches != 267 || generic.GlobalBranchOrder != 8 || generic.NextCreationSeq != 11 ||
			len(generic.StartHeaders) != 2 || len(generic.Elections) != 26 || len(generic.Rounds) != 63 || len(generic.Conflicts) != 4 || len(generic.NoActionDrops) != 6 || len(generic.Stop.Headers) != 1 ||
			generic.Stop.Stats.Nodes != 285 || generic.Stop.Stats.Links != 284 || generic.Stop.Stats.Subtrees != 265 || generic.Stop.Stats.Children != 253 || generic.Stop.Stats.CurrentExactPaths != 1 {
			t.Fatalf("run %d generic scheduler boundary drifted: %+v", run, generic)
		}
		if generic.Stop.Work != (gotreesitter.DiagnosticParserCoreGenericWork{
			Passes: 70, ActionLookups: 97, Dispatches: 69,
			Conflicts: 4, ConflictActions: 8, Forks: 4, ConflictHeads: 8,
			Reductions: 34, OrdinaryShifts: 31, OrdinaryCohorts: 6, NoActionDrops: 6,
			Elections: 26, Canonicalizations: 63, PeakHeaders: 3,
		}) {
			t.Fatalf("run %d generic scheduler work=%+v", run, generic.Stop.Work)
		}
		wantHeaders := []gotreesitter.DiagnosticParserCoreHeaderReceipt{{
			CreationSeq: 10, State: 1189, ByteOffset: 847, ExactPaths: 1, Checkpoint: generic.Elections[25].ScannerAfter.SHA256,
		}}
		for index, want := range wantHeaders {
			if !reflect.DeepEqual(generic.Stop.Headers[index].Header, want) {
				t.Fatalf("run %d final header %d=%+v want=%+v", run, index, generic.Stop.Headers[index].Header, want)
			}
		}
		wantConflicts := []struct {
			election, round                              int
			symbol                                       gotreesitter.Symbol
			start                                        uint32
			beforeOrder, afterOrder, beforeSeq, afterSeq uint64
			secondaryType, primaryType                   gotreesitter.ParseActionType
			secondaryState                               gotreesitter.StateID
			secondarySymbol, primarySymbol               gotreesitter.Symbol
		}{
			{105, 7, 20, 760, 4, 5, 7, 8, gotreesitter.ParseActionShift, gotreesitter.ParseActionReduce, 193, 0, 171},
			{109, 13, 4, 779, 5, 6, 8, 9, gotreesitter.ParseActionShift, gotreesitter.ParseActionReduce, 194, 0, 171},
			{117, 33, 4, 810, 6, 7, 9, 10, gotreesitter.ParseActionShift, gotreesitter.ParseActionReduce, 194, 0, 171},
			{125, 53, 9, 842, 7, 8, 10, 11, gotreesitter.ParseActionReduce, gotreesitter.ParseActionReduce, 0, 171, 121},
		}
		for index, want := range wantConflicts {
			conflict := generic.Conflicts[index]
			if conflict.ElectionIndex != want.election || conflict.Round.Index != want.round || conflict.Token.Symbol != want.symbol || conflict.Token.StartByte != want.start ||
				conflict.BranchOrderBefore != want.beforeOrder || conflict.BranchOrderAfter != want.afterOrder || conflict.NextCreationSeqBefore != want.beforeSeq || conflict.NextCreationSeqAfter != want.afterSeq ||
				len(conflict.Round.Actions) != 2 || conflict.Round.Actions[0].Ordinal != 1 || conflict.Round.Actions[0].BranchOrder != want.afterOrder ||
				conflict.Round.Actions[0].Action.Type != want.secondaryType || conflict.Round.Actions[0].Action.State != want.secondaryState || conflict.Round.Actions[0].Action.Symbol != want.secondarySymbol ||
				conflict.Round.Actions[1].Ordinal != 0 || conflict.Round.Actions[1].BranchOrder != 0 || conflict.Round.Actions[1].Action.Type != want.primaryType || conflict.Round.Actions[1].Action.Symbol != want.primarySymbol ||
				conflict.PrimaryOutput.State == 0 || len(conflict.SecondaryArms) != 1 || len(conflict.SecondaryArms[0].Outputs) != 1 || len(conflict.AdditionalPrimaryOutputs) != 0 || len(conflict.After) != 2 {
				t.Fatalf("run %d conflict %d drifted: %+v", run, index, conflict)
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
		{name: "first-conflict-dispatch", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, MaxDispatches: 207}},
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
