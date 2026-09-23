//go:build !grammar_subset || grammar_subset_gitcommit

package grammarruntime

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the gitcommit grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see gitcommitDefaultSymTable below.
const (
	gitcommitTokConventionalPrefix = 0
	gitcommitTokTrailerValue       = 1 // _trailer_value (never emitted by this scanner)
	gitcommitTokenCount            = 2
)

// gitcommitDefaultSymTable records the concrete gotreesitter.Symbol IDs
// the currently shipped gitcommit.bin assigns to each external, in
// gitcommitTok* order. It exists only as a pre-bind fallback (and as an
// independent value to compare a real bind against in tests);
// ExternalScannerForLanguage below overwrites it with values read from
// the actual loaded Language at bind time, which is what the scanner
// must do to survive a future blob regen that renumbers absolute symbol
// IDs without touching the externals list order.
var gitcommitDefaultSymTable = [gitcommitTokenCount]gotreesitter.Symbol{
	456, // _conventional_type, displays as "type"
	457, // _trailer_value (never emitted by this scanner)
}

// gitcommitExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (gitcommitTok* order).
var gitcommitExternalScannerSpec = ExternalScannerSpec{
	Language:       "gitcommit",
	UpstreamRepo:   "https://github.com/gbprod/tree-sitter-gitcommit",
	UpstreamCommit: "a716678c0f00645fed1e6f1d0eb221481dbd6f6d",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "a37da95637c8716d2725c458e5992595454e04f1767abe0e022202098eb6876a"},
		{Path: "src/scanner.c", SHA256: "5a8fb91fcdf9f5497e8d248a27ee4232ebde2df4f87f5df280c492e927ff743b"},
	},
	Externals: []string{
		"_conventional_type",
		"_trailer_value",
	},
}

func init() {
	RegisterExternalScannerSpec(gitcommitExternalScannerSpec)
}

// GitcommitExternalScanner handles conventional commit prefix detection.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type GitcommitExternalScanner struct {
	symbols         [gitcommitTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers gitcommit's external symbols.
func (GitcommitExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := GitcommitExternalScanner{symbols: gitcommitDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, gitcommitExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s GitcommitExternalScanner) symbolTable() *[gitcommitTokenCount]gotreesitter.Symbol {
	if s.symbols == ([gitcommitTokenCount]gotreesitter.Symbol{}) {
		return &gitcommitDefaultSymTable
	}
	return &s.symbols
}

func (GitcommitExternalScanner) Create() any                           { return nil }
func (GitcommitExternalScanner) Destroy(payload any)                   {}
func (GitcommitExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (GitcommitExternalScanner) Deserialize(payload any, buf []byte)   {}

func (sc GitcommitExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(sc.externalToToken) > 0 {
		var semanticValid [gitcommitTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(sc.externalToToken) {
				continue
			}
			tokenIdx := sc.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < gitcommitTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := sc.symbolTable()

	if !gitcommitValid(validSymbols, gitcommitTokConventionalPrefix) {
		return false
	}

	lexer.SetResultSymbol(syms[gitcommitTokConventionalPrefix])

	ch := lexer.Lookahead()
	if unicode.IsControl(ch) || unicode.IsSpace(ch) || ch == ':' || ch == '!' || ch == 0 {
		return false
	}
	lexer.Advance(false)

	// Consume word characters (not control, space, colon, bang, parens)
	for {
		ch = lexer.Lookahead()
		if unicode.IsControl(ch) || unicode.IsSpace(ch) ||
			ch == ':' || ch == '!' || ch == '(' || ch == ')' || ch == 0 {
			break
		}
		lexer.Advance(false)
	}
	lexer.MarkEnd()

	// Optional scope in parentheses
	if lexer.Lookahead() == '(' {
		lexer.Advance(false)
		if lexer.Lookahead() == ')' {
			return false
		}
		for {
			ch = lexer.Lookahead()
			if unicode.IsControl(ch) || ch == '(' || ch == ')' || ch == 0 {
				break
			}
			lexer.Advance(false)
		}
		if lexer.Lookahead() != ')' {
			return false
		}
		lexer.Advance(false)
	}

	// Optional breaking change indicator
	if lexer.Lookahead() == '!' {
		lexer.Advance(false)
	}

	return lexer.Lookahead() == ':' || lexer.Lookahead() == 0xff1a
}

func gitcommitValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
