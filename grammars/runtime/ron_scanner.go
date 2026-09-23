//go:build !grammar_subset || grammar_subset_ron

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the ron grammar.
const (
	ronTokStringContent = 0
	ronTokRawString     = 1
	ronTokFloat         = 2
	ronTokBlockComment  = 3
	ronTokenCount       = 4
)

// ronDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped ron.bin assigns to each external, in ronTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var ronDefaultSymTable = [ronTokenCount]gotreesitter.Symbol{
	23, // _string_content
	24, // raw_string
	25, // float
	26, // block_comment
}

// ronExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (ronTok* order).
var ronExternalScannerSpec = ExternalScannerSpec{
	Language:       "ron",
	UpstreamRepo:   "https://github.com/amaanq/tree-sitter-ron",
	UpstreamCommit: "78938553b93075e638035f624973083451b29055",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "59bda7aad8629c2583d211fcdfa14b5a21a7eea838c6c57454547d89e04f277b"},
		{Path: "src/scanner.c", SHA256: "5863e811087bcc0c3fa9d975642c4871528fa3655a87b361de56b239769a2985"},
	},
	Externals: []string{
		"_string_content",
		"raw_string",
		"float",
		"block_comment",
	},
}

func init() {
	RegisterExternalScannerSpec(ronExternalScannerSpec)
}

// RonExternalScanner handles string content, raw strings, floats,
// and nestable block comments for RON (Rusty Object Notation).
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type RonExternalScanner struct {
	symbols         [ronTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers ron's external symbols.
func (RonExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := RonExternalScanner{symbols: ronDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, ronExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s RonExternalScanner) symbolTable() *[ronTokenCount]gotreesitter.Symbol {
	if s.symbols == ([ronTokenCount]gotreesitter.Symbol{}) {
		return &ronDefaultSymTable
	}
	return &s.symbols
}

func (RonExternalScanner) Create() any                           { return nil }
func (RonExternalScanner) Destroy(payload any)                   {}
func (RonExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (RonExternalScanner) Deserialize(payload any, buf []byte)   {}
func (RonExternalScanner) SupportsIncrementalReuse() bool        { return true }
func (RonExternalScanner) ExternalScannerIsStateless() bool      { return true }

func (sc RonExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [ronTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < ronTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	// String content (inside a "..." string)
	if ronValid(validSymbols, ronTokStringContent) && !ronValid(validSymbols, ronTokFloat) {
		hasContent := false
		for {
			ch := lexer.Lookahead()
			if ch == '"' || ch == '\\' {
				break
			}
			if ch == 0 {
				return false
			}
			hasContent = true
			lexer.Advance(false)
		}
		lexer.SetResultSymbol(syms[ronTokStringContent])
		return hasContent
	}

	// Skip whitespace
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	// Raw string: r#"..."# or br#"..."#
	if ronValid(validSymbols, ronTokRawString) && (lexer.Lookahead() == 'r' || lexer.Lookahead() == 'b') {
		lexer.SetResultSymbol(syms[ronTokRawString])
		if lexer.Lookahead() == 'b' {
			lexer.Advance(false)
		}
		if lexer.Lookahead() != 'r' {
			return false
		}
		lexer.Advance(false)

		openingHashes := uint32(0)
		for lexer.Lookahead() == '#' {
			lexer.Advance(false)
			openingHashes++
		}
		if lexer.Lookahead() != '"' {
			return false
		}
		lexer.Advance(false)

		for {
			if lexer.Lookahead() == 0 {
				return false
			}
			if lexer.Lookahead() == '"' {
				lexer.Advance(false)
				hashCount := uint32(0)
				for lexer.Lookahead() == '#' && hashCount < openingHashes {
					lexer.Advance(false)
					hashCount++
				}
				if hashCount == openingHashes {
					lexer.MarkEnd()
					return true
				}
			} else {
				lexer.Advance(false)
			}
		}
	}

	// Float literal
	if ronValid(validSymbols, ronTokFloat) && unicode.IsDigit(lexer.Lookahead()) {
		lexer.SetResultSymbol(syms[ronTokFloat])
		lexer.Advance(false)
		for ronIsNumChar(lexer.Lookahead()) {
			lexer.Advance(false)
		}

		hasFraction := false
		hasExponent := false

		if lexer.Lookahead() == '.' {
			hasFraction = true
			lexer.Advance(false)
			if unicode.IsLetter(lexer.Lookahead()) {
				return false
			}
			if lexer.Lookahead() == '.' {
				return false
			}
			for ronIsNumChar(lexer.Lookahead()) {
				lexer.Advance(false)
			}
		}

		lexer.MarkEnd()

		if lexer.Lookahead() == 'e' || lexer.Lookahead() == 'E' {
			hasExponent = true
			lexer.Advance(false)
			if lexer.Lookahead() == '+' || lexer.Lookahead() == '-' {
				lexer.Advance(false)
			}
			if !ronIsNumChar(lexer.Lookahead()) {
				return true
			}
			lexer.Advance(false)
			for ronIsNumChar(lexer.Lookahead()) {
				lexer.Advance(false)
			}
			lexer.MarkEnd()
		}

		if !hasExponent && !hasFraction {
			return false
		}

		if lexer.Lookahead() != 'u' && lexer.Lookahead() != 'i' && lexer.Lookahead() != 'f' {
			return true
		}
		lexer.Advance(false)
		if !unicode.IsDigit(lexer.Lookahead()) {
			return true
		}
		for unicode.IsDigit(lexer.Lookahead()) {
			lexer.Advance(false)
		}
		lexer.MarkEnd()
		return true
	}

	// Nestable block comment: /* ... */
	if lexer.Lookahead() == '/' {
		lexer.Advance(false)
		if lexer.Lookahead() != '*' {
			return false
		}
		lexer.Advance(false)

		afterStar := false
		depth := uint32(1)
		for depth > 0 {
			ch := lexer.Lookahead()
			switch {
			case ch == 0:
				return false
			case ch == '*':
				lexer.Advance(false)
				afterStar = true
			case ch == '/':
				if afterStar {
					lexer.Advance(false)
					afterStar = false
					depth--
				} else {
					lexer.Advance(false)
					afterStar = false
					if lexer.Lookahead() == '*' {
						depth++
						lexer.Advance(false)
					}
				}
			default:
				lexer.Advance(false)
				afterStar = false
			}
		}
		lexer.MarkEnd()
		lexer.SetResultSymbol(syms[ronTokBlockComment])
		return true
	}

	return false
}

func ronIsNumChar(ch rune) bool {
	return ch == '_' || unicode.IsDigit(ch)
}

func ronValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
