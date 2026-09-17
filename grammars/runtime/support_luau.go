//go:build !grammar_subset || grammar_subset_luau

package grammarruntime

// RegisterLuauSupport registers the scanner support for luau.
func RegisterLuauSupport() {
	RegisterExternalScanner("luau", LuauExternalScanner{})
}
