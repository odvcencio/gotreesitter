package gotreesitter

func normalizePerlCompatibility(root *Node, source []byte, lang *Language) {
	normalizePerlPushExpressionLists(root, source, lang)
}

func normalizePerlPushExpressionLists(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "perl" {
		return
	}
	listSym, ok := lang.SymbolByName("list_expression")
	if !ok {
		return
	}
	listNamed := symbolIsNamed(lang, listSym)
	walkResultTree(root, func(n *Node) {
		if n.Type(lang) == "expression_statement" && len(n.children) == 1 {
			list := n.children[0]
			if rewritten := rewritePerlPushExpressionList(n.ownerArena, list, source, lang, listSym, listNamed); rewritten != nil {
				n.children[0] = rewritten
				rewritten.parent = n
				rewritten.childIndex = 0
			}
		}
	})
}

func rewritePerlPushExpressionList(arena *nodeArena, list *Node, source []byte, lang *Language, listSym Symbol, listNamed bool) *Node {
	if list == nil || list.Type(lang) != "list_expression" || len(list.children) < 3 {
		return nil
	}
	call := list.children[0]
	if call == nil || call.Type(lang) != "ambiguous_function_call_expression" || len(call.children) != 2 {
		return nil
	}
	fn := call.children[0]
	firstArg := call.children[1]
	if fn == nil || firstArg == nil || fn.Text(source) != "push" {
		return nil
	}
	argChildren := make([]*Node, 0, len(list.children))
	argChildren = append(argChildren, firstArg)
	argChildren = append(argChildren, list.children[1:]...)
	rewrittenArgs := newParentNodeInArena(arena, listSym, listNamed, argChildren, nil, list.productionID)

	callFieldIDs := append([]FieldID(nil), call.fieldIDs()...)
	if len(callFieldIDs) > 2 {
		callFieldIDs = callFieldIDs[:2]
	}
	rewrittenCall := newParentNodeInArena(arena, call.symbol, call.isNamed(), []*Node{fn, rewrittenArgs}, callFieldIDs, call.productionID)
	if callFieldSources := call.fieldSources(); len(callFieldSources) > 0 {
		rewrittenCallFieldSources := append([]uint8(nil), callFieldSources...)
		if len(rewrittenCallFieldSources) > 2 {
			rewrittenCallFieldSources = rewrittenCallFieldSources[:2]
		}
		rewrittenCall.setFieldSources(rewrittenCallFieldSources)
	}
	return rewrittenCall
}
