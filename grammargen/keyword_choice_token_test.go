package grammargen

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestNamedStringChoiceTokenBecomesKeyword(t *testing.T) {
	g := NewGrammar("named_string_choice_keyword")
	g.Define("source_file", Sym("predefined_type"))
	g.Define("predefined_type", Token(Choice(
		Str("int"),
		Str("string"),
		Str("nint"),
	)))
	g.Define("identifier", Pat(`[A-Za-z_][A-Za-z0-9_]*`))
	g.SetWord("identifier")

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}

	predefinedTypeSym := -1
	for i, sym := range ng.Symbols {
		if sym.Name == "predefined_type" {
			predefinedTypeSym = i
			break
		}
	}
	if predefinedTypeSym < 0 {
		t.Fatal("predefined_type symbol not found")
	}

	foundKeyword := false
	for _, symID := range ng.KeywordSymbols {
		if symID == predefinedTypeSym {
			foundKeyword = true
			break
		}
	}
	if !foundKeyword {
		t.Fatalf("predefined_type sym %d missing from keyword set", predefinedTypeSym)
	}

	for _, term := range ng.Terminals {
		if term.SymbolID == predefinedTypeSym {
			t.Fatalf("predefined_type sym %d still present in main terminals", predefinedTypeSym)
		}
	}

	foundEntry := false
	for _, entry := range ng.KeywordEntries {
		if entry.SymbolID == predefinedTypeSym {
			foundEntry = true
			break
		}
	}
	if !foundEntry {
		t.Fatalf("predefined_type sym %d missing from keyword entries", predefinedTypeSym)
	}
}

func TestBareLexicalChoiceBecomesNamedToken(t *testing.T) {
	g := NewGrammar("bare_lexical_choice_named_token")
	g.Define("source_file", Sym("builtin_type"))
	g.Define("builtin_type", Choice(
		Str("bool"),
		Pat(`(i|u)[1-9][0-9]*`),
	))
	g.Define("identifier", Pat(`[A-Za-z_][A-Za-z0-9_]*`))
	g.SetWord("identifier")

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	builtinTypeSym := -1
	for i, sym := range ng.Symbols {
		if sym.Name == "builtin_type" {
			builtinTypeSym = i
			if sym.Kind != SymbolNamedToken {
				t.Fatalf("builtin_type kind = %v, want SymbolNamedToken", sym.Kind)
			}
			break
		}
	}
	if builtinTypeSym < 0 {
		t.Fatal("builtin_type symbol not found")
	}
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte("i32"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	if got := tree.RootNode().SExpr(lang); got != "(source_file (builtin_type))" {
		t.Fatalf("SExpr = %s, want (source_file (builtin_type))", got)
	}
}

func TestExplicitPrecStringChoiceBecomesNonterminal(t *testing.T) {
	g := NewGrammar("explicit_prec_string_choice_nonterminal")
	g.Define("source_file", Sym("initializer"))
	g.Define("initializer", Seq(Sym("init_class"), Sym("identifier")))
	g.Define("init_class", PrecRight(0, Choice(Str("="), Str("+="), Str("-="))))
	g.Define("identifier", Pat(`[a-z]+`))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	initClassSym := -1
	for i, sym := range ng.Symbols {
		if sym.Name == "init_class" {
			initClassSym = i
			if sym.Kind != SymbolNonterminal {
				t.Fatalf("init_class kind = %v, want SymbolNonterminal", sym.Kind)
			}
			if !sym.Visible || !sym.Named {
				t.Fatalf("init_class metadata = visible:%v named:%v, want visible/named", sym.Visible, sym.Named)
			}
			break
		}
	}
	if initClassSym < 0 {
		t.Fatal("init_class symbol not found")
	}

	for _, term := range ng.Terminals {
		if term.SymbolID == initClassSym {
			t.Fatalf("init_class sym %d was emitted as a terminal", initClassSym)
		}
	}

	wantAlts := map[string]bool{"=": false, "+=": false, "-=": false}
	for _, prod := range ng.Productions {
		if prod.LHS != initClassSym || len(prod.RHS) != 1 {
			continue
		}
		rhs := prod.RHS[0]
		if rhs < 0 || rhs >= len(ng.Symbols) {
			continue
		}
		name := ng.Symbols[rhs].Name
		if _, ok := wantAlts[name]; ok {
			wantAlts[name] = true
		}
	}
	for name, found := range wantAlts {
		if !found {
			t.Fatalf("missing init_class -> %q production", name)
		}
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte("+=abc"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("parse has error: %s", root.SExpr(lang))
	}
	if got := root.SExpr(lang); got != "(source_file (initializer (init_class) (identifier)))" {
		t.Fatalf("SExpr = %s, want init_class wrapper", got)
	}
}

func TestPlainVisibleStringRuleBecomesNamedLeafToken(t *testing.T) {
	g := NewGrammar("plain_visible_string_rule_named_leaf_token")
	g.Define("source_file", Sym("null_literal"))
	g.Define("null_literal", Str("null"))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got := symbolKind(t, ng, "null_literal"); got != SymbolNamedToken {
		t.Fatalf("null_literal kind = %v, want SymbolNamedToken", got)
	}
	for _, sym := range ng.Symbols {
		if sym.Name == "null" && sym.Kind == SymbolTerminal {
			t.Fatalf("plain named string rule also registered anonymous %q terminal", sym.Name)
		}
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte("null"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root == nil || root.ChildCount() != 1 {
		t.Fatalf("root = %v, child count = %d", root, root.ChildCount())
	}
	lit := root.Child(0)
	if got := lit.Type(lang); got != "null_literal" {
		t.Fatalf("child type = %q, want null_literal; tree=%s", got, root.SExpr(lang))
	}
	if !lit.IsNamed() {
		t.Fatalf("null_literal IsNamed = false")
	}
	if got := lit.ChildCount(); got != 0 {
		t.Fatalf("null_literal child count = %d, want 0; tree=%s", got, root.SExpr(lang))
	}
}

// TestPlainVisibleIdentifierStringRuleWithWordBecomesNamedLeafToken pins the
// wave7/fix-word-guard fix: a word-declaring grammar's uniquely-owned bare
// STRING rule now collapses into a 0-child named leaf token exactly like a
// non-word grammar's does (TestPlainVisibleStringRuleBecomesNamedLeafToken),
// matching tree-sitter's C compiler (confirmed against the C oracle for
// java's void_type/null_literal/underscore_pattern, rust's
// mutable_specifier, and python's true/false/none — see
// isPlainVisibleNamedLeafStringLiteral). Before this fix, ANY grammar
// declaring `word:` unconditionally forced every plain-STRING visible rule
// to stay a wrapper nonterminal, regardless of shape or unique ownership.
//
// "match" here is deliberately unreachable from source_file (no production
// references Sym("match")), which doubles as a regression guard for the
// *other* half of the fix: promoting a keyword-shaped literal to a named
// token must not break the pre-existing context-aware keyword-promotion
// fallback (dfaTokenSource.promoteKeyword) that demotes a keyword back to
// the word token when no active parser state has an action for it. Keyword
// extraction and leaf collapse are orthogonal mechanisms in tree-sitter;
// this test exercises both at once on the same symbol.
func TestPlainVisibleIdentifierStringRuleWithWordBecomesNamedLeafToken(t *testing.T) {
	g := NewGrammar("plain_visible_identifier_string_rule_with_word_named_leaf_token")
	g.Define("source_file", Sym("call"))
	g.Define("call", Seq(Sym("identifier"), Str("("), Str(")")))
	g.Define("match", Str("match"))
	g.Define("identifier", Pat(`[a-z]+`))
	g.SetWord("identifier")

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got := symbolKind(t, ng, "match"); got != SymbolNamedToken {
		t.Fatalf("match kind = %v, want SymbolNamedToken", got)
	}
	matchSym := symbolID(t, ng, "match")
	for _, sym := range ng.Symbols {
		if sym.Name == "match" && sym.Kind == SymbolTerminal {
			t.Fatalf("plain named string rule also registered anonymous %q terminal", sym.Name)
		}
	}
	for _, term := range ng.Terminals {
		if term.SymbolID == matchSym {
			t.Fatalf("match sym %d was double-registered as a main-DFA terminal (extractTerminals keywordSet gap)", matchSym)
		}
	}
	foundKeyword := false
	for _, symID := range ng.KeywordSymbols {
		if symID == matchSym {
			foundKeyword = true
			break
		}
	}
	if !foundKeyword {
		t.Fatalf("match sym %d missing from keyword set despite matching the identifier-shaped word DFA", matchSym)
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte("match()"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("parse has error: %s", root.SExpr(lang))
	}
	if got, want := root.SExpr(lang), "(source_file (call (identifier)))"; got != want {
		t.Fatalf("SExpr = %s, want %s (unreachable keyword must fall back to identifier)", got, want)
	}
}

// TestWordGrammarSharedStringRuleStaysNonterminal is the negative
// counterpart of TestPlainVisibleIdentifierStringRuleWithWordBecomesNamedLeafToken:
// in a word-declaring grammar, a plain visible STRING rule whose value is
// ALSO used bare elsewhere in the grammar is not a unique owner, so it must
// keep wrapping the anonymous string token instead of collapsing — the word
// guard's removal must not disable the ownership check itself.
func TestWordGrammarSharedStringRuleStaysNonterminal(t *testing.T) {
	g := NewGrammar("word_grammar_shared_string_rule_nonterminal")
	g.Define("source_file", Seq(Sym("void_type"), Str("void")))
	g.Define("void_type", Str("void"))
	g.Define("identifier", Pat(`[a-z]+`))
	g.SetWord("identifier")
	g.SetExtras(Pat(`\s`))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got := symbolKind(t, ng, "void_type"); got != SymbolNonterminal {
		t.Fatalf("void_type kind = %v, want SymbolNonterminal (value used bare elsewhere, not a unique owner)", got)
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte("void void"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("parse has error: %s", root.SExpr(lang))
	}
	wrapper := root.Child(0)
	if got := wrapper.Type(lang); got != "void_type" {
		t.Fatalf("first child type = %q, want void_type; tree=%s", got, root.SExpr(lang))
	}
	if got := wrapper.ChildCount(); got != 1 {
		t.Fatalf("void_type child count = %d, want wrapper child; tree=%s", got, root.SExpr(lang))
	}
}

// TestWordGrammarCapitalizedUniqueOwnerBecomesNamedLeafToken pins python's
// true/false/none shape exactly: STRING("True") is identifier-shaped only
// case-insensitively. isLowercaseIdentifierLikeKeywordLiteral (used for
// non-word grammars, see TestCapitalizedPlainStringRuleStaysWrapper) would
// reject it, but tree-sitter's C compiler collapses it in word-declaring
// grammars regardless of case — confirmed against the C oracle.
func TestWordGrammarCapitalizedUniqueOwnerBecomesNamedLeafToken(t *testing.T) {
	g := NewGrammar("word_grammar_capitalized_unique_owner_named_leaf_token")
	g.Define("source_file", Sym("true"))
	g.Define("true", Str("True"))
	g.Define("identifier", Pat(`[A-Za-z]+`))
	g.SetWord("identifier")

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got := symbolKind(t, ng, "true"); got != SymbolNamedToken {
		t.Fatalf("true kind = %v, want SymbolNamedToken", got)
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte("True"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root == nil || root.ChildCount() != 1 {
		t.Fatalf("root = %v, child count = %d", root, root.ChildCount())
	}
	lit := root.Child(0)
	if got := lit.Type(lang); got != "true" {
		t.Fatalf("child type = %q, want true; tree=%s", got, root.SExpr(lang))
	}
	if got := lit.ChildCount(); got != 0 {
		t.Fatalf("true child count = %d, want 0; tree=%s", got, root.SExpr(lang))
	}
}

// TestWordGrammarUnderscoreUniqueOwnerBecomesNamedLeafToken pins java's
// underscore_pattern shape exactly: STRING("_") is not identifier-shaped
// under isIdentifierLikeKeywordLiteral (it never contains a letter), so it
// needs the dedicated all-underscore allowance in
// isIdentifierLikeOrAllUnderscoreKeywordLiteral. Also confirms a collapsed
// single-char keyword-shaped literal still disambiguates correctly from a
// longer identifier reachable in the same position.
func TestWordGrammarUnderscoreUniqueOwnerBecomesNamedLeafToken(t *testing.T) {
	g := NewGrammar("word_grammar_underscore_unique_owner_named_leaf_token")
	g.Define("source_file", Repeat(Sym("_arg")))
	g.Define("_arg", Choice(Sym("underscore_pattern"), Sym("identifier")))
	g.Define("underscore_pattern", Str("_"))
	g.Define("identifier", Pat(`[a-zA-Z][a-zA-Z0-9_]*`))
	g.SetWord("identifier")
	g.SetExtras(Pat(`\s`))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got := symbolKind(t, ng, "underscore_pattern"); got != SymbolNamedToken {
		t.Fatalf("underscore_pattern kind = %v, want SymbolNamedToken", got)
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte("_ foo _"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("parse has error: %s", root.SExpr(lang))
	}
	if got, want := root.SExpr(lang), "(source_file (underscore_pattern) (identifier) (underscore_pattern))"; got != want {
		t.Fatalf("SExpr = %s, want %s", got, want)
	}
}

func TestPlainVisiblePunctuationPrefixedStringRuleWithWordBecomesNamedLeafToken(t *testing.T) {
	g := NewGrammar("plain_visible_punctuation_prefixed_string_rule_with_word_named_leaf_token")
	g.Define("source_file", Seq(Sym("identifier"), Sym("important")))
	g.Define("identifier", Pat(`[a-z]+`))
	g.Define("important", Str("!important"))
	g.SetWord("identifier")
	g.Extras = []*Rule{Pat(`[ \t]+`)}

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got := symbolKind(t, ng, "important"); got != SymbolNamedToken {
		t.Fatalf("important kind = %v, want SymbolNamedToken", got)
	}
	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte("selector !important"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse missing root node")
	}
	if root.HasError() || root.ChildCount() != 2 {
		t.Fatalf("root = %v, child count = %d; tree=%s", root, root.ChildCount(), root.SExpr(lang))
	}
	important := root.Child(1)
	if got := important.Type(lang); got != "important" {
		t.Fatalf("child type = %q, want important; tree=%s", got, root.SExpr(lang))
	}
	if !important.IsNamed() {
		t.Fatal("important IsNamed = false")
	}
	if got := important.ChildCount(); got != 0 {
		t.Fatalf("important child count = %d, want 0; tree=%s", got, root.SExpr(lang))
	}
}

func TestPlainVisiblePunctuationStringRuleBecomesNamedLeafToken(t *testing.T) {
	g := NewGrammar("plain_visible_punctuation_string_rule_named_leaf_token")
	g.Define("source_file", Sym("optional_chain"))
	g.Define("optional_chain", Str("?."))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got := symbolKind(t, ng, "optional_chain"); got != SymbolNamedToken {
		t.Fatalf("optional_chain kind = %v, want SymbolNamedToken", got)
	}
	for _, sym := range ng.Symbols {
		if sym.Name == "?." && sym.Kind == SymbolTerminal {
			t.Fatalf("plain named punctuation string rule also registered anonymous %q terminal", sym.Name)
		}
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte("?."))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root == nil || root.ChildCount() != 1 {
		t.Fatalf("root = %v, child count = %d", root, root.ChildCount())
	}
	lit := root.Child(0)
	if got := lit.Type(lang); got != "optional_chain" {
		t.Fatalf("child type = %q, want optional_chain; tree=%s", got, root.SExpr(lang))
	}
	if !lit.IsNamed() {
		t.Fatalf("optional_chain IsNamed = false")
	}
	if got := lit.ChildCount(); got != 0 {
		t.Fatalf("optional_chain child count = %d, want 0; tree=%s", got, root.SExpr(lang))
	}
}

func TestPlainVisibleEscapedStringRulesBecomeNamedLeafTokens(t *testing.T) {
	g := NewGrammar("plain_visible_escaped_string_rules_named_leaf_tokens")
	g.Define("source_file", Seq(Sym("boundary_assertion"), Sym("non_boundary_assertion")))
	g.Define("boundary_assertion", Str("\\b"))
	g.Define("non_boundary_assertion", Str("\\B"))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	for _, name := range []string{"boundary_assertion", "non_boundary_assertion"} {
		if got := symbolKind(t, ng, name); got != SymbolNamedToken {
			t.Fatalf("%s kind = %v, want SymbolNamedToken", name, got)
		}
		sym := symbolID(t, ng, name)
		if sym >= ng.TokenCount() {
			t.Fatalf("%s symbol = %d outside token range %d", name, sym, ng.TokenCount())
		}
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte("\\b\\B"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root == nil || root.ChildCount() != 2 {
		t.Fatalf("root = %v, child count = %d; tree=%s", root, root.ChildCount(), root.SExpr(lang))
	}
	for i, name := range []string{"boundary_assertion", "non_boundary_assertion"} {
		node := root.Child(i)
		if got := node.Type(lang); got != name {
			t.Fatalf("child %d type = %q, want %s; tree=%s", i, got, name, root.SExpr(lang))
		}
		if !node.IsNamed() {
			t.Fatalf("%s IsNamed = false", name)
		}
		if got := node.ChildCount(); got != 0 {
			t.Fatalf("%s child count = %d, want 0; tree=%s", name, got, root.SExpr(lang))
		}
	}
}

func TestPlainVisiblePatternRuleBecomesNamedLeafToken(t *testing.T) {
	g := NewGrammar("plain_visible_pattern_rule_named_leaf_token")
	g.Define("source_file", Sym("any_character"))
	g.Define("any_character", Pat(`.`))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got := symbolKind(t, ng, "any_character"); got != SymbolNamedToken {
		t.Fatalf("any_character kind = %v, want SymbolNamedToken", got)
	}
	anyCharacterSym := symbolID(t, ng, "any_character")
	if anyCharacterSym >= ng.TokenCount() {
		t.Fatalf("any_character symbol = %d outside token range %d", anyCharacterSym, ng.TokenCount())
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte("."))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root == nil || root.ChildCount() != 1 {
		t.Fatalf("root = %v, child count = %d", root, root.ChildCount())
	}
	node := root.Child(0)
	if got := node.Type(lang); got != "any_character" {
		t.Fatalf("child type = %q, want any_character; tree=%s", got, root.SExpr(lang))
	}
	if !node.IsNamed() {
		t.Fatal("any_character IsNamed = false")
	}
	if got := node.ChildCount(); got != 0 {
		t.Fatalf("any_character child count = %d, want 0; tree=%s", got, root.SExpr(lang))
	}
}

func TestPlainVisiblePunctuationStringRuleWithAnonymousSourceStaysWrapper(t *testing.T) {
	g := NewGrammar("plain_visible_punctuation_string_rule_anonymous_collision")
	g.Define("source_file", Seq(Sym("optional_chain"), Str("?.")))
	g.Define("optional_chain", Str("?."))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got := symbolKind(t, ng, "optional_chain"); got != SymbolNonterminal {
		t.Fatalf("optional_chain kind = %v, want SymbolNonterminal", got)
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte("?.?."))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse missing root node")
	}
	if root.HasError() {
		t.Fatalf("parse has error: %s", root.SExpr(lang))
	}
	wrapper := root.Child(0)
	if got := wrapper.Type(lang); got != "optional_chain" {
		t.Fatalf("first child type = %q, want optional_chain; tree=%s", got, root.SExpr(lang))
	}
	if got := wrapper.ChildCount(); got != 1 {
		t.Fatalf("optional_chain child count = %d, want wrapper child; tree=%s", got, root.SExpr(lang))
	}
	child := wrapper.Child(0)
	if got := child.Type(lang); got != "?." || child.IsNamed() {
		t.Fatalf("wrapped child = (%q named=%v), want anonymous ?.; tree=%s", got, child.IsNamed(), root.SExpr(lang))
	}
}

func TestBareStringChoiceStaysNonterminal(t *testing.T) {
	g := NewGrammar("bare_string_choice_nonterminal")
	g.Define("source_file", Seq(Sym("identifier"), Sym("relational_operator"), Sym("identifier")))
	g.Define("identifier", Pat(`[a-z]+`))
	g.Define("relational_operator", Choice(
		Str("<"),
		Str(">"),
		Str("<="),
		Str(">="),
	))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	for _, sym := range ng.Symbols {
		if sym.Name != "relational_operator" {
			continue
		}
		if sym.Kind != SymbolNonterminal {
			t.Fatalf("relational_operator kind = %v, want SymbolNonterminal", sym.Kind)
		}
		return
	}
	t.Fatal("relational_operator symbol not found")
}

func TestHiddenBareStringSharingBareStringChoiceBecomesNonterminal(t *testing.T) {
	g := NewGrammar("hidden_bare_string_choice_collision")
	g.Define("source_file", Seq(Sym("_bang"), Sym("operator")))
	g.Define("_bang", Str("!"))
	g.Define("operator", Choice(
		Str("!"),
		Str("?"),
	))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	for _, name := range []string{"_bang", "operator"} {
		if got := symbolKind(t, ng, name); got != SymbolNonterminal {
			t.Fatalf("%s kind = %v, want SymbolNonterminal", name, got)
		}
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte("!!"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse missing root node")
	}
	if root.HasError() {
		t.Fatalf("parse has error: %s", root.SExpr(lang))
	}
}

func TestHiddenStringTokenSharingAnonymousLiteralBecomesNonterminal(t *testing.T) {
	g := NewGrammar("hidden_string_token_literal_collision")
	g.Define("source_file", Seq(Sym("_semi"), Str(";")))
	g.Define("_semi", Token(Str(";")))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got := symbolKind(t, ng, "_semi"); got != SymbolNonterminal {
		t.Fatalf("_semi kind = %v, want SymbolNonterminal", got)
	}

	semicolonTerminals := 0
	for _, term := range ng.Terminals {
		if ng.Symbols[term.SymbolID].Name == ";" {
			semicolonTerminals++
		}
	}
	if semicolonTerminals != 1 {
		t.Fatalf("semicolon terminal count = %d, want 1", semicolonTerminals)
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte(";;"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse missing root node")
	}
	if root.HasError() {
		t.Fatalf("parse has error: %s", root.SExpr(lang))
	}
}

func TestHiddenStringTokenDuplicatesWithoutAnonymousLiteralRemainTokens(t *testing.T) {
	g := NewGrammar("hidden_string_token_token_only_collision")
	g.Define("source_file", Seq(Sym("_bang"), Sym("_also_bang")))
	g.Define("_bang", Token(Str("!")))
	g.Define("_also_bang", Token(Str("!")))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	for _, name := range []string{"_bang", "_also_bang"} {
		if got := symbolKind(t, ng, name); got != SymbolNamedToken {
			t.Fatalf("%s kind = %v, want SymbolNamedToken", name, got)
		}
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte("!!"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse missing root node")
	}
	if root.HasError() {
		t.Fatalf("parse has error: %s", root.SExpr(lang))
	}
	if got := root.SExpr(lang); got != "(source_file)" {
		t.Fatalf("SExpr = %s, want (source_file)", got)
	}
}

func TestPrecedenceWrappedBareStringChoiceStaysNonterminal(t *testing.T) {
	g := NewGrammar("prec_wrapped_bare_string_choice_nonterminal")
	g.Define("source_file", Sym("operator"))
	g.Define("operator", Prec(1, Choice(
		Str("!"),
		Str("?"),
	)))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got := symbolKind(t, ng, "operator"); got != SymbolNonterminal {
		t.Fatalf("operator kind = %v, want SymbolNonterminal", got)
	}
}

func TestPrecRightStringChoiceSymbolStaysVisibleWrapper(t *testing.T) {
	g := NewGrammar("prec_right_string_choice_symbol_wrapper")
	g.Define("source_file", Sym("init_class"))
	g.Define("init_class", PrecRight(0, Choice(
		Str("="),
		Str("+="),
		Str("-="),
	)))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}

	initClassID := -1
	equalsID := -1
	for id, sym := range ng.Symbols {
		switch {
		case sym.Name == "init_class":
			initClassID = id
			if sym.Kind != SymbolNonterminal || !sym.Visible || !sym.Named {
				t.Fatalf("init_class symbol = %+v, want visible named nonterminal", sym)
			}
		case sym.Name == "=" && sym.Kind == SymbolTerminal && !sym.Named:
			equalsID = id
		}
	}
	if initClassID < 0 {
		t.Fatal("missing init_class symbol")
	}
	if equalsID < 0 {
		t.Fatal("missing anonymous = terminal")
	}
	for _, term := range ng.Terminals {
		if term.SymbolID == initClassID {
			t.Fatalf("init_class was emitted as a terminal pattern")
		}
	}

	hasEqualsProduction := false
	for _, prod := range ng.Productions {
		if prod.LHS == initClassID && len(prod.RHS) == 1 && prod.RHS[0] == equalsID {
			hasEqualsProduction = true
			break
		}
	}
	if !hasEqualsProduction {
		t.Fatal("missing init_class -> \"=\" wrapper production")
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte("="))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if root == nil || root.ChildCount() != 1 {
		t.Fatalf("root = %v, child count = %d", root, root.ChildCount())
	}
	wrapper := root.Child(0)
	if got := wrapper.Type(lang); got != "init_class" {
		t.Fatalf("child type = %q, want init_class; tree=%s", got, root.SExpr(lang))
	}
	if got := wrapper.ChildCount(); got != 1 {
		t.Fatalf("init_class child count = %d, want anonymous operator child; tree=%s", got, root.SExpr(lang))
	}
	child := wrapper.Child(0)
	if got := child.Type(lang); got != "=" || child.IsNamed() {
		t.Fatalf("wrapped child = (%q named=%v), want anonymous =; tree=%s", got, child.IsNamed(), root.SExpr(lang))
	}
}

func TestPrecedenceWrappedPlainStringRuleStaysWrapper(t *testing.T) {
	g := NewGrammar("prec_wrapped_plain_string_rule_wrapper")
	g.Define("source_file", Sym("null_literal"))
	g.Define("null_literal", Prec(1, Str("null")))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got := symbolKind(t, ng, "null_literal"); got != SymbolNonterminal {
		t.Fatalf("null_literal kind = %v, want SymbolNonterminal", got)
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte("null"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root == nil || root.ChildCount() != 1 {
		t.Fatalf("root = %v, child count = %d", root, root.ChildCount())
	}
	lit := root.Child(0)
	if got := lit.Type(lang); got != "null_literal" {
		t.Fatalf("child type = %q, want null_literal; tree=%s", got, root.SExpr(lang))
	}
	if got := lit.ChildCount(); got != 1 {
		t.Fatalf("null_literal child count = %d, want wrapper child; tree=%s", got, root.SExpr(lang))
	}
	child := lit.Child(0)
	if got := child.Type(lang); got != "null" || child.IsNamed() {
		t.Fatalf("wrapped child = (%q named=%v), want anonymous null; tree=%s", got, child.IsNamed(), root.SExpr(lang))
	}
}

func TestDynamicPrecedenceStringRuleStaysVisibleWrapper(t *testing.T) {
	g := NewGrammar("dynamic_prec_string_rule_wrapper")
	g.Define("source_file", Sym("implicit_type"))
	g.Define("implicit_type", PrecDynamic(1, Str("var")))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got := symbolKind(t, ng, "implicit_type"); got != SymbolNonterminal {
		t.Fatalf("implicit_type kind = %v, want SymbolNonterminal", got)
	}
	implicitTypeSym := symbolID(t, ng, "implicit_type")
	if implicitTypeSym < ng.TokenCount() {
		t.Fatalf("implicit_type symbol = %d inside token range %d", implicitTypeSym, ng.TokenCount())
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte("var"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root == nil || root.ChildCount() != 1 {
		t.Fatalf("root = %v, child count = %d", root, root.ChildCount())
	}
	wrapper := root.Child(0)
	if got := wrapper.Type(lang); got != "implicit_type" {
		t.Fatalf("child type = %q, want implicit_type; tree=%s", got, root.SExpr(lang))
	}
	if got := wrapper.ChildCount(); got != 1 {
		t.Fatalf("implicit_type child count = %d, want wrapper child; tree=%s", got, root.SExpr(lang))
	}
	child := wrapper.Child(0)
	if got := child.Type(lang); got != "var" || child.IsNamed() {
		t.Fatalf("wrapped child = (%q named=%v), want anonymous var; tree=%s", got, child.IsNamed(), root.SExpr(lang))
	}
}

func TestChoiceWrapperWithNonterminalAlternativeStaysVisibleWrapper(t *testing.T) {
	g := NewGrammar("choice_wrapper_with_nonterminal_alternative")
	g.Define("source_file", Sym("last_field"))
	g.Define("last_field", Choice(
		Sym("field_decl"),
		Str(".."),
	))
	g.Define("field_decl", Seq(Sym("identifier"), Str("="), Sym("identifier")))
	g.Define("identifier", Pat(`[a-z]+`))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got := symbolKind(t, ng, "last_field"); got != SymbolNonterminal {
		t.Fatalf("last_field kind = %v, want SymbolNonterminal", got)
	}
	lastFieldSym := symbolID(t, ng, "last_field")
	if lastFieldSym < ng.TokenCount() {
		t.Fatalf("last_field symbol = %d inside token range %d", lastFieldSym, ng.TokenCount())
	}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte(".."))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root == nil || root.ChildCount() != 1 {
		t.Fatalf("root = %v, child count = %d", root, root.ChildCount())
	}
	wrapper := root.Child(0)
	if got := wrapper.Type(lang); got != "last_field" {
		t.Fatalf("child type = %q, want last_field; tree=%s", got, root.SExpr(lang))
	}
	if got := wrapper.ChildCount(); got != 1 {
		t.Fatalf("last_field child count = %d, want wrapper child; tree=%s", got, root.SExpr(lang))
	}
	child := wrapper.Child(0)
	if got := child.Type(lang); got != ".." || child.IsNamed() {
		t.Fatalf("wrapped child = (%q named=%v), want anonymous ..; tree=%s", got, child.IsNamed(), root.SExpr(lang))
	}
}

func TestCapitalizedPlainStringRuleStaysWrapper(t *testing.T) {
	g := NewGrammar("capitalized_plain_string_rule_wrapper")
	g.Define("source_file", Sym("true"))
	g.Define("true", Str("True"))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got := symbolKind(t, ng, "true"); got != SymbolNonterminal {
		t.Fatalf("true kind = %v, want SymbolNonterminal", got)
	}
}

func TestTokenWrappedBareStringChoiceRemainsNamedToken(t *testing.T) {
	g := NewGrammar("token_wrapped_bare_string_choice_named_token")
	g.Define("source_file", Sym("operator_token"))
	g.Define("operator_token", Token(Choice(
		Str("!"),
		Str("?"),
	)))

	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got := symbolKind(t, ng, "operator_token"); got != SymbolNamedToken {
		t.Fatalf("operator_token kind = %v, want SymbolNamedToken", got)
	}
}

func TestAliasedInlinePatternWinsSameLengthNamedPatternTie(t *testing.T) {
	g := NewGrammar("aliased_inline_pattern_precedence")
	g.Define("source_file", Choice(
		Sym("preproc_include"),
		Sym("preproc_call"),
	))
	g.Define("preproc_include", Seq(
		Alias(Pat(`#[ \t]*include`), "#include", false),
		Sym("system_lib_string"),
		ImmToken(Pat(`\r?\n`)),
	))
	g.Define("preproc_call", Seq(
		Field("directive", Sym("preproc_directive")),
		Optional(Field("argument", Sym("preproc_arg"))),
		ImmToken(Pat(`\r?\n`)),
	))
	g.Define("preproc_directive", Pat(`#[ \t]*[a-zA-Z0-9]\w*`))
	g.Define("preproc_arg", Token(Pat(`[^\r\n]+`)))
	g.Define("system_lib_string", Token(Seq(
		Str("<"),
		Repeat(Pat(`[^>\n]`)),
		Str(">"),
	)))
	g.Extras = []*Rule{Pat(`[ \t]+`)}

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte("#include <iostream>\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	if got := tree.RootNode().SExpr(lang); got != "(source_file (preproc_include (system_lib_string)))" {
		t.Fatalf("SExpr = %s, want (source_file (preproc_include (system_lib_string)))", got)
	}
}

func TestUppercaseUnicodeEscapeIdentifierDoesNotCaptureDigits(t *testing.T) {
	g := NewGrammar("unicode_escape_identifier_digit_split")
	g.Define("source_file", Choice(
		Sym("upper_case_identifier"),
		Sym("number_literal"),
	))
	g.Define("upper_case_identifier", Pat(`[A-Z\U00010400-\U00010427][A-Z0-9]*`))
	g.Define("number_literal", Pat(`[0-9]+`))

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte("1"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	if got := tree.RootNode().SExpr(lang); got != "(source_file (number_literal))" {
		t.Fatalf("SExpr = %s, want (source_file (number_literal))", got)
	}
}

func symbolKind(t *testing.T, ng *NormalizedGrammar, name string) SymbolKind {
	t.Helper()
	for _, sym := range ng.Symbols {
		if sym.Name == name {
			return sym.Kind
		}
	}
	t.Fatalf("%s symbol not found", name)
	return SymbolTerminal
}

func symbolID(t *testing.T, ng *NormalizedGrammar, name string) int {
	t.Helper()
	for i, sym := range ng.Symbols {
		if sym.Name == name {
			return i
		}
	}
	t.Fatalf("%s symbol not found", name)
	return -1
}
