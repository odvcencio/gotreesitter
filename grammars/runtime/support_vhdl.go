//go:build !grammar_subset || grammar_subset_vhdl

package grammarruntime

// RegisterVhdlSupport registers the scanner support for vhdl.
func RegisterVhdlSupport() {
	RegisterExternalScanner("vhdl", VhdlExternalScanner{})
}
