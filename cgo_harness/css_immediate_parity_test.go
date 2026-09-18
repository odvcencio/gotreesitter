//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"path/filepath"
	"testing"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestGeneratedCSSImmediateTokenLockedCParity(t *testing.T) {
	lockPath, err := findParityLockPath()
	if err != nil {
		t.Fatal(err)
	}
	lock, err := loadParityLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := lock["css"]
	if !ok {
		t.Fatal("CSS grammar is absent from languages.lock")
	}
	repoDir, ok := parityLocalRepoDir(entry)
	if !ok {
		t.Skip("set GTS_PARITY_REPO_ROOT to the seeded grammar root")
	}
	if err := verifyPinnedRepo(repoDir, entry.Commit); err != nil {
		t.Fatal(err)
	}
	grammar, err := importGrammargenSource(grammargenCGOGrammar{jsonPath: filepath.Join(repoDir, "src", "grammar.json")})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := grammargenGenerate(grammar, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	adaptGrammargenCGOExternalScanner("css", grammars.CssLanguage(), generated)
	cLanguage, err := COracleLanguage("css")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name   string
		source string
	}{
		{"escaped unit suffix", `body { padding-right: 8px\9; }`},
		{"ordinary unit", `body { padding-right: 8px; }`},
		{"unit before important", `body { padding-right: 8px!important; }`},
		{"nested escaped unit", `@media screen { body { padding-right: 8px\9; } }`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := []byte(tc.source)
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parser returned no tree")
			}
			defer cTree.Close()
			cRoot := cTree.RootNode()
			if cRoot.HasError() || cRoot.EndByte() != uint(len(source)) {
				t.Fatal("locked C fixture contains errors or omits source bytes")
			}
			parser := gotreesitter.NewParser(generated)
			parser.SetAdmissionCandidateRoute(false)
			tree, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			if tree == nil || tree.RootNode() == nil {
				t.Fatal("generated parser returned no tree")
			}
			defer tree.Release()
			if difference := FirstDivergenceDumpV1(tree.RootNode(), generated, cRoot); difference != nil {
				t.Fatalf("generated CSS differs from locked C: %+v", difference)
			}
		})
	}
}
