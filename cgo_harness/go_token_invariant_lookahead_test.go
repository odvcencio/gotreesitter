//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"bytes"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestGoTokenInvariantLookaheadNumericRepairLockedC(t *testing.T) {
	testGoTokenInvariantLookaheadLockedC(t, "0eX", []byte{'1', '2', '3'}, false)
}

func TestGoTokenInvariantLookaheadNumericReuseLockedC(t *testing.T) {
	testGoTokenInvariantLookaheadLockedC(t, "1", []byte{'2', '3', '1'}, true)
}

func testGoTokenInvariantLookaheadLockedC(t *testing.T, literal string, replacements []byte, requireReuse bool) {
	t.Helper()
	for _, compact := range []bool{false, true} {
		name := "forced_legacy"
		if compact {
			name = "compact_enabled"
		}
		t.Run(name, func(t *testing.T) {
			language := grammars.GoLanguage()
			parser := gts.NewParser(language)
			parser.SetAdmissionCandidateRoute(compact)
			source := []byte("package p\nfunc f(){_=" + literal + "}\n")
			old, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { old.Release() }()
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
			defer func() { cOld.Close() }()
			offset := bytes.Index(source, []byte("_=")) + 2 + len(literal) - 1
			// The repair must combine the preceding integer and edited identifier.
			// Clean numeric edits must retain authenticated reuse across repetitions.
			for _, replacement := range replacements {
				edited := append([]byte(nil), source...)
				edited[offset] = replacement
				edit := gts.InputEdit{
					StartByte: uint32(offset), OldEndByte: uint32(offset + 1), NewEndByte: uint32(offset + 1),
					StartPoint: pointAtOffset(source, offset), OldEndPoint: pointAtOffset(source, offset+1), NewEndPoint: pointAtOffset(edited, offset+1),
				}
				old.Edit(edit)
				next, profile, err := parser.ParseIncrementalProfiled(edited, old)
				if err != nil {
					t.Fatal(err)
				}
				old.Release()
				old = next
				if requireReuse && (profile.TokenInvariantDependencyChecks != 1 ||
					profile.NewNodesAllocated != 0 || profile.ReparseNanos != 0 || profile.ReusedSubtrees == 0) {
					t.Fatalf("numeric edit did not use authenticated reuse: %+v", profile)
				}
				cEdit := realCorpusCInputEdit(edit)
				cOld.Edit(&cEdit)
				cNext := cParser.Parse(edited, cOld)
				if cNext == nil {
					t.Fatal("C parser returned no incremental tree")
				}
				cOld.Close()
				cOld = cNext
				cFresh := cParser.Parse(edited, nil)
				if cFresh == nil {
					t.Fatal("C parser returned no fresh tree")
				}
				fresh, err := parser.Parse(edited)
				if err != nil {
					cFresh.Close()
					t.Fatal(err)
				}
				func() {
					defer cFresh.Close()
					defer fresh.Release()
					assertLockedCTreeExact(t, "fresh numeric edit", fresh, language, cFresh)
					t.Logf("replacement=%c reparse_ns=%d new_nodes=%d", replacement, profile.ReparseNanos, profile.NewNodesAllocated)
					assertLockedCTreeExact(t, "incremental versus C fresh", next, language, cFresh)
					assertLockedCTreeExact(t, "incremental versus C incremental", next, language, cNext)
				}()
				source = edited
			}
		})
	}
}
