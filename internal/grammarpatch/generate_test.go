package grammarpatch

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRegeneratePinnedGenerator(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX shell")
	}
	for _, version := range []string{"0.24.7", "0.26.5"} {
		t.Run(version, func(t *testing.T) {
			dir := t.TempDir()
			grammarDir := filepath.Join(dir, "grammar")
			if err := os.Mkdir(grammarDir, 0755); err != nil {
				t.Fatal(err)
			}
			script := "#!/bin/sh\nif [ \"$1\" = --version ]; then echo tree-sitter " + version + "; exit; fi\n[ \"$1\" = generate ] || exit 1\nprintf generated > parser.marker\n"
			if err := os.WriteFile(filepath.Join(dir, "tree-sitter"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			err := Regenerate(dir, "grammar/src", Spec{GeneratorVersion: "0.24.7"})
			if version != "0.24.7" {
				if err == nil || !strings.Contains(err.Error(), "0.24.7 required") {
					t.Fatalf("wrong-version error = %v", err)
				}
				if _, err := os.Stat(filepath.Join(grammarDir, "parser.marker")); !os.IsNotExist(err) {
					t.Fatal("wrong generator was executed")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(grammarDir, "parser.marker")); err != nil {
				t.Fatal("generator did not run in grammar directory:", err)
			}
		})
	}
}

func TestRegenerateGrammarOwnedGenerator(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX shell")
	}
	dir := t.TempDir()
	generatorDir := filepath.Join(dir, "node_modules", ".bin")
	if err := os.MkdirAll(generatorDir, 0755); err != nil {
		t.Fatal(err)
	}
	grammarDir := filepath.Join(dir, "typescript")
	if err := os.Mkdir(grammarDir, 0755); err != nil {
		t.Fatal(err)
	}
	scripts := map[string]string{
		filepath.Join(dir, "npm"):                  "#!/bin/sh\n[ \"$1\" = ci ] || exit 1\nprintf installed > install.marker\n",
		filepath.Join(generatorDir, "tree-sitter"): "#!/bin/sh\n[ \"$1\" = generate ] || exit 1\n[ -f ../install.marker ] || exit 1\nprintf generated > parser.marker\n",
	}
	for path, script := range scripts {
		if err := os.WriteFile(path, []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	if err := Regenerate(dir, "typescript/src", Spec{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(grammarDir, "parser.marker")); err != nil {
		t.Fatal("grammar-owned generator did not run:", err)
	}
}
