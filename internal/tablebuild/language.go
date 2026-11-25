package tablebuild

import (
	"fmt"
	"math"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// This file converts a Layout into a *gotreesitter.Language the way
// cmd/ts2go converts the parser.c that the generator writes (BuildLanguage):
// the counts, symbol names and metadata, field names and field maps, alias
// sequences, the non-terminal alias map, the parse table, the small parse
// table, the parse actions (indexed by offset), the primary state ids and
// the supertype map. The lex modes, the lex states, the reserved words and
// the external scanner tables are not converted yet.

// Language returns the parse tables of the layout as a Language. It fails
// when a value does not fit the Language's field.
func (l *Layout) Language(name string) (*gotreesitter.Language, error) {
	lang := &gotreesitter.Language{
		Name:               name,
		LanguageVersion:    uint32(l.ABIVersion),
		SymbolCount:        uint32(l.SymbolCount),
		TokenCount:         uint32(l.TokenCount),
		ExternalTokenCount: uint32(l.ExternalTokenCount),
		StateCount:         uint32(l.StateCount),
		LargeStateCount:    uint32(l.LargeStateCount),
		FieldCount:         uint32(l.FieldCount),
		ProductionIDCount:  uint32(l.ProductionIDCount),
		InitialState:       1,
	}
	lang.SymbolNames = append([]string(nil), l.SymbolNames...)
	lang.SymbolMetadata = make([]gotreesitter.SymbolMetadata, len(l.SymbolMetadata))
	for i, m := range l.SymbolMetadata {
		lang.SymbolMetadata[i] = gotreesitter.SymbolMetadata{Name: l.SymbolNames[i], Visible: m.Visible, Named: m.Named, Supertype: m.Supertype}
	}
	if len(l.FieldNames) > 0 {
		lang.FieldNames = append([]string(nil), l.FieldNames...)
		lang.FieldMapSlices = append([][2]uint16(nil), l.FieldMapSlices...)
		lang.FieldMapEntries = make([]gotreesitter.FieldMapEntry, len(l.FieldMapEntries))
		for i, e := range l.FieldMapEntries {
			if e.ChildIndex > math.MaxUint8 {
				return nil, fmt.Errorf("tablebuild: field map entry %d: child index %d does not fit a Language", i, e.ChildIndex)
			}
			lang.FieldMapEntries[i] = gotreesitter.FieldMapEntry{FieldID: gotreesitter.FieldID(e.FieldID), ChildIndex: uint8(e.ChildIndex), Inherited: e.Inherited}
		}
	}
	// cmd/ts2go keeps the alias sequences only when one of them is not empty,
	// and the rows of the alias map only for symbols with aliases: the
	// public symbol, then the aliases.
	for _, row := range l.AliasSequences {
		if !allZero(row) {
			lang.AliasSequences = make([][]gotreesitter.Symbol, len(l.AliasSequences))
			for i, r := range l.AliasSequences {
				lang.AliasSequences[i] = languageSymbols(r)
			}
			break
		}
	}
	for i := 0; i+1 < len(l.NonTerminalAliasMap); {
		sym, count := int(l.NonTerminalAliasMap[i]), int(l.NonTerminalAliasMap[i+1])
		if lang.NonTerminalAliasMap == nil {
			lang.NonTerminalAliasMap = make([][]gotreesitter.Symbol, l.SymbolCount)
		}
		lang.NonTerminalAliasMap[sym] = languageSymbols(l.NonTerminalAliasMap[i+2 : i+2+count])
		i += 2 + count
	}
	if len(l.SupertypeSymbols) > 0 {
		lang.SupertypeSymbols = languageSymbols(l.SupertypeSymbols)
		lang.SupertypeMapSlices = append([][2]uint16(nil), l.SupertypeMapSlices...)
		lang.SupertypeMapEntries = languageSymbols(l.SupertypeMapEntries)
	}
	lang.ParseTable = make([][]uint16, len(l.ParseTable))
	for i, row := range l.ParseTable {
		lang.ParseTable[i] = append([]uint16(nil), row...)
	}
	if len(l.SmallParseTable) > 0 {
		lang.SmallParseTable = append([]uint16(nil), l.SmallParseTable...)
		lang.SmallParseTableMap = append([]uint32(nil), l.SmallParseTableMap...)
	}
	if n := len(l.ParseActions); n > 0 {
		lang.ParseActions = make([]gotreesitter.ParseActionEntry, l.ParseActions[n-1].Offset+1)
	}
	for _, e := range l.ParseActions {
		entry := gotreesitter.ParseActionEntry{Reusable: e.Reusable}
		if len(e.Actions) > 0 {
			entry.Actions = make([]gotreesitter.ParseAction, len(e.Actions))
		}
		for k, a := range e.Actions {
			var act gotreesitter.ParseAction
			switch a.Kind {
			case ActionShift:
				act = gotreesitter.ParseAction{Type: gotreesitter.ParseActionShift, State: gotreesitter.StateID(a.State), Repetition: a.IsRepetition}
			case ActionShiftExtra:
				act = gotreesitter.ParseAction{Type: gotreesitter.ParseActionShift, Extra: true}
			case ActionReduce:
				if a.ChildCount > math.MaxUint8 || a.DynamicPrecedence < math.MinInt16 || a.DynamicPrecedence > math.MaxInt16 {
					return nil, fmt.Errorf("tablebuild: reduce at action list offset %d: child count %d or dynamic precedence %d does not fit a Language", e.Offset, a.ChildCount, a.DynamicPrecedence)
				}
				act = gotreesitter.ParseAction{
					Type:              gotreesitter.ParseActionReduce,
					Symbol:            gotreesitter.Symbol(a.Symbol),
					ChildCount:        uint8(a.ChildCount),
					DynamicPrecedence: int16(a.DynamicPrecedence),
					ProductionID:      a.ProductionID,
				}
			case ActionAccept:
				act = gotreesitter.ParseAction{Type: gotreesitter.ParseActionAccept}
			case ActionRecover:
				act = gotreesitter.ParseAction{Type: gotreesitter.ParseActionRecover}
			}
			entry.Actions[k] = act
		}
		lang.ParseActions[e.Offset] = entry
	}
	lang.PrimaryStateIDs = make([]gotreesitter.StateID, len(l.PrimaryStateIDs))
	for i, p := range l.PrimaryStateIDs {
		lang.PrimaryStateIDs[i] = gotreesitter.StateID(p)
	}
	return lang, nil
}

func allZero(row []uint16) bool {
	for _, v := range row {
		if v != 0 {
			return false
		}
	}
	return true
}

func languageSymbols(ids []uint16) []gotreesitter.Symbol {
	out := make([]gotreesitter.Symbol, len(ids))
	for i, id := range ids {
		out[i] = gotreesitter.Symbol(id)
	}
	return out
}
