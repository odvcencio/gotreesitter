//go:build gts_workcount

package gotreesitter_test

import (
	"crypto/sha256"
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

const erlangIssue984Source = "000\"0A!A \"A\"=0:A0!)A\"0%0000"

func captureErlangIssue984Topology(t *testing.T) (string, gts.DiagnosticTopologyReceipt) {
	t.Helper()
	if got := len(erlangIssue984Source); got != 27 {
		t.Fatalf("Erlang issue 984 witness length = %d, want 27", got)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(erlangIssue984Source))); got != "ad9482f4aabfc3f348a20f6071a2732dcf0473d87b61fa8df4f960e84d4b1ab4" {
		t.Fatalf("Erlang issue 984 witness SHA-256 = %s", got)
	}
	parser := gts.NewParser(grammars.ErlangLanguage())
	parser.SetAdmissionCandidateRoute(false)
	gts.BeginDiagnosticTopologyReceipt()
	tree, err := parser.Parse([]byte(erlangIssue984Source))
	receipt := gts.EndDiagnosticTopologyReceipt()
	if err != nil {
		t.Fatalf("parse the Erlang issue 984 witness: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		t.Fatal("parse the Erlang issue 984 witness: no root")
	}
	defer tree.Release()
	if tree.RootNode().HasError() {
		t.Fatalf("Erlang issue 984 witness has an error root: %s", tree.RootNode().SExpr(grammars.ErlangLanguage()))
	}
	return tree.RootNode().SExpr(grammars.ErlangLanguage()), receipt
}

func assertDiagnosticTopologyEventInvariants(t *testing.T, receipt gts.DiagnosticTopologyReceipt) {
	t.Helper()
	const allowedFlags = uint64(1<<6) - 1
	nextVersionID := uint64(1)
	nextLinkID := uint64(1)
	nextElectionID := uint64(1)
	lastActionID := uint64(0)
	lastPopID := uint64(0)
	lastPathOrdinal := uint64(0)
	for i, event := range receipt.Events {
		if event.EventID != uint64(i+1) {
			t.Fatalf("event %d has ID %d", i, event.EventID)
		}
		if event.Kind < gts.DiagnosticTopologyEventAction || event.Kind > gts.DiagnosticTopologyEventAcceptElection {
			t.Fatalf("event %d has kind %d", event.EventID, event.Kind)
		}
		if event.Flags&^allowedFlags != 0 {
			t.Fatalf("event %d has reserved flags %#x", event.EventID, event.Flags)
		}
		hasAction := event.Flags&gts.DiagnosticTopologyFlagActionContextKnown != 0
		if hasAction && (event.ActionID == 0 || event.ActionType > uint64(gts.ParseActionRecover) || event.ActionOrdinal < -1) {
			t.Fatalf("event %d has invalid action context: %+v", event.EventID, event)
		}
		if !hasAction && (event.ActionID != 0 || event.ActionOrdinal != -1 || event.ActionType != 255) {
			t.Fatalf("event %d has invalid absent action context: %+v", event.EventID, event)
		}
		switch event.Kind {
		case gts.DiagnosticTopologyEventAction:
			if event.ActionID <= lastActionID {
				t.Fatalf("action ID %d follows %d", event.ActionID, lastActionID)
			}
			lastActionID = event.ActionID
		case gts.DiagnosticTopologyEventVersionAdd, gts.DiagnosticTopologyEventVersionCopy:
			if event.VersionID != nextVersionID || event.NodeID == 0 {
				t.Fatalf("version event %d has ID/node %d/%d, want ID %d", event.EventID, event.VersionID, event.NodeID, nextVersionID)
			}
			nextVersionID++
		case gts.DiagnosticTopologyEventVersionRenumber:
			if event.VersionID == 0 || event.SourceVersionID != event.VersionID || event.TargetVersionID == 0 || event.Flags&gts.DiagnosticTopologyFlagTargetReplaced == 0 {
				t.Fatalf("invalid renumber event %d: %+v", event.EventID, event)
			}
		case gts.DiagnosticTopologyEventMerge:
			if event.SurvivorVersionID == 0 || event.RemovedVersionID == 0 || event.SurvivorVersionID != event.SourceVersionID || event.RemovedVersionID != event.TargetVersionID {
				t.Fatalf("invalid merge event %d: %+v", event.EventID, event)
			}
		case gts.DiagnosticTopologyEventLinkInsert:
			if event.NodeID == 0 || event.PredecessorNodeID == 0 || event.LinkID != nextLinkID || event.LinkOrdinal >= 8 {
				t.Fatalf("link event %d has an incomplete identity: %+v", event.EventID, event)
			}
			nextLinkID++
		case gts.DiagnosticTopologyEventPopPath:
			if event.PopID == 0 || event.VersionID == 0 || event.SourceVersionID == 0 || event.NodeID == 0 || event.PopToNodeID == 0 {
				t.Fatalf("pop event %d has an incomplete identity: %+v", event.EventID, event)
			}
			if event.PopID == lastPopID {
				if event.PathOrdinal != lastPathOrdinal+1 {
					t.Fatalf("pop %d path ordinal %d follows %d", event.PopID, event.PathOrdinal, lastPathOrdinal)
				}
			} else if event.PopID <= lastPopID || event.PathOrdinal != 0 {
				t.Fatalf("new pop %d path ordinal %d follows pop %d", event.PopID, event.PathOrdinal, lastPopID)
			}
			lastPopID = event.PopID
			lastPathOrdinal = event.PathOrdinal
		case gts.DiagnosticTopologyEventChildElection, gts.DiagnosticTopologyEventAcceptElection:
			if event.ElectionID != nextElectionID || event.CandidateID == 0 || event.SelectedID == 0 {
				t.Fatalf("election event %d has an incomplete identity: %+v", event.EventID, event)
			}
			if event.Flags&gts.DiagnosticTopologyFlagNoIncumbent == 0 && event.IncumbentID == 0 {
				t.Fatalf("election event %d has no incumbent: %+v", event.EventID, event)
			}
			nextElectionID++
		}
	}
}

func TestDiagnosticTopologyReceiptErlangIssue984(t *testing.T) {
	sexpr, receipt := captureErlangIssue984Topology(t)
	if want := `(source_file (integer) (concatables (string) (var) (string)) (integer) (comment))`; sexpr != want {
		t.Fatalf("production witness tree = %s, want %s", sexpr, want)
	}
	if receipt.Schema != gts.DiagnosticTopologyReceiptSchema || receipt.Capacity != gts.DiagnosticTopologyReceiptCapacity {
		t.Fatalf("receipt identity = %q/%d", receipt.Schema, receipt.Capacity)
	}
	if !receipt.Complete() || receipt.EventsRetained != receipt.EventsSeen || receipt.EventsDropped != 0 {
		t.Fatalf("receipt is incomplete: %+v", receipt)
	}
	assertDiagnosticTopologyEventInvariants(t, receipt)

	counts := make(map[uint64]int)
	anchors := make(map[[4]uint64]bool)
	type actionSliceEntry struct {
		state, actionType uint64
		ordinal           int64
		branch            uint64
	}
	var state320Slice []actionSliceEntry
	state320Started := false
	var acceptEvents []gts.DiagnosticTopologyEvent
	for _, event := range receipt.Events {
		counts[event.Kind]++
		if event.Kind == gts.DiagnosticTopologyEventAction {
			if event.State == 320 && event.ByteOffset == 10 {
				state320Started = true
			}
			if state320Started && event.ByteOffset == 10 {
				state320Slice = append(state320Slice, actionSliceEntry{event.State, event.ActionType, event.ActionOrdinal, event.VersionIndex})
			}
		}
		if event.Kind == gts.DiagnosticTopologyEventAction && event.ActionType == uint64(gts.ParseActionReduce) {
			anchors[[4]uint64{event.State, event.LookaheadSymbol, event.ByteOffset, uint64(event.ActionOrdinal)}] = true
			if (event.State == 320 && event.LookaheadSymbol == 130 && event.ByteOffset == 10) ||
				(event.State == 274 && event.LookaheadSymbol == 133 && event.ByteOffset == 11) {
				t.Logf("anchor event=%d state=%d lookahead=%d byte=%d ordinal=%d version=%d branch=%d", event.EventID, event.State, event.LookaheadSymbol, event.ByteOffset, event.ActionOrdinal, event.VersionID, event.VersionIndex)
			}
		}
		if event.Kind == gts.DiagnosticTopologyEventAcceptElection {
			acceptEvents = append(acceptEvents, event)
		}
	}
	for _, anchor := range [][4]uint64{
		{320, 130, 10, 0},
		{320, 130, 10, 1},
		{274, 133, 11, 0},
		{274, 133, 11, 1},
	} {
		if !anchors[anchor] {
			t.Errorf("missing all-REDUCE anchor state/lookahead/byte/ordinal %v", anchor)
		}
	}
	wantState320Slice := []actionSliceEntry{
		{320, uint64(gts.ParseActionReduce), 1, 1},
		{830, uint64(gts.ParseActionShift), 0, 1},
		{320, uint64(gts.ParseActionReduce), 0, 0},
		{295, uint64(gts.ParseActionReduce), 0, 0},
		{264, uint64(gts.ParseActionReduce), 0, 0},
		{50, uint64(gts.ParseActionShift), 1, 2},
		{50, uint64(gts.ParseActionReduce), 0, 0},
		{44, uint64(gts.ParseActionShift), 0, 0},
	}
	state320Matches := len(state320Slice) == len(wantState320Slice)
	for i := 0; state320Matches && i < len(state320Slice); i++ {
		state320Matches = state320Slice[i] == wantState320Slice[i]
	}
	if !state320Matches {
		t.Fatalf("state-320 frontier slice = %v, want %v", state320Slice, wantState320Slice)
	}
	for _, kind := range []uint64{
		gts.DiagnosticTopologyEventAction,
		gts.DiagnosticTopologyEventVersionAdd,
		gts.DiagnosticTopologyEventVersionCopy,
		gts.DiagnosticTopologyEventMerge,
		gts.DiagnosticTopologyEventLinkInsert,
		gts.DiagnosticTopologyEventPopPath,
		gts.DiagnosticTopologyEventAcceptElection,
	} {
		if counts[kind] == 0 {
			t.Errorf("receipt has no event kind %d", kind)
		}
	}
	if len(acceptEvents) != 4 {
		t.Fatalf("accept elections = %d, want four accepted candidates", len(acceptEvents))
	}
	wantAcceptPayloads := []uint64{1, 1, 1, 1}
	for i := range acceptEvents {
		if acceptEvents[i].PayloadCount != wantAcceptPayloads[i] {
			t.Fatalf("accept payloads = [%d %d %d %d], want %v", acceptEvents[0].PayloadCount, acceptEvents[1].PayloadCount, acceptEvents[2].PayloadCount, acceptEvents[3].PayloadCount, wantAcceptPayloads)
		}
	}
	last := acceptEvents[len(acceptEvents)-1]
	if acceptEvents[0].VersionIndex != 1 || last.SelectedID != acceptEvents[0].CandidateID || last.Flags&gts.DiagnosticTopologyFlagSuccessOrSelected != 0 {
		t.Fatalf("final production selection = %+v, want first branch 1 candidate %d to remain selected", last, acceptEvents[0].CandidateID)
	}
	if receipt.EventsSeen != 387 || receipt.SHA256() != "283e0a0a8405d8f6059787ac441779d234fae30e4a19ca8fb383fc5baa3e89e3" {
		t.Fatalf("canonical receipt = %d/%s", receipt.EventsSeen, receipt.SHA256())
	}
	t.Logf("receipt sha256=%s events=%d kinds=%v selected_candidate=%d", receipt.SHA256(), receipt.EventsSeen, counts, last.SelectedID)

	secondSExpr, second := captureErlangIssue984Topology(t)
	if secondSExpr != sexpr || second.SHA256() != receipt.SHA256() {
		t.Fatalf("receipt is not deterministic: first=%s/%s second=%s/%s", sexpr, receipt.SHA256(), secondSExpr, second.SHA256())
	}
}
