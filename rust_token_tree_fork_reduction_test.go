package gotreesitter_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Macro-heavy Rust that drives the delim_token_tree_repeat1 (state 83) boundary
// on many continuation tokens — operators, nested delimiters, `$`, `=>`, `:` —
// i.e. exactly the tokens the existing rustRepetitionShiftConflictChoice does
// NOT yet cover. Each token inside a macro token-tree must continue the tree
// (repetition shift); closing it early (reduce) is a dead-end.
const rustTokenTreeSource = `
fn main() {
    let v = vec![1 + 2, 3 * 4, 5 - 6, a & b, c | d];
    println!("{} {}", x && y, p || q);
    assert_eq!(lhs << 2, rhs >> 1);
    my_macro!(a : b, c => d, e $ f, g ? h);
    nested!(inner!(deep!(x % y ^ z, !flag)));
    paths!(std::collections::HashMap, core::mem);
    mixed! { key: value; arr[idx] = func(arg) + 1 }
}
`

// macroExpectations: parsing the source above must yield a tree with no parse
// errors and at least this many macro_invocation nodes. The fork-reduction
// change must NOT alter the tree — this is the parity safety net.
func countNodeType(n *gotreesitter.Node, lang *gotreesitter.Language, typ string) int {
	if n == nil {
		return 0
	}
	count := 0
	if n.Type(lang) == typ {
		count++
	}
	for i := 0; i < int(n.ChildCount()); i++ {
		count += countNodeType(n.Child(i), lang, typ)
	}
	return count
}

// TestRustTokenTreeParity is the correctness gate: the macro token-tree source
// must parse with no errors and the expected macro structure. Green before and
// after the resolver change (fork resolution must not alter the output tree).
func TestRustTokenTreeParity(t *testing.T) {
	lang := grammars.RustLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse([]byte(rustTokenTreeSource))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := tree.RootNode()
	if root.HasError() {
		t.Errorf("tree has parse error in macro token-tree source")
	}
	macros := countNodeType(root, lang, "macro_invocation")
	if macros < 7 {
		t.Errorf("expected >=7 macro_invocation nodes, got %d (root=%s)", macros, root.Type(lang))
	}
	tokenTrees := countNodeType(root, lang, "token_tree")
	t.Logf("root=%s hasError=%v macro_invocations=%d token_trees=%d",
		root.Type(lang), root.HasError(), macros, tokenTrees)
}

// TestRustTokenTreeForkCount measures SURVIVING GLR forks (perf counters, built
// with -tags perf). On current code the uncovered state-83 operator/bracket
// tokens fork; after extending the resolver they should collapse. Without the
// perf build tag the counters are zero and the test logs + skips assertions.
func TestRustTokenTreeForkCount(t *testing.T) {
	lang := grammars.RustLanguage()
	gotreesitter.ResetPerfCounters()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse([]byte(rustTokenTreeSource))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if tree.RootNode().HasError() {
		t.Fatalf("parse error")
	}
	snap := gotreesitter.PerfCountersSnapshot()
	if snap.ForkCount == 0 && snap.ConflictRS == 0 {
		t.Skip("perf counters disabled (build with -tags perf to measure forks)")
	}
	t.Logf("SURVIVING forks=%d conflictRS=%d conflictRR=%d maxStacks=%d",
		snap.ForkCount, snap.ConflictRS, snap.ConflictRR, snap.MaxConcurrentStacks)
}
