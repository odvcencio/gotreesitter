//go:build cgo && treesitter_c_parity && gts_parsercorephase0

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestGoCompactRecoveryVersionTurnsLockedC(t *testing.T) {
	// Require the actual artifact profile without adding diagnostic grants.
	language := *grammars.GoLanguage()
	if !language.CompactOwnedEOFRecoveryCertified {
		t.Fatal("Go artifact has no owned EOF recovery profile")
	}
	cLanguage, err := ParityCLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct{ name, source string }{
		{"without_package", "func f(){}\nfunc g(){}\n"},
		{"package_alias", "package p\nfunc f(){}\nfunc g(){}\n"},
		{"parameters", "package p\nfunc f(x int) int { return x + 1 }\nfunc g(){}\n"},
		{"loop", "package p\nfunc f(){for i:=0;i<2;i++{_ = i}}\nfunc g(){}\n"},
		{"strings", "package p\nfunc f(){_ = \"a+b\"; _ = `raw`}\nfunc g(){}\n"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			original := fixture.source
			edited := original[:len(original)-1] + "+"
			edit := gts.InputEdit{StartByte: uint32(len(original) - 1), OldEndByte: uint32(len(original)), NewEndByte: uint32(len(edited)), StartPoint: pointAtOffset([]byte(original), len(original)-1), OldEndPoint: pointAtOffset([]byte(original), len(original)), NewEndPoint: pointAtOffset([]byte(edited), len(edited))}
			for _, incremental := range []bool{false, true} {
				name := "fresh"
				if incremental {
					name = "incremental"
				}
				t.Run(name, func(t *testing.T) {
					parser := gts.NewParser(&language)
					parser.SetAdmissionCandidateRoute(true)
					cp := sitter.NewParser()
					defer cp.Close()
					if err := cp.SetLanguage(cLanguage); err != nil {
						t.Fatal(err)
					}
					var old *gts.Tree
					var cOld *sitter.Tree
					if incremental {
						old, err = parser.Parse([]byte(original))
						if err != nil {
							t.Fatal(err)
						}
						if old.RootNode().Child(0).Parent() != old.RootNode() {
							t.Fatal("old compact tree has an invalid parent link")
						}
						old.Edit(edit)
						cOld = cp.Parse([]byte(original), nil)
						if cOld == nil {
							t.Fatal("C returned no old tree")
						}
						ce := realCorpusCInputEdit(edit)
						cOld.Edit(&ce)
					}
					routedBefore, fallbackBefore := gts.AdmissionCandidateCounters()
					var tree *gts.Tree
					if incremental {
						tree, err = parser.ParseIncremental([]byte(edited), old)
						old.Release()
					} else {
						tree, err = parser.Parse([]byte(edited))
					}
					if err != nil {
						t.Fatal(err)
					}
					defer tree.Release()
					for i := 0; i < tree.RootNode().ChildCount(); i++ {
						if tree.RootNode().Child(i).Parent() != tree.RootNode() {
							t.Fatal("new tree lost its parent links after the old tree was released")
						}
					}
					if tree.ParseRuntime().NormalizationPassesRun != 0 {
						t.Fatal("native compact recovery ran a result normalization pass")
					}
					oracle := cp.Parse([]byte(edited), cOld)
					if cOld != nil {
						cOld.Close()
					}
					if oracle == nil {
						t.Fatal("C returned no edited tree")
					}
					defer oracle.Close()
					fresh := cp.Parse([]byte(edited), nil)
					if fresh == nil {
						t.Fatal("C returned no fresh tree")
					}
					defer fresh.Close()
					for _, ct := range []*sitter.Tree{oracle, fresh} {
						if diff := FirstDivergenceDumpV1(tree.RootNode(), &language, ct.RootNode()); diff != nil {
							t.Fatalf("C divergence: %+v", diff)
						}
						if diff := firstLockedCTreeFlagDivergence(tree.RootNode(), &language, ct.RootNode(), "/"); diff != nil {
							t.Fatal(diff)
						}
					}
					routedAfter, fallbackAfter := gts.AdmissionCandidateCounters()
					if incremental {
						runtime := tree.ParseRuntime()
						if !runtime.CompactIncrementalFullRecoveryRoute || runtime.CompactIncrementalReuseRoute || runtime.CompactIncrementalReusedSubtrees != 0 || runtime.CompactIncrementalReusedBytes != 0 {
							t.Fatalf("incremental did not publish an honest full compact recovery: %+v", runtime)
						}
						if routedAfter != routedBefore || fallbackAfter != fallbackBefore {
							t.Fatal("incremental recovery changed full-parse admission counters")
						}
					} else if routedAfter-routedBefore != 1 || fallbackAfter != fallbackBefore {
						t.Fatalf("compact fresh route=%d fallback=%d reason=%q", routedAfter-routedBefore, fallbackAfter-fallbackBefore, gts.AdmissionCandidateLastFallbackReason())
					}
					if incremental {
						for _, replacement := range []string{"", "\n"} {
							repaired := edited[:len(edited)-1] + replacement
							repairEdit := gts.InputEdit{StartByte: uint32(len(edited) - 1), OldEndByte: uint32(len(edited)), NewEndByte: uint32(len(repaired)), StartPoint: pointAtOffset([]byte(edited), len(edited)-1), OldEndPoint: pointAtOffset([]byte(edited), len(edited)), NewEndPoint: pointAtOffset([]byte(repaired), len(repaired))}
							borrowed := tree.Copy()
							borrowed.Edit(repairEdit)
							repairTree, err := parser.ParseIncremental([]byte(repaired), borrowed)
							borrowed.Release()
							if err != nil {
								t.Fatal(err)
							}
							defer repairTree.Release()
							cBorrowed := oracle.Clone()
							ce := realCorpusCInputEdit(repairEdit)
							cBorrowed.Edit(&ce)
							cRepair := cp.Parse([]byte(repaired), cBorrowed)
							cBorrowed.Close()
							cFreshRepair := cp.Parse([]byte(repaired), nil)
							if cRepair == nil || cFreshRepair == nil {
								t.Fatal("C returned no repaired tree")
							}
							defer cRepair.Close()
							defer cFreshRepair.Close()
							for _, ct := range []*sitter.Tree{cRepair, cFreshRepair} {
								if diff := FirstDivergenceDumpV1(repairTree.RootNode(), &language, ct.RootNode()); diff != nil {
									t.Fatalf("repair %q C divergence: %+v", replacement, diff)
								}
								if diff := firstLockedCTreeFlagDivergence(repairTree.RootNode(), &language, ct.RootNode(), "/"); diff != nil {
									t.Fatal(diff)
								}
							}
							if !tree.RootNode().HasError() || tree.RootNode().EndByte() != uint32(len(edited)) {
								t.Fatal("repairing a copy changed the original recovery tree")
							}
						}
					}
				})
			}
		})
	}
}
