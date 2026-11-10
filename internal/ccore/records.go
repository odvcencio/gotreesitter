package ccore

// Record equality with C, Go side: a pre-order walk over every subtree of the
// finished tree (hidden nodes included) that writes one canonical line per
// subtree and hashes the lines with FNV-1a 64. The C side is
// cgo_harness/internal/coracle/records_c.go (C/subtree.h accessors). The line
// format must stay identical on both sides.
//
// Field C keeps that this core does not keep, so it is left out of the line on
// both sides: lookahead_char of error leaves (rec has no such field).

import (
	"bufio"
	"fmt"
	"io"
)

const (
	fnvOffset = 14695981039346656037
	fnvPrime  = 1099511628211
)

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// recordLine appends the canonical line of subtree h at depth d.
func (p *Parser) recordLine(buf []byte, h Handle, d uint32) []byte {
	pad, size := p.padding(h), p.size(h)
	n := p.childCount(h)
	ext := h&1 == 0 && p.rec(h).flags&heapHasExternalTokens != 0
	extchg := p.hasExternalStateChange(h)
	buf = fmt.Appendf(buf, "d=%d inl=%d sym=%d vis=%d named=%d extra=%d chg=%d miss=%d kw=%d fl=%d fr=%d ext=%d extchg=%d col=%d st=%d pad=%d,%d,%d size=%d,%d,%d la=%d cost=%d n=%d",
		d, b2i(h&1 != 0), p.symbol(h), b2i(p.visible(h)), b2i(p.named(h)), b2i(p.extra(h)),
		b2i(p.hasFlag(h, inlHasChanges, heapHasChanges)), b2i(p.missing(h)), b2i(p.isKeyword(h)),
		b2i(p.fragileLeft(h)), b2i(p.fragileRight(h)), b2i(ext), b2i(extchg),
		b2i(p.hasFlag(h, inlDependsOnColumn, heapDependsOnColumn)), p.parseState(h),
		pad.bytes, pad.row, pad.col, size.bytes, size.row, size.col,
		p.lookaheadBytes(h), p.errorCost(h), n)
	if n > 0 {
		r := p.rec(h)
		buf = fmt.Appendf(buf, " vc=%d nc=%d vd=%d dp=%d rd=%d prod=%d fls=%d flst=%d",
			r.visibleChildCount, r.namedChildCount, p.visibleDescendants(h), p.dynPrec(h),
			p.repeatDepth(h), r.productionID, r.firstLeafSymbol, r.firstLeafState)
	}
	return append(buf, '\n')
}

// Records walks from the finished root. When w is not nil it receives the lines.
func (p *Parser) Records(w io.Writer) (hash uint64, lines uint64) {
	hash = fnvOffset
	if p.finished == 0 {
		return
	}
	var bw *bufio.Writer
	if w != nil {
		bw = bufio.NewWriter(w)
		defer bw.Flush()
	}
	type item struct {
		h Handle
		d uint32
	}
	stack := []item{{p.finished, 0}}
	var buf []byte
	for len(stack) > 0 {
		it := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		buf = p.recordLine(buf[:0], it.h, it.d)
		for _, b := range buf {
			hash = (hash ^ uint64(b)) * fnvPrime
		}
		lines++
		if bw != nil {
			bw.Write(buf)
		}
		if n := p.childCount(it.h); n > 0 {
			kids := p.children(it.h)
			for i := int(n) - 1; i >= 0; i-- {
				stack = append(stack, item{kids[i], it.d + 1})
			}
		}
	}
	return
}
