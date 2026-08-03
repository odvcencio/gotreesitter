package gotreesitter

import "bytes"

// normalizeGoReturnedTreeCompatibilityWithCensus drives Go's post-build
// compatibility pipeline. Two members remain: the compatibility walk and the
// new()/make() type-argument retag. Each runs under its own census subpass id,
// so the R2 census reports which member rewrote the tree, not only whether the
// arm did.
//
// Every stage boundary already re-checked p.activeParseStopReason()
// (Timeout/Cancelled); it now also re-checks the memory budget via
// goCompatMemoryBudgetStopReason so a parse that is already over budget (the
// parse loop stopped, or the arena/runtime heap grew past budget during an
// earlier stage) skips the remaining Go compat work entirely instead of
// normalizing a partial tree that carries no C-faithful normalization
// guarantee. This mirrors the existing timeout/cancellation semantics at the
// same call sites (see the 2026-07-12 gocompat-walk-containment-gap finding).
// incrementalRanges, when non-nil, confines the O(nodes) Go compat walk to the
// reparsed spans of an incremental parse (campaign O(edit), spec.campaign.oedit).
// Reused (byte-identical) top-level siblings outside the ranges keep the
// normalization they were built with, so the walk skips their subtrees and
// becomes O(edit) instead of O(file). Nil (every fresh parse, and any
// incremental parse without reuse) keeps the full walk. Only the range-capable
// walk stage consumes it; the new-make stage is already bounded and stays
// full-tree.
func normalizeGoReturnedTreeCompatibilityWithCensus(root *Node, source []byte, p *Parser, lang *Language, incrementalRanges []Range, census materializationSubpassCensus) ParseStopReason {
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
	var walkReason ParseStopReason
	census.run("dispatch.go.compat-walk", func() {
		walkReason = normalizeGoCompatibilityInRangesWithParser(root, source, lang, incrementalRanges, p)
	})
	if resultMaterializationShouldStop(walkReason) {
		return walkReason
	}
	if reason := p.activeParseStopReason(); parseStopReasonIsActive(reason) {
		return reason
	}
	if reason := p.goCompatMemoryBudgetStopReason(arena); reason == ParseStopMemoryBudget {
		return reason
	}
	census.run("dispatch.go.new-make-type", func() {
		normalizeGoNewMakeTypeArgument(root, source, lang)
	})
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
// This function is the narrow, local, byte-identity-preserving patch: it only
// retags node symbols/named-ness and child field labels (never bytes, never
// node count, never parent/child structure) and only for the one syntactic
// position the owner-confirmed divergence covers — the sole/leading argument
// of a literal `new`/`make` call. It follows the same bounded symbol-retagging
// pattern as the other result-compatibility repairs.
//
// The base case retags a bare identifier (`new(dirInfo)`) to type_identifier.
// goNewMakeTypeRetagCtx.relabel extends the same retag to the three other
// expression shapes that the C oracle parses as a type in this slot:
//
//   - `new(pkg.Type)` — a selector_expression whose operand is a bare
//     identifier becomes a qualified_type (operand identifier -> field
//     "package" package_identifier, field identifier -> field "name"
//     type_identifier). A nested selector (`new(a.b.C)`) stays a
//     selector_expression, because the C grammar's qualified_type requires a
//     single-identifier package part, so C keeps that shape an expression.
//   - `new(*T)` — a unary_expression with the "*" operator becomes a
//     pointer_type wrapping the operand retagged as a type (recursively:
//     `new(**T)` nests pointer_type, `new(*pkg.Type)` wraps a qualified_type).
//     A non-"*" operator (`new(&T)`) or an operand that is not itself a valid
//     type (`new(*a.b.C)`) stays a unary_expression, matching C.
//   - `new((T))` — a parenthesized_expression becomes a parenthesized_type and
//     its inner expression is retagged as a type, but only when the inner is
//     itself a valid type.
//
// canBeType gates every case all-or-nothing: the WHOLE leading argument must
// spell a valid type, mirroring C's decision to parse this slot as _type or
// _expression as a unit. Every relabel preserves byte spans, child count, and
// parent/child links; it only relabels symbols, named-ness, and the field ids
// stored for the node's own children. Composite/unambiguous type arguments
// (slice_type, map_type, channel_type, struct_type, array_type, generic_type,
// ...) already carry the correct type shape from the parse and are never
// visited, because relabel is gated on the argument node's symbol being
// identifier/selector/unary/parenthesized.
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
	ctx := &goNewMakeTypeRetagCtx{
		lang:                lang,
		source:              source,
		identifierSym:       identifierSym,
		typeIdentifierSym:   typeIdentifierSym,
		typeIdentifierNamed: symbolIsNamed(lang, typeIdentifierSym),
	}
	// Resolve the optional structured-type symbols/fields. When any is absent
	// (a grammar variant without these node types) the context degrades to the
	// original bare-identifier-only retag, preserving prior behavior exactly.
	ctx.resolveStructured(lang)
	// Optional symbols used only for filtering; a missing one degrades gracefully.
	commentSym, hasComment := symbolByName(lang, "comment")
	typeArgsSym, hasTypeArgs := symbolByName(lang, "type_arguments")

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
		// A generic instantiation (`new[T](x)`, `Foo[T](x)`) parses with a
		// `type_arguments` child and takes C's ordinary call branch, where the
		// argument stays `identifier`. Only the plain new/make special branch
		// (no type_arguments) retags the type position.
		if hasTypeArgs {
			for i := 0; i < n.NamedChildCount(); i++ {
				if c := n.NamedChild(i); c != nil && c.symbol == typeArgsSym {
					return
				}
			}
		}
		args := n.ChildByFieldName("arguments", lang)
		if args == nil || args.symbol != argListSym {
			return
		}
		// The first NON-EXTRA named argument: Go attaches comments as named
		// extras, so a leading comment (`new(/* c */ T)`) must be skipped —
		// otherwise the comment is mistaken for the argument and no retag fires.
		var first *Node
		for i := 0; i < args.NamedChildCount(); i++ {
			c := args.NamedChild(i)
			if c == nil {
				continue
			}
			if hasComment && c.symbol == commentSym {
				continue
			}
			first = c
			break
		}
		if first == nil {
			return
		}
		// All-or-nothing: only relabel when the WHOLE leading argument spells a
		// valid type. C parses this slot as _type or _expression as a unit, so a
		// partially-convertible expression (`*a.b.C`) must stay an expression.
		if !ctx.canBeType(first) {
			return
		}
		ctx.relabel(first)
	})
}

// goNewMakeTypeRetagCtx caches the symbols and field ids used to relabel a
// new()/make() leading-argument expression as its C-oracle type node. It is
// built once per normalize pass. When structured is false (a required symbol
// or field is missing from the active Go grammar) only the bare-identifier
// base case is available, matching the pre-extension behavior.
type goNewMakeTypeRetagCtx struct {
	lang   *Language
	source []byte

	identifierSym       Symbol
	typeIdentifierSym   Symbol
	typeIdentifierNamed bool

	structured             bool
	selectorSym            Symbol
	unarySym               Symbol
	qualifiedTypeSym       Symbol
	qualifiedTypeNamed     bool
	pointerTypeSym         Symbol
	pointerTypeNamed       bool
	packageIdentifierSym   Symbol
	packageIdentifierNamed bool

	operandFieldID FieldID
	fieldFieldID   FieldID
	packageFieldID FieldID
	nameFieldID    FieldID

	// hasParen gates the parenthesized_expression->parenthesized_type relabel
	// (`new((T))`). It resolves independently of the core structured symbols so
	// a grammar without parenthesized_type still relabels qualified/pointer.
	hasParen       bool
	commentSym     Symbol
	hasComment     bool
	parenExprSym   Symbol
	parenTypeSym   Symbol
	parenTypeNamed bool
}

// resolveStructured looks up every symbol and field id needed for the
// selector_expression->qualified_type and unary_expression->pointer_type
// relabels. If any is missing, structured stays false and only the
// bare-identifier retag runs.
func (c *goNewMakeTypeRetagCtx) resolveStructured(lang *Language) {
	selectorSym, ok1 := symbolByName(lang, "selector_expression")
	unarySym, ok2 := symbolByName(lang, "unary_expression")
	qualifiedTypeSym, ok3 := symbolByName(lang, "qualified_type")
	pointerTypeSym, ok4 := symbolByName(lang, "pointer_type")
	packageIdentifierSym, ok5 := lang.symbolByNamePreferNamed("package_identifier")
	operandFieldID, ok6 := lang.FieldByName("operand")
	fieldFieldID, ok7 := lang.FieldByName("field")
	packageFieldID, ok8 := lang.FieldByName("package")
	nameFieldID, ok9 := lang.FieldByName("name")
	if !(ok1 && ok2 && ok3 && ok4 && ok5 && ok6 && ok7 && ok8 && ok9) {
		return
	}
	if operandFieldID == 0 || fieldFieldID == 0 || packageFieldID == 0 || nameFieldID == 0 {
		return
	}
	c.selectorSym = selectorSym
	c.unarySym = unarySym
	c.qualifiedTypeSym = qualifiedTypeSym
	c.qualifiedTypeNamed = symbolIsNamed(lang, qualifiedTypeSym)
	c.pointerTypeSym = pointerTypeSym
	c.pointerTypeNamed = symbolIsNamed(lang, pointerTypeSym)
	c.packageIdentifierSym = packageIdentifierSym
	c.packageIdentifierNamed = symbolIsNamed(lang, packageIdentifierSym)
	c.operandFieldID = operandFieldID
	c.fieldFieldID = fieldFieldID
	c.packageFieldID = packageFieldID
	c.nameFieldID = nameFieldID
	c.structured = true

	c.commentSym, c.hasComment = symbolByName(lang, "comment")
	parenExprSym, okp1 := symbolByName(lang, "parenthesized_expression")
	parenTypeSym, okp2 := symbolByName(lang, "parenthesized_type")
	if okp1 && okp2 {
		c.parenExprSym = parenExprSym
		c.parenTypeSym = parenTypeSym
		c.parenTypeNamed = symbolIsNamed(lang, parenTypeSym)
		c.hasParen = true
	}
}

// parenInner returns the single inner expression of a parenthesized_expression
// (the first non-comment named child), or nil.
func (c *goNewMakeTypeRetagCtx) parenInner(n *Node) *Node {
	for i := 0; i < n.NamedChildCount(); i++ {
		child := n.NamedChild(i)
		if child == nil {
			continue
		}
		if c.hasComment && child.symbol == c.commentSym {
			continue
		}
		return child
	}
	return nil
}

// canBeType reports whether n (an expression node in a new()/make() leading
// slot) spells a type the C oracle would parse as a type node. It never
// mutates. The recursion mirrors the C grammar's _simple_type shape.
func (c *goNewMakeTypeRetagCtx) canBeType(n *Node) bool {
	if n == nil {
		return false
	}
	if n.symbol == c.identifierSym {
		return true
	}
	if !c.structured {
		return false
	}
	switch n.symbol {
	case c.selectorSym:
		// qualified_type's package part is a single identifier; a nested
		// selector (`a.b.C`) can never be a qualified_type, so C keeps it an
		// expression.
		operand := n.ChildByFieldName("operand", c.lang)
		return operand != nil && operand.symbol == c.identifierSym
	case c.unarySym:
		// Only the "*" operator forms a pointer_type; "&", "-", "!", ... stay
		// expressions. The operand must itself be a valid type.
		op := n.ChildByFieldName("operator", c.lang)
		if op == nil || op.Text(c.source) != "*" {
			return false
		}
		return c.canBeType(n.ChildByFieldName("operand", c.lang))
	case c.parenExprSym:
		if !c.hasParen {
			return false
		}
		return c.canBeType(c.parenInner(n))
	}
	return false
}

// relabel converts n in place to its C-oracle type node. The caller MUST have
// confirmed canBeType(n) first. Only symbols, named-ness, and the node's own
// child field ids change; byte spans, child count, and links are untouched.
func (c *goNewMakeTypeRetagCtx) relabel(n *Node) {
	if n == nil {
		return
	}
	if n.symbol == c.identifierSym {
		n.symbol = c.typeIdentifierSym
		n.setNamed(c.typeIdentifierNamed)
		return
	}
	if !c.structured {
		return
	}
	switch n.symbol {
	case c.selectorSym:
		c.relabelQualified(n)
	case c.unarySym:
		c.relabelPointer(n)
	case c.parenExprSym:
		if c.hasParen {
			c.relabelParen(n)
		}
	}
}

// relabelQualified turns a selector_expression into a qualified_type: the
// operand identifier becomes the "package" package_identifier and the field
// identifier becomes the "name" type_identifier. The "." token keeps field 0.
func (c *goNewMakeTypeRetagCtx) relabelQualified(n *Node) {
	childCount := nodeChildCountNoMaterialize(n)
	newIDs := make([]FieldID, childCount)
	for i := 0; i < childCount; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch nodeFieldIDAt(n, i) {
		case c.operandFieldID:
			child.symbol = c.packageIdentifierSym
			child.setNamed(c.packageIdentifierNamed)
			newIDs[i] = c.packageFieldID
		case c.fieldFieldID:
			child.symbol = c.typeIdentifierSym
			child.setNamed(c.typeIdentifierNamed)
			newIDs[i] = c.nameFieldID
		}
	}
	n.symbol = c.qualifiedTypeSym
	n.setNamed(c.qualifiedTypeNamed)
	n.setFieldMetadata(newIDs, nil)
}

// relabelPointer turns a unary_expression ("*" operand) into a pointer_type
// wrapping the operand retagged as a type. pointer_type carries no child
// fields, so the "operator"/"operand" field ids are cleared.
func (c *goNewMakeTypeRetagCtx) relabelPointer(n *Node) {
	operand := n.ChildByFieldName("operand", c.lang)
	n.symbol = c.pointerTypeSym
	n.setNamed(c.pointerTypeNamed)
	n.clearFieldMetadata()
	if operand != nil {
		c.relabel(operand)
	}
}

// relabelParen turns a parenthesized_expression into a parenthesized_type and
// retags its inner expression as a type. Both nodes carry no child fields, so
// only symbols and named-ness change.
func (c *goNewMakeTypeRetagCtx) relabelParen(n *Node) {
	inner := c.parenInner(n)
	n.symbol = c.parenTypeSym
	n.setNamed(c.parenTypeNamed)
	if inner != nil {
		c.relabel(inner)
	}
}

// goCompatMemoryBudgetStopReason forces a real (unmasked) memory-budget check
// for the Go compat pipeline's stage boundaries in
// normalizeGoReturnedTreeCompatibilityWithCensus. It is called a small, fixed number of
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
	root, _ = flattenInvisibleRootChildrenWithTriviaHint(root, arena, lang, nil)
	return root
}

func flattenInvisibleRootChildrenWithTriviaHint(
	root *Node,
	arena *nodeArena,
	lang *Language,
	source []byte,
) (*Node, bool) {
	if root == nil || lang == nil || resultChildCount(root) == 0 {
		return root, false
	}
	symbolMeta := lang.SymbolMetadata
	changed := false
	hasHiddenTrivia := false
	childCount := resultChildCount(root)
	for i := 0; i < childCount; i++ {
		child := resultChildAt(root, i)
		if shouldFlattenInvisibleRootChild(child, symbolMeta) {
			changed = true
		}
		if len(source) > 0 && rootNodeIsHiddenExtraTrivia(root, child, source, lang) {
			hasHiddenTrivia = true
		}
		if changed && hasHiddenTrivia {
			break
		}
	}
	if !changed {
		return root, hasHiddenTrivia
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
	if len(source) == 0 {
		return root, false
	}
	hasHiddenTrivia = false
	for _, child := range root.children {
		if rootNodeIsHiddenExtraTrivia(root, child, source, lang) {
			hasHiddenTrivia = true
			break
		}
	}
	return root, hasHiddenTrivia
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
