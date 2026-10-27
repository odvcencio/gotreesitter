package highlight_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// forbiddenHighlightCapture reports whether name belongs to the captures that
// carry no color: private helpers, spell-check and conceal markers, reset
// markers and locals-query directives. It is written without the
// implementation's helper so the test checks the shipped behavior.
func forbiddenHighlightCapture(name string) bool {
	switch name {
	case "spell", "nospell", "conceal", "none",
		"local.scope", "local.reference", "local.definition", "local.definition-value":
		return true
	}
	return strings.HasPrefix(name, "_") || strings.HasPrefix(name, "local.definition.")
}

func highlighterForEntry(t *testing.T, name string) (*gotreesitter.Highlighter, *grammars.LangEntry) {
	t.Helper()
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		t.Fatalf("grammar %q is not registered", name)
	}
	lang := entry.Language()
	if lang == nil {
		t.Fatalf("grammar %q has no language", name)
	}
	var opts []gotreesitter.HighlighterOption
	if entry.TokenSourceFactory != nil {
		factory := entry.TokenSourceFactory
		opts = append(opts, gotreesitter.WithTokenSourceFactory(func(source []byte) gotreesitter.TokenSource {
			return factory(source, lang)
		}))
	}
	h, err := gotreesitter.NewHighlighter(lang, entry.HighlightQuery, opts...)
	if err != nil {
		t.Fatalf("NewHighlighter(%s): %v", name, err)
	}
	return h, entry
}

// captureAt returns the capture that covers byte offset, or "" when no range
// covers it.
func captureAt(ranges []gotreesitter.HighlightRange, offset uint32) string {
	for _, r := range ranges {
		if r.StartByte <= offset && offset < r.EndByte {
			return r.Capture
		}
	}
	return ""
}

func describeRanges(source []byte, ranges []gotreesitter.HighlightRange) string {
	var b strings.Builder
	for _, r := range ranges {
		fmt.Fprintf(&b, "\n  %d-%d %s %q", r.StartByte, r.EndByte, r.Capture, source[r.StartByte:r.EndByte])
	}
	return b.String()
}

func requireNoForbiddenCaptures(t *testing.T, source []byte, ranges []gotreesitter.HighlightRange) {
	t.Helper()
	for _, r := range ranges {
		if forbiddenHighlightCapture(r.Capture) {
			t.Errorf("range %d-%d has capture %q, which colors nothing:%s", r.StartByte, r.EndByte, r.Capture, describeRanges(source, ranges))
			return
		}
	}
}

func requireCovered(t *testing.T, ranges []gotreesitter.HighlightRange, source []byte, from, to uint32, want string) {
	t.Helper()
	for offset := from; offset < to; offset++ {
		if got := captureAt(ranges, offset); got != want {
			t.Fatalf("byte %d (%q) has capture %q, want %q:%s", offset, source[offset], got, want, describeRanges(source, ranges))
		}
	}
}

// TestHighlightShippedQueriesNeverReturnNonHighlightCaptures runs every
// shipped highlight query that holds a capture without a color, over the
// grammar's smoke sample.
func TestHighlightShippedQueriesNeverReturnNonHighlightCaptures(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	checked := 0
	for _, entry := range grammars.AllLanguages() {
		if strings.TrimSpace(entry.HighlightQuery) == "" {
			continue
		}
		name := entry.Name
		lang := entry.Language()
		query, err := gotreesitter.NewQuery(entry.HighlightQuery, lang)
		if err != nil {
			t.Errorf("%s: compile highlight query: %v", name, err)
			continue
		}
		affected := false
		for _, capture := range query.CaptureNames() {
			if forbiddenHighlightCapture(capture) {
				affected = true
				break
			}
		}
		if !affected {
			grammars.UnloadEmbeddedLanguage(name + ".bin")
			continue
		}
		sample := []byte(grammars.ParseSmokeSample(name))
		t.Run(name, func(t *testing.T) {
			h, _ := highlighterForEntry(t, name)
			ranges := h.Highlight(sample)
			requireNoForbiddenCaptures(t, sample, ranges)
			for i, r := range ranges {
				if r.EndByte <= r.StartByte || int(r.EndByte) > len(sample) {
					t.Errorf("range %d is out of bounds: %+v", i, r)
				}
				if i > 0 && r.StartByte < ranges[i-1].EndByte {
					t.Errorf("range %d overlaps range %d: %+v, %+v", i, i-1, r, ranges[i-1])
				}
			}
		})
		checked++
		grammars.UnloadEmbeddedLanguage(name + ".bin")
	}
	if checked < 50 {
		t.Fatalf("only %d shipped queries hold non-highlight captures, want at least 50", checked)
	}
}

// vhdl captures the inner comment text as @spell inside an outer @comment. The
// spell capture used to replace the comment color (issue 454).
func TestHighlightVHDLCommentKeepsItsColor(t *testing.T) {
	h, _ := highlighterForEntry(t, "vhdl")
	source := []byte("-- hello world\nentity e is\nend entity;\n")
	ranges := h.Highlight(source)
	requireNoForbiddenCaptures(t, source, ranges)
	requireCovered(t, ranges, source, 0, uint32(len("-- hello world")), "comment")
	if got := captureAt(ranges, uint32(strings.Index(string(source), "entity"))); got != "keyword" {
		t.Fatalf("keyword entity has capture %q:%s", got, describeRanges(source, ranges))
	}
}

// desktop pairs each value pattern with a private @_key capture on the key. The
// private capture used to color the key and hide the key's real capture.
func TestHighlightPrivateCaptureDoesNotColorDesktopKeys(t *testing.T) {
	h, _ := highlighterForEntry(t, "desktop")
	source := []byte("[Desktop Entry]\nType=Application\nVersion=1.0\n")
	ranges := h.Highlight(source)
	requireNoForbiddenCaptures(t, source, ranges)
	for _, key := range []string{"Type", "Version"} {
		if got := captureAt(ranges, uint32(strings.Index(string(source), key))); got != "property" {
			t.Fatalf("key %s has capture %q, want property:%s", key, got, describeRanges(source, ranges))
		}
	}
	if got := captureAt(ranges, uint32(strings.Index(string(source), "Application"))); got != "type" {
		t.Fatalf("value Application has capture %q, want type:%s", got, describeRanges(source, ranges))
	}
}

// starlark resets the string color inside an f-string interpolation with
// (interpolation) @none. The reset must still clear the enclosing @string, but
// the highlighter must not return a "none" range.
func TestHighlightNoneResetsEnclosingStringWithoutReturningARange(t *testing.T) {
	h, _ := highlighterForEntry(t, "starlark")
	source := []byte("x = f\"a{ b }c\"\n")
	ranges := h.Highlight(source)
	requireNoForbiddenCaptures(t, source, ranges)

	open := uint32(strings.Index(string(source), "{"))
	closing := uint32(strings.Index(string(source), "}"))
	if got := captureAt(ranges, open+1); got != "" {
		t.Fatalf("space inside the interpolation has capture %q, want none:%s", got, describeRanges(source, ranges))
	}
	if got := captureAt(ranges, closing-1); got != "" {
		t.Fatalf("space inside the interpolation has capture %q, want none:%s", got, describeRanges(source, ranges))
	}
	if got := captureAt(ranges, open+2); got != "variable" {
		t.Fatalf("interpolated name has capture %q, want variable:%s", got, describeRanges(source, ranges))
	}
	if got := captureAt(ranges, 5); got != "string" {
		t.Fatalf("string text before the interpolation has capture %q, want string:%s", got, describeRanges(source, ranges))
	}
	if got := captureAt(ranges, closing+1); got != "string" {
		t.Fatalf("string text after the interpolation has capture %q, want string:%s", got, describeRanges(source, ranges))
	}
}

// markdown resets fenced code with (code_fence_content) @none.
func TestHighlightMarkdownFenceContentIsResetNotReturned(t *testing.T) {
	h, _ := highlighterForEntry(t, "markdown")
	source := []byte("# T\n\n```foo\ncode here\n```\n")
	ranges := h.Highlight(source)
	requireNoForbiddenCaptures(t, source, ranges)
	start := uint32(strings.Index(string(source), "code here"))
	for offset := start; offset < start+uint32(len("code here")); offset++ {
		if got := captureAt(ranges, offset); got != "" {
			t.Fatalf("fence content byte %d has capture %q, want none:%s", offset, got, describeRanges(source, ranges))
		}
	}
}

// A fenced block in a language with an inner @spell capture shows the child's
// comment color. The host's (code_fence_content) @none covers the same span as
// the child's @comment; the child layer must win.
func TestHighlightMarkdownFenceInjectsChildWithCapturePolicy(t *testing.T) {
	h, _ := highlighterForEntry(t, "markdown")
	source := []byte("# T\n\n```vhdl\n-- note\n```\n")
	ranges := h.Highlight(source)
	requireNoForbiddenCaptures(t, source, ranges)
	start := uint32(strings.Index(string(source), "-- note"))
	requireCovered(t, ranges, source, start, start+uint32(len("-- note")), "comment")
}

// kdl marks a commented-out node with (#set! "priority" 105) so that its
// comment color covers the node's inner captures.
func TestHighlightKDLCommentedNodeHonorsPriority(t *testing.T) {
	h, _ := highlighterForEntry(t, "kdl")
	source := []byte("/-node 1 key=\"v\"\nother 2\n")
	ranges := h.Highlight(source)
	requireNoForbiddenCaptures(t, source, ranges)
	commented := uint32(strings.Index(string(source), "other"))
	requireCovered(t, ranges, source, 0, commented, "comment")
	if got := captureAt(ranges, commented); got != "variable" {
		t.Fatalf("node name after the commented node has capture %q, want variable:%s", got, describeRanges(source, ranges))
	}
}

// elm names real highlight captures local.function.elm. Only the locals-query
// vocabulary is dropped, so these stay.
func TestHighlightElmKeepsLocalFunctionCapture(t *testing.T) {
	h, _ := highlighterForEntry(t, "elm")
	source := []byte("module M exposing (..)\n\nf x =\n    x\n")
	ranges := h.Highlight(source)
	if got := captureAt(ranges, uint32(strings.Index(string(source), "x ="))); got != "local.function.elm" {
		t.Fatalf("parameter x has capture %q, want local.function.elm:%s", got, describeRanges(source, ranges))
	}
}
