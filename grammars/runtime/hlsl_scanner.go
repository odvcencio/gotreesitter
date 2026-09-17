//go:build !grammar_subset || grammar_subset_hlsl

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the hlsl grammar.
const (
	hlslTokRawStringDelimiter = 0
	hlslTokRawStringContent   = 1
)

const (
	hlslSymRawStringDelimiter gotreesitter.Symbol = 244
	hlslSymRawStringContent   gotreesitter.Symbol = 245
)

// HlslExternalScanner handles C++ R"delim(...)delim" raw string literals for HLSL.
type HlslExternalScanner struct{}

func (HlslExternalScanner) Create() any         { return rawStringCreate() }
func (HlslExternalScanner) Destroy(payload any) {}
func (HlslExternalScanner) Serialize(payload any, buf []byte) int {
	return rawStringSerialize(payload, buf)
}
func (HlslExternalScanner) Deserialize(payload any, buf []byte) { rawStringDeserialize(payload, buf) }

func (HlslExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	return rawStringScan(payload, lexer, validSymbols,
		hlslTokRawStringDelimiter, hlslTokRawStringContent,
		hlslSymRawStringDelimiter, hlslSymRawStringContent)
}
