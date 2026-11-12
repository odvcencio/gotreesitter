package lexbuild

import (
	"fmt"
	"sort"
)

// This file holds the Unicode part of the pattern parser: simple case
// folding (pattern.rs: Expander, fold_ascii_safe) and Unicode property
// classes (the Rust crate for patterns, file unicode.rs). Tables are those of version 0.8.11
// (Unicode 16.0.0), in unicode_regex_tables.go.

// classUnion, classIntersect and classDifference work on canonical class
// ranges (sorted, disjoint, not touching), as regex_syntax IntervalSet does.
func classUnion(a, b []classRng) []classRng {
	return canonClass(append(append([]classRng(nil), a...), b...))
}

func classIntersect(a, b []classRng) []classRng {
	var out []classRng
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		lo, hi := maxU32(a[i].lo, b[j].lo), minU32(a[i].hi, b[j].hi)
		if lo <= hi {
			out = append(out, classRng{lo, hi})
		}
		if a[i].hi < b[j].hi {
			i++
		} else {
			j++
		}
	}
	return out
}

// classDifference is regex_syntax: IntervalSet::difference with Interval::difference
// (hir/interval.rs). The pieces that remain of a range use the increment and
// decrement of a char, which step over the surrogate gap, and Interval::create
// puts the ends in order.
func classDifference(a, b []classRng) []classRng {
	create := func(lo, hi uint32) classRng {
		if lo <= hi {
			return classRng{lo, hi}
		}
		return classRng{hi, lo}
	}
	// The generator scans b from its start for each range of a. When both are
	// canonical and no end lies in the surrogate gap, every range of b that
	// ends before r.lo also ends before the start of each later range of a and
	// before every piece cut from r, so the scan skips them all; the skip is
	// done once, with a cursor that only moves forward. Otherwise the cursor
	// stays at 0 and the scan is the generator's.
	forward := classFast(a) && classFast(b)
	start := 0
	var out []classRng
	for _, r := range a {
		cur := r
		gone := false
		if forward {
			for start < len(b) && b[start].hi < r.lo {
				start++
			}
		}
		for _, s := range b[start:] {
			if s.hi < cur.lo {
				continue
			}
			if s.lo > cur.hi {
				break
			}
			// cur and s overlap
			if s.lo <= cur.lo && cur.hi <= s.hi {
				gone = true
				break
			}
			addLower := s.lo > cur.lo
			addUpper := s.hi < cur.hi
			var first, second *classRng
			if addLower {
				x := create(cur.lo, charDecrement(s.lo))
				first = &x
			}
			if addUpper {
				x := create(charIncrement(s.hi), cur.hi)
				if first == nil {
					first = &x
				} else {
					second = &x
				}
			}
			if second != nil {
				out = append(out, *first)
				cur = *second
			} else {
				cur = *first
			}
		}
		if !gone {
			out = append(out, cur)
		}
	}
	return out
}

// classFast reports whether the ranges are canonical (sorted, disjoint, not
// touching) and none has an end in the surrogate gap.
func classFast(rs []classRng) bool {
	for i, r := range rs {
		if r.lo > r.hi || (r.lo >= 0xD800 && r.lo <= 0xDFFF) || (r.hi >= 0xD800 && r.hi <= 0xDFFF) {
			return false
		}
		if i > 0 && rs[i-1].hi+1 >= r.lo {
			return false
		}
	}
	return true
}

// caseFoldSimple is regex_syntax: ClassUnicode::case_fold_simple. Each range
// gains the characters of the case-folding orbit of every character in it.
func caseFoldSimple(rs []classRng) []classRng {
	out := append([]classRng(nil), rs...)
	for _, r := range rs {
		i := sort.Search(len(rxCaseFolding), func(i int) bool { return rxCaseFolding[i].c >= r.lo })
		for ; i < len(rxCaseFolding) && rxCaseFolding[i].c <= r.hi; i++ {
			for _, t := range rxCaseFolding[i].to {
				out = append(out, classRng{t, t})
			}
		}
	}
	return canonClass(out)
}

// foldASCIISafe is pattern.rs: fn Expander::fold_ascii_safe. Folding never adds
// U+017F or U+212A unless the class already holds them.
func foldASCIISafe(rs []classRng) []classRng {
	exotic := []classRng{{0x17F, 0x17F}, {0x212A, 0x212A}}
	asked := classIntersect(exotic, rs)
	c := classDifference(rs, exotic)
	c = caseFoldSimple(c)
	c = classDifference(c, exotic)
	return classUnion(c, asked)
}

// foldLeaf folds one leaf of a class (a literal, a range, or a property class)
// as pattern.rs: fn expand_class_item does: the positive set is folded, then
// negated again if the leaf was negated.
func foldLeaf(positive []classRng, negated bool) []classRng {
	f := foldASCIISafe(positive)
	if negated {
		return negateClass(f)
	}
	return f
}

func symbolicNameNormalize(x string) string {
	b := []byte(x)
	start := 0
	if len(b) >= 2 && (b[0] == 'i' || b[0] == 'I') && (b[1] == 's' || b[1] == 'S') {
		start = 2
	}
	startsWithIs := start == 2
	var out []byte
	for i := start; i < len(b); i++ {
		c := b[i]
		if c == ' ' || c == '_' || c == '-' {
			continue
		}
		if c >= 0x80 {
			continue
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out = append(out, c)
	}
	// unicode.rs: ISO_Comment has the abbreviation isc, which is really the
	// general category Other; since "is" prefixes are ignored it would become c.
	if startsWithIs && len(out) == 1 && out[0] == 'c' {
		return "isc"
	}
	return string(out)
}

func lookupPair(tab [][2]string, key string) (string, bool) {
	i := sort.Search(len(tab), func(i int) bool { return tab[i][0] >= key })
	if i < len(tab) && tab[i][0] == key {
		return tab[i][1], true
	}
	return "", false
}

func lookupRanges(tab []nameRanges, name string) ([]classRng, bool) {
	for _, t := range tab {
		if t.name == name {
			var out []classRng
			for k := 0; k+1 < len(t.ranges); k += 2 {
				out = append(out, classRng{t.ranges[k], t.ranges[k+1]})
			}
			return canonClass(out), true
		}
	}
	return nil, false
}

func u32Ranges(v []uint32) []classRng {
	var out []classRng
	for k := 0; k+1 < len(v); k += 2 {
		out = append(out, classRng{v[k], v[k+1]})
	}
	return canonClass(out)
}

// gencatClass is unicode.rs: fn gencat.
func gencatClass(name string) ([]classRng, error) {
	switch name {
	case "ASCII":
		return []classRng{{0, 0x7F}}, nil
	case "Any":
		return []classRng{{0, 0x10FFFF}}, nil
	case "Assigned":
		un, err := gencatClass("Unassigned")
		if err != nil {
			return nil, err
		}
		return negateClass(un), nil
	case "Decimal_Number":
		return u32Ranges(rxPerlDecimal), nil
	}
	if r, ok := lookupRanges(rxGeneralCategory, name); ok {
		return r, nil
	}
	return nil, fmt.Errorf("pattern parse error: Unicode property value not found: %s", name)
}

// unicodeClass is unicode.rs: fn class, for the queries that are ported: a
// binary property, a general category, and General_Category=value. Other
// properties (Script, Script_Extensions, Age, segmentation breaks) return a
// NotPortedError.
func (p *patParser) unicodeClass(name, value string, hasValue bool) ([]classRng, error) {
	if !hasValue {
		norm := symbolicNameNormalize(name)
		if norm != "cf" && norm != "sc" && norm != "lc" {
			if canon, ok := lookupPair(rxPropertyNames, norm); ok {
				return p.binaryClass(canon)
			}
		}
		if canon, ok := p.canonicalGencat(norm); ok {
			return gencatClass(canon)
		}
		// canonical_script: the script tables are not embedded.
		return nil, p.notPorted("Unicode script class \\p{" + name + "} (or an unknown property)")
	}
	pn := symbolicNameNormalize(name)
	pv := symbolicNameNormalize(value)
	canon, ok := lookupPair(rxPropertyNames, pn)
	if !ok {
		return nil, fmt.Errorf("pattern parse error: Unicode property not found: %s", name)
	}
	if canon == "General_Category" {
		c, ok := p.canonicalGencat(pv)
		if !ok {
			return nil, fmt.Errorf("pattern parse error: Unicode property value not found: %s", value)
		}
		return gencatClass(c)
	}
	return nil, p.notPorted("Unicode property " + canon + "=" + value)
}

func (p *patParser) canonicalGencat(norm string) (string, bool) {
	switch norm {
	case "any":
		return "Any", true
	case "assigned":
		return "Assigned", true
	case "ascii":
		return "ASCII", true
	}
	return lookupPair(rxGeneralCategoryValues, norm)
}

// binaryClass is unicode.rs: fn bool_property.
func (p *patParser) binaryClass(canon string) ([]classRng, error) {
	switch canon {
	case "Decimal_Number":
		return u32Ranges(rxPerlDecimal), nil
	case "White_Space":
		return u32Ranges(rxPerlSpace), nil
	}
	if r, ok := lookupRanges(rxBoolProperty, canon); ok {
		return r, nil
	}
	// bool_property: property_set(BY_NAME, name).ok_or(PropertyNotFound)
	return nil, p.errf("Unicode property not found: %s", canon)
}
