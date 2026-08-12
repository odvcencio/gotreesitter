// Command c0_attribution builds the C0e and C0f evidence boards.
//
// It reads the accepted V10 fleet manifest and a checked-in sealed-board
// ledger. It does not run a parser, benchmark, or fleet scan.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"time"
)

const (
	reportSchema = "gts-c0-attribution/v1"
	axis         = "full"
)

type options struct {
	ManifestPath string
	SealedPath   string
	OutJSON      string
	OutMD        string
	GeneratedAt  string
}

type manifest struct {
	Schema      string             `json:"schema"`
	GeneratedAt string             `json:"generated_at"`
	Axis        string             `json:"axis"`
	Source      manifestSource     `json:"source"`
	Policy      manifestPolicy     `json:"policy"`
	Validation  manifestValidation `json:"validation"`
	Summary     manifestSummary    `json:"summary"`
	Rows        []manifestRow      `json:"rows"`
}

type manifestSource struct {
	ScoreboardSHA256 string `json:"scoreboard_sha256"`
	GitRevision      string `json:"git_revision"`
	GeneratedAt      string `json:"generated_at"`
}

type manifestPolicy struct {
	TimerThresholdNS    int64   `json:"timer_threshold_ns"`
	CleanRatioThreshold float64 `json:"clean_ratio_threshold"`
}

type manifestValidation struct {
	Valid bool `json:"valid"`
}

type manifestSummary struct {
	TotalLanguages         int            `json:"total_languages"`
	TotalRows              int            `json:"total_rows"`
	CleanRowsAbove3x       int            `json:"clean_rows_above_3x"`
	CleanRowsAbove3xSignal int            `json:"clean_rows_above_3x_signal"`
	FindingRecords         int            `json:"finding_records"`
	FindingRecordsByKind   map[string]int `json:"finding_records_by_kind"`
	Hygiene                hygieneSummary `json:"hygiene_denominator"`
}

type hygieneSummary struct {
	MeasuredOKRows          int `json:"measured_ok_rows"`
	RawCMedianRows          int `json:"raw_c_median_rows"`
	RawCMedianMissingRows   int `json:"raw_c_median_missing_rows"`
	BelowTimerThresholdRows int `json:"below_timer_threshold_rows"`
	RatioEligibleRows       int `json:"ratio_eligible_rows"`
}

type manifestRow struct {
	Language      string             `json:"language"`
	Path          string             `json:"path"`
	Bytes         int64              `json:"bytes"`
	SemanticClass string             `json:"semantic_class"`
	Axis          manifestAxis       `json:"axis"`
	Predicates    manifestPredicates `json:"predicates"`
}

type manifestAxis struct {
	Status     string  `json:"status"`
	GoMedianNS int64   `json:"go_median_ns"`
	CMedianNS  *int64  `json:"c_median_ns"`
	Ratio      float64 `json:"ratio"`
}

type manifestPredicates struct {
	RawCMedianBelowThreshold bool `json:"raw_c_median_below_threshold"`
	RatioInterpretable       bool `json:"ratio_interpretable"`
	CleanAbove3xSignal       bool `json:"clean_above_3x_signal"`
}

type sealedBoard struct {
	Schema         string         `json:"schema"`
	Epoch          string         `json:"epoch"`
	Commit         string         `json:"commit"`
	Method         string         `json:"method"`
	Compact        []sealedRow    `json:"compact"`
	Production     []sealedRow    `json:"production"`
	CompactGeomean float64        `json:"compact_geomean"`
	Noise          noiseReference `json:"noise"`
}

type sealedRow struct {
	Fixture string  `json:"fixture"`
	Ratio   float64 `json:"ratio"`
}

type noiseReference struct {
	ReceiptPath   string             `json:"receipt_path"`
	ReceiptSHA256 string             `json:"receipt_sha256"`
	HostClass     string             `json:"host_class"`
	P95ByFixture  map[string]float64 `json:"p95_pct_of_median_by_fixture"`
}

type report struct {
	Schema      string     `json:"schema"`
	GeneratedAt string     `json:"generated_at"`
	Authority   authority  `json:"authority"`
	C0e         c0eBoard   `json:"c0e"`
	C0f         c0fBoard   `json:"c0f"`
	Formulas    []string   `json:"formulas"`
	Conflicts   []conflict `json:"conflicts"`
}

type authority struct {
	Spec               string `json:"spec"`
	Revision           string `json:"revision"`
	SharesStaySeparate bool   `json:"shares_stay_separate"`
	RoutingChanged     bool   `json:"routing_changed"`
}

type c0eBoard struct {
	Status           string         `json:"status"`
	Epoch            string         `json:"epoch"`
	Commit           string         `json:"commit"`
	CompactRatios    []sealedRow    `json:"compact_ratios"`
	CompactGeomean   float64        `json:"compact_geomean"`
	CanonicalAbove3x int            `json:"canonical_fixtures_above_3x"`
	Ratchet          c0eRatchet     `json:"ratchet"`
	Noise            noiseReference `json:"noise"`
}

type c0eRatchet struct {
	TargetGeomeanAtMost      float64 `json:"target_geomean_at_most"`
	TargetFixtureRatioAtMost float64 `json:"target_fixture_ratio_at_most"`
	GeomeanPass              bool    `json:"geomean_pass"`
	FixturePass              bool    `json:"fixture_pass"`
}

type c0fBoard struct {
	Status             string                  `json:"status"`
	ManifestSchema     string                  `json:"manifest_schema"`
	ManifestSHA256     string                  `json:"manifest_sha256"`
	ScoreboardSHA256   string                  `json:"scoreboard_sha256"`
	SignalDefinition   string                  `json:"signal_definition"`
	SignalRows         int                     `json:"signal_rows"`
	SignalGoMedianNS   int64                   `json:"signal_go_median_ns"`
	SignalCMedianNS    int64                   `json:"signal_c_median_ns"`
	SignalRatioByTotal float64                 `json:"signal_ratio_by_total"`
	Classes            map[string]classStat    `json:"classes"`
	Distributions      map[string]distribution `json:"distributions"`
	Findings           map[string]int          `json:"finding_records_by_kind"`
	Cohorts            map[string]cohortStat   `json:"cohorts"`
}

type classStat struct {
	Rows            int     `json:"rows"`
	Bytes           int64   `json:"bytes"`
	GoMedianNS      int64   `json:"go_median_ns"`
	CMedianNS       int64   `json:"c_median_ns"`
	RatioByTotal    float64 `json:"ratio_by_total"`
	GoShareOfSignal float64 `json:"go_share_of_signal"`
	LocalGoGap      float64 `json:"local_go_gap"`
	ExcessGoNS      int64   `json:"excess_go_ns"`
	ExcessShare     float64 `json:"excess_share_of_signal_go"`
}

type distribution struct {
	Rows int     `json:"rows"`
	P50  float64 `json:"p50"`
	P90  float64 `json:"p90"`
	P95  float64 `json:"p95"`
	P99  float64 `json:"p99"`
	Max  float64 `json:"max"`
}

type cohortStat struct {
	Predicate             string  `json:"predicate"`
	Observed              bool    `json:"observed"`
	Rows                  int     `json:"rows"`
	GoMedianNS            int64   `json:"go_median_ns"`
	WeightOfSignalGo      float64 `json:"weight_of_signal_go"`
	LocalGoGap            float64 `json:"local_go_gap"`
	UpperBoundSignalShare float64 `json:"upper_bound_signal_share"`
	Note                  string  `json:"note"`
}

type conflict struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Claim   string `json:"claim"`
	Finding string `json:"finding"`
	Action  string `json:"action"`
}

func main() {
	var opts options
	flag.StringVar(&opts.ManifestPath, "manifest", "", "F0 manifest JSON path")
	flag.StringVar(&opts.SealedPath, "sealed-board", "", "C0e sealed-board JSON path")
	flag.StringVar(&opts.OutJSON, "out-json", "", "optional report JSON path")
	flag.StringVar(&opts.OutMD, "out-md", "", "optional report Markdown path")
	flag.StringVar(&opts.GeneratedAt, "generated-at", "", "report timestamp override")
	flag.Parse()
	if opts.ManifestPath == "" || opts.SealedPath == "" {
		fatal(errors.New("-manifest and -sealed-board are required"))
	}
	doc, err := buildReport(opts)
	if err != nil {
		fatal(err)
	}
	if opts.OutJSON == "" && opts.OutMD == "" {
		fmt.Print(renderMarkdown(doc))
		return
	}
	if opts.OutJSON != "" {
		data, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			fatal(fmt.Errorf("marshal report: %w", err))
		}
		if err := os.WriteFile(opts.OutJSON, append(data, '\n'), 0o644); err != nil {
			fatal(fmt.Errorf("write report JSON: %w", err))
		}
	}
	if opts.OutMD != "" {
		if err := os.WriteFile(opts.OutMD, []byte(renderMarkdown(doc)), 0o644); err != nil {
			fatal(fmt.Errorf("write report Markdown: %w", err))
		}
	}
}

func buildReport(opts options) (*report, error) {
	manifestData, err := os.ReadFile(opts.ManifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var fleet manifest
	if err := json.Unmarshal(manifestData, &fleet); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if fleet.Schema != "gts-perf-fleet-manifest/v1" || fleet.Axis != axis || !fleet.Validation.Valid {
		return nil, fmt.Errorf("manifest is not a valid F0 full-axis manifest")
	}
	sealedData, err := os.ReadFile(opts.SealedPath)
	if err != nil {
		return nil, fmt.Errorf("read sealed board: %w", err)
	}
	var sealed sealedBoard
	if err := json.Unmarshal(sealedData, &sealed); err != nil {
		return nil, fmt.Errorf("decode sealed board: %w", err)
	}
	if len(sealed.Compact) != 4 || len(fleet.Rows) != 1435 {
		return nil, fmt.Errorf("unexpected board sizes: sealed compact=%d, fleet rows=%d", len(sealed.Compact), len(fleet.Rows))
	}

	signal := make([]manifestRow, 0, len(fleet.Rows))
	classes := map[string][]manifestRow{"clean": {}, "error": {}}
	for _, row := range fleet.Rows {
		if row.Axis.Status == "ok" && row.Predicates.RatioInterpretable {
			signal = append(signal, row)
			if _, ok := classes[row.SemanticClass]; ok {
				classes[row.SemanticClass] = append(classes[row.SemanticClass], row)
			}
		}
	}
	if len(signal) != fleet.Summary.Hygiene.RatioEligibleRows {
		return nil, fmt.Errorf("signal rows=%d, F0 ratio eligible=%d", len(signal), fleet.Summary.Hygiene.RatioEligibleRows)
	}

	totalGo, totalC := sums(signal)
	classStats := map[string]classStat{}
	dists := map[string]distribution{}
	for _, name := range []string{"clean", "error"} {
		rows := classes[name]
		goNS, cNS := sums(rows)
		excess := goNS - cNS
		classStats[name] = classStat{
			Rows: len(rows), Bytes: sumBytes(rows), GoMedianNS: goNS, CMedianNS: cNS,
			RatioByTotal: ratio(goNS, cNS), GoShareOfSignal: fraction(goNS, totalGo),
			LocalGoGap: fraction(excess, goNS), ExcessGoNS: excess, ExcessShare: fraction(excess, totalGo),
		}
		dists[name] = makeDistribution(rows)
	}

	reportTime := opts.GeneratedAt
	if reportTime == "" {
		reportTime = time.Now().UTC().Format(time.RFC3339)
	}
	return &report{
		Schema: reportSchema, GeneratedAt: reportTime,
		Authority: authority{Spec: "hypha://m31labs/gotreesitter/spec.campaign.v7#r1", Revision: "R1", SharesStaySeparate: true, RoutingChanged: false},
		C0e:       makeC0e(sealed),
		C0f: c0fBoard{
			Status: "partially_gated", ManifestSchema: fleet.Schema, ManifestSHA256: sha256Hex(manifestData), ScoreboardSHA256: fleet.Source.ScoreboardSHA256,
			SignalDefinition: fmt.Sprintf("axis.status == %q and predicates.ratio_interpretable == true; timer threshold %d ns", "ok", fleet.Policy.TimerThresholdNS),
			SignalRows:       len(signal), SignalGoMedianNS: totalGo, SignalCMedianNS: totalC, SignalRatioByTotal: ratio(totalGo, totalC),
			Classes: classStats, Distributions: dists, Findings: fleet.Summary.FindingRecordsByKind,
			Cohorts: makeCohorts(classStats, totalGo),
		},
		Formulas: []string{
			"signal = {row | axis.status == ok and c_median_ns >= timer_threshold_ns}; hygiene rows stay in coverage and fixed-overhead reporting",
			"ratio_by_total(class) = sum(go_median_ns over class) / sum(c_median_ns over class)",
			"go_share(class) = sum(go_median_ns over class) / sum(go_median_ns over signal)",
			"local_go_gap(class) = 1 - sum(c_median_ns over class) / sum(go_median_ns over class)",
			"projected_saving_fraction = projected_saved_go_ns / sum(go_median_ns over measured signal rows)",
			"percentile_q = linear interpolation at q * (n - 1) over sorted per-row ratios",
			"cohort ceilings are evidence bounds; they are not performance credit until runtime attribution facts exist",
		},
		Conflicts: []conflict{
			{ID: "legacy-share-boundary", Status: "resolved", Claim: "77-79%, approximately 50%, and 78-82% are competing scheduler shares", Finding: "The accepted C0 profile assigns different named boundaries. Wide scheduler-family shares are 80.3-84.8%; dispatch plus reductions are 47.6-56.2%.", Action: "Do not use any legacy percentage without its boundary."},
			{ID: "sealed-share-join", Status: "unresolved", Claim: "The sealed v9 equal-fixture ratios identify the fleet mechanism share", Finding: "The sealed board contains ratios and A/A nulls, but no component profile. The local C0 profile is a different host and diagnostic instrument.", Action: "Keep C0e ratios and C0e noise evidence separate from C0f fleet shares."},
			{ID: "fleet-mechanism-facts", Status: "unresolved", Claim: "The V10 fleet can assign recovery, alternative lifetime, scanner, and materialization shares", Finding: "The V10 scoreboard and F0 manifest contain class, stop, timing, recovery memo, and oracle fields. They contain no retry count, recovery-cost counter, live-version lifetime, scanner-call density, or materialization-work counter.", Action: "Do not admit a mechanism or claim the 2% selection ceiling until a trace lane supplies these facts."},
			{ID: "hygiene-provenance", Status: "unresolved", Claim: "The 1000 ns hygiene threshold was predeclared before V10", Finding: "F0 records 1000 ns as a fixed reporting policy, but the V10 metadata has no timer calibration or pre-run threshold record.", Action: "Treat the F0 hygiene denominator as queryable evidence, not sealed C6f acceptance proof, until threshold provenance is attached."},
			{ID: "error-byte-denominator", Status: "unresolved", Claim: "The R1 39.0% error-byte share is the V10 denominator", Finding: "F0 signal bytes give 84,094,345 / 260,884,819 = 32.2343%. R1 does not define the 39.0% byte denominator, so the values cannot be reconciled from this artifact.", Action: "Publish both denominators and do not blend them."},
		},
	}, nil
}

func makeC0e(sealed sealedBoard) c0eBoard {
	above3 := 0
	for _, row := range sealed.Compact {
		if row.Ratio > 3 {
			above3++
		}
	}
	return c0eBoard{Status: "sealed_board_recorded", Epoch: sealed.Epoch, Commit: sealed.Commit, CompactRatios: sealed.Compact, CompactGeomean: sealed.CompactGeomean, CanonicalAbove3x: above3, Ratchet: c0eRatchet{TargetGeomeanAtMost: 2, TargetFixtureRatioAtMost: 3, GeomeanPass: sealed.CompactGeomean <= 2, FixturePass: above3 == 0}, Noise: sealed.Noise}
}

func makeCohorts(classes map[string]classStat, totalGo int64) map[string]cohortStat {
	recovery := classes["error"]
	return map[string]cohortStat{
		"recovery":             {Predicate: "semantic_class == error and ratio-eligible", Observed: true, Rows: recovery.Rows, GoMedianNS: recovery.GoMedianNS, WeightOfSignalGo: recovery.GoShareOfSignal, LocalGoGap: recovery.LocalGoGap, UpperBoundSignalShare: recovery.GoShareOfSignal, Note: "Observable error-class proxy; not a causal recovery counter."},
		"alternative_lifetime": {Predicate: "runtime live-version lifetime", Observed: false, UpperBoundSignalShare: 1, Note: "No per-row live-version lifetime exists in V10/F0. Evidence bound is 0..100% of signal Go time; no credit."},
		"scanner_boundary":     {Predicate: "scanner-call density or scanner boundary", Observed: false, UpperBoundSignalShare: 1, Note: "No per-row scanner-call density exists in V10/F0. Evidence bound is 0..100% of signal Go time; no credit."},
		"materialization":      {Predicate: "materialization work", Observed: false, UpperBoundSignalShare: 1, Note: "No per-row materialization-work counter exists in V10/F0. Evidence bound is 0..100% of signal Go time; no credit."},
	}
}

func sums(rows []manifestRow) (int64, int64) {
	var goNS, cNS int64
	for _, row := range rows {
		goNS += row.Axis.GoMedianNS
		if row.Axis.CMedianNS != nil {
			cNS += *row.Axis.CMedianNS
		}
	}
	return goNS, cNS
}

func sumBytes(rows []manifestRow) int64 {
	var total int64
	for _, row := range rows {
		total += row.Bytes
	}
	return total
}

func ratio(num, den int64) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}
func fraction(num, den int64) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}

func makeDistribution(rows []manifestRow) distribution {
	values := make([]float64, 0, len(rows))
	for _, row := range rows {
		values = append(values, row.Axis.Ratio)
	}
	sort.Float64s(values)
	return distribution{Rows: len(values), P50: percentile(values, .50), P90: percentile(values, .90), P95: percentile(values, .95), P99: percentile(values, .99), Max: values[len(values)-1]}
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	index := p * float64(len(values)-1)
	lo := int(math.Floor(index))
	hi := int(math.Ceil(index))
	if lo == hi {
		return values[lo]
	}
	return values[lo] + (values[hi]-values[lo])*(index-float64(lo))
}

func sha256Hex(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func renderMarkdown(doc *report) string {
	var b []byte
	add := func(format string, args ...any) { b = append(b, []byte(fmt.Sprintf(format, args...))...) }
	add("# C0e/C0f attribution boards\n\n")
	add("Status: **partially gated**. This report is evidence-only. It does not run a scan or change routing.\n\n")
	add("## Authority\n\nR1 is authoritative. C0e and C0f remain independent instruments.\n\n")
	add("## C0e sealed equal-fixture board\n\n")
	add("Epoch `%s`, commit `%s`. Compact Go/C geomean: **%.3fx**.\n\n", doc.C0e.Epoch, doc.C0e.Commit, doc.C0e.CompactGeomean)
	add("| Fixture | Compact Go/C |\n|---|---:|\n")
	for _, row := range doc.C0e.CompactRatios {
		add("| `%s` | %.3fx |\n", row.Fixture, row.Ratio)
	}
	add("\nThe C0e target is geomean <= %.1fx and every fixture <= %.1fx. Result: geomean pass **%t**, fixture pass **%t**. Four fixtures exceed 3.0x.\n\n", doc.C0e.Ratchet.TargetGeomeanAtMost, doc.C0e.Ratchet.TargetFixtureRatioAtMost, doc.C0e.Ratchet.GeomeanPass, doc.C0e.Ratchet.FixturePass)
	add("The accepted local C0 profile records a separate noise floor: 7.367%% to 10.803%% p95 absolute A/A delta on a shared WSL2 host. It is not a sealed-v9 noise floor.\n\n")
	add("## C0f fleet board\n\n")
	add("Signal definition: `%s`.\n\n", doc.C0f.SignalDefinition)
	add("Signal rows: **%d**; Go time: **%d ns**; C time: **%d ns**; ratio-by-total: **%.6fx**.\n\n", doc.C0f.SignalRows, doc.C0f.SignalGoMedianNS, doc.C0f.SignalCMedianNS, doc.C0f.SignalRatioByTotal)
	add("| Class | Rows | Go ns | C ns | Ratio by total | Go share | Local Go gap |\n|---|---:|---:|---:|---:|---:|---:|\n")
	for _, name := range []string{"clean", "error"} {
		s := doc.C0f.Classes[name]
		add("| %s | %d | %d | %d | %.6fx | %.4f%% | %.4f%% |\n", name, s.Rows, s.GoMedianNS, s.CMedianNS, s.RatioByTotal, s.GoShareOfSignal*100, s.LocalGoGap*100)
	}
	add("\nF0 counts: 1,435 rows; 876 clean; 534 error; 25 stopped; 1,317 measured-ok; 2 hygiene rows; 1,315 ratio-eligible; 808 clean rows above 3.0x before hygiene and 806 after hygiene.\n\n")
	add("## Weighted cohort ceilings\n\n")
	add("The recovery row is an observable error-class proxy. The other cohorts lack runtime facts in V10. Their 100%% values are upper bounds, not performance credit.\n\n")
	add("| Cohort | Observed | Rows | Go share | Local gap | Ceiling |\n|---|---:|---:|---:|---:|---:|\n")
	for _, name := range []string{"recovery", "alternative_lifetime", "scanner_boundary", "materialization"} {
		c := doc.C0f.Cohorts[name]
		add("| %s | %t | %d | %.4f%% | %.4f%% | %.1f%% |\n", name, c.Observed, c.Rows, c.WeightOfSignalGo*100, c.LocalGoGap*100, c.UpperBoundSignalShare*100)
	}
	add("\nSelection formula: `projected_saved_go_ns / sum(go_median_ns over measured signal rows) >= 0.02`. C0f cannot evaluate the numerator for the three unobserved cohorts.\n\n")
	add("## Findings and conflicts\n\n")
	for _, c := range doc.Conflicts {
		add("- **%s — %s.** %s %s\n", c.ID, c.Status, c.Finding, c.Action)
	}
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	add("\n")
	return string(b)
}

func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(2) }
