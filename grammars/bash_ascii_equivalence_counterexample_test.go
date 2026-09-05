//go:build !grammar_subset || grammar_subset_bash

package grammars

import (
	"bytes"
	"reflect"
	"testing"
	_ "unsafe"

	gts "github.com/odvcencio/gotreesitter"
)

func TestBashASCIIDigitEquivalenceCounterexamples(t *testing.T) {
	scanner := BashExternalScanner{}
	t.Run("variable_name_zero", func(t *testing.T) {
		var valid [bshTokErrorRecovery + 1]bool
		valid[bshTokVariableName] = true
		zeroSource, oneSource := []byte("0:"), []byte("1:")
		zero := newBashEquivalenceLexer(zeroSource, 0, 0, 0)
		one := newBashEquivalenceLexer(oneSource, 0, 0, 0)
		zeroState, oneState := scanner.Create(), scanner.Create()
		defer scanner.Destroy(zeroState)
		defer scanner.Destroy(oneState)
		zeroOK := scanner.Scan(zeroState, zero, valid[:])
		oneOK := scanner.Scan(oneState, one, valid[:])
		if zeroOK || !oneOK {
			t.Fatalf("variable-name outcomes zero=%t one=%t", zeroOK, oneOK)
		}
		if _, ok := bashEquivalenceLexerToken(zero); ok {
			t.Fatal("rejected zero published a token")
		}
		token, ok := bashEquivalenceLexerToken(one)
		if !ok || token.Symbol != bshSymVariableName || token.StartByte != 0 || token.EndByte != 1 || token.StartPoint != (gts.Point{}) || token.EndPoint != (gts.Point{Column: 1}) {
			t.Fatalf("accepted variable-name token: %+v, present=%t", token, ok)
		}
		if zero.Column() != 1 || one.Column() != 1 || zero.Lookahead() != ':' || one.Lookahead() != ':' {
			t.Fatal("unexpected failure/success cursor")
		}
		// Equalize source storage only after scanning. Every other lexer field
		// remains available to distinguish marks and result publication.
		copy(zeroSource, oneSource)
		if reflect.DeepEqual(zero, one) {
			t.Fatal("different scan outcomes had identical lexer observations")
		}
	})
	t.Run("heredoc_delimiter", func(t *testing.T) {
		var valid [bshTokErrorRecovery + 1]bool
		valid[bshTokHeredocEnd] = true
		zeroState := &bshState{heredocs: []bshHeredoc{{started: true, delimiter: []byte("0")}}}
		oneState := &bshState{heredocs: []bshHeredoc{{started: true, delimiter: []byte("0")}}}
		zeroSource, oneSource := []byte("0\n"), []byte("1\n")
		zero := newBashEquivalenceLexer(zeroSource, 0, 0, 0)
		one := newBashEquivalenceLexer(oneSource, 0, 0, 0)
		zeroOK := scanner.Scan(zeroState, zero, valid[:])
		oneOK := scanner.Scan(oneState, one, valid[:])
		if !zeroOK || oneOK {
			t.Fatalf("heredoc outcomes zero=%t one=%t", zeroOK, oneOK)
		}
		token, ok := bashEquivalenceLexerToken(zero)
		if !ok || token.Symbol != bshSymHeredocEnd || token.StartByte != 0 || token.EndByte != 1 || token.EndPoint != (gts.Point{Column: 1}) {
			t.Fatalf("heredoc token: %+v", token)
		}
		if _, ok := bashEquivalenceLexerToken(one); ok {
			t.Fatal("mismatched delimiter published a token")
		}
		if zero.Column() != 1 || zero.Lookahead() != '\n' || one.Column() != 0 || one.Lookahead() != '1' {
			t.Fatal("heredoc comparison used unexpected cursor")
		}
		var zeroWire, oneWire [128]byte
		zeroN := scanner.Serialize(zeroState, zeroWire[:])
		oneN := scanner.Serialize(oneState, oneWire[:])
		if bytes.Equal(zeroWire[:zeroN], oneWire[:oneN]) {
			t.Fatal("heredoc match failed to distinguish final scanner state")
		}
		copy(zeroSource, oneSource)
		if reflect.DeepEqual(zero, one) {
			t.Fatal("heredoc cursor/result observations are identical")
		}
	})
}

//go:linkname newBashEquivalenceLexer github.com/odvcencio/gotreesitter.newExternalLexer
func newBashEquivalenceLexer(source []byte, pos int, row, col uint32) *gts.ExternalLexer

//go:linkname bashEquivalenceLexerToken github.com/odvcencio/gotreesitter.(*ExternalLexer).token
func bashEquivalenceLexerToken(*gts.ExternalLexer) (gts.Token, bool)
