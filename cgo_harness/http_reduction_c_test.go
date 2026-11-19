//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"testing"

	"github.com/odvcencio/gotreesitter/cgo_harness/internal/corecords"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestHTTPFullLinkReductionsMatchPinnedC(t *testing.T) {
	const lines = 128
	src := bytes.Repeat([]byte("# note\n"), lines)
	lang, cLang := ccoreLanguages(t, "http")
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	ct := cp.Parse(src, nil)
	if ct == nil {
		t.Fatal("pinned C returned no tree")
	}
	defer ct.Close()
	cHash, cRecords, status := corecords.Hash(ct, "")
	core := ccoreNew(lang)
	if stop := core.Parse(src, nil); stop != "" {
		t.Fatal(stop)
	}
	defer core.ReleaseLast()
	goHash, goRecords := core.RecordsHash("")
	if status != 0 || cRecords == 0 || goRecords != cRecords || goHash != cHash {
		t.Fatalf("C records %x/%d status %d, core %x/%d", cHash, cRecords, status, goHash, goRecords)
	}
	// Every adjacent pair of comment lines can close a section or continue it.
	// With this grammar, constructing parents for saturated, distinguishable
	// links alone pushes the fresh allocation count above the square of lines.
	_, nodes, _ := core.RecordSources()
	if nodes >= lines*lines {
		t.Fatalf("allocated %d parents for %d comment lines, bound %d", nodes, lines, lines*lines)
	}
}
