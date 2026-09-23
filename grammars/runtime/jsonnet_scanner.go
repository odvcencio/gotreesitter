//go:build !grammar_subset || grammar_subset_jsonnet

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the jsonnet grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see jsonnetDefaultSymTable below.
const (
	jsonnetTokStringStart   = 0
	jsonnetTokStringContent = 1
	jsonnetTokStringEnd     = 2
	jsonnetTokenCount       = 3
)

// jsonnetDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped jsonnet.bin assigns to each external, in
// jsonnetTok* order. It exists only as a pre-bind fallback (and as an
// independent value to compare a real bind against in tests);
// ExternalScannerForLanguage below overwrites it with values read from
// the actual loaded Language at bind time, which is what the scanner
// must do to survive a future blob regen that renumbers absolute symbol
// IDs without touching the externals list order.
var jsonnetDefaultSymTable = [jsonnetTokenCount]gotreesitter.Symbol{
	61, // _string_start, displays as "string_start"
	62, // _string_content, displays as "string_content"
	63, // _string_end, displays as "string_end"
}

// jsonnetExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (jsonnetTok* order).
var jsonnetExternalScannerSpec = ExternalScannerSpec{
	Language:       "jsonnet",
	UpstreamRepo:   "https://github.com/sourcegraph/tree-sitter-jsonnet",
	UpstreamCommit: "ddd075f1939aed8147b7aa67f042eda3fce22790",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "f0c19e4b9f0a743be943e3968bd04cc16234c25dc9533f6eb0db3fb1121f755e"},
		{Path: "src/scanner.c", SHA256: "04e98af9fbb1463853d40a5faa7dca36c41b3938c5eb0fe6ab1b222fdeb24abc"},
	},
	Externals: []string{
		"_string_start",
		"_string_content",
		"_string_end",
	},
}

func init() {
	RegisterExternalScannerSpec(jsonnetExternalScannerSpec)
}

// jsonnetState tracks whether we're inside a string and what delimiter ends it.
type jsonnetState struct {
	insideString bool
	endingChar   rune // 0 for ||| block strings, '\'' or '"' for quoted
	levelCount   uint8
}

// JsonnetExternalScanner handles Jsonnet string literals including
// single/double quoted strings and ||| block strings.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type JsonnetExternalScanner struct {
	symbols         [jsonnetTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers jsonnet's external symbols.
func (JsonnetExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := JsonnetExternalScanner{symbols: jsonnetDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, jsonnetExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s JsonnetExternalScanner) symbolTable() *[jsonnetTokenCount]gotreesitter.Symbol {
	if s.symbols == ([jsonnetTokenCount]gotreesitter.Symbol{}) {
		return &jsonnetDefaultSymTable
	}
	return &s.symbols
}

func (JsonnetExternalScanner) Create() any         { return &jsonnetState{} }
func (JsonnetExternalScanner) Destroy(payload any) {}
func (JsonnetExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*jsonnetState)
	if s.insideString {
		buf[0] = 1
	} else {
		buf[0] = 0
	}
	buf[1] = byte(s.endingChar)
	buf[2] = s.levelCount
	return 3
}
func (JsonnetExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*jsonnetState)
	if len(buf) == 0 {
		return
	}
	s.insideString = buf[0] != 0
	if len(buf) >= 2 {
		s.endingChar = rune(buf[1])
	}
	if len(buf) >= 3 {
		s.levelCount = buf[2]
	}
}

func (sc JsonnetExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*jsonnetState)

	syms := sc.symbolTable()

	if s.insideString {
		if jsonnetValid(validSymbols, jsonnetTokStringEnd) && jsonnetScanStringEnd(s, lexer) {
			s.insideString = false
			s.endingChar = 0
			s.levelCount = 0
			lexer.SetResultSymbol(syms[jsonnetTokStringEnd])
			return true
		}
		if jsonnetValid(validSymbols, jsonnetTokStringContent) && jsonnetScanStringContent(s, lexer) {
			lexer.SetResultSymbol(syms[jsonnetTokStringContent])
			return true
		}
		return false
	}

	// Skip whitespace
	for unicode.IsSpace(lexer.Lookahead()) {
		lexer.Advance(true)
	}

	if jsonnetValid(validSymbols, jsonnetTokStringStart) && jsonnetScanStringStart(s, lexer) {
		lexer.SetResultSymbol(syms[jsonnetTokStringStart])
		return true
	}

	return false
}

func jsonnetScanBlockStart(lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != '|' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '|' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '|' {
		return false
	}
	lexer.Advance(false)
	return true
}

func jsonnetScanBlockEnd(lexer *gotreesitter.ExternalLexer) bool {
	if lexer.Lookahead() != '|' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '|' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '|' {
		return false
	}
	lexer.Advance(false)
	return true
}

func jsonnetScanStringStart(s *jsonnetState, lexer *gotreesitter.ExternalLexer) bool {
	ch := lexer.Lookahead()
	if ch == '"' || ch == '\'' {
		s.insideString = true
		s.endingChar = ch
		lexer.Advance(false)
		return true
	}
	if jsonnetScanBlockStart(lexer) {
		s.insideString = true
		s.endingChar = 0
		return true
	}
	return false
}

func jsonnetScanStringEnd(s *jsonnetState, lexer *gotreesitter.ExternalLexer) bool {
	if s.endingChar == 0 {
		return jsonnetScanBlockEnd(lexer)
	}
	if lexer.Lookahead() == s.endingChar {
		lexer.Advance(false)
		return true
	}
	return false
}

func jsonnetScanStringContent(s *jsonnetState, lexer *gotreesitter.ExternalLexer) bool {
	if s.endingChar == 0 {
		return jsonnetScanBlockContent(lexer)
	}

	for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 && lexer.Lookahead() != s.endingChar {
		if lexer.Lookahead() == '\\' {
			lexer.Advance(false)
			if lexer.Lookahead() == 'z' {
				lexer.Advance(false)
				for unicode.IsSpace(lexer.Lookahead()) {
					lexer.Advance(false)
				}
				continue
			}
		}
		if lexer.Lookahead() == 0 {
			return true
		}
		lexer.Advance(false)
	}
	return true
}

func jsonnetScanBlockContent(lexer *gotreesitter.ExternalLexer) bool {
	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == '|' {
			lexer.MarkEnd()
			if jsonnetScanBlockEnd(lexer) {
				return true
			}
		} else {
			lexer.Advance(false)
		}
	}
	return false
}

func jsonnetValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
