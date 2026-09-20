package grammargen

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// immediateTwinGrammar mirrors the Swift shape that regressed: a named
// token.immediate() terminal whose body is one exact string, plus an anonymous
// string literal with the same text. Both terminals are valid in the parser
// state after `identifier`, so the lexer must decide which one the DFA accepts
// for a byte-adjacent `?`.
//
// The C lexer enters the immediate lex state when no layout precedes the
// token, so it accepts `_immediate_quest` there. grammargen must reach the
// same decision.
func immediateTwinGrammar() *Grammar {
	g := NewGrammar("immediate_named_token_twin")
	g.Define("source_file", Repeat1(Sym("_item")))
	g.Define("_item",
		Choice(
			Sym("optional_type"),
			Sym("ternary"),
		))
	g.Define("optional_type",
		Seq(
			Sym("identifier"),
			Repeat1(
				Alias(Sym("_immediate_quest"), "?", false),
			),
		))
	g.Define("ternary",
		Seq(
			Sym("identifier"),
			Str("?"),
			Sym("identifier"),
			Str(":"),
			Sym("identifier"),
		))
	g.Define("_immediate_quest", ImmToken(Str("?")))
	g.Define("identifier", Pat("[a-z]+"))
	g.SetExtras(Pat(`\s+`))
	return g
}

// TestNamedImmediateStringTokenWinsOverPlainTwin pins the lexical decision for
// a named string-bodied token.immediate() terminal that shares its text with an
// anonymous string literal. Without the specificity bonus the anonymous literal
// wins the same-span DFA tie, the parser has no action for it, and a bare
// optional such as Swift's `var opt: Int?` parses with an ERROR.
func TestNamedImmediateStringTokenWinsOverPlainTwin(t *testing.T) {
	lang, err := GenerateLanguage(immediateTwinGrammar())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse([]byte("a?"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	got := root.SExpr(lang)
	if root.HasError() {
		t.Fatalf("byte-adjacent optional parsed with an error: %s", got)
	}
	want := "(source_file (optional_type (identifier)))"
	if got != want {
		t.Fatalf("SExpr = %s, want %s", got, want)
	}
}

// TestNamedImmediateStringTokenKeepsTernaryReachable verifies that the
// specificity bonus does not steal the anonymous literal from the state where
// the parser really wants it.
func TestNamedImmediateStringTokenKeepsTernaryReachable(t *testing.T) {
	lang, err := GenerateLanguage(immediateTwinGrammar())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse([]byte("a ? b : c"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	got := root.SExpr(lang)
	if root.HasError() {
		t.Fatalf("spaced ternary parsed with an error: %s", got)
	}
	want := "(source_file (ternary (identifier) (identifier) (identifier)))"
	if got != want {
		t.Fatalf("SExpr = %s, want %s", got, want)
	}
}

// TestNormalizeNamedImmediateStringTokenPriority pins the normalize-level
// contract: a string-bodied named token.immediate() terminal carries the
// specificity bonus, and a pattern-bodied one keeps its authored precedence.
func TestNormalizeNamedImmediateStringTokenPriority(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rule     *Rule
		priority int
	}{
		{"string", ImmToken(Str("?")), -10000},
		{"string choice", ImmToken(Choice(Str("px"), Str("em"))), -10000},
		{"string with precedence", ImmToken(Prec(2, Str("px"))), -12000},
		{"pattern", ImmToken(Pat(`[a-z]+`)), 0},
		{"pattern with precedence", ImmToken(Prec(2, Pat(`[a-z]+`))), -2000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGrammar("named_immediate_priority")
			g.Define("source_file", Sym("unit"))
			g.Define("unit", tc.rule)
			ng, err := Normalize(g)
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}
			for _, terminal := range ng.Terminals {
				if ng.Symbols[terminal.SymbolID].Name != "unit" {
					continue
				}
				if !terminal.Immediate || terminal.Priority != tc.priority {
					t.Fatalf("immediate=%t priority=%d, want immediate=true priority=%d",
						terminal.Immediate, terminal.Priority, tc.priority)
				}
				return
			}
			t.Fatal("unit terminal not found")
		})
	}
}

// TestNormalizeNamedImmediateStringTokenYieldsToLongerLiteral keeps the
// generator from starving a longer non-immediate literal that starts with the
// immediate token's text. Greedy longest match must still reach it.
func TestNormalizeNamedImmediateStringTokenYieldsToLongerLiteral(t *testing.T) {
	g := NewGrammar("named_immediate_longer_literal")
	g.Define("source_file",
		Choice(
			Sym("hash"),
			Str("#)"),
		))
	g.Define("hash", ImmToken(Str("#")))
	ng, err := Normalize(g)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	for _, terminal := range ng.Terminals {
		if ng.Symbols[terminal.SymbolID].Name != "hash" {
			continue
		}
		if !terminal.Immediate || terminal.Priority != 0 {
			t.Fatalf("immediate=%t priority=%d, want immediate=true priority=0",
				terminal.Immediate, terminal.Priority)
		}
		return
	}
	t.Fatal("hash terminal not found")
}
