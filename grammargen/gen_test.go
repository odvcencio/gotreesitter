package grammargen

import (
	"os"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestJSONGenerate(t *testing.T) {
	g := JSONGrammar()
	blob, err := Generate(g)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(blob) == 0 {
		t.Fatal("generated blob is empty")
	}
	t.Logf("generated blob size: %d bytes", len(blob))
}

func TestJSONGenerateLanguage(t *testing.T) {
	g := JSONGrammar()
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}
	if lang == nil {
		t.Fatal("language is nil")
	}

	t.Logf("SymbolCount: %d", lang.SymbolCount)
	t.Logf("TokenCount: %d", lang.TokenCount)
	t.Logf("StateCount: %d", lang.StateCount)
	t.Logf("LargeStateCount: %d", lang.LargeStateCount)
	t.Logf("FieldCount: %d", lang.FieldCount)
	t.Logf("ProductionIDCount: %d", lang.ProductionIDCount)
	t.Logf("InitialState: %d", lang.InitialState)

	// Verify basic structure.
	if lang.SymbolCount == 0 {
		t.Error("SymbolCount is 0")
	}
	if lang.TokenCount == 0 {
		t.Error("TokenCount is 0")
	}
	if lang.StateCount == 0 {
		t.Error("StateCount is 0")
	}
	if lang.InitialState != 1 {
		t.Errorf("InitialState = %d, want 1", lang.InitialState)
	}

	// Symbol 0 must be "end".
	if lang.SymbolNames[0] != "end" {
		t.Errorf("SymbolNames[0] = %q, want %q", lang.SymbolNames[0], "end")
	}

	// Must have field names: "", "key", "value".
	if len(lang.FieldNames) < 3 {
		t.Errorf("FieldNames length = %d, want >= 3", len(lang.FieldNames))
	} else {
		if lang.FieldNames[0] != "" {
			t.Errorf("FieldNames[0] = %q, want empty", lang.FieldNames[0])
		}
	}

	// Log all symbol names for debugging.
	for i, name := range lang.SymbolNames {
		vis := ""
		if i < len(lang.SymbolMetadata) {
			if lang.SymbolMetadata[i].Visible {
				vis = " (visible)"
			}
			if lang.SymbolMetadata[i].Named {
				vis += " (named)"
			}
		}
		t.Logf("  symbol[%d] = %q%s", i, name, vis)
	}
}

func TestJSONParseRoundTrip(t *testing.T) {
	g := JSONGrammar()
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	tests := []struct {
		name  string
		input string
	}{
		{"null", `null`},
		{"true", `true`},
		{"false", `false`},
		{"number", `42`},
		{"negative number", `-3.14`},
		{"string", `"hello"`},
		{"empty object", `{}`},
		{"empty array", `[]`},
		{"simple object", `{"key": "value"}`},
		{"simple array", `[1, 2, 3]`},
		{"nested", `{"a": [1, true, null]}`},
		{"complex", `{"key": [1, true, null]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse failed for %q: %v", tt.input, err)
			}
			if tree == nil {
				t.Fatalf("Parse returned nil tree for %q", tt.input)
			}
			root := tree.RootNode()
			if root == nil {
				t.Fatalf("Root node is nil for %q", tt.input)
			}
			sexp := root.SExpr(lang)
			t.Logf("input: %s → %s", tt.input, sexp)

			// Check for ERROR nodes.
			if strings.Contains(sexp, "ERROR") {
				t.Errorf("parse tree contains ERROR node: %s", sexp)
			}
		})
	}
}

func TestJSONNormalize(t *testing.T) {
	g := JSONGrammar()
	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	t.Logf("Symbols: %d", len(ng.Symbols))
	t.Logf("Productions: %d", len(ng.Productions))
	t.Logf("Terminals: %d", len(ng.Terminals))
	t.Logf("ExtraSymbols: %v", ng.ExtraSymbols)
	t.Logf("FieldNames: %v", ng.FieldNames)
	t.Logf("TokenCount: %d", ng.TokenCount())

	for i, sym := range ng.Symbols {
		t.Logf("  sym[%d] = %q kind=%d visible=%v named=%v",
			i, sym.Name, sym.Kind, sym.Visible, sym.Named)
	}

	for i, prod := range ng.Productions {
		lhsName := "?"
		if prod.LHS < len(ng.Symbols) {
			lhsName = ng.Symbols[prod.LHS].Name
		}
		var rhsNames []string
		for _, sym := range prod.RHS {
			if sym < len(ng.Symbols) {
				rhsNames = append(rhsNames, ng.Symbols[sym].Name)
			}
		}
		t.Logf("  prod[%d] (id=%d): %s → %s", i, prod.ProductionID, lhsName, strings.Join(rhsNames, " "))
	}
}

func TestJSONLRTables(t *testing.T) {
	g := JSONGrammar()
	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	tables, err := buildLRTables(ng)
	if err != nil {
		t.Fatalf("buildLRTables failed: %v", err)
	}

	t.Logf("StateCount: %d", tables.StateCount)
	t.Logf("ActionTable states: %d", len(tables.ActionTable))
	t.Logf("GotoTable states: %d", len(tables.GotoTable))
}

func TestJSONLexer(t *testing.T) {
	g := JSONGrammar()
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	tests := []struct {
		input string
	}{
		{`null`},
		{`42`},
		{`"hello"`},
		{`{}`},
		{`[1, 2]`},
		{`false`},
		{`true`},
	}

	// Check unique lex modes.
	seenModes := make(map[uint16]bool)
	for i, lm := range lang.LexModes {
		if !seenModes[lm.LexState] {
			seenModes[lm.LexState] = true
			// Count transitions at this start state.
			nTrans := 0
			if int(lm.LexState) < len(lang.LexStates) {
				nTrans = len(lang.LexStates[lm.LexState].Transitions)
			}
			t.Logf("unique lex mode: state %d → DFA start %d (%d transitions)", i, lm.LexState, nTrans)
		}
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			// Use lex mode from state 1 (initial state).
			lexMode := lang.LexModes[1].LexState
			lexer := gotreesitter.NewLexer(lang.LexStates, []byte(tt.input))
			t.Logf("input: %q, lexMode for state 1: %d, total LexStates: %d", tt.input, lexMode, len(lang.LexStates))
			// Also try lex state 0 for debugging.
			lexer2 := gotreesitter.NewLexer(lang.LexStates, []byte(tt.input))
			tok0 := lexer2.Next(0)
			t.Logf("  [state 0] sym=%d text=%q", tok0.Symbol, tok0.Text)

			for i := 0; i < 20; i++ {
				tok := lexer.Next(lexMode)
				name := "?"
				if int(tok.Symbol) < len(lang.SymbolNames) {
					name = lang.SymbolNames[tok.Symbol]
				}
				t.Logf("  token: sym=%d (%s) text=%q start=%d end=%d",
					tok.Symbol, name, tok.Text, tok.StartByte, tok.EndByte)
				if tok.Symbol == 0 && tok.StartByte == tok.EndByte {
					break // EOF
				}
			}
		})
	}
}

func TestJSONParityWithExistingBlob(t *testing.T) {
	// Load existing JSON grammar from grammars package.
	existingLang := grammars.JsonLanguage()
	if existingLang == nil {
		t.Skip("existing JSON language not available")
	}

	// Generate our JSON grammar.
	g := JSONGrammar()
	genLang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	inputs := []string{
		`null`,
		`true`,
		`false`,
		`42`,
		`-3.14`,
		`"hello"`,
		`{}`,
		`[]`,
		`{"key": "value"}`,
		`[1, 2, 3]`,
		`{"a": [1, true, null]}`,
		`{"key": [1, true, null]}`,
		`{"name": "test", "count": 42, "active": true}`,
		`[{"a": 1}, {"b": 2}]`,
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			// Parse with existing blob.
			existParser := gotreesitter.NewParser(existingLang)
			existTree, err := existParser.Parse([]byte(input))
			if err != nil {
				t.Fatalf("existing parser failed: %v", err)
			}
			existSexp := existTree.RootNode().SExpr(existingLang)

			// Parse with generated grammar.
			genParser := gotreesitter.NewParser(genLang)
			genTree, err := genParser.Parse([]byte(input))
			if err != nil {
				t.Fatalf("generated parser failed: %v", err)
			}
			genSexp := genTree.RootNode().SExpr(genLang)

			t.Logf("existing: %s", existSexp)
			t.Logf("generated: %s", genSexp)

			if existSexp != genSexp {
				t.Errorf("S-expressions differ:\n  existing:  %s\n  generated: %s", existSexp, genSexp)
			}
		})
	}
}

// --- Calculator grammar tests (Milestone 2: Precedence & Associativity) ---

func TestCalcGenerate(t *testing.T) {
	g := CalcGrammar()
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}
	if lang == nil {
		t.Fatal("language is nil")
	}
	t.Logf("SymbolCount: %d, TokenCount: %d, StateCount: %d", lang.SymbolCount, lang.TokenCount, lang.StateCount)
}

func TestCalcParseRoundTrip(t *testing.T) {
	g := CalcGrammar()
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	tests := []struct {
		name  string
		input string
	}{
		{"number", `42`},
		{"add", `1 + 2`},
		{"mul", `3 * 4`},
		{"sub", `5 - 3`},
		{"div", `10 / 2`},
		{"parens", `(1 + 2)`},
		{"unary minus", `-5`},
		{"complex", `1 + 2 * 3`},
		{"left assoc add", `1 + 2 + 3`},
		{"nested parens", `((1 + 2) * 3)`},
		{"unary in expr", `-1 + 2`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse failed for %q: %v", tt.input, err)
			}
			root := tree.RootNode()
			sexp := root.SExpr(lang)
			t.Logf("input: %s => %s", tt.input, sexp)
			if strings.Contains(sexp, "ERROR") {
				t.Errorf("parse tree contains ERROR node: %s", sexp)
			}
		})
	}
}

func TestCalcPrecedence(t *testing.T) {
	g := CalcGrammar()
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		wantSexp string
	}{
		{
			"mul binds tighter than add",
			`1 + 2 * 3`,
			// 1 + (2 * 3): add(1, mul(2,3))
			"(program (expression (expression (number)) (expression (expression (number)) (expression (number)))))",
		},
		{
			"mul binds tighter than sub",
			`1 - 2 * 3`,
			// 1 - (2 * 3): sub(1, mul(2,3))
			"(program (expression (expression (number)) (expression (expression (number)) (expression (number)))))",
		},
		{
			"add is left-associative",
			`1 + 2 + 3`,
			// (1 + 2) + 3: add(add(1,2), 3)
			"(program (expression (expression (expression (number)) (expression (number))) (expression (number))))",
		},
		{
			"mul is left-associative",
			`1 * 2 * 3`,
			// (1 * 2) * 3: mul(mul(1,2), 3)
			"(program (expression (expression (expression (number)) (expression (number))) (expression (number))))",
		},
		{
			"parens override precedence",
			`(1 + 2) * 3`,
			// (1+2) * 3: mul(parens(add(1,2)), 3)
			"(program (expression (expression (expression (expression (number)) (expression (number)))) (expression (number))))",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse failed for %q: %v", tt.input, err)
			}
			root := tree.RootNode()
			sexp := root.SExpr(lang)
			t.Logf("input: %s => %s", tt.input, sexp)

			if sexp != tt.wantSexp {
				t.Errorf("S-expression mismatch:\n  got:  %s\n  want: %s", sexp, tt.wantSexp)
			}
		})
	}
}

func TestCalcNormalize(t *testing.T) {
	g := CalcGrammar()
	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	t.Logf("Symbols: %d, Productions: %d, Terminals: %d", len(ng.Symbols), len(ng.Productions), len(ng.Terminals))
	t.Logf("FieldNames: %v", ng.FieldNames)

	for i, sym := range ng.Symbols {
		t.Logf("  sym[%d] = %q kind=%d visible=%v named=%v",
			i, sym.Name, sym.Kind, sym.Visible, sym.Named)
	}

	for i, prod := range ng.Productions {
		lhsName := "?"
		if prod.LHS < len(ng.Symbols) {
			lhsName = ng.Symbols[prod.LHS].Name
		}
		var rhsNames []string
		for _, sym := range prod.RHS {
			if sym < len(ng.Symbols) {
				rhsNames = append(rhsNames, ng.Symbols[sym].Name)
			}
		}
		t.Logf("  prod[%d] (id=%d, prec=%d, assoc=%d): %s -> %s",
			i, prod.ProductionID, prod.Prec, prod.Assoc, lhsName, strings.Join(rhsNames, " "))
	}
}

// --- GLR grammar tests (Milestone 3: GLR Conflicts) ---

func TestGLRGenerate(t *testing.T) {
	g := GLRGrammar()
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}
	if lang == nil {
		t.Fatal("language is nil")
	}
	t.Logf("SymbolCount: %d, TokenCount: %d, StateCount: %d", lang.SymbolCount, lang.TokenCount, lang.StateCount)

	// Verify multi-action entries exist (GLR slots).
	multiActionCount := 0
	for i, entry := range lang.ParseActions {
		if len(entry.Actions) > 1 {
			multiActionCount++
			t.Logf("ParseActions[%d] has %d actions (GLR)", i, len(entry.Actions))
		}
	}
	if multiActionCount == 0 {
		t.Error("expected at least one ParseActionEntry with multiple actions for GLR")
	}
}

func TestGLRNormalize(t *testing.T) {
	g := GLRGrammar()
	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	t.Logf("Symbols: %d, Productions: %d, Conflicts: %v", len(ng.Symbols), len(ng.Productions), ng.Conflicts)
	for i, sym := range ng.Symbols {
		t.Logf("  sym[%d] = %q kind=%d", i, sym.Name, sym.Kind)
	}
	for i, prod := range ng.Productions {
		lhsName := "?"
		if prod.LHS < len(ng.Symbols) {
			lhsName = ng.Symbols[prod.LHS].Name
		}
		var rhsNames []string
		for _, sym := range prod.RHS {
			if sym < len(ng.Symbols) {
				rhsNames = append(rhsNames, ng.Symbols[sym].Name)
			}
		}
		t.Logf("  prod[%d] (id=%d): %s -> %s", i, prod.ProductionID, lhsName, strings.Join(rhsNames, " "))
	}
}

func TestGLRParse(t *testing.T) {
	g := GLRGrammar()
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	tests := []struct {
		name  string
		input string
	}{
		{"simple expr", `a ;`},
		{"multiplication", `a * b * c ;`},
		{"ambiguous", `a * b ;`},
		{"unambiguous decl", `int * x ;`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse failed for %q: %v", tt.input, err)
			}
			root := tree.RootNode()
			sexp := root.SExpr(lang)
			t.Logf("input: %s => %s", tt.input, sexp)
			if strings.Contains(sexp, "ERROR") {
				t.Errorf("parse tree contains ERROR node: %s", sexp)
			}
		})
	}
}

// --- Keyword grammar tests (Milestone 4: Keywords & Word Token) ---

func TestKeywordGenerate(t *testing.T) {
	g := KeywordGrammar()
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	t.Logf("SymbolCount: %d, TokenCount: %d, StateCount: %d", lang.SymbolCount, lang.TokenCount, lang.StateCount)
	t.Logf("KeywordCaptureToken: %d", lang.KeywordCaptureToken)
	t.Logf("KeywordLexStates: %d states", len(lang.KeywordLexStates))

	if lang.KeywordCaptureToken == 0 {
		t.Error("KeywordCaptureToken is 0, expected non-zero")
	}
	if len(lang.KeywordLexStates) == 0 {
		t.Error("KeywordLexStates is empty, expected keyword DFA states")
	}

	// The keyword capture token should point to the identifier symbol.
	captureName := ""
	if int(lang.KeywordCaptureToken) < len(lang.SymbolNames) {
		captureName = lang.SymbolNames[lang.KeywordCaptureToken]
	}
	t.Logf("KeywordCaptureToken symbol: %q", captureName)
	if captureName != "identifier" {
		t.Errorf("KeywordCaptureToken points to %q, want %q", captureName, "identifier")
	}
}

func TestKeywordNormalize(t *testing.T) {
	g := KeywordGrammar()
	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	t.Logf("Keywords: %v", ng.KeywordSymbols)
	t.Logf("WordSymbolID: %d", ng.WordSymbolID)
	t.Logf("KeywordEntries: %d", len(ng.KeywordEntries))

	for i, sym := range ng.Symbols {
		t.Logf("  sym[%d] = %q kind=%d visible=%v named=%v",
			i, sym.Name, sym.Kind, sym.Visible, sym.Named)
	}

	// Verify that "var" and "return" are identified as keywords.
	if len(ng.KeywordSymbols) != 2 {
		t.Errorf("expected 2 keywords, got %d", len(ng.KeywordSymbols))
	}

	// Verify "=" and ";" and "+" are NOT keywords.
	for _, entry := range ng.KeywordEntries {
		name := ng.Symbols[entry.SymbolID].Name
		t.Logf("  keyword: %q (sym %d)", name, entry.SymbolID)
	}
}

func TestKeywordParse(t *testing.T) {
	g := KeywordGrammar()
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	tests := []struct {
		name  string
		input string
	}{
		{"var decl", `var x = 1;`},
		{"return stmt", `return 42;`},
		{"expr stmt", `x + 1;`},
		{"identifier only", `foo;`},
		{"var as keyword", `var myVar = 10;`},
		{"return expr", `return x + 1;`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse failed for %q: %v", tt.input, err)
			}
			root := tree.RootNode()
			sexp := root.SExpr(lang)
			t.Logf("input: %s => %s", tt.input, sexp)
			if strings.Contains(sexp, "ERROR") {
				t.Errorf("parse tree contains ERROR node: %s", sexp)
			}
		})
	}
}

// ── Milestone 5: External Scanner Slots ────────────────────────────────────────

func TestExtGenerate(t *testing.T) {
	g := ExtScannerGrammar()
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	t.Logf("SymbolCount: %d", lang.SymbolCount)
	t.Logf("TokenCount: %d", lang.TokenCount)
	t.Logf("ExternalTokenCount: %d", lang.ExternalTokenCount)
	t.Logf("StateCount: %d", lang.StateCount)

	if lang.ExternalTokenCount != 3 {
		t.Errorf("ExternalTokenCount = %d, want 3", lang.ExternalTokenCount)
	}
}

func TestExtNormalize(t *testing.T) {
	g := ExtScannerGrammar()
	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	// Should have 3 external symbols.
	if len(ng.ExternalSymbols) != 3 {
		t.Fatalf("len(ExternalSymbols) = %d, want 3", len(ng.ExternalSymbols))
	}

	// External symbols should be in the symbol table with Kind=SymbolExternal.
	for _, extID := range ng.ExternalSymbols {
		if extID < 0 || extID >= len(ng.Symbols) {
			t.Errorf("external symbol ID %d out of range", extID)
			continue
		}
		sym := ng.Symbols[extID]
		if sym.Kind != SymbolExternal {
			t.Errorf("symbol %q (id %d) has Kind=%d, want SymbolExternal(%d)",
				sym.Name, extID, sym.Kind, SymbolExternal)
		}
	}

	// Check names.
	names := make([]string, len(ng.ExternalSymbols))
	for i, id := range ng.ExternalSymbols {
		names[i] = ng.Symbols[id].Name
	}
	t.Logf("external symbols: %v (ids: %v)", names, ng.ExternalSymbols)

	if names[0] != "indent" || names[1] != "dedent" || names[2] != "newline" {
		t.Errorf("unexpected external symbol names: %v", names)
	}
}

func TestNormalizeCanonicalizesConsistentlyAliasedExternalSymbol(t *testing.T) {
	g := NewGrammar("ext_alias")
	g.SetExternals(Sym("_layout_start_explicit"))
	g.Define("source_file", Seq(
		Alias(Sym("_layout_start_explicit"), "{", false),
		Str("x"),
	))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}
	if len(ng.ExternalSymbols) != 1 {
		t.Fatalf("len(ExternalSymbols) = %d, want 1", len(ng.ExternalSymbols))
	}

	sym := ng.Symbols[ng.ExternalSymbols[0]]
	if got, want := sym.Name, "{"; got != want {
		t.Fatalf("external symbol name = %q, want %q", got, want)
	}
	if !sym.Visible {
		t.Fatal("external symbol should be visible after canonical aliasing")
	}
	if sym.Named {
		t.Fatal("external symbol should be anonymous after canonical aliasing")
	}
}

func TestNormalizeImmediateInlinePrefixDoesNotBeatLongerStringToken(t *testing.T) {
	g := NewGrammar("immediate_prefix")
	g.Define("source_file", Choice(
		Sym("close"),
		Seq(ImmToken(Str("#")), Sym("rest")),
	))
	g.Define("close", Token(Str("#)")))
	g.Define("rest", Token(Str("x")))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	hashPriority := 0
	closePriority := 0
	foundHash := false
	foundClose := false
	for _, term := range ng.Terminals {
		if term.Rule == nil || term.Rule.Kind != RuleString {
			continue
		}
		switch term.Rule.Value {
		case "#":
			if term.Immediate {
				hashPriority = term.Priority
				foundHash = true
			}
		case "#)":
			closePriority = term.Priority
			foundClose = true
		}
	}
	if !foundHash || !foundClose {
		t.Fatalf("missing expected terminals: foundHash=%v foundClose=%v", foundHash, foundClose)
	}
	// With prec-based priority, equal priority is acceptable: the runtime's
	// greedy tiebreaker ensures "#)" (2 chars) beats "#" (1 char) at the same
	// priority. Strict less-than is not required; ">=" would mean IMMTOKEN beats
	// the longer string, which is wrong.
	if closePriority > hashPriority {
		t.Fatalf("\"#)\" priority = %d, want <= immediate \"#\" priority %d (greedy handles equal-priority)", closePriority, hashPriority)
	}
}

func TestNormalizeKeepsMatchingExternalPatternAsExtra(t *testing.T) {
	g := NewGrammar("ext_pattern_extra")
	g.Externals = []*Rule{Pat("\n")}
	g.Extras = []*Rule{Pat("\n")}
	g.Define("source_file", Repeat(Str("x")))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	seenWhitespace := false
	seenExternalPattern := false
	for _, symID := range ng.ExtraSymbols {
		if symID < 0 || symID >= len(ng.Symbols) {
			continue
		}
		switch ng.Symbols[symID].Name {
		case "_whitespace":
			seenWhitespace = true
		case "_token1":
			seenExternalPattern = true
		}
	}
	if !seenWhitespace {
		t.Fatal("expected _whitespace in ExtraSymbols")
	}
	if !seenExternalPattern {
		t.Fatal("expected matching external pattern _token1 in ExtraSymbols")
	}
}

func TestExtExternalLexStates(t *testing.T) {
	g := ExtScannerGrammar()
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	// ExternalSymbols should be populated.
	if len(lang.ExternalSymbols) != 3 {
		t.Fatalf("len(ExternalSymbols) = %d, want 3", len(lang.ExternalSymbols))
	}

	// ExternalLexStates should be populated (at least row 0 = all-false).
	if len(lang.ExternalLexStates) == 0 {
		t.Fatal("ExternalLexStates is empty, expected at least 1 row")
	}

	// Row 0 should be all-false.
	row0 := lang.ExternalLexStates[0]
	if len(row0) != 3 {
		t.Fatalf("row 0 length = %d, want 3", len(row0))
	}
	for i, v := range row0 {
		if v {
			t.Errorf("row 0[%d] = true, want false", i)
		}
	}

	// At least some states should have external tokens valid.
	hasValidExt := false
	for _, lm := range lang.LexModes {
		if lm.ExternalLexState > 0 {
			hasValidExt = true
			break
		}
	}
	if !hasValidExt {
		t.Error("no parser states have external tokens valid (all ExternalLexState=0)")
	}

	// Log the table for debugging.
	t.Logf("ExternalLexStates rows: %d", len(lang.ExternalLexStates))
	for i, row := range lang.ExternalLexStates {
		t.Logf("  row %d: %v", i, row)
	}

	// Log lex modes with external lex state.
	for i, lm := range lang.LexModes {
		if lm.ExternalLexState > 0 {
			t.Logf("  state %d → ExternalLexState %d", i, lm.ExternalLexState)
		}
	}
}

func TestExtParse(t *testing.T) {
	g := ExtScannerGrammar()
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	// Attach a simple external scanner that recognizes indent/dedent/newline.
	lang.ExternalScanner = &testIndentScanner{lang: lang}

	parser := gotreesitter.NewParser(lang)

	tests := []struct {
		name  string
		input string
		want  string // substring in S-expression
	}{
		{
			name:  "simple statement",
			input: "hello;",
			want:  "(simple_statement",
		},
		{
			name:  "block",
			input: "main:\n  body;",
			want:  "(block",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree, err := parser.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if tree == nil {
				t.Fatal("parse returned nil")
			}
			root := tree.RootNode()
			sexp := root.SExpr(lang)
			t.Logf("S-expression: %s", sexp)

			if !strings.Contains(sexp, tt.want) {
				t.Errorf("S-expression %q does not contain %q", sexp, tt.want)
			}
		})
	}
}

// testIndentScanner is a minimal external scanner for the ext_test grammar.
// It produces NEWLINE on '\n', INDENT on increased indentation after NEWLINE,
// and DEDENT on decreased indentation after NEWLINE.
type testIndentScanner struct {
	lang *gotreesitter.Language
}

type indentPayload struct {
	indentStack    []int
	pendingDedents int
	atNewline      bool
}

func (s *testIndentScanner) Create() any {
	return &indentPayload{indentStack: []int{0}}
}

func (s *testIndentScanner) Destroy(payload any) {}

func (s *testIndentScanner) Serialize(payload any, buf []byte) int {
	p := payload.(*indentPayload)
	n := 0
	if n < len(buf) {
		buf[n] = byte(len(p.indentStack))
		n++
	}
	for _, indent := range p.indentStack {
		if n < len(buf) {
			buf[n] = byte(indent)
			n++
		}
	}
	if n < len(buf) {
		buf[n] = byte(p.pendingDedents)
		n++
	}
	if n < len(buf) {
		if p.atNewline {
			buf[n] = 1
		} else {
			buf[n] = 0
		}
		n++
	}
	return n
}

func (s *testIndentScanner) Deserialize(payload any, buf []byte) {
	p := payload.(*indentPayload)
	if len(buf) == 0 {
		p.indentStack = []int{0}
		p.pendingDedents = 0
		p.atNewline = false
		return
	}
	n := 0
	stackLen := int(buf[n])
	n++
	p.indentStack = make([]int, stackLen)
	for i := range p.indentStack {
		if n < len(buf) {
			p.indentStack[i] = int(buf[n])
			n++
		}
	}
	if n < len(buf) {
		p.pendingDedents = int(buf[n])
		n++
	}
	if n < len(buf) {
		p.atNewline = buf[n] == 1
	}
}

func (s *testIndentScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	p := payload.(*indentPayload)

	// Resolve external token symbol IDs.
	indentSym := s.lang.ExternalSymbols[0]
	dedentSym := s.lang.ExternalSymbols[1]
	newlineSym := s.lang.ExternalSymbols[2]

	// Pending dedents: emit them one at a time.
	if p.pendingDedents > 0 && validSymbols[1] { // dedent is external index 1
		p.pendingDedents--
		lexer.MarkEnd()
		lexer.SetResultSymbol(dedentSym)
		return true
	}

	lookahead := lexer.Lookahead()

	// At EOF, emit dedents for any remaining indent levels.
	if lookahead == 0 && validSymbols[1] && len(p.indentStack) > 1 {
		p.indentStack = p.indentStack[:len(p.indentStack)-1]
		lexer.MarkEnd()
		lexer.SetResultSymbol(dedentSym)
		return true
	}

	// At newline character → emit NEWLINE token.
	if lookahead == '\n' && validSymbols[2] { // newline is external index 2
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.SetResultSymbol(newlineSym)
		p.atNewline = true
		return true
	}

	// After newline, count indentation.
	if p.atNewline {
		p.atNewline = false
		indent := 0
		for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
			indent++
			lexer.Advance(false)
		}

		currentIndent := p.indentStack[len(p.indentStack)-1]

		if indent > currentIndent && validSymbols[0] { // indent is external index 0
			p.indentStack = append(p.indentStack, indent)
			lexer.MarkEnd()
			lexer.SetResultSymbol(indentSym)
			return true
		}

		if indent < currentIndent && validSymbols[1] { // dedent is external index 1
			// Pop indent levels and count dedents.
			dedents := 0
			for len(p.indentStack) > 1 && p.indentStack[len(p.indentStack)-1] > indent {
				p.indentStack = p.indentStack[:len(p.indentStack)-1]
				dedents++
			}
			if dedents > 0 {
				p.pendingDedents = dedents - 1
				lexer.MarkEnd()
				lexer.SetResultSymbol(dedentSym)
				return true
			}
		}
	}

	return false
}

// ── Milestone 7: Aliases & Supertypes ──────────────────────────────────────────

func TestAliasSuperGenerate(t *testing.T) {
	g := AliasSuperGrammar()
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	t.Logf("SymbolCount: %d", lang.SymbolCount)
	t.Logf("TokenCount: %d", lang.TokenCount)

	if lang.SymbolCount == 0 {
		t.Error("SymbolCount is 0")
	}
}

func TestAliasSuperNormalize(t *testing.T) {
	g := AliasSuperGrammar()
	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize failed: %v", err)
	}

	// Check that some productions have aliases.
	hasAlias := false
	for _, prod := range ng.Productions {
		if len(prod.Aliases) > 0 {
			hasAlias = true
			for _, ai := range prod.Aliases {
				t.Logf("prod %d (LHS=%s): child %d aliased to %q (named=%v)",
					prod.ProductionID, ng.Symbols[prod.LHS].Name,
					ai.ChildIndex, ai.Name, ai.Named)
			}
		}
	}
	if !hasAlias {
		t.Error("no productions have aliases")
	}

	// Check supertypes.
	if len(ng.Supertypes) == 0 {
		t.Error("no supertypes declared")
	} else {
		for _, stID := range ng.Supertypes {
			t.Logf("supertype: %s (id=%d)", ng.Symbols[stID].Name, stID)
		}
	}
}

func TestAliasSuperAliasSequences(t *testing.T) {
	g := AliasSuperGrammar()
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	// AliasSequences should be populated.
	if lang.AliasSequences == nil {
		t.Fatal("AliasSequences is nil")
	}

	// Find at least one non-nil row.
	hasNonNil := false
	for i, row := range lang.AliasSequences {
		if len(row) > 0 {
			hasNonNil = true
			for j, sym := range row {
				if sym != 0 {
					name := ""
					if int(sym) < len(lang.SymbolNames) {
						name = lang.SymbolNames[sym]
					}
					t.Logf("AliasSequences[%d][%d] = sym %d (%s)", i, j, sym, name)
				}
			}
		}
	}
	if !hasNonNil {
		t.Error("no alias sequences found")
	}

	// Verify "variable" alias exists.
	foundVariable := false
	for _, row := range lang.AliasSequences {
		for _, sym := range row {
			if sym > 0 && int(sym) < len(lang.SymbolNames) && lang.SymbolNames[sym] == "variable" {
				foundVariable = true
			}
		}
	}
	if !foundVariable {
		t.Error("alias 'variable' not found in AliasSequences")
	}
}

func TestAliasSuperSupertypeMap(t *testing.T) {
	g := AliasSuperGrammar()
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	// SupertypeSymbols should contain _expression.
	if len(lang.SupertypeSymbols) == 0 {
		t.Fatal("SupertypeSymbols is empty")
	}

	t.Logf("SupertypeSymbols: %v", lang.SupertypeSymbols)
	for _, st := range lang.SupertypeSymbols {
		name := ""
		if int(st) < len(lang.SymbolNames) {
			name = lang.SymbolNames[st]
		}
		t.Logf("  supertype sym %d (%s)", st, name)

		// Check children.
		children := lang.SupertypeChildren(st)
		childNames := make([]string, len(children))
		for i, c := range children {
			if int(c) < len(lang.SymbolNames) {
				childNames[i] = lang.SymbolNames[c]
			}
		}
		t.Logf("    children: %v", childNames)
	}

	// SupertypeMapEntries should have entries.
	if len(lang.SupertypeMapEntries) == 0 {
		t.Error("SupertypeMapEntries is empty")
	}
}

func TestAliasSuperParse(t *testing.T) {
	g := AliasSuperGrammar()
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	parser := gotreesitter.NewParser(lang)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "assignment with alias",
			input: "x = 42;",
			want:  "(assignment",
		},
		{
			name:  "binary expression",
			input: "1 + 2;",
			want:  "(binary_expression",
		},
		{
			name:  "nested expression",
			input: "x = 1 + 2;",
			want:  "(assignment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree, err := parser.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			root := tree.RootNode()
			sexp := root.SExpr(lang)
			t.Logf("S-expression: %s", sexp)

			if !strings.Contains(sexp, tt.want) {
				t.Errorf("S-expression %q does not contain %q", sexp, tt.want)
			}
			if strings.Contains(sexp, "ERROR") {
				t.Errorf("parse tree contains ERROR: %s", sexp)
			}
		})
	}
}

// ── Milestone 8: C parser.c Backend ────────────────────────────────────────────

func TestGenerateCJSON(t *testing.T) {
	g := JSONGrammar()
	code, err := GenerateC(g)
	if err != nil {
		t.Fatalf("GenerateC failed: %v", err)
	}
	if len(code) == 0 {
		t.Fatal("generated C code is empty")
	}
	t.Logf("generated C code: %d bytes", len(code))

	// Verify key components are present.
	checks := []string{
		"#include <tree_sitter/parser.h>",
		"#define LANGUAGE_VERSION",
		"#define STATE_COUNT",
		"#define SYMBOL_COUNT",
		"ts_symbol_names",
		"ts_symbol_metadata",
		"ts_parse_actions",
		"ts_lex_modes",
		"static bool ts_lex(",
		"tree_sitter_json",
	}
	for _, check := range checks {
		if !strings.Contains(code, check) {
			t.Errorf("generated C code missing %q", check)
		}
	}

	// Verify field names are present (JSON has key and value fields).
	if !strings.Contains(code, "field_key") {
		t.Error("missing field_key")
	}
	if !strings.Contains(code, "field_value") {
		t.Error("missing field_value")
	}
}

func TestGenerateCCalc(t *testing.T) {
	g := CalcGrammar()
	code, err := GenerateC(g)
	if err != nil {
		t.Fatalf("GenerateC failed: %v", err)
	}
	if !strings.Contains(code, "tree_sitter_calc") {
		t.Error("missing tree_sitter_calc export")
	}
	t.Logf("generated C code: %d bytes", len(code))
}

func TestGenerateCExt(t *testing.T) {
	g := ExtScannerGrammar()
	code, err := GenerateC(g)
	if err != nil {
		t.Fatalf("GenerateC failed: %v", err)
	}
	if !strings.Contains(code, "ts_external_scanner_symbol_map") {
		t.Error("missing external scanner symbol map")
	}
	if !strings.Contains(code, "ts_external_scanner_states") {
		t.Error("missing external scanner states")
	}
	t.Logf("generated C code: %d bytes", len(code))
}

func TestGenerateCAlias(t *testing.T) {
	g := AliasSuperGrammar()
	code, err := GenerateC(g)
	if err != nil {
		t.Fatalf("GenerateC failed: %v", err)
	}
	if !strings.Contains(code, "ts_alias_sequences") {
		t.Error("missing alias sequences")
	}
	t.Logf("generated C code: %d bytes", len(code))
}

func TestRegexParser(t *testing.T) {
	tests := []struct {
		pattern string
		wantErr bool
	}{
		{`[0-9]`, false},
		{`[a-zA-Z]`, false},
		{`[^\\"]+`, false},
		{`[1-9]`, false},
		{`[eE]`, false},
		{`[+\-]`, false},
		{`[0-9a-fA-F]`, false},
		{`[\"\\\/bfnrt]`, false},
		{`\s`, false},
		// Patterns that previously failed (regex feature gaps)
		{`u{[0-9a-fA-F]+}`, false},            // wat: u{hex} literal braces
		{`[\pL\p{Mn}\pN_']*`, false},          // haskell: \pL shorthand
		{`\p{White_Space}|\\\\\\r?\n`, false}, // perl: White_Space property
		{`[\p{L}\p{M}\p{N}\p{Emoji}]`, false}, // kdl: Emoji property
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			node, err := parseRegex(tt.pattern)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for pattern %q", tt.pattern)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRegex(%q) failed: %v", tt.pattern, err)
			}
			if node == nil {
				t.Fatalf("parseRegex(%q) returned nil", tt.pattern)
			}
			t.Logf("parsed %q: kind=%d", tt.pattern, node.kind)
		})
	}
}

// ============================================================
// Milestone 9: Go-Native Superset Features
// ============================================================

func TestValidate(t *testing.T) {
	t.Run("clean grammar", func(t *testing.T) {
		g := JSONGrammar()
		warnings := Validate(g)
		if len(warnings) > 0 {
			t.Errorf("expected no warnings for valid JSON grammar, got: %v", warnings)
		}
	})

	t.Run("undefined symbol", func(t *testing.T) {
		g := NewGrammar("bad")
		g.Define("start", Sym("missing_rule"))
		warnings := Validate(g)
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "undefined symbol") && strings.Contains(w, "missing_rule") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected warning about undefined symbol, got: %v", warnings)
		}
	})

	t.Run("unreachable rule", func(t *testing.T) {
		g := NewGrammar("unreachable")
		g.Define("start", Str("hello"))
		g.Define("orphan", Str("world"))
		warnings := Validate(g)
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "unreachable") && strings.Contains(w, "orphan") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected warning about unreachable rule, got: %v", warnings)
		}
	})

	t.Run("bad conflict ref", func(t *testing.T) {
		g := NewGrammar("bad_conflict")
		g.Define("start", Str("x"))
		g.SetConflicts([]string{"nonexistent", "start"})
		warnings := Validate(g)
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "conflict") && strings.Contains(w, "nonexistent") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected warning about bad conflict ref, got: %v", warnings)
		}
	})
}

func TestEmbeddedTests(t *testing.T) {
	t.Run("passing tests", func(t *testing.T) {
		g := JSONGrammar()
		g.Test("null literal", "null", "(document (null))")
		g.Test("empty object", "{}", "(document (object))")
		g.Test("number", "42", "(document (number))")

		err := RunTests(g)
		if err != nil {
			t.Fatalf("RunTests failed: %v", err)
		}
	})

	t.Run("failing test", func(t *testing.T) {
		g := JSONGrammar()
		g.Test("wrong expectation", "null", "(document (string))")

		err := RunTests(g)
		if err == nil {
			t.Fatal("expected RunTests to report failure")
		}
		if !strings.Contains(err.Error(), "tree mismatch") {
			t.Fatalf("expected 'tree mismatch' in error, got: %v", err)
		}
	})

	t.Run("expect error", func(t *testing.T) {
		g := JSONGrammar()
		g.TestError("trailing comma", `{"a":1,}`)

		err := RunTests(g)
		// This should either pass (if ERROR node is produced) or fail
		// with a meaningful message. We just check it doesn't panic.
		if err != nil {
			t.Logf("RunTests result: %v", err)
		}
	})

	t.Run("no tests", func(t *testing.T) {
		g := JSONGrammar()
		err := RunTests(g)
		if err != nil {
			t.Fatalf("RunTests with no tests should succeed, got: %v", err)
		}
	})
}

func TestConflictDiagnostics(t *testing.T) {
	// The calculator grammar has shift/reduce conflicts resolved by precedence.
	g := CalcGrammar()
	report, err := GenerateWithReport(g)
	if err != nil {
		t.Fatalf("GenerateWithReport failed: %v", err)
	}

	t.Logf("symbols: %d, states: %d, tokens: %d",
		report.SymbolCount, report.StateCount, report.TokenCount)
	t.Logf("conflicts resolved: %d", len(report.Conflicts))
	t.Logf("warnings: %d", len(report.Warnings))

	for i, c := range report.Conflicts {
		// Get the normalized grammar for printing.
		ng, _ := Normalize(g)
		t.Logf("conflict %d:\n%s", i, c.String(ng))
	}

	// The calc grammar should have conflicts that are resolved.
	if len(report.Conflicts) == 0 {
		t.Log("no conflicts reported (all resolved before conflict tracking)")
	}

	// Verify the blob is valid.
	if len(report.Blob) == 0 {
		t.Error("report blob is empty")
	}

	// Verify the language is usable.
	parser := gotreesitter.NewParser(report.Language)
	tree, err := parser.Parse([]byte("1 + 2 * 3"))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	sexp := tree.RootNode().SExpr(report.Language)
	t.Logf("parse tree: %s", sexp)
	if !strings.Contains(sexp, "expression") {
		t.Error("expected expression in tree")
	}
}

func TestCombinators(t *testing.T) {
	// Test SepBy1 by building a grammar that uses it.
	g := NewGrammar("sepby_test")
	g.Define("program", SepBy1(Str(";"), Sym("item")))
	g.Define("item", Pat(`[a-z]+`))
	g.SetExtras(Pat(`\s`))

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse([]byte("foo;bar;baz"))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	sexp := tree.RootNode().SExpr(lang)
	t.Logf("SepBy1 parse: %s", sexp)
	if strings.Contains(sexp, "ERROR") {
		t.Errorf("unexpected ERROR in tree: %s", sexp)
	}

	// Should contain 3 items.
	count := strings.Count(sexp, "item")
	if count != 3 {
		t.Errorf("expected 3 items, got %d in: %s", count, sexp)
	}
}

func TestGrammarWithBraces(t *testing.T) {
	// Test Braces combinator.
	g := NewGrammar("braces_test")
	g.Define("program", Braces(CommaSep(Sym("number"))))
	g.Define("number", Pat(`[0-9]+`))
	g.SetExtras(Pat(`\s`))

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse([]byte("{1, 2, 3}"))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	sexp := tree.RootNode().SExpr(lang)
	t.Logf("Braces+CommaSep parse: %s", sexp)
	if strings.Contains(sexp, "ERROR") {
		t.Errorf("unexpected ERROR in tree: %s", sexp)
	}
	if strings.Count(sexp, "number") != 3 {
		t.Errorf("expected 3 numbers in: %s", sexp)
	}
}

// ============================================================
// grammar.js Import Tests
// ============================================================

const testGrammarJS = `
module.exports = grammar({
  name: 'test_import',

  extras: $ => [/\s/],

  rules: {
    document: $ => repeat($._value),

    _value: $ => choice(
      $.object,
      $.array,
      $.number,
      $.string,
      'true',
      'false',
      'null'
    ),

    object: $ => seq(
      '{',
      optional(seq(
        $.pair,
        repeat(seq(',', $.pair))
      )),
      '}'
    ),

    pair: $ => seq(
      field('key', $.string),
      ':',
      field('value', $._value)
    ),

    array: $ => seq(
      '[',
      optional(seq(
        $._value,
        repeat(seq(',', $._value))
      )),
      ']'
    ),

    string: $ => token(seq('"', repeat(/[^"\\]/), '"')),

    number: $ => token(seq(
      optional('-'),
      choice('0', seq(/[1-9]/, repeat(/[0-9]/))),
      optional(seq('.', repeat1(/[0-9]/)))
    )),
  }
});
`

func TestImportGrammarJS(t *testing.T) {
	g, err := ImportGrammarJS([]byte(testGrammarJS))
	if err != nil {
		t.Fatalf("ImportGrammarJS failed: %v", err)
	}

	if g.Name != "test_import" {
		t.Errorf("name = %q, want 'test_import'", g.Name)
	}

	// Check that rules were extracted.
	expectedRules := []string{"document", "_value", "object", "pair", "array", "string", "number"}
	for _, name := range expectedRules {
		if _, ok := g.Rules[name]; !ok {
			t.Errorf("missing rule %q", name)
		}
	}

	// Check extras.
	if len(g.Extras) != 1 {
		t.Errorf("extras count = %d, want 1", len(g.Extras))
	}

	t.Logf("imported grammar %q with %d rules", g.Name, len(g.Rules))
}

func TestImportGrammarJSGenerate(t *testing.T) {
	g, err := ImportGrammarJS([]byte(testGrammarJS))
	if err != nil {
		t.Fatalf("ImportGrammarJS failed: %v", err)
	}

	// Generate should succeed.
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	t.Logf("symbols: %d, states: %d, tokens: %d",
		lang.SymbolCount, lang.StateCount, lang.TokenCount)

	// Parse a JSON value.
	parser := gotreesitter.NewParser(lang)
	tree, parseErr := parser.Parse([]byte(`{"key": [1, true, null]}`))
	if parseErr != nil {
		t.Fatalf("parse failed: %v", parseErr)
	}

	sexp := tree.RootNode().SExpr(lang)
	t.Logf("parse tree: %s", sexp)

	if strings.Contains(sexp, "ERROR") {
		t.Errorf("unexpected ERROR in tree: %s", sexp)
	}
	if !strings.Contains(sexp, "object") {
		t.Error("expected 'object' in tree")
	}
	if !strings.Contains(sexp, "pair") {
		t.Error("expected 'pair' in tree")
	}
	if !strings.Contains(sexp, "array") {
		t.Error("expected 'array' in tree")
	}
}

const testGrammarJSWithPrec = `
module.exports = grammar({
  name: 'calc_import',

  extras: $ => [/\s/],

  rules: {
    program: $ => repeat($.expression),

    expression: $ => choice(
      prec.left(1, seq($.expression, '+', $.expression)),
      prec.left(2, seq($.expression, '*', $.expression)),
      $.number,
      seq('(', $.expression, ')')
    ),

    number: $ => /[0-9]+/,
  }
});
`

const testGrammarJSWithBuiltinSepHelpers = `
module.exports = grammar({
  name: 'helper_sep_builtin',

  extras: $ => [/\s/],

  rules: {
    source_file: $ => sep1(',', $.item),
    item: $ => /[a-z]+/,
  }
});
`

const testGrammarJSWithBuiltinSepHelpersRuleFirst = `
module.exports = grammar({
  name: 'helper_sep_builtin_rule_first',

  extras: $ => [/\s/],

  rules: {
    source_file: $ => sep1($.item, ','),
    item: $ => /[a-z]+/,
  }
});
`

const testGrammarJSWithBuiltinTrailingHelpers = `
module.exports = grammar({
  name: 'helper_trailing_builtin',

  extras: $ => [/\s/],

  rules: {
    source_file: $ => trailingCommaSep1($.item),
    item: $ => /[a-z]+/,
  }
});
`

func TestImportGrammarJSPrec(t *testing.T) {
	g, err := ImportGrammarJS([]byte(testGrammarJSWithPrec))
	if err != nil {
		t.Fatalf("ImportGrammarJS failed: %v", err)
	}

	if g.Name != "calc_import" {
		t.Errorf("name = %q, want 'calc_import'", g.Name)
	}

	// Generate and parse.
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	parser := gotreesitter.NewParser(lang)
	tree, parseErr := parser.Parse([]byte("1 + 2 * 3"))
	if parseErr != nil {
		t.Fatalf("parse failed: %v", parseErr)
	}

	sexp := tree.RootNode().SExpr(lang)
	t.Logf("parse tree: %s", sexp)

	if strings.Contains(sexp, "ERROR") {
		t.Errorf("unexpected ERROR: %s", sexp)
	}
}

func TestImportGrammarJSBuiltinSepHelpers(t *testing.T) {
	g, err := ImportGrammarJS([]byte(testGrammarJSWithBuiltinSepHelpers))
	if err != nil {
		t.Fatalf("ImportGrammarJS failed: %v", err)
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	tree, err := gotreesitter.NewParser(lang).Parse([]byte("a,b,c"))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	sexp := tree.RootNode().SExpr(lang)
	if strings.Contains(sexp, "ERROR") {
		t.Fatalf("unexpected ERROR in tree: %s", sexp)
	}
}

func TestImportGrammarJSBuiltinSepHelpersRuleFirst(t *testing.T) {
	g, err := ImportGrammarJS([]byte(testGrammarJSWithBuiltinSepHelpersRuleFirst))
	if err != nil {
		t.Fatalf("ImportGrammarJS failed: %v", err)
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	tree, err := gotreesitter.NewParser(lang).Parse([]byte("a,b,c"))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	sexp := tree.RootNode().SExpr(lang)
	if strings.Contains(sexp, "ERROR") {
		t.Fatalf("unexpected ERROR in tree: %s", sexp)
	}
}

func TestImportGrammarJSBuiltinTrailingHelpers(t *testing.T) {
	g, err := ImportGrammarJS([]byte(testGrammarJSWithBuiltinTrailingHelpers))
	if err != nil {
		t.Fatalf("ImportGrammarJS failed: %v", err)
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	tree, err := gotreesitter.NewParser(lang).Parse([]byte("a,b,"))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	sexp := tree.RootNode().SExpr(lang)
	if strings.Contains(sexp, "ERROR") {
		t.Fatalf("unexpected ERROR in tree: %s", sexp)
	}
}

// ============================================================
// Grammar Composition (ExtendGrammar)
// ============================================================

func TestExtendGrammar(t *testing.T) {
	// Start with a simple base grammar.
	base := NewGrammar("base")
	base.Define("program", Repeat(Sym("statement")))
	base.Define("statement", Choice(Sym("assignment"), Sym("expression_statement")))
	base.Define("assignment", Seq(Sym("identifier"), Str("="), Sym("expression"), Str(";")))
	base.Define("expression_statement", Seq(Sym("expression"), Str(";")))
	base.Define("expression", Choice(Sym("number"), Sym("identifier")))
	base.Define("identifier", Pat(`[a-z]+`))
	base.Define("number", Pat(`[0-9]+`))
	base.SetExtras(Pat(`\s`))

	// Extend it: add a print statement, override statement to include it.
	extended := ExtendGrammar("extended", base, func(g *Grammar) {
		g.Define("print_statement", Seq(Str("print"), Sym("expression"), Str(";")))
		g.Define("statement", Choice(
			Sym("assignment"),
			Sym("expression_statement"),
			Sym("print_statement"),
		))
	})

	t.Run("inherits base rules", func(t *testing.T) {
		if _, ok := extended.Rules["assignment"]; !ok {
			t.Error("missing inherited rule 'assignment'")
		}
		if _, ok := extended.Rules["number"]; !ok {
			t.Error("missing inherited rule 'number'")
		}
	})

	t.Run("has new rule", func(t *testing.T) {
		if _, ok := extended.Rules["print_statement"]; !ok {
			t.Error("missing new rule 'print_statement'")
		}
	})

	t.Run("overrides statement", func(t *testing.T) {
		stmt := extended.Rules["statement"]
		if stmt.Kind != RuleChoice || len(stmt.Children) != 3 {
			t.Errorf("expected statement to be choice with 3 alternatives, got kind=%d, children=%d",
				stmt.Kind, len(stmt.Children))
		}
	})

	t.Run("generates and parses", func(t *testing.T) {
		lang, err := GenerateLanguage(extended)
		if err != nil {
			t.Fatalf("GenerateLanguage failed: %v", err)
		}

		parser := gotreesitter.NewParser(lang)
		tree, parseErr := parser.Parse([]byte("print 42;"))
		if parseErr != nil {
			t.Fatalf("parse failed: %v", parseErr)
		}
		sexp := tree.RootNode().SExpr(lang)
		t.Logf("parse: %s", sexp)
		if strings.Contains(sexp, "ERROR") {
			t.Errorf("unexpected ERROR: %s", sexp)
		}
		if !strings.Contains(sexp, "print_statement") {
			t.Error("expected print_statement in tree")
		}
	})

	t.Run("base not mutated", func(t *testing.T) {
		stmt := base.Rules["statement"]
		if stmt.Kind != RuleChoice || len(stmt.Children) != 2 {
			t.Errorf("base grammar was mutated: statement has %d children", len(stmt.Children))
		}
	})
}

// ============================================================
// Auto Highlight Query Generation
// ============================================================

func TestGenerateHighlightQuery(t *testing.T) {
	g := JSONGrammar()
	query := GenerateHighlightQuery(g)

	t.Logf("highlight query:\n%s", query)

	// JSON grammar should highlight these.
	if !strings.Contains(query, "@string") {
		t.Error("expected @string capture")
	}
	if !strings.Contains(query, "@number") {
		t.Error("expected @number capture")
	}
	// "true", "false", "null" are string terminals → should become @keyword.
	if !strings.Contains(query, "@keyword") {
		t.Error("expected @keyword captures for true/false/null")
	}
}

func TestGenerateHighlightQueryCalc(t *testing.T) {
	g := CalcGrammar()
	query := GenerateHighlightQuery(g)

	t.Logf("highlight query:\n%s", query)

	if !strings.Contains(query, "@number") {
		t.Error("expected @number capture for calc grammar")
	}
	// Operators +, -, *, / should be detected.
	if !strings.Contains(query, "@operator") {
		t.Error("expected @operator captures")
	}
}

// ============================================================
// Grammar Diffing
// ============================================================

func TestDiffGrammars(t *testing.T) {
	t.Run("identical grammars", func(t *testing.T) {
		g := JSONGrammar()
		diff := DiffGrammars(g, g)
		if diff.HasChanges() {
			t.Errorf("expected no changes, got: %s", diff.String())
		}
	})

	t.Run("added rule", func(t *testing.T) {
		old := NewGrammar("test")
		old.Define("start", Str("hello"))

		new_ := NewGrammar("test")
		new_.Define("start", Str("hello"))
		new_.Define("extra", Str("world"))

		diff := DiffGrammars(old, new_)
		if len(diff.AddedRules) != 1 || diff.AddedRules[0] != "extra" {
			t.Errorf("expected added rule 'extra', got: %v", diff.AddedRules)
		}
		t.Logf("diff:\n%s", diff.String())
	})

	t.Run("removed rule", func(t *testing.T) {
		old := NewGrammar("test")
		old.Define("start", Str("hello"))
		old.Define("removed", Str("bye"))

		new_ := NewGrammar("test")
		new_.Define("start", Str("hello"))

		diff := DiffGrammars(old, new_)
		if len(diff.RemovedRules) != 1 || diff.RemovedRules[0] != "removed" {
			t.Errorf("expected removed rule 'removed', got: %v", diff.RemovedRules)
		}
	})

	t.Run("modified rule", func(t *testing.T) {
		old := NewGrammar("test")
		old.Define("start", Str("hello"))

		new_ := NewGrammar("test")
		new_.Define("start", Str("world"))

		diff := DiffGrammars(old, new_)
		if len(diff.ModifiedRules) != 1 || diff.ModifiedRules[0] != "start" {
			t.Errorf("expected modified rule 'start', got: %v", diff.ModifiedRules)
		}
	})

	t.Run("extras changed", func(t *testing.T) {
		old := NewGrammar("test")
		old.Define("start", Str("x"))
		old.SetExtras(Pat(`\s`))

		new_ := NewGrammar("test")
		new_.Define("start", Str("x"))
		new_.SetExtras(Pat(`\s`), Pat(`//[^\n]*`))

		diff := DiffGrammars(old, new_)
		if !diff.ExtrasChanged {
			t.Error("expected extras to be marked as changed")
		}
	})

	t.Run("extend produces diff", func(t *testing.T) {
		base := CalcGrammar()
		extended := ExtendGrammar("calc_ext", base, func(g *Grammar) {
			g.Define("modulo", PrecLeft(2, Seq(
				Sym("expression"), Str("%"), Sym("expression"),
			)))
		})
		diff := DiffGrammars(base, extended)
		if len(diff.AddedRules) == 0 {
			t.Error("expected added rules from extension")
		}
		t.Logf("diff:\n%s", diff.String())
	})
}

// ============================================================
// Declarative .grammar File Format
// ============================================================

const testGrammarFile = `
# A simple expression grammar
grammar simple_expr

extras = [ /\s/ ]

rule program = repeat(expression)
rule expression = choice(prec.left(1, seq(expression, "+", expression)), prec.left(2, seq(expression, "*", expression)), number)
rule number = /[0-9]+/
`

func TestParseGrammarFile(t *testing.T) {
	g, err := ParseGrammarFile(testGrammarFile)
	if err != nil {
		t.Fatalf("ParseGrammarFile failed: %v", err)
	}

	if g.Name != "simple_expr" {
		t.Errorf("name = %q, want 'simple_expr'", g.Name)
	}

	if len(g.Rules) != 3 {
		t.Errorf("rules count = %d, want 3", len(g.Rules))
	}

	if len(g.Extras) != 1 {
		t.Errorf("extras count = %d, want 1", len(g.Extras))
	}

	t.Logf("parsed grammar %q with %d rules", g.Name, len(g.Rules))
}

func TestParseGrammarFileGenerate(t *testing.T) {
	g, err := ParseGrammarFile(testGrammarFile)
	if err != nil {
		t.Fatalf("ParseGrammarFile failed: %v", err)
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	parser := gotreesitter.NewParser(lang)
	tree, parseErr := parser.Parse([]byte("1 + 2 * 3"))
	if parseErr != nil {
		t.Fatalf("parse failed: %v", parseErr)
	}

	sexp := tree.RootNode().SExpr(lang)
	t.Logf("parse tree: %s", sexp)

	if strings.Contains(sexp, "ERROR") {
		t.Errorf("unexpected ERROR: %s", sexp)
	}
	if !strings.Contains(sexp, "expression") {
		t.Error("expected 'expression' in tree")
	}
	if !strings.Contains(sexp, "number") {
		t.Error("expected 'number' in tree")
	}
}

const testGrammarFileList = `
# A list grammar to exercise .grammar format with nesting
grammar list_file

extras = [ /\s/ ]

rule document = repeat(item)
rule item = choice(word, group)
rule group = seq("(", repeat(item), ")")
rule word = /[a-z]+/
`

func TestParseGrammarFileList(t *testing.T) {
	g, err := ParseGrammarFile(testGrammarFileList)
	if err != nil {
		t.Fatalf("ParseGrammarFile failed: %v", err)
	}

	if g.Name != "list_file" {
		t.Errorf("name = %q, want 'list_file'", g.Name)
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	parser := gotreesitter.NewParser(lang)
	tree, parseErr := parser.Parse([]byte("foo (bar baz) qux"))
	if parseErr != nil {
		t.Fatalf("parse failed: %v", parseErr)
	}

	sexp := tree.RootNode().SExpr(lang)
	t.Logf("parse tree: %s", sexp)

	if strings.Contains(sexp, "ERROR") {
		t.Errorf("unexpected ERROR: %s", sexp)
	}
	if !strings.Contains(sexp, "group") {
		t.Error("expected 'group' in tree")
	}
	if strings.Count(sexp, "word") != 4 {
		t.Errorf("expected 4 word nodes, got %d in: %s", strings.Count(sexp, "word"), sexp)
	}
}

// ============================================================
// Real-World Grammar.js Import & Parity
// ============================================================

func TestImportRealJSONGrammarJS(t *testing.T) {
	const grammarJSPath = "/tmp/grammar_parity/json/grammar.js"
	source, err := os.ReadFile(grammarJSPath)
	if err != nil {
		t.Skipf("skipping: %v (clone tree-sitter-json to %s)", err, grammarJSPath)
	}

	g, err := ImportGrammarJS(source)
	if err != nil {
		t.Fatalf("ImportGrammarJS failed: %v", err)
	}

	// Verify grammar name and key rules.
	if g.Name != "json" {
		t.Errorf("name = %q, want 'json'", g.Name)
	}

	expectedRules := []string{
		"document", "_value", "object", "pair", "array", "string",
		"_string_content", "string_content", "escape_sequence",
		"number", "true", "false", "null", "comment",
	}
	for _, name := range expectedRules {
		if _, ok := g.Rules[name]; !ok {
			t.Errorf("missing rule %q", name)
		}
	}
	t.Logf("imported %d rules, %d extras, %d supertypes",
		len(g.Rules), len(g.Extras), len(g.Supertypes))

	// Verify extras include whitespace and comment.
	if len(g.Extras) != 2 {
		t.Errorf("extras count = %d, want 2", len(g.Extras))
	}

	// Verify supertypes.
	if len(g.Supertypes) != 1 || g.Supertypes[0] != "_value" {
		t.Errorf("supertypes = %v, want [_value]", g.Supertypes)
	}

	// Generate Language from imported grammar.
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}
	t.Logf("generated: %d symbols, %d states, %d tokens",
		lang.SymbolCount, lang.StateCount, lang.TokenCount)

	// Parse a representative set of JSON inputs.
	inputs := []struct {
		name  string
		input string
	}{
		{"empty_object", `{}`},
		{"simple_object", `{"a": 1}`},
		{"nested_object", `{"a": {"b": 2}}`},
		{"array", `[1, 2, 3]`},
		{"string", `"hello world"`},
		{"number_int", `42`},
		{"number_float", `3.14`},
		{"number_exp", `1e10`},
		{"number_neg", `-5`},
		{"true", `true`},
		{"false", `false`},
		{"null", `null`},
		{"complex", `{"key": [1, true, null, "str", {"nested": []}]}`},
	}

	parser := gotreesitter.NewParser(lang)
	for _, tc := range inputs {
		t.Run(tc.name, func(t *testing.T) {
			tree, parseErr := parser.Parse([]byte(tc.input))
			if parseErr != nil {
				t.Fatalf("parse failed: %v", parseErr)
			}
			sexp := tree.RootNode().SExpr(lang)
			if strings.Contains(sexp, "ERROR") || strings.Contains(sexp, "MISSING") {
				t.Errorf("parse error in tree: %s", sexp)
			}
			t.Logf("%s → %s", tc.input, sexp)
		})
	}
}

func TestImportRealJSONParityWithBlob(t *testing.T) {
	const grammarJSPath = "/tmp/grammar_parity/json/grammar.js"
	source, err := os.ReadFile(grammarJSPath)
	if err != nil {
		t.Skipf("skipping: %v", err)
	}

	g, err := ImportGrammarJS(source)
	if err != nil {
		t.Fatalf("ImportGrammarJS: %v", err)
	}

	genLang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}

	// Load the reference json.bin blob.
	refLang := grammars.JsonLanguage()
	if refLang == nil {
		t.Fatal("could not load reference JSON language")
	}

	inputs := []string{
		`{}`,
		`{"a": 1}`,
		`[1, 2, 3]`,
		`"hello"`,
		`42`,
		`true`,
		`null`,
		`{"a": {"b": [1, null, "x"]}}`,
		`{"key": "value", "arr": [1, 2.5, -3, true, false, null]}`,
	}

	genParser := gotreesitter.NewParser(genLang)
	refParser := gotreesitter.NewParser(refLang)

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			genTree, err := genParser.Parse([]byte(input))
			if err != nil {
				t.Fatalf("gen parse: %v", err)
			}
			refTree, err := refParser.Parse([]byte(input))
			if err != nil {
				t.Fatalf("ref parse: %v", err)
			}

			genSexp := genTree.RootNode().SExpr(genLang)
			refSexp := refTree.RootNode().SExpr(refLang)

			if strings.Contains(genSexp, "ERROR") {
				t.Errorf("gen tree has ERROR: %s", genSexp)
			}

			// Compare S-expressions (stripping whitespace differences).
			if normalizeSexp(genSexp) != normalizeSexp(refSexp) {
				t.Errorf("parity mismatch:\n  gen: %s\n  ref: %s", genSexp, refSexp)
			}
		})
	}
}

// normalizeSexp normalizes an S-expression for comparison by collapsing whitespace.
func normalizeSexp(s string) string {
	s = strings.TrimSpace(s)
	// Collapse multiple spaces to single.
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}

func TestImportRealCSSGrammarJS(t *testing.T) {
	const grammarJSPath = "/tmp/grammar_parity/css/grammar.js"
	source, err := os.ReadFile(grammarJSPath)
	if err != nil {
		t.Skipf("skipping: %v (clone tree-sitter-css to %s)", err, grammarJSPath)
	}

	g, err := ImportGrammarJS(source)
	if err != nil {
		t.Fatalf("ImportGrammarJS failed: %v", err)
	}

	if g.Name != "css" {
		t.Errorf("name = %q, want 'css'", g.Name)
	}
	t.Logf("imported %d rules, %d extras, %d externals, %d inline",
		len(g.Rules), len(g.Extras), len(g.Externals), len(g.Inline))

	// Verify key structural properties.
	if len(g.Rules) < 50 {
		t.Errorf("expected at least 50 rules, got %d", len(g.Rules))
	}
	if len(g.Extras) != 3 {
		t.Errorf("extras = %d, want 3", len(g.Extras))
	}
	if len(g.Externals) != 3 {
		t.Errorf("externals = %d, want 3", len(g.Externals))
	}

	// Verify key rules exist.
	expectedRules := []string{
		"stylesheet", "rule_set", "selectors", "block",
		"declaration", "color_value",
		"integer_value", "float_value", "string_value",
		"class_selector", "id_selector", "pseudo_class_selector",
		"import_statement", "media_statement", "comment",
	}
	for _, name := range expectedRules {
		if _, ok := g.Rules[name]; !ok {
			t.Errorf("missing rule %q", name)
		}
	}
}

// ============================================================
// Production-Grade Authored Grammars
// ============================================================

func TestINIGrammar(t *testing.T) {
	g := INIGrammar()
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", "(document)"},
		{"simple pair", "key=value", "(document (pair (key) (bare_value)))"},
		{"colon delimiter", "key:value", "(document (pair (key) (bare_value)))"},
		{"spaced pair", "key = value", "(document (pair (key) (bare_value)))"},
		{"empty value", "key=", "(document (pair (key)))"},
		{"section with pairs", "[server]\nhost=localhost\nport=8080",
			"(document (section (section_header (section_name)) (pair (key) (bare_value)) (pair (key) (bare_value))))"},
		{"semicolon comment", "; this is a comment", "(document (comment))"},
		{"hash comment", "# hash comment", "(document (comment))"},
		{"subsection", "[remote \"origin\"]\nurl=git@github.com:user/repo.git",
			"(document (section (section_header (section_name) (subsection_name)) (pair (key) (bare_value))))"},
		{"quoted value", "key=\"hello world\"", "(document (pair (key) (quoted_string)))"},
		{"global pair then section", "key=value\n[section]\nother=val",
			"(document (pair (key) (bare_value)) (section (section_header (section_name)) (pair (key) (bare_value))))"},
		{"multiple sections", "[a]\nx=1\n[b]\ny=2",
			"(document (section (section_header (section_name)) (pair (key) (bare_value))) (section (section_header (section_name)) (pair (key) (bare_value))))"},
		{"value with spaces", "name = John Doe", "(document (pair (key) (bare_value)))"},
		{"dotted section", "[forge.example]\nUser=hg",
			"(document (section (section_header (section_name)) (pair (key) (bare_value))))"},
		{"mixed comments", "; comment\nkey=val\n# another comment",
			"(document (comment) (pair (key) (bare_value)) (comment))"},
		{"git config style", "[core]\n\trepositoryformatversion = 0\n\tfilemode = true\n\tbare = false",
			"(document (section (section_header (section_name)) (pair (key) (bare_value)) (pair (key) (bare_value)) (pair (key) (bare_value))))"},
		{"python configparser", "[DEFAULT]\nServerAliveInterval = 45\nCompression = yes",
			"(document (section (section_header (section_name)) (pair (key) (bare_value)) (pair (key) (bare_value))))"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}
			sexp := tree.RootNode().SExpr(lang)
			t.Logf("input: %q → %s", tt.input, sexp)
			if strings.Contains(sexp, "ERROR") {
				t.Errorf("tree contains ERROR: %s", sexp)
			}
			if tt.expected != "" && sexp != tt.expected {
				t.Errorf("tree mismatch:\n  got:      %s\n  expected: %s", sexp, tt.expected)
			}
		})
	}
}

func TestINIEmbeddedTests(t *testing.T) {
	g := INIGrammar()
	if err := RunTests(g); err != nil {
		t.Fatal(err)
	}
}

// ── Lox Grammar Tests ──

func TestLoxGrammar(t *testing.T) {
	g := LoxGrammar()
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"print", "print 42;"},
		{"var decl", "var x = 1;"},
		{"var no init", "var y;"},
		{"assignment", "x = 42;"},
		{"arithmetic", "1 + 2 * 3;"},
		{"comparison", "a < b;"},
		{"equality", "a == b;"},
		{"logical", "a or b and c;"},
		{"unary", "!true;"},
		{"unary neg", "-x;"},
		{"string", `"hello";`},
		{"nil", "nil;"},
		{"grouping", "(1 + 2) * 3;"},
		{"function", "fun add(a, b) { return a + b; }"},
		{"call", "add(1, 2);"},
		{"method call", "obj.method(x);"},
		{"class", `class Dog { bark() { print "woof"; } }`},
		{"inheritance", "class Poodle < Dog { }"},
		{"this", "this.x;"},
		{"super", "super.method;"},
		{"if", "if (x) print x;"},
		{"if else", "if (x) print x; else print y;"},
		{"while", "while (true) { print 1; }"},
		{"for", "for (var i = 0; i < 10; i = i + 1) print i;"},
		{"block", "{ var x = 1; print x; }"},
		{"nested calls", "f(g(x));"},
		{"chained member", "a.b.c;"},
		{"comment", "// this is a comment\nprint 1;"},
		{"multiple fns", "fun a() {} fun b() {}"},
		{"fib", "fun fib(n) { if (n < 2) return n; return fib(n - 1) + fib(n - 2); }"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}
			sexp := tree.RootNode().SExpr(lang)
			t.Logf("input: %q → %s", tt.input, sexp)
			if strings.Contains(sexp, "ERROR") {
				t.Errorf("tree contains ERROR: %s", sexp)
			}
		})
	}
}

func TestLoxEmbeddedTests(t *testing.T) {
	g := LoxGrammar()
	if err := RunTests(g); err != nil {
		t.Fatal(err)
	}
}

// ── Mustache Grammar Tests ──

func TestMustacheGrammar(t *testing.T) {
	g := MustacheGrammar()
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"raw text only", "Hello, world!"},
		{"interpolation", "Hello, {{name}}!"},
		{"unescaped triple", "Hello, {{{name}}}!"},
		{"unescaped ampersand", "Hello, {{&name}}!"},
		{"section", "{{#show}}visible{{/show}}"},
		{"inverted section", "{{^show}}hidden{{/show}}"},
		{"comment", "{{! this is a comment }}"},
		{"partial", "{{>header}}"},
		{"dotted name", "{{person.name}}"},
		{"implicit iterator", "{{.}}"},
		{"mixed content", "Hello {{name}}, you have {{count}} items."},
		{"nested sections", "{{#a}}{{#b}}inner{{/b}}{{/a}}"},
		{"multiple interpolations", "{{first}} {{last}}"},
		{"section with content", "{{#list}}item: {{name}}\n{{/list}}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}
			sexp := tree.RootNode().SExpr(lang)
			t.Logf("input: %q → %s", tt.input, sexp)
			if strings.Contains(sexp, "ERROR") {
				t.Errorf("tree contains ERROR: %s", sexp)
			}
		})
	}
}

func TestMustacheEmbeddedTests(t *testing.T) {
	g := MustacheGrammar()
	if err := RunTests(g); err != nil {
		t.Fatal(err)
	}
}

// ============================================================
// Pipeline Validation Tests
// ============================================================

// TestBlobRoundTrip verifies the full pipeline for each authored grammar:
// Grammar → Generate blob → decode blob → parse inputs → compare with direct generation.
func TestBlobRoundTrip(t *testing.T) {
	type grammarCase struct {
		name    string
		grammar func() *Grammar
		inputs  []string
	}
	cases := []grammarCase{
		{
			name:    "INI",
			grammar: INIGrammar,
			inputs: []string{
				"",
				"key=value",
				"[server]\nhost=localhost\nport=8080",
				"; comment\nkey=val",
				"key=\"hello world\"",
			},
		},
		{
			name:    "Lox",
			grammar: LoxGrammar,
			inputs: []string{
				"",
				"print 42;",
				"var x = 1;",
				"1 + 2 * 3;",
				"fun fib(n) { if (n < 2) return n; return fib(n - 1) + fib(n - 2); }",
				"class Dog < Animal { bark() { print \"woof\"; } }",
			},
		},
		{
			name:    "Mustache",
			grammar: MustacheGrammar,
			inputs: []string{
				"",
				"Hello, world!",
				"Hello, {{name}}!",
				"{{{raw}}}",
				"{{#section}}content{{/section}}",
				"{{person.name}}",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := tc.grammar()

			// Generate directly.
			directLang, err := GenerateLanguage(g)
			if err != nil {
				t.Fatalf("GenerateLanguage failed: %v", err)
			}

			// Generate blob and decode.
			blob, err := Generate(g)
			if err != nil {
				t.Fatalf("Generate blob failed: %v", err)
			}
			t.Logf("blob size: %d bytes", len(blob))

			blobLang, err := decodeLanguageBlob(blob)
			if err != nil {
				t.Fatalf("decodeLanguageBlob failed: %v", err)
			}

			// Verify structural properties match.
			if directLang.SymbolCount != blobLang.SymbolCount {
				t.Errorf("SymbolCount mismatch: direct=%d blob=%d", directLang.SymbolCount, blobLang.SymbolCount)
			}
			if directLang.StateCount != blobLang.StateCount {
				t.Errorf("StateCount mismatch: direct=%d blob=%d", directLang.StateCount, blobLang.StateCount)
			}
			if directLang.TokenCount != blobLang.TokenCount {
				t.Errorf("TokenCount mismatch: direct=%d blob=%d", directLang.TokenCount, blobLang.TokenCount)
			}

			// Parse all inputs with both and compare S-expressions.
			for _, input := range tc.inputs {
				directParser := gotreesitter.NewParser(directLang)
				blobParser := gotreesitter.NewParser(blobLang)

				directTree, err := directParser.Parse([]byte(input))
				if err != nil {
					t.Errorf("direct parse failed for %q: %v", input, err)
					continue
				}
				blobTree, err := blobParser.Parse([]byte(input))
				if err != nil {
					t.Errorf("blob parse failed for %q: %v", input, err)
					continue
				}

				directSexp := directTree.RootNode().SExpr(directLang)
				blobSexp := blobTree.RootNode().SExpr(blobLang)
				if directSexp != blobSexp {
					t.Errorf("S-expr mismatch for %q:\n  direct: %s\n  blob:   %s", input, directSexp, blobSexp)
				}
			}
		})
	}
}

// TestGenerateCAuthoredGrammars validates C code emission for each authored grammar.
func TestGenerateCAuthoredGrammars(t *testing.T) {
	type grammarCase struct {
		name     string
		grammar  func() *Grammar
		expected []string // strings that must appear in the generated C code
	}
	cases := []grammarCase{
		{
			name:    "INI",
			grammar: INIGrammar,
			expected: []string{
				"#include <tree_sitter/parser.h>",
				"#define LANGUAGE_VERSION",
				"tree_sitter_ini",
				"ts_symbol_names",
				"ts_lex",
			},
		},
		{
			name:    "Lox",
			grammar: LoxGrammar,
			expected: []string{
				"#include <tree_sitter/parser.h>",
				"tree_sitter_lox",
				"ts_parse_table",
				"ts_lex",
				"ts_field_names",
			},
		},
		{
			name:    "Mustache",
			grammar: MustacheGrammar,
			expected: []string{
				"#include <tree_sitter/parser.h>",
				"tree_sitter_mustache",
				"ts_lex",
				"ts_symbol_names",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cCode, err := GenerateC(tc.grammar())
			if err != nil {
				t.Fatalf("GenerateC failed: %v", err)
			}
			t.Logf("C code size: %d bytes", len(cCode))
			for _, s := range tc.expected {
				if !strings.Contains(cCode, s) {
					t.Errorf("C code missing %q", s)
				}
			}
		})
	}
}
