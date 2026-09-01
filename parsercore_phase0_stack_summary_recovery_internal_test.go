//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

type stackSummaryRecoveryForkTable struct{}

func (stackSummaryRecoveryForkTable) Actions(state core.StateID, symbol core.Symbol) (core.ActionRow, error) {
	var actions []core.Action
	switch {
	case state == 1 && symbol == 2:
		actions = []core.Action{{Type: core.ActionShift, State: 2}}
	case state == 1 && symbol == 4:
		actions = []core.Action{{Type: core.ActionShift, State: 4}}
	case state == 1 && symbol == 5:
		actions = []core.Action{{Type: core.ActionShift, State: 5}}
	}
	return core.NewActionRow(actions, false), nil
}

func (stackSummaryRecoveryForkTable) Goto(core.StateID, core.Symbol) (core.StateID, error) {
	return 0, nil
}

func (stackSummaryRecoveryForkTable) ProductionFields(uint16, int) ([]core.FieldMapEntry, error) {
	return nil, nil
}

func (stackSummaryRecoveryForkTable) ProductionAliases(uint16, int) ([]core.Symbol, error) {
	return nil, nil
}

func newStackSummaryRecoveryForkScheduler(t *testing.T, armed bool) *diagnosticParserCoreGenericScheduler {
	t.Helper()
	compact, err := core.New(stackSummaryRecoveryForkTable{}, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(core.StateID(1), 0)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	head, err := compact.Shift(seed, core.Symbol(2), 0, core.Token{
		Symbol: 2, StartByte: 0, EndByte: 1,
	}, core.ForkOrder{})
	if err != nil {
		t.Fatalf("Shift: %v", err)
	}
	return &diagnosticParserCoreGenericScheduler{
		compact: compact,
		tokenSource: &dfaTokenSource{
			language: &Language{TokenCount: 5, SymbolCount: 5},
			lexer:    &Lexer{source: []byte("a?")},
		},
		headers: []diagnosticParserCoreHeader{{head: head, creationSeq: 3}},
		token:   Token{Symbol: 4, StartByte: 1, EndByte: 2},
		nextSeq: 10,
		options: DiagnosticParserCorePrefixOptions{
			Recovery:                             true,
			allowCompactStrategy2ErrorRegion:     true,
			allowCompactStackSummaryRecovery:     armed,
			allowCompactRecoveryLineageSelection: true,
		},
	}
}

func TestS4StackSummaryRecoveryForkPublishesBothLineages(t *testing.T) {
	scheduler := newStackSummaryRecoveryForkScheduler(t, true)
	handled, err := scheduler.s4TryStackSummaryRecovery(0)
	if err != nil {
		t.Fatalf("s4TryStackSummaryRecovery: %v", err)
	}
	if !handled {
		t.Fatal("the actionable ancestor did not fork")
	}
	if len(scheduler.headers) != 2 {
		t.Fatalf("headers=%d, want two recovery lineages", len(scheduler.headers))
	}
	if !scheduler.headers[0].isRecoveryLineage() || !scheduler.headers[1].isRecoveryLineage() {
		t.Fatal("the fork did not mark both recovery lineages")
	}
	if !scheduler.recoveryIsolation {
		t.Fatal("the fork did not activate recovery isolation")
	}
	if !scheduler.headers[0].shifted || scheduler.headers[0].recoveryRegion() == nil {
		t.Fatalf("absorb lineage did not consume the real token: %+v", scheduler.headers[0])
	}
	if scheduler.headers[1].shifted || scheduler.headers[1].recoveryRegion() != nil {
		t.Fatalf("recovered lineage consumed the real token: %+v", scheduler.headers[1])
	}
	if scheduler.headers[0].creationSeq != 3 || scheduler.headers[1].creationSeq != 10 || scheduler.nextSeq != 11 {
		t.Fatalf("creation sequence=%d/%d next=%d, want 3/10 next 11",
			scheduler.headers[0].creationSeq, scheduler.headers[1].creationSeq, scheduler.nextSeq)
	}
	if scheduler.headers[0].recoveryGroupIdentity() != 10 ||
		scheduler.headers[1].recoveryGroupIdentity() != 0 ||
		scheduler.headers[1].recoveryMissingGroupIdentity() != 0 {
		t.Fatalf("S4 recovery groups absorb=%d recovered=%d/%d, want 10 and 0/0",
			scheduler.headers[0].recoveryGroupIdentity(),
			scheduler.headers[1].recoveryGroupIdentity(),
			scheduler.headers[1].recoveryMissingGroupIdentity())
	}
	for index := range scheduler.headers {
		if baseline, set := scheduler.headers[index].recoveryNodeBaseline(); !set || baseline != 1 {
			t.Fatalf("S4 header %d baseline=%d/%t, want 1/true", index, baseline, set)
		}
	}
	if scheduler.work.StackSummaryRecoveryForks != 1 {
		t.Fatalf("stack-summary forks=%d, want 1", scheduler.work.StackSummaryRecoveryForks)
	}

	state, byteOffset, err := scheduler.compact.Boundary(scheduler.headers[1].head)
	if err != nil {
		t.Fatalf("recovered boundary: %v", err)
	}
	if state != 1 || byteOffset != 1 {
		t.Fatalf("recovered boundary=%d@%d, want 1@1", state, byteOffset)
	}
	derivations, err := scheduler.compact.Derivations(scheduler.headers[1].head)
	if err != nil {
		t.Fatalf("recovered derivation: %v", err)
	}
	if len(derivations) != 1 || len(derivations[0].Payloads) != 1 {
		t.Fatalf("recovered derivations=%+v, want one ERROR payload", derivations)
	}
	recovered, err := scheduler.compact.MaterializationView(derivations[0].Payloads[0])
	if err != nil {
		t.Fatalf("recovered payload: %v", err)
	}
	if recovered.Symbol != core.Symbol(errorSymbol) || recovered.StartByte != 0 || recovered.EndByte != 1 || len(recovered.Children) != 1 {
		t.Fatalf("recovered payload=%+v, want ERROR over bytes 0..1", recovered)
	}
}

func TestS3ErrorEntryPublishesCurrentVisibleNodeBaseline(t *testing.T) {
	scheduler := newStackSummaryRecoveryForkScheduler(t, true)
	handled, err := scheduler.s3TryOpenErrorRegionWithAlternatives(0, true)
	if err != nil {
		t.Fatalf("s3TryOpenErrorRegionWithAlternatives: %v", err)
	}
	if !handled || scheduler.headers[0].recoveryRegion() == nil {
		t.Fatal("standalone S3 did not open its error region")
	}
	if baseline, set := scheduler.headers[0].recoveryNodeBaseline(); !set || baseline != 1 {
		t.Fatalf("S3 baseline=%d/%t, want one visible shifted node", baseline, set)
	}
}

func TestS4StackSummaryRecoveryForkRequiresArtifactGate(t *testing.T) {
	scheduler := newStackSummaryRecoveryForkScheduler(t, false)
	original := scheduler.headers[0]
	handled, err := scheduler.s4TryStackSummaryRecovery(0)
	if err != nil {
		t.Fatalf("s4TryStackSummaryRecovery: %v", err)
	}
	if handled || len(scheduler.headers) != 1 || scheduler.headers[0] != original {
		t.Fatalf("unarmed S4 changed the frontier: handled=%t headers=%+v", handled, scheduler.headers)
	}
	if scheduler.work.StackSummaryRecoveryForks != 0 || scheduler.nextSeq != 10 || scheduler.recoveryIsolation {
		t.Fatalf("unarmed S4 changed state: forks=%d next=%d isolated=%t",
			scheduler.work.StackSummaryRecoveryForks, scheduler.nextSeq, scheduler.recoveryIsolation)
	}
}

func TestS4StackSummaryRecoveryForkRequiresSoleHeader(t *testing.T) {
	scheduler := newStackSummaryRecoveryForkScheduler(t, true)
	original := scheduler.headers[0]
	scheduler.headers = append(scheduler.headers, original)

	handled, err := scheduler.s4TryStackSummaryRecovery(1)
	if err != nil {
		t.Fatalf("s4TryStackSummaryRecovery: %v", err)
	}
	if handled || len(scheduler.headers) != 2 || scheduler.headers[0] != original || scheduler.headers[1] != original {
		t.Fatalf("multi-header S4 changed the frontier: handled=%t headers=%+v", handled, scheduler.headers)
	}
	if scheduler.work.StackSummaryRecoveryForks != 0 || scheduler.recoveryIsolation {
		t.Fatalf("multi-header S4 changed state: forks=%d isolated=%t",
			scheduler.work.StackSummaryRecoveryForks, scheduler.recoveryIsolation)
	}
}
