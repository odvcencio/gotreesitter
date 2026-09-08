//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"os"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestCompactIncludedRangeRecoveryRetention(t *testing.T) {
	source, err := os.ReadFile("testdata/included_ranges/go_two_fences.go")
	if err != nil {
		t.Fatal(err)
	}
	parser := gts.NewParser(grammars.GoLanguage())
	parser.SetAdmissionCandidateRoute(true)
	var ranges []gts.Range
	for _, span := range [][2]int{{0, 150}, {203, 250}} {
		ranges = append(ranges, gts.Range{StartByte: uint32(span[0]), EndByte: uint32(span[1]), StartPoint: admissionEOFPoint(source[:span[0]]), EndPoint: admissionEOFPoint(source[:span[1]])})
	}
	parser.SetIncludedRanges(ranges)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	if !strings.Contains(gts.AdmissionCandidateLastFallbackReason(), "included-range recovery is not certified") {
		t.Fatalf("recovery did not decline: %s", gts.AdmissionCandidateLastFallbackReason())
	}
	if retained := gts.AdmissionCandidateCompactFootprintBytesForTest(parser); retained > 64<<20 {
		t.Fatalf("decline retained %d bytes", retained)
	}
}
