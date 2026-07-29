package grammars_test

import (
	"bytes"
	"testing"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestIssue454PHPWholeTreeFallbackMatchesFresh(t *testing.T) {
	lang := grammars.PhpLanguage()
	source := benchfixtures.Issue454PHPSource()
	if got := len(source); got != benchfixtures.Issue454PHPFixtureBytes {
		t.Fatalf("fixture bytes = %d, want %d", got, benchfixtures.Issue454PHPFixtureBytes)
	}

	baseStart := time.Now()
	baseShape := issue454PHPFreshShape(t, lang, source)
	baseDuration := time.Since(baseStart)
	if baseShape.hasError {
		t.Fatal("base fresh parse has an error")
	}
	for run := 2; run <= 3; run++ {
		got := issue454PHPFreshShape(t, lang, source)
		if got != baseShape {
			t.Fatalf("fresh base parse %d changed: got %+v, want %+v", run, got, baseShape)
		}
	}

	site := bytes.Index(source, []byte("$x0"))
	if site < 0 {
		t.Fatal("PHP edit marker is absent")
	}
	site++
	edited := make([]byte, 0, len(source)-1)
	edited = append(edited, source[:site]...)
	edited = append(edited, source[site+1:]...)
	if got := edited[site-1 : site+1]; !bytes.Equal(got, []byte("$0")) {
		t.Fatalf("edited marker = %q, want %q", got, "$0")
	}

	freshStart := time.Now()
	want := issue454PHPFreshShape(t, lang, edited)
	freshDuration := time.Since(freshStart)
	if !want.hasError {
		t.Fatal("edited fresh parse did not enter recovery")
	}
	for run := 2; run <= 3; run++ {
		got := issue454PHPFreshShape(t, lang, edited)
		if got != want {
			t.Fatalf("fresh edited parse %d changed: got %+v, want %+v", run, got, want)
		}
	}

	oldParser := gotreesitter.NewParser(lang)
	oldParser.SetAdmissionCandidateRoute(false)
	oldTree, err := oldParser.Parse(source)
	if err != nil {
		t.Fatalf("old Parse: %v", err)
	}
	defer oldTree.Release()
	point := issue454PointAt(source, site)
	editStart := time.Now()
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte:   uint32(site),
		OldEndByte:  uint32(site + 1),
		NewEndByte:  uint32(site),
		StartPoint:  point,
		OldEndPoint: gotreesitter.Point{Row: point.Row, Column: point.Column + 1},
		NewEndPoint: point,
	})
	editDuration := time.Since(editStart)

	incrementalParser := gotreesitter.NewParser(lang)
	incrementalParser.SetAdmissionCandidateRoute(false)
	incrementalStart := time.Now()
	incremental, profile, err := incrementalParser.ParseIncrementalProfiled(edited, oldTree)
	incrementalDuration := time.Since(incrementalStart)
	if err != nil {
		t.Fatalf("ParseIncrementalProfiled: %v", err)
	}
	defer incremental.Release()
	if incremental.ParseStoppedEarly() {
		t.Fatalf("incremental parse stopped early: %s", incremental.ParseRuntime().Summary())
	}

	if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "external_scanner_unsupported" {
		t.Fatalf("PHP scanner fallback profile = %+v", profile)
	}
	if profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
		t.Fatalf("PHP scanner fallback reused old syntax: %+v", profile)
	}
	got := issue454PHPTreeShape(t, lang, incremental)
	if got != want {
		t.Fatalf("incremental shape = %+v, fresh shape = %+v", got, want)
	}
	t.Logf(
		"base_fresh=%s edited_fresh=%s edit=%s incremental=%s ratio_to_base=%.2fx ratio_to_edited_fresh=%.2fx reuse_cursor=%s reparse=%s tokens=%d nodes=%d max_stacks=%d arena=%d scratch=%d parser_loop=%s token_next=%s dispatch=%s merge=%s selection=%s compatibility=%s",
		baseDuration,
		freshDuration,
		editDuration,
		incrementalDuration,
		float64(incrementalDuration)/float64(baseDuration),
		float64(incrementalDuration)/float64(freshDuration),
		time.Duration(profile.ReuseCursorNanos),
		time.Duration(profile.ReparseNanos),
		profile.TokensConsumed,
		profile.NewNodesAllocated,
		profile.MaxStacksSeen,
		profile.ArenaBytesAllocated,
		profile.ScratchBytesAllocated,
		time.Duration(profile.ParserLoopNanos),
		time.Duration(profile.TokenNextNanos),
		time.Duration(profile.ActionDispatchNanos),
		time.Duration(profile.GLRMergeNanos),
		time.Duration(profile.ResultSelectionNanos),
		time.Duration(profile.ResultCompatibilityNanos),
	)
}

type issue454PHPShape struct {
	digest   string
	nodes    uint64
	rootType string
	hasError bool
	endByte  uint32
}

func issue454PHPFreshShape(t *testing.T, lang *gotreesitter.Language, source []byte) issue454PHPShape {
	t.Helper()
	parser := gotreesitter.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("fresh Parse: %v", err)
	}
	defer tree.Release()
	return issue454PHPTreeShape(t, lang, tree)
}

func issue454PHPTreeShape(
	t *testing.T,
	lang *gotreesitter.Language,
	tree *gotreesitter.Tree,
) issue454PHPShape {
	t.Helper()
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned no root")
	}
	if tree.ParseStoppedEarly() {
		t.Fatalf("parse stopped early: %s", tree.ParseRuntime().Summary())
	}
	root := tree.RootNode()
	if got, want := root.EndByte(), uint32(len(tree.Source())); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}
	inspection, err := benchfixtures.InspectGoTree(root, lang)
	if err != nil {
		t.Fatalf("inspect tree: %v", err)
	}
	var nodes uint64
	for _, count := range inspection.NodeKinds {
		nodes += count
	}
	return issue454PHPShape{
		digest:   inspection.SHA256,
		nodes:    nodes,
		rootType: root.Type(lang),
		hasError: root.HasError(),
		endByte:  root.EndByte(),
	}
}
