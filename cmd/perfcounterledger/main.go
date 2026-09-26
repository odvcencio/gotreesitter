// Command perfcounterledger records and checks deterministic parser work.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	ts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

const schema = "benchfixture-counter-ledger-v1"

var syntheticFallback = map[string][]byte{
	"csv":     []byte("name,value\nexample,1\n"),
	"enforce": []byte("class Example { int value; }\n"),
}

type corpusManifest struct {
	Entries []struct {
		Language      string `json:"language"`
		Role          string `json:"role"`
		CommittedPath string `json:"committed_path"`
		SHA256        string `json:"sha256"`
		SessionSHA256 string `json:"session_sha256"`
	} `json:"entries"`
}

type counters struct {
	Tokens                    uint64 `json:"tokens"`
	NewNodes                  uint64 `json:"new_nodes"`
	MaxLiveVersions           uint64 `json:"max_live_versions"`
	MultiVersionTokenSharePPM uint64 `json:"multi_version_token_share_ppm"`
	ReusedBytes               uint64 `json:"reused_bytes"`
	BlockSplices              uint64 `json:"block_splices"`
	StopReason                string `json:"stop_reason"`
	RootEnd                   uint32 `json:"root_end"`
	InputBytes                int    `json:"input_bytes"`
	HasError                  bool   `json:"has_error"`
}

type row struct {
	Language      string    `json:"language"`
	Fixture       string    `json:"fixture"`
	FixtureSHA256 string    `json:"fixture_sha256"`
	SessionSHA256 string    `json:"session_sha256"`
	Route         string    `json:"route"`
	DeclineReason string    `json:"decline_reason,omitempty"`
	Full          *counters `json:"full,omitempty"`
	Edits         []editRow `json:"edits,omitempty"`
}

type editRow struct {
	Step          int      `json:"step"`
	Kind          string   `json:"kind"`
	Route         string   `json:"route"`
	DeclineReason string   `json:"decline_reason,omitempty"`
	Counters      counters `json:"counters"`
}

type ledger struct {
	Schema string `json:"schema"`
	Rows   []row  `json:"rows"`
}

func main() {
	write := flag.Bool("write", false, "write the reviewed ledger")
	output := flag.String("output", filepath.Join("internal", "benchfixtures", "counter_ledger.json"), "ledger JSON path")
	child := flag.String("child-route", "", "internal route process")
	langs := flag.String("langs", "all", "comma-separated diagnostic scope, or all")
	flag.Parse()
	if err := run(*write, *child, *langs, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(write bool, child, langs, output string) error {
	if child != "" {
		rows, err := collect(child, langs)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(rows)
	}
	if write && langs != "all" {
		return fmt.Errorf("--write requires all 206 languages")
	}
	got := ledger{Schema: schema, Rows: []row{}}
	for _, route := range []string{"default", "candidate"} {
		rows, err := collectSubprocess(route, langs)
		if err != nil {
			return err
		}
		got.Rows = append(got.Rows, rows...)
	}
	if langs == "all" && len(got.Rows) != 412 {
		return fmt.Errorf("collected %d route rows, want 412", len(got.Rows))
	}
	sort.Slice(got.Rows, func(i, j int) bool {
		if got.Rows[i].Language != got.Rows[j].Language {
			return got.Rows[i].Language < got.Rows[j].Language
		}
		return got.Rows[i].Route < got.Rows[j].Route
	})
	data, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if langs != "all" {
		_, err = os.Stdout.Write(data)
		return err
	}
	if write {
		return os.WriteFile(output, data, 0o644)
	}
	baseline, err := os.ReadFile(output)
	if err != nil {
		return err
	}
	var want ledger
	if err := json.Unmarshal(baseline, &want); err != nil {
		return err
	}
	if err := compare(want, got); err != nil {
		return err
	}
	fmt.Printf("counter ledger passed: %d rows, %d languages\n", len(got.Rows), len(got.Rows)/2)
	return nil
}

func collectSubprocess(route, langs string) ([]row, error) {
	binary, err := os.Executable()
	if err != nil {
		return nil, err
	}
	names := []string{langs}
	if langs == "all" {
		names = names[:0]
		for _, entry := range grammars.AllLanguages() {
			names = append(names, entry.Name)
		}
		sort.Strings(names)
	}
	mode := "0"
	if route == "candidate" {
		mode = "1"
	}
	rows := []row{}
	for _, name := range names {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		cmd := exec.CommandContext(ctx, binary, "--child-route", route, "--langs", name)
		cmd.Env = append(os.Environ(), "GTS_ADMISSION_CANDIDATE="+mode, "GOWORK=off")
		output, err := cmd.Output()
		cancel()
		if err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				return nil, fmt.Errorf("%s %s collector: %v: %s", name, route, err, strings.TrimSpace(string(exit.Stderr)))
			}
			return nil, fmt.Errorf("%s %s collector: %w", name, route, err)
		}
		var collected []row
		if err := json.Unmarshal(output, &collected); err != nil {
			return nil, err
		}
		rows = append(rows, collected...)
	}
	return rows, nil
}

func collect(route, scope string) ([]row, error) {
	if route != "default" && route != "candidate" {
		return nil, fmt.Errorf("unknown route %q", route)
	}
	if (route == "candidate") != (os.Getenv("GTS_ADMISSION_CANDIDATE") == "1") {
		return nil, fmt.Errorf("%s route has wrong GTS_ADMISSION_CANDIDATE value", route)
	}
	data, err := os.ReadFile(filepath.Join("internal", "benchfixtures", "real_corpus.json"))
	if err != nil {
		return nil, err
	}
	var manifest corpusManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	fixtures := map[string]struct{ path, sha, session string }{}
	for _, entry := range manifest.Entries {
		if entry.Role == "sample" {
			fixtures[entry.Language] = struct{ path, sha, session string }{entry.CommittedPath, entry.SHA256, entry.SessionSHA256}
		}
	}
	selected := map[string]bool{}
	if scope != "all" {
		for _, name := range strings.Split(scope, ",") {
			selected[strings.TrimSpace(name)] = true
		}
	}
	entries := grammars.AllLanguages()
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	rows := make([]row, 0, len(entries))
	for _, entry := range entries {
		if scope != "all" && !selected[entry.Name] {
			continue
		}
		fixture, ok := fixtures[entry.Name]
		if !ok {
			return nil, fmt.Errorf("%s has no corpus manifest sample", entry.Name)
		}
		var source []byte
		fixtureName := "real_sample"
		if fixture.path == "" {
			fixtureName = "synthetic_fallback"
			source = syntheticFallback[entry.Name]
			if len(source) == 0 {
				return nil, fmt.Errorf("%s has no committed or synthetic sample", entry.Name)
			}
		} else {
			source, err = os.ReadFile(filepath.Join("internal", "benchfixtures", fixture.path))
			if err != nil {
				return nil, err
			}
			if digest(source) != fixture.sha {
				return nil, fmt.Errorf("%s fixture SHA-256 changed", entry.Name)
			}
			if benchfixtures.EditingSessionSHA256(source) != fixture.session {
				return nil, fmt.Errorf("%s editing session SHA-256 changed", entry.Name)
			}
		}
		language := entry.Language()
		if language == nil {
			return nil, fmt.Errorf("%s grammar unavailable", entry.Name)
		}
		item, err := collectOne(route, entry.Name, fixtureName, source, language)
		if err != nil {
			return nil, err
		}
		rows = append(rows, item)
	}
	if scope == "all" && len(rows) != 206 {
		return nil, fmt.Errorf("collected %d languages, want 206", len(rows))
	}
	return rows, nil
}

func collectOne(route, name, fixture string, source []byte, language *ts.Language) (row, error) {
	item := row{Language: name, Fixture: fixture, FixtureSHA256: digest(source),
		SessionSHA256: benchfixtures.EditingSessionSHA256(source), Route: route}
	parser := ts.NewParser(language)
	if route == "candidate" {
		parser.SetCompactCertificationTelemetry(true)
	}
	ts.ResetAdmissionCandidateCounters()
	old, err := parser.Parse(source)
	if err != nil {
		return item, fmt.Errorf("%s %s full parse: %w", name, route, err)
	}
	defer old.Release()
	if route == "candidate" {
		routed, fallbacks := ts.AdmissionCandidateCounters()
		if fallbacks > 0 || routed == 0 {
			item.DeclineReason = ts.AdmissionCandidateLastFallbackReason()
			if item.DeclineReason == "" {
				item.DeclineReason = "candidate_not_routed"
			}
			return item, nil
		}
	}
	runtime := old.ParseRuntime()
	maxVersions := uint64(max(0, runtime.MaxStacksSeen))
	multiTokens := runtime.MultiStackTokens
	if route == "candidate" {
		maxVersions = runtime.CompactPeakHeaders
		multiTokens = uint64(runtime.CompactMultiHeaderTokens)
	}
	item.Full = &counters{Tokens: runtime.TokensConsumed, NewNodes: uint64(max(0, runtime.NodesAllocated)),
		MaxLiveVersions: maxVersions, MultiVersionTokenSharePPM: share(multiTokens, runtime.TokensConsumed),
		StopReason: string(runtime.StopReason), RootEnd: old.RootNode().EndByte(), InputBytes: len(source), HasError: old.RootNode().HasError()}
	// The pull-request gate samples the first edit from the pinned session.
	step := benchfixtures.EditingSession(source)[0]
	old.Edit(step.Edit)
	inc, profile, err := parser.ParseIncrementalProfiled(step.Source, old)
	if err != nil {
		return item, fmt.Errorf("%s %s edit: %w", name, route, err)
	}
	defer inc.Release()
	editRuntime := inc.ParseRuntime()
	edit := editRow{Step: 1, Kind: "insert"}
	if route == "default" {
		edit.Route = "default"
	} else {
		switch {
		case editRuntime.CompactIncrementalReuseRoute:
			edit.Route = "compact_reuse"
		case editRuntime.CompactIncrementalFullRecoveryRoute:
			edit.Route = "compact_recovery"
		default:
			edit.Route = "legacy_fallback"
			edit.DeclineReason = editRuntime.CompactIncrementalFallbackReason
			if edit.DeclineReason == "" {
				edit.DeclineReason = "candidate_incremental_not_routed"
			}
		}
	}
	maxVersions = uint64(max(0, profile.MaxStacksSeen))
	multiTokens = profile.MultiStackTokens
	if edit.Route == "compact_reuse" || edit.Route == "compact_recovery" {
		maxVersions = editRuntime.CompactPeakHeaders
		multiTokens = uint64(editRuntime.CompactMultiHeaderTokens)
	}
	edit.Counters = counters{Tokens: profile.TokensConsumed, NewNodes: profile.NewNodesAllocated,
		MaxLiveVersions: maxVersions, MultiVersionTokenSharePPM: share(multiTokens, profile.TokensConsumed),
		ReusedBytes: profile.ReusedBytes, BlockSplices: profile.BlockSpliceSteps,
		StopReason: string(profile.StopReason), RootEnd: inc.RootNode().EndByte(), InputBytes: len(step.Source), HasError: inc.RootNode().HasError()}
	item.Edits = []editRow{edit}
	return item, nil
}

func digest(source []byte) string { return fmt.Sprintf("%x", sha256.Sum256(source)) }

func share(part, total uint64) uint64 {
	if total == 0 {
		return 0
	}
	return part * 1_000_000 / total
}

func compare(want, got ledger) error {
	if want.Schema != schema || got.Schema != schema || len(want.Rows) != len(got.Rows) {
		return fmt.Errorf("counter ledger schema or row count changed; regenerate and review the diff")
	}
	for i := range want.Rows {
		a, b := want.Rows[i], got.Rows[i]
		if a.Language != b.Language || a.Route != b.Route || a.Fixture != b.Fixture ||
			a.FixtureSHA256 != b.FixtureSHA256 || a.SessionSHA256 != b.SessionSHA256 {
			return fmt.Errorf("ledger row %d fixture identity changed", i)
		}
		if a.DeclineReason != b.DeclineReason {
			return fmt.Errorf("%s %s decline changed from %q to %q", a.Language, a.Route, a.DeclineReason, b.DeclineReason)
		}
		if (a.Full == nil) != (b.Full == nil) || len(a.Edits) != len(b.Edits) {
			return fmt.Errorf("%s %s phase availability changed", a.Language, a.Route)
		}
		if a.Full != nil {
			if err := compareCounters(a.Full, b.Full); err != nil {
				return fmt.Errorf("%s %s full: %w", a.Language, a.Route, err)
			}
		}
		for step := range a.Edits {
			before, after := a.Edits[step], b.Edits[step]
			if before.Step != after.Step || before.Kind != after.Kind ||
				before.Route != after.Route || before.DeclineReason != after.DeclineReason {
				return fmt.Errorf("%s %s edit %d route or operation changed", a.Language, a.Route, step+1)
			}
			if err := compareCounters(&before.Counters, &after.Counters); err != nil {
				return fmt.Errorf("%s %s edit %d: %w", a.Language, a.Route, step+1, err)
			}
		}
	}
	return nil
}

func compareCounters(old, now *counters) error {
	for _, metric := range []struct {
		name              string
		baseline, current uint64
	}{
		{"tokens", old.Tokens, now.Tokens}, {"new_nodes", old.NewNodes, now.NewNodes},
		{"max_live_versions", old.MaxLiveVersions, now.MaxLiveVersions},
	} {
		if metric.current*100 > metric.baseline*102 {
			return fmt.Errorf("%s rose from %d to %d (>2%%)", metric.name, metric.baseline, metric.current)
		}
	}
	for _, metric := range []struct {
		name              string
		baseline, current uint64
	}{
		{"reused_bytes", old.ReusedBytes, now.ReusedBytes}, {"block_splices", old.BlockSplices, now.BlockSplices},
	} {
		if metric.current*100 < metric.baseline*98 {
			return fmt.Errorf("%s fell from %d to %d (>2%%)", metric.name, metric.baseline, metric.current)
		}
	}
	if old.StopReason != now.StopReason {
		return fmt.Errorf("stop reason changed from %q to %q", old.StopReason, now.StopReason)
	}
	if old.RootEnd != now.RootEnd || old.InputBytes != now.InputBytes || old.HasError != now.HasError {
		return fmt.Errorf("parse completion or error state changed")
	}
	return nil
}
