package highlight_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestHighlightMarkdownFencedGoForKeyword(t *testing.T) {
	src := []byte("# Sample\n\n```go\nfor i := 0; i < 3; i++ {\n\tprintln(i)\n}\n```\n")

	entry := grammars.DetectLanguage("sample.md")
	if entry == nil {
		t.Fatal("DetectLanguage(sample.md) returned nil")
	}
	lang := entry.Language()
	if lang == nil {
		t.Fatal("markdown language is nil")
	}

	hl, err := gotreesitter.NewHighlighter(lang, entry.HighlightQuery)
	if err != nil {
		t.Fatalf("NewHighlighter(markdown): %v", err)
	}

	ranges := hl.Highlight(src)
	if len(ranges) == 0 {
		t.Fatal("Highlight(markdown) returned 0 ranges")
	}

	foundForKeyword := false
	for _, r := range ranges {
		if r.Capture == "keyword" && string(src[r.StartByte:r.EndByte]) == "for" {
			foundForKeyword = true
			break
		}
	}
	if !foundForKeyword {
		t.Fatal("expected fenced Go 'for' token to be highlighted as keyword")
	}
}
