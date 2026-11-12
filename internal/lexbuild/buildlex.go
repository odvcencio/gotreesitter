package lexbuild

import (
	"fmt"
	"sort"
)

// Analysis holds what the lexer construction derives from the lexical grammar,
// the syntax grammar and the parse states: the token conflict map, the
// coincident token index and the keyword set (build_tables.rs: fn
// build_tables, from get_following_tokens to identify_keywords).
type Analysis struct {
	p         *Prepared
	conflicts *TokenConflictMap
	coinc     *coincidentTokenIndex
	keywords  TokenSet
}

// Analyze is the part of build_tables.rs: fn build_tables that runs between the
// parse table builder and populate_error_state: get_following_tokens,
// TokenConflictMap::new, CoincidentTokenIndex::new and identify_keywords.
//
// coincident holds the valid tokens of each parse state at the time the
// generator builds its coincident token index. That is before the error state
// (state 0) gets its recovery entries and before parse states are merged.
func (p *Prepared) Analyze(coincident []TokenSet) (*Analysis, error) {
	if p.Syntax == nil {
		return nil, fmt.Errorf("lexbuild: no syntax grammar")
	}
	fl := newFirstLast(p.Syntax, p.Lex)
	following := getFollowingTokens(p.Syntax, p.Lex, fl)
	a := &Analysis{p: p}
	a.conflicts = newTokenConflictMap(p.Lex, following)
	a.coinc = newCoincidentTokenIndex(coincident, len(p.Lex.Variables), p.Syntax.WordToken, p.Syntax.HasWord)
	a.keywords = identifyKeywords(p.Lex, p.Syntax, a.conflicts, a.coinc)
	return a, nil
}

// Keywords returns the tokens that the keyword lexer recognizes (empty without
// a word token).
func (a *Analysis) Keywords() TokenSet { return a.keywords.clone() }

// allCharsAreAlphabetical is build_tables.rs: fn all_chars_are_alphabetical.
// char::is_alphabetic is read from the embedded table of the Rust standard
// library that built the generator (unicode_alphabetic.go).
func allCharsAreAlphabetical(cursor *nfaCursor) bool {
	for _, rt := range cursor.transitionChars() {
		if rt.isSep {
			continue
		}
		for _, r := range rt.chars.r {
			for c := r.start; c < r.end; c++ {
				if c >= 0xD800 && c <= 0xDFFF {
					continue // char::from_u32 gives None
				}
				if !(isRustAlphabetic(c) || c == '_') {
					return false
				}
			}
		}
	}
	return true
}

// identifyKeywords is build_tables.rs: fn identify_keywords.
func identifyKeywords(lg *LexicalGrammar, sg *SyntaxGrammar, cm *TokenConflictMap, ci *coincidentTokenIndex) TokenSet {
	var keywords TokenSet
	if !sg.HasWord || !sg.WordToken.IsTerminal() {
		return keywords
	}
	word := sg.WordToken
	cursor := newNfaCursor(&lg.nfa, nil)

	// Candidates: tokens that start with letters or underscore and can match the
	// same string as the word token.
	var candidates TokenSet
	for i, v := range lg.Variables {
		cursor.reset([]uint32{v.StartState})
		if allCharsAreAlphabetical(cursor) && cm.doesMatchSameString(i, int(word.Index)) && !cm.doesMatchDifferentString(i, int(word.Index)) {
			candidates.Insert(terminalSym(i))
		}
	}

	// Exclude keyword candidates that shadow another keyword candidate.
	var shadowFree TokenSet
	for _, tok := range candidates.symbols() {
		keep := true
		for _, other := range candidates.symbols() {
			if other != tok && cm.doesMatchSameString(int(other.Index), int(tok.Index)) {
				keep = false
				break
			}
		}
		if keep {
			shadowFree.Insert(tok)
		}
	}

	// Exclude keyword candidates for which substituting the keyword capture token
	// would introduce new lexical conflicts with other tokens.
	for _, tok := range shadowFree.symbols() {
		keep := true
		for other := 0; other < len(lg.Variables); other++ {
			if candidates.Contains(terminalSym(other)) {
				continue
			}
			if ci.allCoincidentStatesHaveWord(tok, terminalSym(other)) {
				continue
			}
			if !cm.hasSameConflictStatus(int(tok.Index), int(word.Index), other) {
				keep = false
				break
			}
		}
		if keep {
			keywords.Insert(tok)
		}
	}
	return keywords
}

// ErrorStateTokens is build_tables.rs: fn populate_error_state. existing holds
// the terminal entries that state 0 has before the call (the extras); the
// result is state 0's token set after it (existing entries, plus a recovery
// entry for each token that does not conflict with a conflict-free token, plus
// the end token and the external tokens that have no internal counterpart).
// recover is the set of tokens that got a recovery entry.
func (a *Analysis) ErrorStateTokens(existing TokenSet) (tokens, recover TokenSet) {
	lg, sg := a.p.Lex, a.p.Syntax
	n := len(lg.Variables)
	var conflictFree TokenSet
	for i := 0; i < n; i++ {
		conflicts := false
		for j := 0; j < n; j++ {
			if j != i && !a.coinc.containsPair(terminalSym(i), terminalSym(j)) && a.conflicts.doesMatchShorterOrLonger(i, j) {
				conflicts = true
				break
			}
		}
		if !conflicts {
			conflictFree.Insert(terminalSym(i))
		}
	}
	tokens = existing.clone()
	add := func(s Symbol) {
		if !tokens.Contains(s) {
			tokens.Insert(s)
			recover.Insert(s)
		}
	}
	for i := 0; i < n; i++ {
		sym := terminalSym(i)
		if !conflictFree.Contains(sym) && !a.keywords.Contains(sym) && !(sg.HasWord && sg.WordToken == sym) {
			excluded := false
			for _, t := range conflictFree.symbols() {
				if !a.coinc.containsPair(sym, t) && a.conflicts.doesConflict(int(sym.Index), int(t.Index)) {
					excluded = true
					break
				}
			}
			if excluded {
				continue
			}
		}
		add(sym)
	}
	for i, et := range sg.ExternalTokens {
		if !et.HasCorresponding {
			add(externalSym(i))
		}
	}
	// state.terminal_entries.insert(Symbol::end(), recover_entry): replaces any entry.
	tokens.Insert(endSym())
	recover.Insert(endSym())
	return tokens, recover
}

// Fragile is build_tables.rs: fn mark_fragile_tokens, for one parse state: it
// returns the terminals of the state whose entry the generator marks not
// reusable, because another valid terminal of the state overlaps them.
func (a *Analysis) Fragile(state *TokenSet) TokenSet {
	valid := state.terminalIndices()
	var out TokenSet
	for _, t := range valid {
		for _, i := range valid {
			if a.conflicts.doesOverlap(i, t) {
				out.Insert(terminalSym(t))
				break
			}
		}
	}
	return out
}

// TokenSymbolIDs is build_tables.rs: fn populate_used_symbols, for the
// tokens: the C symbol id of each token. The end token is 0; then the terminals
// that some state uses, in index order, except that the word token (when it is
// used) is placed at 1; then the used externals. states are the valid tokens
// of every parse state (including state 0).
func (p *Prepared) TokenSymbolIDs(states []TokenSet) map[Symbol]uint32 {
	n := len(p.Lex.Variables)
	termUsed := make([]bool, n)
	extUsed := make([]bool, len(p.Syntax.ExternalTokens))
	for i := range states {
		for _, s := range states[i].symbols() {
			switch s.Kind {
			case Terminal:
				termUsed[s.Index] = true
			case External:
				extUsed[s.Index] = true
			}
		}
	}
	symbols := []Symbol{endSym()}
	for i, used := range termUsed {
		if used {
			if p.Syntax.HasWord && p.Syntax.WordToken == terminalSym(i) {
				symbols = append(symbols, Symbol{})
				copy(symbols[2:], symbols[1:])
				symbols[1] = terminalSym(i)
			} else {
				symbols = append(symbols, terminalSym(i))
			}
		}
	}
	for i, used := range extUsed {
		if used {
			symbols = append(symbols, externalSym(i))
		}
	}
	ids := map[Symbol]uint32{}
	for i, s := range symbols {
		ids[s] = uint32(i)
	}
	return ids
}

// LexTables is the result of BuildLexTables.
type LexTables struct {
	Main    *Table
	Keyword *Table // nil without a word token
	// LexState is the lex state of each parse state, in Main.
	LexState []uint32
}

// BuildLexTables is build_lex_table.rs: fn build_lex_table. states holds the
// valid tokens of each parse state as the generator has them when it builds
// the lex table (after the error state is filled in and parse states are
// merged). Reserved words are not ported.
func (p *Prepared) BuildLexTables(a *Analysis, states []TokenSet) (*LexTables, error) {
	lg, sg := p.Lex, p.Syntax
	var keywordTable lexTable
	if sg.HasWord {
		b := newLexTableBuilder(lg)
		kw := a.keywords.clone()
		if err := guardSize(func() { b.addStateForTokens(&kw) }); err != nil {
			return nil, err
		}
		keywordTable = b.table
	}

	type group struct {
		tokens TokenSet
		ids    []uint32
	}
	var groups []*group
	for i := range states {
		var tokens TokenSet
		for _, tok := range states[i].symbols() {
			if tok.IsTerminal() {
				if a.keywords.Contains(tok) {
					tokens.Insert(sg.WordToken)
				} else {
					tokens.Insert(tok)
				}
			} else if tok.IsEOF() {
				tokens.Insert(tok)
			}
		}
		did := false
		for _, g := range groups {
			if a.mergeTokenSet(&g.tokens, &tokens) {
				did = true
				g.ids = append(g.ids, uint32(i))
				break
			}
		}
		if !did {
			groups = append(groups, &group{tokens, []uint32{uint32(i)}})
		}
	}

	b := newLexTableBuilder(lg)
	lexState := make([]uint32, len(states))
	if err := guardSize(func() {
		for _, g := range groups {
			id := b.addStateForTokens(&g.tokens)
			for _, pid := range g.ids {
				lexState[pid] = id
			}
		}
	}); err != nil {
		return nil, err
	}
	main := b.table
	b.table = lexTable{}
	minimizeLexTable(&main, lexState)
	sortStates(&main, lexState)

	out := &LexTables{LexState: lexState}
	var err error
	if out.Main, err = p.newTable(main); err != nil {
		return nil, err
	}
	if sg.HasWord {
		if out.Keyword, err = p.newTable(keywordTable); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// checkTokenConflicts is build_lex_table.rs: fn check_token_conflicts.
func (a *Analysis) checkTokenConflicts(i int, set *TokenSet) bool {
	for _, j := range set.terminalIndices() {
		if a.conflicts.conflictOrPrefix[i][j] {
			return true
		}
	}
	for _, j := range set.terminalIndices() {
		if a.conflicts.overlapEither[i][j] && !a.coinc.contains[i][j] {
			return true
		}
	}
	return false
}

// mergeTokenSet is build_lex_table.rs: fn merge_token_set.
func (a *Analysis) mergeTokenSet(tokens, other *TokenSet) bool {
	for _, t := range tokens.terminalIndices() {
		if other.containsTerminal(t) {
			continue
		}
		if a.checkTokenConflicts(t, other) {
			return false
		}
	}
	for _, t := range other.terminalIndices() {
		if tokens.containsTerminal(t) {
			continue
		}
		if a.checkTokenConflicts(t, tokens) {
			return false
		}
	}
	tokens.insertAll(other)
	return true
}

type lexSignature struct {
	first     bool
	accept    Symbol
	hasAccept bool
	hasEOF    bool
	advances  string
}

func signatureOf(i int, st *lexState) lexSignature {
	sig := lexSignature{first: i == 0, accept: st.accept, hasAccept: st.hasAccept, hasEOF: st.hasEOF}
	buf := make([]byte, 0, 16*len(st.advances))
	for _, ca := range st.advances {
		for _, r := range ca.chars.r {
			buf = append(buf, byte(r.start), byte(r.start>>8), byte(r.start>>16), byte(r.end), byte(r.end>>8), byte(r.end>>16))
		}
		if ca.action.inMainToken {
			buf = append(buf, 1)
		} else {
			buf = append(buf, 0)
		}
		buf = append(buf, 0xFF, 0xFF, 0xFF)
	}
	sig.advances = string(buf)
	return sig
}

// minimizeLexTable is build_lex_table.rs: fn minimize_lex_table. Groups start
// from the signature (is state 0, accept, has end-of-input step, the advance
// character sets with in_main_token) and split until no group differs by the
// groups of its targets. The Rust code collects the groups from an FxHashMap
// and then sorts them (Vec<Vec<u32>> compares element by element), which fixes
// their order; that sort is reproduced.
func minimizeLexTable(t *lexTable, parseLex []uint32) {
	bySig := map[lexSignature][]uint32{}
	var sigOrder []lexSignature
	for i := range t.states {
		sig := signatureOf(i, &t.states[i])
		if _, ok := bySig[sig]; !ok {
			sigOrder = append(sigOrder, sig)
		}
		bySig[sig] = append(bySig[sig], uint32(i))
	}
	var groups [][]uint32
	for _, sig := range sigOrder {
		groups = append(groups, bySig[sig])
	}
	sort.Slice(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		for k := 0; k < len(a) && k < len(b); k++ {
			if a[k] != b[k] {
				return a[k] < b[k]
			}
		}
		return len(a) < len(b)
	})
	errorGroup := -1
	for gi, g := range groups {
		for _, id := range g {
			if id == 0 {
				errorGroup = gi
			}
		}
	}
	groups[errorGroup], groups[0] = groups[0], groups[errorGroup]

	groupOf := make([]uint32, len(t.states))
	for gi, g := range groups {
		for _, id := range g {
			groupOf[id] = uint32(gi)
		}
	}
	for splitStateIDGroups(t.states, &groups, groupOf, 1, lexStatesDiffer) {
	}

	newStates := make([]lexState, 0, len(groups))
	for _, g := range groups {
		ns := t.states[g[0]]
		t.states[g[0]] = lexState{}
		for k := range ns.advances {
			ns.advances[k].action.state = groupOf[ns.advances[k].action.state]
		}
		if ns.hasEOF {
			ns.eof.state = groupOf[ns.eof.state]
		}
		newStates = append(newStates, ns)
	}
	for i := range parseLex {
		parseLex[i] = groupOf[parseLex[i]]
	}
	t.states = newStates
}

// lexStatesDiffer is build_lex_table.rs: fn lex_states_differ. Go shape: Rust
// zips the two advance lists (the shorter bounds the loop); the signatures of
// states in one group are equal, so the lists have equal length.
func lexStatesDiffer(left, right *lexState, groupOf []uint32) bool {
	n := len(left.advances)
	if len(right.advances) < n {
		n = len(right.advances)
	}
	for i := 0; i < n; i++ {
		if groupOf[left.advances[i].action.state] != groupOf[right.advances[i].action.state] {
			return true
		}
	}
	return false
}

// splitStateIDGroups is dedup.rs: fn split_state_id_groups.
func splitStateIDGroups(states []lexState, groups *[][]uint32, groupOf []uint32, startGroup uint32, shouldSplit func(l, r *lexState, groupOf []uint32) bool) bool {
	result := false
	isSplit := make([]bool, len(states))
	gid := startGroup
	for int(gid) < len(*groups) {
		ids := (*groups)[gid]
		var splitIDs []uint32
		for i := 0; i < len(ids); i++ {
			left := ids[i]
			if isSplit[left] {
				continue
			}
			for j := i + 1; j < len(ids); j++ {
				right := ids[j]
				if isSplit[right] {
					continue
				}
				if shouldSplit(&states[left], &states[right], groupOf) {
					splitIDs = append(splitIDs, right)
					isSplit[right] = true
				}
			}
		}
		if len(splitIDs) > 0 {
			result = true
			var kept []uint32
			for _, id := range (*groups)[gid] {
				if !isSplit[id] {
					kept = append(kept, id)
				}
			}
			(*groups)[gid] = kept
			newGroup := uint32(len(*groups))
			for _, id := range splitIDs {
				groupOf[id] = newGroup
				isSplit[id] = false
			}
			*groups = append(*groups, splitIDs)
		}
		gid++
	}
	return result
}

func compareSymbolOpt(ah bool, a Symbol, bh bool, b Symbol) int {
	if ah != bh {
		if !ah {
			return -1
		}
		return 1
	}
	if !ah {
		return 0
	}
	if a == b {
		return 0
	}
	if symbolLess(a, b) {
		return -1
	}
	return 1
}

func compareAdvance(a, b advanceAction) int {
	if a.state != b.state {
		if a.state < b.state {
			return -1
		}
		return 1
	}
	if a.inMainToken != b.inMainToken {
		if !a.inMainToken {
			return -1
		}
		return 1
	}
	return 0
}

// compareLexState is the derived Ord of tables.rs: struct LexState: accept
// action, then end-of-input action, then the advance actions as a list of
// (CharacterSet, AdvanceAction).
func compareLexState(a, b *lexState) int {
	if c := compareSymbolOpt(a.hasAccept, a.accept, b.hasAccept, b.accept); c != 0 {
		return c
	}
	if a.hasEOF != b.hasEOF {
		if !a.hasEOF {
			return -1
		}
		return 1
	}
	if a.hasEOF {
		if c := compareAdvance(a.eof, b.eof); c != 0 {
			return c
		}
	}
	for i := 0; i < len(a.advances) && i < len(b.advances); i++ {
		if c := compareCharSet(a.advances[i].chars, b.advances[i].chars); c != 0 {
			return c
		}
		if c := compareAdvance(a.advances[i].action, b.advances[i].action); c != 0 {
			return c
		}
	}
	switch {
	case len(a.advances) < len(b.advances):
		return -1
	case len(a.advances) > len(b.advances):
		return 1
	}
	return 0
}

// sortStates is build_lex_table.rs: fn sort_states. State 0 stays first; the
// others are sorted by content with a stable sort (sort_by_key).
func sortStates(t *lexTable, parseLex []uint32) {
	n := len(t.states)
	oldIDsByNew := make([]int, n)
	for i := range oldIDsByNew {
		oldIDsByNew[i] = i
	}
	rest := oldIDsByNew[1:]
	sort.SliceStable(rest, func(i, j int) bool {
		return compareLexState(&t.states[rest[i]], &t.states[rest[j]]) < 0
	})
	newIDsByOld := make([]uint32, n)
	for id, old := range oldIDsByNew {
		newIDsByOld[old] = uint32(id)
	}
	newStates := make([]lexState, n)
	for i, old := range oldIDsByNew {
		st := t.states[old]
		for k := range st.advances {
			st.advances[k].action.state = newIDsByOld[st.advances[k].action.state]
		}
		if st.hasEOF {
			st.eof.state = newIDsByOld[st.eof.state]
		}
		newStates[i] = st
	}
	for i := range parseLex {
		parseLex[i] = newIDsByOld[parseLex[i]]
	}
	t.states = newStates
}

// isRustAlphabetic is char::is_alphabetic of the Rust standard library.
func isRustAlphabetic(c uint32) bool {
	i := sort.Search(len(rustAlphabeticRanges), func(i int) bool { return rustAlphabeticRanges[i][1] >= c })
	return i < len(rustAlphabeticRanges) && rustAlphabeticRanges[i][0] <= c
}
