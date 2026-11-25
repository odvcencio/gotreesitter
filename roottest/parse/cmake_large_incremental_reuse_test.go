package parse_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

const cmakeLargeDocSize = 600 << 10

// cmakeLargeDoc builds a clean CMake document. Block 2 carries a bracket
// comment body over 4 KiB, and a second one straddles the 512 KiB offset.
func cmakeLargeDoc(target int) []byte {
	var b strings.Builder
	b.WriteString("cmake_minimum_required(VERSION 3.10)\nproject(demo)\n")
	straddled := false
	for i := 0; b.Len() < target; i++ {
		fmt.Fprintf(&b, "set(VAR_%d \"value %d\")\nif(VAR_%d)\n  message(STATUS \"x %d\")\nendif()\n", i, i, i, i)
		fmt.Fprintf(&b, "message([=[ arg %d ]=])\n#[==[ note %d ]==]\n# [[ line %d\n", i, i, i)
		if i == 2 {
			fmt.Fprintf(&b, "#[==[ LONG1 %s ]==]\n", strings.Repeat("body text ", 500))
		}
		if !straddled && b.Len() > 512<<10-2500 {
			straddled = true
			fmt.Fprintf(&b, "#[==[ LONG2 %s ]==]\n", strings.Repeat("body text ", 500))
		}
		fmt.Fprintf(&b, "function(fn_%d arg)\n  message(${arg})\nendfunction()\n", i)
	}
	return []byte(b.String())
}

func cmakeOffsetPoint(src []byte, off int) gotreesitter.Point {
	var p gotreesitter.Point
	for _, c := range src[:off] {
		if c == '\n' {
			p.Row++
			p.Column = 0
		} else {
			p.Column++
		}
	}
	return p
}

func cmakeTreesEqual(t *testing.T, lang *gotreesitter.Language, a, b *gotreesitter.Node) {
	t.Helper()
	type pair struct{ a, b *gotreesitter.Node }
	stack := []pair{{a, b}}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		x, y := p.a, p.b
		if x.Symbol() != y.Symbol() || x.StartByte() != y.StartByte() || x.EndByte() != y.EndByte() ||
			x.StartPoint() != y.StartPoint() || x.EndPoint() != y.EndPoint() ||
			x.IsNamed() != y.IsNamed() || x.IsExtra() != y.IsExtra() || x.IsMissing() != y.IsMissing() ||
			x.IsError() != y.IsError() || x.HasError() != y.HasError() || x.ChildCount() != y.ChildCount() {
			t.Fatalf("trees differ at %s [%d,%d) vs %s [%d,%d)", x.Type(lang), x.StartByte(), x.EndByte(), y.Type(lang), y.StartByte(), y.EndByte())
		}
		for i := x.ChildCount() - 1; i >= 0; i-- {
			if x.FieldNameForChild(i, lang) != y.FieldNameForChild(i, lang) {
				t.Fatalf("field name differs at child %d of %s [%d,%d)", i, x.Type(lang), x.StartByte(), x.EndByte())
			}
			stack = append(stack, pair{x.Child(i), y.Child(i)})
		}
	}
}

type cmakeEdit struct {
	at, del int
	ins     string
}

// cmakeEditAfter returns an edit at the offset of needle plus delta.
func cmakeEditAfter(src []byte, needle string, delta, del int, ins string) cmakeEdit {
	i := bytes.Index(src, []byte(needle))
	if i < 0 {
		panic("missing needle " + needle)
	}
	return cmakeEdit{i + delta, del, ins}
}

// cmakeApply edits old, reparses incrementally, and compares the result with a
// fresh parse. A clean result must reuse bytes unless allowFresh is set.
func cmakeApply(t *testing.T, allowFresh bool, parser *gotreesitter.Parser, lang *gotreesitter.Language, old *gotreesitter.Tree, src []byte, e cmakeEdit) (*gotreesitter.Tree, []byte, gotreesitter.IncrementalParseProfile) {
	t.Helper()
	edited := append(append(append([]byte{}, src[:e.at]...), e.ins...), src[e.at+e.del:]...)
	old.Edit(gotreesitter.InputEdit{
		StartByte: uint32(e.at), OldEndByte: uint32(e.at + e.del), NewEndByte: uint32(e.at + len(e.ins)),
		StartPoint: cmakeOffsetPoint(src, e.at), OldEndPoint: cmakeOffsetPoint(src, e.at+e.del),
		NewEndPoint: cmakeOffsetPoint(edited, e.at+len(e.ins)),
	})
	got, prof, err := parser.ParseIncrementalProfiled(edited, old)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := gotreesitter.NewParser(lang).Parse(edited)
	if err != nil {
		got.Release()
		t.Fatal(err)
	}
	defer fresh.Release()
	cmakeTreesEqual(t, lang, got.RootNode(), fresh.RootNode())
	if !allowFresh && !got.RootNode().HasError() && prof.ReusedBytes == 0 {
		t.Fatalf("clean result reused 0 bytes (unsupported=%v reason=%q new=%d)", prof.ReuseUnsupported, prof.ReuseUnsupportedReason, prof.NewNodesAllocated)
	}
	t.Logf("hasError=%v reused=%d of %d new=%d", got.RootNode().HasError(), prof.ReusedBytes, len(edited), prof.NewNodesAllocated)
	return got, edited, prof
}

// TestCMakeLargeIncrementalReuse guards reuse for documents above 512 KiB.
// CMake's scanner keeps its whole state in the checkpoint and reads only
// through lexer operations, so an edit must reuse the old tree and still equal
// a fresh parse, including edits that change bracket levels.
func TestCMakeLargeIncrementalReuse(t *testing.T) {
	lang := grammars.CmakeLanguage()
	src := cmakeLargeDoc(cmakeLargeDocSize)
	if len(src) <= 512<<10 {
		t.Fatalf("document is %d bytes, want over 512 KiB", len(src))
	}
	straddle := bytes.Index(src, []byte("LONG2"))
	if straddle > 512<<10 || straddle+5000 < 512<<10 {
		t.Fatalf("LONG2 block at %d does not straddle 512 KiB", straddle)
	}
	cases := []struct {
		name string
		edit cmakeEdit
		// minReuseFrac is the share of bytes the case must reuse. Scanner
		// states inside bracket constructs reject reuse at some boundaries, so
		// this document reuses about 90% where plain commands reuse over 97%.
		minReuseFrac int
		// allowFresh accepts a clean result that falls back to a fresh parse.
		allowFresh bool
	}{
		{"identifier", cmakeEditAfter(src, "VAR_3", 4, 0, "x"), 85, false},
		{"argument close widened", cmakeEditAfter(src, "arg 3 ]=]", 7, 0, "="), 0, false},
		{"comment close narrowed", cmakeEditAfter(src, "note 3 ]==]", 8, 1, ""), 0, false},
		{"argument open widened", cmakeEditAfter(src, "message([=[ arg 3", 10, 0, "="), 0, false},
		{"insert opener before argument", cmakeEditAfter(src, "message(${arg})", 8, 0, "[["), 0, false},
		// The edit starts where the previous token ends, so reuse is not
		// certified (incremental.go, unprovenReuse at the boundary-edit check).
		{"delete opener", cmakeEditAfter(src, "message([=[ arg 3", 8, 3, ""), 0, true},
		{"line comment to bracket comment", cmakeEditAfter(src, "# [[ line 3", 1, 1, ""), 0, false},
		{"long body", cmakeEditAfter(src, "LONG1", 2500, 0, "z"), 0, false},
		{"long body across 512 KiB", cmakeEditAfter(src, "LONG2", 2500, 0, "z"), 0, false},
		{"near end", func() cmakeEdit {
			i := bytes.LastIndex(src, []byte("fn_"))
			return cmakeEdit{i + 3, 0, "x"}
		}(), 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parser := gotreesitter.NewParser(lang)
			old, err := parser.Parse(src)
			if err != nil {
				t.Fatal(err)
			}
			defer old.Release()
			if old.RootNode().HasError() {
				t.Fatal("synthetic document does not parse cleanly")
			}
			got, edited, prof := cmakeApply(t, tc.allowFresh, parser, lang, old, src, tc.edit)
			defer got.Release()
			if tc.minReuseFrac > 0 && prof.ReusedBytes*100 < uint64(len(edited))*uint64(tc.minReuseFrac) {
				t.Fatalf("reused %d of %d bytes, want over %d%%", prof.ReusedBytes, len(edited), tc.minReuseFrac)
			}
		})
	}
	t.Run("two-step session", func(t *testing.T) {
		parser := gotreesitter.NewParser(lang)
		old, err := parser.Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		first, edited, _ := cmakeApply(t, false, parser, lang, old, src, cmakeEditAfter(src, "VAR_3", 4, 0, "x"))
		old.Release()
		second, _, prof := cmakeApply(t, false, parser, lang, first, edited, cmakeEditAfter(edited, "note 40 ]==]", 9, 0, "="))
		first.Release()
		second.Release()
		_ = prof
	})
}
