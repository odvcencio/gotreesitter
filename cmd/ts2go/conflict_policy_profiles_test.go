package main

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func certifiedPolicyTestFixture() ([]byte, *ExtractedGrammar, *gotreesitter.Language, certifiedConflictPolicyProfile) {
	source := []byte("synthetic certified parser.c")
	sum := sha256.Sum256(source)
	grammar := &ExtractedGrammar{
		Name:            "synthetic",
		StateCount:      3,
		LargeStateCount: 3,
		SymbolCount:     4,
		TokenCount:      2,
		SymbolNames:     []string{"end", "string_token", "other", "string_repeat1"},
		ParseTable: [][]uint16{
			make([]uint16, 4),
			make([]uint16, 4),
			{0, 5, 0, 0},
		},
		ParseActions: []ActionGroup{{
			Index: 5,
			Actions: []ExtractedAction{
				{Type: "reduce", Symbol: 3, ChildCount: 2},
				{Type: "shift", State: 7, Repetition: true},
			},
		}},
	}
	lang := &gotreesitter.Language{Name: grammar.Name}
	profile := certifiedConflictPolicyProfile{
		parserCSHA256: fmt.Sprintf("%x", sum[:]),
		stateCount:    3,
		symbolCount:   4,
		policies: []certifiedRecoveredRepetitionReducePolicy{{
			state:          2,
			lookahead:      1,
			lookaheadName:  "string_token",
			reduceSymbol:   3,
			reduceName:     "string_repeat1",
			reduceChildren: 2,
			shiftState:     7,
		}},
	}
	return source, grammar, lang, profile
}

func TestApplyCertifiedConflictPolicyProfile(t *testing.T) {
	source, grammar, lang, profile := certifiedPolicyTestFixture()
	if err := applyCertifiedConflictPolicyProfile(source, grammar, lang, profile); err != nil {
		t.Fatalf("applyCertifiedConflictPolicyProfile: %v", err)
	}
	if len(lang.ConflictPolicies) != 1 {
		t.Fatalf("policies = %d, want 1", len(lang.ConflictPolicies))
	}
	got := lang.ConflictPolicies[0]
	if got.State != 2 || got.Lookahead != 1 || got.Kind != gotreesitter.ConflictPolicyRecoveredRepetitionReduce || len(got.ReduceSymbols) != 1 || got.ReduceSymbols[0] != 3 {
		t.Fatalf("policy = %+v, want exact recovered repetition reduce row", got)
	}
}

func TestApplyCertifiedConflictPolicyProfileReadsSmallParseTable(t *testing.T) {
	source, grammar, lang, profile := certifiedPolicyTestFixture()
	grammar.LargeStateCount = 2
	grammar.ParseTable = grammar.ParseTable[:2]
	grammar.SmallParseTable = []uint16{1, 5, 1, 1}
	grammar.SmallParseTableMap = []uint32{0}

	if err := applyCertifiedConflictPolicyProfile(source, grammar, lang, profile); err != nil {
		t.Fatalf("applyCertifiedConflictPolicyProfile: %v", err)
	}
	if len(lang.ConflictPolicies) != 1 || lang.ConflictPolicies[0].State != 2 || lang.ConflictPolicies[0].Lookahead != 1 {
		t.Fatalf("small-table policy = %+v, want exact state 2/lookahead 1", lang.ConflictPolicies)
	}
}

func TestApplyCertifiedConflictPolicyProfileRejectsStaleSourceAndShape(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ExtractedGrammar, *gotreesitter.Language, *certifiedConflictPolicyProfile)
		source  []byte
		wantErr string
	}{
		{name: "source hash", source: []byte("different parser.c"), wantErr: "sha256"},
		{name: "state count", mutate: func(g *ExtractedGrammar, _ *gotreesitter.Language, _ *certifiedConflictPolicyProfile) { g.StateCount++ }, wantErr: "state count"},
		{name: "symbol count", mutate: func(g *ExtractedGrammar, _ *gotreesitter.Language, _ *certifiedConflictPolicyProfile) {
			g.SymbolCount++
		}, wantErr: "symbol count"},
		{name: "lookahead name", mutate: func(g *ExtractedGrammar, _ *gotreesitter.Language, _ *certifiedConflictPolicyProfile) {
			g.SymbolNames[1] = "other_token"
		}, wantErr: "lookahead symbol name"},
		{name: "reduce name", mutate: func(g *ExtractedGrammar, _ *gotreesitter.Language, _ *certifiedConflictPolicyProfile) {
			g.SymbolNames[3] = "other_repeat"
		}, wantErr: "reduce symbol name"},
		{name: "missing row", mutate: func(g *ExtractedGrammar, _ *gotreesitter.Language, _ *certifiedConflictPolicyProfile) {
			g.ParseTable[2][1] = 0
		}, wantErr: "row not found"},
		{name: "multiple reduce", mutate: func(g *ExtractedGrammar, _ *gotreesitter.Language, _ *certifiedConflictPolicyProfile) {
			g.ParseActions[0].Actions = append(g.ParseActions[0].Actions, ExtractedAction{Type: "reduce", Symbol: 3, ChildCount: 2})
		}, wantErr: "action count"},
		{name: "extra shift", mutate: func(g *ExtractedGrammar, _ *gotreesitter.Language, _ *certifiedConflictPolicyProfile) {
			g.ParseActions[0].Actions[1].Extra = true
		}, wantErr: "unexpected shift"},
		{name: "plain shift", mutate: func(g *ExtractedGrammar, _ *gotreesitter.Language, _ *certifiedConflictPolicyProfile) {
			g.ParseActions[0].Actions[1].Repetition = false
		}, wantErr: "unexpected shift"},
		{name: "wrong shift target", mutate: func(g *ExtractedGrammar, _ *gotreesitter.Language, _ *certifiedConflictPolicyProfile) {
			g.ParseActions[0].Actions[1].State = 8
		}, wantErr: "unexpected shift"},
		{name: "wrong reduce symbol", mutate: func(g *ExtractedGrammar, _ *gotreesitter.Language, _ *certifiedConflictPolicyProfile) {
			g.ParseActions[0].Actions[0].Symbol = 2
		}, wantErr: "unexpected reduce"},
		{name: "wrong reduce child count", mutate: func(g *ExtractedGrammar, _ *gotreesitter.Language, _ *certifiedConflictPolicyProfile) {
			g.ParseActions[0].Actions[0].ChildCount = 1
		}, wantErr: "unexpected reduce"},
		{name: "existing row policy", mutate: func(_ *ExtractedGrammar, lang *gotreesitter.Language, _ *certifiedConflictPolicyProfile) {
			lang.ConflictPolicies = []gotreesitter.ConflictPolicy{{State: 2, Lookahead: 1, Kind: gotreesitter.ConflictPolicyShift}}
		}, wantErr: "conflicts with existing"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source, grammar, lang, profile := certifiedPolicyTestFixture()
			if tc.source != nil {
				source = tc.source
			}
			if tc.mutate != nil {
				tc.mutate(grammar, lang, &profile)
			}
			err := applyCertifiedConflictPolicyProfile(source, grammar, lang, profile)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestApplyCertifiedConflictPolicyProfilesIgnoresUnknownLanguage(t *testing.T) {
	source, grammar, lang, _ := certifiedPolicyTestFixture()
	grammar.Name = "unprofiled"
	lang.Name = grammar.Name
	if err := applyCertifiedConflictPolicyProfiles(source, grammar, lang); err != nil {
		t.Fatalf("unprofiled language: %v", err)
	}
	if len(lang.ConflictPolicies) != 0 {
		t.Fatalf("unprofiled language received %d policies", len(lang.ConflictPolicies))
	}
}

func TestCaddyCertifiedConflictPolicyProfileIsPinned(t *testing.T) {
	profile, ok := certifiedConflictPolicyProfiles["caddy"]
	if !ok {
		t.Fatal("caddy certified profile missing")
	}
	if profile.parserCSHA256 != "bf289eeb9aca008f0e981f66c6bcf5a32faa4434f4373dd4d5d8d6fcec7bff6e" || profile.stateCount != 192 || profile.symbolCount != 70 || len(profile.policies) != 1 {
		t.Fatalf("caddy profile = %+v", profile)
	}
	policy := profile.policies[0]
	if policy.state != 86 || policy.lookahead != 25 || policy.reduceSymbol != 68 || policy.reduceChildren != 2 || policy.shiftState != 126 {
		t.Fatalf("caddy recovered policy = %+v", policy)
	}
}

func TestCertifiedUnaryReducePairRejectsChangedActionShape(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mutate    func(*ExtractedGrammar)
		wantError bool
	}{
		{name: "valid"},
		{name: "arity", mutate: func(g *ExtractedGrammar) { g.ParseActions[0].Actions[1].ChildCount = 2 }, wantError: true},
		{name: "precedence", mutate: func(g *ExtractedGrammar) { g.ParseActions[0].Actions[1].Precedence = 1 }, wantError: true},
		{name: "missing", mutate: func(g *ExtractedGrammar) { g.ParseActions[0].Actions = nil }, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source, grammar, lang, profile := certifiedPolicyTestFixture()
			profile.policies = nil
			profile.unaryReducePairs = [][2]int{{2, 3}}
			grammar.ParseActions[0].Actions = []ExtractedAction{{Type: "reduce", Symbol: 2, ChildCount: 1}, {Type: "reduce", Symbol: 3, ChildCount: 1}}
			if tc.mutate != nil {
				tc.mutate(grammar)
			}
			err := applyCertifiedConflictPolicyProfile(source, grammar, lang, profile)
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, wantError = %t", err, tc.wantError)
			}
			if err == nil && (len(lang.ConflictPolicies) != 1 || lang.ConflictPolicies[0].Kind != gotreesitter.ConflictPolicyDeclaredReduceReduceHighestSymbol) {
				t.Fatalf("policies = %+v", lang.ConflictPolicies)
			}
		})
	}
}
