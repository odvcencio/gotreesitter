package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	statusSchema     = "gts-wave3-perf-sweep-status/v1"
	budgetSchema     = "gts-perf-ratio-budgets/v1"
	scoreboardSchema = "gts-perf-scan/v1"
)

type options struct {
	BudgetPath         string
	FleetPath          string
	ScoreboardPatterns string
	GeneratedAt        string
	OutJSON            string
	OutMD              string
}

type budgetFile struct {
	Schema      string                    `json:"schema"`
	GeneratedAt string                    `json:"generated_at"`
	GeneratedBy string                    `json:"generated_by"`
	Basis       measurementBasis          `json:"measurement_basis"`
	SeedSources map[string]string         `json:"_seed_sources"`
	KnownGaps   map[string]knownBudgetGap `json:"known_budget_class_gaps"`
	Languages   map[string]budgetLanguage `json:"languages"`
}

type measurementBasis struct {
	Reps         int      `json:"reps"`
	Warmup       int      `json:"warmup"`
	FileBudgetMS int      `json:"file_budget_ms"`
	MaxFiles     int      `json:"max_files,omitempty"`
	Order        string   `json:"order,omitempty"`
	Axes         []string `json:"axes"`
}

type knownBudgetGap struct {
	File       string `json:"file,omitempty"`
	BacklogRef string `json:"backlog_ref,omitempty"`
	Action     string `json:"action,omitempty"`
}

type budgetLanguage struct {
	Status        string     `json:"status"`
	Wave2BPending ledgerFlag `json:"wave2b_pending,omitempty"`
	MeasuredToday ledgerFlag `json:"measured_today,omitempty"`
	FullAxis      budgetAxis `json:"full_axis"`
	NoEditAxis    budgetAxis `json:"noedit_axis"`
}

type ledgerFlag struct {
	Bool bool
	Text string
}

func (f *ledgerFlag) UnmarshalJSON(data []byte) error {
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		f.Bool = b
		f.Text = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		f.Bool = false
		f.Text = s
		return nil
	}
	return fmt.Errorf("ledger flag must be bool or string")
}

func (f ledgerFlag) IsTrue() bool {
	return f.Bool
}

func (f ledgerFlag) IsNote() bool {
	return f.Text != ""
}

type budgetAxis struct {
	MaxTimeouts                int      `json:"max_timeouts"`
	MaxErrors                  *int     `json:"max_errors,omitempty"`
	MaxCReferenceFailures      *int     `json:"max_c_reference_failures,omitempty"`
	MaxRatioByTotal            float64  `json:"max_ratio_by_total"`
	MaxRatioMedianOfFiles      float64  `json:"max_ratio_median_of_files,omitempty"`
	ObservedRatioByTotal       *float64 `json:"observed_ratio_by_total,omitempty"`
	ObservedRatioMedianOfFiles *float64 `json:"observed_ratio_median_of_files,omitempty"`
}

type scoreboardFile struct {
	Schema      string               `json:"schema"`
	GeneratedAt string               `json:"generated_at"`
	Config      scoreboardConfig     `json:"config"`
	Host        scoreboardHost       `json:"host"`
	Languages   []scoreboardLanguage `json:"languages"`
	Summary     map[string]int       `json:"summary_verdicts"`
	SourcePath  string               `json:"-"`
}

type scoreboardConfig struct {
	Reps          int      `json:"reps"`
	Warmup        int      `json:"warmup"`
	FileBudgetMS  int      `json:"file_budget_ms"`
	LangTimeoutMS int      `json:"lang_timeout_ms"`
	MaxFiles      int      `json:"max_files"`
	Order         string   `json:"order"`
	Axes          []string `json:"axes"`
	Contended     bool     `json:"contended"`
	ContendedNote string   `json:"contended_note,omitempty"`
}

type scoreboardHost struct {
	LoadAvgStart string `json:"loadavg_start"`
	LoadAvgEnd   string `json:"loadavg_end"`
}

type scoreboardLanguage struct {
	Language      string                    `json:"language"`
	Status        string                    `json:"status"`
	FilesSelected int                       `json:"files_selected"`
	FilesMeasured int                       `json:"files_measured"`
	BytesMeasured int64                     `json:"bytes_measured"`
	Verdict       string                    `json:"verdict"`
	Axes          map[string]scoreboardAxis `json:"axes"`
}

type scoreboardAxis struct {
	FilesOK            int     `json:"files_ok"`
	GoTimeouts         int     `json:"go_timeouts"`
	Cliffs             int     `json:"cliffs"`
	RatioByTotal       float64 `json:"ratio_by_total"`
	RatioMedianOfFiles float64 `json:"ratio_median_of_files"`
	Verdict            string  `json:"verdict"`
}

type statusDocument struct {
	Schema             string              `json:"schema"`
	GeneratedAt        string              `json:"generated_at"`
	BudgetPath         string              `json:"budget_path"`
	FleetPath          string              `json:"fleet_path,omitempty"`
	BudgetGeneratedAt  string              `json:"budget_generated_at"`
	BudgetGeneratedBy  string              `json:"budget_generated_by"`
	MeasurementBasis   measurementBasis    `json:"measurement_basis"`
	Coverage           coverageSummary     `json:"coverage"`
	HeldOutLanguages   []string            `json:"held_out_languages"`
	BudgetStatusCounts map[string]int      `json:"budget_status_counts"`
	SeedSources        []string            `json:"seed_sources"`
	KnownGaps          []knownGapSummary   `json:"known_budget_class_gaps"`
	ScoreboardPatterns []string            `json:"scoreboard_patterns,omitempty"`
	ScoreboardCoverage *scoreboardCoverage `json:"scoreboard_coverage,omitempty"`
	Scoreboards        []scoreboardSummary `json:"scoreboards,omitempty"`
	MeasuredLanguages  []measuredLanguage  `json:"measured_languages,omitempty"`
	Caveats            []string            `json:"caveats"`
}

type coverageSummary struct {
	FleetLanguages              int `json:"fleet_languages"`
	BudgetedLanguages           int `json:"budgeted_languages"`
	HeldOutLanguages            int `json:"held_out_languages"`
	KnownBudgetClassGaps        int `json:"known_budget_class_gaps"`
	Wave2BPendingBudgets        int `json:"wave2b_pending_budgets"`
	MeasuredTodayBudgets        int `json:"measured_today_budgets"`
	PartialMeasuredTodayBudgets int `json:"partial_measured_today_budgets"`
}

type knownGapSummary struct {
	Key        string `json:"key"`
	File       string `json:"file,omitempty"`
	BacklogRef string `json:"backlog_ref,omitempty"`
	Action     string `json:"action,omitempty"`
}

type scoreboardCoverage struct {
	ScoreboardFiles          int      `json:"scoreboard_files"`
	ContendedScoreboards     int      `json:"contended_scoreboards"`
	MeasuredLanguages        int      `json:"measured_languages"`
	MeasuredBudgetLanguages  int      `json:"measured_budget_languages"`
	MeasuredHeldOutLanguages []string `json:"measured_held_out_languages,omitempty"`
}

type scoreboardSummary struct {
	Path             string         `json:"path"`
	GeneratedAt      string         `json:"generated_at,omitempty"`
	Languages        int            `json:"languages"`
	BudgetLanguages  int            `json:"budget_languages"`
	HeldOutLanguages []string       `json:"held_out_languages,omitempty"`
	Contended        bool           `json:"contended"`
	ContendedNote    string         `json:"contended_note,omitempty"`
	LoadAvgStart     string         `json:"loadavg_start,omitempty"`
	LoadAvgEnd       string         `json:"loadavg_end,omitempty"`
	SummaryVerdicts  map[string]int `json:"summary_verdicts,omitempty"`
}

type measuredLanguage struct {
	Language        string                 `json:"language"`
	Budgeted        bool                   `json:"budgeted"`
	HeldOut         bool                   `json:"held_out"`
	SourcePath      string                 `json:"source_path"`
	SourceContended bool                   `json:"source_contended"`
	Status          string                 `json:"status"`
	Verdict         string                 `json:"verdict"`
	FilesSelected   int                    `json:"files_selected"`
	FilesMeasured   int                    `json:"files_measured"`
	BytesMeasured   int64                  `json:"bytes_measured"`
	Axes            map[string]axisSummary `json:"axes"`
}

type axisSummary struct {
	FilesOK            int     `json:"files_ok"`
	GoTimeouts         int     `json:"go_timeouts"`
	Cliffs             int     `json:"cliffs"`
	RatioByTotal       float64 `json:"ratio_by_total"`
	RatioMedianOfFiles float64 `json:"ratio_median_of_files"`
	Verdict            string  `json:"verdict"`
}

func main() {
	opts := options{}
	flag.StringVar(&opts.BudgetPath, "budget", "perf_scan/perf_ratio_budgets.json", "perf ratio budget JSON path")
	flag.StringVar(&opts.FleetPath, "fleet", "tier_scan/exts.tsv", "fleet language catalog TSV path; pass empty to disable held-out accounting")
	flag.StringVar(&opts.ScoreboardPatterns, "scoreboards", "", "optional comma-separated scoreboard globs to summarize")
	flag.StringVar(&opts.GeneratedAt, "generated-at", "", "override generated_at timestamp, mainly for tests")
	flag.StringVar(&opts.OutJSON, "out-json", "", "optional status JSON output path")
	flag.StringVar(&opts.OutMD, "out-md", "", "optional status markdown output path")
	flag.Parse()

	doc, err := buildStatus(opts)
	if err != nil {
		fatalf("%v", err)
	}
	if opts.OutJSON == "" && opts.OutMD == "" {
		fmt.Print(renderMarkdown(doc))
		return
	}
	if opts.OutJSON != "" {
		data, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			fatalf("marshal status json: %v", err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(opts.OutJSON, data, 0o644); err != nil {
			fatalf("write status json: %v", err)
		}
	}
	if opts.OutMD != "" {
		if err := os.WriteFile(opts.OutMD, []byte(renderMarkdown(doc)), 0o644); err != nil {
			fatalf("write status markdown: %v", err)
		}
	}
}

func buildStatus(opts options) (*statusDocument, error) {
	budget, err := loadBudget(opts.BudgetPath)
	if err != nil {
		return nil, fmt.Errorf("load budget: %w", err)
	}
	if budget.Schema != budgetSchema {
		return nil, fmt.Errorf("budget schema %q, want %q", budget.Schema, budgetSchema)
	}
	fleet, err := loadFleetLanguages(opts.FleetPath)
	if err != nil {
		return nil, fmt.Errorf("load fleet languages: %w", err)
	}
	generatedAt := opts.GeneratedAt
	if generatedAt == "" {
		generatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	budgetSet := map[string]bool{}
	statusCounts := map[string]int{}
	var pending, measuredToday, partialMeasuredToday int
	for lang, row := range budget.Languages {
		budgetSet[lang] = true
		statusCounts[row.Status]++
		if row.Wave2BPending.IsTrue() {
			pending++
		}
		if row.MeasuredToday.IsTrue() {
			measuredToday++
		}
		if row.MeasuredToday.IsNote() {
			partialMeasuredToday++
		}
	}
	heldOut := diffSorted(fleet, budgetSet)
	fleetCount := len(fleet)
	if fleetCount == 0 {
		fleetCount = len(budget.Languages) + len(heldOut)
	}

	doc := &statusDocument{
		Schema:            statusSchema,
		GeneratedAt:       generatedAt,
		BudgetPath:        opts.BudgetPath,
		FleetPath:         opts.FleetPath,
		BudgetGeneratedAt: budget.GeneratedAt,
		BudgetGeneratedBy: budget.GeneratedBy,
		MeasurementBasis:  budget.Basis,
		Coverage: coverageSummary{
			FleetLanguages:              fleetCount,
			BudgetedLanguages:           len(budget.Languages),
			HeldOutLanguages:            len(heldOut),
			KnownBudgetClassGaps:        len(budget.KnownGaps),
			Wave2BPendingBudgets:        pending,
			MeasuredTodayBudgets:        measuredToday,
			PartialMeasuredTodayBudgets: partialMeasuredToday,
		},
		HeldOutLanguages:   heldOut,
		BudgetStatusCounts: statusCounts,
		SeedSources:        sortedKeys(budget.SeedSources),
		KnownGaps:          summarizeKnownGaps(budget.KnownGaps),
		Caveats:            baseCaveats(heldOut, budget.KnownGaps),
	}

	if opts.ScoreboardPatterns != "" {
		patterns := splitCSV(opts.ScoreboardPatterns)
		paths, err := expandGlobs(patterns)
		if err != nil {
			return nil, err
		}
		doc.ScoreboardPatterns = patterns
		doc.Scoreboards, doc.MeasuredLanguages, doc.ScoreboardCoverage, err = summarizeScoreboards(paths, budgetSet, setFromList(heldOut))
		if err != nil {
			return nil, err
		}
		if doc.ScoreboardCoverage.ContendedScoreboards > 0 {
			doc.Caveats = append(doc.Caveats, "At least one local scoreboard was harness-flagged as contended; treat those ratios as smoke or visibility evidence rather than quiet-box ratchets.")
		}
	}

	return doc, nil
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

func loadFleetLanguages(path string) ([]string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	seen := map[string]bool{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		seen[fields[0]] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return sortedKeysBool(seen), nil
}

func expandGlobs(patterns []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("expand scoreboard glob %q: %w", pattern, err)
		}
		for _, match := range matches {
			if !seen[match] {
				seen[match] = true
				out = append(out, match)
			}
		}
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil, fmt.Errorf("no scoreboard files matched %q", strings.Join(patterns, ","))
	}
	return out, nil
}

func summarizeScoreboards(paths []string, budgetSet map[string]bool, heldOutSet map[string]bool) ([]scoreboardSummary, []measuredLanguage, *scoreboardCoverage, error) {
	var summaries []scoreboardSummary
	measured := map[string]measuredLanguage{}
	var contended int
	for _, path := range paths {
		board, err := loadScoreboard(path)
		if err != nil {
			return nil, nil, nil, err
		}
		budgetLangs := 0
		var heldOut []string
		for _, lang := range board.Languages {
			if budgetSet[lang.Language] {
				budgetLangs++
			}
			if heldOutSet[lang.Language] {
				heldOut = append(heldOut, lang.Language)
			}
			measured[lang.Language] = measuredLanguage{
				Language:        lang.Language,
				Budgeted:        budgetSet[lang.Language],
				HeldOut:         heldOutSet[lang.Language],
				SourcePath:      path,
				SourceContended: board.Config.Contended,
				Status:          lang.Status,
				Verdict:         lang.Verdict,
				FilesSelected:   lang.FilesSelected,
				FilesMeasured:   lang.FilesMeasured,
				BytesMeasured:   lang.BytesMeasured,
				Axes:            summarizeAxes(lang.Axes),
			}
		}
		if board.Config.Contended {
			contended++
		}
		summaries = append(summaries, scoreboardSummary{
			Path:             path,
			GeneratedAt:      board.GeneratedAt,
			Languages:        len(board.Languages),
			BudgetLanguages:  budgetLangs,
			HeldOutLanguages: heldOut,
			Contended:        board.Config.Contended,
			ContendedNote:    board.Config.ContendedNote,
			LoadAvgStart:     board.Host.LoadAvgStart,
			LoadAvgEnd:       board.Host.LoadAvgEnd,
			SummaryVerdicts:  board.Summary,
		})
	}
	measuredList := make([]measuredLanguage, 0, len(measured))
	var measuredBudget int
	measuredHeldOut := map[string]bool{}
	for _, row := range measured {
		measuredList = append(measuredList, row)
		if row.Budgeted {
			measuredBudget++
		}
		if row.HeldOut {
			measuredHeldOut[row.Language] = true
		}
	}
	sort.Slice(measuredList, func(i, j int) bool {
		return measuredList[i].Language < measuredList[j].Language
	})
	coverage := &scoreboardCoverage{
		ScoreboardFiles:          len(paths),
		ContendedScoreboards:     contended,
		MeasuredLanguages:        len(measuredList),
		MeasuredBudgetLanguages:  measuredBudget,
		MeasuredHeldOutLanguages: sortedKeysBool(measuredHeldOut),
	}
	return summaries, measuredList, coverage, nil
}

func loadScoreboard(path string) (*scoreboardFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load scoreboard %s: %w", path, err)
	}
	var out scoreboardFile
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decode scoreboard %s: %w", path, err)
	}
	if out.Schema != scoreboardSchema {
		return nil, fmt.Errorf("scoreboard %s schema %q, want %q", path, out.Schema, scoreboardSchema)
	}
	out.SourcePath = path
	return &out, nil
}

func summarizeAxes(in map[string]scoreboardAxis) map[string]axisSummary {
	out := make(map[string]axisSummary, len(in))
	for axis, row := range in {
		out[axis] = axisSummary{
			FilesOK:            row.FilesOK,
			GoTimeouts:         row.GoTimeouts,
			Cliffs:             row.Cliffs,
			RatioByTotal:       row.RatioByTotal,
			RatioMedianOfFiles: row.RatioMedianOfFiles,
			Verdict:            row.Verdict,
		}
	}
	return out
}

func renderMarkdown(doc *statusDocument) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Wave 3 Perf Sweep Status\n\n")
	fmt.Fprintf(&sb, "- generated_at: `%s`\n", doc.GeneratedAt)
	fmt.Fprintf(&sb, "- budget: `%s`\n", doc.BudgetPath)
	if doc.FleetPath != "" {
		fmt.Fprintf(&sb, "- fleet catalog: `%s`\n", doc.FleetPath)
	}
	fmt.Fprintf(&sb, "- budget_generated_at: `%s`\n", doc.BudgetGeneratedAt)
	fmt.Fprintf(&sb, "- budget_generated_by: `%s`\n\n", mdInline(doc.BudgetGeneratedBy))

	fmt.Fprintf(&sb, "## Coverage\n\n")
	fmt.Fprintf(&sb, "| metric | value |\n")
	fmt.Fprintf(&sb, "|---|---:|\n")
	fmt.Fprintf(&sb, "| fleet languages | %d |\n", doc.Coverage.FleetLanguages)
	fmt.Fprintf(&sb, "| budgeted languages | %d |\n", doc.Coverage.BudgetedLanguages)
	fmt.Fprintf(&sb, "| held out languages | %d |\n", doc.Coverage.HeldOutLanguages)
	fmt.Fprintf(&sb, "| known budget class gaps | %d |\n", doc.Coverage.KnownBudgetClassGaps)
	fmt.Fprintf(&sb, "| wave2b pending budget rows | %d |\n", doc.Coverage.Wave2BPendingBudgets)
	fmt.Fprintf(&sb, "| measured-today budget rows | %d |\n", doc.Coverage.MeasuredTodayBudgets)
	if doc.Coverage.PartialMeasuredTodayBudgets > 0 {
		fmt.Fprintf(&sb, "| partial measured-today notes | %d |\n", doc.Coverage.PartialMeasuredTodayBudgets)
	}
	fmt.Fprintf(&sb, "\n")

	fmt.Fprintf(&sb, "Measurement basis: `reps=%d`, `warmup=%d`, `file_budget_ms=%d`, `max_files=%d`, `order=%s`, `axes=%s`.\n\n",
		doc.MeasurementBasis.Reps,
		doc.MeasurementBasis.Warmup,
		doc.MeasurementBasis.FileBudgetMS,
		doc.MeasurementBasis.MaxFiles,
		mdInline(doc.MeasurementBasis.Order),
		mdInline(strings.Join(doc.MeasurementBasis.Axes, ",")))

	if len(doc.HeldOutLanguages) > 0 {
		fmt.Fprintf(&sb, "Held out of the ratchet: `%s`.\n\n", strings.Join(doc.HeldOutLanguages, "`, `"))
	}

	fmt.Fprintf(&sb, "## Budget Status Counts\n\n")
	fmt.Fprintf(&sb, "| status | languages |\n")
	fmt.Fprintf(&sb, "|---|---:|\n")
	for _, key := range sortedKeysInt(doc.BudgetStatusCounts) {
		fmt.Fprintf(&sb, "| `%s` | %d |\n", mdCell(key), doc.BudgetStatusCounts[key])
	}
	fmt.Fprintf(&sb, "\n")

	if len(doc.KnownGaps) > 0 {
		fmt.Fprintf(&sb, "## Known Gap Ledger\n\n")
		fmt.Fprintf(&sb, "| key | file | action |\n")
		fmt.Fprintf(&sb, "|---|---|---|\n")
		for _, gap := range doc.KnownGaps {
			fmt.Fprintf(&sb, "| `%s` | %s | %s |\n",
				mdCell(gap.Key), mdCell(gap.File), mdCell(gap.Action))
		}
		fmt.Fprintf(&sb, "\n")
	}

	if len(doc.SeedSources) > 0 {
		fmt.Fprintf(&sb, "## Seed Sources\n\n")
		for _, source := range doc.SeedSources {
			fmt.Fprintf(&sb, "- `%s`\n", mdInline(source))
		}
		fmt.Fprintf(&sb, "\n")
	}

	if doc.ScoreboardCoverage != nil {
		renderScoreboardMarkdown(&sb, doc)
	}

	if len(doc.Caveats) > 0 {
		fmt.Fprintf(&sb, "## Caveats\n\n")
		for _, caveat := range doc.Caveats {
			fmt.Fprintf(&sb, "- %s\n", caveat)
		}
		fmt.Fprintf(&sb, "\n")
	}
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

func renderScoreboardMarkdown(sb *strings.Builder, doc *statusDocument) {
	fmt.Fprintf(sb, "## Local Scoreboards\n\n")
	fmt.Fprintf(sb, "| metric | value |\n")
	fmt.Fprintf(sb, "|---|---:|\n")
	fmt.Fprintf(sb, "| scoreboard files | %d |\n", doc.ScoreboardCoverage.ScoreboardFiles)
	fmt.Fprintf(sb, "| contended scoreboards | %d |\n", doc.ScoreboardCoverage.ContendedScoreboards)
	fmt.Fprintf(sb, "| measured languages | %d |\n", doc.ScoreboardCoverage.MeasuredLanguages)
	fmt.Fprintf(sb, "| measured budget languages | %d |\n", doc.ScoreboardCoverage.MeasuredBudgetLanguages)
	fmt.Fprintf(sb, "\n")

	fmt.Fprintf(sb, "| scoreboard | languages | budgeted | contended | loadavg_start | verdicts |\n")
	fmt.Fprintf(sb, "|---|---:|---:|---|---|---|\n")
	for _, row := range doc.Scoreboards {
		fmt.Fprintf(sb, "| `%s` | %d | %d | `%t` | `%s` | %s |\n",
			mdCell(row.Path),
			row.Languages,
			row.BudgetLanguages,
			row.Contended,
			mdCell(row.LoadAvgStart),
			mdCell(formatIntMap(row.SummaryVerdicts)))
	}
	fmt.Fprintf(sb, "\n")
}

func summarizeKnownGaps(gaps map[string]knownBudgetGap) []knownGapSummary {
	keys := sortedKeys(gaps)
	out := make([]knownGapSummary, 0, len(keys))
	for _, key := range keys {
		gap := gaps[key]
		out = append(out, knownGapSummary{
			Key:        key,
			File:       gap.File,
			BacklogRef: gap.BacklogRef,
			Action:     gap.Action,
		})
	}
	return out
}

func baseCaveats(heldOut []string, gaps map[string]knownBudgetGap) []string {
	var caveats []string
	caveats = append(caveats, "The perf ratio budget is a ratchet and evidence ledger, not a universal near-C claim; >2x and cliff rows remain explicit backlog.")
	if len(heldOut) > 0 {
		caveats = append(caveats, fmt.Sprintf("%s are intentionally held out of the language ratchet until their memory/C-reference RCA rows are resolved.", strings.Join(heldOut, ", ")))
	}
	if _, ok := gaps["webworker_generated_d_ts"]; ok {
		caveats = append(caveats, "The TypeScript webworker generated-file entry remains a correctness cross-check caveat even though TypeScript has a timing budget row.")
	}
	return caveats
}

func splitCSV(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func diffSorted(all []string, remove map[string]bool) []string {
	var out []string
	for _, item := range all {
		if !remove[item] {
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return out
}

func setFromList(items []string) map[string]bool {
	out := map[string]bool{}
	for _, item := range items {
		out[item] = true
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func sortedKeysBool(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func sortedKeysInt(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func formatIntMap(m map[string]int) string {
	if len(m) == 0 {
		return ""
	}
	keys := sortedKeysInt(m)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+strconv.Itoa(m[key]))
	}
	return strings.Join(parts, ", ")
}

func mdCell(s string) string {
	if s == "" {
		return "-"
	}
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.ReplaceAll(s, "|", "\\|")
}

func mdInline(s string) string {
	return strings.ReplaceAll(s, "`", "'")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
