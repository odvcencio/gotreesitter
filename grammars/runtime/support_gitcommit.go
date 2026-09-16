//go:build !grammar_subset || grammar_subset_gitcommit

package grammarruntime

// RegisterGitcommitSupport registers the scanner support for gitcommit.
func RegisterGitcommitSupport() {
	RegisterExternalScanner("gitcommit", GitcommitExternalScanner{})
}
