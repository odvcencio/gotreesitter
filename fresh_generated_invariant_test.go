package gotreesitter_test

import (
	"bytes"
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// Large fresh parses use transient reductions and arena retention policies that
// the smaller R4 fixture does not exercise. Keep edits valid to isolate tree
// rebuilding and reuse from recovery behavior.
func TestFreshGeneratedGoLargeInvariants(t *testing.T) {
	for _, size := range []int{137 << 10, 1 << 20} {
		t.Run(fmt.Sprintf("%dKiB", size>>10), func(t *testing.T) {
			source, _, err := benchfixtures.GeneratedSource("go", size)
			if err != nil {
				t.Fatal(err)
			}
			lang := grammars.GoLanguage()
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(false)
			var tree *gts.Tree
			var digest string
			for pass := 0; pass < 3; pass++ {
				next, err := parser.Parse(source)
				if err != nil {
					t.Fatal(err)
				}
				assertV1InvariantTree(t, "fresh", next, lang, source)
				inspection, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				if pass > 0 && inspection.SHA256 != digest {
					t.Fatalf("warm digest=%s cold=%s", inspection.SHA256, digest)
				}
				digest = inspection.SHA256
				if tree != nil {
					tree.Release()
				}
				tree = next
			}
			defer func() { tree.Release() }()
			allocs := testing.AllocsPerRun(5, func() {
				next, err := parser.ParseIncremental(source, tree)
				if err != nil {
					t.Fatal(err)
				}
				next.Release()
			})
			if allocs != 0 {
				t.Fatalf("no-edit allocations=%g, want 0", allocs)
			}
			offsets := []int{bytes.Index(source, []byte("func f")) + 5, len(source) / 2, bytes.LastIndex(source, []byte("func f")) + 5}
			offsets[1] += bytes.Index(source[offsets[1]:], []byte("func f")) + 5
			for step, at := range offsets {
				if at < 0 || at >= len(source) || source[at] != 'f' {
					t.Fatalf("invalid edit offset %d", at)
				}
				nextSource := append([]byte(nil), source...)
				nextSource[at] = 'g'
				row := bytes.Count(source[:at], []byte{'\n'})
				col := at - bytes.LastIndexByte(source[:at], '\n') - 1
				tree.Edit(gts.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at + 1), NewEndByte: uint32(at + 1), StartPoint: gts.Point{Row: uint32(row), Column: uint32(col)}, OldEndPoint: gts.Point{Row: uint32(row), Column: uint32(col + 1)}, NewEndPoint: gts.Point{Row: uint32(row), Column: uint32(col + 1)}})
				incremental, err := parser.ParseIncremental(nextSource, tree)
				if err != nil {
					t.Fatal(err)
				}
				freshParser := gts.NewParser(lang)
				freshParser.SetAdmissionCandidateRoute(false)
				fresh, err := freshParser.Parse(nextSource)
				if err != nil {
					t.Fatal(err)
				}
				assertV1InvariantTree(t, "incremental", incremental, lang, nextSource)
				assertV1InvariantTree(t, "fresh edit", fresh, lang, nextSource)
				a, err := benchfixtures.InspectGoTree(incremental.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				b, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				if a.SHA256 != b.SHA256 {
					t.Fatalf("step %d incremental=%s fresh=%s", step+1, a.SHA256, b.SHA256)
				}
				fresh.Release()
				if tree != incremental {
					tree.Release()
				}
				tree = incremental
				source = nextSource
			}
			t.Logf("bytes=%d warm_passes=3 edits=3 no_edit_allocs=%g", len(source), allocs)
		})
	}
}
