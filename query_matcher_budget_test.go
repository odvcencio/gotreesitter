package gotreesitter

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestQueryExecutionStatusConcurrentCursors(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(program (identifier)+ @item) @root`, lang)
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	errors := make(chan string, 4)
	for worker := 0; worker < 4; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			children := make([]*Node, 128)
			for i := range children {
				children[i] = leaf(Symbol(1), true, uint32(i), uint32(i+1))
			}
			tree := NewTree(parent(Symbol(7), true, children, nil), make([]byte, len(children)), lang)
			for iteration := 0; iteration < 10; iteration++ {
				matches, status := q.ExecuteWithStatus(tree)
				if status != QueryComplete || len(matches) != 1 || len(matches[0].Captures) != 129 {
					errors <- "concurrent match state leaked"
					return
				}
				cursor := q.Exec(tree.RootNode(), lang, tree.Source())
				count := 0
				for {
					if _, ok := cursor.NextCapture(); !ok {
						break
					}
					count++
				}
				if count != 383 || cursor.Status() != QueryComplete {
					errors <- fmt.Sprintf("capture count=%d status=%v", count, cursor.Status())
					return
				}
			}
		}()
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
}

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
			t.Logf("width=%d states=%d matches=%d budget_exceeded=%t", width, defaultQueryMatchWorkBudget-budget.Remaining(), len(matches), budget.Exceeded())
		})
	}
}

func TestQueryQuantifiedSuccessCounters(t *testing.T) {
	lang := queryTestLanguage()
	q, err := NewQuery(`(block (identifier)+ @item)`, lang)
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{32, 256, 4096} {
		t.Run(fmt.Sprintf("width=%d", width), func(t *testing.T) {
			tree := buildWideIdentifierBlock(lang, width)
			defer tree.Release()
			budget := newQueryMatchBudget(defaultQueryMatchWorkBudget)
			matches := q.matchPatternAll(&q.patterns[0], tree.RootNode(), lang, tree.Source(), budget)
			if len(matches) != 1 || len(matches[0]) != width || budget.Exceeded() {
				t.Fatalf("matches=%d budget_exceeded=%t", len(matches), budget.Exceeded())
			}
			t.Logf("width=%d states=%d matches=%d captures=%d budget_exceeded=%t", width, defaultQueryMatchWorkBudget-budget.Remaining(), len(matches), len(matches[0]), budget.Exceeded())
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

func TestQueryExecutionStatusSharedNestedBudget(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildWideIdentifierBlock(lang, 3)
	defer tree.Release()
	q, err := NewQuery(`[(block (identifier) @item)] @root`, lang)
	if err != nil {
		t.Fatal(err)
	}
	limited := q.Exec(tree.RootNode(), lang, tree.Source())
	limited.SetMatchWorkBudget(1)
	if limited.Status() != QueryPending {
		t.Fatal("new cursor must be pending")
	}
	if _, ok := limited.NextMatch(); ok {
		t.Fatal("nested branch escaped its outer work allowance")
	}
	if limited.Status() != QueryWorkBudgetExceeded || !limited.DidExceedMatchLimit() {
		t.Fatalf("status=%v exceeded=%v", limited.Status(), limited.DidExceedMatchLimit())
	}
	limited.SetMatchLimit(10)
	if limited.Status() != QueryWorkBudgetExceeded || !limited.DidExceedMatchLimit() {
		t.Fatal("changing the output limit erased work-budget exhaustion")
	}
	for _, limit := range []int{defaultQueryMatchWorkBudget, 0} {
		cursor := q.Exec(tree.RootNode(), lang, tree.Source())
		cursor.SetMatchWorkBudget(limit)
		matches := drainCursorWithDeadline(t, cursor, 5*time.Second)
		if len(matches) != 3 || cursor.Status() != QueryComplete {
			t.Fatalf("limit=%d matches=%d status=%v", limit, len(matches), cursor.Status())
		}
	}
	ordinary, status := q.ExecuteWithStatus(tree)
	if status != QueryComplete || len(ordinary) != 3 {
		t.Fatalf("batch matches=%d status=%v", len(ordinary), status)
	}
	prefix := QueryMatch{PatternIndex: -1}
	appended, status := q.ExecuteIntoWithStatus(tree, []QueryMatch{prefix})
	if status != QueryComplete || len(appended) != 4 || appended[0].PatternIndex != -1 {
		t.Fatalf("append matches=%d status=%v", len(appended), status)
	}
	if matches, status := q.ExecuteWithStatus(nil); len(matches) != 0 || status != QueryComplete {
		t.Fatalf("nil tree matches=%d status=%v", len(matches), status)
	}
}

func TestQueryExecutionStatusOutputBound(t *testing.T) {
	lang := queryTestLanguage()
	children := make([]*Node, 5000)
	for i := range children {
		children[i] = leaf(Symbol(1), true, uint32(i), uint32(i+1))
	}
	root := parent(Symbol(7), true, children, nil)
	tree := NewTree(root, make([]byte, len(children)), lang)
	q, err := NewQuery(`(program (identifier) @item)`, lang)
	if err != nil {
		t.Fatal(err)
	}
	matches, status := q.ExecuteIntoWithStatus(tree, nil)
	if len(matches) != 4096 || status != QueryWorkBudgetExceeded {
		t.Fatalf("matches=%d status=%v, want 4096 and explicit incomplete status", len(matches), status)
	}
}

func TestQueryRootQuantifiedSuccessCounters(t *testing.T) {
	lang := queryTestLanguage()
	for _, width := range []int{32, 256, 4096} {
		t.Run(fmt.Sprintf("width=%d", width), func(t *testing.T) {
			children := make([]*Node, width)
			for i := range children {
				children[i] = leaf(Symbol(1), true, uint32(i), uint32(i+1))
			}
			root := parent(Symbol(7), true, children, nil)
			q, err := NewQuery(`(identifier)+ @item`, lang)
			if err != nil {
				t.Fatal(err)
			}
			budget := newQueryMatchBudget(defaultQueryMatchWorkBudget)
			matches := q.matchPatternPostorderAll(&q.patterns[0], children[width-1], root, width-1, lang, nil, budget)
			if len(matches) != 1 || len(matches[0]) != width || budget.Exceeded() {
				t.Fatal("inexact or incomplete root run")
			}
			t.Logf("width=%d states=%d captures=%d", width, defaultQueryMatchWorkBudget-budget.Remaining(), len(matches[0]))
		})
	}
}

func TestQueryNextCaptureStreamsSeparatedNodes(t *testing.T) {
	lang := queryTestLanguage()
	children := make([]*Node, 8192)
	for i := range children {
		children[i] = leaf(Symbol(1), true, uint32(i), uint32(i+1))
	}
	root := parent(Symbol(7), true, children, nil)
	tree := NewTree(root, make([]byte, len(children)), lang)
	q, err := NewQuery(`(identifier) @item`, lang)
	if err != nil {
		t.Fatal(err)
	}
	cursor := q.Exec(root, lang, tree.Source())
	for i := range children {
		capture, ok := cursor.NextCapture()
		if !ok || capture.Node != children[i] {
			t.Fatalf("capture %d = %+v, %t", i, capture, ok)
		}
		if len(cursor.captureQueue.Entries) > 1 {
			t.Fatal("cursor retained captures beyond its lookahead")
		}
	}
	if _, ok := cursor.NextCapture(); ok || cursor.Status() != QueryComplete {
		t.Fatal("stream did not complete")
	}
}

func TestQueryExecutionStatusOutputLimit(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildWideIdentifierBlock(lang, 3)
	defer tree.Release()
	q, err := NewQuery(`(identifier) @item`, lang)
	if err != nil {
		t.Fatal(err)
	}
	for _, limit := range []uint32{1, 3} {
		cursor := q.Exec(tree.RootNode(), lang, tree.Source())
		cursor.SetMatchLimit(limit)
		matches := drainCursorWithDeadline(t, cursor, 5*time.Second)
		wantStatus := QueryComplete
		if limit < 3 {
			wantStatus = QueryMatchLimitExceeded
		}
		if len(matches) != int(limit) || cursor.Status() != wantStatus {
			t.Fatalf("limit=%d matches=%d status=%v, want %v", limit, len(matches), cursor.Status(), wantStatus)
		}
	}
}

func TestQueryExecutionStatusWideSuccessfulRun(t *testing.T) {
	lang := queryTestLanguage()
	tree := buildWideIdentifierBlock(lang, 8192)
	defer tree.Release()
	q, err := NewQuery(`(block (identifier)+ @item)`, lang)
	if err != nil {
		t.Fatal(err)
	}
	matches, status := q.ExecuteWithStatus(tree)
	if status != QueryComplete || len(matches) != 1 || len(matches[0].Captures) != 8192 {
		t.Fatalf("matches=%d status=%v", len(matches), status)
	}
	for i, capture := range matches[0].Captures {
		if capture.Node.StartByte() != uint32(2*i) {
			t.Fatalf("capture %d was overwritten during accumulation", i)
		}
	}
}
