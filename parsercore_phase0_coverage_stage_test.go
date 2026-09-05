//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"strings"
	"testing"
)

func TestCompactRecoveryCoverageFailureIdentifiesStage(t *testing.T) {
	for _, stage := range []string{"derivation", "public-tree"} {
		t.Run(stage, func(t *testing.T) {
			root := &Node{symbol: 2, endByte: 1}
			var coverage diagnosticParserCoreAcceptedLeafCoverageScratch
			var nodesByID []*Node
			if stage == "public-tree" {
				// Raw coverage is complete, but the projected nonterminal leaf
				// has no authenticated relationship to the raw terminal.
				coverage.spans = []diagnosticParserCoreAcceptedLeafSpan{{id: 1, endByte: 1}}
				nodesByID = []*Node{nil, {symbol: 1, endByte: 1}}
			}
			err := finalizeDiagnosticParserCoreAcceptedRootSpan(root, []byte("x"), 1, true, 0,
				func() error { return nil }, 2, &coverage, nodesByID)
			if err == nil || !strings.Contains(err.Error(), "coverage="+stage+" gap=0..1") {
				t.Fatalf("coverage failure did not identify %s: %v", stage, err)
			}
		})
	}
}
