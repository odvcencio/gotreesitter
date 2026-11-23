//go:build cgo && treesitter_c_parity && gts_ccore_test_certify

package cgoharness

import (
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestJavaScriptForestReductionDepth(t *testing.T) {
	grantJavaScriptForestForTest(t)
	lang, ref := ccoreLanguages(t, "javascript")
	oracle := sitter.NewParser()
	defer oracle.Close()
	if err := oracle.SetLanguage(ref); err != nil {
		t.Fatal(err)
	}
	for depth := 1; depth <= 24; depth++ {
		var text strings.Builder
		text.WriteString("function f() {\n  if (outer) {\n")
		for i := 0; i < depth; i++ {
			fmt.Fprintf(&text, "    if (x%d) { g(); } else\n", i)
		}
		text.WriteString("    {\n      if (y) { return 1; }\n      z[0] = ")
		text.WriteByte(96)
		text.WriteByte(36)
		text.WriteString("{x}{")
		text.WriteByte(96)
		text.WriteString(";\n    }\n  }\n  if (after) g();\n}\n")
		source := []byte(text.String())
		want := oracle.Parse(source, nil)
		if want == nil || want.RootNode().HasError() {
			t.Fatalf("depth %d: pinned C rejected the source", depth)
		}
		p := gts.NewParser(lang)
		before, _, beforeAbstain := bind.ForestCounters()
		tree, ok := p.ParseForestExperimental(source)
		after, _, afterAbstain := bind.ForestCounters()
		if after-before != 1 || afterAbstain != beforeAbstain {
			t.Errorf("depth %d actual compact forest service = %d abstains = %d", depth, after-before, afterAbstain-beforeAbstain)
		}
		equal := tree != nil && ccoreLegacyDigest(tree, lang) == ccoreCDigest(want)
		if tree != nil {
			if diff := completeForestCIdentityDiff(tree.RootNode(), lang, want.RootNode()); diff != "" {
				t.Errorf("depth %d complete identity: %s", depth, diff)
			}
		}
		at, token, why, states := p.ForestDeclineInfo()
		t.Logf("DEPTH depth=%d accept=%t equal=%t bytes=%d offset=%d token=%d reason=%s states=%v",
			depth, ok, equal, len(source), at, token, why, states)
		want.Close()
		if tree != nil {
			tree.Release()
		}
		if !ok || !equal {
			t.Errorf("depth %d: accepted tree differs from pinned C", depth)
		}
	}
}
