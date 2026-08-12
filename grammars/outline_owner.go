package grammars

import (
	"strings"
	"sync"

	"github.com/odvcencio/gotreesitter"
)

// outlineOwnerRuleCache caches OutlineOwnerRules' gated result by language
// name, exactly like inferredTagsQueryCache caches ResolveTagsQuery's
// inference (tags_query_infer.go). Gating loads the language's compiled
// grammar, so caching keeps repeated calls for the same language cheap.
var outlineOwnerRuleCache sync.Map // map[string][]gotreesitter.OutlineOwnerRule

// outlineOwnerRuleTable is the per-language table of declarative outline
// owner rules, keyed by language name. Each row is a candidate, not a
// guarantee: OutlineOwnerRules gates every row against the language's own
// symbol and field table (Language.SymbolByName, Language.FieldByName)
// before returning it, exactly as inferredTagsQuery gates a shared pattern
// against SymbolByName (tags_query_infer.go). A row whose NodeType,
// OwnerField, or any Unwrap/NameTypes entry the language does not define is
// left out before a caller ever sees it, so a regenerated grammar that
// renames or drops a node type narrows coverage instead of feeding
// gotreesitter.WithOutlineOwnerRules a shape it cannot resolve.
//
// See spec.outline-api.v1 section 2 for the resolution contract this table
// feeds, and section 3's gating argument for why this table is gated by
// symbol/field presence rather than by the blob-SHA mechanism
// runtime_profiles.go uses: an owner rule cannot alter a parse result, so its
// worst failure is an omission or a caught divergence, never corrupted
// output.
var outlineOwnerRuleTable = map[string][]gotreesitter.OutlineOwnerRule{
	// Go: a method's receiver is the sole child of the "receiver" field's
	// parameter_list, unwrapped through the parameter declaration, then an
	// optional pointer and/or generic wrapper, down to the bare type name.
	//
	// Verified against this repository's compiled Go grammar blob by parsing
	// value, pointer, generic-value, and generic-pointer receivers and
	// reading Node.SExpr(lang): every shape reaches exactly one
	// type_identifier through this Unwrap chain. A generic receiver's type
	// ARGUMENT (the "T" in "Box[T]") sits inside "type_arguments", a node
	// type this rule does not list in Unwrap, so the walk never reaches it
	// and resolves the base type name alone.
	"go": {
		{
			NodeType:   "method_declaration",
			OwnerField: "receiver",
			Unwrap:     []string{"parameter_list", "parameter_declaration", "pointer_type", "generic_type"},
			NameTypes:  []string{"type_identifier"},
		},
	},
}

// OutlineOwnerRules returns the declarative owner-resolution rules for entry,
// gated by symbol and field presence in the language's own compiled grammar:
// a candidate row whose NodeType, OwnerField, or any Unwrap/NameTypes entry
// the language does not define is left out. The result is nil for a
// language with no candidate row, or none that survives gating, and
// gotreesitter.WithOutlineOwnerRules(nil) is a no-op, so callers compose
// unconditionally:
//
//	outliner, err := gotreesitter.NewOutliner(lang, query,
//	    gotreesitter.WithOutlineOwnerRules(grammars.OutlineOwnerRules(entry)))
//
// Gating loads entry's compiled grammar on first call for a given language
// name; the result is cached, so repeated calls are cheap.
func OutlineOwnerRules(entry LangEntry) []gotreesitter.OutlineOwnerRule {
	name := strings.TrimSpace(entry.Name)
	if name == "" {
		return nil
	}
	if cached, ok := outlineOwnerRuleCache.Load(name); ok {
		return cached.([]gotreesitter.OutlineOwnerRule)
	}

	gated := gateOutlineOwnerRules(entry, outlineOwnerRuleTable[name])
	outlineOwnerRuleCache.Store(name, gated)
	return gated
}

// gateOutlineOwnerRules drops every candidate row entry's own grammar cannot
// resolve: a NodeType or OwnerField the grammar does not define, or an
// Unwrap/NameTypes entry that names a node type the grammar does not define.
func gateOutlineOwnerRules(entry LangEntry, candidates []gotreesitter.OutlineOwnerRule) []gotreesitter.OutlineOwnerRule {
	if len(candidates) == 0 {
		return nil
	}
	if entry.Language == nil {
		return nil
	}
	lang := entry.Language()
	if lang == nil {
		return nil
	}

	hasSymbol := func(symbol string) bool {
		_, ok := lang.SymbolByName(symbol)
		return ok
	}
	hasField := func(field string) bool {
		_, ok := lang.FieldByName(field)
		return ok
	}

	var gated []gotreesitter.OutlineOwnerRule
	for _, rule := range candidates {
		if !hasSymbol(rule.NodeType) || !hasField(rule.OwnerField) {
			continue
		}
		if !everyOutlineOwnerTypeExists(rule.Unwrap, hasSymbol) {
			continue
		}
		if !everyOutlineOwnerTypeExists(rule.NameTypes, hasSymbol) {
			continue
		}
		gated = append(gated, rule)
	}
	return gated
}

// everyOutlineOwnerTypeExists reports whether hasSymbol accepts every entry
// in types. An empty types list vacuously passes: this gate checks presence,
// not shape, and a shipped row's Unwrap or NameTypes list is never empty
// (validated at gotreesitter.NewOutliner construction for any rule a caller
// attaches; unreachable through this table's shipped rows).
func everyOutlineOwnerTypeExists(types []string, hasSymbol func(string) bool) bool {
	for _, t := range types {
		if !hasSymbol(t) {
			return false
		}
	}
	return true
}
