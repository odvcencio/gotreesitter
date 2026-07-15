//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

type legacyGoldenCheckpoint struct {
	Length int    `json:"n"`
	SHA256 string `json:"h"`
}

type legacyGoldenToken struct {
	Symbol               gotreesitter.Symbol `json:"s"`
	Text                 string              `json:"t,omitempty"`
	StartByte            uint32              `json:"b"`
	EndByte              uint32              `json:"e"`
	StartRow             uint32              `json:"sr,omitempty"`
	StartColumn          uint32              `json:"sc,omitempty"`
	EndRow               uint32              `json:"er,omitempty"`
	EndColumn            uint32              `json:"ec,omitempty"`
	Missing              bool                `json:"m,omitempty"`
	NoLookahead          bool                `json:"n,omitempty"`
	External             bool                `json:"x,omitempty"`
	ExternalScannerStart uint32              `json:"xs,omitempty"`
}

type legacyGoldenElection struct {
	States       []gotreesitter.StateID  `json:"s"`
	Token        legacyGoldenToken       `json:"t"`
	Before       *legacyGoldenCheckpoint `json:"b,omitempty"`
	After        *legacyGoldenCheckpoint `json:"a,omitempty"`
	CurrentValid bool                    `json:"v,omitempty"`
	CurrentStart *legacyGoldenCheckpoint `json:"cs,omitempty"`
	CurrentEnd   *legacyGoldenCheckpoint `json:"ce,omitempty"`
	CurrentBytes [2]uint32               `json:"cb,omitempty"`
}

type legacyGoldenHeader struct {
	CreationSeq uint64               `json:"q,omitempty"`
	State       gotreesitter.StateID `json:"s"`
	ByteOffset  uint32               `json:"b"`
	Flags       uint8                `json:"f,omitempty"`
	ExactPaths  uint64               `json:"p"`
	Checkpoint  string               `json:"c,omitempty"`
}

type legacyGoldenDerivation struct {
	Score       int64  `json:"s"`
	BranchOrder uint64 `json:"o"`
	HasOrder    bool   `json:"h,omitempty"`
}

type legacyGoldenPath struct {
	Header      legacyGoldenHeader       `json:"h"`
	Derivations []legacyGoldenDerivation `json:"d,omitempty"`
	Truncated   bool                     `json:"t,omitempty"`
}

type legacyGoldenGate struct {
	Byte          uint32             `json:"b"`
	ElectionIndex int                `json:"e"`
	Dispatches    uint64             `json:"d"`
	Headers       []legacyGoldenPath `json:"h"`
	Stats         [5]uint64          `json:"s"`
	Work          [18]uint64         `json:"w"`
}

type legacyGoldenNoAction struct {
	ElectionIndex          int                `json:"e"`
	Token                  legacyGoldenToken  `json:"t"`
	Dropped                legacyGoldenHeader `json:"d"`
	Preserved              legacyGoldenHeader `json:"p"`
	DroppedEffectiveCost   uint32             `json:"dc"`
	PreservedEffectiveCost uint32             `json:"pc"`
	DroppedResumed         bool               `json:"r"`
	OraclePinned           bool               `json:"o"`
}

type legacyGolden struct {
	Version           int                    `json:"v"`
	SourceSHA256      string                 `json:"src"`
	GrammarSHA256     string                 `json:"grammar_sha"`
	Grammar           string                 `json:"grammar"`
	DefaultCheckpoint legacyGoldenCheckpoint `json:"checkpoint"`
	Elections         []legacyGoldenElection `json:"elections"`
	Gates             []legacyGoldenGate     `json:"gates"`
	EditsElection     legacyGoldenElection   `json:"edits"`
	EditsFrontier     []legacyGoldenPath     `json:"frontier"`
	EditsStats        [5]uint64              `json:"stats"`
	BranchOrder       uint64                 `json:"order"`
	NextCreationSeq   uint64                 `json:"next"`
	SingleHeadNil     bool                   `json:"single_nil"`
	MultiHeadStates   []gotreesitter.StateID `json:"multi_states"`
	NoActions         []legacyGoldenNoAction `json:"no_actions"`
}

func legacyGoldenHash(hash [32]byte) string { return hex.EncodeToString(hash[:]) }

func legacyGoldenCheckpointFrom(receipt gotreesitter.DiagnosticParserCoreScannerCheckpoint) legacyGoldenCheckpoint {
	return legacyGoldenCheckpoint{Length: receipt.Length, SHA256: legacyGoldenHash(receipt.SHA256)}
}

func legacyGoldenNonDefaultCheckpoint(receipt gotreesitter.DiagnosticParserCoreScannerCheckpoint) *legacyGoldenCheckpoint {
	checkpoint := legacyGoldenCheckpointFrom(receipt)
	if checkpoint == (legacyGoldenCheckpoint{Length: 0, SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}) {
		return nil
	}
	return &checkpoint
}

func legacyGoldenTokenFrom(token gotreesitter.Token) legacyGoldenToken {
	return legacyGoldenToken{
		Symbol: token.Symbol, Text: token.Text, StartByte: token.StartByte, EndByte: token.EndByte,
		StartRow: token.StartPoint.Row, StartColumn: token.StartPoint.Column,
		EndRow: token.EndPoint.Row, EndColumn: token.EndPoint.Column,
		Missing: token.Missing, NoLookahead: token.NoLookahead,
		External: token.ExternalScannerToken, ExternalScannerStart: token.ExternalScannerStartByte,
	}
}

func legacyGoldenElectionFrom(election gotreesitter.DiagnosticParserCoreElection) legacyGoldenElection {
	return legacyGoldenElection{
		States: append([]gotreesitter.StateID(nil), election.States...), Token: legacyGoldenTokenFrom(election.Token),
		Before: legacyGoldenNonDefaultCheckpoint(election.ScannerBefore), After: legacyGoldenNonDefaultCheckpoint(election.ScannerAfter),
		CurrentValid: election.CurrentCheckpointValid,
		CurrentStart: legacyGoldenNonDefaultCheckpoint(election.CurrentCheckpointStart),
		CurrentEnd:   legacyGoldenNonDefaultCheckpoint(election.CurrentCheckpointEnd),
		CurrentBytes: election.CurrentCheckpointBytes,
	}
}

func legacyGoldenHeaderFrom(header gotreesitter.DiagnosticParserCoreHeaderReceipt) legacyGoldenHeader {
	var flags uint8
	if header.Shifted {
		flags |= 1
	}
	if header.Accepted {
		flags |= 2
	}
	if header.Paused {
		flags |= 4
	}
	checkpoint := legacyGoldenHash(header.Checkpoint)
	if checkpoint == "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		checkpoint = ""
	}
	return legacyGoldenHeader{
		CreationSeq: header.CreationSeq, State: header.State, ByteOffset: header.ByteOffset,
		Flags: flags, ExactPaths: header.ExactPaths, Checkpoint: checkpoint,
	}
}

func legacyGoldenPathFrom(path gotreesitter.DiagnosticParserCoreHeaderPathReceipt) legacyGoldenPath {
	out := legacyGoldenPath{Header: legacyGoldenHeaderFrom(path.Header), Truncated: path.DerivationsTruncated}
	for _, derivation := range path.Derivations {
		out.Derivations = append(out.Derivations, legacyGoldenDerivation{
			Score: derivation.Score, BranchOrder: derivation.BranchOrder, HasOrder: derivation.HasBranchOrder,
		})
	}
	return out
}

func legacyGoldenPathsFrom(paths []gotreesitter.DiagnosticParserCoreHeaderPathReceipt) []legacyGoldenPath {
	out := make([]legacyGoldenPath, len(paths))
	for index, path := range paths {
		out[index] = legacyGoldenPathFrom(path)
	}
	return out
}

func legacyGoldenStatsFrom(stats core.Stats) [5]uint64 {
	return [5]uint64{uint64(stats.Nodes), uint64(stats.Links), uint64(stats.Subtrees), uint64(stats.Children), stats.CurrentExactPaths}
}

func legacyGoldenWorkFrom(work gotreesitter.DiagnosticParserCoreGenericWork) [18]uint64 {
	return [18]uint64{
		work.Passes, work.ActionLookups, work.Dispatches, work.Conflicts, work.ConflictActions, work.Forks,
		work.ConflictHeads, work.Reductions, work.OrdinaryShifts, work.OrdinaryCohorts, work.ExtraShifts,
		work.ExtraCohorts, work.Accepts, work.ReductionPauses, work.NoActionDrops, work.Elections,
		work.Canonicalizations, work.PeakHeaders,
	}
}

func legacyGoldenFromSeedProbe(source []byte, probe gotreesitter.DiagnosticParserCoreSeedGoldenProbeForTest) legacyGolden {
	golden := legacyGolden{
		Version: 1, SourceSHA256: legacyGoldenHash(sha256Sum(source)),
		GrammarSHA256: "9cf914d26d962d1a62e7954f8b20b302337a44cb7d4a07218eec482c45a57a08",
		Grammar:       "go", DefaultCheckpoint: legacyGoldenCheckpointFrom(gotreesitter.DiagnosticParserCoreScannerCheckpoint{SHA256: sha256.Sum256(nil)}),
		EditsElection: legacyGoldenElectionFrom(probe.Election),
		EditsFrontier: legacyGoldenPathsFrom(probe.Headers), EditsStats: legacyGoldenStatsFrom(probe.Stats),
		BranchOrder: probe.BranchOrder, NextCreationSeq: probe.NextCreationSeq,
		SingleHeadNil: probe.SingleHeadGLRNil, MultiHeadStates: append([]gotreesitter.StateID(nil), probe.MultiHeadStates...),
	}
	for _, election := range probe.Receipt.Elections {
		golden.Elections = append(golden.Elections, legacyGoldenElectionFrom(election))
	}
	for _, gate := range probe.ClosedGates {
		golden.Gates = append(golden.Gates, legacyGoldenGate{
			Byte: gate.Byte, ElectionIndex: gate.ElectionIndex, Dispatches: gate.Dispatches,
			Headers: legacyGoldenPathsFrom(gate.Headers), Stats: legacyGoldenStatsFrom(gate.Stats), Work: legacyGoldenWorkFrom(gate.Work),
		})
	}
	for _, drop := range probe.Receipt.NoActionDrops {
		golden.NoActions = append(golden.NoActions, legacyGoldenNoAction{
			ElectionIndex: drop.ElectionIndex, Token: legacyGoldenTokenFrom(drop.Token), Dropped: legacyGoldenHeaderFrom(drop.Header.Header),
		})
	}
	return golden
}

func sha256Sum(data []byte) [32]byte {
	return sha256.Sum256(data)
}

func TestDiagnosticParserCoreSeedMatchesImmutableLegacyGolden(t *testing.T) {
	encoded, err := os.ReadFile("testdata/parsercore_phase0_legacy_first103.json")
	if err != nil {
		t.Fatal(err)
	}
	var want legacyGolden
	if err := json.Unmarshal(encoded, &want); err != nil {
		t.Fatal(err)
	}
	source := parserCoreGenericRewriteSource(t)
	probe, err := gotreesitter.RunDiagnosticParserCoreSeedGoldenProbeForTest(grammars.GoExternalScanner{}, source)
	if err != nil || probe.Receipt == nil {
		t.Fatalf("seed golden probe: probe=%+v err=%v", probe, err)
	}
	got := legacyGoldenFromSeedProbe(source, probe)
	if len(want.NoActions) != len(got.NoActions) {
		t.Fatalf("golden no-action decisions=%d live=%d", len(want.NoActions), len(got.NoActions))
	}
	if len(want.NoActions) != 2 || want.NoActions[0].ElectionIndex != 67 || want.NoActions[0].Dropped.State != 254 || want.NoActions[0].Preserved.State != 193 || want.NoActions[0].DroppedEffectiveCost != 100 || want.NoActions[0].PreservedEffectiveCost != 0 || want.NoActions[0].DroppedResumed || !want.NoActions[0].OraclePinned ||
		want.NoActions[1].ElectionIndex != 97 || want.NoActions[1].Dropped.State != 22 || want.NoActions[1].Preserved.State != 248 || want.NoActions[1].DroppedEffectiveCost != 600 || want.NoActions[1].PreservedEffectiveCost != 0 || want.NoActions[1].DroppedResumed || !want.NoActions[1].OraclePinned {
		t.Fatalf("immutable legacy no-action oracle is malformed: %+v", want.NoActions)
	}
	for index := range got.NoActions {
		if want.NoActions[index].ElectionIndex != got.NoActions[index].ElectionIndex ||
			!reflect.DeepEqual(want.NoActions[index].Token, got.NoActions[index].Token) ||
			!reflect.DeepEqual(want.NoActions[index].Dropped, got.NoActions[index].Dropped) {
			t.Fatalf("no-action decision %d drifted: live=%+v golden=%+v", index, got.NoActions[index], want.NoActions[index])
		}
		got.NoActions[index] = want.NoActions[index]
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("seed first-103 receipt diverged from immutable legacy golden")
	}
}
