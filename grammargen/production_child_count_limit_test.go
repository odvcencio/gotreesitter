package grammargen

import (
	"strings"
	"testing"
)

// buildChildCountTestGrammar returns a minimal NormalizedGrammar and LRTables
// with a single production whose right-hand side repeats one terminal
// rhsLen times. It is the smallest input that reaches assemble's
// child-count guard without exercising the rest of LR table construction.
func buildChildCountTestGrammar(rhsLen int) (*NormalizedGrammar, *LRTables) {
	rhs := make([]int, rhsLen)
	for i := range rhs {
		rhs[i] = 0 // every position references the same terminal
	}
	ng := &NormalizedGrammar{
		GrammarName: "child-count-test",
		Symbols: []SymbolInfo{
			{Name: "tok", Kind: SymbolTerminal},
			{Name: "big_rule", Kind: SymbolNonterminal},
		},
		Productions: []Production{
			{LHS: 1, RHS: rhs},
		},
	}
	tables := &LRTables{StateCount: 1}
	return ng, tables
}

// TestAssembleRejectsProductionOverChildCountLimit pins the fix for a
// production with more than 255 right-hand-side symbols: assemble must
// return a clear generation error instead of silently truncating
// ParseAction.ChildCount (a uint8) and shipping a broken reduce action.
func TestAssembleRejectsProductionOverChildCountLimit(t *testing.T) {
	ng, tables := buildChildCountTestGrammar(300)

	_, err := assemble(ng, tables, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("assemble() = nil error, want child-count overflow error for a 300-symbol production")
	}
	for _, want := range []string{"child-count-test", "big_rule", "300", "255"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

// TestAssembleAcceptsProductionAtChildCountLimit pins the boundary: a
// production with exactly 255 right-hand-side symbols — the largest value
// a uint8 ChildCount can hold — must still generate successfully.
func TestAssembleAcceptsProductionAtChildCountLimit(t *testing.T) {
	ng, tables := buildChildCountTestGrammar(255)

	lang, err := assemble(ng, tables, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("assemble() with a 255-symbol production returned an error: %v", err)
	}
	if lang == nil {
		t.Fatal("assemble() = nil Language, want a populated Language")
	}
}

// TestCheckProductionChildCountBoundary pins the exact overflow boundary of
// the guard that protects ParseAction.ChildCount, mirroring
// TestCheckUint16IndexBoundary for the analogous uint8 field.
func TestCheckProductionChildCountBoundary(t *testing.T) {
	cases := []struct {
		name    string
		rhsLen  int
		wantErr bool
	}{
		{"zero", 0, false},
		{"one", 1, false},
		{"limit-minus-one", maxProductionChildCount - 1, false},
		{"at-limit", maxProductionChildCount, false},           // 255 still fits in uint8
		{"just-over-limit", maxProductionChildCount + 1, true}, // 256 wraps to 0
		{"far-over-limit", 1000, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkProductionChildCount("scala", "big_rule", tc.rhsLen)
			if tc.wantErr && err == nil {
				t.Fatalf("checkProductionChildCount(%d) = nil, want overflow error", tc.rhsLen)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("checkProductionChildCount(%d) = %v, want nil", tc.rhsLen, err)
			}
		})
	}
}

// TestSerializeActionGroupKeyDistinguishesLargeShiftTargets pins the fix for
// the action-group dedup key: two shift actions whose target states differ
// only in their high 16 bits (1 and 65537) must produce different keys.
// Before the fix the key truncated a.state to 2 bytes, so both actions
// collided into a single ParseActionEntry and one of the two shift targets
// was silently discarded.
func TestSerializeActionGroupKeyDistinguishesLargeShiftTargets(t *testing.T) {
	ng := &NormalizedGrammar{}

	keyLow := serializeActionGroupKey([]lrAction{{kind: lrShift, state: 1}}, ng)
	keyHigh := serializeActionGroupKey([]lrAction{{kind: lrShift, state: 1 + 1<<16}}, ng)

	if keyLow == keyHigh {
		t.Fatalf("serializeActionGroupKey collided for shift targets 1 and %d: both produced %q",
			1+1<<16, keyLow)
	}
}

// TestSerializeActionGroupKeyStableForSmallShiftTargets guards against a
// regression in the other direction: two identical small shift targets must
// still share a key, and the key must not depend on map/slice iteration
// order artifacts introduced by the fix.
func TestSerializeActionGroupKeyStableForSmallShiftTargets(t *testing.T) {
	ng := &NormalizedGrammar{}

	a := serializeActionGroupKey([]lrAction{{kind: lrShift, state: 42}}, ng)
	b := serializeActionGroupKey([]lrAction{{kind: lrShift, state: 42}}, ng)
	if a != b {
		t.Fatalf("serializeActionGroupKey not stable for identical shift actions: %q != %q", a, b)
	}

	c := serializeActionGroupKey([]lrAction{{kind: lrShift, state: 43}}, ng)
	if a == c {
		t.Fatalf("serializeActionGroupKey collided for distinct small shift targets 42 and 43")
	}
}
