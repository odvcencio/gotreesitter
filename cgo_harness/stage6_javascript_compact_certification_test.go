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

const stage6JavaScriptRecoverySource = "const f = (a) => a + 1;&\nclass A { m() { return 1 } }\n\n"

// TestStage6JavaScriptCompactCertification proves clean and recovery
// JavaScript shapes through the compact route against locked C output.
func TestStage6JavaScriptCompactCertification(t *testing.T) {
	cases := []struct {
		name           string
		source         []byte
		wantReasonPart string
	}{
		{name: "smoke", source: []byte(grammars.ParseSmokeSample("javascript"))},
		{name: "recovery_keyword", source: []byte(stage6JavaScriptRecoverySource), wantReasonPart: ""},
		{name: "ternary", source: []byte("const value = condition ? left : right;\n")},
	}
	language := grammars.JavascriptLanguage()
	cLanguage, err := ParityCLanguage("javascript")
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
			if routedDelta != 1 || fallbackDelta != 0 {
				t.Fatalf("JavaScript %s did not route through compact admission: %d/%d", tc.name, routedDelta, fallbackDelta)
			}
			if tc.wantReasonPart != "" && !strings.Contains(reason, tc.wantReasonPart) {
				t.Fatalf("fallback reason=%q, want substring %q", reason, tc.wantReasonPart)
			}
			if fresh.RootNode().HasError() != cTree.RootNode().HasError() {
				t.Fatalf("JavaScript %s root error differs: Go=%v C=%v", tc.name, fresh.RootNode().HasError(), cTree.RootNode().HasError())
			}
			if diff := FirstDivergenceDumpV1(fresh.RootNode(), language, cTree.RootNode()); diff != nil {
				t.Fatalf("JavaScript %s tree diverges from locked C: %+v", tc.name, diff)
			}

			if tc.name != "smoke" {
				return
			}
			offset := bytes.IndexByte(tc.source, '1')
			if offset < 0 {
				t.Fatal("JavaScript smoke source has no numeric edit witness")
			}
			edited := append([]byte(nil), tc.source...)
			edited[offset] = '2'
			edit := gotreesitter.InputEdit{
				StartByte: uint32(offset), OldEndByte: uint32(offset + 1), NewEndByte: uint32(offset + 1),
				StartPoint: pointAtOffset(tc.source, offset), OldEndPoint: pointAtOffset(tc.source, offset + 1), NewEndPoint: pointAtOffset(edited, offset + 1),
			}
			fresh.Edit(edit)
			incremental, profile, err := parser.ParseIncrementalProfiled(edited, fresh)
			if err != nil {
				t.Fatal(err)
			}
			defer incremental.Release()
			t.Logf("incremental profile=%+v", profile)
			if profile.ReuseUnsupported {
				t.Fatalf("JavaScript incremental certification fell back: %+v", profile)
			}
			cEdited := cParser.Parse(edited, nil)
			if cEdited == nil || cEdited.RootNode() == nil {
				t.Fatal("C parser returned no edited tree")
			}
			defer cEdited.Close()
			if diff := FirstDivergenceDumpV1(incremental.RootNode(), language, cEdited.RootNode()); diff != nil {
				t.Fatalf("JavaScript incremental tree diverges from locked C: %+v", diff)
			}
		})
	}
}
