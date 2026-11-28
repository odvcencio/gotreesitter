package grammargen

import (
	"encoding/json"
	"fmt"
	"sort"
)

// ExportGrammarJSON serializes a Grammar struct to the tree-sitter grammar.json
// format. The output is compatible with ImportGrammarJSON — a round-trip
// ImportGrammarJSON(ExportGrammarJSON(g)) should produce an equivalent grammar.
//
// The JSON structure matches tree-sitter's canonical resolved grammar.json:
//
//	{
//	  "name": "...",
//	  "word": "...",
//	  "rules": { ... },
//	  "extras": [...],
//	  "conflicts": [...],
//	  "precedences": [...],
//	  "externals": [...],
//	  "inline": [...],
//	  "supertypes": [...]
//	}
func ExportGrammarJSON(g *Grammar) ([]byte, error) {
	out := exportGrammar{
		Name:       g.Name,
		Word:       g.Word,
		Inline:     g.Inline,
		Supertypes: g.Supertypes,
	}

	// Rules — preserve definition order via an ordered JSON object.
	out.Rules = &orderedRules{
		order: g.RuleOrder,
		rules: make(map[string]interface{}, len(g.Rules)),
	}
	for name, rule := range g.Rules {
		out.Rules.rules[name] = exportRule(rule)
	}

	// Extras.
	for _, extra := range g.Extras {
		out.Extras = append(out.Extras, exportRule(extra))
	}

	// Conflicts.
	out.Conflicts = g.Conflicts

	// Precedences: each level lists named precedences (STRING) and rules
	// (SYMBOL) from highest to lowest, as ImportGrammarJSON reads them.
	out.Precedences = exportPrecedenceLevels(g.Precedences)

	// Externals.
	for _, ext := range g.Externals {
		out.Externals = append(out.Externals, exportRule(ext))
	}

	// Ensure nil slices become empty arrays in JSON.
	if out.Extras == nil {
		out.Extras = []interface{}{}
	}
	if out.Conflicts == nil {
		out.Conflicts = [][]string{}
	}
	if out.Precedences == nil {
		out.Precedences = [][]interface{}{}
	}
	if out.Externals == nil {
		out.Externals = []interface{}{}
	}
	if out.Inline == nil {
		out.Inline = []string{}
	}
	if out.Supertypes == nil {
		out.Supertypes = []string{}
	}

	// Reserved word sets, in order: the first is the global set.
	if len(g.ReservedWordSets) > 0 {
		out.Reserved = &orderedReserved{sets: g.ReservedWordSets}
	}

	if g.WantsForest {
		out.Gotreesitter = &grammarJSONExtensions{WantsForest: g.WantsForest}
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal grammar.json: %w", err)
	}
	// Append trailing newline for clean file output.
	data = append(data, '\n')
	return data, nil
}

// exportGrammar is the JSON structure for grammar.json output.
type exportGrammar struct {
	Name         string                 `json:"name"`
	Word         string                 `json:"word,omitempty"`
	Rules        *orderedRules          `json:"rules"`
	Extras       []interface{}          `json:"extras"`
	Conflicts    [][]string             `json:"conflicts"`
	Precedences  [][]interface{}        `json:"precedences"`
	Externals    []interface{}          `json:"externals"`
	Inline       []string               `json:"inline"`
	Supertypes   []string               `json:"supertypes"`
	Reserved     *orderedReserved       `json:"reserved,omitempty"`
	Gotreesitter *grammarJSONExtensions `json:"gotreesitter,omitempty"`
}

// exportPrecedenceLevels converts the Grammar's precedence levels to the
// grammar.json form: a STRING entry for a named precedence and a SYMBOL entry
// for a rule.
func exportPrecedenceLevels(levels [][]PrecEntry) [][]interface{} {
	if len(levels) == 0 {
		return nil
	}
	out := make([][]interface{}, 0, len(levels))
	for _, level := range levels {
		entries := make([]interface{}, 0, len(level))
		for _, e := range level {
			if e.IsSymbol {
				entries = append(entries, jsonObject{"type": "SYMBOL", "name": e.Name})
				continue
			}
			entries = append(entries, jsonObject{"type": "STRING", "value": e.Name})
		}
		out = append(out, entries)
	}
	return out
}

// orderedReserved marshals reserved word sets as a JSON object in set order.
type orderedReserved struct {
	sets []ReservedWordSet
}

func (o *orderedReserved) MarshalJSON() ([]byte, error) {
	buf := []byte{'{'}
	for i, set := range o.sets {
		if i > 0 {
			buf = append(buf, ',')
		}
		key, err := json.Marshal(set.Name)
		if err != nil {
			return nil, err
		}
		members := make([]interface{}, len(set.Rules))
		for j, r := range set.Rules {
			members[j] = exportRule(r)
		}
		val, err := json.Marshal(members)
		if err != nil {
			return nil, err
		}
		buf = append(buf, key...)
		buf = append(buf, ':')
		buf = append(buf, val...)
	}
	buf = append(buf, '}')
	return buf, nil
}

// orderedRules preserves rule order when marshaling to JSON.
type orderedRules struct {
	order []string
	rules map[string]interface{}
}

func (o *orderedRules) MarshalJSON() ([]byte, error) {
	// Build an ordered JSON object manually.
	buf := []byte{'{'}
	for i, name := range o.order {
		if i > 0 {
			buf = append(buf, ',')
		}
		key, err := json.Marshal(name)
		if err != nil {
			return nil, err
		}
		val, err := json.Marshal(o.rules[name])
		if err != nil {
			return nil, err
		}
		buf = append(buf, key...)
		buf = append(buf, ':')
		buf = append(buf, val...)
	}
	buf = append(buf, '}')
	return buf, nil
}

// jsonObject is a JSON object whose keys keep the order of the text that
// tree-sitter writes: "type" first, then the keys of the rule in the order its
// JavaScript constructor sets them (ALIAS: content, named, value; the other
// rules: name, value, content, members). A Go map marshals its keys sorted,
// which puts "name" before "type".
type jsonObject map[string]interface{}

var jsonKeyOrder = []string{"type", "name", "value", "content", "members", "named"}
var jsonAliasKeyOrder = []string{"type", "content", "named", "value"}

func (o jsonObject) MarshalJSON() ([]byte, error) {
	order := jsonKeyOrder
	if o["type"] == "ALIAS" {
		order = jsonAliasKeyOrder
	}
	buf := []byte{'{'}
	n := 0
	emit := func(k string) error {
		v, ok := o[k]
		if !ok {
			return nil
		}
		if n > 0 {
			buf = append(buf, ',')
		}
		n++
		key, err := json.Marshal(k)
		if err != nil {
			return err
		}
		val, err := json.Marshal(v)
		if err != nil {
			return err
		}
		buf = append(append(append(buf, key...), ':'), val...)
		return nil
	}
	for _, k := range order {
		if err := emit(k); err != nil {
			return nil, err
		}
	}
	// Any other key follows, sorted.
	var rest []string
	for k := range o {
		known := false
		for _, ok := range order {
			known = known || ok == k
		}
		if !known {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		if err := emit(k); err != nil {
			return nil, err
		}
	}
	return append(buf, '}'), nil
}

// exportRule converts a Rule to its JSON representation.
func exportRule(r *Rule) interface{} {
	return jsonObject(exportRuleMap(r))
}

// exportRuleMap converts a Rule to a map; exportRule orders its keys.
func exportRuleMap(r *Rule) map[string]interface{} {
	if r == nil {
		return map[string]interface{}{"type": "BLANK"}
	}

	switch r.Kind {
	case RuleBlank:
		return map[string]interface{}{"type": "BLANK"}

	case RuleString:
		return map[string]interface{}{
			"type":  "STRING",
			"value": r.Value,
		}

	case RulePattern:
		return map[string]interface{}{
			"type":  "PATTERN",
			"value": r.Value,
		}

	case RuleSymbol:
		return map[string]interface{}{
			"type": "SYMBOL",
			"name": r.Value,
		}

	case RuleSeq:
		members := make([]interface{}, len(r.Children))
		for i, child := range r.Children {
			members[i] = exportRule(child)
		}
		return map[string]interface{}{
			"type":    "SEQ",
			"members": members,
		}

	case RuleChoice:
		members := make([]interface{}, len(r.Children))
		for i, child := range r.Children {
			members[i] = exportRule(child)
		}
		return map[string]interface{}{
			"type":    "CHOICE",
			"members": members,
		}

	case RuleRepeat:
		return map[string]interface{}{
			"type":    "REPEAT",
			"content": exportRule(childOrBlank(r)),
		}

	case RuleRepeat1:
		return map[string]interface{}{
			"type":    "REPEAT1",
			"content": exportRule(childOrBlank(r)),
		}

	case RuleOptional:
		// Optional(x) is sugar for CHOICE(x, BLANK) in grammar.json.
		return map[string]interface{}{
			"type": "CHOICE",
			"members": []interface{}{
				exportRule(childOrBlank(r)),
				map[string]interface{}{"type": "BLANK"},
			},
		}

	case RuleToken:
		return map[string]interface{}{
			"type":    "TOKEN",
			"content": exportRule(childOrBlank(r)),
		}

	case RuleImmToken:
		return map[string]interface{}{
			"type":    "IMMEDIATE_TOKEN",
			"content": exportRule(childOrBlank(r)),
		}

	case RuleField:
		return map[string]interface{}{
			"type":    "FIELD",
			"name":    r.Value,
			"content": exportRule(childOrBlank(r)),
		}

	case RulePrec:
		return map[string]interface{}{
			"type":    "PREC",
			"value":   r.Prec,
			"content": exportRule(childOrBlank(r)),
		}

	case RulePrecLeft:
		return map[string]interface{}{
			"type":    "PREC_LEFT",
			"value":   r.Prec,
			"content": exportRule(childOrBlank(r)),
		}

	case RulePrecRight:
		return map[string]interface{}{
			"type":    "PREC_RIGHT",
			"value":   r.Prec,
			"content": exportRule(childOrBlank(r)),
		}

	case RulePrecDynamic:
		return map[string]interface{}{
			"type":    "PREC_DYNAMIC",
			"value":   r.Prec,
			"content": exportRule(childOrBlank(r)),
		}

	case RuleAlias:
		return map[string]interface{}{
			"type":    "ALIAS",
			"value":   r.Value,
			"named":   r.Named,
			"content": exportRule(childOrBlank(r)),
		}

	default:
		return map[string]interface{}{"type": "BLANK"}
	}
}

// childOrBlank returns the first child of a rule or nil (for Blank).
func childOrBlank(r *Rule) *Rule {
	if len(r.Children) > 0 {
		return r.Children[0]
	}
	return nil
}
