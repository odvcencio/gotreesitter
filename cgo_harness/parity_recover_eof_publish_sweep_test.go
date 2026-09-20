//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// recoverEOFPublishCandidates is review round-2 finding B-B's corrected
// moved-input list, restricted to the 19 grammars a full-shape C-oracle
// comparison confirms are safe. An independent wide-corpus sweep (all 95
// printable ASCII singles, the empty string, whitespace runs, comment-opener
// and operator fragments, and short two/three-character alphanumeric
// fragments) against all 182 default=true grammars in
// testdata/c_recovery_gate_fleet.json found 277 (grammar, input) pairs
// across 21 grammars whose Go root shape moves from a one-child grammar
// root to a childless ERROR root once tryPublishCRecoverEOFRoot publishes
// the C-recovery lineage's bare recover_eof root. Two of those 21 —
// c_sharp and earthfile — are excluded here because the C oracle
// disagrees; see recoverEOFPublishRouteDivergences and
// TestParityRecoverEOFRouteDivergences below. This is the exact grammar
// set CRecoverEOFBareRootReceipted (parser_recover_c.go) receipts; a test
// asserts the two stay equal so neither can drift from the other.
//
// The round-1 receipt (36 pairs across 5 grammars, corn/dtd/jsdoc/
// powershell/vhdl) is superseded by this one: its 19-character input
// alphabet had no letters, digits, whitespace, or empty string, which is
// exactly where the other grammars — and both regressions — move.
//
// This count is itself a lower bound (review round-3 finding N5): an
// independent, roughly 3x wider corpus (NUL and truncated/valid multi-byte
// UTF-8 in addition to this sweep's inputs) found 406 moved pairs across
// the same 19 receipted grammars plus 63 more suppressed pairs across
// c_sharp and earthfile — 469 total. Every one of the 192 pairs beyond
// this file's 214 receipted candidates was independently verified against
// the C oracle with zero mismatches. The grammar set is unaffected: no
// grammar outside the 19 receipted here, or the two route-divergence
// exclusions, moved in that wider corpus either. Widening this file's
// candidate list to match is tracked as a follow-up, not done here.
var recoverEOFPublishCandidates = map[string][]string{
	"corn":       {"", "!", "#", "$", "$$", "%", "&", "(", "()", ")", "*", "*/", ",", "/", "/*", "0", "0b", "1", "1a", "2", "3", "4", "5", "6", "7", "8", "9", ":", "::", ";", ";;", "<", ">", "?", "@", "A", "AB", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M", "N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z", "\\", "^", "_", "__", "`", "``", "a", "a1", "ab", "b", "c", "d", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "u8", "v", "w", "x", "y", "z", "|", "~"},
	"cpon":       {""},
	"dhall":      {""},
	"dot":        {""},
	"dtd":        {"!", "#", "$", "$$", "-", ".", "/", "//", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9", ":", "@", "\\", "]", "^", "`", "``", "{", "{}", "}", "~"},
	"ebnf":       {""},
	"facility":   {""},
	"fidl":       {"", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9"},
	"graphql":    {""},
	"jsdoc":      {"", "!", "!=", "\"", "\"\"", "#", "%", "&", "'", "''", "(", "()", ")", "+", ",", "-", "--", "-->", "->", ".", "0b", "0x", ":", "::", ";", ";;", "<", "<!--", "=", "==", "=>", ">", "?", "\\", "]", "^", "`", "``", "|", "~"},
	"json5":      {""},
	"mermaid":    {"", "\t\t", "\t\n", "\n\n", " ", " \n", "  ", "\"\"", "--", "-->", "=="},
	"nickel":     {""},
	"powershell": {"", "\""},
	"promql":     {""},
	"regex":      {""},
	"ron":        {""},
	"textproto":  {"", "1", "2", "3", "4", "5", "6", "7", "8", "9"},
	"vhdl":       {"`", "``"},
}

// recoverEOFPublishRouteDivergences lists inputs from the same sweep where
// the Go port's recovery selects the recover_eof route but the C oracle's
// own recovery does not always agree with a bare ERROR root there. Both
// grammars are excluded from CRecoverEOFBareRootReceipted:
//
//   - c_sharp (review round-2 finding B-A): C resyncs and reduces
//     compilation_unit before EOF on every ASCII identifier character, so
//     C's kind is compilation_unit, not ERROR. A 103-input census of every
//     input where the port reaches the bare shape found 0 where C agrees;
//     the exclusion is fully justified here.
//   - earthfile (review round-3 finding M1: the prior claim here was
//     wrong): mixed, not uniformly disagreeing. On some inputs (for
//     example "F0|") C's root is already a childless ERROR — bare publish
//     would be an exact match; on others (for example "a1") C's root is
//     ERROR with one invisible child, so bare publish would mismatch the
//     child count. A 30-input census found C agrees on 8 of 30 (27%).
//     Excluding earthfile is the conservative choice given that split.
//
// See TestParityRecoverEOFRouteDivergences.
var recoverEOFPublishRouteDivergences = map[string][]string{
	"c_sharp": {
		"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M", "N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z",
		"_",
		"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z",
		// Review round-3 finding N5: a wider independent census found 59
		// divergent c_sharp inputs against this list's 53; these six were
		// the ones it named as missing (multi-character identifiers and
		// multi-byte identifier text).
		"a1b2", "end", "the", "ée", "日", "日本",
	},
	// Review round-3 finding N5: "x1" and "a1b2" were named as missing from
	// a wider independent census (4 divergent earthfile inputs found,
	// against this list's 2).
	"earthfile": {"a1", "u8", "x1", "a1b2"},
}

func recoverEOFPublishSweepGoLanguage(name string) *gotreesitter.Language {
	switch name {
	case "corn":
		return grammars.CornLanguage()
	case "cpon":
		return grammars.CponLanguage()
	case "dhall":
		return grammars.DhallLanguage()
	case "dot":
		return grammars.DotLanguage()
	case "dtd":
		return grammars.DtdLanguage()
	case "earthfile":
		return grammars.EarthfileLanguage()
	case "ebnf":
		return grammars.EbnfLanguage()
	case "facility":
		return grammars.FacilityLanguage()
	case "fidl":
		return grammars.FidlLanguage()
	case "graphql":
		return grammars.GraphqlLanguage()
	case "jsdoc":
		return grammars.JsdocLanguage()
	case "json5":
		return grammars.Json5Language()
	case "mermaid":
		return grammars.MermaidLanguage()
	case "nickel":
		return grammars.NickelLanguage()
	case "powershell":
		return grammars.PowershellLanguage()
	case "promql":
		return grammars.PromqlLanguage()
	case "regex":
		return grammars.RegexLanguage()
	case "ron":
		return grammars.RonLanguage()
	case "textproto":
		return grammars.TextprotoLanguage()
	case "vhdl":
		return grammars.VhdlLanguage()
	case "c_sharp":
		return grammars.CSharpLanguage()
	default:
		return nil
	}
}

// recoverEOFShape is the full comparable shape of a parsed root: kind,
// child count, span, HasError, and s-expression. Round-1's sweep compared
// only kind and child count; round-2 finding "the sweep test compares less
// than its doc comment claims" widens the comparison to match the doc
// comment's "C agrees with the bare ERROR root" claim.
type recoverEOFShape struct {
	kind     string
	hasError bool
	children int
	start    uint32
	end      uint32
	sexpr    string
}

func cOracleShape(root *sitter.Node) recoverEOFShape {
	return recoverEOFShape{
		kind:     root.Kind(),
		hasError: root.HasError(),
		children: int(root.ChildCount()),
		start:    uint32(root.StartByte()),
		end:      uint32(root.EndByte()),
		sexpr:    root.ToSexp(),
	}
}

func goShape(root *gotreesitter.Node, lang *gotreesitter.Language) recoverEOFShape {
	return recoverEOFShape{
		kind:     root.Type(lang),
		hasError: root.HasError(),
		children: root.ChildCount(),
		start:    root.StartByte(),
		end:      root.EndByte(),
		sexpr:    root.SExpr(lang),
	}
}

// TestParityRecoverEOFPublishSweep is review finding B2/B-B's C-oracle
// receipt. For every (grammar, input) pair in recoverEOFPublishCandidates,
// it parses the input on the pinned tree-sitter C oracle and on the Go
// port, and asserts full shape agreement: C also produces a childless
// ERROR root exactly matching Go's kind, HasError, child count, span, and
// s-expression. Several grammars run fine in one container (a few seconds
// for the whole set); --memory 4g is the only constraint that matters.
func TestParityRecoverEOFPublishSweep(t *testing.T) {
	for grammar, inputs := range recoverEOFPublishCandidates {
		t.Run(grammar, func(t *testing.T) {
			cLanguage, err := COracleLanguage(grammar)
			if err != nil {
				t.Fatalf("COracleLanguage(%s): %v", grammar, err)
			}
			goLang := recoverEOFPublishSweepGoLanguage(grammar)
			if goLang == nil {
				t.Fatalf("no Go language mapped for %q", grammar)
			}

			for _, input := range inputs {
				t.Run(inputSubtestName(input), func(t *testing.T) {
					src := []byte(input)

					cParser := sitter.NewParser()
					defer cParser.Close()
					if err := cParser.SetLanguage(cLanguage); err != nil {
						t.Fatalf("SetLanguage: %v", err)
					}
					cTree := cParser.Parse(src, nil)
					defer cTree.Close()
					cShape := cOracleShape(cTree.RootNode())

					goParser := gotreesitter.NewParser(goLang)
					goTree, err := goParser.Parse(src)
					if err != nil {
						t.Fatalf("Go Parse: %v", err)
					}
					defer goTree.Release()
					goRootShape := goShape(goTree.RootNode(), goLang)

					if goRootShape.kind != "ERROR" || goRootShape.children != 0 {
						t.Fatalf("Go root = %+v, want the published childless ERROR root", goRootShape)
					}

					if cShape != goRootShape {
						t.Fatalf("C oracle disagrees for %s %q:\n  C:  %+v\n  Go: %+v", grammar, input, cShape, goRootShape)
					}
				})
			}
		})
	}
}

// TestParityRecoverEOFRouteDivergences is review round-2 finding B-A's
// regression guard, generalized to cover both excluded grammars
// (recoverEOFPublishRouteDivergences). For c_sharp and earthfile, the Go
// port's recovery selects recover_eof, but the C oracle's own recovery does
// not, so publishing bare would disagree with C. Because
// CRecoverEOFBareRootReceipted excludes both, Go must keep wrapping: this
// test asserts, for every listed input, that neither Go's actual shape nor
// C's shape is the bare childless ERROR root the receipted grammars share
// — confirming the exclusion is still necessary (C never agreed) and still
// sufficient (Go no longer publishes bare). It does not require Go's
// wrapped shape to equal C's wrapped shape: earthfile's ordinary wrapped
// root already disagreed with C before this PR, for reasons unrelated to
// recover_eof publishing. c_sharp's stronger claim — the wrapped shapes are
// identical — is checked separately in
// TestParityCSharpRecoverEOFWrappedRootMatchesC.
func TestParityRecoverEOFRouteDivergences(t *testing.T) {
	for grammar, inputs := range recoverEOFPublishRouteDivergences {
		t.Run(grammar, func(t *testing.T) {
			cLanguage, err := COracleLanguage(grammar)
			if err != nil {
				t.Fatalf("COracleLanguage(%s): %v", grammar, err)
			}
			goLang := recoverEOFPublishSweepGoLanguage(grammar)
			if goLang == nil {
				t.Fatalf("no Go language mapped for %q", grammar)
			}

			for _, input := range inputs {
				t.Run(inputSubtestName(input), func(t *testing.T) {
					src := []byte(input)

					cParser := sitter.NewParser()
					defer cParser.Close()
					if err := cParser.SetLanguage(cLanguage); err != nil {
						t.Fatalf("SetLanguage: %v", err)
					}
					cTree := cParser.Parse(src, nil)
					defer cTree.Close()
					cShape := cOracleShape(cTree.RootNode())

					// This "C agrees" invariant is strict only for c_sharp,
					// where a 103-input census (review round-2 finding B-A,
					// round-3 finding M1) found C bare on none of them.
					// earthfile is mixed — C is childless ERROR on 8 of 30
					// sampled inputs, including "F0|" — so the same check
					// would misreport a known, pre-existing split as a new
					// regression. Do not widen earthfile's input list in
					// recoverEOFPublishRouteDivergences with an input this
					// assertion has not been re-verified against.
					if grammar == "c_sharp" && cShape.kind == "ERROR" && cShape.children == 0 {
						t.Fatalf("%s %q: C now agrees with a bare ERROR root; %s may no longer need the exclusion", grammar, input, grammar)
					}

					goParser := gotreesitter.NewParser(goLang)
					goTree, err := goParser.Parse(src)
					if err != nil {
						t.Fatalf("Go Parse: %v", err)
					}
					defer goTree.Release()
					goRootShape := goShape(goTree.RootNode(), goLang)

					if goRootShape.kind == "ERROR" && goRootShape.children == 0 {
						t.Fatalf("%s %q published a bare ERROR root; the receipt exclusion regressed", grammar, input)
					}
					// This does not require cShape == goRootShape: some
					// excluded grammars (earthfile) already disagreed with C
					// on their ordinary wrapped shape before this PR, for
					// reasons unrelated to recover_eof publishing. What
					// matters here is only that neither side is the bare
					// childless ERROR shape the receipted grammars share —
					// c_sharp's stronger claim (Go's wrapped root exactly
					// matches C) is checked separately in
					// TestParityCSharpRecoverEOFWrappedRootMatchesC.
				})
			}
		})
	}
}

// TestParityCSharpRecoverEOFWrappedRootMatchesC is the fix plan's specific
// head-vs-wrapped assertion for c_sharp: on "A" and the empty string, the
// wrapped "(compilation_unit (ERROR))" shape c_sharp still produces (the
// receipt exclusion keeps recover_eof from publishing bare) is identical
// to the C oracle's own shape — kind, HasError, child count, span, and
// s-expression. Unlike earthfile, c_sharp's ordinary wrapped root already
// matched C on the guard base, so this exact-match claim is meaningful.
func TestParityCSharpRecoverEOFWrappedRootMatchesC(t *testing.T) {
	cLanguage, err := COracleLanguage("c_sharp")
	if err != nil {
		t.Fatalf("COracleLanguage(c_sharp): %v", err)
	}
	goLang := grammars.CSharpLanguage()

	for _, input := range []string{"A", ""} {
		t.Run(inputSubtestName(input), func(t *testing.T) {
			src := []byte(input)

			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatalf("SetLanguage: %v", err)
			}
			cTree := cParser.Parse(src, nil)
			defer cTree.Close()
			cShape := cOracleShape(cTree.RootNode())

			goParser := gotreesitter.NewParser(goLang)
			goTree, err := goParser.Parse(src)
			if err != nil {
				t.Fatalf("Go Parse: %v", err)
			}
			defer goTree.Release()
			goRootShape := goShape(goTree.RootNode(), goLang)

			if goRootShape.kind == "ERROR" && goRootShape.children == 0 {
				t.Fatalf("c_sharp %q published a bare ERROR root; the receipt exclusion regressed", input)
			}
			if cShape != goRootShape {
				t.Fatalf("c_sharp %q: C and Go disagree on the wrapped root:\n  C:  %+v\n  Go: %+v", input, cShape, goRootShape)
			}
		})
	}
}

// inputSubtestName maps a short malformed input to a stable, readable
// subtest name (t.Run sanitizes slashes and spaces, which several inputs
// here are made of).
func inputSubtestName(input string) string {
	names := map[string]string{
		"":     "empty",
		"{":    "brace_open",
		"}":    "brace_close",
		"(":    "paren_open",
		")":    "paren_close",
		"[":    "bracket_open",
		"]":    "bracket_close",
		"<":    "angle_open",
		">":    "angle_close",
		"\"":   "double_quote",
		"'":    "single_quote",
		"`":    "backtick",
		",":    "comma",
		";":    "semicolon",
		":":    "colon",
		".":    "dot",
		"0x":   "hex_prefix",
		"0b":   "bin_prefix",
		"@":    "at",
		"#":    "hash",
		"|":    "pipe",
		"!":    "bang",
		"!=":   "bang_eq",
		"=":    "eq",
		"==":   "eq_eq",
		"=>":   "fat_arrow",
		"->":   "arrow",
		"--":   "dash_dash",
		"-->":  "dash_dash_gt",
		"<!--": "html_comment_open",
		"/":    "slash",
		"//":   "slash_slash",
		"*":    "star",
		"/*":   "slash_star",
		"*/":   "star_slash",
		"?":    "question",
		"\\":   "backslash",
		"^":    "caret",
		"~":    "tilde",
		"$":    "dollar",
		"%":    "percent",
		"&":    "amp",
		"+":    "plus",
		"-":    "minus",
		"::":   "colon_colon",
		";;":   "semi_semi",
		"``":   "backtick_backtick",
		"''":   "quote_quote",
		"\"\"": "dquote_dquote",
		"()":   "paren_paren",
		"{}":   "brace_brace",
		" ":    "space",
		"  ":   "space_space",
		"\t\t": "tab_tab",
		"\n\n": "nl_nl",
		"\t\n": "tab_nl",
		" \n":  "space_nl",
		"_":    "underscore",
		"__":   "underscore_underscore",
	}
	if name, ok := names[input]; ok {
		return name
	}
	return input
}

// TestCRecoverEOFBareRootReceiptMatchesSweep asserts
// gotreesitter.CRecoverEOFBareRootReceiptedNames enumerates exactly the
// grammars this file's sweep receipts (recoverEOFPublishCandidates) plus
// doxygen (receipted separately; see CRecoverEOFBareRootReceipted's doc
// comment), and that every route-divergent exclusion
// (recoverEOFPublishRouteDivergences) and every other shipped grammar
// answers false. This is the fix plan's drift guard: the production table
// and the sweep's evidence can no longer diverge silently.
//
// Review round-3 finding N2: enumerating the production set via
// CRecoverEOFBareRootReceiptedNames (rather than probing only the sweep's
// own candidate names one at a time) also catches a receipted name the
// sweep never mentions at all — including one that names no real shipped
// grammar, which a probe-only check cannot see because it never visits any
// name outside grammars.AllLanguages().
func TestCRecoverEOFBareRootReceiptMatchesSweep(t *testing.T) {
	wantReceipted := map[string]bool{"doxygen": true}
	for grammar := range recoverEOFPublishCandidates {
		wantReceipted[grammar] = true
	}

	gotReceipted := map[string]bool{}
	for _, name := range gotreesitter.CRecoverEOFBareRootReceiptedNames() {
		gotReceipted[name] = true
	}

	for grammar := range wantReceipted {
		if !gotReceipted[grammar] {
			t.Errorf("CRecoverEOFBareRootReceipted(%q) = false, want true (in the sweep's receipted set)", grammar)
		}
	}
	for grammar := range gotReceipted {
		if !wantReceipted[grammar] {
			t.Errorf("CRecoverEOFBareRootReceipted(%q) = true, but %q is in neither the sweep's receipted set nor named as a route-divergence exclusion; add its evidence to one of them", grammar, grammar)
		}
	}

	for grammar := range recoverEOFPublishRouteDivergences {
		if gotReceipted[grammar] {
			t.Errorf("CRecoverEOFBareRootReceipted(%q) = true, want false (a known route-divergent exclusion)", grammar)
		}
	}

	catalog := map[string]bool{}
	for _, entry := range grammars.AllLanguages() {
		catalog[entry.Name] = true
	}
	for grammar := range gotReceipted {
		if !catalog[grammar] {
			t.Errorf("CRecoverEOFBareRootReceipted(%q) = true, but %q names no grammar in grammars.AllLanguages()", grammar, grammar)
		}
	}
}
