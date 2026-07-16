package smacker_test

import (
	"context"
	"testing"

	sitter "github.com/odvcencio/gotreesitter/compat/smacker"
	"github.com/odvcencio/gotreesitter/compat/smacker/golang"
	"github.com/odvcencio/gotreesitter/compat/smacker/python"
)

const goSource = `package main

func add(a int, b int) int {
	return a + b
}
`

// TestParseAndNavigate exercises the smacker-shaped Node surface real consumers
// (e.g. DeepSource/globstar) call: package ParseCtx, Type, ChildByFieldName,
// NamedChild(Count), Content, Range, and IsNull.
func TestParseAndNavigate(t *testing.T) {
	lang := golang.GetLanguage()
	root, err := sitter.ParseCtx(context.Background(), []byte(goSource), lang)
	if err != nil {
		t.Fatalf("ParseCtx: %v", err)
	}
	if root.IsNull() {
		t.Fatal("root is null")
	}
	if got := root.Type(); got != "source_file" {
		t.Fatalf("root type = %q, want source_file", got)
	}

	// Find the function_declaration and read its name field.
	var fn *sitter.Node
	for i := uint32(0); i < root.NamedChildCount(); i++ {
		if c := root.NamedChild(int(i)); c.Type() == "function_declaration" {
			fn = c
			break
		}
	}
	if fn == nil {
		t.Fatal("no function_declaration found")
	}
	name := fn.ChildByFieldName("name")
	if name.IsNull() {
		t.Fatal("function name field is null")
	}
	if got := name.Content([]byte(goSource)); got != "add" {
		t.Fatalf("function name = %q, want add", got)
	}

	// Range and points are threaded through the wrapper.
	r := name.Range()
	if r.StartByte >= r.EndByte {
		t.Fatalf("bad range %+v", r)
	}
	if name.StartPoint().Row != 2 {
		t.Fatalf("name start row = %d, want 2", name.StartPoint().Row)
	}

	// Parent navigation returns a threaded node, and Equal identifies it.
	if !name.Parent().Equal(fn) {
		t.Fatal("name.Parent() should Equal the function node")
	}
}

// TestQueryCaptures exercises the query path globstar depends on:
// NewQuery([]byte,lang), NewQueryCursor, cursor.Exec(query, node),
// cursor.NextMatch, and query.CaptureNameForId(capture.Index).
func TestQueryCaptures(t *testing.T) {
	lang := golang.GetLanguage()
	root, err := sitter.ParseCtx(context.Background(), []byte(goSource), lang)
	if err != nil {
		t.Fatalf("ParseCtx: %v", err)
	}
	query, err := sitter.NewQuery([]byte(`(function_declaration name: (identifier) @fn.name)`), lang)
	if err != nil {
		t.Fatalf("NewQuery: %v", err)
	}
	defer query.Close()

	cursor := sitter.NewQueryCursor()
	defer cursor.Close()
	cursor.Exec(query, root)

	var names []string
	for {
		m, ok := cursor.NextMatch()
		if !ok {
			break
		}
		for _, capture := range m.Captures {
			if query.CaptureNameForId(capture.Index) == "fn.name" {
				names = append(names, capture.Node.Content([]byte(goSource)))
			}
		}
	}
	if len(names) != 1 || names[0] != "add" {
		t.Fatalf("captured names = %v, want [add]", names)
	}
}

// TestNullSafety confirms the synthesized IsNull contract on absent nodes.
func TestNullSafety(t *testing.T) {
	lang := python.GetLanguage()
	root, err := sitter.ParseCtx(context.Background(), []byte("x = 1\n"), lang)
	if err != nil {
		t.Fatalf("ParseCtx: %v", err)
	}
	missing := root.ChildByFieldName("no_such_field")
	if !missing.IsNull() {
		t.Fatal("missing field should be null")
	}
	// Every accessor is null-safe.
	if missing.Type() != "" || missing.ChildCount() != 0 || missing.NamedChild(0) != nil {
		t.Fatal("null node accessors should be safe")
	}
}

// TestIncrementalParser exercises the stateful Parser/Tree path used by other
// smacker consumers (SetLanguage, ParseCtx with a tree, RootNode).
func TestIncrementalParser(t *testing.T) {
	p := sitter.NewParser()
	p.SetLanguage(golang.GetLanguage())
	defer p.Close()

	tree, err := p.ParseCtx(context.Background(), nil, []byte(goSource))
	if err != nil {
		t.Fatalf("Parser.ParseCtx: %v", err)
	}
	defer tree.Close()
	if tree.RootNode().Type() != "source_file" {
		t.Fatal("root type mismatch via Parser path")
	}
}

// TestSiblingsAndFilter covers PrevSibling/NextSibling and the FilterPredicates
// compatibility passthrough used by globstar's checkers.
func TestSiblingsAndFilter(t *testing.T) {
	lang := golang.GetLanguage()
	root, err := sitter.ParseCtx(context.Background(), []byte(goSource), lang)
	if err != nil {
		t.Fatalf("ParseCtx: %v", err)
	}
	// package_clause is the first child; its next sibling is the func decl.
	first := root.NamedChild(0)
	if first.IsNull() {
		t.Fatal("no first child")
	}
	sib := first.NextSibling()
	if sib.IsNull() || sib.PrevSibling() == nil || !sib.PrevSibling().Equal(first) {
		t.Fatal("sibling navigation is not symmetric")
	}

	query, err := sitter.NewQuery([]byte(`(function_declaration) @f`), lang)
	if err != nil {
		t.Fatalf("NewQuery: %v", err)
	}
	cursor := sitter.NewQueryCursor()
	cursor.Exec(query, root)
	m, ok := cursor.NextMatch()
	if !ok {
		t.Fatal("expected a match")
	}
	if got := cursor.FilterPredicates(m, []byte(goSource)); got != m {
		t.Fatal("FilterPredicates should pass the match through unchanged")
	}
}
