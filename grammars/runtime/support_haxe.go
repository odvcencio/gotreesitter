//go:build !grammar_subset || grammar_subset_haxe

package grammarruntime

// RegisterHaxeSupport registers the scanner support for haxe.
func RegisterHaxeSupport() {
	RegisterExternalScanner("haxe", HaxeExternalScanner{})
}
