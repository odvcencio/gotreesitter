//go:build grammar_subset && grammar_subset_doxygen

package grammars

func init() {
	Register(LangEntry{
		Name:           "doxygen",
		Language:       DoxygenLanguage,
		GrammarSource:  GrammarSourceTS2GoBlob,
		HighlightQuery: "((tag_name) @keyword\n  (#set! \"priority\" 105))\n\n[\n  \"@code\"\n  \"@endcode\"\n] @keyword\n\n(identifier) @variable\n\n((tag\n  (tag_name) @_param\n  (identifier) @parameter)\n  (#any-of? @_param \"@param\" \"\\\\param\"))\n\n(function\n  (identifier) @function)\n\n(function_link) @function\n\n(emphasis) @text.emphasis\n\n[\n  \"\\\\a\"\n  \"\\\\c\"\n] @tag\n\n(code_block_language) @label\n\n[\n  \"in\"\n  \"out\"\n  \"inout\"\n] @storageclass\n\n\"~\" @operator\n\n[\n  \"<a\"\n  \">\"\n  \"</a>\"\n] @tag\n\n[\n  \".\"\n  \",\"\n  \"::\"\n  (code_block_start)\n  (code_block_end)\n] @punctuation.delimiter\n\n[\n  \"(\"\n  \")\"\n  \"{\"\n  \"}\"\n  \"[\"\n  \"]\"\n] @punctuation.bracket\n\n(code_block_content) @none\n",
	})
}
