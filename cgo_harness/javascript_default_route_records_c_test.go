//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/cgo_harness/internal/corecords"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestJavaScriptDefaultRouteRecordsC checks the default route of a certified
// forest-default grammar against pinned C. The fresh parse runs on the ordinary
// core, so its raw records equal C's record for record. The first edit after it
// reuses the fresh tree: the reparse is served incrementally, reuses as many
// subtrees as C's reuse_node events, and its records equal C's incremental
// records. A forest-elected fresh tree differed from C in parse states on these
// inputs, and the first edit reparsed the whole file.
func TestJavaScriptDefaultRouteRecordsC(t *testing.T) {
	lang, oracle := ccoreLanguages(t, "javascript")
	if !gts.LanguageWantsForest(lang) {
		t.Fatal("JavaScript is not forest-default; the test needs a forest-default grammar")
	}
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	type edit struct {
		start, end int
		text       string
	}
	cases := []struct {
		name string
		src  []byte
		edit edit
	}{
		{"function_declaration", []byte("function f() {}\n"), edit{16, 16, "\n"}},
		{"class_method", []byte("class A { m() {} }\n"), edit{19, 19, "\n"}},
		{"function_expression", []byte("f = function () {}\n"), edit{19, 19, "\n"}},
		{"function_declaration_rename", []byte("function f() {}\n"), edit{9, 10, "g"}},
	}
	if w5 := javascriptW5Fixture(t); w5 != nil {
		// The editor-latency fixture and its one-byte workload.
		cases = append(cases, struct {
			name string
			src  []byte
			edit edit
		}{"w5_one_byte", w5, edit{5133, 5134, "Z"}})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.src
			next := append(append(append([]byte(nil), src[:tc.edit.start]...), tc.edit.text...), src[tc.edit.end:]...)
			ie := gts.InputEdit{
				StartByte: uint32(tc.edit.start), OldEndByte: uint32(tc.edit.end), NewEndByte: uint32(tc.edit.start + len(tc.edit.text)),
				StartPoint: gradPoint(src, tc.edit.start), OldEndPoint: gradPoint(src, tc.edit.end),
				NewEndPoint: gradPoint(next, tc.edit.start+len(tc.edit.text)),
			}

			var clog []string
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(oracle); err != nil {
				t.Fatal(err)
			}
			cp.SetLogger(func(kind sitter.LogType, msg string) {
				if kind == sitter.LogTypeParse {
					clog = append(clog, msg)
				}
			})
			cFresh := cp.Parse(src, nil)
			if cFresh == nil {
				t.Fatal("pinned C fresh parse failed")
			}
			defer cFresh.Close()
			cFreshRecords := javascriptCRecords(t, cFresh)
			cOld := cFresh.Clone()
			defer cOld.Close()
			cOld.Edit(gradSitterEdit(ie))
			clog = clog[:0]
			cIncr := cp.Parse(next, cOld)
			if cIncr == nil {
				t.Fatal("pinned C incremental parse failed")
			}
			defer cIncr.Close()
			cReused := uint64(gradCount(clog, "reuse_node "))
			cIncrRecords := javascriptCRecords(t, cIncr)

			p := gts.NewParser(lang)
			_, freshBefore, _ := bind.RouteCounters()
			tree, err := p.Parse(src)
			_, freshAfter, _ := bind.RouteCounters()
			if err != nil || tree == nil {
				t.Fatalf("default fresh parse: %v", err)
			}
			defer tree.Release()
			if freshAfter-freshBefore != 1 || tree.UsedForestFastPath() {
				t.Fatalf("default fresh parse: core served %d, forest fast path %t; want 1 and false", freshAfter-freshBefore, tree.UsedForestFastPath())
			}
			if diff := javascriptRecordDiff(javascriptGoRecords(tree), cFreshRecords); diff != "" {
				t.Fatalf("default fresh records differ from pinned C: %s", diff)
			}

			tree.Edit(ie)
			fallbacksBefore := bind.IncrFallbacks()
			_, freshBefore, incrBefore := bind.RouteCounters()
			edited, profile, err := p.ParseIncrementalProfiled(next, tree)
			_, freshAfter, incrAfter := bind.RouteCounters()
			fallbacksAfter := bind.IncrFallbacks()
			if err != nil || edited == nil {
				t.Fatalf("first edit: %v", err)
			}
			if edited != tree {
				defer edited.Release()
			}
			notCertified := fallbacksAfter["incr_not_certified"] - fallbacksBefore["incr_not_certified"]
			if incrAfter-incrBefore != 1 || freshAfter != freshBefore || notCertified != 0 {
				t.Fatalf("first edit: served incremental %d, fresh %d, incr_not_certified %d; want 1, 0, 0", incrAfter-incrBefore, freshAfter-freshBefore, notCertified)
			}
			if profile.ReusedSubtrees == 0 || profile.ReusedSubtrees != cReused {
				t.Fatalf("first edit reused %d subtrees (%d bytes), pinned C %d", profile.ReusedSubtrees, profile.ReusedBytes, cReused)
			}
			if diff := javascriptRecordDiff(javascriptGoRecords(edited), cIncrRecords); diff != "" {
				t.Fatalf("first edit records differ from pinned C's incremental records: %s", diff)
			}
			t.Logf("%s: fresh records %d equal C; first edit reused %d subtrees and %d bytes, C %d reuse_node; records %d equal C incremental",
				tc.name, len(cFreshRecords), profile.ReusedSubtrees, profile.ReusedBytes, cReused, len(cIncrRecords))
		})
	}
}

func javascriptW5Fixture(t *testing.T) []byte {
	t.Helper()
	f, err := os.Open("../internal/benchfixtures/testdata/cliffs/javascript_async_hooks.js.gz")
	if err != nil {
		t.Logf("editor-latency fixture: %v", err)
		return nil
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(z)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func javascriptGoRecords(tree *gts.Tree) []string {
	var b bytes.Buffer
	bind.CompactRecords(tree, &b)
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func javascriptCRecords(t *testing.T, tree *sitter.Tree) []string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "c.records")
	if _, _, rc := corecords.Hash(tree, path); rc != 0 {
		t.Fatal("pinned C records failed")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

// javascriptRecordDiff returns "" when the records are equal, or the number of
// differing rows and the first one.
func javascriptRecordDiff(got, want []string) string {
	if len(got) != len(want) {
		return fmt.Sprintf("row count %d, C %d", len(got), len(want))
	}
	differ, first := 0, ""
	for i := range got {
		if got[i] != want[i] {
			if differ == 0 {
				first = fmt.Sprintf("row %d: %s | C %s", i, got[i], want[i])
			}
			differ++
		}
	}
	if differ == 0 {
		return ""
	}
	return fmt.Sprintf("%d of %d rows differ, first %s", differ, len(got), first)
}
