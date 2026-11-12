package gotreesitter

import (
	"bytes"
	"strings"
)

// csharpTypeDeclarationModifierKeywords lists the C# modifier keywords that
// can precede a class/record declaration (e.g. "public partial class Foo").
// Used by csharpRecoverSourceTopLevelTypeDeclarationFromRange's short-term
// #136 relief so a real-world top-level type (almost always modifier-
// prefixed) is recognized instead of only a bare "class"/"record" keyword.
var csharpTypeDeclarationModifierKeywords = []string{
	"public", "private", "protected", "internal",
	"static", "sealed", "abstract", "partial",
	"unsafe", "new", "file", "readonly",
}

// csharpScanTypeDeclarationModifiers scans zero or more modifier keywords
// starting at start (skipping whitespace/comments between them) and returns
// their byte spans plus the position right after the last one recognized.
func csharpScanTypeDeclarationModifiers(source []byte, start, end uint32) ([][2]uint32, uint32) {
	var mods [][2]uint32
	cursor := start
	for {
		next := csharpSkipSpaceBytes(source, cursor)
		matched := false
		for _, kw := range csharpTypeDeclarationModifierKeywords {
			if !csharpHasKeywordAt(source, next, kw) {
				continue
			}
			kwEnd := next + uint32(len(kw))
			if kwEnd < end && csharpIdentifierContinueByte(source[kwEnd]) {
				continue
			}
			mods = append(mods, [2]uint32{next, kwEnd})
			cursor = kwEnd
			matched = true
			break
		}
		if !matched {
			return mods, cursor
		}
	}
}

// csharpSkipLeadingPreprocDirectiveLines skips whitespace, standalone
// preprocessor directive lines (#endif/#region/#endregion/#if/#else/#elif/
// etc), and comments — in any interleaving — at the start of [start, end),
// returning the position of the first non-trivia byte. Heuristic, not full
// lexing — SHORT-TERM RELIEF for issue #136 (reviewer follow-up): a
// top-level chunk recovered from a whole-file GLR failure can start with a
// DANGLING directive left over from an unbalanced #if/#region elsewhere in
// the same file, optionally followed by the declaration's own leading doc
// comment (e.g. XmlNodeConverter.cs has "...#endif\n#endregion\n\n    ///
// <summary>\n    /// Converts XML to and from JSON.\n    /// </summary>\n
// public class XmlNodeConverter..." — the class itself is balanced, but a
// PRIOR class's own #if block was left open across an intervening
// #endregion), which would otherwise make
// csharpRecoverSourceTopLevelTypeDeclarationFromRange's modifier/keyword
// scan fail to recognize the real declaration that follows. The skipped
// leading comments are not reattached as children (consistent with how
// csharpRecoverTopLevelChunkNodesFromRange's caller already treats a
// chunk's own leading comments as separate, unattached sibling nodes in
// the common case).
func csharpSkipLeadingPreprocDirectiveLines(source []byte, start, end uint32) uint32 {
	cursor := start
	for {
		next := csharpSkipSpaceBytes(source, cursor)
		if next >= end {
			return next
		}
		if source[next] == '#' {
			lineEnd := next
			for lineEnd < end && source[lineEnd] != '\n' {
				lineEnd++
			}
			cursor = lineEnd
			continue
		}
		if next+1 < end && source[next] == '/' && source[next+1] == '/' {
			lineEnd := next + 2
			for lineEnd < end && source[lineEnd] != '\n' {
				lineEnd++
			}
			cursor = lineEnd
			continue
		}
		if next+1 < end && source[next] == '/' && source[next+1] == '*' {
			commentEnd := csharpFindBlockCommentEnd(source, next+2, end)
			if commentEnd <= next+1 {
				return next
			}
			cursor = commentEnd
			continue
		}
		return next
	}
}

func csharpRecoverSourceTopLevelTypeDeclarationFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena, lenient bool) (*Node, bool) {
	if p == nil || p.language == nil || arena == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = csharpTrimSpaceBounds(source, start, end)
	if start >= end {
		return nil, false
	}
	modifierSearchStart := csharpSkipLeadingPreprocDirectiveLines(source, start, end)
	modifierSpans, afterModifiers := csharpScanTypeDeclarationModifiers(source, modifierSearchStart, end)
	declStart := csharpSkipSpaceBytes(source, afterModifiers)
	keyword := ""
	declName := ""
	switch {
	case csharpHasKeywordAt(source, declStart, "class"):
		keyword = "class"
		declName = "class_declaration"
	case csharpHasKeywordAt(source, declStart, "record"):
		keyword = "record"
		declName = "record_declaration"
	// SHORT-TERM RELIEF for issue #136 (reviewer follow-up): interface/struct
	// bodies use the exact same member-declaration syntax as a class body
	// (a bodyless "void M();" signature is syntactically valid inside a
	// "class __Q { ... }" reparse wrapper too — tree-sitter doesn't enforce
	// the C# semantic rule that a body-less method must live in an
	// interface/abstract/partial/extern context), so
	// csharpRecoverSourceTypeMembersFromRange below needs no changes to
	// handle them. Without this, a real-world interface/struct block (e.g.
	// XmlNodeConverter.cs's "internal interface IXmlNode { ... }") is an
	// unrecognized top-level chunk that either falls through to the
	// generic whole-chunk reparse fallback (fine when small) or, if
	// oversized, is skipped outright — losing every method_declaration
	// inside it.
	case csharpHasKeywordAt(source, declStart, "interface"):
		keyword = "interface"
		declName = "interface_declaration"
	case csharpHasKeywordAt(source, declStart, "struct"):
		keyword = "struct"
		declName = "struct_declaration"
	case csharpHasKeywordAt(source, declStart, "enum"):
		// enum_member_declaration_list is comma-separated, not ";"/"}"
		// terminated, so it doesn't fit csharpRecoverSourceTypeMembersFromRange's
		// chunk-splitting model the way class/struct/interface bodies do.
		// Recover it as a single bounded whole-span reparse instead (enums
		// contribute no method_declarations, so there is no per-member
		// recovery to preserve here — either the whole enum parses cleanly
		// or, in lenient mode, it is skipped).
		return csharpRecoverEnumDeclarationFromRange(source, start, end, p, arena, lenient)
	default:
		return nil, false
	}
	lang := p.language
	var modifiers []*Node
	for _, span := range modifierSpans {
		modNode, ok := csharpBuildModifierNodeFromSource(arena, source, lang, span[0], span[1])
		if !ok {
			return nil, false
		}
		modifiers = append(modifiers, modNode)
	}
	keywordEnd := declStart + uint32(len(keyword))
	nameStart, nameEnd, ok := csharpScanIdentifierAt(source, csharpSkipSpaceBytes(source, keywordEnd))
	if !ok {
		return nil, false
	}
	cursor := csharpSkipSpaceBytes(source, nameEnd)
	var parameterList *Node
	if cursor < end && source[cursor] == '(' {
		closeParen, ok := csharpFindMatchingParenByte(source, cursor, end)
		if !ok {
			return nil, false
		}
		parameterList, ok = csharpBuildLambdaParameterListNode(arena, source, lang, cursor, closeParen+1)
		if !ok {
			return nil, false
		}
		cursor = csharpSkipSpaceBytes(source, closeParen+1)
	}
	bodyStart := uint32(0)
	bodyEnd := uint32(0)
	headerEnd := end
	if openBrace := csharpFindTopLevelByte(source, cursor, end, '{'); openBrace < end {
		closeBrace := csharpFindMatchingBraceByte(source, int(openBrace), int(end))
		if closeBrace < 0 || uint32(closeBrace+1) > end {
			return nil, false
		}
		bodyStart = openBrace
		bodyEnd = uint32(closeBrace)
		headerEnd = openBrace
	} else if end > start && source[end-1] == ';' {
		headerEnd = end - 1
	} else {
		return nil, false
	}
	var baseList *Node
	if colon := csharpFindTopLevelByte(source, cursor, headerEnd, ':'); colon < headerEnd {
		baseList, ok = csharpBuildSourceBaseListNode(source, colon, headerEnd, lang, arena)
		if !ok {
			return nil, false
		}
	}
	commentStart := nameEnd
	if parameterList != nil {
		commentStart = parameterList.endByte
	}
	if baseList != nil {
		commentStart = baseList.endByte
	}
	comments := csharpBuildCommentNodesBetween(source, commentStart, headerEnd, lang, arena)
	var declarationList *Node
	if bodyStart < bodyEnd {
		members, ok := csharpRecoverSourceTypeMembersFromRange(source, bodyStart+1, bodyEnd, p, arena, lenient)
		if !ok {
			return nil, false
		}
		declarationList, ok = csharpBuildSourceDeclarationListNode(source, bodyStart, bodyEnd, members, lang, arena)
		if !ok {
			return nil, false
		}
	}
	declSym, ok := symbolByName(lang, declName)
	if !ok {
		return nil, false
	}
	keywordTok, ok := csharpBuildLeafNodeByName(arena, source, lang, keyword, declStart, keywordEnd)
	if !ok {
		return nil, false
	}
	nameNode, ok := csharpBuildIdentifierNodeFromSource(source, nameStart, nameEnd, lang, arena)
	if !ok {
		return nil, false
	}
	children := make([]*Node, 0, len(modifiers)+2)
	children = append(children, modifiers...)
	children = append(children, keywordTok, nameNode)
	if parameterList != nil {
		children = append(children, parameterList)
	}
	if baseList != nil {
		children = append(children, baseList)
	}
	children = append(children, comments...)
	if declarationList != nil {
		children = append(children, declarationList)
	}
	if bodyStart == 0 && end > start && source[end-1] == ';' {
		if semiTok, ok := csharpBuildLeafNodeByName(arena, source, lang, ";", end-1, end); ok {
			children = append(children, semiTok)
		}
	}
	buf := arena.allocNodeSlice(len(children))
	copy(buf, children)
	named := symbolIsNamed(lang, declSym)
	decl := newParentNodeInArena(arena, declSym, named, buf, nil, 0)
	extendNodeEndTo(decl, end, source)
	decl.setHasError(false)
	return decl, true
}

// csharpRecoverEnumDeclarationFromRange recovers a "[modifiers] enum Name
// [: base] { members }" declaration by reparsing the whole span as a
// standalone snippet through the real grammar tables, rather than
// hand-building an enum_member_declaration_list the way
// csharpRecoverSourceTypeMembersFromRange does for class/struct/interface
// bodies. Short-term relief for issue #136 (reviewer follow-up): bounded in
// lenient mode by csharpMaxTopLevelChunkRecoverySourceBytes, matching the
// per-chunk bound csharpRecoverTopLevelChunkNodesFromRange already applies
// to its own unrecognized-chunk fallback, so a single oversized enum cannot
// trigger an unbounded reparse.
func csharpRecoverEnumDeclarationFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena, lenient bool) (*Node, bool) {
	if p == nil || p.language == nil || arena == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start = csharpSkipLeadingPreprocDirectiveLines(source, start, end)
	if start >= end {
		return nil, false
	}
	if lenient && end-start > csharpMaxTopLevelChunkRecoverySourceBytes {
		return nil, false
	}
	tree, err := p.parseForRecovery(source[start:end])
	if err != nil || tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		return nil, false
	}
	defer tree.Release()
	startPoint := advancePointByBytes(Point{}, source[:start])
	offsetRoot := tree.rootNodeWithRecoveryOffset(start, startPoint)
	if offsetRoot == nil || offsetRoot.HasError() {
		return nil, false
	}
	enumDecl := csharpFindFirstNamedDescendantOfType(offsetRoot, p.language, "enum_declaration")
	if enumDecl == nil || enumDecl.endByte != end {
		return nil, false
	}
	return cloneTreeNodesIntoArena(enumDecl, arena), true
}

func csharpFindTopLevelByte(source []byte, start, end uint32, want byte) uint32 {
	if end > uint32(len(source)) {
		end = uint32(len(source))
	}
	parenDepth := 0
	bracketDepth := 0
	for i := start; i < end; i++ {
		if source[i] == want && parenDepth == 0 && bracketDepth == 0 {
			return i
		}
		switch source[i] {
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		}
	}
	return end
}

func csharpBuildSourceBaseListNode(source []byte, colon, end uint32, lang *Language, arena *nodeArena) (*Node, bool) {
	if lang == nil || arena == nil || colon >= end || int(end) > len(source) || source[colon] != ':' {
		return nil, false
	}
	sym, ok := symbolByName(lang, "base_list")
	if !ok {
		return nil, false
	}
	colonTok, ok := csharpBuildLeafNodeByName(arena, source, lang, ":", colon, colon+1)
	if !ok {
		return nil, false
	}
	children := []*Node{colonTok}
	items := csharpSplitTopLevelByComma(source, colon+1, end)
	for i, span := range items {
		itemStart, itemEnd := csharpTrimSpaceBounds(source, span[0], span[1])
		if itemStart >= itemEnd {
			continue
		}
		node, ok := csharpBuildSourceBaseTypeNode(source, itemStart, itemEnd, lang, arena)
		if !ok {
			return nil, false
		}
		children = append(children, node)
		if i < len(items)-1 {
			commaPos := csharpFindCommaBetween(source, span[1], items[i+1][0])
			if commaPos == 0 {
				commaPos = span[1]
			}
			commaTok, ok := csharpBuildLeafNodeByName(arena, source, lang, ",", commaPos, commaPos+1)
			if !ok {
				return nil, false
			}
			children = append(children, commaTok)
		}
	}
	buf := arena.allocNodeSlice(len(children))
	copy(buf, children)
	named := symbolIsNamed(lang, sym)
	return newParentNodeInArena(arena, sym, named, buf, nil, 0), true
}

func csharpBuildSourceBaseTypeNode(source []byte, start, end uint32, lang *Language, arena *nodeArena) (*Node, bool) {
	if end > start && source[end-1] == ')' {
		openParen := csharpFindTopLevelByte(source, start, end, '(')
		if openParen < end {
			typeNode, ok := csharpBuildTypeNameNodeFromSource(arena, source, lang, start, openParen)
			if !ok {
				return nil, false
			}
			args, ok := csharpBuildArgumentListNode(arena, source, lang, openParen, end)
			if !ok {
				return nil, false
			}
			sym, ok := symbolByName(lang, "primary_constructor_base_type")
			if !ok {
				return nil, false
			}
			named := symbolIsNamed(lang, sym)
			return newParentNodeInArena(arena, sym, named, []*Node{typeNode, args}, nil, 0), true
		}
	}
	return csharpBuildTypeNameNodeFromSource(arena, source, lang, start, end)
}

func csharpBuildCommentNodesBetween(source []byte, start, end uint32, lang *Language, arena *nodeArena) []*Node {
	var comments []*Node
	for _, span := range csharpSplitLeadingTopLevelCommentSpans(source, start, end) {
		comment, ok := csharpRecoverTopLevelCommentNodeFromRange(source, span[0], span[1], lang, arena)
		if ok {
			comments = append(comments, comment)
		}
	}
	return comments
}

// csharpRecoverSourceTypeMembersFromRange recovers the members of a
// class/struct/record body directly from source (independent of whatever the
// earlier GLR sub-parse produced). Historically this was only ever reached
// through csharpRecoverTopLevelChunks, which gates on the WHOLE FILE being
// <= csharpMaxTopLevelChunkRecoverySourceBytes — so start/end were always a
// small span and any single member failing to recover could safely abort the
// whole type (strict, lenient=false).
//
// lenient=true is SHORT-TERM RELIEF for issue #136 (see
// csharpMaxTypeBodyRecoveryMembers): it is used when this is reached via a
// body-scoped caller (csharpRecoverNamespaceBodyMembersFromSource) that is
// NOT gated by the enclosing file's size, so start/end can span an entire
// real-world class body. In that mode an unrecoverable or oversized member is
// skipped instead of failing the whole type, so the declarations that do
// recover still surface — the same skip-don't-abort philosophy #116 already
// established for csharpRecoverTypeDeclarationBodyMembers.
func csharpRecoverSourceTypeMembersFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena, lenient bool) ([]*Node, bool) {
	if p == nil || p.language == nil || arena == nil || start > end || int(end) > len(source) {
		return nil, false
	}
	if bytesAreTrivia(source[start:end]) {
		return nil, true
	}
	relSpans := csharpTopLevelChunkSpans(source[start:end])
	if lenient && len(relSpans) > csharpMaxTypeBodyRecoveryMembers {
		return nil, false
	}
	out := make([]*Node, 0, len(relSpans))
	for _, rel := range relSpans {
		spanStart := start + rel[0]
		spanEnd := start + rel[1]
		for _, part := range csharpSplitLeadingTopLevelCommentSpans(source, spanStart, spanEnd) {
			if comment, ok := csharpRecoverTopLevelCommentNodeFromRange(source, part[0], part[1], p.language, arena); ok {
				out = append(out, comment)
				continue
			}
			if lenient && part[1]-part[0] > csharpMaxTopLevelChunkRecoverySourceBytes {
				// Bounded: skip an oversized single member instead of an
				// unbounded reparse (issue #136 short-term relief).
				continue
			}
			member, ok := csharpRecoverClassMethodDeclarationFromRange(source, part[0], part[1], p, arena)
			if !ok {
				member, ok = csharpRecoverClassMemberDeclarationFromRange(source, part[0], part[1], p, p.language, arena)
			}
			if !ok {
				if lenient {
					continue
				}
				return nil, false
			}
			out = append(out, member)
		}
	}
	return out, len(out) > 0 || !lenient
}

func csharpRecoverClassMethodDeclarationFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || arena == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = csharpTrimSpaceBounds(source, start, end)
	if start >= end || source[end-1] != '}' {
		return nil, false
	}
	openBrace := csharpFindTopLevelByte(source, start, end, '{')
	if openBrace >= end {
		return nil, false
	}
	closeBrace := csharpFindMatchingBraceByte(source, int(openBrace), int(end))
	if closeBrace < 0 || uint32(closeBrace+1) != end {
		return nil, false
	}
	const prefix = "class __Q { "
	const suffix = " }\n"
	wrapped := make([]byte, 0, len(prefix)+int(end-start)+len(suffix))
	wrapped = append(wrapped, prefix...)
	wrapped = append(wrapped, source[start:end]...)
	wrapped = append(wrapped, suffix...)
	bodyStart := uint32(len(prefix)) + (openBrace - start)
	bodyEnd := uint32(len(prefix)) + (uint32(closeBrace) - start)
	for i := bodyStart + 1; i < bodyEnd; i++ {
		wrapped[i] = ' '
	}
	tree, err := p.parseForRecovery(wrapped)
	if err != nil || tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		return nil, false
	}
	defer tree.Release()
	method := csharpExtractRecoveredWrappedClassMethod(tree.RootNode(), p.language, arena)
	if method == nil {
		return nil, false
	}
	if !shiftNodeBytes(method, int64(start)-int64(len(prefix))) {
		return nil, false
	}
	statements, ok := csharpRecoverMethodBlockStatementsFromRange(source, openBrace+1, uint32(closeBrace), p, arena)
	if !ok {
		return nil, false
	}
	block, ok := csharpBuildRecoveredMethodBlockNode(source, p.language, arena, openBrace, uint32(closeBrace), statements)
	if !ok {
		return nil, false
	}
	if !csharpReplaceMethodBlock(method, p.language, block) {
		return nil, false
	}
	recomputeNodePointsFromBytes(method, source)
	return method, true
}

// csharpRecoverClassMemberDeclarationFromRange is a generic, source-based
// recovery for a single class/struct/record member that csharpRecoverClass
// MethodDeclarationFromRange does not handle (it requires a "{...}" body): a
// field, auto-property, event field, or other ";"-terminated member. It
// wraps the member's raw source in a throwaway "class __Q { <member> }" and
// reparses just that snippet, then extracts whichever recognized type-body
// member node results (see csharpRecoverTypeDeclarationBodyChild for the
// accepted node types). Short-term relief for issue #136: callers bound the
// span size before calling this (csharpMaxTopLevelChunkRecoverySourceBytes
// per member in lenient mode), so worst-case reparse cost per member is
// predictable.
func csharpRecoverClassMemberDeclarationFromRange(source []byte, start, end uint32, p *Parser, lang *Language, arena *nodeArena) (*Node, bool) {
	if p == nil || lang == nil || arena == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	start, end = csharpTrimSpaceBounds(source, start, end)
	if start >= end {
		return nil, false
	}
	const prefix = "class __Q { "
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
	defer tree.Release()
	classDecl := csharpFindFirstNamedDescendantOfType(tree.RootNode(), lang, "class_declaration")
	if classDecl == nil {
		return nil, false
	}
	declList := csharpFindFirstNamedDescendantOfType(classDecl, lang, "declaration_list")
	if declList == nil {
		return nil, false
	}
	var member *Node
	for _, child := range declList.children {
		if child == nil {
			continue
		}
		if child.HasError() {
			continue
		}
		if candidate, ok := csharpRecoverTypeDeclarationBodyChild(child, lang, arena); ok {
			member = candidate
			break
		}
	}
	if member == nil {
		return nil, false
	}
	if !shiftNodeBytes(member, int64(start)-int64(len(prefix))) {
		return nil, false
	}
	recomputeNodePointsFromBytes(member, source)
	return member, true
}

func csharpBuildSourceDeclarationListNode(source []byte, openBrace, closeBrace uint32, members []*Node, lang *Language, arena *nodeArena) (*Node, bool) {
	if lang == nil || arena == nil || openBrace >= closeBrace || int(closeBrace) >= len(source) {
		return nil, false
	}
	sym, ok := symbolByName(lang, "declaration_list")
	if !ok {
		return nil, false
	}
	openTok, ok := csharpBuildLeafNodeByName(arena, source, lang, "{", openBrace, openBrace+1)
	if !ok {
		return nil, false
	}
	closeTok, ok := csharpBuildLeafNodeByName(arena, source, lang, "}", closeBrace, closeBrace+1)
	if !ok {
		return nil, false
	}
	children := make([]*Node, 0, len(members)+2)
	children = append(children, openTok)
	children = append(children, members...)
	children = append(children, closeTok)
	buf := arena.allocNodeSlice(len(children))
	copy(buf, children)
	named := symbolIsNamed(lang, sym)
	return newParentNodeInArena(arena, sym, named, buf, nil, 0), true
}

func csharpRecoverNonEmptyTypeDeclarationFromError(n *Node, source []byte, p *Parser, lang *Language, arena *nodeArena, lenient bool) (*Node, bool) {
	if n == nil || lang == nil || arena == nil || n.Type(lang) != "ERROR" || len(n.children) == 0 {
		return nil, false
	}
	return csharpRecoverNonEmptyTypeDeclarationFromChildSlice(n.children, 0, source, p, lang, arena, lenient)
}

func csharpRecoverNonEmptyTopLevelTypeDeclarationFromChildren(children []*Node, startIdx int, source []byte, p *Parser, lang *Language, arena *nodeArena, lenient bool) (*Node, int, bool) {
	if startIdx < 0 || startIdx >= len(children) || lang == nil || arena == nil {
		return nil, startIdx, false
	}
	recovered, ok := csharpRecoverNonEmptyTypeDeclarationFromChildSlice(children[startIdx:], 0, source, p, lang, arena, lenient)
	if !ok || recovered == nil {
		return nil, startIdx, false
	}
	nextIdx := startIdx + 1
	for nextIdx < len(children) {
		child := children[nextIdx]
		if child == nil {
			nextIdx++
			continue
		}
		if child.startByte >= recovered.endByte {
			break
		}
		nextIdx++
	}
	return recovered, nextIdx, true
}

func csharpRecoverNonEmptyTypeDeclarationFromChildSlice(children []*Node, startIdx int, source []byte, p *Parser, lang *Language, arena *nodeArena, lenient bool) (*Node, bool) {
	if startIdx < 0 || startIdx >= len(children) || lang == nil || arena == nil {
		return nil, false
	}
	type recoverySpec struct {
		initName string
		declName string
	}
	specs := []recoverySpec{
		{initName: "_class_declaration_initializer", declName: "class_declaration"},
		{initName: "_struct_declaration_initializer", declName: "struct_declaration"},
		{initName: "_record_declaration_initializer", declName: "record_declaration"},
	}
	for _, spec := range specs {
		for _, child := range children[startIdx:] {
			if child == nil || child.Type(lang) != spec.initName {
				continue
			}
			if recovered, ok := csharpBuildRecoveredTypeDeclarationWithBodyFromChildren(children[startIdx:], child, source, p, lang, arena, spec.declName, lenient); ok {
				return recovered, true
			}
		}
	}
	return nil, false
}

func csharpBuildRecoveredTypeDeclarationWithBodyFromChildren(children []*Node, initNode *Node, source []byte, p *Parser, lang *Language, arena *nodeArena, declName string, lenient bool) (*Node, bool) {
	if initNode == nil || lang == nil || arena == nil || int(initNode.endByte) > len(source) {
		return nil, false
	}
	openRel := bytes.IndexByte(source[initNode.endByte:], '{')
	if openRel < 0 {
		return nil, false
	}
	openBrace := int(initNode.endByte) + openRel
	closeBrace := csharpFindMatchingBraceByte(source, openBrace, len(source))
	if closeBrace < 0 || closeBrace <= openBrace {
		return nil, false
	}
	openTok, ok := csharpBuildLeafNodeByName(arena, source, lang, "{", uint32(openBrace), uint32(openBrace+1))
	if !ok {
		return nil, false
	}
	closeTok, ok := csharpBuildLeafNodeByName(arena, source, lang, "}", uint32(closeBrace), uint32(closeBrace+1))
	if !ok {
		return nil, false
	}
	members, ok := csharpRecoverTypeDeclarationBodyMembers(children, initNode, source, p, lang, arena, uint32(openBrace), uint32(closeBrace), lenient)
	if !ok || len(members) == 0 {
		return nil, false
	}
	bodyChildren := make([]*Node, 0, len(members)+2)
	bodyChildren = append(bodyChildren, openTok)
	bodyChildren = append(bodyChildren, members...)
	bodyChildren = append(bodyChildren, closeTok)
	declListSym, ok := symbolByName(lang, "declaration_list")
	if !ok {
		return nil, false
	}
	declListNamed := symbolIsNamed(lang, declListSym)
	if arena != nil {
		buf := arena.allocNodeSlice(len(bodyChildren))
		copy(buf, bodyChildren)
		bodyChildren = buf
	}
	declList := newParentNodeInArena(arena, declListSym, declListNamed, bodyChildren, nil, 0)
	declSym, ok := symbolByName(lang, declName)
	if !ok {
		return nil, false
	}
	declNamed := symbolIsNamed(lang, declSym)
	declChildren := make([]*Node, 0, len(initNode.children)+1)
	for _, child := range initNode.children {
		if child != nil {
			declChildren = append(declChildren, cloneTreeNodesIntoArena(child, arena))
		}
	}
	declChildren = append(declChildren, declList)
	if arena != nil {
		buf := arena.allocNodeSlice(len(declChildren))
		copy(buf, declChildren)
		declChildren = buf
	}
	recovered := newParentNodeInArena(arena, declSym, declNamed, declChildren, nil, 0)
	recovered.setHasError(false)
	extendNodeEndTo(recovered, uint32(closeBrace+1), source)
	return recovered, true
}

func csharpRecoverTypeDeclarationBodyMembers(children []*Node, initNode *Node, source []byte, p *Parser, lang *Language, arena *nodeArena, openBrace, closeBrace uint32, lenient bool) ([]*Node, bool) {
	if lang == nil || arena == nil || openBrace >= closeBrace {
		return nil, false
	}
	members := make([]*Node, 0, len(children))
	for i := 0; i < len(children); {
		child := children[i]
		if child == nil || child == initNode || child.endByte <= openBrace+1 || child.startByte >= closeBrace {
			i++
			continue
		}
		if recovered, next, ok := csharpRecoverMethodDeclarationFromChildren(children, i, source, p, lang, arena, closeBrace); ok {
			members = append(members, recovered)
			i = next
			continue
		}
		if child.Type(lang) == "ERROR" {
			if child.startByte <= openBrace && child.endByte <= openBrace+1 {
				i++
				continue
			}
			// In lenient mode (namespace-with-internal-errors recovery), an
			// unrecoverable ERROR fragment is skipped rather than failing the
			// whole type declaration, so the surrounding class_declaration and
			// the members that did parse remain in the tree (issue #115).
			if lenient {
				i++
				continue
			}
			return nil, false
		}
		member, ok := csharpRecoverTypeDeclarationBodyChild(child, lang, arena)
		if !ok {
			if lenient {
				i++
				continue
			}
			return nil, false
		}
		members = append(members, member)
		i++
	}
	return members, len(members) > 0
}

func csharpRecoverTypeDeclarationBodyChild(n *Node, lang *Language, arena *nodeArena) (*Node, bool) {
	if n == nil || lang == nil || arena == nil {
		return nil, false
	}
	if n.Type(lang) == "declaration" && len(n.children) == 1 && n.children[0] != nil {
		n = n.children[0]
	}
	switch n.Type(lang) {
	case "class_declaration",
		"struct_declaration",
		"record_declaration",
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
		"indexer_declaration",
		"operator_declaration",
		"conversion_operator_declaration",
		"comment":
		return cloneTreeNodesIntoArena(n, arena), true
	default:
		return nil, false
	}
}

func csharpRecoverMethodDeclarationFromChildren(children []*Node, startIdx int, source []byte, p *Parser, lang *Language, arena *nodeArena, enclosingClose uint32) (*Node, int, bool) {
	if p == nil || lang == nil || arena == nil || startIdx < 0 || startIdx >= len(children) || int(enclosingClose) > len(source) {
		return nil, startIdx, false
	}
	header, params, nextIdx, ok := csharpRecoverMethodHeaderFromChildren(children, startIdx, lang, enclosingClose)
	if !ok {
		return nil, startIdx, false
	}

	openBracePos := int(csharpSkipSpaceBytes(source, params.endByte))
	if openBracePos >= int(enclosingClose) || source[openBracePos] != '{' {
		return nil, startIdx, false
	}
	closeBracePos := csharpFindMatchingBraceByte(source, openBracePos, int(enclosingClose))
	if closeBracePos < 0 || closeBracePos <= openBracePos {
		return nil, startIdx, false
	}

	statements, nextIdx, ok := csharpRecoverMethodBodyStatementsFromChildren(children, nextIdx, source, p, lang, arena, openBracePos, closeBracePos)
	if !ok {
		return nil, startIdx, false
	}
	block, ok := csharpBuildRecoveredMethodBlockNode(source, lang, arena, uint32(openBracePos), uint32(closeBracePos), statements)
	if !ok {
		return nil, startIdx, false
	}
	method, ok := csharpBuildRecoveredMethodDeclarationNode(header, block, source, lang, arena, closeBracePos)
	if !ok {
		return nil, startIdx, false
	}
	nextIdx = csharpSkipMethodChildrenBefore(children, nextIdx, uint32(closeBracePos+1))
	return method, nextIdx, true
}

func csharpRecoverMethodHeaderFromChildren(children []*Node, startIdx int, lang *Language, enclosingClose uint32) ([]*Node, *Node, int, bool) {
	i := startIdx
	header := make([]*Node, 0, 4)
	for i < len(children) {
		child := children[i]
		if child == nil || child.startByte >= enclosingClose {
			break
		}
		if child.Type(lang) != "modifier" {
			break
		}
		header = append(header, child)
		i++
	}

	returnType, i, ok := csharpRequiredChildByType(children, i, lang, "type")
	if !ok {
		return nil, nil, startIdx, false
	}
	if len(returnType.children) == 1 && returnType.children[0] != nil {
		returnType = returnType.children[0]
	}
	header = append(header, returnType)

	name, i, ok := csharpRequiredChildByType(children, i, lang, "identifier")
	if !ok {
		return nil, nil, startIdx, false
	}
	header = append(header, name)

	params, i, ok := csharpRequiredChildByType(children, i, lang, "parameter_list")
	if !ok {
		return nil, nil, startIdx, false
	}
	header = append(header, params)
	return header, params, i, true
}

func csharpRequiredChildByType(children []*Node, idx int, lang *Language, typ string) (*Node, int, bool) {
	if idx >= len(children) || children[idx] == nil || children[idx].Type(lang) != typ {
		return nil, idx, false
	}
	return children[idx], idx + 1, true
}

func csharpRecoverMethodBodyStatementsFromChildren(children []*Node, startIdx int, source []byte, p *Parser, lang *Language, arena *nodeArena, openBracePos, closeBracePos int) ([]*Node, int, bool) {
	statements := make([]*Node, 0, 8)
	nextIdx := startIdx
	needSourceStatementRecovery := false
	for nextIdx < len(children) {
		child := children[nextIdx]
		if child == nil {
			nextIdx++
			continue
		}
		if child.startByte >= uint32(closeBracePos+1) {
			break
		}
		if child.endByte <= uint32(openBracePos+1) {
			nextIdx++
			continue
		}
		recovered, ok := csharpRecoverMethodBlockStatementsFromNode(child, lang, arena)
		if ok {
			statements = append(statements, recovered...)
			if csharpStatementsNeedSourceRecovery(recovered, source, lang) {
				needSourceStatementRecovery = true
			}
		} else if !bytesAreTrivia(source[child.startByte:child.endByte]) {
			needSourceStatementRecovery = true
		}
		nextIdx++
	}
	if len(source) <= csharpMaxTopLevelChunkRecoverySourceBytes &&
		(needSourceStatementRecovery || len(statements) == 0 && !bytesAreTrivia(source[openBracePos+1:closeBracePos])) {
		recoveredStatements, ok := csharpRecoverMethodBlockStatementsFromRange(source, uint32(openBracePos+1), uint32(closeBracePos), p, arena)
		if !ok {
			return nil, startIdx, false
		}
		statements = recoveredStatements
	}
	if len(statements) == 0 && !bytesAreTrivia(source[openBracePos+1:closeBracePos]) {
		return nil, nextIdx, false
	}
	return statements, nextIdx, true
}

func csharpBuildRecoveredMethodDeclarationNode(header []*Node, block *Node, source []byte, lang *Language, arena *nodeArena, closeBracePos int) (*Node, bool) {
	methodSym, ok := symbolByName(lang, "method_declaration")
	if !ok {
		return nil, false
	}
	methodNamed := symbolIsNamed(lang, methodSym)
	methodChildren := make([]*Node, 0, len(header)+1)
	for _, child := range header {
		if child != nil {
			methodChildren = append(methodChildren, cloneTreeNodesIntoArena(child, arena))
		}
	}
	methodChildren = append(methodChildren, block)
	if arena != nil {
		buf := arena.allocNodeSlice(len(methodChildren))
		copy(buf, methodChildren)
		methodChildren = buf
	}
	method := newParentNodeInArena(arena, methodSym, methodNamed, methodChildren, nil, 0)
	method.setHasError(false)
	extendNodeEndTo(method, uint32(closeBracePos+1), source)
	return method, true
}

func csharpSkipMethodChildrenBefore(children []*Node, nextIdx int, end uint32) int {
	for nextIdx < len(children) {
		child := children[nextIdx]
		if child == nil || child.startByte >= end {
			break
		}
		nextIdx++
	}
	return nextIdx
}

func csharpRecoverMethodBlockStatementsFromNode(n *Node, lang *Language, arena *nodeArena) ([]*Node, bool) {
	if n == nil || lang == nil || arena == nil {
		return nil, false
	}
	if n.Type(lang) == "statement" {
		if len(n.children) == 1 && n.children[0] != nil {
			return csharpRecoverMethodBlockStatementsFromNode(n.children[0], lang, arena)
		}
	}
	if csharpIsRecoveredMethodBlockStatement(n, lang) {
		return []*Node{cloneTreeNodesIntoArena(n, arena)}, true
	}
	if strings.HasPrefix(n.Type(lang), "block_repeat") {
		out := make([]*Node, 0, len(n.children))
		for _, child := range n.children {
			recovered, ok := csharpRecoverMethodBlockStatementsFromNode(child, lang, arena)
			if ok {
				out = append(out, recovered...)
			}
		}
		return out, len(out) > 0
	}
	return nil, false
}

func csharpIsRecoveredMethodBlockStatement(n *Node, lang *Language) bool {
	if n == nil || lang == nil {
		return false
	}
	typ := n.Type(lang)
	return typ == "comment" || typ == "local_function_statement" || strings.HasSuffix(typ, "_statement")
}

func csharpBuildRecoveredMethodBlockNode(source []byte, lang *Language, arena *nodeArena, openBrace, closeBrace uint32, statements []*Node) (*Node, bool) {
	if lang == nil || openBrace >= closeBrace || int(closeBrace+1) > len(source) {
		return nil, false
	}
	blockSym, ok := symbolByName(lang, "block")
	if !ok {
		return nil, false
	}
	blockNamed := symbolIsNamed(lang, blockSym)
	openTok, ok := csharpBuildLeafNodeByName(arena, source, lang, "{", openBrace, openBrace+1)
	if !ok {
		return nil, false
	}
	closeTok, ok := csharpBuildLeafNodeByName(arena, source, lang, "}", closeBrace, closeBrace+1)
	if !ok {
		return nil, false
	}
	children := make([]*Node, 0, len(statements)+2)
	children = append(children, openTok)
	children = append(children, statements...)
	children = append(children, closeTok)
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	block := newParentNodeInArena(arena, blockSym, blockNamed, children, nil, 0)
	block.setHasError(false)
	return block, true
}
