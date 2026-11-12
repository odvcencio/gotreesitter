package lexbuild

// firstLast holds the FIRST and LAST sets of item_set_builder.rs: struct
// ParseItemSetBuilder, for terminals, externals and non-terminals.
type firstLast struct {
	first    map[Symbol]*TokenSet
	last     map[Symbol]*TokenSet
	reserved map[Symbol]uint16 // reserved_first_sets: the largest reserved set id at a first step
}

// newFirstLast is item_set_builder.rs: fn ParseItemSetBuilder::new, the part
// that fills first_sets and last_sets. For a terminal or external symbol the
// FIRST and LAST sets are the symbol itself. For a non-terminal they are the
// terminals and externals that start (end) its productions, reached through
// non-terminals by an explicit stack. The reserved set id of a non-terminal is
// the largest id found at the first steps that the walk reaches.
func newFirstLast(sg *SyntaxGrammar, lg *LexicalGrammar) *firstLast {
	// The one check of the sizes that this function allocates. FIRST and LAST hold
	// one bit set for each terminal and each external symbol (a singleton whose
	// vector reaches the symbol's own index: n*n and e*e bits in all) and one for
	// each rule (v*(n+e) bits).
	n, e, v := len(lg.Variables), len(sg.ExternalTokens), len(sg.Variables)
	bSymbolCells.check(addCells(addCells(cells(n, n), cells(e, e)), cells(v, addCells(n, e))))
	fl := &firstLast{first: map[Symbol]*TokenSet{}, last: map[Symbol]*TokenSet{}, reserved: map[Symbol]uint16{}}
	for i := range lg.Variables {
		s := terminalSym(i)
		a, b := &TokenSet{}, &TokenSet{}
		a.Insert(s)
		b.Insert(s)
		fl.first[s], fl.last[s] = a, b
	}
	for i := range sg.ExternalTokens {
		s := externalSym(i)
		a, b := &TokenSet{}, &TokenSet{}
		a.Insert(s)
		b.Insert(s)
		fl.first[s], fl.last[s] = a, b
	}
	for i := range sg.Variables {
		sym := nonTerminalSym(i)
		fs, ls := &TokenSet{}, &TokenSet{}
		fl.first[sym], fl.last[sym] = fs, ls

		processed := map[Symbol]bool{}
		stack := []Symbol{sym}
		for len(stack) > 0 {
			s := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			vp := sg.VarProds[s.Index]
			for _, p := range sg.Productions[vp[0]:vp[1]] {
				if len(p.Steps) == 0 {
					continue
				}
				st := p.Steps[0].Symbol
				if st.IsTerminal() || st.isExternal() {
					fs.Insert(st)
				} else if !processed[st] {
					processed[st] = true
					stack = append(stack, st)
				}
				if p.Steps[0].Reserved > fl.reserved[sym] {
					fl.reserved[sym] = p.Steps[0].Reserved
				}
			}
		}
		processed = map[Symbol]bool{}
		stack = []Symbol{sym}
		for len(stack) > 0 {
			s := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			vp := sg.VarProds[s.Index]
			for _, p := range sg.Productions[vp[0]:vp[1]] {
				if len(p.Steps) == 0 {
					continue
				}
				st := p.Steps[len(p.Steps)-1].Symbol
				if st.IsTerminal() || st.isExternal() {
					ls.Insert(st)
				} else if !processed[st] {
					processed[st] = true
					stack = append(stack, st)
				}
			}
		}
	}
	return fl
}

// getFollowingTokens is build_tables.rs: fn get_following_tokens. For each
// terminal, the terminals that can follow it in some production.
func getFollowingTokens(sg *SyntaxGrammar, lg *LexicalGrammar, fl *firstLast) []TokenSet {
	n := len(lg.Variables)
	bSymbolCells.check(cells(n, n)) // one set of terminals for each terminal
	result := make([]TokenSet, n)
	var all TokenSet
	for i := 0; i < n; i++ {
		all.Insert(terminalSym(i))
	}
	for _, p := range sg.Productions {
		steps := p.Steps
		for i := 1; i < len(steps); i++ {
			left := fl.last[steps[i-1].Symbol]
			right := fl.first[steps[i].Symbol]
			rightReserved := &sg.ReservedWordSets[fl.reserved[steps[i].Symbol]]
			for _, lt := range left.symbols() {
				if lt.IsTerminal() {
					result[lt.Index].insertAllTerminals(right)
					result[lt.Index].insertAllTerminals(rightReserved)
				}
			}
		}
	}
	for _, extra := range sg.ExtraSymbols {
		if extra.IsTerminal() {
			for i := range result {
				result[i].Insert(extra)
			}
			result[extra.Index] = all.clone()
		}
	}
	return result
}
