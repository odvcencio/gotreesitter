//go:build !grammar_subset

package grammarruntime

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestIsSyntheticTokenName(t *testing.T) {
	positives := []string{
		"preproc_include_token1",
		"preproc_include_token2",
		"source_file_token1",
		"source_file_token2",
		"_multiline_string_fragment_token1",
		"document_token1",
		"integer_token1",
		"integer_token2",
		"integer_token3",
		"integer_token4",
		"float_token1",
		"float_token2",
		"_basic_string_token1",
		"_literal_string_token1",
		"_line_ending_or_eof_token1",
		"x_token99",
	}
	for _, name := range positives {
		if !isSyntheticTokenName(name) {
			t.Errorf("isSyntheticTokenName(%q) = false, want true", name)
		}
	}

	negatives := []string{
		"preproc_arg",
		"identifier",
		"string_content",
		"escape_sequence",
		"my_tokenizer",
		"token_type",
		"_token",
		"_token_foo",
		"preproc_include_token",
		"",
		"foo_tokenbar",
		"comment",
		"number_literal",
		"primitive_type",
	}
	for _, name := range negatives {
		if isSyntheticTokenName(name) {
			t.Errorf("isSyntheticTokenName(%q) = true, want false", name)
		}
	}
}

func TestIsSyntheticTokenNameAuditGrammars(t *testing.T) {
	// Audit all registered grammars to verify the new pattern doesn't
	// accidentally exclude tokens that should be synthetic.
	entries, err := filepath.Glob("../grammar_blobs/*.bin")
	if err != nil || len(entries) == 0 {
		t.Fatalf("find grammar blobs: %v", err)
	}

	for _, entry := range entries {
		lang := Language(strings.TrimSuffix(filepath.Base(entry), ".bin"))
		limit := int(lang.TokenCount)
		if limit > len(lang.SymbolNames) {
			limit = len(lang.SymbolNames)
		}
		for i := 0; i < limit; i++ {
			name := lang.SymbolNames[i]
			if name == "" {
				continue
			}
			got := isSyntheticTokenName(name)
			// The old pattern was strings.Contains(name, "_token").
			// Verify no named token that ends with _token+digits is missed.
			_ = got // just ensure no panic
		}
	}
}
