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
// An Outliner is safe for concurrent use by several goroutines. OutlineTree
// writes no Outliner field, and every call takes its own query cursor and its
// own working slices. TestOutlineTreeIsSafeForConcurrentUse runs the shared
// path under the race detector and compares every result.
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
// A later call REPLACES the rules an earlier call set; the option does not
// accumulate.
//
// OutlineTree applies the attached rules to every symbol: a rule whose
// NodeType matches resolves OutlineSymbol.Owner when its OwnerField is
// present on the node and its Unwrap/NameTypes walk reaches exactly one
// accepted terminal node (resolveOutlineOwner, outline_owner.go). Any other
// outcome leaves Owner empty and, for a NodeType a rule did match, increments
// OutlineReport.OwnerRuleMisses. A NodeType no attached rule names never
// touches Owner or OwnerRuleMisses at all.
//
// What construction checks today: every rule must name a NodeType and an
// OwnerField, because a rule missing either can never resolve an owner.
//
// What construction does NOT check today: it accepts an empty NameTypes list,
// a blank entry inside Unwrap or NameTypes, and a NodeType or OwnerField the
// language does not define. Each of those fails closed at resolution time and
// inflates OwnerRuleMisses rather than producing a wrong Owner. A caller that
// wants those rows filtered out ahead of time, so a stale rule costs nothing
// at resolution either, gates its own table on symbol and field presence the
// way grammars.OutlineOwnerRules does.
func WithOutlineOwnerRules(rules []OutlineOwnerRule) OutlinerOption {
	return func(o *Outliner) {
		o.ownerRules = nil
		if len(rules) == 0 {
			return
		}
		o.ownerRules = make(map[string][]OutlineOwnerRule, len(rules))
		for _, rule := range rules {
			o.ownerRules[rule.NodeType] = append(o.ownerRules[rule.NodeType], rule)
		}
	}
}

// WithOutlineMatchLimit bounds the number of query matches the outliner
// accepts. When the limit is reached, OutlineReport.Truncated reports true and
// the symbol list is partial.
//
// A limit of zero keeps the query engine default, exactly as a budget of zero
// does in WithOutlineMatchWorkBudget. Zero always means "leave the default
// alone" in both options.
func WithOutlineMatchLimit(limit uint32) OutlinerOption {
	return func(o *Outliner) {
		o.hasMatchLimit = limit > 0
		o.matchLimit = limit
	}
}

// WithOutlineMatchWorkBudget bounds the enumeration steps the matcher may take
// for each pattern and node. Exhausting the budget sets
// OutlineReport.Truncated. Raise it for very large files whose outline comes
// back truncated.
//
// A budget of zero keeps the query engine default. This option cannot disable
// the guard: the underlying engine reads zero as "unlimited", and an outline
// caller writing zero means "default", so the option refuses to pass zero
// through. Removing the guard is not an outline concern.
func WithOutlineMatchWorkBudget(budget int) OutlinerOption {
	return func(o *Outliner) {
		o.hasWorkBudget = budget > 0
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

// DefinitionKinds returns the normalized kinds the compiled query can emit, in
// sorted order. It is derived from the query's capture names, so it states
// what the tags DATA can express for this language.
//
// Use it to read an outline honestly. A Go outline, for example, reports only
// "function" and "method", because the Go tags override carries no pattern for
// a type, a constant, or a variable. Those definitions never become candidates
// and never reach an omission counter, so an all-zero receipt does not mean the
// file held nothing else.
//
// The list is an upper bound on Kind values, not a promise that each appears.
// A node-type refinement can also map a capture to a kind outside this list;
// the refinement rows are documented on outlineKindRefinement.
func (o *Outliner) DefinitionKinds() []string {
	if o == nil || o.query == nil {
		return nil
	}
	seen := make(map[string]struct{})
	for _, name := range o.query.CaptureNames() {
		if !isOutlineDefinitionCapture(name) {
			continue
		}
		seen[normalizeOutlineKind(name, "")] = struct{}{}
	}
	kinds := make([]string, 0, len(seen))
	for kind := range seen {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}

// OutlineTree projects the outline of an already-parsed tree.
//
// The call is read-only: it runs the tags query over the tree, normalizes the
// kinds, drops ambiguous candidates, and assembles the containment forest. It
// parses nothing and mutates nothing.
//
// Trees that hold ERROR or MISSING nodes are not special-cased. The outline is
// the projection of whatever the tags query matched on the tree as the parser
// produced it. The receipt says so through OutlineReport.TreeHasError; read
// that field's documentation before trusting an outline over a damaged tree.
//
// When the outliner refuses to run, the returned report carries a
// DeclineReason and no symbols. An empty DeclineReason with no symbols is a
// different fact: the query ran and matched nothing.
func (o *Outliner) OutlineTree(tree *Tree) ([]OutlineSymbol, OutlineReport) {
	var report OutlineReport
	if o == nil {
		report.DeclineReason = OutlineDeclineNilOutliner
		return nil, report
	}
	if o.queryEmpty || o.query == nil {
		report.DeclineReason = OutlineDeclineQueryEmpty
		return nil, report
	}
	if tree == nil {
		report.DeclineReason = OutlineDeclineNilTree
		return nil, report
	}
	root := tree.RootNode()
	if root == nil {
		report.DeclineReason = OutlineDeclineNilRootNode
		return nil, report
	}
	if !o.treeLanguageMatches(tree) {
		report.DeclineReason = OutlineDeclineLanguageMismatch
		return nil, report
	}

	report.TreeHasError = root.HasError()

	source := tree.Source()
	candidates, truncated := o.collectOutlineCandidates(root, source, &report)
	report.Truncated = truncated

	kept := filterOutlineCandidates(candidates, &report)
	symbols := buildOutlineForest(kept, o.ownerRules, o.lang, source, &report)
	report.Symbols = countOutlineSymbols(symbols)
	return symbols, report
}

// OutlineBound projects an outline from a BoundTree.
// It uses the same validation, query, and receipt path as OutlineTree.
func (o *Outliner) OutlineBound(tree *BoundTree) ([]OutlineSymbol, OutlineReport) {
	if tree == nil {
		return o.OutlineTree(nil)
	}
	return o.OutlineTree(tree.tree)
}

// treeLanguageMatches reports whether the tree came from the same language the
// query compiled against. Query symbol identifiers are language specific, so
// running a query over a foreign tree yields nonsense; the outliner declines
// instead.
//
// The check is pointer identity first, then a structural comparison of the
// name together with the symbol, field, and state counts. Name alone is not
// enough: two distinct grammars can carry the same name, and a name-only
// check would run a foreign query with no receipt. The counts do not prove
// two languages are the same grammar, so this check catches a swapped
// language, not a regenerated one; a regenerated grammar with identical
// counts still recompiles the query at construction, which is where a real
// symbol change surfaces.
func (o *Outliner) treeLanguageMatches(tree *Tree) bool {
	treeLang := tree.Language()
	if treeLang == nil {
		return false
	}
	if treeLang == o.lang {
		return true
	}
	if treeLang.Name == "" || treeLang.Name != o.lang.Name {
		return false
	}
	return treeLang.SymbolCount == o.lang.SymbolCount &&
		treeLang.FieldCount == o.lang.FieldCount &&
		treeLang.StateCount == o.lang.StateCount
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
	// node is the captured definition node itself, kept only so a surviving
	// candidate can resolve its Owner once buildOutlineForest accepts it. No
	// filter or sort keys off this field, and no OutlineSymbol field repeats
	// it: symbols already spend Range for the same span, and Node exposes no
	// other data OutlineSymbol does not already carry through Kind, Name,
	// NodeType, Range, and NameRange.
	node *Node
}

// collectOutlineCandidates runs the tags query and reduces each match to at
// most one candidate.
//
// Capture handling mirrors Tagger.extractTag so the outline and the tagger
// agree on the same query: the last "@name" capture in a match wins. Matches
// that carry no definition capture -- "@reference.X" matches, for instance --
// are discarded without a counter, because they are not outline candidates at
// all.
//
// The one deliberate divergence from the tagger: a match carrying MORE THAN
// ONE "@definition.X" capture is dropped and counted in
// OmittedMultipleDefinitions. The tagger silently keeps the last such capture.
// For an outline that would pick one of two definitions by capture order, so
// the outline refuses. No inferred pattern produces this shape today.
func (o *Outliner) collectOutlineCandidates(root *Node, source []byte, report *OutlineReport) ([]outlineCandidate, bool) {
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
			defCapture string
			defNode    *Node
			defCount   int
			hasName    bool
			nameText   string
			nameSpan   Range
		)
		for _, capture := range match.Captures {
			if capture.Node == nil {
				continue
			}
			switch {
			case capture.Name == outlineNameCapture:
				hasName = true
				nameText = capture.Text(source)
				nameSpan = capture.Node.Range()
			case isOutlineDefinitionCapture(capture.Name):
				defCapture = capture.Name
				defNode = capture.Node
				defCount++
			}
		}
		if defCount == 0 {
			continue
		}
		if defCount > 1 {
			report.OmittedMultipleDefinitions++
			continue
		}

		nodeType := defNode.Type(o.lang)
		candidate := outlineCandidate{
			order:    order,
			kind:     normalizeOutlineKind(defCapture, nodeType),
			nodeType: nodeType,
			rng:      defNode.Range(),
			node:     defNode,
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
//  3. one span carrying two different kinds;
//  4. one span and kind carrying two different names;
//  5. an exact repeat of an accepted (Range, Kind, Name, NameRange) tuple;
//  6. a span that partially overlaps an accepted span (this rule runs in
//     buildOutlineForest, where the containment forest is assembled).
//
// Rules 3 and 4 both drop every member of the group. A group that disagrees
// about what it names cannot be resolved without reading the language, and
// reading the language is what this projection refuses to do. Only rule 5
// keeps a member, and only when the members are indistinguishable, so keeping
// the first is not a choice between them.
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

	// Rules 3, 4, and 5: group by exact span.
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
		first := valid[members[0]]

		kindConflict := false
		nameConflict := false
		for _, idx := range members[1:] {
			if valid[idx].kind != first.kind {
				kindConflict = true
				break
			}
			if !outlineSameName(valid[idx], first) {
				nameConflict = true
			}
		}

		switch {
		case kindConflict:
			// Rule 3: the span means two things at once. Drop the whole
			// group and count every member. Do not guess.
			report.OmittedConflict += len(members)
			continue
		case nameConflict:
			// Rule 4: the span agrees on the kind and disagrees on the
			// name. Picking by capture order publishes whichever binding
			// the grammar happened to reach first, which in C-family
			// method syntax is the return type. Drop the group instead
			// and count it, so the data gap is legible.
			report.OmittedNameConflict += len(members)
			continue
		}

		// Rule 5: the members are indistinguishable, so keep the first in
		// emission order and count the rest as exact repeats.
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

// outlineSameName reports whether two candidates name the same thing at the
// same place. Both the text and the span must agree: a symbol carries both,
// so a disagreement about either leaves the emitted symbol undetermined.
func outlineSameName(a, b outlineCandidate) bool {
	return a.name == b.name &&
		a.nameRange.StartByte == b.nameRange.StartByte &&
		a.nameRange.EndByte == b.nameRange.EndByte
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
//
// Materialization is also where Owner resolves, through resolveOutlineOwner
// (outline_owner.go). Owner reads only the candidate's own node, so resolving
// it here -- once per accepted candidate, alongside the rest of the symbol's
// fields -- is equivalent to resolving it in a separate pass over the built
// forest, without a second walk. ownerRules and lang may be nil, exactly as
// they are whenever no WithOutlineOwnerRules call attached a table; both
// resolveOutlineOwner and its test-only caller (runOutlineRules) rely on that
// nil case costing nothing beyond the nil check.
func buildOutlineForest(candidates []outlineCandidate, ownerRules map[string][]OutlineOwnerRule, lang *Language, source []byte, report *OutlineReport) []OutlineSymbol {
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
			Owner:     resolveOutlineOwner(ownerRules, candidate.node, candidate.nodeType, lang, source, report),
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
