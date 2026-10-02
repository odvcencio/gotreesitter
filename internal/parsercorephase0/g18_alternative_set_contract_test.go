package parsercorephase0

import (
	"encoding/json"
	"testing"
)

type g18FutureHistoricalCertificateTelemetryProvider interface {
	DiagnosticDropCohortArenaIdentityForTest() (uint64, uint64)
	DiagnosticDropCohortSnapshotForTest() []byte
}

type g18FutureHistoricalCertificateTelemetry struct {
	Schema                      string            `json:"schema"`
	ArenaOwner                  uint64            `json:"arena_owner"`
	ArenaEpoch                  uint64            `json:"arena_epoch"`
	ProducerWrites              map[string]uint64 `json:"producer_writes"`
	AuthenticatedHistoryImports uint64            `json:"authenticated_history_imports"`
	UnprovedHistoryImports      uint64            `json:"unproved_history_imports"`
}

var g18HistoricalProducerNames = []string{
	"reduction_establishment",
	"linear_canonicalizer",
	"mapped_canonicalizer",
	"sibling_adoption",
	"conflict_reconciliation",
	"dead_history_import",
}

// TestG18HistoricalAlternativeSetProducerPath exercises the real dead-node
// import. A retired converged boundary must publish its authenticated set on
// the replacement reduction output.
func TestG18HistoricalAlternativeSetProducerPath(t *testing.T) {
	compact, outputs := g18HistoricalAlternativeSetProducerFixture(t, nil, nil)
	members, ok := compact.AlternativeSetMembers(outputs[0].HistoricalAlternativeSet)
	if !ok || len(members) != 3 || outputs[0].HistoricalBlended {
		t.Fatalf(
			"replacement history members=%v valid=%t blended=%t, want three authenticated members",
			members,
			ok,
			outputs[0].HistoricalBlended,
		)
	}
}

func TestG18HistoricalAlternativeSetDefaultOffCompatibility(t *testing.T) {
	compact, outputs := g18HistoricalAlternativeSetProducerFixture(t, nil, nil)
	if len(compact.dropCohortRecords) != 0 {
		t.Fatalf("default-off fixture created %d certificates", len(compact.dropCohortRecords))
	}
	if len(outputs) != 1 || outputs[0].HistoricalBoundaryProvenance != HistoricalBoundaryConverged ||
		outputs[0].HistoricalAlternativeSet.Len() != 3 || !outputs[0].DropCohortRefs.Empty() {
		t.Fatalf("default-off output=%+v, want pre-D2 converged union without refs", outputs)
	}
	if compact.dropCohortAuthenticatedHistory != 1 || compact.dropCohortUnprovedHistory != 0 ||
		compact.dropCohortProducerWrites[dropCohortProducerDeadHistoryImport] != 1 {
		t.Fatalf("default-off telemetry authenticated=%d unproved=%d dead=%d, want 1/0/1",
			compact.dropCohortAuthenticatedHistory,
			compact.dropCohortUnprovedHistory,
			compact.dropCohortProducerWrites[dropCohortProducerDeadHistoryImport],
		)
	}
}

// TestG18HistoricalAlternativeSetCertificateTelemetryRED binds the real
// dead-node import to the future certificate arena. Current main does not
// publish this telemetry.
func TestG18HistoricalAlternativeSetCertificateTelemetryRED(t *testing.T) {
	var provider g18FutureHistoricalCertificateTelemetryProvider
	var before g18FutureHistoricalCertificateTelemetry
	var after g18FutureHistoricalCertificateTelemetry
	g18HistoricalAlternativeSetProducerFixtureAuthenticated(
		t,
		func(compact *Core) {
			var ok bool
			provider, ok = any(compact).(g18FutureHistoricalCertificateTelemetryProvider)
			if !ok {
				t.Fatal("RED: real Core does not implement dead-history behavior telemetry")
			}
			before = g18DecodeHistoricalSnapshot(t, provider)
		},
		func(_ *Core) {
			after = g18DecodeHistoricalSnapshot(t, provider)
		},
	)
	for _, name := range g18HistoricalProducerNames {
		want := uint64(0)
		if name == "dead_history_import" {
			want = 1
		}
		if after.ProducerWrites[name] < before.ProducerWrites[name] {
			t.Fatalf("dead-history producer counter %s decreased", name)
		}
		if got := after.ProducerWrites[name] - before.ProducerWrites[name]; got != want {
			t.Fatalf("dead-history producer delta %s=%d, want %d", name, got, want)
		}
	}
	if after.AuthenticatedHistoryImports < before.AuthenticatedHistoryImports ||
		after.AuthenticatedHistoryImports-before.AuthenticatedHistoryImports != 1 ||
		after.UnprovedHistoryImports != before.UnprovedHistoryImports {
		t.Fatalf(
			"dead-history import counters authenticated=%d/%d unproved=%d/%d, want authenticated +1 and unproved +0",
			before.AuthenticatedHistoryImports,
			after.AuthenticatedHistoryImports,
			before.UnprovedHistoryImports,
			after.UnprovedHistoryImports,
		)
	}
}

func TestG18HistoricalAlternativeSetImportRejectsMissingCertificate(t *testing.T) {
	compact, outputs := g18HistoricalAlternativeSetProducerFixtureWithCertificate(t, nil, nil, false)
	if len(outputs) != 1 || outputs[0].HistoricalBoundaryProvenance != HistoricalBoundaryUnproved ||
		outputs[0].HistoricalAlternativeSet.Len() != 0 || !outputs[0].DropCohortRefs.Empty() {
		t.Fatalf("missing-certificate output=%+v, want unproved history without imported state", outputs)
	}
	if compact.dropCohortAuthenticatedHistory != 0 || compact.dropCohortUnprovedHistory != 1 ||
		compact.dropCohortProducerWrites[dropCohortProducerDeadHistoryImport] != 0 {
		t.Fatalf("missing-certificate telemetry authenticated=%d unproved=%d writes=%d",
			compact.dropCohortAuthenticatedHistory,
			compact.dropCohortUnprovedHistory,
			compact.dropCohortProducerWrites[dropCohortProducerDeadHistoryImport],
		)
	}
}

func TestG18HistoricalAlternativeSetImportRejectsIdentityCorruption(t *testing.T) {
	mutateRef := func(mutate func(*DropCohortRef)) func(*Core) {
		return func(compact *Core) {
			for index := range compact.nodeLineages {
				refID := compact.nodeLineages[index].dropCohortRefs
				if refID == 0 {
					continue
				}
				refs := &compact.nodeDropCohortRefs[refID-1]
				if refs.Empty() || refs.Spilled() {
					continue
				}
				mutate(&refs.Inline[0])
				return
			}
			t.Fatal("certificate fixture has no inline historical reference")
		}
	}
	tests := []struct {
		name   string
		mutate func(*Core)
	}{
		{
			name: "foreign-reference",
			mutate: mutateRef(func(ref *DropCohortRef) {
				ref.Owner ^= 1
			}),
		},
		{
			name: "stale-reference",
			mutate: mutateRef(func(ref *DropCohortRef) {
				ref.Epoch++
			}),
		},
		{
			name: "building",
			mutate: func(compact *Core) {
				compact.dropCohortRecords[0].state = DropCohortBuilding
			},
		},
		{
			name: "terminal",
			mutate: func(compact *Core) {
				compact.dropCohortRecords[0].state = DropCohortOverflowed
			},
		},
		{
			name: "action",
			mutate: func(compact *Core) {
				compact.dropCohortMembers[0].action.Action.Symbol++
			},
		},
		{
			name: "derivation",
			mutate: func(compact *Core) {
				compact.dropCohortDerivationBytes[0] ^= 0xff
			},
		},
		{
			name: "over-budget",
			mutate: func(compact *Core) {
				compact.limits.MaxDropCohortBytes = 1
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compact, outputs := g18HistoricalAlternativeSetProducerFixtureWithCertificate(t, test.mutate, nil, true)
			if len(outputs) != 1 || outputs[0].HistoricalBoundaryProvenance != HistoricalBoundaryUnproved ||
				outputs[0].HistoricalAlternativeSet.Len() != 0 || !outputs[0].DropCohortRefs.Empty() {
				t.Fatalf("corrupt %s output=%+v, want unproved history without imported set", test.name, outputs)
			}
			if compact.dropCohortAuthenticatedHistory != 0 || compact.dropCohortUnprovedHistory != 1 ||
				compact.dropCohortProducerWrites[dropCohortProducerDeadHistoryImport] != 0 {
				t.Fatalf("corrupt %s telemetry authenticated=%d unproved=%d writes=%d", test.name,
					compact.dropCohortAuthenticatedHistory,
					compact.dropCohortUnprovedHistory,
					compact.dropCohortProducerWrites[dropCohortProducerDeadHistoryImport],
				)
			}
		})
	}
}

func g18DecodeHistoricalSnapshot(
	t *testing.T,
	provider g18FutureHistoricalCertificateTelemetryProvider,
) g18FutureHistoricalCertificateTelemetry {
	t.Helper()
	var telemetry g18FutureHistoricalCertificateTelemetry
	if err := json.Unmarshal(provider.DiagnosticDropCohortSnapshotForTest(), &telemetry); err != nil {
		t.Fatalf("decode dead-history certificate snapshot: %v", err)
	}
	owner, epoch := provider.DiagnosticDropCohortArenaIdentityForTest()
	if telemetry.Schema != "gts-drop-cohort-certificate-arena/v2" || owner == 0 || epoch == 0 ||
		telemetry.ArenaOwner != owner || telemetry.ArenaEpoch != epoch {
		t.Fatalf("dead-history certificate snapshot=%+v identity=%d/%d", telemetry, owner, epoch)
	}
	for _, producer := range g18HistoricalProducerNames {
		if _, ok := telemetry.ProducerWrites[producer]; !ok {
			t.Fatalf("dead-history certificate snapshot omits producer counter %q", producer)
		}
	}
	return telemetry
}

func g18HistoricalAlternativeSetProducerFixture(
	t *testing.T,
	beforeReduce func(*Core),
	afterReduce func(*Core),
) (*Core, []ReductionOutput) {
	return g18HistoricalAlternativeSetProducerFixtureWithCertificateAndAuth(t, beforeReduce, afterReduce, false, false)
}

func g18HistoricalAlternativeSetProducerFixtureAuthenticated(
	t *testing.T,
	beforeReduce func(*Core),
	afterReduce func(*Core),
) (*Core, []ReductionOutput) {
	return g18HistoricalAlternativeSetProducerFixtureWithCertificateAndAuth(t, beforeReduce, afterReduce, true, true)
}

func g18HistoricalAlternativeSetProducerFixtureWithCertificate(
	t *testing.T,
	beforeReduce func(*Core),
	afterReduce func(*Core),
	withCertificate bool,
) (*Core, []ReductionOutput) {
	return g18HistoricalAlternativeSetProducerFixtureWithCertificateAndAuth(t, beforeReduce, afterReduce, withCertificate, true)
}

func g18HistoricalAlternativeSetProducerFixtureWithCertificateAndAuth(
	t *testing.T,
	beforeReduce func(*Core),
	afterReduce func(*Core),
	withCertificate bool,
	authenticate bool,
) (*Core, []ReductionOutput) {
	t.Helper()
	tables := &fakeTable{
		actions: map[tableCell][]Action{
			{state: 3, symbol: 9}: {{Type: ActionReduce, Symbol: 2, ChildCount: 1}},
		},
		gotos: map[tableCell]StateID{{state: 1, symbol: 2}: 4},
	}
	compact, err := New(tables, Limits{MaxDerivations: 4, MaxPopPaths: 4, MaxLinksPerBoundary: 2})
	if err != nil {
		t.Fatal(err)
	}
	compact.historicalCertificateAuthentication = authenticate
	compact.diagnostics.foldSamePredecessorShallowPayloads = false
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := compact.appendSubtree(
		subtreeRecord{symbol: 1, terminal: true, endByte: 1}, nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	key := compact.boundaryKey(4, 1)
	retired, err := compact.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: 1})
	if err != nil {
		t.Fatal(err)
	}
	retired, err = compact.condense(key, linkInput{prev: seed.Node, payload: payload, scoreDelta: 2})
	if err != nil {
		t.Fatal(err)
	}
	source, err := compact.appendDiagnosticPayload(
		seed,
		3,
		Token{Symbol: 10, EndByte: 1},
		pathMeta{},
	)
	if err != nil {
		t.Fatal(err)
	}
	classified, err := compact.ClassifyBoundary(source, 9)
	if err != nil {
		t.Fatal(err)
	}

	var outputs []ReductionOutput
	err = compact.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if err := compact.RecordReductionLineageOwned(owner, []ReductionOutput{{
			Head: retired, CleanPathRank: CleanPathRankSelected, MultiplePopPaths: true,
		}}, 7); err != nil {
			return err
		}
		if withCertificate {
			cohort, beginErr := compact.BeginDropCohortOwned(owner, DropCohortActionIdentity{
				BoundaryState: 3,
				Lookahead:     9,
				ActionOrdinal: 0,
				Action:        Action{Type: ActionReduce, Symbol: 2, ChildCount: 1},
			}, 1)
			if beginErr != nil {
				return beginErr
			}
			derivation, derivationErr := compact.BuildDropCohortDerivationOwned(owner, retired, DropCohortSourceCheckpoint{})
			if derivationErr != nil {
				return derivationErr
			}
			if writeErr := compact.WriteDropCohortMemberOwned(owner, cohort, retired, 0, derivation); writeErr != nil {
				return writeErr
			}
			refs, finalizeErr := compact.FinalizeDropCohortOwned(owner, cohort)
			if finalizeErr != nil {
				return finalizeErr
			}
			if err := compact.RecordHeadLineageRefsOwned(owner, retired, refs); err != nil {
				return err
			}
		}
		set := NewAlternativeSetMember(7, 0)
		compact.UnionAlternativeSet(&set, NewAlternativeSetMember(7, 1))
		compact.UnionAlternativeSet(&set, NewAlternativeSetMember(7, 2))
		if err := compact.RecordHeadLineageSetOwned(owner, retired, set, false); err != nil {
			return err
		}
		if beforeReduce != nil {
			beforeReduce(compact)
		}
		var reduceErr error
		outputs, reduceErr = compact.ReduceOutputsClassifiedIntoWithLiveCondenseCandidatesOwned(
			owner,
			nil,
			nil,
			classified,
			0,
			ForkOrder{},
		)
		if reduceErr == nil && afterReduce != nil {
			afterReduce(compact)
		}
		return reduceErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(outputs) != 1 {
		t.Fatalf("replacement outputs=%+v, want one historical boundary", outputs)
	}
	return compact, outputs
}
