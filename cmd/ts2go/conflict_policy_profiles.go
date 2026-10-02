package main

import (
	"crypto/sha256"
	"fmt"
	"sort"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// certifiedConflictPolicyProfile contains narrow conflict decisions that
// cannot be inferred safely for every generated-repeat row. A profile is tied
// to the exact parser.c bytes and must fail regeneration when its source or
// table shape changes so the decision is recertified rather than silently
// carried onto a different grammar.
type certifiedConflictPolicyProfile struct {
	parserCSHA256    string
	stateCount       int
	symbolCount      int
	policies         []certifiedRecoveredRepetitionReducePolicy
	unaryReducePairs [][2]int
}

type certifiedRecoveredRepetitionReducePolicy struct {
	state          int
	lookahead      int
	lookaheadName  string
	reduceSymbol   int
	reduceName     string
	reduceChildren int
	shiftState     int
}

var certifiedConflictPolicyProfiles = map[string]certifiedConflictPolicyProfile{
	"purescript": {
		// Same pinned grammar, with C's single-term expression choice.
		// _fexp (188) keeps the direct expression; _exp_apply (187)
		// introduces an application alias even when there is only one term.
		parserCSHA256:    "1d35f0257ef3d9edcd8964ae2762a6e1e830f19d8a8142c4e07e4d237f4f4957",
		stateCount:       10072,
		symbolCount:      305,
		unaryReducePairs: [][2]int{{187, 188}},
	},
	"caddy": {
		// tree-sitter-caddy 9b3fde99d3d74345b85b655a6d8065e004fbe26f.
		parserCSHA256: "bf289eeb9aca008f0e981f66c6bcf5a32faa4434f4373dd4d5d8d6fcec7bff6e",
		stateCount:    192,
		symbolCount:   70,
		policies: []certifiedRecoveredRepetitionReducePolicy{
			{
				state:          86,
				lookahead:      25,
				lookaheadName:  "string_literal_token1",
				reduceSymbol:   68,
				reduceName:     "string_literal_repeat1",
				reduceChildren: 2,
				shiftState:     126,
			},
		},
	},
}

func applyCertifiedConflictPolicyProfiles(source []byte, grammar *ExtractedGrammar, lang *gotreesitter.Language) error {
	if grammar == nil || lang == nil {
		return fmt.Errorf("certified conflict policy: nil grammar or language")
	}
	profile, ok := certifiedConflictPolicyProfiles[grammar.Name]
	if !ok {
		return nil
	}
	return applyCertifiedConflictPolicyProfile(source, grammar, lang, profile)
}

func applyCertifiedConflictPolicyProfile(source []byte, grammar *ExtractedGrammar, lang *gotreesitter.Language, profile certifiedConflictPolicyProfile) error {
	if grammar == nil || lang == nil {
		return fmt.Errorf("certified conflict policy: nil grammar or language")
	}
	sum := sha256.Sum256(source)
	if got := fmt.Sprintf("%x", sum[:]); got != profile.parserCSHA256 {
		return fmt.Errorf("certified conflict policy %q: parser.c sha256 = %s, want %s; recertification required", grammar.Name, got, profile.parserCSHA256)
	}
	if grammar.StateCount != profile.stateCount {
		return fmt.Errorf("certified conflict policy %q: state count = %d, want %d", grammar.Name, grammar.StateCount, profile.stateCount)
	}
	if grammar.SymbolCount != profile.symbolCount {
		return fmt.Errorf("certified conflict policy %q: symbol count = %d, want %d", grammar.Name, grammar.SymbolCount, profile.symbolCount)
	}
	for _, pair := range profile.unaryReducePairs {
		found := false
		for _, group := range grammar.ParseActions {
			if len(group.Actions) != 2 {
				continue
			}
			a, b := group.Actions[0], group.Actions[1]
			if a.Symbol != pair[0] || b.Symbol != pair[1] {
				continue
			}
			if a.Type != "reduce" || b.Type != "reduce" || a.ChildCount != 1 || b.ChildCount != 1 ||
				a.Extra || b.Extra || a.Repetition || b.Repetition || a.Precedence != b.Precedence {
				return fmt.Errorf("certified unary conflict %v has unexpected actions", pair)
			}
			found = true
		}
		if !found {
			return fmt.Errorf("certified unary conflict %v not found", pair)
		}
		lang.ConflictPolicies = append(lang.ConflictPolicies, gotreesitter.ConflictPolicy{
			State:         gotreesitter.ConflictPolicyAnyState,
			Lookahead:     gotreesitter.ConflictPolicyAnyLookahead,
			Kind:          gotreesitter.ConflictPolicyDeclaredReduceReduceHighestSymbol,
			ReduceSymbols: []gotreesitter.Symbol{gotreesitter.Symbol(pair[0]), gotreesitter.Symbol(pair[1])},
		})
	}

	for _, policy := range profile.policies {
		if err := validateCertifiedRecoveredRepetitionReduce(grammar, policy); err != nil {
			return fmt.Errorf("certified conflict policy %q state=%d lookahead=%d: %w", grammar.Name, policy.state, policy.lookahead, err)
		}
		for i := range lang.ConflictPolicies {
			existing := &lang.ConflictPolicies[i]
			if int(existing.State) == policy.state && int(existing.Lookahead) == policy.lookahead {
				return fmt.Errorf("certified conflict policy %q state=%d lookahead=%d conflicts with existing kind %d", grammar.Name, policy.state, policy.lookahead, existing.Kind)
			}
		}
		lang.ConflictPolicies = append(lang.ConflictPolicies, gotreesitter.ConflictPolicy{
			State:         gotreesitter.StateID(policy.state),
			Lookahead:     gotreesitter.Symbol(policy.lookahead),
			Kind:          gotreesitter.ConflictPolicyRecoveredRepetitionReduce,
			ReduceSymbols: []gotreesitter.Symbol{gotreesitter.Symbol(policy.reduceSymbol)},
		})
	}

	sort.Slice(lang.ConflictPolicies, func(i, j int) bool {
		left, right := lang.ConflictPolicies[i], lang.ConflictPolicies[j]
		if left.State != right.State {
			return left.State < right.State
		}
		if left.Lookahead != right.Lookahead {
			return left.Lookahead < right.Lookahead
		}
		return left.Kind < right.Kind
	})
	return nil
}

func validateCertifiedRecoveredRepetitionReduce(grammar *ExtractedGrammar, policy certifiedRecoveredRepetitionReducePolicy) error {
	if grammar == nil {
		return fmt.Errorf("nil grammar")
	}
	if policy.lookahead < 0 || policy.lookahead >= len(grammar.SymbolNames) || grammar.SymbolNames[policy.lookahead] != policy.lookaheadName {
		got := "<out of range>"
		if policy.lookahead >= 0 && policy.lookahead < len(grammar.SymbolNames) {
			got = grammar.SymbolNames[policy.lookahead]
		}
		return fmt.Errorf("lookahead symbol name = %q, want %q", got, policy.lookaheadName)
	}
	if policy.reduceSymbol < 0 || policy.reduceSymbol >= len(grammar.SymbolNames) || grammar.SymbolNames[policy.reduceSymbol] != policy.reduceName {
		got := "<out of range>"
		if policy.reduceSymbol >= 0 && policy.reduceSymbol < len(grammar.SymbolNames) {
			got = grammar.SymbolNames[policy.reduceSymbol]
		}
		return fmt.Errorf("reduce symbol name = %q, want %q", got, policy.reduceName)
	}
	actions, ok := extractedActionsForStateLookahead(grammar, policy.state, policy.lookahead)
	if !ok {
		return fmt.Errorf("parse action row not found")
	}
	if len(actions) != 2 {
		return fmt.Errorf("action count = %d, want exactly 2", len(actions))
	}
	reduceFound := false
	shiftFound := false
	for _, action := range actions {
		switch action.Type {
		case "reduce":
			if reduceFound || action.Extra || action.Symbol != policy.reduceSymbol || action.ChildCount != policy.reduceChildren {
				return fmt.Errorf("unexpected reduce action %+v", action)
			}
			reduceFound = true
		case "shift":
			if shiftFound || action.Extra || !action.Repetition || action.State != policy.shiftState {
				return fmt.Errorf("unexpected shift action %+v", action)
			}
			shiftFound = true
		default:
			return fmt.Errorf("unexpected %s action", action.Type)
		}
	}
	if !reduceFound || !shiftFound {
		return fmt.Errorf("row must contain one reduce and one repetition shift")
	}
	return nil
}

func extractedActionsForStateLookahead(grammar *ExtractedGrammar, state, lookahead int) ([]ExtractedAction, bool) {
	if grammar == nil || state < 0 || state >= grammar.StateCount || lookahead < 0 || lookahead >= grammar.TokenCount {
		return nil, false
	}
	actionIndex := 0
	if state < grammar.LargeStateCount {
		if state >= len(grammar.ParseTable) {
			return nil, false
		}
		row := grammar.ParseTable[state]
		if lookahead < len(row) {
			actionIndex = int(row[lookahead])
		}
	} else {
		smallIndex := state - grammar.LargeStateCount
		if smallIndex < 0 || smallIndex >= len(grammar.SmallParseTableMap) {
			return nil, false
		}
		table := grammar.SmallParseTable
		pos := int(grammar.SmallParseTableMap[smallIndex])
		if pos < 0 || pos >= len(table) {
			return nil, false
		}
		groupCount := int(table[pos])
		pos++
		for i := 0; i < groupCount; i++ {
			if pos+1 >= len(table) {
				return nil, false
			}
			candidate := int(table[pos])
			symbolCount := int(table[pos+1])
			pos += 2
			if pos+symbolCount > len(table) {
				return nil, false
			}
			for _, symbol := range table[pos : pos+symbolCount] {
				if int(symbol) == lookahead {
					actionIndex = candidate
					break
				}
			}
			pos += symbolCount
			if actionIndex != 0 {
				break
			}
		}
	}
	if actionIndex == 0 {
		return nil, false
	}
	for i := range grammar.ParseActions {
		if grammar.ParseActions[i].Index == actionIndex {
			return grammar.ParseActions[i].Actions, true
		}
	}
	return nil, false
}
