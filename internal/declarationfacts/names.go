package declarationfacts

// TypeNameRule selects a base identifier through a grammar-owned type path.
// Leaf returns this node. Field follows one direct field; an empty Field on
// a non-leaf follows exactly one named child (excluding comments).
type TypeNameRule struct {
	NodeType, Field string
	Leaf            bool
}

// ContainerRule names the direct owner of a type body. Ancestors restrict its
// scope. ParentPath lists the exact path from a nested owner to its parent body.
// An empty ParentPath starts a container; otherwise names extend its dotted path.
// BodyWrappers allows exact type wrappers between body and owner.
type ContainerRule struct {
	NodeType, NameField, NameNodeType, TypeField string
	Ancestors, ParentPath                        []string
	BodyWrappers                                 []TypeNameRule
}

type compiledName struct {
	field uint16
	leaf  bool
}
type compiledContainer struct {
	node, nameField, nameType, typeField uint16
	ancestors, parentPath                []uint16
	wrappers                             map[uint16]compiledName
}

// Names holds compiled base-name and container paths shared by fact projections.
type Names[N comparable] struct {
	reader     Reader[N]
	types      map[uint16]compiledName
	containers []compiledContainer
}

// CompileNames validates every referenced symbol and field, copying all paths.
func CompileNames[N comparable](g Grammar, r Reader[N], types []TypeNameRule, containers []ContainerRule) (*Names[N], bool) {
	p := &Names[N]{reader: r, types: make(map[uint16]compiledName)}
	for _, rule := range types {
		if _, ok := g.Symbol(rule.NodeType); !ok {
			return nil, false
		}
		var f uint16
		if rule.Field != "" {
			var ok bool
			f, ok = g.Field(rule.Field)
			if !ok {
				return nil, false
			}
		}
		for i, name := range g.Names {
			if name == rule.NodeType {
				p.types[uint16(i)] = compiledName{f, rule.Leaf}
			}
		}
	}
	for _, rule := range containers {
		var c compiledContainer
		var ok bool
		if c.node, ok = g.Symbol(rule.NodeType); !ok {
			return nil, false
		}
		if c.nameField, ok = g.Field(rule.NameField); !ok {
			return nil, false
		}
		if c.nameType, ok = g.Symbol(rule.NameNodeType); !ok {
			return nil, false
		}
		if c.typeField, ok = g.Field(rule.TypeField); !ok {
			return nil, false
		}
		for _, path := range []struct {
			names []string
			dst   *[]uint16
		}{{rule.Ancestors, &c.ancestors}, {rule.ParentPath, &c.parentPath}} {
			for _, name := range path.names {
				sym, ok := g.Symbol(name)
				if !ok {
					return nil, false
				}
				*path.dst = append(*path.dst, sym)
			}
		}
		wrappers, valid := CompileNames(g, r, rule.BodyWrappers, nil)
		if !valid {
			return nil, false
		}
		c.wrappers = wrappers.types
		p.containers = append(p.containers, c)
	}
	return p, true
}

// ChildByField returns the first direct child with this field.
func ChildByField[N comparable](r Reader[N], n N, field uint16) N {
	var zero N
	if n == zero || field == 0 {
		return zero
	}
	for i := 0; i < r.ChildCount(n); i++ {
		if r.Field(n, i) == field {
			return r.Child(n, i)
		}
	}
	return zero
}

// SingleChild returns the sole named, present, non-comment direct child.
func SingleChild[N comparable](r Reader[N], n N) N {
	var zero, result N
	if n == zero {
		return zero
	}
	for i := 0; i < r.ChildCount(n); i++ {
		c := r.Child(n, i)
		if c == zero || !r.Named(c) || r.NodeType(c) == "comment" {
			continue
		}
		if r.Missing(c) || result != zero {
			return zero
		}
		result = c
	}
	return result
}

// Base selects an identifier through only the compiled grammar paths.
func (p *Names[N]) Base(n N) N {
	var zero N
	if p == nil || n == zero || p.reader.Missing(n) {
		return zero
	}
	rule, ok := p.types[p.reader.Symbol(n)]
	if !ok {
		return zero
	}
	if rule.leaf {
		return n
	}
	next := SingleChild(p.reader, n)
	if rule.field != 0 {
		next = ChildByField(p.reader, n, rule.field)
	}
	if next == n {
		return zero
	}
	return p.Base(next)
}

// Container is a named syntax owner and its range-bearing node.
type Container[N comparable] struct {
	Name string
	Node N
}

// Containers finds exact owners of body and expands grouped names as siblings.
func (p *Names[N]) Containers(body N, source []byte) []Container[N] {
	var zero N
	if p == nil || body == zero {
		return nil
	}
	r := p.reader
	for _, rule := range p.containers {
		wrapped := body
		owner := r.Parent(wrapped)
		for owner != zero {
			wrapper, ok := rule.wrappers[r.Symbol(owner)]
			if !ok {
				break
			}
			child := SingleChild(r, owner)
			if wrapper.field != 0 {
				child = ChildByField(r, owner, wrapper.field)
			}
			if child != wrapped {
				break
			}
			wrapped = owner
			owner = r.Parent(wrapped)
		}
		if owner == zero {
			continue
		}
		if r.Symbol(owner) != rule.node || ChildByField(r, owner, rule.typeField) != wrapped {
			continue
		}
		if !MatchesAncestors(r, owner, rule.ancestors) {
			continue
		}
		var parents []Container[N]
		if len(rule.parentPath) > 0 {
			parent := owner
			for _, sym := range rule.parentPath {
				parent = r.Parent(parent)
				if parent == zero || r.Symbol(parent) != sym {
					parent = zero
					break
				}
			}
			if parent == zero {
				continue
			}
			parents = p.Containers(parent, source)
			if len(parents) == 0 {
				continue
			}
		}
		var result []Container[N]
		for i := 0; i < r.ChildCount(owner); i++ {
			name := r.Child(owner, i)
			if r.Field(owner, i) != rule.nameField || name == zero || r.Symbol(name) != rule.nameType || r.Missing(name) {
				continue
			}
			text := r.Text(name, source)
			if text == "" || text == "_" {
				continue
			}
			if len(parents) == 0 {
				result = append(result, Container[N]{text, owner})
			} else {
				for _, parent := range parents {
					result = append(result, Container[N]{parent.Name + "." + text, owner})
				}
			}
		}
		return result
	}
	return nil
}

// MatchesAncestors checks an exact parent chain, nearest first.
func MatchesAncestors[N comparable](r Reader[N], n N, path []uint16) bool {
	var zero N
	for _, symbol := range path {
		n = r.Parent(n)
		if n == zero || r.Symbol(n) != symbol {
			return false
		}
	}
	return true
}
