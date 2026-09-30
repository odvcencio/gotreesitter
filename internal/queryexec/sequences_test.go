package queryexec

import "testing"

func TestCaptureSequenceIdentity(t *testing.T) {
	var sequences Sequences[int]
	a := sequences.Extend(0, "root", 1)
	ab := sequences.Extend(a, "item", 2)
	if sequences.Seen(ab) || !sequences.Seen(sequences.Extend(sequences.Extend(0, "root", 1), "item", 2)) {
		t.Fatal("equal sequences did not share identity")
	}
	for _, distinct := range []uint64{a, sequences.Extend(a, "other", 2), sequences.Extend(a, "item", 3), sequences.Extend(0, "item", 2), 0} {
		if sequences.Seen(distinct) {
			t.Fatal("distinct sequence was discarded")
		}
	}
}
