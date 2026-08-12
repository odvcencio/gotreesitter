//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

// TestEnforceConstParamRetirementLockedCParity locks the C-equivalent shape
// for the Enforce construct the retired result-compatibility arm used to
// repair after the fact: a `const int` formal parameter, with or without a
// default value, parsing as `(formal_parameter_modifier) (type_int)
// (identifier) ...` rather than a misread type/name pair that drops the
// parameter's own name. See
// grammars/enforce_const_param_native_regression_test.go for the
// compatibility-free and four-route receipts.
func TestEnforceConstParamRetirementLockedCParity(t *testing.T) {
	for name, src := range map[string]string{
		"const_int_default_value":                 "class A { void m(const int x = 69) {} }\n",
		"const_int_no_default":                    "class A { void m(const int x) {} }\n",
		"const_int_default_among_other_modifiers": "override void foo(const int x = 69, inout int y, out int y) {}\n",
		"const_int_default_expression":            "void foo(const int x = 1 + 2) {}\n",
		"const_int_forward_declaration":           "void foo(const int x = 69);\n",
	} {
		name, src := name, src
		t.Run(name, func(t *testing.T) {
			runParityCase(t, parityCase{name: "enforce"}, "const-param-retirement", []byte(src))
		})
	}
}
