package parsercorephase0

import (
	"errors"
	"testing"
)

func TestSingleLexerPrefixInvalidatesSplitsForeignCoreResetAndClear(t *testing.T) {
	c := newTinyCore(t, 8)
	head, err := c.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	var prefix SingleVersionLexerPrefix
	if prefix.AllowsFork(c, 1) {
		t.Fatal("zero prefix authorized a fork")
	}
	prefix.Begin(c, head)
	if !prefix.AllowsFork(c, 1) || prefix.AllowsFork(c, 2) || prefix.AllowsFork(newTinyCore(t, 8), 1) {
		t.Fatal("prefix ownership or width mismatch")
	}
	prefix.ObserveUnownedWidth(c, 2)
	prefix.ObserveUnownedWidth(c, 1)
	if prefix.AllowsFork(c, 1) {
		t.Fatal("returning to one unowned version revived proof")
	}
	prefix.SharedRejoin(c)
	if !prefix.AllowsFork(c, 1) {
		t.Fatal("authenticated rejoin did not reset prefix")
	}
	if err := c.Reset(); err != nil {
		t.Fatal(err)
	}
	if prefix.AllowsFork(c, 1) {
		t.Fatal("reset revived old cursor proof")
	}
	head, err = c.Seed(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	prefix.Begin(c, head)
	if prefix.AllowsFork(c, 1) {
		t.Fatal("noninitial byte claimed deterministic initial prefix")
	}
	prefix.SharedRejoin(c)
	prefix.Clear()
	if prefix.AllowsFork(c, 1) || prefix.core != nil || prefix.generation != 0 {
		t.Fatal("clear retained proof or owner")
	}
}

func TestOwnedLexerSelectionModeRollsBackResetsAndRejectsStaleOwner(t *testing.T) {
	c := newTinyCore(t, 8)
	stop := errors.New("rollback owned lexer mode")
	var stale SchedulerTransactionToken
	err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		stale = owner
		if err := c.SetVersionLexerOwnershipActiveOwned(owner, true); err != nil {
			return err
		}
		return stop
	})
	if !errors.Is(err, stop) || c.schedulerFrame.ownedLexerRequestsActive {
		t.Fatal("rolled-back lexer mode leaked", err)
	}
	if err := c.SetVersionLexerOwnershipActiveOwned(stale, true); err == nil || c.schedulerFrame.ownedLexerRequestsActive {
		t.Fatal("stale owner changed lexer mode")
	}
	generation := c.ResetGeneration()
	if err := c.ApplySchedulerAtomic(func(owner SchedulerTransactionToken) error {
		if err := c.SetVersionLexerOwnershipActiveOwned(owner, true); err != nil {
			return err
		}
		if !c.schedulerFrame.ownedLexerRequestsActive {
			t.Fatal("mode was inactive inside owner")
		}
		return nil
	}); err != nil || c.schedulerFrame.ownedLexerRequestsActive || c.ResetGeneration() != generation {
		t.Fatal("owner release retained mode or required a reset", err)
	}

	if err := c.Reset(); err != nil || c.schedulerFrame.ownedLexerRequestsActive {
		t.Fatal("reset retained owned lexer mode", err)
	}
}
