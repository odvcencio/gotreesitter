//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

// TestHLSLSubscriptAssignmentLockedCParity checks the same
// subscript-assignment witnesses grammars/hlsl_subscript_assignment_native_regression_test.go
// locks against a native C tree-sitter reference parser, node by node.
func TestHLSLSubscriptAssignmentLockedCParity(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
	}{
		{name: "single_index_in_function", source: "void f() {\n    Foo[bar] = value;\n}\n"},
		{name: "top_level", source: "Foo[bar] = value;\n"},
		{name: "two_indices", source: "void f() {\n    Foo[bar, baz] = value;\n}\n"},
		{name: "numeric_index", source: "void f() {\n    Foo[42] = value;\n}\n"},
		{name: "call_value", source: "void f() {\n    Foo[bar] = compute(baz);\n}\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runParityCase(
				t,
				parityCase{name: "hlsl"},
				test.name,
				[]byte(test.source),
			)
		})
	}
}
