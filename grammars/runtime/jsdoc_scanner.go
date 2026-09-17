//go:build !grammar_subset || grammar_subset_jsdoc

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the jsdoc grammar.
const (
	jsdocTokType          = 0 // "type" — content between { and }
	jsdocTokCodeBlockLine = 1 // "code_block_line"
)

// Concrete symbol IDs from the generated jsdoc grammar ExternalSymbols.
const (
	jsdocSymType          gotreesitter.Symbol = 24
	jsdocSymCodeBlockLine gotreesitter.Symbol = 17
)

// JsdocExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-jsdoc.
//
// The jsdoc grammar uses an external scanner to match the "type" token,
// which represents text inside a JSDoc type annotation between { and }.
// The scanner tracks brace nesting depth and consumes all content until
// the unmatched closing } is found.
type JsdocExternalScanner struct{}

func (JsdocExternalScanner) Create() any                           { return nil }
func (JsdocExternalScanner) Destroy(payload any)                   {}
func (JsdocExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (JsdocExternalScanner) Deserialize(payload any, buf []byte)   {}

func (JsdocExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if jsdocValid(validSymbols, jsdocTokType) {
		return scanJsdocType(lexer)
	}
	return false
}

// scanJsdocType scans a type token inside { ... }. It consumes everything
// until the unmatched closing brace, tracking nested { } pairs. Returns
// false on EOF or newline.
func scanJsdocType(lexer *gotreesitter.ExternalLexer) bool {
	stack := 0
	for {
		ch := lexer.Lookahead()
		switch {
		case ch == 0: // EOF
			return false
		case ch == '{':
			stack++
		case ch == '}':
			stack--
			if stack == -1 {
				// Found the unmatched closing brace — emit token up to here.
				lexer.MarkEnd()
				lexer.SetResultSymbol(jsdocSymType)
				return true
			}
		case ch == '\n' || ch == '\x00':
			return false
		}
		lexer.Advance(false)
	}
}

func jsdocValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
