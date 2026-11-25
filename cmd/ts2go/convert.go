package main

import (
	"fmt"

	"github.com/odvcencio/gotreesitter"
)

// encodeConvertedBlob is the table step that single-file mode, batch mode and
// the blob freshness check share: it builds the runtime language from the
// extracted grammar, applies the certified conflict policies, compacts the
// language when compact is not nil, and encodes the blob.
func encodeConvertedBlob(grammar *ExtractedGrammar, source []byte, compact func(*gotreesitter.Language)) ([]byte, error) {
	lang := BuildLanguage(grammar)
	if err := applyCertifiedConflictPolicyProfiles(source, grammar, lang); err != nil {
		return nil, fmt.Errorf("certified conflict policy: %w", err)
	}
	if compact != nil {
		compact(lang)
	}
	blob, err := EncodeLanguageBlob(lang)
	if err != nil {
		return nil, fmt.Errorf("encode blob: %w", err)
	}
	return blob, nil
}

// convertParserSource converts one parser.c the way batch mode does for the
// lock entry name and returns the encoded blob.
func convertParserSource(name string, source []byte) ([]byte, error) {
	grammar, err := ExtractGrammar(string(source))
	if err != nil {
		return nil, fmt.Errorf("extract: %w", err)
	}
	grammar.Name = name
	return encodeConvertedBlob(grammar, source, NewLanguageCompactor().CompactLanguage)
}
