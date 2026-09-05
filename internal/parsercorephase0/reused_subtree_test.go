package parsercorephase0

import (
	"errors"
	"math"
	"testing"
	"unsafe"
)

func reusedFixture(t *testing.T) (*Core, Head, ReusedSubtree) {
	t.Helper()
	c, err := New(&fakeTable{gotos: map[tableCell]StateID{{state: 1, symbol: 100}: 2}}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	head, err := c.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	return c, head, ReusedSubtree{Key: 1, Symbol: 100, PreGotoState: 1, State: 2, StartByte: 2, EndByte: 10, DynamicPrecedence: math.MaxInt32}
}

func TestReusedSubtreePublicationAndWidePrecedence(t *testing.T) {
	for _, precedence := range []int32{math.MinInt32, math.MaxInt32} {
		c, head, reused := reusedFixture(t)
		reused.DynamicPrecedence = precedence
		var out Head
		var payload SubtreeID
		err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) (err error) {
			out, payload, err = c.PushReusedSubtreeOwned(owner, head, reused)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		state, end, err := c.Boundary(out)
		if err != nil || state != 2 || end != 10 {
			t.Fatalf("boundary = %d, %d, %v", state, end, err)
		}
		paths, err := c.Derivations(out)
		if err != nil || len(paths) != 1 || paths[0].Score != int64(precedence) {
			t.Fatalf("paths = %+v, %v", paths, err)
		}
		if c.nodes[out.Node-1].precedenceMax != int64(precedence) {
			t.Fatal("borrowed precedence was discarded as a leaf")
		}
		view, err := c.MaterializationView(payload)
		if err != nil || view.ReusedKey != reused.Key || view.Terminal || len(view.Children) != 0 || view.DynamicPrecedence != precedence || view.ReusedState != 2 || view.ReusedPreGotoState != 1 {
			t.Fatalf("view = %+v, %v", view, err)
		}
		replay, err := c.MaterializationReplayView(payload)
		if err != nil || replay.ReusedKey != reused.Key || replay.Terminal || replay.ReusedState != 2 {
			t.Fatalf("replay = %+v, %v", replay, err)
		}
		visits := 0
		err = c.VisitMaterializationPostorder([]SubtreeID{payload}, nil, func(_ SubtreeID, view MaterializationSubtreeView) error {
			visits++
			if view.ReusedKey != reused.Key || view.DynamicPrecedence != precedence || view.Terminal {
				t.Fatal("postorder lost borrowed metadata")
			}
			return nil
		})
		if err != nil || visits != 1 {
			t.Fatalf("visits = %d, %v", visits, err)
		}
		if _, err := c.MaterializationOrder([]SubtreeID{payload, payload}, nil); err == nil {
			t.Fatal("repeated ownership was accepted")
		}
		if _, err := c.BuildSelectedStore([]SubtreeID{payload}, SelectedStorePolicy{}, nil, nil); err == nil {
			t.Fatal("selected store expanded opaque payload")
		}
		if unsafe.Sizeof(subtreeRecord{}) != 44 {
			t.Fatal("subtree record grew")
		}
	}
}

func TestReusedSubtreeInvalidAdmission(t *testing.T) {
	cases := []struct {
		name  string
		alter func(*Core, Head, *ReusedSubtree)
	}{
		{"zero key", func(_ *Core, _ Head, r *ReusedSubtree) { r.Key = 0 }},
		{"wrong state", func(_ *Core, _ Head, r *ReusedSubtree) { r.PreGotoState = 3 }},
		{"wrong goto", func(_ *Core, _ Head, r *ReusedSubtree) { r.State = 3 }},
		{"zero width", func(_ *Core, _ Head, r *ReusedSubtree) { r.EndByte = r.StartByte }},
		{"before head", func(c *Core, h Head, _ *ReusedSubtree) { c.nodes[h.Node-1].byteOffset = 3 }},
		{"ambiguous", func(c *Core, h Head, _ *ReusedSubtree) { c.nodes[h.Node-1].pathCount = 2 }},
		{"recovery", func(c *Core, h Head, _ *ReusedSubtree) { c.nodeLineages[h.Node-1].storedErrorCost = 1 }},
		{"conflict", func(c *Core, _ Head, _ *ReusedSubtree) { c.reduceConflictContext = true }},
		{"unauthenticated", func(c *Core, _ Head, _ *ReusedSubtree) { c.metadataConstructionAuthenticated = false }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			c, head, reused := reusedFixture(t)
			test.alter(c, head, &reused)
			err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				_, _, err := c.PushReusedSubtreeOwned(owner, head, reused)
				return err
			})
			if err == nil || len(c.reusedSubtrees) != 0 || len(c.subtrees) != 0 {
				t.Fatalf("invalid admission committed: %v", err)
			}
		})
	}
}

func TestReusedSubtreeRollbackResetAndOwner(t *testing.T) {
	c, head, reused := reusedFixture(t)
	before := c.StorageBytes()
	stop := errors.New("stop")
	var stale SchedulerTransactionToken
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		stale = owner
		if _, _, err := c.PushReusedSubtreeOwned(owner, head, reused); err != nil {
			return err
		}
		if c.StorageBytes() < before+coreReusedSubtreeBytes || c.FootprintBytes() < coreReusedSubtreeBytes {
			t.Fatal("borrowed sidecar omitted from memory accounting")
		}
		return stop
	})
	if !errors.Is(err, stop) || len(c.reusedSubtrees) != 0 || c.StorageBytes() != before {
		t.Fatalf("rollback failed: %v", err)
	}
	if _, _, err := c.PushReusedSubtreeOwned(stale, head, reused); err == nil {
		t.Fatal("stale owner accepted")
	}
	err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		_, _, err := c.PushReusedSubtreeOwned(owner, head, reused)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Reset(); err != nil || len(c.reusedSubtrees) != 0 {
		t.Fatalf("reset failed: %v", err)
	}
	if cap(c.reusedSubtrees) == 0 {
		t.Fatal("ordinary reset dropped bounded sidecar retention")
	}
	if err := c.ResetReleasingRetention(); err != nil || c.reusedSubtrees != nil {
		t.Fatalf("release failed: %v", err)
	}
}

func TestReusedSubtreeOpaqueComparisonsAndAuthentication(t *testing.T) {
	c, head, reused := reusedFixture(t)
	var first, same, other SubtreeID
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) (err error) {
		if _, first, err = c.PushReusedSubtreeOwned(owner, head, reused); err != nil {
			return err
		}
		reused.Key++
		_, other, err = c.PushReusedSubtreeOwned(owner, head, reused)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// Corrupt publication to exercise duplicate public ownership independently of ordered-key admission.
	same = SubtreeID(len(c.subtrees) + 1)
	c.subtrees = append(c.subtrees, c.subtrees[first-1])
	descriptor, _ := c.reusedSubtree(first)
	c.reusedSubtrees = append(c.reusedSubtrees, reusedSubtreeProvenance{payload: same, descriptor: descriptor})
	if equal, err := c.subtreesStructurallyEqual(first, same); err != nil || !equal {
		t.Fatalf("same identity differs: %v", err)
	}
	if equal, err := c.subtreesStructurallyEqual(first, other); err != nil || equal {
		t.Fatalf("different identities equal: %v", err)
	}
	if comparison, err := c.CompareCSelectionSubtrees(first, same); err != nil || comparison != 0 {
		t.Fatalf("same selection identity differs: %v", err)
	}
	if _, err := c.CompareCSelectionSubtrees(first, other); err == nil {
		t.Fatal("opaque C comparison guessed child count")
	}
	if _, err := c.dropCohortCompareSubtree(first, other); err == nil {
		t.Fatal("drop-cohort comparison guessed opaque identity")
	}
	if _, err := c.MaterializationOrder([]SubtreeID{first, same}, nil); err == nil {
		t.Fatal("distinct payloads claimed the same public node")
	}
	if err := c.VisitMaterializationPostorder([]SubtreeID{first, same}, nil, func(SubtreeID, MaterializationSubtreeView) error { return nil }); err == nil {
		t.Fatal("postorder accepted repeated borrowed ownership")
	}
	if _, err := c.RawSelectedSubtreeCensus([]SubtreeID{first}); err == nil {
		t.Fatal("raw census counted opaque content as a leaf")
	}
	if equal, err := c.shallowPayloadsEqual(head.Node, first, head.Node, other); err != nil || equal {
		t.Fatalf("opaque shallow class compared equal: %v", err)
	}
	c.reusedSubtrees = nil
	if _, err := c.MaterializationView(first); err == nil {
		t.Fatal("missing borrowed provenance was accepted")
	}
	if _, err := c.MaterializationOrder([]SubtreeID{first}, nil); err == nil {
		t.Fatal("missing provenance was treated as an empty reduction")
	}
}

func TestReusedSubtreeReductionPreservesWideContribution(t *testing.T) {
	c, head, reused := reusedFixture(t)
	tables := c.tables.(*fakeTable)
	tables.actions = map[tableCell][]Action{{state: 2, symbol: 0}: {{Type: ActionReduce, Symbol: 101, ChildCount: 1, DynamicPrecedence: -7}}}
	tables.gotos[tableCell{state: 1, symbol: 101}] = 3
	var borrowed SubtreeID
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) (err error) {
		head, borrowed, err = c.PushReusedSubtreeOwned(owner, head, reused)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	heads, err := c.Reduce(head, 0, 0, ForkOrder{})
	if err != nil || len(heads) != 1 {
		t.Fatalf("reduce = %v, %v", heads, err)
	}
	paths, err := c.Derivations(heads[0])
	if err != nil || len(paths) != 1 || paths[0].Score != int64(reused.DynamicPrecedence)-7 {
		t.Fatalf("reduced paths = %+v, %v", paths, err)
	}
	view, err := c.MaterializationView(paths[0].Payloads[0])
	if err != nil || len(view.Children) != 1 || view.Children[0] != borrowed || view.ReusedKey != 0 {
		t.Fatalf("parent = %+v, %v", view, err)
	}
	order, err := c.MaterializationOrder(paths[0].Payloads, nil)
	if err != nil || len(order) != 2 || order[0] != borrowed {
		t.Fatalf("materialization order = %v, %v", order, err)
	}
}

func TestReusedSubtreeForeignOwnerPoisonsTransaction(t *testing.T) {
	c, head, reused := reusedFixture(t)
	foreign, _, _ := reusedFixture(t)
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		owner.owner = foreign
		_, _, _ = c.PushReusedSubtreeOwned(owner, head, reused)
		return nil
	})
	if err == nil || len(c.subtrees) != 0 || len(c.reusedSubtrees) != 0 {
		t.Fatal("foreign owner was accepted")
	}
}

func TestReusedSubtreeCleanExternalAncestorRequiresQuiescence(t *testing.T) {
	for _, test := range []struct {
		name                        string
		certified, fragile, missing bool
		wantSuccess                 bool
	}{
		{name: "certified", certified: true, wantSuccess: true},
		{name: "uncertified"},
		{name: "fragile", certified: true, fragile: true},
		{name: "missing", certified: true, missing: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, head, reused := reusedFixture(t)
			if test.certified {
				c.CertifyExternalPayloadsQuiescent()
			}
			c.tables.(*fakeTable).actions = map[tableCell][]Action{
				{state: 1, symbol: 1}: {{Type: ActionShift, State: 1}},
			}
			var err error
			head, err = c.Shift(head, 1, 0, Token{Symbol: 1, EndByte: 1, External: true}, ForkOrder{})
			if err != nil {
				t.Fatal(err)
			}
			c.subtrees[0].fragile, c.subtrees[0].missing = test.fragile, test.missing
			err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				_, _, err := c.PushReusedSubtreeOwned(owner, head, reused)
				return err
			})
			if (err == nil) != test.wantSuccess {
				t.Fatalf("reuse success = %t, want %t: %v", err == nil, test.wantSuccess, err)
			}
		})
	}
}

func TestReusedSubtreeDoesNotCertifyHiddenScannerProvenance(t *testing.T) {
	c, head, reused := reusedFixture(t)
	var payload SubtreeID
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) (err error) {
		_, payload, err = c.PushReusedSubtreeOwned(owner, head, reused)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	hasExternal, exact, err := c.subtreeExternalProvenance(payload)
	if err != nil || !hasExternal || exact {
		t.Fatalf("opaque scanner proof = %t, %t, %v", hasExternal, exact, err)
	}
	view, err := c.MaterializationView(payload)
	if err != nil || view.ExternalScannerCheckpointExact || view.ExternalScannerCheckpointStart != 0 || view.ExternalScannerCheckpointEnd != 0 {
		t.Fatalf("borrowed checkpoint transfer = %+v, %v", view, err)
	}
	if _, err := c.subtreeScannerStatePairsEqual(payload, payload); err == nil {
		t.Fatal("borrowed scanner state was assumed empty")
	}
	c.CertifyExternalPayloadsQuiescent()
	if _, exact, err := c.subtreeExternalProvenance(payload); err != nil || !exact {
		t.Fatalf("language quiescence certificate was ignored: %v", err)
	}
	if _, exact, _ := c.subtrees[payload-1].externalProvenanceState.result(); exact {
		t.Fatal("language certificate rewrote opaque checkpoint provenance")
	}
}
