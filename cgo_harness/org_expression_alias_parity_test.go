//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

func TestOrgExpressionAliasLockedCParity(t *testing.T) {
	for _, source := range []string{
		"#+BEGIN_\"\n#+END_S",
		"#+TITLE: Hello\n",
		"#+X: a\n",
		"#+BEGIN_SRC sh\nprintf x\n#+END_SRC\n",
		"- [X] done\n",
		"plain text\n",
	} {
		t.Run(source, func(t *testing.T) {
			for _, candidate := range []bool{false, true} {
				runParityCase(t, parityCase{name: "org", candidateRoute: &candidate}, "expression-alias", []byte(source))
			}
		})
	}
}
