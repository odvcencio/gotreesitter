package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Run the workflow's actual shell with campaign commands replaced by recorders.
// A fork must run the paired gate on its head, rather than fail or skip it.
func TestEditorLatencyHostedForkRunsPairedGate(t *testing.T) {
	wf := readGateWorkflow(t)
	job := wf.Jobs["editor_latency"]
	wantRoute := "${{ (github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository) && vars.GTS_RUNNER_LABELS != '' && fromJSON(vars.GTS_RUNNER_LABELS) || 'ubuntu-latest' }}"
	if job.RunsOn != wantRoute {
		t.Fatalf("forks must retain the hosted runner trust boundary: %s", job.RunsOn)
	}
	var script string
	checkoutHead, pythonSuite := false, false
	for _, step := range job.Steps {
		if step.With["ref"] == "${{ github.event.pull_request.head.sha || github.sha }}" {
			checkoutHead = true
		}
		if step.Run == "python3 scripts/test_editor_latency_gate.py" {
			pythonSuite = true
		}
		if strings.Contains(step.Run, "bash scripts/run_editor_latency_gate.sh") {
			if step.Env["RUNNER_KIND"] != "${{ runner.environment }}" || step.Env["BASE_SHA"] != "${{ github.event.pull_request.base.sha }}" {
				t.Fatal("campaign must bind the actual runner environment and PR base")
			}
			script = step.Run
		}
	}
	if !checkoutHead || !pythonSuite || script == "" || !containsGate(wf.Jobs["build"].Needs, "editor_latency") {
		t.Fatal("required latency job must check out the head and run Python tests and the paired campaign")
	}
	for _, runner := range []string{"github-hosted", "self-hosted"} {
		t.Run(runner, func(t *testing.T) {
			dir := t.TempDir()
			for name, body := range map[string]string{
				"git":  "#!/bin/bash\nprintf 'git %s\\n' \"$*\" >> \"$CALLS\"\n",
				"bash": "#!/bin/bash\nprintf 'campaign %s memory=%s gomemlimit=%s\\n' \"$*\" \"${GTS_EDITOR_LATENCY_MEMORY_LIMIT:-8g}\" \"${GOMEMLIMIT:-6GiB}\" >> \"$CALLS\"\nexit \"${CAMPAIGN_EXIT:-0}\"\n",
			} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			calls := filepath.Join(dir, "calls")
			for _, status := range []string{"0", "1"} {
				if err := os.WriteFile(calls, nil, 0o644); err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command("/bin/bash", "-c", script)
				cmd.Env = []string{"PATH=" + dir, "CALLS=" + calls, "RUNNER_KIND=" + runner,
					"BASE_SHA=base-sha", "RUNNER_TEMP=" + dir, "CAMPAIGN_EXIT=" + status}
				out, err := cmd.CombinedOutput()
				if (err == nil) != (status == "0") {
					t.Fatalf("campaign result must reach job: status=%s err=%v output=%s", status, err, out)
				}
				data, err := os.ReadFile(calls)
				if err != nil {
					t.Fatal(err)
				}
				memory := "memory=8g gomemlimit=6GiB"
				if runner == "github-hosted" {
					memory = "memory=6g gomemlimit=4GiB"
				}
				want := "git fetch --no-tags origin base-sha\ncampaign scripts/run_editor_latency_gate.sh --base base-sha --output " + dir + "/w5-editor-latency " + memory + "\n"
				if string(data) != want {
					t.Fatalf("paired gate invocation = %q, want %q", data, want)
				}
			}
		})
	}
}

func TestSharedSkippedGapRequiredDockerSelectors(t *testing.T) {
	wf := readGateWorkflow(t)
	if !containsGate(wf.Jobs["build"].Needs, "parity-cgo") {
		t.Fatal("shared recovery must stay in required parity-cgo")
	}
	var script string
	for _, step := range wf.Jobs["parity-cgo"].Steps {
		if strings.Contains(step.Run, "TestSharedSkippedGapLockedC") {
			script = step.Run
		}
	}
	if script == "" {
		t.Fatal("shared skipped-gap tests have no required Docker invocation")
	}
	dir := t.TempDir()
	stub := "#!/bin/bash\nprintf '%s\\n' \"$*\" >> \"$CALLS\"\n"
	if err := os.WriteFile(filepath.Join(dir, "bash"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	calls := filepath.Join(dir, "calls")
	cmd := exec.Command("/bin/bash", "-e", "-c", script)
	cmd.Env = []string{"PATH=" + dir, "CALLS=" + calls, "RUNNER_TEMP=" + dir}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Docker selectors: %v\n%s", err, out)
	}
	data, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected a seed and Docker call for each grammar: %s", data)
	}
	for i, grammar := range []string{"javascript", "typescript"} {
		if !strings.Contains(lines[2*i], "seed_parity_repos.sh") || !strings.HasSuffix(lines[2*i], "--langs "+grammar) {
			t.Fatalf("%s requires its own seeded grammar: %s", grammar, lines[2*i])
		}
		line := lines[2*i+1]
		for _, want := range []string{"docker/run_parity_in_docker.sh", "--memory 4g", "GOWORK=off", "-tags treesitter_c_parity", "-run '^(TestSharedSkippedGapLockedC|TestSharedSkippedGapInvalidPrefixLockedC|TestSharedSkippedGapEditSession)/" + grammar + "$'"} {
			if !strings.Contains(line, want) {
				t.Errorf("%s Docker call omits %q: %s", grammar, want, line)
			}
		}
	}
}
