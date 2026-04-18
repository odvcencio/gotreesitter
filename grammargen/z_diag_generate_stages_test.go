//go:build diagnostic

package grammargen

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
)

func TestDiagGenerateStages(t *testing.T) {
	if testing.Short() {
		t.Skip("diagnostic test")
	}
	if getenvOr("DIAG_GENERATE_STAGES", "") != "1" {
		t.Skip("set DIAG_GENERATE_STAGES=1 to run per-stage generation diagnostics")
	}

	grammarName := strings.TrimSpace(getenvOr("DIAG_GRAMMAR", "cpp"))
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

	t.Logf("diag-generate: grammar=%s timeout=%s rules=%d extras=%d conflicts=%d externals=%d word=%q lr_split=%v broad_lex=%v",
		grammarName,
		timeout,
		len(gram.Rules),
		len(gram.Extras),
		len(gram.Conflicts),
		len(gram.Externals),
		gram.Word,
		gram.EnableLRSplitting,
		useForcedBroadLexFallback(),
	)
	t.Logf("diag-generate: mem[start]=%s", diagGenerateMemSnapshot())

	bgCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	totalStart := time.Now()

	stageStart := time.Now()
	warnings := Validate(gram)
	t.Logf("stage=validate dur=%s warnings=%d mem=%s", time.Since(stageStart), len(warnings), diagGenerateMemSnapshot())

	stageStart = time.Now()
	ng, err := Normalize(gram)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	t.Logf("stage=normalize dur=%s symbols=%d terminals=%d keywords=%d productions=%d tokens=%d extra_symbols=%d externals=%d mem=%s",
		time.Since(stageStart),
		len(ng.Symbols),
		len(ng.Terminals),
		len(ng.KeywordEntries),
		len(ng.Productions),
		ng.TokenCount(),
		len(ng.ExtraSymbols),
		len(ng.ExternalSymbols),
		diagGenerateMemSnapshot(),
	)

	stageStart = time.Now()
	tables, lrCtx, err := buildLRTablesWithProvenanceCtx(bgCtx, ng)
	if err != nil {
		t.Fatalf("build LR tables: %v", err)
	}
	mergedStates := 0
	if lrCtx.provenance != nil {
		mergedStates = lrCtx.provenance.mergedStateCount()
	}
	t.Logf("stage=build_lr dur=%s states=%d item_sets=%d merged_states=%d mem=%s",
		time.Since(stageStart),
		tables.StateCount,
		len(lrCtx.itemSets),
		mergedStates,
		diagGenerateMemSnapshot(),
	)
	defer lrCtx.releaseScratch()

	stageStart = time.Now()
	diags, err := resolveConflictsWithDiag(tables, ng, lrCtx.provenance)
	if err != nil {
		t.Fatalf("resolve conflicts: %v", err)
	}
	glrConflicts := 0
	for _, d := range diags {
		if d.Resolution == "GLR (multiple actions kept)" {
			glrConflicts++
		}
	}
	t.Logf("stage=resolve_conflicts dur=%s conflicts=%d glr_conflicts=%d mem=%s",
		time.Since(stageStart),
		len(diags),
		glrConflicts,
		diagGenerateMemSnapshot(),
	)

	stageStart = time.Now()
	splitCandidates := newSplitOracle(diags, lrCtx.provenance, tables, ng).candidates()
	t.Logf("stage=split_candidates dur=%s candidates=%d lr_split_enabled=%v mem=%s",
		time.Since(stageStart),
		len(splitCandidates),
		gram.EnableLRSplitting,
		diagGenerateMemSnapshot(),
	)

	stageStart = time.Now()
	addNonterminalExtraChains(tables, ng, lrCtx)
	t.Logf("stage=extra_chains dur=%s extra_chain_state_start=%d mem=%s",
		time.Since(stageStart),
		tables.ExtraChainStateStart,
		diagGenerateMemSnapshot(),
	)

	stageStart = time.Now()
	tokenCount := ng.TokenCount()
	immediateTokens := make(map[int]bool)
	for _, term := range ng.Terminals {
		if term.Immediate {
			immediateTokens[term.SymbolID] = true
		}
	}
	keywordSet := make(map[int]bool, len(ng.KeywordSymbols))
	for _, ks := range ng.KeywordSymbols {
		keywordSet[ks] = true
	}
	stringPrefixExtensions := computeStringPrefixExtensions(ng.Terminals)
	termPatSyms := terminalPatternSymSet(ng)

	var lexModes []lexModeSpec
	var stateToMode []int
	var afterWSModes []afterWSModeEntry
	if useForcedBroadLexFallback() {
		allSyms := make(map[int]bool)
		for _, term := range ng.Terminals {
			allSyms[term.SymbolID] = true
		}
		for _, sym := range ng.ExtraSymbols {
			if sym > 0 && sym < tokenCount {
				allSyms[sym] = true
			}
		}
		lexModes = []lexModeSpec{{validSymbols: allSyms, skipWhitespace: true}}
		stateToMode = make([]int, tables.StateCount)
	} else {
		lexModes, stateToMode, afterWSModes = computeLexModes(
			tables.StateCount,
			tokenCount,
			func(state, sym int) bool {
				if acts, ok := tables.ActionTable[state]; ok {
					if entry, ok := acts[sym]; ok && len(entry) > 0 {
						return true
					}
				}
				return false
			},
			stringPrefixExtensions,
			ng.ExtraSymbols,
			tables.ExtraChainStateStart,
			immediateTokens,
			ng.ExternalSymbols,
			ng.WordSymbolID,
			keywordSet,
			termPatSyms,
			buildFollowTokensFunc(tables, tokenCount),
			patternImmediateTokenSet(ng),
		)
	}
	t.Logf("stage=compute_lex_modes dur=%s modes=%d state_to_mode=%d after_ws_modes=%d mem=%s",
		time.Since(stageStart),
		len(lexModes),
		len(stateToMode),
		len(afterWSModes),
		diagGenerateMemSnapshot(),
	)

	stageStart = time.Now()
	skipExtras := computeSkipExtras(ng)
	lexStates, lexModeOffsets, err := buildLexDFA(bgCtx, ng.Terminals, ng.ExtraSymbols, skipExtras, lexModes)
	if err != nil {
		t.Fatalf("build lex DFA: %v", err)
	}
	t.Logf("stage=build_lex_dfa dur=%s lex_states=%d lex_mode_offsets=%d mem=%s",
		time.Since(stageStart),
		len(lexStates),
		len(lexModeOffsets),
		diagGenerateMemSnapshot(),
	)

	var keywordLexStates []gotreesitter.LexState
	if len(ng.KeywordEntries) > 0 {
		stageStart = time.Now()
		keywordLexStates, _, err = buildLexDFA(bgCtx, ng.KeywordEntries, nil, nil, []lexModeSpec{{
			validSymbols:   allSymbolsSet(ng.KeywordEntries),
			skipWhitespace: false,
		}})
		if err != nil {
			t.Fatalf("build keyword DFA: %v", err)
		}
		t.Logf("stage=build_keyword_dfa dur=%s keyword_lex_states=%d mem=%s",
			time.Since(stageStart),
			len(keywordLexStates),
			diagGenerateMemSnapshot(),
		)
	}

	stageStart = time.Now()
	lang, err := assemble(ng, tables, lexStates, stateToMode, lexModeOffsets)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	lang.Name = gram.Name
	t.Logf("stage=assemble dur=%s symbol_count=%d token_count=%d state_count=%d lex_states=%d lex_modes=%d mem=%s",
		time.Since(stageStart),
		lang.SymbolCount,
		lang.TokenCount,
		lang.StateCount,
		len(lang.LexStates),
		len(lang.LexModes),
		diagGenerateMemSnapshot(),
	)

	t.Logf("stage=total dur=%s final_mem=%s", time.Since(totalStart), diagGenerateMemSnapshot())
}

func lookupImportParityGrammar(name string) (importParityGrammar, bool) {
	for _, pg := range importParityGrammars {
		if pg.name == name {
			return pg, true
		}
	}
	return importParityGrammar{}, false
}

func diagGenerateMemSnapshot() string {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return fmt.Sprintf(
		"alloc=%.1fMi heap_alloc=%.1fMi heap_inuse=%.1fMi sys=%.1fMi objs=%d gc=%d",
		float64(ms.Alloc)/(1024*1024),
		float64(ms.HeapAlloc)/(1024*1024),
		float64(ms.HeapInuse)/(1024*1024),
		float64(ms.Sys)/(1024*1024),
		ms.HeapObjects,
		ms.NumGC,
	)
}
