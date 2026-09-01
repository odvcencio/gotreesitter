package parsercorephase0

import (
	"errors"
	"reflect"
	"testing"
)

type recoverEOFAcceptFixture struct {
	core    *Core
	head    Head
	payload []SubtreeID
}

func newRecoverEOFAcceptFixture(t *testing.T) recoverEOFAcceptFixture {
	t.Helper()
	compact, err := New(&fakeTable{}, (Limits{}).withDefaults())
	if err != nil {
		t.Fatal(err)
	}
	compact.CertifyExternalPayloadsQuiescent()
	head, err := compact.Seed(7, 0)
	if err != nil {
		t.Fatal(err)
	}
	var payloads []SubtreeID
	for index, symbol := range []Symbol{11, 12} {
		payload, err := compact.appendSubtree(subtreeRecord{
			symbol: symbol, startByte: uint32(index), endByte: uint32(index + 1), terminal: true,
		}, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		payloads = append(payloads, payload)
		head, err = compact.appendPrivate(8+StateID(index), uint32(index+1), linkInput{
			prev: head.Node, payload: payload,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return recoverEOFAcceptFixture{core: compact, head: head, payload: payloads}
}

func TestRecoverEOFAcceptOwnedPublishesExactNonExtraRoot(t *testing.T) {
	fixture := newRecoverEOFAcceptFixture(t)
	storageBefore, footprintBefore := fixture.core.StorageBytes(), fixture.core.FootprintBytes()
	var recovered Head
	var root SubtreeID
	if err := fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var err error
		recovered, root, err = fixture.core.RecoverEOFAcceptOwned(
			owner, fixture.head, fixture.payload, 0, 2,
		)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	view, err := fixture.core.Subtree(root)
	if err != nil {
		t.Fatal(err)
	}
	if view.Symbol != ErrorRegionSymbol || view.Extra || view.Terminal || view.StartByte != 0 || view.EndByte != 2 {
		t.Fatalf("recover_eof root=%+v, want non-extra non-terminal ERROR 0..2", view)
	}
	if !reflect.DeepEqual(view.Children, fixture.payload) {
		t.Fatalf("recover_eof children=%v, want %v", view.Children, fixture.payload)
	}
	paths, err := fixture.core.Derivations(recovered)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || !reflect.DeepEqual(paths[0].Payloads, []SubtreeID{root}) {
		t.Fatalf("recovered derivations=%+v, want one root payload %d", paths, root)
	}
	var visited []SubtreeID
	if err := fixture.core.VisitMaterializationPostorder([]SubtreeID{root}, nil, func(id SubtreeID, _ MaterializationSubtreeView) error {
		visited = append(visited, id)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	wantVisited := []SubtreeID{fixture.payload[0], fixture.payload[1], root}
	if !reflect.DeepEqual(visited, wantVisited) {
		t.Fatalf("materialization order=%v, want %v", visited, wantVisited)
	}
	wantStorageDelta := coreSubtreeRecordBytes + uint64(len(fixture.payload))*coreChildRecordBytes +
		2*coreNodeRecordBytes + coreLinkRecordBytes + coreSubtreeIDBytes
	if got := fixture.core.StorageBytes() - storageBefore; got != wantStorageDelta {
		t.Fatalf("recover_eof storage delta=%d, want %d", got, wantStorageDelta)
	}
	if got := fixture.core.FootprintBytes(); got < footprintBefore+wantStorageDelta || got < fixture.core.StorageBytes() {
		t.Fatalf("recover_eof footprint=%d before=%d storage=%d", got, footprintBefore, fixture.core.StorageBytes())
	}
	if err := fixture.core.ResetReleasingRetention(); err != nil {
		t.Fatal(err)
	}
	if cap(fixture.core.eofRecoveryRoots) != 0 {
		t.Fatalf("recover_eof reset retained root sidecar capacity %d", cap(fixture.core.eofRecoveryRoots))
	}
}

func TestRecoverEOFAcceptOwnedRejectsForeignOwnerWithoutMutation(t *testing.T) {
	fixture := newRecoverEOFAcceptFixture(t)
	foreign := newRecoverEOFAcceptFixture(t)
	before, err := fixture.core.Stats(fixture.head)
	if err != nil {
		t.Fatal(err)
	}
	var ownerErr error
	if err := foreign.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, _, ownerErr = fixture.core.RecoverEOFAcceptOwned(owner, fixture.head, fixture.payload, 0, 2)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if ownerErr == nil {
		t.Fatal("foreign scheduler owner unexpectedly published recover_eof root")
	}
	after, err := fixture.core.Stats(fixture.head)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("foreign-owner mutation changed stats from %+v to %+v", before, after)
	}
}

func TestRecoverEOFAcceptOwnedRollsBackPartialPublication(t *testing.T) {
	fixture := newRecoverEOFAcceptFixture(t)
	before, err := fixture.core.Stats(fixture.head)
	if err != nil {
		t.Fatal(err)
	}
	beforeSubtrees := fixture.core.SubtreeArenaLen()
	fixture.core.limits.MaxNodes = uint32(len(fixture.core.nodes))
	err = fixture.core.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, _, operationErr := fixture.core.RecoverEOFAcceptOwned(owner, fixture.head, fixture.payload, 0, 2)
		if operationErr == nil {
			return errors.New("recover_eof operation unexpectedly succeeded at node cap")
		}
		return operationErr
	})
	if err == nil {
		t.Fatal("node-cap recover_eof operation unexpectedly committed")
	}
	after, err := fixture.core.Stats(fixture.head)
	if err != nil {
		t.Fatal(err)
	}
	if after != before || fixture.core.SubtreeArenaLen() != beforeSubtrees || len(fixture.core.eofRecoveryRoots) != 0 {
		t.Fatalf("rollback retained recover_eof state: before=%+v after=%+v subtrees=%d roots=%d", before, after, fixture.core.SubtreeArenaLen(), len(fixture.core.eofRecoveryRoots))
	}
}

func TestRecoverEOFAcceptOwnedPreservesExternalPayloadFlag(t *testing.T) {
	compact, err := New(&fakeTable{}, (Limits{}).withDefaults())
	if err != nil {
		t.Fatal(err)
	}
	compact.CertifyExternalPayloadsQuiescent()
	head, err := compact.Seed(7, 0)
	if err != nil {
		t.Fatal(err)
	}
	external, err := compact.appendSubtree(subtreeRecord{
		symbol: 13, startByte: 0, endByte: 1, terminal: true, external: true,
	}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	head, err = compact.appendPrivate(9, 1, linkInput{prev: head.Node, payload: external})
	if err != nil {
		t.Fatal(err)
	}
	var root SubtreeID
	if err := compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		var operationErr error
		_, root, operationErr = compact.RecoverEOFAcceptOwned(
			owner, head, []SubtreeID{external}, 0, 1,
		)
		return operationErr
	}); err != nil {
		t.Fatal(err)
	}
	view, err := compact.Subtree(root)
	if err != nil {
		t.Fatal(err)
	}
	child, err := compact.Subtree(view.Children[0])
	if err != nil {
		t.Fatal(err)
	}
	if !child.External {
		t.Fatalf("recover_eof external payload flag was cleared: child=%+v", child)
	}
}

func TestRecoverEOFAcceptOwnedRejectsInexactExternalPayload(t *testing.T) {
	compact, err := New(&fakeTable{}, (Limits{}).withDefaults())
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.Seed(7, 0)
	if err != nil {
		t.Fatal(err)
	}
	external, err := compact.appendSubtree(subtreeRecord{
		symbol: 13, startByte: 0, endByte: 1, terminal: true, external: true,
	}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	head, err = compact.appendPrivate(9, 1, linkInput{prev: head.Node, payload: external})
	if err != nil {
		t.Fatal(err)
	}
	before, err := compact.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	beforeSubtrees := compact.SubtreeArenaLen()
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, _, operationErr := compact.RecoverEOFAcceptOwned(owner, head, []SubtreeID{external}, 0, 1)
		return operationErr
	})
	if err == nil {
		t.Fatal("inexact external payload unexpectedly published recover_eof root")
	}
	after, statsErr := compact.Stats(head)
	if statsErr != nil {
		t.Fatal(statsErr)
	}
	if after != before || compact.SubtreeArenaLen() != beforeSubtrees || len(compact.eofRecoveryRoots) != 0 {
		t.Fatalf("inexact external rejection mutated Core: before=%+v after=%+v subtrees=%d roots=%d", before, after, compact.SubtreeArenaLen(), len(compact.eofRecoveryRoots))
	}
}
