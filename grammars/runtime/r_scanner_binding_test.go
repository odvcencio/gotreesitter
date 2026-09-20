//go:build !grammar_subset || grammar_subset_r

package grammarruntime

import (
	"slices"
	"testing"
)

// TestRExternalScannerSpecMatchesBlob pins rExternalScannerSpec's Externals
// list -- the binding source for RExternalScanner.ExternalScannerForLanguage
// -- against the shipped r.bin's actual external symbol count and order.
//
// Externals[i] is the upstream grammar.json rule name for external index i,
// while lang.SymbolNames[lang.ExternalSymbols[i]] is the Language's display
// name for that same index; these differ for aliased tokens (for example
// "_raw_string_open" displays as "string_open", "_external_else" displays as
// "else") the same way kotlin's "safe_nav" displays as "\?." and several
// swift externals display as their literal text. That is expected: binding
// is positional (index i binds to token i), not by name, so this test checks
// order and count, not a literal name match, and records any mismatch as a
// documented alias rather than an error.
func TestRExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("r")
	if !ok {
		t.Fatal("missing r external scanner spec")
	}
	wantExternals := []string{
		"_start",
		"_newline",
		"_semicolon",
		"_raw_string_open",
		"_raw_string_content",
		"_raw_string_close",
		"_external_else",
		"_external_open_parenthesis",
		"_external_close_parenthesis",
		"_external_open_brace",
		"_external_close_brace",
		"_external_open_bracket",
		"_external_close_bracket",
		"_external_open_bracket2",
		"_external_close_bracket2",
		"_error_sentinel",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("r spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "58a22794466c0fc15b0d3b40531db751593721e8"; got != want {
		t.Fatalf("r spec upstream commit = %q, want %q", got, want)
	}

	lang := RLanguage()
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("r blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	// The known upstream aliases: rule name -> display name recorded on the
	// shipped blob, in externals order. Indices not listed here must match
	// literally (no known alias).
	knownAliases := map[int]string{
		3:  "string_open",
		4:  "string_content",
		5:  "string_close",
		6:  "else",
		7:  "(",
		8:  ")",
		9:  "{",
		10: "}",
		11: "[",
		12: "]",
		13: "[[",
		14: "]]",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("r external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		want := spec.Externals[i]
		if alias, ok := knownAliases[i]; ok {
			want = alias
		}
		if display != want {
			t.Fatalf("r external index %d: blob display name = %q, want %q (spec rule name %q)", i, display, want, spec.Externals[i])
		}
	}
}
