package gotreesitter

import "bytes"

func normalizeAdaCompatibility(root *Node, source []byte, lang *Language) {
	normalizeAdaCompatibilityWithCensus(root, source, lang, materializationSubpassCensus{})
}

func normalizeAdaCompatibilityWithCensus(
	root *Node,
	source []byte,
	lang *Language,
	census materializationSubpassCensus,
) {
	if root == nil || lang == nil || lang.Name != "ada" || len(source) == 0 {
		return
	}
	recordAggSym, _, ok := symbolMeta(lang, "record_aggregate")
	if !ok {
		return
	}
	positionalArrayAggSym, positionalArrayAggNamed, ok := symbolMeta(lang, "positional_array_aggregate")
	if !ok {
		return
	}
	recordAssocListSym, _, ok := symbolMeta(lang, "record_component_association_list")
	if !ok {
		return
	}
	indexConstraintSym, indexConstraintNamed, ok := symbolMeta(lang, "index_constraint")
	if !ok {
		return
	}
	discriminantConstraintSym, ok := symbolByName(lang, "discriminant_constraint")
	if !ok {
		return
	}
	discriminantAssociationSym, ok := symbolByName(lang, "discriminant_association")
	if !ok {
		return
	}
	expressionSym, ok := symbolByName(lang, "expression")
	if !ok {
		return
	}
	selectedComponentSym, selectedComponentNamed, ok := symbolMeta(lang, "selected_component")
	if !ok {
		return
	}
	tickSym, tickNamed, ok := symbolMeta(lang, "tick")
	if !ok {
		return
	}
	attributeDesignatorSym, attributeDesignatorNamed, ok := symbolMeta(lang, "attribute_designator")
	if !ok {
		return
	}
	identifierSym, identifierNamed, ok := symbolMeta(lang, "identifier")
	if !ok {
		return
	}
	dotSym, dotNamed, ok := symbolMeta(lang, ".")
	if !ok {
		return
	}
	accessSym, accessNamed, ok := symbolMeta(lang, "access")
	if !ok {
		return
	}
	// subtypeMarkFieldID/prefixFieldID/selectorNameFieldID mirror the field
	// names the C oracle attaches when it elects the discrete_range reading
	// of a bare attribute-reference constraint natively: grammar.js's
	// _subtype_indication rule wraps its (hidden) subtype_mark alternative
	// in field('subtype_mark', ...), and that field propagates onto every
	// inlined child of the hidden _attribute_reference/_name chain
	// (selected_component, tick, attribute_designator all carry it);
	// selected_component's own rule separately fields its prefix/selector_name
	// children at every nesting level. adaAttributeReferenceChildren and
	// adaSelectedComponentNode fabricate these nodes by hand, so they must
	// set the same fields explicitly to match the natively-parsed shape.
	subtypeMarkFieldID, ok := lang.FieldByName("subtype_mark")
	if !ok {
		return
	}
	prefixFieldID, ok := lang.FieldByName("prefix")
	if !ok {
		return
	}
	selectorNameFieldID, ok := lang.FieldByName("selector_name")
	if !ok {
		return
	}

	walkResultTree(root, func(n *Node) {
		if n == nil || int(n.endByte) > len(source) || n.startByte > n.endByte {
			return
		}
		children := resultChildSliceForMutation(n)
		if len(children) != 3 || children[1] == nil {
			return
		}
		mid := children[1]
		text := adaTrimmedNodeText(mid, source)
		switch n.symbol {
		case discriminantConstraintSym:
			census.run("dispatch.ada.constraint-kind-election", func() {
				if mid.symbol != discriminantAssociationSym || !bytes.Contains(text, []byte("'")) {
					return
				}
				assocChildren := resultChildSliceForMutation(mid)
				// index_constraint's discrete_range list never carries a
				// discriminant_selector_name/'=>' pair (that shape is only
				// legal for discriminant_constraint's own association list),
				// so a named or multi-child association can never be the
				// index_constraint reading: only the single positional bare
				// expression is a rebuild candidate.
				if len(assocChildren) != 1 || assocChildren[0] == nil || assocChildren[0].symbol != expressionSym {
					return
				}
				rebuiltIsAttributeReference := false
				rebuilt := adaAttributeReferenceChildren(n.ownerArena, source, assocChildren[0], selectedComponentSym, selectedComponentNamed, tickSym, tickNamed, attributeDesignatorSym, attributeDesignatorNamed, identifierSym, identifierNamed, dotSym, dotNamed, accessSym, accessNamed, prefixFieldID, selectorNameFieldID)
				if len(rebuilt) > 0 {
					assocChildren = rebuilt
					rebuiltIsAttributeReference = true
				} else if exprChildren := resultChildSliceForMutation(assocChildren[0]); len(exprChildren) > 0 {
					assocChildren = exprChildren
				}
				out := make([]*Node, 0, len(assocChildren)+2)
				out = append(out, children[0])
				out = append(out, assocChildren...)
				out = append(out, children[2])
				n.symbol = indexConstraintSym
				n.setNamed(indexConstraintNamed)
				replaceNodeChildrenUnfielded(n, cloneNodeSliceIfArena(n.ownerArena, out))
				if rebuiltIsAttributeReference {
					// The hidden _subtype_indication/_attribute_reference
					// chain (grammar.js) pushes field('subtype_mark', ...)
					// onto every inlined child; the rebuilt (selected
					// component, tick, attribute_designator) triple sits at
					// out[1:len(rebuilt)+1].
					for i := 1; i <= len(rebuilt); i++ {
						setNodeChildFieldDirect(n, i, subtypeMarkFieldID)
					}
				}
				nodeInitEquivVersion(n)
			})
		case recordAggSym:
			// Only the positional-list reading survives here.
			// record_component_association_list's positional alternative
			// ("expr, expr, ...", RM 4.3.1) and positional_array_aggregate's
			// own leading alternative are a declared grammar.js conflict
			// ([$.record_component_association_list,
			// $.positional_array_aggregate]) with no dynamic-precedence
			// differentiator; this is a shift/reduce fork the engine still
			// resolves to the record reading (an F-A/F-B table-generation
			// gap, not a runtime election policy this file can fix -- see
			// dispatch.ada's registry entry).
			//
			// The "others"-choice reading that used to share this case
			// (record_aggregate <-> named_array_aggregate, gated on whether
			// the association text started with "others") is gone: that
			// election was driven entirely by the component_choice_list vs
			// discrete_choice tie (RM 4.3.1 vs RM 3.8.1 for a bare "others"),
			// which is now resolved natively before the GLR engine forks
			// (grammars/runtime_profiles.go "ada",
			// ConflictPolicyDeclaredReduceReduceHighestSymbol in
			// conflict_policy.go) -- the raw parse already produces the
			// correct outer aggregate kind, so this file never sees the
			// wrong one to repair.
			census.run("dispatch.ada.aggregate-kind-election", func() {
				if mid.symbol != recordAssocListSym || bytes.Contains(text, []byte("=>")) || !bytes.Contains(text, []byte(",")) {
					return
				}
				listChildren := resultChildSliceForMutation(mid)
				if len(listChildren) == 0 {
					return
				}
				out := make([]*Node, 0, len(listChildren)+2)
				out = append(out, children[0])
				out = append(out, listChildren...)
				out = append(out, children[2])
				n.symbol = positionalArrayAggSym
				n.setNamed(positionalArrayAggNamed)
				replaceNodeChildrenUnfielded(n, cloneNodeSliceIfArena(n.ownerArena, out))
				nodeInitEquivVersion(n)
			})
		}
	})
}

func adaAttributeReferenceChildren(arena *nodeArena, source []byte, expr *Node, selectedComponentSym Symbol, selectedComponentNamed bool, tickSym Symbol, tickNamed bool, attributeDesignatorSym Symbol, attributeDesignatorNamed bool, identifierSym Symbol, identifierNamed bool, dotSym Symbol, dotNamed bool, accessSym Symbol, accessNamed bool, prefixFieldID, selectorNameFieldID FieldID) []*Node {
	if expr == nil || expr.startByte >= expr.endByte || int(expr.endByte) > len(source) {
		return nil
	}
	body := source[expr.startByte:expr.endByte]
	tickOffset := bytes.LastIndexByte(body, '\'')
	if tickOffset <= 0 || tickOffset+1 >= len(body) {
		return nil
	}
	tickStart := expr.startByte + uint32(tickOffset)
	tickEnd := tickStart + 1
	selected := adaSelectedComponentNode(arena, source, expr.startByte, tickStart, selectedComponentSym, selectedComponentNamed, identifierSym, identifierNamed, dotSym, dotNamed, prefixFieldID, selectorNameFieldID)
	if selected == nil {
		return nil
	}
	tick := newLeafNodeInArena(arena, tickSym, tickNamed, tickStart, tickEnd, selected.endPoint, advancePointByBytes(selected.endPoint, body[tickOffset:tickOffset+1]))
	attrIdent := newLeafNodeInArena(arena, accessSym, accessNamed, tickEnd, expr.endByte, tick.endPoint, expr.endPoint)
	attr := newParentNodeInArena(arena, attributeDesignatorSym, attributeDesignatorNamed, []*Node{attrIdent}, nil, 0)
	return cloneNodeSliceIfArena(arena, []*Node{selected, tick, attr})
}

// adaSelectedComponentNode rebuilds the selected_component chain a native
// parse would produce for a dotted prefix (`Pkg.Sub.Obj`), including the
// prefix/selector_name fields grammar.js's selected_component rule attaches
// to every nesting level (field('prefix', $._name) '.'
// field('selector_name', ...)).
func adaSelectedComponentNode(arena *nodeArena, source []byte, start, end uint32, selectedComponentSym Symbol, selectedComponentNamed bool, identifierSym Symbol, identifierNamed bool, dotSym Symbol, dotNamed bool, prefixFieldID, selectorNameFieldID FieldID) *Node {
	if start >= end || int(end) > len(source) {
		return nil
	}
	body := source[start:end]
	dotOffset := bytes.LastIndexByte(body, '.')
	if dotOffset <= 0 || dotOffset+1 >= len(body) {
		startPoint := advancePointByBytes(Point{}, source[:start])
		endPoint := advancePointByBytes(startPoint, body)
		return newLeafNodeInArena(arena, identifierSym, identifierNamed, start, end, startPoint, endPoint)
	}
	dotStart := start + uint32(dotOffset)
	left := adaSelectedComponentNode(arena, source, start, dotStart, selectedComponentSym, selectedComponentNamed, identifierSym, identifierNamed, dotSym, dotNamed, prefixFieldID, selectorNameFieldID)
	if left == nil {
		return nil
	}
	dot := newLeafNodeInArena(arena, dotSym, dotNamed, dotStart, dotStart+1, left.endPoint, advancePointByBytes(left.endPoint, []byte{'.'}))
	rightStart := dotStart + 1
	rightPoint := dot.endPoint
	right := newLeafNodeInArena(arena, identifierSym, identifierNamed, rightStart, end, rightPoint, advancePointByBytes(rightPoint, source[rightStart:end]))
	fieldIDs := []FieldID{prefixFieldID, 0, selectorNameFieldID}
	return newParentNodeInArena(arena, selectedComponentSym, selectedComponentNamed, []*Node{left, dot, right}, fieldIDs, 0)
}

func adaTrimmedNodeText(n *Node, source []byte) []byte {
	if n == nil || n.startByte > n.endByte || int(n.endByte) > len(source) {
		return nil
	}
	return bytes.TrimSpace(source[n.startByte:n.endByte])
}
