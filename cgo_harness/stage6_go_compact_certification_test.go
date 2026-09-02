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

// TestStage6GoCompactCertification proves clean and recovery Go shapes
// through compact admission against the locked C oracle.
func TestStage6GoCompactCertification(t *testing.T) {
	cases := []struct {
		name           string
		source         []byte
		wantRoute      bool
		wantReasonPart string
	}{
		{name: "smoke", source: []byte(grammars.ParseSmokeSample("go")), wantRoute: true},
		{name: "method_and_composite", source: []byte("package p\ntype T struct { X int }\nfunc (t T) M(x int) int { return T{X: t.X + x}.X }\n"), wantRoute: true},
		{name: "type_switch", source: []byte("package p\nfunc f(x any) { switch v := x.(type) { case int: _ = v; default: } }\n"), wantRoute: true},
		{name: "recovery", source: []byte("package main\nfunc f( {\n\treturn 1\n}\n"), wantReasonPart: "recovery"},
	}
	language := grammars.GoLanguage()
	cLanguage, err := ParityCLanguage("go")
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
			goTree, err := parser.Parse(tc.source)
			if err != nil {
				t.Fatal(err)
			}
			defer goTree.Release()
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			routedDelta := routedAfter - routedBefore
			fallbackDelta := fallbackAfter - fallbackBefore
			reason := gotreesitter.AdmissionCandidateLastFallbackReason()
			t.Logf("route=%d/%d reason=%q", routedDelta, fallbackDelta, reason)

			if tc.wantRoute {
				if routedDelta != 1 || fallbackDelta != 0 {
					t.Fatalf("clean case did not route through compact admission: %d/%d", routedDelta, fallbackDelta)
				}
				assertLockedCTreeExact(t, "Go compact "+tc.name, goTree, language, cTree)
			} else {
				if routedDelta != 0 || fallbackDelta != 1 {
					t.Fatalf("recovery case did not fall back once: %d/%d", routedDelta, fallbackDelta)
				}
				if tc.wantReasonPart != "" && !strings.Contains(reason, tc.wantReasonPart) {
					t.Fatalf("fallback reason=%q, want substring %q", reason, tc.wantReasonPart)
				}
				if goTree.RootNode().HasError() != cTree.RootNode().HasError() {
					t.Fatalf("Go %s root error differs: Go=%v C=%v", tc.name, goTree.RootNode().HasError(), cTree.RootNode().HasError())
				}
				if diff := FirstDivergenceDumpV1(goTree.RootNode(), language, cTree.RootNode()); diff != nil {
					t.Fatalf("Go %s error tree diverges from locked C: %+v", tc.name, diff)
				}
			}

			if tc.name != "smoke" {
				return
			}
			offset := bytes.IndexByte(tc.source, '1')
			if offset < 0 {
				t.Fatal("Go smoke source has no numeric edit witness")
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
			goTree.Edit(edit)
			incremental, profile, err := parser.ParseIncrementalProfiled(edited, goTree)
			if err != nil {
				t.Fatal(err)
			}
			defer incremental.Release()
			t.Logf("incremental profile=%+v", profile)
			if profile.ReuseUnsupported || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
				t.Fatalf("Go incremental certification did not reuse the clean prefix: %+v", profile)
			}
			cEdited := cParser.Parse(edited, nil)
			if cEdited == nil || cEdited.RootNode() == nil {
				t.Fatal("C parser returned no edited tree")
			}
			defer cEdited.Close()
			assertLockedCTreeExact(t, "Go compact incremental", incremental, language, cEdited)
		})
	}
}
