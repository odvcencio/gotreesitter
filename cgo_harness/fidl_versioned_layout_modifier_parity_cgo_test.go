//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

// TestFIDLVersionedLayoutModifiersLockedCParity checks the same
// versioned-layout-modifier witnesses grammars/fidl_versioned_layout_modifier_native_regression_test.go
// locks against a native C tree-sitter reference parser, node by node.
func TestFIDLVersionedLayoutModifiersLockedCParity(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
	}{
		{
			name:   "two_modifiers_with_args",
			source: "library test;\ntype Color = strict(removed=2) flexible(added=2) enum {\n    RED = 1;\n};",
		},
		{
			name:   "single_modifier_with_args",
			source: "library test;\ntype Color = strict(removed=2) enum {\n    RED = 1;\n};",
		},
		{
			name:   "three_modifiers_with_args",
			source: "library test;\ntype Color = strict(removed=2) flexible(added=2) resource(added=3) enum {\n    RED = 1;\n};",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			runParityCase(
				t,
				parityCase{name: "fidl"},
				test.name,
				[]byte(test.source),
			)
		})
	}
}
