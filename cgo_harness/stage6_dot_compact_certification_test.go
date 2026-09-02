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

const stage6DotRecoverySource = "digraph G { a -> }\n"

// TestStage6DotCompactCertification proves clean, recovery, and incremental
// DOT shapes through compact admission against the locked C oracle.
func TestStage6DotCompactCertification(t *testing.T) {
	cases := []struct {
		name      string
		source    []byte
		wantRoute bool
	}{
		{name: "smoke", source: []byte(grammars.ParseSmokeSample("dot")), wantRoute: true},
		{name: "recovery", source: []byte(stage6DotRecoverySource), wantRoute: false},
	}
	language := grammars.DotLanguage()
	cLanguage, err := ParityCLanguage("dot")
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
				assertLockedCTreeExact(t, "DOT compact "+tc.name, goTree, language, cTree)
				return
			}
			if routedDelta != 0 || fallbackDelta != 1 || !strings.Contains(reason, "recovery") {
				t.Fatalf("recovery case route=%d/%d reason=%q", routedDelta, fallbackDelta, reason)
			}
			if divergences := compactT3StructuralDivergences(goTree.RootNode(), language, cTree.RootNode()); len(divergences) != 0 {
				t.Fatalf("DOT compact recovery diverges from C at %d node(s); first: %s", len(divergences), compactT3FormatDivergence(divergences[0]))
			}
		})
	}

	t.Run("incremental", func(t *testing.T) {
		source := []byte("digraph G { a -> b; }\n\n")
		offset := bytes.LastIndexByte(source, '\n')
		if offset < 0 {
			t.Fatal("DOT incremental source has no whitespace edit witness")
		}
		edited := append([]byte(nil), source...)
		edited[offset] = ' '
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
			t.Fatalf("DOT incremental reuse unavailable: %+v", profile)
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
		assertLockedCTreeExact(t, "DOT compact incremental", incremental, language, cTree)
	})
}
