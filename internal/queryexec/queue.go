package queryexec

// Entry orders a pending capture by source position, pattern, and the order
// in which its match became available. Value owns the match's remaining stream.
type Entry[T any] struct {
	Start    uint32
	Pattern  int
	Sequence uint64
	Value    T
}

func before[T any](a, b Entry[T]) bool {
	if a.Start != b.Start {
		return a.Start < b.Start
	}
	if a.Pattern != b.Pattern {
		return a.Pattern < b.Pattern
	}
	return a.Sequence < b.Sequence
}

// Queue merges capture streams without sorting or copying their captures.
type Queue[T any] struct{ Entries []Entry[T] }

func (q *Queue[T]) Push(e Entry[T]) {
	q.Entries = append(q.Entries, e)
	for i := len(q.Entries) - 1; i > 0; {
		parent := (i - 1) / 2
		if !before(q.Entries[i], q.Entries[parent]) {
			break
		}
		q.Entries[i], q.Entries[parent] = q.Entries[parent], q.Entries[i]
		i = parent
	}
}

func (q *Queue[T]) Pop() Entry[T] {
	first := q.Entries[0]
	last := len(q.Entries) - 1
	q.Entries[0] = q.Entries[last]
	q.Entries[last] = Entry[T]{}
	q.Entries = q.Entries[:last]
	for i := 0; 2*i+1 < last; {
		child := 2*i + 1
		if child+1 < last && before(q.Entries[child+1], q.Entries[child]) {
			child++
		}
		if !before(q.Entries[child], q.Entries[i]) {
			break
		}
		q.Entries[i], q.Entries[child] = q.Entries[child], q.Entries[i]
		i = child
	}
	return first
}

func (q *Queue[T]) Clear() { clear(q.Entries); q.Entries = q.Entries[:0] }
