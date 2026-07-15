//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"reflect"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
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
			generic.Stop.Boundary != gotreesitter.DiagnosticParserCoreExtra || generic.Stop.Detail != "generic scheduler does not yet apply extra shifts" ||
			generic.Stop.ElectionIndex != 131 || generic.Stop.State != 345 || generic.Stop.ByteOffset != 850 ||
			generic.Stop.Token.Symbol != 92 || generic.Stop.Token.StartByte != 851 || generic.Stop.Token.EndByte != 919 ||
			generic.Stop.Token.Missing || generic.Stop.Token.NoLookahead || generic.Stop.Token.ExternalScannerToken ||
			generic.Tokens != 132 || generic.Dispatches != 280 || generic.GlobalBranchOrder != 8 || generic.NextCreationSeq != 11 ||
			len(generic.StartHeaders) != 2 || len(generic.Elections) != 29 || len(generic.Rounds) != 76 || len(generic.Conflicts) != 4 || len(generic.ExternalShifts) != 2 || len(generic.NoActionDrops) != 6 || len(generic.Stop.Headers) != 1 ||
			generic.Stop.Stats.Nodes != 298 || generic.Stop.Stats.Links != 297 || generic.Stop.Stats.Subtrees != 278 || generic.Stop.Stats.Children != 277 || generic.Stop.Stats.CurrentExactPaths != 1 {
			t.Fatalf("run %d generic scheduler boundary drifted: %+v", run, generic)
		}
		if generic.Stop.Work != (gotreesitter.DiagnosticParserCoreGenericWork{
			Passes: 83, ActionLookups: 111, Dispatches: 82,
			Conflicts: 4, ConflictActions: 8, Forks: 4, ConflictHeads: 8,
			Reductions: 44, OrdinaryShifts: 34, OrdinaryCohorts: 6, NoActionDrops: 6,
			Elections: 29, Canonicalizations: 76, PeakHeaders: 3,
		}) {
			t.Fatalf("run %d generic scheduler work=%+v", run, generic.Stop.Work)
		}
		wantHeaders := []gotreesitter.DiagnosticParserCoreHeaderReceipt{{
			CreationSeq: 10, State: 345, ByteOffset: 850, ExactPaths: 1, Checkpoint: generic.Elections[28].ScannerAfter.SHA256,
		}}
		for index, want := range wantHeaders {
			if !reflect.DeepEqual(generic.Stop.Headers[index].Header, want) {
				t.Fatalf("run %d final header %d=%+v want=%+v", run, index, generic.Stop.Headers[index].Header, want)
			}
		}
		wantExternal := []struct {
			election, electionIndex, round int
			start, end, scannerStart       uint32
			payloadID                      uint32
		}{
			{128, 25, 70, 847, 848, 847, 273},
			{130, 27, 75, 849, 850, 849, 278},
		}
		for index, want := range wantExternal {
			external := generic.ExternalShifts[index]
			election := generic.Elections[want.electionIndex]
			if external.ElectionIndex != want.election || external.RoundIndex != want.round || external.Token.Symbol != 94 ||
				external.Token.StartByte != want.start || external.Token.EndByte != want.end || !external.Token.ExternalScannerToken || external.Token.ExternalScannerStartByte != want.scannerStart ||
				!reflect.DeepEqual(external.Token, election.Token) || external.ScannerBefore != election.ScannerBefore || external.ScannerAfter != election.ScannerAfter ||
				len(external.Payloads) != 1 || external.Payloads[0].ID != want.payloadID || external.Payloads[0].Symbol != 94 || external.Payloads[0].StartByte != want.start || external.Payloads[0].EndByte != want.end ||
				external.Payloads[0].ProductionID != 0 || external.Payloads[0].DynamicPrecedence != 0 || len(external.Payloads[0].Children) != 0 || len(external.Payloads[0].Fields) != 0 || len(external.Payloads[0].Aliases) != 0 ||
				!external.Payloads[0].External || external.Payloads[0].Extra || !external.Payloads[0].Terminal {
				t.Fatalf("run %d external shift %d drifted: receipt=%+v election=%+v", run, index, external, election)
			}
		}
		if generic.Elections[26].Token.Symbol != 21 || generic.Elections[26].Token.StartByte != 848 || generic.Elections[26].Token.EndByte != 849 || generic.Elections[26].Token.ExternalScannerToken ||
			generic.Elections[25].ScannerAfter != generic.Elections[26].ScannerBefore || generic.Elections[26].ScannerAfter != generic.Elections[27].ScannerBefore || generic.Elections[27].ScannerAfter != generic.Elections[28].ScannerBefore {
			t.Fatalf("run %d external/ordinary checkpoint continuity drifted: %+v", run, generic.Elections[25:29])
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
		name          string
		options       gotreesitter.DiagnosticParserCorePrefixOptions
		rollbackError string
	}{
		{name: "first-shift", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, MaxDispatches: 199}},
		{name: "after-election-dispatch", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, MaxDispatches: 200}},
		{name: "after-first-drop-token", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, MaxTokens: 104}},
		{name: "after-second-election-dispatch", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, MaxDispatches: 206}},
		{name: "first-conflict-dispatch", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, MaxDispatches: 207}},
		{name: "first-external-shift-dispatch", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, MaxDispatches: 274}},
		{name: "first-external-shift-subtree", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, Limits: core.Limits{MaxSubtrees: 272}}, rollbackError: "subtree arena cap"},
		{name: "after-second-drop-token", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, MaxTokens: 105}},
		{name: "next-election", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, MaxTokens: 103}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, routeErr := gotreesitter.DiagnosticParseParserCorePrefix(grammars.GoExternalScanner{}, source, test.options)
			if routeErr == nil {
				t.Fatal("cap route unexpectedly succeeded")
			}
			if test.rollbackError == "" && result.Boundary != gotreesitter.DiagnosticParserCoreCap {
				t.Fatalf("cap route boundary=%s err=%v", result.Boundary, routeErr)
			}
			if test.rollbackError != "" && (result.Boundary != gotreesitter.DiagnosticParserCoreCachedDotClosureBoundary || !strings.Contains(routeErr.Error(), test.rollbackError)) {
				t.Fatalf("rollback route boundary=%s err=%v, want cached-dot boundary and %q", result.Boundary, routeErr, test.rollbackError)
			}
			if result.GenericScheduler != nil || result.CachedDotClosure == nil || result.Tokens != 103 || result.Dispatches != 198 ||
				len(result.Elections) != 103 || result.State != 164 || result.Lookahead.Symbol != 86 || result.Lookahead.StartByte != 742 || result.Lookahead.EndByte != 747 {
				t.Fatalf("failed generic transaction leaked publication: result=%+v", result)
			}
		})
	}
}
