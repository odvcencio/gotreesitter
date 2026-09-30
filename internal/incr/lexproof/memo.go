package lexproof

import "bytes"

// WindowCapacity bounds both retained source bytes and cache lookup work.
// A wider proof runs normally and is never cached.
const WindowCapacity = 512

// Edit identifies the byte and point inputs to one same-width primitive proof.
type Edit struct {
	Start, End        uint32
	Row, Column       uint32
	EndRow, EndColumn uint32
}

type memoEntry struct {
	edit                 Edit
	sourceLen, inputSpan uint32
	resultSpan, start    uint32
	length               uint16
	oldBOM, newBOM       [3]byte
	oldBytes, newBytes   [WindowCapacity]byte
}

// Memo remembers two successful, directed proofs. It copies only their bounded
// dependency windows, retains no source buffers, and never infers the inverse.
// Its owner must authenticate immutable grammar tables and scanner parameters.
type Memo struct {
	entries     [2]memoEntry
	count, next uint8
}

func (m *Memo) Lookup(oldSource, newSource []byte, edit Edit, inputSpan uint32) (uint32, bool) {
	if m == nil || len(oldSource) != len(newSource) || uint64(len(oldSource)) > uint64(^uint32(0)) {
		return 0, false
	}
	oldBOM, newBOM := leadingBytes(oldSource), leadingBytes(newSource)
	for i := uint8(0); i < m.count; i++ {
		e := &m.entries[i]
		if e.edit != edit || e.sourceLen != uint32(len(oldSource)) || e.inputSpan != inputSpan ||
			e.oldBOM != oldBOM || e.newBOM != newBOM {
			continue
		}
		end := uint64(e.start) + uint64(e.length)
		if end <= uint64(len(oldSource)) &&
			bytes.Equal(oldSource[e.start:end], e.oldBytes[:e.length]) &&
			bytes.Equal(newSource[e.start:end], e.newBytes[:e.length]) {
			return e.resultSpan, true
		}
	}
	return 0, false
}

func (m *Memo) Remember(oldSource, newSource []byte, edit Edit, inputSpan, resultSpan uint32) bool {
	start, end, ok := dependencyWindow(oldSource, newSource, edit, inputSpan, resultSpan)
	if !ok || m == nil {
		return false
	}
	e := &m.entries[m.next]
	*e = memoEntry{
		edit: edit, sourceLen: uint32(len(oldSource)), inputSpan: inputSpan,
		resultSpan: resultSpan, start: start, length: uint16(end - start),
		oldBOM: leadingBytes(oldSource), newBOM: leadingBytes(newSource),
	}
	copy(e.oldBytes[:], oldSource[start:end])
	copy(e.newBytes[:], newSource[start:end])
	m.next = (m.next + 1) % uint8(len(m.entries))
	if m.count < uint8(len(m.entries)) {
		m.count++
	}
	return true
}

func leadingBytes(source []byte) (result [3]byte) {
	copy(result[:], source)
	return result
}

// Successful probes examine at most resultSpan bytes in the new source. Old
// probes excluded by the captured bound also examine the cutoff's five-byte
// decoding margin. Whitespace gates sample four bytes either side of the edit,
// and each sample decodes another four bytes in either direction.
// Point derivation can inspect the preceding line when crossing a newline.
func dependencyWindow(oldSource, newSource []byte, edit Edit, inputSpan, resultSpan uint32) (uint32, uint32, bool) {
	if len(oldSource) != len(newSource) || uint64(len(oldSource)) > uint64(^uint32(0)) ||
		edit.Start >= edit.End || uint64(edit.End) > uint64(len(oldSource)) || inputSpan == 0 || resultSpan == 0 {
		return 0, 0, false
	}
	origin := uint32(0)
	if edit.Start > inputSpan {
		origin = edit.Start - inputSpan
	}
	start := origin
	if edit.Start > 8 {
		start = min(start, edit.Start-8)
	} else {
		start = 0
	}
	margin := max(uint64(max(inputSpan, resultSpan))+5, 8)
	end := min(uint64(len(oldSource)), uint64(edit.End)+margin)
	if end-uint64(start) > WindowCapacity {
		return 0, 0, false
	}
	if newline := bytes.IndexByte(oldSource[origin:edit.Start], '\n'); newline >= 0 {
		// The earliest crossed newline starts the farthest previous-line scan.
		previous := origin + uint32(newline)
		for previous > 0 && oldSource[previous-1] != '\n' {
			previous--
			if end-uint64(previous) > WindowCapacity {
				return 0, 0, false
			}
		}
		start = min(start, previous)
	}
	return start, uint32(end), true
}
