//go:build !grammar_subset || grammar_subset_editorconfig

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the editorconfig grammar.
const (
	editorconfigTokEndOfFile         = 0
	editorconfigTokIntegerRangeStart = 1
	editorconfigTokenCount           = 2
)

// editorconfigDefaultSymTable seeds a scanner value that is used without
// going through ExternalScannerForLanguage first. Production attachment
// always calls ExternalScannerForLanguage, which rebinds these slots
// positionally against the loaded Language's ExternalSymbols (see
// editorconfigExternalScannerSpec and bindExternalScannerSpec). These two
// values match the editorconfig.bin blob shipped on 2026-09-20; keep them in
// step with ExternalSymbols[0:2] if that ever changes.
var editorconfigDefaultSymTable = [editorconfigTokenCount]gotreesitter.Symbol{
	23, // _end_of_file
	24, // _integer_range_start
}

var editorconfigExternalScannerSpec = ExternalScannerSpec{
	Language:       "editorconfig",
	UpstreamRepo:   "https://github.com/ValdezFOmar/tree-sitter-editorconfig",
	UpstreamCommit: "63f104dab268a25237f773323c172a4a380a00e1",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "1b1a23c5bf5756998424f77bacea1aa8fb2b90863680d9b2ba68ecac41741e15"},
		{Path: "src/scanner.c", SHA256: "53769dba08b8676d775df8e3b96700fd7bdec0d6f88e8428f29dff293ed66730"},
	},
	Externals: []string{
		"_end_of_file",
		"_integer_range_start",
	},
}

func init() {
	RegisterExternalScannerSpec(editorconfigExternalScannerSpec)
}

// EditorconfigExternalScanner handles EOF and integer-range detection for .editorconfig files.
type EditorconfigExternalScanner struct {
	symbols         [editorconfigTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's two token slots to the
// loaded Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers editorconfig's external symbols.
func (EditorconfigExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := EditorconfigExternalScanner{symbols: editorconfigDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, editorconfigExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (EditorconfigExternalScanner) Create() any                           { return nil }
func (EditorconfigExternalScanner) Destroy(payload any)                   {}
func (EditorconfigExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (EditorconfigExternalScanner) Deserialize(payload any, buf []byte)   {}

// The scanner carries no payload and derives every result from local
// lookahead plus validSymbols, so every incremental boundary is quiescent and
// failed scans cannot mutate persistent state.
func (EditorconfigExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (EditorconfigExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (EditorconfigExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (s EditorconfigExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(s.externalToToken) > 0 {
		var semanticValid [editorconfigTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < editorconfigTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	symbols := s.symbolTable()

	eofValid := editorconfigValid(validSymbols, editorconfigTokEndOfFile)
	intValid := editorconfigValid(validSymbols, editorconfigTokIntegerRangeStart)

	// Error recovery: both valid at once
	if eofValid && intValid {
		return false
	}

	if eofValid && lexer.Lookahead() == 0 {
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.SetResultSymbol(symbols[editorconfigTokEndOfFile])
		return true
	}

	if intValid {
		return editorconfigScanIntegerRange(lexer, symbols)
	}

	return false
}

func (s EditorconfigExternalScanner) symbolTable() *[editorconfigTokenCount]gotreesitter.Symbol {
	if s.symbols == ([editorconfigTokenCount]gotreesitter.Symbol{}) {
		return &editorconfigDefaultSymTable
	}
	return &s.symbols
}

func editorconfigScanIntegerRange(lexer *gotreesitter.ExternalLexer, symbols *[editorconfigTokenCount]gotreesitter.Symbol) bool {
	prev := lexer.Lookahead()
	lexer.Advance(false)

	if !isDigitRune(prev) && !(prev == '-' && isDigitRune(lexer.Lookahead())) {
		return false
	}

	for isDigitRune(lexer.Lookahead()) {
		lexer.Advance(false)
	}
	lexer.MarkEnd()

	prev = lexer.Lookahead()
	lexer.Advance(false)
	if !(prev == '.' && lexer.Lookahead() == '.') {
		return false
	}

	lexer.SetResultSymbol(symbols[editorconfigTokIntegerRangeStart])
	return true
}

func isDigitRune(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

func editorconfigValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
