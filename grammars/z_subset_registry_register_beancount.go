//go:build grammar_subset && grammar_subset_beancount

package grammars

func init() {
	Register(LangEntry{
		Name:           "beancount",
		Extensions:     []string{".beancount"},
		Language:       BeancountLanguage,
		GrammarSource:  GrammarSourceTS2GoBlob,
		HighlightQuery: "; Entry type keywords\n[\n  \"open\"\n  \"close\"\n  \"balance\"\n  \"pad\"\n  \"note\"\n  \"document\"\n  \"price\"\n  \"event\"\n  \"query\"\n  \"custom\"\n  \"commodity\"\n  \"txn\"\n] @keyword\n\n; Directive keywords\n[\n  \"pushtag\"\n  \"poptag\"\n  \"pushmeta\"\n  \"popmeta\"\n  \"option\"\n  \"include\"\n  \"plugin\"\n] @keyword.import\n\n; Transaction and posting flags\n(txn) @keyword\n(flag) @keyword\n\n; Dates\n(date) @string.special\n\n; Accounts\n(account) @variable\n\n; Currencies\n(currency) @type\n\n; Strings\n(string) @string\n(unquoted_string) @string\n(narration) @string\n(payee) @string.special\n\n; Numbers\n(number) @number\n\n; Booleans and null\n(bool) @boolean\n\"NULL\" @constant.builtin\n\n; Tags and links\n(tag) @label\n(link) @label\n\n; Metadata keys\n(key_value (key) @property)\n\n; Arithmetic operators\n[\n  (plus)\n  (minus)\n  (asterisk)\n  (slash)\n] @operator\n\n; Price annotation operators\n[\n  (at)\n  (atat)\n] @operator\n\n; Punctuation\n[\"(\" \")\"] @punctuation.bracket\n[\"{\" \"}\" \"{{\" \"}}\"] @punctuation.bracket\n[\",\" \"~\" \":\"] @punctuation.delimiter\n\n; Comments\n(comment) @comment\n\n; Org-mode / markdown section headlines\n(headline (item) @markup.heading)\n",
	})
}
