package gotreesitter

import (
	"fmt"
	"os"
	"strconv"
)

const defaultRawShapeDiagChildLimit = 40
const defaultRawShapeDiagDepthLimit = 3

func rawShapeDiagEnabled() bool {
	return os.Getenv("GOT_GLR_RAW_SHAPE_DIAG") == "1"
}

func rawShapeDiagChildLimit() int {
	limit := defaultRawShapeDiagChildLimit
	if raw := os.Getenv("GOT_GLR_RAW_SHAPE_DIAG_CHILD_LIMIT"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	return limit
}

func rawShapeDiagDepthLimit() int {
	limit := defaultRawShapeDiagDepthLimit
	if raw := os.Getenv("GOT_GLR_RAW_SHAPE_DIAG_DEPTH"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			limit = n
		}
	}
	return limit
}

func (p *Parser) emitRawShapeDiag(phase string, stacks []glrStack, arena *nodeArena) {
	if !rawShapeDiagEnabled() {
		return
	}
	childLimit := rawShapeDiagChildLimit()
	depthLimit := rawShapeDiagDepthLimit()
	acceptedCount := 0
	for i := range stacks {
		if stacks[i].accepted {
			acceptedCount++
		}
	}
	fmt.Fprintf(os.Stderr, "GLR-RAW-SHAPE phase=%s stacks=%d accepted=%d child_limit=%d depth_limit=%d\n", phase, len(stacks), acceptedCount, childLimit, depthLimit)
	for i := range stacks {
		s := &stacks[i]
		if !s.accepted {
			continue
		}
		entries, total := rawShapeDiagResultEntries(s, childLimit)
		fmt.Fprintf(os.Stderr, "GLR-RAW-SHAPE stack=%d accepted=%t dead=%t state=%d byte=%d depth=%d score=%d dyn=%d err_rank=%d roots=%d roots_emitted=%d\n",
			i,
			s.accepted,
			s.dead,
			rawShapeDiagStackState(s),
			s.byteOffset,
			s.depth(),
			s.score,
			stackResultDynamicPrecedence(s),
			stackResultErrorRank(s, arena),
			total,
			len(entries),
		)
		for rootIdx, entry := range entries {
			p.emitRawShapeDiagEntry(arena, fmt.Sprintf("stack=%d root=%d", i, rootIdx), entry, childLimit, depthLimit, 0)
		}
		if total > len(entries) {
			fmt.Fprintf(os.Stderr, "GLR-RAW-SHAPE stack=%d roots_truncated=%d total=%d\n", i, total-len(entries), total)
		}
	}
}

func rawShapeDiagStackState(s *glrStack) StateID {
	if s == nil || s.depth() == 0 {
		return 0
	}
	return s.top().state
}

func rawShapeDiagResultEntries(s *glrStack, limit int) ([]stackEntry, int) {
	if limit <= 0 {
		limit = defaultRawShapeDiagChildLimit
	}
	entries := make([]stackEntry, 0, limit)
	total := 0
	appendEntry := func(entry stackEntry) {
		if !stackEntryMaterializesForResult(entry) {
			return
		}
		total++
		if len(entries) < limit {
			entries = append(entries, entry)
		}
	}
	if len(s.entries) > 0 {
		for i := range s.entries {
			appendEntry(s.entries[i])
		}
		return entries, total
	}
	count := stackMaterializingResultEntryCount(s)
	if count == 0 {
		return nil, 0
	}
	buf := make([]stackEntry, count)
	if materialized, ok := stackMaterializingResultEntries(s, buf[:0], count); ok {
		for i := range materialized {
			appendEntry(materialized[i])
		}
	}
	return entries, total
}

func (p *Parser) emitRawShapeDiagEntry(arena *nodeArena, label string, entry stackEntry, childLimit, depthLimit, depth int) {
	childCount := rawShapeDiagEntryChildCount(arena, entry)
	shapeRef := stackEntryRawShapeRef(entry)
	shapeProductionID := uint16(0)
	shapeChildCount := -1
	if shape, ok := rawShapeForStackEntry(arena, entry); ok {
		shapeProductionID = shape.productionID
		shapeChildCount = shape.childCount()
	}
	fmt.Fprintf(os.Stderr,
		"GLR-RAW-SHAPE %s depth=%d symbol=%d name=%s span=%d:%d children=%d shape_ref=%d shape_pid=%d shape_children=%d named=%t meta_named=%t visible=%t hidden=%t invisible=%t extra=%t missing=%t error=%t kind=%s\n",
		label,
		depth,
		stackEntryNodeSymbol(entry),
		strconv.Quote(p.rawShapeDiagSymbolName(stackEntryNodeSymbol(entry))),
		stackEntryNodeStartByte(entry),
		stackEntryNodeEndByte(entry),
		childCount,
		shapeRef,
		shapeProductionID,
		shapeChildCount,
		stackEntryNodeIsNamed(entry),
		p.rawShapeDiagSymbolNamed(stackEntryNodeSymbol(entry)),
		p.rawShapeDiagSymbolVisible(stackEntryNodeSymbol(entry)),
		!p.rawShapeDiagSymbolVisible(stackEntryNodeSymbol(entry)),
		!p.rawShapeDiagSymbolVisible(stackEntryNodeSymbol(entry)),
		stackEntryNodeIsExtra(entry),
		stackEntryNodeIsMissing(entry),
		stackEntryNodeHasError(entry),
		rawShapeDiagEntryKind(entry),
	)
	emitCount := childCount
	if emitCount > childLimit {
		emitCount = childLimit
	}
	for i := 0; i < emitCount; i++ {
		childLabel := fmt.Sprintf("%s child=%d", label, i)
		child, ok := rawShapeDiagChildAt(arena, entry, i)
		if !ok {
			fmt.Fprintf(os.Stderr, "GLR-RAW-SHAPE %s depth=%d <unavailable>\n", childLabel, depth+1)
			continue
		}
		childShapeRef := stackEntryRawShapeRef(child)
		fmt.Fprintf(os.Stderr,
			"GLR-RAW-SHAPE %s depth=%d symbol=%d name=%s span=%d:%d children=%d shape_ref=%d named=%t meta_named=%t visible=%t hidden=%t invisible=%t extra=%t missing=%t error=%t kind=%s\n",
			childLabel,
			depth+1,
			stackEntryNodeSymbol(child),
			strconv.Quote(p.rawShapeDiagSymbolName(stackEntryNodeSymbol(child))),
			stackEntryNodeStartByte(child),
			stackEntryNodeEndByte(child),
			rawShapeDiagEntryChildCount(arena, child),
			childShapeRef,
			stackEntryNodeIsNamed(child),
			p.rawShapeDiagSymbolNamed(stackEntryNodeSymbol(child)),
			p.rawShapeDiagSymbolVisible(stackEntryNodeSymbol(child)),
			!p.rawShapeDiagSymbolVisible(stackEntryNodeSymbol(child)),
			!p.rawShapeDiagSymbolVisible(stackEntryNodeSymbol(child)),
			stackEntryNodeIsExtra(child),
			stackEntryNodeIsMissing(child),
			stackEntryNodeHasError(child),
			rawShapeDiagEntryKind(child),
		)
		if depth+1 < depthLimit && rawShapeDiagEntryChildCount(arena, child) > 0 {
			p.emitRawShapeDiagEntry(arena, childLabel, child, childLimit, depthLimit, depth+1)
		}
	}
	if childCount > emitCount {
		fmt.Fprintf(os.Stderr, "GLR-RAW-SHAPE %s children_truncated=%d total=%d\n", label, childCount-emitCount, childCount)
	}
}

func rawShapeDiagEntryChildCount(arena *nodeArena, entry stackEntry) int {
	if shape, ok := rawShapeForStackEntry(arena, entry); ok {
		return shape.childCount()
	}
	return stackEntryNodeChildCount(entry)
}

func rawShapeDiagChildAt(arena *nodeArena, entry stackEntry, i int) (stackEntry, bool) {
	if shape, ok := rawShapeForStackEntry(arena, entry); ok {
		children := arena.rawShapeChildren(shape)
		if i < 0 || i >= len(children) {
			return stackEntry{}, false
		}
		child := children[i].entry()
		return child, stackEntryHasNode(child)
	}
	if node := stackEntryNode(entry); node != nil {
		return nodeChildEntryAtNoMaterialize(node, i)
	}
	if parent := stackEntryPendingParent(entry); parent != nil {
		if i < 0 || i >= parent.childEntryCount() {
			return stackEntry{}, false
		}
		child := parent.childEntry(arena, i)
		return child, stackEntryHasNode(child)
	}
	return stackEntry{}, false
}

func rawShapeDiagEntryKind(entry stackEntry) string {
	switch {
	case stackEntryNode(entry) != nil:
		return "node"
	case stackEntryNoTreeNode(entry) != nil:
		return "no_tree"
	case stackEntryCompactFullLeaf(entry) != nil:
		return "compact_leaf"
	case stackEntryPendingParent(entry) != nil:
		return "pending_parent"
	default:
		return "unknown"
	}
}

func (p *Parser) rawShapeDiagSymbolName(sym Symbol) string {
	if p == nil || p.language == nil {
		return ""
	}
	if idx := int(sym); idx >= 0 && idx < len(p.language.SymbolMetadata) {
		if name := p.language.SymbolMetadata[idx].Name; name != "" {
			return name
		}
	}
	if idx := int(sym); idx >= 0 && idx < len(p.language.SymbolNames) {
		return unescapePunctuationSymbolName(p.language.SymbolNames[idx])
	}
	return ""
}

func (p *Parser) rawShapeDiagSymbolNamed(sym Symbol) bool {
	if p == nil || p.language == nil {
		return false
	}
	if idx := int(sym); idx >= 0 && idx < len(p.language.SymbolMetadata) {
		return p.language.SymbolMetadata[idx].Named
	}
	return false
}

func (p *Parser) rawShapeDiagSymbolVisible(sym Symbol) bool {
	if p == nil || p.language == nil {
		return false
	}
	if idx := int(sym); idx >= 0 && idx < len(p.language.SymbolMetadata) {
		return p.language.SymbolMetadata[idx].Visible
	}
	return false
}
