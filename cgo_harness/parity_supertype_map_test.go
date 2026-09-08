//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// goSupertypeMap returns the Go language's supertype map in the same shape
// as cOracleSupertypeMap: supertype labels in table order, each mapped to
// its subtype labels in table order.
func goSupertypeMap(lang *gotreesitter.Language) (order []string, subtypes map[string][]string) {
	label := func(sym gotreesitter.Symbol) string {
		name := ""
		if int(sym) < len(lang.SymbolNames) {
			name = lang.SymbolNames[sym]
		}
		if int(sym) < len(lang.SymbolMetadata) && !lang.SymbolMetadata[sym].Named {
			return name + "(anon)"
		}
		return name
	}
	subtypes = make(map[string][]string, len(lang.SupertypeSymbols))
	for _, super := range lang.SupertypeSymbols {
		name := label(super)
		order = append(order, name)
		var labels []string
		for _, sub := range lang.SupertypeChildren(super) {
			labels = append(labels, label(sub))
		}
		subtypes[name] = labels
	}
	return order, subtypes
}

// supertypeMapDivergence lists the supertypes whose subtype sets differ
// between the two maps, with the entries each side lacks.
func supertypeMapDivergence(cOrder []string, cSubs map[string][]string, goOrder []string, goSubs map[string][]string) []string {
	var out []string
	seen := map[string]bool{}
	for _, name := range append(append([]string{}, cOrder...), goOrder...) {
		if seen[name] {
			continue
		}
		seen[name] = true
		cSet, goSet := map[string]bool{}, map[string]bool{}
		cList, cOK := cSubs[name]
		goList, goOK := goSubs[name]
		for _, s := range cList {
			cSet[s] = true
		}
		for _, s := range goList {
			goSet[s] = true
		}
		var missing, extra []string
		for s := range cSet {
			if !goSet[s] {
				missing = append(missing, s)
			}
		}
		for s := range goSet {
			if !cSet[s] {
				extra = append(extra, s)
			}
		}
		sort.Strings(missing)
		sort.Strings(extra)
		switch {
		case cOK && !goOK:
			out = append(out, fmt.Sprintf("%s: Go lists no such supertype", name))
		case goOK && !cOK:
			out = append(out, fmt.Sprintf("%s: C lists no such supertype", name))
		case len(missing) > 0 || len(extra) > 0:
			out = append(out, fmt.Sprintf("%s: Go lacks %v, Go adds %v", name, missing, extra))
		}
	}
	return out
}

// TestParitySupertypeMap compares the ABI 15 supertype map of every Go
// grammar with the C runtime's ts_language_supertypes/ts_language_subtypes.
// The query compiler validates `super/sub` patterns against this map, so a
// wrong map rejects patterns C accepts or accepts patterns C rejects.
// Divergent languages are reported; GTS_PARITY_SUPERTYPE_MAP_STRICT=1 turns
// the report into a failure.
func TestParitySupertypeMap(t *testing.T) {
	strict := parityEnvBool("GTS_PARITY_SUPERTYPE_MAP_STRICT", false)
	var divergent, agree, absent []string
	for _, entry := range grammars.AllLanguages() {
		lang := entry.Language()
		if lang == nil {
			continue
		}
		hasSupertype := false
		for _, m := range lang.SymbolMetadata {
			if m.Supertype {
				hasSupertype = true
				break
			}
		}
		if !hasSupertype {
			continue
		}
		raw, err := cOracleRawLanguage(entry.Name)
		if err != nil {
			if reason := parityReferenceSkipReason(err); reason != "" {
				absent = append(absent, entry.Name)
				continue
			}
			t.Fatalf("%s: load C oracle: %v", entry.Name, err)
		}
		cOrder, cSubs := cOracleSupertypeMap(raw)
		goOrder, goSubs := goSupertypeMap(lang)
		if len(cOrder) == 0 && len(goOrder) == 0 {
			// ABI 14 on both sides: the flag exists, the map does not.
			agree = append(agree, entry.Name)
			continue
		}
		diff := supertypeMapDivergence(cOrder, cSubs, goOrder, goSubs)
		if len(diff) == 0 {
			agree = append(agree, entry.Name)
			continue
		}
		divergent = append(divergent, entry.Name)
		t.Logf("%s (C ABI %d, %d C supertypes, %d Go supertypes):\n  %s", entry.Name, cOracleABIVersion(raw), len(cOrder), len(goOrder), strings.Join(diff, "\n  "))
	}
	sort.Strings(divergent)
	t.Logf("supertype map board: agree=%d divergent=%d no-C-reference=%d\n divergent: %v", len(agree), len(divergent), len(absent), divergent)
	if strict && len(divergent) > 0 {
		t.Fatalf("%d languages carry a supertype map that differs from C", len(divergent))
	}
}
