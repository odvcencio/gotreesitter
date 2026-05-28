package gotreesitter

// normalizeCollapsedNamedLeafChildren restores collapsed single-anonymous-child
// nodes. When a named node (parentName) wraps a single anonymous token
// (childName) and the collapse logic strips the child, this function
// reconstructs the child so the tree matches C tree-sitter output.
func normalizeCollapsedNamedLeafChildren(root *Node, lang *Language, parentName, childName string) {
	normalizeCollapsedNamedLeafChildrenWithStats(root, lang, parentName, childName)
}

func normalizeCollapsedNamedLeafChildrenBySource(root *Node, source []byte, lang *Language, parentName string, childNames ...string) {
	normalizeCollapsedNamedLeafChildrenBySourceWithStats(root, source, lang, parentName, childNames...)
}

type collapsedNamedLeafRule struct {
	languageName string
	parentName   string
	childName    string
}

var resultCollapsedNamedLeafRules = []collapsedNamedLeafRule{
	{languageName: "c_sharp", parentName: "implicit_type", childName: "var"},
	{languageName: "cobol", parentName: "period", childName: "."},
	{languageName: "COBOL", parentName: "period", childName: "."},
}

func normalizeResultCollapsedNamedLeafChildren(root *Node, lang *Language) {
	if root == nil || lang == nil {
		return
	}
	for _, rule := range resultCollapsedNamedLeafRules {
		if rule.languageName != lang.Name {
			continue
		}
		normalizeCollapsedNamedLeafChildren(root, lang, rule.parentName, rule.childName)
	}
}

func normalizeCollapsedNamedLeafChildrenWithStats(root *Node, lang *Language, parentName, childName string) normalizationPassCounters {
	var counters normalizationPassCounters
	if root == nil || lang == nil {
		return counters
	}
	parentSym, ok := lang.symbolByNameAndNamed(parentName, true)
	if !ok {
		parentSym, ok = symbolByName(lang, parentName)
	}
	if !ok {
		return counters
	}
	childSym, childOk := lang.symbolByNameAndNamed(childName, false)
	if !childOk {
		childSym, childOk = symbolByName(lang, childName)
		if !childOk {
			return counters
		}
	}
	childNamed := symbolIsNamed(lang, childSym)
	walkResultTree(root, func(n *Node) {
		counters.nodesVisited++
		childCount := resultChildCount(n)
		if n.symbol == parentSym && childCount == 0 {
			child := newLeafNodeInArena(n.ownerArena, childSym, childNamed, n.startByte, n.endByte, n.startPoint, n.endPoint)
			child.parent = n
			child.childIndex = 0
			n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{child})
			counters.nodesRewritten++
		}
	})
	return counters
}

func normalizeCollapsedNamedLeafChildrenBySourceWithStats(root *Node, source []byte, lang *Language, parentName string, childNames ...string) normalizationPassCounters {
	var counters normalizationPassCounters
	if root == nil || lang == nil || len(source) == 0 || len(childNames) == 0 {
		return counters
	}
	parentSym, ok := lang.symbolByNameAndNamed(parentName, true)
	if !ok {
		parentSym, ok = symbolByName(lang, parentName)
	}
	if !ok {
		return counters
	}
	childSyms := make(map[string]Symbol, len(childNames))
	childNamed := make(map[Symbol]bool, len(childNames))
	for _, childName := range childNames {
		childSym, ok := lang.symbolByNameAndNamed(childName, false)
		if !ok {
			childSym, ok = symbolByName(lang, childName)
			if !ok {
				continue
			}
		}
		childSyms[childName] = childSym
		childNamed[childSym] = symbolIsNamed(lang, childSym)
	}
	if len(childSyms) == 0 {
		return counters
	}
	walkResultTree(root, func(n *Node) {
		counters.nodesVisited++
		childCount := resultChildCount(n)
		if n.symbol != parentSym || childCount != 0 || int(n.startByte) > len(source) || int(n.endByte) > len(source) || n.startByte > n.endByte {
			return
		}
		childSym, ok := childSyms[string(source[n.startByte:n.endByte])]
		if !ok {
			return
		}
		child := newLeafNodeInArena(n.ownerArena, childSym, childNamed[childSym], n.startByte, n.endByte, n.startPoint, n.endPoint)
		child.parent = n
		child.childIndex = 0
		n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{child})
		counters.nodesRewritten++
	})
	return counters
}
