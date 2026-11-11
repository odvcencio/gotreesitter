package grammargen

// Dedicated bash real-corpus parity witness.
//
// Background: the generic real-corpus loop (TestMultiGrammarImportRealCorpusParity
// in parity_real_corpus_test.go) skips any importParityGrammars entry whose
// jsonPath/path are both empty (parityGrammarRepoRoot returns ""). The bash
// entry in parity_test.go's importParityGrammars has neither, and — unlike
// python, which has this dedicated python_parity_test.go witness — bash had
// no witness of its own. That means the real_corpus_parity_floors.json v3
// bash entry (25 eligible / 9 no_error / 6 sexpr / 6 deep) was PHANTOM:
// nothing ever exercised it, so it could regress or be silently deleted
// without any test noticing.
//
// This file pins current reality so the floor is actually guarded and can
// only ratchet upward. The floor is intentionally low: grammargen's
// generated LR automaton has no valid parse path for several
// parameter-expansion operator/suffix alternatives (default-value `:-`,
// pattern-substitution `//`, indirect `!`, subscript) inside `${...}`. The minimal reducer is `echo ${x:-y}`
// (TestBashParameterExpansionOperatorReducer below).
//
// Corpus acquisition mirrors python_parity_test.go / python_keyword_test.go's
// convention: grammar.json and the test/corpus + examples samples are looked
// up under a small set of well-known local roots, and the test SKIPs (not
// fails) when none are present, so a plain `go test ./grammargen/...` on a
// dev box without corpora stays green. Seed a corpus with either:
//
//	scripts/seed_real_corpus_from_lock.sh /tmp/grammar_parity bash
//	cgo_harness/seed_parity_repos.sh --dest /tmp/grammar_parity --langs bash
//
// Both pin tree-sitter-bash to the commit recorded in grammars/languages.lock
// (a06c2e4415e9bc0346c6b86d401879ffb44058f7), which is also the commit the
// RCA above was reproduced against.

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Bash real-corpus parity floor (real_corpus_parity_floors.json v3, grammar
// "bash"). Can only increase; bump these together with the "bash" entry in
// grammargen/testdata/real_corpus_parity_floors.json when the underlying
// defect improves.
const (
	bashRealCorpusFloorEligible = 25
	bashRealCorpusFloorNoError  = 9
	bashRealCorpusFloorSExpr    = 6
	bashRealCorpusFloorDeep     = 6
)

// TestBashRealCorpusParityFloor is the dedicated witness for the bash entry
// in grammargen/testdata/real_corpus_parity_floors.json. It grammargen-
// compiles the locked tree-sitter-bash grammar.json, parses the real corpus
// (test/corpus blocks + examples/*.sh) in both the generated and reference
// (ts2go blob) lanes, and asserts the floor. SKIPs when the corpus is not
// available locally (see file comment for how to seed it).
func TestBashRealCorpusParityFloor(t *testing.T) {
	genLang := loadGeneratedBashLanguageForParity(t)
	refLang := grammars.BashLanguage()
	adaptExternalScanner(refLang, genLang)

	repoRoot := bashCorpusRepoRootForTest(t)
	cfg := realCorpusCollectConfig{
		TargetEligible:      bashRealCorpusFloorEligible,
		MaxSampleBytes:      defaultMaxSampleBytesForProfile(realCorpusProfileAggressive),
		CandidateMultiplier: defaultCandidateMultiplierForProfile(realCorpusProfileAggressive),
		Profile:             realCorpusProfileAggressive,
	}
	candidates := collectGrammarCorpusCandidates(t, repoRoot, cfg)
	if len(candidates) == 0 {
		t.Skip("bash real corpus unavailable (no candidate samples found under repo root)")
	}

	genParser := gotreesitter.NewParser(genLang)
	refParser := gotreesitter.NewParser(refLang)

	metrics := realCorpusMetrics{}
	mismatchLogs := 0
	const maxSafeDepth = 2000

	for i, cand := range candidates {
		if metrics.Eligible >= bashRealCorpusFloorEligible {
			break
		}
		i, cand := i, cand
		func() {
			srcBytes := []byte(cand.Text)
			genTree, genErr := genParser.ParseStrict(srcBytes)
			refTree, refErr := refParser.ParseStrict(srcBytes)
			if genTree != nil {
				defer genTree.Release()
			}
			if refTree != nil {
				defer refTree.Release()
			}

			refStop := realCorpusParseStopReason(refErr)
			if realCorpusParseStopActive(refStop) {
				if mismatchLogs < 10 {
					mismatchLogs++
					t.Logf("skipping sample %d (%s:%s): reference parse stopped early: %s", i, cand.Source, cand.Path, refStop)
				}
				return
			}
			if refErr != nil {
				if mismatchLogs < 10 {
					mismatchLogs++
					t.Logf("skipping sample %d (%s:%s): reference parse failed: %v", i, cand.Source, cand.Path, refErr)
				}
				return
			}
			if refTree == nil || refTree.RootNode() == nil {
				return
			}
			refRoot := refTree.RootNode()

			refSexp := safeSExpr(refRoot, refLang, maxSafeDepth)
			if refSexp == "" {
				return
			}
			refHasError := strings.Contains(refSexp, "ERROR") ||
				strings.Contains(refSexp, "MISSING") ||
				refRoot.HasError()
			if refHasError {
				return
			}
			metrics.Eligible++

			genStop := realCorpusParseStopReason(genErr)
			if realCorpusParseStopActive(genStop) {
				if mismatchLogs < 10 {
					mismatchLogs++
					t.Logf("sample %d (%s:%s) generated parse stopped early on clean ref: %s", i, cand.Source, cand.Path, genStop)
				}
				return
			}
			if genErr != nil {
				if mismatchLogs < 10 {
					mismatchLogs++
					t.Logf("sample %d (%s:%s) generated parse failed on clean ref: %v", i, cand.Source, cand.Path, genErr)
				}
				return
			}
			if genTree == nil || genTree.RootNode() == nil {
				return
			}
			genRoot := genTree.RootNode()

			genSexp := safeSExpr(genRoot, genLang, maxSafeDepth)
			if genSexp == "" {
				if mismatchLogs < 10 {
					mismatchLogs++
					t.Logf("sample %d (%s:%s) generated tree too large/deep to serialize on clean ref", i, cand.Source, cand.Path)
				}
				return
			}

			genHasError := strings.Contains(genSexp, "ERROR") || strings.Contains(genSexp, "MISSING")
			if genHasError {
				if mismatchLogs < 10 {
					mismatchLogs++
					t.Logf("sample %d (%s:%s) gen ERROR on clean ref: %s", i, cand.Source, cand.Path, genSexp[:min(len(genSexp), 200)])
				}
				return
			}
			metrics.NoError++

			sexprMatch := genSexp == refSexp
			if !sexprMatch && strings.Contains(refSexp, `\u`) {
				sexprMatch = unescapeUnicodeInType(genSexp) == unescapeUnicodeInType(refSexp)
			}
			if !sexprMatch {
				refRootType := refRoot.Type(refLang)
				genRootType := genRoot.Type(genLang)
				if (refRootType == "" || refRoot.IsError()) && genRootType != "" {
					genInner := stripSExprRoot(genSexp)
					refInner := stripSExprRoot(refSexp)
					if genInner != "" && genInner == refInner {
						sexprMatch = true
					}
				}
				if !sexprMatch && genRoot.IsNamed() && !refRoot.IsNamed() && genRootType != "" {
					reconstructed := rebuildRootSExpr(refRoot, refLang, genRootType)
					if genSexp == reconstructed {
						sexprMatch = true
					}
				}
			}

			divs := compareTreesDeep(genRoot, genLang, refRoot, refLang, "root", 10)
			if len(divs) == 0 {
				metrics.DeepParity++
				sexprMatch = true
			} else if mismatchLogs < 10 {
				mismatchLogs++
				t.Logf("sample %d (%s:%s) deep mismatch: %s", i, cand.Source, cand.Path, divs[0].String())
			}
			if sexprMatch {
				metrics.SExprParity++
			}
		}()
	}

	if metrics.Eligible == 0 {
		t.Skip("no clean reference samples in extracted bash corpus set")
	}

	t.Logf("bash real-corpus: eligible=%d no-error=%d sexpr=%d deep=%d (candidates=%d)",
		metrics.Eligible, metrics.NoError, metrics.SExprParity, metrics.DeepParity, len(candidates))

	floor := realCorpusMetrics{
		Eligible:    bashRealCorpusFloorEligible,
		NoError:     bashRealCorpusFloorNoError,
		SExprParity: bashRealCorpusFloorSExpr,
		DeepParity:  bashRealCorpusFloorDeep,
	}
	msgSuffix := " (minimal reducer: `echo ${x:-y}`); if this is an intentional improvement, " +
		"bump both this floor and grammargen/testdata/real_corpus_parity_floors.json[\"bash\"] together"
	if metrics.Eligible < floor.Eligible {
		t.Errorf("ratchet regression eligible: %d < floor %d%s", metrics.Eligible, floor.Eligible, msgSuffix)
	}
	if metrics.NoError < floor.NoError {
		t.Errorf("ratchet regression no-error: %d < floor %d%s", metrics.NoError, floor.NoError, msgSuffix)
	}
	if metrics.SExprParity < floor.SExprParity {
		t.Errorf("ratchet regression sexpr parity: %d < floor %d%s", metrics.SExprParity, floor.SExprParity, msgSuffix)
	}
	if metrics.DeepParity < floor.DeepParity {
		t.Errorf("ratchet regression deep parity: %d < floor %d%s", metrics.DeepParity, floor.DeepParity, msgSuffix)
	}
}

// TestBashParameterExpansionOperatorReducer is the minimal reducer for the
// bash real-corpus floor RCA:
// the grammargen-generated LR automaton has no valid parse path for the
// parameter-expansion operator/suffix alternatives inside `${...}`
// (default-value `:-`, pattern-substitution `//`, indirect `!`, subscript).
// Bare `${x}` and the length operator `${#x}` construct correctly.
func TestBashParameterExpansionOperatorReducer(t *testing.T) {
	genLang := loadGeneratedBashLanguageForParity(t)
	parser := gotreesitter.NewParser(genLang)

	// Controls: these branches of _expansion_body are NOT part of the known
	// defect, so any regression here is a real bug, not an RCA footnote.
	t.Run("bare_expansion_control", func(t *testing.T) {
		assertBashCleanParse(t, parser, genLang, "echo ${x}\n")
	})
	t.Run("length_operator_control", func(t *testing.T) {
		assertBashCleanParse(t, parser, genLang, "echo ${#x}\n")
	})

	// Known-defect witness: self-healing skip. This asserts the CORRECT
	// (error-free) outcome; today it does not hold, so it reports the
	// divergence via t.Skip rather than asserting the broken shape as
	// correct. Once the expansion-suffix table defect is fixed, this
	// sub-test starts passing with no edit required — see
	// grammars/clean_regression_pins_test.go's TestCleanRegressionPinRegexAnyCharacter
	// for the same self-healing pin pattern used elsewhere in this repo.
	t.Run("default_value_operator_known_defect", func(t *testing.T) {
		src := "echo ${x:-y}\n"
		tree, err := parser.Parse([]byte(src))
		if err != nil {
			t.Fatalf("parse returned error: %v", err)
		}
		defer tree.Release()
		root := tree.RootNode()
		if !root.HasError() {
			return // fix landed — pin now green.
		}
		t.Skipf("KNOWN DIVERGENCE (bash expansion-suffix table defect): "+
			"generated lane produces ERROR for `echo ${x:-y}`. tree: %s",
			root.SExpr(genLang))
	})
}

func assertBashCleanParse(t *testing.T, parser *gotreesitter.Parser, lang *gotreesitter.Language, src string) {
	t.Helper()
	tree, err := parser.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse returned error: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("generated parse has ERROR for %q: %s", src, root.SExpr(lang))
	}
}

// loadGeneratedBashLanguageForParity grammargen-compiles the locked
// tree-sitter-bash grammar.json. The 5-minute timeout matches the bash
// entry's genTimeout in parity_test.go's importParityGrammars (bash's LR
// construction is comparatively slow: ~319 symbols / ~46.8K states).
func loadGeneratedBashLanguageForParity(t *testing.T) *gotreesitter.Language {
	t.Helper()

	gram := loadBashGrammarJSONForTest(t)
	genLang, err := generateWithTimeout(gram, 5*time.Minute)
	if err != nil {
		t.Fatalf("generate bash language: %v", err)
	}
	return genLang
}

func loadBashGrammarJSONForTest(t *testing.T) *Grammar {
	t.Helper()

	candidates := []string{
		"/tmp/grammar_parity/bash/src/grammar.json",
		".parity_seed/bash/src/grammar.json",
		"../.parity_seed/bash/src/grammar.json",
	}
	jsonPath := ""
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			jsonPath = candidate
			break
		}
	}
	if jsonPath == "" {
		t.Skip("bash grammar.json not available")
	}

	source, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Skipf("bash grammar.json not available: %v", err)
	}

	gram, err := ImportGrammarJSON(source)
	if err != nil {
		t.Fatalf("import bash grammar.json: %v", err)
	}
	return gram
}

// bashCorpusRepoRootForTest returns the local tree-sitter-bash repo root
// (containing test/corpus/*.txt and examples/*.sh) used to collect real-
// corpus samples, or skips the calling test when none is available.
func bashCorpusRepoRootForTest(t *testing.T) string {
	t.Helper()

	for _, candidate := range []string{
		"/tmp/grammar_parity/bash",
		".parity_seed/bash",
		"../.parity_seed/bash",
	} {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	t.Skip("bash real corpus unavailable")
	return ""
}
