// Command perf_scan_manifest converts a V10 performance scoreboard into the
// row-level evidence manifest required by the R1 fleet ratchet.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	manifestSchema                 = "gts-perf-fleet-manifest/v1"
	scoreboardSchema               = "gts-perf-scan/v2"
	defaultAxis                    = "full"
	defaultHygieneTimerThresholdNS = int64(1000)
	defaultCleanRatioThreshold     = 3.0
)

type options struct {
	ScoreboardPath      string
	OutJSON             string
	OutMD               string
	GeneratedAt         string
	Axis                string
	TimerThresholdNS    int64
	CleanRatioThreshold float64
	ExpectedLanguages   int
	ExpectedRows        int
}

type scoreboard struct {
	Schema          string               `json:"schema"`
	GitRevision     string               `json:"git_revision"`
	GeneratedAt     string               `json:"generated_at"`
	Config          scoreboardConfig     `json:"config"`
	Host            scoreboardHost       `json:"host"`
	SummaryVerdicts map[string]int       `json:"summary_verdicts"`
	Corpus          scoreboardCorpus     `json:"corpus_coverage"`
	Languages       []scoreboardLanguage `json:"languages"`
	HardGate        *scoreboardGate      `json:"hard_gate,omitempty"`
}

type scoreboardConfig struct {
	CorpusRoot          string   `json:"corpus_root,omitempty"`
	Reps                int      `json:"reps"`
	Warmup              int      `json:"warmup"`
	FileBudgetMS        int      `json:"file_budget_ms"`
	LangTimeoutMS       int      `json:"lang_timeout_ms"`
	MaxFiles            int      `json:"max_files"`
	MinFileBytes        int64    `json:"min_file_bytes"`
	MaxFileBytes        int64    `json:"max_file_bytes"`
	Order               string   `json:"order"`
	Axes                []string `json:"axes"`
	ExcludePaths        []string `json:"exclude_paths,omitempty"`
	Contended           bool     `json:"contended"`
	ContendedNote       string   `json:"contended_note,omitempty"`
	ChildRSSLimitMB     int      `json:"child_rss_limit_mb"`
	CGOAdmissionRSSMB   int      `json:"cgo_admission_rss_limit_mb"`
	HardGate            bool     `json:"hard_gate"`
	RequireFleet        bool     `json:"require_fleet"`
	CorpusLockPath      string   `json:"corpus_lock,omitempty"`
	CorpusLockSHA256    string   `json:"corpus_lock_sha256,omitempty"`
	CorpusLockLanguages int      `json:"corpus_lock_languages"`
}

type scoreboardHost struct {
	LoadAvgStart string `json:"loadavg_start,omitempty"`
	LoadAvgEnd   string `json:"loadavg_end,omitempty"`
}

type scoreboardLanguage struct {
	Language      string                    `json:"language"`
	Status        string                    `json:"status"`
	FilesSelected int                       `json:"files_selected"`
	FilesMeasured int                       `json:"files_measured"`
	BytesMeasured int64                     `json:"bytes_measured"`
	Verdict       string                    `json:"verdict"`
	Axes          map[string]scoreboardAxis `json:"axes"`
	Files         []scoreboardFileRow       `json:"files"`
}

type scoreboardAxis struct {
	FilesOK            int     `json:"files_ok"`
	GoTimeouts         int     `json:"go_timeouts"`
	GoStops            int     `json:"go_stops"`
	Cliffs             int     `json:"cliffs"`
	RatioByTotal       float64 `json:"ratio_by_total"`
	RatioMedianOfFiles float64 `json:"ratio_median_of_files"`
	Verdict            string  `json:"verdict"`
}

type scoreboardFileRow struct {
	Path             string                        `json:"path"`
	Bytes            int64                         `json:"bytes,omitempty"`
	MeasurementOrder string                        `json:"measurement_order,omitempty"`
	Classification   scoreboardClassification      `json:"classification"`
	OracleAdmission  *scoreboardOracleAdmission    `json:"oracle_admission,omitempty"`
	Axes             map[string]scoreboardFileAxis `json:"axes"`
}

type scoreboardClassification struct {
	Class                    string `json:"class"`
	Reason                   string `json:"reason,omitempty"`
	GoStatus                 string `json:"go_status,omitempty"`
	FullSpan                 bool   `json:"full_span,omitempty"`
	RootHasError             bool   `json:"root_has_error,omitempty"`
	StoppedEarly             bool   `json:"stopped_early,omitempty"`
	RecoveryNodeMemoPeakTier string `json:"recovery_node_memo_peak_tier,omitempty"`
	RecoveryNodeMemoPeak     int64  `json:"recovery_node_memo_peak_entries,omitempty"`
	RecoveryNodeMemoPeakByte int64  `json:"recovery_node_memo_peak_bytes,omitempty"`
}

type scoreboardOracleAdmission struct {
	DigestFormat     string `json:"digest_format,omitempty"`
	SourceSHA256     string `json:"source_sha256,omitempty"`
	StaticDeepSHA256 string `json:"static_deep_sha256,omitempty"`
	ParityDeepSHA256 string `json:"parity_deep_sha256,omitempty"`
	CGOGrammarSHA256 string `json:"cgo_grammar_sha256,omitempty"`
	CGOPeakRSSBytes  int64  `json:"cgo_peak_rss_bytes,omitempty"`
	Admitted         bool   `json:"admitted"`
}

type scoreboardFileAxis struct {
	Status     string          `json:"status"`
	Detail     string          `json:"detail,omitempty"`
	GoMedianNS *int64          `json:"go_median_ns,omitempty"`
	CMedianNS  *int64          `json:"c_median_ns,omitempty"`
	GoMinNS    *int64          `json:"go_min_ns,omitempty"`
	GoMaxNS    *int64          `json:"go_max_ns,omitempty"`
	CMinNS     *int64          `json:"c_min_ns,omitempty"`
	CMaxNS     *int64          `json:"c_max_ns,omitempty"`
	Ratio      *float64        `json:"ratio,omitempty"`
	Verdict    string          `json:"verdict,omitempty"`
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

type scoreboardCorpus struct {
	LockPath            string   `json:"lock_path,omitempty"`
	LockSHA256          string   `json:"lock_sha256,omitempty"`
	LockLanguages       int      `json:"lock_languages"`
	SelectedLanguages   int      `json:"selected_languages"`
	MissingFromLock     []string `json:"missing_from_lock,omitempty"`
	MissingFromRegistry []string `json:"missing_from_registry,omitempty"`
}

type scoreboardGate struct {
	Status             string                  `json:"status"`
	MaxFullParseRatio  float64                 `json:"max_full_parse_ratio"`
	FastFullParseRatio float64                 `json:"fast_full_parse_ratio"`
	FilesExpected      int                     `json:"files_expected"`
	FilesMeasured      int                     `json:"files_measured"`
	FullFilesEvaluated int                     `json:"full_files_evaluated"`
	FastFullFiles      []scoreboardGateFinding `json:"fast_full_files,omitempty"`
	Failures           []scoreboardGateFinding `json:"failures,omitempty"`
}

type scoreboardGateFinding struct {
	Kind     string          `json:"kind"`
	Language string          `json:"language,omitempty"`
	Path     string          `json:"path,omitempty"`
	Axis     string          `json:"axis,omitempty"`
	Status   string          `json:"status,omitempty"`
	Ratio    *float64        `json:"ratio,omitempty"`
	Limit    *float64        `json:"limit,omitempty"`
	Stop     *scoreboardStop `json:"stop,omitempty"`
	Detail   string          `json:"detail,omitempty"`
}

type manifest struct {
	Schema      string             `json:"schema"`
	GeneratedAt string             `json:"generated_at"`
	Axis        string             `json:"axis"`
	Source      manifestSource     `json:"source"`
	Policy      manifestPolicy     `json:"policy"`
	Validation  manifestValidation `json:"validation"`
	Summary     manifestSummary    `json:"summary"`
	Languages   []manifestLanguage `json:"languages"`
	Rows        []manifestRow      `json:"rows"`
	Findings    []manifestFinding  `json:"findings"`
}

type manifestSource struct {
	Artifact         string           `json:"artifact"`
	ScoreboardSHA256 string           `json:"scoreboard_sha256"`
	Schema           string           `json:"schema"`
	GitRevision      string           `json:"git_revision"`
	GeneratedAt      string           `json:"generated_at"`
	Config           scoreboardConfig `json:"config"`
	Host             scoreboardHost   `json:"host"`
	Corpus           scoreboardCorpus `json:"corpus_coverage"`
	HardGate         manifestGate     `json:"hard_gate"`
	SummaryVerdicts  map[string]int   `json:"summary_verdicts,omitempty"`
}

type manifestGate struct {
	Status             string  `json:"status"`
	MaxFullParseRatio  float64 `json:"max_full_parse_ratio"`
	FastFullParseRatio float64 `json:"fast_full_parse_ratio"`
	FilesExpected      int     `json:"files_expected"`
	FilesMeasured      int     `json:"files_measured"`
	FullFilesEvaluated int     `json:"full_files_evaluated"`
	FailureRecords     int     `json:"failure_records"`
	FastFullRecords    int     `json:"fast_full_records"`
}

type manifestPolicy struct {
	TimerThresholdNS        int64    `json:"timer_threshold_ns"`
	CleanRatioThreshold     float64  `json:"clean_ratio_threshold"`
	ThresholdScope          string   `json:"threshold_scope"`
	HygienePredicate        string   `json:"hygiene_predicate"`
	ClassSeparation         []string `json:"class_separation"`
	FindingsAreOverlapping  bool     `json:"findings_are_overlapping"`
	AdmissionRoutingChanged bool     `json:"admission_routing_changed"`
}

type manifestValidation struct {
	ExpectedLanguages    int  `json:"expected_languages"`
	ActualLanguages      int  `json:"actual_languages"`
	ExpectedRows         int  `json:"expected_rows"`
	ActualRows           int  `json:"actual_rows"`
	DuplicateRows        int  `json:"duplicate_rows"`
	CorpusLanguagesMatch bool `json:"corpus_languages_match"`
	GateRowsMatch        bool `json:"gate_rows_match"`
	Valid                bool `json:"valid"`
}

type manifestSummary struct {
	TotalLanguages                   int                `json:"total_languages"`
	TotalRows                        int                `json:"total_rows"`
	SemanticClassRows                map[string]int     `json:"semantic_class_rows"`
	AxisStatusRows                   map[string]int     `json:"axis_status_rows"`
	MeasuredOKRows                   int                `json:"measured_ok_rows"`
	NonMeasuredRows                  int                `json:"non_measured_rows"`
	RawCMedianPresentRows            int                `json:"raw_c_median_present_rows"`
	RawCMedianMissingRows            int                `json:"raw_c_median_missing_rows"`
	RawCMedianMeasuredRows           int                `json:"raw_c_median_measured_rows"`
	RawCMedianMissingMeasuredRows    int                `json:"raw_c_median_missing_measured_rows"`
	RawCMedianPresentNonMeasuredRows int                `json:"raw_c_median_present_non_measured_rows"`
	RatioInterpretableRows           int                `json:"ratio_interpretable_rows"`
	RatioNoninterpretableRows        int                `json:"ratio_noninterpretable_rows"`
	CleanRows                        int                `json:"clean_rows"`
	ErrorRows                        int                `json:"error_rows"`
	StoppedRows                      int                `json:"stopped_rows"`
	CleanRowsAbove3x                 int                `json:"clean_rows_above_3x"`
	CleanRowsAbove3xSignal           int                `json:"clean_rows_above_3x_signal"`
	FindingRecords                   int                `json:"finding_records"`
	FindingRecordsByKind             map[string]int     `json:"finding_records_by_kind"`
	PathFindingRowsByKind            map[string]int     `json:"path_finding_rows_by_kind"`
	LanguageFindingRecords           int                `json:"language_finding_records"`
	Hygiene                          hygieneDenominator `json:"hygiene_denominator"`
}

type hygieneDenominator struct {
	AllRows                       int `json:"all_rows"`
	MeasuredOKRows                int `json:"measured_ok_rows"`
	RawCMedianRows                int `json:"raw_c_median_rows"`
	RawCMedianMissingRows         int `json:"raw_c_median_missing_rows"`
	MeasuredRawCMedianRows        int `json:"measured_raw_c_median_rows"`
	MeasuredRawCMedianMissingRows int `json:"measured_raw_c_median_missing_rows"`
	BelowTimerThresholdRows       int `json:"below_timer_threshold_rows"`
	AtOrAboveTimerThresholdRows   int `json:"at_or_above_timer_threshold_rows"`
	RatioEligibleRows             int `json:"ratio_eligible_rows"`
	CleanRows                     int `json:"clean_rows"`
	CleanMeasuredRows             int `json:"clean_measured_rows"`
	CleanSignalRows               int `json:"clean_signal_rows"`
	ErrorRows                     int `json:"error_rows"`
	ErrorMeasuredRows             int `json:"error_measured_rows"`
	StoppedRows                   int `json:"stopped_rows"`
}

type manifestLanguage struct {
	Language          string                    `json:"language"`
	Status            string                    `json:"status"`
	Verdict           string                    `json:"verdict"`
	FilesSelected     int                       `json:"files_selected"`
	FilesMeasured     int                       `json:"files_measured"`
	BytesMeasured     int64                     `json:"bytes_measured"`
	Rows              int                       `json:"rows"`
	SemanticClassRows map[string]int            `json:"semantic_class_rows"`
	Axes              map[string]scoreboardAxis `json:"axes,omitempty"`
}

type manifestRow struct {
	ID                 string                     `json:"id"`
	SourceOrdinal      int                        `json:"source_ordinal"`
	Language           string                     `json:"language"`
	Path               string                     `json:"path"`
	Bytes              int64                      `json:"bytes,omitempty"`
	MeasurementOrder   string                     `json:"measurement_order,omitempty"`
	SemanticClass      string                     `json:"semantic_class"`
	Classification     scoreboardClassification   `json:"classification"`
	OracleAdmission    *scoreboardOracleAdmission `json:"oracle_admission,omitempty"`
	Axis               scoreboardFileAxis         `json:"axis"`
	Predicates         rowPredicates              `json:"predicates"`
	PathFindingIDs     []string                   `json:"path_finding_ids,omitempty"`
	LanguageFindingIDs []string                   `json:"language_finding_ids,omitempty"`
}

type rowPredicates struct {
	SemanticClean              bool `json:"semantic_clean"`
	SemanticError              bool `json:"semantic_error"`
	SemanticStopped            bool `json:"semantic_stopped"`
	AxisOK                     bool `json:"axis_ok"`
	Measured                   bool `json:"measured"`
	RawCMedianPresent          bool `json:"raw_c_median_present"`
	RawCMedianBelowThreshold   bool `json:"raw_c_median_below_threshold"`
	RatioInterpretable         bool `json:"ratio_interpretable"`
	CleanAbove3x               bool `json:"clean_above_3x"`
	CleanAbove3xSignal         bool `json:"clean_above_3x_signal"`
	HasPathCoverageFinding     bool `json:"has_path_coverage_finding"`
	HasPathRatioFinding        bool `json:"has_path_ratio_finding"`
	HasPathGoStopFinding       bool `json:"has_path_go_stop_finding"`
	HasPathOracleFinding       bool `json:"has_path_oracle_finding"`
	HasLanguageCoverageFinding bool `json:"has_language_coverage_finding"`
	HasLanguageRatioFinding    bool `json:"has_language_ratio_finding"`
	HasLanguageGoStopFinding   bool `json:"has_language_go_stop_finding"`
	HasLanguageOracleFinding   bool `json:"has_language_oracle_finding"`
}

type manifestFinding struct {
	ID          string          `json:"id"`
	Source      string          `json:"source"`
	Kind        string          `json:"kind"`
	Language    string          `json:"language,omitempty"`
	Path        string          `json:"path,omitempty"`
	Axis        string          `json:"axis,omitempty"`
	Status      string          `json:"status,omitempty"`
	Ratio       *float64        `json:"ratio,omitempty"`
	Limit       *float64        `json:"limit,omitempty"`
	Stop        *scoreboardStop `json:"stop,omitempty"`
	Detail      string          `json:"detail,omitempty"`
	sourceIndex int             `json:"-"`
}

func main() {
	opts := options{}
	flag.StringVar(&opts.ScoreboardPath, "scoreboard", "", "V2 perf scan scoreboard JSON path")
	flag.StringVar(&opts.OutJSON, "out-json", "", "optional manifest JSON output path")
	flag.StringVar(&opts.OutMD, "out-md", "", "optional manifest Markdown output path")
	flag.StringVar(&opts.GeneratedAt, "generated-at", "", "manifest timestamp override")
	flag.StringVar(&opts.Axis, "axis", defaultAxis, "scoreboard axis to manifest")
	flag.Int64Var(&opts.TimerThresholdNS, "timer-threshold-ns", defaultHygieneTimerThresholdNS, "fixed raw C median threshold for ratio hygiene")
	flag.Float64Var(&opts.CleanRatioThreshold, "clean-ratio-threshold", defaultCleanRatioThreshold, "strict ratio threshold for clean tail rows")
	flag.IntVar(&opts.ExpectedLanguages, "expected-languages", 0, "fail unless the scoreboard has this many languages")
	flag.IntVar(&opts.ExpectedRows, "expected-rows", 0, "fail unless the scoreboard has this many file rows")
	flag.Parse()

	if opts.ScoreboardPath == "" {
		fatalf("-scoreboard is required")
	}
	if opts.Axis == "" {
		fatalf("-axis cannot be empty")
	}
	if opts.TimerThresholdNS <= 0 {
		fatalf("-timer-threshold-ns must be positive")
	}
	if opts.CleanRatioThreshold <= 0 || math.IsNaN(opts.CleanRatioThreshold) || math.IsInf(opts.CleanRatioThreshold, 0) {
		fatalf("-clean-ratio-threshold must be finite and positive")
	}

	doc, err := buildManifest(opts)
	if err != nil {
		fatalf("build manifest: %v", err)
	}
	if opts.OutJSON == "" && opts.OutMD == "" {
		fmt.Print(renderMarkdown(doc))
		return
	}
	if opts.OutJSON != "" {
		data, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			fatalf("marshal manifest: %v", err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(opts.OutJSON, data, 0o644); err != nil {
			fatalf("write manifest JSON: %v", err)
		}
	}
	if opts.OutMD != "" {
		if err := os.WriteFile(opts.OutMD, []byte(renderMarkdown(doc)), 0o644); err != nil {
			fatalf("write manifest Markdown: %v", err)
		}
	}
}

func buildManifest(opts options) (*manifest, error) {
	data, err := os.ReadFile(opts.ScoreboardPath)
	if err != nil {
		return nil, fmt.Errorf("read scoreboard: %w", err)
	}
	var board scoreboard
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&board); err != nil {
		return nil, fmt.Errorf("decode scoreboard: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("scoreboard contains trailing JSON")
		}
		return nil, fmt.Errorf("decode trailing scoreboard JSON: %w", err)
	}
	if board.Schema != scoreboardSchema {
		return nil, fmt.Errorf("scoreboard schema %q, want %q", board.Schema, scoreboardSchema)
	}
	if board.Config.CorpusLockSHA256 != "" && board.Corpus.LockSHA256 != "" && board.Config.CorpusLockSHA256 != board.Corpus.LockSHA256 {
		return nil, fmt.Errorf("scoreboard config and corpus lock digests differ")
	}
	if board.Corpus.LockLanguages > 0 && board.Corpus.LockLanguages != len(board.Languages) {
		return nil, fmt.Errorf("scoreboard has %d languages but corpus lock declares %d", len(board.Languages), board.Corpus.LockLanguages)
	}

	findings := collectFindings(board.HardGate, opts.Axis)
	findingsBySource := make(map[int]string, len(findings))
	for i := range findings {
		findings[i].ID = fmt.Sprintf("finding-%03d", i+1)
		findingsBySource[findings[i].sourceIndex] = findings[i].ID
	}

	doc := &manifest{
		Schema:      manifestSchema,
		GeneratedAt: opts.GeneratedAt,
		Axis:        opts.Axis,
		Source: manifestSource{
			Artifact:         "V10 accepted measurement epoch scoreboard",
			ScoreboardSHA256: sha256Hex(data),
			Schema:           board.Schema,
			GitRevision:      board.GitRevision,
			GeneratedAt:      board.GeneratedAt,
			Config:           board.Config,
			Host:             board.Host,
			Corpus:           board.Corpus,
			SummaryVerdicts:  board.SummaryVerdicts,
		},
		Policy: manifestPolicy{
			TimerThresholdNS:        opts.TimerThresholdNS,
			CleanRatioThreshold:     opts.CleanRatioThreshold,
			ThresholdScope:          "F0 report policy; it does not alter the source epoch or parser admission",
			HygienePredicate:        fmt.Sprintf("axis.status == ok and c_median_ns < %d", opts.TimerThresholdNS),
			ClassSeparation:         []string{"clean", "error", "stopped", "hygiene"},
			FindingsAreOverlapping:  true,
			AdmissionRoutingChanged: false,
		},
	}
	if opts.GeneratedAt == "" {
		doc.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if board.HardGate != nil {
		doc.Source.HardGate = manifestGate{
			Status:             board.HardGate.Status,
			MaxFullParseRatio:  board.HardGate.MaxFullParseRatio,
			FastFullParseRatio: board.HardGate.FastFullParseRatio,
			FilesExpected:      board.HardGate.FilesExpected,
			FilesMeasured:      board.HardGate.FilesMeasured,
			FullFilesEvaluated: board.HardGate.FullFilesEvaluated,
			FailureRecords:     len(board.HardGate.Failures),
			FastFullRecords:    len(board.HardGate.FastFullFiles),
		}
	}

	seenRows := map[string]bool{}
	rowOrdinal := 0
	for _, language := range board.Languages {
		languageOut := manifestLanguage{
			Language:          language.Language,
			Status:            language.Status,
			Verdict:           language.Verdict,
			FilesSelected:     language.FilesSelected,
			FilesMeasured:     language.FilesMeasured,
			BytesMeasured:     language.BytesMeasured,
			Rows:              len(language.Files),
			SemanticClassRows: map[string]int{},
			Axes:              language.Axes,
		}
		for _, sourceRow := range language.Files {
			rowOrdinal++
			if language.Language == "" || sourceRow.Path == "" {
				return nil, fmt.Errorf("row %d has an empty language or path", rowOrdinal)
			}
			rowKey := language.Language + "\x00" + sourceRow.Path
			if seenRows[rowKey] {
				return nil, fmt.Errorf("duplicate row %q/%q", language.Language, sourceRow.Path)
			}
			seenRows[rowKey] = true
			axisValue, ok := sourceRow.Axes[opts.Axis]
			if !ok {
				return nil, fmt.Errorf("row %q/%q has no %s axis", language.Language, sourceRow.Path, opts.Axis)
			}
			class := sourceRow.Classification.Class
			if class != "clean" && class != "error" && class != "stopped" {
				return nil, fmt.Errorf("row %q/%q has unknown semantic class %q", language.Language, sourceRow.Path, class)
			}
			languageOut.SemanticClassRows[class]++
			predicates := classifyRow(class, axisValue, opts.TimerThresholdNS, opts.CleanRatioThreshold, findings, language.Language, sourceRow.Path, opts.Axis)
			row := manifestRow{
				ID:               fmt.Sprintf("row-%04d", rowOrdinal),
				SourceOrdinal:    rowOrdinal,
				Language:         language.Language,
				Path:             sourceRow.Path,
				Bytes:            sourceRow.Bytes,
				MeasurementOrder: sourceRow.MeasurementOrder,
				SemanticClass:    class,
				Classification:   sourceRow.Classification,
				OracleAdmission:  sourceRow.OracleAdmission,
				Axis:             axisValue,
				Predicates:       predicates,
			}
			for _, finding := range findings {
				if finding.Language != language.Language {
					continue
				}
				if finding.Axis != "" && finding.Axis != opts.Axis {
					continue
				}
				id := findingsBySource[finding.sourceIndex]
				if finding.Path == "" {
					row.LanguageFindingIDs = append(row.LanguageFindingIDs, id)
				} else if finding.Path == sourceRow.Path {
					row.PathFindingIDs = append(row.PathFindingIDs, id)
				}
			}
			doc.Rows = append(doc.Rows, row)
		}
		doc.Languages = append(doc.Languages, languageOut)
	}
	doc.Findings = findings
	doc.Summary = summarize(doc.Rows, findings, opts.Axis)
	doc.Summary.TotalLanguages = len(doc.Languages)
	doc.Validation = validateManifest(doc, opts, board)
	if !doc.Validation.Valid {
		return nil, fmt.Errorf("manifest validation failed: expected %d languages/%d rows, got %d/%d", doc.Validation.ExpectedLanguages, doc.Validation.ExpectedRows, doc.Validation.ActualLanguages, doc.Validation.ActualRows)
	}
	return doc, nil
}

func collectFindings(gate *scoreboardGate, axis string) []manifestFinding {
	if gate == nil {
		return nil
	}
	findings := make([]manifestFinding, 0, len(gate.Failures)+len(gate.FastFullFiles))
	index := 0
	appendFinding := func(source string, finding scoreboardGateFinding) {
		index++
		findings = append(findings, manifestFinding{
			Source:      source,
			Kind:        finding.Kind,
			Language:    finding.Language,
			Path:        finding.Path,
			Axis:        finding.Axis,
			Status:      finding.Status,
			Ratio:       finding.Ratio,
			Limit:       finding.Limit,
			Stop:        finding.Stop,
			Detail:      finding.Detail,
			sourceIndex: index,
		})
	}
	for _, finding := range gate.Failures {
		appendFinding("failures", finding)
	}
	for _, finding := range gate.FastFullFiles {
		appendFinding("fast_full_files", finding)
	}
	sort.SliceStable(findings, func(i, j int) bool {
		left, right := findings[i], findings[j]
		for _, pair := range [][2]string{{left.Kind, right.Kind}, {left.Language, right.Language}, {left.Path, right.Path}, {left.Axis, right.Axis}, {left.Status, right.Status}, {left.Detail, right.Detail}} {
			if pair[0] != pair[1] {
				return pair[0] < pair[1]
			}
		}
		return left.sourceIndex < right.sourceIndex
	})
	_ = axis
	return findings
}

func classifyRow(class string, axis scoreboardFileAxis, threshold int64, cleanThreshold float64, findings []manifestFinding, language, path, targetAxis string) rowPredicates {
	predicates := rowPredicates{
		SemanticClean:     class == "clean",
		SemanticError:     class == "error",
		SemanticStopped:   class == "stopped",
		AxisOK:            axis.Status == "ok",
		Measured:          axis.Status == "ok",
		RawCMedianPresent: axis.CMedianNS != nil,
	}
	predicates.RawCMedianBelowThreshold = predicates.Measured && predicates.RawCMedianPresent && *axis.CMedianNS < threshold
	predicates.RatioInterpretable = predicates.Measured && predicates.RawCMedianPresent && !predicates.RawCMedianBelowThreshold && axis.Ratio != nil
	predicates.CleanAbove3x = predicates.SemanticClean && predicates.Measured && axis.Ratio != nil && *axis.Ratio > cleanThreshold
	predicates.CleanAbove3xSignal = predicates.CleanAbove3x && predicates.RatioInterpretable
	for _, finding := range findings {
		if finding.Language != language || (finding.Axis != "" && finding.Axis != targetAxis) {
			continue
		}
		languageLevel := finding.Path == ""
		pathMatch := !languageLevel && finding.Path == path
		if !languageLevel && !pathMatch {
			continue
		}
		setFindingPredicate(&predicates, finding.Kind, languageLevel)
	}
	return predicates
}

func setFindingPredicate(predicates *rowPredicates, kind string, languageLevel bool) {
	switch kind {
	case "coverage":
		if languageLevel {
			predicates.HasLanguageCoverageFinding = true
		} else {
			predicates.HasPathCoverageFinding = true
		}
	case "ratio":
		if languageLevel {
			predicates.HasLanguageRatioFinding = true
		} else {
			predicates.HasPathRatioFinding = true
		}
	case "go_stop":
		if languageLevel {
			predicates.HasLanguageGoStopFinding = true
		} else {
			predicates.HasPathGoStopFinding = true
		}
	case "oracle":
		if languageLevel {
			predicates.HasLanguageOracleFinding = true
		} else {
			predicates.HasPathOracleFinding = true
		}
	}
}

func summarize(rows []manifestRow, findings []manifestFinding, axis string) manifestSummary {
	summary := manifestSummary{
		TotalRows:             len(rows),
		SemanticClassRows:     map[string]int{},
		AxisStatusRows:        map[string]int{},
		FindingRecords:        len(findings),
		FindingRecordsByKind:  map[string]int{},
		PathFindingRowsByKind: map[string]int{},
	}
	for _, row := range rows {
		summary.SemanticClassRows[row.SemanticClass]++
		summary.AxisStatusRows[row.Axis.Status]++
		switch row.SemanticClass {
		case "clean":
			summary.CleanRows++
		case "error":
			summary.ErrorRows++
		case "stopped":
			summary.StoppedRows++
		}
		if row.Predicates.Measured {
			summary.MeasuredOKRows++
		} else {
			summary.NonMeasuredRows++
		}
		if row.Predicates.RawCMedianPresent {
			summary.RawCMedianPresentRows++
		} else {
			summary.RawCMedianMissingRows++
		}
		if row.Predicates.Measured {
			if row.Predicates.RawCMedianPresent {
				summary.RawCMedianMeasuredRows++
			} else {
				summary.RawCMedianMissingMeasuredRows++
			}
		} else if row.Predicates.RawCMedianPresent {
			summary.RawCMedianPresentNonMeasuredRows++
		}
		if row.Predicates.RatioInterpretable {
			summary.RatioInterpretableRows++
		} else if row.Predicates.RawCMedianBelowThreshold {
			summary.RatioNoninterpretableRows++
		}
		if row.Predicates.CleanAbove3x {
			summary.CleanRowsAbove3x++
		}
		if row.Predicates.CleanAbove3xSignal {
			summary.CleanRowsAbove3xSignal++
		}
	}
	pathRowsByKind := map[string]map[string]bool{}
	for _, finding := range findings {
		summary.FindingRecordsByKind[finding.Kind]++
		if finding.Path != "" {
			for _, row := range rows {
				if row.Language == finding.Language && row.Path == finding.Path && (finding.Axis == "" || finding.Axis == rowAxis(row, axis)) {
					if pathRowsByKind[finding.Kind] == nil {
						pathRowsByKind[finding.Kind] = map[string]bool{}
					}
					pathRowsByKind[finding.Kind][row.ID] = true
				}
			}
		} else {
			summary.LanguageFindingRecords++
		}
	}
	for kind, rows := range pathRowsByKind {
		summary.PathFindingRowsByKind[kind] = len(rows)
	}
	summary.Hygiene = hygieneDenominator{
		AllRows:                       len(rows),
		MeasuredOKRows:                summary.MeasuredOKRows,
		RawCMedianRows:                summary.RawCMedianPresentRows,
		RawCMedianMissingRows:         summary.RawCMedianMissingRows,
		MeasuredRawCMedianRows:        summary.RawCMedianMeasuredRows,
		MeasuredRawCMedianMissingRows: summary.RawCMedianMissingMeasuredRows,
		BelowTimerThresholdRows:       summary.RatioNoninterpretableRows,
		AtOrAboveTimerThresholdRows:   summary.RatioInterpretableRows,
		RatioEligibleRows:             summary.RatioInterpretableRows,
		CleanRows:                     summary.CleanRows,
		CleanMeasuredRows:             countRows(rows, func(row manifestRow) bool { return row.SemanticClass == "clean" && row.Predicates.Measured }),
		CleanSignalRows:               countRows(rows, func(row manifestRow) bool { return row.SemanticClass == "clean" && row.Predicates.RatioInterpretable }),
		ErrorRows:                     summary.ErrorRows,
		ErrorMeasuredRows:             countRows(rows, func(row manifestRow) bool { return row.SemanticClass == "error" && row.Predicates.Measured }),
		StoppedRows:                   summary.StoppedRows,
	}
	return summary
}

func rowAxis(row manifestRow, axis string) string {
	if row.Axis.Status == "" {
		return ""
	}
	return axis
}

func countRows(rows []manifestRow, predicate func(manifestRow) bool) int {
	count := 0
	for _, row := range rows {
		if predicate(row) {
			count++
		}
	}
	return count
}

func validateManifest(doc *manifest, opts options, board scoreboard) manifestValidation {
	actualLanguages := len(doc.Languages)
	actualRows := len(doc.Rows)
	expectedLanguages := opts.ExpectedLanguages
	expectedRows := opts.ExpectedRows
	if expectedLanguages == 0 {
		expectedLanguages = actualLanguages
	}
	if expectedRows == 0 {
		expectedRows = actualRows
	}
	corpusMatch := board.Corpus.LockLanguages == 0 || board.Corpus.LockLanguages == actualLanguages
	gateMatch := board.HardGate == nil || board.HardGate.FilesExpected == 0 || board.HardGate.FilesExpected == actualRows
	valid := expectedLanguages == actualLanguages && expectedRows == actualRows && corpusMatch && gateMatch
	return manifestValidation{
		ExpectedLanguages:    expectedLanguages,
		ActualLanguages:      actualLanguages,
		ExpectedRows:         expectedRows,
		ActualRows:           actualRows,
		DuplicateRows:        0,
		CorpusLanguagesMatch: corpusMatch,
		GateRowsMatch:        gateMatch,
		Valid:                valid,
	}
}

func loadManifest(path string) (*manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc manifest
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func renderMarkdown(doc *manifest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# V10 fleet manifest\n\n")
	fmt.Fprintf(&b, "This manifest is derived from the accepted V10 scoreboard. It does not change parser admission or routing.\n\n")
	fmt.Fprintf(&b, "- Source revision: `%s`\n", doc.Source.GitRevision)
	fmt.Fprintf(&b, "- Source scoreboard SHA-256: `%s`\n", doc.Source.ScoreboardSHA256)
	fmt.Fprintf(&b, "- Source epoch: `%s`\n", doc.Source.GeneratedAt)
	fmt.Fprintf(&b, "- Axis: `%s`\n", doc.Axis)
	fmt.Fprintf(&b, "- Manifest generated at: `%s`\n", doc.GeneratedAt)
	fmt.Fprintf(&b, "- Validation: `%t` (%d languages, %d rows)\n\n", doc.Validation.Valid, doc.Validation.ActualLanguages, doc.Validation.ActualRows)

	fmt.Fprintf(&b, "## Class and timing denominator\n\n")
	fmt.Fprintf(&b, "The manifest keeps semantic classes and timing hygiene separate.\n\n")
	fmt.Fprintf(&b, "| Predicate | Count |\n|---|---:|\n")
	fmt.Fprintf(&b, "| Clean rows | %d |\n", doc.Summary.CleanRows)
	fmt.Fprintf(&b, "| Error rows | %d |\n", doc.Summary.ErrorRows)
	fmt.Fprintf(&b, "| Stopped rows | %d |\n", doc.Summary.StoppedRows)
	fmt.Fprintf(&b, "| Measured rows | %d |\n", doc.Summary.MeasuredOKRows)
	fmt.Fprintf(&b, "| Non-measured rows | %d |\n", doc.Summary.NonMeasuredRows)
	fmt.Fprintf(&b, "| Raw C median present | %d |\n", doc.Summary.RawCMedianPresentRows)
	fmt.Fprintf(&b, "| Raw C median missing | %d |\n", doc.Summary.RawCMedianMissingRows)
	fmt.Fprintf(&b, "| Measured rows with raw C median | %d |\n", doc.Summary.RawCMedianMeasuredRows)
	fmt.Fprintf(&b, "| Measured rows missing raw C median | %d |\n", doc.Summary.RawCMedianMissingMeasuredRows)
	fmt.Fprintf(&b, "| Non-measured rows with raw C median | %d |\n", doc.Summary.RawCMedianPresentNonMeasuredRows)
	fmt.Fprintf(&b, "| Ratio-noninterpretable rows | %d |\n", doc.Summary.RatioNoninterpretableRows)
	fmt.Fprintf(&b, "| Ratio-interpretable rows | %d |\n", doc.Summary.RatioInterpretableRows)
	fmt.Fprintf(&b, "| Clean rows above 3.0x | %d |\n", doc.Summary.CleanRowsAbove3x)
	fmt.Fprintf(&b, "| Clean rows above 3.0x after hygiene | %d |\n", doc.Summary.CleanRowsAbove3xSignal)
	fmt.Fprintf(&b, "\nThe fixed F0 hygiene threshold is `%d ns`. The threshold applies to reporting only.\n\n", doc.Policy.TimerThresholdNS)

	fmt.Fprintf(&b, "## Gate findings\n\n")
	fmt.Fprintf(&b, "The %d gate findings are predicates. They can overlap the same row.\n\n", doc.Summary.FindingRecords)
	fmt.Fprintf(&b, "| Finding kind | Records |\n|---|---:|\n")
	for _, key := range sortedKeys(doc.Summary.FindingRecordsByKind) {
		fmt.Fprintf(&b, "| `%s` | %d |\n", key, doc.Summary.FindingRecordsByKind[key])
	}
	fmt.Fprintf(&b, "\n## Source gate\n\n")
	fmt.Fprintf(&b, "- Status: `%s`\n", doc.Source.HardGate.Status)
	fmt.Fprintf(&b, "- Files expected: %d\n", doc.Source.HardGate.FilesExpected)
	fmt.Fprintf(&b, "- Files measured: %d\n", doc.Source.HardGate.FilesMeasured)
	fmt.Fprintf(&b, "- Full files evaluated: %d\n", doc.Source.HardGate.FullFilesEvaluated)
	fmt.Fprintf(&b, "- Failure records: %d\n", doc.Source.HardGate.FailureRecords)
	fmt.Fprintf(&b, "\n## Reproduce\n\n")
	fmt.Fprintf(&b, "Run `go run ./cgo_harness/cmd/perf_scan_manifest -scoreboard <v10-scoreboard.json> -expected-languages %d -expected-rows %d -timer-threshold-ns %d -clean-ratio-threshold %.1f -out-json cgo_harness/perf_scan/out/<epoch>/manifest.json -out-md cgo_harness/perf_scan/out/<epoch>/manifest.md`.\n", doc.Validation.ActualLanguages, doc.Validation.ActualRows, doc.Policy.TimerThresholdNS, doc.Policy.CleanRatioThreshold)
	return b.String()
}

func sortedKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "perf_scan_manifest: "+format+"\n", args...)
	os.Exit(1)
}
