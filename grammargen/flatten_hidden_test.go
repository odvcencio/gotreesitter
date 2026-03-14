package grammargen

import (
	"testing"
)

// TestFlattenHiddenPassthrough verifies that cc=1 productions of hidden
// nonterminals are removed and distributed into parent productions.
func TestFlattenHiddenPassthrough(t *testing.T) {
	// Grammar: _value is a hidden rule with both cc=1 and cc>1 alternatives.
	// member references _value. After flattening, _value should only have cc>1.
	g := &Grammar{
		Name: "test_flatten",
		Rules: map[string]*Rule{
			"document": Sym("member"),
			"member":   Seq(Sym("key"), Str(":"), Sym("_value")),
			"_value":   Choice(Sym("string"), Sym("number"), Seq(Str("{"), Sym("member"), Str("}"))),
			"key":      Pat(`[a-z]+`),
			"string":   Pat(`"[^"]*"`),
			"number":   Pat(`[0-9]+`),
		},
		RuleOrder: []string{
			"document", "member", "_value", "key", "string", "number",
		},
	}

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	symNameToID := make(map[string]int)
	for i, info := range ng.Symbols {
		symNameToID[info.Name] = i
	}

	// Check _value's productions: should NOT have any cc=1.
	valueID := symNameToID["_value"]
	memberID := symNameToID["member"]
	stringID := symNameToID["string"]
	numberID := symNameToID["number"]

	var valueCCs []int
	for _, p := range ng.Productions {
		if p.LHS == valueID {
			valueCCs = append(valueCCs, len(p.RHS))
		}
	}

	for _, cc := range valueCCs {
		if cc == 1 {
			t.Errorf("_value still has cc=1 production after flattening (ccs=%v)", valueCCs)
			break
		}
	}
	if len(valueCCs) == 0 {
		t.Error("_value has no productions at all")
	}
	t.Logf("_value ccs: %v", valueCCs)

	// Check member's productions: should have direct refs to string and number
	// (from inlined _value cc=1 alts) in addition to _value ref (for cc>1 alts).
	hasString := false
	hasNumber := false
	hasValue := false
	for _, p := range ng.Productions {
		if p.LHS == memberID {
			for _, sym := range p.RHS {
				if sym == stringID {
					hasString = true
				}
				if sym == numberID {
					hasNumber = true
				}
				if sym == valueID {
					hasValue = true
				}
			}
		}
	}

	if !hasString {
		t.Error("member does not have direct reference to 'string' after flattening")
	}
	if !hasNumber {
		t.Error("member does not have direct reference to 'number' after flattening")
	}
	if !hasValue {
		t.Error("member should still reference '_value' for compound alternatives")
	}

	// Dump all productions for diagnostics.
	for _, p := range ng.Productions {
		rhsNames := make([]string, len(p.RHS))
		for j, id := range p.RHS {
			if id < len(ng.Symbols) {
				rhsNames[j] = ng.Symbols[id].Name
			}
		}
		lhsName := "?"
		if p.LHS < len(ng.Symbols) {
			lhsName = ng.Symbols[p.LHS].Name
		}
		t.Logf("  prod[%d]: %s → %v (cc=%d)", p.ProductionID, lhsName, rhsNames, len(p.RHS))
	}
}

func TestFlattenHiddenTopLevelRepeat1Passthrough(t *testing.T) {
	g := &Grammar{
		Name: "test_flatten_hidden_repeat1",
		Rules: map[string]*Rule{
			"document": Seq(Sym("_items")),
			"_items":   Repeat1(Sym("item")),
			"item":     Pat(`[a-z]+`),
		},
		RuleOrder: []string{"document", "_items", "item"},
	}

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	symNameToID := make(map[string]int)
	for i, info := range ng.Symbols {
		symNameToID[info.Name] = i
	}

	itemsID := symNameToID["_items"]
	documentID := symNameToID["document"]
	itemID := symNameToID["item"]

	var itemCCs []int
	for _, p := range ng.Productions {
		if p.LHS == itemsID {
			itemCCs = append(itemCCs, len(p.RHS))
		}
	}
	if len(itemCCs) == 0 {
		t.Fatal("_items has no productions")
	}
	for _, cc := range itemCCs {
		if cc == 1 {
			t.Fatalf("_items still has cc=1 production after repeat flattening: %v", itemCCs)
		}
	}

	hasDirectItem := false
	hasItemsRef := false
	for _, p := range ng.Productions {
		if p.LHS != documentID {
			continue
		}
		for _, sym := range p.RHS {
			if sym == itemID {
				hasDirectItem = true
			}
			if sym == itemsID {
				hasItemsRef = true
			}
		}
	}
	if !hasDirectItem {
		t.Fatal("document does not have direct reference to item after repeat flattening")
	}
	if !hasItemsRef {
		t.Fatal("document should still reference _items for recursive alternatives")
	}
}

func TestInlineHiddenAllPassthroughChoice(t *testing.T) {
	g := &Grammar{
		Name: "test_inline_hidden_passthrough_only",
		Rules: map[string]*Rule{
			"document": Seq(Sym("_value")),
			"_value":   Choice(Sym("string"), Sym("number")),
			"string":   Pat(`"[^\"]*"`),
			"number":   Pat(`[0-9]+`),
		},
		RuleOrder: []string{"document", "_value", "string", "number"},
	}

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	symNameToID := make(map[string]int)
	for i, info := range ng.Symbols {
		symNameToID[info.Name] = i
	}

	documentID := symNameToID["document"]
	stringID := symNameToID["string"]
	numberID := symNameToID["number"]
	valueID := symNameToID["_value"]

	hasString := false
	hasNumber := false
	hasValue := false
	for _, p := range ng.Productions {
		if p.LHS != documentID {
			continue
		}
		for _, sym := range p.RHS {
			if sym == stringID {
				hasString = true
			}
			if sym == numberID {
				hasNumber = true
			}
			if sym == valueID {
				hasValue = true
			}
		}
	}
	if !hasString || !hasNumber {
		t.Fatalf("document missing direct passthrough refs after flattening: string=%v number=%v", hasString, hasNumber)
	}
	if !hasValue {
		t.Fatal("document should retain original hidden reference alongside direct passthrough refs")
	}
}
