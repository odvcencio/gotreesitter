//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestMakeFunctionVariablesCOracle(t *testing.T) {
	data, err := os.ReadFile("../grammars/testdata/make_function_variables.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Source string
		Error        bool
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	cLanguage, err := ParityCLanguage("make")
	if err != nil {
		t.Fatalf("C make oracle unavailable: %v", err)
	}
	goLanguage := grammars.MakeLanguage()
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			source := []byte(tc.Source)
			cTree := compactT3ParseC(t, cLanguage, source)
			defer cTree.Close()
			goTree, err := gotreesitter.NewParser(goLanguage).Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer goTree.Release()
			cRoot, goRoot := cTree.RootNode(), goTree.RootNode()
			if cRoot == nil || goRoot == nil {
				t.Fatal("missing root")
			}
			if cRoot.HasError() != tc.Error || goRoot.HasError() != tc.Error {
				t.Fatalf("expected HasError=%v; C: %s; Go: %s", tc.Error, cRoot.ToSexp(), goRoot.SExpr(goLanguage))
			}
			if cRoot.EndByte() != uint(len(source)) || goRoot.EndByte() != uint32(len(source)) {
				t.Fatal("root does not cover source")
			}
			var errs []string
			compareNodes(goRoot, goLanguage, cRoot, "root", &errs)
			if len(errs) != 0 {
				t.Fatalf("Go/C trees diverged: %s", joinTopErrors(errs))
			}
		})
	}
}

// The oracle harness is shared across baseline and candidate root modules.
type parityLegacyGeneratorSpec struct{}
type parityGeneratorProbe struct{ called *bool }

func (p parityGeneratorProbe) RegenerateForHarness(repoDir, subdir string) error {
	*p.called = true
	return nil
}

func TestParityRegenerateMixedRevisionSpec(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX shell")
	}
	t.Run("legacy", func(t *testing.T) {
		dir := t.TempDir()
		grammar := filepath.Join(dir, "grammar")
		if err := os.MkdirAll(filepath.Join(dir, "node_modules", ".bin"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(grammar, 0755); err != nil {
			t.Fatal(err)
		}
		for path, body := range map[string]string{
			filepath.Join(dir, "npm"):                                 "#!/bin/sh\n[ \"$1\" = ci ] || exit 1\n",
			filepath.Join(dir, "node_modules", ".bin", "tree-sitter"): "#!/bin/sh\n[ \"$1\" = generate ] || exit 1\nprintf yes > generated\n",
		} {
			if err := os.WriteFile(path, []byte(body), 0755); err != nil {
				t.Fatal(err)
			}
		}
		t.Setenv("PATH", dir)
		if err := regenerateParityGrammar(dir, "grammar/src", parityLegacyGeneratorSpec{}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(grammar, "generated")); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("capability", func(t *testing.T) {
		called := false
		if err := regenerateParityGrammar("repo", "grammar/src", parityGeneratorProbe{&called}); err != nil || !called {
			t.Fatalf("dispatch: called=%v err=%v", called, err)
		}
	})
}
