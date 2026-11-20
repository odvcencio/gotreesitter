//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	"github.com/odvcencio/gotreesitter/cgo_harness/internal/corecords"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestCcoreEBNFRecoveryWork(t *testing.T) {
	lang, cLang := ccoreLanguages(t, "ebnf")
	src, role := gradSample(t, "ebnf")
	if role != "r4" || gradSHA(src) != "fd73d5ddfd985406ac828572de679a4f7c88173009e2990d2e55546763f4df5f" {
		t.Fatalf("unexpected EBNF counter fixture: role=%s sha=%s", role, gradSHA(src))
	}
	step := gradChain(src)[1]
	if step.edit == nil {
		t.Fatal("missing locked edit")
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	ct := cp.Parse(src, nil)
	if ct == nil {
		t.Fatal("pinned C fresh parse failed")
	}
	defer ct.Close()

	core := ccoreNew(lang)
	core.SetShortcut(false)
	check := func(label string, want *sitter.Tree, tokenCount, recordCap int) {
		t.Helper()
		hash, count, errCode := corecords.Hash(want, "")
		gotHash, gotCount := core.RecordsHash("")
		_, tokens, versions, _, _ := core.Stats()
		leaves, parents, copies := core.RecordSources()
		work := leaves + parents + copies
		if errCode != 0 || gotHash != hash || gotCount != count || count == 0 {
			t.Fatalf("%s records changed: Go %d/%x, C %d/%x (status %d)", label, gotCount, gotHash, count, hash, errCode)
		}
		if tokens != tokenCount || versions != 3 || work > recordCap {
			t.Fatalf("%s work: %d tokens, %d live versions, %d records (cap %d)", label, tokens, versions, work, recordCap)
		}
		t.Logf("%s: %d tokens, %d live versions, %d allocated records, C digest %x", label, tokens, versions, work, hash)
	}
	if stop := core.Parse(src, nil); stop != "" {
		t.Fatal(stop)
	}
	old := core.Tree()
	defer core.Release(old)
	check("fresh", ct, 71, 142)

	ct.Edit(gradSitterEdit(*step.edit))
	changed := cp.Parse(step.src, ct)
	if changed == nil {
		t.Fatal("pinned C incremental parse failed")
	}
	defer changed.Close()
	e := step.edit
	edited := core.Edit(old, bindtest.Edit{
		StartByte: e.StartByte, OldEndByte: e.OldEndByte, NewEndByte: e.NewEndByte,
		StartRow: e.StartPoint.Row, StartCol: e.StartPoint.Column,
		OldEndRow: e.OldEndPoint.Row, OldEndCol: e.OldEndPoint.Column,
		NewEndRow: e.NewEndPoint.Row, NewEndCol: e.NewEndPoint.Column,
	})
	defer core.Release(edited)
	next, stop := core.ParseIncremental(step.src, edited, nil)
	if stop != "" || !next.Valid() {
		t.Fatalf("core incremental parse stopped: %q", stop)
	}
	defer core.Release(next)
	check("edited", changed, 61, 138)
	trees, bytes, largest := core.ReuseStats()
	if trees != 29 || bytes != 149 || largest != 26 {
		t.Fatalf("edited reuse changed: %d trees, %d bytes, %d largest", trees, bytes, largest)
	}
}
