package grammars_test

import (
	"fmt"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// Helpers shared by the incremental-versus-fresh regression tests in this
// package.

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
