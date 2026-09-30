package gotreesitter

import "testing"

func TestTreeViewLazyOccurrenceIdentity(t *testing.T) {
	shared := NewLeafNode(1, true, 0, 1, Point{}, Point{Column: 1})
	root := NewParentNode(2, true, []*Node{shared, shared}, nil, 0)
	tree := NewTree(root, []byte("x"), nil)
	defer tree.Release()
	if tree.resultCompatibilityFinalizer.Load() != nil {
		t.Fatal("tree creation allocated a view sidecar")
	}
	view := tree.RootNodeView()
	left, right := view.Child(0), view.Child(1)
	if left == right || left.Node() != right.Node() || left.Parent() != view || right.Parent() != view || left.NextSibling() != right || right.PrevSibling() != left {
		t.Fatal("payload identity was confused with occurrence identity")
	}
	if view.Child(-1) != nil || view.Child(2) != nil || view.NamedChild(-1) != nil || view.NamedChild(2) != nil {
		t.Fatal("out-of-range view navigation succeeded")
	}
	if got := tree.NodeViewForNode(shared); got != left {
		t.Fatal("legacy adapter did not select the first occurrence")
	}
	if tree.hasDeferredResultCompatibility() {
		t.Fatal("view-only sidecar enabled result compatibility")
	}
}

func TestTreeViewDoesNotRewriteSharedParentLinks(t *testing.T) {
	child := NewLeafNode(1, true, 0, 1, Point{}, Point{Column: 1})
	oldRoot := NewParentNode(2, true, []*Node{child}, nil, 0)
	old := NewTree(oldRoot, []byte("x"), nil)
	defer old.Release()
	// Deliberately preserve the old payload's link while making a new tree
	// occurrence. Tree views must not "repair" this shared link in place.
	nextRoot := &Node{children: []*Node{child}}
	next := NewTree(nextRoot, []byte("x"), nil)
	defer next.Release()
	before := child.Parent()
	oldView, nextView := old.RootNodeView().Child(0), next.RootNodeView().Child(0)
	if oldView.Parent().Node() != oldRoot || nextView.Parent().Node() != nextRoot || child.Parent() != before {
		t.Fatal("view navigation changed shared navigation state")
	}
	next.Release()
	if nextView.Node() != nil || oldView.Parent().Node() != oldRoot {
		t.Fatal("releasing the newer tree invalidated the older view")
	}
}

func TestTreeViewNilAndReleased(t *testing.T) {
	var tree *Tree
	var view *NodeView
	if tree.RootNodeView() != nil || tree.NodeViewForNode(nil) != nil || view.Node() != nil || view.Parent() != nil || view.Child(0) != nil || view.ChildCount() != 0 || view.NextNamedSibling() != nil || view.IsNamed() || view.Text(nil) != "" {
		t.Fatal("nil view access did not return empty values")
	}
}
