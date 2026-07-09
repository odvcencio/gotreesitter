package grammargen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
)

// shippedGrammarBlobsDir is grammars/grammar_blobs, read directly from disk
// (not imported via the grammars package) so this file stays within
// grammargen/* — it never touches or depends on grammars/* build tags.
const shippedGrammarBlobsDir = "../grammars/grammar_blobs"

// nonTerminalAliasMapExpectedNonEmptyLanguages lists every language whose
// SHIPPING ts2go blob is known (per the Wave-7 audit, cross-checked by
// TestShippedBlobsNonTerminalAliasMapInventory below) to carry a non-empty
// Language.NonTerminalAliasMap. Lua is the only one today: its
// _doublequote_string_content / _singlequote_string_content aux wrappers are
// each referenced both unaliased (within their own repeat1 self-recursion)
// and aliased to string_content (from _quote_string), so their possible
// display genuinely varies by occurrence and can't be folded into a single
// static entry the way single-target aliases can.
//
// If this set ever needs to grow, add a dedicated parity test mirroring
// TestLuaNonTerminalAliasMapParity for the new language before adding it
// here — this map is a deliberate gate, not just documentation.
var nonTerminalAliasMapExpectedNonEmptyLanguages = map[string]bool{
	"lua": true,
}

// loadShippedBlob reads and decodes a shipped grammar blob by language name
// (the .bin file's base name), skipping the calling test if the blob
// directory isn't present in this checkout.
func loadShippedBlob(t *testing.T, name string) *gotreesitter.Language {
	t.Helper()
	path := filepath.Join(shippedGrammarBlobsDir, name+".bin")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("shipped blob not available at %s: %v", path, err)
	}
	lang, err := gotreesitter.LoadLanguage(data)
	if err != nil {
		t.Fatalf("load shipped %s blob: %v", name, err)
	}
	return lang
}

// TestShippedBlobsNonTerminalAliasMapInventory scans every shipped grammar
// blob and asserts that the set of languages carrying a non-empty
// NonTerminalAliasMap matches nonTerminalAliasMapExpectedNonEmptyLanguages
// exactly. This is a tripwire: if a future ts2go re-extraction starts
// carrying NonTerminalAliasMap data for a language grammargen doesn't yet
// have parity coverage for, this test fails loudly instead of that language
// silently shipping an unverified grammargen-derived alias map later.
func TestShippedBlobsNonTerminalAliasMapInventory(t *testing.T) {
	entries, err := os.ReadDir(shippedGrammarBlobsDir)
	if err != nil {
		t.Skipf("shipped grammar blobs dir not available at %s: %v", shippedGrammarBlobsDir, err)
	}

	scanned := 0
	found := make(map[string]bool)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".bin" {
			continue
		}
		langName := strings.TrimSuffix(e.Name(), ".bin")
		data, err := os.ReadFile(filepath.Join(shippedGrammarBlobsDir, e.Name()))
		if err != nil {
			t.Errorf("read %s: %v", e.Name(), err)
			continue
		}
		lang, err := gotreesitter.LoadLanguage(data)
		if err != nil {
			// Not this test's concern (covered elsewhere); skip malformed/
			// unrelated blobs rather than failing the inventory scan.
			continue
		}
		scanned++
		for _, row := range lang.NonTerminalAliasMap {
			if len(row) > 0 {
				found[langName] = true
				break
			}
		}
	}
	if scanned == 0 {
		t.Skip("no shipped grammar blobs found to scan")
	}

	var unexpected []string
	for lang := range found {
		if !nonTerminalAliasMapExpectedNonEmptyLanguages[lang] {
			unexpected = append(unexpected, lang)
		}
	}
	var missing []string
	for lang := range nonTerminalAliasMapExpectedNonEmptyLanguages {
		if !found[lang] {
			missing = append(missing, lang)
		}
	}
	sort.Strings(unexpected)
	sort.Strings(missing)

	if len(unexpected) > 0 {
		t.Errorf("found %d language(s) with a non-empty shipped NonTerminalAliasMap not covered by a grammargen parity test: %v — add a dedicated parity test (mirroring TestLuaNonTerminalAliasMapParity) and register it in nonTerminalAliasMapExpectedNonEmptyLanguages before trusting grammargen's derivation for them", len(unexpected), unexpected)
	}
	if len(missing) > 0 {
		t.Errorf("expected non-empty NonTerminalAliasMap for %v but their shipped blobs carry none — update nonTerminalAliasMapExpectedNonEmptyLanguages or investigate a ts2go regression", missing)
	}
	t.Logf("scanned %d shipped grammar blobs; non-empty NonTerminalAliasMap languages = %v", scanned, sortedKeys(found))
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// symIdentity identifies a grammar symbol by its externally-visible shape
// (display name, named/anonymous, visible/hidden) for diagnostic display.
// It is NOT used as the primary correlation key between a ts2go blob and a
// grammargen-generated one — see nonTerminalAliasMapByName for why.
type symIdentity struct {
	name    string
	named   bool
	visible bool
}

func (s symIdentity) String() string {
	kind := "anon"
	if s.named {
		kind = "named"
	}
	vis := "hidden"
	if s.visible {
		vis = "visible"
	}
	return fmt.Sprintf("%s(%s,%s)", s.name, kind, vis)
}

// nonTerminalAliasMapRow is one row of a Language's NonTerminalAliasMap,
// keyed by the row symbol's own name (see nonTerminalAliasMapByName), with
// the row symbol's and each alias target's full identity retained for
// diagnostics.
type nonTerminalAliasMapRow struct {
	symbol  symIdentity
	aliases []symIdentity
}

// nonTerminalAliasMapByName re-keys lang.NonTerminalAliasMap from numeric
// symbol IDs to symbol NAMES (dropping IDs from the correlation key), so maps
// from two differently-numbered Language instances (e.g. a ts2go blob vs. a
// grammargen-generated one) can be compared directly. Each row retains the
// full symIdentity (name + named + visible) of both the row symbol and its
// alias targets purely for diagnostic display.
//
// Matching by name only — not the full symIdentity — is deliberate. It keeps
// this comparison scoped to what NonTerminalAliasMap actually encodes ("symbol
// named X can display as symbol(s) named Y"), and makes it robust to a
// separate, pre-existing divergence unrelated to alias-map derivation:
// grammargen's grammar.json importer currently classifies every hidden
// (underscore-prefixed) nonterminal rule as Named=true, while tree-sitter's
// real compiler demotes a hidden rule whose ENTIRE body is a bare
// repeat/repeat1 to Named=false ("aux_sym" status, confirmed directly against
// tree-sitter-lua's pinned parser.c: aux_sym__doublequote_string_content has
// {visible:false, named:false}). grammargen already has the equivalent
// demotion logic (markStructuralGeneratedRepeatAuxSymbols in normalize.go)
// but it only fires for grammargen's OWN auto-generated "*_repeatN" aux
// names, not for user-named rules that expandTopLevelRepeat rewrites into the
// same self-recursive shape. That gap is orthogonal to this feature —
// buildNonTerminalAliasMap (assemble.go) only inspects SymbolKind, never
// Named/Visible — so it's intentionally out of scope for this derivation and
// tracked here (and surfaced non-fatally by TestLuaNonTerminalAliasMapParity)
// instead of silently masked or misreported as an alias-map bug.
func nonTerminalAliasMapByName(lang *gotreesitter.Language) map[string]nonTerminalAliasMapRow {
	identityOf := func(sym int) symIdentity {
		if sym < 0 || sym >= len(lang.SymbolMetadata) {
			return symIdentity{name: fmt.Sprintf("<out-of-range:%d>", sym)}
		}
		m := lang.SymbolMetadata[sym]
		name := m.Name
		if name == "" && sym < len(lang.SymbolNames) {
			name = lang.SymbolNames[sym]
		}
		return symIdentity{name: name, named: m.Named, visible: m.Visible}
	}

	out := make(map[string]nonTerminalAliasMapRow)
	for sym, aliases := range lang.NonTerminalAliasMap {
		if len(aliases) == 0 {
			continue
		}
		rowSym := identityOf(sym)
		vals := make([]symIdentity, len(aliases))
		for i, a := range aliases {
			vals[i] = identityOf(int(a))
		}
		out[rowSym.name] = nonTerminalAliasMapRow{symbol: rowSym, aliases: vals}
	}
	return out
}

// diffNonTerminalAliasMapEntries compares two name-keyed NonTerminalAliasMap
// views (see nonTerminalAliasMapByName) and returns human-readable
// diagnostic lines for every discrepancy: rows present in only one side, and
// rows present on both sides whose alias-target name SETS differ. Comparison
// is set-based (order within a row is not significant — "same symbol→alias
// entries" is a per-symbol set relationship, not an array-equality one) and
// name-based (see nonTerminalAliasMapByName for why Named/Visible are shown
// for diagnosis but do not themselves fail the comparison).
func diffNonTerminalAliasMapEntries(ref, gen map[string]nonTerminalAliasMapRow) []string {
	toNameSet := func(vals []symIdentity) map[string]bool {
		s := make(map[string]bool, len(vals))
		for _, v := range vals {
			s[v.name] = true
		}
		return s
	}

	var diffs []string
	seen := make(map[string]bool)
	for key, refRow := range ref {
		seen[key] = true
		genRow, ok := gen[key]
		if !ok {
			diffs = append(diffs, fmt.Sprintf("MISSING in grammargen: %q -> %v (ts2go has this row, grammargen has none)", key, refRow.aliases))
			continue
		}
		refSet, genSet := toNameSet(refRow.aliases), toNameSet(genRow.aliases)
		var onlyRef, onlyGen []string
		for v := range refSet {
			if !genSet[v] {
				onlyRef = append(onlyRef, v)
			}
		}
		for v := range genSet {
			if !refSet[v] {
				onlyGen = append(onlyGen, v)
			}
		}
		if len(onlyRef) > 0 || len(onlyGen) > 0 {
			sort.Strings(onlyRef)
			sort.Strings(onlyGen)
			diffs = append(diffs, fmt.Sprintf("MISMATCH for %q: ts2go-only=%v grammargen-only=%v (ts2go row=%v, grammargen row=%v)", key, onlyRef, onlyGen, refRow.aliases, genRow.aliases))
		}
	}
	for key, genRow := range gen {
		if seen[key] {
			continue
		}
		diffs = append(diffs, fmt.Sprintf("EXTRA in grammargen: %q -> %v (grammargen has this row, ts2go has none)", key, genRow.aliases))
	}
	sort.Strings(diffs)
	return diffs
}

// luaGrammarJSONPathForTest locates the pinned tree-sitter-lua grammar.json
// (see grammars/grammar_updates.json for the pinned commit), following the
// same candidate-path convention as rustGrammarJSONPathForTest /
// sqlGrammarJSONPathForTest. Seed it locally with:
//
//	cgo_harness/seed_parity_repos.sh --langs lua
func luaGrammarJSONPathForTest(t *testing.T) string {
	t.Helper()

	candidates := []string{
		"/tmp/grammar_parity/lua/src/grammar.json",
		".parity_seed/lua/src/grammar.json",
		"../.parity_seed/lua/src/grammar.json",
	}
	globs := []string{
		"/tmp/gotreesitter-parity-*/repos/lua/src/grammar.json",
	}
	for _, pattern := range globs {
		matches, err := filepath.Glob(pattern)
		if err == nil && len(matches) > 0 {
			candidates = append(candidates, matches...)
		}
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	t.Skip("lua grammar.json not available (run cgo_harness/seed_parity_repos.sh --langs lua)")
	return ""
}

// TestLuaNonTerminalAliasMapParity is THE GATE for grammargen's
// NonTerminalAliasMap derivation: it imports the exact pinned tree-sitter-lua
// grammar.json, generates a Language through grammargen's own LR pipeline,
// and asserts the derived NonTerminalAliasMap is semantically identical (same
// symbol-name -> alias-target-name-set entries; see nonTerminalAliasMapByName
// for why correlation is name-based) to the map ts2go extracted from
// tree-sitter's real generated parser.c and shipped in
// grammars/grammar_blobs/lua.bin.
//
// A wrong alias map corrupts parse trees (see parser_reduce.go's
// buildAliasPreservedWrapperSymbols), so this test is intentionally strict:
// any row present on only one side, or any per-row alias-set mismatch, fails
// the test with a full diagnostic rather than silently passing on a
// coincidental partial match.
func TestLuaNonTerminalAliasMapParity(t *testing.T) {
	refLang := loadShippedBlob(t, "lua")
	refEntries := nonTerminalAliasMapByName(refLang)
	if len(refEntries) == 0 {
		t.Fatal("shipped lua blob unexpectedly has an empty NonTerminalAliasMap; nothing to verify parity against (has lua.bin regressed?)")
	}

	jsonPath := luaGrammarJSONPathForTest(t)
	source, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Skipf("lua grammar.json not available: %v", err)
	}
	gram, err := ImportGrammarJSON(source)
	if err != nil {
		t.Fatalf("import lua grammar.json: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	genLang, err := GenerateLanguageWithContext(ctx, gram)
	if err != nil {
		t.Fatalf("generate lua language via grammargen: %v", err)
	}
	genEntries := nonTerminalAliasMapByName(genLang)
	if len(genEntries) == 0 {
		t.Fatal("grammargen-derived lua NonTerminalAliasMap is empty; expected it to match the shipped ts2go blob's non-empty map")
	}

	if diffs := diffNonTerminalAliasMapEntries(refEntries, genEntries); len(diffs) > 0 {
		t.Fatalf("grammargen-derived NonTerminalAliasMap diverges from the shipped ts2go lua blob (%d row(s) in ts2go's map, %d in grammargen's):\n%s",
			len(refEntries), len(genEntries), strings.Join(diffs, "\n"))
	}

	// Non-fatal diagnostic: report (but do not fail on) any row-symbol or
	// alias-target Named/Visible metadata differences between the two sides.
	// See nonTerminalAliasMapByName's doc comment — this is a separate,
	// pre-existing, out-of-scope divergence in grammargen's Named
	// classification for hidden top-level-repeat rules, tracked here for
	// visibility rather than silently dropped.
	for name, refRow := range refEntries {
		genRow, ok := genEntries[name]
		if !ok {
			continue
		}
		if refRow.symbol.named != genRow.symbol.named || refRow.symbol.visible != genRow.symbol.visible {
			t.Logf("NOTE (non-fatal, pre-existing, out of scope): row symbol %q metadata differs: ts2go=%s grammargen=%s", name, refRow.symbol, genRow.symbol)
		}
		refAliasByName := make(map[string]symIdentity, len(refRow.aliases))
		for _, v := range refRow.aliases {
			refAliasByName[v.name] = v
		}
		for _, gv := range genRow.aliases {
			if rv, ok := refAliasByName[gv.name]; ok && (rv.named != gv.named || rv.visible != gv.visible) {
				t.Logf("NOTE (non-fatal, pre-existing, out of scope): alias-target %q metadata differs: ts2go=%s grammargen=%s", gv.name, rv, gv)
			}
		}
	}
}

// TestNonTerminalAliasMapSyntheticSelfAndAlias is a fast, offline (no
// grammar.json corpus required) regression pin for the core derivation rule:
// a hidden nonterminal whose ENTIRE body is repeat1(...) and is referenced
// exactly once, aliased, from elsewhere — mirroring Lua's
// _doublequote_string_content / _quote_string shape exactly — must end up
// with a NonTerminalAliasMap row containing both itself (from its own
// repeat1 self-recursion, which is unaliased) and the alias target.
func TestNonTerminalAliasMapSyntheticSelfAndAlias(t *testing.T) {
	g := NewGrammar("test_alias_map_self_and_alias")
	g.Define("source_file", Seq(Str(`"`),
		Field("content", Choice(Alias(Sym("_content"), "quoted_content", true), Blank())),
		Str(`"`)))
	g.Define("_content", Repeat1(Choice(Sym("char_tok"), Sym("escape"))))
	g.Define("char_tok", Pat(`[^"\\]+`))
	g.Define("escape", Str(`\`))

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}

	contentSym, quotedSym := -1, -1
	for i, name := range lang.SymbolNames {
		if name == "_content" && contentSym < 0 {
			contentSym = i
		}
		if name == "quoted_content" && quotedSym < 0 {
			quotedSym = i
		}
	}
	if contentSym < 0 {
		t.Fatal("missing _content symbol")
	}
	if quotedSym < 0 {
		t.Fatal("missing quoted_content alias symbol")
	}

	if contentSym >= len(lang.NonTerminalAliasMap) || len(lang.NonTerminalAliasMap[contentSym]) == 0 {
		t.Fatalf("expected a non-empty NonTerminalAliasMap row for _content (symbol %d); got %v", contentSym, lang.NonTerminalAliasMap)
	}
	row := lang.NonTerminalAliasMap[contentSym]
	if len(row) != 2 {
		t.Fatalf("_content alias map row = %v (len %d), want exactly 2 entries (self + quoted_content)", row, len(row))
	}
	if row[0] != gotreesitter.Symbol(contentSym) {
		t.Fatalf("_content alias map row[0] = %d, want self (%d) listed first", row[0], contentSym)
	}
	if row[1] != gotreesitter.Symbol(quotedSym) {
		t.Fatalf("_content alias map row[1] = %d, want quoted_content (%d)", row[1], quotedSym)
	}
}

// TestNonTerminalAliasMapSyntheticSingletonAliasOmitted pins the other half
// of the derivation rule: a nonterminal that is ALWAYS aliased the exact same
// single way, everywhere it's referenced (no unaliased occurrence, no
// varying alias target), gets NO NonTerminalAliasMap entry — its one
// possible display is representable by a single static value, matching
// tree-sitter's public_symbol_map fallback, so no dynamic row is needed.
//
// Two DIFFERENT hidden nonterminals (_always_a, _always_b) both alias to the
// SAME target name ("renamed") deliberately: grammargen's pre-existing
// default-alias-promotion optimization (normalize.go's promoteDefaultAliases)
// would otherwise eat a singly-aliased hidden symbol by renaming it directly
// to its alias target before AliasInfo/NonTerminalAliasMap derivation ever
// sees it, which would make this test pass trivially without exercising
// buildNonTerminalAliasMap's own singleton-omission guard. Two hidden symbols
// wanting the same target name is promoteDefaultAliases' own documented
// collision case — it leaves BOTH un-promoted, so both stay as real,
// distinctly-aliased symbols for this test to check.
func TestNonTerminalAliasMapSyntheticSingletonAliasOmitted(t *testing.T) {
	g := NewGrammar("test_alias_map_singleton_omitted")
	g.Define("source_file", Seq(Sym("user1"), Sym("user2")))
	g.Define("user1", Alias(Sym("_always_a"), "renamed", true))
	g.Define("user2", Alias(Sym("_always_b"), "renamed", true))
	// Seq(...) bodies (not a bare string literal) so normalize.go's
	// hidden-rule-collapses-to-anonymous-terminal optimization doesn't kick
	// in and eat these as plain terminals before they ever become
	// nonterminal symbols — see normalize.go's "Hidden named tokens that are
	// pure STRING literals" handling.
	g.Define("_always_a", Seq(Str("a"), Str("aa")))
	g.Define("_always_b", Seq(Str("b"), Str("bb")))

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}

	checked := 0
	for _, want := range []string{"_always_a", "_always_b"} {
		sym := -1
		for i, name := range lang.SymbolNames {
			if name == want && !lang.SymbolMetadata[i].Visible {
				sym = i
				break
			}
		}
		if sym < 0 {
			t.Fatalf("%s was unexpectedly promoted away or missing; this test requires both hidden symbols to survive as distinct entries to exercise the singleton-omission guard (SymbolNames=%v)", want, lang.SymbolNames)
		}
		checked++
		if sym < len(lang.NonTerminalAliasMap) && len(lang.NonTerminalAliasMap[sym]) != 0 {
			t.Fatalf("%s got a NonTerminalAliasMap row %v, want none (always aliased identically, single static display)", want, lang.NonTerminalAliasMap[sym])
		}
	}
	if checked != 2 {
		t.Fatalf("checked %d symbols, want 2", checked)
	}
}

// TestNonTerminalAliasMapSyntheticTerminalAliasExcluded confirms that
// aliasing a TERMINAL (not a nonterminal) never produces a
// NonTerminalAliasMap entry, matching ts_non_terminal_alias_map's scope.
func TestNonTerminalAliasMapSyntheticTerminalAliasExcluded(t *testing.T) {
	g := NewGrammar("test_alias_map_terminal_excluded")
	g.Define("source_file", Seq(
		Sym("identifier"),
		Alias(Str("+"), "op", false),
		Sym("identifier"),
	))
	g.Define("identifier", Pat(`[a-zA-Z_][a-zA-Z0-9_]*`))

	lang, err := GenerateLanguage(g)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	for sym, row := range lang.NonTerminalAliasMap {
		if len(row) == 0 {
			continue
		}
		t.Fatalf("unexpected non-empty NonTerminalAliasMap row for symbol %d (%s): %v — terminal aliasing must not produce a non-terminal alias map entry", sym, lang.SymbolNames[sym], row)
	}
}

// TestNonTerminalAliasMapDeterministic guards against a map-iteration-order
// regression: buildNonTerminalAliasMap (assemble.go) accumulates each row's
// alias set in a Go map before emitting it as a slice, so a future edit that
// drops the explicit sort before appending non-self entries could make the
// derived blob non-reproducible across otherwise-identical generation runs
// (an ordering flake that would be very hard to bisect later, since gob
// encoding would still "succeed" either way). Generate the same
// three-distinct-alias-targets grammar repeatedly and require byte-identical
// (deep-equal) NonTerminalAliasMap output every time, including the
// ordering of non-self entries within a row.
func TestNonTerminalAliasMapDeterministic(t *testing.T) {
	newGrammar := func() *Grammar {
		g := NewGrammar("test_alias_map_determinism")
		g.Define("source_file", Seq(Sym("user1"), Sym("user2"), Sym("user3"), Sym("user4")))
		// _multi is referenced unaliased once (contributes self) and aliased
		// to three DIFFERENT targets from three other call sites — enough
		// spread in the alias set to make ordering meaningful.
		g.Define("user1", Sym("_multi"))
		g.Define("user2", Alias(Sym("_multi"), "alias_c", true))
		g.Define("user3", Alias(Sym("_multi"), "alias_a", true))
		g.Define("user4", Alias(Sym("_multi"), "alias_b", true))
		g.Define("_multi", Seq(Str("m"), Str("m2")))
		return g
	}

	var first [][]gotreesitter.Symbol
	for i := 0; i < 25; i++ {
		lang, err := GenerateLanguage(newGrammar())
		if err != nil {
			t.Fatalf("iteration %d: GenerateLanguage: %v", i, err)
		}
		if len(lang.NonTerminalAliasMap) == 0 {
			t.Fatalf("iteration %d: NonTerminalAliasMap is empty, expected a _multi row", i)
		}
		if i == 0 {
			first = lang.NonTerminalAliasMap
			continue
		}
		if !reflect.DeepEqual(first, lang.NonTerminalAliasMap) {
			t.Fatalf("iteration %d: NonTerminalAliasMap is non-deterministic across identical generation runs\nfirst=%v\nthis =%v", i, first, lang.NonTerminalAliasMap)
		}
	}
}
