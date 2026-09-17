//go:build !grammar_subset || grammar_subset_haxe

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the haxe grammar.
const (
	haxeTokLookbackSemicolon    = 0
	haxeTokClosingBraceMarker   = 1
	haxeTokClosingBraceUnmarker = 2
)

const (
	haxeSymLookbackSemicolon    gotreesitter.Symbol = 116
	haxeSymClosingBraceMarker   gotreesitter.Symbol = 117
	haxeSymClosingBraceUnmarker gotreesitter.Symbol = 118
)

// haxeState tracks whether a closing brace was just seen.
type haxeState struct {
	justSawBrace bool
}

// HaxeExternalScanner handles lookback semicolons and closing brace
// detection for Haxe automatic semicolon insertion.
type HaxeExternalScanner struct{}

func (HaxeExternalScanner) Create() any         { return &haxeState{} }
func (HaxeExternalScanner) Destroy(payload any) {}
func (HaxeExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*haxeState)
	if s.justSawBrace {
		buf[0] = 1
	} else {
		buf[0] = 0
	}
	return 1
}
func (HaxeExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*haxeState)
	if len(buf) > 0 {
		s.justSawBrace = buf[0] != 0
	} else {
		s.justSawBrace = false
	}
}

func (HaxeExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*haxeState)

	if haxeValid(validSymbols, haxeTokLookbackSemicolon) {
		if lexer.Lookahead() == ';' {
			s.justSawBrace = false
			lexer.SetResultSymbol(haxeSymLookbackSemicolon)
			lexer.Advance(false)
			return true
		}
		if s.justSawBrace {
			s.justSawBrace = false
			lexer.SetResultSymbol(haxeSymLookbackSemicolon)
			return true
		}
		return false
	}

	if haxeValid(validSymbols, haxeTokClosingBraceMarker) {
		lexer.MarkEnd()
		for isHaxeWhitespace(lexer.Lookahead()) {
			lexer.Advance(true)
		}
		if lexer.Lookahead() == '}' {
			s.justSawBrace = true
			lexer.SetResultSymbol(haxeSymClosingBraceMarker)
			return true
		}
	}

	if haxeValid(validSymbols, haxeTokClosingBraceUnmarker) &&
		s.justSawBrace &&
		lexer.Lookahead() != '}' &&
		!isHaxeWhitespace(lexer.Lookahead()) {
		s.justSawBrace = false
		lexer.SetResultSymbol(haxeSymClosingBraceUnmarker)
		return true
	}

	return false
}

func isHaxeWhitespace(ch rune) bool {
	return ch == ' ' || ch == '\n' || ch == '\t' || ch == '\r'
}

func haxeValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
