//go:build cgo && treesitter_c_parity && !gts_incr_census

package cgoharness

import (
	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/diag/incrcensus"
)

// Default builds still inventory identities in this untimed diagnostic helper,
// allowing deterministic before/after comparisons without any engine hooks.
func reuseCensusObserve(oldTree *gts.Tree, run func() (*gts.Tree, error)) (*gts.Tree, incrcensus.Report, error) {
	ids := make(map[*gts.Node]uint64)
	var old []incrcensus.Node
	type item struct {
		node   *gts.Node
		parent uint64
	}
	var stack []item
	if oldTree != nil {
		stack = append(stack, item{node: oldTree.RootNode()})
	}
	for len(stack) > 0 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if v.node == nil {
			continue
		}
		if _, seen := ids[v.node]; seen {
			continue
		}
		id := uint64(len(old) + 1)
		ids[v.node] = id
		old = append(old, incrcensus.Node{ID: id, Parent: v.parent, Start: v.node.StartByte(), End: v.node.EndByte()})
		for i := v.node.ChildCount() - 1; i >= 0; i-- {
			stack = append(stack, item{v.node.Child(i), id})
		}
	}
	recorder := incrcensus.New()
	recorder.Start()
	tree, err := run()
	recorder.Stop()
	retained := make(map[uint64]bool)
	stack = stack[:0]
	if tree != nil {
		stack = append(stack, item{node: tree.RootNode()})
	}
	for len(stack) > 0 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if v.node == nil {
			continue
		}
		if id := ids[v.node]; id != 0 {
			retained[id] = true
		}
		for i := v.node.ChildCount() - 1; i >= 0; i-- {
			stack = append(stack, item{node: v.node.Child(i)})
		}
	}
	return tree, recorder.Finish(old, retained), err
}
