package ccore

// The visible walk as C/node.c does it (used by adapters and tests) and the
// visible-tree digest.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strings"
)

// ---------------------------------------------------------------- output

// digestWriter writes the visible-tree digest: one record per visible node (kind, byte and point range, flags, child count).
type digestWriter struct {
	buf []byte
}

func (w *digestWriter) str(s string) {
	w.buf = binary.LittleEndian.AppendUint32(w.buf, uint32(len(s)))
	w.buf = append(w.buf, s...)
}

func (w *digestWriter) u32(v uint32) { w.buf = binary.LittleEndian.AppendUint32(w.buf, v) }

// DigestRecord writes one node the same way on both sides. norm removes
// backslashes from the name, and reads the name \0 (a table that spells the NUL
// token with a backslash) as the empty name that C's cgo string conversion
// gives for it.
func DigestRecord(buf []byte, kind string, startByte, endByte, sr, sc, er, ec uint32, named, extra, missing, changes bool, children uint32, norm bool) []byte {
	w := digestWriter{buf: buf}
	if norm {
		if kind == "\\0" {
			kind = ""
		}
		kind = strings.ReplaceAll(kind, "\\", "")
	}
	w.str(kind)
	w.u32(startByte)
	w.u32(endByte)
	w.u32(sr)
	w.u32(sc)
	w.u32(er)
	w.u32(ec)
	var f byte
	if named {
		f |= 1
	}
	if extra {
		f |= 2
	}
	if missing {
		f |= 4
	}
	if changes {
		f |= 8
	}
	w.buf = append(w.buf, f)
	w.u32(children)
	return w.buf
}

// visit emits a node and then its visible children in C's order
// (C/node.c ts_node_child_iterator_next and ts_node__child: hidden nodes
// are flattened, aliases from the parent's production apply to
// non-extra children).
func (p *Parser) visit(h Handle, start Length, alias Symbol, emit func(h Handle, start Length, alias Symbol)) {
	emit(h, start, alias)
	p.visitChildren(h, start, emit)
}

func (p *Parser) visitChildren(h Handle, start Length, emit func(h Handle, start Length, alias Symbol)) {
	if p.childCount(h) == 0 {
		return
	}
	r := *p.rec(h)
	seq := p.aliasSeq(r.productionID)
	structural := 0
	pos := start
	for i, child := range p.kidSpan(r.childrenStart, r.childCount) {
		var a Symbol
		if !p.extra(child) {
			if structural < len(seq) {
				a = seq[structural]
			}
			structural++
		}
		if i > 0 {
			pos = lenAdd(pos, p.padding(child))
		}
		if p.visible(child) || a != 0 {
			p.visit(child, pos, a, emit)
		} else if p.childCount(child) > 0 {
			p.visitChildren(child, pos, emit)
		}
		pos = lenAdd(pos, p.size(child))
	}
}

func (p *Parser) digest() string { return p.digestOf(p.finished, false) }

// digestOf digests any root in the arena; changes adds the has_changes marks.
func (p *Parser) digestOf(root Handle, changes bool) string {
	if root == 0 {
		return ""
	}
	h := sha256.New()
	var buf []byte
	p.visit(root, p.padding(root), 0, func(n Handle, start Length, alias Symbol) {
		sym, named := p.symbol(n), p.named(n)
		if alias != 0 {
			sym = alias
			_, named = p.meta(alias)
		}
		end := lenAdd(start, p.size(n))
		var children uint32
		if p.childCount(n) > 0 {
			children = p.rec(n).visibleChildCount
		}
		buf = DigestRecord(buf[:0], p.symName(sym), start.bytes, end.bytes, start.row, start.col, end.row, end.col, named, p.extra(n), p.missing(n), changes && p.hasChanges(n), children, p.NormNames)
		h.Write(buf)
	})
	if p.errorCost(root) > 0 { // root error flag (ts_node_has_error)
		h.Write([]byte{1})
	} else {
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (p *Parser) walk() int {
	count := 0
	if p.finished == 0 {
		return 0
	}
	p.visit(p.finished, p.padding(p.finished), 0, func(n Handle, start Length, alias Symbol) { count++ })
	return count
}
