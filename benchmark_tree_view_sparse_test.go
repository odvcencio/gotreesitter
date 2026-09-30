package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

// BenchmarkTreeViewSparseNavigation measures the opt-in cache's cost when a
// caller visits only a root or one path. Payload construction stays outside
// this API microbenchmark; complete parse/edit costs have separate benchmarks.
func BenchmarkTreeViewSparseNavigation(b *testing.B) {
	leaf := gts.NewLeafNode(1, true, 0, 1, gts.Point{}, gts.Point{Column: 1})
	payload := leaf
	for i := 0; i < 16; i++ {
		payload = gts.NewParentNode(2, true, []*gts.Node{payload}, nil, 0)
	}
	for _, workload := range []struct {
		name  string
		depth int
	}{{"Root", 0}, {"OneChild", 1}, {"Path16", 16}} {
		b.Run(workload.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tree := gts.NewTree(payload, nil, nil)
				view := tree.RootNodeView()
				if view != tree.RootNodeView() || view.Parent() != nil {
					b.Fatal("root identity changed")
				}
				for j := 0; j < workload.depth; j++ {
					child := view.Child(0)
					if child == nil || child.Parent() != view {
						b.Fatal("wrong sparse view parent")
					}
					view = child
				}
				tree.Release()
			}
		})
	}
}
