//go:build !grammar_subset || grammar_subset_cue

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the cue grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see cueDefaultSymTable below.
const (
	cueTokMultiStrContent      = 0
	cueTokMultiBytesContent    = 1
	cueTokRawStrContent        = 2
	cueTokRawBytesContent      = 3
	cueTokMultiRawStrContent   = 4
	cueTokMultiRawBytesContent = 5
	cueTokenCount              = 6
)

// cueDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped cue.bin assigns to each external, in cueTok* order. It
// exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var cueDefaultSymTable = [cueTokenCount]gotreesitter.Symbol{
	95,  // _multi_str_content
	96,  // _multi_bytes_content
	97,  // _raw_str_content
	98,  // _raw_bytes_content
	99,  // _multi_raw_str_content
	100, // _multi_raw_bytes_content
}

// cueExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (cueTok* order).
var cueExternalScannerSpec = ExternalScannerSpec{
	Language:       "cue",
	UpstreamRepo:   "https://github.com/eonpatapon/tree-sitter-cue",
	UpstreamCommit: "be0f609c73cc2929811a9bce0ed90ca71ea87604",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "b7e5aacfc5da04fac8d24507b324d784df70848728c1c44f718b8269c8c3755e"},
		{Path: "src/scanner.c", SHA256: "fdfb86c8525d1ffa84e3c618c75b3b11d4a619d62c0444a21396588380d577db"},
	},
	Externals: []string{
		"_multi_str_content",
		"_multi_bytes_content",
		"_raw_str_content",
		"_raw_bytes_content",
		"_multi_raw_str_content",
		"_multi_raw_bytes_content",
	},
}

func init() {
	RegisterExternalScannerSpec(cueExternalScannerSpec)
}

// CueExternalScanner handles string content scanning for CUE's various
// string types: multi-line, raw, and multi-line raw strings/bytes.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type CueExternalScanner struct {
	symbols         [cueTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers cue's external symbols.
func (CueExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := CueExternalScanner{symbols: cueDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, cueExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s CueExternalScanner) symbolTable() *[cueTokenCount]gotreesitter.Symbol {
	if s.symbols == ([cueTokenCount]gotreesitter.Symbol{}) {
		return &cueDefaultSymTable
	}
	return &s.symbols
}

func (CueExternalScanner) Create() any                           { return nil }
func (CueExternalScanner) Destroy(payload any)                   {}
func (CueExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (CueExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (CueExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (CueExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (CueExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (s CueExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(s.externalToToken) > 0 {
		var semanticValid [cueTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < cueTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()

	if cueValid(validSymbols, cueTokMultiStrContent) {
		return cueScanMultiline(lexer, '"', syms[cueTokMultiStrContent])
	}
	if cueValid(validSymbols, cueTokMultiBytesContent) {
		return cueScanMultiline(lexer, '\'', syms[cueTokMultiBytesContent])
	}
	if cueValid(validSymbols, cueTokMultiRawStrContent) {
		return cueScanRawMultiline(lexer, '"', syms[cueTokMultiRawStrContent])
	}
	if cueValid(validSymbols, cueTokMultiRawBytesContent) {
		return cueScanRawMultiline(lexer, '\'', syms[cueTokMultiRawBytesContent])
	}
	if cueValid(validSymbols, cueTokRawStrContent) {
		return cueScanRaw(lexer, '"', syms[cueTokRawStrContent])
	}
	if cueValid(validSymbols, cueTokRawBytesContent) {
		return cueScanRaw(lexer, '\'', syms[cueTokRawBytesContent])
	}
	return false
}

// cueScanMultiline scans content of triple-quoted (""" or ”') strings.
func cueScanMultiline(lexer *gotreesitter.ExternalLexer, delim rune, sym gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(sym)
	hasContent := false
	for {
		ch := lexer.Lookahead()
		switch {
		case ch == delim:
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == delim {
				lexer.Advance(false)
				if lexer.Lookahead() == delim {
					return hasContent
				}
			}
		case ch == '\\':
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == '(' {
				return hasContent
			}
			lexer.Advance(false)
			hasContent = true
		case ch == 0:
			return false
		default:
			lexer.Advance(false)
			hasContent = true
		}
	}
}

// cueScanRawMultiline scans raw multiline strings (""" with # delim).
func cueScanRawMultiline(lexer *gotreesitter.ExternalLexer, delim rune, sym gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(sym)
	hasContent := false
	for {
		ch := lexer.Lookahead()
		switch {
		case ch == delim:
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == delim {
				lexer.Advance(false)
				if lexer.Lookahead() == delim {
					lexer.Advance(false)
					if lexer.Lookahead() == '#' {
						return hasContent
					}
				}
			}
		case ch == '\\':
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == '#' {
				lexer.Advance(false)
				if lexer.Lookahead() == '(' {
					return hasContent
				}
			}
			hasContent = true
		case ch == 0:
			return false
		default:
			lexer.Advance(false)
			hasContent = true
		}
	}
}

// cueScanRaw scans raw string content (single-line with # delim).
func cueScanRaw(lexer *gotreesitter.ExternalLexer, delim rune, sym gotreesitter.Symbol) bool {
	lexer.SetResultSymbol(sym)
	hasContent := false
	for {
		ch := lexer.Lookahead()
		switch {
		case ch == delim:
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == '#' {
				return hasContent
			}
		case ch == '\\':
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead() == '#' {
				lexer.Advance(false)
				if lexer.Lookahead() == '(' {
					return hasContent
				}
			} else {
				lexer.Advance(false)
			}
			hasContent = true
		case ch == 0:
			return false
		default:
			lexer.Advance(false)
			hasContent = true
		}
	}
}

func cueValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
