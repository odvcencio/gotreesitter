package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// ---- gzip / hash helpers ----

func gunzipFile(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ---- Receipt model ----

type ReceiptComponentShare struct {
	Component   Component `json:"component"`
	NS          int64     `json:"ns"`
	SharePct    float64   `json:"share_pct"` // rounded to 1 decimal for display
	RawSharePct float64   `json:"raw_share_pct"`
}

type ReceiptRow struct {
	Label           string                  `json:"label"` // "diagnostic lane" | "shipped route"
	Fixture         string                  `json:"fixture"`
	SourceBytes     int                     `json:"source_bytes"`
	Iterations      int                     `json:"iterations"`
	WallNSPerOp     float64                 `json:"wall_ns_per_op"`
	ProfiledTotalNS int64                   `json:"profiled_total_ns"`
	ProfiledWallNS  int64                   `json:"profiled_wall_ns"`
	CoveragePct     float64                 `json:"profile_coverage_pct"` // profiled_total_ns / profiled_wall_ns
	Components      []ReceiptComponentShare `json:"components"`
	RoundedShareSum float64                 `json:"rounded_share_sum_pct"`
	WithinTolerance bool                    `json:"within_tolerance"`
	UnclassifiedTop []unclassifiedEntry     `json:"unclassified_top,omitempty"`
}

type ReceiptNoiseFloorRow struct {
	Fixture        string  `json:"fixture"`
	Pairs          int     `json:"pairs"`
	MedianNSPerOp  float64 `json:"median_ns_per_op"`
	P95AbsDeltaNS  float64 `json:"p95_abs_delta_ns"`
	P95PctOfMedian float64 `json:"p95_pct_of_median"`
}

type ReceiptCostPerEventClass struct {
	Class     string  `json:"class"`
	Count     uint64  `json:"count"`
	NSPerUnit float64 `json:"ns_per_event"`
}

type ReceiptCostPerEventRow struct {
	Fixture     string                     `json:"fixture"`
	WallNSPerOp float64                    `json:"wall_ns_per_op"`
	Classes     []ReceiptCostPerEventClass `json:"classes"`
}

type Receipt struct {
	SchemaVersion    string                   `json:"schema_version"`
	GeneratedUTC     string                   `json:"generated_utc"`
	GitCommit        string                   `json:"git_commit"`
	HostClass        string                   `json:"host_class"`
	ToleranceNote    string                   `json:"tolerance_note"`
	AttributionRows  []ReceiptRow             `json:"attribution_rows"`
	NoiseFloor       []ReceiptNoiseFloorRow   `json:"noise_floor"`
	NoiseFloorMethod string                   `json:"noise_floor_method"`
	CostPerEvent     []ReceiptCostPerEventRow `json:"cost_per_event"`
	CostPerEventNote string                   `json:"cost_per_event_note"`
}

const attributionTolerancePP = 0.2 // percentage points; see the perf-attribution record (Hyphae space m31labs/gotreesitter).

func buildReceipt(cfg config, capture *captureManifest, workcounts map[string]workCountResult, noise []noiseSample) (*Receipt, error) {
	receipt := &Receipt{
		SchemaVersion: "gts-perf-attribution-receipt/v1",
		GeneratedUTC:  time.Now().UTC().Format(time.RFC3339),
		GitCommit:     gitHead(cfg.repoRoot),
		HostClass:     "shared WSL2 development host, background load uncontrolled (local-development floor, not quiet-host or enclave)",
		ToleranceNote: fmt.Sprintf("component shares are computed to sum to exactly 100%% of attributed samples by construction (ComponentOther absorbs the remainder); the displayed 1-decimal rows are checked to sum to 100%% within +/-%.1f percentage points of rounding tolerance", attributionTolerancePP),
	}

	for _, row := range capture.Rows {
		profile, err := decodePProfCPUProfile(row.ProfilePath)
		if err != nil {
			return nil, fmt.Errorf("%s %s: decode profile: %w", row.Label, row.Fixture, err)
		}
		attribution := attributeProfile(profile)
		receiptRow := ReceiptRow{
			Label: row.Label, Fixture: row.Fixture, SourceBytes: row.SourceBytes,
			Iterations: row.Iterations, WallNSPerOp: row.NSPerOp,
			ProfiledTotalNS: attribution.TotalSampleNS, ProfiledWallNS: row.ElapsedNS,
		}
		if row.ElapsedNS > 0 {
			receiptRow.CoveragePct = 100 * float64(attribution.TotalSampleNS) / float64(row.ElapsedNS)
		}
		var roundedSum float64
		for _, component := range ComponentOrder {
			ns := attribution.ComponentNS[component]
			var raw float64
			if attribution.TotalSampleNS > 0 {
				raw = 100 * float64(ns) / float64(attribution.TotalSampleNS)
			}
			rounded := math.Round(raw*10) / 10
			roundedSum += rounded
			receiptRow.Components = append(receiptRow.Components, ReceiptComponentShare{
				Component: component, NS: ns, SharePct: rounded, RawSharePct: raw,
			})
		}
		receiptRow.RoundedShareSum = math.Round(roundedSum*10) / 10
		// A tiny epsilon absorbs float64 representation noise (e.g. 99.8 is not
		// exactly representable), so a sum that is exactly at the stated
		// tolerance boundary is not spuriously marked as a miss.
		const toleranceEpsilon = 1e-9
		receiptRow.WithinTolerance = math.Abs(receiptRow.RoundedShareSum-100) <= attributionTolerancePP+toleranceEpsilon
		if len(attribution.UnclassifiedTop) > 0 {
			top := attribution.UnclassifiedTop
			if len(top) > 8 {
				top = top[:8]
			}
			receiptRow.UnclassifiedTop = top
		}
		receipt.AttributionRows = append(receipt.AttributionRows, receiptRow)
	}

	receipt.NoiseFloorMethod = "BenchmarkParserCoreFreshFullCanonical, identical prebuilt binary invoked twice per pair (series A and series B), interleaved A,B,A,B,...; GOMAXPROCS=1; delta_i = |A_i - B_i| per fixture per pair; p95 by linear interpolation over sorted deltas"
	receipt.NoiseFloor = buildNoiseFloorRows(noise)

	receipt.CostPerEventNote = "average attribution, not a causal model: wall ns/op for the diagnostic lane divided by the gts_workcount event count for the same fixture and lifecycle"
	receipt.CostPerEvent = buildCostPerEventRows(capture, workcounts)

	return receipt, nil
}

func buildNoiseFloorRows(samples []noiseSample) []ReceiptNoiseFloorRow {
	byFixture := map[string]map[int]map[string]float64{} // fixture -> pairIndex -> series -> ns
	// We do not have the pair index directly on noiseSample (by design it is
	// order-independent); reconstruct pairing by stable per-fixture, per-series
	// arrival order instead (A_1..A_n paired positionally with B_1..B_n).
	seriesValues := map[string]map[string][]float64{} // fixture -> series -> values in arrival order
	for _, s := range samples {
		if seriesValues[s.Fixture] == nil {
			seriesValues[s.Fixture] = map[string][]float64{}
		}
		seriesValues[s.Fixture][s.Series] = append(seriesValues[s.Fixture][s.Series], s.NSPerOp)
	}
	_ = byFixture

	var rows []ReceiptNoiseFloorRow
	fixtures := make([]string, 0, len(seriesValues))
	for fixture := range seriesValues {
		fixtures = append(fixtures, fixture)
	}
	sort.Strings(fixtures)
	for _, fixture := range fixtures {
		aValues := seriesValues[fixture]["A"]
		bValues := seriesValues[fixture]["B"]
		n := len(aValues)
		if len(bValues) < n {
			n = len(bValues)
		}
		var deltas []float64
		var all []float64
		for i := 0; i < n; i++ {
			deltas = append(deltas, math.Abs(aValues[i]-bValues[i]))
			all = append(all, aValues[i], bValues[i])
		}
		sort.Float64s(deltas)
		sort.Float64s(all)
		median := percentile(all, 50)
		p95 := percentile(deltas, 95)
		row := ReceiptNoiseFloorRow{
			Fixture: fixture, Pairs: n, MedianNSPerOp: median, P95AbsDeltaNS: p95,
		}
		if median > 0 {
			row.P95PctOfMedian = 100 * p95 / median
		}
		rows = append(rows, row)
	}
	return rows
}

func percentile(sortedAsc []float64, p float64) float64 {
	n := len(sortedAsc)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sortedAsc[0]
	}
	rank := p / 100 * float64(n-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return sortedAsc[lo]
	}
	frac := rank - float64(lo)
	return sortedAsc[lo] + (sortedAsc[hi]-sortedAsc[lo])*frac
}

func buildCostPerEventRows(capture *captureManifest, workcounts map[string]workCountResult) []ReceiptCostPerEventRow {
	wallByFixture := map[string]float64{}
	for _, row := range capture.Rows {
		if row.Label == "diagnostic lane" {
			wallByFixture[row.Fixture] = row.NSPerOp
		}
	}
	var rows []ReceiptCostPerEventRow
	for _, spec := range fixtureSpecs {
		wall, ok := wallByFixture[spec.ID]
		if !ok {
			continue
		}
		wc, ok := workcounts[spec.ID]
		if !ok {
			continue
		}
		row := ReceiptCostPerEventRow{Fixture: spec.ID, WallNSPerOp: wall}
		classes := []struct {
			name  string
			count uint64
		}{
			{"shifts", wc.CoreWork.Shifts},
			{"reductions", wc.CoreWork.Reductions},
			{"emitted_pop_paths", wc.CoreWork.EmittedPopPaths},
			{"emitted_pop_payloads", wc.CoreWork.EmittedPopPayloads},
			{"canonicalizations", wc.SchedulerWork.Canonicalizations},
			{"elections", wc.SchedulerWork.Elections},
			{"action_lookups", wc.SchedulerWork.ActionLookups},
		}
		for _, c := range classes {
			var nsPer float64
			if c.count > 0 {
				nsPer = wall / float64(c.count)
			}
			row.Classes = append(row.Classes, ReceiptCostPerEventClass{Class: c.name, Count: c.count, NSPerUnit: nsPer})
		}
		rows = append(rows, row)
	}
	return rows
}

func gitHead(repoRoot string) string {
	cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// ---- Markdown rendering ----

func renderMarkdown(r *Receipt) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Generated %s, git %s.\n\n", r.GeneratedUTC, r.GitCommit)
	fmt.Fprintf(&b, "Host class: %s.\n\n", r.HostClass)
	fmt.Fprintf(&b, "%s.\n\n", r.ToleranceNote)

	fmt.Fprintln(&b, "### Attribution shares")
	fmt.Fprintln(&b)
	header := []string{"lane", "fixture", "wall ns/op", "coverage %"}
	for _, c := range ComponentOrder {
		header = append(header, string(c))
	}
	header = append(header, "sum %")
	fmt.Fprintln(&b, "| "+strings.Join(header, " | ")+" |")
	fmt.Fprintln(&b, "|"+strings.Repeat("---|", len(header)))
	for _, row := range r.AttributionRows {
		cells := []string{
			row.Label, row.Fixture,
			formatNS(row.WallNSPerOp),
			fmt.Sprintf("%.1f", row.CoveragePct),
		}
		for _, comp := range row.Components {
			cells = append(cells, fmt.Sprintf("%.1f", comp.SharePct))
		}
		cells = append(cells, fmt.Sprintf("%.1f%s", row.RoundedShareSum, toleranceMark(row.WithinTolerance)))
		fmt.Fprintln(&b, "| "+strings.Join(cells, " | ")+" |")
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "### Noise floor (interleaved A/A, identical binary)")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| fixture | pairs | median ns/op | p95 \\|delta\\| ns | p95 \\|delta\\| % of median |")
	fmt.Fprintln(&b, "|---|---|---|---|---|")
	for _, row := range r.NoiseFloor {
		fmt.Fprintf(&b, "| %s | %d | %s | %s | %.2f%% |\n",
			row.Fixture, row.Pairs, formatNS(row.MedianNSPerOp), formatNS(row.P95AbsDeltaNS), row.P95PctOfMedian)
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "### Cost per event (diagnostic lane; average attribution, not causal)")
	fmt.Fprintln(&b)
	if len(r.CostPerEvent) > 0 {
		classNames := make([]string, 0, len(r.CostPerEvent[0].Classes))
		for _, c := range r.CostPerEvent[0].Classes {
			classNames = append(classNames, c.Class)
		}
		header := append([]string{"fixture", "wall ns/op"}, classNames...)
		fmt.Fprintln(&b, "| "+strings.Join(header, " | ")+" |")
		fmt.Fprintln(&b, "|"+strings.Repeat("---|", len(header)))
		for _, row := range r.CostPerEvent {
			cells := []string{row.Fixture, formatNS(row.WallNSPerOp)}
			for _, c := range row.Classes {
				cells = append(cells, fmt.Sprintf("%.1f (n=%d)", c.NSPerUnit, c.Count))
			}
			fmt.Fprintln(&b, "| "+strings.Join(cells, " | ")+" |")
		}
	}
	return b.String()
}

func toleranceMark(within bool) string {
	if within {
		return ""
	}
	return " (!)"
}

func formatNS(ns float64) string {
	switch {
	case ns >= 1e9:
		return fmt.Sprintf("%.3fs", ns/1e9)
	case ns >= 1e6:
		return fmt.Sprintf("%.3fms", ns/1e6)
	case ns >= 1e3:
		return fmt.Sprintf("%.3fus", ns/1e3)
	default:
		return fmt.Sprintf("%.1fns", ns)
	}
}
