package syntaxfacts

import "github.com/odvcencio/gotreesitter/internal/declarationfacts"

// CallArgumentFact projects a direct call argument. Its call range joins CallRef.
// Spread keeps the wrapper's real NodeType and full range, including the ellipsis;
// Kind classifies the wrapped operand. Zero-argument calls produce no entries.
type CallArgumentFact struct {
	CallStartByte, CallEndByte uint32
	Index                      int
	Kind, NodeType             string
	StartByte, EndByte         uint32
	Spread                     bool
}

// ExpressionKindRule maps a grammar node type to a normalized expression kind.
// Unmapped named nodes are "other".
type ExpressionKindRule struct{ NodeType, Kind string }

// CallArgumentRule selects direct named children of a call's argument list.
// Comments are excluded. SpreadNodeType wraps one final operand.
type CallArgumentRule struct {
	NodeType, ArgumentsField, ListNodeType, SpreadNodeType string
	Kinds                                                  []ExpressionKindRule
}

type compiledArguments struct {
	field, list, spread uint16
	kinds               map[uint16]string
}

// Arguments holds immutable call-argument instructions.
type Arguments[N comparable] struct {
	reader declarationfacts.Reader[N]
	rules  map[uint16]compiledArguments
}

func compileArguments(g declarationfacts.Grammar, rule CallArgumentRule) (compiledArguments, bool) {
	var c compiledArguments
	var ok bool
	if _, ok = g.Symbol(rule.NodeType); !ok {
		return c, false
	}
	if c.field, ok = g.Field(rule.ArgumentsField); !ok {
		return c, false
	}
	if c.list, ok = g.Symbol(rule.ListNodeType); !ok {
		return c, false
	}
	if rule.SpreadNodeType != "" {
		if c.spread, ok = g.Symbol(rule.SpreadNodeType); !ok {
			return c, false
		}
	}
	c.kinds = make(map[uint16]string)
	for _, row := range rule.Kinds {
		if row.Kind == "" {
			return c, false
		}
		if _, ok = g.Symbol(row.NodeType); !ok {
			return c, false
		}
		for _, sym := range symbols(g, row.NodeType) {
			c.kinds[sym] = row.Kind
		}
	}
	return c, true
}

// ValidCallArgumentRule checks all referenced grammar data.
func ValidCallArgumentRule(g declarationfacts.Grammar, rule CallArgumentRule) bool {
	_, ok := compileArguments(g, rule)
	return ok
}

// CompileArguments owns rows. No valid rows returns nil.
func CompileArguments[N comparable](g declarationfacts.Grammar, r declarationfacts.Reader[N], rules []CallArgumentRule) *Arguments[N] {
	var p *Arguments[N]
	for _, rule := range rules {
		c, ok := compileArguments(g, rule)
		if !ok {
			continue
		}
		if p == nil {
			p = &Arguments[N]{r, make(map[uint16]compiledArguments)}
		}
		for _, sym := range symbols(g, rule.NodeType) {
			p.rules[sym] = c
		}
	}
	return p
}

// Extract appends arguments only when accept recognizes the same call as CallRef.
// It visits nested calls independently; each call's arguments are in source order.
func (p *Arguments[N]) Extract(root N, source []byte, accept func(N, []byte) bool, dst *[]CallArgumentFact) {
	var zero N
	if p == nil || root == zero || dst == nil {
		return
	}
	p.walk(root, source, accept, dst)
}

func (p *Arguments[N]) walk(n N, source []byte, accept func(N, []byte) bool, dst *[]CallArgumentFact) {
	var zero N
	if n == zero {
		return
	}
	r := p.reader
	if rule, ok := p.rules[r.Symbol(n)]; ok && accept(n, source) {
		list := declarationfacts.ChildByField(r, n, rule.field)
		if list != zero && r.Symbol(list) == rule.list {
			index := 0
			for i := 0; i < r.ChildCount(list); i++ {
				arg := r.Child(list, i)
				if arg == zero || !r.Named(arg) || r.NodeType(arg) == "comment" || r.Missing(arg) {
					continue
				}
				operand := arg
				spread := r.Symbol(arg) == rule.spread
				if spread {
					operand = declarationfacts.SingleChild(r, arg)
				}
				kind := "other"
				if operand != zero {
					if mapped := rule.kinds[r.Symbol(operand)]; mapped != "" {
						kind = mapped
					}
				}
				*dst = append(*dst, CallArgumentFact{CallStartByte: r.StartByte(n), CallEndByte: r.EndByte(n), Index: index, Kind: kind, NodeType: r.NodeType(arg), StartByte: r.StartByte(arg), EndByte: r.EndByte(arg), Spread: spread})
				index++
			}
		}
	}
	for i := 0; i < r.ChildCount(n); i++ {
		p.walk(r.Child(n, i), source, accept, dst)
	}
}
