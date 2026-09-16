//go:build !grammar_subset || grammar_subset_kdl

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the kdl grammar.
const (
	kdlTokEof              = 0
	kdlTokMultiLineComment = 1
	kdlTokRawString        = 2
)

const (
	kdlSymEof              gotreesitter.Symbol = 81
	kdlSymMultiLineComment gotreesitter.Symbol = 82
	kdlSymRawString        gotreesitter.Symbol = 83
)

// KdlExternalScanner handles EOF detection, nestable /* */ comments,
// and r#"..."# raw strings for KDL.
type KdlExternalScanner struct{}

func (KdlExternalScanner) Create() any                           { return nil }
func (KdlExternalScanner) Destroy(payload any)                   {}
func (KdlExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (KdlExternalScanner) Deserialize(payload any, buf []byte)   {}

// Nesting depth and raw-string delimiters are local to one Scan call. The
// scanner retains no state between tokens or after a failed scan.
func (KdlExternalScanner) SupportsIncrementalReuse() bool    { return true }
func (KdlExternalScanner) ExternalScannerIsStateless() bool  { return true }
func (KdlExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (KdlExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	// EOF detection
	if kdlValid(validSymbols, kdlTokEof) && lexer.Lookahead() == 0 {
		lexer.Advance(false)
		lexer.SetResultSymbol(kdlSymEof)
		return true
	}

	// Raw string: r#"..."#
	if kdlValid(validSymbols, kdlTokRawString) && lexer.Lookahead() == 'r' {
		lexer.Advance(false)
		numHashes := uint32(0)
		for lexer.Lookahead() == '#' {
			numHashes++
			lexer.Advance(false)
		}
		if lexer.Lookahead() != '"' {
			return false
		}
		lexer.Advance(false)

		for {
			if lexer.Lookahead() == 0 {
				return false
			}
			ch := lexer.Lookahead()
			lexer.Advance(false)
			if ch != '"' {
				continue
			}
			// Try to match closing hashes
			closingHashes := uint32(0)
			for closingHashes < numHashes && lexer.Lookahead() == '#' {
				closingHashes++
				lexer.Advance(false)
			}
			if closingHashes == numHashes {
				lexer.MarkEnd()
				lexer.SetResultSymbol(kdlSymRawString)
				return true
			}
		}
	}

	// Nestable multi-line comment: /* ... */
	if lexer.Lookahead() == '/' {
		lexer.Advance(false)
		if lexer.Lookahead() != '*' {
			return false
		}
		lexer.Advance(false)

		afterStar := false
		depth := uint32(1)
		for depth > 0 {
			ch := lexer.Lookahead()
			switch {
			case ch == 0:
				return false
			case ch == '*':
				lexer.Advance(false)
				afterStar = true
			case ch == '/':
				if afterStar {
					lexer.Advance(false)
					afterStar = false
					depth--
				} else {
					lexer.Advance(false)
					afterStar = false
					if lexer.Lookahead() == '*' {
						depth++
						lexer.Advance(false)
					}
				}
			default:
				lexer.Advance(false)
				afterStar = false
			}
		}
		lexer.MarkEnd()
		lexer.SetResultSymbol(kdlSymMultiLineComment)
		return true
	}

	return false
}

func kdlValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
