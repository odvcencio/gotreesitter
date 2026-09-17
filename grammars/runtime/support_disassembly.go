//go:build !grammar_subset || grammar_subset_disassembly

package grammarruntime

// RegisterDisassemblySupport registers the scanner support for disassembly.
func RegisterDisassemblySupport() {
	RegisterExternalScanner("disassembly", DisassemblyExternalScanner{})
}
