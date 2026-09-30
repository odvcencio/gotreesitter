package gotreesitter

import (
	"fmt"
	"testing"
	"time"
)

// TestQueryQuantifiedWitnessCounters records matcher states before timing.
func TestQueryQuantifiedWitnessCounters(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(block (identifier)* @e (function_declaration) @r)`, lang)
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{16, 32, 64} {
		t.Run(fmt.Sprintf("width=%d", width), func(t *testing.T) {
			tree := buildWideIdentifierBlock(lang, width)
			defer tree.Release()
			budget := newQueryMatchBudget(defaultQueryMatchWorkBudget)
			matches := q.matchPatternAll(&q.patterns[0], tree.RootNode(), lang, tree.Source(), budget)
			if len(matches) != 0 {
				t.Fatalf("matches=%d, want 0", len(matches))
			}
			t.Logf("width=%d states=%d matches=%d budget_exceeded=%t", width, defaultQueryMatchWorkBudget-budget.remaining, len(matches), budget.tripped())
		})
	}
}

// buildWideIdentifierBlock builds a synthetic "block" node with n
// "identifier" children and no trailing "function_declaration" node. A
// pattern like `(block (identifier)* @e (function_declaration) @r)` against
// this tree has a quantified child step ((identifier)*) followed by a
// required step ((function_declaration)) that can never match -- the
// pathological shape from the bug report. Before the work-budget fix,
// matchChildStepsRecursiveAll's tryCombinations enumerator would walk the
// full power set of the n candidate identifiers (2^n combinations) before
// ever reporting failure for this (pattern,node) attempt, since the
// required trailing step is checked only after each candidate subset is
// fully chosen.
func buildWideIdentifierBlock(lang *Language, n int) *Tree {
	source := make([]byte, 0, n*2)
	children := make([]*Node, n)
	fields := make([]FieldID, n)
	pos := uint32(0)
	for i := 0; i < n; i++ {
		if i > 0 {
			source = append(source, ' ')
			pos++
		}
		children[i] = leaf(Symbol(1), true, pos, pos+1)
		source = append(source, 'x')
		pos++
	}
	block := parent(Symbol(14), true, children, fields)
	return NewTree(block, source, lang)
}

// drainCursorWithDeadline exhausts cursor via NextMatch on a background
// goroutine and fails the test if it doesn't finish within the deadline.
// Returns the matches collected (empty if the deadline was hit).
func drainCursorWithDeadline(t *testing.T, cursor *QueryCursor, deadline time.Duration) []QueryMatch {
	t.Helper()
	var matches []QueryMatch
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			m, ok := cursor.NextMatch()
			if !ok {
				return
			}
			matches = append(matches, m)
		}
	}()

	select {
	case <-done:
		return matches
	case <-time.After(deadline):
		t.Fatalf("query matcher did not terminate within %s -- exponential blowup not bounded", deadline)
		return nil
	}
}

// TestQueryMatchWorkBudgetTerminatesPathologicalQuantifier reproduces the
// wide failed-suffix witness. The shared matcher proves that the required
// successor is absent without enumerating subsets, so this is a complete
// empty result, not a budget-exceeded partial result.
func TestQueryMatchWorkBudgetTerminatesPathologicalQuantifier(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildWideIdentifierBlock(lang, 32)

	q, err := NewQuery(`(block (identifier)* @e (function_declaration) @r)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	cursor := q.Exec(tree.RootNode(), tree.Language(), tree.Source())
	matches := drainCursorWithDeadline(t, cursor, 5*time.Second)

	if len(matches) != 0 {
		t.Fatalf("matches: got %d, want 0 (trailing function_declaration step never matches)", len(matches))
	}
	if cursor.DidExceedMatchLimit() {
		t.Fatal("DidExceedMatchLimit: got true for a proven impossible suffix")
	}
}

// TestQueryMatchWorkBudgetUnderBudgetIdentity checks that the work budget
// does not alter results for an ordinary, well-under-budget match: a small
// run of identifiers actually followed by a function_declaration. This
// exercises the *same* matchChildStepsRecursiveAll code path (quantified
// step followed by a required step) as the pathological test above, but one
// where the required step succeeds on the first (greediest) combination
// tried, so the match should be found immediately and be identical to
// pre-budget behavior.
func TestQueryMatchWorkBudgetUnderBudgetIdentity(t *testing.T) {
	lang := queryTestLanguage()
	source := []byte("a b c f")
	id0 := leaf(Symbol(1), true, 0, 1)
	id1 := leaf(Symbol(1), true, 2, 3)
	id2 := leaf(Symbol(1), true, 4, 5)
	fn := leaf(Symbol(5), true, 6, 7)
	block := parent(Symbol(14), true, []*Node{id0, id1, id2, fn}, []FieldID{0, 0, 0, 0})
	tree := NewTree(block, source, lang)

	q, err := NewQuery(`(block (identifier)* @e (function_declaration) @r)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	cursor := q.Exec(tree.RootNode(), tree.Language(), tree.Source())
	matches := drainCursorWithDeadline(t, cursor, 5*time.Second)

	if cursor.DidExceedMatchLimit() {
		t.Fatal("DidExceedMatchLimit: got true, want false for a small under-budget tree")
	}
	if len(matches) != 1 {
		t.Fatalf("matches: got %d, want 1", len(matches))
	}
	if len(matches[0].Captures) != 4 {
		t.Fatalf("captures: got %d, want 4", len(matches[0].Captures))
	}
	for i, wantText := range []string{"a", "b", "c"} {
		if got := matches[0].Captures[i].Node.Text(source); got != wantText {
			t.Fatalf("capture[%d] (@e): got %q, want %q", i, got, wantText)
		}
		if got := matches[0].Captures[i].Name; got != "e" {
			t.Fatalf("capture[%d] name: got %q, want %q", i, got, "e")
		}
	}
	if got := matches[0].Captures[3].Node.Text(source); got != "f" {
		t.Fatalf("capture[3] (@r): got %q, want %q", got, "f")
	}
	if got := matches[0].Captures[3].Name; got != "r" {
		t.Fatalf("capture[3] name: got %q, want %q", got, "r")
	}
}

// TestQueryMatchWorkBudgetUnlimitedEscapeHatch checks that
// SetMatchWorkBudget(0) restores the legacy unbounded behavior. n is kept
// small (18, so 2^18 is a few hundred thousand combinations) so the test
// still completes quickly -- comfortably under the 5s deadline -- even
// without a budget. (Measured empirically: n=18 ~0.8s, n=20 ~2.7s, n=22
// ~11.6s on this enumerator, so 18 leaves solid headroom under 5s.)
func TestQueryMatchWorkBudgetUnlimitedEscapeHatch(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildWideIdentifierBlock(lang, 18)

	q, err := NewQuery(`(block (identifier)* @e (function_declaration) @r)`, lang)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	cursor := q.Exec(tree.RootNode(), tree.Language(), tree.Source())
	cursor.SetMatchWorkBudget(0)
	matches := drainCursorWithDeadline(t, cursor, 5*time.Second)

	if len(matches) != 0 {
		t.Fatalf("matches: got %d, want 0 (trailing function_declaration step never matches)", len(matches))
	}
	if cursor.DidExceedMatchLimit() {
		t.Fatal("DidExceedMatchLimit: got true, want false when SetMatchWorkBudget(0) disables the bound")
	}
}
