package ccore_test

import (
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore"
)

// A reparse of an unedited tree: the allocations of ParseWith plus Release of
// its result, per call. The invariant "a reparse without edits allocates
// nothing" is a requirement on the public route; this package alone does not
// meet it. The test pins what the package allocates today so that the number
// cannot grow unnoticed, and the comment on each bound says what the
// allocations are.
func TestNoEditReparseAllocations(t *testing.T) {
	g := goldenNamed(t, "json-object")
	big := []byte("[" + strings.Repeat("1,", 1<<19) + "1]") // about 1 MiB
	for _, c := range []struct {
		name  string
		src   []byte
		bound float64
	}{
		{"small", g.Src, noEditBoundSmall},
		{"1MiB", big, noEditBound1MiB},
	} {
		p := mustParser(t, ccore.Config{Tables: g.Tables})
		tr, stop := p.Parse(c.src)
		if tr == nil || stop != "" {
			t.Fatalf("%s: parse stopped: %q", c.name, stop)
		}
		got := testing.AllocsPerRun(20, func() {
			nt, _ := p.ParseWith(c.src, tr)
			if nt == nil {
				t.Fatal("reparse returned no tree")
			}
			nt.Release()
		})
		t.Logf("%s: %v allocations per reparse of an unedited tree (bytes %d)", c.name, got, len(c.src))
		if got > c.bound {
			t.Errorf("%s: %v allocations per no-edit reparse, bound %v", c.name, got, c.bound)
		}
		tr.Release()
	}
}

// The bounds are the measured number (3 for both inputs, so the cost does not
// grow with the input). The three allocations of one ParseWith plus Release
// are: the new Tree value (api.go, `&Tree{...}`), its cell with the finalizer
// (tree.go, newCell), and the mark list that findMulti builds from empty on each
// parse (subtree.go, `seen = append(seen, h)`). The queued release of the old
// root grows the pending list by amortized doubling and shows as 0 in the
// average. Removing the last one means keeping that list on the Parser; removing
// the first two means returning the same tree when nothing changed, which
// changes who owns the root and is not done here.
const (
	noEditBoundSmall = 3
	noEditBound1MiB  = 3
)
