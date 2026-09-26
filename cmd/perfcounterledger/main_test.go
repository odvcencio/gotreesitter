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
