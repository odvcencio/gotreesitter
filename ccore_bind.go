package gotreesitter

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"reflect"
	"sort"
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

// The mutex protects language-owned caches.
// It does not retain languages after their callers release them.
var ccoreTablesMu sync.Mutex

type ccoreLanguageTables struct {
	owner          *Language
	base           *ccore.Tables
	nodes          *ccore.NodeTables
	scanner        ExternalScanner
	scannerType    reflect.Type
	scannerKey     []byte
	scannerPresent bool
	scannerError   string
}

// ccoreLanguageTablesForLocked must be called with ccoreTablesMu held.
func ccoreLanguageTablesForLocked(lang *Language) *ccoreLanguageTables {
	var scanner ExternalScanner
	var scannerType reflect.Type
	var key []byte
	var reason string
	present := lang.ExternalScanner != nil
	if present {
		scanner, scannerType, key, reason = ccoreScannerSnapshot(lang.ExternalScanner)
	}
	c := lang.ccoreTables
	if c == nil || c.owner != lang || c.scannerPresent != present || c.scannerType != scannerType || c.scannerError != reason || !bytes.Equal(c.scannerKey, key) {
		c = &ccoreLanguageTables{owner: lang, base: buildCcoreTables(lang), scanner: scanner, scannerType: scannerType, scannerKey: key, scannerPresent: present, scannerError: reason}
		lang.ccoreTables = c
	}
	return c
}

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
	return ccoreLanguageTablesForLocked(lang).base
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

// ccoreNodeTablesFor builds, once per language, the field maps and supertype
// bits that the node readers of internal/ccore use.
func ccoreNodeTablesFor(lang *Language) *ccore.NodeTables {
	ccoreTablesMu.Lock()
	defer ccoreTablesMu.Unlock()
	c := ccoreLanguageTablesForLocked(lang)
	return ccoreNodeTablesForLocked(lang, c)
}

func ccoreNodeTablesForLocked(lang *Language, c *ccoreLanguageTables) *ccore.NodeTables {
	if c.nodes != nil {
		return c.nodes
	}
	t := &ccore.NodeTables{Base: c.base, SymbolCount: lang.SymbolCount, FieldSlices: append([][2]uint16(nil), lang.FieldMapSlices...)}
	t.FieldEntries = make([]ccore.FieldEntry, len(lang.FieldMapEntries))
	for i, e := range lang.FieldMapEntries {
		t.FieldEntries[i] = ccore.FieldEntry{FieldID: uint16(e.FieldID), ChildIndex: e.ChildIndex, Inherited: e.Inherited}
	}
	t.SuperBit = make([]uint32, len(lang.SymbolNames))
	for i := range t.SuperBit {
		t.SuperBit[i] = lang.supertypeBit(Symbol(i))
	}
	c.nodes = t
	return t
}

// ccoreConfig returns the ccore configuration of a language. Each call gets
// its own scanner hook, so that parsers do not share scanner state.
func ccoreConfig(lang *Language) (ccore.Config, error) {
	ccoreTablesMu.Lock()
	c := ccoreLanguageTablesForLocked(lang)
	cfg := ccore.Config{Tables: c.base, Nodes: ccoreNodeTablesForLocked(lang, c)}
	scanner, reason := c.scanner, c.scannerError
	ccoreTablesMu.Unlock()
	if reason != "" {
		return ccore.Config{}, errors.New(reason)
	}
	if scanner != nil {
		a, err := newCcoreExtScanner(scanner)
		if err != nil {
			return ccore.Config{}, err
		}
		cfg.Scanner = a.hook()
	}
	return cfg, nil
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
	return ccoreConfig(l)
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
	bind.Register(bind.Adapter{Config: ccoreBindConfig, ConfigHash: ccoreBindConfigHash, CheckTables: ccoreBindCheckTables})
}

type cfgW struct {
	h   hash.Hash
	buf [8]byte
}

func (w *cfgW) u(v uint64) {
	binary.LittleEndian.PutUint64(w.buf[:], v)
	w.h.Write(w.buf[:])
}
func (w *cfgW) b(v bool) {
	if v {
		w.u(1)
	} else {
		w.u(0)
	}
}
func (w *cfgW) s(v string)   { w.u(uint64(len(v))); w.h.Write([]byte(v)) }
func (w *cfgW) tag(v string) { w.s(v) }

// ccoreConfigHash hashes the language tables and the registered scanner chain.
// Integers are little-endian, slices have lengths, and map keys are sorted.
func ccoreConfigHash(l *Language) ([32]byte, error) {
	w := &cfgW{h: sha256.New()}
	w.tag("counts")
	for _, v := range []uint32{l.SymbolCount, l.TokenCount, l.ExternalTokenCount, l.StateCount, l.LargeStateCount, l.FieldCount, l.ProductionIDCount, uint32(l.InitialState), uint32(l.KeywordCaptureToken), uint32(l.MaxReservedWordSetSize), uint32(l.LayoutFallbackLexState)} {
		w.u(uint64(v))
	}
	w.b(l.HasLayoutFallbackLexState)
	w.tag("names")
	w.u(uint64(len(l.SymbolNames)))
	for _, n := range l.SymbolNames {
		w.s(n)
	}
	w.tag("metadata")
	w.u(uint64(len(l.SymbolMetadata)))
	for _, m := range l.SymbolMetadata {
		w.s(m.Name)
		w.b(m.Visible)
		w.b(m.Named)
		w.b(m.Supertype)
		w.b(m.GeneratedRepeatAux)
	}
	w.tag("dense")
	w.u(uint64(len(l.ParseTable)))
	for _, row := range l.ParseTable {
		w.u(uint64(len(row)))
		for _, v := range row {
			w.u(uint64(v))
		}
	}
	w.tag("small")
	w.u(uint64(len(l.SmallParseTable)))
	for _, v := range l.SmallParseTable {
		w.u(uint64(v))
	}
	w.u(uint64(len(l.SmallParseTableMap)))
	for _, v := range l.SmallParseTableMap {
		w.u(uint64(v))
	}
	w.tag("actions")
	w.u(uint64(len(l.ParseActions)))
	for _, e := range l.ParseActions {
		w.b(e.Reusable)
		w.u(uint64(len(e.Actions)))
		for _, a := range e.Actions {
			w.u(uint64(a.Type))
			w.u(uint64(a.State))
			w.u(uint64(a.Symbol))
			w.u(uint64(a.ChildCount))
			w.u(uint64(uint16(a.DynamicPrecedence)))
			w.u(uint64(a.ProductionID))
			w.b(a.Extra)
			w.b(a.ExtraChain)
			w.b(a.Repetition)
		}
	}
	w.tag("gotos")
	keys := make([]uint64, 0, len(l.LargeStateGotos))
	for k := range l.LargeStateGotos {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	w.u(uint64(len(keys)))
	for _, k := range keys {
		w.u(k)
		w.u(uint64(l.LargeStateGotos[k]))
	}
	w.tag("lexmodes")
	w.u(uint64(len(l.LexModes)))
	for _, m := range l.LexModes {
		w.u(uint64(m.LexState))
		w.u(uint64(m.ExternalLexState))
		w.u(uint64(m.ReservedWordSetID))
		w.u(uint64(m.AfterWhitespaceLexState))
		w.u(uint64(m.LexStateID))
		w.u(uint64(m.AfterWhitespaceLexStateID))
	}
	for pass, states := range [][]LexState{l.LexStates, l.KeywordLexStates} {
		w.tag("lexstates")
		w.u(uint64(pass))
		w.u(uint64(len(states)))
		for _, st := range states {
			w.u(uint64(st.AcceptToken))
			w.u(uint64(uint16(st.AcceptPriority)))
			w.b(st.Skip)
			w.b(st.AcceptEOF)
			w.u(uint64(int64(st.Default)))
			w.u(uint64(int64(st.EOF)))
			w.u(uint64(len(st.Transitions)))
			for _, t := range st.Transitions {
				w.u(uint64(t.Lo))
				w.u(uint64(t.Hi))
				w.u(uint64(int64(t.NextState)))
				w.b(t.Skip)
			}
		}
	}
	w.tag("fields")
	w.u(uint64(len(l.FieldMapSlices)))
	for _, sl := range l.FieldMapSlices {
		w.u(uint64(sl[0]))
		w.u(uint64(sl[1]))
	}
	w.u(uint64(len(l.FieldMapEntries)))
	for _, e := range l.FieldMapEntries {
		w.u(uint64(e.FieldID))
		w.u(uint64(e.ChildIndex))
		w.b(e.Inherited)
	}
	w.u(uint64(len(l.FieldNames)))
	for _, n := range l.FieldNames {
		w.s(n)
	}
	w.tag("alias")
	w.u(uint64(len(l.AliasSequences)))
	for _, seq := range l.AliasSequences {
		w.b(seq == nil)
		w.u(uint64(len(seq)))
		for _, s := range seq {
			w.u(uint64(s))
		}
	}
	w.tag("symbols")
	for _, v := range [][]Symbol{l.ReservedWords, l.SupertypeSymbols, l.SupertypeMapEntries, l.ExternalSymbols} {
		w.u(uint64(len(v)))
		for _, s := range v {
			w.u(uint64(s))
		}
	}
	w.u(uint64(len(l.SupertypeMapSlices)))
	for _, sl := range l.SupertypeMapSlices {
		w.u(uint64(sl[0]))
		w.u(uint64(sl[1]))
	}
	w.tag("external")
	w.u(uint64(len(l.ExternalLexStates)))
	for _, row := range l.ExternalLexStates {
		w.u(uint64(len(row)))
		for _, v := range row {
			w.b(v)
		}
	}
	for _, v := range [][]bool{l.ImmediateTokens, l.ZeroWidthTokens} {
		w.tag("flags")
		w.u(uint64(len(v)))
		for _, x := range v {
			w.b(x)
		}
	}
	// The lookup rows the core walks for small states are derived by the parser
	// setup of the root package, which reads more of the language than the fields
	// above. They are hashed as built, so the hash covers what the core reads and
	// not only the inputs listed here.
	t := ccoreTablesFor(l)
	w.tag("derived")
	w.u(uint64(t.DenseLimit))
	w.u(uint64(t.SmallBase))
	w.u(uint64(len(t.SmallTokenLookup)))
	for _, row := range t.SmallTokenLookup {
		w.u(uint64(len(row)))
		for _, v := range row {
			w.u(uint64(v))
		}
	}
	w.u(uint64(len(t.SmallLookup)))
	for _, row := range t.SmallLookup {
		w.u(uint64(len(row)))
		for _, pr := range row {
			w.u(uint64(pr.Sym))
			w.u(uint64(pr.Val))
		}
	}
	w.tag("scanner")
	var scannerType string
	var key []byte
	if l.ExternalScanner != nil {
		_, typ, snapshot, reason := ccoreScannerSnapshot(l.ExternalScanner)
		if reason != "" {
			return [32]byte{}, errors.New(reason)
		}
		scannerType, key = typ.String(), snapshot
	}
	w.s(scannerType)
	w.u(uint64(len(key)))
	w.h.Write(key)
	var sum [32]byte
	copy(sum[:], w.h.Sum(nil))
	return sum, nil
}

// ccoreBindConfigHash is bind.Adapter.ConfigHash.
func ccoreBindConfigHash(lang any) ([32]byte, error) {
	l, ok := lang.(*Language)
	if !ok || l == nil {
		return [32]byte{}, fmt.Errorf("ccore: want a non-nil *Language, got %T", lang)
	}
	return ccoreConfigHash(l)
}
