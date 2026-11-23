package ccore

import (
	"bytes"
	"slices"
)

// shortcutMemo owns the root before the first edit and a snapshot of its text.
// The cell transfers its root ownership to base when it publishes that edit.
type shortcutMemo struct {
	base  Handle
	src   []byte
	edit  editRange
	edits int
}

// SetShortcut enables token-invariant reparses for trees this parser builds.
func (p *Parser) SetShortcut(on bool) { p.shortcutOn = on }

// ShortcutTaken reports whether the last parse used the shortcut.
func (p *Parser) ShortcutTaken() bool { return p.shortcutTaken }

// ShortcutWhy reports why the last shortcut attempt declined.
func (p *Parser) ShortcutWhy() string { return p.shortcutWhy }

var shortcutReasons = [...]string{"taken", "no-memo", "not-same-length", "edit-mismatch", "newline", "root-extent-or-error", "leaves", "relex-differs", "logger", "leaf-state-or-error"}

// ShortcutCount reports the number of attempts with the given outcome.
func (p *Parser) ShortcutCount(reason string) uint64 {
	for i, name := range shortcutReasons {
		if reason == name {
			return p.shortcutCounts[i]
		}
	}
	return 0
}

func (p *Parser) shortcutDecline(reason string) bool {
	p.shortcutWhy = reason
	for i, name := range shortcutReasons {
		if reason == name {
			p.shortcutCounts[i]++
			break
		}
	}
	return false
}

// shortcutFrame is one node of the paired walk: h in the edited tree, whose
// has_changes mark says whether the edit touches the node, and b, the same node
// in the unedited tree, which gives the positions, the lex state and the leaf
// to compare with. An edit copies the nodes it marks and changes no child list,
// so the two trees have the same shape.
type shortcutFrame struct {
	h       Handle
	b       Handle
	start   Length
	pos     Length
	next    int
	entered bool
}

// shortcutSameLeaf compares the outputs of C/parser.c:505 ts_parser__lex,
// including the external scanner state written by C/parser.c:390.
func (p *Parser) shortcutSameLeaf(a, b Handle) bool {
	return p.symbol(a) == p.symbol(b) && p.padding(a) == p.padding(b) &&
		p.size(a) == p.size(b) && p.lookaheadBytes(a) == p.lookaheadBytes(b) &&
		p.isKeyword(a) == p.isKeyword(b) && p.missing(a) == p.missing(b) &&
		p.hasExternalTokens(a) == p.hasExternalTokens(b) &&
		p.hasFlag(a, inlDependsOnColumn, heapDependsOnColumn) == p.hasFlag(b, inlDependsOnColumn, heapDependsOnColumn) &&
		p.hasExternalStateChange(a) == p.hasExternalStateChange(b) &&
		bytes.Equal(p.extState(a), p.extState(b))
}

// externalOnlyEndLexer recognizes the scanner-owned lexical layout emitted by
// grammargen: the only internal token is unconditional builtin end.
func (t *Tables) externalOnlyEndLexer() bool {
	if len(t.ExternalSymbols) == 0 || uint64(t.TokenCount) != uint64(len(t.ExternalSymbols))+1 ||
		len(t.LexStates) != 1 || len(t.KeywordLexStates) != 0 {
		return false
	}
	s := t.LexStates[0]
	return s.AcceptEnd && s.AcceptToken == 0 && !s.AcceptEOF && !s.Skip &&
		s.Default == -1 && s.EOF == -1 && len(s.Trans) == 0
}

// Reopening a marked ancestor can scan its leading token in the ancestor's
// entry state, even when that token precedes the changed bytes.
func (p *Parser) shortcutExternalPrefixDiffers(entry, first uint16) bool {
	if entry == 0 || entry == stateNone || entry == first || int(entry) >= len(p.t.LexModes) || int(first) >= len(p.t.LexModes) {
		return false
	}
	current, previous := p.t.LexModes[entry].ExternalLexState, p.t.LexModes[first].ExternalLexState
	if current == 0 || current == previous {
		return false
	}
	rows := p.extRows()
	return int(current) >= len(rows) || int(previous) >= len(rows) || !slices.Equal(rows[current], rows[previous])
}

// tryShortcut replays C/parser.c:505 ts_parser__lex for each leaf that the
// edit marked as changed. old is the edited tree: C/subtree.c:634
// ts_subtree_edit set has_changes on the leaves whose text or lookahead the
// edit touches, which are the leaves C's own reparse would lex again, so the
// shortcut lexes no token that C would reuse. Each such leaf is lexed at its
// position and in its state in the unedited tree and compared with the
// unedited leaf. A changed token or an unavailable state leaves the normal
// parse to decide the result. Publication retains the unedited root and sweeps
// the temporary lexed leaves using the normal store rules.
func (p *Parser) tryShortcut(src []byte, old Handle) bool {
	m := p.shortcutMemo
	if p.logf != nil {
		return p.shortcutDecline("logger")
	}
	if m.edits != 1 || m.base == 0 {
		return p.shortcutDecline("no-memo")
	}
	if why := shortcutEditReason(m.src, src, m.edit); why != "" {
		return p.shortcutDecline(why)
	}
	if p.totalSize(m.base).bytes != uint32(len(src)) || p.totalSize(old).bytes != uint32(len(src)) || p.errorCost(m.base) != 0 {
		return p.shortcutDecline("root-extent-or-error")
	}
	p.src, p.stop = src, ""
	p.cacheTok, p.cacheAt = 0, 0
	p.extReset(false)
	defer p.extFinish()
	externalOnly := p.t.externalOnlyEndLexer()
	var frames []shortcutFrame
	frames = push(&p.led, frames, shortcutFrame{h: old, b: m.base})
	var lastExt Handle
	leaves := 0
	for len(frames) != 0 {
		f := &frames[len(frames)-1]
		if !f.entered {
			f.entered = true
			if !p.hasChanges(f.h) {
				if p.hasExternalTokens(f.b) {
					lastExt = p.lastExternalToken(f.b)
				}
				frames = frames[:len(frames)-1]
				continue
			}
			if externalOnly && p.childCount(f.b) != 0 && p.hasExternalTokens(f.b) &&
				p.shortcutExternalPrefixDiffers(p.parseState(f.b), p.leafParseState(f.b)) {
				return p.shortcutDecline("leaf-state-or-error")
			}
			if p.childCount(f.h) != p.childCount(f.b) {
				return p.shortcutDecline("no-memo") // old is not the memo's tree with one edit
			}
			if p.childCount(f.b) == 0 {
				h, start := f.b, f.start
				frames = frames[:len(frames)-1]
				if uint32(p.symbol(h)) >= p.t.TokenCount && p.t.TokenCount > 0 {
					continue
				}
				leaves++
				if leaves > 64 {
					return p.shortcutDecline("leaves")
				}
				state := p.parseState(h)
				if state == 0 || state == stateNone || int(state) >= len(p.t.LexModes) || p.symbol(h) == symError || p.missing(h) {
					return p.shortcutDecline("leaf-state-or-error")
				}
				p.nodes = p.nodes[:0]
				p.heads = p.heads[:0]
				p.nodes = push(&p.led, p.nodes, stackNode{refCount: 1, state: state, position: start})
				p.heads = push(&p.led, p.heads, stackHead{node: 0, lastExternal: lastExt, status: headActive})
				p.cacheTok, p.cacheAt = 0, 0
				p.tokens++
				nh, ok := p.lex(0, state)
				if !ok || nh == 0 || p.stop != "" || !p.shortcutSameLeaf(h, nh) {
					return p.shortcutDecline("relex-differs")
				}
				if p.hasExternalTokens(h) {
					lastExt = h
				}
				continue
			}
			f.pos = f.start
		}
		kids, baseKids := p.children(f.h), p.children(f.b)
		if f.next == len(baseKids) {
			frames = frames[:len(frames)-1]
			continue
		}
		child, baseChild, pos := kids[f.next], baseKids[f.next], f.pos
		f.next++
		f.pos = lenAdd(f.pos, p.totalSize(baseChild)) // positions are the unedited tree's
		frames = push(&p.led, frames, shortcutFrame{h: child, b: baseChild, start: pos})
	}
	if leaves == 0 {
		return p.shortcutDecline("leaves")
	}
	p.finished, p.stop = m.base, ""
	// The one reused node is the root of the whole old tree.
	p.reusedNodes, p.reusedBytes, p.largestReuse = 1, uint64(len(src)), uint32(len(src))
	p.shifts, p.reductions, p.maxVersions, p.multiTokens = 0, 0, 0, 0
	p.maxProcessed, p.selectCandidates = 0, 0
	p.shortcutTaken = true
	p.shortcutCounts[0]++
	return true
}

// shortcutEditReason checks the source and position requirements for one
// replacement. C's incremental parser assumes the edit describes the source;
// verify that assumption before considering reuse of the unedited tree.
func shortcutEditReason(old, src []byte, e editRange) string {
	start, oldEnd, newEnd := e.start.bytes, e.oldEnd.bytes, e.newEnd.bytes
	if start >= oldEnd || start > newEnd || oldEnd-start != newEnd-start ||
		e.oldEnd.row != e.start.row || e.newEnd.row != e.start.row ||
		e.oldEnd.col < e.start.col || e.newEnd.col < e.start.col ||
		e.oldEnd.col-e.start.col != oldEnd-start || e.newEnd.col-e.start.col != newEnd-start {
		return "not-same-length"
	}
	if uint64(oldEnd) > uint64(len(old)) || uint64(newEnd) > uint64(len(src)) || len(old) != len(src) {
		return "edit-mismatch"
	}
	if bytes.IndexByte(old[start:oldEnd], '\n') >= 0 || bytes.IndexByte(src[start:newEnd], '\n') >= 0 {
		return "newline"
	}
	if !bytes.Equal(old[:start], src[:start]) || !bytes.Equal(old[oldEnd:], src[newEnd:]) {
		return "edit-mismatch"
	}
	return ""
}
