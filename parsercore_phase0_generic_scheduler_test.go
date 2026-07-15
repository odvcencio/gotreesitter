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
		if !generic.SeedOwned || generic.StartElectionIndex != -1 || generic.StartToken != (gotreesitter.Token{}) ||
			generic.StartCheckpoint != generic.Elections[0].ScannerBefore ||
			completion == nil || completion.TargetByte != 919 || completion.ElectionIndex != 131 || completion.LastToken.Symbol != 92 || completion.LastToken.StartByte != 851 || completion.LastToken.EndByte != 919 ||
			completion.LastToken.Missing || completion.LastToken.NoLookahead || completion.LastToken.ExternalScannerToken ||
			generic.Tokens != 132 || generic.Dispatches != 279 || generic.GlobalBranchOrder != 8 || generic.NextCreationSeq != 11 ||
			len(generic.StartHeaders) != 1 || generic.StartHeaders[0].Header.State != 1 || generic.StartHeaders[0].Header.ByteOffset != 0 || generic.StartHeaders[0].Header.Shifted || generic.StartHeaders[0].Header.Checkpoint != generic.StartCheckpoint.SHA256 ||
			len(generic.Elections) != 132 || len(generic.Rounds) != 269 || len(generic.Conflicts) != 8 || len(generic.ExternalShifts) != 15 || len(generic.NoActionDrops) != 8 || len(completion.Headers) != 1 ||
			completion.Stats.Nodes != 299 || completion.Stats.Links != 298 || completion.Stats.Subtrees != 277 || completion.Stats.Children != 277 || completion.Stats.CurrentExactPaths != 1 ||
			!reflect.DeepEqual(generic.Stop, gotreesitter.DiagnosticParserCoreGenericStop{}) {
			t.Fatalf("run %d generic scheduler boundary drifted: %+v", run, generic)
		}
		if completion.Work != (gotreesitter.DiagnosticParserCoreGenericWork{
			Passes: 277, ActionLookups: 314, Dispatches: 279,
			Conflicts: 8, ConflictActions: 16, Forks: 8, ConflictHeads: 17,
			Reductions: 131, OrdinaryShifts: 133, OrdinaryCohorts: 10, ExtraShifts: 7, NoActionDrops: 8,
			Elections: 132, Canonicalizations: 269, PeakHeaders: 3,
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
		extraRound := generic.Rounds[268]
		if len(extraRound.Before) != 1 || extraRound.Before[0].State != 345 || extraRound.Before[0].ByteOffset != 850 || extraRound.Before[0].Shifted ||
			len(extraRound.Actions) != 1 || extraRound.Actions[0].HeaderIndex != 0 || extraRound.Actions[0].State != 345 || extraRound.Actions[0].ByteOffset != 850 || extraRound.Actions[0].Ordinal != 0 ||
			extraRound.Actions[0].Action.Type != gotreesitter.ParseActionShift || extraRound.Actions[0].Action.State != 0 || !extraRound.Actions[0].Action.Extra || extraRound.Actions[0].Action.ExtraChain || extraRound.Actions[0].Action.Repetition ||
			len(extraRound.After) != 1 || extraRound.After[0].State != 345 || extraRound.After[0].ByteOffset != 919 || !extraRound.After[0].Shifted || extraRound.After[0].Checkpoint != generic.Elections[28].ScannerAfter.SHA256 {
			t.Fatalf("run %d extra round drifted: %+v", run, extraRound)
		}
		if completion.Stats.Nodes-298 != 1 || completion.Stats.Links-297 != 1 || completion.Stats.Subtrees-276 != 1 || completion.Stats.Children != 277 {
			t.Fatalf("run %d extra physical delta drifted: %+v", run, completion.Stats)
		}
		wantExternal := []struct {
			election, electionIndex, round int
			start, end, scannerStart       uint32
			payloadID                      uint32
		}{
			{128, 128, 262, 847, 848, 847, 271},
			{130, 130, 267, 849, 850, 849, 276},
		}
		for index, want := range wantExternal {
			external := generic.ExternalShifts[index+13]
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
		if generic.Elections[129].Token.Symbol != 21 || generic.Elections[129].Token.StartByte != 848 || generic.Elections[129].Token.EndByte != 849 || generic.Elections[129].Token.ExternalScannerToken ||
			generic.Elections[128].ScannerAfter != generic.Elections[129].ScannerBefore || generic.Elections[129].ScannerAfter != generic.Elections[130].ScannerBefore || generic.Elections[130].ScannerAfter != generic.Elections[131].ScannerBefore {
			t.Fatalf("run %d external/ordinary checkpoint continuity drifted: %+v", run, generic.Elections[128:132])
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
			{105, 199, 20, 760, 4, 5, 7, 8, gotreesitter.ParseActionShift, gotreesitter.ParseActionReduce, 193, 0, 171},
			{109, 205, 4, 779, 5, 6, 8, 9, gotreesitter.ParseActionShift, gotreesitter.ParseActionReduce, 194, 0, 171},
			{117, 225, 4, 810, 6, 7, 9, 10, gotreesitter.ParseActionShift, gotreesitter.ParseActionReduce, 194, 0, 171},
			{125, 245, 9, 842, 7, 8, 10, 11, gotreesitter.ParseActionReduce, gotreesitter.ParseActionReduce, 0, 171, 121},
		}
		for index, want := range wantConflicts {
			conflict := generic.Conflicts[index+4]
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
			{CreationSeq: 4, State: 101, ByteOffset: 747, ExactPaths: 2, Checkpoint: generic.Elections[103].ScannerAfter.SHA256},
			{CreationSeq: 1, State: 826, ByteOffset: 748, ExactPaths: 1, Checkpoint: generic.Elections[104].ScannerAfter.SHA256},
		}
		for index, want := range wantDrops {
			if !reflect.DeepEqual(generic.NoActionDrops[index+2].Header.Header, want) {
				t.Fatalf("run %d no-action drop %d=%+v want=%+v", run, index, generic.NoActionDrops[index+2], want)
			}
		}
		if generic.NoActionDrops[2].ElectionIndex != 103 || generic.NoActionDrops[2].Token.Symbol != 9 ||
			generic.NoActionDrops[3].ElectionIndex != 104 || generic.NoActionDrops[3].Token.Symbol != 86 || generic.NoActionDrops[3].Token.Text != "rewriteEdit" {
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
	tests := []struct {
		name         string
		options      gotreesitter.DiagnosticParserCorePrefixOptions
		detail       string
		typedDecline bool
		election     int
		symbol       gotreesitter.Symbol
		start        uint32
		dispatches   uint64
		stageZero    func(gotreesitter.DiagnosticParserCoreGenericWork) bool
	}{
		{name: "before-first-shift", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, Limits: core.Limits{MaxNodes: 1}}, detail: "node arena cap", election: 0, symbol: 2, start: 0, dispatches: 0, stageZero: func(work gotreesitter.DiagnosticParserCoreGenericWork) bool { return work.OrdinaryShifts == 0 }},
		{name: "before-first-conflict", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, MaxDispatches: 116}, detail: "dispatch cap", typedDecline: true, election: 67, symbol: 20, start: 579, dispatches: 116, stageZero: func(work gotreesitter.DiagnosticParserCoreGenericWork) bool { return work.Conflicts == 0 }},
		{name: "before-first-extra", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, Limits: core.Limits{MaxSubtrees: 22}}, detail: "subtree arena cap", election: 15, symbol: 92, start: 49, dispatches: 22, stageZero: func(work gotreesitter.DiagnosticParserCoreGenericWork) bool { return work.ExtraShifts == 0 }},
		{name: "before-accept", options: gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, MaxDispatches: 2681}, detail: "dispatch cap", typedDecline: true, election: 1035, symbol: 0, start: uint32(len(source)), dispatches: 2681, stageZero: func(work gotreesitter.DiagnosticParserCoreGenericWork) bool { return work.Accepts == 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			private, privateErr := gotreesitter.RunDiagnosticParserCoreSeedCapForTest(grammars.GoExternalScanner{}, source, test.options)
			if privateErr == nil || !strings.Contains(privateErr.Error(), test.detail) || private.ElectionIndex != test.election || private.Token.Symbol != test.symbol || private.Token.StartByte != test.start || private.Dispatches != test.dispatches || !test.stageZero(private.Work) {
				t.Fatalf("private cap point drifted: point=%+v err=%v", private, privateErr)
			}
			result, routeErr := gotreesitter.DiagnosticParseParserCorePrefix(grammars.GoExternalScanner{}, source, test.options)
			if routeErr == nil {
				t.Fatal("cap route unexpectedly succeeded")
			}
			if !strings.Contains(routeErr.Error(), test.detail) || test.typedDecline && result.Boundary != gotreesitter.DiagnosticParserCoreCap || !test.typedDecline && result.Boundary != "" {
				t.Fatalf("cap route boundary=%s err=%v, want %q", result.Boundary, routeErr, test.detail)
			}
			if result.GenericScheduler != nil || result.MaterializedTree != nil || result.Materialized || result.Completed ||
				result.CachedDotClosure != nil || result.Tokens != 0 || result.Dispatches != 0 || len(result.Elections) != 0 || result.State != 0 || result.Lookahead != (gotreesitter.Token{}) {
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
	if result.GenericScheduler != nil || result.MaterializedTree != nil || result.CachedDotClosure != nil || result.Tokens != 0 || result.Dispatches != 0 || len(result.Elections) != 0 {
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
		result.Tokens != 133 || result.Dispatches != 281 || len(generic.Elections) != 133 || len(generic.Rounds) != 271 ||
		completion.TargetByte != 924 || completion.ElectionIndex != 132 || completion.LastToken.Symbol != 12 || completion.LastToken.Text != "func" || completion.LastToken.StartByte != 920 || completion.LastToken.EndByte != 924 ||
		len(completion.Headers) != 1 || completion.Headers[0].Header.State != 16 || completion.Headers[0].Header.ByteOffset != 924 || !completion.Headers[0].Header.Shifted ||
		completion.Stats != (core.Stats{Nodes: 302, Links: 301, Subtrees: 279, Children: 281, CurrentExactPaths: 1}) ||
		completion.Work != (gotreesitter.DiagnosticParserCoreGenericWork{
			Passes: 279, ActionLookups: 316, Dispatches: 281,
			Conflicts: 8, ConflictActions: 16, Forks: 8, ConflictHeads: 17,
			Reductions: 132, OrdinaryShifts: 134, OrdinaryCohorts: 10, ExtraShifts: 7, NoActionDrops: 8,
			Elections: 133, Canonicalizations: 271, PeakHeaders: 3,
		}) {
		t.Fatalf("continuation drifted: result=%+v generic=%+v", result, generic)
	}
	election := generic.Elections[132]
	if election.Token.Symbol != 12 || election.Token.Text != "func" || election.Token.StartByte != 920 || election.Token.EndByte != 924 || election.Token.ExternalScannerToken ||
		election.ScannerBefore != generic.Elections[131].ScannerAfter {
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
		completion.Stats != (core.Stats{Nodes: 1313, Links: 1312, Subtrees: 1144, Children: 1170, CurrentExactPaths: 2}) {
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
		completion.Stats != (core.Stats{Nodes: 1663, Links: 1662, Subtrees: 1453, Children: 1500, CurrentExactPaths: 1}) ||
		completion.Work != (gotreesitter.DiagnosticParserCoreGenericWork{
			Passes: 1441, ActionLookups: 1981, Dispatches: 1494,
			Conflicts: 84, ConflictActions: 176, Forks: 92, ConflictHeads: 188,
			Reductions: 681, OrdinaryShifts: 711, OrdinaryCohorts: 122, ExtraShifts: 18,
			ReductionPauses: 20, NoActionDrops: 92, Elections: 598, Canonicalizations: 1357, PeakHeaders: 4,
		}) || len(result.GenericScheduler.Rounds) != 1357 || len(result.GenericScheduler.NoActionDrops) != 92 {
		t.Fatalf("byte 3070 closure drifted: %+v", completion)
	}
}

func TestDiagnosticParserCoreGenericSchedulerCrossesMultiHeadExtraCohort(t *testing.T) {
	source := parserCoreGenericRewriteSource(t)
	braceTarget := uint32(3383)
	braceResult, braceErr := gotreesitter.DiagnosticParseParserCorePrefix(
		grammars.GoExternalScanner{}, source,
		gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, GenericStopAtClosedByte: &braceTarget},
	)
	if braceErr != nil || !braceResult.Completed || braceResult.GenericScheduler == nil || braceResult.GenericScheduler.Completion == nil {
		t.Fatalf("byte 3383 closure result=%+v err=%v", braceResult, braceErr)
	}
	braceCompletion := braceResult.GenericScheduler.Completion
	if braceCompletion.Stats != (core.Stats{Nodes: 1884, Links: 1883, Subtrees: 1653, Children: 1725, CurrentExactPaths: 1}) ||
		braceCompletion.Work.ExtraShifts != 19 || braceCompletion.Work.ExtraCohorts != 0 {
		t.Fatalf("brace baseline drifted: %+v", braceCompletion)
	}
	commentTarget := uint32(3427)
	commentResult, commentErr := gotreesitter.DiagnosticParseParserCorePrefix(
		grammars.GoExternalScanner{}, source,
		gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, GenericStopAtClosedByte: &commentTarget},
	)
	if commentErr != nil || !commentResult.Completed || commentResult.GenericScheduler == nil || commentResult.GenericScheduler.Completion == nil {
		t.Fatalf("byte 3427 closure result=%+v err=%v", commentResult, commentErr)
	}
	commentCompletion := commentResult.GenericScheduler.Completion
	if commentCompletion.TargetByte != commentTarget || commentCompletion.ElectionIndex != 672 || commentCompletion.LastToken.Symbol != 92 || commentCompletion.LastToken.StartByte != 3386 || commentCompletion.LastToken.EndByte != commentTarget ||
		len(commentCompletion.Headers) != 2 || commentCompletion.Headers[0].Header.CreationSeq != 140 || commentCompletion.Headers[0].Header.State != 40 || commentCompletion.Headers[0].Header.ByteOffset != commentTarget || !commentCompletion.Headers[0].Header.Shifted ||
		commentCompletion.Headers[1].Header.CreationSeq != 141 || commentCompletion.Headers[1].Header.State != 193 || commentCompletion.Headers[1].Header.ByteOffset != commentTarget || !commentCompletion.Headers[1].Header.Shifted ||
		commentCompletion.Stats.Nodes-braceCompletion.Stats.Nodes != 2 || commentCompletion.Stats.Links-braceCompletion.Stats.Links != 2 || commentCompletion.Stats.Subtrees-braceCompletion.Stats.Subtrees != 1 || commentCompletion.Stats.Children != braceCompletion.Stats.Children ||
		commentCompletion.Work.ExtraShifts-braceCompletion.Work.ExtraShifts != 2 || commentCompletion.Work.ExtraCohorts-braceCompletion.Work.ExtraCohorts != 1 || commentCompletion.Work.Dispatches-braceCompletion.Work.Dispatches != 2 {
		t.Fatalf("comment cohort closure drifted: brace=%+v comment=%+v", braceCompletion, commentCompletion)
	}
	if len(commentResult.GenericScheduler.Rounds) != 1546 {
		t.Fatalf("comment rounds=%d, want 1546", len(commentResult.GenericScheduler.Rounds))
	}
	extraRound := commentResult.GenericScheduler.Rounds[1545]
	if len(extraRound.Before) != 2 || extraRound.Before[0].CreationSeq != 140 || extraRound.Before[0].State != 40 || extraRound.Before[0].ByteOffset != 3383 || extraRound.Before[0].Shifted ||
		extraRound.Before[1].CreationSeq != 141 || extraRound.Before[1].State != 193 || extraRound.Before[1].ByteOffset != 3383 || extraRound.Before[1].Shifted ||
		len(extraRound.Actions) != 2 || extraRound.Actions[0].HeaderIndex != 0 || extraRound.Actions[0].State != 40 || extraRound.Actions[0].Action.Type != gotreesitter.ParseActionShift || extraRound.Actions[0].Action.State != 0 || !extraRound.Actions[0].Action.Extra ||
		extraRound.Actions[1].HeaderIndex != 1 || extraRound.Actions[1].State != 193 || extraRound.Actions[1].Action.Type != gotreesitter.ParseActionShift || extraRound.Actions[1].Action.State != 0 || !extraRound.Actions[1].Action.Extra ||
		len(extraRound.After) != 2 || extraRound.After[0].CreationSeq != 140 || extraRound.After[0].State != 40 || extraRound.After[0].ByteOffset != commentTarget || !extraRound.After[0].Shifted ||
		extraRound.After[1].CreationSeq != 141 || extraRound.After[1].State != 193 || extraRound.After[1].ByteOffset != commentTarget || !extraRound.After[1].Shifted {
		t.Fatalf("comment cohort round drifted: %+v", extraRound)
	}
	target := uint32(3432)
	result, routeErr := gotreesitter.DiagnosticParseParserCorePrefix(
		grammars.GoExternalScanner{}, source,
		gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, GenericStopAtClosedByte: &target},
	)
	if routeErr != nil || !result.Completed || result.GenericScheduler == nil || result.GenericScheduler.Completion == nil {
		t.Fatalf("byte 3432 closure result=%+v err=%v", result, routeErr)
	}
	completion := result.GenericScheduler.Completion
	if completion.TargetByte != target || completion.ElectionIndex != 673 || completion.LastToken.Symbol != 49 || completion.LastToken.Text != "if" || completion.LastToken.StartByte != 3430 || completion.LastToken.EndByte != target ||
		len(completion.Headers) != 1 || completion.Headers[0].Header.CreationSeq != 140 || completion.Headers[0].Header.State != 73 || completion.Headers[0].Header.ByteOffset != target || !completion.Headers[0].Header.Shifted || completion.Headers[0].Header.ExactPaths != 1 ||
		completion.Stats != (core.Stats{Nodes: 1887, Links: 1886, Subtrees: 1655, Children: 1725, CurrentExactPaths: 1}) ||
		completion.Work != (gotreesitter.DiagnosticParserCoreGenericWork{
			Passes: 1642, ActionLookups: 2246, Dispatches: 1701,
			Conflicts: 95, ConflictActions: 198, Forks: 103, ConflictHeads: 211,
			Reductions: 785, OrdinaryShifts: 800, OrdinaryCohorts: 138, ExtraShifts: 21, ExtraCohorts: 1,
			ReductionPauses: 22, NoActionDrops: 103, Elections: 674, Canonicalizations: 1547, PeakHeaders: 4,
		}) || len(result.GenericScheduler.Rounds) != 1547 || len(result.GenericScheduler.NoActionDrops) != 103 || len(result.GenericScheduler.ExternalShifts) != 46 {
		t.Fatalf("post-comment boundary drifted: %+v", completion)
	}
	for run := 1; run < 3; run++ {
		next, err := gotreesitter.DiagnosticParseParserCorePrefix(
			grammars.GoExternalScanner{}, source,
			gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true, GenericStopAtClosedByte: &target},
		)
		if err != nil || next.GenericScheduler == nil || !reflect.DeepEqual(result.GenericScheduler, next.GenericScheduler) {
			t.Fatalf("run %d post-comment receipt drifted: next=%+v err=%v", run, next.GenericScheduler, err)
		}
	}
}

func TestDiagnosticParserCoreGenericSchedulerAcceptsAndMaterializesExactRewrite(t *testing.T) {
	source := parserCoreGenericRewriteSource(t)
	result, routeErr := gotreesitter.DiagnosticParseParserCorePrefix(
		grammars.GoExternalScanner{}, source,
		gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true},
	)
	if routeErr != nil || !result.Completed || !result.Materialized || result.MaterializedTree == nil || result.GenericScheduler == nil || result.GenericScheduler.Acceptance == nil {
		t.Fatalf("source=%d completed=%v materialized=%v boundary=%s state=%d tokens=%d dispatches=%d generic=%+v err=%v", len(source), result.Completed, result.Materialized, result.Boundary, result.State, result.Tokens, result.Dispatches, result.GenericScheduler, routeErr)
	}
	defer result.MaterializedTree.Release()
	acceptance := result.GenericScheduler.Acceptance
	if !result.GenericScheduler.SeedOwned || result.GenericScheduler.StartElectionIndex != -1 || result.GenericScheduler.StartToken != (gotreesitter.Token{}) || len(result.GenericScheduler.StartHeaders) != 1 ||
		result.GenericScheduler.StartCheckpoint != result.GenericScheduler.Elections[0].ScannerBefore || result.GenericScheduler.StartHeaders[0].Header.Checkpoint != result.GenericScheduler.StartCheckpoint.SHA256 ||
		result.Boundary != gotreesitter.DiagnosticParserCoreGenericClosed || result.State != 2 || result.Lookahead.Symbol != 0 || result.Lookahead.StartByte != uint32(len(source)) || result.Lookahead.EndByte != uint32(len(source)) ||
		result.Tokens != 1036 || result.Dispatches != 2682 || result.Dispatches != acceptance.Work.Dispatches || acceptance.ElectionIndex != 1035 || acceptance.Token != result.Lookahead ||
		acceptance.Header.Header.CreationSeq != 234 || acceptance.Header.Header.State != 2 || acceptance.Header.Header.ByteOffset != uint32(len(source)) || acceptance.Header.Header.Shifted || !acceptance.Header.Header.Accepted || acceptance.Header.Header.Paused || acceptance.Header.Header.ExactPaths != 1 ||
		!reflect.DeepEqual(acceptance.Payloads, []uint32{2624}) || acceptance.Score != -30 || acceptance.BranchOrder != 168 || !acceptance.HasBranchOrder ||
		acceptance.Stats != (core.Stats{Nodes: 3005, Links: 3004, Subtrees: 2624, Children: 2769, CurrentExactPaths: 1}) ||
		acceptance.Work != (gotreesitter.DiagnosticParserCoreGenericWork{
			Passes: 2597, ActionLookups: 3545, Dispatches: 2682,
			Conflicts: 160, ConflictActions: 328, Forks: 168, ConflictHeads: 357,
			Reductions: 1256, OrdinaryShifts: 1238, OrdinaryCohorts: 215,
			ExtraShifts: 27, ExtraCohorts: 1, Accepts: 1,
			ReductionPauses: 31, NoActionDrops: 167, Elections: 1036,
			Canonicalizations: 2443, PeakHeaders: 4,
		}) || len(result.GenericScheduler.Elections) != 1036 || len(result.GenericScheduler.Rounds) != 2443 || len(result.GenericScheduler.NoActionDrops) != 167 || len(result.GenericScheduler.ExternalShifts) != 83 {
		t.Fatalf("acceptance receipt drifted: result=%+v acceptance=%+v", result, acceptance)
	}
	inspection, err := benchfixtures.InspectGoTree(result.MaterializedTree.RootNode(), grammars.GoLanguage())
	if err != nil {
		t.Fatal(err)
	}
	root := result.MaterializedTree.RootNode()
	if inspection.SHA256 != "b3f9814b65763642d4eac58b9065018048ea13e6f10d56afb28a0479bf5a68a1" || root.Type(grammars.GoLanguage()) != "source_file" || root.StartByte() != 0 || root.EndByte() != uint32(len(source)) || root.HasError() {
		t.Fatalf("materialized tree drifted: digest=%s root=%s %d..%d has_error=%v", inspection.SHA256, root.Type(grammars.GoLanguage()), root.StartByte(), root.EndByte(), root.HasError())
	}
	runtime := result.MaterializedTree.ParseRuntime()
	if runtime.StopReason != gotreesitter.ParseStopAccepted || runtime.Truncated || runtime.SourceLen != uint32(len(source)) || runtime.ExpectedEOFByte != uint32(len(source)) || runtime.RootEndByte != uint32(len(source)) || !runtime.LastTokenWasEOF {
		t.Fatalf("materialized runtime is not an authenticated full-span accept: %s", runtime.Summary())
	}
	edited := append(append([]byte(nil), source...), '\n')
	fallback, profile, err := gotreesitter.NewParser(grammars.GoLanguage()).ParseIncrementalProfiled(edited, result.MaterializedTree)
	if err != nil {
		t.Fatal(err)
	}
	if fallback == nil {
		t.Fatal("incremental fallback returned no tree")
	}
	defer fallback.Release()
	if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason == "" || fallback == result.MaterializedTree || fallback.RootNode().EndByte() != uint32(len(edited)) {
		t.Fatalf("diagnostic tree entered incremental reuse: profile=%+v same_tree=%v fallback_end=%d", profile, fallback == result.MaterializedTree, fallback.RootNode().EndByte())
	}

	for run := 1; run < 3; run++ {
		next, err := gotreesitter.DiagnosticParseParserCorePrefix(
			grammars.GoExternalScanner{}, source,
			gotreesitter.DiagnosticParserCorePrefixOptions{GenericScheduler: true},
		)
		if next.MaterializedTree != nil {
			defer next.MaterializedTree.Release()
		}
		if err != nil || next.MaterializedTree == nil || !reflect.DeepEqual(result.GenericScheduler, next.GenericScheduler) {
			t.Fatalf("acceptance run %d drifted: next=%+v err=%v", run, next.GenericScheduler, err)
		}
		nextInspection, inspectErr := benchfixtures.InspectGoTree(next.MaterializedTree.RootNode(), grammars.GoLanguage())
		nextRoot := next.MaterializedTree.RootNode()
		nextRuntime := next.MaterializedTree.ParseRuntime()
		if inspectErr != nil || nextInspection.SHA256 != inspection.SHA256 || nextRoot.StartByte() != 0 || nextRoot.EndByte() != uint32(len(source)) || nextRoot.HasError() ||
			nextRuntime.StopReason != gotreesitter.ParseStopAccepted || nextRuntime.Truncated || nextRuntime.SourceLen != uint32(len(source)) || nextRuntime.RootEndByte != uint32(len(source)) || !nextRuntime.LastTokenWasEOF {
			t.Fatalf("acceptance run %d tree/runtime drifted: digest=%s root=%d..%d error=%v runtime=%s inspect_err=%v", run, nextInspection.SHA256, nextRoot.StartByte(), nextRoot.EndByte(), nextRoot.HasError(), nextRuntime.Summary(), inspectErr)
		}
	}
}
