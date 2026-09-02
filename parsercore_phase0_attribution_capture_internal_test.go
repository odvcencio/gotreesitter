//go:build gts_parsercorephase0

package gotreesitter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"
)

// This file is campaign v7 tranche C0 measurement infrastructure (attribution
// board). It changes no parser code, routing, or shipped behavior: it only
// captures CPU profiles of the existing compact lane for offline attribution.
// See the perf-attribution record (Hyphae space m31labs/gotreesitter) and cgo_harness/attribution for the one
// documented command that drives it end to end.

// attributionCaptureRow is one profiled sample series: either the diagnostic
// runner path or the shipped Parser.Parse route, each covering all four
// canonical fixtures (tranche B9 removed the 64 KiB admission floor that used
// to keep grammargen_lr shipped-route-ineligible).
type attributionCaptureRow struct {
	Label       string  `json:"label"`
	Fixture     string  `json:"fixture"`
	SourceBytes int     `json:"source_bytes"`
	Iterations  int     `json:"iterations"`
	ElapsedNS   int64   `json:"elapsed_ns"`
	NSPerOp     float64 `json:"ns_per_op"`
	ProfilePath string  `json:"profile_path"`
}

type attributionCaptureManifest struct {
	SchemaVersion string                  `json:"schema_version"`
	GeneratedUTC  string                  `json:"generated_utc"`
	DurationParam string                  `json:"duration_param"`
	Rows          []attributionCaptureRow `json:"rows"`
}

// TestParserCoreAttributionCapture is the campaign v7 C0 attribution capture
// endpoint. It is inert unless GTS_ATTRIBUTION_OUT names a writable directory,
// so it never adds cost to a normal `go test` run.
//
// Each captured CPU profile brackets exactly one pinned lifecycle, for all
// four canonical fixtures on both lanes (tranche B9 removed the 64 KiB
// admission floor that used to keep grammargen_lr shipped-route-ineligible):
//
//   - "diagnostic lane": runner.parse, shallow completeness validation, and
//     Tree.Release -- the same timed region BenchmarkParserCoreFreshFullCanonical
//     measures.
//   - "shipped route": Parser.Parse and Tree.Release -- the timed region
//     BenchmarkDiagnosticParserCoreWarmProductionParseQueryCompile measures.
//     Every sample is verified, by the admission-candidate routed/fallback
//     counters, to have actually taken the compact route rather than falling
//     back to production.
//
// Every one-time preflight, warm pass, and digest check happens outside
// pprof.StartCPUProfile/StopCPUProfile so the profile is not diluted by
// non-representative setup cost the pinned benchmarks themselves exclude.
func TestParserCoreAttributionCapture(t *testing.T) {
	outDir := strings.TrimSpace(os.Getenv("GTS_ATTRIBUTION_OUT"))
	if outDir == "" {
		t.Skip("parser-core attribution capture is not configured (set GTS_ATTRIBUTION_OUT)")
	}
	duration := 800 * time.Millisecond
	if raw := strings.TrimSpace(os.Getenv("GTS_ATTRIBUTION_DURATION")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			t.Fatalf("GTS_ATTRIBUTION_DURATION must be a positive Go duration, got %q", raw)
		}
		duration = parsed
	}
	const minIterations = 5

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("create attribution output dir: %v", err)
	}

	var rows []attributionCaptureRow
	for _, row := range diagnosticParserCoreCanonicalAdmissions {
		row := row
		fixture := loadDiagnosticParserCoreCanonicalFixture(t, row.id)
		requireDiagnosticParserCoreCanonicalFixtureIdentity(t, fixture, row)
		rows = append(rows, captureAttributionDiagnosticLane(t, outDir, fixture, row, duration, minIterations))
	}
	for _, row := range diagnosticParserCoreCanonicalAdmissions {
		row := row
		fixture := loadDiagnosticParserCoreCanonicalFixture(t, row.id)
		requireDiagnosticParserCoreCanonicalFixtureIdentity(t, fixture, row)
		rows = append(rows, captureAttributionShippedRoute(t, outDir, fixture, row, duration, minIterations))
	}

	manifest := attributionCaptureManifest{
		SchemaVersion: "gts-perf-attribution-capture/v1",
		GeneratedUTC:  time.Now().UTC().Format(time.RFC3339),
		DurationParam: duration.String(),
		Rows:          rows,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal capture manifest: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(outDir, "capture_manifest.json"), data, 0o644); err != nil {
		t.Fatalf("write capture manifest: %v", err)
	}
}

func captureAttributionDiagnosticLane(
	t *testing.T,
	outDir string,
	fixture DiagnosticParserCoreCanonicalFixtureForTest,
	row diagnosticParserCoreCanonicalAdmission,
	duration time.Duration,
	minIterations int,
) attributionCaptureRow {
	t.Helper()
	runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
	if err != nil {
		t.Fatalf("%s diagnostic lane: build runner: %v", row.id, err)
	}

	// Warm pass, outside profiling.
	warmTree, err := runner.parse(fixture.Source)
	if err != nil {
		t.Fatalf("%s diagnostic lane: warm parse: %v", row.id, err)
	}
	if err := validateParserCoreFreshFullCanonicalTree(warmTree, len(fixture.Source)); err != nil {
		warmTree.Release()
		t.Fatalf("%s diagnostic lane: warm parse: %v", row.id, err)
	}
	if work := runner.compact.Work(); work != row.work || work.Overflow {
		warmTree.Release()
		t.Fatalf("%s diagnostic lane: warm work=%+v want=%+v", row.id, work, row.work)
	}
	warmTree.Release()
	runtime.GC()
	DrainArenaPools()

	profilePath := filepath.Join(outDir, "diagnostic_"+row.id+".pprof")
	profileFile, err := os.Create(profilePath)
	if err != nil {
		t.Fatalf("%s diagnostic lane: create profile file: %v", row.id, err)
	}
	defer profileFile.Close()
	if err := pprof.StartCPUProfile(profileFile); err != nil {
		t.Fatalf("%s diagnostic lane: start cpu profile: %v", row.id, err)
	}
	start := time.Now()
	iterations := 0
	for iterations < minIterations || time.Since(start) < duration {
		tree, err := runner.parse(fixture.Source)
		if err != nil {
			pprof.StopCPUProfile()
			t.Fatalf("%s diagnostic lane: timed parse: %v", row.id, err)
		}
		if err := validateParserCoreFreshFullCanonicalTree(tree, len(fixture.Source)); err != nil {
			tree.Release()
			pprof.StopCPUProfile()
			t.Fatalf("%s diagnostic lane: timed parse: %v", row.id, err)
		}
		tree.Release()
		iterations++
	}
	elapsed := time.Since(start)
	pprof.StopCPUProfile()

	if work := runner.compact.Work(); work != row.work || work.Overflow {
		t.Fatalf("%s diagnostic lane: timed work=%+v want=%+v", row.id, work, row.work)
	}

	return attributionCaptureRow{
		Label: "diagnostic lane", Fixture: row.id, SourceBytes: len(fixture.Source),
		Iterations: iterations, ElapsedNS: elapsed.Nanoseconds(),
		NSPerOp:     float64(elapsed.Nanoseconds()) / float64(iterations),
		ProfilePath: profilePath,
	}
}

func captureAttributionShippedRoute(
	t *testing.T,
	outDir string,
	fixture DiagnosticParserCoreCanonicalFixtureForTest,
	row diagnosticParserCoreCanonicalAdmission,
	duration time.Duration,
	minIterations int,
) attributionCaptureRow {
	t.Helper()
	runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
	if err != nil {
		t.Fatalf("%s shipped route: build runner: %v", row.id, err)
	}
	parser := runner.parser
	parser.SetAdmissionCandidateRoute(true)
	defer parser.ClearAdmissionCandidateRoute()

	routedBefore, fallbackBefore := AdmissionCandidateCounters()
	warmTree, err := parser.Parse(fixture.Source)
	if err != nil {
		t.Fatalf("%s shipped route: warm parse: %v", row.id, err)
	}
	if err := validateParserCoreFreshFullCanonicalTree(warmTree, len(fixture.Source)); err != nil {
		warmTree.Release()
		t.Fatalf("%s shipped route: warm parse: %v", row.id, err)
	}
	warmTree.Release()
	routedAfterWarm, fallbackAfterWarm := AdmissionCandidateCounters()
	if routedAfterWarm != routedBefore+1 || fallbackAfterWarm != fallbackBefore {
		t.Fatalf("%s shipped route: warm parse did not take the compact route: routed %d->%d fallback %d->%d reason=%q",
			row.id, routedBefore, routedAfterWarm, fallbackBefore, fallbackAfterWarm, AdmissionCandidateLastFallbackReason())
	}
	runtime.GC()
	DrainArenaPools()

	profilePath := filepath.Join(outDir, "shipped_"+row.id+".pprof")
	profileFile, err := os.Create(profilePath)
	if err != nil {
		t.Fatalf("%s shipped route: create profile file: %v", row.id, err)
	}
	defer profileFile.Close()
	if err := pprof.StartCPUProfile(profileFile); err != nil {
		t.Fatalf("%s shipped route: start cpu profile: %v", row.id, err)
	}
	start := time.Now()
	iterations := 0
	for iterations < minIterations || time.Since(start) < duration {
		tree, err := parser.Parse(fixture.Source)
		if err != nil {
			pprof.StopCPUProfile()
			t.Fatalf("%s shipped route: timed parse: %v", row.id, err)
		}
		tree.Release()
		iterations++
	}
	elapsed := time.Since(start)
	pprof.StopCPUProfile()

	routedAfter, fallbackAfter := AdmissionCandidateCounters()
	if routedAfter != routedAfterWarm+uint64(iterations) || fallbackAfter != fallbackAfterWarm {
		t.Fatalf("%s shipped route: timed loop did not stay on the compact route: routed %d->%d (want +%d) fallback %d->%d reason=%q",
			row.id, routedAfterWarm, routedAfter, iterations, fallbackAfterWarm, fallbackAfter, AdmissionCandidateLastFallbackReason())
	}

	return attributionCaptureRow{
		Label: "shipped route", Fixture: row.id, SourceBytes: len(fixture.Source),
		Iterations: iterations, ElapsedNS: elapsed.Nanoseconds(),
		NSPerOp:     float64(elapsed.Nanoseconds()) / float64(iterations),
		ProfilePath: profilePath,
	}
}
