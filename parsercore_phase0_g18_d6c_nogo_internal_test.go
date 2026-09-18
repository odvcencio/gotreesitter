//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

type g18D6cFrontierSnapshot struct {
	Schema    string                 `json:"schema"`
	Frontiers []g18D6cFrontierRecord `json:"frontiers"`
}

type g18D6cFrontierRecord struct {
	State        string              `json:"state"`
	Participants []g18D6cParticipant `json:"participants"`
}

type g18D6cParticipant struct {
	Head    core.Head              `json:"head"`
	Members []g18D6cFrontierMember `json:"members"`
}

type g18D6cFrontierMember struct {
	Derivation core.DropCohortDerivationHandle `json:"derivation"`
}

type g18D6cProjection struct {
	Symbol       core.Symbol
	ProductionID uint16
	StartByte    uint32
	EndByte      uint32
	Children     []core.SubtreeID
	Fields       []core.FieldMapEntry
	Aliases      []core.Symbol
	Terminal     bool
	Fragile      bool
	MetadataAuth bool
}

type g18D6cMemberEvidence struct {
	Digest     string
	Length     int
	State      core.StateID
	ByteOffset uint32
	Projection g18D6cProjection
}

func g18D6cCaptureMemberEvidence(
	t *testing.T,
	compact *core.Core,
	member g18D6cFrontierMember,
	source []byte,
) g18D6cMemberEvidence {
	t.Helper()
	record, ok := compact.DropCohortDerivationRecord(member.Derivation)
	if !ok {
		t.Fatalf("derivation record %v is unavailable", member.Derivation)
	}
	state, byteOffset, err := compact.Boundary(record.Head)
	if err != nil {
		t.Fatalf("derivation head boundary: %v", err)
	}
	if byteOffset != 1046 {
		t.Fatalf("derivation head byte offset=%d, want 1046", byteOffset)
	}
	if len(source) < 1037 {
		t.Fatalf("fixture source length=%d, cannot inspect 1030..1037", len(source))
	}
	if string(source[1030:1037]) != "prodIdx" {
		t.Fatalf("fixture source at 1030..1037=%q, want prodIdx", source[1030:1037])
	}

	var projection g18D6cProjection
	generation := compact.AuthenticationGeneration()
	path, err := compact.VisitEOFAdmissionExactPath(record.Head, generation, nil, func(ordinal uint32, payload core.SubtreeID) error {
		if ordinal != 4 {
			return nil
		}
		return compact.VisitEOFAdmissionSubtree(payload, generation, func(view core.EOFAdmissionSubtreeView) error {
			projection = g18D6cProjection{
				Symbol: view.Symbol, ProductionID: view.ProductionID,
				StartByte: view.StartByte, EndByte: view.EndByte,
				Children: append([]core.SubtreeID(nil), view.Children...),
				Fields:   append([]core.FieldMapEntry(nil), view.Fields...),
				Aliases:  append([]core.Symbol(nil), view.Aliases...),
				Terminal: view.Terminal, Fragile: view.Fragile,
				MetadataAuth: view.MetadataAuthenticated,
			}
			return nil
		})
	})
	if err != nil {
		t.Fatalf("derivation exact path: %v", err)
	}
	if path.Payloads <= 4 {
		t.Fatalf("derivation path payloads=%d, want path [0,4]", path.Payloads)
	}
	if projection.StartByte == 0 && projection.EndByte == 0 {
		t.Fatal("derivation path [0,4] did not expose a subtree")
	}

	return g18D6cMemberEvidence{
		Digest: fmt.Sprintf("%x", record.Digest[:]), Length: len(record.Bytes),
		State: state, ByteOffset: byteOffset, Projection: projection,
	}
}

func g18D6cLoadGrammargenLRFixture(t *testing.T) []byte {
	t.Helper()
	file, err := os.Open("internal/benchfixtures/testdata/grammargen_lr.go.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	source, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(source) != 235626 {
		t.Fatalf("grammargen_lr fixture length=%d, want 235626", len(source))
	}
	return source
}

func TestG18D6cGrammargenLRFrontierDeclinesOnStateAndPublicProjectionMismatch(t *testing.T) {
	source := g18D6cLoadGrammargenLRFixture(t)

	runner := newG18D6bDriverTestRunner(t)
	runner.certificateAdmissionEnabled = true
	runner.frontierRecordingEnabled = true
	runner.frontierVerificationEnabled = true
	var survivor, dropped g18D6cMemberEvidence
	targets := 0
	targetObserver := diagnosticParserCoreSeedObserver{
		frontierPublished: func(scheduler *diagnosticParserCoreGenericScheduler, owner core.SchedulerTransactionToken, dropIndices []int) error {
			if len(dropIndices) != 1 || dropIndices[0] != 1 {
				return nil
			}
			targets++
			if scheduler.electionIndex != 70 {
				return fmt.Errorf("target frontier election index=%d, want 70", scheduler.electionIndex)
			}
			var snapshot g18D6cFrontierSnapshot
			if err := json.Unmarshal(scheduler.compact.DiagnosticDropCohortFrontierSnapshotOwnedForTest(owner), &snapshot); err != nil {
				return fmt.Errorf("decode target frontier: %w", err)
			}
			if snapshot.Schema != "gts-drop-cohort-frontier/v1" || len(snapshot.Frontiers) != 1 {
				return fmt.Errorf("target frontier schema/count=%q/%d", snapshot.Schema, len(snapshot.Frontiers))
			}
			frontier := snapshot.Frontiers[0]
			if frontier.State != "complete" || len(frontier.Participants) != 2 {
				return fmt.Errorf("target frontier state/participants=%q/%d", frontier.State, len(frontier.Participants))
			}
			for index, participant := range frontier.Participants {
				if len(participant.Members) != 1 {
					return fmt.Errorf("target participant %d member count=%d, want 1", index, len(participant.Members))
				}
				evidence := g18D6cCaptureMemberEvidence(t, scheduler.compact, participant.Members[0], source)
				switch index {
				case 0:
					survivor = evidence
				case 1:
					dropped = evidence
				}
			}
			return nil
		},
	}
	_, candidateErr := runner.parseWithObserver(source, targetObserver)
	if targets != 1 {
		t.Fatalf("target frontier publications=%d, want 1", targets)
	}
	var decline *diagnosticParserCoreDecline
	if !errors.As(candidateErr, &decline) {
		t.Fatalf("candidate error=%v, want typed D6b decline", candidateErr)
	}
	if decline.boundary != DiagnosticParserCoreNoAction || decline.detail != "converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor" {
		t.Fatalf("candidate decline=%+v, want typed no_action alternative-set decline", decline)
	}

	if survivor.Digest != "4725c17a4eb81818e0935b924a5d24e50fa69182215dac67491af9093f8a5776" || survivor.Length != 4705 {
		t.Fatalf("survivor derivation=%s/%d, want locked D6b receipt digest/length", survivor.Digest, survivor.Length)
	}
	if dropped.Digest != "67b910ef56c7fd84197996d9a4ddcf75978036215289de05c2d39e1a5eea6470" || dropped.Length != 4610 {
		t.Fatalf("dropped derivation=%s/%d, want locked D6b receipt digest/length", dropped.Digest, dropped.Length)
	}
	if survivor.Digest == dropped.Digest {
		t.Fatal("survivor and dropped derivation digests unexpectedly match")
	}
	if survivor.State != 1275 || dropped.State != 1279 {
		t.Fatalf("continuation states=%d/%d, want survivor 1275 and dropped 1279", survivor.State, dropped.State)
	}
	if survivor.Projection.StartByte != 1030 || survivor.Projection.EndByte != 1037 ||
		dropped.Projection.StartByte != 1030 || dropped.Projection.EndByte != 1037 {
		t.Fatalf("path [0,4] spans=%d..%d/%d..%d, want 1030..1037", survivor.Projection.StartByte, survivor.Projection.EndByte, dropped.Projection.StartByte, dropped.Projection.EndByte)
	}
	if dropped.Projection.Symbol != 1 || dropped.Projection.ProductionID != 0 || !dropped.Projection.Terminal || len(dropped.Projection.Children) != 0 || len(dropped.Projection.Fields) != 0 {
		t.Fatalf("dropped path [0,4] projection=%+v, want terminal identifier production 0", dropped.Projection)
	}
	if survivor.Projection.Symbol != 112 || survivor.Projection.ProductionID != 19 || survivor.Projection.Terminal || len(survivor.Projection.Children) != 1 || len(survivor.Projection.Fields) != 1 || survivor.Projection.Fields[0].FieldID != 31 || survivor.Projection.Fields[0].ChildIndex != 0 {
		t.Fatalf("survivor path [0,4] projection=%+v, want parameter_declaration production 19 with type child", survivor.Projection)
	}
	language, err := authenticatedParserCoreGoLanguage(parserCoreWarmGoScanner)
	if err != nil {
		t.Fatal(err)
	}
	if language.SymbolNames[dropped.Projection.Symbol] != "identifier" || language.SymbolNames[survivor.Projection.Symbol] != "parameter_declaration" || language.FieldNames[survivor.Projection.Fields[0].FieldID] != "type" {
		t.Fatal("frontier projections do not represent an identifier and a typed parameter")
	}
	if survivor.Projection.Symbol == dropped.Projection.Symbol || survivor.Projection.ProductionID == dropped.Projection.ProductionID {
		t.Fatal("survivor and dropped public projections unexpectedly match")
	}
	if !survivor.Projection.MetadataAuth || !dropped.Projection.MetadataAuth {
		t.Fatal("frontier subtree metadata is not authenticated")
	}
}
