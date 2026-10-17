package parsercorephase0

import (
	"math/rand"
	"testing"
)

// Check cached child occurrences against independent materialization accounting
// across shared graphs, warm/cold traversal orders and mutable visibility.
func TestCoreRecoveryVisibleCachedChildrenMatchMaterialization(t *testing.T) {
	for seed := int64(1); seed <= 24; seed++ {
		random := rand.New(rand.NewSource(seed))
		c := newRecoveryVisibleCore(t)
		symbols := visibleSymbols()
		var ids []SubtreeID
		var depths []int
		check := func(id SubtreeID) {
			t.Helper()
			want, referenceErr := RecoveryNodeVisibleSubtreeCount(symbols, recoveryVisibleMaterializationSource{c}, id)
			got, err := c.CachedVisibleSubtreeCount(symbols, id)
			if err != nil || referenceErr != nil || got != want {
				t.Fatalf("seed=%d id=%d count=%d/%v reference=%d/%v", seed, id, got, err, want, referenceErr)
			}
		}
		for index := 0; index < 48; index++ {
			var children []SubtreeID
			depth := 0
			if len(ids) != 0 {
				for count := random.Intn(4); count > 0; count-- {
					child := random.Intn(len(ids))
					if depths[child] >= 5 {
						continue
					}
					children = append(children, ids[child])
					depth = max(depth, depths[child]+1)
					if seed%2 == 0 {
						check(ids[child])
					}
				}
			}
			var aliases []Symbol
			if random.Intn(2) != 0 {
				aliases = make([]Symbol, len(children))
				for child := range aliases {
					if random.Intn(3) == 0 {
						aliases[child] = ordinarySymbol
					}
				}
			}
			choices := [...]Symbol{ordinarySymbol, 6, RecoveryErrorSymbol, RecoveryErrorRepeatSymbol}
			id := appendRecoveryVisibleFixture(t, c, choices[random.Intn(len(choices))], random.Intn(5) == 0, random.Intn(4) == 0, children, aliases)
			ids = append(ids, id)
			depths = append(depths, depth)
			check(id)
		}
		for _, visible := range []bool{false, true} {
			symbols[ordinarySymbol].Visible = visible
			for _, index := range random.Perm(len(ids)) {
				check(ids[index])
			}
		}
	}
}
