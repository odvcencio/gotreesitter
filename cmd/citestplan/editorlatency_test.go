package main

import (
	"encoding/json"
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
				lang := "python"
				if err := os.WriteFile(calls, nil, 0o644); err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command("/bin/bash", "-c", script)
				cmd.Env = []string{"PATH=" + dir, "CALLS=" + calls, "RUNNER_KIND=" + runner,
					"BASE_SHA=base-sha", "RUNNER_TEMP=" + dir, "CAMPAIGN_EXIT=" + status, "W5_LANGUAGE=" + lang}
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
				want := "git fetch --no-tags origin base-sha\ncampaign scripts/run_editor_latency_gate.sh --base base-sha --output " + dir + "/w5-editor-latency --languages " + lang + " " + memory + "\n"
				if string(data) != want {
					t.Fatalf("paired gate invocation = %q, want %q", data, want)
				}
			}
		})
	}
}

func TestEditorLatencyKeepsRawEvidenceLocal(t *testing.T) {
	job := readGateWorkflow(t).Jobs["editor_latency"]
	summary := false
	for _, step := range job.Steps {
		if strings.HasPrefix(step.Uses, "actions/upload-artifact@") {
			t.Fatal("editor latency job must not upload raw evidence")
		}
		if strings.Contains(step.Run, "GITHUB_STEP_SUMMARY") {
			summary = true
		}
	}
	if !summary {
		t.Fatal("editor latency job must retain its concise summary")
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
		// TypeScript's locked oracle applies its grammar patch in this
		// temporary clone, so the reference mount must remain writable.
		if !strings.Contains(line, "--mount "+dir+"/shared-skipped-gap-"+grammar+"-reference:/tmp/grammar_parity ") {
			t.Fatalf("%s requires a writable temporary oracle clone: %s", grammar, line)
		}
		for _, want := range []string{"docker/run_parity_in_docker.sh", "--memory 4g", "GOWORK=off", "-tags treesitter_c_parity", "-run '^(TestSharedSkippedGapLockedC|TestSharedSkippedGapInvalidPrefixLockedC|TestSharedSkippedGapEditSession(Profiled)?)/" + grammar + "$'"} {
			if !strings.Contains(line, want) {
				t.Errorf("%s Docker call omits %q: %s", grammar, want, line)
			}
		}
	}
}

// The ten legs are the languages of the editor-latency manifest, in its order.
func editorLatencyManifestLanguages(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("../../cgo_harness/editor_latency/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Languages    []string `json:"languages"`
		Supplemental []string `json:"supplemental_languages"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return append(m.Languages, m.Supplemental...)
}

func TestEditorLatencyMatrixCoversManifestLanguages(t *testing.T) {
	job := readGateWorkflow(t).Jobs["editor_latency"]
	got, want := job.Strategy.Matrix.Language, editorLatencyManifestLanguages(t)
	if len(want) != 10 || strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("matrix languages = %v, manifest languages = %v", got, want)
	}
	if job.Strategy.FailFast == nil || *job.Strategy.FailFast {
		t.Fatal("every language must finish: fail-fast must be false")
	}
	if job.Timeout != 45 {
		t.Fatalf("each language leg must time out at 45 minutes, got %d", job.Timeout)
	}
}

func TestEditorLatencyLegCommandAndExceptionsAreLanguageBound(t *testing.T) {
	job := readGateWorkflow(t).Jobs["editor_latency"]
	for _, step := range job.Steps {
		if !strings.Contains(step.Run, "bash scripts/run_editor_latency_gate.sh") {
			continue
		}
		if step.Env["W5_LANGUAGE"] != "${{ matrix.language }}" {
			t.Fatalf("leg must pass its matrix language: %q", step.Env["W5_LANGUAGE"])
		}
		if !strings.Contains(step.Run, `--languages "$W5_LANGUAGE"`) {
			t.Fatal("gate command must be restricted to the leg's language")
		}
		if step.Env["GTS_W5_PRIVATE_C_CACHE"] != "1" {
			t.Fatal("private C-oracle cache must stay on in every leg")
		}
		// Both owner-approved base receipts are prepared in the go leg only.
		if n := strings.Count(step.Run, `[ "$W5_LANGUAGE" = go ] && [ "${PR_NUMBER:-}" = 1512 ]`); n != 2 {
			t.Fatalf("both approved receipts must be guarded by the go leg, found %d guards", n)
		}
		return
	}
	t.Fatal("no gate step")
}

// With PR 1512 and an approved base, only the go leg passes a receipt option.
func TestEditorLatencyApprovedReceiptsOnlyInGoLeg(t *testing.T) {
	var script string
	for _, step := range readGateWorkflow(t).Jobs["editor_latency"].Steps {
		if strings.Contains(step.Run, "bash scripts/run_editor_latency_gate.sh") {
			script = step.Run
		}
	}
	for _, base := range []string{"9d14415d0a8d11632292310c578446b31554217b", "b7f1877f72c46ac2992e865ab636edad1d545060"} {
		for _, lang := range editorLatencyManifestLanguages(t) {
			dir := t.TempDir()
			for name, body := range map[string]string{
				"git":  "#!/bin/bash\nexit 0\n",
				"curl": "#!/bin/bash\nwhile [ $# -gt 0 ]; do [ \"$1\" = --output ] && : > \"$2\"; shift; done\n",
				"bash": "#!/bin/bash\nprintf '%s\\n' \"$*\" >> \"$CALLS\"\n",
			} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			calls := filepath.Join(dir, "calls")
			cmd := exec.Command("/bin/bash", "-c", script)
			cmd.Env = []string{"PATH=" + dir + ":/usr/bin:/bin", "CALLS=" + calls, "RUNNER_KIND=self-hosted", "BASE_SHA=" + base,
				"RUNNER_TEMP=" + dir, "PR_NUMBER=1512", "W5_LANGUAGE=" + lang}
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s %s: %v\n%s", lang, base, err, out)
			}
			data, _ := os.ReadFile(calls)
			got := strings.Contains(string(data), "undo-receipt")
			if got != (lang == "go") || !strings.Contains(string(data), "--languages "+lang) {
				t.Fatalf("%s base %s: receipt option=%v, call=%q", lang, base[:8], got, data)
			}
		}
	}
}
