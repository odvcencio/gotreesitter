//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestJuliaTokenInvariantLineCommentsLockedC(t *testing.T) {
	cLanguage, err := ParityCLanguage("julia")
	if err != nil {
		t.Fatal(err)
	}
	for _, initial := range []string{"# This is 1\nx = 1\n", "x = 1 # This is 1\n"} {
		for _, digit := range []bool{false, true} {
			for _, compact := range []bool{false, true} {
				name := "legacy/letter/" + initial
				if digit {
					name = "legacy/digit/" + initial
				}
				if compact {
					name = "compact_enabled/" + name
				}
				t.Run(name, func(t *testing.T) {
					language := grammars.JuliaLanguage()
					parser := gts.NewParser(language)
					parser.SetAdmissionCandidateRoute(compact)
					cParser := sitter.NewParser()
					defer cParser.Close()
					if err := cParser.SetLanguage(cLanguage); err != nil {
						t.Fatal(err)
					}
					source := []byte(initial)
					before, fallbackBefore := gts.AdmissionCandidateCounters()
					old, err := parser.Parse(source)
					if err != nil || old == nil {
						t.Fatalf("initial parse: %v", err)
					}
					defer func() { old.Release() }()
					after, fallbackAfter := gts.AdmissionCandidateCounters()
					t.Logf("initial compact_enabled=%t routed=%d fallback=%d runtime=%s", compact, after-before, fallbackAfter-fallbackBefore, old.ParseRuntime().Summary())
					if (after == before+1) != compact || fallbackAfter != fallbackBefore {
						t.Fatal("initial fixture did not use the requested parser route")
					}
					cOld := cParser.Parse(source, nil)
					if cOld == nil {
						t.Fatal("C initial tree is nil")
					}
					defer func() { cOld.Close() }()
					assertLockedCTreeExact(t, "initial line comment", old, language, cOld)
					offset := strings.Index(initial, "This")
					replacements := []byte{'U', 'T', 'U'}
					if digit {
						offset = strings.Index(initial, "is 1") + 3
						replacements = []byte{'2', '1', '2'}
					}
					for _, replacement := range replacements {
						edited := append([]byte(nil), source...)
						edited[offset] = replacement
						edit := gts.InputEdit{StartByte: uint32(offset), OldEndByte: uint32(offset + 1), NewEndByte: uint32(offset + 1), StartPoint: pointAtOffset(source, offset), OldEndPoint: pointAtOffset(source, offset+1), NewEndPoint: pointAtOffset(edited, offset+1)}
						old.Edit(edit)
						next, profile, err := parser.ParseIncrementalProfiled(edited, old)
						if err != nil || next == nil {
							t.Fatalf("incremental parse: %v", err)
						}
						if next != old {
							old.Release()
						}
						old = next
						cEdit := realCorpusCInputEdit(edit)
						cOld.Edit(&cEdit)
						cNext := cParser.Parse(edited, cOld)
						if cNext == nil {
							t.Fatal("C incremental tree is nil")
						}
						cOld.Close()
						cOld = cNext
						cFresh := cParser.Parse(edited, nil)
						if cFresh == nil {
							t.Fatal("C fresh tree is nil")
						}
						func() {
							defer cFresh.Close()
							fresh, err := gts.NewParser(language).Parse(edited)
							if err != nil || fresh == nil {
								t.Fatalf("fresh parse: %v", err)
							}
							defer fresh.Release()
							assertLockedCTreeExact(t, "fresh line comment", fresh, language, cFresh)
							assertLockedCTreeExact(t, "incremental versus C fresh", next, language, cFresh)
							assertLockedCTreeExact(t, "incremental versus C incremental", next, language, cNext)
						}()
						if profile.TokenInvariantDependencyChecks == 0 || profile.ReparseNanos != 0 || profile.NewNodesAllocated != 0 || profile.ReusedSubtrees == 0 {
							t.Fatalf("line comment lost dependency-proven shortcut: %+v", profile)
						}
						source = edited
					}
				})
			}
		}
	}
}
