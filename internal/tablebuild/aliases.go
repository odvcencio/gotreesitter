package tablebuild

import (
	"sort"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

// Symbol is rules.rs: struct Symbol.
type Symbol = lexbuild.Symbol

// Alias is rules.rs: struct Alias. Set false stands for None where the
// generator holds an Option<Alias>.
type Alias = lexbuild.Alias

// AliasMap is rules.rs: type AliasMap, a BTreeMap from a symbol to its alias.
// Symbols lists the keys in the order of the BTreeMap.
type AliasMap map[Symbol]Alias

// Symbols returns the keys in the order of rules.rs: struct Symbol (derived Ord).
func (m AliasMap) Symbols() []Symbol {
	out := make([]Symbol, 0, len(m))
	for s := range m {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return lexbuild.SymbolLess(out[i], out[j]) })
	return out
}

// get is BTreeMap::get: the alias of the symbol, if it has one.
func (m AliasMap) get(s Symbol) (Alias, bool) {
	a, ok := m[s]
	return a, ok
}

// aliasEq compares two aliases as the generator compares two Option<Alias>
// values: two absent aliases are equal, whatever their other fields hold.
func aliasEq(a, b Alias) bool {
	if !a.Set || !b.Set {
		return a.Set == b.Set
	}
	return a.Value == b.Value && a.Named == b.Named
}

func containsSymbol(list []Symbol, s Symbol) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// symbolStatus is extract_default_aliases.rs: struct SymbolStatus.
type symbolStatus struct {
	aliases          []aliasCount
	appearsUnaliased bool
}

type aliasCount struct {
	alias Alias
	count int
}

// statusLists holds the three status lists of extract_default_aliases.
type statusLists struct {
	terminal, nonTerminal, external []symbolStatus
}

// of is the match on symbol.kind in extract_default_aliases.
func (l *statusLists) of(s Symbol) *symbolStatus {
	switch s.Kind {
	case lexbuild.External:
		return &l.external[s.Index]
	case lexbuild.NonTerminal:
		return &l.nonTerminal[s.Index]
	case lexbuild.Terminal:
		return &l.terminal[s.Index]
	}
	// The generator panics here too. Flattening drops every production with an
	// end token that is not its last step, and removes the last one, so a
	// prepared grammar has no such step.
	panic("tablebuild: extract_default_aliases: unexpected end token")
}

// extractDefaultAliases is extract_default_aliases.rs: fn extract_default_aliases.
// It finds the symbols that always appear under an alias, picks for each the
// alias that appears most often (the first one met on a tie) as its default
// alias, and removes that alias from the steps where it is redundant. It
// modifies the steps of sg in place, as the generator modifies its production
// store. lexicalCount is the number of lexical variables.
func extractDefaultAliases(sg *lexbuild.SyntaxGrammar, lexicalCount int) AliasMap {
	lists := &statusLists{
		terminal:    make([]symbolStatus, lexicalCount),
		nonTerminal: make([]symbolStatus, len(sg.Variables)),
		external:    make([]symbolStatus, len(sg.ExternalTokens)),
	}

	// For each grammar symbol, find all of the aliases under which the symbol
	// appears, and whether the symbol ever appears unaliased.
	for pi := range sg.Productions {
		for _, step := range sg.Productions[pi].Steps {
			status := lists.of(step.Symbol)
			// Default aliases don't work for inlined variables.
			if containsSymbol(sg.VariablesToInline, step.Symbol) {
				continue
			}
			if step.Alias.Set {
				found := false
				for k := range status.aliases {
					if aliasEq(status.aliases[k].alias, step.Alias) {
						status.aliases[k].count++
						found = true
						break
					}
				}
				if !found {
					status.aliases = append(status.aliases, aliasCount{step.Alias, 1})
				}
			} else {
				status.appearsUnaliased = true
			}
		}
	}

	for _, s := range sg.ExtraSymbols {
		lists.of(s).appearsUnaliased = true
	}

	// For each symbol that always appears aliased, the alias that occurs most
	// often becomes the default alias. The generator walks terminals, then
	// non-terminals, then externals; each symbol is decided on its own status,
	// so the walk order reaches no result.
	result := AliasMap{}
	decide := func(sym Symbol, status *symbolStatus) {
		if status.appearsUnaliased {
			status.aliases = status.aliases[:0]
			return
		}
		if len(status.aliases) == 0 {
			return
		}
		// max_by_key(|(i, (_, count))| (count, -i)): the largest count, and
		// among equal counts the smallest index.
		best := 0
		for k := 1; k < len(status.aliases); k++ {
			if status.aliases[k].count > status.aliases[best].count {
				best = k
			}
		}
		entry := status.aliases[best]
		status.aliases = append(status.aliases[:0], entry)
		result[sym] = entry.alias
	}
	for i := range lists.terminal {
		decide(lexbuild.TerminalSym(i), &lists.terminal[i])
	}
	for i := range lists.nonTerminal {
		decide(lexbuild.NonTerminalSym(i), &lists.nonTerminal[i])
	}
	for i := range lists.external {
		decide(lexbuild.ExternalSym(i), &lists.external[i])
	}

	// Wherever a symbol is aliased as its default alias, remove the alias,
	// because it is now redundant, unless another production of the same
	// variable has the same alias at the same index on a symbol whose default
	// alias it is not. The positions are cleared after the whole variable is
	// read, as in the generator.
	type position struct{ prod, step int }
	var toClear []position
	work := 0
	for _, vp := range sg.VarProds {
		toClear = toClear[:0]
		prods := sg.Productions[vp[0]:vp[1]]
		for i := range prods {
			for j, step := range prods[i].Steps {
				status := lists.of(step.Symbol)
				if !step.Alias.Set || len(status.aliases) == 0 || !aliasEq(step.Alias, status.aliases[0].alias) {
					continue
				}
				mustKeep := false
				for oi := range prods {
					work++
					bAliasWork.check(work)
					other := prods[oi].Steps
					if oi != i && len(other) > j && aliasEq(other[j].Alias, step.Alias) {
						if d, ok := result.get(other[j].Symbol); !ok || !aliasEq(d, step.Alias) {
							mustKeep = true
							break
						}
					}
				}
				if !mustKeep {
					toClear = append(toClear, position{vp[0] + i, j})
				}
			}
		}
		for _, p := range toClear {
			sg.Productions[p.prod].Steps[p.step].Alias = Alias{}
		}
	}

	return result
}
