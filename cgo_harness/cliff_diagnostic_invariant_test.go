//go:build linux && cgo && treesitter_c_parity && gts_diag && gts_diag_converged_split_no_action_bypass

package cgoharness

import (
	"fmt"
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestCliffDiagnosticIncrementalInvariant(t *testing.T) {
	if os.Getenv("GTS_ADMISSION_CANDIDATE") != "1" {
		t.Skip("requires the candidate route")
	}
	selected := os.Getenv("GTS_CLIFF_LANGUAGE")
	var fixture *cliffFixture
	for index := range cliffFixtures {
		if cliffFixtures[index].Name == selected {
			fixture = &cliffFixtures[index]
			break
		}
	}
	if fixture == nil {
		t.Fatalf("unknown GTS_CLIFF_LANGUAGE %q", selected)
	}
	if fixture.Name == "html" {
		t.Skip("HTML recovery is outside the no-action proof bypass")
	}

	entry := grammars.DetectLanguageByName(fixture.Grammar)
	if entry == nil || entry.Language() == nil {
		t.Fatalf("Go grammar %q unavailable", fixture.Grammar)
	}
	language := entry.Language()
	parser := gts.NewParser(language)
	parser.SetAdmissionCandidateRoute(true)
	legacyParser := gts.NewParser(language)
	legacyParser.SetAdmissionCandidateRoute(false)
	source := loadCliffSource(t, *fixture)
	oldTree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("initial parse: %v", err)
	}
	defer func() {
		if oldTree != nil {
			oldTree.Release()
		}
	}()
	requireCliffInvariantTree(t, oldTree, source, "initial")
	legacyOldTree, err := legacyParser.Parse(source)
	if err != nil {
		t.Fatalf("initial legacy parse: %v", err)
	}
	defer func() {
		if legacyOldTree != nil {
			legacyOldTree.Release()
		}
	}()

	var diagnosticCounters, legacyCounters cliffEditCounters
	current := source
	for index, step := range benchfixtures.EditingSession(source) {
		oldTree.Edit(step.Edit)
		incremental, profile, err := parser.ParseIncrementalProfiled(step.Source, oldTree)
		if err != nil {
			t.Fatalf("step %d incremental parse: %v", index+1, err)
		}
		diagnosticCounters.add(profile, incremental)
		fresh, err := parser.Parse(step.Source)
		if err != nil {
			incremental.Release()
			t.Fatalf("step %d fresh parse: %v", index+1, err)
		}
		requireCliffInvariantTree(t, incremental, step.Source, fmt.Sprintf("step %d incremental", index+1))
		requireCliffInvariantTree(t, fresh, step.Source, fmt.Sprintf("step %d fresh", index+1))
		incrementalDigest, err := benchfixtures.InspectGoTree(incremental.RootNode(), language)
		if err != nil {
			fresh.Release()
			incremental.Release()
			t.Fatal(err)
		}
		freshDigest, err := benchfixtures.InspectGoTree(fresh.RootNode(), language)
		if err != nil {
			fresh.Release()
			incremental.Release()
			t.Fatal(err)
		}
		if incrementalDigest.SHA256 != freshDigest.SHA256 {
			fresh.Release()
			incremental.Release()
			t.Fatalf("step %d incremental digest %s != fresh digest %s", index+1, incrementalDigest.SHA256, freshDigest.SHA256)
		}
		legacyOldTree.Edit(step.Edit)
		legacyIncremental, legacyProfile, err := legacyParser.ParseIncrementalProfiled(step.Source, legacyOldTree)
		if err != nil {
			fresh.Release()
			incremental.Release()
			t.Fatalf("step %d legacy incremental parse: %v", index+1, err)
		}
		legacyCounters.add(legacyProfile, legacyIncremental)
		legacyOldTree.Release()
		legacyOldTree = legacyIncremental
		oldTree.Release()
		fresh.Release()
		oldTree = incremental
		current = step.Source
	}

	allocations := testing.AllocsPerRun(100, func() {
		noEdit, err := parser.ParseIncremental(current, oldTree)
		if err != nil {
			panic(err)
		}
		noEdit.Release()
	})
	if allocations != 0 {
		t.Fatalf("no-edit reparse allocated %.2f times per run", allocations)
	}
	t.Logf("edit-session counters: legacy=%+v diagnostic=%+v", legacyCounters, diagnosticCounters)
	if diagnosticCounters.TokensConsumed > legacyCounters.TokensConsumed {
		t.Fatalf("diagnostic tokens %d exceed legacy %d", diagnosticCounters.TokensConsumed, legacyCounters.TokensConsumed)
	}
	if diagnosticCounters.NewNodesAllocated > legacyCounters.NewNodesAllocated {
		t.Fatalf("diagnostic new nodes %d exceed legacy %d", diagnosticCounters.NewNodesAllocated, legacyCounters.NewNodesAllocated)
	}
	if diagnosticCounters.MaxLiveVersions > legacyCounters.MaxLiveVersions {
		t.Fatalf("diagnostic max live versions %d exceed legacy %d", diagnosticCounters.MaxLiveVersions, legacyCounters.MaxLiveVersions)
	}
	if diagnosticCounters.ReusedBytes < legacyCounters.ReusedBytes {
		t.Fatalf("diagnostic reused bytes %d below legacy %d", diagnosticCounters.ReusedBytes, legacyCounters.ReusedBytes)
	}
	if diagnosticCounters.BlockSpliceSteps < legacyCounters.BlockSpliceSteps {
		t.Fatalf("diagnostic block splices %d below legacy %d", diagnosticCounters.BlockSpliceSteps, legacyCounters.BlockSpliceSteps)
	}
}

type cliffEditCounters struct {
	TokensConsumed    uint64
	NewNodesAllocated uint64
	MaxLiveVersions   uint64
	ReusedBytes       uint64
	BlockSpliceSteps  uint64
}

func (c *cliffEditCounters) add(profile gts.IncrementalParseProfile, tree *gts.Tree) {
	if c == nil || tree == nil {
		return
	}
	c.TokensConsumed += profile.TokensConsumed
	c.NewNodesAllocated += profile.NewNodesAllocated
	c.ReusedBytes += profile.ReusedBytes
	c.BlockSpliceSteps += profile.BlockSpliceSteps
	maxLive := profile.MaxStacksSeen
	if compactPeak := tree.ParseRuntime().CompactPeakHeaders; uint64(compactPeak) > maxLive {
		maxLive = uint64(compactPeak)
	}
	if maxLive > c.MaxLiveVersions {
		c.MaxLiveVersions = maxLive
	}
}

func requireCliffInvariantTree(t *testing.T, tree *gts.Tree, source []byte, label string) {
	t.Helper()
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("%s returned no root", label)
	}
	root := tree.RootNode()
	if root.IsError() && !root.HasError() {
		t.Fatalf("%s ERROR root reports HasError false", label)
	}
	if root.StartByte() != 0 || root.EndByte() != uint32(len(source)) {
		stop := tree.ParseRuntime().StopReason
		if stop == gts.ParseStopAccepted || stop == gts.ParseStopNone {
			t.Fatalf("%s root covers %d..%d of %d bytes with stop reason %s", label, root.StartByte(), root.EndByte(), len(source), stop)
		}
	}
}
