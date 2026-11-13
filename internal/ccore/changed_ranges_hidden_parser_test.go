package ccore_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore"
)

func TestChangedRangesParserCanProduceHiddenRoot(t *testing.T) {
	g := goldenNamed(t, "json-object")
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	seed, stop := p.ParseWith([]byte(`{}`), nil)
	if stop != "" || seed == nil {
		t.Fatalf("seed %q", stop)
	}
	sym := seed.Reader().Symbol(seed.Root())
	if !seed.Reader().Visible(seed.Root()) {
		t.Fatal("positive control root not visible")
	}
	seed.Release()
	if int(sym) >= len(g.Tables.Meta) {
		t.Fatal("root metadata missing")
	}
	g.Tables.Meta[sym].Visible = false
	p = mustParser(t, ccore.Config{Tables: g.Tables})
	tr, stop := p.ParseWith([]byte(`{}`), nil)
	if stop != "" || tr == nil {
		t.Fatalf("hidden parse %q", stop)
	}
	defer tr.Release()
	if tr.Reader().Visible(tr.Root()) {
		t.Fatal("custom tables did not produce hidden root")
	}
	if _, ok := ccore.ChangedRanges(tr, tr, g.Tables, nil, nil); ok {
		t.Fatal("hidden root accepted")
	}
}
