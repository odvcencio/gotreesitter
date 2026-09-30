//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Compact checkpoint transfer authenticates the changed indentation frontier.
// The unchanged functions must be borrowed with the same tree as a fresh parse.
func TestCompactCheckpointedScannerPrefixFrontierReuse(t *testing.T) {
	t.Setenv("GOT_PARSE_MEMORY_BUDGET_MB", "512")
	gts.ResetParseEnvConfigCacheForTests()
	t.Cleanup(gts.ResetParseEnvConfigCacheForTests)
	for _, targetBytes := range []int{20 * 1024, 137 * 1024} {
		t.Run(fmt.Sprintf("%dKB", targetBytes/1024), func(t *testing.T) {
			source := compactPrefixPythonSource(targetBytes)
			const marker = "    return 0\n"
			offset := bytes.Index(source, []byte(marker))
			if offset < 0 {
				t.Fatalf("compact Python fixture has no first-child marker %q", marker)
			}

			edited := make([]byte, 0, len(source)+4)
			edited = append(edited, source[:offset]...)
			edited = append(edited, []byte("    ")...)
			edited = append(edited, source[offset:]...)
			edit := gts.InputEdit{
				StartByte:   uint32(offset),
				OldEndByte:  uint32(offset),
				NewEndByte:  uint32(offset + 4),
				StartPoint:  compactPrefixPythonPointAt(source, offset),
				OldEndPoint: compactPrefixPythonPointAt(source, offset),
				NewEndPoint: compactPrefixPythonPointAt(edited, offset+4),
			}

			gts.ResetAdmissionCandidateCountersForTest()
			parser := gts.NewParser(grammars.PythonLanguage())
			parser.SetAdmissionCandidateRoute(true)
			routedBefore, fallbackBefore := gts.AdmissionCandidateCounters()
			oldTree, err := parser.Parse(source)
			if err != nil {
				t.Fatalf("compact base parse: %v", err)
			}
			defer oldTree.Release()
			routedAfter, fallbackAfter := gts.AdmissionCandidateCounters()
			if routedAfter != routedBefore+1 || fallbackAfter != fallbackBefore {
				t.Fatalf("Python compact base route counters=%d/%d, want one route and no fallback", routedAfter-routedBefore, fallbackAfter-fallbackBefore)
			}
			if !oldTree.ParseRuntime().CompactExternalScannerCheckpointTransferProven {
				t.Fatalf("compact base lacks scanner checkpoint transfer proof: runtime=%+v", oldTree.ParseRuntime())
			}

			oldTree.Edit(edit)
			incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
			if err != nil {
				t.Fatalf("compact first-child incremental parse: %v", err)
			}
			defer incremental.Release()
			if profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "" || !profile.OldTreeReuseRoute || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 || !incremental.ParseRuntime().CompactIncrementalReuseRoute {
				t.Fatalf("compact first-child reuse profile=%+v", profile)
			}
			if profile.ReuseRejectFrontierProofUnavailable != 0 {
				t.Fatalf("authenticated compact frontier was rejected: %+v", profile)
			}
			if want := uint64(incremental.ParseRuntime().NodesAllocated); profile.NewNodesAllocated != want {
				t.Fatalf("compact reuse node attribution=%d, want fresh tree node count %d: %+v", profile.NewNodesAllocated, want, profile)
			}
			if got := incremental.RootNode().EndByte(); got != uint32(len(edited)) {
				t.Fatalf("compact first-child root end=%d, want %d", got, len(edited))
			}
			freshParser := gts.NewParser(grammars.PythonLanguage())
			freshParser.SetAdmissionCandidateRoute(true)
			fresh, err := freshParser.Parse(edited)
			if err != nil {
				t.Fatalf("fresh Python oracle parse: %v", err)
			}
			defer fresh.Release()
			requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, grammars.PythonLanguage())
			if incremental.RootNode().HasError() != fresh.RootNode().HasError() {
				t.Fatalf("compact first-child error state differs: incremental=%t fresh=%t", incremental.RootNode().HasError(), fresh.RootNode().HasError())
			}
			t.Logf("compact first-child reuse bytes=%d tokens=%d new_nodes=%d reused=%d/%d reparse_nanos=%d", len(source), profile.TokensConsumed, profile.NewNodesAllocated, profile.ReusedSubtrees, profile.ReusedBytes, profile.ReparseNanos)
		})
	}
}

func compactPrefixPythonSource(targetBytes int) []byte {
	var source strings.Builder
	source.Grow(targetBytes + 256)
	for index := 0; source.Len() < targetBytes; index++ {
		fmt.Fprintf(&source, "def f%d(a):\n    if a:\n        return 0\n    return 0\n\n", index)
	}
	return []byte(source.String())
}

func compactPrefixPythonPointAt(source []byte, offset int) gts.Point {
	point := gts.Point{}
	for _, b := range source[:offset] {
		if b == '\n' {
			point.Row++
			point.Column = 0
			continue
		}
		point.Column++
	}
	return point
}

// An opt-out keeps the generic prefix fallback covered independently of the
// opted-in scanner's positive reuse expectation.
type compactPrefixOptOutScanner struct{ gts.ExternalScanner }

func (compactPrefixOptOutScanner) SupportsIncrementalReuse() bool               { return true }
func (compactPrefixOptOutScanner) UsesExternalScannerCheckpoints() bool         { return true }
func (compactPrefixOptOutScanner) RequiresIncrementalPrefixFrontierProof() bool { return true }
func (compactPrefixOptOutScanner) SupportsCompactIncrementalReuse() bool        { return false }

func TestCompactCheckpointedScannerPrefixFrontierFallbackAvoidsCandidateWalk(t *testing.T) {
	lang := *grammars.PythonLanguage()
	lang.ExternalScanner = compactPrefixOptOutScanner{lang.ExternalScanner}
	source := compactPrefixPythonSource(20 * 1024)
	at := bytes.Index(source, []byte("    return 0\n"))
	edited := append(append(append([]byte{}, source[:at]...), []byte("    ")...), source[at:]...)
	p := gts.NewParser(&lang)
	p.SetAdmissionCandidateRoute(true)
	old, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	old.Edit(gts.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at), NewEndByte: uint32(at + 4), StartPoint: compactPrefixPythonPointAt(source, at), OldEndPoint: compactPrefixPythonPointAt(source, at), NewEndPoint: compactPrefixPythonPointAt(edited, at+4)})
	next, profile, err := p.ParseIncrementalProfiled(edited, old)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
	if profile.ReuseUnsupportedReason != "external_scanner_prefix_frontier_unproven" || !profile.ReuseUnsupported || profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 || profile.ReuseCursorNanos != 0 || profile.ReuseRejectFrontierProofUnavailable != 0 {
		t.Fatalf("opt-out fallback scanned candidates: %+v", profile)
	}
	fresh, err := p.Parse(edited)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Release()
	requireIncrementalDeepTreeMatchesFresh(t, next, fresh, &lang)
}
