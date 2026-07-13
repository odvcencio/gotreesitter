package main

import "github.com/odvcencio/gotreesitter"

// utf16EditBetween returns the smallest replacement that transforms oldSource
// into newSource. Boundaries are widened when necessary so an edit never
// splits a UTF-16 surrogate pair.
func utf16EditBetween(oldSource, newSource []uint16) (gotreesitter.UTF16Edit, bool) {
	prefix := 0
	for prefix < len(oldSource) && prefix < len(newSource) && oldSource[prefix] == newSource[prefix] {
		prefix++
	}
	if prefix == len(oldSource) && prefix == len(newSource) {
		return gotreesitter.UTF16Edit{
			StartCodeUnit:  uint32(prefix),
			OldEndCodeUnit: uint32(prefix),
			NewEndCodeUnit: uint32(prefix),
		}, false
	}
	for prefix > 0 && (!utf16Boundary(oldSource, prefix) || !utf16Boundary(newSource, prefix)) {
		prefix--
	}

	oldEnd := len(oldSource)
	newEnd := len(newSource)
	for oldEnd > prefix && newEnd > prefix && oldSource[oldEnd-1] == newSource[newEnd-1] {
		oldEnd--
		newEnd--
	}
	for oldEnd < len(oldSource) && newEnd < len(newSource) &&
		(!utf16Boundary(oldSource, oldEnd) || !utf16Boundary(newSource, newEnd)) {
		oldEnd++
		newEnd++
	}

	return gotreesitter.UTF16Edit{
		StartCodeUnit:  uint32(prefix),
		OldEndCodeUnit: uint32(oldEnd),
		NewEndCodeUnit: uint32(newEnd),
	}, true
}

func utf16Boundary(source []uint16, offset int) bool {
	if offset <= 0 || offset >= len(source) {
		return true
	}
	return !isLeadSurrogate(source[offset-1]) || !isTrailSurrogate(source[offset])
}

func isLeadSurrogate(unit uint16) bool {
	return unit >= 0xd800 && unit <= 0xdbff
}

func isTrailSurrogate(unit uint16) bool {
	return unit >= 0xdc00 && unit <= 0xdfff
}
