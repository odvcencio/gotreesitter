//go:build !grammar_subset || grammar_subset_blade

package grammarruntime

// RegisterBladeSupport registers the scanner support for blade.
func RegisterBladeSupport() {
	RegisterExternalScanner("blade", BladeExternalScanner{})
}
