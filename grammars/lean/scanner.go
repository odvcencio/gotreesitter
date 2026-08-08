package lean

import "github.com/odvcencio/gotreesitter"

const (
	leanTokenBlockComment = iota
	leanTokenDocComment
	leanTokenModuleDocComment
	leanTokenCount
)

// ExternalScanner scans Lean's nested block comments and documentation nodes.
type ExternalScanner struct {
	symbols [leanTokenCount]gotreesitter.Symbol
}

// ExternalScannerForLanguage binds generated symbol identifiers to the scanner.
func (ExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	var scanner ExternalScanner
	if lang != nil {
		copy(scanner.symbols[:], lang.ExternalSymbols)
	}
	return scanner
}

func (ExternalScanner) Create() any                      { return nil }
func (ExternalScanner) Destroy(any)                      {}
func (ExternalScanner) Serialize(any, []byte) int        { return 0 }
func (ExternalScanner) Deserialize(any, []byte)          {}
func (ExternalScanner) SupportsIncrementalReuse() bool   { return true }
func (ExternalScanner) ExternalScannerIsStateless() bool { return true }

// Scan recognizes /- comments, including nested comments and documentation forms.
func (scanner ExternalScanner) Scan(_ any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' || lexer.Lookahead() == '\r' {
		lexer.Advance(true)
	}
	if lexer.Lookahead() != '/' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead() != '-' {
		return false
	}
	lexer.Advance(false)

	token := leanTokenBlockComment
	switch lexer.Lookahead() {
	case '-':
		token = leanTokenDocComment
		lexer.Advance(false)
	case '!':
		token = leanTokenModuleDocComment
		lexer.Advance(false)
	}
	if token >= len(validSymbols) || !validSymbols[token] {
		return false
	}

	depth := 1
	for depth > 0 {
		switch lexer.Lookahead() {
		case 0:
			lexer.MarkEnd()
			lexer.SetResultSymbol(scanner.symbols[token])
			return true
		case '/':
			lexer.Advance(false)
			if lexer.Lookahead() == '-' {
				lexer.Advance(false)
				depth++
			}
		case '-':
			lexer.Advance(false)
			if lexer.Lookahead() == '/' {
				lexer.Advance(false)
				depth--
			}
		default:
			lexer.Advance(false)
		}
	}

	lexer.MarkEnd()
	lexer.SetResultSymbol(scanner.symbols[token])
	return true
}
