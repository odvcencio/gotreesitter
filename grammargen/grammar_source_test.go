package grammargen

import (
	"slices"
	"strings"
	"testing"
)

func TestExtendGrammarCopiesImportMetadata(t *testing.T) {
	base := NewGrammar("base")
	base.Define("program", Sym("identifier"))
	base.Define("identifier", Pat(`[a-z]+`))
	base.SetExtras(Pat(`\s+`))
	base.SetConflicts([]string{"program", "identifier"})
	base.SetExternals(Sym("_external_token"))
	base.SetInline("identifier")
	base.SetWord("identifier")
	base.SetSupertypes("program")
	base.ReservedWordSets = []ReservedWordSet{
		{Name: "global", Rules: []*Rule{Str("if"), Str("else")}},
	}
	base.Precedences = [][]PrecEntry{
		{{Name: "call"}, {IsSymbol: true, Name: "program"}},
	}
	base.EnableLRSplitting = true
	base.BinaryRepeatMode = true
	base.FlattenGeneratedRepeatAux = true
	base.ReuseRepeatAuxForParents = []string{"program", "statement_list"}
	base.PreferExpressionOperatorIdentifierReduces = true
	base.PreferParenthesizedCallDoBlockReduces = true
	base.PreferRemoteCallOperatorReduces = true
	base.PreferStabClauseLeftArrowReduces = true
	base.PreferPreciseExternalLexStates = true
	base.CompactParseStates = true
	base.ChoiceLiftThreshold = 16
	base.ExactPrefixStates = 2048
	base.PreserveHiddenChoicePassthrough = []string{"_capture_expression"}
	base.Test("identifier", "abc", "")

	extended := ExtendGrammar("extended", base, func(g *Grammar) {
		g.Rules["identifier"].Value = `[A-Z]+`
		g.Extras[0].Value = `[ \t]+`
		g.Externals[0].Value = "_different_external"
		g.ReservedWordSets[0].Rules[0].Value = "when"
		g.Precedences[0][0].Name = "member"
	})

	if extended.Name != "extended" {
		t.Fatalf("extended.Name = %q, want extended", extended.Name)
	}
	if !extended.EnableLRSplitting || !extended.BinaryRepeatMode {
		t.Fatalf("extension did not inherit generator mode flags")
	}
	if !extended.FlattenGeneratedRepeatAux {
		t.Fatalf("extension did not inherit generated repeat flattening flag")
	}
	if !extended.PreferExpressionOperatorIdentifierReduces {
		t.Fatalf("extension did not inherit operator-identifier reduce preference flag")
	}
	if !extended.PreferParenthesizedCallDoBlockReduces {
		t.Fatalf("extension did not inherit parenthesized-call do-block reduce preference flag")
	}
	if !extended.PreferRemoteCallOperatorReduces {
		t.Fatalf("extension did not inherit remote-call operator reduce preference flag")
	}
	if !extended.PreferStabClauseLeftArrowReduces {
		t.Fatalf("extension did not inherit stab-clause-left arrow reduce preference flag")
	}
	if !extended.PreferPreciseExternalLexStates {
		t.Fatalf("extension did not inherit precise external lex-state preference flag")
	}
	if !extended.CompactParseStates {
		t.Fatalf("extension did not inherit compact parser-state preference flag")
	}
	if !slices.Equal(extended.ReuseRepeatAuxForParents, []string{"program", "statement_list"}) {
		t.Fatalf("ReuseRepeatAuxForParents = %v, want [program statement_list]", extended.ReuseRepeatAuxForParents)
	}
	if extended.ChoiceLiftThreshold != 16 {
		t.Fatalf("ChoiceLiftThreshold = %d, want 16", extended.ChoiceLiftThreshold)
	}
	if extended.ExactPrefixStates != 2048 {
		t.Fatalf("ExactPrefixStates = %d, want 2048", extended.ExactPrefixStates)
	}
	if !slices.Equal(extended.PreserveHiddenChoicePassthrough, []string{"_capture_expression"}) {
		t.Fatalf("PreserveHiddenChoicePassthrough = %v, want [_capture_expression]", extended.PreserveHiddenChoicePassthrough)
	}
	if got := extended.Word; got != "identifier" {
		t.Fatalf("Word = %q, want identifier", got)
	}
	if !slices.Equal(extended.Inline, []string{"identifier"}) {
		t.Fatalf("Inline = %v, want [identifier]", extended.Inline)
	}
	if !slices.Equal(extended.Supertypes, []string{"program"}) {
		t.Fatalf("Supertypes = %v, want [program]", extended.Supertypes)
	}
	if len(extended.Tests) != 1 || extended.Tests[0].Name != "identifier" {
		t.Fatalf("Tests = %+v, want copied identifier test", extended.Tests)
	}

	if base.Rules["identifier"].Value != `[a-z]+` {
		t.Fatalf("base rule was mutated through extension: %q", base.Rules["identifier"].Value)
	}
	if base.Extras[0].Value != `\s+` {
		t.Fatalf("base extra was mutated through extension: %q", base.Extras[0].Value)
	}
	if base.Externals[0].Value != "_external_token" {
		t.Fatalf("base external was mutated through extension: %q", base.Externals[0].Value)
	}
	if base.ReservedWordSets[0].Rules[0].Value != "if" {
		t.Fatalf("base reserved word was mutated through extension: %q", base.ReservedWordSets[0].Rules[0].Value)
	}
	if base.Precedences[0][0].Name != "call" {
		t.Fatalf("base precedence was mutated through extension: %q", base.Precedences[0][0].Name)
	}
}

func TestEmitGrammarGoIncludesImportMetadata(t *testing.T) {
	g := NewGrammar("metadata")
	g.Define("program", Sym("identifier"))
	g.Define("identifier", Pat(`[a-z]+`))
	g.ReservedWordSets = []ReservedWordSet{
		{Name: "global", Rules: []*Rule{Str("if")}},
	}
	g.Precedences = [][]PrecEntry{
		{{Name: "call"}, {IsSymbol: true, Name: "program"}},
	}
	g.EnableLRSplitting = true
	g.BinaryRepeatMode = true
	g.FlattenGeneratedRepeatAux = true
	g.ReuseRepeatAuxForParents = []string{"program"}
	g.PreferExpressionOperatorIdentifierReduces = true
	g.PreferParenthesizedCallDoBlockReduces = true
	g.PreferRemoteCallOperatorReduces = true
	g.PreferStabClauseLeftArrowReduces = true
	g.PreferPreciseExternalLexStates = true
	g.CompactParseStates = true
	g.ChoiceLiftThreshold = 8
	g.ExactPrefixStates = 2048
	g.PreserveHiddenChoicePassthrough = []string{"_capture_expression"}
	g.Test("valid identifier", "abc", "(program (identifier))")
	g.TestError("invalid identifier", "123")

	source, err := EmitGrammarGo(g, "grammargen", "MetadataGrammar")
	if err != nil {
		t.Fatalf("EmitGrammarGo: %v", err)
	}
	text := string(source)
	for _, want := range []string{
		"g.ReservedWordSets = []ReservedWordSet{",
		"g.Precedences = [][]PrecEntry{",
		"{IsSymbol: true, Name: \"program\"}",
		"g.EnableLRSplitting = true",
		"g.BinaryRepeatMode = true",
		"g.FlattenGeneratedRepeatAux = true",
		"g.ReuseRepeatAuxForParents = []string{",
		"\"program\"",
		"g.PreferExpressionOperatorIdentifierReduces = true",
		"g.PreferParenthesizedCallDoBlockReduces = true",
		"g.PreferRemoteCallOperatorReduces = true",
		"g.PreferStabClauseLeftArrowReduces = true",
		"g.PreferPreciseExternalLexStates = true",
		"g.CompactParseStates = true",
		"g.ChoiceLiftThreshold = 8",
		"g.ExactPrefixStates = 2048",
		"g.PreserveHiddenChoicePassthrough = []string{",
		"\"_capture_expression\"",
		"g.Test(\"valid identifier\", \"abc\", \"(program (identifier))\")",
		"g.TestError(\"invalid identifier\", \"123\")",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("emitted source missing %q:\n%s", want, text)
		}
	}
}

func TestExpandInlineRulesPreservesHiddenChoicePassthrough(t *testing.T) {
	g := NewGrammar("metadata")
	g.Define("program", Sym("inline_rule"))
	g.Define("inline_rule", Sym("_capture_expression"))
	g.Define("_capture_expression", Choice(Sym("identifier"), Seq(Sym("&"), Sym("identifier"))))
	g.Define("identifier", Pat(`[a-z]+`))
	g.SetInline("inline_rule")
	g.PreserveHiddenChoicePassthrough = []string{"_capture_expression"}

	expanded := expandInlineRules(g)
	if !slices.Equal(expanded.PreserveHiddenChoicePassthrough, []string{"_capture_expression"}) {
		t.Fatalf("PreserveHiddenChoicePassthrough = %v, want [_capture_expression]", expanded.PreserveHiddenChoicePassthrough)
	}
}

func TestImportedKotlinSwiftGrammarConstructors(t *testing.T) {
	kotlin := KotlinGrammar()
	if kotlin.Name != "kotlin" {
		t.Fatalf("KotlinGrammar().Name = %q, want kotlin", kotlin.Name)
	}
	if len(kotlin.Rules) != 201 {
		t.Fatalf("KotlinGrammar rule count = %d, want 201", len(kotlin.Rules))
	}
	if got, want := externalRuleNames(kotlin), []string{
		"_automatic_semicolon",
		"_import_list_delimiter",
		"safe_nav",
		"multiline_comment",
		"_string_start",
		"_string_end",
		"string_content",
		"_primary_constructor_keyword",
		"_import_dot",
	}; !slices.Equal(got, want) {
		t.Fatalf("KotlinGrammar externals = %v, want %v", got, want)
	}

	swift := SwiftGrammar()
	if swift.Name != "swift" {
		t.Fatalf("SwiftGrammar().Name = %q, want swift", swift.Name)
	}
	// Match the grammar locked at 00bbb0a2550f8bc0023a2a4992922d51ae045626.
	if len(swift.Rules) != 306 {
		t.Fatalf("SwiftGrammar rule count = %d, want 306", len(swift.Rules))
	}
	if got, want := externalRuleNames(swift), []string{
		"multiline_comment",
		"raw_str_part",
		"raw_str_continuing_indicator",
		"raw_str_end_part",
		"_implicit_semi",
		"_explicit_semi",
		"_arrow_operator_custom",
		"_dot_custom",
		"_conjunction_operator_custom",
		"_disjunction_operator_custom",
		"_nil_coalescing_operator_custom",
		"_double_optional_custom",
		"_eq_custom",
		"_eq_eq_custom",
		"_plus_then_ws",
		"_minus_then_ws",
		"_bang_custom",
		"_throws_keyword",
		"_rethrows_keyword",
		"default_keyword",
		"where_keyword",
		"else",
		"catch_keyword",
		"_as_custom",
		"_as_quest_custom",
		"_as_bang_custom",
		"_async_keyword_custom",
		"_custom_operator",
		"_hash_symbol_custom",
		"_directive_if",
		"_directive_elseif",
		"_directive_else",
		"_directive_endif",
		"_fake_try_bang",
	}; !slices.Equal(got, want) {
		t.Fatalf("SwiftGrammar externals = %v, want %v", got, want)
	}
}

func TestImportedKotlinSwiftGrammarsAreExtendable(t *testing.T) {
	tests := []struct {
		name    string
		grammar *Grammar
	}{
		{name: "kotlin", grammar: KotlinGrammar()},
		{name: "swift", grammar: SwiftGrammar()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			extended := ExtendGrammar(tc.name+"_ext", tc.grammar, func(g *Grammar) {
				g.Define("__got_extension_marker", Str("__got_extension_marker"))
			})
			if extended.Name != tc.name+"_ext" {
				t.Fatalf("extended.Name = %q, want %q", extended.Name, tc.name+"_ext")
			}
			if _, ok := extended.Rules["__got_extension_marker"]; !ok {
				t.Fatalf("extension marker rule was not added")
			}
			if _, ok := tc.grammar.Rules["__got_extension_marker"]; ok {
				t.Fatalf("base grammar was mutated through extension")
			}
		})
	}
}

func TestImportedJavaScriptTypeScriptTSXFortranGrammarConstructors(t *testing.T) {
	tests := []struct {
		name         string
		grammar      *Grammar
		rules        int
		binaryRepeat bool
		externals    []string
	}{
		{
			name:         "javascript",
			grammar:      JavaScriptGrammar(),
			rules:        142,
			binaryRepeat: true,
			externals: []string{
				"_automatic_semicolon",
				"_template_chars",
				"_ternary_qmark",
				"html_comment",
				"||",
				"escape_sequence",
				"regex_pattern",
				"jsx_text",
			},
		},
		{
			name:         "typescript",
			grammar:      TypeScriptGrammar(),
			rules:        229,
			binaryRepeat: true,
			externals: []string{
				"_automatic_semicolon",
				"_template_chars",
				"_ternary_qmark",
				"html_comment",
				"||",
				"escape_sequence",
				"regex_pattern",
				"jsx_text",
				"_function_signature_automatic_semicolon",
				"__error_recovery",
			},
		},
		{
			name:         "tsx",
			grammar:      TSXGrammar(),
			rules:        229,
			binaryRepeat: true,
			externals: []string{
				"_automatic_semicolon",
				"_template_chars",
				"_ternary_qmark",
				"html_comment",
				"||",
				"escape_sequence",
				"regex_pattern",
				"jsx_text",
				"_function_signature_automatic_semicolon",
				"__error_recovery",
			},
		},
		{
			name:         "fortran",
			grammar:      FortranGrammar(),
			rules:        329,
			binaryRepeat: true,
			externals: []string{
				"&",
				"_integer_literal",
				"_float_literal",
				"_boz_literal",
				"_string_literal",
				"_string_literal_kind",
				"_external_end_of_statement",
				"_preproc_unary_operator",
				"hollerith_constant",
				"_do_label",
				"do_label_virtual",
				"_do_label_continue",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.grammar.Name != tc.name {
				t.Fatalf("%s grammar name = %q", tc.name, tc.grammar.Name)
			}
			if len(tc.grammar.Rules) != tc.rules {
				t.Fatalf("%s rule count = %d, want %d", tc.name, len(tc.grammar.Rules), tc.rules)
			}
			if tc.grammar.BinaryRepeatMode != tc.binaryRepeat {
				t.Fatalf("%s BinaryRepeatMode = %v, want %v", tc.name, tc.grammar.BinaryRepeatMode, tc.binaryRepeat)
			}
			if got := externalRuleNames(tc.grammar); !slices.Equal(got, tc.externals) {
				t.Fatalf("%s externals = %v, want %v", tc.name, got, tc.externals)
			}
		})
	}

	if JSXGrammar().Name != "javascript" {
		t.Fatalf("JSXGrammar().Name = %q, want javascript", JSXGrammar().Name)
	}
	if JSGrammar().Name != "javascript" || JavascriptGrammar().Name != "javascript" {
		t.Fatalf("JavaScript aliases did not return javascript grammar")
	}
	if !JavaScriptGrammar().FlattenGeneratedRepeatAux {
		t.Fatalf("JavaScriptGrammar did not enable generated repeat aux flattening")
	}
	if !slices.Equal(JavaScriptGrammar().ReuseRepeatAuxForParents, []string{"jsx_opening_element", "jsx_self_closing_element"}) {
		t.Fatalf("JavaScriptGrammar repeat aux reuse parents = %v", JavaScriptGrammar().ReuseRepeatAuxForParents)
	}
	if TSGrammar().Name != "typescript" || TypescriptGrammar().Name != "typescript" {
		t.Fatalf("TypeScript aliases did not return typescript grammar")
	}
	if TsxGrammar().Name != "tsx" {
		t.Fatalf("TsxGrammar().Name = %q, want tsx", TsxGrammar().Name)
	}
}

func TestImportedJavaScriptTypeScriptTSXFortranGrammarsAreExtendable(t *testing.T) {
	tests := []struct {
		name    string
		grammar *Grammar
	}{
		{name: "javascript", grammar: JavaScriptGrammar()},
		{name: "jsx", grammar: JSXGrammar()},
		{name: "typescript", grammar: TypeScriptGrammar()},
		{name: "tsx", grammar: TSXGrammar()},
		{name: "fortran", grammar: FortranGrammar()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			extended := ExtendGrammar(tc.name+"_ext", tc.grammar, func(g *Grammar) {
				g.Define("__got_extension_marker", Str("__got_extension_marker"))
			})
			if extended.Name != tc.name+"_ext" {
				t.Fatalf("extended.Name = %q, want %q", extended.Name, tc.name+"_ext")
			}
			if _, ok := extended.Rules["__got_extension_marker"]; !ok {
				t.Fatalf("extension marker rule was not added")
			}
			if _, ok := tc.grammar.Rules["__got_extension_marker"]; ok {
				t.Fatalf("base grammar was mutated through extension")
			}
		})
	}
}

func TestImportedJavaScriptTypeScriptInlineRulesAreDefined(t *testing.T) {
	tests := []struct {
		name    string
		grammar *Grammar
	}{
		{name: "javascript", grammar: JavaScriptGrammar()},
		{name: "typescript", grammar: TypeScriptGrammar()},
		{name: "tsx", grammar: TSXGrammar()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, name := range tc.grammar.Inline {
				if _, ok := tc.grammar.Rules[name]; !ok {
					t.Fatalf("inline rule %q is not defined", name)
				}
			}
		})
	}
}

func TestImportedTypeScriptInlineRulesPreserveResolvedGrammarShape(t *testing.T) {
	// tree-sitter-typescript extends JavaScript but intentionally filters these
	// inherited inline helpers out in common/define-grammar.js.
	tests := []struct {
		name    string
		grammar *Grammar
	}{
		{name: "typescript", grammar: TypeScriptGrammar()},
		{name: "tsx", grammar: TSXGrammar()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, name := range []string{"_call_signature", "_formal_parameter"} {
				if slices.Contains(tc.grammar.Inline, name) {
					t.Fatalf("%s inline rules include %q, but resolved tree-sitter-typescript filters it out", tc.name, name)
				}
			}
		})
	}
}

func externalRuleNames(g *Grammar) []string {
	out := make([]string, len(g.Externals))
	for i, rule := range g.Externals {
		if rule != nil {
			out[i] = rule.Value
		}
	}
	return out
}
