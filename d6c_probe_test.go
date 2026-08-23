//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

type tmpD6CDerivationMember struct {
	Participant int                             `json:"participant"`
	Ref         core.DropCohortRef              `json:"ref"`
	Head        core.Head                       `json:"head"`
	BranchOrder uint64                          `json:"branch_order"`
	Action      core.DropCohortActionIdentity   `json:"action"`
	Derivation  core.DropCohortDerivationHandle `json:"derivation"`
	Digest      string                          `json:"digest"`
	Length      uint32                          `json:"length"`
	RootSymbol  core.Symbol                     `json:"root_symbol"`
	StackDepth  uint32                          `json:"stack_depth"`
	Checkpoint  core.DropCohortSourceCheckpoint `json:"checkpoint"`
	BytesBase64 string                          `json:"bytes_base64"`
	Views       []tmpD6CSubtreeView             `json:"subtree_views"`
}

type tmpD6CSubtreeView struct {
	ID                core.SubtreeID       `json:"id"`
	Symbol            core.Symbol          `json:"symbol"`
	ProductionID      uint16               `json:"production_id"`
	DynamicPrecedence int16                `json:"dynamic_precedence"`
	StartByte         uint32               `json:"start_byte"`
	EndByte           uint32               `json:"end_byte"`
	Children          []core.SubtreeID     `json:"children"`
	Fields            []core.FieldMapEntry `json:"fields"`
	Aliases           []core.Symbol        `json:"aliases"`
	Extra             bool                 `json:"extra"`
	External          bool                 `json:"external"`
	Terminal          bool                 `json:"terminal"`
	Fragile           bool                 `json:"fragile"`
}

type tmpD6CFrontier struct {
	ElectionIndex int                      `json:"election_index"`
	DropIndices   []int                    `json:"drop_indices"`
	Snapshot      json.RawMessage          `json:"snapshot"`
	Members       []tmpD6CDerivationMember `json:"members"`
}

type tmpD6CSnapshotEnvelope struct {
	Frontiers []struct {
		Participants []struct {
			Head        core.Head `json:"head"`
			BranchOrder uint64    `json:"branch_order"`
			Members     []struct {
				Participant     uint16                          `json:"participant"`
				Ref             core.DropCohortRef              `json:"ref"`
				ParticipantHead core.Head                       `json:"participant_head"`
				SourceHead      core.Head                       `json:"source_head"`
				BranchOrder     uint64                          `json:"branch_order"`
				Action          core.DropCohortActionIdentity   `json:"action"`
				Derivation      core.DropCohortDerivationHandle `json:"derivation"`
			} `json:"members"`
		} `json:"participants"`
	} `json:"frontiers"`
}

func TestTmpD6CGrammargenLRDerivationDump(t *testing.T) {
	fixture := loadDiagnosticParserCoreCanonicalFixture(t, "grammargen_lr")
	source := fixture.Source
	lang, err := authenticatedParserCoreGoLanguage(parserCoreWarmGoScanner)
	if err != nil {
		t.Fatal(err)
	}
	lang.CompactConvergedReductionSplitDropsCertified = false
	parser := NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	runner, err := newAdmissionCandidateRunner(parser)
	if err != nil {
		t.Fatal(err)
	}
	runner.certificateAdmissionEnabled = true
	runner.frontierRecordingEnabled = true
	runner.options.ReceiptMode = DiagnosticParserCoreReceiptFull
	var frontiers []tmpD6CFrontier
	var targetRounds []DiagnosticParserCoreDispatchRound
	var targetElection DiagnosticParserCoreElection
	var targetElections []DiagnosticParserCoreElection
	var targetConflicts []DiagnosticParserCoreGenericConflict
	var targetHeaders []DiagnosticParserCoreHeaderPathReceipt
	runner.frontierPublishedObserver = func(scheduler *diagnosticParserCoreGenericScheduler, owner core.SchedulerTransactionToken, drops []int) error {
		if scheduler == nil || scheduler.compact == nil || len(drops) == 0 {
			return nil
		}
		raw := scheduler.compact.DiagnosticDropCohortFrontierSnapshotOwnedForTest(owner)
		var envelope tmpD6CSnapshotEnvelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return err
		}
		if len(envelope.Frontiers) != 1 {
			return fmt.Errorf("frontier snapshot count=%d", len(envelope.Frontiers))
		}
		frontier := tmpD6CFrontier{
			ElectionIndex: scheduler.electionIndex,
			DropIndices:   append([]int(nil), drops...),
			Snapshot:      append(json.RawMessage(nil), raw...),
		}
		targetElection = scheduler.currentElection
		if headers, headerErr := diagnosticParserCoreHeaderPathReceipts(scheduler.compact, scheduler.headers); headerErr != nil {
			return headerErr
		} else {
			targetHeaders = headers
		}
		if scheduler.receipt != nil {
			if len(scheduler.receipt.Elections) > 10 {
				targetElections = append([]DiagnosticParserCoreElection(nil), scheduler.receipt.Elections[len(scheduler.receipt.Elections)-10:]...)
			} else {
				targetElections = append([]DiagnosticParserCoreElection(nil), scheduler.receipt.Elections...)
			}
			for _, conflict := range scheduler.receipt.Conflicts {
				if conflict.ElectionIndex >= scheduler.electionIndex-5 && conflict.ElectionIndex <= scheduler.electionIndex {
					targetConflicts = append(targetConflicts, conflict)
				}
			}
		}
		if scheduler.receipt != nil && len(scheduler.receipt.Rounds) > 128 {
			targetRounds = append([]DiagnosticParserCoreDispatchRound(nil), scheduler.receipt.Rounds[len(scheduler.receipt.Rounds)-128:]...)
		} else {
			targetRounds = append([]DiagnosticParserCoreDispatchRound(nil), scheduler.receipt.Rounds...)
		}
		for participantIndex, participant := range envelope.Frontiers[0].Participants {
			for _, member := range participant.Members {
				record, ok := scheduler.compact.DropCohortDerivationRecord(member.Derivation)
				if !ok {
					return fmt.Errorf("derivation record unavailable: participant=%d ref=%+v handle=%+v", participantIndex, member.Ref, member.Derivation)
				}
				var views []tmpD6CSubtreeView
				if derivations, derivationErr := scheduler.compact.Derivations(record.Head); derivationErr != nil {
					return derivationErr
				} else if len(derivations) != 1 {
					return fmt.Errorf("participant=%d derivations=%d", participantIndex, len(derivations))
				} else {
					seen := make(map[core.SubtreeID]bool)
					var appendView func(core.SubtreeID) error
					appendView = func(payload core.SubtreeID) error {
						if seen[payload] {
							return nil
						}
						seen[payload] = true
						view, viewErr := scheduler.compact.Subtree(payload)
						if viewErr != nil {
							return viewErr
						}
						materializationView, materializationErr := scheduler.compact.MaterializationView(payload)
						if materializationErr != nil {
							return materializationErr
						}
						views = append(views, tmpD6CSubtreeView{
							ID: payload, Symbol: view.Symbol, ProductionID: view.ProductionID,
							DynamicPrecedence: view.DynamicPrecedence, StartByte: view.StartByte, EndByte: view.EndByte,
							Children: append([]core.SubtreeID(nil), view.Children...), Fields: append([]core.FieldMapEntry(nil), view.Fields...),
							Aliases: append([]core.Symbol(nil), view.Aliases...), Extra: view.Extra, External: view.External, Terminal: view.Terminal,
							Fragile: materializationView.Fragile,
						})
						for _, child := range view.Children {
							if err := appendView(child); err != nil {
								return err
							}
						}
						return nil
					}
					for _, payload := range derivations[0].Payloads {
						if err := appendView(payload); err != nil {
							return err
						}
					}
				}
				frontier.Members = append(frontier.Members, tmpD6CDerivationMember{
					Participant: int(member.Participant), Ref: member.Ref, Head: participant.Head,
					BranchOrder: member.BranchOrder, Action: member.Action, Derivation: member.Derivation,
					Digest: hex.EncodeToString(record.Digest[:]), Length: uint32(len(record.Bytes)),
					RootSymbol: record.RootSymbol, StackDepth: record.StackDepth,
					Checkpoint: record.Checkpoint, BytesBase64: base64.StdEncoding.EncodeToString(append([]byte(nil), record.Bytes...)), Views: views,
				})
			}
		}
		frontiers = append(frontiers, frontier)
		return nil
	}
	_, parseErr := runner.parseWithObserver(source, diagnosticParserCoreSeedObserver{})
	output := struct {
		Commit    string                                  `json:"commit"`
		ParseErr  string                                  `json:"parse_error"`
		SourceLen int                                     `json:"source_len"`
		Frontiers []tmpD6CFrontier                        `json:"frontiers"`
		Election  DiagnosticParserCoreElection            `json:"target_election"`
		Rounds    []DiagnosticParserCoreDispatchRound     `json:"target_rounds"`
		Elections []DiagnosticParserCoreElection          `json:"target_elections"`
		Conflicts []DiagnosticParserCoreGenericConflict   `json:"target_conflicts"`
		Headers   []DiagnosticParserCoreHeaderPathReceipt `json:"target_headers"`
	}{Commit: "f5adfc7091bad6f8adb5088a32f3b2912561fc72", SourceLen: len(source), Frontiers: frontiers, Election: targetElection, Rounds: targetRounds, Elections: targetElections, Conflicts: targetConflicts, Headers: targetHeaders}
	if parseErr != nil {
		output.ParseErr = parseErr.Error()
	}
	encoded, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("/tmp/d6c-grammargen-lr-derivations.json", encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("parse_err=%v frontiers=%d source_len=%d output=%s", parseErr, len(frontiers), len(source), "/tmp/d6c-grammargen-lr-derivations.json")
	if len(frontiers) == 0 {
		t.Fatal("no target frontier captured")
	}
}
