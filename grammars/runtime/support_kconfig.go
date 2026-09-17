//go:build !grammar_subset || grammar_subset_kconfig

package grammarruntime

// RegisterKconfigSupport registers the scanner support for kconfig.
func RegisterKconfigSupport() {
	RegisterExternalScanner("kconfig", KconfigExternalScanner{})
}
