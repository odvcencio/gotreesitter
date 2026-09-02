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

const stage6PEMRecoverySource = "-----BEGIN CERTIFICATE-----\nMIIC\n"

// TestStage6PEMCompactCertification proves clean, recovery, and incremental
// PEM shapes through compact admission against the locked C oracle.
func TestStage6PEMCompactCertification(t *testing.T) {
	cases := []struct {
		name      string
		source    []byte
		wantRoute bool
	}{
		{name: "smoke", source: []byte(grammars.ParseSmokeSample("pem")), wantRoute: true},
		{name: "recovery", source: []byte(stage6PEMRecoverySource), wantRoute: false},
	}
	language := grammars.PemLanguage()
	cLanguage, err := ParityCLanguage("pem")
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
				assertLockedCTreeExact(t, "PEM compact "+tc.name, goTree, language, cTree)
				return
			}
			if routedDelta != 0 || fallbackDelta != 1 || !strings.Contains(reason, "recovery") {
				t.Fatalf("recovery case route=%d/%d reason=%q", routedDelta, fallbackDelta, reason)
			}
			if divergences := compactT3StructuralDivergences(goTree.RootNode(), language, cTree.RootNode()); len(divergences) != 0 {
				t.Fatalf("PEM compact recovery diverges from C at %d node(s); first: %s", len(divergences), compactT3FormatDivergence(divergences[0]))
			}
		})
	}

	t.Run("incremental", func(t *testing.T) {
		source := []byte(grammars.ParseSmokeSample("pem"))
		offset := bytes.Index(source, []byte("MIIC"))
		if offset < 0 {
			t.Fatal("PEM smoke source has no payload edit witness")
		}
		edited := append([]byte(nil), source[:offset]...)
		edited = append(edited, "MIIX"...)
		edited = append(edited, source[offset+len("MIIC"):]...)
		parser := gotreesitter.NewParser(language)
		parser.SetAdmissionCandidateRoute(true)
		oldTree, err := parser.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		defer oldTree.Release()
		oldTree.Edit(gotreesitter.InputEdit{
			StartByte:   uint32(offset),
			OldEndByte:  uint32(offset + len("MIIC")),
			NewEndByte:  uint32(offset + len("MIIX")),
			StartPoint:  pointAtOffset(source, offset),
			OldEndPoint: pointAtOffset(source, offset+len("MIIC")),
			NewEndPoint: pointAtOffset(edited, offset+len("MIIX")),
		})
		incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
		if err != nil {
			t.Fatal(err)
		}
		defer incremental.Release()
		t.Logf("profile=%+v", profile)
		if profile.ReuseUnsupported || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
			t.Fatalf("PEM incremental reuse unavailable: %+v", profile)
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
		assertLockedCTreeExact(t, "PEM compact incremental", incremental, language, cTree)
	})
}
