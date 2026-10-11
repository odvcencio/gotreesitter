// Package syntaxfacts projects opt-in signature and argument facts from
// grammar-owned rows. It depends on node readers, rather than the parser.
package syntaxfacts

import "github.com/odvcencio/gotreesitter/internal/declarationfacts"

// ParameterFact preserves one parameter name and its exact source type.
// Unnamed entries have empty Name and zero name ranges. Type excludes the
// variadic ellipsis; the declaration range includes it. Pointer is receivers only.
// A parameter without a written type has empty Type and zero type ranges.
// Optional marks a parameter that the source declares optional with "?"; it is
// not set for defaults.
type ParameterFact struct {
	Name, Type                 string
	NameStartByte, NameEndByte uint32
	TypeStartByte, TypeEndByte uint32
	Variadic, Pointer          bool
	StartByte, EndByte         uint32
	Optional                   bool `json:",omitempty"`
}

// SignatureFact describes a declared function or method. Parameter slices are
// independently allocated for each extraction, including ExtractInto.
type SignatureFact struct {
	Lang, Kind, Name, Container, NodeType          string
	StartByte, EndByte, NameStartByte, NameEndByte uint32
	Receiver, TypeParameters, Parameters, Results  []ParameterFact
}

// ParameterRule describes one direct, possibly grouped parameter declaration.
//
// NameSelf uses the declaration as its name; other name/type selectors must
// be empty. Otherwise NameField selects direct children; an empty field selects
// NameNodeType and NameNodeTypes children. AnonymousPatterns preserve unnamed
// positions. Missing names stay unnamed when a type selector is configured.
// TypeField may be empty; TypeOptional also permits an absent configured field.
// Variadic marks the whole row. VariadicNodeTypes marks selected name wrappers;
// UnwrapVariadic strips their wrapper for the name (rest patterns), otherwise
// the name retains its spelling (Python stars). Optional is for TypeScript "?".
type ParameterRule struct {
	NodeType, NameField, NameNodeType, TypeField string
	Variadic                                     bool
	TypeOptional, Optional                       bool
	VariadicNodeTypes                            []string
	UnwrapVariadic                               bool
	NameSelf                                     bool
	NameNodeTypes                                []string
	AnonymousPatterns                            []string
}

// SignatureRule selects a declaration and its direct signature fields.
// ListNodeTypes distinguish parameter lists from a single unnamed result type.
// TypeNames strip receiver wrappers; ContainerPath names interface owners.
// TypeWrapperNodeTypes lists node types, such as a type annotation, that wrap a
// type: the fact keeps the wrapper's single named child as the type.
// DelegateField and DelegateNodeTypes, set together, read the type parameters,
// parameters and results from the child in DelegateField (for example the
// function value bound by a variable declarator) while the fact still names
// and spans the matched node. SingleParameterField names a field holding one
// bare parameter name instead of a list. ParentChildField and
// ParentChildNodeType, set together, require the direct parent to have a
// direct child in that field with that node type.
// RequireContainer skips a match unless ContainerPath resolves an owner.
// All referenced grammar data must resolve or the entire row is dropped.
type SignatureRule struct {
	NodeType, Kind, NameField, NameNodeType                           string
	ReceiverField, TypeParametersField, ParametersField, ResultsField string
	PointerNodeType                                                   string
	ListNodeTypes, Ancestors                                          []string
	Parameters                                                        []ParameterRule
	TypeNames                                                         []declarationfacts.TypeNameRule
	ContainerPath                                                     []declarationfacts.ContainerRule
	TypeWrapperNodeTypes                                              []string
	DelegateField                                                     string
	DelegateNodeTypes                                                 []string
	SingleParameterField                                              string
	RequireContainer                                                  bool
	ParentChildField, ParentChildNodeType                             string
}

type compiledParameter struct {
	name, typ                                              uint16
	nameTypes, variadicTypes, anonymous                    declarationfacts.SymbolSet
	variadic, typeOptional, optional, self, unwrapVariadic bool
}
type compiledSignature[N comparable] struct {
	kind                                                   string
	name, receiver, types, params, results, pointer        uint16
	nameTypes, lists, wrappers, delegateTypes, parentTypes declarationfacts.SymbolSet
	parameters                                             map[uint16][]compiledParameter
	ancestors                                              []declarationfacts.SymbolSet
	names                                                  *declarationfacts.Names[N]
	delegate, single, parentField                          uint16
	requireContainer                                       bool
}

// Signatures holds immutable compiled instructions for declarations.
type Signatures[N comparable] struct {
	reader declarationfacts.Reader[N]
	rules  map[uint16][]compiledSignature[N]
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
	if c.nameTypes, ok = declarationfacts.ResolveSymbols(g, rule.NameNodeType); !ok {
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
	if c.lists, ok = declarationfacts.ResolveSymbolSet(g, rule.ListNodeTypes...); !ok {
		return c, false
	}
	c.parameters = make(map[uint16][]compiledParameter)
	for _, row := range rule.Parameters {
		nodes, valid := declarationfacts.ResolveSymbols(g, row.NodeType)
		if !valid {
			return c, false
		}
		param := compiledParameter{variadic: row.Variadic, typeOptional: row.TypeOptional, optional: row.Optional, self: row.NameSelf, unwrapVariadic: row.UnwrapVariadic}
		if row.NameSelf {
			if row.NameField != "" || row.TypeField != "" || row.NameNodeType != "" || len(row.NameNodeTypes) != 0 || len(row.VariadicNodeTypes) != 0 || len(row.AnonymousPatterns) != 0 || row.UnwrapVariadic {
				return c, false
			}
		} else {
			if row.NameField != "" {
				if param.name, ok = g.Field(row.NameField); !ok {
					return c, false
				}
			}
			if param.nameTypes, ok = declarationfacts.ResolveSymbolSet(g, append([]string{row.NameNodeType}, row.NameNodeTypes...)...); !ok || len(param.nameTypes) == 0 {
				return c, false
			}
			if row.TypeField != "" {
				if param.typ, ok = g.Field(row.TypeField); !ok {
					return c, false
				}
			}
			if param.variadicTypes, ok = declarationfacts.ResolveSymbolSet(g, row.VariadicNodeTypes...); !ok {
				return c, false
			}
			if row.UnwrapVariadic && len(param.variadicTypes) == 0 {
				return c, false
			}
			if param.anonymous, ok = declarationfacts.ResolveSymbolSet(g, row.AnonymousPatterns...); !ok {
				return c, false
			}
		}
		for sym := range nodes {
			c.parameters[sym] = append(c.parameters[sym], param)
		}
	}
	if c.wrappers, ok = declarationfacts.ResolveSymbolSet(g, rule.TypeWrapperNodeTypes...); !ok {
		return c, false
	}
	if rule.DelegateField != "" || len(rule.DelegateNodeTypes) > 0 {
		if len(rule.DelegateNodeTypes) == 0 {
			return c, false
		}
		if c.delegate, ok = g.Field(rule.DelegateField); !ok {
			return c, false
		}
		if c.delegateTypes, ok = declarationfacts.ResolveSymbolSet(g, rule.DelegateNodeTypes...); !ok {
			return c, false
		}
	}
	if rule.SingleParameterField != "" {
		if c.single, ok = g.Field(rule.SingleParameterField); !ok {
			return c, false
		}
	}
	if rule.ParentChildField != "" || rule.ParentChildNodeType != "" {
		if c.parentField, ok = g.Field(rule.ParentChildField); !ok {
			return c, false
		}
		if c.parentTypes, ok = declarationfacts.ResolveSymbols(g, rule.ParentChildNodeType); !ok {
			return c, false
		}
	}
	for _, name := range rule.Ancestors {
		sym, ok := declarationfacts.ResolveSymbols(g, name)
		if !ok {
			return c, false
		}
		c.ancestors = append(c.ancestors, sym)
	}
	c.requireContainer = rule.RequireContainer
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
		set, _ := declarationfacts.ResolveSymbols(g, rule.NodeType)
		for sym := range set {
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
		if c.parentField != 0 && !declarationfacts.HasFieldChild(r, r.Parent(n), c.parentField, c.parentTypes) {
			continue
		}
		name := declarationfacts.ChildByField(r, n, c.name)
		if name == zero || !c.nameTypes[r.Symbol(name)] || r.Missing(name) {
			continue
		}
		src := n
		if c.delegate != 0 {
			src = declarationfacts.ChildByField(r, n, c.delegate)
			if src == zero || r.Missing(src) || !c.delegateTypes[r.Symbol(src)] {
				continue
			}
		}
		nameText := r.Text(name, source)
		if nameText == "" {
			continue
		}
		f := SignatureFact{Lang: r.Language, Kind: c.kind, Name: nameText, NodeType: r.NodeType(n), StartByte: r.StartByte(n), EndByte: r.EndByte(n), NameStartByte: r.StartByte(name), NameEndByte: r.EndByte(name)}
		receiver := declarationfacts.ChildByField(r, src, c.receiver)
		f.Receiver = p.parameters(receiver, source, c, true)
		if len(f.Receiver) > 1 {
			continue
		}
		if receiver != zero {
			param := declarationfacts.SingleChild(r, receiver)
			var typ N
			if rows := c.parameters[r.Symbol(param)]; len(rows) > 0 {
				typ = declarationfacts.ChildByField(r, param, rows[0].typ)
			}
			base := c.names.Base(typ)
			if base != zero {
				f.Container = r.Text(base, source)
			}
		} else {
			body := n
			for range c.ancestors {
				body = r.Parent(body)
			}
			owners := c.names.Containers(body, source)
			if len(owners) == 0 && c.requireContainer {
				continue
			}
			if len(owners) > 0 {
				f.Container = owners[0].Name
			}
		}
		f.TypeParameters = p.parameters(declarationfacts.ChildByField(r, src, c.types), source, c, false)
		f.Parameters = p.parameters(declarationfacts.ChildByField(r, src, c.params), source, c, false)
		if bare := declarationfacts.ChildByField(r, src, c.single); bare != zero && !r.Missing(bare) {
			f.Parameters = p.parameter(bare, source, c, false)
		}
		f.Results = p.parameters(declarationfacts.ChildByField(r, src, c.results), source, c, false)
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
		if c.wrappers[r.Symbol(n)] {
			if n = declarationfacts.SingleChild(r, n); n == zero {
				return nil
			}
		}
		return []ParameterFact{{Type: r.Text(n, source), TypeStartByte: r.StartByte(n), TypeEndByte: r.EndByte(n), StartByte: r.StartByte(n), EndByte: r.EndByte(n)}}
	}
	var facts []ParameterFact
	for i := 0; i < r.ChildCount(n); i++ {
		decl := r.Child(n, i)
		if decl != zero && !r.Missing(decl) {
			facts = append(facts, p.parameter(decl, source, c, receiver)...)
		}
	}
	return facts
}

// parameter tries rows in order. A row can preserve an unnamed parameter,
// name the whole node, or select direct names by field or symbol set.
func (p *Signatures[N]) parameter(decl N, source []byte, c compiledSignature[N], receiver bool) []ParameterFact {
	var zero N
	r := p.reader
	for _, rule := range c.parameters[r.Symbol(decl)] {
		var typ N
		if rule.typ != 0 {
			typ = declarationfacts.ChildByField(r, decl, rule.typ)
			if typ != zero && r.Missing(typ) {
				typ = zero
			}
			if typ != zero && c.wrappers[r.Symbol(typ)] {
				typ = declarationfacts.SingleChild(r, typ)
			}
			if typ == zero && !rule.typeOptional {
				continue
			}
		}
		f := ParameterFact{StartByte: r.StartByte(decl), EndByte: r.EndByte(decl), Variadic: rule.variadic, Optional: rule.optional}
		if typ != zero {
			f.Type, f.TypeStartByte, f.TypeEndByte = r.Text(typ, source), r.StartByte(typ), r.EndByte(typ)
			f.Pointer = receiver && c.names.HasType(typ, c.pointer)
		}
		if rule.self {
			f.Name, f.NameStartByte, f.NameEndByte = r.Text(decl, source), r.StartByte(decl), r.EndByte(decl)
			return []ParameterFact{f}
		}
		var facts []ParameterFact
		selected := false
		for i := 0; i < r.ChildCount(decl); i++ {
			name := r.Child(decl, i)
			if rule.name != 0 {
				if r.Field(decl, i) != rule.name {
					continue
				}
			} else if name == zero || (!rule.nameTypes[r.Symbol(name)] && !rule.variadicTypes[r.Symbol(name)] && !rule.anonymous[r.Symbol(name)]) {
				continue
			}
			selected = true
			if name == zero || r.Missing(name) {
				continue
			}
			param := f
			if rule.variadicTypes[r.Symbol(name)] {
				param.Variadic = true
				if rule.unwrapVariadic {
					name = declarationfacts.SingleChild(r, name)
				}
			}
			if name == zero || r.Missing(name) {
				continue
			}
			switch {
			case rule.nameTypes[r.Symbol(name)]:
				param.Name, param.NameStartByte, param.NameEndByte = r.Text(name, source), r.StartByte(name), r.EndByte(name)
				facts = append(facts, param)
			case rule.anonymous[r.Symbol(name)]:
				facts = append(facts, param)
			}
		}
		if !selected && (rule.typ != 0 || rule.typeOptional) {
			facts = append(facts, f)
		}
		if len(facts) > 0 {
			return facts
		}
	}
	return nil
}
