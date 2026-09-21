//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestBladeCustomContainerMismatchedEndTagCOracle is task #67's regression
// receipt. PR #1195 ("fix(grammars/beancount,blade,caddy): bind external
// scanner symbols and bump three grammars") wrapped its scoped-slot
// regression construct in <div> instead of a Blade component tag such as
// <x-alert>, naming "an unrelated, pre-existing GLR gap on the Go runtime
// with mismatched end tags directly inside a custom-tag-typed container"
// (cgo_harness/blade_scoped_slot_regression_test.go).
//
// The gap: buildReduceChildrenNoAliasNoFieldsPlanned (parser_reduce.go)
// decided whether a reduce's child stayed direct or got flattened by
// consulting only the child's grammar symbol's visibility
// (symbolMeta[n.symbol].Visible). A MISSING leaf synthesized by
// cHandleError's missing-token search (parser_recover_c.go) has zero
// children of its own; when its symbol also happens to be hidden — as
// html-family's "_implicit_end_tag" is, by the leading-underscore naming
// convention — flattening it recurses into those zero children and the
// MISSING node vanishes from the tree entirely, silently, with no
// resulting HasError() either. C never drops it: a missing subtree always
// stays a direct child regardless of its own symbol's visibility (compare
// the C-recovery port's own cAppendVisibleSplice, which already carried
// this exact "ERROR and missing nodes always stay" exemption for the
// C-recovery-specific splice path; the ordinary, non-recovery reduce path
// never had the equivalent rule).
//
// This is not blade- or custom-tag-specific: `<div>hi</span>` (no custom
// tag at all) reproduces the identical divergence, and the fix is a
// general one-line exemption in the shared reduce-child builder. See the
// PR body for the wider verification (html-family sweep, cross-grammar
// spot checks).
func TestBladeCustomContainerMismatchedEndTagCOracle(t *testing.T) {
	cLanguage, err := COracleLanguage("blade")
	if err != nil {
		t.Fatalf("COracleLanguage(blade): %v", err)
	}
	goLang := grammars.BladeLanguage()

	// The custom-tag-container witness the task names, plus the closest
	// non-custom-tag analog that isolates the fix from Blade's own
	// external-scanner tag classification.
	for _, input := range []string{"<x-alert>hi</span>", "<div>hi</span>"} {
		t.Run(inputSubtestName(input), func(t *testing.T) {
			src := []byte(input)

			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatalf("SetLanguage: %v", err)
			}
			cTree := cParser.Parse(src, nil)
			defer cTree.Close()
			cRoot := cTree.RootNode()
			if !cRoot.HasError() {
				t.Fatalf("blade C oracle root has no error for %q:\n%s", input, dumpCTree(cRoot, 0))
			}

			goParser := gotreesitter.NewParser(goLang)
			goTree, err := goParser.Parse(src)
			if err != nil {
				t.Fatalf("Go Parse: %v", err)
			}
			defer goTree.Release()
			goRoot := goTree.RootNode()

			// HasError is the load-bearing assertion the bug broke: before
			// the fix, the vanished MISSING leaf left this false.
			if !goRoot.HasError() {
				t.Fatalf("blade Go root has no error for %q (the MISSING _implicit_end_tag leaf vanished):\n%s", input, dumpGoTree(goRoot, goLang, 0))
			}

			// Structural comparison: kind, child count, span. (SExpr string
			// comparison is not used here because Go's SExpr() dumper does
			// not print the "MISSING" prefix tree-sitter C does for a named
			// missing node — a separate, pre-existing, unrelated dumper
			// gap; Node.IsMissing() is what actually matters and is
			// asserted below.)
			cShape := cOracleShape(cRoot)
			goShapeV := goShape(goRoot, goLang)
			if cShape.kind != goShapeV.kind || cShape.children != goShapeV.children ||
				cShape.hasError != goShapeV.hasError || cShape.start != goShapeV.start || cShape.end != goShapeV.end {
				t.Fatalf("blade %q: C and Go disagree on shape:\n  C:  %+v\n  Go: %+v", input, cShape, goShapeV)
			}

			implicitEnd := findBladeGoNodeByType(goRoot, goLang, "_implicit_end_tag")
			if implicitEnd == nil {
				t.Fatalf("blade %q: no _implicit_end_tag node found; expected a MISSING one", input)
			}
			if !implicitEnd.IsMissing() {
				t.Fatalf("blade %q: _implicit_end_tag node is present but not marked missing", input)
			}
		})
	}
}
