package parsercorephase0

import "errors"

type tableCell struct {
	state  StateID
	symbol Symbol
}

type productionKey struct {
	productionID uint16
	childCount   int
}

type fakeTable struct {
	actions map[tableCell][]Action
	gotos   map[tableCell]StateID
	fields  map[uint16][]FieldMapEntry
	aliases map[productionKey][]Symbol
}

func (f *fakeTable) Actions(state StateID, symbol Symbol) (ActionRow, error) {
	if f == nil {
		return ActionRow{}, errors.New("fake table is nil")
	}
	return NewActionRow(f.actions[tableCell{state: state, symbol: symbol}]), nil
}

func actionRowValues(row ActionRow) []Action {
	out := make([]Action, row.Len())
	for index := range out {
		out[index] = row.At(index)
	}
	return out
}

func (f *fakeTable) Goto(state StateID, symbol Symbol) (StateID, error) {
	if f == nil {
		return 0, errors.New("fake table is nil")
	}
	return f.gotos[tableCell{state: state, symbol: symbol}], nil
}

func (f *fakeTable) ProductionFields(productionID uint16, _ int) ([]FieldMapEntry, error) {
	return f.fields[productionID], nil
}

func (f *fakeTable) ProductionAliases(productionID uint16, childCount int) ([]Symbol, error) {
	return f.aliases[productionKey{productionID: productionID, childCount: childCount}], nil
}
