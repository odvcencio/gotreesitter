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
// LR(1) state splitting is always attempted; a rollback guard reverts to the
// plain LALR table if splitting does not reduce GLR conflicts.
func Generate(g *Grammar) ([]byte, error) {
	report, err := generateWithReport(g, reportBuildOptions{includeLanguage: true, includeBlob: true})
	if err != nil {
		return nil, err
	}
	return report.Blob, nil
}

// GenerateLanguage compiles a Grammar into a Language struct without encoding.
// LR(1) state splitting is always attempted; a rollback guard reverts to the
// plain LALR table if splitting does not reduce GLR conflicts.
func GenerateLanguage(g *Grammar) (*gotreesitter.Language, error) {
	report, err := generateWithReport(g, reportBuildOptions{includeLanguage: true})
	if err != nil {
		return nil, err
	}
	return report.Language, nil
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
