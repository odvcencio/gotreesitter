package gotreesitter

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"io"
	"reflect"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/certified"
	"github.com/odvcencio/gotreesitter/internal/coresource"
	"github.com/odvcencio/gotreesitter/internal/sched"
)

// Adapter between this package and internal/ccore. It registers its configuration
// with internal/ccore/bind and serves certified parses through the public parser.
// A language the core cannot serve is refused with the reason: here for a
// table with no blob identity, and in ccore.NewParser (internal/ccore/validate.go)
// for the initial state, the state, token and symbol counts, the lex modes and
// the keyword capture symbol.

// The mutex protects language-owned caches.
// It does not retain languages after their callers release them.
var ccoreTablesMu sync.Mutex

type ccoreBinding struct{ _ byte }

type ccoreLanguageTables struct {
	token          *ccoreBinding
	configHash     [32]byte
	err            error
	rootOnly       []string
	conv           *ccoreConvTables
	pool           sync.Pool
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
		c.token = &ccoreBinding{}
		c.configHash, c.err = ccoreConfigHashWithTables(lang, c.base)
		if c.scannerError != "" {
			c.err = errors.New(c.scannerError)
		}
		c.rootOnly = ccoreRootOnlyFields(lang)
		c.conv = buildCcoreConvTables(lang)
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
func ccoreLexStates(in []LexState, endStates []bool) []ccore.LexDFAState {
	out := make([]ccore.LexDFAState, len(in))
	for i := range in {
		st := &in[i]
		o := ccore.LexDFAState{
			AcceptToken: ccore.Symbol(st.AcceptToken), AcceptPriority: st.AcceptPriority,
			Skip: st.Skip, AcceptEOF: st.AcceptEOF, Default: int32(st.Default), EOF: int32(st.EOF),
		}
		o.AcceptEnd = i < len(endStates) && endStates[i]
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
	t.LexStates = ccoreLexStates(lang.LexStates, lang.lexerEndStates)
	t.LexAscii = lang.LexAsciiTable()
	if len(lang.KeywordLexStates) != 0 {
		t.KeywordLexStates = ccoreLexStates(lang.KeywordLexStates, nil)
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
	ccoreViewsOn.Store(true)
	bind.Register(bind.Adapter{Config: ccoreBindConfig, ConfigHash: ccoreBindConfigHash, CheckTables: ccoreBindCheckTables, RouteCounters: ccoreRouteCounters, ForestCounters: ccoreForestRouteCounters, CompactRecords: ccoreCompactRecords, IncrFallbacks: ccoreRouteIncrFallbacks, RouteNanos: ccoreRouteNanos, RouteLimits: ccoreRouteLimits, RootOnlyFields: ccoreBindRootOnlyFields, ViewsOn: ccoreSetViewsOn, ViewCounters: ccoreViewCounters})
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
	return ccoreConfigHashWithTables(l, ccoreTablesFor(l))
}

func ccoreConfigHashWithTables(l *Language, t *ccore.Tables) ([32]byte, error) {
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
	var endStates bool
	for i, st := range t.LexStates {
		if st.AcceptEnd {
			if !endStates {
				w.tag("builtin-end")
				endStates = true
			}
			w.u(uint64(i))
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

// ccoreRootOnlyFields names the Language fields the core does not model and
// that this language sets. The core ignores them (the root's lexer reads
// ImmediateTokens, ZeroWidthTokens and the layout fallback state; no engine
// the core replaces reads the after-whitespace lex states through the core's
// tables). A language that sets one is certified only when the certification
// explicitly lists it as checked against pinned C (export_test.go).
func ccoreRootOnlyFields(l *Language) []string {
	var out []string
	anyTrue := func(v []bool) bool {
		for _, b := range v {
			if b {
				return true
			}
		}
		return false
	}
	if anyTrue(l.ImmediateTokens) {
		out = append(out, "ImmediateTokens")
	}
	if anyTrue(l.ZeroWidthTokens) {
		out = append(out, "ZeroWidthTokens")
	}
	if l.HasLayoutFallbackLexState {
		out = append(out, "LayoutFallbackLexState")
	}
	for _, m := range l.LexModes {
		if m.AfterWhitespaceLexState != 0 || m.AfterWhitespaceLexStateID != 0 {
			out = append(out, "AfterWhitespaceLexState")
			break
		}
	}
	return out
}

// ccoreConvTables is the conversion's private snapshot of the language data a
// served tree is built from, taken at the bind from the same values the
// configuration hash covers. The conversion reads nothing else of the language
// value, so a change to the caller's tables after the bind (an alias row, a
// field-map entry, a metadata flag) cannot change the structure of a tree that
// the core serves. Kind-name and field-name accessors (Node.Type, Node.FieldName
// and the like) still read the caller's language value when they are called:
// they return the names the caller's value has then, while the structure (which
// symbol, which alias, which field) is the snapshot's.
type ccoreConvTables struct {
	alias        [][]Symbol
	fieldSlices  [][2]uint16
	fieldEntries []FieldMapEntry
	named        []bool   // SymbolMetadata[sym].Named, for the symbol an alias names
	supertype    []uint32 // symbol -> supertype provenance bit (Language.supertypeBit)
}

func (c *ccoreConvTables) supertypeBit(sym Symbol) uint32 {
	if int(sym) >= len(c.supertype) {
		return 0
	}
	return c.supertype[sym]
}

func buildCcoreConvTables(lang *Language) *ccoreConvTables {
	c := &ccoreConvTables{
		alias:        make([][]Symbol, len(lang.AliasSequences)),
		fieldSlices:  append([][2]uint16(nil), lang.FieldMapSlices...),
		fieldEntries: append([]FieldMapEntry(nil), lang.FieldMapEntries...),
		named:        make([]bool, len(lang.SymbolMetadata)),
		supertype:    make([]uint32, len(lang.SymbolNames)),
	}
	for i, seq := range lang.AliasSequences {
		if seq != nil {
			c.alias[i] = append([]Symbol(nil), seq...)
		}
	}
	for i, m := range lang.SymbolMetadata {
		c.named[i] = m.Named
	}
	ordinal := 0
	for i := range c.supertype { // the rule of Language.buildSymbolMaps: one bit per supertype in symbol order
		sym := Symbol(i)
		if !((i < len(lang.SymbolMetadata) && lang.SymbolMetadata[i].Supertype) || slices.Contains(lang.SupertypeSymbols, sym)) {
			continue
		}
		if ordinal < 32 {
			c.supertype[i] = 1 << uint(ordinal)
		}
		ordinal++
	}
	return c
}

// The mode bits of a certificate. They equal internal/ccore/certgen.
const (
	ccoreModeFresh   = certified.Fresh
	ccoreModeRecover = certified.Recover
	ccoreModeScan    = certified.Scan
	ccoreModeIncr    = certified.Incr
	ccoreModeRanges  = certified.Ranges
)

// ccoreTestParserHook is test-only (nil in production): it is called with a
// core parser just before a parse (before true) and after it, when the parser
// goes back to the pool (before false), so a test can make one parse stop or
// panic and reset the parser afterwards.
var ccoreTestParserHook atomic.Pointer[func(cp *ccore.Parser, before bool)]

// ccoreUnset reports whether the core currently has no certificates.
func ccoreUnset() bool {
	return !certified.EverActive() || (len(certified.Blobs) == 0 && certified.Current() == nil)
}

// ccoreRouteEnabled keeps a per-parser opt-out while allowing a certified blob
// to use the core independently of the compact route's process-wide switch.
func (p *Parser) ccoreRouteEnabled() bool {
	if p == nil || p.admissionCandidateRoute == admissionRouteProductionForced {
		return false
	}
	if p.language != nil && p.language.ccoreBlobCertified() {
		return true
	}
	return p.admissionCandidateRouteEnabled()
}

// ccoreBlobCertified reports whether the language's blob is in the generated
// certificate list. The list does not change after start, so the answer is kept
// on the language and a later call is one atomic load.
func (l *Language) ccoreBlobCertified() bool {
	switch atomic.LoadUint32(&l.ccoreCertState) {
	case 1:
		return false
	case 2:
		return true
	}
	state := uint32(1)
	if len(certified.Blobs) != 0 {
		if sha, ok := l.GrammarBlobSHA256(); ok {
			if _, ok := certified.Blobs[sha]; ok {
				state = 2
			}
		}
	}
	atomic.StoreUint32(&l.ccoreCertState, state)
	return state == 2
}

// ccoreSource says what produces the tokens of a request: the lexer the core
// has itself (the built-in DFA), a scan provider that the certificate lists
// and that the request qualified (a table-backed provider, which the core calls
// in place of its automaton walk), or an ordinary stream, whose Next the core
// never calls.
type ccoreSource uint8

const (
	ccoreSourceStream   ccoreSource = iota // a caller's TokenSource: the present engine's
	ccoreSourceDFA                         // the built-in DFA
	ccoreSourceProvider                    // a qualified scan provider
)

func (p *Parser) ccoreEligible(usingProductionDFA bool) bool {
	if usingProductionDFA {
		return p.ccoreEligibleFor(ccoreSourceDFA)
	}
	return p.ccoreEligibleFor(ccoreSourceStream)
}

// ccoreEligibleFor is ccoreEligible for a kind of token producer. Only an
// ordinary stream carries the scheduler's token-source requirement, which the
// compact route keeps: a qualified provider is the core's own scan, so the
// core does not need the requirement discharged by anything but the
// qualification.
func (p *Parser) ccoreEligibleFor(kind ccoreSource) bool {
	return p.ccoreEligibleWithGrant(kind, 0)
}

// The forest binding discharges only the forest mode after checking its blob grant.
func (p *Parser) ccoreEligibleWithGrant(kind ccoreSource, granted sched.Mode) bool {
	if p == nil || p.admissionRouteSuppressed > 0 || !p.ccoreRouteEnabled() {
		return false
	}
	var modes sched.Mode
	if kind == ccoreSourceStream {
		modes = sched.TokenSource
	}
	modes |= p.schedImpliedModes(modes, nil)
	if kind == ccoreSourceDFA || kind == ccoreSourceProvider {
		// Both the built-in DFA and the qualified table provider can read
		// selected ranges. Admission checks the blob's range grant.
		modes &^= sched.IncludedRanges
		if len(p.included) != 0 {
			// Automatic forest dispatch declines selected ranges. The ordinary
			// core path still checks the matching range certificate.
			modes &^= sched.ForestRoute
		}
	}
	return sched.Supports(sched.Request{Modes: modes &^ granted})
}

// A forest grant authorizes native service for reparses of a compact-backed
// tree. The ordinary old-tree check still decides if its records can be reused.
func (p *Parser) ccoreEligibleForReparse(kind ccoreSource, old *Tree) bool {
	if p.ccoreEligibleFor(kind) {
		return true
	}
	if kind != ccoreSourceDFA || old == nil || old.ccoreStateOf() == nil || !p.ccoreEligibleWithGrant(kind, sched.ForestRoute) {
		return false
	}
	adm, _, ok := p.ccoreAdmitQuiet()
	return ok && adm.modes&(certified.Forest|certified.Incr) == (certified.Forest|certified.Incr)
}

// Decline reasons. Each is counted in ccoreDeclines.
const (
	ccoreDeclNotCertified = iota
	ccoreDeclNoBinding
	ccoreDeclModeMissing
	ccoreDeclRootOnlyField
	ccoreDeclIncludedRanges
	ccoreDeclObserver
	ccoreDeclWorkLimits
	ccoreDeclInternalParser
	ccoreDeclStopDeadline
	ccoreDeclStopCancelled
	ccoreDeclStopMemory
	ccoreDeclCoreStop
	ccoreDeclPanic
	ccoreDeclScannerChanged
	ccoreDeclDepthLimit
	ccoreDeclConfigChanged
	ccoreDeclScannerRefused
	ccoreDeclProviderNotGranted
	ccoreDeclCount
)

var ccoreDeclineNames = [ccoreDeclCount]string{
	"not_certified", "no_binding", "mode_missing", "root_only_field", "included_ranges", "observer",
	"work_limits", "internal_parser", "stop_deadline", "stop_cancelled", "stop_memory_budget", "core_stop", "panic",
	"scanner_changed", "depth_limit", "config_changed", "scanner_refused", "provider_not_granted",
}

var (
	ccoreDeclines    [ccoreDeclCount]atomic.Int64
	ccoreServedFresh atomic.Int64
	// Scans that a provider answered for the core, summed over the requests it served.
	ccoreProviderScans atomic.Int64
	// Nanoseconds spent in the core parse and in the conversion (read by the
	// timing test through export_test.go).
	ccoreParseNanos, ccoreConvertNanos, ccoreEditNanos, ccoreCheckNanos atomic.Int64
)

func ccoreDecline(reason int) { ccoreDeclines[reason].Add(1) }

func ccoreRouteCounters() (map[string]int64, int64, int64) {
	out := make(map[string]int64)
	for i := range ccoreDeclines {
		if v := ccoreDeclines[i].Load(); v != 0 {
			out[ccoreDeclineNames[i]] = v
		}
	}
	return out, ccoreServedFresh.Load(), ccoreServedIncr.Load()
}

func ccoreRouteIncrFallbacks() map[string]int64 {
	out := make(map[string]int64)
	for i := range ccoreIncrFallbacks {
		if v := ccoreIncrFallbacks[i].Load(); v != 0 {
			out[ccoreIncrFallbackNames[i]] = v
		}
	}
	return out
}

func ccoreBindRootOnlyFields(lang any) []string {
	if l, ok := lang.(*Language); ok && l != nil {
		return ccoreRootOnlyFields(l)
	}
	return nil
}

func ccoreRouteNanos(reset bool) (parse, convert, edit, check int64) {
	if reset {
		return ccoreParseNanos.Swap(0), ccoreConvertNanos.Swap(0), ccoreEditNanos.Swap(0), ccoreCheckNanos.Swap(0)
	}
	return ccoreParseNanos.Load(), ccoreConvertNanos.Load(), ccoreEditNanos.Load(), ccoreCheckNanos.Load()
}

func ccoreRouteLimits(lang any, sourceLen int, softOff bool) (budget, ceiling int64, hooked bool) {
	l, ok := lang.(*Language)
	if !ok || l == nil {
		return 0, 0, false
	}
	p := NewParser(l)
	if softOff {
		p.SetMemoryBudgetBytes(-1)
	}
	lim := p.ccoreLimitsFor(sourceLen)
	return lim.budget, lim.ceiling, lim.active()
}

func nowNanos() int64 { return time.Now().UnixNano() }

// ccoreAdmission is the answer of ccoreAdmitProvider: the language state, the blob hash
// and the modes the blob is certified for.
type ccoreAdmission struct {
	shortcut        certified.ShortcutPolicy
	automaticForest bool
	forestElection  *ccore.ForestElection
	st              *ccoreLanguageTables
	sha             [32]byte
	modes           uint8
	binding         *ccoreBinding // the identity of the binding this request qualified with
	// providers are the scan providers the blob's certificates grant, by identity;
	// provider is the one this request asks the core to use ("" for the core's own walk).
	providers []string
	provider  string
}

func (a *ccoreAdmission) grants(id string) bool {
	for _, p := range a.providers {
		if p == id {
			return true
		}
	}
	return false
}

// ccoreScannerIdentity names the scanner a certificate was checked with. The
// identity is the concrete type name of the scanner value: a certificate holds
// for that type, and the configuration hash binds the same name. The hash also
// covers the scanner's receiver configuration (internal/ccore/scannerid: every
// field of the struct the scanner value points to, by value). It does not cover
// what a closure captured, a channel, or the state of a lock or of a Language the
// scanner refers to: a scanner that depends on those is outside what a certificate
// authenticates. The route reaches the scanner only through this function.
func ccoreScannerIdentity(sc ExternalScanner) string {
	return reflect.TypeOf(sc).String() // the same text as %T, without an allocation
}

func ccoreScannerType(sc ExternalScanner) string {
	if sc == nil {
		return ""
	}
	return ccoreScannerIdentity(sc)
}

// ccoreFieldsChecked reports whether every root-only field the language sets
// is among the fields a certificate lists as compared with pinned C.
func ccoreFieldsChecked(set []string, checked func(string) bool) bool {
	for _, f := range set {
		if !checked(f) {
			return false
		}
	}
	return true
}

func (p *Parser) ccoreSuppressed() bool {
	return p.forestDeclineMemo != nil && p.forestDeclineMemo.ccoreRouteSuppressed > 0
}

// ccoreAdmitQuiet checks the certificate before building any tables and returns
// the applicable modes or the reason the request cannot use the core.
func (p *Parser) ccoreAdmitQuiet() (ccoreAdmission, int, bool) {
	var a ccoreAdmission
	lang := p.language
	if lang == nil {
		return a, ccoreDeclNotCertified, false
	}
	if p.recoveryInitialOnly || p.admissionRouteSuppressed > 0 || p.ccoreSuppressed() {
		return a, ccoreDeclInternalParser, false
	}
	sha, ok := lang.GrammarBlobSHA256()
	if !ok {
		return a, ccoreDeclNoBinding, false
	}
	a.sha = sha
	switch {
	case len(p.included) > 0 && len(p.ccoreIncludedRanges()) != len(p.included):
		return a, ccoreDeclIncludedRanges, false
	case p.hasActiveParseObservability() || p.compactCertificationTelemetry:
		return a, ccoreDeclObserver, false
	case p.parseWorkLimits.configured():
		return a, ccoreDeclWorkLimits, false
	}
	cert, hasCert := certified.Blobs[sha]
	tcert := certified.Current()
	if tcert != nil && tcert.Excluded[sha] {
		tcert = nil
	}
	if !hasCert && tcert == nil {
		return a, ccoreDeclNotCertified, false
	}
	ccoreTablesMu.Lock()
	st := lang.ccoreTables
	if st == nil || st.owner != lang || !ccoreScannerMatches(st, lang.ExternalScanner) {
		st = ccoreLanguageTablesForLocked(lang)
	}
	ccoreTablesMu.Unlock()
	if st.err != nil {
		if st.scannerError != "" {
			return a, ccoreDeclScannerRefused, false // the registry refuses the scanner; st.err is its reason
		}
		return a, ccoreDeclNoBinding, false
	}
	a.st = st
	var have uint8
	rootBad, scannerBad, configBad := false, false, false
	if hasCert {
		switch {
		case ccoreScannerType(lang.ExternalScanner) != cert.Scanner:
			scannerBad = true
		case st.configHash != cert.Config:
			configBad = true
		case !ccoreFieldsChecked(st.rootOnly, func(f string) bool {
			for _, c := range cert.Checked {
				if c == f {
					return true
				}
			}
			return false
		}):
			rootBad = true
		default:
			have = cert.Modes
			a.providers = cert.Providers
			a.shortcut = cert.Shortcut
		}
	}
	if tcert != nil {
		if ccoreFieldsChecked(st.rootOnly, func(f string) bool { return tcert.Checked[f] }) {
			have |= tcert.Modes
			if len(tcert.Providers) != 0 {
				a.providers = append(append([]string(nil), a.providers...), tcert.Providers...)
			}
		} else {
			rootBad = true
		}
	}
	if have == 0 {
		switch {
		case scannerBad:
			return a, ccoreDeclScannerChanged, false
		case configBad:
			return a, ccoreDeclConfigChanged, false
		case rootBad:
			return a, ccoreDeclRootOnlyField, false
		}
		return a, ccoreDeclNotCertified, false
	}
	need := ccoreModeFresh | ccoreModeRecover
	if lang.ExternalScanner != nil {
		need |= ccoreModeScan
	}
	if have&need != need {
		return a, ccoreDeclModeMissing, false
	}
	if len(p.included) > 0 && have&ccoreModeRanges == 0 {
		return a, ccoreDeclIncludedRanges, false
	}
	a.modes = have
	a.binding = st.token
	return a, 0, true
}

// ccoreAdmitProvider is ccoreAdmitQuiet that counts the decline reason. A
// request that asks the core to use the scan provider id also needs the blob to
// list the provider; an empty id asks for the core's own walk.
func (p *Parser) ccoreAdmitProvider(id string) (ccoreAdmission, bool) {
	a, reason, ok := p.ccoreAdmitQuiet()
	if ok && id != "" {
		if a.grants(id) {
			a.provider = id
		} else {
			ok, reason = false, ccoreDeclProviderNotGranted
		}
	}
	if !ok {
		ccoreDecline(reason)
	}
	return a, ok
}

// ccoreTableProvider is the identity of the table-backed scan provider, as a
// certificate lists it.
const ccoreTableProvider = ccore.TableScanID

// ccoreTableProviderQualified is ccoreProviderQualified for the table-backed provider.
func (p *Parser) ccoreTableProviderQualified() bool {
	return p.ccoreProviderQualified(ccoreTableProvider)
}

// ccoreProviderQualified reports whether the core would serve a request that
// carries the scan provider id: the language is certified, nothing in the
// parser's configuration declines the route, and the certificate lists id.
func (p *Parser) ccoreProviderQualified(id string) bool {
	if ccoreUnset() || !p.ccoreEligibleFor(ccoreSourceProvider) {
		return false
	}
	a, _, ok := p.ccoreAdmitQuiet()
	return ok && a.grants(id)
}

// ccoreParser takes a core parser of the language from its pool. Each public
// Parser that runs gets a parser (and a scanner hook) of its own: a parser is
// out of the pool while it parses.
func (st *ccoreLanguageTables) get(lang *Language) (*ccore.Parser, error) {
	if cp, ok := st.pool.Get().(*ccore.Parser); ok {
		return cp, nil
	}
	ccoreTablesMu.Lock()
	cfg := ccore.Config{Tables: st.base, Nodes: ccoreNodeTablesForLocked(lang, st)}
	ccoreTablesMu.Unlock()
	if st.scanner != nil {
		sc, err := newCcoreExtScanner(st.scanner)
		if err != nil {
			return nil, err
		}
		cfg.Scanner = sc.hook()
	}
	return ccore.NewParser(cfg)
}

// ccoreLimits are the limits of one request: the soft memory budget, the hard
// ceiling, and whether a timeout or cancellation flag is set.
//
// The soft budget is checked against the core's own accounting, which is
// deterministic: the same input stops at the same place every time. The hard
// ceiling is not: it uses the present engine's heap-growth backstop
// (enterRuntimeMemoryBudget, runtimeMemoryBudgetStopReason), with its source
// size rules, so memory the core does not account for (an external scanner's
// allocations, for example) is seen too.
type ccoreLimits struct {
	budget, ceiling int64
	deadline        bool // a timeout or a cancellation flag is set
}

func (p *Parser) ccoreLimitsFor(sourceLen int) ccoreLimits {
	return ccoreLimits{
		budget:   parseMemoryBudgetForParser(p, sourceLen),
		ceiling:  parseMemoryHardCeilingBytesForParse(p, sourceLen),
		deadline: p.needsParseBudget(),
	}
}

// active reports whether anything limits the parse: a timeout or cancellation
// flag, or a memory budget or hard ceiling. The values are the ones the
// present engine uses (the default budget scaled by the source length, the
// value of GOT_PARSE_MEMORY_BUDGET_MB, SetMemoryBudgetBytes, and the hard
// ceiling of GOT_PARSE_MEMORY_HARD_CEILING_MB, which defaults to 2048 MiB), so
// a parse the core serves stops under the same limits and with the same stop
// reason. Only a parser with every limit switched off runs without the
// progress hook.
func (l ccoreLimits) active() bool { return l.budget > 0 || l.ceiling > 0 || l.deadline }

// ccorePoll is the one stop test: the soft memory budget against the
// footprint (the repository's order, parsercore_phase0_stop_control.go
// pollStopControl), then the heap-growth backstop of the hard ceiling, then the
// deadline and the cancellation flag.
//
// now asks for a real heap sample (runtimeMemoryBudgetStopReasonNow) instead of the
// sampled check. The route asks once, after the conversion and before it
// publishes a tree, so that a parse with few polls (one large scanner token, say)
// cannot end with the heap above the ceiling without the route having looked.
func (p *Parser) ccorePoll(l ccoreLimits, footprint uint64, now bool) ParseStopReason {
	if l.budget > 0 && footprint >= uint64(l.budget) {
		return ParseStopMemoryBudget
	}
	if l.ceiling > 0 {
		var r ParseStopReason
		if now {
			r = p.runtimeMemoryBudgetStopReasonNow()
		} else {
			r = p.runtimeMemoryBudgetStopReason(footprint)
		}
		if r != ParseStopNone {
			return r
		}
	}
	if l.deadline {
		return p.activeParseStopReason()
	}
	return ParseStopNone
}

func ccoreStopDecline(r ParseStopReason) {
	switch r {
	case ParseStopTimeout:
		ccoreDecline(ccoreDeclStopDeadline)
	case ParseStopCancelled:
		ccoreDecline(ccoreDeclStopCancelled)
	case ParseStopMemoryBudget:
		ccoreDecline(ccoreDeclStopMemory)
	default:
		ccoreDecline(ccoreDeclCoreStop)
	}
}

// ccorePoolKeepBytes bounds what a pooled core parser keeps: a parser whose
// buffers and store pages hold more than this after a request is dropped
// instead of pooled.
const ccorePoolKeepBytes = 32 << 20

// ccoreTestAccountHook is test-only (nil in production): when a request ends it
// receives the ledger's Base (the capacity the request started with), its Alloc
// (every array made for it since) and the bytes the conversion reserved for the
// public arena.
var ccoreTestAccountHook atomic.Pointer[func(base, alloc, arenaCharged, arenaHeld int64)]

// ccoreTestScanEvery is test-only (0 in production): when set it replaces the number
// of scanner callbacks between two forced limit checks (a cost measurement switches the
// check off with a very large value).
var ccoreTestScanEvery atomic.Int64

// ccoreTestPollHook is test-only (nil in production): it receives every
// (total, sample) pair the route's limit check sees, in order, and whether the
// public tree is being built (the core's parse has ended).
var ccoreTestPollHook atomic.Pointer[func(total int64, sample bool, converting bool)]

// ccoreTestReduceCheck is test-only (false in production): when set, every core parser a
// request leases compares each linear reduction with the general walk (a difference panics).
var ccoreTestReduceCheck atomic.Bool

// ccoreTestConvertHook is test-only (nil in production): it is called every
// 1024 nodes of a conversion with the node count, so a test can make it panic.
var ccoreTestConvertHook atomic.Pointer[func(count int)]

// ccoreRun parses source with the core and converts the result into a public
// tree. The public tree holds ordinary nodes; it also keeps the core tree
// (ccoreArenaState.ct) so that Tree.Edit and a later reparse can use it. The core
// parser's buffers are reset before this returns. ok is false, with the reason counted, when the present path must
// run. Ownership is guarded from acquisition to the end by the deferred
// function: the core parser is reset and pooled (or dropped when it holds
// too much) on every exit, a panic included.

// ccoreFieldView is what the field searches (relevant, firstRelevant,
// cFieldChild, publicPath, recordFieldAnswers) read of the core tree.
type ccoreFieldView interface {
	Children(h ccore.Handle) []ccore.Handle
	Extra(h ccore.Handle) bool
	Visible(h ccore.Handle) bool
	ChildCount(h ccore.Handle) uint32
	ProductionID(h ccore.Handle) uint16
}

type ccoreConv struct {
	led          *ccore.Ledger // the request's account, shared with the core: every array the conversion makes is reserved in it first
	steps        int           // visited nodes of field searches since the last limit check
	path         []int32       // the public path being built (grown through the ledger)
	cpath        []int32       // the raw child indexes the field search took (grown through the ledger)
	arenaCharged int64         // bytes reserved for the public arena (cold reservation rebased, then each node)
	s            *ccore.Store
	fs           ccoreFieldView // the same store, as the field searches see it (a test supplies a hand-made one)
	lang         *Language
	conv         *ccoreConvTables // the private snapshot taken at the bind
	arena        *nodeArena
	nodes        []*Node        // finished visible children, a stack
	fids         []FieldID      // the field id of each, parallel to nodes
	cfids        []FieldID      // the cursor field of each (ts_tree_cursor_current_field_id), parallel to nodes
	hs           []ccore.Handle // the core node of each, parallel to nodes
	hidden       bool           // a child with a field of its own was hidden (the lookup may differ from the stored ids)
	count        int
	depth        int // recursion depth of build and gather
	stop         bool
	tooDeep      bool
}

// ccoreCheckBytes is the most the conversion allocates between two limit checks: the
// ledger's granularity (ccore.LedgerGranularity, 256 KiB). Every array the conversion
// makes is reserved in the ledger before it is made; the ledger checks the limits when
// the reservations since the last check reach this size, or at once for one reservation
// that large, and the check sees the total with the new array included.
const ccoreCheckBytes = ccore.LedgerGranularity

// ccoreNodeBytes is the size of one public node, as the arena allocates it.
var ccoreNodeBytes = int(unsafe.Sizeof(Node{}))

// l is the conversion's ledger; a conversion built without one (a test) has none, and
// gets a ledger with no limit.
func (c *ccoreConv) l() *ccore.Ledger {
	if c.led == nil {
		c.led = &ccore.Ledger{}
	}
	return c.led
}

// reserve reserves n bytes about to be made. A refusal stops the conversion.
func (c *ccoreConv) reserve(n int64) bool {
	if c.stop {
		return false
	}
	if n <= 0 || c.l().Reserve(n) {
		return true
	}
	c.stop = true
	return false
}

// checkNow asks the limits at once (the deadline and the heap sample cadence of the
// route); a refusal stops the conversion.
func (c *ccoreConv) checkNow() bool {
	if c.stop {
		return false
	}
	if !c.l().Check(false) {
		c.stop = true
		return false
	}
	return true
}

// step counts one visited node of a field search; the limits (the deadline
// included) are checked every 4096 of them.
func (c *ccoreConv) step() bool {
	if c.stop {
		return false
	}
	if c.steps++; c.steps&4095 == 0 && c.l().Active() {
		return c.checkNow()
	}
	return true
}

// push appends one visible child to the conversion stacks. A stack that is full
// grows into a new array that is reserved first, at its whole size; the arrays it
// replaces stay charged. All four stacks grow before any of them is appended to, so
// a refusal leaves them as they were.
func (c *ccoreConv) push(n *Node, f, cf FieldID, h ccore.Handle) bool {
	if c.stop {
		return false
	}
	l := c.l()
	if l.Active() {
		// The new arrays of every full stack are reserved with one check, so a refusal
		// leaves all four as they were.
		nn, ok1 := ccore.GrowSize(c.nodes, 1)
		nf, ok2 := ccore.GrowSize(c.fids, 1)
		nc, ok3 := ccore.GrowSize(c.cfids, 1)
		nh, ok4 := ccore.GrowSize(c.hs, 1)
		if !ok1 || !ok2 || !ok3 || !ok4 {
			c.stop = true
			return false
		}
		total := int64(nn)*ccore.ElemBytes[*Node]() + int64(nf+nc)*ccore.ElemBytes[FieldID]() + int64(nh)*ccore.ElemBytes[ccore.Handle]()
		if total > 0 && !c.reserve(total) {
			return false
		}
		if nn > 0 {
			c.nodes = ccore.GrowTo(c.nodes, nn)
		}
		if nf > 0 {
			c.fids = ccore.GrowTo(c.fids, nf)
		}
		if nc > 0 {
			c.cfids = ccore.GrowTo(c.cfids, nc)
		}
		if nh > 0 {
			c.hs = ccore.GrowTo(c.hs, nh)
		}
	} else {
		c.nodes, _ = ccore.Grow(l, c.nodes, 1)
		c.fids, _ = ccore.Grow(l, c.fids, 1)
		c.cfids, _ = ccore.Grow(l, c.cfids, 1)
		c.hs, _ = ccore.Grow(l, c.hs, 1)
	}
	c.nodes, _ = ccore.Push(l, c.nodes, n)
	c.fids, _ = ccore.Push(l, c.fids, f)
	c.cfids, _ = ccore.Push(l, c.cfids, cf)
	c.hs, _ = ccore.Push(l, c.hs, h)
	return true
}

// ccoreMaxDepth bounds the recursion of the conversion. A Go stack overflow is
// fatal and cannot be recovered, so a tree deeper than this is declined
// (reason depth_limit) and the present path parses it.
const ccoreMaxDepth = 20000

// hiddenInheritedField is the field that a hidden child with children passes
// down to its own children, as C/node.c ts_node_field_name_for_child does:
// the field map entry of the parent's production at the iterator's
// structural_child_index - 1. For a non-extra child that is the child's own
// index (the iterator has already stepped past it); for an extra child, which
// does not step, it is the previous structural child.
func (c *ccoreConv) hiddenInheritedField(prod uint16, structural int, extra bool, own, inherited FieldID) FieldID {
	if extra {
		own = 0
		if structural > 0 {
			own = c.ownField(prod, structural-1)
		}
	}
	if own != 0 {
		return own
	}
	return inherited
}

// alias is the alias sequence of a production (C ts_language_alias_sequence).
func (c *ccoreConv) alias(prod uint16) []Symbol {
	if int(prod) < len(c.conv.alias) {
		return c.conv.alias[prod]
	}
	return nil
}

// ownField is the non-inherited field-map entry of a production at a
// structural child index (C/node.c ts_node_field_name_for_child).
func (c *ccoreConv) ownField(prod uint16, structural int) FieldID {
	for _, e := range c.fieldMapOf(prod) {
		if !e.Inherited && int(e.ChildIndex) == structural {
			return e.FieldID
		}
	}
	return 0
}

// gather appends the visible children of h, flattening hidden ones. start is
// the position of h after its padding. hiddenField is the field inherited from
// the nearest hidden ancestor that has one; mask holds the supertype bits of
// the hidden ancestors between h and the visible parent.
func (c *ccoreConv) gather(h ccore.Handle, start ccore.Length, hiddenField, cursorInherit FieldID, mask uint32) {
	if c.depth++; c.depth > ccoreMaxDepth {
		c.stop, c.tooDeep = true, true
	}
	defer func() { c.depth-- }()
	s := c.s
	prod := s.ProductionID(h)
	seq := c.alias(prod)
	structural := 0
	pos := start
	for i, child := range s.Children(h) {
		if c.stop {
			return
		}
		var a Symbol
		var own, cur FieldID
		extra := s.Extra(child)
		if !extra {
			if structural < len(seq) {
				a = seq[structural]
			}
			own = c.ownField(prod, structural)
			cur = own
			if cur == 0 {
				cur = cursorInherit
			}
			structural++
		}
		if i > 0 {
			pos = pos.Add(s.Padding(child))
		}
		if s.Visible(child) || a != 0 {
			f := own
			if f == 0 && !extra {
				f = hiddenField
			}
			if n := c.build(child, pos, a, mask); n != nil {
				if !c.push(n, f, cur, child) {
					return
				}
			}
		} else if s.ChildCount(child) > 0 {
			hf := c.hiddenInheritedField(prod, structural, extra, own, hiddenField)
			if hf != 0 {
				c.hidden = true // a hidden child passes a field to its children: the lookup may differ from the stored ids
			}
			c.gather(child, pos, hf, cur, mask|c.conv.supertypeBit(Symbol(s.Symbol(child))))
		}
		pos = pos.Add(s.Size(child))
	}
}

func (c *ccoreConv) build(h ccore.Handle, start ccore.Length, alias Symbol, mask uint32) *Node {
	if c.stop {
		return nil
	}
	if c.depth++; c.depth > ccoreMaxDepth {
		c.stop, c.tooDeep = true, true
		c.depth--
		return nil
	}
	defer func() { c.depth-- }()
	if c.count++; c.count&1023 == 0 {
		if h := ccoreTestConvertHook.Load(); h != nil {
			(*h)(c.count)
		}
		if c.l().Active() && !c.checkNow() {
			return nil
		}
	}
	s := c.s
	sym := Symbol(s.Symbol(h))
	named := s.Named(h)
	if alias != 0 {
		sym = alias
		if int(alias) < len(c.conv.named) {
			named = c.conv.named[alias]
		}
	}
	end := start.Add(s.Size(h))
	sp := Point{Row: start.Row(), Column: start.Col()}
	ep := Point{Row: end.Row(), Column: end.Col()}
	var n *Node
	if s.ChildCount(h) > 0 {
		base := len(c.nodes)
		savedHidden := c.hidden
		c.hidden = false
		c.gather(h, start, 0, 0, 0)
		cnt := len(c.nodes) - base
		if cnt > 0 && !c.stop {
			nf := 0 // field ids are kept only when some child has a field
			for _, f := range c.fids[base:] {
				if f != 0 {
					nf = cnt
					break
				}
			}
			if !c.reserveArena(cnt, nf) { // the parent node, its child list and its field storage, before any is made
				c.hidden = savedHidden
				c.nodes, c.fids, c.cfids, c.hs = c.nodes[:base], c.fids[:base], c.cfids[:base], c.hs[:base]
				return nil
			}
			kids := c.arena.allocNodeSlice(cnt)
			copy(kids, c.nodes[base:])
			var fids []FieldID
			if nf > 0 {
				fids = c.arena.allocFieldIDSlice(cnt)
				copy(fids, c.fids[base:])
			}
			n = newParentNodeInArena(c.arena, sym, named, kids, fids, s.ProductionID(h))
			c.recordFieldAnswers(n, h, base, cnt)
		}
		c.hidden = savedHidden
		c.nodes = c.nodes[:base]
		c.fids = c.fids[:base]
		c.cfids = c.cfids[:base]
		c.hs = c.hs[:base]
		if c.stop {
			return nil
		}
		if n != nil {
			n.dynamicPrecedence = s.DynamicPrecedence(h)
		}
	}
	if n == nil {
		if !c.reserveArena(0, 0) { // the leaf node
			return nil
		}
		n = newLeafNodeInArena(c.arena, sym, named, 0, 0, sp, ep)
	}
	n.startByte, n.endByte = start.Bytes(), end.Bytes()
	n.startPoint, n.endPoint = sp, ep
	n.setNamed(named)
	n.setExtra(s.Extra(h))
	n.setMissing(s.Missing(h))
	n.setHasError(s.ErrorCost(h) > 0) // C ts_subtree_error_cost > 0
	if s.HasChanges(h) {              // C ts_node_has_changes
		n.setDirty(true)
	}
	// C ts_node_parse_state: the core's state before the node, kept in inline leaves
	// too (65535 is TS_TREE_STATE_NONE, a fragile node). ParseState is not
	// computed here: ccoreNextParseState derives it when it is called, from the
	// node's state and its grammar symbol (kept in equivVersion, which no engine
	// reads on a core tree; an alias replaces n.symbol).
	n.preGotoState = StateID(s.ParseState(h))
	n.equivVersion = uint32(s.Symbol(h))
	if mask != 0 {
		n.addSupertypeMask(c.arena, mask)
	}
	if s.DependsOnColumn(h) { // C Subtree.depends_on_column; the legacy edit walk reads it
		c.arena.setNodeDependsOnColumnBit(n, true)
	}
	return n
}

// ccoreConvEnd is what the conversion reserved for the public arena, by the same
// measure the ledger uses. Tests read it through ccoreTestAccountHook.
type ccoreConvEnd struct {
	arenaCharged int64
	arenaHeld    int64 // what the public arena holds: slabs and side arrays
}

// ccoreConvertTree converts the core's tree into a public tree. The public tree
// holds ordinary nodes and the language tables, and keeps the core tree
// (ccoreArenaState.ct) until its arena is released.
// It returns nil when the ledger refused a reservation, when the tree is too deep,
// or by panic; the conversion's arena is released here on every such exit. The last
// check (with a real heap sample) runs before the tree is published.
func ccoreConvertTree(ct *ccore.Tree, src []byte, lang *Language, blob [32]byte, st *ccoreLanguageTables, led *ccore.Ledger, end *ccoreConvEnd) (tree *Tree, tooDeep bool) {
	rd := ct.Reader()
	c := &ccoreConv{s: rd, fs: rd, lang: lang, conv: st.conv, led: led}
	if led.Active() && !led.Reserve(ccoreColdArenaBytes()) { // the most a cold arena can make, before it is acquired
		return nil, false
	}
	arena := acquireNodeArena(arenaClassFull)
	forgetArenaCheckout(arena) // before the core tree is attached: see forgetCheckedOut
	c.arena = arena
	done := false
	defer func() {
		if end != nil {
			end.arenaCharged = c.arenaCharged
			end.arenaHeld = int64(arenaAllocatedVolume(arena)) + ccoreArenaSide(arena)
		}
		if !done {
			arena.Release()
		}
	}()
	if led.Active() {
		// Replace the reservation by what the arena holds: a pooled arena keeps slabs, a cold one makes them.
		c.arenaCharged = int64(arenaAllocatedVolume(arena)) + ccoreArenaSide(arena)
		if !led.Rebase(ccoreColdArenaBytes(), c.arenaCharged) {
			return nil, false
		}
	}
	arena.ccore = &ccoreArenaState{tables: st.base, binding: st.token, ct: ct, curLen: int64(len(src))}
	root := c.build(ct.Root(), c.s.Padding(ct.Root()), 0, 0)
	if root == nil || c.stop {
		return nil, c.tooDeep
	}
	if led.Active() && !led.Check(true) { // the last check, with a real heap sample, before the tree is published
		return nil, false
	}
	setNodeRootLink(root)
	markPublishedRootError(root)
	t := &Tree{
		root: root, source: src, sourceEncoding: InputEncodingUTF8, language: lang, arena: arena,
		parseRuntime: acquireParseRuntime(),
		// Barred from reuse by the other engines: they read scanner checkpoints,
		// reuse proofs and node provenance that a converted tree does not carry.
		incrementalReuseDisabled:   true,
		resultCompatibilityApplied: true, // the nodes are final: no legacy normalization
		dependsOnColumnPropagated:  true,
	}
	rt := t.ensureParseRuntime() // the diagnostics of a served parse that apply: the public tree's nodes and arena
	rt.NodesAllocated = c.count
	rt.ArenaBytesAllocated = int64(arenaAllocatedVolume(arena))
	done = true
	return t, false
}

// stateNone values: C marks a fragile node's parse state TS_TREE_STATE_NONE.
const ccoreStateNone = StateID(65535)

// ccoreNextParseState is C ts_node_next_parse_state: the next state of the
// language for the node's parse_state and its grammar symbol (ts_language_next_state:
// a goto for a non-terminal, the shift target for a terminal, the same state
// for an extra, 0 for the error symbols). A node without a state gives 0, which
// is what this package returns for a node without a state.
func ccoreNextParseState(t *ccore.Tables, n *Node) StateID {
	if n.preGotoState == ccoreStateNone {
		return 0
	}
	if n.childIndex == compactViewChildIndex {
		if v := ccoreViewsOf(n); v != nil {
			return StateID(t.NextState(uint16(n.preGotoState), v.grammarSymbol(n)))
		}
	}
	return StateID(t.NextState(uint16(n.preGotoState), ccore.Symbol(n.equivVersion)))
}

// ccoreArenaState is what the arena of a core tree (and of its copies) keeps
// besides the nodes: the language tables, and the answers of the field APIs
// that C gives differently from the one stored field id of each child.
//
// C has three field questions with three answers. ts_node_field_name_for_child
// is the stored id (Node.FieldNameForChild). ts_node_child_by_field_id searches
// the hidden structure and may return a child that the stored id does not name
// (a field of a hidden node, a hidden node with visible children).
// ts_tree_cursor_current_field_id stops at an extra ancestor, where the node
// accessor inherits the previous child's field. The last two are recorded only
// for the nodes where they differ from the stored ids, in extras.
type ccoreArenaState struct {
	forestOrdinaryCompatible bool
	forestElection           bool
	tables                   *ccore.Tables
	binding                  *ccoreBinding // the identity of the binding the tree was parsed with (not the state, so a tree does not keep a parser pool alive)
	extras                   map[*Node]*ccoreNodeExtra
	ct                       *ccore.Tree
	edits                    int
	curLen                   int64
	budget                   int64
	broken                   bool
	incremental              bool
	viewOrigin               bool
	reusedNodes              int
	reusedBytes              uint64
}

// ccoreNodeExtra is the field answers of one parent that differ from its stored ids.
type ccoreNodeExtra struct {
	cursor []FieldID       // ts_tree_cursor_current_field_id of each child; nil when it equals the stored ids
	lookup []ccoreFieldHit // ts_node_child_by_field_id for each field of the production; nil when it equals the stored ids
}

// ccoreFieldHit is the result of ts_node_child_by_field_id for one field: the
// path of child indexes from the parent to the node C returns (a field inherited
// from a visible child gives a descendant, not a child), or none.
type ccoreFieldHit struct {
	field FieldID
	found bool
	path  []int32
}

// forCopy is the state of the arena of a copy: the tables are shared; the
// extras are filled by cloneExtra as the nodes are cloned.
func (a *ccoreArenaState) forCopy() *ccoreArenaState {
	if a == nil {
		return nil
	}
	return &ccoreArenaState{tables: a.tables, binding: a.binding, forestElection: a.forestElection, forestOrdinaryCompatible: a.forestOrdinaryCompatible}
}

func (a *ccoreArenaState) cloneExtra(dst *Node, from *ccoreArenaState, src *Node) {
	if from.extras == nil {
		return
	}
	if e := from.extras[src]; e != nil { // immutable once recorded: shared by the copy
		if a.extras == nil {
			a.extras = map[*Node]*ccoreNodeExtra{}
		}
		a.extras[dst] = e
	}
}

// lookupChild answers ChildByFieldName for a node that has a recorded lookup: the
// path to the result (found false: none), and whether the node has one.
func (a *ccoreArenaState) lookupChild(n *Node, fid FieldID) (path []int32, found, recorded bool) {
	e := a.extras[n]
	if e == nil || e.lookup == nil {
		return nil, false, false
	}
	for _, h := range e.lookup {
		if h.field == fid {
			return h.path, h.found, true
		}
	}
	return nil, false, true // not a field of the production: C finds none
}

func (a *ccoreArenaState) cursorField(parent *Node, child int) (FieldID, bool) {
	e := a.extras[parent]
	if e == nil || e.cursor == nil || child < 0 || child >= len(e.cursor) {
		return 0, false
	}
	return e.cursor[child], true
}

// ccoreStateOf returns the arena state of the tree (nil for another tree).
func (t *Tree) ccoreStateOf() *ccoreArenaState {
	if t == nil || t.arena == nil {
		return nil
	}
	return t.arena.ccore
}

// ccoreTablesOf returns the tables of the core that built the tree (nil for another tree).
func (t *Tree) ccoreTablesOf() *ccore.Tables {
	if st := t.ccoreStateOf(); st != nil {
		return st.tables
	}
	return nil
}

// fieldEntries are the field-map entries of a production for one field id, in
// the table's order (sorted by child index), and whether the production has any
// inherited entry at all.
func (c *ccoreConv) fieldMapOf(prod uint16) []FieldMapEntry {
	l := c.conv
	if int(prod) >= len(l.fieldSlices) {
		return nil
	}
	sl := l.fieldSlices[prod]
	end := int(sl[0]) + int(sl[1])
	if end > len(l.fieldEntries) {
		return nil
	}
	return l.fieldEntries[sl[0]:end]
}

// relevant reports whether a child is a node of the public tree at its own
// level: visible, or given a name by an alias of the production (C
// ts_node__is_relevant with anonymous nodes included).
func (c *ccoreConv) relevant(child ccore.Handle, seq []Symbol, structural int) bool {
	return c.fs.Visible(child) || (structural >= 0 && structural < len(seq) && seq[structural] != 0)
}

// firstRelevant is C ts_node_child(h, 0): the first child that is relevant,
// searching hidden children in order. On success c.cpath ends with the raw child
// index taken at each level below h (the occurrence, not the handle: two equal
// inline leaves have one handle); on failure c.cpath is as it was.
func (c *ccoreConv) firstRelevant(h ccore.Handle) (ccore.Handle, bool) {
	s := c.fs
	mark := len(c.cpath)
	seq := c.alias(s.ProductionID(h))
	structural := 0
	for j, child := range s.Children(h) {
		if !c.step() {
			return 0, false
		}
		idx := structural
		if !s.Extra(child) {
			structural++
		} else {
			idx = -1
		}
		if !c.pushIndex(&c.cpath, int32(j)) {
			return 0, false
		}
		if c.relevant(child, seq, idx) {
			return child, true
		}
		if s.ChildCount(child) > 0 {
			if r, ok := c.firstRelevant(child); ok {
				return r, true
			}
			if c.stop {
				return 0, false
			}
		}
		c.cpath = c.cpath[:mark]
	}
	return 0, false
}

// cFieldChild is C ts_node_child_by_field_id (node.c) on the core node h. On success
// c.cpath ends with the raw child index taken at each level below h, so the caller
// knows which occurrence was found (a handle does not say: equal inline leaves share one
// handle); on failure c.cpath is as it was.
func (c *ccoreConv) cFieldChild(h ccore.Handle, f FieldID, depth int) (ccore.Handle, bool) {
	s := c.fs
	mark := len(c.cpath)
	fail := func() (ccore.Handle, bool) { c.cpath = c.cpath[:mark]; return 0, false }
	for ; depth < ccoreMaxDepth; depth++ {
		if f == 0 || s.ChildCount(h) == 0 || !c.step() {
			return fail()
		}
		prod := s.ProductionID(h)
		// The entries of this field: counted first, then copied into the stack buffer
		// or, when they do not fit it, into one array that is reserved before it is made.
		var local [8]FieldMapEntry
		matches := 0
		for _, e := range c.fieldMapOf(prod) {
			if e.FieldID == f {
				matches++
			}
		}
		if matches == 0 {
			return fail()
		}
		sub := local[:0]
		if matches > len(local) {
			var ok bool
			if sub, ok = ccore.MakeCap[FieldMapEntry](c.l(), matches); !ok {
				c.stop = true
				return fail()
			}
		}
		for _, e := range c.fieldMapOf(prod) {
			if e.FieldID == f {
				sub = sub[:len(sub)+1]
				sub[len(sub)-1] = e
			}
		}
		seq := c.alias(prod)
		structural, k := 0, 0
		next := ccore.Handle(0)
		descend := false
		for j, child := range s.Children(h) {
			if s.Extra(child) {
				continue
			}
			if !c.step() {
				return fail()
			}
			idx := structural
			structural++
			if idx < int(sub[k].ChildIndex) {
				continue
			}
			switch {
			case sub[k].Inherited:
				if k+1 == len(sub) { // the last possible child: continue inside it
					next, descend = child, true
					if !c.pushIndex(&c.cpath, int32(j)) {
						return fail()
					}
				} else {
					at := len(c.cpath)
					if !c.pushIndex(&c.cpath, int32(j)) {
						return fail()
					}
					if r, ok := c.cFieldChild(child, f, depth+1); ok {
						return r, true
					} else if c.stop {
						return fail()
					} else if c.cpath = c.cpath[:at]; k+1 == len(sub) {
						return fail()
					} else {
						k++
					}
				}
			case c.relevant(child, seq, idx):
				if !c.pushIndex(&c.cpath, int32(j)) {
					return fail()
				}
				return child, true
			default:
				// A hidden child that has visible children: C returns the first of them
				// (ts_node_child_count(child) > 0, the visible count, not the raw one). A
				// hidden child with none is passed over, like a leaf.
				if s.ChildCount(child) > 0 {
					at := len(c.cpath)
					if !c.pushIndex(&c.cpath, int32(j)) {
						return fail()
					}
					if r, ok := c.firstRelevant(child); ok {
						return r, true
					}
					c.cpath = c.cpath[:at]
					if c.stop {
						return fail()
					}
				}
				if k++; k == len(sub) {
					return fail()
				}
			}
			if descend {
				break
			}
		}
		if !descend {
			return fail()
		}
		h = next
	}
	return fail()
}

// ccoreExtraBytes is the size of one ccoreNodeExtra. ccoreMapEntryBytes is what one
// entry of the arena's extras map is charged: the bucket slot, the key and the
// value pointer, with the share of the table that grows with it (an estimate;
// TestCcoreLedgerEqualsTheRealAllocation checks the total against the heap).
var ccoreExtraBytes = int(unsafe.Sizeof(ccoreNodeExtra{}))

const ccoreMapEntryBytes = 64

// recordFieldAnswers records, for the parent n just built from the core node h
// whose visible children are c.nodes[base:base+cnt], the field answers that
// differ from the stored ids: the cursor field of each child, and the result of
// ts_node_child_by_field_id for each field of the production. The lookup is
// computed only when the production inherits a field or a field names a hidden
// child; otherwise the stored ids give C's answer. Each allocation is charged
// before it is made, whether or not the answer is kept.
func (c *ccoreConv) recordFieldAnswers(n *Node, h ccore.Handle, base, cnt int) {
	var e *ccoreNodeExtra
	cur := c.cfids[base : base+cnt]
	for i, f := range cur {
		if f != c.fids[base+i] {
			if !c.reserve(int64(ccoreExtraBytes)) { // the record
				return
			}
			cursor, ok := ccore.Make[FieldID](c.l(), len(cur))
			if !ok {
				c.stop = true
				return
			}
			copy(cursor, cur)
			e = &ccoreNodeExtra{cursor: cursor}
			break
		}
	}
	entries := c.fieldMapOf(c.fs.ProductionID(h))
	complexMap := c.hidden
	for _, en := range entries {
		if en.Inherited {
			complexMap = true
			break
		}
	}
	inMap := func(f FieldID) bool {
		for _, en := range entries {
			if en.FieldID == f {
				return true
			}
		}
		return false
	}
	if !complexMap {
		for j := 0; j < cnt && !complexMap; j++ { // a stored field that the production's map lacks: C's lookup finds none
			if f := c.fids[base+j]; f != 0 && !inMap(f) {
				complexMap = true
			}
		}
	}
	if complexMap {
		fields, ok := ccore.MakeCap[FieldID](c.l(), len(entries)+cnt) // the field list, at its largest
		if !ok {
			c.stop = true
			return
		}
		have := func(f FieldID) bool {
			for _, x := range fields {
				if x == f {
					return true
				}
			}
			return false
		}
		for _, en := range entries {
			if !have(en.FieldID) {
				fields = fields[:len(fields)+1]
				fields[len(fields)-1] = en.FieldID
			}
		}
		for j := 0; j < cnt; j++ {
			if f := c.fids[base+j]; f != 0 && !have(f) {
				fields = fields[:len(fields)+1]
				fields[len(fields)-1] = f
			}
		}
		hits, ok := ccore.MakeCap[ccoreFieldHit](c.l(), len(fields)) // allocated whether or not it is kept
		if !ok {
			c.stop = true
			return
		}
		differs := false
		for _, fid := range fields {
			hit := ccoreFieldHit{field: fid}
			c.cpath = c.cpath[:0]
			if _, ok := c.cFieldChild(h, fid, 0); ok {
				if path, ok := c.publicPath(h, c.cpath); ok {
					hit.found, hit.path = true, path
				}
			}
			if c.stop {
				return
			}
			stored := int32(-1)
			for j := 0; j < cnt; j++ {
				if c.fids[base+j] == fid {
					stored = int32(j)
					break
				}
			}
			if hit.found != (stored >= 0) || hit.found && (len(hit.path) != 1 || hit.path[0] != stored) {
				differs = true
			}
			hits = hits[:len(hits)+1]
			hits[len(hits)-1] = hit
		}
		if differs {
			if e == nil {
				if !c.reserve(int64(ccoreExtraBytes)) { // the lookup-only record, before it is made
					return
				}
				e = &ccoreNodeExtra{}
			}
			e.lookup = hits
		}
	}
	if e != nil {
		if !c.reserve(ccoreMapEntryBytes) { // the map entry (an estimate for a Go map)
			return
		}
		st := c.arena.ccore
		if st.extras == nil {
			st.extras = map[*Node]*ccoreNodeExtra{}
		}
		st.extras[n] = e
	}
}

// publicPath is the path of public child indexes from the public node of h to the
// public node that the raw path rp (the child indexes the field search took below
// h, c.cpath) ends at. It follows the occurrence the search found, level by level:
// the index at a level is the number of relevant nodes before it among the children of
// the public parent, with hidden children flattened. The path is built in one shared
// buffer (grown through the ledger) and copied once at the end: O(depth) storage.
func (c *ccoreConv) publicPath(h ccore.Handle, rp []int32) ([]int32, bool) {
	c.path = c.path[:0]
	parent := h
	for len(rp) > 0 {
		idx := int32(0)
		node, rest, ok := c.locate(parent, rp, &idx)
		if !ok || c.stop {
			return nil, false
		}
		if !c.pushIndex(&c.path, idx) {
			return nil, false
		}
		parent, rp = node, rest
	}
	out, ok := ccore.Make[int32](c.l(), len(c.path))
	if !ok {
		c.stop = true
		return nil, false
	}
	copy(out, c.path)
	return out, true
}

// locate follows the raw path rp from h through hidden nodes to the first relevant node
// on it. *idx counts the relevant nodes before that node among the children of h's
// public level (hidden children flattened). It returns the node and the rest of the path.
func (c *ccoreConv) locate(h ccore.Handle, rp []int32, idx *int32) (ccore.Handle, []int32, bool) {
	s := c.fs
	seq := c.alias(s.ProductionID(h))
	structural := 0
	for j, child := range s.Children(h) {
		if !c.step() {
			return 0, nil, false
		}
		sidx := -1
		if !s.Extra(child) {
			sidx = structural
			structural++
		}
		rel := c.relevant(child, seq, sidx)
		if int32(j) == rp[0] {
			if rel {
				return child, rp[1:], true
			}
			if len(rp) == 1 {
				return 0, nil, false // the path ends at a hidden node: the search never returns one
			}
			return c.locate(child, rp[1:], idx)
		}
		if rel {
			*idx++
		} else if s.ChildCount(child) > 0 {
			n, ok := c.countFlat(child)
			if !ok {
				return 0, nil, false
			}
			*idx += n
		}
	}
	return 0, nil, false
}

// countFlat is the number of relevant nodes among the children of the hidden node h,
// hidden children flattened.
func (c *ccoreConv) countFlat(h ccore.Handle) (int32, bool) {
	s := c.fs
	seq := c.alias(s.ProductionID(h))
	structural := 0
	n := int32(0)
	for _, child := range s.Children(h) {
		if !c.step() {
			return 0, false
		}
		sidx := -1
		if !s.Extra(child) {
			sidx = structural
			structural++
		}
		if c.relevant(child, seq, sidx) {
			n++
		} else if s.ChildCount(child) > 0 {
			m, ok := c.countFlat(child)
			if !ok {
				return 0, false
			}
			n += m
		}
	}
	return n, true
}

func (c *ccoreConv) pushIndex(path *[]int32, v int32) bool {
	var ok bool
	if *path, ok = ccore.Push(c.l(), *path, v); !ok { // a full path grows into a new array, reserved first
		c.stop = true
		return false
	}
	return true
}

// The arena side of the ledger. The public arena makes its storage in slabs, a few at
// a time and sometimes 8 MiB at once, inside calls that have no error path. So the
// conversion asks, before each node it builds, what the arena will allocate to serve
// it (ccoreArenaNeed), and reserves that. The functions below mirror the slab choices
// of arena.go (allocNodeSlow, allocNodeSliceInternal, allocFieldIDSlice,
// allocFieldSourceSlice) and of node_field_metadata.go (allocNodeFieldMetadata);
// TestCcoreArenaSlabsAreReservedAtTheirRealSize and TestCcoreLedgerEqualsTheRealAllocation
// compare them with the arena's own counter and with the heap.

// ccoreNodeSide is the bytes of the two side arrays that a node array of l nodes can
// grow (supertype bytes, column-dependency bits), counted at full size whether or not
// they are ever written (at most 1.08% of the node bytes).
func ccoreNodeSide(l int) int64 { return int64(l) + 8*int64(dependsOnColumnWords(l)) }

// ccoreColdArenaBytes is the most a cold full arena makes when it is acquired: the
// four first slabs and the side arrays of its node array.
func ccoreColdArenaBytes() int64 {
	nodes := nodeCapacityForClass(arenaClassFull)
	return nodeBytesForCap(nodes) + childSliceBytesForCap(fullChildSliceCap) +
		fieldSliceBytesForCap(fullFieldSliceCap) + fieldSourceSliceBytesForCap(fullFieldSliceCap) + ccoreNodeSide(nodes)
}

// ccoreArenaSide is the side-array bytes of the node arrays an arena holds now.
func ccoreArenaSide(a *nodeArena) int64 {
	if a == nil {
		return 0
	}
	n := ccoreNodeSide(len(a.nodes))
	for i := range a.nodeSlabs {
		n += ccoreNodeSide(len(a.nodeSlabs[i].data))
	}
	return n
}

// ccoreSliceSlabNeed is the bytes a slab allocator makes to hand out n elements:
// 0 when a slab from the cursor on has room, else a new slab of max(def, n)
// elements (and the default slab first when there is none).
func ccoreSliceSlabNeed(count, cursor int, room func(i int) int, n, def int, elem int64) int64 {
	if count == 0 {
		need := int64(def) * elem
		if def < n {
			need += int64(n) * elem
		}
		return need
	}
	if cursor < 0 || cursor >= count {
		cursor = 0
	}
	for i := cursor; i < count; i++ {
		if room(i) >= n {
			return 0
		}
	}
	return int64(max(def, n)) * elem
}

// ccoreArenaNeed returns the bytes the arena will allocate to serve one node, a child
// slice of kids pointers and, when fids > 0, a field-id slice, a field-source slice
// and one field-metadata record. It reads the arena and changes nothing.
func ccoreArenaNeed(a *nodeArena, kids, fids int) int64 {
	if a == nil {
		return 0
	}
	var need int64
	if a.used >= len(a.nodes) { // allocNodeFast fails: allocNodeSlow
		switch {
		case len(a.nodeSlabs) == 0:
			c := max(nodeCapacityForClass(a.class), minArenaNodeCap)
			need += nodeBytesForCap(c) + ccoreNodeSide(c)
		default:
			cur := a.nodeSlabCursor
			if cur < 0 || cur >= len(a.nodeSlabs) {
				cur = 0
			}
			room := false
			for i := cur; i < len(a.nodeSlabs); i++ {
				if a.nodeSlabs[i].used < len(a.nodeSlabs[i].data) {
					room = true
					break
				}
			}
			if !room {
				c := boundedNextSlabCap(len(a.nodeSlabs[len(a.nodeSlabs)-1].data), minArenaNodeCap, int(unsafe.Sizeof(Node{})))
				need += nodeBytesForCap(c) + ccoreNodeSide(c)
			}
		}
	}
	if kids > 0 {
		need += ccoreSliceSlabNeed(len(a.childSlabs), a.childSlabCursor, func(i int) int { return len(a.childSlabs[i].data) - a.childSlabs[i].used },
			kids, defaultChildSliceCap(a.class), int64(unsafe.Sizeof((*Node)(nil))))
	}
	if fids > 0 {
		need += ccoreSliceSlabNeed(len(a.fieldSlabs), a.fieldSlabCursor, func(i int) int { return len(a.fieldSlabs[i].data) - a.fieldSlabs[i].used },
			fids, defaultFieldSliceCap(a.class), int64(unsafe.Sizeof(FieldID(0))))
		need += ccoreSliceSlabNeed(len(a.fieldSourceSlabs), a.fieldSourceSlabCursor, func(i int) int { return len(a.fieldSourceSlabs[i].data) - a.fieldSourceSlabs[i].used },
			fids, defaultFieldSliceCap(a.class), 1)
		// one field-metadata record
		ms := a.nodeFieldMetadataSlabs
		msz := int64(unsafe.Sizeof(nodeFieldMetadata{}))
		if len(ms) == 0 {
			need += int64(defaultNodeFieldMetadataSlabCap(a.class)) * msz
		} else {
			cur := a.nodeFieldMetadataSlabCursor
			if cur < 0 || cur >= len(ms) {
				cur = 0
			}
			room := false
			for i := cur; i < len(ms); i++ {
				if ms[i].used < len(ms[i].data) {
					room = true
					break
				}
			}
			if !room {
				need += int64(boundedNextSlabCap(len(ms[len(ms)-1].data), minArenaNodeCap, int(msz))) * msz
			}
		}
	}
	return need
}

// reserveArena reserves what the arena will make to serve one node (ccoreArenaNeed),
// before the node is made. It counts the reservation in c.arenaCharged.
func (c *ccoreConv) reserveArena(kids, fids int) bool {
	if !c.l().Active() {
		return true
	}
	n := ccoreArenaNeed(c.arena, kids, fids)
	if n == 0 {
		return true
	}
	if !c.reserve(n) {
		return false
	}
	c.arenaCharged += n
	return true
}

// ccoreLink (seam 2) is the link from a public tree to its core tree, nil for a
// tree without one. Rule: the core tree belongs to the tree's arena; the arena's
// last release (nodeArena.Release) releases it, Tree.Copy takes a copy of it
// (Copy of the core tree retains the root), and a public tree dropped without
// Release is covered by the core tree's finalizer. Nobody else releases it.
func (t *Tree) ccoreLink() *ccore.Tree {
	if st := t.ccoreStateOf(); st != nil {
		return st.ct
	}
	return nil
}

// ccoreDiffChangedRanges calls the port of ts_tree_get_changed_ranges
// (C/tree.c:114) without materializing public node views.
func ccoreDiffChangedRanges(oldTree, newTree *Tree) ([]Range, bool) {
	old, next := oldTree.ccoreStateOf(), newTree.ccoreStateOf()
	if old == nil || next == nil || old.tables == nil || old.tables != next.tables ||
		old.broken || next.broken || old.edits != len(oldTree.edits) || next.edits != len(newTree.edits) {
		return nil, false
	}
	ranges, ok := ccore.ChangedRanges(old.ct, next.ct, old.tables,
		ccoreRangesOf(oldTree.includedRanges), ccoreRangesOf(newTree.includedRanges))
	if len(ranges) == 0 {
		return nil, ok
	}
	out := make([]Range, len(ranges))
	for i, r := range ranges {
		out[i] = Range{
			StartByte: r.StartByte, EndByte: r.EndByte,
			StartPoint: Point{Row: r.StartRow, Column: r.StartCol},
			EndPoint: Point{Row: r.EndRow, Column: r.EndCol},
		}
	}
	return out, true
}

func ccoreRangesOf(ranges []Range) []ccore.Range {
	if len(ranges) == 0 {
		return nil
	}
	out := make([]ccore.Range, len(ranges))
	for i, r := range ranges {
		out[i] = ccore.Range{
			StartByte: r.StartByte, EndByte: r.EndByte,
			StartRow: r.StartPoint.Row, StartCol: r.StartPoint.Column,
			EndRow: r.EndPoint.Row, EndCol: r.EndPoint.Column,
		}
	}
	return out
}

// ccoreRoot is the core root of the public tree after the edits carried so far
// (seam 3, read side); ok is false when the tree has no usable core tree.
func (t *Tree) ccoreRoot() (root ccore.Handle, ok bool) {
	ct := t.ccoreLink()
	if ct == nil || ct.Released() {
		return 0, false
	}
	return ct.Root(), true
}

// releaseCore gives up the core tree (the arena's last release).
func (a *ccoreArenaState) releaseCore() {
	if a != nil && a.ct != nil {
		a.ct.Release()
		a.ct = nil
	}
}

// forTreeCopy is the arena state of a Tree.Copy: forCopy plus a retained copy of
// the core tree (C ts_tree_copy), with the same edits carried.
func (a *ccoreArenaState) forTreeCopy() *ccoreArenaState {
	c := a.forCopy()
	if c == nil {
		return nil
	}
	c.edits, c.curLen, c.broken, c.budget, c.viewOrigin = a.edits, a.curLen, a.broken, a.budget, a.viewOrigin
	if a.ct != nil && !a.ct.Released() {
		c.ct = a.ct.Copy()
	} else {
		c.broken = true
	}
	return c
}

func (p *Parser) ccoreRun(adm ccoreAdmission, source []byte, old *ccore.Tree) (tree *Tree, ok bool, redo bool) {
	st, blob := adm.st, adm.sha
	lang := p.language
	cp, err := st.get(lang)
	if err != nil {
		ccoreDecline(ccoreDeclNoBinding)
		return nil, false, false
	}
	keep := true // the parser may go back to the pool
	stopped := ParseStopNone
	converting := false
	defer func() {
		if r := recover(); r != nil {
			tree, ok, redo, keep = nil, false, false, false
			ccoreDecline(ccoreDeclPanic)
		}
		func() {
			defer func() { _ = recover() }() // a scanner that panics again while closing
			cp.Ledger().SetPoll(nil)
			cp.FinishScanner()
		}()
		if stopped == ParseStopMemoryBudget || cp.BufferBytes() > ccorePoolKeepBytes {
			keep = false
		}
		if keep {
			cp.Reset() // the arena is reset before the call returns
			if h := ccoreTestParserHook.Load(); h != nil {
				(*h)(cp, false)
			}
			st.pool.Put(cp)
		}
	}()
	cp.SetReduceCheck(ccoreTestReduceCheck.Load())
	// An explicit certificate can cover shortcut reuse independently of
	// the fresh forest route. Historical certificates retain their route.
	shortcutQualified := adm.modes&certified.Incr != 0 &&
		(adm.shortcut == certified.ShortcutTokenStable ||
			adm.shortcut == certified.ShortcutDefault && adm.forestElection == nil && adm.modes&certified.Forest == 0)
	cp.SetShortcut(ccoreViewsOn.Load() && shortcutQualified && !ccoreTestShortcutOff.Load())
	if h := ccoreTestParserHook.Load(); h != nil {
		(*h)(cp, true)
	}
	lim := p.ccoreLimitsFor(len(source))
	if adm.automaticForest {
		lim.budget = automaticForestMemoryBudget(p, lim.budget)
	}
	if lim.active() {
		restoreHeap := p.enterRuntimeMemoryBudget(lim.budget, len(source)) // the hard ceiling's heap-growth backstop
		defer restoreHeap.restore()
		// One ledger serves the core and the conversion: every array either allocates
		// is reserved in it before it is made, and this closure is the one limit check.
		cp.Ledger().SetPoll(func(total int64, sample bool) bool {
			if h := ccoreTestPollHook.Load(); h != nil {
				(*h)(total, sample, converting)
			}
			if r := p.ccorePoll(lim, uint64(total), sample); r != ParseStopNone {
				stopped = r
				return false
			}
			return true
		})
		// Between two forced heap samples at most as many scanner callbacks run as the
		// present engine lets dispatches run between two of its own samples.
		every := int(runtimeMemoryPollMask(lim.budget)) + 1
		if o := ccoreTestScanEvery.Load(); o != 0 {
			every = int(o)
		}
		cp.SetScanCheckEvery(every)
	}
	t0 := nowNanos()
	var scan ccore.ScanProvider // nil: the core walks its own automaton
	var tableScan *ccore.TableScan
	if adm.provider == ccore.TableScanID {
		tableScan = ccore.NewTableScan(st.base, source)
		scan = tableScan
	}
	ct, stop, resultErr := ccoreParseCore(cp, old, source, scan, p.ccoreIncludedRanges(), adm.forestElection)
	if adm.forestElection != nil && stop != "" {
		adm.forestElection.Stop = stop
	}
	if tableScan != nil {
		ccoreProviderScans.Add(int64(tableScan.Calls()))
	}
	t1 := nowNanos()
	ccoreParseNanos.Add(t1 - t0)
	retained := false // the public tree keeps ct; every other exit releases it
	defer func() {
		if ct != nil && !retained {
			ct.Release()
		}
	}()
	if stop != "" || ct == nil || ct.Root() == 0 {
		if stopped != ParseStopNone {
			ccoreStopDecline(stopped)
		} else if !ccoreHandledForestDeadEnd(adm.forestElection) {
			ccoreDecline(ccoreDeclCoreStop)
		}
		return nil, false, false
	}

	if resultErr {
		ccoreIncrFallback(ccoreIncrResultError)
		return nil, false, true
	}

	// The parser's arena and buffers stay live while the public tree is built; the
	// conversion reserves its own arrays in the same ledger, on top of the core's.
	var end ccoreConvEnd
	converting = true
	var included []Range
	if len(p.included) > 0 {
		var ok bool
		included, ok = ccore.Make[Range](cp.Ledger(), len(p.included))
		if !ok {
			ccoreStopDecline(stopped)
			return nil, false, false
		}
		copy(included, p.included)
	}
	var built *Tree
	var tooDeep bool
	if ccoreViewsOn.Load() {
		built = ccoreViewTree(ct, source, lang, blob, st, cp.Ledger(), &end)
	} else {
		built, tooDeep = ccoreConvertTree(ct, source, lang, blob, st, cp.Ledger(), &end)
	}
	ccoreConvertNanos.Add(nowNanos() - t1)
	if h := ccoreTestAccountHook.Load(); h != nil {
		(*h)(cp.Ledger().Base, cp.Ledger().Alloc, end.arenaCharged, end.arenaHeld)
	}
	if built == nil {
		if tooDeep {
			ccoreDecline(ccoreDeclDepthLimit)
		} else {
			ccoreStopDecline(stopped)
		}
		return nil, false, false
	}
	retained = true
	built.includedRanges = included
	if cp.ShortcutTaken() {
		ccoreShortcutReparses.Add(1)
	}
	built.ccoreStateOf().budget = lim.budget
	built.ccoreStateOf().incremental = old != nil
	built.ccoreStateOf().reusedNodes, built.ccoreStateOf().reusedBytes, _ = cp.ReuseStats()
	rt := built.ensureParseRuntime()
	rt.StopReason = ParseStopAccepted
	rt.SourceLen = uint32(len(source))
	rt.ExpectedEOFByte = uint32(len(source))
	if n := len(included); n != 0 {
		rt.ExpectedEOFByte = included[n-1].EndByte
	}
	if root := built.root; root != nil {
		rt.RootEndByte = root.endByte
	}
	rt.IncrementalOldTreeReuseRoute = old != nil
	rt.MemoryBudgetBytes = lim.budget
	rt.MaxStacksSeen = cp.MaxVersions()
	rt.TokensConsumed = uint64(cp.Tokens())
	rt.MultiStackTokens = uint64(cp.MultiTokens())
	rt.SingleStackTokens = rt.TokensConsumed - rt.MultiStackTokens
	if cp.ScannedBuiltinEnd() {
		rt.Truncated = rt.RootEndByte < rt.ExpectedEOFByte
		if rt.Truncated && rt.StopReason == ParseStopAccepted {
			rt.StopReason = ParseStopAcceptedPrefix
		}
		markTruncatedTreeHasError(*rt, built.root)
	}
	return built, true, false
}

// ccoreFullParse serves a fresh parse when the core admits the request.
func (p *Parser) ccoreFullParse(source []byte) (tree *Tree, ok bool) {
	return p.ccoreFullParseProvider(source, "")
}

// ccoreFullParseProvider is ccoreFullParse for a request that asks the core to
// use the scan provider id ("" for the core's own walk).
func (p *Parser) ccoreFullParseProvider(source []byte, provider string) (tree *Tree, ok bool) {
	if ccoreUnset() {
		return nil, false
	}
	defer func() { // admission and parser acquisition are inside the recovery boundary
		if r := recover(); r != nil {
			tree, ok = nil, false
			ccoreDecline(ccoreDeclPanic)
		}
	}()
	adm, ok := p.ccoreAdmitProvider(provider)
	if !ok {
		return nil, false
	}
	tree, ok, _ = p.ccoreRun(adm, source, nil)
	if ok {
		ccoreServedFresh.Add(1)
	}
	return tree, ok
}

// ccoreNeverServes is the first test of a reparse without edits, small enough to
// be inlined at the call sites because such a reparse takes a few nanoseconds.
// It is true when the language's blob has no certificate and no test
// certification was ever installed in this process. The core serves only
// certified blobs, so it never served this language and the old tree is not one
// of its trees, whatever the candidate switch says: the tree may be returned.
// The certificate answer is the one ccoreBlobCertified keeps on the language;
// until it has been asked once this test is false and the caller takes
// ccoreUnchangedTreeAllowed.
func (p *Parser) ccoreNeverServes() bool {
	return p.language != nil && atomic.LoadUint32(&p.language.ccoreCertState) == 1 && !certified.EverTest()
}

// ccoreUnchangedTreeAllowed is the full decision for a reparse without edits.
// Through ccoreRouteEnabled it also records whether the language's blob has a
// certificate, which ccoreNeverServes reads.
func (p *Parser) ccoreUnchangedTreeAllowed(oldTree *Tree, usingProductionDFA bool) bool {
	if usingProductionDFA {
		return p.ccoreUnchangedTreeAllowedFor(oldTree, ccoreSourceDFA)
	}
	return p.ccoreUnchangedTreeAllowedFor(oldTree, ccoreSourceStream)
}

// ccoreUnchangedTreeAllowedFor is ccoreUnchangedTreeAllowed for a kind of token producer.
func (p *Parser) ccoreUnchangedTreeAllowedFor(oldTree *Tree, kind ccoreSource) bool {
	st := oldTree.ccoreStateOf()
	if st == nil && !p.ccoreRouteEnabled() {
		return true
	}
	if ccoreUnset() {
		return st == nil
	}
	if !p.ccoreEligibleForReparse(kind, oldTree) { // a request with an ordinary caller token source is the present engine's
		return st == nil
	}
	adm, _, ok := p.ccoreAdmitQuiet()
	if !ok {
		return st == nil
	}
	return st != nil && st.binding == adm.binding
}

// ccoreFirstFresh is the first statement of a fresh parse: when the candidate
// route would be asked and the core admits the request, the core serves it before
// any shortcut of the present engine. A decline falls back to the existing route.
func (p *Parser) ccoreFirstFresh(source []byte) (*Tree, bool) {
	if ccoreUnset() || p.ccoreSuppressed() || p.recoveryInitialOnly || !p.ccoreEligible(true) {
		return nil, false
	}
	tree, ok := p.ccoreFullParse(source)
	if ok && (p.forestDeclineMemo == nil || !p.forestDeclineMemo.admissionCountersSuppressed) {
		admissionCandidateRouted.Add(1) // as attemptAdmissionCandidateFullParse counts a request the candidate route served
	}
	return tree, ok
}

// ccoreParseCore parses with an optional old core tree.
func ccoreParseCore(cp *ccore.Parser, old *ccore.Tree, source []byte, scan ccore.ScanProvider, included []ccore.Range, forest *ccore.ForestElection) (ct *ccore.Tree, stop string, resultErr bool) {
	if forest != nil {
		ct, stop = cp.ParseFreshForest(source, forest)
	} else if len(included) != 0 {
		ct, stop = cp.ParseWithIncludedScan(source, old, included, scan)
	} else {
		ct, stop = cp.ParseWithScan(source, old, scan)
	}
	if stop != "" || ct == nil || ct.Root() == 0 {
		return ct, stop, false
	}
	return ct, "", old != nil && ct.Reader().ErrorCost(ct.Root()) > 0
}

// ccoreEditsDescribe reports whether the edits, applied in order to oldSrc,
// can give newSrc: the final length matches, and every stretch of the old text
// that no edit replaced is equal, byte for byte, in the new text (the prefix,
// each gap between two edits, and the suffix). It keeps a wrong edit list (or an
// edit that never reached the tree, such as Node.Edit) from reaching the core,
// which trusts the caller as C does, and a change between two edits from hiding
// behind the outer window of the list.
//
// The edits are in the order they were made, each in the coordinates of the text
// the earlier ones left. The text is kept as a list of segments, each either a
// stretch of the old text (with its offset there) or text an edit wrote; an edit
// cuts the list at its two ends and puts one written segment in place of the
// cut-out part. Edits that overlap (one reaches text that an earlier one wrote; an edit
// that only touches it, ending where the other starts, does not) are not described. A
// list longer than ccoreMaxDescribedEdits is not described.
func ccoreEditsDescribe(oldSrc, newSrc []byte, edits []InputEdit) bool {
	if len(edits) > ccoreMaxDescribedEdits {
		return false
	}
	type seg struct {
		old    bool
		off, n int64 // offset in oldSrc (old segments) and length
	}
	// Two fixed arrays, swapped for every edit: an edit adds at most two segments (the cut
	// splits one segment in two, and the written text is one more), so the list never holds more
	// than 2*len(edits)+1. Nothing is allocated, so the check needs no memory accounting.
	var bufA, bufB [2*ccoreMaxDescribedEdits + 3]seg
	segs, out := bufA[:0], bufB[:0]
	n0 := int64(len(oldSrc))
	segs = segs[:1]
	segs[0] = seg{old: true, off: 0, n: n0}
	total := n0
	for _, e := range edits {
		s, oe, ne := int64(e.StartByte), int64(e.OldEndByte), int64(e.NewEndByte)
		if s > oe || s > ne || oe > total {
			return false
		}
		out = out[:0]
		pos := int64(0)
		inserted := false
		put := func() {
			if !inserted {
				inserted = true
				if ne > s {
					out = out[:len(out)+1]
					out[len(out)-1] = seg{n: ne - s}
				}
			}
		}
		for _, g := range segs {
			gs, ge := pos, pos+g.n
			pos = ge
			switch {
			case ge <= s: // wholly before the edit
				out = out[:len(out)+1]
				out[len(out)-1] = g
			case gs >= oe: // wholly after it
				put()
				out = out[:len(out)+1]
				out[len(out)-1] = g
			default: // cut by the edit: keep what lies before s and after oe
				if !g.old { // the edit reaches text that an earlier edit wrote: overlapping edits are not described
					return false
				}
				if gs < s {
					left := g
					left.n = s - gs
					out = out[:len(out)+1]
					out[len(out)-1] = left
				}
				put()
				if ge > oe {
					right := g
					if g.old {
						right.off += oe - gs
					}
					right.n = ge - oe
					out = out[:len(out)+1]
					out[len(out)-1] = right
				}
			}
		}
		put() // an edit at the very end
		segs, out = out, segs
		total += ne - oe
	}
	if total != int64(len(newSrc)) {
		return false
	}
	pos := int64(0)
	for _, g := range segs {
		if g.old && !bytes.Equal(oldSrc[g.off:g.off+g.n], newSrc[pos:pos+g.n]) {
			return false
		}
		pos += g.n
	}
	return true
}

// ccoreMaxDescribedEdits bounds the work and the fixed arrays of ccoreEditsDescribe
// (quadratic in the number of edits); a longer list is answered by a fresh parse.
const ccoreMaxDescribedEdits = 64

// Reasons for which a reparse that the core serves is a fresh core parse.
const (
	ccoreIncrNotCertified    = iota // the blob's certificate lacks the incr mode
	ccoreIncrForeign                // the old tree has no core tree: made by another engine
	ccoreIncrReleased               // the old public tree or its core tree is released
	ccoreIncrOtherBinding           // the core tree was made with another binding (scanner, tables) than this request's
	ccoreIncrOtherLanguage          // the old tree belongs to another language
	ccoreIncrEditsNotCarried        // an edit of the public tree did not reach the core tree
	ccoreIncrEditMismatch           // the recorded edits do not turn the old source into the new source
	ccoreIncrOldHasError            // the core tree has an error (C's error cost above zero)
	ccoreIncrResultError            // the incremental result has an error: a fresh parse decides
	ccoreIncrEncoding               // the old tree's source is not UTF-8
	ccoreIncrCount
)

var ccoreIncrFallbackNames = [ccoreIncrCount]string{
	"incr_not_certified", "old_tree_foreign", "old_tree_released", "old_other_binding", "old_other_language",
	"edits_not_carried", "edit_mismatch", "old_has_error", "result_has_error", "old_encoding",
}

var (
	ccoreIncrFallbacks [ccoreIncrCount]atomic.Int64
	ccoreServedIncr    atomic.Int64 // reparses the core served incrementally
)

func ccoreIncrFallback(reason int) { ccoreIncrFallbacks[reason].Add(1) }

// ccoreOldFor takes the old public tree's link, its recorded
// edits and the new source, and returns the core tree the reparse may read, or a reason for
// which the reparse is a fresh core parse.
func (p *Parser) ccoreOldFor(adm ccoreAdmission, old *Tree, source []byte) (*ccore.Tree, int) {
	if adm.modes&certified.Incr == 0 {
		return nil, ccoreIncrNotCertified
	}
	if old == nil || old.released {
		return nil, ccoreIncrReleased
	}
	if len(old.includedRanges) > 0 && adm.modes&ccoreModeRanges == 0 {
		return nil, ccoreIncrNotCertified
	}
	st := old.ccoreStateOf()
	if st == nil || st.ct == nil {
		return nil, ccoreIncrForeign
	}
	switch {
	case st.forestElection && (!st.forestOrdinaryCompatible || adm.provider != ""):
		return nil, ccoreIncrNotCertified
	case old.language != p.language:
		return nil, ccoreIncrOtherLanguage
	case st.binding != adm.binding || st.tables != adm.st.base:
		return nil, ccoreIncrOtherBinding
	case st.ct.Released():
		return nil, ccoreIncrReleased
	case old.sourceEncoding != InputEncodingUTF8:
		return nil, ccoreIncrEncoding
	case st.broken || st.edits != len(old.edits):
		return nil, ccoreIncrEditsNotCarried
	case !ccoreEditsDescribe(old.source, source, old.edits):
		return nil, ccoreIncrEditMismatch
	}
	// "Has an error" is C's test: the error cost of the stored root is above zero
	// (ts_subtree_error_cost(root) > 0, the same value that sets HasError).
	if rd := st.ct.Reader(); rd.ErrorCost(st.ct.Root()) > 0 {
		return nil, ccoreIncrOldHasError
	}
	return st.ct, -1
}

// setCcoreIncludedRanges translates the normalized ranges at configuration
// time. A parse can borrow them without allocating an adapter slice.
func (p *Parser) setCcoreIncludedRanges(ranges []Range) {
	if len(ranges) == 0 {
		if cold := p.forestDeclineMemo; cold != nil {
			cold.ccoreIncluded = nil
		}
		return
	}
	out := make([]ccore.Range, len(ranges))
	for i, r := range ranges {
		out[i] = ccore.Range{StartByte: r.StartByte, EndByte: r.EndByte,
			StartRow: r.StartPoint.Row, StartCol: r.StartPoint.Column,
			EndRow: r.EndPoint.Row, EndCol: r.EndPoint.Column}
	}
	p.ensureParserColdState().ccoreIncluded = out
}

func (p *Parser) ccoreIncludedRanges() []ccore.Range {
	if len(p.included) != 0 && p.forestDeclineMemo != nil {
		return p.forestDeclineMemo.ccoreIncluded
	}
	return nil
}

// ccoreEditOf is a public edit as the core's edit.
func ccoreEditOf(edit InputEdit) ccore.Edit {
	return ccore.Edit{
		StartByte: edit.StartByte, OldEndByte: edit.OldEndByte, NewEndByte: edit.NewEndByte,
		StartRow: edit.StartPoint.Row, StartCol: edit.StartPoint.Column,
		OldEndRow: edit.OldEndPoint.Row, OldEndCol: edit.OldEndPoint.Column,
		NewEndRow: edit.NewEndPoint.Row, NewEndCol: edit.NewEndPoint.Column,
	}
}

// ccoreEdit forwards a public edit to its linked core tree.
func (t *Tree) ccoreEdit(edit InputEdit) bool {
	st := t.ccoreStateOf()
	if st == nil || st.ct == nil || st.broken {
		return false
	}
	started := nowNanos()
	defer func() { ccoreEditNanos.Add(nowNanos() - started) }()
	if edit.StartByte > edit.OldEndByte || edit.StartByte > edit.NewEndByte || int64(edit.OldEndByte) > st.curLen {
		st.broken = true
		return false
	}
	ok := false
	defer func() {
		if r := recover(); r != nil {
			st.broken = true
		}
	}()
	ce := ccoreEditOf(edit)
	if st.budget > 0 {
		// The pages the edit claims are charged against the budget of the request that
		// built the tree, before they are claimed. A refusal poisons the core tree's store
		// (the store's rule for a failed edit: its next parse is a fresh parse in a new
		// store) and leaves its root as it was; the link is marked broken, so the next
		// reparse of the public tree is a fresh parse by the core (edits_not_carried), not
		// an incremental one. The public tree stays readable.
		var led ccore.Ledger
		budget := st.budget
		led.SetPoll(func(total int64, _ bool) bool { return total < budget })
		if !led.Begin(int64(st.ct.PageBytes())) || !st.ct.EditWithin(ce, &led) {
			st.broken = true
			return false
		}
	} else {
		st.ct.Edit(ce)
	}
	st.edits++
	st.curLen += int64(edit.NewEndByte) - int64(edit.OldEndByte)
	ok = true
	return ok
}

// ccoreReparse serves a reparse request for oldTree: incrementally when the
// old tree has a usable core tree, else as a fresh core parse. It answers
// false, with the reason counted, when the present engine must run.
func (p *Parser) ccoreReparse(source []byte, old *Tree) (tree *Tree, ok bool) {
	return p.ccoreReparseProvider(source, old, "")
}

// ccoreReparseProvider is ccoreReparse for a request that asks the core to use
// the scan provider id ("" for the core's own walk).
func (p *Parser) ccoreReparseProvider(source []byte, old *Tree, provider string) (tree *Tree, ok bool) {
	kind := ccoreSourceDFA
	if provider != "" {
		kind = ccoreSourceProvider
	}
	if ccoreUnset() || !p.ccoreEligibleForReparse(kind, old) {
		return nil, false
	}
	defer func() { // admission and old-tree tests are inside the recovery boundary
		if r := recover(); r != nil {
			tree, ok = nil, false
			ccoreDecline(ccoreDeclPanic)
		}
	}()
	adm, ok := p.ccoreAdmitProvider(provider)
	if !ok {
		return nil, false
	}
	checkStart := nowNanos()
	oldCT, reason := p.ccoreOldFor(adm, old, source)
	ccoreCheckNanos.Add(nowNanos() - checkStart)
	if oldCT != nil {
		tree, ok, redo := p.ccoreRun(adm, source, oldCT)
		if ok {
			if old.ccoreStateOf().forestOrdinaryCompatible {
				rt := tree.ensureParseRuntime()
				rt.CompactIncrementalReuseRoute = true
				rt.CompactIncrementalReusedSubtrees = uint64(tree.ccoreStateOf().reusedNodes)
				rt.CompactIncrementalReusedBytes = tree.ccoreStateOf().reusedBytes
			}
			ccoreServedIncr.Add(1)
			return tree, true
		}
		if !redo {
			return nil, false
		}
		// ccoreRun counted the reason; the fresh parse below decides.
	} else {
		ccoreIncrFallback(reason)
	}
	tree, ok, _ = p.ccoreRun(adm, source, nil)
	if ok {
		ccoreServedFresh.Add(1)
	}
	return tree, ok
}

// Lazy public nodes for a tree of internal/ccore.
//
// With the views switch on, a parse of the core builds a public Tree whose root
// is a view and converts nothing. A view is an ordinary Node header, made when
// a caller first reads the node, behind the nodeViewBacking seam. Its children,
// parent and fields come from the core's records through value nodes
// (internal/ccore/node.go). One table per public tree maps the slot that holds a
// node's handle to its header, so a node has one header for the life of its tree.
//
// Header fields of a view: symbol and flags as the eager conversion
// (ccore_bind.go) sets them; rawShape is the slot key; equivVersion is the
// ordinal among the visible children of the parent; dynamicPrecedence holds the
// supertype mask; preGotoState is the record's parse state; parent is set when
// the view is made; children stays nil. Tree.Edit with no view below the root
// updates the root header only. With views below the root it refreshes the
// headers that exist from the edited core tree and makes none (the sparse
// rebind, ccoreEditSparse); when it cannot, it converts the tree in place first
// (materialize). Either way a node a caller holds stays correct.

// ccoreViewsOn controls lazy node views; tests can turn them off.
var ccoreViewsOn atomic.Bool

var ccoreTestShortcutOff atomic.Bool
var ccoreShortcutReparses atomic.Int64

// Reasons for a whole-tree conversion of a view tree, counted in ccoreViewConversions.
const (
	ccoreViewConvEditWithViews = iota // Tree.Edit with a view below the root
	ccoreViewConvNodeEdit             // Node.Edit
	ccoreViewConvAdopt                // a view adopted into a hand-built tree
	ccoreViewConvOffsetRoot           // RootNodeWithOffset
	ccoreViewConvTest                 // asked by a test
	ccoreViewConvOther                // any other caller of materialize
	ccoreViewConvCount
)

var ccoreViewConvNames = [ccoreViewConvCount]string{"tree_edit_with_views", "node_edit", "adopt", "offset_root", "test", "other"}

var (
	ccoreViewConversions [ccoreViewConvCount]atomic.Int64
	ccoreViewsMade       atomic.Int64 // headers made by views of all trees
	ccoreViewTrees       atomic.Int64 // view trees built
	ccoreViewRootEdits   atomic.Int64 // Tree.Edit calls served by the root header alone
	ccoreViewSparseEdits atomic.Int64 // Tree.Edit calls served by the sparse rebind
	ccoreViewSparseMoved atomic.Int64 // headers the sparse rebind refreshed
	ccoreViewSparseStops [ccoreSparseStopCount]atomic.Int64
	ccoreTestSparseOff   atomic.Bool // tests: Tree.Edit with views takes the conversion
)

// Reasons for which Tree.Edit with a view below the root did not take the
// sparse rebind and converted the tree, counted in ccoreViewSparseStops.
const (
	ccoreSparseStopOff        = iota // switched off by a test
	ccoreSparseStopLink              // no usable core tree: the link is broken or the edit is out of bounds
	ccoreSparseStopRefused           // the budget refused the staged edit, or the core's edit failed
	ccoreSparseStopDuplicates        // a child slot has views of two occurrences
	ccoreSparseStopMismatch          // the edited tree and the views do not match, or a new slot is taken
	ccoreSparseStopCount
)

var ccoreSparseStopNames = [ccoreSparseStopCount]string{
	"sparse_stop_off", "sparse_stop_link", "sparse_stop_refused", "sparse_stop_duplicates", "sparse_stop_mismatch",
}

const ccoreViewPageSize = 128

type ccoreViewPage [ccoreViewPageSize]atomic.Pointer[Node]

type ccoreViewTop struct {
	pages []atomic.Pointer[ccoreViewPage]
}

const ccoreLargeKey = uint32(1) << 31 // rawShape of a child of a list that has a page of its own

type ccoreViews struct {
	rd    ccore.NodeReader
	core  *ccore.Tree
	lang  *Language
	arena *nodeArena
	root  *Node

	mu         sync.Mutex
	top        atomic.Pointer[ccoreViewTop]
	large      map[[2]uint32]*Node           // (list, index) of a large list -> view; under mu
	largeKeys  [][2]uint32                   // rawShape&^ccoreLargeKey -> (list, index); under mu
	pageBytes  uint64                        // under mu
	duplicates map[ccoreViewOccurrence]*Node // repeated slots, under mu

	made     atomic.Int64 // views made by this backing
	released atomic.Bool
}

type ccoreViewOccurrence struct {
	parent              *Node
	list, slot, ordinal uint32
}

var _ nodeViewBacking = (*ccoreViews)(nil)

// ccoreViewTree builds the public tree of a core tree without converting it:
// the root is a view. The tree owns ct.
func ccoreViewTree(ct *ccore.Tree, src []byte, lang *Language, blob [32]byte, st *ccoreLanguageTables, led *ccore.Ledger, end *ccoreConvEnd) *Tree {
	if led.Active() && !led.Reserve(ccoreColdArenaBytes()) {
		return nil
	}
	arena := acquireNodeViewArena()
	nodeViewArenaPool.forgetCheckedOut(arena)
	if led.Active() {
		end.arenaCharged = int64(arenaAllocatedVolume(arena)) + ccoreArenaSide(arena)
		if !led.Rebase(ccoreColdArenaBytes(), end.arenaCharged) {
			arena.Release()
			return nil
		}
	}
	done := false
	defer func() {
		if !done {
			arena.Release()
		}
	}()
	arena.ccore = &ccoreArenaState{tables: st.base, binding: st.token, ct: ct, curLen: int64(len(src)), viewOrigin: true}
	v := newCcoreViews(ct, lang, st, arena)
	end.arenaHeld = int64(arenaAllocatedVolume(arena)) + ccoreArenaSide(arena)
	if led.Active() && !led.Check(true) {
		return nil
	}
	markPublishedRootError(v.root)
	t := &Tree{
		root: v.root, source: src, sourceEncoding: InputEncodingUTF8, language: lang, arena: arena,
		parseRuntime:               acquireParseRuntime(),
		incrementalReuseDisabled:   true,
		resultCompatibilityApplied: true,
		dependsOnColumnPropagated:  true,
	}
	// These are the headers and arena volume at publication; later headers are counted by the backing.
	t.parseRuntime.NodesAllocated = int(v.made.Load())
	t.parseRuntime.ArenaBytesAllocated = int64(arenaAllocatedVolume(arena))
	ccoreViewTrees.Add(1)
	done = true
	return t
}

// newCcoreViews makes the backing and the root view of ct and installs it in arena.
func newCcoreViews(ct *ccore.Tree, lang *Language, st *ccoreLanguageTables, arena *nodeArena) *ccoreViews {
	v := &ccoreViews{
		rd:    ccore.NewNodeReader(ct.Reader(), ccoreNodeTablesFor(lang)),
		core:  ct,
		lang:  lang,
		arena: arena,
	}
	arena.views = v
	root := v.rd.RootNode(ct.Root())
	v.root = v.newHeader(root, nil, 0)
	return v
}

// newHeader fills a header from a value node. The caller holds mu, or owns v alone.
func (v *ccoreViews) newHeader(c ccore.Node, parent *Node, key uint32) *Node {
	rd := v.rd
	s := rd.S
	end := rd.End(c)
	n := v.arena.allocNodeFast()
	*n = Node{
		ownerArena:        v.arena,
		parent:            parent,
		startPoint:        Point{Row: c.Start.Row(), Column: c.Start.Col()},
		endPoint:          Point{Row: end.Row(), Column: end.Col()},
		startByte:         c.Start.Bytes(),
		endByte:           end.Bytes(),
		preGotoState:      StateID(s.ParseState(c.H)),
		equivVersion:      c.Ordinal,
		dynamicPrecedence: int32(c.Mask),
		childIndex:        compactViewChildIndex,
		rawShape:          rawShapeRef(key),
		symbol:            Symbol(rd.Symbol(c)),
		productionID:      s.ProductionID(c.H),
	}
	n.setNamed(rd.Named(c))
	n.setExtra(rd.Extra(c))
	n.setMissing(rd.Missing(c))
	n.setHasError(rd.HasError(c))
	if rd.HasChanges(c) {
		n.setDirty(true)
	}
	v.made.Add(1)
	ccoreViewsMade.Add(1)
	return n
}

// slotKey returns the rawShape of the slot a value node was read from, or false
// for a node in a large list (its key is made under mu).
func ccoreSlotKey(c ccore.Node) (uint32, bool) {
	if ccore.LargeSlot(c.List) {
		return 0, false
	}
	return c.Slot, true
}

// lookup returns the view in a small slot, if one was made.
func (v *ccoreViews) lookup(key uint32) *Node {
	top := v.top.Load()
	if top == nil {
		return nil
	}
	pi := int(key / ccoreViewPageSize)
	if pi >= len(top.pages) {
		return nil
	}
	page := top.pages[pi].Load()
	if page == nil {
		return nil
	}
	return page[key%ccoreViewPageSize].Load()
}

// viewFor returns the one header of the node read at c, making it when it does not exist.
func (v *ccoreViews) viewFor(c ccore.Node, parent *Node) *Node {
	if key, ok := ccoreSlotKey(c); ok {
		if n := v.lookup(key); n != nil && n.parent == parent && n.equivVersion == c.Ordinal {
			return n
		}
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.viewForLocked(c, parent)
}

func (v *ccoreViews) viewForLocked(c ccore.Node, parent *Node) *Node {
	if key, ok := ccoreSlotKey(c); ok {
		pi := int(key / ccoreViewPageSize)
		top := v.top.Load()
		if top == nil || pi >= len(top.pages) {
			top = v.growTop(top, pi)
		}
		page := top.pages[pi].Load()
		if page == nil {
			page = new(ccoreViewPage)
			v.pageBytes += uint64(unsafe.Sizeof(*page))
			top.pages[pi].Store(page)
		}
		if n := page[key%ccoreViewPageSize].Load(); n != nil {
			return v.occurrence(c, parent, n, key)
		}
		n := v.newHeader(c, parent, key)
		page[key%ccoreViewPageSize].Store(n)
		return n
	}
	lk := [2]uint32{c.List, c.Slot}
	if n := v.large[lk]; n != nil {
		return v.occurrence(c, parent, n, uint32(n.rawShape))
	}
	if v.large == nil {
		v.large = map[[2]uint32]*Node{}
	}
	key := ccoreLargeKey | uint32(len(v.largeKeys))
	v.largeKeys = append(v.largeKeys, lk)
	n := v.newHeader(c, parent, key)
	v.large[lk] = n
	return n
}

// growTop replaces the page table while retaining its pages. The caller holds mu.
func (v *ccoreViews) growTop(top *ccoreViewTop, pi int) *ccoreViewTop {
	grown := &ccoreViewTop{pages: make([]atomic.Pointer[ccoreViewPage], max(pi+1, 2*lenTop(top)))}
	if top != nil {
		for i := range top.pages {
			grown.pages[i].Store(top.pages[i].Load())
		}
	}
	v.pageBytes += uint64(len(grown.pages)-lenTop(top)) * uint64(unsafe.Sizeof(atomic.Pointer[ccoreViewPage]{}))
	v.top.Store(grown)
	return grown
}

// occurrence keeps distinct headers when a child slot is reached through multiple paths.
// The caller holds mu. Each header retains the slot key for reading the record.
func (v *ccoreViews) occurrence(c ccore.Node, parent, first *Node, key uint32) *Node {
	if first.parent == parent && first.equivVersion == c.Ordinal {
		return first
	}
	k := ccoreViewOccurrence{parent, c.List, c.Slot, c.Ordinal}
	if n := v.duplicates[k]; n != nil {
		return n
	}
	if v.duplicates == nil {
		v.duplicates = make(map[ccoreViewOccurrence]*Node)
	}
	n := v.newHeader(c, parent, key)
	v.duplicates[k] = n
	return n
}

func lenTop(t *ccoreViewTop) int {
	if t == nil {
		return 0
	}
	return len(t.pages)
}

// handleOf reads the handle in the slot of a view.
func (v *ccoreViews) handleOf(n *Node) ccore.Handle {
	if n == v.root {
		return v.core.Root()
	}
	key := uint32(n.rawShape)
	if key&ccoreLargeKey != 0 {
		v.mu.Lock()
		lk := v.largeKeys[key&^ccoreLargeKey]
		v.mu.Unlock()
		return v.rd.S.ChildHandle(lk[0], 0, lk[1])
	}
	return v.rd.S.HandleAt(key)
}

// value rebuilds the value node of a view. It reports false for a released
// backing or an empty slot.
func (v *ccoreViews) value(n *Node) (ccore.Node, bool) {
	if n == nil || v.released.Load() {
		return ccore.Node{}, false
	}
	h := v.handleOf(n)
	if h == 0 {
		return ccore.Node{}, false
	}
	return ccore.Node{
		H:     h,
		Start: ccore.NewLength(n.startByte, n.startPoint.Row, n.startPoint.Column),
		Mask:  uint32(n.dynamicPrecedence),
	}, true
}

func (v *ccoreViews) childCount(n *Node) int {
	c, ok := v.value(n)
	if !ok {
		return 0
	}
	return int(v.rd.ChildCount(c))
}

func (v *ccoreViews) namedChildCount(n *Node) int {
	c, ok := v.value(n)
	if !ok {
		return 0
	}
	return int(v.rd.NamedChildCount(c))
}

func (v *ccoreViews) child(n *Node, i int) *Node {
	if i < 0 || uint64(i) >= 1<<32 {
		return nil
	}
	c, ok := v.value(n)
	if !ok {
		return nil
	}
	k, ok := v.rd.Child(c, uint32(i))
	if !ok {
		return nil
	}
	return v.viewFor(k, n)
}

func (v *ccoreViews) namedChild(n *Node, i int) *Node {
	if i < 0 || uint64(i) >= 1<<32 {
		return nil
	}
	c, ok := v.value(n)
	if !ok {
		return nil
	}
	k, found, quirk := v.rd.NamedChildChecked(c, uint32(i))
	if !quirk {
		if !found {
			return nil
		}
		return v.viewFor(k, n)
	}
	// C's numbering can differ here (an anonymous visible child holds named
	// children, and ts_node_named_child may return a node below it or one past the
	// count). The public tree selects the named nodes among the visible children
	// in sequence, so do that: the node is then the child of n that it is.
	var scratch [16]ccore.Node
	named := 0
	for _, kid := range v.rd.Children(c, scratch[:0]) {
		if v.rd.Named(kid) {
			if named == i {
				return v.viewFor(kid, n)
			}
			named++
		}
	}
	return nil
}

// children returns a new slice of the visible children, making their views.
func (v *ccoreViews) children(n *Node) []*Node {
	c, ok := v.value(n)
	if !ok {
		return nil
	}
	var scratch [16]ccore.Node
	kids := v.rd.Children(c, scratch[:0])
	if len(kids) == 0 {
		return nil
	}
	out := make([]*Node, len(kids))
	for i, k := range kids {
		out[i] = v.viewFor(k, n)
	}
	return out
}

func (v *ccoreViews) parent(n *Node) (*Node, int, bool) {
	if n == nil || n.parent == nil {
		return nil, -1, false
	}
	return n.parent, int(n.equivVersion), true
}

func (v *ccoreViews) supertype(n *Node) uint32 { return uint32(n.dynamicPrecedence) }

// grammarSymbol is the record's symbol, which an alias replaces in the header.
func (v *ccoreViews) grammarSymbol(n *Node) ccore.Symbol {
	if v.released.Load() {
		return 0
	}
	h := v.handleOf(n)
	if h == 0 {
		return 0
	}
	return v.rd.S.Symbol(h)
}

// fieldIDAt is the field of the i-th visible child: ts_node_field_name_for_child.
func (v *ccoreViews) fieldIDAt(n *Node, i int) FieldID {
	c, ok := v.value(n)
	if !ok || i < 0 {
		return 0
	}
	k, ok := v.rd.Child(c, uint32(i))
	if !ok {
		return 0
	}
	return FieldID(k.Field)
}

// cursorFieldAt is the field of the i-th visible child as the tree cursor
// reports it: ts_tree_cursor_current_field_id.
func (v *ccoreViews) cursorFieldAt(n *Node, i int) FieldID {
	c, ok := v.value(n)
	if !ok || i < 0 {
		return 0
	}
	k, ok := v.rd.Child(c, uint32(i))
	if !ok {
		return 0
	}
	return FieldID(k.CField)
}

// cursorChildByField walks the visible children once, using cursor field semantics.
func (v *ccoreViews) cursorChildByField(n *Node, fid FieldID) (*Node, int) {
	c, ok := v.value(n)
	if !ok {
		return nil, -1
	}
	var scratch [16]ccore.Node
	for i, child := range v.rd.Children(c, scratch[:0]) {
		if FieldID(child.CField) == fid {
			return v.viewFor(child, n), i
		}
	}
	return nil, -1
}

// childByField is ts_node_child_by_field_id: the search the eager conversion
// records, run when asked (ccore.NodeReader.FieldPath). It returns positions, so
// equal inline handles in two slots stay distinct.
func (v *ccoreViews) childByField(n *Node, fid FieldID) *Node {
	c, ok := v.value(n)
	if !ok {
		return nil
	}
	path, found := v.rd.FieldPath(c, uint16(fid))
	if !found {
		return nil
	}
	cur := n
	for _, i := range path {
		if cur = v.child(cur, int(i)); cur == nil {
			return nil
		}
	}
	return cur
}

// equalTree compares two trees through the exported methods, as
// incrementalTreesStructurallyEqual does for ordinary trees.
func (v *ccoreViews) equalTree(left, right *Node, lang *Language) bool {
	type pair struct{ a, b *Node }
	stack := []pair{{left, right}}
	for len(stack) != 0 {
		last := len(stack) - 1
		cur := stack[last]
		stack = stack[:last]
		l, r := cur.a, cur.b
		if l == r {
			continue
		}
		if l == nil || r == nil {
			return false
		}
		if l.Symbol() != r.Symbol() || l.Range() != r.Range() ||
			l.IsNamed() != r.IsNamed() || l.IsMissing() != r.IsMissing() ||
			l.IsExtra() != r.IsExtra() || l.IsError() != r.IsError() ||
			l.HasError() != r.HasError() || l.ChildCount() != r.ChildCount() {
			return false
		}
		for i := l.ChildCount() - 1; i >= 0; i-- {
			if l.FieldNameForChild(i, lang) != r.FieldNameForChild(i, lang) {
				return false
			}
			stack = append(stack, pair{l.Child(i), r.Child(i)})
		}
	}
	return true
}

// copyTree is Tree.Copy for a view tree: one retain of the core tree and a new
// root view; the copy starts with an empty table.
func (v *ccoreViews) copyTree(t *Tree) *Tree {
	state := t.ccoreStateOf()
	if state == nil || state.ct == nil || v.released.Load() {
		return nil
	}
	copyState := state.forTreeCopy()
	ct := copyState.ct
	if ct == nil {
		return nil
	}
	arena := acquireNodeViewArena()
	nodeViewArenaPool.forgetCheckedOut(arena)
	arena.ccore = copyState
	nv := newCcoreViews(ct, t.language, nil, arena)
	markPublishedRootError(nv.root)
	out := &Tree{
		root: nv.root, source: t.source, sourceEncoding: t.sourceEncoding, sourceUTF16: t.sourceUTF16, utf16Map: t.utf16Map,
		language: t.language, arena: arena,
		parseRuntime:                      acquireParseRuntime(),
		resultErrorSummary:                t.resultErrorSummary,
		resultCompatibilityApplied:        t.resultCompatibilityApplied,
		tokenInvariantReadSpan:            t.tokenInvariantReadSpan,
		eofExtraTokenSourceProofID:        t.eofExtraTokenSourceProofID,
		forestFastPath:                    t.forestFastPath,
		incrementalReuseDisabled:          t.incrementalReuseDisabled,
		incrementalReuseUnsupportedClause: t.incrementalReuseUnsupportedClause,
		compactMaterialized:               t.compactMaterialized,
		dependsOnColumnPropagated:         t.dependsOnColumnPropagated,
	}
	*out.parseRuntime = *t.rawParseRuntime()
	out.setIncludedRanges(t.includedRanges)
	if len(t.edits) > 0 {
		out.edits = make([]InputEdit, len(t.edits))
		copy(out.edits, t.edits)
	}
	ccoreViewTrees.Add(1)
	return out
}

func (v *ccoreViews) retainedBytes() uint64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	// Pages are counted exactly. The two maps are counted by entry: a key and a
	// pointer each, without the map's own overhead, which the runtime does not
	// report.
	const largeEntry = 8 + 8      // [2]uint32 key and *Node
	const duplicateEntry = 24 + 8 // ccoreViewOccurrence key (rounded up) and *Node
	return v.pageBytes + uint64(len(v.largeKeys))*8 + uint64(len(v.large))*largeEntry + uint64(len(v.duplicates))*duplicateEntry
}

// release ends the backing: later reads find nothing. The core tree is
// released by the tree's release hook.
func (v *ccoreViews) release() {
	v.released.Store(true)
}

// refreshRoot sets the root header from the edited core root: start after the
// padding, end after the size, and C's has_changes. It is the whole public
// effect of Tree.Edit when no view exists below the root.
func (v *ccoreViews) refreshRoot() {
	root := v.root
	c := v.rd.RootNode(v.core.Root())
	if c.Null() {
		return
	}
	end := v.rd.End(c)
	root.startByte, root.endByte = c.Start.Bytes(), end.Bytes()
	root.startPoint = Point{Row: c.Start.Row(), Column: c.Start.Col()}
	root.endPoint = Point{Row: end.Row(), Column: end.Col()}
	root.setDirty(v.rd.HasChanges(c))
}

// onlyRoot reports whether no view exists below the root.
func (v *ccoreViews) onlyRoot() bool { return v.made.Load() <= 1 }

// ---- The sparse rebind: Tree.Edit on a view tree with views below the root ----
//
// The core's edit copies every record on the edit path and gives each copy a new
// child list (internal/ccore/edit.go). After it a view can be wrong in two
// ways: its start has moved, and the slot its key names no longer holds its
// handle. The rebind refreshes the headers that exist and makes none.
//
//  1. The edit is staged on a copy of the core tree (one retain of the root, as
//     Tree.Copy takes). The tree the views describe stays readable and keeps
//     its child lists, so a slot the edit allocates is never a slot a view names.
//  2. Both trees are walked from the root along the records the edit copied,
//     through the views that exist and no others. A view is found by its
//     parent view and its ordinal among the visible children, in the slot the
//     tree before the edit gives that child; not by its handle (one record can
//     occur at several positions) and not by a byte offset. The children of
//     one parent are read in one pass.
//  3. This walk writes nothing: it lists the views of the children of copied
//     records, each with its span, has_changes and slot after the edit. When
//     the two trees do not match, or a new slot is taken, the staged tree is
//     released and Tree.Edit converts the tree as it did before the rebind
//     existed.
//  4. Then each listed header gets its span and has_changes, and a moved one
//     its new slot key. A listed view whose record the edit did not copy but
//     whose start moved is the root of a subtree that only moved: the views
//     below it keep their slots, and a second walk, through those views only,
//     gives each its new span. A view whose record and start are as before is
//     the root of a subtree the edit left alone: nothing below it is read.
//     Last, the backing reads the staged tree, and the tree before the edit is
//     released.
//
// The *Node pointers of the edited tree stay the same. A node of the tree a
// reparse returns is still another pointer than the node of this tree.

// ccoreRebind is the view of one child of a copied record, with what the edit
// made of it.
type ccoreRebind struct {
	n          *Node
	now        ccore.Node // the child in the edited tree
	end        ccore.Length
	moved      bool // the slot is not the slot before the edit
	shifted    bool // the record is the one before the edit, at another start: the views below only move
	hasChanges bool // C's has_changes after the edit
}

// ccoreEditSparse is Tree.Edit for a view tree with views below the root. It
// reports false, with the reason counted, when the tree must be converted; then
// it has changed nothing but, after a refused or failed core edit, the broken
// mark of the link (as ccoreEdit sets it).
func (t *Tree) ccoreEditSparse(v *ccoreViews, edit InputEdit) (done bool) {
	if ccoreTestSparseOff.Load() {
		ccoreViewSparseStops[ccoreSparseStopOff].Add(1)
		return false
	}
	st := t.ccoreStateOf()
	if st == nil || st.ct == nil || st.broken || st.ct != v.core || st.ct.Released() || v.released.Load() ||
		edit.StartByte > edit.OldEndByte || edit.StartByte > edit.NewEndByte || int64(edit.OldEndByte) > st.curLen {
		ccoreViewSparseStops[ccoreSparseStopLink].Add(1)
		return false
	}
	started := nowNanos()
	defer func() { ccoreEditNanos.Add(nowNanos() - started) }()
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.duplicates) != 0 {
		// Two occurrences share a slot: moving one would need the other found and
		// kept. The conversion handles it.
		ccoreViewSparseStops[ccoreSparseStopDuplicates].Add(1)
		return false
	}
	staged := st.ct.Copy()
	if staged == nil {
		ccoreViewSparseStops[ccoreSparseStopLink].Add(1)
		return false
	}
	const (
		editing = iota // the core's edit runs on the staged tree
		walking        // the two trees are compared; nothing is written
		writing        // the headers and the table are written; no failure is handled from here
	)
	stage := editing
	defer func() {
		if stage == writing {
			return
		}
		if r := recover(); r != nil {
			done = false
			if stage == editing {
				st.broken = true // as ccoreEdit: the store may be poisoned
				ccoreViewSparseStops[ccoreSparseStopRefused].Add(1)
			} else {
				ccoreViewSparseStops[ccoreSparseStopMismatch].Add(1)
			}
		}
		staged.Release()
	}()
	ce := ccoreEditOf(edit)
	if st.budget > 0 {
		// The same charge as ccoreEdit makes; a refusal breaks the link in the same way.
		var led ccore.Ledger
		budget := st.budget
		led.SetPoll(func(total int64, _ bool) bool { return total < budget })
		if !led.Begin(int64(staged.PageBytes())) || !staged.EditWithin(ce, &led) {
			st.broken = true
			ccoreViewSparseStops[ccoreSparseStopRefused].Add(1)
			return false
		}
	} else {
		staged.Edit(ce)
	}
	stage = walking
	nrd := ccore.NewNodeReader(staged.Reader(), v.rd.T)
	plan, ok := v.sparsePlan(nrd, staged.Root())
	if !ok {
		ccoreViewSparseStops[ccoreSparseStopMismatch].Add(1)
		return false
	}
	stage = writing
	refreshed := v.sparseApply(nrd, plan)
	before := st.ct
	st.ct, v.core, v.rd = staged, staged, nrd
	v.refreshRoot()
	st.edits++
	st.curLen += int64(edit.NewEndByte) - int64(edit.OldEndByte)
	before.Release()
	ccoreViewSparseEdits.Add(1)
	ccoreViewSparseMoved.Add(int64(refreshed))
	return true
}

// cached returns the view of the occurrence read at c below parent, or nil
// when none was made. It makes nothing. The caller holds mu and has seen that
// no slot has views of two occurrences.
func (v *ccoreViews) cached(c *ccore.Node, parent *Node) *Node {
	var n *Node
	if key, ok := ccoreSlotKey(*c); ok {
		n = v.lookup(key)
	} else {
		n = v.large[[2]uint32{c.List, c.Slot}]
	}
	if n == nil || n.parent != parent || n.equivVersion != c.Ordinal {
		return nil
	}
	return n
}

// sparsePlan walks the tree the views describe and the edited tree (nrd, root)
// together, along the records the edit copied, and lists the views of their
// children. It writes nothing. It reports false when the trees do not match or
// a slot a view would move to holds a view. The caller holds mu.
func (v *ccoreViews) sparsePlan(nrd ccore.NodeReader, root ccore.Handle) ([]ccoreRebind, bool) {
	ord := v.rd
	oldRoot, newRoot := ord.RootNode(v.core.Root()), nrd.RootNode(root)
	if oldRoot.Null() || newRoot.Null() {
		return nil, false
	}
	if oldRoot.H == newRoot.H {
		return nil, true // the edit changed no record
	}
	type frame struct {
		n        *Node
		was, now ccore.Node // a copied record, before and after the edit
	}
	work := []frame{{v.root, oldRoot, newRoot}}
	var plan []ccoreRebind
	var oldKids, newKids []ccore.Node
	for len(work) > 0 {
		f := work[len(work)-1]
		work = work[:len(work)-1]
		oldKids, newKids = ord.Children(f.was, oldKids[:0]), nrd.Children(f.now, newKids[:0])
		if len(oldKids) != len(newKids) {
			return nil, false
		}
		for i := range newKids {
			was, now := &oldKids[i], &newKids[i]
			if was.Ordinal != now.Ordinal || was.Mask != now.Mask || was.Alias != now.Alias ||
				ccore.LargeSlot(was.List) != ccore.LargeSlot(now.List) || ord.GrammarSymbol(*was) != nrd.GrammarSymbol(*now) {
				return nil, false
			}
			n := v.cached(was, f.n)
			if n == nil {
				continue // no view of this child, so none below it
			}
			moved := was.List != now.List || was.Slot != now.Slot
			if moved {
				taken := false
				if key, small := ccoreSlotKey(*now); small {
					taken = v.lookup(key) != nil
				} else {
					taken = v.large[[2]uint32{now.List, now.Slot}] != nil
				}
				if taken {
					return nil, false
				}
			}
			copied := was.H != now.H
			plan = append(plan, ccoreRebind{n: n, now: *now, end: nrd.End(*now), moved: moved, hasChanges: nrd.HasChanges(*now),
				shifted: !copied && (now.Start.Bytes() != n.startByte || now.Start.Row() != n.startPoint.Row || now.Start.Col() != n.startPoint.Column)})
			if copied {
				work = append(work, frame{n: n, now: *now, was: ccore.Node{
					H:     was.H,
					Start: ccore.NewLength(n.startByte, n.startPoint.Row, n.startPoint.Column),
					Mask:  uint32(n.dynamicPrecedence),
				}})
			}
		}
	}
	return plan, true
}

// sparseApply writes the plan of sparsePlan and returns the number of headers
// it refreshed. Every moved view leaves its slot before any enters its new one,
// then each listed header gets its span and has_changes. A table page that the
// moves left empty is dropped. Then the views below each listed view that only
// moved get their spans from the edited tree nrd: their records and slots are
// those before the edit, so there is nothing to match and nothing to move. The
// caller holds mu.
func (v *ccoreViews) sparseApply(nrd ccore.NodeReader, plan []ccoreRebind) (refreshed int) {
	var vacated []int // table pages a view left, in runs
	for i := range plan {
		it := &plan[i]
		if !it.moved {
			continue
		}
		key := uint32(it.n.rawShape)
		if key&ccoreLargeKey != 0 {
			delete(v.large, v.largeKeys[key&^ccoreLargeKey])
			continue
		}
		pi := int(key / ccoreViewPageSize)
		v.top.Load().pages[pi].Load()[key%ccoreViewPageSize].Store(nil)
		if len(vacated) == 0 || vacated[len(vacated)-1] != pi {
			vacated = append(vacated, pi)
		}
	}
	setSpan := func(n *Node, start, end ccore.Length) {
		n.startByte, n.endByte = start.Bytes(), end.Bytes()
		n.startPoint = Point{Row: start.Row(), Column: start.Col()}
		n.endPoint = Point{Row: end.Row(), Column: end.Col()}
	}
	type frame struct {
		n   *Node
		now ccore.Node
	}
	var work []frame
	for i := range plan {
		it := &plan[i]
		n := it.n
		if it.moved {
			if ccore.LargeSlot(it.now.List) {
				lk := [2]uint32{it.now.List, it.now.Slot}
				v.largeKeys[uint32(n.rawShape)&^ccoreLargeKey] = lk
				v.large[lk] = n
			} else {
				slot := it.now.Slot
				pi := int(slot / ccoreViewPageSize)
				top := v.top.Load()
				if top == nil || pi >= len(top.pages) {
					top = v.growTop(top, pi)
				}
				page := top.pages[pi].Load()
				if page == nil {
					page = new(ccoreViewPage)
					v.pageBytes += uint64(unsafe.Sizeof(*page))
					top.pages[pi].Store(page)
				}
				page[slot%ccoreViewPageSize].Store(n)
				n.rawShape = rawShapeRef(slot)
			}
		}
		setSpan(n, it.now.Start, it.end)
		n.setDirty(it.hasChanges)
		if it.shifted {
			if _, kids := nrd.S.ChildSlots(it.now.H); kids != 0 {
				work = append(work, frame{n, it.now})
			}
		}
	}
	refreshed = len(plan)
	top := v.top.Load()
	for _, pi := range vacated {
		page := top.pages[pi].Load()
		if page == nil {
			continue
		}
		empty := true
		for j := range page {
			if page[j].Load() != nil {
				empty = false
				break
			}
		}
		if empty {
			top.pages[pi].Store(nil)
			v.pageBytes -= uint64(unsafe.Sizeof(*page))
		}
	}
	// The subtrees that only moved. The moved views are in their new slots by
	// now, and these keep theirs.
	var kids []ccore.Node
	for len(work) > 0 {
		f := work[len(work)-1]
		work = work[:len(work)-1]
		kids = nrd.Children(f.now, kids[:0])
		for i := range kids {
			k := &kids[i]
			n := v.cached(k, f.n)
			if n == nil {
				continue
			}
			setSpan(n, k.Start, nrd.End(*k))
			refreshed++
			if _, below := nrd.S.ChildSlots(k.H); below != 0 {
				work = append(work, frame{n, *k})
			}
		}
	}
	return refreshed
}

// materialize turns the view tree into an eager tree in place: every node gets
// a header, children, field ids and the field answers the eager conversion
// records, and the arena stops being a view arena. Held pointers stay valid.
// It is a worklist, so the depth of the tree does not limit it. It must run
// before the core tree is edited, because it reads the records of the tree
// the views describe.
func (v *ccoreViews) materialize() { v.materializeFor(ccoreViewConvOther) }

// materializeFor is the one conversion: it counts the reason when it converts.
// Conversions outside parse requests do not charge a memory budget because those calls cannot fail.
func (v *ccoreViews) materializeFor(reason int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.released.Load() || v.arena.views != nodeViewBacking(v) {
		return
	}
	ccoreViewConversions[reason].Add(1)
	rd := v.rd
	s := rd.S
	arena := v.arena
	ccoreTablesMu.Lock()
	st := ccoreLanguageTablesForLocked(v.lang)
	ccoreTablesMu.Unlock()
	conv := &ccoreConv{s: s, fs: s, lang: v.lang, conv: st.conv, arena: arena}
	type item struct {
		n *Node
		c ccore.Node
	}
	root, _ := v.valueLocked(v.root)
	work := []item{{v.root, root}}
	all := make([]item, 0, 64)
	var kids []ccore.Node
	for len(work) > 0 {
		it := work[len(work)-1]
		work = work[:len(work)-1]
		var hidden bool
		kids, hidden = rd.ChildrenHidden(it.c, kids[:0])
		all = append(all, it)
		if len(kids) == 0 {
			continue
		}
		children := arena.allocNodeSlice(len(kids))
		conv.fids, conv.cfids = conv.fids[:0], conv.cfids[:0]
		anyField := false
		for i, k := range kids {
			cv := v.viewForLocked(k, it.n)
			children[i] = cv
			conv.fids = append(conv.fids, FieldID(k.Field))
			conv.cfids = append(conv.cfids, FieldID(k.CField))
			if k.Field != 0 {
				anyField = true
			}
			work = append(work, item{cv, k})
		}
		it.n.children = children
		if anyField {
			ids := arena.allocFieldIDSlice(len(kids))
			copy(ids, conv.fids)
			it.n.setFieldMetadata(ids, defaultFieldSourcesInArena(arena, ids))
		}
		conv.hidden = hidden
		v.recordFieldAnswers(conv, it.n, it.c.H, 0, len(kids))
	}
	for _, it := range all {
		n, c := it.n, it.c
		mask := uint32(n.dynamicPrecedence)
		n.dynamicPrecedence = 0
		if len(n.children) > 0 {
			n.dynamicPrecedence = s.DynamicPrecedence(c.H)
			for i, ch := range n.children {
				setNodeParentLink(ch, n, i)
			}
		}
		n.rawShape = 0
		n.equivVersion = uint32(s.Symbol(c.H))
		if mask != 0 {
			n.addSupertypeMask(arena, mask)
		}
		if s.DependsOnColumn(c.H) {
			arena.setNodeDependsOnColumnBit(n, true)
		}
	}
	setNodeRootLink(v.root)
	arena.views = nil
	v.released.Store(true)
}

// recordFieldAnswers keeps the field and cursor answers when views become ordinary nodes.
func (v *ccoreViews) recordFieldAnswers(c *ccoreConv, n *Node, h ccore.Handle, base, cnt int) {
	var e *ccoreNodeExtra
	cur := c.cfids[base : base+cnt]
	for i, f := range cur {
		if f != c.fids[base+i] {
			if !c.reserve(int64(ccoreExtraBytes)) { // the record
				return
			}
			cursor, ok := ccore.Make[FieldID](c.l(), len(cur))
			if !ok {
				c.stop = true
				return
			}
			copy(cursor, cur)
			e = &ccoreNodeExtra{cursor: cursor}
			break
		}
	}
	entries := c.fieldMapOf(c.fs.ProductionID(h))
	complexMap := c.hidden
	for _, en := range entries {
		if en.Inherited {
			complexMap = true
			break
		}
	}
	inMap := func(f FieldID) bool {
		for _, en := range entries {
			if en.FieldID == f {
				return true
			}
		}
		return false
	}
	if !complexMap {
		for j := 0; j < cnt && !complexMap; j++ { // a stored field that the production's map lacks: C's lookup finds none
			if f := c.fids[base+j]; f != 0 && !inMap(f) {
				complexMap = true
			}
		}
	}
	if complexMap {
		fields, ok := ccore.MakeCap[FieldID](c.l(), len(entries)+cnt) // the field list, at its largest
		if !ok {
			c.stop = true
			return
		}
		have := func(f FieldID) bool {
			for _, x := range fields {
				if x == f {
					return true
				}
			}
			return false
		}
		for _, en := range entries {
			if !have(en.FieldID) {
				fields = fields[:len(fields)+1]
				fields[len(fields)-1] = en.FieldID
			}
		}
		for j := 0; j < cnt; j++ {
			if f := c.fids[base+j]; f != 0 && !have(f) {
				fields = fields[:len(fields)+1]
				fields[len(fields)-1] = f
			}
		}
		hits, ok := ccore.MakeCap[ccoreFieldHit](c.l(), len(fields)) // allocated whether or not it is kept
		if !ok {
			c.stop = true
			return
		}
		differs := false
		for _, fid := range fields {
			hit := ccoreFieldHit{field: fid}
			if path, found := v.rd.FieldPath(ccore.Node{H: h}, uint16(fid)); found {
				hit.found, hit.path = true, path
			}
			if c.stop {
				return
			}
			stored := int32(-1)
			for j := 0; j < cnt; j++ {
				if c.fids[base+j] == fid {
					stored = int32(j)
					break
				}
			}
			if hit.found != (stored >= 0) || hit.found && (len(hit.path) != 1 || hit.path[0] != stored) {
				differs = true
			}
			hits = hits[:len(hits)+1]
			hits[len(hits)-1] = hit
		}
		if differs {
			if e == nil {
				if !c.reserve(int64(ccoreExtraBytes)) { // the lookup-only record, before it is made
					return
				}
				e = &ccoreNodeExtra{}
			}
			e.lookup = hits
		}
	}
	if e != nil {
		if !c.reserve(ccoreMapEntryBytes) { // the map entry (an estimate for a Go map)
			return
		}
		st := c.arena.ccore
		if st.extras == nil {
			st.extras = map[*Node]*ccoreNodeExtra{}
		}
		st.extras[n] = e
	}
}

// valueLocked is value for a caller that holds mu.
func (v *ccoreViews) valueLocked(n *Node) (ccore.Node, bool) {
	var h ccore.Handle
	switch {
	case n == v.root:
		h = v.core.Root()
	case uint32(n.rawShape)&ccoreLargeKey != 0:
		lk := v.largeKeys[uint32(n.rawShape)&^ccoreLargeKey]
		h = v.rd.S.ChildHandle(lk[0], 0, lk[1])
	default:
		h = v.rd.S.HandleAt(uint32(n.rawShape))
	}
	if h == 0 {
		return ccore.Node{}, false
	}
	return ccore.Node{
		H:     h,
		Start: ccore.NewLength(n.startByte, n.startPoint.Row, n.startPoint.Column),
		Mask:  uint32(n.dynamicPrecedence),
	}, true
}

// nodeFieldBacking is implemented by a view backing that answers the field questions.
type nodeFieldBacking interface {
	fieldIDAt(*Node, int) FieldID
	cursorFieldAt(*Node, int) FieldID
	cursorChildByField(*Node, FieldID) (*Node, int)
	childByField(*Node, FieldID) *Node
}

var _ nodeFieldBacking = (*ccoreViews)(nil)

// ccoreViewsOf returns the views backing of a node, or nil for any other node.
func ccoreViewsOf(n *Node) *ccoreViews {
	v, _ := nodeViews(n).(*ccoreViews)
	return v
}

// ccoreMaterializeForReason converts the view tree of n in place and counts the reason.
func ccoreMaterializeForReason(n *Node, reason int) {
	if v := ccoreViewsOf(n); v != nil {
		v.materializeFor(reason)
		if reason == ccoreViewConvNodeEdit || reason == ccoreViewConvAdopt {
			v.arena.ccore.releaseCore()
		}
	}
}

// ccoreSetViewsOn is bind.Adapter.ViewsOn.
func ccoreSetViewsOn(on bool) (prev bool) { return ccoreViewsOn.Swap(on) }

// ccoreViewCounters is bind.Adapter.ViewCounters.
func ccoreViewCounters() (trees, made int64, conversions map[string]int64) {
	conversions = map[string]int64{}
	for i := range ccoreViewConversions {
		if v := ccoreViewConversions[i].Load(); v != 0 {
			conversions[ccoreViewConvNames[i]] = v
		}
	}
	if v := ccoreViewRootEdits.Load(); v != 0 {
		conversions["root_only_edits"] = v // not a conversion: the edits that needed none
	}
	if v := ccoreShortcutReparses.Load(); v != 0 {
		conversions["shortcut_reparses"] = v
	}
	if v := ccoreViewSparseEdits.Load(); v != 0 {
		conversions["sparse_edits"] = v // not a conversion: the edits that refreshed the headers that exist
		conversions["sparse_refreshed"] = ccoreViewSparseMoved.Load()
	}
	for i := range ccoreViewSparseStops {
		if v := ccoreViewSparseStops[i].Load(); v != 0 {
			conversions[ccoreSparseStopNames[i]] = v
		}
	}
	return ccoreViewTrees.Load(), ccoreViewsMade.Load(), conversions
}

var errNoTokenSourceRebuilder = errors.New("token source cannot rebuild")

// coreScanTokenSource is the trusted wrapper that NewCoreScanTokenSource
// returns. It marks a token source as one whose lexing the core can reproduce
// with a certified scan provider (the table-backed provider of internal/ccore,
// "table-v1"), bound to one language and one source buffer.
//
// The parser recognizes only this exact concrete pointer type. A type that
// embeds it, or wraps the interface value, is an ordinary stream. Before the
// present engine runs, the parser takes the wrapper off and passes the
// fallback unchanged, so every optional interface of the fallback behaves as it
// does without the wrapper. A request that the core serves never calls the
// fallback's Next.
type coreScanTokenSource struct {
	fallback TokenSource
	lang     *Language
	src      []byte
}

func init() {
	coresource.Mint = func(fallback, lang any, src []byte) any {
		ts, _ := fallback.(TokenSource)
		l, _ := lang.(*Language)
		if m := newCoreScanTokenSource(ts, l, src); m != nil {
			return m
		}
		return fallback
	}
}

// newCoreScanTokenSource returns fallback marked as a token source whose
// lexing equals the core's table-backed scan provider for lang over src. Parser
// methods recognize the mark and may serve the request with the core when the
// language's certificate lists the provider and the request is for the same
// language and the same source bytes (the same slice start and length). In every
// other case the fallback runs on the present engine, as it would without the
// mark. The library that mints the mark states that fallback produces the token
// stream of lang's own lexer for src; the parser does not check it.
//
// The returned value delegates Next and its other methods to fallback. A nil
// fallback gives nil.
func newCoreScanTokenSource(fallback TokenSource, lang *Language, src []byte) TokenSource {
	if fallback == nil {
		return nil
	}
	if t, ok := fallback.(*coreScanTokenSource); ok && t != nil {
		fallback = t.fallback
	}
	return &coreScanTokenSource{fallback: fallback, lang: lang, src: src}
}

// Next delegates to the fallback.
func (t *coreScanTokenSource) Next() Token { return t.fallback.Next() }

// Close closes the fallback when it can be closed.
func (t *coreScanTokenSource) Close() {
	if c, ok := t.fallback.(closeableTokenSource); ok {
		c.Close()
	}
}

// Reset delegates to the fallback when it can reset. The mark stays bound to the
// original bytes.
func (t *coreScanTokenSource) Reset(source []byte) {
	if r, ok := t.fallback.(resettableTokenSource); ok {
		r.Reset(source)
	}
}

// SetParserState delegates to the fallback when it takes parser states.
func (t *coreScanTokenSource) SetParserState(state StateID) {
	if s, ok := t.fallback.(parserStateTokenSource); ok {
		s.SetParserState(state)
	}
}

// SetGLRStates delegates to the fallback when it takes parser states.
func (t *coreScanTokenSource) SetGLRStates(states []StateID) {
	if s, ok := t.fallback.(parserStateTokenSource); ok {
		s.SetGLRStates(states)
	}
}

// SkipToByte delegates to the fallback when it can skip; a fallback that cannot
// skip gives the end-of-input token.
func (t *coreScanTokenSource) SkipToByte(offset uint32) Token {
	if s, ok := t.fallback.(ByteSkippableTokenSource); ok {
		return s.SkipToByte(offset)
	}
	return Token{}
}

// SupportsIncrementalReuse reports what the fallback reports; false when it
// has no such method.
func (t *coreScanTokenSource) SupportsIncrementalReuse() bool {
	r, ok := t.fallback.(IncrementalReuseTokenSource)
	return ok && r.SupportsIncrementalReuse()
}

// RebuildTokenSource delegates to the fallback and returns what it returns, an
// ordinary token source. A fallback with no rebuilder gives an error.
func (t *coreScanTokenSource) RebuildTokenSource(source []byte, lang *Language) (TokenSource, error) {
	if r, ok := t.fallback.(TokenSourceRebuilder); ok {
		return r.RebuildTokenSource(source, lang)
	}
	return nil, errNoTokenSourceRebuilder
}

// unwrapCoreScanTokenSource returns the fallback of a trusted wrapper, or ts.
func unwrapCoreScanTokenSource(ts TokenSource) TokenSource {
	if t, ok := ts.(*coreScanTokenSource); ok && t != nil {
		return t.fallback
	}
	return ts
}

// unwrapCoreScanFactory makes a factory give the fallback of a trusted wrapper
// in place of the wrapper, so that a reparse the present engine runs from the
// factory sees the same source as it does without the wrapper.
func unwrapCoreScanFactory(f TokenSourceFactory) TokenSourceFactory {
	if f == nil {
		return nil
	}
	return func(source []byte) (TokenSource, error) {
		ts, err := f(source)
		return unwrapCoreScanTokenSource(ts), err
	}
}

// sameSourceBytes reports whether a and b are the same buffer: the same start
// and the same length.
func sameSourceBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	return len(a) == 0 || &a[0] == &b[0]
}

func (p *Parser) runCoreForestTransport(source []byte, automatic bool) (*Tree, *ccore.ForestElection) {
	e := &ccore.ForestElection{WorkLimit: forestWorklistVisitCap, ReduceLimit: forestReduceVisitCap}
	if len(p.included) != 0 {
		e.Stop = "admission"
		return nil, e
	}
	adm, _, ok := p.ccoreAdmitQuiet()
	if !ok || adm.modes&certified.Forest == 0 || !p.ccoreEligibleWithGrant(ccoreSourceDFA, sched.ForestRoute) {
		e.Stop = "admission"
		return nil, e
	}
	if sc := p.language.ExternalScanner; sc != nil {
		stateless, ok := sc.(interface{ ExternalScannerIsStateless() bool })
		if !ok || !stateless.ExternalScannerIsStateless() {
			e.Stop = "scanner_not_stateless"
			return nil, e
		}
	}
	ts := p.acquireParserDFATokenSource(source)
	if ts == nil {
		e.Stop = "token_source"
		return nil, e
	}
	defer ts.Close()
	var ordered []StateID
	var current Token
	e.CanShift = func(offset uint32) bool { return p.guardRealShiftGap(source, &glrStack{byteOffset: offset}, current) }
	e.Next = func(states []uint16) (ccore.Token, bool, bool, bool) {
		ordered = ordered[:0]
		for _, state := range states {
			ordered = append(ordered, StateID(state))
		}
		ts.SetGLRStates(ordered[:])
		ts.SetParserState(ordered[len(ordered)-1])
		t := ts.Next()
		current = t
		if t.Missing {
			e.Stop = "forest_unsupported_missing_token"
		}
		return ccore.Token{Symbol: ccore.Symbol(t.Symbol), StartByte: t.StartByte, EndByte: t.EndByte, StartRow: t.StartPoint.Row, StartCol: t.StartPoint.Column, EndRow: t.EndPoint.Row, EndCol: t.EndPoint.Column, LookaheadEnd: t.lexerLookaheadEndByte}, t.ExternalScannerToken, t.isKeyword(), t.NoLookahead
	}
	e.Resolve = func(state uint16, n int) int {
		all := p.actionsForParseState(StateID(state), current.Symbol, p.language.ParseActions)
		if len(all) != n {
			return -2
		}
		actions := p.forestResolveConflict(StateID(state), current, all)
		if len(actions) > 1 {
			if folded, ok := p.cRepetitionSkipForestConflictChoice(false, actions); ok {
				actions = p.forestSingletonActions(folded)
			}
		}
		if len(actions) == len(all) {
			return -1
		}
		if len(actions) == 1 {
			for i, a := range all {
				if a == actions[0] {
					return i
				}
			}
		}
		return -2
	}
	adm.forestElection = e
	adm.automaticForest = automatic
	tree, _, _ := p.ccoreRun(adm, source, nil)
	e.Next = nil
	e.CanShift = nil
	e.Resolve = nil
	if tree == nil && e.Stop == "" {
		e.Stop = "core_binding_declined"
	}
	if tree != nil {
		// Only the numeric incremental route has independently validated the forest
		// record contract. Legacy subtree reuse still needs its own checkpoints.
		tree.incrementalReuseDisabled = true
		tree.ccoreStateOf().forestElection = true
		tree.ccoreStateOf().forestOrdinaryCompatible = e.OrdinaryCompatible()
	}
	return tree, e
}

var transportAccepted, transportDeclined, transportAbstained atomic.Uint64

func ccoreForestRouteCounters() (uint64, uint64, uint64) {
	return transportAccepted.Load(), transportDeclined.Load(), transportAbstained.Load()
}

// A non-EOF dead end is a complete strict-forest answer. The automatic route
// may then use compact recovery. Resource failures and unhandled stops keep
// their normal route decline counters.
func ccoreHandledForestDeadEnd(e *ccore.ForestElection) bool {
	return e != nil && e.Stop == "dead_end" && e.Token.Symbol != 0
}

// Both public routes use the same native forest result. Only the automatic
// parse may ask the compact recovery parser to complete a semantic decline.
func (p *Parser) serveCoreForestTransport(source []byte, automatic bool) (*Tree, bool) {
	if p == nil || p.language == nil || !p.ccoreRouteEnabled() || glrForestRecover.Load() || languageWantsForestRecover(p.language.Name) {
		return nil, false
	}
	tree, event := p.runCoreForestTransport(source, automatic)
	if tree != nil {
		root := tree.RootNode()
		if root == nil || root.HasError() || root.EndByte() < uint32(len(bytes.TrimRight(source, " \t\r\n"))) {
			tree.Release()
			transportAbstained.Add(1)
			return nil, false
		}
		tree.forestFastPath = true
		rt := tree.ensureParseRuntime()
		rt.ForestFastPath = true
		rt.LastTokenWasEOF = true
		rt.LastTokenEndByte = uint32(len(source))
		rt.LastTokenSymbol = 0
		p.recordForestDecline("", Token{}, nil)
		ccoreServedFresh.Add(1)
		transportAccepted.Add(1)
		return tree, true
	}
	if !ccoreHandledForestDeadEnd(event) {
		if event.Stop != "admission" {
			transportAbstained.Add(1)
		}
		return nil, false
	}
	states := make([]StateID, len(event.States))
	for i, state := range event.States {
		states[i] = StateID(state)
	}
	p.recordForestDecline(event.Stop, Token{Symbol: Symbol(event.Token.Symbol), StartByte: event.Token.StartByte}, states)
	transportDeclined.Add(1)
	if !automatic {
		return nil, true
	}
	recovered, ok := p.ccoreFullParse(source)
	return recovered, ok
}

// ccoreCompactRecords exposes the stored records to the internal equality harness.
func ccoreCompactRecords(value any, w io.Writer) (hash, rows uint64) {
	t, ok := value.(*Tree)
	if !ok || t == nil || t.ccoreLink() == nil {
		return 0, 0
	}
	cp, err := bind.New(t.language)
	if err != nil {
		return 0, 0
	}
	defer cp.FinishScanner()
	return cp.Records(t.ccoreLink(), w)
}
