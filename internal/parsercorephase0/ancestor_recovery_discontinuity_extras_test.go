package parsercorephase0

import (
	"reflect"
	"strings"
	"testing"
)

func TestRecoverToAncestorStateDiscontinuityExtras(t *testing.T) {
	for _, test := range []struct {
		name    string
		body    bool
		extras  int
		markers []int // Positions after payloads; zero precedes every payload.
		region  bool
		nodeCap bool
		refusal string
	}{
		{name: "no-marker-control", body: true, extras: 2},
		{name: "top-marker", body: true, extras: 2, markers: []int{3}},
		{name: "marker-among-extras", body: true, extras: 2, markers: []int{2}},
		{name: "multiple-markers", body: true, extras: 2, markers: []int{1, 2, 3, 3}},
		{name: "no-extra", body: true, markers: []int{1}},
		{name: "open-region", body: true, extras: 2, markers: []int{2, 3}, region: true},
		{name: "marker-only-open-region", markers: []int{0}, region: true},
		{name: "marker-only", markers: []int{0}, refusal: "no ERROR payload"},
		{name: "only-extras", extras: 2, markers: []int{0, 2}, refusal: "only trailing extras"},
		{name: "late-node-cap", body: true, extras: 2, markers: []int{3}, nodeCap: true, refusal: "node arena cap"},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := newAncestorRecoveryTestCore(t, &fakeTable{}, Limits{})
			head, err := c.Seed(1, 0)
			if err != nil {
				t.Fatal(err)
			}
			c.mutateTestLineage(int(head.Node-1), func(r *nodeLineageRecord) { r.storedErrorCost = 9 })
			count, depth := test.extras, len(test.markers)
			if test.body {
				count++
				depth++
			}
			var ids []SubtreeID
			for position := 0; position <= count; position++ {
				for _, at := range test.markers {
					if at != position {
						continue
					}
					err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
						var e error
						head, e = c.AppendRecoveryDiscontinuityOwned(owner, head, RecoveryDiscontinuityContext{ByteOffset: uint32(position)})
						return e
					})
					if err != nil {
						t.Fatal(err)
					}
				}
				if position == count {
					break
				}
				state := StateID(2)
				if !test.body || position != 0 {
					state, _, err = c.Boundary(head)
					if err != nil {
						t.Fatal(err)
					}
				}
				id := appendAncestorRecoveryPayload(t, c, Symbol(20+position), uint32(position), uint32(position+1), !test.body || position != 0)
				ids = append(ids, id)
				head = appendAncestorRecoveryHead(t, c, head, state, id)
			}
			candidates, err := c.StackSummaryCandidates(head, StackSummaryMaxDepth)
			if err != nil {
				t.Fatal(err)
			}
			candidate := ancestorRecoveryCandidateForState(t, candidates, 1)
			if candidate.Depth() != depth || int(candidate.linkDepth) != count+len(test.markers) {
				t.Fatalf("physical candidate=%+v want depth=%d links=%d", candidate, depth, count+len(test.markers))
			}
			var region []SubtreeID
			if test.region {
				region = []SubtreeID{appendAncestorRecoveryPayload(t, c, 99, uint32(count), uint32(count+1), false)}
			}
			if test.nodeCap {
				c.limits.MaxNodes = uint32(c.nodes.Len() + 2) // ERROR and first extra fit.
			}
			before := captureSchedulerTransactionState(c)
			calls := 0
			cost := func(prev NodeID, id SubtreeID) (uint32, error) {
				calls++
				prefix, err := c.RecoveryStoredErrorCost(Head{Node: prev})
				if err != nil {
					return 0, err
				}
				p, err := c.Subtree(id)
				if err != nil {
					return 0, err
				}
				if p.Symbol == ErrorRegionSymbol {
					return prefix + 100, nil
				}
				return prefix + 1, nil
			}
			var out Head
			err = c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
				var e error
				if test.region {
					out, e = c.RecoverToAncestorStateWithOpenRegionAndCostOwned(owner, candidate, uint32(count), uint32(count+1), region, cost)
				} else {
					out, e = c.RecoverToAncestorStateWithCostOwned(owner, candidate, cost)
				}
				if test.refusal != "" {
					if e == nil || !strings.Contains(e.Error(), test.refusal) {
						t.Fatalf("refusal=%v want %q", e, test.refusal)
					}
					return nil // An ignored owned failure must still roll back.
				}
				return e
			})
			if test.refusal != "" {
				if err == nil || !strings.Contains(err.Error(), "poisoned scheduler transaction") || !reflect.DeepEqual(before, captureSchedulerTransactionState(c)) {
					t.Fatalf("failed recovery did not restore arenas, boundary index and work: %v", err)
				}
				if test.nodeCap && calls != 3 {
					t.Fatalf("cap failed before the second extra: cost calls=%d", calls)
				}
				assertTransactionJournalClean(t, c)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			paths, err := c.Derivations(out)
			if err != nil || len(paths) != 1 || len(paths[0].Payloads) == 0 {
				t.Fatalf("paths=%+v err=%v", paths, err)
			}
			var children, extras []SubtreeID
			end, boundaryEnd := uint32(1), uint32(count)
			if test.region {
				children = append(append([]SubtreeID(nil), ids...), region...)
				end, boundaryEnd = uint32(count+1), uint32(count+1)
			} else {
				children, extras = ids[:1], ids[1:]
			}
			wantPath := append([]SubtreeID{paths[0].Payloads[0]}, extras...)
			p, err := c.Subtree(paths[0].Payloads[0])
			if err != nil || p.Symbol != ErrorRegionSymbol || !p.Extra || p.StartByte != 0 || p.EndByte != end || !reflect.DeepEqual(p.Children, children) || len(p.Aliases) != 0 || !reflect.DeepEqual(paths[0].Payloads, wantPath) {
				t.Fatalf("ERROR/extra projection: ERROR=%+v paths=%+v want children=%v extras=%v err=%v", p, paths, children, extras, err)
			}
			if canonical, ok := c.CanonicalBoundary(1, boundaryEnd, true, 0); !ok || canonical != out {
				t.Fatalf("last payload did not publish canonical boundary: %+v/%t want %+v", canonical, ok, out)
			}
			if got, err := c.RecoveryStoredErrorCost(out); err != nil || got != uint32(109+len(extras)) || calls != 1+len(extras) {
				t.Fatalf("cost=%d calls=%d err=%v", got, calls, err)
			}
			assertTransactionJournalClean(t, c)
		})
	}
}
