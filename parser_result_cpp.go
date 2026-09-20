package gotreesitter

import "strings"

func normalizeCppMalformedClassFunctionDefinition(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "cpp" || root.Type(lang) != "translation_unit" || len(root.children) < 2 {
		return
	}
	changed := false
	out := make([]*Node, 0, len(root.children))
	for i := 0; i < len(root.children); i++ {
		if i+1 < len(root.children) {
			if merged, ok := rewriteCppMalformedClassFunctionDefinition(root.children[i], root.children[i+1], source, lang); ok {
				out = append(out, merged)
				i++
				changed = true
				continue
			}
		}
		out = append(out, root.children[i])
	}
	if changed {
		replaceNodeChildrenUnfielded(root, cloneNodeSliceIfArena(root.ownerArena, out))
	}
}

func rewriteCppMalformedClassFunctionDefinition(left, right *Node, source []byte, lang *Language) (*Node, bool) {
	if left == nil || right == nil || lang == nil || len(source) == 0 || right.ownerArena == nil {
		return nil, false
	}
	arena := right.ownerArena
	classSpec := cppMalformedClassSpecifierCarrier(left, lang, arena)
	if classSpec == nil || right.Type(lang) != "function_definition" || len(right.children) != 3 {
		return nil, false
	}

	returnType := right.children[0]
	declarator := right.children[1]
	body := right.children[2]
	if returnType == nil || declarator == nil || body == nil ||
		returnType.Text(source) != "void" ||
		declarator.Type(lang) != "function_declarator" ||
		body.Type(lang) != "compound_statement" ||
		len(declarator.children) != 2 {
		return nil, false
	}

	qualified := declarator.children[0]
	params := declarator.children[1]
	if qualified == nil || params == nil ||
		qualified.Type(lang) != "qualified_identifier" ||
		params.Type(lang) != "parameter_list" ||
		len(qualified.children) != 3 {
		return nil, false
	}

	namespace := qualified.children[0]
	separator := qualified.children[1]
	name := qualified.children[2]
	if namespace == nil || separator == nil || name == nil ||
		namespace.Type(lang) != "namespace_identifier" ||
		separator.Type(lang) != "::" ||
		name.Type(lang) != "identifier" {
		return nil, false
	}

	namespaceSym, ok := symbolByName(lang, "namespace_identifier")
	if !ok {
		return nil, false
	}
	identifierSym, ok := symbolByName(lang, "identifier")
	if !ok {
		return nil, false
	}

	voidNamespace := cppRetaggedClone(arena, returnType, namespaceSym, symbolIsNamed(lang, namespaceSym))
	identifier := cppRetaggedClone(arena, namespace, identifierSym, symbolIsNamed(lang, identifierSym))
	err := cppErrorWrapper(arena, identifier)

	// Field names below mirror the C reference runtime's field map for the
	// productions this rewrite reconstructs (qualified_identifier:
	// scope/name, function_declarator: declarator/parameters,
	// function_definition: type/declarator/body). cppCloneParentWithChildren
	// otherwise leaves rebuilt nodes field-less, which is the mechanism
	// behind the nine FieldName parity divergences this fix addresses.
	rewrittenQualified := cppCloneParentWithChildren(arena, qualified, []*Node{
		voidNamespace,
		err,
		cloneNodeInArena(arena, separator),
		cloneNodeInArena(arena, name),
	}, cppFieldIDsAt(arena, lang, 4, map[int]string{0: "scope", 3: "name"}))
	rewrittenDeclarator := cppCloneParentWithChildren(arena, declarator, []*Node{
		rewrittenQualified,
		cloneNodeInArena(arena, params),
	}, cppFieldIDsAt(arena, lang, 2, map[int]string{0: "declarator", 1: "parameters"}))
	return cppCloneParentWithChildren(arena, right, []*Node{
		cloneNodeInArena(arena, classSpec),
		rewrittenDeclarator,
		cloneNodeInArena(arena, body),
	}, cppFieldIDsAt(arena, lang, 3, map[int]string{0: "type", 1: "declarator", 2: "body"})), true
}

func cppMalformedClassSpecifierCarrier(n *Node, lang *Language, arena *nodeArena) *Node {
	if n == nil || lang == nil {
		return nil
	}
	switch n.Type(lang) {
	case "ERROR":
		if len(n.children) == 1 && n.children[0] != nil && n.children[0].Type(lang) == "class_specifier" {
			return n.children[0]
		}
		return buildCppMalformedClassSpecifierFromError(n, lang, arena)
	case "_declaration_specifiers":
		if n.isExtra() && len(n.children) == 1 && n.children[0] != nil && n.children[0].Type(lang) == "class_specifier" {
			return n.children[0]
		}
	}
	return nil
}

func buildCppMalformedClassSpecifierFromError(n *Node, lang *Language, arena *nodeArena) *Node {
	if n == nil || lang == nil || arena == nil || len(n.children) < 5 {
		return nil
	}

	classTok := n.children[0]
	nameCarrier := n.children[1]
	baseClause := n.children[2]
	openBrace := n.children[3]
	closeBrace := n.children[len(n.children)-1]
	if classTok == nil || nameCarrier == nil || baseClause == nil || openBrace == nil || closeBrace == nil ||
		classTok.Type(lang) != "class" ||
		baseClause.Type(lang) != "base_class_clause" ||
		openBrace.Type(lang) != "{" ||
		closeBrace.Type(lang) != "}" {
		return nil
	}

	name := nameCarrier
	if nameCarrier.Type(lang) == "_class_name" && len(nameCarrier.children) == 1 && nameCarrier.children[0] != nil {
		name = nameCarrier.children[0]
	}
	if name == nil || name.Type(lang) != "type_identifier" {
		return nil
	}

	classSpecSym, ok := symbolByName(lang, "class_specifier")
	if !ok {
		return nil
	}
	fieldDeclListSym, ok := symbolByName(lang, "field_declaration_list")
	if !ok {
		return nil
	}

	fieldChildren := make([]*Node, 0, len(n.children))
	fieldChildren = append(fieldChildren, cloneNodeInArena(arena, openBrace))
	for _, child := range n.children[4 : len(n.children)-1] {
		if child == nil {
			continue
		}
		if strings.Contains(child.Type(lang), "field_declaration_list") && strings.Contains(child.Type(lang), "repeat") {
			for _, repeated := range child.children {
				if repeated != nil {
					fieldChildren = append(fieldChildren, cloneNodeInArena(arena, repeated))
				}
			}
			continue
		}
		fieldChildren = append(fieldChildren, cloneNodeInArena(arena, child))
	}
	fieldChildren = append(fieldChildren, cloneNodeInArena(arena, closeBrace))

	fieldDeclList := cppNewParent(arena, fieldDeclListSym, symbolIsNamed(lang, fieldDeclListSym), fieldChildren, nil)
	// class_specifier: name/body mirror the C reference runtime's field map
	// (class/base_class_clause carry no field in tree-sitter-cpp).
	classSpecFields := cppFieldIDsAt(arena, lang, 4, map[int]string{1: "name", 3: "body"})
	return cppNewParent(arena, classSpecSym, symbolIsNamed(lang, classSpecSym), []*Node{
		cloneNodeInArena(arena, classTok),
		cloneNodeInArena(arena, name),
		cloneNodeInArena(arena, baseClause),
		fieldDeclList,
	}, classSpecFields)
}

func cppRetaggedClone(arena *nodeArena, n *Node, sym Symbol, named bool) *Node {
	clone := cloneNodeInArena(arena, n)
	cppRetagLeaf(clone, sym, named)
	return clone
}

func cppRetagLeaf(n *Node, sym Symbol, named bool) {
	if n == nil {
		return
	}
	n.symbol = sym
	n.setNamed(named)
	n.children = nil
	n.clearFieldMetadata()
	n.setExtra(false)
	n.setHasError(false)
}

func cppErrorWrapper(arena *nodeArena, child *Node) *Node {
	err := newParentNodeInArena(arena, errorSymbol, true, cloneNodeSliceInArena(arena, []*Node{child}), nil, 0)
	err.setExtra(true)
	err.setHasError(true)
	return err
}

func cppNewParent(arena *nodeArena, sym Symbol, named bool, children []*Node, fieldIDs []FieldID) *Node {
	n := newParentNodeInArena(arena, sym, named, cloneNodeSliceInArena(arena, children), fieldIDs, 0)
	n.setExtra(false)
	n.setHasError(false)
	populateParentNode(n, n.children)
	return n
}

func cppCloneParentWithChildren(arena *nodeArena, template *Node, children []*Node, fieldIDs []FieldID) *Node {
	n := cloneNodeInArena(arena, template)
	n.children = cloneNodeSliceInArena(arena, children)
	n.setFieldMetadata(fieldIDs, defaultFieldSourcesInArena(arena, fieldIDs))
	n.setExtra(false)
	n.setHasError(false)
	populateParentNode(n, n.children)
	return n
}

// cppFieldIDsAt builds a field-ID slice of length count with only byIndex's
// positions assigned, resolving each field name through lang. It returns nil
// when no requested name resolves, so a caller can pass the result straight
// into node construction as "no fields" without a separate nil check.
func cppFieldIDsAt(arena *nodeArena, lang *Language, count int, byIndex map[int]string) []FieldID {
	if lang == nil || count <= 0 || len(byIndex) == 0 {
		return nil
	}
	ids := make([]FieldID, count)
	assigned := false
	for idx, name := range byIndex {
		if idx < 0 || idx >= count {
			continue
		}
		fid, ok := lang.FieldByName(name)
		if !ok || fid == 0 {
			continue
		}
		ids[idx] = fid
		assigned = true
	}
	if !assigned {
		return nil
	}
	return cloneFieldIDSliceInArena(arena, ids)
}
