package parserresult_test

import (
	"strings"
	"testing"
)

func TestRubyBinaryMinusBeforeIdentifierParses(t *testing.T) {
	const src = "def formats(values)\n  invalid_values = values - other\nend\n"
	tree, lang := parseByLanguageName(t, "ruby", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected ruby parse error: %s", root.SExpr(lang))
	}
	if !strings.Contains(root.SExpr(lang), "(binary (identifier) (identifier))") {
		t.Fatalf("ruby binary minus missing binary node: %s", root.SExpr(lang))
	}
}
