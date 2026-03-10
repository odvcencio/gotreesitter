package grammargen

import (
	"fmt"
	"os"
	"strings"
)

// coreEntry is a core item (prodIdx, dot) with a bitset of lookahead terminals.
// This avoids expanding N lookaheads into N individual lrItems during closure.
type coreEntry struct {
	prodIdx    int
	dot        int
	lookaheads bitset
}

// lrItemSet is a set of LR(1) items stored in core-based representation.
type lrItemSet struct {
	// cores is the core-based representation: one entry per (prodIdx, dot).
	cores []coreEntry
	// coreIndex maps (prodIdx, dot) → index in cores for fast lookup.
	coreIndex map[coreItem]int
	// packedCoreIndex is the same lookup keyed by packed (prodIdx,dot).
	// LALR LR(0) construction uses this directly so it can retain the dedup map
	// from closure building instead of allocating a second coreIndex map.
	packedCoreIndex map[uint64]int
	// coreHash is a hash of the core items only (without lookaheads).
	coreHash uint64
	// fullHash is a hash of core items + all lookaheads.
	fullHash uint64
	// reduceLAHash is a hash of only the reduce-item lookaheads (for extended merging).
	reduceLAHash uint64
	// boundaryLAHash is a hash of only the EOF/external-token lookaheads across
	// all items. This helps preserve boundary-sensitive contexts in very large
	// external-scanner grammars.
	boundaryLAHash uint64
	// annotationArgTag preserves predecessor-sensitive context only inside the
	// local annotation-argument expression subgraph for large Scala-like grammars.
	annotationArgTag uint32
}

func (set *lrItemSet) coreLookup(prodIdx, dot int) (int, bool) {
	if set.packedCoreIndex != nil {
		idx, ok := set.packedCoreIndex[packCoreItemKey(prodIdx, dot)]
		return idx, ok
	}
	idx, ok := set.coreIndex[coreItem{prodIdx: prodIdx, dot: dot}]
	return idx, ok
}

func (set *lrItemSet) setCoreIndex(prodIdx, dot, idx int) {
	if set.packedCoreIndex != nil {
		set.packedCoreIndex[packCoreItemKey(prodIdx, dot)] = idx
		return
	}
	set.coreIndex[coreItem{prodIdx: prodIdx, dot: dot}] = idx
}

func (set *lrItemSet) ensurePackedCoreIndex() {
	if set.packedCoreIndex != nil {
		return
	}
	packedCoreIndex := make(map[uint64]int, len(set.cores))
	for idx, ce := range set.cores {
		packedCoreIndex[packCoreItemKey(ce.prodIdx, ce.dot)] = idx
	}
	set.packedCoreIndex = packedCoreIndex
	set.coreIndex = nil
}

// lrAction is a parse table action.
type lrAction struct {
	kind    lrActionKind
	state   int   // shift target / goto target
	prodIdx int   // reduce production index
	prec    int   // for shift: precedence of the item's production
	assoc   Assoc // for shift: associativity of the item's production
	lhsSym  int   // LHS nonterminal of the production (for conflict detection)
	lhsSyms []int // additional LHS symbols (when shifts from multiple rules merge)
	isExtra bool  // true if this action comes from a nonterminal extra production
}

type lrActionKind int

const (
	lrShift lrActionKind = iota
	lrReduce
	lrAccept
)

// LRTables holds the generated parse tables.
type LRTables struct {
	// ActionTable[state][symbol] = list of actions (multiple = conflict/GLR)
	ActionTable map[int]map[int][]lrAction
	GotoTable   map[int]map[int]int // [state][nonterminal] → target state
	StateCount  int
}

// buildLRTables constructs LR(1) parse tables from a normalized grammar.
func buildLRTables(ng *NormalizedGrammar) (*LRTables, error) {
	tables, _, err := buildLRTablesInternal(ng, false)
	return tables, err
}

// buildLRTablesWithProvenance constructs LR(1) parse tables and returns
// the merge provenance alongside the tables for diagnostic use.
func buildLRTablesWithProvenance(ng *NormalizedGrammar) (*LRTables, *lrContext, error) {
	return buildLRTablesInternal(ng, true)
}

func buildLRTablesInternal(ng *NormalizedGrammar, trackProvenance bool) (*LRTables, *lrContext, error) {
	ctx := &lrContext{
		ng:              ng,
		firstSets:       make([]bitset, len(ng.Symbols)),
		nullables:       make([]bool, len(ng.Symbols)),
		prodsByLHS:      make(map[int][]int),
		betaCache:       make(map[uint32]*betaResult),
		trackProvenance: trackProvenance,
	}

	tokenCount := ng.TokenCount()
	ctx.tokenCount = tokenCount
	ctx.lookaheadWordCount = (tokenCount + 63) / 64
	if ctx.lookaheadWordCount == 0 {
		ctx.lookaheadWordCount = 1
	}
	ctx.maxLookaheadPool = len(ng.Productions)
	if ctx.maxLookaheadPool < 64 {
		ctx.maxLookaheadPool = 64
	}
	ctx.boundaryLookaheads = newBitset(tokenCount)
	ctx.boundaryLookaheads.add(0) // EOF
	for _, sym := range ng.ExternalSymbols {
		if sym >= 0 && sym < tokenCount {
			ctx.boundaryLookaheads.add(sym)
		}
	}
	// Preserve large-grammar declaration boundaries that otherwise disappear
	// under early core merging. This is especially important for annotation-
	// heavy Scala states where the same LR(0) core can legitimately continue
	// into different declaration forms (def/val/var/enum/class/trait/...).
	definitionBoundary := map[string]bool{
		"@":         true,
		"class":     true,
		"trait":     true,
		"object":    true,
		"enum":      true,
		"given":     true,
		"def":       true,
		"val":       true,
		"var":       true,
		"type":      true,
		"extension": true,
		"case":      true,
		"opaque":    true,
		"import":    true,
		"package":   true,
	}
	for sym := 0; sym < tokenCount; sym++ {
		if definitionBoundary[ng.Symbols[sym].Name] {
			ctx.boundaryLookaheads.add(sym)
		}
	}
	ctx.annotationAtSym = -1
	ctx.annotationDefSym = -1
	ctx.annotationOpenParenSym = -1
	ctx.annotationCloseParenSym = -1
	ctx.annotationArgCarrierLHS = make([]bool, len(ng.Symbols))
	annotationArgCarrierNames := map[string]bool{
		"arguments":                true,
		"_exprs_in_parens":         true,
		"expression":               true,
		"assignment_expression":    true,
		"lambda_expression":        true,
		"postfix_expression":       true,
		"ascription_expression":    true,
		"infix_expression":         true,
		"prefix_expression":        true,
		"return_expression":        true,
		"throw_expression":         true,
		"while_expression":         true,
		"do_while_expression":      true,
		"for_expression":           true,
		"macro_body":               true,
		"_simple_expression":       true,
		"identifier":               true,
		"_non_null_literal":        true,
		"string":                   true,
		"unit":                     true,
		"tuple_expression":         true,
		"parenthesized_expression": true,
		"field_expression":         true,
		"generic_function":         true,
		"call_expression":          true,
		"bindings":                 true,
		"type_parameters":          true,
	}
	for i, sym := range ng.Symbols {
		switch sym.Name {
		case "@":
			ctx.annotationAtSym = i
		case "def":
			ctx.annotationDefSym = i
		case "(":
			ctx.annotationOpenParenSym = i
		case ")":
			ctx.annotationCloseParenSym = i
		}
		if annotationArgCarrierNames[sym.Name] {
			ctx.annotationArgCarrierLHS[i] = true
		}
	}
	// Build production-by-LHS index for fast closure lookups.
	for i := range ng.Productions {
		lhs := ng.Productions[i].LHS
		ctx.prodsByLHS[lhs] = append(ctx.prodsByLHS[lhs], i)
	}

	// Identify nonterminal extra productions and all terminals for injection.
	for i := range ng.Productions {
		if ng.Productions[i].IsExtra {
			ctx.extraProdIndices = append(ctx.extraProdIndices, i)
		}
	}
	if len(ctx.extraProdIndices) > 0 {
		ctx.allTerminals = newBitset(tokenCount)
		for i := 0; i < tokenCount; i++ {
			ctx.allTerminals.add(i)
		}
	}

	// Pre-allocate dot-0 index for fast closure lookups.
	ctx.dot0Index = make([]int, len(ng.Productions))
	for i := range ctx.dot0Index {
		ctx.dot0Index[i] = -1
	}

	// Compute FIRST and nullable sets.
	ctx.computeFirstSets()

	// Build item sets. Use DeRemer/Pennello LALR for large grammars (>400 productions)
	// which would otherwise be slow with the iterative LR(1) construction.
	// Extended merging produces more precise states for mid-size grammars (100-400
	// productions) and is kept for those since some grammars (e.g. HCL) regress
	// significantly with LALR merging.
	var itemSets []lrItemSet
	// Very large grammars with external scanners can lose critical boundary-token
	// distinctions under pure LALR merging. Route those through the more precise
	// core-based builder with boundary-sensitive merging instead.
	useBoundaryLargeLR := len(ng.Productions) > 2000 && len(ng.ExternalSymbols) > 0
	if len(ng.Productions) > 400 && !useBoundaryLargeLR {
		itemSets = ctx.buildItemSetsLALR()
	} else {
		itemSets = ctx.buildItemSets()
	}

	// Build action and goto tables.
	tables := &LRTables{
		ActionTable: make(map[int]map[int][]lrAction),
		GotoTable:   make(map[int]map[int]int),
		StateCount:  len(itemSets),
	}

	for stateIdx, itemSet := range itemSets {
		tables.ActionTable[stateIdx] = make(map[int][]lrAction)
		tables.GotoTable[stateIdx] = make(map[int]int)

		// Use pre-computed transitions instead of recomputing gotoState.
		trans := ctx.transitions[stateIdx]

		for _, ce := range itemSet.cores {
			prod := &ng.Productions[ce.prodIdx]

			if ce.dot < len(prod.RHS) {
				// Dot not at end → shift or goto
				nextSym := prod.RHS[ce.dot]
				targetState, ok := trans[nextSym]
				if !ok {
					continue
				}

				if nextSym < tokenCount {
					// Terminal → shift action
					tables.addAction(stateIdx, nextSym, lrAction{
						kind:    lrShift,
						state:   targetState,
						prec:    prod.Prec,
						assoc:   prod.Assoc,
						lhsSym:  prod.LHS,
						isExtra: prod.IsExtra,
					})
				} else {
					// Nonterminal → goto
					tables.GotoTable[stateIdx][nextSym] = targetState
				}
			} else {
				// Dot at end → reduce or accept
				if ce.prodIdx == ng.AugmentProdID {
					// Augmented start production → accept
					tables.addAction(stateIdx, 0, lrAction{kind: lrAccept})
				} else {
					// Regular reduce — one action per lookahead terminal.
					ce.lookaheads.forEach(func(la int) {
						tables.addAction(stateIdx, la, lrAction{
							kind:    lrReduce,
							prodIdx: ce.prodIdx,
							lhsSym:  prod.LHS,
							isExtra: prod.IsExtra,
						})
					})
				}
			}
		}
	}

	return tables, ctx, nil
}

func (t *LRTables) addAction(state, sym int, action lrAction) {
	existing := t.ActionTable[state][sym]
	// Avoid duplicates.
	for i, a := range existing {
		if a.kind == action.kind && a.state == action.state {
			if a.kind == lrShift {
				// For shifts to the same target, keep the higher prec.
				if !a.isExtra && action.isExtra {
					return // existing non-extra wins
				}
				if a.isExtra && !action.isExtra {
					existing[i].isExtra = false
				}
				if action.prec > a.prec {
					existing[i].prec = action.prec
					existing[i].assoc = action.assoc
				}
				// Accumulate all contributing LHS symbols for conflict detection.
				if action.lhsSym != a.lhsSym && action.lhsSym != 0 {
					found := false
					for _, s := range existing[i].lhsSyms {
						if s == action.lhsSym {
							found = true
							break
						}
					}
					if !found {
						existing[i].lhsSyms = append(existing[i].lhsSyms, action.lhsSym)
					}
				}
				return
			}
			if a.prodIdx == action.prodIdx {
				return
			}
		}
	}
	t.ActionTable[state][sym] = append(existing, action)
}

// lrContext holds state during LR table construction.
type lrContext struct {
	ng         *NormalizedGrammar
	tokenCount int
	firstSets  []bitset // symbol → bitset of terminal first symbols
	nullables  []bool   // symbol → can derive ε

	// Production index: LHS symbol → production indices
	prodsByLHS map[int][]int

	// FIRST(β) cache: packed (prodIdx, dot) → first set + nullable flag
	betaCache map[uint32]*betaResult

	// Item set management
	itemSets    []lrItemSet
	transitions map[int]map[int]int

	// Merge provenance tracking (diagnostic metadata, does not affect construction)
	provenance                 *mergeProvenance
	trackProvenance            bool
	trackLookaheadContributors bool

	// Fast dot-0 lookup: prodIdx → cores slice index (-1 = absent).
	// Allocated once, reused across closureToSet calls.
	dot0Index []int
	dot0Dirty []int // indices to reset between calls

	// Nonterminal extra support
	extraProdIndices []int
	allTerminals     bitset // all terminal symbol IDs

	// Boundary lookaheads are EOF plus external tokens. They are used to keep
	// large external-scanner grammars from losing critical boundary distinctions
	// under aggressive state merging.
	boundaryLookaheads bitset
	// needReduceLAHash is true only when buildItemSets is using extended merging.
	// Boundary-only and pure-core paths do not read reduceLAHash.
	needReduceLAHash bool
	// Narrow annotation-argument tagging metadata. These are precomputed once so
	// buildItemSets can cheaply preserve declaration-family context only while a
	// state remains inside annotation arguments.
	annotationAtSym         int
	annotationDefSym        int
	annotationOpenParenSym  int
	annotationCloseParenSym int
	annotationArgCarrierLHS []bool

	// Reusable closure queue scratch keeps closureToSet/closureIncremental from
	// reallocating worklists and in-queue tracking on every item-set build.
	closureWorklist  []int
	closureQueuedGen []uint32
	closureQueueGen  uint32

	// GOTO scratch reuses transient symbol and advanced-kernel slices while
	// building successor states.
	gotoSymbolsScratch  []int
	gotoAdvancedScratch []coreEntry

	// Lookahead bitset scratch reuses word buffers for temporary closed sets that
	// are discarded after exact-match or merge lookups.
	lookaheadWordCount int
	lookaheadWordPool  [][]uint64
	maxLookaheadPool   int
}

// conflictResolutionCache stores grammar-wide declared-conflict metadata that
// would otherwise be rebuilt for every single resolveActionConflict call.
type conflictResolutionCache struct {
	symbolInConflict []bool
	reverseIdx       [][]int
	groups           [][]int
	groupsBySymbol   [][]int
}

func getConflictResolutionCache(ng *NormalizedGrammar) *conflictResolutionCache {
	if len(ng.Conflicts) == 0 {
		return nil
	}
	if ng.conflictCache != nil {
		return ng.conflictCache
	}

	cache := &conflictResolutionCache{
		symbolInConflict: make([]bool, len(ng.Symbols)),
		reverseIdx:       make([][]int, len(ng.Symbols)),
		groups:           make([][]int, len(ng.Conflicts)),
		groupsBySymbol:   make([][]int, len(ng.Symbols)),
	}

	for groupIdx, group := range ng.Conflicts {
		cache.groups[groupIdx] = append([]int(nil), group...)
		for _, sym := range group {
			if sym < 0 || sym >= len(cache.symbolInConflict) {
				continue
			}
			cache.symbolInConflict[sym] = true
			cache.groupsBySymbol[sym] = append(cache.groupsBySymbol[sym], groupIdx)
		}
	}

	for prodIdx, prod := range ng.Productions {
		for _, sym := range prod.RHS {
			if sym < 0 || sym >= len(cache.reverseIdx) {
				continue
			}
			cache.reverseIdx[sym] = append(cache.reverseIdx[sym], prodIdx)
		}
	}

	ng.conflictCache = cache
	return cache
}

func (ctx *lrContext) nextClosureQueueGen() uint32 {
	ctx.closureQueueGen++
	if ctx.closureQueueGen == 0 {
		for i := range ctx.closureQueuedGen {
			ctx.closureQueuedGen[i] = 0
		}
		ctx.closureQueueGen = 1
	}
	return ctx.closureQueueGen
}

func (ctx *lrContext) ensureClosureQueueCapacity(size int) {
	if size <= len(ctx.closureQueuedGen) {
		return
	}
	ctx.closureQueuedGen = append(ctx.closureQueuedGen, make([]uint32, size-len(ctx.closureQueuedGen))...)
}

func (ctx *lrContext) ensureProvenance() {
	if !ctx.trackProvenance || ctx.provenance != nil {
		return
	}
	ctx.provenance = newMergeProvenance()
}

func (ctx *lrContext) recordFreshState(stateIdx int) {
	if ctx.provenance != nil {
		ctx.provenance.recordFresh(stateIdx)
	}
}

func (ctx *lrContext) recordMergedState(stateIdx int, origin mergeOrigin) {
	if ctx.provenance != nil {
		ctx.provenance.recordMerge(stateIdx, origin)
	}
}

func (ctx *lrContext) recordLookaheadContributor(stateIdx, lookahead, ntTransIdx int) {
	if ctx.provenance != nil && ctx.trackLookaheadContributors {
		ctx.provenance.recordLookaheadContributor(stateIdx, lookahead, ntTransIdx)
	}
}

// releaseScratch drops temporary LR-construction data once table building and
// split diagnostics are complete. This avoids carrying the full build context
// into later lex/assemble/encode phases in GenerateWithReport.
func (ctx *lrContext) releaseScratch() {
	if ctx == nil {
		return
	}
	ctx.firstSets = nil
	ctx.nullables = nil
	ctx.prodsByLHS = nil
	ctx.betaCache = nil
	ctx.itemSets = nil
	ctx.transitions = nil
	ctx.provenance = nil
	ctx.dot0Index = nil
	ctx.dot0Dirty = nil
	ctx.extraProdIndices = nil
	ctx.allTerminals = bitset{}
	ctx.boundaryLookaheads = bitset{}
	ctx.gotoSymbolsScratch = nil
	ctx.gotoAdvancedScratch = nil
	ctx.lookaheadWordPool = nil
}

func (ctx *lrContext) allocLookaheadBitset() bitset {
	if n := len(ctx.lookaheadWordPool); n > 0 {
		words := ctx.lookaheadWordPool[n-1]
		ctx.lookaheadWordPool = ctx.lookaheadWordPool[:n-1]
		clear(words)
		return bitset{words: words}
	}
	return bitset{words: make([]uint64, ctx.lookaheadWordCount)}
}

func (ctx *lrContext) cloneLookaheadBitset(src *bitset) bitset {
	clone := ctx.allocLookaheadBitset()
	copy(clone.words, src.words)
	return clone
}

func (ctx *lrContext) recycleLookaheadBitset(b *bitset) {
	if len(b.words) != ctx.lookaheadWordCount || len(ctx.lookaheadWordPool) >= ctx.maxLookaheadPool {
		b.words = nil
		return
	}
	ctx.lookaheadWordPool = append(ctx.lookaheadWordPool, b.words)
	b.words = nil
}

func (ctx *lrContext) recycleItemSetLookaheads(set *lrItemSet) {
	for i := range set.cores {
		ctx.recycleLookaheadBitset(&set.cores[i].lookaheads)
	}
	set.cores = nil
	set.coreIndex = nil
	set.packedCoreIndex = nil
}

type extraChainBuilder struct {
	tables          *LRTables
	ng              *NormalizedGrammar
	ctx             *lrContext
	tokenCount      int
	syntheticStart  int
	terminalExtras  []int
	chainStateCache map[string]int
	entryStateCache map[string]int
	entrySeen       map[string]bool
	unionStateCache map[string]int
}

func newExtraChainBuilder(tables *LRTables, ng *NormalizedGrammar, ctx *lrContext, terminalExtras []int) *extraChainBuilder {
	return &extraChainBuilder{
		tables:          tables,
		ng:              ng,
		ctx:             ctx,
		tokenCount:      ng.TokenCount(),
		syntheticStart:  tables.StateCount,
		terminalExtras:  terminalExtras,
		chainStateCache: make(map[string]int),
		entryStateCache: make(map[string]int),
		entrySeen:       make(map[string]bool),
		unionStateCache: make(map[string]int),
	}
}

func (b *extraChainBuilder) newState() int {
	stateIdx := b.tables.StateCount
	b.tables.StateCount++
	b.tables.ActionTable[stateIdx] = make(map[int][]lrAction)
	b.tables.GotoTable[stateIdx] = make(map[int]int)
	return stateIdx
}

func (b *extraChainBuilder) finalizeState(stateIdx int) {
	for _, extraSym := range b.terminalExtras {
		if _, ok := b.tables.ActionTable[stateIdx][extraSym]; ok {
			continue
		}
		b.tables.ActionTable[stateIdx][extraSym] = []lrAction{{
			kind:    lrShift,
			state:   stateIdx,
			isExtra: true,
		}}
	}
}

func extraChainStateKey(a, b int, lookaheads *bitset) string {
	var sb strings.Builder
	sb.Grow(32 + len(lookaheads.words)*17)
	fmt.Fprintf(&sb, "%d:%d", a, b)
	for _, w := range lookaheads.words {
		fmt.Fprintf(&sb, ":%x", w)
	}
	return sb.String()
}

func (b *extraChainBuilder) buildProdChain(prodIdx, pos int, follow bitset) int {
	key := extraChainStateKey(prodIdx, pos, &follow)
	if stateIdx, ok := b.chainStateCache[key]; ok {
		return stateIdx
	}

	stateIdx := b.newState()
	b.chainStateCache[key] = stateIdx
	b.addProdContinuation(stateIdx, prodIdx, pos, follow)
	b.finalizeState(stateIdx)
	return stateIdx
}

func (b *extraChainBuilder) buildEntryState(firstSym int, prodIdxs []int, follow bitset) int {
	key := extraChainStateKey(-(firstSym + 1), 0, &follow)
	if stateIdx, ok := b.entryStateCache[key]; ok {
		return stateIdx
	}

	stateIdx := b.newState()
	b.entryStateCache[key] = stateIdx
	for _, prodIdx := range prodIdxs {
		b.addProdContinuation(stateIdx, prodIdx, 1, follow)
	}
	b.finalizeState(stateIdx)
	return stateIdx
}

func (b *extraChainBuilder) unionSyntheticStates(a, c int) int {
	if a == c || a < b.syntheticStart || c < b.syntheticStart {
		return a
	}
	if a > c {
		a, c = c, a
	}
	key := fmt.Sprintf("%d:%d", a, c)
	if stateIdx, ok := b.unionStateCache[key]; ok {
		return stateIdx
	}

	stateIdx := b.newState()
	b.unionStateCache[key] = stateIdx
	for _, src := range []int{a, c} {
		if srcActions, ok := b.tables.ActionTable[src]; ok {
			for sym, acts := range srcActions {
				for _, act := range acts {
					b.tables.addAction(stateIdx, sym, act)
				}
			}
		}
		if srcGotos, ok := b.tables.GotoTable[src]; ok {
			for sym, target := range srcGotos {
				existing, ok := b.tables.GotoTable[stateIdx][sym]
				if !ok || existing == target {
					b.tables.GotoTable[stateIdx][sym] = target
					continue
				}
				if existing >= b.syntheticStart && target >= b.syntheticStart {
					b.tables.GotoTable[stateIdx][sym] = b.unionSyntheticStates(existing, target)
					continue
				}
				if os.Getenv("GTS_TMP_EXTRA_CHAIN_GOTO_CONFLICTS") == "1" {
					fmt.Printf("[extra-goto-conflict] union state=%d sym=%s existing=%d new=%d\n",
						stateIdx, b.ng.Symbols[sym].Name, existing, target)
				}
			}
		}
	}
	b.finalizeState(stateIdx)
	return stateIdx
}

func (b *extraChainBuilder) addProdContinuation(stateIdx, prodIdx, pos int, follow bitset) {
	prod := &b.ng.Productions[prodIdx]
	if pos >= len(prod.RHS) {
		follow.forEach(func(la int) {
			b.tables.addAction(stateIdx, la, lrAction{
				kind:    lrReduce,
				prodIdx: prodIdx,
				lhsSym:  prod.LHS,
				isExtra: prod.IsExtra,
			})
		})
		return
	}

	nextSym := prod.RHS[pos]
	if nextSym < b.tokenCount {
		targetState := b.buildProdChain(prodIdx, pos+1, follow)
		b.tables.addAction(stateIdx, nextSym, lrAction{
			kind:    lrShift,
			state:   targetState,
			prec:    prod.Prec,
			assoc:   prod.Assoc,
			lhsSym:  prod.LHS,
			isExtra: prod.IsExtra,
		})
		return
	}

	targetState := b.buildProdChain(prodIdx, pos+1, follow)
	existing, ok := b.tables.GotoTable[stateIdx][nextSym]
	if !ok || existing == targetState {
		b.tables.GotoTable[stateIdx][nextSym] = targetState
	} else if existing >= b.syntheticStart && targetState >= b.syntheticStart {
		b.tables.GotoTable[stateIdx][nextSym] = b.unionSyntheticStates(existing, targetState)
	} else if os.Getenv("GTS_TMP_EXTRA_CHAIN_GOTO_CONFLICTS") == "1" {
		fmt.Printf("[extra-goto-conflict] prod-cont state=%d sym=%s existing=%d new=%d follow=%s\n",
			stateIdx, b.ng.Symbols[nextSym].Name, existing, targetState, dumpExtraChainBitset(b.ng, &follow))
	}
	nextFollow := b.ctx.firstOfSequenceWithFallback(prod.RHS[pos+1:], &follow)
	b.addNonterminalEntries(stateIdx, nextSym, nextFollow)
}

func (b *extraChainBuilder) addNonterminalEntries(stateIdx, sym int, follow bitset) {
	key := extraChainStateKey(stateIdx, sym, &follow)
	if b.entrySeen[key] {
		return
	}
	b.entrySeen[key] = true

	for _, prodIdx := range b.ctx.prodsByLHS[sym] {
		prod := &b.ng.Productions[prodIdx]
		if len(prod.RHS) == 0 {
			follow.forEach(func(la int) {
				b.tables.addAction(stateIdx, la, lrAction{
					kind:    lrReduce,
					prodIdx: prodIdx,
					lhsSym:  prod.LHS,
					isExtra: prod.IsExtra,
				})
			})
			continue
		}

		firstSym := prod.RHS[0]
		if firstSym < b.tokenCount {
			targetState := b.buildProdChain(prodIdx, 1, follow)
			b.tables.addAction(stateIdx, firstSym, lrAction{
				kind:    lrShift,
				state:   targetState,
				prec:    prod.Prec,
				assoc:   prod.Assoc,
				lhsSym:  prod.LHS,
				isExtra: prod.IsExtra,
			})
			continue
		}

		targetState := b.buildProdChain(prodIdx, 1, follow)
		existing, ok := b.tables.GotoTable[stateIdx][firstSym]
		if !ok || existing == targetState {
			b.tables.GotoTable[stateIdx][firstSym] = targetState
		} else if existing >= b.syntheticStart && targetState >= b.syntheticStart {
			b.tables.GotoTable[stateIdx][firstSym] = b.unionSyntheticStates(existing, targetState)
		} else if os.Getenv("GTS_TMP_EXTRA_CHAIN_GOTO_CONFLICTS") == "1" {
			fmt.Printf("[extra-goto-conflict] nt-entry state=%d sym=%s existing=%d new=%d follow=%s\n",
				stateIdx, b.ng.Symbols[firstSym].Name, existing, targetState, dumpExtraChainBitset(b.ng, &follow))
		}
		nextFollow := b.ctx.firstOfSequenceWithFallback(prod.RHS[1:], &follow)
		b.addNonterminalEntries(stateIdx, firstSym, nextFollow)
	}
}

func dumpExtraChainBitset(ng *NormalizedGrammar, bs *bitset) string {
	if bs == nil {
		return "<nil>"
	}
	names := make([]string, 0)
	bs.forEach(func(sym int) {
		if sym >= 0 && sym < len(ng.Symbols) {
			names = append(names, ng.Symbols[sym].Name)
		} else {
			names = append(names, fmt.Sprintf("%d", sym))
		}
	})
	return strings.Join(names, ",")
}

// addNonterminalExtraChains creates dedicated parse state chains for nonterminal
// extra productions and adds shift actions from every main state.
func addNonterminalExtraChains(tables *LRTables, ng *NormalizedGrammar, ctx *lrContext) {
	tokenCount := ng.TokenCount()
	if len(ng.ExtraSymbols) == 0 {
		return
	}

	var extraProds []int
	for i := range ng.Productions {
		if ng.Productions[i].IsExtra && len(ng.Productions[i].RHS) > 0 {
			extraProds = append(extraProds, i)
		}
	}
	if len(extraProds) == 0 {
		return
	}

	mainStateCount := tables.StateCount

	var terminalExtras []int
	for _, e := range ng.ExtraSymbols {
		if e > 0 && e < tokenCount {
			terminalExtras = append(terminalExtras, e)
		}
	}

	extraStartsByFirstSym := make(map[int][]int)
	var extraFirstSyms []int
	for _, prodIdx := range extraProds {
		prod := &ng.Productions[prodIdx]
		if len(prod.RHS) > 0 && prod.RHS[0] < tokenCount {
			firstSym := prod.RHS[0]
			if _, ok := extraStartsByFirstSym[firstSym]; !ok {
				extraFirstSyms = append(extraFirstSyms, firstSym)
			}
			extraStartsByFirstSym[firstSym] = append(extraStartsByFirstSym[firstSym], prodIdx)
		}
	}

	stateFollowSets := make([]bitset, mainStateCount)
	for state := 0; state < mainStateCount; state++ {
		follow := newBitset(tokenCount)
		if acts, ok := tables.ActionTable[state]; ok {
			for sym, actionList := range acts {
				if sym < tokenCount && len(actionList) > 0 {
					follow.add(sym)
				}
			}
		}
		for _, extraSym := range terminalExtras {
			follow.add(extraSym)
		}
		for _, firstSym := range extraFirstSyms {
			follow.add(firstSym)
		}
		stateFollowSets[state] = follow
	}

	builder := newExtraChainBuilder(tables, ng, ctx, terminalExtras)

	for state := 0; state < mainStateCount; state++ {
		follow := stateFollowSets[state]
		for firstSym, prodIdxs := range extraStartsByFirstSym {
			entryState := builder.buildEntryState(firstSym, prodIdxs, follow)
			tables.addAction(state, firstSym, lrAction{
				kind:    lrShift,
				state:   entryState,
				lhsSym:  0,
				isExtra: true,
			})
		}
	}
}

// computeFirstSets computes FIRST sets for all symbols using bitsets.
func (ctx *lrContext) computeFirstSets() {
	ng := ctx.ng
	tokenCount := ctx.tokenCount

	// Initialize: terminals have FIRST = {self}
	for i, sym := range ng.Symbols {
		ctx.firstSets[i] = newBitset(tokenCount)
		if sym.Kind == SymbolTerminal || sym.Kind == SymbolNamedToken || sym.Kind == SymbolExternal {
			ctx.firstSets[i].add(i)
		}
	}

	// Compute nullables.
	changed := true
	for changed {
		changed = false
		for _, prod := range ng.Productions {
			if ctx.nullables[prod.LHS] {
				continue
			}
			nullable := true
			for _, sym := range prod.RHS {
				if sym < tokenCount || !ctx.nullables[sym] {
					nullable = false
					break
				}
			}
			if nullable {
				ctx.nullables[prod.LHS] = true
				changed = true
			}
		}
	}

	// Iterate until fixed point.
	changed = true
	for changed {
		changed = false
		for _, prod := range ng.Productions {
			for _, sym := range prod.RHS {
				if ctx.firstSets[prod.LHS].unionWith(&ctx.firstSets[sym]) {
					changed = true
				}
				if sym >= tokenCount && ctx.nullables[sym] {
					continue
				}
				break
			}
		}
	}
}

// firstOfSequence computes FIRST(β) for a sequence of symbols.
func (ctx *lrContext) firstOfSequence(syms []int) bitset {
	result := newBitset(ctx.tokenCount)
	for _, sym := range syms {
		result.unionWith(&ctx.firstSets[sym])
		if sym < ctx.tokenCount || !ctx.nullables[sym] {
			return result
		}
	}
	return result
}

// firstOfSequenceWithFallback computes FIRST(β) for a sequence and unions the
// fallback lookaheads when the full sequence is nullable.
func (ctx *lrContext) firstOfSequenceWithFallback(syms []int, fallback *bitset) bitset {
	result := ctx.firstOfSequence(syms)
	for _, sym := range syms {
		if sym < ctx.tokenCount || !ctx.nullables[sym] {
			return result
		}
	}
	if fallback != nil {
		result.unionWith(fallback)
	}
	return result
}

// coreItem identifies an LR(0) core (production + dot position).
type coreItem struct {
	prodIdx, dot int
}

// closureToSet computes the closure of kernel items and returns an lrItemSet
// using core-based representation with bitset lookaheads.
func (ctx *lrContext) closureToSet(kernel []coreEntry) lrItemSet {
	ng := ctx.ng
	tokenCount := ctx.tokenCount

	// Reset dot0Index from previous call.
	for _, pi := range ctx.dot0Dirty {
		ctx.dot0Index[pi] = -1
	}
	ctx.dot0Dirty = ctx.dot0Dirty[:0]

	// Deduplicate only the incoming kernel up front. Newly discovered closure
	// entries are dot=0 items and are tracked by dot0Index during closure; the
	// final packed index can be built once at exact size after closure finishes.
	//
	// Seed the initial core slice capacity with the kernel plus the first-layer
	// production fanout of unique nonterminals visible in that kernel. This is a
	// cheap approximation of closure growth that reduces repeated backing-array
	// expansion at the hot dot-0 append site.
	capHint := len(kernel) * 2
	seenKernelNTs := make(map[int]bool, len(kernel))
	for _, ke := range kernel {
		prod := &ng.Productions[ke.prodIdx]
		if ke.dot >= len(prod.RHS) {
			continue
		}
		nextSym := prod.RHS[ke.dot]
		if nextSym < tokenCount || seenKernelNTs[nextSym] {
			continue
		}
		seenKernelNTs[nextSym] = true
		capHint += len(ctx.prodsByLHS[nextSym])
	}
	kernelIdx := make(map[uint64]int, len(kernel)*2)
	cores := make([]coreEntry, 0, capHint)
	for _, ke := range kernel {
		key := packCoreItemKey(ke.prodIdx, ke.dot)
		if idx, ok := kernelIdx[key]; ok {
			cores[idx].lookaheads.unionWith(&ke.lookaheads)
		} else {
			idx := len(cores)
			kernelIdx[key] = idx
			cores = append(cores, coreEntry{
				prodIdx:    ke.prodIdx,
				dot:        ke.dot,
				lookaheads: ctx.cloneLookaheadBitset(&ke.lookaheads),
			})
			// Populate dot0Index for kernel items at dot=0.
			if ke.dot == 0 {
				ctx.dot0Index[ke.prodIdx] = idx
				ctx.dot0Dirty = append(ctx.dot0Dirty, ke.prodIdx)
			}
		}
	}

	// Worklist of core indices that need (re-)processing.
	ctx.ensureClosureQueueCapacity(len(cores))
	queueGen := ctx.nextClosureQueueGen()
	worklist := ctx.closureWorklist[:0]
	for i := range cores {
		worklist = append(worklist, i)
		ctx.closureQueuedGen[i] = queueGen
	}
	head := 0

	for head < len(worklist) {
		ci := worklist[head]
		head++
		ctx.closureQueuedGen[ci] = 0

		ce := &cores[ci]
		prod := &ng.Productions[ce.prodIdx]
		if ce.dot >= len(prod.RHS) {
			continue
		}

		nextSym := prod.RHS[ce.dot]
		if nextSym < tokenCount {
			continue
		}

		br := ctx.getBetaFirst(ce.prodIdx, ce.dot)

		for _, prodIdx := range ctx.prodsByLHS[nextSym] {
			// Fast path: dot=0 lookup via flat array.
			tidx := ctx.dot0Index[prodIdx]
			exists := tidx >= 0

			if !exists {
				tidx = len(cores)
				ctx.dot0Index[prodIdx] = tidx
				ctx.dot0Dirty = append(ctx.dot0Dirty, prodIdx)
				cores = append(cores, coreEntry{
					prodIdx:    prodIdx,
					dot:        0,
					lookaheads: ctx.allocLookaheadBitset(),
				})
				ctx.ensureClosureQueueCapacity(tidx + 1)
			}

			addedNew := false
			// FIRST(β) lookaheads.
			if cores[tidx].lookaheads.unionWith(&br.first) {
				addedNew = true
			}
			// If β is nullable, propagate all source lookaheads.
			if br.nullable {
				if cores[tidx].lookaheads.unionWith(&ce.lookaheads) {
					addedNew = true
				}
			}
			// Re-process target if it gained new lookaheads.
			if addedNew && ctx.closureQueuedGen[tidx] != queueGen {
				worklist = append(worklist, tidx)
				ctx.closureQueuedGen[tidx] = queueGen
			}
		}
	}
	ctx.closureWorklist = worklist[:0]

	set := lrItemSet{
		cores: cores,
	}
	set.computeHashes(ng.Productions, &ctx.boundaryLookaheads, ctx.needReduceLAHash)
	return set
}

// closureIncremental propagates new lookaheads through an existing item set.
func (ctx *lrContext) closureIncremental(set *lrItemSet, newEntries []coreEntry) {
	ng := ctx.ng
	tokenCount := ctx.tokenCount

	// Merge new entries into existing set and track which cores changed.
	ctx.ensureClosureQueueCapacity(len(set.cores) + len(newEntries))
	queueGen := ctx.nextClosureQueueGen()
	worklist := ctx.closureWorklist[:0]

	for _, ne := range newEntries {
		if idx, ok := set.coreLookup(ne.prodIdx, ne.dot); ok {
			if set.cores[idx].lookaheads.unionWith(&ne.lookaheads) {
				if ctx.closureQueuedGen[idx] != queueGen {
					worklist = append(worklist, idx)
					ctx.closureQueuedGen[idx] = queueGen
				}
			}
		} else {
			idx = len(set.cores)
			set.setCoreIndex(ne.prodIdx, ne.dot, idx)
			set.cores = append(set.cores, coreEntry{
				prodIdx:    ne.prodIdx,
				dot:        ne.dot,
				lookaheads: ctx.cloneLookaheadBitset(&ne.lookaheads),
			})
			ctx.ensureClosureQueueCapacity(idx + 1)
			worklist = append(worklist, idx)
			ctx.closureQueuedGen[idx] = queueGen
		}
	}
	head := 0

	for head < len(worklist) {
		ci := worklist[head]
		head++
		ctx.closureQueuedGen[ci] = 0

		ce := &set.cores[ci]
		prod := &ng.Productions[ce.prodIdx]
		if ce.dot >= len(prod.RHS) {
			continue
		}

		nextSym := prod.RHS[ce.dot]
		if nextSym < tokenCount {
			continue
		}

		br := ctx.getBetaFirst(ce.prodIdx, ce.dot)

		for _, prodIdx := range ctx.prodsByLHS[nextSym] {
			tidx, exists := set.coreLookup(prodIdx, 0)

			if !exists {
				tidx = len(set.cores)
				set.setCoreIndex(prodIdx, 0, tidx)
				set.cores = append(set.cores, coreEntry{
					prodIdx:    prodIdx,
					dot:        0,
					lookaheads: ctx.allocLookaheadBitset(),
				})
				ctx.ensureClosureQueueCapacity(tidx + 1)
			}

			addedNew := false
			if set.cores[tidx].lookaheads.unionWith(&br.first) {
				addedNew = true
			}
			if br.nullable {
				if set.cores[tidx].lookaheads.unionWith(&ce.lookaheads) {
					addedNew = true
				}
			}
			if addedNew {
				if ctx.closureQueuedGen[tidx] != queueGen {
					worklist = append(worklist, tidx)
					ctx.closureQueuedGen[tidx] = queueGen
				}
			}
		}
	}
	ctx.closureWorklist = worklist[:0]

	set.computeHashes(ng.Productions, &ctx.boundaryLookaheads, ctx.needReduceLAHash)
}

// betaResult caches the FIRST set and nullability of a production suffix.
type betaResult struct {
	first    bitset
	nullable bool
}

// getBetaFirst returns the cached FIRST(β) for the suffix after the dot in an item.
func (ctx *lrContext) getBetaFirst(prodIdx, dot int) *betaResult {
	bk := uint32(prodIdx)<<16 | uint32(dot)
	if cached, ok := ctx.betaCache[bk]; ok {
		return cached
	}
	prod := &ctx.ng.Productions[prodIdx]
	beta := prod.RHS[dot+1:]
	result := &betaResult{
		first:    ctx.firstOfSequence(beta),
		nullable: true,
	}
	for _, sym := range beta {
		if sym < ctx.tokenCount || !ctx.nullables[sym] {
			result.nullable = false
			break
		}
	}
	ctx.betaCache[bk] = result
	return result
}

// mixCoreItem hashes a (prodIdx, dot) pair into a well-distributed uint64.
func mixCoreItem(p, d int) uint64 {
	x := uint64(p)*0x9e3779b97f4a7c15 + uint64(d)*0x517cc1b727220a95
	x ^= x >> 33
	x *= 0xff51afd7ed558ccd
	x ^= x >> 33
	return x
}

func maskedBitsetHash(b, mask *bitset) uint64 {
	h := uint64(0xcbf29ce484222325) // FNV offset basis
	maxLen := len(b.words)
	if mask != nil && len(mask.words) > maxLen {
		maxLen = len(mask.words)
	}
	for i := 0; i < maxLen; i++ {
		var bw, mw uint64
		if i < len(b.words) {
			bw = b.words[i]
		}
		if mask != nil && i < len(mask.words) {
			mw = mask.words[i]
		} else {
			mw = ^uint64(0)
		}
		h ^= bw & mw
		h *= 0x100000001b3 // FNV prime
	}
	return h
}

func maskedBitsetEqual(a, b, mask *bitset) bool {
	maxLen := len(a.words)
	if len(b.words) > maxLen {
		maxLen = len(b.words)
	}
	if mask != nil && len(mask.words) > maxLen {
		maxLen = len(mask.words)
	}
	for i := 0; i < maxLen; i++ {
		var aw, bw, mw uint64
		if i < len(a.words) {
			aw = a.words[i]
		}
		if i < len(b.words) {
			bw = b.words[i]
		}
		if mask != nil && i < len(mask.words) {
			mw = mask.words[i]
		} else {
			mw = ^uint64(0)
		}
		if aw&mw != bw&mw {
			return false
		}
	}
	return true
}

func sameAnnotationArgTag(a, b *lrItemSet) bool {
	return a.annotationArgTag == b.annotationArgTag
}

func (ctx *lrContext) isAnnotationArgumentEntrySet(set *lrItemSet) bool {
	if ctx.annotationAtSym < 0 || ctx.annotationDefSym < 0 || ctx.annotationOpenParenSym < 0 {
		return false
	}
	for _, ce := range set.cores {
		prod := ctx.ng.Productions[ce.prodIdx]
		if ctx.ng.Symbols[prod.LHS].Name != "arguments" {
			continue
		}
		if ce.dot != 1 || len(prod.RHS) == 0 || prod.RHS[0] != ctx.annotationOpenParenSym {
			continue
		}
		if ce.lookaheads.contains(ctx.annotationAtSym) && ce.lookaheads.contains(ctx.annotationDefSym) {
			return true
		}
	}
	return false
}

func (ctx *lrContext) isAnnotationArgumentCarrierSet(set *lrItemSet) bool {
	if ctx.annotationCloseParenSym < 0 {
		return false
	}
	for _, ce := range set.cores {
		prod := ctx.ng.Productions[ce.prodIdx]
		if prod.LHS < 0 || prod.LHS >= len(ctx.annotationArgCarrierLHS) || !ctx.annotationArgCarrierLHS[prod.LHS] {
			continue
		}
		if ce.lookaheads.contains(ctx.annotationCloseParenSym) {
			return true
		}
	}
	return false
}

func (ctx *lrContext) annotationArgTagForTransition(sourceState int, closedSet *lrItemSet) uint32 {
	if len(ctx.ng.Productions) < 2000 || sourceState < 0 || sourceState >= len(ctx.itemSets) {
		return 0
	}
	if srcTag := ctx.itemSets[sourceState].annotationArgTag; srcTag != 0 {
		if ctx.isAnnotationArgumentCarrierSet(closedSet) {
			return srcTag
		}
		return 0
	}
	if ctx.isAnnotationArgumentEntrySet(closedSet) {
		return 1
	}
	return 0
}

// computeHashes computes coreHash, fullHash, and reduceLAHash for the item set.
// Uses commutative (additive) hashing so order of cores doesn't matter,
// avoiding the need to sort.
func (set *lrItemSet) computeHashes(prods []Production, boundaryMask *bitset, includeReduceHash bool) {
	var ch, fh, rh, brh uint64
	for _, c := range set.cores {
		m := mixCoreItem(c.prodIdx, c.dot)
		ch += m
		fh += m ^ c.lookaheads.hash()
		if boundaryMask != nil {
			brh += maskedBitsetHash(&c.lookaheads, boundaryMask)
		}
		if includeReduceHash && c.dot >= len(prods[c.prodIdx].RHS) {
			laHash := c.lookaheads.hash()
			rh += laHash
		}
	}
	set.coreHash = ch
	set.fullHash = fh
	if includeReduceHash {
		set.reduceLAHash = ch + rh
	} else {
		set.reduceLAHash = ch
	}
	set.boundaryLAHash = ch + brh
}

// sameCores returns true if two item sets have identical core items.
func sameCoresUsingIndexed(indexed, other *lrItemSet) bool {
	indexed.ensurePackedCoreIndex()
	if len(indexed.cores) != len(other.cores) {
		return false
	}
	for _, oc := range other.cores {
		if _, ok := indexed.coreLookup(oc.prodIdx, oc.dot); !ok {
			return false
		}
	}
	return true
}

// sameFullItemsUsingIndexed returns true if two item sets are identical
// (cores + lookaheads), using the indexed set for core lookups.
func sameFullItemsUsingIndexed(indexed, other *lrItemSet) bool {
	indexed.ensurePackedCoreIndex()
	if len(indexed.cores) != len(other.cores) {
		return false
	}
	for _, oc := range other.cores {
		idx, ok := indexed.coreLookup(oc.prodIdx, oc.dot)
		if !ok {
			return false
		}
		if !indexed.cores[idx].lookaheads.equal(&oc.lookaheads) {
			return false
		}
	}
	return true
}

// sameReduceLookaheadsUsingIndexed returns true if two item sets have the same
// lookaheads on all reduce items, assuming their cores already match.
func sameReduceLookaheadsUsingIndexed(indexed, other *lrItemSet, prods []Production) bool {
	indexed.ensurePackedCoreIndex()
	for _, oc := range other.cores {
		if oc.dot < len(prods[oc.prodIdx].RHS) {
			continue // not a reduce item
		}
		idx, ok := indexed.coreLookup(oc.prodIdx, oc.dot)
		if !ok {
			return false
		}
		if !indexed.cores[idx].lookaheads.equal(&oc.lookaheads) {
			return false
		}
	}
	return true
}

// sameBoundaryLookaheadsUsingIndexed returns true if two item sets have the
// same EOF and external-token lookaheads on all items, assuming their cores
// already match.
func sameBoundaryLookaheadsUsingIndexed(indexed, other *lrItemSet, boundaryMask *bitset) bool {
	indexed.ensurePackedCoreIndex()
	for _, oc := range other.cores {
		idx, ok := indexed.coreLookup(oc.prodIdx, oc.dot)
		if !ok {
			return false
		}
		if !maskedBitsetEqual(&indexed.cores[idx].lookaheads, &oc.lookaheads, boundaryMask) {
			return false
		}
	}
	return true
}

// stateHashEntry is a linked list node for hash-based state lookup.
type stateHashEntry struct {
	stateIdx int
	next     *stateHashEntry
}

// buildItemSets constructs LR(1) item sets with LALR-like merging.
//
// Uses hash-based state deduplication and core-based item representation
// with bitset lookaheads for performance on large grammars.
func (ctx *lrContext) buildItemSets() []lrItemSet {
	ctx.transitions = make(map[int]map[int]int)
	ctx.ensureProvenance()

	tokenCount := ctx.tokenCount

	// Hash tables for state lookup.
	// fullMap: fullHash → chain of states with that hash (exact LR(1) match)
	fullMap := make(map[uint64]*stateHashEntry)
	// coreMap: coreHash → chain of states (for LALR merge)
	coreMap := make(map[uint64]*stateHashEntry)
	// extMap: reduceLAHash → chain of states (for extended merge)
	extMap := make(map[uint64]*stateHashEntry)
	// boundaryMap: boundaryLAHash → chain of states for large-grammar
	// external-token-sensitive merges.
	boundaryMap := make(map[uint64]*stateHashEntry)

	// For large grammars, use LALR merging from the start to avoid state explosion.
	const maxExtendedStates = 8000
	useExtendedMerging := len(ctx.ng.Productions) <= 800
	useBoundaryMerging := len(ctx.ng.ExternalSymbols) > 0 && len(ctx.ng.Productions) > 800
	ctx.needReduceLAHash = useExtendedMerging

	// Initial item set: closure of [S' → .S, $end]
	initialLA := newBitset(tokenCount)
	initialLA.add(0) // $end
	initialSet := ctx.closureToSet([]coreEntry{{
		prodIdx:    ctx.ng.AugmentProdID,
		dot:        0,
		lookaheads: initialLA,
	}})
	ctx.itemSets = []lrItemSet{initialSet}
	addToHashMap(fullMap, initialSet.fullHash, 0)
	if !useExtendedMerging && !useBoundaryMerging {
		addToHashMap(coreMap, initialSet.coreHash, 0)
	}
	if useExtendedMerging {
		addToHashMap(extMap, initialSet.reduceLAHash, 0)
	}
	if useBoundaryMerging {
		addToHashMap(boundaryMap, initialSet.boundaryLAHash, 0)
	}
	ctx.recordFreshState(0)

	worklist := []int{0}
	inWorklist := map[int]bool{0: true}

	for len(worklist) > 0 {
		stateIdx := worklist[0]
		worklist = worklist[1:]
		inWorklist[stateIdx] = false
		itemSet := &ctx.itemSets[stateIdx]

		// Collect all symbols after the dot.
		symsSeen := make(map[int]bool)
		syms := ctx.gotoSymbolsScratch[:0]
		for _, ce := range itemSet.cores {
			prod := &ctx.ng.Productions[ce.prodIdx]
			if ce.dot < len(prod.RHS) {
				sym := prod.RHS[ce.dot]
				if !symsSeen[sym] {
					symsSeen[sym] = true
					syms = append(syms, sym)
				}
			}
		}

		for _, sym := range syms {
			// Compute GOTO(itemSet, sym): advance dot past sym.
			advanced := ctx.gotoAdvancedScratch[:0]
			for _, ce := range itemSet.cores {
				prod := &ctx.ng.Productions[ce.prodIdx]
				if ce.dot < len(prod.RHS) && prod.RHS[ce.dot] == sym {
					advanced = append(advanced, coreEntry{
						prodIdx:    ce.prodIdx,
						dot:        ce.dot + 1,
						lookaheads: ce.lookaheads, // shared ref, closureToSet will clone
					})
				}
			}
			if len(advanced) == 0 {
				continue
			}

			closedSet := ctx.closureToSet(advanced)
			closedSet.annotationArgTag = ctx.annotationArgTagForTransition(stateIdx, &closedSet)
			ctx.gotoAdvancedScratch = advanced[:0]

			targetIdx := ctx.findOrCreateState(
				&closedSet,
				fullMap, coreMap, extMap, boundaryMap,
				useExtendedMerging && len(ctx.itemSets) < maxExtendedStates,
				useBoundaryMerging,
				&worklist, &inWorklist,
			)

			// Record transition for table construction.
			if ctx.transitions[stateIdx] == nil {
				ctx.transitions[stateIdx] = make(map[int]int, len(syms))
			}
			ctx.transitions[stateIdx][sym] = targetIdx
		}
		ctx.gotoSymbolsScratch = syms[:0]
	}

	return ctx.itemSets
}

func addToHashMap(m map[uint64]*stateHashEntry, hash uint64, idx int) {
	m[hash] = &stateHashEntry{stateIdx: idx, next: m[hash]}
}

// findOrCreateState looks up or creates a state for the given item set.
func (ctx *lrContext) findOrCreateState(
	closedSet *lrItemSet,
	fullMap, coreMap, extMap, boundaryMap map[uint64]*stateHashEntry,
	useExtended bool,
	useBoundary bool,
	worklist *[]int,
	inWorklist *map[int]bool,
) int {
	// 1. Check exact LR(1) match via fullHash.
	for entry := fullMap[closedSet.fullHash]; entry != nil; entry = entry.next {
		if sameAnnotationArgTag(&ctx.itemSets[entry.stateIdx], closedSet) &&
			sameFullItemsUsingIndexed(&ctx.itemSets[entry.stateIdx], closedSet) {
			ctx.recycleItemSetLookaheads(closedSet)
			return entry.stateIdx
		}
	}

	if useExtended {
		// 2a. Extended merging: find state with same core AND same reduce lookaheads.
		for entry := extMap[closedSet.reduceLAHash]; entry != nil; entry = entry.next {
			existing := &ctx.itemSets[entry.stateIdx]
			if sameAnnotationArgTag(existing, closedSet) &&
				existing.coreHash == closedSet.coreHash &&
				sameCoresUsingIndexed(existing, closedSet) &&
				sameReduceLookaheadsUsingIndexed(existing, closedSet, ctx.ng.Productions) {
				// Merge lookaheads into existing state.
				targetIdx := ctx.mergeInto(entry.stateIdx, closedSet, fullMap, extMap, boundaryMap, worklist, inWorklist)
				ctx.recycleItemSetLookaheads(closedSet)
				return targetIdx
			}
		}
	} else if useBoundary {
		for entry := boundaryMap[closedSet.boundaryLAHash]; entry != nil; entry = entry.next {
			existing := &ctx.itemSets[entry.stateIdx]
			if sameAnnotationArgTag(existing, closedSet) &&
				existing.coreHash == closedSet.coreHash &&
				sameCoresUsingIndexed(existing, closedSet) &&
				sameBoundaryLookaheadsUsingIndexed(existing, closedSet, &ctx.boundaryLookaheads) {
				targetIdx := ctx.mergeInto(entry.stateIdx, closedSet, fullMap, extMap, boundaryMap, worklist, inWorklist)
				ctx.recycleItemSetLookaheads(closedSet)
				return targetIdx
			}
		}
	} else {
		// 2b. LALR fallback: find state with same core.
		for entry := coreMap[closedSet.coreHash]; entry != nil; entry = entry.next {
			existing := &ctx.itemSets[entry.stateIdx]
			if sameAnnotationArgTag(existing, closedSet) &&
				sameCoresUsingIndexed(existing, closedSet) {
				targetIdx := ctx.mergeInto(entry.stateIdx, closedSet, fullMap, extMap, boundaryMap, worklist, inWorklist)
				ctx.recycleItemSetLookaheads(closedSet)
				return targetIdx
			}
		}
	}

	// 3. No match — create new state.
	newIdx := len(ctx.itemSets)
	ctx.itemSets = append(ctx.itemSets, *closedSet)
	addToHashMap(fullMap, closedSet.fullHash, newIdx)
	if coreMap != nil {
		addToHashMap(coreMap, closedSet.coreHash, newIdx)
	}
	if extMap != nil {
		addToHashMap(extMap, closedSet.reduceLAHash, newIdx)
	}
	if boundaryMap != nil {
		addToHashMap(boundaryMap, closedSet.boundaryLAHash, newIdx)
	}
	ctx.recordFreshState(newIdx)
	*worklist = append(*worklist, newIdx)
	(*inWorklist)[newIdx] = true
	return newIdx
}

// mergeInto merges lookaheads from closedSet into the existing state at idx.
func (ctx *lrContext) mergeInto(
	idx int,
	closedSet *lrItemSet,
	fullMap, extMap, boundaryMap map[uint64]*stateHashEntry,
	worklist *[]int,
	inWorklist *map[int]bool,
) int {
	// Collect new core entries to merge.
	var newEntries []coreEntry
	existing := &ctx.itemSets[idx]
	for _, ce := range closedSet.cores {
		if eidx, ok := existing.coreLookup(ce.prodIdx, ce.dot); ok {
			// Check if any new lookaheads.
			ec := &existing.cores[eidx]
			for wi, w := range ce.lookaheads.words {
				if wi < len(ec.lookaheads.words) {
					if w & ^ec.lookaheads.words[wi] != 0 {
						newEntries = append(newEntries, ce)
						break
					}
				} else if w != 0 {
					newEntries = append(newEntries, ce)
					break
				}
			}
		} else {
			newEntries = append(newEntries, ce)
		}
	}

	if len(newEntries) > 0 {
		oldReduceHash := existing.reduceLAHash
		oldBoundaryHash := existing.boundaryLAHash
		ctx.closureIncremental(existing, newEntries)
		ctx.recordMergedState(idx, mergeOrigin{
			kernelHash:  closedSet.coreHash,
			sourceState: -1,
		})
		// Update hash maps with new hashes.
		addToHashMap(fullMap, existing.fullHash, idx)
		if extMap != nil && existing.reduceLAHash != oldReduceHash {
			addToHashMap(extMap, existing.reduceLAHash, idx)
		}
		if boundaryMap != nil && existing.boundaryLAHash != oldBoundaryHash {
			addToHashMap(boundaryMap, existing.boundaryLAHash, idx)
		}
		if !(*inWorklist)[idx] {
			*worklist = append(*worklist, idx)
			(*inWorklist)[idx] = true
		}
	}
	return idx
}

// resolveConflicts resolves shift/reduce and reduce/reduce conflicts
// using precedence and associativity.
func resolveConflicts(tables *LRTables, ng *NormalizedGrammar) error {
	for state, actions := range tables.ActionTable {
		for sym, acts := range actions {
			if len(acts) <= 1 {
				continue
			}

			resolved, err := resolveActionConflict(sym, acts, ng)
			if err != nil {
				return fmt.Errorf("state %d, symbol %d: %w", state, sym, err)
			}
			tables.ActionTable[state][sym] = resolved
		}
	}
	return nil
}

// resolveActionConflict resolves a conflict between multiple actions.
func resolveActionConflict(lookaheadSym int, actions []lrAction, ng *NormalizedGrammar) ([]lrAction, error) {
	if len(actions) <= 1 {
		return actions, nil
	}
	cache := getConflictResolutionCache(ng)

	// Priority: non-extra actions always win over extra actions.
	hasExtra, hasNonExtra := false, false
	for _, a := range actions {
		if a.isExtra {
			hasExtra = true
		} else {
			hasNonExtra = true
		}
	}
	if hasExtra && hasNonExtra {
		var nonExtra []lrAction
		for _, a := range actions {
			if !a.isExtra {
				nonExtra = append(nonExtra, a)
			}
		}
		if len(nonExtra) <= 1 {
			return nonExtra, nil
		}
		actions = nonExtra
	}

	// Separate shifts and reduces.
	var shifts, reduces []lrAction
	for _, a := range actions {
		switch a.kind {
		case lrShift:
			shifts = append(shifts, a)
		case lrReduce:
			reduces = append(reduces, a)
		case lrAccept:
			return []lrAction{a}, nil
		}
	}

	// Shift/reduce conflict.
	if len(shifts) > 0 && len(reduces) > 0 {
		shift := shifts[0]
		reduce := reduces[0]
		prod := &ng.Productions[reduce.prodIdx]

		if shiftMatchesConflictGroup(shift, reduce.lhsSym, cache) {
			return actions, nil
		}
		if reduceLHSInConflictGroup(reduce.prodIdx, ng, cache) {
			return actions, nil
		}

		shiftPrec := shift.prec
		reducePrec := prod.Prec

		// Apply precedence/associativity resolution when either side has a
		// non-zero precedence OR the production declares explicit associativity.
		// PREC_LEFT(0) sets Assoc=AssocLeft with Prec=0; the associativity must
		// still be respected even though the precedence value is zero.
		if reducePrec != 0 || shiftPrec != 0 || prod.Assoc != AssocNone {
			if reducePrec > shiftPrec {
				return []lrAction{reduce}, nil
			}
			if shiftPrec > reducePrec {
				return []lrAction{shift}, nil
			}
			switch prod.Assoc {
			case AssocLeft:
				return []lrAction{reduce}, nil
			case AssocRight:
				return []lrAction{shift}, nil
			case AssocNone:
				return nil, nil
			}
		}
		if isTransitiveConflict(shift, reduce, ng, cache) {
			return actions, nil
		}

		// Targeted eex ambiguity.
		if len(prod.RHS) == 0 && len(ng.Symbols) > prod.LHS {
			reduceName := ng.Symbols[prod.LHS].Name
			if strings.HasPrefix(reduceName, "_partial_expression_repeat") {
				for _, s := range shifts {
					if s.lhsSym > 0 && s.lhsSym < len(ng.Symbols) &&
						strings.HasPrefix(ng.Symbols[s.lhsSym].Name, "_expression_repeat1_") {
						return actions, nil
					}
					for _, lhs := range s.lhsSyms {
						if lhs > 0 && lhs < len(ng.Symbols) &&
							strings.HasPrefix(ng.Symbols[lhs].Name, "_expression_repeat1_") {
							return actions, nil
						}
					}
				}
			}
		}

		// Default: prefer shift.
		return []lrAction{shift}, nil
	}

	// Reduce/reduce conflict.
	// Tree-sitter resolves ALL R/R conflicts by picking the highest-prec
	// production (then lowest prodIdx) unless they're in a declared conflict
	// group (kept as GLR). The previous hasEpsilon guard only resolved
	// epsilon R/R conflicts, leaving non-epsilon R/R as ambiguous table
	// entries which caused type="" parse failures.
	if len(reduces) > 1 {
		return resolveReduceReduceLegacy(lookaheadSym, reduces, ng, cache)
	}

	return actions, nil
}

func resolveReduceReduceLegacy(lookaheadSym int, reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) ([]lrAction, error) {
	if allInDeclaredConflict(reduces, ng, cache) {
		return reduces, nil
	}
	if shouldKeepRepeatedAnnotationReduces(lookaheadSym, reduces, ng) {
		return reduces, nil
	}

	best := reduces[0]
	bestProd := &ng.Productions[best.prodIdx]
	for _, r := range reduces[1:] {
		rProd := &ng.Productions[r.prodIdx]
		if rProd.Prec > bestProd.Prec {
			best = r
			bestProd = rProd
		} else if rProd.Prec == bestProd.Prec {
			// Tree-sitter uses dynamic precedence as the next tiebreaker,
			// then falls back to production index (earlier declaration wins).
			if rProd.DynPrec > bestProd.DynPrec {
				best = r
				bestProd = rProd
			} else if rProd.DynPrec == bestProd.DynPrec && r.prodIdx < best.prodIdx {
				best = r
				bestProd = rProd
			}
		}
	}
	return []lrAction{best}, nil
}

func shouldKeepRepeatedAnnotationReduces(lookaheadSym int, reduces []lrAction, ng *NormalizedGrammar) bool {
	if len(reduces) < 2 || lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) {
		return false
	}
	if ng.Symbols[lookaheadSym].Name != "@" {
		return false
	}

	for _, r := range reduces {
		prod := &ng.Productions[r.prodIdx]
		if prod.Prec != 0 || prod.DynPrec != 0 || prod.Assoc != AssocNone {
			return false
		}
		if len(prod.RHS) != 1 {
			return false
		}
		rhs := prod.RHS[0]
		if rhs < 0 || rhs >= len(ng.Symbols) || ng.Symbols[rhs].Name != "annotation" {
			return false
		}
		lhs := prod.LHS
		if lhs < 0 || lhs >= len(ng.Symbols) {
			return false
		}
		lhsName := ng.Symbols[lhs].Name
		if !strings.Contains(lhsName, "repeat") {
			return false
		}
	}
	return true
}

func shiftMatchesConflictGroup(shift lrAction, reduceLHS int, cache *conflictResolutionCache) bool {
	if cache == nil || reduceLHS < 0 || reduceLHS >= len(cache.groupsBySymbol) {
		return false
	}
	allShiftLHS := make([]int, 0, 1+len(shift.lhsSyms))
	if shift.lhsSym != 0 {
		allShiftLHS = append(allShiftLHS, shift.lhsSym)
	}
	allShiftLHS = append(allShiftLHS, shift.lhsSyms...)

	for _, groupIdx := range cache.groupsBySymbol[reduceLHS] {
		for _, sym := range cache.groups[groupIdx] {
			for _, shiftLHS := range allShiftLHS {
				if sym == shiftLHS && shiftLHS != reduceLHS {
					return true
				}
			}
		}
	}
	return false
}

// reduceLHSInConflictGroup checks whether the reduce production's LHS symbol
// appears in any declared conflict group. This is used as a fallback in S/R
// conflict resolution: if the reduce's LHS is in a conflict group, the conflict
// is kept as GLR even if the shift's LHS is not in the same group.
func reduceLHSInConflictGroup(prodIdx int, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if cache == nil {
		return false
	}
	prod := &ng.Productions[prodIdx]
	if prod.LHS < 0 || prod.LHS >= len(cache.symbolInConflict) {
		return false
	}
	return cache.symbolInConflict[prod.LHS]
}

func isTransitiveConflict(shift lrAction, reduce lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if cache == nil {
		return false
	}

	reduceLHS := ng.Productions[reduce.prodIdx].LHS
	if reduceLHS < 0 || reduceLHS >= len(cache.symbolInConflict) {
		return false
	}
	if cache.symbolInConflict[reduceLHS] {
		return false
	}

	allShiftLHS := make(map[int]bool)
	if shift.lhsSym != 0 {
		allShiftLHS[shift.lhsSym] = true
	}
	for _, s := range shift.lhsSyms {
		allShiftLHS[s] = true
	}

	shiftConflictSyms := make([]bool, len(cache.symbolInConflict))
	shiftConflictCount := 0
	markShiftConflict := func(sym int) {
		if sym < 0 || sym >= len(shiftConflictSyms) || !cache.symbolInConflict[sym] || shiftConflictSyms[sym] {
			return
		}
		shiftConflictSyms[sym] = true
		shiftConflictCount++
	}
	for s := range allShiftLHS {
		markShiftConflict(s)
		if s < 0 || s >= len(cache.reverseIdx) {
			continue
		}
		for _, pi := range cache.reverseIdx[s] {
			prod := &ng.Productions[pi]
			markShiftConflict(prod.LHS)
			for _, rs := range prod.RHS {
				markShiftConflict(rs)
			}
		}
	}

	if shiftConflictCount == 0 {
		return false
	}

	visited := make([]bool, len(cache.symbolInConflict))
	visited[reduceLHS] = true
	queue := []int{reduceLHS}
	maxDepth := 4

	for depth := 0; depth < maxDepth && len(queue) > 0; depth++ {
		var next []int
		for _, sym := range queue {
			if sym < 0 || sym >= len(cache.reverseIdx) {
				continue
			}
			for _, pi := range cache.reverseIdx[sym] {
				prod := &ng.Productions[pi]
				if anyMarkedConflictGroup(prod.LHS, shiftConflictSyms, cache) {
					return true
				}
				for _, rs := range prod.RHS {
					if rs != sym && anyMarkedConflictGroup(rs, shiftConflictSyms, cache) {
						return true
					}
				}

				if prod.LHS >= 0 && prod.LHS < len(visited) && !visited[prod.LHS] {
					visited[prod.LHS] = true
					next = append(next, prod.LHS)
				}
			}
		}
		queue = next
	}
	return false
}

func anyMarkedConflictGroup(sym int, marked []bool, cache *conflictResolutionCache) bool {
	if cache == nil || sym < 0 || sym >= len(cache.groupsBySymbol) {
		return false
	}
	for _, groupIdx := range cache.groupsBySymbol[sym] {
		for _, member := range cache.groups[groupIdx] {
			if member >= 0 && member < len(marked) && marked[member] {
				return true
			}
		}
	}
	return false
}

func allInDeclaredConflict(reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if len(reduces) < 2 || cache == nil {
		return false
	}
	for _, cgroup := range cache.groups {
		allFound := true
		for _, r := range reduces {
			lhs := ng.Productions[r.prodIdx].LHS
			found := false
			for _, sym := range cgroup {
				if sym == lhs {
					found = true
					break
				}
			}
			if !found {
				allFound = false
				break
			}
		}
		if allFound {
			return true
		}
	}
	return false
}
