//go:build !grammar_subset || grammar_subset_toml

package grammarruntime

// RegisterTomlSupport registers the scanner support for toml.
func RegisterTomlSupport() {
	RegisterExternalScanner("toml", TomlExternalScanner{})
}
