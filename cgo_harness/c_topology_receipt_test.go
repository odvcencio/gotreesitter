//go:build cgo && treesitter_c_parity && gts_merge_census

package cgoharness

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"testing"
)

const (
	cTopologyEventAction          = 1
	cTopologyEventVersionAdd      = 2
	cTopologyEventVersionCopy     = 3
	cTopologyEventVersionRenumber = 4
	cTopologyEventMerge           = 5
	cTopologyEventLinkInsert      = 6
	cTopologyEventPopPath         = 7
	cTopologyEventChildElection   = 8
	cTopologyEventAcceptElection  = 9

	cTopologyFlagSelected       = 1 << 0
	cTopologyFlagPrimaryLink    = 1 << 1
	cTopologyFlagInitialVersion = 1 << 2
	cTopologyFlagTargetReplaced = 1 << 3
	cTopologyFlagNoIncumbent    = 1 << 4
	cTopologyFlagActionContext  = 1 << 5
	cTopologyKnownFlags         = (1 << 6) - 1

	cTopologyActionReduce  = 1
	cTopologyActionUnknown = 255

	cTopologyErlangIssue984EventCount = 280
	cTopologyErlangIssue984SHA256     = "f90b82a19bd52475a0b61376a631fe53b69ac827c89bec6802940e8302d77754"
)

func TestCTopologyReceiptErlangIssue984(t *testing.T) {
	oracle := mergeCensusOracleForTest(t)
	source := []byte("000\"0A!A \"A\"=0:A0!)A\"0%0000")
	if len(source) != 27 {
		t.Fatalf("source bytes=%d, want 27", len(source))
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != "ad9482f4aabfc3f348a20f6071a2732dcf0473d87b61fa8df4f960e84d4b1ab4" {
		t.Fatalf("source SHA-256=%s", got)
	}
	row, err := mergeCensusRunCTopology(oracle, "erlang", a3CertificationSweepSource{
		Name:   "erlang_issue984_no_newline",
		Source: source,
	})
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "ok" || row.SourceBytes != 27 || row.RootEndByte != 27 {
		t.Fatalf("row status=%q source=%d root_end=%d", row.Status, row.SourceBytes, row.RootEndByte)
	}
	if row.RootHasError || row.RootChildCount != 6 {
		t.Fatalf("root has_error=%v child_count=%d", row.RootHasError, row.RootChildCount)
	}

	receipt := row.Receipt
	if receipt.Capacity != cTopologyEventCapacity {
		t.Fatalf("capacity=%d, want %d", receipt.Capacity, cTopologyEventCapacity)
	}
	if receipt.Truncated || receipt.ArithmeticOverflow || receipt.IdentityCollision || receipt.IdentityIncomplete {
		t.Fatalf("incomplete receipt: %+v", receipt)
	}
	if receipt.EventsDropped != 0 || receipt.EventsSeen != uint64(len(receipt.Events)) || receipt.EventsRetained != uint32(len(receipt.Events)) {
		t.Fatalf(
			"event accounting seen=%d retained=%d dropped=%d len=%d",
			receipt.EventsSeen, receipt.EventsRetained, receipt.EventsDropped, len(receipt.Events),
		)
	}
	if len(receipt.Events) == 0 {
		t.Fatal("empty C topology receipt")
	}
	receiptSHA256 := cTopologyReceiptSHA256(receipt)
	if len(receipt.Events) != cTopologyErlangIssue984EventCount || receiptSHA256 != cTopologyErlangIssue984SHA256 {
		t.Fatalf(
			"C topology receipt: events=%d sha256=%s, want events=%d sha256=%s",
			len(receipt.Events), receiptSHA256,
			cTopologyErlangIssue984EventCount, cTopologyErlangIssue984SHA256,
		)
	}
	t.Logf(
		"C topology receipt: events=%d sha256=%s",
		len(receipt.Events), receiptSHA256,
	)

	kinds := make(map[uint64]int)
	nextVersionID := uint64(1)
	nextLinkID := uint64(1)
	nextElectionID := uint64(1)
	lastActionID := uint64(0)
	lastPopID := uint64(0)
	lastPathOrdinal := uint64(0)
	acceptPayloads := make([]int, 0, 4)
	for index, event := range receipt.Events {
		if event.EventID != uint64(index+1) {
			t.Fatalf("event[%d] id=%d", index, event.EventID)
		}
		if event.Kind < cTopologyEventAction || event.Kind > cTopologyEventAcceptElection {
			t.Fatalf("event[%d] kind=%d", index, event.Kind)
		}
		if event.Flags & ^uint64(cTopologyKnownFlags) != 0 {
			t.Fatalf("event[%d] reserved flags=%#x", index, event.Flags)
		}
		if event.Flags&cTopologyFlagActionContext != 0 {
			if event.ActionID == 0 || event.ActionType > 3 || event.ActionOrdinal < -1 {
				t.Fatalf("event[%d] invalid action context: %+v", index, event)
			}
		} else if event.ActionID != 0 || event.ActionOrdinal != -1 || event.ActionType != cTopologyActionUnknown {
			t.Fatalf("event[%d] leaked action context: %+v", index, event)
		}
		kinds[event.Kind]++

		switch event.Kind {
		case cTopologyEventAction:
			if event.ActionID <= lastActionID {
				t.Fatalf("action id=%d after %d", event.ActionID, lastActionID)
			}
			lastActionID = event.ActionID
		case cTopologyEventVersionAdd, cTopologyEventVersionCopy:
			if event.VersionID != nextVersionID || event.NodeID == 0 {
				t.Fatalf("version allocation id=%d node=%d, want id=%d", event.VersionID, event.NodeID, nextVersionID)
			}
			nextVersionID++
		case cTopologyEventVersionRenumber:
			if event.VersionID == 0 || event.SourceVersionID != event.VersionID || event.TargetVersionID == 0 ||
				event.VersionIndex != event.TargetIndex || event.SourceIndex <= event.TargetIndex || event.NodeID == 0 ||
				event.Flags&cTopologyFlagTargetReplaced == 0 {
				t.Fatalf("invalid renumber event: %+v", event)
			}
		case cTopologyEventMerge:
			if event.SurvivorVersionID == 0 || event.RemovedVersionID == 0 || event.SurvivorVersionID != event.SourceVersionID || event.RemovedVersionID != event.TargetVersionID {
				t.Fatalf("invalid merge orientation: %+v", event)
			}
		case cTopologyEventLinkInsert:
			if event.LinkID != nextLinkID || event.NodeID == 0 || event.PredecessorNodeID == 0 || event.LinkOrdinal >= 8 {
				t.Fatalf("link insert=%+v, want link id=%d", event, nextLinkID)
			}
			nextLinkID++
		case cTopologyEventPopPath:
			if event.PopID == 0 || event.VersionID == 0 || event.SourceVersionID == 0 || event.NodeID == 0 || event.PopToNodeID == 0 {
				t.Fatalf("invalid pop path: %+v", event)
			}
			if event.PopID == lastPopID {
				if event.PathOrdinal != lastPathOrdinal+1 {
					t.Fatalf("pop %d path ordinal=%d after %d", event.PopID, event.PathOrdinal, lastPathOrdinal)
				}
			} else {
				if event.PopID <= lastPopID || event.PathOrdinal != 0 {
					t.Fatalf("new pop id=%d path ordinal=%d after pop=%d", event.PopID, event.PathOrdinal, lastPopID)
				}
			}
			lastPopID = event.PopID
			lastPathOrdinal = event.PathOrdinal
		case cTopologyEventChildElection, cTopologyEventAcceptElection:
			if event.ElectionID != nextElectionID || event.CandidateID == 0 || event.SelectedID == 0 {
				t.Fatalf("election=%+v, want election id=%d", event, nextElectionID)
			}
			if event.Flags&cTopologyFlagNoIncumbent == 0 && event.IncumbentID == 0 {
				t.Fatalf("election has no incumbent identity: %+v", event)
			}
			nextElectionID++
			if event.Kind == cTopologyEventAcceptElection {
				acceptPayloads = append(acceptPayloads, int(event.PayloadCount))
			}
		}
	}

	for _, kind := range []uint64{
		cTopologyEventAction,
		cTopologyEventVersionAdd,
		cTopologyEventVersionRenumber,
		cTopologyEventMerge,
		cTopologyEventLinkInsert,
		cTopologyEventPopPath,
		cTopologyEventChildElection,
		cTopologyEventAcceptElection,
	} {
		if kinds[kind] == 0 {
			t.Errorf("event kind %d is absent", kind)
		}
	}
	if kinds[cTopologyEventVersionCopy] != 0 {
		t.Errorf("version-copy events=%d, want zero for the locked-C witness", kinds[cTopologyEventVersionCopy])
	}
	sort.Ints(acceptPayloads)
	wantAcceptPayloads := []int{3}
	if fmt.Sprint(acceptPayloads) != fmt.Sprint(wantAcceptPayloads) {
		t.Errorf("accept election payload counts=%v, want %v", acceptPayloads, wantAcceptPayloads)
	}

	assertCTopologyReduceAnchor(t, receipt.Events, 320, 130, 10, 1)
	assertCTopologyReduceAnchor(t, receipt.Events, 274, 133, 11, 1)
}

func cTopologyReceiptSHA256(receipt cTopologyReceipt) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte("gts-topology-receipt/v1\x00"))
	writeUint64 := func(value uint64) {
		var encoded [8]byte
		binary.LittleEndian.PutUint64(encoded[:], value)
		_, _ = digest.Write(encoded[:])
	}
	writeBool := func(value bool) {
		if value {
			writeUint64(1)
		} else {
			writeUint64(0)
		}
	}

	writeUint64(uint64(receipt.Capacity))
	writeUint64(receipt.EventsSeen)
	writeUint64(uint64(receipt.EventsRetained))
	writeUint64(receipt.EventsDropped)
	writeBool(receipt.Truncated)
	writeBool(receipt.ArithmeticOverflow)
	writeBool(receipt.IdentityCollision)
	writeBool(receipt.IdentityIncomplete)
	for _, event := range receipt.Events {
		writeUint64(event.EventID)
		writeUint64(event.Kind)
		writeUint64(event.ActionID)
		writeUint64(uint64(event.ActionOrdinal))
		writeUint64(event.ActionType)
		writeUint64(event.State)
		writeUint64(event.LookaheadSymbol)
		writeUint64(event.ByteOffset)
		writeUint64(event.VersionID)
		writeUint64(event.VersionIndex)
		writeUint64(event.SourceVersionID)
		writeUint64(event.SourceIndex)
		writeUint64(event.TargetVersionID)
		writeUint64(event.TargetIndex)
		writeUint64(event.SurvivorVersionID)
		writeUint64(event.RemovedVersionID)
		writeUint64(event.NodeID)
		writeUint64(event.PredecessorNodeID)
		writeUint64(event.LinkID)
		writeUint64(event.LinkOrdinal)
		writeUint64(event.PopID)
		writeUint64(event.PathOrdinal)
		writeUint64(event.PopToNodeID)
		writeUint64(event.ElectionID)
		writeUint64(event.IncumbentID)
		writeUint64(event.CandidateID)
		writeUint64(event.SelectedID)
		writeUint64(event.PayloadCount)
		writeUint64(event.Flags)
	}
	return fmt.Sprintf("%x", digest.Sum(nil))
}

func assertCTopologyReduceAnchor(
	t *testing.T,
	events []cTopologyEvent,
	state uint64,
	lookahead uint64,
	byteOffset uint64,
	wantVersions int,
) {
	t.Helper()
	ordinalsByVersion := make(map[uint64][]int64)
	for _, event := range events {
		if event.Kind != cTopologyEventAction || event.State != state || event.LookaheadSymbol != lookahead || event.ByteOffset != byteOffset {
			continue
		}
		if event.ActionType != cTopologyActionReduce {
			t.Fatalf("anchor state=%d lookahead=%d byte=%d has action type=%d", state, lookahead, byteOffset, event.ActionType)
		}
		ordinalsByVersion[event.VersionID] = append(ordinalsByVersion[event.VersionID], event.ActionOrdinal)
	}
	if len(ordinalsByVersion) != wantVersions {
		t.Fatalf(
			"anchor state=%d lookahead=%d byte=%d versions=%d, want %d: %v",
			state, lookahead, byteOffset, len(ordinalsByVersion), wantVersions, ordinalsByVersion,
		)
	}
	for versionID, ordinals := range ordinalsByVersion {
		if fmt.Sprint(ordinals) != "[0 1]" {
			t.Errorf("anchor version=%d ordinals=%v, want [0 1]", versionID, ordinals)
		}
	}
}
