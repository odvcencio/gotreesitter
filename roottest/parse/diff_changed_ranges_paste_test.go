package parse_test

import (
	"bytes"
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestDiffChangedRangesPasteOnExistingEngine pins the range DiffChangedRanges
// reports for a block pasted between two top-level items on a tree from the
// existing engine (the parser is kept off the core). The report covers the
// pasted bytes and a small margin, not the whole document: a changed child
// count at the root must not widen the range over the unchanged items on
// either side.
func TestDiffChangedRangesPasteOnExistingEngine(t *testing.T) {
	type fixture struct {
		name  string
		lang  func() *gts.Language
		item  func(i int) string
		paste string
	}
	fixtures := []fixture{
		{
			name:  "go",
			lang:  grammars.GoLanguage,
			item:  func(i int) string { return fmt.Sprintf("func f%d(a, b int) int {\n\tx := a + b\n\treturn x\n}\n\n", i) },
			paste: "func pasted(a int) int {\n\treturn a * 2\n}\n\n",
		},
		{
			name: "diff",
			lang: grammars.DiffLanguage,
			item: func(i int) string {
				return fmt.Sprintf("diff --git a/f%d.txt b/f%d.txt\n--- a/f%d.txt\n+++ b/f%d.txt\n@@ -1,2 +1,2 @@\n-old\n+new\n", i, i, i, i)
			},
			paste: "diff --git a/p.txt b/p.txt\n--- a/p.txt\n+++ b/p.txt\n@@ -1 +1 @@\n-a\n+b\n",
		},
	}
	for _, fx := range fixtures {
		for _, remove := range []bool{false, true} {
			name := fx.name + "/paste"
			if remove {
				name = fx.name + "/remove"
			}
			t.Run(name, func(t *testing.T) {
				var head, tail bytes.Buffer
				if fx.name == "go" {
					head.WriteString("package main\n\n")
				}
				for i := 0; i < 400; i++ {
					w := &tail
					if i < 200 {
						w = &head
					}
					w.WriteString(fx.item(i))
				}
				oldSrc := append(append([]byte{}, head.Bytes()...), tail.Bytes()...)
				newSrc := append(append(append([]byte{}, head.Bytes()...), fx.paste...), tail.Bytes()...)
				at := head.Len()
				edit := gts.InputEdit{
					StartByte:   uint32(at),
					OldEndByte:  uint32(at),
					NewEndByte:  uint32(at + len(fx.paste)),
					StartPoint:  pointAtOffset(oldSrc, at),
					OldEndPoint: pointAtOffset(oldSrc, at),
					NewEndPoint: pointAtOffset(newSrc, at+len(fx.paste)),
				}
				if remove {
					oldSrc, newSrc = newSrc, oldSrc
					edit = gts.InputEdit{
						StartByte:   uint32(at),
						OldEndByte:  uint32(at + len(fx.paste)),
						NewEndByte:  uint32(at),
						StartPoint:  pointAtOffset(oldSrc, at),
						OldEndPoint: pointAtOffset(oldSrc, at+len(fx.paste)),
						NewEndPoint: pointAtOffset(newSrc, at),
					}
				}

				p := gts.NewParser(fx.lang())
				p.SetAdmissionCandidateRoute(false)
				old, err := p.Parse(oldSrc)
				if err != nil {
					t.Fatal(err)
				}
				defer old.Release()
				old.Edit(edit)
				next, err := p.ParseIncremental(newSrc, old)
				if err != nil {
					t.Fatal(err)
				}
				defer next.Release()

				ranges := gts.DiffChangedRanges(old, next)
				var total, lo, hi uint32 = 0, ^uint32(0), 0
				for _, r := range ranges {
					total += r.EndByte - r.StartByte
					lo, hi = min(lo, r.StartByte), max(hi, r.EndByte)
				}
				// A node ends before the blank lines that follow it.
				pastedEnd := at + len(bytes.TrimRight([]byte(fx.paste), "\n"))
				if remove { // the removed bytes are in the old coordinates; the join point is what the new tree shows
					pastedEnd = at
				}
				if len(ranges) == 0 || lo > uint32(at) || hi < uint32(pastedEnd) {
					t.Fatalf("ranges %v do not cover the pasted bytes [%d,%d)", ranges, at, pastedEnd)
				}
				t.Logf("%d changed bytes in %d range(s) for a %d-byte paste into a %d-byte document", total, len(ranges), len(fx.paste), len(newSrc))
				// One item of margin on each side is more than any tree needs.
				if limit := uint32(len(fx.paste) + 2*len(fx.item(0))); total > limit {
					t.Fatalf("reported %d changed bytes in %d range(s) for a %d-byte paste into a %d-byte document; want at most %d",
						total, len(ranges), len(fx.paste), len(newSrc), limit)
				}
			})
		}
	}
}
