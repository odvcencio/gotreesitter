//go:build !grammar_subset || grammar_subset_org

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the org grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see orgDefaultSymTable below.
const (
	orgTokListStart   = 0
	orgTokListEnd     = 1
	orgTokListItemEnd = 2
	orgTokBullet      = 3
	orgTokHlStars     = 4
	orgTokSectionEnd  = 5
	orgTokEndOfFile   = 6
	orgTokenCount     = 7
)

// orgDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped org.bin assigns to each external, in orgTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var orgDefaultSymTable = [orgTokenCount]gotreesitter.Symbol{
	117, // _liststart
	118, // _listend
	119, // _listitemend
	120, // bullet
	121, // _stars
	122, // _sectionend
	123, // _eof
}

// orgExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (orgTok* order).
var orgExternalScannerSpec = ExternalScannerSpec{
	Language:       "org",
	UpstreamRepo:   "https://github.com/emiasims/tree-sitter-org",
	UpstreamCommit: "64cfbc213f5a83da17632c95382a5a0a2f3357c1",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "38a39e7e00ddd8dbba17ab01119b9a0bf11fb702347a4ed161da567a35de94e5"},
		{Path: "src/scanner.c", SHA256: "7d510dd3076f0dea4862dece13d99e3940a99439bcd908fc521190411f136397"},
	},
	Externals: []string{
		"_liststart",
		"_listend",
		"_listitemend",
		"bullet",
		"_stars",
		"_sectionend",
		"_eof",
	},
}

func init() {
	RegisterExternalScannerSpec(orgExternalScannerSpec)
}

// org bullet types
const (
	orgBulletNone       = 0
	orgBulletDash       = 1
	orgBulletPlus       = 2
	orgBulletStar       = 3
	orgBulletLowerDot   = 4
	orgBulletUpperDot   = 5
	orgBulletLowerParen = 6
	orgBulletUpperParen = 7
	orgBulletNumDot     = 8
	orgBulletNumParen   = 9
)

// orgState tracks indent/bullet/section stacks for org mode.
type orgState struct {
	indents  []int16
	bullets  []int16
	sections []int16
}

// OrgExternalScanner handles list/section/headline detection for org mode.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type OrgExternalScanner struct {
	symbols         [orgTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers org's external symbols.
func (OrgExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := OrgExternalScanner{symbols: orgDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, orgExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s OrgExternalScanner) symbolTable() *[orgTokenCount]gotreesitter.Symbol {
	if s.symbols == ([orgTokenCount]gotreesitter.Symbol{}) {
		return &orgDefaultSymTable
	}
	return &s.symbols
}

func (OrgExternalScanner) Create() any {
	return &orgState{
		indents:  []int16{-1},
		bullets:  []int16{orgBulletNone},
		sections: []int16{0},
	}
}

func (OrgExternalScanner) Destroy(payload any) {}

func (OrgExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*orgState)
	n := 0
	indentCount := len(s.indents) - 1
	if indentCount > 255 {
		indentCount = 255
	}
	if n >= len(buf) {
		return 0
	}
	buf[n] = byte(indentCount)
	n++
	for i := 1; i <= indentCount && n < len(buf); i++ {
		buf[n] = byte(s.indents[i])
		n++
	}
	for i := 1; i <= indentCount && n < len(buf); i++ {
		buf[n] = byte(s.bullets[i])
		n++
	}
	for i := 1; i < len(s.sections) && n < len(buf); i++ {
		buf[n] = byte(s.sections[i])
		n++
	}
	return n
}

func (OrgExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*orgState)
	s.sections = s.sections[:0]
	s.sections = append(s.sections, 0)
	s.indents = s.indents[:0]
	s.indents = append(s.indents, -1)
	s.bullets = s.bullets[:0]
	s.bullets = append(s.bullets, orgBulletNone)

	if len(buf) == 0 {
		return
	}

	i := 0
	indentCount := int(buf[i])
	i++
	for ; i <= indentCount && i < len(buf); i++ {
		s.indents = append(s.indents, int16(buf[i]))
	}
	for ; i <= 2*indentCount && i < len(buf); i++ {
		s.bullets = append(s.bullets, int16(buf[i]))
	}
	for ; i < len(buf); i++ {
		s.sections = append(s.sections, int16(buf[i]))
	}
}

func (sc OrgExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [orgTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < orgTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	s := payload.(*orgState)

	if orgInErrorRecovery(validSymbols) {
		return false
	}

	indentLength := int16(0)
	lexer.MarkEnd()

	// Scan initial whitespace
	for {
		ch := lexer.Lookahead()
		if ch == ' ' {
			indentLength++
		} else if ch == '\t' {
			indentLength += 8
		} else if ch == 0 {
			if orgValid(validSymbols, orgTokListEnd) {
				lexer.SetResultSymbol(syms[orgTokListEnd])
			} else if orgValid(validSymbols, orgTokSectionEnd) {
				lexer.SetResultSymbol(syms[orgTokSectionEnd])
			} else if orgValid(validSymbols, orgTokEndOfFile) {
				lexer.SetResultSymbol(syms[orgTokEndOfFile])
			} else {
				return false
			}
			return true
		} else {
			break
		}
		lexer.Advance(true)
	}

	// List end/item end
	newlines := int16(0)
	if orgValid(validSymbols, orgTokListEnd) || orgValid(validSymbols, orgTokListItemEnd) {
		for {
			ch := lexer.Lookahead()
			if ch == ' ' {
				indentLength++
			} else if ch == '\t' {
				indentLength += 8
			} else if ch == 0 {
				return orgDedent(s, lexer, syms)
			} else if ch == '\n' {
				newlines++
				if newlines > 1 {
					return orgDedent(s, lexer, syms)
				}
				indentLength = 0
			} else {
				break
			}
			lexer.Advance(true)
		}

		back := s.indents[len(s.indents)-1]
		if indentLength < back {
			return orgDedent(s, lexer, syms)
		} else if indentLength == back {
			bullet := orgGetBullet(lexer)
			if bullet == s.bullets[len(s.bullets)-1] {
				lexer.SetResultSymbol(syms[orgTokListItemEnd])
				return true
			}
			return orgDedent(s, lexer, syms)
		}
	}

	// Headlines (stars at column 0)
	if indentLength == 0 && lexer.Lookahead() == '*' {
		lexer.MarkEnd()
		stars := int16(1)
		lexer.Advance(true)
		for lexer.Lookahead() == '*' {
			stars++
			lexer.Advance(true)
		}

		if orgValid(validSymbols, orgTokSectionEnd) && unicode.IsSpace(lexer.Lookahead()) &&
			stars > 0 && stars <= s.sections[len(s.sections)-1] {
			s.sections = s.sections[:len(s.sections)-1]
			lexer.SetResultSymbol(syms[orgTokSectionEnd])
			return true
		} else if orgValid(validSymbols, orgTokHlStars) && unicode.IsSpace(lexer.Lookahead()) {
			s.sections = append(s.sections, stars)
			lexer.SetResultSymbol(syms[orgTokHlStars])
			return true
		}
		return false
	}

	// List start and bullets
	if (orgValid(validSymbols, orgTokListStart) || orgValid(validSymbols, orgTokBullet)) && newlines == 0 {
		bullet := orgGetBullet(lexer)
		back := s.indents[len(s.indents)-1]
		bulletBack := s.bullets[len(s.bullets)-1]

		if orgValid(validSymbols, orgTokBullet) && bullet == bulletBack && indentLength == back {
			lexer.MarkEnd()
			lexer.SetResultSymbol(syms[orgTokBullet])
			return true
		} else if orgValid(validSymbols, orgTokListStart) && bullet != orgBulletNone && indentLength > back {
			s.indents = append(s.indents, indentLength)
			s.bullets = append(s.bullets, int16(bullet))
			lexer.SetResultSymbol(syms[orgTokListStart])
			return true
		}
	}

	return false
}

func orgDedent(s *orgState, lexer *gotreesitter.ExternalLexer, syms *[orgTokenCount]gotreesitter.Symbol) bool {
	s.indents = s.indents[:len(s.indents)-1]
	s.bullets = s.bullets[:len(s.bullets)-1]
	lexer.SetResultSymbol(syms[orgTokListEnd])
	return true
}

func orgGetBullet(lexer *gotreesitter.ExternalLexer) int16 {
	ch := lexer.Lookahead()
	if ch == '-' {
		lexer.Advance(false)
		if unicode.IsSpace(lexer.Lookahead()) {
			return orgBulletDash
		}
	} else if ch == '+' {
		lexer.Advance(false)
		if unicode.IsSpace(lexer.Lookahead()) {
			return orgBulletPlus
		}
	} else if ch == '*' {
		lexer.Advance(false)
		if unicode.IsSpace(lexer.Lookahead()) {
			return orgBulletStar
		}
	} else if ch >= 'a' && ch <= 'z' {
		lexer.Advance(false)
		if lexer.Lookahead() == '.' {
			lexer.Advance(false)
			if unicode.IsSpace(lexer.Lookahead()) {
				return orgBulletLowerDot
			}
		} else if lexer.Lookahead() == ')' {
			lexer.Advance(false)
			if unicode.IsSpace(lexer.Lookahead()) {
				return orgBulletLowerParen
			}
		}
	} else if ch >= 'A' && ch <= 'Z' {
		lexer.Advance(false)
		if lexer.Lookahead() == '.' {
			lexer.Advance(false)
			if unicode.IsSpace(lexer.Lookahead()) {
				return orgBulletUpperDot
			}
		} else if lexer.Lookahead() == ')' {
			lexer.Advance(false)
			if unicode.IsSpace(lexer.Lookahead()) {
				return orgBulletUpperParen
			}
		}
	} else if ch >= '0' && ch <= '9' {
		for lexer.Lookahead() >= '0' && lexer.Lookahead() <= '9' {
			lexer.Advance(false)
		}
		if lexer.Lookahead() == '.' {
			lexer.Advance(false)
			if unicode.IsSpace(lexer.Lookahead()) {
				return orgBulletNumDot
			}
		} else if lexer.Lookahead() == ')' {
			lexer.Advance(false)
			if unicode.IsSpace(lexer.Lookahead()) {
				return orgBulletNumParen
			}
		}
	}
	return orgBulletNone
}

func orgInErrorRecovery(vs []bool) bool {
	return orgValid(vs, orgTokListStart) && orgValid(vs, orgTokListEnd) &&
		orgValid(vs, orgTokListItemEnd) && orgValid(vs, orgTokBullet) &&
		orgValid(vs, orgTokHlStars) && orgValid(vs, orgTokSectionEnd) &&
		orgValid(vs, orgTokEndOfFile)
}

func orgValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
