//go:build !gts_parsercorephase0 && !gts_workcount

package gotreesitter

import (
	"bytes"
	"testing"
)

func TestParserCorePhaseZeroDriverCompilesOutOfOrdinaryBuild(t *testing.T) {
	binary := buildProductionTestBinary(t)
	symbols := runGoTool(t, "nm", binary)
	for _, forbidden := range [][]byte{
		[]byte("DiagnosticParseParserCorePrefix"),
		[]byte("internal/parsercorephase0.(*Core)"),
	} {
		if bytes.Contains(symbols, forbidden) {
			t.Fatalf("ordinary binary retains tagged parser-core driver symbol %q", forbidden)
		}
	}
}
