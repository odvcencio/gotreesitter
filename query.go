package gotreesitter

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Query holds compiled patterns parsed from a tree-sitter .scm query file.
// It can be executed against a syntax tree to find matching nodes and
// return captured names.
type Query struct {
	patterns []Pattern
	captures []string // capture name by index

	rootCandidatesBySymbol map[Symbol][]int
	rootFallbackCandidates []int
}

// Pattern is a single top-level S-expression pattern in a query.
type Pattern struct {
	steps      []QueryStep
	predicates []QueryPredicate
}

// QueryStep is one matching instruction within a pattern.
type QueryStep struct {
	symbol    Symbol  // node type to match, or 0 for wildcard
	field     FieldID // required field on parent, or 0
	captureID int     // index into Query.captures, or -1 if no capture
	isNamed   bool    // whether we expect a named node
	depth     int     // nesting depth (0 = top-level node in pattern)
	// For alternation steps, alternatives lists the alternative symbols
	// that can match at this position. If non-nil, symbol is ignored.
	alternatives []alternativeSymbol
	// textMatch is for string literal matching ("func", "return", etc.).
	// When non-empty, we match anonymous nodes whose symbol name equals this.
	textMatch string
}

type queryPredicateType uint8

const (
	predicateEq queryPredicateType = iota
	predicateMatch
)

// QueryPredicate is a post-match constraint attached to a pattern.
// Supported forms:
//   - (#eq? @a @b)
//   - (#eq? @a "literal")
//   - (#match? @a "regex")
type QueryPredicate struct {
	kind queryPredicateType

	leftCapture  string
	rightCapture string // optional for #eq?
	literal      string // literal or regex source
	regex        *regexp.Regexp
}

// alternativeSymbol is one branch of an alternation like [(true) (false)].
type alternativeSymbol struct {
	symbol  Symbol
	isNamed bool
	// textMatch for string alternatives like "func"
	textMatch string
}

// QueryMatch represents a successful pattern match with its captures.
type QueryMatch struct {
	PatternIndex int
	Captures     []QueryCapture
}

// QueryCapture is a single captured node within a match.
type QueryCapture struct {
	Name string
	Node *Node
}

// NewQuery compiles query source (tree-sitter .scm format) against a language.
// It returns an error if the query syntax is invalid or references unknown
// node types or field names.
func NewQuery(source string, lang *Language) (*Query, error) {
	p := &queryParser{
		input: source,
		lang:  lang,
		q: &Query{
			captures: []string{},
		},
	}
	if err := p.parse(); err != nil {
		return nil, err
	}
	p.q.buildRootPatternIndex()
	return p.q, nil
}

// Execute runs the query against a syntax tree and returns all matches.
func (q *Query) Execute(tree *Tree) []QueryMatch {
	return q.executeNode(tree.RootNode(), tree.Language(), tree.Source())
}

// ExecuteNode runs the query starting from a specific node.
func (q *Query) ExecuteNode(node *Node, lang *Language) []QueryMatch {
	return q.executeNode(node, lang, nil)
}

func (q *Query) executeNode(root *Node, lang *Language, source []byte) []QueryMatch {
	if root == nil {
		return nil
	}
	if q.rootCandidatesBySymbol == nil && q.rootFallbackCandidates == nil {
		q.buildRootPatternIndex()
	}
	var matches []QueryMatch
	q.walkAndMatch(root, lang, source, &matches)
	return matches
}

func (q *Query) rootPatternCandidates(sym Symbol) []int {
	if cands, ok := q.rootCandidatesBySymbol[sym]; ok {
		return cands
	}
	return q.rootFallbackCandidates
}

func mergePatternIndexLists(a, b []int) []int {
	if len(a) == 0 {
		out := make([]int, len(b))
		copy(out, b)
		return out
	}
	if len(b) == 0 {
		out := make([]int, len(a))
		copy(out, a)
		return out
	}

	out := make([]int, 0, len(a)+len(b))
	i, j := 0, 0
	last := -1
	hasLast := false

	appendUnique := func(v int) {
		if hasLast && v == last {
			return
		}
		out = append(out, v)
		last = v
		hasLast = true
	}

	for i < len(a) && j < len(b) {
		if a[i] < b[j] {
			appendUnique(a[i])
			i++
			continue
		}
		if b[j] < a[i] {
			appendUnique(b[j])
			j++
			continue
		}
		appendUnique(a[i])
		i++
		j++
	}
	for ; i < len(a); i++ {
		appendUnique(a[i])
	}
	for ; j < len(b); j++ {
		appendUnique(b[j])
	}
	return out
}

func (q *Query) buildRootPatternIndex() {
	bySymbolExact := make(map[Symbol][]int)
	var wildcard []int
	var complex []int

	for pi, pat := range q.patterns {
		if len(pat.steps) == 0 {
			continue
		}
		step := pat.steps[0]

		if len(step.alternatives) > 0 {
			complexAlt := false
			for _, alt := range step.alternatives {
				if alt.textMatch != "" || alt.symbol == 0 {
					complexAlt = true
					break
				}
			}
			if complexAlt {
				complex = append(complex, pi)
				continue
			}

			seen := make(map[Symbol]struct{}, len(step.alternatives))
			for _, alt := range step.alternatives {
				if _, ok := seen[alt.symbol]; ok {
					continue
				}
				seen[alt.symbol] = struct{}{}
				bySymbolExact[alt.symbol] = append(bySymbolExact[alt.symbol], pi)
			}
			continue
		}

		if step.textMatch != "" {
			complex = append(complex, pi)
			continue
		}
		if step.symbol == 0 {
			wildcard = append(wildcard, pi)
			continue
		}

		bySymbolExact[step.symbol] = append(bySymbolExact[step.symbol], pi)
	}

	fallback := mergePatternIndexLists(wildcard, complex)
	q.rootFallbackCandidates = fallback
	q.rootCandidatesBySymbol = make(map[Symbol][]int, len(bySymbolExact))
	for sym, exact := range bySymbolExact {
		q.rootCandidatesBySymbol[sym] = mergePatternIndexLists(exact, fallback)
	}
}

// walkAndMatch does an iterative depth-first walk of the tree, trying to
// match each pattern at each node. Uses an explicit worklist to avoid
// stack overflow on deep trees (e.g., from parser error recovery).
func (q *Query) walkAndMatch(node *Node, lang *Language, source []byte, matches *[]QueryMatch) {
	// Iterative DFS using an explicit stack.
	worklist := []*Node{node}
	for len(worklist) > 0 {
		// Pop.
		n := worklist[len(worklist)-1]
		worklist = worklist[:len(worklist)-1]

			// Try matching only patterns whose root step can match this symbol.
			for _, pi := range q.rootPatternCandidates(n.Symbol()) {
				pat := q.patterns[pi]
				if caps, ok := q.matchPattern(&pat, n, lang, source); ok {
					*matches = append(*matches, QueryMatch{
						PatternIndex: pi,
						Captures:     caps,
				})
			}
		}

		// Push children in reverse order so leftmost is processed first.
		children := n.Children()
		for i := len(children) - 1; i >= 0; i-- {
			worklist = append(worklist, children[i])
		}
	}
}

// matchPattern tries to match a pattern against the given node.
// The pattern's steps describe a nested structure; step depth 0 matches
// the given node, depth 1 matches its children, etc.
func (q *Query) matchPattern(pat *Pattern, node *Node, lang *Language, source []byte) ([]QueryCapture, bool) {
	if len(pat.steps) == 0 {
		return nil, false
	}

	var captures []QueryCapture
	ok := q.matchSteps(pat.steps, 0, node, lang, &captures)
	if !ok {
		return nil, false
	}
	if !q.matchesPredicates(pat.predicates, captures, source) {
		return nil, false
	}
	return captures, true
}

func (q *Query) matchesPredicates(predicates []QueryPredicate, captures []QueryCapture, source []byte) bool {
	if len(predicates) == 0 {
		return true
	}

	for _, pred := range predicates {
		left, ok := captureText(pred.leftCapture, captures, source)
		if !ok {
			return false
		}

		switch pred.kind {
		case predicateEq:
			right := pred.literal
			if pred.rightCapture != "" {
				var okRight bool
				right, okRight = captureText(pred.rightCapture, captures, source)
				if !okRight {
					return false
				}
			}
			if left != right {
				return false
			}

		case predicateMatch:
			if pred.regex == nil || !pred.regex.MatchString(left) {
				return false
			}
		}
	}

	return true
}

func captureText(name string, captures []QueryCapture, source []byte) (string, bool) {
	for _, c := range captures {
		if c.Name == name {
			if source == nil {
				return "", false
			}
			return c.Node.Text(source), true
		}
	}
	return "", false
}

// matchSteps matches a contiguous slice of steps starting at stepIdx
// against the given node at the expected depth.
func (q *Query) matchSteps(steps []QueryStep, stepIdx int, node *Node, lang *Language, captures *[]QueryCapture) bool {
	if stepIdx >= len(steps) {
		return false
	}

	step := &steps[stepIdx]

	// Check if this node matches the current step.
	if !q.nodeMatchesStep(step, node, lang) {
		return false
	}

	// Collect capture if present.
	if step.captureID >= 0 {
		*captures = append(*captures, QueryCapture{
			Name: q.captures[step.captureID],
			Node: node,
		})
	}

	// Find child steps (steps at depth = step.depth + 1) that are direct
	// descendants of this step.
	childDepth := step.depth + 1
	childStart := stepIdx + 1

	// If there are no more steps, we matched successfully.
	if childStart >= len(steps) {
		return true
	}

	// If the next step is at the same depth or shallower, there are no
	// child constraints -- we matched.
	if steps[childStart].depth <= step.depth {
		return true
	}

	// Collect child step indices at childDepth (stop when we see a step
	// at a depth <= step.depth, meaning it belongs to a sibling/ancestor).
	type childStepInfo struct {
		stepIdx int
		field   FieldID
	}
	var childSteps []childStepInfo
	for i := childStart; i < len(steps); i++ {
		if steps[i].depth <= step.depth {
			break
		}
		if steps[i].depth == childDepth {
			childSteps = append(childSteps, childStepInfo{
				stepIdx: i,
				field:   steps[i].field,
			})
		}
	}

	// Try to match each child step against the node's children.
	for _, cs := range childSteps {
		matched := false
		childStep := &steps[cs.stepIdx]

		if cs.field != 0 {
			// Field-constrained: find child by field.
			fieldName := ""
			if int(cs.field) < len(lang.FieldNames) {
				fieldName = lang.FieldNames[cs.field]
			}
			if fieldName == "" {
				return false
			}
			fieldChild := node.ChildByFieldName(fieldName, lang)
			if fieldChild == nil {
				return false
			}
			if q.matchSteps(steps, cs.stepIdx, fieldChild, lang, captures) {
				matched = true
			}
		} else {
			// No field constraint: search all children for a match.
			for _, child := range node.Children() {
				if q.nodeMatchesStep(childStep, child, lang) {
					if q.matchSteps(steps, cs.stepIdx, child, lang, captures) {
						matched = true
						break
					}
				}
			}
		}

		if !matched {
			return false
		}
	}

	return true
}

// nodeMatchesStep checks if a single node matches a single step's type/symbol constraint.
func (q *Query) nodeMatchesStep(step *QueryStep, node *Node, lang *Language) bool {
	// Alternation matching.
	if len(step.alternatives) > 0 {
		for _, alt := range step.alternatives {
			// Wildcard in alternation `( _ )` should match any node.
			if alt.symbol == 0 && alt.textMatch == "" {
				return true
			}

			if alt.textMatch != "" {
				// String match for anonymous nodes.
				if !node.IsNamed() && node.Type(lang) == alt.textMatch {
					return true
				}
			} else if node.Symbol() == alt.symbol && node.IsNamed() == alt.isNamed {
				return true
			}
		}
		return false
	}

	// Text matching for string literals like "func".
	if step.textMatch != "" {
		return !node.IsNamed() && node.Type(lang) == step.textMatch
	}

	// Wildcard (symbol == 0 and no textMatch and no alternatives).
	if step.symbol == 0 {
		return true
	}

	// Symbol matching.
	if node.Symbol() != step.symbol {
		return false
	}

	// Named check.
	if step.isNamed && !node.IsNamed() {
		return false
	}

	return true
}

// PatternCount returns the number of patterns in the query.
func (q *Query) PatternCount() int {
	return len(q.patterns)
}

// CaptureNames returns the list of unique capture names used in the query.
func (q *Query) CaptureNames() []string {
	return q.captures
}

// --------------------------------------------------------------------------
// S-expression parser
// --------------------------------------------------------------------------

// queryParser parses tree-sitter .scm query files into a Query.
type queryParser struct {
	input string
	pos   int
	lang  *Language
	q     *Query
}

func (p *queryParser) parse() error {
	for {
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) {
			break
		}

		ch := p.input[p.pos]

		if ch == '(' && p.pos+1 < len(p.input) && p.input[p.pos+1] == '#' {
			if len(p.q.patterns) == 0 {
				return fmt.Errorf("query: predicate must follow a pattern at position %d", p.pos)
			}
			pred, err := p.parsePredicate()
			if err != nil {
				return err
			}
			last := &p.q.patterns[len(p.q.patterns)-1]
			last.predicates = append(last.predicates, pred)
			if err := p.validatePatternPredicates(last); err != nil {
				return err
			}
			continue
		}

		switch {
		case ch == '(':
			// A top-level pattern.
			pat, err := p.parsePattern(0)
			if err != nil {
				return err
			}
			p.q.patterns = append(p.q.patterns, *pat)

		case ch == '[':
			// Top-level alternation: ["func" "return"] @keyword
			pat, err := p.parseAlternationPattern(0)
			if err != nil {
				return err
			}
			p.q.patterns = append(p.q.patterns, *pat)

		case ch == '"':
			// Top-level string match: "func" @keyword
			pat, err := p.parseStringPattern(0)
			if err != nil {
				return err
			}
			p.q.patterns = append(p.q.patterns, *pat)

		default:
			return fmt.Errorf("query: unexpected character %q at position %d", string(ch), p.pos)
		}
	}
	return nil
}

// parsePattern parses a parenthesized S-expression pattern.
// depth is the nesting depth for the steps produced.
func (p *queryParser) parsePattern(depth int) (*Pattern, error) {
	if p.pos >= len(p.input) || p.input[p.pos] != '(' {
		return nil, fmt.Errorf("query: expected '(' at position %d", p.pos)
	}
	p.pos++ // consume '('
	p.skipWhitespaceAndComments()

	pat := &Pattern{}

	// Read the node type name.
	nodeType, err := p.readIdentifier()
	if err != nil {
		return nil, fmt.Errorf("query: expected node type after '(' at position %d: %w", p.pos, err)
	}

	sym, isNamed, err := p.resolveSymbol(nodeType)
	if err != nil {
		return nil, err
	}

	step := QueryStep{
		symbol:    sym,
		isNamed:   isNamed,
		captureID: -1,
		depth:     depth,
	}

	pat.steps = append(pat.steps, step)
	rootIdx := len(pat.steps) - 1

	// Parse children, fields, and captures until ')'.
	for {
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) {
			return nil, fmt.Errorf("query: unexpected end of input, expected ')'")
		}

		ch := p.input[p.pos]

		if ch == ')' {
			p.pos++ // consume ')'
			break
		}

		if ch == '@' {
			// Capture for the current node.
			capName, err := p.readCapture()
			if err != nil {
				return nil, err
			}
			capID := p.ensureCapture(capName)
			pat.steps[rootIdx].captureID = capID
			continue
		}

		if ch == '(' {
			// Predicate expression.
			if p.pos+1 < len(p.input) && p.input[p.pos+1] == '#' {
				pred, err := p.parsePredicate()
				if err != nil {
					return nil, err
				}
				pat.predicates = append(pat.predicates, pred)
				continue
			}

			// Nested pattern (child constraint).
			childPat, err := p.parsePattern(depth + 1)
			if err != nil {
				return nil, err
			}
			pat.steps = append(pat.steps, childPat.steps...)
			continue
		}

		if ch == '[' {
			// Alternation child.
			childPat, err := p.parseAlternationPattern(depth + 1)
			if err != nil {
				return nil, err
			}
			pat.steps = append(pat.steps, childPat.steps...)
			continue
		}

		if ch == '"' {
			// String child.
			childPat, err := p.parseStringPattern(depth + 1)
			if err != nil {
				return nil, err
			}
			pat.steps = append(pat.steps, childPat.steps...)
			continue
		}

		// Check for field: syntax (identifier followed by ':')
		if isIdentStart(ch) {
			saved := p.pos
			ident, err := p.readIdentifier()
			if err != nil {
				return nil, err
			}
			p.skipWhitespaceAndComments()
			if p.pos < len(p.input) && p.input[p.pos] == ':' {
				// It's a field constraint.
				p.pos++ // consume ':'
				p.skipWhitespaceAndComments()

				fieldID, err := p.resolveField(ident)
				if err != nil {
					return nil, err
				}

				// The child pattern follows.
				if p.pos >= len(p.input) {
					return nil, fmt.Errorf("query: expected child pattern after field %q", ident)
				}

				var childSteps []QueryStep
				ch2 := p.input[p.pos]
				if ch2 == '(' {
					childPat, err := p.parsePattern(depth + 1)
					if err != nil {
						return nil, err
					}
					childSteps = childPat.steps
				} else if ch2 == '[' {
					childPat, err := p.parseAlternationPattern(depth + 1)
					if err != nil {
						return nil, err
					}
					childSteps = childPat.steps
				} else if ch2 == '"' {
					childPat, err := p.parseStringPattern(depth + 1)
					if err != nil {
						return nil, err
					}
					childSteps = childPat.steps
				} else {
					return nil, fmt.Errorf("query: expected '(' or '[' or '\"' after field %q:", ident)
				}

				// Set the field on the first child step.
				if len(childSteps) > 0 {
					childSteps[0].field = fieldID
				}
				pat.steps = append(pat.steps, childSteps...)
			} else {
				// Not a field, rewind. It might be part of a capture
				// we haven't handled or some other unexpected token.
				p.pos = saved
				return nil, fmt.Errorf("query: unexpected identifier %q at position %d", ident, saved)
			}
			continue
		}

		return nil, fmt.Errorf("query: unexpected character %q at position %d", string(ch), p.pos)
	}

	// Check for capture after the closing paren.
	p.skipWhitespaceAndComments()
	if p.pos < len(p.input) && p.input[p.pos] == '@' {
		capName, err := p.readCapture()
		if err != nil {
			return nil, err
		}
		capID := p.ensureCapture(capName)
		pat.steps[rootIdx].captureID = capID
	}

	if err := p.validatePatternPredicates(pat); err != nil {
		return nil, err
	}

	return pat, nil
}

// parseAlternationPattern parses [...] alternation syntax.
func (p *queryParser) parseAlternationPattern(depth int) (*Pattern, error) {
	if p.pos >= len(p.input) || p.input[p.pos] != '[' {
		return nil, fmt.Errorf("query: expected '[' at position %d", p.pos)
	}
	p.pos++ // consume '['
	p.skipWhitespaceAndComments()

	var alts []alternativeSymbol

	for {
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) {
			return nil, fmt.Errorf("query: unexpected end of input in alternation")
		}

		if p.input[p.pos] == ']' {
			p.pos++ // consume ']'
			break
		}

		if p.input[p.pos] == '(' {
			// (node_type)
			p.pos++ // consume '('
			p.skipWhitespaceAndComments()
			nodeType, err := p.readIdentifier()
			if err != nil {
				return nil, fmt.Errorf("query: expected node type in alternation: %w", err)
			}
			p.skipWhitespaceAndComments()
			if p.pos >= len(p.input) || p.input[p.pos] != ')' {
				return nil, fmt.Errorf("query: expected ')' in alternation at position %d", p.pos)
			}
			p.pos++ // consume ')'

			sym, isNamed, err := p.resolveSymbol(nodeType)
			if err != nil {
				return nil, err
			}
			alts = append(alts, alternativeSymbol{symbol: sym, isNamed: isNamed})
		} else if p.input[p.pos] == '"' {
			// "string"
			text, err := p.readString()
			if err != nil {
				return nil, err
			}
			alts = append(alts, alternativeSymbol{textMatch: text})
		} else {
			return nil, fmt.Errorf("query: unexpected character %q in alternation at position %d", string(p.input[p.pos]), p.pos)
		}
	}

	if len(alts) == 0 {
		return nil, fmt.Errorf("query: empty alternation")
	}

	step := QueryStep{
		captureID:    -1,
		depth:        depth,
		alternatives: alts,
	}

	// Check for capture after ']'.
	p.skipWhitespaceAndComments()
	if p.pos < len(p.input) && p.input[p.pos] == '@' {
		capName, err := p.readCapture()
		if err != nil {
			return nil, err
		}
		step.captureID = p.ensureCapture(capName)
	}

	return &Pattern{steps: []QueryStep{step}}, nil
}

// parseStringPattern parses a "string" pattern for matching anonymous nodes.
func (p *queryParser) parseStringPattern(depth int) (*Pattern, error) {
	text, err := p.readString()
	if err != nil {
		return nil, err
	}

	step := QueryStep{
		captureID: -1,
		depth:     depth,
		textMatch: text,
	}

	// Check for capture after the string.
	p.skipWhitespaceAndComments()
	if p.pos < len(p.input) && p.input[p.pos] == '@' {
		capName, err := p.readCapture()
		if err != nil {
			return nil, err
		}
		step.captureID = p.ensureCapture(capName)
	}

	return &Pattern{steps: []QueryStep{step}}, nil
}

func (p *queryParser) parsePredicate() (QueryPredicate, error) {
	if p.pos >= len(p.input) || p.input[p.pos] != '(' {
		return QueryPredicate{}, fmt.Errorf("query: expected '(' at position %d", p.pos)
	}
	p.pos++ // consume '('
	p.skipWhitespaceAndComments()

	name, err := p.readPredicateName()
	if err != nil {
		return QueryPredicate{}, err
	}

	p.skipWhitespaceAndComments()
	left, leftIsCapture, err := p.readPredicateArg()
	if err != nil {
		return QueryPredicate{}, err
	}
	if !leftIsCapture {
		return QueryPredicate{}, fmt.Errorf("query: first predicate argument must be a capture in %s", name)
	}

	p.skipWhitespaceAndComments()
	right, rightIsCapture, err := p.readPredicateArg()
	if err != nil {
		return QueryPredicate{}, err
	}

	p.skipWhitespaceAndComments()
	if p.pos >= len(p.input) || p.input[p.pos] != ')' {
		return QueryPredicate{}, fmt.Errorf("query: expected ')' to close predicate at position %d", p.pos)
	}
	p.pos++ // consume ')'

	switch name {
	case "#eq?":
		pred := QueryPredicate{
			kind:        predicateEq,
			leftCapture: left,
		}
		if rightIsCapture {
			pred.rightCapture = right
		} else {
			pred.literal = right
		}
		return pred, nil

	case "#match?":
		if rightIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: #match? second argument must be a string literal")
		}
		rx, err := regexp.Compile(right)
		if err != nil {
			return QueryPredicate{}, fmt.Errorf("query: invalid regex in #match?: %w", err)
		}
		return QueryPredicate{
			kind:        predicateMatch,
			leftCapture: left,
			literal:     right,
			regex:       rx,
		}, nil
	default:
		return QueryPredicate{}, fmt.Errorf("query: unsupported predicate %q", name)
	}
}

func (p *queryParser) validatePatternPredicates(pat *Pattern) error {
	if len(pat.predicates) == 0 {
		return nil
	}

	captureSet := make(map[string]struct{})
	for _, s := range pat.steps {
		if s.captureID >= 0 && s.captureID < len(p.q.captures) {
			captureSet[p.q.captures[s.captureID]] = struct{}{}
		}
	}

	for _, pred := range pat.predicates {
		if _, ok := captureSet[pred.leftCapture]; !ok {
			return fmt.Errorf("query: predicate references unknown capture @%s", pred.leftCapture)
		}
		if pred.rightCapture != "" {
			if _, ok := captureSet[pred.rightCapture]; !ok {
				return fmt.Errorf("query: predicate references unknown capture @%s", pred.rightCapture)
			}
		}
	}

	return nil
}

// readIdentifier reads an identifier (node type name, field name).
// Identifiers can contain letters, digits, underscores, dots, and hyphens.
func (p *queryParser) readPredicateName() (string, error) {
	if p.pos >= len(p.input) || p.input[p.pos] != '#' {
		return "", fmt.Errorf("query: expected predicate name at position %d", p.pos)
	}
	start := p.pos
	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if ch == ')' || ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			break
		}
		p.pos++
	}
	if p.pos == start {
		return "", fmt.Errorf("query: expected predicate name at position %d", start)
	}
	return p.input[start:p.pos], nil
}

func (p *queryParser) readPredicateArg() (arg string, isCapture bool, err error) {
	if p.pos >= len(p.input) {
		return "", false, fmt.Errorf("query: expected predicate argument at end of input")
	}

	switch p.input[p.pos] {
	case '@':
		name, err := p.readCapture()
		if err != nil {
			return "", false, err
		}
		return name, true, nil
	case '"':
		text, err := p.readString()
		if err != nil {
			return "", false, err
		}
		return text, false, nil
	default:
		return "", false, fmt.Errorf("query: expected capture or string literal in predicate at position %d", p.pos)
	}
}

func (p *queryParser) readIdentifier() (string, error) {
	start := p.pos
	for p.pos < len(p.input) {
		ch := rune(p.input[p.pos])
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch == '.' || ch == '-' {
			p.pos++
		} else {
			break
		}
	}
	if p.pos == start {
		return "", fmt.Errorf("query: expected identifier at position %d", p.pos)
	}
	return p.input[start:p.pos], nil
}

// readCapture reads a @capture_name token. It consumes the '@' and the name.
func (p *queryParser) readCapture() (string, error) {
	if p.pos >= len(p.input) || p.input[p.pos] != '@' {
		return "", fmt.Errorf("query: expected '@' at position %d", p.pos)
	}
	p.pos++ // consume '@'
	name, err := p.readIdentifier()
	if err != nil {
		return "", fmt.Errorf("query: expected capture name after '@': %w", err)
	}
	return name, nil
}

// readString reads a quoted string like "func". Consumes the quotes.
func (p *queryParser) readString() (string, error) {
	if p.pos >= len(p.input) || p.input[p.pos] != '"' {
		return "", fmt.Errorf("query: expected '\"' at position %d", p.pos)
	}
	p.pos++ // consume opening '"'
	var sb strings.Builder
	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if ch == '\\' && p.pos+1 < len(p.input) {
			p.pos++
			sb.WriteByte(p.input[p.pos])
			p.pos++
			continue
		}
		if ch == '"' {
			p.pos++ // consume closing '"'
			return sb.String(), nil
		}
		sb.WriteByte(ch)
		p.pos++
	}
	return "", fmt.Errorf("query: unterminated string")
}

// skipWhitespaceAndComments skips whitespace and ;-style line comments.
func (p *queryParser) skipWhitespaceAndComments() {
	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			p.pos++
			continue
		}
		if ch == ';' {
			// Skip to end of line.
			for p.pos < len(p.input) && p.input[p.pos] != '\n' {
				p.pos++
			}
			continue
		}
		break
	}
}

// resolveSymbol looks up a node type name in the language, returning the
// symbol ID and whether it's a named symbol. Uses Language.SymbolByName
// for O(1) lookup.
func (p *queryParser) resolveSymbol(name string) (Symbol, bool, error) {
	if name == "_" {
		return 0, false, nil
	}

	sym, ok := p.lang.SymbolByName(name)
	if !ok {
		return 0, false, fmt.Errorf("query: unknown node type %q", name)
	}
	isNamed := false
	if int(sym) < len(p.lang.SymbolMetadata) {
		isNamed = p.lang.SymbolMetadata[sym].Named
	}
	return sym, isNamed, nil
}

// resolveField looks up a field name in the language. Uses Language.FieldByName
// for O(1) lookup.
func (p *queryParser) resolveField(name string) (FieldID, error) {
	fid, ok := p.lang.FieldByName(name)
	if !ok {
		return 0, fmt.Errorf("query: unknown field name %q", name)
	}
	return fid, nil
}

// ensureCapture returns the index for a capture name, adding it if new.
func (p *queryParser) ensureCapture(name string) int {
	for i, cn := range p.q.captures {
		if cn == name {
			return i
		}
	}
	idx := len(p.q.captures)
	p.q.captures = append(p.q.captures, name)
	return idx
}

// isIdentStart reports whether a byte can start an identifier.
func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}
