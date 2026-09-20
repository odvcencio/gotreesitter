//go:build !grammar_subset

package grammarruntime

import (
	"slices"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// This file closes the load-time-binding side of the ocaml-class regression
// guard (see scanner_symbol_guard_test.go for the hardcoded-scanner side).
//
// Eight load-bound languages already carry a dedicated positional-binding pin
// test that proves their spec order against the shipped blob: kotlin, swift,
// dart, rust, javascript, typescript, and tsx in
// external_scanner_positional_binding_test.go, plus hcl in
// hcl_scanner_test.go. c_sharp and scala had no such pin before this file, so
// TestCSharpExternalScannerBindsPositionally and
// TestScalaExternalScannerBindsPositionally add it here, following the same
// pattern: assert externalToToken is the identity map (every external binds
// to its own token, so an upstream externals-count change is caught
// immediately) and assert the bound symbols equal an independently recorded
// table for the currently-shipped blob (so a silent same-length reorder is
// caught too).
//
// python's spec externals match the blob's display names exactly on the
// current blob (zero drift), so TestPythonExternalScannerSpecOrderMatchesBlob
// asserts that directly instead of adding a pin table.
//
// sql does not use bindExternalScannerSpec at all: it resolves symbols
// dynamically from lang.ExternalSymbols at Scan time (see sql_scanner.go),
// so it is structurally immune to absolute-ID drift regardless of spec
// order. TestSqlExternalScannerSpecOrderIsInformational only logs its
// display-name drift for visibility.

func TestCSharpExternalScannerBindsPositionally(t *testing.T) {
	lang := Language("c_sharp")
	scanner, ok := CSharpExternalScanner{}.ExternalScannerForLanguage(lang).(CSharpExternalScanner)
	if !ok {
		t.Fatalf("CSharpExternalScanner binding type = %T, want CSharpExternalScanner", CSharpExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, csTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("c_sharp externalToToken = %v, want %v (all %d externals must bind)", got, want, csTokenCount)
	}
	if got, want := scanner.symbols, csDefaultSymTable; got != want {
		t.Fatalf("c_sharp post-bind symbols = %v, want default table %v", got, want)
	}
}

// scaCurrentSymTable records the scala external symbols bound from the
// currently-shipped scala.bin, in scaTok* order. scala's own scaDefaultSymTable
// is an all-zero placeholder (no production path ever observes it), so this
// table is the only independently recorded value for the identity check
// below to compare against: a future scala.bin that reorders the externals
// array without changing the count would still pass the externalToToken
// check (index-based) but would fail this comparison.
var scaCurrentSymTable = [scaTokenCount]gotreesitter.Symbol{
	140, 141, 142, 143, 144, 145, 146, 147, 148, 149,
	150, 151, 152, 153, 72, 75, 76, 55, 56, 46,
	154, 155, 156, 157, 158, 159, 160, 161, 162, 163,
	164, 165, 166, 167, 168, 169, 170, 171, 172, 173,
	57, 96, 97, 98, 99, 100, 101, 102, 103, 104,
	105, 174, 175, 176, 177,
}

func TestScalaExternalScannerBindsPositionally(t *testing.T) {
	lang := Language("scala")
	scanner, ok := ScalaExternalScanner{}.ExternalScannerForLanguage(lang).(ScalaExternalScanner)
	if !ok {
		t.Fatalf("ScalaExternalScanner binding type = %T, want ScalaExternalScanner", ScalaExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, scaTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("scala externalToToken = %v, want %v (all %d externals must bind)", got, want, scaTokenCount)
	}
	if got, want := scanner.symbols, scaCurrentSymTable; got != want {
		t.Fatalf("scala post-bind symbols = %v, want recorded table %v (a reordered or renumbered externals array would land here)", got, want)
	}
}

// erlangCurrentSymTable records the erlang external symbols bound from the
// currently-shipped erlang.bin, in erlangTok* order. It is the independently
// recorded value TestErlangExternalScannerBindsPositionally compares a real
// bind against, so a future erlang.bin that reorders the externals array
// without changing the count still fails this comparison even though the
// identity externalToToken check would pass.
var erlangCurrentSymTable = [erlangTokenCount]gotreesitter.Symbol{
	145, // _tq_string
	146, // _tq_sigil_string
	147, // error_sentinel
}

func TestErlangExternalScannerBindsPositionally(t *testing.T) {
	lang := Language("erlang")
	scanner, ok := ErlangExternalScanner{}.ExternalScannerForLanguage(lang).(ErlangExternalScanner)
	if !ok {
		t.Fatalf("ErlangExternalScanner binding type = %T, want ErlangExternalScanner", ErlangExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, erlangTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("erlang externalToToken = %v, want %v (all %d externals must bind)", got, want, erlangTokenCount)
	}
	if got, want := scanner.symbols, erlangCurrentSymTable; got != want {
		t.Fatalf("erlang post-bind symbols = %v, want recorded table %v (a reordered or renumbered externals array would land here)", got, want)
	}
}

func TestPythonExternalScannerSpecOrderMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("python")
	if !ok {
		t.Fatal("no ExternalScannerSpec registered for python")
	}
	lang := Language("python")
	if len(spec.Externals) != len(lang.ExternalSymbols) {
		t.Fatalf("python spec declares %d externals but the blob exposes %d; external count drift is not benign", len(spec.Externals), len(lang.ExternalSymbols))
	}
	for i, want := range spec.Externals {
		sym := lang.ExternalSymbols[i]
		got := ""
		if int(sym) < len(lang.SymbolNames) {
			got = lang.SymbolNames[sym]
		}
		if got != want {
			t.Errorf("python external[%d]: spec name %q, blob display name %q; python carries no known benign display-name drift, so this indicates real external reordering", i, want, got)
		}
	}
}

// TestPowershellExternalScannerSpecMatchesBlob pins
// powershellExternalScannerSpec's Externals list -- the binding source for
// PowershellExternalScanner.ExternalScannerForLanguage -- against the
// shipped powershell.bin's actual external symbol count and order.
func TestPowershellExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("powershell")
	if !ok {
		t.Fatal("missing powershell external scanner spec")
	}
	wantExternals := []string{
		"_statement_terminator",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("powershell spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "e7bd348c49fdfd5c853a146a670965ba516a6239"; got != want {
		t.Fatalf("powershell spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("powershell")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("powershell blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := PowershellExternalScanner{}.ExternalScannerForLanguage(lang).(PowershellExternalScanner)
	if !ok {
		t.Fatalf("PowershellExternalScanner binding type = %T, want PowershellExternalScanner", PowershellExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, powershellTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("powershell externalToToken = %v, want %v (all %d externals must bind)", got, want, powershellTokenCount)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("powershell external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("powershell external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

func TestSqlExternalScannerSpecOrderIsInformational(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("sql")
	if !ok {
		t.Fatal("no ExternalScannerSpec registered for sql")
	}
	lang := Language("sql")
	n := min(len(spec.Externals), len(lang.ExternalSymbols))
	mismatches := 0
	for i := 0; i < n; i++ {
		sym := lang.ExternalSymbols[i]
		actual := ""
		if int(sym) < len(lang.SymbolNames) {
			actual = lang.SymbolNames[sym]
		}
		if actual != spec.Externals[i] {
			mismatches++
			t.Logf("sql external[%d]: spec name %q, blob display name %q (informational: sql resolves symbols from lang.ExternalSymbols at Scan time, so display-name drift here is not a correctness risk)", i, spec.Externals[i], actual)
		}
	}
	if len(spec.Externals) != len(lang.ExternalSymbols) {
		t.Errorf("sql spec declares %d externals but the blob exposes %d; sql's sqlTok* indexes assume the two counts match", len(spec.Externals), len(lang.ExternalSymbols))
	}
	t.Logf("sql: %d/%d external display names differ from the spec's rule names", mismatches, n)
}

func TestBeancountExternalScannerBindsPositionally(t *testing.T) {
	lang := Language("beancount")
	scanner, ok := BeancountExternalScanner{}.ExternalScannerForLanguage(lang).(BeancountExternalScanner)
	if !ok {
		t.Fatalf("BeancountExternalScanner binding type = %T, want BeancountExternalScanner", BeancountExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, beancountTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("beancount externalToToken = %v, want %v (all %d externals must bind)", got, want, beancountTokenCount)
	}
	if got, want := scanner.symbols, beancountDefaultSymTable; got != want {
		t.Fatalf("beancount post-bind symbols = %v, want default table %v", got, want)
	}
}

// TestCaddyExternalScannerSpecMatchesBlob pins caddyExternalScannerSpec's
// Externals list -- the binding source for
// CaddyExternalScanner.ExternalScannerForLanguage -- against the shipped
// caddy.bin's actual external symbol count and order.
func TestCaddyExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("caddy")
	if !ok {
		t.Fatal("missing caddy external scanner spec")
	}
	wantExternals := []string{
		"_newline",
		"_indent",
		"_dedent",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("caddy spec externals = %v, want %v", spec.Externals, wantExternals)
	}

	lang := Language("caddy")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("caddy blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := CaddyExternalScanner{}.ExternalScannerForLanguage(lang).(CaddyExternalScanner)
	if !ok {
		t.Fatalf("CaddyExternalScanner binding type = %T, want CaddyExternalScanner", CaddyExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, caddyTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("caddy externalToToken = %v, want %v (all %d externals must bind)", got, want, caddyTokenCount)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("caddy external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("caddy external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestDoxygenExternalScannerSpecMatchesBlob pins doxygenExternalScannerSpec's
// Externals list -- the binding source for
// DoxygenExternalScanner.ExternalScannerForLanguage -- against the shipped
// doxygen.bin's actual external symbol count and order.
func TestDoxygenExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("doxygen")
	if !ok {
		t.Fatal("missing doxygen external scanner spec")
	}
	wantExternals := []string{
		"brief_text",
		"code_block_start",
		"code_block_language",
		"code_block_content",
		"code_block_end",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("doxygen spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "ccd998f378c3f9345ea4eeb223f56d7b84d16687"; got != want {
		t.Fatalf("doxygen spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("doxygen")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("doxygen blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := DoxygenExternalScanner{}.ExternalScannerForLanguage(lang).(DoxygenExternalScanner)
	if !ok {
		t.Fatalf("DoxygenExternalScanner binding type = %T, want DoxygenExternalScanner", DoxygenExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, doxygenTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("doxygen externalToToken = %v, want %v (all %d externals must bind)", got, want, doxygenTokenCount)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("doxygen external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("doxygen external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestBladeExternalScannerSpecMatchesBlob pins bladeExternalScannerSpec's
// Externals list -- the binding source for
// BladeExternalScanner.ExternalScannerForLanguage -- against the shipped
// blade.bin's actual external symbol count and order.
func TestBladeExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("blade")
	if !ok {
		t.Fatal("missing blade external scanner spec")
	}
	wantExternals := []string{
		"_start_tag_name",
		"_script_start_tag_name",
		"_style_start_tag_name",
		"_end_tag_name",
		"erroneous_end_tag_name",
		"/>",
		"_implicit_end_tag",
		"raw_text",
		"comment",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("blade spec externals = %v, want %v", spec.Externals, wantExternals)
	}

	lang := Language("blade")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("blade blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := BladeExternalScanner{}.ExternalScannerForLanguage(lang).(BladeExternalScanner)
	if !ok {
		t.Fatalf("BladeExternalScanner binding type = %T, want BladeExternalScanner", BladeExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, bladeTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("blade externalToToken = %v, want %v (all %d externals must bind)", got, want, bladeTokenCount)
	}
	if got, want := scanner.symbols, bladeDefaultSymTable; got != want {
		t.Fatalf("blade post-bind symbols = %v, want default table %v", got, want)
	}

	// The four tag-name externals (start/script-start/style-start/end) alias
	// to the same visible "tag_name" node type, so their display names
	// collapse; the remaining five externals display exactly as their spec
	// name.
	wantDisplay := []string{
		"tag_name", "tag_name", "tag_name", "tag_name",
		"erroneous_end_tag_name", "/>", "_implicit_end_tag", "raw_text", "comment",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("blade external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("blade external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestAngularExternalScannerSpecMatchesBlob pins angularExternalScannerSpec's
// Externals list -- the binding source for
// AngularExternalScanner.ExternalScannerForLanguage -- against the shipped
// angular.bin's actual external symbol count and order.
func TestAngularExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("angular")
	if !ok {
		t.Fatal("missing angular external scanner spec")
	}
	wantExternals := []string{
		"_start_tag_name",
		"_script_start_tag_name",
		"_style_start_tag_name",
		"_end_tag_name",
		"erroneous_end_tag_name",
		"/>",
		"_implicit_end_tag",
		"raw_text",
		"comment",
		"_interpolation_start",
		"_interpolation_end",
		"_control_flow_start",
		"_empty_quoted_string",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("angular spec externals = %v, want %v", spec.Externals, wantExternals)
	}

	lang := Language("angular")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("angular blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := AngularExternalScanner{}.ExternalScannerForLanguage(lang).(AngularExternalScanner)
	if !ok {
		t.Fatalf("AngularExternalScanner binding type = %T, want AngularExternalScanner", AngularExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, angularTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("angular externalToToken = %v, want %v (all %d externals must bind)", got, want, angularTokenCount)
	}
	if got, want := scanner.symbols, angularDefaultSymTable; got != want {
		t.Fatalf("angular post-bind symbols = %v, want default table %v", got, want)
	}

	// The four tag-name externals (start/script-start/style-start/end) alias
	// to the same visible "tag_name" node type, and the two interpolation
	// delimiters, the control-flow marker, and the empty-quoted-string alias
	// display as their literal text, so their display names collapse or
	// differ from the spec's rule names; the remaining three externals
	// display exactly as their spec name.
	wantDisplay := []string{
		"tag_name", "tag_name", "tag_name", "tag_name",
		"erroneous_end_tag_name", "/>", "_implicit_end_tag", "raw_text", "comment",
		"{{", "}}", "@", "\"\"",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("angular external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("angular external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestCssExternalScannerSpecMatchesBlob pins cssExternalScannerSpec's
// Externals list -- the binding source for
// CssExternalScanner.ExternalScannerForLanguage -- against the shipped
// css.bin's actual external symbol count and order.
func TestCssExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("css")
	if !ok {
		t.Fatal("missing css external scanner spec")
	}
	wantExternals := []string{
		"_descendant_operator",
		"_pseudo_class_selector_colon",
		"__error_recovery",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("css spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "dda5cfc5722c429eaba1c910ca32c2c0c5bb1a3f"; got != want {
		t.Fatalf("css spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("css")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("css blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := CssExternalScanner{}.ExternalScannerForLanguage(lang).(CssExternalScanner)
	if !ok {
		t.Fatalf("CssExternalScanner binding type = %T, want CssExternalScanner", CssExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, cssTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("css externalToToken = %v, want %v (all %d externals must bind)", got, want, cssTokenCount)
	}
	if got, want := scanner.symbols, cssDefaultSymTable; got != want {
		t.Fatalf("css post-bind symbols = %v, want default table %v", got, want)
	}

	// The pseudo-class selector colon displays as its literal text ":" on
	// the loaded Language rather than its spec rule name.
	wantDisplay := []string{"_descendant_operator", ":", "__error_recovery"}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("css external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("css external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestTomlExternalScannerSpecMatchesBlob pins tomlExternalScannerSpec's
// Externals list -- the binding source for
// TomlExternalScanner.ExternalScannerForLanguage -- against the shipped
// toml.bin's actual external symbol count and order.
func TestTomlExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("toml")
	if !ok {
		t.Fatal("missing toml external scanner spec")
	}
	wantExternals := []string{
		"_line_ending_or_eof",
		"_multiline_basic_string_content",
		"_multiline_basic_string_end",
		"_multiline_literal_string_content",
		"_multiline_literal_string_end",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("toml spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "342d9be207c2dba869b9967124c679b5e6fd0ebe"; got != want {
		t.Fatalf("toml spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("toml")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("toml blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := TomlExternalScanner{}.ExternalScannerForLanguage(lang).(TomlExternalScanner)
	if !ok {
		t.Fatalf("TomlExternalScanner binding type = %T, want TomlExternalScanner", TomlExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, tomlTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("toml externalToToken = %v, want %v (all %d externals must bind)", got, want, tomlTokenCount)
	}
	if got, want := scanner.symbols, tomlDefaultSymTable; got != want {
		t.Fatalf("toml post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("toml external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("toml external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestHtmlExternalScannerSpecMatchesBlob pins htmlExternalScannerSpec's
// Externals list -- the binding source for
// HTMLExternalScanner.ExternalScannerForLanguage -- against the shipped
// html.bin's actual external symbol count and order.
func TestHtmlExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("html")
	if !ok {
		t.Fatal("missing html external scanner spec")
	}
	wantExternals := []string{
		"_start_tag_name",
		"_script_start_tag_name",
		"_style_start_tag_name",
		"_end_tag_name",
		"erroneous_end_tag_name",
		"/>",
		"_implicit_end_tag",
		"raw_text",
		"comment",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("html spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "73a3947324f6efddf9e17c0ea58d454843590cc0"; got != want {
		t.Fatalf("html spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("html")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("html blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := HTMLExternalScanner{}.ExternalScannerForLanguage(lang).(HTMLExternalScanner)
	if !ok {
		t.Fatalf("HTMLExternalScanner binding type = %T, want HTMLExternalScanner", HTMLExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, htmlTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("html externalToToken = %v, want %v (all %d externals must bind)", got, want, htmlTokenCount)
	}
	if got, want := scanner.symbols, htmlDefaultSymTable; got != want {
		t.Fatalf("html post-bind symbols = %v, want default table %v", got, want)
	}

	// The four tag-name externals (start/script-start/style-start/end) alias
	// to the same visible "tag_name" node type, so their display names
	// collapse; the remaining five externals display exactly as their spec
	// name.
	wantDisplay := []string{
		"tag_name", "tag_name", "tag_name", "tag_name",
		"erroneous_end_tag_name", "/>", "_implicit_end_tag", "raw_text", "comment",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("html external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("html external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestBashExternalScannerSpecMatchesBlob pins bshExternalScannerSpec's
// Externals list -- the binding source for
// BashExternalScanner.ExternalScannerForLanguage -- against the shipped
// bash.bin's actual external symbol count and order.
func TestBashExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("bash")
	if !ok {
		t.Fatal("missing bash external scanner spec")
	}
	wantExternals := []string{
		"heredoc_start",
		"simple_heredoc_body",
		"_heredoc_body_beginning",
		"heredoc_content",
		"heredoc_end",
		"file_descriptor",
		"_empty_value",
		"_concat",
		"variable_name",
		"test_operator",
		"regex",
		"_regex_no_slash",
		"_regex_no_space",
		"_expansion_word",
		"extglob_pattern",
		"_bare_dollar",
		"_brace_start",
		"_immediate_double_hash",
		"_external_expansion_sym_hash",
		"_external_expansion_sym_bang",
		"_external_expansion_sym_equal",
		"}",
		"]",
		"<<",
		"<<-",
		"\n",
		"(",
		"esac",
		"__error_recovery",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("bash spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "a06c2e4415e9bc0346c6b86d401879ffb44058f7"; got != want {
		t.Fatalf("bash spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("bash")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("bash blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := BashExternalScanner{}.ExternalScannerForLanguage(lang).(BashExternalScanner)
	if !ok {
		t.Fatalf("BashExternalScanner binding type = %T, want BashExternalScanner", BashExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, bshTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("bash externalToToken = %v, want %v (all %d externals must bind)", got, want, bshTokenCount)
	}
	if got, want := scanner.symbols, bshDefaultSymTable; got != want {
		t.Fatalf("bash post-bind symbols = %v, want default table %v", got, want)
	}

	// regex/_regex_no_slash/_regex_no_space all alias to the same visible
	// "regex" node type; the remaining externals mostly display their
	// literal text (bare-dollar, brace-start, immediate-double-hash, the
	// three external-expansion-sym externals) or a grammar-internal alias
	// name ("word" for _expansion_word, "heredoc_redirect_token1" for the
	// bare newline pattern external) instead of their spec rule name.
	wantDisplay := []string{
		"heredoc_start", "heredoc_body", "_heredoc_body_beginning", "heredoc_content", "heredoc_end",
		"file_descriptor", "_empty_value", "_concat", "variable_name", "test_operator",
		"regex", "regex", "regex", "word", "extglob_pattern",
		"$", "{", "##", "#", "!", "=",
		"}", "]", "<<", "<<-", "heredoc_redirect_token1", "(", "esac", "__error_recovery",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("bash external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("bash external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestRubyExternalScannerSpecMatchesBlob pins rbyExternalScannerSpec's
// Externals list -- the binding source for
// RubyExternalScanner.ExternalScannerForLanguage -- against the shipped
// ruby.bin's actual external symbol count and order.
func TestRubyExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("ruby")
	if !ok {
		t.Fatal("missing ruby external scanner spec")
	}
	wantExternals := []string{
		"_line_break",
		"_no_line_break",
		"simple_symbol",
		"_string_start",
		"_symbol_start",
		"_subshell_start",
		"_regex_start",
		"_string_array_start",
		"_symbol_array_start",
		"_heredoc_body_start",
		"string_content",
		"heredoc_content",
		"_string_end",
		"heredoc_end",
		"heredoc_beginning",
		"/",
		"_block_ampersand",
		"_splat_star",
		"_unary_minus",
		"_unary_minus_num",
		"_binary_minus",
		"_binary_star",
		"_singleton_class_left_angle_left_langle",
		"hash_key_symbol",
		"_identifier_suffix",
		"_constant_suffix",
		"_hash_splat_star_star",
		"_binary_star_star",
		"_element_reference_bracket",
		"_short_interpolation",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("ruby spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "ad907a69da0c8a4f7a943a7fe012712208da6dee"; got != want {
		t.Fatalf("ruby spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("ruby")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("ruby blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := RubyExternalScanner{}.ExternalScannerForLanguage(lang).(RubyExternalScanner)
	if !ok {
		t.Fatalf("RubyExternalScanner binding type = %T, want RubyExternalScanner", RubyExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, rbyTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("ruby externalToToken = %v, want %v (all %d externals must bind)", got, want, rbyTokenCount)
	}
	if got, want := scanner.symbols, rbyDefaultSymTable; got != want {
		t.Fatalf("ruby post-bind symbols = %v, want default table %v", got, want)
	}

	// Most literal-delimiter externals (string/symbol/subshell/regex/array
	// starts, the close paren, the "/" forward slash, block-ampersand,
	// splat/binary star(s), minus variants, "<<", "[") display their literal
	// text on the loaded Language rather than their spec rule name; the
	// remaining hidden/named externals display exactly as their spec name.
	wantDisplay := []string{
		"_line_break", "_no_line_break", "simple_symbol", "\"", ":\"",
		"`", "/", "%w(", "%i(", "_heredoc_body_start",
		"string_content", "heredoc_content", ")", "heredoc_end", "heredoc_beginning",
		"/", "&", "*", "-", "-",
		"-", "*", "<<", "hash_key_symbol", "_identifier_suffix",
		"_constant_suffix", "**", "**", "[", "_short_interpolation",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("ruby external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("ruby external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestPhpExternalScannerSpecMatchesBlob pins phpExternalScannerSpec's
// Externals list -- the binding source for
// PhpExternalScanner.ExternalScannerForLanguage -- against the shipped
// php.bin's actual external symbol count and order.
func TestPhpExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("php")
	if !ok {
		t.Fatal("missing php external scanner spec")
	}
	wantExternals := []string{
		"_automatic_semicolon",
		"encapsed_string_chars",
		"encapsed_string_chars_after_variable",
		"execution_string_chars",
		"execution_string_chars_after_variable",
		"encapsed_string_chars_heredoc",
		"encapsed_string_chars_after_variable_heredoc",
		"_eof",
		"heredoc_start",
		"heredoc_end",
		"nowdoc_string",
		"sentinel_error",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("php spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "3fda2fb9577166c6399834917f9844f30370beea"; got != want {
		t.Fatalf("php spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("php")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("php blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := PhpExternalScanner{}.ExternalScannerForLanguage(lang).(PhpExternalScanner)
	if !ok {
		t.Fatalf("PhpExternalScanner binding type = %T, want PhpExternalScanner", PhpExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, phpTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("php externalToToken = %v, want %v (all %d externals must bind)", got, want, phpTokenCount)
	}
	if got, want := scanner.symbols, phpDefaultSymTable; got != want {
		t.Fatalf("php post-bind symbols = %v, want default table %v", got, want)
	}

	// The six encapsed/execution string-chars variants (plain, after-variable,
	// heredoc, and after-variable-heredoc, across normal and execution
	// strings) all alias to the same visible "string_content" node type; the
	// remaining externals display exactly as their spec name.
	wantDisplay := []string{
		"_automatic_semicolon",
		"string_content", "string_content", "string_content", "string_content", "string_content", "string_content",
		"_eof", "heredoc_start", "heredoc_end", "nowdoc_string", "sentinel_error",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("php external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("php external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}
