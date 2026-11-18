//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestHTTPNoFinalNewlineMatchesPinnedC(t *testing.T) {
	lang, cLang := ccoreLanguages(t, "http")
	p := g.NewParser(lang)
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}

	// Issue #1529: the trimmed request has an ERROR child in pinned C.
	sources := [][]byte{[]byte("GET /path"), []byte("GET /path\n"), []byte("GET /path")}
	var previous *g.Tree
	defer func() {
		if previous != nil {
			previous.Release()
		}
	}()
	for i, source := range sources {
		var next *g.Tree
		var err error
		_, freshBefore, incrBefore := bind.RouteCounters()
		if previous == nil {
			next, err = p.Parse(source)
		} else {
			oldSource := sources[i-1]
			at := min(len(oldSource), len(source))
			previous.Edit(g.InputEdit{
				StartByte: uint32(at), OldEndByte: uint32(len(oldSource)), NewEndByte: uint32(len(source)),
				StartPoint: gradPoint(oldSource, at), OldEndPoint: gradPoint(oldSource, len(oldSource)),
				NewEndPoint: gradPoint(source, len(source)),
			})
			next, err = p.ParseIncremental(source, previous)
		}
		if err != nil || next == nil {
			t.Fatalf("step %d: Go parse returned %v, %v", i, next, err)
		}
		if previous != nil {
			previous.Release()
		}
		previous = next
		want := cp.Parse(source, nil)
		if want == nil {
			t.Fatalf("step %d: pinned C returned no tree", i)
		}
		defer want.Close()
		assertLockedCTreeExactWithErrors(t, "HTTP EOF edit", next, lang, want)
		_, freshAfter, incrAfter := bind.RouteCounters()
		if freshAfter-freshBefore+incrAfter-incrBefore != 1 {
			t.Fatalf("step %d: core served fresh=%d incremental=%d", i, freshAfter-freshBefore, incrAfter-incrBefore)
		}
	}
}
