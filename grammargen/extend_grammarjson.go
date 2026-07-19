package grammargen

import (
	"encoding/json"
	"fmt"
)

// ExtendGrammarJSON builds a derived Grammar from two tree-sitter
// grammar.json documents: baseJSON (a full grammar, as accepted by
// ImportGrammarJSON) and deltaJSON (a partial grammar.json fragment
// describing the customization). It is the data-driven counterpart to
// ExtendGrammar — where ExtendGrammar takes a Go closure and therefore only
// works from Go code, ExtendGrammarJSON takes two byte slices and works from
// any caller that can produce JSON (a browser DSL playground, a non-Go
// adopter, a generated tool), with no Go toolchain involved.
//
// deltaJSON uses the same shape as a tree-sitter grammar.json file, so a
// caller writes standard grammar.json fragments rather than learning a new
// format. Every top-level field is optional; only fields present in
// deltaJSON are merged into the base grammar:
//
//   - "name": ignored — the returned grammar always uses the name argument.
//   - "rules": each entry is added if the rule name is new, or REPLACES the
//     base rule of the same name if it already exists (same override
//     semantics as calling Grammar.Define after cloning the base — this
//     mirrors ExtendGrammar's `g.Define(name, rule)` pattern).
//   - "extras", "conflicts", "externals": unioned (appended) after the
//     base's existing entries. Conflicts/externals are rarely redefined by
//     name, so delta entries are always additive.
//   - "precedences": delta levels are appended after the base's existing
//     precedence levels (so delta-only named precedences resolve within the
//     delta's own rules; the delta cannot reorder base precedence levels).
//   - "supertypes", "inline": unioned (appended, de-duplicated, order
//     preserved — base entries first).
//   - "word": if non-empty, REPLACES the base's word token name.
//   - "reserved": each named reserved-word set REPLACES the base set of the
//     same name if present, or is appended as a new set otherwise.
//
// All of the base grammar's resolved tuning flags (EnableLRSplitting,
// BinaryRepeatMode, PreferPreciseExternalLexStates, ExactPrefixStates, the
// per-language Prefer* conflict-resolution biases set by
// ImportGrammarJSON's shape hints, and so on) are carried into the returned
// grammar unchanged, via the same clone helper ExtendGrammar uses
// (cloneGrammarForExtend) — this preserves generation fidelity for imported
// grammars (e.g. extending an imported "javascript" grammar keeps its
// BinaryRepeatMode/FlattenGeneratedRepeatAux shape hints).
//
// The base grammar itself is never mutated: ImportGrammarJSON(baseJSON)
// parses a fresh Grammar and cloneGrammarForExtend deep-copies it before the
// delta is applied.
func ExtendGrammarJSON(name string, baseJSON, deltaJSON []byte) (*Grammar, error) {
	base, err := ImportGrammarJSON(baseJSON)
	if err != nil {
		return nil, fmt.Errorf("extend grammar %q: parse base grammar.json: %w", name, err)
	}

	g := cloneGrammarForExtend(name, base)

	if err := applyGrammarJSONDelta(g, deltaJSON); err != nil {
		return nil, fmt.Errorf("extend grammar %q: apply delta: %w", name, err)
	}

	return g, nil
}

// applyGrammarJSONDelta parses deltaJSON as a (partial) grammar.json
// document and merges it into g in place. See ExtendGrammarJSON for the
// documented merge semantics of each field.
func applyGrammarJSONDelta(g *Grammar, deltaJSON []byte) error {
	var raw jsonGrammar
	if err := json.Unmarshal(deltaJSON, &raw); err != nil {
		return fmt.Errorf("parse delta grammar.json: %w", err)
	}

	// The delta's named precedences (if any) are resolved against the
	// delta's own precedences array only — a delta rule that references a
	// named PREC_LEFT/PREC_RIGHT level must declare that level in its own
	// "precedences" array, same as ImportGrammarJSON requires for a full
	// grammar.json document.
	namedPrecs := buildNamedPrecMap(raw.Precedences)
	conv := &jsonConverter{namedPrecs: namedPrecs}

	// Rules: add new rules, or replace an existing base rule of the same
	// name (Grammar.Define already implements exactly this add-or-replace
	// semantics).
	for _, name := range raw.ruleOrder {
		rule, err := conv.convertRule(raw.Rules[name])
		if err != nil {
			return fmt.Errorf("delta rule %q: %w", name, err)
		}
		g.Define(name, rule)
	}

	// Extras: union.
	for _, extra := range raw.Extras {
		rule, err := conv.convertRule(extra)
		if err != nil {
			return fmt.Errorf("delta extras: %w", err)
		}
		g.Extras = append(g.Extras, rule)
	}

	// Conflicts: union.
	for _, group := range raw.Conflicts {
		if len(group) == 0 {
			continue
		}
		names := append([]string(nil), group...)
		g.Conflicts = append(g.Conflicts, names)
	}

	// Externals: union.
	for _, ext := range raw.Externals {
		rule, err := conv.convertRule(ext)
		if err != nil {
			return fmt.Errorf("delta externals: %w", err)
		}
		g.Externals = append(g.Externals, rule)
	}

	// Precedences: append delta levels after the base's existing levels.
	if len(raw.Precedences) > 0 {
		g.Precedences = append(g.Precedences, importPrecedenceLevels(raw.Precedences)...)
	}

	// Supertypes / inline: union, de-duplicated, base-first order preserved.
	g.Supertypes = unionStringsPreserveOrder(g.Supertypes, raw.Supertypes)
	g.Inline = unionStringsPreserveOrder(g.Inline, raw.Inline)

	// Word: delta replaces the base word token if specified.
	if raw.Word != "" {
		g.Word = raw.Word
	}

	// Reserved word sets: replace an existing base set of the same name, or
	// append a new one.
	for _, name := range raw.reservedOrder {
		members := raw.Reserved[name]
		rules := make([]*Rule, 0, len(members))
		for _, member := range members {
			rule, err := conv.convertRule(member)
			if err != nil {
				return fmt.Errorf("delta reserved %q: %w", name, err)
			}
			rules = append(rules, rule)
		}
		replaced := false
		for i, existing := range g.ReservedWordSets {
			if existing.Name == name {
				g.ReservedWordSets[i] = ReservedWordSet{Name: name, Rules: rules}
				replaced = true
				break
			}
		}
		if !replaced {
			g.ReservedWordSets = append(g.ReservedWordSets, ReservedWordSet{Name: name, Rules: rules})
		}
	}

	return nil
}

// unionStringsPreserveOrder returns base with any entries from extra that
// aren't already present appended in extra's order. base is not mutated in
// place (a new slice is returned when extra contributes anything new).
func unionStringsPreserveOrder(base, extra []string) []string {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]bool, len(base)+len(extra))
	for _, s := range base {
		seen[s] = true
	}
	out := base
	for _, s := range extra {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
