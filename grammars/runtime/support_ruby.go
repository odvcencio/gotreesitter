//go:build !grammar_subset || grammar_subset_ruby

package grammarruntime

// RegisterRubySupport registers the scanner support for ruby.
func RegisterRubySupport() {
	RegisterExternalScanner("ruby", RubyExternalScanner{})
}
