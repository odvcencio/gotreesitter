package parsercorephase0

import "testing"

func TestRecoveryCostMemoTruncateAboveForgetsRolledBackIDs(t *testing.T) {
	var memo RecoveryCostMemo
	memo.store(1, 10)
	memo.store(2, 20)
	memo.store(3, 30)
	memo.TruncateAbove(2)
	if cost, ok := memo.lookup(1); !ok || cost != 10 {
		t.Fatalf("id 1 = (%d, %t), want (10, true)", cost, ok)
	}
	if cost, ok := memo.lookup(2); !ok || cost != 20 {
		t.Fatalf("id 2 = (%d, %t), want (20, true)", cost, ok)
	}
	if _, ok := memo.lookup(3); ok {
		t.Fatal("id 3 survived the truncation")
	}
	memo.store(3, 33)
	if cost, ok := memo.lookup(3); !ok || cost != 33 {
		t.Fatalf("id 3 after restore = (%d, %t), want (33, true)", cost, ok)
	}
	memo.TruncateAbove(0)
	if _, ok := memo.lookup(1); ok {
		t.Fatal("id 1 survived a full truncation")
	}
}
