package main

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
