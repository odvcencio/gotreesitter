package query

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

// Difference is the first place where two record lists of one layer differ:
// the index of the first differing record and the record on each side. Want
// is pinned C's side. A side whose list has ended at Index is empty.
type Difference struct {
	Layer     Layer
	Index     int
	Want      string
	Got       string
	WantCount int
	GotCount  int
}

// String reports the difference on one line, for example
// `L3 record 1 of 2 (got 3): want "p0 @a ..." got "p0 @b ..."`.
func (d Difference) String() string {
	var b strings.Builder
	b.WriteString(string(d.Layer))
	b.WriteString(" record ")
	b.WriteString(strconv.Itoa(d.Index))
	b.WriteString(" of ")
	b.WriteString(strconv.Itoa(d.WantCount))
	b.WriteString(" (got ")
	b.WriteString(strconv.Itoa(d.GotCount))
	b.WriteString("): want ")
	writeSide(&b, d.Want, d.Index < d.WantCount)
	b.WriteString(" got ")
	writeSide(&b, d.Got, d.Index < d.GotCount)
	return b.String()
}

func writeSide(b *strings.Builder, record string, present bool) {
	if !present {
		b.WriteString("<end>")
		return
	}
	b.WriteString(strconv.Quote(record))
}

// FirstDifference compares two record lists of one layer in text form, want
// from pinned C and got from the engine under test, and returns the first
// index where they differ. ok is false when the lists are equal.
func FirstDifference(layer Layer, want, got []string) (d Difference, ok bool) {
	n := len(want)
	if len(got) < n {
		n = len(got)
	}
	i := 0
	for i < n && want[i] == got[i] {
		i++
	}
	if i == len(want) && i == len(got) {
		return Difference{}, false
	}
	d = Difference{Layer: layer, Index: i, WantCount: len(want), GotCount: len(got)}
	if i < len(want) {
		d.Want = want[i]
	}
	if i < len(got) {
		d.Got = got[i]
	}
	return d, true
}

// Texts returns the text form of each record.
func Texts[R Record](records []R) []string {
	out := make([]string, len(records))
	for i, r := range records {
		out[i] = r.String()
	}
	return out
}

// Digest returns a short, stable digest of a record list in text form: the
// record count and the first 16 hex digits of the SHA-256 of the records,
// each followed by a newline. Logs use it to name a record list in one line.
func Digest(records []string) string {
	h := sha256.New()
	for _, r := range records {
		h.Write([]byte(r))
		h.Write([]byte{'\n'})
	}
	return "n=" + strconv.Itoa(len(records)) + " sha256=" + hex.EncodeToString(h.Sum(nil))[:16]
}
