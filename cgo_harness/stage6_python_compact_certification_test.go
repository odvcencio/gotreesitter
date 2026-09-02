//go:build cgo && treesitter_c_parity && gts_parsercorephase0

package cgoharness

import (
	"bytes"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestStage6PythonCompactCertification proves Python clean and recovery
// shapes through the compact route against the locked C oracle.
func TestStage6PythonCompactCertification(t *testing.T) {
	cases := []struct {
		name           string
		source         []byte
		wantRoute      bool
		wantReasonPart string
	}{
		{name: "smoke", source: []byte(grammars.ParseSmokeSample("python")), wantRoute: true},
		{name: "assignment_tuple", source: []byte("x = 1\ny = 2\nz = x, y\n"), wantRoute: true},
		{name: "fstring_tuple", source: []byte("x = 1\ny = 2\nz = f\"{x, y}\"\n"), wantRoute: true},
		{name: "nested_block", source: []byte("def f(x):\n    if x:\n        return x + 1\n    return 0\n"), wantRoute: true},
		{name: "recovery_tail", source: []byte("def f(:\n    return 1\n"), wantReasonPart: "recovery"},
	}
	language := grammars.PythonLanguage()
	cLanguage, err := ParityCLanguage("python")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cParser := sitter.NewParser()
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			defer cParser.Close()
			cTree := cParser.Parse(tc.source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C parser returned no tree")
			}
			defer cTree.Close()

			parser := gotreesitter.NewParser(language)
			parser.SetAdmissionCandidateRoute(true)
			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			fresh, err := parser.Parse(tc.source)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Release()
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			routedDelta := routedAfter - routedBefore
			fallbackDelta := fallbackAfter - fallbackBefore
			reason := gotreesitter.AdmissionCandidateLastFallbackReason()
			t.Logf("route=%d/%d reason=%q", routedDelta, fallbackDelta, reason)

			if tc.wantRoute {
				if routedDelta != 1 || fallbackDelta != 0 {
					t.Fatalf("clean case did not route through compact admission: %d/%d", routedDelta, fallbackDelta)
				}
			} else {
				if routedDelta != 0 || fallbackDelta != 1 {
					t.Fatalf("recovery case did not fall back once: %d/%d", routedDelta, fallbackDelta)
				}
				if tc.wantReasonPart != "" && !strings.Contains(reason, tc.wantReasonPart) {
					t.Fatalf("fallback reason=%q, want substring %q", reason, tc.wantReasonPart)
				}
			}
			if tc.wantRoute {
				assertLockedCTreeExact(t, "Python compact "+tc.name, fresh, language, cTree)
			} else {
				if fresh.RootNode().HasError() != cTree.RootNode().HasError() {
					t.Fatalf("Python compact %s root error differs: Go=%v C=%v", tc.name, fresh.RootNode().HasError(), cTree.RootNode().HasError())
				}
				if diff := FirstDivergenceDumpV1(fresh.RootNode(), language, cTree.RootNode()); diff != nil {
					t.Fatalf("Python compact %s error tree diverges from locked C: %+v", tc.name, diff)
				}
				t.Logf("Python compact %s fallback preserves the locked C error tree", tc.name)
			}

			if tc.name != "smoke" {
				return
			}
			offset := bytes.IndexByte(tc.source, '1')
			if offset < 0 {
				t.Fatal("Python smoke source has no numeric edit witness")
			}
			edited := append([]byte(nil), tc.source...)
			edited[offset] = '2'
			edit := gotreesitter.InputEdit{
				StartByte:   uint32(offset),
				OldEndByte:  uint32(offset + 1),
				NewEndByte:  uint32(offset + 1),
				StartPoint:  pointAtOffset(tc.source, offset),
				OldEndPoint: pointAtOffset(tc.source, offset+1),
				NewEndPoint: pointAtOffset(edited, offset+1),
			}
			fresh.Edit(edit)
			incremental, profile, err := parser.ParseIncrementalProfiled(edited, fresh)
			if err != nil {
				t.Fatal(err)
			}
			defer incremental.Release()
			t.Logf("incremental profile=%+v", profile)
			if profile.ReuseUnsupported {
				t.Fatalf("Python incremental certification fell back: %+v", profile)
			}
			cEdited := cParser.Parse(edited, nil)
			if cEdited == nil || cEdited.RootNode() == nil {
				t.Fatal("C parser returned no edited tree")
			}
			defer cEdited.Close()
			assertLockedCTreeExact(t, "Python compact incremental", incremental, language, cEdited)
		})
	}
}
