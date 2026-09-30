// Package treewalk holds traversal frames shared by the parser's pointer views.
// Walkers keep one frame per active ancestor and visit siblings in place, so
// their auxiliary storage follows depth rather than the number of children.
package treewalk

// ChildFrame resumes an ordered depth-first walk at the next child.
type ChildFrame[N any] struct {
	Node      N
	NextChild int
}

// FlattenFrame keeps the state needed after a hidden child's descendants have
// been emitted. N and P are the facade's node pointer and point types.
type FlattenFrame[N, P any] struct {
	Node          N
	NextChild     int
	NodeStart     int
	ChildStart    int
	RepeatStart   int
	RepeatEpoch   uint32
	Mask          uint32
	PaddingByte   uint32
	PaddingPoint  P
	PaddingSource N
}

// FoldFrame carries a postorder result while its children are visited.
type FoldFrame[N, V any] struct {
	Node      N
	NextChild int
	Entered   bool
	Value     V
}
