package grammars_test

import (
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestPurescriptDeriveInstanceWhereLayout(t *testing.T) {
	cases := []struct{ source, want string }{
		{"derive instance t::F i\ninstance p::A where p=(x)\ni::g", "(purescript (derive_declaration (instance_name) (type_name (type)) (type_name (type_variable))) (class_instance (instance_name) (instance_head (class_name (type))) (where) (function (variable) (exp_parens (exp_name (variable))))) (signature (variable) (type_name (type_variable))))"},
		{"derive instance u::F t\ninstance a::L where p(L)=(x)\ns::r", "(purescript (derive_declaration (instance_name) (type_name (type)) (type_name (type_variable))) (class_instance (instance_name) (instance_head (class_name (type))) (where) (function (variable) (patterns (pat_parens (pat_name (constructor)))) (exp_parens (exp_name (variable))))) (signature (variable) (type_name (type_variable))))"},
		{"derive instance t::F i\ninstance p::A where\n  p=(x)\ni::g", "(purescript (derive_declaration (instance_name) (type_name (type)) (type_name (type_variable))) (class_instance (instance_name) (instance_head (class_name (type))) (where) (function (variable) (exp_parens (exp_name (variable))))) (signature (variable) (type_name (type_variable))))"},
		{"derive instance t::F i\ninstance p::A where p=f x\ni::g", "(purescript (derive_declaration (instance_name) (type_name (type)) (type_name (type_variable))) (class_instance (instance_name) (instance_head (class_name (type))) (where) (function (variable) (exp_apply (exp_name (variable)) (exp_name (variable))))) (signature (variable) (type_name (type_variable))))"},
	}
	for _, tc := range cases {
		for _, compact := range []bool{false, true} {
			t.Run(fmt.Sprintf("compact=%t/%s", compact, tc.source), func(t *testing.T) {
				lang := grammars.PurescriptLanguage()
				parser := gotreesitter.NewParser(lang)
				parser.SetAdmissionCandidateRoute(compact)
				tree, err := parser.Parse([]byte(tc.source))
				if err != nil {
					t.Fatal(err)
				}
				defer func() { tree.Release() }()
				root := tree.RootNode()
				if root == nil {
					t.Fatal("missing root")
				}
				if root.HasError() {
					t.Fatalf("unexpected error: %s", root.SExpr(lang))
				}
				if got := root.SExpr(lang); got != tc.want {
					t.Fatalf("tree = %s, want %s", got, tc.want)
				}
				if root.StartByte() != 0 || root.EndByte() != uint32(len(tc.source)) {
					t.Fatalf("root range = %d..%d", root.StartByte(), root.EndByte())
				}
				instance := root.NamedChild(1)
				if got, want := instance.EndByte(), uint32(strings.LastIndexByte(tc.source, '\n')); got != want {
					t.Fatalf("instance end = %d, want %d", got, want)
				}
				unchanged := []byte(tc.source)
				if allocs := testing.AllocsPerRun(100, func() {
					next, err := parser.ParseIncremental(unchanged, tree)
					if err != nil {
						panic(err)
					}
					next.Release()
				}); allocs != 0 {
					t.Fatalf("no-edit allocations = %g", allocs)
				}
				source := []byte(tc.source)
				for _, replacement := range []byte{'Z', 'g', 'r'} {
					index := len(source) - 1
					row := uint32(strings.Count(string(source[:index]), "\n"))
					column := uint32(index - strings.LastIndexByte(string(source[:index]), '\n') - 1)
					edit := gotreesitter.InputEdit{StartByte: uint32(index), OldEndByte: uint32(index + 1), NewEndByte: uint32(index + 1), StartPoint: gotreesitter.Point{Row: row, Column: column}, OldEndPoint: gotreesitter.Point{Row: row, Column: column + 1}, NewEndPoint: gotreesitter.Point{Row: row, Column: column + 1}}
					tree.Edit(edit)
					source[index] = replacement
					next, err := parser.ParseIncremental(source, tree)
					if err != nil {
						t.Fatal(err)
					}
					fresh, err := parser.Parse(source)
					if err != nil {
						next.Release()
						t.Fatal(err)
					}
					comparePurescriptLayoutNodes(t, lang, next.RootNode(), fresh.RootNode())
					fresh.Release()
					if tree != next {
						tree.Release()
					}
					tree = next
				}

			})
		}
	}
}

func comparePurescriptLayoutNodes(t *testing.T, lang *gotreesitter.Language, incremental, fresh *gotreesitter.Node) {
	t.Helper()
	if incremental == nil || fresh == nil {
		t.Fatal("missing root after edit")
	}
	if incremental.Type(lang) != fresh.Type(lang) || incremental.StartByte() != fresh.StartByte() || incremental.EndByte() != fresh.EndByte() || incremental.StartPoint() != fresh.StartPoint() || incremental.EndPoint() != fresh.EndPoint() || incremental.ChildCount() != fresh.ChildCount() || incremental.HasError() != fresh.HasError() || incremental.IsNamed() != fresh.IsNamed() || incremental.IsExtra() != fresh.IsExtra() || incremental.IsMissing() != fresh.IsMissing() {
		t.Fatalf("incremental differs from fresh: %s / %s", incremental.SExpr(lang), fresh.SExpr(lang))
	}
	for i := 0; i < incremental.ChildCount(); i++ {
		if got, want := incremental.FieldNameForChild(i, lang), fresh.FieldNameForChild(i, lang); got != want {
			t.Fatalf("child %d field = %q, want %q", i, got, want)
		}
		comparePurescriptLayoutNodes(t, lang, incremental.Child(i), fresh.Child(i))
	}
}
