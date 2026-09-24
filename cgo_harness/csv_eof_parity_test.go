//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars/csv"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const (
	csvIncrementalSource = "a,b,c\n1,2,3\n"
	csvIncrementalEdited = "a,b,c\n1,4,3\n"
)

var csvEOFParitySources = []string{"", "a", "a\n", csvIncrementalSource, "a,b,\n", "\n", "a\r\n"}

func TestCsvEOFAcceptLockedC(t *testing.T) {
	cl, err := ParityCLanguage("csv")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	var peakHeaders, peakDerivations uint64
	for _, source := range csvEOFParitySources {
		t.Run(source, func(t *testing.T) {
			oracle := cp.Parse([]byte(source), nil)
			if oracle == nil {
				t.Fatal("C tree is nil")
			}
			defer oracle.Close()
			for _, compact := range []bool{false, true} {
				name := "production"
				if compact {
					name = "compact"
				}
				t.Run(name, func(t *testing.T) {
					lang := csv.Language()
					parser := gts.NewParser(lang)
					parser.SetAdmissionCandidateRoute(compact)
					parser.SetCompactCertificationTelemetry(compact)
					before, failed := gts.AdmissionCandidateCounters()
					tree, err := parser.Parse([]byte(source))
					if err != nil {
						t.Fatal(err)
					}
					defer tree.Release()
					assertLockedCTreeExact(t, name, tree, lang, oracle)
					if compact {
						routed, fallback := gts.AdmissionCandidateCounters()
						if routed != before+1 || fallback != failed {
							t.Fatalf("compact parse fell back: %s", gts.AdmissionCandidateLastFallbackReason())
						}
						runtime := tree.ParseRuntime()
						if runtime.CompactPeakHeaders == 0 || runtime.CompactPeakDerivations == 0 {
							t.Fatalf("compact certification peaks are empty: %+v", runtime)
						}
						if runtime.CompactPeakHeaders > peakHeaders {
							peakHeaders = runtime.CompactPeakHeaders
						}
						if runtime.CompactPeakDerivations > peakDerivations {
							peakDerivations = runtime.CompactPeakDerivations
						}
					}
				})
			}
		})
	}
	t.Logf("CSV EOF compact route=7/0 peak_headers=%d peak_derivations=%d", peakHeaders, peakDerivations)
}

// TestCSVTrailingFieldOpenGap pins the minimal CSV mismatch and its missing leaf.
// Set GTS_STAGE6_CSV_STRICT=1 to require exact locked-C parity after a fix.
func TestCSVTrailingFieldOpenGap(t *testing.T) {
	witnesses := []struct {
		name   string
		source string
		wantC  string
	}{
		{name: "minimal-comma", source: ",", wantC: `(document (row (field (text))) (ERROR))`},
		{name: "trailing-field", source: "a,b,", wantC: `(document (row (field (text)) (field (text)) (field (number (MISSING "number_token1")))))`},
	}
	identity, err := COracleIdentity("csv")
	if err != nil {
		t.Fatal(err)
	}
	lang := csv.Language()
	blobSHA, ok := lang.GrammarBlobSHA256()
	if !ok {
		t.Fatal("CSV language has no blob identity")
	}
	manifest := ""
	for index, witness := range csvEOFParitySources {
		manifest += fmt.Sprintf("eof-%d %x\n", index, sha256.Sum256([]byte(witness)))
	}
	for _, witness := range witnesses {
		manifest += fmt.Sprintf("%s %x\n", witness.name, sha256.Sum256([]byte(witness.source)))
	}
	manifest += fmt.Sprintf("incremental-edited %x\n", sha256.Sum256([]byte(csvIncrementalEdited)))
	manifestSHA := fmt.Sprintf("%x", sha256.Sum256([]byte(manifest)))
	const wantManifestSHA = "c16df28d86737383a58de9977f98027eca88efb52a0347a880a24fe8ae728ecf"
	if manifestSHA != wantManifestSHA {
		t.Fatalf("CSV source manifest changed: got %s want %s", manifestSHA, wantManifestSHA)
	}
	t.Logf("oracle=%+v blob_sha256=%x source_manifest_sha256=%s entries:\n%s", identity, blobSHA, manifestSHA, manifest)
	cl, err := ParityCLanguage("csv")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	strict := os.Getenv("GTS_STAGE6_CSV_STRICT") == "1"
	for _, witness := range witnesses {
		t.Run(witness.name, func(t *testing.T) {
			oracle := cp.Parse([]byte(witness.source), nil)
			if oracle == nil {
				t.Fatal("C tree is nil")
			}
			defer oracle.Close()
			if !oracle.RootNode().HasError() {
				t.Fatal("locked C must report the unterminated field")
			}
			if got := oracle.RootNode().ToSexp(); got != witness.wantC {
				t.Fatalf("locked C tree changed: %s", got)
			}
			for _, compact := range []bool{false, true} {
				name := "production"
				if compact {
					name = "compact"
				}
				t.Run(name, func(t *testing.T) {
					parser := gts.NewParser(lang)
					parser.SetAdmissionCandidateRoute(compact)
					routedBefore, fallbackBefore := gts.AdmissionCandidateCounters()
					tree, err := parser.Parse([]byte(witness.source))
					if err != nil {
						t.Fatal(err)
					}
					defer tree.Release()
					routedAfter, fallbackAfter := gts.AdmissionCandidateCounters()
					t.Logf("route=%d/%d max_stacks=%d", routedAfter-routedBefore, fallbackAfter-fallbackBefore, tree.ParseRuntime().MaxStacksSeen)
					if strict {
						assertLockedCTreeExactWithErrors(t, name, tree, lang, oracle)
						return
					}
					if tree.RootNode().HasError() {
						t.Fatalf("known gap closed; enable strict gate: %s", tree.RootNode().SExpr(lang))
					}
					diff := FirstDivergenceDumpV1(tree.RootNode(), lang, oracle.RootNode())
					if diff == nil || diff.Path != "/document" || diff.Category != "error" || diff.GoValue != "false" || diff.CValue != "true" {
						t.Fatalf("first locked-C divergence changed: %+v", diff)
					}
					t.Logf("open gap: %s; first divergence: %+v", tree.RootNode().SExpr(lang), diff)
				})
			}
		})
	}
}

func TestCSVIncrementalLockedC(t *testing.T) {
	source := []byte(csvIncrementalSource)
	offset := bytes.IndexByte(source, '2')
	if offset < 0 {
		t.Fatal("CSV source has no edit witness")
	}
	edited := bytes.Clone(source)
	edited[offset] = '4'
	if string(edited) != csvIncrementalEdited {
		t.Fatal("CSV edited source no longer matches its receipt")
	}
	edit := gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset + 1),
		StartPoint:  pointAtOffset(source, offset),
		OldEndPoint: pointAtOffset(source, offset+1),
		NewEndPoint: pointAtOffset(edited, offset+1),
	}
	lang := csv.Language()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	oldTree, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer oldTree.Release()
	oldTree.Edit(edit)
	routedBefore, fallbackBefore := gts.AdmissionCandidateCounters()
	incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	defer incremental.Release()
	routedAfter, fallbackAfter := gts.AdmissionCandidateCounters()
	t.Logf("route=%d/%d reused_subtrees=%d reused_bytes=%d reuse_unsupported=%t", routedAfter-routedBefore, fallbackAfter-fallbackBefore, profile.ReusedSubtrees, profile.ReusedBytes, profile.ReuseUnsupported)
	if profile.ReuseUnsupported || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
		t.Fatalf("CSV incremental reuse unavailable: %+v", profile)
	}

	cl, err := ParityCLanguage("csv")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	cOld := cp.Parse(source, nil)
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
	cIncremental := cp.Parse(edited, cOld)
	if cIncremental == nil || cIncremental.RootNode() == nil {
		t.Fatal("C parser returned no incremental tree")
	}
	defer cIncremental.Close()
	assertLockedCTreeExact(t, "CSV incremental", incremental, lang, cIncremental)
	cFresh := cp.Parse(edited, nil)
	if cFresh == nil || cFresh.RootNode() == nil {
		t.Fatal("C parser returned no fresh edited tree")
	}
	defer cFresh.Close()
	incrementalDigest, err := COracleDeepDigest(cIncremental)
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
	t.Logf("source_sha256=%x edited_sha256=%x", sha256.Sum256(source), sha256.Sum256(edited))
}
