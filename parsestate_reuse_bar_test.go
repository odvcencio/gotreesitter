//go:build gts_parsercorephase0

package gotreesitter

import (
	"bytes"
	"testing"
)

// TestCompactMaterializedTreeBarsIncrementalReuse seals the fail-closed half of
// the compact reuse contract. A diagnostic runner that did not request replay
// carries no state proof, so every entry (DFA, custom token source, and Copy)
// must fall back without sharing nodes.
func TestCompactMaterializedTreeBarsIncrementalReuse(t *testing.T) {
	runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	lang := runner.lang

	// A source with a comment so the tree has an extra (abstained-preGoto) node,
	// making an accidental splice observable, plus a digit leaf to edit.
	src := []byte("package p\n\n// c\nvar x = 1\n")
	newSrc := []byte("package p\n\n// c\nvar x = 2\n")
	pos := bytes.IndexByte(src, '1')
	if pos < 0 {
		t.Fatal("fixture invariant: expected a '1' digit to edit")
	}
	col := uint32(pos - bytes.LastIndexByte(src[:pos], '\n') - 1)
	editOf := func() InputEdit {
		return InputEdit{
			StartByte: uint32(pos), OldEndByte: uint32(pos + 1), NewEndByte: uint32(pos + 1),
			StartPoint:  Point{Row: 3, Column: col},
			OldEndPoint: Point{Row: 3, Column: col + 1},
			NewEndPoint: Point{Row: 3, Column: col + 1},
		}
	}

	p := NewParser(lang)
	want, err := NewParser(lang).Parse(newSrc)
	if err != nil {
		t.Fatalf("fresh Parse of edited source: %v", err)
	}
	defer want.Release()

	// requireFreshFallback asserts got is a correct, node-disjoint fresh parse.
	requireFreshFallback := func(t *testing.T, got, oldTree *Tree) {
		t.Helper()
		if got == oldTree {
			t.Fatal("ParseIncremental returned the old tree unchanged for a real edit")
		}
		if n := countSharedNodes(oldTree, got); n != 0 {
			t.Fatalf("compact old tree was reused: %d node objects shared with the reparse (want 0)", n)
		}
		parserCoreWarmRequireDeepEqual(t, got, want, lang)
	}

	t.Run("dfa_entry", func(t *testing.T) {
		treeA, err := runner.parse(src)
		if err != nil {
			t.Fatalf("compact parse: %v", err)
		}
		defer treeA.Release()
		if !treeA.compactMaterialized {
			t.Fatal("compact tree is missing the compactMaterialized provenance flag")
		}
		if !oldTreeDisablesIncrementalReuse(treeA) {
			t.Fatal("compact tree must disable incremental reuse")
		}
		treeA.Edit(editOf())
		// The disabled-tree token-invariant gate refuses the compact tree.
		if reused, ok := p.tryTokenInvariantReuseForDisabledOldTree(newSrc, treeA, nil); ok || reused != nil {
			if reused != nil {
				reused.Release()
			}
			t.Fatal("compact tree was reused via the token-invariant leaf path")
		}
		got, profile, err := p.ParseIncrementalProfiled(newSrc, treeA)
		if err != nil {
			t.Fatalf("ParseIncrementalProfiled: %v", err)
		}
		defer got.Release()
		if !profile.ReuseUnsupported {
			t.Fatal("compact tree fallback did not report unsupported reuse")
		}
		if profile.ReuseUnsupportedReason != compactIncrementalReuseUnsupportedReason {
			t.Fatalf("compact tree fallback reason = %q, want %q", profile.ReuseUnsupportedReason, compactIncrementalReuseUnsupportedReason)
		}
		requireFreshFallback(t, got, treeA)
	})

	t.Run("survives_copy", func(t *testing.T) {
		treeA, err := runner.parse(src)
		if err != nil {
			t.Fatalf("compact parse: %v", err)
		}
		defer treeA.Release()
		cp := treeA.Copy()
		if cp == nil {
			t.Fatal("Copy returned nil")
		}
		defer cp.Release()
		// Copy must carry the provenance flags -- otherwise the copy becomes
		// reuse-eligible on the standard DFA path even though its nodes still
		// hold the replayed/abstained states.
		if !cp.compactMaterialized {
			t.Fatal("Tree.Copy dropped the compactMaterialized provenance flag")
		}
		if !oldTreeDisablesIncrementalReuse(cp) {
			t.Fatal("Tree.Copy dropped the incrementalReuseDisabled flag")
		}
		cp.Edit(editOf())
		if reused, ok := p.tryTokenInvariantReuseForDisabledOldTree(newSrc, cp, nil); ok || reused != nil {
			if reused != nil {
				reused.Release()
			}
			t.Fatal("copied compact tree was reused via the token-invariant leaf path")
		}
		got, err := p.ParseIncremental(newSrc, cp)
		if err != nil {
			t.Fatalf("ParseIncremental over copy: %v", err)
		}
		defer got.Release()
		requireFreshFallback(t, got, cp)
	})

	t.Run("token_source_entry", func(t *testing.T) {
		treeA, err := runner.parse(src)
		if err != nil {
			t.Fatalf("compact parse: %v", err)
		}
		defer treeA.Release()
		treeA.Edit(editOf())
		// A reuse-SUPPORTING token source (the Go DFA source; Go's scanner opts
		// into reuse): without the bar this path would reach the reuseCursor
		// subtree splice. The compact provenance must still force a fresh parse.
		ts := p.acquireParserDFATokenSource(newSrc)
		if !tokenSourceSupportsIncrementalReuse(ts) {
			ts.Close()
			t.Skip("Go DFA token source unexpectedly does not support incremental reuse")
		}
		got, err := p.ParseIncrementalWithTokenSource(newSrc, treeA, ts)
		if err != nil {
			t.Fatalf("ParseIncrementalWithTokenSource: %v", err)
		}
		defer got.Release()
		requireFreshFallback(t, got, treeA)
	})
}

// TestAdmissionCompactTreeCarriesIncrementalReuseProof seals the complementary
// side of the compact provenance contract: the production admission runner
// requests table replay, and a tree whose replay is complete enough for reuse
// and whose scanner is provably quiescent must preserve the O(edit) path.
func TestAdmissionCompactTreeCarriesIncrementalReuseProof(t *testing.T) {
	lang, err := authenticatedParserCoreGoLanguage(parserCoreWarmGoScanner)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("package p\n\n// c\nvar x = 1\n")
	edited := []byte("package p\n\n// c\nvar x = 2\n")
	pos := bytes.IndexByte(src, '1')
	if pos < 0 {
		t.Fatal("fixture invariant: expected digit")
	}

	parser := NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	resetAdmissionCandidateCounters()
	oldTree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("candidate Parse: %v", err)
	}
	defer oldTree.Release()
	routed, fallback := AdmissionCandidateCounters()
	if routed != 1 || fallback != 0 {
		t.Fatalf("candidate route counters=%d/%d, want 1/0 (reason=%q)", routed, fallback, AdmissionCandidateLastFallbackReason())
	}
	if !oldTree.compactMaterialized {
		t.Fatal("admission tree is missing compact provenance")
	}
	if oldTree.incrementalReuseDisabled {
		t.Fatal("proof-carrying admission tree remained barred from incremental reuse")
	}
	requireCompactIncrementalStateProof(t, oldTree)

	oldTree.Edit(InputEdit{
		StartByte: uint32(pos), OldEndByte: uint32(pos + 1), NewEndByte: uint32(pos + 1),
		StartPoint: Point{Row: 3, Column: 8}, OldEndPoint: Point{Row: 3, Column: 9}, NewEndPoint: Point{Row: 3, Column: 9},
	})
	got, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("ParseIncrementalProfiled: %v", err)
	}
	defer got.Release()
	if profile.ReuseUnsupported {
		t.Fatalf("proof-carrying compact tree fell back: %s", profile.ReuseUnsupportedReason)
	}
	if profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
		t.Fatalf("proof-carrying compact tree reused nothing: %+v", profile)
	}

	wantParser := NewParser(lang)
	wantParser.SetAdmissionCandidateRoute(false)
	want, err := wantParser.Parse(edited)
	if err != nil {
		t.Fatalf("production Parse: %v", err)
	}
	defer want.Release()
	parserCoreWarmRequireDeepEqual(t, got, want, lang)
}

func requireCompactIncrementalStateProof(t *testing.T, tree *Tree) {
	t.Helper()
	if tree == nil || tree.root == nil {
		t.Fatal("compact state proof has no root")
	}
	stack := []*Node{tree.root}
	for len(stack) > 0 {
		last := len(stack) - 1
		node := stack[last]
		stack = stack[:last]
		if !node.isCompactMaterialized() || !compactNodeStateProofAvailable(node) {
			t.Fatalf("proof-carrying node %s %d..%d lacks authenticated compact state", node.Type(tree.language), node.startByte, node.endByte)
		}
		children := node.ChildCount()
		if children == 0 {
			if node.parseState == 0 {
				t.Fatalf("proof-carrying leaf %s %d..%d abstained from parseState", node.Type(tree.language), node.startByte, node.endByte)
			}
			continue
		}
		if node != tree.root && node.preGotoState == 0 {
			t.Fatalf("proof-carrying node %s %d..%d abstained from preGotoState", node.Type(tree.language), node.startByte, node.endByte)
		}
		for i := children - 1; i >= 0; i-- {
			stack = append(stack, node.Child(i))
		}
	}
}

// countSharedNodes returns how many *Node objects are pointer-identical between
// the two trees. A genuine fresh parse shares zero; any reuse splice shares the
// reused subtree roots/leaves.
func countSharedNodes(a, b *Tree) int {
	if a == nil || b == nil || a.root == nil || b.root == nil {
		return 0
	}
	set := map[*Node]struct{}{}
	var collect func(n *Node)
	collect = func(n *Node) {
		if n == nil {
			return
		}
		set[n] = struct{}{}
		for i := 0; i < n.ChildCount(); i++ {
			collect(n.Child(i))
		}
	}
	collect(a.root)
	shared := 0
	var scan func(n *Node)
	scan = func(n *Node) {
		if n == nil {
			return
		}
		if _, ok := set[n]; ok {
			shared++
		}
		for i := 0; i < n.ChildCount(); i++ {
			scan(n.Child(i))
		}
	}
	scan(b.root)
	return shared
}
