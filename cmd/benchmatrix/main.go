package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type benchSample struct {
	NsPerOp     float64 `json:"ns_per_op,omitempty"`
	BytesPerOp  float64 `json:"bytes_per_op,omitempty"`
	AllocsPerOp float64 `json:"allocs_per_op,omitempty"`
	MBPerSec    float64 `json:"mb_per_sec,omitempty"`
}

type benchStats struct {
	Samples            int     `json:"samples"`
	MeanNsPerOp        float64 `json:"mean_ns_per_op,omitempty"`
	P95RunToRunNsPerOp float64 `json:"p95_run_to_run_nsop,omitempty"`
	MeanBytesPerOp     float64 `json:"mean_bytes_per_op,omitempty"`
	MeanAllocsPerOp    float64 `json:"mean_allocs_per_op,omitempty"`
	MeanMBPerSec       float64 `json:"mean_mb_per_sec,omitempty"`
}

type resourceStats struct {
	UserSeconds   float64 `json:"user_seconds,omitempty"`
	SystemSeconds float64 `json:"system_seconds,omitempty"`
	Elapsed       string  `json:"elapsed,omitempty"`
	MaxRSSKB      int64   `json:"max_rss_kb,omitempty"`
}

type benchGroupResult struct {
	Name       string                `json:"name"`
	Regex      string                `json:"regex"`
	Package    string                `json:"package"`
	Benchmarks map[string]benchStats `json:"benchmarks"`
	RawOutput  string                `json:"raw_output_path"`
	TimeOutput string                `json:"time_output_path,omitempty"`
	Resources  *resourceStats        `json:"resources,omitempty"`
}

type matrixResult struct {
	GeneratedAtUTC string             `json:"generated_at_utc"`
	GoVersion      string             `json:"go_version"`
	Count          int                `json:"count"`
	Benchtime      string             `json:"benchtime"`
	GOMAXPROCS     int                `json:"gomaxprocs"`
	FamilyUnitN    int                `json:"family_unit_count"`
	StressFuncN    int                `json:"stress_func_count"`
	Groups         []benchGroupResult `json:"groups"`
}

type benchGroupConfig struct {
	Name     string
	Regex    string
	Package  string
	ExtraEnv map[string]string
}

var benchLineRe = regexp.MustCompile(`^Benchmark[^\s]+`)
var suffixNumRe = regexp.MustCompile(`-\d+$`)

func main() {
	var (
		count       int
		benchtime   string
		gomaxprocs  int
		outPath     string
		mdPath      string
		rawDir      string
		familyUnitN int
		stressFuncN int
		noStress    bool
		noFamily    bool
		onlyFamily  bool
		captureRSS  bool
	)

	flag.IntVar(&count, "count", 10, "go test benchmark count")
	flag.StringVar(&benchtime, "benchtime", "750ms", "go test benchmark benchtime")
	flag.IntVar(&gomaxprocs, "gomaxprocs", 1, "GOMAXPROCS value for benchmark subprocesses (0 = inherit)")
	flag.StringVar(&outPath, "out", "bench_out/matrix.json", "output JSON path")
	flag.StringVar(&mdPath, "markdown", "bench_out/matrix.md", "output markdown path")
	flag.StringVar(&rawDir, "raw-dir", "bench_out/raw", "directory for raw benchmark outputs")
	flag.IntVar(&familyUnitN, "family-unit-count", 25, "value for GOT_BENCH_FUNC_COUNT in family full-parse matrix")
	flag.IntVar(&stressFuncN, "stress-func-count", 5000, "value for GOT_BENCH_FUNC_COUNT in stress class")
	flag.BoolVar(&noStress, "no-stress", false, "skip stress class")
	flag.BoolVar(&noFamily, "no-family", false, "skip language-family full-parse matrix")
	flag.BoolVar(&onlyFamily, "only-family", false, "run only the language-family full-parse matrix")
	flag.BoolVar(&captureRSS, "rss", false, "capture /usr/bin/time -v resource stats per group")
	flag.Parse()

	if count <= 0 {
		fatalf("--count must be > 0")
	}
	if strings.TrimSpace(benchtime) == "" {
		fatalf("--benchtime must be non-empty")
	}
	if familyUnitN <= 0 {
		fatalf("--family-unit-count must be > 0")
	}
	if onlyFamily && noFamily {
		fatalf("--only-family cannot be combined with --no-family")
	}

	var groups []benchGroupConfig
	if !onlyFamily {
		groups = append(groups,
			benchGroupConfig{
				Name:    "editor_hot_path",
				Package: ". ./roottest/bench",
				Regex:   "^(BenchmarkGoParseIncrementalSingleByteEdit|BenchmarkGoParseIncrementalRandomSingleByteEdit|BenchmarkGoParseIncrementalNoEdit|BenchmarkHighlightIncremental|BenchmarkTaggerTagIncremental)$",
			},
			benchGroupConfig{
				Name:    "indexing_path",
				Package: ". ./roottest/bench",
				Regex:   "^(BenchmarkGoParseFull|BenchmarkGoParseFullDFA|BenchmarkQueryExecCompiled|BenchmarkTaggerTag)$",
			},
		)
	}
	if !noFamily {
		groups = append(groups, benchGroupConfig{
			Name:    "family_full_parse",
			Package: ".",
			Regex:   "^BenchmarkFamilyParseFullDFA$",
			ExtraEnv: map[string]string{
				"GOT_BENCH_FUNC_COUNT": strconv.Itoa(familyUnitN),
			},
		})
	}
	if !noStress && !onlyFamily {
		groups = append(groups, benchGroupConfig{
			Name:    "stress_path",
			Package: ".",
			Regex:   "^(BenchmarkGoParseFull|BenchmarkGoParseFullDFA)$",
			ExtraEnv: map[string]string{
				"GOT_BENCH_FUNC_COUNT": strconv.Itoa(stressFuncN),
			},
		})
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		fatalf("create JSON output dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(mdPath), 0o755); err != nil {
		fatalf("create markdown output dir: %v", err)
	}
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		fatalf("create raw output dir: %v", err)
	}

	results := make([]benchGroupResult, 0, len(groups))
	for _, group := range groups {
		res, err := runBenchGroup(group, count, benchtime, gomaxprocs, captureRSS, rawDir)
		if err != nil {
			fatalf("group %s failed: %v", group.Name, err)
		}
		results = append(results, res)
	}

	matrix := matrixResult{
		GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339),
		GoVersion:      runtime.Version(),
		Count:          count,
		Benchtime:      benchtime,
		GOMAXPROCS:     gomaxprocs,
		FamilyUnitN:    familyUnitN,
		StressFuncN:    stressFuncN,
		Groups:         results,
	}
	if err := writeJSON(outPath, matrix); err != nil {
		fatalf("write JSON: %v", err)
	}
	if err := writeMarkdown(mdPath, matrix); err != nil {
		fatalf("write markdown: %v", err)
	}

	fmt.Printf("benchmatrix JSON: %s\n", outPath)
	fmt.Printf("benchmatrix markdown: %s\n", mdPath)
}

func runBenchGroup(group benchGroupConfig, count int, benchtime string, gomaxprocs int, captureRSS bool, rawDir string) (benchGroupResult, error) {
	args := []string{
		"test",
		"-run", "^$",
		"-bench", group.Regex,
		"-benchmem",
		"-benchtime", benchtime,
		"-count", strconv.Itoa(count),
	}
	// Package holds one or more package paths separated by spaces. Go runs the
	// benchmarks of several packages one after another.
	args = append(args, strings.Fields(group.Package)...)
	baseEnv := os.Environ()
	if gomaxprocs > 0 {
		baseEnv = append(baseEnv, fmt.Sprintf("GOMAXPROCS=%d", gomaxprocs))
	}
	for k, v := range group.ExtraEnv {
		baseEnv = append(baseEnv, k+"="+v)
	}

	var cmd *exec.Cmd
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if captureRSS {
		if _, err := exec.LookPath("/usr/bin/time"); err != nil {
			return benchGroupResult{}, fmt.Errorf("capture RSS requested but /usr/bin/time is unavailable: %w", err)
		}
		timeArgs := append([]string{"-v", "go"}, args...)
		cmd = exec.Command("/usr/bin/time", timeArgs...)
	} else {
		cmd = exec.Command("go", args...)
	}
	cmd.Dir = "."
	cmd.Env = baseEnv
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return benchGroupResult{}, fmt.Errorf("%s %s: %w\nstderr:\n%s\nstdout:\n%s", cmd.Path, strings.Join(cmd.Args[1:], " "), err, stderr.String(), stdout.String())
	}

	rawPath := filepath.Join(rawDir, group.Name+".txt")
	if err := os.WriteFile(rawPath, stdout.Bytes(), 0o644); err != nil {
		return benchGroupResult{}, fmt.Errorf("write raw output %s: %w", rawPath, err)
	}
	var (
		timePath  string
		resources *resourceStats
	)
	if captureRSS {
		timePath = filepath.Join(rawDir, group.Name+".time.txt")
		if err := os.WriteFile(timePath, stderr.Bytes(), 0o644); err != nil {
			return benchGroupResult{}, fmt.Errorf("write timing output %s: %w", timePath, err)
		}
		stats := parseResourceStats(stderr.Bytes())
		resources = &stats
	}

	parsed, err := parseBenchOutput(stdout.Bytes())
	if err != nil {
		return benchGroupResult{}, fmt.Errorf("parse output: %w", err)
	}
	aggregated := aggregateBenchSamples(parsed)
	return benchGroupResult{
		Name:       group.Name,
		Regex:      group.Regex,
		Package:    group.Package,
		Benchmarks: aggregated,
		RawOutput:  rawPath,
		TimeOutput: timePath,
		Resources:  resources,
	}, nil
}

func parseBenchOutput(out []byte) (map[string][]benchSample, error) {
	samples := map[string][]benchSample{}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !benchLineRe.MatchString(line) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		name := suffixNumRe.ReplaceAllString(fields[0], "")
		sample := benchSample{}
		for i := 2; i+1 < len(fields); i += 2 {
			value, err := strconv.ParseFloat(strings.TrimSpace(fields[i]), 64)
			if err != nil {
				continue
			}
			unit := fields[i+1]
			switch unit {
			case "ns/op":
				sample.NsPerOp = value
			case "B/op":
				sample.BytesPerOp = value
			case "allocs/op":
				sample.AllocsPerOp = value
			case "MB/s":
				sample.MBPerSec = value
			}
		}
		samples[name] = append(samples[name], sample)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return samples, nil
}

func aggregateBenchSamples(raw map[string][]benchSample) map[string]benchStats {
	out := make(map[string]benchStats, len(raw))
	for name, runs := range raw {
		ns := make([]float64, 0, len(runs))
		bytesOp := make([]float64, 0, len(runs))
		allocs := make([]float64, 0, len(runs))
		mbs := make([]float64, 0, len(runs))
		for _, s := range runs {
			if s.NsPerOp > 0 {
				ns = append(ns, s.NsPerOp)
			}
			if s.BytesPerOp > 0 {
				bytesOp = append(bytesOp, s.BytesPerOp)
			}
			if s.AllocsPerOp > 0 {
				allocs = append(allocs, s.AllocsPerOp)
			}
			if s.MBPerSec > 0 {
				mbs = append(mbs, s.MBPerSec)
			}
		}
		out[name] = benchStats{
			Samples:            len(runs),
			MeanNsPerOp:        mean(ns),
			P95RunToRunNsPerOp: percentile(ns, 95),
			MeanBytesPerOp:     mean(bytesOp),
			MeanAllocsPerOp:    mean(allocs),
			MeanMBPerSec:       mean(mbs),
		}
	}
	return out
}

func parseResourceStats(stderr []byte) resourceStats {
	stats := resourceStats{}
	scanner := bufio.NewScanner(bytes.NewReader(stderr))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "User time (seconds):"):
			stats.UserSeconds = parseTrailingFloat(line)
		case strings.HasPrefix(line, "System time (seconds):"):
			stats.SystemSeconds = parseTrailingFloat(line)
		case strings.HasPrefix(line, "Elapsed (wall clock) time"):
			if idx := strings.LastIndex(line, ":"); idx >= 0 && idx+1 < len(line) {
				stats.Elapsed = strings.TrimSpace(line[idx+1:])
			}
		case strings.HasPrefix(line, "Maximum resident set size (kbytes):"):
			stats.MaxRSSKB = parseTrailingInt64(line)
		}
	}
	return stats
}

func parseTrailingFloat(line string) float64 {
	idx := strings.LastIndex(line, ":")
	if idx < 0 || idx+1 >= len(line) {
		return 0
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(line[idx+1:]), 64)
	if err != nil {
		return 0
	}
	return v
}

func parseTrailingInt64(line string) int64 {
	idx := strings.LastIndex(line, ":")
	if idx < 0 || idx+1 >= len(line) {
		return 0
	}
	v, err := strconv.ParseInt(strings.TrimSpace(line[idx+1:]), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func writeJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeMarkdown(path string, matrix matrixResult) error {
	var b strings.Builder
	b.WriteString("# Bench Matrix\n\n")
	b.WriteString(fmt.Sprintf("- generated: %s\n", matrix.GeneratedAtUTC))
	b.WriteString(fmt.Sprintf("- go: `%s`\n", matrix.GoVersion))
	b.WriteString(fmt.Sprintf("- count: `%d`\n", matrix.Count))
	b.WriteString(fmt.Sprintf("- benchtime: `%s`\n", matrix.Benchtime))
	b.WriteString(fmt.Sprintf("- gomaxprocs: `%d`\n", matrix.GOMAXPROCS))
	b.WriteString(fmt.Sprintf("- family unit count: `%d`\n", matrix.FamilyUnitN))
	b.WriteString(fmt.Sprintf("- stress func count: `%d`\n\n", matrix.StressFuncN))

	for _, group := range matrix.Groups {
		b.WriteString(fmt.Sprintf("## %s\n\n", group.Name))
		b.WriteString("| Benchmark | Samples | Mean ns/op | P95 run-to-run ns/op | Mean B/op | Mean allocs/op | Mean MB/s |\n")
		b.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
		names := make([]string, 0, len(group.Benchmarks))
		for name := range group.Benchmarks {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			s := group.Benchmarks[name]
			b.WriteString(fmt.Sprintf("| `%s` | %d | %s | %s | %s | %s | %s |\n",
				name,
				s.Samples,
				fmtFloat(s.MeanNsPerOp),
				fmtFloat(s.P95RunToRunNsPerOp),
				fmtFloat(s.MeanBytesPerOp),
				fmtFloat(s.MeanAllocsPerOp),
				fmtFloat(s.MeanMBPerSec),
			))
		}
		b.WriteString(fmt.Sprintf("\nraw: `%s`\n\n", group.RawOutput))
		if group.TimeOutput != "" {
			b.WriteString(fmt.Sprintf("time: `%s`\n\n", group.TimeOutput))
		}
		if group.Resources != nil {
			b.WriteString(fmt.Sprintf("- max RSS (KB): `%d`\n", group.Resources.MaxRSSKB))
			b.WriteString(fmt.Sprintf("- user seconds: `%s`\n", fmtFloat(group.Resources.UserSeconds)))
			b.WriteString(fmt.Sprintf("- system seconds: `%s`\n", fmtFloat(group.Resources.SystemSeconds)))
			if group.Resources.Elapsed != "" {
				b.WriteString(fmt.Sprintf("- elapsed: `%s`\n", group.Resources.Elapsed))
			}
			b.WriteString("\n")
		}
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	ys := make([]float64, len(xs))
	copy(ys, xs)
	sort.Float64s(ys)
	if len(ys) == 1 {
		return ys[0]
	}
	rank := (p / 100.0) * float64(len(ys)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return ys[lo]
	}
	w := rank - float64(lo)
	return ys[lo]*(1.0-w) + ys[hi]*w
}

func fmtFloat(v float64) string {
	if v == 0 {
		return ""
	}
	if v >= 1000 {
		return fmt.Sprintf("%.0f", v)
	}
	if v >= 10 {
		return fmt.Sprintf("%.2f", v)
	}
	return fmt.Sprintf("%.4f", v)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
