package grammargen

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// coreEntry is a core item (prodIdx, dot) with a bitset of lookahead terminals.
// This avoids expanding N lookaheads into N individual lrItems during closure.
type coreEntry struct {
	prodIdx    uint32
	dot        uint32
	lookaheads bitset
}

// lr0CoreEntry packs the retained LR(0) core into 4 bytes by storing the
// production index in 24 bits and the dot position in 8 bits. Large LALR
// builds keep hundreds of millions of these entries live until lookahead
// materialization, so shrinking from the old uint32/uint32 pair materially
// lowers peak heap usage at Fortran-scale core counts.
//
// The LR(0) path only needs the production index and dot position. Dot
// positions are already guarded to one byte. Normalized grammars stay far below
// 16M productions, but pack time still checks the 24-bit limit so an outlier
// fails loudly instead of silently corrupting state identity.
type lr0CoreEntry uint32

func packLR0CoreEntry(prodIdx, dot int) lr0CoreEntry {
	if prodIdx < 0 || prodIdx > 0x00FFFFFF {
		panic(fmt.Sprintf("lr0 prodIdx out of range: %d", prodIdx))
	}
	if dot < 0 || dot > 0xFF {
		panic(fmt.Sprintf("lr0 dot out of range: %d", dot))
	}
	return lr0CoreEntry(uint32(prodIdx) | uint32(dot)<<24)
}

func (ce lr0CoreEntry) prodIdx() uint32 {
	return uint32(ce) & 0x00FFFFFF
}

func (ce lr0CoreEntry) dot() uint8 {
	return uint8(uint32(ce) >> 24)
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
	// completionLAHash is a hash of lookaheads on the completion frontier:
	// completed items plus items with exactly one symbol remaining. Extended
	// merging preserves these contexts because they become effective reduce
	// lookaheads after at most one transition.
	completionLAHash uint64
	// boundaryLAHash is a hash of only the EOF/external-token lookaheads across
	// all items. This helps preserve boundary-sensitive contexts in very large
	// external-scanner grammars.
	boundaryLAHash uint64
	// annotationArgTag packs narrow predecessor-sensitive context bits that keep
	// large Scala-like fallback automata from over-merging.
	annotationArgTag uint32
}

type lr0ItemSet struct {
	cores            []lr0CoreEntry
	coreHash         uint64
	annotationArgTag uint32
}

type lrTransition struct {
	sym    uint32
	target uint32
}

type lrTransitionRow []lrTransition

const (
	templateContextTagShift          = 16
	templateContextTagMask    uint32 = 0x00ff0000
	templateContextPendingTag uint32 = 1 << templateContextTagShift
	conditionalTypeContextTag uint32 = 1 << 10
)

func (set *lrItemSet) coreLookup(prodIdx, dot int) (int, bool) {
	if set.packedCoreIndex != nil {
		idx, ok := set.packedCoreIndex[packCoreItemKey(prodIdx, dot)]
		return idx, ok
	}
	if set.coreIndex != nil {
		idx, ok := set.coreIndex[coreItem{prodIdx: prodIdx, dot: dot}]
		return idx, ok
	}
	lo, hi := 0, len(set.cores)
	prodIdx32 := uint32(prodIdx)
	dot32 := uint32(dot)
	for lo < hi {
		mid := (lo + hi) / 2
		ce := set.cores[mid]
		if ce.prodIdx < prodIdx32 || (ce.prodIdx == prodIdx32 && ce.dot < dot32) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(set.cores) {
		ce := set.cores[lo]
		if ce.prodIdx == prodIdx32 && ce.dot == dot32 {
			return lo, true
		}
	}
	return 0, false
}

func (set *lr0ItemSet) coreLookup(prodIdx, dot int) (int, bool) {
	lo, hi := 0, len(set.cores)
	prodIdx32 := uint32(prodIdx)
	dot8 := uint8(dot)
	for lo < hi {
		mid := (lo + hi) / 2
		ce := set.cores[mid]
		ceProdIdx := ce.prodIdx()
		ceDot := ce.dot()
		if ceProdIdx < prodIdx32 || (ceProdIdx == prodIdx32 && ceDot < dot8) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(set.cores) {
		ce := set.cores[lo]
		if ce.prodIdx() == prodIdx32 && ce.dot() == dot8 {
			return lo, true
		}
	}
	return 0, false
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
		packedCoreIndex[packCoreItemKey(int(ce.prodIdx), int(ce.dot))] = idx
	}
	set.packedCoreIndex = packedCoreIndex
	set.coreIndex = nil
}

func sameSortedLR0CoreEntries(a, b []lr0CoreEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// lrAction is a parse table action.
type lrAction struct {
	kind              lrActionKind
	state             int   // shift target / goto target
	prodIdx           int   // reduce production index
	prec              int   // for shift: precedence of the item's production
	hasPrec           bool  // production had an explicit compile-time precedence wrapper
	assoc             Assoc // for shift: associativity of the item's production
	lhsSym            int   // LHS nonterminal of the production (for conflict detection)
	lhsSyms           []int // additional LHS symbols (when shifts from multiple rules merge)
	shiftContributors []lrShiftContributor
	isExtra           bool  // true if this action comes from a nonterminal extra production
	repeat            bool  // true if this shift continues a recursive repeat wrapper
	repeatLHS         int   // generated repeat-helper LHS continued by this shift, or 0 when unknown
	repeatLHSSyms     []int // additional generated repeat-helper LHS symbols for merged shifts
}

type lrShiftContributor struct {
	lhsSym  int
	prec    int
	hasPrec bool
	assoc   Assoc
}

func (a *lrAction) addRepeatLHS(lhs int) {
	if lhs <= 0 {
		return
	}
	a.repeat = true
	if a.repeatLHS == 0 {
		a.repeatLHS = lhs
		return
	}
	if a.repeatLHS == lhs {
		return
	}
	for _, existing := range a.repeatLHSSyms {
		if existing == lhs {
			return
		}
	}
	a.repeatLHSSyms = append(a.repeatLHSSyms, lhs)
}

func (a *lrAction) addRepeatLHSFrom(other lrAction) {
	a.addRepeatLHS(other.repeatLHS)
	for _, lhs := range other.repeatLHSSyms {
		a.addRepeatLHS(lhs)
	}
	if other.repeat && a.repeatLHS == 0 && len(a.repeatLHSSyms) == 0 {
		a.repeat = true
	}
}

func (a lrAction) hasRepeatLHS(lhs int) bool {
	if lhs <= 0 {
		return false
	}
	if a.repeatLHS == lhs {
		return true
	}
	for _, existing := range a.repeatLHSSyms {
		if existing == lhs {
			return true
		}
	}
	return false
}

func (a *lrAction) ensureShiftContributors() {
	if a == nil || a.kind != lrShift {
		return
	}
	if len(a.shiftContributors) > 0 {
		return
	}
	a.addShiftContributor(a.lhsSym, a.prec, a.hasPrec, a.assoc)
	for _, lhs := range a.lhsSyms {
		a.addShiftContributor(lhs, a.prec, a.hasPrec, a.assoc)
	}
}

func (a *lrAction) addShiftContributor(lhs, prec int, hasPrec bool, assoc Assoc) {
	if a == nil || lhs <= 0 {
		return
	}
	for _, existing := range a.shiftContributors {
		if existing.lhsSym == lhs && existing.prec == prec && existing.hasPrec == hasPrec && existing.assoc == assoc {
			return
		}
	}
	a.shiftContributors = append(a.shiftContributors, lrShiftContributor{
		lhsSym:  lhs,
		prec:    prec,
		hasPrec: hasPrec,
		assoc:   assoc,
	})
}

func (a *lrAction) mergeShiftContributors(other lrAction) {
	if a == nil || a.kind != lrShift || other.kind != lrShift {
		return
	}
	a.ensureShiftContributors()
	other.ensureShiftContributors()
	for _, contributor := range other.shiftContributors {
		a.addShiftContributor(contributor.lhsSym, contributor.prec, contributor.hasPrec, contributor.assoc)
	}
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
	ActionTable          map[int]map[int][]lrAction
	GotoTable            map[int]map[int]int // [state][nonterminal] → target state
	StateCount           int
	ExtraChainStateStart int // first synthetic nonterminal-extra state, or -1 if none
}

// buildLRTables constructs LR(1) parse tables from a normalized grammar.
func buildLRTables(ng *NormalizedGrammar) (*LRTables, error) {
	tables, _, err := buildLRTablesInternal(context.Background(), ng, false)
	return tables, err
}

// buildLRTablesWithProvenance constructs LR(1) parse tables and returns
// the merge provenance alongside the tables for diagnostic use.
func buildLRTablesWithProvenance(ng *NormalizedGrammar) (*LRTables, *lrContext, error) {
	return buildLRTablesInternal(context.Background(), ng, true)
}

func buildLRTablesInternal(bgCtx context.Context, ng *NormalizedGrammar, trackProvenance bool) (*LRTables, *lrContext, error) {
	newCtx := func() *lrContext {
		ctx := &lrContext{
			bgCtx:           bgCtx,
			ng:              ng,
			firstSets:       make([]bitset, len(ng.Symbols)),
			nullables:       make([]bool, len(ng.Symbols)),
			prodsByLHS:      make(map[int][]int),
			betaCache:       make(map[uint32]*betaResult),
			trackProvenance: trackProvenance,
		}
		if v := os.Getenv("GOT_LALR_LR0_STATE_BUDGET"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				ctx.lalrLR0StateBudget = n
			}
		}
		if v := os.Getenv("GOT_LALR_LR0_CORE_BUDGET"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				ctx.lalrLR0CoreBudget = n
			}
		}
		if trackProvenance && os.Getenv("GOT_DEBUG_LALR_LOOKAHEADS") == "1" {
			ctx.trackLookaheadContributors = true
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
		ctx.templateDefinitionCarrierLHS = make([]bool, len(ng.Symbols))
		for _, sym := range ng.ExternalSymbols {
			if sym >= 0 && sym < tokenCount {
				ctx.boundaryLookaheads.add(sym)
			}
		}
		// JS/TS-style grammars rely on an external automatic-semicolon token and
		// frequently need to distinguish declaration-complete states that are
		// immediately followed by a block-closing brace. Preserving `}` as an
		// additional boundary lookahead keeps those states from collapsing under
		// large-grammar core merging.
		hasAutomaticSemicolon := false
		closeBraceSym := -1
		for sym := 0; sym < tokenCount; sym++ {
			switch ng.Symbols[sym].Name {
			case "_automatic_semicolon":
				hasAutomaticSemicolon = true
			case "}":
				closeBraceSym = sym
			}
		}
		if hasAutomaticSemicolon && closeBraceSym >= 0 {
			ctx.boundaryLookaheads.add(closeBraceSym)
		}
		ctx.annotationAtSym = -1
		ctx.annotationDefSym = -1
		ctx.annotationOpenParenSym = -1
		ctx.annotationCloseParenSym = -1
		ctx.bracedTemplateBodySym = -1
		ctx.bracedTemplateBody1Sym = -1
		ctx.bracedTemplateBody2Sym = -1
		ctx.operatorIdentSym = -1
		ctx.operatorStarSym = -1
		ctx.nonNullLiteralSym = -1
		ctx.conditionalTypeSym = -1
		ctx.conditionalTypeExternalQmarkSym = -1
		ctx.conditionalTypeExtendsSym = -1
		ctx.conditionalTypePlainQmarkSym = -1
		ctx.annotationArgCarrierLHS = make([]bool, len(ng.Symbols))
		ctx.repeatWrapperLHS = make([]bool, len(ng.Symbols))
		ctx.conditionalTypeCarrierLHS = make([]bool, len(ng.Symbols))
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
		templateDefinitionCarrierNames := map[string]bool{
			"annotation":               true,
			"_block":                   true,
			"template_body":            true,
			"_indented_template_body":  true,
			"_braced_template_body":    true,
			"_braced_template_body1":   true,
			"_braced_template_body2":   true,
			"with_template_body":       true,
			"_extension_template_body": true,
			"class_definition":         true,
			"_class_definition":        true,
			"_class_constructor":       true,
			"object_definition":        true,
			"trait_definition":         true,
			"enum_definition":          true,
			"given_definition":         true,
			"extension_definition":     true,
			"function_definition":      true,
			"function_declaration":     true,
			"_function_declaration":    true,
			"_function_constructor":    true,
			"parameters":               true,
			"parameter":                true,
			"class_parameters":         true,
			"class_parameter":          true,
			"val_definition":           true,
			"val_declaration":          true,
			"_start_val":               true,
			"var_definition":           true,
			"var_declaration":          true,
			"_start_var":               true,
			"type_definition":          true,
		}
		conditionalTypeCarrierNames := map[string]bool{
			"type":                   true,
			"primary_type":           true,
			"conditional_type":       true,
			"function_type":          true,
			"readonly_type":          true,
			"constructor_type":       true,
			"infer_type":             true,
			"parenthesized_type":     true,
			"predefined_type":        true,
			"generic_type":           true,
			"object_type":            true,
			"array_type":             true,
			"tuple_type":             true,
			"flow_maybe_type":        true,
			"type_query":             true,
			"index_type_query":       true,
			"existential_type":       true,
			"literal_type":           true,
			"lookup_type":            true,
			"template_literal_type":  true,
			"intersection_type":      true,
			"union_type":             true,
			"type_arguments":         true,
			"nested_type_identifier": true,
			"identifier":             true,
			"member_expression":      true,
			"call_expression":        true,
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
			case "_braced_template_body":
				ctx.bracedTemplateBodySym = i
			case "_braced_template_body1":
				ctx.bracedTemplateBody1Sym = i
			case "_braced_template_body2":
				ctx.bracedTemplateBody2Sym = i
			case "operator_identifier":
				ctx.operatorIdentSym = i
			case "*":
				ctx.operatorStarSym = i
			case "_non_null_literal":
				ctx.nonNullLiteralSym = i
			case "conditional_type":
				ctx.conditionalTypeSym = i
			case "?":
				ctx.conditionalTypeExternalQmarkSym = i
			case "extends":
				ctx.conditionalTypeExtendsSym = i
			case "\\?":
				ctx.conditionalTypePlainQmarkSym = i
			}
			if annotationArgCarrierNames[sym.Name] {
				ctx.annotationArgCarrierLHS[i] = true
			}
			if templateDefinitionCarrierNames[sym.Name] {
				ctx.templateDefinitionCarrierLHS[i] = true
			}
			if sym.GeneratedRepeatAux {
				ctx.repeatWrapperLHS[i] = true
			}
			if conditionalTypeCarrierNames[sym.Name] {
				ctx.conditionalTypeCarrierLHS[i] = true
			}
		}
		expandTemplateDefinitionCarriers(ng, ctx.templateDefinitionCarrierLHS, tokenCount)
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
		return ctx
	}
	ctx := newCtx()
	tokenCount := ctx.tokenCount

	// Build item sets. Use DeRemer/Pennello LALR for large grammars (>400 productions)
	// which would otherwise be slow with the iterative LR(1) construction.
	// Extended merging produces more precise states for mid-size grammars (100-400
	// productions) and is kept for those since some grammars (e.g. HCL) regress
	// significantly with LALR merging.
	var itemSets []lrItemSet
	// External-scanner grammars are much more sensitive to predecessor context
	// than the pure LR(0)+lookahead-propagation path captures. Route all of them
	// through the more precise core-based builder so we can preserve a canonical
	// LR(1) prefix before any compaction starts.
	usePreciseExternalBuilder := len(ng.ExternalSymbols) > 0
	if len(ng.ExternalSymbols) >= 24 && !ng.PreferPreciseExternalLexStates {
		usePreciseExternalBuilder = false
	}
	// Very large grammars (>5000 productions) are intractable for the LR(1)
	// builder even with the precise-state budget: each BFS state expands
	// hundreds of core items through closureToSet, and reaching the 20K
	// budget limit takes minutes. Route them directly to LALR.
	if len(ng.Productions) > 5000 {
		usePreciseExternalBuilder = false
	}
	if os.Getenv("GOT_LR_FORCE_EXTERNAL_LALR") == "1" {
		usePreciseExternalBuilder = false
	}
	if os.Getenv("GOT_LR_FORCE_PRECISE_EXTERNAL") == "1" {
		usePreciseExternalBuilder = len(ng.ExternalSymbols) > 0
	}
	if len(ng.Productions) > 400 && !usePreciseExternalBuilder {
		itemSets = ctx.buildItemSetsLALR()
	} else {
		itemSets = ctx.buildItemSets()
		const maxRuntimeStateID = int(^uint16(0))
		if usePreciseExternalBuilder && (ctx.preciseStateBudgetExceeded || len(itemSets) > maxRuntimeStateID) {
			ctx = newCtx()
			itemSets = ctx.buildItemSetsLALR()
		}
	}
	if ctx.lalrLR0StateBudgetExceeded {
		return nil, ctx, fmt.Errorf("build LR tables: LALR LR0 state budget exceeded (%d states > budget %d, core entries=%d)",
			len(ctx.lalrLR0ItemSets), ctx.lalrLR0StateBudget, ctx.lalrLR0CoreEntries)
	}
	if ctx.lalrLR0CoreBudgetExceeded {
		return nil, ctx, fmt.Errorf("build LR tables: LALR LR0 core budget exceeded (%d core entries > budget %d, states=%d)",
			ctx.lalrLR0CoreEntries, ctx.lalrLR0CoreBudget, len(ctx.lalrLR0ItemSets))
	}
	// Check for context cancellation after item set construction. If the
	// context was cancelled mid-build, return immediately so the goroutine
	// can release LR builder memory.
	if err := bgCtx.Err(); err != nil {
		return nil, ctx, fmt.Errorf("build LR tables: %w", err)
	}

	// StateID is uint32 in the runtime (expanded from uint16 to support large
	// grammars like COBOL with 67K states). Cap at uint32 max.
	const maxRuntimeStateID = int(^uint32(0))
	if len(itemSets) > maxRuntimeStateID {
		return nil, ctx, fmt.Errorf("parser state count %d exceeds max representable state id %d", len(itemSets), maxRuntimeStateID)
	}

	// Build action and goto tables.
	tables := &LRTables{
		ActionTable:          make(map[int]map[int][]lrAction),
		GotoTable:            make(map[int]map[int]int),
		StateCount:           len(itemSets),
		ExtraChainStateStart: -1,
	}

	for stateIdx, itemSet := range itemSets {
		tables.ActionTable[stateIdx] = make(map[int][]lrAction)
		tables.GotoTable[stateIdx] = make(map[int]int)

		for _, ce := range itemSet.cores {
			prod := &ng.Productions[int(ce.prodIdx)]

			if int(ce.dot) < len(prod.RHS) {
				// Dot not at end → shift or goto
				nextSym := prod.RHS[ce.dot]
				targetState, ok := ctx.transitionTarget(stateIdx, nextSym)
				if !ok {
					continue
				}

				if nextSym < tokenCount {
					// Terminal → shift action.
					// For closure-derived items (dot == 0), suppress the production's
					// own precedence. Tree-sitter's conflict resolver only considers
					// shift precedence from items whose dot has advanced past position 0
					// (step_index > 0). Without this, a high-precedence closure item
					// (e.g. unary_expression prec=14 within sizeof's operand) can
					// dominate the shift's precedence and incorrectly win S/R conflicts
					// against the enclosing reduce (e.g. sizeof_expression prec=13).
					// The enclosing kernel item's precedence is propagated afterward
					// by propagateEntryShiftMetadata.
					shiftPrec := prod.Prec
					shiftAssoc := prod.Assoc
					if ce.dot == 0 {
						shiftPrec = 0
						shiftAssoc = AssocNone
					}
					repeatLHSs := ctx.repetitionShiftHelperLHSSyms(stateIdx, nextSym, targetState)
					action := lrAction{
						kind:    lrShift,
						state:   targetState,
						prec:    shiftPrec,
						hasPrec: prod.HasExplicitPrec,
						assoc:   shiftAssoc,
						lhsSym:  prod.LHS,
						isExtra: prod.IsExtra,
					}
					for _, lhs := range repeatLHSs {
						action.addRepeatLHS(lhs)
					}
					tables.addAction(stateIdx, nextSym, action)
				} else {
					// Nonterminal → goto
					tables.GotoTable[stateIdx][nextSym] = targetState
				}
			} else {
				// Dot at end → reduce or accept
				if int(ce.prodIdx) == ng.AugmentProdID {
					// Augmented start production → accept
					tables.addAction(stateIdx, 0, lrAction{kind: lrAccept})
				} else {
					// Regular reduce — one action per lookahead terminal.
					ce.lookaheads.forEach(func(la int) {
						tables.addAction(stateIdx, la, lrAction{
							kind:    lrReduce,
							prodIdx: int(ce.prodIdx),
							prec:    prod.Prec,
							hasPrec: prod.HasExplicitPrec,
							assoc:   prod.Assoc,
							lhsSym:  prod.LHS,
							isExtra: prod.IsExtra,
						})
					})
				}
			}
		}
	}
	propagateEntryShiftMetadata(tables, itemSets, ctx, ng)
	augmentSingleReduceLookaheadsForNonterminalExtraStarts(tables, ng, ctx)

	return tables, ctx, nil
}

// propagateEntryShiftMetadata preserves the precedence/associativity of an
// enclosing production when a conflict-relevant terminal shift comes from the
// immediately-entered nonterminal at the dot. Without this, conflicts like
// call-vs-unary can see the shift side as the precedence of the entry rule
// (for example argument_list) instead of the higher-precedence enclosing rule
// (for example call_expression).
func propagateEntryShiftMetadata(tables *LRTables, itemSets []lrItemSet, ctx *lrContext, ng *NormalizedGrammar) {
	if tables == nil || ctx == nil {
		return
	}
	tokenCount := ctx.tokenCount
	leadingCache := make(map[int]map[int]bool)
	for stateIdx, itemSet := range itemSets {
		for _, ce := range itemSet.cores {
			prod := &ng.Productions[int(ce.prodIdx)]
			if int(ce.dot) >= len(prod.RHS) {
				continue
			}
			nextSym := prod.RHS[ce.dot]
			if nextSym < tokenCount {
				continue
			}

			ctx.firstSets[nextSym].forEach(func(la int) {
				acts := tables.ActionTable[stateIdx][la]
				leading := leadingNonterminalsFrom(nextSym, tokenCount, ng, ctx, leadingCache, ce.dot > 0)
				for _, act := range acts {
					if act.kind != lrShift || !shiftMatchesEntrySymbol(act, nextSym, leading) {
						continue
					}
					tables.addAction(stateIdx, la, lrAction{
						kind:          lrShift,
						state:         act.state,
						prec:          prod.Prec,
						hasPrec:       prod.HasExplicitPrec,
						assoc:         prod.Assoc,
						lhsSym:        prod.LHS,
						isExtra:       prod.IsExtra,
						repeat:        act.repeat,
						repeatLHS:     act.repeatLHS,
						repeatLHSSyms: append([]int(nil), act.repeatLHSSyms...),
					})
				}
			})
		}
	}
}

func shiftMatchesEntrySymbol(act lrAction, sym int, leading map[int]bool) bool {
	if shiftLHSMatchesEntry(act.lhsSym, sym, leading) {
		return true
	}
	for _, lhs := range act.lhsSyms {
		if shiftLHSMatchesEntry(lhs, sym, leading) {
			return true
		}
	}
	return false
}

func shiftLHSMatchesEntry(lhs, sym int, leading map[int]bool) bool {
	if lhs == sym {
		return true
	}
	return leading != nil && leading[lhs]
}

// leadingNonterminalsFrom returns nonterminals whose leading terminal shifts can
// stand in for sym during entry-shift metadata propagation. Metadata can cross
// generated repeat helpers and pure unary transparent wrappers. After a prefix,
// it can also cross a pure one-symbol choice wrapper or a visible suffix wrapper
// whose first symbol is the shifted nonterminal, so suffix alternatives keep the
// enclosing production's precedence without opening arbitrary continuations.
// Multi-symbol leading-edge propagation remains limited to generated repeat
// helpers; hidden ordinary wrappers with required suffixes are not transparent.
func leadingNonterminalsFrom(sym, tokenCount int, ng *NormalizedGrammar, ctx *lrContext, cache map[int]map[int]bool, includeContinuations bool) map[int]bool {
	if sym < tokenCount || sym < 0 || sym >= len(ng.Symbols) {
		return nil
	}
	cacheKey := sym
	if includeContinuations {
		cacheKey = -sym - 1
	}
	if cached, ok := cache[cacheKey]; ok {
		return cached
	}
	seen := make(map[int]bool)
	var walk func(int)
	walk = func(cur int) {
		if cur < tokenCount || cur < 0 || cur >= len(ng.Symbols) || seen[cur] {
			return
		}
		seen[cur] = true
		if !entryShiftMetadataTransparentWrapper(cur, ng) {
			if includeContinuations && entryShiftMetadataPostPrefixChoiceWrapper(cur, tokenCount, ng, ctx) {
				for _, prodIdx := range ctx.prodsByLHS[cur] {
					prod := &ng.Productions[prodIdx]
					walk(prod.RHS[0])
				}
			} else if includeContinuations && entryShiftMetadataPostPrefixVisibleLeadingWrapper(cur, tokenCount, ng, ctx) {
				for _, prodIdx := range ctx.prodsByLHS[cur] {
					prod := &ng.Productions[prodIdx]
					walk(prod.RHS[0])
				}
			}
			return
		}
		for _, prodIdx := range ctx.prodsByLHS[cur] {
			prod := &ng.Productions[prodIdx]
			if includeContinuations && ng.Symbols[cur].GeneratedRepeatAux {
				walkLeadingEdge(prod.RHS, tokenCount, ctx, walk)
			} else if len(prod.RHS) == 1 {
				rhs := prod.RHS[0]
				if rhs >= tokenCount {
					walk(rhs)
				}
			}
		}
	}
	walk(sym)
	cache[cacheKey] = seen
	return seen
}

func entryShiftMetadataTransparentWrapper(sym int, ng *NormalizedGrammar) bool {
	if ng == nil || sym < 0 || sym >= len(ng.Symbols) {
		return false
	}
	info := ng.Symbols[sym]
	return info.GeneratedRepeatAux || info.Supertype || (!info.Visible && !info.Named)
}

func entryShiftMetadataPostPrefixChoiceWrapper(sym, tokenCount int, ng *NormalizedGrammar, ctx *lrContext) bool {
	if ng == nil || ctx == nil || sym < tokenCount || sym < 0 || sym >= len(ng.Symbols) {
		return false
	}
	prods := ctx.prodsByLHS[sym]
	if len(prods) == 0 {
		return false
	}
	for _, prodIdx := range prods {
		if prodIdx < 0 || prodIdx >= len(ng.Productions) {
			return false
		}
		prod := &ng.Productions[prodIdx]
		if len(prod.RHS) != 1 || prod.RHS[0] < tokenCount {
			return false
		}
	}
	return true
}

func entryShiftMetadataPostPrefixVisibleLeadingWrapper(sym, tokenCount int, ng *NormalizedGrammar, ctx *lrContext) bool {
	if ng == nil || ctx == nil || sym < tokenCount || sym < 0 || sym >= len(ng.Symbols) {
		return false
	}
	info := ng.Symbols[sym]
	if !info.Visible && !info.Named && !info.Supertype {
		return false
	}
	prods := ctx.prodsByLHS[sym]
	if len(prods) == 0 {
		return false
	}
	for _, prodIdx := range prods {
		if prodIdx < 0 || prodIdx >= len(ng.Productions) {
			return false
		}
		prod := &ng.Productions[prodIdx]
		if len(prod.RHS) == 0 || prod.RHS[0] < tokenCount {
			return false
		}
	}
	return true
}

func walkLeadingEdge(rhs []int, tokenCount int, ctx *lrContext, walk func(int)) {
	for _, sym := range rhs {
		if sym >= tokenCount {
			walk(sym)
		}
		if sym < 0 || sym >= len(ctx.nullables) || !ctx.nullables[sym] {
			return
		}
	}
}

func (t *LRTables) addAction(state, sym int, action lrAction) {
	if action.kind == lrShift {
		action.ensureShiftContributors()
	}
	existing := t.ActionTable[state][sym]
	// Avoid duplicates.
	for i, a := range existing {
		if a.kind == action.kind && a.state == action.state {
			if a.kind == lrShift {
				// For shifts to the same target, keep the higher scalar precedence
				// for legacy callers, but retain per-LHS contributors so conflict
				// resolution can compare the reduce against the local shift source.
				if !a.isExtra && action.isExtra {
					return // existing non-extra wins
				}
				if a.isExtra && !action.isExtra {
					existing[i].isExtra = false
				}
				existing[i].addRepeatLHSFrom(action)
				existing[i].mergeShiftContributors(action)
				if action.prec > a.prec {
					existing[i].prec = action.prec
					existing[i].hasPrec = action.hasPrec
					existing[i].assoc = action.assoc
				} else if action.prec == a.prec && action.hasPrec && !existing[i].hasPrec {
					existing[i].hasPrec = true
					if existing[i].assoc == AssocNone {
						existing[i].assoc = action.assoc
					}
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
	bgCtx      context.Context // cancellation context for long-running LR builds
	ng         *NormalizedGrammar
	tokenCount int
	firstSets  []bitset // symbol → bitset of terminal first symbols
	nullables  []bool   // symbol → can derive ε

	// Production index: LHS symbol → production indices
	prodsByLHS map[int][]int

	// FIRST(β) cache: packed (prodIdx, dot) → first set + nullable flag
	betaCache map[uint32]*betaResult

	// Item set management
	itemSets        []lrItemSet
	lalrLR0ItemSets []lr0ItemSet
	transitions     []lrTransitionRow
	// LALR transition follow sets are retained so local LR(1) splitting can
	// reconstruct nonterminal predecessor partitions with meaningful lookaheads
	// instead of the empty LR(0) kernels emitted by DeRemer/Pennello.
	lalrFollowByTransition map[[2]int]bitset
	lalrNTTransitions      []ntTransition
	// Retained DeRemer/Pennello LALR data for use by the LR(1) splitter.
	// Only populated when trackProvenance is true.
	lalrLookbacks    []lookbackEntry
	lalrFollowSets   []bitset
	lalrNTTransIndex map[[2]int]int

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
	// needCompletionLAHash is true only when buildItemSets is using extended
	// merging. Boundary-only and pure-core paths do not read completionLAHash.
	needCompletionLAHash bool
	// Narrow annotation-argument tagging metadata. These are precomputed once so
	// buildItemSets can cheaply preserve declaration-family context only while a
	// state remains inside annotation arguments.
	annotationAtSym                 int
	annotationDefSym                int
	annotationOpenParenSym          int
	annotationCloseParenSym         int
	bracedTemplateBodySym           int
	bracedTemplateBody1Sym          int
	bracedTemplateBody2Sym          int
	annotationArgCarrierLHS         []bool
	templateDefinitionCarrierLHS    []bool
	repeatWrapperLHS                []bool
	operatorIdentSym                int
	operatorStarSym                 int
	nonNullLiteralSym               int
	conditionalTypeSym              int
	conditionalTypeExternalQmarkSym int
	conditionalTypeExtendsSym       int
	conditionalTypePlainQmarkSym    int
	conditionalTypeCarrierLHS       []bool

	// Reusable closure queue scratch keeps closureToSet/closureIncremental from
	// reallocating worklists and in-queue tracking on every item-set build.
	closureWorklist  []int
	closureQueuedGen []uint32
	closureQueueGen  uint32

	// GOTO scratch reuses transient symbol and advanced-kernel slices while
	// building successor states.
	gotoSymbolsScratch     []int
	gotoAdvancedScratch    []coreEntry
	lr0KernelScratch       []coreItem
	lr0ClosureScratch      []lr0CoreEntry
	lr0Dot0ClosureSeeds    [][]int
	lr0RetainedChunks      [][]lr0CoreEntry
	lr0RetainedChunkUsed   int
	lr0SymbolBucketIdx     []int
	lr0SymbolBucketCount   []int
	lr0SymbolBucketOffset  []int
	lr0TargetRepeatWrapper []int
	lr0SymbolSeenGen       []uint32
	lr0SymbolSeenEpoch     uint32
	lr0RepeatSourceGen     []uint32
	lr0RepeatSourceEpoch   uint32

	// Lookahead bitset scratch reuses word buffers for temporary closed sets that
	// are discarded after exact-match or merge lookups.
	lookaheadWordCount int
	lookaheadWordPool  [][]uint64
	maxLookaheadPool   int

	repeatWrapperStateSymSymsCache map[uint64][]int

	// preciseStateBudgetExceeded marks that the precise external-grammar LR(1)
	// builder crossed its configured state budget and should be retried via the
	// cheaper LALR path.
	preciseStateBudgetExceeded bool
	lalrLR0StateBudget         int
	lalrLR0CoreBudget          int
	lalrLR0StateBudgetExceeded bool
	lalrLR0CoreBudgetExceeded  bool
	lalrLR0CoreEntries         int
}

// conflictResolutionCache stores grammar-wide declared-conflict metadata that
// would otherwise be rebuilt for every single resolveActionConflict call.
type conflictResolutionCache struct {
	groups         [][]int
	groupsBySymbol [][]int
	prodsByLHS     [][]int
	nullable       []bool
	rhsParents     [][]int
	auxParents     [][]int
	auxComputed    []bool
	auxVisiting    []bool

	firstSets                    [][]uint64
	repeatStartLookaheadSets     [][]uint64
	repeatStartLookaheadComputed bool
	structuralRepeatHelperMemo   []int8

	shiftReduceConflictGroupMemo map[string]bool
	reduceLHSConflictGroupMemo   map[int]bool
	structuralStats              conflictResolutionStats
}

func (cache *conflictResolutionCache) resetStructuralStats() {
	if cache != nil {
		cache.structuralStats = conflictResolutionStats{}
	}
}

func (cache *conflictResolutionCache) snapshotStructuralStats() conflictResolutionStats {
	if cache == nil {
		return conflictResolutionStats{}
	}
	return cache.structuralStats
}

func getConflictResolutionCache(ng *NormalizedGrammar) *conflictResolutionCache {
	if ng == nil {
		return nil
	}
	if ng.conflictCache != nil {
		return ng.conflictCache
	}

	cache := &conflictResolutionCache{
		groups:                     make([][]int, len(ng.Conflicts)),
		groupsBySymbol:             make([][]int, len(ng.Symbols)),
		prodsByLHS:                 make([][]int, len(ng.Symbols)),
		nullable:                   make([]bool, len(ng.Symbols)),
		rhsParents:                 make([][]int, len(ng.Symbols)),
		auxParents:                 make([][]int, len(ng.Symbols)),
		auxComputed:                make([]bool, len(ng.Symbols)),
		auxVisiting:                make([]bool, len(ng.Symbols)),
		structuralRepeatHelperMemo: make([]int8, len(ng.Symbols)),
	}

	for groupIdx, group := range ng.Conflicts {
		cache.groups[groupIdx] = append([]int(nil), group...)
		for _, sym := range group {
			if sym >= 0 && sym < len(cache.groupsBySymbol) {
				cache.groupsBySymbol[sym] = append(cache.groupsBySymbol[sym], groupIdx)
			}
		}
	}
	for prodIdx, prod := range ng.Productions {
		if prod.LHS >= 0 && prod.LHS < len(cache.prodsByLHS) {
			cache.prodsByLHS[prod.LHS] = append(cache.prodsByLHS[prod.LHS], prodIdx)
		}
		for _, sym := range prod.RHS {
			if sym >= 0 && sym < len(cache.rhsParents) {
				cache.rhsParents[sym] = append(cache.rhsParents[sym], prod.LHS)
			}
		}
	}
	computeNullableSymbolsForConflictCache(ng, cache)

	ng.conflictCache = cache
	return cache
}

func computeNullableSymbolsForConflictCache(ng *NormalizedGrammar, cache *conflictResolutionCache) {
	if ng == nil || cache == nil {
		return
	}
	changed := true
	for changed {
		changed = false
		for i := range ng.Productions {
			prod := &ng.Productions[i]
			if prod.LHS < 0 || prod.LHS >= len(cache.nullable) || cache.nullable[prod.LHS] {
				continue
			}
			allNullable := true
			for _, sym := range prod.RHS {
				if sym < 0 || sym >= len(cache.nullable) || !cache.nullable[sym] {
					allNullable = false
					break
				}
			}
			if allNullable {
				cache.nullable[prod.LHS] = true
				changed = true
			}
		}
	}
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
	ctx.lalrLR0ItemSets = nil
	ctx.transitions = nil
	ctx.provenance = nil
	ctx.dot0Index = nil
	ctx.dot0Dirty = nil
	ctx.extraProdIndices = nil
	ctx.allTerminals = bitset{}
	ctx.boundaryLookaheads = bitset{}
	ctx.gotoSymbolsScratch = nil
	ctx.gotoAdvancedScratch = nil
	ctx.lr0KernelScratch = nil
	ctx.lr0ClosureScratch = nil
	ctx.lr0Dot0ClosureSeeds = nil
	ctx.lr0RetainedChunks = nil
	ctx.lr0RetainedChunkUsed = 0
	ctx.lr0SymbolBucketIdx = nil
	ctx.lr0SymbolBucketCount = nil
	ctx.lr0SymbolBucketOffset = nil
	ctx.lr0TargetRepeatWrapper = nil
	ctx.lr0SymbolSeenGen = nil
	ctx.lr0SymbolSeenEpoch = 0
	ctx.lr0RepeatSourceGen = nil
	ctx.lr0RepeatSourceEpoch = 0
	ctx.lookaheadWordPool = nil
	ctx.repeatWrapperStateSymSymsCache = nil
	ctx.lalrNTTransitions = nil
}

func (ctx *lrContext) nextLR0SymbolSeenEpoch() uint32 {
	ctx.lr0SymbolSeenEpoch++
	if ctx.lr0SymbolSeenEpoch == 0 {
		for i := range ctx.lr0SymbolSeenGen {
			ctx.lr0SymbolSeenGen[i] = 0
		}
		ctx.lr0SymbolSeenEpoch = 1
	}
	return ctx.lr0SymbolSeenEpoch
}

func (ctx *lrContext) ensureLR0SymbolSeenCapacity(size int) {
	if size <= len(ctx.lr0SymbolSeenGen) {
		return
	}
	ctx.lr0SymbolSeenGen = append(ctx.lr0SymbolSeenGen, make([]uint32, size-len(ctx.lr0SymbolSeenGen))...)
}

func (ctx *lrContext) ensureLR0SymbolBucketCapacity(size int) {
	if size > len(ctx.lr0SymbolBucketIdx) {
		ctx.lr0SymbolBucketIdx = append(ctx.lr0SymbolBucketIdx, make([]int, size-len(ctx.lr0SymbolBucketIdx))...)
	}
	if size > len(ctx.lr0SymbolBucketCount) {
		ctx.lr0SymbolBucketCount = append(ctx.lr0SymbolBucketCount, make([]int, size-len(ctx.lr0SymbolBucketCount))...)
	}
	if size > len(ctx.lr0SymbolBucketOffset) {
		ctx.lr0SymbolBucketOffset = append(ctx.lr0SymbolBucketOffset, make([]int, size-len(ctx.lr0SymbolBucketOffset))...)
	}
	if size > len(ctx.lr0TargetRepeatWrapper) {
		ctx.lr0TargetRepeatWrapper = append(ctx.lr0TargetRepeatWrapper, make([]int, size-len(ctx.lr0TargetRepeatWrapper))...)
	}
}

func (ctx *lrContext) nextLR0RepeatSourceEpoch() uint32 {
	ctx.lr0RepeatSourceEpoch++
	if ctx.lr0RepeatSourceEpoch == 0 {
		for i := range ctx.lr0RepeatSourceGen {
			ctx.lr0RepeatSourceGen[i] = 0
		}
		ctx.lr0RepeatSourceEpoch = 1
	}
	return ctx.lr0RepeatSourceEpoch
}

func (ctx *lrContext) ensureLR0RepeatSourceCapacity(size int) {
	if size <= len(ctx.lr0RepeatSourceGen) {
		return
	}
	ctx.lr0RepeatSourceGen = append(ctx.lr0RepeatSourceGen, make([]uint32, size-len(ctx.lr0RepeatSourceGen))...)
}

const defaultLR0RetainedChunkEntries = 1 << 20

func (ctx *lrContext) retainLR0Cores(cores []lr0CoreEntry) []lr0CoreEntry {
	if len(cores) == 0 {
		return nil
	}
	if len(ctx.lr0RetainedChunks) == 0 {
		chunkCap := defaultLR0RetainedChunkEntries
		if len(cores) > chunkCap {
			chunkCap = len(cores)
		}
		ctx.lr0RetainedChunks = append(ctx.lr0RetainedChunks, make([]lr0CoreEntry, chunkCap))
		ctx.lr0RetainedChunkUsed = 0
	}
	chunk := ctx.lr0RetainedChunks[len(ctx.lr0RetainedChunks)-1]
	if len(chunk)-ctx.lr0RetainedChunkUsed < len(cores) {
		chunkCap := defaultLR0RetainedChunkEntries
		if len(cores) > chunkCap {
			chunkCap = len(cores)
		}
		chunk = make([]lr0CoreEntry, chunkCap)
		ctx.lr0RetainedChunks = append(ctx.lr0RetainedChunks, chunk)
		ctx.lr0RetainedChunkUsed = 0
	}
	start := ctx.lr0RetainedChunkUsed
	end := start + len(cores)
	copy(chunk[start:end], cores)
	ctx.lr0RetainedChunkUsed = end
	return chunk[start:end:end]
}

func (ctx *lrContext) ensureTransitionState(state int) {
	if state < len(ctx.transitions) {
		return
	}
	ctx.transitions = append(ctx.transitions, make([]lrTransitionRow, state-len(ctx.transitions)+1)...)
}

func (ctx *lrContext) transitionRow(state int) lrTransitionRow {
	if state < 0 || state >= len(ctx.transitions) {
		return nil
	}
	return ctx.transitions[state]
}

func (ctx *lrContext) addTransition(state, sym, target int) {
	ctx.ensureTransitionState(state)
	for i := range ctx.transitions[state] {
		if int(ctx.transitions[state][i].sym) == sym {
			ctx.transitions[state][i].target = uint32(target)
			return
		}
	}
	ctx.transitions[state] = append(ctx.transitions[state], lrTransition{
		sym:    uint32(sym),
		target: uint32(target),
	})
}

func (ctx *lrContext) sortStateTransitions(state int) {
	if state < 0 || state >= len(ctx.transitions) || len(ctx.transitions[state]) < 2 {
		return
	}
	row := ctx.transitions[state]
	sort.Slice(row, func(i, j int) bool {
		return row[i].sym < row[j].sym
	})
}

func (ctx *lrContext) transitionTarget(state, sym int) (int, bool) {
	row := ctx.transitionRow(state)
	if len(row) == 0 {
		return 0, false
	}
	want := uint32(sym)
	idx := sort.Search(len(row), func(i int) bool {
		return row[i].sym >= want
	})
	if idx < len(row) && row[idx].sym == want {
		return int(row[idx].target), true
	}
	return 0, false
}

func (ctx *lrContext) ensureLookaheadBitsetConfig() {
	if ctx.lookaheadWordCount == 0 {
		ctx.lookaheadWordCount = (ctx.tokenCount + 63) / 64
		if ctx.lookaheadWordCount == 0 {
			ctx.lookaheadWordCount = 1
		}
	}
	if ctx.maxLookaheadPool == 0 {
		ctx.maxLookaheadPool = 64
		if ctx.ng != nil && len(ctx.ng.Productions) > ctx.maxLookaheadPool {
			ctx.maxLookaheadPool = len(ctx.ng.Productions)
		}
	}
}

func (ctx *lrContext) allocLookaheadBitset() bitset {
	ctx.ensureLookaheadBitsetConfig()
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
	ctx.ensureLookaheadBitsetConfig()
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

func (ctx *lrContext) ensureRepeatWrapperLHS() {
	if ctx == nil || ctx.ng == nil {
		return
	}
	if len(ctx.repeatWrapperLHS) == len(ctx.ng.Symbols) {
		return
	}
	ctx.repeatWrapperLHS = make([]bool, len(ctx.ng.Symbols))
	for i, sym := range ctx.ng.Symbols {
		if sym.GeneratedRepeatAux {
			ctx.repeatWrapperLHS[i] = true
		}
	}
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

type terminalStartMatcher struct {
	any   bool
	runes map[rune]struct{}
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
	// Synthetic states for nonterminal extras model the interior of that extra
	// production. Do not inject the grammar's terminal extras here: allowing
	// unrelated extras mid-chain lets zero-width/layout extras interrupt
	// constructs like block comments immediately after their opener.
	_ = stateIdx
}

func (b *extraChainBuilder) mergeSyntheticTerminalShift(stateIdx, sym int, action lrAction) {
	acts := b.tables.ActionTable[stateIdx][sym]
	mergedTarget := action.state
	mergeIdx := -1
	for i, act := range acts {
		if act.kind != lrShift || !act.isExtra || act.lhsSym != action.lhsSym {
			continue
		}
		if act.state == action.state {
			acts[i].addRepeatLHSFrom(action)
			b.tables.ActionTable[stateIdx][sym] = acts
			return
		}
		if act.state >= b.syntheticStart && action.state >= b.syntheticStart {
			mergedTarget = b.unionSyntheticStates(act.state, mergedTarget)
			if mergeIdx < 0 {
				mergeIdx = i
			}
		}
	}
	if mergeIdx >= 0 {
		acts[mergeIdx].state = mergedTarget
		acts[mergeIdx].addRepeatLHSFrom(action)
		b.tables.ActionTable[stateIdx][sym] = acts
		return
	}
	b.tables.addAction(stateIdx, sym, action)
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
			syms := make([]int, 0, len(srcActions))
			for sym := range srcActions {
				syms = append(syms, sym)
			}
			sort.Ints(syms)
			for _, sym := range syms {
				for _, act := range srcActions[sym] {
					if act.kind == lrShift && act.isExtra && sym < b.tokenCount {
						b.mergeSyntheticTerminalShift(stateIdx, sym, act)
						continue
					}
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
				prec:    prod.Prec,
				hasPrec: prod.HasExplicitPrec,
				assoc:   prod.Assoc,
				lhsSym:  prod.LHS,
				isExtra: prod.IsExtra,
			})
		})
		return
	}

	nextSym := prod.RHS[pos]
	if nextSym < b.tokenCount {
		targetState := b.buildProdChain(prodIdx, pos+1, follow)
		repeatLHSs := b.ctx.repetitionShiftHelperLHSSyms(stateIdx, nextSym, targetState)
		action := lrAction{
			kind:    lrShift,
			state:   targetState,
			prec:    prod.Prec,
			hasPrec: prod.HasExplicitPrec,
			assoc:   prod.Assoc,
			lhsSym:  prod.LHS,
			isExtra: false,
		}
		for _, lhs := range repeatLHSs {
			action.addRepeatLHS(lhs)
		}
		b.mergeSyntheticTerminalShift(stateIdx, nextSym, action)
		return
	}

	targetState := b.buildProdChain(prodIdx, pos+1, follow)
	existing, ok := b.tables.GotoTable[stateIdx][nextSym]
	if !ok || existing == targetState {
		b.tables.GotoTable[stateIdx][nextSym] = targetState
	} else if existing >= b.syntheticStart && targetState >= b.syntheticStart {
		b.tables.GotoTable[stateIdx][nextSym] = b.unionSyntheticStates(existing, targetState)
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
					prec:    prod.Prec,
					hasPrec: prod.HasExplicitPrec,
					assoc:   prod.Assoc,
					lhsSym:  prod.LHS,
					isExtra: prod.IsExtra,
				})
			})
			continue
		}

		firstSym := prod.RHS[0]
		if firstSym < b.tokenCount {
			targetState := b.buildProdChain(prodIdx, 1, follow)
			repeatLHSs := b.ctx.repetitionShiftHelperLHSSyms(stateIdx, firstSym, targetState)
			action := lrAction{
				kind:    lrShift,
				state:   targetState,
				prec:    prod.Prec,
				hasPrec: prod.HasExplicitPrec,
				assoc:   prod.Assoc,
				lhsSym:  prod.LHS,
				isExtra: false,
			}
			for _, lhs := range repeatLHSs {
				action.addRepeatLHS(lhs)
			}
			b.mergeSyntheticTerminalShift(stateIdx, firstSym, action)
			continue
		}

		targetState := b.buildProdChain(prodIdx, 1, follow)
		existing, ok := b.tables.GotoTable[stateIdx][firstSym]
		if !ok || existing == targetState {
			b.tables.GotoTable[stateIdx][firstSym] = targetState
		} else if existing >= b.syntheticStart && targetState >= b.syntheticStart {
			b.tables.GotoTable[stateIdx][firstSym] = b.unionSyntheticStates(existing, targetState)
		}
		nextFollow := b.ctx.firstOfSequenceWithFallback(prod.RHS[1:], &follow)
		b.addNonterminalEntries(stateIdx, firstSym, nextFollow)
	}
}

func buildTerminalStartMatchers(patterns []TerminalPattern) map[int]terminalStartMatcher {
	bySym := make(map[int]terminalStartMatcher)
	for _, pat := range patterns {
		matcher := terminalStartMatcherForPattern(pat)
		if existing, ok := bySym[pat.SymbolID]; ok {
			bySym[pat.SymbolID] = mergeTerminalStartMatchers(existing, matcher)
		} else {
			bySym[pat.SymbolID] = matcher
		}
	}
	return bySym
}

func mergeTerminalStartMatchers(a, b terminalStartMatcher) terminalStartMatcher {
	if a.any || b.any {
		return terminalStartMatcher{any: true}
	}
	if len(a.runes) == 0 {
		return b
	}
	if len(b.runes) == 0 {
		return a
	}
	out := terminalStartMatcher{runes: make(map[rune]struct{}, len(a.runes)+len(b.runes))}
	for r := range a.runes {
		out.runes[r] = struct{}{}
	}
	for r := range b.runes {
		out.runes[r] = struct{}{}
	}
	return out
}

func terminalStartMatcherForPattern(p TerminalPattern) terminalStartMatcher {
	if p.Rule == nil {
		return terminalStartMatcher{any: true}
	}
	nfa, err := buildCombinedNFA([]TerminalPattern{p})
	if err != nil || nfa == nil {
		return terminalStartMatcher{any: true}
	}
	startClosure := epsilonClosure(nfa, []int{nfa.start})
	out := terminalStartMatcher{runes: make(map[rune]struct{})}
	const maxExplicitRunes = 64
	for _, s := range startClosure {
		for _, tr := range nfa.states[s].transitions {
			if tr.epsilon {
				continue
			}
			if tr.hi < tr.lo {
				continue
			}
			if tr.hi-tr.lo > maxExplicitRunes || len(out.runes) > maxExplicitRunes {
				return terminalStartMatcher{any: true}
			}
			for r := tr.lo; r <= tr.hi; r++ {
				out.runes[r] = struct{}{}
				if len(out.runes) > maxExplicitRunes {
					return terminalStartMatcher{any: true}
				}
			}
		}
	}
	if len(out.runes) == 0 {
		return terminalStartMatcher{any: true}
	}
	return out
}

func terminalStartMatchersOverlap(a, b terminalStartMatcher) bool {
	if a.any || b.any {
		return true
	}
	if len(a.runes) == 0 || len(b.runes) == 0 {
		return true
	}
	if len(a.runes) > len(b.runes) {
		a, b = b, a
	}
	for r := range a.runes {
		if _, ok := b.runes[r]; ok {
			return true
		}
	}
	return false
}

func terminalStartMatcherHasSingleRune(m terminalStartMatcher, want rune) bool {
	if m.any || len(m.runes) != 1 {
		return false
	}
	_, ok := m.runes[want]
	return ok
}

// augmentSingleReduceLookaheadsForNonterminalExtraStarts lets a completed
// production close before a visible nonterminal extra begins. Tree-sitter extras
// may appear between any two tokens; without this reduce lookahead, a state that
// has just completed an item can shift the extra chain first and later recover
// the completed item as ERROR. Keep this deliberately conservative: only states
// with one non-extra reduce are augmented, and real structural shifts on the
// extra starter still win.
func realStartSymbol(ng *NormalizedGrammar) int {
	if ng == nil || ng.AugmentProdID < 0 || ng.AugmentProdID >= len(ng.Productions) || len(ng.Productions[ng.AugmentProdID].RHS) == 0 {
		return -1
	}
	return ng.Productions[ng.AugmentProdID].RHS[0]
}
func augmentSingleReduceLookaheadsForNonterminalExtraStarts(tables *LRTables, ng *NormalizedGrammar, ctx *lrContext) int {
	if tables == nil || ng == nil || len(ng.ExtraSymbols) == 0 {
		return 0
	}
	tokenCount := ng.TokenCount()
	extraSymbolSet := make(map[int]struct{}, len(ng.ExtraSymbols))
	for _, sym := range ng.ExtraSymbols {
		extraSymbolSet[sym] = struct{}{}
	}
	extraStarts := make(map[int]struct{})
	internalExtraStructuralStarts := make(map[int]struct{})
	for i := range ng.Productions {
		prod := &ng.Productions[i]
		if !prod.IsExtra || len(prod.RHS) == 0 {
			continue
		}
		if _, ok := extraSymbolSet[prod.LHS]; !ok {
			continue
		}
		first := prod.RHS[0]
		if first > 0 && first < tokenCount {
			extraStarts[first] = struct{}{}
		}
		for pos := 1; pos < len(prod.RHS); pos++ {
			sym := prod.RHS[pos]
			if sym > 0 && sym < tokenCount {
				internalExtraStructuralStarts[sym] = struct{}{}
				continue
			}
			if ctx != nil && sym >= 0 && sym < len(ctx.firstSets) {
				ctx.firstSets[sym].forEach(func(first int) {
					if first > 0 && first < tokenCount {
						internalExtraStructuralStarts[first] = struct{}{}
					}
				})
			}
		}
	}
	for start := range internalExtraStructuralStarts {
		delete(extraStarts, start)
	}
	if len(extraStarts) == 0 {
		return 0
	}

	added := 0
	for state, bySym := range tables.ActionTable {
		var reduce lrAction
		reduceSet := false
		ambiguous := false
		for _, actions := range bySym {
			for _, action := range actions {
				if action.kind != lrReduce || action.isExtra {
					continue
				}
				if !reduceSet {
					reduce = action
					reduceSet = true
					continue
				}
				if action.prodIdx != reduce.prodIdx {
					ambiguous = true
					break
				}
			}
			if ambiguous {
				break
			}
		}
		if !reduceSet || ambiguous {
			continue
		}
		if reduce.prodIdx >= 0 && reduce.prodIdx < len(ng.Productions) && ng.Productions[reduce.prodIdx].LHS == realStartSymbol(ng) {
			continue
		}
		for start := range extraStarts {
			blocked := false
			for _, action := range bySym[start] {
				if action.kind == lrAccept || (action.kind == lrShift && !action.isExtra) {
					blocked = true
					break
				}
			}
			if blocked {
				continue
			}
			before := len(tables.ActionTable[state][start])
			tables.addAction(state, start, reduce)
			if len(tables.ActionTable[state][start]) > before {
				added++
			}
		}
	}
	return added
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
	if tables.ExtraChainStateStart < 0 {
		tables.ExtraChainStateStart = mainStateCount
	}

	var terminalExtras []int
	extraSymbolSet := make(map[int]struct{}, len(ng.ExtraSymbols))
	for _, e := range ng.ExtraSymbols {
		extraSymbolSet[e] = struct{}{}
		if e > 0 && e < tokenCount {
			terminalExtras = append(terminalExtras, e)
		}
	}
	externalSymbolSet := make(map[int]struct{}, len(ng.ExternalSymbols))
	for _, sym := range ng.ExternalSymbols {
		externalSymbolSet[sym] = struct{}{}
	}

	extraStartsByFirstSym := make(map[int][]int)
	var extraFirstSyms []int
	hasExternalExtraStart := false
	hasNonExternalExtraStart := false
	for _, prodIdx := range extraProds {
		prod := &ng.Productions[prodIdx]
		if len(prod.RHS) > 0 && prod.RHS[0] < tokenCount {
			firstSym := prod.RHS[0]
			if _, ok := extraStartsByFirstSym[firstSym]; !ok {
				extraFirstSyms = append(extraFirstSyms, firstSym)
				if _, ok := externalSymbolSet[firstSym]; ok {
					hasExternalExtraStart = true
				} else {
					hasNonExternalExtraStart = true
				}
			}
			extraStartsByFirstSym[firstSym] = append(extraStartsByFirstSym[firstSym], prodIdx)
		}
	}
	internalExtraStructuralStarts := make(map[int]struct{})
	for _, prodIdx := range extraProds {
		prod := &ng.Productions[prodIdx]
		if len(prod.RHS) == 0 {
			continue
		}
		_, rootExtraProduction := extraSymbolSet[prod.LHS]
		start := 0
		if rootExtraProduction {
			start = 1
		}
		for pos := start; pos < len(prod.RHS); pos++ {
			sym := prod.RHS[pos]
			if sym > 0 && sym < tokenCount {
				internalExtraStructuralStarts[sym] = struct{}{}
			}
		}
	}
	startMatchers := buildTerminalStartMatchers(ng.Terminals)

	builder := newExtraChainBuilder(tables, ng, ctx, terminalExtras)
	stateFollowSet := func(state int) bitset {
		follow := newBitset(tokenCount)
		follow.add(0)
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
		return follow
	}
	var externalExtraFollow bitset
	if hasExternalExtraStart {
		externalExtraFollow = newBitset(tokenCount)
		for state := 0; state < mainStateCount; state++ {
			follow := stateFollowSet(state)
			follow.forEach(func(sym int) {
				externalExtraFollow.add(sym)
			})
		}
	}
	stateHasContinuation := func(state int) bool {
		if acts, ok := tables.ActionTable[state]; ok {
			for _, actionList := range acts {
				for _, act := range actionList {
					if act.kind == lrShift {
						return true
					}
				}
			}
		}
		return len(tables.GotoTable[state]) > 0
	}
	stateOnlyReducesCompletedExtra := func(state int) bool {
		if stateHasContinuation(state) {
			return false
		}
		acts, ok := tables.ActionTable[state]
		if !ok {
			return false
		}
		hasReduce := false
		for _, actionList := range acts {
			for _, act := range actionList {
				if act.kind != lrReduce || !act.isExtra {
					return false
				}
				if act.prodIdx < 0 || act.prodIdx >= len(ng.Productions) {
					return false
				}
				if _, ok := extraSymbolSet[ng.Productions[act.prodIdx].LHS]; !ok {
					return false
				}
				hasReduce = true
			}
		}
		return hasReduce
	}
	syntheticStateMayInjectExtraStart := func(state, firstSym int) bool {
		if state < mainStateCount {
			return true
		}
		if _, ok := internalExtraStructuralStarts[firstSym]; ok {
			// A token that is structural syntax inside an extra chain must not
			// be reinterpreted as a sibling extra while that chain is active.
			// This lets block-comment bodies own tokens such as line-comment
			// openers without disabling normal nested extras with distinct
			// starters.
			return false
		}
		if _, ok := externalSymbolSet[firstSym]; ok {
			// External-scanner extras are context sensitive. Recursively
			// injecting their starts into synthetic extra-chain states can make
			// scanner-driven extras such as Perl POD/heredocs expand without a
			// structural bound, while main LR states still receive the extra
			// entry actions they need.
			return false
		}
		extraMatcher, ok := startMatchers[firstSym]
		if !ok {
			return true
		}
		// Narrow pruning for directive-style extras. Languages like Scala rely
		// on nested comment extras inside synthetic states; the current
		// generation pathology is driven by C#-style preprocessor extras whose
		// starters are all '#'-prefixed and do not meaningfully nest.
		if !terminalStartMatcherHasSingleRune(extraMatcher, '#') {
			return true
		}
		acts, ok := tables.ActionTable[state]
		if !ok {
			return false
		}
		for sym, actionList := range acts {
			if sym <= 0 || sym >= tokenCount {
				continue
			}
			hasStructuralShift := false
			for _, act := range actionList {
				if act.kind == lrShift && !act.isExtra {
					hasStructuralShift = true
					break
				}
			}
			if !hasStructuralShift {
				continue
			}
			if matcher, ok := startMatchers[sym]; !ok || terminalStartMatchersOverlap(extraMatcher, matcher) {
				return true
			}
		}
		return false
	}

	// Iterate over the growing state space so synthetic extra-chain states also
	// gain extra entry shifts. This closes the construction under nesting:
	// once block comments (or other nonterminal extras) can start in a
	// synthetic state, newly created states are revisited later in this loop.
	for state := 0; state < tables.StateCount; state++ {
		if state >= mainStateCount && stateOnlyReducesCompletedExtra(state) {
			continue
		}
		var follow bitset
		if hasNonExternalExtraStart {
			follow = stateFollowSet(state)
		}
		for _, firstSym := range extraFirstSyms {
			if !syntheticStateMayInjectExtraStart(state, firstSym) {
				continue
			}
			hasNonExtraAction := false
			for _, act := range tables.ActionTable[state][firstSym] {
				if !act.isExtra {
					hasNonExtraAction = true
					break
				}
			}
			if hasNonExtraAction {
				continue
			}
			entryFollow := follow
			if _, ok := externalSymbolSet[firstSym]; ok {
				entryFollow = externalExtraFollow
			} else if len(entryFollow.words) == 0 {
				entryFollow = stateFollowSet(state)
			}
			prodIdxs := extraStartsByFirstSym[firstSym]
			entryState := builder.buildEntryState(firstSym, prodIdxs, entryFollow)
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
		if int(ke.dot) >= len(prod.RHS) {
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
		key := packCoreItemKey(int(ke.prodIdx), int(ke.dot))
		if idx, ok := kernelIdx[key]; ok {
			cores[idx].lookaheads.unionWith(&ke.lookaheads)
		} else {
			idx := len(cores)
			kernelIdx[key] = idx
			cores = append(cores, coreEntry{
				prodIdx:    uint32(ke.prodIdx),
				dot:        uint32(ke.dot),
				lookaheads: ctx.cloneLookaheadBitset(&ke.lookaheads),
			})
			// Populate dot0Index for kernel items at dot=0.
			if ke.dot == 0 {
				ctx.dot0Index[ke.prodIdx] = idx
				ctx.dot0Dirty = append(ctx.dot0Dirty, int(ke.prodIdx))
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
		prod := &ng.Productions[int(ce.prodIdx)]
		if int(ce.dot) >= len(prod.RHS) {
			continue
		}

		nextSym := prod.RHS[ce.dot]
		if nextSym < tokenCount {
			continue
		}

		br := ctx.getBetaFirst(int(ce.prodIdx), int(ce.dot))

		for _, prodIdx := range ctx.prodsByLHS[nextSym] {
			// Fast path: dot=0 lookup via flat array.
			tidx := ctx.dot0Index[prodIdx]
			exists := tidx >= 0

			if !exists {
				tidx = len(cores)
				ctx.dot0Index[prodIdx] = tidx
				ctx.dot0Dirty = append(ctx.dot0Dirty, prodIdx)
				cores = append(cores, coreEntry{
					prodIdx:    uint32(prodIdx),
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
	set.computeHashes(ng.Productions, &ctx.boundaryLookaheads, ctx.needCompletionLAHash)
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
		if idx, ok := set.coreLookup(int(ne.prodIdx), int(ne.dot)); ok {
			if set.cores[idx].lookaheads.unionWith(&ne.lookaheads) {
				if ctx.closureQueuedGen[idx] != queueGen {
					worklist = append(worklist, idx)
					ctx.closureQueuedGen[idx] = queueGen
				}
			}
		} else {
			idx = len(set.cores)
			set.setCoreIndex(int(ne.prodIdx), int(ne.dot), idx)
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
		prod := &ng.Productions[int(ce.prodIdx)]
		if int(ce.dot) >= len(prod.RHS) {
			continue
		}

		nextSym := prod.RHS[ce.dot]
		if nextSym < tokenCount {
			continue
		}

		br := ctx.getBetaFirst(int(ce.prodIdx), int(ce.dot))

		for _, prodIdx := range ctx.prodsByLHS[nextSym] {
			tidx, exists := set.coreLookup(prodIdx, 0)

			if !exists {
				tidx = len(set.cores)
				set.setCoreIndex(prodIdx, 0, tidx)
				set.cores = append(set.cores, coreEntry{
					prodIdx:    uint32(prodIdx),
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

	set.computeHashes(ng.Productions, &ctx.boundaryLookaheads, ctx.needCompletionLAHash)
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

func sameAnnotationArgTagLR0(a, b *lr0ItemSet) bool {
	return a.annotationArgTag == b.annotationArgTag
}

func (ctx *lrContext) isAnnotationArgumentEntrySet(set *lrItemSet) bool {
	if ctx.annotationAtSym < 0 || ctx.annotationDefSym < 0 || ctx.annotationOpenParenSym < 0 {
		return false
	}
	for _, ce := range set.cores {
		prod := ctx.ng.Productions[int(ce.prodIdx)]
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
		prod := ctx.ng.Productions[int(ce.prodIdx)]
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
	if os.Getenv("GOT_LR_DISABLE_CONTEXT_TAGS") == "1" {
		return 0
	}
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

func (ctx *lrContext) isBracedTemplateFamilySet(set *lrItemSet) bool {
	if ctx.bracedTemplateBodySym < 0 {
		return false
	}
	for _, ce := range set.cores {
		switch ctx.ng.Productions[int(ce.prodIdx)].LHS {
		case ctx.bracedTemplateBodySym, ctx.bracedTemplateBody1Sym, ctx.bracedTemplateBody2Sym:
			return true
		}
	}
	return false
}

func expandTemplateDefinitionCarriers(ng *NormalizedGrammar, carriers []bool, tokenCount int) {
	if len(carriers) == 0 {
		return
	}
	changed := true
	for changed {
		changed = false
		for _, prod := range ng.Productions {
			if prod.LHS < 0 || prod.LHS >= len(carriers) || carriers[prod.LHS] {
				continue
			}
			if !isTemplateDefinitionCarrierWrapper(prod, carriers, tokenCount) {
				continue
			}
			carriers[prod.LHS] = true
			changed = true
		}
	}
}

func isTemplateDefinitionCarrierWrapper(prod Production, carriers []bool, tokenCount int) bool {
	switch len(prod.RHS) {
	case 1:
		sym := prod.RHS[0]
		return sym >= tokenCount && sym < len(carriers) && carriers[sym]
	case 2:
		left, right := prod.RHS[0], prod.RHS[1]
		if left == prod.LHS && right >= tokenCount && right < len(carriers) && carriers[right] {
			return true
		}
		if right == prod.LHS && left >= tokenCount && left < len(carriers) && carriers[left] {
			return true
		}
	}
	return false
}

func (ctx *lrContext) isTemplateDefinitionCarrierSet(set *lrItemSet) bool {
	if len(ctx.templateDefinitionCarrierLHS) == 0 {
		return false
	}
	for _, ce := range set.cores {
		prod := ctx.ng.Productions[int(ce.prodIdx)]
		if prod.LHS >= 0 && prod.LHS < len(ctx.templateDefinitionCarrierLHS) && ctx.templateDefinitionCarrierLHS[prod.LHS] {
			return true
		}
	}
	return false
}

func (ctx *lrContext) isCompletedRepeatWrapperForSymbol(set *lrItemSet, sym int) bool {
	return ctx.completedRepeatWrapperLHS(set, sym) >= 0
}

func (ctx *lrContext) completedRepeatWrapperLHS(set *lrItemSet, sym int) int {
	lhs := ctx.completedRepeatWrapperLHSSymsAcrossTransitions(set, sym, false)
	if len(lhs) == 0 {
		return -1
	}
	return lhs[0]
}

func (ctx *lrContext) completedRepeatWrapperLHSSymsAcrossTransitions(set *lrItemSet, sym int, allowTerminal bool) []int {
	ctx.ensureRepeatWrapperLHS()
	if sym < ctx.tokenCount {
		if !allowTerminal {
			return nil
		}
	}
	var lhsSyms []int
	for _, ce := range set.cores {
		prod := ctx.ng.Productions[int(ce.prodIdx)]
		if int(ce.dot) != len(prod.RHS) || len(prod.RHS) != 1 || prod.RHS[0] != sym {
			continue
		}
		if prod.LHS < 0 || prod.LHS >= len(ctx.ng.Symbols) {
			continue
		}
		if ctx.repeatWrapperLHS[prod.LHS] {
			found := false
			for _, lhs := range lhsSyms {
				if lhs == prod.LHS {
					found = true
					break
				}
			}
			if !found {
				lhsSyms = append(lhsSyms, prod.LHS)
			}
		}
	}
	return lhsSyms
}

func (ctx *lrContext) completedRepeatWrapperStateLHSSyms(state, sym int) []int {
	if ctx == nil || state < 0 || state >= len(ctx.itemSets) {
		return nil
	}
	if ctx.repeatWrapperStateSymSymsCache == nil {
		ctx.repeatWrapperStateSymSymsCache = make(map[uint64][]int)
	}
	key := packCoreItemKey(state, sym)
	if cached, ok := ctx.repeatWrapperStateSymSymsCache[key]; ok {
		return cached
	}
	lhsSyms := ctx.completedRepeatWrapperLHSSymsAcrossTransitions(&ctx.itemSets[state], sym, true)
	ctx.repeatWrapperStateSymSymsCache[key] = lhsSyms
	return lhsSyms
}

func (ctx *lrContext) repetitionShiftHelperLHSSyms(sourceState, sym, targetState int) []int {
	if ctx == nil || sourceState < 0 || targetState < 0 || sourceState >= len(ctx.itemSets) || targetState >= len(ctx.itemSets) {
		return nil
	}
	targetLHSs := ctx.completedRepeatWrapperStateLHSSyms(targetState, sym)
	var lhsSyms []int
	for _, lhs := range targetLHSs {
		if ctx.stateHasRecursiveRepeatSource(&ctx.itemSets[sourceState], lhs) {
			lhsSyms = append(lhsSyms, lhs)
		}
	}
	return lhsSyms
}

func (ctx *lrContext) stateHasRecursiveRepeatSource(set *lrItemSet, lhs int) bool {
	if set == nil || lhs < 0 {
		return false
	}
	for _, ce := range set.cores {
		prod := ctx.ng.Productions[int(ce.prodIdx)]
		if prod.LHS != lhs || int(ce.dot) != len(prod.RHS) {
			continue
		}
		for _, sym := range prod.RHS {
			if sym == lhs {
				return true
			}
		}
	}
	return false
}

func (ctx *lrContext) repeatWrapperSourceTagForTransition(sourceState, sym int, closedSet *lrItemSet) uint32 {
	if os.Getenv("GOT_LR_DISABLE_CONTEXT_TAGS") == "1" {
		return 0
	}
	if len(ctx.ng.Productions) < 2000 || sourceState < 0 || sourceState >= len(ctx.itemSets) {
		return 0
	}
	lhs := ctx.completedRepeatWrapperLHS(closedSet, sym)
	if lhs < 0 {
		return 0
	}
	if ctx.stateHasRecursiveRepeatSource(&ctx.itemSets[sourceState], lhs) {
		return 1 << 24
	}
	return 0
}

func (ctx *lrContext) isConditionalTypeCarrierSet(set *lrItemSet) bool {
	if ctx == nil || len(ctx.conditionalTypeCarrierLHS) == 0 {
		return false
	}
	for _, ce := range set.cores {
		prod := ctx.ng.Productions[int(ce.prodIdx)]
		if prod.LHS >= 0 && prod.LHS < len(ctx.conditionalTypeCarrierLHS) && ctx.conditionalTypeCarrierLHS[prod.LHS] {
			return true
		}
	}
	return false
}

func (ctx *lrContext) stateEntersConditionalTypeRHS(state, sym int) bool {
	if ctx == nil || state < 0 || state >= len(ctx.itemSets) {
		return false
	}
	if ctx.conditionalTypeSym < 0 || ctx.conditionalTypeExtendsSym < 0 || ctx.conditionalTypePlainQmarkSym < 0 {
		return false
	}
	if sym == ctx.conditionalTypePlainQmarkSym {
		return false
	}
	for _, ce := range ctx.itemSets[state].cores {
		prod := ctx.ng.Productions[int(ce.prodIdx)]
		if prod.LHS != ctx.conditionalTypeSym || len(prod.RHS) < 4 {
			continue
		}
		if prod.RHS[1] != ctx.conditionalTypeExtendsSym || prod.RHS[3] != ctx.conditionalTypePlainQmarkSym {
			continue
		}
		if ce.dot == 1 && int(ce.dot) < len(prod.RHS) && prod.RHS[ce.dot] == ctx.conditionalTypeExtendsSym && sym == ctx.conditionalTypeExtendsSym {
			return true
		}
	}
	return false
}

func (ctx *lrContext) conditionalTypeContextTagForTransition(sourceState, sym int, closedSet *lrItemSet) uint32 {
	if os.Getenv("GOT_LR_DISABLE_CONTEXT_TAGS") == "1" {
		return 0
	}
	if len(ctx.ng.Productions) < 2000 || sourceState < 0 || sourceState >= len(ctx.itemSets) {
		return 0
	}
	if !ctx.isConditionalTypeCarrierSet(closedSet) {
		return 0
	}
	if ctx.itemSets[sourceState].annotationArgTag&conditionalTypeContextTag != 0 {
		return conditionalTypeContextTag
	}
	if ctx.stateEntersConditionalTypeRHS(sourceState, sym) {
		return conditionalTypeContextTag
	}
	return 0
}

func (ctx *lrContext) templateContextTagForTransition(sourceState, sym int, closedSet *lrItemSet) uint32 {
	if os.Getenv("GOT_LR_DISABLE_CONTEXT_TAGS") == "1" {
		return 0
	}
	if len(ctx.ng.Productions) < 2000 || sourceState < 0 || sourceState >= len(ctx.itemSets) {
		return 0
	}

	sourceCarrier := ctx.isBracedTemplateFamilySet(&ctx.itemSets[sourceState]) ||
		ctx.isTemplateDefinitionCarrierSet(&ctx.itemSets[sourceState])
	targetCarrier := ctx.isBracedTemplateFamilySet(closedSet) ||
		ctx.isTemplateDefinitionCarrierSet(closedSet)

	srcTag := ctx.itemSets[sourceState].annotationArgTag & templateContextTagMask
	if srcTag != 0 && ctx.isCompletedRepeatWrapperForSymbol(closedSet, sym) {
		return srcTag
	}
	if !sourceCarrier && !targetCarrier {
		return 0
	}
	if ctx.annotationAtSym >= 0 && sym == ctx.annotationAtSym && targetCarrier {
		if srcTag != 0 && srcTag != templateContextPendingTag {
			return srcTag
		}
		return templateContextPendingTag
	}
	if srcTag != 0 && targetCarrier {
		return srcTag
	}
	return 0
}

func (ctx *lrContext) operatorLiteralMergeTag(set *lrItemSet) uint32 {
	if os.Getenv("GOT_LR_DISABLE_CONTEXT_TAGS") == "1" {
		return 0
	}
	if len(ctx.ng.Productions) < 2000 || ctx.operatorIdentSym < 0 || ctx.operatorStarSym < 0 || ctx.nonNullLiteralSym < 0 {
		return 0
	}
	const (
		operatorLiteralHasIdent uint32 = 1 << 8
		operatorLiteralHasStar  uint32 = 1 << 9
	)
	var hasOpIdent bool
	var hasStar bool
	for _, ce := range set.cores {
		prod := ctx.ng.Productions[int(ce.prodIdx)]
		if prod.LHS != ctx.nonNullLiteralSym || int(ce.dot) < len(prod.RHS) {
			continue
		}
		if ce.lookaheads.contains(ctx.operatorIdentSym) {
			hasOpIdent = true
		}
		if ce.lookaheads.contains(ctx.operatorStarSym) {
			hasStar = true
		}
	}
	if !hasOpIdent {
		return 0
	}
	tag := operatorLiteralHasIdent
	if hasStar {
		tag |= operatorLiteralHasStar
	}
	return tag
}

func completionFrontierItem(prods []Production, prodIdx, dot int) bool {
	rhsLen := len(prods[prodIdx].RHS)
	remaining := rhsLen - dot
	return remaining >= 0 && remaining <= 1
}

// computeHashes computes coreHash, fullHash, and completionLAHash for the item set.
// Uses commutative (additive) hashing so order of cores doesn't matter,
// avoiding the need to sort.
func (set *lrItemSet) computeHashes(prods []Production, boundaryMask *bitset, includeCompletionHash bool) {
	var ch, fh, completionHash, brh uint64
	for _, c := range set.cores {
		m := mixCoreItem(int(c.prodIdx), int(c.dot))
		ch += m
		fh += m ^ c.lookaheads.hash()
		if boundaryMask != nil {
			brh += maskedBitsetHash(&c.lookaheads, boundaryMask)
		}
		if includeCompletionHash && completionFrontierItem(prods, int(c.prodIdx), int(c.dot)) {
			completionHash += c.lookaheads.hash()
		}
	}
	set.coreHash = ch
	set.fullHash = fh
	if includeCompletionHash {
		set.completionLAHash = ch + completionHash
	} else {
		set.completionLAHash = ch
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
		if _, ok := indexed.coreLookup(int(oc.prodIdx), int(oc.dot)); !ok {
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
		idx, ok := indexed.coreLookup(int(oc.prodIdx), int(oc.dot))
		if !ok {
			return false
		}
		if !indexed.cores[idx].lookaheads.equal(&oc.lookaheads) {
			return false
		}
	}
	return true
}

// sameCompletionLookaheadsUsingIndexed returns true if two item sets have the
// same lookaheads on the completion frontier (completed items plus items with
// exactly one symbol remaining), assuming their cores already match.
func sameCompletionLookaheadsUsingIndexed(indexed, other *lrItemSet, prods []Production) bool {
	indexed.ensurePackedCoreIndex()
	for _, oc := range other.cores {
		if !completionFrontierItem(prods, int(oc.prodIdx), int(oc.dot)) {
			continue
		}
		idx, ok := indexed.coreLookup(int(oc.prodIdx), int(oc.dot))
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
		idx, ok := indexed.coreLookup(int(oc.prodIdx), int(oc.dot))
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
	ctx.transitions = nil
	ctx.ensureProvenance()

	tokenCount := ctx.tokenCount
	disableStateMerging := os.Getenv("GOT_LR_DISABLE_STATE_MERGE") == "1"

	// Hash tables for state lookup.
	// fullMap: fullHash → chain of states with that hash (exact LR(1) match)
	fullMap := make(map[uint64]*stateHashEntry)
	// coreMap: coreHash → chain of states (for LALR merge)
	var coreMap map[uint64]*stateHashEntry
	// extMap: completionLAHash → chain of states (for extended merge)
	var extMap map[uint64]*stateHashEntry
	// boundaryMap: boundaryLAHash → chain of states for large-grammar
	// external-token-sensitive merges.
	var boundaryMap map[uint64]*stateHashEntry

	// For larger grammars, prefer reduced-lookahead merging when it is still
	// tractable. Medium-sized external-scanner grammars like YAML need more than
	// boundary-token lookaheads in order to preserve key/value distinctions.
	const maxExtendedStates = 8000
	useExtendedMerging := len(ctx.ng.Productions) <= 800 ||
		(len(ctx.ng.ExternalSymbols) > 0 && len(ctx.ng.Productions) <= 2000)
	useBoundaryMerging := len(ctx.ng.ExternalSymbols) > 0 && len(ctx.ng.Productions) > 2000
	exactPrefixStates := 0
	if ctx.ng.ExactPrefixStates > 0 {
		exactPrefixStates = ctx.ng.ExactPrefixStates
	} else if len(ctx.ng.ExternalSymbols) > 0 {
		exactPrefixStates = 1024
	}
	if v := os.Getenv("GOT_LR_EXACT_PREFIX_STATES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			exactPrefixStates = n
		}
	}
	preciseStateBudget := 0
	if len(ctx.ng.ExternalSymbols) > 0 {
		// Preserve the precise external path where it converges quickly, but
		// stop before it grows far beyond the runtime-sized automata we can
		// actually use. Large scanner-heavy grammars can then fall back to LALR
		// instead of burning the full generation timeout.
		preciseStateBudget = 20000
		if v := os.Getenv("GOT_LR_PRECISE_EXTERNAL_STATE_BUDGET"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				preciseStateBudget = n
			}
		}
	}
	activateMergeMaps := func() {
		if disableStateMerging || len(ctx.itemSets) < exactPrefixStates {
			return
		}
		// Intentionally do not backfill the canonical prefix into the merge maps.
		// Those early states stay exact and can only be reused via full LR(1)
		// matches, while later states become eligible for compaction.
		if useExtendedMerging {
			if extMap == nil {
				extMap = make(map[uint64]*stateHashEntry)
			}
			return
		}
		if useBoundaryMerging {
			if boundaryMap == nil {
				boundaryMap = make(map[uint64]*stateHashEntry)
			}
			return
		}
		if coreMap == nil {
			coreMap = make(map[uint64]*stateHashEntry)
		}
	}
	if !disableStateMerging {
		activateMergeMaps()
	}
	ctx.needCompletionLAHash = useExtendedMerging

	// Initial item set: closure of [S' → .S, $end]
	initialLA := newBitset(tokenCount)
	initialLA.add(0) // $end
	initialSet := ctx.closureToSet([]coreEntry{{
		prodIdx:    uint32(ctx.ng.AugmentProdID),
		dot:        0,
		lookaheads: initialLA,
	}})
	ctx.itemSets = []lrItemSet{initialSet}
	addToHashMap(fullMap, initialSet.fullHash, 0)
	if coreMap != nil {
		addToHashMap(coreMap, initialSet.coreHash, 0)
	}
	if extMap != nil {
		addToHashMap(extMap, initialSet.completionLAHash, 0)
	}
	if boundaryMap != nil {
		addToHashMap(boundaryMap, initialSet.boundaryLAHash, 0)
	}
	ctx.recordFreshState(0)

	worklist := []int{0}
	inWorklist := map[int]bool{0: true}
	worklistIter := 0

	for len(worklist) > 0 {
		// Check for cancellation periodically (every 64 iterations) to avoid
		// the overhead of a channel receive on every loop pass.
		worklistIter++
		if worklistIter&63 == 0 {
			select {
			case <-ctx.bgCtx.Done():
				return ctx.itemSets
			default:
			}
		}
		stateIdx := worklist[0]
		worklist = worklist[1:]
		inWorklist[stateIdx] = false
		itemSet := &ctx.itemSets[stateIdx]
		activateMergeMaps()

		// Collect all symbols after the dot.
		symsSeen := make(map[int]bool)
		syms := ctx.gotoSymbolsScratch[:0]
		for _, ce := range itemSet.cores {
			prod := &ctx.ng.Productions[int(ce.prodIdx)]
			if int(ce.dot) < len(prod.RHS) {
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
				prod := &ctx.ng.Productions[int(ce.prodIdx)]
				if int(ce.dot) < len(prod.RHS) && prod.RHS[ce.dot] == sym {
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
			closedSet.annotationArgTag |= ctx.templateContextTagForTransition(stateIdx, sym, &closedSet)
			closedSet.annotationArgTag |= ctx.repeatWrapperSourceTagForTransition(stateIdx, sym, &closedSet)
			closedSet.annotationArgTag |= ctx.conditionalTypeContextTagForTransition(stateIdx, sym, &closedSet)
			closedSet.annotationArgTag |= ctx.operatorLiteralMergeTag(&closedSet)
			ctx.gotoAdvancedScratch = advanced[:0]

			targetIdx := ctx.findOrCreateState(
				&closedSet,
				stateIdx,
				fullMap, coreMap, extMap, boundaryMap,
				extMap != nil && len(ctx.itemSets) < maxExtendedStates,
				boundaryMap != nil,
				&worklist, &inWorklist,
			)

			// Record transition for table construction.
			ctx.addTransition(stateIdx, sym, targetIdx)
			if preciseStateBudget > 0 && len(ctx.itemSets) > preciseStateBudget {
				ctx.preciseStateBudgetExceeded = true
				return ctx.itemSets
			}
		}
		ctx.sortStateTransitions(stateIdx)
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
	sourceState int,
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
		// 2a. Extended merging: find state with same core AND same completion-frontier lookaheads.
		for entry := extMap[closedSet.completionLAHash]; entry != nil; entry = entry.next {
			existing := &ctx.itemSets[entry.stateIdx]
			if sameAnnotationArgTag(existing, closedSet) &&
				existing.coreHash == closedSet.coreHash &&
				sameCoresUsingIndexed(existing, closedSet) &&
				sameCompletionLookaheadsUsingIndexed(existing, closedSet, ctx.ng.Productions) {
				// Merge lookaheads into existing state.
				targetIdx := ctx.mergeInto(entry.stateIdx, sourceState, closedSet, fullMap, extMap, boundaryMap, worklist, inWorklist)
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
				targetIdx := ctx.mergeInto(entry.stateIdx, sourceState, closedSet, fullMap, extMap, boundaryMap, worklist, inWorklist)
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
				targetIdx := ctx.mergeInto(entry.stateIdx, sourceState, closedSet, fullMap, extMap, boundaryMap, worklist, inWorklist)
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
		addToHashMap(extMap, closedSet.completionLAHash, newIdx)
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
	sourceState int,
	closedSet *lrItemSet,
	fullMap, extMap, boundaryMap map[uint64]*stateHashEntry,
	worklist *[]int,
	inWorklist *map[int]bool,
) int {
	// Collect new core entries to merge.
	var newEntries []coreEntry
	existing := &ctx.itemSets[idx]
	for _, ce := range closedSet.cores {
		if eidx, ok := existing.coreLookup(int(ce.prodIdx), int(ce.dot)); ok {
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
		oldCompletionHash := existing.completionLAHash
		oldBoundaryHash := existing.boundaryLAHash
		ctx.closureIncremental(existing, newEntries)
		ctx.recordMergedState(idx, mergeOrigin{
			kernelHash:  closedSet.coreHash,
			sourceState: sourceState,
		})
		// Update hash maps with new hashes.
		addToHashMap(fullMap, existing.fullHash, idx)
		if extMap != nil && existing.completionLAHash != oldCompletionHash {
			addToHashMap(extMap, existing.completionLAHash, idx)
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
func resolveConflicts(ctx context.Context, tables *LRTables, ng *NormalizedGrammar) (conflictResolutionStats, error) {
	return resolveConflictsWithTrace(ctx, tables, ng, phaseTrace{})
}

func resolveConflictsWithTrace(ctx context.Context, tables *LRTables, ng *NormalizedGrammar, trace phaseTrace) (conflictResolutionStats, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var stats conflictResolutionStats

	endAugment := trace.start("resolve_conflicts_augment", nil)
	augmentStats, err := augmentAdjacentRepeatElementReduceLookaheadsWithTrace(ctx, tables, ng, trace)
	stats.add(augmentStats)
	if err != nil {
		endAugment(stats.augmentTraceFields())
		return stats, err
	}
	endAugment(augmentStats.augmentTraceFields())

	cache := getConflictResolutionCache(ng)
	if cache != nil {
		cache.resetStructuralStats()
	}
	endActions := trace.start("resolve_conflicts_actions", nil)
	actionFields := func() map[string]any {
		current := stats
		current.add(cache.snapshotStructuralStats())
		return current.actionTraceFields()
	}
	finishActions := func() {
		stats.add(cache.snapshotStructuralStats())
		endActions(stats.actionTraceFields())
	}
	if err := checkConflictResolutionContext(ctx, "before action resolution"); err != nil {
		finishActions()
		return stats, err
	}

	states := make([]int, 0, len(tables.ActionTable))
	for state := range tables.ActionTable {
		states = append(states, state)
	}
	// Sort in reverse so earlier states (lower index) are resolved last and
	// any error message points to the earliest conflicting state.
	// Actually sort ascending: report errors in state order and allow
	// deterministic resolution regardless of map iteration order.
	sort.Ints(states)
	for _, state := range states {
		if err := checkConflictResolutionContext(ctx, "scanning states"); err != nil {
			finishActions()
			return stats, err
		}
		stats.StatesScanned++
		actions := tables.ActionTable[state]
		syms := make([]int, 0, len(actions))
		for sym := range actions {
			syms = append(syms, sym)
		}
		sort.Ints(syms)
		for _, sym := range syms {
			stats.ActionEntriesScanned++
			if stats.ActionEntriesScanned&1023 == 0 {
				if trace.enabled {
					trace.log("resolve_conflicts_actions", "progress", 0, actionFields())
				}
				if err := checkConflictResolutionContext(ctx, "scanning action entries"); err != nil {
					finishActions()
					return stats, err
				}
			}
			acts := actions[sym]
			if len(acts) <= 1 {
				continue
			}
			stats.ConflictsResolved++
			if len(acts) > stats.MaxActionsPerConflict {
				stats.MaxActionsPerConflict = len(acts)
			}

			resolved, err := resolveActionConflict(sym, acts, ng)
			if err != nil {
				finishActions()
				return stats, fmt.Errorf("state %d, symbol %d: %w", state, sym, err)
			}
			tables.ActionTable[state][sym] = resolved
		}
	}
	if err := checkConflictResolutionContext(ctx, "after action resolution"); err != nil {
		finishActions()
		return stats, err
	}
	finishActions()
	return stats, nil
}

func augmentAdjacentRepeatElementReduceLookaheads(ctx context.Context, tables *LRTables, ng *NormalizedGrammar) (conflictResolutionStats, error) {
	return augmentAdjacentRepeatElementReduceLookaheadsWithTrace(ctx, tables, ng, phaseTrace{})
}

func augmentAdjacentRepeatElementReduceLookaheadsWithTrace(ctx context.Context, tables *LRTables, ng *NormalizedGrammar, trace phaseTrace) (conflictResolutionStats, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var stats conflictResolutionStats
	const augmentProgressInterval = 65536
	augmentFields := func(state, stateCandidates, stateTerminalEntries int) map[string]any {
		fields := stats.augmentTraceFields()
		fields["current_state"] = state
		fields["current_state_candidates"] = stateCandidates
		fields["current_state_terminal_entries"] = stateTerminalEntries
		return fields
	}
	if err := checkConflictResolutionContext(ctx, "before repeat lookahead augmentation"); err != nil {
		return stats, err
	}
	if tables == nil || ng == nil {
		return stats, nil
	}
	cache := getConflictResolutionCache(ng)
	if cache == nil {
		return stats, nil
	}
	type reduceCandidate struct {
		action lrAction
		lhs    int
	}
	type repeatLookaheadKey struct {
		lhs       int
		lookahead int
	}
	repeatStartMemo := make(map[repeatLookaheadKey]bool)
	repeatStartCanLookahead := func(lhs, sym int) (bool, error) {
		key := repeatLookaheadKey{lhs: lhs, lookahead: sym}
		if cached, ok := repeatStartMemo[key]; ok {
			stats.AugmentRepeatStartCacheHits++
			return cached, nil
		}
		stats.AugmentRepeatStartCacheMisses++
		if err := checkConflictResolutionContext(ctx, "before repeat lookahead cache miss"); err != nil {
			return false, err
		}
		canStart := repeatElementCanStartAdjacentRepeatOnLookaheadCtx(ctx, lhs, sym, ng, cache)
		repeatStartMemo[key] = canStart
		if err := checkConflictResolutionContext(ctx, "after repeat lookahead cache miss"); err != nil {
			return false, err
		}
		return canStart, nil
	}
	for state, actions := range tables.ActionTable {
		if err := checkConflictResolutionContext(ctx, "augmenting repeat lookaheads"); err != nil {
			return stats, err
		}
		stats.AugmentStatesScanned++
		if trace.enabled && stats.AugmentStatesScanned&511 == 0 {
			trace.log("resolve_conflicts_augment", "progress", 0, augmentFields(state, 0, 0))
		}
		hasShiftTarget := false
		stateTerminalEntries := 0
		for sym, acts := range actions {
			stats.AugmentActionEntriesScanned++
			if trace.enabled && stats.AugmentActionEntriesScanned%augmentProgressInterval == 0 {
				trace.log("resolve_conflicts_augment", "progress", 0, augmentFields(state, 0, stateTerminalEntries))
			}
			if stats.AugmentActionEntriesScanned&1023 == 0 {
				if err := checkConflictResolutionContext(ctx, "scanning repeat lookahead shift targets"); err != nil {
					return stats, err
				}
			}
			if !isLRConflictTerminalSymbol(sym, ng) {
				continue
			}
			stateTerminalEntries++
			if lrActionListHasShift(acts) {
				hasShiftTarget = true
			}
		}
		if stateTerminalEntries > stats.AugmentMaxTerminalEntriesState {
			stats.AugmentMaxTerminalEntriesState = stateTerminalEntries
		}
		if !hasShiftTarget {
			stats.AugmentStatesWithoutShiftTarget++
			continue
		}
		var candidates []reduceCandidate
		seen := make(map[int]bool)
		for sym, acts := range actions {
			stats.AugmentActionEntriesScanned++
			if trace.enabled && stats.AugmentActionEntriesScanned%augmentProgressInterval == 0 {
				trace.log("resolve_conflicts_augment", "progress", 0, augmentFields(state, len(candidates), stateTerminalEntries))
			}
			if stats.AugmentActionEntriesScanned&1023 == 0 {
				if err := checkConflictResolutionContext(ctx, "scanning repeat lookahead candidate actions"); err != nil {
					return stats, err
				}
			}
			if !isLRConflictTerminalSymbol(sym, ng) {
				continue
			}
			for _, act := range acts {
				lhs, ok := neutralAdjacentRepeatElementReduceLHS(act, ng, cache)
				if !ok || seen[act.prodIdx] {
					continue
				}
				canStart, err := repeatStartCanLookahead(lhs, sym)
				if err != nil {
					return stats, err
				}
				if !canStart {
					continue
				}
				seen[act.prodIdx] = true
				candidates = append(candidates, reduceCandidate{action: act, lhs: lhs})
				stats.AugmentCandidates++
			}
		}
		if len(candidates) > stats.AugmentMaxCandidatesPerState {
			stats.AugmentMaxCandidatesPerState = len(candidates)
		}
		if len(candidates) == 0 {
			continue
		}
		for sym, acts := range actions {
			stats.AugmentActionEntriesScanned++
			if trace.enabled && stats.AugmentActionEntriesScanned%augmentProgressInterval == 0 {
				trace.log("resolve_conflicts_augment", "progress", 0, augmentFields(state, len(candidates), stateTerminalEntries))
			}
			if stats.AugmentActionEntriesScanned&1023 == 0 {
				if err := checkConflictResolutionContext(ctx, "augmenting repeat lookahead action entries"); err != nil {
					return stats, err
				}
			}
			if !isLRConflictTerminalSymbol(sym, ng) || len(acts) == 0 {
				continue
			}
			if lrActionListHasShift(acts) {
				stats.AugmentSecondPassShiftEntries++
			} else if lrActionListReduceOnly(acts) {
				stats.AugmentSecondPassReduceOnly++
				continue
			} else {
				continue
			}
			for _, candidate := range candidates {
				stats.AugmentCandidateChecks++
				if trace.enabled && stats.AugmentCandidateChecks%augmentProgressInterval == 0 {
					trace.log("resolve_conflicts_augment", "progress", 0, augmentFields(state, len(candidates), stateTerminalEntries))
				}
				if stats.AugmentCandidateChecks&1023 == 0 {
					if err := checkConflictResolutionContext(ctx, "checking repeat lookahead candidates"); err != nil {
						return stats, err
					}
				}
				if lrActionListHasReduce(acts, candidate.action.prodIdx) {
					continue
				}
				canStart, err := repeatStartCanLookahead(candidate.lhs, sym)
				if err != nil {
					return stats, err
				}
				if !canStart {
					continue
				}
				tables.addAction(state, sym, candidate.action)
				stats.AugmentLookaheadsAdded++
				acts = tables.ActionTable[state][sym]
			}
		}
		if trace.enabled && len(candidates) > 0 && stats.AugmentStatesScanned&511 == 0 {
			trace.log("resolve_conflicts_augment", "progress", 0, augmentFields(state, len(candidates), stateTerminalEntries))
		}
	}
	if err := checkConflictResolutionContext(ctx, "after repeat lookahead augmentation"); err != nil {
		return stats, err
	}
	return stats, nil
}

func checkConflictResolutionContext(ctx context.Context, phase string) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("conflict resolution %s: %w", phase, err)
	}
	return nil
}

func isLRConflictTerminalSymbol(sym int, ng *NormalizedGrammar) bool {
	return ng != nil &&
		sym >= 0 &&
		sym < len(ng.Symbols) &&
		ng.Symbols[sym].Kind != SymbolNonterminal
}

func neutralAdjacentRepeatElementReduceLHS(action lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) (int, bool) {
	if action.kind != lrReduce || action.prodIdx < 0 || action.prodIdx >= len(ng.Productions) ||
		isRepeatHelperReduce(action, ng, cache) {
		return 0, false
	}
	prod := &ng.Productions[action.prodIdx]
	if prod.LHS < 0 || prod.LHS >= len(ng.Symbols) {
		return 0, false
	}
	return prod.LHS, true
}

func lrActionListHasReduce(actions []lrAction, prodIdx int) bool {
	for _, action := range actions {
		if action.kind == lrReduce && action.prodIdx == prodIdx {
			return true
		}
	}
	return false
}

func lrActionListHasShift(actions []lrAction) bool {
	for _, action := range actions {
		if action.kind == lrShift {
			return true
		}
	}
	return false
}

func lrActionListReduceOnly(actions []lrAction) bool {
	hasReduce := false
	for _, action := range actions {
		switch action.kind {
		case lrShift:
			return false
		case lrReduce:
			hasReduce = true
		}
	}
	return hasReduce
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
		if repeated, ok := repetitionShiftActions(lookaheadSym, shifts, reduces, ng, cache); ok {
			return repeated, nil
		}
		if repeated, ok := loweredRepeatMixedContinuationActions(lookaheadSym, shifts, reduces, ng, cache); ok {
			return repeated, nil
		}
		if repeated, ok := loweredRepeatHelperContinuationActions(lookaheadSym, shifts, reduces, ng, cache); ok {
			return repeated, nil
		}
		if repeated, ok := visibleLoweredRepeatBodyContinuationActions(lookaheadSym, actions, shifts, reduces, ng, cache); ok {
			return repeated, nil
		}

		shift := shifts[0]
		reduce := reduces[0]
		prod := &ng.Productions[reduce.prodIdx]
		shiftMeta := shiftMetadataForReduce(shift, prod.LHS, ng, cache)
		reduceMeta := reduceMetadataForShiftConflict(reduce, shift, ng, cache)

		if isRepeatHelperReduce(reduce, ng, cache) &&
			!repeatHelperReduceContinuesWithShift(lookaheadSym, reduce, shift, ng, cache) &&
			!shiftHasHigherPrecedenceThanReduceHelper(shift, prod) &&
			!shiftReduceInConflictGroup(shifts, reduces, ng, cache) {
			return []lrAction{reduce}, nil
		}
		if shouldKeepRepeatHelperReduceWithUnrelatedShift(lookaheadSym, shifts, reduces, ng, cache) {
			return actions, nil
		}
		if repeated, ok := repeatElementReduceActions(lookaheadSym, actions, shifts, reduces, ng, cache); ok {
			return repeated, nil
		}
		if shouldPreserveDerivedKeywordIdentifierShiftReduce(lookaheadSym, reduces, ng) {
			return actions, nil
		}
		if preferred, ok := preferredVisibleSiblingAlternativeContinuationShift(lookaheadSym, shifts, reduces, ng, cache); ok {
			return []lrAction{preferred}, nil
		}
		if shouldPreferAssignmentExpressionShift(lookaheadSym, shifts, reduces, ng) {
			return []lrAction{shift}, nil
		}
		if preferred, ok := preferredRightAssocFinalOperandContinuationShift(lookaheadSym, shifts, reduces, ng, cache); ok {
			return []lrAction{preferred}, nil
		}
		if preferred, ok := preferredRightAssocSameLHSOptionalContinuationShift(lookaheadSym, shifts, reduces, ng, cache); ok {
			return []lrAction{preferred}, nil
		}
		if preferred, ok := preferredArithmeticExpressionContinuation(lookaheadSym, shifts, reduces, ng); ok {
			return []lrAction{preferred}, nil
		}
		if preferred, ok := preferredArithmeticWrapperShift(lookaheadSym, shifts, reduces, ng); ok {
			return []lrAction{preferred}, nil
		}
		if preferred, ok := preferredArithmeticDelimiterShift(lookaheadSym, shifts, reduces, ng); ok {
			return []lrAction{preferred}, nil
		}
		if preferred, ok := preferredKeywordContinuationShift(lookaheadSym, shifts, reduces, ng); ok {
			return []lrAction{preferred}, nil
		}
		if shouldPreserveKeywordIdentifierShiftReduce(lookaheadSym, reduces, ng) {
			return actions, nil
		}
		if preferred, ok := preferredCompletedCallDoBlockReduce(lookaheadSym, shifts, reduces, ng); ok {
			return preferred, nil
		}
		if preferred, ok := preferredRemoteCallOperatorReduce(lookaheadSym, shifts, reduces, ng); ok {
			return preferred, nil
		}
		if preferred, ok := preferredStabClauseLeftArrowReduce(lookaheadSym, shifts, reduces, ng); ok {
			return preferred, nil
		}
		if preserved, ok := preservedStabClauseArrowExpressionAmbiguity(lookaheadSym, shifts, reduces, ng); ok {
			return preserved, nil
		}
		if preferred, ok := preferredAtomToExpressionOperatorIdentifierReduce(lookaheadSym, shifts, reduces, ng); ok {
			return preferred, nil
		}
		if shouldKeepExpressionStructInitializerConflict(lookaheadSym, shifts, reduces, ng) {
			return actions, nil
		}
		if preferred, ok := preferredClosureParametersReduce(shifts, reduces, ng); ok {
			return preferred, nil
		}

		// Tree-sitter keeps S/R as GLR when the reduce LHS and a shift LHS
		// are both in the same declared conflict group.
		if shiftReduceInConflictGroup(shifts, reduces, ng, cache) {
			// When the shift and reduce share the same LHS symbol (intra-
			// symbol conflict, e.g. binary_expression && vs ||), explicit
			// precedence/associativity should still resolve the conflict.
			// Without this, all binary operators with different precedences
			// would be kept as GLR, causing wrong associativity at runtime.
			// Inter-symbol conflicts (different LHS) stay as GLR — those
			// represent genuine ambiguities declared by the grammar author.
			sameLHS := shiftActionMatchesReduceLHSFamily(shift, prod.LHS, ng, cache)
			if sameLHS {
				shiftP := shiftMeta.prec
				reduceP := reduceMeta.prec
				if (shiftP != 0 || reduceP != 0) && shiftP != reduceP {
					if reduceP > shiftP {
						return []lrAction{reduce}, nil
					}
					return []lrAction{shift}, nil
				}
				if shiftP == reduceP && reduceMeta.assoc != AssocNone {
					switch reduceMeta.assoc {
					case AssocLeft:
						return []lrAction{reduce}, nil
					case AssocRight:
						return []lrAction{shift}, nil
					}
				}
			}
			if preferred, ok := preferredExpressionOperatorIdentifierReduce(lookaheadSym, shifts, reduces, ng); ok {
				return preferred, nil
			}
			return actions, nil
		}

		// Fallback: if the reduce LHS is in ANY conflict group, keep GLR —
		// UNLESS explicit precedence clearly resolves the conflict.
		// Tree-sitter C resolves S/R conflicts via precedence even when
		// symbols are in conflict groups. The original all-GLR fallback
		// was too broad, generating thousands of unnecessary GLR entries
		// for grammars like Swift where many symbols appear in conflict
		// groups but have unambiguous precedence relationships.
		if reduceLHSInAnyConflictGroup(reduces, ng, cache) {
			shiftP := shiftMeta.prec
			reduceP := reduceMeta.prec
			// Consult precedences table for SYMBOL-level ordering before
			// falling through to numeric prec comparison. This ensures
			// that SYMBOL entries like update_expression can resolve
			// conflicts even within conflict group contexts.
			if ng.PrecedenceOrder != nil {
				// Case 1: reduce LHS is SYMBOL (prec 0), shift prec is named (> 0).
				if reduceP == 0 && shiftP > 0 && prod.LHS < len(ng.Symbols) {
					lhsName := ng.Symbols[prod.LHS].Name
					cmp := ng.PrecedenceOrder.resolveSymbolVsNamedPrec(lhsName, shiftP)
					if cmp > 0 {
						return []lrAction{reduce}, nil
					}
					if cmp < 0 {
						return []lrAction{shift}, nil
					}
				}
				// Case 2: shift LHS is SYMBOL (prec 0), reduce prec is named (> 0).
				if shiftP == 0 && reduceP > 0 {
					cmp := resolveShiftLHSVsNamedPrec(shift, reduceP, ng)
					if cmp > 0 {
						return []lrAction{shift}, nil
					}
					if cmp < 0 {
						return []lrAction{reduce}, nil
					}
				}
			}
			// Check if precedence can resolve this definitively.
			if preferred, ok := preferredLoweredRepeatContinuationShift(lookaheadSym, shifts, reduces, ng, cache); ok {
				return []lrAction{preferred}, nil
			}
			if (shiftP != 0 || reduceP != 0) && shiftP != reduceP {
				// Clear precedence difference — resolve deterministically.
				if reduceP > shiftP {
					return []lrAction{reduce}, nil
				}
				return []lrAction{shift}, nil
			}
			// Before applying associativity, check SYMBOL vs SYMBOL
			// ordering from the precedences table. When two symbols
			// in the same precedence level have equal numeric prec,
			// the ordering determines which binds tighter.
			if shiftP == reduceP && ng.PrecedenceOrder != nil &&
				prod.LHS >= 0 && prod.LHS < len(ng.Symbols) {
				cmp := resolveShiftLHSVsReduceLHSPrecedence(shift, prod.LHS, ng)
				if cmp > 0 {
					return []lrAction{shift}, nil
				}
				if cmp < 0 {
					return []lrAction{reduce}, nil
				}
			}
			// Same precedence or both zero — check associativity.
			if shiftP == reduceP && reduceMeta.assoc != AssocNone {
				if reduceMeta.assoc == AssocLeft {
					if preferred, ok := preferredSameLHSContinuationShift(shifts, reduces, ng, cache); ok {
						return []lrAction{preferred}, nil
					}
					if preferred, ok := preferredLoweredRepeatContinuationShift(lookaheadSym, shifts, reduces, ng, cache); ok {
						return []lrAction{preferred}, nil
					}
				}
				switch reduceMeta.assoc {
				case AssocLeft:
					return []lrAction{reduce}, nil
				case AssocRight:
					return []lrAction{shift}, nil
				}
			}
			if preferred, ok := preferredExpressionOperatorIdentifierReduce(lookaheadSym, shifts, reduces, ng); ok {
				return preferred, nil
			}
			// No clear resolution — keep as GLR.
			return actions, nil
		}

		shiftPrec := shiftMeta.prec
		reducePrec := reduceMeta.prec
		// Consult the precedences table for SYMBOL-level ordering.
		// Only apply when:
		// 1. The reduce production's LHS is a SYMBOL entry in the table
		// 2. The reduce prec is 0 (from the grammar's PREC(0) wrapper)
		// 3. The shift prec is non-zero (from a named STRING prec like "logical_and")
		// Guard: shiftPrec must be > 0 because value 0 is ambiguous (could be
		// the default/unset value or a named prec like "object" that happens
		// to map to 0). Only named precs with positive values are unambiguous.
		if ng.PrecedenceOrder != nil && reducePrec == 0 && shiftPrec > 0 && prod.LHS < len(ng.Symbols) {
			lhsName := ng.Symbols[prod.LHS].Name
			cmp := ng.PrecedenceOrder.resolveSymbolVsNamedPrec(lhsName, shiftPrec)
			if cmp > 0 {
				return []lrAction{reduce}, nil
			}
			if cmp < 0 {
				return []lrAction{shift}, nil
			}
		}
		// Case 2: shift LHS is SYMBOL (prec 0), reduce prec is named STRING (> 0).
		if ng.PrecedenceOrder != nil && shiftPrec == 0 && reducePrec > 0 {
			cmp := resolveShiftLHSVsNamedPrec(shift, reducePrec, ng)
			if cmp > 0 {
				return []lrAction{shift}, nil
			}
			if cmp < 0 {
				return []lrAction{reduce}, nil
			}
		}
		// Apply precedence/associativity resolution when either side has a
		// non-zero precedence OR the production declares explicit associativity.
		if reducePrec != 0 || shiftPrec != 0 || reduceMeta.assoc != AssocNone {
			if preferred, ok := preferredLoweredRepeatContinuationShift(lookaheadSym, shifts, reduces, ng, cache); ok {
				return []lrAction{preferred}, nil
			}
			if reducePrec > shiftPrec {
				return []lrAction{reduce}, nil
			}
			if shiftPrec > reducePrec {
				return []lrAction{shift}, nil
			}
			// Prec values are equal — before applying associativity,
			// check if the SYMBOL ordering from the precedences table
			// can break the tie. Two symbols in the same level with
			// equal numeric prec but different ordering positions should
			// be resolved by position (higher-ordered binds tighter).
			// Example: TypeScript [intersection_type, union_type,
			// conditional_type, function_type] all have PREC_LEFT(0).
			// For "() => T | U", union_type (higher pos) should bind
			// tighter than function_type (lower pos), so shift wins.
			if ng.PrecedenceOrder != nil &&
				prod.LHS >= 0 && prod.LHS < len(ng.Symbols) {
				cmp := resolveShiftLHSVsReduceLHSPrecedence(shift, prod.LHS, ng)
				if cmp > 0 {
					return []lrAction{shift}, nil
				}
				if cmp < 0 {
					return []lrAction{reduce}, nil
				}
			}
			switch reduceMeta.assoc {
			case AssocLeft:
				if preferred, ok := preferredSameLHSContinuationShift(shifts, reduces, ng, cache); ok {
					return []lrAction{preferred}, nil
				}
				if preferred, ok := preferredLoweredRepeatContinuationShift(lookaheadSym, shifts, reduces, ng, cache); ok {
					return []lrAction{preferred}, nil
				}
				return []lrAction{reduce}, nil
			case AssocRight:
				return []lrAction{shift}, nil
			case AssocNone:
				return nil, nil
			}
		}

		if preferred, ok := preferredExpressionOperatorIdentifierReduce(lookaheadSym, shifts, reduces, ng); ok {
			return preferred, nil
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

func shiftActionMatchesReduceLHSFamily(shift lrAction, reduceLHS int, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if reduceLHS < 0 || ng == nil || reduceLHS >= len(ng.Symbols) {
		return false
	}
	if shift.lhsSym == reduceLHS {
		return true
	}
	for _, lhs := range shift.lhsSyms {
		if lhs == reduceLHS {
			return true
		}
	}
	if cache == nil {
		return false
	}
	for lhs := range shiftContinuationTargets(shift, len(ng.Symbols)) {
		for _, parent := range resolveAuxToParents(lhs, ng, cache) {
			if parent == reduceLHS {
				return true
			}
		}
	}
	return false
}

func preferredSameLHSContinuationShift(shifts, reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) (lrAction, bool) {
	if ng == nil || cache == nil || len(shifts) == 0 || len(reduces) != 1 {
		return lrAction{}, false
	}
	var matched []lrAction
	for _, shift := range shifts {
		if shift.kind != lrShift {
			continue
		}
		if leftAssocReduceContinuesWithShift(reduces[0], shift, ng, cache) ||
			visibleSameLHSOptionalTailContinuesWithShift(reduces[0], shift, ng, cache) {
			matched = append(matched, shift)
		}
	}
	if len(matched) != 1 {
		return lrAction{}, false
	}
	return matched[0], true
}

func visibleSameLHSOptionalTailContinuesWithShift(reduce, shift lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if reduce.kind != lrReduce || reduce.prodIdx < 0 || ng == nil || cache == nil || reduce.prodIdx >= len(ng.Productions) {
		return false
	}
	prod := &ng.Productions[reduce.prodIdx]
	if prod.LHS < 0 || prod.LHS >= len(cache.prodsByLHS) {
		return false
	}
	targets := shiftContinuationTargets(shift, len(ng.Symbols))
	if !targets[prod.LHS] {
		return false
	}
	for _, prodIdx := range cache.prodsByLHS[prod.LHS] {
		if prodIdx < 0 || prodIdx >= len(ng.Productions) {
			continue
		}
		candidate := &ng.Productions[prodIdx]
		if candidate.LHS != prod.LHS || len(candidate.RHS) <= len(prod.RHS) || !rhsHasPrefix(candidate.RHS, prod.RHS) {
			continue
		}
		suffix := candidate.RHS[len(prod.RHS):]
		if len(suffix) == 0 || suffix[0] < 0 || suffix[0] >= len(ng.Symbols) {
			continue
		}
		suffixInfo := ng.Symbols[suffix[0]]
		if suffixInfo.Kind != SymbolNonterminal || !suffixInfo.Visible {
			continue
		}
		if rhsCanBeginWithAny(suffix, targets, cache, ng) {
			return true
		}
	}
	return false
}
func preferredVisibleSiblingAlternativeContinuationShift(lookaheadSym int, shifts, reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) (lrAction, bool) {
	if ng == nil || cache == nil || len(shifts) != 1 || len(reduces) == 0 ||
		lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) {
		return lrAction{}, false
	}
	if !symbolIsParenthesisOpener(lookaheadSym, ng) {
		return lrAction{}, false
	}
	shift := shifts[0]
	if shift.kind != lrShift {
		return lrAction{}, false
	}
	shiftTargets := shiftContinuationTargets(shift, len(ng.Symbols))
	if len(shiftTargets) == 0 {
		return lrAction{}, false
	}
	lookaheadTargets := map[int]bool{lookaheadSym: true}
	for _, reduce := range reduces {
		if reduce.kind != lrReduce || reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
			continue
		}
		reduceProd := &ng.Productions[reduce.prodIdx]
		if len(reduceProd.RHS) == 0 || reduceProd.LHS < 0 || reduceProd.LHS >= len(cache.prodsByLHS) {
			continue
		}
		for candidateLHS := 0; candidateLHS < len(cache.prodsByLHS); candidateLHS++ {
			if candidateLHS == reduceProd.LHS ||
				candidateLHS < 0 || candidateLHS >= len(ng.Symbols) ||
				ng.Symbols[candidateLHS].Kind != SymbolNonterminal ||
				(!ng.Symbols[candidateLHS].Visible && !ng.Symbols[candidateLHS].Named) ||
				!symbolHasUnaryPath(reduceProd.LHS, candidateLHS, ng, cache) {
				continue
			}
			for _, prodIdx := range cache.prodsByLHS[candidateLHS] {
				if prodIdx < 0 || prodIdx >= len(ng.Productions) {
					continue
				}
				candidate := &ng.Productions[prodIdx]
				if candidate.LHS != candidateLHS {
					continue
				}
				suffix, ok := siblingAlternativeContinuationSuffix(candidate.RHS, reduceProd.RHS, ng, cache)
				if !ok {
					continue
				}
				if rhsCanBeginWithAny(suffix, lookaheadTargets, cache, ng) &&
					rhsCanStartWithAnySymbol(suffix, shiftTargets, cache) {
					return shift, true
				}
			}
		}
	}
	return lrAction{}, false
}

func siblingAlternativeContinuationSuffix(candidateRHS, reduceRHS []int, ng *NormalizedGrammar, cache *conflictResolutionCache) ([]int, bool) {
	if len(candidateRHS) <= len(reduceRHS) || len(reduceRHS) == 0 {
		return nil, false
	}
	if rhsHasPrefix(candidateRHS, reduceRHS) {
		return candidateRHS[len(reduceRHS):], true
	}
	if len(reduceRHS) == 1 && symbolHasUnaryPath(candidateRHS[0], reduceRHS[0], ng, cache) {
		return candidateRHS[1:], true
	}
	return nil, false
}

func symbolIsParenthesisOpener(sym int, ng *NormalizedGrammar) bool {
	if sym < 0 || ng == nil || sym >= len(ng.Symbols) {
		return false
	}
	return ng.Symbols[sym].Name == "("
}

func symbolHasUnaryPath(from, to int, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if from == to {
		return true
	}
	if ng == nil || cache == nil || from < 0 || from >= len(cache.prodsByLHS) || to < 0 || to >= len(ng.Symbols) {
		return false
	}
	seen := make(map[int]bool)
	var walk func(int) bool
	walk = func(sym int) bool {
		if sym == to {
			return true
		}
		if sym < 0 || sym >= len(cache.prodsByLHS) || seen[sym] {
			return false
		}
		seen[sym] = true
		for _, prodIdx := range cache.prodsByLHS[sym] {
			if prodIdx < 0 || prodIdx >= len(ng.Productions) {
				continue
			}
			prod := &ng.Productions[prodIdx]
			if prod.LHS == sym && len(prod.RHS) == 1 && prod.RHS[0] >= 0 && walk(prod.RHS[0]) {
				return true
			}
		}
		return false
	}
	return walk(from)
}

func rhsCanStartWithAnySymbol(rhs []int, targets map[int]bool, cache *conflictResolutionCache) bool {
	if len(rhs) == 0 || len(targets) == 0 || cache == nil {
		return false
	}
	for _, sym := range rhs {
		if targets[sym] {
			return true
		}
		if sym < 0 || sym >= len(cache.nullable) || !cache.nullable[sym] {
			return false
		}
	}
	return false
}

func preferredRightAssocFinalOperandContinuationShift(lookaheadSym int, shifts, reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) (lrAction, bool) {
	if ng == nil || cache == nil || len(shifts) != 1 || len(reduces) != 1 ||
		lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) {
		return lrAction{}, false
	}
	shift := shifts[0]
	reduce := reduces[0]
	if shift.kind != lrShift || reduce.kind != lrReduce ||
		reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
		return lrAction{}, false
	}
	reduceProd := &ng.Productions[reduce.prodIdx]
	if reduceProd.Assoc != AssocRight || len(reduceProd.RHS) == 0 {
		return lrAction{}, false
	}
	targets := shiftContinuationTargets(shift, len(ng.Symbols))
	if len(targets) == 0 {
		return lrAction{}, false
	}
	lookaheadTargets := map[int]bool{lookaheadSym: true}
	for target := range targets {
		if target < 0 || target >= len(cache.prodsByLHS) {
			continue
		}
		for _, prodIdx := range cache.prodsByLHS[target] {
			if prodIdx < 0 || prodIdx >= len(ng.Productions) {
				continue
			}
			candidate := &ng.Productions[prodIdx]
			if candidate.LHS != target || len(candidate.RHS) < 2 || !candidate.HasExplicitPrec {
				continue
			}
			finalOperand := reduceProd.RHS[len(reduceProd.RHS)-1]
			if candidate.RHS[0] != finalOperand {
				continue
			}
			tail := candidate.RHS[1:]
			if len(tail) == 1 && singleTailSymbolIsPostfixLikeContinuation(tail[0], lookaheadTargets, finalOperand, cache, ng) {
				return shift, true
			}
		}
	}
	return lrAction{}, false
}

func preferredRightAssocSameLHSOptionalContinuationShift(lookaheadSym int, shifts, reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) (lrAction, bool) {
	if ng == nil || cache == nil || len(shifts) != 1 || len(reduces) != 1 ||
		lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) {
		return lrAction{}, false
	}
	shift := shifts[0]
	reduce := reduces[0]
	if shift.kind != lrShift || reduce.kind != lrReduce ||
		reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
		return lrAction{}, false
	}
	reduceProd := &ng.Productions[reduce.prodIdx]
	if len(reduceProd.RHS) == 0 || reduceProd.LHS < 0 || reduceProd.LHS >= len(cache.prodsByLHS) {
		return lrAction{}, false
	}
	lookaheadTargets := map[int]bool{lookaheadSym: true}
	finalOperand := reduceProd.RHS[len(reduceProd.RHS)-1]
	for _, prodIdx := range cache.prodsByLHS[reduceProd.LHS] {
		if prodIdx < 0 || prodIdx >= len(ng.Productions) || prodIdx == reduce.prodIdx {
			continue
		}
		candidate := &ng.Productions[prodIdx]
		if candidate.LHS != reduceProd.LHS || !candidate.HasExplicitPrec ||
			candidate.Assoc != AssocRight ||
			len(candidate.RHS) <= len(reduceProd.RHS) ||
			!rhsHasPrefix(candidate.RHS, reduceProd.RHS) {
			continue
		}
		suffix := candidate.RHS[len(reduceProd.RHS):]
		if rhsIsSafeRightAssocPostfixContinuation(suffix, lookaheadTargets, finalOperand, cache, ng) {
			return shift, true
		}
	}
	return lrAction{}, false
}

func rhsIsSafeRightAssocPostfixContinuation(rhs []int, lookaheadTargets map[int]bool, finalOperand int, cache *conflictResolutionCache, ng *NormalizedGrammar) bool {
	if len(rhs) == 0 {
		return false
	}
	visiting := make([]bool, len(ng.Symbols))
	matched, safe := rhsIsPostfixLikeContinuation(rhs, lookaheadTargets, finalOperand, cache, ng, visiting)
	return matched && safe
}

func singleTailSymbolIsPostfixLikeContinuation(sym int, lookaheadTargets map[int]bool, finalOperand int, cache *conflictResolutionCache, ng *NormalizedGrammar) bool {
	if sym < 0 || ng == nil || sym >= len(ng.Symbols) {
		return false
	}
	if ng.Symbols[sym].Kind != SymbolNonterminal {
		return lookaheadTargets[sym]
	}
	visiting := make([]bool, len(ng.Symbols))
	matched, safe := symbolHasPostfixLikeContinuation(sym, lookaheadTargets, finalOperand, cache, ng, visiting)
	return matched && safe
}

func symbolHasPostfixLikeContinuation(sym int, lookaheadTargets map[int]bool, finalOperand int, cache *conflictResolutionCache, ng *NormalizedGrammar, visiting []bool) (bool, bool) {
	if cache == nil || sym < 0 || sym >= len(ng.Symbols) || sym >= len(cache.prodsByLHS) {
		return false, true
	}
	if ng.Symbols[sym].Kind != SymbolNonterminal {
		return lookaheadTargets[sym], true
	}
	if visiting[sym] {
		return false, true
	}
	visiting[sym] = true
	defer func() { visiting[sym] = false }()

	matched := false
	for _, prodIdx := range cache.prodsByLHS[sym] {
		if prodIdx < 0 || prodIdx >= len(ng.Productions) {
			continue
		}
		prodMatched, prodSafe := rhsIsPostfixLikeContinuation(ng.Productions[prodIdx].RHS, lookaheadTargets, finalOperand, cache, ng, visiting)
		if !prodMatched {
			continue
		}
		if !prodSafe {
			return true, false
		}
		matched = true
	}
	return matched, true
}

func rhsIsPostfixLikeContinuation(rhs []int, lookaheadTargets map[int]bool, finalOperand int, cache *conflictResolutionCache, ng *NormalizedGrammar, visiting []bool) (bool, bool) {
	if len(rhs) == 0 {
		return false, true
	}
	first := rhs[0]
	if first < 0 || ng == nil || first >= len(ng.Symbols) {
		return false, true
	}
	if lookaheadTargets[first] {
		if symbolIsPostfixOpener(first, ng) {
			return true, true
		}
		finalOperandTargets := map[int]bool{finalOperand: true}
		return true, !rhsCanBeginWithAny(rhs[1:], finalOperandTargets, cache, ng)
	}
	if ng.Symbols[first].Kind != SymbolNonterminal {
		return false, true
	}
	if len(rhs) != 1 {
		return false, true
	}
	return symbolHasPostfixLikeContinuation(first, lookaheadTargets, finalOperand, cache, ng, visiting)
}

func symbolIsPostfixOpener(sym int, ng *NormalizedGrammar) bool {
	if sym < 0 || ng == nil || sym >= len(ng.Symbols) {
		return false
	}
	switch ng.Symbols[sym].Name {
	case "(", "[", "{":
		return true
	default:
		return false
	}
}

func shiftHasHigherPrecedenceThanReduceHelper(shift lrAction, reduceProd *Production) bool {
	if reduceProd == nil {
		return false
	}
	return shift.prec > reduceProd.Prec
}

func shouldKeepRepeatHelperReduceWithUnrelatedShift(lookaheadSym int, shifts, reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if ng == nil || cache == nil || len(shifts) != 1 || len(reduces) != 1 ||
		lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) {
		return false
	}
	shift := shifts[0]
	reduce := reduces[0]
	if shift.kind != lrShift || reduce.kind != lrReduce ||
		reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) ||
		!isRepeatHelperReduce(reduce, ng, cache) {
		return false
	}
	if shift.repeat {
		return false
	}
	prod := &ng.Productions[reduce.prodIdx]
	if repeatHelperReduceContinuesWithShift(lookaheadSym, reduce, shift, ng, cache) {
		return false
	}
	if repeatHelperReduceSharesShiftFamily(prod.LHS, shift, ng, cache) {
		return false
	}
	return true
}

func repeatElementReduceActions(lookaheadSym int, actions, shifts, reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) ([]lrAction, bool) {
	if ng == nil || cache == nil || len(shifts) != 1 || len(reduces) != 1 ||
		lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) {
		return nil, false
	}
	shift := shifts[0]
	reduce := reduces[0]
	if shift.kind != lrShift || reduce.kind != lrReduce ||
		reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) ||
		isRepeatHelperReduce(reduce, ng, cache) {
		return nil, false
	}
	prod := &ng.Productions[reduce.prodIdx]
	if prod.LHS < 0 || prod.LHS >= len(ng.Symbols) {
		return nil, false
	}
	if !repeatElementCanStartAdjacentRepeatOnLookahead(prod.LHS, lookaheadSym, ng, cache) {
		return nil, false
	}
	if repeatElementShiftSharesFamily(prod.LHS, shift, ng, cache) {
		if repeatElementReduceVetoedByHigherPrecedenceContinuation(lookaheadSym, shift, reduce, ng, cache) &&
			!shiftReduceInConflictGroup(shifts, reduces, ng, cache) {
			return nil, false
		}
		return actions, true
	}
	return []lrAction{reduce}, true
}

func repeatElementReduceVetoedByHigherPrecedenceContinuation(lookaheadSym int, shift, reduce lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if ng == nil || cache == nil ||
		lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) ||
		reduce.kind != lrReduce || reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
		return false
	}
	reduceProd := &ng.Productions[reduce.prodIdx]
	targets := shiftContinuationTargets(shift, len(ng.Symbols))
	if len(targets) == 0 {
		return false
	}
	lookaheadTargets := map[int]bool{lookaheadSym: true}
	for lhs := range targets {
		if lhs < 0 || lhs >= len(cache.prodsByLHS) {
			continue
		}
		for _, prodIdx := range cache.prodsByLHS[lhs] {
			if prodIdx < 0 || prodIdx >= len(ng.Productions) {
				continue
			}
			candidate := &ng.Productions[prodIdx]
			if !candidate.HasExplicitPrec || candidate.Prec <= reduceProd.Prec ||
				len(candidate.RHS) <= len(reduceProd.RHS) ||
				!rhsHasPrefix(candidate.RHS, reduceProd.RHS) {
				continue
			}
			if rhsCanBeginWithAny(candidate.RHS[len(reduceProd.RHS):], lookaheadTargets, cache, ng) {
				return true
			}
		}
	}
	return false
}

func repeatElementCanStartAdjacentRepeatOnLookahead(elemLHS, lookaheadSym int, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	return repeatElementCanStartAdjacentRepeatOnLookaheadCtx(context.Background(), elemLHS, lookaheadSym, ng, cache)
}

func repeatElementCanStartAdjacentRepeatOnLookaheadCtx(ctx context.Context, elemLHS, lookaheadSym int, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if ng == nil || cache == nil ||
		elemLHS < 0 || elemLHS >= len(cache.rhsParents) ||
		lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) {
		return false
	}
	if err := cache.ensureRepeatStartLookaheadSets(ctx, ng); err != nil {
		return false
	}
	if elemLHS >= len(cache.repeatStartLookaheadSets) {
		return false
	}
	return bitsetHas(cache.repeatStartLookaheadSets[elemLHS], lookaheadSym)
}

func repeatElementShiftSharesFamily(elemLHS int, shift lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	targets := shiftContinuationTargets(shift, len(ng.Symbols))
	if len(targets) == 0 {
		return false
	}
	if targets[elemLHS] {
		return true
	}
	return rhsCanBeginWithAny([]int{elemLHS}, targets, cache, ng)
}

func rhsCanStartWithSymbol(rhs []int, target int, cache *conflictResolutionCache) bool {
	for _, sym := range rhs {
		if sym == target {
			return true
		}
		if sym < 0 || sym >= len(cache.nullable) || !cache.nullable[sym] {
			return false
		}
	}
	return false
}

func (cache *conflictResolutionCache) ensureRepeatStartLookaheadSets(ctx context.Context, ng *NormalizedGrammar) error {
	if cache == nil || ng == nil {
		return nil
	}
	if cache.repeatStartLookaheadComputed {
		return nil
	}
	firstSets, err := cache.ensureFirstSets(ctx, ng)
	if err != nil {
		return err
	}
	wordCount := bitsetWordCount(len(ng.Symbols))
	repeatStart := make([][]uint64, len(ng.Symbols))
	checks := 0
	for elemLHS := range ng.Symbols {
		if elemLHS&255 == 0 {
			if err := checkConflictResolutionContext(ctx, "precomputing repeat start lookaheads"); err != nil {
				return err
			}
		}
		for _, repeatSym := range cache.rhsParents[elemLHS] {
			checks++
			if checks&1023 == 0 {
				if err := checkConflictResolutionContext(ctx, "precomputing repeat start lookahead parents"); err != nil {
					return err
				}
			}
			if repeatSym < 0 || repeatSym >= len(cache.prodsByLHS) ||
				!isStructurallyGeneratedRepeatHelper(repeatSym, ng, cache) {
				continue
			}
			for _, prodIdx := range cache.prodsByLHS[repeatSym] {
				if prodIdx < 0 || prodIdx >= len(ng.Productions) {
					continue
				}
				rhs := ng.Productions[prodIdx].RHS
				if len(rhs) > 0 && rhs[0] == repeatSym {
					rhs = rhs[1:]
				}
				if !rhsCanStartWithSymbol(rhs, elemLHS, cache) {
					continue
				}
				if repeatStart[elemLHS] == nil {
					repeatStart[elemLHS] = make([]uint64, wordCount)
				}
				cache.orRHSFirstSet(repeatStart[elemLHS], rhs, firstSets)
			}
		}
	}
	cache.repeatStartLookaheadSets = repeatStart
	cache.repeatStartLookaheadComputed = true
	return nil
}

func (cache *conflictResolutionCache) ensureFirstSets(ctx context.Context, ng *NormalizedGrammar) ([][]uint64, error) {
	if cache == nil || ng == nil {
		return nil, nil
	}
	if cache.firstSets != nil {
		return cache.firstSets, nil
	}
	wordCount := bitsetWordCount(len(ng.Symbols))
	firstSets := make([][]uint64, len(ng.Symbols))
	for sym := range firstSets {
		firstSets[sym] = make([]uint64, wordCount)
		bitsetSet(firstSets[sym], sym)
	}
	changed := true
	for changed {
		if err := checkConflictResolutionContext(ctx, "precomputing conflict FIRST sets"); err != nil {
			return nil, err
		}
		changed = false
		for prodIdx := range ng.Productions {
			if prodIdx&1023 == 0 {
				if err := checkConflictResolutionContext(ctx, "precomputing conflict FIRST sets"); err != nil {
					return nil, err
				}
			}
			prod := &ng.Productions[prodIdx]
			if prod.LHS < 0 || prod.LHS >= len(firstSets) {
				continue
			}
			if cache.orRHSFirstSet(firstSets[prod.LHS], prod.RHS, firstSets) {
				changed = true
			}
		}
	}
	cache.firstSets = firstSets
	return firstSets, nil
}

func (cache *conflictResolutionCache) orRHSFirstSet(dst []uint64, rhs []int, firstSets [][]uint64) bool {
	changed := false
	for _, sym := range rhs {
		if sym < 0 || sym >= len(firstSets) {
			return changed
		}
		if bitsetOr(dst, firstSets[sym]) {
			changed = true
		}
		if sym >= len(cache.nullable) || !cache.nullable[sym] {
			return changed
		}
	}
	return changed
}

func bitsetWordCount(bits int) int {
	if bits <= 0 {
		return 0
	}
	return (bits + 63) >> 6
}

func bitsetSet(words []uint64, bit int) {
	if bit < 0 {
		return
	}
	word := bit >> 6
	if word < 0 || word >= len(words) {
		return
	}
	words[word] |= uint64(1) << uint(bit&63)
}

func bitsetHas(words []uint64, bit int) bool {
	if bit < 0 {
		return false
	}
	word := bit >> 6
	if word < 0 || word >= len(words) {
		return false
	}
	return words[word]&(uint64(1)<<uint(bit&63)) != 0
}

func bitsetOr(dst, src []uint64) bool {
	changed := false
	for i := range dst {
		before := dst[i]
		dst[i] |= src[i]
		if dst[i] != before {
			changed = true
		}
	}
	return changed
}

func repeatHelperReduceSharesShiftFamily(reduceLHS int, shift lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if reduceLHS < 0 || reduceLHS >= len(ng.Symbols) {
		return false
	}
	if shift.hasRepeatLHS(reduceLHS) {
		return true
	}
	reduceFamily := make(map[int]bool)
	for _, sym := range resolveAuxToParents(reduceLHS, ng, cache) {
		reduceFamily[sym] = true
	}
	for sym := range shiftContinuationTargets(shift, len(ng.Symbols)) {
		if sym == reduceLHS || reduceFamily[sym] {
			return true
		}
		for _, parent := range resolveAuxToParents(sym, ng, cache) {
			if reduceFamily[parent] {
				return true
			}
		}
	}
	return false
}

func leftAssocReduceContinuesWithShift(reduce, shift lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if reduce.kind != lrReduce || reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
		return false
	}
	prod := &ng.Productions[reduce.prodIdx]
	if prod.Assoc != AssocLeft || prod.LHS < 0 || prod.LHS >= len(ng.Symbols) {
		return false
	}
	targets := shiftContinuationTargets(shift, len(ng.Symbols))
	if len(targets) == 0 {
		return false
	}
	if cache == nil || prod.LHS >= len(cache.prodsByLHS) {
		return false
	}
	for _, prodIdx := range cache.prodsByLHS[prod.LHS] {
		if prodIdx < 0 || prodIdx >= len(ng.Productions) {
			continue
		}
		candidate := &ng.Productions[prodIdx]
		if candidate.LHS != prod.LHS || len(candidate.RHS) <= len(prod.RHS) {
			continue
		}
		if !rhsHasPrefix(candidate.RHS, prod.RHS) {
			continue
		}
		if rhsCanBeginWithAny(candidate.RHS[len(prod.RHS):], targets, cache, ng) {
			return true
		}
	}
	return false
}

func preferredLoweredRepeatContinuationShift(lookaheadSym int, shifts, reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) (lrAction, bool) {
	if ng == nil || cache == nil || len(shifts) != 1 || len(reduces) != 1 ||
		lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) {
		return lrAction{}, false
	}
	shift := shifts[0]
	reduce := reduces[0]
	if shift.kind != lrShift || reduce.kind != lrReduce ||
		reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
		return lrAction{}, false
	}
	prod := &ng.Productions[reduce.prodIdx]
	if prod.Assoc != AssocLeft || prod.LHS < 0 || prod.LHS >= len(cache.prodsByLHS) || len(prod.RHS) < 2 {
		return lrAction{}, false
	}
	lookaheadTargets := map[int]bool{lookaheadSym: true}
	shiftTargets := shiftContinuationTargets(shift, len(ng.Symbols))
	if len(shiftTargets) == 0 {
		return lrAction{}, false
	}
	for split := 1; split < len(prod.RHS); split++ {
		prefix := prod.RHS[:split]
		unit := prod.RHS[split:]
		unitBeginsShift := rhsCanBeginWithAny(unit, lookaheadTargets, cache, ng) &&
			rhsCanBeginWithAny(unit, shiftTargets, cache, ng)
		if unitBeginsShift && loweredRepeatSiblingContinuesUnit(prod.LHS, prefix, unit, ng, cache) {
			return shift, true
		}
		if loweredRepeatSiblingContinuesPartialUnit(prod.LHS, prefix, unit, lookaheadTargets, shiftTargets, ng, cache) {
			return shift, true
		}
	}
	return lrAction{}, false
}

// loweredRepeatSiblingContinuesUnit recognizes grammar.js repeat lowering of a
// left-associative production A -> P U into a sibling A -> P R where repeat
// helper R starts with the same continuation unit U. In that shape, shifting U
// continues A rather than starting an unrelated operator expression.
func loweredRepeatSiblingContinuesUnit(lhs int, prefix, unit []int, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	for _, prodIdx := range cache.prodsByLHS[lhs] {
		if prodIdx < 0 || prodIdx >= len(ng.Productions) {
			continue
		}
		candidate := &ng.Productions[prodIdx]
		if candidate.LHS != lhs || len(candidate.RHS) != len(prefix)+1 {
			continue
		}
		if !rhsHasPrefix(candidate.RHS, prefix) {
			continue
		}
		repeatSym := candidate.RHS[len(prefix)]
		if repeatSym < 0 || repeatSym >= len(ng.Symbols) ||
			ng.Symbols[repeatSym].Kind != SymbolNonterminal {
			continue
		}
		if repeatHelperCanBeginUnit(repeatSym, unit, ng, cache) {
			return true
		}
	}
	return false
}

// loweredRepeatSiblingContinuesPartialUnit recognizes the state reached after a
// prefix of a lowered repeat unit has already been consumed. For a source shape
// like A -> P repeat(seq(Op, U)), normalization can leave a conflict while
// reducing A -> P Op U with lookahead in FIRST(U). Shifting continues the
// partially consumed repeat unit; reducing A would prematurely close the parent.
func loweredRepeatSiblingContinuesPartialUnit(lhs int, consumedPrefix, unit []int, lookaheadTargets, shiftTargets map[int]bool, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if len(consumedPrefix) < 2 || len(unit) == 0 {
		return false
	}
	for baseLen := 1; baseLen < len(consumedPrefix); baseLen++ {
		basePrefix := consumedPrefix[:baseLen]
		repeatPrefix := consumedPrefix[baseLen:]
		if loweredRepeatSiblingContinuesPrefixedUnit(lhs, basePrefix, repeatPrefix, unit, lookaheadTargets, shiftTargets, ng, cache) {
			return true
		}
	}
	return false
}

func loweredRepeatSiblingContinuesPrefixedUnit(lhs int, basePrefix, repeatPrefix, unit []int, lookaheadTargets, shiftTargets map[int]bool, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	for _, prodIdx := range cache.prodsByLHS[lhs] {
		if prodIdx < 0 || prodIdx >= len(ng.Productions) {
			continue
		}
		candidate := &ng.Productions[prodIdx]
		if candidate.LHS != lhs || len(candidate.RHS) != len(basePrefix)+1 {
			continue
		}
		if !rhsHasPrefix(candidate.RHS, basePrefix) {
			continue
		}
		repeatSym := candidate.RHS[len(basePrefix)]
		if repeatSym < 0 || repeatSym >= len(ng.Symbols) ||
			ng.Symbols[repeatSym].Kind != SymbolNonterminal {
			continue
		}
		seq := make([]int, 0, len(repeatPrefix)+len(unit))
		seq = append(seq, repeatPrefix...)
		seq = append(seq, unit...)
		if repeatHelperCanBeginSequence(repeatSym, seq, ng, cache) &&
			rhsCanContinueWithAny(unit, lookaheadTargets, shiftTargets, cache, ng) {
			return true
		}
	}
	return false
}

func repeatHelperCanBeginUnit(repeatSym int, unit []int, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	return repeatHelperCanBeginSequence(repeatSym, unit, ng, cache)
}

func repeatHelperCanBeginSequence(repeatSym int, seq []int, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if repeatSym < 0 || repeatSym >= len(cache.prodsByLHS) || len(seq) == 0 {
		return false
	}
	if !isStructurallyGeneratedRepeatHelper(repeatSym, ng, cache) {
		return false
	}
	for _, prodIdx := range cache.prodsByLHS[repeatSym] {
		if prodIdx < 0 || prodIdx >= len(ng.Productions) {
			continue
		}
		rhs := ng.Productions[prodIdx].RHS
		if rhsHasPrefix(rhs, seq) {
			return true
		}
		if len(rhs) > 0 && rhs[0] == repeatSym && rhsHasPrefix(rhs[1:], seq) {
			return true
		}
	}
	return false
}

func isStructurallyGeneratedRepeatHelper(sym int, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if ng == nil || cache == nil ||
		sym < 0 || sym >= len(ng.Symbols) ||
		sym >= len(cache.prodsByLHS) ||
		ng.Symbols[sym].Kind != SymbolNonterminal {
		return false
	}
	if sym < len(cache.structuralRepeatHelperMemo) {
		switch cache.structuralRepeatHelperMemo[sym] {
		case 1:
			return true
		case -1:
			return false
		}
	}
	result := computeStructurallyGeneratedRepeatHelper(sym, ng, cache)
	if sym < len(cache.structuralRepeatHelperMemo) {
		if result {
			cache.structuralRepeatHelperMemo[sym] = 1
		} else {
			cache.structuralRepeatHelperMemo[sym] = -1
		}
	}
	return result
}

func computeStructurallyGeneratedRepeatHelper(sym int, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if ng.Symbols[sym].Visible || ng.Symbols[sym].Named {
		return false
	}
	if !ng.Symbols[sym].GeneratedRepeatAux {
		return false
	}

	var baseRHS [][]int
	var recursiveTails [][]int
	hasBinaryRecursive := false
	for _, prodIdx := range cache.prodsByLHS[sym] {
		if prodIdx < 0 || prodIdx >= len(ng.Productions) {
			continue
		}
		rhs := ng.Productions[prodIdx].RHS
		if len(rhs) == 0 {
			return false
		}
		if rhs[0] == sym {
			if len(rhs) < 2 {
				return false
			}
			if len(rhs) == 2 && rhs[1] == sym {
				hasBinaryRecursive = true
				continue
			}
			recursiveTails = append(recursiveTails, rhs[1:])
			continue
		}
		baseRHS = append(baseRHS, rhs)
	}
	if len(baseRHS) == 0 {
		return false
	}
	if hasBinaryRecursive {
		return true
	}
	if len(recursiveTails) == 0 {
		return false
	}
	for _, tail := range recursiveTails {
		if !repeatHelperHasBasePrefix(baseRHS, tail) {
			return false
		}
	}
	return true
}

func repeatHelperHasBasePrefix(baseRHS [][]int, prefix []int) bool {
	if len(prefix) == 0 {
		return false
	}
	for _, rhs := range baseRHS {
		if rhsHasPrefix(rhs, prefix) {
			return true
		}
	}
	return false
}

// loweredRepeatMixedContinuationActions handles the same lowered-repeat shape
// when a conflict row contains both the visible parent reduce and the repeat
// helper tail reduce. Keeping the helper reduce plus a repeat-marked shift
// matches tree-sitter's repeat continuation behavior while dropping the parent
// reduce that would prematurely materialize the visible node.
func loweredRepeatMixedContinuationActions(lookaheadSym int, shifts, reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) ([]lrAction, bool) {
	if ng == nil || cache == nil || len(shifts) != 1 || len(reduces) != 2 ||
		lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) {
		return nil, false
	}
	shift := shifts[0]
	if shift.kind != lrShift {
		return nil, false
	}
	var parentReduce, repeatReduce lrAction
	var haveParentReduce, haveRepeatReduce bool
	for _, reduce := range reduces {
		if reduce.kind != lrReduce || reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
			return nil, false
		}
		if isRepeatHelperReduce(reduce, ng, cache) {
			if haveRepeatReduce {
				return nil, false
			}
			repeatReduce = reduce
			haveRepeatReduce = true
		} else {
			if haveParentReduce {
				return nil, false
			}
			parentReduce = reduce
			haveParentReduce = true
		}
	}
	if !haveParentReduce || !haveRepeatReduce {
		return nil, false
	}
	parent := &ng.Productions[parentReduce.prodIdx]
	repeat := &ng.Productions[repeatReduce.prodIdx]
	if parent.Assoc != AssocLeft || parent.LHS < 0 || parent.LHS >= len(cache.prodsByLHS) ||
		repeat.LHS < 0 || repeat.LHS >= len(ng.Symbols) || len(parent.RHS) < 2 {
		return nil, false
	}
	lookaheadTargets := map[int]bool{lookaheadSym: true}
	shiftTargets := shiftContinuationTargets(shift, len(ng.Symbols))
	if len(shiftTargets) == 0 {
		return nil, false
	}
	for split := 1; split < len(parent.RHS); split++ {
		prefix := parent.RHS[:split]
		unit := parent.RHS[split:]
		if !rhsCanBeginWithAny(unit, lookaheadTargets, cache, ng) ||
			!rhsCanBeginWithAny(unit, shiftTargets, cache, ng) ||
			!loweredRepeatSiblingMatches(parent.LHS, prefix, repeat.LHS, ng, cache) ||
			!repeatReduceContinuesUnit(repeat, unit) {
			continue
		}
		shift.addRepeatLHS(repeat.LHS)
		if resolved, ok := resolveLoweredRepeatHelperShiftReduce(repeatReduce, shift, parent); ok {
			return resolved, true
		}
		return []lrAction{repeatReduce, shift}, true
	}
	return nil, false
}

func loweredRepeatHelperContinuationActions(lookaheadSym int, shifts, reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) ([]lrAction, bool) {
	if ng == nil || cache == nil || len(shifts) != 1 || len(reduces) != 1 ||
		lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) {
		return nil, false
	}
	shift := shifts[0]
	reduce := reduces[0]
	if shift.kind != lrShift {
		return nil, false
	}
	if !shift.repeat {
		if parent, ok := loweredRepeatHelperVisibleParentProduction(reduce, ng, cache); ok &&
			repeatHelperReduceContinuesWithFamilyShift(lookaheadSym, reduce, shift, ng, cache) {
			if resolved, resolvedOK := resolveLoweredRepeatHelperShiftReduce(reduce, shift, parent); resolvedOK {
				return resolved, true
			}
		}
		return nil, false
	}
	if !repeatHelperReduceContinuesWithShift(lookaheadSym, reduce, shift, ng, cache) {
		return nil, false
	}
	if parent, ok := loweredRepeatHelperVisibleParentProduction(reduce, ng, cache); ok {
		if resolved, resolvedOK := resolveLoweredRepeatHelperShiftReduce(reduce, shift, parent); resolvedOK {
			return resolved, true
		}
	}
	return []lrAction{reduce, shift}, true
}

func repeatHelperReduceContinuesWithFamilyShift(lookaheadSym int, reduce, shift lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if reduce.kind != lrReduce || reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) ||
		lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) {
		return false
	}
	prod := &ng.Productions[reduce.prodIdx]
	if prod.LHS < 0 || prod.LHS >= len(cache.prodsByLHS) ||
		!isStructurallyGeneratedRepeatHelper(prod.LHS, ng, cache) ||
		!repeatHelperReduceSharesShiftFamily(prod.LHS, shift, ng, cache) {
		return false
	}
	lookaheadTargets := map[int]bool{lookaheadSym: true}
	shiftTargets := shiftContinuationTargets(shift, len(ng.Symbols))
	return repeatHelperCanContinueWithAny(prod.LHS, lookaheadTargets, shiftTargets, ng, cache)
}

func resolveLoweredRepeatHelperShiftReduce(reduce, shift lrAction, parent *Production) ([]lrAction, bool) {
	if parent == nil {
		return nil, false
	}
	if shift.prec == 0 && parent.Prec == 0 {
		return nil, false
	}
	if shift.prec > parent.Prec {
		return []lrAction{shift}, true
	}
	if shift.prec < parent.Prec {
		return []lrAction{reduce}, true
	}
	switch parent.Assoc {
	case AssocLeft:
		return []lrAction{reduce}, true
	case AssocRight:
		return []lrAction{shift}, true
	default:
		if shift.hasPrec || parent.HasExplicitPrec || shift.prec != 0 || parent.Prec != 0 {
			return nil, false
		}
		return nil, false
	}
}

func loweredRepeatHelperVisibleParentProduction(reduce lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) (*Production, bool) {
	if reduce.kind != lrReduce || reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
		return nil, false
	}
	repeat := &ng.Productions[reduce.prodIdx]
	if repeat.LHS < 0 || repeat.LHS >= len(cache.rhsParents) ||
		!isStructurallyGeneratedRepeatHelper(repeat.LHS, ng, cache) {
		return nil, false
	}
	var best *Production
	for _, parentLHS := range cache.rhsParents[repeat.LHS] {
		if parentLHS < 0 || parentLHS >= len(cache.prodsByLHS) ||
			parentLHS == repeat.LHS ||
			parentLHS >= len(ng.Symbols) ||
			!ng.Symbols[parentLHS].Visible {
			continue
		}
		for _, prodIdx := range cache.prodsByLHS[parentLHS] {
			if prodIdx < 0 || prodIdx >= len(ng.Productions) {
				continue
			}
			parent := &ng.Productions[prodIdx]
			if parent.LHS != parentLHS || !rhsContainsSymbol(parent.RHS, repeat.LHS) {
				continue
			}
			if best == nil ||
				parent.Prec > best.Prec ||
				(parent.Prec == best.Prec && best.Assoc == AssocNone && parent.Assoc != AssocNone) {
				best = parent
			}
		}
	}
	if best == nil {
		return nil, false
	}
	return best, true
}

func visibleLoweredRepeatBodyContinuationActions(lookaheadSym int, actions, shifts, reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) ([]lrAction, bool) {
	if ng == nil || cache == nil || len(shifts) != 1 || len(reduces) != 1 ||
		lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) {
		return nil, false
	}
	shift := shifts[0]
	reduce := reduces[0]
	if shift.kind != lrShift || reduce.kind != lrReduce ||
		reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
		return nil, false
	}
	prod := &ng.Productions[reduce.prodIdx]
	if prod.LHS < 0 || prod.LHS >= len(ng.Symbols) ||
		!ng.Symbols[prod.LHS].Visible ||
		len(prod.RHS) < 2 {
		return nil, false
	}
	lookaheadTargets := map[int]bool{lookaheadSym: true}
	shiftTargets := shiftContinuationTargets(shift, len(ng.Symbols))
	if len(shiftTargets) == 0 {
		return nil, false
	}
	prefix := prod.RHS[:len(prod.RHS)-1]
	tailSym := prod.RHS[len(prod.RHS)-1]
	if isStructurallyGeneratedRepeatHelper(tailSym, ng, cache) {
		if repeatHelperCanContinueWithAny(tailSym, lookaheadTargets, shiftTargets, ng, cache) {
			if repeatElementCanStartAdjacentRepeatOnLookahead(prod.LHS, lookaheadSym, ng, cache) {
				return actions, true
			}
			if prod.HasExplicitPrec || prod.Assoc != AssocNone || shift.hasPrec || shift.prec != 0 {
				return []lrAction{shift}, true
			}
			return actions, true
		}
		return nil, false
	}
	unit := prod.RHS[len(prod.RHS)-1:]
	if !rhsCanBeginWithAny(unit, lookaheadTargets, cache, ng) ||
		!rhsCanBeginWithAny(unit, shiftTargets, cache, ng) {
		return nil, false
	}
	for _, prodIdx := range cache.prodsByLHS[prod.LHS] {
		if prodIdx < 0 || prodIdx >= len(ng.Productions) || prodIdx == reduce.prodIdx {
			continue
		}
		candidate := &ng.Productions[prodIdx]
		if candidate.LHS != prod.LHS || len(candidate.RHS) != len(prefix)+1 ||
			!rhsHasPrefix(candidate.RHS, prefix) {
			continue
		}
		repeatSym := candidate.RHS[len(prefix)]
		if isStructurallyGeneratedRepeatHelper(repeatSym, ng, cache) &&
			repeatHelperCanBeginSequence(repeatSym, unit, ng, cache) {
			return actions, true
		}
	}
	return nil, false
}

func repeatHelperReduceContinuesWithShift(lookaheadSym int, reduce, shift lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if cache != nil {
		cache.structuralStats.RepeatHelperReduceShiftCalls++
	}
	if reduce.kind != lrReduce || reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
		return false
	}
	prod := &ng.Productions[reduce.prodIdx]
	if prod.LHS < 0 || prod.LHS >= len(cache.prodsByLHS) ||
		len(prod.RHS) == 0 ||
		!shift.hasRepeatLHS(prod.LHS) ||
		!isStructurallyGeneratedRepeatHelper(prod.LHS, ng, cache) {
		return false
	}
	lookaheadTargets := map[int]bool{lookaheadSym: true}
	shiftTargets := shiftContinuationTargets(shift, len(ng.Symbols))
	if len(shiftTargets) == 0 {
		return false
	}
	for _, prodIdx := range cache.prodsByLHS[prod.LHS] {
		if prodIdx < 0 || prodIdx >= len(ng.Productions) {
			continue
		}
		recursive := &ng.Productions[prodIdx]
		if recursive.LHS != prod.LHS || len(recursive.RHS) < 2 || recursive.RHS[0] != prod.LHS {
			continue
		}
		tail := recursive.RHS[1:]
		if rhsCanBeginWithAny(tail, lookaheadTargets, cache, ng) &&
			rhsCanBeginWithAny(tail, shiftTargets, cache, ng) {
			return true
		}
	}
	return false
}

func repeatHelperCanContinueWithAny(repeatSym int, lookaheadTargets, shiftTargets map[int]bool, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if cache != nil {
		cache.structuralStats.RepeatHelperContinueCalls++
	}
	if repeatSym < 0 || repeatSym >= len(cache.prodsByLHS) ||
		!isStructurallyGeneratedRepeatHelper(repeatSym, ng, cache) {
		return false
	}
	for _, prodIdx := range cache.prodsByLHS[repeatSym] {
		if prodIdx < 0 || prodIdx >= len(ng.Productions) {
			continue
		}
		recursive := &ng.Productions[prodIdx]
		if recursive.LHS != repeatSym || len(recursive.RHS) < 2 || recursive.RHS[0] != repeatSym {
			continue
		}
		tail := recursive.RHS[1:]
		if rhsCanBeginWithAny(tail, lookaheadTargets, cache, ng) &&
			rhsCanBeginWithAny(tail, shiftTargets, cache, ng) {
			return true
		}
	}
	return false
}

func loweredRepeatSiblingMatches(lhs int, prefix []int, repeatSym int, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if lhs < 0 || lhs >= len(cache.prodsByLHS) ||
		repeatSym < 0 || repeatSym >= len(ng.Symbols) ||
		ng.Symbols[repeatSym].Kind != SymbolNonterminal ||
		!isStructurallyGeneratedRepeatHelper(repeatSym, ng, cache) {
		return false
	}
	for _, prodIdx := range cache.prodsByLHS[lhs] {
		if prodIdx < 0 || prodIdx >= len(ng.Productions) {
			continue
		}
		candidate := &ng.Productions[prodIdx]
		if candidate.LHS == lhs && len(candidate.RHS) == len(prefix)+1 &&
			rhsHasPrefix(candidate.RHS, prefix) && candidate.RHS[len(prefix)] == repeatSym {
			return true
		}
	}
	return false
}

func repeatReduceContinuesUnit(repeat *Production, unit []int) bool {
	if repeat == nil || len(unit) == 0 {
		return false
	}
	if rhsHasPrefix(repeat.RHS, unit) {
		return true
	}
	return len(repeat.RHS) > 0 && repeat.RHS[0] == repeat.LHS && rhsHasPrefix(repeat.RHS[1:], unit)
}

func shiftContinuationTargets(shift lrAction, symbolCount int) map[int]bool {
	targets := make(map[int]bool)
	if shift.lhsSym >= 0 && shift.lhsSym < symbolCount {
		targets[shift.lhsSym] = true
	}
	for _, lhs := range shift.lhsSyms {
		if lhs >= 0 && lhs < symbolCount {
			targets[lhs] = true
		}
	}
	return targets
}

type lrConflictMetadata struct {
	prec    int
	hasPrec bool
	assoc   Assoc
}

func shiftMetadataForReduce(shift lrAction, reduceLHS int, ng *NormalizedGrammar, cache *conflictResolutionCache) lrConflictMetadata {
	fallback := lrConflictMetadata{prec: shift.prec, hasPrec: shift.hasPrec, assoc: shift.assoc}
	if shift.kind != lrShift || ng == nil || reduceLHS < 0 || reduceLHS >= len(ng.Symbols) {
		return fallback
	}
	contributors := shift.shiftContributors
	if len(contributors) == 0 {
		contributors = append(contributors, lrShiftContributor{lhsSym: shift.lhsSym, prec: shift.prec, hasPrec: shift.hasPrec, assoc: shift.assoc})
		for _, lhs := range shift.lhsSyms {
			contributors = append(contributors, lrShiftContributor{lhsSym: lhs, prec: shift.prec, hasPrec: shift.hasPrec, assoc: shift.assoc})
		}
	}
	primary := fallback
	for _, contributor := range contributors {
		if contributor.lhsSym == shift.lhsSym {
			primary = lrConflictMetadata{prec: contributor.prec, hasPrec: contributor.hasPrec, assoc: contributor.assoc}
			break
		}
	}
	best := primary
	found := false
	for _, contributor := range contributors {
		if !shiftContributorMatchesReduce(contributor.lhsSym, reduceLHS, ng, cache) {
			continue
		}
		candidate := lrConflictMetadata{prec: contributor.prec, hasPrec: contributor.hasPrec, assoc: contributor.assoc}
		if compareConflictMetadata(candidate, best) > 0 {
			best = candidate
		}
		found = true
	}
	if found {
		return best
	}
	return fallback
}

func reduceMetadataForShiftConflict(reduce lrAction, shift lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) lrConflictMetadata {
	if reduce.kind != lrReduce || reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
		return lrConflictMetadata{}
	}
	prod := &ng.Productions[reduce.prodIdx]
	metadata := lrConflictMetadata{prec: prod.Prec, hasPrec: prod.HasExplicitPrec, assoc: prod.Assoc}
	if metadata.prec != 0 || metadata.hasPrec || metadata.assoc != AssocNone || prod.DynPrec != 0 {
		return metadata
	}
	if inferred, ok := inferOptionalPrefixReduceMetadata(prod, ng, cache); ok {
		return inferred
	}
	return metadata
}

func inferOptionalPrefixReduceMetadata(prod *Production, ng *NormalizedGrammar, cache *conflictResolutionCache) (lrConflictMetadata, bool) {
	if prod == nil || ng == nil || cache == nil || prod.LHS < 0 || prod.LHS >= len(cache.prodsByLHS) {
		return lrConflictMetadata{}, false
	}
	var inferred lrConflictMetadata
	found := false
	for _, prodIdx := range cache.prodsByLHS[prod.LHS] {
		if prodIdx < 0 || prodIdx >= len(ng.Productions) {
			continue
		}
		candidate := &ng.Productions[prodIdx]
		if candidate == prod || candidate.LHS != prod.LHS || len(candidate.RHS) <= len(prod.RHS) || !rhsHasPrefix(candidate.RHS, prod.RHS) {
			continue
		}
		if !candidate.HasExplicitPrec || candidate.Assoc == AssocNone {
			continue
		}
		metadata := lrConflictMetadata{prec: candidate.Prec, hasPrec: true, assoc: candidate.Assoc}
		if found && metadata != inferred {
			return lrConflictMetadata{}, false
		}
		inferred = metadata
		found = true
	}
	return inferred, found
}

func shiftContributorMatchesReduce(lhs, reduceLHS int, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if lhs <= 0 || reduceLHS <= 0 {
		return false
	}
	if lhs == reduceLHS {
		return true
	}
	if ng == nil || cache == nil {
		return false
	}
	reduceFamily := make(map[int]bool)
	reduceFamily[reduceLHS] = true
	for _, parent := range resolveAuxToParents(reduceLHS, ng, cache) {
		reduceFamily[parent] = true
	}
	if reduceFamily[lhs] {
		return true
	}
	for _, parent := range resolveAuxToParents(lhs, ng, cache) {
		if reduceFamily[parent] {
			return true
		}
	}
	return false
}

func compareConflictMetadata(a, b lrConflictMetadata) int {
	if a.prec != b.prec {
		if a.prec > b.prec {
			return 1
		}
		return -1
	}
	if a.hasPrec != b.hasPrec {
		if a.hasPrec {
			return 1
		}
		return -1
	}
	if a.assoc != b.assoc {
		if a.assoc != AssocNone && b.assoc == AssocNone {
			return 1
		}
		if a.assoc == AssocNone && b.assoc != AssocNone {
			return -1
		}
	}
	return 0
}

func rhsHasPrefix(rhs, prefix []int) bool {
	if len(rhs) < len(prefix) {
		return false
	}
	for i, sym := range prefix {
		if rhs[i] != sym {
			return false
		}
	}
	return true
}

func rhsCanContinueWithAny(rhs []int, lookaheadTargets, shiftTargets map[int]bool, cache *conflictResolutionCache, ng *NormalizedGrammar) bool {
	if cache != nil {
		cache.structuralStats.RHSContinueCalls++
	}
	if len(rhs) == 0 || cache == nil || ng == nil {
		return false
	}
	visiting := make([]bool, len(ng.Symbols))
	for i := len(rhs) - 1; i >= 0; i-- {
		if symbolCanContinueWithAny(rhs[i], lookaheadTargets, shiftTargets, cache, visiting, ng) {
			return true
		}
		if rhs[i] < 0 || rhs[i] >= len(cache.nullable) || !cache.nullable[rhs[i]] {
			return false
		}
	}
	return false
}

func symbolCanContinueWithAny(sym int, lookaheadTargets, shiftTargets map[int]bool, cache *conflictResolutionCache, visiting []bool, ng *NormalizedGrammar) bool {
	if sym < 0 || sym >= len(ng.Symbols) || len(shiftTargets) == 0 {
		return false
	}
	for target := range shiftTargets {
		if target < 0 || target >= len(ng.Symbols) || ng.Symbols[target].Kind != SymbolNonterminal {
			continue
		}
		if nonterminalCanContinueSymbol(target, sym, lookaheadTargets, cache, visiting, ng) {
			return true
		}
	}
	return false
}

func nonterminalCanContinueSymbol(target, sym int, lookaheadTargets map[int]bool, cache *conflictResolutionCache, visiting []bool, ng *NormalizedGrammar) bool {
	if target < 0 || target >= len(cache.prodsByLHS) || visiting[target] {
		return false
	}
	visiting[target] = true
	defer func() { visiting[target] = false }()

	symTargets := map[int]bool{sym: true}
	for _, prodIdx := range cache.prodsByLHS[target] {
		if prodIdx < 0 || prodIdx >= len(ng.Productions) {
			continue
		}
		rhs := ng.Productions[prodIdx].RHS
		for split := 1; split < len(rhs); split++ {
			if rhsCanEndWithAny(rhs[:split], symTargets, cache, visiting, ng) &&
				rhsCanBeginWithAny(rhs[split:], lookaheadTargets, cache, ng) {
				return true
			}
		}
	}
	return false
}

func rhsCanEndWithAny(rhs []int, targets map[int]bool, cache *conflictResolutionCache, visiting []bool, ng *NormalizedGrammar) bool {
	if cache != nil {
		cache.structuralStats.RHSEndCalls++
	}
	for i := len(rhs) - 1; i >= 0; i-- {
		if symbolCanEndWithAny(rhs[i], targets, cache, visiting, ng) {
			return true
		}
		if rhs[i] < 0 || rhs[i] >= len(cache.nullable) || !cache.nullable[rhs[i]] {
			return false
		}
	}
	return false
}

func symbolCanEndWithAny(sym int, targets map[int]bool, cache *conflictResolutionCache, visiting []bool, ng *NormalizedGrammar) bool {
	if sym < 0 || sym >= len(ng.Symbols) {
		return false
	}
	if targets[sym] {
		return true
	}
	if ng.Symbols[sym].Kind != SymbolNonterminal {
		return false
	}
	if visiting[sym] {
		return false
	}
	visiting[sym] = true
	defer func() { visiting[sym] = false }()
	if sym >= len(cache.prodsByLHS) {
		return false
	}
	for _, prodIdx := range cache.prodsByLHS[sym] {
		if prodIdx < 0 || prodIdx >= len(ng.Productions) {
			continue
		}
		if rhsCanEndWithAny(ng.Productions[prodIdx].RHS, targets, cache, visiting, ng) {
			return true
		}
	}
	return false
}

func rhsCanBeginWithAny(rhs []int, targets map[int]bool, cache *conflictResolutionCache, ng *NormalizedGrammar) bool {
	if cache != nil {
		cache.structuralStats.RHSBeginCalls++
	}
	if cache == nil || ng == nil {
		return false
	}
	if result, ok := rhsCanBeginWithAnyFirstSets(rhs, targets, cache, ng); ok {
		cache.structuralStats.RHSBeginHits++
		return result
	}
	cache.structuralStats.RHSBeginMisses++
	return false
}

func rhsCanBeginWithAnyFirstSets(rhs []int, targets map[int]bool, cache *conflictResolutionCache, ng *NormalizedGrammar) (bool, bool) {
	if len(rhs) == 0 || len(targets) == 0 {
		return false, true
	}
	firstSets := cache.firstSets
	if firstSets == nil {
		var err error
		firstSets, err = cache.ensureFirstSets(context.Background(), ng)
		if err != nil || firstSets == nil {
			return false, false
		}
	}
	for _, sym := range rhs {
		if sym < 0 || sym >= len(firstSets) {
			return false, true
		}
		if firstSetIntersectsTargets(firstSets[sym], targets) {
			return true, true
		}
		if sym >= len(cache.nullable) || !cache.nullable[sym] {
			return false, true
		}
	}
	return false, true
}

func firstSetIntersectsTargets(firstSet []uint64, targets map[int]bool) bool {
	for target, ok := range targets {
		if ok && bitsetHas(firstSet, target) {
			return true
		}
	}
	return false
}

func preferredClosureParametersReduce(shifts, reduces []lrAction, ng *NormalizedGrammar) ([]lrAction, bool) {
	if ng == nil || len(shifts) == 0 || len(reduces) == 0 {
		return nil, false
	}
	var preferred []lrAction
	for _, reduce := range reduces {
		if symbolNameMatches(reduce.lhsSym, ng, "closure_parameters") {
			preferred = append(preferred, reduce)
		}
	}
	if len(preferred) == 0 {
		return nil, false
	}
	return preferred, true
}

func shouldKeepExpressionStructInitializerConflict(lookaheadSym int, shifts, reduces []lrAction, ng *NormalizedGrammar) bool {
	if ng == nil || lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) || ng.Symbols[lookaheadSym].Name != "{" {
		return false
	}
	hasFieldInitializerShift := false
	for _, shift := range shifts {
		if shift.kind != lrShift || shift.lhsSym < 0 || shift.lhsSym >= len(ng.Symbols) {
			continue
		}
		if ng.Symbols[shift.lhsSym].Name == "field_initializer_list" {
			hasFieldInitializerShift = true
			break
		}
	}
	if !hasFieldInitializerShift {
		return false
	}
	for _, reduce := range reduces {
		if reduce.kind != lrReduce || reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
			continue
		}
		prod := &ng.Productions[reduce.prodIdx]
		if prod.Prec != 0 || prod.LHS < 0 || prod.LHS >= len(ng.Symbols) {
			continue
		}
		// In C tree-sitter this Rust table conflict is kept as GLR: after an
		// identifier, "{" can either start a struct initializer's field list or
		// a later block. The predecessor context decides which branch survives.
		if ng.Symbols[prod.LHS].Name == "_expression_except_range" {
			return true
		}
	}
	return false
}

func isAssignmentOperatorLookahead(name string) bool {
	if name == "=" {
		return true
	}
	if !strings.HasSuffix(name, "=") {
		return false
	}
	switch name {
	case "==", "!=", "<=", ">=", "=>", "===", "!==":
		return false
	default:
		return true
	}
}

func shouldPreferAssignmentExpressionShift(lookaheadSym int, shifts, reduces []lrAction, ng *NormalizedGrammar) bool {
	if ng == nil || len(shifts) != 1 || len(reduces) == 0 {
		return false
	}
	shift := shifts[0]
	if shift.lhsSym < 0 || shift.lhsSym >= len(ng.Symbols) {
		return false
	}
	shiftLHSName := ng.Symbols[shift.lhsSym].Name
	if !isAssignmentShiftLHS(shiftLHSName) {
		return false
	}
	if lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) {
		return false
	}
	lookaheadName := ng.Symbols[lookaheadSym].Name
	if !isAssignmentOperatorLookahead(lookaheadName) && !(lookaheadName == "operator" && isAssignmentShiftLHS(shiftLHSName)) {
		return false
	}
	for _, reduce := range reduces {
		if reduce.kind != lrReduce || reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
			return false
		}
		prod := &ng.Productions[reduce.prodIdx]
		if prod.LHS >= 0 && prod.LHS < len(ng.Symbols) && ng.Symbols[prod.LHS].Name == "pattern" {
			return false
		}
		if prod.Prec != 0 || prod.Assoc != AssocNone || prod.HasExplicitPrec {
			return false
		}
	}
	return true
}

func preferredArithmeticWrapperShift(lookaheadSym int, shifts, reduces []lrAction, ng *NormalizedGrammar) (lrAction, bool) {
	if ng == nil || len(shifts) != 1 || len(reduces) == 0 ||
		lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) {
		return lrAction{}, false
	}
	if !isArithmeticOperatorLookaheadName(ng.Symbols[lookaheadSym].Name) {
		return lrAction{}, false
	}
	shift := shifts[0]
	if shift.lhsSym < 0 || shift.lhsSym >= len(ng.Symbols) ||
		ng.Symbols[shift.lhsSym].Name != "binary_expression" {
		return lrAction{}, false
	}
	for _, reduce := range reduces {
		if reduce.kind != lrReduce || reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
			return lrAction{}, false
		}
		prod := &ng.Productions[reduce.prodIdx]
		if len(prod.RHS) != 1 || prod.LHS < 0 || prod.LHS >= len(ng.Symbols) {
			return lrAction{}, false
		}
		lhsName := ng.Symbols[prod.LHS].Name
		if lhsName != "_arithmetic_expression" && lhsName != "_arithmetic_literal" {
			return lrAction{}, false
		}
	}
	return shift, true
}

func preferredArithmeticExpressionContinuation(lookaheadSym int, shifts, reduces []lrAction, ng *NormalizedGrammar) (lrAction, bool) {
	if ng == nil || len(shifts) != 1 || len(reduces) == 0 ||
		lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) {
		return lrAction{}, false
	}
	if !isArithmeticOperatorLookaheadName(ng.Symbols[lookaheadSym].Name) {
		return lrAction{}, false
	}
	var expressionReduces []lrAction
	for _, reduce := range reduces {
		if reduce.kind != lrReduce || reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
			return lrAction{}, false
		}
		prod := &ng.Productions[reduce.prodIdx]
		if prod.LHS < 0 || prod.LHS >= len(ng.Symbols) {
			return lrAction{}, false
		}
		lhsName := ng.Symbols[prod.LHS].Name
		switch lhsName {
		case "_arithmetic_expression", "_arithmetic_literal":
			continue
		case "binary_expression", "unary_expression", "ternary_expression", "postfix_expression", "parenthesized_expression":
			expressionReduces = append(expressionReduces, reduce)
		default:
			// Skip unrelated reduces — they're chain-reductions through
			// supertypes/wrappers (e.g. `_query → binary_expression`) that
			// shouldn't override the operator-precedence-driven choice. The
			// LR state machine will still chain-reduce them via GOTO after
			// the expression reduce wins. Bailing out here let an unrelated
			// prec=0 reduce dominate the shift comparison.
			continue
		}
	}
	if len(expressionReduces) == 0 {
		return lrAction{}, false
	}
	best := rrPickBest(expressionReduces, ng)[0]
	bestProd := &ng.Productions[best.prodIdx]
	shift := shifts[0]
	if shift.prec == 0 && bestProd.Prec > 0 {
		cmp := resolveShiftLHSVsNamedPrec(shift, bestProd.Prec, ng)
		if cmp > 0 {
			return shift, true
		}
		if cmp < 0 {
			return best, true
		}
	}
	if bestProd.Prec > shift.prec {
		return best, true
	}
	if shift.prec > bestProd.Prec {
		return shift, true
	}
	switch bestProd.Assoc {
	case AssocLeft:
		return best, true
	case AssocRight:
		return shift, true
	default:
		return lrAction{}, false
	}
}

func isArithmeticOperatorLookaheadName(name string) bool {
	switch name {
	case "++", "--",
		"+=", "-=", "*=", "/=", "%=", "**=", "<<=", ">>=", "&=", "^=", "|=",
		"=", "=~",
		"||", "&&", "|", "^", "&",
		"==", "!=", "<", ">", "<=", ">=",
		"<<", ">>", "+", "-", "*", "/", "%", "**":
		return true
	default:
		return false
	}
}

func preferredArithmeticDelimiterShift(lookaheadSym int, shifts, reduces []lrAction, ng *NormalizedGrammar) (lrAction, bool) {
	if ng == nil || len(shifts) != 1 || len(reduces) == 0 ||
		lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) {
		return lrAction{}, false
	}
	lookaheadName := ng.Symbols[lookaheadSym].Name
	if !isArithmeticDelimiterLookaheadName(lookaheadName) {
		return lrAction{}, false
	}
	shift := shifts[0]
	if shift.lhsSym < 0 || shift.lhsSym >= len(ng.Symbols) {
		return lrAction{}, false
	}
	shiftLHSName := ng.Symbols[shift.lhsSym].Name
	if !isArithmeticDelimiterShiftLHS(shiftLHSName, lookaheadName) {
		return lrAction{}, false
	}
	for _, reduce := range reduces {
		if reduce.kind != lrReduce || reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
			return lrAction{}, false
		}
		prod := &ng.Productions[reduce.prodIdx]
		if len(prod.RHS) != 1 || prod.LHS < 0 || prod.LHS >= len(ng.Symbols) {
			return lrAction{}, false
		}
		lhsName := ng.Symbols[prod.LHS].Name
		if lhsName != "_arithmetic_expression" && lhsName != "_arithmetic_literal" {
			return lrAction{}, false
		}
		if prod.Assoc != AssocNone || prod.Prec != shift.prec {
			return lrAction{}, false
		}
	}
	return shift, true
}

func isArithmeticDelimiterLookaheadName(name string) bool {
	switch name {
	case "))", "]", ",", ")":
		return true
	default:
		return false
	}
}

func isArithmeticDelimiterShiftLHS(lhsName, lookaheadName string) bool {
	switch lookaheadName {
	case "))", "]", ",":
		return lhsName == "arithmetic_expansion"
	case ")":
		return lhsName == "parenthesized_expression"
	default:
		return false
	}
}

func isAssignmentShiftLHS(name string) bool {
	switch name {
	case "assignment_expression", "assignment", "_closed_assignment":
		return true
	default:
		return false
	}
}

func shouldPreserveKeywordIdentifierShiftReduce(lookaheadSym int, reduces []lrAction, ng *NormalizedGrammar) bool {
	if ng == nil || !ng.PreserveKeywordIdentifierConflicts {
		return false
	}
	if !keywordIdentifierConflictLookahead(lookaheadSym, ng) {
		return false
	}
	for _, reduce := range reduces {
		if isVisibleKeywordIdentifierReduce(reduce, ng) {
			return true
		}
	}
	return false
}

func shouldPreserveDerivedKeywordIdentifierShiftReduce(lookaheadSym int, reduces []lrAction, ng *NormalizedGrammar) bool {
	return ng != nil &&
		ng.DerivedKeywordIdentifierConflicts &&
		shouldPreserveKeywordIdentifierShiftReduce(lookaheadSym, reduces, ng)
}

func preferredExpressionOperatorIdentifierReduce(lookaheadSym int, shifts, reduces []lrAction, ng *NormalizedGrammar) ([]lrAction, bool) {
	if ng == nil || !ng.PreferExpressionOperatorIdentifierReduces {
		return nil, false
	}
	if lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) ||
		!isElixirOperatorIdentifierConflictLookahead(ng.Symbols[lookaheadSym].Name) {
		return nil, false
	}
	hasOperatorIdentifierShift := false
	for _, shift := range shifts {
		if shiftLHSIncludesName(shift, ng, "operator_identifier") {
			hasOperatorIdentifierShift = true
			break
		}
	}
	if !hasOperatorIdentifierShift {
		return nil, false
	}
	preferred := make([]lrAction, 0, len(reduces))
	for _, reduce := range reduces {
		if !isExpressionOperatorConflictReduce(reduce, ng) {
			continue
		}
		if operatorIdentifierShiftOutranksReduce(shifts, reduce, ng) {
			continue
		}
		preferred = append(preferred, reduce)
	}
	return preferred, len(preferred) > 0
}

func preferredAtomToExpressionOperatorIdentifierReduce(lookaheadSym int, shifts, reduces []lrAction, ng *NormalizedGrammar) ([]lrAction, bool) {
	if ng == nil || !ng.PreferExpressionOperatorIdentifierReduces {
		return nil, false
	}
	if lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) ||
		!isElixirOperatorIdentifierConflictLookahead(ng.Symbols[lookaheadSym].Name) {
		return nil, false
	}
	hasOperatorIdentifierShift := false
	for _, shift := range shifts {
		if shiftLHSIncludesName(shift, ng, "operator_identifier") {
			hasOperatorIdentifierShift = true
			break
		}
	}
	if !hasOperatorIdentifierShift {
		return nil, false
	}
	preferred := make([]lrAction, 0, len(reduces))
	for _, reduce := range reduces {
		if isAtomToExpressionOperatorConflictReduce(reduce, ng) {
			preferred = append(preferred, reduce)
		}
	}
	return preferred, len(preferred) > 0
}

func preferredCompletedCallDoBlockReduce(lookaheadSym int, shifts, reduces []lrAction, ng *NormalizedGrammar) ([]lrAction, bool) {
	if ng == nil || !ng.PreferParenthesizedCallDoBlockReduces {
		return nil, false
	}
	if lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) || ng.Symbols[lookaheadSym].Name != "do" {
		return nil, false
	}
	hasDoBlockShift := false
	for _, shift := range shifts {
		if shiftLHSIncludesName(shift, ng, "do_block") {
			hasDoBlockShift = true
			break
		}
	}
	if !hasDoBlockShift {
		return nil, false
	}
	preferred := make([]lrAction, 0, len(reduces))
	for _, reduce := range reduces {
		if isCompletedCallWithoutDoBlockReduce(reduce, ng) {
			preferred = append(preferred, reduce)
		}
	}
	return preferred, len(preferred) > 0
}

func isCompletedCallWithoutDoBlockReduce(action lrAction, ng *NormalizedGrammar) bool {
	if action.kind != lrReduce || action.prodIdx < 0 || action.prodIdx >= len(ng.Productions) {
		return false
	}
	prod := &ng.Productions[action.prodIdx]
	if prod.LHS < 0 || prod.LHS >= len(ng.Symbols) {
		return false
	}
	lhsName := ng.Symbols[prod.LHS].Name
	switch lhsName {
	case "_local_call_with_parentheses", "_remote_call_with_parentheses", "_double_call", "_remote_call_without_parentheses":
	default:
		return false
	}
	if lhsName == "_remote_call_without_parentheses" && !productionContainsSymbolName(prod, ng, "_remote_dot") {
		return false
	}
	hasParenthesizedArguments := false
	for _, sym := range prod.RHS {
		if sym < 0 || sym >= len(ng.Symbols) {
			continue
		}
		switch ng.Symbols[sym].Name {
		case "_call_arguments_with_parentheses", "_call_arguments_with_parentheses_immediate":
			hasParenthesizedArguments = true
		case "do_block", "_newline_before_do":
			return false
		}
	}
	return hasParenthesizedArguments || lhsName == "_remote_call_without_parentheses"
}

func productionContainsSymbolName(prod *Production, ng *NormalizedGrammar, name string) bool {
	if prod == nil || ng == nil {
		return false
	}
	for _, sym := range prod.RHS {
		if symbolNameMatches(sym, ng, name) {
			return true
		}
	}
	return false
}

func preferredRemoteCallOperatorReduce(lookaheadSym int, shifts, reduces []lrAction, ng *NormalizedGrammar) ([]lrAction, bool) {
	if ng == nil || !ng.PreferRemoteCallOperatorReduces {
		return nil, false
	}
	if lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) ||
		!isElixirOperatorIdentifierConflictLookahead(ng.Symbols[lookaheadSym].Name) {
		return nil, false
	}
	hasBinaryOperatorShift := false
	for _, shift := range shifts {
		if shiftLHSIncludesName(shift, ng, "binary_operator") {
			hasBinaryOperatorShift = true
			break
		}
	}
	if !hasBinaryOperatorShift {
		return nil, false
	}
	preferred := make([]lrAction, 0, len(reduces))
	for _, reduce := range reduces {
		if isCompletedRemoteCallReduce(reduce, ng) {
			preferred = append(preferred, reduce)
		}
	}
	return preferred, len(preferred) > 0
}

func isCompletedRemoteCallReduce(action lrAction, ng *NormalizedGrammar) bool {
	if action.kind != lrReduce || action.prodIdx < 0 || action.prodIdx >= len(ng.Productions) {
		return false
	}
	prod := &ng.Productions[action.prodIdx]
	if prod.LHS < 0 || prod.LHS >= len(ng.Symbols) {
		return false
	}
	switch ng.Symbols[prod.LHS].Name {
	case "_remote_call_without_parentheses", "_remote_call_with_parentheses":
		return true
	default:
		return false
	}
}

func preferredStabClauseLeftArrowReduce(lookaheadSym int, shifts, reduces []lrAction, ng *NormalizedGrammar) ([]lrAction, bool) {
	if ng == nil || !ng.PreferStabClauseLeftArrowReduces {
		return nil, false
	}
	if lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) || ng.Symbols[lookaheadSym].Name != "->" {
		return nil, false
	}
	hasStabClauseShift := false
	for _, shift := range shifts {
		if shiftLHSIncludesName(shift, ng, "stab_clause") {
			hasStabClauseShift = true
			break
		}
	}
	if !hasStabClauseShift {
		return nil, false
	}
	preferred := make([]lrAction, 0, len(reduces))
	for _, reduce := range reduces {
		if isStabClauseLeftReduce(reduce, ng) {
			preferred = append(preferred, reduce)
		}
	}
	return preferred, len(preferred) > 0
}

func preservedStabClauseArrowExpressionAmbiguity(lookaheadSym int, shifts, reduces []lrAction, ng *NormalizedGrammar) ([]lrAction, bool) {
	if ng == nil || !ng.PreferStabClauseLeftArrowReduces {
		return nil, false
	}
	if lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) || ng.Symbols[lookaheadSym].Name != "->" {
		return nil, false
	}
	hasOperatorIdentifierShift := false
	for _, shift := range shifts {
		if shiftLHSIncludesName(shift, ng, "operator_identifier") {
			hasOperatorIdentifierShift = true
			break
		}
	}
	if !hasOperatorIdentifierShift {
		return nil, false
	}
	for _, reduce := range reduces {
		if isAtomToExpressionOperatorConflictReduce(reduce, ng) {
			preserved := make([]lrAction, 0, len(shifts)+len(reduces))
			preserved = append(preserved, shifts...)
			preserved = append(preserved, reduces...)
			return preserved, true
		}
	}
	return nil, false
}

func isStabClauseLeftReduce(action lrAction, ng *NormalizedGrammar) bool {
	if action.kind != lrReduce || action.prodIdx < 0 || action.prodIdx >= len(ng.Productions) {
		return false
	}
	prod := &ng.Productions[action.prodIdx]
	return symbolNameMatches(prod.LHS, ng, "_stab_clause_left")
}

func operatorIdentifierShiftOutranksReduce(shifts []lrAction, reduce lrAction, ng *NormalizedGrammar) bool {
	if reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
		return false
	}
	prod := &ng.Productions[reduce.prodIdx]
	if isAtomToExpressionOperatorConflictReduce(reduce, ng) {
		return false
	}
	reducePrec := prod.Prec
	reduceLHSName := ""
	if prod.LHS >= 0 && prod.LHS < len(ng.Symbols) {
		reduceLHSName = ng.Symbols[prod.LHS].Name
	}

	for _, shift := range shifts {
		shiftPrec := shift.prec

		if ng.PrecedenceOrder != nil {
			if reducePrec == 0 && shiftPrec > 0 && reduceLHSName != "" {
				if ng.PrecedenceOrder.resolveSymbolVsNamedPrec(reduceLHSName, shiftPrec) < 0 {
					return true
				}
			}
			if shiftPrec == 0 && reducePrec > 0 {
				if resolveShiftLHSVsNamedPrec(shift, reducePrec, ng) > 0 {
					return true
				}
			}
			if shiftPrec == reducePrec && reduceLHSName != "" {
				if resolveShiftLHSVsReduceLHSPrecedence(shift, prod.LHS, ng) > 0 {
					return true
				}
			}
		}

		if (shiftPrec != 0 || reducePrec != 0) && shiftPrec > reducePrec {
			return true
		}
		if shiftPrec == reducePrec && prod.Assoc == AssocRight {
			return true
		}
	}
	return false
}

func shiftLHSIncludesName(shift lrAction, ng *NormalizedGrammar, name string) bool {
	if symbolNameMatches(shift.lhsSym, ng, name) {
		return true
	}
	for _, lhs := range shift.lhsSyms {
		if symbolNameMatches(lhs, ng, name) {
			return true
		}
	}
	return false
}

func resolveShiftLHSVsNamedPrec(shift lrAction, namedPrec int, ng *NormalizedGrammar) int {
	if ng == nil || ng.PrecedenceOrder == nil {
		return 0
	}
	sawLower := false
	seen := map[int]struct{}{}
	check := func(lhs int) int {
		if lhs < 0 || lhs >= len(ng.Symbols) {
			return 0
		}
		if _, ok := seen[lhs]; ok {
			return 0
		}
		seen[lhs] = struct{}{}
		cmp := ng.PrecedenceOrder.resolveSymbolVsNamedPrec(ng.Symbols[lhs].Name, namedPrec)
		if cmp > 0 {
			return 1
		}
		if cmp < 0 {
			sawLower = true
		}
		return 0
	}
	if check(shift.lhsSym) > 0 {
		return 1
	}
	for _, lhs := range shift.lhsSyms {
		if check(lhs) > 0 {
			return 1
		}
	}
	if sawLower {
		return -1
	}
	return 0
}

func resolveShiftLHSVsReduceLHSPrecedence(shift lrAction, reduceLHS int, ng *NormalizedGrammar) int {
	if ng == nil || ng.PrecedenceOrder == nil || reduceLHS < 0 || reduceLHS >= len(ng.Symbols) {
		return 0
	}
	reduceName := ng.Symbols[reduceLHS].Name
	if reduceName == "" {
		return 0
	}

	sawLower := false
	seen := map[int]struct{}{}
	check := func(lhs int) int {
		if lhs < 0 || lhs >= len(ng.Symbols) {
			return 0
		}
		if _, ok := seen[lhs]; ok {
			return 0
		}
		seen[lhs] = struct{}{}
		shiftName := ng.Symbols[lhs].Name
		if shiftName == "" || shiftName == reduceName {
			return 0
		}
		cmp := ng.PrecedenceOrder.resolveSymbolVsSymbol(shiftName, reduceName)
		if cmp > 0 {
			return 1
		}
		if cmp < 0 {
			sawLower = true
		}
		return 0
	}
	if check(shift.lhsSym) > 0 {
		return 1
	}
	for _, lhs := range shift.lhsSyms {
		if check(lhs) > 0 {
			return 1
		}
	}
	if sawLower {
		return -1
	}
	return 0
}

func symbolNameMatches(sym int, ng *NormalizedGrammar, name string) bool {
	return ng != nil && sym >= 0 && sym < len(ng.Symbols) && ng.Symbols[sym].Name == name
}

func isExpressionOperatorConflictReduce(action lrAction, ng *NormalizedGrammar) bool {
	if action.kind != lrReduce || action.prodIdx < 0 || action.prodIdx >= len(ng.Productions) {
		return false
	}
	prod := &ng.Productions[action.prodIdx]
	if symbolNameMatches(prod.LHS, ng, "binary_operator") {
		return true
	}
	return isAtomToExpressionOperatorConflictReduce(action, ng)
}

func isAtomToExpressionOperatorConflictReduce(action lrAction, ng *NormalizedGrammar) bool {
	if action.kind != lrReduce || action.prodIdx < 0 || action.prodIdx >= len(ng.Productions) {
		return false
	}
	prod := &ng.Productions[action.prodIdx]
	if !symbolNameMatches(prod.LHS, ng, "_expression") || len(prod.RHS) != 1 {
		return false
	}
	rhs := prod.RHS[0]
	if rhs < 0 || rhs >= len(ng.Symbols) {
		return false
	}
	switch ng.Symbols[rhs].Name {
	case "identifier", "alias", "integer", "float", "char", "boolean", "nil",
		"_atom", "string", "charlist", "sigil", "list", "tuple", "bitstring", "map":
		return true
	default:
		return false
	}
}

func isElixirOperatorIdentifierConflictLookahead(name string) bool {
	switch name {
	case "<-", "\\\\", "when", "::", "|", "=>", "=", "||", "|||", "or",
		"&&", "&&&", "and", "==", "!=", "=~", "===", "!==", "<", ">",
		"<=", ">=", "|>", "<<<", ">>>", "<<~", "~>>", "<~", "~>",
		"<~>", "<|>", "in", "not in", "^^^", "//", "++", "--", "+++",
		"---", "<>", "..", "+", "-", "*", "/", "**":
		return true
	default:
		return false
	}
}

func preferredKeywordContinuationShift(lookaheadSym int, shifts, reduces []lrAction, ng *NormalizedGrammar) (lrAction, bool) {
	if ng == nil || !ng.PreserveKeywordIdentifierConflicts || ng.DerivedKeywordIdentifierConflicts {
		return lrAction{}, false
	}
	if !keywordIdentifierConflictLookahead(lookaheadSym, ng) {
		return lrAction{}, false
	}
	hasKeywordIdentifierReduce := false
	for _, reduce := range reduces {
		if isVisibleKeywordIdentifierReduce(reduce, ng) {
			hasKeywordIdentifierReduce = true
			break
		}
	}
	if !hasKeywordIdentifierReduce {
		return lrAction{}, false
	}
	for _, shift := range shifts {
		if keywordContinuationShiftIsSpecific(shift, ng) {
			return shift, true
		}
	}
	return lrAction{}, false
}

func keywordIdentifierConflictLookahead(lookaheadSym int, ng *NormalizedGrammar) bool {
	if ng == nil || lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) {
		return false
	}
	switch ng.Symbols[lookaheadSym].Name {
	case "(", "[":
		return true
	default:
		return false
	}
}

func keywordContinuationShiftIsSpecific(shift lrAction, ng *NormalizedGrammar) bool {
	if keywordContinuationLHSIsSpecific(shift.lhsSym, ng) {
		return true
	}
	for _, lhs := range shift.lhsSyms {
		if keywordContinuationLHSIsSpecific(lhs, ng) {
			return true
		}
	}
	return false
}

func keywordContinuationLHSIsSpecific(lhs int, ng *NormalizedGrammar) bool {
	if lhs < 0 || lhs >= len(ng.Symbols) {
		return false
	}
	name := ng.Symbols[lhs].Name
	return name == "null_literal" || strings.HasSuffix(name, "_statement")
}

func isVisibleKeywordIdentifierReduce(action lrAction, ng *NormalizedGrammar) bool {
	if action.kind != lrReduce || action.prodIdx < 0 || action.prodIdx >= len(ng.Productions) {
		return false
	}
	prod := &ng.Productions[action.prodIdx]
	return productionIsDirectKeywordToWord(prod, ng) || productionHasKeywordAliasToWord(prod, ng)
}

func repetitionShiftActions(lookaheadSym int, shifts, reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) ([]lrAction, bool) {
	if len(shifts) != 1 || len(reduces) == 0 {
		return nil, false
	}
	for _, r := range reduces {
		if !isRecursiveRepeatReduce(r, ng, cache) {
			return nil, false
		}
	}
	repeatLHS, sameRepeatLHS := commonReduceLHS(reduces, ng)
	shift := shifts[0]
	shiftMatchesReduces := shift.repeat && recursiveRepeatShiftMatchesReduces(shift, reduces, ng)
	if !shiftMatchesReduces && lookaheadSym != shift.lhsSym &&
		!recursiveRepeatShiftCanContinueLookahead(lookaheadSym, shift, reduces, ng, cache) {
		return nil, false
	}
	kept := make([]lrAction, 0, len(reduces)+1)
	kept = append(kept, reduces...)
	shift.repeat = true
	if sameRepeatLHS {
		shift.addRepeatLHS(repeatLHS)
	}
	if sameRepeatLHS && len(reduces) == 1 {
		if parent, ok := loweredRepeatHelperVisibleParentProduction(reduces[0], ng, cache); ok {
			if resolved, resolvedOK := resolveLoweredRepeatHelperShiftReduce(reduces[0], shift, parent); resolvedOK {
				return resolved, true
			}
		}
	}
	kept = append(kept, shift)
	return kept, true
}

func commonReduceLHS(reduces []lrAction, ng *NormalizedGrammar) (int, bool) {
	if ng == nil || len(reduces) == 0 {
		return 0, false
	}
	common := -1
	for _, reduce := range reduces {
		if reduce.kind != lrReduce || reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
			return 0, false
		}
		lhs := ng.Productions[reduce.prodIdx].LHS
		if common < 0 {
			common = lhs
			continue
		}
		if lhs != common {
			return 0, false
		}
	}
	if common <= 0 {
		return 0, false
	}
	return common, true
}

func recursiveRepeatShiftMatchesReduces(shift lrAction, reduces []lrAction, ng *NormalizedGrammar) bool {
	if ng == nil {
		return false
	}
	for _, reduce := range reduces {
		if reduce.kind != lrReduce || reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
			return false
		}
		if !shift.hasRepeatLHS(ng.Productions[reduce.prodIdx].LHS) {
			return false
		}
	}
	return true
}

func recursiveRepeatShiftCanContinueLookahead(lookaheadSym int, shift lrAction, reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if ng == nil || cache == nil || lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) {
		return false
	}
	shiftTargets := shiftContinuationTargets(shift, len(ng.Symbols))
	if len(shiftTargets) == 0 {
		return false
	}
	lookaheadTargets := map[int]bool{lookaheadSym: true}
	for _, reduce := range reduces {
		if reduce.kind != lrReduce || reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
			return false
		}
		prod := &ng.Productions[reduce.prodIdx]
		if prod.LHS < 0 || prod.LHS >= len(ng.Symbols) {
			return false
		}
		for i, sym := range prod.RHS {
			if sym != prod.LHS {
				continue
			}
			tail := prod.RHS[i+1:]
			if len(tail) == 0 {
				continue
			}
			if rhsCanBeginWithAny(tail, lookaheadTargets, cache, ng) &&
				rhsCanBeginWithAny(tail, shiftTargets, cache, ng) {
				return true
			}
		}
	}
	return false
}

func isRecursiveRepeatReduce(action lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if action.kind != lrReduce || action.prodIdx < 0 || action.prodIdx >= len(ng.Productions) {
		return false
	}
	prod := &ng.Productions[action.prodIdx]
	if prod.LHS < 0 || prod.LHS >= len(ng.Symbols) {
		return false
	}
	if cache == nil || prod.LHS >= len(cache.prodsByLHS) {
		return false
	}
	if !isStructurallyGeneratedRepeatHelper(prod.LHS, ng, cache) {
		return false
	}
	for _, sym := range prod.RHS {
		if sym == prod.LHS {
			return true
		}
	}
	return false
}

func isRepeatHelperReduce(action lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if action.kind != lrReduce || action.prodIdx < 0 || action.prodIdx >= len(ng.Productions) {
		return false
	}
	prod := &ng.Productions[action.prodIdx]
	if prod.LHS < 0 || prod.LHS >= len(ng.Symbols) {
		return false
	}
	return isStructurallyGeneratedRepeatHelper(prod.LHS, ng, cache)
}

func resolveReduceReduceLegacy(lookaheadSym int, reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) ([]lrAction, error) {
	if kept, ok := keepBashStatementBoundaryReduces(lookaheadSym, reduces, ng); ok {
		return kept, nil
	}
	if preferred, ok := preferredBashStatementReduce(lookaheadSym, reduces, ng); ok {
		return []lrAction{preferred}, nil
	}
	if repeatReduces, ok := generatedRepeatContinuationReduceSubset(reduces, ng, cache); ok {
		return repeatReduces, nil
	}
	if filtered, ok := filterRedundantHiddenUnaryWrapperReduces(reduces, ng, cache); ok {
		reduces = orderReduceConflictByChildCount(filtered, ng)
		if allInDeclaredConflict(reduces, ng, cache) {
			return reduces, nil
		}
		if resolvedByPrec := rrPrecResolve(reduces, ng); resolvedByPrec != nil {
			return resolvedByPrec, nil
		}
		return reduces, nil
	}
	if declared, ok := keepDeclaredSameRHSNeutralUnarySubset(reduces, ng, cache); ok {
		return declared, nil
	}
	if declared, ok := keepDeclaredEqualRankVisibleSubset(reduces, ng, cache); ok {
		return declared, nil
	}
	if resolved, ok := resolveTwoReduceHiddenUnaryPassthrough(reduces, ng, cache); ok {
		return resolved, nil
	}
	if allInDeclaredConflict(reduces, ng, cache) {
		return reduces, nil
	}
	if shouldKeepSameRHSExplicitNegativeReduces(lookaheadSym, reduces, ng) {
		return reduces, nil
	}
	if shouldKeepTypeValueTokenReduces(lookaheadSym, reduces, ng) {
		if resolvedByPrec := rrPrecResolve(reduces, ng); resolvedByPrec != nil {
			return resolvedByPrec, nil
		}
		return reduces, nil
	}
	if shouldKeepRepeatedAnnotationReduces(lookaheadSym, reduces, ng) {
		return reduces, nil
	}
	if shouldKeepNestedWrapperReduces(reduces, ng) {
		// Even for nested wrapper reduces, if there is a clear precedence
		// difference among the competing reductions, resolve deterministically.
		// This matches tree-sitter C's behavior more closely: precedence
		// always wins over GLR when the grammar author specified it.
		if resolvedByPrec := rrPrecResolve(reduces, ng); resolvedByPrec != nil {
			return resolvedByPrec, nil
		}
		return reduces, nil
	}

	// Keep GLR when competing reduces produce distinct repeat helpers
	// with the same precedence. These repeat helpers serve different
	// parent productions (e.g. declaration_repeat17 for `declaration`
	// requiring ";" vs last_declaration_repeat18 for `last_declaration`
	// without ";"). Picking one deterministically kills the other
	// parse path, causing ERROR when the unchosen parent context is
	// needed. The correct disambiguation happens at the parent level.
	if shouldKeepDistinctRepeatReduces(reduces, ng) {
		return reduces, nil
	}

	return rrPickBest(reduces, ng), nil
}

func generatedRepeatContinuationReduceSubset(reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) ([]lrAction, bool) {
	if len(reduces) < 2 || ng == nil || cache == nil {
		return nil, false
	}
	var repeatReduces []lrAction
	repeatLHS := make(map[int]bool)
	hasNonRepeat := false
	repeatRHS := make(map[string]bool)
	for _, reduce := range reduces {
		if reduce.kind != lrReduce || reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
			return nil, false
		}
		prod := &ng.Productions[reduce.prodIdx]
		if isGeneratedRepeatReduceProd(prod, ng, cache) {
			repeatReduces = append(repeatReduces, reduce)
			repeatLHS[prod.LHS] = true
			repeatRHS[symbolSeqKey(prod.RHS)] = true
			continue
		}
		hasNonRepeat = true
	}
	if len(repeatReduces) == 0 {
		return nil, false
	}
	if len(repeatLHS) >= 2 {
		unbounded := make([]lrAction, 0, len(repeatReduces))
		for _, reduce := range repeatReduces {
			prod := &ng.Productions[reduce.prodIdx]
			if !generatedRepeatHasRequiredSuffixParent(prod.LHS, ng, cache) {
				unbounded = append(unbounded, reduce)
			}
		}
		if len(unbounded) > 0 && len(unbounded) < len(repeatReduces) {
			return unbounded, true
		}
	}
	if !hasNonRepeat {
		return nil, false
	}
	if len(repeatLHS) >= 2 {
		return repeatReduces, true
	}
	for _, reduce := range reduces {
		prod := &ng.Productions[reduce.prodIdx]
		if isGeneratedRepeatReduceProd(prod, ng, cache) {
			continue
		}
		if repeatRHS[symbolSeqKey(prod.RHS)] {
			return repeatReduces, true
		}
	}
	return nil, false
}

func generatedRepeatHasRequiredSuffixParent(repeatLHS int, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if repeatLHS < 0 || repeatLHS >= len(cache.rhsParents) {
		return false
	}
	for _, parentLHS := range cache.rhsParents[repeatLHS] {
		if parentLHS == repeatLHS || parentLHS < 0 || parentLHS >= len(cache.prodsByLHS) {
			continue
		}
		for _, prodIdx := range cache.prodsByLHS[parentLHS] {
			if prodIdx < 0 || prodIdx >= len(ng.Productions) {
				continue
			}
			rhs := ng.Productions[prodIdx].RHS
			for i, sym := range rhs {
				if sym == repeatLHS && i+1 < len(rhs) {
					return true
				}
			}
		}
	}
	return false
}

func isGeneratedRepeatReduceProd(prod *Production, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if prod == nil || len(prod.RHS) == 0 || prod.LHS < 0 || prod.LHS >= len(ng.Symbols) {
		return false
	}
	return isStructurallyGeneratedRepeatHelper(prod.LHS, ng, cache)
}

func symbolSeqKey(seq []int) string {
	var b strings.Builder
	for i, sym := range seq {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(sym))
	}
	return b.String()
}

func keepDeclaredEqualRankVisibleSubset(reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) ([]lrAction, bool) {
	if len(reduces) < 3 || ng == nil || cache == nil {
		return nil, false
	}
	for _, reduce := range reduces {
		if reduce.kind != lrReduce || reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
			return nil, false
		}
	}

	best := reduces[0]
	kept := []lrAction{best}
	for _, reduce := range reduces[1:] {
		switch cmp := compareReduceConflictRank(reduce, best, ng); {
		case cmp > 0:
			best = reduce
			kept = kept[:0]
			kept = append(kept, reduce)
		case cmp == 0:
			kept = append(kept, reduce)
		}
	}
	if len(kept) < 2 || len(kept) == len(reduces) {
		return nil, false
	}
	if !hasDistinctVisibleReduceFamilies(kept, ng) {
		return nil, false
	}
	if !allInDeclaredConflict(kept, ng, cache) {
		return nil, false
	}
	return kept, true
}

func hasDistinctVisibleReduceFamilies(reduces []lrAction, ng *NormalizedGrammar) bool {
	seen := make(map[int]bool, len(reduces))
	for _, reduce := range reduces {
		if reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
			return false
		}
		lhs := ng.Productions[reduce.prodIdx].LHS
		if lhs < 0 || lhs >= len(ng.Symbols) {
			return false
		}
		info := ng.Symbols[lhs]
		if info.Kind != SymbolNonterminal || (!info.Visible && !info.Named) {
			return false
		}
		seen[lhs] = true
	}
	return len(seen) >= 2
}

func compareReduceConflictRank(a, b lrAction, ng *NormalizedGrammar) int {
	aProd := &ng.Productions[a.prodIdx]
	bProd := &ng.Productions[b.prodIdx]
	if ng.PrecedenceOrder != nil {
		aLHSName := reduceLHSName(aProd, ng)
		bLHSName := reduceLHSName(bProd, ng)

		if aProd.Prec == 0 && bProd.Prec > 0 && aLHSName != "" {
			cmp := ng.PrecedenceOrder.resolveSymbolVsNamedPrec(aLHSName, bProd.Prec)
			if cmp != 0 {
				return cmp
			}
		}
		if bProd.Prec == 0 && aProd.Prec > 0 && bLHSName != "" {
			cmp := ng.PrecedenceOrder.resolveSymbolVsNamedPrec(bLHSName, aProd.Prec)
			if cmp != 0 {
				return -cmp
			}
		}
		if aProd.Prec == 0 && bProd.Prec == 0 && aLHSName != "" && bLHSName != "" && aLHSName != bLHSName {
			if cmp := ng.PrecedenceOrder.resolveSymbolVsSymbol(aLHSName, bLHSName); cmp != 0 {
				return cmp
			}
		}
	}
	if aProd.Prec != bProd.Prec {
		if aProd.Prec > bProd.Prec {
			return 1
		}
		return -1
	}
	if aProd.DynPrec != bProd.DynPrec {
		if aProd.DynPrec > bProd.DynPrec {
			return 1
		}
		return -1
	}
	aExplicit := aProd.HasExplicitPrec || aProd.Assoc != AssocNone
	bExplicit := bProd.HasExplicitPrec || bProd.Assoc != AssocNone
	if aExplicit != bExplicit {
		if aExplicit {
			return 1
		}
		return -1
	}
	return 0
}

func reduceLHSName(prod *Production, ng *NormalizedGrammar) string {
	if prod == nil || ng == nil || prod.LHS < 0 || prod.LHS >= len(ng.Symbols) {
		return ""
	}
	return ng.Symbols[prod.LHS].Name
}

func keepDeclaredSameRHSNeutralUnarySubset(reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) ([]lrAction, bool) {
	if len(reduces) < 3 || ng == nil || cache == nil {
		return nil, false
	}
	unaries := make([]hiddenUnaryPassthrough, 0, len(reduces))
	childByLHS := make(map[int]int, len(reduces))
	declared := make([]lrAction, 0, len(reduces))
	prec := 0
	dynPrec := 0
	for i, reduce := range reduces {
		u, ok := unaryReduce(reduce, ng)
		if !ok {
			return nil, false
		}
		prod := &ng.Productions[reduce.prodIdx]
		if i == 0 {
			prec = prod.Prec
			dynPrec = prod.DynPrec
		} else if prod.Prec != prec || prod.DynPrec != dynPrec {
			return nil, false
		}
		unaries = append(unaries, u)
		childByLHS[u.lhs] = u.child
		if reduceHasDeclaredConflictPartner(i, reduces, ng, cache) {
			declared = append(declared, reduce)
		}
	}
	child := -1
	for _, u := range unaries {
		leaf := neutralUnaryConflictLeaf(u.child, childByLHS, ng, cache)
		if child == -1 {
			child = leaf
		} else if child != leaf {
			return nil, false
		}
	}
	visible := visibleNeutralUnaryReduces(unaries, ng)
	if len(declared) < 2 {
		if len(visible) < 2 {
			return nil, false
		}
		return visible, true
	}
	if len(declared) == len(reduces) {
		return reduces, true
	}
	if !allInDeclaredConflict(declared, ng, cache) {
		if !hasDistinctVisibleReduceFamilies(declared, ng) {
			if len(visible) < 2 {
				return nil, false
			}
			return visible, true
		}
	}
	return declared, true
}

func visibleNeutralUnaryReduces(unaries []hiddenUnaryPassthrough, ng *NormalizedGrammar) []lrAction {
	visible := make([]lrAction, 0, len(unaries))
	seen := make(map[int]bool, len(unaries))
	for _, u := range unaries {
		if u.lhs < 0 || u.lhs >= len(ng.Symbols) {
			continue
		}
		info := ng.Symbols[u.lhs]
		if info.Kind != SymbolNonterminal || (!info.Visible && !info.Named) || seen[u.lhs] {
			continue
		}
		seen[u.lhs] = true
		visible = append(visible, u.action)
	}
	return visible
}

func neutralUnaryConflictLeaf(sym int, childByLHS map[int]int, ng *NormalizedGrammar, cache *conflictResolutionCache) int {
	seen := make(map[int]bool, len(childByLHS))
	for {
		if seen[sym] {
			return sym
		}
		seen[sym] = true
		if child, ok := childByLHS[sym]; ok {
			sym = child
			continue
		}
		if ng == nil || cache == nil || sym < 0 || sym >= len(cache.prodsByLHS) {
			return sym
		}
		prods := cache.prodsByLHS[sym]
		if len(prods) != 1 {
			return sym
		}
		prod := &ng.Productions[prods[0]]
		if len(prod.RHS) != 1 ||
			prod.Prec != 0 || prod.DynPrec != 0 || prod.Assoc != AssocNone || prod.HasExplicitPrec {
			return sym
		}
		sym = prod.RHS[0]
	}
}

func keepBashStatementBoundaryReduces(lookaheadSym int, reduces []lrAction, ng *NormalizedGrammar) ([]lrAction, bool) {
	if !bashStatementBoundaryLookahead(lookaheadSym, ng) {
		return nil, false
	}
	notSubshell, notPipeline, ok := bashStatementReducePair(reduces, ng)
	if !ok {
		return nil, false
	}
	return []lrAction{notSubshell, notPipeline}, true
}

func preferredBashStatementReduce(lookaheadSym int, reduces []lrAction, ng *NormalizedGrammar) (lrAction, bool) {
	notSubshell, notPipeline, ok := bashStatementReducePair(reduces, ng)
	if !ok {
		return lrAction{}, false
	}
	if lookaheadSym >= 0 && lookaheadSym < len(ng.Symbols) {
		switch ng.Symbols[lookaheadSym].Name {
		case "|", "|&":
			return notPipeline, true
		}
	}
	return notSubshell, true
}

func bashStatementReducePair(reduces []lrAction, ng *NormalizedGrammar) (lrAction, lrAction, bool) {
	if len(reduces) < 2 || ng == nil {
		return lrAction{}, lrAction{}, false
	}
	var notSubshell, notPipeline lrAction
	hasNotSubshell := false
	hasNotPipeline := false
	relevant := 0
	for _, reduce := range reduces {
		if reduce.kind != lrReduce || reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
			continue
		}
		prod := &ng.Productions[reduce.prodIdx]
		if prod.LHS < 0 || prod.LHS >= len(ng.Symbols) {
			continue
		}
		switch ng.Symbols[prod.LHS].Name {
		case "_statement_not_subshell":
			notSubshell = reduce
			hasNotSubshell = true
			relevant++
		case "_statement_not_pipeline":
			notPipeline = reduce
			hasNotPipeline = true
			relevant++
		}
	}
	return notSubshell, notPipeline, hasNotSubshell && hasNotPipeline && relevant == len(reduces)
}

func bashStatementBoundaryLookahead(lookaheadSym int, ng *NormalizedGrammar) bool {
	if ng == nil {
		return false
	}
	if lookaheadSym <= 0 {
		return true
	}
	if lookaheadSym >= len(ng.Symbols) {
		return false
	}
	switch ng.Symbols[lookaheadSym].Name {
	case "end", "$end", "\x00",
		"\\n", ";", ";;", "&", "&&", "||",
		"<", ">", "<<", "<<-", ">>", "<<<", "&>", "&>>", "<&", ">&", "<&-", ">&-", ">|",
		")", "}", "]",
		"then", "do", "else", "elif", "fi", "done", "esac":
		return true
	default:
		return false
	}
}

func shouldKeepTypeValueTokenReduces(lookaheadSym int, reduces []lrAction, ng *NormalizedGrammar) bool {
	if len(reduces) < 2 || ng == nil {
		return false
	}
	if lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) {
		return false
	}
	switch ng.Symbols[lookaheadSym].Name {
	case ">", "?", ":", ";", ",", ")", "]", "extends":
	default:
		return false
	}

	rhsSym := -1
	hasTypeLike := false
	hasValueLike := false
	for _, r := range reduces {
		if r.prodIdx < 0 || r.prodIdx >= len(ng.Productions) {
			return false
		}
		prod := ng.Productions[r.prodIdx]
		if len(prod.RHS) != 1 {
			return false
		}
		if rhsSym == -1 {
			rhsSym = prod.RHS[0]
		} else if prod.RHS[0] != rhsSym {
			return false
		}
		if prod.LHS < 0 || prod.LHS >= len(ng.Symbols) {
			return false
		}
		lhsName := ng.Symbols[prod.LHS].Name
		if isTypeLikeTokenWrapper(lhsName) {
			hasTypeLike = true
			continue
		}
		if isValueLikeTokenWrapper(lhsName) {
			hasValueLike = true
			continue
		}
		return false
	}
	return hasTypeLike && hasValueLike
}

func isTypeLikeTokenWrapper(name string) bool {
	switch name {
	case "type_identifier", "predefined_type", "literal_type", "primary_type":
		return true
	default:
		return false
	}
}

func isValueLikeTokenWrapper(name string) bool {
	switch name {
	case "identifier", "property_identifier", "primary_expression":
		return true
	default:
		return false
	}
}

// rrPrecResolve tries to resolve R/R conflicts via precedence. Returns nil
// if all reduces share the same (prec, dynPrec) and no resolution is possible.
func rrPrecResolve(reduces []lrAction, ng *NormalizedGrammar) []lrAction {
	// Check if there's a meaningful precedence difference.
	allSamePrec := true
	firstProd := &ng.Productions[reduces[0].prodIdx]
	for _, r := range reduces[1:] {
		rProd := &ng.Productions[r.prodIdx]
		if rProd.Prec != firstProd.Prec || rProd.DynPrec != firstProd.DynPrec {
			allSamePrec = false
			break
		}
	}
	if allSamePrec {
		return nil // no precedence difference — can't resolve
	}
	return rrPickBest(reduces, ng)
}

// rrPickBest selects the highest-precedence reduce from a set.
func rrPickBest(reduces []lrAction, ng *NormalizedGrammar) []lrAction {
	best := reduces[0]
	bestProd := &ng.Productions[best.prodIdx]
	for _, r := range reduces[1:] {
		rProd := &ng.Productions[r.prodIdx]
		if ng.PrecedenceOrder != nil {
			bestLHSName := ""
			if bestProd.LHS >= 0 && bestProd.LHS < len(ng.Symbols) {
				bestLHSName = ng.Symbols[bestProd.LHS].Name
			}
			rLHSName := ""
			if rProd.LHS >= 0 && rProd.LHS < len(ng.Symbols) {
				rLHSName = ng.Symbols[rProd.LHS].Name
			}

			// Tree-sitter's named precedence levels can outrank or lose to
			// symbol entries in the same precedence table. Preserve that for
			// reduce/reduce conflicts too, not just shift/reduce conflicts.
			if bestProd.Prec == 0 && rProd.Prec > 0 && bestLHSName != "" {
				cmp := ng.PrecedenceOrder.resolveSymbolVsNamedPrec(bestLHSName, rProd.Prec)
				if cmp > 0 {
					continue
				}
				if cmp < 0 {
					best = r
					bestProd = rProd
					continue
				}
			}
			if rProd.Prec == 0 && bestProd.Prec > 0 && rLHSName != "" {
				cmp := ng.PrecedenceOrder.resolveSymbolVsNamedPrec(rLHSName, bestProd.Prec)
				if cmp > 0 {
					best = r
					bestProd = rProd
					continue
				}
				if cmp < 0 {
					continue
				}
			}
			if bestProd.Prec == 0 && rProd.Prec == 0 && bestLHSName != "" && rLHSName != "" && bestLHSName != rLHSName {
				cmp := ng.PrecedenceOrder.resolveSymbolVsSymbol(rLHSName, bestLHSName)
				if cmp > 0 {
					best = r
					bestProd = rProd
					continue
				}
				if cmp < 0 {
					continue
				}
			}
		}
		if rProd.Prec > bestProd.Prec {
			best = r
			bestProd = rProd
		} else if rProd.Prec == bestProd.Prec {
			// Tree-sitter uses dynamic precedence as the next tiebreaker,
			// then explicit compile-time precedence/associativity metadata,
			// then falls back to production index (earlier declaration wins).
			// This matters for cases like TypeScript type_query, where
			// prec.right(0, ...) should outrank an implicit default-zero
			// primary_expression reduce even though both numeric prec values
			// are 0.
			if rProd.DynPrec > bestProd.DynPrec {
				best = r
				bestProd = rProd
			} else if rProd.DynPrec == bestProd.DynPrec {
				rExplicit := rProd.HasExplicitPrec || rProd.Assoc != AssocNone
				bestExplicit := bestProd.HasExplicitPrec || bestProd.Assoc != AssocNone
				if rExplicit != bestExplicit {
					if rExplicit {
						best = r
						bestProd = rProd
					}
				} else if r.prodIdx < best.prodIdx {
					best = r
					bestProd = rProd
				}
			}
		}
	}
	return []lrAction{best}
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
		if !ng.Symbols[lhs].GeneratedRepeatAux {
			return false
		}
	}
	return true
}

type hiddenUnaryWrapperReduce struct {
	index int
	lhs   int
	child int
}

type dropCandidate struct {
	index int
}

func filterRedundantHiddenUnaryWrapperReduces(reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) ([]lrAction, bool) {
	if len(reduces) < 3 || ng == nil {
		return reduces, false
	}

	reduceLHS := make(map[int]bool, len(reduces))
	for _, r := range reduces {
		if r.kind != lrReduce || r.prodIdx < 0 || r.prodIdx >= len(ng.Productions) {
			return reduces, false
		}
		prod := &ng.Productions[r.prodIdx]
		reduceLHS[prod.LHS] = true
	}

	var candidates []dropCandidate
	for i, r := range reduces {
		wrapper, ok := hiddenUnaryWrapperReduceAt(i, r, ng)
		if !ok || !neutralHiddenUnaryWrapperReduce(r, ng) {
			continue
		}
		enclosingCount := countEnclosingReducesForWrapper(reduces, wrapper, ng)
		if enclosingCount == 0 || (!reduceLHS[wrapper.child] && enclosingCount < 2) {
			continue
		}
		candidates = append(candidates, dropCandidate{index: i})
	}
	if len(candidates) == 0 {
		return reduces, false
	}

	drop := make(map[int]bool)
	hasUnprotectedCandidate := false
	for _, candidate := range candidates {
		if !reduceHasDeclaredConflictPartner(candidate.index, reduces, ng, cache) {
			hasUnprotectedCandidate = true
			break
		}
	}
	for _, candidate := range candidates {
		if hasUnprotectedCandidate {
			if reduceHasDeclaredConflictPartner(candidate.index, reduces, ng, cache) {
				continue
			}
			if hiddenUnaryCandidateEnclosesOtherCandidate(candidate.index, candidates, reduces, ng) {
				continue
			}
		}
		drop[candidate.index] = true
	}
	if len(drop) == 0 {
		return reduces, false
	}

	filtered := make([]lrAction, 0, len(reduces)-len(drop))
	for i, r := range reduces {
		if !drop[i] {
			filtered = append(filtered, r)
		}
	}
	if len(filtered) < 2 {
		return reduces, false
	}
	return filtered, true
}

func hiddenUnaryCandidateEnclosesOtherCandidate(index int, candidates []dropCandidate, reduces []lrAction, ng *NormalizedGrammar) bool {
	if index < 0 || index >= len(reduces) || ng == nil {
		return false
	}
	enclosing, ok := unaryReduce(reduces[index], ng)
	if !ok {
		return false
	}
	for _, candidate := range candidates {
		if candidate.index == index || candidate.index < 0 || candidate.index >= len(reduces) {
			continue
		}
		wrapped, ok := unaryReduce(reduces[candidate.index], ng)
		if ok && enclosing.child == wrapped.lhs {
			return true
		}
	}
	return false
}

func reduceHasDeclaredConflictPartner(index int, reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if index < 0 || index >= len(reduces) || cache == nil {
		return false
	}
	for i, r := range reduces {
		if i == index {
			continue
		}
		if allInDeclaredConflict([]lrAction{reduces[index], r}, ng, cache) {
			return true
		}
	}
	return false
}

func hiddenUnaryWrapperReduceAt(index int, action lrAction, ng *NormalizedGrammar) (hiddenUnaryWrapperReduce, bool) {
	if action.kind != lrReduce || action.prodIdx < 0 || action.prodIdx >= len(ng.Productions) {
		return hiddenUnaryWrapperReduce{}, false
	}
	prod := &ng.Productions[action.prodIdx]
	if len(prod.RHS) != 1 ||
		prod.LHS < 0 || prod.LHS >= len(ng.Symbols) ||
		prod.RHS[0] < 0 || prod.RHS[0] >= len(ng.Symbols) {
		return hiddenUnaryWrapperReduce{}, false
	}
	if !isHiddenUnaryWrapperSymbol(ng.Symbols[prod.LHS]) {
		return hiddenUnaryWrapperReduce{}, false
	}
	return hiddenUnaryWrapperReduce{index: index, lhs: prod.LHS, child: prod.RHS[0]}, true
}

func isHiddenUnaryWrapperSymbol(info SymbolInfo) bool {
	return info.Kind == SymbolNonterminal &&
		!info.Visible &&
		!info.Named &&
		!info.GeneratedRepeatAux
}

func neutralHiddenUnaryWrapperReduce(action lrAction, ng *NormalizedGrammar) bool {
	if action.kind != lrReduce || action.prodIdx < 0 || action.prodIdx >= len(ng.Productions) {
		return false
	}
	prod := &ng.Productions[action.prodIdx]
	return prod.Prec == 0 && prod.DynPrec == 0
}

func resolveTwoReduceHiddenUnaryPassthrough(reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) ([]lrAction, bool) {
	if len(reduces) != 2 || ng == nil {
		return nil, false
	}
	first, second := reduces[0], reduces[1]
	wrapper, enclosing, ok := hiddenUnaryPassthroughPair(first, second, ng)
	if !ok {
		wrapper, enclosing, ok = hiddenUnaryPassthroughPair(second, first, ng)
	}
	if !ok {
		return nil, false
	}
	if actualPairInDeclaredConflict(wrapper.lhs, enclosing.lhs, cache) {
		return reduces, true
	}
	if resolvedByPrec := rrPrecResolve(reduces, ng); resolvedByPrec != nil {
		return resolvedByPrec, true
	}
	return []lrAction{enclosing.action}, true
}

type hiddenUnaryPassthrough struct {
	action lrAction
	lhs    int
	child  int
}

func hiddenUnaryPassthroughPair(wrapperAction, enclosingAction lrAction, ng *NormalizedGrammar) (hiddenUnaryPassthrough, hiddenUnaryPassthrough, bool) {
	wrapper, ok := strictNeutralHiddenUnaryPassthrough(wrapperAction, ng)
	if !ok {
		return hiddenUnaryPassthrough{}, hiddenUnaryPassthrough{}, false
	}
	enclosing, ok := unaryReduce(enclosingAction, ng)
	if !ok || enclosing.child != wrapper.lhs {
		return hiddenUnaryPassthrough{}, hiddenUnaryPassthrough{}, false
	}
	return wrapper, enclosing, true
}

func strictNeutralHiddenUnaryPassthrough(action lrAction, ng *NormalizedGrammar) (hiddenUnaryPassthrough, bool) {
	wrapper, ok := unaryReduce(action, ng)
	if !ok {
		return hiddenUnaryPassthrough{}, false
	}
	if !isHiddenUnaryWrapperSymbol(ng.Symbols[wrapper.lhs]) {
		return hiddenUnaryPassthrough{}, false
	}
	prod := &ng.Productions[action.prodIdx]
	if prod.Prec != 0 || prod.DynPrec != 0 || prod.Assoc != AssocNone || prod.HasExplicitPrec {
		return hiddenUnaryPassthrough{}, false
	}
	return wrapper, true
}

func unaryReduce(action lrAction, ng *NormalizedGrammar) (hiddenUnaryPassthrough, bool) {
	if action.kind != lrReduce || action.prodIdx < 0 || action.prodIdx >= len(ng.Productions) {
		return hiddenUnaryPassthrough{}, false
	}
	prod := &ng.Productions[action.prodIdx]
	if len(prod.RHS) != 1 ||
		prod.LHS < 0 || prod.LHS >= len(ng.Symbols) ||
		prod.RHS[0] < 0 || prod.RHS[0] >= len(ng.Symbols) ||
		ng.Symbols[prod.LHS].Kind != SymbolNonterminal {
		return hiddenUnaryPassthrough{}, false
	}
	return hiddenUnaryPassthrough{action: action, lhs: prod.LHS, child: prod.RHS[0]}, true
}

func actualPairInDeclaredConflict(a, b int, cache *conflictResolutionCache) bool {
	if cache == nil || a == b {
		return false
	}
	for _, group := range cache.groups {
		hasA := false
		hasB := false
		for _, sym := range group {
			hasA = hasA || sym == a
			hasB = hasB || sym == b
		}
		if hasA && hasB {
			return true
		}
	}
	return false
}

func countEnclosingReducesForWrapper(reduces []lrAction, wrapper hiddenUnaryWrapperReduce, ng *NormalizedGrammar) int {
	count := 0
	for i, r := range reduces {
		if i == wrapper.index || r.kind != lrReduce || r.prodIdx < 0 || r.prodIdx >= len(ng.Productions) {
			continue
		}
		prod := &ng.Productions[r.prodIdx]
		if prod.LHS == wrapper.lhs || prod.LHS == wrapper.child {
			continue
		}
		if rhsContainsSymbol(prod.RHS, wrapper.lhs) || rhsContainsSymbol(prod.RHS, wrapper.child) {
			count++
		}
	}
	return count
}

func rhsContainsSymbol(rhs []int, sym int) bool {
	for _, rhsSym := range rhs {
		if rhsSym == sym {
			return true
		}
	}
	return false
}

func orderReduceConflictByChildCount(reduces []lrAction, ng *NormalizedGrammar) []lrAction {
	if len(reduces) < 2 || ng == nil {
		return reduces
	}
	ordered := append([]lrAction(nil), reduces...)
	sort.SliceStable(ordered, func(i, j int) bool {
		iLen := reduceChildCount(ordered[i], ng)
		jLen := reduceChildCount(ordered[j], ng)
		return iLen >= 0 && jLen >= 0 && iLen < jLen
	})
	return ordered
}

func reduceChildCount(action lrAction, ng *NormalizedGrammar) int {
	if action.kind != lrReduce || action.prodIdx < 0 || action.prodIdx >= len(ng.Productions) {
		return -1
	}
	return len(ng.Productions[action.prodIdx].RHS)
}

func shouldKeepNestedWrapperReduces(reduces []lrAction, ng *NormalizedGrammar) bool {
	if len(reduces) < 2 {
		return false
	}

	wrappedSyms := make(map[int]bool)
	wrapperSyms := make(map[int]bool)
	hasEnclosingReduce := false
	for i, r := range reduces {
		if wrapper, ok := hiddenUnaryWrapperReduceAt(i, r, ng); ok {
			wrappedSyms[wrapper.child] = true
			wrapperSyms[wrapper.lhs] = true
			continue
		}
		hasEnclosingReduce = true
	}
	if len(wrappedSyms) == 0 || !hasEnclosingReduce {
		return false
	}

	for i, r := range reduces {
		if _, ok := hiddenUnaryWrapperReduceAt(i, r, ng); ok {
			continue
		}
		prod := &ng.Productions[r.prodIdx]
		for _, sym := range prod.RHS {
			if wrappedSyms[sym] || wrapperSyms[sym] {
				return true
			}
		}
	}
	return false
}

// shouldKeepDistinctRepeatReduces returns true when all competing reduces
// produce distinct generated repeat helper symbols with the same precedence.
// These helpers serve different parent contexts — e.g. one parent requires a
// trailing ";" and the other doesn't — so picking one deterministically kills
// the other parse path. GLR preserves both paths until the parent production
// disambiguates.
func shouldKeepDistinctRepeatReduces(reduces []lrAction, ng *NormalizedGrammar) bool {
	if len(reduces) < 2 || ng == nil {
		return false
	}
	if reduces[0].kind != lrReduce || reduces[0].prodIdx < 0 || reduces[0].prodIdx >= len(ng.Productions) {
		return false
	}
	cache := getConflictResolutionCache(ng)
	if cache == nil {
		return false
	}
	// All must be generated repeat helpers and share the same (prec, dynPrec).
	firstProd := &ng.Productions[reduces[0].prodIdx]
	lhsSet := make(map[int]bool, len(reduces))
	for _, r := range reduces {
		if r.kind != lrReduce || r.prodIdx < 0 || r.prodIdx >= len(ng.Productions) {
			return false
		}
		prod := &ng.Productions[r.prodIdx]
		if prod.Prec != firstProd.Prec || prod.DynPrec != firstProd.DynPrec {
			return false // precedence differs — let rrPickBest resolve
		}
		if !isStructurallyGeneratedRepeatHelper(prod.LHS, ng, cache) {
			return false
		}
		lhsSet[prod.LHS] = true
	}
	// Must produce at least two distinct repeat helpers.
	return len(lhsSet) >= 2
}

func shouldKeepSameRHSExplicitNegativeReduces(lookaheadSym int, reduces []lrAction, ng *NormalizedGrammar) bool {
	if len(reduces) != 2 || ng == nil {
		return false
	}
	if lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) || ng.Symbols[lookaheadSym].Name != "{" {
		return false
	}
	seenScopedIdentifier := false
	seenScopedTypeInExpression := false
	seenNegativeScopedTypeInExpression := false
	var firstRHS []int
	for _, reduce := range reduces {
		if reduce.kind != lrReduce || reduce.prodIdx < 0 || reduce.prodIdx >= len(ng.Productions) {
			return false
		}
		prod := &ng.Productions[reduce.prodIdx]
		if prod.LHS < 0 || prod.LHS >= len(ng.Symbols) {
			return false
		}
		if firstRHS == nil {
			firstRHS = prod.RHS
		} else if !equalSymbolSeq(firstRHS, prod.RHS) {
			return false
		}
		switch ng.Symbols[prod.LHS].Name {
		case "scoped_identifier":
			seenScopedIdentifier = true
		case "scoped_type_identifier_in_expression_position":
			seenScopedTypeInExpression = true
			if prod.HasExplicitPrec && prod.Prec < 0 {
				seenNegativeScopedTypeInExpression = true
			}
		default:
			return false
		}
	}
	// C tree-sitter preserves both same-RHS wrapper reductions here so the
	// following "{" can still be parsed as a Rust struct expression. Choosing
	// only the numerically higher scoped_identifier reduce loses that branch
	// before parent context can disambiguate it.
	return seenScopedIdentifier && seenScopedTypeInExpression && seenNegativeScopedTypeInExpression
}

func equalSymbolSeq(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// shiftReduceInConflictGroup checks whether any (reduce LHS, shift LHS) pair
// appears together in a declared conflict group. This matches tree-sitter C's
// conflict resolution: keep S/R as GLR only when the symbols producing the
// shift and reduce are in the same declared conflict group.
func shiftReduceInConflictGroup(shifts, reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if cache != nil {
		cache.structuralStats.ConflictGroupShiftReduceCalls++
	}
	if cache == nil || len(cache.groups) == 0 {
		return false
	}

	shiftParents := resolvedShiftParentSet(shifts, ng, cache)
	reduceParents := resolvedReduceParentSet(reduces, ng, cache)
	key := conflictGroupPairKey(shiftParents, reduceParents)
	if cache.shiftReduceConflictGroupMemo != nil {
		if cached, ok := cache.shiftReduceConflictGroupMemo[key]; ok {
			cache.structuralStats.ConflictGroupShiftReduceHits++
			return cached
		}
	}
	cache.structuralStats.ConflictGroupShiftReduceMisses++

	result := conflictGroupSetsIntersect(shiftParents, reduceParents, cache)
	if cache.shiftReduceConflictGroupMemo == nil {
		cache.shiftReduceConflictGroupMemo = make(map[string]bool)
	}
	cache.shiftReduceConflictGroupMemo[key] = result
	return result
}

func resolvedShiftParentSet(shifts []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) []int {
	seen := make(map[int]bool)
	for _, s := range shifts {
		if s.lhsSym != 0 {
			for _, parent := range resolveAuxToParents(s.lhsSym, ng, cache) {
				seen[parent] = true
			}
		}
		for _, lhs := range s.lhsSyms {
			for _, parent := range resolveAuxToParents(lhs, ng, cache) {
				seen[parent] = true
			}
		}
	}
	return sortedSymbolSet(seen)
}

func resolvedReduceParentSet(reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) []int {
	seen := make(map[int]bool)
	for _, r := range reduces {
		if r.prodIdx < 0 || r.prodIdx >= len(ng.Productions) {
			continue
		}
		reduceLHS := ng.Productions[r.prodIdx].LHS
		for _, parent := range resolveAuxToParents(reduceLHS, ng, cache) {
			seen[parent] = true
		}
	}
	return sortedSymbolSet(seen)
}

func sortedSymbolSet(seen map[int]bool) []int {
	if len(seen) == 0 {
		return nil
	}
	symbols := make([]int, 0, len(seen))
	for sym := range seen {
		symbols = append(symbols, sym)
	}
	sort.Ints(symbols)
	return symbols
}

func conflictGroupPairKey(shiftParents, reduceParents []int) string {
	var b strings.Builder
	appendSymbolSetKey(&b, shiftParents)
	b.WriteByte('|')
	appendSymbolSetKey(&b, reduceParents)
	return b.String()
}

func appendSymbolSetKey(b *strings.Builder, symbols []int) {
	for i, sym := range symbols {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(sym))
	}
}

func conflictGroupSetsIntersect(shiftParents, reduceParents []int, cache *conflictResolutionCache) bool {
	if len(shiftParents) == 0 || len(reduceParents) == 0 {
		return false
	}
	shiftSet := make(map[int]bool, len(shiftParents))
	for _, parent := range shiftParents {
		shiftSet[parent] = true
	}
	for _, parent := range reduceParents {
		if parent < 0 || parent >= len(cache.groupsBySymbol) {
			continue
		}
		for _, groupIdx := range cache.groupsBySymbol[parent] {
			for _, sym := range cache.groups[groupIdx] {
				if shiftSet[sym] {
					return true
				}
			}
		}
	}
	return false
}

// resolveAuxToParents maps a symbol to its "parent" symbols for conflict
// group matching. Auxiliary symbols (generated repeat helpers, inline tokens)
// are traced back to the grammar symbols that reference them. Non-auxiliary
// symbols return themselves.
func resolveAuxToParents(sym int, ng *NormalizedGrammar, cache *conflictResolutionCache) []int {
	if sym < 0 || sym >= len(ng.Symbols) {
		return []int{sym}
	}
	if !isConflictAuxSymbol(sym, ng) {
		return []int{sym}
	}
	if cache != nil {
		return cache.resolveAuxToParents(sym, ng)
	}
	visited := make(map[int]bool)
	var parents []int
	resolveAuxToParentsRec(sym, ng, visited, &parents)
	if len(parents) == 0 {
		return []int{sym}
	}
	return parents
}

func isConflictAuxSymbol(sym int, ng *NormalizedGrammar) bool {
	if ng == nil || sym < 0 || sym >= len(ng.Symbols) {
		return false
	}
	info := ng.Symbols[sym]
	return info.GeneratedRepeatAux || strings.Contains(info.Name, "_token")
}

func (cache *conflictResolutionCache) resolveAuxToParents(sym int, ng *NormalizedGrammar) []int {
	if sym < 0 || sym >= len(cache.auxParents) || sym >= len(ng.Symbols) {
		return []int{sym}
	}
	if cache.auxComputed[sym] {
		return cache.auxParents[sym]
	}
	if cache.auxVisiting[sym] {
		return []int{sym}
	}
	cache.auxVisiting[sym] = true
	defer func() {
		cache.auxVisiting[sym] = false
		cache.auxComputed[sym] = true
		if len(cache.auxParents[sym]) == 0 {
			cache.auxParents[sym] = []int{sym}
		}
	}()

	if !isConflictAuxSymbol(sym, ng) {
		cache.auxParents[sym] = []int{sym}
		return cache.auxParents[sym]
	}

	seen := make(map[int]bool)
	parents := make([]int, 0, len(cache.rhsParents[sym]))
	for _, parentSym := range cache.rhsParents[sym] {
		for _, resolved := range cache.resolveAuxToParents(parentSym, ng) {
			if !seen[resolved] {
				seen[resolved] = true
				parents = append(parents, resolved)
			}
		}
	}
	sort.Ints(parents)
	cache.auxParents[sym] = parents
	return cache.auxParents[sym]
}

func resolveAuxToParentsRec(sym int, ng *NormalizedGrammar, visited map[int]bool, parents *[]int) {
	if visited[sym] {
		return
	}
	visited[sym] = true
	isAux := isConflictAuxSymbol(sym, ng)
	if !isAux {
		*parents = append(*parents, sym)
		return
	}
	found := false
	for _, prod := range ng.Productions {
		for _, rhsSym := range prod.RHS {
			if rhsSym == sym {
				found = true
				resolveAuxToParentsRec(prod.LHS, ng, visited, parents)
			}
		}
	}
	if !found {
		*parents = append(*parents, sym)
	}
}

// reduceLHSInAnyConflictGroup checks whether the primary reduce's LHS symbol
// appears in any declared conflict group. This is a broader check than
// shiftReduceInConflictGroup — it keeps GLR whenever the grammar author
// declared the reduce symbol as conflicting, regardless of what the shift is.
// Only the first reduce is checked to avoid creating excessive GLR forks
// from S/R/R conflicts where secondary reduces happen to have conflict-group LHS.
func reduceLHSInAnyConflictGroup(reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if cache != nil {
		cache.structuralStats.ConflictGroupReduceLHSCalls++
	}
	if cache == nil || len(cache.groups) == 0 || len(reduces) == 0 {
		return false
	}
	lhs := ng.Productions[reduces[0].prodIdx].LHS
	if cache.reduceLHSConflictGroupMemo != nil {
		if cached, ok := cache.reduceLHSConflictGroupMemo[lhs]; ok {
			cache.structuralStats.ConflictGroupReduceLHSHits++
			return cached
		}
	}
	cache.structuralStats.ConflictGroupReduceLHSMisses++
	result := false
	for _, parent := range resolveAuxToParents(lhs, ng, cache) {
		if parent >= 0 && parent < len(cache.groupsBySymbol) && len(cache.groupsBySymbol[parent]) > 0 {
			result = true
			break
		}
	}
	if cache.reduceLHSConflictGroupMemo == nil {
		cache.reduceLHSConflictGroupMemo = make(map[int]bool)
	}
	cache.reduceLHSConflictGroupMemo[lhs] = result
	return result
}

func allInDeclaredConflict(reduces []lrAction, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if len(reduces) < 2 || cache == nil {
		return false
	}
	for _, cgroup := range cache.groups {
		allFound := true
		for _, r := range reduces {
			lhs := ng.Productions[r.prodIdx].LHS
			// Resolve auxiliary symbols (repeat helpers, alias wrappers) to their
			// parent symbols for conflict group matching, mirroring the logic in
			// shiftReduceInConflictGroup.
			parentLHSs := resolveAuxToParents(lhs, ng, cache)
			found := false
			for _, parent := range parentLHSs {
				for _, sym := range cgroup {
					if sym == parent {
						found = true
						break
					}
				}
				if found {
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
