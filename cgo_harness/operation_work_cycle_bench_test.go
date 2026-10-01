//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"bytes"
	"testing"

	"github.com/odvcencio/gotreesitter"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// The complete operation includes Tree.Edit, parsing, result checks, and
// releasing the old handle. Each process runs an explicit Go-C-C-Go cycle.
func BenchmarkAccountingCompleteGoCCGo(b *testing.B) {
	for _, tc := range loadCanonicalGoIncrementalCases(b) {
		if tc.spec.Name != "same_line_length_change" {
			continue
		}
		goLang := canonicalIncrementalGoLanguage(b, "go")
		cLang := canonicalIncrementalCLanguage(b, "go")
		admitCanonicalGoIncrementalCase(b, tc, goLang, cLang)
		b.Run("GoBefore", func(b *testing.B) { benchmarkCanonicalGoIncremental(b, tc, goLang) })
		b.Run("CBefore", func(b *testing.B) { benchmarkCanonicalCIncremental(b, tc, cLang) })
		b.Run("CAfter", func(b *testing.B) { benchmarkCanonicalCIncremental(b, tc, cLang) })
		b.Run("GoAfter", func(b *testing.B) { benchmarkCanonicalGoIncremental(b, tc, goLang) })
		return
	}
	b.Fatal("missing fixed workload")
}

// Root repair constructs an additional parent outside the dispatch loop.
// Complete operation accounting must retain it without changing the C tree.
func TestAccountingLeadingCommentResultParity(t *testing.T) {
	source := []byte("# leading extra\n\ndef first():\n    return 1\n\ndef second():\n    return 2\n")
	position := bytes.Index(source, []byte("return 1")) + len("return 1")
	edited := append([]byte(nil), source[:position]...)
	edited = append(edited, '9')
	edited = append(edited, source[position:]...)
	edit := gotreesitter.InputEdit{StartByte: uint32(position), OldEndByte: uint32(position), NewEndByte: uint32(position + 1),
		StartPoint: pointAtOffset(source, position), OldEndPoint: pointAtOffset(source, position), NewEndPoint: pointAtOffset(edited, position+1)}
	lang := canonicalIncrementalGoLanguage(t, "python")
	p := gotreesitter.NewParser(lang)
	p.SetAdmissionCandidateRoute(false)
	old, err := p.Parse(source)
	requireCanonicalGoIncrementalTree(t, old, source, "leading comment initial", err)
	defer old.Release()
	old.Edit(edit)
	next, profile, err := p.ParseIncrementalProfiled(edited, old)
	requireCanonicalGoIncrementalTree(t, next, edited, "leading comment edit", err)
	defer next.Release()
	rt := next.ParseRuntime()
	if profile.NewNodesAllocated != rt.OperationWork.Total.Nodes || rt.OperationWork.Total.Nodes <= uint64(rt.NodesAllocated) {
		t.Fatalf("result allocation omitted: profile=%d loop=%d operation=%+v", profile.NewNodesAllocated, rt.NodesAllocated, rt.OperationWork)
	}
	fresh, err := p.Parse(edited)
	requireCanonicalGoIncrementalTree(t, fresh, edited, "leading comment fresh", err)
	defer fresh.Release()
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(canonicalIncrementalCLanguage(t, "python")); err != nil {
		t.Fatal(err)
	}
	ct := cp.Parse(edited, nil)
	if ct == nil {
		t.Fatal("C parse returned no tree")
	}
	defer ct.Close()
	got := canonicalGoTreeDigest(t, next, lang, "leading comment incremental")
	freshDigest := canonicalGoTreeDigest(t, fresh, lang, "leading comment fresh")
	cDigest := canonicalCTreeDigest(t, ct, "leading comment C")
	if got != freshDigest || got != cDigest {
		t.Fatalf("incremental=%s fresh=%s C=%s", got, freshDigest, cDigest)
	}
	t.Logf("loop nodes=%d operation nodes=%d tokens=%d C digest=%s", rt.NodesAllocated, rt.OperationWork.Total.Nodes, rt.OperationWork.Total.Tokens, got)
}
