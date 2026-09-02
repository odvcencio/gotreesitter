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

const (
	stage6JSONNestedSource   = "{\"items\":[{\"id\":1,\"ok\":true},{\"id\":2,\"ok\":false}],\"name\":\"x\"}\n"
	stage6JSONRecoverySource = "{\"a\":1\n"
)

// TestStage6JSONCompactCertification proves clean and recovery JSON shapes
// through compact admission against the locked C oracle.
func TestStage6JSONCompactCertification(t *testing.T) {
	cases := []struct {
		name      string
		source    []byte
		wantRoute bool
	}{
		{name: "smoke", source: []byte(grammars.ParseSmokeSample("json")), wantRoute: true},
		{name: "nested", source: []byte(stage6JSONNestedSource), wantRoute: true},
		{name: "recovery", source: []byte(stage6JSONRecoverySource), wantRoute: false},
	}
	language := grammars.JsonLanguage()
	cLanguage, err := ParityCLanguage("json")
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
				assertLockedCTreeExact(t, "JSON compact "+tc.name, goTree, language, cTree)
			} else {
				if routedDelta != 0 || fallbackDelta != 1 || !strings.Contains(reason, "recovery") {
					t.Fatalf("recovery case route=%d/%d reason=%q", routedDelta, fallbackDelta, reason)
				}
				if goTree.RootNode().HasError() != cTree.RootNode().HasError() {
					t.Fatalf("JSON recovery root error differs: Go=%v C=%v", goTree.RootNode().HasError(), cTree.RootNode().HasError())
				}
				if diff := FirstDivergenceDumpV1(goTree.RootNode(), language, cTree.RootNode()); diff != nil {
					t.Fatalf("JSON recovery tree diverges from locked C: %+v", diff)
				}
			}
		})
	}

	t.Run("incremental", func(t *testing.T) {
		source := []byte(grammars.ParseSmokeSample("json"))
		offset := bytes.IndexByte(source, '1')
		if offset < 0 {
			t.Fatal("JSON smoke source has no numeric edit witness")
		}
		edited := append([]byte(nil), source...)
		edited[offset] = '2'
		parser := gotreesitter.NewParser(language)
		parser.SetAdmissionCandidateRoute(true)
		oldTree, err := parser.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		defer oldTree.Release()
		oldTree.Edit(gotreesitter.InputEdit{
			StartByte:   uint32(offset),
			OldEndByte:  uint32(offset + 1),
			NewEndByte:  uint32(offset + 1),
			StartPoint:  pointAtOffset(source, offset),
			OldEndPoint: pointAtOffset(source, offset+1),
			NewEndPoint: pointAtOffset(edited, offset+1),
		})
		incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
		if err != nil {
			t.Fatal(err)
		}
		defer incremental.Release()
		t.Logf("profile=%+v", profile)
		if profile.ReuseUnsupported || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
			t.Fatalf("JSON incremental reuse unavailable: %+v", profile)
		}

		cParser := sitter.NewParser()
		if err := cParser.SetLanguage(cLanguage); err != nil {
			t.Fatal(err)
		}
		defer cParser.Close()
		cTree := cParser.Parse(edited, nil)
		if cTree == nil || cTree.RootNode() == nil {
			t.Fatal("C parser returned no edited tree")
		}
		defer cTree.Close()
		assertLockedCTreeExact(t, "JSON compact incremental", incremental, language, cTree)
	})
}
