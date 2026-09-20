//go:build !grammar_subset || grammar_subset_nix

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the nix grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see nixDefaultSymTable below.
const (
	nixTokStringFragment         = 0
	nixTokIndentedStringFragment = 1
	nixTokPathStart              = 2
	nixTokPathFragment           = 3
	nixTokDollarEscape           = 4
	nixTokIndentedDollarEscape   = 5
	nixTokenCount                = 6
)

// nixDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped nix.bin assigns to each external, in nixTok* order. It
// exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order. _indented_string_fragment aliases string_fragment's display node
// and _path_start aliases path_fragment's display node.
var nixDefaultSymTable = [nixTokenCount]gotreesitter.Symbol{
	56, // string_fragment
	57, // _indented_string_fragment (display: string_fragment)
	58, // _path_start (display: path_fragment)
	59, // path_fragment
	60, // dollar_escape
	61, // _indented_dollar_escape (display: dollar_escape)
}

// nixExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (nixTok* order).
var nixExternalScannerSpec = ExternalScannerSpec{
	Language:       "nix",
	UpstreamRepo:   "https://github.com/nix-community/tree-sitter-nix",
	UpstreamCommit: "17f290c8b5104d9aba8a1ba7383a2ca83c3d14c4",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "40f87f5117abbd9559270b7806b36ecaf28274e930e18522b1516c6847aad889"},
		{Path: "src/scanner.c", SHA256: "45e12521e8be62ea47525417c64d9372fa34e9e12bff643c430b8b3607be9d56"},
	},
	Externals: []string{
		"string_fragment",
		"_indented_string_fragment",
		"_path_start",
		"path_fragment",
		"dollar_escape",
		"_indented_dollar_escape",
	},
}

func init() {
	RegisterExternalScannerSpec(nixExternalScannerSpec)
}

// NixExternalScanner handles string fragments, path literals, and dollar
// escapes for the Nix expression language.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type NixExternalScanner struct {
	symbols         [nixTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers nix's external symbols.
func (NixExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := NixExternalScanner{symbols: nixDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, nixExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s NixExternalScanner) symbolTable() *[nixTokenCount]gotreesitter.Symbol {
	if s.symbols == ([nixTokenCount]gotreesitter.Symbol{}) {
		return &nixDefaultSymTable
	}
	return &s.symbols
}

func (NixExternalScanner) Create() any                           { return nil }
func (NixExternalScanner) Destroy(payload any)                   {}
func (NixExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (NixExternalScanner) Deserialize(payload any, buf []byte)   {}

// String and path modes come exclusively from validSymbols; no mode or
// delimiter survives a Scan call. Failed scans therefore preserve empty state.
func (NixExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (NixExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (NixExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (s NixExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(s.externalToToken) > 0 {
		var semanticValid [nixTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < nixTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()

	// Error recovery: all valid
	if nixValid(validSymbols, nixTokStringFragment) &&
		nixValid(validSymbols, nixTokIndentedStringFragment) &&
		nixValid(validSymbols, nixTokPathStart) &&
		nixValid(validSymbols, nixTokPathFragment) &&
		nixValid(validSymbols, nixTokDollarEscape) &&
		nixValid(validSymbols, nixTokIndentedDollarEscape) {
		return false
	}

	if nixValid(validSymbols, nixTokStringFragment) {
		if lexer.Lookahead() == '\\' {
			return nixScanDollarEscape(lexer, syms[nixTokDollarEscape])
		}
		return nixScanStringFragment(lexer, syms[nixTokStringFragment])
	}

	if nixValid(validSymbols, nixTokIndentedStringFragment) {
		if lexer.Lookahead() == '\'' {
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == '\'' {
				return nixScanIndentedDollarEscape(lexer, syms[nixTokIndentedDollarEscape])
			}
		}
		return nixScanIndentedStringFragment(lexer, syms[nixTokIndentedStringFragment])
	}

	if nixValid(validSymbols, nixTokPathFragment) && nixIsPathChar(lexer.Lookahead()) {
		return nixScanPathFragment(lexer, syms[nixTokPathFragment])
	}

	if nixValid(validSymbols, nixTokPathStart) {
		return nixScanPathStart(lexer, syms[nixTokPathFragment], syms[nixTokPathStart])
	}

	return false
}

func nixScanDollarEscape(lexer *gotreesitter.ExternalLexer, dollarEscapeSym gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(dollarEscapeSym)
	lexer.Advance(false)
	lexer.MarkEnd()
	return lexer.Lookahead() == '$'
}

func nixScanIndentedDollarEscape(lexer *gotreesitter.ExternalLexer, indentedDollarEscapeSym gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(indentedDollarEscapeSym)
	lexer.Advance(false)
	lexer.MarkEnd()
	if lexer.Lookahead() == '$' {
		return true
	}
	if lexer.Lookahead() == '\\' {
		lexer.Advance(false)
		if lexer.Lookahead() == '$' {
			lexer.MarkEnd()
			return true
		}
	}
	return false
}

func nixScanStringFragment(lexer *gotreesitter.ExternalLexer, stringFragmentSym gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(stringFragmentSym)
	hasContent := false
	for {
		lexer.MarkEnd()
		switch lexer.Lookahead() {
		case '"', '\\':
			return hasContent
		case '$':
			lexer.Advance(false)
			if lexer.Lookahead() == '{' {
				return hasContent
			}
			if lexer.Lookahead() != '"' && lexer.Lookahead() != '\\' {
				lexer.Advance(false)
			}
		case 0:
			return false
		default:
			lexer.Advance(false)
		}
		hasContent = true
	}
}

func nixScanIndentedStringFragment(lexer *gotreesitter.ExternalLexer, indentedStringFragmentSym gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(indentedStringFragmentSym)
	hasContent := false
	for {
		lexer.MarkEnd()
		switch lexer.Lookahead() {
		case '$':
			lexer.Advance(false)
			if lexer.Lookahead() == '{' {
				return hasContent
			}
			if lexer.Lookahead() != '\'' {
				lexer.Advance(false)
			}
		case '\'':
			lexer.Advance(false)
			if lexer.Lookahead() == '\'' {
				return hasContent
			}
		case 0:
			return false
		default:
			lexer.Advance(false)
		}
		hasContent = true
	}
}

func nixIsPathChar(ch rune) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') || ch == '-' || ch == '+' ||
		ch == '_' || ch == '.' || ch == '/'
}

func nixScanPathStart(lexer *gotreesitter.ExternalLexer, pathFragmentSym, pathStartSym gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(pathFragmentSym) // path_start uses the same result sym=58
	// Actually, let me use the correct symbol
	lexer.SetResultSymbol(pathStartSym)

	// Skip leading whitespace
	for {
		ch := lexer.Lookahead()
		if ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t' {
			lexer.Advance(true)
		} else {
			break
		}
	}

	haveSep := false
	haveAfterSep := false
	for {
		lexer.MarkEnd()
		ch := lexer.Lookahead()
		if ch == '/' {
			haveSep = true
		} else if nixIsPathChar(ch) {
			if haveSep {
				haveAfterSep = true
			}
		} else if ch == '$' {
			return haveSep
		} else {
			return haveAfterSep
		}
		lexer.Advance(false)
	}
}

func nixScanPathFragment(lexer *gotreesitter.ExternalLexer, pathFragmentSym gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(pathFragmentSym)
	hasContent := false
	for {
		lexer.MarkEnd()
		if !nixIsPathChar(lexer.Lookahead()) {
			return hasContent
		}
		lexer.Advance(false)
		hasContent = true
	}
}

func nixValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
