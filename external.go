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
	if lexer == nil {
		return lang.ExternalScanner.Scan(payload, lexer, validSymbols)
	}
	// Retain one observer per scratch lexer, outside its value-copy state.
	// A scanner can restore its cursor after a speculative read.
	if lexer.readFrontier == nil {
		lexer.readFrontier = new(externalReadFrontier)
	}
	observer := lexer.readFrontier
	*observer = externalReadFrontier{lookahead: lexer.lookaheadEndByte, examined: tokenInvariantExaminedEnd(lexer.source, lexer.lookaheadEndByte)}
	accepted := lang.ExternalScanner.Scan(payload, lexer, validSymbols)
	lexer.lookaheadEndByte = maxUint32(lexer.lookaheadEndByte, observer.lookahead)
	return accepted
}
