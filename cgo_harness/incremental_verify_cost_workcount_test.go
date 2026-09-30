//go:build cgo && treesitter_c_parity && gts_workcount

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

func verifyCostBeginWorkCount() { gts.BeginDiagnosticWorkCount() }

func verifyCostEndWorkCount(t *testing.T) {
	counts := gts.EndDiagnosticWorkCount()
	t.Logf("WORK attempts=%d shifts=%d reductions=%d lookups=%d lex_calls=%d leaves=%d parents=%d", len(counts.Attempts), counts.Shifts, counts.Reductions, counts.TableLookupsProxy, counts.LexerFrontDoorCallsProxy, counts.LeafConstructionsProxy, counts.ParentConstructionsProxy)
}
