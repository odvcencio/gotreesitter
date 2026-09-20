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
	// reduceActions dropped from 335 to 331 on 2026-09-20 when
	// grammars/grammar_blobs/go.bin was regenerated with cmd/grammargen (no
	// -lr-split; see docs/grammar-ownership.md): later grammargen fixes
	// remove a few redundant reduce actions from the Go table.
	if reduceActions != 331 || len(tables.reductionPlans) != 154 || tables.reductionPlanStride != 10 || len(tables.reductionPlanIndex) != 1340 || unsafe.Sizeof(tables.reductionPlanIndex[0])*uintptr(len(tables.reductionPlanIndex)) != 2680 {
		t.Fatalf("authenticated reduction plan census actions=%d plans=%d stride=%d index=%d/%d bytes", reduceActions, len(tables.reductionPlans), tables.reductionPlanStride, len(tables.reductionPlanIndex), unsafe.Sizeof(tables.reductionPlanIndex[0])*uintptr(len(tables.reductionPlanIndex)))
	}
}
