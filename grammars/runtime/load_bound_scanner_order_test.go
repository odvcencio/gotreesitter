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

// TestCmakeExternalScannerSpecMatchesBlob pins cmakeExternalScannerSpec's
// Externals list -- the binding source for
// CmakeExternalScanner.ExternalScannerForLanguage -- against the shipped
// cmake.bin's actual external symbol count and order.
func TestCmakeExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("cmake")
	if !ok {
		t.Fatal("missing cmake external scanner spec")
	}
	wantExternals := []string{
		"bracket_argument_open",
		"bracket_argument_content",
		"bracket_argument_close",
		"bracket_comment_open",
		"bracket_comment_content",
		"bracket_comment_close",
		"line_comment",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("cmake spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "58993af75218bc99a1f5a04c832a5937e7c422cb"; got != want {
		t.Fatalf("cmake spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("cmake")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("cmake blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := CmakeExternalScanner{}.ExternalScannerForLanguage(lang).(CmakeExternalScanner)
	if !ok {
		t.Fatalf("CmakeExternalScanner binding type = %T, want CmakeExternalScanner", CmakeExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, cmakeTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("cmake externalToToken = %v, want %v (all %d externals must bind)", got, want, cmakeTokenCount)
	}
	if got, want := scanner.symbols, cmakeDefaultSymTable; got != want {
		t.Fatalf("cmake post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("cmake external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("cmake external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestCppExternalScannerSpecMatchesBlob pins cppExternalScannerSpec's
// Externals list -- the binding source for
// CppExternalScanner.ExternalScannerForLanguage -- against the shipped
// cpp.bin's actual external symbol count and order.
func TestCppExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("cpp")
	if !ok {
		t.Fatal("missing cpp external scanner spec")
	}
	wantExternals := []string{
		"raw_string_delimiter",
		"raw_string_content",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("cpp spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "c009222808634c1014f82438d4883753516a2c24"; got != want {
		t.Fatalf("cpp spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("cpp")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("cpp blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := CppExternalScanner{}.ExternalScannerForLanguage(lang).(CppExternalScanner)
	if !ok {
		t.Fatalf("CppExternalScanner binding type = %T, want CppExternalScanner", CppExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, cppTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("cpp externalToToken = %v, want %v (all %d externals must bind)", got, want, cppTokenCount)
	}
	if got, want := scanner.symbols, cppDefaultSymTable; got != want {
		t.Fatalf("cpp post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("cpp external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("cpp external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestElixirExternalScannerSpecMatchesBlob pins elixirExternalScannerSpec's
// Externals list -- the binding source for
// ElixirExternalScanner.ExternalScannerForLanguage -- against the shipped
// elixir.bin's actual external symbol count and order.
func TestElixirExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("elixir")
	if !ok {
		t.Fatal("missing elixir external scanner spec")
	}
	wantExternals := []string{
		"_quoted_content_i_single",
		"_quoted_content_i_double",
		"_quoted_content_i_heredoc_single",
		"_quoted_content_i_heredoc_double",
		"_quoted_content_i_parenthesis",
		"_quoted_content_i_curly",
		"_quoted_content_i_square",
		"_quoted_content_i_angle",
		"_quoted_content_i_bar",
		"_quoted_content_i_slash",
		"_quoted_content_single",
		"_quoted_content_double",
		"_quoted_content_heredoc_single",
		"_quoted_content_heredoc_double",
		"_quoted_content_parenthesis",
		"_quoted_content_curly",
		"_quoted_content_square",
		"_quoted_content_angle",
		"_quoted_content_bar",
		"_quoted_content_slash",
		"_newline_before_do",
		"_newline_before_binary_operator",
		"_newline_before_comment",
		"_before_unary_op",
		"_not_in",
		"_quoted_atom_start",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("elixir spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "4b0c7118760af58a2e7081bbc8396e136f820b37"; got != want {
		t.Fatalf("elixir spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("elixir")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("elixir blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := ElixirExternalScanner{}.ExternalScannerForLanguage(lang).(ElixirExternalScanner)
	if !ok {
		t.Fatalf("ElixirExternalScanner binding type = %T, want ElixirExternalScanner", ElixirExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, elixirTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("elixir externalToToken = %v, want %v (all %d externals must bind)", got, want, elixirTokenCount)
	}
	if got, want := scanner.symbols, elixirDefaultSymTable; got != want {
		t.Fatalf("elixir post-bind symbols = %v, want default table %v", got, want)
	}

	// The 20 _quoted_content_* variants all alias to the same visible
	// "quoted_content" node type; _not_in displays as "not in" and
	// _quoted_atom_start displays as ":"; the remaining externals display
	// exactly as their spec name.
	wantDisplay := []string{
		"quoted_content", "quoted_content", "quoted_content", "quoted_content", "quoted_content",
		"quoted_content", "quoted_content", "quoted_content", "quoted_content", "quoted_content",
		"quoted_content", "quoted_content", "quoted_content", "quoted_content", "quoted_content",
		"quoted_content", "quoted_content", "quoted_content", "quoted_content", "quoted_content",
		"_newline_before_do", "_newline_before_binary_operator", "_newline_before_comment",
		"_before_unary_op", "not in", ":",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("elixir external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("elixir external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestElmExternalScannerSpecMatchesBlob pins elmExternalScannerSpec's
// Externals list -- the binding source for
// ElmExternalScanner.ExternalScannerForLanguage -- against the shipped
// elm.bin's actual external symbol count and order.
func TestElmExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("elm")
	if !ok {
		t.Fatal("missing elm external scanner spec")
	}
	wantExternals := []string{
		"_virtual_end_decl",
		"_virtual_open_section",
		"_virtual_end_section",
		"minus_without_trailing_whitespace",
		"glsl_content",
		"_block_comment_content",
		"_string_content_multiline",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("elm spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "e1e8fea161a1e66f3997855d316be2a43e4e956f"; got != want {
		t.Fatalf("elm spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("elm")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("elm blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := ElmExternalScanner{}.ExternalScannerForLanguage(lang).(ElmExternalScanner)
	if !ok {
		t.Fatalf("ElmExternalScanner binding type = %T, want ElmExternalScanner", ElmExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, elmTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("elm externalToToken = %v, want %v (all %d externals must bind)", got, want, elmTokenCount)
	}
	if got, want := scanner.symbols, elmDefaultSymTable; got != want {
		t.Fatalf("elm post-bind symbols = %v, want default table %v", got, want)
	}

	// minus_without_trailing_whitespace displays as "operator_identifier" and
	// _string_content_multiline displays as "regular_string_part" on the
	// loaded Language; the remaining externals display exactly as their spec
	// name.
	wantDisplay := []string{
		"_virtual_end_decl", "_virtual_open_section", "_virtual_end_section",
		"operator_identifier", "glsl_content", "_block_comment_content", "regular_string_part",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("elm external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("elm external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestGoExternalScannerSpecMatchesBlob pins goExternalScannerSpec's
// Externals list -- the binding source for
// GoExternalScanner.ExternalScannerForLanguage -- against the shipped
// go.bin's actual external symbol count and order. Unlike every other
// language here, this external is not ported from an upstream C scanner.c
// (upstream tree-sitter-go has none); see goExternalScannerSpec's doc
// comment in go_scanner.go for why grammargen invents it.
func TestGoExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("go")
	if !ok {
		t.Fatal("missing go external scanner spec")
	}
	wantExternals := []string{
		"_automatic_semicolon",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("go spec externals = %v, want %v", spec.Externals, wantExternals)
	}

	lang := Language("go")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("go blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := GoExternalScanner{}.ExternalScannerForLanguage(lang).(GoExternalScanner)
	if !ok {
		t.Fatalf("GoExternalScanner binding type = %T, want GoExternalScanner", GoExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, goTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("go externalToToken = %v, want %v (all %d externals must bind)", got, want, goTokenCount)
	}
	if got, want := scanner.symbols, goDefaultSymTable; got != want {
		t.Fatalf("go post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("go external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("go external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestHaskellExternalScannerSpecMatchesBlob pins hsExternalScannerSpec's
// Externals list -- the binding source for
// HaskellExternalScanner.ExternalScannerForLanguage -- against the shipped
// haskell.bin's actual external symbol count and order.
func TestHaskellExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("haskell")
	if !ok {
		t.Fatal("missing haskell external scanner spec")
	}
	wantExternals := []string{
		"error_sentinel",
		"_cond_layout_semicolon",
		"_cmd_layout_start",
		"_cmd_layout_start_do",
		"_cmd_layout_start_case",
		"_cmd_layout_start_if",
		"_cmd_layout_start_let",
		"_cmd_layout_start_quote",
		"_cmd_layout_start_explicit",
		"_cond_layout_end",
		"_cond_layout_end_explicit",
		"_cmd_brace_open",
		"_cmd_brace_close",
		"_cmd_texp_start",
		"_cmd_texp_end",
		"_phantom_where",
		"_phantom_in",
		"_phantom_arrow",
		"_phantom_bar",
		"_phantom_deriving",
		"comment",
		"haddock",
		"cpp",
		"pragma",
		"_cond_quote_start",
		"quasiquote_body",
		"_cond_splice",
		"_cond_qual_dot",
		"_cond_tight_dot",
		"_cond_prefix_dot",
		"_cond_dotdot",
		"_cond_tight_at",
		"_cond_prefix_at",
		"_cond_tight_bang",
		"_cond_prefix_bang",
		"_cond_tight_tilde",
		"_cond_prefix_tilde",
		"_cond_prefix_percent",
		"_cond_qualified_op",
		"_cond_left_section_op",
		"_cond_no_section_op",
		"_cond_minus",
		"_cond_context",
		"_cond_infix",
		"_cond_data_infix",
		"_cond_assoc_tyinst",
		"_varsym",
		"_consym",
		"\n",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("haskell spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "0975ef72fc3c47b530309ca93937d7d143523628"; got != want {
		t.Fatalf("haskell spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("haskell")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("haskell blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := HaskellExternalScanner{}.ExternalScannerForLanguage(lang).(HaskellExternalScanner)
	if !ok {
		t.Fatalf("HaskellExternalScanner binding type = %T, want HaskellExternalScanner", HaskellExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, hsTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("haskell externalToToken = %v, want %v (all %d externals must bind)", got, want, hsTokenCount)
	}
	if got, want := scanner.symbols, hsDefaultSymTable; got != want {
		t.Fatalf("haskell post-bind symbols = %v, want default table %v", got, want)
	}

	// _cmd_layout_start_explicit and _cond_layout_end_explicit display as
	// their literal text ("{" and "}"); the trailing bare newline pattern
	// external displays as the grammar-internal alias "_token1"; the
	// remaining externals display exactly as their spec name.
	wantDisplay := []string{
		"error_sentinel", "_cond_layout_semicolon", "_cmd_layout_start", "_cmd_layout_start_do",
		"_cmd_layout_start_case", "_cmd_layout_start_if", "_cmd_layout_start_let", "_cmd_layout_start_quote",
		"{", "_cond_layout_end", "}", "_cmd_brace_open",
		"_cmd_brace_close", "_cmd_texp_start", "_cmd_texp_end", "_phantom_where",
		"_phantom_in", "_phantom_arrow", "_phantom_bar", "_phantom_deriving",
		"comment", "haddock", "cpp", "pragma",
		"_cond_quote_start", "quasiquote_body", "_cond_splice", "_cond_qual_dot",
		"_cond_tight_dot", "_cond_prefix_dot", "_cond_dotdot", "_cond_tight_at",
		"_cond_prefix_at", "_cond_tight_bang", "_cond_prefix_bang", "_cond_tight_tilde",
		"_cond_prefix_tilde", "_cond_prefix_percent", "_cond_qualified_op", "_cond_left_section_op",
		"_cond_no_section_op", "_cond_minus", "_cond_context", "_cond_infix",
		"_cond_data_infix", "_cond_assoc_tyinst", "_varsym", "_consym",
		"_token1",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("haskell external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("haskell external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestJuliaExternalScannerSpecMatchesBlob pins juliaExternalScannerSpec's
// Externals list -- the binding source for
// JuliaExternalScanner.ExternalScannerForLanguage -- against the shipped
// julia.bin's actual external symbol count and order.
func TestJuliaExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("julia")
	if !ok {
		t.Fatal("missing julia external scanner spec")
	}
	wantExternals := []string{
		"_block_comment_rest",
		"_immediate_paren",
		"_immediate_bracket",
		"_immediate_brace",
		"_immediate_string_start",
		"_immediate_command_start",
		"_content_cmd_1",
		"_content_cmd_1_raw",
		"_content_cmd_3",
		"_content_cmd_3_raw",
		"_content_str_1",
		"_content_str_1_raw",
		"_content_str_3",
		"_content_str_3_raw",
		"_end_cmd",
		"_end_str",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("julia spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "e0f9dcd180fdcfcfa8d79a3531e11d99e79321d3"; got != want {
		t.Fatalf("julia spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("julia")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("julia blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := JuliaExternalScanner{}.ExternalScannerForLanguage(lang).(JuliaExternalScanner)
	if !ok {
		t.Fatalf("JuliaExternalScanner binding type = %T, want JuliaExternalScanner", JuliaExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, juliaTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("julia externalToToken = %v, want %v (all %d externals must bind)", got, want, juliaTokenCount)
	}
	if got, want := scanner.symbols, juliaDefaultSymTable; got != want {
		t.Fatalf("julia post-bind symbols = %v, want default table %v", got, want)
	}

	// The eight _content_* externals (indexes 6-13) alias to the same
	// visible "content" node type; the remaining externals display exactly
	// as their spec name.
	wantDisplay := []string{
		"_block_comment_rest", "_immediate_paren", "_immediate_bracket", "_immediate_brace",
		"_immediate_string_start", "_immediate_command_start", "content", "content",
		"content", "content", "content", "content",
		"content", "content", "_end_cmd", "_end_str",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("julia external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("julia external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestLuaExternalScannerSpecMatchesBlob pins luaExternalScannerSpec's
// Externals list -- the binding source for
// LuaExternalScanner.ExternalScannerForLanguage -- against the shipped
// lua.bin's actual external symbol count and order.
func TestLuaExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("lua")
	if !ok {
		t.Fatal("missing lua external scanner spec")
	}
	wantExternals := []string{
		"_block_comment_start",
		"_block_comment_content",
		"_block_comment_end",
		"_block_string_start",
		"_block_string_content",
		"_block_string_end",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("lua spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "10fe0054734eec83049514ea2e718b2a56acd0c9"; got != want {
		t.Fatalf("lua spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("lua")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("lua blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := LuaExternalScanner{}.ExternalScannerForLanguage(lang).(LuaExternalScanner)
	if !ok {
		t.Fatalf("LuaExternalScanner binding type = %T, want LuaExternalScanner", LuaExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, luaTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("lua externalToToken = %v, want %v (all %d externals must bind)", got, want, luaTokenCount)
	}
	if got, want := scanner.symbols, luaDefaultSymTable; got != want {
		t.Fatalf("lua post-bind symbols = %v, want default table %v", got, want)
	}

	// _block_comment_start/_block_string_start display as the literal "[["
	// and _block_comment_end/_block_string_end display as the literal "]]",
	// each sharing a Symbol ID with every other occurrence of that literal
	// elsewhere in the grammar; the two content externals display as their
	// grammar-collapsed node names.
	wantDisplay := []string{
		"[[", "comment_content", "]]", "[[", "string_content", "]]",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("lua external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("lua external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestMarkdownExternalScannerSpecMatchesBlob pins mdExternalScannerSpec's
// Externals list -- the binding source for
// MarkdownExternalScanner.ExternalScannerForLanguage -- against the shipped
// markdown.bin's actual external symbol count and order.
func TestMarkdownExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("markdown")
	if !ok {
		t.Fatal("missing markdown external scanner spec")
	}
	wantExternals := []string{
		"_line_ending",
		"_soft_line_ending",
		"_block_close",
		"block_continuation",
		"_block_quote_start",
		"_indented_chunk_start",
		"atx_h1_marker",
		"atx_h2_marker",
		"atx_h3_marker",
		"atx_h4_marker",
		"atx_h5_marker",
		"atx_h6_marker",
		"setext_h1_underline",
		"setext_h2_underline",
		"_thematic_break",
		"_list_marker_minus",
		"_list_marker_plus",
		"_list_marker_star",
		"_list_marker_parenthesis",
		"_list_marker_dot",
		"_list_marker_minus_dont_interrupt",
		"_list_marker_plus_dont_interrupt",
		"_list_marker_star_dont_interrupt",
		"_list_marker_parenthesis_dont_interrupt",
		"_list_marker_dot_dont_interrupt",
		"_fenced_code_block_start_backtick",
		"_fenced_code_block_start_tilde",
		"_blank_line_start",
		"_fenced_code_block_end_backtick",
		"_fenced_code_block_end_tilde",
		"_html_block_1_start",
		"_html_block_1_end",
		"_html_block_2_start",
		"_html_block_3_start",
		"_html_block_4_start",
		"_html_block_5_start",
		"_html_block_6_start",
		"_html_block_7_start",
		"_close_block",
		"_no_indented_chunk",
		"_error",
		"_trigger_error",
		"_eof",
		"minus_metadata",
		"plus_metadata",
		"_pipe_table_start",
		"_pipe_table_line_ending",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("markdown spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "a0a00f817d02412bd92c54d316f164d827b57b5c"; got != want {
		t.Fatalf("markdown spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("markdown")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("markdown blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := MarkdownExternalScanner{}.ExternalScannerForLanguage(lang).(MarkdownExternalScanner)
	if !ok {
		t.Fatalf("MarkdownExternalScanner binding type = %T, want MarkdownExternalScanner", MarkdownExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, mdTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("markdown externalToToken = %v, want %v (all %d externals must bind)", got, want, mdTokenCount)
	}
	if got, want := scanner.symbols, mdDefaultSymTable; got != want {
		t.Fatalf("markdown post-bind symbols = %v, want default table %v", got, want)
	}

	// _block_quote_start displays as "block_quote_marker"; the four fenced
	// code block start/end externals (indexes 25, 26, 28, 29) all display as
	// "fenced_code_block_delimiter"; the remaining externals display exactly
	// as their spec name.
	wantDisplay := []string{
		"_line_ending", "_soft_line_ending", "_block_close", "block_continuation",
		"block_quote_marker", "_indented_chunk_start", "atx_h1_marker", "atx_h2_marker",
		"atx_h3_marker", "atx_h4_marker", "atx_h5_marker", "atx_h6_marker",
		"setext_h1_underline", "setext_h2_underline", "_thematic_break", "_list_marker_minus",
		"_list_marker_plus", "_list_marker_star", "_list_marker_parenthesis", "_list_marker_dot",
		"_list_marker_minus_dont_interrupt", "_list_marker_plus_dont_interrupt", "_list_marker_star_dont_interrupt", "_list_marker_parenthesis_dont_interrupt",
		"_list_marker_dot_dont_interrupt", "fenced_code_block_delimiter", "fenced_code_block_delimiter", "_blank_line_start",
		"fenced_code_block_delimiter", "fenced_code_block_delimiter", "_html_block_1_start", "_html_block_1_end",
		"_html_block_2_start", "_html_block_3_start", "_html_block_4_start", "_html_block_5_start",
		"_html_block_6_start", "_html_block_7_start", "_close_block", "_no_indented_chunk",
		"_error", "_trigger_error", "_eof", "minus_metadata",
		"plus_metadata", "_pipe_table_start", "_pipe_table_line_ending",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("markdown external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("markdown external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestNixExternalScannerSpecMatchesBlob pins nixExternalScannerSpec's
// Externals list -- the binding source for
// NixExternalScanner.ExternalScannerForLanguage -- against the shipped
// nix.bin's actual external symbol count and order.
func TestNixExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("nix")
	if !ok {
		t.Fatal("missing nix external scanner spec")
	}
	wantExternals := []string{
		"string_fragment",
		"_indented_string_fragment",
		"_path_start",
		"path_fragment",
		"dollar_escape",
		"_indented_dollar_escape",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("nix spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "17f290c8b5104d9aba8a1ba7383a2ca83c3d14c4"; got != want {
		t.Fatalf("nix spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("nix")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("nix blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := NixExternalScanner{}.ExternalScannerForLanguage(lang).(NixExternalScanner)
	if !ok {
		t.Fatalf("NixExternalScanner binding type = %T, want NixExternalScanner", NixExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, nixTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("nix externalToToken = %v, want %v (all %d externals must bind)", got, want, nixTokenCount)
	}
	if got, want := scanner.symbols, nixDefaultSymTable; got != want {
		t.Fatalf("nix post-bind symbols = %v, want default table %v", got, want)
	}

	// _indented_string_fragment aliases string_fragment's display node and
	// _path_start aliases path_fragment's display node; the remaining
	// externals display exactly as their spec name.
	wantDisplay := []string{
		"string_fragment", "string_fragment", "path_fragment", "path_fragment",
		"dollar_escape", "dollar_escape",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("nix external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("nix external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestScssExternalScannerSpecMatchesBlob pins scssExternalScannerSpec's
// Externals list -- the binding source for
// ScssExternalScanner.ExternalScannerForLanguage -- against the shipped
// scss.bin's actual external symbol count and order.
func TestScssExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("scss")
	if !ok {
		t.Fatal("missing scss external scanner spec")
	}
	wantExternals := []string{
		"_descendant_operator",
		"_pseudo_class_selector_colon",
		"__error_recovery",
		"_concat",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("scss spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "2ef6d42e3ad7a8208900f9346f4529806ae0f9f9"; got != want {
		t.Fatalf("scss spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("scss")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("scss blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := ScssExternalScanner{}.ExternalScannerForLanguage(lang).(ScssExternalScanner)
	if !ok {
		t.Fatalf("ScssExternalScanner binding type = %T, want ScssExternalScanner", ScssExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, scssTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("scss externalToToken = %v, want %v (all %d externals must bind)", got, want, scssTokenCount)
	}
	if got, want := scanner.symbols, scssDefaultSymTable; got != want {
		t.Fatalf("scss post-bind symbols = %v, want default table %v", got, want)
	}

	// _pseudo_class_selector_colon displays as the literal ":"; the
	// remaining externals display exactly as their spec name.
	wantDisplay := []string{
		"_descendant_operator", ":", "__error_recovery", "_concat",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("scss external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("scss external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestSvelteExternalScannerSpecMatchesBlob pins svelteExternalScannerSpec's
// Externals list -- the binding source for
// SvelteExternalScanner.ExternalScannerForLanguage -- against the shipped
// svelte.bin's actual external symbol count and order.
func TestSvelteExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("svelte")
	if !ok {
		t.Fatal("missing svelte external scanner spec")
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
		"svelte_raw_text",
		"svelte_raw_text_each",
		"svelte_raw_text_snippet_arguments",
		"@",
		"#",
		"/",
		":",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("svelte spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "ae5199db47757f785e43a14b332118a5474de1a2"; got != want {
		t.Fatalf("svelte spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("svelte")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("svelte blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := SvelteExternalScanner{}.ExternalScannerForLanguage(lang).(SvelteExternalScanner)
	if !ok {
		t.Fatalf("SvelteExternalScanner binding type = %T, want SvelteExternalScanner", SvelteExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, svelteTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("svelte externalToToken = %v, want %v (all %d externals must bind)", got, want, svelteTokenCount)
	}
	if got, want := scanner.symbols, svelteDefaultSymTable; got != want {
		t.Fatalf("svelte post-bind symbols = %v, want default table %v", got, want)
	}

	// The four tag_name variants (start/script/style/end) each display as
	// "tag_name"; the four sigil externals (/>, @, #, /, :) each display as
	// their own literal; svelte_raw_text_each and
	// svelte_raw_text_snippet_arguments both display as "svelte_raw_text";
	// the remaining externals display exactly as their spec name.
	wantDisplay := []string{
		"tag_name", "tag_name", "tag_name", "tag_name",
		"erroneous_end_tag_name", "/>", "_implicit_end_tag", "raw_text",
		"comment", "svelte_raw_text", "svelte_raw_text", "svelte_raw_text",
		"@", "#", "/", ":",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("svelte external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("svelte external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestAwkExternalScannerSpecMatchesBlob pins awkExternalScannerSpec's
// Externals list -- the binding source for
// AwkExternalScanner.ExternalScannerForLanguage -- against the shipped
// awk.bin's actual external symbol count and order.
func TestAwkExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("awk")
	if !ok {
		t.Fatal("missing awk external scanner spec")
	}
	wantExternals := []string{
		"concatenating_space",
		"_if_else_separator",
		"_no_space",
		"_func_call",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("awk spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "34bbdc7cce8e803096f47b625979e34c1be38127"; got != want {
		t.Fatalf("awk spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("awk")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("awk blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := AwkExternalScanner{}.ExternalScannerForLanguage(lang).(AwkExternalScanner)
	if !ok {
		t.Fatalf("AwkExternalScanner binding type = %T, want AwkExternalScanner", AwkExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, awkTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("awk externalToToken = %v, want %v (all %d externals must bind)", got, want, awkTokenCount)
	}
	if got, want := scanner.symbols, awkDefaultSymTable; got != want {
		t.Fatalf("awk post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("awk external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("awk external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestXMLExternalScannerSpecMatchesBlob pins xmlExternalScannerSpec's
// Externals list -- the binding source for
// XMLExternalScanner.ExternalScannerForLanguage -- against the shipped
// xml.bin's actual external symbol count and order.
func TestXMLExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("xml")
	if !ok {
		t.Fatal("missing xml external scanner spec")
	}
	wantExternals := []string{
		"PITarget",
		"_pi_content",
		"Comment",
		"CharData",
		"CData",
		"xml-model",
		"xml-stylesheet",
		"_start_tag_name",
		"_end_tag_name",
		"_erroneous_end_name",
		"/>",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("xml spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "5000ae8f22d11fbe93939b05c1e37cf21117162d"; got != want {
		t.Fatalf("xml spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("xml")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("xml blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := XMLExternalScanner{}.ExternalScannerForLanguage(lang).(XMLExternalScanner)
	if !ok {
		t.Fatalf("XMLExternalScanner binding type = %T, want XMLExternalScanner", XMLExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, xmlTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("xml externalToToken = %v, want %v (all %d externals must bind)", got, want, xmlTokenCount)
	}
	if got, want := scanner.symbols, xmlDefaultSymTable; got != want {
		t.Fatalf("xml post-bind symbols = %v, want default table %v", got, want)
	}

	// _start_tag_name and _end_tag_name both display as "Name"; the
	// remaining externals display exactly as their spec name.
	wantDisplay := []string{
		"PITarget", "_pi_content", "Comment", "CharData",
		"CData", "xml-model", "xml-stylesheet", "Name",
		"Name", "_erroneous_end_name", "/>",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("xml external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("xml external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestYamlExternalScannerSpecMatchesBlob pins yamlExternalScannerSpec's
// Externals list -- the binding source for
// YamlExternalScanner.ExternalScannerForLanguage -- against the shipped
// yaml.bin's actual external symbol count and order.
func TestYamlExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("yaml")
	if !ok {
		t.Fatal("missing yaml external scanner spec")
	}
	wantExternals := []string{
		"_eof",
		"_s_dir_yml_bgn",
		"_r_dir_yml_ver",
		"_s_dir_tag_bgn",
		"_r_dir_tag_hdl",
		"_r_dir_tag_pfx",
		"_s_dir_rsv_bgn",
		"_r_dir_rsv_prm",
		"_s_drs_end",
		"_s_doc_end",
		"_r_blk_seq_bgn",
		"_br_blk_seq_bgn",
		"_b_blk_seq_bgn",
		"_r_blk_key_bgn",
		"_br_blk_key_bgn",
		"_b_blk_key_bgn",
		"_r_blk_val_bgn",
		"_br_blk_val_bgn",
		"_b_blk_val_bgn",
		"_r_blk_imp_bgn",
		"_r_blk_lit_bgn",
		"_br_blk_lit_bgn",
		"_r_blk_fld_bgn",
		"_br_blk_fld_bgn",
		"_br_blk_str_ctn",
		"_r_flw_seq_bgn",
		"_br_flw_seq_bgn",
		"_b_flw_seq_bgn",
		"_r_flw_seq_end",
		"_br_flw_seq_end",
		"_b_flw_seq_end",
		"_r_flw_map_bgn",
		"_br_flw_map_bgn",
		"_b_flw_map_bgn",
		"_r_flw_map_end",
		"_br_flw_map_end",
		"_b_flw_map_end",
		"_r_flw_sep_bgn",
		"_br_flw_sep_bgn",
		"_r_flw_key_bgn",
		"_br_flw_key_bgn",
		"_r_flw_jsv_bgn",
		"_br_flw_jsv_bgn",
		"_r_flw_njv_bgn",
		"_br_flw_njv_bgn",
		"_r_dqt_str_bgn",
		"_br_dqt_str_bgn",
		"_b_dqt_str_bgn",
		"_r_dqt_str_ctn",
		"_br_dqt_str_ctn",
		"_r_dqt_esc_nwl",
		"_br_dqt_esc_nwl",
		"_r_dqt_esc_seq",
		"_br_dqt_esc_seq",
		"_r_dqt_str_end",
		"_br_dqt_str_end",
		"_r_sqt_str_bgn",
		"_br_sqt_str_bgn",
		"_b_sqt_str_bgn",
		"_r_sqt_str_ctn",
		"_br_sqt_str_ctn",
		"_r_sqt_esc_sqt",
		"_br_sqt_esc_sqt",
		"_r_sqt_str_end",
		"_br_sqt_str_end",
		"_r_sgl_pln_nul_blk",
		"_br_sgl_pln_nul_blk",
		"_b_sgl_pln_nul_blk",
		"_r_sgl_pln_nul_flw",
		"_br_sgl_pln_nul_flw",
		"_r_sgl_pln_bol_blk",
		"_br_sgl_pln_bol_blk",
		"_b_sgl_pln_bol_blk",
		"_r_sgl_pln_bol_flw",
		"_br_sgl_pln_bol_flw",
		"_r_sgl_pln_int_blk",
		"_br_sgl_pln_int_blk",
		"_b_sgl_pln_int_blk",
		"_r_sgl_pln_int_flw",
		"_br_sgl_pln_int_flw",
		"_r_sgl_pln_flt_blk",
		"_br_sgl_pln_flt_blk",
		"_b_sgl_pln_flt_blk",
		"_r_sgl_pln_flt_flw",
		"_br_sgl_pln_flt_flw",
		"_r_sgl_pln_tms_blk",
		"_br_sgl_pln_tms_blk",
		"_b_sgl_pln_tms_blk",
		"_r_sgl_pln_tms_flw",
		"_br_sgl_pln_tms_flw",
		"_r_sgl_pln_str_blk",
		"_br_sgl_pln_str_blk",
		"_b_sgl_pln_str_blk",
		"_r_sgl_pln_str_flw",
		"_br_sgl_pln_str_flw",
		"_r_mtl_pln_str_blk",
		"_br_mtl_pln_str_blk",
		"_r_mtl_pln_str_flw",
		"_br_mtl_pln_str_flw",
		"_r_tag",
		"_br_tag",
		"_b_tag",
		"_r_acr_bgn",
		"_br_acr_bgn",
		"_b_acr_bgn",
		"_r_acr_ctn",
		"_r_als_bgn",
		"_br_als_bgn",
		"_b_als_bgn",
		"_r_als_ctn",
		"_bl",
		"comment",
		"_err_rec",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("yaml spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "a1c4812a73ec5e089de8e441fdea3a921e8d5079"; got != want {
		t.Fatalf("yaml spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("yaml")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("yaml blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := YamlExternalScanner{}.ExternalScannerForLanguage(lang).(YamlExternalScanner)
	if !ok {
		t.Fatalf("YamlExternalScanner binding type = %T, want YamlExternalScanner", YamlExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, yTokCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("yaml externalToToken = %v, want %v (all %d externals must bind)", got, want, yTokCount)
	}
	if got, want := scanner.symbols, yamlDefaultSymTable; got != want {
		t.Fatalf("yaml post-bind symbols = %v, want default table %v", got, want)
	}

	// Many externals collapse onto a small set of shared display node
	// types (block/flow indicator punctuation, scalar kinds, tag/anchor/
	// alias markers); this table records the currently-shipped blob's
	// per-index display name directly rather than re-deriving the
	// aliasing rules.
	wantDisplay := []string{
		"_eof", "_s_dir_yml_bgn", "yaml_version", "_s_dir_tag_bgn",
		"tag_handle", "tag_prefix", "directive_name", "directive_parameter",
		"---", "...", "-", "-",
		"-", "?", "?", "?",
		":", ":", ":", ":",
		"|", "|", ">", ">",
		"_br_blk_str_ctn", "[", "[", "[",
		"]", "]", "]", "{",
		"{", "{", "}", "}",
		"}", ",", ",", "?",
		"?", ":", ":", ":",
		":", "\"", "\"", "\"",
		"_r_dqt_str_ctn", "_br_dqt_str_ctn", "escape_sequence", "escape_sequence",
		"escape_sequence", "escape_sequence", "\"", "\"",
		"'", "'", "'", "_r_sqt_str_ctn",
		"_br_sqt_str_ctn", "escape_sequence", "escape_sequence", "'",
		"'", "null_scalar", "null_scalar", "null_scalar",
		"null_scalar", "null_scalar", "boolean_scalar", "boolean_scalar",
		"boolean_scalar", "boolean_scalar", "boolean_scalar", "integer_scalar",
		"integer_scalar", "integer_scalar", "integer_scalar", "integer_scalar",
		"float_scalar", "float_scalar", "float_scalar", "float_scalar",
		"float_scalar", "timestamp_scalar", "timestamp_scalar", "timestamp_scalar",
		"timestamp_scalar", "timestamp_scalar", "string_scalar", "string_scalar",
		"string_scalar", "string_scalar", "string_scalar", "string_scalar",
		"string_scalar", "string_scalar", "string_scalar", "tag",
		"tag", "tag", "&", "&",
		"&", "anchor_name", "*", "*",
		"*", "alias_name", "_bl", "comment",
		"_err_rec",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("yaml external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("yaml external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestAgdaExternalScannerSpecMatchesBlob pins agdaExternalScannerSpec's
// Externals list -- the binding source for
// AgdaExternalScanner.ExternalScannerForLanguage -- against the shipped
// agda.bin's actual external symbol count and order.
func TestAgdaExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("agda")
	if !ok {
		t.Fatal("missing agda external scanner spec")
	}
	wantExternals := []string{
		"_newline",
		"_indent",
		"_dedent",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("agda spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "e8d47a6987effe34d5595baf321d82d3519a8527"; got != want {
		t.Fatalf("agda spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("agda")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("agda blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := AgdaExternalScanner{}.ExternalScannerForLanguage(lang).(AgdaExternalScanner)
	if !ok {
		t.Fatalf("AgdaExternalScanner binding type = %T, want AgdaExternalScanner", AgdaExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, agdaTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("agda externalToToken = %v, want %v (all %d externals must bind)", got, want, agdaTokenCount)
	}
	if got, want := scanner.symbols, agdaDefaultSymTable; got != want {
		t.Fatalf("agda post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("agda external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("agda external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestArduinoExternalScannerSpecMatchesBlob pins arduinoExternalScannerSpec's
// Externals list -- the binding source for
// ArduinoExternalScanner.ExternalScannerForLanguage -- against the shipped
// arduino.bin's actual external symbol count and order.
func TestArduinoExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("arduino")
	if !ok {
		t.Fatal("missing arduino external scanner spec")
	}
	wantExternals := []string{
		"raw_string_delimiter",
		"raw_string_content",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("arduino spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "11dd46c9ae25135c473c0003a133bb06a484af0c"; got != want {
		t.Fatalf("arduino spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("arduino")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("arduino blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := ArduinoExternalScanner{}.ExternalScannerForLanguage(lang).(ArduinoExternalScanner)
	if !ok {
		t.Fatalf("ArduinoExternalScanner binding type = %T, want ArduinoExternalScanner", ArduinoExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, arduinoTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("arduino externalToToken = %v, want %v (all %d externals must bind)", got, want, arduinoTokenCount)
	}
	if got, want := scanner.symbols, arduinoDefaultSymTable; got != want {
		t.Fatalf("arduino post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("arduino external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("arduino external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestAstroExternalScannerSpecMatchesBlob pins astroExternalScannerSpec's
// Externals list -- the binding source for
// AstroExternalScanner.ExternalScannerForLanguage -- against the shipped
// astro.bin's actual external symbol count and order.
func TestAstroExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("astro")
	if !ok {
		t.Fatal("missing astro external scanner spec")
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
		"_html_interpolation_start",
		"_html_interpolation_end",
		"frontmatter_js_block",
		"attribute_js_expr",
		"attribute_backtick_string",
		"permissible_text",
		"_fragment_tag_delim",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("astro spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "213f6e6973d9b456c6e50e86f19f66877e7ef0ee"; got != want {
		t.Fatalf("astro spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("astro")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("astro blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := AstroExternalScanner{}.ExternalScannerForLanguage(lang).(AstroExternalScanner)
	if !ok {
		t.Fatalf("AstroExternalScanner binding type = %T, want AstroExternalScanner", AstroExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, astroTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("astro externalToToken = %v, want %v (all %d externals must bind)", got, want, astroTokenCount)
	}
	if got, want := scanner.symbols, astroDefaultSymTable; got != want {
		t.Fatalf("astro post-bind symbols = %v, want default table %v", got, want)
	}

	// The three tag_name start variants and the end tag_name each display
	// as "tag_name"; _html_interpolation_start/_end display as the
	// literals "{"/"}"; _fragment_tag_delim displays as the literal ">";
	// the remaining externals display exactly as their spec name.
	wantDisplay := []string{
		"tag_name", "tag_name", "tag_name", "tag_name",
		"erroneous_end_tag_name", "/>", "_implicit_end_tag", "raw_text",
		"comment", "{", "}", "frontmatter_js_block",
		"attribute_js_expr", "attribute_backtick_string", "permissible_text", ">",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("astro external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("astro external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestBicepExternalScannerSpecMatchesBlob pins bicepExternalScannerSpec's
// Externals list -- the binding source for
// BicepExternalScanner.ExternalScannerForLanguage -- against the shipped
// bicep.bin's actual external symbol count and order.
func TestBicepExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("bicep")
	if !ok {
		t.Fatal("missing bicep external scanner spec")
	}
	wantExternals := []string{
		"_external_asterisk",
		"_multiline_string_content",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("bicep spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "bff59884307c0ab009bd5e81afd9324b46a6c0f9"; got != want {
		t.Fatalf("bicep spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("bicep")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("bicep blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := BicepExternalScanner{}.ExternalScannerForLanguage(lang).(BicepExternalScanner)
	if !ok {
		t.Fatalf("BicepExternalScanner binding type = %T, want BicepExternalScanner", BicepExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, bicepTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("bicep externalToToken = %v, want %v (all %d externals must bind)", got, want, bicepTokenCount)
	}
	if got, want := scanner.symbols, bicepDefaultSymTable; got != want {
		t.Fatalf("bicep post-bind symbols = %v, want default table %v", got, want)
	}

	// _external_asterisk displays as the literal "*"; the remaining
	// external displays exactly as its spec name.
	wantDisplay := []string{"*", "_multiline_string_content"}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("bicep external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("bicep external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestBitbakeExternalScannerSpecMatchesBlob pins bbExternalScannerSpec's
// Externals list -- the binding source for
// BitbakeExternalScanner.ExternalScannerForLanguage -- against the shipped
// bitbake.bin's actual external symbol count and order.
func TestBitbakeExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("bitbake")
	if !ok {
		t.Fatal("missing bitbake external scanner spec")
	}
	wantExternals := []string{
		"_concat",
		"_newline",
		"_indent",
		"_dedent",
		"string_start",
		"_string_content",
		"escape_interpolation",
		"string_end",
		"comment",
		"]",
		")",
		"}",
		"shell_content",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("bitbake spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "a5d04fdb5a69a02b8fa8eb5525a60dfb5309b73b"; got != want {
		t.Fatalf("bitbake spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("bitbake")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("bitbake blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := BitbakeExternalScanner{}.ExternalScannerForLanguage(lang).(BitbakeExternalScanner)
	if !ok {
		t.Fatalf("BitbakeExternalScanner binding type = %T, want BitbakeExternalScanner", BitbakeExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, bbTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("bitbake externalToToken = %v, want %v (all %d externals must bind)", got, want, bbTokenCount)
	}
	if got, want := scanner.symbols, bbDefaultSymTable; got != want {
		t.Fatalf("bitbake post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("bitbake external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("bitbake external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestCairoExternalScannerSpecMatchesBlob pins cairoExternalScannerSpec's
// Externals list -- the binding source for
// CairoExternalScanner.ExternalScannerForLanguage -- against the shipped
// cairo.bin's actual external symbol count and order.
func TestCairoExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("cairo")
	if !ok {
		t.Fatal("missing cairo external scanner spec")
	}
	wantExternals := []string{
		"%{",
		"code_line",
		"_failure",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("cairo spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "6238f609bea233040fe927858156dee5515a0745"; got != want {
		t.Fatalf("cairo spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("cairo")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("cairo blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := CairoExternalScanner{}.ExternalScannerForLanguage(lang).(CairoExternalScanner)
	if !ok {
		t.Fatalf("CairoExternalScanner binding type = %T, want CairoExternalScanner", CairoExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, cairoTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("cairo externalToToken = %v, want %v (all %d externals must bind)", got, want, cairoTokenCount)
	}
	if got, want := scanner.symbols, cairoDefaultSymTable; got != want {
		t.Fatalf("cairo post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("cairo external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("cairo external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestCudaExternalScannerSpecMatchesBlob pins cudaExternalScannerSpec's
// Externals list -- the binding source for
// CudaExternalScanner.ExternalScannerForLanguage -- against the shipped
// cuda.bin's actual external symbol count and order.
func TestCudaExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("cuda")
	if !ok {
		t.Fatal("missing cuda external scanner spec")
	}
	wantExternals := []string{
		"raw_string_delimiter",
		"raw_string_content",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("cuda spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "48b066f334f4cf2174e05a50218ce2ed98b6fd01"; got != want {
		t.Fatalf("cuda spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("cuda")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("cuda blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := CudaExternalScanner{}.ExternalScannerForLanguage(lang).(CudaExternalScanner)
	if !ok {
		t.Fatalf("CudaExternalScanner binding type = %T, want CudaExternalScanner", CudaExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, cudaTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("cuda externalToToken = %v, want %v (all %d externals must bind)", got, want, cudaTokenCount)
	}
	if got, want := scanner.symbols, cudaDefaultSymTable; got != want {
		t.Fatalf("cuda post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("cuda external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("cuda external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestCommentExternalScannerSpecMatchesBlob pins commentExternalScannerSpec's
// Externals list -- the binding source for
// CommentExternalScanner.ExternalScannerForLanguage -- against the shipped
// comment.bin's actual external symbol count and order.
func TestCommentExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("comment")
	if !ok {
		t.Fatal("missing comment external scanner spec")
	}
	wantExternals := []string{
		"name",
		"invalid_token",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("comment spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "66272d2b6c73fb61157541b69dd0a7ce7b42a5ad"; got != want {
		t.Fatalf("comment spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("comment")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("comment blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := CommentExternalScanner{}.ExternalScannerForLanguage(lang).(CommentExternalScanner)
	if !ok {
		t.Fatalf("CommentExternalScanner binding type = %T, want CommentExternalScanner", CommentExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, commentTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("comment externalToToken = %v, want %v (all %d externals must bind)", got, want, commentTokenCount)
	}
	if got, want := scanner.symbols, commentDefaultSymTable; got != want {
		t.Fatalf("comment post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("comment external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("comment external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestCooklangExternalScannerSpecMatchesBlob pins cooklangExternalScannerSpec's
// Externals list -- the binding source for
// CooklangExternalScanner.ExternalScannerForLanguage -- against the shipped
// cooklang.bin's actual external symbol count and order.
func TestCooklangExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("cooklang")
	if !ok {
		t.Fatal("missing cooklang external scanner spec")
	}
	wantExternals := []string{
		"_newline",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("cooklang spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "4ebe237c1cf64cf3826fc249e9ec0988fe07e58e"; got != want {
		t.Fatalf("cooklang spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("cooklang")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("cooklang blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := CooklangExternalScanner{}.ExternalScannerForLanguage(lang).(CooklangExternalScanner)
	if !ok {
		t.Fatalf("CooklangExternalScanner binding type = %T, want CooklangExternalScanner", CooklangExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, cooklangTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("cooklang externalToToken = %v, want %v (all %d externals must bind)", got, want, cooklangTokenCount)
	}
	if got, want := scanner.symbols, cooklangDefaultSymTable; got != want {
		t.Fatalf("cooklang post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("cooklang external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("cooklang external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}
