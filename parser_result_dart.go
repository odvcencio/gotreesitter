package gotreesitter

// normalizeDartCompatibility repairs the dart result tree after materialization.
//
// Grammar be07cf7118d3 fixed upstream issues 102 and 103. It now parses a
// single type argument free call, such as `calloc<Size>(1)`, as a selector with
// an argument_part, which matches the C oracle. The three rewrites that
// reshaped those calls into a relational_expression chain are retired.
func normalizeDartCompatibility(root *Node, source []byte, lang *Language) {
	normalizeDartConstructorSignatureKinds(root, source, lang)
	normalizeDartComplexTypeArgumentFreeCalls(root, source, lang)
}

func normalizeDartComplexTypeArgumentFreeCalls(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "dart" || len(source) == 0 {
		return
	}
	ctx, ok := newDartComplexTypeArgumentFreeCallContext(lang)
	if !ok {
		return
	}
	walkResultTree(root, func(parent *Node) {
		for i := 0; i < resultChildCount(parent); i++ {
			child := resultChildAt(parent, i)
			callee, selector, ok := rewriteDartComplexTypeArgumentFreeCallParts(child, source, lang, ctx)
			if !ok {
				continue
			}
			replaceChildRangeWithNodes(parent, i, i+1, []*Node{callee, selector})
			i++
		}
	})
}

type dartComplexTypeArgumentFreeCallContext struct {
	selectorSym       Symbol
	selectorNamed     bool
	argumentPartSym   Symbol
	argumentPartNamed bool
	argumentsSym      Symbol
	argumentsNamed    bool
	argumentSym       Symbol
	argumentNamed     bool
	typeArgumentsSym  Symbol
	typeArgumentsName bool
	typeIdentifierSym Symbol
	typeIdentifier    bool
}

func newDartComplexTypeArgumentFreeCallContext(lang *Language) (dartComplexTypeArgumentFreeCallContext, bool) {
	var ctx dartComplexTypeArgumentFreeCallContext
	var ok bool
	if ctx.selectorSym, ok = lang.SymbolByName("selector"); !ok {
		return ctx, false
	}
	if ctx.argumentPartSym, ok = lang.SymbolByName("argument_part"); !ok {
		return ctx, false
	}
	if ctx.argumentsSym, ok = lang.SymbolByName("arguments"); !ok {
		return ctx, false
	}
	if ctx.argumentSym, ok = lang.SymbolByName("argument"); !ok {
		return ctx, false
	}
	if ctx.typeArgumentsSym, ok = lang.SymbolByName("type_arguments"); !ok {
		return ctx, false
	}
	if ctx.typeIdentifierSym, ok = lang.SymbolByName("type_identifier"); !ok {
		return ctx, false
	}
	ctx.selectorNamed = symbolIsNamed(lang, ctx.selectorSym)
	ctx.argumentPartNamed = symbolIsNamed(lang, ctx.argumentPartSym)
	ctx.argumentsNamed = symbolIsNamed(lang, ctx.argumentsSym)
	ctx.argumentNamed = symbolIsNamed(lang, ctx.argumentSym)
	ctx.typeArgumentsName = symbolIsNamed(lang, ctx.typeArgumentsSym)
	ctx.typeIdentifier = symbolIsNamed(lang, ctx.typeIdentifierSym)
	return ctx, true
}

func rewriteDartComplexTypeArgumentFreeCallParts(rel *Node, source []byte, lang *Language, ctx dartComplexTypeArgumentFreeCallContext) (*Node, *Node, bool) {
	if rel == nil || lang == nil || rel.Type(lang) != "relational_expression" || resultChildCount(rel) != 3 {
		return nil, nil, false
	}
	left := resultChildAt(rel, 0)
	greaterOp := resultChildAt(rel, 1)
	paren := resultChildAt(rel, 2)
	if left == nil || left.Type(lang) != "relational_expression" || resultChildCount(left) < 4 {
		return nil, nil, false
	}
	if !dartRelationalOperatorWrapsToken(greaterOp, lang, ">") {
		return nil, nil, false
	}
	callee := resultChildAt(left, 0)
	lessOp := resultChildAt(left, 1)
	if callee == nil || callee.Type(lang) != "identifier" || !dartRelationalOperatorWrapsToken(lessOp, lang, "<") {
		return nil, nil, false
	}
	arena := rel.ownerArena
	typeArgChildren, ok := dartComplexTypeArgumentChildren(left, lessOp, greaterOp, source, lang, ctx, arena)
	if !ok {
		return nil, nil, false
	}
	arguments, ok := dartArgumentsFromParenthesizedExpression(paren, lang, ctx, arena)
	if !ok {
		return nil, nil, false
	}
	typeArgs := newParentNodeInArena(arena, ctx.typeArgumentsSym, ctx.typeArgumentsName, typeArgChildren, nil, 0)
	argPart := newParentNodeInArena(arena, ctx.argumentPartSym, ctx.argumentPartNamed, []*Node{typeArgs, arguments}, nil, 0)
	selector := newParentNodeInArena(arena, ctx.selectorSym, ctx.selectorNamed, []*Node{argPart}, nil, 0)
	return cloneTreeNodesIntoArena(callee, arena), selector, true
}

func dartComplexTypeArgumentChildren(left, lessOp, greaterOp *Node, source []byte, lang *Language, ctx dartComplexTypeArgumentFreeCallContext, arena *nodeArena) ([]*Node, bool) {
	lessTok := dartRelationalOperatorToken(lessOp, lang, "<")
	greaterTok := dartRelationalOperatorToken(greaterOp, lang, ">")
	if lessTok == nil || greaterTok == nil {
		return nil, false
	}
	children := []*Node{cloneTreeNodesIntoArena(lessTok, arena)}
	hasSelectorTypeArguments := false
	for i := 2; i < resultChildCount(left); i++ {
		part := resultChildAt(left, i)
		converted, selectorTypeArgs, ok := dartComplexTypeArgumentPartChildren(part, source, lang, ctx, arena)
		if !ok {
			return nil, false
		}
		hasSelectorTypeArguments = hasSelectorTypeArguments || selectorTypeArgs
		children = append(children, converted...)
	}
	if !hasSelectorTypeArguments {
		return nil, false
	}
	children = append(children, cloneTreeNodesIntoArena(greaterTok, arena))
	return children, true
}

func dartComplexTypeArgumentPartChildren(part *Node, source []byte, lang *Language, ctx dartComplexTypeArgumentFreeCallContext, arena *nodeArena) ([]*Node, bool, bool) {
	if part == nil {
		return nil, false, false
	}
	switch part.Type(lang) {
	case "identifier", "type_identifier":
		return []*Node{dartCloneAsTypeIdentifier(part, ctx, arena)}, false, true
	case "selector":
		if resultChildCount(part) != 1 {
			return nil, false, false
		}
		inner := resultChildAt(part, 0)
		if inner == nil {
			return nil, false, false
		}
		switch inner.Type(lang) {
		case "type_arguments":
			if !dartTypeArgumentsContainPreFunctionTypeArguments(inner, source, lang) {
				return nil, false, false
			}
			return []*Node{cloneTreeNodesIntoArena(inner, arena)}, true, true
		case "unconditional_assignable_selector":
			if resultChildCount(inner) != 2 {
				return nil, false, false
			}
			dot := resultChildAt(inner, 0)
			ident := resultChildAt(inner, 1)
			if dot == nil || ident == nil || dot.Type(lang) != "." || ident.Type(lang) != "identifier" {
				return nil, false, false
			}
			return []*Node{
				cloneTreeNodesIntoArena(dot, arena),
				dartCloneAsTypeIdentifier(ident, ctx, arena),
			}, false, true
		default:
			return nil, false, false
		}
	default:
		return nil, false, false
	}
}

func dartTypeArgumentsContainPreFunctionTypeArguments(typeArgs *Node, source []byte, lang *Language) bool {
	if typeArgs == nil || lang == nil || typeArgs.Type(lang) != "type_arguments" {
		return false
	}
	for i := 0; i < resultChildCount(typeArgs); i++ {
		child := resultChildAt(typeArgs, i)
		if child != nil && child.Type(lang) == "function_type" && dartFunctionTypeHasGenericReturnType(child, source, lang) {
			return true
		}
	}
	return false
}

func dartFunctionTypeHasGenericReturnType(fn *Node, source []byte, lang *Language) bool {
	return dartFunctionTypeHasReturnTypeArguments(fn, lang)
}

func dartFunctionTypeHasReturnTypeArguments(fn *Node, lang *Language) bool {
	if fn == nil || lang == nil || fn.Type(lang) != "function_type" {
		return false
	}
	for i := 0; i < resultChildCount(fn); i++ {
		child := resultChildAt(fn, i)
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "Function":
			return false
		case "type_arguments":
			return true
		}
	}
	return false
}

func dartArgumentsFromParenthesizedExpression(paren *Node, lang *Language, ctx dartComplexTypeArgumentFreeCallContext, arena *nodeArena) (*Node, bool) {
	if paren == nil || paren.Type(lang) != "parenthesized_expression" || resultChildCount(paren) != 3 {
		return nil, false
	}
	open := resultChildAt(paren, 0)
	argValue := resultChildAt(paren, 1)
	close := resultChildAt(paren, 2)
	if open == nil || argValue == nil || close == nil || open.Type(lang) != "(" || close.Type(lang) != ")" {
		return nil, false
	}
	arg := newParentNodeInArena(arena, ctx.argumentSym, ctx.argumentNamed, []*Node{cloneTreeNodesIntoArena(argValue, arena)}, nil, 0)
	return newParentNodeInArena(arena, ctx.argumentsSym, ctx.argumentsNamed, []*Node{
		cloneTreeNodesIntoArena(open, arena),
		arg,
		cloneTreeNodesIntoArena(close, arena),
	}, nil, 0), true
}

func dartCloneAsTypeIdentifier(n *Node, ctx dartComplexTypeArgumentFreeCallContext, arena *nodeArena) *Node {
	cloned := cloneTreeNodesIntoArena(n, arena)
	if cloned == nil {
		return nil
	}
	cloned.symbol = ctx.typeIdentifierSym
	cloned.setNamed(ctx.typeIdentifier)
	return cloned
}

func dartRelationalOperatorWrapsToken(n *Node, lang *Language, want string) bool {
	return dartRelationalOperatorToken(n, lang, want) != nil
}

func dartRelationalOperatorToken(n *Node, lang *Language, want string) *Node {
	if n == nil || lang == nil || n.Type(lang) != "relational_operator" || resultChildCount(n) != 1 {
		return nil
	}
	child := resultChildAt(n, 0)
	if child == nil || child.Type(lang) != want {
		return nil
	}
	return child
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
	constructorNamed := symbolIsNamed(lang, constructorSym)
	walkResultTree(root, func(n *Node) {
		if dartConstructorOwnerType(n.Type(lang)) {
			className := n.ChildByFieldName("name", lang)
			body := n.ChildByFieldName("body", lang)
			if className != nil && body != nil {
				classText := className.Text(source)
				for _, member := range body.children {
					sig := dartConstructorCandidateSignature(member, lang)
					if sig == nil || len(sig.children) != 2 {
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
					sig.setNamed(constructorNamed)
					if len(sig.fieldIDs()) != len(sig.children) {
						ensureNodeFieldStorage(sig, len(sig.children))
					}
					if parametersID != 0 && len(sig.fieldIDs()) > 1 {
						sig.fieldIDs()[1] = parametersID
						if len(sig.fieldSources()) == len(sig.children) {
							sig.fieldSources()[1] = fieldSourceDirect
						}
					}
				}
			}
		}
	})
}

func dartConstructorOwnerType(typ string) bool {
	switch typ {
	case "class_definition", "enum_declaration":
		return true
	default:
		return false
	}
}

func dartConstructorCandidateSignature(member *Node, lang *Language) *Node {
	if member == nil || lang == nil {
		return nil
	}
	switch member.Type(lang) {
	case "method_signature", "declaration":
		if resultChildCount(member) != 1 {
			return nil
		}
		sig := resultChildAt(member, 0)
		if sig != nil && sig.Type(lang) == "function_signature" {
			return sig
		}
	}
	return nil
}

func dartProgramChildrenLookComplete(nodes []*Node, lang *Language) bool {
	if len(nodes) == 0 || lang == nil || lang.Name != "dart" {
		return false
	}
	seen := 0
	for _, n := range nodes {
		if n == nil || n.isExtra() {
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
