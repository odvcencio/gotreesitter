package grammarruntime

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestGraphQLBlockStringEscapedTripleQuoteTokenizedAsContent(t *testing.T) {
	lang := GraphqlLanguage()
	src := []byte("\"\"\"\nblock string uses \\\"\"\"\n\"\"\"")

	quoteSym, ok := lang.SymbolByName("\"\"\"")
	if !ok {
		t.Fatal("graphql language missing triple-quote token")
	}

	ts := &GenericTokenSource{
		src:            src,
		lang:           lang,
		cur:            newSourceCursor(src),
		tripleQuoteSym: quoteSym,
	}

	open, ok := ts.scanGraphQLBlockString(quoteSym, 0)
	if !ok {
		t.Fatal("scanGraphQLBlockString returned false")
	}
	if open.Symbol != quoteSym || open.StartByte != 0 || open.EndByte != 3 {
		t.Fatalf("open token = %s [%d:%d], want triple quote [0:3]", open.Text, open.StartByte, open.EndByte)
	}
	if len(ts.pending) != 1 {
		t.Fatalf("pending token count = %d, want exactly final close token: %#v", len(ts.pending), ts.pending)
	}
	close := ts.pending[0]
	if close.Symbol != quoteSym || close.Text != "\"\"\"" || close.StartByte != uint32(len(src)-3) {
		t.Fatalf("close token = %s [%d:%d], want final triple quote", close.Text, close.StartByte, close.EndByte)
	}
}

func TestGraphQLParseBlockStringEscapedTripleQuote(t *testing.T) {
	lang := GraphqlLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("fragment frag on Friend {\n  foo(obj: { block: \"\"\"\n    block string uses \\\"\"\"\n    \"\"\" })\n}\n")

	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	defer tree.Release()

	root := tree.RootNode()
	if root.Type(lang) != "source_file" {
		t.Fatalf("root type = %q, want source_file; tree: %s", root.Type(lang), root.SExpr(lang))
	}
	if !root.HasError() {
		t.Fatalf("expected localized C-oracle recovery error, got clean tree: %s", root.SExpr(lang))
	}
	if root.ChildCount() != 1 || root.Child(0).Type(lang) != "document" {
		t.Fatalf("expected recovered document definition, got: %s", root.SExpr(lang))
	}
	doc := root.Child(0)
	if doc.ChildCount() != 1 || doc.Child(0).Type(lang) != "definition" {
		t.Fatalf("expected recovered document definition, got: %s", root.SExpr(lang))
	}
}
