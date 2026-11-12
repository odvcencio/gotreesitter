package parse_test

// Copies of declarations from root test files that stay in the root package.
// Each entry names its source file in a "// from" comment. The root package
// keeps the originals for the tests that remain there.

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// from parser_go_newmake_edge_test.go
// findAllNodesOfType does a depth-first traversal collecting every node
// (named or not) with the given Type() in source order. It's the multi-match
// counterpart to findNamedChild (parser_go_test.go), needed here because
// several of these regression fixtures deliberately contain more than one
// call_expression and the tests must pin each one independently.
func findAllNodesOfType(lang *gotreesitter.Language, node *gotreesitter.Node, typeName string) []*gotreesitter.Node {
	var out []*gotreesitter.Node
	var walk func(n *gotreesitter.Node)
	walk = func(n *gotreesitter.Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == typeName {
			out = append(out, n)
		}
		for i := 0; i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(node)
	return out
}

// from parser_go_newmake_edge_test.go
// assertNoErrorOrMissing walks the whole tree and fails the test if any
// ERROR or MISSING node is found anywhere, printing the S-expression for
// diagnosis. Modeled on the existing GoInterpretedStringFalseErrors /
// GoWhitespaceAdjacentImmediateStringToken / GoIfElseSameLineKeywordPromotion
// walk-assertion pattern in parser_go_test.go.
func assertNoErrorOrMissing(t *testing.T, lang *gotreesitter.Language, root *gotreesitter.Node, label string) {
	t.Helper()
	if root.HasError() {
		t.Fatalf("%s: root.HasError()==true:\n%s", label, root.SExpr(lang))
	}
	var walk func(n *gotreesitter.Node)
	walk = func(n *gotreesitter.Node) {
		if n == nil {
			return
		}
		if n.IsError() || n.IsMissing() {
			t.Fatalf("%s: found ERROR/MISSING node:\n%s", label, root.SExpr(lang))
		}
		for i := 0; i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
}

// from parser_go_test.go
// parseGo is a test helper that creates a parser, lexes and parses Go source.
// Uses the custom GoTokenSource when the current Go blob is ts2go-compiled
// (detected by presence of the `source_file_token1` anonymous composite
// symbol); otherwise falls back to the DFA baked into the grammargen blob.
func parseGo(t *testing.T, src string) (*gotreesitter.Tree, *gotreesitter.Language) {
	t.Helper()
	lang := grammars.GoLanguage()
	parser := gotreesitter.NewParser(lang)
	srcBytes := []byte(src)
	var (
		tree *gotreesitter.Tree
		err  error
	)
	if _, ok := lang.SymbolByName("source_file_token1"); ok {
		ts := mustGoTokenSource(t, srcBytes, lang)
		tree, err = parser.ParseWithTokenSource(srcBytes, ts)
	} else {
		tree, err = parser.Parse(srcBytes)
	}
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	return tree, lang
}

// from parser_typescript_merge_cap_subsumption_test.go
// ironwoodParseNoFatal parses source with the named grammar and returns the
// tree and language without failing on a parse-level ERROR, so the caller can
// assert the error state itself.
func ironwoodParseNoFatal(t *testing.T, name, src string) (*gotreesitter.Tree, *gotreesitter.Language) {
	t.Helper()
	var entry grammars.LangEntry
	var report grammars.ParseSupport
	for _, e := range grammars.AllLanguages() {
		if e.Name == name {
			entry = e
		}
	}
	for _, r := range grammars.AuditParseSupport() {
		if r.Name == name {
			report = r
		}
	}
	lang := entry.Language()
	parser := gotreesitter.NewParser(lang)
	srcBytes := []byte(src)
	var tree *gotreesitter.Tree
	var err error
	if report.Backend == grammars.ParseBackendTokenSource {
		tree, err = parser.ParseWithTokenSource(srcBytes, entry.TokenSourceFactory(srcBytes, lang))
	} else {
		tree, err = parser.Parse(srcBytes)
	}
	if err != nil {
		t.Fatalf("%s parse failed: %v", name, err)
	}
	return tree, lang
}
