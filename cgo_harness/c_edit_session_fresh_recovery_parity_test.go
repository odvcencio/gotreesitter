//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"github.com/odvcencio/gotreesitter/grammars"
	"testing"
)

// These witnesses were reduced from cumulative edits of Git's ctype.c.
// Compare every node, including fields, spans, missing flags and ERROR extras,
// against the languages.lock C runtime rather than checking only the SExpr.
func TestCEditSessionFreshRecoveryWitnessParity(t *testing.T) {
	lang := grammars.DetectLanguageByName("c").Language()
	previous := lang.RecoveryStackVersionOrderEnabled
	lang.RecoveryStackVersionOrderEnabled = true
	t.Cleanup(func() { lang.RecoveryStackVersionOrderEnabled = previous })
	witnesses := []struct {
		name   string
		source string
	}{
		{"enumerator-recovery-order", "enum{,G//\nL E}"},
		{"initializer-action-transaction", "r s={A,A,A,x G,U,"},
		{"closed-error-packed-links", "r s\tA,"},
		{"recovery-competitor-position", "X=GITT_CNTRL x|"},
		{"paused-keyword-lookahead", "L\tt for]"},
	}
	for _, witness := range witnesses {
		for _, candidate := range []bool{false, true} {
			name := witness.name + "/default"
			if candidate {
				name = witness.name + "/candidate"
			}
			t.Run(name, func(t *testing.T) {
				runParityCase(t, parityCase{name: "c", candidateRoute: &candidate}, witness.name, []byte(witness.source))
			})
		}
	}
}
