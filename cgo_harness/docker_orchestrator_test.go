//go:build docker_parity

package cgoharness

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// dockerParityRunner wraps Docker CLI calls via os/exec for running parity
// tests inside the cgo-harness container. No testcontainers dependency — only
// stdlib os/exec.
type dockerParityRunner struct {
	imageOnce sync.Once
	imageTag  string
	imageErr  error
}

var defaultRunner = &dockerParityRunner{}

const defaultImageTag = "gotreesitter/cgo-harness:go1.25-local"

// buildImage runs `docker build` once per test binary invocation and returns
// the image tag. Subsequent calls return the cached result.
func (r *dockerParityRunner) buildImage(t *testing.T) string {
	t.Helper()
	r.imageOnce.Do(func() {
		repoRoot := detectRepoRoot(t)
		dockerfilePath := filepath.Join(repoRoot, "cgo_harness", "docker", "Dockerfile")

		if _, err := os.Stat(dockerfilePath); err != nil {
			r.imageErr = fmt.Errorf("Dockerfile not found at %s: %w", dockerfilePath, err)
			return
		}

		cmd := exec.Command("docker", "build",
			"-t", defaultImageTag,
			"-f", dockerfilePath,
			repoRoot,
		)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		cmd.Stdout = os.Stderr // stream build output for visibility

		t.Logf("building Docker image %s ...", defaultImageTag)
		if err := cmd.Run(); err != nil {
			r.imageErr = fmt.Errorf("docker build failed: %w\nstderr: %s", err, stderr.String())
			return
		}
		r.imageTag = defaultImageTag
	})
	if r.imageErr != nil {
		t.Fatalf("docker image build: %v", r.imageErr)
	}
	return r.imageTag
}

// containerOpts configures a single container run.
type containerOpts struct {
	RepoRoot string            // default: auto-detect via filepath.Abs("..")
	RunRegex string            // -run pattern passed to go test
	EnvVars  map[string]string // extra env vars to pass into the container
	Memory   string            // default "8g"
	CPUs     string            // default "4"
	Timeout  time.Duration     // default 600s
}

func (o *containerOpts) applyDefaults(t *testing.T) {
	t.Helper()
	if o.RepoRoot == "" {
		o.RepoRoot = detectRepoRoot(t)
	}
	if o.Memory == "" {
		o.Memory = "8g"
	}
	if o.CPUs == "" {
		o.CPUs = "4"
	}
	if o.Timeout == 0 {
		o.Timeout = 600 * time.Second
	}
}

// parityResult captures the outcome of a containerized test run.
type parityResult struct {
	ExitCode  int
	Events    []test2jsonEvent
	OOMKilled bool
	Duration  time.Duration
}

// test2jsonEvent matches the JSON objects emitted by `go test -json`.
type test2jsonEvent struct {
	Time    string  `json:"Time"`
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Output  string  `json:"Output"`
	Elapsed float64 `json:"Elapsed"`
}

// parseTest2JSON parses newline-delimited go test -json output into events.
func parseTest2JSON(output []byte) []test2jsonEvent {
	var events []test2jsonEvent
	for _, line := range bytes.Split(output, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var ev test2jsonEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			// Non-JSON lines (e.g. build output) are silently skipped.
			continue
		}
		events = append(events, ev)
	}
	return events
}

// runTest creates a container, runs the parity test suite inside it, captures
// go test -json output, and returns structured results. The container is
// removed via t.Cleanup.
func (r *dockerParityRunner) runTest(t *testing.T, opts containerOpts) *parityResult {
	t.Helper()
	opts.applyDefaults(t)

	image := r.buildImage(t)

	// Unique container name: test name (sanitized) + timestamp.
	sanitized := strings.NewReplacer("/", "-", " ", "_").Replace(t.Name())
	containerName := fmt.Sprintf("parity-%s-%d", sanitized, time.Now().UnixNano())

	// Build docker run args.
	args := []string{
		"run",
		"--name", containerName,
		"--init",
		"--memory", opts.Memory,
		"--cpus", opts.CPUs,
		"--pids-limit", "4096",
		// Bind mount repo root.
		"-v", opts.RepoRoot + ":/workspace",
		// Volume mounts for Go caches.
		"-v", "gotreesitter-go-mod-cache:/go/pkg/mod",
		"-v", "gotreesitter-go-build-cache:/root/.cache/go-build",
		"-w", "/workspace/cgo_harness",
	}

	// Pass through recognized env vars from host, then overlay opts.EnvVars.
	passthroughEnvs := []string{
		"GOMAXPROCS",
		"GOT_GLR_FOREST",
		"GOT_GLR_MAX_STACKS",
		"GOT_PARSE_NODE_LIMIT_SCALE",
		"GOT_GLR_FORCE_CONFLICT_WIDTH",
		"GTS_PARITY_MODE",
		"GTS_PARITY_SKIP_LANGS",
		"GTS_PARITY_RUN_SKIPPED_BACKLOG",
		"GTS_PARITY_BREAKER",
		"GTS_PARITY_IGNORE_KNOWN_SKIPS",
	}

	// Collect effective env: host passthrough first, opts.EnvVars override.
	effective := make(map[string]string)
	for _, key := range passthroughEnvs {
		if val, ok := os.LookupEnv(key); ok {
			effective[key] = val
		}
	}
	for k, v := range opts.EnvVars {
		effective[k] = v
	}
	for k, v := range effective {
		args = append(args, "-e", k+"="+v)
	}

	args = append(args, image)

	// The command executed inside the container.
	goTestArgs := []string{
		"go", "test", "-json", ".",
		"-tags", "cgo,treesitter_c_parity",
		"-count=1", "-v",
		"-timeout", fmt.Sprintf("%ds", int(opts.Timeout.Seconds())),
	}
	if opts.RunRegex != "" {
		goTestArgs = append(goTestArgs, "-run", opts.RunRegex)
	}
	args = append(args, goTestArgs...)

	t.Logf("docker run: container=%s run=%q timeout=%s", containerName, opts.RunRegex, opts.Timeout)

	// Register cleanup to forcibly remove the container.
	t.Cleanup(func() {
		rm := exec.Command("docker", "rm", "-f", containerName)
		rm.Stdout = nil
		rm.Stderr = nil
		_ = rm.Run()
	})

	start := time.Now()

	cmd := exec.Command("docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	elapsed := time.Since(start)

	// The container runs as root, so it leaves root-owned files in the
	// bind-mounted repository. Hand them back to the host user.
	repairHostOwnership(t, opts.RepoRoot, image)

	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("docker run exec error: %v\nstderr: %s", runErr, stderr.String())
		}
	}

	// Check for OOM kill.
	oomKilled := false
	inspect := exec.Command("docker", "inspect", "--format", "{{.State.OOMKilled}}", containerName)
	if inspectOut, err := inspect.Output(); err == nil {
		oomKilled = strings.TrimSpace(string(inspectOut)) == "true"
	}

	events := parseTest2JSON(stdout.Bytes())

	if stderr.Len() > 0 {
		t.Logf("container stderr (last 2000 chars):\n%s", truncateTail(stderr.String(), 2000))
	}

	return &parityResult{
		ExitCode:  exitCode,
		Events:    events,
		OOMKilled: oomKilled,
		Duration:  elapsed,
	}
}

// ownershipRepairArgs returns the docker arguments of the container that
// returns root-owned entries under repoRoot to the given host user. It skips
// .git/objects, which the test container never writes.
func ownershipRepairArgs(repoRoot, image string, uid, gid int) []string {
	return []string{
		"run", "--rm",
		"-v", repoRoot + ":/workspace",
		"-e", fmt.Sprintf("GTS_HOST_UID=%d", uid),
		"-e", fmt.Sprintf("GTS_HOST_GID=%d", gid),
		image,
		"bash", "-c",
		`find /workspace -xdev -path "*/.git/objects" -prune -o -user 0 -print0 | xargs -0r chown -h "$GTS_HOST_UID:$GTS_HOST_GID"`,
	}
}

// repairHostOwnership runs the ownership repair container. A host that runs
// as root has nothing to repair. A failed repair is logged, not fatal.
func repairHostOwnership(t *testing.T, repoRoot, image string) {
	t.Helper()
	if os.Getuid() == 0 {
		return
	}
	out, err := exec.Command("docker", ownershipRepairArgs(repoRoot, image, os.Getuid(), os.Getgid())...).CombinedOutput()
	if err != nil {
		t.Logf("ownership repair failed: %v\n%s", err, out)
	}
}

// TestOwnershipRepairArgs checks that the repair container mounts the
// repository read-write and chowns to the host user.
func TestOwnershipRepairArgs(t *testing.T) {
	args := strings.Join(ownershipRepairArgs("/repo", "img", 1000, 1001), " ")
	for _, want := range []string{"-v /repo:/workspace", "GTS_HOST_UID=1000", "GTS_HOST_GID=1001", "-user 0", "chown -h"} {
		if !strings.Contains(args, want) {
			t.Errorf("repair args lack %q: %s", want, args)
		}
	}
}

// detectRepoRoot returns the repository root (parent of cgo_harness).
func detectRepoRoot(t *testing.T) string {
	t.Helper()
	// Try git rev-parse first for accuracy.
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	if out, err := cmd.Output(); err == nil {
		root := strings.TrimSpace(string(out))
		if root != "" {
			return root
		}
	}
	// Fallback: cgo_harness is a direct child of repo root.
	abs, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("cannot determine repo root: %v", err)
	}
	return abs
}

// truncateTail returns the last n bytes of s, prefixed with "..." if truncated.
func truncateTail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}

// ---------------------------------------------------------------------------
// Result analysis helpers
// ---------------------------------------------------------------------------

// testOutcome extracts pass/fail/skip counts from parsed test2json events.
func testOutcome(events []test2jsonEvent) (passed, failed, skipped int) {
	for _, ev := range events {
		if ev.Test == "" {
			continue
		}
		switch ev.Action {
		case "pass":
			passed++
		case "fail":
			failed++
		case "skip":
			skipped++
		}
	}
	return
}

// failedTests returns the names of tests with Action "fail".
func failedTests(events []test2jsonEvent) []string {
	var names []string
	seen := make(map[string]bool)
	for _, ev := range events {
		if ev.Action == "fail" && ev.Test != "" && !seen[ev.Test] {
			seen[ev.Test] = true
			names = append(names, ev.Test)
		}
	}
	return names
}

// testOutputFor collects all Output lines for a given test name.
func testOutputFor(events []test2jsonEvent, testName string) string {
	var b strings.Builder
	for _, ev := range events {
		if ev.Test == testName && ev.Output != "" {
			b.WriteString(ev.Output)
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Test functions
// ---------------------------------------------------------------------------

// TestDockerParityStructural runs the full structural parity suite inside a
// Docker container and asserts that all curated languages pass.
func TestDockerParityStructural(t *testing.T) {
	assertDockerAvailable(t)

	result := defaultRunner.runTest(t, containerOpts{
		RunRegex: "^TestParityFreshParse$",
		Timeout:  600 * time.Second,
	})

	t.Logf("structural parity: exit=%d oom=%v duration=%s", result.ExitCode, result.OOMKilled, result.Duration.Round(time.Second))

	if result.OOMKilled {
		t.Fatal("container was OOM-killed")
	}

	passed, failed, skipped := testOutcome(result.Events)
	t.Logf("results: passed=%d failed=%d skipped=%d", passed, failed, skipped)

	if failed > 0 {
		names := failedTests(result.Events)
		for _, name := range names {
			out := testOutputFor(result.Events, name)
			t.Logf("FAIL %s:\n%s", name, truncateTail(out, 1000))
		}
		t.Fatalf("%d structural parity test(s) failed: %s", failed, strings.Join(names, ", "))
	}

	if result.ExitCode != 0 {
		t.Fatalf("container exited with code %d (no individual test failures parsed)", result.ExitCode)
	}
}

// TestDockerParityBacklog runs the skip backlog diagnostic inside a Docker
// container. This is informational — it runs languages currently in the
// known-degraded and skip lists to track progress.
func TestDockerParityBacklog(t *testing.T) {
	assertDockerAvailable(t)

	result := defaultRunner.runTest(t, containerOpts{
		RunRegex: "^TestParitySkippedBacklogFreshParse$",
		EnvVars: map[string]string{
			"GTS_PARITY_RUN_SKIPPED_BACKLOG": "1",
			"GTS_PARITY_IGNORE_KNOWN_SKIPS":  "1",
		},
		Timeout: 600 * time.Second,
	})

	t.Logf("backlog parity: exit=%d oom=%v duration=%s", result.ExitCode, result.OOMKilled, result.Duration.Round(time.Second))

	if result.OOMKilled {
		t.Fatal("container was OOM-killed during backlog run")
	}

	passed, failed, skipped := testOutcome(result.Events)
	t.Logf("backlog results: passed=%d failed=%d skipped=%d", passed, failed, skipped)

	// Log failures for visibility but don't fail the test — this is diagnostic.
	if failed > 0 {
		names := failedTests(result.Events)
		t.Logf("backlog failures (%d): %s", failed, strings.Join(names, ", "))
		for _, name := range names {
			out := testOutputFor(result.Events, name)
			t.Logf("BACKLOG FAIL %s:\n%s", name, truncateTail(out, 500))
		}
	}
}

// TestDockerParityPerLanguage runs parity for specific languages in parallel
// containers. Gated on the DOCKER_PARITY_LANGS env var (comma-separated list
// of language names).
//
// Example:
//
//	DOCKER_PARITY_LANGS=go,rust,python go test -tags docker_parity -run TestDockerParityPerLanguage -v
func TestDockerParityPerLanguage(t *testing.T) {
	assertDockerAvailable(t)

	raw := strings.TrimSpace(os.Getenv("DOCKER_PARITY_LANGS"))
	if raw == "" {
		t.Skip("set DOCKER_PARITY_LANGS=lang1,lang2,... to run per-language Docker parity")
	}

	langs := splitLangs(raw)
	if len(langs) == 0 {
		t.Skip("DOCKER_PARITY_LANGS is empty after parsing")
	}

	// Pre-build the image once before spawning parallel subtests.
	defaultRunner.buildImage(t)

	for _, lang := range langs {
		lang := lang
		t.Run(lang, func(t *testing.T) {
			t.Parallel()

			result := defaultRunner.runTest(t, containerOpts{
				RunRegex: fmt.Sprintf("^TestParityFreshParse/%s$", lang),
				Timeout:  300 * time.Second,
			})

			t.Logf("[%s] exit=%d oom=%v duration=%s", lang, result.ExitCode, result.OOMKilled, result.Duration.Round(time.Second))

			if result.OOMKilled {
				t.Fatalf("[%s] container was OOM-killed", lang)
			}

			passed, failed, skipped := testOutcome(result.Events)
			t.Logf("[%s] passed=%d failed=%d skipped=%d", lang, passed, failed, skipped)

			if failed > 0 {
				names := failedTests(result.Events)
				for _, name := range names {
					out := testOutputFor(result.Events, name)
					t.Logf("[%s] FAIL %s:\n%s", lang, name, truncateTail(out, 1000))
				}
				t.Fatalf("[%s] %d test(s) failed", lang, failed)
			}

			if result.ExitCode != 0 {
				t.Fatalf("[%s] container exited with code %d", lang, result.ExitCode)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Utilities
// ---------------------------------------------------------------------------

// assertDockerAvailable skips the test if Docker is not reachable.
func assertDockerAvailable(t *testing.T) {
	t.Helper()
	cmd := exec.Command("docker", "info")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		t.Skipf("docker not available: %v", err)
	}
}

// splitLangs splits a comma-separated string into trimmed, non-empty names.
func splitLangs(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}
