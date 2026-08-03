package gotreesitter

import (
	"sort"
	"strings"
)

// outlineDefinitionPrefix is the tags-query capture prefix that marks a
// definition. It matches the convention Tagger uses (see extractTag).
const outlineDefinitionPrefix = "definition."

// outlineNameCapture is the tags-query capture that carries a definition name.
const outlineNameCapture = "name"

// outlineKindTable is the one fixed, language-neutral normalization table for
// definition kinds. The key is the "@definition.X" capture suffix; the value is
// the normalized Kind. The table is keyed by capture data, never by language
// name, so a new language needs no code change.
//
// A suffix that is not listed passes through unchanged. New tags data
// therefore works immediately, and the outline never invents a kind.
var outlineKindTable = map[string]string{
	"function":    "function",
	"method":      "method",
	"class":       "class",
	"interface":   "interface",
	"type":        "type",
	"constructor": "constructor",
	"constant":    "constant",
	"variable":    "variable",
	"module":      "module",
}

// outlineKindRefinement is the second fixed table. It refines a kind by the
// grammar node type when the shared tags patterns cannot express the
// distinction. The key is the node type; the value is the refined kind.
//
// A row applies only when the capture already resolved to one of the kinds in
// outlineRefinableKinds, so a refinement can never overrule a more specific
// capture. Node types are cross-language data, not language names: every
// grammar that spells an enumeration "enum_declaration" gets the same answer.
var outlineKindRefinement = map[string]string{
	"enum_declaration":   "enum",
	"enum_item":          "enum",
	"record_declaration": "record",
}

// outlineRefinableKinds lists the kinds a node-type refinement may replace.
var outlineRefinableKinds = map[string]bool{
	"type":  true,
	"class": true,
}

// Outliner projects a language-neutral file outline from a parsed tree. It
// compiles one tags query and reuses it across trees.
//
// The outliner is a read-only projection. It runs no parse, it changes no
// tree, and it holds no parser state. Build one per language and call
// OutlineTree with trees that language produced.
//
// Outliner is not safe for concurrent use.
type Outliner struct {
	lang       *Language
	query      *Query
	queryEmpty bool

	ownerRules map[string][]OutlineOwnerRule

	hasMatchLimit bool
	matchLimit    uint32
	hasWorkBudget bool
	workBudget    int
}

// OutlinerOption configures an Outliner.
type OutlinerOption func(*Outliner)

// WithOutlineOwnerRules attaches declarative owner rules, indexed by node
// type. The grammars package owns the per-language rows; the core holds none.
//
// Owner resolution is NOT applied in this change. The outliner validates and
// retains the rules, every OutlineSymbol.Owner stays empty, and
// OutlineReport.OwnerRuleMisses stays zero. Owner reads a child field name, and
// field-name projection became trustworthy only after the field-projection
// alignment change; ownership therefore ships in its own change, behind its own
// differential gate.
//
// A rule with an empty NodeType or an empty OwnerField is rejected at
// construction, because such a rule can never resolve an owner.
func WithOutlineOwnerRules(rules []OutlineOwnerRule) OutlinerOption {
	return func(o *Outliner) {
		if len(rules) == 0 {
			return
		}
		if o.ownerRules == nil {
			o.ownerRules = make(map[string][]OutlineOwnerRule, len(rules))
		}
		for _, rule := range rules {
			o.ownerRules[rule.NodeType] = append(o.ownerRules[rule.NodeType], rule)
		}
	}
}

// WithOutlineMatchLimit bounds the number of query matches the outliner
// accepts. When the limit is reached, OutlineReport.Truncated reports true and
// the symbol list is partial. A limit of zero keeps the query engine default.
func WithOutlineMatchLimit(limit uint32) OutlinerOption {
	return func(o *Outliner) {
		o.hasMatchLimit = true
		o.matchLimit = limit
	}
}

// WithOutlineMatchWorkBudget bounds the enumeration steps the matcher may take
// for each pattern and node. Exhausting the budget sets
// OutlineReport.Truncated. Raise it for very large files whose outline comes
// back truncated. A budget of zero means unlimited.
func WithOutlineMatchWorkBudget(budget int) OutlinerOption {
	return func(o *Outliner) {
		o.hasWorkBudget = true
		o.workBudget = budget
	}
}

// NewOutliner creates an Outliner for a language and its tags query.
//
// An empty or blank tags query is not an error. The outliner declines: it
// returns no symbols and sets OutlineReport.QueryEmpty, so a language without
// tags data is observably uncovered instead of silently empty.
func NewOutliner(lang *Language, tagsQuery string, opts ...OutlinerOption) (*Outliner, error) {
	if lang == nil {
		return nil, errOutlineNilLanguage
	}

	o := &Outliner{lang: lang}
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}
	if err := validateOutlineOwnerRules(o.ownerRules); err != nil {
		return nil, err
	}

	if strings.TrimSpace(tagsQuery) == "" {
		o.queryEmpty = true
		return o, nil
	}

	q, err := NewQuery(tagsQuery, lang)
	if err != nil {
		return nil, err
	}
	o.query = q
	return o, nil
}

// Language returns the language this outliner was built for.
func (o *Outliner) Language() *Language {
	if o == nil {
		return nil
	}
	return o.lang
}

// QueryEmpty reports whether the outliner declined for want of tags data.
func (o *Outliner) QueryEmpty() bool {
	return o != nil && o.queryEmpty
}

// OutlineTree projects the outline of an already-parsed tree.
//
// The call is read-only: it runs the tags query over the tree, normalizes the
// kinds, drops ambiguous candidates, and assembles the containment forest. It
// parses nothing and mutates nothing.
//
// Trees that hold ERROR or MISSING nodes are not special-cased. The outline is
// the projection of whatever the tags query matched on the tree as the parser
// produced it. Definitions the parser could not recover simply do not appear.
// The result stays well-defined: every candidate is emitted or counted.
func (o *Outliner) OutlineTree(tree *Tree) ([]OutlineSymbol, OutlineReport) {
	var report OutlineReport
	if o == nil {
		return nil, report
	}
	report.QueryEmpty = o.queryEmpty
	if o.queryEmpty || o.query == nil {
		return nil, report
	}
	if tree == nil {
		return nil, report
	}
	root := tree.RootNode()
	if root == nil {
		return nil, report
	}
	if !o.treeLanguageMatches(tree) {
		report.LanguageMismatch = true
		return nil, report
	}

	candidates, truncated := o.collectOutlineCandidates(root, tree.Source())
	report.Truncated = truncated

	kept := filterOutlineCandidates(candidates, &report)
	symbols := buildOutlineForest(kept, &report)
	report.Symbols = countOutlineSymbols(symbols)
	return symbols, report
}

// treeLanguageMatches reports whether the tree came from the same language the
// query compiled against. Query symbol identifiers are language specific, so
// running a query over a foreign tree yields nonsense; the outliner declines
// instead.
func (o *Outliner) treeLanguageMatches(tree *Tree) bool {
	treeLang := tree.Language()
	if treeLang == nil {
		return false
	}
	if treeLang == o.lang {
		return true
	}
	return treeLang.Name != "" && treeLang.Name == o.lang.Name
}

// outlineCandidate is one definition capture paired with its name capture,
// before the ambiguity rules run.
type outlineCandidate struct {
	// order is the emission index of the match. It is the deterministic
	// tie-break for candidates that are otherwise identical.
	order     int
	kind      string
	name      string
	nodeType  string
	rng       Range
	nameRange Range
}

// collectOutlineCandidates runs the tags query and reduces each match to at
// most one candidate.
//
// Capture handling mirrors Tagger.extractTag so the outline and the tagger
// always agree on the same query: the last "@name" capture and the last
// "@definition.X" capture in a match win. Matches that carry no definition
// capture -- "@reference.X" matches, for instance -- are discarded without a
// counter, because they are not outline candidates at all.
func (o *Outliner) collectOutlineCandidates(root *Node, source []byte) ([]outlineCandidate, bool) {
	cursor := o.query.Exec(root, o.lang, source)
	if cursor == nil {
		return nil, false
	}
	if o.hasMatchLimit {
		cursor.SetMatchLimit(o.matchLimit)
	}
	if o.hasWorkBudget {
		cursor.SetMatchWorkBudget(o.workBudget)
	}

	var candidates []outlineCandidate
	order := 0
	for {
		match, ok := cursor.NextMatch()
		if !ok {
			break
		}

		var (
			defCapture  string
			defNode     *Node
			hasName     bool
			nameText    string
			nameSpan    Range
			hasDefNode  bool
			nameCapture QueryCapture
		)
		for _, capture := range match.Captures {
			switch {
			case capture.Name == outlineNameCapture:
				if capture.Node == nil {
					continue
				}
				nameCapture = capture
				hasName = true
				nameText = nameCapture.Text(source)
				nameSpan = capture.Node.Range()
			case isOutlineDefinitionCapture(capture.Name):
				if capture.Node == nil {
					continue
				}
				defCapture = capture.Name
				defNode = capture.Node
				hasDefNode = true
			}
		}
		if !hasDefNode {
			continue
		}

		candidate := outlineCandidate{
			order:    order,
			kind:     normalizeOutlineKind(defCapture, defNode.Type(o.lang)),
			nodeType: defNode.Type(o.lang),
			rng:      defNode.Range(),
		}
		if hasName {
			candidate.name = nameText
			candidate.nameRange = nameSpan
		}
		candidates = append(candidates, candidate)
		order++
	}

	return candidates, cursor.DidExceedMatchLimit()
}

// isOutlineDefinitionCapture reports whether a capture name marks a definition
// and carries a non-empty kind suffix.
func isOutlineDefinitionCapture(name string) bool {
	return len(name) > len(outlineDefinitionPrefix) &&
		strings.HasPrefix(name, outlineDefinitionPrefix)
}

// normalizeOutlineKind maps a "@definition.X" capture name to a normalized
// kind through the two fixed tables. It reads capture data and node types
// only; it never reads the language name.
func normalizeOutlineKind(captureName, nodeType string) string {
	suffix := strings.TrimPrefix(captureName, outlineDefinitionPrefix)
	kind := suffix
	if mapped, ok := outlineKindTable[suffix]; ok {
		kind = mapped
	}
	if outlineRefinableKinds[kind] {
		if refined, ok := outlineKindRefinement[nodeType]; ok {
			kind = refined
		}
	}
	return kind
}

// filterOutlineCandidates applies the ambiguity rules and returns the
// survivors in candidate order. Every dropped candidate lands in exactly one
// counter.
//
// "Ambiguous", operationally, is any of:
//
//  1. no usable name;
//  2. a name span that is not inside the definition span;
//  3. a repeat of an accepted (Range, Kind) pair;
//  4. one span carrying two different kinds;
//  5. a span that partially overlaps an accepted span (rule 5 runs in
//     buildOutlineForest, where the containment forest is assembled).
func filterOutlineCandidates(candidates []outlineCandidate, report *OutlineReport) []outlineCandidate {
	if len(candidates) == 0 {
		return nil
	}

	// Rules 1 and 2: per-candidate validity.
	valid := make([]outlineCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		candidate.name = strings.TrimSpace(candidate.name)
		if candidate.name == "" {
			report.OmittedNoName++
			continue
		}
		if !outlineRangeContains(candidate.rng, candidate.nameRange) {
			report.OmittedInvalidNameRange++
			continue
		}
		valid = append(valid, candidate)
	}
	if len(valid) == 0 {
		return nil
	}

	// Rules 3 and 4: group by exact span.
	type spanKey struct{ start, end uint32 }
	groups := make(map[spanKey][]int, len(valid))
	spanOrder := make([]spanKey, 0, len(valid))
	for i, candidate := range valid {
		key := spanKey{start: candidate.rng.StartByte, end: candidate.rng.EndByte}
		if _, seen := groups[key]; !seen {
			spanOrder = append(spanOrder, key)
		}
		groups[key] = append(groups[key], i)
	}

	kept := make([]outlineCandidate, 0, len(valid))
	for _, key := range spanOrder {
		members := groups[key]
		conflict := false
		for _, idx := range members[1:] {
			if valid[idx].kind != valid[members[0]].kind {
				conflict = true
				break
			}
		}
		if conflict {
			// Rule 4: the span means two things at once. Drop the whole
			// group and count every member. Do not guess.
			report.OmittedConflict += len(members)
			continue
		}
		// Rule 3: keep the first member in emission order.
		best := members[0]
		for _, idx := range members[1:] {
			if valid[idx].order < valid[best].order {
				best = idx
			}
		}
		report.OmittedDuplicate += len(members) - 1
		kept = append(kept, valid[best])
	}

	return kept
}

// outlineRangeContains reports whether inner sits inside outer, by bytes. A
// span equal to outer counts as contained. An inverted outer span contains
// nothing.
func outlineRangeContains(outer, inner Range) bool {
	if outer.EndByte < outer.StartByte {
		return false
	}
	if inner.EndByte < inner.StartByte {
		return false
	}
	return inner.StartByte >= outer.StartByte && inner.EndByte <= outer.EndByte
}

// buildOutlineForest assembles the containment forest.
//
// It sorts by start byte ascending, then end byte descending, so a container
// always precedes everything it contains. It then walks the sorted list with a
// stack: pop while the top does not reach the current span, and the remaining
// top is the parent. A span that starts inside the top but ends after it
// overlaps without containing -- rule 5 -- so it is dropped and counted.
//
// Materialization runs in reverse index order. Every child has a higher sorted
// index than its parent, so each parent finds its children already built. The
// pass is iterative and allocates one slice per parent that has children.
func buildOutlineForest(candidates []outlineCandidate, report *OutlineReport) []OutlineSymbol {
	if len(candidates) == 0 {
		return nil
	}

	sorted := make([]outlineCandidate, len(candidates))
	copy(sorted, candidates)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].rng.StartByte != sorted[j].rng.StartByte {
			return sorted[i].rng.StartByte < sorted[j].rng.StartByte
		}
		if sorted[i].rng.EndByte != sorted[j].rng.EndByte {
			return sorted[i].rng.EndByte > sorted[j].rng.EndByte
		}
		return sorted[i].order < sorted[j].order
	})

	accepted := make([]outlineCandidate, 0, len(sorted))
	parentOf := make([]int, 0, len(sorted))
	childrenOf := make([][]int, 0, len(sorted))
	stack := make([]int, 0, 16)

	for _, candidate := range sorted {
		for len(stack) > 0 {
			top := accepted[stack[len(stack)-1]]
			if candidate.rng.StartByte < top.rng.EndByte {
				break
			}
			stack = stack[:len(stack)-1]
		}

		parent := -1
		if len(stack) > 0 {
			topIdx := stack[len(stack)-1]
			top := accepted[topIdx]
			if candidate.rng.EndByte > top.rng.EndByte {
				// Rule 5: partial overlap. The forest has no well-defined
				// shape here, so drop the later start.
				report.OmittedOverlap++
				continue
			}
			parent = topIdx
		}

		idx := len(accepted)
		accepted = append(accepted, candidate)
		parentOf = append(parentOf, parent)
		childrenOf = append(childrenOf, nil)
		if parent >= 0 {
			childrenOf[parent] = append(childrenOf[parent], idx)
		}
		stack = append(stack, idx)
	}

	built := make([]OutlineSymbol, len(accepted))
	for idx := len(accepted) - 1; idx >= 0; idx-- {
		candidate := accepted[idx]
		symbol := OutlineSymbol{
			Kind:      candidate.kind,
			Name:      candidate.name,
			NodeType:  candidate.nodeType,
			Range:     candidate.rng,
			NameRange: candidate.nameRange,
		}
		if kids := childrenOf[idx]; len(kids) > 0 {
			symbol.Children = make([]OutlineSymbol, 0, len(kids))
			for _, kid := range kids {
				symbol.Children = append(symbol.Children, built[kid])
			}
		}
		built[idx] = symbol
	}

	roots := make([]OutlineSymbol, 0, len(accepted))
	for idx := range accepted {
		if parentOf[idx] == -1 {
			roots = append(roots, built[idx])
		}
	}
	return roots
}

// countOutlineSymbols counts a forest, nested symbols included.
func countOutlineSymbols(symbols []OutlineSymbol) int {
	total := 0
	stack := make([][]OutlineSymbol, 0, 8)
	stack = append(stack, symbols)
	for len(stack) > 0 {
		level := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		total += len(level)
		for i := range level {
			if len(level[i].Children) > 0 {
				stack = append(stack, level[i].Children)
			}
		}
	}
	return total
}

// validateOutlineOwnerRules rejects rules that can never resolve an owner.
func validateOutlineOwnerRules(rules map[string][]OutlineOwnerRule) error {
	for nodeType, rows := range rules {
		if strings.TrimSpace(nodeType) == "" {
			return errOutlineOwnerRuleNodeType
		}
		for _, row := range rows {
			if strings.TrimSpace(row.NodeType) == "" {
				return errOutlineOwnerRuleNodeType
			}
			if strings.TrimSpace(row.OwnerField) == "" {
				return errOutlineOwnerRuleField
			}
		}
	}
	return nil
}

type outlineError string

func (e outlineError) Error() string { return string(e) }

const (
	errOutlineNilLanguage       outlineError = "outline: language is nil"
	errOutlineOwnerRuleNodeType outlineError = "outline: owner rule needs a node type"
	errOutlineOwnerRuleField    outlineError = "outline: owner rule needs an owner field"
)
