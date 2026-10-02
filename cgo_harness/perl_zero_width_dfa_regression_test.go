//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestPerlZeroWidthMarkersSelectLockedCBranches(t *testing.T) {
	// The external marker must select the C branch for the call in grep's
	// block, on both engines (the map-grep CI witness).
	sources := []string{
		"grep { ok($_) } @array;\n",
		"foo(1, 2;\n",
		"our $x = 1;\nsub f { local $x = 2; g(); }\n",
		"# block form\nmap { +\"\\L$_\"  => 1 } @array;\n# <- function.builtin\nmap { ; \"\\L$_\" => 1 } @array;\n# <- function.builtin\nmap { ( \"\\L$_\" => 1 ) } @array;\n# <- function.builtin\nmap { lc($_) => 1 } @array;\n# <- function.builtin\n\ngrep { ok($_) } @array;\n# <- function.builtin\n\n# expr form\nmap +( lc($_) => 1 ), @array;\n# <- function.builtin\nmap +{ lc($_) => 1 }, @array;\n# <- function.builtin\nmap { \"\\L$_\"   => 1 }, @array;\n# <- function.builtin\n\ngrep ok($_), @array;\n# <- function.builtin\n\nmap { lc($_) => 1 } 1, 2, 3;\n# <- function.builtin\nmap +(lc($_) => 1 ), 1, 2, 3;\n# <- function.builtin\nmap { lc($_) => 1 } (1, 2, 3);\n# <- function.builtin\nmap +(lc($_) => 1 ), (1, 2), 3;\n# <- function.builtin\n",
	}
	cLanguage, err := COracleLanguage("perl")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	for i, source := range sources {
		t.Run(fmt.Sprintf("witness-%d", i), func(t *testing.T) {
			cTree := cParser.Parse([]byte(source), nil)
			if cTree == nil {
				t.Fatal("locked C returned no tree")
			}
			defer cTree.Close()
			language := grammars.PerlLanguage()
			for _, compact := range []bool{false, true} {
				t.Run(fmt.Sprintf("compact=%t", compact), func(t *testing.T) {
					parser := gotreesitter.NewParser(language)
					parser.SetAdmissionCandidateRoute(compact)
					tree, err := parser.Parse([]byte(source))
					if err != nil {
						t.Fatal(err)
					}
					defer tree.Release()
					if i < 3 {
						assertLockedCTreeExact(t, "Perl zero-width marker", tree, language, cTree)
						return
					}
					// The full highlight fixture has an unrelated list-expression
					// mismatch later in the file. Check the call affected by the
					// marker without treating that existing mismatch as parity.
					start := strings.Index(source, "ok($_)")
					goCall := findGoNodeByTypeAndStart(tree.RootNode(), language, "function_call_expression", uint32(start))
					cCall := findCNodeByKindAndStart(cTree.RootNode(), "function_call_expression", uint(start))
					if goCall == nil || cCall == nil {
						t.Fatal("grep call did not select the locked C function-call branch")
					}
					if diff := FirstDivergenceDumpV1(goCall, language, cCall); diff != nil {
						t.Fatalf("grep call differs from locked C: %+v", diff)
					}
					if diff := firstLockedCTreeFlagDivergence(goCall, language, cCall, "/function_call_expression"); diff != nil {
						t.Fatal(diff)
					}
				})
			}
		})
	}
}
