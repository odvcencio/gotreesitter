//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"testing"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
)

func TestGraduatedCHandlesIssue454Edits(t *testing.T) {
	lang, reference := ccoreLanguages(t, "c")
	source := benchfixtures.Issue454CSource()
	offset := bytes.Index(source, []byte("x0"))
	if offset < 0 {
		t.Fatal("missing #454 edit site")
	}

	for _, edit := range []struct {
		name   string
		remove int
		insert string
		err    bool
	}{
		{name: "replace", remove: 1, insert: "y"},
		{name: "insert", insert: "x"},
		{name: "delete", remove: 1, err: true},
	} {
		t.Run(edit.name, func(t *testing.T) {
			p := g.NewParser(lang)
			_, f0, _ := bind.RouteCounters()
			old, err := p.Parse(source)
			if err != nil || old == nil {
				t.Fatalf("original parse: %v", err)
			}
			defer old.Release()
			_, f1, i1 := bind.RouteCounters()
			if f1-f0 != 1 || old.RootNode().HasError() || old.RootNode().EndByte() != uint32(len(source)) {
				t.Fatal("certified route did not serve the exact clean original")
			}

			text := make([]byte, 0, len(source)-edit.remove+len(edit.insert))
			text = append(text, source[:offset]...)
			text = append(text, edit.insert...)
			text = append(text, source[offset+edit.remove:]...)
			old.Edit(canonicalGoInputEdit(source, text, offset, offset+edit.remove, offset+len(edit.insert)))
			next, profile, err := p.ParseIncrementalProfiled(text, old)
			if err != nil || next == nil {
				t.Fatalf("certified edit: %v", err)
			}
			if next != old {
				defer next.Release()
			}
			_, f2, i2 := bind.RouteCounters()
			if f2-f1+i2-i1 != 1 {
				t.Fatal("certified route did not serve the edit")
			}
			if edit.err {
				if f2-f1 != 1 || i2-i1 != 0 || profile.ReuseUnsupportedReason != "ccore_fresh" {
					t.Fatalf("malformed edit did not use certified fresh recovery: %+v", profile)
				}
			} else if i2-i1 != 1 || profile.ReusedSubtrees == 0 {
				t.Fatalf("clean edit lost incremental reuse: %+v", profile)
			}

			fresh, err := p.Parse(text)
			if err != nil || fresh == nil {
				t.Fatalf("certified edited fresh parse: %v", err)
			}
			defer fresh.Release()
			oracle := compactT3ParseC(t, reference, text)
			defer oracle.Close()
			if oracle.RootNode().HasError() != edit.err {
				t.Fatal("pinned C no longer has the expected recovery shape")
			}
			assertLockedCTreeExactWithErrors(t, "certified edit", next, lang, oracle)
			assertLockedCTreeExactWithErrors(t, "certified fresh", fresh, lang, oracle)
			t.Logf("certified fresh=%d incremental=%d trees reused=%d reused bytes=%d", f2-f1, i2-i1, profile.ReusedSubtrees, profile.ReusedBytes)
		})
	}
}
