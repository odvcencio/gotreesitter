//go:build !grammar_subset || grammar_subset_comment

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the comment grammar. This is the external
// index (the position of the token in the grammar's `externals: [...]`
// list), which is exactly what tree-sitter's `valid_symbols` array and
// C's result_symbol enum are indexed by. The external index is stable
// across a blob regen as long as the externals list itself does not
// reorder; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this
// scanner never hardcodes them -- see commentDefaultSymTable below.
//
// commentTokInvalidToken never reaches SetResultSymbol (it only gates the
// correction-mode decline check below), but it still binds positionally
// like every other external.
const (
	commentTokName         = 0 // "name" — tag keyword like TODO, FIXME, NOTE
	commentTokInvalidToken = 1 // "invalid_token" — error recovery
	commentTokenCount      = 2
)

// commentDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped comment.bin assigns to each external, in commentTok*
// order. It exists only as a pre-bind fallback (and as an independent
// value to compare a real bind against in tests); ExternalScannerForLanguage
// below overwrites it with values read from the actual loaded Language at
// bind time, which is what the scanner must do to survive a future blob
// regen that renumbers absolute symbol IDs without touching the externals
// list order.
var commentDefaultSymTable = [commentTokenCount]gotreesitter.Symbol{
	25, // name
	26, // invalid_token
}

// commentExternalScannerSpec records the source contract for this
// hand-written port, so updater tooling can tell a grammar-only upstream
// change apart from one that also touches the external scanner or its
// token list. Its Externals list is also the binding source for
// ExternalScannerForLanguage: index i here is scanner token index i
// (commentTok* order).
var commentExternalScannerSpec = ExternalScannerSpec{
	Language:       "comment",
	UpstreamRepo:   "https://github.com/stsewd/tree-sitter-comment",
	UpstreamCommit: "66272d2b6c73fb61157541b69dd0a7ce7b42a5ad",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "fd16ed3e8b5e165f9fad323ca80fa540c71b2924f6cd8ab7afaa688d053444d8"},
		{Path: "src/scanner.c", SHA256: "3e92aef5b0ee6ba1bd85e92302aa86449607562c3b1af77cc9b550f9b36613cf"},
	},
	Externals: []string{
		"name",
		"invalid_token",
	},
}

func init() {
	RegisterExternalScannerSpec(commentExternalScannerSpec)
}

// CommentExternalScanner implements gotreesitter.ExternalScanner for tree-sitter-comment.
//
// The comment grammar (tree-sitter-comment) parses structured comment text
// such as "TODO: fix this" or "FIXME(user): description". The external
// scanner is responsible for producing the "name" token which represents
// a tag keyword (TODO, FIXME, NOTE, HACK, etc.).
//
// The scanner must be careful not to match arbitrary text as a "name",
// since the DFA handles regular text via _text_token1. A name is only
// returned when the scanned word is immediately followed by ':' or '(',
// indicating it forms part of a tag construct.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type CommentExternalScanner struct {
	symbols         [commentTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slots to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers comment's external symbols.
func (CommentExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := CommentExternalScanner{symbols: commentDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, commentExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s CommentExternalScanner) symbolTable() *[commentTokenCount]gotreesitter.Symbol {
	if s.symbols == ([commentTokenCount]gotreesitter.Symbol{}) {
		return &commentDefaultSymTable
	}
	return &s.symbols
}

func (CommentExternalScanner) Create() any                           { return nil }
func (CommentExternalScanner) Destroy(payload any)                   {}
func (CommentExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (CommentExternalScanner) Deserialize(payload any, buf []byte)   {}
func (CommentExternalScanner) SupportsIncrementalReuse() bool        { return true }
func (CommentExternalScanner) ExternalScannerIsStateless() bool      { return true }

func (s CommentExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(s.externalToToken) > 0 {
		var semanticValid [commentTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < commentTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	syms := s.symbolTable()

	// Upstream treats invalid_token as correction mode. The Go-generated
	// external valid set can conservatively include invalid_token beside name,
	// so only decline when name is not also explicitly valid.
	if len(validSymbols) > commentTokInvalidToken && validSymbols[commentTokInvalidToken] &&
		!(len(validSymbols) > commentTokName && validSymbols[commentTokName]) {
		return false
	}

	// name: scan a tag keyword like TODO, FIXME, NOTE, etc.
	if len(validSymbols) > commentTokName && validSymbols[commentTokName] {
		return scanCommentName(lexer, syms[commentTokName])
	}

	return false
}

// scanCommentName scans a name token (tag keyword), mirroring
// tree-sitter-comment's scanner:
//   - the name starts with an uppercase ASCII letter,
//   - continues with uppercase ASCII, digits, '-' or '_',
//   - cannot end with '-' or '_',
//   - may be followed by optional non-newline space plus a non-empty user
//     component in parentheses,
//   - and must end with ':' followed by whitespace.
func scanCommentName(lexer *gotreesitter.ExternalLexer, nameSym gotreesitter.Symbol) bool {
	if !isCommentUpper(lexer.Lookahead()) {
		return false
	}

	previous := lexer.Lookahead()
	lexer.Advance(false)
	for isCommentUpper(lexer.Lookahead()) ||
		isCommentDigit(lexer.Lookahead()) ||
		isCommentInternal(lexer.Lookahead()) {
		previous = lexer.Lookahead()
		lexer.Advance(false)
	}
	lexer.MarkEnd()

	if isCommentInternal(previous) {
		return false
	}

	if (isCommentSpace(lexer.Lookahead()) && !isCommentNewline(lexer.Lookahead())) ||
		lexer.Lookahead() == '(' {
		for isCommentSpace(lexer.Lookahead()) && !isCommentNewline(lexer.Lookahead()) {
			lexer.Advance(false)
		}
		if lexer.Lookahead() != '(' {
			return false
		}
		lexer.Advance(false)

		userLength := 0
		for lexer.Lookahead() != ')' {
			if isCommentNewline(lexer.Lookahead()) {
				return false
			}
			lexer.Advance(false)
			userLength++
		}
		if userLength <= 0 {
			return false
		}
		lexer.Advance(false)
	}

	if lexer.Lookahead() != ':' {
		return false
	}
	lexer.Advance(false)
	if !isCommentSpace(lexer.Lookahead()) {
		return false
	}

	lexer.SetResultSymbol(nameSym)
	return true
}

func isCommentUpper(ch rune) bool {
	if ch >= 'A' && ch <= 'Z' {
		return true
	}
	return false
}

func isCommentDigit(ch rune) bool {
	if ch >= '0' && ch <= '9' {
		return true
	}
	return false
}

func isCommentInternal(ch rune) bool {
	return ch == '-' || ch == '_'
}

func isCommentNewline(ch rune) bool {
	return ch == 0 || ch == '\n' || ch == '\r'
}

func isCommentSpace(ch rune) bool {
	switch ch {
	case ' ', '\f', '\t', '\v':
		return true
	default:
		return isCommentNewline(ch)
	}
}
