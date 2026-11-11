package lexbuild

import (
	"bytes"
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// Variable is grammars.rs: struct Variable.
type Variable struct {
	Ident string
	Root  RuleID
}

// PrecedenceEntry is grammars.rs: enum PrecedenceEntry.
type PrecedenceEntry struct {
	IsSymbol bool
	Ident    string
}

// ReservedWordContext is grammars.rs: struct ReservedWordContext.
type ReservedWordContext struct {
	Ident string
	Roots []RuleID
}

// InputGrammar is grammars.rs: struct InputGrammar. Precedence orderings,
// conflicts, supertypes and inline names are parsed and carried; the lexer
// path does not use them (validatePrecedences only checks them).
type InputGrammar struct {
	Pool                *RulePool
	Ident               string
	Variables           []Variable
	ExternalRoots       []RuleID
	ExtraRoots          []RuleID
	ReservedSets        []ReservedWordContext
	SupertypeNames      []string
	ConflictNames       [][]string
	InlineNames         []string
	WordName            string
	HasWord             bool
	PrecedenceOrderings [][]PrecedenceEntry
}

// ruleJSON is parse_grammar.rs: enum RuleJSON (internally tagged by "type").
type ruleJSON struct {
	Type        string          `json:"type"`
	Content     *ruleJSON       `json:"content"`
	Members     []ruleJSON      `json:"members"`
	Value       json.RawMessage `json:"value"`
	Flags       *string         `json:"flags"`
	Ident       string          `json:"name"`
	Named       bool            `json:"named"`
	ContextName string          `json:"context_name"`
}

// orderedObject keeps the key order of a JSON object. The generator reads
// grammar.json with the preserve_order feature of its JSON reader (the Cargo
// manifest: features = ["preserve_order"]), so the rule map is an IndexMap: rule
// order is file order, and a repeated key keeps its first position with the
// last value.
type orderedObject struct {
	keys []string
	vals []json.RawMessage
}

func (o *orderedObject) UnmarshalJSON(b []byte) error {
	k, v, _, err := decodeObject(b)
	o.keys, o.vals = k, v
	return err
}

// grammarJSON is parse_grammar.rs: struct GrammarJSON, as read by
// readGrammarJSON.
type grammarJSON struct {
	Ident       string
	Rules       orderedObject
	Precedences [][]ruleJSON
	Conflicts   [][]string
	Externals   []ruleJSON
	Extras      []ruleJSON
	Inline      []string
	Supertypes  []string
	Word        *string
	Reserved    json.RawMessage
}

func (o objView) array(name string) ([]json.RawMessage, error) {
	raw, ok := o.m[name]
	if !ok {
		return nil, nil
	}
	if t := bytes.TrimSpace(raw); len(t) == 0 || t[0] != '[' {
		return nil, fmt.Errorf("field `%s` is not an array", name)
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (o objView) rules(name string) ([]ruleJSON, error) {
	items, err := o.array(name)
	if err != nil {
		return nil, err
	}
	var out []ruleJSON
	for _, it := range items {
		r, err := decodeRule(it, true)
		if err != nil {
			return nil, fmt.Errorf("field `%s`: %v", name, err)
		}
		out = append(out, *r)
	}
	return out, nil
}

func (o objView) strings(name string) ([]string, error) {
	items, err := o.array(name)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, it := range items {
		var s string
		if t := bytes.TrimSpace(it); len(t) == 0 || t[0] != '"' || json.Unmarshal(it, &s) != nil {
			return nil, fmt.Errorf("field `%s` holds a value that is not a string", name)
		}
		out = append(out, s)
	}
	return out, nil
}

// readGrammarJSON reads grammar.json as serde does for struct GrammarJSON.
func readGrammarJSON(input []byte) (*grammarJSON, error) {
	if !utf8.Valid(input) {
		return nil, fmt.Errorf("stream did not contain valid UTF-8")
	}
	o, err := viewObject(input, true)
	if err != nil {
		return nil, err
	}
	// The fields that GrammarJSON knows. Every other key is skipped: it may repeat, and
	// its strings and numbers are not decoded or checked. The fields that are read are
	// checked for lone surrogate escapes and for numbers out of range.
	topFields := []string{"name", "rules", "precedences", "conflicts", "externals", "extras", "inline", "supertypes", "word", "reserved"}
	if err := o.checkDups(topFields...); err != nil {
		return nil, err
	}
	for _, f := range topFields {
		if raw, ok := o.m[f]; ok {
			if err := checkSurrogates(raw); err != nil {
				return nil, fmt.Errorf("field `%s`: %v", f, err)
			}
			if err := checkNumbers(raw); err != nil {
				return nil, fmt.Errorf("field `%s`: %v", f, err)
			}
			if err := checkDepth(raw, 1); err != nil {
				return nil, fmt.Errorf("field `%s`: %v", f, err)
			}
		}
	}
	gj := &grammarJSON{}
	if gj.Ident, _, err = o.str("name", true); err != nil {
		return nil, err
	}
	rr, ok := o.m["rules"]
	if !ok {
		return nil, fmt.Errorf("missing field `rules`")
	}
	if t := bytes.TrimSpace(rr); len(t) == 0 || t[0] != '{' {
		return nil, fmt.Errorf("field `rules` is not an object")
	}
	if err := gj.Rules.UnmarshalJSON(rr); err != nil {
		return nil, err
	}
	items, err := o.array("precedences")
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		var row []json.RawMessage
		if t := bytes.TrimSpace(it); len(t) == 0 || t[0] != '[' || json.Unmarshal(it, &row) != nil {
			return nil, fmt.Errorf("field `precedences` holds a value that is not an array")
		}
		var list []ruleJSON
		for _, e := range row {
			r, err := decodeRule(e, true)
			if err != nil {
				return nil, fmt.Errorf("field `precedences`: %v", err)
			}
			list = append(list, *r)
		}
		gj.Precedences = append(gj.Precedences, list)
	}
	items, err = o.array("conflicts")
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		var row []string
		var rawRow []json.RawMessage
		if t := bytes.TrimSpace(it); len(t) == 0 || t[0] != '[' || json.Unmarshal(it, &rawRow) != nil {
			return nil, fmt.Errorf("field `conflicts` holds a value that is not an array")
		}
		for _, e := range rawRow {
			var s string
			if t := bytes.TrimSpace(e); len(t) == 0 || t[0] != '"' || json.Unmarshal(e, &s) != nil {
				return nil, fmt.Errorf("field `conflicts` holds a value that is not a string")
			}
			row = append(row, s)
		}
		gj.Conflicts = append(gj.Conflicts, row)
	}
	if gj.Externals, err = o.rules("externals"); err != nil {
		return nil, err
	}
	if gj.Extras, err = o.rules("extras"); err != nil {
		return nil, err
	}
	if gj.Inline, err = o.strings("inline"); err != nil {
		return nil, err
	}
	if gj.Supertypes, err = o.strings("supertypes"); err != nil {
		return nil, err
	}
	if w, has, err := o.str("word", false); err != nil {
		return nil, err
	} else if has {
		gj.Word = &w
	}
	if r, ok := o.m["reserved"]; ok {
		if t := bytes.TrimSpace(r); len(t) == 0 || t[0] != '{' {
			return nil, fmt.Errorf("field `reserved` is not an object")
		}
		gj.Reserved = r
	}
	return gj, nil
}

// ParseGrammar is parse_grammar.rs: fn parse_grammar. Diagnostics (empty
// string matches, unary choices, unsupported pattern flags) are not produced.
func ParseGrammar(input []byte) (*InputGrammar, error) {
	gj, err := readGrammarJSON(input)
	if err != nil {
		return nil, fmt.Errorf("lexbuild: grammar.json: %v", err)
	}
	pool := &RulePool{}
	g := &InputGrammar{Pool: pool}

	for _, item := range gj.Extras {
		root, err := pool.parseRule(item, false)
		if err != nil {
			return nil, err
		}
		if n := pool.node(root); n.Kind == RString && n.Str == "" {
			return nil, fmt.Errorf("Rules in the `extras` array must not contain empty strings")
		}
		g.ExtraRoots = append(g.ExtraRoots, root)
	}
	for _, e := range gj.Externals {
		root, err := pool.parseRule(e, false)
		if err != nil {
			return nil, err
		}
		g.ExternalRoots = append(g.ExternalRoots, root)
	}
	for _, list := range gj.Precedences {
		var ordering []PrecedenceEntry
		for _, e := range list {
			switch e.Type {
			case "STRING":
				var s string
				if err := json.Unmarshal(e.Value, &s); err != nil {
					return nil, err
				}
				ordering = append(ordering, PrecedenceEntry{false, s})
			case "SYMBOL":
				ordering = append(ordering, PrecedenceEntry{true, e.Ident})
			default:
				return nil, fmt.Errorf("Invalid rule in precedences array. Only strings and symbols are allowed")
			}
		}
		g.PrecedenceOrderings = append(g.PrecedenceOrderings, ordering)
	}
	var reserved orderedObject
	if r := bytes.TrimSpace(gj.Reserved); len(r) != 0 && string(r) != "null" {
		if err := reserved.UnmarshalJSON(r); err != nil {
			return nil, fmt.Errorf("Reserved word sets must be arrays")
		}
	}
	for i, k := range gj.Rules.keys {
		rj, err := decodeRule(gj.Rules.vals[i], false)
		if err != nil {
			return nil, fmt.Errorf("lexbuild: rule %q: %v", k, err)
		}
		root, err := pool.parseRule(*rj, false)
		if err != nil {
			return nil, fmt.Errorf("lexbuild: rule %q: %w", k, err)
		}
		g.Variables = append(g.Variables, Variable{Ident: k, Root: root})
	}
	for i, k := range reserved.keys {
		var rawRules []json.RawMessage
		if t := bytes.TrimSpace(reserved.vals[i]); len(t) == 0 || t[0] != '[' || json.Unmarshal(reserved.vals[i], &rawRules) != nil {
			return nil, fmt.Errorf("Reserved word sets must be arrays")
		}
		set := ReservedWordContext{Ident: k}
		for _, rawRule := range rawRules {
			rj, err := decodeRule(rawRule, false)
			if err != nil {
				return nil, fmt.Errorf("lexbuild: reserved set %q: %v", k, err)
			}
			root, err := pool.parseRule(*rj, false)
			if err != nil {
				return nil, err
			}
			set.Roots = append(set.Roots, root)
		}
		g.ReservedSets = append(g.ReservedSets, set)
	}
	g.SupertypeNames = gj.Supertypes
	g.ConflictNames = gj.Conflicts
	g.InlineNames = gj.Inline
	if gj.Word != nil {
		g.WordName, g.HasWord = *gj.Word, true
	}
	g.Ident = gj.Ident
	g.normalize()
	return g, nil
}

// normalize is parse_grammar.rs: fn InputGrammar::normalize. It drops the
// variables that the start rule, the word token, the extras and the externals
// do not reach. (The empty-match diagnostic is not produced.)
func (g *InputGrammar) normalize() {
	pool := g.Pool
	byName := map[string]RuleID{}
	for _, v := range g.Variables {
		byName[v.Ident] = v.Root
	}
	visited := map[string]bool{}
	var stack []string
	if len(g.Variables) > 0 {
		stack = append(stack, g.Variables[0].Ident)
	}
	if g.HasWord {
		stack = append(stack, g.WordName)
	}
	for _, root := range g.ExtraRoots {
		pool.collectReferencedIDs(root, false, &stack)
	}
	for _, root := range g.ExternalRoots {
		pool.collectReferencedIDs(root, true, &stack)
	}
	for _, set := range g.ReservedSets {
		for _, root := range set.Roots {
			pool.collectReferencedIDs(root, false, &stack)
		}
	}
	for len(stack) > 0 {
		name := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if visited[name] {
			continue
		}
		visited[name] = true
		if root, ok := byName[name]; ok {
			pool.collectReferencedIDs(root, false, &stack)
		}
	}
	var dropped []string
	var kept []Variable
	for _, v := range g.Variables {
		if visited[v.Ident] {
			kept = append(kept, v)
		} else {
			dropped = append(dropped, v.Ident)
		}
	}
	g.Variables = kept
	for _, name := range dropped {
		g.ConflictNames = retainConflicts(g.ConflictNames, name)
		g.SupertypeNames = retainStrings(g.SupertypeNames, name)
		g.InlineNames = retainStrings(g.InlineNames, name)
		g.ExtraRoots = retainRoots(pool, g.ExtraRoots, name)
		g.ExternalRoots = retainRoots(pool, g.ExternalRoots, name)
		var po [][]PrecedenceEntry
		for _, o := range g.PrecedenceOrderings {
			drop := false
			for _, e := range o {
				if e.IsSymbol && e.Ident == name {
					drop = true
				}
			}
			if !drop {
				po = append(po, o)
			}
		}
		g.PrecedenceOrderings = po
		for k := range g.ReservedSets {
			var roots []RuleID
			for _, r := range g.ReservedSets[k].Roots {
				if !pool.ruleIsReferenced(r, name, false) {
					roots = append(roots, r)
				}
			}
			g.ReservedSets[k].Roots = roots
		}
	}
}

func retainStrings(s []string, name string) []string {
	var out []string
	for _, x := range s {
		if x != name {
			out = append(out, x)
		}
	}
	return out
}

func retainConflicts(c [][]string, name string) [][]string {
	var out [][]string
	for _, set := range c {
		has := false
		for _, x := range set {
			if x == name {
				has = true
			}
		}
		if !has {
			out = append(out, set)
		}
	}
	return out
}

func retainRoots(pool *RulePool, roots []RuleID, name string) []RuleID {
	var out []RuleID
	for _, r := range roots {
		if !pool.ruleIsReferenced(r, name, true) {
			out = append(out, r)
		}
	}
	return out
}

// parseRule is parse_grammar.rs: fn parse_rule.
func (p *RulePool) parseRule(j ruleJSON, isToken bool) (RuleID, error) {
	content := func() (RuleID, error) {
		if j.Content == nil {
			return 0, fmt.Errorf("rule of type %s has no content", j.Type)
		}
		return p.parseRule(*j.Content, isToken)
	}
	members := func(tok bool) ([]RuleID, error) {
		var out []RuleID
		for _, m := range j.Members {
			id, err := p.parseRule(m, tok)
			if err != nil {
				return nil, err
			}
			out = append(out, id)
		}
		return out, nil
	}
	precValue := func() (Precedence, error) {
		var i int32
		if err := json.Unmarshal(j.Value, &i); err == nil {
			return Precedence{Kind: PrecInteger, Int: i}, nil
		}
		var s string
		if err := json.Unmarshal(j.Value, &s); err != nil {
			return Precedence{}, fmt.Errorf("precedence value: %v", err)
		}
		return Precedence{Kind: PrecName, Ident: s}, nil
	}
	switch j.Type {
	case "ALIAS":
		c, err := content()
		if err != nil {
			return 0, err
		}
		var v string
		if err := json.Unmarshal(j.Value, &v); err != nil {
			return 0, err
		}
		return p.alias(c, v, j.Named), nil
	case "BLANK":
		return p.blank(), nil
	case "PATTERN":
		var v string
		if err := json.Unmarshal(j.Value, &v); err != nil {
			return 0, err
		}
		flags := ""
		if j.Flags != nil {
			for _, c := range *j.Flags {
				if c == 'i' {
					flags += "i"
				}
			}
		}
		return p.pattern(v, flags), nil
	case "SYMBOL":
		if isToken {
			return 0, fmt.Errorf("Grammar Error: Unexpected rule `%s` in `token()` call", j.Ident)
		}
		return p.namedSymbol(j.Ident), nil
	case "CHOICE":
		ms, err := members(isToken)
		if err != nil {
			return 0, err
		}
		return p.choice(ms), nil
	case "SEQ":
		ms, err := members(isToken)
		if err != nil {
			return 0, err
		}
		return p.seq(ms), nil
	case "FIELD":
		c, err := content()
		if err != nil {
			return 0, err
		}
		return p.field(j.Ident, c), nil
	case "REPEAT":
		c, err := content()
		if err != nil {
			return 0, err
		}
		r := p.repeat(c)
		b := p.blank()
		return p.choice([]RuleID{r, b}), nil
	case "REPEAT1":
		c, err := content()
		if err != nil {
			return 0, err
		}
		return p.repeat(c), nil
	case "PREC", "PREC_LEFT", "PREC_RIGHT":
		c, err := content()
		if err != nil {
			return 0, err
		}
		v, err := precValue()
		if err != nil {
			return 0, err
		}
		switch j.Type {
		case "PREC":
			return p.prec(v, c), nil
		case "PREC_LEFT":
			return p.precLeft(v, c), nil
		}
		return p.precRight(v, c), nil
	case "PREC_DYNAMIC":
		c, err := content()
		if err != nil {
			return 0, err
		}
		var v int32
		if err := json.Unmarshal(j.Value, &v); err != nil {
			return 0, err
		}
		return p.precDynamic(v, c), nil
	case "RESERVED":
		c, err := content()
		if err != nil {
			return 0, err
		}
		return p.reserved(c, j.ContextName), nil
	case "TOKEN", "IMMEDIATE_TOKEN":
		if j.Content == nil {
			return 0, fmt.Errorf("rule of type %s has no content", j.Type)
		}
		c, err := p.parseRule(*j.Content, true)
		if err != nil {
			return 0, err
		}
		if j.Type == "TOKEN" {
			return p.token(c), nil
		}
		return p.immediateToken(c), nil
	case "STRING":
		var v string
		if err := json.Unmarshal(j.Value, &v); err != nil {
			return 0, err
		}
		return p.str(v), nil
	case "EOF":
		return p.eof(), nil
	}
	return 0, fmt.Errorf("unknown rule type %q", j.Type)
}
