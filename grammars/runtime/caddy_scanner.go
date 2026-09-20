//go:build !grammar_subset || grammar_subset_caddy

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the caddy grammar. This is the external index
// (the position of the token in the grammar's `externals: [...]` list),
// which is exactly what tree-sitter's `valid_symbols` array and C's
// `result_symbol` enum are indexed by. The external index is stable across a
// blob regen as long as the externals list itself does not reorder;
// concrete numeric gotreesitter.Symbol IDs are NOT stable (they shift
// whenever the grammar's total symbol count changes), so this scanner never
// hardcodes them -- see the symbols field on CaddyExternalScanner below.
const (
	caddyTokNewline = 0
	caddyTokIndent  = 1
	caddyTokDedent  = 2
	caddyTokenCount = 3
)

// caddyDefaultSymTable records the concrete gotreesitter.Symbol IDs the
// currently shipped caddy.bin assigns to each external, in caddyTok* order.
// It exists only as a pre-bind fallback (and as an independent value to
// compare a real bind against in tests); ExternalScannerForLanguage below
// overwrites it with the values read from the actual loaded Language at bind
// time, which is what the scanner must do to survive a future blob regen
// that renumbers absolute symbol IDs without touching the externals list
// order.
var caddyDefaultSymTable = [caddyTokenCount]gotreesitter.Symbol{
	37, // _newline
	38, // _indent
	39, // _dedent
}

// caddyExternalScannerSpec records the source contract for this hand-written
// port, so updater tooling can tell a grammar-only upstream change apart
// from one that also touches the external scanner or its token list. Its
// Externals list is also the binding source for ExternalScannerForLanguage:
// index i here is scanner token index i (caddyTok* order).
var caddyExternalScannerSpec = ExternalScannerSpec{
	Language:       "caddy",
	UpstreamRepo:   "https://github.com/opa-oz/tree-sitter-caddy",
	UpstreamCommit: "2b0dd9066900568a3d6b33dc51d2e271cb48bd92",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "16391c3eb44eb5d72a1b5f9833098b5d5e954a50047cf06e6eac9d4295dcbb68"},
		{Path: "src/scanner.c", SHA256: "cb8a1cb4d712f7afee596cebd92bf1616827c7cd86b3088a4aeeade20f5a59d3"},
	},
	Externals: []string{
		"_newline",
		"_indent",
		"_dedent",
	},
}

func init() {
	RegisterExternalScannerSpec(caddyExternalScannerSpec)
}

// caddyMaxIndentDepth caps the indent stack the same way upstream commit
// f784fd1 (tree-sitter-caddy) does with its MAX_INDENT_DEPTH macro. The cap
// stops a crafted input with excessive indentation from growing the stack
// without bound.
const caddyMaxIndentDepth = 1024

type caddyScannerState struct {
	indents []uint16
}

// CaddyExternalScanner implements gotreesitter.ExternalScanner for
// tree-sitter-caddy. Handles _newline, _indent, and _dedent tokens for
// indentation tracking.
//
// symbols holds the concrete gotreesitter.Symbol each external index maps to
// in the Language this instance was bound to (see ExternalScannerForLanguage).
// The scanner never hardcodes an absolute Symbol value: a blob regen can
// renumber the grammar's absolute symbol IDs without touching the externals
// list order, and a scanner that still called SetResultSymbol with a stale
// hardcoded ID would silently emit the wrong (but still structurally valid)
// node type instead of failing loudly.
type CaddyExternalScanner struct {
	symbols         [caddyTokenCount]gotreesitter.Symbol
	externalToToken []int
}

// ExternalScannerForLanguage binds this scanner's token indices to lang's
// concrete external symbol IDs, positionally, via caddyExternalScannerSpec.
func (CaddyExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := CaddyExternalScanner{symbols: caddyDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, caddyExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (CaddyExternalScanner) Create() any {
	return &caddyScannerState{indents: []uint16{0}}
}

func (CaddyExternalScanner) Destroy(payload any) {}

func (CaddyExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*caddyScannerState)
	size := 0
	for i := 1; i < len(s.indents) && size < len(buf); i++ {
		buf[size] = byte(s.indents[i])
		size++
	}
	return size
}

func (CaddyExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*caddyScannerState)
	s.indents = s.indents[:0]
	s.indents = append(s.indents, 0)
	// tree-sitter-caddy commit f784fd1 caps the number of indents restored
	// from a serialized buffer at caddyMaxIndentDepth-1, mirroring the same
	// cap Scan enforces below.
	maxPush := len(buf)
	if maxPush > caddyMaxIndentDepth-1 {
		maxPush = caddyMaxIndentDepth - 1
	}
	for _, b := range buf[:maxPush] {
		s.indents = append(s.indents, uint16(b))
	}
}

func (s CaddyExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	state := payload.(*caddyScannerState)
	if len(s.externalToToken) > 0 {
		var semanticValid [caddyTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < caddyTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}

	if lexer.Lookahead() == '\n' {
		if caddyValid(validSymbols, caddyTokNewline) {
			lexer.Advance(true)
			lexer.MarkEnd()
			lexer.SetResultSymbol(s.symbols[caddyTokNewline])
			return true
		}
		return false
	}

	if lexer.Lookahead() != 0 && lexer.Column() == 0 {
		var indentLen uint16

		lexer.MarkEnd()

		for {
			ch := lexer.Lookahead()
			if ch == ' ' {
				indentLen++
				lexer.Advance(true)
			} else if ch == '\t' {
				indentLen += 8
				lexer.Advance(true)
			} else {
				break
			}
		}

		top := state.indents[len(state.indents)-1]
		if indentLen > top && caddyValid(validSymbols, caddyTokIndent) {
			// tree-sitter-caddy commit f784fd1: only grow the indent stack
			// while it stays below caddyMaxIndentDepth, guarding against
			// unbounded memory growth from pathological indentation.
			if len(state.indents) < caddyMaxIndentDepth {
				state.indents = append(state.indents, indentLen)
				lexer.MarkEnd()
				lexer.SetResultSymbol(s.symbols[caddyTokIndent])
				return true
			}
		}
		if indentLen < top && caddyValid(validSymbols, caddyTokDedent) {
			state.indents = state.indents[:len(state.indents)-1]
			lexer.MarkEnd()
			lexer.SetResultSymbol(s.symbols[caddyTokDedent])
			return true
		}
	}

	return false
}

func caddyValid(validSymbols []bool, idx int) bool {
	return idx >= 0 && idx < len(validSymbols) && validSymbols[idx]
}
