//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestTokenInvariantJuliaLineComments(t *testing.T) {
	for _, source := range []string{"# This is 1\nx = 1\n", "x = 1 # This is 1\n"} {
		testJuliaInvariantComment(t, source, strings.Index(source, "This"), []byte{'U', 'T', 'U'}, true)
		testJuliaInvariantComment(t, source, strings.Index(source, "is 1")+3, []byte{'2', '1', '2'}, true)
	}
}

func TestTokenInvariantJuliaCommentBoundaries(t *testing.T) {
	testJuliaInvariantComment(t, "# This\nx = 1\n", 0, []byte{'x'}, false)
	testJuliaInvariantComment(t, "# This\nx = 1\n", 1, []byte{'='}, false)
	source := "function f()\n # This\n 1\nend\n"
	testJuliaInvariantComment(t, source, strings.Index(source, "This"), []byte{'U'}, false)
}

func testJuliaInvariantComment(t *testing.T, initial string, offset int, replacements []byte, positive bool) {
	t.Helper()
	for _, compact := range []bool{false, true} {
		name := "legacy/" + initial
		if compact {
			name = "compact_enabled/" + initial
		}
		t.Run(name, func(t *testing.T) {
			language := grammars.JuliaLanguage()
			parser := gts.NewParser(language)
			parser.SetAdmissionCandidateRoute(compact)
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
			if old.RootNode().HasError() {
				t.Fatal("initial fixture is malformed")
			}
			for _, replacement := range replacements {
				edited := append([]byte(nil), source...)
				edited[offset] = replacement
				edit := gts.InputEdit{StartByte: uint32(offset), OldEndByte: uint32(offset + 1), NewEndByte: uint32(offset + 1), StartPoint: pointForOffset(source, offset), OldEndPoint: pointForOffset(source, offset+1), NewEndPoint: pointForOffset(edited, offset+1)}
				old.Edit(edit)
				next, profile, err := parser.ParseIncrementalProfiled(edited, old)
				if err != nil || next == nil {
					t.Fatalf("incremental parse: %v", err)
				}
				if next != old {
					old.Release()
				}
				old = next
				fresh, err := gts.NewParser(language).Parse(edited)
				if err != nil || fresh == nil {
					t.Fatalf("fresh parse: %v", err)
				}
				func() {
					defer fresh.Release()
					requireForestFallbackNodeIdentity(t, "root", next.RootNode(), fresh.RootNode(), language)
				}()
				if positive && (profile.TokenInvariantDependencyChecks == 0 || profile.ReparseNanos != 0 || profile.NewNodesAllocated != 0 || profile.ReusedSubtrees == 0) {
					t.Fatalf("line comment lost dependency-proven shortcut: %+v", profile)
				}
				if !positive && profile.ReparseNanos == 0 && profile.NewNodesAllocated == 0 {
					t.Fatalf("boundary/nested edit took shortcut: %+v", profile)
				}
				source = edited
			}
		})
	}
}
