package parsercorephase0

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMetadataAuthenticationLexicalProvenanceRatchet(t *testing.T) {
	allowedRawAppenders := map[string]int{
		"PushReusedSubtreeOwnedWithPoll": 0,
		"appendAuthenticatedTerminal":    0,
		"appendSubtree":                  0,
		"reductionParentForPath":         0,
	}
	allowedTerminalCallers := map[string]int{
		"appendDiagnosticPayload":                     0,
		"shiftClassifiedUncheckpointed":               0,
		"shiftDirectUncheckpointed":                   0,
		"shiftOrdinaryClassifiedCohortUncheckpointed": 0,
		"shiftExtraClassifiedCohortUncheckpointed":    0,
	}
	authenticationWrites := map[string]map[bool]int{
		"appendSubtree": {false: 0},
		"Reset":         {true: 0},
	}
	constructorAuthentication := 0

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Clean(name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			functionName := function.Name.Name
			ast.Inspect(function.Body, func(node ast.Node) bool {
				switch node := node.(type) {
				case *ast.AssignStmt:
					for index, lhs := range node.Lhs {
						selector, ok := lhs.(*ast.SelectorExpr)
						if !ok || selector.Sel.Name != "metadataConstructionAuthenticated" || index >= len(node.Rhs) {
							continue
						}
						value, literal := booleanLiteral(node.Rhs[index])
						allowed := authenticationWrites[functionName]
						if !literal || allowed == nil {
							t.Errorf("%s: construction-authentication write outside lexical seam: %s", fset.Position(node.Pos()), functionName)
						} else if _, ok := allowed[value]; !ok {
							t.Errorf("%s: unexpected construction-authentication value %t in %s", fset.Position(node.Pos()), value, functionName)
						} else {
							allowed[value]++
						}
					}
				case *ast.KeyValueExpr:
					key, ok := node.Key.(*ast.Ident)
					if !ok || key.Name != "metadataConstructionAuthenticated" {
						break
					}
					value, literal := booleanLiteral(node.Value)
					if functionName != "New" || !literal || !value {
						t.Errorf("%s: constructor authentication literal outside New(true)", fset.Position(node.Pos()))
					} else {
						constructorAuthentication++
					}
				case *ast.CallExpr:
					selector, ok := node.Fun.(*ast.SelectorExpr)
					if !ok {
						break
					}
					switch selector.Sel.Name {
					case "appendSubtreeRecord":
						if _, allowed := allowedRawAppenders[functionName]; !allowed {
							t.Errorf("%s: appendSubtreeRecord call outside lexical publication seam: %s", fset.Position(node.Pos()), functionName)
						} else {
							allowedRawAppenders[functionName]++
						}
					case "appendAuthenticatedTerminal":
						if _, allowed := allowedTerminalCallers[functionName]; !allowed {
							t.Errorf("%s: appendAuthenticatedTerminal call outside authenticated shift seam: %s", fset.Position(node.Pos()), functionName)
						} else {
							allowedTerminalCallers[functionName]++
							if !authenticatedTerminalLiteral(node) {
								t.Errorf("%s: appendAuthenticatedTerminal in %s must receive one subtreeRecord literal with terminal:true and one lexer provenance length", fset.Position(node.Pos()), functionName)
							}
						}
					}
				}
				return true
			})
		}
	}

	if constructorAuthentication != 1 {
		t.Errorf("metadataConstructionAuthenticated=true literals in New = %d, want 1", constructorAuthentication)
	}
	for functionName, values := range authenticationWrites {
		for value, count := range values {
			if count != 1 {
				t.Errorf("metadataConstructionAuthenticated=%t assignments in %s = %d, want 1", value, functionName, count)
			}
		}
	}
	for functionName, count := range allowedRawAppenders {
		if count != 1 {
			t.Errorf("appendSubtreeRecord calls in %s = %d, want 1", functionName, count)
		}
	}
	for functionName, count := range allowedTerminalCallers {
		if count != 1 {
			t.Errorf("appendAuthenticatedTerminal calls in %s = %d, want 1", functionName, count)
		}
	}
}

func authenticatedTerminalLiteral(call *ast.CallExpr) bool {
	if len(call.Args) != 2 {
		return false
	}
	literal, ok := call.Args[0].(*ast.CompositeLit)
	if !ok {
		return false
	}
	typeName, ok := literal.Type.(*ast.Ident)
	if !ok || typeName.Name != "subtreeRecord" {
		return false
	}
	found := 0
	for _, element := range literal.Elts {
		field, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := field.Key.(*ast.Ident)
		if !ok || key.Name != "terminal" {
			continue
		}
		value, literal := booleanLiteral(field.Value)
		if !literal || !value {
			return false
		}
		found++
	}
	return found == 1
}

func booleanLiteral(expression ast.Expr) (value, ok bool) {
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return false, false
	}
	switch identifier.Name {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}
