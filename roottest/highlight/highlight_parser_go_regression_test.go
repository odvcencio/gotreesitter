package highlight_test

import (
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestHighlightParserGoRealFile(t *testing.T) {
	src, err := os.ReadFile("../../parser.go")
	if err != nil {
		t.Fatalf("read parser.go: %v", err)
	}

	entry := grammars.DetectLanguage("parser.go")
	if entry == nil {
		t.Fatal("DetectLanguage(parser.go) returned nil")
	}
	lang := entry.Language()
	if lang == nil {
		t.Fatal("go language is nil")
	}

	parser := gotreesitter.NewParser(lang)
	var tree *gotreesitter.Tree
	if entry.TokenSourceFactory != nil {
		// ts2go Go blob path — custom lexer registered.
		tree, err = parser.ParseWithTokenSource(src, entry.TokenSourceFactory(src, lang))
	} else {
		// grammargen Go blob path (default in 0.14.0+). The baked DFA parses
		// Go without a custom lexer.
		tree, err = parser.Parse(src)
	}
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse root is nil")
	}
	if got, want := tree.ParseStopReason(), gotreesitter.ParseStopAccepted; got != want {
		t.Fatalf("parse stop reason = %q, want %q (runtime=%s)", got, want, tree.ParseRuntime().Summary())
	}
	if root.HasError() {
		t.Fatalf("parse root has error=true (runtime=%s)", tree.ParseRuntime().Summary())
	}

	hlOpts := []gotreesitter.HighlighterOption{}
	if entry.TokenSourceFactory != nil {
		hlOpts = append(hlOpts, gotreesitter.WithTokenSourceFactory(func(source []byte) gotreesitter.TokenSource {
			return entry.TokenSourceFactory(source, lang)
		}))
	}
	hl, err := gotreesitter.NewHighlighter(lang, entry.HighlightQuery, hlOpts...)
	if err != nil {
		t.Fatalf("NewHighlighter: %v", err)
	}

	ranges := hl.Highlight(src)
	if len(ranges) == 0 {
		t.Fatal("Highlight(parser.go) returned 0 ranges")
	}

	foundPackageKeyword := false
	for _, r := range ranges {
		if r.Capture == "keyword" && string(src[r.StartByte:r.EndByte]) == "package" {
			foundPackageKeyword = true
			break
		}
	}
	if !foundPackageKeyword {
		t.Fatal("missing keyword capture for package token in parser.go")
	}
}
