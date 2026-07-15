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

func TestDiagnosticParserCoreGenericSchedulerClosesAtRequestedByte(t *testing.T) {
	source := parserCoreGenericRewriteSource(t)
	target := uint32(919)
	var first *gotreesitter.DiagnosticParserCoreGenericScheduler
	for run := 0; run < 3; run++ {
		result, routeErr := gotreesitter.DiagnosticParseParserCorePrefix(
			grammars.GoExternalScanner{}, source,
			gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, GenericStopAtClosedByte: &target},
		)
		if routeErr != nil || result.GenericScheduler == nil || !result.Completed {
			t.Fatalf("run %d generic scheduler result=%+v err=%v", run, result.GenericScheduler, routeErr)
		}
		generic := result.GenericScheduler
		completion := generic.Completion
		if generic.StartElectionIndex != 102 || generic.StartToken.Symbol != 86 || generic.StartToken.StartByte != 742 || generic.StartToken.EndByte != 747 ||
			completion == nil || completion.TargetByte != 919 || completion.ElectionIndex != 131 || completion.LastToken.Symbol != 92 || completion.LastToken.StartByte != 851 || completion.LastToken.EndByte != 919 ||
			completion.LastToken.Missing || completion.LastToken.NoLookahead || completion.LastToken.ExternalScannerToken ||
			generic.Tokens != 132 || generic.Dispatches != 281 || generic.GlobalBranchOrder != 8 || generic.NextCreationSeq != 11 ||
			len(generic.StartHeaders) != 2 || len(generic.Elections) != 29 || len(generic.Rounds) != 77 || len(generic.Conflicts) != 4 || len(generic.ExternalShifts) != 2 || len(generic.NoActionDrops) != 6 || len(completion.Headers) != 1 ||
			completion.Stats.Nodes != 299 || completion.Stats.Links != 298 || completion.Stats.Subtrees != 279 || completion.Stats.Children != 277 || completion.Stats.CurrentExactPaths != 1 ||
			!reflect.DeepEqual(generic.Stop, gotreesitter.DiagnosticParserCoreGenericStop{}) {
			t.Fatalf("run %d generic scheduler boundary drifted: %+v", run, generic)
		}
		if completion.Work != (gotreesitter.DiagnosticParserCoreGenericWork{
			Passes: 83, ActionLookups: 111, Dispatches: 83,
			Conflicts: 4, ConflictActions: 8, Forks: 4, ConflictHeads: 8,
			Reductions: 44, OrdinaryShifts: 34, OrdinaryCohorts: 6, ExtraShifts: 1, NoActionDrops: 6,
			Elections: 29, Canonicalizations: 77, PeakHeaders: 3,
		}) {
			t.Fatalf("run %d generic scheduler work=%+v", run, completion.Work)
		}
		wantHeaders := []gotreesitter.DiagnosticParserCoreHeaderReceipt{{
			CreationSeq: 10, State: 345, ByteOffset: 919, Shifted: true, ExactPaths: 1, Checkpoint: generic.Elections[28].ScannerAfter.SHA256,
		}}
		for index, want := range wantHeaders {
			if !reflect.DeepEqual(completion.Headers[index].Header, want) {
				t.Fatalf("run %d final header %d=%+v want=%+v", run, index, completion.Headers[index].Header, want)
			}
		}
		extraRound := generic.Rounds[76]
		if len(extraRound.Before) != 1 || extraRound.Before[0].State != 345 || extraRound.Before[0].ByteOffset != 850 || extraRound.Before[0].Shifted ||
			len(extraRound.Actions) != 1 || extraRound.Actions[0].HeaderIndex != 0 || extraRound.Actions[0].State != 345 || extraRound.Actions[0].ByteOffset != 850 || extraRound.Actions[0].Ordinal != 0 ||
			extraRound.Actions[0].Action.Type != gotreesitter.ParseActionShift || extraRound.Actions[0].Action.State != 0 || !extraRound.Actions[0].Action.Extra || extraRound.Actions[0].Action.ExtraChain || extraRound.Actions[0].Action.Repetition ||
			len(extraRound.After) != 1 || extraRound.After[0].State != 345 || extraRound.After[0].ByteOffset != 919 || !extraRound.After[0].Shifted || extraRound.After[0].Checkpoint != generic.Elections[28].ScannerAfter.SHA256 {
			t.Fatalf("run %d extra round drifted: %+v", run, extraRound)
		}
		if completion.Stats.Nodes-298 != 1 || completion.Stats.Links-297 != 1 || completion.Stats.Subtrees-278 != 1 || completion.Stats.Children != 277 {
			t.Fatalf("run %d extra physical delta drifted: %+v", run, completion.Stats)
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
		if result.Boundary != gotreesitter.DiagnosticParserCoreGenericClosed || result.State != 345 || result.Lookahead != (gotreesitter.Token{}) ||
			result.Tokens != generic.Tokens || result.Dispatches != generic.Dispatches || len(result.Elections) != 132 ||
			!reflect.DeepEqual(result.Elections[131].Token, completion.LastToken) {
			t.Fatalf("run %d top-level publication drifted: result=%+v completion=%+v", run, result, completion)
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
	phaseA := uint32(919)
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
		{name: "first-extra-shift-dispatch", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, GenericStopAtClosedByte: &phaseA, MaxDispatches: 280}},
		{name: "first-extra-shift-subtree", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, GenericStopAtClosedByte: &phaseA, Limits: core.Limits{MaxSubtrees: 278}}, rollbackError: "subtree arena cap"},
		{name: "first-extra-shift-link", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, GenericStopAtClosedByte: &phaseA, Limits: core.Limits{MaxLinks: 297}}, rollbackError: "link arena cap"},
		{name: "first-extra-shift-node", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, GenericStopAtClosedByte: &phaseA, Limits: core.Limits{MaxNodes: 298}}, rollbackError: "node arena cap"},
		{name: "after-extra-next-election", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, MaxTokens: 132}},
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

func TestDiagnosticParserCoreGenericSchedulerRejectsOvershotClosedByte(t *testing.T) {
	source := parserCoreGenericRewriteSource(t)
	target := uint32(918)
	result, routeErr := gotreesitter.DiagnosticParseParserCorePrefix(
		grammars.GoExternalScanner{}, source,
		gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, GenericStopAtClosedByte: &target},
	)
	if routeErr == nil || !strings.Contains(routeErr.Error(), "straddled or passed") {
		t.Fatalf("overshot target result=%+v err=%v", result.GenericScheduler, routeErr)
	}
	if result.GenericScheduler != nil || result.CachedDotClosure == nil || result.Tokens != 103 || result.Dispatches != 198 {
		t.Fatalf("overshot target leaked generic publication: %+v", result)
	}
}

func TestDiagnosticParserCoreGenericSchedulerContinuesToNextRequestedClosedByte(t *testing.T) {
	source := parserCoreGenericRewriteSource(t)
	target := uint32(924)
	result, routeErr := gotreesitter.DiagnosticParseParserCorePrefix(
		grammars.GoExternalScanner{}, source,
		gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, GenericStopAtClosedByte: &target},
	)
	if routeErr != nil || !result.Completed || result.GenericScheduler == nil || result.GenericScheduler.Completion == nil {
		t.Fatalf("continuation result=%+v err=%v", result, routeErr)
	}
	generic := result.GenericScheduler
	completion := generic.Completion
	if result.Boundary != gotreesitter.DiagnosticParserCoreGenericClosed || result.State != 16 || result.Lookahead != (gotreesitter.Token{}) ||
		result.Tokens != 133 || result.Dispatches != 283 || len(generic.Elections) != 30 || len(generic.Rounds) != 79 ||
		completion.TargetByte != 924 || completion.ElectionIndex != 132 || completion.LastToken.Symbol != 12 || completion.LastToken.Text != "func" || completion.LastToken.StartByte != 920 || completion.LastToken.EndByte != 924 ||
		len(completion.Headers) != 1 || completion.Headers[0].Header.State != 16 || completion.Headers[0].Header.ByteOffset != 924 || !completion.Headers[0].Header.Shifted ||
		completion.Stats != (core.Stats{Nodes: 302, Links: 301, Subtrees: 281, Children: 281, CurrentExactPaths: 1}) ||
		completion.Work != (gotreesitter.DiagnosticParserCoreGenericWork{
			Passes: 85, ActionLookups: 113, Dispatches: 85,
			Conflicts: 4, ConflictActions: 8, Forks: 4, ConflictHeads: 8,
			Reductions: 45, OrdinaryShifts: 35, OrdinaryCohorts: 6, ExtraShifts: 1, NoActionDrops: 6,
			Elections: 30, Canonicalizations: 79, PeakHeaders: 3,
		}) {
		t.Fatalf("continuation drifted: result=%+v generic=%+v", result, generic)
	}
	election := generic.Elections[29]
	if election.Token.Symbol != 12 || election.Token.Text != "func" || election.Token.StartByte != 920 || election.Token.EndByte != 924 || election.Token.ExternalScannerToken ||
		election.ScannerBefore != generic.Elections[28].ScannerAfter {
		t.Fatalf("continuation election drifted: %+v", election)
	}
}

func TestDiagnosticParserCoreGenericSchedulerCrossesFirstShallowLinkCap(t *testing.T) {
	source := parserCoreGenericRewriteSource(t)
	target := uint32(2440)
	result, routeErr := gotreesitter.DiagnosticParseParserCorePrefix(
		grammars.GoExternalScanner{}, source,
		gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, GenericStopAtClosedByte: &target},
	)
	if routeErr != nil || !result.Completed || result.GenericScheduler == nil || result.GenericScheduler.Completion == nil {
		t.Fatalf("state 46 byte 2440 closure result=%+v err=%v", result, routeErr)
	}
	completion := result.GenericScheduler.Completion
	// Before shallow same-predecessor selection, condensation failed at the
	// unshifted (state 46, byte 2440) boundary. Completing the authenticated
	// token through shifted state 473 proves that boundary is now crossed.
	if completion.TargetByte != target || completion.ElectionIndex != 485 || completion.LastToken.Symbol != 21 || completion.LastToken.StartByte != 2439 || completion.LastToken.EndByte != target ||
		len(completion.Headers) != 1 || completion.Headers[0].Header.State != 473 || completion.Headers[0].Header.ByteOffset != target || !completion.Headers[0].Header.Shifted || completion.Headers[0].Header.ExactPaths != 2 ||
		completion.Stats != (core.Stats{Nodes: 1313, Links: 1312, Subtrees: 1146, Children: 1170, CurrentExactPaths: 2}) {
		t.Fatalf("state 46 byte 2440 closure drifted: %+v", completion)
	}
}

func TestDiagnosticParserCoreGenericSchedulerCrossesReductionFreshnessCycle(t *testing.T) {
	source := parserCoreGenericRewriteSource(t)
	target := uint32(3070)
	result, routeErr := gotreesitter.DiagnosticParseParserCorePrefix(
		grammars.GoExternalScanner{}, source,
		gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, GenericStopAtClosedByte: &target},
	)
	if routeErr != nil || !result.Completed || result.GenericScheduler == nil || result.GenericScheduler.Completion == nil {
		t.Fatalf("byte 3070 closure result=%+v err=%v", result, routeErr)
	}
	completion := result.GenericScheduler.Completion
	if completion.TargetByte != target || completion.ElectionIndex != 597 || completion.LastToken.Symbol != 75 || completion.LastToken.Text != "&&" || completion.LastToken.StartByte != 3068 || completion.LastToken.EndByte != target ||
		len(completion.Headers) != 1 || completion.Headers[0].Header.CreationSeq != 109 || completion.Headers[0].Header.State != 182 || completion.Headers[0].Header.ByteOffset != target || !completion.Headers[0].Header.Shifted || completion.Headers[0].Header.Paused || completion.Headers[0].Header.ExactPaths != 1 ||
		completion.Stats != (core.Stats{Nodes: 1663, Links: 1662, Subtrees: 1455, Children: 1500, CurrentExactPaths: 1}) ||
		completion.Work != (gotreesitter.DiagnosticParserCoreGenericWork{
			Passes: 1247, ActionLookups: 1778, Dispatches: 1298,
			Conflicts: 80, ConflictActions: 168, Forks: 88, ConflictHeads: 179,
			Reductions: 594, OrdinaryShifts: 612, OrdinaryCohorts: 118, ExtraShifts: 12,
			ReductionPauses: 20, NoActionDrops: 90, Elections: 495, Canonicalizations: 1165, PeakHeaders: 4,
		}) || len(result.GenericScheduler.Rounds) != 1165 || len(result.GenericScheduler.NoActionDrops) != 90 {
		t.Fatalf("byte 3070 closure drifted: %+v", completion)
	}
}
