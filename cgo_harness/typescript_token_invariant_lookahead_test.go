//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestTypeScriptTokenInvariantNumericLockedC(t *testing.T) {
	testTypeScriptTokenInvariantLockedC(t, []string{
		"const n = 1;\n", "const n = 1e1;\n",
		"let x: Array<Array<number>>;\nconst n = 1;\n",
		"const n = 8 >> 1;\n", "type N = 1;\n",
		"type N = Box<Box<1>>;\n", "foo<1>(2);\n", "const n = 1 >> 2;\n",
	}, []byte{'2', '1', '2'}, true)
}

func TestTypeScriptTokenInvariantSyntaxLockedC(t *testing.T) {
	testTypeScriptTokenInvariantLockedC(t, []string{"const n = 0b1;\n"}, []byte{'2'}, false)
	testTypeScriptTokenInvariantLockedC(t, []string{"const n = 1e1;\n"}, []byte{'+'}, false)
	testTypeScriptTokenInvariantLockedC(t, []string{"const n = '1';\n"}, []byte{'2'}, false)
}

func testTypeScriptTokenInvariantLockedC(t *testing.T, initials []string, replacements []byte, positive bool) {
	t.Helper()
	cLanguage, err := ParityCLanguage("typescript")
	if err != nil {
		t.Fatal(err)
	}
	for _, initial := range initials {
		for _, compact := range []bool{false, true} {
			name := "legacy/" + initial
			if compact {
				name = "compact/" + initial
			}
			t.Run(name, func(t *testing.T) {
				language := grammars.TypescriptLanguage()
				parser := gts.NewParser(language)
				parser.SetAdmissionCandidateRoute(compact)
				cParser := sitter.NewParser()
				defer cParser.Close()
				if err := cParser.SetLanguage(cLanguage); err != nil {
					t.Fatal(err)
				}
				source := []byte(initial)
				old, err := parser.Parse(source)
				if err != nil || old == nil {
					t.Fatalf("initial parse: %v", err)
				}
				defer func() { old.Release() }()
				cOld := cParser.Parse(source, nil)
				if cOld == nil {
					t.Fatal("C initial parse returned no tree")
				}
				defer func() { cOld.Close() }()
				if cOld.RootNode().HasError() {
					t.Fatal("C initial fixture is malformed")
				}
				assertLockedCTreeExact(t, "initial numeric tree", old, language, cOld)
				offset := strings.LastIndex(initial, "1")
				for _, replacement := range replacements {
					edited := append([]byte(nil), source...)
					edited[offset] = replacement
					edit := gts.InputEdit{StartByte: uint32(offset), OldEndByte: uint32(offset + 1), NewEndByte: uint32(offset + 1), StartPoint: pointAtOffset(source, offset), OldEndPoint: pointAtOffset(source, offset+1), NewEndPoint: pointAtOffset(edited, offset+1)}
					old.Edit(edit)
					next, profile, err := parser.ParseIncrementalProfiled(edited, old)
					if err != nil || next == nil {
						t.Fatalf("incremental parse: %v", err)
					}
					if next != old {
						old.Release()
					}
					old = next
					cEdit := realCorpusCInputEdit(edit)
					cOld.Edit(&cEdit)
					cNext := cParser.Parse(edited, cOld)
					if cNext == nil {
						t.Fatal("C incremental parse returned no tree")
					}
					cOld.Close()
					cOld = cNext
					cFresh := cParser.Parse(edited, nil)
					if cFresh == nil {
						t.Fatal("C fresh parse returned no tree")
					}
					func() {
						defer cFresh.Close()
						fresh, err := gts.NewParser(language).Parse(edited)
						if err != nil || fresh == nil {
							t.Fatalf("fresh parse: %v", err)
						}
						defer fresh.Release()
						compare := assertLockedCTreeExact
						if !positive {
							compare = assertTypeScriptSyntaxTreeLockedC
						}
						compare(t, "fresh numeric tree", fresh, language, cFresh)
						compare(t, "incremental versus C fresh", next, language, cFresh)
						compare(t, "incremental versus C incremental", next, language, cNext)
					}()
					if positive && (profile.TokenInvariantDependencyChecks != 1 || profile.ReparseNanos != 0 || profile.NewNodesAllocated != 0 || profile.ReusedSubtrees == 0) {
						t.Fatalf("numeric edit lost dependency-proven shortcut: %+v", profile)
					}
					if !positive && profile.ReparseNanos == 0 && profile.NewNodesAllocated == 0 {
						t.Fatalf("syntax/text edit incorrectly took token shortcut: %+v", profile)
					}
					source = edited
				}
			})
		}
	}
}

// Syntax edits can produce errors. Compare those errors without requiring a clean root.
func assertTypeScriptSyntaxTreeLockedC(t *testing.T, label string, tree *gts.Tree, language *gts.Language, oracle *sitter.Tree) {
	t.Helper()
	if diff := FirstDivergenceDumpV1(tree.RootNode(), language, oracle.RootNode()); diff != nil {
		t.Fatalf("%s: %+v", label, diff)
	}
	if diff := firstLockedCTreeFlagDivergence(tree.RootNode(), language, oracle.RootNode(), "/"); diff != nil {
		t.Fatalf("%s: %v", label, diff)
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := COracleDeepDigest(oracle)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.SHA256 != digest {
		t.Fatalf("%s: deep digest Go=%s C=%s", label, inspection.SHA256, digest)
	}
}
