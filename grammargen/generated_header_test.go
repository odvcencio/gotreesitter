package grammargen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestEmitGrammarGoMarksGeneratedSource(t *testing.T) {
	source, err := EmitGrammarGo(NewGrammar("header_test"), "grammargen", "HeaderTestGrammar")
	if err != nil {
		t.Fatalf("EmitGrammarGo: %v", err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "header_test.go", source, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse generated source: %v", err)
	}
	if !ast.IsGenerated(file) {
		t.Fatal("generated source lacks a leading Go generated-code marker")
	}
}
