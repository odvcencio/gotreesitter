package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestCompareTreesRejectsDifferentTreesWithEqualNodeCounts(t *testing.T) {
	lang := grammars.GoLanguage()
	parser := ts.NewParser(lang)
	parse := func(source string) *ts.Tree {
		tree, err := parser.Parse([]byte(source))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(tree.Release)
		return tree
	}
	left := parse("package p\nvar x = 1\n")
	same := parse("package p\nvar x = 2\n")
	different := parse("package p\nvar x = y\n")
	if countNodes(left.RootNode()) != countNodes(different.RootNode()) {
		t.Fatal("fixture must preserve node count")
	}
	if _, _, _, err := compareTrees(left.RootNode(), same.RootNode(), lang); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := compareTrees(left.RootNode(), different.RootNode(), lang); err == nil {
		t.Fatal("different node kinds passed comparison")
	}
	if _, _, _, err := compareTrees(nil, same.RootNode(), lang); err == nil {
		t.Fatal("missing root passed comparison")
	}
}

func TestRunModesAndFlushesProfile(t *testing.T) {
	t.Setenv("ISSUE454_CPUPROFILE", "")
	for _, mode := range []string{"full", "replace", "insert", "delete"} {
		t.Run(mode, func(t *testing.T) {
			if code := run([]string{"go", "1", mode, "3"}); code != 0 {
				t.Fatalf("exit code = %d", code)
			}
		})
	}
	profile := filepath.Join(t.TempDir(), "cpu.pprof")
	t.Setenv("ISSUE454_CPUPROFILE", profile)
	if code := run([]string{"go", "1", "invalid"}); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	f, err := os.Open(profile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil || len(data) == 0 {
		t.Fatalf("profile not flushed: bytes=%d err=%v", len(data), err)
	}
}

func TestCommentFixtureAndQueryMode(t *testing.T) {
	source, _ := gen("nushell-comments", 8<<10)
	if len(source) < 8<<10 || !bytes.HasPrefix(source, []byte("# note 0\n")) || !bytes.Contains(source, []byte("# note 754\n")) {
		t.Fatalf("unexpected comment fixture: %d bytes", len(source))
	}
	if code := run([]string{"nushell-comments", "1", "query", "1"}); code != 0 {
		t.Fatalf("query exit: %d", code)
	}
}

func TestReporterCommentHighlights(t *testing.T) {
	for _, name := range []string{"zig", "scss", "diff"} {
		t.Run(name, func(t *testing.T) {
			entry := grammars.DetectLanguageByName(name)
			h, err := ts.NewHighlighter(entry.Language(), entry.HighlightQuery)
			if err != nil {
				t.Fatal(err)
			}
			for _, size := range []int{2, 32} {
				source, _ := gen(name+"-comments", size<<10)
				ranges := h.Highlight(source)
				if len(ranges) == 0 {
					t.Fatalf("no highlights at %dKB", size)
				}
				comments := 0
				for _, r := range ranges {
					if strings.HasPrefix(r.Capture, "comment") {
						comments++
					}
					if r.Capture == "spell" {
						t.Fatalf("spell won at %dKB: %#v", size, r)
					}
				}
				if comments == 0 {
					t.Fatalf("no comment highlights at %dKB: %#v", size, ranges[:min(len(ranges), 3)])
				}
			}
		})
	}
}
