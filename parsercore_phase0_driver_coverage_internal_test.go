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
	coverage.reset()
	if len(coverage.spans) != 0 {
		t.Fatalf("reset retained %d spans", len(coverage.spans))
	}
	if err := coverage.append(2, core.MaterializationSubtreeView{StartByte: 1, EndByte: 2, Terminal: true}, 2, false); err != nil {
		t.Fatalf("reuse append: %v", err)
	}
	if len(coverage.spans) != 1 || coverage.spans[0].startByte != 1 {
		t.Fatalf("reused spans=%v", coverage.spans)
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
