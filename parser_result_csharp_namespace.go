package gotreesitter

// csharpBuildRecoveredNamespaceDeclarationFromErrorRoot synthesises a
// namespace_declaration from the ERROR root of a namespace sub-parse whose body
// could not be parsed cleanly. The keyword, name and braces are read directly
// from source (mirroring csharpRecoverNamespaceFromChildren), and the body
// declarations are recovered leniently from the already-parsed children of
// errRoot — so no additional reparse is performed. This is the partial-recovery
// fallback for issue #115, where large real-world files collapse to a single
// ERROR node and the strict whole-namespace extraction finds no clean
// namespace_declaration.
func csharpBuildRecoveredNamespaceDeclarationFromErrorRoot(errRoot *Node, source []byte, start, end uint32, p *Parser, lang *Language, arena *nodeArena) (*Node, bool) {
	if errRoot == nil || lang == nil || arena == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	nsKwStart := csharpSkipSpaceBytes(source, start)
	if !csharpHasKeywordAt(source, nsKwStart, "namespace") {
		return nil, false
	}
	nsKwEnd := nsKwStart + uint32(len("namespace"))
	nameStart := csharpSkipSpaceBytes(source, nsKwEnd)
	openBrace := csharpFindTopLevelByte(source, nameStart, end, '{')
	if openBrace >= end {
		return nil, false
	}
	nameEnd := csharpTrimRightSpaceBytes(source, openBrace)
	if nameStart >= nameEnd {
		return nil, false
	}
	closeBrace := csharpFindMatchingBraceByte(source, int(openBrace), int(end))
	if closeBrace < 0 || uint32(closeBrace+1) != end {
		return nil, false
	}

	keywordTok, ok := csharpBuildLeafNodeByName(arena, source, lang, "namespace", nsKwStart, nsKwEnd)
	if !ok {
		return nil, false
	}
	nameNode, ok := csharpNamespaceNameNodeFromErrorRoot(errRoot, source, nameStart, openBrace, lang, arena)
	if !ok {
		return nil, false
	}
	members, ok := csharpRecoverNamespaceBodyMembersFromErrorRoot(errRoot, source, openBrace, uint32(closeBrace), p, lang, arena)
	// The child-based pass recovers a type shell but, on the shredded bodies of
	// large files (issue #136), none of its methods — and sometimes not even the
	// class. When it yields no recoverable method, reconstruct the body's type
	// declarations and their members directly from source (bounded per member).
	// Keep the source-based pass only when it surfaces strictly more methods (or
	// the child-based pass failed outright): if neither finds a method, the
	// child-based members are structurally more accurate for non-method members.
	if !ok || csharpCountMethodDeclarations(members, lang) == 0 {
		if sourceMembers, sok := csharpRecoverNamespaceBodyMembersFromSource(source, openBrace, uint32(closeBrace), p, arena); sok {
			if !ok || csharpCountMethodDeclarations(sourceMembers, lang) > csharpCountMethodDeclarations(members, lang) {
				members, ok = sourceMembers, true
			}
		}
	}
	if !ok || len(members) == 0 {
		return nil, false
	}
	declList, ok := csharpBuildSourceDeclarationListNode(source, openBrace, uint32(closeBrace), members, lang, arena)
	if !ok {
		return nil, false
	}

	declSym, ok := symbolByName(lang, "namespace_declaration")
	if !ok {
		return nil, false
	}
	nameFieldID, _ := lang.FieldByName("name")
	bodyFieldID, _ := lang.FieldByName("body")
	children := arena.allocNodeSlice(3)
	children[0] = keywordTok
	children[1] = nameNode
	children[2] = declList
	fieldIDs := cloneFieldIDSliceInArena(arena, []FieldID{0, nameFieldID, bodyFieldID})
	decl := newParentNodeInArena(arena, declSym, symbolIsNamed(lang, declSym), children, fieldIDs, 0)
	decl.setHasError(false)
	extendNodeEndTo(decl, end, source)
	return decl, true
}

// csharpNamespaceNameNodeFromErrorRoot returns the namespace name node, reusing
// the already-parsed qualified_name/identifier from the sub-parse children when
// available (it is offset-adjusted to the original source) and falling back to
// rebuilding it from source.
func csharpNamespaceNameNodeFromErrorRoot(errRoot *Node, source []byte, nameStart, openBrace uint32, lang *Language, arena *nodeArena) (*Node, bool) {
	for _, c := range errRoot.children {
		if c == nil || c.HasError() {
			continue
		}
		switch c.Type(lang) {
		case "qualified_name", "identifier":
			if c.startByte >= nameStart && c.endByte <= openBrace {
				return cloneTreeNodesIntoArena(c, arena), true
			}
		}
	}
	nameEnd := csharpTrimRightSpaceBytes(source, openBrace)
	if nameStart >= nameEnd {
		return nil, false
	}
	return csharpBuildQualifiedNameNode(source, nameStart, nameEnd, lang, arena)
}

// csharpRecoverNamespaceBodyMembersFromErrorRoot recovers the declarations that
// live inside a namespace body from the (offset-adjusted) children of the
// namespace sub-parse ERROR root. It performs a single O(children) pass: each
// step either consumes a recovered type declaration (advancing past all of its
// children) or skips an unrecoverable fragment. Type bodies are recovered in
// lenient mode so internal ERROR fragments do not abort the enclosing type.
func csharpRecoverNamespaceBodyMembersFromErrorRoot(errRoot *Node, source []byte, openBrace, closeBrace uint32, p *Parser, lang *Language, arena *nodeArena) ([]*Node, bool) {
	if errRoot == nil || lang == nil || arena == nil || openBrace >= closeBrace {
		return nil, false
	}
	children := errRoot.children
	members := make([]*Node, 0, len(children))
	for i := 0; i < len(children); {
		child := children[i]
		if child == nil || child.endByte <= openBrace+1 || child.startByte >= closeBrace {
			i++
			continue
		}
		if recovered, next, ok := csharpRecoverNonEmptyTopLevelTypeDeclarationFromChildren(children, i, source, p, lang, arena, true); ok {
			members = append(members, recovered)
			i = next
			continue
		}
		if member, ok := csharpRecoverTypeDeclarationBodyChild(child, lang, arena); ok {
			members = append(members, member)
			i++
			continue
		}
		// Unrecoverable fragment (e.g. an ERROR span or namespace header token):
		// skip it so the declarations that did parse still surface (issue #115).
		i++
	}
	return members, len(members) > 0
}
