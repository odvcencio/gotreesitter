package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Lanes of race_packages that packages_no_race leaves out, because
// draft-correctness runs their packages in full, and the package filter each
// one must keep for that to hold.
var lanesCoveredByCorrectness = map[string]string{
	"grammars-a-c":   "/grammars($|/runtime$)",
	"grammars-d-j":   "/grammars($|/runtime$)",
	"grammars-k-p":   "/grammars($|/runtime$)",
	"grammars-q-z":   "/grammars($|/runtime$)",
	"grammars-other": "/grammars($|/runtime$)",
	"parse":          "/roottest/parse$",
	"admission":      "/roottest/admission$",
}

// A light run tests every non-root package: each race_packages lane runs in
// exactly one packages_no_race shard, or draft-correctness runs its packages.
// grammargen runs as the stable subset of grammargen_stable.
func TestPackagesNoRaceCoversEveryRaceLane(t *testing.T) {
	raw := readRawWorkflow(t)
	jobs, _ := raw["jobs"].(map[string]any)
	job, _ := jobs["packages_no_race"].(map[string]any)
	strategy, _ := job["strategy"].(map[string]any)
	matrix, _ := strategy["matrix"].(map[string]any)
	include, _ := matrix["include"].([]any)
	if len(include) < 2 || len(include) > 4 {
		t.Fatalf("packages_no_race needs 2 to 4 shards, has %d", len(include))
	}
	if strategy["fail-fast"] != false {
		t.Error("every shard must finish: fail-fast must be false")
	}
	seen := map[string]int{}
	for _, entry := range include {
		leg, _ := entry.(map[string]any)
		for _, lane := range strings.Fields(fmt.Sprint(leg["lanes"])) {
			seen[lane]++
			if lane == "grammargen" && leg["go_version_file"] != "cgo_harness/go.mod" {
				t.Errorf("the grammargen shard needs cgo_harness's toolchain floor: %v", leg["go_version_file"])
			}
		}
	}
	data, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var wf workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatal(err)
	}
	raceLanes := map[string]bool{}
	for _, lane := range wf.Jobs["race_packages"].Strategy.Matrix.Include {
		raceLanes[lane.Name] = true
		if filter, ok := lanesCoveredByCorrectness[lane.Name]; ok {
			if seen[lane.Name] != 0 {
				t.Errorf("lane %s is covered by draft-correctness and must not run in a shard too", lane.Name)
			}
			if lane.PackageFilter != filter || lane.TestFilter == "" && lane.Name != "parse" && lane.Name != "admission" {
				// The grammars lanes only split one package set by test name.
				t.Errorf("lane %s changed (filter %q, test filter %q): recheck its coverage by draft-correctness", lane.Name, lane.PackageFilter, lane.TestFilter)
			}
			continue
		}
		if seen[lane.Name] != 1 {
			t.Errorf("race lane %s runs in %d packages_no_race shards, want 1", lane.Name, seen[lane.Name])
		}
	}
	for lane := range lanesCoveredByCorrectness {
		if !raceLanes[lane] {
			t.Errorf("lanesCoveredByCorrectness names %s, which race_packages no longer has", lane)
		}
	}
	for lane, n := range seen {
		if lane != "grammargen" && !raceLanes[lane] {
			t.Errorf("shard lane %s is not a race_packages lane", lane)
		}
		if lane == "grammargen" && n != 1 {
			t.Errorf("grammargen runs in %d shards, want 1", n)
		}
	}
	if seen["grammargen"] != 1 {
		t.Error("no shard runs grammargen")
	}

	gate := readGateWorkflow(t)
	var correctness string
	for _, step := range gate.Jobs["draft-correctness"].Steps {
		if strings.Contains(step.Run, "go test ") {
			correctness = step.Run
		}
	}
	if !regexp.MustCompile(`(?m)^go test \. \./grammars \./grammars/runtime \./roottest/admission \./roottest/bench \./roottest/parse -count=1 `).MatchString(correctness) {
		t.Fatalf("draft-correctness must run ./grammars ./grammars/runtime ./roottest/admission ./roottest/parse in full:\n%s", correctness)
	}

	var stableSkip, step gateWorkflowStep
	for _, s := range gate.Jobs["grammargen_stable"].Steps {
		if strings.Contains(s.Run, "go test ./grammargen -race") {
			stableSkip = s
		}
	}
	for _, s := range gate.Jobs["packages_no_race"].Steps {
		if strings.Contains(s.Run, "go run ./cmd/citestplan -lane") {
			step = s
		}
	}
	if step.Run == "" {
		t.Fatal("packages_no_race does not take its packages from cmd/citestplan -lane")
	}
	if strings.Contains(step.Run, "-race") {
		t.Error("packages_no_race must run without the race detector")
	}
	if want := "-skip '" + step.Env["GRAMMARGEN_SKIP"] + "'"; step.Env["GRAMMARGEN_SKIP"] == "" || !strings.Contains(stableSkip.Run, want) {
		t.Errorf("GRAMMARGEN_SKIP must equal grammargen_stable's -skip list: %q", step.Env["GRAMMARGEN_SKIP"])
	}
	if step.Env["LANES"] != "${{ matrix.lanes }}" {
		t.Errorf("the step must read its lanes from the matrix: %q", step.Env["LANES"])
	}

	// Run the step with a stub go: every lane runs, and one failed lane fails the step.
	for _, failLane := range []string{"", "b"} {
		dir := t.TempDir()
		calls := filepath.Join(dir, "calls")
		stub := "#!/bin/bash\nprintf '%s\\n' \"$*\" >> \"$CALLS\"\n" +
			"if [ \"$1\" = run ]; then echo \"example.org/p/$4\"; exit 0; fi\n" +
			"if [ -n \"$FAIL_LANE\" ] && [[ \" $* \" == *\" example.org/p/$FAIL_LANE \"* ]]; then exit 1; fi\nexit 0\n"
		if err := os.WriteFile(filepath.Join(dir, "go"), []byte(stub), 0o755); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("bash", "-c", step.Run)
		cmd.Env = []string{"PATH=" + dir + ":" + os.Getenv("PATH"), "CALLS=" + calls, "FAIL_LANE=" + failLane,
			"LANES=a b grammargen c", "GRAMMARGEN_SKIP=" + step.Env["GRAMMARGEN_SKIP"]}
		out, err := cmd.CombinedOutput()
		if (err != nil) != (failLane != "") {
			t.Fatalf("fail lane %q: err=%v\n%s", failLane, err, out)
		}
		if failLane != "" && !strings.Contains(string(out), "lanes failed: b") {
			t.Errorf("the failed lane must be named: %s", out)
		}
		log, _ := os.ReadFile(calls)
		for _, want := range []string{
			"run ./cmd/citestplan -lane a\n", "test example.org/p/a -count=1 -timeout 12m -p 1\n",
			"test example.org/p/b -count=1", "test example.org/p/c -count=1",
			"test ./grammargen -count=1 -timeout 15m -skip " + step.Env["GRAMMARGEN_SKIP"] + "\n",
		} {
			if !strings.Contains(string(log), want) {
				t.Errorf("fail lane %q: missing call %q in:\n%s", failLane, want, log)
			}
		}
	}
}

// The light tier runs ccore-equality-cgo when a change touches the certified
// core: every file under internal/ccore/, ccore_bind.go and ccore_scanner.go,
// and every package of this module that those two files import.
func TestCcoreScopeCoversCoreImports(t *testing.T) {
	var scope gateWorkflowStep
	for _, step := range readGateWorkflow(t).Jobs["exhaustive_parity_scope"].Steps {
		if step.ID == "scope" {
			scope = step
		}
	}
	m := regexp.MustCompile(`(?m)^ccore_pattern='([^']+)'$`).FindStringSubmatch(scope.Run)
	if m == nil {
		t.Fatal("the scope step has no ccore_pattern")
	}
	pattern := regexp.MustCompile(m[1])
	matches := func(rel string) {
		t.Helper()
		if !pattern.MatchString(rel) {
			t.Errorf("ccore_pattern does not match %s", rel)
		}
	}
	const module = "github.com/odvcencio/gotreesitter"
	root := "../.."
	count := 0
	err := filepath.WalkDir(filepath.Join(root, "internal/ccore"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		matches(filepath.ToSlash(rel))
		count++
		return nil
	})
	if err != nil || count == 0 {
		t.Fatalf("walk internal/ccore: %d files, %v", count, err)
	}
	for _, src := range []string{"ccore_bind.go", "ccore_scanner.go"} {
		matches(src)
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, src), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range file.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if !strings.HasPrefix(path, module+"/") {
				continue
			}
			dir := strings.TrimPrefix(path, module+"/")
			entries, err := os.ReadDir(filepath.Join(root, dir))
			if err != nil {
				t.Fatalf("%s imports %s: %v", src, path, err)
			}
			matches(dir + "/new_file.go")
			for _, e := range entries {
				if !e.IsDir() {
					matches(dir + "/" + e.Name())
				}
			}
		}
	}
	for _, rel := range []string{
		"tree.go", "cursor.go", "arena.go", "external_lexer.go", "parser_cold_state.go", "query_reader.go",
		"grammars/runtime/python_scanner.go", "grammars/runtime/python_scanner_shared.go",
		"grammars/runtime/typescript_scanner_c.go", "grammars/runtime/tsx_scanner.go",
		"grammars/grammar_blobs/json.bin", "testdata/ccore_certificates.json",
		"cgo_harness/go.mod", "cgo_harness/internal/coracle/lib.c", "cgo_harness/internal/corecords/records.go",
		"cgo_harness/testdata/ccore/json/sample.json", "cgo_harness/docker/ccore_equality.sh",
		"scripts/graduation_gate.sh", ".github/workflows/ci.yml",
	} {
		matches(rel)
	}
	for _, rel := range []string{"parser.go", "docs/ci-gate-coverage.md", "grammars/runtime/go_scanner.go", "internal/incr/reuse.go", ".github/workflows/release.yml", "ccore_bind.go.orig/x"} {
		if pattern.MatchString(rel) {
			t.Errorf("ccore_pattern matches %s, which the core does not use", rel)
		}
	}
}
