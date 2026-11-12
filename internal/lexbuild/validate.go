package lexbuild

import (
	"fmt"
	"sort"
)

// The checks in this file refuse the grammars that prepare_grammar.rs and
// process_inlines.rs refuse. They change no table.

// validatePrecedences is prepare_grammar.rs: fn validate_precedences. It runs on
// the grammar before symbols are interned.
func validatePrecedences(g *InputGrammar) error {
	pool := g.Pool
	display := func(e PrecedenceEntry) string {
		if e.IsSymbol {
			return "$." + e.Ident
		}
		return "'" + e.Ident + "'"
	}
	// cmp: names before symbols; within a kind, by string.
	greater := func(a, b PrecedenceEntry) bool {
		if a.IsSymbol != b.IsSymbol {
			return a.IsSymbol // (Symbol, Name) is Greater
		}
		return a.Ident > b.Ident
	}
	type pair struct{ a, b PrecedenceEntry }
	pairs := map[pair]bool{} // value: ordering is Less
	for _, list := range g.PrecedenceOrderings {
		for i, e1 := range list {
			// Rust declares `mut entry1` once per outer iteration and swaps it inside the
			// inner loop, so a swap carries to the next entry2. That is reproduced.
			for _, e2 := range list[i+1:] {
				if e1 == e2 {
					continue
				}
				less := false // ordering = Greater
				if greater(e1, e2) {
					less = true
					e1, e2 = e2, e1
				}
				if prev, ok := pairs[pair{e1, e2}]; !ok {
					bPrecPairs.check(len(pairs) + 1)
					pairs[pair{e1, e2}] = less
				} else if prev != less {
					return fmt.Errorf("Conflicting orderings for precedences %s and %s", display(e1), display(e2))
				}
			}
		}
	}
	names := map[string]bool{}
	for _, list := range g.PrecedenceOrderings {
		for _, e := range list {
			if !e.IsSymbol {
				names[e.Ident] = true
			}
		}
	}
	for _, v := range g.Variables {
		stack := []RuleID{v.Root}
		for len(stack) > 0 {
			id := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			n := pool.node(id)
			switch n.Kind {
			case RRepeat:
				stack = append(stack, n.Inner)
			case RSeq, RChoice:
				stack = append(stack, n.Children...)
			case RMetadata:
				if p := pool.paramsOf(n.Params).Precedence; p.Kind == PrecName && !names[p.Ident] {
					return fmt.Errorf("Undeclared precedence '%s' in rule '%s'", p.Ident, v.Ident)
				}
				stack = append(stack, n.Inner)
			}
		}
	}
	return nil
}

// validateIndirectRecursion is prepare_grammar.rs: fn validate_indirect_recursion.
// Go shape: the Rust successor sets are BTreeSets of interned ids; here they are
// sorted by name. Only the choice of which cycle is reported can differ, not
// whether one is found.
func validateIndirectRecursion(g *InputGrammar, sg *SyntaxGrammar) error {
	edges := map[string][]string{}
	var order []string
	for i, v := range g.Variables {
		vp := sg.VarProds[i]
		set := map[string]bool{}
		for _, p := range sg.Productions[vp[0]:vp[1]] {
			if len(p.Steps) == 1 && p.Steps[0].Symbol.isNonTerminal() {
				name := g.Variables[p.Steps[0].Symbol.Index].Ident
				if name != v.Ident {
					set[name] = true
				}
			}
		}
		var lst []string
		for n := range set {
			lst = append(lst, n)
		}
		sort.Strings(lst)
		if _, ok := edges[v.Ident]; !ok {
			order = append(order, v.Ident)
		}
		edges[v.Ident] = lst
	}
	visited := map[string]bool{}
	work := 0
	var getCycle func(cur string, path *[]string) bool
	getCycle = func(cur string, path *[]string) bool {
		work += len(*path) + 1
		bCycleWork.check(work)
		for _, s := range *path {
			if s == cur {
				*path = append(*path, cur)
				return true
			}
		}
		if visited[cur] {
			return false
		}
		*path = append(*path, cur)
		visited[cur] = true
		for _, next := range edges[cur] {
			if getCycle(next, path) {
				return true
			}
		}
		*path = (*path)[:len(*path)-1]
		return false
	}
	for _, start := range order {
		var path []string
		if getCycle(start, &path) {
			s := "Grammar contains an indirectly recursive rule: "
			for i, n := range path {
				if i > 0 {
					s += " -> "
				}
				s += n
			}
			return fmt.Errorf("%s", s)
		}
	}
	return nil
}

// checkInlines is process_inlines.rs: fn process_inlines, the checks only: an
// inlined symbol must be a non-terminal other than the first rule, and a rule
// whose every production vanishes when the inlined rules are expanded is an
// error if it is the first rule or is used. The expansion is followed far enough
// to see whether a production survives; it builds no production map.
func checkInlines(g *InputGrammar, m *extractedMeta, sg *SyntaxGrammar) error {
	if len(m.inline) == 0 {
		return nil
	}
	for _, s := range m.inline {
		switch s.Kind {
		case External:
			return fmt.Errorf("External token `%s` cannot be inlined", m.externalTokens[s.Index].Ident)
		case Terminal:
			return fmt.Errorf("Token `%s` cannot be inlined", m.lexicalVariables[s.Index].Ident)
		case NonTerminal:
			if s.Index == 0 {
				return fmt.Errorf("Rule `%s` cannot be inlined because it is the first rule", g.Variables[0].Ident)
			}
		}
	}
	inline := map[Symbol]bool{}
	for _, s := range m.inline {
		inline[s] = true
	}
	calls, copied := 0, 0
	var survives func(steps []Symbol, eof bool, idx int) (bool, error)
	survives = func(steps []Symbol, eof bool, idx int) (bool, error) {
		for idx < len(steps) && !inline[steps[idx]] {
			idx++
		}
		if idx >= len(steps) {
			return true, nil
		}
		calls++
		bInlineCalls.check(calls)
		vp := sg.VarProds[steps[idx].Index]
		for _, q := range sg.Productions[vp[0]:vp[1]] {
			if q.RequiresEOF && idx+1 < len(steps) {
				continue
			}
			copied += len(steps) - 1 + len(q.Steps)
			bInlineSymbols.check(copied)
			ns := append(append(append([]Symbol(nil), steps[:idx]...), symsOf(q.Steps)...), steps[idx+1:]...)
			ok, err := survives(ns, eof || q.RequiresEOF, idx)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}
	for i, vp := range sg.VarProds {
		if vp[0] == vp[1] {
			continue
		}
		allDead := true
		for _, p := range sg.Productions[vp[0]:vp[1]] {
			ok, err := survives(symsOf(p.Steps), p.RequiresEOF, 0)
			if err != nil {
				return err
			}
			if ok {
				allDead = false
				break
			}
		}
		if !allDead {
			continue
		}
		sym := nonTerminalSym(i)
		used := i == 0
		for _, p := range sg.Productions {
			for _, s := range p.Steps {
				if s.Symbol == sym {
					used = true
				}
			}
		}
		if used {
			return fmt.Errorf("Rule `%s` has no reachable productions after inlining", g.Variables[i].Ident)
		}
	}
	return nil
}

// symsOf lists the symbols of production steps.
func symsOf(steps []ProductionStep) []Symbol {
	out := make([]Symbol, len(steps))
	for i, s := range steps {
		out[i] = s.Symbol
	}
	return out
}
