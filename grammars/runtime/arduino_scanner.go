//go:build !grammar_subset || grammar_subset_arduino

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the arduino grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see arduinoDefaultSymTable below.
const (
	arduinoTokRawStringDelimiter = 0
	arduinoTokRawStringContent   = 1
	arduinoTokenCount            = 2
)

// arduinoDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped arduino.bin assigns to each external, in arduinoTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var arduinoDefaultSymTable = [arduinoTokenCount]gotreesitter.Symbol{
	221, // raw_string_delimiter
	222, // raw_string_content
}

// arduinoExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (arduinoTok* order).
var arduinoExternalScannerSpec = ExternalScannerSpec{
	Language:       "arduino",
	UpstreamRepo:   "https://github.com/ObserverOfTime/tree-sitter-arduino",
	UpstreamCommit: "11dd46c9ae25135c473c0003a133bb06a484af0c",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "ebbc790d7ef7dddea44fb6986b8144db9c0b8c05c08d6ada1c059a8a8c1a9d42"},
		{Path: "src/scanner.c", SHA256: "acc6a2092a8012b20937509b76444e82b80aeca6a2333d3e53e87413fb5591d3"},
	},
	Externals: []string{
		"raw_string_delimiter",
		"raw_string_content",
	},
}

func init() {
	RegisterExternalScannerSpec(arduinoExternalScannerSpec)
}

// ArduinoExternalScanner handles C++ R"delim(...)delim" raw string literals
// for Arduino. It reuses the shared rawstring_scanner.go helper (also used
// by cpp, cuda, and hlsl), passing this instance's bound symbols in as
// concrete gotreesitter.Symbol arguments; the shared helper itself never
// hardcodes an absolute Symbol value.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type ArduinoExternalScanner struct {
	symbols         [arduinoTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers arduino's external symbols.
func (ArduinoExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := ArduinoExternalScanner{symbols: arduinoDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, arduinoExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s ArduinoExternalScanner) symbolTable() *[arduinoTokenCount]gotreesitter.Symbol {
	if s.symbols == ([arduinoTokenCount]gotreesitter.Symbol{}) {
		return &arduinoDefaultSymTable
	}
	return &s.symbols
}

func (ArduinoExternalScanner) Create() any         { return rawStringCreate() }
func (ArduinoExternalScanner) Destroy(payload any) {}
func (ArduinoExternalScanner) Serialize(payload any, buf []byte) int {
	return rawStringSerialize(payload, buf)
}
func (ArduinoExternalScanner) Deserialize(payload any, buf []byte) {
	rawStringDeserialize(payload, buf)
}

func (s ArduinoExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(s.externalToToken) > 0 {
		var semanticValid [arduinoTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < arduinoTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()
	return rawStringScan(payload, lexer, validSymbols,
		arduinoTokRawStringDelimiter, arduinoTokRawStringContent,
		syms[arduinoTokRawStringDelimiter], syms[arduinoTokRawStringContent])
}
