package gotreesitter_test

// Stage-1 acceptance test for the faithful port of tree-sitter C's error
// recovery (ts_parser__recover) — gated to the `requirements` grammar via
// errorCostCompetitionLanguage.
//
// The minimal reproducer from recovery-cost-competition.md (moved to
// gotreesitter-specs (external)):
// a trailing comment after an environment marker is unparseable in BOTH
// parsers, but C completes the in-progress `requirement` production via
// do_all_potential_reductions before wrapping only the failed suffix:
//
//	file [0:39]
//	  requirement [0:30]        "pkg ; python_version >= '3.13'"
//	  ERROR [30:38] (extra)     "  # note"
//	    comment [32:38]         "# note"
//
// (C oracle shape verified against tree-sitter/go-tree-sitter v0.25.0 in
// docker; see TestCTreeDumpDiag.) Without the port, Go shatters the line and
// roots the tree at ERROR.

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func reqRecoverParse(t *testing.T, src string) (*gts.Tree, *gts.Language) {
	t.Helper()
	t.Setenv("GOT_C_RECOVERY", "requirements")
	gts.ResetParseEnvConfigCacheForTests()
	t.Cleanup(gts.ResetParseEnvConfigCacheForTests)
	lang := grammars.RequirementsLanguage()
	p := gts.NewParser(lang)
	tree, err := p.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("nil tree/root")
	}
	t.Cleanup(tree.Release)
	return tree, lang
}

func assertNodeShape(t *testing.T, n *gts.Node, lang *gts.Language, wantType string, wantStart, wantEnd uint32, path string) {
	t.Helper()
	if n == nil {
		t.Fatalf("%s: nil node (want %s [%d:%d])", path, wantType, wantStart, wantEnd)
	}
	if got := n.Type(lang); got != wantType {
		t.Errorf("%s: type=%q want %q", path, got, wantType)
	}
	if n.StartByte() != wantStart || n.EndByte() != wantEnd {
		t.Errorf("%s: span [%d:%d] want [%d:%d]", path, n.StartByte(), n.EndByte(), wantStart, wantEnd)
	}
}

// TestRequirementsTrailingCommentRecoveryMatchesC pins the C oracle shape for
// the minimal recovery reproducer.
func TestRequirementsTrailingCommentRecoveryMatchesC(t *testing.T) {
	src := "pkg ; python_version >= '3.13'  # note\n"
	tree, lang := reqRecoverParse(t, src)
	root := tree.RootNode()

	assertNodeShape(t, root, lang, "file", 0, 39, "root")
	if got := root.ChildCount(); got != 2 {
		t.Fatalf("root child count = %d, want 2 (requirement, ERROR); tree:\n%s", got, dumpTreeForTest(root, lang, []byte(src), 0))
	}
	req := root.Child(0)
	assertNodeShape(t, req, lang, "requirement", 0, 30, "root.child[0]")
	errNode := root.Child(1)
	assertNodeShape(t, errNode, lang, "ERROR", 30, 38, "root.child[1]")
	if got := errNode.ChildCount(); got != 1 {
		t.Fatalf("ERROR child count = %d, want 1 (comment); tree:\n%s", got, dumpTreeForTest(root, lang, []byte(src), 0))
	}
	assertNodeShape(t, errNode.Child(0), lang, "comment", 32, 38, "root.child[1].child[0]")

	if t.Failed() {
		t.Logf("full tree:\n%s", dumpTreeForTest(root, lang, []byte(src), 0))
	}
}

// TestRequirementsCleanParseUnchanged guards that the recovery gate does not
// disturb clean requirements parses.
func TestRequirementsCleanParseUnchanged(t *testing.T) {
	src := "pkg ; python_version >= '3.13'\n# note\n"
	tree, lang := reqRecoverParse(t, src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("clean source produced error tree:\n%s", dumpTreeForTest(root, lang, []byte(src), 0))
	}
	assertNodeShape(t, root, lang, "file", 0, uint32(len(src)), "root")
	if got := root.ChildCount(); got != 2 {
		t.Fatalf("root child count = %d, want 2 (requirement, comment); tree:\n%s", got, dumpTreeForTest(root, lang, []byte(src), 0))
	}
	assertNodeShape(t, root.Child(0), lang, "requirement", 0, 30, "root.child[0]")
	assertNodeShape(t, root.Child(1), lang, "comment", 31, 37, "root.child[1]")
}

func dumpTreeForTest(n *gts.Node, lang *gts.Language, src []byte, depth int) string {
	if n == nil {
		return ""
	}
	out := ""
	for i := 0; i < depth; i++ {
		out += "  "
	}
	s, e := n.StartByte(), n.EndByte()
	txt := ""
	if int(e) <= len(src) && e-s <= 60 {
		txt = " " + string(src[s:e])
	}
	out += n.Type(lang) + " [" + uitoaTest(s) + ":" + uitoaTest(e) + "]" + txt + "\n"
	for i := 0; i < n.ChildCount(); i++ {
		out += dumpTreeForTest(n.Child(i), lang, src, depth+1)
	}
	return out
}

func uitoaTest(v uint32) string {
	if v == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
