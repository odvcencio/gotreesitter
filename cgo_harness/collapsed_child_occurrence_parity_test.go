//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type collapsedChildParityRow struct {
	parent     string
	child      string
	childNamed bool
}

// TestCollapsedChildOccurrenceParity pins the exact C-tree shape for every
// language represented by the collapsed named-leaf occurrence ledger. The
// fixtures collectively exercise all 24 registered parent/raw-child pairs;
// run subtests one language at a time in Docker.
func TestCollapsedChildOccurrenceParity(t *testing.T) {
	tests := []struct {
		name   string
		source string
		rows   []collapsedChildParityRow
	}{
		{name: "ruby", source: "%w(foo)\n%i(bar)\n", rows: []collapsedChildParityRow{
			{parent: "bare_string", child: "string_content", childNamed: true},
			{parent: "bare_symbol", child: "string_content", childNamed: true},
		}},
		{name: "kotlin", source: "package a\nimport d.*\n", rows: []collapsedChildParityRow{
			{parent: "identifier", child: "simple_identifier", childNamed: true},
		}},
		{name: "hack", source: "<?hh function f(): void { $a = true; $b = false; $c = null; }\n", rows: []collapsedChildParityRow{
			{parent: "true", child: "true"}, {parent: "false", child: "false"}, {parent: "null", child: "null"},
		}},
		{name: "dart", source: "class A { void f(){ this.f(); } } class B extends A { B(): super(); }\n", rows: []collapsedChildParityRow{
			{parent: "this", child: "this"}, {parent: "super", child: "super"},
		}},
		{name: "elixir", source: "nil\n", rows: []collapsedChildParityRow{{parent: "nil", child: "nil"}}},
		{name: "rust", source: "fn f(s: &str) -> &str { &s[..] }\n", rows: []collapsedChildParityRow{{parent: "range_expression", child: ".."}}},
		{name: "apex", source: `trigger T on Account (before insert, before update, before delete, after insert, after update, after delete, after undelete) { System.debug(UserInfo.getUserId()); }
public class C { public C(){ super(); } void f(Account a, User u) { insert a; delete a; undelete a; upsert a; insert as user a; update as system a; System.runAs(u) { } } }
`, rows: []collapsedChildParityRow{
			{parent: "after_delete", child: "after_delete"}, {parent: "after_insert", child: "after_insert"},
			{parent: "after_undelete", child: "after_undelete"}, {parent: "after_update", child: "after_update"},
			{parent: "before_delete", child: "before_delete"}, {parent: "before_insert", child: "before_insert"},
			{parent: "before_update", child: "before_update"}, {parent: "delete", child: "delete"},
			{parent: "insert", child: "insert"}, {parent: "super", child: "super"},
			{parent: "system", child: "system"}, {parent: "undelete", child: "undelete"},
			{parent: "upsert", child: "upsert"}, {parent: "user", child: "user"},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)
			runParityCase(t, parityCase{name: test.name}, "collapsed-child-occurrence", source)
			assertCollapsedChildOccurrenceCensus(t, test.name, source, test.rows)
		})
	}
}

func assertCollapsedChildOccurrenceCensus(t *testing.T, language string, source []byte, rows []collapsedChildParityRow) {
	t.Helper()
	goTree, goLang, err := parseWithGo(parityCase{name: language}, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseGoTree(goTree)

	cLang, err := ParityCLanguage(language)
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("C oracle returned nil tree")
	}
	defer cTree.Close()

	for _, row := range rows {
		goCount := countGoCollapsedChildOccurrences(goTree.RootNode(), goLang, row)
		cCount := countCCollapsedChildOccurrences(cTree.RootNode(), row)
		if goCount == 0 || cCount == 0 {
			t.Fatalf("%s/%s pair occurrence census go=%d c=%d; fixture no longer exercises the ledger row", row.parent, row.child, goCount, cCount)
		}
		if goCount != cCount {
			t.Fatalf("%s/%s pair occurrence census go=%d c=%d", row.parent, row.child, goCount, cCount)
		}
	}
}

func countGoCollapsedChildOccurrences(node *gts.Node, lang *gts.Language, row collapsedChildParityRow) int {
	if node == nil {
		return 0
	}
	count := 0
	if node.IsNamed() && node.Type(lang) == row.parent && node.ChildCount() == 1 {
		child := node.Child(0)
		if child != nil && child.Type(lang) == row.child && child.IsNamed() == row.childNamed {
			count++
		}
	}
	for i := 0; i < node.ChildCount(); i++ {
		count += countGoCollapsedChildOccurrences(node.Child(i), lang, row)
	}
	return count
}

func countCCollapsedChildOccurrences(node *sitter.Node, row collapsedChildParityRow) int {
	if node == nil {
		return 0
	}
	count := 0
	if node.IsNamed() && node.Kind() == row.parent && node.ChildCount() == 1 {
		child := node.Child(0)
		if child != nil && child.Kind() == row.child && child.IsNamed() == row.childNamed {
			count++
		}
	}
	for i := uint(0); i < node.ChildCount(); i++ {
		count += countCCollapsedChildOccurrences(node.Child(i), row)
	}
	return count
}
