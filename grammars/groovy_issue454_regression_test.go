package grammars_test

import (
	"bytes"
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestIssue454GroovyJuxtFunctionCallSingleStatement is the minimal,
// non-incremental reproduction behind TestIssue454GroovyIncrementalMatchesFresh:
// a plain fresh Parse of a command-chain call (e.g. `println "x"`, no
// parentheses) recognizes it as juxt_function_call only when it is the FIRST
// statement in a block. A second statement in the same block loses that
// derivation, even though the identical construct parses correctly at the
// top level or as a block's only statement. This is a grammar-table gap
// (grammargen/tree-sitter-groovy), not a parser-runtime defect, and it is
// unaffected by GOT_GLR_MAX_STACKS at any width (checked 2 through 32): not
// a survivor-culling artifact.
//
// This test pins the CURRENT (gap) behavior so a future grammar fix is
// visible here, and documents why languageDisablesIncrementalReuse routes
// every groovy incremental parse through a fresh full parse (parser_retry.go):
// incremental reuse's leaf-by-leaf splice can accidentally dispatch a later
// statement from a state that does support the juxtaposition, producing a
// tree a fresh parse of the same bytes never would.
func TestIssue454GroovyJuxtFunctionCallSingleStatement(t *testing.T) {
	lang := grammars.GroovyLanguage()

	tests := []struct {
		name     string
		src      string
		wantJuxt bool
	}{
		{"top_level_only_statement", "println \"hi\"\n", true},
		{"top_level_second_statement", "println \"a\"\nprintln \"hi\"\n", true},
		{"block_only_statement", "def f(a,b) {\n println \"hi\"\n}\n", true},
		{"block_second_statement_known_gap", "def f(a,b) {\n def x0 = a + b\n println \"hi\"\n}\n", false},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			tree, err := gotreesitter.NewParser(lang).Parse([]byte(test.src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			defer tree.Release()
			if tree.RootNode().HasError() {
				t.Fatalf("unexpected parse error for %q", test.src)
			}
			gotJuxt := bytes.Contains([]byte(tree.RootNode().SExpr(lang)), []byte("juxt_function_call"))
			if gotJuxt != test.wantJuxt {
				t.Fatalf("%s: juxt_function_call present = %v, want %v\nsexpr=%s", test.name, gotJuxt, test.wantJuxt, tree.RootNode().SExpr(lang))
			}
		})
	}
}

// TestIssue454GroovyIncrementalMatchesFresh is issue #454's groovy finding:
// an incremental parse gained exactly 4 extra nodes (57,370 vs a fresh
// parse's 57,366 in the downstream report) on every length-changing edit, on
// both the compact and production admission routes. Root cause: groovy's
// grammar table only recognizes a juxt_function_call (command-chain call,
// e.g. `println "x"`) as a block's FIRST statement (see
// TestIssue454GroovyJuxtFunctionCallSingleStatement); incremental reuse's
// leaf splicing through a preceding statement can accidentally land the
// following statement's dispatch in a state that does support the
// juxtaposition, gaining wrapper nodes no fresh parse of the identical bytes
// produces. The fix routes every groovy incremental parse through the same
// full-parse path Parse() uses (languageDisablesIncrementalReuse,
// parser_retry.go), trading incremental reuse performance for guaranteed
// parity with a fresh parse until the grammar table gap itself is fixed.
func TestIssue454GroovyIncrementalMatchesFresh(t *testing.T) {
	lang := grammars.GroovyLanguage()
	src := issue454GenGroovy(24 << 10)
	site := bytes.Index(src, []byte("x0"))
	if site < 0 {
		t.Fatal("fixture has no \"x0\" marker")
	}
	point := issue454PointAt(src, site)

	edits := []struct {
		name   string
		edited []byte
		edit   gotreesitter.InputEdit
	}{
		{
			name: "replace",
			edited: func() []byte {
				e := append([]byte(nil), src...)
				e[site]++
				return e
			}(),
			edit: gotreesitter.InputEdit{
				StartByte: uint32(site), OldEndByte: uint32(site + 1), NewEndByte: uint32(site + 1),
				StartPoint: point, OldEndPoint: gotreesitter.Point{Row: point.Row, Column: point.Column + 1}, NewEndPoint: gotreesitter.Point{Row: point.Row, Column: point.Column + 1},
			},
		},
		{
			name:   "insert",
			edited: append(append(append([]byte(nil), src[:site]...), src[site]), src[site:]...),
			edit: gotreesitter.InputEdit{
				StartByte: uint32(site), OldEndByte: uint32(site), NewEndByte: uint32(site + 1),
				StartPoint: point, OldEndPoint: point, NewEndPoint: gotreesitter.Point{Row: point.Row, Column: point.Column + 1},
			},
		},
		{
			name:   "delete",
			edited: append(append([]byte(nil), src[:site]...), src[site+1:]...),
			edit: gotreesitter.InputEdit{
				StartByte: uint32(site), OldEndByte: uint32(site + 1), NewEndByte: uint32(site),
				StartPoint: point, OldEndPoint: gotreesitter.Point{Row: point.Row, Column: point.Column + 1}, NewEndPoint: point,
			},
		},
	}

	for _, edit := range edits {
		edit := edit
		t.Run(edit.name, func(t *testing.T) {
			old, err := gotreesitter.NewParser(lang).Parse(src)
			if err != nil {
				t.Fatalf("old Parse: %v", err)
			}
			defer old.Release()
			old.Edit(edit.edit)

			incremental, profile, err := gotreesitter.NewParser(lang).ParseIncrementalProfiled(edit.edited, old)
			if err != nil {
				t.Fatalf("ParseIncrementalProfiled: %v", err)
			}
			defer incremental.Release()

			fresh, err := gotreesitter.NewParser(lang).Parse(edit.edited)
			if err != nil {
				t.Fatalf("fresh Parse: %v", err)
			}
			defer fresh.Release()

			incRoot, freshRoot := incremental.RootNode(), fresh.RootNode()
			if incRoot == nil || freshRoot == nil {
				t.Fatal("parse returned no root")
			}
			if got, want := incRoot.SExpr(lang), freshRoot.SExpr(lang); got != want {
				t.Fatalf("%s: incremental tree diverges from a fresh parse (profile reason=%q)", edit.name, profile.ReuseUnsupportedReason)
			}
			if incRoot.HasError() != freshRoot.HasError() {
				t.Fatalf("%s: HasError mismatch: incremental=%v fresh=%v", edit.name, incRoot.HasError(), freshRoot.HasError())
			}
		})
	}
}

func issue454GenGroovy(n int) []byte {
	var b bytes.Buffer
	for i := 0; b.Len() < n; i++ {
		fmt.Fprintf(&b, "def f%d(a, b) {\n    def x0 = a + b\n    println \"f%d ${x0}\"\n    return x0\n}\n\n", i, i)
	}
	return b.Bytes()
}
