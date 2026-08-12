package gotreesitter

import "sync"

// This file builds, per Language and on demand, an approximation of "which
// named node types can ever be an immediate child of which other named node
// type", read directly from the language's compiled LR parse tables (the
// same ParseTable/SmallParseTable/ParseActions/LargeStateGotos data the
// parser itself executes). ValidateQueryPatterns (query_impossible_pattern.go)
// is the only caller; see its doc comment for what the resulting analysis
// does and does not prove.
//
// The short version: gotreesitter does not have tree-sitter's original
// grammar production right-hand sides available for languages decoded from a
// ts2go blob (Language.ProductionSignatures is a grammargen-only field), so
// this cannot reproduce C's ts_query__analyze_patterns exactly. Instead it
// answers a narrower, existence-only question ("can child ever occur
// somewhere under parent") by walking the compiled automaton's shift/goto
// graph backward from every state that can reduce to `parent`, which is
// sound in one direction: every signal this collects (raw automaton edges,
// AliasSequences overrides, and hidden-symbol flattening) can only add
// candidate children, never remove a genuine one. A missing PatternIssue is
// therefore not proof a pattern is meaningful, only that this analysis could
// not disprove it.

// maxLanguageChildAnalysisStates bounds how large a language's compiled state
// space this analysis will walk. Every fleet language observed while writing
// this stays well under this (the largest, C++, has roughly 11.7k states);
// the bound exists only to fail closed (skip the whole language, report
// nothing) against a pathological or adversarial custom Language rather than
// spend unbounded time/memory building the edge graph.
const maxLanguageChildAnalysisStates = 200_000

// maxHiddenChildRecursionDepth bounds how many hidden (invisible) wrapper
// symbols this analysis will transparently unwrap while resolving a parent's
// legal children. Real grammars observed while writing this nest generated
// wrappers (repeat/choice helpers, `_type`-style hidden alternatives) at most
// two or three deep; this leaves ample headroom while guaranteeing
// termination independent of the cycle guard in legalNamedChildrenLocked.
const maxHiddenChildRecursionDepth = 8

// languageChildAnalysisCache memoizes buildLanguageChildAnalysis per
// *Language, mirroring the snippetParserPools cache in parser.go (parser.go
// line ~467): languages are long-lived, process-wide singletons in every
// real caller, so keying by pointer is safe and avoids rebuilding the
// automaton edge graph on every ValidateQueryPatterns call.
var languageChildAnalysisCache sync.Map // map[*Language]*languageChildAnalysis

// childEdge is one shift/goto transition in the compiled parser automaton,
// recorded on the state it lands in (so languageChildAnalysis.predecessors
// answers "how could we have arrived here").
type childEdge struct {
	from StateID
	sym  Symbol
}

// stateReduce records that some state has a reduce action publicly producing
// the given named symbol; productionID looks up that production's
// AliasSequences row, and childCount bounds how many shift/goto hops
// backward from this state belong to this exact production (see
// walkBackwardBounded).
type stateReduce struct {
	target       Symbol
	productionID uint16
	childCount   uint8
}

// stateChildCountKey dedupes seed (state, childCount) pairs before walking:
// ParseActions stores one reduce entry per (state, lookahead terminal), so
// the same production is typically recorded dozens of times per state (once
// per terminal that can trigger it).
type stateChildCountKey struct {
	state      int
	childCount uint8
}

// childSetResult is the memoized outcome for one parent symbol: ok is false
// when the analysis has no confident answer (no reduce to that symbol was
// ever observed, a recursion bound was hit, or the language could not be
// analyzed at all) and must never be treated as "no legal children".
type childSetResult struct {
	children map[Symbol]bool
	ok       bool
}

// languageChildAnalysis is the per-Language, lazily-built index described at
// the top of this file.
type languageChildAnalysis struct {
	lang *Language

	// predecessors[state] lists every (fromState, symbol) shift/goto edge
	// whose target is `state`.
	predecessors [][]childEdge
	// reducesByState[state] lists every reduce action available in `state`,
	// canonicalized to its public named symbol.
	reducesByState [][]stateReduce

	ok bool // false if the tables could not be walked; always fail closed

	mu    sync.Mutex
	cache map[Symbol]childSetResult
}

// languageChildAnalysisFor returns the memoized analysis for lang, building
// it on first use. Returned value is never nil, but callers must check .ok.
func languageChildAnalysisFor(lang *Language) *languageChildAnalysis {
	if lang == nil {
		return &languageChildAnalysis{}
	}
	if cached, found := languageChildAnalysisCache.Load(lang); found {
		return cached.(*languageChildAnalysis)
	}
	built := buildLanguageChildAnalysis(lang)
	actual, _ := languageChildAnalysisCache.LoadOrStore(lang, built)
	return actual.(*languageChildAnalysis)
}

// buildLanguageChildAnalysis walks every reachable parser state once,
// recording shift/goto edges (inverted, keyed by target state) and reduce
// actions. It reuses (*Parser).lookupActionIndex and (*Parser).lookupGoto --
// the same single-symbol primitives the real parser calls during actual
// parsing -- rather than re-decoding the dense/small parse table encodings
// itself, including the ts2go-direct-state-id-vs-action-index goto encoding
// split (see (*Parser).lookupGoto in parser_tables.go) that a hand-rolled
// decoder here would otherwise have to rediscover and could get subtly
// wrong per language.
func buildLanguageChildAnalysis(lang *Language) *languageChildAnalysis {
	a := &languageChildAnalysis{lang: lang, cache: make(map[Symbol]childSetResult)}
	if lang == nil || lang.TokenCount == 0 || len(lang.ParseActions) == 0 {
		return a
	}
	if len(lang.ParseTable) == 0 && len(lang.SmallParseTableMap) == 0 {
		return a
	}

	p := NewParser(lang)
	if p == nil {
		return a
	}

	totalStates := int(lang.StateCount)
	if totalStates <= 0 {
		totalStates = len(lang.ParseTable)
		if alt := p.smallBase + len(lang.SmallParseTableMap); alt > totalStates {
			totalStates = alt
		}
	}
	if totalStates <= 0 || totalStates > maxLanguageChildAnalysisStates {
		return a
	}

	predecessors := make([][]childEdge, totalStates)
	reducesByState := make([][]stateReduce, totalStates)

	symbolCount := int(lang.SymbolCount)
	if symbolCount < len(lang.SymbolNames) {
		symbolCount = len(lang.SymbolNames)
	}

	// Enumerate every (state, symbol) cell directly through
	// (*Parser).lookupActionIndex/lookupGoto -- the same single-lookup
	// primitives the real parser calls during actual parsing -- rather than
	// (*Parser).forEachActionIndexInState's row enumeration. The latter can
	// legitimately skip a "small" (sparse-table) state's entire dense
	// terminal row: it returns as soon as it finds a non-empty
	// smallLookup overflow slice for that state, without also checking
	// smallTokenLookup, which is correct only when the two happen to be
	// mutually exclusive per state. They are not always: a state with many
	// terminal-triggered reduce actions (so smallTokenLookup builds a dense
	// row for it) that also has any nonterminal goto entry (so smallLookup
	// is non-empty too) silently loses its entire terminal row under that
	// enumeration. This was found empirically while writing this analysis:
	// it made every reduce action for Rust's call_expression invisible to a
	// forEachActionIndexInState-based walk. lookupActionIndex/lookupGoto
	// check both structures correctly for a single symbol, so paying for
	// state*symbolCount lookups here avoids depending on a fix to that
	// shared enumeration helper, which is out of scope for this analysis.
	for s := 0; s < totalStates; s++ {
		state := StateID(s)
		for sym := Symbol(0); uint32(sym) < lang.TokenCount; sym++ {
			idx := p.lookupActionIndex(state, sym)
			if idx == 0 {
				continue
			}
			recordTerminalActions(lang, predecessors, reducesByState, state, sym, idx, totalStates)
		}
		for sym := Symbol(lang.TokenCount); int(sym) < symbolCount; sym++ {
			target := p.lookupGoto(state, sym)
			if target != 0 && int(target) < totalStates {
				predecessors[target] = append(predecessors[target], childEdge{from: state, sym: sym})
			}
		}
	}

	// LargeStateGotos can hold (state, symbol) entries that never appear as a
	// literal row entry in ParseTable/SmallParseTable -- it is overflow
	// storage for wide grammars (see Language.LargeStateGotos) -- so walk it
	// separately to avoid silently missing those edges.
	for key, target := range lang.LargeStateGotos {
		state := StateID(key >> 32)
		sym := Symbol(uint32(key))
		if target == 0 || int(state) >= totalStates || int(target) >= totalStates {
			continue
		}
		predecessors[target] = append(predecessors[target], childEdge{from: state, sym: sym})
	}

	a.predecessors = predecessors
	a.reducesByState = reducesByState
	a.ok = true
	return a
}

func recordTerminalActions(lang *Language, predecessors [][]childEdge, reducesByState [][]stateReduce, state StateID, sym Symbol, idx uint16, totalStates int) {
	if int(idx) >= len(lang.ParseActions) {
		return
	}
	for _, act := range lang.ParseActions[idx].Actions {
		switch act.Type {
		case ParseActionShift:
			if int(act.State) < totalStates {
				predecessors[act.State] = append(predecessors[act.State], childEdge{from: state, sym: sym})
			}
		case ParseActionReduce:
			pub := lang.PublicSymbolForNamedness(act.Symbol, true)
			reducesByState[state] = append(reducesByState[state], stateReduce{target: pub, productionID: act.ProductionID, childCount: act.ChildCount})
		}
	}
}

// legalNamedChildren returns the set of named symbols this analysis can
// confirm are reachable as an immediate child of `parent` somewhere in the
// grammar, and whether the analysis reached a confident answer at all. Only
// call with a symbol already canonicalized via
// Language.PublicSymbolForNamedness(sym, true).
func (a *languageChildAnalysis) legalNamedChildren(parent Symbol) (map[Symbol]bool, bool) {
	if a == nil || !a.ok {
		return nil, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.legalNamedChildrenLocked(parent, 0, make(map[Symbol]bool))
}

// legalNamedChildrenLocked does the real work. It must be called with a.mu
// held; visiting tracks symbols on the current recursion path (not every
// symbol ever seen) so a self-referential hidden wrapper (e.g. a generated
// repeat helper) terminates instead of looping. Cycle- or
// depth-bound bailouts deliberately do not write to a.cache: that outcome is
// an artifact of this particular call stack, not a fact about the symbol, so
// a later, differently-nested lookup for the same symbol must get a fresh
// chance to resolve it.
func (a *languageChildAnalysis) legalNamedChildrenLocked(parent Symbol, depth int, visiting map[Symbol]bool) (map[Symbol]bool, bool) {
	if cached, found := a.cache[parent]; found {
		return cached.children, cached.ok
	}
	if depth > maxHiddenChildRecursionDepth || visiting[parent] {
		return nil, false
	}
	visiting[parent] = true
	defer delete(visiting, parent)

	var seeds []stateReduce
	var seedStates []int
	for s, reduces := range a.reducesByState {
		for _, r := range reduces {
			if r.target == parent {
				seeds = append(seeds, r)
				seedStates = append(seedStates, s)
			}
		}
	}
	if len(seeds) == 0 {
		a.cache[parent] = childSetResult{ok: false}
		return nil, false
	}

	children := make(map[Symbol]bool)

	// Alias signal: a production's AliasSequences row forces a specific
	// display symbol for one of its children regardless of that child's
	// natural reduced symbol, so any nonzero entry is a confirmed legal
	// child of `parent` independent of the edge walk below.
	for _, r := range seeds {
		if int(r.productionID) >= len(a.lang.AliasSequences) {
			continue
		}
		for _, alias := range a.lang.AliasSequences[r.productionID] {
			if alias != 0 {
				children[a.lang.PublicSymbolForNamedness(alias, true)] = true
			}
		}
	}

	// Backward reachability signal: for each distinct (seed state,
	// production) pair, walk shift/goto predecessor edges backward EXACTLY
	// as many hops as that production's own child count, then stop.
	//
	// Bounding by child count is essential, not an optimization. An
	// unbounded walk keeps following predecessor edges past `parent`'s own
	// production into whatever else happens to reach the same states, and
	// LALR table compaction merges states whenever their remaining behavior
	// is identical -- which erases exactly the "which production is this
	// edge part of" context this analysis needs. Left unbounded, the
	// reported legal-children set balloons to include most of the grammar
	// within a handful of hops for any language with meaningfully shared
	// structure (confirmed empirically against this repository's own
	// fleet grammars while writing this: unbounded walks starting from
	// JavaScript's `method_definition` reached "identifier", "typeof",
	// "new", and dozens of unrelated statement/expression types by hop 5).
	// See the package doc comment for why bounding by production length
	// still cannot recover full per-position precision the way C's
	// stack-tracked forward analysis does.
	seenSeed := make(map[stateChildCountKey]bool, len(seeds))
	var hiddenChildren []Symbol
	seenHidden := make(map[Symbol]bool)
	for i, r := range seeds {
		key := stateChildCountKey{state: seedStates[i], childCount: r.childCount}
		if seenSeed[key] {
			continue
		}
		seenSeed[key] = true
		walkBackwardBounded(a, seedStates[i], int(r.childCount), r.productionID, children, &hiddenChildren, seenHidden)
	}

	// Hidden wrapper symbols are invisible in the real tree -- their own
	// children are spliced directly into the parent -- so a pattern
	// targeting the flattened descendant must still resolve as legal.
	for _, h := range hiddenChildren {
		if grandChildren, ok := a.legalNamedChildrenLocked(h, depth+1, visiting); ok {
			for c := range grandChildren {
				children[c] = true
			}
		}
	}

	a.cache[parent] = childSetResult{children: children, ok: true}
	return children, true
}

// walkBackwardBounded explores predecessor edges backward from seedState for
// exactly maxHops layers (maxHops is the triggering production's own child
// count: how many stack entries its reduce action pops), classifying each
// layer's edge labels into children (visible, named -- a confirmed
// candidate) or hiddenChildren (invisible -- the caller expands these
// transitively). Each layer is its own BFS frontier over ALL states reached
// at that exact hop distance from seedState; a state reachable at multiple
// hop distances is processed once per distance it appears at, matching what
// "N hops back from this specific reduce" means positionally.
//
// productionID identifies exactly which production seedState's reduce
// belongs to, which -- combined with the loop's hop count -- gives the
// child index each layer corresponds to (hop h, 0-indexed, is child index
// maxHops-1-h). tree-sitter grammars commonly write a child position as
// alias($.raw_rule, $.display_type) (for example
// tree-sitter-javascript aliases a bare identifier to property_identifier
// wherever an object/class member name can also just be a plain
// identifier); AliasSequences[productionID][childIndex] records that
// override, and this must classify the ALIASED display symbol at that
// position, not the raw edge label -- using the raw label there would
// silently readmit the pre-alias symbol as if it were separately legal,
// which is exactly what made this analysis unable to prove
// "identifier can never be a property name" the way tree-sitter's C
// query compiler can, before this override lookup was added.
func walkBackwardBounded(a *languageChildAnalysis, seedState, maxHops int, productionID uint16, children map[Symbol]bool, hiddenChildren *[]Symbol, seenHidden map[Symbol]bool) {
	if maxHops <= 0 {
		return
	}
	var aliasRow []Symbol
	if int(productionID) < len(a.lang.AliasSequences) {
		aliasRow = a.lang.AliasSequences[productionID]
	}
	frontier := map[int]bool{seedState: true}
	for hop := 0; hop < maxHops; hop++ {
		childIndex := maxHops - 1 - hop
		var aliasSym Symbol
		if childIndex >= 0 && childIndex < len(aliasRow) {
			aliasSym = aliasRow[childIndex]
		}
		next := make(map[int]bool)
		for s := range frontier {
			if s < 0 || s >= len(a.predecessors) {
				continue
			}
			for _, edge := range a.predecessors[s] {
				effective := edge.sym
				if aliasSym != 0 {
					effective = aliasSym
				}
				meta, known := symbolMetaFor(a.lang, effective)
				switch {
				case !known:
					// Cannot classify; the predecessor state is still worth
					// walking further back for earlier positions.
				case meta.Visible && meta.Named:
					children[a.lang.PublicSymbolForNamedness(effective, true)] = true
				case !meta.Visible:
					if !seenHidden[effective] {
						seenHidden[effective] = true
						*hiddenChildren = append(*hiddenChildren, effective)
					}
				}
				next[int(edge.from)] = true
			}
		}
		if len(next) == 0 {
			return
		}
		frontier = next
	}
}

func symbolMetaFor(lang *Language, sym Symbol) (SymbolMetadata, bool) {
	if lang == nil || int(sym) < 0 || int(sym) >= len(lang.SymbolMetadata) {
		return SymbolMetadata{}, false
	}
	return lang.SymbolMetadata[sym], true
}
