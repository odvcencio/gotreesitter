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
	States            []LexState
	KeywordStates     []LexState
	KeywordCapture    SymbolID
	HasKeywordCapture bool
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
	cTokenQuestion
	cTokenColon
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

	// Also extract character_set functions (newer tree-sitter format).
	funcSets, err := extractCharacterSetFunctions(source)
	if err != nil {
		return nil, err
	}
	if len(funcSets) > 0 {
		if charSets == nil {
			charSets = funcSets
		} else {
			for k, v := range funcSets {
				charSets[k] = v
			}
		}
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

// characterRangeDeclRe matches the head of a character range array as the
// tree-sitter generator writes it: at the start of a line, one of three
// storage prefixes, the type, one array name, empty brackets and the opening
// brace. Group 1 is the prefix and group 2 is the array name. It is applied
// to a copy of the source in which comments and the bodies of string and
// character literals are blanked.
var characterRangeDeclRe = regexp.MustCompile(`(?m)^(static const |static |const )TSCharacterRange ([A-Za-z_][A-Za-z0-9_]*)\[\] = \{`)

// characterRangeTypeRe matches the type name as a whole identifier.
var characterRangeTypeRe = regexp.MustCompile(`\bTSCharacterRange\b`)

// characterRangeArray is one array read from a parser.c.
type characterRangeArray struct {
	name   string
	offset int // byte offset of the declaration
	ranges []lexRange
}

// precheckCharacterRangeSource refuses text that changes the meaning of the
// source before comments and tokens are recognized: a backslash followed by a
// line break, a NUL byte, a carriage return that does not start a CRLF pair,
// and a trigraph. The error gives the byte offset and no array name.
func precheckCharacterRangeSource(source string) error {
	for i := 0; i < len(source); i++ {
		switch source[i] {
		case '\\':
			if i+1 < len(source) && (source[i+1] == '\n' || source[i+1] == '\r') {
				return fmt.Errorf("byte offset %d: a backslash followed by a line break is not supported", i)
			}
		case 0:
			return fmt.Errorf("byte offset %d: NUL byte", i)
		case '\r':
			if i+1 >= len(source) || source[i+1] != '\n' {
				return fmt.Errorf("byte offset %d: carriage return without a line feed", i)
			}
		case '?':
			if i+2 < len(source) && source[i+1] == '?' && strings.IndexByte("=/'()!<>-", source[i+2]) >= 0 {
				return fmt.Errorf("byte offset %d: trigraph", i)
			}
		}
	}
	return nil
}

// blankCommentsAndLiterals returns a copy of source with the same length in
// which every comment, and the characters between the quotes of every string
// and character literal, are replaced by spaces (line feeds stay). Byte
// offsets are therefore the same as in source. It returns an error that gives
// the byte offset when a comment or literal is not closed.
func blankCommentsAndLiterals(source string) (string, error) {
	out := []byte(source)
	blank := func(from, to int) {
		for k := from; k < to; k++ {
			if out[k] != '\n' && out[k] != '\r' {
				out[k] = ' '
			}
		}
	}
	for i := 0; i < len(source); {
		switch c := source[i]; {
		case c == '/' && i+1 < len(source) && source[i+1] == '/':
			end := strings.IndexByte(source[i:], '\n')
			if end < 0 {
				end = len(source) - i
			}
			blank(i, i+end)
			i += end
		case c == '/' && i+1 < len(source) && source[i+1] == '*':
			end := strings.Index(source[i+2:], "*/")
			if end < 0 {
				return "", fmt.Errorf("byte offset %d: comment is not closed", i)
			}
			blank(i, i+end+4)
			i += end + 4
		case c == '"' || c == '\'':
			if c == '\'' && i > 0 && isIdentPart(source[i-1]) {
				return "", fmt.Errorf("byte offset %d: an apostrophe after a letter, digit or underscore (a digit separator or a prefixed character literal) is not supported", i)
			}
			j := i + 1
			for j < len(source) && source[j] != c {
				if source[j] == '\n' {
					return "", fmt.Errorf("byte offset %d: quoted literal is not closed", i)
				}
				if source[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(source) {
				return "", fmt.Errorf("byte offset %d: quoted literal is not closed", i)
			}
			blank(i+1, j)
			i = j + 1
		default:
			i++
		}
	}
	return string(out), nil
}

// readCharacterRangeArrays reads every character range array in source.
//
// It accepts only the text that the generator wrote in the pinned sources:
//
//	array = head "\n  " line { ",\n  " line } ",\n};\n"
//	head  = ("static const " | "static " | "const ") "TSCharacterRange " name "[] = {"
//	line  = entry { ", " entry }          (8 entries; the last line has 1 to 8)
//	entry = "{" bound ", " bound "}"
//	bound = char | "0x" hexdigit{2,6} | "0"
//	char  = "'" ( printable ASCII except ' and \ | "\\" | "\'" | "\t" | "\r" | "\n" | "\f" ) "'"
//	name  = [A-Za-z_][A-Za-z0-9_]*       hexdigit = 0-9 or a-f
//
// Line breaks are LF. Values lie in 0..0x10ffff and lo is not above hi.
// Character literals are read as ASCII, which is their value when the source
// is compiled for an ASCII-compatible execution character set. A head is
// looked for only outside comments and quoted literals. The source is refused
// when the type name occurs anywhere else, and before any array is read when
// it holds a backslash-newline, a NUL, a lone carriage return, a trigraph, or
// an apostrophe that follows a letter, digit or underscore. A declaration
// starts at column zero. Two arrays with one name refuse the source; the error
// gives the byte offset of the second declaration.
// Errors from inside an array name the array and give a byte offset; errors
// from the checks before and between arrays give only a byte offset.
func readCharacterRangeArrays(source string) ([]characterRangeArray, error) {
	if err := precheckCharacterRangeSource(source); err != nil {
		return nil, err
	}
	blanked, err := blankCommentsAndLiterals(source)
	if err != nil {
		return nil, err
	}
	matches := characterRangeDeclRe.FindAllStringSubmatchIndex(blanked, -1)
	declared := make(map[int]bool, len(matches))
	for _, m := range matches {
		declared[m[3]] = true // offset of the type name after the prefix
	}
	for _, loc := range characterRangeTypeRe.FindAllStringIndex(source, -1) {
		if !declared[loc[0]] {
			return nil, fmt.Errorf("byte offset %d: TSCharacterRange is not part of a recognized array declaration", loc[0])
		}
	}
	arrays := make([]characterRangeArray, 0, len(matches))
	for _, m := range matches {
		name := source[m[4]:m[5]]
		ranges, err := readCharacterRangeArray(source, name, m[1])
		if err != nil {
			return nil, err
		}
		arrays = append(arrays, characterRangeArray{name: name, offset: m[0], ranges: ranges})
	}
	return arrays, nil
}

// extractCharacterSets reads every character range array in source and
// returns the normalized ranges by array name. See readCharacterRangeArrays
// for the accepted text.
func extractCharacterSets(source string) (map[string][]lexRange, error) {
	arrays, err := readCharacterRangeArrays(source)
	if err != nil {
		return nil, err
	}
	if len(arrays) == 0 {
		return nil, nil
	}
	sets := make(map[string][]lexRange, len(arrays))
	for _, a := range arrays {
		if _, dup := sets[a.name]; dup {
			return nil, fmt.Errorf("byte offset %d: character range array %q is declared twice", a.offset, a.name)
		}
		sets[a.name] = normalizeRanges(a.ranges)
	}
	return sets, nil
}

// readCharacterRangeArray reads the entries of one array. pos is the byte
// offset just after the opening brace of the array.
func readCharacterRangeArray(source, name string, pos int) ([]lexRange, error) {
	fail := func(at int, format string, args ...any) error {
		return fmt.Errorf("character range array %q: byte offset %d: %s", name, at, fmt.Sprintf(format, args...))
	}
	expect := func(at int, text string) bool { return strings.HasPrefix(source[at:], text) }
	if !expect(pos, "\n  {") {
		return nil, fail(pos, "expected a line break, two spaces and '{' after the opening brace")
	}
	pos += len("\n  ")
	var ranges []lexRange
	onLine := 0
	for {
		entryStart := pos
		pos++ // '{'
		lo, next, err := readCRangeValue(source, pos)
		if err != nil {
			return nil, fail(pos, "first value: %v", err)
		}
		pos = next
		if !expect(pos, ", ") {
			return nil, fail(pos, "expected ', ' after the first value")
		}
		pos += 2
		hi, next, err := readCRangeValue(source, pos)
		if err != nil {
			return nil, fail(pos, "second value: %v", err)
		}
		pos = next
		if !expect(pos, "}") {
			return nil, fail(pos, "expected '}' after the second value")
		}
		pos++
		if lo > hi {
			return nil, fail(entryStart, "range start %#x is above range end %#x", lo, hi)
		}
		ranges = append(ranges, lexRange{lo: lo, hi: hi})
		onLine++

		switch {
		case onLine < 8 && expect(pos, ", {"):
			pos += 2
		case onLine == 8 && expect(pos, ",\n  {"):
			pos += 4
			onLine = 0
		case expect(pos, ",\n};\n"):
			return ranges, nil
		default:
			return nil, fail(pos, "expected ', {' (fewer than 8 entries on the line), ',' and a new line with two spaces and '{' (after 8 entries), or ',' then '};' and a line break")
		}
	}
}

// readCRangeValue reads one bound at pos and returns its value and the offset
// after it. The accepted spellings are listed at readCharacterRangeArrays.
func readCRangeValue(s string, pos int) (rune, int, error) {
	if pos >= len(s) {
		return 0, pos, fmt.Errorf("end of input")
	}
	switch c := s[pos]; {
	case c == '\'':
		return readCCharLiteral(s, pos)
	case strings.HasPrefix(s[pos:], "0x"):
		end := pos + 2
		for end < len(s) && (s[end] >= '0' && s[end] <= '9' || s[end] >= 'a' && s[end] <= 'f') {
			end++
		}
		if n := end - pos - 2; n < 2 || n > 6 {
			return 0, pos, fmt.Errorf("hexadecimal literal must have 2 to 6 lower-case digits")
		}
		v, _ := strconv.ParseInt(s[pos+2:end], 16, 32)
		if v > maxLookaheadRune {
			return 0, pos, fmt.Errorf("value %#x is above %#x", v, maxLookaheadRune)
		}
		return rune(v), end, nil
	case c == '0':
		return 0, pos + 1, nil
	}
	return 0, pos, fmt.Errorf("no bound at %q", s[pos:min(len(s), pos+8)])
}

// readCCharLiteral reads a character literal whose opening quote is at pos:
// one printable ASCII character other than the quote and the backslash, or
// one of the escapes \\ \' \t \r \n \f. The value is the ASCII code. That is
// the value a C compiler gives when the execution character set is
// ASCII-compatible; the tool supports generated sources compiled that way.
func readCCharLiteral(s string, pos int) (rune, int, error) {
	i := pos + 1
	if i >= len(s) {
		return 0, pos, fmt.Errorf("unterminated character literal")
	}
	var v rune
	switch c := s[i]; {
	case c == '\\':
		if i+1 >= len(s) {
			return 0, pos, fmt.Errorf("unterminated escape")
		}
		e, ok := map[byte]rune{'\\': '\\', '\'': '\'', 't': '\t', 'r': '\r', 'n': '\n', 'f': '\f'}[s[i+1]]
		if !ok {
			return 0, pos, fmt.Errorf("escape \\%c is not accepted", s[i+1])
		}
		v, i = e, i+2
	case c >= 0x20 && c <= 0x7e && c != '\'':
		v, i = rune(c), i+1
	default:
		return 0, pos, fmt.Errorf("character %#x is not accepted inside quotes", c)
	}
	if i >= len(s) || s[i] != '\'' {
		return 0, pos, fmt.Errorf("character literal is not closed")
	}
	return v, i + 1, nil
}

// extractCharacterSetFunctions finds `static inline bool xxx_character_set_N(int32_t c)`
// function definitions in newer tree-sitter grammars and parses their bodies into
// character ranges. These are semantically equivalent to TSCharacterRange arrays but
// encoded as nested ternary expressions.
func extractCharacterSetFunctions(source string) (map[string][]lexRange, error) {
	re := regexp.MustCompile(`(?m)static\s+(?:inline\s+)?bool\s+(\w+character_set_\d+)\s*\(\s*int32_t\s+(\w+)\s*\)\s*\{`)
	locs := re.FindAllStringSubmatchIndex(source, -1)
	if len(locs) == 0 {
		return nil, nil
	}

	sets := make(map[string][]lexRange, len(locs))
	for _, loc := range locs {
		name := source[loc[2]:loc[3]]
		paramName := source[loc[4]:loc[5]]
		bodyStart := loc[1]

		// Find matching closing brace, skipping char/string literals.
		depth := 1
		i := bodyStart
		for i < len(source) && depth > 0 {
			switch source[i] {
			case '\'':
				// Skip character literal (e.g., '}', '{', '\\')
				i++
				for i < len(source) {
					if source[i] == '\\' {
						i += 2
						continue
					}
					if source[i] == '\'' {
						i++
						break
					}
					i++
				}
				continue
			case '"':
				// Skip string literal
				i++
				for i < len(source) {
					if source[i] == '\\' {
						i += 2
						continue
					}
					if source[i] == '"' {
						i++
						break
					}
					i++
				}
				continue
			case '{':
				depth++
			case '}':
				depth--
			}
			i++
		}
		if depth != 0 {
			continue
		}
		body := source[bodyStart : i-1]

		// Extract the return expression.
		retIdx := strings.Index(body, "return")
		if retIdx < 0 {
			continue
		}
		expr := strings.TrimSpace(body[retIdx+6:])
		expr = strings.TrimSuffix(expr, ";")
		expr = strings.TrimSpace(expr)

		// Normalize parameter name to 'c' if different.
		if paramName != "c" {
			expr = replaceWholeWord(expr, paramName, "c")
		}

		ranges, err := parseCharSetExpr(expr)
		if err != nil {
			continue // skip unparseable functions
		}
		sets[name] = normalizeRanges(ranges)
	}
	return sets, nil
}

// charSetExprParser parses the nested ternary expressions generated by tree-sitter
// for character_set functions. The general structure is a binary search tree:
//
//	(c < pivot ? left_expr : right_expr)
//
// with leaf conditions like:
//
//	(c >= lo && c <= hi), c == val, c <= val
type charSetExprParser struct {
	tokens []cToken
	pos    int
}

func (p *charSetExprParser) peek() cToken {
	if p.pos >= len(p.tokens) {
		return cToken{kind: cTokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *charSetExprParser) next() cToken {
	t := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return t
}

// parseCharSetExpr tokenizes and parses a character_set function body expression.
func parseCharSetExpr(expr string) ([]lexRange, error) {
	toks, err := tokenizeCharSetExpr(expr)
	if err != nil {
		return nil, fmt.Errorf("tokenize: %w", err)
	}
	p := &charSetExprParser{tokens: toks}
	return p.parseOr()
}

// tokenizeCharSetExpr extends the existing C condition tokenizer with ? and : tokens.
func tokenizeCharSetExpr(src string) ([]cToken, error) {
	toks := make([]cToken, 0, 64)
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
		case '<':
			toks = append(toks, cToken{kind: cTokenLt, text: "<"})
		case '>':
			toks = append(toks, cToken{kind: cTokenGt, text: ">"})
		case '(':
			toks = append(toks, cToken{kind: cTokenLParen, text: "("})
		case ')':
			toks = append(toks, cToken{kind: cTokenRParen, text: ")"})
		case '?':
			toks = append(toks, cToken{kind: cTokenQuestion, text: "?"})
		case ':':
			toks = append(toks, cToken{kind: cTokenColon, text: ":"})
		case '!':
			toks = append(toks, cToken{kind: cTokenNot, text: "!"})
		default:
			return nil, fmt.Errorf("unsupported token starting at %q", src[i:])
		}
		i++
	}
	toks = append(toks, cToken{kind: cTokenEOF})
	return toks, nil
}

// parseOr: expr || expr
func (p *charSetExprParser) parseOr() ([]lexRange, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == cTokenOr {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = unionRanges(left, right)
	}
	return left, nil
}

// parseAnd: expr && expr
func (p *charSetExprParser) parseAnd() ([]lexRange, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == cTokenAnd {
		p.next()
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		left = intersectRanges(left, right)
	}
	return left, nil
}

// parsePrimary handles parenthesized expressions, comparisons, and ternaries.
func (p *charSetExprParser) parsePrimary() ([]lexRange, error) {
	if p.peek().kind == cTokenLParen {
		p.next() // consume '('
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}

		// Check for ternary: after inner expression, if we see '?' it's a ternary condition.
		// But ternaries inside parens have structure: (c < PIVOT ? LEFT : RIGHT)
		// The inner parse would have consumed "c < PIVOT" as a comparison.
		// We need to detect if next is '?'.
		if p.peek().kind == cTokenQuestion {
			p.next() // consume '?'
			// inner is the condition ranges (chars for which condition is true)
			// For ternary: cond ? left : right
			// LEFT applies when cond is true, RIGHT when cond is false
			left, err := p.parseOr()
			if err != nil {
				return nil, err
			}
			if p.peek().kind != cTokenColon {
				return nil, fmt.Errorf("expected ':' in ternary")
			}
			p.next() // consume ':'
			right, err := p.parseOr()
			if err != nil {
				return nil, err
			}
			if p.peek().kind != cTokenRParen {
				return nil, fmt.Errorf("expected ')' after ternary")
			}
			p.next() // consume ')'

			// inner = ranges where condition holds (e.g., c < PIVOT → [0, PIVOT-1])
			// condComplement = ranges where condition doesn't hold
			condComplement := complementRanges(inner)
			leftConstrained := intersectRanges(left, inner)
			rightConstrained := intersectRanges(right, condComplement)
			return unionRanges(leftConstrained, rightConstrained), nil
		}

		if p.peek().kind != cTokenRParen {
			return nil, fmt.Errorf("expected ')'")
		}
		p.next() // consume ')'
		return inner, nil
	}

	// Identifier: expect 'c' followed by comparison operator
	if p.peek().kind == cTokenIdent && p.peek().text == "c" {
		return p.parseComparison()
	}

	// Boolean literal (tree-sitter sometimes uses true/false in leaves)
	if p.peek().kind == cTokenIdent {
		tok := p.next()
		if tok.text == "true" {
			return universeRanges(), nil
		}
		if tok.text == "false" {
			return nil, nil
		}
		return nil, fmt.Errorf("unexpected identifier %q", tok.text)
	}

	// Reversed comparison: LITERAL OP c (e.g., '@' <= c)
	if p.peek().kind == cTokenNumber || p.peek().kind == cTokenChar {
		tok := p.next()
		val, ok := parseCLookaheadLiteral(tok.text)
		if !ok {
			return nil, fmt.Errorf("invalid literal %q", tok.text)
		}
		op := p.peek()
		if op.kind == cTokenLe || op.kind == cTokenLt || op.kind == cTokenGe || op.kind == cTokenGt || op.kind == cTokenEq || op.kind == cTokenNe {
			p.next()
			// Expect 'c'
			cTok := p.next()
			if cTok.kind != cTokenIdent || cTok.text != "c" {
				return nil, fmt.Errorf("expected 'c' in reversed comparison")
			}
			// Reverse: val OP c → c REVERSE_OP val
			switch op.kind {
			case cTokenLe: // val <= c → c >= val
				return []lexRange{{lo: val, hi: maxLookaheadRune}}, nil
			case cTokenLt: // val < c → c > val
				if val >= maxLookaheadRune {
					return nil, nil
				}
				return []lexRange{{lo: val + 1, hi: maxLookaheadRune}}, nil
			case cTokenGe: // val >= c → c <= val
				return []lexRange{{lo: 0, hi: val}}, nil
			case cTokenGt: // val > c → c < val
				if val <= 0 {
					return nil, nil
				}
				return []lexRange{{lo: 0, hi: val - 1}}, nil
			case cTokenEq: // val == c → c == val
				return []lexRange{{lo: val, hi: val}}, nil
			case cTokenNe: // val != c → c != val
				return complementRanges([]lexRange{{lo: val, hi: val}}), nil
			}
		}
		return nil, fmt.Errorf("unexpected literal %q without comparison", tok.text)
	}

	return nil, fmt.Errorf("unexpected token %q in charset expr", p.peek().text)
}

// parseComparison parses: c OP value
func (p *charSetExprParser) parseComparison() ([]lexRange, error) {
	p.next() // consume 'c'
	op := p.next()
	val, err := p.parseValue()
	if err != nil {
		return nil, err
	}

	switch op.kind {
	case cTokenLt:
		if val <= 0 {
			return nil, nil
		}
		return []lexRange{{lo: 0, hi: val - 1}}, nil
	case cTokenLe:
		return []lexRange{{lo: 0, hi: val}}, nil
	case cTokenGt:
		if val >= maxLookaheadRune {
			return nil, nil
		}
		return []lexRange{{lo: val + 1, hi: maxLookaheadRune}}, nil
	case cTokenGe:
		return []lexRange{{lo: val, hi: maxLookaheadRune}}, nil
	case cTokenEq:
		return []lexRange{{lo: val, hi: val}}, nil
	case cTokenNe:
		return complementRanges([]lexRange{{lo: val, hi: val}}), nil
	default:
		return nil, fmt.Errorf("unexpected operator %q in comparison", op.text)
	}
}

// parseValue parses a numeric or character literal.
func (p *charSetExprParser) parseValue() (rune, error) {
	tok := p.next()
	switch tok.kind {
	case cTokenNumber, cTokenChar:
		v, ok := parseCLookaheadLiteral(tok.text)
		if !ok {
			return 0, fmt.Errorf("invalid literal %q", tok.text)
		}
		return v, nil
	default:
		return 0, fmt.Errorf("expected value, got %q", tok.text)
	}
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
			st.Transitions = append(st.Transitions, action.mapTransitions...)
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
			// Check if this is a character_set function call: name(lookahead)
			if ranges, ok := p.charSets[tok.text]; ok && p.peek().kind == cTokenLParen {
				p.next() // consume '('
				lookTok := p.next()
				if lookTok.kind != cTokenIdent || lookTok.text != "lookahead" {
					return condOperand{}, fmt.Errorf("character_set function expects lookahead argument")
				}
				if p.peek().kind != cTokenRParen {
					return condOperand{}, fmt.Errorf("character_set function expects ')'")
				}
				p.next() // consume ')'
				return condOperand{
					kind: operandCondition,
					condition: conditionResult{
						nonEOF: ranges,
						eof:    false,
					},
				}, nil
			}
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

// replaceWholeWord replaces all whole-word occurrences of old with new in s.
func replaceWholeWord(s, old, new string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if i+len(old) <= len(s) && s[i:i+len(old)] == old {
			before := i == 0 || !isIdentPart(s[i-1])
			after := i+len(old) >= len(s) || !isIdentPart(s[i+len(old)])
			if before && after {
				b.WriteString(new)
				i += len(old)
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
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
