//go:build !grammar_subset || grammar_subset_perl

package grammarruntime

import "testing"

// TestPerlExternalScannerSpecMatchesBlob asserts that
// perlExternalScannerSpec.Externals lines up with the shipped perl.bin
// blob's external symbols, in count and in position.
//
// This does not compare spec.Externals[i] against lang.SymbolNames[sym] by
// raw string equality: several of tree-sitter-perl's externals are declared
// with a shared grammar alias, so the blob legitimately reports the same
// display name (for example "'") at more than one external index (quote
// open/close variants), while the spec still records each one's real
// grammar.json rule name for provenance. bindExternalScannerSymbolNames
// documents the same display-vs-rule-name gap for kotlin's "?." and
// swift's 18 custom operators; it treats a name disagreement as
// diagnostic drift, not an error, for exactly this reason.
//
// What must hold, and what this test checks instead: binding is
// positional (external index i binds to spec token i), so the spec must
// declare exactly as many externals as the blob does, and
// ExternalScannerForLanguage must bind every one of them (no -1 gaps, no
// out-of-range token indexes). A spec/blob ordering drift future grammar
// bumps introduce would surface here as a length mismatch or an unbound
// external, before it could silently mis-bind a scanner token.
func TestPerlExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("perl")
	if !ok {
		t.Fatal("missing perl external scanner spec")
	}

	lang := PerlLanguage()
	if lang == nil {
		t.Fatal("PerlLanguage() returned nil")
	}

	if got, want := len(spec.Externals), len(lang.ExternalSymbols); got != want {
		t.Fatalf("perl spec has %d externals, blob has %d", got, want)
	}

	bound, ok := PerlExternalScanner{}.ExternalScannerForLanguage(lang).(PerlExternalScanner)
	if !ok {
		t.Fatal("ExternalScannerForLanguage did not return a PerlExternalScanner")
	}
	if got, want := len(bound.externalToToken), len(lang.ExternalSymbols); got != want {
		t.Fatalf("externalToToken has %d entries, want %d", got, want)
	}
	for externalIdx, tokenIdx := range bound.externalToToken {
		if tokenIdx != externalIdx {
			t.Fatalf("external %d (%s) bound to token index %d, want %d (identity binding)",
				externalIdx, spec.Externals[externalIdx], tokenIdx, externalIdx)
		}
	}
}

// TestPerlExternalScannerSpecExternalsPinned locks perlExternalScannerSpec's
// externals to tree-sitter-perl@ad74e6db's grammar.json externals array, so
// an edit to this list is deliberate and reviewable.
func TestPerlExternalScannerSpecExternalsPinned(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("perl")
	if !ok {
		t.Fatal("missing perl external scanner spec")
	}
	want := []string{
		"_single_quote",
		"_double_quote",
		"_backtick_quote",
		"_search_slash_quote",
		"_no_search_slash_plz",
		"_open_readline_bracket",
		"_open_fileglob_bracket",
		"_PERLY_SEMICOLON",
		"_PERLY_HEREDOC",
		"_ctrl_z_hack",
		"_quotelike_begin_quote",
		"_quotelike_middle_close_quote",
		"_quotelike_middle_skip",
		"_quotelike_end_zw",
		"_quotelike_end_quote",
		"_q_string_content",
		"_qq_string_content",
		"escape_sequence",
		"escaped_delimiter",
		"_dollar_in_regexp",
		"pod",
		"_gobbled_content",
		"_attribute_value_begin",
		"attribute_value",
		"prototype",
		"_signature_start",
		"_heredoc_delimiter",
		"_command_heredoc_delimiter",
		"_heredoc_start",
		"_heredoc_middle",
		"heredoc_end",
		"_fat_comma_autoquoted",
		"_filetest",
		"_brace_autoquoted_token",
		"_brace_end_zw",
		"_dollar_ident_zw",
		"_no_interp_whitespace_zw",
		"_NONASSOC",
		"_ERROR",
	}
	if len(spec.Externals) != len(want) {
		t.Fatalf("perl spec has %d externals, want %d", len(spec.Externals), len(want))
	}
	for i, name := range want {
		if spec.Externals[i] != name {
			t.Fatalf("perl spec external %d = %q, want %q", i, spec.Externals[i], name)
		}
	}
}
