package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// CI has two tiers. The light tier runs for every pull request and push; the
// heavy tier adds the race, cgo, timing and wasm jobs and runs nightly, for a
// manual run, and for a pull request labeled ci:full.

const heavyTerm = "needs.exhaustive_parity_scope.outputs.tier == 'heavy'"
const lightTerm = "needs.exhaustive_parity_scope.outputs.tier == 'light'"

// exhaustiveJobIf: parity-cgo-exhaustive runs in both tiers when the scope
// sets run_exhaustive (path-scoped on a PR).
const exhaustiveJobIf = "needs.exhaustive_parity_scope.outputs.run_code_ci == 'true' && needs.exhaustive_parity_scope.outputs.run_exhaustive == 'true'"

// ciJobTiers lists every job id in ci.yml with its tier. A job added without a
// line here fails TestEveryCIJobHasATier.
//
//	light      runs in both tiers (its other conditions unchanged)
//	light-only runs only in the light tier (a heavy race job covers it)
//	heavy      runs only in the heavy tier
//	scoped     heavy tier always, light tier when the change touches the core
//	exhaustive both tiers, when the scope sets run_exhaustive
//	control    the scope job, the build gate, and the nightly failure issue
var ciJobTiers = map[string]string{
	"compile":                       "light",
	"selected_store_admission":      "light",
	"compact_admission_ratchet":     "light",
	"phase0_tagged_suite":           "light",
	"admission_route_equality_fuzz": "light",
	"draft_focused_tests":           "light",
	"draft-correctness":             "light",
	"parity_report":                 "light",
	"perf_scan_budget":              "light",
	"glr_gss_demotion_scaling_gate": "light",
	"counter_ledger":                "light",
	"freshness":                     "light",
	"apidiff":                       "light",
	"pr_commit_buildability":        "light",
	"parity-cgo":                    "light",

	"packages_no_race": "light-only",

	"race_root_shards":                   "heavy",
	"root_race_plan":                     "heavy",
	"race_root_isolated":                 "heavy",
	"race_root":                          "heavy",
	"race_packages":                      "heavy",
	"grammargen_stable":                  "heavy",
	"admission_route_performance_sanity": "heavy",
	"oedit_latency_gate_full":            "heavy",
	"editor_latency":                     "heavy",
	"v1_edit_sessions":                   "heavy",
	"wasm_cross_build":                   "heavy",
	"wasm_tinygo_build":                  "heavy",
	"compact-t3-oracle-cgo":              "heavy",
	"merge-event-census-cgo":             "heavy",
	"grammar-lock-stamp-receipts-cgo":    "heavy",
	"ts2go_blob_freshness":               "heavy",
	"perf-regression":                    "heavy",

	"ccore-equality-cgo":    "scoped",
	"parity-cgo-exhaustive": "exhaustive",

	"exhaustive_parity_scope": "control",
	"build":                   "control",
	"nightly_failure_issue":   "control",
}

// Jobs outside build.needs: the race sub-jobs (race_root aggregates them),
// the advisory or manual-only jobs, and the control jobs that build cannot need.
var notInBuildNeeds = map[string]bool{
	"race_root_shards": true, "root_race_plan": true, "race_root_isolated": true,
	"oedit_latency_gate_full": true, "perf-regression": true, "build": true,
	"nightly_failure_issue": true,
}

// The nightly failure issue: one job after build, on the schedule only, that
// reports and cannot change the run's verdict.
func TestNightlyFailureIssueJob(t *testing.T) {
	wf := readGateWorkflow(t)
	job, ok := wf.Jobs["nightly_failure_issue"]
	if !ok {
		t.Fatal("job nightly_failure_issue is missing")
	}
	if job.Needs.Value != "build" {
		t.Errorf("nightly_failure_issue must need build alone: %v", job.Needs.Value)
	}
	wantIf := "${{ !cancelled() && github.event_name == 'schedule' && (needs.build.result == 'success' || needs.build.result == 'failure') }}"
	if job.If != wantIf {
		t.Errorf("nightly_failure_issue condition:\n got %s\nwant %s", job.If, wantIf)
	}
	if job.Permissions["issues"] != "write" || job.Permissions["actions"] != "read" || len(job.Permissions) != 3 {
		t.Errorf("nightly_failure_issue permissions: %v", job.Permissions)
	}
	if containsGate(wf.Jobs["build"].Needs, "nightly_failure_issue") {
		t.Error("build must not wait for the issue reporter")
	}
	found := false
	for _, step := range job.Steps {
		if strings.Contains(step.Run, "python3 .github/scripts/ci_nightly_issue.py") {
			found = true
			if step.Env["BUILD_RESULT"] != "${{ needs.build.result }}" || !strings.Contains(step.Run, `--build-result "$BUILD_RESULT"`) {
				t.Errorf("the issue step must pass the build result: %+v", step)
			}
		}
	}
	if !found {
		t.Fatal("nightly_failure_issue does not run ci_nightly_issue.py")
	}
	// The script's dry run: a failure opens the issue, a pass closes it.
	dir := t.TempDir()
	jobs := filepath.Join(dir, "jobs.json")
	open := filepath.Join(dir, "open.json")
	none := filepath.Join(dir, "none.json")
	for path, body := range map[string]string{
		jobs: `{"jobs":[{"name":"build","conclusion":"failure"},{"name":"race_root","conclusion":"failure"},{"name":"compile","conclusion":"success"}]}`,
		open: `[{"number":7,"title":"ci-nightly: heavy tier failed on main"}]`,
		none: `[{"number":8,"title":"ci-nightly: heavy tier failed on main (old)"}]`,
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ result, issues, want string }{
		{"failure", none, "DRY gh issue create --repo o/r --title ci-nightly: heavy tier failed on main"},
		{"failure", open, "DRY gh issue edit 7"},
		{"success", open, "DRY gh issue close 7"},
		{"success", none, "no open issue"},
		{"cancelled", open, "no change"},
	} {
		cmd := exec.Command("python3", "-I", "../../.github/scripts/ci_nightly_issue.py", "--repo", "o/r", "--run-id", "9",
			"--attempt", "1", "--sha", "abc", "--run-url", "https://example.invalid/runs/9", "--dry-run",
			"--jobs-json", jobs, "--issues-json", tc.issues, "--build-result", tc.result)
		out, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(out), tc.want) {
			t.Errorf("build=%s issues=%s: want %q: err=%v\n%s", tc.result, filepath.Base(tc.issues), tc.want, err, out)
		}
		if tc.result == "failure" && !strings.Contains(string(out), "build failed (2 jobs)") {
			t.Errorf("failed job count: %s", out)
		}
	}
}

func readRawWorkflow(t *testing.T) map[string]any {
	t.Helper()
	path := os.Getenv("GTS_TEST_GATE_WORKFLOW")
	if path == "" {
		path = "../../.github/workflows/ci.yml"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var wf map[string]any
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatal(err)
	}
	return wf
}

// The tier is decided once: the scope step's TIER expression. The run name
// shows the same expression, so a heavy run can be found by name.
func TestTierExpressionIsSharedByRunNameAndScope(t *testing.T) {
	raw := readRawWorkflow(t)
	runName, _ := raw["run-name"].(string)
	var scope gateWorkflowStep
	for _, step := range readGateWorkflow(t).Jobs["exhaustive_parity_scope"].Steps {
		if step.ID == "scope" {
			scope = step
		}
	}
	expr := scope.Env["TIER"]
	if !strings.HasPrefix(expr, "${{ ") || !strings.HasSuffix(expr, " }}") {
		t.Fatalf("scope step must take TIER from one expression: %q", expr)
	}
	if !strings.HasPrefix(runName, "ci [tier: "+expr+"] ") {
		t.Fatalf("run-name must start with \"ci [tier: <scope TIER expression>] \":\n run-name %q\n TIER     %q", runName, expr)
	}
	for _, want := range []string{
		"github.event_name == 'schedule'",
		"github.event_name == 'workflow_dispatch' && inputs.tier != 'light'",
		"github.event_name == 'pull_request' && contains(github.event.pull_request.labels.*.name, 'ci:full')",
		"&& 'heavy' || 'light'",
	} {
		if !strings.Contains(expr, want) {
			t.Errorf("tier expression omits %q: %s", want, expr)
		}
	}
	if strings.Contains(scope.Run, "gh api") || strings.Contains(scope.Run, "gh pr") {
		t.Error("the scope step must read the label from the event payload, not from an API call")
	}
	on, _ := raw["on"].(map[string]any)
	pr, _ := on["pull_request"].(map[string]any)
	types, _ := pr["types"].([]any)
	labeled := false
	for _, ty := range types {
		labeled = labeled || ty == "labeled"
	}
	if !labeled {
		t.Error("pull_request types must include labeled, so adding ci:full starts a heavy run")
	}
	dispatch, _ := on["workflow_dispatch"].(map[string]any)
	inputs, _ := dispatch["inputs"].(map[string]any)
	tier, _ := inputs["tier"].(map[string]any)
	if tier["default"] != "heavy" {
		t.Errorf("a manual run defaults to the heavy tier: %v", tier)
	}
}

func TestEveryCIJobHasATier(t *testing.T) {
	wf := readGateWorkflow(t)
	var got, want []string
	for name := range wf.Jobs {
		got = append(got, name)
	}
	for name := range ciJobTiers {
		want = append(want, name)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("ci.yml jobs and the tier table differ:\n ci.yml %v\n table  %v", got, want)
	}
	gate := buildGateStep(t, wf)
	for name, tier := range ciJobTiers {
		cond := wf.Jobs[name].If
		switch tier {
		case "light":
			if strings.Contains(cond, "outputs.tier") || strings.Contains(cond, "draft") {
				t.Errorf("light job %s must not test the tier or a draft: %s", name, cond)
			}
		case "light-only":
			if !strings.Contains(cond, "&& "+lightTerm) {
				t.Errorf("light-only job %s must require %s: %s", name, lightTerm, cond)
			}
		case "heavy":
			if !strings.Contains(cond, "&& "+heavyTerm) {
				t.Errorf("heavy job %s must require %s: %s", name, heavyTerm, cond)
			}
		case "scoped":
			if cond != ccoreEqualityJobIf {
				t.Errorf("scoped job %s condition: %s", name, cond)
			}
		case "exhaustive":
			if cond != exhaustiveJobIf {
				t.Errorf("exhaustive job %s condition: %s", name, cond)
			}
		}
		if tier == "control" || notInBuildNeeds[name] {
			continue
		}
		if !containsGate(wf.Jobs["build"].Needs, name) {
			t.Errorf("build.needs omits %s", name)
		}
		bound := false
		for _, value := range gate.Env {
			bound = bound || value == "${{ needs."+name+".result }}"
		}
		if !bound {
			t.Errorf("the build gate does not bind %s.result", name)
		}
	}
}

// The gate in both tiers: a light run requires the light jobs and requires the
// heavy jobs to be skipped; a heavy run requires both.
func TestBuildGateTiers(t *testing.T) {
	wf := readGateWorkflow(t)
	step := buildGateStep(t, wf)
	keyOf := map[string]string{}
	for key, value := range step.Env {
		if strings.HasPrefix(value, "${{ needs.") && strings.HasSuffix(value, ".result }}") {
			keyOf[strings.TrimSuffix(strings.TrimPrefix(value, "${{ needs."), ".result }}")] = key
		}
	}
	heavyOnly := []string{}
	for name, tier := range ciJobTiers {
		if tier == "heavy" && !notInBuildNeeds[name] {
			heavyOnly = append(heavyOnly, name)
		}
	}
	sort.Strings(heavyOnly)
	type env map[string]string
	run := func(tier, event string, results env, extra env) ([]byte, error) {
		e := env{
			"IS_PULL_REQUEST": "false", "IS_MANUAL_RUN": "false", "RUN_CODE_CI": "true",
			"EXHAUSTIVE_PARITY_SCOPE": "false", "RUN_EDITOR_LATENCY": "true", "TIER": tier,
			"RUN_CCORE_EQUALITY": "false", "EDITOR_LATENCY_LEGS_RESULT": "",
		}
		if event == "pull_request" {
			e["IS_PULL_REQUEST"] = "true"
		}
		if event == "workflow_dispatch" {
			e["IS_MANUAL_RUN"] = "true"
		}
		for job, key := range keyOf {
			e[key] = "success"
			switch ciJobTiers[job] {
			case "heavy", "scoped":
				if tier == "light" {
					e[key] = "skipped"
				}
			case "light-only":
				if tier == "heavy" {
					e[key] = "skipped"
				}
			case "exhaustive":
				e[key] = "skipped"
			}
		}
		if tier == "heavy" {
			e["EDITOR_LATENCY_LEGS_RESULT"] = "success"
		}
		for job, result := range results {
			e[keyOf[job]] = result
		}
		for k, v := range extra {
			e[k] = v
		}
		cmd := exec.Command("bash", "-c", step.Run)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
		for k, v := range e {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
		return cmd.CombinedOutput()
	}
	check := func(name string, pass bool, msg string, out []byte, err error) {
		t.Helper()
		if pass && err != nil {
			t.Errorf("%s must pass: %v\n%s", name, err, out)
		}
		if !pass && (err == nil || !strings.Contains(string(out), msg)) {
			t.Errorf("%s must fail with %q: err=%v\n%s", name, msg, err, out)
		}
	}
	for _, event := range []string{"pull_request", "push", "workflow_dispatch"} {
		out, err := run("light", event, nil, nil)
		check("light "+event, true, "", out, err)
		out, err = run("heavy", event, nil, nil)
		check("heavy "+event, true, "", out, err)
	}
	for _, job := range heavyOnly {
		// v1_edit_sessions and editor_latency are read only for the events
		// that run them.
		event := "pull_request"
		if job == "v1_edit_sessions" {
			event = "workflow_dispatch"
		}
		out, err := run("light", event, env{job: "success"}, nil)
		check("light: "+job+" ran", false, job+" finished with result=success but tier=light requires skipped", out, err)
		out, err = run("light", event, env{job: "failure"}, nil)
		check("light: "+job+" failed", false, job+" finished with result=failure but tier=light requires skipped", out, err)
		out, err = run("heavy", event, env{job: "skipped"}, nil)
		check("heavy: "+job+" skipped", false, job+" finished with result=skipped", out, err)
	}
	// The light-only package tests: required in a light run, skipped in a heavy one.
	for _, result := range []string{"skipped", "failure", "cancelled"} {
		out, err := run("light", "pull_request", env{"packages_no_race": result}, nil)
		check("light: packages_no_race "+result, false, "packages_no_race finished with result="+result, out, err)
	}
	out, err := run("heavy", "pull_request", env{"packages_no_race": "success"}, nil)
	check("heavy: packages_no_race ran", false, "packages_no_race finished with result=success but tier=heavy requires skipped", out, err)
	// The exhaustive sweep follows run_exhaustive in both tiers.
	for _, tier := range []string{"light", "heavy"} {
		out, err = run(tier, "pull_request", env{"parity-cgo-exhaustive": "success"}, env{"EXHAUSTIVE_PARITY_SCOPE": "true"})
		check(tier+" exhaustive run", true, "", out, err)
		for _, result := range []string{"skipped", "failure", "cancelled"} {
			out, err = run(tier, "pull_request", env{"parity-cgo-exhaustive": result}, env{"EXHAUSTIVE_PARITY_SCOPE": "true"})
			check(tier+" exhaustive "+result, false, "parity-cgo-exhaustive finished with result="+result, out, err)
		}
	}
	for _, value := range []string{"", "maybe"} {
		out, err = run("light", "pull_request", nil, env{"EXHAUSTIVE_PARITY_SCOPE": value})
		check("run_exhaustive "+value, false, "emitted invalid run_exhaustive="+value, out, err)
	}
	for name, tier := range ciJobTiers {
		if tier != "light" || notInBuildNeeds[name] {
			continue
		}
		event := "push"
		if name == "pr_commit_buildability" {
			event = "pull_request"
		}
		for _, result := range []string{"skipped", "failure", "cancelled"} {
			out, err := run("light", event, env{name: result}, nil)
			check("light: "+name+" "+result, false, name+" finished with result="+result, out, err)
		}
	}
	// The certified-core comparison in the light tier follows the scope output.
	out, err = run("light", "pull_request", env{"ccore-equality-cgo": "success"}, env{"RUN_CCORE_EQUALITY": "true"})
	check("light ccore run", true, "", out, err)
	out, err = run("light", "pull_request", env{"ccore-equality-cgo": "skipped"}, env{"RUN_CCORE_EQUALITY": "true"})
	check("light ccore skipped", false, "ccore-equality-cgo finished with result=skipped", out, err)
	out, err = run("light", "pull_request", env{"ccore-equality-cgo": "success"}, nil)
	check("light ccore out of scope", false, "ccore-equality-cgo finished with result=success but tier=light requires skipped", out, err)
	// A heavy run requires it even when the scope says the core is untouched.
	out, err = run("heavy", "pull_request", env{"ccore-equality-cgo": "skipped"}, nil)
	check("heavy ccore skipped", false, "ccore-equality-cgo finished with result=skipped", out, err)
	// Invalid scope outputs block the build.
	for _, tier := range []string{"", "medium"} {
		out, err = run(tier, "pull_request", nil, nil)
		check("tier "+tier, false, "emitted invalid tier="+tier, out, err)
	}
	out, err = run("light", "pull_request", nil, env{"RUN_CCORE_EQUALITY": ""})
	check("ccore scope missing", false, "emitted invalid run_ccore_equality=", out, err)
	// The docs-only route does not read the tier.
	out, err = run("", "pull_request", nil, env{"RUN_CODE_CI": "false"})
	check("docs-only", true, "", out, err)
}

// A push scopes the light-tier core comparison by its own range; an unknown
// range runs it, and a heavy run always runs it.
func TestScopeStepTierAndPushCcoreScope(t *testing.T) {
	var scope gateWorkflowStep
	for _, step := range readGateWorkflow(t).Jobs["exhaustive_parity_scope"].Steps {
		if step.ID == "scope" {
			scope = step
		}
	}
	if scope.Run == "" || scope.Env["PUSH_BEFORE"] != "${{ github.event.before }}" {
		t.Fatalf("scope step must bind the push range: %+v", scope.Env)
	}
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
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("parser.go", "package p\n")
	write("internal/ccore/a.go", "package ccore\n")
	git("init", "-q")
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	base := git("rev-parse", "HEAD")
	write("parser.go", "package p // 2\n")
	git("commit", "-qam", "engine")
	engine := git("rev-parse", "HEAD")
	write("internal/ccore/a.go", "package ccore // 2\n")
	git("commit", "-qam", "core")

	run := func(event, tier, before string) (map[string]string, []byte, error) {
		output := filepath.Join(dir, "output")
		_ = os.Remove(output)
		cmd := exec.Command("bash", "-c", scope.Run)
		cmd.Dir = repo
		cmd.Env = append(env, "EVENT_NAME="+event, "PR_BASE_SHA=", "PUSH_BEFORE="+before, "TIER="+tier,
			"RUNNER_TEMP="+dir, "GITHUB_OUTPUT="+output, "GITHUB_STEP_SUMMARY="+filepath.Join(dir, "summary"))
		out, err := cmd.CombinedOutput()
		got := map[string]string{}
		data, _ := os.ReadFile(output)
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			key, value, _ := strings.Cut(line, "=")
			got[key] = value
		}
		return got, out, err
	}
	for _, tc := range []struct {
		name, event, tier, before, ccore, exhaustive string
	}{
		// HEAD is the core commit: both ranges below include the core change.
		{"push of the core commit", "push", "light", engine, "true", "false"},
		{"push of both commits", "push", "light", base, "true", "false"},
		{"push without a range", "push", "light", "", "true", "false"},
		{"new branch push", "push", "light", "0000000000000000000000000000000000000000", "true", "false"},
		{"unknown before", "push", "light", "1111111111111111111111111111111111111111", "true", "false"},
		{"heavy push", "push", "heavy", engine, "true", "false"},
		{"nightly", "schedule", "heavy", "", "true", "true"},
		{"light manual run", "workflow_dispatch", "light", "", "true", "true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, out, err := run(tc.event, tc.tier, tc.before)
			if err != nil {
				t.Fatalf("scope step: %v\n%s", err, out)
			}
			if got["tier"] != tc.tier || got["run_ccore_equality"] != tc.ccore || got["run_exhaustive"] != tc.exhaustive || got["run_code_ci"] != "true" {
				t.Fatalf("outputs = %v, want tier=%s run_ccore_equality=%s run_exhaustive=%s", got, tc.tier, tc.ccore, tc.exhaustive)
			}
		})
	}
	// A push whose range leaves the core alone skips the comparison.
	git("checkout", "-q", "-b", "side", engine)
	if got, out, err := run("push", "light", base); err != nil || got["run_ccore_equality"] != "false" {
		t.Fatalf("push base..engine: outputs=%v err=%v\n%s", got, err, out)
	}
	for _, tier := range []string{"", "medium"} {
		if _, out, err := run("push", tier, base); err == nil || !strings.Contains(string(out), "invalid tier="+tier) {
			t.Errorf("tier %q must fail the scope step: err=%v\n%s", tier, err, out)
		}
	}
}
