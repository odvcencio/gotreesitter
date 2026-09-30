package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

var treeViewBenchmarkVisits int

// BenchmarkTreeNavigationComplete includes payload parsing, lazy navigation
// construction, parent checks on both live edit versions, and tree release.
// Payload measures the legacy API; Views measures correct tree-scoped edges.
func BenchmarkTreeNavigationCompletePayload(b *testing.B) { benchmarkTreeNavigation(b, false) }

func BenchmarkTreeNavigationCompleteViews(b *testing.B) { benchmarkTreeNavigation(b, true) }

func benchmarkTreeNavigation(b *testing.B, views bool) {
	for _, name := range []string{"go", "c_sharp"} {
		b.Run(name, func(b *testing.B) {
			for _, edit := range []bool{false, true} {
				operation := "Full"
				if edit {
					operation = "Edit"
				}
				b.Run(operation, func(b *testing.B) { benchmarkTreeNavigationComplete(b, name, edit, views) })
			}
		})
	}
}

func benchmarkTreeNavigationComplete(b *testing.B, name string, editing, views bool) {
	lang, source, at := treeViewWitness(b, name)
	parser := gts.NewParser(lang)
	var tree *gts.Tree
	if editing {
		var err error
		tree, err = parser.Parse(source)
		if err != nil {
			b.Fatal(err)
		}
		if views {
			walkViewParentBenchmark(tree.RootNodeView())
		} else {
			walkPayloadParentBenchmark(tree.RootNode())
		}
		defer func() { tree.Release() }()
	}
	start, end := pointAtOffset(source, at), pointAtOffset(source, at+1)
	edit := gts.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at + 1), NewEndByte: uint32(at + 1), StartPoint: start, OldEndPoint: end, NewEndPoint: end}
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	wrong := 0
	for i := 0; i < b.N; i++ {
		var next *gts.Tree
		var err error
		if editing {
			if source[at] == '+' {
				source[at] = '-'
			} else {
				source[at] = '+'
			}
			tree.Edit(edit)
			next, err = parser.ParseIncremental(source, tree)
		} else {
			next, err = parser.Parse(source)
		}
		if err != nil {
			b.Fatal(err)
		}
		if views {
			wrong += walkViewParentBenchmark(next.RootNodeView())
			if editing {
				wrong += walkViewParentBenchmark(tree.RootNodeView())
			}
		} else {
			wrong += walkPayloadParentBenchmark(next.RootNode())
			if editing {
				wrong += walkPayloadParentBenchmark(tree.RootNode())
			}
		}
		if editing {
			tree.Release()
			tree = next
		} else {
			next.Release()
		}
	}
	b.StopTimer()
	if views && wrong != 0 {
		b.Fatalf("tree view operation returned %d wrong parents", wrong)
	}
	b.ReportMetric(float64(wrong)/float64(b.N), "wrong-parent/op")
}

func walkViewParentBenchmark(parent *gts.NodeView) int {
	wrong := 0
	for i := 0; i < parent.ChildCount(); i++ {
		child := parent.Child(i)
		if child.Parent() != parent {
			wrong++
		}
		treeViewBenchmarkVisits++
		wrong += walkViewParentBenchmark(child)
	}
	return wrong
}

func walkPayloadParentBenchmark(parent *gts.Node) int {
	wrong := 0
	for i := 0; i < parent.ChildCount(); i++ {
		child := parent.Child(i)
		if child.Parent() != parent {
			wrong++
		}
		treeViewBenchmarkVisits++
		wrong += walkPayloadParentBenchmark(child)
	}
	return wrong
}
