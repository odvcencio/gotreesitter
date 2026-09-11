//go:build !grammar_subset

package grammars

func init() {
	registerTokenSourceFactory("authzed", NewAuthzedTokenSourceOrEOF)
	registerTokenSourceFactory("c", NewCTokenSourceOrEOF)
	registerTokenSourceFactory("cpp", NewCTokenSourceOrEOF)
	// Go uses the locked C grammar's DFA tables without an external scanner.
	// GoTokenSource and GoExternalScanner remain available for explicit legacy use.
	registerTokenSourceFactory("java", NewJavaTokenSourceOrEOF)
	registerTokenSourceFactory("json", NewJSONTokenSourceOrEOF)
	// Lua now parses via the blob's DFA lexer plus LuaExternalScanner (a
	// line-faithful port of upstream scanner.c), which matches the C oracle
	// where the hand-tuned LuaTokenSource diverged (7/40 corpus parity).
	// LuaTokenSource remains available to downstream callers via the public
	// API.
}
