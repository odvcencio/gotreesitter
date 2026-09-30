package treewalk

// Walk visits entries in source order, or reverse source order when reverse is
// true. visit decides whether to descend and whether to stop the whole walk.
// The child callbacks must describe a stable view for the duration of the walk.
func Walk[N any](root N, reverse bool, count func(N) int, child func(N, int) N, visit func(N) (descend, stop bool)) {
	var inline [32]ChildFrame[N]
	frames := append(inline[:0], ChildFrame[N]{Node: root, NextChild: -1})
	for len(frames) > 0 {
		f := &frames[len(frames)-1]
		if f.NextChild == -1 {
			descend, stop := visit(f.Node)
			if stop {
				return
			}
			if !descend {
				frames[len(frames)-1] = ChildFrame[N]{}
				frames = frames[:len(frames)-1]
				continue
			}
			f.NextChild = 0
		}
		n := count(f.Node)
		if f.NextChild == n {
			frames[len(frames)-1] = ChildFrame[N]{}
			frames = frames[:len(frames)-1]
			continue
		}
		i := f.NextChild
		if reverse {
			i = n - 1 - i
		}
		f.NextChild++
		entry := child(f.Node, i)
		frames = append(frames, ChildFrame[N]{Node: entry, NextChild: -1})
	}
}
