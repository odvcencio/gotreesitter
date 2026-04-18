package gotreesitter

func normalizeMakeConditionalConsequenceFields(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "make" {
		return
	}
	consequenceID, ok := lang.FieldByName("consequence")
	if !ok {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		switch n.Type(lang) {
		case "conditional", "elsif_directive", "else_directive":
			ensureNodeFieldStorage(n, len(n.children))
			start, end := -1, -1
			for i := 0; i < len(n.children); i++ {
				if n.fieldIDs[i] != consequenceID {
					continue
				}
				if start < 0 {
					start = i
				}
				end = i
			}
			if start >= 0 && end >= start {
				for start > 0 {
					prev := n.children[start-1]
					if prev == nil || prev.isNamed || prev.isExtra || prev.Type(lang) != "\t" {
						break
					}
					start--
				}
				for i := start; i <= end; i++ {
					if n.children[i] == nil {
						continue
					}
					n.fieldIDs[i] = consequenceID
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

func normalizeIniSectionStarts(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "ini" {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == "section" {
			for _, child := range n.children {
				if child == nil {
					continue
				}
				if n.startByte < child.startByte {
					n.startByte = child.startByte
					n.startPoint = child.startPoint
				}
				break
			}
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}

func normalizeZigEmptyInitListFields(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "zig" {
		return
	}
	fieldConstantID, ok := lang.FieldByName("field_constant")
	if !ok {
		return
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if len(n.fieldIDs) == len(n.children) {
			for i, child := range n.children {
				if child == nil || n.fieldIDs[i] != fieldConstantID || child.Type(lang) != "InitList" {
					continue
				}
				if n.Type(lang) != "SuffixExpr" || len(n.children) != 2 || i != 1 || n.children[0] == nil || n.children[0].Type(lang) != "." {
					continue
				}
				n.fieldIDs[i] = 0
				if len(n.fieldSources) == len(n.children) {
					n.fieldSources[i] = fieldSourceNone
				}
			}
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}
