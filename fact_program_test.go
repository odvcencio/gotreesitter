package gotreesitter_test

import (
	"slices"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestFactProgramMatchesLegacyExtractors(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		source   string
	}{
		{
			name:     "go",
			filename: "main.go",
			source: `package main

import alias "example.com/library"

type Service struct{}

func (s Service) Run() {
	alias.Helper()
}
`,
		},
		{
			name:     "javascript",
			filename: "main.js",
			source: `class Child extends Base {
  method() {
    this.work();
  }
}
`,
		},
		{
			name:     "typescript",
			filename: "main.ts",
			source: `class Child extends Base {
  method(): void {
    helper();
  }
}
`,
		},
		{
			name:     "python",
			filename: "main.py",
			source: `from package import helper as run

class Child(Base, mixins.Helper):
    def method(self):
        run()
`,
		},
		{
			name:     "java",
			filename: "Main.java",
			source: `package example;

import java.util.List;

class Child extends Base implements Runnable {
  void method() {
    helper();
  }
}
`,
		},
		{
			name:     "starlark",
			filename: "BUILD.bazel",
			source: `load("//tools:defs.bzl", "helper")

def run():
    helper()
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tree := parseUnderstandingTree(t, test.filename, []byte(test.source))
			defer tree.Release()

			program, err := gotreesitter.NewFactProgram(tree.Language(), gotreesitter.FactAll)
			if err != nil {
				t.Fatalf("NewFactProgram failed: %v", err)
			}
			facts := program.Extract(tree)

			if want := gotreesitter.ExtractDefinitionSpans(tree); !slices.Equal(facts.Definitions, want) {
				t.Fatalf("definitions differ:\nprogram: %#v\nlegacy:  %#v", facts.Definitions, want)
			}
			if want := gotreesitter.ExtractCalls(tree); !slices.Equal(facts.Calls, want) {
				t.Fatalf("calls differ:\nprogram: %#v\nlegacy:  %#v", facts.Calls, want)
			}
			if want := gotreesitter.ExtractHeritage(tree); !slices.Equal(facts.Heritage, want) {
				t.Fatalf("heritage differs:\nprogram: %#v\nlegacy:  %#v", facts.Heritage, want)
			}
			if want := gotreesitter.ExtractImports(tree); !slices.Equal(facts.Imports, want) {
				t.Fatalf("imports differ:\nprogram: %#v\nlegacy:  %#v", facts.Imports, want)
			}
		})
	}
}

func TestFactProgramSelectionAndLanguageGuard(t *testing.T) {
	goTree := parseUnderstandingTree(t, "main.go", []byte("package main\nfunc run() { helper() }\n"))
	defer goTree.Release()

	program, err := gotreesitter.NewFactProgram(goTree.Language(), gotreesitter.FactDefinitions|gotreesitter.FactCalls)
	if err != nil {
		t.Fatalf("NewFactProgram failed: %v", err)
	}
	if got := program.Kinds(); got != gotreesitter.FactDefinitions|gotreesitter.FactCalls {
		t.Fatalf("Kinds = %v, want definitions and calls", got)
	}
	facts := program.Extract(goTree)
	if len(facts.Definitions) != 1 || len(facts.Calls) != 1 {
		t.Fatalf("selected facts = %#v, want one definition and one call", facts)
	}
	if facts.Heritage != nil || facts.Imports != nil {
		t.Fatalf("unselected facts are not nil: %#v", facts)
	}

	javaTree := parseUnderstandingTree(t, "Main.java", []byte("class Main extends Base {}\n"))
	defer javaTree.Release()
	if got := program.Extract(javaTree); got.Definitions != nil || got.Calls != nil || got.Heritage != nil || got.Imports != nil {
		t.Fatalf("language-mismatched extraction = %#v, want empty set", got)
	}

	heritageProgram, err := gotreesitter.NewFactProgram(javaTree.Language(), gotreesitter.FactHeritage)
	if err != nil {
		t.Fatalf("NewFactProgram heritage failed: %v", err)
	}
	heritageFacts := heritageProgram.Extract(javaTree)
	if len(heritageFacts.Heritage) != 1 || heritageFacts.Heritage[0].Parent != "Base" {
		t.Fatalf("heritage-only facts = %#v, want Main extends Base", heritageFacts)
	}
	if heritageFacts.Definitions != nil || heritageFacts.Calls != nil || heritageFacts.Imports != nil {
		t.Fatalf("heritage-only program emitted unselected facts: %#v", heritageFacts)
	}
}

func TestFactProgramExtractBoundMatchesExtract(t *testing.T) {
	tree := parseUnderstandingTree(t, "main.go", []byte("package main\nfunc run() { helper() }\n"))
	defer tree.Release()

	program, err := gotreesitter.NewFactProgram(tree.Language(), gotreesitter.FactAll)
	if err != nil {
		t.Fatalf("NewFactProgram failed: %v", err)
	}
	want := program.Extract(tree)
	got := program.ExtractBound(gotreesitter.Bind(tree))
	if !slices.Equal(got.Definitions, want.Definitions) ||
		!slices.Equal(got.Calls, want.Calls) ||
		!slices.Equal(got.Heritage, want.Heritage) ||
		!slices.Equal(got.Imports, want.Imports) {
		t.Fatalf("ExtractBound = %#v, want %#v", got, want)
	}

	empty := program.ExtractBound(nil)
	if empty.Definitions != nil || empty.Calls != nil || empty.Heritage != nil || empty.Imports != nil {
		t.Fatalf("ExtractBound(nil) = %#v, want empty set", empty)
	}
}

func TestNewFactProgramRejectsInvalidConfiguration(t *testing.T) {
	if _, err := gotreesitter.NewFactProgram(nil, gotreesitter.FactAll); err == nil {
		t.Fatal("NewFactProgram accepted a nil language")
	}
	if _, err := gotreesitter.NewFactProgram(grammars.GoLanguage(), gotreesitter.FactKind(1<<7)); err == nil {
		t.Fatal("NewFactProgram accepted an unknown fact kind")
	}
	empty, err := gotreesitter.NewFactProgram(grammars.GoLanguage(), 0)
	if err != nil {
		t.Fatalf("NewFactProgram zero mask failed: %v", err)
	}
	if empty.Kinds() != 0 {
		t.Fatalf("zero-mask Kinds = %v, want 0", empty.Kinds())
	}
}
