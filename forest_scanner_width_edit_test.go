package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	grm "github.com/odvcencio/gotreesitter/grammars"
)

func TestForestCSSScannerWidthEdits(t *testing.T) {
	testForestScannerWidthEdits(t, grm.CssLanguage())
}

func TestForestSCSSScannerWidthEdits(t *testing.T) {
	testForestScannerWidthEdits(t, grm.ScssLanguage())
}

func testForestScannerWidthEdits(t *testing.T, language *gts.Language) {
	t.Helper()
	gts.SetGLRForestEnabled(true)
	t.Cleanup(func() { gts.SetGLRForestEnabled(true) })
	language = explicitForestLanguage(t, language)
	for _, test := range []struct {
		name              string
		before            string
		needle            string
		replacements      []string
		requireFirstReuse bool
	}{
		{"numeric", ".a { margin: 1px; }\n.b { padding: 4px; }\n", "1px", []string{"2px", "22px", "333px", "1px"}, true},
		{"colon_semicolon_to_brace", "a:hover { color: red; }\n", "red;", []string{"red { };", "red;"}, false},
		{"colon_brace_to_semicolon", "a:hover { color: red; }\n", "hover {", []string{"hover; {", "hover {"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			parser := gts.NewParser(language)
			source := []byte(test.before)
			old, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			if old == nil {
				t.Fatal("initial tree is nil")
			}
			defer func() {
				if old != nil {
					old.Release()
				}
			}()
			requireAcceptedForestRuntime(t, "initial tree", old)
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
					StartPoint: pointForOffset(source, start), OldEndPoint: pointForOffset(source, oldEnd), NewEndPoint: pointForOffset(edited, newEnd),
				}
				old.Edit(edit)
				next, profile, err := parser.ParseIncrementalProfiled(edited, old)
				if err != nil {
					t.Fatal(err)
				}
				if next == nil {
					t.Fatal("incremental tree is nil")
				}
				if next != old {
					old.Release()
				}
				old = next
				if index == 0 && test.requireFirstReuse && (profile.ReuseUnsupported || profile.ReusedSubtrees == 0) {
					t.Fatalf("digit control lost reuse: %+v", profile)
				}
				fresh, err := gts.NewParser(language).Parse(edited)
				if err != nil {
					t.Fatal(err)
				}
				if fresh == nil {
					t.Fatal("fresh tree is nil")
				}
				func() {
					defer fresh.Release()
					requireForestFallbackNodeIdentity(t, "root", next.RootNode(), fresh.RootNode(), language)
				}()
				source, needle = edited, replacement
			}
		})
	}
}
