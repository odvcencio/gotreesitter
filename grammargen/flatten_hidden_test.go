package grammargen

import (
	"reflect"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestFlattenHiddenMixedPassthroughPreservesReductionPrecedence(t *testing.T) {
	tests := []struct {
		name string
		wrap func(*Rule) *Rule
	}{
		{"static", func(r *Rule) *Rule { return Prec(7, r) }},
		{"explicit_zero", func(r *Rule) *Rule { return Prec(0, r) }},
		{"negative", func(r *Rule) *Rule { return Prec(-1, r) }},
		{"left", func(r *Rule) *Rule { return PrecLeft(0, r) }},
		{"right", func(r *Rule) *Rule { return PrecRight(0, r) }},
		{"dynamic", func(r *Rule) *Rule { return PrecDynamic(3, r) }},
		{"dynamic_zero", func(r *Rule) *Rule { return PrecDynamic(0, r) }},
		{"field", func(r *Rule) *Rule { return Field("value", PrecLeft(7, r)) }},
		{"alias", func(r *Rule) *Rule { return Alias(PrecLeft(7, r), "value", true) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, outer := range []bool{false, true} {
				g := NewGrammar("preserve_reduction_precedence")
				g.Define("document", Seq(Sym("_value"), Str("!")))
				compound := Seq(Str("("), Sym("number"), Str(")"))
				rule := Choice(tt.wrap(Sym("number")), compound)
				if outer {
					rule = tt.wrap(Choice(Sym("number"), compound))
				}
				g.Define("_value", rule)
				g.Define("number", Pat(`[0-9]+`))
				want := cloneRule(g.Rules["_value"])
				wantDocument := cloneRule(g.Rules["document"])

				got := flattenHiddenChoiceAlts(g, nil)
				if !reflect.DeepEqual(got.Rules["_value"], want) {
					t.Errorf("outer=%t: changed the precedence-bearing reduction", outer)
				}
				if !reflect.DeepEqual(got.Rules["document"], wantDocument) {
					t.Errorf("outer=%t: bypassed the precedence-bearing reduction", outer)
				}
			}
		})
	}
}

func TestFlattenHiddenPrecedenceRetainsChildReductions(t *testing.T) {
	g := NewGrammar("preserve_child_reductions")
	g.Define("document", Sym("_wrapper"))
	g.Define("_wrapper", PrecLeft(7, Choice(Sym("_retained"), Sym("_replaced"), Seq(Str("("), Str(")")))))
	g.Define("_retained", PrecRight(0, Choice(Sym("a"), Sym("b"))))
	g.Define("_replaced", Choice(Sym("c"), Seq(Str("["), Sym("c"), Str("]"))))
	for _, name := range []string{"a", "b", "c"} {
		g.Define(name, Str(name))
	}
	got := flattenHiddenChoiceAlts(g, nil)
	refs := map[string]bool{}
	for _, alt := range getTopLevelChoiceAlts(got.Rules["_wrapper"]) {
		if name, ok := directSymbolRefName(alt); ok {
			refs[name] = true
		}
	}
	want := map[string]bool{"_retained": true, "_replaced": true}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("wrapper references = %v, want %v", refs, want)
	}
}

func TestFlattenHiddenPrecedencePreservesChildClosureBeforeSkip(t *testing.T) {
	for _, name := range []string{"wide_choice", "configured_preserve", "cyclic_child", "generated_bridge"} {
		t.Run(name, func(t *testing.T) {
			g := NewGrammar("preserve_child_closure")
			g.Define("document", Sym("_root"))
			alts := []*Rule{PrecLeft(7, Sym("_child")), Seq(Str("("), Str(")"))}
			if name == "wide_choice" {
				for i := 1; i <= 8; i++ {
					alts = append(alts, Str(strings.Repeat("x", i)))
				}
			}
			g.Define("_root", Choice(alts...))
			g.Define("_child", Choice(Sym("a"), Seq(Str("["), Str("]"))))
			g.Define("a", Str("a"))
			if name == "configured_preserve" {
				g.PreserveHiddenChoicePassthrough = []string{"_root"}
			}
			if name == "cyclic_child" {
				g.Define("_child", Choice(Sym("_cycle"), Seq(Str("["), Str("]"))))
				g.Define("_cycle", Choice(Sym("_child"), Sym("a")))
			}
			var generated map[string]bool
			if name == "generated_bridge" {
				g.Define("_root", Choice(PrecLeft(7, Sym("repeat_aux")), Seq(Str("("), Str(")"))))
				g.Define("repeat_aux", Sym("_child"))
				generated = map[string]bool{"repeat_aux": true}
			}
			wantRoot, wantChild := cloneRule(g.Rules["_root"]), cloneRule(g.Rules["_child"])
			got := flattenHiddenChoiceAlts(g, generated)
			if !reflect.DeepEqual(got.Rules["_root"], wantRoot) || !reflect.DeepEqual(got.Rules["_child"], wantChild) {
				t.Fatal("flattening bypassed a protected child reduction")
			}
		})
	}
}

func TestHiddenMixedPassthroughPrecedenceMatchesC(t *testing.T) {
	// Tree-sitter v0.24.7 reduces _app before '&'. Both competing actions
	// have precedence 7 and left associativity. The result has no compound.
	g := NewGrammar("hidden_passthrough_precedence")
	g.Define("source_file", Sym("_apps"))
	g.Define("_apps", PrecLeft(0, Choice(
		Seq(Sym("_app"), Repeat(Seq(Str(","), Sym("_app")))),
		Seq(Sym("_app"), Repeat(Seq(Str("&"), Sym("_app")))),
	)))
	g.Define("_app", PrecLeft(7, Choice(
		Sym("_atom"), Sym("compound"), Seq(Sym("_atom"), Str("("), Str(")")),
	)))
	g.Define("_atom", Sym("identifier"))
	g.Define("compound", PrecLeft(7, Seq(Sym("_atom"), Repeat1(Seq(Str("&"), Sym("_atom"))))))
	g.Define("identifier", Pat(`[A-Z]`))
	ng, err := Normalize(g)
	if err != nil {
		t.Fatal(err)
	}
	foundReduction := false
	for _, prod := range ng.Productions {
		lhs := ng.Symbols[prod.LHS].Name
		if lhs == "_app" && len(prod.RHS) == 1 && ng.Symbols[prod.RHS[0]].Name == "_atom" {
			foundReduction = true
			if prod.Prec != 7 || prod.Assoc != AssocLeft || !prod.HasExplicitPrec {
				t.Errorf("_app reduction lost its precedence: %+v", prod)
			}
		}
		if lhs == "_apps" || strings.HasPrefix(lhs, "_apps_repeat") {
			for _, id := range prod.RHS {
				if name := ng.Symbols[id].Name; name == "_atom" || name == "compound" {
					t.Errorf("%s bypasses the _app reduction with %s", lhs, name)
				}
			}
		}
	}
	if !foundReduction {
		t.Fatal("normalization removed _app -> _atom")
	}
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"A", "A&B", "A,B", "A&B&C"} {
		t.Run(source, func(t *testing.T) {
			parser := gotreesitter.NewParser(lang)
			parser.SetAdmissionCandidateRoute(false)
			tree, err := parser.Parse([]byte(source))
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if root.Type(lang) != "source_file" || root.HasError() || root.StartByte() != 0 || root.EndByte() != uint32(len(source)) ||
				root.StartPoint() != (gotreesitter.Point{}) || root.EndPoint() != (gotreesitter.Point{Column: uint32(len(source))}) {
				t.Fatalf("unexpected root: %s, range [%d,%d)", root.SExpr(lang), root.StartByte(), root.EndByte())
			}
			if root.ChildCount() != len(source) {
				t.Fatalf("child count = %d, want %d: %s", root.ChildCount(), len(source), root.SExpr(lang))
			}
			for i := 0; i < len(source); i++ {
				child := root.Child(i)
				wantType := "identifier"
				if i%2 != 0 {
					wantType = source[i : i+1]
				}
				if child.Type(lang) != wantType || child.IsNamed() != (i%2 == 0) ||
					child.StartByte() != uint32(i) || child.EndByte() != uint32(i+1) ||
					child.StartPoint() != (gotreesitter.Point{Column: uint32(i)}) ||
					child.EndPoint() != (gotreesitter.Point{Column: uint32(i + 1)}) ||
					child.ChildCount() != 0 || child.IsMissing() || child.HasError() {
					t.Errorf("child %d = %s [%d,%d), want %s [%d,%d)", i, child.SExpr(lang), child.StartByte(), child.EndByte(), wantType, i, i+1)
				}
			}
		})
	}
}

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

func TestFlattenHiddenPassthroughPreservesAliasReferencedRule(t *testing.T) {
	g := &Grammar{
		Name: "test_flatten_alias_preserve",
		Rules: map[string]*Rule{
			"document": Seq(Alias(Sym("_value"), "wrapped", true), Sym("member")),
			"member":   Seq(Sym("_value")),
			"_value":   Choice(Sym("item"), Seq(Str("("), Sym("item"), Str(")"))),
			"item":     Pat(`[a-z]+`),
		},
		RuleOrder: []string{"document", "member", "_value", "item"},
	}

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	symNameToID := make(map[string]int)
	for i, info := range ng.Symbols {
		symNameToID[info.Name] = i
	}

	valueID := symNameToID["_value"]
	itemID := symNameToID["item"]
	memberID := symNameToID["member"]

	hasValueItem := false
	hasMemberItem := false
	for _, p := range ng.Productions {
		switch p.LHS {
		case valueID:
			if len(p.RHS) == 1 && p.RHS[0] == itemID {
				hasValueItem = true
			}
		case memberID:
			for _, sym := range p.RHS {
				if sym == itemID {
					hasMemberItem = true
				}
			}
		}
	}

	if !hasValueItem {
		t.Fatal("_value pass-through item production should be retained for alias references")
	}
	if !hasMemberItem {
		t.Fatal("non-alias _value references should still receive inlined pass-through alternatives")
	}
}

func TestFlattenGeneratedHiddenPassthroughPreservesAliasReferencedRule(t *testing.T) {
	g := NewGrammar("test_flatten_generated_alias_preserve")
	g.FlattenGeneratedRepeatAux = true
	g.Define("document", Seq(Alias(Sym("list_repeat1"), "wrapped", true), Sym("member")))
	g.Define("member", Seq(Sym("list_repeat1")))
	g.Define("list_repeat1", Choice(Sym("item"), Seq(Sym("item"), Sym("item"))))
	g.Define("item", Pat(`[a-z]+`))

	flattened := flattenHiddenChoiceAlts(g, map[string]bool{"list_repeat1": true})

	aux := flattened.Rules["list_repeat1"]
	if !choiceHasSymbolAlt(aux, "item") {
		t.Fatal("generated hidden aux pass-through item production should be retained for alias references")
	}

	document := flattened.Rules["document"]
	if alias := document.Children[0]; alias.Kind != RuleAlias || alias.Children[0].Kind != RuleSymbol || alias.Children[0].Value != "list_repeat1" {
		t.Fatalf("alias reference should remain a direct aux symbol, got %#v", document.Children[0])
	}

	member := flattened.Rules["member"]
	if member.Kind != RuleSeq || len(member.Children) != 1 || !choiceHasSymbolAlt(member.Children[0], "item") {
		t.Fatalf("ordinary generated hidden aux reference should inline pass-through item alternative, got %#v", member)
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

func TestFlattenHiddenPassthroughTransitiveChoice(t *testing.T) {
	g := &Grammar{
		Name: "test_flatten_hidden_passthrough_transitive",
		Rules: map[string]*Rule{
			"document": Seq(Str("FROM"), Sym("_outer"), Str("BY")),
			"_outer":   Choice(Sym("_inner"), Seq(Str("LEN"), Sym("ident"))),
			"_inner":   Choice(Sym("number"), Seq(Str("ALL"), Sym("number"))),
			"ident":    Pat(`[a-z]+`),
			"number":   Pat(`[0-9]+`),
		},
		RuleOrder: []string{"document", "_outer", "_inner", "ident", "number"},
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
	outerID := symNameToID["_outer"]
	innerID := symNameToID["_inner"]
	numberID := symNameToID["number"]

	hasOuter := false
	hasInner := false
	hasNumber := false
	for _, p := range ng.Productions {
		if p.LHS != documentID {
			continue
		}
		for _, sym := range p.RHS {
			if sym == outerID {
				hasOuter = true
			}
			if sym == innerID {
				hasInner = true
			}
			if sym == numberID {
				hasNumber = true
			}
		}
	}

	if !hasOuter {
		t.Fatal("document should retain original _outer reference for compound alternatives")
	}
	if !hasInner {
		t.Fatal("document should retain direct _inner reference from first-level passthrough flattening")
	}
	if !hasNumber {
		t.Fatal("document missing transitive passthrough reference to number")
	}
}

func TestFlattenHiddenPassthroughPreservesContinuationSensitiveWrappers(t *testing.T) {
	g := &Grammar{
		Name: "test_flatten_hidden_continuation_sensitive",
		Rules: map[string]*Rule{
			"document": Seq(Sym("_outer"), Sym("tail")),
			"_outer":   Choice(Sym("_inner"), Seq(Sym("_inner"), Sym("tail"))),
			"_inner":   Choice(Sym("item"), Seq(Str("all"), Sym("item"))),
			"item":     Pat(`[a-z]+`),
			"tail":     Pat(`[0-9]+`),
		},
		RuleOrder: []string{"document", "_outer", "_inner", "item", "tail"},
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
	outerID := symNameToID["_outer"]
	innerID := symNameToID["_inner"]
	itemID := symNameToID["item"]

	hasOuterInner := false
	hasDocumentInner := false
	hasDocumentItem := false
	for _, p := range ng.Productions {
		switch p.LHS {
		case outerID:
			if len(p.RHS) == 1 && p.RHS[0] == innerID {
				hasOuterInner = true
			}
		case documentID:
			for _, sym := range p.RHS {
				if sym == innerID {
					hasDocumentInner = true
				}
				if sym == itemID {
					hasDocumentItem = true
				}
			}
		}
	}

	if !hasOuterInner {
		t.Fatal("_outer pass-through _inner production should be retained when it also starts a compound continuation")
	}
	if !hasDocumentInner || !hasDocumentItem {
		t.Fatalf("document missing inlined passthrough refs: inner=%v item=%v", hasDocumentInner, hasDocumentItem)
	}
}

func choiceHasSymbolAlt(r *Rule, name string) bool {
	if r == nil || r.Kind != RuleChoice {
		return false
	}
	for _, child := range r.Children {
		if child.Kind == RuleSymbol && child.Value == name {
			return true
		}
	}
	return false
}
