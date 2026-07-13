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
// the sweep. Structured parser, wall, RSS, OOM/kill, and signal stops retain
// active-attempt attribution. In hard-gate mode they fail closed instead of
// being hidden behind language aggregates.
//
// Build/run discipline mirrors the parity suites: requires the build tags
// "treesitter_c_parity treesitter_c_perfscan", the container-or-
// GTS_PARITY_ALLOW_HOST=1 TestMain guard, and the GTS_PERF_SCAN=1 env gate,
// so it never burdens normal builds or the fast PR lane. The authenticated
// fleet gate runs on its dedicated runner once that infrastructure is
// provisioned.
//
// Usage (from cgo_harness/):
//
//	GOWORK=off GTS_PARITY_ALLOW_HOST=1 GTS_PERF_SCAN=1 GTS_PERF_SCAN_HARD_GATE=0 \
//	  go test -tags "treesitter_c_parity treesitter_c_perfscan" \
//	  -run '^TestPerfScanSweep$' -v -timeout 0 .
//
// See perf_scan/README.md for the full knob reference.

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const (
	perfScanSchema = "gts-perf-scan/v1"

	perfScanEnvGate         = "GTS_PERF_SCAN"
	perfScanEnvLang         = "GTS_PERF_SCAN_LANG"
	perfScanEnvLangs        = "GTS_PERF_SCAN_LANGS"
	perfScanEnvOut          = "GTS_PERF_SCAN_OUT"
	perfScanEnvCorpusRoot   = "GTS_PERF_SCAN_CORPUS_ROOT"
	perfScanEnvReps         = "GTS_PERF_SCAN_REPS"
	perfScanEnvWarmup       = "GTS_PERF_SCAN_WARMUP"
	perfScanEnvFileBudget   = "GTS_PERF_SCAN_FILE_BUDGET_MS"
	perfScanEnvLangTimeout  = "GTS_PERF_SCAN_LANG_TIMEOUT_MS"
	perfScanEnvMaxFiles     = "GTS_PERF_SCAN_MAX_FILES"
	perfScanEnvMinBytes     = "GTS_PERF_SCAN_MIN_FILE_BYTES"
	perfScanEnvMaxBytes     = "GTS_PERF_SCAN_MAX_FILE_BYTES"
	perfScanEnvExclude      = "GTS_PERF_SCAN_EXCLUDE_PATHS"
	perfScanEnvOrder        = "GTS_PERF_SCAN_ORDER"
	perfScanEnvAxes         = "GTS_PERF_SCAN_AXES"
	perfScanEnvContended    = "GTS_PERF_SCAN_CONTENDED"
	perfScanEnvInProcess    = "GTS_PERF_SCAN_INPROCESS"
	perfScanEnvEditCands    = "GTS_PERF_SCAN_EDIT_CANDIDATES"
	perfScanEnvChildRSSMB   = "GTS_PERF_SCAN_CHILD_RSS_LIMIT_MB"
	perfScanEnvHardGate     = "GTS_PERF_SCAN_HARD_GATE"
	perfScanEnvRequireFleet = "GTS_PERF_SCAN_REQUIRE_FLEET"
	perfScanEnvCorpusLock   = "GTS_REAL_CORPUS_BENCH_LOCK"
	perfScanEnvRevision     = "GTS_PERF_SCAN_GIT_REVISION"
	perfScanEnvGitClean     = "GTS_PERF_SCAN_GIT_CLEAN"

	perfScanAxisFull   = "full"
	perfScanAxisNoEdit = "noedit"
	perfScanAxisEdit   = "edit"

	perfScanBucketLePoint1 = "<=0.10x"
	perfScanBucketLe12     = "<=1.2x"
	perfScanBucketLe2      = "<=2x"
	perfScanBucketGt2      = ">2x"
	perfScanBucketCliff    = "cliff>10x"
	perfScanBucketNoData   = "n/a"

	perfScanStatusOK      = "ok"
	perfScanStatusRunning = "running"

	perfScanClassClean   = "clean"
	perfScanClassError   = "error"
	perfScanClassStopped = "stopped"

	perfScanGatePass = "pass"
	perfScanGateFail = "fail"

	perfScanHardFullRatio = 10.0
	perfScanFastFullRatio = 0.10

	perfScanStopParserTimeout = "parser_timeout"
	perfScanStopParserBudget  = "parser_budget"
	perfScanStopParserStopped = "parser_stopped"
	perfScanStopCTimeout      = "c_timeout"
	perfScanStopWallTimeout   = "wall_timeout"
	perfScanStopRSSLimit      = "rss_limit"
	perfScanStopOOMOrKill     = "oom_or_kill"
	perfScanStopProcessSignal = "process_signal"
)

type perfScanConfig struct {
	CorpusRoot          string   `json:"corpus_root"`
	Reps                int      `json:"reps"`
	Warmup              int      `json:"warmup"`
	FileBudgetMS        int      `json:"file_budget_ms"`
	LangTimeoutMS       int      `json:"lang_timeout_ms"`
	MaxFiles            int      `json:"max_files"`
	MinFileBytes        int      `json:"min_file_bytes"`
	MaxFileBytes        int      `json:"max_file_bytes"`
	ExcludePaths        []string `json:"exclude_paths,omitempty"`
	Order               string   `json:"order"`
	Axes                []string `json:"axes"`
	Contended           bool     `json:"contended"`
	ContendedNote       string   `json:"contended_note,omitempty"`
	ChildRSSMB          int      `json:"child_rss_limit_mb,omitempty"`
	HardGate            bool     `json:"hard_gate"`
	RequireFleet        bool     `json:"require_fleet"`
	CorpusLock          string   `json:"corpus_lock,omitempty"`
	CorpusLockSHA256    string   `json:"corpus_lock_sha256,omitempty"`
	CorpusLockLanguages int      `json:"corpus_lock_languages,omitempty"`
	Languages           []string `json:"languages,omitempty"`
}

type perfScanHost struct {
	Hostname     string `json:"hostname"`
	GOOS         string `json:"goos"`
	GOARCH       string `json:"goarch"`
	NumCPU       int    `json:"num_cpu"`
	GOMAXPROCS   int    `json:"gomaxprocs"`
	GoVersion    string `json:"go_version"`
	LoadavgStart string `json:"loadavg_start,omitempty"`
	LoadavgEnd   string `json:"loadavg_end,omitempty"`
}

type perfScanFileAxis struct {
	Status            string        `json:"status"`
	Detail            string        `json:"detail,omitempty"`
	GoMedianNs        int64         `json:"go_median_ns,omitempty"`
	CMedianNs         int64         `json:"c_median_ns,omitempty"`
	GoMinNs           int64         `json:"go_min_ns,omitempty"`
	GoMaxNs           int64         `json:"go_max_ns,omitempty"`
	CMinNs            int64         `json:"c_min_ns,omitempty"`
	CMaxNs            int64         `json:"c_max_ns,omitempty"`
	Ratio             float64       `json:"ratio,omitempty"`
	RatioIsLowerBound bool          `json:"ratio_is_lower_bound,omitempty"`
	Verdict           string        `json:"verdict,omitempty"`
	Stop              *perfScanStop `json:"stop,omitempty"`
}

type perfScanStop struct {
	Class          string `json:"class"`
	Reason         string `json:"reason,omitempty"`
	Implementation string `json:"implementation,omitempty"`
	Phase          string `json:"phase,omitempty"`
	Attempt        int    `json:"attempt,omitempty"`
	Detail         string `json:"detail,omitempty"`
}

type perfScanFile struct {
	Path           string                       `json:"path"`
	Bytes          int                          `json:"bytes"`
	Classification *perfScanFileClassification  `json:"classification,omitempty"`
	Axes           map[string]*perfScanFileAxis `json:"axes"`
}

// perfScanFileClassification records the untimed Go full-parse corpus-policy
// classification. A clean file completed, spans the source, and has no ERROR
// nodes. Stopped is kept distinct at file level for attribution; language
// summaries include stopped files in the non-clean/error side, matching the
// parse-gap tools' existing Clean=false policy.
type perfScanFileClassification struct {
	Class        string `json:"class"`
	Reason       string `json:"reason"`
	GoStatus     string `json:"go_status"`
	FullSpan     bool   `json:"full_span"`
	RootHasError bool   `json:"root_has_error"`
	StoppedEarly bool   `json:"stopped_early"`
	StopReason   string `json:"stop_reason,omitempty"`
}

type perfScanClassTiming struct {
	Files              int     `json:"files"`
	FilesOK            int     `json:"files_ok"`
	GoTotalNs          int64   `json:"go_total_ns"`
	CTotalNs           int64   `json:"c_total_ns"`
	RatioByTotal       float64 `json:"ratio_by_total,omitempty"`
	RatioMedianOfFiles float64 `json:"ratio_median_of_files,omitempty"`
}

type perfScanCleanErrorSplit struct {
	ClassifiedFiles int                 `json:"classified_files"`
	StoppedFiles    int                 `json:"stopped_files"`
	ErrorShare      float64             `json:"error_share"`
	Clean           perfScanClassTiming `json:"clean"`
	// Error includes every non-clean file. StoppedFiles is the stopped subset;
	// only status=ok rows contribute timings and ratios.
	Error perfScanClassTiming `json:"error"`
}

type perfScanLangAxis struct {
	FilesOK            int     `json:"files_ok"`
	GoTotalNs          int64   `json:"go_total_ns"`
	CTotalNs           int64   `json:"c_total_ns"`
	RatioByTotal       float64 `json:"ratio_by_total,omitempty"`
	RatioMedianOfFiles float64 `json:"ratio_median_of_files,omitempty"`
	Cliffs             int     `json:"cliffs"`
	GoTimeouts         int     `json:"go_timeouts"`
	GoStops            int     `json:"go_stops"`
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
	FullParseSplit   *perfScanCleanErrorSplit     `json:"full_parse_split,omitempty"`
	Notes            []string                     `json:"notes,omitempty"`
	Files            []*perfScanFile              `json:"files,omitempty"`
	Stop             *perfScanStop                `json:"stop,omitempty"`
	activeFileDetail string
}

type perfScanCorpusCoverage struct {
	LockPath            string   `json:"lock_path,omitempty"`
	LockSHA256          string   `json:"lock_sha256,omitempty"`
	LockLanguages       int      `json:"lock_languages"`
	SelectedLanguages   int      `json:"selected_languages"`
	MissingFromLock     []string `json:"missing_from_lock,omitempty"`
	MissingFromRegistry []string `json:"missing_from_registry,omitempty"`
}

type perfScanGateFinding struct {
	Kind     string        `json:"kind"`
	Language string        `json:"language,omitempty"`
	Path     string        `json:"path,omitempty"`
	Axis     string        `json:"axis,omitempty"`
	Status   string        `json:"status,omitempty"`
	Ratio    float64       `json:"ratio,omitempty"`
	Limit    float64       `json:"limit,omitempty"`
	Stop     *perfScanStop `json:"stop,omitempty"`
	Detail   string        `json:"detail,omitempty"`
}

type perfScanGateReport struct {
	Status             string                `json:"status"`
	MaxFullParseRatio  float64               `json:"max_full_parse_ratio"`
	FastFullParseRatio float64               `json:"fast_full_parse_ratio"`
	FilesExpected      int                   `json:"files_expected"`
	FilesMeasured      int                   `json:"files_measured"`
	FullFilesEvaluated int                   `json:"full_files_evaluated"`
	FastFullFiles      []perfScanGateFinding `json:"fast_full_files,omitempty"`
	Failures           []perfScanGateFinding `json:"failures,omitempty"`
}

type perfScanReductionProvenance struct {
	GitRevision string `json:"git_revision"`
	GitClean    bool   `json:"git_clean"`
}

type perfScanScoreboard struct {
	Schema         string                       `json:"schema"`
	GeneratedAt    string                       `json:"generated_at"`
	GitRevision    string                       `json:"git_revision,omitempty"`
	GitClean       bool                         `json:"git_clean"`
	Host           perfScanHost                 `json:"host"`
	Config         perfScanConfig               `json:"config"`
	Notes          []string                     `json:"notes,omitempty"`
	Summary        map[string]int               `json:"summary_verdicts"`
	Languages      []*perfScanLanguage          `json:"languages"`
	FullParseSplit *perfScanCleanErrorSplit     `json:"full_parse_split,omitempty"`
	Corpus         perfScanCorpusCoverage       `json:"corpus_coverage"`
	Gate           *perfScanGateReport          `json:"hard_gate,omitempty"`
	Reduction      *perfScanReductionProvenance `json:"reduction,omitempty"`
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
	requireFleetDefault := strings.TrimSpace(os.Getenv(perfScanEnvLangs)) == ""
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
		HardGate:      parityEnvBool(perfScanEnvHardGate, true),
		RequireFleet:  parityEnvBool(perfScanEnvRequireFleet, requireFleetDefault),
		CorpusLock:    strings.TrimSpace(os.Getenv(perfScanEnvCorpusLock)),
		Languages:     perfScanLanguageList(os.Getenv(perfScanEnvLangs)),
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

func perfScanLanguageList(raw string) []string {
	seen := map[string]bool{}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

type perfScanVCSCandidate struct {
	Revision   string
	Known      bool
	CleanKnown bool
	Modified   bool
}

type perfScanRepositoryState struct {
	Revision string
	Clean    bool
}

func perfScanRepositoryProvenance(requireClean bool) (perfScanRepositoryState, error) {
	git, err := perfScanGitCandidate()
	if err != nil {
		return perfScanRepositoryState{}, err
	}
	build := perfScanBuildCandidate()
	return perfScanSelectRepositoryProvenance(
		git,
		build,
		strings.TrimSpace(os.Getenv(perfScanEnvRevision)),
		parityEnvBool(perfScanEnvGitClean, false),
		requireClean,
	)
}

func perfScanGitCandidate() (perfScanVCSCandidate, error) {
	cmd := exec.Command("git", "rev-parse", "--verify", "HEAD")
	data, err := cmd.Output()
	if err != nil {
		return perfScanVCSCandidate{}, nil
	}
	revision, err := perfScanNormalizeRevision(string(data))
	if err != nil {
		return perfScanVCSCandidate{}, fmt.Errorf("git HEAD: %w", err)
	}
	status, err := exec.Command("git", "status", "--porcelain=v1", "--untracked-files=all").Output()
	if err != nil {
		return perfScanVCSCandidate{}, fmt.Errorf("inspect git worktree: %w", err)
	}
	return perfScanVCSCandidate{Revision: revision, Known: true, CleanKnown: true, Modified: strings.TrimSpace(string(status)) != ""}, nil
}

func perfScanBuildCandidate() perfScanVCSCandidate {
	var candidate perfScanVCSCandidate
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				candidate.Revision = strings.TrimSpace(setting.Value)
				candidate.Known = candidate.Revision != ""
			case "vcs.modified":
				value := strings.TrimSpace(setting.Value)
				candidate.CleanKnown = strings.EqualFold(value, "true") || strings.EqualFold(value, "false")
				candidate.Modified = strings.EqualFold(value, "true")
			}
		}
	}
	return candidate
}

func perfScanSelectRepositoryProvenance(git, build perfScanVCSCandidate, override string, assertedClean, requireClean bool) (perfScanRepositoryState, error) {
	override = strings.TrimSpace(override)
	validateOverride := func(revision string) error {
		if override == "" {
			return nil
		}
		normalized, err := perfScanNormalizeRevision(override)
		if err != nil {
			return fmt.Errorf("%s: %w", perfScanEnvRevision, err)
		}
		if normalized != revision {
			return fmt.Errorf("%s=%s cannot supersede discovered repository revision %s", perfScanEnvRevision, normalized, revision)
		}
		return nil
	}
	var discoveredRevision string
	clean := true
	for _, source := range []struct {
		name      string
		candidate perfScanVCSCandidate
	}{{"git", git}, {"Go build", build}} {
		if !source.candidate.Known {
			continue
		}
		revision, err := perfScanNormalizeRevision(source.candidate.Revision)
		if err != nil {
			return perfScanRepositoryState{}, fmt.Errorf("%s repository revision: %w", source.name, err)
		}
		if source.candidate.Modified {
			clean = false
			if requireClean {
				return perfScanRepositoryState{}, fmt.Errorf("%s reports a dirty or modified repository; authoritative scoreboards require tracked and untracked cleanliness", source.name)
			}
		}
		if !source.candidate.CleanKnown && !assertedClean {
			clean = false
			if requireClean {
				return perfScanRepositoryState{}, fmt.Errorf("%s does not report repository cleanliness; explicit %s=1 is required", source.name, perfScanEnvGitClean)
			}
		}
		if discoveredRevision == "" {
			discoveredRevision = revision
		} else if discoveredRevision != revision {
			return perfScanRepositoryState{}, fmt.Errorf("Git and Go build repository revisions disagree: %s != %s", discoveredRevision, revision)
		}
	}
	if discoveredRevision != "" {
		if err := validateOverride(discoveredRevision); err != nil {
			return perfScanRepositoryState{}, err
		}
		return perfScanRepositoryState{Revision: discoveredRevision, Clean: clean}, nil
	}
	if override == "" {
		return perfScanRepositoryState{}, fmt.Errorf("determine repository revision (set %s and %s=1 only when git and Go VCS metadata are unavailable)", perfScanEnvRevision, perfScanEnvGitClean)
	}
	revision, err := perfScanNormalizeRevision(override)
	if err != nil {
		return perfScanRepositoryState{}, fmt.Errorf("%s: %w", perfScanEnvRevision, err)
	}
	if !assertedClean && requireClean {
		return perfScanRepositoryState{}, fmt.Errorf("metadata-poor revision override requires explicit %s=1", perfScanEnvGitClean)
	}
	return perfScanRepositoryState{Revision: revision, Clean: assertedClean}, nil
}

func perfScanNormalizeRevision(raw string) (string, error) {
	revision := strings.ToLower(strings.TrimSpace(raw))
	if len(revision) != 40 && len(revision) != 64 {
		return "", fmt.Errorf("repository revision %q must be a full 40- or 64-hex object ID", raw)
	}
	if _, err := hex.DecodeString(revision); err != nil {
		return "", fmt.Errorf("repository revision %q is not hexadecimal: %w", raw, err)
	}
	return revision, nil
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
	case ratio <= perfScanFastFullRatio:
		return perfScanBucketLePoint1
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

func perfScanPrepareCorpusLock(cfg *perfScanConfig) (map[string]realCorpusBenchmarkLockEntry, perfScanCorpusCoverage, error) {
	coverage := perfScanCorpusCoverage{}
	if cfg == nil {
		return nil, coverage, fmt.Errorf("nil perf scan config")
	}
	lockPath := strings.TrimSpace(cfg.CorpusLock)
	if lockPath == "" {
		if cfg.HardGate {
			return nil, coverage, fmt.Errorf("%s must name the authenticated corpus lock in hard-gate mode", perfScanEnvCorpusLock)
		}
		return nil, coverage, nil
	}
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return nil, coverage, fmt.Errorf("read corpus lock %s: %w", lockPath, err)
	}
	actualDigest := fmt.Sprintf("%x", sha256.Sum256(data))
	if cfg.HardGate {
		expectedDigest, err := perfScanExpectedCorpusLockDigest()
		if err != nil {
			return nil, coverage, err
		}
		if actualDigest != expectedDigest {
			return nil, coverage, fmt.Errorf("corpus lock sha256 %s, want %s from perf_scan/corpus_sources.lock.sha256", actualDigest, expectedDigest)
		}
	}
	entries, err := realCorpusBenchmarkLockEntries(lockPath)
	if err != nil {
		return nil, coverage, fmt.Errorf("parse corpus lock %s: %w", lockPath, err)
	}
	if len(entries) == 0 {
		return nil, coverage, fmt.Errorf("corpus lock %s contains no languages", lockPath)
	}
	cfg.CorpusLockSHA256 = actualDigest
	cfg.CorpusLockLanguages = len(entries)
	coverage.LockPath = lockPath
	coverage.LockSHA256 = actualDigest
	coverage.LockLanguages = len(entries)
	for lang := range entries {
		if _, ok := parityEntriesByName[lang]; !ok {
			coverage.MissingFromRegistry = append(coverage.MissingFromRegistry, lang)
		}
	}
	sort.Strings(coverage.MissingFromRegistry)
	return entries, coverage, nil
}

func perfScanExpectedCorpusLockDigest() (string, error) {
	var lastErr error
	for _, candidate := range []string{
		filepath.Join("perf_scan", "corpus_sources.lock.sha256"),
		filepath.Join("cgo_harness", "perf_scan", "corpus_sources.lock.sha256"),
		filepath.Join("..", "cgo_harness", "perf_scan", "corpus_sources.lock.sha256"),
	} {
		data, err := os.ReadFile(candidate)
		if err != nil {
			lastErr = err
			continue
		}
		fields := strings.Fields(string(data))
		if len(fields) == 0 || len(fields[0]) != sha256.Size*2 {
			return "", fmt.Errorf("invalid corpus lock digest file %s", candidate)
		}
		for _, r := range fields[0] {
			if !strings.ContainsRune("0123456789abcdef", r) {
				return "", fmt.Errorf("invalid corpus lock sha256 %q in %s", fields[0], candidate)
			}
		}
		return fields[0], nil
	}
	return "", fmt.Errorf("read perf_scan/corpus_sources.lock.sha256: %w", lastErr)
}

func perfScanFinalizeCorpusCoverage(coverage perfScanCorpusCoverage, entries map[string]realCorpusBenchmarkLockEntry, langs []string) perfScanCorpusCoverage {
	coverage.SelectedLanguages = len(langs)
	for _, lang := range langs {
		if _, ok := entries[lang]; !ok {
			coverage.MissingFromLock = append(coverage.MissingFromLock, lang)
		}
	}
	sort.Strings(coverage.MissingFromLock)
	return coverage
}

func perfScanIsGoStop(stop *perfScanStop) bool {
	return stop != nil && stop.Implementation == "go"
}

func perfScanCanonicalizeGateReport(report perfScanGateReport) perfScanGateReport {
	report.FastFullFiles = append([]perfScanGateFinding(nil), report.FastFullFiles...)
	report.Failures = append([]perfScanGateFinding(nil), report.Failures...)
	sort.Slice(report.FastFullFiles, func(i, j int) bool {
		return perfScanCompareGateFinding(report.FastFullFiles[i], report.FastFullFiles[j]) < 0
	})
	sort.Slice(report.Failures, func(i, j int) bool {
		return perfScanCompareGateFinding(report.Failures[i], report.Failures[j]) < 0
	})
	return report
}

func perfScanCompareGateFinding(a, b perfScanGateFinding) int {
	for _, pair := range [][2]string{
		{a.Kind, b.Kind},
		{a.Language, b.Language},
		{a.Path, b.Path},
		{a.Axis, b.Axis},
		{a.Status, b.Status},
	} {
		if order := cmp.Compare(pair[0], pair[1]); order != 0 {
			return order
		}
	}
	if order := cmp.Compare(a.Ratio, b.Ratio); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Limit, b.Limit); order != 0 {
		return order
	}
	if order := perfScanCompareGateStop(a.Stop, b.Stop); order != 0 {
		return order
	}
	return cmp.Compare(a.Detail, b.Detail)
}

func perfScanCompareGateStop(a, b *perfScanStop) int {
	if a == nil {
		if b == nil {
			return 0
		}
		return -1
	}
	if b == nil {
		return 1
	}
	for _, pair := range [][2]string{
		{a.Class, b.Class},
		{a.Reason, b.Reason},
		{a.Implementation, b.Implementation},
		{a.Phase, b.Phase},
	} {
		if order := cmp.Compare(pair[0], pair[1]); order != 0 {
			return order
		}
	}
	if order := cmp.Compare(a.Attempt, b.Attempt); order != 0 {
		return order
	}
	return cmp.Compare(a.Detail, b.Detail)
}

func perfScanEvaluateHardGate(board *perfScanScoreboard) perfScanGateReport {
	report := perfScanGateReport{
		Status:             perfScanGatePass,
		MaxFullParseRatio:  perfScanHardFullRatio,
		FastFullParseRatio: perfScanFastFullRatio,
	}
	addFailure := func(finding perfScanGateFinding) {
		report.Failures = append(report.Failures, finding)
		report.Status = perfScanGateFail
	}
	if board == nil {
		addFailure(perfScanGateFinding{Kind: "coverage", Detail: "nil scoreboard"})
		return perfScanCanonicalizeGateReport(report)
	}
	if len(board.Languages) == 0 {
		addFailure(perfScanGateFinding{Kind: "coverage", Status: "no_evidence", Detail: "scoreboard contains no language rows"})
	}
	for _, lang := range board.Corpus.MissingFromLock {
		addFailure(perfScanGateFinding{Kind: "coverage", Language: lang, Detail: "language missing from authenticated corpus lock"})
	}
	for _, lang := range board.Corpus.MissingFromRegistry {
		addFailure(perfScanGateFinding{Kind: "coverage", Language: lang, Detail: "locked corpus language missing from grammar registry"})
	}
	if board.Config.RequireFleet && board.Corpus.SelectedLanguages != board.Corpus.LockLanguages {
		addFailure(perfScanGateFinding{
			Kind:   "coverage",
			Status: "fleet_incomplete",
			Detail: fmt.Sprintf("selected %d language(s), authenticated lock contains %d", board.Corpus.SelectedLanguages, board.Corpus.LockLanguages),
		})
	}
	if board.Config.RequireFleet && len(board.Languages) != board.Corpus.SelectedLanguages {
		addFailure(perfScanGateFinding{
			Kind:   "coverage",
			Status: "fleet_incomplete",
			Detail: fmt.Sprintf("scoreboard has %d language row(s), selected %d", len(board.Languages), board.Corpus.SelectedLanguages),
		})
	}
	for _, row := range board.Languages {
		if row == nil {
			addFailure(perfScanGateFinding{Kind: "coverage", Detail: "nil language row"})
			continue
		}
		if row.FilesSelected <= 0 || row.FilesMeasured <= 0 || len(row.Files) <= 0 {
			addFailure(perfScanGateFinding{Kind: "coverage", Language: row.Language, Status: "no_evidence", Detail: "language has no selected and measured file evidence"})
		}
		report.FilesExpected += row.FilesSelected
		report.FilesMeasured += row.FilesMeasured
		if perfScanIsGoStop(row.Stop) {
			addFailure(perfScanGateFinding{
				Kind:     "go_stop",
				Language: row.Language,
				Path:     row.ActiveFile,
				Axis:     row.ActiveAxis,
				Status:   row.Status,
				Stop:     row.Stop,
				Detail:   row.Detail,
			})
		}
		if row.Status != perfScanStatusOK && !perfScanIsGoStop(row.Stop) {
			addFailure(perfScanGateFinding{Kind: "coverage", Language: row.Language, Path: row.ActiveFile, Axis: row.ActiveAxis, Status: row.Status, Stop: row.Stop, Detail: row.Detail})
		}
		if row.FilesMeasured != row.FilesSelected || len(row.Files) != row.FilesSelected {
			addFailure(perfScanGateFinding{
				Kind:     "coverage",
				Language: row.Language,
				Status:   "files_incomplete",
				Detail:   fmt.Sprintf("measured=%d rows=%d selected=%d", row.FilesMeasured, len(row.Files), row.FilesSelected),
			})
		}
		for _, file := range row.Files {
			if file == nil {
				addFailure(perfScanGateFinding{Kind: "coverage", Language: row.Language, Detail: "nil file row"})
				continue
			}
			for axis, result := range file.Axes {
				if result != nil && perfScanIsGoStop(result.Stop) {
					addFailure(perfScanGateFinding{Kind: "go_stop", Language: row.Language, Path: file.Path, Axis: axis, Status: result.Status, Stop: result.Stop, Detail: result.Detail})
				}
			}
			full := file.Axes[perfScanAxisFull]
			if full == nil {
				addFailure(perfScanGateFinding{Kind: "coverage", Language: row.Language, Path: file.Path, Axis: perfScanAxisFull, Status: "missing", Detail: "full-parse axis missing"})
				continue
			}
			if full.Status != perfScanStatusOK || full.GoMedianNs <= 0 || full.CMedianNs <= 0 {
				if !perfScanIsGoStop(full.Stop) {
					addFailure(perfScanGateFinding{Kind: "coverage", Language: row.Language, Path: file.Path, Axis: perfScanAxisFull, Status: full.Status, Stop: full.Stop, Detail: "full-parse Go/C ratio was not measured: " + full.Detail})
				}
				continue
			}
			report.FullFilesEvaluated++
			ratio := float64(full.GoMedianNs) / float64(full.CMedianNs)
			finding := perfScanGateFinding{Language: row.Language, Path: file.Path, Axis: perfScanAxisFull, Status: full.Status, Ratio: ratio}
			switch {
			case ratio > perfScanHardFullRatio:
				finding.Kind = "ratio"
				finding.Limit = perfScanHardFullRatio
				finding.Detail = fmt.Sprintf("full-parse Go/C ratio %.4fx exceeds %.1fx", ratio, perfScanHardFullRatio)
				addFailure(finding)
			case ratio <= perfScanFastFullRatio:
				finding.Kind = "fast"
				finding.Limit = perfScanFastFullRatio
				finding.Detail = fmt.Sprintf("full parse is %.2fx faster than C", 1/ratio)
				report.FastFullFiles = append(report.FastFullFiles, finding)
			}
		}
	}
	if report.FullFilesEvaluated == 0 {
		addFailure(perfScanGateFinding{Kind: "coverage", Status: "no_evidence", Detail: "scoreboard contains no evaluated full-parse file ratios"})
	}
	return perfScanCanonicalizeGateReport(report)
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
	if _, _, err := perfScanPrepareCorpusLock(&cfg); err != nil {
		t.Fatalf("prepare authenticated corpus lock: %v", err)
	}
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
	report, ok := paritySupportForName(lang)
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
			if axis == perfScanAxisFull {
				fileRow.Axes[axis], fileRow.Classification = m.measureFull(src)
				continue
			}
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
	// Full-axis hooks are nil in production and exist so protocol tests can
	// deterministically separate timed failures from classification probes.
	goFullAttempt func(src []byte, keepTree bool) (*gotreesitter.Tree, perfScanAttempt)
	cFullAttempt  func(src []byte, oldTree *sitter.Tree, keepTree bool) (*sitter.Tree, perfScanAttempt)
}

type perfScanAttempt struct {
	ns     int64
	status string // "" == ok
	detail string
	stop   *perfScanStop
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

func (m *perfScanLangMeasurer) attemptGoFull(src []byte, keepTree bool) (*gotreesitter.Tree, perfScanAttempt) {
	if m.goFullAttempt != nil {
		return m.goFullAttempt(src, keepTree)
	}
	return m.goAttemptFullForClassification(src, keepTree)
}

func (m *perfScanLangMeasurer) attemptCFull(src []byte, oldTree *sitter.Tree, keepTree bool) (*sitter.Tree, perfScanAttempt) {
	if m.cFullAttempt != nil {
		return m.cFullAttempt(src, oldTree, keepTree)
	}
	return m.cAttempt(src, oldTree, keepTree)
}

// goAttemptFull runs one timed Go full parse. The returned tree is nil unless
// the parse completed cleanly.
func (m *perfScanLangMeasurer) goAttemptFull(src []byte, keepTree bool) (*gotreesitter.Tree, perfScanAttempt) {
	return m.goAttemptFullWithRetention(src, keepTree, false)
}

// goAttemptFullForClassification preserves a valid rejected tree when the
// caller asks to retain it, allowing diagnostics to inspect the rejection
// before measureFull releases the tree.
func (m *perfScanLangMeasurer) goAttemptFullForClassification(src []byte, keepTree bool) (*gotreesitter.Tree, perfScanAttempt) {
	return m.goAttemptFullWithRetention(src, keepTree, keepTree)
}

func (m *perfScanLangMeasurer) goAttemptFullWithRetention(src []byte, keepTree, retainRejected bool) (*gotreesitter.Tree, perfScanAttempt) {
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
	return m.classifyGoAttemptWithRetention(tree, err, panicked, src, keepTree, retainRejected, att)
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
	return m.classifyGoAttemptWithRetention(tree, err, panicked, src, keepTree, false, att)
}

func (m *perfScanLangMeasurer) classifyGoAttemptWithRetention(tree *gotreesitter.Tree, err error, panicked string, src []byte, keepTree, retainRejected bool, att perfScanAttempt) (*gotreesitter.Tree, perfScanAttempt) {
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
		reason := tree.ParseStopReason()
		att.stop = perfScanGoParserStop(reason, m.budget)
		att.status = "go_stopped"
		if reason == gotreesitter.ParseStopTimeout {
			att.status = "go_timeout"
		} else if att.stop.Class == perfScanStopParserBudget {
			att.status = "go_budget_stop"
		}
		att.detail = att.stop.Detail
		if keepTree && retainRejected {
			return tree, att
		}
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
		if keepTree && retainRejected {
			return tree, att
		}
		releaseGoTree(tree)
		return nil, att
	}
	// The root spans the input from here on. Enforce consistency so a covered
	// tree cannot masquerade as clean.
	if rt.Truncated {
		// Coverage and the Truncated flag disagree: an internal inconsistency.
		att.status = "go_error"
		att.detail = fmt.Sprintf("inconsistent: root covers %d but ParseRuntime.Truncated=true", got)
		if keepTree && retainRejected {
			return tree, att
		}
		releaseGoTree(tree)
		return nil, att
	}
	if root.IsError() {
		// A full-span ERROR root is a degenerate parse (whole input recovered as
		// one ERROR node); 'ok' used to mask this (the webworker case).
		att.status = "go_error"
		att.detail = "error_root: root symbol is ERROR spanning the input"
		if keepTree && retainRejected {
			return tree, att
		}
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
		att.stop = &perfScanStop{
			Class:          perfScanStopCTimeout,
			Implementation: "c",
			Detail:         att.detail,
		}
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

func perfScanGoParserStop(reason gotreesitter.ParseStopReason, budget time.Duration) *perfScanStop {
	class := perfScanStopParserStopped
	switch reason {
	case gotreesitter.ParseStopTimeout:
		class = perfScanStopParserTimeout
	case gotreesitter.ParseStopMemoryBudget,
		gotreesitter.ParseStopNodeLimit,
		gotreesitter.ParseStopIterationLimit,
		gotreesitter.ParseStopStackDepthLimit:
		class = perfScanStopParserBudget
	}
	return &perfScanStop{
		Class:          class,
		Reason:         string(reason),
		Implementation: "go",
		Detail:         fmt.Sprintf("parse stopped early (%v) at file budget %s", reason, budget),
	}
}

func perfScanRecordAttemptStop(out *perfScanFileAxis, att perfScanAttempt, phase string, attempt int) {
	if out == nil || att.stop == nil {
		return
	}
	stop := *att.stop
	stop.Phase = phase
	stop.Attempt = attempt
	// Preserve a Go stop if the C side also stops later in the same axis. The
	// hard gate must retain the causal Go failure instead of letting a
	// subsequent C timeout overwrite it.
	if out.Stop != nil && out.Stop.Implementation == "go" && stop.Implementation != "go" {
		return
	}
	out.Stop = &stop
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
		out, _ := m.measureFull(src)
		return out
	case perfScanAxisNoEdit:
		return m.measureNoEdit(src)
	case perfScanAxisEdit:
		return m.measureEdit(src)
	default:
		return &perfScanFileAxis{Status: "skipped", Detail: "unknown axis " + axis}
	}
}

func perfScanClassifyGoFull(tree *gotreesitter.Tree, att perfScanAttempt, sourceBytes int) *perfScanFileClassification {
	status := att.status
	if status == "" {
		status = perfScanStatusOK
	}
	classification := &perfScanFileClassification{
		Class:    perfScanClassError,
		Reason:   att.detail,
		GoStatus: status,
	}
	if tree != nil && tree.RootNode() != nil {
		root := tree.RootNode()
		classification.FullSpan = root.StartByte() == 0 && root.EndByte() == uint32(sourceBytes)
		classification.RootHasError = root.HasError() || root.IsError()
		classification.StoppedEarly = tree.ParseStoppedEarly()
	}
	if att.stop != nil {
		classification.Class = perfScanClassStopped
		classification.StoppedEarly = true
		classification.StopReason = att.stop.Reason
		if classification.Reason == "" {
			classification.Reason = att.stop.Detail
		}
		if classification.Reason == "" {
			classification.Reason = "Go full parse stopped before acceptance"
		}
		return classification
	}
	if att.status != "" {
		if classification.Reason == "" {
			classification.Reason = "Go full parse did not produce an accepted full-span tree"
		}
		return classification
	}
	if tree == nil || tree.RootNode() == nil {
		classification.Reason = "Go full parse returned a nil tree"
		return classification
	}

	root := tree.RootNode()
	if IsAcceptedFullSpanCleanGoTree(tree, sourceBytes) {
		classification.Class = perfScanClassClean
		classification.Reason = "accepted full-span Go tree without ERROR nodes"
		return classification
	}
	if classification.StoppedEarly {
		classification.Class = perfScanClassStopped
		classification.StopReason = string(tree.ParseStopReason())
		classification.Reason = fmt.Sprintf("Go full parse stopped early (%s)", classification.StopReason)
		return classification
	}
	if !classification.FullSpan {
		classification.Reason = fmt.Sprintf("Go root spans bytes [%d,%d), want [0,%d)", root.StartByte(), root.EndByte(), sourceBytes)
		return classification
	}
	if classification.RootHasError {
		classification.Reason = "Go root contains ERROR nodes"
		return classification
	}
	classification.Reason = "Go full parse was not accepted as a clean full-span tree"
	return classification
}

func (m *perfScanLangMeasurer) measureFull(src []byte) (*perfScanFileAxis, *perfScanFileClassification) {
	out := &perfScanFileAxis{Status: perfScanStatusOK}
	var classification *perfScanFileClassification

	goOK := true
	var goDetail string
	for i := 0; i < m.cfg.Warmup; i++ {
		m.checkpoint(perfScanAxisFull, "go", "warmup", i+1)
		keepTree := i == 0
		tree, att := m.attemptGoFull(src, keepTree)
		if i == 0 {
			classification = perfScanClassifyGoFull(tree, att, len(src))
		}
		releaseGoTree(tree)
		if att.status != "" {
			goOK = false
			out.Status = att.status
			goDetail = att.detail
			perfScanRecordAttemptStop(out, att, "warmup", i+1)
			break
		}
	}
	cOK := true
	for i := 0; i < m.cfg.Warmup; i++ {
		m.checkpoint(perfScanAxisFull, "c", "warmup", i+1)
		_, att := m.attemptCFull(src, nil, false)
		if att.status != "" {
			cOK = false
			if out.Status == perfScanStatusOK {
				out.Status = att.status
			}
			out.Detail = strings.TrimSpace(out.Detail + " " + att.detail)
			perfScanRecordAttemptStop(out, att, "warmup", i+1)
			break
		}
	}

	var goSamples, cSamples []int64
	for i := 0; i < m.cfg.Reps; i++ {
		if goOK {
			m.checkpoint(perfScanAxisFull, "go", "rep", i+1)
			_, att := m.attemptGoFull(src, false)
			if att.status != "" {
				goOK = false
				out.Status = att.status
				goDetail = att.detail
				perfScanRecordAttemptStop(out, att, "rep", i+1)
			} else {
				goSamples = append(goSamples, att.ns)
			}
		}
		if cOK {
			m.checkpoint(perfScanAxisFull, "c", "rep", i+1)
			_, att := m.attemptCFull(src, nil, false)
			if att.status != "" {
				cOK = false
				if out.Status == perfScanStatusOK {
					out.Status = att.status
				}
				out.Detail = strings.TrimSpace(out.Detail + " " + att.detail)
				perfScanRecordAttemptStop(out, att, "rep", i+1)
			} else {
				cSamples = append(cSamples, att.ns)
			}
		}
	}
	// With warmup=0, classify after the timed samples so the classification
	// probe cannot warm parser caches or otherwise alter the requested timing
	// protocol. Its elapsed time is deliberately discarded.
	if m.cfg.Warmup == 0 {
		m.checkpoint(perfScanAxisFull, "go", "classify", 1)
		tree, att := m.attemptGoFull(src, true)
		classification = perfScanClassifyGoFull(tree, att, len(src))
		releaseGoTree(tree)
	}
	if goDetail != "" {
		out.Detail = strings.TrimSpace(goDetail + " " + out.Detail)
	}
	perfScanFillAxis(out, goSamples, cSamples, goOK, cOK, m.budget)
	return out, classification
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
		perfScanRecordAttemptStop(out, baseAtt, "base", 1)
	} else {
		for i := 0; i < m.cfg.Reps; i++ {
			m.checkpoint(perfScanAxisNoEdit, "go", "rep", i+1)
			newTree, att := m.goAttemptIncremental(src, goTree, true)
			if att.status != "" {
				goOK = false
				out.Status = att.status
				out.Detail = strings.TrimSpace(out.Detail + " " + att.detail)
				perfScanRecordAttemptStop(out, att, "rep", i+1)
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
		perfScanRecordAttemptStop(out, cBaseAtt, "base", 1)
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
				perfScanRecordAttemptStop(out, att, "rep", i+1)
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
		perfScanRecordAttemptStop(out, baseAtt, "base", 1)
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
				perfScanRecordAttemptStop(out, att, "rep", i+1)
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
		perfScanRecordAttemptStop(out, cBaseAtt, "base", 1)
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
				perfScanRecordAttemptStop(out, att, "rep", i+1)
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
			if perfScanIsGoStop(fa.Stop) {
				agg.GoStops++
			}
			if fa.Verdict == perfScanBucketCliff || (fa.RatioIsLowerBound && fa.Ratio > perfScanHardFullRatio) || perfScanIsGoStop(fa.Stop) {
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
	perfScanAggregateCleanErrorSplit(row)
}

func perfScanAggregateCleanErrorSplit(row *perfScanLanguage) {
	if row == nil {
		return
	}
	split := &perfScanCleanErrorSplit{}
	var cleanRatios, errorRatios []float64
	for _, file := range row.Files {
		if file == nil || file.Classification == nil {
			continue
		}
		split.ClassifiedFiles++
		timing := &split.Error
		ratios := &errorRatios
		if file.Classification.Class == perfScanClassClean {
			timing = &split.Clean
			ratios = &cleanRatios
		} else if file.Classification.Class == perfScanClassStopped {
			split.StoppedFiles++
		}
		timing.Files++
		full := file.Axes[perfScanAxisFull]
		if full == nil || full.Status != perfScanStatusOK || full.GoMedianNs <= 0 || full.CMedianNs <= 0 {
			continue
		}
		timing.FilesOK++
		timing.GoTotalNs += full.GoMedianNs
		timing.CTotalNs += full.CMedianNs
		*ratios = append(*ratios, float64(full.GoMedianNs)/float64(full.CMedianNs))
	}
	if split.ClassifiedFiles == 0 {
		return
	}
	if split.Clean.CTotalNs > 0 {
		split.Clean.RatioByTotal = float64(split.Clean.GoTotalNs) / float64(split.Clean.CTotalNs)
	}
	if split.Error.CTotalNs > 0 {
		split.Error.RatioByTotal = float64(split.Error.GoTotalNs) / float64(split.Error.CTotalNs)
	}
	split.Clean.RatioMedianOfFiles = perfScanMedianFloat(cleanRatios)
	split.Error.RatioMedianOfFiles = perfScanMedianFloat(errorRatios)
	split.ErrorShare = float64(split.Error.Files) / float64(split.ClassifiedFiles)
	row.FullParseSplit = split
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
	provenance, err := perfScanRepositoryProvenance(cfg.HardGate)
	if err != nil {
		t.Fatalf("repository provenance: %v", err)
	}
	lockEntries, corpusCoverage, err := perfScanPrepareCorpusLock(&cfg)
	if err != nil {
		t.Fatalf("prepare authenticated corpus lock: %v", err)
	}

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

	langs := perfScanSweepLanguages(t, cfg, lockEntries)
	if len(langs) == 0 {
		t.Fatalf("no languages selected: set %s or provide a corpus root with per-language dirs", perfScanEnvLangs)
	}
	corpusCoverage = perfScanFinalizeCorpusCoverage(corpusCoverage, lockEntries, langs)
	t.Logf("perf scan sweep: %d language(s): %s", len(langs), strings.Join(langs, ","))
	t.Logf("perf scan out dir: %s", absOut)

	board := &perfScanScoreboard{
		Schema:      perfScanSchema,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		GitRevision: provenance.Revision,
		GitClean:    provenance.Clean,
		Config:      cfg,
		Corpus:      corpusCoverage,
		Summary:     map[string]int{},
		Host: perfScanHost{
			Hostname:     perfScanHostname(),
			GOOS:         runtime.GOOS,
			GOARCH:       runtime.GOARCH,
			NumCPU:       runtime.NumCPU(),
			GOMAXPROCS:   runtime.GOMAXPROCS(0),
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
	board.FullParseSplit = perfScanAggregateFleetCleanErrorSplit(board.Languages)
	board.Host.LoadavgEnd = perfScanReadLoadavg()
	if contended, note := perfScanContended(); contended {
		board.Config.Contended = true
		if board.Config.ContendedNote == "" {
			board.Config.ContendedNote = "end of run: " + note
		} else {
			board.Config.ContendedNote += "; end of run: " + note
		}
		board.Notes = append(board.Notes, "CONTENDED END OF RUN — measurements are not authoritative ("+note+").")
	}
	gate := perfScanEvaluateHardGate(board)
	board.Gate = &gate

	if err := perfScanWriteScoreboard(absOut, board); err != nil {
		t.Fatalf("write scoreboard: %v", err)
	}
	t.Logf("scoreboard: %s", filepath.Join(absOut, "scoreboard.json"))
	t.Logf("scoreboard: %s", filepath.Join(absOut, "scoreboard.md"))
	if cfg.HardGate && gate.Status != perfScanGatePass {
		t.Errorf("hard zero-cliff gate failed: %d finding(s), %d/%d full files evaluated; scoreboard: %s",
			len(gate.Failures), gate.FullFilesEvaluated, gate.FilesExpected, filepath.Join(absOut, "scoreboard.json"))
	}
}

func perfScanSweepLanguages(t *testing.T, cfg perfScanConfig, lockEntries map[string]realCorpusBenchmarkLockEntry) []string {
	if len(cfg.Languages) > 0 {
		return append([]string(nil), cfg.Languages...)
	}
	if cfg.RequireFleet && len(lockEntries) > 0 {
		out := make([]string, 0, len(lockEntries))
		for lang := range lockEntries {
			out = append(out, lang)
		}
		sort.Strings(out)
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
	runErr, childStop := perfScanRunChild(ctx, cmd, cfg.ChildRSSMB)
	elapsed := time.Since(start)

	fragment, fragErr := perfScanReadLangFragment(absOut, lang)
	if childStop != nil && fragment != nil {
		childStop = perfScanStopWithActiveAttempt(childStop, fragment)
		fragment.Stop = childStop
	}
	timedOut := childStop != nil && childStop.Class == perfScanStopWallTimeout

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
		if childStop != nil {
			stopDetail = childStop.Detail
		}
		if runErr != nil && fragment.Status == perfScanStatusOK {
			fragment.Notes = append(fragment.Notes, "child exited with error after fragment write: "+stopDetail)
		}
		if fragment.Status == perfScanStatusRunning {
			fragment.Status = perfScanLanguageStopStatus(childStop)
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
		} else if childStop != nil {
			status = perfScanLanguageStopStatus(childStop)
			detail = childStop.Detail + " before any file completed"
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
			Stop:      childStop,
		}
	}
}

func perfScanRunChild(ctx context.Context, cmd *exec.Cmd, rssLimitMB int) (error, *perfScanStop) {
	if rssLimitMB <= 0 {
		err := cmd.Run()
		return err, perfScanChildExitStop(ctx, err)
	}
	if err := cmd.Start(); err != nil {
		return err, nil
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
	checkRSS := func() (bool, *perfScanStop) {
		if cmd.Process == nil {
			return false, nil
		}
		rssBytes, ok := perfScanProcessRSSBytes(cmd.Process.Pid)
		if !ok || rssBytes < limitBytes {
			return false, nil
		}
		detail := fmt.Sprintf("child rss exceeded %d MiB limit (rss=%d MiB)",
			rssLimitMB, (rssBytes+(1<<20)-1)>>20)
		return true, &perfScanStop{Class: perfScanStopRSSLimit, Detail: detail}
	}
	for {
		select {
		case err := <-done:
			return err, perfScanChildExitStop(ctx, err)
		default:
		}
		if kill, stop := checkRSS(); kill {
			_ = cmd.Process.Kill()
			return <-done, stop
		}
		select {
		case err := <-done:
			return err, perfScanChildExitStop(ctx, err)
		case <-ctx.Done():
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			return <-done, &perfScanStop{Class: perfScanStopWallTimeout, Detail: "language subprocess exceeded hard wall timeout"}
		case <-ticker.C:
		}
	}
}

func perfScanChildExitStop(ctx context.Context, err error) *perfScanStop {
	if ctx != nil && ctx.Err() == context.DeadlineExceeded {
		return &perfScanStop{Class: perfScanStopWallTimeout, Detail: "language subprocess exceeded hard wall timeout"}
	}
	return perfScanUnexpectedChildStop(err)
}

func perfScanUnexpectedChildStop(err error) *perfScanStop {
	if err == nil {
		return nil
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return nil
	}
	waitStatus, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !waitStatus.Signaled() {
		return nil
	}
	signal := waitStatus.Signal()
	class := perfScanStopProcessSignal
	if signal == syscall.SIGKILL {
		class = perfScanStopOOMOrKill
	}
	return &perfScanStop{
		Class:  class,
		Reason: signal.String(),
		Detail: fmt.Sprintf("language subprocess terminated by %s", signal),
	}
}

func perfScanStopWithActiveAttempt(stop *perfScanStop, row *perfScanLanguage) *perfScanStop {
	if stop == nil || row == nil {
		return stop
	}
	out := *stop
	out.Implementation = row.ActiveImpl
	out.Phase = row.ActivePhase
	if row.ActiveAttempt != nil {
		out.Attempt = *row.ActiveAttempt
	}
	return &out
}

func perfScanLanguageStopStatus(stop *perfScanStop) string {
	if stop == nil {
		return "error"
	}
	switch stop.Class {
	case perfScanStopWallTimeout:
		return "lang_timeout"
	case perfScanStopRSSLimit:
		return "rss_limit"
	case perfScanStopOOMOrKill:
		return "oom_or_kill"
	case perfScanStopProcessSignal:
		return "process_signal"
	default:
		return "error"
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
	return perfScanWriteScoreboardWithRename(outDir, board, os.Rename)
}

func perfScanWriteScoreboardWithRename(outDir string, board *perfScanScoreboard, rename func(string, string) error) error {
	data, err := json.MarshalIndent(board, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	markdownTemp, err := perfScanStageScoreboardFile(outDir, "scoreboard.md", []byte(perfScanRenderMarkdown(board)))
	if err != nil {
		return err
	}
	defer os.Remove(markdownTemp)
	jsonTemp, err := perfScanStageScoreboardFile(outDir, "scoreboard.json", append(data, '\n'))
	if err != nil {
		return err
	}
	defer os.Remove(jsonTemp)

	markdownPath := filepath.Join(outDir, "scoreboard.md")
	jsonPath := filepath.Join(outDir, "scoreboard.json")
	var markdownBackup string
	if info, statErr := os.Stat(markdownPath); statErr == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("existing %s is not a regular file", markdownPath)
		}
		backup, createErr := os.CreateTemp(outDir, ".scoreboard.md.backup-")
		if createErr != nil {
			return createErr
		}
		markdownBackup = backup.Name()
		if closeErr := backup.Close(); closeErr != nil {
			_ = os.Remove(markdownBackup)
			return closeErr
		}
		if removeErr := os.Remove(markdownBackup); removeErr != nil {
			return removeErr
		}
		if err := rename(markdownPath, markdownBackup); err != nil {
			return err
		}
		defer os.Remove(markdownBackup)
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	restoreMarkdown := func() error {
		_ = os.Remove(markdownPath)
		if markdownBackup != "" {
			return rename(markdownBackup, markdownPath)
		}
		return nil
	}

	// Publish Markdown first and JSON last. scoreboard.json is the commit
	// marker; if that final rename fails, roll Markdown back before returning.
	if err := rename(markdownTemp, markdownPath); err != nil {
		if restoreErr := restoreMarkdown(); restoreErr != nil {
			return fmt.Errorf("publish markdown: %v (restore: %v)", err, restoreErr)
		}
		return err
	}
	if err := rename(jsonTemp, jsonPath); err != nil {
		if restoreErr := restoreMarkdown(); restoreErr != nil {
			return fmt.Errorf("publish JSON: %v (restore markdown: %v)", err, restoreErr)
		}
		return err
	}
	if markdownBackup != "" {
		_ = os.Remove(markdownBackup)
		markdownBackup = ""
	}
	dir, err := os.Open(outDir)
	if err != nil {
		return err
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func perfScanStageScoreboardFile(outDir, name string, data []byte) (tempPath string, err error) {
	file, err := os.CreateTemp(outDir, "."+name+".tmp-")
	if err != nil {
		return "", err
	}
	tempPath = file.Name()
	cleanupPath := tempPath
	defer func() {
		if err != nil {
			_ = file.Close()
			_ = os.Remove(cleanupPath)
		}
	}()
	if err = file.Chmod(0o644); err != nil {
		return "", err
	}
	if _, err = file.Write(data); err != nil {
		return "", err
	}
	if err = file.Sync(); err != nil {
		return "", err
	}
	if err = file.Close(); err != nil {
		return "", err
	}
	return tempPath, nil
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

func perfScanFmtClassRatio(agg perfScanClassTiming) string {
	if agg.RatioByTotal <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.2fx", agg.RatioByTotal)
}

func perfScanFmtShare(share float64) string {
	return fmt.Sprintf("%.1f%%", share*100)
}

func perfScanRenderCleanErrorSplit(b *strings.Builder, rows []*perfScanLanguage) {
	hasSplit := false
	for _, row := range rows {
		if row.FullParseSplit != nil {
			hasSplit = true
			break
		}
	}
	if !hasSplit {
		return
	}
	fmt.Fprintf(b, "\n## Full-parse clean/error split\n\n")
	fmt.Fprintf(b, "`error` includes every non-clean file; `stopped` is the stopped subset. Only status=`ok` files contribute timing totals and ratios.\n\n")
	fmt.Fprintf(b, "| language | clean timed/files | clean Go | clean C | clean ratio | error timed/files | stopped | error Go | error C | error ratio | error share |\n")
	fmt.Fprintf(b, "|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, row := range rows {
		split := row.FullParseSplit
		if split == nil {
			continue
		}
		fmt.Fprintf(b, "| %s | %d/%d | %s | %s | %s | %d/%d | %d | %s | %s | %s | %s |\n",
			row.Language,
			split.Clean.FilesOK, split.Clean.Files,
			perfScanFmtNs(split.Clean.GoTotalNs), perfScanFmtNs(split.Clean.CTotalNs), perfScanFmtClassRatio(split.Clean),
			split.Error.FilesOK, split.Error.Files, split.StoppedFiles,
			perfScanFmtNs(split.Error.GoTotalNs), perfScanFmtNs(split.Error.CTotalNs), perfScanFmtClassRatio(split.Error),
			perfScanFmtShare(split.ErrorShare))
	}
}

func perfScanRenderNonCleanClassifications(b *strings.Builder, rows []*perfScanLanguage) {
	var lines []string
	for _, row := range rows {
		for _, file := range row.Files {
			if file.Classification == nil || file.Classification.Class == perfScanClassClean {
				continue
			}
			lines = append(lines, fmt.Sprintf("- **%s** `%s` class=%s status=%s — %s",
				row.Language, file.Path, file.Classification.Class, file.Classification.GoStatus, file.Classification.Reason))
		}
	}
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## Non-clean full-parse classifications\n\n")
	for _, line := range lines {
		fmt.Fprintf(b, "%s\n", line)
	}
}

func perfScanRenderMarkdown(board *perfScanScoreboard) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Go-vs-C real-corpus perf scoreboard\n\n")
	fmt.Fprintf(&b, "- schema: `%s` generated: %s\n", board.Schema, board.GeneratedAt)
	if board.GitRevision != "" {
		fmt.Fprintf(&b, "- measurement git revision: `%s`\n", board.GitRevision)
		fmt.Fprintf(&b, "- measurement git worktree clean: `%t`\n", board.GitClean)
	}
	if board.Reduction != nil {
		fmt.Fprintf(&b, "- reducer git revision: `%s`\n", board.Reduction.GitRevision)
		fmt.Fprintf(&b, "- reducer git worktree clean: `%t`\n", board.Reduction.GitClean)
	}
	fmt.Fprintf(&b, "- host: %s %s/%s cpus=%d gomaxprocs=%d %s\n",
		board.Host.Hostname, board.Host.GOOS, board.Host.GOARCH, board.Host.NumCPU, board.Host.GOMAXPROCS, board.Host.GoVersion)
	fmt.Fprintf(&b, "- loadavg start `%s` end `%s`\n", board.Host.LoadavgStart, board.Host.LoadavgEnd)
	fmt.Fprintf(&b, "- corpus: `%s` order=%s max_files=%d reps=%d warmup=%d file_budget=%dms axes=%s\n",
		board.Config.CorpusRoot, board.Config.Order, board.Config.MaxFiles,
		board.Config.Reps, board.Config.Warmup, board.Config.FileBudgetMS,
		strings.Join(board.Config.Axes, ","))
	fmt.Fprintf(&b, "- hard gate: `%t` require_fleet=`%t` max_full_ratio=`%.1fx` fast_full_ratio=`%.2fx`\n",
		board.Config.HardGate, board.Config.RequireFleet, perfScanHardFullRatio, perfScanFastFullRatio)
	if board.Corpus.LockSHA256 != "" {
		fmt.Fprintf(&b, "- corpus lock: `%s` sha256=`%s` languages=%d selected=%d\n",
			board.Corpus.LockPath, board.Corpus.LockSHA256, board.Corpus.LockLanguages, board.Corpus.SelectedLanguages)
	}
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
	for _, bucket := range []string{perfScanBucketLePoint1, perfScanBucketLe12, perfScanBucketLe2, perfScanBucketGt2, perfScanBucketCliff, perfScanBucketNoData} {
		if n := board.Summary[bucket]; n > 0 {
			fmt.Fprintf(&b, "- `%s`: %d\n", bucket, n)
		}
	}

	if board.Gate != nil {
		fmt.Fprintf(&b, "\n## Hard zero-cliff gate\n\n")
		fmt.Fprintf(&b, "- outcome: `%s`\n", strings.ToUpper(board.Gate.Status))
		fmt.Fprintf(&b, "- full files evaluated: `%d/%d` (measured rows `%d`)\n",
			board.Gate.FullFilesEvaluated, board.Gate.FilesExpected, board.Gate.FilesMeasured)
		fmt.Fprintf(&b, "- failures: `%d`; full files at or below `%.2fx`: `%d`\n",
			len(board.Gate.Failures), board.Gate.FastFullParseRatio, len(board.Gate.FastFullFiles))
		fmt.Fprintf(&b, "\nThis is a timing/resource gate. Structural and error-tree parity remain owned by the separate correctness suites.\n")
		if len(board.Gate.Failures) > 0 {
			fmt.Fprintf(&b, "\n### Gate failures\n\n")
			for _, finding := range board.Gate.Failures {
				fmt.Fprintf(&b, "- **%s** `%s` axis=%s kind=%s status=%s ratio=%.4fx — %s\n",
					finding.Language, finding.Path, finding.Axis, finding.Kind, finding.Status, finding.Ratio, finding.Detail)
			}
		}
		if len(board.Gate.FastFullFiles) > 0 {
			fmt.Fprintf(&b, "\n### Full-parse files at least 10x faster than C\n\n")
			for _, finding := range board.Gate.FastFullFiles {
				fmt.Fprintf(&b, "- **%s** `%s` ratio=%.4fx — %s\n", finding.Language, finding.Path, finding.Ratio, finding.Detail)
			}
		}
	}

	if split := board.FullParseSplit; split != nil {
		fmt.Fprintf(&b, "\n## Fleet full-parse clean/error split\n\n")
		fmt.Fprintf(&b, "- classified files: `%d`; stopped subset: `%d`; error share: `%s`\n",
			split.ClassifiedFiles, split.StoppedFiles, perfScanFmtShare(split.ErrorShare))
		fmt.Fprintf(&b, "- clean: `%d/%d` timed, Go `%s`, C `%s`, ratio `%s`\n",
			split.Clean.FilesOK, split.Clean.Files, perfScanFmtNs(split.Clean.GoTotalNs),
			perfScanFmtNs(split.Clean.CTotalNs), perfScanFmtClassRatio(split.Clean))
		fmt.Fprintf(&b, "- error/non-clean: `%d/%d` timed, Go `%s`, C `%s`, ratio `%s`\n",
			split.Error.FilesOK, split.Error.Files, perfScanFmtNs(split.Error.GoTotalNs),
			perfScanFmtNs(split.Error.CTotalNs), perfScanFmtClassRatio(split.Error))
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

	perfScanRenderCleanErrorSplit(&b, board.Languages)

	var cliffLines []string
	for _, row := range board.Languages {
		for _, file := range row.Files {
			for _, axis := range []string{perfScanAxisFull, perfScanAxisNoEdit, perfScanAxisEdit} {
				fa, ok := file.Axes[axis]
				if !ok {
					continue
				}
				isCliff := fa.Verdict == perfScanBucketCliff || perfScanIsGoStop(fa.Stop)
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

	perfScanRenderNonCleanClassifications(&b, board.Languages)

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
	fmt.Fprintf(&b, "\nBuckets: `%s` / `%s` / `%s` / `%s` / `%s` (ratio = Go median / C median; per-language ratio-by-total = sum of Go file medians / sum of C file medians; `>=` marks a lower bound from a budget timeout).\n",
		perfScanBucketLePoint1, perfScanBucketLe12, perfScanBucketLe2, perfScanBucketGt2, perfScanBucketCliff)
	return b.String()
}

// ---------------------------------------------------------------------------
// Pure-helper self-checks (no corpus, no C grammars, no subprocesses).
// ---------------------------------------------------------------------------

func TestPerfScanHelpersUnit(t *testing.T) {
	if got := perfScanVerdictBucket(0.10); got != perfScanBucketLePoint1 {
		t.Fatalf("bucket(0.10)=%s", got)
	}
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
	if got := perfScanGoParserStop(gotreesitter.ParseStopMemoryBudget, 5*time.Second); got.Class != perfScanStopParserBudget || got.Reason != string(gotreesitter.ParseStopMemoryBudget) {
		t.Fatalf("memory stop classification = %+v", got)
	}
	if got := perfScanGoParserStop(gotreesitter.ParseStopTimeout, 5*time.Second); got.Class != perfScanStopParserTimeout {
		t.Fatalf("timeout stop classification = %+v", got)
	}
	axisStop := &perfScanFileAxis{}
	perfScanRecordAttemptStop(axisStop, perfScanAttempt{stop: &perfScanStop{Class: perfScanStopParserBudget, Implementation: "go"}}, "rep", 1)
	perfScanRecordAttemptStop(axisStop, perfScanAttempt{stop: &perfScanStop{Class: perfScanStopCTimeout, Implementation: "c"}}, "rep", 2)
	if axisStop.Stop == nil || axisStop.Stop.Implementation != "go" || axisStop.Stop.Class != perfScanStopParserBudget {
		t.Fatalf("C stop overwrote causal Go stop: %+v", axisStop.Stop)
	}
	if got, err := perfScanExpectedCorpusLockDigest(); err != nil || got != "cf108b005fe41c4513bae14eafd4f4ec72e64454ca2eb6bbf4d18d25caab24f2" {
		t.Fatalf("checked corpus lock digest = %q,%v", got, err)
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

func TestPerfScanHardGateUnit(t *testing.T) {
	fullAxis := func(ratio float64) *perfScanFileAxis {
		return &perfScanFileAxis{
			Status:     perfScanStatusOK,
			GoMedianNs: int64(ratio * 1000),
			CMedianNs:  1000,
			Ratio:      ratio,
		}
	}
	board := &perfScanScoreboard{
		Config: perfScanConfig{RequireFleet: true},
		Corpus: perfScanCorpusCoverage{LockLanguages: 1, SelectedLanguages: 1},
		Languages: []*perfScanLanguage{{
			Language:      "go",
			Status:        perfScanStatusOK,
			FilesSelected: 3,
			FilesMeasured: 3,
			Files: []*perfScanFile{
				{Path: "boundary.go", Axes: map[string]*perfScanFileAxis{perfScanAxisFull: fullAxis(10)}},
				{Path: "fast.go", Axes: map[string]*perfScanFileAxis{perfScanAxisFull: fullAxis(0.10)}},
				{Path: "cliff.go", Axes: map[string]*perfScanFileAxis{perfScanAxisFull: fullAxis(10.001)}},
			},
		}},
	}

	report := perfScanEvaluateHardGate(board)
	if report.Status != perfScanGateFail || len(report.Failures) != 1 || report.Failures[0].Kind != "ratio" {
		t.Fatalf("cliff gate report = %+v", report)
	}
	if report.FullFilesEvaluated != 3 || len(report.FastFullFiles) != 1 || report.FastFullFiles[0].Path != "fast.go" {
		t.Fatalf("full/fast file accounting = %+v", report)
	}

	board.Languages[0].Files[2].Axes[perfScanAxisFull] = fullAxis(10)
	report = perfScanEvaluateHardGate(board)
	if report.Status != perfScanGatePass || len(report.Failures) != 0 {
		t.Fatalf("exact 10x boundary must pass: %+v", report)
	}

	board.Languages[0].Files[0].Axes[perfScanAxisNoEdit] = &perfScanFileAxis{
		Status: "go_budget_stop",
		Stop: &perfScanStop{
			Class:          perfScanStopParserBudget,
			Reason:         string(gotreesitter.ParseStopMemoryBudget),
			Implementation: "go",
		},
	}
	report = perfScanEvaluateHardGate(board)
	if report.Status != perfScanGateFail || len(report.Failures) != 1 || report.Failures[0].Kind != "go_stop" {
		t.Fatalf("Go parser-budget stop must fail independently of full ratio: %+v", report)
	}
}

func TestPerfScanFullParseClassificationUnit(t *testing.T) {
	src := []byte("abcdef")
	cleanRoot := gotreesitter.NewLeafNode(1, true, 0, uint32(len(src)), gotreesitter.Point{}, gotreesitter.Point{Column: uint32(len(src))})
	cleanTree := gotreesitter.NewTree(cleanRoot, src, nil)
	clean := perfScanClassifyGoFull(cleanTree, perfScanAttempt{}, len(src))
	cleanTree.Release()
	if clean.Class != perfScanClassClean || !clean.FullSpan || clean.RootHasError || clean.StoppedEarly || clean.GoStatus != perfScanStatusOK {
		t.Fatalf("clean classification = %+v", clean)
	}

	prefixRoot := gotreesitter.NewLeafNode(1, true, 1, uint32(len(src)), gotreesitter.Point{Column: 1}, gotreesitter.Point{Column: uint32(len(src))})
	prefixTree := gotreesitter.NewTree(prefixRoot, src, nil)
	prefix := perfScanClassifyGoFull(prefixTree, perfScanAttempt{}, len(src))
	prefixTree.Release()
	if prefix.Class != perfScanClassError || prefix.FullSpan || !strings.Contains(prefix.Reason, "want [0,6)") {
		t.Fatalf("prefix-only classification = %+v", prefix)
	}

	errorRoot := gotreesitter.NewLeafNode(gotreesitter.Symbol(65535), true, 0, uint32(len(src)), gotreesitter.Point{}, gotreesitter.Point{Column: uint32(len(src))})
	errorTree := gotreesitter.NewTree(errorRoot, src, nil)
	errorClass := perfScanClassifyGoFull(errorTree, perfScanAttempt{}, len(src))
	errorTree.Release()
	if errorClass.Class != perfScanClassError || !errorClass.FullSpan || !errorClass.RootHasError || !strings.Contains(errorClass.Reason, "ERROR") {
		t.Fatalf("error classification = %+v", errorClass)
	}

	stopped := perfScanClassifyGoFull(nil, perfScanAttempt{
		status: "go_budget_stop",
		detail: "parse stopped early (memory_budget)",
		stop: &perfScanStop{
			Class:  perfScanStopParserBudget,
			Reason: string(gotreesitter.ParseStopMemoryBudget),
			Detail: "parse stopped early (memory_budget)",
		},
	}, len(src))
	if stopped.Class != perfScanClassStopped || !stopped.StoppedEarly || stopped.StopReason != string(gotreesitter.ParseStopMemoryBudget) || stopped.GoStatus != "go_budget_stop" {
		t.Fatalf("stopped classification = %+v", stopped)
	}
}

func perfScanSyntheticCleanTree(src []byte) *gotreesitter.Tree {
	root := gotreesitter.NewLeafNode(1, true, 0, uint32(len(src)), gotreesitter.Point{}, gotreesitter.Point{Column: uint32(len(src))})
	return gotreesitter.NewTree(root, src, nil)
}

func perfScanSyntheticTimeoutAttempt() perfScanAttempt {
	return perfScanAttempt{
		ns:     500,
		status: "go_timeout",
		detail: "timed Go parse stopped",
		stop: &perfScanStop{
			Class:          perfScanStopParserTimeout,
			Reason:         string(gotreesitter.ParseStopTimeout),
			Implementation: "go",
			Detail:         "timed Go parse stopped",
		},
	}
}

func TestPerfScanWarmupClassificationSurvivesTimedFailureUnit(t *testing.T) {
	src := []byte("{}")
	goCalls := 0
	m := &perfScanLangMeasurer{
		cfg:    perfScanConfig{Warmup: 1, Reps: 1},
		budget: time.Second,
		goFullAttempt: func(_ []byte, keepTree bool) (*gotreesitter.Tree, perfScanAttempt) {
			goCalls++
			if goCalls == 1 {
				if !keepTree {
					t.Fatal("first warmup did not retain its classification tree")
				}
				return perfScanSyntheticCleanTree(src), perfScanAttempt{ns: 100}
			}
			return nil, perfScanSyntheticTimeoutAttempt()
		},
		cFullAttempt: func([]byte, *sitter.Tree, bool) (*sitter.Tree, perfScanAttempt) {
			return nil, perfScanAttempt{ns: 50}
		},
	}

	axis, classification := m.measureFull(src)
	if goCalls != 2 {
		t.Fatalf("Go full attempts=%d, want warmup+timed=2", goCalls)
	}
	if classification == nil || classification.Class != perfScanClassClean {
		t.Fatalf("timed failure overwrote first warmup classification: %+v", classification)
	}
	if axis.Status != "go_timeout" || axis.Stop == nil || axis.Stop.Phase != "rep" {
		t.Fatalf("timed failure was not retained on timing axis: %+v", axis)
	}
}

func TestPerfScanRejectedErrorRootRetainsClassificationFactsUnit(t *testing.T) {
	src := []byte("invalid")
	var retained *gotreesitter.Tree
	m := &perfScanLangMeasurer{
		cfg:    perfScanConfig{Warmup: 1, Reps: 1},
		budget: time.Second,
		cFullAttempt: func([]byte, *sitter.Tree, bool) (*sitter.Tree, perfScanAttempt) {
			return nil, perfScanAttempt{ns: 50}
		},
	}
	m.goFullAttempt = func(_ []byte, keepTree bool) (*gotreesitter.Tree, perfScanAttempt) {
		root := gotreesitter.NewLeafNode(gotreesitter.Symbol(65535), true, 0, uint32(len(src)), gotreesitter.Point{}, gotreesitter.Point{Column: uint32(len(src))})
		tree := gotreesitter.NewTree(root, src, nil)
		retained = tree
		return m.classifyGoAttemptWithRetention(tree, nil, "", src, keepTree, keepTree, perfScanAttempt{ns: 100})
	}

	axis, classification := m.measureFull(src)
	if axis.Status != "go_error" {
		t.Fatalf("rejected ERROR-root axis = %+v, want go_error", axis)
	}
	if classification == nil || classification.Class != perfScanClassError || !classification.FullSpan || !classification.RootHasError {
		t.Fatalf("rejected ERROR-root classification = %+v", classification)
	}
	encoded, err := json.Marshal(classification)
	if err != nil {
		t.Fatalf("marshal classification: %v", err)
	}
	if !strings.Contains(string(encoded), `"full_span":true`) || !strings.Contains(string(encoded), `"root_has_error":true`) {
		t.Fatalf("serialized ERROR-root facts = %s", encoded)
	}
	if retained == nil || retained.RootNode() != nil {
		t.Fatalf("classification tree was not released after measurement")
	}
}

func TestPerfScanWarmupZeroTimedFailureStillRunsClassificationProbeUnit(t *testing.T) {
	src := []byte("{}")
	goCalls := 0
	var phases []string
	m := &perfScanLangMeasurer{
		cfg:    perfScanConfig{Warmup: 0, Reps: 1},
		budget: time.Second,
		progress: func(axis, impl, phase string, attempt int) {
			phases = append(phases, fmt.Sprintf("%s/%s/%s/%d", axis, impl, phase, attempt))
		},
		goFullAttempt: func(_ []byte, keepTree bool) (*gotreesitter.Tree, perfScanAttempt) {
			goCalls++
			if goCalls == 1 {
				return nil, perfScanSyntheticTimeoutAttempt()
			}
			if !keepTree {
				t.Fatal("classification probe did not retain its tree")
			}
			return perfScanSyntheticCleanTree(src), perfScanAttempt{ns: 100}
		},
		cFullAttempt: func([]byte, *sitter.Tree, bool) (*sitter.Tree, perfScanAttempt) {
			return nil, perfScanAttempt{ns: 50}
		},
	}

	axis, classification := m.measureFull(src)
	if goCalls != 2 || len(phases) == 0 || phases[len(phases)-1] != "full/go/classify/1" {
		t.Fatalf("warmup=0 attempts=%d phases=%v, want timed failure then distinct probe", goCalls, phases)
	}
	if classification == nil || classification.Class != perfScanClassClean {
		t.Fatalf("post-failure classification probe = %+v", classification)
	}
	if axis.Status != "go_timeout" || axis.Stop == nil || axis.Stop.Phase != "rep" {
		t.Fatalf("classification probe changed timed failure: %+v", axis)
	}
}

func TestPerfScanClassificationProbeFailureDoesNotAffectGateUnit(t *testing.T) {
	src := []byte("{}")
	goCalls := 0
	m := &perfScanLangMeasurer{
		cfg:    perfScanConfig{Warmup: 0, Reps: 1},
		budget: time.Second,
		goFullAttempt: func([]byte, bool) (*gotreesitter.Tree, perfScanAttempt) {
			goCalls++
			if goCalls == 1 {
				return nil, perfScanAttempt{ns: 200}
			}
			return nil, perfScanAttempt{
				status: "go_budget_stop",
				detail: "classification probe stopped",
				stop: &perfScanStop{
					Class:          perfScanStopParserBudget,
					Reason:         string(gotreesitter.ParseStopMemoryBudget),
					Implementation: "go",
					Detail:         "classification probe stopped",
				},
			}
		},
		cFullAttempt: func([]byte, *sitter.Tree, bool) (*sitter.Tree, perfScanAttempt) {
			return nil, perfScanAttempt{ns: 100}
		},
	}

	axis, classification := m.measureFull(src)
	if classification == nil || classification.Class != perfScanClassStopped {
		t.Fatalf("classification-only failure = %+v", classification)
	}
	if axis.Status != perfScanStatusOK || axis.Stop != nil || axis.GoMedianNs != 200 || axis.CMedianNs != 100 || axis.Ratio != 2 {
		t.Fatalf("classification-only failure mutated timing axis: %+v", axis)
	}
	file := &perfScanFile{Path: "probe.json", Classification: classification, Axes: map[string]*perfScanFileAxis{perfScanAxisFull: axis}}
	board := &perfScanScoreboard{Languages: []*perfScanLanguage{{
		Language: "json", Status: perfScanStatusOK, FilesSelected: 1, FilesMeasured: 1, Files: []*perfScanFile{file},
	}}}
	report := perfScanEvaluateHardGate(board)
	if report.Status != perfScanGatePass || len(report.Failures) != 0 || report.FullFilesEvaluated != 1 {
		t.Fatalf("classification-only failure changed hard gate: %+v", report)
	}
}

func TestPerfScanCleanErrorSplitUnit(t *testing.T) {
	file := func(path, class string, goNs, cNs int64, status string) *perfScanFile {
		axis := &perfScanFileAxis{Status: status, GoMedianNs: goNs, CMedianNs: cNs}
		if goNs > 0 && cNs > 0 {
			axis.Ratio = float64(goNs) / float64(cNs)
		}
		return &perfScanFile{
			Path: path,
			Classification: &perfScanFileClassification{
				Class:    class,
				Reason:   class + " fixture",
				GoStatus: status,
			},
			Axes: map[string]*perfScanFileAxis{perfScanAxisFull: axis},
		}
	}

	t.Run("mixed", func(t *testing.T) {
		row := &perfScanLanguage{Language: "mixed", Files: []*perfScanFile{
			file("clean-a", perfScanClassClean, 200, 100, perfScanStatusOK),
			file("clean-b", perfScanClassClean, 400, 100, perfScanStatusOK),
			file("error-a", perfScanClassError, 900, 100, perfScanStatusOK),
			file("stopped-a", perfScanClassStopped, 0, 0, "go_budget_stop"),
		}}
		perfScanAggregateCleanErrorSplit(row)
		split := row.FullParseSplit
		if split == nil || split.ClassifiedFiles != 4 || split.Clean.Files != 2 || split.Clean.FilesOK != 2 || split.Error.Files != 2 || split.Error.FilesOK != 1 || split.StoppedFiles != 1 {
			t.Fatalf("mixed counts = %+v", split)
		}
		if split.Clean.GoTotalNs != 600 || split.Clean.CTotalNs != 200 || split.Clean.RatioByTotal != 3 || split.Error.GoTotalNs != 900 || split.Error.CTotalNs != 100 || split.Error.RatioByTotal != 9 || split.ErrorShare != 0.5 {
			t.Fatalf("mixed timings = %+v", split)
		}
	})

	t.Run("all-clean", func(t *testing.T) {
		row := &perfScanLanguage{Files: []*perfScanFile{
			file("clean-a", perfScanClassClean, 100, 100, perfScanStatusOK),
			file("clean-b", perfScanClassClean, 200, 100, perfScanStatusOK),
		}}
		perfScanAggregateCleanErrorSplit(row)
		if row.FullParseSplit == nil || row.FullParseSplit.Clean.Files != 2 || row.FullParseSplit.Error.Files != 0 || row.FullParseSplit.ErrorShare != 0 {
			t.Fatalf("all-clean split = %+v", row.FullParseSplit)
		}
	})

	t.Run("all-error", func(t *testing.T) {
		row := &perfScanLanguage{Files: []*perfScanFile{
			file("error-a", perfScanClassError, 300, 100, perfScanStatusOK),
			file("stopped-a", perfScanClassStopped, 0, 0, "go_timeout"),
		}}
		perfScanAggregateCleanErrorSplit(row)
		if row.FullParseSplit == nil || row.FullParseSplit.Clean.Files != 0 || row.FullParseSplit.Error.Files != 2 || row.FullParseSplit.StoppedFiles != 1 || row.FullParseSplit.ErrorShare != 1 {
			t.Fatalf("all-error split = %+v", row.FullParseSplit)
		}
	})
}

func TestPerfScanCleanErrorSerializationAndMarkdownUnit(t *testing.T) {
	row := &perfScanLanguage{
		Language:      "synthetic",
		Status:        perfScanStatusOK,
		FilesSelected: 2,
		FilesMeasured: 2,
		Axes:          map[string]*perfScanLangAxis{},
		Files: []*perfScanFile{
			{
				Path: "clean.go",
				Classification: &perfScanFileClassification{
					Class: perfScanClassClean, Reason: "accepted full-span Go tree without ERROR nodes", GoStatus: perfScanStatusOK, FullSpan: true,
				},
				Axes: map[string]*perfScanFileAxis{perfScanAxisFull: {Status: perfScanStatusOK, GoMedianNs: 200, CMedianNs: 100, Ratio: 2}},
			},
			{
				Path: "broken.go",
				Classification: &perfScanFileClassification{
					Class: perfScanClassError, Reason: "Go root contains ERROR nodes", GoStatus: perfScanStatusOK, FullSpan: true, RootHasError: true,
				},
				Axes: map[string]*perfScanFileAxis{perfScanAxisFull: {Status: perfScanStatusOK, GoMedianNs: 600, CMedianNs: 100, Ratio: 6}},
			},
		},
	}
	perfScanAggregateCleanErrorSplit(row)
	board := &perfScanScoreboard{
		Schema:    perfScanSchema,
		Config:    perfScanConfig{Axes: []string{perfScanAxisFull}},
		Summary:   map[string]int{},
		Languages: []*perfScanLanguage{row},
	}

	data, err := json.Marshal(board)
	if err != nil {
		t.Fatalf("marshal scoreboard: %v", err)
	}
	text := string(data)
	for _, want := range []string{`"classification"`, `"class":"error"`, `"full_parse_split"`, `"error_share":0.5`, `"ratio_by_total":6`} {
		if !strings.Contains(text, want) {
			t.Fatalf("scoreboard JSON missing %s: %s", want, text)
		}
	}

	var old perfScanFile
	if err := json.Unmarshal([]byte(`{"path":"old.go","bytes":1,"axes":{}}`), &old); err != nil {
		t.Fatalf("unmarshal legacy file row: %v", err)
	}
	if old.Classification != nil {
		t.Fatalf("legacy row unexpectedly classified: %+v", old.Classification)
	}

	markdown := perfScanRenderMarkdown(board)
	for _, want := range []string{
		"## Full-parse clean/error split",
		"| synthetic | 1/1 |",
		"6.00x | 50.0% |",
		"## Non-clean full-parse classifications",
		"`broken.go` class=error status=ok — Go root contains ERROR nodes",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("scoreboard markdown missing %q:\n%s", want, markdown)
		}
	}
}

func TestPerfScanWarmupZeroClassifiesAfterTimedSamplesUnit(t *testing.T) {
	const lang = "json"
	entry, ok := parityEntriesByName[lang]
	if !ok {
		t.Fatal("json registry entry is unavailable")
	}
	report, ok := paritySupportForName(lang)
	if !ok {
		t.Fatal("json support report is unavailable")
	}
	goLang := entry.Language()
	if goLang == nil {
		t.Fatal("json Go language is nil")
	}
	cLang, err := ParityCLanguage(lang)
	if err != nil {
		t.Fatalf("load JSON C language: %v", err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("set JSON C language: %v", err)
	}

	const reps = 2
	var phases []string
	m := &perfScanLangMeasurer{
		cfg:    perfScanConfig{Reps: reps, Warmup: 0},
		lang:   lang,
		entry:  entry,
		report: report,
		goLang: goLang,
		cLang:  cLang,
		goPsr:  gotreesitter.NewParser(goLang),
		cPsr:   cParser,
		budget: 5 * time.Second,
		progress: func(axis, impl, phase string, attempt int) {
			phases = append(phases, fmt.Sprintf("%s/%s/%s/%d", axis, impl, phase, attempt))
		},
	}
	m.goPsr.SetTimeoutMicros(uint64(m.budget.Microseconds()))
	m.cPsr.SetTimeoutMicros(uint64(m.budget.Microseconds()))

	axis, classification := m.measureFull([]byte(`{"answer":42}`))
	if axis.Status != perfScanStatusOK || axis.GoMedianNs <= 0 || axis.CMedianNs <= 0 || axis.Ratio <= 0 {
		t.Fatalf("warmup=0 full axis lost successful timing totals: %+v", axis)
	}
	if classification == nil || classification.Class != perfScanClassClean {
		t.Fatalf("warmup=0 classification = %+v", classification)
	}
	want := []string{
		"full/go/rep/1",
		"full/c/rep/1",
		"full/go/rep/2",
		"full/c/rep/2",
		"full/go/classify/1",
	}
	if strings.Join(phases, ",") != strings.Join(want, ",") {
		t.Fatalf("warmup=0 phase order = %v, want %v", phases, want)
	}
}
