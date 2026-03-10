package grammargen

import (
	"fmt"
	"strconv"
	"unicode/utf8"
)

// nfaTransition is a single NFA transition.
type nfaTransition struct {
	lo, hi    rune // character range (inclusive), 0/0 for epsilon
	epsilon   bool
	nextState int
}

// nfaState is a single state in the NFA.
type nfaState struct {
	transitions []nfaTransition
	accept      int  // symbol ID if accepting, 0 if not
	priority    int  // for disambiguation: lower = higher priority
}

// nfa holds the complete NFA.
type nfa struct {
	states []nfaState
	start  int
}

// nfaFragment is a sub-NFA with designated start and end states.
type nfaFragment struct {
	start, end int
}

// nfaBuilder constructs an NFA using Thompson's construction.
type nfaBuilder struct {
	states []nfaState
}

func newNFABuilder() *nfaBuilder {
	return &nfaBuilder{}
}

func (b *nfaBuilder) addState() int {
	id := len(b.states)
	b.states = append(b.states, nfaState{})
	return id
}

func (b *nfaBuilder) addEpsilon(from, to int) {
	b.states[from].transitions = append(b.states[from].transitions,
		nfaTransition{epsilon: true, nextState: to})
}

func (b *nfaBuilder) addCharRange(from int, lo, hi rune, to int) {
	b.states[from].transitions = append(b.states[from].transitions,
		nfaTransition{lo: lo, hi: hi, nextState: to})
}

// buildFromRule constructs an NFA fragment from a Rule tree.
func (b *nfaBuilder) buildFromRule(r *Rule) (nfaFragment, error) {
	if r == nil {
		return b.buildEpsilon(), nil
	}
	switch r.Kind {
	case RuleString:
		return b.buildString(r.Value), nil
	case RulePattern:
		return b.buildCharClass(r.Value)
	case RuleSeq:
		return b.buildSeq(r.Children)
	case RuleChoice:
		return b.buildChoice(r.Children)
	case RuleRepeat:
		if len(r.Children) == 0 {
			return b.buildEpsilon(), nil
		}
		return b.buildStar(r.Children[0])
	case RuleRepeat1:
		if len(r.Children) == 0 {
			return b.buildEpsilon(), nil
		}
		return b.buildPlus(r.Children[0])
	case RuleOptional:
		if len(r.Children) == 0 {
			return b.buildEpsilon(), nil
		}
		return b.buildOptional(r.Children[0])
	case RuleBlank:
		return b.buildEpsilon(), nil
	default:
		return nfaFragment{}, fmt.Errorf("unsupported rule kind %d in NFA construction", r.Kind)
	}
}

func (b *nfaBuilder) buildEpsilon() nfaFragment {
	s := b.addState()
	e := b.addState()
	b.addEpsilon(s, e)
	return nfaFragment{s, e}
}

func (b *nfaBuilder) buildString(s string) nfaFragment {
	if len(s) == 0 {
		return b.buildEpsilon()
	}
	start := b.addState()
	cur := start
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		next := b.addState()
		b.addCharRange(cur, r, r, next)
		cur = next
		i += size
	}
	return nfaFragment{start, cur}
}

// buildCharClass handles a pre-formatted character class pattern like [a-z] or [^...].
func (b *nfaBuilder) buildCharClass(pattern string) (nfaFragment, error) {
	ranges, err := parseCharClassPattern(pattern)
	if err != nil {
		return nfaFragment{}, err
	}
	start := b.addState()
	end := b.addState()
	for _, rr := range ranges {
		b.addCharRange(start, rr.lo, rr.hi, end)
	}
	return nfaFragment{start, end}, nil
}

func (b *nfaBuilder) buildSeq(children []*Rule) (nfaFragment, error) {
	if len(children) == 0 {
		return b.buildEpsilon(), nil
	}
	first, err := b.buildFromRule(children[0])
	if err != nil {
		return nfaFragment{}, err
	}
	cur := first
	for _, c := range children[1:] {
		next, err := b.buildFromRule(c)
		if err != nil {
			return nfaFragment{}, err
		}
		b.addEpsilon(cur.end, next.start)
		cur = nfaFragment{cur.start, next.end}
	}
	return cur, nil
}

func (b *nfaBuilder) buildChoice(children []*Rule) (nfaFragment, error) {
	if len(children) == 0 {
		return b.buildEpsilon(), nil
	}
	start := b.addState()
	end := b.addState()
	for _, c := range children {
		frag, err := b.buildFromRule(c)
		if err != nil {
			return nfaFragment{}, err
		}
		b.addEpsilon(start, frag.start)
		b.addEpsilon(frag.end, end)
	}
	return nfaFragment{start, end}, nil
}

func (b *nfaBuilder) buildStar(r *Rule) (nfaFragment, error) {
	inner, err := b.buildFromRule(r)
	if err != nil {
		return nfaFragment{}, err
	}
	start := b.addState()
	end := b.addState()
	b.addEpsilon(start, inner.start)
	b.addEpsilon(start, end)
	b.addEpsilon(inner.end, inner.start)
	b.addEpsilon(inner.end, end)
	return nfaFragment{start, end}, nil
}

func (b *nfaBuilder) buildPlus(r *Rule) (nfaFragment, error) {
	inner, err := b.buildFromRule(r)
	if err != nil {
		return nfaFragment{}, err
	}
	start := b.addState()
	end := b.addState()
	b.addEpsilon(start, inner.start)
	b.addEpsilon(inner.end, inner.start)
	b.addEpsilon(inner.end, end)
	return nfaFragment{start, end}, nil
}

func (b *nfaBuilder) buildOptional(r *Rule) (nfaFragment, error) {
	inner, err := b.buildFromRule(r)
	if err != nil {
		return nfaFragment{}, err
	}
	start := b.addState()
	end := b.addState()
	b.addEpsilon(start, inner.start)
	b.addEpsilon(start, end)
	b.addEpsilon(inner.end, end)
	return nfaFragment{start, end}, nil
}

// buildCombinedNFA creates a combined NFA for all terminal patterns.
// A single start state has epsilon transitions to each terminal's NFA.
// Each terminal's accept state is tagged with its symbol ID.
func buildCombinedNFA(patterns []TerminalPattern) (*nfa, error) {
	b := newNFABuilder()
	start := b.addState()

	for _, pat := range patterns {
		frag, err := b.buildFromRule(pat.Rule)
		if err != nil {
			return nil, fmt.Errorf("terminal %d: %w", pat.SymbolID, err)
		}
		b.addEpsilon(start, frag.start)
		b.states[frag.end].accept = pat.SymbolID
		b.states[frag.end].priority = pat.Priority
	}

	return &nfa{states: b.states, start: start}, nil
}

// parseCharClassPattern parses a character class like [a-z] or [^...] into
// a list of inclusive rune ranges. Handles negation by computing complement.
func parseCharClassPattern(pattern string) ([]runeRange, error) {
	if len(pattern) < 2 || pattern[0] != '[' || pattern[len(pattern)-1] != ']' {
		return nil, fmt.Errorf("invalid char class pattern: %q", pattern)
	}
	inner := pattern[1 : len(pattern)-1]
	negate := false
	if len(inner) > 0 && inner[0] == '^' {
		negate = true
		inner = inner[1:]
	}

	var ranges []runeRange
	i := 0
	for i < len(inner) {
		ch, size := decodeCharClassRune(inner, i)
		i += size

		// Check for range: ch-hi
		if i < len(inner) && inner[i] == '-' && i+1 < len(inner) {
			i++ // skip '-'
			hi, size2 := decodeCharClassRune(inner, i)
			i += size2
			ranges = append(ranges, runeRange{ch, hi})
		} else {
			ranges = append(ranges, runeRange{ch, ch})
		}
	}

	if negate {
		ranges = complementRanges(ranges)
	}
	return ranges, nil
}

// decodeCharClassRune decodes a single character from a char class pattern,
// handling escape sequences.
func decodeCharClassRune(s string, pos int) (rune, int) {
	if pos >= len(s) {
		return 0, 0
	}
	if s[pos] == '\\' && pos+1 < len(s) {
		next, size := utf8.DecodeRuneInString(s[pos+1:])
		switch next {
		case 'n':
			return '\n', 1 + size
		case 'r':
			return '\r', 1 + size
		case 't':
			return '\t', 1 + size
		case 'x':
			// \xNN — two hex digits
			hexStart := pos + 1 + size
			if hexStart+2 <= len(s) {
				n, err := strconv.ParseUint(s[hexStart:hexStart+2], 16, 8)
				if err == nil {
					return rune(n), 1 + size + 2
				}
			}
			return next, 1 + size
		default:
			return next, 1 + size
		}
	}
	r, size := utf8.DecodeRuneInString(s[pos:])
	return r, size
}

// complementRanges computes the complement of a set of rune ranges
// within [0, maxRune]. Returns sorted non-overlapping ranges.
func complementRanges(ranges []runeRange) []runeRange {
	if len(ranges) == 0 {
		return []runeRange{{0, maxSupportedRune}}
	}

	// Sort and merge ranges.
	sorted := mergeRanges(ranges)

	var result []runeRange
	pos := rune(0)
	for _, r := range sorted {
		if r.lo > pos {
			result = append(result, runeRange{pos, r.lo - 1})
		}
		pos = r.hi + 1
	}
	if pos <= maxSupportedRune {
		result = append(result, runeRange{pos, maxSupportedRune})
	}
	return result
}

// mergeRanges sorts and merges overlapping rune ranges.
func mergeRanges(ranges []runeRange) []runeRange {
	if len(ranges) == 0 {
		return nil
	}
	sorted := make([]runeRange, len(ranges))
	copy(sorted, ranges)

	// Sort by lo.
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].lo < sorted[j-1].lo; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	result := []runeRange{sorted[0]}
	for _, r := range sorted[1:] {
		last := &result[len(result)-1]
		if r.lo <= last.hi+1 {
			if r.hi > last.hi {
				last.hi = r.hi
			}
		} else {
			result = append(result, r)
		}
	}
	return result
}

// maxSupportedRune is the maximum rune we generate transitions for.
// Using 0x10FFFF (full Unicode) would create huge transition tables,
// so we cap at 0x7F for ASCII-oriented grammars and expand as needed.
const maxSupportedRune = 0x10FFFF
