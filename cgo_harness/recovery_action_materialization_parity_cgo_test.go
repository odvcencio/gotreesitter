//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

func TestForthRecoveryActionMaterializationLockedCParity(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
	}{
		{name: "body", source: ": foo 1 2\n"},
		{name: "empty", source: ": foo\n"},
		{name: "terminated", source: ": foo 1 2 ;\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runParityCase(
				t,
				parityCase{name: "forth"},
				test.name,
				[]byte(test.source),
			)
		})
	}
}

func TestLuauRecoveryActionMaterializationLockedCParity(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
	}{
		{name: "end", source: "end"},
		{name: "declaration_end", source: "local x = 1\nend\n"},
		{name: "if_end", source: "if true then\nend\nend\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runParityCase(
				t,
				parityCase{name: "luau"},
				test.name,
				[]byte(test.source),
			)
		})
	}
}
