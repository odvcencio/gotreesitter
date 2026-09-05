//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
	"testing"
)

func TestJavaScriptCompactNestedReuseLookaheadLockedC(t *testing.T) {
	source, edited := []byte("a+b ;"), []byte("a+b *c;")
	edit := gts.InputEdit{StartByte: 4, OldEndByte: 5, NewEndByte: 7, StartPoint: gts.Point{Column: 4}, OldEndPoint: gts.Point{Column: 5}, NewEndPoint: gts.Point{Column: 7}}
	lang := grammars.JavascriptLanguage()
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(true)
	old, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	old.Edit(edit)
	next, err := p.ParseIncremental(edited, old)
	old.Release()
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
	runtime := next.ParseRuntime()
	t.Logf("compact=%t reused=%d/%d decline=%q tree=%s", runtime.CompactIncrementalReuseRoute, runtime.CompactIncrementalReusedSubtrees, runtime.CompactIncrementalReusedBytes, runtime.CompactIncrementalFallbackReason, next.RootNode().SExpr(lang))
	cl, err := ParityCLanguage("javascript")
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
	ce := realCorpusCInputEdit(edit)
	co.Edit(&ce)
	ci := cp.Parse(edited, co)
	co.Close()
	cf := cp.Parse(edited, nil)
	if ci == nil || cf == nil {
		t.Fatal("C result is nil")
	}
	defer ci.Close()
	defer cf.Close()
	for _, oracle := range []*sitter.Tree{cf, ci} {
		assertLockedCTreeExact(t, "nested reduction lookahead", next, lang, oracle)
	}
}
