package grammargen

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// regexNode represents a node in the parsed regex AST.
type regexNode struct {
	kind     regexKind
	children []*regexNode
	lo, hi   rune // for charRange
	runes    []runeRange // for charClass
	negate   bool        // for charClass
	value    rune        // for literal
	count    int         // for counted repetition {n} min
	countMax int         // for {n,m}: max (-1 = unbounded, 0 = same as count)
}

type regexKind int

const (
	regexLiteral  regexKind = iota // single character
	regexCharClass                 // [a-z] or [^a-z]
	regexDot                       // . (any character except \n)
	regexSeq                       // concatenation
	regexAlt                       // alternation |
	regexStar                      // * zero-or-more
	regexPlus                      // + one-or-more
	regexQuestion                  // ? optional
	regexCount                     // {n} exactly n times
)

// runeRange is an inclusive character range.
type runeRange struct {
	lo, hi rune
}

// parseRegex parses a tree-sitter-compatible regex pattern into an AST.
func parseRegex(pattern string) (*regexNode, error) {
	p := &regexParser{input: pattern}
	node, err := p.parseAlt()
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.input) {
		return nil, fmt.Errorf("unexpected character at position %d: %q", p.pos, string(p.input[p.pos]))
	}
	return node, nil
}

type regexParser struct {
	input string
	pos   int
}

func (p *regexParser) peek() (rune, bool) {
	if p.pos >= len(p.input) {
		return 0, false
	}
	r, _ := utf8.DecodeRuneInString(p.input[p.pos:])
	return r, true
}

func (p *regexParser) advance() rune {
	r, size := utf8.DecodeRuneInString(p.input[p.pos:])
	p.pos += size
	return r
}

// parseAlt parses alternation: a|b|c
func (p *regexParser) parseAlt() (*regexNode, error) {
	left, err := p.parseSeq()
	if err != nil {
		return nil, err
	}
	r, ok := p.peek()
	if !ok || r != '|' {
		return left, nil
	}
	alts := []*regexNode{left}
	for {
		r, ok = p.peek()
		if !ok || r != '|' {
			break
		}
		p.advance() // consume '|'
		alt, err := p.parseSeq()
		if err != nil {
			return nil, err
		}
		alts = append(alts, alt)
	}
	return &regexNode{kind: regexAlt, children: alts}, nil
}

// parseSeq parses concatenation of atoms.
func (p *regexParser) parseSeq() (*regexNode, error) {
	var items []*regexNode
	for {
		r, ok := p.peek()
		if !ok || r == '|' || r == ')' {
			break
		}
		atom, err := p.parseQuantified()
		if err != nil {
			return nil, err
		}
		items = append(items, atom)
	}
	if len(items) == 0 {
		return &regexNode{kind: regexSeq}, nil // empty sequence
	}
	if len(items) == 1 {
		return items[0], nil
	}
	return &regexNode{kind: regexSeq, children: items}, nil
}

// parseQuantified parses an atom with optional quantifier: a*, a+, a?, a{n}
func (p *regexParser) parseQuantified() (*regexNode, error) {
	atom, err := p.parseAtom()
	if err != nil {
		return nil, err
	}
	r, ok := p.peek()
	if !ok {
		return atom, nil
	}
	switch r {
	case '*':
		p.advance()
		return &regexNode{kind: regexStar, children: []*regexNode{atom}}, nil
	case '+':
		p.advance()
		return &regexNode{kind: regexPlus, children: []*regexNode{atom}}, nil
	case '?':
		p.advance()
		return &regexNode{kind: regexQuestion, children: []*regexNode{atom}}, nil
	case '{':
		saved := p.pos
		min, max, err := p.parseCount()
		if err != nil {
			// Not a valid count (e.g. u{hex} in regex) — backtrack so { is parsed as literal
			p.pos = saved
			return atom, nil
		}
		return &regexNode{kind: regexCount, children: []*regexNode{atom}, count: min, countMax: max}, nil
	}
	return atom, nil
}

// parseCount parses counted repetition: {n}, {n,m}, {n,}.
// Returns (min, max) where max=-1 means unbounded.
func (p *regexParser) parseCount() (int, int, error) {
	p.advance() // consume '{'
	start := p.pos
	for {
		r, ok := p.peek()
		if !ok {
			return 0, 0, fmt.Errorf("unterminated {}")
		}
		if r == '}' {
			break
		}
		p.advance()
	}
	content := p.input[start:p.pos]
	p.advance() // consume '}'

	if idx := strings.Index(content, ","); idx >= 0 {
		minStr := content[:idx]
		maxStr := content[idx+1:]
		min, err := strconv.Atoi(strings.TrimSpace(minStr))
		if err != nil {
			return 0, 0, fmt.Errorf("invalid min in {%s}: %w", content, err)
		}
		if strings.TrimSpace(maxStr) == "" {
			return min, -1, nil // {n,} — unbounded
		}
		max, err := strconv.Atoi(strings.TrimSpace(maxStr))
		if err != nil {
			return 0, 0, fmt.Errorf("invalid max in {%s}: %w", content, err)
		}
		return min, max, nil
	}

	n, err := strconv.Atoi(content)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid count in {%s}: %w", content, err)
	}
	return n, n, nil // {n} — exactly n
}

// parseAtom parses a single atom: literal, charclass, group, dot, escape.
func (p *regexParser) parseAtom() (*regexNode, error) {
	r, ok := p.peek()
	if !ok {
		return nil, fmt.Errorf("unexpected end of pattern")
	}
	switch r {
	case '(':
		p.advance() // consume '('
		// Check for non-capturing group (?:...)
		if r2, ok := p.peek(); ok && r2 == '?' {
			p.advance() // consume '?'
			if r3, ok := p.peek(); ok && r3 == ':' {
				p.advance() // consume ':'
			}
		}
		inner, err := p.parseAlt()
		if err != nil {
			return nil, err
		}
		r, ok = p.peek()
		if !ok || r != ')' {
			return nil, fmt.Errorf("expected ')' at position %d", p.pos)
		}
		p.advance() // consume ')'
		return inner, nil
	case '[':
		return p.parseCharClass()
	case '.':
		p.advance()
		return &regexNode{kind: regexDot}, nil
	case '\\':
		return p.parseEscape()
	default:
		p.advance()
		return &regexNode{kind: regexLiteral, value: r}, nil
	}
}

// parseCharClass parses [a-z], [^a-z], etc.
func (p *regexParser) parseCharClass() (*regexNode, error) {
	p.advance() // consume '['
	negate := false
	if r, ok := p.peek(); ok && r == '^' {
		negate = true
		p.advance()
	}
	var ranges []runeRange
	first := true
	for {
		r, ok := p.peek()
		if !ok {
			return nil, fmt.Errorf("unterminated character class")
		}
		if r == ']' && !first {
			p.advance()
			break
		}
		first = false

		// Special case: shorthand classes and \p{...}/\P{...} inside character class —
		// these expand to multiple ranges and can't be handled by parseCharClassChar.
		if r == '\\' && p.pos+1 < len(p.input) {
			next := rune(p.input[p.pos+1])
			switch next {
			case 'p', 'P':
				p.advance() // consume '\\'
				p.advance() // consume 'p'/'P'
				propRanges, err := p.parseUnicodeProperty()
				if err != nil {
					return nil, err
				}
				ranges = append(ranges, propRanges...)
				continue
			case 's': // \s → whitespace chars
				p.advance() // consume '\\'
				p.advance() // consume 's'
				ranges = append(ranges, runeRange{' ', ' '}, runeRange{'\t', '\t'}, runeRange{'\n', '\n'}, runeRange{'\r', '\r'}, runeRange{'\f', '\f'}, runeRange{'\v', '\v'})
				continue
			case 'd': // \d → digits
				p.advance()
				p.advance()
				ranges = append(ranges, runeRange{'0', '9'})
				continue
			case 'w': // \w → word chars
				p.advance()
				p.advance()
				ranges = append(ranges, runeRange{'a', 'z'}, runeRange{'A', 'Z'}, runeRange{'0', '9'}, runeRange{'_', '_'})
				continue
			}
		}

		ch, err := p.parseCharClassChar()
		if err != nil {
			return nil, err
		}
		// Check for range: a-z
		if r2, ok := p.peek(); ok && r2 == '-' {
			saved := p.pos
			p.advance() // consume '-'
			if r3, ok := p.peek(); ok && r3 != ']' {
				hi, err := p.parseCharClassChar()
				if err != nil {
					return nil, err
				}
				ranges = append(ranges, runeRange{ch, hi})
				continue
			}
			p.pos = saved // backtrack, '-' is literal at end
		}
		ranges = append(ranges, runeRange{ch, ch})
	}
	return &regexNode{kind: regexCharClass, runes: ranges, negate: negate}, nil
}

// parseCharClassChar parses a single character inside a character class.
func (p *regexParser) parseCharClassChar() (rune, error) {
	r, ok := p.peek()
	if !ok {
		return 0, fmt.Errorf("unexpected end in character class")
	}
	if r == '\\' {
		p.advance()
		return p.parseEscapeChar()
	}
	p.advance()
	return r, nil
}

// parseEscape parses an escape sequence.
func (p *regexParser) parseEscape() (*regexNode, error) {
	p.advance() // consume '\\'
	// Check for shorthand character classes before consuming.
	r, ok := p.peek()
	if !ok {
		return nil, fmt.Errorf("unexpected end after \\")
	}
	switch r {
	case 'd': // \d → [0-9]
		p.advance()
		return &regexNode{kind: regexCharClass, runes: []runeRange{{'0', '9'}}}, nil
	case 'D': // \D → [^0-9]
		p.advance()
		return &regexNode{kind: regexCharClass, runes: []runeRange{{'0', '9'}}, negate: true}, nil
	case 'w': // \w → [a-zA-Z0-9_]
		p.advance()
		return &regexNode{kind: regexCharClass, runes: []runeRange{
			{'a', 'z'}, {'A', 'Z'}, {'0', '9'}, {'_', '_'},
		}}, nil
	case 'W': // \W → [^a-zA-Z0-9_]
		p.advance()
		return &regexNode{kind: regexCharClass, runes: []runeRange{
			{'a', 'z'}, {'A', 'Z'}, {'0', '9'}, {'_', '_'},
		}, negate: true}, nil
	case 's': // \s → [\t\n\r \f\v]
		p.advance()
		return &regexNode{kind: regexCharClass, runes: []runeRange{
			{' ', ' '}, {'\t', '\t'}, {'\n', '\n'}, {'\r', '\r'}, {'\f', '\f'}, {'\v', '\v'},
		}}, nil
	case 'S': // \S → [^\t\n\r \f\v]
		p.advance()
		return &regexNode{kind: regexCharClass, runes: []runeRange{
			{' ', ' '}, {'\t', '\t'}, {'\n', '\n'}, {'\r', '\r'}, {'\f', '\f'}, {'\v', '\v'},
		}, negate: true}, nil
	case 'p': // \p{PropertyName} — Unicode property (positive)
		p.advance()
		ranges, err := p.parseUnicodeProperty()
		if err != nil {
			return nil, err
		}
		return &regexNode{kind: regexCharClass, runes: ranges}, nil
	case 'P': // \P{PropertyName} — Unicode property (negated)
		p.advance()
		ranges, err := p.parseUnicodeProperty()
		if err != nil {
			return nil, err
		}
		return &regexNode{kind: regexCharClass, runes: ranges, negate: true}, nil
	}
	ch, err := p.parseEscapeChar()
	if err != nil {
		return nil, err
	}
	return &regexNode{kind: regexLiteral, value: ch}, nil
}

// parseEscapeChar returns the escaped character value.
func (p *regexParser) parseEscapeChar() (rune, error) {
	r, ok := p.peek()
	if !ok {
		return 0, fmt.Errorf("unexpected end after \\")
	}
	p.advance()
	switch r {
	case 'n':
		return '\n', nil
	case 'r':
		return '\r', nil
	case 't':
		return '\t', nil
	case 'a':
		return '\a', nil
	case 'b':
		return '\b', nil
	case 'f':
		return '\f', nil
	case 'v':
		return '\v', nil
	case '0':
		return 0, nil
	case '\\':
		return '\\', nil
	case '"':
		return '"', nil
	case '/':
		return '/', nil
	case '[', ']', '(', ')', '{', '}', '.', '*', '+', '?', '|', '^', '$', '-':
		return r, nil
	case 'u':
		return p.parseUnicodeEscape()
	case 'x':
		return p.parseHexEscape()
	default:
		return r, nil
	}
}

// parseUnicodeProperty parses \p{PropertyName}, \P{PropertyName}, or single-letter \pL.
// Expects the 'p'/'P' to have already been consumed.
func (p *regexParser) parseUnicodeProperty() ([]runeRange, error) {
	r, ok := p.peek()
	if !ok {
		return nil, fmt.Errorf("expected property name after \\p")
	}
	// Single-letter shorthand: \pL, \pN, etc.
	if r != '{' {
		if unicode.IsLetter(r) {
			p.advance()
			return unicodePropertyRanges(string(r))
		}
		return nil, fmt.Errorf("expected '{' or letter after \\p, got %q", r)
	}
	p.advance() // consume '{'

	start := p.pos
	for {
		r, ok := p.peek()
		if !ok {
			return nil, fmt.Errorf("unterminated \\p{...}")
		}
		if r == '}' {
			break
		}
		p.advance()
	}
	propName := p.input[start:p.pos]
	p.advance() // consume '}'

	return unicodePropertyRanges(propName)
}

// unicodePropertyRanges returns rune ranges for a Unicode property name.
// Uses Go's unicode package for accurate ranges.
func unicodePropertyRanges(name string) ([]runeRange, error) {
	var table *unicode.RangeTable
	switch name {
	case "L", "Letter":
		table = unicode.Letter
	case "Lu", "Uppercase_Letter":
		table = unicode.Lu
	case "Ll", "Lowercase_Letter":
		table = unicode.Ll
	case "Lt", "Titlecase_Letter":
		table = unicode.Lt
	case "Lm", "Modifier_Letter":
		table = unicode.Lm
	case "Lo", "Other_Letter":
		table = unicode.Lo
	case "N", "Number":
		table = unicode.Number
	case "Nd", "Decimal_Number", "Decimal_Digit":
		table = unicode.Nd
	case "Nl", "Letter_Number":
		table = unicode.Nl
	case "No", "Other_Number":
		table = unicode.No
	case "P", "Punctuation":
		table = unicode.Punct
	case "S", "Symbol":
		table = unicode.Symbol
	case "Z", "Separator":
		table = unicode.Z
	case "Zs", "Space_Separator":
		table = unicode.Zs
	case "Zl", "Line_Separator":
		table = unicode.Zl
	case "Zp", "Paragraph_Separator":
		table = unicode.Zp
	case "M", "Mark":
		table = unicode.Mark
	case "Mn", "Nonspacing_Mark":
		table = unicode.Mn
	case "Mc", "Spacing_Mark":
		table = unicode.Mc
	case "Sm", "Math_Symbol":
		table = unicode.Sm
	case "So", "Other_Symbol":
		table = unicode.So
	case "Sk", "Modifier_Symbol":
		table = unicode.Sk
	case "Sc", "Currency_Symbol":
		table = unicode.Sc
	case "Pc", "Connector_Punctuation":
		table = unicode.Pc
	case "Cc", "Control":
		table = unicode.Cc
	case "Cf", "Format":
		table = unicode.Cf
	case "XID_Start", "ID_Start":
		// Approximation: Letter ∪ Nl (covers the vast majority of XID_Start/ID_Start)
		ranges := rangeTableToRuneRanges(unicode.Letter)
		ranges = append(ranges, rangeTableToRuneRanges(unicode.Nl)...)
		return ranges, nil
	case "XID_Continue", "ID_Continue":
		// Approximation: Letter ∪ Nl ∪ Mn ∪ Mc ∪ Nd ∪ Pc
		ranges := rangeTableToRuneRanges(unicode.Letter)
		ranges = append(ranges, rangeTableToRuneRanges(unicode.Nl)...)
		ranges = append(ranges, rangeTableToRuneRanges(unicode.Mn)...)
		ranges = append(ranges, rangeTableToRuneRanges(unicode.Mc)...)
		ranges = append(ranges, rangeTableToRuneRanges(unicode.Nd)...)
		ranges = append(ranges, rangeTableToRuneRanges(unicode.Pc)...)
		return ranges, nil
	default:
		// Check Go's unicode property/category/script tables
		if t, ok := unicode.Properties[name]; ok {
			return rangeTableToRuneRanges(t), nil
		}
		if t, ok := unicode.Categories[name]; ok {
			return rangeTableToRuneRanges(t), nil
		}
		if t, ok := unicode.Scripts[name]; ok {
			return rangeTableToRuneRanges(t), nil
		}
		// Manual approximations for properties not in Go's unicode package
		switch name {
		case "Emoji":
			return emojiRanges(), nil
		}
		return nil, fmt.Errorf("unsupported Unicode property %q", name)
	}

	return rangeTableToRuneRanges(table), nil
}

// emojiRanges returns an approximate set of Unicode Emoji code point ranges.
func emojiRanges() []runeRange {
	return []runeRange{
		{0x23, 0x23},       // #
		{0x2A, 0x2A},       // *
		{0x30, 0x39},       // 0-9
		{0xA9, 0xA9},       // ©
		{0xAE, 0xAE},       // ®
		{0x200D, 0x200D},   // ZWJ
		{0x203C, 0x203C},   // ‼
		{0x2049, 0x2049},   // ⁉
		{0x20E3, 0x20E3},   // combining enclosing keycap
		{0x2122, 0x2122},   // ™
		{0x2139, 0x2139},   // ℹ
		{0x2194, 0x2199},   // ↔-↙
		{0x21A9, 0x21AA},   // ↩↪
		{0x231A, 0x231B},   // ⌚⌛
		{0x2328, 0x2328},   // ⌨
		{0x23CF, 0x23CF},   // ⏏
		{0x23E9, 0x23F3},   // ⏩-⏳
		{0x23F8, 0x23FA},   // ⏸-⏺
		{0x24C2, 0x24C2},   // Ⓜ
		{0x25AA, 0x25AB},   // ▪▫
		{0x25B6, 0x25B6},   // ▶
		{0x25C0, 0x25C0},   // ◀
		{0x25FB, 0x25FE},   // ◻-◾
		{0x2600, 0x27BF},   // misc symbols + dingbats
		{0x2934, 0x2935},   // ⤴⤵
		{0x2B05, 0x2B07},   // ⬅-⬇
		{0x2B1B, 0x2B1C},   // ⬛⬜
		{0x2B50, 0x2B50},   // ⭐
		{0x2B55, 0x2B55},   // ⭕
		{0x3030, 0x3030},   // 〰
		{0x303D, 0x303D},   // 〽
		{0x3297, 0x3297},   // ㊗
		{0x3299, 0x3299},   // ㊙
		{0xFE0F, 0xFE0F},   // variation selector-16
		{0x1F000, 0x1FAFF}, // all major emoji blocks
	}
}

// rangeTableToRuneRanges converts a unicode.RangeTable to runeRange slices.
func rangeTableToRuneRanges(table *unicode.RangeTable) []runeRange {
	var ranges []runeRange
	for _, r16 := range table.R16 {
		lo := rune(r16.Lo)
		hi := rune(r16.Hi)
		stride := rune(r16.Stride)
		if stride == 1 {
			ranges = append(ranges, runeRange{lo, hi})
		} else {
			for c := lo; c <= hi; c += stride {
				ranges = append(ranges, runeRange{c, c})
			}
		}
	}
	for _, r32 := range table.R32 {
		lo := rune(r32.Lo)
		hi := rune(r32.Hi)
		stride := rune(r32.Stride)
		if stride == 1 {
			ranges = append(ranges, runeRange{lo, hi})
		} else {
			for c := lo; c <= hi; c += stride {
				ranges = append(ranges, runeRange{c, c})
			}
		}
	}
	return ranges
}

// parseHexEscape parses \xNN (exactly 2 hex digits).
func (p *regexParser) parseHexEscape() (rune, error) {
	if p.pos+2 > len(p.input) {
		return 0, fmt.Errorf("incomplete \\x escape")
	}
	hex := p.input[p.pos : p.pos+2]
	p.pos += 2
	n, err := strconv.ParseUint(hex, 16, 8)
	if err != nil {
		return 0, fmt.Errorf("invalid \\x escape: %s", hex)
	}
	return rune(n), nil
}

// parseUnicodeEscape parses \uXXXX or \u{XXXX} (variable-length braced form).
func (p *regexParser) parseUnicodeEscape() (rune, error) {
	// Check for braced form: \u{XXXX}
	if p.pos < len(p.input) && p.input[p.pos] == '{' {
		p.pos++ // consume '{'
		end := strings.IndexByte(p.input[p.pos:], '}')
		if end < 0 {
			return 0, fmt.Errorf("unterminated \\u{...} escape")
		}
		hex := p.input[p.pos : p.pos+end]
		p.pos += end + 1 // consume hex digits and '}'
		n, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return 0, fmt.Errorf("invalid \\u escape: {%s}", hex)
		}
		return rune(n), nil
	}
	// Standard form: \uXXXX (exactly 4 hex digits)
	if p.pos+4 > len(p.input) {
		return 0, fmt.Errorf("incomplete \\u escape")
	}
	hex := p.input[p.pos : p.pos+4]
	p.pos += 4
	n, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid \\u escape: %s", hex)
	}
	return rune(n), nil
}

// expandRegexToRule converts a parsed regex AST into a grammar Rule tree
// suitable for NFA construction.
func expandRegexToRule(node *regexNode) *Rule {
	switch node.kind {
	case regexLiteral:
		return &Rule{Kind: RuleString, Value: string(node.value)}
	case regexCharClass:
		return regexCharClassToRule(node)
	case regexDot:
		// Match any character except \n
		return regexCharClassToRule(&regexNode{
			kind:   regexCharClass,
			runes:  []runeRange{{'\n', '\n'}},
			negate: true,
		})
	case regexSeq:
		if len(node.children) == 0 {
			return Blank()
		}
		if len(node.children) == 1 {
			return expandRegexToRule(node.children[0])
		}
		children := make([]*Rule, len(node.children))
		for i, c := range node.children {
			children[i] = expandRegexToRule(c)
		}
		return Seq(children...)
	case regexAlt:
		children := make([]*Rule, len(node.children))
		for i, c := range node.children {
			children[i] = expandRegexToRule(c)
		}
		return Choice(children...)
	case regexStar:
		return Repeat(expandRegexToRule(node.children[0]))
	case regexPlus:
		return Repeat1(expandRegexToRule(node.children[0]))
	case regexQuestion:
		return Optional(expandRegexToRule(node.children[0]))
	case regexCount:
		inner := expandRegexToRule(node.children[0])
		min := node.count
		max := node.countMax
		if min <= 0 && max <= 0 {
			return Blank()
		}
		if max == -1 {
			// {n,} — n required + zero-or-more
			parts := make([]*Rule, min+1)
			for i := 0; i < min; i++ {
				parts[i] = cloneRule(inner)
			}
			parts[min] = Repeat(cloneRule(inner))
			return Seq(parts...)
		}
		// {n,m} or {n} — n required + (m-n) optional
		parts := make([]*Rule, max)
		for i := 0; i < min; i++ {
			parts[i] = cloneRule(inner)
		}
		for i := min; i < max; i++ {
			parts[i] = Optional(cloneRule(inner))
		}
		return Seq(parts...)
	default:
		return Blank()
	}
}

func regexCharClassToRule(node *regexNode) *Rule {
	// Encode as a special Pattern rule with the char class info
	r := &Rule{Kind: RulePattern}
	var buf strings.Builder
	buf.WriteByte('[')
	if node.negate {
		buf.WriteByte('^')
	}
	for _, rr := range node.runes {
		writeRuneForCharClass(&buf, rr.lo)
		if rr.hi != rr.lo {
			buf.WriteByte('-')
			writeRuneForCharClass(&buf, rr.hi)
		}
	}
	buf.WriteByte(']')
	r.Value = buf.String()
	return r
}

func writeRuneForCharClass(buf *strings.Builder, r rune) {
	switch r {
	case '\\', ']', '-', '^':
		buf.WriteByte('\\')
		buf.WriteRune(r)
	default:
		buf.WriteRune(r)
	}
}

// cloneRule creates a deep copy of a Rule.
func cloneRule(r *Rule) *Rule {
	if r == nil {
		return nil
	}
	cp := *r
	if len(r.Children) > 0 {
		cp.Children = make([]*Rule, len(r.Children))
		for i, c := range r.Children {
			cp.Children[i] = cloneRule(c)
		}
	}
	return &cp
}

// expandPatternRule parses a regex pattern string and returns a Rule tree.
func expandPatternRule(pattern string) (*Rule, error) {
	node, err := parseRegex(pattern)
	if err != nil {
		return nil, fmt.Errorf("regex parse %q: %w", pattern, err)
	}
	return expandRegexToRule(node), nil
}
