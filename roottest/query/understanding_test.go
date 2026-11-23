package query_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestExtractDefinitionSpansAndCallsGo(t *testing.T) {
	source := []byte(`package main

type Service struct{}

func (s Service) Run() {
	helper()
}

func helper() {}
`)
	tree := parseUnderstandingTree(t, "main.go", source)
	defer tree.Release()

	defs := gotreesitter.ExtractDefinitionSpans(tree)
	assertDefinitionNames(t, defs, "Service", "Run", "helper")

	calls := gotreesitter.ExtractCalls(tree)
	assertCallNames(t, calls, "helper")

	offset := uint32(strings.Index(string(source), "helper()"))
	enclosing, ok := gotreesitter.EnclosingDefinition(tree, offset)
	if !ok || enclosing.Name != "Run" || enclosing.Kind != "method" {
		t.Fatalf("EnclosingDefinition = %#v, %v; want method Run", enclosing, ok)
	}
}

func TestExtractDefinitionSpansCallsAndHeritageJavaScript(t *testing.T) {
	source := []byte(`class Child extends Base {
  method() {
    this.work();
    helper();
  }
}

function helper() {}
`)
	tree := parseUnderstandingTree(t, "main.js", source)
	defer tree.Release()

	defs := gotreesitter.ExtractDefinitionSpans(tree)
	assertDefinitionNames(t, defs, "Child", "method", "helper")

	calls := gotreesitter.ExtractCalls(tree)
	assertCallNames(t, calls, "work", "helper")

	heritage := gotreesitter.ExtractHeritage(tree)
	if !hasHeritage(heritage, "Child", "Base") {
		t.Fatalf("ExtractHeritage = %#v, want Child extends Base", heritage)
	}
}

func TestExtractDefinitionSpansCallsAndHeritagePython(t *testing.T) {
	source := []byte(`class Child(Base, mixins.Helper):
    def method(self):
        self.work()
        helper()

def helper():
    pass
`)
	tree := parseUnderstandingTree(t, "script.py", source)
	defer tree.Release()

	defs := gotreesitter.ExtractDefinitionSpans(tree)
	assertDefinitionNames(t, defs, "Child", "method", "helper")

	calls := gotreesitter.ExtractCalls(tree)
	assertCallNames(t, calls, "work", "helper")

	heritage := gotreesitter.ExtractHeritage(tree)
	if !hasHeritage(heritage, "Child", "Base") || !hasHeritage(heritage, "Child", "mixins.Helper") {
		t.Fatalf("ExtractHeritage = %#v, want Child bases", heritage)
	}
}

func TestExtractDefinitionSpansCallsAndHeritageJava(t *testing.T) {
	source := []byte(`package example;

class Child extends Base implements Runnable {
  void method() {
    helper();
  }
  void helper() {}
}
`)
	tree := parseUnderstandingTree(t, "Child.java", source)
	defer tree.Release()

	defs := gotreesitter.ExtractDefinitionSpans(tree)
	assertDefinitionNames(t, defs, "Child", "method", "helper")

	calls := gotreesitter.ExtractCalls(tree)
	assertCallNames(t, calls, "helper")

	heritage := gotreesitter.ExtractHeritage(tree)
	if !hasHeritage(heritage, "Child", "Base") || !hasHeritage(heritage, "Child", "Runnable") {
		t.Fatalf("ExtractHeritage = %#v, want Child extends Base implements Runnable", heritage)
	}
}

func TestExtractDefinitionSpansTypeScript(t *testing.T) {
	source := []byte(`class Child extends Base {
  method(): void {
    helper();
  }
}

function helper(): void {}
`)
	tree := parseUnderstandingTree(t, "main.ts", source)
	defer tree.Release()

	defs := gotreesitter.ExtractDefinitionSpans(tree)
	assertDefinitionNames(t, defs, "Child", "method", "helper")

	calls := gotreesitter.ExtractCalls(tree)
	assertCallNames(t, calls, "helper")
}

func parseUnderstandingTree(t *testing.T, filename string, source []byte) *gotreesitter.Tree {
	t.Helper()
	entry := grammars.DetectLanguage(filename)
	if entry == nil {
		t.Fatalf("DetectLanguage(%q) returned nil", filename)
	}
	lang := entry.Language()
	parser := gotreesitter.NewParser(lang)
	var (
		tree *gotreesitter.Tree
		err  error
	)
	if entry.TokenSourceFactory != nil {
		tree, err = parser.ParseWithTokenSource(source, entry.TokenSourceFactory(source, lang))
	} else {
		tree, err = parser.Parse(source)
	}
	if err != nil {
		t.Fatalf("parse %s failed: %v", filename, err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("parse %s returned nil tree/root", filename)
	}
	if tree.RootNode().HasError() {
		t.Fatalf("parse %s has errors: %s", filename, tree.RootNode().SExpr(lang))
	}
	return tree
}

func assertDefinitionNames(t *testing.T, defs []gotreesitter.DefinitionSpan, names ...string) {
	t.Helper()
	var got []string
	for _, def := range defs {
		got = append(got, def.Name)
	}
	for _, name := range names {
		if !slices.Contains(got, name) {
			t.Fatalf("definition names = %v, want %q in %#v", got, name, defs)
		}
	}
}

func assertCallNames(t *testing.T, calls []gotreesitter.CallRef, names ...string) {
	t.Helper()
	var got []string
	for _, call := range calls {
		got = append(got, call.Name)
	}
	for _, name := range names {
		if !slices.Contains(got, name) {
			t.Fatalf("call names = %v, want %q in %#v", got, name, calls)
		}
	}
}

func hasHeritage(refs []gotreesitter.HeritageRef, name, parent string) bool {
	for _, ref := range refs {
		if ref.Name == name && ref.Parent == parent {
			return true
		}
	}
	return false
}

func TestHeritageExpressionRanges(t *testing.T) {
	type base struct{ kind, text, nodeType string }
	typed := "abstract class Base<T> extends /* before */ Base<T> implements pkg.I<A, B>, /* between */ J {}\ninterface K extends pkg.I<A, B>, J {}"
	typedWant := []base{{"extends", "Base<T>", "identifier"}, {"implements", "pkg.I<A, B>", "generic_type"}, {"implements", "J", "type_identifier"}, {"extends", "pkg.I<A, B>", "generic_type"}, {"extends", "J", "type_identifier"}}
	for _, tc := range []struct {
		file, src string
		want      []base
	}{
		{"a.js", "class Box extends factory(Box, {x: 1}) {}", []base{{"extends", "factory(Box, {x: 1})", "call_expression"}}},
		{"a.ts", typed, typedWant}, {"a.tsx", typed, typedWant},
		{"a.ts", "class C extends mixin(B) implements I {}", []base{{"extends", "mixin(B)", "call_expression"}, {"implements", "I", "type_identifier"}}},
		{"a.py", "class C(C,\r\n  A[X, Y], # note\r\n  make(B), *bases, metaclass=M, **options): pass\n", []base{{"base", "C", "identifier"}, {"base", "A[X, Y]", "subscript"}, {"base", "make(B)", "call"}, {"base", "*bases", "list_splat"}}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			tree := parseUnderstandingTree(t, tc.file, []byte(tc.src))
			defer tree.Release()
			got := gotreesitter.ExtractHeritage(tree)
			p, err := gotreesitter.NewFactProgram(tree.Language(), gotreesitter.FactHeritage)
			if err != nil {
				t.Fatal(err)
			}
			if facts := p.Extract(tree).Heritage; !slices.Equal(facts, got) {
				t.Fatalf("compiled = %+v, direct = %+v", facts, got)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("heritage = %+v", got)
			}
			// Match the actual expression, even when the declaration has the same spelling.
			offset := strings.IndexAny(tc.src, "(")
			if i := strings.Index(tc.src, "extends"); i >= 0 {
				offset = i
			}
			for i, w := range tc.want {
				r := got[i]
				at := strings.Index(tc.src[offset:], w.text) + offset
				if r.Kind != w.kind || r.Parent != w.text || r.ParentNodeType != w.nodeType || int(r.ParentStartByte) != at || int(r.ParentEndByte) != at+len(w.text) || r.Name == "" || r.ParentStartByte < r.StartByte || r.ParentEndByte > r.EndByte {
					t.Fatalf("heritage[%d] = %+v, want %+v at %d", i, r, w, at)
				}
				offset = int(r.ParentEndByte)
			}
		})
	}
}
