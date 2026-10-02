package gotreesitter

import (
	"bytes"
	"strings"
)

func normalizeYAMLRecoveredRoot(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "yaml" || len(root.children) == 0 {
		return
	}
	if root.Type(lang) != "stream" && root.Type(lang) != "ERROR" {
		return
	}
	if root.IsError() || root.HasError() {
		// Keep native recovery boundaries unless the flat mapping prefix has
		// enough information to rebuild the locked grammar's recovered shape.
		yamlRecoverFlatMappingError(root, source, lang)
		return
	}
	if yamlRootLooksCanonical(root, lang) {
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

					retagResultRoot(root, streamSym, symbolIsNamed(lang, streamSym))
					replaceNodeChildrenUnfielded(root, cloneNodeSliceInArena(root.ownerArena, streamChildren))
				}
			}
		}
	}
	// Recovery declined (doc stayed nil above, or the root was never
	// canonical to begin with) and the root reduces to nothing but a single
	// ERROR child: there is no content left for the passes below to
	// normalize. Promote that ERROR node to be the root itself instead of
	// leaving it wrapped in a "stream" -- the grammar's start rule, which
	// implies at least a well-formed (possibly empty) document -- to match
	// the C reference. C returns a bare (ERROR ...) root, never
	// (stream (ERROR ...)), when nothing in the input reduces to valid
	// document content at all (for example a lone unterminated "[").
	if root.Type(lang) != "ERROR" && len(root.children) == 1 && root.children[0] != nil && root.children[0].Type(lang) == "ERROR" {
		errChild := root.children[0]
		retagResultRoot(root, errChild.symbol, symbolIsNamed(lang, errChild.symbol))
		replaceNodeChildrenUnfielded(root, errChild.children)
		root.setHasError(true)
		root.startByte = 0
		root.startPoint = Point{}
		root.endByte = uint32(len(source))
		root.endPoint = pointAtOffsetYAML(source, len(source))
		return
	}
	yamlNormalizeRecoveredSubtrees(root, source, lang)
	yamlWrapDocumentBlockCollections(root, lang)
	yamlUnwrapCommentLedSequenceDocuments(root, lang)
	root.startByte = 0
	root.startPoint = Point{}
	root.endByte = uint32(len(source))
	root.endPoint = pointAtOffsetYAML(source, len(source))
}

// yamlRecoverFlatMappingError retains the reduced prefix and merges empty
// recovery markers into an ERROR span. Other erroneous trees stay untouched.
func yamlRecoverFlatMappingError(root *Node, source []byte, lang *Language) {
	if root.Type(lang) != "stream" || len(root.children) != 1 || !root.children[0].IsError() {
		return
	}
	nodes := root.children[0].children
	if len(nodes) == 0 || len(nodes) == 1 && nodes[0] != nil && !nodes[0].IsNamed() {
		// An irreducible token has no document content to frame.
		retagResultRoot(root, errorSymbol, true)
		replaceNodeChildrenUnfielded(root, nodes)
		if len(nodes) == 0 {
			root.startByte = 0
			root.startPoint = Point{}
		}
		root.endByte = uint32(len(source))
		root.endPoint = pointAtOffsetYAML(source, len(source))
		return
	}
	if len(nodes) < 4 || nodes[0] == nil || nodes[1] == nil || nodes[1].Type(lang) != ":" {
		return
	}
	keyType := nodes[0].Type(lang)
	if keyType != "flow_node" && keyType != "plain_scalar" {
		return
	}
	tail := nodes[len(nodes)-1]
	if tail == nil || !tail.IsError() || len(tail.children) != 0 {
		return
	}
	shape := 0
	switch {
	case len(nodes) == 5 && nodes[2].Type(lang) == "flow_node" && nodes[3].Type(lang) == "plain_scalar":
		// A lexical failure can invalidate the earlier mapping prefix too.
		// Leave its recovery boundary intact rather than moving the error.
		if bytes.ContainsAny(source[nodes[3].startByte:], "\\\"'") {
			return
		}
		shape = 1
	case len(nodes) == 4 && nodes[2].Type(lang) == "flow_node" && tail.startByte == nodes[2].endByte && nodes[2].endPoint.Row > nodes[0].startPoint.Row:
		shape = 2
	case len(nodes) == 7 && nodes[2].Type(lang) == "[" && nodes[3].Type(lang) == "flow_node" && nodes[4].Type(lang) == "," && nodes[5].Type(lang) == "flow_node":
		shape = 3
	default:
		return
	}
	var keyField, valueField FieldID
	for id, name := range lang.FieldNames {
		if name == "key" {
			keyField = FieldID(id)
		} else if name == "value" {
			valueField = FieldID(id)
		}
	}
	if keyField == 0 || valueField == 0 {
		return
	}
	names := [...]string{"flow_node", "plain_scalar", "integer_scalar", "block_mapping_pair", "block_mapping", "block_node", "document"}
	var symbols [len(names)]Symbol
	for i, name := range names {
		var ok bool
		if symbols[i], ok = symbolByName(lang, name); !ok {
			return
		}
	}
	end := len(source)
	for end > 0 && (source[end-1] == '\n' || source[end-1] == '\r' || source[end-1] == ' ' || source[end-1] == '\t') {
		end--
	}
	parent := func(sym Symbol, children []*Node, fields []FieldID, endByte uint32) *Node {
		node := newParentNodeInArena(root.ownerArena, sym, true, cloneNodeSliceInArena(root.ownerArena, children), fields, 0)
		node.endByte = endByte
		node.endPoint = pointAtOffsetYAML(source, int(endByte))
		return node
	}
	document := func(items []*Node) *Node {
		mapping := parent(symbols[4], items, nil, root.endByte)
		block := parent(symbols[5], []*Node{mapping}, nil, root.endByte)
		return parent(symbols[6], []*Node{block}, nil, root.endByte)
	}
	key := nodes[0]
	if keyType == "plain_scalar" {
		key = parent(symbols[0], []*Node{key}, nil, key.endByte)
	}
	var children []*Node
	if shape == 2 {
		// C recovers the unexpected colon as a value-only mapping pair.
		at := int(tail.startByte)
		if at >= end || source[at] != ':' {
			return
		}
		start := at + 1
		for start < end && (source[start] == ' ' || source[start] == '\t') {
			start++
		}
		if start == end {
			return
		}
		for _, c := range source[start:end] {
			if c < '0' || c > '9' {
				return
			}
		}
		colon := newLeafNodeInArena(root.ownerArena, nodes[1].symbol, false, uint32(at), uint32(at+1), pointAtOffsetYAML(source, at), pointAtOffsetYAML(source, at+1))
		integer := newLeafNodeInArena(root.ownerArena, symbols[2], true, uint32(start), uint32(end), pointAtOffsetYAML(source, start), pointAtOffsetYAML(source, end))
		plain := parent(symbols[1], []*Node{integer}, nil, uint32(end))
		value := parent(symbols[0], []*Node{plain}, nil, uint32(end))
		pair := parent(symbols[3], []*Node{key, nodes[1], nodes[2]}, []FieldID{keyField, 0, valueField}, nodes[2].endByte)
		errNode := parent(errorSymbol, []*Node{pair}, nil, pair.endByte)
		errNode.setExtra(true)
		valuePair := parent(symbols[3], []*Node{colon, value}, []FieldID{0, valueField}, uint32(end))
		children = []*Node{errNode, document([]*Node{valuePair})}
	} else {
		pairChildren := []*Node{key, nodes[1]}
		fields := []FieldID{keyField, 0}
		errChildren := nodes[2 : len(nodes)-1]
		if shape == 1 {
			pairChildren = append(pairChildren, nodes[2])
			fields = append(fields, valueField)
			flow := parent(symbols[0], []*Node{nodes[3]}, nil, nodes[3].endByte)
			errChildren = []*Node{flow}
		}
		pair := parent(symbols[3], pairChildren, fields, pairChildren[len(pairChildren)-1].endByte)
		errNode := parent(errorSymbol, errChildren, nil, uint32(end))
		errNode.setExtra(true)
		children = []*Node{document([]*Node{pair, errNode})}
	}
	replaceNodeChildrenUnfielded(root, cloneNodeSliceInArena(root.ownerArena, children))
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
	doc := newParentNodeInArena(arena, documentSym, symbolIsNamed(lang, documentSym), children, nil, 0)
	doc.endByte = endByte
	doc.endPoint = endPoint
	doc.setHasError(false)
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
		if !yamlRecoveredCollectionComplete(bodyNodes, "block_mapping_pair", lang) {
			return nil
		}
		core = yamlWrapYAMLCollection("block_mapping", bodyNodes, endByte, endPoint, arena, lang)
	case "block_sequence_item":
		if !yamlRecoveredCollectionComplete(bodyNodes, "block_sequence_item", lang) {
			return nil
		}
		core = yamlWrapYAMLCollection("block_sequence", bodyNodes, endByte, endPoint, arena, lang)
	case ">", "|":
		blockScalarSym, ok := symbolByName(lang, "block_scalar")
		if !ok {
			return nil
		}
		core = newParentNodeInArena(arena, blockScalarSym, symbolIsNamed(lang, blockScalarSym), bodyNodes, nil, 0)
		core.endByte = endByte
		core.endPoint = endPoint
		core.setHasError(false)
	case "block_scalar":
		if len(bodyNodes) != 1 {
			return nil
		}
		core = first
	case "[", "{":
		// A bare, unmatched flow-collection opener is never complete YAML on
		// its own: every valid flow sequence or mapping needs a matching
		// close. Falling through to the default branch below would wrap this
		// lone token as recovered document content and clear HasError,
		// silently losing the error for genuinely unterminated input (for
		// example a lone "[" at EOF) -- see
		// TestYAMLUnclosedFlowSequenceRootHasErrorBaseBehavior, which pins
		// the C reference's (ERROR "[") shape for this exact input. Decline
		// recovery here so the caller keeps the original ERROR root instead.
		return nil
	default:
		// A recovered document needs a named core node (a flow_node, a
		// block collection, a scalar). An ERROR node or a bare anonymous
		// token such as "[" is not one: rebuilding a document around it
		// dropped the sibling content and published a clean (stream
		// (document)) for input C reports as an ERROR root (for example
		// "[a", or "[" after an incremental suffix insert). Leave the
		// recovered ERROR shape alone in that case. Even a named first
		// node is insufficient when siblings remain: choosing only a
		// mapping key would discard its colon and incomplete value.
		if first.IsError() || !first.IsNamed() || len(bodyNodes) != 1 {
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
		core.setHasError(false)
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

// A reconstructed collection must retain every recovered token. A partial
// mapping or sequence can leave keys, punctuation, or ERROR nodes alongside
// complete items; discarding those would turn malformed input into clean YAML.
func yamlRecoveredCollectionComplete(nodes []*Node, itemType string, lang *Language) bool {
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if typ := node.Type(lang); typ != itemType && typ != "comment" {
			return false
		}
	}
	return true
}

func yamlWrapYAMLNode(name string, children []*Node, endByte uint32, endPoint Point, arena *nodeArena, lang *Language) *Node {
	sym, ok := symbolByName(lang, name)
	if !ok {
		return nil
	}
	node := newParentNodeInArena(arena, sym, symbolIsNamed(lang, sym), children, nil, 0)
	node.endByte = endByte
	node.endPoint = endPoint
	node.setHasError(false)
	return node
}

func yamlWrapYAMLCollection(name string, children []*Node, endByte uint32, endPoint Point, arena *nodeArena, lang *Language) *Node {
	return yamlWrapYAMLNode(name, children, endByte, endPoint, arena, lang)
}

func yamlWrapYAMLBlockNode(children []*Node, endByte uint32, endPoint Point, arena *nodeArena, lang *Language) *Node {
	return yamlWrapYAMLNode("block_node", children, endByte, endPoint, arena, lang)
}

func yamlWrapDocumentBlockCollections(root *Node, lang *Language) {
	if root == nil || lang == nil || root.Type(lang) != "stream" {
		return
	}
	for _, doc := range root.children {
		if doc == nil || doc.Type(lang) != "document" || len(doc.children) != 1 {
			continue
		}
		child := doc.children[0]
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "block_mapping":
		default:
			continue
		}
		blockNode := yamlWrapYAMLBlockNode([]*Node{child}, doc.endByte, doc.endPoint, doc.ownerArena, lang)
		if blockNode == nil {
			continue
		}
		blockNode.startByte = child.startByte
		blockNode.startPoint = child.startPoint
		blockNode.endByte = doc.endByte
		blockNode.endPoint = doc.endPoint
		replaceNodeChildrenUnfielded(doc, cloneNodeSliceInArena(doc.ownerArena, []*Node{blockNode}))
		doc.setHasError(false)
	}
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
	plain := newParentNodeInArena(node.ownerArena, plainScalarSym, symbolIsNamed(lang, plainScalarSym), []*Node{child}, nil, 0)
	replaceNodeChildrenUnfielded(node, cloneNodeSliceInArena(node.ownerArena, []*Node{plain}))
	node.setHasError(false)
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
	replaceNodeChildrenUnfielded(node, cloneNodeSliceInArena(node.ownerArena, filtered))
	node.setHasError(false)
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
		replaceNodeChildrenUnfielded(node, cloneNodeSliceInArena(node.ownerArena, []*Node{recovered}))
		node.setHasError(false)
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
	replaceNodeChildrenUnfielded(node, cloneNodeSliceInArena(node.ownerArena, children))
	node.setHasError(false)
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
		replaceNodeChildrenUnfielded(node, cloneNodeSliceInArena(node.ownerArena, child.children))
		node.setHasError(false)
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
	replaceNodeChildrenUnfielded(node, cloneNodeSliceInArena(node.ownerArena, []*Node{seq}))
	node.setHasError(false)
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
