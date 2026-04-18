package grammargen

import (
	"fmt"
	"strings"
	"unicode"
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
	accept      int // symbol ID if accepting, 0 if not
	priority    int // for disambiguation: lower = higher priority
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
		return b.buildPattern(r.Value)
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

// buildPattern parses a regex pattern string and builds the NFA from
// the full regex AST. This correctly handles character class features
// like intersection (&&[^0-9]) and Unicode properties (\p{XID_Start}).
func (b *nfaBuilder) buildPattern(pattern string) (nfaFragment, error) {
	node, err := parseRegex(pattern)
	if err != nil {
		return nfaFragment{}, fmt.Errorf("parse regex %q: %w", pattern, err)
	}
	return b.buildFromRegexNode(node)
}

func (b *nfaBuilder) buildFromRegexNode(node *regexNode) (nfaFragment, error) {
	if node == nil {
		return b.buildEpsilon(), nil
	}
	switch node.kind {
	case regexLiteral:
		start := b.addState()
		end := b.addState()
		b.addCharRange(start, node.value, node.value, end)
		return nfaFragment{start, end}, nil

	case regexCharClass:
		start := b.addState()
		end := b.addState()
		ranges := node.runes
		if node.negate {
			ranges = complementRanges(ranges)
		} else {
			ranges = mergeRanges(ranges)
		}
		for _, rr := range ranges {
			b.addCharRange(start, rr.lo, rr.hi, end)
		}
		return nfaFragment{start, end}, nil

	case regexDot:
		start := b.addState()
		end := b.addState()
		// . matches everything except \n
		b.addCharRange(start, 0, '\n'-1, end)
		b.addCharRange(start, '\n'+1, unicode.MaxRune, end)
		return nfaFragment{start, end}, nil

	case regexSeq:
		if len(node.children) == 0 {
			return b.buildEpsilon(), nil
		}
		first, err := b.buildFromRegexNode(node.children[0])
		if err != nil {
			return nfaFragment{}, err
		}
		cur := first
		for _, c := range node.children[1:] {
			next, err := b.buildFromRegexNode(c)
			if err != nil {
				return nfaFragment{}, err
			}
			b.addEpsilon(cur.end, next.start)
			cur = nfaFragment{cur.start, next.end}
		}
		return cur, nil

	case regexAlt:
		if len(node.children) == 0 {
			return b.buildEpsilon(), nil
		}
		start := b.addState()
		end := b.addState()
		for _, c := range node.children {
			frag, err := b.buildFromRegexNode(c)
			if err != nil {
				return nfaFragment{}, err
			}
			b.addEpsilon(start, frag.start)
			b.addEpsilon(frag.end, end)
		}
		return nfaFragment{start, end}, nil

	case regexStar:
		if len(node.children) == 0 {
			return b.buildEpsilon(), nil
		}
		inner, err := b.buildFromRegexNode(node.children[0])
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

	case regexPlus:
		if len(node.children) == 0 {
			return b.buildEpsilon(), nil
		}
		inner, err := b.buildFromRegexNode(node.children[0])
		if err != nil {
			return nfaFragment{}, err
		}
		start := b.addState()
		end := b.addState()
		b.addEpsilon(start, inner.start)
		b.addEpsilon(inner.end, inner.start)
		b.addEpsilon(inner.end, end)
		return nfaFragment{start, end}, nil

	case regexQuestion:
		if len(node.children) == 0 {
			return b.buildEpsilon(), nil
		}
		inner, err := b.buildFromRegexNode(node.children[0])
		if err != nil {
			return nfaFragment{}, err
		}
		start := b.addState()
		end := b.addState()
		b.addEpsilon(start, inner.start)
		b.addEpsilon(start, end)
		b.addEpsilon(inner.end, end)
		return nfaFragment{start, end}, nil

	case regexCount:
		if len(node.children) == 0 {
			return b.buildEpsilon(), nil
		}
		minCount := node.count
		maxCount := node.countMax
		if maxCount == 0 {
			maxCount = minCount
		}
		// Build min required copies
		var cur nfaFragment
		for i := 0; i < minCount; i++ {
			copy, err := b.buildFromRegexNode(node.children[0])
			if err != nil {
				return nfaFragment{}, err
			}
			if i == 0 {
				cur = copy
			} else {
				b.addEpsilon(cur.end, copy.start)
				cur = nfaFragment{cur.start, copy.end}
			}
		}
		if minCount == 0 {
			cur = b.buildEpsilon()
		}
		// Build optional copies up to max
		if maxCount < 0 {
			// {n,} = unbounded: add a star after the required copies
			star, err := b.buildFromRegexNode(node.children[0])
			if err != nil {
				return nfaFragment{}, err
			}
			loop := b.addState()
			b.addEpsilon(cur.end, loop)
			b.addEpsilon(loop, star.start)
			b.addEpsilon(star.end, loop)
			return nfaFragment{cur.start, loop}, nil
		}
		for i := minCount; i < maxCount; i++ {
			opt, err := b.buildFromRegexNode(node.children[0])
			if err != nil {
				return nfaFragment{}, err
			}
			end := b.addState()
			b.addEpsilon(cur.end, opt.start)
			b.addEpsilon(cur.end, end)
			b.addEpsilon(opt.end, end)
			cur = nfaFragment{cur.start, end}
		}
		return cur, nil

	default:
		return nfaFragment{}, fmt.Errorf("unsupported regex node kind %d in NFA construction", node.kind)
	}
}

func (b *nfaBuilder) buildSeq(children []*Rule) (nfaFragment, error) {
	if len(children) == 0 {
		return b.buildEpsilon(), nil
	}
	var cur nfaFragment
	initialized := false
	for i := 0; i < len(children); {
		var next nfaFragment
		if children[i] != nil && children[i].Kind == RuleString {
			var sb strings.Builder
			for i < len(children) && children[i] != nil && children[i].Kind == RuleString {
				sb.WriteString(children[i].Value)
				i++
			}
			next = b.buildString(sb.String())
		} else {
			frag, err := b.buildFromRule(children[i])
			if err != nil {
				return nfaFragment{}, err
			}
			next = frag
			i++
		}
		if !initialized {
			cur = next
			initialized = true
			continue
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
	if frag, ok := b.buildStringChoice(children); ok {
		return frag, nil
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

func (b *nfaBuilder) buildStringChoice(children []*Rule) (nfaFragment, bool) {
	for _, child := range children {
		if child == nil || child.Kind != RuleString {
			return nfaFragment{}, false
		}
	}

	start := b.addState()
	end := b.addState()
	edges := make(map[int]map[rune]int)
	nextState := func(from int, r rune) int {
		if byRune, ok := edges[from]; ok {
			if to, ok := byRune[r]; ok {
				return to
			}
		} else {
			edges[from] = make(map[rune]int)
		}
		to := b.addState()
		b.addCharRange(from, r, r, to)
		edges[from][r] = to
		return to
	}

	for _, child := range children {
		cur := start
		for i := 0; i < len(child.Value); {
			r, size := utf8.DecodeRuneInString(child.Value[i:])
			cur = nextState(cur, r)
			i += size
		}
		b.addEpsilon(cur, end)
	}
	return nfaFragment{start, end}, true
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
	stringCounts := make(map[string]int)
	for _, pat := range patterns {
		if pat.Rule != nil && pat.Rule.Kind == RuleString {
			stringCounts[pat.Rule.Value]++
		}
	}
	trieEdges := make(map[int]map[rune]int)
	addStringPattern := func(lit string, symID, priority int) {
		cur := start
		for i := 0; i < len(lit); {
			r, size := utf8.DecodeRuneInString(lit[i:])
			byRune := trieEdges[cur]
			if byRune == nil {
				byRune = make(map[rune]int)
				trieEdges[cur] = byRune
			}
			next, ok := byRune[r]
			if !ok {
				next = b.addState()
				b.addCharRange(cur, r, r, next)
				byRune[r] = next
			}
			cur = next
			i += size
		}
		b.states[cur].accept = symID
		b.states[cur].priority = priority
	}

	for _, pat := range patterns {
		if pat.Rule != nil && pat.Rule.Kind == RuleString && stringCounts[pat.Rule.Value] == 1 {
			addStringPattern(pat.Rule.Value, pat.SymbolID, pat.Priority)
			continue
		}
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
