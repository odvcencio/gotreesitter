package grammars

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// TestDjotHighlightOffsetShrinksCheckedRange verifies that Djot's embedded
// highlight query, which uses the #offset! directive to colorize just the
// "x" inside a checked task-list marker "[x]", now produces the intended
// adjusted range now that gotreesitter applies #offset!.
//
// Sample: "- [x] done\n" parses a `checked` node spanning "[x]" (bytes
// [2,5)); the highlight query's
// `(#offset! @constant.builtin 0 1 0 -1)` should shrink the reported
// constant.builtin range to just the "x" (bytes [3,4)).
func TestDjotHighlightOffsetShrinksCheckedRange(t *testing.T) {
	entry := DetectLanguageByName("djot")
	if entry == nil {
		t.Skip("Djot grammar not available")
	}
	lang := entry.Language()
	if lang == nil {
		t.Skip("Djot language failed to load")
	}

	h, err := gotreesitter.NewHighlighter(lang, entry.HighlightQuery)
	if err != nil {
		t.Fatalf("NewHighlighter: %v", err)
	}

	source := []byte("- [x] done\n")
	ranges := h.Highlight(source)

	var found bool
	for _, r := range ranges {
		if r.Capture != "constant.builtin" {
			continue
		}
		found = true
		if r.StartByte != 3 || r.EndByte != 4 {
			t.Errorf("constant.builtin range = [%d,%d), want [3,4)", r.StartByte, r.EndByte)
		}
		if got, want := string(source[r.StartByte:r.EndByte]), "x"; got != want {
			t.Errorf("constant.builtin text = %q, want %q", got, want)
		}
	}
	if !found {
		t.Fatalf("expected a constant.builtin highlight range in %+v", ranges)
	}
}
