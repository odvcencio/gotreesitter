//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// This lexer deliberately elects int as a type identifier. It supplies no
// proof for another lexer's policy, even though its old tree is accepted.
type javaIdentifierTypeWitness struct {
	base       *grammars.JavaTokenSource
	identifier gotreesitter.Symbol
}

func (ts *javaIdentifierTypeWitness) retag(token gotreesitter.Token) gotreesitter.Token {
	if token.Text == "int" {
		token.Symbol = ts.identifier
	}
	return token
}
func (ts *javaIdentifierTypeWitness) Next() gotreesitter.Token { return ts.retag(ts.base.Next()) }
func (ts *javaIdentifierTypeWitness) SkipToByte(offset uint32) gotreesitter.Token {
	return ts.retag(ts.base.SkipToByte(offset))
}
func (ts *javaIdentifierTypeWitness) SupportsIncrementalReuse() bool { return true }
func (ts *javaIdentifierTypeWitness) SetParserState(state gotreesitter.StateID) {
	ts.base.SetParserState(state)
}

func TestJavaEOFCommentLexerWitnessMatchesLockedC(t *testing.T) {
	lang := grammars.JavaLanguage()
	cLang, err := COracleLanguage("java")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	source := []byte("class Main { int x; }\n// ")
	edited := append(bytes.Clone(source), 'x')
	cTree := cParser.Parse(edited, nil)
	if cTree == nil {
		t.Fatal("locked C returned no tree")
	}
	defer cTree.Close()
	foreign := *lang
	foreign.SymbolNames = append([]string(nil), lang.SymbolNames...)
	intSymbol, _ := lang.SymbolByName("int")
	foreign.SymbolNames[intSymbol] = "foreign_int"
	identifier, _ := lang.SymbolByName("identifier")
	for _, candidate := range []bool{false, true} {
		for _, backend := range []string{"custom", "foreign_language"} {
			t.Run(fmt.Sprintf("candidate=%t/%s", candidate, backend), func(t *testing.T) {
				parser := gotreesitter.NewParser(lang)
				parser.SetAdmissionCandidateRoute(candidate)
				var tokens gotreesitter.TokenSource = grammars.NewJavaTokenSourceOrEOF(source, &foreign)
				if backend == "custom" {
					base, err := grammars.NewJavaTokenSource(source, lang)
					if err != nil {
						t.Fatal(err)
					}
					tokens = &javaIdentifierTypeWitness{base: base, identifier: identifier}
				}
				old, err := parser.ParseWithTokenSource(source, tokens)
				if err != nil || old == nil || old.RootNode().HasError() {
					t.Fatalf("old lexer did not produce a clean tree: %v", err)
				}
				defer old.Release()
				old.Edit(gotreesitter.InputEdit{StartByte: uint32(len(source)), OldEndByte: uint32(len(source)), NewEndByte: uint32(len(edited)),
					StartPoint: gotreesitter.Point{Row: 1, Column: 3}, OldEndPoint: gotreesitter.Point{Row: 1, Column: 3},
					NewEndPoint: gotreesitter.Point{Row: 1, Column: 4}})
				next, err := parser.ParseIncrementalWithTokenSource(edited, old, grammars.NewJavaTokenSourceOrEOF(edited, lang))
				if err != nil {
					t.Fatal(err)
				}
				defer next.Release()
				if next.ParseStopReason() != gotreesitter.ParseStopAccepted || next.RootNode().EndByte() != uint32(len(edited)) {
					t.Fatal("incremental result did not accept the complete input")
				}
				if diff := FirstDivergenceDumpV1(next.RootNode(), lang, cTree.RootNode()); diff != nil {
					t.Fatalf("new lexer differs from locked C: %+v", diff)
				}
			})
		}
	}
}
