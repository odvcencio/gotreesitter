package lexexact

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestCorpusHeadersAndSources checks a folder of grammar checkouts: every
// tree_sitter/parser.h next to a parser.c must be one of the known headers, and
// every parser.c next to one must pass Parse (header identity, whole-file item
// rule and extraction). Set LEXEXACT_CORPUS to the folder; without it the test skips.
func TestCorpusHeadersAndSources(t *testing.T) {
	root := os.Getenv("LEXEXACT_CORPUS")
	if root == "" {
		t.Skip("LEXEXACT_CORPUS is not set: no folder of grammar checkouts to scan")
	}
	if _, err := os.Stat(root); err != nil {
		t.Skipf("LEXEXACT_CORPUS=%s is not readable: %v", root, err)
	}
	var sources []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == "parser.c" {
			sources = append(sources, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	distinct := map[string]int{}
	headers, known, accepted, refused := 0, 0, 0, 0
	for _, path := range sources {
		hb, err := os.ReadFile(filepath.Join(filepath.Dir(path), "tree_sitter", "parser.h"))
		if err != nil {
			continue
		}
		headers++
		h := string(hb)
		distinct[headerHash(h)]++
		if entry, err := parseHeader(h); err == nil {
			known++
			if want := factsOf(h); entry != want {
				t.Errorf("%s: entry %+v differs from the facts read from the header %+v", path, entry, want)
			}
		} else {
			t.Errorf("%s: %v", path, err)
		}
		c, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parseInc(string(c), h); err != nil {
			refused++
			t.Errorf("%s: %v", path, err)
		} else {
			accepted++
		}
	}
	t.Logf("corpus: %d parser.c files with a parser.h; %d distinct headers, %d of %d resolve to an entry; %d parser.c accepted, %d refused", headers, len(distinct), known, headers, accepted, refused)
	if headers == 0 {
		t.Fatal("no src/parser.c with a src/tree_sitter/parser.h under " + root)
	}
}
