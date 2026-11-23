package grammargen

import (
	"encoding/json"
	"fmt"
	"sort"

	gts "github.com/odvcencio/gotreesitter"
)

type effectiveField struct {
	Name      string
	Child     int
	Inherited bool
}
type effectiveProductionInfo struct {
	Aliases []AliasInfo
	Fields  []effectiveField
}

// Collect IDs before the row quotient drops construction states. A production
// first contributes metadata when its item is complete in FIFO discovery.
func (n *NormalizedGrammar) collectCompletedProductionOrder(ctx *lrContext) {
	visited := map[int]bool{0: true}
	seen := map[int]bool{}
	q := []int{0}
	for i := 0; i < len(q); i++ {
		u := q[i]
		for _, c := range ctx.itemSets[u].cores {
			j := int(c.prodIdx)
			if !seen[j] && int(c.dot) == len(n.Productions[j].RHS) {
				seen[j] = true
				n.completedProductionOrder = append(n.completedProductionOrder, j)
			}
		}
		edges := append(lrTransitionRow(nil), ctx.transitionRow(u)...)
		sort.Slice(edges, func(i, j int) bool { return edges[i].sym < edges[j].sym })
		for _, e := range edges {
			v := int(e.target)
			if !visited[v] {
				visited[v] = true
				q = append(q, v)
			}
		}
	}
	for i := range n.Productions {
		if !seen[i] {
			n.completedProductionOrder = append(n.completedProductionOrder, i)
		}
	}
}
func (n *NormalizedGrammar) internEffectiveProductionMetadata() {
	type use struct {
		seenRaw bool
		aliases []aliasSymbolKey
		counts  map[aliasSymbolKey]int
	}
	uses := make([]use, len(n.Symbols))
	for _, x := range n.ExtraSymbols {
		uses[x].seenRaw = true
	}
	// Count authored occurrences before removing matching positional aliases.
	aliasAt := func(p Production) map[int]aliasSymbolKey {
		a := map[int]aliasSymbolKey{}
		for _, k := range p.Aliases {
			a[k.ChildIndex] = aliasSymbolKey{k.Name, k.Named}
		}
		return a
	}
	for _, p := range n.Productions {
		a := aliasAt(p)
		for i, x := range p.RHS {
			u := &uses[x]
			k, ok := a[i]
			if !ok {
				u.seenRaw = true
				continue
			}
			if u.counts == nil {
				u.counts = map[aliasSymbolKey]int{}
			}
			if u.counts[k] == 0 {
				u.aliases = append(u.aliases, k)
			}
			u.counts[k]++
		}
	}
	if n.defaultAliasDisplays == nil {
		n.defaultAliasDisplays = map[int]aliasSymbolKey{}
	}
	for x, u := range uses {
		if u.seenRaw || len(u.aliases) == 0 {
			continue
		}
		best := u.aliases[0]
		for _, k := range u.aliases[1:] {
			if u.counts[k] > u.counts[best] {
				best = k
			}
		}
		n.defaultAliasDisplays[x] = best
	}

	for i := range n.Productions {
		p := &n.Productions[i]
		v := p.Aliases[:0]
		for _, a := range p.Aliases {
			if a.ChildIndex >= 0 && a.ChildIndex < len(p.RHS) && n.defaultAliasDisplays[p.RHS[a.ChildIndex]] == (aliasSymbolKey{a.Name, a.Named}) {
				continue
			}
			v = append(v, a)
		}
		p.Aliases = v
	}
	// Hidden children expose their fields through arbitrarily deep hidden wrappers.
	fs := make([]map[string]bool, len(n.Symbols))
	for i := range fs {
		fs[i] = map[string]bool{}
	}
	for again := true; again; {
		again = false
		for _, p := range n.Productions {
			out := fs[p.LHS]
			for _, f := range p.Fields {
				if !out[f.FieldName] {
					again = true
					out[f.FieldName] = true
				}
			}
			aliases := aliasAt(p)
			for i, x := range p.RHS {
				_, has := n.defaultAliasDisplays[x]
				if n.rawSymbolVisible[x] || has || aliases[i].name != "" {
					continue
				}
				for f := range fs[x] {
					if !out[f] {
						again = true
						out[f] = true
					}
				}
			}
		}
	}
	n.effectiveInfos = []effectiveProductionInfo{{}}
	by := map[string]int{}
	order := n.completedProductionOrder
	for _, id := range order {
		p := &n.Productions[id]
		info := effectiveProductionInfo{Aliases: append([]AliasInfo(nil), p.Aliases...)}
		sort.Slice(info.Aliases, func(i, j int) bool { return info.Aliases[i].ChildIndex < info.Aliases[j].ChildIndex })
		for _, f := range p.Fields {
			info.Fields = append(info.Fields, effectiveField{f.FieldName, f.ChildIndex, false})
		}
		for i, x := range p.RHS {
			if x < n.TokenCount() || n.rawSymbolVisible[x] {
				continue
			}
			for f := range fs[x] {
				info.Fields = append(info.Fields, effectiveField{f, i, true})
			}
		}
		sort.Slice(info.Fields, func(i, j int) bool {
			a, b := info.Fields[i], info.Fields[j]
			if a.Name != b.Name {
				return a.Name < b.Name
			}
			if a.Child != b.Child {
				return a.Child < b.Child
			}
			return !a.Inherited && b.Inherited
		})
		if len(info.Fields) == 0 && len(info.Aliases) == 0 {
			p.ProductionID = 0
			continue
		}
		k, _ := json.Marshal(info)
		v, ok := by[string(k)]
		if !ok {
			v = len(n.effectiveInfos)
			n.effectiveInfos = append(n.effectiveInfos, info)
			by[string(k)] = v
		}
		p.ProductionID = v
	}
}
func (n *NormalizedGrammar) emitEffectiveFields(l *gts.Language) error {
	l.FieldMapSlices = make([][2]uint16, len(n.effectiveInfos))
	for i, v := range n.effectiveInfos {
		off := len(l.FieldMapEntries)
		for _, f := range v.Fields {
			id, ok := n.fieldID(f.Name)
			if !ok {
				return fmt.Errorf("unknown inherited field %q", f.Name)
			}
			if f.Child < 0 || f.Child > 255 || off > 65535 || len(v.Fields) > 65535 {
				return fmt.Errorf("effective field map exceeds encoding at row %d", i)
			}
			l.FieldMapEntries = append(l.FieldMapEntries, gts.FieldMapEntry{FieldID: gts.FieldID(id), ChildIndex: uint8(f.Child), Inherited: f.Inherited})
		}
		l.FieldMapSlices[i] = [2]uint16{uint16(off), uint16(len(v.Fields))}
	}
	return nil
}
func (n *NormalizedGrammar) applyEffectiveDisplays(l *gts.Language) {
	for s, a := range n.defaultAliasDisplays {
		l.SymbolNames[s] = a.name
		l.SymbolMetadata[s].Name = a.name
		l.SymbolMetadata[s].Visible = true
		l.SymbolMetadata[s].Named = a.named
	}
}
