//go:build !grammar_subset || grammar_subset_markdown

package grammarruntime

// RegisterMarkdownSupport registers the scanner support for markdown.
func RegisterMarkdownSupport() {
	RegisterExternalScanner("markdown", MarkdownExternalScanner{})
}
