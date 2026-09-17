//go:build !grammar_subset || grammar_subset_lua

package grammarruntime

// RegisterLuaSupport registers the scanner support for lua.
func RegisterLuaSupport() {
	RegisterExternalScanner("lua", LuaExternalScanner{})
	RegisterExternalLexStates("lua", luaExternalLexStates)
}
