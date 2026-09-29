package grammars_test

import (
	"fmt"
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

// spliceTestEdit returns the single contiguous edit from before to after.
func spliceTestEdit(before, after []byte) gotreesitter.InputEdit {
	start := 0
	for start < len(before) && start < len(after) && before[start] == after[start] {
		start++
	}
	suffix := 0
	for suffix < len(before)-start && suffix < len(after)-start && before[len(before)-1-suffix] == after[len(after)-1-suffix] {
		suffix++
	}
	oldEnd, newEnd := len(before)-suffix, len(after)-suffix
	return gotreesitter.InputEdit{
		StartByte: uint32(start), OldEndByte: uint32(oldEnd), NewEndByte: uint32(newEnd),
		StartPoint: spliceTestPoint(before, start), OldEndPoint: spliceTestPoint(before, oldEnd),
		NewEndPoint: spliceTestPoint(after, newEnd),
	}
}

func spliceTestPoint(src []byte, offset int) gotreesitter.Point {
	var pt gotreesitter.Point
	for _, b := range src[:offset] {
		if b == '\n' {
			pt.Row++
			pt.Column = 0
		} else {
			pt.Column++
		}
	}
	return pt
}

// spliceTreeDiff compares node type, byte span, error and missing flags,
// child count, and field names in pre-order. It returns "" when equal.
func spliceTreeDiff(a, b *gotreesitter.Node, lang *gotreesitter.Language, path string) string {
	if a == nil || b == nil {
		if a == nil && b == nil {
			return ""
		}
		return fmt.Sprintf("%s: nil incremental=%v fresh=%v", path, a == nil, b == nil)
	}
	here := path + "/" + a.Type(lang)
	switch {
	case a.Type(lang) != b.Type(lang):
		return fmt.Sprintf("%s: type %s vs %s", here, a.Type(lang), b.Type(lang))
	case a.StartByte() != b.StartByte() || a.EndByte() != b.EndByte():
		return fmt.Sprintf("%s: span %d..%d vs %d..%d", here, a.StartByte(), a.EndByte(), b.StartByte(), b.EndByte())
	case a.HasError() != b.HasError():
		return fmt.Sprintf("%s: has_error %v vs %v", here, a.HasError(), b.HasError())
	case a.IsMissing() != b.IsMissing():
		return fmt.Sprintf("%s: missing %v vs %v", here, a.IsMissing(), b.IsMissing())
	case a.ChildCount() != b.ChildCount():
		return fmt.Sprintf("%s: children %d vs %d", here, a.ChildCount(), b.ChildCount())
	}
	for i := 0; i < a.ChildCount(); i++ {
		fa, fb := a.FieldNameForChild(i, lang), b.FieldNameForChild(i, lang)
		if fa != fb {
			return fmt.Sprintf("%s[%d]: field %q vs %q", here, i, fa, fb)
		}
		if diff := spliceTreeDiff(a.Child(i), b.Child(i), lang, fmt.Sprintf("%s[%d]", here, i)); diff != "" {
			return diff
		}
	}
	return ""
}
