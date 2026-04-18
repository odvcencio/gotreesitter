//go:build diagnostic

package grammargen

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDiagGenerateLanguageFastPath(t *testing.T) {
	if testing.Short() {
		t.Skip("diagnostic test")
	}
	if getenvOr("DIAG_GENERATE_FASTPATH", "") != "1" {
		t.Skip("set DIAG_GENERATE_FASTPATH=1 to run plain GenerateLanguage diagnostics")
	}

	grammarName := strings.TrimSpace(getenvOr("DIAG_GRAMMAR", "fortran"))
	pg, ok := lookupImportParityGrammar(grammarName)
	if !ok {
		t.Fatalf("%s not found in importParityGrammars", grammarName)
	}

	gram, err := importParityGrammarSource(pg)
	if err != nil {
		t.Fatalf("import %s: %v", grammarName, err)
	}

	if getenvOr("DIAG_ENABLE_LR_SPLIT", "") == "1" {
		gram.EnableLRSplitting = true
	}

	timeout := pg.genTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	if raw := strings.TrimSpace(os.Getenv("DIAG_GENERATE_TIMEOUT")); raw != "" {
		override, err := time.ParseDuration(raw)
		if err != nil {
			t.Fatalf("parse DIAG_GENERATE_TIMEOUT=%q: %v", raw, err)
		}
		timeout = override
	}

	t.Logf("diag-generate-fastpath: grammar=%s timeout=%s rules=%d extras=%d conflicts=%d externals=%d word=%q lr_split=%v",
		grammarName,
		timeout,
		len(gram.Rules),
		len(gram.Extras),
		len(gram.Conflicts),
		len(gram.Externals),
		gram.Word,
		gram.EnableLRSplitting,
	)
	t.Logf("diag-generate-fastpath: mem[start]=%s", diagGenerateMemSnapshot())

	bgCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	lang, err := GenerateLanguageWithContext(bgCtx, gram)
	if err != nil {
		t.Fatalf("GenerateLanguageWithContext: %v", err)
	}

	t.Logf("stage=generate_language dur=%s symbol_count=%d token_count=%d state_count=%d parse_actions=%d lex_states=%d lex_modes=%d mem=%s",
		time.Since(start),
		lang.SymbolCount,
		lang.TokenCount,
		lang.StateCount,
		len(lang.ParseActions),
		len(lang.LexStates),
		len(lang.LexModes),
		diagGenerateMemSnapshot(),
	)
}
