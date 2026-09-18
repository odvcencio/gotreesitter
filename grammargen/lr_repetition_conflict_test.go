package grammargen

import (
	"context"
	"testing"
)

func TestRepetitionShiftHelperReturnsPluralProducerProvenance(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "item", Kind: SymbolNamedToken},
			{Name: "list_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "other_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
			{LHS: 2, RHS: []int{2, 1}},
			{LHS: 3, RHS: []int{1}},
			{LHS: 3, RHS: []int{3, 1}},
		},
	}
	ctx := &lrContext{
		ng:         ng,
		tokenCount: 2,
		itemSets: []lrItemSet{
			{cores: []coreEntry{
				{prodIdx: 1, dot: 2},
				{prodIdx: 3, dot: 2},
			}},
			{cores: []coreEntry{
				{prodIdx: 0, dot: 1},
				{prodIdx: 2, dot: 1},
			}},
		},
	}

	lhsSyms := ctx.repetitionShiftHelperLHSSyms(0, 1, 1)
	if len(lhsSyms) != 2 || lhsSyms[0] != 2 || lhsSyms[1] != 3 {
		t.Fatalf("repeat helper provenance=%v, want [2 3]", lhsSyms)
	}

	action := lrAction{kind: lrShift, state: 1, lhsSym: 1}
	for _, lhs := range lhsSyms {
		action.addRepeatLHS(lhs)
	}
	if !action.repeat || !action.hasRepeatLHS(2) || !action.hasRepeatLHS(3) {
		t.Fatalf("producer-populated action=%+v, want repeat provenance for both helpers", action)
	}
}

func TestRepetitionShiftHelperNoSyntheticStateProvenance(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "item", Kind: SymbolNamedToken},
			{Name: "list_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
			{LHS: 2, RHS: []int{2, 1}},
		},
	}
	ctx := &lrContext{
		ng:         ng,
		tokenCount: 2,
		itemSets: []lrItemSet{
			{cores: []coreEntry{{prodIdx: 1, dot: 2}}},
			{cores: []coreEntry{{prodIdx: 0, dot: 1}}},
		},
	}

	if got := ctx.repetitionShiftHelperLHSSyms(2, 1, 1); len(got) != 0 {
		t.Fatalf("synthetic source provenance=%v, want none", got)
	}
	if got := ctx.repetitionShiftHelperLHSSyms(0, 1, 2); len(got) != 0 {
		t.Fatalf("synthetic target provenance=%v, want none", got)
	}
}

func TestAddActionMergesRepeatShiftHelperProvenance(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "op", Kind: SymbolNonterminal},
			{Name: "item", Kind: SymbolNamedToken},
			{Name: "list_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "other_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1, 2, 1, 2}},
			{LHS: 3, RHS: []int{3, 1, 2}},
			{LHS: 4, RHS: []int{1, 2, 1, 2}},
			{LHS: 4, RHS: []int{4, 1, 2}},
		},
	}
	tables := &LRTables{
		ActionTable: map[int]map[int][]lrAction{0: {}},
		GotoTable:   map[int]map[int]int{0: {}},
	}
	tables.addAction(0, 1, lrAction{
		kind:      lrShift,
		state:     17,
		lhsSym:    1,
		repeat:    true,
		repeatLHS: 3,
	})
	tables.addAction(0, 1, lrAction{
		kind:      lrShift,
		state:     17,
		lhsSym:    1,
		repeat:    true,
		repeatLHS: 4,
	})

	acts := tables.ActionTable[0][1]
	if len(acts) != 1 {
		t.Fatalf("merged action count=%d, want 1: %+v", len(acts), acts)
	}
	shift := acts[0]
	if !shift.hasRepeatLHS(3) || !shift.hasRepeatLHS(4) {
		t.Fatalf("merged shift repeat provenance=%+v, want helper LHS 3 and 4", shift)
	}

	for _, tc := range []struct {
		name    string
		prodIdx int
		lhs     int
	}{
		{name: "first helper", prodIdx: 0, lhs: 3},
		{name: "second helper", prodIdx: 2, lhs: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveActionConflict(1, []lrAction{
				{kind: lrReduce, prodIdx: tc.prodIdx},
				shift,
			}, ng)
			if err != nil {
				t.Fatalf("resolveActionConflict: %v", err)
			}
			if len(got) != 2 {
				t.Fatalf("resolved len=%d, want 2: %+v", len(got), got)
			}
			if got[0].kind != lrReduce || got[0].prodIdx != tc.prodIdx {
				t.Fatalf("resolved[0]=%+v, want reduce prod %d", got[0], tc.prodIdx)
			}
			if got[1].kind != lrShift || !got[1].hasRepeatLHS(tc.lhs) {
				t.Fatalf("resolved[1]=%+v, want shift with repeat helper LHS %d", got[1], tc.lhs)
			}
		})
	}
}

func TestResolveActionConflictKeepsRecursiveRepeatShiftReduce(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "item", Kind: SymbolNamedToken},
			{Name: "list_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
			{LHS: 2, RHS: []int{2, 1}},
		},
	}

	actions := []lrAction{
		{kind: lrReduce, prodIdx: 1, lhsSym: 2},
		{kind: lrShift, state: 17, lhsSym: 1, repeat: true, repeatLHS: 2},
	}

	got, err := resolveActionConflict(1, actions, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved len=%d, want 2", len(got))
	}
	if got[0].kind != lrReduce {
		t.Fatalf("resolved[0].kind=%v, want reduce", got[0].kind)
	}
	if got[1].kind != lrShift || got[1].state != 17 || !got[1].repeat {
		t.Fatalf("resolved shift=%+v, want repetition-marked shift", got[1])
	}
	if got[1].repeatLHS != 2 {
		t.Fatalf("resolved shift repeatLHS=%d, want helper LHS 2", got[1].repeatLHS)
	}
}

func TestResolveActionConflictKeepsRecursiveRepeatStructuralContinuationWithUnrelatedRepeatProvenance(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "comma", Kind: SymbolTerminal},
			{Name: "item", Kind: SymbolNamedToken},
			{Name: "list_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "maybe_item", Kind: SymbolNonterminal},
			{Name: "other_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{4, 1}},
			{LHS: 3, RHS: []int{3, 4, 1}},
			{LHS: 4, RHS: nil},
			{LHS: 4, RHS: []int{2}},
			{LHS: 5, RHS: []int{2}},
			{LHS: 5, RHS: []int{5, 2}},
		},
	}

	for _, tc := range []struct {
		name  string
		shift lrAction
	}{
		{
			name:  "unmarked shift",
			shift: lrAction{kind: lrShift, state: 17, lhsSym: 2},
		},
		{
			name:  "unrelated repeat provenance",
			shift: lrAction{kind: lrShift, state: 17, lhsSym: 2, repeat: true, repeatLHS: 5},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveActionConflict(1, []lrAction{
				{kind: lrReduce, prodIdx: 1, lhsSym: 3},
				tc.shift,
			}, ng)
			if err != nil {
				t.Fatalf("resolveActionConflict: %v", err)
			}
			if len(got) != 2 {
				t.Fatalf("resolved len=%d, want reduce+shift: %+v", len(got), got)
			}
			if got[0].kind != lrReduce || got[0].prodIdx != 1 {
				t.Fatalf("resolved[0]=%+v, want recursive repeat reduce", got[0])
			}
			if got[1].kind != lrShift || got[1].state != 17 || !got[1].hasRepeatLHS(3) {
				t.Fatalf("resolved[1]=%+v, want shift containing matching repeat helper LHS 3", got[1])
			}
		})
	}
}

func TestResolveActionConflictKeepsLoweredRepeatHelperContinuation(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "op", Kind: SymbolNonterminal},
			{Name: "item", Kind: SymbolNamedToken},
			{Name: "list_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1, 2, 1, 2}},
			{LHS: 3, RHS: []int{3, 1, 2}},
		},
	}

	actions := []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrShift, state: 17, lhsSym: 1, prec: 6, assoc: AssocLeft, repeat: true, repeatLHS: 3},
	}

	got, err := resolveActionConflict(1, actions, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved len=%d, want 2", len(got))
	}
	if got[0].kind != lrReduce || got[0].prodIdx != 0 {
		t.Fatalf("resolved[0]=%+v, want lowered repeat helper reduce", got[0])
	}
	if got[1].kind != lrShift || got[1].state != 17 || !got[1].repeat {
		t.Fatalf("resolved[1]=%+v, want repeat-marked shift", got[1])
	}
}

func TestResolveActionConflictLoweredRepeatSameOperatorContinuesParentRepeat(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "and_op", Kind: SymbolTerminal},
			{Name: "atom", Kind: SymbolNamedToken},
			{Name: "and_tail_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "and_expr", Kind: SymbolNonterminal, Visible: true, Named: true},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1, 2}},
			{LHS: 3, RHS: []int{3, 1, 2}},
			{LHS: 4, RHS: []int{2, 3}, Prec: 6, HasExplicitPrec: true, Assoc: AssocLeft},
			{LHS: 4, RHS: []int{2, 1, 2}, Prec: 6, HasExplicitPrec: true, Assoc: AssocLeft},
		},
	}

	got, err := resolveActionConflict(1, []lrAction{
		{kind: lrReduce, prodIdx: 3, prec: 6, hasPrec: true, assoc: AssocLeft, lhsSym: 4},
		{kind: lrShift, state: 17, lhsSym: 1},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift || got[0].state != 17 {
		t.Fatalf("resolved=%+v, want same-operator lowered-repeat continuation shift", got)
	}
}

func TestResolveActionConflictLoweredRepeatHelperUsesVisibleParentLeftAssoc(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "and_op", Kind: SymbolTerminal},
			{Name: "rel_op", Kind: SymbolTerminal},
			{Name: "atom", Kind: SymbolNamedToken},
			{Name: "rel_expr", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "and_tail_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "and_expr", Kind: SymbolNonterminal, Visible: true, Named: true},
		},
		Productions: []Production{
			{LHS: 4, RHS: []int{3, 2, 3}, Prec: 8, HasExplicitPrec: true},
			{LHS: 5, RHS: []int{1, 3}},
			{LHS: 5, RHS: []int{5, 1, 3}},
			{LHS: 6, RHS: []int{3, 5}, Prec: 6, HasExplicitPrec: true, Assoc: AssocLeft},
		},
	}

	got, err := resolveActionConflict(1, []lrAction{
		{kind: lrReduce, prodIdx: 1, lhsSym: 5},
		{kind: lrShift, state: 17, lhsSym: 1, prec: 6, hasPrec: true, assoc: AssocLeft, repeat: true, repeatLHS: 5},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 1 {
		t.Fatalf("resolved=%+v, want repeat helper reduce by visible parent left associativity", got)
	}

	got, err = resolveActionConflict(2, []lrAction{
		{kind: lrReduce, prodIdx: 1, lhsSym: 5},
		{kind: lrShift, state: 23, lhsSym: 4, prec: 8, hasPrec: true, repeat: true, repeatLHS: 5},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift || got[0].state != 23 {
		t.Fatalf("resolved=%+v, want higher-precedence suffix shift over repeat helper reduce", got)
	}
}

func TestResolveActionConflictVisibleLoweredRepeatBodyAssociativity(t *testing.T) {
	for _, tc := range []struct {
		name        string
		assoc       Assoc
		reducePrec  int
		shiftPrec   int
		wantActions int
	}{
		{"right at zero precedence", AssocRight, 0, 0, 1},
		{"right at positive precedence", AssocRight, 2, 2, 1},
		{"no associativity preserves conflict", AssocNone, 0, 0, 2},
		{"left associativity preserves conflict", AssocLeft, 0, 0, 2},
		{"higher shift precedence preserves conflict", AssocRight, 0, 1, 2},
		{"lower shift precedence preserves conflict", AssocRight, 0, -1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ng := &NormalizedGrammar{
				Symbols: []SymbolInfo{
					{Name: "end", Kind: SymbolTerminal},
					{Name: ".", Kind: SymbolTerminal},
					{Name: ".*", Kind: SymbolTerminal},
					{Name: "identifier", Kind: SymbolNamedToken},
					{Name: "get_attr", Kind: SymbolNonterminal, Visible: true, Named: true},
					{Name: "attr_splat_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
					{Name: "attr_splat", Kind: SymbolNonterminal, Visible: true, Named: true},
				},
				Productions: []Production{
					{LHS: 4, RHS: []int{1, 3}},
					{LHS: 6, RHS: []int{2, 4}, Prec: tc.reducePrec, HasExplicitPrec: true, Assoc: tc.assoc},
					{LHS: 6, RHS: []int{2, 5}, Prec: tc.reducePrec, HasExplicitPrec: true, Assoc: tc.assoc},
					{LHS: 5, RHS: []int{5, 4}},
					{LHS: 5, RHS: []int{4, 4}},
				},
			}
			got, err := resolveActionConflict(1, []lrAction{
				{kind: lrReduce, prodIdx: 1, lhsSym: 6},
				{kind: lrShift, state: 17, lhsSym: 4, prec: tc.shiftPrec, hasPrec: true},
			}, ng)
			if err != nil {
				t.Fatalf("resolveActionConflict: %v", err)
			}
			if len(got) != tc.wantActions {
				t.Fatalf("resolved=%+v, want %d actions", got, tc.wantActions)
			}
			if tc.wantActions == 1 && (got[0].kind != lrShift || got[0].state != 17) {
				t.Fatalf("resolved=%+v, want continuation shift", got)
			}
		})
	}
}

func TestResolveActionConflictLoweredRepeatHelperUsesSameFamilyShiftWithoutRepeatFlag(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "and_op", Kind: SymbolTerminal},
			{Name: "atom", Kind: SymbolNamedToken},
			{Name: "and_tail_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "and_expr", Kind: SymbolNonterminal, Visible: true, Named: true},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1, 2}},
			{LHS: 3, RHS: []int{3, 1, 2}},
			{LHS: 4, RHS: []int{2, 3}, Prec: 6, HasExplicitPrec: true, Assoc: AssocLeft},
		},
	}

	got, err := resolveActionConflict(1, []lrAction{
		{kind: lrReduce, prodIdx: 1, lhsSym: 3},
		{kind: lrShift, state: 17, lhsSym: 1, lhsSyms: []int{4, 3}, prec: 6, assoc: AssocLeft},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 1 {
		t.Fatalf("resolved=%+v, want repeat helper reduce by visible parent left associativity", got)
	}
}

func TestResolveActionConflictKeepsVisibleBodyElementBeforeLoweredRepeatContinuation(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "open", Kind: SymbolTerminal},
			{Name: "section_start", Kind: SymbolNonterminal},
			{Name: "body", Kind: SymbolNonterminal},
			{Name: "section", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "body_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1}},
			{LHS: 4, RHS: []int{2, 3}},
			{LHS: 4, RHS: []int{2, 5}},
			{LHS: 5, RHS: []int{3}},
			{LHS: 5, RHS: []int{5, 3}},
		},
	}

	got, err := resolveActionConflict(1, []lrAction{
		{kind: lrReduce, prodIdx: 1},
		{kind: lrShift, state: 17, lhsSym: 3, lhsSyms: []int{5}},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved actions=%+v, want visible reduce and continuation shift kept", got)
	}
}

func TestResolveActionConflictKeepsVisibleGeneratedRepeatBodyContinuation(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "open", Kind: SymbolTerminal},
			{Name: "section_start", Kind: SymbolNonterminal},
			{Name: "body", Kind: SymbolNonterminal},
			{Name: "section", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "body_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1}},
			{LHS: 4, RHS: []int{2, 3}},
			{LHS: 4, RHS: []int{2, 5}},
			{LHS: 5, RHS: []int{3}},
			{LHS: 5, RHS: []int{5, 3}},
		},
	}

	got, err := resolveActionConflict(1, []lrAction{
		{kind: lrReduce, prodIdx: 2},
		{kind: lrShift, state: 17, lhsSym: 3, lhsSyms: []int{5}},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved actions=%+v, want visible repeat-body reduce and continuation shift kept", got)
	}
}

func TestResolveActionConflictDoesNotKeepVisibleBodyContinuationWithoutGeneratedRepeatShape(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "open", Kind: SymbolTerminal},
			{Name: "section_start", Kind: SymbolNonterminal},
			{Name: "body", Kind: SymbolNonterminal},
			{Name: "section", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "other", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1}},
			{LHS: 4, RHS: []int{2, 3}},
			{LHS: 5, RHS: []int{1}, Prec: 8, HasExplicitPrec: true},
		},
	}

	got, err := resolveActionConflict(1, []lrAction{
		{kind: lrReduce, prodIdx: 1},
		{kind: lrShift, state: 17, lhsSym: 5, prec: 8, hasPrec: true},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift || got[0].state != 17 {
		t.Fatalf("resolved actions=%+v, want normal unrelated shift precedence resolution", got)
	}
}

func TestResolveActionConflictDoesNotKeepUnrelatedRepeatMarkedShift(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "op", Kind: SymbolNonterminal},
			{Name: "item", Kind: SymbolNamedToken},
			{Name: "list_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "unrelated", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1, 2, 1, 2}, Prec: 1},
			{LHS: 3, RHS: []int{3, 1, 2}},
		},
	}

	actions := []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrShift, state: 17, lhsSym: 1, prec: 2, assoc: AssocLeft, repeat: true, repeatLHS: 4},
	}

	got, err := resolveActionConflict(1, actions, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("resolved len=%d, want normal precedence resolution to one action: %+v", len(got), got)
	}
	if got[0].kind != lrShift || got[0].state != 17 {
		t.Fatalf("resolved=%+v, want unrelated shift to win by precedence", got)
	}
}

func TestResolveActionConflictUnrelatedRepeatMarkedShiftPrefersRepeatHelperReduce(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "op", Kind: SymbolNonterminal},
			{Name: "item", Kind: SymbolNamedToken},
			{Name: "list_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "other_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1, 2, 1, 2}},
			{LHS: 3, RHS: []int{3, 1, 2}},
		},
	}

	actions := []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrShift, state: 17, lhsSym: 1, repeat: true, repeatLHS: 4},
	}

	got, err := resolveActionConflict(1, actions, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 0 {
		t.Fatalf("resolved=%+v, want unrelated repeat-marked shift not to suppress helper reduce preference", got)
	}
}

func TestResolveActionConflictKeepsRepeatHelperReduceWithUnrelatedHigherPrecedenceShift(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "separator", Kind: SymbolTerminal},
			{Name: "item", Kind: SymbolNamedToken},
			{Name: "list_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "unrelated_label", Kind: SymbolNonterminal},
			{Name: "list", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{2}},
			{LHS: 3, RHS: []int{3, 2}},
			{LHS: 4, RHS: []int{1, 2}, Prec: 8, HasExplicitPrec: true},
			{LHS: 5, RHS: []int{3}},
		},
	}

	got, err := resolveActionConflict(1, []lrAction{
		{kind: lrShift, state: 17, lhsSym: 4, prec: 8, hasPrec: true},
		{kind: lrReduce, prodIdx: 0},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved len=%d, want GLR shift+repeat reduce: %+v", len(got), got)
	}
	hasShift := false
	hasReduce := false
	for _, action := range got {
		if action.kind == lrShift && action.state == 17 {
			hasShift = true
		}
		if action.kind == lrReduce && action.prodIdx == 0 {
			hasReduce = true
		}
	}
	if !hasShift || !hasReduce {
		t.Fatalf("resolved actions=%+v, want unrelated shift and repeat helper reduce", got)
	}
}

func TestResolveActionConflictKeepsAdjacentRepeatElementReduceWithSameFamilyShift(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "label", Kind: SymbolTerminal},
			{Name: "body", Kind: SymbolNamedToken},
			{Name: "group", Kind: SymbolNonterminal},
			{Name: "group_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "block", Kind: SymbolNonterminal},
			{Name: "unrelated_modifier", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1, 2}, HasExplicitPrec: true, Assoc: AssocLeft},
			{LHS: 4, RHS: []int{3}},
			{LHS: 4, RHS: []int{4, 3}},
			{LHS: 5, RHS: []int{4}},
			{LHS: 6, RHS: []int{1}, Prec: 8, HasExplicitPrec: true},
		},
	}

	got, err := resolveActionConflict(1, []lrAction{
		{kind: lrShift, state: 17, lhsSym: 1, lhsSyms: []int{3}, prec: 8, hasPrec: true},
		{kind: lrReduce, prodIdx: 0},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved len=%d, want GLR shift+adjacent repeat element reduce: %+v", len(got), got)
	}
	hasShift := false
	hasReduce := false
	for _, action := range got {
		if action.kind == lrShift && action.state == 17 {
			hasShift = true
		}
		if action.kind == lrReduce && action.prodIdx == 0 {
			hasReduce = true
		}
	}
	if !hasShift || !hasReduce {
		t.Fatalf("resolved actions=%+v, want same-family shift and adjacent repeat element reduce", got)
	}
}

func TestResolveActionConflictVetoesAdjacentRepeatElementReduceForHigherPrecedenceContinuation(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: ":", Kind: SymbolTerminal},
			{Name: "metavariable", Kind: SymbolNamedToken},
			{Name: "fragment_specifier", Kind: SymbolNamedToken},
			{Name: "_token_pattern", Kind: SymbolNonterminal},
			{Name: "token_binding_pattern", Kind: SymbolNonterminal},
			{Name: "_token_pattern_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "macro_rule", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 4, RHS: []int{2}},
			{LHS: 4, RHS: []int{5}},
			{LHS: 4, RHS: []int{1, 3}},
			{LHS: 5, RHS: []int{2, 1, 3}, Prec: 8, HasExplicitPrec: true},
			{LHS: 6, RHS: []int{4}},
			{LHS: 6, RHS: []int{6, 4}},
			{LHS: 7, RHS: []int{6}},
		},
	}

	got, err := resolveActionConflict(1, []lrAction{
		{kind: lrShift, state: 17, lhsSym: 5, prec: 8},
		{kind: lrReduce, prodIdx: 0},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift || got[0].state != 17 {
		t.Fatalf("resolved actions=%+v, want normal higher-precedence continuation shift", got)
	}
}

func TestResolveActionConflictKeepsAdjacentRepeatElementReduceWithoutHigherExplicitContinuationPrecedence(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "default", Kind: SymbolTerminal},
			{Name: "label_body", Kind: SymbolNamedToken},
			{Name: "switch_label", Kind: SymbolNonterminal},
			{Name: "switch_default_label", Kind: SymbolNonterminal},
			{Name: "switch_label_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "switch_block", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1}},
			{LHS: 3, RHS: []int{4}},
			{LHS: 4, RHS: []int{1, 2}},
			{LHS: 5, RHS: []int{3}},
			{LHS: 5, RHS: []int{5, 3}},
			{LHS: 6, RHS: []int{5}},
		},
	}

	got, err := resolveActionConflict(1, []lrAction{
		{kind: lrShift, state: 17, lhsSym: 4, prec: 8},
		{kind: lrReduce, prodIdx: 0},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resolved len=%d, want GLR shift+adjacent repeat element reduce: %+v", len(got), got)
	}
	hasShift := false
	hasReduce := false
	for _, action := range got {
		if action.kind == lrShift && action.state == 17 {
			hasShift = true
		}
		if action.kind == lrReduce && action.prodIdx == 0 {
			hasReduce = true
		}
	}
	if !hasShift || !hasReduce {
		t.Fatalf("resolved actions=%+v, want Java-like same-family shift and adjacent repeat element reduce", got)
	}
}

func TestResolveActionConflictPrefersAdjacentRepeatElementReduceOverUnrelatedShift(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "label", Kind: SymbolTerminal},
			{Name: "body", Kind: SymbolNamedToken},
			{Name: "group", Kind: SymbolNonterminal},
			{Name: "group_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "block", Kind: SymbolNonterminal},
			{Name: "unrelated_modifier", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{1, 2}, HasExplicitPrec: true, Assoc: AssocLeft},
			{LHS: 4, RHS: []int{3}},
			{LHS: 4, RHS: []int{4, 3}},
			{LHS: 5, RHS: []int{4}},
			{LHS: 6, RHS: []int{1}, Prec: 8, HasExplicitPrec: true},
		},
	}

	got, err := resolveActionConflict(1, []lrAction{
		{kind: lrShift, state: 17, lhsSym: 6, prec: 8, hasPrec: true},
		{kind: lrReduce, prodIdx: 0},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 0 {
		t.Fatalf("resolved actions=%+v, want adjacent repeat element reduce over unrelated shift", got)
	}
}

func TestResolveConflictsAugmentsAdjacentRepeatElementReduceLookahead(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "case", Kind: SymbolTerminal},
			{Name: "default", Kind: SymbolTerminal},
			{Name: "body", Kind: SymbolNamedToken},
			{Name: "group", Kind: SymbolNonterminal},
			{Name: "group_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "block", Kind: SymbolNonterminal},
			{Name: "unrelated_modifier", Kind: SymbolNonterminal},
		},
		Terminals: []TerminalPattern{{SymbolID: 0}, {SymbolID: 1}, {SymbolID: 2}, {SymbolID: 3}},
		Productions: []Production{
			{LHS: 4, RHS: []int{1, 3}},
			{LHS: 4, RHS: []int{2, 3}},
			{LHS: 5, RHS: []int{4}},
			{LHS: 5, RHS: []int{5, 4}},
			{LHS: 6, RHS: []int{5}},
			{LHS: 7, RHS: []int{2}},
		},
	}
	tables := &LRTables{
		ActionTable: map[int]map[int][]lrAction{
			0: {
				1: {{kind: lrReduce, prodIdx: 0}},
				2: {{kind: lrShift, state: 17, lhsSym: 7}},
			},
		},
	}

	if _, err := resolveConflicts(context.Background(), tables, ng); err != nil {
		t.Fatalf("resolveConflicts: %v", err)
	}
	got := tables.ActionTable[0][2]
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 0 {
		t.Fatalf("default actions=%+v, want augmented adjacent-repeat reduce to beat unrelated shift", got)
	}
}

func TestResolveConflictsDoesNotAugmentAdjacentRepeatElementReduceOntoReduceOnlyLookahead(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "case", Kind: SymbolTerminal},
			{Name: "default", Kind: SymbolTerminal},
			{Name: "body", Kind: SymbolNamedToken},
			{Name: "group", Kind: SymbolNonterminal},
			{Name: "group_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "block", Kind: SymbolNonterminal},
			{Name: "unrelated_modifier", Kind: SymbolNonterminal},
		},
		Terminals: []TerminalPattern{{SymbolID: 0}, {SymbolID: 1}, {SymbolID: 2}, {SymbolID: 3}},
		Productions: []Production{
			{LHS: 4, RHS: []int{1, 3}},
			{LHS: 4, RHS: []int{2, 3}},
			{LHS: 5, RHS: []int{4}},
			{LHS: 5, RHS: []int{5, 4}},
			{LHS: 6, RHS: []int{5}},
			{LHS: 7, RHS: []int{2}},
		},
	}
	tables := &LRTables{
		ActionTable: map[int]map[int][]lrAction{
			0: {
				1: {{kind: lrReduce, prodIdx: 0}},
				2: {{kind: lrReduce, prodIdx: 5}},
			},
		},
	}

	stats, err := augmentAdjacentRepeatElementReduceLookaheads(context.Background(), tables, ng)
	if err != nil {
		t.Fatalf("augmentAdjacentRepeatElementReduceLookaheads: %v", err)
	}
	if stats.AugmentStatesWithoutShiftTarget != 1 {
		t.Fatalf("AugmentStatesWithoutShiftTarget = %d, want 1", stats.AugmentStatesWithoutShiftTarget)
	}
	if stats.AugmentSecondPassReduceOnly != 0 {
		t.Fatalf("AugmentSecondPassReduceOnly = %d, want 0 after state-level skip", stats.AugmentSecondPassReduceOnly)
	}
	if stats.AugmentCandidateChecks != 0 {
		t.Fatalf("AugmentCandidateChecks = %d, want 0 for reduce-only target row", stats.AugmentCandidateChecks)
	}
	got := tables.ActionTable[0][2]
	if len(got) != 1 || got[0].kind != lrReduce || got[0].prodIdx != 5 {
		t.Fatalf("default actions=%+v, want original reduce-only action unchanged", got)
	}
}

func TestResolveActionConflictDoesNotKeepSeparatedRepeatElementReduce(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: ",", Kind: SymbolTerminal},
			{Name: "item_token", Kind: SymbolNamedToken},
			{Name: "item", Kind: SymbolNonterminal},
			{Name: "item_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "list", Kind: SymbolNonterminal},
			{Name: "separator", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: []int{2}},
			{LHS: 4, RHS: []int{3}},
			{LHS: 4, RHS: []int{4, 1, 3}},
			{LHS: 5, RHS: []int{4}},
			{LHS: 6, RHS: []int{1}, Prec: 8, HasExplicitPrec: true},
		},
	}

	got, err := resolveActionConflict(1, []lrAction{
		{kind: lrShift, state: 17, lhsSym: 6, prec: 8, hasPrec: true},
		{kind: lrReduce, prodIdx: 0},
	}, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 || got[0].kind != lrShift || got[0].state != 17 {
		t.Fatalf("resolved actions=%+v, want normal higher-precedence separator shift", got)
	}
}

func TestVisibleRecursiveRuleIsNotGeneratedRepeatHelperContinuation(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "atom", Kind: SymbolNamedToken},
			{Name: "S", Kind: SymbolNonterminal, Visible: true, Named: true},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
			{LHS: 2, RHS: []int{2, 2}, Prec: 1},
		},
	}

	actions := []lrAction{
		{kind: lrReduce, prodIdx: 1},
		{kind: lrShift, state: 23, lhsSym: 2, prec: 2, repeat: true, repeatLHS: 2},
	}

	got, err := resolveActionConflict(1, actions, ng)
	if err != nil {
		t.Fatalf("resolveActionConflict: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("resolved len=%d, want normal precedence resolution to one action: %+v", len(got), got)
	}
	if got[0].kind != lrShift || got[0].state != 23 {
		t.Fatalf("resolved=%+v, want visible recursive rule shift to win by precedence", got)
	}
}

func TestHiddenUserRecursiveRuleIsNotGeneratedRepeatHelper(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "atom", Kind: SymbolNamedToken},
			{Name: "_expr", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
			{LHS: 2, RHS: []int{2, 2}},
		},
	}

	cache := getConflictResolutionCache(ng)
	if isStructurallyGeneratedRepeatHelper(2, ng, cache) {
		t.Fatalf("hidden user recursive rule was classified as generated repeat helper")
	}
}

func TestNormalizeMarksGeneratedRepeatAuxSymbols(t *testing.T) {
	g := NewGrammar("generated_repeat_metadata")
	g.Define("source_file", Repeat(Sym("item")))
	g.Define("item", Str("x"))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}

	for _, sym := range ng.Symbols {
		if sym.GeneratedRepeatAux {
			if sym.Kind != SymbolNonterminal || sym.Visible || sym.Named {
				t.Fatalf("generated repeat aux metadata on unexpected symbol: %+v", sym)
			}
			lang, err := GenerateLanguage(g)
			if err != nil {
				t.Fatalf("GenerateLanguage: %v", err)
			}
			for _, meta := range lang.SymbolMetadata {
				if meta.GeneratedRepeatAux {
					return
				}
			}
			t.Fatalf("GenerateLanguage did not propagate generated repeat aux metadata")
		}
	}
	t.Fatalf("Normalize did not mark any generated repeat aux symbol")
}

func TestNormalizeMarksStructuralImportedRepeatAuxSymbols(t *testing.T) {
	g := NewGrammar("imported_repeat_metadata")
	g.Define("source_file", Sym("module_repeat1"))
	g.Define("module_repeat1", Choice(
		Seq(Sym("module_repeat1"), Sym("statement")),
		Sym("statement"),
	))
	g.Define("statement", Str("x"))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}

	found := false
	for _, sym := range ng.Symbols {
		if sym.Name != "module_repeat1" {
			continue
		}
		found = true
		if !sym.GeneratedRepeatAux {
			t.Fatalf("module_repeat1 GeneratedRepeatAux = false, want true")
		}
		if sym.Visible || sym.Named {
			t.Fatalf("module_repeat1 visible/named = %v/%v, want false/false", sym.Visible, sym.Named)
		}
	}
	if !found {
		t.Fatal("module_repeat1 symbol not found")
	}
}

func TestHiddenNameOnlyRepeatRuleIsNotGeneratedRepeatHelper(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "atom", Kind: SymbolNamedToken},
			{Name: "_expr_repeat1", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
			{LHS: 2, RHS: []int{2, 1}},
		},
	}

	cache := getConflictResolutionCache(ng)
	if isStructurallyGeneratedRepeatHelper(2, ng, cache) {
		t.Fatalf("name-only hidden recursive rule was classified as generated repeat helper")
	}
}

func TestShouldKeepDistinctRepeatReducesRequiresGeneratedRepeatMetadata(t *testing.T) {
	makeGrammar := func(generated bool) *NormalizedGrammar {
		return &NormalizedGrammar{
			Symbols: []SymbolInfo{
				{Name: "end", Kind: SymbolTerminal},
				{Name: "atom", Kind: SymbolNamedToken},
				{Name: "left_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: generated},
				{Name: "right_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: generated},
			},
			Productions: []Production{
				{LHS: 2, RHS: []int{1}},
				{LHS: 2, RHS: []int{2, 1}},
				{LHS: 3, RHS: []int{1}},
				{LHS: 3, RHS: []int{3, 1}},
			},
		}
	}
	reduces := []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 2},
	}

	if !shouldKeepDistinctRepeatReduces(reduces, makeGrammar(true)) {
		t.Fatalf("generated repeat aux reduces were not preserved")
	}
	if shouldKeepDistinctRepeatReduces(reduces, makeGrammar(false)) {
		t.Fatalf("name-only repeat aux reduces were preserved without metadata")
	}
}

func TestHiddenUnaryWrapperPruningSkipsGeneratedRepeatAux(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "token", Kind: SymbolTerminal},
			{Name: "item", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "_left_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "_right_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "left_parent", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "right_parent", Kind: SymbolNonterminal, Visible: true, Named: true},
		},
		Productions: []Production{
			{LHS: 2, RHS: []int{1}},
			{LHS: 3, RHS: []int{2}},
			{LHS: 4, RHS: []int{2}},
			{LHS: 5, RHS: []int{3}},
			{LHS: 6, RHS: []int{4}},
		},
	}
	reduces := []lrAction{
		{kind: lrReduce, prodIdx: 0},
		{kind: lrReduce, prodIdx: 1},
		{kind: lrReduce, prodIdx: 2},
		{kind: lrReduce, prodIdx: 3},
		{kind: lrReduce, prodIdx: 4},
	}
	cache := getConflictResolutionCache(ng)

	if _, ok := hiddenUnaryWrapperReduceAt(1, reduces[1], ng); ok {
		t.Fatalf("generated repeat aux was classified as a hidden unary wrapper")
	}
	if filtered, ok := filterRedundantHiddenUnaryWrapperReduces(reduces, ng, cache); ok || len(filtered) != len(reduces) {
		t.Fatalf("generated repeat aux reduces were pruned before repeat preservation: ok=%v len=%d", ok, len(filtered))
	}
}

func TestRHSCanBeginWithAnyFirstSetsMatchesIndependentOracle(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: "a", Kind: SymbolTerminal},
			{Name: "b", Kind: SymbolTerminal},
			{Name: "c", Kind: SymbolTerminal},
			{Name: "nullable", Kind: SymbolNonterminal},
			{Name: "_hidden", Kind: SymbolNonterminal},
			{Name: "named_wrapper", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "cycle_a", Kind: SymbolNonterminal},
			{Name: "cycle_b", Kind: SymbolNonterminal},
			{Name: "target_nt", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "nullable_suffix", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 4, RHS: nil},
			{LHS: 5, RHS: []int{9}},
			{LHS: 6, RHS: []int{4, 5, 10}},
			{LHS: 7, RHS: []int{8}},
			{LHS: 8, RHS: []int{7}},
			{LHS: 8, RHS: []int{2}},
			{LHS: 9, RHS: []int{1}},
			{LHS: 10, RHS: nil},
			{LHS: 10, RHS: []int{2}},
		},
	}
	cache := getConflictResolutionCache(ng)
	cases := []struct {
		name    string
		rhs     []int
		targets map[int]bool
	}{
		{name: "nullable prefix reaches hidden unary terminal", rhs: []int{4, 5}, targets: map[int]bool{1: true}},
		{name: "hidden unary exposes nonterminal target", rhs: []int{5}, targets: map[int]bool{9: true}},
		{name: "nullable suffix can begin after nullable prefix", rhs: []int{4, 10}, targets: map[int]bool{2: true}},
		{name: "nonnullable symbol blocks nullable suffix", rhs: []int{9, 10}, targets: map[int]bool{2: true}},
		{name: "cycle reaches terminal alternative", rhs: []int{7}, targets: map[int]bool{2: true}},
		{name: "cycle without target stays false", rhs: []int{7}, targets: map[int]bool{3: true}},
		{name: "wrapper reaches hidden nonterminal target", rhs: []int{6}, targets: map[int]bool{5: true}},
		{name: "nullable prefix reaches nonterminal target", rhs: []int{4, 5}, targets: map[int]bool{9: true}},
	}
	for _, tc := range cases {
		got, ok := rhsCanBeginWithAnyFirstSets(tc.rhs, tc.targets, cache, ng)
		if !ok {
			t.Fatalf("%s: first-set proof was unavailable", tc.name)
		}
		want := independentRHSCanBeginWithAny(tc.rhs, tc.targets, ng)
		if got != want {
			t.Fatalf("%s: got %v, want %v", tc.name, got, want)
		}
	}
}

func independentRHSCanBeginWithAny(rhs []int, targets map[int]bool, ng *NormalizedGrammar) bool {
	nullable := independentNullableSymbols(ng)
	visiting := make([]bool, len(ng.Symbols))
	var rhsCanBegin func([]int) bool
	var symCanBegin func(int) bool
	rhsCanBegin = func(seq []int) bool {
		for _, sym := range seq {
			if symCanBegin(sym) {
				return true
			}
			if sym < 0 || sym >= len(nullable) || !nullable[sym] {
				return false
			}
		}
		return false
	}
	symCanBegin = func(sym int) bool {
		if sym < 0 || sym >= len(ng.Symbols) {
			return false
		}
		if targets[sym] {
			return true
		}
		if ng.Symbols[sym].Kind != SymbolNonterminal || visiting[sym] {
			return false
		}
		visiting[sym] = true
		defer func() { visiting[sym] = false }()
		for _, prod := range ng.Productions {
			if prod.LHS == sym && rhsCanBegin(prod.RHS) {
				return true
			}
		}
		return false
	}
	return rhsCanBegin(rhs)
}

func independentNullableSymbols(ng *NormalizedGrammar) []bool {
	nullable := make([]bool, len(ng.Symbols))
	changed := true
	for changed {
		changed = false
		for _, prod := range ng.Productions {
			if prod.LHS < 0 || prod.LHS >= len(nullable) || nullable[prod.LHS] {
				continue
			}
			allNullable := true
			for _, sym := range prod.RHS {
				if sym < 0 || sym >= len(nullable) || !nullable[sym] {
					allNullable = false
					break
				}
			}
			if allNullable {
				nullable[prod.LHS] = true
				changed = true
			}
		}
	}
	return nullable
}

func TestRepeatElementAdjacentRepeatLookaheadPrecomputeMatchesSlowLogic(t *testing.T) {
	ng := &NormalizedGrammar{
		Symbols: []SymbolInfo{
			{Name: "end", Kind: SymbolTerminal},
			{Name: ",", Kind: SymbolTerminal},
			{Name: "item_token", Kind: SymbolNamedToken},
			{Name: "maybe_separator", Kind: SymbolNonterminal},
			{Name: "item", Kind: SymbolNonterminal, Visible: true, Named: true},
			{Name: "item_repeat1", Kind: SymbolNonterminal, GeneratedRepeatAux: true},
			{Name: "unrelated", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 3, RHS: nil},
			{LHS: 3, RHS: []int{1}},
			{LHS: 4, RHS: []int{2}},
			{LHS: 5, RHS: []int{3, 4}},
			{LHS: 5, RHS: []int{5, 3, 4}},
			{LHS: 6, RHS: []int{1}},
		},
	}
	cache := getConflictResolutionCache(ng)
	for elemLHS := range ng.Symbols {
		for lookahead := range ng.Symbols {
			got := repeatElementCanStartAdjacentRepeatOnLookahead(elemLHS, lookahead, ng, cache)
			want := repeatElementCanStartAdjacentRepeatOnLookaheadSlow(elemLHS, lookahead, ng, cache)
			if got != want {
				t.Fatalf("repeat start elem=%s lookahead=%s got %v, want %v",
					ng.Symbols[elemLHS].Name, ng.Symbols[lookahead].Name, got, want)
			}
		}
	}
}

func repeatElementCanStartAdjacentRepeatOnLookaheadSlow(elemLHS, lookaheadSym int, ng *NormalizedGrammar, cache *conflictResolutionCache) bool {
	if elemLHS < 0 || elemLHS >= len(cache.rhsParents) ||
		lookaheadSym < 0 || lookaheadSym >= len(ng.Symbols) {
		return false
	}
	lookaheadTargets := map[int]bool{lookaheadSym: true}
	for _, repeatSym := range cache.rhsParents[elemLHS] {
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
			if rhsCanStartWithSymbol(rhs, elemLHS, cache) &&
				rhsCanBeginWithAny(rhs, lookaheadTargets, cache, ng) {
				return true
			}
		}
	}
	return false
}
