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

// TestCueExternalScannerSpecMatchesBlob pins cueExternalScannerSpec's
// Externals list -- the binding source for
// CueExternalScanner.ExternalScannerForLanguage -- against the shipped
// cue.bin's actual external symbol count and order.
func TestCueExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("cue")
	if !ok {
		t.Fatal("missing cue external scanner spec")
	}
	wantExternals := []string{
		"_multi_str_content",
		"_multi_bytes_content",
		"_raw_str_content",
		"_raw_bytes_content",
		"_multi_raw_str_content",
		"_multi_raw_bytes_content",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("cue spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "be0f609c73cc2929811a9bce0ed90ca71ea87604"; got != want {
		t.Fatalf("cue spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("cue")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("cue blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := CueExternalScanner{}.ExternalScannerForLanguage(lang).(CueExternalScanner)
	if !ok {
		t.Fatalf("CueExternalScanner binding type = %T, want CueExternalScanner", CueExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, cueTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("cue externalToToken = %v, want %v (all %d externals must bind)", got, want, cueTokenCount)
	}
	if got, want := scanner.symbols, cueDefaultSymTable; got != want {
		t.Fatalf("cue post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("cue external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("cue external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestDisassemblyExternalScannerSpecMatchesBlob pins
// disasmExternalScannerSpec's Externals list -- the binding source for
// DisassemblyExternalScanner.ExternalScannerForLanguage -- against the
// shipped disassembly.bin's actual external symbol count and order.
func TestDisassemblyExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("disassembly")
	if !ok {
		t.Fatal("missing disassembly external scanner spec")
	}
	wantExternals := []string{
		"code_identifier",
		"instruction",
		"memory_dump",
		"_error_sentinel",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("disassembly spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "0229c0211dba909c5d45129ac784a3f4d49c243a"; got != want {
		t.Fatalf("disassembly spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("disassembly")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("disassembly blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := DisassemblyExternalScanner{}.ExternalScannerForLanguage(lang).(DisassemblyExternalScanner)
	if !ok {
		t.Fatalf("DisassemblyExternalScanner binding type = %T, want DisassemblyExternalScanner", DisassemblyExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, disasmTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("disassembly externalToToken = %v, want %v (all %d externals must bind)", got, want, disasmTokenCount)
	}
	if got, want := scanner.symbols, disasmDefaultSymTable; got != want {
		t.Fatalf("disassembly post-bind symbols = %v, want default table %v", got, want)
	}

	// code_identifier displays as the grammar-collapsed node name
	// "identifier"; the remaining externals display exactly as their
	// spec name.
	wantDisplay := []string{"identifier", "instruction", "memory_dump", "_error_sentinel"}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("disassembly external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("disassembly external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestDjotExternalScannerSpecMatchesBlob pins djotExternalScannerSpec's
// Externals list -- the binding source for
// DjotExternalScanner.ExternalScannerForLanguage -- against the shipped
// djot.bin's actual external symbol count and order.
func TestDjotExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("djot")
	if !ok {
		t.Fatal("missing djot external scanner spec")
	}
	wantExternals := []string{
		"_ignored",
		"_block_close",
		"_eof_or_newline",
		"_newline",
		"_newline_inline",
		"_non_whitespace_check",
		"hard_line_break",
		"frontmatter_marker",
		"_heading_begin",
		"_heading_continuation",
		"_div_begin",
		"_div_end",
		"_code_block_begin",
		"_code_block_end",
		"list_marker_dash",
		"list_marker_star",
		"list_marker_plus",
		"_list_marker_task_begin",
		"list_marker_definition",
		"list_marker_decimal_period",
		"list_marker_lower_alpha_period",
		"list_marker_upper_alpha_period",
		"list_marker_lower_roman_period",
		"list_marker_upper_roman_period",
		"list_marker_decimal_paren",
		"list_marker_lower_alpha_paren",
		"list_marker_upper_alpha_paren",
		"list_marker_lower_roman_paren",
		"list_marker_upper_roman_paren",
		"list_marker_decimal_parens",
		"list_marker_lower_alpha_parens",
		"list_marker_upper_alpha_parens",
		"list_marker_lower_roman_parens",
		"list_marker_upper_roman_parens",
		"_list_item_continuation",
		"_list_item_end",
		"_indented_content_spacer",
		"_close_paragraph",
		"_block_quote_begin",
		"_block_quote_continuation",
		"_thematic_break_dash",
		"_thematic_break_star",
		"_footnote_mark_begin",
		"_footnote_continuation",
		"_footnote_end",
		"_link_ref_def_mark_begin",
		"_link_ref_def_label_end",
		"_table_header_begin",
		"_table_separator_begin",
		"_table_row_begin",
		"_table_row_end_newline",
		"_table_cell_end",
		"_table_caption_begin",
		"_table_caption_end",
		"_block_attribute_begin",
		"_comment_end_marker",
		"_comment_close",
		"_inline_comment_begin",
		"_verbatim_begin",
		"_verbatim_end",
		"_verbatim_content",
		"_emphasis_mark_begin",
		"emphasis_end",
		"_strong_mark_begin",
		"strong_end",
		"_superscript_mark_begin",
		"superscript_end",
		"_subscript_mark_begin",
		"subscript_end",
		"_highlighted_mark_begin",
		"highlighted_end",
		"_insert_mark_begin",
		"insert_end",
		"_delete_mark_begin",
		"delete_end",
		"_parens_span_mark_begin",
		"_parens_span_end",
		"_curly_bracket_span_mark_begin",
		"_curly_bracket_span_end",
		"_square_bracket_span_mark_begin",
		"_square_bracket_span_end",
		"_in_fallback",
		"_error",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("djot spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "74fac1f53c6d52aeac104b6874e5506be6d0cfe6"; got != want {
		t.Fatalf("djot spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("djot")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("djot blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := DjotExternalScanner{}.ExternalScannerForLanguage(lang).(DjotExternalScanner)
	if !ok {
		t.Fatalf("DjotExternalScanner binding type = %T, want DjotExternalScanner", DjotExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, djotTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("djot externalToToken = %v, want %v (all %d externals must bind)", got, want, djotTokenCount)
	}
	if got, want := scanner.symbols, djotDefaultSymTable; got != want {
		t.Fatalf("djot post-bind symbols = %v, want default table %v", got, want)
	}

	// Many externals collapse onto a small set of shared display node
	// types (heading/table/math/span punctuation markers); this table
	// records the currently-shipped blob's per-index display name
	// directly rather than re-deriving the aliasing rules.
	wantDisplay := []string{
		"_ignored", "_block_close", "_eof_or_newline", "_newline",
		"_newline_inline", "_non_whitespace_check", "hard_line_break", "frontmatter_marker",
		"marker", "marker", "div_marker_begin", "div_marker_end",
		"code_block_marker_begin", "code_block_marker_end", "list_marker_dash", "list_marker_star",
		"list_marker_plus", "_list_marker_task_begin", "list_marker_definition", "list_marker_decimal_period",
		"list_marker_lower_alpha_period", "list_marker_upper_alpha_period", "list_marker_lower_roman_period", "list_marker_upper_roman_period",
		"list_marker_decimal_paren", "list_marker_lower_alpha_paren", "list_marker_upper_alpha_paren", "list_marker_lower_roman_paren",
		"list_marker_upper_roman_paren", "list_marker_decimal_parens", "list_marker_lower_alpha_parens", "list_marker_upper_alpha_parens",
		"list_marker_lower_roman_parens", "list_marker_upper_roman_parens", "_list_item_continuation", "_list_item_end",
		"_indented_content_spacer", "_close_paragraph", "block_quote_marker", "_block_quote_continuation",
		"_thematic_break_dash", "_thematic_break_star", "_footnote_mark_begin", "_footnote_continuation",
		"_footnote_end", "_link_ref_def_mark_begin", "_link_ref_def_label_end", "|",
		"|", "|", "_table_row_end_newline", "|",
		"marker", "_table_caption_end", "{", "%",
		"_comment_close", "_inline_comment_begin", "math_marker_begin", "math_marker_end",
		"content", "_emphasis_mark_begin", "emphasis_end", "_strong_mark_begin",
		"strong_end", "_superscript_mark_begin", "superscript_end", "_subscript_mark_begin",
		"subscript_end", "_highlighted_mark_begin", "highlighted_end", "_insert_mark_begin",
		"insert_end", "_delete_mark_begin", "delete_end", "_parens_span_mark_begin",
		")", "_curly_bracket_span_mark_begin", "}", "_square_bracket_span_mark_begin",
		"]", "_in_fallback", "_error",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("djot external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("djot external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestEarthfileExternalScannerSpecMatchesBlob pins
// earthfileExternalScannerSpec's Externals list -- the binding source for
// EarthfileExternalScanner.ExternalScannerForLanguage -- against the
// shipped earthfile.bin's actual external symbol count and order.
func TestEarthfileExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("earthfile")
	if !ok {
		t.Fatal("missing earthfile external scanner spec")
	}
	wantExternals := []string{
		"_indent",
		"_dedent",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("earthfile spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "5baef88717ad0156fd29a8b12d0d8245bb1096a8"; got != want {
		t.Fatalf("earthfile spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("earthfile")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("earthfile blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := EarthfileExternalScanner{}.ExternalScannerForLanguage(lang).(EarthfileExternalScanner)
	if !ok {
		t.Fatalf("EarthfileExternalScanner binding type = %T, want EarthfileExternalScanner", EarthfileExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, earthfileTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("earthfile externalToToken = %v, want %v (all %d externals must bind)", got, want, earthfileTokenCount)
	}
	if got, want := scanner.symbols, earthfileDefaultSymTable; got != want {
		t.Fatalf("earthfile post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("earthfile external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("earthfile external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestDtdExternalScannerSpecMatchesBlob pins dtdExternalScannerSpec's
// Externals list -- the binding source for
// DtdExternalScanner.ExternalScannerForLanguage -- against the shipped
// dtd.bin's actual external symbol count and order.
func TestDtdExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("dtd")
	if !ok {
		t.Fatal("missing dtd external scanner spec")
	}
	wantExternals := []string{
		"PITarget",
		"_pi_content",
		"Comment",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("dtd spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "5000ae8f22d11fbe93939b05c1e37cf21117162d"; got != want {
		t.Fatalf("dtd spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("dtd")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("dtd blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := DtdExternalScanner{}.ExternalScannerForLanguage(lang).(DtdExternalScanner)
	if !ok {
		t.Fatalf("DtdExternalScanner binding type = %T, want DtdExternalScanner", DtdExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, dtdTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("dtd externalToToken = %v, want %v (all %d externals must bind)", got, want, dtdTokenCount)
	}
	if got, want := scanner.symbols, dtdDefaultSymTable; got != want {
		t.Fatalf("dtd post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("dtd external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("dtd external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestDockerfileExternalScannerSpecMatchesBlob pins
// dockerfileExternalScannerSpec's Externals list -- the binding source for
// DockerfileExternalScanner.ExternalScannerForLanguage -- against the
// shipped dockerfile.bin's actual external symbol count and order.
func TestDockerfileExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("dockerfile")
	if !ok {
		t.Fatal("missing dockerfile external scanner spec")
	}
	wantExternals := []string{
		"heredoc_marker",
		"heredoc_line",
		"heredoc_end",
		"heredoc_nl",
		"error_sentinel",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("dockerfile spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "971acdd908568b4531b0ba28a445bf0bb720aba5"; got != want {
		t.Fatalf("dockerfile spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("dockerfile")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("dockerfile blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := DockerfileExternalScanner{}.ExternalScannerForLanguage(lang).(DockerfileExternalScanner)
	if !ok {
		t.Fatalf("DockerfileExternalScanner binding type = %T, want DockerfileExternalScanner", DockerfileExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, dockerfileTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("dockerfile externalToToken = %v, want %v (all %d externals must bind)", got, want, dockerfileTokenCount)
	}
	if got, want := scanner.symbols, dockerfileDefaultSymTable; got != want {
		t.Fatalf("dockerfile post-bind symbols = %v, want default table %v", got, want)
	}

	// heredoc_nl (external index 3) aliases to the grammar-internal display
	// node "_heredoc_nl"; the remaining four externals display exactly as
	// their spec name.
	wantDisplay := []string{
		"heredoc_marker",
		"heredoc_line",
		"heredoc_end",
		"_heredoc_nl",
		"error_sentinel",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("dockerfile external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("dockerfile external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestFennelExternalScannerSpecMatchesBlob pins fennelExternalScannerSpec's
// Externals list -- the binding source for
// FennelExternalScanner.ExternalScannerForLanguage -- against the shipped
// fennel.bin's actual external symbol count and order.
func TestFennelExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("fennel")
	if !ok {
		t.Fatal("missing fennel external scanner spec")
	}
	wantExternals := []string{
		"_hashfn_reader_macro_char",
		"_quote_reader_macro_char",
		"_quasi_quote_reader_macro_char",
		"_unquote_reader_macro_char",
		"__reader_macro_count",
		"__colon_string_start_mark",
		"__colon_string_end_mark",
		"shebang",
		"__token_count",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("fennel spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "3f0f6b24d599e92460b969aabc4f4c5a914d15a0"; got != want {
		t.Fatalf("fennel spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("fennel")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("fennel blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := FennelExternalScanner{}.ExternalScannerForLanguage(lang).(FennelExternalScanner)
	if !ok {
		t.Fatalf("FennelExternalScanner binding type = %T, want FennelExternalScanner", FennelExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, fennelTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("fennel externalToToken = %v, want %v (all %d externals must bind)", got, want, fennelTokenCount)
	}
	if got, want := scanner.symbols, fennelDefaultSymTable; got != want {
		t.Fatalf("fennel post-bind symbols = %v, want default table %v", got, want)
	}

	// The four reader-macro externals display as their literal character;
	// the remaining five display exactly as their spec name.
	wantDisplay := []string{
		"#", "'", "`", ",",
		"__reader_macro_count", "__colon_string_start_mark", "__colon_string_end_mark",
		"shebang", "__token_count",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("fennel external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("fennel external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestCobolExternalScannerSpecMatchesBlob pins cobolExternalScannerSpec's
// Externals list -- the binding source for
// CobolExternalScanner.ExternalScannerForLanguage -- against the shipped
// cobol.bin's actual external symbol count and order.
func TestCobolExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("cobol")
	if !ok {
		t.Fatal("missing cobol external scanner spec")
	}
	wantExternals := []string{
		"_WHITE_SPACES",
		"_LINE_PREFIX_COMMENT",
		"_LINE_SUFFIX_COMMENT",
		"_LINE_COMMENT",
		"comment_entry",
		"_multiline_string",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("cobol spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "e99dbdc3d800d5fa2796476efd60af91f6b43d93"; got != want {
		t.Fatalf("cobol spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("cobol")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("cobol blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := CobolExternalScanner{}.ExternalScannerForLanguage(lang).(CobolExternalScanner)
	if !ok {
		t.Fatalf("CobolExternalScanner binding type = %T, want CobolExternalScanner", CobolExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, cobolTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("cobol externalToToken = %v, want %v (all %d externals must bind)", got, want, cobolTokenCount)
	}
	if got, want := scanner.symbols, cobolDefaultSymTable; got != want {
		t.Fatalf("cobol post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("cobol external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("cobol external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestCrystalExternalScannerSpecMatchesBlob pins cryExternalScannerSpec's
// Externals list -- the binding source for
// CrystalExternalScanner.ExternalScannerForLanguage -- against the shipped
// crystal.bin's actual external symbol count and order.
func TestCrystalExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("crystal")
	if !ok {
		t.Fatal("missing crystal external scanner spec")
	}
	wantExternals := []string{
		"_line_break",
		"_line_continuation",
		"_start_of_brace_block",
		"_start_of_hash_or_tuple",
		"_start_of_named_tuple",
		"_start_of_tuple_type",
		"_start_of_named_tuple_type",
		"_start_of_index_operator",
		"_end_of_with_expression",
		"unary_plus",
		"unary_minus",
		"binary_plus",
		"binary_minus",
		"unary_wrapping_plus",
		"unary_wrapping_minus",
		"binary_wrapping_plus",
		"binary_wrapping_minus",
		"_unary_star",
		"_binary_star",
		"_unary_double_star",
		"_binary_double_star",
		"_block_ampersand",
		"binary_ampersand",
		"_beginless_range_operator",
		"_regex_start",
		"_binary_slash",
		"_binary_double_slash",
		"_regular_if_keyword",
		"_modifier_if_keyword",
		"_regular_unless_keyword",
		"_modifier_unless_keyword",
		"_regular_rescue_keyword",
		"_modifier_rescue_keyword",
		"_regular_ensure_keyword",
		"_modifier_ensure_keyword",
		"_modulo_operator",
		"_string_literal_start",
		"_delimited_string_contents",
		"_string_literal_end",
		"_string_percent_literal_start",
		"_command_percent_literal_start",
		"_string_array_percent_literal_start",
		"_symbol_array_percent_literal_start",
		"_regex_percent_literal_start",
		"_percent_literal_end",
		"_delimited_array_element_start",
		"_delimited_array_element_end",
		"heredoc_start",
		"_heredoc_body_start",
		"heredoc_content",
		"heredoc_end",
		"regex_modifier",
		"_start_of_parenless_args",
		"_end_of_range",
		"_error_recovery",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("crystal spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "51ad1411de9414b4600227553bb70953c352a627"; got != want {
		t.Fatalf("crystal spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("crystal")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("crystal blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := CrystalExternalScanner{}.ExternalScannerForLanguage(lang).(CrystalExternalScanner)
	if !ok {
		t.Fatalf("CrystalExternalScanner binding type = %T, want CrystalExternalScanner", CrystalExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, cryTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("crystal externalToToken = %v, want %v (all %d externals must bind)", got, want, cryTokenCount)
	}
	if got, want := scanner.symbols, cryDefaultSymTable; got != want {
		t.Fatalf("crystal post-bind symbols = %v, want default table %v", got, want)
	}

	// Many externals collapse onto a small set of shared display node names
	// ("{", "operator", keyword text, and so on); this table records the
	// currently-shipped blob's actual per-index display name directly
	// rather than re-deriving the aliasing rules.
	wantDisplay := []string{
		"_line_break", "_line_continuation", "{", "{", "{", "{", "{", "[",
		"_end_of_with_expression", "+", "-", "operator", "operator", "operator",
		"operator", "operator", "operator", "*", "operator", "**", "operator",
		"&", "operator", "operator", "/", "operator", "operator", "if", "if",
		"unless", "unless", "rescue", "rescue", "ensure", "ensure", "operator",
		"_string_literal_start", "_delimited_string_contents", "_string_literal_end",
		"_string_percent_literal_start", "_command_percent_literal_start",
		"_string_array_percent_literal_start", "_symbol_array_percent_literal_start",
		"_regex_percent_literal_start", "_percent_literal_end",
		"_delimited_array_element_start", "_delimited_array_element_end",
		"heredoc_start", "_heredoc_body_start", "heredoc_content", "heredoc_end",
		"regex_modifier", "_start_of_parenless_args", "_end_of_range", "_error_recovery",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("crystal external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("crystal external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestDhallExternalScannerSpecMatchesBlob pins dhallExternalScannerSpec's
// Externals list -- the binding source for
// DhallExternalScanner.ExternalScannerForLanguage -- against the shipped
// dhall.bin's actual external symbol count and order.
func TestDhallExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("dhall")
	if !ok {
		t.Fatal("missing dhall external scanner spec")
	}
	wantExternals := []string{
		"block_comment_content",
		"block_comment_end",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("dhall spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "62013259b26ac210d5de1abf64cf1b047ef88000"; got != want {
		t.Fatalf("dhall spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("dhall")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("dhall blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := DhallExternalScanner{}.ExternalScannerForLanguage(lang).(DhallExternalScanner)
	if !ok {
		t.Fatalf("DhallExternalScanner binding type = %T, want DhallExternalScanner", DhallExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, dhallTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("dhall externalToToken = %v, want %v (all %d externals must bind)", got, want, dhallTokenCount)
	}
	if got, want := scanner.symbols, dhallDefaultSymTable; got != want {
		t.Fatalf("dhall post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("dhall external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("dhall external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestFirrtlExternalScannerSpecMatchesBlob pins firrtlExternalScannerSpec's
// Externals list -- the binding source for
// FirrtlExternalScanner.ExternalScannerForLanguage -- against the shipped
// firrtl.bin's actual external symbol count and order.
func TestFirrtlExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("firrtl")
	if !ok {
		t.Fatal("missing firrtl external scanner spec")
	}
	wantExternals := []string{
		"_newline",
		"_indent",
		"_dedent",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("firrtl spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "8503d3a0fe0f9e427863cb0055699ff2d29ae5f5"; got != want {
		t.Fatalf("firrtl spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("firrtl")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("firrtl blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := FirrtlExternalScanner{}.ExternalScannerForLanguage(lang).(FirrtlExternalScanner)
	if !ok {
		t.Fatalf("FirrtlExternalScanner binding type = %T, want FirrtlExternalScanner", FirrtlExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, firrtlTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("firrtl externalToToken = %v, want %v (all %d externals must bind)", got, want, firrtlTokenCount)
	}
	if got, want := scanner.symbols, firrtlDefaultSymTable; got != want {
		t.Fatalf("firrtl post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("firrtl external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("firrtl external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestFishExternalScannerSpecMatchesBlob pins fishExternalScannerSpec's
// Externals list -- the binding source for
// FishExternalScanner.ExternalScannerForLanguage -- against the shipped
// fish.bin's actual external symbol count and order.
func TestFishExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("fish")
	if !ok {
		t.Fatal("missing fish external scanner spec")
	}
	wantExternals := []string{
		"_concat",
		"_brace_concat",
		"_concat_list",
		"_begin_brace",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("fish spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "fa2143f5d66a9eb6c007ba9173525ea7aaafe788"; got != want {
		t.Fatalf("fish spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("fish")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("fish blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := FishExternalScanner{}.ExternalScannerForLanguage(lang).(FishExternalScanner)
	if !ok {
		t.Fatalf("FishExternalScanner binding type = %T, want FishExternalScanner", FishExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, fishTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("fish externalToToken = %v, want %v (all %d externals must bind)", got, want, fishTokenCount)
	}
	if got, want := scanner.symbols, fishDefaultSymTable; got != want {
		t.Fatalf("fish post-bind symbols = %v, want default table %v", got, want)
	}

	// _begin_brace (external index 3) aliases to the grammar-internal
	// display node "{"; the remaining three externals display exactly as
	// their spec name.
	wantDisplay := []string{
		"_concat",
		"_brace_concat",
		"_concat_list",
		"{",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("fish external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("fish external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestFoamExternalScannerSpecMatchesBlob pins foamExternalScannerSpec's
// Externals list -- the binding source for
// FoamExternalScanner.ExternalScannerForLanguage -- against the shipped
// foam.bin's actual external symbol count and order.
func TestFoamExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("foam")
	if !ok {
		t.Fatal("missing foam external scanner spec")
	}
	wantExternals := []string{
		"identifier",
		"boolean",
		"_eof",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("foam spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "472c24f11a547820327fb1be565bcfff98ea96a4"; got != want {
		t.Fatalf("foam spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("foam")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("foam blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := FoamExternalScanner{}.ExternalScannerForLanguage(lang).(FoamExternalScanner)
	if !ok {
		t.Fatalf("FoamExternalScanner binding type = %T, want FoamExternalScanner", FoamExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, foamTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("foam externalToToken = %v, want %v (all %d externals must bind)", got, want, foamTokenCount)
	}
	if got, want := scanner.symbols, foamDefaultSymTable; got != want {
		t.Fatalf("foam post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("foam external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("foam external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestFortranExternalScannerSpecMatchesBlob pins ftnExternalScannerSpec's
// Externals list -- the binding source for
// FortranExternalScanner.ExternalScannerForLanguage -- against the shipped
// fortran.bin's actual external symbol count and order.
func TestFortranExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("fortran")
	if !ok {
		t.Fatal("missing fortran external scanner spec")
	}
	wantExternals := []string{
		"&",
		"_integer_literal",
		"_float_literal",
		"_boz_literal",
		"_string_literal",
		"_string_literal_kind",
		"_external_end_of_statement",
		"_preproc_unary_operator",
		"hollerith_constant",
		"_do_label",
		"do_label_virtual",
		"_do_label_continue",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("fortran spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "2880b7aab4fb7cc618de1ef3d4c6d93b2396c031"; got != want {
		t.Fatalf("fortran spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("fortran")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("fortran blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := FortranExternalScanner{}.ExternalScannerForLanguage(lang).(FortranExternalScanner)
	if !ok {
		t.Fatalf("FortranExternalScanner binding type = %T, want FortranExternalScanner", FortranExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, ftnTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("fortran externalToToken = %v, want %v (all %d externals must bind)", got, want, ftnTokenCount)
	}
	if got, want := scanner.symbols, ftnDefaultSymTable; got != want {
		t.Fatalf("fortran post-bind symbols = %v, want default table %v", got, want)
	}

	// _string_literal_kind (index 5) displays as "identifier",
	// _do_label (index 9) displays as "statement_label_reference", and
	// _do_label_continue (index 11) displays as "statement_label"; the
	// remaining externals display exactly as their spec name.
	wantDisplay := []string{
		"&",
		"_integer_literal",
		"_float_literal",
		"_boz_literal",
		"_string_literal",
		"identifier",
		"_external_end_of_statement",
		"_preproc_unary_operator",
		"hollerith_constant",
		"statement_label_reference",
		"do_label_virtual",
		"statement_label",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("fortran external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("fortran external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestFsharpExternalScannerSpecMatchesBlob pins fsExternalScannerSpec's
// Externals list -- the binding source for
// FsharpExternalScanner.ExternalScannerForLanguage -- against the shipped
// fsharp.bin's actual external symbol count and order.
func TestFsharpExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("fsharp")
	if !ok {
		t.Fatal("missing fsharp external scanner spec")
	}
	wantExternals := []string{
		"_newline",
		"_indent",
		"_dedent",
		"then",
		"else",
		"elif",
		"#if",
		"#else",
		"#endif",
		"class",
		"_struct_begin",
		"_interface_begin",
		"end",
		"and",
		"with",
		"_triple_quoted_content",
		"block_comment_content",
		"_inside_string_marker",
		"_newline_not_aligned",
		"_tuple_marker",
		"_error_sentinel",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("fsharp spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "5141851c278a99958469eb1736c7afc4ec738e47"; got != want {
		t.Fatalf("fsharp spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("fsharp")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("fsharp blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := FsharpExternalScanner{}.ExternalScannerForLanguage(lang).(FsharpExternalScanner)
	if !ok {
		t.Fatalf("FsharpExternalScanner binding type = %T, want FsharpExternalScanner", FsharpExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, fsTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("fsharp externalToToken = %v, want %v (all %d externals must bind)", got, want, fsTokenCount)
	}
	if got, want := scanner.symbols, fsDefaultSymTable; got != want {
		t.Fatalf("fsharp post-bind symbols = %v, want default table %v", got, want)
	}

	// _struct_begin displays as "struct" and _interface_begin displays as
	// "interface"; the remaining externals display exactly as their spec
	// name (literal-valued externals display as their literal text).
	wantDisplay := []string{
		"_newline",
		"_indent",
		"_dedent",
		"then",
		"else",
		"elif",
		"#if",
		"#else",
		"#endif",
		"class",
		"struct",
		"interface",
		"end",
		"and",
		"with",
		"_triple_quoted_content",
		"block_comment_content",
		"_inside_string_marker",
		"_newline_not_aligned",
		"_tuple_marker",
		"_error_sentinel",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("fsharp external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("fsharp external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestGdscriptExternalScannerSpecMatchesBlob pins gdsExternalScannerSpec's
// Externals list -- the binding source for
// GdscriptExternalScanner.ExternalScannerForLanguage -- against the shipped
// gdscript.bin's actual external symbol count and order.
func TestGdscriptExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("gdscript")
	if !ok {
		t.Fatal("missing gdscript external scanner spec")
	}
	wantExternals := []string{
		"_newline",
		"_indent",
		"_dedent",
		"_string_start",
		"_string_content",
		"_string_end",
		"_string_name_start",
		"_node_path_start",
		"]",
		")",
		"}",
		",",
		"_body_end",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("gdscript spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "89e66b6bdc002ab976283f277cbb48b780c5d0e9"; got != want {
		t.Fatalf("gdscript spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("gdscript")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("gdscript blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := GdscriptExternalScanner{}.ExternalScannerForLanguage(lang).(GdscriptExternalScanner)
	if !ok {
		t.Fatalf("GdscriptExternalScanner binding type = %T, want GdscriptExternalScanner", GdscriptExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, gdsTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("gdscript externalToToken = %v, want %v (all %d externals must bind)", got, want, gdsTokenCount)
	}
	if got, want := scanner.symbols, gdsDefaultSymTable; got != want {
		t.Fatalf("gdscript post-bind symbols = %v, want default table %v", got, want)
	}

	// _string_start/_string_end display as the literal quote character,
	// _string_name_start/_node_path_start display with their sigil prefix,
	// and the four close-bracket externals (indexes 8-11) alias to shared
	// literal display nodes; the remaining externals display exactly as
	// their spec name.
	wantDisplay := []string{
		"_newline",
		"_indent",
		"_dedent",
		"\"",
		"_string_content",
		"\"",
		"&\"",
		"^\"",
		"]",
		")",
		"}",
		",",
		"_body_end",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("gdscript external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("gdscript external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestGitcommitExternalScannerSpecMatchesBlob pins
// gitcommitExternalScannerSpec's Externals list -- the binding source for
// GitcommitExternalScanner.ExternalScannerForLanguage -- against the
// shipped gitcommit.bin's actual external symbol count and order.
func TestGitcommitExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("gitcommit")
	if !ok {
		t.Fatal("missing gitcommit external scanner spec")
	}
	wantExternals := []string{
		"_conventional_type",
		"_trailer_value",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("gitcommit spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "a716678c0f00645fed1e6f1d0eb221481dbd6f6d"; got != want {
		t.Fatalf("gitcommit spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("gitcommit")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("gitcommit blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := GitcommitExternalScanner{}.ExternalScannerForLanguage(lang).(GitcommitExternalScanner)
	if !ok {
		t.Fatalf("GitcommitExternalScanner binding type = %T, want GitcommitExternalScanner", GitcommitExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, gitcommitTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("gitcommit externalToToken = %v, want %v (all %d externals must bind)", got, want, gitcommitTokenCount)
	}
	if got, want := scanner.symbols, gitcommitDefaultSymTable; got != want {
		t.Fatalf("gitcommit post-bind symbols = %v, want default table %v", got, want)
	}

	// _conventional_type displays as "type"; _trailer_value displays
	// exactly as its spec name.
	wantDisplay := []string{
		"type",
		"_trailer_value",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("gitcommit external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("gitcommit external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestGleamExternalScannerSpecMatchesBlob pins gleamExternalScannerSpec's
// Externals list -- the binding source for
// GleamExternalScanner.ExternalScannerForLanguage -- against the shipped
// gleam.bin's actual external symbol count and order.
func TestGleamExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("gleam")
	if !ok {
		t.Fatal("missing gleam external scanner spec")
	}
	wantExternals := []string{
		"quoted_content",
		"doc_comment_content",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("gleam spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "6ea757f7eb8d391dbf24dbb9461990757946dd5e"; got != want {
		t.Fatalf("gleam spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("gleam")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("gleam blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := GleamExternalScanner{}.ExternalScannerForLanguage(lang).(GleamExternalScanner)
	if !ok {
		t.Fatalf("GleamExternalScanner binding type = %T, want GleamExternalScanner", GleamExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, gleamTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("gleam externalToToken = %v, want %v (all %d externals must bind)", got, want, gleamTokenCount)
	}
	if got, want := scanner.symbols, gleamDefaultSymTable; got != want {
		t.Fatalf("gleam post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("gleam external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("gleam external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestGnExternalScannerSpecMatchesBlob pins gnExternalScannerSpec's
// Externals list -- the binding source for
// GnExternalScanner.ExternalScannerForLanguage -- against the shipped
// gn.bin's actual external symbol count and order.
func TestGnExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("gn")
	if !ok {
		t.Fatal("missing gn external scanner spec")
	}
	wantExternals := []string{
		"_string_content",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("gn spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "bc06955bc1e3c9ff8e9b2b2a55b38b94da923c05"; got != want {
		t.Fatalf("gn spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("gn")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("gn blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := GnExternalScanner{}.ExternalScannerForLanguage(lang).(GnExternalScanner)
	if !ok {
		t.Fatalf("GnExternalScanner binding type = %T, want GnExternalScanner", GnExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, gnTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("gn externalToToken = %v, want %v (all %d externals must bind)", got, want, gnTokenCount)
	}
	if got, want := scanner.symbols, gnDefaultSymTable; got != want {
		t.Fatalf("gn post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("gn external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("gn external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestGodotResourceExternalScannerSpecMatchesBlob pins
// godotResourceExternalScannerSpec's Externals list -- the binding source
// for GodotResourceExternalScanner.ExternalScannerForLanguage -- against
// the shipped godot_resource.bin's actual external symbol count and order.
func TestGodotResourceExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("godot_resource")
	if !ok {
		t.Fatal("missing godot_resource external scanner spec")
	}
	wantExternals := []string{
		"string",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("godot_resource spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "302c1895f54bf74d53a08572f7b26a6614209adc"; got != want {
		t.Fatalf("godot_resource spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("godot_resource")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("godot_resource blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := GodotResourceExternalScanner{}.ExternalScannerForLanguage(lang).(GodotResourceExternalScanner)
	if !ok {
		t.Fatalf("GodotResourceExternalScanner binding type = %T, want GodotResourceExternalScanner", GodotResourceExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, godotResourceTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("godot_resource externalToToken = %v, want %v (all %d externals must bind)", got, want, godotResourceTokenCount)
	}
	if got, want := scanner.symbols, godotResourceDefaultSymTable; got != want {
		t.Fatalf("godot_resource post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("godot_resource external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("godot_resource external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestHackExternalScannerSpecMatchesBlob pins hackExternalScannerSpec's
// Externals list -- the binding source for
// HackExternalScanner.ExternalScannerForLanguage -- against the shipped
// hack.bin's actual external symbol count and order.
func TestHackExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("hack")
	if !ok {
		t.Fatal("missing hack external scanner spec")
	}
	wantExternals := []string{
		"_heredoc_start",
		"_heredoc_start_newline",
		"_heredoc_body",
		"_heredoc_end_newline",
		"_heredoc_end",
		"_embedded_opening_brace",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("hack spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "1a7ded90288189746c54861ac144ede97df95081"; got != want {
		t.Fatalf("hack spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("hack")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("hack blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := HackExternalScanner{}.ExternalScannerForLanguage(lang).(HackExternalScanner)
	if !ok {
		t.Fatalf("HackExternalScanner binding type = %T, want HackExternalScanner", HackExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, hackTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("hack externalToToken = %v, want %v (all %d externals must bind)", got, want, hackTokenCount)
	}
	if got, want := scanner.symbols, hackDefaultSymTable; got != want {
		t.Fatalf("hack post-bind symbols = %v, want default table %v", got, want)
	}

	// _heredoc_start_newline and _heredoc_end_newline display as "\n", and
	// _embedded_opening_brace displays as "{"; the remaining externals
	// display exactly as their spec name.
	wantDisplay := []string{
		"_heredoc_start",
		"\n",
		"_heredoc_body",
		"\n",
		"_heredoc_end",
		"{",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("hack external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("hack external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestHaxeExternalScannerSpecMatchesBlob pins haxeExternalScannerSpec's
// Externals list -- the binding source for
// HaxeExternalScanner.ExternalScannerForLanguage -- against the shipped
// haxe.bin's actual external symbol count and order.
func TestHaxeExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("haxe")
	if !ok {
		t.Fatal("missing haxe external scanner spec")
	}
	wantExternals := []string{
		"_lookback_semicolon",
		"_closing_brace_marker",
		"_closing_brace_unmarker",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("haxe spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "f2a2394d9ca7a6099f78d8b0d178530e7c9a8e26"; got != want {
		t.Fatalf("haxe spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("haxe")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("haxe blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := HaxeExternalScanner{}.ExternalScannerForLanguage(lang).(HaxeExternalScanner)
	if !ok {
		t.Fatalf("HaxeExternalScanner binding type = %T, want HaxeExternalScanner", HaxeExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, haxeTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("haxe externalToToken = %v, want %v (all %d externals must bind)", got, want, haxeTokenCount)
	}
	if got, want := scanner.symbols, haxeDefaultSymTable; got != want {
		t.Fatalf("haxe post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("haxe external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("haxe external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestHlslExternalScannerSpecMatchesBlob pins hlslExternalScannerSpec's
// Externals list -- the binding source for
// HlslExternalScanner.ExternalScannerForLanguage -- against the shipped
// hlsl.bin's actual external symbol count and order.
func TestHlslExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("hlsl")
	if !ok {
		t.Fatal("missing hlsl external scanner spec")
	}
	wantExternals := []string{
		"raw_string_delimiter",
		"raw_string_content",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("hlsl spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "bab9111922d53d43668fabb61869bec51bbcb915"; got != want {
		t.Fatalf("hlsl spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("hlsl")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("hlsl blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := HlslExternalScanner{}.ExternalScannerForLanguage(lang).(HlslExternalScanner)
	if !ok {
		t.Fatalf("HlslExternalScanner binding type = %T, want HlslExternalScanner", HlslExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, hlslTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("hlsl externalToToken = %v, want %v (all %d externals must bind)", got, want, hlslTokenCount)
	}
	if got, want := scanner.symbols, hlslDefaultSymTable; got != want {
		t.Fatalf("hlsl post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("hlsl external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("hlsl external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestJanetExternalScannerSpecMatchesBlob pins janetExternalScannerSpec's
// Externals list -- the binding source for
// JanetExternalScanner.ExternalScannerForLanguage -- against the shipped
// janet.bin's actual external symbol count and order.
func TestJanetExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("janet")
	if !ok {
		t.Fatal("missing janet external scanner spec")
	}
	wantExternals := []string{
		"long_buf_lit",
		"long_str_lit",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("janet spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "d183186995204314700be3e9e0a48053ea16b350"; got != want {
		t.Fatalf("janet spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("janet")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("janet blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := JanetExternalScanner{}.ExternalScannerForLanguage(lang).(JanetExternalScanner)
	if !ok {
		t.Fatalf("JanetExternalScanner binding type = %T, want JanetExternalScanner", JanetExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, janetTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("janet externalToToken = %v, want %v (all %d externals must bind)", got, want, janetTokenCount)
	}
	if got, want := scanner.symbols, janetDefaultSymTable; got != want {
		t.Fatalf("janet post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("janet external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("janet external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestJsdocExternalScannerSpecMatchesBlob pins jsdocExternalScannerSpec's
// Externals list -- the binding source for
// JsdocExternalScanner.ExternalScannerForLanguage -- against the shipped
// jsdoc.bin's actual external symbol count and order.
func TestJsdocExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("jsdoc")
	if !ok {
		t.Fatal("missing jsdoc external scanner spec")
	}
	wantExternals := []string{
		"type",
		"code_block_line",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("jsdoc spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "658d18dcdddb75c760363faa4963427a7c6b52db"; got != want {
		t.Fatalf("jsdoc spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("jsdoc")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("jsdoc blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := JsdocExternalScanner{}.ExternalScannerForLanguage(lang).(JsdocExternalScanner)
	if !ok {
		t.Fatalf("JsdocExternalScanner binding type = %T, want JsdocExternalScanner", JsdocExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, jsdocTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("jsdoc externalToToken = %v, want %v (all %d externals must bind)", got, want, jsdocTokenCount)
	}
	if got, want := scanner.symbols, jsdocDefaultSymTable; got != want {
		t.Fatalf("jsdoc post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("jsdoc external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("jsdoc external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestJsonnetExternalScannerSpecMatchesBlob pins
// jsonnetExternalScannerSpec's Externals list -- the binding source for
// JsonnetExternalScanner.ExternalScannerForLanguage -- against the
// shipped jsonnet.bin's actual external symbol count and order.
func TestJsonnetExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("jsonnet")
	if !ok {
		t.Fatal("missing jsonnet external scanner spec")
	}
	wantExternals := []string{
		"_string_start",
		"_string_content",
		"_string_end",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("jsonnet spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "ddd075f1939aed8147b7aa67f042eda3fce22790"; got != want {
		t.Fatalf("jsonnet spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("jsonnet")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("jsonnet blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := JsonnetExternalScanner{}.ExternalScannerForLanguage(lang).(JsonnetExternalScanner)
	if !ok {
		t.Fatalf("JsonnetExternalScanner binding type = %T, want JsonnetExternalScanner", JsonnetExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, jsonnetTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("jsonnet externalToToken = %v, want %v (all %d externals must bind)", got, want, jsonnetTokenCount)
	}
	if got, want := scanner.symbols, jsonnetDefaultSymTable; got != want {
		t.Fatalf("jsonnet post-bind symbols = %v, want default table %v", got, want)
	}

	// All three externals drop their leading underscore in the blob's
	// display name.
	wantDisplay := []string{
		"string_start",
		"string_content",
		"string_end",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("jsonnet external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("jsonnet external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestJustExternalScannerSpecMatchesBlob pins justExternalScannerSpec's
// Externals list -- the binding source for
// JustExternalScanner.ExternalScannerForLanguage -- against the shipped
// just.bin's actual external symbol count and order.
func TestJustExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("just")
	if !ok {
		t.Fatal("missing just external scanner spec")
	}
	wantExternals := []string{
		"_indent",
		"_dedent",
		"_newline",
		"text",
		"error_recovery",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("just spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "60df3d5b3fda2a22fdb3621226cafab50b763663"; got != want {
		t.Fatalf("just spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("just")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("just blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := JustExternalScanner{}.ExternalScannerForLanguage(lang).(JustExternalScanner)
	if !ok {
		t.Fatalf("JustExternalScanner binding type = %T, want JustExternalScanner", JustExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, justTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("just externalToToken = %v, want %v (all %d externals must bind)", got, want, justTokenCount)
	}
	if got, want := scanner.symbols, justDefaultSymTable; got != want {
		t.Fatalf("just post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("just external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("just external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestKconfigExternalScannerSpecMatchesBlob pins
// kconfigExternalScannerSpec's Externals list -- the binding source for
// KconfigExternalScanner.ExternalScannerForLanguage -- against the
// shipped kconfig.bin's actual external symbol count and order.
func TestKconfigExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("kconfig")
	if !ok {
		t.Fatal("missing kconfig external scanner spec")
	}
	wantExternals := []string{
		"_help_text",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("kconfig spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "9ac99fe4c0c27a35dc6f757cef534c646e944881"; got != want {
		t.Fatalf("kconfig spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("kconfig")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("kconfig blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := KconfigExternalScanner{}.ExternalScannerForLanguage(lang).(KconfigExternalScanner)
	if !ok {
		t.Fatalf("KconfigExternalScanner binding type = %T, want KconfigExternalScanner", KconfigExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, kconfigTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("kconfig externalToToken = %v, want %v (all %d externals must bind)", got, want, kconfigTokenCount)
	}
	if got, want := scanner.symbols, kconfigDefaultSymTable; got != want {
		t.Fatalf("kconfig post-bind symbols = %v, want default table %v", got, want)
	}

	// _help_text displays as "text".
	wantDisplay := []string{
		"text",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("kconfig external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("kconfig external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestKdlExternalScannerSpecMatchesBlob pins kdlExternalScannerSpec's
// Externals list -- the binding source for
// KdlExternalScanner.ExternalScannerForLanguage -- against the shipped
// kdl.bin's actual external symbol count and order.
func TestKdlExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("kdl")
	if !ok {
		t.Fatal("missing kdl external scanner spec")
	}
	wantExternals := []string{
		"_eof",
		"multi_line_comment",
		"_raw_string",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("kdl spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "b37e3d58e5c5cf8d739b315d6114e02d42e66664"; got != want {
		t.Fatalf("kdl spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("kdl")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("kdl blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := KdlExternalScanner{}.ExternalScannerForLanguage(lang).(KdlExternalScanner)
	if !ok {
		t.Fatalf("KdlExternalScanner binding type = %T, want KdlExternalScanner", KdlExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, kdlTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("kdl externalToToken = %v, want %v (all %d externals must bind)", got, want, kdlTokenCount)
	}
	if got, want := scanner.symbols, kdlDefaultSymTable; got != want {
		t.Fatalf("kdl post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("kdl external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("kdl external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestLessExternalScannerSpecMatchesBlob pins lessExternalScannerSpec's
// Externals list -- the binding source for
// LessExternalScanner.ExternalScannerForLanguage -- against the shipped
// less.bin's actual external symbol count and order.
func TestLessExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("less")
	if !ok {
		t.Fatal("missing less external scanner spec")
	}
	wantExternals := []string{
		"_descendant_operator",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("less spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "2bd739e106a3485bca210cf7b6d25ba09fd10dff"; got != want {
		t.Fatalf("less spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("less")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("less blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := LessExternalScanner{}.ExternalScannerForLanguage(lang).(LessExternalScanner)
	if !ok {
		t.Fatalf("LessExternalScanner binding type = %T, want LessExternalScanner", LessExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, lessTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("less externalToToken = %v, want %v (all %d externals must bind)", got, want, lessTokenCount)
	}
	if got, want := scanner.symbols, lessDefaultSymTable; got != want {
		t.Fatalf("less post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("less external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("less external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestLuauExternalScannerSpecMatchesBlob pins luauExternalScannerSpec's
// Externals list -- the binding source for
// LuauExternalScanner.ExternalScannerForLanguage -- against the shipped
// luau.bin's actual external symbol count and order.
func TestLuauExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("luau")
	if !ok {
		t.Fatal("missing luau external scanner spec")
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
		t.Fatalf("luau spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "a8914d6c1fc5131f8e1c13f769fa704c9f5eb02f"; got != want {
		t.Fatalf("luau spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("luau")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("luau blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := LuauExternalScanner{}.ExternalScannerForLanguage(lang).(LuauExternalScanner)
	if !ok {
		t.Fatalf("LuauExternalScanner binding type = %T, want LuauExternalScanner", LuauExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, luauTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("luau externalToToken = %v, want %v (all %d externals must bind)", got, want, luauTokenCount)
	}
	if got, want := scanner.symbols, luauDefaultSymTable; got != want {
		t.Fatalf("luau post-bind symbols = %v, want default table %v", got, want)
	}

	// The comment and string delimiters collapse onto shared literal/generic
	// display names rather than each keeping its own rule name.
	wantDisplay := []string{
		"[[",
		"comment_content",
		"]]",
		"[[",
		"string_content",
		"]]",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("luau external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("luau external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestMarkdownInlineExternalScannerSpecMatchesBlob pins
// mdiExternalScannerSpec's Externals list -- the binding source for
// MarkdownInlineExternalScanner.ExternalScannerForLanguage -- against the
// shipped markdown_inline.bin's actual external symbol count and order.
func TestMarkdownInlineExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("markdown_inline")
	if !ok {
		t.Fatal("missing markdown_inline external scanner spec")
	}
	wantExternals := []string{
		"_error",
		"_trigger_error",
		"_code_span_start",
		"_code_span_close",
		"_emphasis_open_star",
		"_emphasis_open_underscore",
		"_emphasis_close_star",
		"_emphasis_close_underscore",
		"_last_token_whitespace",
		"_last_token_punctuation",
		"_strikethrough_open",
		"_strikethrough_close",
		"_latex_span_start",
		"_latex_span_close",
		"_unclosed_span",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("markdown_inline spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "f969cd3ae3f9fbd4e43205431d0ae286014c05b5"; got != want {
		t.Fatalf("markdown_inline spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("markdown_inline")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("markdown_inline blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := MarkdownInlineExternalScanner{}.ExternalScannerForLanguage(lang).(MarkdownInlineExternalScanner)
	if !ok {
		t.Fatalf("MarkdownInlineExternalScanner binding type = %T, want MarkdownInlineExternalScanner", MarkdownInlineExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, mdiTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("markdown_inline externalToToken = %v, want %v (all %d externals must bind)", got, want, mdiTokenCount)
	}
	if got, want := scanner.symbols, mdiDefaultSymTable; got != want {
		t.Fatalf("markdown_inline post-bind symbols = %v, want default table %v", got, want)
	}

	// Several externals collapse onto shared display names: the code span
	// start/close pair both display as "code_span_delimiter", the star/
	// underscore emphasis close pair and the strikethrough close token all
	// display as "emphasis_delimiter", and the latex span start/close pair
	// both display as "latex_span_delimiter".
	wantDisplay := []string{
		"_error",
		"_trigger_error",
		"code_span_delimiter",
		"code_span_delimiter",
		"_emphasis_open_star",
		"_emphasis_open_underscore",
		"emphasis_delimiter",
		"emphasis_delimiter",
		"_last_token_whitespace",
		"_last_token_punctuation",
		"_strikethrough_open",
		"emphasis_delimiter",
		"latex_span_delimiter",
		"latex_span_delimiter",
		"_unclosed_span",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("markdown_inline external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("markdown_inline external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestMatlabExternalScannerSpecMatchesBlob pins matExternalScannerSpec's
// Externals list -- the binding source for
// MatlabExternalScanner.ExternalScannerForLanguage -- against the shipped
// matlab.bin's actual external symbol count and order.
func TestMatlabExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("matlab")
	if !ok {
		t.Fatal("missing matlab external scanner spec")
	}
	wantExternals := []string{
		"comment",
		"line_continuation",
		"command_name",
		"command_argument",
		"_single_quote_string_start",
		"_single_quote_string_end",
		"_double_quote_string_start",
		"_double_quote_string_end",
		"formatting_sequence",
		"escape_sequence",
		"string_content",
		"_entry_delimiter",
		"_multioutput_variable_start",
		"_external_identifier",
		"_catch_identifier",
		"_transpose",
		"_ctranspose",
		"error_sentinel",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("matlab spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "574dde565caddf8cf44eec7df3cb89eb96053ed7"; got != want {
		t.Fatalf("matlab spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("matlab")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("matlab blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := MatlabExternalScanner{}.ExternalScannerForLanguage(lang).(MatlabExternalScanner)
	if !ok {
		t.Fatalf("MatlabExternalScanner binding type = %T, want MatlabExternalScanner", MatlabExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, matTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("matlab externalToToken = %v, want %v (all %d externals must bind)", got, want, matTokenCount)
	}
	if got, want := scanner.symbols, matDefaultSymTable; got != want {
		t.Fatalf("matlab post-bind symbols = %v, want default table %v", got, want)
	}

	// The single/double quote string delimiters and the transpose token all
	// collapse onto punctuation-literal display names, and _catch_identifier
	// aliases to "identifier" -- unlike _external_identifier at index 13,
	// which keeps its own rule name with no alias.
	wantDisplay := []string{
		"comment",
		"line_continuation",
		"command_name",
		"command_argument",
		"'",
		"'",
		"\"",
		"\"",
		"formatting_sequence",
		"escape_sequence",
		"string_content",
		",",
		"[",
		"_external_identifier",
		"identifier",
		"'",
		".'",
		"error_sentinel",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("matlab external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("matlab external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestMojoExternalScannerSpecMatchesBlob pins mojoExternalScannerSpec's
// Externals list -- the binding source for
// MojoExternalScanner.ExternalScannerForLanguage -- against the shipped
// mojo.bin's actual external symbol count and order.
func TestMojoExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("mojo")
	if !ok {
		t.Fatal("missing mojo external scanner spec")
	}
	wantExternals := []string{
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
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("mojo spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "c307dab71a43add26b4715f14e2d6de2a42e6007"; got != want {
		t.Fatalf("mojo spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("mojo")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("mojo blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := MojoExternalScanner{}.ExternalScannerForLanguage(lang).(MojoExternalScanner)
	if !ok {
		t.Fatalf("MojoExternalScanner binding type = %T, want MojoExternalScanner", MojoExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, mojoTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("mojo externalToToken = %v, want %v (all %d externals must bind)", got, want, mojoTokenCount)
	}
	if got, want := scanner.symbols, mojoDefaultSymTable; got != want {
		t.Fatalf("mojo post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("mojo external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("mojo external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestMoveExternalScannerSpecMatchesBlob pins moveExternalScannerSpec's
// Externals list -- the binding source for
// MoveExternalScanner.ExternalScannerForLanguage -- against the shipped
// move.bin's actual external symbol count and order.
func TestMoveExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("move")
	if !ok {
		t.Fatal("missing move external scanner spec")
	}
	wantExternals := []string{
		"_block_doc_comment_marker",
		"_block_comment_content",
		"_doc_line_comment",
		"_error_sentinel",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("move spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "12906b341de7cef81cf03d7d91dae51d8a9299e7"; got != want {
		t.Fatalf("move spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("move")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("move blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := MoveExternalScanner{}.ExternalScannerForLanguage(lang).(MoveExternalScanner)
	if !ok {
		t.Fatalf("MoveExternalScanner binding type = %T, want MoveExternalScanner", MoveExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, moveTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("move externalToToken = %v, want %v (all %d externals must bind)", got, want, moveTokenCount)
	}
	if got, want := scanner.symbols, moveDefaultSymTable; got != want {
		t.Fatalf("move post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("move external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("move external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestNginxExternalScannerSpecMatchesBlob pins nginxExternalScannerSpec's
// Externals list -- the binding source for
// NginxExternalScanner.ExternalScannerForLanguage -- against the shipped
// nginx.bin's actual external symbol count and order.
func TestNginxExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("nginx")
	if !ok {
		t.Fatal("missing nginx external scanner spec")
	}
	wantExternals := []string{
		"_newline",
		"_indent",
		"_dedent",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("nginx spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "47ade644d754cce57974aac44d2c9450e823d4f4"; got != want {
		t.Fatalf("nginx spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("nginx")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("nginx blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := NginxExternalScanner{}.ExternalScannerForLanguage(lang).(NginxExternalScanner)
	if !ok {
		t.Fatalf("NginxExternalScanner binding type = %T, want NginxExternalScanner", NginxExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, nginxTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("nginx externalToToken = %v, want %v (all %d externals must bind)", got, want, nginxTokenCount)
	}
	if got, want := scanner.symbols, nginxDefaultSymTable; got != want {
		t.Fatalf("nginx post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("nginx external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("nginx external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestNickelExternalScannerSpecMatchesBlob pins nickelExternalScannerSpec's
// Externals list -- the binding source for
// NickelExternalScanner.ExternalScannerForLanguage -- against the shipped
// nickel.bin's actual external symbol count and order.
func TestNickelExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("nickel")
	if !ok {
		t.Fatal("missing nickel external scanner spec")
	}
	wantExternals := []string{
		"multstr_start",
		"multstr_end",
		"_str_start",
		"_str_end",
		"interpolation_start",
		"interpolation_end",
		"quoted_enum_tag_start",
		"comment",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("nickel spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "b5b6cc3bc7b9ea19f78fed264190685419cd17a8"; got != want {
		t.Fatalf("nickel spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("nickel")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("nickel blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := NickelExternalScanner{}.ExternalScannerForLanguage(lang).(NickelExternalScanner)
	if !ok {
		t.Fatalf("NickelExternalScanner binding type = %T, want NickelExternalScanner", NickelExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, nickelTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("nickel externalToToken = %v, want %v (all %d externals must bind)", got, want, nickelTokenCount)
	}
	if got, want := scanner.symbols, nickelDefaultSymTable; got != want {
		t.Fatalf("nickel post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("nickel external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("nickel external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestNimExternalScannerSpecMatchesBlob pins nimExternalScannerSpec's
// Externals list -- the binding source for
// NimExternalScanner.ExternalScannerForLanguage -- against the shipped
// nim.bin's actual external symbol count and order.
func TestNimExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("nim")
	if !ok {
		t.Fatal("missing nim external scanner spec")
	}
	wantExternals := []string{
		"_block_comment_content",
		"_block_documentation_comment_content",
		"comment_content",
		"_long_string_quote",
		"_layout_start",
		"_layout_end",
		"_layout_terminator",
		"_layout_empty",
		"_inhibit_layout_end",
		"_inhibit_keyword_termination",
		",",
		"_synchronize",
		"_invalid_layout",
		"_sigil_operator",
		"_prefix_operator",
		"_want_export_marker",
		"_case_of",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("nim spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "9b4ede21a6ca866d29263f6b66c070961bc622b4"; got != want {
		t.Fatalf("nim spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("nim")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("nim blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := NimExternalScanner{}.ExternalScannerForLanguage(lang).(NimExternalScanner)
	if !ok {
		t.Fatalf("NimExternalScanner binding type = %T, want NimExternalScanner", NimExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, nimTokLen)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("nim externalToToken = %v, want %v (all %d externals must bind)", got, want, nimTokLen)
	}
	if got, want := scanner.symbols, nimDefaultSymTable; got != want {
		t.Fatalf("nim post-bind symbols = %v, want default table %v", got, want)
	}

	// _block_comment_content, _block_documentation_comment_content, and
	// comment_content all collapse onto the shared "comment_content"
	// display name; _sigil_operator and _prefix_operator both display as
	// "operator"; the literal "," external keeps its literal display; and
	// _case_of displays as the "of" keyword.
	wantDisplay := []string{
		"comment_content",
		"comment_content",
		"comment_content",
		"_long_string_quote",
		"_layout_start",
		"_layout_end",
		"_layout_terminator",
		"_layout_empty",
		"_inhibit_layout_end",
		"_inhibit_keyword_termination",
		",",
		"_synchronize",
		"_invalid_layout",
		"operator",
		"operator",
		"_want_export_marker",
		"of",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("nim external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("nim external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestNorgExternalScannerSpecMatchesBlob pins norgExternalScannerSpec's
// Externals list -- the binding source for
// NorgExternalScanner.ExternalScannerForLanguage -- against the shipped
// norg.bin's actual external symbol count and order.
func TestNorgExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("norg")
	if !ok {
		t.Fatal("missing norg external scanner spec")
	}
	wantExternals := []string{
		"_",
		"space",
		"lowercase_word",
		"capitalized_word",
		"line_break",
		"paragraph_break",
		"escape_sequence_prefix",
		"trailing_modifier",
		"detached_modifier_extension_begin",
		"mod_extension_delimiter",
		"detached_modifier_extension_end",
		"_priority",
		"_timestamp",
		"todo_item_undone",
		"todo_item_pending",
		"todo_item_done",
		"todo_item_on_hold",
		"todo_item_cancelled",
		"todo_item_urgent",
		"todo_item_uncertain",
		"_todo_item_recurring",
		"heading1_prefix",
		"heading2_prefix",
		"heading3_prefix",
		"heading4_prefix",
		"heading5_prefix",
		"heading6_prefix",
		"quote1_prefix",
		"quote2_prefix",
		"quote3_prefix",
		"quote4_prefix",
		"quote5_prefix",
		"quote6_prefix",
		"unordered_list1_prefix",
		"unordered_list2_prefix",
		"unordered_list3_prefix",
		"unordered_list4_prefix",
		"unordered_list5_prefix",
		"unordered_list6_prefix",
		"ordered_list1_prefix",
		"ordered_list2_prefix",
		"ordered_list3_prefix",
		"ordered_list4_prefix",
		"ordered_list5_prefix",
		"ordered_list6_prefix",
		"single_definition_prefix",
		"multi_definition_prefix",
		"multi_definition_suffix",
		"single_footnote_prefix",
		"multi_footnote_prefix",
		"multi_footnote_suffix",
		"single_table_cell_prefix",
		"multi_table_cell_prefix",
		"multi_table_cell_suffix",
		"strong_paragraph_delimiter",
		"weak_paragraph_delimiter",
		"horizontal_line",
		"link_description_begin",
		"link_description_end",
		"link_location_begin",
		"link_location_end",
		"link_file_begin",
		"link_file_end",
		"link_file_text",
		"link_target_url",
		"link_target_line_number",
		"link_target_wiki",
		"link_target_generic",
		"link_target_external_file",
		"link_target_timestamp",
		"link_target_definition",
		"link_target_footnote",
		"link_target_heading1",
		"link_target_heading2",
		"link_target_heading3",
		"link_target_heading4",
		"link_target_heading5",
		"link_target_heading6",
		"timestamp_data",
		"priority_data",
		"tag_delimiter",
		"macro_tag_prefix",
		"macro_tag_end_prefix",
		"ranged_tag_prefix",
		"ranged_tag_end_prefix",
		"ranged_verbatim_tag_prefix",
		"ranged_verbatim_tag_end_prefix",
		"infirm_tag_prefix",
		"weak_carryover_prefix",
		"strong_carryover_prefix",
		"link_modifier",
		"intersecting_modifier",
		"attached_mod_extension_begin",
		"attached_mod_extension_end",
		"bold_open",
		"bold_close",
		"italic_open",
		"italic_close",
		"strikethrough_open",
		"strikethrough_close",
		"underline_open",
		"underline_close",
		"spoiler_open",
		"spoiler_close",
		"superscript_open",
		"superscript_close",
		"subscript_open",
		"subscript_close",
		"verbatim_open",
		"verbatim_close",
		"inline_comment_open",
		"inline_comment_close",
		"inline_math_open",
		"inline_math_close",
		"inline_macro_open",
		"inline_macro_close",
		"free_form_open",
		"free_form_close",
		"inline_link_target_open",
		"inline_link_target_close",
		"slide_begin",
		"indent_segment_begin",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("norg spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "d89d95af13d409f30a6c7676387bde311ec4a2c8"; got != want {
		t.Fatalf("norg spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("norg")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("norg blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := NorgExternalScanner{}.ExternalScannerForLanguage(lang).(NorgExternalScanner)
	if !ok {
		t.Fatalf("NorgExternalScanner binding type = %T, want NorgExternalScanner", NorgExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, norgTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("norg externalToToken = %v, want %v (all %d externals must bind)", got, want, norgTokenCount)
	}
	if got, want := scanner.symbols, norgDefaultSymTable; got != want {
		t.Fatalf("norg post-bind symbols = %v, want default table %v", got, want)
	}

	// Norg's 122 externals collapse heavily onto shared generic display
	// names in the shipped blob (many pairs/groups alias to "_word",
	// "_open", "_close", "_prefix", "_delimiter", "_begin", or "_end"
	// rather than each keeping its own upstream rule name), so this pins
	// the actual display name observed per index rather than asserting
	// against spec.Externals directly.
	wantDisplay := []string{
		"_",
		"_space",
		"_lowercase",
		"_uppercase",
		"_line_break",
		"_paragraph_break",
		"escape_sequence_prefix",
		"_trailing_modifier",
		"_begin",
		"_delimiter",
		"_end",
		"_priority",
		"_timestamp",
		"todo_item_undone",
		"todo_item_pending",
		"todo_item_done",
		"todo_item_on_hold",
		"todo_item_cancelled",
		"todo_item_urgent",
		"todo_item_uncertain",
		"_todo_item_recurring",
		"heading1_prefix",
		"heading2_prefix",
		"heading3_prefix",
		"heading4_prefix",
		"heading5_prefix",
		"heading6_prefix",
		"quote1_prefix",
		"quote2_prefix",
		"quote3_prefix",
		"quote4_prefix",
		"quote5_prefix",
		"quote6_prefix",
		"unordered_list1_prefix",
		"unordered_list2_prefix",
		"unordered_list3_prefix",
		"unordered_list4_prefix",
		"unordered_list5_prefix",
		"unordered_list6_prefix",
		"ordered_list1_prefix",
		"ordered_list2_prefix",
		"ordered_list3_prefix",
		"ordered_list4_prefix",
		"ordered_list5_prefix",
		"ordered_list6_prefix",
		"single_definition_prefix",
		"multi_definition_prefix",
		"multi_definition_suffix",
		"single_footnote_prefix",
		"multi_footnote_prefix",
		"multi_footnote_suffix",
		"single_table_cell_prefix",
		"multi_table_cell_prefix",
		"multi_table_cell_suffix",
		"strong_paragraph_delimiter",
		"weak_paragraph_delimiter",
		"horizontal_line",
		"_word",
		"_word",
		"_begin",
		"_end",
		"_word",
		"_word",
		"link_file_text",
		"link_target_url",
		"link_target_line_number",
		"link_target_wiki",
		"link_target_generic",
		"link_target_external_file",
		"link_target_timestamp",
		"link_target_definition",
		"link_target_footnote",
		"link_target_heading1",
		"link_target_heading2",
		"link_target_heading3",
		"link_target_heading4",
		"link_target_heading5",
		"link_target_heading6",
		"timestamp_data",
		"priority_data",
		"_delimiter",
		"_prefix",
		"_prefix",
		"_prefix",
		"_prefix",
		"_prefix",
		"_prefix",
		"_prefix",
		"_prefix",
		"_prefix",
		"link_modifier",
		"intersecting_modifier",
		"_word",
		"_word",
		"_open",
		"_close",
		"_open",
		"_close",
		"_open",
		"_close",
		"_open",
		"_close",
		"_open",
		"_close",
		"_open",
		"_close",
		"_open",
		"_close",
		"_open",
		"_close",
		"_open",
		"_close",
		"_open",
		"_close",
		"_open",
		"_close",
		"free_form_open",
		"free_form_close",
		"_word",
		"_word",
		"_slide",
		"indent_segment_begin",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("norg external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("norg external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestNushellExternalScannerSpecMatchesBlob pins nushellExternalScannerSpec's
// Externals list -- the binding source for
// NushellExternalScanner.ExternalScannerForLanguage -- against the shipped
// nushell.bin's actual external symbol count and order.
func TestNushellExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("nushell")
	if !ok {
		t.Fatal("missing nushell external scanner spec")
	}
	wantExternals := []string{
		"raw_string_begin",
		"raw_string_content",
		"raw_string_end",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("nushell spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "bb3f533e5792260291945e1f329e1f0a779def6e"; got != want {
		t.Fatalf("nushell spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("nushell")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("nushell blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := NushellExternalScanner{}.ExternalScannerForLanguage(lang).(NushellExternalScanner)
	if !ok {
		t.Fatalf("NushellExternalScanner binding type = %T, want NushellExternalScanner", NushellExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, nushellTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("nushell externalToToken = %v, want %v (all %d externals must bind)", got, want, nushellTokenCount)
	}
	if got, want := scanner.symbols, nushellDefaultSymTable; got != want {
		t.Fatalf("nushell post-bind symbols = %v, want default table %v", got, want)
	}

	// raw_string_content displays as "string_content".
	wantDisplay := []string{
		"raw_string_begin",
		"string_content",
		"raw_string_end",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("nushell external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("nushell external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestOdinExternalScannerSpecMatchesBlob pins odinExternalScannerSpec's
// Externals list -- the binding source for
// OdinExternalScanner.ExternalScannerForLanguage -- against the shipped
// odin.bin's actual external symbol count and order.
func TestOdinExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("odin")
	if !ok {
		t.Fatal("missing odin external scanner spec")
	}
	wantExternals := []string{
		"_newline",
		"_backslash",
		"_nl_comma",
		"float",
		"block_comment",
		"{",
		"\"",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("odin spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "d2ca8efb4487e156a60d5bd6db2598b872629403"; got != want {
		t.Fatalf("odin spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("odin")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("odin blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := OdinExternalScanner{}.ExternalScannerForLanguage(lang).(OdinExternalScanner)
	if !ok {
		t.Fatalf("OdinExternalScanner binding type = %T, want OdinExternalScanner", OdinExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, odinTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("odin externalToToken = %v, want %v (all %d externals must bind)", got, want, odinTokenCount)
	}
	if got, want := scanner.symbols, odinDefaultSymTable; got != want {
		t.Fatalf("odin post-bind symbols = %v, want default table %v", got, want)
	}

	// _nl_comma displays as the "," literal; the "{" and "\"" externals
	// keep their literal display names.
	wantDisplay := []string{
		"_newline",
		"_backslash",
		",",
		"float",
		"block_comment",
		"{",
		"\"",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("odin external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("odin external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestOrgExternalScannerSpecMatchesBlob pins orgExternalScannerSpec's
// Externals list -- the binding source for
// OrgExternalScanner.ExternalScannerForLanguage -- against the shipped
// org.bin's actual external symbol count and order.
func TestOrgExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("org")
	if !ok {
		t.Fatal("missing org external scanner spec")
	}
	wantExternals := []string{
		"_liststart",
		"_listend",
		"_listitemend",
		"bullet",
		"_stars",
		"_sectionend",
		"_eof",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("org spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "64cfbc213f5a83da17632c95382a5a0a2f3357c1"; got != want {
		t.Fatalf("org spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("org")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("org blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := OrgExternalScanner{}.ExternalScannerForLanguage(lang).(OrgExternalScanner)
	if !ok {
		t.Fatalf("OrgExternalScanner binding type = %T, want OrgExternalScanner", OrgExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, orgTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("org externalToToken = %v, want %v (all %d externals must bind)", got, want, orgTokenCount)
	}
	if got, want := scanner.symbols, orgDefaultSymTable; got != want {
		t.Fatalf("org post-bind symbols = %v, want default table %v", got, want)
	}

	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("org external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != spec.Externals[i] {
			t.Fatalf("org external index %d: blob display name = %q, want %q", i, display, spec.Externals[i])
		}
	}
}

// TestPklExternalScannerSpecMatchesBlob pins pklExternalScannerSpec's
// Externals list -- the binding source for
// PklExternalScanner.ExternalScannerForLanguage -- against the shipped
// pkl.bin's actual external symbol count and order.
func TestPklExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("pkl")
	if !ok {
		t.Fatal("missing pkl external scanner spec")
	}
	wantExternals := []string{
		"_sl_string_chars",
		"_sl1_string_chars",
		"_sl2_string_chars",
		"_sl3_string_chars",
		"_sl4_string_chars",
		"_sl5_string_chars",
		"_sl6_string_chars",
		"_ml_string_chars",
		"_ml1_string_chars",
		"_ml2_string_chars",
		"_ml3_string_chars",
		"_ml4_string_chars",
		"_ml5_string_chars",
		"_ml6_string_chars",
		"_open_subscript_bracket",
		"_open_argument_paren",
		"_binary_minus",
	}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("pkl spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "a02fc36f6001a22e7fdf35eaabbadb7b39c74ba5"; got != want {
		t.Fatalf("pkl spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("pkl")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("pkl blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := PklExternalScanner{}.ExternalScannerForLanguage(lang).(PklExternalScanner)
	if !ok {
		t.Fatalf("PklExternalScanner binding type = %T, want PklExternalScanner", PklExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, pklTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("pkl externalToToken = %v, want %v (all %d externals must bind)", got, want, pklTokenCount)
	}
	if got, want := scanner.symbols, pklDefaultSymTable; got != want {
		t.Fatalf("pkl post-bind symbols = %v, want default table %v", got, want)
	}

	// _open_subscript_bracket, _open_argument_paren, and _binary_minus
	// display as their literal characters rather than their rule names.
	wantDisplay := []string{
		"_sl_string_chars",
		"_sl1_string_chars",
		"_sl2_string_chars",
		"_sl3_string_chars",
		"_sl4_string_chars",
		"_sl5_string_chars",
		"_sl6_string_chars",
		"_ml_string_chars",
		"_ml1_string_chars",
		"_ml2_string_chars",
		"_ml3_string_chars",
		"_ml4_string_chars",
		"_ml5_string_chars",
		"_ml6_string_chars",
		"[",
		"(",
		"-",
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("pkl external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("pkl external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestPropertiesExternalScannerSpecMatchesBlob pins propertiesExternalScannerSpec's
// externals list (the binding source for PropertiesExternalScanner) against
// properties.bin's actual external symbol count and order.
func TestPropertiesExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("properties")
	if !ok {
		t.Fatal("missing properties external scanner spec")
	}
	wantExternals := []string{"_eof"}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("properties spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "6310671b24d4e04b803577b1c675d765cbd5773b"; got != want {
		t.Fatalf("properties spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("properties")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("properties blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := PropertiesExternalScanner{}.ExternalScannerForLanguage(lang).(PropertiesExternalScanner)
	if !ok {
		t.Fatalf("PropertiesExternalScanner binding type = %T, want PropertiesExternalScanner", PropertiesExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, propertiesTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("properties externalToToken = %v, want %v (all %d externals must bind)", got, want, propertiesTokenCount)
	}
	if got, want := scanner.symbols, propertiesDefaultSymTable; got != want {
		t.Fatalf("properties post-bind symbols = %v, want default table %v", got, want)
	}

	wantDisplay := []string{"_eof"}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("properties external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("properties external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestPugExternalScannerSpecMatchesBlob pins pugExternalScannerSpec's
// externals list (the binding source for PugExternalScanner) against
// pug.bin's actual external symbol count and order.
func TestPugExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("pug")
	if !ok {
		t.Fatal("missing pug external scanner spec")
	}
	wantExternals := []string{"_newline", "_indent", "_dedent"}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("pug spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "13e9195370172c86a8b88184cc358b23b677cc46"; got != want {
		t.Fatalf("pug spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("pug")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("pug blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := PugExternalScanner{}.ExternalScannerForLanguage(lang).(PugExternalScanner)
	if !ok {
		t.Fatalf("PugExternalScanner binding type = %T, want PugExternalScanner", PugExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, pugTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("pug externalToToken = %v, want %v (all %d externals must bind)", got, want, pugTokenCount)
	}
	if got, want := scanner.symbols, pugDefaultSymTable; got != want {
		t.Fatalf("pug post-bind symbols = %v, want default table %v", got, want)
	}

	wantDisplay := []string{"_newline", "_indent", "_dedent"}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("pug external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("pug external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestTclExternalScannerSpecMatchesBlob pins tclExternalScannerSpec's
// externals list (the binding source for TclExternalScanner) against
// tcl.bin's actual external symbol count and order.
func TestTclExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("tcl")
	if !ok {
		t.Fatal("missing tcl external scanner spec")
	}
	wantExternals := []string{"_concat", "_immediate"}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("tcl spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "8f11ac7206a54ed11210491cee1e0657e2962c47"; got != want {
		t.Fatalf("tcl spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("tcl")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("tcl blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := TclExternalScanner{}.ExternalScannerForLanguage(lang).(TclExternalScanner)
	if !ok {
		t.Fatalf("TclExternalScanner binding type = %T, want TclExternalScanner", TclExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, tclTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("tcl externalToToken = %v, want %v (all %d externals must bind)", got, want, tclTokenCount)
	}
	if got, want := scanner.symbols, tclDefaultSymTable; got != want {
		t.Fatalf("tcl post-bind symbols = %v, want default table %v", got, want)
	}

	wantDisplay := []string{"_concat", "_immediate"}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("tcl external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("tcl external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestUxntalExternalScannerSpecMatchesBlob pins uxntalExternalScannerSpec's
// externals list (the binding source for UxntalExternalScanner) against
// uxntal.bin's actual external symbol count and order.
func TestUxntalExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("uxntal")
	if !ok {
		t.Fatal("missing uxntal external scanner spec")
	}
	wantExternals := []string{"comment"}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("uxntal spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "ad9b638b914095320de85d59c49ab271603af048"; got != want {
		t.Fatalf("uxntal spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("uxntal")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("uxntal blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := UxntalExternalScanner{}.ExternalScannerForLanguage(lang).(UxntalExternalScanner)
	if !ok {
		t.Fatalf("UxntalExternalScanner binding type = %T, want UxntalExternalScanner", UxntalExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, uxntalTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("uxntal externalToToken = %v, want %v (all %d externals must bind)", got, want, uxntalTokenCount)
	}
	if got, want := scanner.symbols, uxntalDefaultSymTable; got != want {
		t.Fatalf("uxntal post-bind symbols = %v, want default table %v", got, want)
	}

	wantDisplay := []string{"comment"}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("uxntal external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("uxntal external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestSquirrelExternalScannerSpecMatchesBlob pins squirrelExternalScannerSpec's
// externals list (the binding source for SquirrelExternalScanner) against
// squirrel.bin's actual external symbol count and order.
func TestSquirrelExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("squirrel")
	if !ok {
		t.Fatal("missing squirrel external scanner spec")
	}
	wantExternals := []string{"verbatim_string"}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("squirrel spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "072c969749e66f000dba35a33c387650e203e96e"; got != want {
		t.Fatalf("squirrel spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("squirrel")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("squirrel blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := SquirrelExternalScanner{}.ExternalScannerForLanguage(lang).(SquirrelExternalScanner)
	if !ok {
		t.Fatalf("SquirrelExternalScanner binding type = %T, want SquirrelExternalScanner", SquirrelExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, squirrelTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("squirrel externalToToken = %v, want %v (all %d externals must bind)", got, want, squirrelTokenCount)
	}
	if got, want := scanner.symbols, squirrelDefaultSymTable; got != want {
		t.Fatalf("squirrel post-bind symbols = %v, want default table %v", got, want)
	}

	wantDisplay := []string{"verbatim_string"}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("squirrel external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("squirrel external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestRacketExternalScannerSpecMatchesBlob pins racketExternalScannerSpec's
// externals list (the binding source for RacketExternalScanner) against
// racket.bin's actual external symbol count and order.
func TestRacketExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("racket")
	if !ok {
		t.Fatal("missing racket external scanner spec")
	}
	wantExternals := []string{"_here_string_body"}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("racket spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "56b57807f86aa4ddb14892572b318edd4bc90ebe"; got != want {
		t.Fatalf("racket spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("racket")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("racket blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := RacketExternalScanner{}.ExternalScannerForLanguage(lang).(RacketExternalScanner)
	if !ok {
		t.Fatalf("RacketExternalScanner binding type = %T, want RacketExternalScanner", RacketExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, racketTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("racket externalToToken = %v, want %v (all %d externals must bind)", got, want, racketTokenCount)
	}
	if got, want := scanner.symbols, racketDefaultSymTable; got != want {
		t.Fatalf("racket post-bind symbols = %v, want default table %v", got, want)
	}

	wantDisplay := []string{"_here_string_body"}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("racket external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("racket external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestYuckExternalScannerSpecMatchesBlob pins yuckExternalScannerSpec's
// externals list (the binding source for YuckExternalScanner) against
// yuck.bin's actual external symbol count and order.
func TestYuckExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("yuck")
	if !ok {
		t.Fatal("missing yuck external scanner spec")
	}
	wantExternals := []string{"_unescaped_single_quote_string_fragment", "_unescaped_double_quote_string_fragment", "_unescaped_backtick_string_fragment"}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("yuck spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "e877f6ade4b77d5ef8787075141053631ba12318"; got != want {
		t.Fatalf("yuck spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("yuck")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("yuck blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := YuckExternalScanner{}.ExternalScannerForLanguage(lang).(YuckExternalScanner)
	if !ok {
		t.Fatalf("YuckExternalScanner binding type = %T, want YuckExternalScanner", YuckExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, yuckTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("yuck externalToToken = %v, want %v (all %d externals must bind)", got, want, yuckTokenCount)
	}
	if got, want := scanner.symbols, yuckDefaultSymTable; got != want {
		t.Fatalf("yuck post-bind symbols = %v, want default table %v", got, want)
	}

	wantDisplay := []string{"_unescaped_single_quote_string_fragment", "_unescaped_double_quote_string_fragment", "_unescaped_backtick_string_fragment"}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("yuck external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("yuck external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestTablegenExternalScannerSpecMatchesBlob pins tablegenExternalScannerSpec's
// externals list (the binding source for TablegenExternalScanner) against
// tablegen.bin's actual external symbol count and order.
func TestTablegenExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("tablegen")
	if !ok {
		t.Fatal("missing tablegen external scanner spec")
	}
	wantExternals := []string{"multiline_comment"}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("tablegen spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "b1170880c61355aaf38fc06f4af7d3c55abdabc4"; got != want {
		t.Fatalf("tablegen spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("tablegen")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("tablegen blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := TablegenExternalScanner{}.ExternalScannerForLanguage(lang).(TablegenExternalScanner)
	if !ok {
		t.Fatalf("TablegenExternalScanner binding type = %T, want TablegenExternalScanner", TablegenExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, tablegenTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("tablegen externalToToken = %v, want %v (all %d externals must bind)", got, want, tablegenTokenCount)
	}
	if got, want := scanner.symbols, tablegenDefaultSymTable; got != want {
		t.Fatalf("tablegen post-bind symbols = %v, want default table %v", got, want)
	}

	wantDisplay := []string{"multiline_comment"}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("tablegen external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("tablegen external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestWgslExternalScannerSpecMatchesBlob pins wgslExternalScannerSpec's
// externals list (the binding source for WgslExternalScanner) against
// wgsl.bin's actual external symbol count and order.
func TestWgslExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("wgsl")
	if !ok {
		t.Fatal("missing wgsl external scanner spec")
	}
	wantExternals := []string{"block_comment"}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("wgsl spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "40259f3c77ea856841a4e0c4c807705f3e4a2b65"; got != want {
		t.Fatalf("wgsl spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("wgsl")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("wgsl blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := WgslExternalScanner{}.ExternalScannerForLanguage(lang).(WgslExternalScanner)
	if !ok {
		t.Fatalf("WgslExternalScanner binding type = %T, want WgslExternalScanner", WgslExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, wgslTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("wgsl externalToToken = %v, want %v (all %d externals must bind)", got, want, wgslTokenCount)
	}
	if got, want := scanner.symbols, wgslDefaultSymTable; got != want {
		t.Fatalf("wgsl post-bind symbols = %v, want default table %v", got, want)
	}

	wantDisplay := []string{"block_comment"}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("wgsl external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("wgsl external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestWolframExternalScannerSpecMatchesBlob pins wolframExternalScannerSpec's
// externals list (the binding source for WolframExternalScanner) against
// wolfram.bin's actual external symbol count and order.
func TestWolframExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("wolfram")
	if !ok {
		t.Fatal("missing wolfram external scanner spec")
	}
	wantExternals := []string{"comment"}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("wolfram spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "63ebdac6f040d9082d3d8fa88be96ce24549adc5"; got != want {
		t.Fatalf("wolfram spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("wolfram")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("wolfram blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := WolframExternalScanner{}.ExternalScannerForLanguage(lang).(WolframExternalScanner)
	if !ok {
		t.Fatalf("WolframExternalScanner binding type = %T, want WolframExternalScanner", WolframExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, wolframTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("wolfram externalToToken = %v, want %v (all %d externals must bind)", got, want, wolframTokenCount)
	}
	if got, want := scanner.symbols, wolframDefaultSymTable; got != want {
		t.Fatalf("wolfram post-bind symbols = %v, want default table %v", got, want)
	}

	wantDisplay := []string{"comment"}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("wolfram external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("wolfram external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestRonExternalScannerSpecMatchesBlob pins ronExternalScannerSpec's
// externals list (the binding source for RonExternalScanner) against
// ron.bin's actual external symbol count and order.
func TestRonExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("ron")
	if !ok {
		t.Fatal("missing ron external scanner spec")
	}
	wantExternals := []string{"_string_content", "raw_string", "float", "block_comment"}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("ron spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "78938553b93075e638035f624973083451b29055"; got != want {
		t.Fatalf("ron spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("ron")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("ron blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := RonExternalScanner{}.ExternalScannerForLanguage(lang).(RonExternalScanner)
	if !ok {
		t.Fatalf("RonExternalScanner binding type = %T, want RonExternalScanner", RonExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, ronTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("ron externalToToken = %v, want %v (all %d externals must bind)", got, want, ronTokenCount)
	}
	if got, want := scanner.symbols, ronDefaultSymTable; got != want {
		t.Fatalf("ron post-bind symbols = %v, want default table %v", got, want)
	}

	wantDisplay := []string{"_string_content", "raw_string", "float", "block_comment"}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("ron external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("ron external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestTemplExternalScannerSpecMatchesBlob pins templExternalScannerSpec's
// externals list (the binding source for TemplExternalScanner) against
// templ.bin's actual external symbol count and order.
func TestTemplExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("templ")
	if !ok {
		t.Fatal("missing templ external scanner spec")
	}
	wantExternals := []string{"css_property_value", "script_block_text", "switch_element_text", "element_text"}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("templ spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "1c6db04effbcd7773c826bded9783cbc3061bd55"; got != want {
		t.Fatalf("templ spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("templ")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("templ blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := TemplExternalScanner{}.ExternalScannerForLanguage(lang).(TemplExternalScanner)
	if !ok {
		t.Fatalf("TemplExternalScanner binding type = %T, want TemplExternalScanner", TemplExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, templTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("templ externalToToken = %v, want %v (all %d externals must bind)", got, want, templTokenCount)
	}
	if got, want := scanner.symbols, templDefaultSymTable; got != want {
		t.Fatalf("templ post-bind symbols = %v, want default table %v", got, want)
	}

	wantDisplay := []string{"css_property_value", "script_block_text", "element_text", "element_text"}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("templ external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("templ external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestVueExternalScannerSpecMatchesBlob pins vueExternalScannerSpec's
// externals list (the binding source for VueExternalScanner) against
// vue.bin's actual external symbol count and order.
func TestVueExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("vue")
	if !ok {
		t.Fatal("missing vue external scanner spec")
	}
	wantExternals := []string{"_start_tag_name", "_script_start_tag_name", "_style_start_tag_name", "_end_tag_name", "erroneous_end_tag_name", "/>", "_implicit_end_tag", "raw_text", "comment", "_template_start_tag_name", "_text_fragment", "_interpolation_text"}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("vue spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "ce8011a414fdf8091f4e4071752efc376f4afb08"; got != want {
		t.Fatalf("vue spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("vue")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("vue blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := VueExternalScanner{}.ExternalScannerForLanguage(lang).(VueExternalScanner)
	if !ok {
		t.Fatalf("VueExternalScanner binding type = %T, want VueExternalScanner", VueExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, vueTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("vue externalToToken = %v, want %v (all %d externals must bind)", got, want, vueTokenCount)
	}
	if got, want := scanner.symbols, vueDefaultSymTable; got != want {
		t.Fatalf("vue post-bind symbols = %v, want default table %v", got, want)
	}

	wantDisplay := []string{"tag_name", "tag_name", "tag_name", "tag_name", "erroneous_end_tag_name", "/>", "_implicit_end_tag", "raw_text", "comment", "tag_name", "_text_fragment", "raw_text"}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("vue external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("vue external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestTealExternalScannerSpecMatchesBlob pins tealExternalScannerSpec's
// externals list (the binding source for TealExternalScanner) against
// teal.bin's actual external symbol count and order.
func TestTealExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("teal")
	if !ok {
		t.Fatal("missing teal external scanner spec")
	}
	wantExternals := []string{"comment", "_long_string_start", "_long_string_char", "_long_string_end", "_short_string_start", "_short_string_char", "_short_string_end"}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("teal spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "05d276e737055e6f77a21335b7573c9d3c091e2f"; got != want {
		t.Fatalf("teal spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("teal")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("teal blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := TealExternalScanner{}.ExternalScannerForLanguage(lang).(TealExternalScanner)
	if !ok {
		t.Fatalf("TealExternalScanner binding type = %T, want TealExternalScanner", TealExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, tealTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("teal externalToToken = %v, want %v (all %d externals must bind)", got, want, tealTokenCount)
	}
	if got, want := scanner.symbols, tealDefaultSymTable; got != want {
		t.Fatalf("teal post-bind symbols = %v, want default table %v", got, want)
	}

	wantDisplay := []string{"comment", "long_string_start", "_long_string_char", "long_string_end", "short_string_start", "_short_string_char", "short_string_end"}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("teal external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("teal external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}

// TestStarlarkExternalScannerSpecMatchesBlob pins slExternalScannerSpec's
// externals list (the binding source for StarlarkExternalScanner) against
// starlark.bin's actual external symbol count and order.
func TestStarlarkExternalScannerSpecMatchesBlob(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("starlark")
	if !ok {
		t.Fatal("missing starlark external scanner spec")
	}
	wantExternals := []string{"_newline", "_indent", "_dedent", "string_start", "_string_content", "escape_interpolation", "string_end", "comment", "]", ")", "}", "except"}
	if !slices.Equal(spec.Externals, wantExternals) {
		t.Fatalf("starlark spec externals = %v, want %v", spec.Externals, wantExternals)
	}
	if got, want := spec.UpstreamCommit, "a453dbf3ba433db0e5ec621a38a7e59d72e4dc69"; got != want {
		t.Fatalf("starlark spec upstream commit = %q, want %q", got, want)
	}

	lang := Language("starlark")
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("starlark blob external symbol count = %d, want %d (spec.Externals length)", got, want)
	}

	scanner, ok := StarlarkExternalScanner{}.ExternalScannerForLanguage(lang).(StarlarkExternalScanner)
	if !ok {
		t.Fatalf("StarlarkExternalScanner binding type = %T, want StarlarkExternalScanner", StarlarkExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	want := make([]int, slTokenCount)
	for i := range want {
		want[i] = i
	}
	if got := scanner.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("starlark externalToToken = %v, want %v (all %d externals must bind)", got, want, slTokenCount)
	}
	if got, want := scanner.symbols, slDefaultSymTable; got != want {
		t.Fatalf("starlark post-bind symbols = %v, want default table %v", got, want)
	}

	wantDisplay := []string{"_newline", "_indent", "_dedent", "string_start", "_string_content", "escape_interpolation", "string_end", "comment", "]", ")", "}", "except"}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("starlark external index %d: symbol %d out of range of SymbolNames (len=%d)", i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("starlark external index %d: blob display name = %q, want %q", i, display, wantDisplay[i])
		}
	}
}
