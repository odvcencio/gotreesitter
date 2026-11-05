//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestKotlinCompleteStatementsMatchLockedC compares both ambiguous statements
// and their neighboring forms with the grammar and runtime pinned for C.
func TestKotlinCompleteStatementsMatchLockedC(t *testing.T) {
	language := grammars.KotlinLanguage()
	cLanguage, languageError := COracleLanguage("kotlin")
	if languageError != nil {
		t.Fatal(languageError)
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if languageError := cParser.SetLanguage(cLanguage); languageError != nil {
		t.Fatal(languageError)
	}
	folder := "../grammars/testdata/kotlin_complete_statements"
	files, readError := os.ReadDir(folder)
	if readError != nil {
		t.Fatal(readError)
	}
	for _, file := range files {
		t.Run(file.Name(), func(t *testing.T) {
			source, readError := os.ReadFile(filepath.Join(folder, file.Name()))
			if readError != nil {
				t.Fatal(readError)
			}
			parser := gotreesitter.NewParser(language)
			tree, parseError := parser.ParseStrict(source)
			if parseError != nil {
				t.Fatal(parseError)
			}
			t.Cleanup(tree.Release)
			cTree := cParser.Parse(source, nil)
			if cTree == nil {
				t.Fatal("locked C parse returned no tree")
			}
			t.Cleanup(cTree.Close)
			assertLockedCTreeExact(t, file.Name(), tree, language, cTree)
		})
	}
}
