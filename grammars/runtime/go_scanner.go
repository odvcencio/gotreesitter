//go:build !grammar_subset || grammar_subset_go

package grammarruntime

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the go grammar (order matches grammargen's
// GoGrammar SetExternals call — a single external token). This is the
// external index (the position of the token in the grammar's externals
// list), which is exactly what tree-sitter's `valid_symbols` array and C's
// result_symbol enum are indexed by. The external index is stable across a
// blob regen as long as grammargen's SetExternals call order does not
// change; concrete numeric gotreesitter.Symbol IDs are NOT stable (they
// shift whenever the grammar's total symbol count changes), so this scanner
// never hardcodes them -- see goDefaultSymTable below.
const (
	goTokAutoSemicolon = 0
	goTokenCount       = 1
)

// goDefaultSymTable records the concrete gotreesitter.Symbol ID the
// currently shipped go.bin assigns to the sole external, in goTok* order. It
// exists only as a pre-bind fallback (and as an independent value to compare
// a real bind against in tests); ExternalScannerForLanguage below overwrites
// it with the value read from the actual loaded Language at bind time, which
// is what the scanner must do to survive a future blob regen that renumbers
// absolute symbol IDs without touching the externals list order. Fixed today
// by grammargen.GoGrammar's `g.SetExternals(Sym("_automatic_semicolon"))`
// plus everything defined earlier in the grammar; regenerate go.bin via
// `go run ./cmd/grammargen emit go -bin grammars/grammar_blobs/go.bin` and
// this table rebinds itself automatically at load time
// (grammargen/go_external_symbol_test.go additionally pins the expected
// value against grammargen's own generated Language, independent of this
// runtime scanner, so a mismatch there fails loudly too).
var goDefaultSymTable = [goTokenCount]gotreesitter.Symbol{
	94, // _automatic_semicolon
}

// goExternalScannerSpec records the source contract for this scanner. Unlike
// every other hand-written port in this package, _automatic_semicolon is NOT
// ported from an upstream C scanner.c: upstream tree-sitter-go has no
// scanner.c and no externals at all (see the GoExternalScanner doc comment
// below for why gotreesitter's own grammargen backend invents this external
// to route around a shared-DFA tie-break bug). SourceFiles is therefore
// empty -- there is no upstream scanner source to hash for drift detection --
// and Externals lists the rule name grammargen itself assigns via
// `Sym("_automatic_semicolon")` in grammargen/go_grammar.go, not a name read
// from upstream's src/grammar.json.
var goExternalScannerSpec = ExternalScannerSpec{
	Language:       "go",
	UpstreamRepo:   "https://github.com/tree-sitter/tree-sitter-go",
	UpstreamCommit: "2346a3ab1bb3857b48b29d779a1ef9799a248cd7",
	Externals: []string{
		"_automatic_semicolon",
	},
}

func init() {
	RegisterExternalScannerSpec(goExternalScannerSpec)
}

// GoExternalScanner resolves the Go grammar's `terminator` rule — automatic
// semicolon insertion (ASI) — via an external scanner instead of a plain DFA
// alternation.
//
// Background: upstream tree-sitter-go has no scanner.c for this at all. Its
// grammar.js models ASI as `terminator = choice(/\n/, ';', '\0')`, and
// upstream's own generated parser gives every LR state its own lexer
// function, so the real newline pattern and the zero-width EOF sentinel
// never compete within a shared table. gotreesitter's grammargen backend
// instead compiles one shared, merged DFA across all states; at every
// terminator position the zero-width '\0' EOF sentinel and the one-byte
// `\n` pattern both accept, and the runtime's shared tie-break
// (parser_dfa_token_source.go: preferZeroWidthStartAcceptForState) prefers
// the zero-width accept unconditionally. That silently drops the trailing
// newline byte from the enclosing statement/declaration span everywhere
// except genuine end-of-file, which is the root cause of the Go C-parity
// gate failure this scanner fixes.
//
// Routing `terminator`'s newline/EOF alternatives through a dedicated
// external scanner sidesteps the shared-DFA tie-break entirely: Scan is
// only ever invoked in parser states where a terminator is structurally
// legal (every occurrence in grammargen/go_grammar.go directly follows a
// completed statement, declaration, or spec, so there is no need to track
// "was the previous token an identifier/literal/keyword" — the grammar
// itself already gates that), and it makes an unambiguous decision from the
// raw byte stream with no shared-state tie-break involved. This mirrors how
// JavaScriptExternalScanner (grammars/javascript_scanner.go) resolves
// `_automatic_semicolon` for the JS/TS grammars in this package.
//
// symbols holds the concrete gotreesitter.Symbol the sole external maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type GoExternalScanner struct {
	symbols         [goTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds the scanner's token slot to the loaded
// Language's ExternalSymbols positionally. A hardcoded absolute
// gotreesitter.Symbol constant here would emit the wrong token whenever a
// grammar bump renumbers go's external symbol.
func (GoExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := GoExternalScanner{symbols: goDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, goExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (s GoExternalScanner) symbolTable() *[goTokenCount]gotreesitter.Symbol {
	if s.symbols == ([goTokenCount]gotreesitter.Symbol{}) {
		return &goDefaultSymTable
	}
	return &s.symbols
}

func (GoExternalScanner) Create() any                           { return nil }
func (GoExternalScanner) Destroy(payload any)                   {}
func (GoExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (GoExternalScanner) Deserialize(payload any, buf []byte)   {}

// SupportsIncrementalReuse: the scanner carries no serialized state (Create
// returns nil), so incremental subtree reuse is always safe.
func (GoExternalScanner) SupportsIncrementalReuse() bool { return true }

// ExternalScannerIsStateless discharges the campaign O(edit) W4 scanner-
// quiescence proof for Go (spec.campaign.oedit). It reports that the scanner
// holds no cross-token state, so its state at any boundary equals the state a
// fresh parse holds there and every reuse boundary is quiescent. The five
// proof obligations, each verified against this file, live in the package doc
// of external_scanner_quiescence.go:
//
//  1. No persisted state: Create returns nil, Serialize returns 0, Deserialize
//     is a no-op (above).
//  2. Scan is a pure function of the local lookahead and the valid-symbol set
//     (Scan below reads only lexer.Lookahead() and validSymbols).
//  3. Terminator legality is grammar-gated, not history-gated (the scanner
//     runs only where the grammar makes _automatic_semicolon valid).
//  4. Raw strings never induce scanner state (the DFA lexes raw_string_literal
//     as one token; the terminator symbol is never valid inside it).
//  5. Comments before a declaration never induce scanner state (Scan declines,
//     the DFA matches the comment as an extra, and extras keep the LR state).
//
// The classifier reads this marker as a proof, so an incorrect true is silent
// incremental corruption. Keep the marker in lockstep with the scanner.
func (GoExternalScanner) ExternalScannerIsStateless() bool { return true }

// PreservesStateOnScanFailure: Scan never mutates any persisted payload
// before returning false (there is no payload), so the token source can
// skip the snapshot/restore it would otherwise do around a failed scan.
func (GoExternalScanner) PreservesStateOnScanFailure() bool { return true }

func (s GoExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	if len(s.externalToToken) > 0 {
		var semanticValid [goTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < goTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	if !goValidSym(validSymbols, goTokAutoSemicolon) {
		return false
	}
	syms := s.symbolTable()

	// Skip horizontal whitespace only ('\n' itself decides the match).
	// Comments are intentionally left alone here: declining below (without
	// having called SetResultSymbol) causes the token source to discard
	// this entire scan attempt and reset the lexer to the byte position it
	// started at, so the normal DFA path matches the comment as its own
	// `comment` extra, then the parser retries this scanner once past it
	// (the LR parser state — and thus which external tokens are valid — is
	// unchanged by extras).
	for {
		switch lexer.Lookahead() {
		case ' ', '\t', '\r':
			lexer.Advance(true)
			continue
		}
		break
	}

	switch lexer.Lookahead() {
	case '\n':
		// Consume the newline itself as the terminator token's span, matching
		// upstream's `/\n/` pattern alternative byte-for-byte.
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.SetResultSymbol(syms[goTokAutoSemicolon])
		return true
	case 0:
		// True end-of-file: zero-width match, matching upstream's `'\0'`
		// sentinel alternative.
		lexer.MarkEnd()
		lexer.SetResultSymbol(syms[goTokAutoSemicolon])
		return true
	default:
		// Anything else (an explicit ';', the start of a comment, or a
		// genuine syntax error) is not this scanner's concern: decline and
		// let the DFA's `Str(";")` alternative or `comment` extra handle it.
		return false
	}
}

func goValidSym(vs []bool, i int) bool { return i < len(vs) && vs[i] }
