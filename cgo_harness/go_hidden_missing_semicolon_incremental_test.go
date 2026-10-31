//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Deleting the separator preserves both declarations but requires a hidden
// MISSING separator. Restoring it must clear that error on the next edit.
func TestGoHiddenMissingSemicolonEditsMatchLockedC(t *testing.T) {
	clean := []byte("package p\nfunc f(){}\nfunc g(){}\n")
	malformed := []byte("package p\nfunc f(){}func g(){}\n")
	if len(clean) != 32 || fmt.Sprintf("%x", sha256.Sum256(clean)) != "6e8d1ce19f93bd86ca4ba8277a5632f656d3b6170d7e1a105ea410dcc3b719ce" ||
		len(malformed) != 31 || fmt.Sprintf("%x", sha256.Sum256(malformed)) != "5a5c21bf5909a04645aabc144879f4e52b143681dbb2eabfa70d97deddb8b942" {
		t.Fatal("small joined/separated witnesses changed")
	}
	site := bytes.Index(clean, []byte("}\nfunc")) + 1
	if site != 20 {
		t.Fatalf("separator byte=%d, want 20", site)
	}
	language := grammars.GoLanguage()
	if language == nil {
		t.Fatal("Go grammar unavailable")
	}
	cLanguage := loadCanonicalGoCLanguage(t)
	for _, route := range []struct {
		name      string
		candidate bool
	}{{"production", false}, {"candidate", true}} {
		t.Run(route.name, func(t *testing.T) {
			gp := gts.NewParser(language)
			gp.SetAdmissionCandidateRoute(route.candidate)
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			source := clean
			beforeRouted, beforeFallback := gts.AdmissionCandidateCounters()
			current, err := gp.Parse(source)
			if err != nil || current == nil {
				t.Fatalf("initial Go parse: %v", err)
			}
			defer func() {
				if current != nil {
					current.Release()
				}
			}()
			currentC := cp.Parse(source, nil)
			if currentC == nil {
				t.Fatal("initial C parse returned no tree")
			}
			defer func() {
				if currentC != nil {
					currentC.Close()
				}
			}()
			logGoHiddenMissingRoute(t, "initial", current, beforeRouted, beforeFallback)

			check := func(label string, wantError bool) {
				t.Helper()
				requireGoHiddenMissingAccepted(t, label+" incremental Go", current, source, wantError)
				freshParser := gts.NewParser(language)
				freshParser.SetAdmissionCandidateRoute(route.candidate)
				routed, fallback := gts.AdmissionCandidateCounters()
				fresh, err := freshParser.Parse(source)
				if err != nil || fresh == nil {
					t.Fatalf("%s fresh Go: %v", label, err)
				}
				defer fresh.Release()
				logGoHiddenMissingRoute(t, label+" fresh", fresh, routed, fallback)
				requireGoHiddenMissingAccepted(t, label+" fresh Go", fresh, source, wantError)
				freshCParser := sitter.NewParser()
				defer freshCParser.Close()
				if err := freshCParser.SetLanguage(cLanguage); err != nil {
					t.Fatal(err)
				}
				freshC := freshCParser.Parse(source, nil)
				if freshC == nil || freshC.RootNode() == nil {
					t.Fatal("fresh C returned no tree")
				}
				defer freshC.Close()
				if freshC.RootNode().HasError() != wantError || freshC.RootNode().StartByte() != 0 || freshC.RootNode().EndByte() != uint(len(source)) {
					t.Fatalf("%s fresh C lost expected coverage/error state", label)
				}
				assertLockedCTreeExactWithErrors(t, label+" fresh Go/fresh C", fresh, language, freshC)
				assertLockedCTreeExactWithErrors(t, label+" incremental Go/fresh C", current, language, freshC)
				assertLockedCTreeExactWithErrors(t, label+" incremental Go/incremental C", current, language, currentC)

				var noEditErr error
				noEditNil := false
				allocations := testing.AllocsPerRun(5, func() {
					same, err := gp.ParseIncremental(source, current)
					if err != nil {
						noEditErr = err
						return
					}
					if same == nil {
						noEditNil = true
						return
					}
					same.Release()
				})
				if noEditErr != nil || noEditNil || allocations != 0 {
					t.Fatalf("%s no-edit: allocations=%g nil=%t error=%v", label, allocations, noEditNil, noEditErr)
				}
				requireGoHiddenMissingAccepted(t, label+" after no-edit", current, source, wantError)
				t.Logf("state=%s bytes=%d source_sha256=%x HasError=%t no_edit_allocs=%g", label, len(source), sha256.Sum256(source), wantError, allocations)
			}
			check("initial-clean", false)
			for cycle := 0; cycle < 2; cycle++ {
				for _, deleted := range []bool{true, false} {
					after := clean
					oldEnd, newEnd := site, site+1
					label := fmt.Sprintf("cycle-%d/restore-newline", cycle+1)
					if deleted {
						after = malformed
						oldEnd, newEnd = site+1, site
						label = fmt.Sprintf("cycle-%d/delete-newline", cycle+1)
					}
					edit := gts.InputEdit{
						StartByte: uint32(site), OldEndByte: uint32(oldEnd), NewEndByte: uint32(newEnd),
						StartPoint: pointAtOffset(source, site), OldEndPoint: pointAtOffset(source, oldEnd), NewEndPoint: pointAtOffset(after, newEnd),
					}
					beforeRouted, beforeFallback = gts.AdmissionCandidateCounters()
					previous := current
					previous.Edit(edit)
					current, err = gp.ParseIncremental(after, previous)
					previous.Release()
					if err != nil || current == nil {
						t.Fatalf("%s incremental Go: %v", label, err)
					}
					logGoHiddenMissingRoute(t, label, current, beforeRouted, beforeFallback)
					previousC := currentC
					cEdit := realCorpusCInputEdit(edit)
					previousC.Edit(&cEdit)
					currentC = cp.Parse(after, previousC)
					previousC.Close()
					if currentC == nil {
						t.Fatalf("%s incremental C returned no tree", label)
					}
					source = after
					check(label, deleted)
				}
			}
		})
	}
}

func requireGoHiddenMissingAccepted(t *testing.T, label string, tree *gts.Tree, source []byte, wantError bool) {
	t.Helper()
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("%s returned no root", label)
	}
	runtime := tree.ParseRuntime()
	root := tree.RootNode()
	if tree.ParseStoppedEarly() || runtime.StopReason != gts.ParseStopAccepted || root.StartByte() != 0 || root.EndByte() != uint32(len(source)) {
		t.Fatalf("%s did not accept the whole source: %s", label, runtime.Summary())
	}
	if root.HasError() != wantError || (root.IsError() && !root.HasError()) {
		t.Fatalf("%s HasError=%t IsError=%t, want HasError=%t", label, root.HasError(), root.IsError(), wantError)
	}
}

func logGoHiddenMissingRoute(t *testing.T, label string, tree *gts.Tree, beforeRouted, beforeFallback uint64) {
	t.Helper()
	routed, fallback := gts.AdmissionCandidateCounters()
	decline := ""
	if fallback > beforeFallback {
		decline = gts.AdmissionCandidateLastFallbackReason()
	}
	runtime := tree.ParseRuntime()
	t.Logf("state=%s actual_compact=%d actual_fallback=%d decline=%q old_tree_reuse=%t compact_reuse=%t compact_full_recovery=%t incremental_fallback=%q",
		label, routed-beforeRouted, fallback-beforeFallback, decline, runtime.IncrementalOldTreeReuseRoute,
		runtime.CompactIncrementalReuseRoute, runtime.CompactIncrementalFullRecoveryRoute, runtime.CompactIncrementalFallbackReason)
}
