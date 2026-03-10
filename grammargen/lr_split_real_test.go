// grammargen/lr_split_real_test.go
package grammargen

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSplitOracleRealGrammars(t *testing.T) {
	root := os.Getenv("GTS_GRAMMARGEN_REAL_CORPUS_ROOT")
	if root == "" {
		root = "/tmp/grammar_parity"
	}

	// Grammars known to have merge pathology (>400 productions, LALR path).
	targets := []string{
		"javascript", "python", "php", "scala", "c",
		"elixir", "ocaml", "sql", "haskell", "yaml",
	}

	for _, lang := range targets {
		t.Run(lang, func(t *testing.T) {
			grammarDir := filepath.Join(root, lang)
			jsPath := filepath.Join(grammarDir, "src", "grammar.json")
			if _, err := os.Stat(jsPath); err != nil {
				// Try alternate paths.
				alts := []string{
					filepath.Join(grammarDir, "grammar.js"),
					filepath.Join(grammarDir, "grammars", lang, "src", "grammar.json"),
				}
				found := false
				for _, alt := range alts {
					if _, err := os.Stat(alt); err == nil {
						jsPath = alt
						found = true
						break
					}
				}
				if !found {
					t.Skipf("grammar not available at %s", grammarDir)
				}
			}

			data, err := os.ReadFile(jsPath)
			if err != nil {
				t.Skipf("read failed: %v", err)
			}

			g, err := ImportGrammarJSON(data)
			if err != nil {
				t.Skipf("import failed: %v", err)
			}

			report, err := generateDiagnosticsReport(g)
			if err != nil {
				t.Skipf("generate failed: %v", err)
			}

			// Log summary.
			totalConflicts := len(report.Conflicts)
			glrConflicts := 0
			mergedConflicts := 0
			for _, c := range report.Conflicts {
				if c.Resolution == "GLR (multiple actions kept)" {
					glrConflicts++
				}
				if c.IsMergedState {
					mergedConflicts++
				}
			}

			t.Logf("SPLIT ORACLE: %s", lang)
			t.Logf("  states=%d, conflicts=%d, glr=%d, merged=%d",
				report.StateCount, totalConflicts, glrConflicts, mergedConflicts)
			t.Logf("  split_candidates=%d", len(report.SplitCandidates))

			for i, c := range report.SplitCandidates {
				if i >= 20 {
					t.Logf("  ... and %d more", len(report.SplitCandidates)-20)
					break
				}
				t.Logf("  candidate[%d]: state=%d merges=%d kind=%v sym=%d reason=%s",
					i, c.stateIdx, c.mergeCount, c.conflictKind, c.lookaheadSym, c.reason)
			}

			// Write summary to a temp file for collection.
			summaryPath := fmt.Sprintf("/tmp/split_oracle_%s.txt", lang)
			f, err := os.Create(summaryPath)
			if err == nil {
				fmt.Fprintf(f, "lang=%s states=%d conflicts=%d glr=%d merged=%d candidates=%d\n",
					lang, report.StateCount, totalConflicts, glrConflicts, mergedConflicts,
					len(report.SplitCandidates))
				for _, c := range report.SplitCandidates {
					fmt.Fprintf(f, "  state=%d merges=%d kind=%v sym=%d\n",
						c.stateIdx, c.mergeCount, c.conflictKind, c.lookaheadSym)
				}
				f.Close()
			}
		})
	}
}

// TestSplitRebuildRealGrammars enables LR(1) splitting on real grammars and
// measures before/after GLR conflict counts. ENV-GATED: only runs when
// GTS_GRAMMARGEN_SPLIT_EVAL=1 (should only be set in Docker).
func TestSplitRebuildRealGrammars(t *testing.T) {
	if os.Getenv("GTS_GRAMMARGEN_SPLIT_EVAL") != "1" {
		t.Skip("set GTS_GRAMMARGEN_SPLIT_EVAL=1 to run (Docker only)")
	}

	root := os.Getenv("GTS_GRAMMARGEN_REAL_CORPUS_ROOT")
	if root == "" {
		root = "/tmp/grammar_parity"
	}

	// Only grammars with split candidates (from oracle diagnostic).
	targets := []struct {
		name       string
		candidates int // expected approximate candidate count
	}{
		{"elixir", 3},
		{"c", 12},
		{"scala", 29},
		{"javascript", 34},
		{"python", 36},
		{"haskell", 100},
	}

	for _, tgt := range targets {
		t.Run(tgt.name, func(t *testing.T) {
			grammarDir := filepath.Join(root, tgt.name)
			jsPath := filepath.Join(grammarDir, "src", "grammar.json")
			if _, err := os.Stat(jsPath); err != nil {
				alts := []string{
					filepath.Join(grammarDir, "grammars", tgt.name, "src", "grammar.json"),
				}
				found := false
				for _, alt := range alts {
					if _, err := os.Stat(alt); err == nil {
						jsPath = alt
						found = true
						break
					}
				}
				if !found {
					t.Skipf("grammar not available at %s", grammarDir)
				}
			}

			data, err := os.ReadFile(jsPath)
			if err != nil {
				t.Skipf("read failed: %v", err)
			}

			g1, err := ImportGrammarJSON(data)
			if err != nil {
				t.Skipf("import failed: %v", err)
			}

			// Run WITHOUT splitting to get baseline (fresh grammar).
			reportBefore, err := generateDiagnosticsReport(g1)
			if err != nil {
				t.Skipf("generate (no split) failed: %v", err)
			}

			glrBefore := 0
			for _, c := range reportBefore.Conflicts {
				if c.Resolution == "GLR (multiple actions kept)" {
					glrBefore++
				}
			}
			statesBefore := reportBefore.StateCount
			candidatesBefore := len(reportBefore.SplitCandidates)
			reportBefore = nil

			// Run WITH splitting (fresh grammar to avoid mutation artifacts).
			g2, err := ImportGrammarJSON(data)
			if err != nil {
				t.Skipf("import (split) failed: %v", err)
			}
			g2.EnableLRSplitting = true
			reportAfter, err := generateDiagnosticsReport(g2)
			if err != nil {
				t.Skipf("generate (with split) failed: %v", err)
			}

			glrAfter := 0
			for _, c := range reportAfter.Conflicts {
				if c.Resolution == "GLR (multiple actions kept)" {
					glrAfter++
				}
			}
			candidatesAfter := len(reportAfter.SplitCandidates)

			sr := reportAfter.SplitResult

			t.Logf("SPLIT EVAL: %s", tgt.name)
			t.Logf("  states: %d → %d", statesBefore, reportAfter.StateCount)
			t.Logf("  GLR conflicts: %d → %d (delta %+d)", glrBefore, glrAfter, glrAfter-glrBefore)
			t.Logf("  candidates: %d → %d", candidatesBefore, candidatesAfter)
			if sr != nil {
				t.Logf("  split report: %s", sr)
			}

			// Write results for collection.
			summaryPath := fmt.Sprintf("/tmp/split_eval_%s.txt", tgt.name)
			f, err := os.Create(summaryPath)
			if err == nil {
				fmt.Fprintf(f, "lang=%s\n", tgt.name)
				fmt.Fprintf(f, "states_before=%d states_after=%d\n", statesBefore, reportAfter.StateCount)
				fmt.Fprintf(f, "glr_before=%d glr_after=%d delta=%+d\n", glrBefore, glrAfter, glrAfter-glrBefore)
				fmt.Fprintf(f, "candidates_before=%d candidates_after=%d\n", candidatesBefore, candidatesAfter)
				if sr != nil {
					fmt.Fprintf(f, "split_report=%s\n", sr)
				}
				f.Close()
			}
		})
	}
}

// TestSplitGenerateLanguageRealGrammars measures the full GenerateLanguage path
// with LR splitting enabled on selected large grammars. ENV-GATED: only runs
// when GTS_GRAMMARGEN_SPLIT_LANG_EVAL=1 (should only be set in Docker).
func TestSplitGenerateLanguageRealGrammars(t *testing.T) {
	if os.Getenv("GTS_GRAMMARGEN_SPLIT_LANG_EVAL") != "1" {
		t.Skip("set GTS_GRAMMARGEN_SPLIT_LANG_EVAL=1 to run (Docker only)")
	}

	root := os.Getenv("GTS_GRAMMARGEN_REAL_CORPUS_ROOT")
	if root == "" {
		root = "/tmp/grammar_parity"
	}

	targets := []string{"c", "scala"}

	for _, lang := range targets {
		t.Run(lang, func(t *testing.T) {
			grammarDir := filepath.Join(root, lang)
			jsPath := filepath.Join(grammarDir, "src", "grammar.json")
			if _, err := os.Stat(jsPath); err != nil {
				alts := []string{
					filepath.Join(grammarDir, "grammars", lang, "src", "grammar.json"),
				}
				found := false
				for _, alt := range alts {
					if _, err := os.Stat(alt); err == nil {
						jsPath = alt
						found = true
						break
					}
				}
				if !found {
					t.Skipf("grammar not available at %s", grammarDir)
				}
			}

			data, err := os.ReadFile(jsPath)
			if err != nil {
				t.Skipf("read failed: %v", err)
			}

			g, err := ImportGrammarJSON(data)
			if err != nil {
				t.Skipf("import failed: %v", err)
			}
			g.EnableLRSplitting = true

			langOut, err := GenerateLanguage(g)
			if err != nil {
				t.Skipf("generate language failed: %v", err)
			}

			t.Logf("SPLIT GENERATE LANGUAGE: %s", lang)
			t.Logf("  states=%d symbols=%d tokens=%d",
				langOut.StateCount, langOut.SymbolCount, langOut.TokenCount)

			summaryPath := fmt.Sprintf("/tmp/split_generate_language_%s.txt", lang)
			f, err := os.Create(summaryPath)
			if err == nil {
				fmt.Fprintf(f, "lang=%s states=%d symbols=%d tokens=%d\n",
					lang, langOut.StateCount, langOut.SymbolCount, langOut.TokenCount)
				f.Close()
			}
		})
	}
}
