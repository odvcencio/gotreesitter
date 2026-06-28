package gotreesitter

import (
	"errors"
	"testing"
	"time"
)

func goDotCompatibilityTestLanguage() *Language {
	return &Language{
		Name:       "go",
		TokenCount: 2,
		SymbolNames: []string{
			"EOF",
			".",
			"dot",
			"source_file",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: ".", Visible: true, Named: false},
			{Name: "dot", Visible: true, Named: true},
			{Name: "source_file", Visible: true, Named: true},
		},
	}
}

func TestNormalizeGoDotLeafChildrenHonorsStopCheck(t *testing.T) {
	lang := goDotCompatibilityTestLanguage()
	arena := newNodeArena(arenaClassFull)
	const dotLeaves = 4096
	children := make([]*Node, 0, dotLeaves)
	for i := 0; i < dotLeaves; i++ {
		children = append(children, newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1}))
	}
	root := newParentNodeInArena(arena, 3, true, children, nil, 0)
	source := []byte(".")
	checks := 0
	poller := parseStopPoller{
		check: func() ParseStopReason {
			checks++
			if checks < 2 {
				return ParseStopNone
			}
			return ParseStopTimeout
		},
	}

	reason := normalizeGoDotLeafChildrenWithStop(root, source, lang, &poller)

	if reason != ParseStopTimeout {
		t.Fatalf("normalizeGoDotLeafChildrenWithStop reason = %q, want %q", reason, ParseStopTimeout)
	}
	var rewritten int
	for i := 0; i < resultChildCount(root); i++ {
		if resultChildCount(resultChildAt(root, i)) > 0 {
			rewritten++
		}
	}
	if rewritten == 0 || rewritten >= dotLeaves {
		t.Fatalf("rewritten dot leaves = %d, want partial rewrite before timeout", rewritten)
	}
}

func TestParseForRecoveryHonorsExpiredActiveBudget(t *testing.T) {
	parser := NewParser(buildArithmeticLanguage())
	parser.SetTimeoutMicros(1)
	endBudget := parser.enterParseBudgetAt(time.Now().Add(-time.Millisecond))
	defer endBudget()

	tree, err := parser.parseForRecovery([]byte("1+2"))

	if tree != nil {
		t.Fatalf("parseForRecovery returned tree %p, want nil", tree)
	}
	if !errors.Is(err, ErrParseStoppedEarly) {
		t.Fatalf("parseForRecovery error = %v, want ErrParseStoppedEarly", err)
	}
	var stopped *ParseStoppedEarlyError
	if !errors.As(err, &stopped) {
		t.Fatalf("parseForRecovery error type = %T, want *ParseStoppedEarlyError", err)
	}
	if stopped.Reason != ParseStopTimeout {
		t.Fatalf("stopped reason = %q, want %q", stopped.Reason, ParseStopTimeout)
	}
	if got := parser.activeParseStopReason(); got != ParseStopTimeout {
		t.Fatalf("activeParseStopReason = %q, want %q", got, ParseStopTimeout)
	}
}
