package gotreesitter

// ExternalScannerState holds serialized state for an external scanner
// between incremental parse runs.
type ExternalScannerState struct {
	Data []byte
}

// RunExternalScanner invokes the language's external scanner if present.
// Returns true if the scanner produced a token, false otherwise.
func RunExternalScanner(lang *Language, payload any, lexer *ExternalLexer, validSymbols []bool) bool {
	if lang.ExternalScanner == nil {
		return false
	}
	return lang.ExternalScanner.Scan(payload, lexer, validSymbols)
}
