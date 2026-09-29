package grammars_test

import (
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	grammarruntime "github.com/odvcencio/gotreesitter/grammars/runtime"
)

// TestCTokenSourceDirectiveReuseMatchesFresh covers incremental parses through
// the registered C and C++ TokenSource where reuse ends inside a preprocessor
// directive line. The token source tracks directive state (for example "the
// next newline ends this directive"). Resuming it after a reused token in the
// middle of a directive line lost that state, so the directive's closing
// newline was skipped and the incremental tree differed from a fresh parse.
// The end-of-file cases are the ones reported in issue #454; the first case is
// minimized from a grammar receipt edit session.
func TestCTokenSourceDirectiveReuseMatchesFresh(t *testing.T) {
	cases := []struct {
		name   string
		lang   string
		before string
		after  string
	}{
		{name: "directive inside block, insert before", lang: "cpp", before: " f{#T\n}", after: "x f{#T\n}"},
		{name: "define then append space", lang: "cpp", before: "#define N 1\n\nint f() { return N; }\n", after: "#define N 1\n\nint f() { return N; }\n "},
		{name: "define then append space", lang: "c", before: "#define N 1\n\nint f() { return N; }\n", after: "#define N 1\n\nint f() { return N; }\n "},
		{name: "define then append function", lang: "cpp", before: "#define N 1\n\nint f() { return N; }\n", after: "#define N 1\n\nint f() { return N; }\nint g() { return 1; }\n"},
		{name: "define then append function", lang: "c", before: "#define N 1\n\nint f() { return N; }\n", after: "#define N 1\n\nint f() { return N; }\nint g() { return 1; }\n"},
		{name: "include then append space", lang: "cpp", before: "#include <iostream>\n\nint f() { return N; }\n", after: "#include <iostream>\n\nint f() { return N; }\n "},
		{name: "include then append space", lang: "c", before: "#include <iostream>\n\nint f() { return N; }\n", after: "#include <iostream>\n\nint f() { return N; }\n "},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.lang+"/"+tc.name, func(t *testing.T) {
			entry := grammars.DetectLanguageByName(tc.lang)
			if entry == nil || entry.Language() == nil {
				t.Skipf("language %q unavailable in this build", tc.lang)
			}
			factory := grammarruntime.TokenSourceFactory(tc.lang)
			if factory == nil {
				t.Skipf("no registered TokenSource for %q", tc.lang)
			}
			lang := entry.Language()
			before, after := []byte(tc.before), []byte(tc.after)
			edit := directiveTestEdit(before, after)

			old, err := gotreesitter.NewParser(lang).ParseWithTokenSource(before, factory(before, lang))
			if err != nil {
				t.Fatalf("parse before: %v", err)
			}
			old.Edit(edit)
			incremental, err := gotreesitter.NewParser(lang).ParseIncrementalWithTokenSource(after, old, factory(after, lang))
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			fresh, err := gotreesitter.NewParser(lang).ParseWithTokenSource(after, factory(after, lang))
			if err != nil {
				t.Fatalf("fresh parse: %v", err)
			}
			if diff := directiveTreeDiff(incremental.RootNode(), fresh.RootNode(), lang, ""); diff != "" {
				t.Fatalf("incremental tree differs from a fresh parse at %s\nincremental: %s\nfresh:       %s",
					diff, incremental.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang))
			}
		})
	}
}

// directiveTestEdit returns the single contiguous edit from before to after.
func directiveTestEdit(before, after []byte) gotreesitter.InputEdit {
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
		StartPoint: directiveTestPoint(before, start), OldEndPoint: directiveTestPoint(before, oldEnd),
		NewEndPoint: directiveTestPoint(after, newEnd),
	}
}

func directiveTestPoint(src []byte, offset int) gotreesitter.Point {
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

// directiveTreeDiff compares node type, byte span, error and missing flags,
// child count, and field names in pre-order. It returns "" when equal.
func directiveTreeDiff(a, b *gotreesitter.Node, lang *gotreesitter.Language, path string) string {
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
		if diff := directiveTreeDiff(a.Child(i), b.Child(i), lang, fmt.Sprintf("%s[%d]", here, i)); diff != "" {
			return diff
		}
	}
	return ""
}
