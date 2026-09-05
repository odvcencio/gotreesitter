//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"bytes"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestGoCompactIncrementalExecutionParity(t *testing.T) {
	for _, tc := range []struct {
		name, separator, before, after string
		reuse                          bool
	}{
		{"growth", "", "1", "1+3", true},
		{"comment", "// between declarations\n", "1", "1+3", true},
		{"same_width_kind_change", "", "1", "x", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := []byte("package p\nfunc a() { _ = 1 }\n" + tc.separator + "func b() { _ = 2 }\n")
			offset := bytes.Index(source, []byte(tc.before))
			edited := bytes.Replace(source, []byte(tc.before), []byte(tc.after), 1)
			edit := gts.InputEdit{StartByte: uint32(offset), OldEndByte: uint32(offset + len(tc.before)), NewEndByte: uint32(offset + len(tc.after)), StartPoint: pointAtOffset(source, offset), OldEndPoint: pointAtOffset(source, offset+len(tc.before)), NewEndPoint: pointAtOffset(edited, offset+len(tc.after))}
			language := grammars.GoLanguage()
			parser := gts.NewParser(language)
			parser.SetAdmissionCandidateRoute(true)
			old, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			oldB := findGoNodeByTypeAndStart(old.RootNode(), language, "function_declaration", uint32(bytes.Index(source, []byte("func b"))))
			if oldB == nil {
				old.Release()
				t.Fatal("initial tree has no function b")
			}
			old.Edit(edit)
			routed, fallback := gts.AdmissionCandidateCounters()
			next, profile, err := parser.ParseIncrementalProfiled(edited, old)
			old.Release()
			if err != nil {
				t.Fatal(err)
			}
			defer next.Release()
			if r, f := gts.AdmissionCandidateCounters(); r != routed || f != fallback {
				t.Fatal("incremental parse changed full admission counters")
			}
			if tc.reuse {
				newB := findGoNodeByTypeAndStart(next.RootNode(), language, "function_declaration", uint32(bytes.Index(edited, []byte("func b"))))
				if newB != oldB {
					t.Fatal("function b lost its node identity")
				}
				if profile.ReuseUnsupported || !profile.OldTreeReuseRoute || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
					t.Fatalf("missing reuse: %+v", profile)
				}
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
				t.Fatal("C parser returned no source tree")
			}
			cEdit := realCorpusCInputEdit(edit)
			cOld.Edit(&cEdit)
			cIncremental := cParser.Parse(edited, cOld)
			cOld.Close()
			cFresh := cParser.Parse(edited, nil)
			if cIncremental == nil || cFresh == nil {
				t.Fatal("C parser returned no result")
			}
			defer cIncremental.Close()
			defer cFresh.Close()
			for _, oracle := range []*sitter.Tree{cFresh, cIncremental} {
				if next.RootNode().HasError() != oracle.RootNode().HasError() {
					t.Fatal("root error differs from C")
				}
				if diff := FirstDivergenceDumpV1(next.RootNode(), language, oracle.RootNode()); diff != nil {
					t.Fatalf("C tree divergence: %+v", diff)
				}
				if diff := firstLockedCTreeFlagDivergence(next.RootNode(), language, oracle.RootNode(), "/source_file"); diff != nil {
					t.Fatal(diff)
				}
				if tc.reuse {
					assertLockedCTreeExact(t, tc.name, next, language, oracle)
				}
			}
			if tc.reuse {
				runtime := next.ParseRuntime()
				if !runtime.CompactIncrementalReuseRoute || runtime.CompactIncrementalReusedSubtrees == 0 || runtime.CompactIncrementalReusedBytes == 0 {
					t.Fatalf("missing compact execution: decline=%q", runtime.CompactIncrementalFallbackReason)
				}
			}
		})
	}
}

func TestGoCompactIncrementalExecutionRepeatedSameWidthParity(t *testing.T) {
	source := []byte("package p\nfunc a() { _ = 1 }\nfunc b() { _ = 2 }\n")
	lang := grammars.GoLanguage()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	old, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { old.Release() }()
	bStart := uint32(bytes.Index(source, []byte("func b")))
	oldB := findGoNodeByTypeAndStart(old.RootNode(), lang, "function_declaration", bStart)
	if oldB == nil {
		t.Fatal("initial tree has no function b")
	}
	cLang, err := ParityCLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	cOld := cp.Parse(source, nil)
	if cOld == nil {
		t.Fatal("C parser returned no source tree")
	}
	defer func() { cOld.Close() }()
	offset := bytes.IndexByte(source, '1')
	for _, replacement := range []byte{'x', '3', 'y'} {
		edited := append([]byte(nil), source...)
		edited[offset] = replacement
		edit := gts.InputEdit{StartByte: uint32(offset), OldEndByte: uint32(offset + 1), NewEndByte: uint32(offset + 1), StartPoint: pointAtOffset(source, offset), OldEndPoint: pointAtOffset(source, offset+1), NewEndPoint: pointAtOffset(edited, offset+1)}
		old.Edit(edit)
		next, err := parser.ParseIncremental(edited, old)
		if err != nil {
			t.Fatal(err)
		}
		if next == old {
			t.Fatal("kind change returned the old tree")
		}
		old.Release()
		old = next
		ce := realCorpusCInputEdit(edit)
		cOld.Edit(&ce)
		cNext := cp.Parse(edited, cOld)
		if cNext == nil {
			t.Fatal("C incremental parser returned no tree")
		}
		cOld.Close()
		cOld = cNext
		cFresh := cp.Parse(edited, nil)
		if cFresh == nil {
			t.Fatal("C fresh parser returned no tree")
		}
		assertLockedCTreeExact(t, string(replacement)+" versus fresh C", next, lang, cFresh)
		cFresh.Close()
		assertLockedCTreeExact(t, string(replacement)+" versus incremental C", next, lang, cNext)
		if got := findGoNodeByTypeAndStart(next.RootNode(), lang, "function_declaration", bStart); got != oldB {
			t.Fatal("repeated kind change lost function b identity")
		}
		if !next.ParseRuntime().CompactIncrementalReuseRoute {
			t.Errorf("edit to %q skipped compact execution: %q", replacement, next.ParseRuntime().CompactIncrementalFallbackReason)
		}
		source = edited
	}
}
