//go:build grammar_subset && grammar_subset_yaml

package grammars

func init() {
	Register(LangEntry{
		Name:           "yaml",
		Extensions:     []string{".yaml", ".yml"},
		Language:       YamlLanguage,
		GrammarSource:  GrammarSourceGrammargenBlob,
		HighlightQuery: "(boolean_scalar) @boolean\n\n(null_scalar) @constant.builtin\n\n[\n  (double_quote_scalar)\n  (single_quote_scalar)\n  (block_scalar)\n  (string_scalar)\n] @string\n\n[\n  (integer_scalar)\n  (float_scalar)\n] @number\n\n(comment) @comment\n\n[\n  (anchor_name)\n  (alias_name)\n] @label\n\n(tag) @type\n\n[\n  (yaml_directive)\n  (tag_directive)\n  (reserved_directive)\n] @attribute\n\n(block_mapping_pair\n  key: (flow_node\n    [\n      (double_quote_scalar)\n      (single_quote_scalar)\n    ] @property))\n\n(block_mapping_pair\n  key: (flow_node\n    (plain_scalar\n      (string_scalar) @property)))\n\n(flow_mapping\n  (_\n    key: (flow_node\n      [\n        (double_quote_scalar)\n        (single_quote_scalar)\n      ] @property)))\n\n(flow_mapping\n  (_\n    key: (flow_node\n      (plain_scalar\n        (string_scalar) @property))))\n\n; Recovery fallback for malformed plain scalars like:\n; key: value: trailing\n(stream\n  (flow_node\n    (plain_scalar\n      (string_scalar) @property))\n  \":\"\n  (string_scalar))\n\n[\n  \",\"\n  \"-\"\n  \":\"\n  \">\"\n  \"?\"\n  \"|\"\n] @punctuation.delimiter\n\n[\n  \"[\"\n  \"]\"\n  \"{\"\n  \"}\"\n] @punctuation.bracket\n\n[\n  \"*\"\n  \"&\"\n  \"---\"\n  \"...\"\n] @punctuation.special\n",
	})
}
