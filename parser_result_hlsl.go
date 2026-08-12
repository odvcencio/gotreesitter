package gotreesitter

func normalizeHLSLCompatibility(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "hlsl" || len(source) == 0 {
		return
	}
	walkResultTreePostorder(root, func(n *Node) {
		normalizeHLSLCastNegativeNumber(n, source, lang)
		normalizeHLSLUnormBufferDeclaration(n, source, lang)
	})
}

func normalizeHLSLCastNegativeNumber(n *Node, source []byte, lang *Language) {
	if n == nil || n.Type(lang) != "cast_expression" || resultChildCount(n) != 4 || int(n.endByte) > len(source) {
		return
	}
	open := resultChildAt(n, 0)
	typeDesc := resultChildAt(n, 1)
	close := resultChildAt(n, 2)
	number := resultChildAt(n, 3)
	if open == nil || typeDesc == nil || close == nil || number == nil ||
		open.Type(lang) != "(" || typeDesc.Type(lang) != "type_descriptor" || close.Type(lang) != ")" ||
		number.Type(lang) != "number_literal" || number.startByte >= number.endByte ||
		int(number.startByte) >= len(source) || source[number.startByte] != '-' {
		return
	}
	typeIdent := hlslSingleChildOfType(typeDesc, lang, "type_identifier")
	if typeIdent == nil {
		return
	}
	binarySym, ok := symbolByName(lang, "binary_expression")
	if !ok {
		return
	}
	parenSym, ok := symbolByName(lang, "parenthesized_expression")
	if !ok {
		return
	}
	identifierSym, ok := symbolByName(lang, "identifier")
	if !ok {
		return
	}
	minusSym, ok := symbolByName(lang, "-")
	if !ok {
		return
	}
	numberSym, ok := symbolByName(lang, "number_literal")
	if !ok {
		return
	}

	identifier := hlslCloneLeafAs(typeIdent, identifierSym, symbolIsNamed(lang, identifierSym))
	paren := newParentNodeInArena(n.ownerArena, parenSym, symbolIsNamed(lang, parenSym), []*Node{open, identifier, close}, nil, 0)
	minus := newLeafNodeInArena(n.ownerArena, minusSym, symbolIsNamed(lang, minusSym), number.startByte, number.startByte+1, number.startPoint, advancePointByBytes(number.startPoint, source[number.startByte:number.startByte+1]))
	strippedNumber := newLeafNodeInArena(n.ownerArena, numberSym, symbolIsNamed(lang, numberSym), number.startByte+1, number.endByte, minus.endPoint, number.endPoint)
	n.symbol = binarySym
	n.setNamed(symbolIsNamed(lang, binarySym))
	replaceNodeChildrenUnfielded(n, []*Node{paren, minus, strippedNumber})
}

func normalizeHLSLUnormBufferDeclaration(stmt *Node, source []byte, lang *Language) {
	if stmt == nil || stmt.Type(lang) != "expression_statement" || resultChildCount(stmt) != 2 || int(stmt.endByte) > len(source) {
		return
	}
	pack := resultChildAt(stmt, 0)
	semi := resultChildAt(stmt, 1)
	if pack == nil || semi == nil || pack.Type(lang) != "parameter_pack_expansion" || semi.Type(lang) != ";" ||
		resultChildCount(pack) != 2 {
		return
	}
	outer := resultChildAt(pack, 0)
	if outer == nil || outer.Type(lang) != "binary_expression" || resultChildCount(outer) != 3 {
		return
	}
	templateExpr := resultChildAt(outer, 0)
	gt := resultChildAt(outer, 1)
	name := resultChildAt(outer, 2)
	if templateExpr == nil || gt == nil || name == nil ||
		templateExpr.Type(lang) != "binary_expression" || gt.Type(lang) != ">" || name.Type(lang) != "identifier" ||
		resultChildCount(templateExpr) != 3 {
		return
	}
	buffer := resultChildAt(templateExpr, 0)
	lt := resultChildAt(templateExpr, 1)
	qualified := resultChildAt(templateExpr, 2)
	if buffer == nil || lt == nil || qualified == nil ||
		buffer.Type(lang) != "identifier" || lt.Type(lang) != "<" || qualified.Type(lang) != "qualified_identifier" ||
		resultChildCount(qualified) != 3 {
		return
	}
	unorm := resultChildAt(qualified, 0)
	scope := resultChildAt(qualified, 1)
	elemType := resultChildAt(qualified, 2)
	if unorm == nil || scope == nil || elemType == nil ||
		unorm.Type(lang) != "namespace_identifier" || !scope.IsMissing() || elemType.Type(lang) != "identifier" {
		return
	}
	declSym, ok := symbolByName(lang, "declaration")
	if !ok {
		return
	}
	templateTypeSym, ok := symbolByName(lang, "template_type")
	if !ok {
		return
	}
	templateArgsSym, ok := symbolByName(lang, "template_argument_list")
	if !ok {
		return
	}
	errorSym := errorSymbol
	identifierSym, ok := symbolByName(lang, "identifier")
	if !ok {
		return
	}
	typeIdentifierSym, ok := symbolByName(lang, "type_identifier")
	if !ok {
		return
	}
	typeDescSym, ok := symbolByName(lang, "type_descriptor")
	if !ok {
		return
	}

	bufferType := hlslCloneLeafAs(buffer, typeIdentifierSym, symbolIsNamed(lang, typeIdentifierSym))
	unormIdent := hlslCloneLeafAs(unorm, identifierSym, symbolIsNamed(lang, identifierSym))
	err := newParentNodeInArena(stmt.ownerArena, errorSym, true, []*Node{unormIdent}, nil, 0)
	err.setHasError(true)
	err.setExtra(true)
	elemTypeIdent := hlslCloneLeafAs(elemType, typeIdentifierSym, symbolIsNamed(lang, typeIdentifierSym))
	typeDesc := newParentNodeInArena(stmt.ownerArena, typeDescSym, symbolIsNamed(lang, typeDescSym), []*Node{elemTypeIdent}, nil, 0)
	templateArgs := newParentNodeInArena(stmt.ownerArena, templateArgsSym, symbolIsNamed(lang, templateArgsSym), []*Node{lt, err, typeDesc, gt}, nil, 0)
	templateType := newParentNodeInArena(stmt.ownerArena, templateTypeSym, symbolIsNamed(lang, templateTypeSym), []*Node{bufferType, templateArgs}, nil, 0)

	stmt.symbol = declSym
	stmt.setNamed(symbolIsNamed(lang, declSym))
	stmt.setHasError(true)
	replaceNodeChildrenUnfielded(stmt, []*Node{templateType, name, semi})
}

func hlslSingleChildOfType(n *Node, lang *Language, typ string) *Node {
	if n == nil || resultChildCount(n) != 1 {
		return nil
	}
	child := resultChildAt(n, 0)
	if child == nil || child.Type(lang) != typ {
		return nil
	}
	return child
}

func hlslCloneLeafAs(n *Node, sym Symbol, named bool) *Node {
	if n == nil {
		return nil
	}
	return newLeafNodeInArena(n.ownerArena, sym, named, n.startByte, n.endByte, n.startPoint, n.endPoint)
}
