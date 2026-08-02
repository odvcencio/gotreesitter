package gotreesitter

// collapsePythonRootFragments and its three collapse* helpers below
// (collapsePythonClassFragments, collapsePythonFunctionFragments,
// collapsePythonTerminalIfSuffix) no longer force setHasError(false) on the
// block/class/function/if nodes they build; see each helper's own comment.
// An independent PR #636 review, and an isolated-revert re-sweep performed
// here (re-adding the removed setHasError(false) calls, then re-running the
// full incremental-gate corpus, 5,241 sites across all three edit classes),
// both found zero witnesses where restoring the old override changes the
// final tree's root HasError(): whatever intermediate error state these
// helpers construct on the way in is fully resolved by the time
// dropZeroWidthUnnamedTail/repairPythonNode-style repair finishes. (An
// unrelated root-level mechanism, syntheticRootCanDropError /
// pythonModuleChildrenLookComplete, used to also catch a residual case here;
// a follow-up measurement found it inert on every input that reached it and
// it was deleted outright -- elm/synthetic-root-drop-retirement.) The
// removals are kept anyway, as defense in depth: forcing HasError to false
// regardless of children is never correct per populateParentNode's own
// contract, and removing that override costs nothing. If a future input
// reaches these functions with an error that genuinely does survive to the
// final tree, populateParentNode's own children-OR propagation is what keeps
// it visible at the root.
func collapsePythonRootFragments(nodes []*Node, arena *nodeArena, lang *Language) []*Node {
	if len(nodes) == 0 || lang == nil || lang.Name != "python" {
		return nodes
	}
	nodes = dropZeroWidthUnnamedTail(nodes, lang)
	for {
		next, changed := collapsePythonClassFragments(nodes, arena, lang)
		if changed {
			nodes = next
			nodes = dropZeroWidthUnnamedTail(nodes, lang)
			continue
		}
		next, changed = collapsePythonFunctionFragments(nodes, arena, lang)
		if changed {
			nodes = next
			nodes = dropZeroWidthUnnamedTail(nodes, lang)
			continue
		}
		next, changed = collapsePythonTerminalIfSuffix(nodes, arena, lang)
		if changed {
			nodes = next
			nodes = dropZeroWidthUnnamedTail(nodes, lang)
			continue
		}
		return normalizePythonModuleChildren(nodes, arena, lang)
	}
}

func collapsePythonClassFragments(nodes []*Node, arena *nodeArena, lang *Language) ([]*Node, bool) {
	if len(nodes) < 5 {
		return nodes, false
	}
	classDefSym, ok := symbolByName(lang, "class_definition")
	if !ok {
		return nodes, false
	}
	blockSym, ok := symbolByName(lang, "block")
	if !ok {
		return nodes, false
	}
	for i := 0; i < len(nodes)-4; i++ {
		j := i
		classNode := nodes[j]
		nameNode := nodes[j+1]
		if classNode == nil || nameNode == nil {
			continue
		}
		if classNode.Type(lang) != "class" || nameNode.Type(lang) != "identifier" {
			continue
		}
		var argNode *Node
		if j+5 < len(nodes) && nodes[j+2] != nil && nodes[j+2].Type(lang) == "argument_list" {
			argNode = nodes[j+2]
			j++
		}
		colonNode := nodes[j+2]
		indentNode := nodes[j+3]
		bodyNode := nodes[j+4]
		if colonNode == nil || indentNode == nil || bodyNode == nil {
			continue
		}
		if colonNode.Type(lang) != ":" || indentNode.Type(lang) != "_indent" {
			continue
		}

		bodyStart := j + 4
		bodyEnd := bodyStart + 1
		var bodyChildren []*Node
		if bodyNode.Type(lang) == "module_repeat1" {
			bodyChildren = flattenPythonModuleRepeat(bodyNode, nil, lang)
		} else {
			var ok bool
			bodyChildren, bodyEnd, ok = pythonCollectIndentedSuite(nodes, bodyStart, classNode.startPoint.Column)
			if !ok {
				continue
			}
			bodyChildren = collapsePythonRootFragments(bodyChildren, arena, lang)
		}
		if len(bodyChildren) == 0 {
			continue
		}
		if arena != nil {
			buf := arena.allocNodeSlice(len(bodyChildren))
			copy(buf, bodyChildren)
			bodyChildren = buf
		}
		// blockNode/classDef are freshly built by newParentNodeInArena, which
		// already runs populateParentNode over their exact children -- HasError
		// is already the correct OR-over-children value (bodyChildren for
		// blockNode; classChildren, which includes blockNode, for classDef).
		// Do NOT force it to false here: a body fragment can be shaped exactly
		// like a complete class body while still containing a genuinely
		// erroring descendant several levels down, and forcing false would
		// silently discard that signal on its way up to the module root.
		blockNode := newParentNodeInArena(arena, blockSym, true, bodyChildren, nil, 0)
		if repairedBlock, changed := repairPythonBlock(blockNode, arena, lang, true); changed {
			blockNode = repairedBlock
		}

		classChildren := make([]*Node, 0, 5)
		classChildren = append(classChildren, classNode, nameNode)
		if argNode != nil {
			classChildren = append(classChildren, argNode)
		}
		classChildren = append(classChildren, colonNode, blockNode)
		if arena != nil {
			buf := arena.allocNodeSlice(len(classChildren))
			copy(buf, classChildren)
			classChildren = buf
		}
		classFieldIDs := pythonSyntheticClassFieldIDs(arena, len(classChildren), argNode != nil, lang)
		classDef := newParentNodeInArena(arena, classDefSym, true, classChildren, classFieldIDs, 0)

		out := make([]*Node, 0, len(nodes)-(bodyEnd-i)+1)
		out = append(out, nodes[:i]...)
		out = append(out, classDef)
		out = append(out, nodes[bodyEnd:]...)
		if arena != nil {
			buf := arena.allocNodeSlice(len(out))
			copy(buf, out)
			out = buf
		}
		return out, true
	}
	return nodes, false
}

func collapsePythonFunctionFragments(nodes []*Node, arena *nodeArena, lang *Language) ([]*Node, bool) {
	if len(nodes) < 6 || lang == nil || lang.Name != "python" {
		return nodes, false
	}
	functionDefSym, ok := symbolByName(lang, "function_definition")
	if !ok {
		return nodes, false
	}
	blockSym, ok := symbolByName(lang, "block")
	if !ok {
		return nodes, false
	}
	for i := 0; i < len(nodes)-5; i++ {
		defNode := nodes[i]
		nameNode := nodes[i+1]
		paramsNode := nodes[i+2]
		colonNode := nodes[i+3]
		indentNode := nodes[i+4]
		if defNode == nil || nameNode == nil || paramsNode == nil || colonNode == nil || indentNode == nil {
			continue
		}
		if defNode.Type(lang) != "def" || nameNode.Type(lang) != "identifier" || paramsNode.Type(lang) != "parameters" {
			continue
		}
		if colonNode.Type(lang) != ":" || indentNode.Type(lang) != "_indent" {
			continue
		}
		bodyChildren, bodyEnd, ok := pythonCollectIndentedSuite(nodes, i+5, defNode.startPoint.Column)
		if !ok {
			continue
		}
		bodyChildren = collapsePythonRootFragments(bodyChildren, arena, lang)
		if len(bodyChildren) == 0 {
			continue
		}
		if arena != nil {
			buf := arena.allocNodeSlice(len(bodyChildren))
			copy(buf, bodyChildren)
			bodyChildren = buf
		}
		// See the matching comment in collapsePythonClassFragments: blockNode
		// and fn already carry the correct populateParentNode-derived HasError
		// from their real children; forcing false here would drop a genuine
		// nested error on the floor instead of letting it propagate to root.
		blockNode := newParentNodeInArena(arena, blockSym, true, bodyChildren, nil, 0)
		if repairedBlock, changed := repairPythonBlock(blockNode, arena, lang, false); changed {
			blockNode = repairedBlock
		}
		fnChildren := []*Node{defNode, nameNode, paramsNode, colonNode, blockNode}
		if arena != nil {
			buf := arena.allocNodeSlice(len(fnChildren))
			copy(buf, fnChildren)
			fnChildren = buf
		}
		fn := newParentNodeInArena(arena, functionDefSym, true, fnChildren, pythonSyntheticFunctionFieldIDs(arena, len(fnChildren), lang), 0)

		out := make([]*Node, 0, len(nodes)-(bodyEnd-i)+1)
		out = append(out, nodes[:i]...)
		out = append(out, fn)
		out = append(out, nodes[bodyEnd:]...)
		if arena != nil {
			buf := arena.allocNodeSlice(len(out))
			copy(buf, out)
			out = buf
		}
		return out, true
	}
	return nodes, false
}

func collapsePythonTerminalIfSuffix(nodes []*Node, arena *nodeArena, lang *Language) ([]*Node, bool) {
	if len(nodes) < 6 {
		return nodes, false
	}
	ifSym, ok := symbolByName(lang, "if_statement")
	if !ok {
		return nodes, false
	}
	blockSym, ok := symbolByName(lang, "block")
	if !ok {
		return nodes, false
	}
	n := len(nodes)
	ifNode := nodes[n-6]
	condNode := nodes[n-5]
	colonNode := nodes[n-4]
	indentNode := nodes[n-3]
	bodyNode := nodes[n-2]
	dedentNode := nodes[n-1]
	if ifNode == nil || condNode == nil || colonNode == nil || indentNode == nil || bodyNode == nil || dedentNode == nil {
		return nodes, false
	}
	if ifNode.Type(lang) != "if" || colonNode.Type(lang) != ":" || indentNode.Type(lang) != "_indent" || bodyNode.Type(lang) != "_simple_statements" || dedentNode.Type(lang) != "_dedent" {
		return nodes, false
	}
	if !condNode.IsNamed() {
		return nodes, false
	}

	blockChildren := []*Node{indentNode, bodyNode, dedentNode}
	if arena != nil {
		buf := arena.allocNodeSlice(len(blockChildren))
		copy(buf, blockChildren)
		blockChildren = buf
	}
	// blockNode/ifStmt again already carry the correct
	// populateParentNode-derived HasError from their real children (see
	// collapsePythonClassFragments); no override needed or wanted here.
	blockNode := newParentNodeInArena(arena, blockSym, true, blockChildren, nil, 0)

	ifChildren := []*Node{ifNode, condNode, colonNode, blockNode}
	if arena != nil {
		buf := arena.allocNodeSlice(len(ifChildren))
		copy(buf, ifChildren)
		ifChildren = buf
	}
	ifFieldIDs := pythonSyntheticIfFieldIDs(arena, len(ifChildren), lang)
	ifStmt := newParentNodeInArena(arena, ifSym, true, ifChildren, ifFieldIDs, 0)

	out := make([]*Node, 0, n-5)
	out = append(out, nodes[:n-6]...)
	out = append(out, ifStmt)
	if arena != nil {
		buf := arena.allocNodeSlice(len(out))
		copy(buf, out)
		return buf, true
	}
	return out, true
}

func flattenPythonModuleRepeat(node *Node, out []*Node, lang *Language) []*Node {
	if node == nil {
		return out
	}
	if node.Type(lang) == "module_repeat1" {
		childCount := resultChildCount(node)
		for i := 0; i < childCount; i++ {
			child := resultChildAt(node, i)
			out = flattenPythonModuleRepeat(child, out, lang)
		}
		return out
	}
	if node.IsNamed() {
		out = append(out, node)
	}
	return out
}

func pythonCollectIndentedSuite(nodes []*Node, start int, baseColumn uint32) ([]*Node, int, bool) {
	if start >= len(nodes) {
		return nil, start, false
	}
	end := start
	for end < len(nodes) {
		cur := nodes[end]
		if cur == nil {
			end++
			continue
		}
		if cur.startPoint.Column <= baseColumn {
			break
		}
		end++
	}
	if end == start {
		return nil, start, false
	}
	return nodes[start:end], end, true
}
