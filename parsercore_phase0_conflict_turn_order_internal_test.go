//go:build gts_parsercorephase0

package gotreesitter

import (
	"errors"
	"reflect"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestRecoveryTurnConflictPhysicalOrder(t *testing.T) {
	reduce7 := core.Action{Type: core.ActionReduce, Symbol: 7, ChildCount: 1}
	reduce8 := core.Action{Type: core.ActionReduce, Symbol: 8, ChildCount: 1}
	shift := core.Action{Type: core.ActionShift, State: 11}
	for _, tc := range []struct {
		name      string
		actions   []core.Action
		states    []core.StateID
		sequences []uint64
	}{
		{"shift_reduce", []core.Action{reduce7, shift}, []core.StateID{11, 4}, []uint64{1, 2}},
		{"multiple_reduce_shift", []core.Action{reduce7, reduce8, shift}, []core.StateID{11, 4, 6}, []uint64{1, 2, 3}},
		{"reduce_reduce", []core.Action{reduce7, reduce8}, []core.StateID{6, 4}, []uint64{3, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, recovery := range []bool{false, true} {
				for _, rollback := range []bool{false, true} {
					table := &genericConflictTable{cells: map[genericConflictCell][]core.Action{{state: 1, symbol: 5}: {{Type: core.ActionShift, State: 3}}, {state: 3, symbol: 10}: tc.actions}, gotos: map[genericConflictCell]core.StateID{{state: 1, symbol: 7}: 4, {state: 1, symbol: 8}: 6}}
					compact, err := core.New(table, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
					if err != nil {
						t.Fatal(err)
					}
					seed, err := compact.Seed(1, 0)
					if err != nil {
						t.Fatal(err)
					}
					var head core.Head
					if recovery {
						head, err = compact.ShiftMissingLeaf(seed, 3, 5, 0)
					} else {
						head, err = compact.Shift(seed, 5, 0, core.Token{Symbol: 5, EndByte: 1}, core.ForkOrder{})
					}
					if err != nil {
						t.Fatal(err)
					}
					header := diagnosticParserCoreHeader{head: head, creationSeq: 1}
					if recovery {
						header.markRecoveryLineage()
					}
					s := &diagnosticParserCoreGenericScheduler{compact: compact, headers: []diagnosticParserCoreHeader{header}, token: Token{Symbol: 10, EndByte: 1}, tokenSource: &dfaTokenSource{language: &Language{TokenCount: 11, SymbolMetadata: []SymbolMetadata{{}, {}, {}, {}, {}, {Visible: true}}}}, branchOrder: 1, nextSeq: 2, options: DiagnosticParserCorePrefixOptions{MaxDispatches: 10, materializationSource: []byte("x")}, receipt: &DiagnosticParserCoreGenericScheduler{}}
					if !recovery {
						s.token.StartByte, s.token.EndByte = 1, 2
						s.options.materializationSource = []byte("xy")
					}
					s.recoveryTurns.active = recovery
					s.recoveryIsolation = recovery
					before, err := diagnosticParserCoreHeaderReceipts(compact, s.headers)
					if err != nil {
						t.Fatal(err)
					}
					fault := errors.New("conflict rollback")
					if rollback {
						s.conflictPostExecutionFault = func() error { return fault }
					}
					err = s.applyGenericConflict(before, mustDiagnosticParserCoreGenericCell(t, compact, 0, header, 10))
					if rollback {
						if !errors.Is(err, fault) || len(s.headers) != 1 || s.headers[0].head != head || s.headers[0].creationSeq != 1 || s.nextSeq != 2 {
							t.Fatalf("rollback: err=%v headers=%+v next=%d", err, s.headers, s.nextSeq)
						}
						continue
					}
					if err != nil {
						t.Fatal(err)
					}
					var states []core.StateID
					var sequences []uint64
					for _, h := range s.headers {
						state, _, err := compact.Boundary(h.head)
						if err != nil {
							t.Fatal(err)
						}
						states = append(states, state)
						sequences = append(sequences, h.creationSeq)
					}
					if !reflect.DeepEqual(states, tc.states) || !reflect.DeepEqual(sequences, tc.sequences) {
						t.Fatalf("recovery=%t states=%v sequences=%v; want %v %v", recovery, states, sequences, tc.states, tc.sequences)
					}
					conflict := s.receipt.Conflicts[0]
					if conflict.PrimaryOutput.State != StateID(tc.states[0]) || conflict.PrimaryPaused || conflict.PrimaryAdopted {
						t.Fatalf("recovery=%t primary receipt=%+v", recovery, conflict)
					}
					for _, arm := range conflict.SecondaryArms {
						wantOrder := uint64(0)
						if arm.Ordinal != 0 {
							wantOrder = uint64(arm.Ordinal + 1)
						}
						if arm.Ordinal == len(tc.actions)-1 || arm.BranchOrder != wantOrder || arm.Paused || arm.Adopted {
							t.Fatalf("recovery=%t secondary receipt=%+v", recovery, arm)
						}
					}
					for ordinal, action := range s.receipt.Conflicts[0].Round.Actions {
						wantOrder := uint64(0)
						if ordinal != 0 {
							wantOrder = uint64(ordinal + 1)
						}
						if action.Ordinal != ordinal || action.BranchOrder != wantOrder {
							t.Fatalf("recovery=%t action %d=%+v, want order %d", recovery, ordinal, action, wantOrder)
						}
					}
				}
			}
		})
	}
}

func TestReductionMultipleOutputsPreserveExistingSuffixOrder(t *testing.T) {
	for _, recovery := range []bool{false, true} {
		t.Run(map[bool]string{false: "clean", true: "recovery"}[recovery], func(t *testing.T) {
			table := &genericConflictTable{
				cells: map[genericConflictCell][]core.Action{
					{state: 1, symbol: 9}:  {{Type: core.ActionShift, State: 3}},
					{state: 2, symbol: 9}:  {{Type: core.ActionShift, State: 3}},
					{state: 3, symbol: 10}: {{Type: core.ActionReduce, Symbol: 7, ChildCount: 1}},
				},
				gotos: map[genericConflictCell]core.StateID{
					{state: 1, symbol: 7}: 236,
					{state: 2, symbol: 7}: 240,
				},
			}
			compact, err := core.New(table, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
			if err != nil {
				t.Fatal(err)
			}
			var head core.Head
			for _, state := range []core.StateID{1, 2} {
				seed, err := compact.Seed(state, 0)
				if err != nil {
					t.Fatal(err)
				}
				head, err = compact.Shift(seed, 9, 0, core.Token{Symbol: 9, EndByte: 1}, core.ForkOrder{Present: true, Value: uint64(state)})
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := compact.BeginFrontier(); err != nil {
				t.Fatal(err)
			}
			suffix, err := compact.Seed(320, 1)
			if err != nil {
				t.Fatal(err)
			}
			header := diagnosticParserCoreHeader{head: head, creationSeq: 2}
			if recovery {
				header.markRecoveryLineage()
			}
			s := &diagnosticParserCoreGenericScheduler{
				compact: compact, headers: []diagnosticParserCoreHeader{header, {head: suffix, creationSeq: 5}},
				token: Token{Symbol: 10, StartByte: 1, EndByte: 2}, nextSeq: 6, nextCleanPathLineage: 1,
				tokenSource: &dfaTokenSource{language: &Language{TokenCount: 11, SymbolMetadata: make([]SymbolMetadata, 11)}},
				options:     DiagnosticParserCorePrefixOptions{MaxDispatches: 10, materializationSource: []byte("xy")},
				receipt:     &DiagnosticParserCoreGenericScheduler{},
			}
			s.recoveryTurns.active = recovery
			s.recoveryIsolation = recovery
			before, err := diagnosticParserCoreHeaderReceipts(compact, s.headers)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.applyGenericReduction(before, mustDiagnosticParserCoreGenericCell(t, compact, 0, header, 10)); err != nil {
				t.Fatal(err)
			}
			var states []core.StateID
			var sequences []uint64
			for _, h := range s.headers {
				state, _, err := compact.Boundary(h.head)
				if err != nil {
					t.Fatal(err)
				}
				states = append(states, state)
				sequences = append(sequences, h.creationSeq)
			}
			if !reflect.DeepEqual(states, []core.StateID{236, 320, 240}) || !reflect.DeepEqual(sequences, []uint64{2, 5, 6}) {
				t.Fatalf("states=%v sequences=%v, want [236 320 240] and [2 5 6]", states, sequences)
			}
			if s.headers[1].head != suffix || s.nextSeq != 7 {
				t.Fatalf("suffix or allocation changed: headers=%+v next=%d", s.headers, s.nextSeq)
			}
		})
	}
}
