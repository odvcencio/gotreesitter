package gotreesitter

import "bytes"

// normalizeSwiftCompatibility recovers the leading control-keyword token that
// grammargen's reduce path drops from `control_transfer_statement` nodes.
// The C reference parser keeps "return"/"continue"/"break"/"yield" as the
// first child of the statement; grammargen reduces through the hidden
// _optionally_valueful_control_keyword in a way that loses the anonymous
// token, leaving either a childless node (bare `return`) or a node whose
// span starts at the result expression (`return 42`).
func normalizeSwiftCompatibility(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "swift" {
		return
	}
	// Bare keyword case (childCount=0, span covers exactly the keyword).
	normalizeCollapsedNamedLeafChildrenBySource(
		root, source, lang,
		"control_transfer_statement",
		"return", "continue", "break", "yield",
	)
	// `return <expr>` case: existing children present but the keyword leaf is
	// missing as the first child and the span starts at the result expression.
	prependSwiftControlTransferKeyword(root, source, lang)
}

func prependSwiftControlTransferKeyword(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || len(source) == 0 {
		return
	}
	ctsSym, ok := lang.symbolByNameAndNamed("control_transfer_statement", true)
	if !ok {
		ctsSym, ok = symbolByName(lang, "control_transfer_statement")
		if !ok {
			return
		}
	}
	keywordSyms := map[string]Symbol{}
	for _, kw := range []string{"return", "continue", "break", "yield"} {
		s, ok := lang.symbolByNameAndNamed(kw, false)
		if !ok {
			s, ok = symbolByName(lang, kw)
			if !ok {
				continue
			}
		}
		keywordSyms[kw] = s
	}
	if len(keywordSyms) == 0 {
		return
	}
	// Walk top-down with an explicit ancestor stack so we can extend ancestor
	// spans (parent links aren't wired yet at this point in result building).
	var path []*Node
	var walk func(n *Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		path = append(path, n)
		defer func() { path = path[:len(path)-1] }()
		if n.symbol == ctsSym && len(n.children) > 0 {
			first := n.children[0]
			isKeywordChild := false
			if first != nil {
				if _, ok := keywordSyms[first.Type(lang)]; ok {
					isKeywordChild = true
				}
			}
			if !isKeywordChild {
				if kw, kwEnd, ok := findSwiftLeadingControlKeyword(source, n.startByte, keywordSyms); ok {
					sym := keywordSyms[kw]
					keywordEnd := n.startByte - uint32(kwEnd-len(kw))
					keywordStart := keywordEnd - uint32(len(kw))
					leaf := newLeafNodeInArena(
						n.ownerArena, sym, false,
						keywordStart, keywordEnd,
						n.startPoint, n.startPoint,
					)
					leaf.parent = n
					leaf.childIndex = 0
					newChildren := make([]*Node, 0, len(n.children)+1)
					newChildren = append(newChildren, leaf)
					for i, c := range n.children {
						if c != nil {
							c.childIndex = int32(i + 1)
						}
						newChildren = append(newChildren, c)
					}
					n.children = cloneNodeSliceInArena(n.ownerArena, newChildren)
					// Extend n and its ancestors that share the old start.
					oldStart := n.startByte
					n.startByte = keywordStart
					for i := len(path) - 2; i >= 0; i-- {
						p := path[i]
						if p == nil || p.startByte != oldStart {
							break
						}
						p.startByte = keywordStart
					}
				}
			}
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	walk(root)
}

// findSwiftLeadingControlKeyword scans source backwards from rhsStart, skipping
// horizontal whitespace, to find one of the swift control keywords. Returns
// the matched keyword string and the offset (in bytes) from the rhsStart to
// where the keyword ends (i.e. how many bytes of whitespace were skipped + the
// keyword length).
func findSwiftLeadingControlKeyword(source []byte, rhsStart uint32, keywordSyms map[string]Symbol) (string, int, bool) {
	if int(rhsStart) > len(source) {
		return "", 0, false
	}
	pos := int(rhsStart)
	// Skip trailing whitespace right before rhsStart.
	for pos > 0 {
		c := source[pos-1]
		if c != ' ' && c != '\t' {
			break
		}
		pos--
	}
	for _, kw := range []string{"return", "continue", "break", "yield"} {
		if _, ok := keywordSyms[kw]; !ok {
			continue
		}
		if pos < len(kw) {
			continue
		}
		if !bytes.Equal(source[pos-len(kw):pos], []byte(kw)) {
			continue
		}
		// Ensure the byte before kw is a word boundary.
		if pos-len(kw) > 0 {
			prev := source[pos-len(kw)-1]
			if (prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z') || (prev >= '0' && prev <= '9') || prev == '_' {
				continue
			}
		}
		// Return how far rhsStart is from the END of the keyword.
		return kw, int(rhsStart) - (pos - len(kw)), true
	}
	return "", 0, false
}
