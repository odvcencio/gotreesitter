package parse_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func cmakeLargeDoc(target int) []byte {
	var b strings.Builder
	b.WriteString("cmake_minimum_required(VERSION 3.10)\nproject(demo)\n")
	for i := 0; b.Len() < target; i++ {
		fmt.Fprintf(&b, "set(VAR_%d \"value %d\")\nif(VAR_%d)\n  message(STATUS \"x %d\")\nendif()\n", i, i, i, i)
		fmt.Fprintf(&b, "#[==[ note %d ]] still ]=] comment ]==]\nfunction(fn_%d arg)\n  message([=[ %d ]]=] ${arg})\nendfunction()\n", i, i, i)
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
		if p.a.Symbol() != p.b.Symbol() || p.a.StartByte() != p.b.StartByte() || p.a.EndByte() != p.b.EndByte() ||
			p.a.IsNamed() != p.b.IsNamed() || p.a.IsExtra() != p.b.IsExtra() || p.a.IsMissing() != p.b.IsMissing() ||
			p.a.HasError() != p.b.HasError() || p.a.ChildCount() != p.b.ChildCount() {
			t.Fatalf("trees differ at %s [%d,%d) vs %s [%d,%d)", p.a.Type(lang), p.a.StartByte(), p.a.EndByte(), p.b.Type(lang), p.b.StartByte(), p.b.EndByte())
		}
		for i := p.a.ChildCount() - 1; i >= 0; i-- {
			stack = append(stack, pair{p.a.Child(i), p.b.Child(i)})
		}
	}
}

// TestCMakeLargeIncrementalReuse guards reuse for documents at or above
// 512 KiB. CMake's scanner keeps its whole state in the checkpoint and reads
// only through lexer operations, so an edit must reuse the old tree and still
// equal a fresh parse.
func TestCMakeLargeIncrementalReuse(t *testing.T) {
	lang := grammars.CmakeLanguage()
	src := cmakeLargeDoc(1 << 20)
	cases := []struct {
		name      string
		needle    string
		delta     int
		insert    byte
		wantReuse bool
	}{
		{"identifier", "VAR_3", 4, 'x', true},
		{"bracket comment body", "note 3", 5, 'z', false},
		{"bracket comment close", "still ]=] comment ]==]", 7, '=', false},
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
			at := strings.Index(string(src), tc.needle) + tc.delta
			edited := append(append(append([]byte{}, src[:at]...), tc.insert), src[at:]...)
			pt := cmakeOffsetPoint(src, at)
			old.Edit(gotreesitter.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at), NewEndByte: uint32(at + 1),
				StartPoint: pt, OldEndPoint: pt, NewEndPoint: cmakeOffsetPoint(edited, at+1)})
			got, prof, err := parser.ParseIncrementalProfiled(edited, old)
			if err != nil {
				t.Fatal(err)
			}
			defer got.Release()
			fresh, err := gotreesitter.NewParser(lang).Parse(edited)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Release()
			cmakeTreesEqual(t, lang, got.RootNode(), fresh.RootNode())
			if tc.wantReuse && prof.ReusedBytes*10 < uint64(len(edited))*9 {
				t.Fatalf("reused %d of %d bytes, want over 90%% (unsupported=%v reason=%q)", prof.ReusedBytes, len(edited), prof.ReuseUnsupported, prof.ReuseUnsupportedReason)
			}
			t.Logf("reused=%d of %d new=%d", prof.ReusedBytes, len(edited), prof.NewNodesAllocated)
		})
	}
}
