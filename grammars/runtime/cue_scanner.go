//go:build !grammar_subset || grammar_subset_cue

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the cue grammar.
const (
	cueTokMultiStrContent      = 0
	cueTokMultiBytesContent    = 1
	cueTokRawStrContent        = 2
	cueTokRawBytesContent      = 3
	cueTokMultiRawStrContent   = 4
	cueTokMultiRawBytesContent = 5
)

const (
	cueSymMultiStrContent      gotreesitter.Symbol = 95
	cueSymMultiBytesContent    gotreesitter.Symbol = 96
	cueSymRawStrContent        gotreesitter.Symbol = 97
	cueSymRawBytesContent      gotreesitter.Symbol = 98
	cueSymMultiRawStrContent   gotreesitter.Symbol = 99
	cueSymMultiRawBytesContent gotreesitter.Symbol = 100
)

// CueExternalScanner handles string content scanning for CUE's various
// string types: multi-line, raw, and multi-line raw strings/bytes.
type CueExternalScanner struct{}

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

func (CueExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if cueValid(validSymbols, cueTokMultiStrContent) {
		return cueScanMultiline(lexer, '"', cueSymMultiStrContent)
	}
	if cueValid(validSymbols, cueTokMultiBytesContent) {
		return cueScanMultiline(lexer, '\'', cueSymMultiBytesContent)
	}
	if cueValid(validSymbols, cueTokMultiRawStrContent) {
		return cueScanRawMultiline(lexer, '"', cueSymMultiRawStrContent)
	}
	if cueValid(validSymbols, cueTokMultiRawBytesContent) {
		return cueScanRawMultiline(lexer, '\'', cueSymMultiRawBytesContent)
	}
	if cueValid(validSymbols, cueTokRawStrContent) {
		return cueScanRaw(lexer, '"', cueSymRawStrContent)
	}
	if cueValid(validSymbols, cueTokRawBytesContent) {
		return cueScanRaw(lexer, '\'', cueSymRawBytesContent)
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
