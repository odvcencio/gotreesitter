package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseManifestWithExtensions(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "manifest.txt")
	content := `
# name repo subdir extensions
python https://github.com/tree-sitter/tree-sitter-python src .py,.pyi
go https://github.com/tree-sitter/tree-sitter-go src .go
`
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := ParseManifest(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if len(entries[0].Extensions) != 2 {
		t.Fatalf("python extensions = %d, want 2", len(entries[0].Extensions))
	}
	if entries[0].Extensions[0] != ".py" || entries[0].Extensions[1] != ".pyi" {
		t.Fatalf("python extensions = %#v", entries[0].Extensions)
	}
}

func TestSafeFileBase(t *testing.T) {
	if got := safeFileBase("tree-sitter-c-sharp"); got != "tree_sitter_c_sharp" {
		t.Fatalf("safeFileBase got %q", got)
	}
}

func TestLanguageFuncNameSanitize(t *testing.T) {
	if got := languageFuncName("c-sharp"); got != "CSharpLanguage" {
		t.Fatalf("languageFuncName = %q, want %q", got, "CSharpLanguage")
	}
}

func TestFindParserC(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(srcDir, "parser.c")
	if err := os.WriteFile(p, []byte("/* parser */"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := findParserC(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != p {
		t.Fatalf("findParserC = %q, want %q", got, p)
	}
}

func TestRunBatchManifestLocalRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "queries"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(repo, "src", "parser.c"), []byte(miniParserC), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "queries", "highlights.scm"), []byte("(number) @number\n"), 0644); err != nil {
		t.Fatal(err)
	}

	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, strings.TrimSpace(string(out)))
		}
	}
	run(repo, "init")
	run(repo, "config", "user.email", "test@example.com")
	run(repo, "config", "user.name", "test")
	run(repo, "add", ".")
	run(repo, "commit", "-m", "init")

	manifest := filepath.Join(root, "manifest.txt")
	line := "testlang " + repo + " src .tl\n"
	if err := os.WriteFile(manifest, []byte(line), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(root, "out")
	if err := RunBatchManifest(manifest, outDir, "grammars"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "testlang_grammar.go")); err != nil {
		t.Fatalf("missing generated grammar file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "testlang_register.go")); err != nil {
		t.Fatalf("missing generated register file: %v", err)
	}
}
