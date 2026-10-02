//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

func TestHaskellLayoutEndLockedCParity(t *testing.T) {
	for _, source := range []string{
		"a=do\"\"[\"\"]\n",
		"a=do\"\"[\"\"]",
		"a = do\n  f [\"\"]\n",
		"a = do\n  f [\"\"]\n\nb = 1\n",
		"a = do\n  f [\"\"]\n  -- trailing comment\n",
	} {
		t.Run(source, func(t *testing.T) {
			for _, candidate := range []bool{false, true} {
				runParityCase(t, parityCase{name: "haskell", candidateRoute: &candidate}, "layout-end", []byte(source))
			}
		})
	}
}
