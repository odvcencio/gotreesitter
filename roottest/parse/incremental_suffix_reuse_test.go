package parse_test

import (
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// suffixReuseGoFile builds a valid Go source file with nFuncs top-level
// functions, large enough that a single-byte edit near the top leaves almost
// the entire file as an unchanged suffix.
func suffixReuseGoFile(nFuncs int) []byte {
	var b strings.Builder
	b.WriteString("package main\n\n")
	for i := 0; i < nFuncs; i++ {
		fmt.Fprintf(&b, "func f%d(x int) int {\n\treturn x + %d\n}\n\n", i, i)
	}
	return []byte(b.String())
}

func suffixReusePointAt(src []byte, off int) gts.Point {
	row, col := 0, 0
	for i := 0; i < off && i < len(src); i++ {
		if src[i] == '\n' {
			row++
			col = 0
		} else {
			col++
		}
	}
	return gts.Point{Row: uint32(row), Column: uint32(col)}
}

func suffixReuseCountNodes(n *gts.Node) int {
	if n == nil {
		return 0
	}
	c := 1
	for i := 0; i < int(n.ChildCount()); i++ {
		c += suffixReuseCountNodes(n.Child(i))
	}
	return c
}

func suffixReuseHasError(n *gts.Node) bool {
	if n == nil {
		return false
	}
	if n.IsError() || n.IsMissing() {
		return true
	}
	for i := 0; i < int(n.ChildCount()); i++ {
		if suffixReuseHasError(n.Child(i)) {
			return true
		}
	}
	return false
}

// TestIncrementalSuffixReuseOnLengthChangingEdit is the regression guard for
// GitHub issue #380: a length-changing edit (insert or delete) must reuse the
// unchanged suffix subtrees rather than re-lexing the whole tail.
//
// Before the coordinate fix, the reuse byte-equality guard (nodeBytesEqual)
// sliced the pre-edit oldSource at the node's POST-edit (shifted) coordinates.
// For any edit that changed byte length, every suffix node's bytes therefore
// spuriously "differed", the node was rejected as ancestor-dirty
// (ReuseRejectAncestorDirtyBeforeEdit), and the parser re-lexed the entire
// suffix (ReusedBytes collapsed to the tiny pre-edit prefix). This asserts both
// halves of the fix: correctness (incremental == fresh) AND that reuse actually
// engages (no ancestor-dirty rejections, a macroscopic ReusedBytes).
func TestIncrementalSuffixReuseOnLengthChangingEdit(t *testing.T) {
	e := grammars.DetectLanguageByName("go")
	if e == nil {
		t.Fatal("go language unavailable")
	}
	lang := e.Language()

	const nFuncs = 2000
	src := suffixReuseGoFile(nFuncs)
	base := gts.NewParser(lang)
	baseTree, err := base.Parse(src)
	if err != nil || baseTree == nil || baseTree.RootNode() == nil {
		t.Fatalf("baseline parse failed: %v", err)
	}
	if suffixReuseHasError(baseTree.RootNode()) {
		t.Fatal("baseline parse has errors")
	}

	// Edit point in the leading whitespace right after "package main\n"
	// (offset 13) — nearly the whole file is unchanged suffix.
	const ip = 13

	cases := []struct {
		name  string
		build func() ([]byte, gts.InputEdit)
	}{
		{
			name: "insert", // OldEndByte != NewEndByte, +1 byte
			build: func() ([]byte, gts.InputEdit) {
				edited := make([]byte, 0, len(src)+1)
				edited = append(edited, src[:ip]...)
				edited = append(edited, ' ')
				edited = append(edited, src[ip:]...)
				return edited, gts.InputEdit{
					StartByte: ip, OldEndByte: ip, NewEndByte: ip + 1,
					StartPoint: suffixReusePointAt(src, ip), OldEndPoint: suffixReusePointAt(src, ip), NewEndPoint: suffixReusePointAt(edited, ip+1),
				}
			},
		},
		{
			name: "delete", // OldEndByte != NewEndByte, -1 byte (negative delta)
			build: func() ([]byte, gts.InputEdit) {
				edited := make([]byte, 0, len(src)-1)
				edited = append(edited, src[:ip]...)
				edited = append(edited, src[ip+1:]...)
				return edited, gts.InputEdit{
					StartByte: ip, OldEndByte: ip + 1, NewEndByte: ip,
					StartPoint: suffixReusePointAt(src, ip), OldEndPoint: suffixReusePointAt(src, ip+1), NewEndPoint: suffixReusePointAt(src, ip),
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			edited, edit := tc.build()

			// Fresh parse of the edited text = ground truth.
			fresh, _ := gts.NewParser(lang).Parse(edited)
			if fresh == nil || fresh.RootNode() == nil {
				t.Fatal("fresh parse of edited text failed")
			}
			if suffixReuseHasError(fresh.RootNode()) {
				t.Fatal("fresh parse of edited text has errors (bad fixture)")
			}
			wantNodes := suffixReuseCountNodes(fresh.RootNode())

			old := baseTree.Copy()
			old.Edit(edit)
			incrParser := gts.NewParser(lang)
			incr, prof, err := incrParser.ParseIncrementalProfiled(edited, old)
			if err != nil || incr == nil || incr.RootNode() == nil {
				t.Fatalf("incremental parse failed: %v", err)
			}

			// (a) Correctness: incremental must equal fresh.
			if suffixReuseHasError(incr.RootNode()) {
				t.Errorf("incremental tree has errors that fresh parse does not")
			}
			if got := suffixReuseCountNodes(incr.RootNode()); got != wantNodes {
				t.Errorf("node count mismatch: incremental=%d fresh=%d", got, wantNodes)
			}
			if incr.RootNode().Type(lang) != fresh.RootNode().Type(lang) {
				t.Errorf("root type mismatch: incremental=%q fresh=%q", incr.RootNode().Type(lang), fresh.RootNode().Type(lang))
			}
			if int(incr.RootNode().EndByte()) != len(edited) {
				t.Errorf("incremental root does not span edited buffer: end=%d len=%d", incr.RootNode().EndByte(), len(edited))
			}

			// (b) The suffix must actually be reused — this is the #380 bug
			// signature. Pre-fix, the coordinate mismatch rejected ~every
			// suffix node: ancestorDirtyBeforeEdit ≈ node count (thousands) and
			// reusedBytes collapsed to the tiny pre-edit prefix. Post-fix only a
			// few genuinely-changed boundary nodes at the edit site may reject,
			// so the count must be a small constant, nowhere near node count.
			if prof.ReuseRejectAncestorDirtyBeforeEdit > 50 {
				t.Errorf("suffix reuse rejected as ancestor-dirty (%d rejections, node count %d) — coordinate reverse-map regressed",
					prof.ReuseRejectAncestorDirtyBeforeEdit, wantNodes)
			}
			if prof.ReusedBytes < uint64(len(edited)/4) {
				t.Errorf("suffix not reused: reusedBytes=%d for a %d-byte edited file (edit changed 1 byte near the top)",
					prof.ReusedBytes, len(edited))
			}
			t.Logf("%s: reusedBytes=%d reusedSubtrees=%d ancestorDirtyBeforeEdit=%d nodes=%d",
				tc.name, prof.ReusedBytes, prof.ReusedSubtrees, prof.ReuseRejectAncestorDirtyBeforeEdit, wantNodes)
		})
	}
}
