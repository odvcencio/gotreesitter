//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"crypto/sha256"
	"os"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Run one backend per process under /usr/bin/time -v. The fixed large fixture
// includes full parsing, Tree.Edit, incremental parsing, validation, and release.
func TestAccountingLargeCompleteOperationRSS(t *testing.T) {
	const line = "func f(a int) int { x := a + 1; return x }\n"
	source := []byte("package p\n" + strings.Repeat(line, (1<<20)/len(line)) + "func tail() {}\n")
	edited := append([]byte(nil), source...)
	position := len(edited) / 2
	for edited[position] != '1' {
		position++
	}
	edited[position] = '2'
	edit := gts.InputEdit{StartByte: uint32(position), OldEndByte: uint32(position + 1), NewEndByte: uint32(position + 1),
		StartPoint: pointAtOffset(source, position), OldEndPoint: pointAtOffset(source, position+1), NewEndPoint: pointAtOffset(edited, position+1)}
	if os.Getenv("GTS_ACCOUNTING_RSS_BACKEND") == "C" {
		p := sitter.NewParser()
		defer p.Close()
		if err := p.SetLanguage(loadCanonicalGoCLanguage(t)); err != nil {
			t.Fatal(err)
		}
		old := p.Parse(source, nil)
		if old == nil || old.RootNode().EndByte() != uint(len(source)) || old.RootNode().HasError() {
			t.Fatal("C initial parse incomplete")
		}
		defer old.Close()
		ce := realCorpusCInputEdit(edit)
		old.Edit(&ce)
		next := p.Parse(edited, old)
		defer next.Close()
		if next == nil || next.RootNode().EndByte() != uint(len(edited)) || next.RootNode().HasError() {
			t.Fatal("C incomplete")
		}
		t.Logf("backend=C bytes=%d sha256=%x digest=%s", len(source), sha256.Sum256(source), canonicalCTreeDigest(t, next, "large C edit"))
		return
	}
	p := gts.NewParser(grammars.GoLanguage())
	p.SetAdmissionCandidateRoute(true)
	old, err := p.Parse(source)
	requireCanonicalGoIncrementalTree(t, old, source, "large initial", err)
	defer old.Release()
	old.Edit(edit)
	next, profile, err := p.ParseIncrementalProfiled(edited, old)
	requireCanonicalGoIncrementalTree(t, next, edited, "large edit", err)
	defer next.Release()
	t.Logf("backend=Go bytes=%d sha256=%x tokens=%d nodes=%d digest=%s runtime=%s", len(source), sha256.Sum256(source), profile.TokensConsumed, profile.NewNodesAllocated, canonicalGoTreeDigest(t, next, grammars.GoLanguage(), "large Go edit"), next.ParseRuntime().Summary())
}
