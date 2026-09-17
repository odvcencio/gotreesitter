//go:build !grammar_subset || grammar_subset_kotlin

package grammarruntime

// RegisterKotlinSupport registers the scanner support for kotlin.
func RegisterKotlinSupport() {
	RegisterExternalScanner("kotlin", KotlinExternalScanner{})
}
