//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

// TestElixirExtrasAndPairsRetirementLockedCParity locks the C-equivalent
// shape for the two Elixir constructs the retired result-compatibility arm
// used to repair after the fact: the external scanner's hidden
// `_newline_before_comment` synchronization token never surfacing as a tree
// node, and map keyword-pair/update-operator grouping. See
// grammars/elixir_extras_and_pairs_native_regression_test.go for the
// compatibility-free and four-route receipts.
func TestElixirExtrasAndPairsRetirementLockedCParity(t *testing.T) {
	for name, src := range map[string]string{
		"root_comments_separated_by_newline":      "# one\n# two\n\n[]\n",
		"do_block_comment_before_keyword":         "fun x\n# comment\ndo\n  x\nend\n",
		"map_consecutive_keyword_pairs":           "%{a: 1, b: 2}\n",
		"map_keyword_pairs_split_by_comment":      "%{\n  a: 1,\n  # comment\n  b: 2\n}\n",
		"map_update_syntax_wraps_binary_operator": "%{map | name: \"Silly\"}\n",
		"map_arrow_entries_wrap_binary_operator":  "%{1 => 2, 3 => 4}\n",
		"map_mixed_arrow_and_keyword_entries":     "%{\"a\" => 1, b: 2, c: 3}\n",
	} {
		name, src := name, src
		t.Run(name, func(t *testing.T) {
			runParityCase(t, parityCase{name: "elixir"}, "extras-and-pairs-retirement", []byte(src))
		})
	}
}
