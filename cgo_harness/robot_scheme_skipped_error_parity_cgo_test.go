//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

func TestRobotSkippedErrorLockedCParity(t *testing.T) {
	runParityCase(
		t,
		parityCase{name: "robot"},
		"expression",
		[]byte(`${tc['\${i}']}`),
	)
}

func TestSchemeQuoteFamilySkippedErrorLockedCParity(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
	}{
		{name: "quote_adjacent_space", source: "'| x"},
		{name: "quote_separated_space", source: "' | x"},
		{name: "quote_adjacent", source: "'|x"},
		{name: "quote_separated", source: "' |x"},
		{name: "quasiquote", source: "`| x"},
		{name: "unquote", source: ",| x"},
		{name: "unquote_splicing", source: ",@| x"},
		{name: "syntax", source: "#'| x"},
		{name: "quasisyntax", source: "#`| x"},
		{name: "unsyntax", source: "#,| x"},
		{name: "unsyntax_splicing", source: "#,@| x"},
		{name: "list", source: "('| x)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runParityCase(
				t,
				parityCase{name: "scheme"},
				test.name,
				[]byte(test.source),
			)
		})
	}
}
