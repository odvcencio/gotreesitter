package grammars

import (
	"reflect"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// Languages with a registered token source (grammars/runtime/token_source_factory_builtin.go).
var entryTokenSourceLanguages = []string{"authzed", "c", "cpp", "java"}

func entryForTest(t *testing.T, name string) *LangEntry {
	t.Helper()
	entry := DetectLanguageByName(name)
	if entry == nil || entry.Language == nil || entry.Language() == nil {
		t.Skipf("language %q unavailable in this build", name)
	}
	return entry
}

// TestLangEntryParseMatchesParseFile checks that entry.Parse makes the same
// choice of lexer as ParseFile, for each language with a registered token source.
func TestLangEntryParseMatchesParseFile(t *testing.T) {
	for _, name := range entryTokenSourceLanguages {
		t.Run(name, func(t *testing.T) {
			entry := entryForTest(t, name)
			if entry.TokenSourceFactory == nil {
				t.Fatalf("%s: expected a registered token source", name)
			}
			src := []byte(ParseSmokeSample(name))
			got, err := entry.Parse(src)
			if err != nil {
				t.Fatalf("entry.Parse: %v", err)
			}
			defer got.Release()
			want, err := ParseFile("sample"+entry.Extensions[0], src)
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			defer want.Release()
			lang := entry.Language()
			if g, w := got.RootNode().SExpr(lang), want.RootNode().SExpr(lang); g != w {
				t.Errorf("entry.Parse differs from ParseFile\n got: %s\nwant: %s", g, w)
			}
		})
	}
}

// TestLangEntryParseUsesDFAWithoutTokenSource checks the fallback: with no
// factory, entry.Parse equals a plain parser's Parse.
func TestLangEntryParseUsesDFAWithoutTokenSource(t *testing.T) {
	entry := entryForTest(t, "python")
	if entry.TokenSourceFactory != nil {
		t.Skip("python gained a registered token source")
	}
	src := []byte(ParseSmokeSample("python"))
	got, err := entry.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Release()
	want, err := entry.NewParser().Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	defer want.Release()
	lang := entry.Language()
	if g, w := got.RootNode().SExpr(lang), want.RootNode().SExpr(lang); g != w {
		t.Errorf("entry.Parse differs from the DFA parse\n got: %s\nwant: %s", g, w)
	}
}

// TestLangEntryNewHighlighterUsesTokenSource builds the highlighter by hand
// with and without the factory and compares ranges with the helper's.
func TestLangEntryNewHighlighterUsesTokenSource(t *testing.T) {
	for _, name := range []string{"json", "python"} {
		t.Run(name, func(t *testing.T) {
			entry := entryForTest(t, name)
			if entry.HighlightQuery == "" {
				t.Skipf("%s has no highlight query", name)
			}
			lang := entry.Language()
			var opts []gotreesitter.HighlighterOption
			if factory := entry.TokenSourceFactory; factory != nil {
				opts = append(opts, gotreesitter.WithTokenSourceFactory(func(src []byte) gotreesitter.TokenSource {
					return factory(src, lang)
				}))
			}
			want, err := gotreesitter.NewHighlighter(lang, entry.HighlightQuery, opts...)
			if err != nil {
				t.Fatal(err)
			}
			got, err := entry.NewHighlighter()
			if err != nil {
				t.Fatal(err)
			}
			src := []byte(ParseSmokeSample(name))
			gotRanges, wantRanges := got.Highlight(src), want.Highlight(src)
			if len(wantRanges) == 0 {
				t.Fatalf("%s: sample produced no highlight ranges", name)
			}
			if !reflect.DeepEqual(gotRanges, wantRanges) {
				t.Errorf("entry.NewHighlighter ranges differ\n got: %v\nwant: %v", gotRanges, wantRanges)
			}
		})
	}
}

func TestLangEntryWithoutLoaderReturnsError(t *testing.T) {
	var entry LangEntry
	if _, err := entry.Parse([]byte("x")); err == nil {
		t.Error("Parse on an entry without a loader returned no error")
	}
	if _, err := entry.NewHighlighter(); err == nil {
		t.Error("NewHighlighter on an entry without a loader returned no error")
	}
}
