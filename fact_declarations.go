package gotreesitter

// DeclarationRule describes one declaration shape using grammar data only.
// Names must be direct children in NameField with exactly NameNodeType.
// Every nonblank name emits a sibling span covering the whole NodeType node.
// A rule never searches descendants for a guessed name.
//
// Ancestors lists an exact parent chain, nearest first. ContainerNameField
// and ContainerNameNodeType select a single name on its last ancestor;
// ContainerTypeField must link that ancestor to the preceding ancestor.
// An empty ContainerNameField means the rule has no container.
//
// TypeField/TypeNodeType optionally constrain the declaration's direct type
// child. Shape is set only on Kind "type". For several matching rules on one
// node, the first applicable rule wins. Missing grammar data fails closed.
type DeclarationRule struct {
	NodeType              string
	Kind                  string
	NameField             string
	NameNodeType          string
	Ancestors             []string
	TypeField             string
	TypeNodeType          string
	Shape                 string
	ContainerNameField    string
	ContainerNameNodeType string
	ContainerTypeField    string
}

// FactProgramOption configures a FactProgram.
type FactProgramOption func(*FactProgram)

// WithDeclarationRules attaches grammar-owned declaration rules. A later
// option replaces earlier rules. The option has no effect unless
// FactDeclarations is selected; ordinary programs compile no extra operations
// and allocate no rule storage. NewFactProgram takes its own copy of the data.
func WithDeclarationRules(rules []DeclarationRule) FactProgramOption {
	return func(p *FactProgram) {
		if p.kinds&FactDeclarations == 0 {
			return
		}
		p.declarationRules = rules
	}
}

// compiledDeclarationRule contains only resolved symbols and fields.
type compiledDeclarationRule struct {
	kind, shape        string
	nameField          FieldID
	nameType           Symbol
	ancestors          []Symbol
	typeField          FieldID
	typeNode           Symbol
	containerNameField FieldID
	containerNameType  Symbol
	containerTypeField FieldID
}

func (p *FactProgram) compileDeclarationInstructions() {
	if p.kinds&FactDeclarations == 0 {
		return
	}
	for _, rule := range p.declarationRules {
		compiled, ok := compileDeclarationRule(p.language, rule)
		if !ok {
			continue
		}
		for symbol, name := range p.language.SymbolNames {
			if name != rule.NodeType {
				continue
			}
			if p.declarations == nil {
				p.declarations = make(map[Symbol][]compiledDeclarationRule)
			}
			p.declarations[Symbol(symbol)] = append(p.declarations[Symbol(symbol)], compiled)
			p.code[symbol] = p.code[symbol]&^factDefinitionKindMask | factInstruction(factDefinitionDeclaration)
			p.hasOperations = true
		}
	}
	p.declarationRules = nil
}

func compileDeclarationRule(lang *Language, rule DeclarationRule) (compiledDeclarationRule, bool) {
	var compiled compiledDeclarationRule
	if rule.Kind == "" || rule.NodeType == "" || rule.NameField == "" || rule.NameNodeType == "" {
		return compiled, false
	}
	if _, ok := lang.SymbolByName(rule.NodeType); !ok {
		return compiled, false
	}
	var ok bool
	compiled.kind, compiled.shape = rule.Kind, rule.Shape
	if rule.Kind != "type" {
		compiled.shape = ""
	}
	if compiled.nameField, ok = lang.FieldByName(rule.NameField); !ok {
		return compiled, false
	}
	if compiled.nameType, ok = lang.SymbolByName(rule.NameNodeType); !ok {
		return compiled, false
	}
	for _, ancestor := range rule.Ancestors {
		symbol, ok := lang.SymbolByName(ancestor)
		if !ok {
			return compiled, false
		}
		compiled.ancestors = append(compiled.ancestors, symbol)
	}
	if rule.TypeField != "" || rule.TypeNodeType != "" {
		if compiled.typeField, ok = lang.FieldByName(rule.TypeField); !ok {
			return compiled, false
		}
		if compiled.typeNode, ok = lang.SymbolByName(rule.TypeNodeType); !ok {
			return compiled, false
		}
	}
	if rule.ContainerNameField != "" || rule.ContainerNameNodeType != "" || rule.ContainerTypeField != "" {
		if len(compiled.ancestors) < 2 {
			return compiled, false
		}
		if compiled.containerNameField, ok = lang.FieldByName(rule.ContainerNameField); !ok {
			return compiled, false
		}
		if compiled.containerNameType, ok = lang.SymbolByName(rule.ContainerNameNodeType); !ok {
			return compiled, false
		}
		if compiled.containerTypeField, ok = lang.FieldByName(rule.ContainerTypeField); !ok {
			return compiled, false
		}
	}
	return compiled, true
}

func (p *FactProgram) appendDeclarationSpans(n *Node, instruction factInstruction, source []byte, facts *FactSet) {
	for _, rule := range p.declarations[n.Symbol()] {
		if rule.typeField != 0 {
			typ := factChildByField(n, rule.typeField)
			if typ == nil || typ.Symbol() != rule.typeNode {
				continue
			}
		}
		ancestor, child := n, n
		matches := true
		for _, symbol := range rule.ancestors {
			child, ancestor = ancestor, ancestor.Parent()
			if ancestor == nil || ancestor.Symbol() != symbol {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		var containerName string
		if rule.containerNameField != 0 {
			// The containing type must directly declare this struct/interface body.
			// A nearest named ancestor alone would wrongly own anonymous members.
			if factChildByField(ancestor, rule.containerTypeField) != child {
				continue
			}
			name := declarationSingleName(ancestor, rule.containerNameField, rule.containerNameType)
			if name == nil {
				continue
			}
			containerName = name.Text(source)
			if containerName == "" || containerName == "_" {
				continue
			}
		}
		count := nodeChildCountNoMaterialize(n)
		for i := 0; i < count; i++ {
			if nodeFieldIDAt(n, i) != rule.nameField {
				continue
			}
			nameNode := nodeChildAtForReason(n, i, materializeForParentAPI)
			if nameNode == nil || nameNode.Symbol() != rule.nameType || nameNode.IsMissing() {
				continue
			}
			name := nameNode.Text(source)
			if name == "" || name == "_" {
				continue
			}
			span := DefinitionSpan{
				Lang: p.language.Name, Kind: rule.kind, Name: name, NodeType: n.Type(p.language),
				StartByte: n.StartByte(), EndByte: n.EndByte(),
				NameStartByte: nameNode.StartByte(), NameEndByte: nameNode.EndByte(),
				Shape: rule.shape, Container: containerName,
			}
			if containerName != "" {
				span.ContainerStartByte, span.ContainerEndByte = ancestor.StartByte(), ancestor.EndByte()
			}
			facts.Definitions = append(facts.Definitions, span)
			if instruction&factOpHeritage != 0 {
				appendHeritageForNodeWithSpan(span, n, p.language, source, &facts.Heritage)
			}
		}
		return
	}
}

func declarationSingleName(n *Node, field FieldID, nodeType Symbol) *Node {
	var name *Node
	count := nodeChildCountNoMaterialize(n)
	for i := 0; i < count; i++ {
		if nodeFieldIDAt(n, i) != field {
			continue
		}
		candidate := nodeChildAtForReason(n, i, materializeForParentAPI)
		if candidate == nil || candidate.Symbol() != nodeType || candidate.IsMissing() || name != nil {
			return nil
		}
		name = candidate
	}
	return name
}
