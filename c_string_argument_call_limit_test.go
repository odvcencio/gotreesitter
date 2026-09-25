package gotreesitter_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// The C parser must keep both conflict branches after the stack passes 4096
// entries. Long sibling lists also need more than 256 final reductions.
func TestCStringArgumentCallCountBoundary(t *testing.T) {
	cases := []struct {
		count     int
		functions bool
	}{
		{count: 4090, functions: true},
		{count: 4091, functions: true},
		{count: 4100, functions: true},
		{count: 4090},
		{count: 4091},
		{count: 4092},
		{count: 8192},
		{count: 65536},
	}
	for _, tc := range cases {
		for _, route := range []string{"token_source", "dfa"} {
			shape := "calls"
			if tc.functions {
				shape = "functions"
			}
			t.Run(fmt.Sprintf("%d/%s/%s", tc.count, shape, route), func(t *testing.T) {
				lang := grammars.CLanguage()
				before := cStringArgumentCalls(tc.count-1, tc.functions)
				after := cStringArgumentCalls(tc.count, tc.functions)
				parser := gotreesitter.NewParser(lang)
				parser.SetAdmissionCandidateRoute(false)
				parseFresh := func(source []byte) *gotreesitter.Tree {
					t.Helper()
					var tree *gotreesitter.Tree
					var err error
					if route == "token_source" {
						tree, err = parser.ParseWithTokenSource(source, grammars.NewCTokenSourceOrEOF(source, lang))
					} else {
						tree, err = parser.Parse(source)
					}
					if err != nil {
						t.Fatal(err)
					}
					cRequireCompleteStringCalls(t, tree, source)
					return tree
				}
				oldTree := parseFresh(before)
				fresh := parseFresh(after)
				freshDigest, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
				fresh.Release()
				if err != nil {
					oldTree.Release()
					t.Fatalf("fresh tree digest: %v", err)
				}
				cEditInsertedCalls(oldTree, before, after)
				var incremental *gotreesitter.Tree
				if route == "token_source" {
					incremental, err = parser.ParseIncrementalWithTokenSource(after, oldTree, grammars.NewCTokenSourceOrEOF(after, lang))
				} else {
					incremental, err = parser.ParseIncremental(after, oldTree)
				}
				oldTree.Release()
				if err != nil {
					t.Fatalf("incremental parse: %v", err)
				}
				defer incremental.Release()
				cRequireCompleteStringCalls(t, incremental, after)
				incrementalDigest, err := benchfixtures.InspectGoTree(incremental.RootNode(), lang)
				if err != nil {
					t.Fatalf("incremental tree digest: %v", err)
				}
				if incrementalDigest.SHA256 != freshDigest.SHA256 {
					t.Fatalf("incremental digest %s differs from fresh %s", incrementalDigest.SHA256, freshDigest.SHA256)
				}
			})
		}
	}
}

func cStringArgumentCalls(count int, functions bool) []byte {
	var source strings.Builder
	if functions {
		source.Grow(count * 90)
		source.WriteString("#include <stdio.h>\n\n")
		for i := range count {
			fmt.Fprintf(&source, "int f%d(int a, int b) {\n\tint x%d = a + b;\n\tprintf(\"%%d\\n\", x%d);\n\treturn x%d;\n}\n\n", i, i, i, i)
		}
	} else {
		source.Grow(count * 16)
		source.WriteString("void f(void) {\n")
		for i := range count {
			fmt.Fprintf(&source, "g(\"x\", %d);\n", i)
		}
		source.WriteString("}\n")
	}
	return []byte(source.String())
}

func cRequireCompleteStringCalls(t *testing.T, tree *gotreesitter.Tree, source []byte) {
	t.Helper()
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("nil C tree")
	}
	root := tree.RootNode()
	if root.StartByte() != 0 || root.EndByte() != uint32(len(source)) ||
		root.HasError() || tree.ParseStoppedEarly() || tree.ParseRuntime().StopReason != gotreesitter.ParseStopAccepted {
		t.Fatalf("incomplete C tree: bytes=%d root=%d..%d error=%t %s",
			len(source), root.StartByte(), root.EndByte(), root.HasError(), tree.ParseRuntime().Summary())
	}
}

func cEditInsertedCalls(tree *gotreesitter.Tree, before, after []byte) {
	start := 0
	for start < len(before) && before[start] == after[start] {
		start++
	}
	oldEnd, newEnd := len(before), len(after)
	for oldEnd > start && newEnd > start && before[oldEnd-1] == after[newEnd-1] {
		oldEnd--
		newEnd--
	}
	tree.Edit(gotreesitter.InputEdit{
		StartByte:   uint32(start),
		OldEndByte:  uint32(oldEnd),
		NewEndByte:  uint32(newEnd),
		StartPoint:  cCallPoint(before, start),
		OldEndPoint: cCallPoint(before, oldEnd),
		NewEndPoint: cCallPoint(after, newEnd),
	})
}

func cCallPoint(source []byte, offset int) gotreesitter.Point {
	prefix := source[:offset]
	row := bytes.Count(prefix, []byte{'\n'})
	lastNewline := bytes.LastIndexByte(prefix, '\n')
	return gotreesitter.Point{Row: uint32(row), Column: uint32(offset - lastNewline - 1)}
}
