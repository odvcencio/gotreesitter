package main

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
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
