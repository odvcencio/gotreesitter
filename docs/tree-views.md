# Experimental tree-scoped navigation

`Tree.RootNodeView()` opts into tree-scoped navigation over existing parser
payloads. This API is experimental. `Tree.RootNode()` and existing `*Node`
methods keep their current behavior; callers that retain multiple tree
versions should use views for parent and sibling navigation.

```go
oldRoot := oldTree.RootNodeView()
newRoot := newTree.RootNodeView()
oldChild := oldRoot.Child(0)
newChild := newRoot.Child(0)
// Payload identity can survive reuse:
shared := oldChild.Node() == newChild.Node()
// Each occurrence has its own tree's navigation:
oldParent := oldChild.Parent() // oldRoot
newParent := newChild.Parent() // newRoot
_, _, _ = shared, oldParent, newParent
```

## Identity

Repeated navigation to the same occurrence in a tree returns the same
`*NodeView`, including through `NamedChild`, field lookup, sibling navigation,
and the adapters. Different live trees have different views. `Tree.Copy()`
also has its own views. An unchanged incremental parse returns another handle
to the same tree, so it keeps view identity and allocates nothing.

`NodeView.Node()` exposes the existing shared `*Node` payload. Its pointer
identity is separate from occurrence identity. A hand-built tree may place
the same payload at two child positions; those positions have distinct views.
Views never use a payload's `Parent()` to establish membership or navigation.

## Compatibility adapters

`Tree.NodeViewForNode(node)` binds an existing payload to a view in that tree.
It searches child edges and returns nil for an absent payload. It costs
O(nodes searched) on a cold lookup and creates views only along the matched
path. For repeated payloads in a hand-built tree, it chooses the first
preorder occurrence. It does not infer membership from coordinates, which
may overlap for missing or error nodes.

`TreeCursor.CurrentNodeView()` uses the cursor's child path to preserve the
current occurrence. A subtree cursor first binds its root with the same
first-occurrence rule. A cursor without a tree returns nil. Existing query
captures can be bound with `tree.NodeViewForNode(capture.Node)`. Converting a
view back with `view.Node()` loses navigation context; retain the view when
following parents or siblings.

These adapters do not change the legacy query matcher or incremental splice
selection. They provide tree-local navigation for the nodes those APIs
return. Integrating views into those engines is a separate change.

## Lifetime and mutation

Views borrow the returned tree handle. They do not add a reference or require
a separate release. Keep a tree handle alive while using its views, and call
`Release()` once for each parse result. Either old or new tree may be released
first; the survivor retains the borrowed payload arenas. After the last tree
handle is released, view access returns empty values and its cache drops
payload references. A view cannot revive a released tree.

Concurrent read navigation through views is supported, including lazy view
creation in different trees sharing an arena. Edit, parse, copy, and release
must not overlap navigation. Legacy payload materialization must also not
overlap view navigation. `Tree.Edit` preserves the child graph but changes
payload coordinates and dirty flags. Existing payloads are not immutable
coordinate snapshots across edits; use `Tree.Copy()` when such a snapshot is
needed. Tree-local relations remain valid for both live versions.

## Cost

Parsing allocates no view storage. The first root request installs a cold
cache without changing the pinned `Node` or `Tree` sizes. Relations use stable
slabs of occurrence records and child-index arrays. The cache starts with one
root record, then grows small blocks before using larger slabs. Each visited
occurrence embeds its view in its record, avoiding one heap allocation per
view. A child slot array is added when its parent is first traversed; unvisited
child views and payloads stay lazy.
There is no whole-tree wiring pass and no payload-to-view map. Parent lookup
and payload reads use immutable records without a lock; cached child lookup
allocates nothing. Final release scrubs the records before dropping the slabs,
so retained view pointers cannot keep payload arenas alive. Visiting every node
intentionally creates every view; charge that work to the complete navigation
operation.

This increment does not replace legacy payload storage with structure-of-arrays
output, fix legacy raw `*Node` navigation, change reuse admission, or claim a
parse-speed improvement. Complete-operation timing and memory measurements are
recorded in [the receipt](tree-views-receipt.md).
