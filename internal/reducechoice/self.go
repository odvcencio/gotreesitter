package reducechoice

// Reduction describes the parts of a table action needed for a self conflict.
type Reduction struct {
	Symbol     uint16
	Children   uint8
	Dynamic    int16
	Production uint16
	Plain      bool
}

// LongestSelf chooses the wider production of a certified same-symbol conflict.
// Mixed symbols, precedence, field productions, and non-reduce actions stay live.
func LongestSelf[T any](actions []T, read func(T) Reduction) (int, bool) {
	if len(actions) < 2 {
		return 0, false
	}
	first := read(actions[0])
	if !first.Plain || first.Children == 0 {
		return 0, false
	}
	best, distinct := 0, false
	for i := 1; i < len(actions); i++ {
		a := read(actions[i])
		if !a.Plain || a.Children == 0 || a.Symbol != first.Symbol ||
			a.Dynamic != first.Dynamic || a.Production != first.Production {
			return 0, false
		}
		if a.Children != first.Children {
			distinct = true
		}
		if a.Children > read(actions[best]).Children {
			best = i
		}
	}
	return best, distinct
}
