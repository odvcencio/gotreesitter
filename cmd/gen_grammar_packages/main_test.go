package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

func TestGeneratedPackagesAndCompatibility(t *testing.T) {
	changeTestDirectory(t, t.TempDir())
	for _, dir := range []string{"grammars/runtime", "grammars/grammar_blobs"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, data string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("grammars/grammar_blobs/go.bin", "grammar fixture")
	write("grammars/grammar_blobs/python.bin", "second fixture")
	write("grammars/runtime/scanner.go", "package grammarruntime\ntype Scanner struct{}\nfunc Public(_ int, rest ...string) int { return 0 }\n")
	if err := generate(false); err != nil {
		t.Fatal(err)
	}
	if err := generate(true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("grammars/go/go.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"package golang", "grammarblobs.Go", `grammarruntime.Language("go.bin")`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("Go wrapper lacks %q", want)
		}
	}
	data, err = os.ReadFile("grammars/scanner.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"type Scanner = grammarruntime.Scanner", "func Public(", "grammarruntime.Public(arg0_0, rest...)"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("compatibility source lacks %q: %s", want, data)
		}
	}
	write("grammars/go/go.go", "modified")
	if err := generate(true); err == nil {
		t.Fatal("check accepted modified output")
	}
	if err := generate(false); err != nil {
		t.Fatal(err)
	}
	write("grammars/obsolete.go", marker+"package grammars\n")
	write("grammars/manual.go", "package grammars\n")
	if err := generate(true); err == nil {
		t.Fatal("check accepted obsolete output")
	}
	if err := generate(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("grammars/obsolete.go"); !os.IsNotExist(err) {
		t.Fatalf("obsolete output remains: %v", err)
	}
	if _, err := os.Stat("grammars/manual.go"); err != nil {
		t.Fatal(err)
	}
}

func TestCompatibilitySkipsRuntimeWiring(t *testing.T) {
	source := []byte(`package grammarruntime
type BashExternalScanner struct{}
type ExternalScanner struct{}
type GoExternalScanner struct{}
type Kept struct{}
type ReservedWordTable struct{}
func RegisterBashSupport() {}
func RegisterExternalScanner(name string) {}
func LookupExternalScanner(name string) int { return 0 }
func RegisterReservedWords(name string, table ReservedWordTable) {}
func LookupReservedWords(name string) (ReservedWordTable, bool) { return ReservedWordTable{}, false }
func DecodeAndCertifyLanguageBlob(name string) error { return nil }
`)
	fs := token.NewFileSet()
	file, err := parser.ParseFile(fs, "source.go", source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	got := string(forwarders(fs, file, source))
	for _, want := range []string{
		"type ExternalScanner =", "type GoExternalScanner =", "type Kept =",
		"func RegisterExternalScanner(", "func LookupExternalScanner(",
		// cmd/ts2go sidecars call these without a package qualifier.
		"type ReservedWordTable =", "func RegisterReservedWords(", "func LookupReservedWords(",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("compatibility source lacks %q: %s", want, got)
		}
	}
	for _, unwanted := range []string{"BashExternalScanner", "RegisterBashSupport", "DecodeAndCertifyLanguageBlob"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("compatibility source forwards %q: %s", unwanted, got)
		}
	}
}

// TestToolsDoNotEmitOmittedFacadeNames fails when a Go tool under cmd/ writes a
// name that the facade omits into generated source. Static analysis misses
// these uses because the name sits inside a string literal, and .canopyignore
// hides cmd/ts2go. A name that a tool emits must stay out of skippedForwarders.
func TestToolsDoNotEmitOmittedFacadeNames(t *testing.T) {
	changeTestDirectory(t, filepath.Join("..", ".."))
	omitted := map[string]bool{}
	runtimeFiles, err := filepath.Glob("grammars/runtime/*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range runtimeFiles {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					if s, ok := spec.(*ast.TypeSpec); ok && ast.IsExported(s.Name.Name) && skipForwarder(s.Name.Name, true) {
						omitted[s.Name.Name] = true
					}
				}
			case *ast.FuncDecl:
				if d.Recv == nil && ast.IsExported(d.Name.Name) && skipForwarder(d.Name.Name, false) {
					omitted[d.Name.Name] = true
				}
			}
		}
	}
	if len(omitted) == 0 {
		t.Fatal("found no omitted facade names; the scan is not looking at grammars/runtime")
	}
	scanned := 0
	err = filepath.WalkDir("cmd", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path == filepath.Join("cmd", "gen_grammar_packages") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fs := token.NewFileSet()
		file, err := parser.ParseFile(fs, path, nil, 0)
		if err != nil {
			return err
		}
		scanned++
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			// Unquote so that an escape such as \t does not glue to the next word.
			text, err := strconv.Unquote(lit.Value)
			if err != nil {
				text = lit.Value
			}
			for _, word := range strings.FieldsFunc(text, func(r rune) bool {
				return !(r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r))
			}) {
				if omitted[word] {
					t.Errorf("%s emits %s, which the grammars facade omits; remove it from skippedForwarders or emit grammarruntime.%s", fs.Position(lit.Pos()), word, word)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned == 0 {
		t.Fatal("scanned no tool sources under cmd/")
	}
}

func TestCompatibilityPreservesBuildConstraint(t *testing.T) {
	source := []byte("//go:build grammar_subset_go\n\npackage grammarruntime\nconst Value = 2\n")
	fs := token.NewFileSet()
	file, err := parser.ParseFile(fs, "source.go", source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	got := string(forwarders(fs, file, source))
	if !strings.HasPrefix(got, "//go:build grammar_subset_go\n") {
		t.Fatal("compatibility source lost its build constraint")
	}
}

func TestGeneratorRejectsMissingBlobs(t *testing.T) {
	changeTestDirectory(t, t.TempDir())
	if err := generate(true); err == nil {
		t.Fatal("check accepted a missing blob directory")
	}
}

func TestRepositoryGeneratedFilesAreCurrent(t *testing.T) {
	changeTestDirectory(t, filepath.Join("..", ".."))
	if err := generate(true); err != nil {
		t.Fatal(err)
	}
}

func changeTestDirectory(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Error(err)
		}
	})
}
