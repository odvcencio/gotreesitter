package gotreesitter

import (
	"bytes"
	"encoding/gob"
	"testing"
)

func TestConflictPolicyChoiceRepetitionShift(t *testing.T) {
	lang := &Language{
		Name:                  "synthetic_policy_test",
		GeneratedByGrammargen: true,
		ConflictPolicies:      []ConflictPolicy{{State: 42, Lookahead: 7, Kind: ConflictPolicyRepetitionShift, ReduceSymbols: []Symbol{3}}},
		SymbolNames:           []string{"end", "token", "other", "items_repeat1"},
		SymbolMetadata:        []SymbolMetadata{{Name: "end"}, {Name: "token"}, {Name: "other"}, {Name: "items_repeat1"}},
	}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 2},
		{Type: ParseActionShift, State: 99, Repetition: true},
	}

	chosen, ok := conflictPolicyChoice(lang, Token{Symbol: 7}, 42, actions)
	if !ok {
		t.Fatal("conflictPolicyChoice = false, want true")
	}
	if chosen.Type != ParseActionShift || chosen.State != 99 || !chosen.Repetition {
		t.Fatalf("conflictPolicyChoice picked %+v, want repetition shift", chosen)
	}

	parser := &Parser{language: lang}
	chosen, ok = parser.deterministicConflictChoiceForDispatch(nil, nil, Token{Symbol: 7}, 42, actions, 1, nil)
	if !ok {
		t.Fatal("deterministicConflictChoiceForDispatch = false, want generated policy choice")
	}
	if chosen.Type != ParseActionShift || chosen.State != 99 || !chosen.Repetition {
		t.Fatalf("deterministicConflictChoiceForDispatch picked %+v, want repetition shift", chosen)
	}
}

func TestConflictPolicyChoiceRepetitionReduce(t *testing.T) {
	lang := &Language{
		Name:             "synthetic_policy_test",
		ConflictPolicies: []ConflictPolicy{{State: 42, Lookahead: 7, Kind: ConflictPolicyRepetitionReduce, ReduceSymbols: []Symbol{3}}},
	}
	reduce := ParseAction{Type: ParseActionReduce, Symbol: 3, ChildCount: 2}
	actions := []ParseAction{
		reduce,
		{Type: ParseActionShift, State: 99, Repetition: true},
	}

	chosen, ok := conflictPolicyChoice(lang, Token{Symbol: 7}, 42, actions)
	if !ok {
		t.Fatal("conflictPolicyChoice = false, want reduce policy choice")
	}
	if chosen != reduce {
		t.Fatalf("conflictPolicyChoice picked %+v, want %+v", chosen, reduce)
	}
}

func TestConflictPolicyChoiceCompactFrontierMinimum(t *testing.T) {
	lang := &Language{
		ConflictPolicies: []ConflictPolicy{{
			State: 42, Lookahead: 7, Kind: ConflictPolicyRepetitionReduce,
			CompactOnly: true, CompactMinFrontierHeaders: 2, ReduceSymbols: []Symbol{3},
		}},
	}
	reduce := ParseAction{Type: ParseActionReduce, Symbol: 3, ChildCount: 2}
	actions := []ParseAction{
		reduce,
		{Type: ParseActionShift, State: 99, Repetition: true},
	}

	if chosen, ok := conflictPolicyChoice(lang, Token{Symbol: 7}, 42, actions); ok {
		t.Fatalf("production conflict policy choice = %+v, want no choice", chosen)
	}
	if chosen, ok := conflictPolicyChoiceForCompact(lang, Token{Symbol: 7}, 42, actions, 1); ok {
		t.Fatalf("single-header compact choice = %+v, want no choice", chosen)
	}
	if chosen, ok := conflictPolicyChoiceForCompact(lang, Token{Symbol: 7}, 42, actions, 2); !ok || chosen != reduce {
		t.Fatalf("two-header compact choice = %+v, %t, want reduce", chosen, ok)
	}
}

func TestConflictPolicyChoiceShift(t *testing.T) {
	lang := &Language{
		Name:                  "synthetic_policy_test",
		GeneratedByGrammargen: true,
		ConflictPolicies:      []ConflictPolicy{{State: 42, Lookahead: 7, Kind: ConflictPolicyShift, ReduceSymbols: []Symbol{3}}},
	}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
		{Type: ParseActionShift, State: 99},
	}

	chosen, ok := conflictPolicyChoice(lang, Token{Symbol: 7}, 42, actions)
	if !ok {
		t.Fatal("conflictPolicyChoice = false, want shift policy choice")
	}
	if chosen.Type != ParseActionShift || chosen.State != 99 {
		t.Fatalf("conflictPolicyChoice picked %+v, want shift", chosen)
	}
}

func TestConflictPolicyChoiceShiftRequiresReduceSymbols(t *testing.T) {
	lang := &Language{
		ConflictPolicies: []ConflictPolicy{{State: 42, Lookahead: 7, Kind: ConflictPolicyShift}},
	}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
		{Type: ParseActionShift, State: 99},
	}

	if chosen, ok := conflictPolicyChoice(lang, Token{Symbol: 7}, 42, actions); ok {
		t.Fatalf("conflictPolicyChoice picked %+v, want no unconstrained shift policy", chosen)
	}
}

func TestConflictPolicyChoiceShiftRejectsExtraAndRepetitionShifts(t *testing.T) {
	for _, tc := range []struct {
		name  string
		shift ParseAction
	}{
		{
			name:  "extra",
			shift: ParseAction{Type: ParseActionShift, State: 99, Extra: true},
		},
		{
			name:  "repetition",
			shift: ParseAction{Type: ParseActionShift, State: 99, Repetition: true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lang := &Language{
				ConflictPolicies: []ConflictPolicy{{State: 42, Lookahead: 7, Kind: ConflictPolicyShift, ReduceSymbols: []Symbol{3}}},
			}
			actions := []ParseAction{
				{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
				tc.shift,
			}

			if chosen, ok := conflictPolicyChoice(lang, Token{Symbol: 7}, 42, actions); ok {
				t.Fatalf("conflictPolicyChoice picked %+v, want rejection", chosen)
			}
		})
	}
}

func TestConflictPolicyChoiceRejectsNonmatchingStateLookahead(t *testing.T) {
	lang := &Language{
		ConflictPolicies: []ConflictPolicy{{State: 42, Lookahead: 7, Kind: ConflictPolicyRepetitionShift}},
	}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 2},
		{Type: ParseActionShift, State: 99, Repetition: true},
	}

	if _, ok := conflictPolicyChoice(lang, Token{Symbol: 7}, 41, actions); ok {
		t.Fatal("conflictPolicyChoice matched wrong state")
	}
	if _, ok := conflictPolicyChoice(lang, Token{Symbol: 8}, 42, actions); ok {
		t.Fatal("conflictPolicyChoice matched wrong lookahead")
	}
}

func TestConflictPolicyChoiceAllowsPolicyNamedGeneratedRepeatBoundary(t *testing.T) {
	lang := &Language{
		Name:                  "synthetic_policy_test",
		GeneratedByGrammargen: true,
		ConflictPolicies:      []ConflictPolicy{{State: 42, Lookahead: 7, Kind: ConflictPolicyRepetitionShift, ReduceSymbols: []Symbol{3}}},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end"},
			{Name: "token"},
			{Name: "other"},
			{Name: "items_repeat1", GeneratedRepeatAux: true},
		},
	}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 2},
		{Type: ParseActionShift, State: 99, Repetition: true},
	}

	chosen, ok := conflictPolicyChoice(lang, Token{Symbol: 7}, 42, actions)
	if !ok {
		t.Fatal("conflictPolicyChoice = false, want generated-repeat policy choice")
	}
	if chosen.Type != ParseActionShift || chosen.State != 99 || !chosen.Repetition {
		t.Fatalf("conflictPolicyChoice picked %+v, want repetition shift", chosen)
	}

	parser := &Parser{language: lang}
	chosen, ok = parser.deterministicConflictChoiceForDispatch(nil, nil, Token{Symbol: 7}, 42, actions, 1, nil)
	if !ok {
		t.Fatal("deterministicConflictChoiceForDispatch = false, want generated policy choice")
	}
	if chosen.Type != ParseActionShift || chosen.State != 99 || !chosen.Repetition {
		t.Fatalf("deterministicConflictChoiceForDispatch picked %+v, want repetition shift", chosen)
	}
}

func TestConflictPolicyChoiceRejectsGeneratedRepeatBoundaryPolicyMiss(t *testing.T) {
	lang := &Language{
		Name:                  "synthetic_policy_test",
		GeneratedByGrammargen: true,
		ConflictPolicies:      []ConflictPolicy{{State: 42, Lookahead: 8, Kind: ConflictPolicyRepetitionShift, ReduceSymbols: []Symbol{3}}},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end"},
			{Name: "token"},
			{Name: "other"},
			{Name: "items_repeat1", GeneratedRepeatAux: true},
		},
	}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 2},
		{Type: ParseActionShift, State: 99, Repetition: true},
	}

	if _, ok := conflictPolicyChoice(lang, Token{Symbol: 7}, 42, actions); ok {
		t.Fatal("conflictPolicyChoice matched wrong lookahead")
	}
	parser := &Parser{language: lang}
	if chosen, ok := parser.deterministicConflictChoiceForDispatch(nil, nil, Token{Symbol: 7}, 42, actions, 1, nil); ok {
		t.Fatalf("deterministicConflictChoiceForDispatch picked %+v, want generated-repeat-boundary veto after policy miss", chosen)
	}
}

func TestConflictPolicyChoiceDispatchesConcreteProgramRepeatShape(t *testing.T) {
	lang := &Language{
		Name:                  "synthetic_policy_test",
		GeneratedByGrammargen: true,
		SymbolNames:           []string{"end", "namespace", "\\", "name", "use", "new", "program_repeat1"},
		ConflictPolicies:      []ConflictPolicy{{State: 2, Lookahead: 1, Kind: ConflictPolicyRepetitionShift, ReduceSymbols: []Symbol{6}}},
	}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 6, ChildCount: 2},
		{Type: ParseActionShift, State: 1846, Repetition: true},
	}

	policy, ok := conflictPolicyChoice(lang, Token{Symbol: 1}, 2, actions)
	if !ok {
		t.Fatal("conflictPolicyChoice = false, want concrete program-repeat policy choice")
	}
	if policy.Type != ParseActionShift || policy.State != 1846 || !policy.Repetition {
		t.Fatalf("conflictPolicyChoice picked %+v, want repetition shift", policy)
	}

	parser := &Parser{language: lang}
	chosen, ok := parser.deterministicConflictChoiceForDispatch(nil, nil, Token{Symbol: 1}, 2, actions, 1, nil)
	if !ok {
		t.Fatal("deterministicConflictChoiceForDispatch = false, want generated policy choice")
	}
	if chosen != policy {
		t.Fatalf("deterministicConflictChoiceForDispatch picked %+v, want policy %+v", chosen, policy)
	}
}

func TestGeneratedConflictPolicyPrecedesLanguageFallbacks(t *testing.T) {
	for _, name := range []string{"java", "dart", "typescript"} {
		t.Run(name, func(t *testing.T) {
			lang := &Language{
				Name:                  name,
				GeneratedByGrammargen: true,
				SymbolNames:           []string{"end", "identifier", "statement_list"},
				SymbolMetadata:        []SymbolMetadata{{Name: "end"}, {Name: "identifier"}, {Name: "statement_list"}},
				ConflictPolicies:      []ConflictPolicy{{State: 123, Lookahead: 1, Kind: ConflictPolicyRepetitionShift, ReduceSymbols: []Symbol{2}}},
			}
			actions := []ParseAction{
				{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
				{Type: ParseActionShift, State: 456, Repetition: true},
			}

			parser := &Parser{language: lang}
			chosen, ok := parser.deterministicConflictChoiceForDispatch(nil, nil, Token{Symbol: 1}, 123, actions, 1, nil)
			if !ok {
				t.Fatal("deterministicConflictChoiceForDispatch = false, want generated policy before language switch")
			}
			if chosen.Type != ParseActionShift || chosen.State != 456 || !chosen.Repetition {
				t.Fatalf("deterministicConflictChoiceForDispatch picked %+v, want policy repetition shift", chosen)
			}
		})
	}
}

func TestEmbeddedConflictPolicyPrecedesGlobalRepetitionFold(t *testing.T) {
	lang := &Language{
		Name:             "php",
		SymbolNames:      []string{"end", "namespace", "\\", "name", "use", "new", "program_repeat1", "list"},
		SymbolMetadata:   []SymbolMetadata{{Name: "end"}, {Name: "namespace"}, {Name: "\\"}, {Name: "name"}, {Name: "use"}, {Name: "new"}, {Name: "program_repeat1"}, {Name: "list"}},
		ConflictPolicies: []ConflictPolicy{{State: 123, Lookahead: 1, Kind: ConflictPolicyRepetitionShift, ReduceSymbols: []Symbol{7}}},
	}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 7, ChildCount: 2},
		{Type: ParseActionShift, State: 456, Repetition: true},
	}
	parser := &Parser{language: lang}
	chosen, ok := parser.deterministicConflictChoiceForDispatch(nil, &glrStack{}, Token{Symbol: 1}, 123, actions, 1, nil)
	if !ok {
		t.Fatal("deterministicConflictChoiceForDispatch = false, want embedded policy choice")
	}
	if chosen.Type != ParseActionShift || chosen.State != 456 || !chosen.Repetition {
		t.Fatalf("deterministicConflictChoiceForDispatch picked %+v, want policy repetition shift", chosen)
	}
}

func TestGeneratedConflictPolicyMissUsesGeneratedBoundaryVeto(t *testing.T) {
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 6, ChildCount: 2},
		{Type: ParseActionShift, State: 1846, Repetition: true},
	}
	for _, tc := range []struct {
		name     string
		policies []ConflictPolicy
	}{
		{name: "no policy"},
		{
			name:     "nonmatching policy",
			policies: []ConflictPolicy{{State: 99, Lookahead: 1, Kind: ConflictPolicyRepetitionShift, ReduceSymbols: []Symbol{6}}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lang := &Language{
				Name:                  "php",
				GeneratedByGrammargen: true,
				SymbolNames:           []string{"end", "namespace", "\\", "name", "use", "new", "program_repeat1"},
				SymbolMetadata: []SymbolMetadata{
					{Name: "end"},
					{Name: "namespace"},
					{Name: "\\"},
					{Name: "name"},
					{Name: "use"},
					{Name: "new"},
					{Name: "program_repeat1"},
				},
				ConflictPolicies: tc.policies,
			}
			parser := &Parser{language: lang}
			if chosen, ok := parser.deterministicConflictChoiceForDispatch(nil, nil, Token{Symbol: 1}, 2, actions, 1, nil); ok {
				t.Fatalf("deterministicConflictChoiceForDispatch picked %+v, want no generated fallback", chosen)
			}
		})
	}
}

func TestForestResolveConflictUsesGeneratedPolicy(t *testing.T) {
	lang := &Language{
		Name:                  "synthetic_policy_test",
		GeneratedByGrammargen: true,
		ConflictPolicies:      []ConflictPolicy{{State: 42, Lookahead: 7, Kind: ConflictPolicyRepetitionShift, ReduceSymbols: []Symbol{3}}},
	}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 2},
		{Type: ParseActionShift, State: 99, Repetition: true},
	}

	got := NewParser(lang).forestResolveConflict(42, Token{Symbol: 7}, actions)
	if len(got) != 1 {
		t.Fatalf("forestResolveConflict returned %d actions, want singleton", len(got))
	}
	if got[0].Type != ParseActionShift || got[0].State != 99 || !got[0].Repetition {
		t.Fatalf("forestResolveConflict picked %+v, want generated policy repetition shift", got[0])
	}
}

func TestForestResolveConflictGeneratedPolicyMissKeepsActions(t *testing.T) {
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 6, ChildCount: 2},
		{Type: ParseActionShift, State: 1846, Repetition: true},
	}
	lang := &Language{
		Name:                  "php",
		GeneratedByGrammargen: true,
		SymbolNames:           []string{"end", "namespace", "\\", "name", "use", "new", "program_repeat1"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end"},
			{Name: "namespace"},
			{Name: "\\"},
			{Name: "name"},
			{Name: "use"},
			{Name: "new"},
			{Name: "program_repeat1"},
		},
		ConflictPolicies: []ConflictPolicy{{State: 99, Lookahead: 1, Kind: ConflictPolicyRepetitionShift, ReduceSymbols: []Symbol{6}}},
	}
	got := NewParser(lang).forestResolveConflict(2, Token{Symbol: 1}, actions)
	if len(got) != len(actions) {
		t.Fatalf("forestResolveConflict returned %d actions, want original %d", len(got), len(actions))
	}
	for i := range actions {
		if got[i] != actions[i] {
			t.Fatalf("forestResolveConflict[%d] = %+v, want original %+v", i, got[i], actions[i])
		}
	}
}

// TestForestResolveConflictDotLegacyUsesStateAndLookahead exercises
// forestResolveConflict's generic ConflictPolicy check using a synthetic
// state/lookahead-scoped row shaped like dot's retired
// dotRepetitionShiftConflictChoice helper. dot itself ships no such policy
// (see grammars/runtime_profiles.go's "NOTE on dot"); this
// only covers the state+lookahead exact-match mechanism the helper's shape
// would have used had it been certified.
func TestForestResolveConflictDotLegacyUsesStateAndLookahead(t *testing.T) {
	lang := &Language{
		Name:           "dot",
		SymbolNames:    []string{"end", "identifier", "other", "stmt_list_repeat1"},
		SymbolMetadata: []SymbolMetadata{{Name: "end"}, {Name: "identifier"}, {Name: "other"}, {Name: "stmt_list_repeat1"}},
		ConflictPolicies: []ConflictPolicy{
			{State: 4, Lookahead: 1, Kind: ConflictPolicyRepetitionShift, ReduceSymbols: []Symbol{3}},
		},
	}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 2},
		{Type: ParseActionShift, State: 99, Repetition: true},
	}

	parser := NewParser(lang)
	got := parser.forestResolveConflict(4, Token{Symbol: 1}, actions)
	if len(got) != 1 {
		t.Fatalf("forestResolveConflict returned %d actions, want dot singleton", len(got))
	}
	if got[0].Type != ParseActionShift || got[0].State != 99 || !got[0].Repetition {
		t.Fatalf("forestResolveConflict picked %+v, want dot repetition shift", got[0])
	}

	got = parser.forestResolveConflict(5, Token{Symbol: 1}, actions)
	if len(got) != len(actions) {
		t.Fatalf("forestResolveConflict with wrong state returned %d actions, want original %d", len(got), len(actions))
	}
	got = parser.forestResolveConflict(4, Token{Symbol: 2}, actions)
	if len(got) != len(actions) {
		t.Fatalf("forestResolveConflict with wrong lookahead returned %d actions, want original %d", len(got), len(actions))
	}
}

func TestCRepetitionSkipForestConflictChoiceHonorsOptOutAndRecoveryGate(t *testing.T) {
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 2},
		{Type: ParseActionShift, State: 99, Repetition: true},
	}

	parser := NewParser(&Language{Name: "bash"})
	chosen, ok := parser.cRepetitionSkipForestConflictChoice(false, actions)
	if !ok {
		t.Fatal("cRepetitionSkipForestConflictChoice = false for non-recover non-opt-out language, want fold")
	}
	if chosen.Type != ParseActionReduce || chosen.Symbol != 3 {
		t.Fatalf("cRepetitionSkipForestConflictChoice picked %+v, want reduce", chosen)
	}

	if chosen, ok := parser.cRepetitionSkipForestConflictChoice(true, actions); ok {
		t.Fatalf("cRepetitionSkipForestConflictChoice picked %+v while recovery-active, want GLR fork", chosen)
	}

	parser = NewParser(&Language{Name: "dart"})
	if chosen, ok := parser.cRepetitionSkipForestConflictChoice(false, actions); ok {
		t.Fatalf("cRepetitionSkipForestConflictChoice picked %+v for opt-out language, want GLR fork", chosen)
	}
}

func TestRecoveredRepetitionReducePolicyAppliesOnlyToIdleRecoveredLineage(t *testing.T) {
	lang := &Language{
		Name: "caddy",
		ConflictPolicies: []ConflictPolicy{{
			State:         86,
			Lookahead:     25,
			Kind:          ConflictPolicyRecoveredRepetitionReduce,
			ReduceSymbols: []Symbol{68},
		}},
	}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 68, ChildCount: 2},
		{Type: ParseActionShift, State: 126, Repetition: true},
	}
	parser := NewParser(lang)
	tok := Token{Symbol: 25}

	if chosen, ok := conflictPolicyChoice(lang, tok, 86, actions); ok {
		t.Fatalf("generic conflictPolicyChoice picked recovery-scoped policy %+v", chosen)
	}

	recovered := &glrStack{cEverErrored: true}
	chosen, ok := parser.deterministicConflictChoiceForDispatch(nil, recovered, tok, 86, actions, 2, nil)
	if !ok {
		t.Fatal("recovered policy did not choose the certified row")
	}
	if chosen.Type != ParseActionReduce || chosen.Symbol != 68 {
		t.Fatalf("recovered policy picked %+v, want reduce symbol 68", chosen)
	}
	chosen, ok = parser.deterministicConflictChoiceForDispatch(nil, recovered, tok, 86, actions, 2, &reuseCursor{})
	if !ok || chosen.Type != ParseActionReduce || chosen.Symbol != 68 {
		t.Fatalf("incremental-reuse dispatch picked %+v, want global C repetition reduce", chosen)
	}

	// Keep the two mechanisms distinct: c_sharp opts out of the global C fold,
	// so this otherwise identical recovery policy must still be gated off while
	// dispatching around reused syntax.
	optOutLang := &Language{
		Name:             "c_sharp",
		ConflictPolicies: lang.ConflictPolicies,
	}
	optOutParser := NewParser(optOutLang)
	chosen, ok = optOutParser.deterministicConflictChoiceForDispatch(nil, recovered, tok, 86, actions, 2, nil)
	if !ok || chosen.Type != ParseActionReduce || chosen.Symbol != 68 {
		t.Fatalf("opt-out fresh dispatch picked %+v, want recovery-scoped reduce", chosen)
	}
	if chosen, ok := optOutParser.deterministicConflictChoiceForDispatch(nil, recovered, tok, 86, actions, 2, &reuseCursor{}); ok {
		t.Fatalf("opt-out incremental-reuse dispatch picked recovery-scoped policy %+v", chosen)
	}

	activeRecovery := []*glrStack{
		{cEverErrored: true, cRec: &cRecoverState{}},
		{cEverErrored: true, cPaused: true},
		{cEverErrored: true, cRecoverMissingGroup: &cRecGroup{}},
	}
	for i, stack := range activeRecovery {
		if chosen, ok := conflictPolicyChoiceForDispatch(lang, stack, tok, 86, actions); ok {
			t.Fatalf("active recovery case %d picked recovered policy %+v, want policy miss", i, chosen)
		}
	}
}

func TestRecoveredRepetitionReducePolicyRequiresExactRowAndShape(t *testing.T) {
	lang := &Language{
		Name: "caddy",
		ConflictPolicies: []ConflictPolicy{{
			State:         86,
			Lookahead:     25,
			Kind:          ConflictPolicyRecoveredRepetitionReduce,
			ReduceSymbols: []Symbol{68},
		}},
	}
	stack := &glrStack{cEverErrored: true}
	valid := []ParseAction{
		{Type: ParseActionReduce, Symbol: 68, ChildCount: 2},
		{Type: ParseActionShift, State: 126, Repetition: true},
	}
	tests := []struct {
		name    string
		state   StateID
		token   Symbol
		actions []ParseAction
	}{
		{name: "wrong state", state: 87, token: 25, actions: valid},
		{name: "wrong lookahead", state: 86, token: 24, actions: valid},
		{name: "wrong reduce", state: 86, token: 25, actions: []ParseAction{{Type: ParseActionReduce, Symbol: 67}, {Type: ParseActionShift, State: 126, Repetition: true}}},
		{name: "plain shift", state: 86, token: 25, actions: []ParseAction{{Type: ParseActionReduce, Symbol: 68}, {Type: ParseActionShift, State: 126}}},
		{name: "multiple reduces", state: 86, token: 25, actions: []ParseAction{{Type: ParseActionReduce, Symbol: 68}, {Type: ParseActionReduce, Symbol: 68}, {Type: ParseActionShift, State: 126, Repetition: true}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if chosen, ok := conflictPolicyChoiceForDispatch(lang, stack, Token{Symbol: tc.token}, tc.state, tc.actions); ok {
				t.Fatalf("picked recovered policy %+v, want policy miss", chosen)
			}
		})
	}
}

func TestRecoveredRepetitionReducePolicyDoesNotEnableGeneratedBoundaryVeto(t *testing.T) {
	lang := &Language{
		Name: "caddy",
		SymbolMetadata: []SymbolMetadata{
			{},
			{GeneratedRepeatAux: true},
		},
		ConflictPolicies: []ConflictPolicy{{
			State:         86,
			Lookahead:     25,
			Kind:          ConflictPolicyRecoveredRepetitionReduce,
			ReduceSymbols: []Symbol{1},
		}},
	}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 1, ChildCount: 2},
		{Type: ParseActionShift, State: 126, Repetition: true},
	}
	if generatedRepeatBoundaryConflict(lang, actions) {
		t.Fatal("recovery-only policy enabled the generator boundary veto")
	}
}

func TestRecoveredRepetitionReducePolicyDoesNotAffectForestRecovery(t *testing.T) {
	lang := &Language{
		Name: "caddy",
		ConflictPolicies: []ConflictPolicy{{
			State:         86,
			Lookahead:     25,
			Kind:          ConflictPolicyRecoveredRepetitionReduce,
			ReduceSymbols: []Symbol{68},
		}},
	}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 68, ChildCount: 2},
		{Type: ParseActionShift, State: 126, Repetition: true},
	}
	got := NewParser(lang).forestResolveConflict(86, Token{Symbol: 25}, actions)
	if len(got) != len(actions) {
		t.Fatalf("forest returned %d actions, want original %d", len(got), len(actions))
	}
}

func TestConflictPolicyGobRoundTrip(t *testing.T) {
	lang := &Language{
		Name: "synthetic_policy_test",
		ConflictPolicies: []ConflictPolicy{
			{State: 42, Lookahead: 7, Kind: ConflictPolicyRepetitionShift, CompactOnly: true, CompactMinFrontierHeaders: 2, ReduceSymbols: []Symbol{3, 4}},
			{State: 86, Lookahead: 25, Kind: ConflictPolicyRecoveredRepetitionReduce, ReduceSymbols: []Symbol{68}},
		},
	}

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(lang); err != nil {
		t.Fatalf("encode Language with ConflictPolicies: %v", err)
	}
	var decoded Language
	if err := gob.NewDecoder(&buf).Decode(&decoded); err != nil {
		t.Fatalf("decode Language with ConflictPolicies: %v", err)
	}
	if len(decoded.ConflictPolicies) != 2 {
		t.Fatalf("decoded %d policies, want 2", len(decoded.ConflictPolicies))
	}
	got := decoded.ConflictPolicies[0]
	if got.State != 42 || got.Lookahead != 7 || got.Kind != ConflictPolicyRepetitionShift || !got.CompactOnly || got.CompactMinFrontierHeaders != 2 {
		t.Fatalf("decoded policy = %+v, want state/lookahead/kind preserved", got)
	}
	if len(got.ReduceSymbols) != 2 || got.ReduceSymbols[0] != 3 || got.ReduceSymbols[1] != 4 {
		t.Fatalf("decoded ReduceSymbols = %v, want [3 4]", got.ReduceSymbols)
	}
	recovered := decoded.ConflictPolicies[1]
	if recovered.State != 86 || recovered.Lookahead != 25 || recovered.Kind != ConflictPolicyRecoveredRepetitionReduce || len(recovered.ReduceSymbols) != 1 || recovered.ReduceSymbols[0] != 68 {
		t.Fatalf("decoded recovered policy = %+v, want exact row preserved", recovered)
	}
}

func TestCompactOnlyDeclaredConflictDoesNotFoldLegacy(t *testing.T) {
	lang := &Language{ConflictPolicies: []ConflictPolicy{{
		State: 42, Lookahead: 7, Kind: ConflictPolicyDeclaredReduceReduceHighestSymbol,
		CompactOnly: true, ReduceSymbols: []Symbol{3, 4},
	}}}
	actions := []ParseAction{{Type: ParseActionReduce, Symbol: 3, ChildCount: 2}, {Type: ParseActionReduce, Symbol: 4, ChildCount: 2}}
	if action, ok := declaredReduceReduceConflictPolicyChoice(lang, 42, Token{Symbol: 7}, actions); ok {
		t.Fatalf("legacy folded a compact-only conflict: %+v", action)
	}
	if action, ok := conflictPolicyChoiceForCompact(lang, Token{Symbol: 7}, 42, actions, 1); !ok || action != actions[1] {
		t.Fatalf("compact fold = %+v, %t; want %+v", action, ok, actions[1])
	}
}
