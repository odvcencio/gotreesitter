//go:build !grammar_subset || grammar_subset_authzed

package grammarruntime

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// testdata/authzed_regenerated.bin is a fresh conversion of the pinned
// authzed parser.c (tree-sitter-authzed 83e5c26a8687, src/parser.c) by the
// converter of this commit:
//
//	go run ./cmd/ts2go -input <checkout>/src/parser.c -output authzed.go -name authzed
//
// The shipped blob, converted before July 2026, names the NUL token "\\0"; the
// regenerated blob names it with the NUL byte. The token source must find the
// token under either name: without it the lexer never emits the NUL token at
// end of input, and the regenerated blob leaves an error in 7 of the 8 inputs
// that a November 2026 audit compared with the pinned C parser.
func TestAuthzedNullTokenInBothNameSpellings(t *testing.T) {
	regenerated, err := os.ReadFile("testdata/authzed_regenerated.bin")
	if err != nil {
		t.Fatal(err)
	}
	langs := map[string]*gotreesitter.Language{}
	for _, side := range []struct {
		name     string
		blob     []byte
		spelling string
	}{
		{"shipped", BlobByName("authzed"), "\\0"},
		{"regenerated", regenerated, "\x00"},
	} {
		lang, err := LoadLanguage("authzed", side.blob)
		if err != nil {
			t.Fatalf("%s: %v", side.name, err)
		}
		syms := lang.TokenSymbolsByName(side.spelling)
		if len(syms) != 1 {
			t.Fatalf("%s blob: %d tokens named %q, want 1", side.name, len(syms), side.spelling)
		}
		ts, err := NewAuthzedTokenSource(nil, lang)
		if err != nil {
			t.Fatalf("%s: %v", side.name, err)
		}
		if ts.nullSym != syms[0] {
			t.Fatalf("%s blob: the token source's NUL token is %d, want %d (%q)", side.name, ts.nullSym, syms[0], side.spelling)
		}
		langs[side.name] = lang
	}

	// Both blobs give the same tree, node for node.
	schema := "caveat foo(someParam int) {\n\tsomeParam == 42\n}\n\n" +
		"definition document {\n" +
		"\trelation viewer: user | user:*\n" +
		"\trelation editor: user | group#member with foo\n" +
		"\tpermission edit = editor\n" +
		"\tpermission view = viewer + edit + parent->view\n" +
		"}\n"
	inputs := []string{
		"definition user {}\n",
		"definition user {}",
		"// caveat\ndefinition d_caveat {}",
		"// teams\ndefinition teams {\n\trelation d_caveat: d_caveat\n}",
		schema,
		strings.TrimSuffix(schema, "\n"),
	}
	for i, input := range inputs {
		shipped := authzedTreeRecords(t, langs["shipped"], input)
		regen := authzedTreeRecords(t, langs["regenerated"], input)
		if shipped != regen {
			t.Errorf("input %d %q: the trees differ\nshipped:\n%s\nregenerated:\n%s", i, input, shipped, regen)
		}
		if strings.Contains(regen, " error") {
			t.Errorf("input %d %q: the regenerated blob's tree has an error:\n%s", i, input, regen)
		}
	}
}

// authzedTreeRecords lists every node with its type, span and flags. The NUL
// token's name is written as NUL in both spellings.
func authzedTreeRecords(t *testing.T, lang *gotreesitter.Language, input string) string {
	t.Helper()
	src := []byte(input)
	tree, err := gotreesitter.NewParser(lang).ParseWithTokenSource(src, NewAuthzedTokenSourceOrEOF(src, lang))
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	var b strings.Builder
	var walk func(n *gotreesitter.Node, depth int)
	walk = func(n *gotreesitter.Node, depth int) {
		typ := n.Type(lang)
		if typ == "\\0" || typ == "\x00" {
			typ = "NUL"
		}
		fmt.Fprintf(&b, "%s%s [%d,%d) named=%t", strings.Repeat("  ", depth), typ, n.StartByte(), n.EndByte(), n.IsNamed())
		if n.IsMissing() {
			b.WriteString(" missing")
		}
		if n.HasError() {
			b.WriteString(" error")
		}
		b.WriteByte('\n')
		for i := 0; i < n.ChildCount(); i++ {
			walk(n.Child(i), depth+1)
		}
	}
	walk(tree.RootNode(), 0)
	return b.String()
}
