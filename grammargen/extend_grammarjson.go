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
//   - "extras", "conflicts": unioned (appended) after the base's existing
//     entries. Conflicts are rarely redefined by name, so delta entries are
//     always additive.
//   - "externals": unioned (appended) after the base's existing entries,
//     de-duplicated by name against the base's externals (and against
//     earlier entries in the same delta) — a delta external that names an
//     external the base already declares is skipped rather than appended a
//     second time. This matters beyond simple redundancy: externals are
//     registered into the same shared terminal symbol table as every other
//     token, keyed by name, so an unconditional append would register a
//     duplicate ExternalSymbols slot for a symbol ID that's already
//     registered, inflating ExternalTokenCount and desyncing the compiled
//     grammar's external-scanner ABI from the actual number of distinct
//     external tokens. Only externals with a real, stable name (declared
//     via a symbol reference or an anonymous string literal) participate in
//     this de-dup; anonymous regex-pattern externals have no pre-registration
//     name and are always additive, same as before.
//   - "precedences": delta levels are appended after the base's existing
//     precedence levels (so the delta cannot reorder base precedence
//     levels), and a delta rule's named PREC_LEFT/PREC_RIGHT/PREC/PREC_DYNAMIC
//     reference is resolved against the MERGED base+delta named precedences
//     — a delta rule can reference a named level the BASE declares (proper
//     inheritance), not only one the delta declares itself. A name that is
//     still unresolved after merging both is a hard error (ExtendGrammarJSON
//     fails loudly rather than silently emitting a rule with precedence 0).
//   - "supertypes", "inline": unioned (appended, de-duplicated, order
//     preserved — base entries first).
//   - "word": if non-empty, REPLACES the base's word token name.
//   - "reserved": each named reserved-word set REPLACES the base set of the
//     same name if present, or is appended as a new set otherwise. The
//     merged reserved sets are then pruned to nil under the same condition
//     ImportGrammarJSON prunes them: if neither the base grammar nor any
//     delta rule ever used a RESERVED wrapper, the sets have no per-context
//     usage to attach to and are dropped, matching ImportGrammarJSON's
//     runtime (no-filter) fallback for the equivalent single-document
//     import.
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

	// The delta's named precedences are resolved against the MERGED
	// base+delta precedence levels (base levels first, delta levels
	// appended after, matching how g.Precedences itself is merged below) —
	// a delta rule can therefore reference a named PREC_LEFT/PREC_RIGHT
	// level that the base grammar already declares (common: JS/TS-style
	// grammars define named precs like "binary"/"unary" once in the base
	// and derived grammars only add rules that use them), not just levels
	// the delta declares itself. Proper inheritance, not delta-only scoping.
	//
	// strictNamedPrecs is set so an unresolved name is a hard error rather
	// than silently resolving to prec 0 — a delta typo or a genuinely
	// missing precedence level should fail loudly instead of producing a
	// grammar with silently wrong (flattened-to-0) precedence.
	deltaPrecLevels := importPrecedenceLevels(raw.Precedences)
	mergedPrecLevels := append(append([][]PrecEntry(nil), g.Precedences...), deltaPrecLevels...)
	namedPrecs := buildNamedPrecMapFromLevels(mergedPrecLevels)
	conv := &jsonConverter{namedPrecs: namedPrecs, strictNamedPrecs: true}

	// baseHadReservedSets records whether g already carried reserved word
	// sets before this delta was applied. ImportGrammarJSON only keeps a
	// grammar's ReservedWordSets when at least one rule used a RESERVED
	// wrapper (see its sawReservedNode-gated pruning); a non-empty
	// g.ReservedWordSets here therefore already means the base grammar (or
	// an earlier delta application) needed them. Used below to apply the
	// equivalent gating to the merged result.
	baseHadReservedSets := len(g.ReservedWordSets) > 0

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

	// Externals: union, de-duplicated by name against the base's existing
	// externals. registerExternalSymbols (normalize.go) registers each
	// external through symbolTable.addSymbol keyed by name, sharing the
	// same terminal namespace as every other terminal; a delta external
	// that shares a name with a base external therefore resolves to the
	// SAME symbol ID at that point. But registerExternalSymbols still
	// unconditionally appends that (possibly-reused) symbol ID to
	// ExternalSymbols for every entry in g.Externals, so an unconditional
	// append here would silently double-count the external in
	// ExternalTokenCount (assemble.go: ExternalTokenCount =
	// len(ng.ExternalSymbols)) and corrupt the external-scanner ABI, since
	// the scanner is called with an index into ExternalTokenCount-sized
	// bitsets/arrays that no longer line up with the base's own scanner
	// contract. Skip a delta external whose name already exists (in the
	// base, or earlier in this same delta) instead.
	externalNames := make(map[string]bool, len(g.Externals))
	for _, ext := range g.Externals {
		if name, ok := externalIdentityName(ext); ok {
			externalNames[name] = true
		}
	}
	for _, ext := range raw.Externals {
		rule, err := conv.convertRule(ext)
		if err != nil {
			return fmt.Errorf("delta externals: %w", err)
		}
		if name, ok := externalIdentityName(rule); ok {
			if externalNames[name] {
				continue
			}
			externalNames[name] = true
		}
		g.Externals = append(g.Externals, rule)
	}

	// Precedences: append delta levels after the base's existing levels.
	if len(deltaPrecLevels) > 0 {
		g.Precedences = append(g.Precedences, deltaPrecLevels...)
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

	// Align with ImportGrammarJSON's sawReservedNode-gated pruning: if
	// neither the base (baseHadReservedSets) nor this delta's own rules
	// (conv.sawReservedNode) ever used a RESERVED wrapper, drop any
	// reserved sets the delta's "reserved" field declared purely
	// declaratively — they'd have no per-context RESERVED usage to attach
	// to, so ImportGrammarJSON would have dropped them too had this same
	// merged document been imported as one grammar.json. Without this, a
	// delta that adds a "reserved" block but never wraps a rule in
	// RESERVED would keep sets ImportGrammarJSON would silently discard for
	// the equivalent single-document grammar, diverging from the runtime
	// (no-filter) fallback ImportGrammarJSON's callers depend on.
	if !baseHadReservedSets && !conv.sawReservedNode {
		g.ReservedWordSets = nil
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

// externalIdentityName returns the name registerExternalSymbols
// (normalize.go) would register an external Rule's symbol table entry
// under, for the two external kinds that carry a real, stable name:
//   - RuleSymbol (e.g. `$._ext`): name is the symbol name.
//   - RuleString (e.g. an anonymous external literal): name is the literal
//     value — registerExternalSymbols uses ext.Value as the registration
//     name for both kinds, and symbolTable.addSymbol's terminal namespace
//     is shared across all terminal kinds, so a RuleSymbol and RuleString
//     external with the same Value would collide there too.
//
// RulePattern externals return ok=false: registerExternalSymbols only
// gives them a stable name when the same pattern already exists as an
// inline terminal (in which case it reuses that terminal's symbol instead
// of registering a new one); otherwise it assigns a synthetic name
// (_token1, _token2, ...) derived from position within the FULL merged
// externals list, which isn't knowable at delta-merge time. RulePattern
// externals are therefore always additive here, same as before this fix.
func externalIdentityName(r *Rule) (string, bool) {
	if r == nil {
		return "", false
	}
	switch r.Kind {
	case RuleSymbol, RuleString:
		return r.Value, true
	default:
		return "", false
	}
}
