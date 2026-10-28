package main

import (
	"hash/fnv"
	"slices"

	"github.com/odvcencio/gotreesitter"
)

// LanguageCompactor compacts generated language tables and reuses repeated
// symbol/transition slices across grammars in a generation run.
type LanguageCompactor struct {
	strings     map[string]string
	transitions map[uint64][][]gotreesitter.LexTransition
	rows        map[uint64][][]uint16
}

func NewLanguageCompactor() *LanguageCompactor {
	return &LanguageCompactor{
		strings:     map[string]string{},
		transitions: map[uint64][][]gotreesitter.LexTransition{},
		rows:        map[uint64][][]uint16{},
	}
}

func (c *LanguageCompactor) CompactLanguage(lang *gotreesitter.Language) {
	if c == nil || lang == nil {
		return
	}

	for i := range lang.SymbolNames {
		lang.SymbolNames[i] = c.internString(lang.SymbolNames[i])
	}
	for i := range lang.FieldNames {
		lang.FieldNames[i] = c.internString(lang.FieldNames[i])
	}
	for i := range lang.SymbolMetadata {
		lang.SymbolMetadata[i].Name = c.internString(lang.SymbolMetadata[i].Name)
	}

	for i := range lang.ParseTable {
		lang.ParseTable[i] = c.internUint16Row(lang.ParseTable[i])
	}

	for i := range lang.LexStates {
		lang.LexStates[i].Transitions = c.internTransitions(compactTransitions(lang.LexStates[i].Transitions))
	}
	for i := range lang.KeywordLexStates {
		lang.KeywordLexStates[i].Transitions = c.internTransitions(compactTransitions(lang.KeywordLexStates[i].Transitions))
	}
}

func (c *LanguageCompactor) internString(s string) string {
	if s == "" {
		return s
	}
	if v, ok := c.strings[s]; ok {
		return v
	}
	c.strings[s] = s
	return s
}

func (c *LanguageCompactor) internUint16Row(row []uint16) []uint16 {
	if len(row) == 0 {
		return row
	}
	hash := hashUint16s(row)
	if bucket, ok := c.rows[hash]; ok {
		for _, existing := range bucket {
			if slices.Equal(existing, row) {
				return existing
			}
		}
	}
	canonical := append([]uint16(nil), row...)
	c.rows[hash] = append(c.rows[hash], canonical)
	return canonical
}

func (c *LanguageCompactor) internTransitions(ts []gotreesitter.LexTransition) []gotreesitter.LexTransition {
	if len(ts) == 0 {
		return ts
	}
	hash := hashLexTransitions(ts)
	if bucket, ok := c.transitions[hash]; ok {
		for _, existing := range bucket {
			if slices.Equal(existing, ts) {
				return existing
			}
		}
	}
	canonical := append([]gotreesitter.LexTransition(nil), ts...)
	c.transitions[hash] = append(c.transitions[hash], canonical)
	return canonical
}

func compactTransitions(ts []gotreesitter.LexTransition) []gotreesitter.LexTransition {
	if len(ts) == 0 {
		return ts
	}
	merged := make([]gotreesitter.LexTransition, 0, len(ts))
	for _, tr := range ts {
		n := len(merged)
		if n > 0 {
			prev := merged[n-1]
			if prev.NextState == tr.NextState &&
				prev.Skip == tr.Skip &&
				prev.Hi < tr.Lo &&
				prev.Hi+1 == tr.Lo {
				merged[n-1].Hi = tr.Hi
				continue
			}
		}
		merged = append(merged, tr)
	}
	return merged
}

func hashUint16s(row []uint16) uint64 {
	h := fnv.New64a()
	var buf [2]byte
	for _, v := range row {
		buf[0] = byte(v)
		buf[1] = byte(v >> 8)
		_, _ = h.Write(buf[:])
	}
	return h.Sum64()
}

func hashLexTransitions(ts []gotreesitter.LexTransition) uint64 {
	h := fnv.New64a()
	var buf [24]byte
	for _, tr := range ts {
		putUint64(buf[0:8], uint64(tr.Lo))
		putUint64(buf[8:16], uint64(tr.Hi))
		putUint32(buf[16:20], uint32(tr.NextState))
		if tr.Skip {
			buf[20] = 1
		} else {
			buf[20] = 0
		}
		_, _ = h.Write(buf[:21])
	}
	return h.Sum64()
}

func putUint64(dst []byte, v uint64) {
	dst[0] = byte(v)
	dst[1] = byte(v >> 8)
	dst[2] = byte(v >> 16)
	dst[3] = byte(v >> 24)
	dst[4] = byte(v >> 32)
	dst[5] = byte(v >> 40)
	dst[6] = byte(v >> 48)
	dst[7] = byte(v >> 56)
}

func putUint32(dst []byte, v uint32) {
	dst[0] = byte(v)
	dst[1] = byte(v >> 8)
	dst[2] = byte(v >> 16)
	dst[3] = byte(v >> 24)
}
