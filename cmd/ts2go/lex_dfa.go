package main

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/odvcencio/gotreesitter"
)

const maxLookaheadRune = utf8.MaxRune

var ErrNoLexFunction = errors.New("ts_lex function not found")

// SymbolID is the grammar symbol identifier type used by lexer extraction.
type SymbolID = gotreesitter.Symbol

// LexState is one node in the extracted lexer DFA.
type LexState struct {
	Transitions []LexTransition
	Accept      SymbolID
	HasAccept   bool
	IsKeyword   bool
	EOF         int
}

// LexTransition is one DFA edge.
type LexTransition struct {
	Lo, Hi rune
	Next   int
	Skip   bool
}

// LexDFA contains the main and keyword lexer DFAs extracted from parser.c.
type LexDFA struct {
	States             []LexState
	KeywordStates      []LexState
	KeywordCapture     SymbolID
	HasKeywordCapture  bool
}

type lexRange struct {
	lo rune
	hi rune
}

type conditionResult struct {
	nonEOF []lexRange
	eof    bool
}

type cTokenKind int

const (
	cTokenEOF cTokenKind = iota
	cTokenIdent
	cTokenNumber
	cTokenChar
	cTokenLParen
	cTokenRParen
	cTokenComma
	cTokenAnd
	cTokenOr
	cTokenNot
	cTokenEq
	cTokenNe
	cTokenLt
	cTokenLe
	cTokenGt
	cTokenGe
)

type cToken struct {
	kind cTokenKind
	text string
}

type condParser struct {
	tokens   []cToken
	pos      int
	charSets map[string][]lexRange
}

type operandKind int

const (
	operandInvalid operandKind = iota
	operandValue
	operandCondition
)

type condOperand struct {
	kind      operandKind
	value     rune
	isLook    bool
	condition conditionResult
}

// ExtractLexDFA extracts lexer DFA data from a tree-sitter generated parser.c.
func ExtractLexDFA(source string) (*LexDFA, error) {
	enumValues := extractEnum(source)
	charSets, err := extractCharacterSets(source)
	if err != nil {
		return nil, err
	}

	states, err := extractLexFunctionStates(source, "ts_lex", enumValues, charSets)
	if err != nil {
		return nil, err
	}

	dfa := &LexDFA{
		States: states,
	}

	kwStates, err := extractLexFunctionStates(source, "ts_lex_keywords", enumValues, charSets)
	if err == nil {
		dfa.KeywordStates = kwStates
	}

	if sym, ok := extractKeywordCaptureToken(source, enumValues); ok {
		dfa.KeywordCapture = sym
		dfa.HasKeywordCapture = true
		if len(dfa.KeywordStates) > 0 {
			for i := range dfa.States {
				if dfa.States[i].HasAccept && dfa.States[i].Accept == sym {
					dfa.States[i].IsKeyword = true
				}
			}
		}
	}

	return dfa, nil
}

func extractKeywordCaptureToken(source string, enums map[string]int) (SymbolID, bool) {
	re := regexp.MustCompile(`\.keyword_capture_token\s*=\s*(\w+)`)
	m := re.FindStringSubmatch(source)
	if m == nil {
		return 0, false
	}
	if v, ok := resolveSymbolLocal(m[1], enums); ok {
		return SymbolID(v), true
	}
	return 0, false
}

func extractCharacterSets(source string) (map[string][]lexRange, error) {
	re := regexp.MustCompile(`(?m)static\s+(?:const\s+)?TSCharacterRange\s+(\w+)\s*\[[^\]]*\]\s*=\s*\{`)
	matches := re.FindAllStringSubmatch(source, -1)
	if len(matches) == 0 {
		return nil, nil
	}

	sets := make(map[string][]lexRange, len(matches))
	entryRe := regexp.MustCompile(`\{\s*([^,}]+)\s*,\s*([^}]+)\s*\}`)
	for _, m := range matches {
		name := m[1]
		body, err := findArrayBody(source, name)
		if err != nil {
			continue
		}

		entries := entryRe.FindAllStringSubmatch(body, -1)
		if len(entries) == 0 {
			continue
		}

		ranges := make([]lexRange, 0, len(entries))
		for _, e := range entries {
			lo, ok := parseCLookaheadLiteral(strings.TrimSpace(e[1]))
			if !ok {
				continue
			}
			hi, ok := parseCLookaheadLiteral(strings.TrimSpace(e[2]))
			if !ok {
				continue
			}
			if lo > hi {
				lo, hi = hi, lo
			}
			if hi < 0 || lo > maxLookaheadRune {
				continue
			}
			if lo < 0 {
				lo = 0
			}
			if hi > maxLookaheadRune {
				hi = maxLookaheadRune
			}
			ranges = append(ranges, lexRange{lo: lo, hi: hi})
		}
		sets[name] = normalizeRanges(ranges)
	}

	return sets, nil
}

func extractLexFunctionStates(source, funcName string, enums map[string]int, charSets map[string][]lexRange) ([]LexState, error) {
	body, _, ok := findFunctionBody(source, funcName)
	if !ok {
		if funcName == "ts_lex" {
			return nil, ErrNoLexFunction
		}
		return nil, fmt.Errorf("%s not found", funcName)
	}
	body = stripCComments(body)

	switchBody, ok := findSwitchBody(body)
	if !ok {
		return nil, fmt.Errorf("%s: switch(state) body not found", funcName)
	}

	caseRe := regexp.MustCompile(`(?m)case\s+(\d+)\s*:`)
	locs := caseRe.FindAllStringSubmatchIndex(switchBody, -1)
	if len(locs) == 0 {
		return nil, fmt.Errorf("%s: no case labels found", funcName)
	}

	statesByID := make(map[int]LexState, len(locs))
	maxID := 0
	for i, loc := range locs {
		id, _ := strconv.Atoi(switchBody[loc[2]:loc[3]])
		if id > maxID {
			maxID = id
		}
		start := loc[1]
		end := len(switchBody)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		block := switchBody[start:end]
		st, err := parseLexCaseBlock(block, enums, charSets)
		if err != nil {
			return nil, fmt.Errorf("%s case %d: %w", funcName, id, err)
		}
		statesByID[id] = st
	}

	states := make([]LexState, maxID+1)
	for i := range states {
		states[i].EOF = -1
	}
	for id, st := range statesByID {
		states[id] = st
	}
	return states, nil
}

func findSwitchBody(body string) (string, bool) {
	re := regexp.MustCompile(`(?m)\bswitch\s*\(\s*state\s*\)\s*\{`)
	loc := re.FindStringIndex(body)
	if loc == nil {
		return "", false
	}
	open := strings.Index(body[loc[0]:loc[1]], "{")
	if open < 0 {
		return "", false
	}
	open += loc[0]
	end, err := findMatchingBrace(body, open)
	if err != nil {
		return "", false
	}
	return body[open+1 : end], true
}

func parseLexCaseBlock(block string, enums map[string]int, charSets map[string][]lexRange) (LexState, error) {
	st := LexState{EOF: -1}

	for i := 0; i < len(block); {
		i = skipSpace(block, i)
		if i >= len(block) {
			break
		}

		if hasWordAt(block, i, "if") {
			next, err := parseConditionalTransition(block, i, &st, charSets)
			if err != nil {
				return st, err
			}
			i = next
			continue
		}

		action, next, err := parseLexAction(block, i)
		if err != nil {
			i++
			continue
		}
		i = next

		switch action.kind {
		case lexActionAccept:
			sym, ok := resolveSymbolLocal(strings.TrimSpace(action.arg), enums)
			if !ok {
				return st, fmt.Errorf("unknown accept symbol %q", action.arg)
			}
			st.Accept = SymbolID(sym)
			st.HasAccept = true
		case lexActionAdvance:
			addTransition(&st, conditionResult{nonEOF: universeRanges()}, action.next, false)
		case lexActionSkip:
			addTransition(&st, conditionResult{nonEOF: universeRanges()}, action.next, true)
		case lexActionAdvanceMap:
			for _, tr := range action.mapTransitions {
				st.Transitions = append(st.Transitions, tr)
			}
		case lexActionEndState:
			return st, nil
		}
	}

	return st, nil
}

func parseConditionalTransition(block string, start int, st *LexState, charSets map[string][]lexRange) (int, error) {
	i := start + 2 // len("if")
	i = skipSpace(block, i)
	if i >= len(block) || block[i] != '(' {
		return start + 1, fmt.Errorf("if without condition")
	}
	endCond, ok := findMatchingParen(block, i)
	if !ok {
		return start + 1, fmt.Errorf("unterminated if condition")
	}

	condExpr := strings.TrimSpace(block[i+1 : endCond])
	cond, err := parseConditionExpression(condExpr, charSets)
	if err != nil {
		return start + 1, err
	}

	j := skipSpace(block, endCond+1)
	action, next, err := parseLexAction(block, j)
	if err != nil {
		return start + 1, err
	}

	switch action.kind {
	case lexActionAdvance:
		addTransition(st, cond, action.next, false)
	case lexActionSkip:
		addTransition(st, cond, action.next, true)
	default:
		return start + 1, fmt.Errorf("unsupported conditional action %q", action.kind)
	}
	return next, nil
}

func addTransition(st *LexState, cond conditionResult, next int, skip bool) {
	if cond.eof && st.EOF < 0 {
		st.EOF = next
	}
	for _, r := range cond.nonEOF {
		st.Transitions = append(st.Transitions, LexTransition{
			Lo:   r.lo,
			Hi:   r.hi,
			Next: next,
			Skip: skip,
		})
	}
}

type lexActionKind string

const (
	lexActionUnknown    lexActionKind = "unknown"
	lexActionAdvance    lexActionKind = "advance"
	lexActionSkip       lexActionKind = "skip"
	lexActionAccept     lexActionKind = "accept"
	lexActionAdvanceMap lexActionKind = "advance_map"
	lexActionEndState   lexActionKind = "end_state"
)

type lexAction struct {
	kind           lexActionKind
	next           int
	arg            string
	mapTransitions []LexTransition
}

func parseLexAction(block string, start int) (lexAction, int, error) {
	name, args, next, err := parseMacroCall(block, start)
	if err != nil {
		return lexAction{}, start, err
	}

	switch name {
	case "ADVANCE":
		n, err := strconv.Atoi(strings.TrimSpace(args))
		if err != nil {
			return lexAction{}, start, fmt.Errorf("ADVANCE arg: %w", err)
		}
		return lexAction{kind: lexActionAdvance, next: n}, next, nil
	case "SKIP":
		n, err := strconv.Atoi(strings.TrimSpace(args))
		if err != nil {
			return lexAction{}, start, fmt.Errorf("SKIP arg: %w", err)
		}
		return lexAction{kind: lexActionSkip, next: n}, next, nil
	case "ACCEPT_TOKEN":
		return lexAction{kind: lexActionAccept, arg: strings.TrimSpace(args)}, next, nil
	case "END_STATE":
		return lexAction{kind: lexActionEndState}, next, nil
	case "ADVANCE_MAP":
		pairs, err := parseAdvanceMapArgs(args)
		if err != nil {
			return lexAction{}, start, err
		}
		return lexAction{kind: lexActionAdvanceMap, mapTransitions: pairs}, next, nil
	default:
		return lexAction{kind: lexActionUnknown}, next, nil
	}
}

func parseAdvanceMapArgs(args string) ([]LexTransition, error) {
	parts := splitCSV(args)
	if len(parts)%2 != 0 {
		return nil, fmt.Errorf("ADVANCE_MAP expects key/state pairs")
	}
	transitions := make([]LexTransition, 0, len(parts)/2)
	for i := 0; i < len(parts); i += 2 {
		ch, ok := parseCLookaheadLiteral(parts[i])
		if !ok {
			return nil, fmt.Errorf("invalid ADVANCE_MAP key %q", parts[i])
		}
		n, err := strconv.Atoi(strings.TrimSpace(parts[i+1]))
		if err != nil {
			return nil, fmt.Errorf("invalid ADVANCE_MAP next state %q", parts[i+1])
		}
		transitions = append(transitions, LexTransition{
			Lo:   ch,
			Hi:   ch,
			Next: n,
			Skip: false,
		})
	}
	return transitions, nil
}

func splitCSV(s string) []string {
	var out []string
	start := 0
	inChar := false
	escaped := false
	depth := 0

	flush := func(end int) {
		part := strings.TrimSpace(s[start:end])
		if part != "" {
			out = append(out, part)
		}
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		if inChar {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '\'' {
				inChar = false
			}
			continue
		}

		switch c {
		case '\'':
			inChar = true
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				flush(i)
				start = i + 1
			}
		}
	}

	if start <= len(s) {
		flush(len(s))
	}

	return out
}

func parseMacroCall(s string, start int) (name, args string, next int, err error) {
	i := skipSpace(s, start)
	if i >= len(s) {
		return "", "", start, fmt.Errorf("eof")
	}
	if !isIdentStart(s[i]) {
		return "", "", start, fmt.Errorf("expected macro name")
	}
	j := i + 1
	for j < len(s) && isIdentPart(s[j]) {
		j++
	}
	name = s[i:j]
	j = skipSpace(s, j)
	if j >= len(s) || s[j] != '(' {
		return "", "", start, fmt.Errorf("expected '(' after %s", name)
	}
	end, ok := findMatchingParen(s, j)
	if !ok {
		return "", "", start, fmt.Errorf("unterminated %s(...)", name)
	}
	args = s[j+1 : end]
	k := skipSpace(s, end+1)
	if k < len(s) && s[k] == ';' {
		k++
	}
	return name, args, k, nil
}

func parseConditionExpression(expr string, charSets map[string][]lexRange) (conditionResult, error) {
	toks, err := lexCondition(expr)
	if err != nil {
		return conditionResult{}, err
	}
	p := &condParser{
		tokens:   toks,
		charSets: charSets,
	}
	result, err := p.parseOr()
	if err != nil {
		return conditionResult{}, err
	}
	if p.peek().kind != cTokenEOF {
		return conditionResult{}, fmt.Errorf("unexpected token %q", p.peek().text)
	}
	return result, nil
}

func lexCondition(src string) ([]cToken, error) {
	toks := make([]cToken, 0, 16)
	for i := 0; i < len(src); {
		c := src[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}
		if isIdentStart(c) {
			j := i + 1
			for j < len(src) && isIdentPart(src[j]) {
				j++
			}
			toks = append(toks, cToken{kind: cTokenIdent, text: src[i:j]})
			i = j
			continue
		}
		if isDigit(c) {
			j := i + 1
			if c == '0' && j < len(src) && (src[j] == 'x' || src[j] == 'X') {
				j++
				for j < len(src) && isHex(src[j]) {
					j++
				}
			} else {
				for j < len(src) && isDigit(src[j]) {
					j++
				}
			}
			toks = append(toks, cToken{kind: cTokenNumber, text: src[i:j]})
			i = j
			continue
		}
		if c == '\'' {
			j := i + 1
			escaped := false
			for j < len(src) {
				if escaped {
					escaped = false
					j++
					continue
				}
				if src[j] == '\\' {
					escaped = true
					j++
					continue
				}
				if src[j] == '\'' {
					j++
					break
				}
				j++
			}
			if j > len(src) || src[j-1] != '\'' {
				return nil, fmt.Errorf("unterminated char literal")
			}
			toks = append(toks, cToken{kind: cTokenChar, text: src[i:j]})
			i = j
			continue
		}

		if strings.HasPrefix(src[i:], "&&") {
			toks = append(toks, cToken{kind: cTokenAnd, text: "&&"})
			i += 2
			continue
		}
		if strings.HasPrefix(src[i:], "||") {
			toks = append(toks, cToken{kind: cTokenOr, text: "||"})
			i += 2
			continue
		}
		if strings.HasPrefix(src[i:], "==") {
			toks = append(toks, cToken{kind: cTokenEq, text: "=="})
			i += 2
			continue
		}
		if strings.HasPrefix(src[i:], "!=") {
			toks = append(toks, cToken{kind: cTokenNe, text: "!="})
			i += 2
			continue
		}
		if strings.HasPrefix(src[i:], "<=") {
			toks = append(toks, cToken{kind: cTokenLe, text: "<="})
			i += 2
			continue
		}
		if strings.HasPrefix(src[i:], ">=") {
			toks = append(toks, cToken{kind: cTokenGe, text: ">="})
			i += 2
			continue
		}

		switch c {
		case '!':
			toks = append(toks, cToken{kind: cTokenNot, text: "!"})
		case '<':
			toks = append(toks, cToken{kind: cTokenLt, text: "<"})
		case '>':
			toks = append(toks, cToken{kind: cTokenGt, text: ">"})
		case '(':
			toks = append(toks, cToken{kind: cTokenLParen, text: "("})
		case ')':
			toks = append(toks, cToken{kind: cTokenRParen, text: ")"})
		case ',':
			toks = append(toks, cToken{kind: cTokenComma, text: ","})
		default:
			return nil, fmt.Errorf("unsupported token starting at %q", src[i:])
		}
		i++
	}
	toks = append(toks, cToken{kind: cTokenEOF})
	return toks, nil
}

func (p *condParser) parseOr() (conditionResult, error) {
	left, err := p.parseAnd()
	if err != nil {
		return conditionResult{}, err
	}
	for p.peek().kind == cTokenOr {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return conditionResult{}, err
		}
		left = conditionResult{
			nonEOF: unionRanges(left.nonEOF, right.nonEOF),
			eof:    left.eof || right.eof,
		}
	}
	return left, nil
}

func (p *condParser) parseAnd() (conditionResult, error) {
	left, err := p.parseUnary()
	if err != nil {
		return conditionResult{}, err
	}
	for p.peek().kind == cTokenAnd {
		p.next()
		right, err := p.parseUnary()
		if err != nil {
			return conditionResult{}, err
		}
		left = conditionResult{
			nonEOF: intersectRanges(left.nonEOF, right.nonEOF),
			eof:    left.eof && right.eof,
		}
	}
	return left, nil
}

func (p *condParser) parseUnary() (conditionResult, error) {
	if p.peek().kind == cTokenNot {
		p.next()
		sub, err := p.parseUnary()
		if err != nil {
			return conditionResult{}, err
		}
		return conditionResult{
			nonEOF: complementRanges(sub.nonEOF),
			eof:    !sub.eof,
		}, nil
	}
	return p.parsePrimary()
}

func (p *condParser) parsePrimary() (conditionResult, error) {
	if p.peek().kind == cTokenLParen {
		p.next()
		v, err := p.parseOr()
		if err != nil {
			return conditionResult{}, err
		}
		if p.peek().kind != cTokenRParen {
			return conditionResult{}, fmt.Errorf("expected ')'")
		}
		p.next()
		return v, nil
	}

	left, err := p.parseAtom()
	if err != nil {
		return conditionResult{}, err
	}

	switch p.peek().kind {
	case cTokenEq, cTokenNe, cTokenLt, cTokenLe, cTokenGt, cTokenGe:
		op := p.next()
		right, err := p.parseAtom()
		if err != nil {
			return conditionResult{}, err
		}
		return compareOperands(left, op.kind, right)
	default:
		if left.kind != operandCondition {
			return conditionResult{}, fmt.Errorf("value used as condition")
		}
		return left.condition, nil
	}
}

func (p *condParser) parseAtom() (condOperand, error) {
	tok := p.peek()
	switch tok.kind {
	case cTokenIdent:
		p.next()
		switch tok.text {
		case "eof":
			return condOperand{
				kind: operandCondition,
				condition: conditionResult{
					nonEOF: nil,
					eof:    true,
				},
			}, nil
		case "lookahead":
			return condOperand{
				kind:   operandValue,
				isLook: true,
			}, nil
		case "set_contains":
			return p.parseSetContains()
		default:
			return condOperand{}, fmt.Errorf("unknown identifier %q", tok.text)
		}
	case cTokenNumber:
		p.next()
		v, ok := parseCLookaheadLiteral(tok.text)
		if !ok {
			return condOperand{}, fmt.Errorf("invalid number literal %q", tok.text)
		}
		return condOperand{
			kind:  operandValue,
			value: v,
		}, nil
	case cTokenChar:
		p.next()
		v, ok := parseCLookaheadLiteral(tok.text)
		if !ok {
			return condOperand{}, fmt.Errorf("invalid char literal %q", tok.text)
		}
		return condOperand{
			kind:  operandValue,
			value: v,
		}, nil
	default:
		return condOperand{}, fmt.Errorf("unexpected token %q", tok.text)
	}
}

func (p *condParser) parseSetContains() (condOperand, error) {
	if p.peek().kind != cTokenLParen {
		return condOperand{}, fmt.Errorf("set_contains expects '('")
	}
	p.next()

	setNameTok := p.next()
	if setNameTok.kind != cTokenIdent {
		return condOperand{}, fmt.Errorf("set_contains expects set name")
	}
	if p.peek().kind != cTokenComma {
		return condOperand{}, fmt.Errorf("set_contains missing comma")
	}
	p.next()

	// Length arg; we don't need it for extraction.
	lenTok := p.next()
	if lenTok.kind != cTokenNumber {
		return condOperand{}, fmt.Errorf("set_contains expects numeric length")
	}
	if p.peek().kind != cTokenComma {
		return condOperand{}, fmt.Errorf("set_contains missing second comma")
	}
	p.next()

	lookTok := p.next()
	if lookTok.kind != cTokenIdent || lookTok.text != "lookahead" {
		return condOperand{}, fmt.Errorf("set_contains expects lookahead argument")
	}
	if p.peek().kind != cTokenRParen {
		return condOperand{}, fmt.Errorf("set_contains expects ')'")
	}
	p.next()

	ranges := p.charSets[setNameTok.text]
	if len(ranges) == 0 {
		return condOperand{}, fmt.Errorf("unknown or empty character set %q", setNameTok.text)
	}

	return condOperand{
		kind: operandCondition,
		condition: conditionResult{
			nonEOF: ranges,
			eof:    false,
		},
	}, nil
}

func compareOperands(left condOperand, op cTokenKind, right condOperand) (conditionResult, error) {
	if left.kind != operandValue || right.kind != operandValue {
		return conditionResult{}, fmt.Errorf("comparison requires values")
	}
	if left.isLook == right.isLook {
		return conditionResult{}, fmt.Errorf("comparison must involve lookahead and literal")
	}

	var ranges []lexRange
	if left.isLook {
		ranges = rangesForComparison(op, right.value)
	} else {
		// literal OP lookahead  => invert direction
		inv, ok := invertComparison(op)
		if !ok {
			return conditionResult{}, fmt.Errorf("unsupported comparison")
		}
		ranges = rangesForComparison(inv, left.value)
	}

	return conditionResult{
		nonEOF: ranges,
		eof:    false,
	}, nil
}

func rangesForComparison(op cTokenKind, lit rune) []lexRange {
	if lit < 0 {
		lit = 0
	}
	if lit > maxLookaheadRune {
		lit = maxLookaheadRune
	}

	switch op {
	case cTokenEq:
		return []lexRange{{lo: lit, hi: lit}}
	case cTokenNe:
		return complementRanges([]lexRange{{lo: lit, hi: lit}})
	case cTokenLt:
		if lit <= 0 {
			return nil
		}
		return []lexRange{{lo: 0, hi: lit - 1}}
	case cTokenLe:
		return []lexRange{{lo: 0, hi: lit}}
	case cTokenGt:
		if lit >= maxLookaheadRune {
			return nil
		}
		return []lexRange{{lo: lit + 1, hi: maxLookaheadRune}}
	case cTokenGe:
		return []lexRange{{lo: lit, hi: maxLookaheadRune}}
	default:
		return nil
	}
}

func invertComparison(op cTokenKind) (cTokenKind, bool) {
	switch op {
	case cTokenEq:
		return cTokenEq, true
	case cTokenNe:
		return cTokenNe, true
	case cTokenLt:
		return cTokenGt, true
	case cTokenLe:
		return cTokenGe, true
	case cTokenGt:
		return cTokenLt, true
	case cTokenGe:
		return cTokenLe, true
	default:
		return cTokenEOF, false
	}
}

func (p *condParser) peek() cToken {
	if p.pos >= len(p.tokens) {
		return cToken{kind: cTokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *condParser) next() cToken {
	t := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return t
}

func universeRanges() []lexRange {
	return []lexRange{{lo: 0, hi: maxLookaheadRune}}
}

func normalizeRanges(in []lexRange) []lexRange {
	if len(in) == 0 {
		return nil
	}
	ranges := make([]lexRange, 0, len(in))
	for _, r := range in {
		if r.hi < r.lo {
			continue
		}
		if r.hi < 0 || r.lo > maxLookaheadRune {
			continue
		}
		if r.lo < 0 {
			r.lo = 0
		}
		if r.hi > maxLookaheadRune {
			r.hi = maxLookaheadRune
		}
		ranges = append(ranges, r)
	}
	if len(ranges) == 0 {
		return nil
	}

	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].lo != ranges[j].lo {
			return ranges[i].lo < ranges[j].lo
		}
		return ranges[i].hi < ranges[j].hi
	})

	out := make([]lexRange, 0, len(ranges))
	cur := ranges[0]
	for _, r := range ranges[1:] {
		if r.lo <= cur.hi+1 {
			if r.hi > cur.hi {
				cur.hi = r.hi
			}
			continue
		}
		out = append(out, cur)
		cur = r
	}
	out = append(out, cur)
	return out
}

func unionRanges(a, b []lexRange) []lexRange {
	if len(a) == 0 {
		return normalizeRanges(b)
	}
	if len(b) == 0 {
		return normalizeRanges(a)
	}
	merged := make([]lexRange, 0, len(a)+len(b))
	merged = append(merged, a...)
	merged = append(merged, b...)
	return normalizeRanges(merged)
}

func intersectRanges(a, b []lexRange) []lexRange {
	a = normalizeRanges(a)
	b = normalizeRanges(b)
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	out := make([]lexRange, 0, min(len(a), len(b)))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		lo := a[i].lo
		if b[j].lo > lo {
			lo = b[j].lo
		}
		hi := a[i].hi
		if b[j].hi < hi {
			hi = b[j].hi
		}
		if lo <= hi {
			out = append(out, lexRange{lo: lo, hi: hi})
		}
		if a[i].hi < b[j].hi {
			i++
		} else {
			j++
		}
	}
	return out
}

func complementRanges(in []lexRange) []lexRange {
	ranges := normalizeRanges(in)
	if len(ranges) == 0 {
		return universeRanges()
	}
	out := make([]lexRange, 0, len(ranges)+1)
	cur := rune(0)
	for _, r := range ranges {
		if cur < r.lo {
			out = append(out, lexRange{lo: cur, hi: r.lo - 1})
		}
		if r.hi == maxLookaheadRune {
			cur = maxLookaheadRune + 1
			break
		}
		cur = r.hi + 1
	}
	if cur <= maxLookaheadRune {
		out = append(out, lexRange{lo: cur, hi: maxLookaheadRune})
	}
	return out
}

func parseCLookaheadLiteral(s string) (rune, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'") && len(s) >= 2 {
		return parseCChar(s[1 : len(s)-1])
	}
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		v, err := strconv.ParseInt(s[2:], 16, 32)
		if err != nil {
			return 0, false
		}
		return rune(v), true
	}
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, false
	}
	return rune(v), true
}

func resolveSymbolLocal(s string, enums map[string]int) (int, bool) {
	if n, err := strconv.Atoi(s); err == nil {
		return n, true
	}
	if v, ok := enums[s]; ok {
		return v, true
	}
	return 0, false
}

func hasWordAt(s string, i int, word string) bool {
	if i < 0 || i+len(word) > len(s) || s[i:i+len(word)] != word {
		return false
	}
	beforeOK := i == 0 || !isIdentPart(s[i-1])
	after := i + len(word)
	afterOK := after >= len(s) || !isIdentPart(s[after])
	return beforeOK && afterOK
}

func skipSpace(s string, i int) int {
	for i < len(s) {
		switch s[i] {
		case ' ', '\t', '\n', '\r':
			i++
		default:
			return i
		}
	}
	return i
}

func isIdentStart(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

func isIdentPart(b byte) bool {
	return isIdentStart(b) || (b >= '0' && b <= '9')
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func isHex(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}
