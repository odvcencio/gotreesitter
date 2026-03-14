package grammargen

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
)

func TestDiagCompareLanguages(t *testing.T) {
	if os.Getenv("DIAG_GRAMMAR") == "" {
		t.Skip("set DIAG_GRAMMAR to run language comparison diagnostics")
	}
	grammarName := "ini"
	if v := os.Getenv("DIAG_GRAMMAR"); v != "" {
		grammarName = v
	}

	var g importParityGrammar
	for _, pg := range importParityGrammars {
		if pg.name == grammarName {
			g = pg
			break
		}
	}
	if g.name == "" {
		t.Fatalf("%s not found in importParityGrammars", grammarName)
	}

	gram, err := importParityGrammarSource(g)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	timeout := g.genTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	genLang, err := generateWithTimeout(gram, timeout)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	refLang := g.blobFunc()

	t.Logf("=== %s Language Comparison ===", grammarName)
	t.Logf("SymbolCount: gen=%d ref=%d", genLang.SymbolCount, refLang.SymbolCount)
	t.Logf("TokenCount:  gen=%d ref=%d", genLang.TokenCount, refLang.TokenCount)
	t.Logf("StateCount:  gen=%d ref=%d", genLang.StateCount, refLang.StateCount)

	// Symbol metadata comparison.
	t.Logf("\n--- SymbolMetadata ---")
	metaDiffs := 0
	genNameToMeta := map[string]gotreesitter.SymbolMetadata{}
	refNameToMeta := map[string]gotreesitter.SymbolMetadata{}
	for i, name := range genLang.SymbolNames {
		if i < len(genLang.SymbolMetadata) {
			genNameToMeta[name] = genLang.SymbolMetadata[i]
		}
	}
	for i, name := range refLang.SymbolNames {
		if i < len(refLang.SymbolMetadata) {
			refNameToMeta[name] = refLang.SymbolMetadata[i]
		}
	}
	for name, gm := range genNameToMeta {
		rm, ok := refNameToMeta[name]
		if !ok {
			continue
		}
		if gm.Visible != rm.Visible || gm.Named != rm.Named {
			t.Logf("  DIFF %q: gen(vis=%v,named=%v) ref(vis=%v,named=%v)",
				name, gm.Visible, gm.Named, rm.Visible, rm.Named)
			metaDiffs++
		}
	}
	genOnly := []string{}
	for name := range genNameToMeta {
		if _, ok := refNameToMeta[name]; !ok && !strings.HasPrefix(name, "_") {
			genOnly = append(genOnly, name)
		}
	}
	if len(genOnly) > 0 {
		sort.Strings(genOnly)
		t.Logf("  GEN-ONLY: %v", genOnly)
	}
	refOnly := []string{}
	for name := range refNameToMeta {
		if _, ok := genNameToMeta[name]; !ok && !strings.HasPrefix(name, "_") {
			refOnly = append(refOnly, name)
		}
	}
	if len(refOnly) > 0 {
		sort.Strings(refOnly)
		t.Logf("  REF-ONLY: %v", refOnly)
	}

	// Production map (reduce child counts per symbol).
	t.Logf("\n--- Productions (reduce child counts) ---")
	genProds := extractReduceChildCounts(genLang)
	refProds := extractReduceChildCounts(refLang)
	prodDiffs := 0
	allSyms := map[string]bool{}
	for k := range genProds {
		allSyms[k] = true
	}
	for k := range refProds {
		allSyms[k] = true
	}
	syms := make([]string, 0, len(allSyms))
	for k := range allSyms {
		syms = append(syms, k)
	}
	sort.Strings(syms)
	for _, sym := range syms {
		gc := genProds[sym]
		rc := refProds[sym]
		if !intSetsEqual(gc, rc) {
			gMissing := intSetDiff(rc, gc) // in ref but not gen
			gExtra := intSetDiff(gc, rc)   // in gen but not ref
			t.Logf("  DIFF %q: gen=%v ref=%v missing=%v extra=%v",
				sym, sortedIntSet(gc), sortedIntSet(rc), sortedIntSet(gMissing), sortedIntSet(gExtra))
			prodDiffs++
		}
	}

	// Alias sequences comparison.
	t.Logf("\n--- Aliases ---")
	t.Logf("  AliasSequences: gen=%d ref=%d", len(genLang.AliasSequences), len(refLang.AliasSequences))

	// Field comparison.
	t.Logf("\n--- Fields ---")
	t.Logf("  FieldNames: gen=%v ref=%v", genLang.FieldNames, refLang.FieldNames)
	t.Logf("  FieldMapEntries: gen=%d ref=%d", len(genLang.FieldMapEntries), len(refLang.FieldMapEntries))

	// Lex comparison.
	t.Logf("\n--- Lexer ---")
	t.Logf("  LexStates: gen=%d ref=%d", len(genLang.LexStates), len(refLang.LexStates))
	t.Logf("  LexModes:  gen=%d ref=%d", len(genLang.LexModes), len(refLang.LexModes))

	t.Logf("\n=== SUMMARY %s: metaDiffs=%d prodDiffs=%d ===", grammarName, metaDiffs, prodDiffs)
}

func extractReduceChildCounts(lang *gotreesitter.Language) map[string]map[int]bool {
	result := map[string]map[int]bool{}
	for _, entry := range lang.ParseActions {
		for _, a := range entry.Actions {
			if a.Type == gotreesitter.ParseActionReduce {
				name := fmt.Sprintf("sym%d", a.Symbol)
				if int(a.Symbol) < len(lang.SymbolNames) {
					name = lang.SymbolNames[a.Symbol]
				}
				if result[name] == nil {
					result[name] = map[int]bool{}
				}
				result[name][int(a.ChildCount)] = true
			}
		}
	}
	return result
}

func intSetsEqual(a, b map[int]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func intSetDiff(a, b map[int]bool) map[int]bool {
	d := map[int]bool{}
	for k := range a {
		if !b[k] {
			d[k] = true
		}
	}
	return d
}

func sortedIntSet(m map[int]bool) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}
