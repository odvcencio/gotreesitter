//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

func TestCommonLispReadTimeFieldLockedCParity(t *testing.T) {
	for _, source := range []string{
		"(#.())",
		"(#.()()(((()))))",
		"(#.(a)b)",
		"(#.(+ 1 2) other)",
		"('a `b ,c ,@d)",
		"(()()(((()))))",
	} {
		t.Run(source, func(t *testing.T) {
			for _, candidate := range []bool{false, true} {
				runParityCase(t, parityCase{name: "commonlisp", candidateRoute: &candidate}, "read-time-field", []byte(source))
			}
		})
	}
}
