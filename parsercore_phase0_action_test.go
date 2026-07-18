//go:build gts_parsercorephase0

package gotreesitter

import (
	"errors"
	"reflect"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestParserCoreActionConversionIsExhaustiveAndLossless(t *testing.T) {
	types := []struct {
		name string
		root ParseActionType
		core core.ActionType
	}{
		{name: "shift", root: ParseActionShift, core: core.ActionShift},
		{name: "reduce", root: ParseActionReduce, core: core.ActionReduce},
		{name: "accept", root: ParseActionAccept, core: core.ActionAccept},
		{name: "recover", root: ParseActionRecover, core: core.ActionRecover},
	}
	for _, test := range types {
		t.Run(test.name, func(t *testing.T) {
			input := ParseAction{
				Type: test.root, State: 1234, Symbol: 567, ChildCount: 8,
				DynamicPrecedence: -13, ProductionID: 901,
				Extra: true, ExtraChain: true, Repetition: true,
			}
			got, err := parserCoreAction(input)
			if err != nil {
				t.Fatal(err)
			}
			want := core.Action{
				Type: test.core, State: 1234, Symbol: 567, ChildCount: 8,
				DynamicPrecedence: -13, ProductionID: 901,
				Extra: true, ExtraChain: true, Repetition: true,
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("converted action = %+v, want %+v", got, want)
			}
		})
	}
	if _, err := parserCoreAction(ParseAction{Type: ParseActionType(255)}); err == nil {
		t.Fatal("invalid root action type was admitted")
	}
}

func TestParserCoreRootTablesCacheImmutableConvertedRows(t *testing.T) {
	sourceAction := ParseAction{
		Type: ParseActionShift, State: 17, Symbol: 23, ChildCount: 2,
		DynamicPrecedence: -3, ProductionID: 41, Extra: true,
	}
	lang := &Language{
		LargeStateCount: 1,
		ParseTable:      [][]uint16{{0, 1}},
		ParseActions:    []ParseActionEntry{{}, {Actions: []ParseAction{sourceAction}}},
	}
	tables, err := newParserCoreRootTables(NewParser(lang))
	if err != nil {
		t.Fatal(err)
	}
	first, err := tables.Actions(0, 1)
	if err != nil || first.Len() != 1 {
		t.Fatalf("first cached lookup row=%+v len=%d err=%v", first, first.Len(), err)
	}
	want, err := parserCoreAction(sourceAction)
	if err != nil {
		t.Fatal(err)
	}
	if got := first.At(0); got != want {
		t.Fatalf("cached action=%+v want=%+v", got, want)
	}

	// Neither source-table mutation nor mutation of the by-value result can
	// alter the private cached row observed by a later lookup.
	lang.ParseActions[1].Actions[0].State = 99
	copy := first.At(0)
	copy.State = 101
	second, err := tables.Actions(0, 1)
	if err != nil || second.Len() != 1 || second.At(0) != want {
		t.Fatalf("cached row mutated across lookups: action=%+v len=%d err=%v", second.At(0), second.Len(), err)
	}
	var measured core.ActionRow
	var measuredErr error
	if allocs := testing.AllocsPerRun(1000, func() {
		measured, measuredErr = tables.Actions(0, 1)
	}); allocs != 0 || measuredErr != nil || measured.Len() != 1 {
		t.Fatalf("cached lookup allocs=%v len=%d err=%v", allocs, measured.Len(), measuredErr)
	} else if measured.At(0) != want {
		t.Fatalf("cached lookup action=%+v want=%+v", measured.At(0), want)
	}
}

func TestParserCoreRootTablesRejectInvalidActionWhileCaching(t *testing.T) {
	lang := &Language{ParseActions: []ParseActionEntry{{}, {Actions: []ParseAction{{Type: ParseActionType(255)}}}}}
	if _, err := newParserCoreRootTables(NewParser(lang)); err == nil {
		t.Fatal("invalid decoded action was cached")
	}
	if _, err := newParserCoreRootTables(nil); err == nil {
		t.Fatal("nil parser was cached")
	}
}

func TestParserCoreRootlessTablesRemainUsableWithoutSelectedStorePolicy(t *testing.T) {
	lang := &Language{
		LargeStateCount: 1,
		ParseTable:      [][]uint16{{0}},
		ParseActions:    []ParseActionEntry{{}},
	}
	parser := NewParser(lang)
	if parser.hasRootSymbol {
		t.Fatal("fixture unexpectedly authenticated a root symbol")
	}
	tables, err := newParserCoreRootTables(parser)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := tables.SelectedStorePolicy()
	if err != nil || len(policy.Symbols) != 0 {
		t.Fatalf("rootless selected policy=%+v err=%v", policy, err)
	}
	compact, err := core.New(tables, core.Limits{})
	if err != nil {
		t.Fatalf("rootless compact core construction: %v", err)
	}
	if store, err := compact.BuildAuthenticatedSelectedStore([]core.SubtreeID{1}, nil, nil); err == nil || store != nil {
		t.Fatalf("rootless selected store=%v err=%v", store, err)
	}
}

func TestParserCoreSameLookaheadGuardsDeclineBeforeMutation(t *testing.T) {
	tests := []struct {
		name    string
		token   Token
		actions core.ActionRow
	}{
		{name: "no-lookahead", token: Token{NoLookahead: true}, actions: core.NewActionRow([]core.Action{{Type: core.ActionShift, State: 2}})},
		{name: "repetition", actions: core.NewActionRow([]core.Action{{Type: core.ActionShift, State: 2, Repetition: true}})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateDiagnosticParserCoreCell(test.token, test.actions)
			var decline *diagnosticParserCoreDecline
			if !errors.As(err, &decline) || decline.boundary != DiagnosticParserCoreRoute {
				t.Fatalf("guard error=%v, want unsupported-route decline", err)
			}
		})
	}
}
