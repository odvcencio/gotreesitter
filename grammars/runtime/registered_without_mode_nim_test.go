//go:build !grammar_subset && !gotreesitter_no_copyleft

package grammarruntime

func init() {
	extraRegisteredWithoutModeCases = append(extraRegisteredWithoutModeCases, registeredWithoutModeCase{"nim", NimExternalScanner{}})
}
