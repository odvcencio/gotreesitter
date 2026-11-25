package main

import (
	"strings"
	"testing"
)

func TestCounterRatchetTwoPercentBoundary(t *testing.T) {
	base := &counters{Tokens: 100, NewNodes: 100, MaxLiveVersions: 100,
		ReusedBytes: 100, BlockSplices: 100, StopReason: "accepted", RootEnd: 10, InputBytes: 10}
	atLimit := *base
	atLimit.Tokens = 102
	atLimit.NewNodes = 102
	atLimit.MaxLiveVersions = 102
	atLimit.ReusedBytes = 98
	atLimit.BlockSplices = 98
	if err := compareCounters(base, &atLimit); err != nil {
		t.Fatalf("exact two percent change failed: %v", err)
	}
	for _, tc := range []struct {
		name   string
		change func(*counters)
	}{
		{"tokens", func(c *counters) { c.Tokens = 103 }},
		{"new_nodes", func(c *counters) { c.NewNodes = 103 }},
		{"max_live_versions", func(c *counters) { c.MaxLiveVersions = 103 }},
		{"reused_bytes", func(c *counters) { c.ReusedBytes = 97 }},
		{"block_splices", func(c *counters) { c.BlockSplices = 97 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := *base
			tc.change(&current)
			if err := compareCounters(base, &current); err == nil || !strings.Contains(err.Error(), tc.name) {
				t.Fatalf("%s regression passed: %v", tc.name, err)
			}
		})
	}
}

func TestCounterRatchetRejectsNewWorkFromZero(t *testing.T) {
	if err := compareCounters(&counters{}, &counters{Tokens: 1}); err == nil {
		t.Fatal("new work from zero passed")
	}
}

func TestLedgerRejectsRouteDeclineOrFixtureChange(t *testing.T) {
	base := ledger{Schema: schema, Rows: []row{{Language: "go", Route: "candidate", Fixture: "real_sample",
		FixtureSHA256: "source", SessionSHA256: "session", DeclineReason: "no_action"}}}
	changed := base
	changed.Rows = append([]row(nil), base.Rows...)
	changed.Rows[0].DeclineReason = ""
	if err := compare(base, changed); err == nil {
		t.Fatal("route decline disappeared without regeneration")
	}
	changed.Rows[0] = base.Rows[0]
	changed.Rows[0].SessionSHA256 = "changed"
	if err := compare(base, changed); err == nil {
		t.Fatal("editing session changed without regeneration")
	}
	changed.Rows[0] = base.Rows[0]
	changed.Rows[0].Edits = []editRow{{Step: 1, Kind: "insert", Route: "legacy_fallback",
		DeclineReason: "candidate_incremental_not_routed"}}
	if err := compare(base, changed); err == nil {
		t.Fatal("edit route changed without regeneration")
	}
	base.Rows[0].Edits = []editRow{{Step: 1, Kind: "insert", Route: "default"}}
	changed.Rows[0].Edits = []editRow{{Step: 1, Kind: "insert", Route: "legacy_fallback"}}
	if err := compare(base, changed); err == nil {
		t.Fatal("edit fallback changed without regeneration")
	}
}

func ledgerWithFull(c counters) ledger {
	return ledger{Schema: schema, Rows: []row{{Language: "go", Route: "default", Fixture: "real_sample",
		FixtureSHA256: "source", SessionSHA256: "session", Full: &c}}}
}

func TestStaleDetectsChangesTheRatchetAccepts(t *testing.T) {
	base := counters{Tokens: 100, NewNodes: 100, MaxLiveVersions: 4, MultiVersionTokenSharePPM: 500,
		ReusedBytes: 100, BlockSplices: 10, StopReason: "accepted", RootEnd: 10, InputBytes: 10}
	for _, tc := range []struct {
		name, counter string
		change        func(*counters)
	}{
		{"share_ppm", "multi_version_token_share_ppm", func(c *counters) { c.MultiVersionTokenSharePPM = 900 }},
		{"new_nodes_fall_10_percent", "new_nodes", func(c *counters) { c.NewNodes = 90 }},
		{"reused_bytes_rise", "reused_bytes", func(c *counters) { c.ReusedBytes = 150 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := base
			tc.change(&now)
			want, got := ledgerWithFull(base), ledgerWithFull(now)
			if err := compare(want, got); err != nil {
				t.Fatalf("the one-sided check rejected an allowed change: %v", err)
			}
			moved, err := staleCells(want, got)
			if err != nil || len(moved) != 1 || moved[0].Counter != tc.counter {
				t.Fatalf("stale cells = %v, %v; want one %s cell", moved, err, tc.counter)
			}
			if err := checkStale(want, got, &strings.Builder{}); err == nil {
				t.Fatal("stale mode passed a changed counter")
			}
		})
	}
}

func TestStalePassesOnIdenticalLedgers(t *testing.T) {
	c := counters{Tokens: 1, StopReason: "accepted"}
	var out strings.Builder
	if err := checkStale(ledgerWithFull(c), ledgerWithFull(c), &out); err != nil || out.Len() != 0 {
		t.Fatalf("identical ledgers: err=%v output=%q", err, out.String())
	}
}

func TestStaleReportIsOrderedAndEndsWithRegenerationCommand(t *testing.T) {
	a := counters{Tokens: 100, NewNodes: 100, StopReason: "accepted"}
	b := a
	b.NewNodes = 90
	b.Tokens = 101
	want := ledger{Schema: schema, Rows: []row{
		{Language: "a", Route: "default", Full: &a}, {Language: "b", Route: "default", Full: &a}}}
	got := ledger{Schema: schema, Rows: []row{
		{Language: "a", Route: "default", Full: &b}, {Language: "b", Route: "default", Full: &b}}}
	var out strings.Builder
	err := checkStale(want, got, &out)
	if err == nil {
		t.Fatal("stale ledger passed")
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	wantLines := []string{
		"a default full tokens 100 -> 101",
		"a default full new_nodes 100 -> 90",
		"b default full tokens 100 -> 101",
		"b default full new_nodes 100 -> 90",
	}
	for i, w := range wantLines {
		if lines[i] != w {
			t.Fatalf("line %d = %q, want %q", i, lines[i], w)
		}
	}
	if last := lines[len(lines)-1]; !strings.Contains(last, regenerateCommand) {
		t.Fatalf("last line %q lacks the regeneration command", last)
	}
	if !strings.Contains(err.Error(), "2 rows") {
		t.Fatalf("error %q does not count rows", err)
	}
}

func TestStaleStructureChangeIsAnError(t *testing.T) {
	c := counters{}
	want := ledgerWithFull(c)
	got := ledger{Schema: schema}
	if _, err := staleCells(want, got); err == nil {
		t.Fatal("row count change passed")
	}
}
