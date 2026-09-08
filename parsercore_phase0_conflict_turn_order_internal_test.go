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
			for _, rollback := range []bool{false, true} {
				table := &genericConflictTable{cells: map[genericConflictCell][]core.Action{{state: 3, symbol: 10}: tc.actions}, gotos: map[genericConflictCell]core.StateID{{state: 1, symbol: 7}: 4, {state: 1, symbol: 8}: 6}}
				compact, err := core.New(table, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
				if err != nil {
					t.Fatal(err)
				}
				seed, err := compact.Seed(1, 0)
				if err != nil {
					t.Fatal(err)
				}
				head, err := compact.ShiftMissingLeaf(seed, 3, 5, 0)
				if err != nil {
					t.Fatal(err)
				}
				header := diagnosticParserCoreHeader{head: head, creationSeq: 1}
				header.markRecoveryLineage()
				s := &diagnosticParserCoreGenericScheduler{compact: compact, headers: []diagnosticParserCoreHeader{header}, token: Token{Symbol: 10, EndByte: 1}, tokenSource: &dfaTokenSource{language: &Language{TokenCount: 11, SymbolMetadata: []SymbolMetadata{{}, {}, {}, {}, {}, {Visible: true}}}}, branchOrder: 1, nextSeq: 2, options: DiagnosticParserCorePrefixOptions{MaxDispatches: 10, materializationSource: []byte("x")}, receipt: &DiagnosticParserCoreGenericScheduler{}}
				s.recoveryTurns.active = true
				s.recoveryIsolation = true
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
					t.Fatalf("states=%v sequences=%v; want %v %v", states, sequences, tc.states, tc.sequences)
				}
			}
		})
	}
}
