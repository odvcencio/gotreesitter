//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestCSSForestScannerWidthLockedC(t *testing.T) {
	testForestCSSScannerWidthLockedC(t, "css", grammars.CssLanguage())
}

func TestSCSSForestScannerWidthLockedC(t *testing.T) {
	testForestCSSScannerWidthLockedC(t, "scss", grammars.ScssLanguage())
}

func testForestCSSScannerWidthLockedC(t *testing.T, name string, base *gts.Language) {
	t.Helper()
	languageCopy := *base
	language := &languageCopy
	language.WantsForest = true
	cLanguage, err := ParityCLanguage(name)
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, needle string
		replacements []string
	}{
		{"numeric", "1px", []string{"2px", "22px", "333px", "1px"}},
		{"selector_colon", "a:hover", []string{"a:focus", "a::before", "a:hover"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			parser := gts.NewParser(language)
			source := []byte("a:hover { margin: 1px; color: red; }\n.b { padding: 4px; }\n")
			old, ok := parser.ParseForestExperimental(source)
			if !ok || old == nil {
				t.Fatal("initial forest parse declined")
			}
			defer func() { old.Release() }()
			if !old.ParseRuntime().ForestFastPath || old.RootNode().HasError() {
				t.Fatal("initial result is not a clean forest tree")
			}
			cOld := cParser.Parse(source, nil)
			if cOld == nil {
				t.Fatal("C initial parse returned no tree")
			}
			defer func() { cOld.Close() }()
			assertLockedCTreeExact(t, "initial forest", old, language, cOld)
			needle := test.needle
			for index, replacement := range test.replacements {
				start := strings.Index(string(source), needle)
				if start < 0 {
					t.Fatalf("missing edit site %q", needle)
				}
				edited := []byte(string(source[:start]) + replacement + string(source[start+len(needle):]))
				oldEnd, newEnd := start+len(needle), start+len(replacement)
				for start < oldEnd && start < newEnd && source[start] == edited[start] {
					start++
				}
				for oldEnd > start && newEnd > start && source[oldEnd-1] == edited[newEnd-1] {
					oldEnd--
					newEnd--
				}
				edit := gts.InputEdit{
					StartByte: uint32(start), OldEndByte: uint32(oldEnd), NewEndByte: uint32(newEnd),
					StartPoint: pointAtOffset(source, start), OldEndPoint: pointAtOffset(source, oldEnd), NewEndPoint: pointAtOffset(edited, newEnd),
				}
				old.Edit(edit)
				next, profile, err := parser.ParseIncrementalProfiled(edited, old)
				if err != nil || next == nil {
					t.Fatalf("incremental parse: %v", err)
				}
				if next != old {
					old.Release()
				}
				old = next
				if test.name == "numeric" && index == 0 && (profile.ReuseUnsupported || profile.ReusedSubtrees == 0 ||
					profile.TokenInvariantDependencyChecks != 1 || profile.ReparseNanos != 0 || profile.NewNodesAllocated != 0) {
					t.Fatalf("same-width numeric control lost shortcut: %+v", profile)
				}
				cEdit := realCorpusCInputEdit(edit)
				cOld.Edit(&cEdit)
				cNext := cParser.Parse(edited, cOld)
				if cNext == nil {
					t.Fatal("C incremental parse returned no tree")
				}
				cOld.Close()
				cOld = cNext
				cFresh := cParser.Parse(edited, nil)
				if cFresh == nil {
					t.Fatal("C fresh parse returned no tree")
				}
				func() {
					defer cFresh.Close()
					fresh, err := gts.NewParser(language).Parse(edited)
					if err != nil || fresh == nil {
						t.Fatalf("fresh parse: %v", err)
					}
					defer fresh.Release()
					assertLockedCTreeExact(t, "fresh Go versus C fresh", fresh, language, cFresh)
					assertLockedCTreeExact(t, "incremental Go versus C fresh", next, language, cFresh)
					assertLockedCTreeExact(t, "incremental Go versus C incremental", next, language, cNext)
				}()
				source, needle = edited, replacement
			}
		})
	}
}
