package gotreesitter

import (
	"strings"
	"testing"
	"time"
)

// bigArithmeticSource returns arithmetic-language source text large enough
// that an unbounded parse would take many thousands of parser steps: a run
// of n "1+" pairs terminated by a trailing "1".
func bigArithmeticSource(n int) []byte {
	var b strings.Builder
	b.Grow(n*2 + 1)
	for i := 0; i < n; i++ {
		b.WriteString("1+")
	}
	b.WriteByte('1')
	return []byte(b.String())
}

// TestHighlighterInjectedParseRespectsTimeout verifies that a Highlighter's
// configured timeout (WithHighlighterTimeoutMicros) now bounds every
// injected parse it performs, not just the document parse. Before the fix,
// parseInjectedTree always created injected parsers with no timeout at all,
// so a large injected block could run unbounded.
func TestHighlighterInjectedParseRespectsTimeout(t *testing.T) {
	parentLang := buildContainerLanguage()
	childLang := buildArithmeticLanguage()

	const parentName = "gts_offset_test_container_hl_timeout"
	parentLang.Name = parentName
	RegisterHighlighterInjection(parentName, HighlighterInjectionSpec{
		Query: `(content) @injection.content (#set! injection.language "child")`,
		ResolveLanguage: func(hint string) (*Language, string, func([]byte) TokenSource, bool) {
			if hint != "child" {
				return nil, "", nil, false
			}
			return childLang, `(NUMBER) @number`, nil, true
		},
	})

	h, err := NewHighlighter(parentLang, `(document) @doc`, WithHighlighterTimeoutMicros(1))
	if err != nil {
		t.Fatalf("NewHighlighter: %v", err)
	}

	content := bigArithmeticSource(200_000)
	source := append(append([]byte{'['}, content...), ']')

	// Direct check: the exact method Highlight uses to parse an injected
	// block must itself honor the highlighter's timeout and report a visible
	// stop reason on the returned tree.
	start := time.Now()
	tree, err := h.parseInjectedTree(childLang, nil, content)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("parseInjectedTree: %v", err)
	}
	if tree == nil {
		t.Fatal("parseInjectedTree returned a nil tree")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("parseInjectedTree took %v, want < 2s", elapsed)
	}
	if got := tree.ParseStopReason(); got != ParseStopTimeout {
		t.Fatalf("injected tree ParseStopReason = %v, want %v", got, ParseStopTimeout)
	}

	// End-to-end: Highlight must also return quickly for the same reason.
	start = time.Now()
	_ = h.Highlight(source)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Highlight took %v, want < 2s", elapsed)
	}
}

// TestHighlighterInjectedParseInheritsCancellationFlag verifies parseInjectedTree
// propagates the document parser's cancellation flag to the injected parser.
func TestHighlighterInjectedParseInheritsCancellationFlag(t *testing.T) {
	parentLang := buildContainerLanguage()
	childLang := buildArithmeticLanguage()

	h, err := NewHighlighter(parentLang, `(document) @doc`)
	if err != nil {
		t.Fatalf("NewHighlighter: %v", err)
	}
	var flag uint32
	h.parser.SetCancellationFlag(&flag)
	flag = 1 // already cancelled

	content := bigArithmeticSource(200_000)
	start := time.Now()
	tree, err := h.parseInjectedTree(childLang, nil, content)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("parseInjectedTree: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("parseInjectedTree took %v, want < 2s", elapsed)
	}
	if tree == nil {
		t.Fatal("parseInjectedTree returned a nil tree")
	}
	if got := tree.ParseStopReason(); got != ParseStopCancelled {
		t.Fatalf("injected tree ParseStopReason = %v, want %v", got, ParseStopCancelled)
	}
}
