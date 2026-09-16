//go:build !grammar_subset || grammar_subset_fortran

package grammarruntime

// RegisterFortranSupport registers the scanner support for fortran.
func RegisterFortranSupport() {
	RegisterExternalScanner("fortran", FortranExternalScanner{})
}
