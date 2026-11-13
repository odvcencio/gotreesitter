package lexbuild

// checkShape applies the caps on the shape of the input grammar. ParseGrammar
// calls it once the rules are read, before anything is built from them. A cap
// names the quantity it limits in its *SizeError. Everything behind it (the
// construction budgets, and the bounds stated in doc.go) takes these caps as
// its premise.
func checkShape(g *InputGrammar) {
	bRules.check(len(g.Variables))
	bExternals.check(len(g.ExternalRoots))
	bPrecOrderings.check(len(g.PrecedenceOrderings))
	for _, o := range g.PrecedenceOrderings {
		bPrecNames.check(len(o))
		for _, e := range o {
			bNameLen.check(len(e.Ident))
		}
	}
	name := func(s string) { bNameLen.check(len(s)) }
	name(g.Ident)
	name(g.WordName)
	bListLen.check(len(g.SupertypeNames))
	bListLen.check(len(g.InlineNames))
	bListLen.check(len(g.ConflictNames))
	for _, row := range g.ConflictNames {
		bListLen.check(len(row))
	}
	bListLen.check(len(g.ExtraRoots))
	bListLen.check(len(g.ReservedSets))
	for _, rs := range g.ReservedSets {
		bListLen.check(len(rs.Roots))
	}
	for _, v := range g.Variables {
		name(v.Ident)
	}
	for _, s := range g.SupertypeNames {
		name(s)
	}
	for _, s := range g.InlineNames {
		name(s)
	}
	for _, row := range g.ConflictNames {
		for _, s := range row {
			name(s)
		}
	}
	pool := g.Pool
	type item struct {
		id    RuleID
		depth int
	}
	var stack []item
	push := func(id RuleID) { stack = append(stack, item{id, 1}) }
	for _, v := range g.Variables {
		push(v.Root)
	}
	for _, r := range g.ExternalRoots {
		push(r)
	}
	for _, r := range g.ExtraRoots {
		push(r)
	}
	for _, rs := range g.ReservedSets {
		name(rs.Ident)
		for _, r := range rs.Roots {
			push(r)
		}
	}
	for len(stack) > 0 {
		it := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		bRuleDepth.check(it.depth)
		n := pool.node(it.id)
		switch n.Kind {
		case RString:
			bStringLen.check(len(n.Str))
		case RPattern:
			bPatternLen.check(len(n.Str))
		case RNamedSymbol:
			name(n.Str)
		case RSeq, RChoice:
			bListLen.check(len(n.Children))
			for _, c := range n.Children {
				stack = append(stack, item{c, it.depth + 1})
			}
		case RRepeat:
			stack = append(stack, item{n.Inner, it.depth + 1})
		case RReserved:
			name(n.Str)
			stack = append(stack, item{n.Inner, it.depth + 1})
		case RMetadata:
			m := pool.paramsOf(n.Params)
			name(m.Field)
			name(m.Alias.Value)
			name(m.Precedence.Ident)
			stack = append(stack, item{n.Inner, it.depth + 1})
		}
	}
}
