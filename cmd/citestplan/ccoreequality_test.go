package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The required job ccore-equality-cgo cannot be softened: no continue-on-error
// on the job or a step, no condition on its steps, a matrix with the single key
// language, and a language list equal to the languages that
// cgo_harness/docker/ccore_equality.sh accepts (read from the script's case
// statement; the test fails if the list cannot be read).
func TestCcoreEqualityGateCannotBeSoftened(t *testing.T) {
	path := os.Getenv("GTS_TEST_GATE_WORKFLOW")
	if path == "" {
		path = "../../.github/workflows/ci.yml"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Jobs map[string]map[string]any `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatal(err)
	}
	job, ok := wf.Jobs["ccore-equality-cgo"]
	if !ok {
		t.Fatal("job ccore-equality-cgo is missing")
	}
	if _, has := job["continue-on-error"]; has {
		t.Error("job ccore-equality-cgo must not set continue-on-error")
	}
	strategy, _ := job["strategy"].(map[string]any)
	matrix, _ := strategy["matrix"].(map[string]any)
	if _, has := matrix["language"]; !has || len(matrix) != 1 {
		t.Fatalf("ccore-equality-cgo matrix must have the single key language: %v", matrix)
	}
	var inMatrix []string
	langs, _ := matrix["language"].([]any)
	for _, l := range langs {
		inMatrix = append(inMatrix, fmt.Sprint(l))
	}
	sort.Strings(inMatrix)

	script, err := os.ReadFile("../../cgo_harness/docker/ccore_equality.sh")
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)
	start := strings.Index(text, "case \"$lang\" in")
	if start < 0 {
		t.Fatal("cannot read the language list: no case statement in ccore_equality.sh")
	}
	end := strings.Index(text[start:], "esac")
	if end < 0 {
		t.Fatal("cannot read the language list: the case statement has no esac")
	}
	arm := regexp.MustCompile(`(?m)^\s+([A-Za-z0-9_]+)\)`)
	var accepted []string
	for _, m := range arm.FindAllStringSubmatch(text[start:start+end], -1) {
		accepted = append(accepted, m[1])
	}
	if len(accepted) == 0 {
		t.Fatal("cannot read the language list: no language arm in ccore_equality.sh")
	}
	sort.Strings(accepted)
	if strings.Join(inMatrix, ",") != strings.Join(accepted, ",") {
		t.Errorf("matrix languages %v differ from the languages of ccore_equality.sh %v", inMatrix, accepted)
	}

	steps, _ := job["steps"].([]any)
	if len(steps) == 0 {
		t.Fatal("job ccore-equality-cgo has no steps")
	}
	equalityStep := false
	var equalityRun string
	for i, raw := range steps {
		step, _ := raw.(map[string]any)
		if _, has := step["continue-on-error"]; has {
			t.Errorf("ccore-equality-cgo step %d (%v) must not set continue-on-error", i, step["name"])
		}
		name := fmt.Sprint(step["name"])
		// Only the runner preparation may be conditional.
		if _, has := step["if"]; has && name != "Prepare job-owned Go cache and scratch" {
			t.Errorf("ccore-equality-cgo step %d (%v) must not have an if", i, name)
		}
		if strings.Contains(name, "equals pinned C") {
			if equalityStep {
				t.Error("ccore-equality-cgo has more than one equality step")
			}
			equalityStep = true
			equalityRun, _ = step["run"].(string)
		}
	}
	if !equalityStep {
		t.Error("ccore-equality-cgo has no equality step (name containing \"equals pinned C\")")
	}

	// The job itself: its condition, its dependency and its time limit.
	if job["if"] != ccoreEqualityJobIf {
		t.Errorf("ccore-equality-cgo job condition changed:\n got %v\nwant %s", job["if"], ccoreEqualityJobIf)
	}
	if job["needs"] != "exhaustive_parity_scope" {
		t.Errorf("ccore-equality-cgo must need exhaustive_parity_scope alone: %v", job["needs"])
	}
	if job["timeout-minutes"] != 60 {
		t.Errorf("ccore-equality-cgo timeout-minutes changed: %v", job["timeout-minutes"])
	}

	// The aggregate that makes the job required is pinned elsewhere:
	// TestEditorLatencyGateCannotBeSoftened rejects continue-on-error on the build
	// job and on its steps and requires the gate step's condition always();
	// TestO1BuildAggregateRequiresGateResults requires ccore-equality-cgo in
	// build.needs and runs the gate step's shell with each result.

	// The equality step runs the intended command, and nothing else.
	if got := ccoreNormalizeRun(equalityRun); got != ccoreEqualityRun {
		t.Fatalf("the equality step's run block changed:\n got %q\nwant %q", got, ccoreEqualityRun)
	}
	// Run the block with a failing runner stub in place of the Docker runner: the
	// stub must be called with the label, the wall timeout and the equality script
	// for the language, and its failure must make the block fail.
	for _, lang := range accepted {
		dir := t.TempDir()
		docker := filepath.Join(dir, "cgo_harness", "docker")
		if err := os.MkdirAll(docker, 0o755); err != nil {
			t.Fatal(err)
		}
		calls := filepath.Join(dir, "calls")
		stub := "#!/bin/bash\nprintf 'mode=%s\\n' \"$GTS_PARITY_MODE\" >> '" + calls + "'\nfor a in \"$@\"; do printf 'arg=%s\\n' \"$a\" >> '" + calls + "'; done\nexit 7\n"
		if err := os.WriteFile(filepath.Join(docker, "run_parity_in_docker.sh"), []byte(stub), 0o755); err != nil {
			t.Fatal(err)
		}
		block := strings.ReplaceAll(equalityRun, "${{ matrix.language }}", lang)
		cmd := exec.Command("bash", "-e", "-o", "pipefail", "-c", block)
		cmd.Dir = dir
		cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("%s: the equality step succeeded although the runner failed\n%s", lang, out)
		}
		raw, rerr := os.ReadFile(calls)
		if rerr != nil {
			t.Errorf("%s: the runner stub was not called: %v", lang, rerr)
			continue
		}
		got := string(raw)
		for _, want := range []string{"mode=smoke\n", "arg=--label\narg=ci-ccore-" + lang + "\n", "arg=--wall-timeout\narg=45m\n", "arg=bash /workspace/cgo_harness/docker/ccore_equality.sh " + lang + "\n"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s: the runner was not called with %q; calls:\n%s", lang, want, got)
			}
		}
	}
}

// ccoreEqualityJobIf is the condition of the equality job: it runs when the
// scope job says code CI runs.
const ccoreEqualityJobIf = "needs.exhaustive_parity_scope.outputs.run_code_ci == 'true'"

// ccoreEqualityRun is the run block of the equality step with its line
// continuations and runs of spaces collapsed.
const ccoreEqualityRun = "GTS_PARITY_MODE=smoke bash cgo_harness/docker/run_parity_in_docker.sh --label ci-ccore-${{ matrix.language }} --memory 8g --cpus 2 --pids 4096 --gomemlimit 6GiB --goflags -p=1 --c-ref-build-jobs 1 --wall-timeout 45m -- \"bash /workspace/cgo_harness/docker/ccore_equality.sh ${{ matrix.language }}\""

func ccoreNormalizeRun(run string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(run, "\\\n", " ")), " ")
}
