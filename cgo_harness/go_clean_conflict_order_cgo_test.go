//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestGoCleanConflictOrderLockedC(t *testing.T) {
	full, err := os.ReadFile("../testdata/incremental_gate/go_print.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name   string
		source []byte
	}{
		{"minimal", []byte("package p\nfunc f(){ g(reflect.ValueOf(v)) }\n")},
		{"generic_instantiation", []byte("package p\n\ntype Foo[T any] struct {\n\tV T\n}\n\nfunc f() {\n\ta := Foo[int]{}\n\tb := Foo[int](a)\n\t_ = a\n\t_ = b\n}\n")},
		{"full", full},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(loadCanonicalGoCLanguage(t)); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(fixture.source, nil)
			if cTree == nil {
				t.Fatal("C returned no tree")
			}
			defer cTree.Close()
			if cTree.RootNode().HasError() {
				t.Fatal("C fixture has errors")
			}
			if fixture.name == "minimal" {
				t.Logf("C tree: %s", cTree.RootNode().ToSexp())
			}
			for _, route := range []string{"production", "forest"} {
				t.Run(route, func(t *testing.T) {
					language := grammars.GoLanguage()
					parser := gts.NewParser(language)
					parser.SetAdmissionCandidateRoute(false)
					var tree *gts.Tree
					if route == "forest" {
						var ok bool
						tree, ok = parser.ParseForestExperimental(fixture.source)
						if tree != nil {
							defer tree.Release()
						}
						if !ok {
							t.Fatal("forest parse declined")
						}
					} else {
						var parseErr error
						tree, parseErr = parser.Parse(fixture.source)
						if tree != nil {
							defer tree.Release()
						}
						if parseErr != nil {
							t.Fatal(parseErr)
						}
					}
					if tree == nil || tree.RootNode() == nil {
						t.Fatal("Go returned no tree")
					}
					if tree.ParseRuntime().CRecoveryEnteredErrorState {
						t.Fatal("clean conflict fixture entered recovery")
					}
					assertLockedCTreeExact(t, fixture.name+" "+route, tree, language, cTree)
				})
			}
		})
	}
}
