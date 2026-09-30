package main

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The D-D3 gate must execute tests, rather than silently reverting to builds.
func TestWeeklyPlatformExecutionCoverage(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/platform-weekly.yml")
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		On struct {
			Schedule []struct{ Cron string } `yaml:"schedule"`
		} `yaml:"on"`
		Env  map[string]string `yaml:"env"`
		Jobs map[string]struct {
			Strategy struct {
				FailFast bool `yaml:"fail-fast"`
				Matrix   struct {
					Include []struct {
						Target, Runner, GOOS, GOARCH string
					} `yaml:"include"`
				} `yaml:"matrix"`
			} `yaml:"strategy"`
			Steps []struct {
				Name, Run, Uses, If string
				With                map[string]string
			}
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatal(err)
	}
	if len(wf.On.Schedule) != 1 || len(strings.Fields(wf.On.Schedule[0].Cron)) != 5 {
		t.Fatal("missing weekly schedule")
	}
	// A weekly schedule chooses one weekday, with every month and day of month.
	fields := strings.Fields(wf.On.Schedule[0].Cron)
	if fields[2] != "*" || fields[3] != "*" || strings.ContainsAny(fields[4], "*,/-") {
		t.Fatalf("schedule is not weekly: %q", wf.On.Schedule[0].Cron)
	}
	if wf.Env["GOWORK"] != "off" || wf.Env["CGO_ENABLED"] != "0" {
		t.Fatal("platform tests must use the standalone pure Go runtime")
	}
	job, ok := wf.Jobs["runtime"]
	if !ok || job.Strategy.FailFast {
		t.Fatal("runtime matrix must report every target even when another fails")
	}
	want := map[string]string{
		"linux-amd64": "linux/amd64", "linux-arm64": "linux/arm64",
		"darwin-arm64": "darwin/arm64", "windows-amd64": "windows/amd64",
		"windows-arm64": "windows/arm64", "wasip1-wasm": "wasip1/wasm",
	}
	for _, target := range job.Strategy.Matrix.Include {
		if got := target.GOOS + "/" + target.GOARCH; want[target.Target] != got || target.Runner == "" {
			t.Fatalf("unexpected or duplicate matrix target: %+v", target)
		}
		delete(want, target.Target)
	}
	if len(want) != 0 {
		t.Fatalf("missing targets: %v", want)
	}
	var executes, summary, artifact, wasi bool
	for _, step := range job.Steps {
		executes = executes || (strings.Contains(step.Run, "go test ./roottest/parse") &&
			strings.Contains(step.Run, "-run '^TestPlatformRuntime$'") &&
			strings.Contains(step.Run, "--- PASS: TestPlatformRuntime ") &&
			strings.Contains(step.Run, "executing on $GOOS/$GOARCH "))
		wasi = wasi || strings.Contains(step.Run, "go_wasip1_wasm_exec")
		summary = summary || (step.If == "always()" && strings.Contains(step.Run, "GITHUB_STEP_SUMMARY"))
		artifact = artifact || (step.If == "always()" && strings.HasPrefix(step.Uses, "actions/upload-artifact@") && step.With["path"] == "platform-results/")
	}
	if !executes || !wasi || !summary || !artifact {
		t.Fatalf("execution/publication coverage: tests=%v WASI=%v summary=%v artifacts=%v", executes, wasi, summary, artifact)
	}
}
