//go:build !grammar_subset || grammar_subset_fish

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the fish grammar.
const (
	fishTokConcat        = 0
	fishTokBracketConcat = 1
	fishTokConcatList    = 2
	fishTokBeginBrace    = 3
)

const (
	fishSymConcat        gotreesitter.Symbol = 53
	fishSymBracketConcat gotreesitter.Symbol = 54
	fishSymConcatList    gotreesitter.Symbol = 55
	fishSymBeginBrace    gotreesitter.Symbol = 56
)

// FishExternalScanner handles concatenation detection and begin-brace
// disambiguation for the Fish shell.
type FishExternalScanner struct{}

func (FishExternalScanner) Create() any                           { return nil }
func (FishExternalScanner) Destroy(payload any)                   {}
func (FishExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (FishExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (FishExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (FishExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (FishExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (FishExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	// BEGIN_BRACE: { followed by whitespace or ;
	if fishValid(validSymbols, fishTokBeginBrace) {
		for unicode.IsSpace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
		if lexer.Lookahead() == '{' {
			lexer.Advance(false)
			if lexer.Lookahead() == ';' || unicode.IsSpace(lexer.Lookahead()) {
				lexer.MarkEnd()
				lexer.SetResultSymbol(fishSymBeginBrace)
				return true
			}
		}
	}

	// CONCAT_LIST: next char is [
	if fishValid(validSymbols, fishTokConcatList) {
		if lexer.Lookahead() == '[' {
			lexer.SetResultSymbol(fishSymConcatList)
			return true
		}
	}

	// CONCAT: not followed by certain chars
	if fishValid(validSymbols, fishTokConcat) {
		ch := lexer.Lookahead()
		if ch != 0 && ch != '>' && ch != '<' && ch != ')' &&
			ch != ';' && ch != '&' && ch != '|' && !unicode.IsSpace(ch) {
			lexer.SetResultSymbol(fishSymConcat)
			return true
		}
	}

	// BRACKET_CONCAT
	if fishValid(validSymbols, fishTokBracketConcat) {
		ch := lexer.Lookahead()
		if ch != 0 && ch != ')' && ch != '(' && ch != '}' &&
			ch != ',' && !unicode.IsSpace(ch) {
			lexer.SetResultSymbol(fishSymBracketConcat)
			return true
		}
	}

	return false
}

func fishValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
