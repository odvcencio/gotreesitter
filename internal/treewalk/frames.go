// Package treewalk holds traversal frames shared by the parser's pointer views.
// Walkers keep one frame per active ancestor and visit siblings in place, so
// their auxiliary storage follows depth rather than the number of children.
package treewalk

// GrowForPath reserves the next descent when a frame stack is full. Counting
// only that path avoids both a whole-tree sizing pass and repeated geometric
// allocations for long hidden lists. The caller supplies the same child and
// stopping rules used by its walk.
func GrowForPath[F, N any](frames []F, node N, next func(N) (N, bool)) []F {
	if len(frames) < cap(frames) {
		return frames
	}
	depth := 1
	for {
		child, ok := next(node)
		if !ok {
			break
		}
		depth++
		node = child
	}
	capacity := len(frames) + depth
	if capacity < 2*cap(frames) {
		capacity = 2 * cap(frames)
	}
	grown := make([]F, len(frames), capacity)
	copy(grown, frames)
	return grown
}

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

// EditFrame carries the child boundary and column rule for an edit walk.
// E is a payload entry, A its arena, and R the facade's column rule.
type EditFrame[E, A, R any] struct {
	Entry      E
	Arena      A
	Rule       R
	NextChild  int
	ChildCount int
	PrevEndRow uint32
	Entered    bool
	Descended  bool
}
