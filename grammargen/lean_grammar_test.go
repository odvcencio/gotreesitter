package grammargen

import (
	"fmt"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestLeanGrammarCoreCommands(t *testing.T) {
	source := []byte("\ndef x := 1\ntheorem y : True := by trivial\n")
	lang, err := GenerateLanguage(LeanGrammar())
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse(source)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("parse has error:\n%s\ntree: %s", leanNodeDump(root, lang, source, 0), root.SExpr(lang))
	}
}

func leanNodeDump(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, depth int) string {
	if node == nil {
		return ""
	}
	start, end := int(node.StartByte()), int(node.EndByte())
	if start < 0 || end < start || end > len(source) {
		start, end = 0, 0
	}
	var out strings.Builder
	fmt.Fprintf(&out, "%s%s [%d:%d] %q error=%v missing=%v extra=%v\n",
		strings.Repeat("  ", depth), node.Type(lang), start, end, source[start:end],
		node.IsError(), node.IsMissing(), node.IsExtra())
	for i := 0; i < node.ChildCount(); i++ {
		out.WriteString(leanNodeDump(node.Child(i), lang, source, depth+1))
	}
	return out.String()
}
