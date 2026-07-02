package gotreesitter

// Swift ternary/conditional-operator recovery (issue #135).
//
// The runtime Swift blob (ts2go-converted from tree-sitter-swift) never fires
// the `ternary_expression` reduction: `cond ? a : b` drops the `? a : b` tail
// into an ERROR node in every position (top level, `return`, call argument),
// and inside a function it collapses the enclosing declaration to
// `_modifierless_function_declaration_no_body`, so the whole function's
// structure is lost. (Optional `?`, `??`, `?.`, `as?` all parse fine — only the
// conditional operator is missing.)
//
// Unlike the trailing-closure ambiguity (#118/#123/#131), no rewrite of the
// ternary itself parses on this blob, so the paren-injection-and-reparse trick
// cannot apply. Instead this pass reconstructs the `ternary_expression` node:
//
//  1. Source-scan for ternary sites (a `?` that has a matching `:`), skipping
//     `??`, `?.`, postfix `x?`, and `as?`/`try?`.
//  2. Reparse the source with each `? if_true : if_false` tail blanked out with
//     spaces (length-preserving, so byte offsets are unchanged). Because the
//     conditional operator is the only thing that fails, the blanked source
//     reparses cleanly with the condition expression left in place.
//  3. Reparse each `if_true` / `if_false` operand span on its own.
//  4. Splice a synthesised `ternary_expression` (children
//     `[condition, "?", if_true, ":", if_false]`, fields
//     condition/if_true/if_false — the exact upstream layout) in place of the
//     condition node in the clean skeleton.
//
// The rewrite is accepted only when the reconstructed tree is error-free and
// byte-faithful; otherwise the original (broken) tree is left untouched, so a
// mis-detected site can never make the result worse.

// swiftTernarySite describes one `condition ? if_true : if_false` occurrence,
// in original-source byte coordinates. All spans are trivia-trimmed.
type swiftTernarySite struct {
	condStart    uint32 // start of the condition expression
	condEnd      uint32 // end of the condition expression (== trimmed `?` pos)
	questPos     uint32 // the `?`
	ifTrueStart  uint32
	ifTrueEnd    uint32
	colonPos     uint32 // the `:`
	ifFalseStart uint32
	ifFalseEnd   uint32
}

// swiftMaxTernaryRecoverySites caps how many ternary sites one file recovery
// will reconstruct, bounding the reparse work on pathological input.
const swiftMaxTernaryRecoverySites = 256

func normalizeSwiftRecoveredTernaryExpressions(root *Node, source []byte, p *Parser, lang *Language) {
	if root == nil || p == nil || p.skipRecoveryReparse || lang == nil || root.ownerArena == nil || len(source) == 0 {
		return
	}
	if root.Type(lang) != "source_file" || !root.HasError() {
		return
	}
	ternarySym, ok := symbolByName(lang, "ternary_expression")
	if !ok {
		return
	}
	questSym, ok := symbolByName(lang, "?")
	if !ok {
		return
	}
	colonSym, ok := symbolByName(lang, ":")
	if !ok {
		return
	}
	condFID, ok := lang.FieldByName("condition")
	if !ok {
		return
	}
	ifTrueFID, ok := lang.FieldByName("if_true")
	if !ok {
		return
	}
	ifFalseFID, ok := lang.FieldByName("if_false")
	if !ok {
		return
	}

	sites := swiftScanTernarySites(source)
	if len(sites) == 0 || len(sites) > swiftMaxTernaryRecoverySites {
		return
	}
	blankRanges := make([][2]uint32, 0, len(sites))
	for i := range sites {
		s := &sites[i]
		if s.condStart >= s.condEnd {
			return
		}
		blankRanges = append(blankRanges, [2]uint32{s.questPos, s.ifFalseEnd})
	}

	transformed := swiftBlankRanges(source, blankRanges)
	tree, err := p.parseForRecovery(transformed)
	if err != nil || tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		return
	}
	skeleton := tree.RootNode()
	// Accept only a fully clean, byte-faithful skeleton — same gate the
	// trailing-closure recovery uses.
	if skeleton.HasError() || skeleton.Type(lang) != "source_file" || skeleton.endByte != uint32(len(transformed)) {
		tree.Release()
		return
	}

	arena := root.ownerArena
	builder := &swiftTernaryBuilder{
		source:     source,
		lang:       lang,
		arena:      arena,
		parser:     p,
		ternarySym: ternarySym,
		questSym:   questSym,
		colonSym:   colonSym,
		condFID:    condFID,
		ifTrueFID:  ifTrueFID,
		ifFalseFID: ifFalseFID,
		sites:      sites,
	}
	newRoot := builder.rebuild(skeleton)
	tree.Release()
	if newRoot == nil || !builder.wrappedAll() {
		return
	}
	// Final safety gate: byte-faithful and error-free.
	if swiftAnyChildHasError(newRoot) || newRoot.endByte != uint32(len(source)) {
		return
	}

	root.symbol = newRoot.symbol
	root.productionID = newRoot.productionID
	root.fieldIDs = newRoot.fieldIDs
	root.children = cloneNodeSliceIfArena(arena, newRoot.children)
	populateParentNode(root, root.children)
	root.startByte = 0
	root.endByte = uint32(len(source))
	recomputeNodePointsFromBytes(root, source)
	root.setHasError(false)
}

// swiftTernaryBuilder clones the clean skeleton into the arena, wrapping each
// site's condition node in a reconstructed ternary_expression.
type swiftTernaryBuilder struct {
	source     []byte
	lang       *Language
	arena      *nodeArena
	parser     *Parser
	ternarySym Symbol
	questSym   Symbol
	colonSym   Symbol
	condFID    FieldID
	ifTrueFID  FieldID
	ifFalseFID FieldID
	sites      []swiftTernarySite
	wrapped    int
}

func (b *swiftTernaryBuilder) wrappedAll() bool { return b.wrapped == len(b.sites) }

// rebuild deep-clones n into the arena. When n exactly spans a site's condition,
// it is wrapped into a ternary_expression instead (and its descendants are not
// searched for that site). Returns nil on any failure so the caller can bail.
func (b *swiftTernaryBuilder) rebuild(n *Node) *Node {
	if n == nil {
		return nil
	}
	// Wrap the DEEPEST node whose span is exactly the condition: wrappers such as
	// value_argument share the span with the inner expression, but the ternary
	// belongs inside them (value_argument(ternary(...))), not outside.
	if n.isNamed() && n.Type(b.lang) != "ERROR" {
		for i := range b.sites {
			s := &b.sites[i]
			if n.startByte == s.condStart && n.endByte == s.condEnd && !swiftChildSharesSpan(n, s.condStart, s.condEnd) {
				if node := b.buildTernary(n, s); node != nil {
					b.wrapped++
					return node
				}
				return nil
			}
		}
	}
	if resultChildCount(n) == 0 {
		leaf := newLeafNodeInArena(b.arena, n.symbol, n.isNamed(), n.startByte, n.endByte, n.startPoint, n.endPoint)
		leaf.setExtra(n.IsExtra())
		return leaf
	}
	kids := make([]*Node, 0, resultChildCount(n))
	for i := 0; i < resultChildCount(n); i++ {
		rc := b.rebuild(resultChildAt(n, i))
		if rc == nil {
			return nil
		}
		kids = append(kids, rc)
	}
	kids = cloneNodeSliceInArena(b.arena, kids)
	fieldIDs := cloneFieldIDSliceInArena(b.arena, n.fieldIDs)
	node := newParentNodeInArena(b.arena, n.symbol, n.isNamed(), kids, fieldIDs, n.productionID)
	node.setExtra(n.IsExtra())
	// Preserve the original span but extend it to cover any child a nested
	// ternary widened (never shrink — the parent may cover trailing trivia).
	node.startByte = n.startByte
	node.endByte = n.endByte
	if len(kids) > 0 {
		if kids[0].startByte < node.startByte {
			node.startByte = kids[0].startByte
		}
		if end := kids[len(kids)-1].endByte; end > node.endByte {
			node.endByte = end
		}
	}
	return node
}

// swiftChildSharesSpan reports whether any direct NAMED child of n spans exactly
// [start,end) — i.e. n is a pure wrapper around a same-span nested expression, so
// the ternary belongs one level deeper. Anonymous token children (e.g. the
// `true` keyword inside boolean_literal) do not count.
func swiftChildSharesSpan(n *Node, start, end uint32) bool {
	for i := 0; i < resultChildCount(n); i++ {
		c := resultChildAt(n, i)
		if c != nil && c.isNamed() && c.startByte == start && c.endByte == end {
			return true
		}
	}
	return false
}

// buildTernary constructs a ternary_expression whose condition is a clone of
// condNode (from the skeleton) and whose operands are reparsed from source.
func (b *swiftTernaryBuilder) buildTernary(condNode *Node, s *swiftTernarySite) *Node {
	condClone := cloneTreeNodesIntoArena(condNode, b.arena)
	if condClone == nil {
		return nil
	}
	ifTrue := b.buildOperand(s.ifTrueStart, s.ifTrueEnd)
	if ifTrue == nil {
		return nil
	}
	ifFalse := b.buildOperand(s.ifFalseStart, s.ifFalseEnd)
	if ifFalse == nil {
		return nil
	}
	quest := newLeafNodeInArena(b.arena, b.questSym, false, s.questPos, s.questPos+1, Point{}, Point{})
	colon := newLeafNodeInArena(b.arena, b.colonSym, false, s.colonPos, s.colonPos+1, Point{}, Point{})

	children := cloneNodeSliceInArena(b.arena, []*Node{condClone, quest, ifTrue, colon, ifFalse})
	fieldIDs := cloneFieldIDSliceInArena(b.arena, []FieldID{b.condFID, 0, b.ifTrueFID, 0, b.ifFalseFID})
	node := newParentNodeInArena(b.arena, b.ternarySym, symbolIsNamed(b.lang, b.ternarySym), children, fieldIDs, 0)
	node.startByte = s.condStart
	node.endByte = s.ifFalseEnd
	recomputeNodePointsFromBytes(node, b.source)
	node.setHasError(false)
	return node
}

// buildOperand reparses source[start:end] as a standalone expression and returns
// a clone of the single expression node it produces, shifted to original
// coordinates. Returns nil unless the operand parses cleanly to exactly one
// full-span named node (nested/chained ternaries therefore bail safely).
func (b *swiftTernaryBuilder) buildOperand(start, end uint32) *Node {
	if start >= end || end > uint32(len(b.source)) {
		return nil
	}
	sub := b.source[start:end]
	tree, err := b.parser.parseForRecovery(sub)
	if err != nil || tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		return nil
	}
	r := tree.RootNode()
	if r.HasError() || r.endByte != uint32(len(sub)) {
		tree.Release()
		return nil
	}
	var expr *Node
	for i := 0; i < resultChildCount(r); i++ {
		c := resultChildAt(r, i)
		if c == nil || c.IsExtra() {
			continue
		}
		if expr != nil {
			tree.Release()
			return nil // more than one top-level node
		}
		expr = c
	}
	if expr == nil || expr.startByte != 0 || expr.endByte != uint32(len(sub)) {
		tree.Release()
		return nil
	}
	clone := cloneTreeNodesIntoArena(expr, b.arena)
	tree.Release()
	if clone == nil || !shiftNodeBytes(clone, int64(start)) {
		return nil
	}
	recomputeNodePointsFromBytes(clone, b.source)
	return clone
}

// swiftScanTernarySites walks source (skipping strings and comments, tracking
// bracket depth) and returns each `condition ? if_true : if_false` occurrence it
// can bound. Scanning resumes past a matched site's if_false, so a ternary
// nested inside another's operand is not reported separately (the operand
// reparse bails on it, leaving the whole site unrecovered).
func swiftScanTernarySites(source []byte) []swiftTernarySite {
	var sites []swiftTernarySite
	n := uint32(len(source))
	// operandStack[d] is the byte offset at which the current expression operand
	// began at bracket depth d. The ternary condition is the operand in progress
	// when its `?` is reached — i.e. everything since the last opener, separator,
	// assignment, `:` or expression-introducing keyword at that depth.
	operandStack := []uint32{0}
	setOperand := func(pos uint32) {
		operandStack[len(operandStack)-1] = swiftSkipSpaceAndComments(source, pos)
	}
	i := uint32(0)
	for i < n {
		j, _ := swiftSkipStringOrComment(source, i)
		if j != i {
			i = j
			continue
		}
		b := source[i]
		// Whole-word handling: reset the operand after expression-introducing
		// keywords so the condition doesn't swallow `return`/`case`/etc.
		if isSwiftWordByte(b) && (i == 0 || !isSwiftWordByte(source[i-1])) {
			k := i + 1
			for k < n && isSwiftWordByte(source[k]) {
				k++
			}
			if swiftIsExprIntroKeyword(source[i:k]) {
				setOperand(k)
			}
			i = k
			continue
		}
		switch b {
		case '(', '[', '{':
			operandStack = append(operandStack, swiftSkipSpaceAndComments(source, i+1))
			i++
			continue
		case ')', ']', '}':
			// A closed bracket group is part of the enclosing operand (`(a) + b`,
			// `foo()`, `arr[i]`), so the operand start is left unchanged.
			if len(operandStack) > 1 {
				operandStack = operandStack[:len(operandStack)-1]
			}
			i++
			continue
		case ',', ';', '\n':
			setOperand(i + 1)
			i++
			continue
		case '=':
			// Assignment (not ==, !=, <=, >=, +=, …) introduces a fresh operand.
			if (i+1 >= n || source[i+1] != '=') && (i == 0 || !swiftIsOperatorByte(source[i-1])) {
				setOperand(i + 1)
			}
			i++
			continue
		case ':':
			setOperand(i + 1)
			i++
			continue
		case '?':
			// Exclude `??` (nil-coalescing) and `?.` (optional chaining).
			if i+1 < n && (source[i+1] == '?' || source[i+1] == '.') {
				i += 2
				continue
			}
			// Exclude the `?` of `as?` / `try?`.
			if swiftPrecededByWord(source, i, "as") || swiftPrecededByWord(source, i, "try") {
				i++
				continue
			}
			condStart := operandStack[len(operandStack)-1]
			site, ok := swiftMatchTernaryTail(source, i)
			if !ok || condStart >= site.condEnd {
				i++
				continue
			}
			site.condStart = condStart
			sites = append(sites, site)
			// Resume after the whole ternary; the ternary itself is now the
			// operand at this depth.
			operandStack[len(operandStack)-1] = condStart
			i = site.ifFalseEnd
			continue
		}
		i++
	}
	return sites
}

// swiftIsExprIntroKeyword reports whether word begins a fresh expression, so a
// following ternary condition must not extend back across it.
func swiftIsExprIntroKeyword(word []byte) bool {
	switch string(word) {
	case "return", "throw", "yield", "in", "where", "case", "else", "if", "while", "guard":
		return true
	}
	return false
}

// swiftIsOperatorByte reports whether b is a Swift operator character, used to
// tell an assignment `=` apart from `==`/`!=`/`<=`/`+=`/… .
func swiftIsOperatorByte(b byte) bool {
	switch b {
	case '=', '!', '<', '>', '+', '-', '*', '/', '%', '&', '|', '^', '~', '?':
		return true
	}
	return false
}

// swiftMatchTernaryTail, given the position of a candidate ternary `?`, locates
// the matching `:` (accounting for nested ternaries and bracket groups) and the
// if_true / if_false operand spans. Returns ok=false if no matching `:` is found
// before the expression terminates.
func swiftMatchTernaryTail(source []byte, questPos uint32) (swiftTernarySite, bool) {
	n := uint32(len(source))
	condEnd := swiftTrimTrailingWs(source, questPos)
	ifTrueStart := swiftSkipSpaceAndComments(source, questPos+1)

	// Find the matching `:`.
	depth := 0
	questCount := 0
	colonPos := uint32(0)
	found := false
	i := ifTrueStart
	for i < n {
		j, _ := swiftSkipStringOrComment(source, i)
		if j != i {
			i = j
			continue
		}
		switch source[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth == 0 {
				return swiftTernarySite{}, false
			}
			depth--
		case ';':
			if depth == 0 {
				return swiftTernarySite{}, false
			}
		case '?':
			if depth == 0 && !(i+1 < n && (source[i+1] == '?' || source[i+1] == '.')) &&
				!swiftPrecededByWord(source, i, "as") && !swiftPrecededByWord(source, i, "try") {
				questCount++
			}
		case ':':
			if depth == 0 {
				if questCount > 0 {
					questCount--
				} else {
					colonPos = i
					found = true
				}
			}
		}
		if found {
			break
		}
		i++
	}
	if !found {
		return swiftTernarySite{}, false
	}
	ifTrueEnd := swiftTrimTrailingWs(source, colonPos)
	if ifTrueStart >= ifTrueEnd {
		return swiftTernarySite{}, false
	}
	ifFalseStart := swiftSkipSpaceAndComments(source, colonPos+1)
	ifFalseEnd := swiftScanExpressionEnd(source, ifFalseStart)
	if ifFalseStart >= ifFalseEnd {
		return swiftTernarySite{}, false
	}
	return swiftTernarySite{
		condEnd:      condEnd,
		questPos:     questPos,
		ifTrueStart:  ifTrueStart,
		ifTrueEnd:    ifTrueEnd,
		colonPos:     colonPos,
		ifFalseStart: ifFalseStart,
		ifFalseEnd:   ifFalseEnd,
	}, true
}

// swiftScanExpressionEnd returns the trivia-trimmed end of the expression that
// begins at start, stopping at the first depth-zero expression terminator
// (`)`, `]`, `}`, `,`, `;`, a body-opening `{`, or a newline) or EOF.
func swiftScanExpressionEnd(source []byte, start uint32) uint32 {
	n := uint32(len(source))
	depth := 0
	i := start
	for i < n {
		j, _ := swiftSkipStringOrComment(source, i)
		if j != i {
			i = j
			continue
		}
		switch source[i] {
		case '(', '[':
			depth++
		case '{':
			if depth == 0 {
				return swiftTrimTrailingWs(source, i)
			}
			depth++
		case ')', ']', '}':
			if depth == 0 {
				return swiftTrimTrailingWs(source, i)
			}
			depth--
		case ',', ';', '\n':
			if depth == 0 {
				return swiftTrimTrailingWs(source, i)
			}
		}
		i++
	}
	return swiftTrimTrailingWs(source, n)
}

// swiftSkipStringOrComment returns the index just past a string literal or
// comment starting at i, or i unchanged if i is not the start of one. isCode is
// false when a string/comment was skipped.
func swiftSkipStringOrComment(source []byte, i uint32) (uint32, bool) {
	n := uint32(len(source))
	if i >= n {
		return i, true
	}
	b := source[i]
	if b == '/' && i+1 < n {
		if source[i+1] == '/' {
			k := i + 2
			for k < n && source[k] != '\n' {
				k++
			}
			return k, false
		}
		if source[i+1] == '*' {
			k := i + 2
			depth := 1
			for k+1 < n && depth > 0 {
				if source[k] == '/' && source[k+1] == '*' {
					depth++
					k += 2
				} else if source[k] == '*' && source[k+1] == '/' {
					depth--
					k += 2
				} else {
					k++
				}
			}
			if depth > 0 {
				return n, false
			}
			return k, false
		}
	}
	if b == '"' {
		if i+2 < n && source[i+1] == '"' && source[i+2] == '"' {
			k := i + 3
			for k+2 < n && !(source[k] == '"' && source[k+1] == '"' && source[k+2] == '"') {
				if source[k] == '\\' {
					k++
				}
				k++
			}
			k += 3
			if k > n {
				k = n
			}
			return k, false
		}
		k := i + 1
		for k < n && source[k] != '"' {
			if source[k] == '\\' {
				k++
			}
			k++
		}
		k++
		if k > n {
			k = n
		}
		return k, false
	}
	return i, true
}

// swiftTrimTrailingWs returns the index just past the last non-whitespace byte
// before pos (i.e. the trivia-trimmed end of the span ending at pos).
func swiftTrimTrailingWs(source []byte, pos uint32) uint32 {
	i := pos
	for i > 0 {
		switch source[i-1] {
		case ' ', '\t', '\n', '\r':
			i--
			continue
		}
		break
	}
	return i
}

// swiftPrecededByWord reports whether the identifier word immediately before pos
// (skipping no whitespace) equals word — used to spot `as?` / `try?`.
func swiftPrecededByWord(source []byte, pos uint32, word string) bool {
	w := uint32(len(word))
	if pos < w {
		return false
	}
	start := pos - w
	if string(source[start:pos]) != word {
		return false
	}
	// The char before the word must not be an identifier byte (so `class?` etc.
	// don't match `as`), and the word must be a whole token.
	if start > 0 && isSwiftWordByte(source[start-1]) {
		return false
	}
	return true
}

// swiftBlankRanges returns a copy of source with each [start,end) range replaced
// by spaces (preserving newlines so line/column offsets stay stable). Length is
// unchanged, so all byte offsets outside the ranges are identical.
func swiftBlankRanges(source []byte, ranges [][2]uint32) []byte {
	out := make([]byte, len(source))
	copy(out, source)
	for _, r := range ranges {
		for i := r[0]; i < r[1] && i < uint32(len(out)); i++ {
			if out[i] != '\n' && out[i] != '\r' {
				out[i] = ' '
			}
		}
	}
	return out
}
