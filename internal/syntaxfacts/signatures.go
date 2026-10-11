// Package syntaxfacts projects opt-in signature and argument facts from
// grammar-owned rows. It depends on node readers, rather than the parser.
package syntaxfacts

import "github.com/odvcencio/gotreesitter/internal/declarationfacts"

// ParameterFact preserves one parameter name and its exact source type.
// Unnamed entries have empty Name and zero name ranges. Type excludes the
// variadic ellipsis; the declaration range includes it. Pointer is receivers only.
type ParameterFact struct {
	Name, Type                 string
	NameStartByte, NameEndByte uint32
	TypeStartByte, TypeEndByte uint32
	Variadic, Pointer          bool
	StartByte, EndByte         uint32
}

// SignatureFact describes a declared function or method. Parameter slices are
// independently allocated for each extraction, including ExtractInto.
type SignatureFact struct {
	Lang, Kind, Name, Container, NodeType          string
	StartByte, EndByte, NameStartByte, NameEndByte uint32
	Receiver, TypeParameters, Parameters, Results  []ParameterFact
}

// ParameterRule describes one direct, possibly grouped parameter declaration.
type ParameterRule struct {
	NodeType, NameField, NameNodeType, TypeField string
	Variadic                                     bool
}

// SignatureRule selects a declaration and its direct signature fields.
// ListNodeTypes distinguish parameter lists from a single unnamed result type.
// TypeNames strip receiver wrappers; ContainerPath names interface owners.
// All referenced grammar data must resolve or the entire row is dropped.
type SignatureRule struct {
	NodeType, Kind, NameField, NameNodeType                           string
	ReceiverField, TypeParametersField, ParametersField, ResultsField string
	PointerNodeType                                                   string
	ListNodeTypes, Ancestors                                          []string
	Parameters                                                        []ParameterRule
	TypeNames                                                         []declarationfacts.TypeNameRule
	ContainerPath                                                     []declarationfacts.ContainerRule
}

type compiledParameter struct {
	name, nameType, typ uint16
	variadic            bool
}
type compiledSignature[N comparable] struct {
	kind                                                      string
	name, nameType, receiver, types, params, results, pointer uint16
	lists                                                     map[uint16]bool
	parameters                                                map[uint16]compiledParameter
	ancestors                                                 []uint16
	names                                                     *declarationfacts.Names[N]
}

// Signatures holds immutable compiled instructions for declarations.
type Signatures[N comparable] struct {
	reader declarationfacts.Reader[N]
	rules  map[uint16][]compiledSignature[N]
}

func symbols(g declarationfacts.Grammar, name string) []uint16 {
	var result []uint16
	for i, n := range g.Names {
		if n == name {
			result = append(result, uint16(i))
		}
	}
	return result
}

func compileSignature[N comparable](g declarationfacts.Grammar, r declarationfacts.Reader[N], rule SignatureRule) (compiledSignature[N], bool) {
	var c compiledSignature[N]
	var ok bool
	if rule.Kind == "" || len(rule.ListNodeTypes) == 0 || len(rule.Parameters) == 0 {
		return c, false
	}
	if _, ok = g.Symbol(rule.NodeType); !ok {
		return c, false
	}
	c.kind = rule.Kind
	if c.name, ok = g.Field(rule.NameField); !ok {
		return c, false
	}
	if c.nameType, ok = g.Symbol(rule.NameNodeType); !ok {
		return c, false
	}
	for _, f := range []struct {
		name string
		dst  *uint16
	}{{rule.ReceiverField, &c.receiver}, {rule.TypeParametersField, &c.types}, {rule.ParametersField, &c.params}, {rule.ResultsField, &c.results}} {
		if f.name != "" {
			if *f.dst, ok = g.Field(f.name); !ok {
				return c, false
			}
		}
	}
	if rule.PointerNodeType != "" {
		if c.pointer, ok = g.Symbol(rule.PointerNodeType); !ok {
			return c, false
		}
	}
	c.lists = make(map[uint16]bool)
	for _, name := range rule.ListNodeTypes {
		if _, ok = g.Symbol(name); !ok {
			return c, false
		}
		for _, sym := range symbols(g, name) {
			c.lists[sym] = true
		}
	}
	c.parameters = make(map[uint16]compiledParameter)
	for _, p := range rule.Parameters {
		if _, ok = g.Symbol(p.NodeType); !ok {
			return c, false
		}
		var param compiledParameter
		if param.name, ok = g.Field(p.NameField); !ok {
			return c, false
		}
		if param.nameType, ok = g.Symbol(p.NameNodeType); !ok {
			return c, false
		}
		if param.typ, ok = g.Field(p.TypeField); !ok {
			return c, false
		}
		param.variadic = p.Variadic
		for _, sym := range symbols(g, p.NodeType) {
			c.parameters[sym] = param
		}
	}
	for _, name := range rule.Ancestors {
		sym, ok := g.Symbol(name)
		if !ok {
			return c, false
		}
		c.ancestors = append(c.ancestors, sym)
	}
	c.names, ok = declarationfacts.CompileNames(g, r, rule.TypeNames, rule.ContainerPath)
	return c, ok
}

// ValidSignatureRule checks all referenced grammar data.
func ValidSignatureRule(g declarationfacts.Grammar, rule SignatureRule) bool {
	_, ok := compileSignature(g, declarationfacts.Reader[int]{}, rule)
	return ok
}

// CompileSignatures resolves and owns rows. An empty valid set returns nil.
func CompileSignatures[N comparable](g declarationfacts.Grammar, r declarationfacts.Reader[N], rules []SignatureRule) *Signatures[N] {
	var p *Signatures[N]
	for _, rule := range rules {
		c, ok := compileSignature(g, r, rule)
		if !ok {
			continue
		}
		if p == nil {
			p = &Signatures[N]{r, make(map[uint16][]compiledSignature[N])}
		}
		for _, sym := range symbols(g, rule.NodeType) {
			p.rules[sym] = append(p.rules[sym], c)
		}
	}
	return p
}

// Extract appends signatures in source-tree order.
func (p *Signatures[N]) Extract(root N, source []byte, dst *[]SignatureFact) {
	var zero N
	if p == nil || root == zero || dst == nil {
		return
	}
	p.walk(root, source, dst)
}

func (p *Signatures[N]) walk(n N, source []byte, dst *[]SignatureFact) {
	var zero N
	if n == zero {
		return
	}
	r := p.reader
	for _, c := range p.rules[r.Symbol(n)] {
		if !declarationfacts.MatchesAncestors(r, n, c.ancestors) {
			continue
		}
		name := declarationfacts.ChildByField(r, n, c.name)
		if name == zero || r.Symbol(name) != c.nameType || r.Missing(name) {
			continue
		}
		nameText := r.Text(name, source)
		if nameText == "" {
			continue
		}
		f := SignatureFact{Lang: r.Language, Kind: c.kind, Name: nameText, NodeType: r.NodeType(n), StartByte: r.StartByte(n), EndByte: r.EndByte(n), NameStartByte: r.StartByte(name), NameEndByte: r.EndByte(name)}
		receiver := declarationfacts.ChildByField(r, n, c.receiver)
		f.Receiver = p.parameters(receiver, source, c, true)
		if len(f.Receiver) > 1 {
			continue
		}
		if receiver != zero {
			param := declarationfacts.SingleChild(r, receiver)
			typ := declarationfacts.ChildByField(r, param, c.parameters[r.Symbol(param)].typ)
			base := c.names.Base(typ)
			if base != zero {
				f.Container = r.Text(base, source)
			}
		} else if len(c.ancestors) > 0 {
			body := n
			for range c.ancestors {
				body = r.Parent(body)
			}
			owners := c.names.Containers(body, source)
			if len(owners) > 0 {
				f.Container = owners[0].Name
			}
		}
		f.TypeParameters = p.parameters(declarationfacts.ChildByField(r, n, c.types), source, c, false)
		f.Parameters = p.parameters(declarationfacts.ChildByField(r, n, c.params), source, c, false)
		f.Results = p.parameters(declarationfacts.ChildByField(r, n, c.results), source, c, false)
		*dst = append(*dst, f)
		break
	}
	for i := 0; i < r.ChildCount(n); i++ {
		p.walk(r.Child(n, i), source, dst)
	}
}

func (p *Signatures[N]) parameters(n N, source []byte, c compiledSignature[N], receiver bool) []ParameterFact {
	var zero N
	if n == zero || p.reader.Missing(n) {
		return nil
	}
	r := p.reader
	if !c.lists[r.Symbol(n)] {
		return []ParameterFact{{Type: r.Text(n, source), TypeStartByte: r.StartByte(n), TypeEndByte: r.EndByte(n), StartByte: r.StartByte(n), EndByte: r.EndByte(n)}}
	}
	var facts []ParameterFact
	for i := 0; i < r.ChildCount(n); i++ {
		decl := r.Child(n, i)
		if decl == zero || r.Missing(decl) {
			continue
		}
		rule, ok := c.parameters[r.Symbol(decl)]
		if !ok {
			continue
		}
		typ := declarationfacts.ChildByField(r, decl, rule.typ)
		if typ == zero || r.Missing(typ) {
			continue
		}
		f := ParameterFact{Type: r.Text(typ, source), TypeStartByte: r.StartByte(typ), TypeEndByte: r.EndByte(typ), StartByte: r.StartByte(decl), EndByte: r.EndByte(decl), Variadic: rule.variadic, Pointer: receiver && c.names.HasType(typ, c.pointer)}
		named := false
		for j := 0; j < r.ChildCount(decl); j++ {
			if r.Field(decl, j) != rule.name {
				continue
			}
			named = true
			name := r.Child(decl, j)
			if name == zero || r.Symbol(name) != rule.nameType || r.Missing(name) {
				continue
			}
			f.Name = r.Text(name, source)
			f.NameStartByte = r.StartByte(name)
			f.NameEndByte = r.EndByte(name)
			facts = append(facts, f)
		}
		if !named {
			facts = append(facts, f)
		}
	}
	return facts
}
