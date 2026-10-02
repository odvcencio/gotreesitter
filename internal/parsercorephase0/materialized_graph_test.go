package parsercorephase0

import (
	"errors"
	"testing"
)

func TestMaterializationReleasesGraphButPreservesSyntax(t *testing.T) {
	c := reserveTestCore(t, reserveTestLimits())
	head, err := c.Seed(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	head, err = c.appendDiagnosticPayload(head, 1, Token{Symbol: 1, EndByte: 1}, pathMeta{})
	if err != nil {
		t.Fatal(err)
	}
	stats, err := c.Stats(head)
	if err != nil {
		t.Fatal(err)
	}
	syntax := c.subtrees[0]
	c.nodes = growArena(c.nodes, int(coreRetentionCapBytes/coreNodeRecordBytes)+1)
	if err := c.ReleaseStackGraphForMaterialization(); err != nil {
		t.Fatal(err)
	}
	if c.nodes != nil || c.nodeLineages != nil || c.links != nil {
		t.Fatal("materialization retained the stack graph")
	}
	if len(c.subtrees) != 1 || c.subtrees[0] != syntax || stats.Nodes == 0 {
		t.Fatal("graph release changed accepted syntax or copied census")
	}
	if err := c.Reset(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Seed(0, 0); err != nil {
		t.Fatalf("reset did not restore scheduling: %v", err)
	}
}

func TestMaterializationDoesNotReleaseAnOwnedTransaction(t *testing.T) {
	c := reserveTestCore(t, reserveTestLimits())
	if _, err := c.Seed(0, 0); err != nil {
		t.Fatal(err)
	}
	c.nodes = growArena(c.nodes, int(coreRetentionCapBytes/coreNodeRecordBytes)+1)
	decline := errors.New("rollback")
	if err := c.ApplySchedulerAtomic(func(SchedulerTransactionToken) error {
		if err := c.ReleaseStackGraphForMaterialization(); err == nil {
			t.Fatal("released graph during an owned transaction")
		}
		if len(c.nodes) != 1 {
			t.Fatal("failed release changed the graph")
		}
		return decline
	}); !errors.Is(err, decline) {
		t.Fatal(err)
	}
}
