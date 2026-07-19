package gotreesitter

// queryNodeReader is the compile-time-specialized syntax-tree seam used by
// the non-streaming matcher. N and C are value types selected by the caller;
// concrete readers must not manufacture public *Node proxies for other tree
// backings.
//
// The type parameter keeps the hot matcher monomorphic. This is intentionally
// not a stored interface: reader calls do not box nodes while walking a tree.
type queryNodeReader[N comparable, C any] interface {
	IsNil(N) bool
	Symbol(N) Symbol
	IsNamed(N) bool
	IsMissing(N) bool
	Type(N, *Language) string
	StartByte(N) uint32
	EndByte(N) uint32
	Text(N, []byte) string
	Parent(N) (N, bool)
	ChildCount(N) int
	Child(N, int) (N, bool)
	ChildNamed(N, int) (bool, bool)
	ChildCanMatch(*Query, *QueryStep, N, int, *Language) bool
	ChildHasField(N, int, FieldID, *Language) bool
	HasChildWithField(N, FieldID, *Language) bool

	NewCapture(string, N) C
	CaptureName(C) string
	CaptureNode(C) N
	CaptureTextOverride(C) string
	SetCaptureTextOverride(*C, string)
}

// queryValueCapture is the compact-consumer capture representation. It keeps
// the reader's node value directly and therefore never requires a public Node
// proxy. Readers for alternate immutable stores can use this type unchanged.
type queryValueCapture[N comparable] struct {
	Name         string
	Node         N
	TextOverride string
}

type queryReaderMatch[C any] struct {
	PatternIndex int
	Captures     []C
}

type queryReaderWorkItem[N comparable] struct {
	node     N
	parent   N
	childIdx int
	post     bool
}

// executeQueryWithReader is the sole non-streaming matcher traversal. dst and
// worklist are caller-owned reusable buffers; returned captures hold reader
// value nodes directly.
func executeQueryWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, root N, lang *Language, source []byte, reader R, dst []queryReaderMatch[C], worklist []queryReaderWorkItem[N]) ([]queryReaderMatch[C], []queryReaderWorkItem[N]) {
	dst = dst[:0]
	worklist = worklist[:0]
	if q == nil || lang == nil || reader.IsNil(root) {
		return dst, worklist
	}
	if q.rootCandidatesBySymbol == nil && q.rootFallbackCandidates == nil {
		q.buildRootPatternIndex()
	}
	worklist = append(worklist, queryReaderWorkItem[N]{node: root, childIdx: -1})
	for len(worklist) > 0 {
		last := len(worklist) - 1
		item := worklist[last]
		worklist = worklist[:last]
		node := item.node
		if reader.IsNil(node) {
			continue
		}
		if item.post {
			candidates := q.postorderPatternCandidates(lang.PublicSymbolForNamedness(reader.Symbol(node), reader.IsNamed(node)))
			candidates = mergePatternIndexLists(q.rootRepetitionPostPatterns, candidates)
			for _, patternIndex := range candidates {
				if q.isPatternDisabled(patternIndex) {
					continue
				}
				pattern := &q.patterns[patternIndex]
				budget := newQueryMatchBudget(defaultQueryMatchWorkBudget)
				var captureSets [][]C
				if pattern.steps[0].quantifier == queryQuantifierZeroOrMore || pattern.steps[0].quantifier == queryQuantifierOneOrMore {
					captureSets = matchPatternPostorderAllWithReader(q, pattern, node, item.parent, item.childIdx, lang, source, budget, reader)
				} else {
					captureSets = matchPatternAllWithReader(q, pattern, node, lang, source, budget, reader)
				}
				for _, captures := range captureSets {
					dst = append(dst, queryReaderMatch[C]{PatternIndex: patternIndex, Captures: captures})
				}
			}
			continue
		}
		if q.hasPostorderPatterns() {
			worklist = append(worklist, queryReaderWorkItem[N]{node: node, parent: item.parent, childIdx: item.childIdx, post: true})
		}
		for index := reader.ChildCount(node) - 1; index >= 0; index-- {
			child, ok := reader.Child(node, index)
			if ok {
				worklist = append(worklist, queryReaderWorkItem[N]{node: child, parent: node, childIdx: index})
			}
		}
		for _, patternIndex := range q.rootPatternCandidates(lang.PublicSymbolForNamedness(reader.Symbol(node), reader.IsNamed(node))) {
			if q.isPatternDisabled(patternIndex) {
				continue
			}
			pattern := &q.patterns[patternIndex]
			captures := matchPatternAllWithReader(q, pattern, node, lang, source, newQueryMatchBudget(defaultQueryMatchWorkBudget), reader)
			for _, set := range captures {
				dst = append(dst, queryReaderMatch[C]{PatternIndex: patternIndex, Captures: set})
			}
		}
	}
	return dst, worklist
}

// publicQueryReader preserves the public Node/QueryCapture representation as
// one concrete specialization of the shared matcher.
type publicQueryReader struct{}

func (publicQueryReader) IsNil(node *Node) bool                  { return node == nil }
func (publicQueryReader) Symbol(node *Node) Symbol               { return node.Symbol() }
func (publicQueryReader) IsNamed(node *Node) bool                { return node.IsNamed() }
func (publicQueryReader) IsMissing(node *Node) bool              { return node.IsMissing() }
func (publicQueryReader) Type(node *Node, lang *Language) string { return node.Type(lang) }
func (publicQueryReader) StartByte(node *Node) uint32            { return node.StartByte() }
func (publicQueryReader) EndByte(node *Node) uint32              { return node.EndByte() }
func (publicQueryReader) Text(node *Node, source []byte) string  { return node.Text(source) }
func (publicQueryReader) Parent(node *Node) (*Node, bool) {
	if node == nil {
		return nil, false
	}
	parent := node.Parent()
	return parent, parent != nil
}
func (publicQueryReader) ChildCount(node *Node) int {
	return nodeChildCountNoMaterialize(node)
}
func (publicQueryReader) Child(node *Node, index int) (*Node, bool) {
	entry, ok := nodeChildEntryAtNoMaterialize(node, index)
	if !ok {
		return nil, false
	}
	if child := stackEntryNode(entry); child != nil {
		return child, true
	}
	child := nodeChildAtForReason(node, index, materializeForQuery)
	return child, child != nil
}
func (publicQueryReader) ChildNamed(node *Node, index int) (bool, bool) {
	entry, ok := nodeChildEntryAtNoMaterialize(node, index)
	return ok && stackEntryNodeIsNamed(entry), ok
}
func (publicQueryReader) ChildCanMatch(q *Query, step *QueryStep, parent *Node, index int, lang *Language) bool {
	entry, ok := nodeChildEntryAtNoMaterialize(parent, index)
	return ok && q.stackEntryCanMatchStep(step, entry, lang)
}
func (publicQueryReader) ChildHasField(parent *Node, index int, field FieldID, lang *Language) bool {
	if field == 0 {
		return true
	}
	if parent == nil || lang == nil || int(field) <= 0 || int(field) >= len(lang.FieldNames) {
		return false
	}
	name := lang.FieldNames[field]
	return name != "" && parent.FieldNameForChild(index, lang) == name
}
func (r publicQueryReader) HasChildWithField(parent *Node, field FieldID, lang *Language) bool {
	if parent == nil {
		return false
	}
	for index := 0; index < r.ChildCount(parent); index++ {
		if r.ChildHasField(parent, index, field, lang) {
			return true
		}
	}
	return false
}
func (publicQueryReader) NewCapture(name string, node *Node) QueryCapture {
	return QueryCapture{Name: name, Node: node}
}
func (publicQueryReader) CaptureName(capture QueryCapture) string { return capture.Name }
func (publicQueryReader) CaptureNode(capture QueryCapture) *Node  { return capture.Node }
func (publicQueryReader) CaptureTextOverride(capture QueryCapture) string {
	return capture.TextOverride
}
func (publicQueryReader) SetCaptureTextOverride(capture *QueryCapture, text string) {
	capture.TextOverride = text
}
