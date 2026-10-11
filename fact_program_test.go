package gotreesitter_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
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
			var reused gotreesitter.FactSet
			for i := 0; i < 2; i++ {
				program.ExtractInto(tree, &reused)
				assertFactSetsEqual(t, reused, facts)
			}
		})
	}
}

func assertFactSetsEqual(t *testing.T, got, want gotreesitter.FactSet) {
	t.Helper()
	if !slices.Equal(got.Declarations, want.Declarations) ||
		!slices.Equal(got.Definitions, want.Definitions) ||
		!slices.Equal(got.Calls, want.Calls) ||
		!slices.Equal(got.Heritage, want.Heritage) ||
		!slices.Equal(got.Imports, want.Imports) {
		t.Fatalf("facts = %#v, want %#v", got, want)
	}
}

func TestFactProgramExtractIntoReplacesAndClears(t *testing.T) {
	tree := parseUnderstandingTree(t, "main.go", []byte("package main\nfunc run() { helper() }\n"))
	defer tree.Release()
	program, err := gotreesitter.NewFactProgram(tree.Language(), gotreesitter.FactAll)
	if err != nil {
		t.Fatal(err)
	}
	callsOnly, err := gotreesitter.NewFactProgram(tree.Language(), gotreesitter.FactCalls)
	if err != nil {
		t.Fatal(err)
	}
	zeroKinds, err := gotreesitter.NewFactProgram(tree.Language(), 0)
	if err != nil {
		t.Fatal(err)
	}
	otherLanguage := *tree.Language()
	mismatched, err := gotreesitter.NewFactProgram(&otherLanguage, gotreesitter.FactAll)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name    string
		program *gotreesitter.FactProgram
		tree    *gotreesitter.Tree
	}{
		{"smaller_result", program, tree},
		{"selected_kinds", callsOnly, tree},
		{"nil_program", nil, tree},
		{"nil_tree", program, nil},
		{"empty_tree", program, &gotreesitter.Tree{}},
		{"language_mismatch", mismatched, tree},
		{"zero_kinds", zeroKinds, tree},
	} {
		t.Run(test.name, func(t *testing.T) {
			facts := gotreesitter.FactSet{
				Definitions: make([]gotreesitter.DefinitionSpan, 8),
				Calls:       make([]gotreesitter.CallRef, 8),
				Heritage:    make([]gotreesitter.HeritageRef, 8),
				Imports:     make([]gotreesitter.ImportRef, 8),
			}
			for i := 0; i < 8; i++ {
				facts.Definitions[i].Name = "old definition"
				facts.Calls[i].Name = "old call"
				facts.Heritage[i].Parent = "old parent"
				facts.Imports[i].Path = "old import"
			}
			storage := facts
			test.program.ExtractInto(test.tree, &facts)
			assertFactSetsEqual(t, facts, test.program.Extract(test.tree))
			assertFactStorageReused(t, storage.Definitions, facts.Definitions)
			assertFactStorageReused(t, storage.Calls, facts.Calls)
			assertFactStorageReused(t, storage.Heritage, facts.Heritage)
			assertFactStorageReused(t, storage.Imports, facts.Imports)
			test.program.ExtractInto(test.tree, nil)
		})
	}
}

func assertFactStorageReused[T comparable](t *testing.T, previous, current []T) {
	t.Helper()
	if cap(current) != cap(previous) || &previous[0] != &current[:cap(current)][0] {
		t.Fatal("extraction did not reuse the result storage")
	}
	var zero T
	for i, value := range previous[len(current):] {
		if value != zero {
			t.Fatalf("unused entry %d retains a previous result: %#v", len(current)+i, value)
		}
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
	if !slices.Equal(got.Declarations, want.Declarations) ||
		!slices.Equal(got.Definitions, want.Definitions) ||
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

const goDeclarationsExample = `package example

type Reader interface {
    Read(dst []byte) (int, error)
}

type Record struct {
    Key, Value string
}

type Alias = Record

var first, second string
const left, right = 1, 2

func (r Record) String() string { return r.Key }
`

func declarationProgram(t *testing.T, kinds gotreesitter.FactKind) *gotreesitter.FactProgram {
	t.Helper()
	entry := grammars.DetectLanguage("main.go")
	program, err := gotreesitter.NewFactProgramWithOptions(entry.Language(), kinds,
		gotreesitter.WithDeclarationRules(grammars.DeclarationRules(*entry)))
	if err != nil {
		t.Fatal(err)
	}
	return program
}

type declarationWant struct {
	name, kind, nodeType, construct, shape, container, containerConstruct string
	embedded                                                              bool
}

func assertDeclarations(t *testing.T, source string, defs []gotreesitter.DeclarationFact, want ...declarationWant) {
	t.Helper()
	if len(defs) != len(want) {
		t.Fatalf("definitions = %#v; want %d facts", defs, len(want))
	}
	for i, w := range want {
		start := strings.Index(source, w.construct)
		if w.container != "" {
			base := strings.Index(source, w.containerConstruct)
			if base >= 0 {
				at := strings.Index(w.containerConstruct, w.construct)
				if at >= 0 {
					start = base + at
				}
			}
		}
		if start < 0 {
			t.Fatalf("missing construct %q", w.construct)
		}
		name := strings.Index(w.construct, w.name)
		if name < 0 {
			t.Fatalf("missing name %q in %q", w.name, w.construct)
		}
		expected := gotreesitter.DeclarationFact{
			Lang: "go", Kind: w.kind, Name: w.name, NodeType: w.nodeType,
			StartByte: uint32(start), EndByte: uint32(start + len(w.construct)),
			NameStartByte: uint32(start + name), NameEndByte: uint32(start + name + len(w.name)),
			Shape: w.shape, Container: w.container, Embedded: w.embedded,
		}
		if w.container != "" {
			container := strings.Index(source, w.containerConstruct)
			if container < 0 {
				t.Fatalf("missing container %q", w.containerConstruct)
			}
			expected.ContainerStartByte = uint32(container)
			expected.ContainerEndByte = uint32(container + len(w.containerConstruct))
		}
		if defs[i] != expected {
			t.Errorf("fact %d = %#v; want %#v", i, defs[i], expected)
		}
	}
}

func TestFactProgramLegacyAPI(t *testing.T) {
	var constructor func(*gotreesitter.Language, gotreesitter.FactKind) (*gotreesitter.FactProgram, error) = gotreesitter.NewFactProgram
	program, err := constructor(grammars.GoLanguage(), gotreesitter.FactDefinitions)
	if err != nil || program.Kinds() != gotreesitter.FactDefinitions {
		t.Fatalf("legacy constructor: %v", err)
	}
	// Keep positional literals valid: this type's layout must stay unchanged.
	span := gotreesitter.DefinitionSpan{"go", "type", "Example", "type_spec", 1, 8, 1, 8}
	if span.Name != "Example" {
		t.Fatal(span)
	}
}

func TestFactDeclarationsDisabledAllocations(t *testing.T) {
	tree := parseUnderstandingTree(t, "main.go", []byte(goDeclarationsExample))
	defer tree.Release()
	rules := grammars.DeclarationRules(*grammars.DetectLanguage("main.go"))
	plain, err := gotreesitter.NewFactProgram(tree.Language(), gotreesitter.FactAll)
	if err != nil {
		t.Fatal(err)
	}
	configured, err := gotreesitter.NewFactProgramWithOptions(tree.Language(), gotreesitter.FactAll, gotreesitter.WithDeclarationRules(rules))
	if err != nil {
		t.Fatal(err)
	}
	var facts gotreesitter.FactSet
	baseline := testing.AllocsPerRun(20, func() { facts = plain.Extract(tree) })
	withRules := testing.AllocsPerRun(20, func() { facts = configured.Extract(tree) })
	if baseline != withRules || facts.Declarations != nil {
		t.Fatalf("disabled declarations: plain=%v configured=%v facts=%#v", baseline, withRules, facts)
	}
	assertFactSetsEqual(t, facts, plain.Extract(tree))
}

func TestFactDeclarationsGoExample(t *testing.T) {
	tree := parseUnderstandingTree(t, "main.go", []byte(goDeclarationsExample))
	defer tree.Release()
	program := declarationProgram(t, gotreesitter.FactDefinitions|gotreesitter.FactDeclarations)
	if got := program.Extract(tree).Definitions; !slices.Equal(got, gotreesitter.ExtractDefinitionSpans(tree)) {
		t.Fatalf("Definitions changed: %#v", got)
	}
	reader := "Reader interface {\n    Read(dst []byte) (int, error)\n}"
	record := "Record struct {\n    Key, Value string\n}"
	assertDeclarations(t, goDeclarationsExample, program.Extract(tree).Declarations,
		declarationWant{"Reader", "type", "type_spec", reader, "interface", "", "", false},
		declarationWant{"Read", "method", "method_elem", "Read(dst []byte) (int, error)", "", "Reader", reader, false},
		declarationWant{"Record", "type", "type_spec", record, "struct", "", "", false},
		declarationWant{"Key", "field", "field_declaration", "Key, Value string", "", "Record", record, false},
		declarationWant{"Value", "field", "field_declaration", "Key, Value string", "", "Record", record, false},
		declarationWant{"Alias", "type", "type_alias", "Alias = Record", "alias", "", "", false},
		declarationWant{"first", "variable", "var_spec", "first, second string", "", "", "", false},
		declarationWant{"second", "variable", "var_spec", "first, second string", "", "", "", false},
		declarationWant{"left", "constant", "const_spec", "left, right = 1, 2", "", "", "", false},
		declarationWant{"right", "constant", "const_spec", "left, right = 1, 2", "", "", "", false},
	)
}

func TestFactDeclarationsGoGroupedBindings(t *testing.T) {
	const source = `package p
var ( a = 1; b, c int; _, d = 2, 3 )
const ( A = iota; B; left, right = 1, 2; _, C = 3, 4 )
var x = 1; var y, z int
const solo = 9; const other = 10
`
	tree := parseUnderstandingTree(t, "main.go", []byte(source))
	defer tree.Release()
	program := declarationProgram(t, gotreesitter.FactDeclarations)
	var want []declarationWant
	for _, group := range []struct {
		kind, nodeType, construct string
		names                     []string
	}{
		{"variable", "var_spec", "a = 1", []string{"a"}},
		{"variable", "var_spec", "b, c int", []string{"b", "c"}},
		{"variable", "var_spec", "_, d = 2, 3", []string{"d"}},
		{"constant", "const_spec", "A = iota", []string{"A"}},
		{"constant", "const_spec", "B", []string{"B"}},
		{"constant", "const_spec", "left, right = 1, 2", []string{"left", "right"}},
		{"constant", "const_spec", "_, C = 3, 4", []string{"C"}},
		{"variable", "var_spec", "x = 1", []string{"x"}},
		{"variable", "var_spec", "y, z int", []string{"y", "z"}},
		{"constant", "const_spec", "solo = 9", []string{"solo"}},
		{"constant", "const_spec", "other = 10", []string{"other"}},
	} {
		for _, name := range group.names {
			want = append(want, declarationWant{name, group.kind, group.nodeType, group.construct, "", "", "", false})
		}
	}
	assertDeclarations(t, source, program.Extract(tree).Declarations, want...)
}

func TestFactDeclarationsGoScopeAndOmissions(t *testing.T) {
	const source = `package p
type Record struct {
 Embedded
 *Pointer
 Named struct { hidden int }
 _ int
}
type Reader interface {
 Embedded
 ~int | ~string
 Read()
}
var anonymous struct { hiddenVar int }
func run() {
 var local, other int
 const localConst = 1
 var (
  grouped = 2
 )
 const (
  groupedConst = 3
 )
 _ = func() { var closure int; const closureConst = 4 }
}
`
	tree := parseUnderstandingTree(t, "main.go", []byte(source))
	defer tree.Release()
	defs := declarationProgram(t, gotreesitter.FactDefinitions|gotreesitter.FactDeclarations).Extract(tree).Declarations
	record := "Record struct {\n Embedded\n *Pointer\n Named struct { hidden int }\n _ int\n}"
	reader := "Reader interface {\n Embedded\n ~int | ~string\n Read()\n}"
	assertDeclarations(t, source, defs,
		declarationWant{"Record", "type", "type_spec", record, "struct", "", "", false},
		declarationWant{"Embedded", "field", "field_declaration", "Embedded", "", "Record", record, true},
		declarationWant{"Pointer", "field", "field_declaration", "*Pointer", "", "Record", record, true},
		declarationWant{"Named", "field", "field_declaration", "Named struct { hidden int }", "", "Record", record, false},
		declarationWant{"hidden", "field", "field_declaration", "hidden int", "", "Record.Named", "Named struct { hidden int }", false},
		declarationWant{"Reader", "type", "type_spec", reader, "interface", "", "", false},
		declarationWant{"Embedded", "embed", "type_elem", "Embedded", "", "Reader", reader, false},
		declarationWant{"Read", "method", "method_elem", "Read()", "", "Reader", reader, false},
		declarationWant{"anonymous", "variable", "var_spec", "anonymous struct { hiddenVar int }", "", "", "", false},
		declarationWant{"hiddenVar", "field", "field_declaration", "hiddenVar int", "", "anonymous", "anonymous struct { hiddenVar int }", false},
	)
	offset := uint32(strings.Index(source, "local, other"))
	if enclosing, ok := gotreesitter.EnclosingDefinition(tree, offset); !ok || enclosing.Name != "run" {
		t.Fatalf("enclosing = %#v, %v", enclosing, ok)
	}
}

func TestFactDeclarationsGoGenericTypes(t *testing.T) {
	const source = "package p\ntype List[T any] struct { head *T }\ntype Count int\n"
	tree := parseUnderstandingTree(t, "main.go", []byte(source))
	defer tree.Release()
	assertDeclarations(t, source, declarationProgram(t, gotreesitter.FactDeclarations).Extract(tree).Declarations,
		declarationWant{"List", "type", "type_spec", "List[T any] struct { head *T }", "struct", "", "", false},
		declarationWant{"head", "field", "field_declaration", "head *T", "", "List", "List[T any] struct { head *T }", false},
		declarationWant{"Count", "type", "type_spec", "Count int", "", "", "", false},
	)
}

func TestFactDeclarationsOptInAndReuse(t *testing.T) {
	tree := parseUnderstandingTree(t, "main.go", []byte(goDeclarationsExample))
	defer tree.Release()
	for _, kind := range []gotreesitter.FactKind{gotreesitter.FactDefinitions, gotreesitter.FactAll} {
		program := declarationProgram(t, kind)
		if got := program.Extract(tree).Definitions; !slices.Equal(got, gotreesitter.ExtractDefinitionSpans(tree)) {
			t.Fatalf("legacy definitions changed: %#v", got)
		}
	}
	replaced, err := gotreesitter.NewFactProgramWithOptions(tree.Language(), gotreesitter.FactDeclarations,
		gotreesitter.WithDeclarationRules(grammars.DeclarationRules(*grammars.DetectLanguage("main.go"))), gotreesitter.WithDeclarationRules(nil))
	if err != nil {
		t.Fatal(err)
	}
	if got := replaced.Extract(tree); len(got.Declarations) != 0 {
		t.Fatalf("later option did not replace rules: %#v", got)
	}
	if gotreesitter.FactAll&gotreesitter.FactDeclarations != 0 {
		t.Fatal("FactAll silently includes declarations")
	}
	noRules, err := gotreesitter.NewFactProgram(tree.Language(), gotreesitter.FactDeclarations)
	if err != nil {
		t.Fatal(err)
	}
	if got := noRules.Extract(tree); len(got.Declarations) != 0 {
		t.Fatalf("rules invented: %#v", got)
	}
	program := declarationProgram(t, gotreesitter.FactAll|gotreesitter.FactDeclarations)
	facts := program.Extract(tree)
	if !slices.Equal(facts.Definitions, gotreesitter.ExtractDefinitionSpans(tree)) {
		t.Fatalf("enabling declarations changed Definitions: %#v", facts.Definitions)
	}
	if len(facts.Declarations) != 10 {
		t.Fatalf("Declarations = %#v", facts.Declarations)
	}
	storage := facts
	program.ExtractInto(tree, &facts)
	assertFactSetsEqual(t, facts, program.Extract(tree))
	assertFactStorageReused(t, storage.Definitions, facts.Definitions)
	assertFactStorageReused(t, storage.Declarations, facts.Declarations)
	small := parseUnderstandingTree(t, "main.go", []byte("package p\nvar only int\n"))
	defer small.Release()
	program.ExtractInto(small, &facts)
	assertFactSetsEqual(t, facts, program.Extract(small))
	assertFactStorageReused(t, storage.Definitions, facts.Definitions)
	assertFactStorageReused(t, storage.Declarations, facts.Declarations)
	legacy := declarationProgram(t, gotreesitter.FactDefinitions)
	legacy.ExtractInto(tree, &facts)
	assertFactSetsEqual(t, facts, legacy.Extract(tree))
	assertFactStorageReused(t, storage.Definitions, facts.Definitions)
	assertFactStorageReused(t, storage.Declarations, facts.Declarations)
	program.ExtractInto(nil, &facts)
	assertFactStorageReused(t, storage.Definitions, facts.Definitions)
	assertFactStorageReused(t, storage.Declarations, facts.Declarations)
}

// These receipts were generated at 952c5f79b before declaration extraction.
// Include the Go fixtures from understanding, FactProgram, and outline tests.
func TestFactProgramGoCompatibility(t *testing.T) {
	sources := []string{
		"package main\n\ntype Service struct{}\n\nfunc (s Service) Run() {\n\thelper()\n}\n\nfunc helper() {}\n",
		"package main\n\nimport alias \"example.com/library\"\n\ntype Service struct{}\n\nfunc (s Service) Run() {\n\talias.Helper()\n}\n",
		"package main\nfunc run() { helper() }\n",
	}
	for _, fixture := range outlineGoldenFixtures {
		if fixture.Language != "go" {
			continue
		}
		source, err := os.ReadFile(filepath.Join(outlineGoldenRoot, fixture.File))
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, string(source))
	}
	entry := grammars.DetectLanguage("main.go")
	for i, source := range sources {
		// The broken outline fixture intentionally contains syntax errors.
		tree, err := gotreesitter.NewParser(entry.Language()).Parse([]byte(source))
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Release()
		definitions, err := gotreesitter.NewFactProgramWithOptions(tree.Language(), gotreesitter.FactDefinitions, gotreesitter.WithDeclarationRules(grammars.DeclarationRules(*entry)))
		if err != nil {
			t.Fatal(err)
		}
		all, err := gotreesitter.NewFactProgramWithOptions(tree.Language(), gotreesitter.FactAll, gotreesitter.WithDeclarationRules(grammars.DeclarationRules(*entry)))
		if err != nil {
			t.Fatal(err)
		}
		defs := definitions.Extract(tree).Definitions
		if !slices.Equal(defs, gotreesitter.ExtractDefinitionSpans(tree)) {
			t.Fatal("legacy definitions differ")
		}
		outline, err := gotreesitter.NewOutliner(tree.Language(), grammars.ResolveTagsQuery(*entry))
		if err != nil {
			t.Fatal(err)
		}
		symbols, report := outline.OutlineTree(tree)
		enclosing := make([]gotreesitter.DefinitionSpan, len(source))
		for offset := range len(source) {
			enclosing[offset], _ = gotreesitter.EnclosingDefinition(tree, uint32(offset))
		}
		receipt := struct {
			Source      string
			Definitions []gotreesitter.DefinitionSpan
			All         gotreesitter.FactSet
			Calls       []gotreesitter.CallRef
			Enclosing   []gotreesitter.DefinitionSpan
			Outline     []gotreesitter.OutlineSymbol
			Report      gotreesitter.OutlineReport
			Kinds       []string
		}{source, defs, all.Extract(tree), gotreesitter.ExtractCalls(tree), enclosing, symbols, report, outline.DefinitionKinds()}
		data, err := json.Marshal(receipt)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join("testdata", "facts", "go-compatibility-"+string(rune('0'+i))+".json")
		if os.Getenv("GTS_UPDATE_FACT_COMPAT") == "1" {
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
				t.Fatal(err)
			}
		}
		expected, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != strings.TrimSpace(string(expected)) {
			t.Fatalf("legacy outputs changed for fixture %d (%s)", i, path)
		}
	}
}

func richGoProgram(t *testing.T, kinds gotreesitter.FactKind) *gotreesitter.FactProgram {
	t.Helper()
	entry := grammars.DetectLanguage("main.go")
	p, err := gotreesitter.NewFactProgramWithOptions(entry.Language(), kinds,
		gotreesitter.WithDeclarationRules(grammars.DeclarationRules(*entry)),
		gotreesitter.WithSignatureRules(grammars.SignatureRules(*entry)),
		gotreesitter.WithCallArgumentRules(grammars.CallArgumentRules(*entry)))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFactDeclarationsGoEmbeddings(t *testing.T) {
	const source = `package p
type Record struct { io.Reader; *Base; pkg.Box[int] }
type API interface { io.Reader; fmt.Stringer; Box[int]; pkg.comparable; comparable; ~int; ~int | ~string; int | string; *Base; []int; Read() }
`
	tree := parseUnderstandingTree(t, "main.go", []byte(source))
	defer tree.Release()
	record := "Record struct { io.Reader; *Base; pkg.Box[int] }"
	api := "API interface { io.Reader; fmt.Stringer; Box[int]; pkg.comparable; comparable; ~int; ~int | ~string; int | string; *Base; []int; Read() }"
	assertDeclarations(t, source, declarationProgram(t, gotreesitter.FactDeclarations).Extract(tree).Declarations,
		declarationWant{"Record", "type", "type_spec", record, "struct", "", "", false},
		declarationWant{"Reader", "field", "field_declaration", "io.Reader", "", "Record", record, true},
		declarationWant{"Base", "field", "field_declaration", "*Base", "", "Record", record, true},
		declarationWant{"Box", "field", "field_declaration", "pkg.Box[int]", "", "Record", record, true},
		declarationWant{"API", "type", "type_spec", api, "interface", "", "", false},
		declarationWant{"Reader", "embed", "type_elem", "io.Reader", "", "API", api, false},
		declarationWant{"Stringer", "embed", "type_elem", "fmt.Stringer", "", "API", api, false},
		declarationWant{"Box", "embed", "type_elem", "Box[int]", "", "API", api, false},
		declarationWant{"comparable", "embed", "type_elem", "pkg.comparable", "", "API", api, false},
		declarationWant{"Read", "method", "method_elem", "Read()", "", "API", api, false})
}

func TestFactDeclarationsGoAnonymousContainers(t *testing.T) {
	const source = `package p
type Config struct { Server struct { Host string; TLS struct { Enabled bool; Deep struct { Port int } } } }
var cfg struct{ Debug bool; Nested struct { Value int } }
var ( grouped struct { Flag bool }; )
func local() { var hidden struct { Nope int } }
`
	tree := parseUnderstandingTree(t, "main.go", []byte(source))
	defer tree.Release()
	config := "Config struct { Server struct { Host string; TLS struct { Enabled bool; Deep struct { Port int } } } }"
	server := "Server struct { Host string; TLS struct { Enabled bool; Deep struct { Port int } } }"
	tls := "TLS struct { Enabled bool; Deep struct { Port int } }"
	deep := "Deep struct { Port int }"
	cfg := "cfg struct{ Debug bool; Nested struct { Value int } }"
	grouped := "grouped struct { Flag bool }"
	assertDeclarations(t, source, declarationProgram(t, gotreesitter.FactDeclarations).Extract(tree).Declarations,
		declarationWant{"Config", "type", "type_spec", config, "struct", "", "", false},
		declarationWant{"Server", "field", "field_declaration", server, "", "Config", config, false},
		declarationWant{"Host", "field", "field_declaration", "Host string", "", "Config.Server", server, false},
		declarationWant{"TLS", "field", "field_declaration", tls, "", "Config.Server", server, false},
		declarationWant{"Enabled", "field", "field_declaration", "Enabled bool", "", "Config.Server.TLS", tls, false},
		declarationWant{"Deep", "field", "field_declaration", deep, "", "Config.Server.TLS", tls, false},
		declarationWant{"Port", "field", "field_declaration", "Port int", "", "Config.Server.TLS.Deep", deep, false},
		declarationWant{"cfg", "variable", "var_spec", cfg, "", "", "", false},
		declarationWant{"Debug", "field", "field_declaration", "Debug bool", "", "cfg", cfg, false},
		declarationWant{"Nested", "field", "field_declaration", "Nested struct { Value int }", "", "cfg", cfg, false},
		declarationWant{"Value", "field", "field_declaration", "Value int", "", "cfg.Nested", "Nested struct { Value int }", false},
		declarationWant{"grouped", "variable", "var_spec", grouped, "", "", "", false},
		declarationWant{"Flag", "field", "field_declaration", "Flag bool", "", "grouped", grouped, false})
}

type parameterWant struct {
	decl, name, typ   string
	variadic, pointer bool
}

func wantParameters(t *testing.T, source, signature string, specs []parameterWant) []gotreesitter.ParameterFact {
	t.Helper()
	var result []gotreesitter.ParameterFact
	base := strings.Index(source, signature)
	if base < 0 {
		t.Fatalf("missing signature %q", signature)
	}
	for _, w := range specs {
		at := strings.Index(signature, w.decl)
		if at < 0 {
			t.Fatalf("missing parameter %q", w.decl)
		}
		typ := strings.LastIndex(w.decl, w.typ)
		if typ < 0 {
			t.Fatalf("missing type %q", w.typ)
		}
		f := gotreesitter.ParameterFact{Name: w.name, Type: w.typ, Variadic: w.variadic, Pointer: w.pointer,
			StartByte: uint32(base + at), EndByte: uint32(base + at + len(w.decl)),
			TypeStartByte: uint32(base + at + typ), TypeEndByte: uint32(base + at + typ + len(w.typ))}
		if w.name != "" {
			n := strings.Index(w.decl, w.name)
			f.NameStartByte = uint32(base + at + n)
			f.NameEndByte = f.NameStartByte + uint32(len(w.name))
		}
		result = append(result, f)
	}
	return result
}

func TestFactSignaturesGoGolden(t *testing.T) {
	const source = `package p
func Plain(a, b int, rest ...string) (x, y int, err error) { return }
func Unnamed(int, string, ...bool) (int, error) { return }
func Single() error { return nil }
func Generic[A, B interface{ ~int | ~string }, C comparable](value /* keep */ map[string] /* exact */ *A) *B { return nil }
func (v Value) Get() int { return 0 }
func (p *Value) Set(x int) { }
func (l *List[T]) Push(item T) error { return nil }
func (List[T]) Empty() bool { return true }
type API interface { Read(dst []byte) (n int, err error); Reset(); io.Reader }
var literal = func(x int) string { return "" }
type Function func(int) error
`
	tree := parseUnderstandingTree(t, "main.go", []byte(source))
	defer tree.Release()
	p := richGoProgram(t, gotreesitter.FactSignatures)
	specs := []struct {
		decl, kind, name, container, nodeType string
		receiver, types, params, results      []parameterWant
	}{
		{"func Plain(a, b int, rest ...string) (x, y int, err error) { return }", "function", "Plain", "", "function_declaration", nil, nil,
			[]parameterWant{{"a, b int", "a", "int", false, false}, {"a, b int", "b", "int", false, false}, {"rest ...string", "rest", "string", true, false}},
			[]parameterWant{{"x, y int", "x", "int", false, false}, {"x, y int", "y", "int", false, false}, {"err error", "err", "error", false, false}}},
		{"func Unnamed(int, string, ...bool) (int, error) { return }", "function", "Unnamed", "", "function_declaration", nil, nil,
			[]parameterWant{{"int", "", "int", false, false}, {"string", "", "string", false, false}, {"...bool", "", "bool", true, false}},
			[]parameterWant{{"int", "", "int", false, false}, {"error", "", "error", false, false}}},
		{"func Single() error { return nil }", "function", "Single", "", "function_declaration", nil, nil, nil, []parameterWant{{"error", "", "error", false, false}}},
		{"func Generic[A, B interface{ ~int | ~string }, C comparable](value /* keep */ map[string] /* exact */ *A) *B { return nil }", "function", "Generic", "", "function_declaration", nil,
			[]parameterWant{{"A, B interface{ ~int | ~string }", "A", "interface{ ~int | ~string }", false, false}, {"A, B interface{ ~int | ~string }", "B", "interface{ ~int | ~string }", false, false}, {"C comparable", "C", "comparable", false, false}},
			[]parameterWant{{"value /* keep */ map[string] /* exact */ *A", "value", "map[string] /* exact */ *A", false, false}}, []parameterWant{{"*B", "", "*B", false, false}}},
		{"func (v Value) Get() int { return 0 }", "method", "Get", "Value", "method_declaration", []parameterWant{{"v Value", "v", "Value", false, false}}, nil, nil, []parameterWant{{"int", "", "int", false, false}}},
		{"func (p *Value) Set(x int) { }", "method", "Set", "Value", "method_declaration", []parameterWant{{"p *Value", "p", "*Value", false, true}}, nil, []parameterWant{{"x int", "x", "int", false, false}}, nil},
		{"func (l *List[T]) Push(item T) error { return nil }", "method", "Push", "List", "method_declaration", []parameterWant{{"l *List[T]", "l", "*List[T]", false, true}}, nil, []parameterWant{{"item T", "item", "T", false, false}}, []parameterWant{{"error", "", "error", false, false}}},
		{"func (List[T]) Empty() bool { return true }", "method", "Empty", "List", "method_declaration", []parameterWant{{"List[T]", "", "List[T]", false, false}}, nil, nil, []parameterWant{{"bool", "", "bool", false, false}}},
		{"Read(dst []byte) (n int, err error)", "method", "Read", "API", "method_elem", nil, nil, []parameterWant{{"dst []byte", "dst", "[]byte", false, false}}, []parameterWant{{"n int", "n", "int", false, false}, {"err error", "err", "error", false, false}}},
		{"Reset()", "method", "Reset", "API", "method_elem", nil, nil, nil, nil},
	}
	got := p.Extract(tree).Signatures
	if len(got) != len(specs) {
		t.Fatalf("signatures = %#v", got)
	}
	for i, w := range specs {
		at := strings.Index(source, w.decl)
		name := strings.Index(w.decl, w.name)
		want := gotreesitter.SignatureFact{Lang: "go", Kind: w.kind, Name: w.name, Container: w.container, NodeType: w.nodeType,
			StartByte: uint32(at), EndByte: uint32(at + len(w.decl)), NameStartByte: uint32(at + name), NameEndByte: uint32(at + name + len(w.name)),
			Receiver: wantParameters(t, source, w.decl, w.receiver), TypeParameters: wantParameters(t, source, w.decl, w.types),
			Parameters: wantParameters(t, source, w.decl, w.params), Results: wantParameters(t, source, w.decl, w.results)}
		// The unnamed int result follows the parameter list, rather than its first int.
		if w.name == "Unnamed" {
			start := strings.Index(source, "(int, error)") + 1
			want.Results[0].StartByte = uint32(start)
			want.Results[0].EndByte = uint32(start + 3)
			want.Results[0].TypeStartByte = uint32(start)
			want.Results[0].TypeEndByte = uint32(start + 3)
		}
		if !reflect.DeepEqual(got[i], want) {
			t.Errorf("signature %d = %#v; want %#v", i, got[i], want)
		}
	}
}

func TestFactCallArgumentsGoGoldenAndJoin(t *testing.T) {
	const source = "package p\nfunc run() { outer(x, pkg.Value, inner(a), T{X: 1}, func(y int) {}, 42, 1.5, 2i, \"s\", `raw`, 'r', true, false, nil, &x, *p, <-ch, a+b, xs[0], xs[:], x.(T), (x), args...); int64(x); make([]int, n); new(T); obj.Method(x); empty() }\n"
	tree := parseUnderstandingTree(t, "main.go", []byte(source))
	defer tree.Release()
	p := richGoProgram(t, gotreesitter.FactCalls|gotreesitter.FactCallArguments)
	facts := p.Extract(tree)
	type arg = struct {
		text, kind, node string
		spread           bool
	}
	specs := []struct {
		call string
		args []arg
	}{}
	specs = append(specs, struct {
		call string
		args []arg
	}{"outer(x, pkg.Value, inner(a), T{X: 1}, func(y int) {}, 42, 1.5, 2i, \"s\", `raw`, 'r', true, false, nil, &x, *p, <-ch, a+b, xs[0], xs[:], x.(T), (x), args...)", []arg{
		{"x", "identifier", "identifier", false}, {"pkg.Value", "selector", "selector_expression", false}, {"inner(a)", "call", "call_expression", false}, {"T{X: 1}", "composite_literal", "composite_literal", false}, {"func(y int) {}", "func_literal", "func_literal", false},
		{"42", "literal", "int_literal", false}, {"1.5", "literal", "float_literal", false}, {"2i", "literal", "imaginary_literal", false}, {"\"s\"", "literal", "interpreted_string_literal", false}, {"`raw`", "literal", "raw_string_literal", false}, {"'r'", "literal", "rune_literal", false}, {"true", "literal", "true", false}, {"false", "literal", "false", false}, {"nil", "literal", "nil", false},
		{"&x", "unary", "unary_expression", false}, {"*p", "unary", "unary_expression", false}, {"<-ch", "unary", "unary_expression", false}, {"a+b", "binary", "binary_expression", false}, {"xs[0]", "index", "index_expression", false}, {"xs[:]", "slice", "slice_expression", false}, {"x.(T)", "type_assertion", "type_assertion_expression", false}, {"(x)", "parenthesized", "parenthesized_expression", false}, {"args...", "identifier", "variadic_argument", true},
	}})
	for _, w := range []struct {
		call string
		args []arg
	}{
		{"inner(a)", []arg{{"a", "identifier", "identifier", false}}},
		{"int64(x)", []arg{{"x", "identifier", "identifier", false}}},
		{"make([]int, n)", []arg{{"[]int", "type", "slice_type", false}, {"n", "identifier", "identifier", false}}},
		{"new(T)", []arg{{"T", "type", "type_identifier", false}}},
		{"obj.Method(x)", []arg{{"x", "identifier", "identifier", false}}},
	} {
		specs = append(specs, w)
	}
	var want []gotreesitter.CallArgumentFact
	for _, w := range specs {
		callAt := strings.Index(source, w.call)
		next := strings.Index(w.call, "(") + 1
		for i, a := range w.args {
			at := strings.Index(w.call[next:], a.text) + next
			next = at + len(a.text)
			want = append(want, gotreesitter.CallArgumentFact{CallStartByte: uint32(callAt), CallEndByte: uint32(callAt + len(w.call)), Index: i, Kind: a.kind, NodeType: a.node, StartByte: uint32(callAt + at), EndByte: uint32(callAt + at + len(a.text)), Spread: a.spread})
		}
	}
	if !reflect.DeepEqual(facts.CallArguments, want) {
		t.Fatalf("arguments = %#v; want %#v", facts.CallArguments, want)
	}
	for _, a := range facts.CallArguments {
		matches := 0
		for _, c := range facts.Calls {
			if c.StartByte == a.CallStartByte && c.EndByte == a.CallEndByte {
				matches++
			}
		}
		if matches != 1 {
			t.Fatalf("argument joins %d calls: %#v", matches, a)
		}
	}
	if len(facts.Calls) != 7 {
		t.Fatalf("calls = %#v", facts.Calls)
	}
	only := richGoProgram(t, gotreesitter.FactCallArguments).Extract(tree)
	if !reflect.DeepEqual(only.CallArguments, facts.CallArguments) || len(only.Calls) != 0 {
		t.Fatal("argument-only selection changed join")
	}
}

func TestFactRichOptInAndReuse(t *testing.T) {
	tree := parseUnderstandingTree(t, "main.go", []byte(goDeclarationsExample))
	defer tree.Release()
	legacy, err := gotreesitter.NewFactProgram(tree.Language(), gotreesitter.FactAll)
	if err != nil {
		t.Fatal(err)
	}
	configured := richGoProgram(t, gotreesitter.FactAll)
	var facts gotreesitter.FactSet
	plain := testing.AllocsPerRun(20, func() { facts = legacy.Extract(tree) })
	off := testing.AllocsPerRun(20, func() { facts = configured.Extract(tree) })
	if plain != off || facts.Signatures != nil || facts.CallArguments != nil {
		t.Fatal("disabled cost or output changed")
	}
	assertFactSetsEqual(t, facts, legacy.Extract(tree))
	if gotreesitter.FactAll != 15 {
		t.Fatal("FactAll changed")
	}
	for _, kind := range []gotreesitter.FactKind{gotreesitter.FactSignatures, gotreesitter.FactCallArguments} {
		noRules, e := gotreesitter.NewFactProgram(tree.Language(), kind)
		if e != nil {
			t.Fatal(e)
		}
		f := noRules.Extract(tree)
		if len(f.Signatures) != 0 || len(f.CallArguments) != 0 {
			t.Fatal("rules invented")
		}
	}
	entry := grammars.DetectLanguage("main.go")
	replaced, e := gotreesitter.NewFactProgramWithOptions(tree.Language(), gotreesitter.FactSignatures|gotreesitter.FactCallArguments,
		gotreesitter.WithSignatureRules(grammars.SignatureRules(*entry)), gotreesitter.WithSignatureRules(nil),
		gotreesitter.WithCallArgumentRules(grammars.CallArgumentRules(*entry)), gotreesitter.WithCallArgumentRules(nil))
	if e != nil {
		t.Fatal(e)
	}
	if f := replaced.Extract(tree); len(f.Signatures) != 0 || len(f.CallArguments) != 0 {
		t.Fatal("later options did not replace rules")
	}

	full := richGoProgram(t, gotreesitter.FactAll|gotreesitter.FactDeclarations|gotreesitter.FactSignatures|gotreesitter.FactCallArguments)
	tree2 := parseUnderstandingTree(t, "main.go", []byte("package p\nfunc F(x int) { g(x) }\n"))
	defer tree2.Release()
	facts = full.Extract(tree2)
	sig := facts.Signatures
	args := facts.CallArguments
	full.ExtractInto(tree2, &facts)
	if !reflect.DeepEqual(facts, full.Extract(tree2)) {
		t.Fatal("ExtractInto differs")
	}
	if &facts.Signatures[0] != &sig[0] || &facts.CallArguments[0] != &args[0] {
		t.Fatal("outer storage not reused")
	}
	legacy.ExtractInto(tree, &facts)
	if len(facts.Signatures) != 0 || len(facts.CallArguments) != 0 || !reflect.DeepEqual(sig[0], gotreesitter.SignatureFact{}) || args[0] != (gotreesitter.CallArgumentFact{}) {
		t.Fatal("stale facts retained")
	}
	full.ExtractInto(nil, &facts)
	other := parseUnderstandingTree(t, "main.py", []byte("def f(): pass\n"))
	defer other.Release()
	full.ExtractInto(other, &facts)
	if len(facts.Signatures) != 0 || len(facts.CallArguments) != 0 {
		t.Fatal("guard failed")
	}
}

func TestFactDeclarationsGoAnonymousSiblingOwners(t *testing.T) {
	const source = `package p
type Alias = struct { Proxy *struct { Live bool }; A, B struct { Code int } }
var first, second struct { Debug bool }
`
	tree := parseUnderstandingTree(t, "main.go", []byte(source))
	defer tree.Release()
	alias := "Alias = struct { Proxy *struct { Live bool }; A, B struct { Code int } }"
	assertDeclarations(t, source, declarationProgram(t, gotreesitter.FactDeclarations).Extract(tree).Declarations,
		declarationWant{"Alias", "type", "type_alias", alias, "alias", "", "", false},
		declarationWant{"Proxy", "field", "field_declaration", "Proxy *struct { Live bool }", "", "Alias", alias, false},
		declarationWant{"Live", "field", "field_declaration", "Live bool", "", "Alias.Proxy", "Proxy *struct { Live bool }", false},
		declarationWant{"A", "field", "field_declaration", "A, B struct { Code int }", "", "Alias", alias, false},
		declarationWant{"B", "field", "field_declaration", "A, B struct { Code int }", "", "Alias", alias, false},
		declarationWant{"Code", "field", "field_declaration", "Code int", "", "Alias.A", "A, B struct { Code int }", false},
		declarationWant{"Code", "field", "field_declaration", "Code int", "", "Alias.B", "A, B struct { Code int }", false},
		declarationWant{"first", "variable", "var_spec", "first, second struct { Debug bool }", "", "", "", false},
		declarationWant{"second", "variable", "var_spec", "first, second struct { Debug bool }", "", "", "", false},
		declarationWant{"Debug", "field", "field_declaration", "Debug bool", "", "first", "first, second struct { Debug bool }", false},
		declarationWant{"Debug", "field", "field_declaration", "Debug bool", "", "second", "first, second struct { Debug bool }", false})
}

func TestFactCallArgumentsGoOtherAndComments(t *testing.T) {
	const source = "package p\nfunc F(){ f(/* before */ x, y /* tail */ ...) }\n"
	tree := parseUnderstandingTree(t, "main.go", []byte(source))
	defer tree.Release()
	entry := grammars.DetectLanguage("main.go")
	rules := grammars.CallArgumentRules(*entry)
	rules[0].Kinds = nil // Unknown named expressions retain their real node type.
	p, err := gotreesitter.NewFactProgramWithOptions(tree.Language(), gotreesitter.FactCallArguments, gotreesitter.WithCallArgumentRules(rules))
	if err != nil {
		t.Fatal(err)
	}
	got := p.Extract(tree).CallArguments
	call := "f(/* before */ x, y /* tail */ ...)"
	callAt := strings.Index(source, call)
	var want []gotreesitter.CallArgumentFact
	for i, w := range []struct {
		text, node string
		spread     bool
	}{{"x", "identifier", false}, {"y /* tail */ ...", "variadic_argument", true}} {
		at := strings.Index(source, w.text)
		want = append(want, gotreesitter.CallArgumentFact{CallStartByte: uint32(callAt), CallEndByte: uint32(callAt + len(call)), Index: i, Kind: "other", NodeType: w.node, StartByte: uint32(at), EndByte: uint32(at + len(w.text)), Spread: w.spread})
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("facts = %#v; want %#v", got, want)
	}
}
