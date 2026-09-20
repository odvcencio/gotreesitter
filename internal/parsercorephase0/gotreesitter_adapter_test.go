package parsercorephase0_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// gotreesitterTableAdapter is a frozen test-only oracle adapter. The future
// tagged root driver must use the root parser's canonical table helpers rather
// than copying this raw decoder.
type gotreesitterTableAdapter struct{ language *gts.Language }

func adaptActions(actions []gts.ParseAction) []core.Action {
	out := make([]core.Action, len(actions))
	for i, action := range actions {
		out[i] = core.Action{
			Type: core.ActionType(action.Type), State: core.StateID(action.State), Symbol: core.Symbol(action.Symbol),
			ChildCount: action.ChildCount, DynamicPrecedence: action.DynamicPrecedence,
			ProductionID: action.ProductionID, Extra: action.Extra,
			ExtraChain: action.ExtraChain, Repetition: action.Repetition,
		}
	}
	return out
}

func (a gotreesitterTableAdapter) Actions(state core.StateID, symbol core.Symbol) (core.ActionRow, error) {
	index, err := a.lookup(state, symbol)
	if err != nil || index == 0 {
		return core.ActionRow{}, err
	}
	if int(index) >= len(a.language.ParseActions) {
		return core.ActionRow{}, errors.New("test adapter: action index out of range")
	}
	return core.NewActionRow(adaptActions(a.language.ParseActions[index].Actions), a.language.ParseActions[index].Reusable), nil
}

func actionRowValues(row core.ActionRow) []core.Action {
	out := make([]core.Action, row.Len())
	for index := range out {
		out[index] = row.At(index)
	}
	return out
}

func (a gotreesitterTableAdapter) Goto(state core.StateID, symbol core.Symbol) (core.StateID, error) {
	lang := a.language
	if lang.TokenCount > 0 && uint32(symbol) >= lang.TokenCount {
		if target := lang.LargeStateGotos[uint64(state)<<32|uint64(symbol)]; target != 0 {
			return core.StateID(target), nil
		}
	}
	raw, err := a.lookup(state, symbol)
	if err != nil || raw == 0 {
		return 0, err
	}
	if lang.TokenCount > 0 && uint32(symbol) >= lang.TokenCount && lang.StateCount > 0 && lang.InitialState > 0 {
		return core.StateID(raw), nil
	}
	if int(raw) >= len(lang.ParseActions) || len(lang.ParseActions[raw].Actions) == 0 || lang.ParseActions[raw].Actions[0].Type != gts.ParseActionShift {
		return 0, errors.New("test adapter: invalid hand-built goto")
	}
	return core.StateID(lang.ParseActions[raw].Actions[0].State), nil
}

func (a gotreesitterTableAdapter) ProductionFields(productionID uint16, _ int) ([]core.FieldMapEntry, error) {
	lang := a.language
	if int(productionID) >= len(lang.FieldMapSlices) {
		return nil, nil
	}
	span := lang.FieldMapSlices[productionID]
	start, end := int(span[0]), int(span[0])+int(span[1])
	if start > end || end > len(lang.FieldMapEntries) {
		return nil, errors.New("test adapter: invalid field span")
	}
	out := make([]core.FieldMapEntry, end-start)
	for i, field := range lang.FieldMapEntries[start:end] {
		out[i] = core.FieldMapEntry{FieldID: core.FieldID(field.FieldID), ChildIndex: field.ChildIndex, Inherited: field.Inherited}
	}
	return out, nil
}

func (a gotreesitterTableAdapter) ProductionAliases(productionID uint16, childCount int) ([]core.Symbol, error) {
	lang := a.language
	if int(productionID) >= len(lang.AliasSequences) || childCount <= 0 || len(lang.AliasSequences[productionID]) == 0 {
		return nil, nil
	}
	aliases := make([]core.Symbol, childCount)
	for i, alias := range lang.AliasSequences[productionID] {
		if i >= childCount {
			break
		}
		aliases[i] = core.Symbol(alias)
	}
	for _, alias := range aliases {
		if alias != 0 {
			return aliases, nil
		}
	}
	return nil, nil
}

func (a gotreesitterTableAdapter) lookup(state core.StateID, symbol core.Symbol) (uint16, error) {
	lang := a.language
	denseLimit := int(lang.LargeStateCount)
	if denseLimit == 0 {
		denseLimit = len(lang.ParseTable)
	}
	if int(state) < denseLimit {
		if int(state) >= len(lang.ParseTable) || int(symbol) >= len(lang.ParseTable[state]) {
			return 0, nil
		}
		return lang.ParseTable[state][symbol], nil
	}
	index := int(state) - int(lang.LargeStateCount)
	if index < 0 || index >= len(lang.SmallParseTableMap) {
		return 0, nil
	}
	position := int(lang.SmallParseTableMap[index])
	if position >= len(lang.SmallParseTable) {
		return 0, errors.New("test adapter: sparse offset out of range")
	}
	groups := int(lang.SmallParseTable[position])
	position++
	for group := 0; group < groups; group++ {
		if position+1 >= len(lang.SmallParseTable) {
			return 0, errors.New("test adapter: truncated sparse group")
		}
		value, count := lang.SmallParseTable[position], int(lang.SmallParseTable[position+1])
		position += 2
		if position+count > len(lang.SmallParseTable) {
			return 0, errors.New("test adapter: truncated sparse symbols")
		}
		for _, candidate := range lang.SmallParseTable[position : position+count] {
			if candidate == uint16(symbol) {
				return value, nil
			}
		}
		position += count
	}
	return 0, nil
}

// TestRealGoTableAdapterPreservesPinnedProperties pins a live shift/reduce
// and reduce/reduce conflict cell (state 20, the identifier vs.
// `_simple_type`/`_expression` ambiguity on "." and "(") straight from the
// shipped Go blob, so this test-only adapter's dense/sparse cell decoding
// stays honest against the real production table, not a synthetic one.
//
// "Sparse identity" is `lang.LargeStateCount` (the count of states at the
// front of the table dense-encoded in `ParseTable`; states at or above it
// live in the sparse `SmallParseTable`/`SmallParseTableMap` encoding) plus
// `lang.SmallParseTableMap[18]`, state 20's byte offset into
// `SmallParseTable` (state 20 - LargeStateCount(2) == sparse index 18). The
// offset is a position, not a semantic property: it moves whenever any
// state before 20 gains or loses an action, so every blob regeneration is
// expected to move it. `LargeStateCount`, state 20 itself (by construction
// state numbers 0..StateCount-1 are stable positions; only what a given
// number denotes can drift), and the (20,4)/(20,6) cell indices are the
// properties actually worth pinning, and did not move here.
//
// Re-anchored 2026-09-20 against grammars/grammar_blobs/go.bin SHA-256
// df63fc35604c4e4e7a484abde9eb2110b61640045601c23991723f323a48310d
// (cmd/grammargen, no -lr-split; see docs/grammar-ownership.md). Offset
// 814 -> 790 and conflict shift target state 194 -> 190 both come from the
// 2026-07-20 DFA-minimization fix and later grammargen fixes (field-map
// dedup, supertype hidden-choice collapse) shrinking the table (1511 -> 1507
// states) ahead of state 20. Symbols "." (4), "(" (6), `_simple_type` (121),
// and `_expression` (171) keep their IDs (verified against the previous
// shipped blob by name, not just by number); production 44 keeps its ID
// too. Only `type_identifier` renumbers, 229 -> 225, because the table
// dropped 4 symbols overall (233 -> 229) ahead of it.
func TestRealGoTableAdapterPreservesPinnedProperties(t *testing.T) {
	lang := grammars.GoLanguage()
	adapter := gotreesitterTableAdapter{language: lang}
	compact, err := core.New(adapter, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	wantConflict := []core.Action{{Type: core.ActionReduce, Symbol: 171, ChildCount: 1}, {Type: core.ActionShift, State: 190}}
	if lang.LargeStateCount != 2 || lang.SmallParseTableMap[18] != 790 {
		t.Fatalf("Go sparse identity drifted: large=%d offset=%d", lang.LargeStateCount, lang.SmallParseTableMap[18])
	}
	if index, _ := adapter.lookup(20, 4); index != 106 {
		t.Fatalf("Go cell (20,4) index=%d want=106", index)
	}
	if got, err := compact.Actions(20, 4); err != nil || !reflect.DeepEqual(actionRowValues(got), wantConflict) {
		t.Fatalf("Go conflict actions=%+v err=%v", got, err)
	}
	wantReduce := []core.Action{{Type: core.ActionReduce, Symbol: 121, ChildCount: 1, DynamicPrecedence: -1, ProductionID: 44}, {Type: core.ActionReduce, Symbol: 171, ChildCount: 1}}
	if index, _ := adapter.lookup(20, 6); index != 107 {
		t.Fatalf("Go cell (20,6) index=%d want=107", index)
	}
	if got, err := compact.Actions(20, 6); err != nil || !reflect.DeepEqual(actionRowValues(got), wantReduce) {
		t.Fatalf("Go reduction actions=%+v err=%v", got, err)
	}
	if state, err := adapter.Goto(1, 121); err != nil || state != 101 {
		t.Fatalf("Go goto (1,121)=%d err=%v want=101", state, err)
	}
	fields, err := adapter.ProductionFields(44, 1)
	if err != nil || len(fields) != 0 {
		t.Fatalf("Go production 44 fields=%v err=%v want empty", fields, err)
	}
	aliases, err := adapter.ProductionAliases(44, 1)
	if err != nil || !slices.Equal(aliases, []core.Symbol{225}) {
		t.Fatalf("Go production 44 aliases=%v err=%v want [225]", aliases, err)
	}
	if aliases, err := adapter.ProductionAliases(0, 1); err != nil || aliases != nil {
		t.Fatalf("Go nil alias row=%v err=%v", aliases, err)
	}
	if aliases, err := adapter.ProductionAliases(44, 2); err != nil || !slices.Equal(aliases, []core.Symbol{225, 0}) {
		t.Fatalf("Go short alias row=%v err=%v want [225 0]", aliases, err)
	}
}
