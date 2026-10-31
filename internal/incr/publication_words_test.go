package incr

import "testing"

func TestPublicationWordPreservesStorageAndBudget(t *testing.T) {
	words := make([]uint32, 2, 4)
	words[0] = 17
	reserve := func(int64) bool { t.Fatal("retained capacity charged again"); return false }
	w, growth := PublicationWord(&words, 3, 4, reserve)
	if w == nil || *w != 0 || growth != 0 || words[0] != 17 || len(words) != 4 {
		t.Fatal("retained capacity or prior receipt lost")
	}
	*w = 23
	w, growth = PublicationWord(&words, 5, 8, func(n int64) bool { return n == 16 })
	if w == nil || growth != 16 || words[0] != 17 || words[3] != 23 || len(words) != 8 {
		t.Fatal("growth changed receipts or charge")
	}
	old := &words[0]
	w, growth = PublicationWord(&words, 9, 12, func(int64) bool { return false })
	if w != nil || growth != 0 || len(words) != 8 || &words[0] != old {
		t.Fatal("budget rejection mutated storage")
	}
	for _, index := range []int{-1, 8} {
		if w, growth = PublicationWord(&words, index, 8, reserve); w != nil || growth != 0 {
			t.Fatal("invalid slot accepted")
		}
	}
}
