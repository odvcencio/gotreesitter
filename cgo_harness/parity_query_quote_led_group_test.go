//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

func TestParityQueryQuoteLedSiblings(t *testing.T) {
	cases := []exactQueryCase{
		{
			name:   "vue_directive",
			lang:   "vue",
			source: "<template>\n<div v-for=\"member in members\" :key=\"member.name\" class=\"member\"></div>\n</template>\n",
			query:  "(\":\" . (directive_value) @value)",
		},
		{
			name:   "nested_list_siblings",
			lang:   "json",
			source: "[1, 2, 3]",
			query:  "(array (\",\" @separator . (number) @after))",
		},
		{
			name:   "comment_and_capture",
			lang:   "json",
			source: "[1, 2, 3]",
			query:  "(array (\",\" @separator ; this is still a group\n . (number) @after))",
		},
		{
			name:   "singleton_literal",
			lang:   "json",
			source: "[1, 2, 3]",
			query:  "(\",\" @separator) @outer",
		},
		{
			name:   "literal_predicate",
			lang:   "json",
			source: "[1, 2, 3]",
			query:  "(\",\" @separator (#eq? @separator \",\"))",
		},
		{
			name:   "only_relative_neighbor",
			lang:   "json",
			source: "[true, 2, false, 3]",
			query:  "(array (\",\" . (number) @after))",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertExactQueryParity(t, tc) })
	}
}
