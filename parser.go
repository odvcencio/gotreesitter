package gotreesitter

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

// Parser reads parse tables from a Language and produces a syntax tree.
// It supports GLR parsing: when a (state, symbol) pair maps to multiple
// actions, the parser forks the stack and explores all alternatives in
// parallel while preserving distinct parse paths. Duplicate stack
// versions are collapsed and ambiguities are resolved at selection time.
type Parser struct {
	language          *Language
	reuseCursor       reuseCursor
	reuseScratch      reuseScratch
	reuseMu           sync.Mutex
	fullArenaHint     uint32
	hasRecoverState   []bool
	hasRecoverSymbol  []bool
	recoverByState    [][]recoverSymbolAction
	hasKeywordState   []bool
	forceRawSpanAll   bool
	forceRawSpanTable []bool
	included          []Range
	denseLimit        int
	smallBase         int
	smallLookup       [][]smallActionPair
}

type smallActionPair struct {
	sym uint16
	val uint16
}

type recoverSymbolAction struct {
	sym    uint16
	action ParseAction
}

type parserScratch struct {
	merge      glrMergeScratch
	entries    glrEntryScratch
	gss        gssScratch
	tmpEntries []stackEntry
	glrStates  []StateID
}

var parserScratchPool = sync.Pool{
	New: func() any {
		return &parserScratch{}
	},
}

// IncrementalParseProfile attributes incremental parse time into coarse buckets.
//
// ReuseCursorNanos includes reuse-cursor setup and subtree-candidate checks.
// ReparseNanos includes the remainder of incremental parsing/rebuild work.
type IncrementalParseProfile struct {
	ReuseCursorNanos   int64
	ReparseNanos       int64
	ReusedSubtrees     uint64
	ReusedBytes        uint64
	NewNodesAllocated  uint64
	RecoverSearches    uint64
	RecoverStateChecks uint64
	RecoverStateSkips  uint64
	RecoverSymbolSkips uint64
	RecoverLookups     uint64
	RecoverHits        uint64
	MaxStacksSeen      int
	EntryScratchPeak   uint64
}

type incrementalParseTiming struct {
	totalNanos         int64
	reuseNanos         int64
	reusedSubtrees     uint64
	reusedBytes        uint64
	newNodes           uint64
	recoverSearches    uint64
	recoverStateChecks uint64
	recoverStateSkips  uint64
	recoverSymbolSkips uint64
	recoverLookups     uint64
	recoverHits        uint64
	maxStacksSeen      int
	entryScratchPeak   uint64
}

type parseReuseState struct {
	reusedAny bool
	arenaRefs []*nodeArena
}

func (s *parseReuseState) markReused(node *Node, primary *nodeArena) {
	if s == nil {
		return
	}
	s.reusedAny = true
	if node == nil {
		return
	}
	s.arenaRefs = appendUniqueArenaRef(s.arenaRefs, node.ownerArena, primary)
}

func (s *parseReuseState) retainBorrowed(primary *nodeArena) []*nodeArena {
	if s == nil || !s.reusedAny || len(s.arenaRefs) == 0 {
		return nil
	}
	uniq := uniqueArenas(s.arenaRefs, primary)
	if len(uniq) == 0 {
		return nil
	}
	for _, a := range uniq {
		a.Retain()
	}
	return uniq
}

func (t *incrementalParseTiming) toProfile() IncrementalParseProfile {
	if t == nil {
		return IncrementalParseProfile{}
	}
	reparse := t.totalNanos - t.reuseNanos
	if reparse < 0 {
		reparse = 0
	}
	return IncrementalParseProfile{
		ReuseCursorNanos:   t.reuseNanos,
		ReparseNanos:       reparse,
		ReusedSubtrees:     t.reusedSubtrees,
		ReusedBytes:        t.reusedBytes,
		NewNodesAllocated:  t.newNodes,
		RecoverSearches:    t.recoverSearches,
		RecoverStateChecks: t.recoverStateChecks,
		RecoverStateSkips:  t.recoverStateSkips,
		RecoverSymbolSkips: t.recoverSymbolSkips,
		RecoverLookups:     t.recoverLookups,
		RecoverHits:        t.recoverHits,
		MaxStacksSeen:      t.maxStacksSeen,
		EntryScratchPeak:   t.entryScratchPeak,
	}
}

func acquireParserScratch() *parserScratch {
	return parserScratchPool.Get().(*parserScratch)
}

func releaseParserScratch(s *parserScratch) {
	if s == nil {
		return
	}
	if len(s.merge.result) > 0 {
		clear(s.merge.result)
	}
	s.merge.result = s.merge.result[:0]
	if len(s.merge.slots) > 0 {
		s.merge.slots = s.merge.slots[:0]
	}
	s.merge.perKeyCap = 0
	if cap(s.tmpEntries) > 0 {
		buf := s.tmpEntries[:cap(s.tmpEntries)]
		clear(buf)
		if cap(buf) > maxRetainedStackEntryCap {
			s.tmpEntries = nil
		} else {
			s.tmpEntries = buf[:0]
		}
	}
	if cap(s.glrStates) > maxGLRStacks {
		s.glrStates = nil
	} else if len(s.glrStates) > 0 {
		s.glrStates = s.glrStates[:0]
	}
	s.entries.reset()
	s.gss.reset()
	parserScratchPool.Put(s)
}

// NewParser creates a new Parser for the given language.
func NewParser(lang *Language) *Parser {
	p := &Parser{language: lang}
	if lang != nil {
		p.forceRawSpanAll = lang.Name == "yaml"
		for i, name := range lang.SymbolNames {
			if name != "statement_list" {
				continue
			}
			if p.forceRawSpanTable == nil {
				p.forceRawSpanTable = make([]bool, len(lang.SymbolNames))
			}
			p.forceRawSpanTable[i] = true
		}
		if lang.LargeStateCount > 0 {
			p.denseLimit = int(lang.LargeStateCount)
		} else {
			p.denseLimit = len(lang.ParseTable)
		}
		p.smallBase = int(lang.LargeStateCount)
		if len(lang.SmallParseTableMap) > 0 && len(lang.SmallParseTable) > 0 {
			p.smallLookup = buildSmallLookup(lang)
		}
		p.recoverByState, p.hasRecoverState, p.hasRecoverSymbol = buildRecoverActionsByState(lang)
		p.hasKeywordState = buildKeywordStates(lang)
	}
	return p
}

func buildStateRecoverTable(lang *Language) []bool {
	_, table, _ := buildRecoverActionsByState(lang)
	return table
}

func buildKeywordStates(lang *Language) []bool {
	if lang == nil || lang.KeywordCaptureToken == 0 || len(lang.KeywordLexStates) == 0 {
		return nil
	}

	symbolCount := int(lang.SymbolCount)
	if symbolCount <= 0 {
		symbolCount = len(lang.SymbolNames)
	}
	if symbolCount <= 0 {
		symbolCount = 64
	}
	keywordSymbols := make([]bool, symbolCount)
	ensureSymbolCap := func(sym Symbol) {
		idx := int(sym)
		if idx < len(keywordSymbols) {
			return
		}
		newLen := len(keywordSymbols)
		if newLen == 0 {
			newLen = 64
		}
		for idx >= newLen {
			newLen *= 2
		}
		expanded := make([]bool, newLen)
		copy(expanded, keywordSymbols)
		keywordSymbols = expanded
	}

	keywordCount := 0
	for i := range lang.KeywordLexStates {
		sym := lang.KeywordLexStates[i].AcceptToken
		if sym == 0 || sym == lang.KeywordCaptureToken {
			continue
		}
		ensureSymbolCap(sym)
		if !keywordSymbols[sym] {
			keywordSymbols[sym] = true
			keywordCount++
		}
	}
	if keywordCount == 0 {
		return nil
	}

	stateCount := int(lang.StateCount)
	if stateCount <= 0 {
		stateCount = len(lang.ParseTable)
	}
	if smallCount := int(lang.LargeStateCount) + len(lang.SmallParseTableMap); smallCount > stateCount {
		stateCount = smallCount
	}
	if stateCount <= 0 {
		return nil
	}

	hasKeyword := make([]bool, stateCount)
	anyState := false
	for state := 0; state < len(lang.ParseTable) && state < stateCount; state++ {
		row := lang.ParseTable[state]
		for sym, idx := range row {
			if idx == 0 {
				continue
			}
			if sym < len(keywordSymbols) && keywordSymbols[sym] {
				hasKeyword[state] = true
				anyState = true
				break
			}
		}
	}

	if len(lang.SmallParseTableMap) > 0 && len(lang.SmallParseTable) > 0 {
		base := int(lang.LargeStateCount)
		table := lang.SmallParseTable
		for smallIdx, offset := range lang.SmallParseTableMap {
			state := base + smallIdx
			if state < 0 || state >= stateCount || hasKeyword[state] {
				continue
			}
			pos := int(offset)
			if pos >= len(table) {
				continue
			}
			groupCount := int(table[pos])
			pos++
			for i := 0; i < groupCount; i++ {
				if pos+1 >= len(table) {
					break
				}
				sectionValue := table[pos]
				groupSymbolCount := int(table[pos+1])
				pos += 2
				end := pos + groupSymbolCount
				if end > len(table) {
					end = len(table)
				}
				if sectionValue != 0 {
					for _, sym := range table[pos:end] {
						if int(sym) < len(keywordSymbols) && keywordSymbols[sym] {
							hasKeyword[state] = true
							anyState = true
							break
						}
					}
				}
				pos = end
				if hasKeyword[state] {
					break
				}
			}
		}
	}

	if !anyState {
		return nil
	}
	return hasKeyword
}

func buildRecoverActionsByState(lang *Language) ([][]recoverSymbolAction, []bool, []bool) {
	if lang == nil {
		return nil, nil, nil
	}
	if len(lang.ParseActions) == 0 {
		return nil, nil, nil
	}

	recoverActions := make([]ParseAction, len(lang.ParseActions))
	hasRecoverAction := make([]bool, len(lang.ParseActions))
	anyRecoverAction := false
	for i := range lang.ParseActions {
		act, ok := recoverAction(&lang.ParseActions[i])
		if !ok {
			continue
		}
		recoverActions[i] = act
		hasRecoverAction[i] = true
		anyRecoverAction = true
	}
	if !anyRecoverAction {
		return nil, nil, nil
	}

	stateCount := int(lang.StateCount)
	if stateCount <= 0 {
		stateCount = len(lang.ParseTable)
	}
	if smallCount := int(lang.LargeStateCount) + len(lang.SmallParseTableMap); smallCount > stateCount {
		stateCount = smallCount
	}
	if stateCount <= 0 {
		return nil, nil, nil
	}
	symbolCount := int(lang.SymbolCount)
	if symbolCount <= 0 {
		symbolCount = len(lang.SymbolNames)
	}
	if symbolCount <= 0 {
		symbolCount = 64
	}

	recoverByState := make([][]recoverSymbolAction, stateCount)
	hasRecoverSymbol := make([]bool, symbolCount)
	ensureSymbolCap := func(sym uint16) {
		idx := int(sym)
		if idx < len(hasRecoverSymbol) {
			return
		}
		newLen := len(hasRecoverSymbol)
		if newLen == 0 {
			newLen = 64
		}
		for idx >= newLen {
			newLen *= 2
		}
		expanded := make([]bool, newLen)
		copy(expanded, hasRecoverSymbol)
		hasRecoverSymbol = expanded
	}
	for state := 0; state < len(lang.ParseTable) && state < stateCount; state++ {
		row := lang.ParseTable[state]
		for sym, idx := range row {
			if int(idx) < len(hasRecoverAction) && hasRecoverAction[idx] {
				ensureSymbolCap(uint16(sym))
				hasRecoverSymbol[sym] = true
				recoverByState[state] = append(recoverByState[state], recoverSymbolAction{
					sym:    uint16(sym),
					action: recoverActions[idx],
				})
			}
		}
	}

	if len(lang.SmallParseTableMap) > 0 && len(lang.SmallParseTable) > 0 {
		base := int(lang.LargeStateCount)
		table := lang.SmallParseTable
		for smallIdx, offset := range lang.SmallParseTableMap {
			state := base + smallIdx
			if state < 0 || state >= stateCount {
				continue
			}
			pos := int(offset)
			if pos >= len(table) {
				continue
			}
			groupCount := table[pos]
			pos++
			for i := uint16(0); i < groupCount; i++ {
				if pos+1 >= len(table) {
					break
				}
				sectionValue := table[pos]
				symbolCount := int(table[pos+1])
				pos += 2
				hasRecover := int(sectionValue) < len(hasRecoverAction) && hasRecoverAction[sectionValue]
				for j := 0; j < symbolCount; j++ {
					if pos >= len(table) {
						break
					}
					if hasRecover {
						ensureSymbolCap(table[pos])
						hasRecoverSymbol[table[pos]] = true
						recoverByState[state] = append(recoverByState[state], recoverSymbolAction{
							sym:    table[pos],
							action: recoverActions[sectionValue],
						})
					}
					pos++
				}
			}
		}
	}

	hasRecoverState := make([]bool, stateCount)
	anyState := false
	for i := range recoverByState {
		if len(recoverByState[i]) > 0 {
			hasRecoverState[i] = true
			anyState = true
		}
	}
	if !anyState {
		return nil, nil, nil
	}
	anySymbol := false
	for i := range hasRecoverSymbol {
		if hasRecoverSymbol[i] {
			anySymbol = true
			break
		}
	}
	if !anySymbol {
		hasRecoverSymbol = nil
	}
	return recoverByState, hasRecoverState, hasRecoverSymbol
}

func (p *Parser) stateCanRecover(state StateID) bool {
	if len(p.hasRecoverState) == 0 {
		return true
	}
	idx := int(state)
	return idx >= 0 && idx < len(p.hasRecoverState) && p.hasRecoverState[idx]
}

func (p *Parser) symbolCanRecover(sym Symbol) bool {
	if len(p.hasRecoverSymbol) == 0 {
		return true
	}
	idx := int(sym)
	return idx >= 0 && idx < len(p.hasRecoverSymbol) && p.hasRecoverSymbol[idx]
}

func (p *Parser) recoverActionForState(state StateID, sym Symbol) (ParseAction, bool) {
	if len(p.recoverByState) == 0 {
		return ParseAction{}, false
	}
	idx := int(state)
	if idx < 0 || idx >= len(p.recoverByState) {
		return ParseAction{}, false
	}
	entries := p.recoverByState[idx]
	if len(entries) == 0 {
		return ParseAction{}, false
	}
	target := uint16(sym)
	for i := range entries {
		if entries[i].sym == target {
			return entries[i].action, true
		}
	}
	return ParseAction{}, false
}

func buildSmallLookup(lang *Language) [][]smallActionPair {
	out := make([][]smallActionPair, len(lang.SmallParseTableMap))
	table := lang.SmallParseTable
	for smallIdx, offset := range lang.SmallParseTableMap {
		pos := int(offset)
		if pos >= len(table) {
			continue
		}
		groupCount := table[pos]
		pos++
		total := 0
		countPos := pos
		for i := uint16(0); i < groupCount; i++ {
			if countPos+1 >= len(table) {
				total = 0
				break
			}
			symbolCount := int(table[countPos+1])
			total += symbolCount
			countPos += 2 + symbolCount
		}
		if total == 0 {
			continue
		}

		pairs := make([]smallActionPair, 0, total)
		for i := uint16(0); i < groupCount; i++ {
			if pos+1 >= len(table) {
				break
			}
			val := table[pos]
			symbolCount := table[pos+1]
			pos += 2
			for j := uint16(0); j < symbolCount; j++ {
				if pos >= len(table) {
					break
				}
				pairs = append(pairs, smallActionPair{sym: table[pos], val: val})
				pos++
			}
		}
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].sym < pairs[j].sym })
		out[smallIdx] = pairs
	}
	return out
}

// SetIncludedRanges configures parser include ranges.
// Tokens outside these ranges are skipped.
func (p *Parser) SetIncludedRanges(ranges []Range) {
	if p == nil {
		return
	}
	p.included = normalizeIncludedRanges(ranges)
}

// IncludedRanges returns a copy of the configured include ranges.
func (p *Parser) IncludedRanges() []Range {
	if p == nil || len(p.included) == 0 {
		return nil
	}
	out := make([]Range, len(p.included))
	copy(out, p.included)
	return out
}

func (p *Parser) wrapIncludedRanges(ts TokenSource) TokenSource {
	if p == nil || len(p.included) == 0 || ts == nil {
		return ts
	}
	return newIncludedRangeTokenSource(ts, p.included)
}

// TokenSource provides tokens to the parser. This interface abstracts over
// different lexer implementations: the built-in DFA lexer (for hand-built
// grammars) or custom bridges like GoTokenSource (for real grammars where
// we can't extract the C lexer DFA).
type TokenSource interface {
	// Next returns the next token. It should skip whitespace and comments
	// as appropriate for the language. Returns a zero-Symbol token at EOF.
	Next() Token
}

// ByteSkippableTokenSource can jump to a byte offset and return the first
// token at or after that position.
type ByteSkippableTokenSource interface {
	TokenSource
	SkipToByte(offset uint32) Token
}

// PointSkippableTokenSource extends ByteSkippableTokenSource with a hint-based
// skip that avoids recomputing row/column from byte offset. During incremental
// parsing the reused node already carries its endpoint, so passing it directly
// eliminates the O(n) offset-to-point scan.
type PointSkippableTokenSource interface {
	ByteSkippableTokenSource
	SkipToByteWithPoint(offset uint32, pt Point) Token
}

type parserStateTokenSource interface {
	SetParserState(state StateID)
	// SetGLRStates provides all active GLR stack states so the token source
	// can compute valid external symbols as the union across all stacks.
	// This is critical for grammars with external scanners and GLR conflicts.
	SetGLRStates(states []StateID)
}

// stackEntry is a single entry on the parser's LR stack, pairing a parser
// state with the syntax tree node that was shifted or reduced into that state.
type stackEntry struct {
	state StateID
	node  *Node
}

// errorSymbol is the well-known symbol ID used for error nodes.
const errorSymbol = Symbol(65535)

// Parse tokenizes and parses source using the built-in DFA lexer, returning
// a syntax tree. This works for hand-built grammars that provide LexStates.
// For real grammars that need a custom lexer, use ParseWithTokenSource.
// If the input is empty, it returns a tree with a nil root and no error.
func (p *Parser) Parse(source []byte) (*Tree, error) {
	if err := p.checkLanguageCompatible(); err != nil {
		return nil, err
	}
	if err := p.checkDFALexer(); err != nil {
		return nil, err
	}
	lexer := NewLexer(p.language.LexStates, source)
	ts := &dfaTokenSource{
		lexer:             lexer,
		language:          p.language,
		lookupActionIndex: p.lookupActionIndex,
		hasKeywordState:   p.hasKeywordState,
	}
	if p.language.ExternalScanner != nil {
		ts.externalPayload = p.language.ExternalScanner.Create()
	}
	return p.parseInternal(source, p.wrapIncludedRanges(ts), nil, nil, arenaClassFull, nil), nil
}

// ParseWithTokenSource parses source using a custom token source.
// This is used for real grammars where the lexer DFA isn't available
// as data tables (e.g., Go grammar using go/scanner as a bridge).
func (p *Parser) ParseWithTokenSource(source []byte, ts TokenSource) (*Tree, error) {
	if err := p.checkLanguageCompatible(); err != nil {
		return nil, err
	}
	return p.parseInternal(source, p.wrapIncludedRanges(ts), nil, nil, arenaClassFull, nil), nil
}

// ParseIncremental re-parses source after edits were applied to oldTree.
// It reuses unchanged subtrees from the old tree for better performance.
// Call oldTree.Edit() for each edit before calling this method.
func (p *Parser) ParseIncremental(source []byte, oldTree *Tree) (*Tree, error) {
	if err := p.checkLanguageCompatible(); err != nil {
		return nil, err
	}
	if err := p.checkDFALexer(); err != nil {
		return nil, err
	}
	if canReuseUnchangedTree(source, oldTree, p.language) {
		return oldTree, nil
	}
	lexer := NewLexer(p.language.LexStates, source)
	ts := &dfaTokenSource{
		lexer:             lexer,
		language:          p.language,
		lookupActionIndex: p.lookupActionIndex,
		hasKeywordState:   p.hasKeywordState,
	}
	if p.language.ExternalScanner != nil {
		ts.externalPayload = p.language.ExternalScanner.Create()
	}
	return p.parseIncrementalInternal(source, oldTree, p.wrapIncludedRanges(ts), nil), nil
}

// ParseIncrementalWithTokenSource is like ParseIncremental but uses a custom
// token source.
func (p *Parser) ParseIncrementalWithTokenSource(source []byte, oldTree *Tree, ts TokenSource) (*Tree, error) {
	if err := p.checkLanguageCompatible(); err != nil {
		return nil, err
	}
	if canReuseUnchangedTree(source, oldTree, p.language) {
		return oldTree, nil
	}
	return p.parseIncrementalInternal(source, oldTree, p.wrapIncludedRanges(ts), nil), nil
}

// ParseIncrementalProfiled is like ParseIncremental and also returns runtime
// attribution for incremental reuse work vs parse/rebuild work.
func (p *Parser) ParseIncrementalProfiled(source []byte, oldTree *Tree) (*Tree, IncrementalParseProfile, error) {
	if err := p.checkLanguageCompatible(); err != nil {
		return nil, IncrementalParseProfile{}, err
	}
	if err := p.checkDFALexer(); err != nil {
		return nil, IncrementalParseProfile{}, err
	}
	if canReuseUnchangedTree(source, oldTree, p.language) {
		return oldTree, IncrementalParseProfile{}, nil
	}
	lexer := NewLexer(p.language.LexStates, source)
	ts := &dfaTokenSource{
		lexer:             lexer,
		language:          p.language,
		lookupActionIndex: p.lookupActionIndex,
		hasKeywordState:   p.hasKeywordState,
	}
	if p.language.ExternalScanner != nil {
		ts.externalPayload = p.language.ExternalScanner.Create()
	}
	timing := &incrementalParseTiming{}
	tree := p.parseIncrementalInternal(source, oldTree, p.wrapIncludedRanges(ts), timing)
	return tree, timing.toProfile(), nil
}

// ParseIncrementalWithTokenSourceProfiled is like ParseIncrementalWithTokenSource
// and also returns runtime attribution for incremental reuse work vs parse/rebuild work.
func (p *Parser) ParseIncrementalWithTokenSourceProfiled(source []byte, oldTree *Tree, ts TokenSource) (*Tree, IncrementalParseProfile, error) {
	if err := p.checkLanguageCompatible(); err != nil {
		return nil, IncrementalParseProfile{}, err
	}
	if canReuseUnchangedTree(source, oldTree, p.language) {
		return oldTree, IncrementalParseProfile{}, nil
	}
	timing := &incrementalParseTiming{}
	tree := p.parseIncrementalInternal(source, oldTree, p.wrapIncludedRanges(ts), timing)
	return tree, timing.toProfile(), nil
}

// ErrNoLanguage is returned when a Parser has no language configured.
var ErrNoLanguage = errors.New("parser has no language configured")

// checkLanguageCompatible returns an error if the parser's language is nil or
// incompatible with the runtime.
func (p *Parser) checkLanguageCompatible() error {
	if p.language == nil {
		return ErrNoLanguage
	}
	if !p.language.CompatibleWithRuntime() {
		return fmt.Errorf("language version %d incompatible with parser", p.language.LanguageVersion)
	}
	return nil
}

// checkDFALexer returns an error if the parser's language has no DFA lexer tables.
func (p *Parser) checkDFALexer() error {
	if p.language == nil || len(p.language.LexStates) == 0 {
		return fmt.Errorf("no DFA lexer available for language (use ParseWithTokenSource instead)")
	}
	return nil
}

func (p *Parser) parseIncrementalInternal(source []byte, oldTree *Tree, ts TokenSource, timing *incrementalParseTiming) *Tree {
	// Fast path: unchanged source and no recorded edits.
	if canReuseUnchangedTree(source, oldTree, p.language) {
		return oldTree
	}

	// DFA with external scanner cannot safely skip — scanner state must be
	// preserved token-by-token, so reuse is disabled for that combination.
	// All other token sources (DFA without external scanner, custom lexers)
	// support efficient skipping via PointSkippableTokenSource.
	if _, isDFA := ts.(*dfaTokenSource); isDFA && p.language != nil && p.language.ExternalScanner != nil {
		return p.parseInternal(source, ts, nil, nil, arenaClassFull, timing)
	}

	p.reuseMu.Lock()
	defer p.reuseMu.Unlock()

	var reuse *reuseCursor
	if timing != nil {
		reuseStart := time.Now()
		reuse = p.reuseCursor.reset(oldTree, source, &p.reuseScratch)
		timing.reuseNanos += time.Since(reuseStart).Nanoseconds()
	} else {
		reuse = p.reuseCursor.reset(oldTree, source, &p.reuseScratch)
	}
	arenaClass := incrementalArenaClassForSource(source)
	tree := p.parseInternal(source, ts, reuse, oldTree, arenaClass, timing)
	if reuse != nil {
		if timing != nil {
			reuseStart := time.Now()
			reuse.commitScratch(&p.reuseScratch)
			timing.reuseNanos += time.Since(reuseStart).Nanoseconds()
		} else {
			reuse.commitScratch(&p.reuseScratch)
		}
	}
	return tree
}

func incrementalArenaClassForSource(source []byte) arenaClass {
	arenaClass := arenaClassIncremental
	// Very large files can outgrow incremental defaults and trigger repeated
	// fallback allocations; use full-parse slab sizing only beyond this point.
	const incrementalUseFullArenaThreshold = 1 * 1024 * 1024
	if len(source) >= incrementalUseFullArenaThreshold {
		arenaClass = arenaClassFull
	}
	return arenaClass
}

func canReuseUnchangedTree(source []byte, oldTree *Tree, lang *Language) bool {
	if oldTree == nil || oldTree.language != lang || len(oldTree.edits) != 0 {
		return false
	}
	oldSource := oldTree.source
	if len(oldSource) != len(source) {
		return false
	}
	if len(source) == 0 {
		return true
	}
	// Common incremental no-edit case: caller passes the same source slice.
	// Pointer equality avoids memcmp on hot no-op reparses.
	if &oldSource[0] == &source[0] {
		return true
	}
	return bytes.Equal(oldSource, source)
}

// dfaTokenSource wraps the built-in DFA Lexer as a TokenSource.
// It tracks the current parser state to select the correct lex mode.
type dfaTokenSource struct {
	lexer    *Lexer
	language *Language
	state    StateID

	lookupActionIndex func(state StateID, sym Symbol) uint16
	hasKeywordState   []bool
	externalPayload   any
	externalValid     []bool
	glrStates         []StateID // all active GLR stack states
}

func (d *dfaTokenSource) Close() {
	if d.language == nil || d.language.ExternalScanner == nil || d.externalPayload == nil {
		return
	}
	d.language.ExternalScanner.Destroy(d.externalPayload)
	d.externalPayload = nil
}

// DebugDFA enables trace logging for DFA token production.
var DebugDFA bool

func (d *dfaTokenSource) Next() Token {
	startPos := 0
	if perfCountersEnabled {
		startPos = d.lexer.pos
	}
	if tok, ok := d.nextExternalToken(); ok {
		if perfCountersEnabled {
			consumed := d.lexer.pos - startPos
			if consumed < 0 {
				consumed = 0
			}
			perfRecordLexed(consumed, 1)
		}
		if DebugDFA {
			name := ""
			if int(tok.Symbol) < len(d.language.SymbolNames) {
				name = d.language.SymbolNames[tok.Symbol]
			}
			println("  EXT tok", tok.Symbol, name, tok.StartByte, tok.EndByte, tok.Text)
		}
		return tok
	}

	lexState := uint16(0)
	if int(d.state) < len(d.language.LexModes) {
		lexState = d.language.LexModes[d.state].LexState
	}
	tok := d.lexer.Next(lexState)
	tok = d.promoteKeyword(tok)
	if perfCountersEnabled {
		consumed := d.lexer.pos - startPos
		if consumed < 0 {
			consumed = 0
		}
		perfRecordLexed(consumed, 1)
	}
	if DebugDFA {
		name := ""
		if int(tok.Symbol) < len(d.language.SymbolNames) {
			name = d.language.SymbolNames[tok.Symbol]
		}
		println("  DFA tok", tok.Symbol, name, tok.StartByte, tok.EndByte, tok.Text, "state=", d.state, "lexState=", lexState)
	}
	return tok
}

func (d *dfaTokenSource) SetParserState(state StateID) {
	d.state = state
}

func (d *dfaTokenSource) SetGLRStates(states []StateID) {
	d.glrStates = states
}

func (d *dfaTokenSource) SkipToByte(offset uint32) Token {
	target := int(offset)
	if target < d.lexer.pos {
		// Rewind isn't supported for DFA token sources during parse.
		return d.Next()
	}
	startPos := 0
	if perfCountersEnabled {
		startPos = d.lexer.pos
	}
	for d.lexer.pos < target {
		d.lexer.skipOneRune()
	}
	if perfCountersEnabled {
		consumed := d.lexer.pos - startPos
		if consumed < 0 {
			consumed = 0
		}
		perfRecordLexed(consumed, 0)
	}
	return d.Next()
}

func (d *dfaTokenSource) SkipToByteWithPoint(offset uint32, pt Point) Token {
	target := int(offset)
	if target > len(d.lexer.source) {
		target = len(d.lexer.source)
	}
	if target >= d.lexer.pos {
		d.lexer.pos = target
		d.lexer.row = pt.Row
		d.lexer.col = pt.Column
	}
	return d.Next()
}

func (d *dfaTokenSource) nextExternalToken() (Token, bool) {
	if d.language == nil || d.lookupActionIndex == nil {
		return Token{}, false
	}
	if len(d.language.ExternalSymbols) == 0 {
		return Token{}, false
	}

	if cap(d.externalValid) < len(d.language.ExternalSymbols) {
		d.externalValid = make([]bool, len(d.language.ExternalSymbols))
	}
	valid := d.externalValid[:len(d.language.ExternalSymbols)]
	for i := range valid {
		valid[i] = false
	}

	// Compute valid external symbols as the union across all active GLR
	// stacks. Different stacks may be in different parser states with
	// different valid external tokens. The scanner needs to see the union
	// so it can produce tokens that any stack might need. Stacks that
	// can't use the resulting token will be pruned by the action phase.
	anyValid := false
	states := d.glrStates
	if len(states) == 0 {
		states = []StateID{d.state}
	}
	for _, st := range states {
		for i, sym := range d.language.ExternalSymbols {
			if !valid[i] && d.lookupActionIndex(st, sym) != 0 {
				valid[i] = true
				anyValid = true
			}
		}
	}
	if !anyValid {
		return Token{}, false
	}

	if d.language.ExternalScanner == nil {
		return d.syntheticExternalToken(valid)
	}

	el := newExternalLexer(d.lexer.source, d.lexer.pos, d.lexer.row, d.lexer.col)
	if !RunExternalScanner(d.language, d.externalPayload, el, valid) {
		return Token{}, false
	}
	tok, ok := el.token()
	if !ok {
		return Token{}, false
	}

	d.lexer.pos = int(tok.EndByte)
	d.lexer.row = tok.EndPoint.Row
	d.lexer.col = tok.EndPoint.Column
	return tok, true
}

func (d *dfaTokenSource) syntheticExternalToken(valid []bool) (Token, bool) {
	// Conservative fallback when no external scanner is registered:
	// synthesize automatic-semicolon style external tokens only when the
	// grammar explicitly allows them in the current state.
	if d.language == nil || d.lexer == nil {
		return Token{}, false
	}

	for i, sym := range d.language.ExternalSymbols {
		if i >= len(valid) || !valid[i] {
			continue
		}
		nameIdx := int(sym)
		if nameIdx < 0 || nameIdx >= len(d.language.SymbolNames) {
			continue
		}
		switch d.language.SymbolNames[nameIdx] {
		case "_automatic_semicolon", "_function_signature_automatic_semicolon", "_implicit_semicolon":
			return d.syntheticAutomaticSemicolon(sym)
		case "_line_break", "_newline":
			return d.syntheticLineBreak(sym)
		case "_line_ending_or_eof":
			return d.syntheticLineEndingOrEOF(sym)
		case "jsx_text":
			return d.syntheticJSXText(sym)
		}
	}

	return Token{}, false
}

func (d *dfaTokenSource) syntheticAutomaticSemicolon(sym Symbol) (Token, bool) {
	if d.lexer == nil {
		return Token{}, false
	}
	source := d.lexer.source
	startPos := d.lexer.pos
	startPoint := Point{Row: d.lexer.row, Column: d.lexer.col}

	// EOF insertion is always allowed when the grammar requests it.
	if startPos >= len(source) {
		return Token{
			Symbol:     sym,
			StartByte:  uint32(startPos),
			EndByte:    uint32(startPos),
			StartPoint: startPoint,
			EndPoint:   startPoint,
		}, true
	}

	pos := startPos
	endRow := d.lexer.row
	endCol := d.lexer.col
	sawLineBreak := false

	// Consume horizontal space, then allow insertion on line break or EOF.
	for pos < len(source) {
		switch source[pos] {
		case ' ', '\t', '\f':
			pos++
			endCol++
		case '\r':
			pos++
			if pos < len(source) && source[pos] == '\n' {
				pos++
			}
			endRow++
			endCol = 0
			sawLineBreak = true
			goto done
		case '\n':
			pos++
			endRow++
			endCol = 0
			sawLineBreak = true
			goto done
		default:
			return Token{}, false
		}
	}

	// Reached EOF after horizontal space.
	return Token{
		Symbol:     sym,
		StartByte:  uint32(startPos),
		EndByte:    uint32(pos),
		StartPoint: startPoint,
		EndPoint:   Point{Row: endRow, Column: endCol},
	}, true

done:
	if !sawLineBreak {
		return Token{}, false
	}

	// Consume indentation after newline so lexing resumes at next token.
	for pos < len(source) {
		switch source[pos] {
		case ' ', '\t', '\f':
			pos++
			endCol++
		default:
			return Token{
				Symbol:     sym,
				StartByte:  uint32(startPos),
				EndByte:    uint32(pos),
				StartPoint: startPoint,
				EndPoint:   Point{Row: endRow, Column: endCol},
			}, true
		}
	}

	return Token{
		Symbol:     sym,
		StartByte:  uint32(startPos),
		EndByte:    uint32(pos),
		StartPoint: startPoint,
		EndPoint:   Point{Row: endRow, Column: endCol},
	}, true
}

func (d *dfaTokenSource) syntheticLineBreak(sym Symbol) (Token, bool) {
	if d.lexer == nil {
		return Token{}, false
	}
	source := d.lexer.source
	startPos := d.lexer.pos
	startPoint := Point{Row: d.lexer.row, Column: d.lexer.col}

	pos := startPos
	endRow := d.lexer.row
	endCol := d.lexer.col

	for pos < len(source) {
		switch source[pos] {
		case ' ', '\t', '\f':
			pos++
			endCol++
		case '\r':
			pos++
			if pos < len(source) && source[pos] == '\n' {
				pos++
			}
			endRow++
			endCol = 0
			goto consumeIndent
		case '\n':
			pos++
			endRow++
			endCol = 0
			goto consumeIndent
		default:
			return Token{}, false
		}
	}

	return Token{}, false

consumeIndent:
	for pos < len(source) {
		switch source[pos] {
		case ' ', '\t', '\f':
			pos++
			endCol++
		default:
			return Token{
				Symbol:     sym,
				StartByte:  uint32(startPos),
				EndByte:    uint32(pos),
				StartPoint: startPoint,
				EndPoint:   Point{Row: endRow, Column: endCol},
			}, true
		}
	}

	return Token{
		Symbol:     sym,
		StartByte:  uint32(startPos),
		EndByte:    uint32(pos),
		StartPoint: startPoint,
		EndPoint:   Point{Row: endRow, Column: endCol},
	}, true
}

func (d *dfaTokenSource) syntheticLineEndingOrEOF(sym Symbol) (Token, bool) {
	if d.lexer == nil {
		return Token{}, false
	}
	if tok, ok := d.syntheticLineBreak(sym); ok {
		return tok, true
	}

	source := d.lexer.source
	startPos := d.lexer.pos
	startPoint := Point{Row: d.lexer.row, Column: d.lexer.col}
	if startPos >= len(source) {
		return Token{
			Symbol:     sym,
			StartByte:  uint32(startPos),
			EndByte:    uint32(startPos),
			StartPoint: startPoint,
			EndPoint:   startPoint,
		}, true
	}

	pos := startPos
	endCol := d.lexer.col
	for pos < len(source) {
		switch source[pos] {
		case ' ', '\t', '\f':
			pos++
			endCol++
		default:
			return Token{}, false
		}
	}

	return Token{
		Symbol:     sym,
		StartByte:  uint32(startPos),
		EndByte:    uint32(pos),
		StartPoint: startPoint,
		EndPoint:   Point{Row: d.lexer.row, Column: endCol},
	}, true
}

func (d *dfaTokenSource) syntheticJSXText(sym Symbol) (Token, bool) {
	if d.lexer == nil {
		return Token{}, false
	}
	source := d.lexer.source
	startPos := d.lexer.pos
	if startPos >= len(source) {
		return Token{}, false
	}

	switch source[startPos] {
	case '<', '{', '}':
		return Token{}, false
	}

	pos := startPos
	endRow := d.lexer.row
	endCol := d.lexer.col

	for pos < len(source) {
		switch source[pos] {
		case '<', '{', '}':
			if pos == startPos {
				return Token{}, false
			}
			startPoint := Point{Row: d.lexer.row, Column: d.lexer.col}
			return Token{
				Symbol:     sym,
				StartByte:  uint32(startPos),
				EndByte:    uint32(pos),
				StartPoint: startPoint,
				EndPoint:   Point{Row: endRow, Column: endCol},
			}, true
		case '\r':
			pos++
			if pos < len(source) && source[pos] == '\n' {
				pos++
			}
			endRow++
			endCol = 0
		case '\n':
			pos++
			endRow++
			endCol = 0
		default:
			_, size := utf8.DecodeRune(source[pos:])
			if size <= 0 {
				size = 1
			}
			pos += size
			endCol++
		}
	}

	if pos == startPos {
		return Token{}, false
	}
	startPoint := Point{Row: d.lexer.row, Column: d.lexer.col}
	return Token{
		Symbol:     sym,
		StartByte:  uint32(startPos),
		EndByte:    uint32(pos),
		StartPoint: startPoint,
		EndPoint:   Point{Row: endRow, Column: endCol},
	}, true
}

func (d *dfaTokenSource) promoteKeyword(tok Token) Token {
	if d.language == nil {
		return tok
	}
	if tok.Symbol == 0 {
		return tok
	}
	if len(d.language.KeywordLexStates) == 0 {
		return tok
	}
	if d.language.KeywordCaptureToken == 0 {
		return tok
	}
	if tok.Symbol != d.language.KeywordCaptureToken {
		return tok
	}
	if tok.EndByte <= tok.StartByte {
		return tok
	}
	if len(d.hasKeywordState) > 0 {
		state := int(d.state)
		if state >= 0 && state < len(d.hasKeywordState) && !d.hasKeywordState[state] {
			return tok
		}
	}

	start := int(tok.StartByte)
	end := int(tok.EndByte)
	if start < 0 || end < start || end > len(d.lexer.source) {
		return tok
	}

	kw := Lexer{
		states: d.language.KeywordLexStates,
		source: d.lexer.source[start:end],
	}
	kwTok := kw.Next(0)
	if kwTok.Symbol == 0 {
		return tok
	}
	if kwTok.StartByte != 0 {
		return tok
	}
	if kwTok.EndByte != uint32(end-start) {
		return tok
	}

	// Context-aware promotion: only use the keyword symbol if the current
	// parser state has a valid action for it. This prevents contextual
	// keywords like "get"/"set" from being promoted in positions where
	// they should be treated as identifiers (e.g., obj.get(...)).
	if d.lookupActionIndex != nil {
		kwHasAction := d.lookupActionIndex(d.state, kwTok.Symbol) != 0
		idHasAction := d.lookupActionIndex(d.state, tok.Symbol) != 0
		if !kwHasAction && idHasAction {
			return tok // parser expects identifier, not keyword
		}
	}

	tok.Symbol = kwTok.Symbol
	return tok
}

// parseIterations returns the iteration limit scaled to input size.
// A correctly-parsed file needs roughly (tokens * grammar_depth) iterations.
// For typical source (~5 bytes/token, ~10 reduce depth), that's sourceLen*2.
// We use sourceLen*20 as a generous upper bound that still prevents runaway
// parsing from OOMing the machine.
func parseIterations(sourceLen int) int {
	return max(10_000, sourceLen*20)
}

// parseStackDepth returns the stack depth limit scaled to input size.
func parseStackDepth(sourceLen int) int {
	return max(1_000, sourceLen*2)
}

// parseNodeLimit returns the maximum number of Node allocations allowed.
// This is the hard ceiling that prevents OOM regardless of iteration count.
func parseNodeLimit(sourceLen int) int {
	return max(50_000, sourceLen*10)
}

func parseFullArenaNodeCapacity(sourceLen, hint int) int {
	base := nodeCapacityForClass(arenaClassFull)
	if hint > 0 {
		if hint < base {
			return base
		}
		limit := parseNodeLimit(sourceLen)
		if sourceLen <= 0 {
			return max(base, hint)
		}
		if hint > limit {
			return max(base, limit)
		}
		return hint
	}
	if sourceLen <= 0 {
		return base
	}
	// Conservative first-pass sizing. We refine this with adaptive hints
	// from observed full-parse node usage.
	estimate := sourceLen * 6
	const maxPreallocNodes = 1_500_000
	if estimate > maxPreallocNodes {
		estimate = maxPreallocNodes
	}
	return max(base, estimate)
}

func (p *Parser) fullArenaHintCapacity() int {
	if p == nil {
		return 0
	}
	return int(atomic.LoadUint32(&p.fullArenaHint))
}

func (p *Parser) recordFullArenaUsage(used int) {
	if p == nil || used <= 0 {
		return
	}
	target := used + used/4 // keep 25% headroom above observed peak.
	base := nodeCapacityForClass(arenaClassFull)
	if target < base {
		target = base
	}
	const maxHintNodes = 2_000_000
	if target > maxHintNodes {
		target = maxHintNodes
	}

	for {
		old := atomic.LoadUint32(&p.fullArenaHint)
		var next uint32
		if old == 0 {
			next = uint32(target)
		} else {
			blended := (int(old)*3 + target) / 4
			if blended < base {
				blended = base
			}
			next = uint32(blended)
		}
		if old == next || atomic.CompareAndSwapUint32(&p.fullArenaHint, old, next) {
			return
		}
	}
}

func parseFullEntryScratchCapacity(sourceLen int) int {
	if sourceLen <= 0 {
		return defaultStackEntrySlabCap
	}
	estimate := sourceLen * 12
	if estimate < defaultStackEntrySlabCap {
		estimate = defaultStackEntrySlabCap
	}
	// Keep initial scratch growth bounded; larger capacities are still
	// reached on demand and retained up to maxRetainedStackEntryCap.
	const maxPreallocEntries = 768 * 1024
	if estimate > maxPreallocEntries {
		estimate = maxPreallocEntries
	}
	return estimate
}

func parseIncrementalArenaNodeCapacity(sourceLen int) int {
	base := nodeCapacityForClass(arenaClassIncremental)
	if sourceLen <= 0 {
		return base
	}
	estimate := sourceLen * 4
	const maxPreallocNodes = 512 * 1024
	if estimate > maxPreallocNodes {
		estimate = maxPreallocNodes
	}
	return max(base, estimate)
}

func parseIncrementalEntryScratchCapacity(sourceLen int) int {
	if sourceLen <= 0 {
		return defaultStackEntrySlabCap
	}
	estimate := sourceLen * 8
	if estimate < defaultStackEntrySlabCap {
		estimate = defaultStackEntrySlabCap
	}
	const maxPreallocEntries = 256 * 1024
	if estimate > maxPreallocEntries {
		estimate = maxPreallocEntries
	}
	return estimate
}

func parseErrorTree(source []byte, lang *Language) *Tree {
	end := Point{}
	for i := 0; i < len(source); {
		if source[i] == '\n' {
			end.Row++
			end.Column = 0
			i++
			continue
		}
		_, size := utf8.DecodeRune(source[i:])
		if size <= 0 {
			size = 1
		}
		i += size
		end.Column++
	}

	root := NewLeafNode(errorSymbol, false, 0, uint32(len(source)), Point{}, end)
	root.hasError = true
	return NewTree(root, source, lang)
}

func isWhitespaceOnlySource(source []byte) bool {
	for i := 0; i < len(source); i++ {
		switch source[i] {
		case ' ', '\t', '\n', '\r', '\f':
		default:
			return false
		}
	}
	return true
}

func extendNodeToTrailingWhitespace(n *Node, source []byte) {
	if n == nil {
		return
	}
	sourceEnd := uint32(len(source))
	if n.endByte >= sourceEnd {
		return
	}
	tail := source[n.endByte:sourceEnd]
	for i := 0; i < len(tail); i++ {
		switch tail[i] {
		case ' ', '\t', '\n', '\r', '\f':
		default:
			return
		}
	}

	pt := n.endPoint
	for i := 0; i < len(tail); {
		if tail[i] == '\n' {
			pt.Row++
			pt.Column = 0
			i++
			continue
		}
		_, size := utf8.DecodeRune(tail[i:])
		if size <= 0 {
			size = 1
		}
		i += size
		pt.Column++
	}

	n.endByte = sourceEnd
	n.endPoint = pt
}

// parseInternal is the core GLR parsing loop shared by Parse and
// ParseWithTokenSource.
//
// It maintains a set of parse stacks. For unambiguous grammars (single
// action per table entry), there is exactly one stack and the algorithm
// reduces to standard LR parsing. When multiple actions exist for a
// (state, symbol) pair, the parser forks: one stack per alternative.
// Stacks that error out are dropped. Only duplicate stack versions are
// merged; distinct alternatives are preserved.
func (p *Parser) parseInternal(source []byte, ts TokenSource, reuse *reuseCursor, oldTree *Tree, arenaClass arenaClass, timing *incrementalParseTiming) *Tree {
	var parseStart time.Time
	if timing != nil {
		parseStart = time.Now()
	}
	if closer, ok := ts.(interface{ Close() }); ok {
		defer closer.Close()
	}
	scratch := acquireParserScratch()
	defer releaseParserScratch(scratch)

	arena := acquireNodeArena(arenaClass)
	if timing != nil {
		startUsed := arena.used
		defer func() {
			timing.totalNanos += time.Since(parseStart).Nanoseconds()
			if arena.used >= startUsed {
				timing.newNodes += uint64(arena.used - startUsed)
			}
			peak := uint64(scratch.entries.peakEntriesUsed())
			if peak > timing.entryScratchPeak {
				timing.entryScratchPeak = peak
			}
		}()
	}
	if arenaClass == arenaClassFull {
		defer func() {
			p.recordFullArenaUsage(arena.used)
		}()
	}
	switch arenaClass {
	case arenaClassFull:
		arena.ensureNodeCapacity(parseFullArenaNodeCapacity(len(source), p.fullArenaHintCapacity()))
		scratch.entries.ensureInitialCap(parseFullEntryScratchCapacity(len(source)))
	case arenaClassIncremental:
		arena.ensureNodeCapacity(parseIncrementalArenaNodeCapacity(len(source)))
		scratch.entries.ensureInitialCap(parseIncrementalEntryScratchCapacity(len(source)))
	}
	var reuseState parseReuseState

	finalize := func(stacks []glrStack) *Tree {
		return p.buildResultFromGLR(stacks, source, arena, oldTree, &reuseState)
	}

	var stacksBuf [4]glrStack
	stacks := stacksBuf[:1]
	initialStackCap := 256 * 1024
	if reuse != nil {
		// Incremental reparses often borrow scratch slabs from an earlier full
		// parse. Preallocating that full retained capacity forces large memclr
		// work on every edit; keep incremental stack preallocation modest.
		initialStackCap = defaultStackEntrySlabCap
	}
	stacks[0] = newGLRStackWithScratchCap(p.language.InitialState, &scratch.entries, initialStackCap)
	stacks[0].recoverabilityKnown = true
	stacks[0].mayRecover = p.stateCanRecover(p.language.InitialState)
	if timing != nil && timing.maxStacksSeen < len(stacks) {
		timing.maxStacksSeen = len(stacks)
	}
	maxStacks := maxGLRStacks
	mergePerKeyCap := maxStacksPerMergeKey
	if reuse != nil {
		// Incremental reparses benefit from tighter GLR retention because
		// edits are localized and we prioritize latency over broad ambiguity fanout.
		maxStacks = 32
		mergePerKeyCap = 4
	}
	scratch.merge.perKeyCap = mergePerKeyCap

	maxIter := parseIterations(len(source))
	maxDepth := parseStackDepth(len(source))
	maxNodes := parseNodeLimit(len(source))
	nodeCount := 0

	needToken := true
	var tok Token
	var perfTokensConsumed uint64

	// Per-primary-stack infinite-reduce detection.
	var lastReduceState StateID
	var consecutiveReduces int

	for iter := 0; iter < maxIter; iter++ {
		if perfCountersEnabled {
			perfRecordMaxConcurrentStacks(len(stacks))
		}
		if timing != nil && len(stacks) > timing.maxStacksSeen {
			timing.maxStacksSeen = len(stacks)
		}
		// Fast-path the overwhelmingly common non-GLR case with one live stack.
		if len(stacks) == 1 {
			if stacks[0].dead {
				arena.Release()
				return parseErrorTree(source, p.language)
			}
		} else {
			// Prune dead stacks and collapse only truly duplicate stack versions.
			stacks = mergeStacksWithScratch(stacks, &scratch.merge)
			if len(stacks) == 0 {
				arena.Release()
				return parseErrorTree(source, p.language)
			}
		}
		// Cap the number of parallel stacks to prevent combinatorial explosion.
		// Keep the most promising stacks instead of truncating by insertion
		// order, which can discard viable parses on highly-ambiguous inputs.
		if len(stacks) > maxStacks {
			sort.SliceStable(stacks, func(i, j int) bool {
				return stackCompare(stacks[i], stacks[j]) > 0
			})
			stacks = stacks[:maxStacks]
		}

		// Keep the most promising stack in slot 0 because several parser
		// heuristics (lex-mode selection, reduce-loop detection, depth cap)
		// currently key off the primary stack.
		if len(stacks) > 1 {
			p.promotePrimaryStack(stacks)
		}
		for i := range stacks {
			stacks[i].cacheEntries = false
			if stacks[i].gss.head != nil {
				stacks[i].entries = nil
			}
		}

		// Safety: if the primary stack has grown beyond the depth cap,
		// or we've allocated too many nodes, return what we have.
		if stacks[0].depth() > maxDepth || nodeCount > maxNodes {
			return finalize(stacks)
		}

		// Use the primary (first) stack's state for DFA lex mode selection.
		// Pass all active GLR stack states so external scanner valid symbols
		// are computed as the union across all stacks.
		if stateful, ok := ts.(parserStateTokenSource); ok {
			stateful.SetParserState(stacks[0].top().state)
			if len(stacks) > 1 {
				glrBuf := scratch.glrStates[:0]
				if cap(glrBuf) < len(stacks) {
					glrBuf = make([]StateID, 0, len(stacks))
				}
				for si := range stacks {
					if !stacks[si].dead {
						glrBuf = append(glrBuf, stacks[si].top().state)
					}
				}
				scratch.glrStates = glrBuf
				stateful.SetGLRStates(glrBuf)
			} else {
				if len(scratch.glrStates) > 0 {
					scratch.glrStates = scratch.glrStates[:0]
				}
				stateful.SetGLRStates(nil)
			}
		}

		if needToken {
			tok = ts.Next()
			if perfCountersEnabled {
				perfTokensConsumed++
			}
			// Clear per-stack shifted flags so all stacks process the
			// new token.
			for si := range stacks {
				stacks[si].shifted = false
			}
		}

		// Incremental parsing fast-path: when there is a single active stack,
		// try to reuse an unchanged subtree starting at the current token.
		if reuse != nil && len(stacks) == 1 && !stacks[0].dead && tok.Symbol != 0 {
			if timing != nil {
				reuseStart := time.Now()
				nextTok, reusedBytes, ok := p.tryReuseSubtree(&stacks[0], tok, ts, reuse, &scratch.entries, &scratch.gss)
				timing.reuseNanos += time.Since(reuseStart).Nanoseconds()
				if ok {
					timing.reusedSubtrees++
					timing.reusedBytes += uint64(reusedBytes)
					reuseState.markReused(stacks[0].top().node, arena)
					tok = nextTok
					needToken = false
					consecutiveReduces = 0
					continue
				}
			} else {
				if nextTok, _, ok := p.tryReuseSubtree(&stacks[0], tok, ts, reuse, &scratch.entries, &scratch.gss); ok {
					reuseState.markReused(stacks[0].top().node, arena)
					tok = nextTok
					needToken = false
					consecutiveReduces = 0
					continue
				}
			}
		}

		// Process all alive stacks for this token.
		// We iterate by index because forks may append to `stacks`.
		numStacks := len(stacks)
		anyReduced := false

		parseActions := p.language.ParseActions
		for si := 0; si < numStacks; si++ {
			s := &stacks[si]
			if s.dead || s.shifted {
				continue
			}

			currentState := s.top().state
			actionIdx := p.lookupActionIndex(currentState, tok.Symbol)
			var actions []ParseAction
			if actionIdx != 0 && int(actionIdx) < len(parseActions) {
				actions = parseActions[actionIdx].Actions
			}

			// --- Extra token handling (comments, whitespace) ---
			if len(actions) > 0 &&
				actions[0].Type == ParseActionShift && actions[0].Extra {
				named := p.isNamedSymbol(tok.Symbol)
				leaf := newLeafNodeInArena(arena, tok.Symbol, named,
					tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
				leaf.isExtra = true
				leaf.preGotoState = currentState
				leaf.parseState = currentState
				p.pushStackNode(s, currentState, leaf, &scratch.entries, &scratch.gss)
				nodeCount++
				needToken = true
				continue
			}

			// --- No action: error handling ---
			if len(actions) == 0 {
				if tok.Symbol == 0 {
					if tok.StartByte == tok.EndByte {
						// True EOF. If this is the only stack, return result.
						if len(stacks) == 1 {
							return finalize(stacks)
						}
						// Multiple stacks at EOF: this one is done.
						// Mark dead so merge picks the best remaining.
						s.dead = true
						continue
					}
					// Zero-symbol width token: skip.
					needToken = true
					continue
				}

				// When multiple alternatives exist, drop no-action stacks
				// immediately instead of running deep recovery scans.
				if len(stacks) > 1 {
					s.dead = true
					continue
				}

				// Try grammar-directed recovery by searching the stack for
				// the nearest state that can recover on this lookahead.
				if depth, recoverAct, ok := p.findRecoverActionOnStack(s, tok.Symbol, timing); ok {
					if !s.truncate(depth + 1) {
						s.dead = true
						continue
					}
					p.applyAction(s, recoverAct, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries)
					needToken = true
					continue
				}

				// Only stack: error recovery — wrap token in error node.
				if s.depth() == 0 {
					return finalize(stacks)
				}
				errNode := newLeafNodeInArena(arena, errorSymbol, false,
					tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
				errNode.hasError = true
				errNode.parseState = currentState
				p.pushStackNode(s, currentState, errNode, &scratch.entries, &scratch.gss)
				nodeCount++
				needToken = true
				continue
			}

			// --- GLR: fork for multiple actions ---
			// For single-action entries (the common case), no fork occurs.
			// For multi-action entries, clone the stack for each alternative.
			if len(actions) > 1 {
				if reuse != nil {
					p.applyAction(s, actions[0], tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries)
					continue
				}
				if perfCountersEnabled {
					perfRecordFork(len(actions), perfTokensConsumed)
				}
				// Deep-stack GLR forks can trigger pathological clone volumes on
				// very large inputs. At extreme depths, take the primary action
				// to keep parsing bounded.
				const maxForkCloneDepth = 4 * 1024
				if s.depth() > maxForkCloneDepth {
					p.applyAction(s, actions[0], tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries)
					continue
				}
				// Copy the current stack value before appending forks.
				// Appending can reallocate `stacks`, which would invalidate `s`.
				base := *s
				for ai := 1; ai < len(actions); ai++ {
					fork := base.cloneWithScratch(&scratch.gss)
					p.applyAction(&fork, actions[ai], tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries)
					stacks = append(stacks, fork)
				}
				// Re-acquire the pointer after possible reallocation.
				s = &stacks[si]
				p.applyAction(s, actions[0], tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries)
			} else {
				p.applyAction(s, actions[0], tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries)
			}
		}

		// After processing all stacks: determine whether to advance the
		// token. If any stack reduced, reuse the same token (the reducing
		// stacks have new top states and need to re-check the action for
		// the current lookahead). Otherwise, advance to next token.
		if anyReduced {
			needToken = false

			// Infinite-reduce detection (for the primary stack).
			if len(stacks) > 0 && !stacks[0].dead {
				topState := stacks[0].top().state
				if topState == lastReduceState {
					consecutiveReduces++
				} else {
					lastReduceState = topState
					consecutiveReduces = 1
				}
				if consecutiveReduces > 10 {
					needToken = true
					consecutiveReduces = 0
				}
			}
		} else {
			needToken = true
			consecutiveReduces = 0
		}

		// Check for accept on any stack.
		for si := range stacks {
			if stacks[si].accepted {
				return finalize(stacks[si : si+1])
			}
		}
	}

	// Iteration limit reached.
	return finalize(stacks)
}

func appendUniqueArenaRef(refs []*nodeArena, arenaRef, exclude *nodeArena) []*nodeArena {
	if arenaRef == nil || arenaRef == exclude {
		return refs
	}
	for i := range refs {
		if refs[i] == arenaRef {
			return refs
		}
	}
	return append(refs, arenaRef)
}

func (p *Parser) promotePrimaryStack(stacks []glrStack) {
	if len(stacks) <= 1 {
		return
	}
	best := 0
	for i := 1; i < len(stacks); i++ {
		if stackCompare(stacks[i], stacks[best]) > 0 {
			best = i
		}
	}
	if best != 0 {
		stacks[0], stacks[best] = stacks[best], stacks[0]
	}
}

// applyAction applies a single parse action to a GLR stack.
func (p *Parser) applyAction(s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry) {
	switch act.Type {
	case ParseActionShift:
		named := p.isNamedSymbol(tok.Symbol)
		leaf := newLeafNodeInArena(arena, tok.Symbol, named,
			tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
		leaf.isExtra = act.Extra
		leaf.preGotoState = s.top().state
		leaf.parseState = act.State
		p.pushStackNode(s, act.State, leaf, entryScratch, gssScratch)
		s.shifted = true
		*nodeCount++

	case ParseActionReduce:
		entries := s.entries
		borrowed := false
		if entries == nil {
			if !s.cacheEntries && s.gss.head != nil {
				tmp := []stackEntry(nil)
				if tmpEntries != nil {
					tmp = *tmpEntries
				}
				p.applyReduceActionFromGSS(s, act, anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, tmp)
				return
			}
			if s.cacheEntries {
				entries = s.ensureEntries(entryScratch)
			} else {
				tmp := []stackEntry(nil)
				if tmpEntries != nil {
					tmp = *tmpEntries
				}
				entries, borrowed = s.entriesForRead(tmp)
			}
		}
		p.applyReduceAction(s, act, anyReduced, nodeCount, arena, entryScratch, gssScratch, entries)
		if borrowed && tmpEntries != nil {
			*tmpEntries = entries[:0]
		}

	case ParseActionAccept:
		s.accepted = true

	case ParseActionRecover:
		if tok.Symbol == 0 && tok.StartByte == tok.EndByte {
			s.accepted = true
			return
		}
		errNode := newLeafNodeInArena(arena, errorSymbol, false,
			tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
		errNode.hasError = true
		recoverState := s.top().state
		if act.State != 0 {
			recoverState = act.State
		}
		errNode.parseState = recoverState
		p.pushStackNode(s, recoverState, errNode, entryScratch, gssScratch)
		*nodeCount++
	}
}

func (p *Parser) pushStackNode(s *glrStack, state StateID, node *Node, entryScratch *glrEntryScratch, gssScratch *gssScratch) {
	s.push(state, node, entryScratch, gssScratch)
	if !s.recoverabilityKnown {
		return
	}
	if !s.mayRecover && p.stateCanRecover(state) {
		s.mayRecover = true
	}
}

func reduceWindowFromGSS(s *glrStack, childCount int, buf []stackEntry) ([]stackEntry, StateID, bool) {
	if s == nil || s.gss.head == nil || s.depth() == 0 {
		return nil, 0, false
	}
	if childCount == 0 {
		return buf[:0], s.top().state, true
	}

	rev := buf[:0]
	nonExtraFound := 0
	n := s.gss.head
	for n != nil {
		rev = append(rev, n.entry)
		if n.entry.node != nil && !n.entry.node.isExtra {
			nonExtraFound++
			if nonExtraFound == childCount {
				break
			}
		}
		n = n.prev
	}
	if nonExtraFound < childCount || n == nil || n.prev == nil {
		return rev[:0], 0, false
	}
	topState := n.prev.entry.state

	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev, topState, true
}

func (p *Parser) applyReduceActionFromGSS(s *glrStack, act ParseAction, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, tmp []stackEntry) {
	childCount := int(act.ChildCount)
	windowEntries, topState, ok := reduceWindowFromGSS(s, childCount, tmp)
	if !ok {
		s.dead = true
		if tmpEntries != nil {
			*tmpEntries = windowEntries[:0]
		}
		return
	}

	actualEnd := len(windowEntries)
	reducedEnd := actualEnd
	for i := actualEnd - 1; i >= 0; i-- {
		n := windowEntries[i].node
		if n == nil || !n.isExtra {
			break
		}
		reducedEnd--
	}

	span := computeReduceRawSpan(windowEntries, 0, reducedEnd)
	children, fieldIDs := p.buildReduceChildren(windowEntries, 0, reducedEnd, childCount, act.ProductionID, arena)

	var trailingExtras []*Node
	if actualEnd > reducedEnd {
		var trailingBuf [8]*Node
		if actualEnd-reducedEnd <= len(trailingBuf) {
			trailingExtras = trailingBuf[:0]
		} else {
			trailingExtras = make([]*Node, 0, actualEnd-reducedEnd)
		}
		for i := reducedEnd; i < actualEnd; i++ {
			extra := windowEntries[i].node
			if extra != nil {
				trailingExtras = append(trailingExtras, extra)
			}
		}
	}

	targetDepth := s.depth() - actualEnd
	if targetDepth < 0 || !s.truncate(targetDepth) {
		s.dead = true
		if tmpEntries != nil {
			*tmpEntries = windowEntries[:0]
		}
		return
	}

	named := p.isNamedSymbol(act.Symbol)
	parent := newParentNodeInArena(arena, act.Symbol, named, children, fieldIDs, act.ProductionID)
	shouldUseRawSpan := len(children) == 0
	if !shouldUseRawSpan && p.forceRawSpanAll {
		shouldUseRawSpan = true
	}
	if !shouldUseRawSpan && int(act.Symbol) < len(p.forceRawSpanTable) && p.forceRawSpanTable[act.Symbol] {
		shouldUseRawSpan = true
	}
	if shouldUseRawSpan && reducedEnd > 0 {
		parent.startByte = span.startByte
		parent.endByte = span.endByte
		parent.startPoint = span.startPoint
		parent.endPoint = span.endPoint
	}
	*nodeCount++

	gotoState := p.lookupGoto(topState, act.Symbol)
	targetState := topState
	if gotoState != 0 {
		targetState = gotoState
	}
	parent.preGotoState = topState
	parent.parseState = targetState
	p.pushStackNode(s, targetState, parent, entryScratch, gssScratch)
	for i := range trailingExtras {
		extra := trailingExtras[i]
		extra.parseState = targetState
		p.pushStackNode(s, targetState, extra, entryScratch, gssScratch)
	}

	s.score += int(act.DynamicPrecedence)
	*anyReduced = true
	if tmpEntries != nil {
		*tmpEntries = windowEntries[:0]
	}
}

type reduceRange struct {
	start      int
	reducedEnd int
	actualEnd  int
	topState   StateID
}

type reduceRawSpan struct {
	startByte  uint32
	endByte    uint32
	startPoint Point
	endPoint   Point
}

func computeReduceRange(entries []stackEntry, childCount int) (reduceRange, bool) {
	start := len(entries)
	nonExtraFound := 0
	for nonExtraFound < childCount && start > 1 {
		start--
		if entries[start].node != nil && !entries[start].node.isExtra {
			nonExtraFound++
		}
	}
	if nonExtraFound < childCount {
		return reduceRange{}, false
	}

	actualEnd := len(entries)
	reducedEnd := actualEnd
	for i := actualEnd - 1; i >= start; i-- {
		n := entries[i].node
		if n == nil || !n.isExtra {
			break
		}
		reducedEnd--
	}
	return reduceRange{
		start:      start,
		reducedEnd: reducedEnd,
		actualEnd:  actualEnd,
		topState:   entries[start-1].state,
	}, true
}

func computeReduceRawSpan(entries []stackEntry, start, end int) reduceRawSpan {
	span := reduceRawSpan{}
	if end <= start {
		return span
	}

	foundStart := false
	for i := start; i < end; i++ {
		n := entries[i].node
		if n != nil && !n.isExtra {
			span.startByte = n.startByte
			span.startPoint = n.startPoint
			foundStart = true
			break
		}
	}

	foundEnd := false
	for i := end - 1; i >= start; i-- {
		n := entries[i].node
		if n != nil && !n.isExtra {
			span.endByte = n.endByte
			span.endPoint = n.endPoint
			foundEnd = true
			break
		}
	}

	firstRaw := entries[start].node
	lastRaw := entries[end-1].node
	if !foundStart && firstRaw != nil {
		span.startByte = firstRaw.startByte
		span.startPoint = firstRaw.startPoint
	}
	if !foundEnd && lastRaw != nil {
		span.endByte = lastRaw.endByte
		span.endPoint = lastRaw.endPoint
	}
	return span
}

func (p *Parser) buildReduceChildren(entries []stackEntry, start, end, childCount int, productionID uint16, arena *nodeArena) ([]*Node, []FieldID) {
	lang := p.language
	symbolMeta := lang.SymbolMetadata

	var aliasSeq []Symbol
	if pid := int(productionID); pid >= 0 && pid < len(lang.AliasSequences) {
		aliasSeq = lang.AliasSequences[pid]
	}

	normalizedCount := 0
	structuralChildIndex := 0
	for i := start; i < end; i++ {
		n := entries[i].node
		if n == nil {
			continue
		}
		effectiveSymbol := n.symbol
		if !n.isExtra {
			if structuralChildIndex < len(aliasSeq) {
				if alias := aliasSeq[structuralChildIndex]; alias != 0 {
					effectiveSymbol = alias
				}
			}
			structuralChildIndex++
		}
		visible := true
		if idx := int(effectiveSymbol); idx < len(symbolMeta) {
			visible = symbolMeta[effectiveSymbol].Visible
		}
		if visible {
			normalizedCount++
		} else {
			normalizedCount += len(n.children)
		}
	}

	rawFieldIDs := p.buildFieldIDs(childCount, productionID, arena)
	children := arena.allocNodeSlice(normalizedCount)
	var fieldIDs []FieldID
	needFieldIDs := rawFieldIDs != nil
	if !needFieldIDs {
		for i := start; i < end; i++ {
			n := entries[i].node
			if n == nil {
				continue
			}
			if symbolVisible(lang, n.symbol) {
				continue
			}
			if len(n.fieldIDs) > 0 {
				needFieldIDs = true
				break
			}
		}
	}
	if needFieldIDs {
		fieldIDs = arena.allocFieldIDSlice(normalizedCount)
	}

	out := 0
	structuralChildIndex = 0
	for i := start; i < end; i++ {
		n := entries[i].node
		if n == nil {
			continue
		}
		var fid FieldID
		if !n.isExtra {
			if structuralChildIndex < len(rawFieldIDs) {
				fid = rawFieldIDs[structuralChildIndex]
			}
			if structuralChildIndex < len(aliasSeq) {
				if alias := aliasSeq[structuralChildIndex]; alias != 0 {
					n = aliasedNodeInArena(arena, lang, n, alias)
				}
			}
			structuralChildIndex++
		}

		visible := true
		if idx := int(n.symbol); idx < len(symbolMeta) {
			visible = symbolMeta[n.symbol].Visible
		}
		if visible {
			children[out] = n
			if fieldIDs != nil {
				fieldIDs[out] = fid
			}
			out++
			continue
		}

		kids := n.children
		if len(kids) == 0 {
			continue
		}
		copy(children[out:], kids)
		if fieldIDs != nil {
			// Preserve field labels already assigned on the inlined node's
			// children so flattened trees keep child-field lookups consistent.
			copyCount := len(kids)
			if len(n.fieldIDs) < copyCount {
				copyCount = len(n.fieldIDs)
			}
			if copyCount > 0 {
				copy(fieldIDs[out:out+copyCount], n.fieldIDs[:copyCount])
			}

			if fid != 0 {
				findTarget := func() int {
					if len(n.fieldIDs) == len(kids) {
						for i, kidFieldID := range n.fieldIDs {
							if kidFieldID == fid {
								return i
							}
						}
					}
					for i, kid := range kids {
						if kid != nil && kid.isNamed {
							return i
						}
					}
					return 0
				}
				fieldIDs[out+findTarget()] = fid
			}
		}
		out += len(kids)
	}
	if out != len(children) {
		children = children[:out]
		if fieldIDs != nil {
			fieldIDs = fieldIDs[:out]
		}
	}
	return children, fieldIDs
}

func (p *Parser) applyReduceAction(s *glrStack, act ParseAction, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, entries []stackEntry) {
	childCount := int(act.ChildCount)
	window, ok := computeReduceRange(entries, childCount)
	if !ok {
		// Not enough stack entries — kill this stack version.
		s.dead = true
		return
	}

	span := computeReduceRawSpan(entries, window.start, window.reducedEnd)
	children, fieldIDs := p.buildReduceChildren(entries, window.start, window.reducedEnd, childCount, act.ProductionID, arena)

	trailingStart := window.reducedEnd
	trailingEnd := window.actualEnd
	var trailingExtras []*Node
	if trailingEnd > trailingStart {
		var trailingBuf [8]*Node
		if trailingEnd-trailingStart <= len(trailingBuf) {
			trailingExtras = trailingBuf[:0]
		} else {
			trailingExtras = make([]*Node, 0, trailingEnd-trailingStart)
		}
		for i := trailingStart; i < trailingEnd; i++ {
			extra := entries[i].node
			if extra != nil {
				trailingExtras = append(trailingExtras, extra)
			}
		}
	}

	// Pop all reduced entries in one step after collection.
	if !s.truncate(window.start) {
		s.dead = true
		return
	}

	named := p.isNamedSymbol(act.Symbol)
	parent := newParentNodeInArena(arena, act.Symbol, named, children, fieldIDs, act.ProductionID)
	shouldUseRawSpan := len(children) == 0
	if !shouldUseRawSpan && p.forceRawSpanAll {
		shouldUseRawSpan = true
	}
	if !shouldUseRawSpan && int(act.Symbol) < len(p.forceRawSpanTable) && p.forceRawSpanTable[act.Symbol] {
		shouldUseRawSpan = true
	}
	if shouldUseRawSpan && window.reducedEnd > window.start {
		parent.startByte = span.startByte
		parent.endByte = span.endByte
		parent.startPoint = span.startPoint
		parent.endPoint = span.endPoint
	}
	*nodeCount++

	gotoState := p.lookupGoto(window.topState, act.Symbol)
	targetState := window.topState
	if gotoState != 0 {
		targetState = gotoState
	}
	parent.preGotoState = window.topState
	parent.parseState = targetState
	p.pushStackNode(s, targetState, parent, entryScratch, gssScratch)
	for i := range trailingExtras {
		extra := trailingExtras[i]
		extra.parseState = targetState
		p.pushStackNode(s, targetState, extra, entryScratch, gssScratch)
	}

	s.score += int(act.DynamicPrecedence)
	*anyReduced = true
}

func recoverAction(entry *ParseActionEntry) (ParseAction, bool) {
	if entry == nil {
		return ParseAction{}, false
	}
	for _, act := range entry.Actions {
		if act.Type == ParseActionRecover {
			return act, true
		}
	}
	return ParseAction{}, false
}

func (p *Parser) findRecoverActionOnStack(s *glrStack, sym Symbol, timing *incrementalParseTiming) (int, ParseAction, bool) {
	if s == nil {
		return 0, ParseAction{}, false
	}
	if s.recoverabilityKnown && !s.mayRecover {
		return 0, ParseAction{}, false
	}
	if timing != nil {
		timing.recoverSearches++
	}
	if !p.symbolCanRecover(sym) {
		if timing != nil {
			timing.recoverSymbolSkips++
		}
		return 0, ParseAction{}, false
	}

	if len(s.entries) > 0 {
		entries := s.entries
		for depth := len(entries) - 1; depth >= 0; depth-- {
			state := entries[depth].state
			if timing != nil {
				timing.recoverStateChecks++
			}
			if !p.stateCanRecover(state) {
				if timing != nil {
					timing.recoverStateSkips++
				}
				continue
			}
			if timing != nil {
				timing.recoverLookups++
			}
			if act, ok := p.recoverActionForState(state, sym); ok {
				if timing != nil {
					timing.recoverHits++
				}
				return depth, act, true
			}
		}
		return 0, ParseAction{}, false
	}

	if s.gss.head == nil {
		return 0, ParseAction{}, false
	}

	depth := s.gss.len() - 1
	for n := s.gss.head; n != nil; n = n.prev {
		state := n.entry.state
		if timing != nil {
			timing.recoverStateChecks++
		}
		if !p.stateCanRecover(state) {
			if timing != nil {
				timing.recoverStateSkips++
			}
			depth--
			continue
		}
		if timing != nil {
			timing.recoverLookups++
		}
		if act, ok := p.recoverActionForState(state, sym); ok {
			if timing != nil {
				timing.recoverHits++
			}
			return depth, act, true
		}
		depth--
	}
	return 0, ParseAction{}, false
}

func aliasedNodeInArena(arena *nodeArena, lang *Language, n *Node, alias Symbol) *Node {
	if n == nil || alias == 0 || n.symbol == alias {
		return n
	}

	if arena == nil {
		cloned := &Node{}
		*cloned = *n
		cloned.symbol = alias
		if lang != nil && int(alias) < len(lang.SymbolMetadata) {
			cloned.isNamed = lang.SymbolMetadata[alias].Named
		}
		return cloned
	}

	cloned := arena.allocNode()
	*cloned = *n
	cloned.symbol = alias
	if lang != nil && int(alias) < len(lang.SymbolMetadata) {
		cloned.isNamed = lang.SymbolMetadata[alias].Named
	}
	cloned.ownerArena = arena
	return cloned
}

func cloneNodeInArena(arena *nodeArena, n *Node) *Node {
	if n == nil {
		return nil
	}
	if arena == nil {
		cloned := &Node{}
		*cloned = *n
		return cloned
	}
	cloned := arena.allocNode()
	*cloned = *n
	cloned.ownerArena = arena
	return cloned
}

// buildFieldIDs creates the field ID slice for a reduce action.
func (p *Parser) buildFieldIDs(childCount int, productionID uint16, arena *nodeArena) []FieldID {
	if childCount <= 0 || len(p.language.FieldMapEntries) == 0 {
		return nil
	}

	pid := int(productionID)
	if pid >= len(p.language.FieldMapSlices) {
		return nil
	}

	fm := p.language.FieldMapSlices[pid]
	count := int(fm[1])
	if count == 0 {
		return nil
	}

	fieldIDs := arena.allocFieldIDSlice(childCount)
	start := int(fm[0])
	assigned := false
	for i := 0; i < count; i++ {
		entryIdx := start + i
		if entryIdx >= len(p.language.FieldMapEntries) {
			break
		}
		entry := p.language.FieldMapEntries[entryIdx]
		if int(entry.ChildIndex) < len(fieldIDs) {
			fieldIDs[entry.ChildIndex] = entry.FieldID
			assigned = true
		}
	}

	if !assigned {
		return nil
	}
	return fieldIDs
}

// buildResultFromGLR picks the best stack and constructs the final tree.
// Prefers accepted stacks, then highest score, then most entries.
func (p *Parser) buildResultFromGLR(stacks []glrStack, source []byte, arena *nodeArena, oldTree *Tree, reuseState *parseReuseState) *Tree {
	if len(stacks) == 0 {
		arena.Release()
		return parseErrorTree(source, p.language)
	}

	best := 0
	for i := 1; i < len(stacks); i++ {
		if stackCompare(stacks[i], stacks[best]) > 0 {
			best = i
		}
	}

	selected := stacks[best]
	if len(selected.entries) > 0 {
		return p.buildResult(selected.entries, source, arena, oldTree, reuseState)
	}
	if selected.gss.head == nil {
		return p.buildResult(nil, source, arena, oldTree, reuseState)
	}
	return p.buildResultFromNodes(nodesFromGSS(selected.gss), source, arena, oldTree, reuseState)
}

// lookupAction looks up the parse action for the given state and symbol.
func (p *Parser) lookupAction(state StateID, sym Symbol) *ParseActionEntry {
	idx := p.lookupActionIndex(state, sym)
	if idx == 0 {
		return nil
	}
	if int(idx) < len(p.language.ParseActions) {
		return &p.language.ParseActions[idx]
	}
	return nil
}

// lookupActionIndex returns the parse action index for (state, symbol).
// Returns 0 (the error/no-action entry) if not found.
func (p *Parser) lookupActionIndex(state StateID, sym Symbol) uint16 {
	if int(state) < p.denseLimit {
		return p.lookupActionIndexDense(state, sym)
	}
	return p.lookupActionIndexSmall(state, sym)
}

func (p *Parser) lookupActionIndexDense(state StateID, sym Symbol) uint16 {
	if int(state) >= len(p.language.ParseTable) {
		return 0
	}
	row := p.language.ParseTable[state]
	if int(sym) >= len(row) {
		return 0
	}
	return row[sym]
}

func (p *Parser) lookupActionIndexSmall(state StateID, sym Symbol) uint16 {
	// Small (compressed sparse) table lookup.
	smallIdx := int(state) - p.smallBase
	if smallIdx < 0 || smallIdx >= len(p.language.SmallParseTableMap) {
		return 0
	}
	if smallIdx < len(p.smallLookup) {
		pairs := p.smallLookup[smallIdx]
		if len(pairs) > 0 {
			target := uint16(sym)
			if len(pairs) <= 8 {
				for i := 0; i < len(pairs); i++ {
					if pairs[i].sym == target {
						return pairs[i].val
					}
					if pairs[i].sym > target {
						return 0
					}
				}
				return 0
			}
			lo, hi := 0, len(pairs)
			for lo < hi {
				mid := int(uint(lo+hi) >> 1)
				if pairs[mid].sym < target {
					lo = mid + 1
				} else {
					hi = mid
				}
			}
			if lo < len(pairs) && pairs[lo].sym == target {
				return pairs[lo].val
			}
			return 0
		}
	}
	offset := p.language.SmallParseTableMap[smallIdx]
	table := p.language.SmallParseTable
	if int(offset) >= len(table) {
		return 0
	}

	groupCount := table[offset]
	pos := int(offset) + 1
	for i := uint16(0); i < groupCount; i++ {
		if pos+1 >= len(table) {
			break
		}
		sectionValue := table[pos]
		symbolCount := table[pos+1]
		pos += 2
		for j := uint16(0); j < symbolCount; j++ {
			if pos >= len(table) {
				break
			}
			if table[pos] == uint16(sym) {
				return sectionValue
			}
			pos++
		}
	}
	return 0
}

// lookupGoto returns the GOTO target state for a nonterminal symbol.
func (p *Parser) lookupGoto(state StateID, sym Symbol) StateID {
	raw := p.lookupActionIndex(state, sym)
	if raw == 0 {
		return 0
	}

	// ts2go-generated grammars encode nonterminal GOTO values directly as
	// parser state IDs. Hand-built grammars encode parse-action indices.
	// ts2go always sets InitialState=1 (tree-sitter convention); hand-built
	// grammars default to InitialState=0.
	if p.language.TokenCount > 0 &&
		uint32(sym) >= p.language.TokenCount &&
		p.language.StateCount > 0 &&
		p.language.InitialState > 0 {
		return StateID(raw)
	}

	// Hand-built grammar or terminal symbol: look up in parse actions.
	if int(raw) < len(p.language.ParseActions) {
		entry := &p.language.ParseActions[raw]
		if len(entry.Actions) > 0 && entry.Actions[0].Type == ParseActionShift {
			return entry.Actions[0].State
		}
	}
	return 0
}

// isNamedSymbol checks whether a symbol is a named symbol.
func (p *Parser) isNamedSymbol(sym Symbol) bool {
	if int(sym) < len(p.language.SymbolMetadata) {
		return p.language.SymbolMetadata[sym].Named
	}
	return false
}

func nodesFromGSS(stack gssStack) []*Node {
	if stack.head == nil {
		return nil
	}
	count := 0
	for n := stack.head; n != nil; n = n.prev {
		if n.entry.node != nil {
			count++
		}
	}
	if count == 0 {
		return nil
	}
	nodes := make([]*Node, count)
	i := count - 1
	for n := stack.head; n != nil; n = n.prev {
		if n.entry.node != nil {
			nodes[i] = n.entry.node
			i--
		}
	}
	return nodes
}

// buildResult constructs the final Tree from a stack of entries.
func (p *Parser) buildResult(stack []stackEntry, source []byte, arena *nodeArena, oldTree *Tree, reuseState *parseReuseState) *Tree {
	var nodes []*Node
	for _, entry := range stack {
		if entry.node != nil {
			nodes = append(nodes, entry.node)
		}
	}
	return p.buildResultFromNodes(nodes, source, arena, oldTree, reuseState)
}

func (p *Parser) buildResultFromNodes(nodes []*Node, source []byte, arena *nodeArena, oldTree *Tree, reuseState *parseReuseState) *Tree {
	if len(nodes) == 0 {
		arena.Release()
		if isWhitespaceOnlySource(source) {
			return NewTree(nil, source, p.language)
		}
		return parseErrorTree(source, p.language)
	}

	if arena != nil && arena.used == 0 {
		arena.Release()
		arena = nil
	}

	expectedRootSymbol := Symbol(0)
	hasExpectedRoot := false
	if oldTree != nil && oldTree.RootNode() != nil {
		expectedRootSymbol = oldTree.RootNode().symbol
		hasExpectedRoot = true
	}
	borrowedResolved := false
	var borrowed []*nodeArena
	getBorrowed := func() []*nodeArena {
		if borrowedResolved {
			return borrowed
		}
		borrowed = reuseState.retainBorrowed(arena)
		borrowedResolved = true
		return borrowed
	}

	if len(nodes) == 1 {
		candidate := nodes[0]
		extendNodeToTrailingWhitespace(candidate, source)
		if !hasExpectedRoot || candidate.symbol == expectedRootSymbol {
			return newTreeWithArenas(candidate, source, p.language, arena, getBorrowed())
		}

		// Incremental reuse guard: if the only stacked node doesn't match the
		// previous root symbol, synthesize an expected root wrapper instead of
		// returning a reused child as the new tree root.
		rootChildren := make([]*Node, 1)
		rootChildren[0] = candidate
		if arena != nil {
			rootChildren = arena.allocNodeSlice(1)
			rootChildren[0] = candidate
		}
		root := newParentNodeInArena(arena, expectedRootSymbol, true, rootChildren, nil, 0)
		extendNodeToTrailingWhitespace(root, source)
		return newTreeWithArenas(root, source, p.language, arena, getBorrowed())
	}

	// When multiple nodes remain on the stack, check whether all but one
	// are extras (e.g. leading whitespace/comments). If so, fold the extras
	// into the real root rather than wrapping everything in an error node.
	var realRoot *Node
	var allExtras []*Node
	var extras []*Node
	for _, n := range nodes {
		if n.isExtra {
			allExtras = append(allExtras, n)
			// Ignore invisible extras in final-root recovery; they should not
			// force an error wrapper or inflate root child counts.
			if p != nil && p.language != nil && int(n.symbol) < len(p.language.SymbolMetadata) && p.language.SymbolMetadata[n.symbol].Visible {
				extras = append(extras, n)
			}
		} else {
			if realRoot != nil {
				realRoot = nil // more than one non-extra → genuine error
				break
			}
			realRoot = n
		}
	}
	if realRoot != nil {
		if reuseState != nil && reuseState.reusedAny {
			realRoot = cloneNodeInArena(arena, realRoot)
			realRoot.parent = nil
			realRoot.childIndex = -1
		}
		if len(extras) > 0 {
			// Fold visible extras into the real root as leading/trailing children.
			merged := make([]*Node, 0, len(extras)+len(realRoot.children))
			leadingCount := 0
			for _, e := range extras {
				if e.startByte <= realRoot.startByte {
					merged = append(merged, e)
					leadingCount++
				}
			}
			merged = append(merged, realRoot.children...)
			for _, e := range extras {
				if e.startByte > realRoot.startByte {
					merged = append(merged, e)
				}
			}
			if arena != nil {
				out := arena.allocNodeSlice(len(merged))
				copy(out, merged)
				merged = out
			}
			realRoot.children = merged
			// Keep fieldIDs aligned with children: extras have no field (0).
			if len(realRoot.fieldIDs) > 0 {
				trailingCount := len(extras) - leadingCount
				padded := make([]FieldID, leadingCount+len(realRoot.fieldIDs)+trailingCount)
				copy(padded[leadingCount:], realRoot.fieldIDs)
				realRoot.fieldIDs = padded
			}
			// Update parent pointers and child indexes for folded extras.
			for i, c := range realRoot.children {
				if c == nil {
					continue
				}
				c.parent = realRoot
				c.childIndex = i
			}
			// Extend root range to cover the extras.
			for _, e := range extras {
				if e.startByte < realRoot.startByte {
					realRoot.startByte = e.startByte
					realRoot.startPoint = e.startPoint
				}
				if e.endByte > realRoot.endByte {
					realRoot.endByte = e.endByte
					realRoot.endPoint = e.endPoint
				}
			}
		}
		// Invisible extras should still contribute to the root byte/point range.
		for _, e := range allExtras {
			if e.startByte < realRoot.startByte {
				realRoot.startByte = e.startByte
				realRoot.startPoint = e.startPoint
			}
			if e.endByte > realRoot.endByte {
				realRoot.endByte = e.endByte
				realRoot.endPoint = e.endPoint
			}
		}
		extendNodeToTrailingWhitespace(realRoot, source)
		if !hasExpectedRoot || realRoot.symbol == expectedRootSymbol {
			return newTreeWithArenas(realRoot, source, p.language, arena, getBorrowed())
		}
	}

	rootChildren := nodes
	rootSymbol := nodes[len(nodes)-1].symbol
	if hasExpectedRoot {
		rootSymbol = expectedRootSymbol
	}
	root := newParentNodeInArena(arena, rootSymbol, true, rootChildren, nil, 0)
	root.hasError = true
	extendNodeToTrailingWhitespace(root, source)
	return newTreeWithArenas(root, source, p.language, arena, getBorrowed())
}
