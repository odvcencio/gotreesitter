//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammargen"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func generatedYAMLCore(t *testing.T) (*gts.Language, *sitter.Parser) {
	t.Helper()
	_, blob, err := grammargen.GenerateLanguageAndBlob(grammargen.YAMLCoreGrammar())
	if err != nil {
		t.Fatal(err)
	}
	lang, err := gts.LoadLanguage(blob)
	if err != nil {
		t.Fatal(err)
	}
	if !grammars.AdaptScannerForLanguage("yaml", lang) {
		t.Fatal("failed to attach YAML scanner to generated tables")
	}
	oracle, err := ParityCLanguage("yaml")
	if err != nil {
		t.Fatal(err)
	}
	c := sitter.NewParser()
	t.Cleanup(c.Close)
	if err := c.SetLanguage(oracle); err != nil {
		t.Fatal(err)
	}
	return lang, c
}

func TestGeneratedYAMLSequenceSpansMatchPinnedC(t *testing.T) {
	lang, c := generatedYAMLCore(t)

	for _, test := range []struct {
		name, header, item string
	}{
		{"root", "", "- a\n"},
		{"after comment", "# header\n", "- a\n"},
		{"under a key", "root:\n", "  - a\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			core := ccoreNew(lang)
			defer core.ReleaseLast()
			src := []byte(test.header + strings.Repeat(test.item, 6))
			if stop := core.Parse(src, nil); stop != "" {
				t.Fatalf("ccore parse stopped: %q", stop)
			}
			reference := c.Parse(src, nil)
			if reference == nil {
				t.Fatal("pinned C returned no tree")
			}
			defer reference.Close()
			root := reference.RootNode()
			if root.HasError() || root.EndByte() != uint(len(src)) {
				t.Fatal("expected the YAML sequence to parse cleanly and cover the source")
			}
			if got, want := core.Digest(), ccoreCDigestChanges(reference, false); got != want {
				t.Fatalf("sequence item spans differ from pinned C: digest %s, want %s", got, want)
			}
		})
	}
}

func TestGeneratedYAMLCommentRecoveryMatchesPinnedC(t *testing.T) {
	lang, c := generatedYAMLCore(t)
	const base = "a:\n  # one\n  # c\n  b: 1\n"
	const changed = "a:\n  x# one\n  # c\n  b: 1\n"
	for _, tc := range []struct {
		name, source string
		hasError     bool
	}{
		{"comment", base, false},
		{"plain scalar before comment", changed, true},
		{"dedented scalar before comment", "a:\n  x# one\nx # d\n  b: 1\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := []byte(tc.source)
			reference := c.Parse(source, nil)
			if reference == nil {
				t.Fatal("pinned C returned no tree")
			}
			defer reference.Close()
			root := reference.RootNode()
			if root.Kind() != "stream" || root.HasError() != tc.hasError || root.EndByte() != uint(len(source)) {
				t.Fatal("pinned C disagrees with the witness")
			}
			core := ccoreNew(lang)
			defer core.ReleaseLast()
			if stop := core.Parse(source, nil); stop != "" {
				t.Fatalf("ccore parse stopped: %q", stop)
			}
			if got, want := core.Digest(), ccoreCDigestChanges(reference, false); got != want {
				t.Fatalf("YAML comment recovery differs from pinned C: digest %s, want %s", got, want)
			}
		})
	}
}
