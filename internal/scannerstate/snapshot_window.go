package scannerstate

// SnapshotWindow retains a bounded set of immutable arena-owned snapshot
// references. The caller validates both ownership and complete byte equality.
// Reset the window when the arena's snapshot storage is reset or released.
type SnapshotWindow[T comparable] struct {
	values     [8]T
	used, next uint8
}

func (w *SnapshotWindow[T]) Find(match func(T) bool) (T, bool) {
	if w != nil && match != nil {
		for i := uint8(0); i < w.used; i++ {
			if match(w.values[i]) {
				return w.values[i], true
			}
		}
	}
	var zero T
	return zero, false
}
func (w *SnapshotWindow[T]) Remember(value T) {
	if w == nil {
		return
	}
	for i := uint8(0); i < w.used; i++ {
		if w.values[i] == value {
			return
		}
	}
	if w.used < uint8(len(w.values)) {
		w.values[w.used] = value
		w.used++
		return
	}
	w.values[w.next] = value
	w.next = (w.next + 1) % uint8(len(w.values))
}
