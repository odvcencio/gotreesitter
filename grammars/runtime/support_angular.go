//go:build !grammar_subset || grammar_subset_angular

package grammarruntime

// RegisterAngularSupport registers the scanner support for angular.
func RegisterAngularSupport() {
	RegisterExternalScanner("angular", AngularExternalScanner{})
}
