//go:build gts_parsercorephase0

package gotreesitter

import (
	"testing"
	"unsafe"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func mustDiagnosticParserCoreGenericCell(t testing.TB, compact *core.Core, headerIndex int, header diagnosticParserCoreHeader, lookahead core.Symbol) diagnosticParserCoreGenericCell {
	t.Helper()
	boundary, err := compact.ClassifyBoundary(header.head, lookahead)
	if err != nil {
		t.Fatal(err)
	}
	return diagnosticParserCoreGenericCell{headerIndex: int32(headerIndex), boundary: boundary}
}

func TestDiagnosticParserCoreClassifiedBoundaryAndReductionPlanShape(t *testing.T) {
	prepared, err := parserCoreWarmPrepare()
	if err != nil {
		t.Fatal(err)
	}
	tables, err := newParserCoreRootTables(prepared.parser)
	if err != nil {
		t.Fatal(err)
	}
	reduceActions := 0
	for _, row := range tables.actionRows {
		for ordinal := 0; ordinal < row.Len(); ordinal++ {
			if row.At(ordinal).Type == core.ActionReduce {
				reduceActions++
			}
		}
	}
	if got := unsafe.Sizeof(diagnosticParserCoreGenericCell{}); got > 64 {
		t.Fatalf("classified dispatch cell size=%d, want <=64", got)
	}
	if reduceActions != 545 || len(tables.reductionPlans) != 132 || tables.reductionPlanStride != 10 || len(tables.reductionPlanIndex) != 1110 || unsafe.Sizeof(tables.reductionPlanIndex[0])*uintptr(len(tables.reductionPlanIndex)) != 2220 {
		t.Fatalf("authenticated reduction plan census actions=%d plans=%d stride=%d index=%d/%d bytes", reduceActions, len(tables.reductionPlans), tables.reductionPlanStride, len(tables.reductionPlanIndex), unsafe.Sizeof(tables.reductionPlanIndex[0])*uintptr(len(tables.reductionPlanIndex)))
	}
}
