package lexexact

import (
	"fmt"
	"strconv"
	"strings"
)

// tokKind is the kind of one C token that the extractor understands.
type tokKind uint8

const (
	tokEOF tokKind = iota
	tokIdent
	tokNumber // integer literal, value in token.num
	tokChar   // character literal, value in token.num
	tokString // string literal; the body is opaque, text holds the raw literal
	tokDir    // preprocessor directive; text holds the line with white space collapsed
	tokPunct  // operator or separator, text in token.text
)

type token struct {
	kind tokKind
	text string
	num  int64
	line int
	off  int // byte offset of the token in the source
	// bad is set for a number or character constant the extractor cannot value;
	// reading such a token with the scanner is an error, skipping it in a table body is not.
	bad string
}

func (t token) String() string {
	switch t.kind {
	case tokEOF:
		return "end of text"
	case tokNumber, tokChar:
		return fmt.Sprintf("%s (line %d)", t.text, t.line)
	}
	return fmt.Sprintf("%q (line %d)", t.text, t.line)
}

// maxNesting is the deepest nesting of brackets, braces and parentheses (and of
// expressions) that the reader follows; deeper text is refused. The deepest
// nesting in generated lexers is 17.
const maxNesting = 256

// precheck refuses the byte sequences that different C front ends read
// differently or that join lines: a NUL byte, a carriage return that is not
// followed by a line feed, a trigraph, and a backslash followed by a line end.
func precheck(src string) error {
	for i := 0; i < len(src); i++ {
		switch c := src[i]; c {
		case 0:
			return fmt.Errorf("parser.c: NUL byte at offset %d", i)
		case '\r':
			if i+1 >= len(src) || src[i+1] != '\n' {
				return fmt.Errorf("parser.c: carriage return without line feed at offset %d", i)
			}
		case '?':
			if i+2 < len(src) && src[i+1] == '?' && strings.IndexByte("=/'()!<>-", src[i+2]) >= 0 {
				return fmt.Errorf("parser.c: trigraph at offset %d", i)
			}
		case '\\':
			j := i + 1
			for j < len(src) && (src[j] == ' ' || src[j] == '\t' || src[j] == '\v' || src[j] == '\f') {
				j++
			}
			if j < len(src) && (src[j] == '\n' || src[j] == '\r') {
				return fmt.Errorf("parser.c: backslash-newline at offset %d", i)
			}
		}
	}
	return nil
}

func isIdentStart(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func isIdentPart(b byte) bool { return isIdentStart(b) || isDigit(b) }

var punct2 = []string{"==", "!=", "<=", ">=", "&&", "||", "->"}

// tokenize splits the whole of parser.c into tokens after precheck. Comments
// are dropped, string and character literal bodies are opaque, and a line that
// starts with '#' (after white space and comments) becomes one directive token.
// A directive that holds a comment, and any byte or digraph that the extractor
// does not know, is an error with its offset.
func tokenize(src string) ([]token, error) {
	if err := precheck(src); err != nil {
		return nil, err
	}
	var toks []token
	line := 1
	lineStart := true
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == '\n':
			line++
			lineStart = true
			i++
		case c == ' ' || c == '\t' || c == '\r' || c == '\f' || c == '\v':
			i++
		case c == '/' && strings.HasPrefix(src[i:], "//"):
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '/' && strings.HasPrefix(src[i:], "/*"):
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				return nil, fmt.Errorf("parser.c: unterminated comment at offset %d", i)
			}
			line += strings.Count(src[i:i+2+end], "\n")
			i += end + 4
		case c == '#' && lineStart:
			end := strings.IndexByte(src[i:], '\n')
			l := src[i:]
			if end >= 0 {
				l = src[i : i+end]
			}
			if strings.Contains(l, "/*") || strings.Contains(l, "//") {
				return nil, fmt.Errorf("parser.c: directive at offset %d carries a comment", i)
			}
			toks = append(toks, token{kind: tokDir, text: strings.Join(strings.Fields(l), " "), line: line, off: i})
			i += len(l)
		case isIdentStart(c):
			start := i
			for i < len(src) && isIdentPart(src[i]) {
				i++
			}
			if w := src[start:i]; w == "_Pragma" || w == "__pragma" {
				return nil, fmt.Errorf("parser.c: pragma operator %s at offset %d", w, start)
			}
			toks = append(toks, token{kind: tokIdent, text: src[start:i], line: line, off: start})
			lineStart = false
		case isDigit(c):
			start := i
			for i < len(src) && isIdentPart(src[i]) {
				i++
			}
			text := src[start:i]
			v, err := parseCInt(text)
			tk := token{kind: tokNumber, text: text, num: v, line: line, off: start}
			if err != nil {
				tk.bad = fmt.Sprintf("parser.c: offset %d, line %d: %v", start, line, err)
			}
			toks = append(toks, tk)
			lineStart = false
		case c == '"' || c == '\'':
			start := i
			i++
			for i < len(src) && src[i] != c {
				if src[i] == '\n' {
					return nil, fmt.Errorf("parser.c: literal at offset %d does not end on its line", start)
				}
				if src[i] == '\\' {
					i++
				}
				i++
			}
			if i >= len(src) {
				return nil, fmt.Errorf("parser.c: unterminated literal at offset %d", start)
			}
			i++
			text := src[start:i]
			if c == '"' {
				toks = append(toks, token{kind: tokString, text: text, line: line, off: start})
			} else {
				v, err := parseCChar(text[1 : len(text)-1])
				tk := token{kind: tokChar, text: text, num: v, line: line, off: start}
				if err != nil {
					tk.bad = fmt.Sprintf("parser.c: offset %d, line %d: character literal %s: %v", start, line, text, err)
				}
				toks = append(toks, tk)
			}
			lineStart = false
		default:
			if i+1 < len(src) {
				two := src[i : i+2]
				switch two {
				case "<:", ":>", "<%", "%>", "%:":
					return nil, fmt.Errorf("parser.c: digraph %s at offset %d", two, i)
				}
				found := false
				for _, p := range punct2 {
					if two == p {
						toks = append(toks, token{kind: tokPunct, text: p, line: line, off: i})
						i += 2
						found = true
						break
					}
				}
				if found {
					lineStart = false
					continue
				}
			}
			if strings.IndexByte("(){}[],;:?!<>=*&.#+-/%|^~", c) < 0 {
				return nil, fmt.Errorf("parser.c: unexpected byte %q at offset %d", c, i)
			}
			toks = append(toks, token{kind: tokPunct, text: string(c), line: line, off: i})
			i++
			lineStart = false
		}
	}
	return toks, nil
}

// scanner walks a token slice.
type scanner struct {
	toks []token
	i    int
}

func newScanner(toks []token, i int) *scanner { return &scanner{toks: toks, i: i} }

// next returns the next token, or a tokEOF token at the end.
func (s *scanner) next() (token, error) {
	if s.i >= len(s.toks) {
		return token{kind: tokEOF}, nil
	}
	t := s.toks[s.i]
	s.i++
	if t.kind == tokDir {
		return token{}, fmt.Errorf("parser.c: directive %q at offset %d inside a declaration", t.text, t.off)
	}
	if t.bad != "" {
		return token{}, fmt.Errorf("%s", t.bad)
	}
	return t, nil
}

// parseCInt reads a C integer literal: decimal, 0x hexadecimal or leading-0
// octal, as the C compiler does ("00" is 0, "010" is 8). A suffix is an error.
// A value above 2^31-1 is an error: C gives it an unsigned or wider type, and
// the extractor compares signed 32-bit values only (lookahead -1 would equal
// 0xffffffff in C).
func parseCInt(text string) (int64, error) {
	var v uint64
	var err error
	switch {
	case strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X"):
		v, err = strconv.ParseUint(text[2:], 16, 31)
	case len(text) > 1 && text[0] == '0':
		v, err = strconv.ParseUint(text[1:], 8, 31)
	default:
		v, err = strconv.ParseUint(text, 10, 31)
	}
	if err != nil {
		return 0, fmt.Errorf("integer literal %q is not a plain non-negative 31-bit number", text)
	}
	return int64(v), nil
}

var simpleEscapes = map[byte]int64{
	'n': 10, 't': 9, 'r': 13, '\\': 92, '\'': 39, '"': 34,
	'a': 7, 'b': 8, 'f': 12, 'v': 11, '?': 63,
}

// parseCChar reads the inside of a one-character literal.
func parseCChar(body string) (int64, error) {
	switch {
	case body == "":
		return 0, fmt.Errorf("empty")
	case body[0] != '\\':
		if len(body) != 1 || body[0] >= 0x80 {
			return 0, fmt.Errorf("not a single ASCII character")
		}
		return int64(body[0]), nil
	case len(body) == 2:
		if v, ok := simpleEscapes[body[1]]; ok {
			return v, nil
		}
		if body[1] == '0' {
			return 0, nil
		}
	case body[1] == 'x':
		v, err := strconv.ParseUint(body[2:], 16, 8)
		if err == nil && v < 0x80 { // above 0x7f a char is negative in C on most targets
			return int64(v), nil
		}
	default:
		if len(body)-1 > 3 {
			return 0, fmt.Errorf("octal escape longer than three digits")
		}
		v, err := strconv.ParseUint(body[1:], 8, 8)
		if err == nil && v < 0x80 {
			return int64(v), nil
		}
	}
	return 0, fmt.Errorf("unknown escape")
}
