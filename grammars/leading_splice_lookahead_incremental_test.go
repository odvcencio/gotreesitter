package grammars_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestLeadingSpliceKeepsItemWhoseLookaheadWasEdited covers issue #454's
// report that the leading top-level splice reused an item whose extent was
// decided by the token after it. In Go, "func g(a int) " is reduced when the
// parser sees the newline that follows it. Typing "i" before that newline
// makes "i" the result type, so g must be parsed again. The old splice kept g
// because g ends before the edit, which produced a tree with no error that a
// fresh parse does not build.
func TestLeadingSpliceKeepsItemWhoseLookaheadWasEdited(t *testing.T) {
	cases := []struct {
		name   string
		lang   string
		before string
		after  string
	}{
		{name: "result type typed after a space", lang: "go", before: "package main\n\nfunc g(a int) \n\nfunc f() {}\n", after: "package main\n\nfunc g(a int) i\n\nfunc f() {}\n"},
		{name: "result type typed after the parameters", lang: "go", before: "package main\n\nfunc g(a int)\n\nfunc f() {}\n", after: "package main\n\nfunc g(a int)i\n\nfunc f() {}\n"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.lang+"/"+tc.name, func(t *testing.T) {
			entry := grammars.DetectLanguageByName(tc.lang)
			if entry == nil || entry.Language() == nil {
				t.Skipf("language %q unavailable in this build", tc.lang)
			}
			lang := entry.Language()
			before, after := []byte(tc.before), []byte(tc.after)

			old, err := gotreesitter.NewParser(lang).Parse(before)
			if err != nil {
				t.Fatalf("parse before: %v", err)
			}
			old.Edit(spliceTestEdit(before, after))
			incremental, err := gotreesitter.NewParser(lang).ParseIncremental(after, old)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			fresh, err := gotreesitter.NewParser(lang).Parse(after)
			if err != nil {
				t.Fatalf("fresh parse: %v", err)
			}
			if diff := spliceTreeDiff(incremental.RootNode(), fresh.RootNode(), lang, ""); diff != "" {
				t.Fatalf("incremental tree differs from a fresh parse at %s\nincremental: %s\nfresh:       %s",
					diff, incremental.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang))
			}
		})
	}
}
