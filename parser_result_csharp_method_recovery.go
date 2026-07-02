package gotreesitter

// Lenient source-based C# type/member recovery (issue #136).
//
// #115/#116 gave large real-world files (e.g. Newtonsoft.Json's JsonReader.cs /
// JsonTextReader.cs) a recovered namespace_declaration shell, but its body still
// held only comments — every class, and therefore every method_declaration, was
// lost. Those files exceed the whole-file recovery gate
// (csharpMaxTopLevelChunkRecoverySourceBytes), so the source-based type/method
// reconstruction was never reached; and the child-based reconstruction needs a
// clean sibling sequence the GLR failure destroys.
//
// This pass reconstructs a type declaration and its members directly from source
// with PER-MEMBER bounds instead of a whole-file bound: the type shell (keyword,
// name, base list, braces) is read from source, and each top-level member chunk
// is reparsed on its own inside a minimal `class __Q { … }` wrapper. Because the
// #115 failure is cumulative — every construct parses in isolation — reparsing
// members one at a time recovers them, while a member that still won't parse is
// skipped rather than failing the whole type. Every reparse is a single small
// member snippet, capped by size and count, so the anti-OOM guarantees from
// #64/#98/#106 hold on pathological input.

// csharpRecoverNamespaceBodyMembersFromSource recovers the declarations inside a
// namespace body [openBrace+1, closeBrace) directly from source: each top-level
// chunk is recovered as a comment, a lenient type declaration (class/struct/
// record/interface with per-member reparse), or — for small chunks such as enums
// — a whole-chunk reparse. Unrecoverable chunks are skipped.
func csharpRecoverNamespaceBodyMembersFromSource(source []byte, openBrace, closeBrace uint32, p *Parser, arena *nodeArena) ([]*Node, bool) {
	if p == nil || p.language == nil || arena == nil || openBrace >= closeBrace || closeBrace > uint32(len(source)) {
		return nil, false
	}
	bodyStart := openBrace + 1
	if bodyStart >= closeBrace || bytesAreTrivia(source[bodyStart:closeBrace]) {
		return nil, false
	}
	var members []*Node
	relSpans := csharpTopLevelChunkSpans(source[bodyStart:closeBrace])
	for _, rel := range relSpans {
		spanStart := bodyStart + rel[0]
		spanEnd := bodyStart + rel[1]
		for _, part := range csharpSplitLeadingTopLevelCommentSpans(source, spanStart, spanEnd) {
			if comment, ok := csharpRecoverTopLevelCommentNodeFromRange(source, part[0], part[1], p.language, arena); ok {
				members = append(members, comment)
				continue
			}
			if decl, ok := csharpRecoverNamespaceTypeMemberFromSource(source, part[0], part[1], p, arena); ok {
				members = append(members, decl)
			}
			// else: unrecoverable chunk — skip it leniently.
		}
	}
	// Only claim success when at least one real declaration (not just comments)
	// was recovered.
	for _, m := range members {
		if m != nil && m.Type(p.language) != "comment" {
			return members, true
		}
	}
	return nil, false
}

// csharpRecoverNamespaceTypeMemberFromSource recovers a single namespace member
// (a type declaration) from source[start:end). Large class/struct/record/
// interface declarations are reconstructed shell + lenient members; other or
// small declarations (e.g. enum, delegate) are reparsed whole when they fit.
func csharpRecoverNamespaceTypeMemberFromSource(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	start, end = csharpTrimSpaceBounds(source, start, end)
	if start >= end {
		return nil, false
	}
	if decl, ok := csharpRecoverSourceTypeDeclarationLenient(source, start, end, p, arena); ok {
		return decl, true
	}
	// Fall back to reparsing the whole chunk (bounded) — handles enum/delegate and
	// any type the lenient path doesn't reconstruct.
	if end-start <= csharpMaxMemberRecoverySourceBytes {
		if node, ok := csharpReparseWrappedNamespaceMember(source, start, end, p, arena); ok {
			return node, true
		}
	}
	return nil, false
}

// csharpRecoverSourceTypeDeclarationLenient reconstructs a class/struct/record/
// interface declaration from source, recovering its body members one at a time
// and skipping any that will not parse. Its header (modifiers, attributes, name,
// type parameters and base list) is reparsed with a synthetic empty body so the
// parser handles all header shapes, then the empty body is swapped for the real
// recovered members. Returns ok=false for other keywords so the caller can fall
// back to a whole-chunk reparse.
func csharpRecoverSourceTypeDeclarationLenient(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || arena == nil || start >= end || end > uint32(len(source)) {
		return nil, false
	}
	lang := p.language
	kwStart := csharpSkipModifiersAndAttributes(source, start, end)
	if !(csharpHasKeywordAt(source, kwStart, "class") ||
		csharpHasKeywordAt(source, kwStart, "struct") ||
		csharpHasKeywordAt(source, kwStart, "record") ||
		csharpHasKeywordAt(source, kwStart, "interface")) {
		return nil, false
	}
	openBrace := csharpFindTopLevelByte(source, kwStart, end, '{')
	if openBrace >= end {
		return nil, false
	}
	closeBrace := csharpFindMatchingBraceByte(source, int(openBrace), int(end))
	if closeBrace < 0 || uint32(closeBrace+1) > end {
		return nil, false
	}
	members, ok := csharpRecoverClassBodyMembersLenient(source, uint32(openBrace), uint32(closeBrace), p, arena)
	if !ok || len(members) == 0 {
		return nil, false
	}
	// Reparse the header (up to and including `{`) with a synthetic `}` so the
	// parser produces the type-declaration shell with modifiers/name/base parsed.
	shell := csharpRecoverTypeDeclarationShell(source, start, uint32(openBrace), p, arena)
	if shell == nil {
		return nil, false
	}
	declList, ok := csharpBuildSourceDeclarationListNode(source, uint32(openBrace), uint32(closeBrace), members, lang, arena)
	if !ok {
		return nil, false
	}
	if !csharpReplaceDeclarationList(shell, lang, declList) {
		return nil, false
	}
	shell.setHasError(false)
	extendNodeEndTo(shell, uint32(closeBrace+1), source)
	recomputeNodePointsFromBytes(shell, source)
	return shell, true
}

// csharpRecoverTypeDeclarationShell reparses source[start:openBrace] + " {}" and
// returns the recovered type-declaration node (with an empty declaration_list),
// shifted to original coordinates. The synthetic body is replaced by the caller.
func csharpRecoverTypeDeclarationShell(source []byte, start, openBrace uint32, p *Parser, arena *nodeArena) *Node {
	if start >= openBrace || openBrace > uint32(len(source)) {
		return nil
	}
	header := source[start:openBrace]
	snippet := make([]byte, 0, len(header)+3)
	snippet = append(snippet, header...)
	snippet = append(snippet, " {}"...)
	tree, err := p.parseForRecovery(snippet)
	if err != nil || tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		return nil
	}
	root := tree.RootNode()
	if root.HasError() {
		tree.Release()
		return nil
	}
	var shell *Node
	for _, name := range []string{"class_declaration", "struct_declaration", "record_declaration", "record_struct_declaration", "interface_declaration"} {
		if n := csharpFindFirstNamedDescendantOfType(root, p.language, name); n != nil {
			shell = cloneTreeNodesIntoArena(n, arena)
			break
		}
	}
	tree.Release()
	if shell == nil {
		return nil
	}
	if !shiftNodeBytes(shell, int64(start)) {
		return nil
	}
	return shell
}

// csharpReplaceDeclarationList swaps the (synthetic, empty) declaration_list
// child of a type declaration for the supplied one.
func csharpReplaceDeclarationList(decl *Node, lang *Language, declList *Node) bool {
	if decl == nil || declList == nil {
		return false
	}
	for i, c := range decl.children {
		if c != nil && c.Type(lang) == "declaration_list" {
			decl.children[i] = declList
			declList.parent = decl
			declList.childIndex = int32(i)
			// Re-wire parent pointers and child indices so upward navigation
			// (method → class) stays consistent, matching csharpReplaceMethodBlock.
			populateParentNode(decl, decl.children)
			return true
		}
	}
	return false
}

// csharpSkipModifiersAndAttributes advances past leading attribute groups
// (`[...]`) and modifier keywords to the start of the type keyword.
func csharpSkipModifiersAndAttributes(source []byte, start, end uint32) uint32 {
	i := csharpSkipSpaceBytes(source, start)
	for i < end {
		if source[i] == '[' {
			close, ok := csharpFindMatchingParenLikeBracket(source, i, end)
			if !ok {
				return i
			}
			i = csharpSkipSpaceBytes(source, close+1)
			continue
		}
		wordStart, wordEnd, ok := csharpScanIdentifierAt(source, i)
		if !ok || wordStart != i {
			return i
		}
		if !csharpIsTypeModifierWord(source[wordStart:wordEnd]) {
			return i
		}
		i = csharpSkipSpaceBytes(source, wordEnd)
	}
	return i
}

// csharpFindMatchingParenLikeBracket returns the index of the `]` matching the
// `[` at openPos, honoring nesting.
func csharpFindMatchingParenLikeBracket(source []byte, openPos, end uint32) (uint32, bool) {
	depth := 0
	for i := openPos; i < end; i++ {
		switch source[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

// csharpIsTypeModifierWord reports whether word is a C# type-declaration modifier.
func csharpIsTypeModifierWord(word []byte) bool {
	switch string(word) {
	case "public", "private", "protected", "internal", "static", "sealed",
		"abstract", "partial", "readonly", "unsafe", "new", "file", "ref", "required":
		return true
	}
	return false
}

// csharpRecoverClassBodyMembersLenient recovers the members inside a type body
// [openBrace+1, closeBrace) one chunk at a time, reparsing each member in a
// minimal class wrapper and skipping any that will not parse cleanly. Bounded by
// csharpMaxMemberRecoverySourceBytes per member and csharpMaxTypeMemberRecoveries
// members in total.
func csharpRecoverClassBodyMembersLenient(source []byte, openBrace, closeBrace uint32, p *Parser, arena *nodeArena) ([]*Node, bool) {
	if p == nil || p.language == nil || arena == nil || openBrace >= closeBrace {
		return nil, false
	}
	bodyStart := openBrace + 1
	if bodyStart >= closeBrace {
		return nil, false
	}
	if bytesAreTrivia(source[bodyStart:closeBrace]) {
		return nil, true
	}
	var members []*Node
	reparses := 0
	relSpans := csharpTopLevelChunkSpans(source[bodyStart:closeBrace])
	for _, rel := range relSpans {
		spanStart := bodyStart + rel[0]
		spanEnd := bodyStart + rel[1]
		for _, part := range csharpSplitLeadingTopLevelCommentSpans(source, spanStart, spanEnd) {
			if comment, ok := csharpRecoverTopLevelCommentNodeFromRange(source, part[0], part[1], p.language, arena); ok {
				members = append(members, comment)
				continue
			}
			ps, pe := csharpTrimSpaceBounds(source, part[0], part[1])
			if ps >= pe {
				continue
			}
			// A nested type declaration recurses through the lenient path so its
			// own members are recovered too.
			if nested, ok := csharpRecoverSourceTypeDeclarationLenient(source, ps, pe, p, arena); ok {
				members = append(members, nested)
				continue
			}
			if reparses >= csharpMaxTypeMemberRecoveries || pe-ps > csharpMaxMemberRecoverySourceBytes {
				continue
			}
			reparses++
			// Reparse the whole member first — fast, and yields a complete body when
			// it parses. If it doesn't (a method body with a construct that still
			// fails in isolation), fall back for method-shaped members to the
			// signature shell + lenient block path, which tolerates the bad body.
			if member, ok := csharpReparseWrappedClassMember(source, ps, pe, p, arena); ok {
				members = append(members, member)
				continue
			}
			if source[pe-1] == '}' {
				if method, ok := csharpRecoverClassMethodDeclarationFromRange(source, ps, pe, p, arena); ok {
					members = append(members, method)
					continue
				}
			}
		}
	}
	return members, len(members) > 0
}

// csharpReparseWrappedClassMember reparses source[start:end) as a single member
// inside `class __Q { … }` and returns the recovered member node shifted to
// original coordinates, or ok=false if it does not parse cleanly to a single
// recognised member.
func csharpReparseWrappedClassMember(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	const prefix = "class __Q {\n"
	const suffix = "\n}\n"
	wrapped := make([]byte, 0, len(prefix)+int(end-start)+len(suffix))
	wrapped = append(wrapped, prefix...)
	wrapped = append(wrapped, source[start:end]...)
	wrapped = append(wrapped, suffix...)
	tree, err := p.parseForRecovery(wrapped)
	if err != nil || tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		return nil, false
	}
	root := tree.RootNode()
	if root.HasError() {
		tree.Release()
		return nil, false
	}
	member := csharpExtractSingleWrappedClassMember(root, p.language, arena)
	tree.Release()
	if member == nil {
		return nil, false
	}
	if !shiftNodeBytes(member, int64(start)-int64(len(prefix))) {
		return nil, false
	}
	recomputeNodePointsFromBytes(member, source)
	if member.startByte < start || member.endByte > end {
		return nil, false
	}
	return member, true
}

// csharpReparseWrappedNamespaceMember reparses source[start:end) as a standalone
// top-level declaration (e.g. an enum) and returns the single recovered
// declaration node shifted to original coordinates.
func csharpReparseWrappedNamespaceMember(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	tree, err := p.parseForRecovery(source[start:end])
	if err != nil || tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		return nil, false
	}
	startPoint := advancePointByBytes(Point{}, source[:start])
	offsetRoot := tree.RootNodeWithOffset(start, startPoint)
	if offsetRoot == nil || offsetRoot.HasError() {
		tree.Release()
		return nil, false
	}
	var found *Node
	for _, child := range offsetRoot.children {
		if child == nil || child.IsExtra() || child.HasError() {
			continue
		}
		c := child
		if c.Type(p.language) == "declaration" && len(c.children) == 1 && c.children[0] != nil {
			c = c.children[0]
		}
		if !csharpIsRecoverableMemberType(c.Type(p.language)) {
			continue
		}
		if found != nil {
			tree.Release()
			return nil, false // more than one declaration
		}
		found = cloneTreeNodesIntoArena(c, arena)
	}
	tree.Release()
	return found, found != nil
}

// csharpExtractSingleWrappedClassMember returns the single member declaration
// found inside the wrapper class's declaration_list, or nil if the wrapper did
// not yield exactly one recognised member.
func csharpExtractSingleWrappedClassMember(root *Node, lang *Language, arena *nodeArena) *Node {
	if root == nil || lang == nil {
		return nil
	}
	classDecl := csharpFindFirstNamedDescendantOfType(root, lang, "class_declaration")
	if classDecl == nil {
		return nil
	}
	var declList *Node
	for _, c := range classDecl.children {
		if c != nil && c.Type(lang) == "declaration_list" {
			declList = c
			break
		}
	}
	if declList == nil {
		return nil
	}
	var member *Node
	for _, c := range declList.children {
		if c == nil || !c.IsNamed() {
			continue
		}
		n := c
		if n.Type(lang) == "declaration" && len(n.children) == 1 && n.children[0] != nil {
			n = n.children[0]
		}
		if n.Type(lang) == "comment" {
			continue
		}
		if !csharpIsRecoverableMemberType(n.Type(lang)) {
			return nil
		}
		if member != nil {
			return nil // more than one member — chunk wasn't a single declaration
		}
		member = cloneTreeNodesIntoArena(n, arena)
	}
	return member
}

// csharpCountMethodDeclarations counts method_declaration nodes anywhere within
// the given members (a type body's members are nested under its declaration_list).
func csharpCountMethodDeclarations(members []*Node, lang *Language) int {
	total := 0
	var walk func(n *Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == "method_declaration" {
			total++
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	for _, m := range members {
		walk(m)
	}
	return total
}

// csharpIsRecoverableMemberType reports whether typ is a C# member/type
// declaration this recovery is willing to surface.
func csharpIsRecoverableMemberType(typ string) bool {
	switch typ {
	case "class_declaration",
		"struct_declaration",
		"record_declaration",
		"record_struct_declaration",
		"interface_declaration",
		"enum_declaration",
		"delegate_declaration",
		"namespace_declaration",
		"file_scoped_namespace_declaration",
		"constructor_declaration",
		"destructor_declaration",
		"field_declaration",
		"method_declaration",
		"property_declaration",
		"event_declaration",
		"event_field_declaration",
		"indexer_declaration",
		"operator_declaration",
		"conversion_operator_declaration":
		return true
	}
	return false
}
