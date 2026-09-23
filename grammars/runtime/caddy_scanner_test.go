//go:build (!grammar_subset || grammar_subset_caddy) && !gotreesitter_no_copyleft

package grammarruntime

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// TestCaddyDeserializeCapsIndentDepth pins tree-sitter-caddy commit f784fd1
// (MAX_INDENT_DEPTH): a serialized checkpoint that claims more indent levels
// than caddyMaxIndentDepth must restore at most caddyMaxIndentDepth-1
// entries, matching tree_sitter_caddy_external_scanner_deserialize's capped
// loop. Before the port this loop pushed every byte of buf unconditionally,
// so a crafted incremental-edit checkpoint could grow the in-memory stack
// without bound.
func TestCaddyDeserializeCapsIndentDepth(t *testing.T) {
	scanner := CaddyExternalScanner{}
	state := &caddyScannerState{}

	buf := make([]byte, caddyMaxIndentDepth*2)
	for i := range buf {
		buf[i] = byte(i%250 + 1)
	}

	scanner.Deserialize(state, buf)

	if got, want := len(state.indents), caddyMaxIndentDepth; got != want {
		t.Fatalf("len(state.indents) = %d, want %d (base level 0 plus %d restored levels)", got, want, caddyMaxIndentDepth-1)
	}
	if state.indents[0] != 0 {
		t.Fatalf("state.indents[0] = %d, want 0 (base level)", state.indents[0])
	}

	// A buffer within the cap restores every byte, unaffected by the guard.
	small := &caddyScannerState{}
	scanner.Deserialize(small, []byte{1, 2, 3})
	if got, want := len(small.indents), 4; got != want {
		t.Fatalf("len(small.indents) = %d, want %d", got, want)
	}
}

// caddyIndentProbeLanguage is a minimal hand-built grammar used only to
// drive CaddyExternalScanner.Scan through the real gotreesitter.Parser and
// ExternalLexer machinery. tree-sitter-caddy's shipped grammar.json declares
// _newline/_indent/_dedent as externals but no production ever references
// them (verified against src/grammar.json and src/parser.c at both
// 9b3fde99d3d7 and 2b0dd9066900), so valid_symbols[INDENT] and
// valid_symbols[DEDENT] are always false for the real caddy.bin: the
// MAX_INDENT_DEPTH cap this file ports is unreachable through the shipped
// grammar for any input. This probe grammar accepts an arbitrary run of
// newline/indent/dedent tokens (list -> token | list token) purely so the
// port's cap logic can be exercised end to end with a real ExternalLexer.
func caddyIndentProbeLanguage() *gotreesitter.Language {
	const (
		symNewline gotreesitter.Symbol = 1
		symIndent  gotreesitter.Symbol = 2
		symDedent  gotreesitter.Symbol = 3
		symList    gotreesitter.Symbol = 4
	)
	return &gotreesitter.Language{
		Name:               "caddy_indent_probe",
		SymbolCount:        5,
		TokenCount:         4,
		ExternalTokenCount: 3,
		StateCount:         4,
		ProductionIDCount:  2,
		InitialState:       0,

		SymbolNames: []string{"EOF", "_newline", "_indent", "_dedent", "list"},
		SymbolMetadata: []gotreesitter.SymbolMetadata{
			{Name: "EOF", Visible: false, Named: false},
			{Name: "_newline", Visible: true, Named: false},
			{Name: "_indent", Visible: true, Named: false},
			{Name: "_dedent", Visible: true, Named: false},
			{Name: "list", Visible: true, Named: true},
		},
		FieldNames: []string{""},

		ExternalSymbols: []gotreesitter.Symbol{symNewline, symIndent, symDedent},
		ExternalLexStates: [][]bool{
			nil,
			{true, true, true},
		},

		ParseActions: []gotreesitter.ParseActionEntry{
			{Actions: nil}, // 0: error
			{Actions: []gotreesitter.ParseAction{{Type: gotreesitter.ParseActionShift, State: 1}}},                                         // 1: shift token -> S1
			{Actions: []gotreesitter.ParseAction{{Type: gotreesitter.ParseActionReduce, Symbol: symList, ChildCount: 1, ProductionID: 0}}}, // 2: reduce list -> token
			{Actions: []gotreesitter.ParseAction{{Type: gotreesitter.ParseActionShift, State: 2}}},                                         // 3: goto list -> S2
			{Actions: []gotreesitter.ParseAction{{Type: gotreesitter.ParseActionShift, State: 3}}},                                         // 4: shift token -> S3
			{Actions: []gotreesitter.ParseAction{{Type: gotreesitter.ParseActionAccept}}},                                                  // 5: accept
			{Actions: []gotreesitter.ParseAction{{Type: gotreesitter.ParseActionReduce, Symbol: symList, ChildCount: 2, ProductionID: 1}}}, // 6: reduce list -> list token
		},

		// Columns: EOF, _newline, _indent, _dedent, list.
		ParseTable: [][]uint16{
			{0, 1, 1, 1, 3}, // S0: shift token->S1; goto list->S2
			{2, 2, 2, 2, 0}, // S1: reduce list->token on any lookahead
			{5, 4, 4, 4, 0}, // S2: accept on EOF; shift token->S3
			{6, 6, 6, 6, 0}, // S3: reduce list->list token on any lookahead
		},

		LexModes: []gotreesitter.LexMode{
			{LexState: 0, ExternalLexState: 1},
			{LexState: 0, ExternalLexState: 1},
			{LexState: 0, ExternalLexState: 1},
			{LexState: 0, ExternalLexState: 1},
		},
		LexStates: []gotreesitter.LexState{
			{AcceptToken: 0, Default: -1, EOF: -1},
		},
	}
}

// TestCaddyScanCapsIndentDepthUnderProbeGrammar drives CaddyExternalScanner
// through caddyIndentProbeLanguage with far more than caddyMaxIndentDepth
// strictly-increasing indentation lines. It pins tree-sitter-caddy commit
// f784fd1's scan-time guard: state.indents must never grow past
// caddyMaxIndentDepth, matching upstream's
// `if (scanner->indents.len < MAX_INDENT_DEPTH)` check around VEC_PUSH.
// Before the port, this loop pushed unconditionally, growing the indent
// stack without bound on pathological input.
//
// The peak length is tracked from every Scan call (peakIndentsLen), not read
// back from the payload after Parse returns: a full parse legitimately
// starts a second attempt from a fresh Create() (observed with this probe
// grammar too), and that second attempt's own Serialize/Deserialize
// round-trip through the standard TREE_SITTER_SERIALIZATION_BUFFER_SIZE-like
// checkpoint buffer would otherwise mask an uncapped push loop by
// truncating the restored stack anyway. Reading the peak observed by any
// live payload during the whole parse avoids that confound.
//
// See caddyIndentProbeLanguage's doc comment for why this cannot be driven
// through the real, shipped caddy grammar: _indent/_dedent are declared
// externals with no referencing production, so valid_symbols[INDENT] is
// always false there.
func TestCaddyScanCapsIndentDepthUnderProbeGrammar(t *testing.T) {
	lang := caddyIndentProbeLanguage()

	bound := CaddyExternalScanner{}.ExternalScannerForLanguage(lang)
	var peakIndentsLen int
	lang.ExternalScanner = peakTrackingCaddyScanner{inner: bound, peak: &peakIndentsLen}

	var src strings.Builder
	const lines = caddyMaxIndentDepth + 200
	for i := 1; i <= lines; i++ {
		src.WriteString(strings.Repeat(" ", i))
		src.WriteString("\n")
	}

	tree, err := gotreesitter.NewParser(lang).Parse([]byte(src.String()))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if tree == nil {
		t.Fatal("parse returned nil tree")
	}
	defer tree.Release()
	if tree.RootNode().HasError() {
		t.Fatal("probe grammar parse unexpectedly produced an error tree")
	}

	// The probe fed `lines` (caddyMaxIndentDepth+200) strictly-increasing
	// indentation levels, so a scanner that reaches the cap at all must have
	// pushed to exactly caddyMaxIndentDepth (base level 0 plus
	// caddyMaxIndentDepth-1 real levels) before further INDENT tokens stop
	// growing the stack.
	if got, want := peakIndentsLen, caddyMaxIndentDepth; got != want {
		t.Fatalf("peak len(state.indents) = %d, want exactly %d (MAX_INDENT_DEPTH cap reached, not exceeded)", got, want)
	}
}

// peakTrackingCaddyScanner wraps a bound CaddyExternalScanner and records the
// largest len(state.indents) observed across every live payload during a
// parse, so the test above can assert the cap holds even across an internal
// restart-from-scratch attempt.
type peakTrackingCaddyScanner struct {
	inner gotreesitter.ExternalScanner
	peak  *int
}

func (c peakTrackingCaddyScanner) Create() any         { return c.inner.Create() }
func (c peakTrackingCaddyScanner) Destroy(payload any) { c.inner.Destroy(payload) }
func (c peakTrackingCaddyScanner) Serialize(payload any, buf []byte) int {
	return c.inner.Serialize(payload, buf)
}
func (c peakTrackingCaddyScanner) Deserialize(payload any, buf []byte) {
	c.inner.Deserialize(payload, buf)
}
func (c peakTrackingCaddyScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	ok := c.inner.Scan(payload, lexer, validSymbols)
	if s, isState := payload.(*caddyScannerState); isState && len(s.indents) > *c.peak {
		*c.peak = len(s.indents)
	}
	return ok
}
