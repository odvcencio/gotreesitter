package parsercorephase0

// SingleVersionLexerPrefix tracks a shared cursor reached without an unowned
// multi-version frontier. A validated owned-to-shared rejoin starts a new
// single-version prefix. Observing an unowned split can only invalidate it.
// The root calls SharedRejoin only after its existing owned snapshot restore,
// destination, checkpoint and sole shifted-header checks have all succeeded.
type SingleVersionLexerPrefix struct {
	core       *Core
	generation uint64
	valid      bool
}

func (p *SingleVersionLexerPrefix) Clear() { *p = SingleVersionLexerPrefix{} }
func (p *SingleVersionLexerPrefix) Begin(c *Core, head Head) {
	p.Clear()
	if c == nil || c.nodes.Len() != 1 || head.Node != 1 {
		return
	}
	node := c.nodes.At(0)
	paths, err := c.HeadExactPathCount(head)
	if err != nil || paths != 1 || node == nil || node.firstLink != 0 || node.linkCount != 0 || node.byteOffset != 0 {
		return
	}
	p.SharedRejoin(c)
}
func (p *SingleVersionLexerPrefix) AllowsFork(c *Core, width int) bool {
	return p.valid && width == 1 && c != nil && p.core == c && p.generation != 0 && p.generation == c.ResetGeneration()
}
func (p *SingleVersionLexerPrefix) ObserveUnownedWidth(c *Core, width int) {
	if !p.AllowsFork(c, width) {
		p.valid = false
	}
}
func (p *SingleVersionLexerPrefix) SharedRejoin(c *Core) {
	p.Clear()
	if c != nil {
		p.core = c
		p.generation = c.ResetGeneration()
		p.valid = p.generation != 0
	}
}

// SetVersionLexerOwnershipActiveOwned names real owned-request execution for
// reduction selection; it never follows from a diagnostic switch alone.
func (c *Core) SetVersionLexerOwnershipActiveOwned(owner SchedulerTransactionToken, active bool) (err error) {
	if err = c.beginSchedulerOwned(owner); err != nil {
		return err
	}
	defer c.recoverSchedulerOwnedPanic(owner)
	c.schedulerFrame.ownedLexerRequestsActive = active
	return c.finishSchedulerOwned(owner, nil)
}

func (c *Core) VersionLexerOwnershipActive() bool {
	return c != nil && c.schedulerFrame.active && c.schedulerFrame.ownedLexerRequestsActive
}
