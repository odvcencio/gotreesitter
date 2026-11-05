package grammargen

import (
	"go/ast"
	goparser "go/parser"
	"go/token"
	"slices"
	"strconv"
	"testing"
)

// The ordered reserved.global strings come from tree-sitter-go's primary
// src/grammar.json at commit 2346a3ab1bb3857b48b29d779a1ef9799a248cd7:
// https://github.com/tree-sitter/tree-sitter-go/blob/2346a3ab1bb3857b48b29d779a1ef9799a248cd7/src/grammar.json
// That file's SHA-256 is 0dcc665aa521a1b73a200be13b4e376780cca037d764a165f12864babfe28000.
// Keep this independent of GoGrammar: omitting these sets lets error-mode
// keyword capture treat a reserved keyword such as func as an identifier.
var goReservedWordsPinned = [...]string{
	"break", "default", "func", "interface", "select",
	"case", "defer", "go", "map", "struct",
	"chan", "else", "goto", "package", "switch",
	"const", "fallthrough", "if", "range", "type",
	"continue", "for", "import", "return", "var",
}

func TestGoGrammarReservedWordsMatchPinnedUpstream(t *testing.T) {
	requireGoReservedWords(t, GoGrammar())
}

func requireGoReservedWords(t *testing.T, g *Grammar) {
	t.Helper()
	if g.Word != "identifier" || len(g.ReservedWordSets) != 1 || g.ReservedWordSets[0].Name != "global" {
		t.Fatalf("Go word=%q reserved sets=%+v; want identifier and one global set", g.Word, g.ReservedWordSets)
	}
	var words []string
	for _, rule := range g.ReservedWordSets[0].Rules {
		if rule == nil || rule.Kind != RuleString {
			t.Fatalf("global reserved rule=%+v; want a literal string", rule)
		}
		words = append(words, rule.Value)
	}
	if !slices.Equal(words, goReservedWordsPinned[:]) {
		t.Fatalf("Go global reserved words=%q; want ordered primary-source list %q", words, goReservedWordsPinned)
	}
}

// Round-trip the emitted Go AST's reservation metadata, without compiling a
// temporary package or depending on an upstream archive during ordinary tests.
func TestGoGrammarReservedWordsSurviveGoEmission(t *testing.T) {
	source, err := EmitGrammarGo(GoGrammar(), "grammargen", "EmittedGoGrammar")
	if err != nil {
		t.Fatal(err)
	}
	file, err := goparser.ParseFile(token.NewFileSet(), "emitted_go_grammar.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip := &Grammar{}
	var sets *ast.CompositeLit
	wordCalls, reservedAssignments := 0, 0
	ast.Inspect(file, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "SetWord" {
				wordCalls++
				if len(call.Args) != 1 {
					t.Fatal("emitted SetWord has no single argument")
				}
				roundTrip.Word = goReservedWordString(t, call.Args[0])
			}
		}
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return true
		}
		selector, ok := assignment.Lhs[0].(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "ReservedWordSets" {
			return true
		}
		reservedAssignments++
		sets, _ = assignment.Rhs[0].(*ast.CompositeLit)
		return false
	})
	if wordCalls != 1 || reservedAssignments != 1 || sets == nil {
		t.Fatalf("emission lost reservation metadata: word calls=%d reserved assignments=%d literal=%v", wordCalls, reservedAssignments, sets != nil)
	}
	for _, element := range sets.Elts {
		literal, ok := element.(*ast.CompositeLit)
		if !ok {
			t.Fatal("emitted reserved set is not a literal")
		}
		var set ReservedWordSet
		for _, element := range literal.Elts {
			field, ok := element.(*ast.KeyValueExpr)
			if !ok {
				t.Fatal("emitted reserved set has an unkeyed field")
			}
			key, ok := field.Key.(*ast.Ident)
			if !ok {
				t.Fatal("emitted reserved field has no name")
			}
			switch key.Name {
			case "Name":
				set.Name = goReservedWordString(t, field.Value)
			case "Rules":
				rules, ok := field.Value.(*ast.CompositeLit)
				if !ok {
					t.Fatal("emitted reserved rules are not a literal")
				}
				for _, element := range rules.Elts {
					call, ok := element.(*ast.CallExpr)
					if !ok || len(call.Args) != 1 {
						t.Fatal("emitted reserved word has no single string argument")
					}
					constructor, ok := call.Fun.(*ast.Ident)
					if !ok || constructor.Name != "Str" {
						t.Fatal("emitted reserved word is not a literal string rule")
					}
					set.Rules = append(set.Rules, Str(goReservedWordString(t, call.Args[0])))
				}
			default:
				t.Fatalf("unexpected reserved field %q", key.Name)
			}
		}
		roundTrip.ReservedWordSets = append(roundTrip.ReservedWordSets, set)
	}
	requireGoReservedWords(t, roundTrip)
}

func goReservedWordString(t *testing.T, expression ast.Expr) string {
	t.Helper()
	literal, ok := expression.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		t.Fatal("emitted reservation value is not a string literal")
	}
	value, err := strconv.Unquote(literal.Value)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
