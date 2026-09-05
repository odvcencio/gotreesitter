//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"strings"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestDiagnosticParserCoreAcceptedLeafCoverageRejectsMalformedSpans(t *testing.T) {
	tests := []struct {
		name   string
		first  core.MaterializationSubtreeView
		second *core.MaterializationSubtreeView
		want   string
	}{
		{
			name:  "backward",
			first: core.MaterializationSubtreeView{StartByte: 2, EndByte: 3, Terminal: true},
			second: &core.MaterializationSubtreeView{
				StartByte: 1, EndByte: 2, Terminal: true,
			},
			want: "backward",
		},
		{
			name:  "overlap",
			first: core.MaterializationSubtreeView{StartByte: 0, EndByte: 3, Terminal: true},
			second: &core.MaterializationSubtreeView{
				StartByte: 2, EndByte: 4, Terminal: true,
			},
			want: "overlap",
		},
		{
			name: "reversed",
			first: core.MaterializationSubtreeView{
				StartByte: 3, EndByte: 2, Terminal: true,
			},
			want: "outside source",
		},
		{
			name: "out-of-source",
			first: core.MaterializationSubtreeView{
				StartByte: 1, EndByte: 5, Terminal: true,
			},
			want: "outside source",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var coverage diagnosticParserCoreAcceptedLeafCoverageScratch
			if err := coverage.append(1, test.first, 4, false); test.second == nil {
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("append error=%v, want %q", err, test.want)
				}
				return
			} else if err != nil {
				t.Fatalf("first append: %v", err)
			}
			if err := coverage.append(2, *test.second, 4, false); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("append error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestDiagnosticParserCoreAcceptedLeafCoveragePollsCancellation(t *testing.T) {
	var coverage diagnosticParserCoreAcceptedLeafCoverageScratch
	if err := coverage.append(1, core.MaterializationSubtreeView{StartByte: 0, EndByte: 1, Terminal: true}, 1, false); err != nil {
		t.Fatal(err)
	}
	want := errors.New("cancelled")
	polls := 0
	poll := func() error {
		polls++
		return want
	}
	if _, _, _, err := diagnosticParserCoreAcceptedDerivationLeafCoverageGap(&coverage, []byte("x"), 0, 1, poll); !errors.Is(err, want) {
		t.Fatalf("derivation audit error=%v, want %v", err, want)
	}
	if _, _, _, err := diagnosticParserCoreAcceptedTreeLeafCoverageGap(nil, []byte("x"), 0, 1, 100, &coverage, nil, poll); !errors.Is(err, want) {
		t.Fatalf("public audit error=%v, want %v", err, want)
	}
	if polls < 2 {
		t.Fatalf("poll count=%d, want both audits polled", polls)
	}
}

func TestDiagnosticParserCoreAcceptedLeafCoverageScratchResetsAndReuses(t *testing.T) {
	var coverage diagnosticParserCoreAcceptedLeafCoverageScratch
	if err := coverage.append(1, core.MaterializationSubtreeView{StartByte: 0, EndByte: 1, Terminal: true}, 2, false); err != nil {
		t.Fatal(err)
	}
	coverage.recordAuthenticatedAlias(&Node{})
	coverage.reset()
	if len(coverage.spans) != 0 || len(coverage.authenticatedAliases) != 0 {
		t.Fatalf("reset retained %d spans and %d aliases", len(coverage.spans), len(coverage.authenticatedAliases))
	}
	if err := coverage.append(2, core.MaterializationSubtreeView{StartByte: 1, EndByte: 2, Terminal: true}, 2, false); err != nil {
		t.Fatalf("reuse append: %v", err)
	}
	if len(coverage.spans) != 1 || coverage.spans[0].startByte != 1 {
		t.Fatalf("reused spans=%v", coverage.spans)
	}
}

func TestDiagnosticParserCoreAcceptedAliasSetAccountingAndReset(t *testing.T) {
	var coverage diagnosticParserCoreAcceptedLeafCoverageScratch
	if coverage.footprintBytes() != 0 || coverage.hasAuthenticatedAlias(&Node{}) {
		t.Fatal("empty coverage has storage or authentication")
	}
	nodes := make([]Node, 4096)
	for index := range nodes {
		coverage.recordAuthenticatedAlias(&nodes[index])
	}
	before := coverage.footprintBytes()
	if before < uint64(len(nodes))*64 {
		t.Fatalf("alias footprint=%d does not include live storage", before)
	}
	for index := range nodes {
		if !coverage.hasAuthenticatedAlias(&nodes[index]) {
			t.Fatalf("alias %d lost authentication", index)
		}
		coverage.recordAuthenticatedAlias(&nodes[index])
	}
	if coverage.footprintBytes() != before || len(coverage.authenticatedAliases) != len(nodes) {
		t.Fatal("duplicate aliases increased storage")
	}
	scheduler := &diagnosticParserCoreGenericScheduler{}
	scheduler.options.stopControlMemoryBudgetBytes = int64(before - 1)
	if reason := scheduler.stopControlMemoryBudgetReasonWithAdditionalBytes(before); reason != ParseStopMemoryBudget {
		t.Fatalf("alias storage did not trip memory budget: %s", reason)
	}
	coverage.reset()
	if coverage.authenticatedAliases != nil || coverage.footprintBytes() != 0 || coverage.hasAuthenticatedAlias(&nodes[0]) {
		t.Fatal("reset retained alias storage or authentication")
	}
	coverage.recordAuthenticatedAlias(&nodes[0])
	if !coverage.hasAuthenticatedAlias(&nodes[0]) || coverage.hasAuthenticatedAlias(&nodes[1]) {
		t.Fatal("reuse retained stale alias authentication")
	}
}

func TestDiagnosticParserCoreAcceptedLeafCoverageRejectsHollowPublicError(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	root := newLeafNodeInArena(arena, errorSymbol, true, 0, 1, Point{}, Point{Column: 1})
	var coverage diagnosticParserCoreAcceptedLeafCoverageScratch
	if _, _, gapped, err := diagnosticParserCoreAcceptedTreeLeafCoverageGap(root, []byte("x"), 0, 1, 100, &coverage, nil, nil); err != nil || !gapped {
		t.Fatalf("hollow ERROR audit err=%v gapped=%t, want gapped", err, gapped)
	}
}

func TestDiagnosticParserCoreAcceptedLeafCoverageAcceptsRecoveredTerminalError(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	root := newLeafNodeInArena(arena, errorSymbol, true, 0, 1, Point{}, Point{Column: 1})
	var coverage diagnosticParserCoreAcceptedLeafCoverageScratch
	if err := coverage.append(1, core.MaterializationSubtreeView{
		Symbol: core.RecoveryErrorSymbol, StartByte: 0, EndByte: 1, Terminal: true,
	}, 1, false); err != nil {
		t.Fatal(err)
	}
	nodesByID := make([]*Node, 2)
	nodesByID[1] = root
	if _, _, gapped, err := diagnosticParserCoreAcceptedTreeLeafCoverageGap(root, []byte("x"), 0, 1, 100, &coverage, nodesByID, nil); err != nil || gapped {
		t.Fatalf("recovered ERROR audit err=%v gapped=%t, want complete", err, gapped)
	}
}

func TestDiagnosticParserCoreAcceptedLeafCoverageRejectsUnrelatedVisibleTerminal(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	root := newLeafNodeInArena(arena, errorSymbol, true, 0, 1, Point{}, Point{Column: 1})
	unrelated := newLeafNodeInArena(arena, errorSymbol, true, 0, 1, Point{}, Point{Column: 1})
	var coverage diagnosticParserCoreAcceptedLeafCoverageScratch
	if err := coverage.append(1, core.MaterializationSubtreeView{
		Symbol: core.RecoveryErrorSymbol, StartByte: 0, EndByte: 1, Terminal: true,
	}, 1, false); err != nil {
		t.Fatal(err)
	}
	nodesByID := make([]*Node, 2)
	nodesByID[1] = unrelated
	if _, _, gapped, err := diagnosticParserCoreAcceptedTreeLeafCoverageGap(root, []byte("x"), 0, 1, 100, &coverage, nodesByID, nil); err != nil || !gapped {
		t.Fatalf("unrelated terminal audit err=%v gapped=%t, want gapped", err, gapped)
	}
}

func TestDiagnosticParserCoreAcceptedLeafCoverageAcceptsAuthenticatedTerminalAlias(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	raw := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	parser := &Parser{
		language:       &Language{TokenCount: 100, SymbolMetadata: make([]SymbolMetadata, 102)},
		reduceAliasSeq: [][]Symbol{nil, {101}},
	}
	root := parser.aliasedNodeInArena(arena, raw, Symbol(101))
	var coverage diagnosticParserCoreAcceptedLeafCoverageScratch
	if err := coverage.append(1, core.MaterializationSubtreeView{
		Symbol: 1, StartByte: 0, EndByte: 1, Terminal: true,
	}, 1, false); err != nil {
		t.Fatal(err)
	}
	nodesByID := make([]*Node, 2)
	nodesByID[1] = raw
	coverage.authenticateDirectTerminalAliases(
		parser, []stackEntry{newStackEntryNode(0, raw)}, []*Node{root}, 1, Symbol(101), nodesByID,
	)
	if _, _, gapped, err := diagnosticParserCoreAcceptedTreeLeafCoverageGap(root, []byte("x"), 0, 1, 100, &coverage, nodesByID, nil); err != nil || gapped {
		t.Fatalf("terminal alias audit err=%v gapped=%t, want complete", err, gapped)
	}
}

func TestDiagnosticParserCoreAcceptedLeafCoverageRejectsUnrelatedTerminalAlias(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	raw := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	parser := &Parser{
		language:       &Language{TokenCount: 100, SymbolMetadata: make([]SymbolMetadata, 102)},
		reduceAliasSeq: [][]Symbol{nil, {101}},
	}
	authenticated := parser.aliasedNodeInArena(arena, raw, Symbol(101))
	unrelated := parser.aliasedNodeInArena(arena, raw, Symbol(101))
	var coverage diagnosticParserCoreAcceptedLeafCoverageScratch
	if err := coverage.append(1, core.MaterializationSubtreeView{
		Symbol: 1, StartByte: 0, EndByte: 1, Terminal: true,
	}, 1, false); err != nil {
		t.Fatal(err)
	}
	nodesByID := make([]*Node, 2)
	nodesByID[1] = raw
	coverage.authenticateDirectTerminalAliases(
		parser, []stackEntry{newStackEntryNode(0, raw)}, []*Node{authenticated}, 1, Symbol(101), nodesByID,
	)
	if _, _, gapped, err := diagnosticParserCoreAcceptedTreeLeafCoverageGap(unrelated, []byte("x"), 0, 1, 100, &coverage, nodesByID, nil); err != nil || !gapped {
		t.Fatalf("unrelated terminal alias audit err=%v gapped=%t, want gapped", err, gapped)
	}
}

func TestDiagnosticParserCoreAcceptedLeafCoverageProductionAliases(t *testing.T) {
	for _, name := range []string{"valid", "zero_alias", "wrong_range", "nonterminal", "unmatched_raw", "extra", "duplicate_clone", "wrong_production"} {
		t.Run(name, func(t *testing.T) {
			arena := acquireNodeArena(arenaClassFull)
			defer arena.Release()
			parser := &Parser{
				language:       &Language{TokenCount: 100, SymbolMetadata: make([]SymbolMetadata, 103)},
				reduceAliasSeq: [][]Symbol{nil, {101}},
			}
			raw := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
			clone := parser.aliasedNodeInArena(arena, raw, 101)
			children := []*Node{clone}
			production := uint16(1)
			nodesByID := []*Node{nil, raw}
			var coverage diagnosticParserCoreAcceptedLeafCoverageScratch
			if err := coverage.append(1, core.MaterializationSubtreeView{
				Symbol: 1, StartByte: 0, EndByte: 1, Terminal: true,
			}, 2, false); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "zero_alias":
				parser.reduceAliasSeq[1][0] = 0
			case "wrong_range":
				clone.endByte = 2
			case "nonterminal":
				raw.symbol = 102
			case "unmatched_raw":
				nodesByID[1] = cloneNodeInArena(arena, raw)
			case "extra":
				raw.setExtra(true)
			case "duplicate_clone":
				children = append(children, parser.aliasedNodeInArena(arena, raw, 101))
			case "wrong_production":
				production = 0
			}
			coverage.authenticateDirectTerminalAliases(parser, []stackEntry{newStackEntryNode(0, raw)}, children, production, 0, nodesByID)
			if got := coverage.hasAuthenticatedAlias(clone); got != (name == "valid") {
				t.Fatalf("production alias authenticated=%t", got)
			}
			if name == "valid" || name == "wrong_production" || name == "unmatched_raw" || name == "duplicate_clone" {
				_, _, gapped, err := diagnosticParserCoreAcceptedTreeLeafCoverageGap(clone, []byte("x"), 0, 1, 100, &coverage, nodesByID, nil)
				if err != nil || gapped != (name != "valid") {
					t.Fatalf("public alias coverage gap=%t err=%v", gapped, err)
				}
			}
			// Authentication belongs to the exact projected node, not its range.
			if coverage.hasAuthenticatedAlias(parser.aliasedNodeInArena(arena, raw, 101)) {
				t.Fatal("an unrelated clone inherited production authentication")
			}
		})
	}
}

func TestDiagnosticParserCoreAcceptedHiddenLeafCoversTrailingTrivia(t *testing.T) {
	var coverage diagnosticParserCoreAcceptedLeafCoverageScratch
	if err := coverage.append(1, core.MaterializationSubtreeView{StartByte: 0, EndByte: 1, Terminal: true}, 2, true); err != nil {
		t.Fatal(err)
	}
	next := 0
	covered, err := diagnosticParserCoreAcceptedHiddenLeafCovers(&coverage, []byte("x "), 0, 2, &next, nil)
	if err != nil || !covered {
		t.Fatalf("hidden coverage err=%v covered=%t, want covered", err, covered)
	}
}

func TestDiagnosticParserCoreAcceptedLeafCoveragePollsLongTrivia(t *testing.T) {
	source := make([]byte, 2048)
	for index := range source {
		source[index] = ' '
	}
	var coverage diagnosticParserCoreAcceptedLeafCoverageScratch
	if err := coverage.append(1, core.MaterializationSubtreeView{
		StartByte: uint32(len(source)), EndByte: uint32(len(source)), Terminal: true,
	}, uint32(len(source)), false); err != nil {
		t.Fatal(err)
	}
	want := errors.New("cancelled during trivia")
	polls := 0
	poll := func() error {
		polls++
		if polls == 2 {
			return want
		}
		return nil
	}
	if _, _, _, err := diagnosticParserCoreAcceptedDerivationLeafCoverageGap(&coverage, source, 0, uint32(len(source)), poll); !errors.Is(err, want) {
		t.Fatalf("long-trivia audit error=%v, want %v", err, want)
	}
	if polls < 2 {
		t.Fatalf("poll count=%d, want bounded trivia polling", polls)
	}
}
