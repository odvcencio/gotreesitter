package gotreesitter

import (
	"fmt"
	"unicode"
)

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
			last.endByte = uint32(p.pos)
			if err := p.validatePatternPredicates(last); err != nil {
				return err
			}
			continue
		}

		switch {
		case ch == '(':
			// A top-level pattern.
			startByte := uint32(p.pos)
			pat, err := p.parsePattern(0, 0)
			if err != nil {
				return err
			}
			applyWildcardRootSkip(pat)
			pat.startByte = startByte
			pat.endByte = uint32(p.pos)
			p.q.patterns = append(p.q.patterns, *pat)

		case ch == '[':
			// Top-level alternation: ["func" "return"] @keyword
			startByte := uint32(p.pos)
			pat, err := p.parseAlternationPattern(0, 0)
			if err != nil {
				return err
			}
			pat.startByte = startByte
			pat.endByte = uint32(p.pos)
			p.q.patterns = append(p.q.patterns, *pat)

		case ch == '"':
			// Top-level string match: "func" @keyword
			startByte := uint32(p.pos)
			pat, err := p.parseStringPattern(0)
			if err != nil {
				return err
			}
			pat.startByte = startByte
			pat.endByte = uint32(p.pos)
			p.q.patterns = append(p.q.patterns, *pat)

		case ch == '_' && !p.identifierContinuesAt(p.pos+1):
			// Top-level bare wildcard: _ @node matches any node, named or
			// anonymous, as in the C query parser.
			startByte := uint32(p.pos)
			p.pos++
			pat, err := p.parseIdentifierPatternFromName(0, "_")
			if err != nil {
				return err
			}
			pat.startByte = startByte
			pat.endByte = uint32(p.pos)
			p.q.patterns = append(p.q.patterns, *pat)

		case isIdentStart(ch):
			// Top-level field shorthand: field: (pattern)
			startByte := uint32(p.pos)
			pat, err := p.parseFieldShorthandPattern(0)
			if err != nil {
				return err
			}
			pat.startByte = startByte
			pat.endByte = uint32(p.pos)
			p.q.patterns = append(p.q.patterns, *pat)

		case ch == '.':
			return fmt.Errorf("query: unexpected top-level anchor '.' at position %d", p.pos)

		default:
			return fmt.Errorf("query: unexpected character %q at position %d", string(ch), p.pos)
		}
	}
	return nil
}

// parsePattern parses a parenthesized S-expression pattern.
// depth is the nesting depth for the steps produced.
func (p *queryParser) parsePattern(depth int, parentSymbolHint Symbol) (*Pattern, error) {
	if p.pos >= len(p.input) || p.input[p.pos] != '(' {
		return nil, fmt.Errorf("query: expected '(' at position %d", p.pos)
	}
	p.pos++ // consume '('
	p.skipWhitespaceAndComments()

	pat := &Pattern{}
	rootIdx, err := p.parsePatternRoot(pat, depth, parentSymbolHint)
	if err != nil {
		return nil, err
	}
	if err := p.parsePatternBody(pat, depth, parentSymbolHint, rootIdx); err != nil {
		return nil, err
	}
	if err := p.parseStepSuffix(pat, rootIdx); err != nil {
		return nil, err
	}
	if err := p.validatePatternPredicates(pat); err != nil {
		return nil, err
	}

	return pat, nil
}

func (p *queryParser) parsePatternRoot(pat *Pattern, depth int, parentSymbolHint Symbol) (int, error) {
	if p.pos >= len(p.input) {
		return -1, fmt.Errorf("query: unexpected end of input, expected node type or pattern")
	}

	switch ch := p.input[p.pos]; {
	case isIdentStart(ch):
		return p.parseIdentifierPatternRoot(pat, depth)
	case ch == '"':
		return p.parseStringPatternRoot(pat, depth)
	case ch == '(' || ch == '[':
		return p.parseGroupedPatternRoot(pat, depth, parentSymbolHint)
	default:
		return -1, fmt.Errorf("query: expected node type after '(' at position %d: query: expected identifier at position %d", p.pos, p.pos)
	}
}

func (p *queryParser) parseIdentifierPatternRoot(pat *Pattern, depth int) (int, error) {
	nodeType, err := p.readIdentifier()
	if err != nil {
		return -1, fmt.Errorf("query: expected node type after '(' at position %d: %w", p.pos, err)
	}
	step, err := p.stepFromIdentifierName(depth, nodeType)
	if err != nil {
		return -1, err
	}
	if nodeType == "_" {
		// Tree-sitter distinguishes parenthesized `(_)` from bare `_`.
		step.isNamed = true
	}
	pat.steps = append(pat.steps, step)
	return 0, nil
}

func (p *queryParser) parseStringPatternRoot(pat *Pattern, depth int) (int, error) {
	text, err := p.readString()
	if err != nil {
		return -1, err
	}
	pat.steps = append(pat.steps, QueryStep{
		depth:     depth,
		textMatch: text,
	})
	return 0, nil
}

func (p *queryParser) parseGroupedPatternRoot(pat *Pattern, depth int, parentSymbolHint Symbol) (int, error) {
	innerPat, err := p.parsePatternElement(depth, parentSymbolHint)
	if err != nil {
		return -1, err
	}
	if len(innerPat.steps) == 0 {
		return -1, fmt.Errorf("query: empty grouped pattern at position %d", p.pos)
	}

	if p.peekNextIsPatternElement() {
		pat.steps = append(pat.steps, QueryStep{
			symbol:    0,
			isNamed:   false,
			depth:     depth,
			synthetic: true,
		})
		for i := range innerPat.steps {
			innerPat.steps[i].depth++
		}
		pat.steps = append(pat.steps, innerPat.steps...)
	} else {
		pat.steps = append(pat.steps, innerPat.steps...)
	}
	pat.predicates = append(pat.predicates, innerPat.predicates...)
	return 0, nil
}

type patternBodyParser struct {
	p                *queryParser
	pat              *Pattern
	depth            int
	parentSymbolHint Symbol
	rootIdx          int
	pendingAnchor    bool
	lastChildRootIdx int
}

func (p *queryParser) parsePatternBody(pat *Pattern, depth int, parentSymbolHint Symbol, rootIdx int) error {
	body := patternBodyParser{
		p:                p,
		pat:              pat,
		depth:            depth,
		parentSymbolHint: parentSymbolHint,
		rootIdx:          rootIdx,
		lastChildRootIdx: -1,
	}
	return body.parse()
}

func (b *patternBodyParser) parse() error {
	for {
		b.p.skipWhitespaceAndComments()
		if b.p.pos >= len(b.p.input) {
			return fmt.Errorf("query: unexpected end of input, expected ')'")
		}

		ch := b.p.input[b.p.pos]
		switch {
		case ch == ')':
			b.close()
			return nil
		case ch == '.':
			b.p.pos++
			b.pendingAnchor = true
		case ch == '!':
			if err := b.parseAbsentField(); err != nil {
				return err
			}
		case ch == '@':
			if err := b.parseRootCapture(); err != nil {
				return err
			}
		case ch == '(' && b.p.pos+1 < len(b.p.input) && b.p.input[b.p.pos+1] == '#':
			if err := b.parsePredicate(); err != nil {
				return err
			}
		case ch == '(' || ch == '[' || ch == '"':
			if err := b.parseChildElement(); err != nil {
				return err
			}
		case isIdentStart(ch):
			if err := b.parseFieldOrIdentifierChild(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("query: unexpected character %q at position %d", string(ch), b.p.pos)
		}
	}
}

func (b *patternBodyParser) close() {
	if b.pendingAnchor && b.lastChildRootIdx >= 0 {
		b.pat.steps[b.lastChildRootIdx].anchorAfter = true
	}
	b.p.pos++ // consume ')'
}

func (b *patternBodyParser) appendChildPattern(childPat *Pattern) {
	if childPat == nil || len(childPat.steps) == 0 {
		return
	}
	if b.appendSyntheticGroupedChildPattern(childPat) {
		return
	}
	if b.pendingAnchor {
		childPat.steps[0].anchorBefore = true
		b.pendingAnchor = false
	}
	childRootIdx := len(b.pat.steps)
	b.appendChildPredicates(childPat)
	b.pat.steps = append(b.pat.steps, childPat.steps...)
	b.lastChildRootIdx = childRootIdx
}

func (b *patternBodyParser) appendSyntheticGroupedChildPattern(childPat *Pattern) bool {
	root := &childPat.steps[0]
	if !canInlineSyntheticGroupedChildRoot(root) || len(childPat.steps) <= 1 {
		return false
	}

	rootDepth := root.depth
	firstAppendedIdx := len(b.pat.steps)
	lastRootIdx := -1
	b.appendChildPredicates(childPat)
	for i := 1; i < len(childPat.steps); i++ {
		step := childPat.steps[i]
		step.depth--
		if i == 1 && (b.pendingAnchor || root.anchorBefore) {
			step.anchorBefore = true
			b.pendingAnchor = false
		}
		b.pat.steps = append(b.pat.steps, step)
		if step.depth == rootDepth {
			lastRootIdx = firstAppendedIdx + i - 1
		}
	}
	if root.anchorAfter && lastRootIdx >= 0 {
		b.pat.steps[lastRootIdx].anchorAfter = true
	}
	b.lastChildRootIdx = lastRootIdx
	return true
}

func (b *patternBodyParser) appendChildPredicates(childPat *Pattern) {
	if childPat == nil || len(childPat.predicates) == 0 {
		return
	}
	b.pat.predicates = append(b.pat.predicates, b.predicatesForChildPattern(childPat)...)
}

func (b *patternBodyParser) predicatesForChildPattern(childPat *Pattern) []QueryPredicate {
	if childPat == nil || len(childPat.steps) == 0 || !stepCanMatchZero(&childPat.steps[0]) {
		if childPat == nil {
			return nil
		}
		return childPat.predicates
	}
	predicates := childPat.predicates
	if len(predicates) == 0 {
		return predicates
	}
	captureNames := b.captureNamesForPattern(childPat)
	if len(captureNames) == 0 {
		return predicates
	}

	out := make([]QueryPredicate, len(predicates))
	copy(out, predicates)
	for i := range out {
		if predicateReferencesAnyCapture(out[i], captureNames) {
			out[i].allowMissing = true
		}
	}
	return out
}

func (b *patternBodyParser) captureNamesForPattern(pat *Pattern) map[string]struct{} {
	names := make(map[string]struct{})
	for i := range pat.steps {
		b.addCaptureNames(names, pat.steps[i].captureIDs)
		for j := range pat.steps[i].alternatives {
			b.addCaptureNames(names, pat.steps[i].alternatives[j].captureIDs)
			for k := range pat.steps[i].alternatives[j].steps {
				b.addCaptureNames(names, pat.steps[i].alternatives[j].steps[k].captureIDs)
			}
		}
	}
	return names
}

func (b *patternBodyParser) addCaptureNames(names map[string]struct{}, ids []int) {
	for _, id := range ids {
		if id >= 0 && id < len(b.p.q.captures) {
			names[b.p.q.captures[id]] = struct{}{}
		}
	}
}

func stepCanMatchZero(step *QueryStep) bool {
	return step.quantifier == queryQuantifierZeroOrOne || step.quantifier == queryQuantifierZeroOrMore
}

func predicateReferencesAnyCapture(pred QueryPredicate, names map[string]struct{}) bool {
	if pred.leftCapture != "" {
		if _, ok := names[pred.leftCapture]; ok {
			return true
		}
	}
	if pred.rightCapture != "" {
		if _, ok := names[pred.rightCapture]; ok {
			return true
		}
	}
	return false
}

func canInlineSyntheticGroupedChildRoot(step *QueryStep) bool {
	return step.synthetic &&
		step.quantifier == queryQuantifierOne &&
		step.field == 0 &&
		len(step.absentFields) == 0 &&
		len(step.captureIDs) == 0 &&
		len(step.alternatives) == 0 &&
		step.altIndex == nil &&
		step.textMatch == ""
}

func (b *patternBodyParser) rootStep() *QueryStep {
	if b.rootIdx >= 0 && b.rootIdx < len(b.pat.steps) {
		return &b.pat.steps[b.rootIdx]
	}
	return nil
}

func (b *patternBodyParser) rootSymbol() Symbol {
	if root := b.rootStep(); root != nil {
		return root.symbol
	}
	return 0
}

func (b *patternBodyParser) parseAbsentField() error {
	b.p.pos++
	b.p.skipWhitespaceAndComments()
	fieldName, err := b.p.readIdentifier()
	if err != nil {
		return err
	}
	root := b.rootStep()
	if root == nil {
		return nil
	}
	fieldID, err := b.p.resolveField(fieldName, root.symbol, b.parentSymbolHint)
	if err != nil {
		return err
	}
	root.absentFields = append(root.absentFields, fieldID)
	return nil
}

func (b *patternBodyParser) parseRootCapture() error {
	capName, err := b.p.readCapture()
	if err != nil {
		return err
	}
	if root := b.rootStep(); root != nil {
		captureID := b.p.ensureCapture(capName)
		if !root.synthetic {
			root.captureIDs = append(root.captureIDs, captureID)
		}
	}
	return nil
}

func (b *patternBodyParser) parsePredicate() error {
	pred, err := b.p.parsePredicate()
	if err != nil {
		return err
	}
	b.pat.predicates = append(b.pat.predicates, pred)
	return nil
}

func (b *patternBodyParser) parseChildElement() error {
	childPat, err := b.p.parsePatternElement(b.depth+1, b.rootSymbol())
	if err != nil {
		return err
	}
	b.appendChildPattern(childPat)
	return nil
}

func (b *patternBodyParser) parseFieldOrIdentifierChild() error {
	ident, err := b.p.readIdentifier()
	if err != nil {
		return err
	}
	afterIdent := b.p.pos
	b.p.skipWhitespaceAndComments()
	if b.p.pos < len(b.p.input) && b.p.input[b.p.pos] == ':' {
		return b.parseFieldChild(ident)
	}

	b.p.pos = afterIdent
	childPat, err := b.p.parseIdentifierPatternFromName(b.depth+1, ident)
	if err != nil {
		return err
	}
	b.appendChildPattern(childPat)
	return nil
}

func (b *patternBodyParser) parseFieldChild(fieldName string) error {
	b.p.pos++ // consume ':'
	b.p.skipWhitespaceAndComments()

	parentSymbol := b.rootSymbol()
	fieldID, err := b.p.resolveField(fieldName, parentSymbol, b.parentSymbolHint)
	if err != nil {
		return err
	}
	if b.p.pos >= len(b.p.input) {
		return fmt.Errorf("query: expected child pattern after field %q", fieldName)
	}

	childPat, err := b.p.parsePatternElement(b.depth+1, parentSymbol)
	if err != nil {
		return err
	}
	if len(childPat.steps) > 0 {
		childPat.steps[0].field = fieldID
	}
	b.appendChildPattern(childPat)
	return nil
}

func (p *queryParser) parseStepSuffix(pat *Pattern, rootIdx int) error {
	return p.parseStepSuffixInto(patternRootStep(pat, rootIdx))
}

func (p *queryParser) parseStepSuffixInto(step *QueryStep) error {
	p.skipWhitespaceAndComments()
	if quantifier, ok := p.readStepQuantifier(); ok {
		if step != nil {
			step.quantifier = quantifier
		}
		p.skipWhitespaceAndComments()
	}
	for p.pos < len(p.input) && p.input[p.pos] == '@' {
		capName, err := p.readCapture()
		if err != nil {
			return err
		}
		if step != nil {
			captureID := p.ensureCapture(capName)
			if !step.synthetic {
				step.captureIDs = append(step.captureIDs, captureID)
			}
		}
		p.skipWhitespaceAndComments()
	}
	return nil
}

func patternRootStep(pat *Pattern, rootIdx int) *QueryStep {
	if pat != nil && rootIdx >= 0 && rootIdx < len(pat.steps) {
		return &pat.steps[rootIdx]
	}
	return nil
}

// parseAlternationPattern parses [...] alternation syntax.
func (p *queryParser) parseAlternationPattern(depth int, parentSymbolHint Symbol) (*Pattern, error) {
	if p.pos >= len(p.input) || p.input[p.pos] != '[' {
		return nil, fmt.Errorf("query: expected '[' at position %d", p.pos)
	}
	p.pos++ // consume '['
	p.skipWhitespaceAndComments()

	alts, err := p.parseAlternationBranches(depth, parentSymbolHint)
	if err != nil {
		return nil, err
	}
	if len(alts) == 0 {
		return nil, fmt.Errorf("query: empty alternation")
	}

	step := QueryStep{
		depth:        depth,
		alternatives: alts,
	}
	if err := p.parseStepSuffixInto(&step); err != nil {
		return nil, err
	}

	return &Pattern{steps: []QueryStep{step}}, nil
}

func (p *queryParser) parseAlternationBranches(depth int, parentSymbolHint Symbol) ([]alternativeSymbol, error) {
	var alts []alternativeSymbol
	for {
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) {
			return nil, fmt.Errorf("query: unexpected end of input in alternation")
		}
		if p.input[p.pos] == ']' {
			p.pos++ // consume ']'
			return alts, nil
		}
		if p.input[p.pos] == '.' {
			p.pos++
			continue
		}

		alt, ok, err := p.parseAlternationBranch(depth, parentSymbolHint)
		if err != nil {
			return nil, err
		}
		if ok {
			alts = append(alts, alt)
		}
	}
}

func (p *queryParser) parseAlternationBranch(depth int, parentSymbolHint Symbol) (alternativeSymbol, bool, error) {
	branchPat, altField, err := p.parseAlternationBranchPattern(depth, parentSymbolHint)
	if err != nil {
		return alternativeSymbol{}, false, err
	}
	if len(branchPat.steps) == 0 {
		return alternativeSymbol{}, false, nil
	}

	root := branchPat.steps[0]
	alt := alternativeSymbol{
		symbol:    root.symbol,
		isNamed:   root.isNamed,
		isMissing: root.isMissing,
		supertype: root.supertype,
		field:     altField,
		textMatch: root.textMatch,
	}
	if root.field != 0 {
		alt.field = root.field
		branchPat.steps[0].field = 0
	}
	if len(branchPat.predicates) > 0 || len(branchPat.steps) > 1 || alt.field != 0 {
		alt.steps = make([]QueryStep, len(branchPat.steps))
		copy(alt.steps, branchPat.steps)
		alt.predicates = make([]QueryPredicate, len(branchPat.predicates))
		copy(alt.predicates, branchPat.predicates)
	} else {
		alt.captureIDs = append(alt.captureIDs, root.captureIDs...)
	}
	return alt, true, nil
}

func (p *queryParser) parseAlternationBranchPattern(depth int, parentSymbolHint Symbol) (*Pattern, FieldID, error) {
	ch := p.input[p.pos]
	if ch == '(' || ch == '[' || ch == '"' {
		pat, err := p.parsePatternElement(depth, parentSymbolHint)
		return pat, 0, err
	}
	if !isIdentStart(ch) {
		return nil, 0, fmt.Errorf("query: unexpected character %q in alternation at position %d", string(ch), p.pos)
	}

	ident, err := p.readIdentifier()
	if err != nil {
		return nil, 0, err
	}
	p.skipWhitespaceAndComments()
	if p.pos < len(p.input) && p.input[p.pos] == ':' {
		return p.parseAlternationFieldBranch(depth, parentSymbolHint, ident)
	}
	pat, err := p.parseIdentifierPatternFromName(depth, ident)
	return pat, 0, err
}

func (p *queryParser) parseAlternationFieldBranch(depth int, parentSymbolHint Symbol, fieldName string) (*Pattern, FieldID, error) {
	p.pos++ // consume ':'
	p.skipWhitespaceAndComments()
	fieldID, err := p.resolveField(fieldName, parentSymbolHint, parentSymbolHint)
	if err != nil {
		return nil, 0, err
	}
	pat, err := p.parsePatternElement(depth, parentSymbolHint)
	return pat, fieldID, err
}

// parseStringPattern parses a "string" pattern for matching anonymous nodes.
func (p *queryParser) parseStringPattern(depth int) (*Pattern, error) {
	text, err := p.readString()
	if err != nil {
		return nil, err
	}

	step := QueryStep{
		depth:     depth,
		textMatch: text,
	}
	if err := p.parseStepSuffixInto(&step); err != nil {
		return nil, err
	}

	return &Pattern{steps: []QueryStep{step}}, nil
}

// parsePatternElement parses one query element at the given depth.
// Supported forms:
//   - (pattern ...)
//   - [alternation ...]
//   - "string"
//   - identifier / _ (shorthand single-node pattern)
func (p *queryParser) parsePatternElement(depth int, parentSymbolHint Symbol) (*Pattern, error) {
	if p.pos >= len(p.input) {
		return nil, fmt.Errorf("query: expected pattern element at end of input")
	}

	switch ch := p.input[p.pos]; {
	case ch == '(':
		return p.parsePattern(depth, parentSymbolHint)
	case ch == '[':
		return p.parseAlternationPattern(depth, parentSymbolHint)
	case ch == '"':
		return p.parseStringPattern(depth)
	case isIdentStart(ch):
		name, err := p.readIdentifier()
		if err != nil {
			return nil, err
		}
		return p.parseIdentifierPatternFromName(depth, name)
	default:
		return nil, fmt.Errorf("query: expected '(' or '[' or '\"' or identifier at position %d", p.pos)
	}
}

func (p *queryParser) stepFromIdentifierName(depth int, name string) (QueryStep, error) {
	if name == "MISSING" {
		return p.stepFromMissingKeyword(depth)
	}

	sym, isNamed, supertype, err := p.resolveNodePattern(name)
	if err != nil {
		return QueryStep{}, err
	}

	return QueryStep{
		symbol:    sym,
		isNamed:   isNamed,
		supertype: supertype,
		depth:     depth,
	}, nil
}

// stepFromMissingKeyword parses the special "MISSING" node pattern
// (upstream tree-sitter 0.24+ semantics): (MISSING), (MISSING <kind>), or
// (MISSING "<token>"). It is called immediately after the "MISSING"
// identifier itself has been consumed, mirroring the corresponding branch
// of ts_query__parse_pattern in the C runtime. A "MISSING" step tests
// Node.IsMissing() at match time (see nodeMatchesScalarStep /
// stackEntryMatchesScalarStep); when a kind or token qualifier is given,
// the node's symbol must also match it.
//
// Anything other than an immediate kind identifier, a string-literal
// token, or a closing ')' is rejected as a syntax error -- in particular
// "(MISSING (child))" is invalid, matching upstream's TSQueryErrorSyntax.
func (p *queryParser) stepFromMissingKeyword(depth int) (QueryStep, error) {
	p.skipWhitespaceAndComments()
	step := QueryStep{depth: depth, isMissing: true}

	switch {
	case p.pos < len(p.input) && isIdentStart(p.input[p.pos]):
		name, err := p.readIdentifier()
		if err != nil {
			return QueryStep{}, err
		}
		sym, isNamed, err := p.resolveSymbol(name)
		if err != nil {
			return QueryStep{}, err
		}
		step.symbol = sym
		step.isNamed = isNamed
	case p.pos < len(p.input) && p.input[p.pos] == '"':
		text, err := p.readString()
		if err != nil {
			return QueryStep{}, err
		}
		step.textMatch = text
	case p.pos < len(p.input) && p.input[p.pos] == ')':
		// Bare (MISSING): matches any missing node, named or anonymous.
		step.isNamed = true
	default:
		return QueryStep{}, fmt.Errorf("query: expected node type, string literal, or ')' after 'MISSING' at position %d", p.pos)
	}

	return step, nil
}

func (p *queryParser) parseIdentifierPatternFromName(depth int, name string) (*Pattern, error) {
	step, err := p.stepFromIdentifierName(depth, name)
	if err != nil {
		return nil, err
	}
	if err := p.parseStepSuffixInto(&step); err != nil {
		return nil, err
	}

	return &Pattern{steps: []QueryStep{step}}, nil
}

func (p *queryParser) parseFieldShorthandPattern(depth int) (*Pattern, error) {
	fieldName, err := p.readIdentifier()
	if err != nil {
		return nil, err
	}
	p.skipWhitespaceAndComments()
	if p.pos >= len(p.input) || p.input[p.pos] != ':' {
		return nil, fmt.Errorf("query: unexpected identifier %q at position %d", fieldName, p.pos)
	}
	p.pos++ // consume ':'
	p.skipWhitespaceAndComments()

	fieldID, err := p.resolveField(fieldName, 0, 0)
	if err != nil {
		return nil, err
	}

	childPat, err := p.parsePatternElement(depth+1, 0)
	if err != nil {
		return nil, err
	}
	if len(childPat.steps) > 0 {
		childPat.steps[0].field = fieldID
	}

	// Use a wildcard root so field constraints can still be represented in the
	// existing matcher shape.
	root := QueryStep{
		symbol:  0,
		isNamed: false,
		depth:   depth,
	}
	pat := &Pattern{steps: []QueryStep{root}}
	pat.steps = append(pat.steps, childPat.steps...)
	pat.predicates = append(pat.predicates, childPat.predicates...)
	return pat, nil
}

func (p *queryParser) validatePatternPredicates(pat *Pattern) error {
	if len(pat.predicates) == 0 {
		return nil
	}
	// Keep validation permissive. Runtime predicate evaluation rejects matches
	// when required captures are missing.
	return nil
}

// applyWildcardRootSkip ports the wildcard-root rule of the C query compiler
// (ts_query_new: "If a pattern has a wildcard at its root, but it has a
// non-wildcard child, then optimize the matching process by skipping matching
// the wildcard"). The C cursor keys such a pattern on its first child step
// and never tests the root step: the root only has to exist and not be an
// ERROR node, and its captures take the child's parent. A supertype root is a
// wildcard step, so `(expression (identifier) @i)` matches every identifier
// whose parent is not an ERROR node, whatever the parent is.
func applyWildcardRootSkip(pat *Pattern) {
	if pat == nil || len(pat.steps) < 2 {
		return
	}
	root := &pat.steps[0]
	if root.symbol != 0 || root.textMatch != "" || len(root.alternatives) > 0 ||
		root.field != 0 || root.depth != 0 || root.synthetic || root.isMissing {
		return
	}
	second := &pat.steps[1]
	if second.depth != 1 || second.anchorBefore || !stepKeysOnSymbol(second) {
		return
	}
	root.isNamed = false
	root.supertype = 0
	root.absentFields = nil
}

// stepKeysOnSymbol reports whether the C compiler sees a non-wildcard symbol
// in the step: a concrete node type or a string literal, or, for an
// alternation, in its first branch.
func stepKeysOnSymbol(step *QueryStep) bool {
	if len(step.alternatives) > 0 {
		alt := &step.alternatives[0]
		return alt.symbol != 0 || alt.textMatch != ""
	}
	return step.symbol != 0 || step.textMatch != ""
}

// identifierContinuesAt reports whether an identifier character sits at pos,
// so that a bare `_` is not the start of a longer name.
func (p *queryParser) identifierContinuesAt(pos int) bool {
	if pos >= len(p.input) {
		return false
	}
	ch := rune(p.input[pos])
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch == '.' || ch == '-' || ch == '/'
}
