// Package queryindex builds immutable candidate lists in query declaration
// order. Matching still checks every candidate's full constraints.
package queryindex

import "slices"

// Literals groups grammar symbols by their displayed node type. Canonical
// resolves aliases with the anonymous namedness used by string patterns.
func Literals[S ~uint16](names, wanted []string, display func(string) string, canonical func(S) S) map[string][]S {
	byText := make(map[string][]S, len(wanted))
	for _, text := range wanted {
		byText[text] = nil
	}
	if len(byText) == 0 {
		return byText
	}
	for i, name := range names {
		text := display(name)
		if _, needed := byText[text]; !needed {
			continue
		}
		symbol := canonical(S(i))
		if !slices.Contains(byText[text], symbol) {
			byText[text] = append(byText[text], symbol)
		}
	}
	return byText
}

// Dense merges each symbol's exact patterns with the wildcard fallback. All
// lists are sorted in declaration order and contain each pattern at most once.
// Missing buckets share the immutable fallback rather than requiring a map
// lookup while visiting nodes.
func Dense[S ~uint16](exact map[S][]int, fallback []int) (map[S][]int, [][]int) {
	bySymbol := make(map[S][]int, len(exact))
	var maximum S
	for symbol, patterns := range exact {
		maximum = max(maximum, symbol)
		bySymbol[symbol] = Merge(patterns, fallback)
	}
	dense := make([][]int, int(maximum)+1)
	for i := range dense {
		dense[i] = fallback
	}
	for symbol, patterns := range bySymbol {
		dense[symbol] = patterns
	}
	return bySymbol, dense
}

// Merge returns the sorted union of two sorted pattern lists.
func Merge(a, b []int) []int {
	out := make([]int, 0, len(a)+len(b))
	for len(a) > 0 || len(b) > 0 {
		var next int
		if len(b) == 0 || (len(a) > 0 && a[0] <= b[0]) {
			next, a = a[0], a[1:]
		} else {
			next, b = b[0], b[1:]
		}
		if len(out) == 0 || out[len(out)-1] != next {
			out = append(out, next)
		}
	}
	return out
}
