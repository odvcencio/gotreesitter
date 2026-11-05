package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGrammarReceiptNightlyWorkflow(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/grammar-receipts.yml")
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		On struct {
			Schedule []struct {
				Cron string `yaml:"cron"`
			} `yaml:"schedule"`
			Release struct {
				Types []string `yaml:"types"`
			} `yaml:"release"`
		} `yaml:"on"`
		Jobs map[string]struct {
			Steps []struct {
				Name string            `yaml:"name"`
				Run  string            `yaml:"run"`
				If   string            `yaml:"if"`
				Env  map[string]string `yaml:"env"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatal(err)
	}
	if len(wf.On.Schedule) != 1 {
		t.Fatalf("want one nightly schedule, got %v", wf.On.Schedule)
	}
	cron := strings.Fields(wf.On.Schedule[0].Cron)
	if len(cron) != 5 || cron[2] != "*" || cron[3] != "*" || cron[4] != "*" {
		t.Fatalf("receipt schedule must run every day: %v", cron)
	}
	if len(wf.On.Release.Types) != 1 || wf.On.Release.Types[0] != "published" {
		t.Fatalf("receipt workflow must run for every published release: %v", wf.On.Release.Types)
	}
	checkedCoverage, checkedUpload := false, false
	for _, step := range wf.Jobs["summary"].Steps {
		if step.Name == "Summarize" {
			checkedCoverage = step.Env["EXPECTED_MATRIX"] == "${{ needs.plan.outputs.matrix }}" &&
				strings.Contains(step.Run, `--expected-matrix "$EXPECTED_MATRIX"`) &&
				strings.Contains(step.Run, `exit "$status"`)
		}
		if step.Name == "Upload the summary" {
			checkedUpload = step.If == "always()"
		}
	}
	if !checkedCoverage || !checkedUpload {
		t.Fatal("summary must enforce planned coverage and publish failed-run evidence")
	}
}

func TestGrammarReceiptPlanningAndCoverage(t *testing.T) {
	cmd := exec.Command("python3", "../../scripts/grammar_receipts_test.py")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("receipt planning and coverage: %v\n%s", err, output)
	}
}
