//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// A fallback lexer can skip a terminal that its current state cannot accept.
// Recovery must retain the error-mode token under an extra ERROR, with the
// token's actual extent, rather than replacing the skipped gap with a leaf.
func TestSharedSkippedGapLockedC(t *testing.T) {
	for _, name := range []string{"javascript", "typescript"} {
		t.Run(name, func(t *testing.T) {
			// Exercise the shared fallback on both grammars. JavaScript ships
			// with this recovery route; TypeScript normally uses C-style recovery.
			t.Setenv("GOT_C_RECOVERY", "0")
			entry := grammars.DetectLanguageByName(name)
			language := *entry.Language()
			language.AutomaticForestEnabledByDefault = false
			cLanguage, err := ParityCLanguage(name)
			if err != nil {
				t.Fatal(err)
			}
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			// Reuse each parser across growing and shrinking inputs, including
			// a clean parse between errors. Released recovery wrappers must not
			// leave stale children or lexer state in the next operation.
			var parsers [2]*gts.Parser
			for i := range parsers {
				parsers[i] = gts.NewParser(&language)
				parsers[i].SetAdmissionCandidateRoute(i == 1)
			}
			for _, source := range []string{"%O", "  %O", "let a = 1;\n\n%O();\n", "let a = 1;\n\t%F();\n%G();\n", "let a = 1;\n", "%O"} {
				t.Run(source, func(t *testing.T) {
					oracle := cp.Parse([]byte(source), nil)
					if oracle == nil {
						t.Fatal("C returned no tree")
					}
					defer oracle.Close()
					wantError := source != "let a = 1;\n"
					if oracle.RootNode().HasError() != wantError {
						t.Fatalf("C HasError=%t, want %t", oracle.RootNode().HasError(), wantError)
					}
					for i, parser := range parsers {
						compact := i == 1
						tree, err := parser.Parse([]byte(source))
						if err != nil || tree == nil {
							t.Fatalf("parse compact=%t: %v", compact, err)
						}
						defer tree.Release()
						inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), &language)
						if err != nil {
							t.Fatal(err)
						}
						want, err := COracleDeepDigest(oracle)
						if err != nil {
							t.Fatal(err)
						}
						if inspection.SHA256 != want {
							t.Fatalf("compact=%t digest Go=%s C=%s divergence=%+v", compact, inspection.SHA256, want,
								FirstDivergenceDumpV1(tree.RootNode(), &language, oracle.RootNode()))
						}
						if tree.ParseStopReason() != gts.ParseStopAccepted {
							t.Fatalf("compact=%t stop=%s", compact, tree.ParseStopReason())
						}
					}
				})
			}
		})
	}
}

// Invalid prefixes must retain the same root and error extents as locked C.
// The span-only fallback has an existing ERROR-shape difference from C; the
// exact-token cases above still require full deep-digest equality.
func TestSharedSkippedGapInvalidPrefixLockedC(t *testing.T) {
	for _, name := range []string{"javascript", "typescript"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("GOT_C_RECOVERY", "0")
			language := *grammars.DetectLanguageByName(name).Language()
			language.AutomaticForestEnabledByDefault = false
			cLanguage, err := ParityCLanguage(name)
			if err != nil {
				t.Fatal(err)
			}
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			for _, source := range []string{"\x01%O", "\x00%O", "\x02%O", "\x01 \t%O"} {
				t.Run(source, func(t *testing.T) {
					oracle := cp.Parse([]byte(source), nil)
					if oracle == nil {
						t.Fatal("C returned no tree")
					}
					defer oracle.Close()
					want := oracle.RootNode()
					if want.StartByte() != 0 || want.EndByte() != uint(len(source)) || !want.HasError() || want.ChildCount() == 0 || want.Child(0).Kind() != "ERROR" {
						t.Fatal("C fixture must retain the invalid prefix under ERROR")
					}
					for _, compact := range []bool{false, true} {
						parser := gts.NewParser(&language)
						parser.SetAdmissionCandidateRoute(compact)
						tree, err := parser.Parse([]byte(source))
						if err != nil || tree == nil {
							t.Fatalf("compact=%t parse: %v", compact, err)
						}
						defer tree.Release()
						root := tree.RootNode()
						if tree.ParseStopReason() != gts.ParseStopAccepted || uint(root.StartByte()) != want.StartByte() || uint(root.EndByte()) != want.EndByte() || root.HasError() != want.HasError() {
							t.Fatalf("compact=%t stop=%s Go span=%d..%d error=%t C span=%d..%d error=%t", compact, tree.ParseStopReason(), root.StartByte(), root.EndByte(), root.HasError(), want.StartByte(), want.EndByte(), want.HasError())
						}
						if root.ChildCount() == 0 || root.Child(0).Type(&language) != "ERROR" || uint(root.Child(0).StartByte()) != want.Child(0).StartByte() || uint(root.Child(0).EndByte()) != want.Child(0).EndByte() || !root.Child(0).HasError() {
							t.Fatalf("compact=%t invalid prefix lost from ERROR: %s", compact, root.SExpr(&language))
						}
					}
				})
			}
		})
	}
}

// The extra ERROR wrapper must not make reuse return a stale token or point.
func TestSharedSkippedGapEditSession(t *testing.T) {
	testSharedSkippedGapEditSession(t, false)
}

func TestSharedSkippedGapEditSessionProfiled(t *testing.T) {
	testSharedSkippedGapEditSession(t, true)
}

func testSharedSkippedGapEditSession(t *testing.T, profiled bool) {
	for _, name := range []string{"javascript", "typescript"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("GOT_C_RECOVERY", "0")
			language := *grammars.DetectLanguageByName(name).Language()
			language.AutomaticForestEnabledByDefault = false
			for _, input := range []string{"let a = 1;\n\t%F();\n%G();\n", "\x01%O"} {
				for _, compact := range []bool{false, true} {
					parser := gts.NewParser(&language)
					parser.SetAdmissionCandidateRoute(compact)
					source := []byte(input)
					old, err := parser.Parse(source)
					if err != nil {
						t.Fatal(err)
					}
					defer func() { old.Release() }()
					if allocs := testing.AllocsPerRun(5, func() {
						next, err := parser.ParseIncremental(source, old)
						if err != nil {
							t.Fatal(err)
						}
						next.Release()
					}); allocs != 0 {
						t.Fatalf("compact=%t no-edit allocations=%g", compact, allocs)
					}
					for i, step := range benchfixtures.EditingSession(source) {
						old.Edit(step.Edit)
						var next *gts.Tree
						var err error
						if profiled {
							next, _, err = parser.ParseIncrementalProfiled(step.Source, old)
						} else {
							next, err = parser.ParseIncremental(step.Source, old)
						}
						if err != nil {
							t.Fatal(err)
						}
						freshParser := gts.NewParser(&language)
						freshParser.SetAdmissionCandidateRoute(compact)
						fresh, err := freshParser.Parse(step.Source)
						if err != nil {
							t.Fatal(err)
						}
						for _, tree := range []*gts.Tree{next, fresh} {
							root := tree.RootNode()
							if root.IsError() && !root.HasError() {
								t.Fatalf("step=%d ERROR root lost HasError", i+1)
							}
							if root.EndByte() < uint32(len(step.Source)) && tree.ParseStopReason() == gts.ParseStopAccepted {
								t.Fatalf("step=%d accepted root ends at %d of %d", i+1, root.EndByte(), len(step.Source))
							}
						}
						a, err := benchfixtures.InspectGoTree(next.RootNode(), &language)
						if err != nil {
							t.Fatal(err)
						}
						b, err := benchfixtures.InspectGoTree(fresh.RootNode(), &language)
						if err != nil {
							t.Fatal(err)
						}
						if a.SHA256 != b.SHA256 {
							t.Fatalf("compact=%t step=%d incremental=%s fresh=%s", compact, i+1, a.SHA256, b.SHA256)
						}
						if old != next {
							old.Release()
						}
						fresh.Release()
						old = next
					}
				}
			}
		})
	}
}
