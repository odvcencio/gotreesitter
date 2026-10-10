package gotreesitter_test

import (
	"slices"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

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

func declarationProgram(t *testing.T, kinds gts.FactKind) *gts.FactProgram {
	t.Helper()
	entry := grammars.DetectLanguage("main.go")
	program, err := gts.NewFactProgram(entry.Language(), kinds,
		gts.WithDeclarationRules(grammars.DeclarationRules(*entry)))
	if err != nil {
		t.Fatal(err)
	}
	return program
}

type declarationWant struct {
	name, kind, nodeType, construct, shape, container, containerConstruct string
}

func assertDeclarations(t *testing.T, source string, defs []gts.DefinitionSpan, want ...declarationWant) {
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
		expected := gts.DefinitionSpan{
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

func TestFactDeclarationsGoExample(t *testing.T) {
	tree := parseUnderstandingTree(t, "main.go", []byte(goDeclarationsExample))
	defer tree.Release()
	program := declarationProgram(t, gts.FactDefinitions|gts.FactDeclarations)
	reader := "Reader interface {\n    Read(dst []byte) (int, error)\n}"
	record := "Record struct {\n    Key, Value string\n}"
	assertDeclarations(t, goDeclarationsExample, program.Extract(tree).Definitions,
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
		declarationWant{"String", "method", "method_declaration", "func (r Record) String() string { return r.Key }", "", "", ""},
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
	program := declarationProgram(t, gts.FactDeclarations)
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
	assertDeclarations(t, source, program.Extract(tree).Definitions, want...)
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
	defs := declarationProgram(t, gts.FactDefinitions|gts.FactDeclarations).Extract(tree).Definitions
	record := "Record struct {\n Embedded\n *Pointer\n Named struct { hidden int }\n _ int\n}"
	reader := "Reader interface {\n Embedded\n ~int | ~string\n Read()\n}"
	assertDeclarations(t, source, defs,
		declarationWant{"Record", "type", "type_spec", record, "struct", "", ""},
		declarationWant{"Named", "field", "field_declaration", "Named struct { hidden int }", "", "Record", record},
		declarationWant{"Reader", "type", "type_spec", reader, "interface", "", ""},
		declarationWant{"Read", "method", "method_elem", "Read()", "", "Reader", reader},
		declarationWant{"anonymous", "variable", "var_spec", "anonymous struct { hiddenVar int }", "", "", ""},
		declarationWant{"run", "function", "function_declaration", strings.TrimSuffix(source[strings.Index(source, "func run()"):], "\n"), "", "", ""},
	)
	offset := uint32(strings.Index(source, "local, other"))
	if enclosing, ok := gts.EnclosingDefinition(tree, offset); !ok || enclosing.Name != "run" {
		t.Fatalf("enclosing = %#v, %v", enclosing, ok)
	}
}

func TestFactDeclarationsGoGenericTypes(t *testing.T) {
	const source = "package p\ntype List[T any] struct { head *T }\ntype Count int\n"
	tree := parseUnderstandingTree(t, "main.go", []byte(source))
	defer tree.Release()
	assertDeclarations(t, source, declarationProgram(t, gts.FactDeclarations).Extract(tree).Definitions,
		declarationWant{"List", "type", "type_spec", "List[T any] struct { head *T }", "struct", "", ""},
		declarationWant{"head", "field", "field_declaration", "head *T", "", "List", "List[T any] struct { head *T }"},
		declarationWant{"Count", "type", "type_spec", "Count int", "", "", ""},
	)
}

func TestFactDeclarationsOptInAndReuse(t *testing.T) {
	tree := parseUnderstandingTree(t, "main.go", []byte(goDeclarationsExample))
	defer tree.Release()
	for _, kind := range []gts.FactKind{gts.FactDefinitions, gts.FactAll} {
		program := declarationProgram(t, kind)
		if got := program.Extract(tree).Definitions; !slices.Equal(got, gts.ExtractDefinitionSpans(tree)) {
			t.Fatalf("legacy definitions changed: %#v", got)
		}
	}
	if gts.FactAll&gts.FactDeclarations != 0 {
		t.Fatal("FactAll silently includes declarations")
	}
	noRules, err := gts.NewFactProgram(tree.Language(), gts.FactDeclarations)
	if err != nil {
		t.Fatal(err)
	}
	if got := noRules.Extract(tree); len(got.Definitions) != 0 {
		t.Fatalf("rules invented: %#v", got)
	}
	program := declarationProgram(t, gts.FactAll|gts.FactDeclarations)
	facts := program.Extract(tree)
	storage := facts
	program.ExtractInto(tree, &facts)
	assertFactSetsEqual(t, facts, program.Extract(tree))
	assertFactStorageReused(t, storage.Definitions, facts.Definitions)
	small := parseUnderstandingTree(t, "main.go", []byte("package p\nvar only int\n"))
	defer small.Release()
	program.ExtractInto(small, &facts)
	assertFactSetsEqual(t, facts, program.Extract(small))
	assertFactStorageReused(t, storage.Definitions, facts.Definitions)
	legacy := declarationProgram(t, gts.FactDefinitions)
	legacy.ExtractInto(tree, &facts)
	assertFactSetsEqual(t, facts, legacy.Extract(tree))
	assertFactStorageReused(t, storage.Definitions, facts.Definitions)
	program.ExtractInto(nil, &facts)
	assertFactStorageReused(t, storage.Definitions, facts.Definitions)
}
