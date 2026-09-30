//go:build !gts_incr_census && !gts_workcount

package gotreesitter

import (
	"bytes"
	"testing"
)

func TestIncrementalCensusProductionHooksErased(t *testing.T) {
	binary := buildProductionTestBinary(t)
	nm := runGoTool(t, "nm", binary)
	for _, symbol := range []string{"gotreesitter.incrCensus", "gotreesitter.DiagnosticObserveIncrementalReuse", "internal/diag/incrcensus"} {
		if bytes.Contains(nm, []byte(symbol)) {
			t.Fatalf("production binary retains census symbol %s", symbol)
		}
	}
}
