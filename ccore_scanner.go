package gotreesitter

import (
	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
)

// External scanner hook for internal/ccore: the language's ExternalScanner
// over an ExternalLexer, one call per scan. The lex automata are not here:
// internal/ccore walks them itself (lexdfa.go) over the lex tables that
// ccore_bind.go converts. Nothing in this file parses; ccore_bind.go hands the
// hook to internal/ccore/bind, which the equality tests in cgo_harness use.

// ccoreExtScanner serves ccore.ExtScanner (through bind.FuncScanner, so that no
// exported method lands on a root type and in the API snapshots) over a language's
// ExternalScanner. The payload is created on the first Scan and destroyed by
// Close.
type ccoreExtScanner struct {
	sc      ExternalScanner
	payload any
	live    bool
	el      ExternalLexer
}

// scan runs C's deserialize, scan and serialize in that order
// (ts_parser__external_scanner_deserialize, _scan, _serialize).
func (a *ccoreExtScanner) scan(src []byte, pos int, row, col uint32, prev []byte, valid []bool, buf []byte, out *ccore.ExtScan) bool {
	if !a.live { // ts_parser__external_scanner_create
		a.payload, a.live = a.sc.Create(), true
	}
	el := &a.el
	el.reset(src, pos, row, col) // ts_lexer_start
	a.sc.Deserialize(a.payload, prev)
	found := a.sc.Scan(a.payload, el, valid)
	out.DidGetColumn = el.didGetColumn
	out.LookaheadEnd = el.lookaheadEndByteAtCursor() // ts_lexer_finish
	if !found {
		return false
	}
	tok, ok := el.token()
	if !ok {
		return false
	}
	out.StateLen = a.sc.Serialize(a.payload, buf)
	out.Token = ccore.Token{
		Symbol:       ccore.Symbol(tok.Symbol),
		StartByte:    tok.StartByte,
		EndByte:      tok.EndByte,
		StartRow:     tok.StartPoint.Row,
		StartCol:     tok.StartPoint.Column,
		EndRow:       tok.EndPoint.Row,
		EndCol:       tok.EndPoint.Column,
		LookaheadEnd: tok.lexerLookaheadEndByte,
	}
	return true
}

// close destroys the scanner payload if one exists (ts_parser__external_scanner_destroy).
func (a *ccoreExtScanner) close() {
	if a.live {
		a.sc.Destroy(a.payload)
		a.payload, a.live = nil, false
	}
}

// hook returns the ccore.ExtScanner for this scanner.
func (a *ccoreExtScanner) hook() ccore.ExtScanner {
	return &bind.FuncScanner{ScanFn: a.scan, CloseFn: a.close}
}
