package gotreesitter_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Golden hashes of golang.org/x/sys/windows/zerrors_windows.go (a 945KB single
// giant const block) parsed with GOT_C_RECOVERY=0. They pin the exact tree the
// go result normalizer produces so the O(n^2) hardening in
// parser_result_go_compat.go (lazy child-count + deduped cyclic-descent
// fallback) stays byte-for-byte output-preserving. Captured before the change;
// re-verified identical after.
//
// zerrorsGoldenSpans was regenerated when goNodeSpansHash started folding each
// child's field name into the hash (review C finding #5): the produced TREE is
// unchanged — zerrorsGoldenSExpr, which is independent of goNodeSpansHash, was
// re-verified identical across that change — only the hash function gained an
// extra `field=%q` line per child edge, which changes the digest.
const (
	zerrorsGoldenSExpr = "e32b333c4b72bd386018f54468c9ad0353409060186e66d308903c1ba745174f"
	zerrorsGoldenSpans = "a8c6810c0da9415dcece890ee67b3b3bfe85f6f0c6455f2de4e04f5134b2c153"
)

var zerrorsCandidatePaths = []string{
	"/home/draco/work/gotreesitter-corpora/corpus_sources/go/src/cmd/vendor/golang.org/x/sys/windows/zerrors_windows.go",
	"/home/draco/work/gotreesitter-corpora/corpus_sources/fidl/third_party/golibs/vendor/golang.org/x/sys/windows/zerrors_windows.go",
}

// goNodeSpansHash hashes the tree shape by type/named/span AND, per child
// edge, the field name assigned to that child (via FieldNameForChild). Folding
// field assignment into the hash means field drift (a child silently gaining,
// losing, or changing its field) changes the golden hash instead of passing
// silently — the prior span-only hash could not detect that class of
// regression.
func goNodeSpansHash(lang *gotreesitter.Language, root *gotreesitter.Node) string {
	h := sha256.New()
	var walk func(n *gotreesitter.Node)
	walk = func(n *gotreesitter.Node) {
		if n == nil {
			return
		}
		named := byte(0)
		if n.IsNamed() {
			named = 1
		}
		sp, ep := n.StartPoint(), n.EndPoint()
		fmt.Fprintf(h, "%s|%d|%d|%d|%d:%d|%d:%d\n", n.Type(lang), named,
			n.StartByte(), n.EndByte(), sp.Row, sp.Column, ep.Row, ep.Column)
		for i := 0; i < n.ChildCount(); i++ {
			fmt.Fprintf(h, "field=%q\n", n.FieldNameForChild(i, lang))
			walk(n.Child(i))
		}
	}
	walk(root)
	return hex.EncodeToString(h.Sum(nil))
}

// TestGoZerrorsNormalizerByteIdentity re-parses the giant-const-block corpus with
// recovery off and fails if the normalizer output drifts from the pinned golden.
func TestGoZerrorsNormalizerByteIdentity(t *testing.T) {
	t.Setenv("GOT_C_RECOVERY", "0")
	gotreesitter.ResetParseEnvConfigCacheForTests()
	t.Cleanup(gotreesitter.ResetParseEnvConfigCacheForTests)

	var src []byte
	var err error
	for _, p := range zerrorsCandidatePaths {
		if src, err = os.ReadFile(p); err == nil {
			break
		}
	}
	if src == nil {
		t.Skipf("zerrors_windows.go corpus not found: %v", err)
	}

	lang := grammars.GoLanguage()
	tree, perr := gotreesitter.NewParser(lang).Parse(src)
	if perr != nil {
		t.Fatalf("parse: %v", perr)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("nil root")
	}
	if got := uint32(len(src)); root.EndByte() != got {
		t.Fatalf("root.EndByte=%d want %d (truncated parse)", root.EndByte(), got)
	}
	sx := sha256.Sum256([]byte(root.SExpr(lang)))
	if gotS := hex.EncodeToString(sx[:]); gotS != zerrorsGoldenSExpr {
		t.Fatalf("SExpr drift: got %s want %s", gotS, zerrorsGoldenSExpr)
	}
	if gotS := goNodeSpansHash(lang, root); gotS != zerrorsGoldenSpans {
		t.Fatalf("spans drift: got %s want %s", gotS, zerrorsGoldenSpans)
	}
}
