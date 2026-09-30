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
			for _, source := range []string{"%O", "  %O", "let a = 1;\n\n%O();\n", "let a = 1;\n\t%F();\n%G();\n"} {
				t.Run(source, func(t *testing.T) {
					oracle := cp.Parse([]byte(source), nil)
					if oracle == nil {
						t.Fatal("C returned no tree")
					}
					defer oracle.Close()
					if !oracle.RootNode().HasError() {
						t.Fatal("fixture did not exercise recovery")
					}
					for _, compact := range []bool{false, true} {
						parser := gts.NewParser(&language)
						parser.SetAdmissionCandidateRoute(compact)
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

// The extra ERROR wrapper must not make reuse return a stale token or point.
func TestSharedSkippedGapEditSession(t *testing.T) {
	for _, name := range []string{"javascript", "typescript"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("GOT_C_RECOVERY", "0")
			language := *grammars.DetectLanguageByName(name).Language()
			language.AutomaticForestEnabledByDefault = false
			for _, compact := range []bool{false, true} {
				parser := gts.NewParser(&language)
				parser.SetAdmissionCandidateRoute(compact)
				source := []byte("let a = 1;\n\t%F();\n%G();\n")
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
					next, err := parser.ParseIncremental(step.Source, old)
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
		})
	}
}
