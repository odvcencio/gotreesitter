//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestTokenInvariantTypeScriptNumericControls(t *testing.T) {
	for _, source := range []string{
		"const n = 1;\n", "const n = 1e1;\n",
		"let x: Array<Array<number>>;\nconst n = 1;\n",
		"const n = 8 >> 1;\n", "type N = 1;\n",
		"type N = Box<Box<1>>;\n", "foo<1>(2);\n", "const n = 1 >> 2;\n",
	} {
		testTypeScriptInvariantEdits(t, source, strings.LastIndex(source, "1"), []byte{'2', '1', '2'}, true)
	}
}

func TestTokenInvariantTypeScriptSyntaxAndTextChanges(t *testing.T) {
	for _, test := range []struct {
		source, needle string
		replacement    byte
	}{
		{"const n = 0b1;\n", "1", '2'},
		{"const n = 1e1;\n", "1", '+'},
		{"const n = '1';\n", "1", '2'},
		{"const n = abc;\n", "c;", 'd'},
	} {
		testTypeScriptInvariantEdits(t, test.source, strings.LastIndex(test.source, test.needle), []byte{test.replacement}, false)
	}
}

func testTypeScriptInvariantEdits(t *testing.T, initial string, offset int, replacements []byte, positive bool) {
	t.Helper()
	for _, compact := range []bool{false, true} {
		name := "legacy/" + initial
		if compact {
			name = "compact/" + initial
		}
		t.Run(name, func(t *testing.T) {
			language := grammars.TypescriptLanguage()
			parser := gts.NewParser(language)
			parser.SetAdmissionCandidateRoute(compact)
			parser.SetCompactCertificationTelemetry(compact)
			source := []byte(initial)
			before, _ := gts.AdmissionCandidateCounters()
			old, err := parser.Parse(source)
			if err != nil || old == nil {
				t.Fatalf("initial parse: %v", err)
			}
			defer func() { old.Release() }()
			if gts.CcoreTestFromCore(old) {
				t.Fatal("the core served the fixture; this test checks the compact and legacy routes")
			}
			if old.RootNode().HasError() {
				t.Fatal("initial fixture is malformed")
			}
			after, _ := gts.AdmissionCandidateCounters()
			if compact && initial == "const n = 1;\n" && after != before+1 {
				t.Fatalf("integer fixture did not use compact initial route: %s", gts.AdmissionCandidateLastFallbackReason())
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
				if positive {
					if profile.TokenInvariantDependencyChecks != 1 || profile.ReparseNanos != 0 || profile.NewNodesAllocated != 0 || profile.ReusedSubtrees == 0 || profile.ReuseUnsupported {
						t.Fatalf("numeric shortcut did not prove dependencies: %+v", profile)
					}
				} else if profile.ReparseNanos == 0 && profile.NewNodesAllocated == 0 {
					t.Fatalf("syntax/text edit incorrectly took token shortcut: %+v", profile)
				}
				fresh, err := gts.NewParser(language).Parse(edited)
				if err != nil || fresh == nil {
					t.Fatalf("fresh parse: %v", err)
				}
				func() {
					defer fresh.Release()
					requireForestFallbackNodeIdentity(t, "root", next.RootNode(), fresh.RootNode(), language)
				}()
				source = edited
			}
		})
	}
}

// TestTokenInvariantTypeScriptNumericControlsCore holds the default route,
// which the certified core serves for TypeScript, to the invariant the other
// routes meet: after a numeric edit the incremental tree equals a fresh parse
// and the parse reuses old subtrees.
func TestTokenInvariantTypeScriptNumericControlsCore(t *testing.T) {
	for _, initial := range []string{
		"const n = 1;\n", "const n = 1e1;\n",
		"let x: Array<Array<number>>;\nconst n = 1;\n",
		"const n = 8 >> 1;\n", "type N = 1;\n",
		"type N = Box<Box<1>>;\n", "foo<1>(2);\n", "const n = 1 >> 2;\n",
	} {
		t.Run(initial, func(t *testing.T) {
			language := grammars.TypescriptLanguage()
			parser := gts.NewParser(language)
			source := []byte(initial)
			offset := strings.LastIndex(initial, "1")
			old, err := parser.Parse(source)
			if err != nil || old == nil {
				t.Fatalf("initial parse: %v", err)
			}
			defer func() { old.Release() }()
			if !gts.CcoreTestFromCore(old) || old.RootNode().HasError() {
				t.Fatal("initial fixture was not served cleanly by the core")
			}
			for _, replacement := range []byte{'2', '1', '2'} {
				edited := append([]byte(nil), source...)
				edited[offset] = replacement
				old.Edit(gts.InputEdit{StartByte: uint32(offset), OldEndByte: uint32(offset + 1), NewEndByte: uint32(offset + 1), StartPoint: pointForOffset(source, offset), OldEndPoint: pointForOffset(source, offset+1), NewEndPoint: pointForOffset(edited, offset+1)})
				next, profile, err := parser.ParseIncrementalProfiled(edited, old)
				if err != nil || next == nil {
					t.Fatalf("incremental parse: %v", err)
				}
				if next != old {
					old.Release()
				}
				old = next
				if profile.ReuseUnsupported || profile.ReusedSubtrees == 0 {
					t.Fatalf("numeric edit did not reuse old subtrees: %+v", profile)
				}
				fresh, err := gts.NewParser(language).Parse(edited)
				if err != nil || fresh == nil {
					t.Fatalf("fresh parse: %v", err)
				}
				func() {
					defer fresh.Release()
					requireForestFallbackNodeIdentity(t, "root", next.RootNode(), fresh.RootNode(), language)
				}()
				source = edited
			}
		})
	}
}
