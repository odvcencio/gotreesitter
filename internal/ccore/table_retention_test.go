package ccore

import "testing"

func TestStoreReleasesTableIdentityAfterLastTree(t *testing.T) {
	c := newStoreCore()
	tables := &Tables{}
	c.parseTables = tables
	tree := &Tree{c: c.newCell(Handle(inlInline)), rd: c.view()}
	tree.rd.owner = tree.c
	copy := tree.Copy()
	if copy == nil {
		t.Fatal("copy of live tree returned nil")
	}

	tree.Release()
	c.lease.Lock()
	retained := c.parseTables
	c.lease.Unlock()
	if retained != tables {
		t.Fatal("store lost table identity while copy is live")
	}

	copy.Release()
	c.lease.Lock()
	retained = c.parseTables
	c.lease.Unlock()
	if retained != nil {
		t.Fatal("ended store retains its table identity")
	}
}
