//go:build !grammar_subset || grammar_subset_cpp

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the cpp grammar.
const (
	cppTokRawStringDelimiter = 0
	cppTokRawStringContent   = 1
)

const (
	cppSymRawStringDelimiter gotreesitter.Symbol = 223
	cppSymRawStringContent   gotreesitter.Symbol = 224
)

// CppExternalScanner handles C++ R"delim(...)delim" raw string literals.
type CppExternalScanner struct{}

func (CppExternalScanner) Create() any         { return rawStringCreate() }
func (CppExternalScanner) Destroy(payload any) {}
func (CppExternalScanner) Serialize(payload any, buf []byte) int {
	return rawStringSerialize(payload, buf)
}
func (CppExternalScanner) Deserialize(payload any, buf []byte) { rawStringDeserialize(payload, buf) }

func (CppExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	return rawStringScan(payload, lexer, validSymbols,
		cppTokRawStringDelimiter, cppTokRawStringContent,
		cppSymRawStringDelimiter, cppSymRawStringContent)
}
