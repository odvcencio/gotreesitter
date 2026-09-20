//go:build !grammar_subset || (grammar_subset_doxygen && grammar_subset_c_sharp)

package grammars

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/odvcencio/gotreesitter"
	grammarruntime "github.com/odvcencio/gotreesitter/grammars/runtime"
)

// TestCRecoveryGateDoxygenOptOut is task #71 item 1's receipt for pine's
// 2026-09-21 doxygen diagnosis. The shipped doxygen.bin has zero
// ExternalLexStates rows, so the C-recovery cost-competition gate is off
// today. A routine regeneration from the locked parser.c restores the 8
// ExternalLexStates rows and, without an explicit opt-out, would flip the
// gate on with no recovery-board evidence (generatedCRecoveryDefaultSafe,
// parser_recover_c.go). The cRecoveryDefaultOptOut("doxygen") entry pins
// today's recovery behavior for both witnesses even against that
// regenerated blob:
//
//   - "/** Adds all words in \a s to document \a doc with weight \a wfd */"
//   - "/**\n * @param {int} value\n * @brief Example\n */"
//
// c_sharp is the control: it is capable and on by default today (its own
// precise ExternalLexStates sidecar plus attached scanner earn it default
// election — TestCSharpExternalLexStatesRegression). This guards against the
// opt-out check silently suppressing every language's default instead of
// just doxygen's.
func TestCRecoveryGateDoxygenOptOut(t *testing.T) {
	const parserSrc = "/tmp/grammar_parity/doxygen/src/parser.c"
	if _, err := os.Stat(parserSrc); err != nil {
		t.Skipf("doxygen grammar not seeded (run cgo_harness/seed_parity_repos.sh --langs doxygen first): %v", err)
	}

	scratch := t.TempDir()
	outputGo := filepath.Join(scratch, "doxygen.go")
	genCmd := exec.Command("go", "run", "./cmd/ts2go",
		"-input", parserSrc,
		"-output", outputGo,
		"-name", "doxygen",
		"-package", "grammars",
	)
	genCmd.Dir = ".." // cmd/ts2go lives at the module root, one level above grammars/.
	if out, err := genCmd.CombinedOutput(); err != nil {
		t.Fatalf("go run ./cmd/ts2go: %v\n%s", err, out)
	}
	blob, err := os.ReadFile(filepath.Join(scratch, "grammar_blobs", "doxygen.bin"))
	if err != nil {
		t.Fatalf("read regenerated blob: %v", err)
	}

	regen, err := gotreesitter.LoadLanguage(blob)
	if err != nil {
		t.Fatalf("LoadLanguage: %v", err)
	}
	if !grammarruntime.AttachLanguageSupport("doxygen", regen) {
		t.Fatal("AttachLanguageSupport(doxygen) failed to attach the external scanner")
	}
	if len(regen.ExternalLexStates) == 0 {
		t.Fatal("regenerated doxygen blob has zero ExternalLexStates rows; seed a fresher grammar_parity checkout")
	}
	if !regen.CRecoveryCostCompetitionCapable {
		t.Fatal("regenerated doxygen blob is not C-recovery capable; the opt-out check needs a capable language for this test to be meaningful")
	}
	if regen.CRecoveryCostCompetitionEnabledByDefault {
		t.Fatalf(
			"doxygen gate is on by default with a regenerated blob (extRows=%d); cRecoveryDefaultOptOut(\"doxygen\") should keep it off until the recovery board has evidence",
			len(regen.ExternalLexStates),
		)
	}

	control := CSharpLanguage()
	if !control.CRecoveryCostCompetitionCapable || !control.CRecoveryCostCompetitionEnabledByDefault {
		t.Fatalf("control language c_sharp gate changed: capable=%v default=%v (want both true)",
			control.CRecoveryCostCompetitionCapable, control.CRecoveryCostCompetitionEnabledByDefault)
	}
}
