//go:build !grammar_subset || grammar_subset_cooklang

package grammarruntime

// RegisterCooklangSupport registers the scanner support for cooklang.
func RegisterCooklangSupport() {
	RegisterExternalScanner("cooklang", CooklangExternalScanner{})
}
