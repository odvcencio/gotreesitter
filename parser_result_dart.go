package gotreesitter

func normalizeDartCompatibility(root *Node, source []byte, lang *Language) {
	normalizeDartConstructorSignatureKinds(root, source, lang)
	normalizeDartSingleTypeArgumentFreeCalls(root, lang)
	normalizeDartSwitchExpressionBodyFields(root, lang)
}

func normalizeDartSingleTypeArgumentFreeCalls(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "dart" {
		return
	}
	relExprSym, ok := lang.SymbolByName("relational_expression")
	if !ok {
		return
	}
	relOpSym, ok := lang.SymbolByName("relational_operator")
	if !ok {
		return
	}
	parenSym, ok := lang.SymbolByName("parenthesized_expression")
	if !ok {
		return
	}
	relExprNamed := false
	if idx := int(relExprSym); idx < len(lang.SymbolMetadata) {
		relExprNamed = lang.SymbolMetadata[relExprSym].Named
	}
	relOpNamed := false
	if idx := int(relOpSym); idx < len(lang.SymbolMetadata) {
		relOpNamed = lang.SymbolMetadata[relOpSym].Named
	}
	parenNamed := false
	if idx := int(parenSym); idx < len(lang.SymbolMetadata) {
		parenNamed = lang.SymbolMetadata[parenSym].Named
	}

	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		for i := 0; i+1 < len(n.children); i++ {
			if rewriteDartSingleTypeArgumentFreeCall(n, i, lang, relExprSym, relExprNamed, relOpSym, relOpNamed, parenSym, parenNamed) {
				break
			}
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}

func normalizeDartConstructorSignatureKinds(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "dart" {
		return
	}
	constructorSym, ok := lang.SymbolByName("constructor_signature")
	if !ok {
		return
	}
	parametersID, _ := lang.FieldByName("parameters")
	constructorNamed := false
	if idx := int(constructorSym); idx < len(lang.SymbolMetadata) {
		constructorNamed = lang.SymbolMetadata[constructorSym].Named
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == "class_definition" {
			className := n.ChildByFieldName("name", lang)
			body := n.ChildByFieldName("body", lang)
			if className != nil && body != nil {
				classText := className.Text(source)
				for _, member := range body.children {
					if member == nil || member.Type(lang) != "method_signature" || len(member.children) != 1 {
						continue
					}
					sig := member.children[0]
					if sig == nil || sig.Type(lang) != "function_signature" || len(sig.children) != 2 {
						continue
					}
					name := sig.children[0]
					params := sig.children[1]
					if name == nil || params == nil || name.Type(lang) != "identifier" || params.Type(lang) != "formal_parameter_list" {
						continue
					}
					if name.Text(source) != classText {
						continue
					}
					sig.symbol = constructorSym
					sig.isNamed = constructorNamed
					if len(sig.fieldIDs) != len(sig.children) {
						ensureNodeFieldStorage(sig, len(sig.children))
					}
					if parametersID != 0 && len(sig.fieldIDs) > 1 {
						sig.fieldIDs[1] = parametersID
						if len(sig.fieldSources) == len(sig.children) {
							sig.fieldSources[1] = fieldSourceDirect
						}
					}
				}
			}
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}

func normalizeDartSwitchExpressionBodyFields(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "dart" {
		return
	}
	bodyID, ok := lang.FieldByName("body")
	if !ok {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == "switch_expression" && len(n.children) > 0 {
			ensureNodeFieldStorage(n, len(n.children))
			start := -1
			for i := 0; i < len(n.children); i++ {
				if n.fieldIDs[i] == bodyID {
					start = i
					break
				}
			}
			if start >= 0 {
				for i := start; i < len(n.children); i++ {
					if n.children[i] == nil {
						continue
					}
					n.fieldIDs[i] = bodyID
					if len(n.fieldSources) == len(n.children) {
						n.fieldSources[i] = fieldSourceDirect
					}
				}
			}
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}
func rewriteDartSingleTypeArgumentFreeCall(parent *Node, idx int, lang *Language, relExprSym Symbol, relExprNamed bool, relOpSym Symbol, relOpNamed bool, parenSym Symbol, parenNamed bool) bool {
	if parent == nil || idx < 0 || idx+1 >= len(parent.children) || lang == nil {
		return false
	}
	callee := parent.children[idx]
	selector := parent.children[idx+1]
	if callee == nil || selector == nil || callee.Type(lang) != "identifier" || selector.Type(lang) != "selector" || len(selector.children) != 1 {
		return false
	}
	argPart := selector.children[0]
	if argPart == nil || argPart.Type(lang) != "argument_part" || len(argPart.children) != 2 {
		return false
	}
	typeArgs := argPart.children[0]
	args := argPart.children[1]
	if typeArgs == nil || args == nil || typeArgs.Type(lang) != "type_arguments" || args.Type(lang) != "arguments" {
		return false
	}
	typeIdent, lt, gt, ok := dartSimpleTypeArgumentParts(typeArgs, lang)
	if !ok {
		return false
	}
	if len(args.children) < 2 {
		return false
	}

	arena := parent.ownerArena
	if typeIdent.Type(lang) == "type_identifier" {
		identSym, ok := lang.SymbolByName("identifier")
		if !ok {
			return false
		}
		identNamed := false
		if idx := int(identSym); idx < len(lang.SymbolMetadata) {
			identNamed = lang.SymbolMetadata[identSym].Named
		}
		typeIdent = newLeafNodeInArena(arena, identSym, identNamed, typeIdent.startByte, typeIdent.endByte, typeIdent.startPoint, typeIdent.endPoint)
	}
	lessOp := newParentNodeInArena(arena, relOpSym, relOpNamed, []*Node{lt}, nil, 0)
	left := newParentNodeInArena(arena, relExprSym, relExprNamed, []*Node{callee, lessOp, typeIdent}, nil, 0)
	greaterOp := newParentNodeInArena(arena, relOpSym, relOpNamed, []*Node{gt}, nil, 0)
	parenChildren := dartParenthesizedExpressionChildren(args, lang)
	paren := newParentNodeInArena(arena, parenSym, parenNamed, parenChildren, nil, args.productionID)
	outer := newParentNodeInArena(arena, relExprSym, relExprNamed, []*Node{left, greaterOp, paren}, nil, 0)
	replaceChildRangeWithSingleNode(parent, idx, idx+2, outer)
	return true
}

func dartSimpleTypeArgumentParts(typeArgs *Node, lang *Language) (*Node, *Node, *Node, bool) {
	if typeArgs == nil || lang == nil || typeArgs.Type(lang) != "type_arguments" || len(typeArgs.children) < 3 {
		return nil, nil, nil, false
	}
	lt := typeArgs.children[0]
	gt := typeArgs.children[len(typeArgs.children)-1]
	if lt == nil || gt == nil || lt.Type(lang) != "<" || gt.Type(lang) != ">" {
		return nil, nil, nil, false
	}
	if got := typeArgs.NamedChildCount(); got != 1 {
		return nil, nil, nil, false
	}
	typeIdent := typeArgs.NamedChild(0)
	if typeIdent == nil || typeIdent.Type(lang) != "type_identifier" || nodeContainsNamedType(typeIdent, lang, "type_arguments") {
		return nil, nil, nil, false
	}
	return typeIdent, lt, gt, true
}

func nodeContainsNamedType(root *Node, lang *Language, want string) bool {
	if root == nil || lang == nil {
		return false
	}
	for _, child := range root.children {
		if child == nil {
			continue
		}
		if child.Type(lang) == want {
			return true
		}
		if nodeContainsNamedType(child, lang, want) {
			return true
		}
	}
	return false
}

func dartParenthesizedExpressionChildren(args *Node, lang *Language) []*Node {
	if args == nil || lang == nil {
		return nil
	}
	if len(args.children) != 3 {
		return append([]*Node(nil), args.children...)
	}
	open := args.children[0]
	mid := args.children[1]
	close := args.children[2]
	if open == nil || mid == nil || close == nil {
		return append([]*Node(nil), args.children...)
	}
	if mid.Type(lang) != "argument" || len(mid.children) != 1 || mid.children[0] == nil {
		return append([]*Node(nil), args.children...)
	}
	return []*Node{open, mid.children[0], close}
}
func dartProgramChildrenLookComplete(nodes []*Node, lang *Language) bool {
	if len(nodes) == 0 || lang == nil || lang.Name != "dart" {
		return false
	}
	seen := 0
	for _, n := range nodes {
		if n == nil || n.isExtra {
			continue
		}
		if n.IsNamed() {
			seen++
			continue
		}
		switch n.Type(lang) {
		case ";":
			seen++
		default:
			return false
		}
	}
	return seen > 0
}
