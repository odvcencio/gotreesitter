package gotreesitter

type unaryWrapperFlatteningKey struct {
	parent              Symbol
	wrapper             Symbol
	wrapperPreGotoState StateID
}

func compileUnaryWrapperFlatteningPolicy(lang *Language) map[unaryWrapperFlatteningKey]Symbol {
	if lang == nil || len(lang.NativeUnaryWrapperFlattening) == 0 {
		return nil
	}
	rules := make(map[unaryWrapperFlatteningKey]Symbol, len(lang.NativeUnaryWrapperFlattening))
	for _, rule := range lang.NativeUnaryWrapperFlattening {
		if !validUnaryWrapperFlatteningRule(lang, rule) {
			continue
		}
		key := unaryWrapperFlatteningKey{
			parent:              rule.PublicParent,
			wrapper:             rule.Wrapper,
			wrapperPreGotoState: rule.WrapperPreGotoState,
		}
		rules[key] = rule.Leaf
	}
	if len(rules) == 0 {
		return nil
	}
	return rules
}

func validUnaryWrapperFlatteningRule(lang *Language, rule UnaryWrapperFlatteningRule) bool {
	if lang == nil ||
		rule.PublicParent == rule.Wrapper ||
		rule.PublicParent == rule.Leaf ||
		rule.Wrapper == rule.Leaf ||
		uint32(rule.WrapperPreGotoState) >= lang.StateCount {
		return false
	}
	meta := lang.SymbolMetadata
	for _, symbol := range []Symbol{rule.PublicParent, rule.Wrapper, rule.Leaf} {
		if int(symbol) >= len(meta) || !meta[symbol].Visible || !meta[symbol].Named {
			return false
		}
	}
	return true
}

func (p *Parser) flattenNativeUnaryWrapperChildren(parent Symbol, children []*Node) []*Node {
	if p == nil || len(p.unaryWrapperFlatteningSet) == 0 || len(children) == 0 {
		return children
	}
	for index, wrapper := range children {
		if wrapper == nil {
			continue
		}
		key := unaryWrapperFlatteningKey{
			parent:              parent,
			wrapper:             wrapper.symbol,
			wrapperPreGotoState: wrapper.preGotoState,
		}
		leafSymbol, ok := p.unaryWrapperFlatteningSet[key]
		if !ok ||
			wrapper.isExtra() ||
			wrapper.isMissing() ||
			wrapper.hasError() ||
			nodeChildCountNoMaterialize(wrapper) != 1 {
			continue
		}
		entry, ok := nodeChildEntryAtNoMaterialize(wrapper, 0)
		if !ok {
			continue
		}
		leaf := stackEntryNode(entry)
		if leaf == nil ||
			leaf.symbol != leafSymbol ||
			leaf.isExtra() ||
			leaf.isMissing() ||
			leaf.hasError() ||
			nodeChildCountNoMaterialize(leaf) != 0 ||
			wrapper.startByte != leaf.startByte ||
			wrapper.endByte != leaf.endByte {
			continue
		}
		children[index] = leaf
	}
	return children
}
