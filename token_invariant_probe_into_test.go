package gotreesitter

import (
	"github.com/odvcencio/gotreesitter/internal/incr"
	"testing"
)

func TestPrimitiveProbeIntoContract(t *testing.T) {
	for _, src := range [][]byte{[]byte("12x"), []byte("z"), []byte("1é"), {0xff}, nil} {
		for _, scans := range []uint32{0, 1, 8} {
			for _, bytes := range []uint32{0, 1, 128} {
				for _, limit := range []uint32{0, 1, 8} {
					lang := primitiveProofDigitLanguage()
					d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, src), lang, nil, nil, nil, nil, false)
					a, b := tokenInvariantPrimitiveBudget{bytes: bytes, scans: scans}, tokenInvariantPrimitiveBudget{bytes: bytes, scans: scans}
					want, ws, wa, wok := d.referencePrimitiveProbe(src, 0, Point{}, 0, &a, limit)
					got := Token{Symbol: 1}
					gs := incr.DFAProbeState{Position: 1}
					ga, gok := d.tokenInvariantProbeDFALimitedInto(src, 0, Point{}, 0, &b, limit, &got, &gs)
					if got != want || gs != ws || ga != wa || gok != wok || a != b {
						t.Fatalf("src=%x scans=%d bytes=%d limit=%d got=%+v/%+v/%t/%t/%+v want=%+v/%+v/%t/%t/%+v", src, scans, bytes, limit, got, gs, ga, gok, b, want, ws, wa, wok, a)
					}
					d.Close()
				}
			}
		}
	}
}

// Frozen baseline implementation is an independent reference for the output-parameter probe.
func (d *dfaTokenSource) referencePrimitiveProbe(source []byte, origin uint32, point Point, mode uint32, budget *tokenInvariantPrimitiveBudget, oldMaximum uint32) (Token, incr.DFAProbeState, bool, bool) {
	if budget.scans == 0 || budget.bytes == 0 {
		return Token{}, incr.DFAProbeState{}, false, false
	}
	limit, proofCut := tokenInvariantProbeLimit(source, origin, budget, oldMaximum)
	probe := *d.lexer
	probe.source = source[:int(limit)]
	probe.pos, probe.row, probe.col = int(origin), point.Row, point.Column
	probe.tokenInvariantReadSpanMax = nil
	probe.failTokenStartPos, probe.failTokenStartRow, probe.failTokenStartCol, probe.failTokenStartRangeIdx = 0, 0, 0, 0
	probe.failTokenEnd = includedLexerCursor{}
	var tok Token
	accepted := probe.scanInto(mode, int(origin), point.Row, point.Column, &tok)
	frontier := tokenInvariantExaminedEnd(probe.source, tok.lexerLookaheadEndByte)
	// This private token carries the proof bound, not the public C frontier.
	tok.lexerLookaheadEndByte = frontier
	if frontier < origin || (limit < uint64(len(source)) && uint64(frontier) >= limit && !proofCut) || !budget.charge(frontier-origin) {
		return Token{}, incr.DFAProbeState{}, false, false
	}
	return tok, incr.DFAProbeState{
		Position: probe.pos, Row: probe.row, Column: probe.col,
		FailurePosition: probe.failTokenStartPos, FailureRow: probe.failTokenStartRow, FailureColumn: probe.failTokenStartCol,
	}, accepted, true
}
