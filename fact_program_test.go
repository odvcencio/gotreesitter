package gotreesitter_test

import (
	"encoding/json"
	"os"
	"path/filepath"
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
}

func assertDeclarations(t *testing.T, source string, defs []gotreesitter.DeclarationFact, want ...declarationWant) {
	t.Helper()
	if len(defs) != len(want) {
		t.Fatalf("definitions = %#v; want %d facts", defs, len(want))
	}
	for i, w := range want {
		start := strings.Index(source, w.construct)
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
			Shape: w.shape, Container: w.container,
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
		declarationWant{"Reader", "type", "type_spec", reader, "interface", "", ""},
		declarationWant{"Read", "method", "method_elem", "Read(dst []byte) (int, error)", "", "Reader", reader},
		declarationWant{"Record", "type", "type_spec", record, "struct", "", ""},
		declarationWant{"Key", "field", "field_declaration", "Key, Value string", "", "Record", record},
		declarationWant{"Value", "field", "field_declaration", "Key, Value string", "", "Record", record},
		declarationWant{"Alias", "type", "type_alias", "Alias = Record", "alias", "", ""},
		declarationWant{"first", "variable", "var_spec", "first, second string", "", "", ""},
		declarationWant{"second", "variable", "var_spec", "first, second string", "", "", ""},
		declarationWant{"left", "constant", "const_spec", "left, right = 1, 2", "", "", ""},
		declarationWant{"right", "constant", "const_spec", "left, right = 1, 2", "", "", ""},
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
			want = append(want, declarationWant{name, group.kind, group.nodeType, group.construct, "", "", ""})
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
		declarationWant{"Record", "type", "type_spec", record, "struct", "", ""},
		declarationWant{"Named", "field", "field_declaration", "Named struct { hidden int }", "", "Record", record},
		declarationWant{"Reader", "type", "type_spec", reader, "interface", "", ""},
		declarationWant{"Read", "method", "method_elem", "Read()", "", "Reader", reader},
		declarationWant{"anonymous", "variable", "var_spec", "anonymous struct { hiddenVar int }", "", "", ""},
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
		declarationWant{"List", "type", "type_spec", "List[T any] struct { head *T }", "struct", "", ""},
		declarationWant{"head", "field", "field_declaration", "head *T", "", "List", "List[T any] struct { head *T }"},
		declarationWant{"Count", "type", "type_spec", "Count int", "", "", ""},
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
