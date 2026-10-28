//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// eofNewlineWitness is a valid sample with its final newline removed, or a
// tiny input that ends at the same point. The
// locked C parser reports a hidden MISSING newline token or accepts a
// zero-width or NUL token at the end of the input; the default engine must
// produce the same tree, error flags included.
type eofNewlineWitness struct {
	lang   string
	source string
}

var eofNewlineWitnesses = []eofNewlineWitness{
	// hidden MISSING token under visible ancestors (parser_reduce.go, parser.go).
	{"chatito", "%[greet]\n    hi"},
	{"make", "all:\n\t@echo hi"},
	{"beancount", "2024-01-01 open Assets:Bank"},
	{"cylc", "[scheduling]"},
	{"desktop", "[Desktop Entry]\nEncoding=UTF-8\nType=Application\nName=Weston\nComment=Wayland Compostitor\nExec=weston\nIcon=weston\nTerminal=false\nCategories=Utility;"},
	{"disassembly", "0x400601 <__libc_csu_init+33>   sub    %r12,%rbp\n1000: 55                   push   rbp"},
	{"git_config", "[core]\n\tbare = false"},
	{"ini", "[section]\nkey = value"},
	{"kconfig", "config FOO\n  bool \"Enable foo\""},
	{"ninja", "rule cc\n  command = gcc"},
	{"robot", "*** Test Cases ***\nTest\n  Log  hello"},
	{"ssh_config", "Host example\n  User admin"},
	// zero-width token and NUL token at end of input (lexer.go).
	{"tlaplus", "---- MODULE Test ----\n===="},
	{"bitbake", "SUMMARY = \"hello\""},
	{"earthfile", "FROM alpine"},
	{"gitattributes", "*.txt text"},
	// tiny inputs that end where the lexer's end-of-input path accepts a
	// zero-width token (lexer.go).
	{"asm", "#"},
	{"awk", "#"},
	{"cylc", "#"},
	{"disassembly", "#"},
	{"hyprlang", "#"},
	{"nushell", "#"},
	{"fennel", ";"},
	{"editorconfig", "root ="},
	{"commonlisp", "(defun "},
	// external scanner emits no newline at end of input (pug_scanner.go).
	{"pug", "p hello"},
	// incomplete forest parse is not a clean root (glr_forest.go).
	{"ledger", "2024-01-01 Groceries\n  Expenses:Food  $50\n  Assets:Bank"},
}

func assertEOFNewlineMatchesLockedC(t *testing.T, lang, source string) {
	t.Helper()
	cLang, err := ParityCLanguage(lang)
	if err != nil {
		t.Fatalf("load C oracle: %v", err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cLang); err != nil {
		t.Fatalf("set C language: %v", err)
	}
	src := []byte(source)
	cTree := cp.Parse(src, nil)
	if cTree == nil {
		t.Fatal("C tree is nil")
	}
	defer cTree.Close()
	goTree, goLang, err := parseWithGo(parityCase{name: lang, source: source}, src, nil)
	if err != nil {
		t.Fatalf("parse with Go: %v", err)
	}
	defer releaseGoTree(goTree)
	if goTree.RootNode().HasError() != cTree.RootNode().HasError() {
		t.Fatalf("HasError Go=%v C=%v", goTree.RootNode().HasError(), cTree.RootNode().HasError())
	}
	assertLockedCTreeExactWithErrors(t, lang, goTree, goLang, cTree)
}

func TestEOFNewlineMatchesLockedC(t *testing.T) {
	for _, w := range eofNewlineWitnesses {
		w := w
		t.Run(w.lang, func(t *testing.T) {
			assertEOFNewlineMatchesLockedC(t, w.lang, w.source)
		})
	}
}

// eofNewlineRealSampleLanguages lists the languages whose committed real
// sample matches the locked C parser with and without its final newline.
var eofNewlineRealSampleLanguages = []string{
	"bitbake", "chatito", "desktop", "diff", "eds", "git_config", "http", "ini", "ledger", "ninja", "pug", "vimdoc",
}

func TestEOFNewlineRealSamplesMatchLockedC(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "internal", "benchfixtures", "real_corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Entries []struct {
			Language      string `json:"language"`
			Role          string `json:"role"`
			CommittedPath string `json:"committed_path"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	for _, entry := range manifest.Entries {
		if entry.Role == "sample" && entry.CommittedPath != "" {
			paths[entry.Language] = entry.CommittedPath
		}
	}
	for _, lang := range eofNewlineRealSampleLanguages {
		lang := lang
		path, ok := paths[lang]
		if !ok {
			t.Fatalf("no committed real sample for %s", lang)
		}
		data, err := os.ReadFile(filepath.Join("..", "internal", "benchfixtures", path))
		if err != nil {
			t.Fatal(err)
		}
		withNewline := string(data)
		t.Run(lang+"/with_newline", func(t *testing.T) {
			assertEOFNewlineMatchesLockedC(t, lang, withNewline)
		})
		t.Run(lang+"/without_newline", func(t *testing.T) {
			assertEOFNewlineMatchesLockedC(t, lang, strings.TrimRight(withNewline, "\n"))
		})
	}
}
