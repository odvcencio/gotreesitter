//go:build !grammar_subset || grammar_subset_markdown_inline

package grammarruntime

// RegisterMarkdown_inlineSupport registers the scanner support for markdown_inline.
func RegisterMarkdown_inlineSupport() {
	RegisterExternalScanner("markdown_inline", MarkdownInlineExternalScanner{})
}
