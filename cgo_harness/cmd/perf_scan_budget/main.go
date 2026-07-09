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
	budgetSchema     = "gts-perf-ratio-budgets/v1"
	scoreboardSchema = "gts-perf-scan/v1"

	axisFull   = "full"
	axisNoEdit = "noedit"
	statusOK   = "ok"
)

type budgetFile struct {
	Schema           string                 `json:"schema"`
	GeneratedAt      string                 `json:"generated_at"`
	MeasurementBasis budgetMeasurementBasis `json:"measurement_basis"`
	Languages        map[string]budgetLang  `json:"languages"`
}

type budgetMeasurementBasis struct {
	Reps         int      `json:"reps"`
	Warmup       int      `json:"warmup"`
	FileBudgetMS int      `json:"file_budget_ms"`
	MaxFiles     int      `json:"max_files,omitempty"`
	Order        string   `json:"order,omitempty"`
	Axes         []string `json:"axes"`
	ExcludePaths []string `json:"exclude_paths,omitempty"`
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
}

type scoreboardConfig struct {
	Reps         int      `json:"reps"`
	Warmup       int      `json:"warmup"`
	FileBudgetMS int      `json:"file_budget_ms"`
	MaxFiles     int      `json:"max_files"`
	Order        string   `json:"order"`
	Axes         []string `json:"axes"`
	ExcludePaths []string `json:"exclude_paths,omitempty"`
}

type scoreboardLang struct {
	Language string                    `json:"language"`
	Status   string                    `json:"status"`
	Axes     map[string]scoreboardAxis `json:"axes"`
	Files    []scoreboardFileRow       `json:"files"`
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
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type evalFinding struct {
	Language string
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
		outMD                 string
	)

	flag.StringVar(&budgetPath, "budget", "perf_scan/perf_ratio_budgets.json", "perf ratio budget JSON path")
	flag.StringVar(&scoreboardPath, "scoreboard", "", "optional perf_scan scoreboard.json path to compare against the budget")
	flag.StringVar(&langsRaw, "langs", "", "optional comma-separated language filter")
	flag.BoolVar(&requireAllBudgetLangs, "require-all-budget-langs", false, "fail if the scoreboard omits a budgeted language")
	flag.BoolVar(&strictConfig, "strict-config", true, "require scoreboard measurement knobs to match structured budget metadata")
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
}

func compareScoreboard(b *budgetFile, s *scoreboardFile, opts compareOptions) []evalFinding {
	var out []evalFinding
	if s.Schema != scoreboardSchema {
		out = append(out, evalFinding{Metric: "scoreboard.schema", Got: s.Schema, Want: scoreboardSchema})
	}
	if opts.StrictConfig {
		out = append(out, compareConfig(b.MeasurementBasis, s.Config)...)
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
		if _, ok := b.Languages[row.Language]; !ok {
			out = append(out, evalFinding{Language: row.Language, Metric: "budget", Got: "missing", Want: "language budget"})
		}
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

func compareConfig(b budgetMeasurementBasis, s scoreboardConfig) []evalFinding {
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
	for _, axis := range b.Axes {
		if !contains(s.Axes, axis) {
			out = append(out, evalFinding{Axis: axis, Metric: "config.axes", Got: strings.Join(s.Axes, ","), Want: "include " + axis})
		}
	}
	if len(b.ExcludePaths) > 0 || len(s.ExcludePaths) > 0 {
		got := normalizedPathList(s.ExcludePaths)
		want := normalizedPathList(b.ExcludePaths)
		if !stringSlicesEqual(got, want) {
			out = append(out, evalFinding{Metric: "config.exclude_paths", Got: strings.Join(got, ","), Want: strings.Join(want, ",")})
		}
	}
	return out
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
	fmt.Fprintf(&sb, "| language | axis | metric | got | want |\n")
	fmt.Fprintf(&sb, "|---|---|---|---|---|\n")
	for _, f := range findings {
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s |\n",
			mdCell(f.Language), mdCell(f.Axis), mdCell(f.Metric), mdCell(f.Got), mdCell(f.Want))
	}
	return sb.String()
}

func printFindings(prefix string, findings []evalFinding) {
	fmt.Fprintln(os.Stderr, prefix)
	for _, f := range findings {
		fmt.Fprintf(os.Stderr, "%s\t%s\t%s\tgot=%s\twant=%s\n", f.Language, f.Axis, f.Metric, f.Got, f.Want)
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
