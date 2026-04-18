package gotreesitter

import (
	"bytes"
	"strings"
)

func normalizePythonInterpolationPatterns(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "python" {
		return
	}
	patternListSym, ok := symbolByName(lang, "pattern_list")
	if !ok {
		return
	}
	listSplatPatternSym, hasListSplatPattern := symbolByName(lang, "list_splat_pattern")
	expressionListSym, hasExpressionList := symbolByName(lang, "expression_list")
	listSplatSym, hasListSplat := symbolByName(lang, "list_splat")

	patternListNamed := false
	if int(patternListSym) < len(lang.SymbolMetadata) {
		patternListNamed = lang.SymbolMetadata[patternListSym].Named
	}
	listSplatPatternNamed := false
	if hasListSplatPattern && int(listSplatPatternSym) < len(lang.SymbolMetadata) {
		listSplatPatternNamed = lang.SymbolMetadata[listSplatPatternSym].Named
	}

	var rewrite func(*Node, bool)
	rewrite = func(n *Node, inInterpolation bool) {
		if n == nil {
			return
		}
		here := inInterpolation || n.Type(lang) == "interpolation"
		if here {
			if hasExpressionList && n.symbol == expressionListSym {
				n.symbol = patternListSym
				n.isNamed = patternListNamed
			}
			if hasListSplatPattern && hasListSplat && n.symbol == listSplatSym {
				n.symbol = listSplatPatternSym
				n.isNamed = listSplatPatternNamed
			}
		}
		for _, child := range n.children {
			rewrite(child, here)
		}
	}
	rewrite(root, false)
}

func bytesAreTrivia(b []byte) bool {
	for _, c := range b {
		switch c {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return false
		}
	}
	return true
}

func lastNonTriviaByteEnd(source []byte) uint32 {
	for i := len(source); i > 0; i-- {
		switch source[i-1] {
		case ' ', '\t', '\n', '\r', '\f':
			continue
		default:
			return uint32(i)
		}
	}
	return 0
}

func normalizeHCLConfigFileRoot(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "hcl" || root.Type(lang) != "config_file" || len(root.children) == 0 {
		return
	}
	filtered := make([]*Node, 0, len(root.children))
	filteredChanged := false
	for _, child := range root.children {
		if child == nil {
			continue
		}
		if child.Type(lang) == "_whitespace" {
			filteredChanged = true
			continue
		}
		filtered = append(filtered, child)
	}
	if filteredChanged {
		if root.ownerArena != nil {
			buf := root.ownerArena.allocNodeSlice(len(filtered))
			copy(buf, filtered)
			filtered = buf
		}
		root.children = filtered
		root.fieldIDs = nil
		root.fieldSources = nil
	}
	for _, child := range root.children {
		if child == nil || child.Type(lang) != "body" {
			continue
		}
		snapHCLBodyBounds(child)
	}
}

func snapHCLBodyBounds(body *Node) {
	if body == nil || len(body.children) == 0 {
		return
	}
	first, last := firstAndLastNonNilChild(body.children)
	if first == nil || last == nil {
		return
	}
	body.startByte = first.startByte
	body.startPoint = first.startPoint
	body.endByte = last.endByte
	body.endPoint = last.endPoint
}

func firstAndLastNonNilChild(children []*Node) (*Node, *Node) {
	var first *Node
	for _, child := range children {
		if child != nil {
			first = child
			break
		}
	}
	if first == nil {
		return nil, nil
	}
	for i := len(children) - 1; i >= 0; i-- {
		if children[i] != nil {
			return first, children[i]
		}
	}
	return first, first
}

func normalizeYAMLRecoveredRoot(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "yaml" || len(root.children) == 0 {
		return
	}
	if root.Type(lang) != "stream" && root.Type(lang) != "ERROR" {
		return
	}
	if !yamlRootLooksCanonical(root, lang) {
		flat := yamlFlattenRecoveredRootChildren(root.children, lang)
		if len(flat) != 0 {
			leadingComments := 0
			for leadingComments < len(flat) && flat[leadingComments] != nil && flat[leadingComments].Type(lang) == "comment" {
				leadingComments++
			}
			doc := yamlBuildRecoveredSingleDocument(flat[leadingComments:], root.endByte, root.endPoint, root.ownerArena, lang)
			if doc != nil {
				streamSym, ok := symbolByName(lang, "stream")
				if ok {
					streamChildren := make([]*Node, 0, leadingComments+1)
					streamChildren = append(streamChildren, flat[:leadingComments]...)
					streamChildren = append(streamChildren, doc)

					root.symbol = streamSym
					root.isNamed = lang.SymbolMetadata[streamSym].Named
					root.children = cloneNodeSliceInArena(root.ownerArena, streamChildren)
					root.fieldIDs = nil
					root.fieldSources = nil
					root.hasError = false
					populateParentNode(root, root.children)
				}
			}
		}
	}
	yamlNormalizeRecoveredSubtrees(root, source, lang)
	yamlExtendExplicitDocumentRangesToLeadingComments(root, lang)
	yamlUnwrapCommentLedSequenceDocuments(root, lang)
	root.startByte = 0
	root.startPoint = Point{}
	root.endByte = uint32(len(source))
	root.endPoint = pointAtOffsetYAML(source, len(source))
}

func yamlRootLooksCanonical(root *Node, lang *Language) bool {
	if root == nil || lang == nil || root.Type(lang) != "stream" {
		return false
	}
	for _, child := range root.children {
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "comment", "document":
			continue
		default:
			return false
		}
	}
	return true
}

func yamlFlattenRecoveredRootChildren(children []*Node, lang *Language) []*Node {
	return yamlFlattenRecoverableNodes(children, lang)
}

func yamlBuildRecoveredSingleDocument(nodes []*Node, endByte uint32, endPoint Point, arena *nodeArena, lang *Language) *Node {
	if len(nodes) == 0 || lang == nil {
		return nil
	}
	documentSym, ok := symbolByName(lang, "document")
	if !ok {
		return nil
	}

	i := 0
	prefix := make([]*Node, 0, len(nodes))
	for i < len(nodes) {
		switch nodes[i].Type(lang) {
		case "tag_directive", "yaml_directive", "reserved_directive", "---", "...":
			prefix = append(prefix, nodes[i])
			i++
		default:
			goto prefixDone
		}
	}
prefixDone:

	body := yamlBuildRecoveredDocumentBody(nodes[i:], endByte, endPoint, arena, lang)
	if body == nil {
		return nil
	}

	children := make([]*Node, 0, len(prefix)+1)
	children = append(children, prefix...)
	children = append(children, body)
	doc := newParentNodeInArena(arena, documentSym, lang.SymbolMetadata[documentSym].Named, children, nil, 0)
	doc.endByte = endByte
	doc.endPoint = endPoint
	doc.hasError = false
	return doc
}

func yamlBuildRecoveredDocumentBody(nodes []*Node, endByte uint32, endPoint Point, arena *nodeArena, lang *Language) *Node {
	if len(nodes) == 0 || lang == nil {
		return nil
	}
	for _, node := range nodes {
		yamlWrapPlainScalarFlowNodes(node, lang)
	}

	decoratorsEnd := 0
	for decoratorsEnd < len(nodes) {
		switch nodes[decoratorsEnd].Type(lang) {
		case "tag", "anchor", "alias":
			decoratorsEnd++
		default:
			goto decoratorsDone
		}
	}
decoratorsDone:

	bodyNodes := nodes[decoratorsEnd:]
	if len(bodyNodes) == 0 {
		return nil
	}

	var core *Node
	first := yamlFirstNonComment(bodyNodes, lang)
	if first == nil {
		return nil
	}
	switch first.Type(lang) {
	case "block_mapping_pair":
		core = yamlWrapYAMLCollection("block_mapping", bodyNodes, endByte, endPoint, arena, lang)
	case "block_sequence_item":
		core = yamlWrapYAMLCollection("block_sequence", bodyNodes, endByte, endPoint, arena, lang)
	case ">", "|":
		blockScalarSym, ok := symbolByName(lang, "block_scalar")
		if !ok {
			return nil
		}
		core = newParentNodeInArena(arena, blockScalarSym, lang.SymbolMetadata[blockScalarSym].Named, bodyNodes, nil, 0)
		core.endByte = endByte
		core.endPoint = endPoint
		core.hasError = false
	case "block_scalar":
		core = first
	default:
		if first.Type(lang) == "ERROR" {
			return nil
		}
		core = first
	}

	if decoratorsEnd == 0 {
		if core.Type(lang) == "block_mapping" || core.Type(lang) == "block_sequence" || core.Type(lang) == "flow_node" {
			return yamlWrapYAMLBlockNode([]*Node{core}, endByte, endPoint, arena, lang)
		}
		core.endByte = endByte
		core.endPoint = endPoint
		core.hasError = false
		return core
	}

	blockChildren := make([]*Node, 0, decoratorsEnd+1)
	blockChildren = append(blockChildren, nodes[:decoratorsEnd]...)
	if core.Type(lang) == "block_node" {
		blockChildren = append(blockChildren, core.children...)
	} else {
		blockChildren = append(blockChildren, core)
	}
	return yamlWrapYAMLBlockNode(blockChildren, endByte, endPoint, arena, lang)
}

func yamlWrapYAMLNode(name string, children []*Node, endByte uint32, endPoint Point, arena *nodeArena, lang *Language) *Node {
	sym, ok := symbolByName(lang, name)
	if !ok {
		return nil
	}
	node := newParentNodeInArena(arena, sym, lang.SymbolMetadata[sym].Named, children, nil, 0)
	node.endByte = endByte
	node.endPoint = endPoint
	node.hasError = false
	return node
}

func yamlWrapYAMLCollection(name string, children []*Node, endByte uint32, endPoint Point, arena *nodeArena, lang *Language) *Node {
	return yamlWrapYAMLNode(name, children, endByte, endPoint, arena, lang)
}

func yamlWrapYAMLBlockNode(children []*Node, endByte uint32, endPoint Point, arena *nodeArena, lang *Language) *Node {
	return yamlWrapYAMLNode("block_node", children, endByte, endPoint, arena, lang)
}

func yamlWrapPlainScalarFlowNodes(node *Node, lang *Language) {
	if node == nil || lang == nil {
		return
	}
	for _, child := range node.children {
		yamlWrapPlainScalarFlowNodes(child, lang)
	}
	if node.Type(lang) != "flow_node" || len(node.children) != 1 {
		return
	}
	child := node.children[0]
	if child == nil || child.Type(lang) == "plain_scalar" {
		return
	}
	switch child.Type(lang) {
	case "string_scalar", "null_scalar", "boolean_scalar", "integer_scalar", "float_scalar", "timestamp_scalar":
	default:
		return
	}
	plainScalarSym, ok := symbolByName(lang, "plain_scalar")
	if !ok {
		return
	}
	plain := newParentNodeInArena(node.ownerArena, plainScalarSym, lang.SymbolMetadata[plainScalarSym].Named, []*Node{child}, nil, 0)
	node.children = cloneNodeSliceInArena(node.ownerArena, []*Node{plain})
	node.fieldIDs = nil
	node.fieldSources = nil
	node.hasError = false
	populateParentNode(node, node.children)
}

func yamlNormalizeRecoveredSubtrees(node *Node, source []byte, lang *Language) {
	if node == nil || lang == nil {
		return
	}
	for _, child := range node.children {
		yamlNormalizeRecoveredSubtrees(child, source, lang)
	}
	switch node.Type(lang) {
	case "block_mapping":
		yamlNormalizeYAMLCollectionNode(node, "block_mapping_pair", lang)
	case "block_sequence":
		yamlNormalizeYAMLCollectionNode(node, "block_sequence_item", lang)
	case "block_node":
		yamlNormalizeYAMLBlockNode(node, lang)
	case "flow_node":
		yamlNormalizeYAMLFlowNode(node, source, lang)
	case "flow_mapping", "flow_sequence", "double_quote_scalar", "single_quote_scalar":
		yamlCollapseNestedYAMLWrapper(node, lang)
	}
}

func yamlNormalizeYAMLCollectionNode(node *Node, itemType string, lang *Language) {
	if node == nil || lang == nil || !yamlChildrenNeedRecovery(node.children, lang) {
		return
	}
	flat := yamlFlattenRecoverableNodes(node.children, lang)
	if len(flat) == 0 {
		return
	}
	filtered := make([]*Node, 0, len(flat))
	for _, child := range flat {
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case itemType, "comment":
			filtered = append(filtered, child)
		}
	}
	if len(filtered) == 0 {
		return
	}
	node.children = cloneNodeSliceInArena(node.ownerArena, filtered)
	node.fieldIDs = nil
	node.fieldSources = nil
	node.hasError = false
	populateParentNode(node, node.children)
}

func yamlNormalizeYAMLBlockNode(node *Node, lang *Language) {
	if node == nil || lang == nil {
		return
	}
	flat := yamlFlattenRecoverableNodes(node.children, lang)
	if len(flat) == 0 {
		return
	}
	needsRecovery := yamlChildrenNeedRecovery(node.children, lang)
	if !needsRecovery {
		for _, child := range flat {
			if child == nil {
				continue
			}
			switch child.Type(lang) {
			case "block_mapping_pair", "block_sequence_item", ">", "|":
				needsRecovery = true
			}
			if needsRecovery {
				break
			}
		}
	}
	if !needsRecovery {
		return
	}
	recovered := yamlBuildRecoveredDocumentBody(flat, node.endByte, node.endPoint, node.ownerArena, lang)
	if recovered == nil {
		return
	}
	if recovered.Type(lang) == "block_node" {
		*node = *recovered
		return
	}
	switch recovered.Type(lang) {
	case "block_mapping", "block_sequence", "block_scalar":
		node.children = cloneNodeSliceInArena(node.ownerArena, []*Node{recovered})
		node.fieldIDs = nil
		node.fieldSources = nil
		node.hasError = false
		populateParentNode(node, node.children)
	}
}

func yamlNormalizeYAMLFlowNode(node *Node, source []byte, lang *Language) {
	if node == nil || lang == nil {
		return
	}
	yamlWrapPlainScalarFlowNodes(node, lang)
	flat := yamlFlattenRecoverableNodes(node.children, lang)
	if len(flat) == 0 {
		return
	}
	trimmed := yamlTrimNodeSource(node, source)
	needsRecovery := yamlChildrenNeedRecovery(node.children, lang)
	if !needsRecovery {
		if len(trimmed) >= 2 {
			switch trimmed[0] {
			case '"', '\'', '[', '{':
				needsRecovery = true
			}
		}
	}
	if !needsRecovery {
		for _, child := range flat {
			if child != nil && child.Type(lang) == "flow_pair" {
				needsRecovery = true
				break
			}
		}
	}
	if !needsRecovery {
		return
	}
	decoratorsEnd := 0
	for decoratorsEnd < len(flat) {
		switch flat[decoratorsEnd].Type(lang) {
		case "tag", "anchor", "alias":
			decoratorsEnd++
		default:
			goto decoratorsDone
		}
	}
decoratorsDone:
	bodyNodes := flat[decoratorsEnd:]
	if len(bodyNodes) == 0 {
		return
	}
	var core *Node
	switch {
	case len(trimmed) >= 2 && trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"':
		if existing := yamlFirstNodeOfType(bodyNodes, "double_quote_scalar", lang); existing != nil {
			core = existing
		} else {
			core = yamlWrapYAMLNode("double_quote_scalar", bodyNodes, node.endByte, node.endPoint, node.ownerArena, lang)
		}
	case len(trimmed) >= 2 && trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'':
		if existing := yamlFirstNodeOfType(bodyNodes, "single_quote_scalar", lang); existing != nil {
			core = existing
		} else {
			core = yamlWrapYAMLNode("single_quote_scalar", bodyNodes, node.endByte, node.endPoint, node.ownerArena, lang)
		}
	case len(trimmed) >= 2 && trimmed[0] == '[' && trimmed[len(trimmed)-1] == ']':
		if existing := yamlFirstNodeOfType(bodyNodes, "flow_sequence", lang); existing != nil {
			core = existing
		} else {
			core = yamlWrapYAMLCollection("flow_sequence", bodyNodes, node.endByte, node.endPoint, node.ownerArena, lang)
		}
	case len(trimmed) >= 2 && trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}':
		if existing := yamlFirstNodeOfType(bodyNodes, "flow_mapping", lang); existing != nil {
			core = existing
		} else {
			core = yamlWrapYAMLCollection("flow_mapping", bodyNodes, node.endByte, node.endPoint, node.ownerArena, lang)
		}
	case yamlSliceContainsType(bodyNodes, "flow_pair", lang):
		core = yamlWrapYAMLCollection("flow_mapping", bodyNodes, node.endByte, node.endPoint, node.ownerArena, lang)
	default:
		first := yamlFirstNonComment(bodyNodes, lang)
		if first == nil {
			return
		}
		switch first.Type(lang) {
		case "flow_mapping", "flow_sequence":
			core = first
		default:
			return
		}
	}
	children := make([]*Node, 0, decoratorsEnd+1)
	children = append(children, flat[:decoratorsEnd]...)
	children = append(children, core)
	node.children = cloneNodeSliceInArena(node.ownerArena, children)
	node.fieldIDs = nil
	node.fieldSources = nil
	node.hasError = false
	populateParentNode(node, node.children)
}

func yamlChildrenNeedRecovery(children []*Node, lang *Language) bool {
	for _, child := range children {
		if child == nil {
			continue
		}
		if child.IsError() || strings.HasPrefix(child.Type(lang), "_") {
			return true
		}
	}
	return false
}

func yamlFlattenRecoverableNodes(children []*Node, lang *Language) []*Node {
	flat := make([]*Node, 0, len(children))
	var appendNode func(*Node)
	appendNode = func(node *Node) {
		if node == nil {
			return
		}
		typ := node.Type(lang)
		switch {
		case typ == "_bl":
			return
		case strings.HasPrefix(typ, "_r_blk_str_repeat"):
			return
		case node.IsError():
			for _, child := range node.children {
				appendNode(child)
			}
		case strings.HasPrefix(typ, "_"):
			for _, child := range node.children {
				appendNode(child)
			}
		default:
			flat = append(flat, node)
		}
	}
	for _, child := range children {
		appendNode(child)
	}
	return flat
}

func yamlExtendExplicitDocumentRangesToLeadingComments(root *Node, lang *Language) {
	if root == nil || lang == nil || root.Type(lang) != "stream" {
		return
	}
	var firstLeadingComment *Node
	for _, child := range root.children {
		if child == nil {
			continue
		}
		if child.Type(lang) == "comment" {
			if firstLeadingComment == nil {
				firstLeadingComment = child
			}
			continue
		}
		if child.Type(lang) == "document" && firstLeadingComment != nil {
			child.startByte = firstLeadingComment.startByte
			child.startPoint = firstLeadingComment.startPoint
		}
		firstLeadingComment = nil
	}
}

func yamlSliceContainsType(nodes []*Node, want string, lang *Language) bool {
	for _, node := range nodes {
		if node != nil && node.Type(lang) == want {
			return true
		}
	}
	return false
}

func yamlFirstNodeOfType(nodes []*Node, want string, lang *Language) *Node {
	for _, node := range nodes {
		if node != nil && node.Type(lang) == want {
			return node
		}
	}
	return nil
}

func yamlTrimNodeSource(node *Node, source []byte) []byte {
	if node == nil || len(source) == 0 || int(node.startByte) >= len(source) || node.endByte > uint32(len(source)) || node.startByte >= node.endByte {
		return nil
	}
	return bytes.TrimSpace(source[node.startByte:node.endByte])
}

func yamlCollapseNestedYAMLWrapper(node *Node, lang *Language) {
	if node == nil || lang == nil {
		return
	}
	nodeType := node.Type(lang)
	for node.NamedChildCount() == 1 {
		child := node.NamedChild(0)
		if child == nil || child.Type(lang) != nodeType {
			return
		}
		startByte, startPoint := node.startByte, node.startPoint
		endByte, endPoint := node.endByte, node.endPoint
		node.children = cloneNodeSliceInArena(node.ownerArena, child.children)
		node.fieldIDs = nil
		node.fieldSources = nil
		node.hasError = false
		populateParentNode(node, node.children)
		node.startByte = startByte
		node.startPoint = startPoint
		node.endByte = endByte
		node.endPoint = endPoint
	}
}

func yamlUnwrapCommentLedSequenceDocuments(root *Node, lang *Language) {
	if root == nil || lang == nil || root.Type(lang) != "stream" {
		return
	}
	seenLeadingComment := false
	for _, child := range root.children {
		if child == nil {
			continue
		}
		if child.Type(lang) == "comment" {
			seenLeadingComment = true
			continue
		}
		if child.Type(lang) == "document" && (seenLeadingComment || yamlDocumentSequenceBlockNodeShouldUnwrap(child, lang)) {
			yamlUnwrapDocumentSequenceBlockNode(child, lang)
		}
		seenLeadingComment = false
	}
}

func yamlUnwrapDocumentSequenceBlockNode(node *Node, lang *Language) {
	if node == nil || lang == nil || node.Type(lang) != "document" || node.NamedChildCount() != 1 {
		return
	}
	blockNode := node.NamedChild(0)
	if blockNode == nil || blockNode.Type(lang) != "block_node" || blockNode.NamedChildCount() != 1 {
		return
	}
	seq := blockNode.NamedChild(0)
	if seq == nil || seq.Type(lang) != "block_sequence" {
		return
	}
	startByte, startPoint := node.startByte, node.startPoint
	endByte, endPoint := node.endByte, node.endPoint
	node.children = cloneNodeSliceInArena(node.ownerArena, []*Node{seq})
	node.fieldIDs = nil
	node.fieldSources = nil
	node.hasError = false
	populateParentNode(node, node.children)
	node.startByte = startByte
	node.startPoint = startPoint
	node.endByte = endByte
	node.endPoint = endPoint
}

func yamlDocumentSequenceBlockNodeShouldUnwrap(node *Node, lang *Language) bool {
	if node == nil || lang == nil || node.Type(lang) != "document" || node.NamedChildCount() != 1 {
		return false
	}
	blockNode := node.NamedChild(0)
	if blockNode == nil || blockNode.Type(lang) != "block_node" || blockNode.NamedChildCount() != 1 {
		return false
	}
	seq := blockNode.NamedChild(0)
	if seq == nil || seq.Type(lang) != "block_sequence" {
		return false
	}
	itemCount := 0
	for i := 0; i < seq.NamedChildCount(); i++ {
		item := seq.NamedChild(i)
		if item == nil || item.Type(lang) != "block_sequence_item" || item.NamedChildCount() != 1 {
			return false
		}
		itemBody := item.NamedChild(0)
		if itemBody == nil || itemBody.Type(lang) != "block_node" || itemBody.NamedChildCount() != 1 {
			return false
		}
		if scalar := itemBody.NamedChild(0); scalar == nil || scalar.Type(lang) != "block_scalar" {
			return false
		}
		itemCount++
	}
	return itemCount > 0
}

func yamlFirstNonComment(nodes []*Node, lang *Language) *Node {
	for _, node := range nodes {
		if node == nil || node.Type(lang) == "comment" {
			continue
		}
		return node
	}
	return nil
}

func pointAtOffsetYAML(src []byte, offset int) Point {
	if offset < 0 {
		offset = 0
	}
	if offset > len(src) {
		offset = len(src)
	}
	var p Point
	for i := 0; i < offset; i++ {
		if src[i] == '\n' {
			p.Row++
			p.Column = 0
		} else {
			p.Column++
		}
	}
	return p
}
