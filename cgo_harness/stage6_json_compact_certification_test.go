//go:build cgo && treesitter_c_parity && gts_parsercorephase0

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"fmt"
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
	gotreesitter.ResetAdmissionCandidateCounters()
	cases := []struct {
		name      string
		source    []byte
		wantRoute bool
	}{
		{name: "smoke", source: []byte(grammars.ParseSmokeSample("json")), wantRoute: true},
		{name: "nested", source: []byte(stage6JSONNestedSource), wantRoute: true},
		{name: "recovery", source: []byte(stage6JSONRecoverySource), wantRoute: false},
	}
	source := []byte(grammars.ParseSmokeSample("json"))
	offset := bytes.IndexByte(source, '1')
	if offset < 0 {
		t.Fatal("JSON smoke source has no numeric edit witness")
	}
	edited := append([]byte(nil), source...)
	edited[offset] = '2'
	var manifest strings.Builder
	for _, tc := range cases {
		fmt.Fprintf(&manifest, "%s %x\n", tc.name, sha256.Sum256(tc.source))
	}
	fmt.Fprintf(&manifest, "incremental-edited %x\n", sha256.Sum256(edited))
	manifestSHA := fmt.Sprintf("%x", sha256.Sum256([]byte(manifest.String())))
	const wantManifestSHA = "592806b195730d12401a93e9f8b9e2fe34d1ff2d1f5ed658c7d3dcb7af5807bb"
	if manifestSHA != wantManifestSHA {
		t.Fatalf("JSON source manifest changed: got %s want %s", manifestSHA, wantManifestSHA)
	}
	t.Logf("source_manifest_sha256=%s entries:\n%s", manifestSHA, manifest.String())
	language := grammars.JsonLanguage()
	cLanguage, err := ParityCLanguage("json")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := COracleIdentity("json")
	if err != nil {
		t.Fatal(err)
	}
	blobSHA, ok := language.GrammarBlobSHA256()
	if !ok {
		t.Fatal("JSON language has no blob identity")
	}
	t.Logf("oracle=%+v blob_sha256=%x", identity, blobSHA)

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
			parser.SetCompactCertificationTelemetry(true)
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
			runtime := goTree.ParseRuntime()
			t.Logf("max_stacks=%d compact_reductions=%d peak_headers=%d peak_derivations=%d", runtime.MaxStacksSeen, runtime.CompactReductions, runtime.CompactPeakHeaders, runtime.CompactPeakDerivations)

			if tc.wantRoute {
				if routedDelta != 1 || fallbackDelta != 0 {
					t.Fatalf("clean case did not route through compact admission: %d/%d", routedDelta, fallbackDelta)
				}
				if runtime.CompactPeakHeaders == 0 || runtime.CompactPeakDerivations == 0 {
					t.Fatalf("compact certification peaks are empty: %+v", runtime)
				}
				assertLockedCTreeExact(t, "JSON compact "+tc.name, goTree, language, cTree)
			} else {
				if routedDelta != 0 || fallbackDelta != 1 || !strings.Contains(reason, "recovery") {
					t.Fatalf("recovery case route=%d/%d reason=%q", routedDelta, fallbackDelta, reason)
				}
				assertLockedCTreeExactWithErrors(t, "JSON compact "+tc.name, goTree, language, cTree)
			}
		})
	}

	t.Run("incremental", func(t *testing.T) {
		parser := gotreesitter.NewParser(language)
		parser.SetAdmissionCandidateRoute(true)
		parser.SetCompactCertificationTelemetry(true)
		oldTree, err := parser.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		defer oldTree.Release()
		edit := gotreesitter.InputEdit{
			StartByte:   uint32(offset),
			OldEndByte:  uint32(offset + 1),
			NewEndByte:  uint32(offset + 1),
			StartPoint:  pointAtOffset(source, offset),
			OldEndPoint: pointAtOffset(source, offset+1),
			NewEndPoint: pointAtOffset(edited, offset+1),
		}
		oldTree.Edit(edit)
		routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
		incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
		if err != nil {
			t.Fatal(err)
		}
		defer incremental.Release()
		routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
		t.Logf("incremental route=%d/%d reused_subtrees=%d reused_bytes=%d reuse_unsupported=%t compact_peaks=%d/%d", routedAfter-routedBefore, fallbackAfter-fallbackBefore, profile.ReusedSubtrees, profile.ReusedBytes, profile.ReuseUnsupported, incremental.ParseRuntime().CompactPeakHeaders, incremental.ParseRuntime().CompactPeakDerivations)
		if profile.ReuseUnsupported || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
			t.Fatalf("JSON incremental reuse unavailable: %+v", profile)
		}

		cParser := sitter.NewParser()
		if err := cParser.SetLanguage(cLanguage); err != nil {
			t.Fatal(err)
		}
		defer cParser.Close()
		cOld := cParser.Parse(source, nil)
		if cOld == nil || cOld.RootNode() == nil {
			t.Fatal("C parser returned no original tree")
		}
		defer cOld.Close()
		cOld.Edit(&sitter.InputEdit{
			StartByte:      uint(edit.StartByte),
			OldEndByte:     uint(edit.OldEndByte),
			NewEndByte:     uint(edit.NewEndByte),
			StartPosition:  sitter.Point{Row: uint(edit.StartPoint.Row), Column: uint(edit.StartPoint.Column)},
			OldEndPosition: sitter.Point{Row: uint(edit.OldEndPoint.Row), Column: uint(edit.OldEndPoint.Column)},
			NewEndPosition: sitter.Point{Row: uint(edit.NewEndPoint.Row), Column: uint(edit.NewEndPoint.Column)},
		})
		cTree := cParser.Parse(edited, cOld)
		if cTree == nil || cTree.RootNode() == nil {
			t.Fatal("C parser returned no edited tree")
		}
		defer cTree.Close()
		assertLockedCTreeExact(t, "JSON compact incremental", incremental, language, cTree)
		cFresh := cParser.Parse(edited, nil)
		if cFresh == nil || cFresh.RootNode() == nil {
			t.Fatal("C parser returned no fresh edited tree")
		}
		defer cFresh.Close()
		incrementalDigest, err := COracleDeepDigest(cTree)
		if err != nil {
			t.Fatal(err)
		}
		freshDigest, err := COracleDeepDigest(cFresh)
		if err != nil {
			t.Fatal(err)
		}
		if incrementalDigest != freshDigest {
			t.Fatalf("C incremental digest %s differs from fresh %s", incrementalDigest, freshDigest)
		}
	})
}
