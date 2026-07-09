//go:build cgo && treesitter_c_parity && treesitter_c_perfscan

package cgoharness

// Per-language Go-vs-C real-corpus timing scan ("perf scan").
//
// Produces a machine-readable JSON scoreboard plus a human markdown summary
// under cgo_harness/perf_scan/out/, measuring for every language with a C
// reference grammar (grammars/languages.lock via ParityCLanguage) and local
// corpus files:
//
//   - full     fresh full-parse wall time, Go vs C, median of N reps
//   - noedit   no-edit reparse with the previous tree (Go ParseIncremental /
//              C ts_parser_parse with old tree), median of N reps
//   - edit     single-byte-edit incremental reparse (opt-in axis; see README)
//
// Cliff containment: every parse attempt runs under a per-file budget
// (Go: Parser.SetTimeoutMicros -> ParseStoppedEarly; C: SetTimeoutMicros ->
// nil tree), and every language runs in its own subprocess with a hard
// wall-clock kill, so one pathological file or grammar cannot hang or crash
// the sweep. Timed-out files are surfaced as lower-bound ratios and "cliff"
// verdicts instead of hanging.
//
// Build/run discipline mirrors the parity suites: requires the build tags
// "treesitter_c_parity treesitter_c_perfscan", the container-or-
// GTS_PARITY_ALLOW_HOST=1 TestMain guard, and the GTS_PERF_SCAN=1 env gate,
// so it never burdens normal builds or CI.
//
// Usage (from cgo_harness/):
//
//	GOWORK=off GTS_PARITY_ALLOW_HOST=1 GTS_PERF_SCAN=1 \
//	  go test -tags "treesitter_c_parity treesitter_c_perfscan" \
//	  -run '^TestPerfScanSweep$' -v -timeout 0 .
//
// See perf_scan/README.md for the full knob reference.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const (
	perfScanSchema = "gts-perf-scan/v1"

	perfScanEnvGate        = "GTS_PERF_SCAN"
	perfScanEnvLang        = "GTS_PERF_SCAN_LANG"
	perfScanEnvLangs       = "GTS_PERF_SCAN_LANGS"
	perfScanEnvOut         = "GTS_PERF_SCAN_OUT"
	perfScanEnvCorpusRoot  = "GTS_PERF_SCAN_CORPUS_ROOT"
	perfScanEnvReps        = "GTS_PERF_SCAN_REPS"
	perfScanEnvWarmup      = "GTS_PERF_SCAN_WARMUP"
	perfScanEnvFileBudget  = "GTS_PERF_SCAN_FILE_BUDGET_MS"
	perfScanEnvLangTimeout = "GTS_PERF_SCAN_LANG_TIMEOUT_MS"
	perfScanEnvMaxFiles    = "GTS_PERF_SCAN_MAX_FILES"
	perfScanEnvMinBytes    = "GTS_PERF_SCAN_MIN_FILE_BYTES"
	perfScanEnvMaxBytes    = "GTS_PERF_SCAN_MAX_FILE_BYTES"
	perfScanEnvExclude     = "GTS_PERF_SCAN_EXCLUDE_PATHS"
	perfScanEnvOrder       = "GTS_PERF_SCAN_ORDER"
	perfScanEnvAxes        = "GTS_PERF_SCAN_AXES"
	perfScanEnvContended   = "GTS_PERF_SCAN_CONTENDED"
	perfScanEnvInProcess   = "GTS_PERF_SCAN_INPROCESS"
	perfScanEnvEditCands   = "GTS_PERF_SCAN_EDIT_CANDIDATES"
	perfScanEnvChildRSSMB  = "GTS_PERF_SCAN_CHILD_RSS_LIMIT_MB"

	perfScanAxisFull   = "full"
	perfScanAxisNoEdit = "noedit"
	perfScanAxisEdit   = "edit"

	perfScanBucketLe12   = "<=1.2x"
	perfScanBucketLe2    = "<=2x"
	perfScanBucketGt2    = ">2x"
	perfScanBucketCliff  = "cliff>10x"
	perfScanBucketNoData = "n/a"

	perfScanStatusOK      = "ok"
	perfScanStatusRunning = "running"
)

type perfScanConfig struct {
	CorpusRoot    string   `json:"corpus_root"`
	Reps          int      `json:"reps"`
	Warmup        int      `json:"warmup"`
	FileBudgetMS  int      `json:"file_budget_ms"`
	LangTimeoutMS int      `json:"lang_timeout_ms"`
	MaxFiles      int      `json:"max_files"`
	MinFileBytes  int      `json:"min_file_bytes"`
	MaxFileBytes  int      `json:"max_file_bytes"`
	ExcludePaths  []string `json:"exclude_paths,omitempty"`
	Order         string   `json:"order"`
	Axes          []string `json:"axes"`
	Contended     bool     `json:"contended"`
	ContendedNote string   `json:"contended_note,omitempty"`
	ChildRSSMB    int      `json:"child_rss_limit_mb,omitempty"`
}

type perfScanHost struct {
	Hostname     string `json:"hostname"`
	GOOS         string `json:"goos"`
	GOARCH       string `json:"goarch"`
	NumCPU       int    `json:"num_cpu"`
	GoVersion    string `json:"go_version"`
	LoadavgStart string `json:"loadavg_start,omitempty"`
	LoadavgEnd   string `json:"loadavg_end,omitempty"`
}

type perfScanFileAxis struct {
	Status            string  `json:"status"`
	Detail            string  `json:"detail,omitempty"`
	GoMedianNs        int64   `json:"go_median_ns,omitempty"`
	CMedianNs         int64   `json:"c_median_ns,omitempty"`
	GoMinNs           int64   `json:"go_min_ns,omitempty"`
	GoMaxNs           int64   `json:"go_max_ns,omitempty"`
	CMinNs            int64   `json:"c_min_ns,omitempty"`
	CMaxNs            int64   `json:"c_max_ns,omitempty"`
	Ratio             float64 `json:"ratio,omitempty"`
	RatioIsLowerBound bool    `json:"ratio_is_lower_bound,omitempty"`
	Verdict           string  `json:"verdict,omitempty"`
}

type perfScanFile struct {
	Path  string                       `json:"path"`
	Bytes int                          `json:"bytes"`
	Axes  map[string]*perfScanFileAxis `json:"axes"`
}

type perfScanLangAxis struct {
	FilesOK            int     `json:"files_ok"`
	GoTotalNs          int64   `json:"go_total_ns"`
	CTotalNs           int64   `json:"c_total_ns"`
	RatioByTotal       float64 `json:"ratio_by_total,omitempty"`
	RatioMedianOfFiles float64 `json:"ratio_median_of_files,omitempty"`
	Cliffs             int     `json:"cliffs"`
	GoTimeouts         int     `json:"go_timeouts"`
	Verdict            string  `json:"verdict"`
}

type perfScanLanguage struct {
	Language      string `json:"language"`
	Status        string `json:"status"`
	Detail        string `json:"detail,omitempty"`
	Backend       string `json:"backend,omitempty"`
	FilesSelected int    `json:"files_selected"`
	FilesMeasured int    `json:"files_measured"`
	BytesMeasured int64  `json:"bytes_measured"`
	ElapsedMS     int64  `json:"elapsed_ms"`
	Verdict       string `json:"verdict"`
	// ActiveFile is the canonical active-measurement signal. The numeric
	// fields are pointers so active zero-byte files still serialize bytes=0.
	ActiveFile       string                       `json:"active_file,omitempty"`
	ActiveFileIndex  *int                         `json:"active_file_index,omitempty"`
	ActiveFileBytes  *int64                       `json:"active_file_bytes,omitempty"`
	ActiveAxis       string                       `json:"active_axis,omitempty"`
	ActiveImpl       string                       `json:"active_impl,omitempty"`
	ActivePhase      string                       `json:"active_phase,omitempty"`
	ActiveAttempt    *int                         `json:"active_attempt,omitempty"`
	Axes             map[string]*perfScanLangAxis `json:"axes,omitempty"`
	Notes            []string                     `json:"notes,omitempty"`
	Files            []*perfScanFile              `json:"files,omitempty"`
	activeFileDetail string
}

type perfScanScoreboard struct {
	Schema      string              `json:"schema"`
	GeneratedAt string              `json:"generated_at"`
	Host        perfScanHost        `json:"host"`
	Config      perfScanConfig      `json:"config"`
	Notes       []string            `json:"notes,omitempty"`
	Summary     map[string]int      `json:"summary_verdicts"`
	Languages   []*perfScanLanguage `json:"languages"`
}

func perfScanGateEnabled() bool {
	return parityEnvBool(perfScanEnvGate, false)
}

func perfScanEnvIntDefault(name string, def int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return def
	}
	return n
}

func perfScanLoadConfig() perfScanConfig {
	cfg := perfScanConfig{
		CorpusRoot:    perfScanCorpusRoot(),
		Reps:          perfScanEnvIntDefault(perfScanEnvReps, 5),
		Warmup:        perfScanEnvIntDefault(perfScanEnvWarmup, 1),
		FileBudgetMS:  perfScanEnvIntDefault(perfScanEnvFileBudget, 5000),
		LangTimeoutMS: perfScanEnvIntDefault(perfScanEnvLangTimeout, 10*60*1000),
		MaxFiles:      perfScanEnvIntDefault(perfScanEnvMaxFiles, 16),
		MinFileBytes:  perfScanEnvIntDefault(perfScanEnvMinBytes, 0),
		MaxFileBytes:  perfScanEnvIntDefault(perfScanEnvMaxBytes, 4<<20),
		ExcludePaths:  perfScanPathList(os.Getenv(perfScanEnvExclude)),
		Order:         strings.TrimSpace(os.Getenv(perfScanEnvOrder)),
		Axes:          perfScanAxes(),
		ChildRSSMB:    perfScanEnvIntDefault(perfScanEnvChildRSSMB, 0),
	}
	if cfg.Reps < 1 {
		cfg.Reps = 1
	}
	if cfg.Order == "" {
		cfg.Order = "largest"
	}
	cfg.Contended, cfg.ContendedNote = perfScanContended()
	return cfg
}

func perfScanAxes() []string {
	raw := strings.TrimSpace(os.Getenv(perfScanEnvAxes))
	if raw == "" {
		return []string{perfScanAxisFull, perfScanAxisNoEdit}
	}
	var axes []string
	for _, part := range strings.Split(raw, ",") {
		axis := strings.ToLower(strings.TrimSpace(part))
		switch axis {
		case perfScanAxisFull, perfScanAxisNoEdit, perfScanAxisEdit:
			axes = append(axes, axis)
		}
	}
	if len(axes) == 0 {
		return []string{perfScanAxisFull, perfScanAxisNoEdit}
	}
	return axes
}

func perfScanPathList(raw string) []string {
	seen := map[string]bool{}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.ReplaceAll(strings.TrimSpace(part), "\\", "/")
		part = strings.TrimPrefix(part, "./")
		part = strings.TrimPrefix(part, "/")
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	sort.Strings(out)
	return out
}

func perfScanPathExcluded(lang, rel string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	rel = filepath.ToSlash(strings.TrimPrefix(rel, "./"))
	langRel := lang + "/" + rel
	for _, pattern := range patterns {
		if perfScanPathPatternMatches(pattern, rel) || perfScanPathPatternMatches(pattern, langRel) {
			return true
		}
	}
	return false
}

func perfScanPathPatternMatches(pattern, candidate string) bool {
	pattern = strings.ReplaceAll(strings.TrimSpace(pattern), "\\", "/")
	pattern = strings.TrimPrefix(pattern, "./")
	pattern = strings.TrimPrefix(pattern, "/")
	candidate = strings.ReplaceAll(strings.TrimSpace(candidate), "\\", "/")
	candidate = strings.TrimPrefix(candidate, "./")
	candidate = strings.Trim(candidate, "/")
	if pattern == "" {
		return false
	}
	if pattern == candidate {
		return true
	}
	if strings.HasSuffix(pattern, "/") && strings.HasPrefix(candidate, pattern) {
		return true
	}
	if strings.ContainsAny(pattern, "*?[") {
		if ok, err := path.Match(pattern, candidate); err == nil && ok {
			return true
		}
	}
	return false
}

func perfScanCorpusRoot() string {
	if root := strings.TrimSpace(os.Getenv(perfScanEnvCorpusRoot)); root != "" {
		return root
	}
	if root := strings.TrimSpace(os.Getenv("GTS_REAL_CORPUS_BENCH_ROOT")); root != "" {
		return root
	}
	for _, candidate := range []string{
		"corpus_real",
		filepath.Join("cgo_harness", "corpus_real"),
		filepath.Join("..", "cgo_harness", "corpus_real"),
	} {
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate
		}
	}
	return "corpus_real"
}

func perfScanContended() (bool, string) {
	raw := strings.TrimSpace(os.Getenv(perfScanEnvContended))
	if raw != "" {
		return parityEnvBool(perfScanEnvContended, false), "explicit " + perfScanEnvContended + "=" + raw
	}
	load1, ok := perfScanLoadavg1()
	if !ok {
		return false, ""
	}
	threshold := float64(runtime.NumCPU()) / 4
	if threshold < 2 {
		threshold = 2
	}
	if load1 >= threshold {
		return true, fmt.Sprintf("auto-detected: loadavg1=%.2f >= %.2f", load1, threshold)
	}
	return false, ""
}

func perfScanLoadavg1() (float64, bool) {
	raw := perfScanReadLoadavg()
	if raw == "" {
		return 0, false
	}
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return 0, false
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func perfScanReadLoadavg() string {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func perfScanVerdictBucket(ratio float64) string {
	switch {
	case ratio <= 0:
		return perfScanBucketNoData
	case ratio <= 1.2:
		return perfScanBucketLe12
	case ratio <= 2:
		return perfScanBucketLe2
	case ratio <= 10:
		return perfScanBucketGt2
	default:
		return perfScanBucketCliff
	}
}

func perfScanMedianNs(samples []int64) int64 {
	if len(samples) == 0 {
		return 0
	}
	sorted := append([]int64(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

func perfScanMedianFloat(samples []float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	sorted := append([]float64(nil), samples...)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

func perfScanMinMaxNs(samples []int64) (int64, int64) {
	if len(samples) == 0 {
		return 0, 0
	}
	minV, maxV := samples[0], samples[0]
	for _, s := range samples[1:] {
		if s < minV {
			minV = s
		}
		if s > maxV {
			maxV = s
		}
	}
	return minV, maxV
}

// ---------------------------------------------------------------------------
// Child: measure one language.
// ---------------------------------------------------------------------------

// TestPerfScanLanguage measures a single language (GTS_PERF_SCAN_LANG) and
// writes a per-language JSON fragment into GTS_PERF_SCAN_OUT/langs/. It is
// normally invoked as a subprocess by TestPerfScanSweep so that a hard hang or
// a native crash in one grammar cannot take down the whole sweep.
func TestPerfScanLanguage(t *testing.T) {
	if !perfScanGateEnabled() {
		t.Skipf("set %s=1 to run the perf scan", perfScanEnvGate)
	}
	lang := strings.TrimSpace(os.Getenv(perfScanEnvLang))
	if lang == "" {
		t.Skipf("set %s to a language name (child mode)", perfScanEnvLang)
	}
	outDir := strings.TrimSpace(os.Getenv(perfScanEnvOut))
	if outDir == "" {
		t.Fatalf("%s must be set in child mode", perfScanEnvOut)
	}
	cfg := perfScanLoadConfig()
	row := perfScanMeasureLanguage(t, lang, cfg, func(partial *perfScanLanguage) {
		if err := perfScanWriteLangFragment(outDir, partial); err != nil {
			t.Logf("write partial fragment: %v", err)
		}
	})
	if err := perfScanWriteLangFragment(outDir, row); err != nil {
		t.Fatalf("write language fragment: %v", err)
	}
	t.Logf("perf scan %s: status=%s verdict=%s files=%d/%d elapsed=%dms",
		row.Language, row.Status, row.Verdict, row.FilesMeasured, row.FilesSelected, row.ElapsedMS)
}

func perfScanWriteLangFragment(outDir string, row *perfScanLanguage) error {
	dir := filepath.Join(outDir, "langs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(row, "", "  ")
	if err != nil {
		return err
	}
	final := filepath.Join(dir, paritySafeName(row.Language)+".json")
	tmp := fmt.Sprintf("%s.tmp.%d", final, os.Getpid())
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

func perfScanSetActiveFile(row *perfScanLanguage, file perfScanCorpusFile, index, total int) {
	activeFileIndex := index
	activeFileBytes := file.size
	row.ActiveFile = file.rel
	row.ActiveFileIndex = &activeFileIndex
	row.ActiveFileBytes = &activeFileBytes
	row.activeFileDetail = fmt.Sprintf("measuring file %d/%d: %s (%d bytes)", index, total, file.rel, file.size)
	row.Detail = row.activeFileDetail
}

func perfScanSetActiveAttempt(row *perfScanLanguage, axis, impl, phase string, attempt int) {
	row.ActiveAxis = axis
	row.ActiveImpl = impl
	row.ActivePhase = phase
	if attempt > 0 {
		activeAttempt := attempt
		row.ActiveAttempt = &activeAttempt
	} else {
		row.ActiveAttempt = nil
	}

	attemptDetail := fmt.Sprintf("%s/%s/%s", axis, impl, phase)
	if attempt > 0 {
		attemptDetail = fmt.Sprintf("%s attempt %d", attemptDetail, attempt)
	}
	if row.activeFileDetail != "" {
		row.Detail = row.activeFileDetail + "; " + attemptDetail
	} else if row.Detail != "" {
		row.Detail += "; " + attemptDetail
	} else {
		row.Detail = "measuring " + attemptDetail
	}
}

func perfScanClearActiveAttempt(row *perfScanLanguage) {
	row.ActiveAxis = ""
	row.ActiveImpl = ""
	row.ActivePhase = ""
	row.ActiveAttempt = nil
	if row.activeFileDetail != "" {
		row.Detail = row.activeFileDetail
	}
}

// perfScanClearActiveFile resets active-file tracking and its file-progress
// detail message.
func perfScanClearActiveFile(row *perfScanLanguage) {
	perfScanClearActiveAttempt(row)
	row.ActiveFile = ""
	row.ActiveFileIndex = nil
	row.ActiveFileBytes = nil
	row.activeFileDetail = ""
	row.Detail = ""
}

func perfScanMeasureLanguage(t *testing.T, lang string, cfg perfScanConfig, flush func(*perfScanLanguage)) *perfScanLanguage {
	start := time.Now()
	row := &perfScanLanguage{
		Language: lang,
		Status:   perfScanStatusRunning,
		Verdict:  perfScanBucketNoData,
		Axes:     map[string]*perfScanLangAxis{},
	}
	finish := func(status, detail string) *perfScanLanguage {
		row.Status = status
		row.Detail = detail
		row.ElapsedMS = time.Since(start).Milliseconds()
		return row
	}

	if parityLanguageExcluded(lang) {
		return finish("excluded", "excluded by GTS_PARITY_SKIP_LANGS")
	}
	entry, ok := parityEntriesByName[lang]
	if !ok {
		return finish("no_registry_entry", "language not present in grammars registry")
	}
	report, ok := paritySupportByName[lang]
	if !ok || report.Backend == grammars.ParseBackendUnsupported {
		return finish("unsupported_backend", fmt.Sprintf("parse backend %q", report.Backend))
	}
	row.Backend = string(report.Backend)
	if reason := paritySkipReason(lang); reason != "" {
		row.Notes = append(row.Notes, "known structural mismatch (timed anyway): "+reason)
	}

	langRoot := realCorpusBenchmarkLanguageRoot(t, cfg.CorpusRoot, lang)
	if st, err := os.Stat(langRoot); err != nil || !st.IsDir() {
		return finish("no_corpus", fmt.Sprintf("no corpus directory at %s", langRoot))
	}
	files, err := perfScanSelectFiles(t, lang, cfg, langRoot)
	if err != nil {
		return finish("no_corpus_files", err.Error())
	}
	row.FilesSelected = len(files)
	if flush != nil {
		row.ElapsedMS = time.Since(start).Milliseconds()
		flush(row)
	}

	cLang, err := ParityCLanguage(lang)
	if err != nil {
		if skipReason := parityReferenceSkipReason(err); skipReason != "" {
			return finish("no_c_reference", "known C reference skip: "+skipReason)
		}
		return finish("no_c_reference", fmt.Sprintf("load C parser: %v", err))
	}

	goLang := entry.Language()
	if goLang == nil {
		return finish("error", "grammars registry returned nil Go language")
	}

	m := &perfScanLangMeasurer{
		cfg:     cfg,
		lang:    lang,
		entry:   entry,
		report:  report,
		goLang:  goLang,
		cLang:   cLang,
		budget:  time.Duration(cfg.FileBudgetMS) * time.Millisecond,
		goPsr:   gotreesitter.NewParser(goLang),
		editMax: perfScanEnvIntDefault(perfScanEnvEditCands, 16),
	}
	m.goPsr.SetTimeoutMicros(uint64(m.budget.Microseconds()))
	cParser := sitter.NewParser()
	if err := cParser.SetLanguage(cLang); err != nil {
		cParser.Close()
		return finish("no_c_reference", fmt.Sprintf("C SetLanguage: %v", err))
	}
	cParser.SetTimeoutMicros(uint64(m.budget.Microseconds()))
	m.cPsr = cParser
	defer cParser.Close()

	for i, file := range files {
		perfScanSetActiveFile(row, file, i+1, len(files))
		if flush != nil {
			row.ElapsedMS = time.Since(start).Milliseconds()
			flush(row)
		}
		src, err := os.ReadFile(file.path)
		if err != nil {
			row.Notes = append(row.Notes, fmt.Sprintf("read %s: %v", file.rel, err))
			perfScanClearActiveFile(row)
			row.Detail = fmt.Sprintf("read error on file %d/%d: %s: %v", i+1, len(files), file.rel, err)
			if flush != nil {
				row.ElapsedMS = time.Since(start).Milliseconds()
				flush(row)
			}
			continue
		}
		fileRow := &perfScanFile{
			Path:  file.rel,
			Bytes: len(src),
			Axes:  map[string]*perfScanFileAxis{},
		}
		m.progress = nil
		if flush != nil {
			fileIndex := i + 1
			fileTotal := len(files)
			activeFile := file
			m.progress = func(axis, impl, phase string, attempt int) {
				perfScanSetActiveFile(row, activeFile, fileIndex, fileTotal)
				perfScanSetActiveAttempt(row, axis, impl, phase, attempt)
				row.ElapsedMS = time.Since(start).Milliseconds()
				flush(row)
			}
		}
		for _, axis := range cfg.Axes {
			fileRow.Axes[axis] = m.measureFileAxis(axis, src)
		}
		row.Files = append(row.Files, fileRow)
		row.FilesMeasured++
		row.BytesMeasured += int64(len(src))
		perfScanClearActiveFile(row)
		if flush != nil {
			row.ElapsedMS = time.Since(start).Milliseconds()
			flush(row)
		}
	}

	perfScanAggregateLanguage(row, cfg)
	return finish(perfScanStatusOK, "")
}

type perfScanCorpusFile struct {
	path string
	rel  string
	size int64
}

func perfScanSelectFiles(t *testing.T, lang string, cfg perfScanConfig, langRoot string) ([]perfScanCorpusFile, error) {
	filters := realCorpusBenchmarkFileFiltersFor(t, lang, cfg.CorpusRoot)
	var all []perfScanCorpusFile
	err := filepath.WalkDir(langRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".gradle", "bazel-bin", "bazel-out", "bazel-testlogs", "build", "node_modules", "target":
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		rel := path
		if r, err := filepath.Rel(langRoot, path); err == nil {
			rel = r
		}
		rel = filepath.ToSlash(rel)
		if !realCorpusBenchmarkFileAllowed(rel, filters) {
			return nil
		}
		if perfScanPathExcluded(lang, rel, cfg.ExcludePaths) {
			return nil
		}
		size := info.Size()
		if cfg.MinFileBytes > 0 && size < int64(cfg.MinFileBytes) {
			return nil
		}
		if cfg.MaxFileBytes > 0 && size > int64(cfg.MaxFileBytes) {
			return nil
		}
		all = append(all, perfScanCorpusFile{path: path, rel: rel, size: size})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %v", langRoot, err)
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("no corpus files matched under %s", langRoot)
	}
	switch cfg.Order {
	case "path":
		sort.Slice(all, func(i, j int) bool { return all[i].rel < all[j].rel })
	case "smallest":
		sort.Slice(all, func(i, j int) bool {
			if all[i].size != all[j].size {
				return all[i].size < all[j].size
			}
			return all[i].rel < all[j].rel
		})
	default: // largest
		sort.Slice(all, func(i, j int) bool {
			if all[i].size != all[j].size {
				return all[i].size > all[j].size
			}
			return all[i].rel < all[j].rel
		})
	}
	if cfg.MaxFiles > 0 && len(all) > cfg.MaxFiles {
		all = all[:cfg.MaxFiles]
	}
	// Deterministic final ordering by path within the selected set.
	sort.Slice(all, func(i, j int) bool { return all[i].rel < all[j].rel })
	return all, nil
}

// ---------------------------------------------------------------------------
// Measurement core.
// ---------------------------------------------------------------------------

type perfScanLangMeasurer struct {
	cfg      perfScanConfig
	lang     string
	entry    grammars.LangEntry
	report   grammars.ParseSupport
	goLang   *gotreesitter.Language
	cLang    *sitter.Language
	goPsr    *gotreesitter.Parser
	cPsr     *sitter.Parser
	budget   time.Duration
	editMax  int
	progress func(axis, impl, phase string, attempt int)
}

type perfScanAttempt struct {
	ns     int64
	status string // "" == ok
	detail string
}

func (m *perfScanLangMeasurer) benchCase(src []byte) realCorpusBenchmarkCase {
	return realCorpusBenchmarkCase{
		name:   m.lang,
		path:   m.lang,
		source: src,
		entry:  m.entry,
		report: m.report,
		goLang: m.goLang,
		cLang:  m.cLang,
	}
}

func (m *perfScanLangMeasurer) checkpoint(axis, impl, phase string, attempt int) {
	if m.progress != nil {
		m.progress(axis, impl, phase, attempt)
	}
}

// goAttemptFull runs one timed Go full parse. The returned tree is nil unless
// the parse completed cleanly.
func (m *perfScanLangMeasurer) goAttemptFull(src []byte, keepTree bool) (*gotreesitter.Tree, perfScanAttempt) {
	var tree *gotreesitter.Tree
	var err error
	att := perfScanAttempt{}
	panicked := perfScanRecover(func() {
		start := time.Now()
		switch m.report.Backend {
		case grammars.ParseBackendTokenSource:
			if m.entry.TokenSourceFactory == nil {
				err = fmt.Errorf("token source backend without factory")
				return
			}
			tree, err = m.goPsr.ParseWithTokenSource(src, m.entry.TokenSourceFactory(src, m.goLang))
		default:
			tree, err = m.goPsr.Parse(src)
		}
		att.ns = time.Since(start).Nanoseconds()
	})
	return m.classifyGoAttempt(tree, err, panicked, src, keepTree, att)
}

func (m *perfScanLangMeasurer) goAttemptIncremental(src []byte, oldTree *gotreesitter.Tree, keepTree bool) (*gotreesitter.Tree, perfScanAttempt) {
	var tree *gotreesitter.Tree
	var err error
	att := perfScanAttempt{}
	panicked := perfScanRecover(func() {
		start := time.Now()
		switch m.report.Backend {
		case grammars.ParseBackendTokenSource:
			if m.entry.TokenSourceFactory == nil {
				err = fmt.Errorf("token source backend without factory")
				return
			}
			tree, err = m.goPsr.ParseIncrementalWithTokenSource(src, oldTree, m.entry.TokenSourceFactory(src, m.goLang))
		default:
			tree, err = m.goPsr.ParseIncremental(src, oldTree)
		}
		att.ns = time.Since(start).Nanoseconds()
	})
	return m.classifyGoAttempt(tree, err, panicked, src, keepTree, att)
}

func (m *perfScanLangMeasurer) classifyGoAttempt(tree *gotreesitter.Tree, err error, panicked string, src []byte, keepTree bool, att perfScanAttempt) (*gotreesitter.Tree, perfScanAttempt) {
	if panicked != "" {
		att.status = "go_panic"
		att.detail = panicked
		releaseGoTree(tree)
		return nil, att
	}
	if err != nil {
		att.status = "go_error"
		att.detail = err.Error()
		releaseGoTree(tree)
		return nil, att
	}
	if tree == nil || tree.RootNode() == nil {
		att.status = "go_error"
		att.detail = "nil tree"
		releaseGoTree(tree)
		return nil, att
	}
	if tree.ParseStoppedEarly() {
		att.status = "go_timeout"
		att.detail = fmt.Sprintf("parse stopped early (%v) at file budget %s", tree.ParseStopReason(), m.budget)
		releaseGoTree(tree)
		return nil, att
	}
	// Coverage + internal-consistency verdict (wave2b): status=ok must mean the
	// result tree COVERS the input AND its runtime error signals are self-
	// consistent — not merely "finished within wall-clock". No C oracle: these
	// are pure internal-consistency checks on the returned tree.
	root := tree.RootNode()
	rt := tree.ParseRuntime()
	got, want := root.EndByte(), uint32(len(src))
	if got != want {
		// The result tree does not span the input. Always a failure, but flag
		// whether the truncation is SILENT (the wave2b class: no reliable error
		// signal at all) vs. honestly flagged. The library contract is that a
		// truncated returned tree carries Truncated=true OR root.HasError().
		signal := "flagged"
		if !rt.Truncated && !root.HasError() {
			signal = "SILENT"
		}
		att.status = "go_error"
		att.detail = fmt.Sprintf("truncated[%s]: root.EndByte=%d want=%d hasErr=%v Truncated=%v",
			signal, got, want, root.HasError(), rt.Truncated)
		releaseGoTree(tree)
		return nil, att
	}
	// The root spans the input from here on. Enforce consistency so a covered
	// tree cannot masquerade as clean.
	if rt.Truncated {
		// Coverage and the Truncated flag disagree: an internal inconsistency.
		att.status = "go_error"
		att.detail = fmt.Sprintf("inconsistent: root covers %d but ParseRuntime.Truncated=true", got)
		releaseGoTree(tree)
		return nil, att
	}
	if root.IsError() {
		// A full-span ERROR root is a degenerate parse (whole input recovered as
		// one ERROR node); 'ok' used to mask this (the webworker case).
		att.status = "go_error"
		att.detail = "error_root: root symbol is ERROR spanning the input"
		releaseGoTree(tree)
		return nil, att
	}
	if !keepTree {
		releaseGoTree(tree)
		return nil, att
	}
	return tree, att
}

func (m *perfScanLangMeasurer) cAttempt(src []byte, oldTree *sitter.Tree, keepTree bool) (*sitter.Tree, perfScanAttempt) {
	att := perfScanAttempt{}
	start := time.Now()
	tree := m.cPsr.Parse(src, oldTree)
	att.ns = time.Since(start).Nanoseconds()
	if tree == nil {
		// The C API returns a nil tree when the timeout fires; the parser must
		// be reset before it can parse a different document.
		m.cPsr.Reset()
		att.status = "c_timeout"
		att.detail = fmt.Sprintf("nil tree (halted at file budget %s)", m.budget)
		return nil, att
	}
	if !isCompleteRealCorpusCTree(tree, src) {
		att.status = "c_error"
		att.detail = "truncated C tree"
		tree.Close()
		return nil, att
	}
	if !keepTree {
		tree.Close()
		return nil, att
	}
	return tree, att
}

func perfScanRecover(fn func()) (panicked string) {
	defer func() {
		if r := recover(); r != nil {
			panicked = fmt.Sprintf("panic: %v", r)
		}
	}()
	fn()
	return ""
}

func (m *perfScanLangMeasurer) measureFileAxis(axis string, src []byte) *perfScanFileAxis {
	switch axis {
	case perfScanAxisFull:
		return m.measureFull(src)
	case perfScanAxisNoEdit:
		return m.measureNoEdit(src)
	case perfScanAxisEdit:
		return m.measureEdit(src)
	default:
		return &perfScanFileAxis{Status: "skipped", Detail: "unknown axis " + axis}
	}
}

func (m *perfScanLangMeasurer) measureFull(src []byte) *perfScanFileAxis {
	out := &perfScanFileAxis{Status: perfScanStatusOK}

	goOK := true
	var goDetail string
	for i := 0; i < m.cfg.Warmup; i++ {
		m.checkpoint(perfScanAxisFull, "go", "warmup", i+1)
		_, att := m.goAttemptFull(src, false)
		if att.status != "" {
			goOK = false
			out.Status = att.status
			goDetail = att.detail
			break
		}
	}
	cOK := true
	for i := 0; i < m.cfg.Warmup; i++ {
		m.checkpoint(perfScanAxisFull, "c", "warmup", i+1)
		_, att := m.cAttempt(src, nil, false)
		if att.status != "" {
			cOK = false
			if out.Status == perfScanStatusOK {
				out.Status = att.status
			}
			out.Detail = strings.TrimSpace(out.Detail + " " + att.detail)
			break
		}
	}

	var goSamples, cSamples []int64
	for i := 0; i < m.cfg.Reps; i++ {
		if goOK {
			m.checkpoint(perfScanAxisFull, "go", "rep", i+1)
			_, att := m.goAttemptFull(src, false)
			if att.status != "" {
				goOK = false
				out.Status = att.status
				goDetail = att.detail
			} else {
				goSamples = append(goSamples, att.ns)
			}
		}
		if cOK {
			m.checkpoint(perfScanAxisFull, "c", "rep", i+1)
			_, att := m.cAttempt(src, nil, false)
			if att.status != "" {
				cOK = false
				if out.Status == perfScanStatusOK {
					out.Status = att.status
				}
				out.Detail = strings.TrimSpace(out.Detail + " " + att.detail)
			} else {
				cSamples = append(cSamples, att.ns)
			}
		}
	}
	if goDetail != "" {
		out.Detail = strings.TrimSpace(goDetail + " " + out.Detail)
	}
	perfScanFillAxis(out, goSamples, cSamples, goOK, cOK, m.budget)
	return out
}

func (m *perfScanLangMeasurer) measureNoEdit(src []byte) *perfScanFileAxis {
	out := &perfScanFileAxis{Status: perfScanStatusOK}

	// Go side: base full parse (untimed sample), then timed no-edit reparses.
	m.checkpoint(perfScanAxisNoEdit, "go", "base", 1)
	goTree, baseAtt := m.goAttemptFull(src, true)
	goOK := baseAtt.status == ""
	var goSamples []int64
	if !goOK {
		out.Status = baseAtt.status
		out.Detail = "base full parse: " + baseAtt.detail
	} else {
		for i := 0; i < m.cfg.Reps; i++ {
			m.checkpoint(perfScanAxisNoEdit, "go", "rep", i+1)
			newTree, att := m.goAttemptIncremental(src, goTree, true)
			if att.status != "" {
				goOK = false
				out.Status = att.status
				out.Detail = strings.TrimSpace(out.Detail + " " + att.detail)
				break
			}
			goSamples = append(goSamples, att.ns)
			if newTree != goTree {
				releaseGoTree(goTree)
			}
			goTree = newTree
		}
	}
	releaseGoTree(goTree)

	// C side: base full parse, then timed no-edit reparses with the old tree.
	m.checkpoint(perfScanAxisNoEdit, "c", "base", 1)
	cTree, cBaseAtt := m.cAttempt(src, nil, true)
	cOK := cBaseAtt.status == ""
	var cSamples []int64
	if !cOK {
		if out.Status == perfScanStatusOK {
			out.Status = cBaseAtt.status
		}
		out.Detail = strings.TrimSpace(out.Detail + " C base: " + cBaseAtt.detail)
	} else {
		for i := 0; i < m.cfg.Reps; i++ {
			m.checkpoint(perfScanAxisNoEdit, "c", "rep", i+1)
			newTree, att := m.cAttempt(src, cTree, true)
			if att.status != "" {
				cOK = false
				if out.Status == perfScanStatusOK {
					out.Status = att.status
				}
				out.Detail = strings.TrimSpace(out.Detail + " " + att.detail)
				break
			}
			cSamples = append(cSamples, att.ns)
			if newTree != cTree {
				cTree.Close()
			}
			cTree = newTree
		}
	}
	if cTree != nil {
		cTree.Close()
	}

	perfScanFillAxis(out, goSamples, cSamples, goOK, cOK, m.budget)
	return out
}

func (m *perfScanLangMeasurer) measureEdit(src []byte) *perfScanFileAxis {
	out := &perfScanFileAxis{Status: perfScanStatusOK}
	tc := m.benchCase(src)

	editCase, ok := m.findEditCase(tc)
	if !ok {
		out.Status = "no_edit_site"
		out.Detail = "no verified single-byte replacement site"
		return out
	}

	// Go side.
	goSrc := append([]byte(nil), src...)
	m.checkpoint(perfScanAxisEdit, "go", "base", 1)
	goTree, baseAtt := m.goAttemptFull(goSrc, true)
	goOK := baseAtt.status == ""
	var goSamples []int64
	if !goOK {
		out.Status = baseAtt.status
		out.Detail = "base full parse: " + baseAtt.detail
	} else {
		for i := 0; i < m.cfg.Reps; i++ {
			m.checkpoint(perfScanAxisEdit, "go", "tree_edit", i+1)
			toggleRealCorpusEditByte(goSrc, editCase)
			goTree.Edit(editCase.goEdit)
			m.checkpoint(perfScanAxisEdit, "go", "rep", i+1)
			newTree, att := m.goAttemptIncremental(goSrc, goTree, true)
			if att.status != "" {
				goOK = false
				out.Status = att.status
				out.Detail = strings.TrimSpace(out.Detail + " " + att.detail)
				break
			}
			goSamples = append(goSamples, att.ns)
			if newTree != goTree {
				releaseGoTree(goTree)
			}
			goTree = newTree
		}
	}
	releaseGoTree(goTree)

	// C side.
	cSrc := append([]byte(nil), src...)
	m.checkpoint(perfScanAxisEdit, "c", "base", 1)
	cTree, cBaseAtt := m.cAttempt(cSrc, nil, true)
	cOK := cBaseAtt.status == ""
	var cSamples []int64
	if !cOK {
		if out.Status == perfScanStatusOK {
			out.Status = cBaseAtt.status
		}
		out.Detail = strings.TrimSpace(out.Detail + " C base: " + cBaseAtt.detail)
	} else {
		cState := realCorpusCIncrementalState{tc: editCase, src: cSrc, tree: cTree}
		for i := 0; i < m.cfg.Reps; i++ {
			m.checkpoint(perfScanAxisEdit, "c", "tree_edit", i+1)
			toggleRealCorpusEditByte(cState.src, cState.tc)
			cState.tree.Edit(&cState.tc.cEdit)
			m.checkpoint(perfScanAxisEdit, "c", "rep", i+1)
			newTree, att := m.cAttempt(cState.src, cState.tree, true)
			if att.status != "" {
				cOK = false
				if out.Status == perfScanStatusOK {
					out.Status = att.status
				}
				out.Detail = strings.TrimSpace(out.Detail + " " + att.detail)
				break
			}
			cSamples = append(cSamples, att.ns)
			if newTree != cState.tree {
				cState.tree.Close()
			}
			cState.tree = newTree
		}
		cTree = cState.tree
	}
	if cTree != nil {
		cTree.Close()
	}

	perfScanFillAxis(out, goSamples, cSamples, goOK, cOK, m.budget)
	if out.Status == perfScanStatusOK {
		out.Detail = strings.TrimSpace("edit=" + editCase.label + " " + out.Detail)
	}
	return out
}

// findEditCase picks the first single-byte replacement candidate whose
// incremental reparse completes on both parsers. Structural parity of the
// incremental result is NOT verified here (timing-grade, not
// correctness-grade; the parity suites own correctness).
func (m *perfScanLangMeasurer) findEditCase(tc realCorpusBenchmarkCase) (realCorpusIncrementalCase, bool) {
	tried := 0
	for _, candidate := range incrementalEditCandidates(tc.source) {
		if candidate.oldEnd != candidate.start+1 || len(candidate.replacement) != 1 {
			continue
		}
		if m.editMax > 0 && tried >= m.editMax {
			break
		}
		tried++
		editCase := makeRealCorpusIncrementalCase(tc, candidate)
		edited := applyEditCandidate(tc.source, candidate)

		m.checkpoint(perfScanAxisEdit, "go", "select_base", tried)
		goTree, baseAtt := m.goAttemptFull(tc.source, true)
		if baseAtt.status != "" {
			return realCorpusIncrementalCase{}, false
		}
		m.checkpoint(perfScanAxisEdit, "go", "select_tree_edit", tried)
		goTree.Edit(editCase.goEdit)
		m.checkpoint(perfScanAxisEdit, "go", "select_incremental", tried)
		goIncr, goAtt := m.goAttemptIncremental(edited, goTree, true)
		releaseGoTree(goTree)
		goOK := goAtt.status == ""
		releaseGoTree(goIncr)

		m.checkpoint(perfScanAxisEdit, "c", "select_base", tried)
		cTree, cBaseAtt := m.cAttempt(tc.source, nil, true)
		if cBaseAtt.status != "" {
			return realCorpusIncrementalCase{}, false
		}
		m.checkpoint(perfScanAxisEdit, "c", "select_tree_edit", tried)
		cTree.Edit(&editCase.cEdit)
		m.checkpoint(perfScanAxisEdit, "c", "select_incremental", tried)
		cIncr, cAtt := m.cAttempt(edited, cTree, true)
		cTree.Close()
		cOK := cAtt.status == ""
		if cIncr != nil {
			cIncr.Close()
		}

		if goOK && cOK {
			return editCase, true
		}
	}
	return realCorpusIncrementalCase{}, false
}

// perfScanFillAxis computes medians, ratio, and verdict. When the Go side hit
// the per-file budget the ratio is reported as a lower bound computed from the
// budget, which is how cliffs are surfaced without hanging the sweep.
func perfScanFillAxis(out *perfScanFileAxis, goSamples, cSamples []int64, goOK, cOK bool, budget time.Duration) {
	if len(goSamples) > 0 {
		out.GoMedianNs = perfScanMedianNs(goSamples)
		out.GoMinNs, out.GoMaxNs = perfScanMinMaxNs(goSamples)
	}
	if len(cSamples) > 0 {
		out.CMedianNs = perfScanMedianNs(cSamples)
		out.CMinNs, out.CMaxNs = perfScanMinMaxNs(cSamples)
	}
	switch {
	case goOK && cOK && out.GoMedianNs > 0 && out.CMedianNs > 0:
		out.Ratio = float64(out.GoMedianNs) / float64(out.CMedianNs)
		out.Verdict = perfScanVerdictBucket(out.Ratio)
	case !goOK && strings.HasPrefix(out.Status, "go_timeout") && out.CMedianNs > 0:
		out.Ratio = float64(budget.Nanoseconds()) / float64(out.CMedianNs)
		out.RatioIsLowerBound = true
		out.Verdict = perfScanVerdictBucket(out.Ratio)
	default:
		out.Verdict = perfScanBucketNoData
	}
}

func perfScanAggregateLanguage(row *perfScanLanguage, cfg perfScanConfig) {
	worst := perfScanBucketNoData
	for _, axis := range cfg.Axes {
		agg := &perfScanLangAxis{Verdict: perfScanBucketNoData}
		var ratios []float64
		for _, file := range row.Files {
			fa, ok := file.Axes[axis]
			if !ok {
				continue
			}
			if fa.Status == "go_timeout" {
				agg.GoTimeouts++
			}
			if fa.Verdict == perfScanBucketCliff || (fa.RatioIsLowerBound && fa.Ratio > 10) || fa.Status == "go_timeout" {
				agg.Cliffs++
			}
			if fa.Status == perfScanStatusOK && fa.GoMedianNs > 0 && fa.CMedianNs > 0 {
				agg.FilesOK++
				agg.GoTotalNs += fa.GoMedianNs
				agg.CTotalNs += fa.CMedianNs
				ratios = append(ratios, fa.Ratio)
			}
		}
		if agg.CTotalNs > 0 {
			agg.RatioByTotal = float64(agg.GoTotalNs) / float64(agg.CTotalNs)
		}
		agg.RatioMedianOfFiles = perfScanMedianFloat(ratios)
		switch {
		case agg.Cliffs > 0:
			agg.Verdict = perfScanBucketCliff
		case agg.RatioByTotal > 0:
			agg.Verdict = perfScanVerdictBucket(agg.RatioByTotal)
		}
		row.Axes[axis] = agg
		if axis == perfScanAxisFull {
			worst = agg.Verdict
		}
	}
	// Language verdict: primary axis is full parse; any cliff anywhere escalates.
	for _, agg := range row.Axes {
		if agg.Cliffs > 0 {
			worst = perfScanBucketCliff
		}
	}
	row.Verdict = worst
}

// ---------------------------------------------------------------------------
// Sweep driver.
// ---------------------------------------------------------------------------

// TestPerfScanSweep runs the per-language measurement as isolated subprocesses
// (hard wall-clock kill per language) and merges the per-language fragments
// into perf_scan scoreboard artifacts (scoreboard.json + scoreboard.md).
func TestPerfScanSweep(t *testing.T) {
	if !perfScanGateEnabled() {
		t.Skipf("set %s=1 to run the perf scan sweep", perfScanEnvGate)
	}
	if strings.TrimSpace(os.Getenv(perfScanEnvLang)) != "" {
		t.Skipf("%s is set; refusing to sweep inside a child invocation", perfScanEnvLang)
	}
	cfg := perfScanLoadConfig()

	outDir := strings.TrimSpace(os.Getenv(perfScanEnvOut))
	if outDir == "" {
		outDir = filepath.Join("perf_scan", "out", "scan_"+time.Now().UTC().Format("20060102T150405Z"))
	}
	absOut, err := filepath.Abs(outDir)
	if err != nil {
		t.Fatalf("resolve out dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(absOut, "langs"), 0o755); err != nil {
		t.Fatalf("create out dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(absOut, "logs"), 0o755); err != nil {
		t.Fatalf("create log dir: %v", err)
	}

	langs := perfScanSweepLanguages(t, cfg)
	if len(langs) == 0 {
		t.Fatalf("no languages selected: set %s or provide a corpus root with per-language dirs", perfScanEnvLangs)
	}
	t.Logf("perf scan sweep: %d language(s): %s", len(langs), strings.Join(langs, ","))
	t.Logf("perf scan out dir: %s", absOut)

	board := &perfScanScoreboard{
		Schema:      perfScanSchema,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Config:      cfg,
		Summary:     map[string]int{},
		Host: perfScanHost{
			Hostname:     perfScanHostname(),
			GOOS:         runtime.GOOS,
			GOARCH:       runtime.GOARCH,
			NumCPU:       runtime.NumCPU(),
			GoVersion:    runtime.Version(),
			LoadavgStart: perfScanReadLoadavg(),
		},
	}
	if cfg.Contended {
		board.Notes = append(board.Notes,
			"CONTENDED RUN — smoke-only numbers; box had concurrent load ("+cfg.ContendedNote+"). Re-run on a quiet box for authoritative ratios.")
	}

	inProcess := parityEnvBool(perfScanEnvInProcess, false)
	for _, lang := range langs {
		var row *perfScanLanguage
		if inProcess {
			row = perfScanMeasureLanguage(t, lang, cfg, nil)
		} else {
			row = perfScanRunLanguageSubprocess(t, lang, cfg, absOut)
		}
		board.Languages = append(board.Languages, row)
		board.Summary[row.Verdict]++
		t.Logf("  %-14s status=%-14s verdict=%-9s files=%d/%d elapsed=%dms %s",
			lang, row.Status, row.Verdict, row.FilesMeasured, row.FilesSelected, row.ElapsedMS, row.Detail)
	}
	board.Host.LoadavgEnd = perfScanReadLoadavg()

	if err := perfScanWriteScoreboard(absOut, board); err != nil {
		t.Fatalf("write scoreboard: %v", err)
	}
	t.Logf("scoreboard: %s", filepath.Join(absOut, "scoreboard.json"))
	t.Logf("scoreboard: %s", filepath.Join(absOut, "scoreboard.md"))
}

func perfScanSweepLanguages(t *testing.T, cfg perfScanConfig) []string {
	if raw := strings.TrimSpace(os.Getenv(perfScanEnvLangs)); raw != "" {
		var out []string
		for _, part := range strings.Split(raw, ",") {
			name := strings.TrimSpace(part)
			if name != "" {
				out = append(out, name)
			}
		}
		return out
	}
	entries, err := os.ReadDir(cfg.CorpusRoot)
	if err != nil {
		t.Fatalf("read corpus root %s: %v (set %s)", cfg.CorpusRoot, err, perfScanEnvCorpusRoot)
	}
	var out []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if _, ok := parityEntriesByName[name]; !ok {
			continue
		}
		if parityLanguageExcluded(name) {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func perfScanRunLanguageSubprocess(t *testing.T, lang string, cfg perfScanConfig, absOut string) *perfScanLanguage {
	t.Helper()
	langTimeout := time.Duration(cfg.LangTimeoutMS) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), langTimeout)
	defer cancel()

	self := os.Args[0]
	if !filepath.IsAbs(self) {
		if abs, err := filepath.Abs(self); err == nil {
			self = abs
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	logPath := filepath.Join(absOut, "logs", paritySafeName(lang)+".log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create %s: %v", logPath, err)
	}
	defer logFile.Close()

	cmd := exec.CommandContext(ctx, self,
		"-test.run=^TestPerfScanLanguage$",
		"-test.timeout=0",
		"-test.v=true",
	)
	cmd.Dir = cwd
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = perfScanMergeEnv(os.Environ(), map[string]string{
		perfScanEnvGate: "1",
		perfScanEnvLang: lang,
		perfScanEnvOut:  absOut,
	})

	start := time.Now()
	runErr, rssStop := perfScanRunChild(ctx, cmd, cfg.ChildRSSMB)
	elapsed := time.Since(start)

	fragment, fragErr := perfScanReadLangFragment(absOut, lang)
	timedOut := ctx.Err() == context.DeadlineExceeded

	switch {
	case fragment != nil && fragment.Status == perfScanStatusOK && runErr == nil:
		return fragment
	case fragment != nil && timedOut:
		fragment.Status = "lang_timeout"
		fragment.Detail = strings.TrimSpace(fmt.Sprintf(
			"killed after %s (per-language hard timeout); partial results for %d file(s). %s",
			langTimeout, fragment.FilesMeasured, fragment.Detail))
		fragment.ElapsedMS = elapsed.Milliseconds()
		perfScanAggregateLanguage(fragment, cfg)
		if fragment.Verdict == perfScanBucketNoData {
			fragment.Verdict = perfScanBucketCliff
		}
		return fragment
	case fragment != nil:
		stopDetail := fmt.Sprintf("%v", runErr)
		if rssStop != "" {
			stopDetail = rssStop
		}
		if runErr != nil && fragment.Status == perfScanStatusOK {
			fragment.Notes = append(fragment.Notes, "child exited with error after fragment write: "+stopDetail)
		}
		if fragment.Status == perfScanStatusRunning {
			fragment.Status = "error"
			fragment.Detail = strings.TrimSpace(fmt.Sprintf("child exited early (%s); partial results. %s", stopDetail, fragment.Detail))
			perfScanAggregateLanguage(fragment, cfg)
		}
		return fragment
	default:
		status := "error"
		detail := fmt.Sprintf("child produced no fragment (%v)", runErr)
		if timedOut {
			status = "lang_timeout"
			detail = fmt.Sprintf("killed after %s before any file completed", langTimeout)
		} else if rssStop != "" {
			detail = rssStop + " before any file completed"
		} else if fragErr != nil && runErr == nil {
			detail = fmt.Sprintf("fragment read failed: %v", fragErr)
		}
		if tail := perfScanLogTail(logPath, 3); tail != "" {
			detail += " | log: " + tail
		}
		return &perfScanLanguage{
			Language:  lang,
			Status:    status,
			Detail:    detail,
			Verdict:   perfScanBucketNoData,
			ElapsedMS: elapsed.Milliseconds(),
		}
	}
}

func perfScanRunChild(ctx context.Context, cmd *exec.Cmd, rssLimitMB int) (error, string) {
	if rssLimitMB <= 0 {
		return cmd.Run(), ""
	}
	if err := cmd.Start(); err != nil {
		return err, ""
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	// This knob is for containment probes, not authoritative timing runs; keep
	// the poll tight so a fast RSS climb is stopped before the container OOMs.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	limitBytes := int64(rssLimitMB) << 20
	checkRSS := func() (bool, string) {
		if cmd.Process == nil {
			return false, ""
		}
		rssBytes, ok := perfScanProcessRSSBytes(cmd.Process.Pid)
		if !ok || rssBytes < limitBytes {
			return false, ""
		}
		return true, fmt.Sprintf("child rss exceeded %d MiB limit (rss=%d MiB)",
			rssLimitMB, (rssBytes+(1<<20)-1)>>20)
	}
	for {
		select {
		case err := <-done:
			return err, ""
		default:
		}
		if kill, detail := checkRSS(); kill {
			_ = cmd.Process.Kill()
			return <-done, detail
		}
		select {
		case err := <-done:
			return err, ""
		case <-ctx.Done():
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			return <-done, ""
		case <-ticker.C:
		}
	}
}

func perfScanProcessRSSBytes(pid int) (int64, bool) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, false
	}
	return perfScanParseStatusRSSBytes(string(data))
}

func perfScanParseStatusRSSBytes(status string) (int64, bool) {
	for _, line := range strings.Split(status, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, false
		}
		kib, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || kib < 0 {
			return 0, false
		}
		return kib * 1024, true
	}
	return 0, false
}

func perfScanReadLangFragment(outDir, lang string) (*perfScanLanguage, error) {
	path := filepath.Join(outDir, "langs", paritySafeName(lang)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var row perfScanLanguage
	if err := json.Unmarshal(data, &row); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	if row.Axes == nil {
		row.Axes = map[string]*perfScanLangAxis{}
	}
	return &row, nil
}

func perfScanMergeEnv(base []string, overrides map[string]string) []string {
	out := make([]string, 0, len(base)+len(overrides))
	for _, kv := range base {
		key := kv
		if idx := strings.IndexByte(kv, '='); idx >= 0 {
			key = kv[:idx]
		}
		if _, ok := overrides[key]; ok {
			continue
		}
		out = append(out, kv)
	}
	keys := make([]string, 0, len(overrides))
	for k := range overrides {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, k+"="+overrides[k])
	}
	return out
}

func perfScanLogTail(path string, lines int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return ""
	}
	all := strings.Split(text, "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, " / ")
}

func perfScanHostname() string {
	name, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return name
}

// ---------------------------------------------------------------------------
// Scoreboard rendering.
// ---------------------------------------------------------------------------

func perfScanWriteScoreboard(outDir string, board *perfScanScoreboard) error {
	data, err := json.MarshalIndent(board, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "scoreboard.json"), append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "scoreboard.md"), []byte(perfScanRenderMarkdown(board)), 0o644)
}

func perfScanFmtNs(ns int64) string {
	switch {
	case ns <= 0:
		return "-"
	case ns < 1_000_000:
		return fmt.Sprintf("%.1fµs", float64(ns)/1_000)
	case ns < 1_000_000_000:
		return fmt.Sprintf("%.2fms", float64(ns)/1_000_000)
	default:
		return fmt.Sprintf("%.2fs", float64(ns)/1_000_000_000)
	}
}

func perfScanFmtRatio(agg *perfScanLangAxis) string {
	if agg == nil {
		return "-"
	}
	if agg.RatioByTotal <= 0 {
		if agg.Cliffs > 0 {
			return "cliff"
		}
		return "-"
	}
	s := fmt.Sprintf("%.2fx", agg.RatioByTotal)
	if agg.RatioMedianOfFiles > 0 {
		s += fmt.Sprintf(" (med %.2fx)", agg.RatioMedianOfFiles)
	}
	if agg.Cliffs > 0 {
		s += fmt.Sprintf(" +%d cliff", agg.Cliffs)
	}
	return s
}

func perfScanRenderMarkdown(board *perfScanScoreboard) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Go-vs-C real-corpus perf scoreboard\n\n")
	fmt.Fprintf(&b, "- schema: `%s` generated: %s\n", board.Schema, board.GeneratedAt)
	fmt.Fprintf(&b, "- host: %s %s/%s cpus=%d %s\n",
		board.Host.Hostname, board.Host.GOOS, board.Host.GOARCH, board.Host.NumCPU, board.Host.GoVersion)
	fmt.Fprintf(&b, "- loadavg start `%s` end `%s`\n", board.Host.LoadavgStart, board.Host.LoadavgEnd)
	fmt.Fprintf(&b, "- corpus: `%s` order=%s max_files=%d reps=%d warmup=%d file_budget=%dms axes=%s\n",
		board.Config.CorpusRoot, board.Config.Order, board.Config.MaxFiles,
		board.Config.Reps, board.Config.Warmup, board.Config.FileBudgetMS,
		strings.Join(board.Config.Axes, ","))
	if board.Config.ChildRSSMB > 0 {
		fmt.Fprintf(&b, "- child RSS limit: `%d MiB`\n", board.Config.ChildRSSMB)
	}
	if board.Config.Contended {
		fmt.Fprintf(&b, "\n**WARNING: contended run (%s) — smoke-only numbers, not authoritative.**\n", board.Config.ContendedNote)
	}
	for _, note := range board.Notes {
		fmt.Fprintf(&b, "\n> %s\n", note)
	}

	fmt.Fprintf(&b, "\n## Verdict summary\n\n")
	for _, bucket := range []string{perfScanBucketLe12, perfScanBucketLe2, perfScanBucketGt2, perfScanBucketCliff, perfScanBucketNoData} {
		if n := board.Summary[bucket]; n > 0 {
			fmt.Fprintf(&b, "- `%s`: %d\n", bucket, n)
		}
	}

	fmt.Fprintf(&b, "\n## Per-language scoreboard\n\n")
	fmt.Fprintf(&b, "| language | status | files | bytes | full Go | full C | full ratio | noedit ratio | verdict |\n")
	fmt.Fprintf(&b, "|---|---|---|---|---|---|---|---|---|\n")
	for _, row := range board.Languages {
		full := row.Axes[perfScanAxisFull]
		noedit := row.Axes[perfScanAxisNoEdit]
		var goNs, cNs int64
		if full != nil {
			goNs, cNs = full.GoTotalNs, full.CTotalNs
		}
		fmt.Fprintf(&b, "| %s | %s | %d/%d | %d | %s | %s | %s | %s | %s |\n",
			row.Language, row.Status, row.FilesMeasured, row.FilesSelected, row.BytesMeasured,
			perfScanFmtNs(goNs), perfScanFmtNs(cNs),
			perfScanFmtRatio(full), perfScanFmtRatio(noedit), row.Verdict)
	}

	var cliffLines []string
	for _, row := range board.Languages {
		for _, file := range row.Files {
			for _, axis := range []string{perfScanAxisFull, perfScanAxisNoEdit, perfScanAxisEdit} {
				fa, ok := file.Axes[axis]
				if !ok {
					continue
				}
				isCliff := fa.Verdict == perfScanBucketCliff || fa.Status == "go_timeout"
				if !isCliff {
					continue
				}
				bound := ""
				if fa.RatioIsLowerBound {
					bound = ">="
				}
				cliffLines = append(cliffLines, fmt.Sprintf(
					"- **%s** `%s` axis=%s status=%s go=%s c=%s ratio%s%.1fx — %s",
					row.Language, file.Path, axis, fa.Status,
					perfScanFmtNs(fa.GoMedianNs), perfScanFmtNs(fa.CMedianNs),
					bound, fa.Ratio, fa.Detail))
			}
		}
	}
	if len(cliffLines) > 0 {
		fmt.Fprintf(&b, "\n## Cliff files (surfaced, not hung)\n\n")
		for _, line := range cliffLines {
			fmt.Fprintf(&b, "%s\n", line)
		}
	}

	var problems []string
	for _, row := range board.Languages {
		if row.Status != perfScanStatusOK {
			problems = append(problems, fmt.Sprintf("- **%s**: %s — %s", row.Language, row.Status, row.Detail))
		}
	}
	if len(problems) > 0 {
		fmt.Fprintf(&b, "\n## Languages not fully measured\n\n")
		for _, line := range problems {
			fmt.Fprintf(&b, "%s\n", line)
		}
	}
	fmt.Fprintf(&b, "\nBuckets: `%s` / `%s` / `%s` / `%s` (ratio = Go median / C median; per-language ratio-by-total = sum of Go file medians / sum of C file medians; `>=` marks a lower bound from a budget timeout).\n",
		perfScanBucketLe12, perfScanBucketLe2, perfScanBucketGt2, perfScanBucketCliff)
	return b.String()
}

// ---------------------------------------------------------------------------
// Pure-helper self-checks (no corpus, no C grammars, no subprocesses).
// ---------------------------------------------------------------------------

func TestPerfScanHelpersUnit(t *testing.T) {
	if got := perfScanVerdictBucket(1.0); got != perfScanBucketLe12 {
		t.Fatalf("bucket(1.0)=%s", got)
	}
	if got := perfScanVerdictBucket(1.9); got != perfScanBucketLe2 {
		t.Fatalf("bucket(1.9)=%s", got)
	}
	if got := perfScanVerdictBucket(9.9); got != perfScanBucketGt2 {
		t.Fatalf("bucket(9.9)=%s", got)
	}
	if got := perfScanVerdictBucket(17); got != perfScanBucketCliff {
		t.Fatalf("bucket(17)=%s", got)
	}
	if got := perfScanMedianNs([]int64{5, 1, 3}); got != 3 {
		t.Fatalf("median odd=%d", got)
	}
	if got := perfScanMedianNs([]int64{4, 2}); got != 3 {
		t.Fatalf("median even=%d", got)
	}
	out := &perfScanFileAxis{Status: "go_timeout"}
	perfScanFillAxis(out, nil, []int64{10_000_000}, false, true, 5*time.Second)
	if !out.RatioIsLowerBound || out.Verdict != perfScanBucketCliff {
		t.Fatalf("timeout lower-bound fill = %+v", out)
	}
	env := perfScanMergeEnv([]string{"A=1", "GTS_PERF_SCAN_LANG=old", "B=2"}, map[string]string{"GTS_PERF_SCAN_LANG": "go"})
	joined := strings.Join(env, " ")
	if strings.Contains(joined, "LANG=old") || !strings.Contains(joined, "GTS_PERF_SCAN_LANG=go") {
		t.Fatalf("mergeEnv=%v", env)
	}
	paths := perfScanPathList(" .\\compiler\\src\\dmd\\expressionsem.d, fsharp/examples/*.fs, groovy/subprojects/ ")
	if got, want := strings.Join(paths, ","), "compiler/src/dmd/expressionsem.d,fsharp/examples/*.fs,groovy/subprojects/"; got != want {
		t.Fatalf("path list = %q, want %q", got, want)
	}
	for _, tc := range []struct {
		name     string
		lang     string
		rel      string
		patterns []string
		want     bool
	}{
		{name: "exact relative", lang: "d", rel: "compiler/src/dmd/expressionsem.d", patterns: []string{"compiler/src/dmd/expressionsem.d"}, want: true},
		{name: "backslash relative", lang: "d", rel: "compiler/src/dmd/expressionsem.d", patterns: []string{`compiler\src\dmd\expressionsem.d`}, want: true},
		{name: "exact language relative", lang: "d", rel: "compiler/src/dmd/expressionsem.d", patterns: []string{"d/compiler/src/dmd/expressionsem.d"}, want: true},
		{name: "glob", lang: "fsharp", rel: "examples/ProvidedTypes.fs", patterns: []string{"fsharp/examples/*.fs"}, want: true},
		{name: "directory prefix", lang: "groovy", rel: "subprojects/performance/x.groovy", patterns: []string{"groovy/subprojects/"}, want: true},
		{name: "miss", lang: "go", rel: "src/main.go", patterns: []string{"d/compiler/src/dmd/expressionsem.d"}, want: false},
	} {
		if got := perfScanPathExcluded(tc.lang, tc.rel, tc.patterns); got != tc.want {
			t.Fatalf("%s: excluded=%v want %v", tc.name, got, tc.want)
		}
	}
	rss, ok := perfScanParseStatusRSSBytes("Name:\tperfscan\nVmRSS:\t  1537 kB\n")
	if !ok || rss != 1537*1024 {
		t.Fatalf("parse VmRSS = %d,%v; want %d,true", rss, ok, 1537*1024)
	}
	if _, ok := perfScanParseStatusRSSBytes("Name:\tperfscan\n"); ok {
		t.Fatal("parse VmRSS succeeded without VmRSS line")
	}
	t.Setenv(perfScanEnvChildRSSMB, "321")
	if got := perfScanLoadConfig().ChildRSSMB; got != 321 {
		t.Fatalf("ChildRSSMB = %d, want 321", got)
	}

	fragDir := t.TempDir()
	frag := &perfScanLanguage{
		Language: "perf_scan_synthetic",
		Status:   perfScanStatusRunning,
		Verdict:  perfScanBucketNoData,
	}
	perfScanSetActiveFile(frag, perfScanCorpusFile{rel: "src/synthetic.go", size: 0}, 2, 3)
	if err := perfScanWriteLangFragment(fragDir, frag); err != nil {
		t.Fatalf("write active fragment: %v", err)
	}
	fragData, err := os.ReadFile(filepath.Join(fragDir, "langs", "perf_scan_synthetic.json"))
	if err != nil {
		t.Fatalf("read active fragment: %v", err)
	}
	fragText := string(fragData)
	for _, want := range []string{
		`"active_file": "src/synthetic.go"`,
		`"active_file_index": 2`,
		`"active_file_bytes": 0`,
		`"detail": "measuring file 2/3: src/synthetic.go (0 bytes)"`,
	} {
		if !strings.Contains(fragText, want) {
			t.Fatalf("active fragment missing %s:\n%s", want, fragText)
		}
	}
	perfScanSetActiveAttempt(frag, perfScanAxisFull, "go", "rep", 1)
	if err := perfScanWriteLangFragment(fragDir, frag); err != nil {
		t.Fatalf("write active attempt fragment: %v", err)
	}
	fragData, err = os.ReadFile(filepath.Join(fragDir, "langs", "perf_scan_synthetic.json"))
	if err != nil {
		t.Fatalf("read active attempt fragment: %v", err)
	}
	fragText = string(fragData)
	for _, want := range []string{
		`"active_axis": "full"`,
		`"active_impl": "go"`,
		`"active_phase": "rep"`,
		`"active_attempt": 1`,
		`"detail": "measuring file 2/3: src/synthetic.go (0 bytes); full/go/rep attempt 1"`,
	} {
		if !strings.Contains(fragText, want) {
			t.Fatalf("active attempt fragment missing %s:\n%s", want, fragText)
		}
	}
	perfScanClearActiveFile(frag)
	if err := perfScanWriteLangFragment(fragDir, frag); err != nil {
		t.Fatalf("write cleared fragment: %v", err)
	}
	fragData, err = os.ReadFile(filepath.Join(fragDir, "langs", "perf_scan_synthetic.json"))
	if err != nil {
		t.Fatalf("read cleared fragment: %v", err)
	}
	fragText = string(fragData)
	if strings.Contains(fragText, "active_") || strings.Contains(fragText, `"detail":`) {
		t.Fatalf("cleared fragment retained active fields:\n%s", fragText)
	}

	lang := &gotreesitter.Language{
		Name: "perf_scan_synthetic",
		SymbolNames: []string{
			"EOF",
			"source_file",
		},
		SymbolMetadata: []gotreesitter.SymbolMetadata{
			{Name: "EOF"},
			{Name: "source_file", Visible: true, Named: true},
		},
	}
	src := []byte("abcdef")
	root := gotreesitter.NewLeafNode(1, true, 0, 3, gotreesitter.Point{}, gotreesitter.Point{Column: 3})
	tree := gotreesitter.NewTree(root, src, lang)
	_, att := (&perfScanLangMeasurer{budget: 5 * time.Second}).classifyGoAttempt(tree, nil, "", src, false, perfScanAttempt{})
	if att.status != "go_error" || !strings.Contains(att.detail, "truncated[SILENT]") {
		t.Fatalf("silent prefix tree classified as status=%q detail=%q, want go_error truncated[SILENT]", att.status, att.detail)
	}
}
