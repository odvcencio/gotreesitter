//go:build !grammar_subset || grammar_subset_php

package grammarruntime

// RegisterPhpSupport registers the scanner support for php.
func RegisterPhpSupport() {
	RegisterExternalScanner("php", PhpExternalScanner{})
}
