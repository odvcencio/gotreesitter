//go:build (!grammar_subset || grammar_subset_disassembly) && !gotreesitter_no_copyleft

package grammarruntime

// RegisterDisassemblySupport registers the scanner support for disassembly.
// See disassembly_no_copyleft_stub.go for the gotreesitter_no_copyleft
// build, which keeps this function's signature but drops its
// GPL-3.0-derived body.
func RegisterDisassemblySupport() {
	RegisterExternalScanner("disassembly", DisassemblyExternalScanner{})
}
