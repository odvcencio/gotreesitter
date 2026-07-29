//go:build gts_parsercorephase0

package gotreesitter

import (
	"crypto/sha256"
	"errors"
	"reflect"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestParserCoreCheckpointPreservesExactEmptyAndNonEmptyIdentity(t *testing.T) {
	empty := parserCoreCheckpoint(nil)
	if empty.Length != 0 || empty.SHA256 != sha256.Sum256(nil) || empty != parserCoreCheckpoint([]byte{}) {
		t.Fatalf("empty checkpoint identity=%+v", empty)
	}
	payload := []byte{0, 1, 2, 3}
	nonEmpty := parserCoreCheckpoint(payload)
	if nonEmpty.Length != len(payload) || nonEmpty.SHA256 != sha256.Sum256(payload) || nonEmpty == empty {
		t.Fatalf("non-empty checkpoint identity=%+v", nonEmpty)
	}
}

func TestParserCoreCheckpointIDsOwnStatefulScannerScratch(t *testing.T) {
	lang := &Language{Name: "checkpoint-id-test", ExternalScanner: byteStateExternalScanner{}}
	tokenSource := acquireDFATokenSource(NewLexer(nil, nil), lang, nil, nil, nil, nil)
	defer tokenSource.Close()
	compact, err := core.New(&genericConflictTable{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	var scratch []byte
	state := tokenSource.externalPayload.(*byte)
	*state = 7
	firstBytes := tokenSource.captureExternalScannerStateInto(&scratch)
	firstID := mustDiagnosticCheckpointID(t, compact, firstBytes)
	firstReceipt := parserCoreCheckpoint(firstBytes)
	*state = 9
	secondBytes := tokenSource.captureExternalScannerStateInto(&scratch)
	secondID := mustDiagnosticCheckpointID(t, compact, secondBytes)
	if firstID == secondID || firstID == 0 || secondID == 0 {
		t.Fatalf("stateful scanner IDs collapsed: first=%d second=%d", firstID, secondID)
	}
	length, digest, ok := compact.CheckpointReceipt(firstID)
	if !ok || length != uint32(firstReceipt.Length) || digest != firstReceipt.SHA256 {
		t.Fatalf("first scanner receipt after scratch reuse=(%d,%x,%t), want %+v", length, digest, ok, firstReceipt)
	}
	*state = 7
	if repeated := mustDiagnosticCheckpointID(t, compact, tokenSource.captureExternalScannerStateInto(&scratch)); repeated != firstID {
		t.Fatalf("restored scanner state ID=%d, want %d", repeated, firstID)
	}
}

func TestDiagnosticParserCoreSeedPublicationRejectsMaterializationTransactionally(t *testing.T) {
	base := DiagnosticParserCorePrefixResult{
		Grammar: "go", ExactRootDFA: true,
		SourceSHA256: [32]byte{1}, GrammarBlobSHA256: [32]byte{2},
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		receipt: &DiagnosticParserCoreGenericScheduler{
			Tokens: 1, Dispatches: 1,
			Acceptance: &DiagnosticParserCoreGenericAcceptance{
				Work: DiagnosticParserCoreGenericWork{Dispatches: 1},
			},
		},
	}
	wantErr := errors.New("materialization fault")
	result, err := publishDiagnosticParserCoreGenericResult(base, scheduler, func(core.Head) (*Tree, error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("materialization error=%v, want %v", err, wantErr)
	}
	if !reflect.DeepEqual(result, base) || result.GenericScheduler != nil || result.MaterializedTree != nil || result.Materialized || result.Completed || len(result.Elections) != 0 || result.Tokens != 0 || result.Dispatches != 0 {
		t.Fatalf("failed materialization leaked publication: %+v", result)
	}
	if acceptance := scheduler.receipt.Acceptance; acceptance.SelectedNodes != 0 || acceptance.SelectedParents != 0 || acceptance.SelectedLeaves != 0 {
		t.Fatalf("failed materialization leaked selected census: %+v", acceptance)
	}
	result, err = publishDiagnosticParserCoreGenericResult(base, scheduler, func(core.Head) (*Tree, error) {
		return nil, nil
	})
	if err == nil || !reflect.DeepEqual(result, base) || result.GenericScheduler != nil || result.MaterializedTree != nil || result.Materialized || result.Completed {
		t.Fatalf("empty materialization leaked publication: result=%+v err=%v", result, err)
	}
	if acceptance := scheduler.receipt.Acceptance; acceptance.SelectedNodes != 0 || acceptance.SelectedParents != 0 || acceptance.SelectedLeaves != 0 {
		t.Fatalf("empty materialization leaked selected census: %+v", acceptance)
	}
}

func TestDiagnosticParserCoreGenericSeedConstructorFailsClosed(t *testing.T) {
	compact, err := core.New(&genericConflictTable{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := parserCoreCheckpoint(nil)
	var scratch []byte
	diagnosticScheduler, err := newDiagnosticParserCoreGenericScheduler(compact, &dfaTokenSource{}, &scratch, seed, 0, checkpoint, diagnosticParserCoreSeedObserver{}, DiagnosticParserCorePrefixOptions{})
	if err != nil {
		t.Fatalf("valid seed start declined: %v", err)
	}
	if diagnosticScheduler.receipt == &diagnosticScheduler.receiptBacking {
		t.Fatal("public diagnostic scheduler embedded its published receipt")
	}
	freshScheduler, err := newDiagnosticParserCoreGenericScheduler(compact, &dfaTokenSource{}, &scratch, seed, 0, checkpoint, diagnosticParserCoreSeedObserver{}, DiagnosticParserCorePrefixOptions{freshSchedulerSession: true})
	if err != nil {
		t.Fatalf("fresh seed start declined: %v", err)
	}
	if freshScheduler.receipt != &freshScheduler.receiptBacking {
		t.Fatal("fresh scheduler did not use its embedded receipt")
	}
	if _, err := newDiagnosticParserCoreGenericScheduler(nil, &dfaTokenSource{}, &scratch, seed, 0, checkpoint, diagnosticParserCoreSeedObserver{}, DiagnosticParserCorePrefixOptions{}); err == nil {
		t.Fatal("nil compact core was admitted")
	}
	if _, err := newDiagnosticParserCoreGenericScheduler(compact, nil, &scratch, seed, 0, checkpoint, diagnosticParserCoreSeedObserver{}, DiagnosticParserCorePrefixOptions{}); err == nil {
		t.Fatal("nil token source was admitted")
	}
	if _, err := newDiagnosticParserCoreGenericScheduler(compact, &dfaTokenSource{}, nil, seed, 0, checkpoint, diagnosticParserCoreSeedObserver{}, DiagnosticParserCorePrefixOptions{}); err == nil {
		t.Fatal("nil scratch was admitted")
	}
	if _, err := newDiagnosticParserCoreGenericScheduler(compact, &dfaTokenSource{}, &scratch, core.Head{}, 0, checkpoint, diagnosticParserCoreSeedObserver{}, DiagnosticParserCorePrefixOptions{}); err == nil {
		t.Fatal("empty seed head was admitted")
	}
	nonemptyID := mustDiagnosticCheckpointID(t, compact, []byte{1})
	if _, err := newDiagnosticParserCoreGenericScheduler(compact, &dfaTokenSource{}, &scratch, seed, nonemptyID, parserCoreCheckpoint([]byte{1}), diagnosticParserCoreSeedObserver{}, DiagnosticParserCorePrefixOptions{}); err == nil {
		t.Fatal("seed created under a different checkpoint identity was admitted")
	}
	if _, err := newDiagnosticParserCoreGenericScheduler(compact, &dfaTokenSource{}, &scratch, seed, 0, parserCoreCheckpoint([]byte{1}), diagnosticParserCoreSeedObserver{}, DiagnosticParserCorePrefixOptions{}); err == nil {
		t.Fatal("checkpoint receipt/identity mismatch was admitted")
	}
}

// DiagnosticParserCoreSeedClosedGateForTest is one closed seed-owned frontier
// observed immediately before its next scanner election.
type DiagnosticParserCoreSeedClosedGateForTest struct {
	Byte          uint32
	ElectionIndex int
	Dispatches    uint64
	Headers       []DiagnosticParserCoreHeaderPathReceipt
	Stats         core.Stats
	Work          DiagnosticParserCoreGenericWork
}

// DiagnosticParserCoreSeedGoldenProbeForTest captures the immutable golden's
// pre-dispatch election barrier and named closed-frontier gates.
type DiagnosticParserCoreSeedGoldenProbeForTest struct {
	Receipt          *DiagnosticParserCoreGenericScheduler
	Election         DiagnosticParserCoreElection
	Headers          []DiagnosticParserCoreHeaderPathReceipt
	Stats            core.Stats
	ClosedGates      []DiagnosticParserCoreSeedClosedGateForTest
	BranchOrder      uint64
	NextCreationSeq  uint64
	SingleHeadGLRNil bool
	MultiHeadStates  []StateID
}

// DiagnosticParserCoreSeedCapForTest identifies the exact operation that a
// seed-owned scheduler cap prevented before public result publication.
type DiagnosticParserCoreSeedCapForTest struct {
	ElectionIndex int
	Token         Token
	Dispatches    uint64
	Work          DiagnosticParserCoreGenericWork
}

// RunDiagnosticParserCoreSeedCapForTest runs one capped seed-owned diagnostic
// scheduler and returns its private stop point. It is available only to tagged
// tests; DiagnosticParseParserCorePrefix deliberately does not publish it.
func RunDiagnosticParserCoreSeedCapForTest(scanner ExternalScanner, source []byte, options DiagnosticParserCorePrefixOptions) (DiagnosticParserCoreSeedCapForTest, error) {
	lang, err := authenticatedParserCoreGoLanguage(scanner)
	if err != nil {
		return DiagnosticParserCoreSeedCapForTest{}, err
	}
	parser := NewParser(lang)
	tables, err := newParserCoreRootTables(parser)
	if err != nil {
		return DiagnosticParserCoreSeedCapForTest{}, err
	}
	compact, err := core.New(tables, options.Limits)
	if err != nil {
		return DiagnosticParserCoreSeedCapForTest{}, err
	}
	tokenSource := parser.acquireParserDFATokenSource(source)
	if tokenSource == nil {
		return DiagnosticParserCoreSeedCapForTest{}, errors.New("parser-core phase zero: production DFA unavailable")
	}
	defer tokenSource.Close()
	if options.MaxDispatches == 0 {
		options.MaxDispatches = 100000
	}
	if options.MaxTokens == 0 {
		options.MaxTokens = 100000
	}
	var scannerScratch []byte
	scheduler, runErr := executeDiagnosticParserCoreGenericSchedulerFromSeed(
		compact, tokenSource, &scannerScratch, lang.InitialState, options, diagnosticParserCoreSeedObserver{},
	)
	if scheduler == nil {
		return DiagnosticParserCoreSeedCapForTest{}, runErr
	}
	return DiagnosticParserCoreSeedCapForTest{
		ElectionIndex: scheduler.electionIndex, Token: scheduler.token,
		Dispatches: scheduler.dispatches, Work: scheduler.work,
	}, runErr
}

// RunDiagnosticParserCoreSeedGoldenProbeForTest runs the generic clean scheduler
// from Core.Seed and stops after the edits election, before dispatching it.
// It is test-only and compares the sole route against checked-in legacy data.
func RunDiagnosticParserCoreSeedGoldenProbeForTest(scanner ExternalScanner, source []byte) (DiagnosticParserCoreSeedGoldenProbeForTest, error) {
	lang, err := authenticatedParserCoreGoLanguage(scanner)
	if err != nil {
		return DiagnosticParserCoreSeedGoldenProbeForTest{}, err
	}
	parser := NewParser(lang)
	tables, err := newParserCoreRootTables(parser)
	if err != nil {
		return DiagnosticParserCoreSeedGoldenProbeForTest{}, err
	}
	compact, err := core.New(tables, core.Limits{})
	if err != nil {
		return DiagnosticParserCoreSeedGoldenProbeForTest{}, err
	}
	tokenSource := parser.acquireParserDFATokenSource(source)
	if tokenSource == nil {
		return DiagnosticParserCoreSeedGoldenProbeForTest{}, errors.New("parser-core phase zero: production DFA unavailable")
	}
	defer tokenSource.Close()

	wantedGates := map[uint32]bool{116: true, 580: true, 725: true, 732: true, 742: true}
	probe := DiagnosticParserCoreSeedGoldenProbeForTest{}
	var scannerScratch []byte
	observer := diagnosticParserCoreSeedObserver{
		beforeElection: func(s *diagnosticParserCoreGenericScheduler) error {
			paths, err := diagnosticParserCoreHeaderPathReceipts(s.compact, s.headers)
			if err != nil {
				return err
			}
			if len(paths) == 0 {
				return errors.New("parser-core phase zero: seed golden probe observed an empty closed frontier")
			}
			closedByte := paths[0].Header.ByteOffset
			if !wantedGates[closedByte] {
				return nil
			}
			for _, path := range paths {
				if path.Header.ByteOffset != closedByte || !path.Header.Shifted || path.Header.Accepted || path.Header.Checkpoint != s.checkpoint.SHA256 {
					return errors.New("parser-core phase zero: seed golden probe gate is not closed and checkpoint-continuous")
				}
			}
			stats, err := s.compact.Stats(s.headers[0].head)
			if err != nil {
				return err
			}
			probe.ClosedGates = append(probe.ClosedGates, DiagnosticParserCoreSeedClosedGateForTest{
				Byte: closedByte, ElectionIndex: s.electionIndex, Dispatches: s.dispatches,
				Headers: paths, Stats: stats, Work: s.work,
			})
			return nil
		},
		afterElection: func(s *diagnosticParserCoreGenericScheduler) (bool, error) {
			if s.electionIndex == 0 {
				probe.SingleHeadGLRNil = s.tokenSource.glrStates == nil
			}
			if s.electionIndex != 102 {
				return false, nil
			}
			if s.token.StartByte != 742 || s.token.EndByte != 747 {
				return false, errors.New("parser-core phase zero: seed golden probe election 102 did not reach the edits token")
			}
			paths, err := diagnosticParserCoreHeaderPathReceipts(s.compact, s.headers)
			if err != nil {
				return false, err
			}
			stats, err := s.compact.Stats(s.headers[0].head)
			if err != nil {
				return false, err
			}
			probe.Election = s.currentElection
			probe.Headers = paths
			probe.Stats = stats
			probe.BranchOrder = s.branchOrder
			probe.NextCreationSeq = s.nextSeq
			probe.MultiHeadStates = append([]StateID(nil), s.tokenSource.glrStates...)
			return true, nil
		},
	}
	scheduler, err := executeDiagnosticParserCoreGenericSchedulerFromSeed(
		compact, tokenSource, &scannerScratch, lang.InitialState,
		DiagnosticParserCorePrefixOptions{MaxDispatches: 100000, MaxTokens: 100000},
		observer,
	)
	if scheduler != nil {
		probe.Receipt = scheduler.receipt
	}
	return probe, err
}
