//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

func TestGleamIntegerSelfConflictLockedCParity(t *testing.T) {
	for _, source := range []string{
		"{-1}", "{1}", "{-0}", "{0}", "{-123_456}", "{123_456}",
		"{-0xff}", "{0xff}", "{-0b1010}", "{0b1010}",
		"{-1.5}", "{1.5}", "{-x}", "{1 - 2}", "{1 - -2}",
		"{f(-1, 2)}", "{[-1, 0, 1]}", "{#(-1, 2)}", "{(-1)}",
		"fn main() { -1 }\n", "fn main() { 1 }\n",
	} {
		t.Run(source, func(t *testing.T) {
			for _, candidate := range []bool{false, true} {
				runParityCase(t, parityCase{name: "gleam", candidateRoute: &candidate}, "integer-self-conflict", []byte(source))
			}
		})
	}
}
