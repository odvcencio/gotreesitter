//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

func TestFSharpUnaryWrapperRetirementCParity(t *testing.T) {
	source := []byte(
		"open Namespace.Module\n" +
			"let simple = Alpha\n",
	)
	runParityCase(
		t,
		parityCase{name: "fsharp"},
		"unary-wrapper-retirement",
		source,
	)
}
