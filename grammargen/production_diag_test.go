package grammargen

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// TestProductionChildCountDiag imports grammars known to have childCount
// mismatches and dumps production RHS lengths for the affected symbols.
// This isolates whether the issue is in production generation (Normalize)
// or downstream (LR tables / assembly).
//
// Run with: GTS_GRAMMARGEN_REAL_CORPUS_ENABLE=1 go test -run TestProductionChildCountDiag -v
func TestProductionChildCountDiag(t *testing.T) {
	if os.Getenv("GTS_GRAMMARGEN_REAL_CORPUS_ENABLE") != "1" {
		t.Skip("set GTS_GRAMMARGEN_REAL_CORPUS_ENABLE=1")
	}

	corpusRoot := os.Getenv("GTS_GRAMMARGEN_REAL_CORPUS_ROOT")
	if corpusRoot == "" {
		corpusRoot = "/tmp/grammar_parity"
	}

	// Grammars and symbols flagged by logReduceActionDiff as having
	// gen-cc ≠ ref-cc (missing shorter variants).
	type diagCase struct {
		grammar string // grammar dir name
		symbols []string // symbols to check
	}

	cases := []diagCase{
		{"ron", []string{"string", "string_content"}},
		{"jsdoc", []string{"document", "description"}},
		{"dockerfile", []string{"json_string"}},
		{"regex", []string{"character_class"}},
		{"lua", []string{"arguments"}},
		{"sql", []string{"create_function_parameters"}},
		{"go", []string{"interpreted_string_literal"}},
		{"toml", []string{"string", "basic_string"}},
		{"nix", []string{"string_expression"}},
		{"css", []string{"string_value"}},
	}

	for _, tc := range cases {
		t.Run(tc.grammar, func(t *testing.T) {
			grammarDir := filepath.Join(corpusRoot, "tree-sitter-"+tc.grammar)
			if _, err := os.Stat(grammarDir); err != nil {
				// Try without prefix
				grammarDir = filepath.Join(corpusRoot, tc.grammar)
				if _, err := os.Stat(grammarDir); err != nil {
					t.Skipf("grammar dir not found: %s", tc.grammar)
					return
				}
			}

			// Find grammar.json / src/grammar.json
			jsonPath := filepath.Join(grammarDir, "src", "grammar.json")
			if _, err := os.Stat(jsonPath); err != nil {
				jsonPath = filepath.Join(grammarDir, "grammar.json")
				if _, err := os.Stat(jsonPath); err != nil {
					t.Skipf("grammar.json not found for %s", tc.grammar)
					return
				}
			}

			data, err := os.ReadFile(jsonPath)
			if err != nil {
				t.Fatalf("read grammar.json: %v", err)
			}

			g, err := ImportGrammarJSON(data)
			if err != nil {
				t.Fatalf("import: %v", err)
			}

			ng, err := Normalize(g)
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}

			// Build symbol name → ID map from ng.Symbols.
			symNameToID := make(map[string]int)
			for i, info := range ng.Symbols {
				symNameToID[info.Name] = i
			}

			// For each target symbol, find all productions.
			for _, symName := range tc.symbols {
				symID, ok := symNameToID[symName]
				if !ok {
					t.Logf("  symbol %q not found in symbol table", symName)
					continue
				}

				var prods []Production
				for _, p := range ng.Productions {
					if p.LHS == symID {
						prods = append(prods, p)
					}
				}

				t.Logf("  symbol %q (id=%d): %d productions", symName, symID, len(prods))
				for i, p := range prods {
					rhsNames := make([]string, len(p.RHS))
					for j, rid := range p.RHS {
						if rid >= 0 && rid < len(ng.Symbols) {
							rhsNames[j] = ng.Symbols[rid].Name
						} else {
							rhsNames[j] = fmt.Sprintf("?%d", rid)
						}
					}
					t.Logf("    prod[%d]: cc=%d rhs=[%s]", i, len(p.RHS), strings.Join(rhsNames, ", "))
				}

				// Also collect the unique RHS lengths.
				ccSet := make(map[int]bool)
				for _, p := range prods {
					ccSet[len(p.RHS)] = true
				}
				ccs := make([]int, 0, len(ccSet))
				for cc := range ccSet {
					ccs = append(ccs, cc)
				}
				sort.Ints(ccs)
				t.Logf("    unique cc values: %v", ccs)
			}

			// Additionally: dump the rule tree for the target symbols
			// to see the pre-normalization structure.
			for _, symName := range tc.symbols {
				rule, exists := g.Rules[symName]
				if !exists {
					continue
				}
				t.Logf("  rule-tree[%s]: %s", symName, ruleTreeString(rule, 0))
			}
		})
	}
}

// ruleTreeString produces a compact string representation of a rule tree.
func ruleTreeString(r *Rule, depth int) string {
	if r == nil {
		return "nil"
	}
	if depth > 6 {
		return "..."
	}

	switch r.Kind {
	case RuleString:
		return fmt.Sprintf("str(%q)", r.Value)
	case RulePattern:
		return fmt.Sprintf("pat(%q)", r.Value)
	case RuleSymbol:
		return fmt.Sprintf("sym(%s)", r.Value)
	case RuleBlank:
		return "blank"
	case RuleSeq:
		parts := make([]string, len(r.Children))
		for i, c := range r.Children {
			parts[i] = ruleTreeString(c, depth+1)
		}
		return fmt.Sprintf("seq(%s)", strings.Join(parts, ", "))
	case RuleChoice:
		parts := make([]string, len(r.Children))
		for i, c := range r.Children {
			parts[i] = ruleTreeString(c, depth+1)
		}
		return fmt.Sprintf("choice(%s)", strings.Join(parts, ", "))
	case RuleOptional:
		return fmt.Sprintf("opt(%s)", ruleTreeString(r.Children[0], depth+1))
	case RuleRepeat:
		return fmt.Sprintf("repeat(%s)", ruleTreeString(r.Children[0], depth+1))
	case RuleRepeat1:
		return fmt.Sprintf("repeat1(%s)", ruleTreeString(r.Children[0], depth+1))
	case RuleToken:
		return fmt.Sprintf("token(%s)", ruleTreeString(r.Children[0], depth+1))
	case RuleImmToken:
		return fmt.Sprintf("imm(%s)", ruleTreeString(r.Children[0], depth+1))
	case RuleField:
		return fmt.Sprintf("field(%s, %s)", r.Value, ruleTreeString(r.Children[0], depth+1))
	case RuleAlias:
		return fmt.Sprintf("alias(%s, %s)", r.Value, ruleTreeString(r.Children[0], depth+1))
	case RulePrec:
		return fmt.Sprintf("prec(%d, %s)", r.Prec, ruleTreeString(r.Children[0], depth+1))
	case RulePrecLeft:
		return fmt.Sprintf("prec.left(%d, %s)", r.Prec, ruleTreeString(r.Children[0], depth+1))
	case RulePrecRight:
		return fmt.Sprintf("prec.right(%d, %s)", r.Prec, ruleTreeString(r.Children[0], depth+1))
	case RulePrecDynamic:
		return fmt.Sprintf("prec.dyn(%d, %s)", r.Prec, ruleTreeString(r.Children[0], depth+1))
	default:
		return fmt.Sprintf("kind%d", r.Kind)
	}
}

// TestProductionPipelineTrace traces a synthetic grammar with optional()
// through the full pipeline to verify both alternatives are preserved.
func TestProductionPipelineTrace(t *testing.T) {
	// Synthetic grammar: string → '"' content? '"'
	// Should produce two productions: cc=3 (with content) and cc=2 (without).
	g := &Grammar{
		Name: "test_optional",
		Rules: map[string]*Rule{
			"document": Seq(Sym("string")),
			"string": Seq(
				Str("\""),
				Optional(Sym("string_content")),
				Str("\""),
			),
			"string_content": Pat(`[^"]+`),
		},
		RuleOrder: []string{"document", "string", "string_content"},
	}

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	// Find the "string" symbol.
	stringID := -1
	for i, info := range ng.Symbols {
		if info.Name == "string" {
			stringID = i
			break
		}
	}
	if stringID < 0 {
		t.Fatal("string symbol not found")
	}

	// Collect productions for "string".
	var prods []Production
	for _, p := range ng.Productions {
		if p.LHS == stringID {
			prods = append(prods, p)
		}
	}

	t.Logf("string productions: %d", len(prods))
	ccSet := make(map[int]bool)
	for i, p := range prods {
		rhsNames := make([]string, len(p.RHS))
		for j, rid := range p.RHS {
			if rid >= 0 && rid < len(ng.Symbols) {
				rhsNames[j] = ng.Symbols[rid].Name
			} else {
				rhsNames[j] = fmt.Sprintf("?%d", rid)
			}
		}
		t.Logf("  prod[%d]: cc=%d rhs=[%s]", i, len(p.RHS), strings.Join(rhsNames, ", "))
		ccSet[len(p.RHS)] = true
	}

	// Verify both cc=2 and cc=3 exist.
	if !ccSet[3] {
		t.Error("MISSING: production with cc=3 (string → '\"' content '\"')")
	}
	if !ccSet[2] {
		t.Error("MISSING: production with cc=2 (string → '\"' '\"')")
	}

	// Now run the full pipeline and check parse actions.
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}

	// Find reduce actions for "string" in the parse actions.
	stringSymIdx := -1
	for i, name := range lang.SymbolNames {
		if name == "string" {
			stringSymIdx = i
			break
		}
	}
	if stringSymIdx < 0 {
		t.Fatal("string symbol not in Language.SymbolNames")
	}

	parseCC := make(map[uint8]bool)
	for _, pa := range lang.ParseActions {
		for _, a := range pa.Actions {
			if a.Type == gotreesitter.ParseActionReduce && int(a.Symbol) == stringSymIdx {
				parseCC[a.ChildCount] = true
			}
		}
	}
	t.Logf("parse action cc values for string: %v", parseCC)
	if !parseCC[3] {
		t.Error("MISSING: parse action cc=3 for string")
	}
	if !parseCC[2] {
		t.Error("MISSING: parse action cc=2 for string")
	}
}

// TestProductionWithField tests optional inside a field wrapper.
func TestProductionWithField(t *testing.T) {
	g := &Grammar{
		Name: "test_field_opt",
		Rules: map[string]*Rule{
			"document": Seq(Sym("pair")),
			"pair": Seq(
				Sym("key"),
				Str(":"),
				Field("value", Optional(Sym("value"))),
			),
			"key":   Pat(`[a-z]+`),
			"value": Pat(`[0-9]+`),
		},
		RuleOrder: []string{"document", "pair", "key", "value"},
	}

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	pairID := -1
	for i, info := range ng.Symbols {
		if info.Name == "pair" {
			pairID = i
			break
		}
	}
	if pairID < 0 {
		t.Fatal("pair symbol not found")
	}

	var prods []Production
	for _, p := range ng.Productions {
		if p.LHS == pairID {
			prods = append(prods, p)
		}
	}

	t.Logf("pair productions: %d", len(prods))
	ccSet := make(map[int]bool)
	for i, p := range prods {
		rhsNames := make([]string, len(p.RHS))
		for j, rid := range p.RHS {
			if rid >= 0 && rid < len(ng.Symbols) {
				rhsNames[j] = ng.Symbols[rid].Name
			} else {
				rhsNames[j] = fmt.Sprintf("?%d", rid)
			}
		}
		t.Logf("  prod[%d]: cc=%d rhs=[%s] fields=%v", i, len(p.RHS), strings.Join(rhsNames, ", "), p.Fields)
		ccSet[len(p.RHS)] = true
	}

	if !ccSet[3] {
		t.Error("MISSING: cc=3 (pair → key ':' value)")
	}
	if !ccSet[2] {
		t.Error("MISSING: cc=2 (pair → key ':')")
	}
}

// TestProductionWithPrec tests optional inside precedence wrappers.
func TestProductionWithPrec(t *testing.T) {
	g := &Grammar{
		Name: "test_prec_opt",
		Rules: map[string]*Rule{
			"document": Seq(Sym("expr")),
			"expr": Choice(
				PrecLeft(1, Seq(Sym("expr"), Str("+"), Sym("expr"))),
				Sym("number"),
				Seq(Str("("), Optional(Sym("expr")), Str(")")),
			),
			"number": Pat(`[0-9]+`),
		},
		RuleOrder: []string{"document", "expr", "number"},
	}

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	exprID := -1
	for i, info := range ng.Symbols {
		if info.Name == "expr" {
			exprID = i
			break
		}
	}
	if exprID < 0 {
		t.Fatal("expr symbol not found")
	}

	var prods []Production
	for _, p := range ng.Productions {
		if p.LHS == exprID {
			prods = append(prods, p)
		}
	}

	t.Logf("expr productions: %d", len(prods))
	ccSet := make(map[int]bool)
	for i, p := range prods {
		rhsNames := make([]string, len(p.RHS))
		for j, rid := range p.RHS {
			if rid >= 0 && rid < len(ng.Symbols) {
				rhsNames[j] = ng.Symbols[rid].Name
			} else {
				rhsNames[j] = fmt.Sprintf("?%d", rid)
			}
		}
		t.Logf("  prod[%d]: cc=%d rhs=[%s] prec=%d assoc=%d", i, len(p.RHS), strings.Join(rhsNames, ", "), p.Prec, p.Assoc)
		ccSet[len(p.RHS)] = true
	}

	// expr → '(' expr ')' → cc=3
	// expr → '(' ')' → cc=2
	if !ccSet[3] {
		t.Error("MISSING: cc=3 (expr → '(' expr ')')")
	}
	if !ccSet[2] {
		t.Error("MISSING: cc=2 (expr → '(' ')')")
	}
}

// TestProductionNestedOptionals tests seq with multiple optionals.
func TestProductionNestedOptionals(t *testing.T) {
	g := &Grammar{
		Name: "test_nested_opt",
		Rules: map[string]*Rule{
			"document": Seq(Sym("list")),
			"list": Seq(
				Str("["),
				Optional(Sym("items")),
				Optional(Str(",")),
				Str("]"),
			),
			"items": Pat(`[a-z]+`),
		},
		RuleOrder: []string{"document", "list", "items"},
	}

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	listID := -1
	for i, info := range ng.Symbols {
		if info.Name == "list" {
			listID = i
			break
		}
	}
	if listID < 0 {
		t.Fatal("list symbol not found")
	}

	var prods []Production
	for _, p := range ng.Productions {
		if p.LHS == listID {
			prods = append(prods, p)
		}
	}

	t.Logf("list productions: %d", len(prods))
	ccSet := make(map[int]bool)
	for i, p := range prods {
		rhsNames := make([]string, len(p.RHS))
		for j, rid := range p.RHS {
			if rid >= 0 && rid < len(ng.Symbols) {
				rhsNames[j] = ng.Symbols[rid].Name
			} else {
				rhsNames[j] = fmt.Sprintf("?%d", rid)
			}
		}
		t.Logf("  prod[%d]: cc=%d rhs=[%s]", i, len(p.RHS), strings.Join(rhsNames, ", "))
		ccSet[len(p.RHS)] = true
	}

	// Should produce 4 alternatives: cc=4 (both), cc=3 (no comma), cc=3 (no items), cc=2 (neither)
	if !ccSet[4] {
		t.Error("MISSING: cc=4 (list → '[' items ',' ']')")
	}
	if !ccSet[3] {
		t.Error("MISSING: cc=3 (list → '[' items ']' or list → '[' ',' ']')")
	}
	if !ccSet[2] {
		t.Error("MISSING: cc=2 (list → '[' ']')")
	}
	if len(prods) != 4 {
		t.Errorf("expected 4 productions, got %d", len(prods))
	}
}

// TestProductionAliasOptional tests optional inside an alias.
func TestProductionAliasOptional(t *testing.T) {
	g := &Grammar{
		Name: "test_alias_opt",
		Rules: map[string]*Rule{
			"document": Seq(Sym("string")),
			"string": Seq(
				Alias(Str("\""), "quote", false),
				Optional(Sym("content")),
				Alias(Str("\""), "quote", false),
			),
			"content": Pat(`[^"]+`),
		},
		RuleOrder: []string{"document", "string", "content"},
	}

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	stringID := -1
	for i, info := range ng.Symbols {
		if info.Name == "string" {
			stringID = i
			break
		}
	}
	if stringID < 0 {
		t.Fatal("string symbol not found")
	}

	var prods []Production
	for _, p := range ng.Productions {
		if p.LHS == stringID {
			prods = append(prods, p)
		}
	}

	t.Logf("string productions: %d", len(prods))
	ccSet := make(map[int]bool)
	for i, p := range prods {
		rhsNames := make([]string, len(p.RHS))
		for j, rid := range p.RHS {
			if rid >= 0 && rid < len(ng.Symbols) {
				rhsNames[j] = ng.Symbols[rid].Name
			} else {
				rhsNames[j] = fmt.Sprintf("?%d", rid)
			}
		}
		t.Logf("  prod[%d]: cc=%d rhs=[%s] aliases=%v", i, len(p.RHS), strings.Join(rhsNames, ", "), p.Aliases)
		ccSet[len(p.RHS)] = true
	}

	if !ccSet[3] {
		t.Error("MISSING: cc=3 (string → quote content quote)")
	}
	if !ccSet[2] {
		t.Error("MISSING: cc=2 (string → quote quote)")
	}
}

// TestProductionRepeatInSeq tests that repeat inside a seq creates correct aux productions.
// Tree-sitter lowers repeat(x) as optional(repeat1(x)), which means the parent
// gets both cc=N (with repeat) and cc=N-1 (without repeat) production variants.
func TestProductionRepeatInSeq(t *testing.T) {
	g := &Grammar{
		Name: "test_repeat_seq",
		Rules: map[string]*Rule{
			"document": Seq(Sym("string")),
			"string": Seq(
				Str("\""),
				Repeat(Sym("string_content")),
				Str("\""),
			),
			"string_content": Pat(`[^"\\]+`),
		},
		RuleOrder: []string{"document", "string", "string_content"},
	}

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	// Find the string symbol.
	stringID := -1
	for i, info := range ng.Symbols {
		if info.Name == "string" {
			stringID = i
			break
		}
	}
	if stringID < 0 {
		t.Fatal("string symbol not found")
	}

	// Dump ALL productions.
	t.Logf("total productions: %d", len(ng.Productions))
	for i, p := range ng.Productions {
		lhsName := "?"
		if p.LHS >= 0 && p.LHS < len(ng.Symbols) {
			lhsName = ng.Symbols[p.LHS].Name
		}
		rhsNames := make([]string, len(p.RHS))
		for j, rid := range p.RHS {
			if rid >= 0 && rid < len(ng.Symbols) {
				rhsNames[j] = ng.Symbols[rid].Name
			} else {
				rhsNames[j] = fmt.Sprintf("?%d", rid)
			}
		}
		t.Logf("  prod[%d]: %s → [%s] (cc=%d)", i, lhsName, strings.Join(rhsNames, ", "), len(p.RHS))
	}

	// Verify string has both cc=2 and cc=3 (matching tree-sitter).
	ccSet := make(map[int]bool)
	for _, p := range ng.Productions {
		if p.LHS == stringID {
			ccSet[len(p.RHS)] = true
		}
	}
	if !ccSet[3] {
		t.Error("MISSING: cc=3 (string → '\"' repeat '\"')")
	}
	if !ccSet[2] {
		t.Error("MISSING: cc=2 (string → '\"' '\"')")
	}

	// Verify repeat aux has only cc=2 productions, matching tree-sitter's
	// helper shape where the parent handles the 0- and 1-item cases directly.
	for i, info := range ng.Symbols {
		if strings.HasPrefix(info.Name, "_string_repeat") {
			var auxCCs []int
			for _, p := range ng.Productions {
				if p.LHS == i {
					auxCCs = append(auxCCs, len(p.RHS))
				}
			}
			t.Logf("  aux %q: cc values = %v", info.Name, auxCCs)
			hasTwo := false
			for _, cc := range auxCCs {
				if cc == 2 {
					hasTwo = true
					continue
				}
				t.Errorf("repeat aux %q has non-tree-sitter child count %d (ccs=%v)", info.Name, cc, auxCCs)
			}
			if !hasTwo {
				t.Errorf("repeat aux %q missing cc=2 production", info.Name)
			}
		}
	}
}

// TestProductionChoiceWithBlank tests that imported CHOICE+BLANK (without Optional lowering) works.
func TestProductionChoiceWithBlank(t *testing.T) {
	// Simulates what grammar.json import produces: CHOICE with BLANK directly.
	g := &Grammar{
		Name: "test_choice_blank",
		Rules: map[string]*Rule{
			"document": Seq(Sym("string")),
			"string": Seq(
				Str("\""),
				Choice(Sym("string_content"), Blank()),
				Str("\""),
			),
			"string_content": Pat(`[^"]+`),
		},
		RuleOrder: []string{"document", "string", "string_content"},
	}

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	stringID := -1
	for i, info := range ng.Symbols {
		if info.Name == "string" {
			stringID = i
			break
		}
	}
	if stringID < 0 {
		t.Fatal("string symbol not found")
	}

	var prods []Production
	for _, p := range ng.Productions {
		if p.LHS == stringID {
			prods = append(prods, p)
		}
	}

	t.Logf("string productions: %d", len(prods))
	ccSet := make(map[int]bool)
	for i, p := range prods {
		rhsNames := make([]string, len(p.RHS))
		for j, rid := range p.RHS {
			if rid >= 0 && rid < len(ng.Symbols) {
				rhsNames[j] = ng.Symbols[rid].Name
			} else {
				rhsNames[j] = fmt.Sprintf("?%d", rid)
			}
		}
		t.Logf("  prod[%d]: cc=%d rhs=[%s]", i, len(p.RHS), strings.Join(rhsNames, ", "))
		ccSet[len(p.RHS)] = true
	}

	if !ccSet[3] {
		t.Error("MISSING: cc=3 (string → '\"' content '\"')")
	}
	if !ccSet[2] {
		t.Error("MISSING: cc=2 (string → '\"' '\"')")
	}
}
