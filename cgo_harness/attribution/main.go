// Command attribution is campaign v7 tranche C0's attribution board tool. It
// is measurement infrastructure only: it builds and runs existing test
// binaries in the repository under existing, already-committed build tags and
// entry points, and never edits parser code, routing, or shipped behavior.
//
// It produces one boundary-defined attribution receipt (a markdown table and
// a JSON file) over the compact parser core's fresh-full-parse CPU, plus a
// noise floor and a cost-per-event join. See the perf-attribution record (Hyphae space m31labs/gotreesitter) for the
// methodology this tool implements and the first published receipt.
//
// Usage (from the repository root):
//
//	go run ./cgo_harness/attribution
//
// All flags are optional; see -help. The command builds both required test
// binaries first, extracts the four pinned canonical fixtures, then runs
// every measurement in one final pass, and finally writes the receipt.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "attribution: error:", err)
		os.Exit(1)
	}
}

type config struct {
	repoRoot        string
	outDir          string
	captureDuration time.Duration
	noiseSamples    int
	noiseBenchtime  time.Duration
	core            string
}

func run() error {
	var cfg config
	var repoFlag, outFlag, durationFlag, noiseBenchtimeFlag string
	var reanalyze bool
	flag.StringVar(&repoFlag, "repo", "", "repository root (default: current directory)")
	flag.StringVar(&outFlag, "out", "", "output directory (default: <repo>/harness_out/perf_attribution/<timestamp>)")
	flag.StringVar(&durationFlag, "duration", "800ms", "per-fixture CPU-profile capture duration (wall clock floor; a 5-iteration minimum always applies)")
	flag.IntVar(&cfg.noiseSamples, "noise-samples", 10, "interleaved A/A pairs for the noise floor (n>=10 required)")
	flag.StringVar(&noiseBenchtimeFlag, "noise-benchtime", "700ms", "benchtime per noise-floor sample")
	flag.StringVar(&cfg.core, "core", "", "optional CPU to pin with taskset (default: no pinning)")
	flag.BoolVar(&reanalyze, "reanalyze", false, "skip build and measurement; recompute the receipt from an existing -out directory's artifacts")
	flag.Parse()

	if repoFlag == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getwd: %w", err)
		}
		repoFlag = wd
	}
	repoRoot, err := filepath.Abs(repoFlag)
	if err != nil {
		return err
	}
	if err := verifyRepoRoot(repoRoot); err != nil {
		return err
	}
	cfg.repoRoot = repoRoot

	duration, err := time.ParseDuration(durationFlag)
	if err != nil || duration <= 0 {
		return fmt.Errorf("-duration must be a positive Go duration, got %q", durationFlag)
	}
	cfg.captureDuration = duration

	noiseBenchtime, err := time.ParseDuration(noiseBenchtimeFlag)
	if err != nil || noiseBenchtime <= 0 {
		return fmt.Errorf("-noise-benchtime must be a positive Go duration, got %q", noiseBenchtimeFlag)
	}
	cfg.noiseBenchtime = noiseBenchtime
	if cfg.noiseSamples < 10 {
		return fmt.Errorf("-noise-samples must be >= 10, got %d", cfg.noiseSamples)
	}

	if outFlag == "" {
		outFlag = filepath.Join(repoRoot, "harness_out", "perf_attribution", time.Now().UTC().Format("20060102T150405Z"))
	}
	outDir, err := filepath.Abs(outFlag)
	if err != nil {
		return err
	}
	cfg.outDir = outDir

	for _, sub := range []string{"bin", "fixtures", "profiles", "workcount", "noise"} {
		if err := os.MkdirAll(filepath.Join(outDir, sub), 0o755); err != nil {
			return err
		}
	}

	fmt.Println("attribution: repo       =", cfg.repoRoot)
	fmt.Println("attribution: out        =", cfg.outDir)
	fmt.Println("attribution: duration   =", cfg.captureDuration)
	fmt.Println("attribution: noise pairs=", cfg.noiseSamples)

	var captureManifest *captureManifest
	var workcounts map[string]workCountResult
	var noiseSamples []noiseSample

	if reanalyze {
		// Recompute the receipt from an existing -out directory's already-
		// captured artifacts. Skips build and every timed measurement -- use
		// this only to iterate on classify.go/receipt.go against a
		// measurement pass that already ran, never to fabricate a new
		// measurement without rerunning the timed phase.
		fmt.Println("attribution: [reanalyze] loading existing artifacts from", outDir)
		var err error
		captureManifest, err = loadCaptureManifest(outDir)
		if err != nil {
			return fmt.Errorf("reanalyze: load capture manifest: %w", err)
		}
		workcounts, err = loadWorkCounts(outDir)
		if err != nil {
			return fmt.Errorf("reanalyze: load work counts: %w", err)
		}
		noiseSamples, err = loadNoiseSamples(outDir)
		if err != nil {
			return fmt.Errorf("reanalyze: load noise samples: %w", err)
		}
	} else {
		// ---- Build phase: everything is built and written first, before any
		// timed measurement runs, per the campaign's noise-sequencing rule. ----
		fmt.Println("attribution: [build] extracting canonical fixtures")
		fixturePaths, err := extractFixtures(cfg)
		if err != nil {
			return fmt.Errorf("extract fixtures: %w", err)
		}

		fmt.Println("attribution: [build] compiling capture test binary (tags gts_parsercorephase0)")
		captureBin := filepath.Join(outDir, "bin", "capture.test")
		if err := goTestBuild(cfg.repoRoot, "gts_parsercorephase0", captureBin); err != nil {
			return fmt.Errorf("build capture binary: %w", err)
		}

		fmt.Println("attribution: [build] compiling work-count test binary (tags gts_parsercorephase0,gts_workcount)")
		workcountBin := filepath.Join(outDir, "bin", "workcount.test")
		if err := goTestBuild(cfg.repoRoot, "gts_parsercorephase0,gts_workcount", workcountBin); err != nil {
			return fmt.Errorf("build workcount binary: %w", err)
		}

		// ---- Measurement phase: one final pass. ----
		fmt.Println("attribution: [measure] capturing CPU profiles (diagnostic lane x4, shipped route x4)")
		captureManifest, err = runCapture(cfg, captureBin)
		if err != nil {
			return fmt.Errorf("run capture: %w", err)
		}

		fmt.Println("attribution: [measure] running work-count children (event counts per fixture)")
		workcounts, err = runWorkCounts(cfg, workcountBin, fixturePaths)
		if err != nil {
			return fmt.Errorf("run work counts: %w", err)
		}

		fmt.Println("attribution: [measure] running interleaved A/A noise floor")
		noiseSamples, err = runNoiseFloor(cfg, captureBin)
		if err != nil {
			return fmt.Errorf("run noise floor: %w", err)
		}
	}

	// ---- Analysis phase. ----
	fmt.Println("attribution: [analyze] mapping CPU profiles to the attribution tree")
	receipt, err := buildReceipt(cfg, captureManifest, workcounts, noiseSamples)
	if err != nil {
		return fmt.Errorf("build receipt: %w", err)
	}

	jsonPath := filepath.Join(outDir, "receipt.json")
	if err := writeJSON(jsonPath, receipt); err != nil {
		return err
	}
	mdPath := filepath.Join(outDir, "receipt.md")
	if err := os.WriteFile(mdPath, []byte(renderMarkdown(receipt)), 0o644); err != nil {
		return err
	}
	fmt.Println("attribution: wrote", jsonPath)
	fmt.Println("attribution: wrote", mdPath)
	fmt.Println(renderMarkdown(receipt))
	return nil
}

func verifyRepoRoot(root string) error {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return fmt.Errorf("-repo %s does not look like the gotreesitter repository root: %w", root, err)
	}
	if !strings.Contains(string(data), "module github.com/odvcencio/gotreesitter\n") {
		return fmt.Errorf("-repo %s go.mod is not the gotreesitter root module", root)
	}
	return nil
}

// ---- Build helpers ----

type fixtureSpec struct {
	ID           string
	Asset        string
	Bytes        int
	SourceSHA256 string
}

// fixtureSpecs mirrors the same four pinned canonical fixtures already
// authenticated in diagnosticParserCoreCanonicalAdmissions (root package) and
// cgo_harness/pure_c/run_canonical_go_full_parse.sh. Triple-pinning identity
// constants across independent tools is this repository's existing
// convention for these fixtures.
var fixtureSpecs = []fixtureSpec{
	{ID: "rewrite", Asset: "rewrite.go.gz", Bytes: 5116, SourceSHA256: "74c0705f8729670559492fb5460a01b2a1a2a109928e1aeb52736e485e8ff097"},
	{ID: "query_compile", Asset: "query_compile.go.gz", Bytes: 20168, SourceSHA256: "b788ee19b0075f0b9b567a9f93ea657e715bc8a6a40a99d3ca5c761404e71894"},
	{ID: "language", Asset: "language.go.gz", Bytes: 41387, SourceSHA256: "009aa9fd5352c712f3839670c7df8a9b00ae878ee20dc88131a438b2d5edfd9a"},
	{ID: "grammargen_lr", Asset: "grammargen_lr.go.gz", Bytes: 235626, SourceSHA256: "a7e4a1a64b25a60aea36183b9d6d53dcd9240942cdb10e67a3cf9e6ce30f95b2"},
}

func extractFixtures(cfg config) (map[string]string, error) {
	paths := map[string]string{}
	for _, spec := range fixtureSpecs {
		assetPath := filepath.Join(cfg.repoRoot, "internal", "benchfixtures", "testdata", spec.Asset)
		data, err := gunzipFile(assetPath)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", spec.ID, err)
		}
		if len(data) != spec.Bytes {
			return nil, fmt.Errorf("%s: got %d bytes, want %d", spec.ID, len(data), spec.Bytes)
		}
		if got := sha256Hex(data); got != spec.SourceSHA256 {
			return nil, fmt.Errorf("%s: source sha256=%s want=%s", spec.ID, got, spec.SourceSHA256)
		}
		outPath := filepath.Join(cfg.outDir, "fixtures", spec.ID+".go")
		if err := os.WriteFile(outPath, data, 0o644); err != nil {
			return nil, err
		}
		paths[spec.ID] = outPath
	}
	return paths, nil
}

func goTestBuild(repoRoot, tags, outBin string) error {
	cmd := exec.Command("go", "test", "-tags", tags, "-c", "-o", outBin, ".")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOMAXPROCS=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v\n%s", err, out)
	}
	return nil
}

// ---- Capture phase ----

type captureRow struct {
	Label       string  `json:"label"`
	Fixture     string  `json:"fixture"`
	SourceBytes int     `json:"source_bytes"`
	Iterations  int     `json:"iterations"`
	ElapsedNS   int64   `json:"elapsed_ns"`
	NSPerOp     float64 `json:"ns_per_op"`
	ProfilePath string  `json:"profile_path"`
}

type captureManifest struct {
	SchemaVersion string       `json:"schema_version"`
	GeneratedUTC  string       `json:"generated_utc"`
	DurationParam string       `json:"duration_param"`
	Rows          []captureRow `json:"rows"`
}

func runCapture(cfg config, captureBin string) (*captureManifest, error) {
	profilesDir := filepath.Join(cfg.outDir, "profiles")
	args := []string{"-test.run", "^TestParserCoreAttributionCapture$", "-test.v"}
	cmd := commandWithCore(cfg, captureBin, args...)
	cmd.Dir = cfg.repoRoot
	cmd.Env = append(os.Environ(),
		"GOMAXPROCS=1",
		"GTS_ATTRIBUTION_OUT="+profilesDir,
		"GTS_ATTRIBUTION_DURATION="+cfg.captureDuration.String(),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%v\n%s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(profilesDir, "capture_manifest.json"))
	if err != nil {
		return nil, err
	}
	var manifest captureManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// loadCaptureManifest re-reads a prior run's capture_manifest.json for
// -reanalyze, without invoking the capture binary again.
func loadCaptureManifest(outDir string) (*captureManifest, error) {
	data, err := os.ReadFile(filepath.Join(outDir, "profiles", "capture_manifest.json"))
	if err != nil {
		return nil, err
	}
	var manifest captureManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// ---- Work-count phase ----

type workCountResult struct {
	Fixture     string `json:"fixture"`
	SourceBytes uint32 `json:"source_bytes"`
	CoreWork    struct {
		Shifts               uint64 `json:"shifts"`
		Reductions           uint64 `json:"reductions"`
		ReductionPopRequests uint64 `json:"reduction_pop_requests"`
		EmittedPopPaths      uint64 `json:"emitted_pop_paths"`
		EmittedPopPayloads   uint64 `json:"emitted_pop_payloads"`
	} `json:"core_work"`
	SchedulerWork struct {
		ActionLookups     uint64 `json:"action_lookups"`
		Elections         uint64 `json:"elections"`
		Conflicts         uint64 `json:"conflicts"`
		Canonicalizations uint64 `json:"canonicalizations"`
		PeakHeaders       uint64 `json:"peak_headers"`
	} `json:"scheduler_work"`
}

func runWorkCounts(cfg config, workcountBin string, fixturePaths map[string]string) (map[string]workCountResult, error) {
	results := map[string]workCountResult{}
	for _, spec := range fixtureSpecs {
		resultPath := filepath.Join(cfg.outDir, "workcount", spec.ID+".json")
		cmd := commandWithCore(cfg, workcountBin, "-test.run", "^TestParserCoreWorkCountChild$", "-test.v")
		cmd.Dir = cfg.repoRoot
		cmd.Env = append(os.Environ(),
			"GOMAXPROCS=1",
			"GTS_WORK_COUNT_FIXTURE="+spec.ID,
			"GTS_WORK_COUNT_SOURCE="+fixturePaths[spec.ID],
			"GTS_WORK_COUNT_RESULT="+resultPath,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("%s: %v\n%s", spec.ID, err, out)
		}
		data, err := os.ReadFile(resultPath)
		if err != nil {
			return nil, err
		}
		var result workCountResult
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, err
		}
		results[spec.ID] = result
	}
	return results, nil
}

// loadWorkCounts re-reads a prior run's per-fixture work-count JSON files for
// -reanalyze, without invoking the work-count binary again.
func loadWorkCounts(outDir string) (map[string]workCountResult, error) {
	results := map[string]workCountResult{}
	for _, spec := range fixtureSpecs {
		data, err := os.ReadFile(filepath.Join(outDir, "workcount", spec.ID+".json"))
		if err != nil {
			return nil, err
		}
		var result workCountResult
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, err
		}
		results[spec.ID] = result
	}
	return results, nil
}

// ---- Noise floor phase ----

type noiseSample struct {
	Series  string // "A" or "B"
	Fixture string
	NSPerOp float64
}

// The "-N" GOMAXPROCS suffix only appears when the binary is invoked with
// -test.cpu specifying multiple CPU counts; a plain run (this tool's case)
// omits it, so the suffix is optional here.
var benchLineRe = regexp.MustCompile(`^BenchmarkParserCoreFreshFullCanonical/([A-Za-z0-9_]+)(?:-[0-9]+)?\s+([0-9]+)\s+([0-9.]+)\s+ns/op`)

func runNoiseFloor(cfg config, captureBin string) ([]noiseSample, error) {
	var samples []noiseSample
	for i := 0; i < cfg.noiseSamples; i++ {
		for _, series := range []string{"A", "B"} {
			raw, err := runBenchOnce(cfg, captureBin, i, series)
			if err != nil {
				return nil, err
			}
			perFixture := parseBenchOutput(raw)
			if len(perFixture) != len(fixtureSpecs) {
				return nil, fmt.Errorf("noise floor pair %d series %s: got %d fixture results, want %d\n%s", i, series, len(perFixture), len(fixtureSpecs), raw)
			}
			for fixture, ns := range perFixture {
				samples = append(samples, noiseSample{Series: series, Fixture: fixture, NSPerOp: ns})
			}
		}
	}
	return samples, nil
}

func runBenchOnce(cfg config, captureBin string, pairIndex int, series string) (string, error) {
	rawPath := filepath.Join(cfg.outDir, "noise", fmt.Sprintf("pair%02d_%s.txt", pairIndex, series))
	cmd := commandWithCore(cfg, captureBin,
		"-test.run", "^$",
		"-test.bench", "^BenchmarkParserCoreFreshFullCanonical$",
		"-test.benchtime", cfg.noiseBenchtime.String(),
		"-test.count=1",
	)
	cmd.Dir = cfg.repoRoot
	cmd.Env = append(os.Environ(), "GOMAXPROCS=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("bench pair %d series %s: %v\n%s", pairIndex, series, err, out)
	}
	if err := os.WriteFile(rawPath, out, 0o644); err != nil {
		return "", err
	}
	return string(out), nil
}

// loadNoiseSamples re-parses a prior run's saved noise-floor raw benchmark
// output files for -reanalyze, without invoking the capture binary again.
func loadNoiseSamples(outDir string) ([]noiseSample, error) {
	var samples []noiseSample
	for _, series := range []string{"A", "B"} {
		matches, err := filepath.Glob(filepath.Join(outDir, "noise", "pair*_"+series+".txt"))
		if err != nil {
			return nil, err
		}
		sort.Strings(matches)
		if len(matches) == 0 {
			return nil, fmt.Errorf("no saved noise-floor series %s files under %s", series, outDir)
		}
		for _, path := range matches {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			perFixture := parseBenchOutput(string(data))
			if len(perFixture) != len(fixtureSpecs) {
				return nil, fmt.Errorf("%s: got %d fixture results, want %d", path, len(perFixture), len(fixtureSpecs))
			}
			for fixture, ns := range perFixture {
				samples = append(samples, noiseSample{Series: series, Fixture: fixture, NSPerOp: ns})
			}
		}
	}
	return samples, nil
}

func parseBenchOutput(output string) map[string]float64 {
	result := map[string]float64{}
	for _, line := range strings.Split(output, "\n") {
		m := benchLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ns, err := strconv.ParseFloat(m[3], 64)
		if err != nil {
			continue
		}
		result[m[1]] = ns
	}
	return result
}

// commandWithCore optionally wraps the command with taskset -c <core>.
func commandWithCore(cfg config, bin string, args ...string) *exec.Cmd {
	if cfg.core != "" {
		full := append([]string{"-c", cfg.core, bin}, args...)
		return exec.Command("taskset", full...)
	}
	return exec.Command(bin, args...)
}

// ---- small helpers ----

func writeJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func sortedFixtureIDs() []string {
	ids := make([]string, len(fixtureSpecs))
	for i, spec := range fixtureSpecs {
		ids[i] = spec.ID
	}
	sort.Strings(ids)
	return ids
}
