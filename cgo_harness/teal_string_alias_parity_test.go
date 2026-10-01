//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

func TestTealStringAliasLockedCParity(t *testing.T) {
	for _, source := range []string{
		`d{["%"]=""}`,
		`local t = { ["%"] = "" }`,
		`local s = "%"`,
		`local s = "a%b"`,
		`local s = [[%]]`,
		`local s = "plain"`,
	} {
		t.Run(source, func(t *testing.T) {
			for _, candidate := range []bool{false, true} {
				runParityCase(t, parityCase{name: "teal", candidateRoute: &candidate}, "string-alias", []byte(source))
			}
		})
	}
}
