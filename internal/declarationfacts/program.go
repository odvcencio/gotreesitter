// Package declarationfacts compiles grammar-owned declaration rules and projects
// their facts without depending on the parser or its public facade.
package declarationfacts

// Rule describes a declaration using direct name fields and exact parent paths.
// Ancestors lists parents nearest first; the last is the optional container.
// ContainerTypeField must link the container to the preceding ancestor.
// TypeField/TypeNodeType optionally constrain the node's direct type child.
// The first applicable rule wins; missing grammar data fails closed.
type Rule struct {
	NodeType, Kind, NameField, NameNodeType                       string
	Ancestors                                                     []string
	TypeField, TypeNodeType, Shape                                string
	ContainerNameField, ContainerNameNodeType, ContainerTypeField string
	// BaseName selects an embedded type through TypeNames; BaseNameField is
	// empty for a node with one named child. ExcludeNames filters bare constraint terms.
	BaseName      bool
	BaseNameField string
	Embedded      bool
	ExcludeNames  []string
	TypeNames     []TypeNameRule
	// ContainerPath replaces fixed container ownership with recursive exact paths.
	ContainerPath []ContainerRule
}

// Fact describes one declared name. Equal construct ranges mean siblings;
// each name retains its own name range. Metadata is zero when not applicable.
type Fact struct {
	Lang, Kind, Name, NodeType                     string
	StartByte, EndByte, NameStartByte, NameEndByte uint32
	Shape                                          string `json:",omitempty"`
	Container                                      string `json:",omitempty"`
	ContainerStartByte                             uint32 `json:",omitempty"`
	ContainerEndByte                               uint32 `json:",omitempty"`
	Embedded                                       bool   `json:",omitempty"`
}

// Grammar resolves the symbol and field data needed to compile rules.
type Grammar struct {
	Names  []string
	Symbol func(string) (uint16, bool)
	Field  func(string) (uint16, bool)
}

// Reader adapts the parser's node representation without materializing wrappers.
type Reader[N comparable] struct {
	Language           string
	Symbol             func(N) uint16
	Parent             func(N) N
	ChildCount         func(N) int
	Child              func(N, int) N
	Field              func(N, int) uint16
	Missing            func(N) bool
	Named              func(N) bool
	Text               func(N, []byte) string
	NodeType           func(N) string
	StartByte, EndByte func(N) uint32
}

type compiledRule struct {
	kind, shape                                               string
	nameField, nameType                                       uint16
	ancestors                                                 []uint16
	typeField, typeNode                                       uint16
	containerNameField, containerNameType, containerTypeField uint16
	baseName                                                  bool
	baseNameField                                             uint16
	embedded                                                  bool
	excludeNames                                              []string
}

// Program holds immutable compiled rules. Concurrent extractions need separate
// destination slices. Compile returns nil when no rule can be resolved.
type Program[N comparable] struct {
	reader Reader[N]
	rules  map[uint16][]compiledRule
	names  map[uint16][]*Names[N]
}

// Compile resolves grammar data once and owns all retained ancestor storage.
func Compile[N comparable](grammar Grammar, reader Reader[N], rules []Rule) *Program[N] {
	var program *Program[N]
	for _, rule := range rules {
		compiled, ok := compileRule(grammar, rule)
		names, validNames := CompileNames(grammar, reader, rule.TypeNames, rule.ContainerPath)
		ok = ok && validNames
		if !ok {
			continue
		}
		for symbol, name := range grammar.Names {
			if name != rule.NodeType {
				continue
			}
			if program == nil {
				program = &Program[N]{reader: reader, rules: make(map[uint16][]compiledRule), names: make(map[uint16][]*Names[N])}
			}
			program.rules[uint16(symbol)] = append(program.rules[uint16(symbol)], compiled)
			program.names[uint16(symbol)] = append(program.names[uint16(symbol)], names)
		}
	}
	return program
}

func compileRule(grammar Grammar, rule Rule) (compiledRule, bool) {
	var compiled compiledRule
	if rule.Kind == "" || rule.NodeType == "" || (!rule.BaseName && (rule.NameField == "" || rule.NameNodeType == "")) {
		return compiled, false
	}
	if _, ok := grammar.Symbol(rule.NodeType); !ok {
		return compiled, false
	}
	var ok bool
	compiled.kind, compiled.shape = rule.Kind, rule.Shape
	if rule.Kind != "type" {
		compiled.shape = ""
	}
	if rule.NameField != "" {
		if compiled.nameField, ok = grammar.Field(rule.NameField); !ok {
			return compiled, false
		}
	}
	if rule.NameNodeType != "" {
		if compiled.nameType, ok = grammar.Symbol(rule.NameNodeType); !ok {
			return compiled, false
		}
	}
	compiled.baseName, compiled.embedded = rule.BaseName, rule.Embedded
	compiled.excludeNames = append([]string(nil), rule.ExcludeNames...)
	if rule.BaseNameField != "" {
		if compiled.baseNameField, ok = grammar.Field(rule.BaseNameField); !ok {
			return compiled, false
		}
	}
	for _, ancestor := range rule.Ancestors {
		symbol, ok := grammar.Symbol(ancestor)
		if !ok {
			return compiled, false
		}
		compiled.ancestors = append(compiled.ancestors, symbol)
	}
	if rule.TypeField != "" || rule.TypeNodeType != "" {
		if compiled.typeField, ok = grammar.Field(rule.TypeField); !ok {
			return compiled, false
		}
		if compiled.typeNode, ok = grammar.Symbol(rule.TypeNodeType); !ok {
			return compiled, false
		}
	}
	if rule.ContainerNameField != "" || rule.ContainerNameNodeType != "" || rule.ContainerTypeField != "" {
		if len(compiled.ancestors) < 2 {
			return compiled, false
		}
		if compiled.containerNameField, ok = grammar.Field(rule.ContainerNameField); !ok {
			return compiled, false
		}
		if compiled.containerNameType, ok = grammar.Symbol(rule.ContainerNameNodeType); !ok {
			return compiled, false
		}
		if compiled.containerTypeField, ok = grammar.Field(rule.ContainerTypeField); !ok {
			return compiled, false
		}
	}
	return compiled, true
}

// Extract appends facts in source-tree order. A nil program or root does nothing.
func (p *Program[N]) Extract(root N, source []byte, dst *[]Fact) {
	var zero N
	if p == nil || root == zero || dst == nil {
		return
	}
	p.walk(root, source, dst)
}

func (p *Program[N]) walk(n N, source []byte, dst *[]Fact) {
	var zero N
	if n == zero {
		return
	}
	p.appendFacts(n, source, dst)
	for i, count := 0, p.reader.ChildCount(n); i < count; i++ {
		p.walk(p.reader.Child(n, i), source, dst)
	}
}

func (p *Program[N]) childByField(n N, field uint16) N {
	for i, count := 0, p.reader.ChildCount(n); i < count; i++ {
		if p.reader.Field(n, i) == field {
			return p.reader.Child(n, i)
		}
	}
	var zero N
	return zero
}

func (p *Program[N]) appendFacts(n N, source []byte, dst *[]Fact) {
	var zero N
	r := p.reader
	for index, rule := range p.rules[r.Symbol(n)] {
		if rule.typeField != 0 {
			typ := p.childByField(n, rule.typeField)
			if typ == zero || r.Symbol(typ) != rule.typeNode {
				continue
			}
		}
		ancestor, child := n, n
		matches := true
		for _, symbol := range rule.ancestors {
			child, ancestor = ancestor, r.Parent(ancestor)
			if ancestor == zero || r.Symbol(ancestor) != symbol {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		names := p.names[r.Symbol(n)][index]
		var containers []Container[N]
		if len(names.containers) > 0 {
			containers = names.Containers(ancestor, source)
			if len(containers) == 0 {
				continue
			}
		}
		var containerName string
		if rule.containerNameField != 0 {
			// A named ancestor must declare this exact body, not an anonymous body
			// nested inside it. Never infer ownership from the nearest name alone.
			if p.childByField(ancestor, rule.containerTypeField) != child {
				continue
			}
			name := p.singleName(ancestor, rule.containerNameField, rule.containerNameType)
			if name == zero {
				continue
			}
			containerName = r.Text(name, source)
			if containerName == "" || containerName == "_" {
				continue
			}
		}
		if rule.baseName {
			// Named fields must never also become embeddings.
			if rule.nameField != 0 && p.childByField(n, rule.nameField) != zero {
				continue
			}
			typ := SingleChild(r, n)
			if rule.baseNameField != 0 {
				typ = p.childByField(n, rule.baseNameField)
			}
			nameNode := names.Base(typ)
			if nameNode == zero {
				continue
			}
			name := r.Text(nameNode, source)
			excluded := name == "" || name == "_"
			// A qualified identifier is a named reference, even when its base
			// spelling matches a predeclared constraint term.
			if nameNode == typ {
				for _, term := range rule.excludeNames {
					if name == term {
						excluded = true
					}
				}
			}
			if excluded {
				continue
			}
			f := Fact{Lang: r.Language, Kind: rule.kind, Name: name, NodeType: r.NodeType(n), StartByte: r.StartByte(n), EndByte: r.EndByte(n), NameStartByte: r.StartByte(nameNode), NameEndByte: r.EndByte(nameNode), Embedded: rule.embedded, Container: containerName}
			if containerName != "" {
				f.ContainerStartByte, f.ContainerEndByte = r.StartByte(ancestor), r.EndByte(ancestor)
			}
			p.appendContainers(f, containers, dst)
			return
		}
		before := len(*dst)
		for i, count := 0, r.ChildCount(n); i < count; i++ {
			if r.Field(n, i) != rule.nameField {
				continue
			}
			nameNode := r.Child(n, i)
			if nameNode == zero || r.Symbol(nameNode) != rule.nameType || r.Missing(nameNode) {
				continue
			}
			name := r.Text(nameNode, source)
			if name == "" || name == "_" {
				continue
			}
			fact := Fact{
				Lang: r.Language, Kind: rule.kind, Name: name, NodeType: r.NodeType(n),
				StartByte: r.StartByte(n), EndByte: r.EndByte(n),
				NameStartByte: r.StartByte(nameNode), NameEndByte: r.EndByte(nameNode),
				Shape: rule.shape, Container: containerName,
			}
			if containerName != "" {
				fact.ContainerStartByte, fact.ContainerEndByte = r.StartByte(ancestor), r.EndByte(ancestor)
			}
			p.appendContainers(fact, containers, dst)
		}
		if len(*dst) > before {
			return
		}
	}
}

func (p *Program[N]) singleName(n N, field, nodeType uint16) N {
	var zero, name N
	for i, count := 0, p.reader.ChildCount(n); i < count; i++ {
		if p.reader.Field(n, i) != field {
			continue
		}
		candidate := p.reader.Child(n, i)
		if candidate == zero || p.reader.Symbol(candidate) != nodeType || p.reader.Missing(candidate) || name != zero {
			return zero
		}
		name = candidate
	}
	return name
}

func (p *Program[N]) appendContainers(f Fact, containers []Container[N], dst *[]Fact) {
	if len(containers) == 0 {
		*dst = append(*dst, f)
		return
	}
	for _, c := range containers {
		f.Container = c.Name
		f.ContainerStartByte = p.reader.StartByte(c.Node)
		f.ContainerEndByte = p.reader.EndByte(c.Node)
		*dst = append(*dst, f)
	}
}

// ValidRule reports whether all of a rule's grammar references resolve.
func ValidRule(g Grammar, rule Rule) bool {
	_, ok := compileRule(g, rule)
	_, namesOK := CompileNames(g, Reader[int]{}, rule.TypeNames, rule.ContainerPath)
	return ok && namesOK
}
