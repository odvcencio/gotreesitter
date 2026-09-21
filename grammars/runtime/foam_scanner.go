//go:build !grammar_subset || grammar_subset_foam

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the foam grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner
// never hardcodes them -- see foamDefaultSymTable below.
const (
	foamTokIdentifier = 0 // "identifier"
	foamTokBoolean    = 1 // "boolean"
	foamTokEOF        = 2 // "_eof"
	foamTokenCount    = 3
)

// foamDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped foam.bin assigns to each external, in foamTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var foamDefaultSymTable = [foamTokenCount]gotreesitter.Symbol{
	35, // identifier
	36, // boolean
	37, // _eof
}

// foamExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (foamTok* order).
var foamExternalScannerSpec = ExternalScannerSpec{
	Language:       "foam",
	UpstreamRepo:   "https://github.com/FoamScience/tree-sitter-foam",
	UpstreamCommit: "472c24f11a547820327fb1be565bcfff98ea96a4",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "553538422e6f0d33c69b695fd4322ad6d51f134206aca6ef5b5bdd92252dce5d"},
		{Path: "src/scanner.c", SHA256: "71ebd7e89f57905784e2d0f0a8f179aadb3c66eea9d923b83eef059ae2ae1223"},
	},
	Externals: []string{
		"identifier",
		"boolean",
		"_eof",
	},
}

func init() {
	RegisterExternalScannerSpec(foamExternalScannerSpec)
}

// FoamExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-foam.
//
// This is a Go port of the C external scanner from tree-sitter-foam
// (https://github.com/FoamScience/tree-sitter-foam). The scanner handles:
//   - identifier: OpenFOAM identifiers (keyword names, paths, etc.)
//   - boolean: "on", "off", "true", "false"
//   - _eof: end-of-file marker
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type FoamExternalScanner struct {
	symbols         [foamTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers foam's external symbols.
func (FoamExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := FoamExternalScanner{symbols: foamDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, foamExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s FoamExternalScanner) symbolTable() *[foamTokenCount]gotreesitter.Symbol {
	if s.symbols == ([foamTokenCount]gotreesitter.Symbol{}) {
		return &foamDefaultSymTable
	}
	return &s.symbols
}

func (FoamExternalScanner) Create() any                           { return nil }
func (FoamExternalScanner) Destroy(payload any)                   {}
func (FoamExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (FoamExternalScanner) Deserialize(payload any, buf []byte)   {}
func (FoamExternalScanner) SupportsIncrementalReuse() bool        { return true }
func (FoamExternalScanner) ExternalScannerIsStateless() bool      { return true }

func (sc FoamExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [foamTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < foamTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

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
			lexer.SetResultSymbol(syms[foamTokEOF])
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
					lexer.SetResultSymbol(syms[foamTokBoolean])
					return true
				}
				continue
			}
		}

		lexer.Advance(false)
	}

	// Return as identifier if the parser wants one.
	if foamValid(validSymbols, foamTokIdentifier) {
		lexer.SetResultSymbol(syms[foamTokIdentifier])
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
