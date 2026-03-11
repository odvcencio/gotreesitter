package grammargen

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"fmt"

	"github.com/odvcencio/gotreesitter"
)

// Generate compiles a Grammar definition into a binary blob that
// gotreesitter can load via DecodeLanguageBlob / loadEmbeddedLanguage.
func Generate(g *Grammar) ([]byte, error) {
	// Phase 1: Normalize grammar.
	ng, err := Normalize(g)
	if err != nil {
		return nil, fmt.Errorf("normalize: %w", err)
	}

	// Phase 2: Build LR(1) parse tables.
	tables, ctx, err := buildLRTablesInternal(ng, false)
	if err != nil {
		return nil, fmt.Errorf("build LR tables: %w", err)
	}

	// Phase 3: Resolve conflicts.
	if err := resolveConflicts(tables, ng); err != nil {
		return nil, fmt.Errorf("resolve conflicts: %w", err)
	}

	// Phase 3b: Add nonterminal extra parse chains.
	addNonterminalExtraChains(tables, ng, ctx)
	ctx.releaseScratch()

	// Phase 4: Compute lex modes based on parse table.
	tokenCount := ng.TokenCount()
	immediateTokens := make(map[int]bool)
	for _, t := range ng.Terminals {
		if t.Immediate {
			immediateTokens[t.SymbolID] = true
		}
	}

	keywordSet := make(map[int]bool, len(ng.KeywordSymbols))
	for _, ks := range ng.KeywordSymbols {
		keywordSet[ks] = true
	}
	stringPrefixExtensions := computeStringPrefixExtensions(ng.Terminals)

	lexModes, stateToMode := computeLexModes(
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
		immediateTokens,
		ng.ExternalSymbols,
		ng.WordSymbolID,
		keywordSet,
	)

	// Phase 5: Build lex DFA per mode.
	skipExtras := computeSkipExtras(ng)
	lexStates, lexModeOffsets, err := buildLexDFA(ng.Terminals, ng.ExtraSymbols, skipExtras, lexModes)
	if err != nil {
		return nil, fmt.Errorf("build lex DFA: %w", err)
	}

	// Phase 5b: Build keyword DFA if word token is declared.
	var keywordLexStates []gotreesitter.LexState
	if len(ng.KeywordEntries) > 0 {
		kls, _, err := buildLexDFA(ng.KeywordEntries, nil, nil, []lexModeSpec{{
			validSymbols:   allSymbolsSet(ng.KeywordEntries),
			skipWhitespace: false,
		}})
		if err != nil {
			return nil, fmt.Errorf("build keyword DFA: %w", err)
		}
		keywordLexStates = kls
	}

	// Phase 6: Assemble Language struct.
	lang, err := assemble(ng, tables, lexStates, stateToMode, lexModeOffsets)
	if err != nil {
		return nil, fmt.Errorf("assemble: %w", err)
	}
	lang.Name = g.Name

	// Set keyword fields.
	if len(keywordLexStates) > 0 {
		lang.KeywordLexStates = keywordLexStates
		lang.KeywordCaptureToken = gotreesitter.Symbol(ng.WordSymbolID)
	}

	// Phase 7: Encode to binary blob.
	blob, err := encodeLanguageBlob(lang)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}

	return blob, nil
}

// GenerateLanguage compiles a Grammar into a Language struct without encoding.
func GenerateLanguage(g *Grammar) (*gotreesitter.Language, error) {
	// LR splitting requires provenance/item-set data from the full pipeline.
	if g.EnableLRSplitting {
		report, err := generateWithReport(g, reportBuildOptions{
			includeLanguage: true,
		})
		if err != nil {
			return nil, err
		}
		return report.Language, nil
	}

	ng, err := Normalize(g)
	if err != nil {
		return nil, fmt.Errorf("normalize: %w", err)
	}

	tables, ctx, err := buildLRTablesInternal(ng, false)
	if err != nil {
		return nil, fmt.Errorf("build LR tables: %w", err)
	}

	if err := resolveConflicts(tables, ng); err != nil {
		return nil, fmt.Errorf("resolve conflicts: %w", err)
	}

	addNonterminalExtraChains(tables, ng, ctx)
	ctx.releaseScratch()

	tokenCount := ng.TokenCount()
	immediateTokens := make(map[int]bool)
	for _, t := range ng.Terminals {
		if t.Immediate {
			immediateTokens[t.SymbolID] = true
		}
	}

	keywordSet := make(map[int]bool, len(ng.KeywordSymbols))
	for _, ks := range ng.KeywordSymbols {
		keywordSet[ks] = true
	}
	stringPrefixExtensions := computeStringPrefixExtensions(ng.Terminals)

	lexModes, stateToMode := computeLexModes(
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
		immediateTokens,
		ng.ExternalSymbols,
		ng.WordSymbolID,
		keywordSet,
	)

	skipExtras := computeSkipExtras(ng)
	lexStates, lexModeOffsets, err := buildLexDFA(ng.Terminals, ng.ExtraSymbols, skipExtras, lexModes)
	if err != nil {
		return nil, fmt.Errorf("build lex DFA: %w", err)
	}

	// Build keyword DFA if word token is declared.
	var keywordLexStates []gotreesitter.LexState
	if len(ng.KeywordEntries) > 0 {
		kls, _, err := buildLexDFA(ng.KeywordEntries, nil, nil, []lexModeSpec{{
			validSymbols:   allSymbolsSet(ng.KeywordEntries),
			skipWhitespace: false,
		}})
		if err != nil {
			return nil, fmt.Errorf("build keyword DFA: %w", err)
		}
		keywordLexStates = kls
	}

	lang, err := assemble(ng, tables, lexStates, stateToMode, lexModeOffsets)
	if err != nil {
		return nil, fmt.Errorf("assemble: %w", err)
	}
	lang.Name = g.Name

	// Set keyword fields.
	if len(keywordLexStates) > 0 {
		lang.KeywordLexStates = keywordLexStates
		lang.KeywordCaptureToken = gotreesitter.Symbol(ng.WordSymbolID)
	}

	return lang, nil
}

// allSymbolsSet returns a set containing all symbol IDs from the patterns.
func allSymbolsSet(patterns []TerminalPattern) map[int]bool {
	s := make(map[int]bool, len(patterns))
	for _, p := range patterns {
		s[p.SymbolID] = true
	}
	return s
}

// computeSkipExtras returns the set of extra symbol IDs that should be
// silently consumed (Skip=true in the DFA). Only invisible/anonymous extras
// are skipped. Visible extras like `comment` produce tree nodes.
func computeSkipExtras(ng *NormalizedGrammar) map[int]bool {
	skip := make(map[int]bool)
	for _, e := range ng.ExtraSymbols {
		if e > 0 && e < len(ng.Symbols) && !ng.Symbols[e].Visible {
			skip[e] = true
		}
	}
	return skip
}

// encodeLanguageBlob serializes a Language using gob+gzip.
func encodeLanguageBlob(lang *gotreesitter.Language) ([]byte, error) {
	var out bytes.Buffer
	gzw := gzip.NewWriter(&out)
	if err := gob.NewEncoder(gzw).Encode(lang); err != nil {
		_ = gzw.Close()
		return nil, fmt.Errorf("encode language blob: %w", err)
	}
	if err := gzw.Close(); err != nil {
		return nil, fmt.Errorf("finalize language blob: %w", err)
	}
	return out.Bytes(), nil
}

// decodeLanguageBlob deserializes a gob+gzip Language blob.
func decodeLanguageBlob(data []byte) (*gotreesitter.Language, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	defer gzr.Close()

	var lang gotreesitter.Language
	if err := gob.NewDecoder(gzr).Decode(&lang); err != nil {
		return nil, fmt.Errorf("decode language blob: %w", err)
	}
	return &lang, nil
}
