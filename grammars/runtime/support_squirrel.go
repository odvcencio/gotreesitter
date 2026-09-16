//go:build !grammar_subset || grammar_subset_squirrel

package grammarruntime

// RegisterSquirrelSupport registers the scanner support for squirrel.
func RegisterSquirrelSupport() {
	RegisterExternalScanner("squirrel", SquirrelExternalScanner{})
}
