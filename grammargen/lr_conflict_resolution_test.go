package grammargen

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestResolveConflictsHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "item", Kind: SymbolNonterminal},
		},
		Productions: []Production{{LHS: 1}},
	}
	tables := &LRTables{
		ActionTable: map[int]map[int][]lrAction{
			0: {
				0: {
					{kind: lrShift, state: 1},
					{kind: lrReduce, prodIdx: 0},
				},
			},
		},
		StateCount: 1,
	}

	stats, err := resolveConflicts(ctx, tables, ng)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("resolveConflicts error = %v, want context.Canceled", err)
	}
	if stats.ConflictsResolved != 0 {
		t.Fatalf("conflicts resolved after cancellation = %d, want 0", stats.ConflictsResolved)
	}
}

func TestResolveShiftReduceUsesAdvancedItemPrecedence(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "{", Kind: SymbolTerminal},
			{Name: "lambda_literal", Kind: SymbolNonterminal},
			{Name: "call_expression", Kind: SymbolNonterminal},
			{Name: "_if_let_binding", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3},
		},
	}
	shift := lrAction{
		kind:    lrShift,
		state:   10,
		prec:    0,
		hasPrec: true,
		lhsSym:  1,
	}
	shift.addConflictContributor(2, -2, true, AssocNone)
	reduce := lrAction{kind: lrReduce, prodIdx: 0}

	got, err := resolveActionConflict(0, []lrAction{shift, reduce}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce {
		t.Fatalf("resolved actions = %+v, want the higher-precedence reduce", got)
	}
}

func TestResolveShiftReducePreservesEqualAdvancedSelfConflict(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "{", Kind: SymbolTerminal},
			{Name: "lambda_literal", Kind: SymbolNonterminal},
			{Name: "call_suffix", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, Prec: -2, HasExplicitPrec: true},
		},
		Conflicts: [][]int{{2}},
	}
	shift := lrAction{
		kind:    lrShift,
		state:   10,
		prec:    0,
		hasPrec: true,
		lhsSym:  1,
		lhsSyms: []int{2},
	}
	shift.addConflictContributor(2, -2, true, AssocNone)
	reduce := lrAction{kind: lrReduce, prodIdx: 0}

	got, err := resolveActionConflict(0, []lrAction{shift, reduce}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved actions = %+v, want both declared interpretations", got)
	}
}

func TestResolveShiftReduceDoesNotUseClosureItemPrecedence(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "{", Kind: SymbolTerminal},
			{Name: "lambda_literal", Kind: SymbolNonterminal},
			{Name: "_if_let_binding", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2},
		},
	}
	shift := lrAction{
		kind:    lrShift,
		state:   10,
		prec:    0,
		hasPrec: true,
		lhsSym:  1,
	}
	reduce := lrAction{kind: lrReduce, prodIdx: 0}

	got, err := resolveActionConflict(0, []lrAction{shift, reduce}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift {
		t.Fatalf("resolved actions = %+v, want the default shift", got)
	}
}

func TestRRPickBestUsesSymbolVsNamedPrecedenceOrder(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "declaration", Kind: SymbolNonterminal},
			{Name: "expression", Kind: SymbolNonterminal},
			{Name: "internal_module", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 0, RHS: []int{2}, Prec: 13, HasExplicitPrec: true},
			{LHS: 1, RHS: []int{2}},
		},
		PrecedenceOrder: &precOrderTable{
			symbolPositions:    map[string]int{"expression": 2},
			symbolLevels:       map[string]int{"expression": 0},
			namedPrecPositions: map[int]int{13: 1},
		},
	}

	got := rrPickBest([]lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
	}, ng)
	if len(got) != 1 || got[0].prodIdx != 1 {
		t.Fatalf("rrPickBest picked %+v, want expression reduce prodIdx=1", got)
	}
}

func TestResolveReduceReduceKeepsTypeValueSingleTokenAmbiguity(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: ">", Kind: SymbolTerminal},
			{Name: "string", Kind: SymbolNamedToken},
			{Name: "property_identifier", Kind: SymbolNonterminal},
			{Name: "predefined_type", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
			{LHS: 3, RHS: []int{1}},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved actions = %+v, want both reduces kept", got)
	}
}

func TestResolveReduceReduceKeepsDeclaredEqualRankVisibleFamilies(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "operand", Kind: SymbolNonterminal},
			{Name: "low_wrapper", Kind: SymbolNonterminal},
			{Name: "visible_path", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "visible_access", Kind: SymbolNonterminal, Visible: true, Named: true},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}, Prec: 1},
			{LHS: 3, RHS: []int{1}, Prec: 5},
			{LHS: 4, RHS: []int{1}, Prec: 5},
		},
		Conflicts: [][]int{{3, 4}},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
		{kind: lrReduce, prodIdx: 2},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 || got[0].prodIdx != 1 || got[1].prodIdx != 2 {
		t.Fatalf("resolved actions = %+v, want declared equal-rank visible reduces", got)
	}
}

func TestResolveReduceReduceKeepsDeclaredSameChildVisibleSubset(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "operand", Kind: SymbolNonterminal},
			{Name: "unrelated_visible", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "declared_value", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "declared_type", Kind: SymbolNonterminal, Visible: true, Named: true},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
			{LHS: 3, RHS: []int{1}},
			{LHS: 4, RHS: []int{1}},
		},
		Conflicts: [][]int{{3, 4}},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
		{kind: lrReduce, prodIdx: 2},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 || got[0].prodIdx != 1 || got[1].prodIdx != 2 {
		t.Fatalf("resolved actions = %+v, want declared same-child visible subset", got)
	}
}

func TestResolveReduceReduceKeepsDeclaredUnaryChainVisibleSubset(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "operand", Kind: SymbolNonterminal},
			{Name: "neutral_wrapper", Kind: SymbolNonterminal},
			{Name: "unrelated_visible", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "declared_value", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "declared_type", Kind: SymbolNonterminal, Visible: true, Named: true},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
			{LHS: 3, RHS: []int{1}},
			{LHS: 4, RHS: []int{1}},
			{LHS: 5, RHS: []int{2}},
		},
		Conflicts: [][]int{{4, 5}},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 1},
		{kind: lrReduce, prodIdx: 2},
		{kind: lrReduce, prodIdx: 3},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 || got[0].prodIdx != 2 || got[1].prodIdx != 3 {
		t.Fatalf("resolved actions = %+v, want declared unary-chain visible subset", got)
	}
}

func TestResolveReduceReduceKeepsSameLeafVisibleFamilies(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "operand", Kind: SymbolNonterminal},
			{Name: "hidden_wrapper", Kind: SymbolNonterminal},
			{Name: "visible_left", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "visible_right", Kind: SymbolNonterminal, Visible: true, Named: true},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
			{LHS: 3, RHS: []int{1}, HasExplicitPrec: true, Assoc: AssocLeft},
			{LHS: 4, RHS: []int{2}},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
		{kind: lrReduce, prodIdx: 2},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 || got[0].prodIdx != 1 || got[1].prodIdx != 2 {
		t.Fatalf("resolved actions = %+v, want same-leaf visible families", got)
	}
}

func TestResolveShiftReducePrefersVisibleSiblingAlternativeContinuation(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "(", Kind: SymbolTerminal},
			{Name: "atom", Kind: SymbolNonterminal},
			{Name: "argument_suffix", Kind: SymbolNonterminal},
			{Name: "wrapper_type", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "applied_type", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "token_wrapper", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{0}},
			{LHS: 3, RHS: []int{1}, Prec: 2, HasExplicitPrec: true, Assoc: AssocLeft},
			{LHS: 3, RHS: []int{4}},
			{LHS: 4, RHS: []int{5, 2}},
			{LHS: 5, RHS: []int{1}},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 1},
		{kind: lrReduce, prodIdx: 4},
		{kind: lrShift, state: 1, lhsSym: 2},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift {
		t.Fatalf("resolved actions = %+v, want longer visible sibling continuation shift", got)
	}
}

func TestResolveShiftReduceDoesNotPreferNonParenthesizedSiblingContinuation(t *testing.T) {
	for _, lookaheadName := range []string{":", "["} {
		t.Run(lookaheadName, func(t *testing.T) {
			ng := &NormalizedGrammar{
				Symbols: []SymbolInfo{
					{Name: lookaheadName, Kind: SymbolTerminal},
					{Name: "atom", Kind: SymbolNonterminal},
					{Name: "separator_suffix", Kind: SymbolNonterminal},
					{Name: "wrapper_type", Kind: SymbolNonterminal, Visible: true, Named: true},
					{Name: "continued_type", Kind: SymbolNonterminal, Visible: true, Named: true},
					{Name: "token_wrapper", Kind: SymbolNonterminal},
				},
				Productions: []Production{
					{LHS: 2, RHS: []int{0}},
					{LHS: 3, RHS: []int{1}, Prec: 2, HasExplicitPrec: true, Assoc: AssocLeft},
					{LHS: 3, RHS: []int{4}},
					{LHS: 4, RHS: []int{5, 2}},
					{LHS: 5, RHS: []int{1}},
				},
			}

			got, err := resolveActionConflict(0, []lrAction{
				{kind: lrReduce, prodIdx: 1},
				{kind: lrReduce, prodIdx: 4},
				{kind: lrShift, state: 1, lhsSym: 2},
			}, ng)
			if err != nil {
				t.Fatalf("resolveActionConflict: %v", err)
			}
			if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 1 {
				t.Fatalf("resolved actions = %+v, want precedence reduce for non-parenthesized continuation", got)
			}
		})
	}
}

func TestResolveReduceReduceKeepsBashStatementBoundaryReduces(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "|", Kind: SymbolTerminal},
			{Name: ">", Kind: SymbolTerminal},
			{Name: "_statement_not_subshell", Kind: SymbolNonterminal},
			{Name: "_statement_not_pipeline", Kind: SymbolNonterminal},
			{Name: "command", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{5}},
			{LHS: 4, RHS: []int{5}},
		},
	}
	reduces := []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
	}

	for _, lookahead := range []int{0, 2} {
		got, err := resolveActionConflict(lookahead, reduces, ng)
		if err != nil {
			t.Fatalf("resolveActionConflict(%d): %v", lookahead, err)
		}
		if len(got) != 2 || got[0].prodIdx != 0 || got[1].prodIdx != 1 {
			t.Fatalf("resolveActionConflict(%d) = %+v, want both Bash statement reductions", lookahead, got)
		}
	}
}

func TestResolveShiftReduceKeepsRepeatHelperConflictAmbiguity(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "-", Kind: SymbolTerminal},
			{Name: "class_character", Kind: SymbolNamedToken},
			{Name: "class_range", Kind: SymbolNonterminal},
			{Name: "character_class_repeat3", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "character_class", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1}},
			{LHS: 4, RHS: []int{3}},
		},
		Conflicts: [][]int{{2, 4}},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrShift, state: 10, lhsSym: 2, hasPrec: true, assoc: AssocRight},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved actions = %+v, want repeat reduce and class_range shift kept", got)
	}
}

func TestResolveReduceReduceKeepsGeneratedRepeatContinuationsAtBlockTerminator(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "block_end", Kind: SymbolTerminal},
			{Name: "item", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "end_item", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "header_items_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "body_items_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "header_prefix", Kind: SymbolNonterminal},
			{Name: "body", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{3}},
			{LHS: 3, RHS: []int{1}},
			{LHS: 4, RHS: []int{2, 2}},
			{LHS: 4, RHS: []int{4, 2}},
			{LHS: 5, RHS: []int{2, 2}},
			{LHS: 5, RHS: []int{5, 2}},
			{LHS: 6, RHS: []int{4, 3}, Prec: 1, HasExplicitPrec: true},
			{LHS: 7, RHS: []int{5}},
		},
	}

	got, err := resolveActionConflict(1, []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 2},
		{kind: lrReduce, prodIdx: 4},
		{kind: lrReduce, prodIdx: 6},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].prodIdx != 4 {
		t.Fatalf("resolved actions = %+v, want suffix-free generated repeat base reduction", got)
	}

	got, err = resolveActionConflict(1, []lrAction{
		{kind: lrReduce, prodIdx: 2},
		{kind: lrReduce, prodIdx: 4},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict pure repeats: %v", err)
	}
	if len(got) != 1 || got[0].prodIdx != 4 {
		t.Fatalf("pure repeat actions = %+v, want suffix-free generated repeat base reduction", got)
	}

	got, err = resolveActionConflict(1, []lrAction{
		{kind: lrReduce, prodIdx: 3},
		{kind: lrReduce, prodIdx: 5},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict recursive repeats: %v", err)
	}
	if len(got) != 1 || got[0].prodIdx != 5 {
		t.Fatalf("recursive repeat actions = %+v, want suffix-free generated repeat continuation reduction", got)
	}
}

func TestResolveReduceReduceDoesNotPreferSingleGeneratedRepeatOverUnrelatedReduce(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "block_end", Kind: SymbolTerminal},
			{Name: "item", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "end_item", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "items_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "unrelated", Kind: SymbolNonterminal, Visible: true, Named: true},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{3}},
			{LHS: 3, RHS: []int{1}},
			{LHS: 4, RHS: []int{4, 3}},
			{LHS: 5, RHS: []int{1}, Prec: 1, HasExplicitPrec: true},
		},
	}

	got, err := resolveActionConflict(1, []lrAction{
		{kind: lrReduce, prodIdx: 2},
		{kind: lrReduce, prodIdx: 3},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].prodIdx != 3 {
		t.Fatalf("resolved actions = %+v, want ordinary precedence resolution", got)
	}
}

func TestResolveReduceReducePrefersBashPipelineContinuationReduce(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "|", Kind: SymbolTerminal},
			{Name: "_statement_not_subshell", Kind: SymbolNonterminal},
			{Name: "_statement_not_pipeline", Kind: SymbolNonterminal},
			{Name: "command", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{4}},
			{LHS: 3, RHS: []int{4}},
		},
	}

	got, err := resolveActionConflict(1, []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].prodIdx != 1 {
		t.Fatalf("resolved actions = %+v, want _statement_not_pipeline reduce", got)
	}
}

func TestResolveReduceReduceDropsHiddenUnaryWrapperWhenChildReduceCompetes(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "token", Kind: SymbolTerminal},
			{Name: "part", Kind: SymbolNonterminal},
			{Name: "selector", Kind: SymbolNonterminal},
			{Name: "_wrapper", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
			{LHS: 3, RHS: []int{4}},
			{LHS: 4, RHS: []int{2}},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
		{kind: lrReduce, prodIdx: 2},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 || got[0].prodIdx != 0 || got[1].prodIdx != 1 {
		t.Fatalf("resolved actions = %+v, want child and enclosing reduces only", got)
	}
}

func TestResolveReduceReduceSuppressesHiddenUnaryPassthroughWithoutDeclaredPair(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "part", Kind: SymbolNonterminal},
			{Name: "selector", Kind: SymbolNonterminal},
			{Name: "_wrapper", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{3}},
			{LHS: 3, RHS: []int{1}},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].prodIdx != 0 {
		t.Fatalf("resolved actions = %+v, want enclosing selector reduce only", got)
	}
}

func TestResolveReduceReduceKeepsDeclaredHiddenUnaryPassthroughPair(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "part", Kind: SymbolNonterminal},
			{Name: "selector", Kind: SymbolNonterminal},
			{Name: "_wrapper", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{3}},
			{LHS: 3, RHS: []int{1}},
		},
		Conflicts: [][]int{{2, 3}},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 || got[0].prodIdx != 0 || got[1].prodIdx != 1 {
		t.Fatalf("resolved actions = %+v, want actual declared wrapper/enclosing pair preserved", got)
	}
}

func TestResolveReduceReduceSuppressesHiddenUnaryPassthroughDespiteDeclaredParentConflict(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "leaf", Kind: SymbolNonterminal},
			{Name: "_assignable_selector_part", Kind: SymbolNonterminal},
			{Name: "selector", Kind: SymbolNonterminal},
			{Name: "_assignable_selector", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{4}},
			{LHS: 3, RHS: []int{4}},
			{LHS: 4, RHS: []int{1}},
		},
		Conflicts: [][]int{{2, 3}},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 1},
		{kind: lrReduce, prodIdx: 2},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].prodIdx != 1 {
		t.Fatalf("resolved actions = %+v, want selector reduce only", got)
	}
}

func TestResolveReduceReduceDropsHiddenUnaryWrapperBetweenTwoEnclosingReduces(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "token", Kind: SymbolTerminal},
			{Name: "part", Kind: SymbolNonterminal},
			{Name: "selector", Kind: SymbolNonterminal},
			{Name: "_wrapper", Kind: SymbolNonterminal},
			{Name: "wrapped_leaf", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{4}},
			{LHS: 3, RHS: []int{4}},
			{LHS: 4, RHS: []int{5}},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
		{kind: lrReduce, prodIdx: 2},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 || got[0].prodIdx != 0 || got[1].prodIdx != 1 {
		t.Fatalf("resolved actions = %+v, want enclosing reduces only", got)
	}
}

func TestResolveReduceReduceFiltersHiddenUnaryWrapperBeforeDeclaredConflict(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "token", Kind: SymbolTerminal},
			{Name: "part", Kind: SymbolNonterminal},
			{Name: "selector", Kind: SymbolNonterminal},
			{Name: "_wrapper", Kind: SymbolNonterminal},
			{Name: "wrapped_leaf", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{4}},
			{LHS: 3, RHS: []int{4}},
			{LHS: 4, RHS: []int{5}},
		},
		Conflicts: [][]int{{2, 3}},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
		{kind: lrReduce, prodIdx: 2},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 || got[0].prodIdx != 0 || got[1].prodIdx != 1 {
		t.Fatalf("resolved actions = %+v, want declared parent conflict after dropping passthrough wrapper", got)
	}
}

func TestResolveReduceReduceKeepsDeclaredHiddenUnaryWrapperPair(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "token", Kind: SymbolTerminal},
			{Name: "_part", Kind: SymbolNonterminal},
			{Name: "selector", Kind: SymbolNonterminal},
			{Name: "_wrapper", Kind: SymbolNonterminal},
			{Name: "wrapped_leaf", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{5}},
			{LHS: 3, RHS: []int{5}},
			{LHS: 4, RHS: []int{5}},
		},
		Conflicts: [][]int{{2, 3}},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
		{kind: lrReduce, prodIdx: 2},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 || got[0].prodIdx != 0 || got[1].prodIdx != 1 {
		t.Fatalf("resolved actions = %+v, want declared hidden part and selector reduces only", got)
	}
}

func TestResolveReduceReduceKeepsDeclaredSameRHSUnarySubset(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "token", Kind: SymbolTerminal},
			{Name: "_expr", Kind: SymbolNonterminal},
			{Name: "_name", Kind: SymbolNonterminal},
			{Name: "_concatable", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
			{LHS: 3, RHS: []int{1}},
			{LHS: 4, RHS: []int{1}},
		},
		Conflicts: [][]int{{2, 4}},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
		{kind: lrReduce, prodIdx: 2},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 || got[0].prodIdx != 0 || got[1].prodIdx != 2 {
		t.Fatalf("resolved actions = %+v, want declared same-RHS unary subset", got)
	}
}

func TestResolveReduceReduceKeepsPairwiseDeclaredSameRHSUnaryGraph(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "token", Kind: SymbolTerminal},
			{Name: "_expr", Kind: SymbolNonterminal},
			{Name: "_name", Kind: SymbolNonterminal},
			{Name: "_concatable", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
			{LHS: 3, RHS: []int{1}},
			{LHS: 4, RHS: []int{1}},
		},
		Conflicts: [][]int{{2, 3}, {3, 4}},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
		{kind: lrReduce, prodIdx: 2},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("resolved actions = %+v, want pairwise-declared same-RHS unary graph kept", got)
	}
}

func TestResolveReduceReducePrecedenceUsesFilteredWrapperConflict(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "token", Kind: SymbolTerminal},
			{Name: "part", Kind: SymbolNonterminal},
			{Name: "selector", Kind: SymbolNonterminal},
			{Name: "_wrapper", Kind: SymbolNonterminal},
			{Name: "wrapped_leaf", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{4}, Prec: -1},
			{LHS: 3, RHS: []int{4}, Prec: -1},
			{LHS: 4, RHS: []int{5}},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
		{kind: lrReduce, prodIdx: 2},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 || got[0].prodIdx != 0 || got[1].prodIdx != 1 {
		t.Fatalf("resolved actions = %+v, want filtered parent conflict without resurrecting wrapper", got)
	}
}

func TestResolveReduceReduceOrdersFilteredWrapperConflictByChildCount(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "token_a", Kind: SymbolTerminal},
			{Name: "token_b", Kind: SymbolTerminal},
			{Name: "part", Kind: SymbolNonterminal},
			{Name: "selector", Kind: SymbolNonterminal},
			{Name: "_wrapper", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1, 2}},
			{LHS: 4, RHS: []int{5}},
			{LHS: 5, RHS: []int{3}},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 2},
		{kind: lrReduce, prodIdx: 1},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 || got[0].prodIdx != 1 || got[1].prodIdx != 0 {
		t.Fatalf("resolved actions = %+v, want enclosing one-child reduce before two-child child reduce", got)
	}
}

func TestResolveShiftReduceCanPreserveKeywordIdentifierCallAmbiguity(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "(", Kind: SymbolTerminal, Visible: true},
			{Name: "data", Kind: SymbolTerminal, Visible: true, Named: false},
			{Name: "identifier", Kind: SymbolNonterminal},
			{Name: "call_expression", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{2}},
		},
		KeywordSymbols:                     []int{2},
		WordSymbolID:                       3,
		PreserveKeywordIdentifierConflicts: true,
	}

	got, err := resolveActionConflict(1, []lrAction{
		{kind: lrShift, state: 10, lhsSym: 4, prec: 100},
		{kind: lrReduce, prodIdx: 0},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved actions = %+v, want keyword identifier ambiguity kept", got)
	}
}

func TestResolveShiftReducePreservesDerivedKeywordAliasCallAmbiguity(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "(", Kind: SymbolTerminal, Visible: true},
			{Name: "soft", Kind: SymbolTerminal, Visible: true, Named: false},
			{Name: "identifier", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "primary_expression", Kind: SymbolNonterminal},
			{Name: "soft_statement", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 4, RHS: []int{2}, Aliases: []AliasInfo{{ChildIndex: 0, Name: "identifier", Named: true}}},
		},
		KeywordSymbols:                     []int{2},
		WordSymbolID:                       3,
		PreserveKeywordIdentifierConflicts: true,
		DerivedKeywordIdentifierConflicts:  true,
	}

	got, err := resolveActionConflict(1, []lrAction{
		{kind: lrShift, state: 10, lhsSym: 5, prec: 100},
		{kind: lrReduce, prodIdx: 0},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved actions = %+v, want derived keyword alias ambiguity kept", got)
	}
}

func TestResolveShiftReducePrefersElixirExpressionOperatorIdentifierReduce(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "**", Kind: SymbolTerminal, Visible: true, Named: false},
			{Name: "identifier", Kind: SymbolNonterminal},
			{Name: "_expression", Kind: SymbolNonterminal},
			{Name: "operator_identifier", Kind: SymbolNonterminal},
			{Name: "binary_operator", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
			{LHS: 4, RHS: []int{2, 0, 2}},
		},
		PreferExpressionOperatorIdentifierReduces: true,
	}

	for _, tc := range []struct {
		name   string
		sym    string
		shift  lrAction
		reduce lrAction
	}{
		{
			name:   "atom to expression ignores operator identifier precedence",
			sym:    "**",
			shift:  lrAction{kind: lrShift, state: 10, lhsSym: 3, prec: 180, hasPrec: true},
			reduce: lrAction{kind: lrReduce, prodIdx: 0},
		},
		{
			name:   "completed binary operator",
			sym:    "**",
			shift:  lrAction{kind: lrShift, state: 10, lhsSym: 3},
			reduce: lrAction{kind: lrReduce, prodIdx: 1},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ng.Symbols[0].Name = tc.sym
			got, err := resolveActionConflict(0, []lrAction{
				tc.shift,
				tc.reduce,
			}, ng)
			if err != nil {
				t.Fatalf("resolveActionConflict: %v", err)
			}
			if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != tc.reduce.prodIdx {
				t.Fatalf("resolved actions = %+v, want reduce prodIdx=%d", got, tc.reduce.prodIdx)
			}
		})
	}
}

func TestResolveShiftReduceElixirOperatorIdentifierDoesNotPreferGenericArrowReduce(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "->", Kind: SymbolTerminal, Visible: true, Named: false},
			{Name: "identifier", Kind: SymbolNonterminal},
			{Name: "_expression", Kind: SymbolNonterminal},
			{Name: "operator_identifier", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
		},
		PreferExpressionOperatorIdentifierReduces: true,
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 10, lhsSym: 3, prec: 180, hasPrec: true},
		{kind: lrReduce, prodIdx: 0},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift {
		t.Fatalf("resolved actions = %+v, want generic operator_identifier shift for ->", got)
	}
}

func TestResolveShiftReduceElixirStabClauseArrowPreservesExpressionAmbiguity(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "->", Kind: SymbolTerminal, Visible: true, Named: false},
			{Name: "identifier", Kind: SymbolNonterminal},
			{Name: "_expression", Kind: SymbolNonterminal},
			{Name: "operator_identifier", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
		},
		PreferExpressionOperatorIdentifierReduces: true,
		PreferStabClauseLeftArrowReduces:          true,
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 10, lhsSym: 3, prec: 180, hasPrec: true},
		{kind: lrReduce, prodIdx: 0},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved actions = %+v, want arrow expression/operator ambiguity preserved", got)
	}
}

func TestResolveShiftReduceElixirOperatorIdentifierHonorsPrecedence(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "*", Kind: SymbolTerminal, Visible: true, Named: false},
			{Name: "_expression", Kind: SymbolNonterminal},
			{Name: "operator_identifier", Kind: SymbolNonterminal},
			{Name: "binary_operator", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1, 0, 1}, Prec: 10, Assoc: AssocLeft, HasExplicitPrec: true},
		},
		PreferExpressionOperatorIdentifierReduces: true,
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 10, lhsSym: 2, prec: 20, hasPrec: true},
		{kind: lrReduce, prodIdx: 0},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict higher shift precedence: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift {
		t.Fatalf("higher shift precedence actions = %+v, want operator_identifier shift", got)
	}

	got, err = resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 11, lhsSym: 2, prec: 10, hasPrec: true},
		{kind: lrReduce, prodIdx: 0},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict same precedence: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 0 {
		t.Fatalf("same precedence actions = %+v, want left-associative reduce", got)
	}
}

func TestResolveShiftReduceElixirOperatorIdentifierReduceRequiresExpressionShape(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "+", Kind: SymbolTerminal, Visible: true, Named: false},
			{Name: "operator_identifier", Kind: SymbolNonterminal},
			{Name: "unrelated", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{0}},
		},
		PreferExpressionOperatorIdentifierReduces: true,
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 10, lhsSym: 1},
		{kind: lrReduce, prodIdx: 0},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift {
		t.Fatalf("resolved actions = %+v, want operator_identifier shift", got)
	}
}

func TestResolveShiftReducePrefersElixirParenthesizedCallBeforeDoBlock(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "do", Kind: SymbolTerminal, Visible: true, Named: false},
			{Name: "identifier", Kind: SymbolNonterminal},
			{Name: "_call_arguments_with_parentheses_immediate", Kind: SymbolNonterminal},
			{Name: "do_block", Kind: SymbolNonterminal},
			{Name: "_local_call_with_parentheses", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 4, RHS: []int{1, 2}},
		},
		PreferParenthesizedCallDoBlockReduces: true,
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 10, lhsSym: 3},
		{kind: lrReduce, prodIdx: 0},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 0 {
		t.Fatalf("resolved actions = %+v, want parenthesized call reduce", got)
	}
}

func TestResolveShiftReducePrefersElixirRemoteCallBeforeDoBlock(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "do", Kind: SymbolTerminal, Visible: true, Named: false},
			{Name: "_remote_dot", Kind: SymbolNonterminal},
			{Name: "do_block", Kind: SymbolNonterminal},
			{Name: "_remote_call_without_parentheses", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1}},
		},
		PreferParenthesizedCallDoBlockReduces: true,
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 10, lhsSym: 2},
		{kind: lrReduce, prodIdx: 0},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 0 {
		t.Fatalf("resolved actions = %+v, want remote-call reduce before do_block", got)
	}
}

func TestResolveShiftReducePrefersElixirRemoteCallBeforeBinaryOperator(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "|>", Kind: SymbolTerminal, Visible: true, Named: false},
			{Name: "alias", Kind: SymbolNonterminal},
			{Name: "identifier", Kind: SymbolNonterminal},
			{Name: "_remote_call_without_parentheses", Kind: SymbolNonterminal},
			{Name: "binary_operator", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1, 2}},
		},
		PreferRemoteCallOperatorReduces: true,
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 10, lhsSym: 4},
		{kind: lrReduce, prodIdx: 0},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 0 {
		t.Fatalf("resolved actions = %+v, want remote-call reduce before binary operator", got)
	}
}

func TestResolveShiftReduceElixirRemoteCallReduceRequiresBinaryOperatorShift(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "-", Kind: SymbolTerminal, Visible: true, Named: false},
			{Name: "alias", Kind: SymbolNonterminal},
			{Name: "identifier", Kind: SymbolNonterminal},
			{Name: "_remote_call_without_parentheses", Kind: SymbolNonterminal},
			{Name: "unary_operator", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1, 2}},
		},
		PreferRemoteCallOperatorReduces: true,
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 10, lhsSym: 4},
		{kind: lrReduce, prodIdx: 0},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift {
		t.Fatalf("resolved actions = %+v, want ordinary shift without binary-operator continuation", got)
	}
}

func TestResolveShiftReducePrefersElixirStabClauseLeftBeforeArrow(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "->", Kind: SymbolTerminal, Visible: true, Named: false},
			{Name: "_stab_clause_arguments_without_parentheses", Kind: SymbolNonterminal},
			{Name: "_stab_clause_left", Kind: SymbolNonterminal},
			{Name: "stab_clause", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
		},
		PreferStabClauseLeftArrowReduces: true,
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 10, lhsSym: 3},
		{kind: lrReduce, prodIdx: 0},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 0 {
		t.Fatalf("resolved actions = %+v, want stab-clause-left reduce", got)
	}
}

func TestResolveShiftReducePrefersSpecificKeywordContinuation(t *testing.T) {
	tests := []struct {
		name  string
		shift lrAction
	}{
		{
			name:  "direct literal continuation",
			shift: lrAction{kind: lrShift, state: 10, lhsSym: 4},
		},
		{
			name:  "statement contributor continuation",
			shift: lrAction{kind: lrShift, state: 10, lhsSym: 5, lhsSyms: []int{6}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ng := &NormalizedGrammar{
				Symbols: []SymbolInfo{
					{Name: "end", Kind: SymbolTerminal},
					{Name: "(", Kind: SymbolTerminal, Visible: true},
					{Name: "null", Kind: SymbolTerminal, Visible: true, Named: false},
					{Name: "identifier", Kind: SymbolNonterminal},
					{Name: "null_literal", Kind: SymbolNonterminal},
					{Name: "_io_arguments", Kind: SymbolNonterminal},
					{Name: "open_statement", Kind: SymbolNonterminal},
				},
				Productions: []Production{
					{LHS: 3, RHS: []int{2}},
				},
				PreserveKeywordIdentifierConflicts: true,
			}

			got, err := resolveActionConflict(1, []lrAction{
				tc.shift,
				{kind: lrReduce, prodIdx: 0},
			}, ng)
			if err != nil {
				t.Fatalf("resolveActionConflict: %v", err)
			}
			if len(got) != 1 || got[0].kind != lrShift {
				t.Fatalf("resolved actions = %+v, want specific keyword shift", got)
			}
		})
	}
}

func TestResolveShiftReducePrefersRightAssocFinalOperandPostfixContinuation(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "(", Kind: SymbolTerminal, Visible: true},
			{Name: "?=", Kind: SymbolTerminal, Visible: true},
			{Name: "_expr", Kind: SymbolNonterminal},
			{Name: "cond_expr", Kind: SymbolNonterminal},
			{Name: "postfix_expr", Kind: SymbolNonterminal},
			{Name: "args", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{2, 1, 2}, Prec: 81, HasExplicitPrec: true, Assoc: AssocRight},
			{LHS: 4, RHS: []int{2, 5}, Prec: 80, HasExplicitPrec: true},
			{LHS: 5, RHS: []int{0}},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 17, lhsSym: 4, prec: 80, hasPrec: true},
		{kind: lrReduce, prodIdx: 0, lhsSym: 3, prec: 81, hasPrec: true, assoc: AssocRight},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift || got[0].state != 17 {
		t.Fatalf("resolved actions = %+v, want postfix continuation shift", got)
	}
}

func TestResolveShiftReduceDoesNotTreatLowerPrecedenceRepeatMarkerAsPostfixContinuation(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "*", Kind: SymbolTerminal, Visible: true},
			{Name: "@", Kind: SymbolTerminal, Visible: true},
			{Name: "pattern", Kind: SymbolNonterminal},
			{Name: "capture_pattern", Kind: SymbolNonterminal},
			{Name: "repeat_pattern", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{2, 1, 2}, Prec: 8, HasExplicitPrec: true, Assoc: AssocRight},
			{LHS: 4, RHS: []int{2, 0}, HasExplicitPrec: true, Assoc: AssocRight},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 17, lhsSym: 4, hasPrec: true, assoc: AssocRight},
		{kind: lrReduce, prodIdx: 0, lhsSym: 3, prec: 8, hasPrec: true, assoc: AssocRight},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 0 {
		t.Fatalf("resolved actions = %+v, want higher-precedence capture reduce", got)
	}
}

func TestResolveShiftReducePrefersRightAssocSameLHSOptionalPostfixContinuation(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "(", Kind: SymbolTerminal, Visible: true},
			{Name: "#", Kind: SymbolTerminal, Visible: true},
			{Name: "operand", Kind: SymbolNonterminal},
			{Name: "expr", Kind: SymbolNonterminal},
			{Name: "args", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1, 2}, Prec: 81, HasExplicitPrec: true, Assoc: AssocRight},
			{LHS: 3, RHS: []int{1, 2, 4}, Prec: 80, HasExplicitPrec: true, Assoc: AssocRight},
			{LHS: 4, RHS: []int{0}},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 17, lhsSym: 3, prec: 80, hasPrec: true},
		{kind: lrReduce, prodIdx: 0, lhsSym: 3, prec: 81, hasPrec: true, assoc: AssocRight},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift || got[0].state != 17 {
		t.Fatalf("resolved actions = %+v, want same-LHS optional postfix continuation shift", got)
	}
}

func TestResolveShiftReducePrefersRightAssocSameLHSOptionalPostfixContinuationFromUnannotatedPrefix(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "(", Kind: SymbolTerminal, Visible: true},
			{Name: "#", Kind: SymbolTerminal, Visible: true},
			{Name: "operand", Kind: SymbolNonterminal},
			{Name: "expr", Kind: SymbolNonterminal},
			{Name: "args", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1, 2}, Prec: 81},
			{LHS: 3, RHS: []int{1, 2, 4}, Prec: 80, HasExplicitPrec: true, Assoc: AssocRight},
			{LHS: 4, RHS: []int{0}},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 17, lhsSym: 3, prec: 80, hasPrec: true},
		{kind: lrReduce, prodIdx: 0, lhsSym: 3, prec: 81, hasPrec: true},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift || got[0].state != 17 {
		t.Fatalf("resolved actions = %+v, want same-LHS optional postfix continuation shift", got)
	}
}

func TestResolveShiftReduceDoesNotTreatSameLHSInfixTailAsRightAssocPostfixContinuation(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "*", Kind: SymbolTerminal, Visible: true},
			{Name: "#", Kind: SymbolTerminal, Visible: true},
			{Name: "operand", Kind: SymbolNonterminal},
			{Name: "expr", Kind: SymbolNonterminal},
			{Name: "tail", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1, 2}, Prec: 81, HasExplicitPrec: true, Assoc: AssocRight},
			{LHS: 3, RHS: []int{1, 2, 4}, Prec: 80, HasExplicitPrec: true, Assoc: AssocRight},
			{LHS: 4, RHS: []int{0, 2}},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 17, lhsSym: 4, prec: 80, hasPrec: true},
		{kind: lrReduce, prodIdx: 0, lhsSym: 3, prec: 81, hasPrec: true, assoc: AssocRight},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 0 {
		t.Fatalf("resolved actions = %+v, want ordinary higher-precedence reduce", got)
	}
}

func TestResolveShiftReduceDoesNotTreatInfixTailAsRightAssocPostfixContinuation(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "*", Kind: SymbolTerminal, Visible: true},
			{Name: "^", Kind: SymbolTerminal, Visible: true},
			{Name: "_expr", Kind: SymbolNonterminal},
			{Name: "outer_expr", Kind: SymbolNonterminal},
			{Name: "binary_expr", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{2, 1, 2}, Prec: 81, HasExplicitPrec: true, Assoc: AssocRight},
			{LHS: 4, RHS: []int{2, 0, 2}, Prec: 80, HasExplicitPrec: true},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 17, lhsSym: 4, prec: 80, hasPrec: true},
		{kind: lrReduce, prodIdx: 0, lhsSym: 3, prec: 81, hasPrec: true, assoc: AssocRight},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 0 {
		t.Fatalf("resolved actions = %+v, want ordinary higher-precedence reduce", got)
	}
}

func TestResolveShiftReduceDoesNotTreatFactoredInfixTailAsRightAssocPostfixContinuation(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "*", Kind: SymbolTerminal, Visible: true},
			{Name: "^", Kind: SymbolTerminal, Visible: true},
			{Name: "_expr", Kind: SymbolNonterminal},
			{Name: "outer_expr", Kind: SymbolNonterminal},
			{Name: "binary_expr", Kind: SymbolNonterminal},
			{Name: "infix_tail", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{2, 1, 2}, Prec: 81, HasExplicitPrec: true, Assoc: AssocRight},
			{LHS: 4, RHS: []int{2, 5}, Prec: 80, HasExplicitPrec: true},
			{LHS: 5, RHS: []int{0, 2}},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 17, lhsSym: 4, prec: 80, hasPrec: true},
		{kind: lrReduce, prodIdx: 0, lhsSym: 3, prec: 81, hasPrec: true, assoc: AssocRight},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 0 {
		t.Fatalf("resolved actions = %+v, want ordinary higher-precedence reduce", got)
	}
}

func TestResolveShiftReducePrefersArithmeticCloseDelimiter(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "))", Kind: SymbolTerminal},
			{Name: "_arithmetic_expression", Kind: SymbolNonterminal},
			{Name: "postfix_expression", Kind: SymbolNonterminal},
			{Name: "arithmetic_expansion", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 1, RHS: []int{2}, Prec: 1},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 12, prec: 1, lhsSym: 3},
		{kind: lrReduce, prodIdx: 0, lhsSym: 1},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift {
		t.Fatalf("resolved actions = %+v, want close-delimiter shift", got)
	}
}

func TestResolveShiftReduceFiltersLowerPrecedenceBeforeLeftAssociativity(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "operator", Kind: SymbolTerminal},
			{Name: "operand", Kind: SymbolNonterminal},
			{Name: "lower_wrapper", Kind: SymbolNonterminal},
			{Name: "choice", Kind: SymbolNonterminal},
			{Name: "infix", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
			{LHS: 3, RHS: []int{1}, Prec: 6, HasExplicitPrec: true, Assoc: AssocLeft},
			{LHS: 4, RHS: []int{1, 0, 1}, Prec: 6, HasExplicitPrec: true, Assoc: AssocLeft},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0, lhsSym: 2},
		{kind: lrReduce, prodIdx: 1, lhsSym: 3},
		{kind: lrReduce, prodIdx: 2, lhsSym: 4},
		{kind: lrShift, state: 9, lhsSym: 4, prec: 6, hasPrec: true, assoc: AssocLeft},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 2 {
		t.Fatalf("resolved actions = %+v, want completed left-associative infix reduce", got)
	}
}

func TestResolveShiftReducePreservesActionOrderAfterReductionFiltering(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "operator", Kind: SymbolTerminal},
			{Name: "operand", Kind: SymbolNonterminal},
			{Name: "lower_wrapper", Kind: SymbolNonterminal},
			{Name: "short_choice", Kind: SymbolNonterminal},
			{Name: "long_choice", Kind: SymbolNonterminal},
			{Name: "continuation", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
			{LHS: 3, RHS: []int{1}, Prec: 6, HasExplicitPrec: true, Assoc: AssocLeft},
			{LHS: 4, RHS: []int{1, 0, 1}, Prec: 6, HasExplicitPrec: true, Assoc: AssocLeft},
		},
		Conflicts: [][]int{{3, 4, 5}},
	}
	shift := lrAction{kind: lrShift, state: 9, lhsSym: 5, prec: 6, hasPrec: true, assoc: AssocLeft}
	shortReduce := lrAction{kind: lrReduce, prodIdx: 1, lhsSym: 3, prec: 6, hasPrec: true, assoc: AssocLeft}
	longReduce := lrAction{kind: lrReduce, prodIdx: 2, lhsSym: 4, prec: 6, hasPrec: true, assoc: AssocLeft}

	got, err := resolveActionConflict(0, []lrAction{
		shift,
		{kind: lrReduce, prodIdx: 0, lhsSym: 2},
		shortReduce,
		longReduce,
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	want := []lrAction{shift, shortReduce, longReduce}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolved actions = %+v, want shift-first order %+v", got, want)
	}
}

func TestHighestStaticReducePrecedenceTieRequiresTwoSurvivors(t *testing.T) {
	ng := &NormalizedGrammar{
		Productions: []Production{
			{Prec: 0},
			{Prec: 7, HasExplicitPrec: true, Assoc: AssocLeft},
		},
	}
	reduces := []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
	}

	got, ok := highestStaticReducePrecedenceTie(reduces, ng, getConflictResolutionCache(ng))
	if ok {
		t.Fatalf("highestStaticReducePrecedenceTie returned ok for one survivor: %+v", got)
	}
	if len(got) != len(reduces) {
		t.Fatalf("reduces = %+v, want the original ambiguity", got)
	}
}

func TestHighestStaticReducePrecedenceTieKeepsSymbolWinner(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "lookahead", Kind: SymbolTerminal},
			{Name: "named_a", Kind: SymbolNonterminal},
			{Name: "named_b", Kind: SymbolNonterminal},
			{Name: "symbol_winner", Kind: SymbolNonterminal},
			{Name: "shift", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 1, RHS: []int{0}, Prec: 13, HasExplicitPrec: true},
			{LHS: 2, RHS: []int{0}, Prec: 13, HasExplicitPrec: true},
			{LHS: 3, RHS: []int{0}},
		},
		PrecedenceOrder: &precOrderTable{
			symbolPositions:    map[string]int{"symbol_winner": 2},
			symbolLevels:       map[string]int{"symbol_winner": 0},
			namedPrecPositions: map[int]int{13: 1},
		},
	}
	reduces := []lrAction{
		{kind: lrReduce, prodIdx: 0, lhsSym: 1},
		{kind: lrReduce, prodIdx: 1, lhsSym: 2},
		{kind: lrReduce, prodIdx: 2, lhsSym: 3},
	}

	filtered, ok := highestStaticReducePrecedenceTie(reduces, ng, getConflictResolutionCache(ng))
	if !ok || len(filtered) != 1 || filtered[0].prodIdx != 2 {
		t.Fatalf("static precedence filter = %+v, %t, want symbol winner", filtered, ok)
	}
	actions := append([]lrAction{{kind: lrShift, state: 9, lhsSym: 4, prec: 13, hasPrec: true}}, reduces...)
	resolved, err := resolveActionConflict(0, actions, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(resolved) != 1 || resolved[0].kind != lrReduce || resolved[0].prodIdx != 2 {
		t.Fatalf("resolved actions = %+v, want symbol-precedence reduce", resolved)
	}
}

func TestHighestStaticReducePrecedenceTiePreservesLowerRepeatHelper(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "operator", Kind: SymbolTerminal},
			{Name: "item", Kind: SymbolNonterminal},
			{Name: "items_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "short_choice", Kind: SymbolNonterminal},
			{Name: "long_choice", Kind: SymbolNonterminal},
			{Name: "continuation", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
			{LHS: 2, RHS: []int{2, 1}},
			{LHS: 3, RHS: []int{1}, Prec: 6, HasExplicitPrec: true, Assoc: AssocLeft},
			{LHS: 4, RHS: []int{1, 0, 1}, Prec: 6, HasExplicitPrec: true, Assoc: AssocLeft},
		},
	}
	reduces := []lrAction{
		{kind: lrReduce, prodIdx: 1},
		{kind: lrReduce, prodIdx: 2},
		{kind: lrReduce, prodIdx: 3},
	}

	got, ok := highestStaticReducePrecedenceTie(reduces, ng, getConflictResolutionCache(ng))
	if ok {
		t.Fatalf("highestNumericReducePrecedenceTie removed a repeat helper: %+v", got)
	}
	if !reflect.DeepEqual(got, reduces) {
		t.Fatalf("reduces = %+v, want the original repeat ambiguity %+v", got, reduces)
	}

	resolved, err := resolveActionConflict(0, append([]lrAction{
		{kind: lrShift, state: 9, lhsSym: 5, prec: 6, hasPrec: true, assoc: AssocLeft},
	}, reduces...), ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(resolved) != 1 || resolved[0].kind != lrShift {
		t.Fatalf("resolved actions = %+v, want the unchanged shift result after the repeat-filter veto", resolved)
	}
}

func TestPreferredMixedLeftAssociativeReduceRequiresStrictPrefixFamily(t *testing.T) {
	ng := &NormalizedGrammar{
		Productions: []Production{
			{RHS: []int{1}, Prec: 6, DynPrec: 2, Assoc: AssocLeft},
			{RHS: []int{1, 0, 1}, Prec: 6, DynPrec: 2, Assoc: AssocLeft},
			{RHS: []int{1, 2}, Prec: 6, DynPrec: 2, Assoc: AssocLeft},
		},
	}
	reduces := []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
		{kind: lrReduce, prodIdx: 2},
	}

	if got, ok := preferredMixedLeftAssociativeReduce(reduces, ng); ok {
		t.Fatalf("preferredMixedLeftAssociativeReduce selected unrelated reduction %+v", got)
	}

	ng.Productions[2].RHS = []int{3, 2}
	if got, ok := preferredMixedLeftAssociativeReduce(reduces[:2], ng); !ok || got.prodIdx != 1 {
		t.Fatalf("preferredMixedLeftAssociativeReduce = (%+v, %t), want strict-prefix production 1", got, ok)
	}
	if got, ok := preferredMixedLeftAssociativeReduce([]lrAction{reduces[0], reduces[2]}, ng); ok {
		t.Fatalf("preferredMixedLeftAssociativeReduce selected different-prefix reduction %+v", got)
	}
}

func TestResolveShiftReducePrefersAnnotationTypeBeforeArguments(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "(", Kind: SymbolTerminal},
			{Name: "identifier", Kind: SymbolTerminal},
			{Name: "arguments", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "_simple_type", Kind: SymbolNonterminal},
			{Name: "_type_identifier", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1}},
			{LHS: 4, RHS: []int{1}},
		},
	}
	actions := []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
		{kind: lrShift, state: 7, lhsSym: 2, templateAnnotationContext: true},
	}

	got, err := resolveActionConflict(0, actions, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 0 {
		t.Fatalf("resolved actions = %+v, want annotation type reduction", got)
	}

	actions[2].templateAnnotationContext = false
	got, err = resolveActionConflict(0, actions, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict control: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift {
		t.Fatalf("control actions = %+v, want constructor argument shift", got)
	}

	actions[2].templateAnnotationContext = true
	t.Run("explicit shift precedence", func(t *testing.T) {
		guardActions := append([]lrAction(nil), actions...)
		guardActions[2].hasPrec = true
		if shouldPreferTemplateAnnotationTypeReduce(0, guardActions[2:], guardActions[:2], ng) {
			t.Fatal("annotation rule overrode explicit shift precedence")
		}
	})
	t.Run("explicit reduce precedence", func(t *testing.T) {
		guardGrammar := *ng
		guardGrammar.Productions = append([]Production(nil), ng.Productions...)
		guardGrammar.Productions[1].HasExplicitPrec = true
		got, err := resolveActionConflict(0, actions, &guardGrammar)
		if err != nil {
			t.Fatalf("resolve explicit reduce precedence: %v", err)
		}
		if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 1 {
			t.Fatalf("resolved actions = %+v, want the explicit reduction", got)
		}
	})
	t.Run("dynamic reduce precedence", func(t *testing.T) {
		guardGrammar := *ng
		guardGrammar.Productions = append([]Production(nil), ng.Productions...)
		guardGrammar.Productions[1].DynPrec = 1
		got, err := resolveActionConflict(0, actions, &guardGrammar)
		if err != nil {
			t.Fatalf("resolve dynamic reduce precedence: %v", err)
		}
		if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 1 {
			t.Fatalf("resolved actions = %+v, want the dynamic-precedence reduction", got)
		}
	})
}

func TestMergeShiftContributorsPreservesAnnotationContext(t *testing.T) {
	left := lrAction{kind: lrShift, lhsSym: 1}
	right := lrAction{kind: lrShift, lhsSym: 2, templateAnnotationContext: true}

	left.mergeShiftContributors(right)

	if !left.templateAnnotationContext {
		t.Fatal("merged shift lost its annotation context")
	}
}

func TestAddActionPreservesAnnotationContextFromExtraShift(t *testing.T) {
	tables := &LRTables{ActionTable: map[int]map[int][]lrAction{0: {}}}
	tables.addAction(0, 1, lrAction{kind: lrShift, state: 2})
	tables.addAction(0, 1, lrAction{
		kind:                      lrShift,
		state:                     2,
		isExtra:                   true,
		templateAnnotationContext: true,
	})

	actions := tables.ActionTable[0][1]
	if len(actions) != 1 {
		t.Fatalf("actions = %+v, want one merged shift", actions)
	}
	if actions[0].isExtra {
		t.Fatal("an extra shift replaced the normal shift")
	}
	if !actions[0].templateAnnotationContext {
		t.Fatal("the normal shift lost the annotation context from the extra shift")
	}
}

func TestTemplateContextTagPropagatesIntoAnnotationTypeContinuation(t *testing.T) {
	productions := make([]Production, 2000)
	productions[1998] = Production{LHS: 4, RHS: []int{1}}
	productions[1999] = Production{LHS: 3, RHS: []int{1, 2}}
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "identifier", Kind: SymbolTerminal},
			{Name: "arguments", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "applied_constructor_type", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "unrelated", Kind: SymbolNonterminal, Visible: true, Named: true},
		},
		Productions: productions,
	}
	ctx := &lrContext{
		ng:                        ng,
		annotationArgumentsSym:    2,
		appliedConstructorTypeSym: 3,
		bracedTemplateBodySym:     -1,
		bracedTemplateBody1Sym:    -1,
		bracedTemplateBody2Sym:    -1,
		itemSets: []lrItemSet{{
			annotationArgTag: templateContextPendingTag,
			cores:            []coreEntry{{prodIdx: 1999, dot: 0}},
		}},
	}
	continuation := &lrItemSet{cores: []coreEntry{{prodIdx: 1999, dot: 1}}}

	if got := ctx.templateContextTagForTransition(0, 1, continuation); got != templateContextPendingTag {
		t.Fatalf("continuation tag = %d, want %d", got, templateContextPendingTag)
	}
	ctx.itemSets[0].cores = []coreEntry{{prodIdx: 1998, dot: 0}}
	nonContinuation := &lrItemSet{cores: []coreEntry{{prodIdx: 1999, dot: 0}}}
	if got := ctx.templateContextTagForTransition(0, 1, nonContinuation); got != 0 {
		t.Fatalf("non-continuation tag = %d, want 0", got)
	}
	mixedTarget := &lrItemSet{cores: []coreEntry{
		{prodIdx: 1998, dot: 1},
		{prodIdx: 1999, dot: 1},
	}}
	if got := ctx.templateContextTagForTransition(0, 1, mixedTarget); got != 0 {
		t.Fatalf("unrelated transition tag = %d, want 0", got)
	}
}

func TestResolveShiftReduceUsesArithmeticExpressionReducePrecedence(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "+", Kind: SymbolTerminal},
			{Name: "*", Kind: SymbolTerminal},
			{Name: "||", Kind: SymbolTerminal},
			{Name: "_arithmetic_expression", Kind: SymbolNonterminal},
			{Name: "_arithmetic_literal", Kind: SymbolNonterminal},
			{Name: "binary_expression", Kind: SymbolNonterminal},
			{Name: "unary_expression", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{4}, Prec: 1},
			{LHS: 4, RHS: []int{4}, Prec: 1},
			{LHS: 5, RHS: []int{4, 0, 4}, Prec: 13, Assoc: AssocLeft, HasExplicitPrec: true},
			{LHS: 6, RHS: []int{0, 4}, Prec: 11, HasExplicitPrec: true},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0, lhsSym: 3},
		{kind: lrReduce, prodIdx: 1, lhsSym: 4},
		{kind: lrReduce, prodIdx: 2, lhsSym: 5},
		{kind: lrShift, state: 12, prec: 13, lhsSym: 5},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict same-precedence: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 2 {
		t.Fatalf("same-precedence actions = %+v, want left-associative binary reduce", got)
	}

	got, err = resolveActionConflict(1, []lrAction{
		{kind: lrReduce, prodIdx: 0, lhsSym: 3},
		{kind: lrReduce, prodIdx: 1, lhsSym: 4},
		{kind: lrReduce, prodIdx: 2, lhsSym: 5},
		{kind: lrShift, state: 13, prec: 14, lhsSym: 5},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict higher-shift: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift {
		t.Fatalf("higher-shift actions = %+v, want higher-precedence shift", got)
	}

	got, err = resolveActionConflict(2, []lrAction{
		{kind: lrReduce, prodIdx: 0, lhsSym: 3},
		{kind: lrReduce, prodIdx: 3, lhsSym: 6},
		{kind: lrShift, state: 14, prec: 3, lhsSym: 5},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict unary: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 3 {
		t.Fatalf("unary actions = %+v, want high-precedence unary reduce", got)
	}
}

func TestResolveShiftReduceShiftsArithmeticAssignmentFromWrapper(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "=", Kind: SymbolTerminal},
			{Name: "_arithmetic_expression", Kind: SymbolNonterminal},
			{Name: "_arithmetic_literal", Kind: SymbolNonterminal},
			{Name: "binary_expression", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 1, RHS: []int{2}, Prec: 1},
			{LHS: 2, RHS: []int{2}, Prec: 1},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0, lhsSym: 1},
		{kind: lrReduce, prodIdx: 1, lhsSym: 2},
		{kind: lrShift, state: 9, prec: 1, lhsSym: 3},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift {
		t.Fatalf("resolved actions = %+v, want arithmetic assignment shift", got)
	}
}

func TestResolveShiftReduceHonorsExplicitZeroAssignmentAssociativity(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "=", Kind: SymbolTerminal},
			{Name: "_expression", Kind: SymbolNonterminal},
			{Name: "assignment_expression", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{
				LHS:             2,
				RHS:             []int{1, 0, 1},
				Assoc:           AssocLeft,
				HasExplicitPrec: true,
			},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 9, lhsSym: 2},
		{kind: lrReduce, prodIdx: 0, lhsSym: 2},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 0 {
		t.Fatalf("resolved actions = %+v, want left-associative assignment reduce", got)
	}
}

func TestResolveShiftReducePrefersVisibleSameLHSOptionalTailShift(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "'", Kind: SymbolTerminal},
			{Name: "continue", Kind: SymbolTerminal},
			{Name: "label", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "continue_expression", Kind: SymbolNonterminal, Visible: true, Named: true},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1}},
			{LHS: 3, RHS: []int{1, 2}, HasExplicitPrec: true, Assoc: AssocLeft},
		},
	}
	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 7, lhsSym: 2, lhsSyms: []int{3}},
		{kind: lrReduce, prodIdx: 0, lhsSym: 3},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift || got[0].state != 7 {
		t.Fatalf("resolved actions = %+v, want visible optional-tail shift", got)
	}
}

func TestResolveShiftReduceDoesNotPreferHiddenSameLHSOptionalTailShift(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "{", Kind: SymbolTerminal},
			{Name: "break", Kind: SymbolTerminal},
			{Name: "block", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "_expression", Kind: SymbolNonterminal},
			{Name: "break_expression", Kind: SymbolNonterminal, Visible: true, Named: true},
		},
		Productions: []Production{
			{LHS: 4, RHS: []int{1}},
			{LHS: 4, RHS: []int{1, 3}, HasExplicitPrec: true, Assoc: AssocLeft},
		},
	}
	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 9, lhsSym: 2, lhsSyms: []int{3, 4}},
		{kind: lrReduce, prodIdx: 0, lhsSym: 4},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 0 {
		t.Fatalf("resolved actions = %+v, want hidden optional-tail reduce", got)
	}
}
func TestResolveShiftReduceKeepsImmediateShiftOverLowerPrecedenceContributor(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "|", Kind: SymbolTerminal},
			{Name: "identifier", Kind: SymbolTerminal},
			{Name: "closure_parameters", Kind: SymbolNonterminal},
			{Name: "_pattern", Kind: SymbolNonterminal},
			{Name: "or_pattern", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 4, RHS: []int{0, 3}, Prec: -2, HasExplicitPrec: true, Assoc: AssocLeft},
		},
	}
	tables := &LRTables{ActionTable: map[int]map[int][]lrAction{0: {}}}
	tables.addAction(0, 0, lrAction{kind: lrShift, state: 9, lhsSym: 2})
	tables.addAction(0, 0, lrAction{kind: lrShift, state: 9, lhsSym: 4, prec: -2, hasPrec: true, assoc: AssocLeft})

	actions := tables.ActionTable[0][0]
	cache := getConflictResolutionCache(ng)
	if meta := shiftMetadataForReduce(actions[0], 4, ng, cache); meta.prec != 0 {
		t.Fatalf("local shift metadata = %+v, want immediate closure-parameter shift precedence", meta)
	}
	got, err := resolveActionConflict(0, []lrAction{
		actions[0],
		{kind: lrReduce, prodIdx: 0, lhsSym: 4},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift || got[0].state != 9 {
		t.Fatalf("resolved actions = %+v, want immediate closure-parameter shift", got)
	}
}
func TestResolveShiftReduceUsesLocalShiftContributorPrecedence(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "{", Kind: SymbolTerminal},
			{Name: "break", Kind: SymbolTerminal},
			{Name: "block", Kind: SymbolNonterminal},
			{Name: "_expression", Kind: SymbolNonterminal},
			{Name: "break_expression", Kind: SymbolNonterminal},
			{Name: "call_expression", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 4, RHS: []int{1}},
			{LHS: 4, RHS: []int{1, 3}, HasExplicitPrec: true, Assoc: AssocLeft},
			{LHS: 3, RHS: []int{2}},
			{LHS: 3, RHS: []int{4}},
			{LHS: 5, RHS: []int{3}, Prec: 15, HasExplicitPrec: true},
		},
	}
	tables := &LRTables{ActionTable: map[int]map[int][]lrAction{0: {}}}
	tables.addAction(0, 0, lrAction{kind: lrShift, state: 9, lhsSym: 2})
	tables.addAction(0, 0, lrAction{kind: lrShift, state: 9, lhsSym: 4})
	tables.addAction(0, 0, lrAction{kind: lrShift, state: 9, lhsSym: 5, prec: 15, hasPrec: true})

	actions := tables.ActionTable[0][0]
	if len(actions) != 1 || actions[0].prec != 15 {
		t.Fatalf("merged shift actions = %+v, want one scalar high-precedence shift", actions)
	}
	cache := getConflictResolutionCache(ng)
	if inferred, ok := inferOptionalPrefixReduceMetadata(&ng.Productions[0], ng, cache); !ok || inferred.assoc != AssocLeft {
		t.Fatalf("optional-prefix metadata = %+v, %v; want left assoc", inferred, ok)
	}
	if meta := shiftMetadataForReduce(actions[0], 4, ng, cache); meta.prec != 0 || meta.assoc != AssocNone {
		t.Fatalf("local shift metadata = %+v, want zero/no-assoc break contributor", meta)
	}
	got, err := resolveActionConflict(0, []lrAction{
		actions[0],
		{kind: lrReduce, prodIdx: 0, lhsSym: 4},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 0 {
		t.Fatalf("resolved actions = %+v, want local break_expression reduce", got)
	}
}

func TestResolveShiftReduceUsesExactSameLHSContributorForMergedShift(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "..", Kind: SymbolTerminal},
			{Name: "_expression", Kind: SymbolNonterminal},
			{Name: "range_expression", Kind: SymbolNonterminal},
			{Name: "call_expression", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{0}, Prec: 1, HasExplicitPrec: true, Assoc: AssocLeft},
			{LHS: 2, RHS: []int{0, 1}, Prec: 1, HasExplicitPrec: true, Assoc: AssocLeft},
			{LHS: 1, RHS: []int{2}},
			{LHS: 1, RHS: []int{3}},
		},
		PrecedenceOrder: &precOrderTable{
			symbolPositions: map[string]int{
				"range_expression": 1,
				"call_expression":  2,
			},
			symbolLevels: map[string]int{
				"range_expression": 0,
				"call_expression":  0,
			},
		},
	}
	tables := &LRTables{ActionTable: map[int]map[int][]lrAction{0: {}}}
	tables.addAction(0, 0, lrAction{kind: lrShift, state: 9, lhsSym: 2, hasPrec: true})
	tables.addAction(0, 0, lrAction{kind: lrShift, state: 9, lhsSym: 2, prec: 1, hasPrec: true, assoc: AssocLeft})
	tables.addAction(0, 0, lrAction{kind: lrShift, state: 9, lhsSym: 1})
	tables.addAction(0, 0, lrAction{kind: lrShift, state: 9, lhsSym: 3, prec: 15, hasPrec: true})

	actions := tables.ActionTable[0][0]
	cache := getConflictResolutionCache(ng)
	meta := shiftMetadataForReduce(actions[0], 2, ng, cache)
	if meta.prec != 1 || meta.assoc != AssocLeft {
		t.Fatalf("range shift metadata = %+v, want exact precedence 1 and left associativity", meta)
	}
	if preferred, ok := preferredSameLHSContinuationShift(
		[]lrAction{actions[0]},
		[]lrAction{{kind: lrReduce, prodIdx: 0, lhsSym: 2}},
		ng,
		cache,
	); ok {
		t.Fatalf("merged same-LHS continuation = %+v, want explicit associativity", preferred)
	}
	got, err := resolveActionConflict(0, []lrAction{
		actions[0],
		{kind: lrReduce, prodIdx: 0, lhsSym: 2},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 0 {
		t.Fatalf("resolved actions = %+v, want exact range reduction", got)
	}
}

func TestResolveShiftReducePrefersSameLHSContinuationShift(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "(", Kind: SymbolTerminal},
			{Name: "prefix", Kind: SymbolTerminal},
			{Name: "selector", Kind: SymbolNonterminal},
			{Name: "section", Kind: SymbolNonterminal},
			{Name: "repeat_args", Kind: SymbolNonterminal},
			{Name: "argument_part", Kind: SymbolNonterminal},
			{Name: "arguments", Kind: SymbolNonterminal},
			{Name: "unrelated", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1, 2}, Assoc: AssocLeft, HasExplicitPrec: true},
			{LHS: 3, RHS: []int{1, 2, 4}, Assoc: AssocLeft, HasExplicitPrec: true},
			{LHS: 4, RHS: []int{5}},
			{LHS: 5, RHS: []int{6}},
			{LHS: 7, RHS: []int{6}},
		},
	}
	if cache := getConflictResolutionCache(ng); cache == nil {
		t.Fatal("conflict cache is nil for grammar without declared conflicts")
	} else if len(cache.groups) != 0 || len(cache.prodsByLHS[3]) != 2 {
		t.Fatalf("conflict cache = groups:%d sectionProds:%v, want no groups and two section productions", len(cache.groups), cache.prodsByLHS[3])
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 9, lhsSym: 6},
		{kind: lrReduce, prodIdx: 0, lhsSym: 3},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift || got[0].state != 9 {
		t.Fatalf("resolved actions = %+v, want continuation shift", got)
	}

	got, err = resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 9, lhsSym: 7},
		{kind: lrReduce, prodIdx: 0, lhsSym: 3},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict unrelated: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 0 {
		t.Fatalf("unrelated actions = %+v, want normal left-associative reduce", got)
	}

	cache := getConflictResolutionCache(ng)
	if preferred, ok := preferredSameLHSContinuationShift(
		[]lrAction{{kind: lrShift, state: 9, lhsSym: 6}},
		[]lrAction{{kind: lrReduce, prodIdx: 0, lhsSym: 3}, {kind: lrReduce, prodIdx: 2, lhsSym: 4}},
		ng,
		cache,
	); ok {
		t.Fatalf("multi-reduce continuation helper = %+v, want no preference", preferred)
	}
	if preferred, ok := preferredSameLHSContinuationShift(
		[]lrAction{{kind: lrShift, state: 9, lhsSym: 6}, {kind: lrShift, state: 10, lhsSym: 6}},
		[]lrAction{{kind: lrReduce, prodIdx: 0, lhsSym: 3}},
		ng,
		cache,
	); ok {
		t.Fatalf("two-shift continuation helper = %+v, want no preference", preferred)
	}

	ng.Conflicts = [][]int{{3, 6}}
	ng.conflictCache = nil
	got, err = resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 9, lhsSym: 6},
		{kind: lrReduce, prodIdx: 0, lhsSym: 3},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict pairwise declared conflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("pairwise declared conflict actions = %+v, want GLR preserved", got)
	}

	ng.Conflicts = [][]int{{3, 7}}
	ng.conflictCache = nil
	got, err = resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 9, lhsSym: 6},
		{kind: lrReduce, prodIdx: 0, lhsSym: 3},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict reduce-LHS declared conflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift || got[0].state != 9 {
		t.Fatalf("reduce-LHS conflict actions = %+v, want continuation shift after non-pairwise conflict handling", got)
	}
}

func TestResolveShiftReducePrefersLoweredRepeatContinuationShift(t *testing.T) {
	ng := loweredRepeatContinuationGrammar()

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 9, lhsSym: 2},
		{kind: lrReduce, prodIdx: 0, lhsSym: 3},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift || got[0].state != 9 {
		t.Fatalf("resolved actions = %+v, want lowered-repeat continuation shift", got)
	}
}

func TestResolveShiftReducePrefersLoweredRepeatContinuationBeforeParentPrecedence(t *testing.T) {
	ng := loweredRepeatContinuationGrammar()
	ng.Productions[0].Prec = 10
	ng.Productions[0].HasExplicitPrec = true
	ng.Productions[1].Prec = 10
	ng.Productions[1].HasExplicitPrec = true

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 9, lhsSym: 2, prec: 9, hasPrec: true},
		{kind: lrReduce, prodIdx: 0, lhsSym: 3, prec: 10, hasPrec: true},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift || got[0].state != 9 {
		t.Fatalf("resolved actions = %+v, want lowered-repeat continuation shift despite parent precedence", got)
	}
}

func TestResolveShiftReducePrefersLoweredRepeatPartialUnitContinuation(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "cast", Kind: SymbolTerminal},
			{Name: "|", Kind: SymbolTerminal},
			{Name: "prefix", Kind: SymbolNonterminal},
			{Name: "unit", Kind: SymbolNonterminal},
			{Name: "section", Kind: SymbolNonterminal},
			{Name: "section_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "cast_expression", Kind: SymbolNonterminal},
			{Name: "cast_tail", Kind: SymbolNonterminal},
			{Name: "unrelated", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 4, RHS: []int{2, 1, 3}, Prec: 10, Assoc: AssocLeft, HasExplicitPrec: true},
			{LHS: 4, RHS: []int{2, 5}, Prec: 10, Assoc: AssocLeft, HasExplicitPrec: true},
			{LHS: 3, RHS: []int{0}, Prec: 9, HasExplicitPrec: true},
			{LHS: 5, RHS: []int{1, 3}},
			{LHS: 5, RHS: []int{5, 1, 3}},
			{LHS: 6, RHS: []int{3, 7}, Prec: 9, HasExplicitPrec: true},
			{LHS: 7, RHS: []int{0}},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 9, lhsSym: 7, lhsSyms: []int{6}, prec: 9, hasPrec: true},
		{kind: lrReduce, prodIdx: 0, lhsSym: 4, prec: 10, hasPrec: true},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift || got[0].state != 9 {
		t.Fatalf("resolved actions = %+v, want partial lowered-repeat unit continuation shift", got)
	}

	got, err = resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 9, lhsSym: 8, prec: 9, hasPrec: true},
		{kind: lrReduce, prodIdx: 0, lhsSym: 4, prec: 10, hasPrec: true},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict unrelated shift: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 0 {
		t.Fatalf("unrelated shift actions = %+v, want parent reduce when unit continuation is not proven", got)
	}
}

func TestResolveShiftReduceDoesNotUseRepeatNameAsLoweredRepeatProof(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "op", Kind: SymbolTerminal},
			{Name: "prefix", Kind: SymbolNonterminal},
			{Name: "unit", Kind: SymbolNonterminal},
			{Name: "section", Kind: SymbolNonterminal},
			{Name: "ordinary_repeat_named", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1, 2}, Prec: 10, Assoc: AssocLeft, HasExplicitPrec: true},
			{LHS: 3, RHS: []int{1, 4}, Prec: 10, Assoc: AssocLeft, HasExplicitPrec: true},
			{LHS: 2, RHS: []int{0}, Prec: 9, HasExplicitPrec: true},
			{LHS: 4, RHS: []int{2}},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 9, lhsSym: 2, prec: 9, hasPrec: true},
		{kind: lrReduce, prodIdx: 0, lhsSym: 3, prec: 10, hasPrec: true},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 0 {
		t.Fatalf("resolved actions = %+v, want parent precedence reduce for non-repeat symbol with repeat in name", got)
	}
}

func TestResolveActionConflictKeepsRecursiveRepeatShiftByLookaheadFirstSet(t *testing.T) {
	ng := loweredRepeatContinuationGrammar()

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 9, lhsSym: 2},
		{kind: lrReduce, prodIdx: 3, lhsSym: 4},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved actions = %+v, want recursive repeat reduce plus shift", got)
	}
	if got[0].kind != lrReduce || got[0].prodIdx != 3 {
		t.Fatalf("first action = %+v, want recursive repeat reduce", got[0])
	}
	if got[1].kind != lrShift || !got[1].repeat || got[1].state != 9 {
		t.Fatalf("second action = %+v, want repeat shift", got[1])
	}
}

func TestResolveShiftReduceKeepsLoweredRepeatMixedContinuation(t *testing.T) {
	ng := loweredRepeatContinuationGrammar()

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 9, lhsSym: 2},
		{kind: lrReduce, prodIdx: 0, lhsSym: 3},
		{kind: lrReduce, prodIdx: 4, lhsSym: 4},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved actions = %+v, want repeat-helper reduce plus shift", got)
	}
	if got[0].kind != lrReduce || got[0].prodIdx != 4 {
		t.Fatalf("first action = %+v, want repeat-helper tail reduce", got[0])
	}
	if got[1].kind != lrShift || !got[1].repeat || got[1].state != 9 {
		t.Fatalf("second action = %+v, want repeat shift", got[1])
	}
}

func TestResolveShiftReduceKeepsVisibleTailRepeatBoundary(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "label", Kind: SymbolTerminal},
			{Name: ":", Kind: SymbolTerminal},
			{Name: "statement", Kind: SymbolNonterminal},
			{Name: "group", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "group_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "statement_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{0, 1, 5}, Assoc: AssocLeft},
			{LHS: 4, RHS: []int{3}},
			{LHS: 4, RHS: []int{4, 3}},
			{LHS: 5, RHS: []int{2}},
			{LHS: 5, RHS: []int{5, 2}},
			{LHS: 2, RHS: []int{0}},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 9, lhsSym: 2},
		{kind: lrReduce, prodIdx: 0, lhsSym: 3},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved actions = %+v, want boundary GLR preserved", got)
	}
	if got[0].kind != lrShift || got[1].kind != lrReduce {
		t.Fatalf("resolved actions = %+v, want original shift/reduce boundary order", got)
	}
}

func loweredRepeatContinuationGrammar() *NormalizedGrammar {
	return &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "op", Kind: SymbolTerminal},
			{Name: "prefix", Kind: SymbolNonterminal},
			{Name: "unit", Kind: SymbolNonterminal},
			{Name: "section", Kind: SymbolNonterminal},
			{Name: "section_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1, 2}, Assoc: AssocLeft, HasExplicitPrec: true},
			{LHS: 3, RHS: []int{1, 4}, Assoc: AssocLeft, HasExplicitPrec: true},
			{LHS: 2, RHS: []int{0}},
			{LHS: 4, RHS: []int{4, 2}},
			{LHS: 4, RHS: []int{2, 2}},
		},
	}
}

func TestResolveShiftReduceUsesContributorSymbolVsNamedPrecedence(t *testing.T) {
	ng := javascriptUpdateExpressionConflictGrammar(false)

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 9, lhsSym: 3, lhsSyms: []int{4}},
		{kind: lrReduce, prodIdx: 0, lhsSym: 2},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift {
		t.Fatalf("resolved actions = %+v, want update_expression contributor shift", got)
	}
}

func TestResolveShiftReduceUsesContributorSymbolVsNamedPrecedenceInConflictGroup(t *testing.T) {
	ng := javascriptUpdateExpressionConflictGrammar(true)

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 9, lhsSym: 3, lhsSyms: []int{-1, 4, 4}},
		{kind: lrReduce, prodIdx: 0, lhsSym: 2},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift {
		t.Fatalf("resolved actions = %+v, want update_expression contributor shift", got)
	}
}

func TestResolveShiftReduceUsesContributorSymbolVsSymbolPrecedence(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "[", Kind: SymbolTerminal},
			{Name: "type_query", Kind: SymbolNonterminal},
			{Name: "primary_expression", Kind: SymbolNonterminal},
			{Name: "_type_query_subscript_expression", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 1, RHS: []int{}, Assoc: AssocRight, HasExplicitPrec: true},
		},
		PrecedenceOrder: &precOrderTable{
			symbolPositions: map[string]int{
				"type_query":                       2,
				"_type_query_subscript_expression": 1,
			},
			symbolLevels: map[string]int{
				"type_query":                       0,
				"_type_query_subscript_expression": 0,
			},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 9, lhsSym: 2, lhsSyms: []int{3}},
		{kind: lrReduce, prodIdx: 0, lhsSym: 1},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce {
		t.Fatalf("resolved actions = %+v, want type_query reduce", got)
	}
}

func javascriptUpdateExpressionConflictGrammar(withConflictGroup bool) *NormalizedGrammar {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "||", Kind: SymbolTerminal},
			{Name: "identifier", Kind: SymbolNonterminal},
			{Name: "binary_expression", Kind: SymbolNonterminal},
			{Name: "unrelated_expression", Kind: SymbolNonterminal},
			{Name: "update_expression", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1, 0, 1}, Prec: 5, Assoc: AssocLeft, HasExplicitPrec: true},
		},
		PrecedenceOrder: &precOrderTable{
			symbolPositions:    map[string]int{"update_expression": 2},
			symbolLevels:       map[string]int{"update_expression": 0},
			namedPrecPositions: map[int]int{5: 1},
		},
	}
	if withConflictGroup {
		ng.Conflicts = [][]int{{2, 99}}
	}
	return ng
}

func TestResolveShiftReduceKeepsExpressionStructInitializerAmbiguity(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "{", Kind: SymbolTerminal},
			{Name: "identifier", Kind: SymbolNonterminal},
			{Name: "_expression_except_range", Kind: SymbolNonterminal},
			{Name: "field_initializer_list", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{
				LHS:             2,
				RHS:             []int{1},
				Assoc:           AssocLeft,
				HasExplicitPrec: true,
			},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 9, lhsSym: 3},
		{kind: lrReduce, prodIdx: 0, lhsSym: 2},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved actions = %+v, want expression reduce and field initializer shift kept", got)
	}
}

func TestResolveShiftReducePrefersCompletedClosureParameters(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "identifier", Kind: SymbolNamedToken},
			{Name: "closure_parameters", Kind: SymbolNonterminal},
			{Name: "generic_type", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 1, RHS: []int{0}, Prec: 0},
			{LHS: 2, RHS: []int{0}, Prec: 1},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0, lhsSym: 1},
		{kind: lrShift, state: 10, lhsSym: 2, prec: 1, hasPrec: true},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].lhsSym != 1 {
		t.Fatalf("resolved actions = %+v, want closure_parameters reduce", got)
	}
}

func TestResolveShiftReduceStructInitializerAmbiguityRequiresExpressionReduce(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "{", Kind: SymbolTerminal},
			{Name: "identifier", Kind: SymbolNonterminal},
			{Name: "call_expression", Kind: SymbolNonterminal},
			{Name: "field_initializer_list", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{
				LHS:             2,
				RHS:             []int{1},
				Assoc:           AssocLeft,
				HasExplicitPrec: true,
			},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 9, lhsSym: 3},
		{kind: lrReduce, prodIdx: 0, lhsSym: 2},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce {
		t.Fatalf("resolved actions = %+v, want normal explicit-left reduce", got)
	}
}

func TestResolveReduceReduceKeepsSameRHSExplicitNegativeAmbiguity(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "{", Kind: SymbolTerminal},
			{Name: "identifier", Kind: SymbolNonterminal},
			{Name: "::", Kind: SymbolTerminal},
			{Name: "scoped_identifier", Kind: SymbolNonterminal},
			{Name: "scoped_type_identifier_in_expression_position", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1, 2, 1}},
			{
				LHS:             4,
				RHS:             []int{1, 2, 1},
				Prec:            -2,
				HasExplicitPrec: true,
			},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0, lhsSym: 3},
		{kind: lrReduce, prodIdx: 1, lhsSym: 4},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved actions = %+v, want both same-RHS wrapper reductions kept", got)
	}
}

func TestResolveReduceReduceKeepsNestedScopedSameRHSExplicitNegativeAmbiguity(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "{", Kind: SymbolTerminal},
			{Name: "identifier", Kind: SymbolNonterminal},
			{Name: "::", Kind: SymbolTerminal},
			{Name: "scoped_identifier", Kind: SymbolNonterminal},
			{Name: "scoped_type_identifier_in_expression_position", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{3, 2, 1}},
			{
				LHS:             4,
				RHS:             []int{3, 2, 1},
				Prec:            -2,
				HasExplicitPrec: true,
			},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0, lhsSym: 3},
		{kind: lrReduce, prodIdx: 1, lhsSym: 4},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved actions = %+v, want nested same-RHS wrapper reductions kept", got)
	}
}

func TestResolveReduceReduceSameRHSExplicitNegativeRequiresScopedWrappers(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "{", Kind: SymbolTerminal},
			{Name: "identifier", Kind: SymbolNonterminal},
			{Name: "::", Kind: SymbolTerminal},
			{Name: "wrapper_a", Kind: SymbolNonterminal},
			{Name: "wrapper_b", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1, 2, 1}},
			{
				LHS:             4,
				RHS:             []int{1, 2, 1},
				Prec:            -2,
				HasExplicitPrec: true,
			},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0, lhsSym: 3},
		{kind: lrReduce, prodIdx: 1, lhsSym: 4},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("resolved actions = %+v, want normal reduce/reduce precedence resolution", got)
	}
}

func TestResolveReduceReduceSameRHSExplicitNegativeRequiresEqualRHS(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "{", Kind: SymbolTerminal},
			{Name: "identifier", Kind: SymbolNonterminal},
			{Name: "::", Kind: SymbolTerminal},
			{Name: "scoped_identifier", Kind: SymbolNonterminal},
			{Name: "scoped_type_identifier_in_expression_position", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{3, 2, 1}},
			{
				LHS:             4,
				RHS:             []int{1, 2, 1},
				Prec:            -2,
				HasExplicitPrec: true,
			},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0, lhsSym: 3},
		{kind: lrReduce, prodIdx: 1, lhsSym: 4},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("resolved actions = %+v, want normal reduce/reduce precedence resolution", got)
	}
}

func TestResolveReduceReduceSameRHSExplicitNegativeRequiresNegativeScopedTypePrec(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "{", Kind: SymbolTerminal},
			{Name: "identifier", Kind: SymbolNonterminal},
			{Name: "::", Kind: SymbolTerminal},
			{Name: "scoped_identifier", Kind: SymbolNonterminal},
			{Name: "scoped_type_identifier_in_expression_position", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{3, 2, 1}},
			{
				LHS:             4,
				RHS:             []int{3, 2, 1},
				HasExplicitPrec: true,
			},
		},
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrReduce, prodIdx: 0, lhsSym: 3},
		{kind: lrReduce, prodIdx: 1, lhsSym: 4},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("resolved actions = %+v, want normal reduce/reduce precedence resolution", got)
	}
}

func TestPropagateEntryShiftMetadataThroughRepeatHelper(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "(", Kind: SymbolTerminal},
			{Name: "_expression", Kind: SymbolNonterminal},
			{Name: "call_expression_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "argument_list", Kind: SymbolNonterminal},
			{Name: "call_expression", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 5, RHS: []int{2, 3}, Prec: 80, HasExplicitPrec: true},
			{LHS: 3, RHS: []int{4}},
			{LHS: 4, RHS: []int{1}},
		},
	}
	ctx := &lrContext{
		tokenCount:       2,
		firstSets:        make([]bitset, len(ng.Symbols)),
		nullables:        make([]bool, len(ng.Symbols)),
		prodsByLHS:       map[int][]int{3: {1}, 4: {2}},
		repeatWrapperLHS: make([]bool, len(ng.Symbols)),
	}
	ctx.firstSets[3] = newBitset(2)
	ctx.firstSets[3].add(1)

	tables := &LRTables{
		ActionTable: map[int]map[int][]lrAction{
			0: {
				1: {{kind: lrShift, state: 1, lhsSym: 4}},
			},
		},
	}
	itemSets := []lrItemSet{{
		cores: []coreEntry{{prodIdx: 0, dot: 1}},
	}}

	propagateEntryShiftMetadata(tables, itemSets, ctx, ng)
	got := tables.ActionTable[0][1]
	if len(got) != 1 || got[0].prec != 80 || got[0].lhsSym != 4 {
		t.Fatalf("shift action = %+v, want argument_list shift upgraded to prec 80", got)
	}
	foundCallLHS := false
	for _, lhs := range got[0].lhsSyms {
		if lhs == 5 {
			foundCallLHS = true
			break
		}
	}
	if !foundCallLHS {
		t.Fatalf("shift lhsSyms = %v, want call_expression contributor", got[0].lhsSyms)
	}
}

func TestPropagateEntryShiftMetadataThroughLeadingWrapper(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "op", Kind: SymbolTerminal},
			{Name: "wrapper", Kind: SymbolNonterminal},
			{Name: "operator", Kind: SymbolNonterminal},
			{Name: "outer_expression", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 4, RHS: []int{2}, Prec: 90, HasExplicitPrec: true},
			{LHS: 2, RHS: []int{3}},
			{LHS: 3, RHS: []int{1}},
		},
	}
	ctx := &lrContext{
		tokenCount: 2,
		firstSets:  make([]bitset, len(ng.Symbols)),
		nullables:  make([]bool, len(ng.Symbols)),
		prodsByLHS: map[int][]int{
			2: {1},
			3: {2},
		},
	}
	ctx.firstSets[2] = newBitset(2)
	ctx.firstSets[2].add(1)

	tables := &LRTables{
		ActionTable: map[int]map[int][]lrAction{
			0: {
				1: {{kind: lrShift, state: 1, lhsSym: 3}},
			},
		},
	}
	itemSets := []lrItemSet{{
		cores: []coreEntry{{prodIdx: 0, dot: 0}},
	}}

	propagateEntryShiftMetadata(tables, itemSets, ctx, ng)
	got := tables.ActionTable[0][1]
	if len(got) != 1 || got[0].prec != 90 || got[0].lhsSym != 3 {
		t.Fatalf("shift action = %+v, want operator shift upgraded to prec 90", got)
	}
	foundOuterLHS := false
	for _, lhs := range got[0].lhsSyms {
		if lhs == 4 {
			foundOuterLHS = true
			break
		}
	}
	if !foundOuterLHS {
		t.Fatalf("shift lhsSyms = %v, want outer_expression contributor", got[0].lhsSyms)
	}
}

func TestPropagateEntryShiftMetadataSkipsNonPassThroughLeadingWrapper(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "op", Kind: SymbolTerminal},
			{Name: "tail", Kind: SymbolTerminal},
			{Name: "wrapper", Kind: SymbolNonterminal},
			{Name: "operator", Kind: SymbolNonterminal},
			{Name: "outer_expression", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 5, RHS: []int{3}, Prec: 90, HasExplicitPrec: true},
			{LHS: 3, RHS: []int{4, 2}},
			{LHS: 4, RHS: []int{1}},
		},
	}
	ctx := &lrContext{
		tokenCount: 3,
		firstSets:  make([]bitset, len(ng.Symbols)),
		nullables:  make([]bool, len(ng.Symbols)),
		prodsByLHS: map[int][]int{
			3: {1},
			4: {2},
		},
	}
	ctx.firstSets[3] = newBitset(3)
	ctx.firstSets[3].add(1)

	tables := &LRTables{
		ActionTable: map[int]map[int][]lrAction{
			0: {
				1: {{kind: lrShift, state: 1, lhsSym: 4}},
			},
		},
	}
	itemSets := []lrItemSet{{
		cores: []coreEntry{{prodIdx: 0, dot: 0}},
	}}

	propagateEntryShiftMetadata(tables, itemSets, ctx, ng)
	got := tables.ActionTable[0][1]
	if len(got) != 1 {
		t.Fatalf("shift actions = %+v, want no propagated duplicate", got)
	}
	if got[0].prec != 0 || got[0].hasPrec || len(got[0].lhsSyms) != 0 {
		t.Fatalf("shift action = %+v, want no enclosing precedence through non-pass-through wrapper", got[0])
	}
}

func TestPropagateEntryShiftMetadataSkipsLeadingWrapperWithNonNullableSuffix(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "op", Kind: SymbolTerminal},
			{Name: "required_tail", Kind: SymbolTerminal},
			{Name: "wrapper", Kind: SymbolNonterminal},
			{Name: "operator", Kind: SymbolNonterminal},
			{Name: "outer_expression", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 5, RHS: []int{3}, Prec: 90, HasExplicitPrec: true},
			{LHS: 3, RHS: []int{4, 2}},
			{LHS: 4, RHS: []int{1}},
		},
	}
	ctx := &lrContext{
		tokenCount: 3,
		firstSets:  make([]bitset, len(ng.Symbols)),
		nullables:  make([]bool, len(ng.Symbols)),
		prodsByLHS: map[int][]int{
			3: {1},
			4: {2},
		},
	}
	ctx.firstSets[3] = newBitset(3)
	ctx.firstSets[3].add(1)

	tables := &LRTables{
		ActionTable: map[int]map[int][]lrAction{
			0: {
				1: {{kind: lrShift, state: 1, lhsSym: 4}},
			},
		},
	}
	itemSets := []lrItemSet{{
		cores: []coreEntry{{prodIdx: 0, dot: 0}},
	}}

	propagateEntryShiftMetadata(tables, itemSets, ctx, ng)
	got := tables.ActionTable[0][1]
	if len(got) != 1 {
		t.Fatalf("shift actions = %+v, want no propagated duplicate", got)
	}
	if got[0].prec != 0 || got[0].hasPrec || len(got[0].lhsSyms) != 0 {
		t.Fatalf("shift action = %+v, want no enclosing precedence through wrapper with non-nullable suffix", got[0])
	}
}

func TestPropagateEntryShiftMetadataThroughPostPrefixGeneratedRepeatContinuation(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "op", Kind: SymbolTerminal},
			{Name: "tail", Kind: SymbolTerminal},
			{Name: "operand", Kind: SymbolNonterminal},
			{Name: "operator", Kind: SymbolNonterminal},
			{Name: "continuation_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "outer_expression", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 6, RHS: []int{3, 5}, Prec: 90, HasExplicitPrec: true},
			{LHS: 5, RHS: []int{4, 2}},
			{LHS: 4, RHS: []int{1}},
		},
	}
	ctx := &lrContext{
		tokenCount: 3,
		firstSets:  make([]bitset, len(ng.Symbols)),
		nullables:  make([]bool, len(ng.Symbols)),
		prodsByLHS: map[int][]int{
			5: {1},
			4: {2},
		},
	}
	ctx.firstSets[5] = newBitset(3)
	ctx.firstSets[5].add(1)

	tables := &LRTables{
		ActionTable: map[int]map[int][]lrAction{
			0: {
				1: {{kind: lrShift, state: 1, lhsSym: 4}},
			},
		},
	}
	itemSets := []lrItemSet{{
		cores: []coreEntry{{prodIdx: 0, dot: 1}},
	}}

	propagateEntryShiftMetadata(tables, itemSets, ctx, ng)
	got := tables.ActionTable[0][1]
	if len(got) != 1 || got[0].prec != 90 || got[0].lhsSym != 4 {
		t.Fatalf("shift action = %+v, want post-prefix continuation shift upgraded to prec 90", got)
	}
	foundOuterLHS := false
	for _, lhs := range got[0].lhsSyms {
		if lhs == 6 {
			foundOuterLHS = true
			break
		}
	}
	if !foundOuterLHS {
		t.Fatalf("shift lhsSyms = %v, want outer_expression contributor", got[0].lhsSyms)
	}
}

func TestPropagateEntryShiftMetadataThroughPostPrefixSuffixChoiceWrapper(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "op", Kind: SymbolTerminal},
			{Name: "tail", Kind: SymbolTerminal},
			{Name: "operand", Kind: SymbolNonterminal},
			{Name: "operator", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "suffix_cast", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "suffix_call", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "suffix_choice", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "outer_expression", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 8, RHS: []int{3, 7}, Prec: 90, HasExplicitPrec: true},
			{LHS: 7, RHS: []int{5}},
			{LHS: 7, RHS: []int{6}},
			{LHS: 5, RHS: []int{4, 2}},
			{LHS: 4, RHS: []int{1}},
			{LHS: 6, RHS: []int{4}},
		},
	}
	ctx := &lrContext{
		tokenCount: 3,
		firstSets:  make([]bitset, len(ng.Symbols)),
		nullables:  make([]bool, len(ng.Symbols)),
		prodsByLHS: map[int][]int{
			7: {1, 2},
			5: {3},
			4: {4},
			6: {5},
		},
	}
	ctx.firstSets[7] = newBitset(3)
	ctx.firstSets[7].add(1)

	tables := &LRTables{
		ActionTable: map[int]map[int][]lrAction{
			0: {
				1: {{kind: lrShift, state: 1, lhsSym: 4, lhsSyms: []int{5}}},
			},
		},
	}
	itemSets := []lrItemSet{{
		cores: []coreEntry{{prodIdx: 0, dot: 1}},
	}}

	propagateEntryShiftMetadata(tables, itemSets, ctx, ng)
	got := tables.ActionTable[0][1]
	if len(got) != 1 || got[0].prec != 90 || got[0].lhsSym != 4 {
		t.Fatalf("shift action = %+v, want suffix alternative shift upgraded to prec 90", got)
	}
	foundOuterLHS := false
	for _, lhs := range got[0].lhsSyms {
		if lhs == 8 {
			foundOuterLHS = true
			break
		}
	}
	if !foundOuterLHS {
		t.Fatalf("shift lhsSyms = %v, want outer_expression contributor", got[0].lhsSyms)
	}
}

func TestPropagateEntryShiftMetadataThroughPostPrefixVisibleSuffixWrapper(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "op", Kind: SymbolTerminal},
			{Name: "tail", Kind: SymbolTerminal},
			{Name: "operand", Kind: SymbolNonterminal},
			{Name: "operator", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "suffix", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "outer_expression", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 6, RHS: []int{3, 5}, Prec: 90, HasExplicitPrec: true},
			{LHS: 5, RHS: []int{4, 2}},
			{LHS: 4, RHS: []int{1}},
		},
	}
	ctx := &lrContext{
		tokenCount: 3,
		firstSets:  make([]bitset, len(ng.Symbols)),
		nullables:  make([]bool, len(ng.Symbols)),
		prodsByLHS: map[int][]int{
			5: {1},
			4: {2},
		},
	}
	ctx.firstSets[5] = newBitset(3)
	ctx.firstSets[5].add(1)

	tables := &LRTables{
		ActionTable: map[int]map[int][]lrAction{
			0: {
				1: {{kind: lrShift, state: 1, lhsSym: 4}},
			},
		},
	}
	itemSets := []lrItemSet{{
		cores: []coreEntry{{prodIdx: 0, dot: 1}},
	}}

	propagateEntryShiftMetadata(tables, itemSets, ctx, ng)
	got := tables.ActionTable[0][1]
	if len(got) != 1 || got[0].prec != 90 || got[0].lhsSym != 4 {
		t.Fatalf("shift action = %+v, want visible suffix shift upgraded to prec 90", got)
	}
	foundOuterLHS := false
	for _, lhs := range got[0].lhsSyms {
		if lhs == 6 {
			foundOuterLHS = true
			break
		}
	}
	if !foundOuterLHS {
		t.Fatalf("shift lhsSyms = %v, want outer_expression contributor", got[0].lhsSyms)
	}
}

func TestPropagateEntryShiftMetadataSkipsPostPrefixNonRepeatContinuation(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "op", Kind: SymbolTerminal},
			{Name: "tail", Kind: SymbolTerminal},
			{Name: "operand", Kind: SymbolNonterminal},
			{Name: "operator", Kind: SymbolNonterminal},
			{Name: "continuation", Kind: SymbolNonterminal},
			{Name: "outer_expression", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 6, RHS: []int{3, 5}, Prec: 90, HasExplicitPrec: true},
			{LHS: 5, RHS: []int{4, 2}},
			{LHS: 4, RHS: []int{1}},
		},
	}
	ctx := &lrContext{
		tokenCount: 3,
		firstSets:  make([]bitset, len(ng.Symbols)),
		nullables:  make([]bool, len(ng.Symbols)),
		prodsByLHS: map[int][]int{
			5: {1},
			4: {2},
		},
	}
	ctx.firstSets[5] = newBitset(3)
	ctx.firstSets[5].add(1)

	tables := &LRTables{
		ActionTable: map[int]map[int][]lrAction{
			0: {
				1: {{kind: lrShift, state: 1, lhsSym: 4}},
			},
		},
	}
	itemSets := []lrItemSet{{
		cores: []coreEntry{{prodIdx: 0, dot: 1}},
	}}

	propagateEntryShiftMetadata(tables, itemSets, ctx, ng)
	got := tables.ActionTable[0][1]
	if len(got) != 1 {
		t.Fatalf("shift actions = %+v, want no propagated duplicate", got)
	}
	if got[0].prec != 0 || got[0].hasPrec || len(got[0].lhsSyms) != 0 {
		t.Fatalf("shift action = %+v, want no enclosing precedence through non-repeat continuation", got[0])
	}
}

func TestPropagateEntryShiftMetadataSkipsUnrelatedLeadingWrapperShift(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "op", Kind: SymbolTerminal},
			{Name: "wrapper", Kind: SymbolNonterminal},
			{Name: "operator", Kind: SymbolNonterminal},
			{Name: "outer_expression", Kind: SymbolNonterminal},
			{Name: "unrelated_operator", Kind: SymbolNonterminal},
			{Name: "lower_expression", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 4, RHS: []int{2}, Prec: 90, HasExplicitPrec: true},
			{LHS: 2, RHS: []int{3}},
			{LHS: 3, RHS: []int{1}},
			{LHS: 5, RHS: []int{1}},
			{LHS: 6, RHS: []int{0}, Prec: 10, HasExplicitPrec: true},
		},
	}
	ctx := &lrContext{
		tokenCount: 2,
		firstSets:  make([]bitset, len(ng.Symbols)),
		nullables:  make([]bool, len(ng.Symbols)),
		prodsByLHS: map[int][]int{
			2: {1},
			3: {2},
			5: {3},
		},
	}
	ctx.firstSets[2] = newBitset(2)
	ctx.firstSets[2].add(1)

	tables := &LRTables{
		ActionTable: map[int]map[int][]lrAction{
			0: {
				1: {
					{kind: lrShift, state: 1, lhsSym: 3},
					{kind: lrShift, state: 2, lhsSym: 5},
				},
			},
		},
	}
	itemSets := []lrItemSet{{
		cores: []coreEntry{{prodIdx: 0, dot: 0}},
	}}

	propagateEntryShiftMetadata(tables, itemSets, ctx, ng)
	got := tables.ActionTable[0][1]
	var matching, unrelated *lrAction
	for i := range got {
		switch got[i].state {
		case 1:
			matching = &got[i]
		case 2:
			unrelated = &got[i]
		}
	}
	if matching == nil {
		t.Fatalf("actions = %+v, missing leading-wrapper shift", got)
	}
	if unrelated == nil {
		t.Fatalf("actions = %+v, missing unrelated shift", got)
	}
	if matching.prec != 90 || matching.lhsSym != 3 {
		t.Fatalf("matching shift = %+v, want operator shift upgraded to prec 90", *matching)
	}
	foundOuterLHS := false
	for _, lhs := range matching.lhsSyms {
		if lhs == 4 {
			foundOuterLHS = true
			break
		}
	}
	if !foundOuterLHS {
		t.Fatalf("matching shift lhsSyms = %v, want outer_expression contributor", matching.lhsSyms)
	}
	if unrelated.prec != 0 || unrelated.hasPrec || len(unrelated.lhsSyms) != 0 {
		t.Fatalf("unrelated shift = %+v, want no parent metadata", *unrelated)
	}

	resolved, err := resolveActionConflict(1, []lrAction{
		*unrelated,
		{kind: lrReduce, prodIdx: 4, lhsSym: 6, prec: 10, hasPrec: true},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict unrelated shift: %v", err)
	}
	if len(resolved) != 1 || resolved[0].kind != lrReduce || resolved[0].prodIdx != 4 {
		t.Fatalf("unrelated conflict actions = %+v, want lower_expression reduce", resolved)
	}
}

func TestResolveAuxToParentsUsesCachedReverseParents(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "expression", Kind: SymbolNonterminal},
			{Name: "value_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "value_token1", Kind: SymbolNamedToken},
		},
		Productions: []Production{
			{LHS: 1, RHS: []int{2}},
			{LHS: 0, RHS: []int{1}},
		},
		Conflicts: [][]int{{0}},
	}

	cache := getConflictResolutionCache(ng)
	got := resolveAuxToParents(2, ng, cache)
	if len(got) != 1 || got[0] != 0 {
		t.Fatalf("resolveAuxToParents(value_token1) = %v, want [0]", got)
	}

	again := resolveAuxToParents(2, ng, cache)
	if len(again) != 1 || again[0] != 0 {
		t.Fatalf("cached resolveAuxToParents(value_token1) = %v, want [0]", again)
	}
}

// TestResolveShiftReduceKeepsLabeledStatementPropertyNameDeclaredConflict
// pins the T1 election-table-parity witness (spec.a2-election-table-parity,
// tranche T1): the real tree-sitter-javascript grammar.js declares
//
//	conflicts: $ => [ ..., [$.labeled_statement, $._property_name], ... ]
//
// because at statement position, `identifier ':'` right after `{` is
// ambiguous between a labeled_statement (`label ':' body`) and an object
// literal's pair (`_property_name ':' value`). On lookahead ':', the state
// offers a SHIFT that continues labeled_statement's `label • ':' body` and a
// REDUCE that completes `_property_name -> identifier`. Upstream's generator
// keeps both actions (a GLR table entry) whenever precedence does not
// resolve a declared-conflict pair; it never silently drops one side. This
// test asserts resolveActionConflict does the same for the exact shape that
// reaches it once shift/reduce metadata is separated (grammargen/lr.go
// shiftReduceInConflictGroup, ~lr.go:3676-3709): shift LHS and reduce LHS
// are different members of the same declared conflict group, so both
// actions must survive so the runtime GLR engine (labeled_statement carries
// prec.dynamic(-1); _property_name carries none) can elect the object
// literal at merge time.
func TestResolveShiftReduceKeepsLabeledStatementPropertyNameDeclaredConflict(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: ":", Kind: SymbolTerminal},
			{Name: "identifier", Kind: SymbolTerminal},
			{Name: "labeled_statement", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "_property_name", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1}}, // _property_name -> identifier
		},
		Conflicts: [][]int{{2, 3}}, // declared: [labeled_statement, _property_name]
	}

	got, err := resolveActionConflict(0, []lrAction{
		{kind: lrShift, state: 10, lhsSym: 2}, // labeled_statement: label . ':' body
		{kind: lrReduce, prodIdx: 0},          // _property_name -> identifier .
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved actions = %+v, want both the labeled_statement shift and the _property_name reduce kept (declared conflict, GLR fork)", got)
	}
}
