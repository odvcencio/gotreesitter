//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"bytes"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestGoCompactIncrementalExecutionLargeDirtyRegionParity(t *testing.T) {
	source := []byte("package p\n" + strings.Repeat("func f(a int) int { x := a + 1; return x }\n", 3500) + "func tail() {}\n")
	edited := bytes.ReplaceAll(source, []byte("x :="), []byte("y :="))
	start := bytes.Index(source, []byte("x :="))
	end := bytes.LastIndex(source, []byte("x :=")) + 1
	edit := gts.InputEdit{
		StartByte: uint32(start), OldEndByte: uint32(end), NewEndByte: uint32(end),
		StartPoint: pointAtOffset(source, start), OldEndPoint: pointAtOffset(source, end),
		NewEndPoint: pointAtOffset(edited, end),
	}
	lang := grammars.GoLanguage()
	initialParser := gts.NewParser(lang)
	initialParser.SetAdmissionCandidateRoute(true)
	old, err := initialParser.Parse(source)
	requireCanonicalGoIncrementalTree(t, old, source, "initial", err)
	defer old.Release()
	old.Edit(edit)

	// A new parser makes the incremental arena start without retained capacity.
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	next, profile, err := parser.ParseIncrementalProfiled(edited, old)
	requireCanonicalGoIncrementalTree(t, next, edited, "wide edit", err)
	if next != old {
		defer next.Release()
	}
	if profile.ReusedSubtrees == 0 || !next.ParseRuntime().CompactIncrementalReuseRoute {
		t.Fatalf("wide edit did not reuse on the compact route: %s", next.ParseRuntime().Summary())
	}

	cLanguage, err := ParityCLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	cOld := cParser.Parse(source, nil)
	if cOld == nil {
		t.Fatal("C parser returned no initial tree")
	}
	defer cOld.Close()
	cEdit := realCorpusCInputEdit(edit)
	cOld.Edit(&cEdit)
	cIncremental := cParser.Parse(edited, cOld)
	cFresh := cParser.Parse(edited, nil)
	if cIncremental == nil || cFresh == nil {
		t.Fatal("C parser returned no result")
	}
	defer cIncremental.Close()
	defer cFresh.Close()
	assertLockedCTreeExact(t, "wide edit versus fresh C", next, lang, cFresh)
	assertLockedCTreeExact(t, "wide edit versus incremental C", next, lang, cIncremental)
}
