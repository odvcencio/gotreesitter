package gotreesitter

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
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

// ccoreBlobAllowlist is the twin of admissionCandidateLanguageAllowlist keyed
// by grammar blob SHA-256: the blobs for which the default route widens. It is
// empty and the candidate route does not consult it yet; only
// TestCcoreUnroutedMarkerMatchesEmptyLists reads it.
var ccoreBlobAllowlist = map[[32]byte]bool{}

// ccoreTestParserHook is test-only (nil in production): it is called with a
// core parser just before a parse (before true) and after it, when the parser
// goes back to the pool (before false), so a test can make one parse stop or
// panic and reset the parser afterwards.
var ccoreTestParserHook atomic.Pointer[func(cp *ccore.Parser, before bool)]

// ccoreUnset reports that nothing is certified, which is the default.
func ccoreUnset() bool {
	return !certified.EverActive() || (len(certified.Blobs) == 0 && certified.Current() == nil)
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
	ccoreDeclCount
)

var ccoreDeclineNames = [ccoreDeclCount]string{
	"not_certified", "no_binding", "mode_missing", "root_only_field", "included_ranges", "observer",
	"work_limits", "internal_parser", "stop_deadline", "stop_cancelled", "stop_memory_budget", "core_stop", "panic",
	"scanner_changed", "depth_limit", "config_changed", "scanner_refused",
}

var (
	ccoreDeclines    [ccoreDeclCount]atomic.Int64
	ccoreServedFresh atomic.Int64
	// Nanoseconds spent in the core parse and in the conversion (read by the
	// timing test through export_test.go).
	ccoreParseNanos, ccoreConvertNanos atomic.Int64
)

func ccoreDecline(reason int) { ccoreDeclines[reason].Add(1) }

func ccoreRouteCounters() (map[string]int64, int64, int64) {
	out := make(map[string]int64)
	for i := range ccoreDeclines {
		if v := ccoreDeclines[i].Load(); v != 0 {
			out[ccoreDeclineNames[i]] = v
		}
	}
	return out, ccoreServedFresh.Load(), 0
}

func ccoreBindRootOnlyFields(lang any) []string {
	if l, ok := lang.(*Language); ok && l != nil {
		return ccoreRootOnlyFields(l)
	}
	return nil
}

func ccoreRouteNanos(reset bool) (parse, convert, edit, check int64) {
	if reset {
		return ccoreParseNanos.Swap(0), ccoreConvertNanos.Swap(0), 0, 0
	}
	return ccoreParseNanos.Load(), ccoreConvertNanos.Load(), 0, 0
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

// ccoreAdmission is the answer of ccoreAdmit: the language state, the blob hash
// and the modes the blob is certified for.
type ccoreAdmission struct {
	st      *ccoreLanguageTables
	sha     [32]byte
	modes   uint8
	binding *ccoreBinding // the identity of the binding this request qualified with
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
	case len(p.included) > 0: // the core does not apply included ranges
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
		}
	}
	if tcert != nil {
		if ccoreFieldsChecked(st.rootOnly, func(f string) bool { return tcert.Checked[f] }) {
			have |= tcert.Modes
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
	a.modes = have
	a.binding = st.token
	return a, 0, true
}

// ccoreAdmit is ccoreAdmitQuiet that counts the decline reason.
func (p *Parser) ccoreAdmit() (ccoreAdmission, bool) {
	a, reason, ok := p.ccoreAdmitQuiet()
	if !ok {
		ccoreDecline(reason)
	}
	return a, ok
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
	arena.ccore = &ccoreArenaState{tables: st.base, binding: st.token, ct: ct}
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
	tables  *ccore.Tables
	binding *ccoreBinding // the identity of the binding the tree was parsed with (not the state, so a tree does not keep a parser pool alive)
	extras  map[*Node]*ccoreNodeExtra
	ct      *ccore.Tree
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
	return &ccoreArenaState{tables: a.tables, binding: a.binding}
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
	if a.ct != nil && !a.ct.Released() {
		c.ct = a.ct.Copy()
	}
	return c
}

func (p *Parser) ccoreRun(adm ccoreAdmission, source []byte) (tree *Tree, ok bool) {
	st, blob := adm.st, adm.sha
	lang := p.language
	cp, err := st.get(lang)
	if err != nil {
		ccoreDecline(ccoreDeclNoBinding)
		return nil, false
	}
	keep := true // the parser may go back to the pool
	stopped := ParseStopNone
	converting := false
	defer func() {
		if r := recover(); r != nil {
			tree, ok, keep = nil, false, false
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
	if h := ccoreTestParserHook.Load(); h != nil {
		(*h)(cp, true)
	}
	lim := p.ccoreLimitsFor(len(source))
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
	ct, stop := ccoreParseCore(cp, source)
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
		} else {
			ccoreDecline(ccoreDeclCoreStop)
		}
		return nil, false
	}

	// The parser's arena and buffers stay live while the public tree is built; the
	// conversion reserves its own arrays in the same ledger, on top of the core's.
	var end ccoreConvEnd
	converting = true
	built, tooDeep := ccoreConvertTree(ct, source, lang, blob, st, cp.Ledger(), &end)
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
		return nil, false
	}
	retained = true
	rt := built.ensureParseRuntime()
	rt.StopReason = ParseStopAccepted
	rt.SourceLen = uint32(len(source))
	rt.ExpectedEOFByte = uint32(len(source))
	if root := built.root; root != nil {
		rt.RootEndByte = root.endByte
	}
	rt.MemoryBudgetBytes = lim.budget
	rt.MaxStacksSeen = cp.MaxVersions()
	rt.TokensConsumed = uint64(cp.Tokens())
	return built, true
}

// ccoreFullParse serves a fresh parse when the core admits the request.
func (p *Parser) ccoreFullParse(source []byte) (tree *Tree, ok bool) {
	if ccoreUnset() {
		return nil, false
	}
	defer func() { // admission and parser acquisition are inside the recovery boundary
		if r := recover(); r != nil {
			tree, ok = nil, false
			ccoreDecline(ccoreDeclPanic)
		}
	}()
	adm, ok := p.ccoreAdmit()
	if !ok {
		return nil, false
	}
	tree, ok = p.ccoreRun(adm, source)
	if ok {
		ccoreServedFresh.Add(1)
	}
	return tree, ok
}

// ccoreUnchangedTreeAllowed decides, once a certificate has existed in this
// process, whether a reparse of unchanged source may return the old tree. The
// callers ask canReuseUnchangedTree first, as they did before the route, and then
// certified.EverActive: while no certificate has existed no tree has a core link
// and this function is not called, so a reparse without edits costs one flag read
// more than before and no extra call.
func (p *Parser) ccoreUnchangedTreeAllowed(oldTree *Tree, usingProductionDFA bool) bool {
	st := oldTree.ccoreStateOf()
	if ccoreUnset() {
		return st == nil
	}
	if !p.admissionCandidateFullParseEligible(nil, usingProductionDFA) { // a request with a caller token source is the present engine's
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
	if ccoreUnset() || p.ccoreSuppressed() || p.recoveryInitialOnly || !p.admissionCandidateFullParseEligible(nil, true) {
		return nil, false
	}
	tree, ok := p.ccoreFullParse(source)
	if ok && (p.forestDeclineMemo == nil || !p.forestDeclineMemo.admissionCountersSuppressed) {
		admissionCandidateRouted.Add(1) // as attemptAdmissionCandidateFullParse counts a request the candidate route served
	}
	return tree, ok
}

// ccoreParseCore is the point where a future old-tree path can be added.
func ccoreParseCore(cp *ccore.Parser, source []byte) (*ccore.Tree, string) {
	return cp.ParseWith(source, nil)
}
