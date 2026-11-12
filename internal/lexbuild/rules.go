package lexbuild

import "fmt"

// SymbolType is rules.rs: enum SymbolType. The numeric values give the
// derived order of rules.rs: struct Symbol (kind first, then index).
type SymbolType uint8

const (
	External              SymbolType = 0
	End                   SymbolType = 1
	EndOfNonTerminalExtra SymbolType = 2
	Terminal              SymbolType = 3
	NonTerminal           SymbolType = 4
)

// Symbol is rules.rs: struct Symbol.
type Symbol struct {
	Kind  SymbolType
	Index uint32
}

func terminalSym(i int) Symbol    { return Symbol{Terminal, uint32(i)} }
func nonTerminalSym(i int) Symbol { return Symbol{NonTerminal, uint32(i)} }
func externalSym(i int) Symbol    { return Symbol{External, uint32(i)} }
func endSym() Symbol              { return Symbol{End, 0} }

// IsTerminal reports whether the symbol is a lexical token.
func (s Symbol) IsTerminal() bool    { return s.Kind == Terminal }
func (s Symbol) isNonTerminal() bool { return s.Kind == NonTerminal }
func (s Symbol) isExternal() bool    { return s.Kind == External }

// IsEOF reports whether the symbol is the end-of-input token.
func (s Symbol) IsEOF() bool { return s.Kind == End }

// symbolLess is the derived Ord of rules.rs: struct Symbol.
func symbolLess(a, b Symbol) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	return a.Index < b.Index
}

// VariableType is grammars.rs: enum VariableType.
type VariableType uint8

const (
	VarHidden VariableType = iota
	VarAuxiliary
	VarAnonymous
	VarNamed
)

// RuleKind is the discriminant of rules.rs: enum Rule.
type RuleKind uint8

const (
	RBlank RuleKind = iota
	RString
	RPattern
	RNamedSymbol
	RSym
	RSeq
	RChoice
	RRepeat
	REof
	RMetadata
	RReserved
)

// RuleID indexes RulePool.nodes (rules.rs: struct RuleId).
type RuleID int

// ParamsID indexes RulePool.params (rules.rs: struct ParamsId).
type ParamsID int

// Rule is rules.rs: enum Rule, as one struct. Which fields are used depends on
// Kind: RString uses Str; RPattern uses Str and Flags; RNamedSymbol uses Str;
// RSym uses SymKind and Index; RSeq and RChoice use Children; RRepeat uses
// Inner; RMetadata uses Params and Inner; RReserved uses Inner and Str.
//
// Go shape: the Rust enum copies by value and Seq/Choice hold a range into a
// shared children vector. Here Children is a slice that is never mutated after
// it is built, which gives the same behaviour.
type Rule struct {
	Kind     RuleKind
	Str      string
	Flags    string
	SymKind  SymbolType
	Index    uint32
	Children []RuleID
	Inner    RuleID
	Params   ParamsID
}

// PrecKind is the discriminant of rules.rs: enum Precedence.
type PrecKind uint8

const (
	PrecNone PrecKind = iota
	PrecInteger
	PrecName
)

// Precedence is rules.rs: enum Precedence.
type Precedence struct {
	Kind  PrecKind
	Int   int32
	Ident string
}

// Associativity is rules.rs: enum Associativity (0 means none).
type Associativity uint8

const (
	AssocNone Associativity = iota
	AssocLeft
	AssocRight
)

// Alias is rules.rs: struct Alias, with Set for the Option.
type Alias struct {
	Set   bool
	Value string
	Named bool
}

// MetadataParams is rules.rs: struct MetadataParams. It is comparable, as the
// derived PartialEq is.
type MetadataParams struct {
	Precedence        Precedence
	Associativity     Associativity
	DynamicPrecedence int32
	Alias             Alias
	FieldSet          bool
	Field             string
	IsToken           bool
	IsMainToken       bool
}

// RulePool is rules.rs: struct RulePool, an arena of rule nodes. Strings are
// held directly instead of through an interner (a StrId equals another StrId
// exactly when the strings are equal, so equality is unchanged).
type RulePool struct {
	nodes  []Rule
	params []MetadataParams
}

func (p *RulePool) node(id RuleID) Rule                 { return p.nodes[id] }
func (p *RulePool) setNode(id RuleID, r Rule)           { p.nodes[id] = r }
func (p *RulePool) paramsOf(id ParamsID) MetadataParams { return p.params[id] }

func (p *RulePool) pushNode(r Rule) RuleID {
	p.nodes = append(p.nodes, r)
	return RuleID(len(p.nodes) - 1)
}

func (p *RulePool) pushParams(m MetadataParams) ParamsID {
	p.params = append(p.params, m)
	return ParamsID(len(p.params) - 1)
}

func (r Rule) symbol() (Symbol, bool) {
	if r.Kind == RSym {
		return Symbol{r.SymKind, r.Index}, true
	}
	return Symbol{}, false
}

func symRule(s Symbol) Rule { return Rule{Kind: RSym, SymKind: s.Kind, Index: s.Index} }

// metadataWith is rules.rs: fn metadata_with.
func (p *RulePool) metadataWith(content RuleID, f func(*MetadataParams)) RuleID {
	if n := p.node(content); n.Kind == RMetadata {
		m := p.paramsOf(n.Params)
		if !m.IsToken {
			f(&m)
			p.params[n.Params] = m
			return content
		}
	}
	var m MetadataParams
	f(&m)
	id := p.pushParams(m)
	return p.pushNode(Rule{Kind: RMetadata, Params: id, Inner: content})
}

// The constructors below are rules.rs: fn blank, string, pattern,
// named_symbol, field, alias, token, immediate_token, prec, prec_left,
// prec_right, prec_dynamic, repeat, eof, seq, reserved.
func (p *RulePool) blank() RuleID { return p.pushNode(Rule{Kind: RBlank}) }
func (p *RulePool) str(v string) RuleID {
	return p.pushNode(Rule{Kind: RString, Str: v})
}
func (p *RulePool) pattern(v, flags string) RuleID {
	return p.pushNode(Rule{Kind: RPattern, Str: v, Flags: flags})
}
func (p *RulePool) namedSymbol(name string) RuleID {
	return p.pushNode(Rule{Kind: RNamedSymbol, Str: name})
}
func (p *RulePool) field(name string, c RuleID) RuleID {
	return p.metadataWith(c, func(m *MetadataParams) { m.FieldSet, m.Field = true, name })
}
func (p *RulePool) alias(c RuleID, v string, named bool) RuleID {
	return p.metadataWith(c, func(m *MetadataParams) { m.Alias = Alias{true, v, named} })
}
func (p *RulePool) token(c RuleID) RuleID {
	return p.metadataWith(c, func(m *MetadataParams) { m.IsToken = true })
}
func (p *RulePool) immediateToken(c RuleID) RuleID {
	return p.metadataWith(c, func(m *MetadataParams) { m.IsToken = true; m.IsMainToken = true })
}
func (p *RulePool) prec(v Precedence, c RuleID) RuleID {
	return p.metadataWith(c, func(m *MetadataParams) { m.Precedence = v })
}
func (p *RulePool) precLeft(v Precedence, c RuleID) RuleID {
	return p.metadataWith(c, func(m *MetadataParams) { m.Associativity = AssocLeft; m.Precedence = v })
}
func (p *RulePool) precRight(v Precedence, c RuleID) RuleID {
	return p.metadataWith(c, func(m *MetadataParams) { m.Associativity = AssocRight; m.Precedence = v })
}
func (p *RulePool) precDynamic(v int32, c RuleID) RuleID {
	return p.metadataWith(c, func(m *MetadataParams) { m.DynamicPrecedence = v })
}
func (p *RulePool) repeat(c RuleID) RuleID { return p.pushNode(Rule{Kind: RRepeat, Inner: c}) }
func (p *RulePool) eof() RuleID            { return p.pushNode(Rule{Kind: REof}) }
func (p *RulePool) seq(ids []RuleID) RuleID {
	return p.pushNode(Rule{Kind: RSeq, Children: append([]RuleID(nil), ids...)})
}
func (p *RulePool) reserved(c RuleID, ctx string) RuleID {
	return p.pushNode(Rule{Kind: RReserved, Inner: c, Str: ctx})
}

// choice is rules.rs: fn choice. It flattens nested choices left to right and
// drops a later element that is subtree-equal to an earlier one.
func (p *RulePool) choice(ids []RuleID) RuleID {
	var elements []RuleID
	stack := make([]RuleID, 0, len(ids))
	for i := len(ids) - 1; i >= 0; i-- {
		stack = append(stack, ids[i])
	}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n := p.node(id); n.Kind == RChoice {
			base := len(stack)
			stack = append(stack, n.Children...)
			reverseIDs(stack[base:])
		} else {
			dup := false
			for _, e := range elements {
				if p.subtreeEq(e, id) {
					dup = true
					break
				}
			}
			if !dup {
				elements = append(elements, id)
			}
		}
	}
	return p.pushNode(Rule{Kind: RChoice, Children: elements})
}

func reverseIDs(s []RuleID) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

// subtreeEq is rules.rs: fn subtree_eq.
func (p *RulePool) subtreeEq(a, b RuleID) bool {
	type pair struct{ a, b RuleID }
	stack := []pair{{a, b}}
	for len(stack) > 0 {
		pr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		x, y := p.node(pr.a), p.node(pr.b)
		if x.Kind != y.Kind {
			return false
		}
		switch x.Kind {
		case RBlank, REof:
		case RString, RNamedSymbol:
			if x.Str != y.Str {
				return false
			}
		case RPattern:
			if x.Str != y.Str || x.Flags != y.Flags {
				return false
			}
		case RSym:
			if x.SymKind != y.SymKind || x.Index != y.Index {
				return false
			}
		case RSeq, RChoice:
			if len(x.Children) != len(y.Children) {
				return false
			}
			for i := range x.Children {
				stack = append(stack, pair{x.Children[i], y.Children[i]})
			}
		case RRepeat:
			stack = append(stack, pair{x.Inner, y.Inner})
		case RMetadata:
			if p.paramsOf(x.Params) != p.paramsOf(y.Params) {
				return false
			}
			stack = append(stack, pair{x.Inner, y.Inner})
		case RReserved:
			if x.Str != y.Str {
				return false
			}
			stack = append(stack, pair{x.Inner, y.Inner})
		}
	}
	return true
}

// subtreeMatchesEmptyStr is rules.rs: fn subtree_matches_empty_str.
func (p *RulePool) subtreeMatchesEmptyStr(id RuleID) bool {
	n := p.node(id)
	switch n.Kind {
	case RString:
		return n.Str == ""
	case RMetadata, RRepeat, RReserved:
		return p.subtreeMatchesEmptyStr(n.Inner)
	case RChoice:
		for _, c := range n.Children {
			if p.subtreeMatchesEmptyStr(c) {
				return true
			}
		}
		return false
	case RSeq:
		for _, c := range n.Children {
			if !p.subtreeMatchesEmptyStr(c) {
				return false
			}
		}
		return true
	}
	return false
}

// ruleIsReferenced is rules.rs: fn rule_is_referenced.
func (p *RulePool) ruleIsReferenced(id RuleID, target string, isExternal bool) bool {
	n := p.node(id)
	switch n.Kind {
	case RNamedSymbol:
		return n.Str == target && !isExternal
	case RChoice, RSeq:
		for _, c := range n.Children {
			if p.ruleIsReferenced(c, target, false) {
				return true
			}
		}
		return false
	case RMetadata, RReserved:
		return p.ruleIsReferenced(n.Inner, target, isExternal)
	case RRepeat:
		return p.ruleIsReferenced(n.Inner, target, false)
	}
	return false
}

// collectReferencedIDs is rules.rs: fn collect_referenced_ids.
func (p *RulePool) collectReferencedIDs(id RuleID, skipTopLevel bool, out *[]string) {
	n := p.node(id)
	switch n.Kind {
	case RNamedSymbol:
		if !skipTopLevel {
			*out = append(*out, n.Str)
		}
	case RChoice, RSeq:
		for _, c := range n.Children {
			p.collectReferencedIDs(c, false, out)
		}
	case RMetadata, RReserved:
		p.collectReferencedIDs(n.Inner, skipTopLevel, out)
	case RRepeat:
		p.collectReferencedIDs(n.Inner, false, out)
	}
}

// NotPortedError reports a construct that the port does not handle yet.
type NotPortedError struct {
	Construct string
	Where     string
}

func (e *NotPortedError) Error() string {
	if e.Where != "" {
		return fmt.Sprintf("lexbuild: not ported yet: %s (in %s)", e.Construct, e.Where)
	}
	return fmt.Sprintf("lexbuild: not ported yet: %s", e.Construct)
}
