//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"bytes"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestGoMissingBraceIncrementalLockedC(t *testing.T) {
	source := []byte("package p\nfunc a() { _ = 1 }\nfunc b() { _ = 2 }\n")
	start := bytes.IndexByte(source, '}')
	edited := bytes.Replace(source, []byte("}"), nil, 1)
	edit := gts.InputEdit{
		StartByte: uint32(start), OldEndByte: uint32(start + 1), NewEndByte: uint32(start),
		StartPoint: pointAtOffset(source, start), OldEndPoint: pointAtOffset(source, start+1), NewEndPoint: pointAtOffset(edited, start),
	}
	lang := grammars.GoLanguage()
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(true)
	old, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	old.Edit(edit)
	next, err := p.ParseIncremental(edited, old)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
	runtime := next.ParseRuntime()
	t.Logf("incremental compact=%v recovery=%v fallback=%q", runtime.CompactIncrementalReuseRoute, runtime.CompactIncrementalFullRecoveryRoute, runtime.CompactIncrementalFallbackReason)
	cl, err := ParityCLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	co := cp.Parse(source, nil)
	if co == nil {
		t.Fatal("C old tree is nil")
	}
	defer co.Close()
	ce := realCorpusCInputEdit(edit)
	co.Edit(&ce)
	for _, tc := range []struct {
		name string
		old  *sitter.Tree
	}{{"fresh", nil}, {"incremental", co}} {
		t.Run(tc.name, func(t *testing.T) {
			oracle := cp.Parse(edited, tc.old)
			if oracle == nil {
				t.Fatal("C recovery tree is nil")
			}
			defer oracle.Close()
			assertG18LockedCExact(t, "missing brace", next, lang, oracle)
		})
	}
}
