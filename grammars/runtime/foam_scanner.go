//go:build !grammar_subset || grammar_subset_foam

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the foam grammar.
const (
	foamTokIdentifier = 0 // "identifier"
	foamTokBoolean    = 1 // "boolean"
	foamTokEOF        = 2 // "_eof"
)

// Concrete symbol IDs from the generated foam grammar ExternalSymbols.
const (
	foamSymIdentifier gotreesitter.Symbol = 35
	foamSymBoolean    gotreesitter.Symbol = 36
	foamSymEOF        gotreesitter.Symbol = 37
)

// FoamExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-foam.
//
// This is a Go port of the C external scanner from tree-sitter-foam
// (https://github.com/FoamScience/tree-sitter-foam). The scanner handles:
//   - identifier: OpenFOAM identifiers (keyword names, paths, etc.)
//   - boolean: "on", "off", "true", "false"
//   - _eof: end-of-file marker
type FoamExternalScanner struct{}

func (FoamExternalScanner) Create() any                           { return nil }
func (FoamExternalScanner) Destroy(payload any)                   {}
func (FoamExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (FoamExternalScanner) Deserialize(payload any, buf []byte)   {}
func (FoamExternalScanner) SupportsIncrementalReuse() bool        { return true }
func (FoamExternalScanner) ExternalScannerIsStateless() bool      { return true }

func (FoamExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	// Skip whitespace (matching original C scanner behavior).
	for isFoamWhitespace(lexer.Lookahead()) && lexer.Lookahead() != 0 {
		lexer.Advance(true) // skip=true: excluded from token span
	}

	// After skipping whitespace, check if the current char can start an identifier.
	ch := lexer.Lookahead()
	if !isFoamAlpha(ch) && ch != '_' {
		// Not an identifier start. Check for EOF.
		if ch == 0 && foamValid(validSymbols, foamTokEOF) {
			lexer.MarkEnd()
			lexer.SetResultSymbol(foamSymEOF)
			return true
		}
		return false
	}

	// Begin scanning an identifier/boolean.
	// Track the first 5 characters for boolean keyword matching.
	var currentIdent [6]byte // null-terminated, max 5 chars for boolean check
	nestingLevel := 0
	idx := 0

	// Consume the first character.
	if idx < 5 {
		currentIdent[idx] = byte(ch)
		idx++
	}
	lexer.Advance(false)

	// Scan the rest of the identifier.
	for {
		ch = lexer.Lookahead()

		if ch == 0 {
			// EOF: end the identifier here.
			lexer.MarkEnd()
			break
		}

		// Stop if non-identifier char and nesting level is 0,
		// or if nesting level falls below 0 (extra ')').
		if isFoamNonIdentChar(ch) && nestingLevel == 0 {
			lexer.MarkEnd()
			break
		}

		if ch == '(' {
			nestingLevel++
		} else if ch == ')' {
			nestingLevel--
			if nestingLevel == -1 {
				lexer.MarkEnd()
				break
			}
		}

		// Build up the boolean candidate string.
		if idx < 5 {
			currentIdent[idx] = byte(ch)
			idx++
			word := string(currentIdent[:idx])
			if foamIsBooleanKeyword(word) {
				// Consume the current rune and only emit a boolean token if
				// the keyword is a full token (not an identifier prefix).
				lexer.Advance(false)
				next := lexer.Lookahead()
				if foamWouldTerminateIdentifier(next, nestingLevel) && foamValid(validSymbols, foamTokBoolean) {
					lexer.MarkEnd()
					lexer.SetResultSymbol(foamSymBoolean)
					return true
				}
				continue
			}
		}

		lexer.Advance(false)
	}

	// Return as identifier if the parser wants one.
	if foamValid(validSymbols, foamTokIdentifier) {
		lexer.SetResultSymbol(foamSymIdentifier)
		return true
	}

	return false
}

// foamValid checks if the external token at the given index is valid.
func foamValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}

// isFoamWhitespace returns true for whitespace characters.
func isFoamWhitespace(ch rune) bool {
	switch ch {
	case ' ', '\t', '\n', '\r', '\f', '\x0b':
		return true
	}
	return false
}

// isFoamAlpha returns true for alphabetic characters.
func isFoamAlpha(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

// isFoamNonIdentChar returns true for characters that cannot appear in a foam
// identifier (matching the C scanner's non_identifier_char function).
func isFoamNonIdentChar(ch rune) bool {
	switch ch {
	case '"', '\'', ';', '$', '#', ' ',
		'{', '}', '[', ']',
		'\t', '\n', '\r', '\f', '\x0b', 0:
		return true
	}
	return false
}

func foamIsBooleanKeyword(word string) bool {
	switch word {
	case "on", "off", "true", "false":
		return true
	default:
		return false
	}
}

func foamWouldTerminateIdentifier(ch rune, nestingLevel int) bool {
	if ch == 0 {
		return true
	}
	if isFoamNonIdentChar(ch) && nestingLevel == 0 {
		return true
	}
	return ch == ')' && nestingLevel == 0
}
