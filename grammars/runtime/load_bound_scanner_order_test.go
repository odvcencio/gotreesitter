//go:build !grammar_subset

package grammarruntime

import (
	"fmt"
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

// scannerSpecCase is one row of TestExternalScannerSpecMatchesBlob. Each row
// pins a language's ExternalScannerSpec -- the binding source for its
// <Lang>ExternalScanner.ExternalScannerForLanguage -- against the shipped
// <lang>.bin's actual external symbol count and order.
type scannerSpecCase struct {
	lang string
	// commit is the upstream commit the spec records. Empty skips the check
	// (the spec has no upstream scanner, or does not record one).
	commit    string
	externals []string
	// display lists the blob's display name per external index. Nil means
	// every external displays exactly as its spec name. Set it where the
	// grammar aliases an external onto a shared or literal node type.
	display []string
	// tokenCount is the scanner's token-slot count. Every external must bind.
	tokenCount int
	// defaultSymbols is the scanner's default symbol table. Nil skips the
	// post-bind symbols check.
	defaultSymbols any
	bind           func(*gotreesitter.Language) scannerBinding
}

// scannerBinding is what a case observes after binding a scanner to a Language.
type scannerBinding struct {
	err             string
	externalToToken []int
	symbols         any
}

type externalScannerFactory interface {
	ExternalScannerForLanguage(*gotreesitter.Language) gotreesitter.ExternalScanner
}

// scannerBinder binds factory's scanner to a Language and reads the two
// fields the checks compare. S is the concrete type the factory returns.
func scannerBinder[F externalScannerFactory, S any](factory F, fields func(S) ([]int, any)) func(*gotreesitter.Language) scannerBinding {
	return func(lang *gotreesitter.Language) scannerBinding {
		got := factory.ExternalScannerForLanguage(lang)
		s, ok := got.(S)
		if !ok {
			var want S
			return scannerBinding{err: fmt.Sprintf("%T binding type = %T, want %T", factory, got, want)}
		}
		e2t, symbols := fields(s)
		return scannerBinding{externalToToken: e2t, symbols: symbols}
	}
}

func checkScannerSpecMatchesBlob(t *testing.T, c scannerSpecCase) {
	t.Helper()
	spec, ok := LookupExternalScannerSpec(c.lang)
	if !ok {
		t.Fatalf("missing %s external scanner spec", c.lang)
	}
	if !slices.Equal(spec.Externals, c.externals) {
		t.Fatalf("%s spec externals = %v, want %v", c.lang, spec.Externals, c.externals)
	}
	if c.commit != "" {
		if got, want := spec.UpstreamCommit, c.commit; got != want {
			t.Fatalf("%s spec upstream commit = %q, want %q", c.lang, got, want)
		}
	}

	lang := Language(c.lang)
	if got, want := len(lang.ExternalSymbols), len(spec.Externals); got != want {
		t.Fatalf("%s blob external symbol count = %d, want %d (spec.Externals length)", c.lang, got, want)
	}

	bound := c.bind(lang)
	if bound.err != "" {
		t.Fatal(bound.err)
	}
	want := make([]int, c.tokenCount)
	for i := range want {
		want[i] = i
	}
	if got := bound.externalToToken; !slices.Equal(got, want) {
		t.Fatalf("%s externalToToken = %v, want %v (all %d externals must bind)", c.lang, got, want, c.tokenCount)
	}
	if c.defaultSymbols != nil {
		if got, want := bound.symbols, c.defaultSymbols; got != want {
			t.Fatalf("%s post-bind symbols = %v, want default table %v", c.lang, got, want)
		}
	}

	wantDisplay := c.display
	if wantDisplay == nil {
		wantDisplay = spec.Externals
	}
	for i, sym := range lang.ExternalSymbols {
		if int(sym) < 0 || int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("%s external index %d: symbol %d out of range of SymbolNames (len=%d)", c.lang, i, sym, len(lang.SymbolNames))
		}
		display := lang.SymbolNames[sym]
		if display != wantDisplay[i] {
			t.Fatalf("%s external index %d: blob display name = %q, want %q", c.lang, i, display, wantDisplay[i])
		}
	}
}

func TestExternalScannerSpecMatchesBlob(t *testing.T) {
	for _, c := range scannerSpecCases {
		t.Run(c.lang, func(t *testing.T) { checkScannerSpecMatchesBlob(t, c) })
	}
}

var scannerSpecCases = []scannerSpecCase{
	{
		lang:   "powershell",
		commit: "e7bd348c49fdfd5c853a146a670965ba516a6239",
		externals: []string{
			"_statement_terminator",
		},
		tokenCount: powershellTokenCount,
		bind:       scannerBinder(PowershellExternalScanner{}, func(s PowershellExternalScanner) ([]int, any) { return s.externalToToken, nil }),
	},
	{
		lang: "caddy",
		externals: []string{
			"_newline",
			"_indent",
			"_dedent",
		},
		tokenCount: caddyTokenCount,
		bind:       scannerBinder(CaddyExternalScanner{}, func(s CaddyExternalScanner) ([]int, any) { return s.externalToToken, nil }),
	},
	{
		lang:   "doxygen",
		commit: "ccd998f378c3f9345ea4eeb223f56d7b84d16687",
		externals: []string{
			"brief_text",
			"code_block_start",
			"code_block_language",
			"code_block_content",
			"code_block_end",
		},
		tokenCount: doxygenTokenCount,
		bind:       scannerBinder(DoxygenExternalScanner{}, func(s DoxygenExternalScanner) ([]int, any) { return s.externalToToken, nil }),
	},
	{
		lang: "blade",
		externals: []string{
			"_start_tag_name",
			"_script_start_tag_name",
			"_style_start_tag_name",
			"_end_tag_name",
			"erroneous_end_tag_name",
			"/>",
			"_implicit_end_tag",
			"raw_text",
			"comment",
		},
		// The four tag-name externals (start/script-start/style-start/end) alias
		// to the same visible "tag_name" node type, so their display names
		// collapse; the remaining five externals display exactly as their spec
		// name.
		display: []string{
			"tag_name", "tag_name", "tag_name", "tag_name",
			"erroneous_end_tag_name", "/>", "_implicit_end_tag", "raw_text", "comment",
		},
		tokenCount:     bladeTokenCount,
		defaultSymbols: bladeDefaultSymTable,
		bind:           scannerBinder(BladeExternalScanner{}, func(s BladeExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang: "angular",
		externals: []string{
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
		},
		// The four tag-name externals (start/script-start/style-start/end) alias
		// to the same visible "tag_name" node type, and the two interpolation
		// delimiters, the control-flow marker, and the empty-quoted-string alias
		// display as their literal text, so their display names collapse or
		// differ from the spec's rule names; the remaining three externals
		// display exactly as their spec name.
		display: []string{
			"tag_name", "tag_name", "tag_name", "tag_name",
			"erroneous_end_tag_name", "/>", "_implicit_end_tag", "raw_text", "comment",
			"{{", "}}", "@", "\"\"",
		},
		tokenCount:     angularTokenCount,
		defaultSymbols: angularDefaultSymTable,
		bind:           scannerBinder(AngularExternalScanner{}, func(s AngularExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "css",
		commit: "dda5cfc5722c429eaba1c910ca32c2c0c5bb1a3f",
		externals: []string{
			"_descendant_operator",
			"_pseudo_class_selector_colon",
			"__error_recovery",
		},
		// The pseudo-class selector colon displays as its literal text ":" on
		// the loaded Language rather than its spec rule name.
		display:        []string{"_descendant_operator", ":", "__error_recovery"},
		tokenCount:     cssTokenCount,
		defaultSymbols: cssDefaultSymTable,
		bind:           scannerBinder(CssExternalScanner{}, func(s CssExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "toml",
		commit: "342d9be207c2dba869b9967124c679b5e6fd0ebe",
		externals: []string{
			"_line_ending_or_eof",
			"_multiline_basic_string_content",
			"_multiline_basic_string_end",
			"_multiline_literal_string_content",
			"_multiline_literal_string_end",
		},
		tokenCount:     tomlTokenCount,
		defaultSymbols: tomlDefaultSymTable,
		bind:           scannerBinder(TomlExternalScanner{}, func(s TomlExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "html",
		commit: "73a3947324f6efddf9e17c0ea58d454843590cc0",
		externals: []string{
			"_start_tag_name",
			"_script_start_tag_name",
			"_style_start_tag_name",
			"_end_tag_name",
			"erroneous_end_tag_name",
			"/>",
			"_implicit_end_tag",
			"raw_text",
			"comment",
		},
		// The four tag-name externals (start/script-start/style-start/end) alias
		// to the same visible "tag_name" node type, so their display names
		// collapse; the remaining five externals display exactly as their spec
		// name.
		display: []string{
			"tag_name", "tag_name", "tag_name", "tag_name",
			"erroneous_end_tag_name", "/>", "_implicit_end_tag", "raw_text", "comment",
		},
		tokenCount:     htmlTokenCount,
		defaultSymbols: htmlDefaultSymTable,
		bind:           scannerBinder(HTMLExternalScanner{}, func(s HTMLExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "bash",
		commit: "a06c2e4415e9bc0346c6b86d401879ffb44058f7",
		externals: []string{
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
		},
		// regex/_regex_no_slash/_regex_no_space all alias to the same visible
		// "regex" node type; the remaining externals mostly display their
		// literal text (bare-dollar, brace-start, immediate-double-hash, the
		// three external-expansion-sym externals) or a grammar-internal alias
		// name ("word" for _expansion_word, "heredoc_redirect_token1" for the
		// bare newline pattern external) instead of their spec rule name.
		display: []string{
			"heredoc_start", "heredoc_body", "_heredoc_body_beginning", "heredoc_content", "heredoc_end",
			"file_descriptor", "_empty_value", "_concat", "variable_name", "test_operator",
			"regex", "regex", "regex", "word", "extglob_pattern",
			"$", "{", "##", "#", "!", "=",
			"}", "]", "<<", "<<-", "heredoc_redirect_token1", "(", "esac", "__error_recovery",
		},
		tokenCount:     bshTokenCount,
		defaultSymbols: bshDefaultSymTable,
		bind:           scannerBinder(BashExternalScanner{}, func(s BashExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "ruby",
		commit: "ad907a69da0c8a4f7a943a7fe012712208da6dee",
		externals: []string{
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
		},
		// Most literal-delimiter externals (string/symbol/subshell/regex/array
		// starts, the close paren, the "/" forward slash, block-ampersand,
		// splat/binary star(s), minus variants, "<<", "[") display their literal
		// text on the loaded Language rather than their spec rule name; the
		// remaining hidden/named externals display exactly as their spec name.
		display: []string{
			"_line_break", "_no_line_break", "simple_symbol", "\"", ":\"",
			"`", "/", "%w(", "%i(", "_heredoc_body_start",
			"string_content", "heredoc_content", ")", "heredoc_end", "heredoc_beginning",
			"/", "&", "*", "-", "-",
			"-", "*", "<<", "hash_key_symbol", "_identifier_suffix",
			"_constant_suffix", "**", "**", "[", "_short_interpolation",
		},
		tokenCount:     rbyTokenCount,
		defaultSymbols: rbyDefaultSymTable,
		bind:           scannerBinder(RubyExternalScanner{}, func(s RubyExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "php",
		commit: "3fda2fb9577166c6399834917f9844f30370beea",
		externals: []string{
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
		},
		// The six encapsed/execution string-chars variants (plain, after-variable,
		// heredoc, and after-variable-heredoc, across normal and execution
		// strings) all alias to the same visible "string_content" node type; the
		// remaining externals display exactly as their spec name.
		display: []string{
			"_automatic_semicolon",
			"string_content", "string_content", "string_content", "string_content", "string_content", "string_content",
			"_eof", "heredoc_start", "heredoc_end", "nowdoc_string", "sentinel_error",
		},
		tokenCount:     phpTokenCount,
		defaultSymbols: phpDefaultSymTable,
		bind:           scannerBinder(PhpExternalScanner{}, func(s PhpExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "cmake",
		commit: "58993af75218bc99a1f5a04c832a5937e7c422cb",
		externals: []string{
			"bracket_argument_open",
			"bracket_argument_content",
			"bracket_argument_close",
			"bracket_comment_open",
			"bracket_comment_content",
			"bracket_comment_close",
			"line_comment",
		},
		tokenCount:     cmakeTokenCount,
		defaultSymbols: cmakeDefaultSymTable,
		bind:           scannerBinder(CmakeExternalScanner{}, func(s CmakeExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "cpp",
		commit: "c009222808634c1014f82438d4883753516a2c24",
		externals: []string{
			"raw_string_delimiter",
			"raw_string_content",
		},
		tokenCount:     cppTokenCount,
		defaultSymbols: cppDefaultSymTable,
		bind:           scannerBinder(CppExternalScanner{}, func(s CppExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "elixir",
		commit: "4b0c7118760af58a2e7081bbc8396e136f820b37",
		externals: []string{
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
		},
		// The 20 _quoted_content_* variants all alias to the same visible
		// "quoted_content" node type; _not_in displays as "not in" and
		// _quoted_atom_start displays as ":"; the remaining externals display
		// exactly as their spec name.
		display: []string{
			"quoted_content", "quoted_content", "quoted_content", "quoted_content", "quoted_content",
			"quoted_content", "quoted_content", "quoted_content", "quoted_content", "quoted_content",
			"quoted_content", "quoted_content", "quoted_content", "quoted_content", "quoted_content",
			"quoted_content", "quoted_content", "quoted_content", "quoted_content", "quoted_content",
			"_newline_before_do", "_newline_before_binary_operator", "_newline_before_comment",
			"_before_unary_op", "not in", ":",
		},
		tokenCount:     elixirTokenCount,
		defaultSymbols: elixirDefaultSymTable,
		bind:           scannerBinder(ElixirExternalScanner{}, func(s ElixirExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "elm",
		commit: "e1e8fea161a1e66f3997855d316be2a43e4e956f",
		externals: []string{
			"_virtual_end_decl",
			"_virtual_open_section",
			"_virtual_end_section",
			"minus_without_trailing_whitespace",
			"glsl_content",
			"_block_comment_content",
			"_string_content_multiline",
		},
		// minus_without_trailing_whitespace displays as "operator_identifier" and
		// _string_content_multiline displays as "regular_string_part" on the
		// loaded Language; the remaining externals display exactly as their spec
		// name.
		display: []string{
			"_virtual_end_decl", "_virtual_open_section", "_virtual_end_section",
			"operator_identifier", "glsl_content", "_block_comment_content", "regular_string_part",
		},
		tokenCount:     elmTokenCount,
		defaultSymbols: elmDefaultSymTable,
		bind:           scannerBinder(ElmExternalScanner{}, func(s ElmExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		// Unlike every other language here, this external is not ported from an
		// upstream C scanner.c (upstream tree-sitter-go has none); see
		// goExternalScannerSpec's doc comment in go_scanner.go for why grammargen
		// invents it.
		lang: "go",
		externals: []string{
			"_automatic_semicolon",
		},
		tokenCount:     goTokenCount,
		defaultSymbols: goDefaultSymTable,
		bind:           scannerBinder(GoExternalScanner{}, func(s GoExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "haskell",
		commit: "0975ef72fc3c47b530309ca93937d7d143523628",
		externals: []string{
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
		},
		// _cmd_layout_start_explicit and _cond_layout_end_explicit display as
		// their literal text ("{" and "}"); the trailing bare newline pattern
		// external displays as the grammar-internal alias "_token1"; the
		// remaining externals display exactly as their spec name.
		display: []string{
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
		},
		tokenCount:     hsTokenCount,
		defaultSymbols: hsDefaultSymTable,
		bind:           scannerBinder(HaskellExternalScanner{}, func(s HaskellExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "julia",
		commit: "e0f9dcd180fdcfcfa8d79a3531e11d99e79321d3",
		externals: []string{
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
		},
		// The eight _content_* externals (indexes 6-13) alias to the same
		// visible "content" node type; the remaining externals display exactly
		// as their spec name.
		display: []string{
			"_block_comment_rest", "_immediate_paren", "_immediate_bracket", "_immediate_brace",
			"_immediate_string_start", "_immediate_command_start", "content", "content",
			"content", "content", "content", "content",
			"content", "content", "_end_cmd", "_end_str",
		},
		tokenCount:     juliaTokenCount,
		defaultSymbols: juliaDefaultSymTable,
		bind:           scannerBinder(JuliaExternalScanner{}, func(s JuliaExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "lua",
		commit: "10fe0054734eec83049514ea2e718b2a56acd0c9",
		externals: []string{
			"_block_comment_start",
			"_block_comment_content",
			"_block_comment_end",
			"_block_string_start",
			"_block_string_content",
			"_block_string_end",
		},
		// _block_comment_start/_block_string_start display as the literal "[["
		// and _block_comment_end/_block_string_end display as the literal "]]",
		// each sharing a Symbol ID with every other occurrence of that literal
		// elsewhere in the grammar; the two content externals display as their
		// grammar-collapsed node names.
		display: []string{
			"[[", "comment_content", "]]", "[[", "string_content", "]]",
		},
		tokenCount:     luaTokenCount,
		defaultSymbols: luaDefaultSymTable,
		bind:           scannerBinder(LuaExternalScanner{}, func(s LuaExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "markdown",
		commit: "a0a00f817d02412bd92c54d316f164d827b57b5c",
		externals: []string{
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
		},
		// _block_quote_start displays as "block_quote_marker"; the four fenced
		// code block start/end externals (indexes 25, 26, 28, 29) all display as
		// "fenced_code_block_delimiter"; the remaining externals display exactly
		// as their spec name.
		display: []string{
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
		},
		tokenCount:     mdTokenCount,
		defaultSymbols: mdDefaultSymTable,
		bind:           scannerBinder(MarkdownExternalScanner{}, func(s MarkdownExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "nix",
		commit: "17f290c8b5104d9aba8a1ba7383a2ca83c3d14c4",
		externals: []string{
			"string_fragment",
			"_indented_string_fragment",
			"_path_start",
			"path_fragment",
			"dollar_escape",
			"_indented_dollar_escape",
		},
		// _indented_string_fragment aliases string_fragment's display node and
		// _path_start aliases path_fragment's display node; the remaining
		// externals display exactly as their spec name.
		display: []string{
			"string_fragment", "string_fragment", "path_fragment", "path_fragment",
			"dollar_escape", "dollar_escape",
		},
		tokenCount:     nixTokenCount,
		defaultSymbols: nixDefaultSymTable,
		bind:           scannerBinder(NixExternalScanner{}, func(s NixExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "scss",
		commit: "2ef6d42e3ad7a8208900f9346f4529806ae0f9f9",
		externals: []string{
			"_descendant_operator",
			"_pseudo_class_selector_colon",
			"__error_recovery",
			"_concat",
		},
		// _pseudo_class_selector_colon displays as the literal ":"; the
		// remaining externals display exactly as their spec name.
		display: []string{
			"_descendant_operator", ":", "__error_recovery", "_concat",
		},
		tokenCount:     scssTokenCount,
		defaultSymbols: scssDefaultSymTable,
		bind:           scannerBinder(ScssExternalScanner{}, func(s ScssExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "svelte",
		commit: "ae5199db47757f785e43a14b332118a5474de1a2",
		externals: []string{
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
		},
		// The four tag_name variants (start/script/style/end) each display as
		// "tag_name"; the four sigil externals (/>, @, #, /, :) each display as
		// their own literal; svelte_raw_text_each and
		// svelte_raw_text_snippet_arguments both display as "svelte_raw_text";
		// the remaining externals display exactly as their spec name.
		display: []string{
			"tag_name", "tag_name", "tag_name", "tag_name",
			"erroneous_end_tag_name", "/>", "_implicit_end_tag", "raw_text",
			"comment", "svelte_raw_text", "svelte_raw_text", "svelte_raw_text",
			"@", "#", "/", ":",
		},
		tokenCount:     svelteTokenCount,
		defaultSymbols: svelteDefaultSymTable,
		bind:           scannerBinder(SvelteExternalScanner{}, func(s SvelteExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "awk",
		commit: "34bbdc7cce8e803096f47b625979e34c1be38127",
		externals: []string{
			"concatenating_space",
			"_if_else_separator",
			"_no_space",
			"_func_call",
		},
		tokenCount:     awkTokenCount,
		defaultSymbols: awkDefaultSymTable,
		bind:           scannerBinder(AwkExternalScanner{}, func(s AwkExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "xml",
		commit: "5000ae8f22d11fbe93939b05c1e37cf21117162d",
		externals: []string{
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
		},
		// _start_tag_name and _end_tag_name both display as "Name"; the
		// remaining externals display exactly as their spec name.
		display: []string{
			"PITarget", "_pi_content", "Comment", "CharData",
			"CData", "xml-model", "xml-stylesheet", "Name",
			"Name", "_erroneous_end_name", "/>",
		},
		tokenCount:     xmlTokenCount,
		defaultSymbols: xmlDefaultSymTable,
		bind:           scannerBinder(XMLExternalScanner{}, func(s XMLExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "yaml",
		commit: "a1c4812a73ec5e089de8e441fdea3a921e8d5079",
		externals: []string{
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
		},
		// Many externals collapse onto a small set of shared display node
		// types (block/flow indicator punctuation, scalar kinds, tag/anchor/
		// alias markers); this table records the currently-shipped blob's
		// per-index display name directly rather than re-deriving the
		// aliasing rules.
		display: []string{
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
		},
		tokenCount:     yTokCount,
		defaultSymbols: yamlDefaultSymTable,
		bind:           scannerBinder(YamlExternalScanner{}, func(s *YamlExternalScanner) ([]int, any) { return s.binding.externalToToken, s.binding.symbols }),
	},
	{
		lang:   "agda",
		commit: "e8d47a6987effe34d5595baf321d82d3519a8527",
		externals: []string{
			"_newline",
			"_indent",
			"_dedent",
		},
		tokenCount:     agdaTokenCount,
		defaultSymbols: agdaDefaultSymTable,
		bind:           scannerBinder(AgdaExternalScanner{}, func(s AgdaExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "arduino",
		commit: "11dd46c9ae25135c473c0003a133bb06a484af0c",
		externals: []string{
			"raw_string_delimiter",
			"raw_string_content",
		},
		tokenCount:     arduinoTokenCount,
		defaultSymbols: arduinoDefaultSymTable,
		bind:           scannerBinder(ArduinoExternalScanner{}, func(s ArduinoExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "astro",
		commit: "213f6e6973d9b456c6e50e86f19f66877e7ef0ee",
		externals: []string{
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
		},
		// The three tag_name start variants and the end tag_name each display
		// as "tag_name"; _html_interpolation_start/_end display as the
		// literals "{"/"}"; _fragment_tag_delim displays as the literal ">";
		// the remaining externals display exactly as their spec name.
		display: []string{
			"tag_name", "tag_name", "tag_name", "tag_name",
			"erroneous_end_tag_name", "/>", "_implicit_end_tag", "raw_text",
			"comment", "{", "}", "frontmatter_js_block",
			"attribute_js_expr", "attribute_backtick_string", "permissible_text", ">",
		},
		tokenCount:     astroTokenCount,
		defaultSymbols: astroDefaultSymTable,
		bind:           scannerBinder(AstroExternalScanner{}, func(s AstroExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "bicep",
		commit: "bff59884307c0ab009bd5e81afd9324b46a6c0f9",
		externals: []string{
			"_external_asterisk",
			"_multiline_string_content",
		},
		// _external_asterisk displays as the literal "*"; the remaining
		// external displays exactly as its spec name.
		display:        []string{"*", "_multiline_string_content"},
		tokenCount:     bicepTokenCount,
		defaultSymbols: bicepDefaultSymTable,
		bind:           scannerBinder(BicepExternalScanner{}, func(s BicepExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "bitbake",
		commit: "a5d04fdb5a69a02b8fa8eb5525a60dfb5309b73b",
		externals: []string{
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
		},
		tokenCount:     bbTokenCount,
		defaultSymbols: bbDefaultSymTable,
		bind:           scannerBinder(BitbakeExternalScanner{}, func(s BitbakeExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "cairo",
		commit: "6238f609bea233040fe927858156dee5515a0745",
		externals: []string{
			"%{",
			"code_line",
			"_failure",
		},
		tokenCount:     cairoTokenCount,
		defaultSymbols: cairoDefaultSymTable,
		bind:           scannerBinder(CairoExternalScanner{}, func(s CairoExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "cuda",
		commit: "48b066f334f4cf2174e05a50218ce2ed98b6fd01",
		externals: []string{
			"raw_string_delimiter",
			"raw_string_content",
		},
		tokenCount:     cudaTokenCount,
		defaultSymbols: cudaDefaultSymTable,
		bind:           scannerBinder(CudaExternalScanner{}, func(s CudaExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "comment",
		commit: "66272d2b6c73fb61157541b69dd0a7ce7b42a5ad",
		externals: []string{
			"name",
			"invalid_token",
		},
		tokenCount:     commentTokenCount,
		defaultSymbols: commentDefaultSymTable,
		bind:           scannerBinder(CommentExternalScanner{}, func(s CommentExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "cooklang",
		commit: "4ebe237c1cf64cf3826fc249e9ec0988fe07e58e",
		externals: []string{
			"_newline",
		},
		tokenCount:     cooklangTokenCount,
		defaultSymbols: cooklangDefaultSymTable,
		bind:           scannerBinder(CooklangExternalScanner{}, func(s CooklangExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "cue",
		commit: "be0f609c73cc2929811a9bce0ed90ca71ea87604",
		externals: []string{
			"_multi_str_content",
			"_multi_bytes_content",
			"_raw_str_content",
			"_raw_bytes_content",
			"_multi_raw_str_content",
			"_multi_raw_bytes_content",
		},
		tokenCount:     cueTokenCount,
		defaultSymbols: cueDefaultSymTable,
		bind:           scannerBinder(CueExternalScanner{}, func(s CueExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "disassembly",
		commit: "0229c0211dba909c5d45129ac784a3f4d49c243a",
		externals: []string{
			"code_identifier",
			"instruction",
			"memory_dump",
			"_error_sentinel",
		},
		// code_identifier displays as the grammar-collapsed node name
		// "identifier"; the remaining externals display exactly as their
		// spec name.
		display:        []string{"identifier", "instruction", "memory_dump", "_error_sentinel"},
		tokenCount:     disasmTokenCount,
		defaultSymbols: disasmDefaultSymTable,
		bind:           scannerBinder(DisassemblyExternalScanner{}, func(s DisassemblyExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "djot",
		commit: "74fac1f53c6d52aeac104b6874e5506be6d0cfe6",
		externals: []string{
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
		},
		// Many externals collapse onto a small set of shared display node
		// types (heading/table/math/span punctuation markers); this table
		// records the currently-shipped blob's per-index display name
		// directly rather than re-deriving the aliasing rules.
		display: []string{
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
		},
		tokenCount:     djotTokenCount,
		defaultSymbols: djotDefaultSymTable,
		bind:           scannerBinder(DjotExternalScanner{}, func(s DjotExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "earthfile",
		commit: "5baef88717ad0156fd29a8b12d0d8245bb1096a8",
		externals: []string{
			"_indent",
			"_dedent",
		},
		tokenCount:     earthfileTokenCount,
		defaultSymbols: earthfileDefaultSymTable,
		bind:           scannerBinder(EarthfileExternalScanner{}, func(s EarthfileExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "dtd",
		commit: "5000ae8f22d11fbe93939b05c1e37cf21117162d",
		externals: []string{
			"PITarget",
			"_pi_content",
			"Comment",
		},
		tokenCount:     dtdTokenCount,
		defaultSymbols: dtdDefaultSymTable,
		bind:           scannerBinder(DtdExternalScanner{}, func(s DtdExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "dockerfile",
		commit: "971acdd908568b4531b0ba28a445bf0bb720aba5",
		externals: []string{
			"heredoc_marker",
			"heredoc_line",
			"heredoc_end",
			"heredoc_nl",
			"error_sentinel",
		},
		// heredoc_nl (external index 3) aliases to the grammar-internal display
		// node "_heredoc_nl"; the remaining four externals display exactly as
		// their spec name.
		display: []string{
			"heredoc_marker",
			"heredoc_line",
			"heredoc_end",
			"_heredoc_nl",
			"error_sentinel",
		},
		tokenCount:     dockerfileTokenCount,
		defaultSymbols: dockerfileDefaultSymTable,
		bind:           scannerBinder(DockerfileExternalScanner{}, func(s DockerfileExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "fennel",
		commit: "3f0f6b24d599e92460b969aabc4f4c5a914d15a0",
		externals: []string{
			"_hashfn_reader_macro_char",
			"_quote_reader_macro_char",
			"_quasi_quote_reader_macro_char",
			"_unquote_reader_macro_char",
			"__reader_macro_count",
			"__colon_string_start_mark",
			"__colon_string_end_mark",
			"shebang",
			"__token_count",
		},
		// The four reader-macro externals display as their literal character;
		// the remaining five display exactly as their spec name.
		display: []string{
			"#", "'", "`", ",",
			"__reader_macro_count", "__colon_string_start_mark", "__colon_string_end_mark",
			"shebang", "__token_count",
		},
		tokenCount:     fennelTokenCount,
		defaultSymbols: fennelDefaultSymTable,
		bind:           scannerBinder(FennelExternalScanner{}, func(s FennelExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "cobol",
		commit: "e99dbdc3d800d5fa2796476efd60af91f6b43d93",
		externals: []string{
			"_WHITE_SPACES",
			"_LINE_PREFIX_COMMENT",
			"_LINE_SUFFIX_COMMENT",
			"_LINE_COMMENT",
			"comment_entry",
			"_multiline_string",
		},
		tokenCount:     cobolTokenCount,
		defaultSymbols: cobolDefaultSymTable,
		bind:           scannerBinder(CobolExternalScanner{}, func(s CobolExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "crystal",
		commit: "51ad1411de9414b4600227553bb70953c352a627",
		externals: []string{
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
		},
		// Many externals collapse onto a small set of shared display node names
		// ("{", "operator", keyword text, and so on); this table records the
		// currently-shipped blob's actual per-index display name directly
		// rather than re-deriving the aliasing rules.
		display: []string{
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
		},
		tokenCount:     cryTokenCount,
		defaultSymbols: cryDefaultSymTable,
		bind:           scannerBinder(CrystalExternalScanner{}, func(s CrystalExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "dhall",
		commit: "62013259b26ac210d5de1abf64cf1b047ef88000",
		externals: []string{
			"block_comment_content",
			"block_comment_end",
		},
		tokenCount:     dhallTokenCount,
		defaultSymbols: dhallDefaultSymTable,
		bind:           scannerBinder(DhallExternalScanner{}, func(s DhallExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "firrtl",
		commit: "8503d3a0fe0f9e427863cb0055699ff2d29ae5f5",
		externals: []string{
			"_newline",
			"_indent",
			"_dedent",
		},
		tokenCount:     firrtlTokenCount,
		defaultSymbols: firrtlDefaultSymTable,
		bind:           scannerBinder(FirrtlExternalScanner{}, func(s FirrtlExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "fish",
		commit: "fa2143f5d66a9eb6c007ba9173525ea7aaafe788",
		externals: []string{
			"_concat",
			"_brace_concat",
			"_concat_list",
			"_begin_brace",
		},
		// _begin_brace (external index 3) aliases to the grammar-internal
		// display node "{"; the remaining three externals display exactly as
		// their spec name.
		display: []string{
			"_concat",
			"_brace_concat",
			"_concat_list",
			"{",
		},
		tokenCount:     fishTokenCount,
		defaultSymbols: fishDefaultSymTable,
		bind:           scannerBinder(FishExternalScanner{}, func(s FishExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "foam",
		commit: "472c24f11a547820327fb1be565bcfff98ea96a4",
		externals: []string{
			"identifier",
			"boolean",
			"_eof",
		},
		tokenCount:     foamTokenCount,
		defaultSymbols: foamDefaultSymTable,
		bind:           scannerBinder(FoamExternalScanner{}, func(s FoamExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "fortran",
		commit: "2880b7aab4fb7cc618de1ef3d4c6d93b2396c031",
		externals: []string{
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
		},
		// _string_literal_kind (index 5) displays as "identifier",
		// _do_label (index 9) displays as "statement_label_reference", and
		// _do_label_continue (index 11) displays as "statement_label"; the
		// remaining externals display exactly as their spec name.
		display: []string{
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
		},
		tokenCount:     ftnTokenCount,
		defaultSymbols: ftnDefaultSymTable,
		bind:           scannerBinder(FortranExternalScanner{}, func(s FortranExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "fsharp",
		commit: "5141851c278a99958469eb1736c7afc4ec738e47",
		externals: []string{
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
		},
		// _struct_begin displays as "struct" and _interface_begin displays as
		// "interface"; the remaining externals display exactly as their spec
		// name (literal-valued externals display as their literal text).
		display: []string{
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
		},
		tokenCount:     fsTokenCount,
		defaultSymbols: fsDefaultSymTable,
		bind:           scannerBinder(FsharpExternalScanner{}, func(s FsharpExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "gdscript",
		commit: "89e66b6bdc002ab976283f277cbb48b780c5d0e9",
		externals: []string{
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
		},
		// _string_start/_string_end display as the literal quote character,
		// _string_name_start/_node_path_start display with their sigil prefix,
		// and the four close-bracket externals (indexes 8-11) alias to shared
		// literal display nodes; the remaining externals display exactly as
		// their spec name.
		display: []string{
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
		},
		tokenCount:     gdsTokenCount,
		defaultSymbols: gdsDefaultSymTable,
		bind:           scannerBinder(GdscriptExternalScanner{}, func(s GdscriptExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "gitcommit",
		commit: "a716678c0f00645fed1e6f1d0eb221481dbd6f6d",
		externals: []string{
			"_conventional_type",
			"_trailer_value",
		},
		// _conventional_type displays as "type"; _trailer_value displays
		// exactly as its spec name.
		display: []string{
			"type",
			"_trailer_value",
		},
		tokenCount:     gitcommitTokenCount,
		defaultSymbols: gitcommitDefaultSymTable,
		bind:           scannerBinder(GitcommitExternalScanner{}, func(s GitcommitExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "gleam",
		commit: "6ea757f7eb8d391dbf24dbb9461990757946dd5e",
		externals: []string{
			"quoted_content",
			"doc_comment_content",
		},
		tokenCount:     gleamTokenCount,
		defaultSymbols: gleamDefaultSymTable,
		bind:           scannerBinder(GleamExternalScanner{}, func(s GleamExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "gn",
		commit: "bc06955bc1e3c9ff8e9b2b2a55b38b94da923c05",
		externals: []string{
			"_string_content",
		},
		tokenCount:     gnTokenCount,
		defaultSymbols: gnDefaultSymTable,
		bind:           scannerBinder(GnExternalScanner{}, func(s GnExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "godot_resource",
		commit: "302c1895f54bf74d53a08572f7b26a6614209adc",
		externals: []string{
			"string",
		},
		tokenCount:     godotResourceTokenCount,
		defaultSymbols: godotResourceDefaultSymTable,
		bind:           scannerBinder(GodotResourceExternalScanner{}, func(s GodotResourceExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "hack",
		commit: "1a7ded90288189746c54861ac144ede97df95081",
		externals: []string{
			"_heredoc_start",
			"_heredoc_start_newline",
			"_heredoc_body",
			"_heredoc_end_newline",
			"_heredoc_end",
			"_embedded_opening_brace",
		},
		// _heredoc_start_newline and _heredoc_end_newline display as "\n", and
		// _embedded_opening_brace displays as "{"; the remaining externals
		// display exactly as their spec name.
		display: []string{
			"_heredoc_start",
			"\n",
			"_heredoc_body",
			"\n",
			"_heredoc_end",
			"{",
		},
		tokenCount:     hackTokenCount,
		defaultSymbols: hackDefaultSymTable,
		bind:           scannerBinder(HackExternalScanner{}, func(s HackExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "haxe",
		commit: "f2a2394d9ca7a6099f78d8b0d178530e7c9a8e26",
		externals: []string{
			"_lookback_semicolon",
			"_closing_brace_marker",
			"_closing_brace_unmarker",
		},
		tokenCount:     haxeTokenCount,
		defaultSymbols: haxeDefaultSymTable,
		bind:           scannerBinder(HaxeExternalScanner{}, func(s HaxeExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "hlsl",
		commit: "bab9111922d53d43668fabb61869bec51bbcb915",
		externals: []string{
			"raw_string_delimiter",
			"raw_string_content",
		},
		tokenCount:     hlslTokenCount,
		defaultSymbols: hlslDefaultSymTable,
		bind:           scannerBinder(HlslExternalScanner{}, func(s HlslExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "janet",
		commit: "d183186995204314700be3e9e0a48053ea16b350",
		externals: []string{
			"long_buf_lit",
			"long_str_lit",
		},
		tokenCount:     janetTokenCount,
		defaultSymbols: janetDefaultSymTable,
		bind:           scannerBinder(JanetExternalScanner{}, func(s JanetExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "jsdoc",
		commit: "658d18dcdddb75c760363faa4963427a7c6b52db",
		externals: []string{
			"type",
			"code_block_line",
		},
		tokenCount:     jsdocTokenCount,
		defaultSymbols: jsdocDefaultSymTable,
		bind:           scannerBinder(JsdocExternalScanner{}, func(s JsdocExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "jsonnet",
		commit: "ddd075f1939aed8147b7aa67f042eda3fce22790",
		externals: []string{
			"_string_start",
			"_string_content",
			"_string_end",
		},
		// All three externals drop their leading underscore in the blob's
		// display name.
		display: []string{
			"string_start",
			"string_content",
			"string_end",
		},
		tokenCount:     jsonnetTokenCount,
		defaultSymbols: jsonnetDefaultSymTable,
		bind:           scannerBinder(JsonnetExternalScanner{}, func(s JsonnetExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "just",
		commit: "60df3d5b3fda2a22fdb3621226cafab50b763663",
		externals: []string{
			"_indent",
			"_dedent",
			"_newline",
			"text",
			"error_recovery",
		},
		tokenCount:     justTokenCount,
		defaultSymbols: justDefaultSymTable,
		bind:           scannerBinder(JustExternalScanner{}, func(s JustExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "kconfig",
		commit: "9ac99fe4c0c27a35dc6f757cef534c646e944881",
		externals: []string{
			"_help_text",
		},
		// _help_text displays as "text".
		display: []string{
			"text",
		},
		tokenCount:     kconfigTokenCount,
		defaultSymbols: kconfigDefaultSymTable,
		bind:           scannerBinder(KconfigExternalScanner{}, func(s KconfigExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "kdl",
		commit: "b37e3d58e5c5cf8d739b315d6114e02d42e66664",
		externals: []string{
			"_eof",
			"multi_line_comment",
			"_raw_string",
		},
		tokenCount:     kdlTokenCount,
		defaultSymbols: kdlDefaultSymTable,
		bind:           scannerBinder(KdlExternalScanner{}, func(s KdlExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "less",
		commit: "2bd739e106a3485bca210cf7b6d25ba09fd10dff",
		externals: []string{
			"_descendant_operator",
		},
		tokenCount:     lessTokenCount,
		defaultSymbols: lessDefaultSymTable,
		bind:           scannerBinder(LessExternalScanner{}, func(s LessExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "luau",
		commit: "a8914d6c1fc5131f8e1c13f769fa704c9f5eb02f",
		externals: []string{
			"_block_comment_start",
			"_block_comment_content",
			"_block_comment_end",
			"_block_string_start",
			"_block_string_content",
			"_block_string_end",
		},
		// The comment and string delimiters collapse onto shared literal/generic
		// display names rather than each keeping its own rule name.
		display: []string{
			"[[",
			"comment_content",
			"]]",
			"[[",
			"string_content",
			"]]",
		},
		tokenCount:     luauTokenCount,
		defaultSymbols: luauDefaultSymTable,
		bind:           scannerBinder(LuauExternalScanner{}, func(s LuauExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "markdown_inline",
		commit: "f969cd3ae3f9fbd4e43205431d0ae286014c05b5",
		externals: []string{
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
		},
		// Several externals collapse onto shared display names: the code span
		// start/close pair both display as "code_span_delimiter", the star/
		// underscore emphasis close pair and the strikethrough close token all
		// display as "emphasis_delimiter", and the latex span start/close pair
		// both display as "latex_span_delimiter".
		display: []string{
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
		},
		tokenCount:     mdiTokenCount,
		defaultSymbols: mdiDefaultSymTable,
		bind:           scannerBinder(MarkdownInlineExternalScanner{}, func(s MarkdownInlineExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "matlab",
		commit: "574dde565caddf8cf44eec7df3cb89eb96053ed7",
		externals: []string{
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
		},
		// The single/double quote string delimiters and the transpose token all
		// collapse onto punctuation-literal display names, and _catch_identifier
		// aliases to "identifier" -- unlike _external_identifier at index 13,
		// which keeps its own rule name with no alias.
		display: []string{
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
		},
		tokenCount:     matTokenCount,
		defaultSymbols: matDefaultSymTable,
		bind:           scannerBinder(MatlabExternalScanner{}, func(s MatlabExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "mojo",
		commit: "c307dab71a43add26b4715f14e2d6de2a42e6007",
		externals: []string{
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
		},
		tokenCount:     mojoTokenCount,
		defaultSymbols: mojoDefaultSymTable,
		bind:           scannerBinder(MojoExternalScanner{}, func(s MojoExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "move",
		commit: "12906b341de7cef81cf03d7d91dae51d8a9299e7",
		externals: []string{
			"_block_doc_comment_marker",
			"_block_comment_content",
			"_doc_line_comment",
			"_error_sentinel",
		},
		tokenCount:     moveTokenCount,
		defaultSymbols: moveDefaultSymTable,
		bind:           scannerBinder(MoveExternalScanner{}, func(s MoveExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "nginx",
		commit: "47ade644d754cce57974aac44d2c9450e823d4f4",
		externals: []string{
			"_newline",
			"_indent",
			"_dedent",
		},
		tokenCount:     nginxTokenCount,
		defaultSymbols: nginxDefaultSymTable,
		bind:           scannerBinder(NginxExternalScanner{}, func(s NginxExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "nickel",
		commit: "b5b6cc3bc7b9ea19f78fed264190685419cd17a8",
		externals: []string{
			"multstr_start",
			"multstr_end",
			"_str_start",
			"_str_end",
			"interpolation_start",
			"interpolation_end",
			"quoted_enum_tag_start",
			"comment",
		},
		tokenCount:     nickelTokenCount,
		defaultSymbols: nickelDefaultSymTable,
		bind:           scannerBinder(NickelExternalScanner{}, func(s NickelExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "nim",
		commit: "9b4ede21a6ca866d29263f6b66c070961bc622b4",
		externals: []string{
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
		},
		// _block_comment_content, _block_documentation_comment_content, and
		// comment_content all collapse onto the shared "comment_content"
		// display name; _sigil_operator and _prefix_operator both display as
		// "operator"; the literal "," external keeps its literal display; and
		// _case_of displays as the "of" keyword.
		display: []string{
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
		},
		tokenCount:     nimTokLen,
		defaultSymbols: nimDefaultSymTable,
		bind:           scannerBinder(NimExternalScanner{}, func(s NimExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "norg",
		commit: "d89d95af13d409f30a6c7676387bde311ec4a2c8",
		externals: []string{
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
		},
		// Norg's 122 externals collapse heavily onto shared generic display
		// names in the shipped blob (many pairs/groups alias to "_word",
		// "_open", "_close", "_prefix", "_delimiter", "_begin", or "_end"
		// rather than each keeping its own upstream rule name), so this pins
		// the actual display name observed per index rather than asserting
		// against spec.Externals directly.
		display: []string{
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
		},
		tokenCount:     norgTokenCount,
		defaultSymbols: norgDefaultSymTable,
		bind:           scannerBinder(NorgExternalScanner{}, func(s NorgExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "nushell",
		commit: "bb3f533e5792260291945e1f329e1f0a779def6e",
		externals: []string{
			"raw_string_begin",
			"raw_string_content",
			"raw_string_end",
		},
		// raw_string_content displays as "string_content".
		display: []string{
			"raw_string_begin",
			"string_content",
			"raw_string_end",
		},
		tokenCount:     nushellTokenCount,
		defaultSymbols: nushellDefaultSymTable,
		bind:           scannerBinder(NushellExternalScanner{}, func(s NushellExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "odin",
		commit: "d2ca8efb4487e156a60d5bd6db2598b872629403",
		externals: []string{
			"_newline",
			"_backslash",
			"_nl_comma",
			"float",
			"block_comment",
			"{",
			"\"",
		},
		// _nl_comma displays as the "," literal; the "{" and "\"" externals
		// keep their literal display names.
		display: []string{
			"_newline",
			"_backslash",
			",",
			"float",
			"block_comment",
			"{",
			"\"",
		},
		tokenCount:     odinTokenCount,
		defaultSymbols: odinDefaultSymTable,
		bind:           scannerBinder(OdinExternalScanner{}, func(s OdinExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "org",
		commit: "64cfbc213f5a83da17632c95382a5a0a2f3357c1",
		externals: []string{
			"_liststart",
			"_listend",
			"_listitemend",
			"bullet",
			"_stars",
			"_sectionend",
			"_eof",
		},
		tokenCount:     orgTokenCount,
		defaultSymbols: orgDefaultSymTable,
		bind:           scannerBinder(OrgExternalScanner{}, func(s OrgExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:   "pkl",
		commit: "a02fc36f6001a22e7fdf35eaabbadb7b39c74ba5",
		externals: []string{
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
		},
		// _open_subscript_bracket, _open_argument_paren, and _binary_minus
		// display as their literal characters rather than their rule names.
		display: []string{
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
		},
		tokenCount:     pklTokenCount,
		defaultSymbols: pklDefaultSymTable,
		bind:           scannerBinder(PklExternalScanner{}, func(s PklExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:           "properties",
		commit:         "6310671b24d4e04b803577b1c675d765cbd5773b",
		externals:      []string{"_eof"},
		display:        []string{"_eof"},
		tokenCount:     propertiesTokenCount,
		defaultSymbols: propertiesDefaultSymTable,
		bind:           scannerBinder(PropertiesExternalScanner{}, func(s PropertiesExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:           "pug",
		commit:         "13e9195370172c86a8b88184cc358b23b677cc46",
		externals:      []string{"_newline", "_indent", "_dedent"},
		display:        []string{"_newline", "_indent", "_dedent"},
		tokenCount:     pugTokenCount,
		defaultSymbols: pugDefaultSymTable,
		bind:           scannerBinder(PugExternalScanner{}, func(s PugExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:           "tcl",
		commit:         "8f11ac7206a54ed11210491cee1e0657e2962c47",
		externals:      []string{"_concat", "_immediate"},
		display:        []string{"_concat", "_immediate"},
		tokenCount:     tclTokenCount,
		defaultSymbols: tclDefaultSymTable,
		bind:           scannerBinder(TclExternalScanner{}, func(s TclExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:           "uxntal",
		commit:         "ad9b638b914095320de85d59c49ab271603af048",
		externals:      []string{"comment"},
		display:        []string{"comment"},
		tokenCount:     uxntalTokenCount,
		defaultSymbols: uxntalDefaultSymTable,
		bind:           scannerBinder(UxntalExternalScanner{}, func(s UxntalExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:           "squirrel",
		commit:         "072c969749e66f000dba35a33c387650e203e96e",
		externals:      []string{"verbatim_string"},
		display:        []string{"verbatim_string"},
		tokenCount:     squirrelTokenCount,
		defaultSymbols: squirrelDefaultSymTable,
		bind:           scannerBinder(SquirrelExternalScanner{}, func(s SquirrelExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:           "racket",
		commit:         "56b57807f86aa4ddb14892572b318edd4bc90ebe",
		externals:      []string{"_here_string_body"},
		display:        []string{"_here_string_body"},
		tokenCount:     racketTokenCount,
		defaultSymbols: racketDefaultSymTable,
		bind:           scannerBinder(RacketExternalScanner{}, func(s RacketExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:           "yuck",
		commit:         "e877f6ade4b77d5ef8787075141053631ba12318",
		externals:      []string{"_unescaped_single_quote_string_fragment", "_unescaped_double_quote_string_fragment", "_unescaped_backtick_string_fragment"},
		display:        []string{"_unescaped_single_quote_string_fragment", "_unescaped_double_quote_string_fragment", "_unescaped_backtick_string_fragment"},
		tokenCount:     yuckTokenCount,
		defaultSymbols: yuckDefaultSymTable,
		bind:           scannerBinder(YuckExternalScanner{}, func(s YuckExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:           "tablegen",
		commit:         "b1170880c61355aaf38fc06f4af7d3c55abdabc4",
		externals:      []string{"multiline_comment"},
		display:        []string{"multiline_comment"},
		tokenCount:     tablegenTokenCount,
		defaultSymbols: tablegenDefaultSymTable,
		bind:           scannerBinder(TablegenExternalScanner{}, func(s TablegenExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:           "wgsl",
		commit:         "40259f3c77ea856841a4e0c4c807705f3e4a2b65",
		externals:      []string{"block_comment"},
		display:        []string{"block_comment"},
		tokenCount:     wgslTokenCount,
		defaultSymbols: wgslDefaultSymTable,
		bind:           scannerBinder(WgslExternalScanner{}, func(s WgslExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:           "wolfram",
		commit:         "63ebdac6f040d9082d3d8fa88be96ce24549adc5",
		externals:      []string{"comment"},
		display:        []string{"comment"},
		tokenCount:     wolframTokenCount,
		defaultSymbols: wolframDefaultSymTable,
		bind:           scannerBinder(WolframExternalScanner{}, func(s WolframExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:           "ron",
		commit:         "78938553b93075e638035f624973083451b29055",
		externals:      []string{"_string_content", "raw_string", "float", "block_comment"},
		display:        []string{"_string_content", "raw_string", "float", "block_comment"},
		tokenCount:     ronTokenCount,
		defaultSymbols: ronDefaultSymTable,
		bind:           scannerBinder(RonExternalScanner{}, func(s RonExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:           "templ",
		commit:         "1c6db04effbcd7773c826bded9783cbc3061bd55",
		externals:      []string{"css_property_value", "script_block_text", "switch_element_text", "element_text"},
		display:        []string{"css_property_value", "script_block_text", "element_text", "element_text"},
		tokenCount:     templTokenCount,
		defaultSymbols: templDefaultSymTable,
		bind:           scannerBinder(TemplExternalScanner{}, func(s TemplExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:           "vue",
		commit:         "ce8011a414fdf8091f4e4071752efc376f4afb08",
		externals:      []string{"_start_tag_name", "_script_start_tag_name", "_style_start_tag_name", "_end_tag_name", "erroneous_end_tag_name", "/>", "_implicit_end_tag", "raw_text", "comment", "_template_start_tag_name", "_text_fragment", "_interpolation_text"},
		display:        []string{"tag_name", "tag_name", "tag_name", "tag_name", "erroneous_end_tag_name", "/>", "_implicit_end_tag", "raw_text", "comment", "tag_name", "_text_fragment", "raw_text"},
		tokenCount:     vueTokenCount,
		defaultSymbols: vueDefaultSymTable,
		bind:           scannerBinder(VueExternalScanner{}, func(s VueExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:           "teal",
		commit:         "05d276e737055e6f77a21335b7573c9d3c091e2f",
		externals:      []string{"comment", "_long_string_start", "_long_string_char", "_long_string_end", "_short_string_start", "_short_string_char", "_short_string_end"},
		display:        []string{"comment", "long_string_start", "_long_string_char", "long_string_end", "short_string_start", "_short_string_char", "short_string_end"},
		tokenCount:     tealTokenCount,
		defaultSymbols: tealDefaultSymTable,
		bind:           scannerBinder(TealExternalScanner{}, func(s TealExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:           "starlark",
		commit:         "a453dbf3ba433db0e5ec621a38a7e59d72e4dc69",
		externals:      []string{"_newline", "_indent", "_dedent", "string_start", "_string_content", "escape_interpolation", "string_end", "comment", "]", ")", "}", "except"},
		display:        []string{"_newline", "_indent", "_dedent", "string_start", "_string_content", "escape_interpolation", "string_end", "comment", "]", ")", "}", "except"},
		tokenCount:     slTokenCount,
		defaultSymbols: slDefaultSymTable,
		bind:           scannerBinder(StarlarkExternalScanner{}, func(s StarlarkExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:           "rescript",
		commit:         "43c2f1f35024918d415dc933d4cc534d6419fedf",
		externals:      []string{"_newline", "comment", "_newline_and_comment", "\"", "`", "_template_chars", "_lparen", "_rparen", "_list_constructor", "_dict_constructor", "_decorator", "_decorator_inline"},
		display:        []string{"_newline", "comment", "comment", "\"", "`", "_template_chars", "_lparen", "_rparen", "_list_constructor", "dict", "decorator_identifier", "decorator_identifier"},
		tokenCount:     rescriptTokenCount,
		defaultSymbols: rescriptDefaultSymTable,
		bind:           scannerBinder(RescriptExternalScanner{}, func(s RescriptExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:           "typst",
		commit:         "46cf4ded12ee974a70bf8457263b67ad7ee0379d",
		externals:      []string{"_indent", "_dedent", "_redent", "_line_start_check", "_token_content", "_token_strong", "_token_emph", "_barrier", "_token_bracket", "_token_section", "_termination", "_token_inlined_item_end", "_token_inlined_stmt_end", "_token_blocked_expr_end", "_token_math_letter", "_token_math_ident", "_token_math_frac", "_token_math_group_end", "_token_else", "_token_unit", "_token_url", "_token_item", "_token_term", "_token_head_1", "_token_head_2", "_token_head_3", "_token_head_4", "_token_head_5", "_token_head_p", "_token_string_blob", "_token_raw_span_blob", "_token_raw_blck_ldlm", "_token_raw_blck_rdlm", "_token_raw_blck_blob", "_token_raw_lang", "_token_identifier", "_token_label", "_token_anti_markup", "comment", "_sp", "_immediate", "_immediate_paren", "_immediate_brack", "_immediate_ident", "_immediate_math_call", "_immediate_math_apply", "_immediate_math_field", "_immediate_math_prime", "_recovery"},
		display:        []string{"_indent", "_dedent", "_redent", "_line_start_check", "[", "*", "_", "_barrier", "text", "_token_section", "_termination", "end", "end", "sep", "letter", "ident", "/", ")", "else", "_token_unit", "_token_url", "-", "/", "=", "==", "===", "====", "=====", "======", "_token_string_blob", "blob", "```", "```", "blob", "ident", "_token_identifier", "_token_label", "_token_anti_markup", "comment", "_sp", "_immediate", "_immediate_paren", "_immediate_brack", "_immediate_ident", "_immediate_math_call", "_immediate_math_apply", "_immediate_math_field", "_immediate_math_prime", "_recovery"},
		tokenCount:     typstTokenCount,
		defaultSymbols: typstDefaultSymTable,
		bind:           scannerBinder(TypstExternalScanner{}, func(s TypstExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:           "purescript",
		commit:         "f541f95ffd6852fbbe88636317c613285bc105af",
		externals:      []string{"_layout_semicolon", "_layout_start", "_layout_end", "_dot", "where", "_tyconsym", "comment", "comma", "@", "=", "|", "in", "\\n", "empty_file"},
		display:        []string{"_layout_semicolon", "_layout_start", "_layout_end", "_dot", "where", "type_operator", "comment", "comma", "@", "=", "|", "in", "_token1", "empty_file"},
		tokenCount:     psTokenCount,
		defaultSymbols: psDefaultSymTable,
		bind:           scannerBinder(PurescriptExternalScanner{}, func(s PurescriptExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:           "rst",
		commit:         "4e562e1598b95b93db4f3f64fe40ddefbc677a15",
		externals:      []string{"_newline", "_blankline", "_indent", "_newline_indent", "_dedent", "_overline", "_underline", "transition", "_char_bullet", "_numeric_bullet", "_field_mark", "_field_mark_end", "_literal_indented_block_mark", "_literal_quoted_block_mark", "_quoted_literal_block", "_line_block_mark", "_attribution_mark", "_doctest_block_mark", "_text", "emphasis", "strong", "_interpreted_text", "_interpreted_text_prefix", "_role_name_prefix", "_role_name_suffix", "literal", "substitution_reference", "inline_target", "footnote_reference", "citation_reference", "reference", "standalone_hyperlink", "_explicit_markup_start", "_footnote_label", "_citation_label", "_target_name", "_anonymous_target_mark", "_directive_name", "_substitution_mark", "_empty_comment", "_invalid_token"},
		display:        []string{"_newline", "_blankline", "_indent", "_newline_indent", "_dedent", "adornment", "adornment", "transition", "bullet", "bullet", ":", ":", "::", "::", "literal_block", "|", "--", "_doctest_block_mark", "text", "emphasis", "strong", "interpreted_text", "interpreted_text", "role", "role", "literal", "substitution_reference", "inline_target", "footnote_reference", "citation_reference", "reference", "standalone_hyperlink", "..", "label", "label", "name", "__", "type", "substitution", "comment", "_invalid_token"},
		tokenCount:     rstTokenCount,
		defaultSymbols: rstDefaultSymTable,
		bind:           scannerBinder(RstExternalScanner{}, func(s RstExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:           "tlaplus",
		commit:         "add40814fda369f6efd989977b2c498aaddde984",
		externals:      []string{"leading_extramodular_text", "trailing_extramodular_text", "_indent", "_bullet", "_dedent", "_begin_proof", "_begin_proof_step", "PROOF", "BY", "OBVIOUS", "OMITTED", "QED", "WF_", "SF_", "_notify_pcal_algorithm_start", "_notify_pcal_algorithm_end", "_double_excl", "_error_sentinel"},
		display:        []string{"extramodular_text", "extramodular_text", "_indent", "_bullet", "_dedent", "_begin_proof", "_begin_proof_step", "PROOF", "BY", "OBVIOUS", "OMITTED", "QED", "WF_", "SF_", "_notify_pcal_algorithm_start", "_notify_pcal_algorithm_end", "!!", "_error_sentinel"},
		tokenCount:     tlaTokenCount,
		defaultSymbols: tlaDefaultSymTable,
		bind:           scannerBinder(TlaplusExternalScanner{}, func(s TlaplusExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
	{
		lang:           "vhdl",
		commit:         "a09b8dc58b59abf959e798849f12bc473b07bbb2",
		externals:      []string{"identifier", "ABS", "ACCESS", "AFTER", "ALIAS", "ALL", "AND", "ARCHITECTURE", "ARRAY", "ASSERT", "ASSUME", "ATTRIBUTE", "BEGIN", "BLOCK", "BODY", "BUFFER", "BUS", "CASE", "COMPONENT", "CONFIGURATION", "CONSTANT", "CONTEXT", "COVER", "DEFAULT", "DISCONNECT", "DOWNTO", "ELSE", "ELSIF", "END", "ENTITY", "EXIT", "FAIRNESS", "FILE", "FOR", "FORCE", "FUNCTION", "GENERATE", "GENERIC", "GROUP", "GUARDED", "IF", "IMPURE", "IN", "INERTIAL", "INOUT", "IS", "LABEL", "LIBRARY", "LINKAGE", "LITERAL", "LOOP", "MAP", "MOD", "NAND", "NEW", "NEXT", "NOR", "NOT", "NULL", "OF", "ON", "OPEN", "OR", "OTHERS", "OUT", "PACKAGE", "PARAMETER", "PORT", "POSTPONED", "PROCEDURE", "PROCESS", "PROPERTY", "PROTECTED", "PRIVATE", "PURE", "RANGE", "RECORD", "REGISTER", "REJECT", "RELEASE", "REM", "REPORT", "RESTRICT", "RETURN", "ROL", "ROR", "SELECT", "SEQUENCE", "SEVERITY", "SIGNAL", "SHARED", "SLA", "SLL", "SRA", "SRL", "STRONG", "SUBTYPE", "THEN", "TO", "TRANSPORT", "TYPE", "UNAFFECTED", "UNITS", "UNTIL", "USE", "VARIABLE", "VIEW", "VMODE", "VPKG", "VPROP", "VUNIT", "WAIT", "WHEN", "WHILE", "WITH", "XNOR", "XOR", "reserved_end_marker", "directive_body", "directive_constant_builtin", "directive_error", "directive_protect", "directive_warning", "_directive_newline", "_grave_accent", "box", "delimiter_end_marker", "decimal_integer", "decimal_float", "based_base", "based_integer", "based_float", "character_literal", "string_literal", "string_literal_std_logic", "bit_string_length", "bit_string_base", "bit_string_value", "operator_symbol", "_line_comment_start", "_block_comment_start", "_block_comment_end", "comment_content", "token_end_marker", "attribute_function", "attribute_impure_function", "attribute_mode_view", "attribute_pure_function", "attribute_range", "attribute_signal", "attribute_subtype", "attribute_type", "attribute_value", "library_attribute", "library_constant", "library_constant_boolean", "library_constant_character", "library_constant_debug", "library_constant_env", "library_constant_standard", "library_constant_std_logic", "library_constant_unit", "library_function", "library_namespace", "library_type", "_end_of_file", "error_sentinel"},
		display:        []string{"identifier", "abs", "access", "after", "alias", "ALL", "and", "architecture", "array", "assert", "ASSUME", "attribute", "begin", "block", "body", "buffer", "bus", "case", "component", "configuration", "constant", "context", "COVER", "DEFAULT", "disconnect", "downto", "else", "elsif", "end", "entity", "exit", "FAIRNESS", "file", "for", "force", "function", "generate", "generic", "group", "guarded", "if", "impure", "in", "inertial", "inout", "is", "label", "library", "linkage", "literal", "loop", "map", "mod", "nand", "new", "next", "nor", "not", "null", "of", "on", "OPEN", "or", "OTHERS", "out", "package", "parameter", "port", "postponed", "procedure", "process", "property", "protected", "private", "pure", "range", "record", "register", "reject", "release", "rem", "report", "RESTRICT", "return", "rol", "ror", "select", "sequence", "severity", "signal", "shared", "sla", "sll", "sra", "srl", "STRONG", "subtype", "then", "to", "transport", "type", "unaffected", "units", "until", "use", "variable", "view", "VMODE", "VPKG", "VPROP", "vunit", "wait", "when", "while", "with", "xnor", "xor", "reserved_end_marker", "directive_body", "directive_constant_builtin", "directive_error", "directive_protect", "directive_warning", "_directive_newline", "_grave_accent", "<>", "delimiter_end_marker", "decimal_integer", "decimal_float", "based_base", "based_integer", "based_float", "character_literal", "string_literal", "string_literal_std_logic", "bit_string_length", "bit_string_base", "bit_string_value", "operator_symbol", "_line_comment_start", "_block_comment_start", "_block_comment_end", "comment_content", "token_end_marker", "attribute_function", "attribute_impure_function", "attribute_mode_view", "attribute_pure_function", "attribute_range", "attribute_signal", "attribute_subtype", "attribute_type", "attribute_value", "library_attribute", "library_constant", "library_constant_boolean", "library_constant_character", "library_constant_debug", "library_constant_env", "library_constant_standard", "library_constant_std_logic", "library_constant_unit", "library_function", "library_namespace", "library_type", "_end_of_file", "error_sentinel"},
		tokenCount:     vhdlTokenCount,
		defaultSymbols: vhdlDefaultSymTable,
		bind:           scannerBinder(VhdlExternalScanner{}, func(s VhdlExternalScanner) ([]int, any) { return s.externalToToken, s.symbols }),
	},
}
