//go:build !grammar_subset || grammar_subset_powershell

package grammarruntime

// RegisterPowershellSupport registers the scanner support for powershell.
func RegisterPowershellSupport() {
	RegisterExternalScanner("powershell", PowershellExternalScanner{})
}
