package gotreesitter

import "testing"

func ownershipTestVersions(skipped int) (*Parser, glrStack, glrStack) {
	p := &Parser{language: &Language{SymbolMetadata: []SymbolMetadata{{}, {Visible: true}}}, errorCostCompetition: true}
	errNode := &Node{symbol: errorSymbol, startByte: uint32(36 - skipped), endByte: 36, equivVersion: 1}
	for i := 0; i < skipped; i++ {
		errNode.children = append(errNode.children, &Node{symbol: 1, startByte: uint32(36 - skipped + i), endByte: uint32(37 - skipped + i), equivVersion: 1})
	}
	group := &cRecGroup{}
	absorbing := glrStack{entries: []stackEntry{{state: 1}, newStackEntryNode(cErrorState, errNode)}, byteOffset: 36, cEverErrored: true, cRec: &cRecoverState{group: group, openErr: errNode}}
	missingNode := &Node{symbol: 1, startByte: 34, endByte: 34, equivVersion: 1}
	missingNode.setMissing(true)
	missing := glrStack{entries: []stackEntry{{state: 1}, newStackEntryNode(2, missingNode)}, byteOffset: 36, cEverErrored: true, cRecoverMissingGroup: group}
	return p, absorbing, missing
}

func condenseOwnershipTestVersions(t *testing.T, p *Parser, first, second glrStack) []glrStack {
	t.Helper()
	var count int
	stacks, redispatch, _, reason := p.cCondenseAndResume([]glrStack{first, second}, nil, nil, Token{Symbol: 1, StartByte: 36, EndByte: 37}, &count, nil, nil, nil, nil, nil, nil)
	if reason != ParseStopNone || redispatch {
		t.Fatalf("condense stopped or resumed unexpectedly: reason=%v redispatch=%v", reason, redispatch)
	}
	return stacks
}

func TestCCondenseOwnershipAllowsDecisiveRemoval(t *testing.T) {
	for _, missingFirst := range []bool{false, true} {
		name := "absorber_first"
		if missingFirst {
			name = "missing_first"
		}
		t.Run(name, func(t *testing.T) {
			p, absorbing, missing := ownershipTestVersions(2)
			if a, b := p.cVersionStatus(&absorbing), p.cVersionStatus(&missing); a.cost != 702 || b.cost != 610 || !a.isInError || b.isInError {
				t.Fatalf("incorrect cost/category witness: absorber=%+v missing=%+v", a, b)
			}
			first, second := absorbing, missing
			if missingFirst {
				first, second = missing, absorbing
			}
			got := condenseOwnershipTestVersions(t, p, first, second)
			if len(got) != 1 || got[0].cRec != nil || got[0].cRecoverMissingGroup != missing.cRecoverMissingGroup || !got[0].cEverErrored {
				t.Fatalf("decisive active version did not survive alone: count=%d", len(got))
			}
		})
	}
}

func TestCCondenseOwnershipPreservesNondecisiveOrder(t *testing.T) {
	for _, missingFirst := range []bool{false, true} {
		name := "absorber_first"
		if missingFirst {
			name = "missing_first"
		}
		t.Run(name, func(t *testing.T) {
			p, absorbing, missing := ownershipTestVersions(1)
			if a, b := p.cVersionStatus(&absorbing), p.cVersionStatus(&missing); a.cost != 601 || b.cost != 610 {
				t.Fatalf("incorrect preference witness: absorber=%+v missing=%+v", a, b)
			}
			first, second := absorbing, missing
			if missingFirst {
				first, second = missing, absorbing
			}
			got := condenseOwnershipTestVersions(t, p, first, second)
			if len(got) != 2 || got[0].cRec == nil || got[1].cRecoverMissingGroup != missing.cRecoverMissingGroup {
				t.Fatalf("nondecisive ownership order changed: count=%d", len(got))
			}
		})
	}
}

func TestCCondenseOwnershipLifecycleControls(t *testing.T) {
	t.Run("unrelated_missing", func(t *testing.T) {
		p, absorbing, missing := ownershipTestVersions(2)
		missing.cRecoverMissingGroup = &cRecGroup{}
		got := condenseOwnershipTestVersions(t, p, absorbing, missing)
		if len(got) != 1 || got[0].cRec != nil {
			t.Fatal("unrelated cheaper active version lost")
		}
	})
	t.Run("accepted_missing_is_not_a_condense_competitor", func(t *testing.T) {
		p, absorbing, missing := ownershipTestVersions(2)
		missing.accepted = true
		got := condenseOwnershipTestVersions(t, p, missing, absorbing)
		if len(got) != 2 || got[0].cRec == nil || !got[1].accepted {
			t.Fatal("accepted result entered the condense competition")
		}
	})
	t.Run("paused_absorber", func(t *testing.T) {
		p, absorbing, missing := ownershipTestVersions(2)
		p.cPauseStack(&absorbing)
		got := condenseOwnershipTestVersions(t, p, absorbing, missing)
		if len(got) != 1 || got[0].cPaused || got[0].cRec != nil {
			t.Fatal("paused absorber displaced the active missing version")
		}
	})
	t.Run("paused_missing", func(t *testing.T) {
		p, absorbing, missing := ownershipTestVersions(2)
		p.cPauseStack(&missing)
		got := condenseOwnershipTestVersions(t, p, missing, absorbing)
		if len(got) != 1 || got[0].cPaused || got[0].cRec == nil {
			t.Fatal("paused missing version displaced the absorber")
		}
	})
}
