//go:build !grammar_subset || grammar_subset_cooklang

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the cooklang grammar.
const (
	cooklangTokNewline = 0
)

const (
	cooklangSymNewline gotreesitter.Symbol = 25
)

// CooklangExternalScanner handles newline detection for Cooklang recipe files.
type CooklangExternalScanner struct{}

func (CooklangExternalScanner) Create() any                           { return nil }
func (CooklangExternalScanner) Destroy(payload any)                   {}
func (CooklangExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (CooklangExternalScanner) Deserialize(payload any, buf []byte)   {}

func (CooklangExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if !cooklangValid(validSymbols, cooklangTokNewline) {
		return false
	}
	ch := lexer.Lookahead()
	if ch == '\n' {
		lexer.SetResultSymbol(cooklangSymNewline)
		return true
	}
	return false
}

func cooklangValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
