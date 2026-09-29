package grammars_test

import (
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestHTMLIncrementalAttributeQuoteReplaceMatchesFresh(t *testing.T) {
	entry := grammars.DetectLanguageByName("html")
	if entry == nil || entry.Language() == nil {
		t.Fatal("html language unavailable")
	}
	lang := entry.Language()
	before, after := []byte(`<meta=">`), []byte(`<meta=x>`)
	// The candidate route exposes the fresh/incremental recovery mismatch;
	// pin it so this regression does not depend on the process default.
	beforeParser := gotreesitter.NewParser(lang)
	beforeParser.SetAdmissionCandidateRoute(true)
	old, err := beforeParser.Parse(before)
	if err != nil {
		t.Fatalf("Parse before: %v", err)
	}
	defer old.Release()
	old.Edit(spliceTestEdit(before, after))
	incParser := gotreesitter.NewParser(lang)
	incParser.SetAdmissionCandidateRoute(true)
	inc, err := incParser.ParseIncremental(after, old)
	if err != nil {
		t.Fatalf("ParseIncremental: %v", err)
	}
	defer inc.Release()
	freshParser := gotreesitter.NewParser(lang)
	freshParser.SetAdmissionCandidateRoute(true)
	fresh, err := freshParser.Parse(after)
	if err != nil {
		t.Fatalf("Parse after: %v", err)
	}
	defer fresh.Release()
	if !fresh.RootNode().HasError() {
		t.Fatalf("fresh root has no error: %s", fresh.RootNode().SExpr(lang))
	}
	if got, want := fresh.RootNode().SExpr(lang), "(document (element (start_tag (tag_name) (ERROR (ERROR)))))"; got != want {
		t.Fatalf("fresh tree = %s, want %s", got, want)
	}
	if diff := spliceTreeDiff(inc.RootNode(), fresh.RootNode(), lang, ""); diff != "" {
		t.Fatalf("incremental differs from fresh at %s\nincremental: %s\nfresh: %s", diff, inc.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang))
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
