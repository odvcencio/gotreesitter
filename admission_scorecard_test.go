//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// TestAdmissionCandidateScorecard206 is the admission scorecard's missing half:
// it drives the compact candidate route across all 206 registered languages
// (one smoke fixture each) through the public Parse API and records, per
// language, whether the compact route served a byte-exact tree, declined and
// fell back, or diverged. The compact route had only ever been validated on the
// four canonical Go fixtures; this run reports how far it reaches.
//
// Scope: the per-language fixtures are trivial smoke snippets, so this is the
// breadth ratchet -- which grammars the compact route accepts at all -- rather
// than corpus-scale fidelity proof. Frozen canonical and representative-depth
// digests provide the deeper companion gate.
//
// It is a scorecard, not a gate: it never fails on a fallback (a fallback is the
// fail-closed, correct behavior for an unsupported grammar). It fails only on a
// DIVERGE — the compact route accepted a clean tree that disagrees with
// production — because a silent wrong tree is the one outcome the admission
// contract forbids. Set GTS_ADMISSION_SCORECARD_STRICT=1 to also fail if any
// DIVERGE is observed even when only logging is wanted otherwise.
//
// Statuses:
//
//   - PASS     candidate routed and its tree digest equals production's;
//   - DIVERGE  candidate routed but its tree digest differs from production;
//   - FALLBACK candidate declined; production served the parse (fail-closed);
//   - SKIP     language is not routable via the DFA Parse path (token source);
//   - ERROR    production itself failed or a panic was recovered.
const (
	scorecardPass     = "PASS"
	scorecardDiverge  = "DIVERGE"
	scorecardFallback = "FALLBACK"
	scorecardSkip     = "SKIP"
	scorecardError    = "ERROR"
)

type scorecardRow struct {
	name               string
	backend            string
	status             string
	detail             string
	productionHasError bool
}

// admissionScorecardRequiredCompactPasses is the frozen per-language admission
// manifest from the generalized clean-tail epoch. It intentionally lists only
// languages that the public compact route currently admits: a FALLBACK -> PASS
// improvement remains welcome, but a listed PASS -> FALLBACK/DIVERGE/ERROR is a
// release regression even when another language happens to improve and keeps
// the aggregate totals unchanged. This is a test-only release ratchet, never a
// runtime routing allowlist.
var admissionScorecardRequiredCompactPasses = map[string]struct{}{
	"ada": {}, "agda": {}, "angular": {}, "apex": {}, "arduino": {},
	"asm": {}, "astro": {}, "awk": {}, "bash": {}, "bass": {}, "beancount": {}, "bibtex": {},
	"bicep": {}, "bitbake": {}, "blade": {}, "brightscript": {}, "c_sharp": {},
	"caddy": {}, "cairo": {}, "capnp": {}, "chatito": {}, "circom": {},
	"clojure": {}, "cmake": {}, "cobol": {}, "comment": {}, "commonlisp": {}, "cooklang": {}, "corn": {}, "cpon": {}, "crystal": {}, "css": {},
	"csv": {}, "cuda": {}, "cue": {}, "cylc": {}, "d": {}, "dart": {},
	"desktop": {}, "devicetree": {}, "dhall": {}, "diff": {}, "disassembly": {}, "djot": {}, "dockerfile": {},
	"dot": {}, "doxygen": {}, "dtd": {}, "earthfile": {}, "ebnf": {}, "editorconfig": {},
	"eds": {}, "eex": {}, "elisp": {}, "elixir": {}, "elm": {}, "elsa": {}, "embedded_template": {}, "enforce": {},
	"erlang": {}, "facility": {}, "faust": {}, "fennel": {}, "fidl": {},
	"firrtl": {}, "fish": {}, "foam": {}, "forth": {}, "fortran": {}, "fsharp": {},
	"gdscript": {}, "git_config": {}, "git_rebase": {}, "gitattributes": {}, "gitcommit": {}, "gitignore": {}, "gleam": {},
	"glsl": {}, "gn": {}, "go": {}, "godot_resource": {}, "gomod": {}, "graphql": {},
	"groovy": {}, "hack": {}, "hare": {}, "haskell": {}, "haxe": {}, "hcl": {}, "heex": {},
	"hlsl": {}, "html": {}, "http": {}, "hurl": {}, "hyprlang": {}, "ini": {}, "janet": {}, "javascript": {}, "jinja2": {}, "jq": {}, "jsdoc": {},
	"json5": {}, "jsonnet": {}, "julia": {}, "just": {}, "kconfig": {}, "kdl": {}, "kotlin": {},
	"ledger": {}, "less": {}, "linkerscript": {}, "liquid": {}, "llvm": {}, "lua": {},
	"luau": {}, "make": {}, "markdown": {}, "matlab": {}, "mermaid": {}, "mojo": {},
	"move": {}, "nginx": {}, "nickel": {}, "nim": {}, "ninja": {}, "nix": {}, "norg": {}, "nushell": {},
	"objc": {}, "ocaml": {}, "odin": {}, "org": {}, "pascal": {}, "pem": {}, "perl": {},
	"php": {}, "pkl": {}, "powershell": {}, "prisma": {}, "prolog": {}, "promql": {},
	"properties": {}, "proto": {}, "pug": {}, "puppet": {}, "purescript": {}, "python": {}, "ql": {},
	"r": {}, "racket": {}, "regex": {}, "rego": {}, "requirements": {}, "rescript": {}, "robot": {}, "ron": {},
	"rst": {}, "ruby": {}, "rust": {}, "scala": {}, "scheme": {}, "scss": {}, "smithy": {},
	"solidity": {}, "sparql": {}, "sql": {}, "squirrel": {}, "starlark": {}, "svelte": {},
	"ssh_config": {}, "swift": {}, "tablegen": {}, "tcl": {}, "teal": {}, "templ": {}, "textproto": {},
	"thrift": {}, "tlaplus": {}, "tmux": {}, "todotxt": {}, "toml": {}, "tsx": {}, "turtle": {}, "twig": {},
	"typescript": {}, "typst": {}, "uxntal": {}, "v": {}, "verilog": {}, "vhdl": {}, "vimdoc": {},
	"vue": {}, "wat": {}, "wgsl": {}, "wolfram": {}, "xml": {}, "yaml": {}, "yuck": {}, "zig": {},
}

func TestAdmissionCandidateScorecard206(t *testing.T) {
	// This scorecard loads every registered grammar, which inflates process heap
	// enough to disturb the whole-process TestArenaGCRetentionAfterRelease gate.
	// It is a deliberate diagnostic, so gate it behind an explicit opt-in and
	// keep the routine suite unaffected. Run it with:
	//   GTS_ADMISSION_SCORECARD=1 go test -tags gts_parsercorephase0 \
	//     -run TestAdmissionCandidateScorecard206 -v .
	if os.Getenv("GTS_ADMISSION_SCORECARD") != "1" {
		t.Skip("set GTS_ADMISSION_SCORECARD=1 to run the 206-language admission scorecard")
	}
	// Purge the embedded grammar cache afterward so it does not inflate process
	// heap for later suite tests.
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	entries := grammars.AllLanguages()
	rows := make([]scorecardRow, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, runAdmissionScorecardLanguage(entry))
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })

	counts := map[string]int{}
	var divergences []scorecardRow
	for _, row := range rows {
		counts[row.status]++
		if row.status == scorecardDiverge {
			divergences = append(divergences, row)
		}
	}

	t.Logf("=== Phase-3 admission candidate scorecard (%d languages) ===", len(rows))
	for _, row := range rows {
		t.Logf("%-9s %-16s %-6s %s", row.status, row.name, row.backend, row.detail)
	}
	t.Logf("--- summary: PASS=%d DIVERGE=%d FALLBACK=%d SKIP=%d ERROR=%d total=%d ---",
		counts[scorecardPass], counts[scorecardDiverge], counts[scorecardFallback],
		counts[scorecardSkip], counts[scorecardError], len(rows))

	if len(divergences) > 0 {
		for _, row := range divergences {
			t.Logf("DIVERGENCE FINDING: %s (%s) %s", row.name, row.backend, row.detail)
		}
		if os.Getenv("GTS_ADMISSION_SCORECARD_STRICT") == "1" {
			t.Fatalf("%d language(s) diverged through the compact route", len(divergences))
		}
	}

	if os.Getenv("GTS_ADMISSION_SCORECARD_RATCHET") == "1" {
		// Frozen at the generalized clean-tail admission epoch. Improvements are
		// welcome (more PASS, fewer FALLBACK); silent correctness failures,
		// registry drift, or surrendered route coverage require an explicit
		// review and ratchet update.
		//
		// minPass moved 200 -> 199 and maxFallback 1 -> 2 when
		// selectCompactAcceptanceDerivation's materiality gate
		// (parsercore_phase0_driver.go, compactAcceptanceElectionIsVacuous)
		// landed: meson's smoke sample ("message('hello')") has a genuine,
		// score-tied grammar ambiguity between two distinct real symbols
		// (variableunit and var_unit -- both present in the compiled
		// language's SymbolNames). The old positional rule happened to pick
		// the derivation that matches production; the gate cannot prove that
		// pick correct without a C oracle, so it now declines instead of
		// publishing an unproven tree, and the route falls back to
		// production (still a PASS-safe FALLBACK, never a DIVERGE -- see
		// counts[scorecardDiverge] below, unaffected at 0).
		const (
			wantTotal   = 206
			minPass     = 199
			maxFallback = 2
			wantSkip    = 5
		)
		if got := len(admissionScorecardRequiredCompactPasses); got != minPass {
			t.Fatalf("compact admission manifest has %d entries, want %d", got, minPass)
		}
		seen := make(map[string]struct{}, len(admissionScorecardRequiredCompactPasses))
		for _, row := range rows {
			if _, required := admissionScorecardRequiredCompactPasses[row.name]; !required {
				continue
			}
			seen[row.name] = struct{}{}
			if row.status != scorecardPass {
				t.Errorf("compact admission regression for %q: got %s (%s), want PASS", row.name, row.status, row.detail)
			}
		}
		for name := range admissionScorecardRequiredCompactPasses {
			if _, found := seen[name]; !found {
				t.Errorf("compact admission manifest language %q is missing from the scorecard", name)
			}
		}
		if t.Failed() {
			t.Fatal("per-language compact admission ratchet failed")
		}
		if len(rows) != wantTotal || counts[scorecardPass] < minPass ||
			counts[scorecardFallback] > maxFallback || counts[scorecardSkip] != wantSkip ||
			counts[scorecardDiverge] != 0 || counts[scorecardError] != 0 {
			t.Fatalf("admission breadth ratchet failed: PASS=%d (min %d) DIVERGE=%d FALLBACK=%d (max %d) SKIP=%d (want %d) ERROR=%d total=%d (want %d)",
				counts[scorecardPass], minPass, counts[scorecardDiverge], counts[scorecardFallback], maxFallback,
				counts[scorecardSkip], wantSkip, counts[scorecardError], len(rows), wantTotal)
		}
	}
}

func TestAdmissionCandidateNoLookaheadSmokeRatchet(t *testing.T) {
	wanted := map[string]struct{}{
		"doxygen": {},
		"jsdoc":   {},
		"vhdl":    {},
	}
	var doxygen grammars.LangEntry
	for _, entry := range grammars.AllLanguages() {
		if _, ok := wanted[entry.Name]; !ok {
			continue
		}
		if entry.Name == "doxygen" {
			doxygen = entry
		}
		row := runAdmissionScorecardLanguage(entry)
		if row.status != scorecardPass {
			t.Errorf("%s compact no-lookahead route=%s: %s", row.name, row.status, row.detail)
		}
		delete(wanted, entry.Name)
	}
	for name := range wanted {
		t.Errorf("compact no-lookahead ratchet language %q is missing", name)
	}
	if doxygen.Name == "" {
		return
	}
	row := runAdmissionScorecardSource(
		doxygen,
		[]byte("/** first */\n/** second */\n"),
	)
	if row.status != scorecardFallback ||
		!strings.Contains(row.detail, "root reduction on no-lookahead was not followed by authenticated EOF") {
		t.Errorf("doxygen mid-source no-lookahead route=%s: %s", row.status, row.detail)
	}
}

func TestAdmissionCandidateCooklangSmokeRatchet(t *testing.T) {
	const cleanSmoke = "Add @salt{1%tsp}\n"
	if got := grammars.ParseSmokeSample("cooklang"); got != cleanSmoke {
		t.Fatalf("cooklang smoke=%q, want %q", got, cleanSmoke)
	}
	var cooklang grammars.LangEntry
	for _, entry := range grammars.AllLanguages() {
		if entry.Name == "cooklang" {
			cooklang = entry
			break
		}
	}
	if cooklang.Name == "" {
		t.Fatal("cooklang is missing from the grammar registry")
	}
	if row := runAdmissionScorecardSource(cooklang, []byte(cleanSmoke)); row.status != scorecardPass {
		t.Errorf("clean cooklang compact route=%s: %s", row.status, row.detail)
	}
	if row := runAdmissionScorecardSource(cooklang, []byte("Add @salt{1%tsp}.\n")); row.status != scorecardFallback {
		t.Errorf("recovered dotted cooklang route=%s: %s", row.status, row.detail)
	}
}

func TestAdmissionScorecardLabelsProductionErrorTree(t *testing.T) {
	var goEntry grammars.LangEntry
	for _, entry := range grammars.AllLanguages() {
		if entry.Name == "go" {
			goEntry = entry
			break
		}
	}
	if goEntry.Name == "" {
		t.Fatal("go is missing from the grammar registry")
	}
	row := runAdmissionScorecardSource(goEntry, []byte("package p\nfunc"))
	if row.status != scorecardFallback || !row.productionHasError {
		t.Fatalf("invalid Go route=%s production_error_tree=%t: %s", row.status, row.productionHasError, row.detail)
	}
}

// TestAdmissionSwitchRoutePrecedenceRatchet pins the three dispatch outcomes
// that release admission relies on. A certified forest policy owns the default
// route; an explicit compact request intentionally overrides it; with neither
// feature enabled, the original production route remains the only route.
func TestAdmissionSwitchRoutePrecedenceRatchet(t *testing.T) {
	previousDefault := gts.AdmissionCandidateRouteDefault()
	previousForest := os.Getenv("GOT_GLR_FOREST") != "0"
	t.Cleanup(func() {
		gts.SetAdmissionCandidateRouteDefault(previousDefault)
		gts.SetGLRForestEnabled(previousForest)
	})

	lang := grammars.AwkLanguage()
	if !lang.AutomaticForestEnabledByDefault {
		t.Fatal("awk must retain its exact-artifact certified forest profile")
	}
	source := []byte(grammars.ParseSmokeSample("awk"))

	t.Run("default follows certified forest", func(t *testing.T) {
		gts.SetAdmissionCandidateRouteDefault(true)
		gts.SetGLRForestEnabled(true)
		gts.ResetAdmissionCandidateCountersForTest()

		parser := gts.NewParser(lang) // no per-Parser override: forest keeps precedence
		tree, err := parser.Parse(source)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		defer tree.Release()
		if routed, fallback := gts.AdmissionCandidateCounters(); routed != 0 || fallback != 0 {
			t.Fatalf("default certified-forest parse consulted compact route: routed=%d fallback=%d", routed, fallback)
		}
		if !tree.ParseRuntime().ForestFastPath {
			_, _, reason, _ := parser.ForestDeclineInfo()
			if reason == "" {
				t.Fatal("default parser bypassed both the certified forest route and its recorded decline")
			}
		}
	})

	t.Run("explicit compact override wins", func(t *testing.T) {
		gts.SetAdmissionCandidateRouteDefault(true)
		gts.SetGLRForestEnabled(true)
		gts.ResetAdmissionCandidateCountersForTest()

		parser := gts.NewParser(lang)
		parser.SetAdmissionCandidateRoute(true)
		tree, err := parser.Parse(source)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		defer tree.Release()
		if routed, fallback := gts.AdmissionCandidateCounters(); routed != 1 || fallback != 0 {
			t.Fatalf("explicit compact override did not win: routed=%d fallback=%d", routed, fallback)
		}
	})

	t.Run("neither enabled remains production", func(t *testing.T) {
		gts.SetAdmissionCandidateRouteDefault(false)
		gts.SetGLRForestEnabled(false)
		gts.ResetAdmissionCandidateCountersForTest()

		parser := gts.NewParser(lang) // no compact override and no forest master switch
		tree, err := parser.Parse(source)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		defer tree.Release()
		if routed, fallback := gts.AdmissionCandidateCounters(); routed != 0 || fallback != 0 {
			t.Fatalf("production-only parse consulted compact route: routed=%d fallback=%d", routed, fallback)
		}
		if tree.ParseRuntime().ForestFastPath {
			t.Fatal("neither-enabled parse escaped production through the forest route")
		}
	})
}

func runAdmissionScorecardLanguage(entry grammars.LangEntry) (row scorecardRow) {
	return runAdmissionScorecardSource(entry, []byte(grammars.ParseSmokeSample(entry.Name)))
}

func runAdmissionScorecardSource(entry grammars.LangEntry, source []byte) (row scorecardRow) {
	row = scorecardRow{name: entry.Name, status: scorecardError}
	defer func() {
		if r := recover(); r != nil {
			row.status = scorecardError
			row.detail = fmt.Sprintf("panic: %v", r)
		}
	}()

	lang := entry.Language()
	if lang == nil {
		row.detail = "nil language"
		return row
	}
	support := grammars.EvaluateParseSupport(entry, lang)
	row.backend = string(support.Backend)

	// Only the fresh DFA Parse path is routable through the compact candidate.
	if support.Backend != grammars.ParseBackendDFA {
		row.status = scorecardSkip
		row.detail = "not DFA-routable: " + support.Reason
		return row
	}

	production := gts.NewParser(lang)
	production.SetAdmissionCandidateRoute(false)
	productionTree, err := production.Parse(source)
	if err != nil || productionTree == nil || productionTree.RootNode() == nil {
		row.detail = fmt.Sprintf("production parse failed: %v", err)
		return row
	}
	defer productionTree.Release()
	row.productionHasError = productionTree.RootNode().HasError()
	productionInspection, err := benchfixtures.InspectGoTree(productionTree.RootNode(), lang)
	if err != nil {
		row.detail = "production digest failed: " + err.Error()
		return row
	}

	candidate := gts.NewParser(lang)
	candidate.SetAdmissionCandidateRoute(true)
	gts.ResetAdmissionCandidateCountersForTest()
	candidateTree, err := candidate.Parse(source)
	if err != nil || candidateTree == nil {
		row.detail = fmt.Sprintf("candidate parse failed: %v", err)
		return row
	}
	defer candidateTree.Release()
	routed, fallback := gts.AdmissionCandidateCounters()

	if routed == 0 {
		if fallback == 0 {
			row.status = scorecardSkip
			row.detail = "compact route was not eligible for this source"
			return row
		}
		row.status = scorecardFallback
		row.detail = gts.AdmissionCandidateLastFallbackReason()
		return row
	}
	if candidateTree.RootNode() == nil {
		row.detail = "candidate routed but produced a nil root"
		return row
	}
	candidateInspection, err := benchfixtures.InspectGoTree(candidateTree.RootNode(), lang)
	if err != nil {
		row.detail = "candidate digest failed: " + err.Error()
		return row
	}
	if candidateInspection.SHA256 == productionInspection.SHA256 {
		row.status = scorecardPass
		row.detail = "digest " + candidateInspection.SHA256[:12]
		return row
	}
	row.status = scorecardDiverge
	first := firstAdmissionTreeDivergence(candidateTree.RootNode(), productionTree.RootNode(), lang, "root")
	if first == "" {
		first = "digest mismatch without a visible node mismatch"
	}
	row.detail = fmt.Sprintf(
		"candidate=%s production=%s first=%s",
		candidateInspection.SHA256[:12],
		productionInspection.SHA256[:12],
		first,
	)
	return row
}

func firstAdmissionTreeDivergence(candidate, production *gts.Node, lang *gts.Language, path string) string {
	if candidate == nil || production == nil {
		return fmt.Sprintf("%s nil candidate=%t production=%t", path, candidate == nil, production == nil)
	}
	candidateStart, productionStart := candidate.StartPoint(), production.StartPoint()
	candidateEnd, productionEnd := candidate.EndPoint(), production.EndPoint()
	if candidate.Type(lang) != production.Type(lang) ||
		candidate.StartByte() != production.StartByte() ||
		candidate.EndByte() != production.EndByte() ||
		candidateStart != productionStart ||
		candidateEnd != productionEnd ||
		candidate.IsNamed() != production.IsNamed() ||
		candidate.IsExtra() != production.IsExtra() ||
		candidate.IsMissing() != production.IsMissing() ||
		candidate.IsError() != production.IsError() ||
		candidate.ChildCount() != production.ChildCount() {
		return fmt.Sprintf(
			"%s candidate=%s[%d:%d] children=%d flags=%t/%t/%t/%t production=%s[%d:%d] children=%d flags=%t/%t/%t/%t",
			path,
			candidate.Type(lang),
			candidate.StartByte(),
			candidate.EndByte(),
			candidate.ChildCount(),
			candidate.IsNamed(),
			candidate.IsExtra(),
			candidate.IsMissing(),
			candidate.IsError(),
			production.Type(lang),
			production.StartByte(),
			production.EndByte(),
			production.ChildCount(),
			production.IsNamed(),
			production.IsExtra(),
			production.IsMissing(),
			production.IsError(),
		)
	}
	for index := 0; index < candidate.ChildCount(); index++ {
		candidateField := candidate.FieldNameForChild(index, lang)
		productionField := production.FieldNameForChild(index, lang)
		if candidateField != productionField {
			return fmt.Sprintf(
				"%s/%d field candidate=%q production=%q",
				path,
				index,
				candidateField,
				productionField,
			)
		}
		childPath := fmt.Sprintf("%s/%s[%d]", path, candidate.Child(index).Type(lang), index)
		if diff := firstAdmissionTreeDivergence(candidate.Child(index), production.Child(index), lang, childPath); diff != "" {
			return diff
		}
	}
	if candidate.HasError() != production.HasError() {
		return fmt.Sprintf(
			"%s has_error candidate=%t production=%t",
			path,
			candidate.HasError(),
			production.HasError(),
		)
	}
	return ""
}
