//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	sitter "github.com/tree-sitter/go-tree-sitter"
	"testing"
)

func TestCcoreRustBlockCommentEOFAndLowByte(t *testing.T) {
	lang, cl := ccoreLanguages(t, "rust")
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	core, err := bindtest.NewParser(lang)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, source string }{
		{"empty", ""}, {"open", "/*"}, {"inner", "/*!"}, {"outer", "/**"},
		{"nul", "/*a\x00"}, {"unicode_low_star", "/*a\u012a/"},
		{"unicode_low_slash", "/*a\u012f*end*/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.source)
			ct := cp.Parse(src, nil)
			if ct == nil {
				t.Fatal("C returned nil tree")
			}
			defer ct.Close()
			if stop := core.Parse(src, nil); stop != "" {
				t.Fatalf("core stop: %s", stop)
			}
			defer core.ReleaseLast()
			if core.Digest() != ccoreCDigest(ct) {
				t.Fatalf("core differs from C for %q", src)
			}
			if int(core.RootEnd()) != len(src) {
				t.Fatalf("root covers %d of %d", core.RootEnd(), len(src))
			}
		})
	}
}
