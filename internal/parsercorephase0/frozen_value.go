package parsercorephase0

// FrozenValue retains one immutable comparable value. ReuseFrozenValue replaces
// the holder when its value changes, leaving every previously returned holder
// intact. Reading a value never exposes its retained storage. This is a shallow
// holder: pointer or interface members must themselves refer to immutable data.
type FrozenValue[T comparable] struct{ value T }

func ReuseFrozenValue[T comparable](previous *FrozenValue[T], value T) *FrozenValue[T] {
	if previous != nil && previous.value == value {
		return previous
	}
	return &FrozenValue[T]{value: value}
}

func (f *FrozenValue[T]) Value() T {
	if f == nil {
		var zero T
		return zero
	}
	return f.value
}
