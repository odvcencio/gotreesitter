//go:build !grammar_subset || grammar_subset_crystal

package grammarruntime

// RegisterCrystalSupport registers the scanner support for crystal.
func RegisterCrystalSupport() {
	RegisterExternalScanner("crystal", CrystalExternalScanner{})
}
