package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Exercise two runner services sharing HOME, as on a VM. Cache restores and
// temporary build files must land in separate directories that the runner
// cleans between jobs, rather than in HOME or the host's shared /tmp.
func TestPrepareRunnerSeparatesJobs(t *testing.T) {
	sharedHome := t.TempDir()
	var caches []string
	for _, name := range []string{"runner one", "runner two"} {
		jobTemp := filepath.Join(t.TempDir(), name)
		if err := os.MkdirAll(jobTemp, 0o755); err != nil {
			t.Fatal(err)
		}
		envFile := filepath.Join(jobTemp, "env")
		cmd := exec.Command("bash", "../../.github/scripts/prepare_runner.sh")
		cmd.Env = append(os.Environ(), "HOME="+sharedHome, "RUNNER_TEMP="+jobTemp, "GITHUB_ENV="+envFile)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("prepare runner: %v\n%s", err, output)
		}
		data, err := os.ReadFile(envFile)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range []string{"TMPDIR=" + jobTemp, "GOTMPDIR=" + jobTemp, "GOCACHE=" + jobTemp + "/go-build", "GOPATH=" + jobTemp + "/go"} {
			if !strings.Contains(string(data), entry+"\n") {
				t.Fatalf("missing %q in environment: %s", entry, data)
			}
		}
		cache := filepath.Join(jobTemp, "go-build")
		if err := os.WriteFile(filepath.Join(cache, "import.a"), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		caches = append(caches, cache)
	}
	for i, cache := range caches {
		data, err := os.ReadFile(filepath.Join(cache, "import.a"))
		if err != nil || string(data) != []string{"runner one", "runner two"}[i] {
			t.Fatalf("another job overwrote cache %s: %q, %v", cache, data, err)
		}
	}
}

func TestConfiguredRunnerWorkflowPreparation(t *testing.T) {
	for _, name := range []string{"ci", "cliff-report", "fuzz-nightly", "grammar-receipts"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("../../.github/workflows", name+".yml"))
			if err != nil {
				t.Fatal(err)
			}
			var wf struct {
				Env  map[string]string `yaml:"env"`
				Jobs map[string]struct {
					RunsOn string `yaml:"runs-on"`
					Steps  []struct {
						Uses string            `yaml:"uses"`
						Run  string            `yaml:"run"`
						If   string            `yaml:"if"`
						With map[string]string `yaml:"with"`
					} `yaml:"steps"`
				} `yaml:"jobs"`
			}
			if err := yaml.Unmarshal(data, &wf); err != nil {
				t.Fatal(err)
			}
			if wf.Env["GOWORK"] != "off" || wf.Env["GOFLAGS"] != "-p=1" || wf.Env["GOMAXPROCS"] != "2" {
				t.Fatalf("shared runner build limits missing: %v", wf.Env)
			}
			for jobName, job := range wf.Jobs {
				// Keep both the fork trust boundary and the empty-variable fallback.
				wantRoute := "${{ (github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository) && vars.GTS_RUNNER_LABELS != '' && fromJSON(vars.GTS_RUNNER_LABELS) || 'ubuntu-latest' }}"
				isolatedRoute := strings.ReplaceAll(wantRoute, "GTS_RUNNER_LABELS", "GTS_ISOLATED_RUNNER_LABELS")
				if job.RunsOn != wantRoute && job.RunsOn != isolatedRoute {
					t.Errorf("%s lost the configured runner trust boundary: %s", jobName, job.RunsOn)
				}
				if (name == "ci" && (jobName == "grammargen_stable" || jobName == "race_root_shards" || jobName == "race_root_isolated" || jobName == "race_packages" || jobName == "perf-regression" || jobName == "admission_route_equality_fuzz" || jobName == "compile" || jobName == "wasm_cross_build" || jobName == "parity-cgo")) || (name == "grammar-receipts" && jobName == "receipt") || name == "fuzz-nightly" || name == "cliff-report" {
					if job.RunsOn != isolatedRoute {
						t.Errorf("%s must retain hosted isolation without a dedicated runner pool", jobName)
					}
				}
				checkedOut, prepared := false, false
				for _, step := range job.Steps {
					if strings.HasPrefix(step.Uses, "actions/checkout@") {
						checkedOut = true
					}
					if step.Run == "bash .github/scripts/prepare_runner.sh" {
						if !checkedOut {
							t.Errorf("%s prepares before checkout", jobName)
						}
						if step.If != "runner.environment == 'self-hosted'" {
							t.Errorf("%s must preserve hosted cache paths", jobName)
						}
						prepared = true
					}
					if strings.HasPrefix(step.Uses, "actions/setup-go@") {
						if !prepared {
							t.Errorf("%s restores shared Go caches before preparation", jobName)
						}
						if step.With["cache"] != "${{ runner.environment != 'self-hosted' }}" {
							t.Errorf("%s can restore archives into another VM job's cache paths", jobName)
						}
					}
				}
				if checkedOut && !prepared {
					t.Errorf("%s has no job-owned scratch preparation", jobName)
				}
			}
			if name == "grammar-receipts" && wf.Env["GTS_GRAMMAR_RECEIPT_ALLOW_CONCURRENT"] != "1" {
				t.Error("receipt matrix must permit independently limited containers on one VM")
			}
		})
	}
}
