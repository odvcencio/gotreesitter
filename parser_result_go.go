package gotreesitter

import "bytes"

// normalizeGoReturnedTreeCompatibility drives Go's post-build compatibility
// pipeline. Every stage boundary already re-checked p.activeParseStopReason()
// (Timeout/Cancelled); it now also re-checks the memory budget via
// goCompatMemoryBudgetStopReason so a parse that is already over budget (the
// parse loop stopped, or the arena/runtime heap grew past budget during an
// earlier stage) skips the remaining Go compat work entirely instead of
// normalizing a partial tree that carries no C-faithful normalization
// guarantee. This mirrors the existing timeout/cancellation semantics at the
// same call sites (see the 2026-07-12 gocompat-walk-containment-gap finding).
func normalizeGoReturnedTreeCompatibility(root *Node, source []byte, p *Parser, lang *Language) ParseStopReason {
	var arena *nodeArena
	if root != nil {
		arena = root.ownerArena
	}
	if reason := p.activeParseStopReason(); parseStopReasonIsActive(reason) {
		return reason
	}
	if reason := p.goCompatMemoryBudgetStopReason(arena); reason == ParseStopMemoryBudget {
		return reason
	}
	normalizeGoSourceFileRoot(root, source, p)
	if reason := p.activeParseStopReason(); parseStopReasonIsActive(reason) {
		return reason
	}
	if reason := p.goCompatMemoryBudgetStopReason(arena); reason == ParseStopMemoryBudget {
		return reason
	}
	if reason := normalizeGoCompatibilityWithParser(root, source, lang, p); resultMaterializationShouldStop(reason) {
		return reason
	}
	if reason := p.activeParseStopReason(); parseStopReasonIsActive(reason) {
		return reason
	}
	if reason := p.goCompatMemoryBudgetStopReason(arena); reason == ParseStopMemoryBudget {
		return reason
	}
	normalizeRootEOFNewlineSpan(root, source, lang)
	if reason := p.activeParseStopReason(); parseStopReasonIsActive(reason) {
		return reason
	}
	if reason := p.goCompatMemoryBudgetStopReason(arena); reason == ParseStopMemoryBudget {
		return reason
	}
	normalizeGoNewMakeTypeArgument(root, source, lang)
	return p.goCompatMemoryBudgetStopReason(arena)
}

// normalizeGoNewMakeTypeArgument reclassifies the leading argument of a call
// to the "new" or "make" builtins from `identifier` to `type_identifier` when
// it is a bare identifier (e.g. `new(dirInfo)`).
//
// Root cause: tree-sitter-go's grammar declares `[$._simple_type,
// $._expression]` as an explicit ambiguity (grammar.js conflicts array) for
// exactly this position — special_argument_list's leading `_type` slot
// (grammargen/go_grammar.go's special_argument_list/call_expression, mirrored
// from grammar.js's call_expression/special_argument_list rules) overlaps
// with argument_list's `_expression` slot for a single bare identifier
// followed by "," or ")". The real tree-sitter table generator resolves this
// reduce/reduce conflict *statically*, at table-build time, in favor of the
// earlier-declared `_simple_type` production — confirmed empirically via the
// real C runtime's own parse logger (ts_parser_set_logger), which shows
// version_count staying at 1 (no GLR fork at all) and a single deterministic
// "reduce sym:_simple_type" for this state. gotreesitter's LR table instead
// treats this as a genuine two-way GLR fork (see AmbiguityProfile: state 186,
// lookahead ")", two reduce actions — _simple_type dynPrec=-1 vs _expression
// dynPrec=0) and its runtime merge picks the higher-dynamic-precedence
// alternative (_expression), which is the opposite of what the real offline
// table-build-time resolution picks. That is a genuine LALR/GLR
// table-generation divergence in grammargen's conflict resolution — not a
// per-language quirk — but replicating the real compiler's "earlier
// declaration wins" static tie-break generally is a broad, cross-grammar
// change to grammargen/lr.go's resolveReduceReduceLegacy family (used by
// every language with declared conflicts) with real blast-radius risk.
//
// This function is the narrow, local, byte-identity-preserving patch: it
// only retags the node's symbol/named-ness (never bytes, never structure) and
// only for the one syntactic shape the owner-confirmed divergence covers —
// the sole/leading argument of a literal `new`/`make` call. It mirrors the
// existing precedent for this class of fix (see
// normalizeArduinoBuiltinPrimitiveTypes in parser_result_c.go).
func normalizeGoNewMakeTypeArgument(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || len(source) == 0 {
		return
	}
	if !bytes.Contains(source, []byte("new")) && !bytes.Contains(source, []byte("make")) {
		return
	}
	callSym, ok := symbolByName(lang, "call_expression")
	if !ok {
		return
	}
	argListSym, ok := symbolByName(lang, "argument_list")
	if !ok {
		return
	}
	identifierSym, ok := lang.symbolByNamePreferNamed("identifier")
	if !ok {
		return
	}
	typeIdentifierSym, ok := lang.symbolByNamePreferNamed("type_identifier")
	if !ok {
		return
	}
	typeIdentifierNamed := symbolIsNamed(lang, typeIdentifierSym)

	walkResultTree(root, func(n *Node) {
		if n == nil || n.symbol != callSym {
			return
		}
		fn := n.ChildByFieldName("function", lang)
		if fn == nil || fn.symbol != identifierSym {
			return
		}
		name := fn.Text(source)
		if name != "new" && name != "make" {
			return
		}
		args := n.ChildByFieldName("arguments", lang)
		if args == nil || args.symbol != argListSym {
			return
		}
		first := args.NamedChild(0)
		if first == nil || first.symbol != identifierSym {
			return
		}
		first.symbol = typeIdentifierSym
		first.setNamed(typeIdentifierNamed)
	})
}

// goCompatMemoryBudgetStopReason forces a real (unmasked) memory-budget check
// for the Go compat pipeline's stage boundaries in
// normalizeGoReturnedTreeCompatibility. It is called a small, fixed number of
// times per parse (never per node), so — unlike the walk-internal poll, which
// intentionally samples at a coarse stride to stay cheap — it always reads
// current arena/runtime state directly rather than relying on
// resultMaterializationStopReason's shared, throttled poll counter, which can
// otherwise miss an already-tripped budget at these infrequent checkpoints.
func (p *Parser) goCompatMemoryBudgetStopReason(arena *nodeArena) ParseStopReason {
	if p == nil {
		return ParseStopNone
	}
	if arena != nil && arena.budgetExhausted() {
		return p.noteMemoryBudgetStop(parseMemoryBudgetStopSourceArena)
	}
	return p.compatRuntimeMemoryBudgetStopReason()
}

func normalizeGoSourceFileRoot(root *Node, source []byte, p *Parser) {
	if root == nil || p == nil || p.language == nil || p.language.Name != "go" || root.Type(p.language) != "ERROR" {
		return
	}
	lang := p.language
	sym, ok := symbolByName(lang, "source_file")
	if !ok {
		return
	}
	if !rootLooksLikeGoTopLevel(root, lang) {
		recoverGoRootTopLevelChunks(root, source, p)
	}
	if !rootLooksLikeGoTopLevel(root, lang) {
		return
	}
	retagResultRootAndRefreshError(root, sym, symbolIsNamed(lang, sym))
	if root.endByte < uint32(len(source)) && bytesAreTrivia(source[root.endByte:]) {
		extendNodeEndTo(root, uint32(len(source)), source)
	}
}

func rootLooksLikeGoTopLevel(root *Node, lang *Language) bool {
	if root == nil || lang == nil || resultChildCount(root) == 0 {
		return false
	}
	sawTopLevel := false
	for i := 0; i < resultChildCount(root); i++ {
		child := resultChildAt(root, i)
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "package_clause",
			"import_declaration",
			"function_declaration",
			"method_declaration",
			"const_declaration",
			"type_declaration",
			"var_declaration",
			"comment":
			sawTopLevel = true
		default:
			return false
		}
	}
	return sawTopLevel
}

func recoverGoRootTopLevelChunks(root *Node, source []byte, p *Parser) {
	if root == nil || p == nil || p.language == nil || p.skipRecoveryReparse || len(source) == 0 || resultChildCount(root) == 0 {
		return
	}
	firstBad := firstGoNonTopLevelChildIndex(root, p.language)
	if firstBad <= 0 {
		return
	}
	start := goRootRecoveryStartByte(resultChildAt(root, firstBad), source)
	if int(start) >= len(source) {
		return
	}
	recovered, ok := goReparsedTopLevelChunks(source, start, p, root.ownerArena)
	if !ok {
		return
	}
	children := resultChildSliceForMutation(root)
	keepPrefix := goRecoveredTopLevelPrefixLen(children, firstBad, recovered, p.language)
	newChildren := make([]*Node, 0, keepPrefix+len(recovered))
	newChildren = append(newChildren, children[:keepPrefix]...)
	newChildren = append(newChildren, recovered...)
	if !goChildrenLookLikeTopLevel(newChildren, p.language) {
		return
	}
	if arena := root.ownerArena; arena != nil {
		buf := arena.allocNodeSlice(len(newChildren))
		copy(buf, newChildren)
		newChildren = buf
	}
	replaceNodeChildrenUnfielded(root, newChildren)
}

func goRecoveredTopLevelPrefixLen(children []*Node, firstBad int, recovered []*Node, lang *Language) int {
	if firstBad <= 0 || firstBad > len(children) || len(recovered) == 0 || lang == nil {
		return firstBad
	}
	prev := children[firstBad-1]
	first := recovered[0]
	if prev == nil || first == nil || prev.startByte != first.startByte || prev.endByte >= first.endByte {
		return firstBad
	}
	switch prev.Type(lang) {
	case "function_declaration", "method_declaration":
		return firstBad - 1
	default:
		return firstBad
	}
}

func firstGoNonTopLevelChildIndex(root *Node, lang *Language) int {
	if root == nil || lang == nil {
		return -1
	}
	for i := 0; i < resultChildCount(root); i++ {
		child := resultChildAt(root, i)
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "package_clause",
			"import_declaration",
			"function_declaration",
			"method_declaration",
			"const_declaration",
			"type_declaration",
			"var_declaration",
			"comment":
			continue
		default:
			return i
		}
	}
	return -1
}

func goChildrenLookLikeTopLevel(children []*Node, lang *Language) bool {
	root := &Node{children: children}
	return rootLooksLikeGoTopLevel(root, lang)
}

func goRootRecoveryStartByte(node *Node, source []byte) uint32 {
	if node == nil {
		return uint32(len(source))
	}
	start := node.startByte
	for start > 0 && source[start-1] != '\n' {
		start--
	}
	return start
}

func goReparsedTopLevelChunks(source []byte, start uint32, p *Parser, arena *nodeArena) ([]*Node, bool) {
	if p == nil || p.language == nil || int(start) >= len(source) {
		return nil, false
	}
	const prefix = "package p\n"
	prefixPoint := advancePointByBytes(Point{}, []byte(prefix))
	chunkStarts := goTopLevelChunkStarts(source, start)
	if len(chunkStarts) == 0 {
		return nil, false
	}
	recovered := make([]*Node, 0, len(chunkStarts))
	for i, chunkStart := range chunkStarts {
		chunkEnd := uint32(len(source))
		if i+1 < len(chunkStarts) {
			chunkEnd = chunkStarts[i+1]
		}
		if chunkStart >= chunkEnd {
			continue
		}
		wrapped := make([]byte, 0, len(prefix)+int(chunkEnd-chunkStart))
		wrapped = append(wrapped, prefix...)
		wrapped = append(wrapped, source[chunkStart:chunkEnd]...)
		tree, err := p.parseForRecovery(wrapped)
		if err != nil || tree == nil || tree.RootNode() == nil {
			if tree != nil {
				tree.Release()
			}
			return nil, false
		}
		if tree.RootNode().HasError() {
			tree.Release()
			recoveredNode, ok := goRecoverWrappedFunctionChunk(source, chunkStart, chunkEnd, p, arena)
			if !ok {
				return nil, false
			}
			recovered = append(recovered, recoveredNode)
			continue
		}
		startPoint := advancePointByBytes(Point{}, source[:chunkStart])
		if startPoint.Row < prefixPoint.Row {
			tree.Release()
			return nil, false
		}
		offsetRoot := tree.RootNodeWithOffset(
			chunkStart-uint32(len(prefix)),
			Point{Row: startPoint.Row - prefixPoint.Row, Column: startPoint.Column},
		)
		tree.Release()
		if offsetRoot == nil {
			return nil, false
		}
		var added int
		for j := 0; j < offsetRoot.NamedChildCount(); j++ {
			child := offsetRoot.NamedChild(j)
			if child == nil || child.Type(p.language) == "package_clause" {
				continue
			}
			recovered = append(recovered, cloneTreeNodesIntoArena(child, arena))
			added++
		}
		if added == 0 {
			return nil, false
		}
	}
	return recovered, len(recovered) > 0
}

func goRecoverWrappedFunctionChunk(source []byte, chunkStart, chunkEnd uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || len(source) == 0 || chunkStart >= chunkEnd || int(chunkEnd) > len(source) {
		return nil, false
	}
	const prefix = "package p\n"
	wrapped := make([]byte, 0, len(prefix)+int(chunkEnd-chunkStart))
	wrapped = append(wrapped, prefix...)
	wrapped = append(wrapped, source[chunkStart:chunkEnd]...)
	funcStart := len(prefix)
	openBrace := bytes.IndexByte(wrapped[funcStart:], '{')
	if openBrace < 0 {
		return nil, false
	}
	openBrace += funcStart
	closeBrace := findMatchingBraceByte(wrapped, openBrace, len(wrapped))
	if closeBrace < 0 || closeBrace <= openBrace {
		return nil, false
	}

	skeleton := make([]byte, 0, openBrace+4)
	skeleton = append(skeleton, wrapped[:openBrace]...)
	skeleton = append(skeleton, '{', '}', '\n')
	tree, err := p.parseForRecovery(skeleton)
	if err != nil || tree == nil || tree.RootNode() == nil || tree.RootNode().HasError() {
		if tree != nil {
			tree.Release()
		}
		return nil, false
	}
	defer tree.Release()

	startPoint := advancePointByBytes(Point{}, source[:chunkStart])
	prefixPoint := advancePointByBytes(Point{}, []byte(prefix))
	if startPoint.Row < prefixPoint.Row {
		return nil, false
	}
	offsetRoot := tree.RootNodeWithOffset(
		chunkStart-uint32(len(prefix)),
		Point{Row: startPoint.Row - prefixPoint.Row, Column: startPoint.Column},
	)
	if offsetRoot == nil {
		return nil, false
	}

	fn := goFirstFunctionLikeChild(offsetRoot, p.language)
	if fn == nil || fn.ChildCount() < 4 {
		return nil, false
	}
	openBraceAbs := chunkStart + uint32(openBrace-len(prefix))
	closeBraceAbs := chunkStart + uint32(closeBrace-len(prefix))
	bodyNodes, ok := goRecoverFunctionBodyNodes(source, openBraceAbs+1, closeBraceAbs, p, arena)
	if !ok {
		return nil, false
	}
	recoveredFn := cloneTreeNodesIntoArena(fn, arena)
	block, ok := goBuildRecoveredBlockNode(source, openBraceAbs, closeBraceAbs, bodyNodes, arena, p.language)
	if !ok {
		return nil, false
	}
	recoveredFn.children[len(recoveredFn.children)-1] = block
	block.parent = recoveredFn
	block.childIndex = int32(len(recoveredFn.children) - 1)
	populateParentNode(recoveredFn, recoveredFn.children)
	return recoveredFn, true
}

func goRecoverFunctionBodyNodes(source []byte, start, end uint32, p *Parser, arena *nodeArena) ([]*Node, bool) {
	if int(start) >= len(source) || start >= end {
		return nil, false
	}
	ranges := goFunctionStatementRanges(source, start, end)
	if len(ranges) == 0 {
		return nil, true
	}
	out := make([]*Node, 0, len(ranges))
	for _, r := range ranges {
		nodes, ok := goRecoverStatementNodesFromRange(source, r[0], r[1], p, arena)
		if !ok {
			return nil, false
		}
		out = append(out, nodes...)
	}
	return out, true
}

func goRecoverStatementNodesFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) ([]*Node, bool) {
	if start >= end {
		return nil, true
	}
	const prefix = "package p\nfunc _() {\n"
	stmt := source[start:end]
	wrapped := make([]byte, 0, len(prefix)+len(stmt)+4)
	wrapped = append(wrapped, prefix...)
	wrapped = append(wrapped, stmt...)
	wrapped = append(wrapped, '\n', '}', '\n')
	tree, err := p.parseForRecovery(wrapped)
	if err == nil && tree != nil && tree.RootNode() != nil {
		startPoint := advancePointByBytes(Point{}, source[:start])
		prefixPoint := advancePointByBytes(Point{}, []byte(prefix))
		if startPoint.Row >= prefixPoint.Row {
			offsetRoot := tree.RootNodeWithOffset(start-uint32(len(prefix)), Point{Row: startPoint.Row - prefixPoint.Row, Column: startPoint.Column})
			if offsetRoot != nil {
				if !offsetRoot.HasError() {
					nodes := goExtractRecoveredStatementNodes(offsetRoot, source, p.language, arena)
					tree.Release()
					if len(nodes) > 0 {
						return nodes, true
					}
				}
				if node := goExtractSingleRecoveredStatement(offsetRoot, source, p.language, arena); node != nil {
					tree.Release()
					return []*Node{node}, true
				}
			}
		}
		tree.Release()
	}
	if node, ok := goRecoverIfStatementFromRange(source, start, end, p, arena); ok {
		return []*Node{node}, true
	}
	if node, ok := goRecoverForStatementFromRange(source, start, end, p, arena); ok {
		return []*Node{node}, true
	}
	return nil, false
}

func goRecoverForStatementFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	trimmedStart, trimmedEnd, ok := trimGoSourceRange(source, start, end)
	if !ok || !bytes.HasPrefix(source[trimmedStart:trimmedEnd], []byte("for")) {
		return nil, false
	}
	if trimmedStart+3 < trimmedEnd {
		switch source[trimmedStart+3] {
		case ' ', '\t', '\r', '\n', '{':
		default:
			return nil, false
		}
	}
	openBraceAbs, closeBraceAbs, forNode, releaseForHeader, ok := findGoForStatementBlock(source, trimmedStart, trimmedEnd, p)
	if !ok {
		return nil, false
	}
	defer releaseForHeader()
	bodyNodes, ok := goRecoverFunctionBodyNodes(source, openBraceAbs+1, closeBraceAbs, p, arena)
	if !ok {
		return nil, false
	}
	block, ok := goBuildRecoveredBlockNode(source, openBraceAbs, closeBraceAbs, bodyNodes, arena, p.language)
	if !ok {
		return nil, false
	}
	recoveredFor := cloneTreeNodesIntoArena(forNode, arena)
	blockIndex := -1
	for i := recoveredFor.ChildCount() - 1; i >= 0; i-- {
		if child := recoveredFor.Child(i); child != nil && child.Type(p.language) == "block" {
			blockIndex = i
			break
		}
	}
	if blockIndex < 0 {
		return nil, false
	}
	recoveredFor.children[blockIndex] = block
	block.parent = recoveredFor
	block.childIndex = int32(blockIndex)
	populateParentNode(recoveredFor, recoveredFor.children)
	return recoveredFor, true
}

func findGoForStatementBlock(source []byte, start, end uint32, p *Parser) (uint32, uint32, *Node, func(), bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return 0, 0, nil, nil, false
	}
	candidates, complete := goStatementOpeningBraceCandidates(source, start, end)
	if !complete {
		return 0, 0, nil, nil, false
	}
	for _, openBraceAbs := range candidates {
		closeBraceAbs, ok := findMatchingGoBraceByte(source, openBraceAbs, end)
		if !ok || closeBraceAbs <= openBraceAbs {
			return 0, 0, nil, nil, false
		}
		if !goForStatementHasOnlyTrailingTrivia(source, closeBraceAbs+1, end) {
			continue
		}
		forNode, release, ok := parseGoForHeaderSkeleton(source, start, openBraceAbs, p)
		if !ok {
			continue
		}
		return openBraceAbs, closeBraceAbs, forNode, release, true
	}
	return 0, 0, nil, nil, false
}

func goStatementOpeningBraceCandidates(source []byte, start, end uint32) ([]uint32, bool) {
	var (
		candidates     []uint32
		parenDepth     int
		bracketDepth   int
		inLineComment  bool
		inBlockComment bool
		inString       bool
		inRune         bool
		inRawString    bool
		escape         bool
	)
	for i := int(start); i < int(end); i++ {
		b := source[i]
		if inLineComment {
			if b == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if b == '*' && i+1 < int(end) && source[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if inString {
			if escape {
				escape = false
				continue
			}
			if b == '\\' {
				escape = true
				continue
			}
			if b == '"' {
				inString = false
			}
			continue
		}
		if inRune {
			if escape {
				escape = false
				continue
			}
			if b == '\\' {
				escape = true
				continue
			}
			if b == '\'' {
				inRune = false
			}
			continue
		}
		if inRawString {
			if b == '`' {
				inRawString = false
			}
			continue
		}
		switch b {
		case '/':
			if i+1 < int(end) && source[i+1] == '/' {
				inLineComment = true
				i++
				continue
			}
			if i+1 < int(end) && source[i+1] == '*' {
				inBlockComment = true
				i++
				continue
			}
		case '"':
			inString = true
		case '\'':
			inRune = true
		case '`':
			inRawString = true
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
		case '{':
			if parenDepth == 0 && bracketDepth == 0 {
				candidates = append(candidates, uint32(i))
			}
		}
	}
	return candidates, !inBlockComment && !inString && !inRune && !inRawString
}

func findMatchingGoBraceByte(source []byte, openPos, limit uint32) (uint32, bool) {
	if int(openPos) >= len(source) || int(limit) > len(source) || openPos >= limit || source[openPos] != '{' {
		return 0, false
	}
	var (
		depth          int
		inLineComment  bool
		inBlockComment bool
		inString       bool
		inRune         bool
		inRawString    bool
		escape         bool
	)
	for i := int(openPos); i < int(limit); i++ {
		b := source[i]
		if inLineComment {
			if b == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if b == '*' && i+1 < int(limit) && source[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if inString {
			if escape {
				escape = false
				continue
			}
			if b == '\\' {
				escape = true
				continue
			}
			if b == '"' {
				inString = false
			}
			continue
		}
		if inRune {
			if escape {
				escape = false
				continue
			}
			if b == '\\' {
				escape = true
				continue
			}
			if b == '\'' {
				inRune = false
			}
			continue
		}
		if inRawString {
			if b == '`' {
				inRawString = false
			}
			continue
		}
		switch b {
		case '/':
			if i+1 < int(limit) && source[i+1] == '/' {
				inLineComment = true
				i++
				continue
			}
			if i+1 < int(limit) && source[i+1] == '*' {
				inBlockComment = true
				i++
				continue
			}
		case '"':
			inString = true
		case '\'':
			inRune = true
		case '`':
			inRawString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return uint32(i), true
			}
			if depth < 0 {
				return 0, false
			}
		}
	}
	return 0, false
}

func goForStatementHasOnlyTrailingTrivia(source []byte, start, end uint32) bool {
	for start < end {
		switch source[start] {
		case ' ', '\t', '\r', '\n':
			start++
		case '/':
			if start+1 >= end {
				return false
			}
			switch source[start+1] {
			case '/':
				start += 2
				for start < end && source[start] != '\n' {
					start++
				}
			case '*':
				start += 2
				for start+1 < end && !(source[start] == '*' && source[start+1] == '/') {
					start++
				}
				if start+1 >= end {
					return false
				}
				start += 2
			default:
				return false
			}
		default:
			return false
		}
	}
	return true
}

func parseGoForHeaderSkeleton(source []byte, start, openBrace uint32, p *Parser) (*Node, func(), bool) {
	const prefix = "package p\nfunc _() {\n"
	header := source[start:openBrace]
	skeleton := make([]byte, 0, len(prefix)+len(header)+5)
	skeleton = append(skeleton, prefix...)
	skeleton = append(skeleton, header...)
	skeleton = append(skeleton, '{', '}', '\n', '}', '\n')
	tree, err := p.parseForRecovery(skeleton)
	if err != nil || tree == nil || tree.RootNode() == nil || tree.RootNode().HasError() {
		if tree != nil {
			tree.Release()
		}
		return nil, nil, false
	}

	startPoint := advancePointByBytes(Point{}, source[:start])
	prefixPoint := advancePointByBytes(Point{}, []byte(prefix))
	if startPoint.Row < prefixPoint.Row {
		tree.Release()
		return nil, nil, false
	}
	offsetRoot := tree.RootNodeWithOffset(
		start-uint32(len(prefix)),
		Point{Row: startPoint.Row - prefixPoint.Row, Column: startPoint.Column},
	)
	forNode := goFirstNodeOfType(offsetRoot, p.language, "for_statement")
	if forNode == nil || forNode.ChildCount() == 0 || forNode.startByte != start {
		tree.Release()
		return nil, nil, false
	}
	return forNode, tree.Release, true
}

func trimGoSourceRange(source []byte, start, end uint32) (uint32, uint32, bool) {
	for start < end {
		switch source[start] {
		case ' ', '\t', '\r', '\n':
			start++
		default:
			goto startReady
		}
	}
	return 0, 0, false

startReady:
	for end > start {
		switch source[end-1] {
		case ' ', '\t', '\r', '\n':
			end--
		default:
			return start, end, true
		}
	}
	return 0, 0, false
}

func goRecoverIfStatementFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	trimmedStart := start
	for trimmedStart < end {
		switch source[trimmedStart] {
		case ' ', '\t', '\r', '\n':
			trimmedStart++
		default:
			goto trimmedStartReady
		}
	}
	return nil, false

trimmedStartReady:
	trimmedEnd := end
	for trimmedEnd > trimmedStart {
		switch source[trimmedEnd-1] {
		case ' ', '\t', '\r', '\n':
			trimmedEnd--
		default:
			goto trimmedEndReady
		}
	}
	return nil, false

trimmedEndReady:
	stmt := source[trimmedStart:trimmedEnd]
	if !bytes.HasPrefix(stmt, []byte("if ")) {
		return nil, false
	}
	openBrace := bytes.IndexByte(stmt, '{')
	if openBrace < 0 {
		return nil, false
	}
	closeBrace := findMatchingBraceByte(stmt, openBrace, len(stmt))
	if closeBrace < 0 || closeBrace <= openBrace {
		return nil, false
	}
	openBraceAbs := trimmedStart + uint32(openBrace)
	closeBraceAbs := trimmedStart + uint32(closeBrace)
	condStart := trimmedStart + uint32(len("if "))
	condEnd := openBraceAbs
	for condStart < condEnd {
		switch source[condStart] {
		case ' ', '\t', '\r', '\n':
			condStart++
		default:
			goto condStartReady
		}
	}
	return nil, false

condStartReady:
	for condEnd > condStart {
		switch source[condEnd-1] {
		case ' ', '\t', '\r', '\n':
			condEnd--
		default:
			goto condEndReady
		}
	}
	return nil, false

condEndReady:
	condition, ok := goRecoverExpressionNodeFromRange(source, condStart, condEnd, p, arena)
	if !ok || condition == nil {
		return nil, false
	}
	bodyAbsStart := openBraceAbs + 1
	bodyAbsEnd := closeBraceAbs
	bodyNodes, ok := goRecoverFunctionBodyNodes(source, bodyAbsStart, bodyAbsEnd, p, arena)
	if !ok {
		return nil, false
	}
	block, ok := goBuildRecoveredBlockNode(source, openBraceAbs, closeBraceAbs, bodyNodes, arena, p.language)
	if !ok {
		return nil, false
	}
	ifStmtSym, ok := symbolByName(p.language, "if_statement")
	if !ok {
		return nil, false
	}
	ifTokenSym, ok := symbolByName(p.language, "if")
	if !ok {
		return nil, false
	}
	ifStmtNamed := symbolIsNamed(p.language, ifStmtSym)
	ifLeafStart := advancePointByBytes(Point{}, source[:trimmedStart])
	ifLeafEnd := advancePointByBytes(ifLeafStart, source[trimmedStart:trimmedStart+2])
	ifLeaf := newLeafNodeInArena(arena, ifTokenSym, false, trimmedStart, trimmedStart+2, ifLeafStart, ifLeafEnd)
	children := []*Node{ifLeaf, condition, block}
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	return newParentNodeInArena(arena, ifStmtSym, ifStmtNamed, children, goSyntheticIfFieldIDs(arena, len(children), p.language), 0), true
}

func goFunctionStatementRanges(source []byte, start, end uint32) [][2]uint32 {
	var ranges [][2]uint32
	chunkStart := uint32(0)
	inChunk := false
	var (
		braceDepth     int
		parenDepth     int
		bracketDepth   int
		inLineComment  bool
		inBlockComment bool
		inString       bool
		inRune         bool
		inRawString    bool
		escape         bool
	)
	flush := func(pos uint32) {
		if !inChunk || pos <= chunkStart {
			inChunk = false
			return
		}
		ranges = append(ranges, [2]uint32{chunkStart, pos})
		inChunk = false
	}
	for i := int(start); i < int(end); i++ {
		b := source[i]
		if !inChunk && (b == ' ' || b == '\t' || b == '\r' || b == '\n') {
			continue
		}
		if !inChunk {
			chunkStart = uint32(i)
			inChunk = true
		}
		if inLineComment {
			if b == '\n' {
				inLineComment = false
				if braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 {
					flush(uint32(i))
				}
			}
			continue
		}
		if inBlockComment {
			if b == '*' && i+1 < int(end) && source[i+1] == '/' {
				inBlockComment = false
				i++
				continue
			}
			continue
		}
		if inString {
			if escape {
				escape = false
				continue
			}
			if b == '\\' {
				escape = true
				continue
			}
			if b == '"' {
				inString = false
			}
			continue
		}
		if inRune {
			if escape {
				escape = false
				continue
			}
			if b == '\\' {
				escape = true
				continue
			}
			if b == '\'' {
				inRune = false
			}
			continue
		}
		if inRawString {
			if b == '`' {
				inRawString = false
			}
			continue
		}
		switch b {
		case '/':
			if i+1 < int(end) && source[i+1] == '/' {
				inLineComment = true
				i++
				continue
			}
			if i+1 < int(end) && source[i+1] == '*' {
				inBlockComment = true
				i++
				continue
			}
		case '"':
			inString = true
		case '\'':
			inRune = true
		case '`':
			inRawString = true
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
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
		case '\n':
			if braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 {
				flush(uint32(i))
			}
		}
	}
	if inChunk {
		flush(end)
	}
	return ranges
}

func goFirstFunctionLikeChild(root *Node, lang *Language) *Node {
	return goFirstNodeOfType(root, lang, "function_declaration", "method_declaration")
}

func goFirstNodeOfType(root *Node, lang *Language, types ...string) *Node {
	if root == nil || lang == nil {
		return nil
	}
	for i := 0; i < root.ChildCount(); i++ {
		child := root.Child(i)
		if child == nil {
			continue
		}
		childType := child.Type(lang)
		for _, typ := range types {
			if childType == typ {
				return child
			}
		}
		if found := goFirstNodeOfType(child, lang, types...); found != nil {
			return found
		}
	}
	return nil
}

func goExtractRecoveredStatementNodes(root *Node, source []byte, lang *Language, arena *nodeArena) []*Node {
	fn := goFirstFunctionLikeChild(root, lang)
	if fn == nil || fn.ChildCount() == 0 {
		return nil
	}
	block := fn.Child(fn.ChildCount() - 1)
	if block == nil || block.Type(lang) != "block" || block.ChildCount() < 2 {
		return nil
	}
	var out []*Node
	for i := 1; i < block.ChildCount()-1; i++ {
		child := block.Child(i)
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "statement_list", "statement_list_repeat1":
			for j := 0; j < child.ChildCount(); j++ {
				grand := child.Child(j)
				if grand != nil {
					if arena != nil {
						cloned := cloneTreeNodesIntoArena(grand, arena)
						recomputeNodePointsFromBytes(cloned, source)
						out = append(out, cloned)
					} else {
						out = append(out, grand)
					}
				}
			}
		default:
			if arena != nil {
				cloned := cloneTreeNodesIntoArena(child, arena)
				recomputeNodePointsFromBytes(cloned, source)
				out = append(out, cloned)
			} else {
				out = append(out, child)
			}
		}
	}
	return out
}

func goExtractSingleRecoveredStatement(root *Node, source []byte, lang *Language, arena *nodeArena) *Node {
	nodes := goExtractRecoveredStatementNodes(root, source, lang, arena)
	if len(nodes) == 1 {
		return nodes[0]
	}
	return nil
}

func goRecoverExpressionNodeFromRange(source []byte, start, end uint32, p *Parser, arena *nodeArena) (*Node, bool) {
	if p == nil || p.language == nil || start >= end || int(end) > len(source) {
		return nil, false
	}
	const prefix = "package p\nvar _ = "
	expr := bytes.TrimSpace(source[start:end])
	if len(expr) == 0 {
		return nil, false
	}
	wrapped := make([]byte, 0, len(prefix)+len(expr)+1)
	wrapped = append(wrapped, prefix...)
	wrapped = append(wrapped, expr...)
	wrapped = append(wrapped, '\n')
	tree, err := p.parseForRecovery(wrapped)
	if err != nil || tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		return nil, false
	}
	defer tree.Release()
	startPoint := advancePointByBytes(Point{}, source[:start])
	prefixPoint := advancePointByBytes(Point{}, []byte(prefix))
	if startPoint.Row < prefixPoint.Row {
		return nil, false
	}
	offsetRoot := tree.RootNodeWithOffset(start-uint32(len(prefix)), Point{Row: startPoint.Row - prefixPoint.Row, Column: startPoint.Column})
	if offsetRoot == nil || offsetRoot.HasError() {
		return nil, false
	}
	exprNode := goExtractRecoveredVarInitializer(offsetRoot, p.language, arena)
	recomputeNodePointsFromBytes(exprNode, source)
	return exprNode, exprNode != nil
}

func goExtractRecoveredVarInitializer(root *Node, lang *Language, arena *nodeArena) *Node {
	if root == nil || lang == nil {
		return nil
	}
	var walk func(*Node) *Node
	walk = func(n *Node) *Node {
		if n == nil {
			return nil
		}
		if n.Type(lang) == "expression_list" {
			for i := 0; i < n.ChildCount(); i++ {
				child := n.Child(i)
				if child != nil && child.IsNamed() {
					if arena != nil {
						return cloneTreeNodesIntoArena(child, arena)
					}
					return child
				}
			}
		}
		for i := 0; i < n.ChildCount(); i++ {
			if out := walk(n.Child(i)); out != nil {
				return out
			}
		}
		return nil
	}
	return walk(root)
}

func goBuildRecoveredBlockNode(source []byte, openBrace, closeBrace uint32, bodyNodes []*Node, arena *nodeArena, lang *Language) (*Node, bool) {
	if lang == nil || int(closeBrace) >= len(source) || openBrace >= closeBrace {
		return nil, false
	}
	blockSym, ok := symbolByName(lang, "block")
	if !ok {
		return nil, false
	}
	blockNamed := symbolIsNamed(lang, blockSym)
	stmtListSym, ok := symbolByName(lang, "statement_list")
	if !ok {
		return nil, false
	}
	stmtListNamed := symbolIsNamed(lang, stmtListSym)
	openSym, ok := symbolByName(lang, "{")
	if !ok {
		return nil, false
	}
	closeSym, ok := symbolByName(lang, "}")
	if !ok {
		return nil, false
	}
	openTok := newLeafNodeInArena(arena, openSym, false, openBrace, openBrace+1, advancePointByBytes(Point{}, source[:openBrace]), advancePointByBytes(Point{}, source[:openBrace+1]))
	closeTok := newLeafNodeInArena(arena, closeSym, false, closeBrace, closeBrace+1, advancePointByBytes(Point{}, source[:closeBrace]), advancePointByBytes(Point{}, source[:closeBrace+1]))
	var stmtList *Node
	if len(bodyNodes) > 0 {
		stmtChildren := bodyNodes
		if arena != nil {
			buf := arena.allocNodeSlice(len(bodyNodes))
			copy(buf, bodyNodes)
			stmtChildren = buf
		}
		stmtList = newParentNodeInArena(arena, stmtListSym, stmtListNamed, stmtChildren, nil, 0)
	}
	children := make([]*Node, 0, 3)
	children = append(children, openTok)
	if stmtList != nil {
		children = append(children, stmtList)
	}
	children = append(children, closeTok)
	return newParentNodeInArena(arena, blockSym, blockNamed, children, nil, 0), true
}

func recomputeNodePointsFromBytes(n *Node, source []byte) {
	if n == nil || len(source) == 0 {
		return
	}
	if int(n.startByte) <= len(source) {
		n.startPoint = advancePointByBytes(Point{}, source[:n.startByte])
	}
	if int(n.endByte) <= len(source) {
		n.endPoint = advancePointByBytes(Point{}, source[:n.endByte])
	}
	for i := 0; i < resultChildCount(n); i++ {
		recomputeNodePointsFromBytes(resultChildAt(n, i), source)
	}
}

func shiftNodeBytes(n *Node, delta int64) bool {
	if n == nil || delta == 0 {
		return n != nil
	}
	var walk func(*Node) bool
	walk = func(cur *Node) bool {
		if cur == nil {
			return false
		}
		start := int64(cur.startByte) + delta
		end := int64(cur.endByte) + delta
		if start < 0 || end < start {
			return false
		}
		cur.startByte = uint32(start)
		cur.endByte = uint32(end)
		for i := 0; i < resultChildCount(cur); i++ {
			child := resultChildAt(cur, i)
			if !walk(child) {
				return false
			}
			child.parent = cur
			child.childIndex = int32(i)
		}
		return true
	}
	return walk(n)
}

func goSyntheticIfFieldIDs(arena *nodeArena, childCount int, lang *Language) []FieldID {
	fieldIDs := make([]FieldID, childCount)
	if arena != nil {
		fieldIDs = arena.allocFieldIDSlice(childCount)
	}
	if fid, ok := lang.FieldByName("condition"); ok && childCount > 1 {
		fieldIDs[1] = fid
	}
	if fid, ok := lang.FieldByName("consequence"); ok && childCount > 2 {
		fieldIDs[2] = fid
	}
	return fieldIDs
}

func goTopLevelChunkStarts(source []byte, start uint32) []uint32 {
	if int(start) >= len(source) {
		return nil
	}
	var starts []uint32
	var (
		braceDepth     int
		parenDepth     int
		bracketDepth   int
		inLineComment  bool
		inBlockComment bool
		inString       bool
		inRune         bool
		inRawString    bool
		escape         bool
		lineStart      = uint32(0)
		atLineStart    = true
	)
	for i := 0; i < len(source); i++ {
		b := source[i]
		if inLineComment {
			if b == '\n' {
				inLineComment = false
				lineStart = uint32(i + 1)
				atLineStart = true
			}
			continue
		}
		if inBlockComment {
			if b == '*' && i+1 < len(source) && source[i+1] == '/' {
				inBlockComment = false
				i++
				continue
			}
			if b == '\n' {
				lineStart = uint32(i + 1)
				atLineStart = true
			}
			continue
		}
		if inString {
			if escape {
				escape = false
				continue
			}
			if b == '\\' {
				escape = true
				continue
			}
			if b == '"' {
				inString = false
			}
			if b == '\n' {
				lineStart = uint32(i + 1)
				atLineStart = true
			}
			continue
		}
		if inRune {
			if escape {
				escape = false
				continue
			}
			if b == '\\' {
				escape = true
				continue
			}
			if b == '\'' {
				inRune = false
			}
			if b == '\n' {
				lineStart = uint32(i + 1)
				atLineStart = true
			}
			continue
		}
		if inRawString {
			if b == '`' {
				inRawString = false
				continue
			}
			if b == '\n' {
				lineStart = uint32(i + 1)
				atLineStart = true
			}
			continue
		}
		if atLineStart {
			j := i
			for j < len(source) && (source[j] == ' ' || source[j] == '\t' || source[j] == '\r') {
				j++
			}
			if braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 && uint32(j) >= start && goLineStartsTopLevelChunk(source[j:]) {
				starts = append(starts, uint32(j))
			}
			atLineStart = false
		}
		switch b {
		case '/':
			if i+1 < len(source) && source[i+1] == '/' {
				inLineComment = true
				i++
				continue
			}
			if i+1 < len(source) && source[i+1] == '*' {
				inBlockComment = true
				i++
				continue
			}
		case '"':
			inString = true
		case '\'':
			inRune = true
		case '`':
			inRawString = true
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
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
		case '\n':
			lineStart = uint32(i + 1)
			atLineStart = true
		}
		_ = lineStart
	}
	return starts
}

func goLineStartsTopLevelChunk(line []byte) bool {
	switch {
	case len(line) == 0:
		return false
	case bytes.HasPrefix(line, []byte("//")),
		bytes.HasPrefix(line, []byte("/*")),
		bytes.HasPrefix(line, []byte("func ")),
		bytes.HasPrefix(line, []byte("var ")),
		bytes.HasPrefix(line, []byte("const ")),
		bytes.HasPrefix(line, []byte("type ")),
		bytes.HasPrefix(line, []byte("import ")):
		return true
	default:
		return false
	}
}

func flattenRootSelfFragments(nodes []*Node, arena *nodeArena, rootSymbol Symbol) []*Node {
	if len(nodes) <= 1 {
		return nodes
	}
	changed := false
	out := make([]*Node, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.symbol == rootSymbol && resultChildCount(node) > 0 {
			for i := 0; i < resultChildCount(node); i++ {
				out = append(out, resultChildAt(node, i))
			}
			changed = true
			continue
		}
		out = append(out, node)
	}
	if !changed {
		return nodes
	}
	if arena != nil {
		buf := arena.allocNodeSlice(len(out))
		copy(buf, out)
		return buf
	}
	return out
}

func flattenInvisibleRootChildren(root *Node, arena *nodeArena, lang *Language) *Node {
	if root == nil || lang == nil || resultChildCount(root) == 0 {
		return root
	}
	symbolMeta := lang.SymbolMetadata
	changed := false
	childCount := resultChildCount(root)
	for i := 0; i < childCount; i++ {
		child := resultChildAt(root, i)
		if shouldFlattenInvisibleRootChild(child, symbolMeta) {
			changed = true
			break
		}
	}
	if !changed {
		return root
	}
	// Capture the pre-flatten span before mutating: an invisible (hidden,
	// non-extra) LEAF child contributes zero substitute children when
	// flattened (appendFlattenedInvisibleRootChildWalk's recursion has
	// nothing to inline for a childless node), so it can vanish from the
	// output entirely. populateParentNode (via replaceNodeChildrenUnfielded
	// below) then recomputes root's span strictly from whatever children
	// survive, silently SHRINKING the root below the real content it
	// structurally absorbed — e.g. a trailing invisible token dropped this
	// way took the root's endByte from the true end-of-input back to the
	// last surviving (visible or extra-wrapped) child's end. A hidden node's
	// bytes are still part of its parent's span in tree-sitter C even though
	// the node itself never appears in the concrete tree; widen (never
	// shrink) back to the original extent afterward to preserve that
	// invariant.
	origStartByte, origEndByte := root.startByte, root.endByte
	origStartPoint, origEndPoint := root.startPoint, root.endPoint
	children := resultChildSliceForMutation(root)
	out := make([]*Node, 0, len(children))
	for _, child := range children {
		if child == nil {
			continue
		}
		out = appendFlattenedInvisibleRootChild(out, child, arena, symbolMeta)
	}
	if arena != nil {
		buf := arena.allocNodeSlice(len(out))
		copy(buf, out)
		out = buf
	}
	replaceNodeChildrenUnfielded(root, out)
	widenNodeSpanToChildSpan(root, origStartByte, origEndByte, origStartPoint, origEndPoint)
	return root
}

func appendFlattenedInvisibleRootChild(out []*Node, child *Node, arena *nodeArena, symbolMeta []SymbolMetadata) []*Node {
	return appendFlattenedInvisibleRootChildWalk(out, child, arena, symbolMeta, nil, 0)
}

func appendFlattenedInvisibleRootChildWalk(out []*Node, child *Node, arena *nodeArena, symbolMeta []SymbolMetadata, onPath map[*Node]struct{}, depth int) []*Node {
	if child == nil {
		return out
	}
	if !shouldFlattenInvisibleRootChild(child, symbolMeta) {
		return append(out, child)
	}
	if depth > maxTreeWalkDepth {
		return append(out, child)
	}
	if onPath == nil {
		onPath = make(map[*Node]struct{}, 8)
	}
	if _, ancestor := onPath[child]; ancestor {
		return out
	}
	onPath[child] = struct{}{}
	childCount := resultChildCount(child)
	for i := 0; i < childCount; i++ {
		out = appendFlattenedInvisibleRootChildWalk(out, resultChildAt(child, i), arena, symbolMeta, onPath, depth+1)
	}
	delete(onPath, child)
	return out
}

func shouldFlattenInvisibleRootChild(child *Node, symbolMeta []SymbolMetadata) bool {
	if child == nil || child.isExtra() || child.isMissing() {
		return false
	}
	return !symbolStructuralForHiddenFlattening(child.symbol, symbolMeta, nil)
}
