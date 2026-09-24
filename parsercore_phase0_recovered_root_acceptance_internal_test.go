//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"slices"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func newRecoveredRootMergedScheduler(t *testing.T, maxDerivations uint64) *diagnosticParserCoreGenericScheduler {
	t.Helper()
	table := &genericConflictTable{cells: map[genericConflictCell][]core.Action{
		{state: 2, symbol: 5}:  {{Type: core.ActionShift, State: 3}},
		{state: 2, symbol: 6}:  {{Type: core.ActionShift, State: 3}},
		{state: 2, symbol: 7}:  {{Type: core.ActionShift, State: 3}},
		{state: 2, symbol: 8}:  {{Type: core.ActionShift, State: 3}},
		{state: 2, symbol: 9}:  {{Type: core.ActionShift, State: 3}},
		{state: 2, symbol: 10}: {{Type: core.ActionShift, State: 3}},
	}}
	compact, err := core.New(table, core.Limits{MaxDerivations: maxDerivations, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	child, err := compact.ErrorRegionLeaf(4, 0, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := compact.ErrorRegionResume(seed, 2, 0, 1, []core.SubtreeID{child})
	if err != nil {
		t.Fatal(err)
	}
	first, err := compact.Shift(recovered, 5, 0, core.Token{Symbol: 5, StartByte: 1, EndByte: 2}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	if err := compact.BeginFrontier(); err != nil {
		t.Fatal(err)
	}
	second, err := compact.Shift(recovered, 6, 0, core.Token{Symbol: 6, StartByte: 1, EndByte: 2}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	if canonical, ok := compact.CanonicalBoundary(3, 2, true, 0); !ok || canonical != second {
		t.Fatalf("canonical shifted boundary=%+v/%t, want %+v", canonical, ok, second)
	}
	var merged core.Head
	err = compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		var mergeErr error
		merged, mergeErr = compact.MergeEquivalentRecoveryHeadsOwned(
			owner, 3, 2, 0, true, second, first,
			func(core.SubtreeID) (bool, error) { return true, nil },
		)
		return mergeErr
	})
	if err != nil {
		t.Fatalf("merge recovery heads: %v", err)
	}
	lang := &Language{SymbolMetadata: make([]SymbolMetadata, 16)}
	for index := range lang.SymbolMetadata {
		lang.SymbolMetadata[index] = SymbolMetadata{Visible: true, Named: true}
	}
	s := &diagnosticParserCoreGenericScheduler{
		compact:     compact,
		tokenSource: &dfaTokenSource{language: lang},
		options: DiagnosticParserCorePrefixOptions{
			allowCompactRecoveryLineageSelection: true,
			materializationSource:                []byte("ab"),
			ReceiptMode:                          DiagnosticParserCoreReceiptSummary,
		},
		s3RegionOpened: true,
		token:          Token{StartByte: 2, EndByte: 2},
		work:           DiagnosticParserCoreGenericWork{Accepts: 1},
	}
	s.headers = []diagnosticParserCoreHeader{{head: merged, accepted: true}}
	s.headers[0].markRecoveryLineage()
	s.headers[0].publishRecoveryCondenseState(11, 0, 0, true)
	s.receipt = &s.receiptBacking
	return s
}

func newRecoveredRootMaterialHead(t *testing.T, s *diagnosticParserCoreGenericScheduler, symbol core.Symbol) core.Head {
	t.Helper()
	compact := s.compact
	if err := compact.BeginFrontier(); err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	child, err := compact.ErrorRegionLeaf(4, 0, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := compact.ErrorRegionResume(seed, 2, 0, 1, []core.SubtreeID{child})
	if err != nil {
		t.Fatal(err)
	}
	incoming, err := compact.Shift(recovered, symbol, 0,
		core.Token{Symbol: symbol, StartByte: 1, EndByte: 2}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	return incoming
}

func appendRecoveredRootMaterialPath(t *testing.T, s *diagnosticParserCoreGenericScheduler, symbol core.Symbol) {
	t.Helper()
	compact := s.compact
	incoming := newRecoveredRootMaterialHead(t, s, symbol)
	var merged core.Head
	err := compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		var mergeErr error
		merged, mergeErr = compact.MergeEquivalentRecoveryHeadsOwned(
			owner, 3, 2, 0, true, incoming, s.headers[0].head,
			func(core.SubtreeID) (bool, error) { return true, nil },
		)
		return mergeErr
	})
	if err != nil {
		t.Fatal(err)
	}
	s.headers[0].head = merged
}

func TestRecoveredRootAcceptanceRetainsForkArmsAcrossHeaders(t *testing.T) {
	s := newRecoveredRootMergedScheduler(t, 8)
	second := newRecoveredRootMaterialHead(t, s, 7)
	secondPaths, err := s.compact.Derivations(second)
	if err != nil || len(secondPaths) != 1 {
		t.Fatalf("second grammar arm=%+v error=%v", secondPaths, err)
	}
	s.headers = append(s.headers, diagnosticParserCoreHeader{head: second, accepted: true})
	s.headers[1].markRecoveryLineage()
	s.headers[1].publishRecoveryCondenseState(11, 0, 0, true)
	s.recoveryIsolation = true
	s.work.Accepts = 2
	if err := s.completeAcceptance(); err != nil {
		t.Fatal(err)
	}
	if s.receipt.Acceptance == nil || !s.receipt.Acceptance.RecoveredElectionCertified ||
		s.work.RecoveryLineageSelections != 1 || s.work.RecoveryRootSelections != 1 ||
		!slices.Equal(s.acceptedPayloads, secondPaths[0].Payloads) || len(s.headers) != 1 ||
		s.headers[0].isRecoveryLineage() {
		t.Fatalf("recovered fork arms did not reach one root election: work=%+v payloads=%v headers=%+v",
			s.work, s.acceptedPayloads, s.headers)
	}
}

func TestRecoveredRootAcceptanceRejectsDifferentCostsInsideOneGroup(t *testing.T) {
	s := newRecoveredRootMergedScheduler(t, 8)
	if err := s.compact.BeginFrontier(); err != nil {
		t.Fatal(err)
	}
	seed, err := s.compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	missing, err := s.compact.ShiftMissingLeaf(seed, 3, 7, 2)
	if err != nil {
		t.Fatal(err)
	}
	s.headers = append(s.headers, diagnosticParserCoreHeader{head: missing, accepted: true})
	s.headers[1].markRecoveryLineage()
	s.headers[1].publishRecoveryCondenseState(11, 0, 0, true)
	s.recoveryIsolation = true
	s.work.Accepts = 2
	before := append([]diagnosticParserCoreHeader(nil), s.headers...)
	err = s.completeAcceptance()
	if err == nil && s.receipt.Stop.Boundary == "" {
		t.Fatal("a recovery group with different arm costs reached acceptance")
	}
	if len(s.headers) != 2 || s.headers[0].head != before[0].head ||
		s.headers[1].head != before[1].head || s.work.RecoveryRootSelections != 0 ||
		s.receipt.Acceptance != nil {
		t.Fatalf("invalid group changed the frontier: work=%+v headers=%+v",
			s.work, s.headers)
	}
}

func TestRecoveredRootAcceptanceSixMergedPaths(t *testing.T) {
	s := newRecoveredRootMergedScheduler(t, 8)
	s.options.ReceiptMode = DiagnosticParserCoreReceiptFull
	for _, symbol := range []core.Symbol{7, 8, 9, 10} {
		appendRecoveredRootMaterialPath(t, s, symbol)
	}
	paths, err := s.compact.Derivations(s.headers[0].head)
	if err != nil || len(paths) != 6 {
		t.Fatalf("merged absorber paths=%d error=%v, want six", len(paths), err)
	}
	regions := make(map[core.SubtreeID]bool)
	for _, path := range paths {
		regions[path.Payloads[0]] = true
	}
	if len(regions) != 5 {
		t.Fatalf("merged paths retained %d distinct ERROR regions, want five", len(regions))
	}
	if work := s.compact.Work(); work.PhysicalHeadMergeSuccesses != 5 {
		t.Fatalf("physical recovery merges=%d, want five", work.PhysicalHeadMergeSuccesses)
	}
	if err := s.completeAcceptance(); err != nil {
		t.Fatal(err)
	}
	if s.receipt.Acceptance == nil || s.work.RecoveryRootSelections != 1 ||
		!slices.Equal(s.acceptedPayloads, paths[len(paths)-1].Payloads) {
		t.Fatalf("six-path C fold failed: work=%+v payloads=%v", s.work, s.acceptedPayloads)
	}
	if got, want := s.receipt.Acceptance.Payloads, paths[len(paths)-1].Payloads; len(got) != len(want) {
		t.Fatalf("receipt payload count=%d, want %d", len(got), len(want))
	} else {
		for index := range want {
			if got[index] != uint32(want[index]) {
				t.Fatalf("receipt payload %d=%d, want %d", index, got[index], want[index])
			}
		}
	}
}

func TestRecoveredRootAcceptanceResolvesTwoForkedGroupsOnce(t *testing.T) {
	s := newRecoveredRootMergedScheduler(t, 8)
	compact := s.compact
	if err := compact.BeginFrontier(); err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	child, err := compact.ErrorRegionLeaf(4, 0, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := compact.ErrorRegionResume(seed, 2, 0, 1, []core.SubtreeID{child})
	if err != nil {
		t.Fatal(err)
	}
	first, err := compact.Shift(recovered, 7, 0, core.Token{Symbol: 7, StartByte: 1, EndByte: 2}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	if err := compact.BeginFrontier(); err != nil {
		t.Fatal(err)
	}
	second, err := compact.Shift(recovered, 8, 0, core.Token{Symbol: 8, StartByte: 1, EndByte: 2}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	var merged core.Head
	err = compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		var mergeErr error
		merged, mergeErr = compact.MergeEquivalentRecoveryHeadsOwned(
			owner, 3, 2, 0, true, second, first,
			func(core.SubtreeID) (bool, error) { return true, nil },
		)
		return mergeErr
	})
	if err != nil {
		t.Fatal(err)
	}
	paths, err := compact.Derivations(merged)
	if err != nil || len(paths) != 2 {
		t.Fatalf("second recovery group paths=%+v error=%v", paths, err)
	}
	s.headers = append(s.headers, diagnosticParserCoreHeader{head: merged, accepted: true})
	s.headers[1].markRecoveryLineage()
	s.headers[1].publishRecoveryCondenseState(12, 0, 0, true)
	s.recoveryIsolation = true
	s.work.Accepts = 2
	if err := s.completeAcceptance(); err != nil {
		t.Fatal(err)
	}
	if s.receipt.Acceptance == nil || s.work.RecoveryLineageSelections != 1 ||
		s.work.RecoveryRootSelections != 1 || len(s.headers) != 1 ||
		!slices.Equal(s.acceptedPayloads, paths[1].Payloads) {
		t.Fatalf("two-group recovery election failed: work=%+v headers=%+v payloads=%v",
			s.work, s.headers, s.acceptedPayloads)
	}
	if tail := s.headers[:cap(s.headers)][1]; tail.head.Node != 0 || tail.versionState != nil {
		t.Fatalf("losing group retained a header reference: %+v", tail)
	}
}

func TestRecoveredRootAcceptanceElectsEveryMergedPathOnce(t *testing.T) {
	s := newRecoveredRootMergedScheduler(t, 8)
	paths, err := s.compact.Derivations(s.headers[0].head)
	if err != nil || len(paths) != 2 {
		t.Fatalf("merged paths=%+v error=%v, want two", paths, err)
	}
	if paths[0].Payloads[0] != paths[1].Payloads[0] ||
		paths[0].Payloads[1] == paths[1].Payloads[1] {
		t.Fatalf("recovered fork did not retain both grammar arms: %+v", paths)
	}
	if err := s.completeAcceptance(); err != nil {
		t.Fatalf("complete merged acceptance: %v", err)
	}
	if s.receipt.Acceptance == nil || !s.receipt.Acceptance.RecoveredElectionCertified ||
		s.work.RecoveryRootSelections != 1 || s.headers[0].isRecoveryLineage() {
		t.Fatalf("recovery election or cleanup failed: work=%+v receipt=%+v header=%+v",
			s.work, s.receipt.Acceptance, s.headers[0])
	}
	if !slices.Equal(s.acceptedPayloads, paths[1].Payloads) {
		t.Fatalf("accepted payloads=%v, want later path %v", s.acceptedPayloads, paths[1].Payloads)
	}
}

func TestCleanOrdinaryAmbiguityDoesNotUseRecoveryCost(t *testing.T) {
	s := newRecoveredRootMergedScheduler(t, 8)
	s.headers[0].clearRecoveryLineage()
	s.s3RegionOpened = false
	if err := s.completeAcceptance(); err == nil && s.receipt.Stop.Boundary == "" {
		t.Fatal("uncertified clean ambiguity had no decline")
	}
	if s.work.RecoveryRootSelections != 0 || s.work.RecoveryLineageSelections != 0 ||
		s.receipt.Acceptance != nil || s.acceptedHead.Node != 0 {
		t.Fatalf("clean ambiguity used recovery election: work=%+v acceptance=%+v",
			s.work, s.receipt.Acceptance)
	}
}

func TestRecoveredRootAcceptanceDeclineKeepsFrontier(t *testing.T) {
	for _, test := range []struct {
		name            string
		derivationLimit uint64
		missingSource   bool
	}{
		{name: "path cap", derivationLimit: 1},
		{name: "missing source", derivationLimit: 8, missingSource: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := newRecoveredRootMergedScheduler(t, test.derivationLimit)
			if test.missingSource {
				s.options.materializationSource = nil
			}
			before := s.headers[0]
			err := s.completeAcceptance()
			if err == nil && s.receipt.Stop.Boundary == "" {
				t.Fatal("invalid recovered-root election had no decline")
			}
			if len(s.headers) != 1 || s.headers[0].head != before.head ||
				!s.headers[0].isRecoveryLineage() || s.work.RecoveryRootSelections != 0 ||
				s.acceptedHead.Node != 0 || s.receipt.Acceptance != nil {
				t.Fatalf("declined election changed frontier: headers=%+v work=%+v accepted=%+v",
					s.headers, s.work, s.receipt.Acceptance)
			}
		})
	}
}
