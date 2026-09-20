//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// TestAdmissionCandidateEOFScannerQuiescenceProbeFaultsDeclineFailClosed
// reaches each per-state probe decline on the real Scala two-head frontier.
//
// The probe measures three facts per head state: the token the lexer returned,
// the count of scanner tokens the token source accepted, and the serialized
// scanner payload. Each subtest rewrites exactly one of those measurements to a
// failing value and requires the whole route to decline with that measurement's
// own reason. The served tree must still equal production's, because a decline
// is a fallback, never a wrong answer.
func TestAdmissionCandidateEOFScannerQuiescenceProbeFaultsDeclineFailClosed(t *testing.T) {
	tokenReason, offerReason, payloadReason := gts.CompactEOFScannerQuiescenceDeclineReasonsForTest()
	lang := grammars.ScalaLanguage()
	source := []byte(scalaEOFScannerQuiescenceWitnesses[0].source)

	production := gts.NewParser(lang)
	production.SetAdmissionCandidateRoute(false)
	productionTree, err := production.Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer productionTree.Release()
	productionInspection, err := benchfixtures.InspectGoTree(productionTree.RootNode(), lang)
	if err != nil {
		t.Fatalf("inspect production tree: %v", err)
	}

	tests := []struct {
		name    string
		token   func(gts.StateID, gts.Token) gts.Token
		offered func(gts.StateID, uint32) uint32
		payload func(gts.StateID, bool) bool
		want    string
	}{
		{
			// A head state whose own row yields a positive-width external
			// token names a head C would shift, not a dead head.
			name: "state_lexes_its_own_token",
			token: func(_ gts.StateID, candidate gts.Token) gts.Token {
				candidate.Symbol = 140
				candidate.EndByte = candidate.StartByte + 1
				candidate.ExternalScannerToken = true
				return candidate
			},
			want: tokenReason,
		},
		{
			// The lexer returned end of input, but the scanner had offered a
			// token the lexer then dropped. The head is not quiescent.
			name:    "scanner_offered_a_dropped_token",
			offered: func(_ gts.StateID, count uint32) uint32 { return count + 1 },
			want:    offerReason,
		},
		{
			// The run left the serialized scanner state different from the
			// authenticated election-start payload.
			name:    "state_changed_the_scanner_payload",
			payload: func(gts.StateID, bool) bool { return false },
			want:    payloadReason,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			restore := gts.CompactEOFScannerQuiescenceProbeFaultForTest(test.token, test.offered, test.payload)
			defer restore()

			gts.ResetAdmissionCandidateCountersForTest()
			candidate := gts.NewParser(lang)
			candidate.SetAdmissionCandidateRoute(true)
			candidateTree, err := candidate.Parse(source)
			if err != nil {
				t.Fatalf("candidate parse: %v", err)
			}
			defer candidateTree.Release()

			routed, fallback := gts.AdmissionCandidateCounters()
			if routed != 0 || fallback != 1 {
				t.Fatalf("route counters = %d/%d, want 0/1 (the fault must decline)", routed, fallback)
			}
			if reason := gts.CompactEOFScannerQuiescenceLastDeclineForTest(); reason != test.want {
				t.Fatalf("probe decline = %q, want %q", reason, test.want)
			}
			// The route reason the public API reports collapses to the
			// acceptance-gate class unless the census opt-in is set, so assert
			// only that the parse did fall back through that class.
			if reason := gts.AdmissionCandidateLastFallbackReason(); !strings.Contains(reason, "did not accept EOF") &&
				!strings.Contains(reason, test.want) {
				t.Fatalf("fallback reason = %q, want an acceptance-gate decline", reason)
			}
			candidateInspection, err := benchfixtures.InspectGoTree(candidateTree.RootNode(), lang)
			if err != nil {
				t.Fatalf("inspect candidate tree: %v", err)
			}
			if candidateInspection.SHA256 != productionInspection.SHA256 {
				t.Fatalf(
					"declined route served %s, want production's %s",
					candidateInspection.SHA256, productionInspection.SHA256,
				)
			}
		})
	}
}
