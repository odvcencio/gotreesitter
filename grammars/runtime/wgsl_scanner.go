//go:build !grammar_subset || grammar_subset_wgsl

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the wgsl grammar.
const (
	wgslTokBlockComment = 0
	wgslTokenCount      = 1
)

// wgslDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped wgsl.bin assigns to each external, in wgslTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var wgslDefaultSymTable = [wgslTokenCount]gotreesitter.Symbol{
	135, // block_comment
}

// wgslExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (wgslTok* order).
var wgslExternalScannerSpec = ExternalScannerSpec{
	Language:       "wgsl",
	UpstreamRepo:   "https://github.com/szebniok/tree-sitter-wgsl",
	UpstreamCommit: "40259f3c77ea856841a4e0c4c807705f3e4a2b65",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "d953f97c338e3998c229cc145ac8f4a5d711383416282748fec567073d67aefc"},
		{Path: "src/scanner.c", SHA256: "3abd8bbe927f047de74c1b1016a80dbaaff17d181263eedace58bf4f90a6b7b6"},
	},
	Externals: []string{
		"block_comment",
	},
}

func init() {
	RegisterExternalScannerSpec(wgslExternalScannerSpec)
}

// WgslExternalScanner handles nestable /* */ block comments for WGSL.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type WgslExternalScanner struct {
	symbols         [wgslTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers wgsl's external symbols.
func (WgslExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := WgslExternalScanner{symbols: wgslDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, wgslExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s WgslExternalScanner) symbolTable() *[wgslTokenCount]gotreesitter.Symbol {
	if s.symbols == ([wgslTokenCount]gotreesitter.Symbol{}) {
		return &wgslDefaultSymTable
	}
	return &s.symbols
}

func (WgslExternalScanner) Create() any                           { return nil }
func (WgslExternalScanner) Destroy(payload any)                   {}
func (WgslExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (WgslExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (WgslExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (WgslExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (WgslExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (sc WgslExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [wgslTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < wgslTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	if !wgslValid(validSymbols, wgslTokBlockComment) {
		return false
	}

	// Mirror C scanner: skip leading whitespace before the comment opener so
	// the external token may be recognized at any position the parser offers
	// it (indented comments, comments following a token + space, a second
	// comment after `/* a */ `, etc.). C advances with skip=true so the
	// skipped whitespace is excluded from the token span.
	for wgslIsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	if lexer.Lookahead() != '/' {
		return false
	}
	lexer.Advance(false)

	if lexer.Lookahead() != '*' {
		return false
	}
	lexer.Advance(false)

	// Nestable /* */ comment. Byte-faithful port of the C state machine:
	// on '/' followed by '*' the depth increases; on '*' followed by '/' the
	// depth decreases, and reaching depth 0 emits the token. End-of-input
	// before closing returns false (no token) so an unterminated comment is
	// surfaced as an error, exactly as the C scanner does.
	commentDepth := 1
	for {
		ch := lexer.Lookahead()
		switch {
		case ch == '/':
			lexer.Advance(false)
			if lexer.Lookahead() == '*' {
				lexer.Advance(false)
				commentDepth++
			}
		case ch == '*':
			lexer.Advance(false)
			if lexer.Lookahead() == '/' {
				lexer.Advance(false)
				commentDepth--
				if commentDepth == 0 {
					lexer.SetResultSymbol(syms[wgslTokBlockComment])
					return true
				}
			}
		case ch == 0:
			// End of input reached before the comment closed.
			return false
		default:
			lexer.Advance(false)
		}
	}
}

func wgslValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }

// wgslIsSpace mirrors C's iswspace for the ASCII whitespace the WGSL scanner
// skips before a block comment opener.
func wgslIsSpace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	default:
		return false
	}
}
