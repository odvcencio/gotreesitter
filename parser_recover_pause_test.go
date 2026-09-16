package gotreesitter

import "testing"

func pauseTestVisibleTree(count int) *Node {
	n := &Node{symbol: 1, equivVersion: 1}
	for i := 1; i < count; i++ {
		n.children = append(n.children, &Node{symbol: 1, equivVersion: 1})
	}
	return n
}

func TestCPauseStackRecordsCurrentProgress(t *testing.T) {
	for _, representation := range []string{"entries", "gss", "entries_memo", "gss_memo"} {
		for _, recovering := range []bool{false, true} {
			name := representation + "/fresh"
			if recovering {
				name = representation + "/recovering"
			}
			t.Run(name, func(t *testing.T) {
				p := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{{}, {Visible: true}}}}
				if representation == "entries_memo" || representation == "gss_memo" {
					p.cNodeMemoCache = make([]cNodeMemoCacheEntry, cNodeMemoCacheSize)
				}
				prefix := pauseTestVisibleTree(17)
				// An invisible wrapper contributes only its visible descendants.
				wrapper := &Node{symbol: 0, equivVersion: 1, children: []*Node{prefix}}
				s := glrStack{entries: []stackEntry{{state: 1}, newStackEntryNode(2, wrapper)}, cNodeBaseline: 99}
				if recovering {
					s.cEverErrored = true
					s.cRec = &cRecoverState{summary: []cStackSummaryEntry{{state: 2, depth: 1}}}
				}
				var scratch gssScratch
				if representation == "gss" || representation == "gss_memo" {
					s.ensureGSS(&scratch)
				}
				if got := p.cStackCumulativeNodeCount(&s); got != 17 {
					t.Fatalf("warm count = %d, want 17", got)
				}
				recovery := s.cRec
				p.cPauseStack(&s)
				if !s.cPaused || s.cNodeBaseline != 17 || p.cNodeCountSinceError(&s) != 0 {
					t.Fatalf("paused=%v baseline=%d progress=%d, want true, 17, 0", s.cPaused, s.cNodeBaseline, p.cNodeCountSinceError(&s))
				}
				if !p.crecoveryCostCompetitionRelevant || !p.crecoveryCostCompetitionWalkEnabled {
					t.Fatal("pause did not mark recovery costs relevant")
				}
				if s.cRec != recovery || s.cEverErrored != recovering {
					t.Fatal("pause changed the recovery summary or lineage history")
				}
				s.push(3, pauseTestVisibleTree(2), nil, &scratch)
				if got := p.cNodeCountSinceError(&s); got != 2 {
					t.Fatalf("progress after push = %d, want 2", got)
				}
				p.cPauseStack(&s)
				if s.cNodeBaseline != 19 || p.cNodeCountSinceError(&s) != 0 {
					t.Fatal("second pause retained the first baseline")
				}
				if !s.truncate(2) || p.cNodeCountSinceError(&s) != 0 || s.cNodeBaseline != 17 {
					t.Fatal("pop below the pause baseline did not preserve the C clamp")
				}
			})
		}
	}

	t.Run("empty", func(t *testing.T) {
		p := &Parser{}
		s := glrStack{cNodeBaseline: 99}
		p.cPauseStack(&s)
		if !s.cPaused || s.cNodeBaseline != 0 || p.cNodeCountSinceError(&s) != 0 {
			t.Fatal("empty pause did not clear the previous baseline")
		}
	})
}

func TestCPauseStackMissingForkKeepsPauseBaseline(t *testing.T) {
	for _, cloneKind := range []string{"entries", "gss", "scratch"} {
		t.Run(cloneKind, func(t *testing.T) {
			p := &Parser{
				language:       &Language{SymbolMetadata: []SymbolMetadata{{}, {Visible: true}}},
				cNodeMemoCache: make([]cNodeMemoCacheEntry, cNodeMemoCacheSize),
			}
			seed := glrStack{entries: []stackEntry{{state: 1}, newStackEntryNode(2, pauseTestVisibleTree(17))}, byteOffset: 34}
			var scratch gssScratch
			if cloneKind == "gss" {
				seed.ensureGSS(&scratch)
			}
			p.cPauseStack(&seed)
			// cHandleError resumes the stack before making recovery copies.
			seed.cPaused = false
			seed.cEverErrored = true
			var missing glrStack
			if cloneKind == "scratch" {
				missing = seed.cloneWithScratch(&scratch)
			} else {
				missing = seed.clone()
			}
			if missing.cPaused || !missing.cEverErrored || missing.cNodeBaseline != 17 {
				t.Fatal("recovery copy lost its pause baseline or lineage state")
			}
			// The C trace reaches 21 cumulative nodes after inserting and reducing ')'.
			addition := pauseTestVisibleTree(4)
			addition.children[0].setMissing(true)
			missing.push(3, addition, nil, &scratch)
			status := p.cVersionStatus(&missing)
			if status.cost != 610 || status.nodeCount != 4 || status.isInError {
				t.Fatalf("missing status = %+v, want cost 610 and progress 4 outside ERROR_STATE", status)
			}
			absorbing := []glrStack{seed.clone()}
			absorbing[0].push(4, pauseTestVisibleTree(3), nil, &scratch)
			if got := p.cApplyMergedErrorGroupBaseline(absorbing); got != 20 {
				t.Fatalf("merged baseline = %d, want 20", got)
			}
			if len(p.cRecordSummary(cStackEntriesTopFirst(&absorbing[0], nil))) == 0 {
				t.Fatal("absorbing summary is empty")
			}
			if missing.cNodeBaseline != 17 || p.cNodeCountSinceError(&missing) != 4 {
				t.Fatal("later absorbing baseline or summary changed the missing copy")
			}
			stacks := []glrStack{absorbing[0], missing}
			if p.cBetterVersionExists(stacks, 0, false, 703) {
				t.Fatal("pre-error nodes made the missing copy reject the cost-703 recovery")
			}
			// Preserve the rejection threshold once this branch makes real progress.
			stacks[1].push(5, pauseTestVisibleTree(16), nil, &scratch)
			if !p.cBetterVersionExists(stacks, 0, false, 703) {
				t.Fatal("sufficient post-error progress did not reject the more expensive recovery")
			}
		})
	}
}
