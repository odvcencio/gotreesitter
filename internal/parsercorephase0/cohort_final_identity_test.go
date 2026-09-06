package parsercorephase0

import "testing"

func TestShiftCohortReturnsFinalPhysicalHeads(t *testing.T) {
	for _, extra := range []bool{false, true} {
		kind := "ordinary"
		if extra {
			kind = "extra"
		}
		for _, sharedTarget := range []bool{false, true} {
			name := "distinct_targets"
			if sharedTarget {
				name = "shared_target"
			}
			t.Run(kind+"/"+name, func(t *testing.T) {
				rightTarget := StateID(5)
				if sharedTarget {
					rightTarget = 4
				}
				tables := &fakeTable{actions: map[tableCell][]Action{
					{state: 1, symbol: 3}: {{Type: ActionShift, State: 2}},
					{state: 1, symbol: 4}: {{Type: ActionShift, State: 3}},
					{state: 2, symbol: 9}: {{Type: ActionShift, State: 4, Extra: extra}},
					{state: 3, symbol: 9}: {{Type: ActionShift, State: rightTarget, Extra: extra}},
				}}
				compact, err := New(tables, Limits{MaxDerivations: 8, MaxPopPaths: 8})
				if err != nil {
					t.Fatal(err)
				}
				seed, err := compact.Seed(1, 0)
				if err != nil {
					t.Fatal(err)
				}
				boundaries := make([]ClassifiedBoundary, 2)
				prefixHeads := make([]Head, 2)
				for i, symbol := range []Symbol{3, 4} {
					head, err := compact.Shift(seed, symbol, 0, Token{Symbol: symbol, EndByte: 1}, ForkOrder{})
					if err != nil {
						t.Fatal(err)
					}
					prefixHeads[i] = head
				}
				for i, head := range prefixHeads {
					boundaries[i], err = compact.ClassifyBoundary(head, 9)
					if err != nil {
						t.Fatal(err)
					}
				}
				var heads []Head
				// Only this owned invocation can merge the new cohort outputs.
				err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
					token := Token{Symbol: 9, StartByte: 1, EndByte: 2, Extra: extra}
					var err error
					if extra {
						heads, err = compact.ShiftExtraClassifiedCohortWithLiveCondenseCandidatesOwned(owner, nil, boundaries, token)
					} else {
						heads, err = compact.ShiftOrdinaryClassifiedCohortWithLiveCondenseCandidatesOwned(owner, nil, boundaries, token)
					}
					return err
				})
				if err != nil {
					t.Fatal(err)
				}
				if len(heads) != 2 || (heads[0] == heads[1]) != sharedTarget {
					t.Fatalf("cohort heads=%+v, shared target=%t", heads, sharedTarget)
				}
				for i, head := range heads {
					paths, err := compact.Derivations(head)
					if err != nil {
						t.Fatal(err)
					}
					wantPaths := 1
					if sharedTarget {
						wantPaths = 2
					}
					if len(paths) != wantPaths {
						t.Fatalf("head %d paths=%+v, want %d", i, paths, wantPaths)
					}
					seen := make(map[Symbol]int)
					for _, path := range paths {
						if len(path.Payloads) != 2 {
							t.Fatalf("path=%+v, want prefix and shifted token", path)
						}
						prefix, err := compact.Subtree(path.Payloads[0])
						if err != nil {
							t.Fatal(err)
						}
						seen[prefix.Symbol]++
						tail, err := compact.Subtree(path.Payloads[1])
						if err != nil || tail.Symbol != 9 || tail.Extra != extra {
							t.Fatalf("shifted payload=%+v err=%v", tail, err)
						}
					}
					if sharedTarget {
						if seen[3] != 1 || seen[4] != 1 {
							t.Fatalf("merged prefix counts=%v, want each prefix once", seen)
						}
					} else if seen[Symbol(3+i)] != 1 {
						t.Fatalf("head %d prefix counts=%v, want its original prefix", i, seen)
					}
				}
			})
		}
	}
}
