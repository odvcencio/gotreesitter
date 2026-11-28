//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestAdmissionRecoverEOFYAMLIsServedByTheCertifiedCore: the shipped YAML tables
// (the yaml-core recipe) carry no compact recover_eof grant, so the compact route
// declines "[\n" without a recover_eof acceptance. The default route is the
// certified core. It returns pinned C's root, an ERROR over "[" at 0..1, and
// reports the newline after that root as the accepted prefix.
func TestAdmissionRecoverEOFYAMLIsServedByTheCertifiedCore(t *testing.T) {
	lang := grammars.YamlLanguage()
	source := []byte("[\n")
	tree, ok, reason := gts.TryCompactFullParseRouteForTest(gts.NewParser(lang), source)
	if tree != nil {
		tree.Release()
	}
	if ok || reason == "" {
		t.Fatalf("compact route ok=%t reason=%q, want a decline", ok, reason)
	}
	tree, telemetry, ok, reason := gts.TryCompactRecoverEOFAcceptTelemetryForTest(gts.NewParser(lang), source)
	if tree != nil {
		tree.Release()
	}
	if ok || reason == "" || telemetry.RecoverEOFAccepts != 0 {
		t.Fatalf("compact recover_eof route ok=%t reason=%q accepts=%d, want a decline without an acceptance", ok, reason, telemetry.RecoverEOFAccepts)
	}

	got, err := gts.NewParser(lang).Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Release()
	root := got.RootNode()
	if root == nil || !root.IsError() || !root.HasError() || root.IsExtra() || root.IsMissing() ||
		root.StartByte() != 0 || root.EndByte() != 1 || root.ChildCount() != 1 {
		t.Fatalf("default root=%v, want pinned C's non-extra ERROR 0..1 with one child", root)
	}
	child := root.Child(0)
	if child == nil || child.StartByte() != 0 || child.EndByte() != 1 || child.Type(lang) != "[" {
		t.Fatalf("default root child=%v, want [ at 0..1", child)
	}
	if !got.ParseStoppedEarly() || got.ParseRuntime().StopReason != gts.ParseStopAcceptedPrefix {
		t.Fatalf("default runtime=%s, want the accepted prefix before the trailing newline", got.ParseRuntime().Summary())
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
			// The certified core serves YAML on the candidate route and returns
			// pinned C's tree; the telemetry keeps this request off the core, so the
			// test still compares the compact route's fallback with the present engine.
			candidate.SetCompactCertificationTelemetry(true)
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

func TestAdmissionRecoverEOFUnfinishedYAMLIncrementalFallsBackCorrectly(t *testing.T) {
	lang := grammars.YamlLanguage()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	// Keep the compact route's fallback under test: the certified core would
	// otherwise serve both parses with pinned C's trees.
	parser.SetCompactCertificationTelemetry(true)
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

// TestAdmissionRecoverEOFTieKeepsFirstFinishedTreeAsIncumbent pins the
// nightly-fuzz witness Go "," (FuzzAdmissionRouteEquality seed
// 0d6dbe8c80002ec6). Compact recovery finishes two trees at the same error
// cost: the recover_eof ERROR root first, then source_file over an extra
// ERROR after the summary recovery returns to the start state. C's
// ts_parser__select_tree keeps the earlier finished tree as the incumbent and
// takes the later tree on an equal positive cost, so the locked C oracle
// publishes (source_file (ERROR)). The compact route must fold its finished
// trees in that acceptance order, not in header order.
func TestAdmissionRecoverEOFTieKeepsFirstFinishedTreeAsIncumbent(t *testing.T) {
	lang := frozenCompactGoLanguage(t)
	tree, telemetry, ok, reason := gts.TryCompactRecoverEOFAcceptTelemetryForTest(
		gts.NewParser(lang), []byte(","),
	)
	if tree != nil {
		defer tree.Release()
	}
	if !ok || reason != "" || tree == nil || tree.RootNode() == nil {
		t.Fatalf("compact route ok=%t reason=%q tree=%t, want the compact route to serve the input", ok, reason, tree != nil)
	}
	if telemetry.RecoverEOFAccepts != 1 {
		t.Fatalf("RecoverEOFAccepts=%d, want exactly one", telemetry.RecoverEOFAccepts)
	}
	root := tree.RootNode()
	if root.Type(lang) != "source_file" || root.IsError() || !root.HasError() ||
		root.StartByte() != 0 || root.EndByte() != 1 || root.ChildCount() != 1 {
		t.Fatalf("root=%s error=%t has_error=%t span=%d..%d children=%d, want source_file[0:1] with one child",
			root.Type(lang), root.IsError(), root.HasError(), root.StartByte(), root.EndByte(), root.ChildCount())
	}
	child := root.Child(0)
	if !child.IsError() || !child.IsExtra() || child.StartByte() != 0 || child.EndByte() != 1 {
		t.Fatalf("child=%s error=%t extra=%t span=%d..%d, want an extra ERROR over 0..1",
			child.Type(lang), child.IsError(), child.IsExtra(), child.StartByte(), child.EndByte())
	}
}
