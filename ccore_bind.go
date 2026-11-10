package gotreesitter

import (
	"errors"
	"fmt"
	"sync"

	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
)

// Adapter between this package and internal/ccore. It exports nothing: it
// registers itself with internal/ccore/bind in init, and the equality tests in
// cgo_harness ask bind for a parser. No parse entry point of this package uses
// it. A language the core cannot serve is refused with the reason: here for a
// table with no blob identity, and in ccore.NewParser (internal/ccore/validate.go)
// for the initial state, the state, token and symbol counts, the lex modes and
// the keyword capture symbol.

// Tables for internal/ccore, built once per *Language and cached in a
// package-level map (no field in Language). Slices of basic types are shared
// with the Language; the tables whose element type is a root struct (actions,
// lex modes, metadata, alias sequences, reserved words) are converted once.
// The map keeps every language it has seen alive.

var (
	ccoreTablesMu    sync.Mutex
	ccoreTablesCache = map[*Language]*ccore.Tables{}
)

// The action kinds of internal/ccore must equal the root's.
var (
	_ [ccore.ActionShift - uint8(ParseActionShift)]struct{}
	_ [uint8(ParseActionShift) - ccore.ActionShift]struct{}
	_ [ccore.ActionReduce - uint8(ParseActionReduce)]struct{}
	_ [uint8(ParseActionReduce) - ccore.ActionReduce]struct{}
	_ [ccore.ActionAccept - uint8(ParseActionAccept)]struct{}
	_ [uint8(ParseActionAccept) - ccore.ActionAccept]struct{}
	_ [ccore.ActionRecover - uint8(ParseActionRecover)]struct{}
	_ [uint8(ParseActionRecover) - ccore.ActionRecover]struct{}
)

func ccoreTablesFor(lang *Language) *ccore.Tables {
	ccoreTablesMu.Lock()
	defer ccoreTablesMu.Unlock()
	if t, ok := ccoreTablesCache[lang]; ok {
		return t
	}
	t := buildCcoreTables(lang)
	ccoreTablesCache[lang] = t
	return t
}

// The ASCII table markers of internal/ccore must equal the root's.
var (
	_ [lexAsciiNoMatch - ccore.LexAsciiNoMatch]struct{}
	_ [ccore.LexAsciiNoMatch - lexAsciiNoMatch]struct{}
	_ [lexAsciiSkipBit - ccore.LexAsciiSkipBit]struct{}
	_ [ccore.LexAsciiSkipBit - lexAsciiSkipBit]struct{}
)

// ccoreLexStates converts the root's lex states once per language.
func ccoreLexStates(in []LexState) []ccore.LexDFAState {
	out := make([]ccore.LexDFAState, len(in))
	for i := range in {
		st := &in[i]
		o := ccore.LexDFAState{
			AcceptToken: ccore.Symbol(st.AcceptToken), AcceptPriority: st.AcceptPriority,
			Skip: st.Skip, AcceptEOF: st.AcceptEOF, Default: int32(st.Default), EOF: int32(st.EOF),
		}
		if len(st.Transitions) != 0 {
			o.Trans = make([]ccore.LexTrans, len(st.Transitions))
			for j, tr := range st.Transitions {
				o.Trans[j] = ccore.LexTrans{Lo: tr.Lo, Hi: tr.Hi, Next: int32(tr.NextState), Skip: tr.Skip}
			}
		}
		out[i] = o
	}
	return out
}

func buildCcoreTables(lang *Language) *ccore.Tables {
	p := NewParser(lang)
	t := &ccore.Tables{
		TokenCount:             lang.TokenCount,
		StateCount:             lang.StateCount,
		InitialState:           uint32(lang.InitialState),
		DenseLimit:             p.denseLimit,
		SmallBase:              p.smallBase,
		Dense:                  lang.ParseTable,
		SmallMap:               lang.SmallParseTableMap,
		Small:                  lang.SmallParseTable,
		SmallTokenLookup:       p.smallTokenLookup,
		Names:                  lang.SymbolNames,
		KeywordCapture:         ccore.Symbol(lang.KeywordCaptureToken),
		MaxReservedWordSetSize: lang.MaxReservedWordSetSize,
		ExternalLexStates:      lang.ExternalLexStates,
	}
	t.LexStates = ccoreLexStates(lang.LexStates)
	t.LexAscii = lang.LexAsciiTable()
	if len(lang.KeywordLexStates) != 0 {
		t.KeywordLexStates = ccoreLexStates(lang.KeywordLexStates)
		t.KeywordLexAscii = lang.KeywordLexAsciiTable()
	}
	if len(p.smallLookup) > 0 {
		t.SmallLookup = make([][]ccore.SmallPair, len(p.smallLookup))
		for i, pairs := range p.smallLookup {
			if len(pairs) == 0 {
				continue
			}
			row := make([]ccore.SmallPair, len(pairs))
			for j, pr := range pairs {
				row[j] = ccore.SmallPair{Sym: pr.sym, Val: pr.val}
			}
			t.SmallLookup[i] = row
		}
	}
	if len(lang.LargeStateGotos) > 0 {
		t.LargeStateGotos = make(map[uint64]uint32, len(lang.LargeStateGotos))
		for k, v := range lang.LargeStateGotos {
			t.LargeStateGotos[k] = uint32(v)
		}
	}
	total := 0
	for i := range lang.ParseActions {
		total += len(lang.ParseActions[i].Actions)
	}
	flat := make([]ccore.Action, 0, total)
	t.Actions = make([]ccore.ActionEntry, len(lang.ParseActions))
	for i := range lang.ParseActions {
		e := &lang.ParseActions[i]
		start := len(flat)
		for _, a := range e.Actions {
			flat = append(flat, ccore.Action{
				Type: uint8(a.Type), State: uint32(a.State), Symbol: ccore.Symbol(a.Symbol),
				ChildCount: a.ChildCount, DynamicPrecedence: a.DynamicPrecedence,
				ProductionID: a.ProductionID, Extra: a.Extra, Repetition: a.Repetition,
			})
		}
		t.Actions[i] = ccore.ActionEntry{Reusable: e.Reusable, Actions: flat[start:len(flat):len(flat)]}
	}
	// The after-whitespace lex states are root-only fields: C's TSLexerMode
	// (runtime 0.27.1) has none, and the core never reads them. They are copied
	// so that the tables file the golden test writes (internal/ccore/testdata/
	// tables) stays the same bytes. They make the core's mode equality
	// (canReuseFirstLeaf compares whole modes) stricter than C's only for two
	// modes that differ in these fields alone.
	t.LexModes = make([]ccore.LexMode, len(lang.LexModes))
	for i, m := range lang.LexModes {
		t.LexModes[i] = ccore.LexMode{
			LexState: m.LexState, ExternalLexState: m.ExternalLexState, ReservedWordSetID: m.ReservedWordSetID,
			AfterWhitespaceLexState: m.AfterWhitespaceLexState, LexStateID: m.LexStateID,
			AfterWhitespaceLexStateID: m.AfterWhitespaceLexStateID, Index: m.LexStateIndex(),
		}
	}
	t.Meta = make([]ccore.SymbolMeta, len(lang.SymbolMetadata))
	for i, m := range lang.SymbolMetadata {
		t.Meta[i] = ccore.SymbolMeta{Visible: m.Visible, Named: m.Named}
	}
	t.Alias = make([][]ccore.Symbol, len(lang.AliasSequences))
	for i, seq := range lang.AliasSequences {
		if seq == nil {
			continue
		}
		row := make([]ccore.Symbol, len(seq))
		for j, s := range seq {
			row[j] = ccore.Symbol(s)
		}
		t.Alias[i] = row
	}
	t.ReservedWords = make([]ccore.Symbol, len(lang.ReservedWords))
	for i, s := range lang.ReservedWords {
		t.ReservedWords[i] = ccore.Symbol(s)
	}
	t.ExternalSymbols = make([]ccore.Symbol, len(lang.ExternalSymbols))
	for i, s := range lang.ExternalSymbols {
		t.ExternalSymbols[i] = ccore.Symbol(s)
	}
	return t
}

// ccoreConfig returns the ccore configuration of a language. Each call gets
// its own scanner hook, so that parsers do not share scanner state.
func ccoreConfig(lang *Language) ccore.Config {
	cfg := ccore.Config{Tables: ccoreTablesFor(lang)}
	if lang.ExternalScanner != nil {
		cfg.Scanner = (&ccoreExtScanner{sc: lang.ExternalScanner}).hook()
	}
	return cfg
}

// ccoreBindConfig is bind.Adapter.Config.
func ccoreBindConfig(lang any) (ccore.Config, error) {
	l, ok := lang.(*Language)
	if !ok || l == nil {
		return ccore.Config{}, fmt.Errorf("ccore: want a non-nil *Language, got %T", lang)
	}
	if _, ok := l.GrammarBlobSHA256(); !ok {
		return ccore.Config{}, errors.New("ccore: language has no blob identity (a hand-built or generated table)")
	}
	return ccoreConfig(l), nil
}

// ccoreBindCheckTables is bind.Adapter.CheckTables: it compares the ccore
// Tables of lang with the root's own lookups for every state and symbol. It
// returns the number of pairs checked and the number that differ (action index
// or goto).
func ccoreBindCheckTables(lang any) (checked, differ int, err error) {
	l, ok := lang.(*Language)
	if !ok || l == nil {
		return 0, 0, fmt.Errorf("ccore: want a non-nil *Language, got %T", lang)
	}
	t := ccoreTablesFor(l)
	p := NewParser(l)
	states := int(l.StateCount)
	if states == 0 {
		states = len(l.ParseTable) + len(l.SmallParseTableMap)
	}
	for s := 0; s < states; s++ {
		for sym := 0; sym < int(l.SymbolCount); sym++ {
			checked++
			if t.ActionIndex(uint32(s), ccore.Symbol(sym)) != p.lookupActionIndex(StateID(s), Symbol(sym)) ||
				t.Goto(uint32(s), ccore.Symbol(sym)) != uint32(p.lookupGoto(StateID(s), Symbol(sym))) {
				differ++
			}
		}
	}
	return checked, differ, nil
}

func init() {
	bind.Register(bind.Adapter{Config: ccoreBindConfig, CheckTables: ccoreBindCheckTables})
}
