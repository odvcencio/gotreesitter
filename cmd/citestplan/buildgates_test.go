package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Exercise the actual workflow shell, so adding a needs edge without enforcing
// its result (or enforcing an unbound environment variable) cannot pass.
type gateWorkflow struct {
	Jobs map[string]struct {
		Needs  yaml.Node `yaml:"needs"`
		RunsOn string    `yaml:"runs-on"`
		Steps  []struct {
			Env  map[string]string `yaml:"env"`
			With map[string]string `yaml:"with"`
			Run  string            `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

func readGateWorkflow(t *testing.T) gateWorkflow {
	t.Helper()
	path := os.Getenv("GTS_TEST_GATE_WORKFLOW")
	if path == "" {
		path = "../../.github/workflows/ci.yml"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var wf gateWorkflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatal(err)
	}
	return wf
}

func TestO1BuildAggregateRequiresGateResults(t *testing.T) {
	wf := readGateWorkflow(t)
	build := wf.Jobs["build"]
	if len(build.Steps) != 1 {
		t.Fatalf("expected one aggregate step, got %d", len(build.Steps))
	}
	step := build.Steps[0]
	base := map[string]string{
		"IS_DRAFT": "false", "IS_PULL_REQUEST": "true",
		"IS_MANUAL_RUN": "false",
		"RUN_CODE_CI":   "true", "EXHAUSTIVE_PARITY_SCOPE": "false",
	}
	for key := range step.Env {
		if strings.HasSuffix(key, "_RESULT") {
			base[key] = "success"
		}
	}
	run := func(overrides map[string]string) ([]byte, error) {
		cmd := exec.Command("bash", "-c", step.Run)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
		for key, value := range base {
			if v, ok := overrides[key]; ok {
				value = v
			}
			cmd.Env = append(cmd.Env, key+"="+value)
		}
		return cmd.CombinedOutput()
	}
	if out, err := run(nil); err != nil {
		t.Fatalf("successful code gates rejected: %v\n%s", err, out)
	}
	for _, gate := range []string{"exhaustive_parity_scope", "phase0_tagged_suite", "parity-cgo", "glr_gss_demotion_scaling_gate", "apidiff", "editor_latency", "wasm_cross_build"} {
		t.Run(gate, func(t *testing.T) {
			if !containsGate(build.Needs, gate) {
				t.Fatalf("build.needs omits %s", gate)
			}
			key := ""
			for name, value := range step.Env {
				if value == "${{ needs."+gate+".result }}" {
					key = name
				}
			}
			if key == "" {
				t.Fatalf("aggregate does not bind %s.result", gate)
			}
			for _, result := range []string{"failure", "cancelled", "skipped"} {
				out, err := run(map[string]string{key: result})
				if err == nil || !strings.Contains(string(out), gate+" finished with result="+result) {
					t.Errorf("%s must block build: err=%v output=%s", result, err, out)
				}
			}
		})
	}
	// A cancelled scope job emits no outputs. Its dependent jobs never run,
	// so neither the missing flag nor their cancellation can mean docs-only.
	for _, result := range []string{"failure", "cancelled", "skipped"} {
		t.Run("missing scope outputs/"+result, func(t *testing.T) {
			out, err := run(map[string]string{
				"EXHAUSTIVE_PARITY_SCOPE_RESULT": result,
				"RUN_CODE_CI":                    "",
				"EXHAUSTIVE_PARITY_SCOPE":        "",
			})
			if err == nil || !strings.Contains(string(out), "exhaustive_parity_scope finished with result="+result) ||
				!strings.Contains(string(out), "exhaustive_parity_scope emitted invalid run_code_ci=") {
				t.Fatalf("missing scope outputs must block build: err=%v output=%s", err, out)
			}
		})
	}
	if _, exists := wf.Jobs["grammargen_visibility"]; exists {
		t.Error("obsolete grammargen_visibility job still exists")
	}
}

func containsGate(names yaml.Node, target string) bool {
	for _, name := range names.Content {
		if name.Value == target {
			return true
		}
	}
	return false
}

func TestWasmCrossBuildTargetsHaveSeparateBudgets(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Jobs map[string]struct {
			Timeout  int `yaml:"timeout-minutes"`
			Strategy struct {
				FailFast *bool `yaml:"fail-fast"`
				Matrix   struct {
					GOOS []string `yaml:"goos"`
				} `yaml:"matrix"`
			} `yaml:"strategy"`
			Steps []struct {
				Run string            `yaml:"run"`
				Env map[string]string `yaml:"env"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatal(err)
	}
	job := wf.Jobs["wasm_cross_build"]
	if job.Timeout != 10 || job.Strategy.FailFast == nil || *job.Strategy.FailFast || strings.Join(job.Strategy.Matrix.GOOS, ",") != "js,wasip1" {
		t.Fatalf("Wasm targets must each retain a 10-minute budget without fail-fast: %+v", job)
	}
	builds := 0
	for _, step := range job.Steps {
		if strings.Contains(step.Run, "go build") {
			builds++
			if step.Run != "go build ./..." || step.Env["GOOS"] != "${{ matrix.goos }}" || step.Env["GOARCH"] != "wasm" {
				t.Errorf("Wasm step must build the full module for its matrix target: %+v", step)
			}
		}
	}
	if builds != 1 {
		t.Fatalf("build steps per Wasm target = %d, want 1", builds)
	}
	wfGate := readGateWorkflow(t)
	if !containsGate(wfGate.Jobs["build"].Needs, "wasm_cross_build") {
		t.Fatal("aggregate build gate must require both Wasm targets")
	}
}

func TestO1APIDiffUsesLatestStableTag(t *testing.T) {
	wf := readGateWorkflow(t)
	var script string
	for _, step := range wf.Jobs["apidiff"].Steps {
		if strings.Contains(step.Run, "APIDIFF_BASELINE_TAG=") {
			script = step.Run
		}
	}
	// Execute the production selector and its empty-tag guard, stopping before
	// API export (which needs Go dependencies and is tested by the apidiff job).
	start := strings.Index(script, "APIDIFF_BASELINE_TAG=")
	end := strings.Index(script, "OLD_DIR=")
	if start < 0 || end <= start {
		t.Fatal("apidiff does not derive and validate its tag baseline")
	}
	selector := "set -euo pipefail\n" + script[start:end]
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("-c", "user.name=CI test", "-c", "user.email=ci@example.org", "commit", "--allow-empty", "-qm", "fixture")
	run := func() ([]byte, error) {
		cmd := exec.Command("bash", "-c", selector)
		cmd.Dir = dir
		return cmd.CombinedOutput()
	}
	if out, err := run(); err == nil || !strings.Contains(string(out), "no stable version tag") {
		t.Fatalf("missing baseline must fail: %v\n%s", err, out)
	}
	for _, tag := range []string{"v0.9.0", "v0.49.0", "v0.55.0", "v1.0.0-rc.1", "v9.0.0-beta.1", "v3-not-a-release"} {
		git("tag", tag)
	}
	for _, want := range []string{"v0.55.0", "v0.55.1", "v0.100.0", "v1.0.0"} {
		if want != "v0.55.0" {
			git("tag", want)
		}
		out, err := run()
		if err != nil || strings.TrimSpace(string(out)) != fmt.Sprintf("Using public API baseline %s", want) {
			t.Fatalf("latest stable tag = %s: %v\n%s", want, err, out)
		}
	}
}
