//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"regexp"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// cFieldLabelPattern matches a C s-expression field label, for example
// "left: " in "(binary_expression left: (identifier) ...)".
// Node.SExpr does not print field labels (a pre-existing, unrelated
// limitation this test works around rather than fixes — see
// stripCFieldLabels).
var cFieldLabelPattern = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*:\s`)

// stripCFieldLabels drops C's s-expression field labels so a comparison
// against Node.SExpr's output is not defeated by that dumper gap alone.
// This mirrors the field-label projection the task #77/#80 review used
// for its own oracle comparisons (round-2 PR #1215 review, Q3): kind,
// child count, span, and HasError already come from typed fields, not
// the s-expression string, so this projection only affects the
// structural cross-check below, not the primary shape assertion.
func stripCFieldLabels(sexpr string) string {
	return cFieldLabelPattern.ReplaceAllString(sexpr, "")
}

// TestParityCullAcceptedRankLastRust is task #80's regression receipt.
// compareStackCullKeys (parser.go) used to rank an accepted stack ahead of
// a not-yet-accepted one in the per-iteration stack cull, unconditionally.
// C never lets an accepted version compete for a cull slot at all —
// ts_parser__accept removes it from the pool immediately
// (ts_stack_remove_version, ts_stack_halt, parser.c:1095-1096). This port
// cannot remove accepted stacks the same way, because buildResultFromGLR
// needs every one of them for a single final fold at the end of the parse
// (see the comment beside the fix in compareStackCullKeys). Ranking
// accepted stacks last, instead of first, restores C's intent without
// moving them out of the position task #77 made load-bearing for the
// final fold's "prefer the later candidate" tie-break.
//
// A 182-grammar sweep on a 288-case malformed-input corpus, head against
// the pre-fix engine, found exactly two moved pairs: rust "a->b" and
// "a=>b". Both moved from the pre-fix shape
// "(source_file (binary_expression (identifier) (ERROR) (identifier)))"
// to "(source_file (ERROR (binary_expression (identifier) (ERROR)
// (identifier))))". This test pins the fixed shape against the C oracle
// directly. No other grammar in the sweep moved.
func TestParityCullAcceptedRankLastRust(t *testing.T) {
	cLanguage, err := COracleLanguage("rust")
	if err != nil {
		t.Fatalf("COracleLanguage(rust): %v", err)
	}
	goLang := grammars.RustLanguage()

	for _, input := range []string{"a->b", "a=>b"} {
		t.Run(inputSubtestName(input), func(t *testing.T) {
			src := []byte(input)

			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatalf("SetLanguage: %v", err)
			}
			cTree := cParser.Parse(src, nil)
			defer cTree.Close()
			cShape := cOracleShape(cTree.RootNode())

			goParser := gotreesitter.NewParser(goLang)
			goTree, err := goParser.Parse(src)
			if err != nil {
				t.Fatalf("Go Parse: %v", err)
			}
			defer goTree.Release()
			goRootShape := goShape(goTree.RootNode(), goLang)

			if cShape.kind != goRootShape.kind ||
				cShape.children != goRootShape.children ||
				cShape.hasError != goRootShape.hasError ||
				cShape.start != goRootShape.start ||
				cShape.end != goRootShape.end {
				t.Fatalf("rust %q: C and Go disagree on shape:\n  C:  %+v\n  Go: %+v", input, cShape, goRootShape)
			}
			cSexprNoFields := stripCFieldLabels(cShape.sexpr)
			if cSexprNoFields != goRootShape.sexpr {
				t.Fatalf("rust %q: C and Go disagree on structure once C's field labels are stripped:\n  C (raw):     %s\n  C (stripped): %s\n  Go:           %s",
					input, cShape.sexpr, cSexprNoFields, goRootShape.sexpr)
			}
		})
	}
}
