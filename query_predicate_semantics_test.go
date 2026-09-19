package gotreesitter_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// These tests cover three query-engine defects, each on both matcher paths:
// the cursor path (Query.Exec / QueryCursor.NextMatch) and the generic
// reader path (Query.Execute). Every test asserts both paths agree.

// parseJSForPredicateTest parses source with the JavaScript grammar and
// registers cleanup to release the tree.
func parseJSForPredicateTest(t *testing.T, source string) (*gotreesitter.Tree, *gotreesitter.Language) {
	t.Helper()
	lang := grammars.JavascriptLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse([]byte(source))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	t.Cleanup(tree.Release)
	return tree, lang
}

// predicateSemanticsMatches runs q against tree on both matcher paths and
// returns a sorted, human-readable rendering of each path's matches so a
// test can assert both paths produce the same result.
func predicateSemanticsMatches(tree *gotreesitter.Tree, lang *gotreesitter.Language, source []byte, q *gotreesitter.Query) (readerPath, cursorPath []string) {
	for _, m := range q.Execute(tree) {
		readerPath = append(readerPath, formatPredicateSemanticsMatch(m, source))
	}
	cursor := q.Exec(tree.RootNode(), lang, source)
	for {
		m, ok := cursor.NextMatch()
		if !ok {
			break
		}
		cursorPath = append(cursorPath, formatPredicateSemanticsMatch(m, source))
	}
	sort.Strings(readerPath)
	sort.Strings(cursorPath)
	return readerPath, cursorPath
}

func formatPredicateSemanticsMatch(m gotreesitter.QueryMatch, source []byte) string {
	var b strings.Builder
	fmt.Fprintf(&b, "p%d[", m.PatternIndex)
	for i, c := range m.Captures {
		if i > 0 {
			b.WriteByte(',')
		}
		if c.Node == nil {
			fmt.Fprintf(&b, "%s=<nil>", c.Name)
			continue
		}
		fmt.Fprintf(&b, "%s=%q@%d-%d", c.Name, c.Text(source), c.Node.StartByte(), c.Node.EndByte())
	}
	b.WriteByte(']')
	return b.String()
}

// TestQuantifiedPredicateNotEqRejectsAnyViolatingNode covers defect 1: a
// predicate on a quantified (`+`/`*`) capture must check every bound node,
// not just the first. `((program (comment)+ @doc) (#not-eq? @doc "// b"))`
// binds @doc to both "// a" and "// b"; the second violates #not-eq?, so the
// pattern must produce zero matches on both matcher paths.
func TestQuantifiedPredicateNotEqRejectsAnyViolatingNode(t *testing.T) {
	source := "// a\n// b\nlet x = 1;\n"
	tree, lang := parseJSForPredicateTest(t, source)

	q, err := gotreesitter.NewQuery(`((program (comment)+ @doc) (#not-eq? @doc "// b"))`, lang)
	if err != nil {
		t.Fatalf("compile query: %v", err)
	}

	readerPath, cursorPath := predicateSemanticsMatches(tree, lang, []byte(source), q)
	if len(readerPath) != 0 {
		t.Errorf("reader path (Execute): got %d matches, want 0: %v", len(readerPath), readerPath)
	}
	if len(cursorPath) != 0 {
		t.Errorf("cursor path (Exec/NextMatch): got %d matches, want 0: %v", len(cursorPath), cursorPath)
	}
	if strings.Join(readerPath, "\n") != strings.Join(cursorPath, "\n") {
		t.Fatalf("matcher paths disagree\nreader: %v\ncursor: %v", readerPath, cursorPath)
	}
}

// TestQuantifiedPredicateEqAcceptsWhenEveryNodeSatisfies is the positive
// counterpart: every @doc node equals "// a", so #eq? must hold and the
// pattern must produce exactly one match on both matcher paths.
func TestQuantifiedPredicateEqAcceptsWhenEveryNodeSatisfies(t *testing.T) {
	source := "// a\n// a\nlet x = 1;\n"
	tree, lang := parseJSForPredicateTest(t, source)

	q, err := gotreesitter.NewQuery(`((program (comment)+ @doc) (#eq? @doc "// a"))`, lang)
	if err != nil {
		t.Fatalf("compile query: %v", err)
	}

	readerPath, cursorPath := predicateSemanticsMatches(tree, lang, []byte(source), q)
	if len(readerPath) != 1 {
		t.Errorf("reader path (Execute): got %d matches, want 1: %v", len(readerPath), readerPath)
	}
	if len(cursorPath) != 1 {
		t.Errorf("cursor path (Exec/NextMatch): got %d matches, want 1: %v", len(cursorPath), cursorPath)
	}
	if strings.Join(readerPath, "\n") != strings.Join(cursorPath, "\n") {
		t.Fatalf("matcher paths disagree\nreader: %v\ncursor: %v", readerPath, cursorPath)
	}
}

// TestQuantifiedPredicateAnyEqAcceptsWhenOneNodeSatisfies covers the
// "at least one" bucket (#any-eq?, #any-not-eq?, #any-match?,
// #any-not-match?): only one of the two @doc nodes needs to equal "// b".
func TestQuantifiedPredicateAnyEqAcceptsWhenOneNodeSatisfies(t *testing.T) {
	source := "// a\n// b\nlet x = 1;\n"
	tree, lang := parseJSForPredicateTest(t, source)

	q, err := gotreesitter.NewQuery(`((program (comment)+ @doc) (#any-eq? @doc "// b"))`, lang)
	if err != nil {
		t.Fatalf("compile query: %v", err)
	}

	readerPath, cursorPath := predicateSemanticsMatches(tree, lang, []byte(source), q)
	if len(readerPath) != 1 {
		t.Errorf("reader path (Execute): got %d matches, want 1: %v", len(readerPath), readerPath)
	}
	if len(cursorPath) != 1 {
		t.Errorf("cursor path (Exec/NextMatch): got %d matches, want 1: %v", len(cursorPath), cursorPath)
	}
	if strings.Join(readerPath, "\n") != strings.Join(cursorPath, "\n") {
		t.Fatalf("matcher paths disagree\nreader: %v\ncursor: %v", readerPath, cursorPath)
	}
}

// TestPredicateUndefinedCaptureIsCompileError covers defect 2: a predicate
// that names a capture the pattern never binds must fail to compile, the
// way C's TSQueryErrorCapture does, instead of compiling into a predicate
// that can never be satisfied.
func TestPredicateUndefinedCaptureIsCompileError(t *testing.T) {
	lang := grammars.JavascriptLanguage()

	_, err := gotreesitter.NewQuery(`((comment) @c (#eq? @typo "// a"))`, lang)
	if err == nil {
		t.Fatal("expected a compile error for a predicate referencing an undefined capture")
	}
	if !strings.Contains(err.Error(), "typo") {
		t.Fatalf("error should name the undefined capture: %v", err)
	}

	// A predicate over a capture the pattern does bind must still compile.
	if _, err := gotreesitter.NewQuery(`((comment) @c (#eq? @c "// a"))`, lang); err != nil {
		t.Fatalf("a predicate over a bound capture must compile: %v", err)
	}
}

// TestDisableCaptureDoesNotChangeMatching covers defect 3: DisableCapture's
// doc comment promises unchanged matching -- predicates must still see a
// disabled capture's nodes, and only the returned match should omit it.
func TestDisableCaptureDoesNotChangeMatching(t *testing.T) {
	source := "// a\n// b\nlet x = 1;\n"
	tree, lang := parseJSForPredicateTest(t, source)

	q, err := gotreesitter.NewQuery(`((comment) @c (#eq? @c "// a"))`, lang)
	if err != nil {
		t.Fatalf("compile query: %v", err)
	}

	beforeReader, beforeCursor := predicateSemanticsMatches(tree, lang, []byte(source), q)
	if len(beforeReader) != 1 || len(beforeCursor) != 1 {
		t.Fatalf("before DisableCapture: want 1 match on each path, got reader=%v cursor=%v", beforeReader, beforeCursor)
	}

	q.DisableCapture("c")

	afterReaderMatches := q.Execute(tree)
	if len(afterReaderMatches) != 1 {
		t.Fatalf("reader path (Execute) after DisableCapture: got %d matches, want 1 (matching must be unchanged)", len(afterReaderMatches))
	}
	if len(afterReaderMatches[0].Captures) != 0 {
		t.Fatalf("reader path (Execute) after DisableCapture: got %d captures, want 0 (disabled capture must be omitted from output)", len(afterReaderMatches[0].Captures))
	}

	cursor := q.Exec(tree.RootNode(), lang, []byte(source))
	afterCursorMatch, ok := cursor.NextMatch()
	if !ok {
		t.Fatal("cursor path (Exec/NextMatch) after DisableCapture: got 0 matches, want 1 (matching must be unchanged)")
	}
	if len(afterCursorMatch.Captures) != 0 {
		t.Fatalf("cursor path (Exec/NextMatch) after DisableCapture: got %d captures, want 0 (disabled capture must be omitted from output)", len(afterCursorMatch.Captures))
	}
	if _, ok := cursor.NextMatch(); ok {
		t.Fatal("cursor path (Exec/NextMatch) after DisableCapture: got a second match, want exactly 1")
	}
}

// TestDisableCaptureDoesNotChangeMatchingForRepeatedRootPattern is the
// postorder counterpart of TestDisableCaptureDoesNotChangeMatching: a
// repeated-root pattern (`+`/`*`) goes through a distinct finalize path in
// both query_matcher.go and query_matcher_generic.go, so it needs its own
// coverage.
func TestDisableCaptureDoesNotChangeMatchingForRepeatedRootPattern(t *testing.T) {
	source := "// a\n// b\nlet x = 1;\n"
	tree, lang := parseJSForPredicateTest(t, source)

	q, err := gotreesitter.NewQuery(`(comment)+ @doc`, lang)
	if err != nil {
		t.Fatalf("compile query: %v", err)
	}

	beforeReader, beforeCursor := predicateSemanticsMatches(tree, lang, []byte(source), q)
	if len(beforeReader) == 0 || len(beforeCursor) == 0 {
		t.Fatalf("before DisableCapture: want at least 1 match on each path, got reader=%v cursor=%v", beforeReader, beforeCursor)
	}
	wantMatchCount := len(beforeReader)

	q.DisableCapture("doc")

	afterReaderMatches := q.Execute(tree)
	if len(afterReaderMatches) != wantMatchCount {
		t.Fatalf("reader path (Execute) after DisableCapture: got %d matches, want %d (matching must be unchanged)", len(afterReaderMatches), wantMatchCount)
	}
	for _, m := range afterReaderMatches {
		if len(m.Captures) != 0 {
			t.Fatalf("reader path (Execute) after DisableCapture: got %d captures, want 0", len(m.Captures))
		}
	}

	cursor := q.Exec(tree.RootNode(), lang, []byte(source))
	afterCursorCount := 0
	for {
		m, ok := cursor.NextMatch()
		if !ok {
			break
		}
		afterCursorCount++
		if len(m.Captures) != 0 {
			t.Fatalf("cursor path (Exec/NextMatch) after DisableCapture: got %d captures, want 0", len(m.Captures))
		}
	}
	if afterCursorCount != wantMatchCount {
		t.Fatalf("cursor path (Exec/NextMatch) after DisableCapture: got %d matches, want %d (matching must be unchanged)", afterCursorCount, wantMatchCount)
	}
}
