//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestCompactAcceptanceDerivationTreesEqual covers the structural axes
// compactAcceptanceDerivationTreesEqual (parsercore_phase0_driver.go)
// compares directly: nil handling, reflexivity on a real parsed tree,
// symbol/span divergence, and child-count divergence. Field-assignment
// sensitivity -- the axis that is invisible in an SExpr dump and the reason
// the comparator checks fields explicitly -- is covered at the integration
// level by TestA3ArmNoOpConfirmationApexClassLiteralDeclinesAsMaterialElection
// (admission_switch_a3_arm_noop_confirmation_test.go), which exercises the
// real apex class_literal_alias witness where two tied derivations share a
// symbol and shape and differ only in an "object" field assignment; a unit
// test would need to hand-construct that exact ambiguity, which the real
// witness already does more faithfully.
func TestCompactAcceptanceDerivationTreesEqual(t *testing.T) {
	lang := grammars.GoLanguage()

	parse := func(t *testing.T, source string) *gts.Tree {
		t.Helper()
		p := gts.NewParser(lang)
		tree, err := p.Parse([]byte(source))
		if err != nil {
			t.Fatalf("parse %q: %v", source, err)
		}
		t.Cleanup(tree.Release)
		return tree
	}

	t.Run("nil-nil is equal", func(t *testing.T) {
		if !gts.CompactAcceptanceDerivationTreesEqualForTest(lang, nil, nil) {
			t.Fatal("nil, nil = false, want true")
		}
	})

	t.Run("nil-vs-node is not equal", func(t *testing.T) {
		tree := parse(t, "package main\n")
		if gts.CompactAcceptanceDerivationTreesEqualForTest(lang, tree.RootNode(), nil) {
			t.Fatal("node, nil = true, want false")
		}
		if gts.CompactAcceptanceDerivationTreesEqualForTest(lang, nil, tree.RootNode()) {
			t.Fatal("nil, node = true, want false")
		}
	})

	t.Run("two fresh parses of the same source are equal", func(t *testing.T) {
		source := "package main\n\nfunc f(x int, y int) int {\n\treturn x + y\n}\n"
		a := parse(t, source)
		b := parse(t, source)
		if !gts.CompactAcceptanceDerivationTreesEqualForTest(lang, a.RootNode(), b.RootNode()) {
			t.Fatalf("two deterministic parses of the same source compared unequal:\na=%s\nb=%s",
				a.RootNode().SExpr(lang), b.RootNode().SExpr(lang))
		}
	})

	t.Run("different source is not equal (symbol/span divergence)", func(t *testing.T) {
		a := parse(t, "package main\n\nfunc f() {}\n")
		b := parse(t, "package main\n\nvar x = 1\n")
		if gts.CompactAcceptanceDerivationTreesEqualForTest(lang, a.RootNode(), b.RootNode()) {
			t.Fatalf("structurally different sources compared equal:\na=%s\nb=%s",
				a.RootNode().SExpr(lang), b.RootNode().SExpr(lang))
		}
	})

	t.Run("child count divergence is not equal", func(t *testing.T) {
		// Same root symbol (source_file) and same start byte (0), but a
		// different child count and a different second child, so this must
		// fail on the child-count check specifically, not the root's own
		// symbol/span/flags.
		a := parse(t, "package main\n")
		b := parse(t, "package main\n\nfunc f() {}\n")
		if a.RootNode().Symbol() != b.RootNode().Symbol() {
			t.Fatalf("test fixture invalid: root symbols differ (%d vs %d)", a.RootNode().Symbol(), b.RootNode().Symbol())
		}
		if a.RootNode().ChildCount() == b.RootNode().ChildCount() {
			t.Fatalf("test fixture invalid: root child counts match (%d)", a.RootNode().ChildCount())
		}
		if gts.CompactAcceptanceDerivationTreesEqualForTest(lang, a.RootNode(), b.RootNode()) {
			t.Fatalf("different child counts compared equal:\na=%s\nb=%s",
				a.RootNode().SExpr(lang), b.RootNode().SExpr(lang))
		}
	})

	t.Run("a subtree is not equal to its own parent", func(t *testing.T) {
		tree := parse(t, "package main\n\nfunc f() {}\n")
		root := tree.RootNode()
		if root.ChildCount() == 0 {
			t.Fatal("test fixture invalid: root has no children")
		}
		child := root.Child(0)
		if gts.CompactAcceptanceDerivationTreesEqualForTest(lang, root, child) {
			t.Fatalf("root compared equal to its own child:\nroot=%s\nchild=%s", root.SExpr(lang), child.SExpr(lang))
		}
	})
}
