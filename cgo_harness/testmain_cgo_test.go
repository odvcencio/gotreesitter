//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

const (
	parityInDockerEnv   = "GTS_PARITY_IN_DOCKER"
	parityAllowHostEnv  = "GTS_PARITY_ALLOW_HOST"
	parityGuardrailText = "cgo parity tests are container-only; use cgo_harness/docker/run_parity_in_docker.sh or set GTS_PARITY_ALLOW_HOST=1 for a focused local debug run"
)

func TestMain(m *testing.M) {
	if !parityRunningInContainer() && !parityEnvBool(parityAllowHostEnv, false) {
		fmt.Fprintln(os.Stderr, parityGuardrailText)
		os.Exit(2)
	}
	// GTS_ADMISSION_CENSUS=1 makes the A3 certification sweeps' decline
	// classification honest (a3DeclineReasonClass,
	// a3_certification_sweep_helpers_test.go): without it, every soft
	// decline collapses to the generic "did not accept EOF" detail
	// (parsercore_phase0_fresh_full_runner.go,
	// requireParserCoreFreshFullAcceptance), which shadows the more
	// specific material-acceptance-election bucket. gotreesitter's own
	// admission_census.go reads this once via sync.Once, so it must be set
	// here, before m.Run(), rather than per-test with t.Setenv -- any test
	// that reaches a decline first would otherwise lock the cached read to
	// whatever the env var held at that moment, regardless of what a later
	// test sets. Safe process-wide: per admission_census.go's own
	// contract, this only changes the TEXT of an already-decided decline
	// reason (never what the compact route accepts, declines, or
	// materializes), and no test in this package asserts an exact decline
	// reason string (only substring checks and %s formatting into failure
	// messages).
	os.Setenv("GTS_ADMISSION_CENSUS", "1")
	os.Exit(m.Run())
}

func parityRunningInContainer() bool {
	if parityEnvBool(parityInDockerEnv, false) {
		return true
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	data, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	text := string(data)
	return strings.Contains(text, "docker") ||
		strings.Contains(text, "kubepods") ||
		strings.Contains(text, "containerd")
}

func parityEnvBool(name string, def bool) bool {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}
