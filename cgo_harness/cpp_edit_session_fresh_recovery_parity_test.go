//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestCppEditSessionFreshRecoveryWitnessParity(t *testing.T) {
	lang := grammars.DetectLanguageByName("cpp").Language()
	previous := lang.RecoveryStackVersionOrderEnabled
	lang.RecoveryStackVersionOrderEnabled = true
	t.Cleanup(func() { lang.RecoveryStackVersionOrderEnabled = previous })
	t.Setenv("GOT_C_RECOVERY", "cpp")
	gotreesitter.ResetParseEnvConfigCacheForTests()
	t.Cleanup(gotreesitter.ResetParseEnvConfigCacheForTests)
	witnesses := []struct {
		name   string
		source string
	}{
		{"literal-suffix-relex", `""t`},
		{"hidden-missing-newline-cost", ".#i"},
		{"eof-missing-token-trial", ",;e"},
		{"pause-progress-baseline", ", ;\n se"},
	}
	for _, witness := range witnesses {
		for _, candidate := range []bool{false, true} {
			name := witness.name + "/default"
			if candidate {
				name = witness.name + "/candidate"
			}
			t.Run(name, func(t *testing.T) {
				runParityCase(t, parityCase{name: "cpp", candidateRoute: &candidate}, witness.name, []byte(witness.source))
			})
		}
	}
}
