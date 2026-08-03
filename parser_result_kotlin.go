package gotreesitter

import "strings"

func normalizeKotlinCompatibility(root *Node, source []byte, lang *Language) {
	normalizeKotlinCompatibilityWithCensus(root, source, lang, materializationSubpassCensus{})
}

func normalizeKotlinCompatibilityWithCensus(
	root *Node,
	source []byte,
	lang *Language,
	census materializationSubpassCensus,
) {
	census.run("dispatch.kotlin.recovered-source-file-root", func() {
		normalizeKotlinRecoveredSourceFileRoot(root, source, lang)
	})
	normalizeKotlinInterpolatedCallExpressions(root, lang)
	normalizeKotlinGenericCallTypeArguments(root, source, lang)
	normalizeKotlinPrefixComparisonExpressions(root, source, lang)
	normalizeKotlinRawStringTrailingContent(root, source, lang)
	normalizeKotlinCallableReferenceNavigations(root, source, lang)
	normalizeKotlinReceiverFunctionNames(root, source, lang)
}

// normalizeKotlinGenericCallTypeArguments rewrites the GLR choice
// `f<T>(args)` can take through comparison_expression back into C
// tree-sitter's call_expression + call_suffix(type_arguments, ...).
func normalizeKotlinGenericCallTypeArguments(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "kotlin" || len(source) == 0 {
		return
	}
	comparisonSym, _, ok := symbolMeta(lang, "comparison_expression")
	if !ok {
		return
	}
	callSym, callNamed, ok := symbolMeta(lang, "call_expression")
	if !ok {
		return
	}
	callSuffixSym, callSuffixNamed, ok := symbolMeta(lang, "call_suffix")
	if !ok {
		return
	}
	navigationSym, navigationNamed, ok := symbolMeta(lang, "navigation_expression")
	if !ok {
		return
	}
	typeArgsSym, typeArgsNamed, ok := symbolMeta(lang, "type_arguments")
	if !ok {
		return
	}
	typeProjectionSym, typeProjectionNamed, ok := symbolMeta(lang, "type_projection")
	if !ok {
		return
	}
	userTypeSym, userTypeNamed, ok := symbolMeta(lang, "user_type")
	if !ok {
		return
	}
	typeIDSym, typeIDNamed, ok := symbolMeta(lang, "type_identifier")
	if !ok {
		return
	}
	valueArgsSym, valueArgsNamed, ok := symbolMeta(lang, "value_arguments")
	if !ok {
		return
	}
	valueArgSym, valueArgNamed, ok := symbolMeta(lang, "value_argument")
	if !ok {
		return
	}
	annotatedLambdaSym, annotatedLambdaNamed, ok := symbolMeta(lang, "annotated_lambda")
	if !ok {
		return
	}
	rewroteRecoveredSuffix := false
	walkResultTree(root, func(n *Node) {
		if kotlinNormalizeRecoveredGenericCallSuffix(n, lang, callSuffixSym, callSuffixNamed, navigationSym, navigationNamed) {
			rewroteRecoveredSuffix = true
			return
		}
		if n == nil || n.symbol != comparisonSym || resultChildCount(n) != 3 {
			return
		}
		leftCmp, gt, tail := resultChildAt(n, 0), resultChildAt(n, 1), resultChildAt(n, 2)
		if leftCmp == nil || gt == nil || tail == nil || leftCmp.Type(lang) != "comparison_expression" {
			return
		}
		if !kotlinTokenText(gt, source, ">") || resultChildCount(leftCmp) != 3 || resultChildCount(tail) == 0 {
			return
		}
		base, lt, typeNode := resultChildAt(leftCmp, 0), resultChildAt(leftCmp, 1), resultChildAt(leftCmp, 2)
		if base == nil || lt == nil || typeNode == nil || !kotlinTokenText(lt, source, "<") || lt.endByte != typeNode.startByte || typeNode.endByte != gt.startByte {
			return
		}
		if typ := typeNode.Type(lang); typ != "simple_identifier" && typ != "type_identifier" {
			return
		}
		if resultChildCount(typeNode) != 0 {
			return
		}
		arena := n.ownerArena
		typeNode.symbol = typeIDSym
		typeNode.setNamed(typeIDNamed)
		userType := newParentNodeInArena(arena, userTypeSym, userTypeNamed, cloneNodeSliceInArena(arena, []*Node{typeNode}), nil, 0)
		typeProjection := newParentNodeInArena(arena, typeProjectionSym, typeProjectionNamed, cloneNodeSliceInArena(arena, []*Node{userType}), nil, 0)
		typeArgs := newParentNodeInArena(arena, typeArgsSym, typeArgsNamed, cloneNodeSliceInArena(arena, []*Node{lt, typeProjection, gt}), nil, 0)

		suffixChildren := []*Node{typeArgs}
		switch tail.Type(lang) {
		case "call_expression":
			first := resultChildAt(tail, 0)
			if first == nil {
				return
			}
			if first.Type(lang) == "navigation_expression" {
				if resultChildCount(first) != 2 || resultChildCount(tail) < 2 {
					return
				}
				paren, navSuffix := resultChildAt(first, 0), resultChildAt(first, 1)
				if paren == nil || navSuffix == nil || paren.startByte != gt.endByte {
					return
				}
				if !kotlinRetagParenthesizedAsValueArguments(paren, source, lang, valueArgsSym, valueArgsNamed, valueArgSym, valueArgNamed) {
					return
				}
				innerSuffix := newParentNodeInArena(arena, callSuffixSym, callSuffixNamed, cloneNodeSliceInArena(arena, []*Node{typeArgs, paren}), nil, 0)
				innerCall := newParentNodeInArena(arena, callSym, callNamed, cloneNodeSliceInArena(arena, []*Node{base, innerSuffix}), nil, 0)
				nav := newParentNodeInArena(arena, navigationSym, navigationNamed, cloneNodeSliceInArena(arena, []*Node{innerCall, navSuffix}), nil, 0)
				finalSuffixChildren := make([]*Node, 0, 2)
				kotlinAppendCallSuffixChildren(&finalSuffixChildren, tail, 1, lang)
				if len(finalSuffixChildren) == 0 {
					return
				}
				finalSuffix := newParentNodeInArena(arena, callSuffixSym, callSuffixNamed, cloneNodeSliceInArena(arena, finalSuffixChildren), nil, 0)
				n.symbol = callSym
				n.setNamed(callNamed)
				n.productionID = 0
				replaceNodeChildrenUnfielded(n, cloneNodeSliceInArena(arena, []*Node{nav, finalSuffix}))
				return
			}
			if first.startByte != gt.endByte {
				return
			}
			if !kotlinRetagParenthesizedAsValueArguments(first, source, lang, valueArgsSym, valueArgsNamed, valueArgSym, valueArgNamed) {
				return
			}
			suffixChildren = append(suffixChildren, first)
			kotlinAppendCallSuffixChildren(&suffixChildren, tail, 1, lang)
		case "parenthesized_expression":
			if tail.startByte != gt.endByte {
				return
			}
			if !kotlinRetagParenthesizedAsValueArguments(tail, source, lang, valueArgsSym, valueArgsNamed, valueArgSym, valueArgNamed) {
				return
			}
			suffixChildren = append(suffixChildren, tail)
		case "prefix_expression":
			if resultChildCount(tail) != 2 || tail.startByte <= gt.endByte {
				return
			}
			label, lambda := resultChildAt(tail, 0), resultChildAt(tail, 1)
			if label == nil || lambda == nil || label.Type(lang) != "label" || lambda.Type(lang) != "lambda_literal" {
				return
			}
			annotatedLambda := newParentNodeInArena(arena, annotatedLambdaSym, annotatedLambdaNamed, cloneNodeSliceInArena(arena, []*Node{label, lambda}), nil, 0)
			suffixChildren = append(suffixChildren, annotatedLambda)
		case "lambda_literal":
			if tail.startByte <= gt.endByte {
				return
			}
			annotatedLambda := newParentNodeInArena(arena, annotatedLambdaSym, annotatedLambdaNamed, cloneNodeSliceInArena(arena, []*Node{tail}), nil, 0)
			suffixChildren = append(suffixChildren, annotatedLambda)
		default:
			return
		}
		callSuffix := newParentNodeInArena(arena, callSuffixSym, callSuffixNamed, cloneNodeSliceInArena(arena, suffixChildren), nil, 0)
		n.symbol = callSym
		n.setNamed(callNamed)
		n.productionID = 0
		replaceNodeChildrenUnfielded(n, cloneNodeSliceInArena(arena, []*Node{base, callSuffix}))
	})
	if rewroteRecoveredSuffix {
		kotlinRecomputeHasError(root)
	}
}

func kotlinNormalizeRecoveredGenericCallSuffix(n *Node, lang *Language, callSuffixSym Symbol, callSuffixNamed bool, navigationSym Symbol, navigationNamed bool) bool {
	if n == nil || n.Type(lang) != "call_expression" {
		return false
	}
	switch resultChildCount(n) {
	case 3:
		return kotlinNormalizeRecoveredGenericCallSuffixErrorShape(n, lang, callSuffixSym, callSuffixNamed, navigationSym, navigationNamed)
	case 4:
		return kotlinNormalizeRecoveredGenericCallSuffixFlatShape(n, lang, callSuffixSym, callSuffixNamed, navigationSym, navigationNamed)
	default:
		return false
	}
}

// kotlinNormalizeRecoveredGenericCallSuffixErrorShape handles the C-recovery
// shape where the navigation suffix and/or generic type arguments following a
// call's base expression were absorbed into a single ERROR carrier: base,
// ERROR(navigation_suffix?, type_arguments), call_suffix.
func kotlinNormalizeRecoveredGenericCallSuffixErrorShape(n *Node, lang *Language, callSuffixSym Symbol, callSuffixNamed bool, navigationSym Symbol, navigationNamed bool) bool {
	base, errNode, suffix := resultChildAt(n, 0), resultChildAt(n, 1), resultChildAt(n, 2)
	if base == nil || errNode == nil || suffix == nil {
		return false
	}
	if !errNode.IsError() || suffix.Type(lang) != "call_suffix" {
		return false
	}
	if base.endByte != errNode.startByte || errNode.endByte != suffix.startByte {
		return false
	}
	callBase := base
	var typeArgs *Node
	switch resultChildCount(errNode) {
	case 1:
		typeArgs = resultChildAt(errNode, 0)
		if typeArgs == nil || typeArgs.Type(lang) != "type_arguments" {
			return false
		}
		if typeArgs.startByte != errNode.startByte || typeArgs.endByte != errNode.endByte {
			return false
		}
	case 2:
		navSuffix := resultChildAt(errNode, 0)
		typeArgs = resultChildAt(errNode, 1)
		if navSuffix == nil || typeArgs == nil || navSuffix.Type(lang) != "navigation_suffix" || typeArgs.Type(lang) != "type_arguments" {
			return false
		}
		if navSuffix.startByte != errNode.startByte || navSuffix.endByte != typeArgs.startByte || typeArgs.endByte != errNode.endByte {
			return false
		}
		callBase = newParentNodeInArena(n.ownerArena, navigationSym, navigationNamed, cloneNodeSliceInArena(n.ownerArena, []*Node{base, navSuffix}), nil, 0)
	default:
		return false
	}

	arena := n.ownerArena
	suffixChildren := make([]*Node, 0, resultChildCount(suffix)+1)
	suffixChildren = append(suffixChildren, typeArgs)
	for i := 0; i < resultChildCount(suffix); i++ {
		if child := resultChildAt(suffix, i); child != nil {
			suffixChildren = append(suffixChildren, child)
		}
	}
	newSuffix := newParentNodeInArena(arena, callSuffixSym, callSuffixNamed, cloneNodeSliceInArena(arena, suffixChildren), nil, 0)
	replaceNodeChildrenUnfielded(n, cloneNodeSliceInArena(arena, []*Node{callBase, newSuffix}))
	return true
}

// kotlinNormalizeRecoveredGenericCallSuffixFlatShape handles the same
// navigation-suffix + generic-type-arguments merge, but for a clean (no
// ERROR) C-recovery result that leaves the navigation suffix and type
// arguments as direct call_expression children instead of nesting them under
// navigation_expression/call_suffix: base, navigation_suffix, type_arguments,
// call_suffix. This is the error-free counterpart of the ERROR-carrier shape
// above (same recovered construct, but the faithful C-recovery election
// resolved it without needing to record an error span).
func kotlinNormalizeRecoveredGenericCallSuffixFlatShape(n *Node, lang *Language, callSuffixSym Symbol, callSuffixNamed bool, navigationSym Symbol, navigationNamed bool) bool {
	base, navSuffix, typeArgs, suffix := resultChildAt(n, 0), resultChildAt(n, 1), resultChildAt(n, 2), resultChildAt(n, 3)
	if base == nil || navSuffix == nil || typeArgs == nil || suffix == nil {
		return false
	}
	if navSuffix.Type(lang) != "navigation_suffix" || typeArgs.Type(lang) != "type_arguments" || suffix.Type(lang) != "call_suffix" {
		return false
	}
	if navSuffix.HasError() || typeArgs.HasError() || suffix.HasError() {
		return false
	}
	if base.endByte != navSuffix.startByte || navSuffix.endByte != typeArgs.startByte || typeArgs.endByte != suffix.startByte {
		return false
	}

	arena := n.ownerArena
	callBase := newParentNodeInArena(arena, navigationSym, navigationNamed, cloneNodeSliceInArena(arena, []*Node{base, navSuffix}), nil, 0)
	suffixChildren := make([]*Node, 0, resultChildCount(suffix)+1)
	suffixChildren = append(suffixChildren, typeArgs)
	for i := 0; i < resultChildCount(suffix); i++ {
		if child := resultChildAt(suffix, i); child != nil {
			suffixChildren = append(suffixChildren, child)
		}
	}
	newSuffix := newParentNodeInArena(arena, callSuffixSym, callSuffixNamed, cloneNodeSliceInArena(arena, suffixChildren), nil, 0)
	replaceNodeChildrenUnfielded(n, cloneNodeSliceInArena(arena, []*Node{callBase, newSuffix}))
	return true
}

func kotlinRecomputeHasError(n *Node) bool {
	if n == nil {
		return false
	}
	hasError := n.symbol == errorSymbol || n.isMissing()
	for i := 0; i < resultChildCount(n); i++ {
		if kotlinRecomputeHasError(resultChildAt(n, i)) {
			hasError = true
		}
	}
	n.setHasError(hasError)
	return hasError
}

func kotlinRetagParenthesizedAsValueArguments(paren *Node, source []byte, lang *Language, valueArgsSym Symbol, valueArgsNamed bool, valueArgSym Symbol, valueArgNamed bool) bool {
	if paren == nil || paren.Type(lang) != "parenthesized_expression" || resultChildCount(paren) < 2 {
		return false
	}
	open := resultChildAt(paren, 0)
	close := resultChildAt(paren, resultChildCount(paren)-1)
	if !kotlinTokenText(open, source, "(") || !kotlinTokenText(close, source, ")") {
		return false
	}
	children := []*Node{open}
	switch resultChildCount(paren) {
	case 2:
	case 3:
		arg := resultChildAt(paren, 1)
		if arg == nil {
			return false
		}
		if arg.startByte != arg.endByte {
			valueArg := newParentNodeInArena(paren.ownerArena, valueArgSym, valueArgNamed, cloneNodeSliceInArena(paren.ownerArena, []*Node{arg}), nil, 0)
			children = append(children, valueArg)
		}
	default:
		return false
	}
	children = append(children, close)
	paren.symbol = valueArgsSym
	paren.setNamed(valueArgsNamed)
	replaceNodeChildrenUnfielded(paren, cloneNodeSliceInArena(paren.ownerArena, children))
	return true
}

func kotlinAppendCallSuffixChildren(dst *[]*Node, call *Node, start int, lang *Language) {
	for i := start; i < resultChildCount(call); i++ {
		child := resultChildAt(call, i)
		if child == nil {
			continue
		}
		if child.Type(lang) == "call_suffix" {
			for j := 0; j < resultChildCount(child); j++ {
				if suffixChild := resultChildAt(child, j); suffixChild != nil {
					*dst = append(*dst, suffixChild)
				}
			}
			continue
		}
		*dst = append(*dst, child)
	}
}

// normalizeKotlinPrefixComparisonExpressions fixes the local precedence shape
// where `++x < y` is materialized as `++(x < y)` rather than `(++x) < y`.
func normalizeKotlinPrefixComparisonExpressions(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "kotlin" || len(source) == 0 {
		return
	}
	prefixSym, prefixNamed, ok := symbolMeta(lang, "prefix_expression")
	if !ok {
		return
	}
	comparisonSym, comparisonNamed, ok := symbolMeta(lang, "comparison_expression")
	if !ok {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n == nil || n.symbol != prefixSym || resultChildCount(n) != 2 {
			return
		}
		inc, cmp := resultChildAt(n, 0), resultChildAt(n, 1)
		if inc == nil || cmp == nil || cmp.symbol != comparisonSym || resultChildCount(cmp) != 3 {
			return
		}
		if !kotlinTokenText(inc, source, "++") && !kotlinTokenText(inc, source, "--") {
			return
		}
		left, op, right := resultChildAt(cmp, 0), resultChildAt(cmp, 1), resultChildAt(cmp, 2)
		if left == nil || op == nil || right == nil || inc.endByte != left.startByte {
			return
		}
		arena := n.ownerArena
		prefix := newParentNodeInArena(arena, prefixSym, prefixNamed, cloneNodeSliceInArena(arena, []*Node{inc, left}), nil, 0)
		n.symbol = comparisonSym
		n.setNamed(comparisonNamed)
		n.productionID = 0
		replaceNodeChildrenUnfielded(n, cloneNodeSliceInArena(arena, []*Node{prefix, op, right}))
	})
}

func kotlinTokenText(n *Node, source []byte, text string) bool {
	if n == nil || n.startByte > n.endByte || int(n.endByte) > len(source) {
		return false
	}
	return string(source[n.startByte:n.endByte]) == text
}

// normalizeKotlinReceiverFunctionNames splits the function name back out of a
// receiver type that swallowed it. For `fun Channel.Factory.range(...)`, C
// tree-sitter parses receiver_type `Channel.Factory`, "." and the
// simple_identifier name `range` as separate function_declaration children;
// gotreesitter's GLR instead folds the whole dotted path into receiver_type's
// user_type and leaves a zero-width simple_identifier where the name should
// be. Detect that exact shape and rebuild the C one: shrink the user_type by
// its trailing "." + type_identifier pair, and splice those two tokens into
// the function_declaration as the "." separator and the (retagged)
// simple_identifier name, replacing the zero-width placeholder.
func normalizeKotlinReceiverFunctionNames(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "kotlin" || len(source) == 0 {
		return
	}
	fnSym, ok := lang.symbolByNameAndNamed("function_declaration", true)
	if !ok {
		return
	}
	recvSym, ok := lang.symbolByNameAndNamed("receiver_type", true)
	if !ok {
		return
	}
	userSym, ok := lang.symbolByNameAndNamed("user_type", true)
	if !ok {
		return
	}
	simpleSym, ok := lang.symbolByNameAndNamed("simple_identifier", true)
	if !ok {
		return
	}
	typeIdSym, ok := lang.symbolByNameAndNamed("type_identifier", true)
	if !ok {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n.symbol != fnSym {
			return
		}
		cc := resultChildCount(n)
		idx := -1
		for i := 0; i+1 < cc; i++ {
			c0, c1 := n.children[i], n.children[i+1]
			if c0 != nil && c1 != nil && c0.symbol == recvSym && c1.symbol == simpleSym && c1.startByte == c1.endByte {
				idx = i
				break
			}
		}
		if idx < 0 {
			return
		}
		recv := n.children[idx]
		if resultChildCount(recv) != 1 {
			return
		}
		user := recv.children[0]
		if user == nil || user.symbol != userSym {
			return
		}
		ucc := resultChildCount(user)
		if ucc < 3 {
			return
		}
		dotTok, nameTok := user.children[ucc-2], user.children[ucc-1]
		if dotTok == nil || nameTok == nil || nameTok.symbol != typeIdSym {
			return
		}
		if dotTok.IsNamed() || dotTok.startByte+1 != dotTok.endByte ||
			int(dotTok.endByte) > len(source) || source[dotTok.startByte] != '.' {
			return
		}
		// Shrink user_type (and the receiver_type wrapping it) to end before
		// the trailing "." + name pair.
		user.children = cloneNodeSliceInArena(user.ownerArena, user.children[:ucc-2])
		lastKept := user.children[len(user.children)-1]
		user.endByte = lastKept.endByte
		user.endPoint = lastKept.endPoint
		populateParentNode(user, user.children)
		recv.endByte = user.endByte
		recv.endPoint = user.endPoint
		// The swallowed trailing type_identifier is the function name.
		nameTok.symbol = simpleSym
		children := make([]*Node, 0, cc+1)
		children = append(children, n.children[:idx+1]...)
		children = append(children, dotTok, nameTok)
		children = append(children, n.children[idx+2:]...)
		nodeFieldIDs := n.fieldIDs()
		if len(nodeFieldIDs) == cc {
			fieldIDs := make([]FieldID, 0, cc+1)
			fieldIDs = append(fieldIDs, nodeFieldIDs[:idx+1]...)
			fieldIDs = append(fieldIDs, 0, 0)
			fieldIDs = append(fieldIDs, nodeFieldIDs[idx+2:]...)
			fieldIDs = cloneFieldIDSliceInArena(n.ownerArena, fieldIDs)
			n.setFieldMetadata(fieldIDs, defaultFieldSourcesInArena(n.ownerArena, fieldIDs))
		}
		n.children = cloneNodeSliceInArena(n.ownerArena, children)
		populateParentNode(n, n.children)
	})
}

// normalizeKotlinCallableReferenceNavigations rewrites `Name::target`
// navigation_expression parses into the callable_reference shape C
// tree-sitter selects. The grammar makes `Foo::bar` ambiguous between
// navigation_expression(simple_identifier, navigation_suffix("::" target))
// and callable_reference(type_identifier "::" target); C's GLR resolves the
// conflict to callable_reference, gotreesitter's keeps the navigation parse.
// Only the bare single-identifier base form is rewritten — chained bases
// (`a.b::c`) stay navigation_expressions in C too — and only when the suffix
// carries no type_arguments, which callable_reference cannot hold.
func normalizeKotlinCallableReferenceNavigations(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "kotlin" || len(source) == 0 {
		return
	}
	navSym, ok := lang.symbolByNameAndNamed("navigation_expression", true)
	if !ok {
		return
	}
	suffixSym, ok := lang.symbolByNameAndNamed("navigation_suffix", true)
	if !ok {
		return
	}
	simpleSym, ok := lang.symbolByNameAndNamed("simple_identifier", true)
	if !ok {
		return
	}
	crSym, ok := lang.symbolByNameAndNamed("callable_reference", true)
	if !ok {
		return
	}
	typeIdSym, ok := lang.symbolByNameAndNamed("type_identifier", true)
	if !ok {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n.symbol != navSym || resultChildCount(n) != 2 {
			return
		}
		base, suffix := n.children[0], n.children[1]
		if base == nil || suffix == nil || base.symbol != simpleSym || suffix.symbol != suffixSym {
			return
		}
		if resultChildCount(suffix) != 2 {
			return
		}
		op, target := suffix.children[0], suffix.children[1]
		if op == nil || target == nil || op.startByte+2 != op.endByte ||
			int(op.endByte) > len(source) || string(source[op.startByte:op.endByte]) != "::" {
			return
		}
		if target.symbol != simpleSym {
			if int(target.endByte) > len(source) || string(source[target.startByte:target.endByte]) != "class" || target.IsNamed() {
				return
			}
		}
		base.symbol = typeIdSym
		n.symbol = crSym
		n.setNamed(true)
		n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{base, op, target})
		n.clearFieldMetadata()
		n.productionID = 0
		populateParentNode(n, n.children)
	})
}

// normalizeKotlinRawStringTrailingContent restores the final raw-string
// content chunk after an embedded quote. C tree-sitter keeps parsing
// string_content until the closing triple quote; the Go parse can stop the
// child list at the embedded quote even though the string_literal span is
// correct.
func normalizeKotlinRawStringTrailingContent(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "kotlin" || len(source) == 0 {
		return
	}
	stringSym, ok := lang.symbolByNameAndNamed("string_literal", true)
	if !ok {
		return
	}
	contentSym, ok := lang.symbolByNameAndNamed("string_content", true)
	if !ok {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n == nil || n.symbol != stringSym {
			return
		}
		if n.startByte+6 > n.endByte || int(n.endByte) > len(source) {
			return
		}
		if string(source[n.startByte:n.startByte+3]) != `"""` || string(source[n.endByte-3:n.endByte]) != `"""` {
			return
		}
		children := resultChildSliceForMutation(n)
		if len(children) == 0 {
			return
		}
		last := children[len(children)-1]
		contentEnd := n.endByte - 3
		if last == nil || last.symbol != contentSym || last.endByte >= contentEnd {
			return
		}
		gap := source[last.endByte:contentEnd]
		if len(gap) == 0 || strings.Contains(string(gap), "$") {
			return
		}
		content := newLeafNodeInArena(
			n.ownerArena,
			contentSym,
			symbolIsNamed(lang, contentSym),
			last.endByte,
			contentEnd,
			last.endPoint,
			advancePointByBytes(last.endPoint, gap),
		)
		startByte, endByte := n.startByte, n.endByte
		startPoint, endPoint := n.startPoint, n.endPoint
		children = append(append([]*Node{}, children...), content)
		n.children = cloneNodeSliceInArena(n.ownerArena, children)
		n.clearFieldMetadata()
		populateParentNode(n, n.children)
		n.startByte, n.endByte = startByte, endByte
		n.startPoint, n.endPoint = startPoint, endPoint
	})
}

// normalizeKotlinRecoveredSourceFileRoot retags a recovered ERROR root as
// `source_file` when its children still look like Kotlin top-level
// declarations.
//
// This member was deleted on 2026-08-02 as dead code and is restored here. The
// census behind the deletion measured the fresh, over-64-KiB, incremental and
// pinned routes. It did not measure Parser.SetIncludedRanges. On that route the
// member fires and moves the published root toward the reference runtime.
//
// Witness: testdata/included_ranges/kotlin_work_queue_test.kt with the three
// ranges pinned by parser_included_ranges_kotlin_root_test.go. Without this
// member the root is `ERROR`; with it the root is `source_file`, which is the
// kind the locked Kotlin C oracle publishes for the same input.
//
// The route gate lives in parser_included_ranges_kotlin_root_test.go and
// cgo_harness/included_ranges_kotlin_root_parity_cgo_test.go.
//
// READ THIS BEFORE PROPOSING THIS MEMBER FOR RETIREMENT AGAIN. This comment
// justifies the member TODAY. It is not a permanent justification, for two
// measured reasons.
//
//  1. The route is defective, and the member may be live only because of that
//     defect. bytesAreParserPadding (parser.go), reached through
//     realTokenAttachmentGapIsParserPadding, scans raw bytes between the stack
//     offset and the next token start with no range clipping. A non-whitespace
//     gap between two included ranges therefore kills every GLR stack and
//     forces C-recovery where the reference runtime never recovers. The
//     range-aware twin already exists: parserTailAllowsCleanAcceptance. Make
//     the ranges of the witness contiguous and both facts appear together: the
//     parse reaches the oracle's span with no error, and this member goes
//     inert. After the clipping fix lands, re-run the census and decide this
//     member's fate on the corrected route.
//
//  2. What this member DOES is retag the root. What it CAUSES is wider, and
//     not always toward the reference runtime. On
//     kotlinx-coroutines-core/common/test/DelayTest.kt with three ranges, the
//     retag alone is necessary and sufficient to make a later stage flatten a
//     clean class_declaration into root-level members: 7 children become 12,
//     while the C oracle and the pre-restoration build both keep the
//     class_declaration. Neither normalizeKotlinTopLevelFunctionFragments nor
//     the error refresh is involved; skipping either still reproduces the
//     flattening, and skipping only retagResultRoot removes it. So a census
//     receipt for this member bounds what it rewrote, not what its retag led
//     downstream stages to do.
func normalizeKotlinRecoveredSourceFileRoot(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "kotlin" || root.Type(lang) != "ERROR" {
		return
	}
	if !kotlinRootLooksRecoverableSourceFile(root, lang) {
		return
	}
	normalizeKotlinTopLevelFunctionFragments(root, source, lang)
	sym, ok := symbolByName(lang, "source_file")
	if !ok {
		return
	}
	retagResultRootAndRefreshError(root, sym, symbolIsNamed(lang, sym))
}

func kotlinRootLooksRecoverableSourceFile(root *Node, lang *Language) bool {
	if root == nil || lang == nil || resultChildCount(root) == 0 {
		return false
	}
	for i := 0; i < resultChildCount(root); i++ {
		child := resultChildAt(root, i)
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "package_header",
			"import_list",
			"class_declaration",
			"function_declaration",
			"object_declaration",
			"property_declaration",
			"typealias_declaration",
			"multiline_comment",
			"line_comment":
			return true
		}
	}
	return false
}

func normalizeKotlinTopLevelFunctionFragments(root *Node, source []byte, lang *Language) {
	fnSym, ok := symbolByName(lang, "function_declaration")
	if !ok {
		return
	}
	funSym, ok := symbolByName(lang, "fun")
	if !ok {
		return
	}
	children := resultChildSliceForMutation(root)
	if len(children) < 3 {
		return
	}
	arena := root.ownerArena
	if arena == nil {
		return
	}
	rebuilt := make([]*Node, 0, len(children))
	changed := false
	for i := 0; i < len(children); i++ {
		if fn, ok := kotlinRecoveredTopLevelFunction(arena, children, i, source, lang, fnSym, funSym); ok {
			rebuilt = append(rebuilt, fn)
			i += 2
			changed = true
			continue
		}
		rebuilt = append(rebuilt, children[i])
	}
	if !changed {
		return
	}
	replaceNodeChildrenUnfielded(root, cloneNodeSliceInArena(arena, rebuilt))
}

func kotlinRecoveredTopLevelFunction(arena *nodeArena, children []*Node, idx int, source []byte, lang *Language, fnSym, funSym Symbol) (*Node, bool) {
	if idx+2 >= len(children) {
		return nil, false
	}
	funKeyword := children[idx]
	name := children[idx+1]
	params := children[idx+2]
	if funKeyword == nil || name == nil || params == nil {
		return nil, false
	}
	if funKeyword.Type(lang) != "ERROR" || strings.TrimSpace(funKeyword.Text(source)) != "fun" {
		return nil, false
	}
	if name.Type(lang) != "simple_identifier" || params.Type(lang) != "function_value_parameters" {
		return nil, false
	}
	retagResultRoot(funKeyword, funSym, symbolIsNamed(lang, funSym))
	funKeyword.setHasError(false)
	fnChildren := cloneNodeSliceInArena(funKeyword.ownerArena, []*Node{funKeyword, name, params})
	fn := newParentNodeInArena(funKeyword.ownerArena, fnSym, symbolIsNamed(lang, fnSym), fnChildren, nil, 0)
	fn.setHasError(true)
	return fn, true
}

func normalizeKotlinInterpolatedCallExpressions(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "kotlin" {
		return
	}
	callExpressionSym, ok := lang.symbolByNameAndNamed("call_expression", true)
	if !ok {
		callExpressionSym, ok = symbolByName(lang, "call_expression")
		if !ok {
			return
		}
	}
	callExpressionNamed := symbolIsNamed(lang, callExpressionSym)

	walkResultTree(root, func(n *Node) {
		if n == nil || symbolTypeName(lang, n.symbol) != "interpolated_expression" || resultChildCount(n) != 2 {
			return
		}
		first := resultChildAt(n, 0)
		second := resultChildAt(n, 1)
		if first == nil || second == nil ||
			symbolTypeName(lang, first.symbol) != "navigation_expression" ||
			symbolTypeName(lang, second.symbol) != "call_suffix" {
			return
		}
		arena := n.ownerArena
		callChildren := cloneNodeSliceInArena(arena, []*Node{first, second})
		call := newParentNodeInArena(arena, callExpressionSym, callExpressionNamed, callChildren, nil, 0)
		replaceNodeChildrenUnfielded(n, cloneNodeSliceInArena(arena, []*Node{call}))
	})
}
