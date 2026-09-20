//go:build !grammar_subset || grammar_subset_beancount

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the beancount grammar.
const (
	beancountTokStars      = 0
	beancountTokSectionEnd = 1
	beancountTokEof        = 2
	beancountTokenCount    = 3
)

// beancountDefaultSymTable seeds a scanner value that is used without going
// through ExternalScannerForLanguage first. Production attachment always
// calls ExternalScannerForLanguage, which rebinds these slots positionally
// against the loaded Language's ExternalSymbols (see
// beancountExternalScannerSpec and bindExternalScannerSpec). These three
// values match the beancount.bin blob shipped on 2026-09-20; keep them in
// step with ExternalSymbols[0:3] if that ever changes.
var beancountDefaultSymTable = [beancountTokenCount]gotreesitter.Symbol{
	60, // _stars
	61, // _sectionend
	62, // _eof
}

// beancountExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its token
// list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (beancountTok* order).
var beancountExternalScannerSpec = ExternalScannerSpec{
	Language:       "beancount",
	UpstreamRepo:   "https://github.com/polarmutex/tree-sitter-beancount",
	UpstreamCommit: "d7a03a7506fbbbc4b16a9a2054ff7c2b337744b8",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "914b13c1489edd09725a467636a3f09eee58eb73accd6d026490f73503a3251e"},
		{Path: "src/scanner.c", SHA256: "14c07a7bcf353f578c934f763ab5f9cc63f8febb5b84952a8503c97cacbe96dd"},
	},
	Externals: []string{
		"_stars",
		"_sectionend",
		"_eof",
	},
}

func init() {
	RegisterExternalScannerSpec(beancountExternalScannerSpec)
}

const beancountTabWidth = 8

// beancountState tracks section nesting for org-mode style headers in Beancount.
type beancountState struct {
	orgSectionStack []int16
	eofReturned     bool
}

// BeancountExternalScanner handles org-mode style section headers and EOF for
// Beancount.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see
// ExternalScannerForLanguage). The scanner never hardcodes an absolute
// Symbol value: a blob regen can renumber the grammar's absolute symbol IDs
// without touching the externals list order, and a scanner that still called
// SetResultSymbol with a stale hardcoded ID would silently emit the wrong
// (but still structurally valid) node type instead of failing loudly.
type BeancountExternalScanner struct {
	symbols         [beancountTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's three token slots to the
// loaded Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers beancount's external symbols.
func (BeancountExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := BeancountExternalScanner{symbols: beancountDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, beancountExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (BeancountExternalScanner) Create() any {
	return &beancountState{orgSectionStack: []int16{0}}
}
func (BeancountExternalScanner) Destroy(payload any) {}
func (BeancountExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*beancountState)
	i := 0
	if s.eofReturned {
		buf[i] = 1
	} else {
		buf[i] = 0
	}
	i++
	// We skip indent_length_stack (unused in section parsing)
	buf[i] = 0
	i++
	// Write org section stack (skip base element 0)
	count := len(s.orgSectionStack) - 1
	if count > 255 {
		count = 255
	}
	buf[i] = byte(count)
	i++
	for j := 1; j < len(s.orgSectionStack) && i < len(buf); j++ {
		buf[i] = byte(s.orgSectionStack[j])
		i++
	}
	return i
}
func (BeancountExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*beancountState)
	s.orgSectionStack = s.orgSectionStack[:0]
	s.orgSectionStack = append(s.orgSectionStack, 0)
	s.eofReturned = false
	if len(buf) == 0 {
		return
	}
	i := 0
	s.eofReturned = buf[i] != 0
	i++
	if i >= len(buf) {
		return
	}
	// Skip indent count
	indentCount := int(buf[i])
	i++
	i += indentCount // skip indent data
	if i >= len(buf) {
		return
	}
	sectionCount := int(buf[i])
	i++
	for j := 0; j < sectionCount && i < len(buf); j++ {
		s.orgSectionStack = append(s.orgSectionStack, int16(buf[i]))
		i++
	}
}

func (s BeancountExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(s.externalToToken) > 0 {
		var semanticValid [beancountTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < beancountTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	symbols := s.symbolTable()
	st := payload.(*beancountState)

	// Don't produce tokens during error recovery
	if beancountValid(validSymbols, beancountTokStars) &&
		beancountValid(validSymbols, beancountTokSectionEnd) &&
		beancountValid(validSymbols, beancountTokEof) {
		return false
	}

	lexer.MarkEnd()

	// Count leading whitespace
	indentLength := int16(0)
	for {
		ch := lexer.Lookahead()
		if ch == ' ' {
			indentLength++
			lexer.Advance(true)
		} else if ch == '\t' {
			indentLength += beancountTabWidth
			lexer.Advance(true)
		} else {
			break
		}
	}

	// Handle EOF
	if lexer.Lookahead() == 0 {
		if beancountValid(validSymbols, beancountTokSectionEnd) {
			lexer.SetResultSymbol(symbols[beancountTokSectionEnd])
			return true
		}
		if beancountValid(validSymbols, beancountTokEof) && !st.eofReturned {
			st.eofReturned = true
			lexer.SetResultSymbol(symbols[beancountTokEof])
			return true
		}
		return false
	}

	// Check for section headers (at column 0)
	if indentLength == 0 && isBeancountHeadlineMarker(lexer.Lookahead()) {
		lexer.MarkEnd()
		stars := int16(1)
		lexer.Advance(true)
		for isBeancountHeadlineMarker(lexer.Lookahead()) {
			stars++
			lexer.Advance(true)
		}
		if !unicode.IsSpace(lexer.Lookahead()) {
			return false
		}

		if beancountValid(validSymbols, beancountTokSectionEnd) && stars > 0 &&
			len(st.orgSectionStack) > 0 &&
			stars <= st.orgSectionStack[len(st.orgSectionStack)-1] {
			st.orgSectionStack = st.orgSectionStack[:len(st.orgSectionStack)-1]
			lexer.SetResultSymbol(symbols[beancountTokSectionEnd])
			return true
		}
		if beancountValid(validSymbols, beancountTokStars) {
			st.orgSectionStack = append(st.orgSectionStack, stars)
			lexer.SetResultSymbol(symbols[beancountTokStars])
			return true
		}
	}

	return false
}

func (s BeancountExternalScanner) symbolTable() *[beancountTokenCount]gotreesitter.Symbol {
	if s.symbols == ([beancountTokenCount]gotreesitter.Symbol{}) {
		return &beancountDefaultSymTable
	}
	return &s.symbols
}

func isBeancountHeadlineMarker(ch rune) bool {
	return ch == '*' || ch == '#'
}

func beancountValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
