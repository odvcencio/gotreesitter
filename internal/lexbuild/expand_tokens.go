package lexbuild

import (
	"fmt"
	"sort"
)

// LexicalVariable is grammars.rs: struct LexicalVariable.
type LexicalVariable struct {
	Ident              string
	Kind               VariableType
	ImplicitPrecedence int32
	StartState         uint32
}

// LexicalGrammar is grammars.rs: struct LexicalGrammar.
type LexicalGrammar struct {
	nfa       nfa
	Variables []LexicalVariable
}

// variableIndexForNFAState is grammars.rs: fn variable_index_for_nfa_state. The
// NFA is built in reverse, so each variable's start state is the highest id
// allocated for it, and start states increase with the variable index.
func (g *LexicalGrammar) variableIndexForNFAState(stateID uint32) int {
	return sort.Search(len(g.Variables), func(i int) bool { return g.Variables[i].StartState >= stateID })
}

// variableIndicesForNFAStates is grammars.rs: fn variable_indices_for_nfa_states.
func (g *LexicalGrammar) variableIndicesForNFAStates(ids []uint32) []int {
	var out []int
	prev := -1
	for _, id := range ids {
		v := g.variableIndexForNFAState(id)
		if v == prev {
			continue
		}
		prev = v
		out = append(out, v)
	}
	return out
}

// nfaBuilder is expand_tokens.rs: struct NfaBuilder.
type nfaBuilder struct {
	nfa             nfa
	isSep           bool
	precedenceStack []int32
	ranges          int // character ranges summed over advance states (bNFARanges)
	patRanges       int // character ranges read into the classes of all patterns (bPatternRanges)
}

// getImplicitPrecedence is expand_tokens.rs: fn get_implicit_precedence.
func getImplicitPrecedence(pool *RulePool, root RuleID) int32 {
	id := root
	var boost int32
	for {
		n := pool.node(id)
		switch n.Kind {
		case RString:
			return 2 + boost
		case RMetadata:
			if pool.paramsOf(n.Params).IsMainToken {
				boost++
			}
			id = n.Inner
		default:
			return boost
		}
	}
}

// getCompletionPrecedence is expand_tokens.rs: fn get_completion_precedence.
func getCompletionPrecedence(pool *RulePool, id RuleID) int32 {
	if n := pool.node(id); n.Kind == RMetadata {
		if p := pool.paramsOf(n.Params).Precedence; p.Kind == PrecInteger {
			return p.Int
		}
	}
	return 0
}

// expandTokens is expand_tokens.rs: fn expand_tokens.
func expandTokens(pool *RulePool, lexical []LexicalToken, separatorRoots []RuleID) (lg *LexicalGrammar, err error) {
	defer func() {
		if r := recover(); r != nil {
			if l, ok := r.(lexTooLarge); ok {
				lg, err = nil, l.err
				return
			}
			panic(r)
		}
	}()
	b := &nfaBuilder{isSep: true, precedenceStack: []int32{0}}
	separatorRoot := buildSeparator(pool, separatorRoots)
	var vars []LexicalVariable
	for i, v := range lexical {
		if pool.subtreeMatchesEmptyStr(v.Root) {
			return nil, fmt.Errorf("The rule `%s` matches the empty string.\nTree-sitter does not support syntactic rules that match the empty string\nunless they are used only as the grammar's start rule.\n", v.Ident)
		}
		isImmediate := false
		if n := pool.node(v.Root); n.Kind == RMetadata {
			isImmediate = pool.paramsOf(n.Params).IsMainToken
		}
		b.isSep = false
		b.nfa.states = append(b.nfa.states, nfaState{kind: nfaAccept, variableIndex: i, precedence: getCompletionPrecedence(pool, v.Root)})
		if _, err := b.expandRule(pool, v.Root, b.nfa.lastStateID()); err != nil {
			return nil, fmt.Errorf("Error processing rule %s: %w", v.Ident, err)
		}
		if !isImmediate {
			b.isSep = true
			if _, err := b.expandRule(pool, separatorRoot, b.nfa.lastStateID()); err != nil {
				return nil, err
			}
		}
		vars = append(vars, LexicalVariable{
			Ident:              v.Ident,
			Kind:               v.Kind,
			ImplicitPrecedence: getImplicitPrecedence(pool, v.Root),
			StartState:         b.nfa.lastStateID(),
		})
	}
	return &LexicalGrammar{nfa: b.nfa, Variables: vars}, nil
}

// buildSeparator is expand_tokens.rs: fn build_separator.
func buildSeparator(pool *RulePool, roots []RuleID) RuleID {
	blank := pool.pushNode(Rule{Kind: RBlank})
	if len(roots) == 0 {
		return blank
	}
	var elements []RuleID
	stack := []RuleID{blank}
	for i := len(roots) - 1; i >= 0; i-- {
		stack = append(stack, roots[i])
	}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n := pool.node(id); n.Kind == RChoice {
			base := len(stack)
			stack = append(stack, n.Children...)
			reverseIDs(stack[base:])
		} else {
			dup := false
			for _, e := range elements {
				if pool.subtreeEq(e, id) {
					dup = true
					break
				}
			}
			if !dup {
				elements = append(elements, id)
			}
		}
	}
	choice := pool.pushNode(Rule{Kind: RChoice, Children: elements})
	return pool.pushNode(Rule{Kind: RRepeat, Inner: choice})
}

// checkSize applies the automaton budgets before a state is added. The
// generator has no bound; a pattern like b{0,4294967295} asks it for billions
// of states and it runs until it is killed.
func (b *nfaBuilder) checkSize(ranges int) {
	bNFAStates.check(len(b.nfa.states) + 1)
	if ranges > 0 {
		b.ranges += ranges
		bNFARanges.check(b.ranges)
	}
}

func (b *nfaBuilder) pushAdvance(chars CharSet, stateID uint32) {
	b.checkSize(chars.rangeCount())
	b.nfa.states = append(b.nfa.states, nfaState{
		kind: nfaAdvance, chars: chars, stateID: stateID,
		precedence: b.precedenceStack[len(b.precedenceStack)-1], isSep: b.isSep,
	})
}

func (b *nfaBuilder) pushSplit(stateID uint32) {
	b.checkSize(0)
	last := b.nfa.lastStateID()
	b.nfa.states = append(b.nfa.states, nfaState{kind: nfaSplit, left: stateID, right: last})
}

func sortDedupU32(v []uint32) []uint32 {
	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
	var out []uint32
	for i, x := range v {
		if i == 0 || x != v[i-1] {
			out = append(out, x)
		}
	}
	return out
}

// expandRule is expand_tokens.rs: fn NfaBuilder::expand_rule.
func (b *nfaBuilder) expandRule(pool *RulePool, id RuleID, next uint32) (bool, error) {
	n := pool.node(id)
	switch n.Kind {
	case RPattern:
		s := perlClassRewrite(n.Str)
		h, err := parsePatternCounting(s, containsByte(n.Flags, 'i'), &b.patRanges)
		if err != nil {
			return false, err
		}
		return b.expandRegex(h, next)
	case RString:
		rs := []rune(n.Str)
		for i := len(rs) - 1; i >= 0; i-- {
			b.pushAdvance(charSetFromChar(rs[i]), next)
			next = b.nfa.lastStateID()
		}
		return len(rs) != 0, nil
	case RChoice:
		var alt []uint32
		did := false
		for _, el := range n.Children {
			ok, err := b.expandRule(pool, el, next)
			if err != nil {
				return false, err
			}
			if ok {
				did = true
				alt = append(alt, b.nfa.lastStateID())
			} else {
				alt = append(alt, next)
			}
		}
		alt = sortDedupU32(alt)
		last := b.nfa.lastStateID()
		for _, a := range alt {
			if a != last {
				b.pushSplit(a)
			}
		}
		return did, nil
	case RSeq:
		result := false
		for i := len(n.Children) - 1; i >= 0; i-- {
			ok, err := b.expandRule(pool, n.Children[i], next)
			if err != nil {
				return false, err
			}
			if ok {
				result = true
			}
			next = b.nfa.lastStateID()
		}
		return result, nil
	case RRepeat:
		b.nfa.states = append(b.nfa.states, nfaState{kind: nfaAccept}) // placeholder for split
		split := b.nfa.lastStateID()
		ok, err := b.expandRule(pool, n.Inner, split)
		if err != nil {
			return false, err
		}
		if ok {
			b.nfa.states[split] = nfaState{kind: nfaSplit, left: b.nfa.lastStateID(), right: next}
			return true, nil
		}
		b.nfa.states = b.nfa.states[:len(b.nfa.states)-1]
		return false, nil
	case RMetadata:
		p := pool.paramsOf(n.Params).Precedence
		has := p.Kind == PrecInteger
		if has {
			b.precedenceStack = append(b.precedenceStack, p.Int)
		}
		r, err := b.expandRule(pool, n.Inner, next)
		if has {
			b.precedenceStack = b.precedenceStack[:len(b.precedenceStack)-1]
		}
		return r, err
	case RBlank:
		return false, nil
	case REof:
		return false, fmt.Errorf("`eof()` cannot be used inside a token. A lexical rule cannot check for end of input, so use `eof()` only at the end of a syntactic rule.")
	case RSym:
		return false, fmt.Errorf("unexpected symbol %v", Symbol{n.SymKind, n.Index})
	case RReserved:
		return false, fmt.Errorf("unexpected reserved-word context %s", n.Str)
	}
	panic("lexbuild: expandRule: unexpected rule kind")
}

func containsByte(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}

// expandRegex is expand_tokens.rs: fn NfaBuilder::expand_regex.
func (b *nfaBuilder) expandRegex(h *hir, next uint32) (bool, error) {
	switch h.kind {
	case hirEmpty:
		return false, nil
	case hirLiteral:
		for i := len(h.lit) - 1; i >= 0; i-- {
			b.pushAdvance(charSetFromChar(h.lit[i]), next)
			next = b.nfa.lastStateID()
		}
		return true, nil
	case hirClass:
		b.pushAdvance(charSetFromClass(h.class), next)
		return true, nil
	case hirRepetition:
		min, max, hasMax := h.min, h.max, h.hasMax
		switch {
		case min == 0 && hasMax && max == 1:
			return b.expandZeroOrOne(h.sub, next)
		case min == 1 && !hasMax:
			return b.expandOneOrMore(h.sub, next)
		case min == 0 && !hasMax:
			return b.expandZeroOrMore(h.sub, next)
		case hasMax && min == max:
			return b.expandCount(h.sub, min, next)
		case !hasMax:
			ok, err := b.expandZeroOrMore(h.sub, next)
			if err != nil {
				return false, err
			}
			if ok {
				return b.expandCount(h.sub, min, next)
			}
			return false, nil
		default:
			result, err := b.expandCount(h.sub, min, next)
			if err != nil {
				return false, err
			}
			for k := min; k < max; k++ {
				if result {
					next = b.nfa.lastStateID()
				}
				ok, err := b.expandZeroOrOne(h.sub, next)
				if err != nil {
					return false, err
				}
				if ok {
					result = true
				}
			}
			return result, nil
		}
	case hirLook:
		return false, fmt.Errorf("pattern error: Assertions are not supported")
	case hirCapture:
		return b.expandRegex(h.sub, next)
	case hirConcat:
		result := false
		for i := len(h.subs) - 1; i >= 0; i-- {
			ok, err := b.expandRegex(h.subs[i], next)
			if err != nil {
				return false, err
			}
			if ok {
				result = true
				next = b.nfa.lastStateID()
			}
		}
		return result, nil
	case hirAlternation:
		var alt []uint32
		for _, s := range h.subs {
			ok, err := b.expandRegex(s, next)
			if err != nil {
				return false, err
			}
			if ok {
				alt = append(alt, b.nfa.lastStateID())
			} else {
				alt = append(alt, next)
			}
		}
		alt = sortDedupU32(alt)
		last := b.nfa.lastStateID()
		for _, a := range alt {
			if a != last {
				b.pushSplit(a)
			}
		}
		return true, nil
	}
	panic("lexbuild: expandRegex: unexpected hir kind")
}

// expandOneOrMore is expand_tokens.rs: fn expand_one_or_more.
func (b *nfaBuilder) expandOneOrMore(h *hir, next uint32) (bool, error) {
	b.nfa.states = append(b.nfa.states, nfaState{kind: nfaAccept}) // placeholder for split
	split := b.nfa.lastStateID()
	ok, err := b.expandRegex(h, split)
	if err != nil {
		return false, err
	}
	if ok {
		b.nfa.states[split] = nfaState{kind: nfaSplit, left: b.nfa.lastStateID(), right: next}
		return true, nil
	}
	b.nfa.states = b.nfa.states[:len(b.nfa.states)-1]
	return false, nil
}

// expandZeroOrOne is expand_tokens.rs: fn expand_zero_or_one.
func (b *nfaBuilder) expandZeroOrOne(h *hir, next uint32) (bool, error) {
	ok, err := b.expandRegex(h, next)
	if err != nil {
		return false, err
	}
	if ok {
		b.pushSplit(next)
		return true, nil
	}
	return false, nil
}

// expandZeroOrMore is expand_tokens.rs: fn expand_zero_or_more.
func (b *nfaBuilder) expandZeroOrMore(h *hir, next uint32) (bool, error) {
	ok, err := b.expandOneOrMore(h, next)
	if err != nil {
		return false, err
	}
	if ok {
		b.pushSplit(next)
		return true, nil
	}
	return false, nil
}

// expandCount is expand_tokens.rs: fn expand_count.
func (b *nfaBuilder) expandCount(h *hir, count uint32, next uint32) (bool, error) {
	result := false
	for i := uint32(0); i < count; i++ {
		ok, err := b.expandRegex(h, next)
		if err != nil {
			return false, err
		}
		if ok {
			result = true
			next = b.nfa.lastStateID()
		}
	}
	return result, nil
}
