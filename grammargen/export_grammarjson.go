package grammargen

import (
	"encoding/json"
	"fmt"
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
	if out.Externals == nil {
		out.Externals = []interface{}{}
	}
	if out.Inline == nil {
		out.Inline = []string{}
	}
	if out.Supertypes == nil {
		out.Supertypes = []string{}
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
	Externals    []interface{}          `json:"externals"`
	Inline       []string               `json:"inline"`
	Supertypes   []string               `json:"supertypes"`
	Gotreesitter *grammarJSONExtensions `json:"gotreesitter,omitempty"`
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

// exportRule converts a Rule to its JSON representation as a map.
func exportRule(r *Rule) interface{} {
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
