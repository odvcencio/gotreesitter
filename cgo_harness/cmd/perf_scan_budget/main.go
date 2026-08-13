package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
)

const (
	budgetSchema       = "gts-perf-ratio-budgets/v1"
	scoreboardSchemaV1 = "gts-perf-scan/v1"
	scoreboardSchemaV2 = "gts-perf-scan/v2"

	axisFull   = "full"
	axisNoEdit = "noedit"
	statusOK   = "ok"

	hardMaxFullParseRatio = 10.0
	fastFullParseRatio    = 0.10
)

type budgetFile struct {
	Schema           string                 `json:"schema"`
	GeneratedAt      string                 `json:"generated_at"`
	MeasurementBasis budgetMeasurementBasis `json:"measurement_basis"`
	Languages        map[string]budgetLang  `json:"languages"`
}

type budgetMeasurementBasis struct {
	Reps                  int      `json:"reps"`
	Warmup                int      `json:"warmup"`
	FileBudgetMS          int      `json:"file_budget_ms"`
	MaxFiles              int      `json:"max_files,omitempty"`
	Order                 string   `json:"order,omitempty"`
	Axes                  []string `json:"axes"`
	ExcludePaths          []string `json:"exclude_paths,omitempty"`
	HardMaxFullParseRatio float64  `json:"hard_max_full_parse_ratio"`
	FastFullParseRatio    float64  `json:"fast_full_parse_ratio"`
	CorpusLockSHA256      string   `json:"corpus_lock_sha256"`
}

type budgetLang struct {
	Status     string     `json:"status"`
	FullAxis   budgetAxis `json:"full_axis"`
	NoEditAxis budgetAxis `json:"noedit_axis"`
}

type budgetAxis struct {
	MaxTimeouts           int  `json:"max_timeouts"`
	MaxErrors             *int `json:"max_errors"`
	MaxCReferenceFailures *int `json:"max_c_reference_failures,omitempty"`

	MaxRatioByTotal       float64 `json:"max_ratio_by_total"`
	MaxRatioMedianOfFiles float64 `json:"max_ratio_median_of_files"`
}

type scoreboardFile struct {
	Schema    string           `json:"schema"`
	Config    scoreboardConfig `json:"config"`
	Languages []scoreboardLang `json:"languages"`
	Corpus    scoreboardCorpus `json:"corpus_coverage"`
	Gate      *scoreboardGate  `json:"hard_gate,omitempty"`
}

type scoreboardConfig struct {
	Reps             int      `json:"reps"`
	Warmup           int      `json:"warmup"`
	FileBudgetMS     int      `json:"file_budget_ms"`
	MaxFiles         int      `json:"max_files"`
	Order            string   `json:"order"`
	Axes             []string `json:"axes"`
	ExcludePaths     []string `json:"exclude_paths,omitempty"`
	HardGate         bool     `json:"hard_gate"`
	RequireFleet     bool     `json:"require_fleet"`
	RuntimeEvidence  bool     `json:"runtime_evidence,omitempty"`
	CorpusLockSHA256 string   `json:"corpus_lock_sha256,omitempty"`
}

type scoreboardLang struct {
	Language      string                    `json:"language"`
	Status        string                    `json:"status"`
	FilesSelected int                       `json:"files_selected"`
	FilesMeasured int                       `json:"files_measured"`
	Axes          map[string]scoreboardAxis `json:"axes"`
	Files         []scoreboardFileRow       `json:"files"`
	ActiveFile    string                    `json:"active_file,omitempty"`
	ActiveAxis    string                    `json:"active_axis,omitempty"`
	Stop          *scoreboardStop           `json:"stop,omitempty"`
}

type scoreboardAxis struct {
	FilesOK            int     `json:"files_ok"`
	RatioByTotal       float64 `json:"ratio_by_total"`
	RatioMedianOfFiles float64 `json:"ratio_median_of_files"`
	GoTimeouts         int     `json:"go_timeouts"`
}

type scoreboardFileRow struct {
	Path string                        `json:"path"`
	Axes map[string]scoreboardFileAxis `json:"axes"`
}

type scoreboardFileAxis struct {
	Status     string          `json:"status"`
	Detail     string          `json:"detail,omitempty"`
	GoMedianNs int64           `json:"go_median_ns,omitempty"`
	CMedianNs  int64           `json:"c_median_ns,omitempty"`
	Ratio      float64         `json:"ratio,omitempty"`
	Stop       *scoreboardStop `json:"stop,omitempty"`
}

type scoreboardStop struct {
	Class          string `json:"class"`
	Reason         string `json:"reason,omitempty"`
	Implementation string `json:"implementation,omitempty"`
	Phase          string `json:"phase,omitempty"`
	Attempt        int    `json:"attempt,omitempty"`
	Detail         string `json:"detail,omitempty"`
}

type scoreboardGate struct {
	Status             string                  `json:"status"`
	MaxFullParseRatio  float64                 `json:"max_full_parse_ratio"`
	FastFullParseRatio float64                 `json:"fast_full_parse_ratio"`
	FilesExpected      int                     `json:"files_expected"`
	FilesMeasured      int                     `json:"files_measured"`
	FullFilesEvaluated int                     `json:"full_files_evaluated"`
	Failures           []scoreboardGateFinding `json:"failures,omitempty"`
}

type scoreboardCorpus struct {
	LockSHA256          string   `json:"lock_sha256,omitempty"`
	LockLanguages       int      `json:"lock_languages"`
	SelectedLanguages   int      `json:"selected_languages"`
	MissingFromLock     []string `json:"missing_from_lock,omitempty"`
	MissingFromRegistry []string `json:"missing_from_registry,omitempty"`
}

type scoreboardGateFinding struct {
	Kind     string `json:"kind"`
	Language string `json:"language,omitempty"`
	Path     string `json:"path,omitempty"`
	Axis     string `json:"axis,omitempty"`
	Status   string `json:"status,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

type evalFinding struct {
	Language string
	Path     string
	Axis     string
	Metric   string
	Got      string
	Want     string
}

func main() {
	var (
		budgetPath            string
		scoreboardPath        string
		langsRaw              string
		requireAllBudgetLangs bool
		strictConfig          bool
		hardGateOnly          bool
		outMD                 string
	)

	flag.StringVar(&budgetPath, "budget", "perf_scan/perf_ratio_budgets.json", "perf ratio budget JSON path")
	flag.StringVar(&scoreboardPath, "scoreboard", "", "optional perf_scan scoreboard.json path to compare against the budget")
	flag.StringVar(&langsRaw, "langs", "", "optional comma-separated language filter")
	flag.BoolVar(&requireAllBudgetLangs, "require-all-budget-langs", false, "fail if the scoreboard omits a budgeted language")
	flag.BoolVar(&strictConfig, "strict-config", true, "require scoreboard measurement knobs to match structured budget metadata")
	flag.BoolVar(&hardGateOnly, "hard-gate-only", false, "check fail-closed fleet coverage and per-file hard limits without applying historical aggregate ratchets")
	flag.StringVar(&outMD, "out-md", "", "optional markdown summary output path")
	flag.Parse()

	budget, err := loadBudget(budgetPath)
	if err != nil {
		fatalf("load budget: %v", err)
	}
	if findings := validateBudget(budget); len(findings) > 0 {
		printFindings("budget validation failed", findings)
		os.Exit(1)
	}

	var findings []evalFinding
	if scoreboardPath != "" {
		board, err := loadScoreboard(scoreboardPath)
		if err != nil {
			fatalf("load scoreboard: %v", err)
		}
		langs := parseList(langsRaw)
		findings = compareScoreboard(budget, board, compareOptions{
			Languages:             langs,
			RequireAllBudgetLangs: requireAllBudgetLangs,
			StrictConfig:          strictConfig,
			HardGateOnly:          hardGateOnly,
		})
	}

	summary := renderSummary(budget, scoreboardPath, findings)
	fmt.Print(summary)
	if outMD != "" {
		if err := os.WriteFile(outMD, []byte(summary), 0o644); err != nil {
			fatalf("write markdown summary: %v", err)
		}
	}
	if len(findings) > 0 {
		os.Exit(1)
	}
}

func loadBudget(path string) (*budgetFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out budgetFile
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func loadScoreboard(path string) (*scoreboardFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out scoreboardFile
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func validateBudget(b *budgetFile) []evalFinding {
	var out []evalFinding
	if b.Schema != budgetSchema {
		out = append(out, evalFinding{Metric: "schema", Got: b.Schema, Want: budgetSchema})
	}
	if len(b.Languages) == 0 {
		out = append(out, evalFinding{Metric: "languages", Got: "0", Want: ">0"})
	}
	for _, axis := range []string{axisFull, axisNoEdit} {
		if !contains(b.MeasurementBasis.Axes, axis) {
			out = append(out, evalFinding{Axis: axis, Metric: "measurement_basis.axes", Got: strings.Join(b.MeasurementBasis.Axes, ","), Want: "include " + axis})
		}
	}
	if b.MeasurementBasis.HardMaxFullParseRatio != hardMaxFullParseRatio {
		out = append(out, evalFinding{Axis: axisFull, Metric: "measurement_basis.hard_max_full_parse_ratio", Got: fmt.Sprintf("%.6g", b.MeasurementBasis.HardMaxFullParseRatio), Want: fmt.Sprintf("%.1f", hardMaxFullParseRatio)})
	}
	if b.MeasurementBasis.FastFullParseRatio != fastFullParseRatio {
		out = append(out, evalFinding{Axis: axisFull, Metric: "measurement_basis.fast_full_parse_ratio", Got: fmt.Sprintf("%.6g", b.MeasurementBasis.FastFullParseRatio), Want: fmt.Sprintf("%.2f", fastFullParseRatio)})
	}
	if !validSHA256(b.MeasurementBasis.CorpusLockSHA256) {
		out = append(out, evalFinding{Metric: "measurement_basis.corpus_lock_sha256", Got: b.MeasurementBasis.CorpusLockSHA256, Want: "64 lowercase hex characters"})
	}
	for _, pattern := range normalizedPathList(b.MeasurementBasis.ExcludePaths) {
		if strings.ContainsAny(pattern, "*?[") {
			if _, err := path.Match(pattern, "x"); err != nil {
				out = append(out, evalFinding{Metric: "measurement_basis.exclude_paths", Got: pattern, Want: "valid path.Match pattern"})
			}
		}
	}
	for _, lang := range sortedBudgetLanguages(b) {
		entry := b.Languages[lang]
		if strings.TrimSpace(entry.Status) == "" {
			out = append(out, evalFinding{Language: lang, Metric: "status", Got: "", Want: "non-empty"})
		}
		out = append(out, validateBudgetAxis(lang, axisFull, entry.FullAxis)...)
		out = append(out, validateBudgetAxis(lang, axisNoEdit, entry.NoEditAxis)...)
	}
	return out
}

func validateBudgetAxis(lang, axis string, b budgetAxis) []evalFinding {
	var out []evalFinding
	if b.MaxTimeouts < 0 {
		out = append(out, evalFinding{Language: lang, Axis: axis, Metric: "max_timeouts", Got: fmt.Sprint(b.MaxTimeouts), Want: ">=0"})
	}
	if b.MaxErrors != nil && *b.MaxErrors < 0 {
		out = append(out, evalFinding{Language: lang, Axis: axis, Metric: "max_errors", Got: fmt.Sprint(*b.MaxErrors), Want: ">=0"})
	}
	if b.MaxCReferenceFailures != nil && *b.MaxCReferenceFailures < 0 {
		out = append(out, evalFinding{Language: lang, Axis: axis, Metric: "max_c_reference_failures", Got: fmt.Sprint(*b.MaxCReferenceFailures), Want: ">=0"})
	}
	if b.MaxRatioByTotal <= 0 {
		out = append(out, evalFinding{Language: lang, Axis: axis, Metric: "max_ratio_by_total", Got: fmt.Sprintf("%.6g", b.MaxRatioByTotal), Want: ">0"})
	}
	if b.MaxRatioMedianOfFiles < 0 {
		out = append(out, evalFinding{Language: lang, Axis: axis, Metric: "max_ratio_median_of_files", Got: fmt.Sprintf("%.6g", b.MaxRatioMedianOfFiles), Want: ">=0"})
	}
	return out
}

type compareOptions struct {
	Languages             []string
	RequireAllBudgetLangs bool
	StrictConfig          bool
	HardGateOnly          bool
}

func compareScoreboard(b *budgetFile, s *scoreboardFile, opts compareOptions) []evalFinding {
	var out []evalFinding
	if s.Config.RuntimeEvidence {
		out = append(out, evalFinding{
			Metric: "config.runtime_evidence",
			Got:    "true",
			Want:   "false (C6f ratchet)",
		})
	}
	switch s.Schema {
	case scoreboardSchemaV1:
		if opts.HardGateOnly {
			out = append(out, evalFinding{Metric: "scoreboard.schema_mode", Got: s.Schema, Want: scoreboardSchemaV2 + " for a current hard-gate verdict"})
		}
	case scoreboardSchemaV2:
		if !opts.HardGateOnly {
			out = append(out, evalFinding{Metric: "scoreboard.schema_mode", Got: s.Schema, Want: "--hard-gate-only (historical v1 aggregate ratchets are not comparable to the static full-only transport)"})
		}
	default:
		out = append(out, evalFinding{Metric: "scoreboard.schema", Got: s.Schema, Want: scoreboardSchemaV1 + " or " + scoreboardSchemaV2})
	}
	if opts.StrictConfig {
		out = append(out, compareConfig(b.MeasurementBasis, s.Config, opts.HardGateOnly)...)
	}
	if opts.HardGateOnly && !opts.StrictConfig && !s.Config.HardGate {
		out = append(out, evalFinding{Metric: "config.hard_gate", Got: "false", Want: "true"})
	}
	if s.Config.HardGate {
		out = append(out, compareHardCoverage(s)...)
	}

	filter := map[string]bool{}
	for _, lang := range opts.Languages {
		filter[lang] = true
	}
	scoreboardLangs := map[string]scoreboardLang{}
	for _, row := range s.Languages {
		if len(filter) > 0 && !filter[row.Language] {
			continue
		}
		scoreboardLangs[row.Language] = row
		if s.Config.HardGate || opts.HardGateOnly {
			out = append(out, compareHardGate(row)...)
		}
		if _, ok := b.Languages[row.Language]; !ok && !opts.HardGateOnly && !(s.Config.HardGate && s.Config.RequireFleet && s.Gate != nil) {
			out = append(out, evalFinding{Language: row.Language, Metric: "budget", Got: "missing", Want: "language budget"})
		}
	}
	if opts.HardGateOnly {
		return out
	}

	for _, lang := range sortedBudgetLanguages(b) {
		if len(filter) > 0 && !filter[lang] {
			continue
		}
		row, ok := scoreboardLangs[lang]
		if !ok {
			if opts.RequireAllBudgetLangs || len(filter) > 0 {
				out = append(out, evalFinding{Language: lang, Metric: "scoreboard", Got: "missing", Want: "measured language"})
			}
			continue
		}
		if row.Status != statusOK {
			out = append(out, evalFinding{Language: lang, Metric: "language_status", Got: row.Status, Want: statusOK})
		}
		entry := b.Languages[lang]
		out = append(out, compareAxis(lang, axisFull, entry.FullAxis, row)...)
		out = append(out, compareAxis(lang, axisNoEdit, entry.NoEditAxis, row)...)
	}
	return out
}

func compareHardCoverage(s *scoreboardFile) []evalFinding {
	var out []evalFinding
	if s.Gate == nil {
		return append(out, evalFinding{Metric: "hard_gate_report", Got: "missing", Want: "embedded fail-closed report"})
	}
	if s.Gate.Status != "pass" {
		out = append(out, evalFinding{Metric: "hard_gate_report", Got: s.Gate.Status, Want: "pass"})
	}
	if s.Gate.MaxFullParseRatio != hardMaxFullParseRatio {
		out = append(out, evalFinding{Axis: axisFull, Metric: "hard_gate.max_full_parse_ratio", Got: fmt.Sprintf("%.6g", s.Gate.MaxFullParseRatio), Want: fmt.Sprintf("%.1f", hardMaxFullParseRatio)})
	}
	if s.Gate.FastFullParseRatio != fastFullParseRatio {
		out = append(out, evalFinding{Axis: axisFull, Metric: "hard_gate.fast_full_parse_ratio", Got: fmt.Sprintf("%.6g", s.Gate.FastFullParseRatio), Want: fmt.Sprintf("%.2f", fastFullParseRatio)})
	}
	if s.Corpus.LockSHA256 != s.Config.CorpusLockSHA256 {
		out = append(out, evalFinding{Metric: "hard_fleet_coverage", Got: "coverage lock=" + s.Corpus.LockSHA256, Want: "config lock=" + s.Config.CorpusLockSHA256})
	}
	if s.Corpus.LockLanguages <= 0 {
		out = append(out, evalFinding{Metric: "hard_fleet_coverage", Got: fmt.Sprint(s.Corpus.LockLanguages), Want: "positive authenticated lock language count"})
	}
	if !s.Config.RequireFleet {
		return out
	}
	if s.Corpus.SelectedLanguages != s.Corpus.LockLanguages {
		out = append(out, evalFinding{Metric: "hard_fleet_coverage", Got: fmt.Sprintf("selected=%d lock=%d", s.Corpus.SelectedLanguages, s.Corpus.LockLanguages), Want: "selected=lock"})
	}
	if len(s.Languages) != s.Corpus.SelectedLanguages {
		out = append(out, evalFinding{Metric: "hard_fleet_coverage", Got: fmt.Sprintf("rows=%d selected=%d", len(s.Languages), s.Corpus.SelectedLanguages), Want: "one row per selected language"})
	}
	if len(s.Corpus.MissingFromLock) > 0 {
		out = append(out, evalFinding{Metric: "hard_fleet_coverage", Got: "missing_from_lock=" + strings.Join(s.Corpus.MissingFromLock, ","), Want: "none"})
	}
	if len(s.Corpus.MissingFromRegistry) > 0 {
		out = append(out, evalFinding{Metric: "hard_fleet_coverage", Got: "missing_from_registry=" + strings.Join(s.Corpus.MissingFromRegistry, ","), Want: "none"})
	}
	return out
}

func compareHardGate(row scoreboardLang) []evalFinding {
	var out []evalFinding
	if row.Status != statusOK {
		out = append(out, evalFinding{Language: row.Language, Path: row.ActiveFile, Axis: row.ActiveAxis, Metric: "hard_coverage", Got: row.Status, Want: statusOK})
	}
	if row.FilesMeasured != row.FilesSelected || len(row.Files) != row.FilesSelected {
		out = append(out, evalFinding{
			Language: row.Language,
			Metric:   "hard_coverage",
			Got:      fmt.Sprintf("measured=%d rows=%d selected=%d", row.FilesMeasured, len(row.Files), row.FilesSelected),
			Want:     "complete selected-file coverage",
		})
	}
	if scoreboardStopIsGo(row.Stop) {
		out = append(out, evalFinding{
			Language: row.Language,
			Path:     row.ActiveFile,
			Axis:     row.ActiveAxis,
			Metric:   "hard_go_stop",
			Got:      hardStopDescription(row.Stop, row.Status),
			Want:     "no Go timeout, parser-budget, wall, RSS, or OOM stop",
		})
	}
	for _, file := range row.Files {
		for axis, result := range file.Axes {
			if scoreboardStopIsGo(result.Stop) || legacyGoStopStatus(result.Status) {
				out = append(out, evalFinding{
					Language: row.Language,
					Path:     file.Path,
					Axis:     axis,
					Metric:   "hard_go_stop",
					Got:      hardStopDescription(result.Stop, result.Status),
					Want:     "no Go timeout or parser-budget stop",
				})
			}
		}
		full, ok := file.Axes[axisFull]
		if !ok {
			out = append(out, evalFinding{Language: row.Language, Path: file.Path, Axis: axisFull, Metric: "hard_full_measurement", Got: "missing", Want: "exact per-file Go/C ratio"})
			continue
		}
		if full.Status != statusOK || full.GoMedianNs <= 0 || full.CMedianNs <= 0 {
			if !scoreboardStopIsGo(full.Stop) && !legacyGoStopStatus(full.Status) {
				got := full.Status
				if got == "" {
					got = "missing timing"
				}
				out = append(out, evalFinding{Language: row.Language, Path: file.Path, Axis: axisFull, Metric: "hard_full_measurement", Got: got, Want: "status=ok with exact Go/C ratio"})
			}
			continue
		}
		ratio := float64(full.GoMedianNs) / float64(full.CMedianNs)
		if ratio > hardMaxFullParseRatio {
			out = append(out, evalFinding{
				Language: row.Language,
				Path:     file.Path,
				Axis:     axisFull,
				Metric:   "hard_full_ratio",
				Got:      fmt.Sprintf("%.4fx", ratio),
				Want:     fmt.Sprintf("<=%.1fx", hardMaxFullParseRatio),
			})
		}
	}
	return out
}

func scoreboardStopIsGo(stop *scoreboardStop) bool {
	return stop != nil && stop.Implementation == "go"
}

func legacyGoStopStatus(status string) bool {
	switch status {
	case "go_timeout", "go_budget_stop", "go_stopped":
		return true
	default:
		return false
	}
}

func hardStopDescription(stop *scoreboardStop, status string) string {
	if stop == nil {
		return status
	}
	description := stop.Class
	if stop.Reason != "" {
		description += ":" + stop.Reason
	}
	return description
}

func compareConfig(b budgetMeasurementBasis, s scoreboardConfig, hardGateOnly bool) []evalFinding {
	var out []evalFinding
	if b.Reps > 0 && s.Reps != b.Reps {
		out = append(out, evalFinding{Metric: "config.reps", Got: fmt.Sprint(s.Reps), Want: fmt.Sprint(b.Reps)})
	}
	if b.Warmup > 0 && s.Warmup != b.Warmup {
		out = append(out, evalFinding{Metric: "config.warmup", Got: fmt.Sprint(s.Warmup), Want: fmt.Sprint(b.Warmup)})
	}
	if b.FileBudgetMS > 0 && s.FileBudgetMS != b.FileBudgetMS {
		out = append(out, evalFinding{Metric: "config.file_budget_ms", Got: fmt.Sprint(s.FileBudgetMS), Want: fmt.Sprint(b.FileBudgetMS)})
	}
	if b.MaxFiles > 0 && s.MaxFiles != b.MaxFiles {
		out = append(out, evalFinding{Metric: "config.max_files", Got: fmt.Sprint(s.MaxFiles), Want: fmt.Sprint(b.MaxFiles)})
	}
	if b.Order != "" && s.Order != b.Order {
		out = append(out, evalFinding{Metric: "config.order", Got: s.Order, Want: b.Order})
	}
	if hardGateOnly {
		if len(s.Axes) != 1 || s.Axes[0] != axisFull {
			out = append(out, evalFinding{Axis: axisFull, Metric: "config.axes", Got: strings.Join(s.Axes, ","), Want: axisFull + " only"})
		}
	} else {
		for _, axis := range b.Axes {
			if !contains(s.Axes, axis) {
				out = append(out, evalFinding{Axis: axis, Metric: "config.axes", Got: strings.Join(s.Axes, ","), Want: "include " + axis})
			}
		}
	}
	if hardGateOnly {
		if got := normalizedPathList(s.ExcludePaths); len(got) > 0 {
			out = append(out, evalFinding{Metric: "config.exclude_paths", Got: strings.Join(got, ","), Want: "none for the universal hard gate"})
		}
	} else if len(b.ExcludePaths) > 0 || len(s.ExcludePaths) > 0 {
		got := normalizedPathList(s.ExcludePaths)
		want := normalizedPathList(b.ExcludePaths)
		if !stringSlicesEqual(got, want) {
			out = append(out, evalFinding{Metric: "config.exclude_paths", Got: strings.Join(got, ","), Want: strings.Join(want, ",")})
		}
	}
	if !s.HardGate {
		out = append(out, evalFinding{Metric: "config.hard_gate", Got: "false", Want: "true"})
	}
	if s.CorpusLockSHA256 != b.CorpusLockSHA256 {
		out = append(out, evalFinding{Metric: "config.corpus_lock_sha256", Got: s.CorpusLockSHA256, Want: b.CorpusLockSHA256})
	}
	return out
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

func compareAxis(lang, axis string, budget budgetAxis, row scoreboardLang) []evalFinding {
	var out []evalFinding
	actual, ok := row.Axes[axis]
	if !ok {
		return append(out, evalFinding{Language: lang, Axis: axis, Metric: "axis", Got: "missing", Want: "measured"})
	}
	if len(row.Files) == 0 {
		out = append(out, evalFinding{Language: lang, Axis: axis, Metric: "files", Got: "0", Want: ">0"})
	}
	if actual.GoTimeouts > budget.MaxTimeouts {
		out = append(out, evalFinding{Language: lang, Axis: axis, Metric: "go_timeouts", Got: fmt.Sprint(actual.GoTimeouts), Want: fmt.Sprintf("<=%d", budget.MaxTimeouts)})
	}
	errors := countGoErrors(row, axis)
	if budget.MaxErrors != nil && errors > *budget.MaxErrors {
		out = append(out, evalFinding{Language: lang, Axis: axis, Metric: "go_errors", Got: fmt.Sprint(errors), Want: fmt.Sprintf("<=%d", *budget.MaxErrors)})
	}
	cProblems := countCProblems(row, axis)
	maxCProblems := 0
	if budget.MaxCReferenceFailures != nil {
		maxCProblems = *budget.MaxCReferenceFailures
	}
	if cProblems > maxCProblems {
		out = append(out, evalFinding{Language: lang, Axis: axis, Metric: "c_reference_failures", Got: fmt.Sprint(cProblems), Want: fmt.Sprintf("<=%d", maxCProblems)})
	}
	if budget.MaxRatioByTotal > 0 && actual.RatioByTotal > budget.MaxRatioByTotal {
		out = append(out, evalFinding{Language: lang, Axis: axis, Metric: "ratio_by_total", Got: fmt.Sprintf("%.4fx", actual.RatioByTotal), Want: fmt.Sprintf("<=%.4fx", budget.MaxRatioByTotal)})
	}
	if budget.MaxRatioMedianOfFiles > 0 && actual.RatioMedianOfFiles > budget.MaxRatioMedianOfFiles {
		out = append(out, evalFinding{Language: lang, Axis: axis, Metric: "ratio_median_of_files", Got: fmt.Sprintf("%.4fx", actual.RatioMedianOfFiles), Want: fmt.Sprintf("<=%.4fx", budget.MaxRatioMedianOfFiles)})
	}
	return out
}

func countGoErrors(row scoreboardLang, axis string) int {
	n := 0
	for _, file := range row.Files {
		fa, ok := file.Axes[axis]
		if !ok {
			continue
		}
		if isGoErrorStatus(fa.Status) {
			n++
		}
	}
	return n
}

func isGoErrorStatus(status string) bool {
	switch status {
	case "go_error", "go_panic", "go_truncated", "go_partial":
		return true
	case "go_timeout", statusOK, "":
		return false
	default:
		return strings.HasPrefix(status, "go_") && strings.Contains(status, "trunc")
	}
}

func countCProblems(row scoreboardLang, axis string) int {
	n := 0
	for _, file := range row.Files {
		fa, ok := file.Axes[axis]
		if !ok {
			continue
		}
		if strings.HasPrefix(fa.Status, "c_") {
			n++
		}
	}
	return n
}

func renderSummary(b *budgetFile, scoreboardPath string, findings []evalFinding) string {
	var sb strings.Builder
	mode := "validate"
	if scoreboardPath != "" {
		mode = "compare"
	}
	fmt.Fprintf(&sb, "### Perf Scan Budget %s\n\n", titleWord(mode))
	fmt.Fprintf(&sb, "- budget schema: `%s`\n", b.Schema)
	fmt.Fprintf(&sb, "- budget languages: `%d`\n", len(b.Languages))
	if scoreboardPath != "" {
		fmt.Fprintf(&sb, "- scoreboard: `%s`\n", scoreboardPath)
	}
	if len(findings) == 0 {
		fmt.Fprintf(&sb, "- outcome: `PASS`\n")
		return sb.String()
	}
	fmt.Fprintf(&sb, "- outcome: `FAIL`\n\n")
	fmt.Fprintf(&sb, "| language | file | axis | metric | got | want |\n")
	fmt.Fprintf(&sb, "|---|---|---|---|---|---|\n")
	for _, f := range findings {
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s | %s |\n",
			mdCell(f.Language), mdCell(f.Path), mdCell(f.Axis), mdCell(f.Metric), mdCell(f.Got), mdCell(f.Want))
	}
	return sb.String()
}

func printFindings(prefix string, findings []evalFinding) {
	fmt.Fprintln(os.Stderr, prefix)
	for _, f := range findings {
		fmt.Fprintf(os.Stderr, "%s\t%s\t%s\t%s\tgot=%s\twant=%s\n", f.Language, f.Path, f.Axis, f.Metric, f.Got, f.Want)
	}
}

func sortedBudgetLanguages(b *budgetFile) []string {
	out := make([]string, 0, len(b.Languages))
	for lang := range b.Languages {
		out = append(out, lang)
	}
	sort.Strings(out)
	return out
}

func parseList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func normalizedPathList(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range items {
		item = strings.ReplaceAll(strings.TrimSpace(item), "\\", "/")
		item = strings.TrimPrefix(item, "./")
		item = strings.TrimPrefix(item, "/")
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

func mdCell(s string) string {
	if s == "" {
		return "-"
	}
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.ReplaceAll(s, "|", "\\|")
}

func titleWord(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
