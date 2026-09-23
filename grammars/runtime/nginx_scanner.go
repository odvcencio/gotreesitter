//go:build !grammar_subset || grammar_subset_nginx

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the nginx grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner
// never hardcodes them -- see nginxDefaultSymTable below.
const (
	nginxTokNewline = 0
	nginxTokIndent  = 1
	nginxTokDedent  = 2
	nginxTokenCount = 3
)

// nginxDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped nginx.bin assigns to each external, in nginxTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var nginxDefaultSymTable = [nginxTokenCount]gotreesitter.Symbol{
	85, // _newline
	86, // _indent
	87, // _dedent
}

// nginxExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (nginxTok* order).
var nginxExternalScannerSpec = ExternalScannerSpec{
	Language:       "nginx",
	UpstreamRepo:   "https://github.com/opa-oz/tree-sitter-nginx",
	UpstreamCommit: "47ade644d754cce57974aac44d2c9450e823d4f4",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "2a48ccb3cfaeecce1c3cd2c993a430d284b43bd957166918f4a3f8d40d82a468"},
		{Path: "src/scanner.c", SHA256: "7d1568ef7838a93b17b057f2d12c7888cd8e406251e5e6416ce05c99cd594c9d"},
	},
	Externals: []string{
		"_newline",
		"_indent",
		"_dedent",
	},
}

func init() {
	RegisterExternalScannerSpec(nginxExternalScannerSpec)
}

// nginxScannerState holds the indent stack for the nginx external scanner.
type nginxScannerState struct {
	indents []uint16
}

// NginxExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-nginx.
//
// This is a Go port of the C external scanner from tree-sitter-nginx
// (https://github.com/opa-oz/tree-sitter-nginx). The scanner handles:
//   - _newline: newline characters
//   - _indent: increase in indentation level
//   - _dedent: decrease in indentation level
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type NginxExternalScanner struct {
	symbols         [nginxTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers nginx's external symbols.
func (NginxExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := NginxExternalScanner{symbols: nginxDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, nginxExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s NginxExternalScanner) symbolTable() *[nginxTokenCount]gotreesitter.Symbol {
	if s.symbols == ([nginxTokenCount]gotreesitter.Symbol{}) {
		return &nginxDefaultSymTable
	}
	return &s.symbols
}

func (NginxExternalScanner) Create() any {
	return &nginxScannerState{indents: []uint16{0}}
}

func (NginxExternalScanner) Destroy(payload any) {}

func (NginxExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*nginxScannerState)
	// Skip the initial 0 sentinel; serialize from index 1 onward.
	size := 0
	for i := 1; i < len(s.indents) && size+1 < len(buf); i++ {
		v := s.indents[i]
		buf[size] = byte(v)
		buf[size+1] = byte(v >> 8)
		size += 2
	}
	return size
}

func (NginxExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*nginxScannerState)
	s.indents = s.indents[:0]
	s.indents = append(s.indents, 0) // sentinel
	// Backward compatibility: older scanner states serialized one byte per indent.
	if len(buf)%2 != 0 {
		for _, b := range buf {
			s.indents = append(s.indents, uint16(b))
		}
		return
	}
	for i := 0; i+1 < len(buf); i += 2 {
		v := uint16(buf[i]) | uint16(buf[i+1])<<8
		s.indents = append(s.indents, v)
	}
}

func (sc NginxExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	syms := sc.symbolTable()

	s := payload.(*nginxScannerState)

	// If lookahead is newline and NEWLINE is valid, consume it.
	if lexer.Lookahead() == '\n' {
		if nginxValid(validSymbols, nginxTokNewline) {
			lexer.Advance(true) // skip
			lexer.SetResultSymbol(syms[nginxTokNewline])
			return true
		}
		return false
	}

	// At column 0, measure indentation and emit INDENT/DEDENT.
	if lexer.Lookahead() != 0 && lexer.Column() == 0 {
		var indentLen uint16

		// Indent tokens are zero width.
		lexer.MarkEnd()

		for {
			ch := lexer.Lookahead()
			if ch == ' ' {
				indentLen++
				lexer.Advance(true)
			} else if ch == '\t' {
				indentLen += 8
				lexer.Advance(true)
			} else {
				break
			}
		}

		top := s.indents[len(s.indents)-1]
		if indentLen > top && nginxValid(validSymbols, nginxTokIndent) {
			s.indents = append(s.indents, indentLen)
			lexer.SetResultSymbol(syms[nginxTokIndent])
			return true
		}
		if indentLen < top && nginxValid(validSymbols, nginxTokDedent) {
			s.indents = s.indents[:len(s.indents)-1]
			lexer.SetResultSymbol(syms[nginxTokDedent])
			return true
		}
	}

	return false
}

func nginxValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
