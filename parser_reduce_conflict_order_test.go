package gotreesitter

import "testing"

func TestOrdinaryReductionConflictTableOrder(t *testing.T) {
	reduce := ParseAction{Type: ParseActionReduce}
	for _, tc := range []struct {
		name    string
		actions []ParseAction
		want    bool
	}{
		{"two_reductions", []ParseAction{reduce, reduce}, true},
		{"three_reductions", []ParseAction{reduce, reduce, reduce}, true},
		{"single", []ParseAction{reduce}, false},
		{"mixed", []ParseAction{reduce, {Type: ParseActionShift}}, false},
		{"recover", []ParseAction{reduce, {Type: ParseActionRecover}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := cAllReductionConflict(tc.actions); got != tc.want {
				t.Fatalf("all reductions=%t, want %t", got, tc.want)
			}
		})
	}
}

func TestOrdinaryReductionConflictPromotesLastSurvivingArm(t *testing.T) {
	source, suffix := newGLRStack(1), newGLRStack(320)
	source.dead = true
	source.branchOrder, suffix.branchOrder = 10, 20
	first, second := newGLRStack(236), newGLRStack(240)
	first.branchOrder, second.branchOrder = 30, 40
	dead := newGLRStack(999)
	dead.dead = true
	stacks := []glrStack{source, suffix, first, dead, second}
	selected := firstLiveConflictReductionVersion(stacks, 2)
	if selected != 2 {
		t.Fatalf("first arm selected=%d", selected)
	}
	if live := firstLiveConflictReductionVersion(stacks, 3); live >= 0 {
		selected = live
	}
	if selected != 4 {
		t.Fatalf("last arm selected=%d", selected)
	}
	if live := firstLiveConflictReductionVersion(stacks, len(stacks)); live >= 0 {
		selected = live
	}
	stacks, ok := cRenumberReductionVersion(stacks, selected, 0)
	if !ok || len(stacks) != 4 || stacks[0].top().state != 240 || stacks[1].top().state != 320 || stacks[2].top().state != 236 {
		t.Fatalf("promotion changed physical order: %+v", stacks)
	}
	if stacks[0].branchOrder != 40 || stacks[1].branchOrder != 20 || stacks[2].branchOrder != 30 {
		t.Fatal("promotion changed grammar ranks")
	}
}

func TestOrdinaryReductionConflictImmediateSelectionPrecedesContinuation(t *testing.T) {
	first, later := newGLRStack(236), newGLRStack(240)
	selected := immediateConflictReductionVersion(&first, nil, 0, true)
	if selected != 0 {
		t.Fatal("first reduction produced no source result")
	}
	selected = immediateConflictReductionVersion(&later, nil, 0, true)
	later.dead = true
	if selected != 0 {
		t.Fatal("successful reduction lost source ownership")
	}
	failed := newGLRStack(999)
	failed.dead = true
	if got := immediateConflictReductionVersion(&failed, []glrStack{failed, first}, 0, true); got != 2 {
		t.Fatalf("immediate pending output=%d, want 2", got)
	}
	if got := immediateConflictReductionVersion(&failed, nil, 0, true); got != -1 {
		t.Fatalf("failed reduction produced output %d", got)
	}
	if got := immediateConflictReductionVersion(&failed, []glrStack{first}, 0, false); got != -1 {
		t.Fatalf("undrained output selected=%d", got)
	}
	if got := immediateConflictReductionVersion(&failed, []glrStack{first, failed}, 1, true); got != -1 {
		t.Fatalf("stale pending prefix selected=%d", got)
	}
	if got := immediateConflictReductionVersion(&failed, []glrStack{first, failed, first}, 1, true); got != 3 {
		t.Fatalf("fresh output lost its queue offset: %d", got)
	}
}

func TestOrdinaryReductionConflictFailedFinalArmParse(t *testing.T) {
	lang := buildArithmeticRecoveryGarbageLanguage()
	lang.ParseActions[2].Actions = []ParseAction{
		{Type: ParseActionReduce, Symbol: 4, ChildCount: 1},
		{Type: ParseActionReduce, Symbol: 4, ChildCount: 2, ProductionID: 1},
	}
	parser := NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	profile := NewAmbiguityProfile()
	parser.SetAmbiguityProfile(profile)
	tree := mustParse(t, parser, []byte("1"))
	defer tree.Release()
	if tree.RootNode() == nil || tree.RootNode().SExpr(lang) != "(expression (NUMBER))" {
		t.Fatalf("failed final arm lost the earlier result: %v", tree.RootNode())
	}
	if tree.ParseRuntime().CRecoveryEnteredErrorState {
		t.Fatal("failed final arm required recovery")
	}
	dispatched := false
	for _, stat := range profile.SnapshotTop(10) {
		if stat.State == 2 && stat.ActionCount == 2 && stat.ReduceCount == 2 && stat.Forks > 0 {
			dispatched = true
		}
	}
	if !dispatched {
		t.Fatal("parse did not dispatch the two-reduction conflict")
	}
}
