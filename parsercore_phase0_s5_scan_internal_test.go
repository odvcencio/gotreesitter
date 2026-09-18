//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"slices"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

type s5RecordingTable struct {
	genericConflictTable
	visited []core.Symbol
}

func (table *s5RecordingTable) Actions(state core.StateID, symbol core.Symbol) (core.ActionRow, error) {
	table.visited = append(table.visited, symbol)
	return table.genericConflictTable.Actions(state, symbol)
}

func TestS5ReductionScanRange(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mode      diagnosticParserCoreS5ReductionMode
		tokens    uint32
		lookahead core.Symbol
		want      []core.Symbol
	}{
		{"any", diagnosticParserCoreS5AnyTerminal, 4, 0, []core.Symbol{1, 2, 3}},
		{"no_tokens", diagnosticParserCoreS5AnyTerminal, 0, 0, nil},
		{"end_only", diagnosticParserCoreS5AnyTerminal, 1, 0, nil},
		{"exact", diagnosticParserCoreS5ExactToken, 4, 2, []core.Symbol{2}},
		{"exact_end", diagnosticParserCoreS5ExactToken, 4, 0, []core.Symbol{0}},
		{"exact_max", diagnosticParserCoreS5ExactToken, 65535, 65535, []core.Symbol{65535}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			table := &s5RecordingTable{}
			compact, err := core.New(table, core.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			scheduler := &diagnosticParserCoreGenericScheduler{compact: compact,
				tokenSource: &dfaTokenSource{language: &Language{TokenCount: tc.tokens}}}
			_, _, err = scheduler.s5CollectReductionActions(5, tc.mode, tc.lookahead)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(table.visited, tc.want) {
				t.Fatalf("visited=%v, want %v", table.visited, tc.want)
			}
		})
	}
}

func TestS5ReductionScanPreservesFirstActionAndOrder(t *testing.T) {
	first := core.Action{Type: core.ActionReduce, Symbol: 9, ChildCount: 1, ProductionID: 11}
	second := core.Action{Type: core.ActionReduce, Symbol: 8, ChildCount: 2, ProductionID: 12}
	duplicate := first
	duplicate.ProductionID = 13
	table := &genericConflictTable{cells: map[genericConflictCell][]core.Action{
		{state: 5, symbol: 0}: {{Type: core.ActionRecover}},
		{state: 5, symbol: 1}: {{Type: core.ActionReduce}, first},
		{state: 5, symbol: 2}: {duplicate, second},
		{state: 5, symbol: 3}: {{Type: core.ActionShift, Extra: true}, {Type: core.ActionShift, Repetition: true}},
	}}
	compact, err := core.New(table, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	scheduler := &diagnosticParserCoreGenericScheduler{compact: compact,
		tokenSource: &dfaTokenSource{language: &Language{TokenCount: 4}}}
	actions, shift, err := scheduler.s5CollectReductionActions(5, diagnosticParserCoreS5AnyTerminal, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []diagnosticParserCoreS5ReductionAction{
		{symbol: 1, ordinal: 1, value: first},
		{symbol: 2, ordinal: 1, value: second},
	}
	if shift || !slices.Equal(actions, want) {
		t.Fatalf("shift=%v actions=%+v, want false/%+v", shift, actions, want)
	}
}

func BenchmarkS5ScalaReductionScan(b *testing.B) {
	language, err := LoadLanguage(package2ScalaBlob)
	if err != nil {
		b.Fatal(err)
	}
	tables, err := newParserCoreRootTables(NewParser(language))
	if err != nil {
		b.Fatal(err)
	}
	compact, err := core.New(tables, core.Limits{})
	if err != nil {
		b.Fatal(err)
	}
	scheduler := &diagnosticParserCoreGenericScheduler{compact: compact,
		tokenSource: &dfaTokenSource{language: language}}
	for _, tc := range []struct {
		name  string
		state core.StateID
	}{
		{"before_missing", 8336}, {"reduced_head", 9753},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, _, err := scheduler.s5CollectReductionActions(tc.state, diagnosticParserCoreS5AnyTerminal, 0); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
