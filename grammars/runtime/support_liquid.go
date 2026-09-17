//go:build !grammar_subset || grammar_subset_liquid

package grammarruntime

// RegisterLiquidSupport registers the scanner support for liquid.
func RegisterLiquidSupport() {
	RegisterExternalScanner("liquid", LiquidExternalScanner{})
}
