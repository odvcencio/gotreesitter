//go:build cgo

package tree_sitter

/*
#include <stdint.h>
#include <tree_sitter/api.h>
uint32_t gts_query_table_size(const TSQuery *self, uint32_t table);
uint32_t gts_query_table_row(const TSQuery *self, uint32_t table, uint32_t index, uint32_t *out);
*/
import "C"

// QueryTable names one table of a compiled query that QueryTableRows reads.
// The row layouts are documented beside gts_query_table_row in src/query.c.
type QueryTable uint32

// The tables of a compiled query, as query.c stores them.
const (
	QueryTableSteps         QueryTable = 0
	QueryTablePatternMap    QueryTable = 1
	QueryTablePatterns      QueryTable = 2
	QueryTableStepOffsets   QueryTable = 3
	QueryTableNegatedFields QueryTable = 4
	QueryTableRepeatSymbols QueryTable = 5
)

// The flag bits in the last value of a step row.
const (
	QueryStepIsNamed                 = 1 << 0
	QueryStepIsImmediate             = 1 << 1
	QueryStepIsLastChild             = 1 << 2
	QueryStepIsPassThrough           = 1 << 3
	QueryStepIsDeadEnd               = 1 << 4
	QueryStepIsInsideAlternation     = 1 << 5
	QueryStepContainsCaptures        = 1 << 6
	QueryStepRootPatternGuaranteed   = 1 << 7
	QueryStepParentPatternGuaranteed = 1 << 8
	QueryStepIsMissing               = 1 << 9
	QueryStepAlternativeIsSkip       = 1 << 10
)

const queryTableRowWidth = 10

// QueryTableRows returns every row of one table of the compiled query. Each
// row holds the values that query.c stores, in the order that
// gts_query_table_row writes them. It reads the query and changes nothing.
func (q *Query) QueryTableRows(table QueryTable) [][]uint32 {
	n := uint32(C.gts_query_table_size(q._inner, C.uint32_t(table)))
	rows := make([][]uint32, 0, n)
	var buf [queryTableRowWidth]C.uint32_t
	for i := uint32(0); i < n; i++ {
		width := uint32(C.gts_query_table_row(q._inner, C.uint32_t(table), C.uint32_t(i), &buf[0]))
		row := make([]uint32, width)
		for j := range row {
			row[j] = uint32(buf[j])
		}
		rows = append(rows, row)
	}
	return rows
}
