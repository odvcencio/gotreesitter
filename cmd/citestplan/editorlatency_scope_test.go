package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The scope step decides with the base's copy of the rule, so a pull request
// cannot switch the latency job off by editing the script. The step's own
// shell runs here in a throwaway repository.
func TestEditorLatencyScopeStepRunsTheBaseRule(t *testing.T) {
	wf := readGateWorkflow(t)
	var scope gateWorkflowStep
	for _, step := range wf.Jobs["exhaustive_parity_scope"].Steps {
		if step.ID == "scope" {
			scope = step
		}
	}
	if scope.Run == "" {
		t.Fatal("scope step not found")
	}
	if strings.Contains(scope.Run, "bash .github/scripts/editor_latency_scope.sh") {
		t.Fatal("the scope step must not run the pull request's own copy of the rule")
	}
	rule, err := os.ReadFile("../../.github/scripts/editor_latency_scope.sh")
	if err != nil {
		t.Fatal(err)
	}
	const rulePath = ".github/scripts/editor_latency_scope.sh"
	for _, tc := range []struct {
		name        string
		baseHasRule bool
		change      map[string]string
		want        string
	}{
		{"only the new core changes: skip", true, map[string]string{"internal/ccore/a.go": "package ccore // 2\n"}, "false"},
		{"engine change with the rule rewritten to skip: run", true, map[string]string{"parser.go": "package p // 2\n", rulePath: "echo false\n"}, "true"},
		{"the rule rewritten to skip beside a core change: run", true, map[string]string{"internal/ccore/a.go": "package ccore // 2\n", rulePath: "echo false\n"}, "true"},
		{"the rule deleted beside a core change: run", true, map[string]string{"internal/ccore/a.go": "package ccore // 2\n", rulePath: ""}, "true"},
		{"base without the rule: run", false, map[string]string{"internal/ccore/a.go": "package ccore // 2\n"}, "true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			repo := filepath.Join(dir, "repo")
			env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"}
			git := func(args ...string) string {
				t.Helper()
				cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main"}, args...)...)
				cmd.Dir, cmd.Env = repo, env
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("git %v: %v\n%s", args, err, out)
				}
				return strings.TrimSpace(string(out))
			}
			write := func(rel, content string) {
				t.Helper()
				path := filepath.Join(repo, rel)
				if content == "" {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
					return
				}
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			write(".github/ccore-unrouted", "marker\n")
			write("parser.go", "package p\n")
			write("internal/ccore/a.go", "package ccore\n")
			if tc.baseHasRule {
				write(rulePath, string(rule))
			}
			git("init", "-q")
			git("add", "-A")
			git("commit", "-q", "-m", "base")
			base := git("rev-parse", "HEAD")
			for rel, content := range tc.change {
				write(rel, content)
			}
			git("add", "-A")
			git("commit", "-q", "-m", "head")

			output := filepath.Join(dir, "output")
			cmd := exec.Command("bash", "-c", scope.Run)
			cmd.Dir = repo
			cmd.Env = append(env, "EVENT_NAME=pull_request", "PR_BASE_SHA="+base, "RUNNER_TEMP="+dir,
				"GITHUB_OUTPUT="+output, "GITHUB_STEP_SUMMARY="+filepath.Join(dir, "summary"))
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("scope step: %v\n%s", err, out)
			}
			data, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]string{}
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				key, value, _ := strings.Cut(line, "=")
				got[key] = value
			}
			if got["run_code_ci"] != "true" || got["run_editor_latency"] != tc.want {
				summary, _ := os.ReadFile(filepath.Join(dir, "summary"))
				t.Fatalf("outputs = %v, want run_code_ci=true run_editor_latency=%s\n%s", got, tc.want, summary)
			}
		})
	}
}

// Edits that would drop a language from the gate, or let a failed language
// pass, without changing the language list: a second matrix key (exclude or
// include), continue-on-error, or a condition on the gate step.
func TestEditorLatencyGateCannotBeSoftened(t *testing.T) {
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
	latency, build := wf.Jobs["editor_latency"], wf.Jobs["build"]
	strategy, _ := latency["strategy"].(map[string]any)
	matrix, _ := strategy["matrix"].(map[string]any)
	if _, ok := matrix["language"]; !ok || len(matrix) != 1 {
		t.Fatalf("editor_latency matrix must have the single key language: %v", matrix)
	}
	for name, job := range map[string]map[string]any{"editor_latency": latency, "build": build} {
		if _, ok := job["continue-on-error"]; ok {
			t.Errorf("job %s must not set continue-on-error", name)
		}
		steps, _ := job["steps"].([]any)
		if len(steps) == 0 {
			t.Fatalf("job %s has no steps", name)
		}
		for i, raw := range steps {
			step, _ := raw.(map[string]any)
			if _, ok := step["continue-on-error"]; ok {
				t.Errorf("job %s step %d (%v) must not set continue-on-error", name, i, step["name"])
			}
			if step["name"] == "Check Required Build Gate" && step["if"] != "always()" {
				t.Errorf("the gate step must run always: if = %v", step["if"])
			}
		}
	}
}
