//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestAdmissionRecoverEOFSingletonAdmissionAndPriorityFree(t *testing.T) {
	lang := grammars.YamlLanguage()
	tree, ok, reason := gts.TryCompactFullParseRouteForTest(gts.NewParser(lang), []byte("[\n"))
	if tree != nil {
		defer tree.Release()
	}
	if !ok || reason != "" {
		t.Fatalf("singleton recover_eof route ok=%t reason=%q", ok, reason)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("singleton recover_eof route returned no tree")
	}
	root := tree.RootNode()
	if !root.IsError() || !root.HasError() || root.IsExtra() || root.IsMissing() ||
		root.StartByte() != 0 || root.EndByte() != 1 || root.ChildCount() != 1 {
		t.Fatalf("singleton recover_eof root type=%s error=%t/%t extra=%t missing=%t span=%d..%d children=%d",
			root.Type(lang), root.IsError(), root.HasError(), root.IsExtra(), root.IsMissing(),
			root.StartByte(), root.EndByte(), root.ChildCount())
	}
	child := root.Child(0)
	if child == nil || child.StartByte() != 0 || child.EndByte() != 1 || child.Type(lang) != "[" {
		t.Fatalf("singleton recover_eof child=%v, want [ at 0..1", child)
	}
}

func TestAdmissionRecoverEOFLiveTelemetry(t *testing.T) {
	lang := grammars.YamlLanguage()
	tree, telemetry, ok, reason := gts.TryCompactRecoverEOFAcceptTelemetryForTest(
		gts.NewParser(lang), []byte("[\n"),
	)
	if tree != nil {
		defer tree.Release()
	}
	if !ok || reason != "" || tree == nil {
		t.Fatalf("live recover_eof route ok=%t reason=%q tree=%v", ok, reason, tree != nil)
	}
	if telemetry.RecoverEOFAccepts != 1 {
		t.Fatalf("RecoverEOFAccepts=%d, want exactly one", telemetry.RecoverEOFAccepts)
	}
	runtime := telemetry.Runtime
	if runtime.StopReason != gts.ParseStopAccepted || !runtime.LastTokenWasEOF ||
		runtime.LastTokenEndByte != 2 || runtime.ExpectedEOFByte != 2 || runtime.RootEndByte != 1 {
		t.Fatalf("recover_eof runtime=%+v, want accepted EOF at 2 with root end 1", runtime)
	}
}

func TestAdmissionRecoverEOFArtifactReceiptTamperingDeclines(t *testing.T) {
	lang := grammars.YamlLanguage()
	want := lang.CompactRecoverEOFArtifactReceipt
	tamper := []struct {
		name string
		edit func(*gts.CompactRecoverEOFArtifactReceipt)
	}{
		{name: "blob", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.BlobSHA256[0] ^= 1 }},
		{name: "terminal", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.TerminalSymbol++ }},
		{name: "state", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.EOFState++ }},
		{name: "offset", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.EOFByteOffset++ }},
		{name: "passes", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.Passes++ }},
		{name: "elections", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.Elections++ }},
		{name: "lookups", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.ActionLookups++ }},
		{name: "dispatches", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.Dispatches++ }},
		{name: "ordinary_shifts", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.OrdinaryShifts++ }},
		{name: "ordinary_cohorts", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.OrdinaryCohorts++ }},
		{name: "extra_shifts", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.ExtraShifts++ }},
		{name: "extra_cohorts", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.ExtraCohorts++ }},
		{name: "reductions", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.Reductions++ }},
		{name: "conflicts", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.Conflicts++ }},
		{name: "conflict_actions", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.ConflictActions++ }},
		{name: "forks", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.Forks++ }},
		{name: "repetition_folds", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.RepetitionFolds++ }},
		{name: "recovery_work", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.RecoveryWork++ }},
		{name: "no_action_drops", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.NoActionDrops++ }},
		{name: "reduction_pauses", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.ReductionPauses++ }},
		{name: "accepts", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.Accepts++ }},
		{name: "canonicalizations", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.Canonicalizations++ }},
		{name: "peak_headers", edit: func(receipt *gts.CompactRecoverEOFArtifactReceipt) { receipt.PeakHeaders++ }},
	}
	for _, tt := range tamper {
		t.Run(tt.name, func(t *testing.T) {
			candidate := want
			tt.edit(&candidate)
			lang.CompactRecoverEOFArtifactReceipt = candidate
			defer func() { lang.CompactRecoverEOFArtifactReceipt = want }()
			tree, telemetry, ok, reason := gts.TryCompactRecoverEOFAcceptTelemetryForTest(
				gts.NewParser(lang), []byte("[\n"),
			)
			if tree != nil {
				tree.Release()
			}
			if ok || reason == "" || telemetry.RecoverEOFAccepts != 0 {
				t.Fatalf("tampered receipt routed: ok=%t reason=%q telemetry=%+v", ok, reason, telemetry)
			}
		})
	}
}

func TestAdmissionRecoverEOFPayloadCountFallsBackForUnfinishedYAML(t *testing.T) {
	lang := grammars.YamlLanguage()
	for _, source := range []string{"a: [1\n", "\"a\": [\"x\"\n"} {
		t.Run(source, func(t *testing.T) {
			tree, ok, reason := gts.TryCompactFullParseRouteForTest(gts.NewParser(lang), []byte(source))
			if tree != nil {
				tree.Release()
			}
			if ok || reason == "" {
				t.Fatalf("unfinished YAML route ok=%t reason=%q, want fail-closed fallback", ok, reason)
			}

			production := gts.NewParser(lang)
			production.SetAdmissionCandidateRoute(false)
			want, err := production.Parse([]byte(source))
			if err != nil {
				t.Fatal(err)
			}
			defer want.Release()

			candidate := gts.NewParser(lang)
			candidate.SetAdmissionCandidateRoute(true)
			got, err := candidate.Parse([]byte(source))
			if err != nil {
				t.Fatal(err)
			}
			defer got.Release()
			if got.RootNode().SExpr(lang) != want.RootNode().SExpr(lang) {
				t.Fatalf("unfinished YAML fallback tree=%s want=%s", got.RootNode().SExpr(lang), want.RootNode().SExpr(lang))
			}
		})
	}
}

func TestAdmissionRecoverEOFIncrementalMarkerFallsBackForUncertifiedScanner(t *testing.T) {
	lang := grammars.YamlLanguage()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	oldSource := []byte("[\n")
	oldTree, err := parser.Parse(oldSource)
	if err != nil {
		t.Fatal(err)
	}
	if oldTree == nil || oldTree.RootNode() == nil || !oldTree.RootNode().IsError() {
		if oldTree != nil {
			oldTree.Release()
		}
		t.Fatal("recover_eof source did not produce its ERROR root")
	}
	defer oldTree.Release()
	oldTree.Edit(gts.InputEdit{
		StartByte:   0,
		OldEndByte:  1,
		NewEndByte:  1,
		StartPoint:  gts.Point{},
		OldEndPoint: gts.Point{Column: 1},
		NewEndPoint: gts.Point{Column: 1},
	})
	newSource := []byte("]\n")
	got, profile, err := parser.ParseIncrementalProfiled(newSource, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.RootNode() == nil {
		if got != nil {
			got.Release()
		}
		t.Fatal("incremental marker fallback returned no tree")
	}
	defer got.Release()
	freshParser := gts.NewParser(lang)
	freshParser.SetAdmissionCandidateRoute(false)
	want, err := freshParser.Parse(newSource)
	if err != nil {
		t.Fatal(err)
	}
	defer want.Release()
	gotRoot, wantRoot := got.RootNode(), want.RootNode()
	if gotRoot.SExpr(lang) != wantRoot.SExpr(lang) || gotRoot.Range() != wantRoot.Range() {
		t.Fatalf("incremental marker fallback tree=%s range=%+v want fresh=%s range=%+v",
			gotRoot.SExpr(lang), gotRoot.Range(), wantRoot.SExpr(lang), wantRoot.Range())
	}
	requireIncrementalDeepTreeMatchesFresh(t, got, want, lang)
	if profile.ReuseUnsupportedReason == "old tree carried a compact recover_eof EOF runtime" {
		t.Fatalf("recover_eof marker reported the obsolete permanent bar: %+v", profile)
	}
	if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "external_scanner_unsupported" ||
		profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
		t.Fatalf("recover_eof marker did not fail closed for the uncertified scanner: %+v", profile)
	}
	runtime := got.ParseRuntime()
	if runtime.StopReason != gts.ParseStopAccepted || !runtime.LastTokenWasEOF ||
		runtime.SourceLen != uint32(len(newSource)) || runtime.LastTokenEndByte != uint32(len(newSource)) ||
		runtime.ExpectedEOFByte != uint32(len(newSource)) || runtime.RootEndByte != uint32(len(newSource)) ||
		runtime.RootEndByte != got.RootNode().EndByte() ||
		runtime.IncrementalOldTreeReuseRoute {
		t.Fatalf("recover_eof fallback runtime=%+v, want fresh EOF at %d", runtime, len(newSource))
	}
}

func TestAdmissionRecoverEOFUnfinishedYAMLIncrementalFallsBackCorrectly(t *testing.T) {
	lang := grammars.YamlLanguage()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	oldSource := []byte("\"a\": [\"x\"\n")
	oldTree, err := parser.Parse(oldSource)
	if err != nil {
		t.Fatal(err)
	}
	defer oldTree.Release()
	oldTree.Edit(gts.InputEdit{
		StartByte:   7,
		OldEndByte:  8,
		NewEndByte:  8,
		StartPoint:  gts.Point{Column: 7},
		OldEndPoint: gts.Point{Column: 8},
		NewEndPoint: gts.Point{Column: 8},
	})
	newSource := []byte("\"a\": [\"y\"\n")
	got, _, err := parser.ParseIncrementalProfiled(newSource, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Release()
	freshParser := gts.NewParser(lang)
	freshParser.SetAdmissionCandidateRoute(false)
	want, err := freshParser.Parse(newSource)
	if err != nil {
		t.Fatal(err)
	}
	defer want.Release()
	if got.RootNode().SExpr(lang) != want.RootNode().SExpr(lang) {
		t.Fatalf("unfinished YAML incremental tree=%s want fresh=%s", got.RootNode().SExpr(lang), want.RootNode().SExpr(lang))
	}
}
