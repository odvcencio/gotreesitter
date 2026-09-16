package gotreesitter

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/odvcencio/gotreesitter/internal/luapattern"
)

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

	switch name {
	case "#eq?":
		return p.parseEqualityPredicate(name, predicateEq)
	case "#not-eq?":
		return p.parseEqualityPredicate(name, predicateNotEq)
	case "#match?":
		return p.parseRegexPredicate(name, predicateMatch, regexp.Compile, "invalid regex")
	case "#not-match?":
		return p.parseRegexPredicate(name, predicateNotMatch, regexp.Compile, "invalid regex")
	case "#any-eq?":
		return p.parseEqualityPredicate(name, predicateAnyEq)
	case "#any-not-eq?":
		return p.parseEqualityPredicate(name, predicateAnyNotEq)
	case "#any-match?":
		return p.parseRegexPredicate(name, predicateAnyMatch, regexp.Compile, "invalid regex")
	case "#any-not-match?":
		return p.parseRegexPredicate(name, predicateAnyNotMatch, regexp.Compile, "invalid regex")
	case "#lua-match?":
		return p.parseRegexPredicate(name, predicateLuaMatch, luapattern.Compile, "invalid lua pattern")
	case "#any-of?":
		return p.parseListPredicate(name, predicateAnyOf, "query: #any-of? requires at least one string literal")
	case "#not-any-of?":
		return p.parseListPredicate(name, predicateNotAnyOf, "query: #not-any-of? requires at least one literal")
	case "#has-ancestor?", "#not-has-ancestor?", "#has-parent?", "#not-has-parent?":
		return p.parseNodeTypePredicate(name)
	case "#is?", "#is-not?":
		return p.parseIsPredicate(name)
	case "#set!":
		return p.parseSetPredicate()
	case "#offset!":
		return p.parseOffsetPredicate()
	case "#select-adjacent!":
		return p.parseSelectAdjacentPredicate()
	case "#strip!":
		return p.parseStripPredicate()
	case "#count?":
		return p.parseCountPredicate()
	case "#is-exported?":
		return p.parseIsExportedPredicate()
	default:
		return QueryPredicate{}, fmt.Errorf("query: unsupported predicate %q", name)
	}
}

type parsedPredicateArg struct {
	value string
	kind  predicateArgKind
}

func (p *queryParser) closePredicate() error {
	p.skipWhitespaceAndComments()
	if p.pos >= len(p.input) || p.input[p.pos] != ')' {
		return fmt.Errorf("query: expected ')' to close predicate at position %d", p.pos)
	}
	p.pos++
	return nil
}

func (p *queryParser) readPredicateArgAfterWhitespace() (string, bool, error) {
	p.skipWhitespaceAndComments()
	return p.readPredicateArg()
}

func (p *queryParser) readPredicateTokenAfterWhitespace() (string, predicateArgKind, error) {
	p.skipWhitespaceAndComments()
	return p.readPredicateToken()
}

func (p *queryParser) readPredicateFirstCapture(name string) (string, error) {
	arg, isCapture, err := p.readPredicateArgAfterWhitespace()
	if err != nil {
		return "", err
	}
	if !isCapture {
		return "", fmt.Errorf("query: first predicate argument must be a capture in %s", name)
	}
	return arg, nil
}

func (p *queryParser) readPredicateTokenListUntilClose(captureErr string) ([]string, error) {
	var values []string
	for {
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) {
			return nil, fmt.Errorf("query: expected ')' to close predicate at position %d", p.pos)
		}
		if p.input[p.pos] == ')' {
			p.pos++
			return values, nil
		}
		value, kind, err := p.readPredicateToken()
		if err != nil {
			return nil, err
		}
		if kind == predicateArgCapture && captureErr != "" {
			return nil, errors.New(captureErr)
		}
		values = append(values, value)
	}
}

func (p *queryParser) readPredicateArgsUntilClose() ([]parsedPredicateArg, error) {
	var args []parsedPredicateArg
	for {
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) {
			return nil, fmt.Errorf("query: expected ')' to close predicate at position %d", p.pos)
		}
		if p.input[p.pos] == ')' {
			p.pos++
			return args, nil
		}
		value, kind, err := p.readPredicateToken()
		if err != nil {
			return nil, err
		}
		args = append(args, parsedPredicateArg{value: value, kind: kind})
	}
}

func (p *queryParser) parseEqualityPredicate(name string, kind queryPredicateType) (QueryPredicate, error) {
	left, err := p.readPredicateFirstCapture(name)
	if err != nil {
		return QueryPredicate{}, err
	}
	right, rightIsCapture, err := p.readPredicateArgAfterWhitespace()
	if err != nil {
		return QueryPredicate{}, err
	}
	if err := p.closePredicate(); err != nil {
		return QueryPredicate{}, err
	}
	pred := QueryPredicate{
		kind:        kind,
		leftCapture: left,
	}
	if rightIsCapture {
		pred.rightCapture = right
	} else {
		pred.literal = right
	}
	return pred, nil
}

func (p *queryParser) parseRegexPredicate(name string, kind queryPredicateType, compile func(string) (*regexp.Regexp, error), errLabel string) (QueryPredicate, error) {
	left, err := p.readPredicateFirstCapture(name)
	if err != nil {
		return QueryPredicate{}, err
	}
	pattern, patternIsCapture, err := p.readPredicateArgAfterWhitespace()
	if err != nil {
		return QueryPredicate{}, err
	}
	if err := p.closePredicate(); err != nil {
		return QueryPredicate{}, err
	}
	if patternIsCapture {
		return QueryPredicate{}, fmt.Errorf("query: %s second argument must be a string literal", name)
	}
	rx, err := compile(pattern)
	if err != nil {
		return QueryPredicate{}, fmt.Errorf("query: %s in %s: %w", errLabel, name, err)
	}
	return QueryPredicate{
		kind:        kind,
		leftCapture: left,
		literal:     pattern,
		regex:       rx,
	}, nil
}

func (p *queryParser) parseListPredicate(name string, kind queryPredicateType, emptyErr string) (QueryPredicate, error) {
	left, err := p.readPredicateFirstCapture(name)
	if err != nil {
		return QueryPredicate{}, err
	}
	values, err := p.readPredicateTokenListUntilClose(fmt.Sprintf("query: %s arguments after first must be non-capture literals", name))
	if err != nil {
		return QueryPredicate{}, err
	}
	if len(values) == 0 {
		return QueryPredicate{}, errors.New(emptyErr)
	}
	return QueryPredicate{
		kind:        kind,
		leftCapture: left,
		values:      values,
	}, nil
}

func (p *queryParser) parseNodeTypePredicate(name string) (QueryPredicate, error) {
	left, err := p.readPredicateFirstCapture(name)
	if err != nil {
		return QueryPredicate{}, err
	}
	types, err := p.readPredicateTokenListUntilClose(fmt.Sprintf("query: %s node type arguments must be non-capture identifiers", name))
	if err != nil {
		return QueryPredicate{}, err
	}
	if len(types) == 0 {
		return QueryPredicate{}, fmt.Errorf("query: %s requires at least one node type name", name)
	}
	kind := predicateHasAncestor
	switch name {
	case "#not-has-ancestor?":
		kind = predicateNotHasAncestor
	case "#has-parent?":
		kind = predicateHasParent
	case "#not-has-parent?":
		kind = predicateNotHasParent
	}
	return QueryPredicate{
		kind:        kind,
		leftCapture: left,
		values:      types,
	}, nil
}

func (p *queryParser) parseIsPredicate(name string) (QueryPredicate, error) {
	args, err := p.readPredicateArgsUntilClose()
	if err != nil {
		return QueryPredicate{}, err
	}
	if len(args) == 0 {
		return QueryPredicate{}, fmt.Errorf("query: %s requires arguments", name)
	}
	pred := QueryPredicate{kind: predicateIs}
	if name == "#is-not?" {
		pred.kind = predicateIsNot
	}
	if args[0].kind == predicateArgCapture {
		pred.leftCapture = args[0].value
		if len(args) < 2 {
			return QueryPredicate{}, fmt.Errorf("query: %s capture form requires a property argument", name)
		}
		if args[1].kind == predicateArgCapture {
			return QueryPredicate{}, fmt.Errorf("query: %s property argument cannot be a capture", name)
		}
		pred.property = args[1].value
		return pred, nil
	}
	pred.property = args[0].value
	if len(args) >= 2 {
		if args[1].kind != predicateArgCapture {
			return QueryPredicate{}, fmt.Errorf("query: %s second argument must be a capture when provided", name)
		}
		pred.leftCapture = args[1].value
	}
	return pred, nil
}

func (p *queryParser) parseSetPredicate() (QueryPredicate, error) {
	key, kind, err := p.readPredicateTokenAfterWhitespace()
	if err != nil {
		return QueryPredicate{}, err
	}
	if kind == predicateArgCapture {
		return QueryPredicate{}, fmt.Errorf("query: #set! key must be a non-capture token")
	}
	values, err := p.readPredicateTokenListUntilClose("")
	if err != nil {
		return QueryPredicate{}, err
	}
	return QueryPredicate{
		kind:    predicateSet,
		literal: key,
		values:  values,
	}, nil
}

func (p *queryParser) parseOffsetPredicate() (QueryPredicate, error) {
	capName, kind, err := p.readPredicateTokenAfterWhitespace()
	if err != nil {
		return QueryPredicate{}, err
	}
	if kind != predicateArgCapture {
		return QueryPredicate{}, fmt.Errorf("query: #offset! first argument must be a capture")
	}
	var nums [4]int
	for i := 0; i < 4; i++ {
		tok, tokKind, err := p.readPredicateTokenAfterWhitespace()
		if err != nil {
			return QueryPredicate{}, err
		}
		if tokKind == predicateArgCapture {
			return QueryPredicate{}, fmt.Errorf("query: #offset! numeric arguments must be literals")
		}
		n, convErr := strconv.Atoi(tok)
		if convErr != nil {
			return QueryPredicate{}, fmt.Errorf("query: #offset! invalid integer %q", tok)
		}
		nums[i] = n
	}
	if err := p.closePredicate(); err != nil {
		return QueryPredicate{}, err
	}
	return QueryPredicate{
		kind:        predicateOffset,
		leftCapture: capName,
		offset:      nums,
	}, nil
}

func (p *queryParser) parseSelectAdjacentPredicate() (QueryPredicate, error) {
	items, itemsIsCapture, err := p.readPredicateArgAfterWhitespace()
	if err != nil {
		return QueryPredicate{}, err
	}
	if !itemsIsCapture {
		return QueryPredicate{}, fmt.Errorf("query: #select-adjacent! first argument must be a capture")
	}
	anchor, anchorIsCapture, err := p.readPredicateArgAfterWhitespace()
	if err != nil {
		return QueryPredicate{}, err
	}
	if !anchorIsCapture {
		return QueryPredicate{}, fmt.Errorf("query: #select-adjacent! second argument must be a capture")
	}
	if err := p.closePredicate(); err != nil {
		return QueryPredicate{}, err
	}
	return QueryPredicate{
		kind:         predicateSelectAdjacent,
		leftCapture:  items,
		rightCapture: anchor,
	}, nil
}

func (p *queryParser) parseStripPredicate() (QueryPredicate, error) {
	capName, capIsCapture, err := p.readPredicateArgAfterWhitespace()
	if err != nil {
		return QueryPredicate{}, err
	}
	if !capIsCapture {
		return QueryPredicate{}, fmt.Errorf("query: #strip! first argument must be a capture")
	}
	pattern, patternIsCapture, err := p.readPredicateArgAfterWhitespace()
	if err != nil {
		return QueryPredicate{}, err
	}
	if patternIsCapture {
		return QueryPredicate{}, fmt.Errorf("query: #strip! second argument must be a string literal (regex)")
	}
	if err := p.closePredicate(); err != nil {
		return QueryPredicate{}, err
	}
	rx, err := regexp.Compile(pattern)
	if err != nil {
		return QueryPredicate{}, fmt.Errorf("query: invalid regex in #strip!: %w", err)
	}
	return QueryPredicate{
		kind:        predicateStrip,
		leftCapture: capName,
		literal:     pattern,
		regex:       rx,
	}, nil
}

func (p *queryParser) parseCountPredicate() (QueryPredicate, error) {
	capName, capKind, err := p.readPredicateTokenAfterWhitespace()
	if err != nil {
		return QueryPredicate{}, err
	}
	if capKind != predicateArgCapture {
		return QueryPredicate{}, fmt.Errorf("query: #count? first argument must be a capture")
	}
	op, opKind, err := p.readPredicateTokenAfterWhitespace()
	if err != nil {
		return QueryPredicate{}, err
	}
	if opKind == predicateArgCapture {
		return QueryPredicate{}, fmt.Errorf("query: #count? operator must be a string or atom, not a capture")
	}
	switch op {
	case ">", "<", ">=", "<=", "==", "!=":
	default:
		return QueryPredicate{}, fmt.Errorf("query: #count? invalid operator %q (expected >, <, >=, <=, ==, !=)", op)
	}
	valStr, valKind, err := p.readPredicateTokenAfterWhitespace()
	if err != nil {
		return QueryPredicate{}, err
	}
	if valKind == predicateArgCapture {
		return QueryPredicate{}, fmt.Errorf("query: #count? value must be a string or atom, not a capture")
	}
	val, convErr := strconv.Atoi(valStr)
	if convErr != nil {
		return QueryPredicate{}, fmt.Errorf("query: #count? invalid integer %q", valStr)
	}
	if err := p.closePredicate(); err != nil {
		return QueryPredicate{}, err
	}
	return QueryPredicate{
		kind:        predicateCount,
		leftCapture: capName,
		countOp:     op,
		countValue:  val,
	}, nil
}

func (p *queryParser) parseIsExportedPredicate() (QueryPredicate, error) {
	capName, capIsCapture, err := p.readPredicateArgAfterWhitespace()
	if err != nil {
		return QueryPredicate{}, err
	}
	if !capIsCapture {
		return QueryPredicate{}, fmt.Errorf("query: #is-exported? argument must be a capture")
	}
	if err := p.closePredicate(); err != nil {
		return QueryPredicate{}, err
	}
	return QueryPredicate{
		kind:        predicateIsExported,
		leftCapture: capName,
	}, nil
}

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

type predicateArgKind uint8

const (
	predicateArgCapture predicateArgKind = iota
	predicateArgString
	predicateArgAtom
)

func (p *queryParser) readPredicateToken() (arg string, kind predicateArgKind, err error) {
	if p.pos >= len(p.input) {
		return "", predicateArgAtom, fmt.Errorf("query: expected predicate argument at end of input")
	}

	switch p.input[p.pos] {
	case '@':
		name, err := p.readCapture()
		if err != nil {
			return "", predicateArgAtom, err
		}
		return name, predicateArgCapture, nil
	case '"':
		text, err := p.readString()
		if err != nil {
			return "", predicateArgAtom, err
		}
		return text, predicateArgString, nil
	default:
		start := p.pos
		for p.pos < len(p.input) {
			ch := p.input[p.pos]
			if ch == ')' || ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
				break
			}
			p.pos++
		}
		if p.pos == start {
			return "", predicateArgAtom, fmt.Errorf("query: expected predicate argument at position %d", p.pos)
		}
		return p.input[start:p.pos], predicateArgAtom, nil
	}
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
