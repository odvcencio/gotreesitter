//go:build !grammar_subset || grammar_subset_arduino

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the arduino grammar.
const (
	arduinoTokRawStringDelimiter = 0
	arduinoTokRawStringContent   = 1
)

const (
	arduinoSymRawStringDelimiter gotreesitter.Symbol = 221
	arduinoSymRawStringContent   gotreesitter.Symbol = 222
)

// ArduinoExternalScanner handles C++ R"delim(...)delim" raw string literals for Arduino.
type ArduinoExternalScanner struct{}

func (ArduinoExternalScanner) Create() any         { return rawStringCreate() }
func (ArduinoExternalScanner) Destroy(payload any) {}
func (ArduinoExternalScanner) Serialize(payload any, buf []byte) int {
	return rawStringSerialize(payload, buf)
}
func (ArduinoExternalScanner) Deserialize(payload any, buf []byte) {
	rawStringDeserialize(payload, buf)
}

func (ArduinoExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	return rawStringScan(payload, lexer, validSymbols,
		arduinoTokRawStringDelimiter, arduinoTokRawStringContent,
		arduinoSymRawStringDelimiter, arduinoSymRawStringContent)
}
