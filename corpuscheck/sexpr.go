package corpuscheck

import (
	"fmt"
	"strings"
)

// SNode is a canonical, whitespace-insensitive representation of one node
// in an S-expression tree -- either the one a grammar author wrote by hand
// in a corpus fixture, or the one gotreesitter's own tree renders to. Both
// sides are converted to this shape so that comparison never has to worry
// about indentation, line wrapping, or trailing newlines.
type SNode struct {
	// Field is the field name this node is attached under in its parent,
	// or "" if none.
	Field string
	// Type is the node's type name: a grammar rule name for ordinary
	// nodes, "ERROR" for error nodes, or (for a Missing anonymous token)
	// the token's literal text.
	Type string
	// Missing marks a MISSING node (always a leaf).
	Missing bool
	// MissingIsAnon marks that Type is a literal anonymous token's text
	// (was double-quoted in the fixture) rather than a named node type.
	MissingIsAnon bool
	// Unexpected marks an UNEXPECTED node -- the C tree-sitter runtime's
	// marker for a single lexically-unrecognized byte inside an ERROR
	// node. gotreesitter has no equivalent concept (see compare.go); this
	// flag exists purely so the comparator can name that gap explicitly
	// instead of reporting a generic shape mismatch.
	Unexpected bool
	// Children holds this node's named children, in order.
	Children []*SNode
}

// ParseExpected parses the expected-output text from a corpus test case
// (everything after the "---" divider) into a canonical SNode tree.
//
// Supported: node types, field names ("field: (...)"), ERROR nodes, and
// MISSING nodes (both quoted-anonymous-token and bare-named-type forms,
// e.g. `(MISSING ".")` and `(MISSING identifier)`).
//
// NOT supported: UNEXPECTED nodes are recognized syntactically (so a
// fixture that contains one doesn't fail to parse) but are flagged via
// SNode.Unexpected rather than modeled with real content, since
// gotreesitter has no equivalent node kind to compare them against.
func ParseExpected(text string) (*SNode, error) {
	p := &sexprParser{s: text}
	p.skipSpace()
	if p.pos >= len(p.s) {
		return nil, fmt.Errorf("empty expected output")
	}
	node, err := p.parseNode()
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if p.pos != len(p.s) {
		return nil, fmt.Errorf("trailing content at offset %d: %q", p.pos, p.s[p.pos:min(p.pos+40, len(p.s))])
	}
	return node, nil
}

type sexprParser struct {
	s   string
	pos int
}

func (p *sexprParser) skipSpace() {
	for p.pos < len(p.s) {
		switch p.s[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func isIdentByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-'
}

// parseNode parses one "(type ...)" form, optionally preceded by this
// call's caller already having consumed a "field: " prefix.
func (p *sexprParser) parseNode() (*SNode, error) {
	if p.pos >= len(p.s) || p.s[p.pos] != '(' {
		return nil, fmt.Errorf("offset %d: expected '(', found %q", p.pos, p.remainder())
	}
	p.pos++
	p.skipSpace()

	head, err := p.readIdent()
	if err != nil {
		return nil, err
	}

	switch head {
	case "MISSING":
		return p.parseMissing()
	case "UNEXPECTED":
		return p.parseUnexpected()
	default:
		node := &SNode{Type: head}
		for {
			p.skipSpace()
			if p.pos >= len(p.s) {
				return nil, fmt.Errorf("offset %d: unterminated node %q", p.pos, head)
			}
			if p.s[p.pos] == ')' {
				p.pos++
				return node, nil
			}
			child, field, err := p.parseChild()
			if err != nil {
				return nil, err
			}
			child.Field = field
			node.Children = append(node.Children, child)
		}
	}
}

// parseChild parses an optional "field: " prefix followed by a child node.
func (p *sexprParser) parseChild() (*SNode, string, error) {
	start := p.pos
	if isIdentStart(p.s[p.pos]) {
		ident, err := p.readIdent()
		if err == nil {
			p.skipSpace()
			if p.pos < len(p.s) && p.s[p.pos] == ':' && !strings.HasPrefix(p.s[p.pos:], "::") {
				p.pos++
				p.skipSpace()
				child, err := p.parseNode()
				if err != nil {
					return nil, "", err
				}
				return child, ident, nil
			}
		}
		// Not actually a "field:" prefix; rewind and parse as a plain node.
		p.pos = start
	}
	child, err := p.parseNode()
	if err != nil {
		return nil, "", err
	}
	return child, "", nil
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func (p *sexprParser) readIdent() (string, error) {
	start := p.pos
	for p.pos < len(p.s) && isIdentByte(p.s[p.pos]) {
		p.pos++
	}
	if p.pos == start {
		return "", fmt.Errorf("offset %d: expected identifier, found %q", p.pos, p.remainder())
	}
	return p.s[start:p.pos], nil
}

// parseMissing parses the contents of a "(MISSING ...)" node, after the
// "MISSING" head token has already been consumed.
func (p *sexprParser) parseMissing() (*SNode, error) {
	p.skipSpace()
	if p.pos >= len(p.s) {
		return nil, fmt.Errorf("offset %d: unterminated MISSING", p.pos)
	}
	node := &SNode{Missing: true}
	if p.s[p.pos] == '"' {
		// Anonymous token text. The fixture format doesn't escape
		// embedded quote characters (a MISSING closing '"' token renders
		// literally as `(MISSING """)`), so take everything between the
		// opening quote and the LAST quote before this node's closing
		// paren -- MISSING has no nested parens, so that paren is
		// unambiguous.
		closeParen := strings.IndexByte(p.s[p.pos:], ')')
		if closeParen < 0 {
			return nil, fmt.Errorf("offset %d: unterminated MISSING string", p.pos)
		}
		window := p.s[p.pos : p.pos+closeParen]
		lastQuote := strings.LastIndexByte(window, '"')
		if lastQuote <= 0 {
			return nil, fmt.Errorf("offset %d: malformed MISSING string %q", p.pos, window)
		}
		node.Type = window[1:lastQuote]
		node.MissingIsAnon = true
		p.pos += lastQuote + 1
	} else {
		ident, err := p.readIdent()
		if err != nil {
			return nil, fmt.Errorf("offset %d: malformed MISSING value: %w", p.pos, err)
		}
		node.Type = ident
	}
	p.skipSpace()
	if p.pos >= len(p.s) || p.s[p.pos] != ')' {
		return nil, fmt.Errorf("offset %d: expected ')' closing MISSING, found %q", p.pos, p.remainder())
	}
	p.pos++
	return node, nil
}

// parseUnexpected parses the contents of a "(UNEXPECTED '...')" node.
// gotreesitter has no equivalent node kind (see the Unexpected field's
// doc comment); this only exists so a fixture containing one still parses
// instead of aborting the whole test.
func (p *sexprParser) parseUnexpected() (*SNode, error) {
	p.skipSpace()
	if p.pos >= len(p.s) || p.s[p.pos] != '\'' {
		return nil, fmt.Errorf("offset %d: expected quoted byte after UNEXPECTED, found %q", p.pos, p.remainder())
	}
	start := p.pos
	p.pos++
	end := strings.IndexByte(p.s[p.pos:], '\'')
	if end < 0 {
		return nil, fmt.Errorf("offset %d: unterminated UNEXPECTED literal", start)
	}
	p.pos += end + 1
	p.skipSpace()
	if p.pos >= len(p.s) || p.s[p.pos] != ')' {
		return nil, fmt.Errorf("offset %d: expected ')' closing UNEXPECTED, found %q", p.pos, p.remainder())
	}
	p.pos++
	return &SNode{Unexpected: true}, nil
}

func (p *sexprParser) remainder() string {
	end := p.pos + 20
	if end > len(p.s) {
		end = len(p.s)
	}
	return p.s[p.pos:end]
}
