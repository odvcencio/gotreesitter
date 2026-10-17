//go:build !gts_parsercorephase0_selectedstore_onepass

package parsercorephase0

// BuildSelectedStore seals accepted roots directly from compact immutable
// payload arenas. It does not enumerate a head, construct public Nodes, or
// invoke a per-occurrence callback. It borrows Core-owned scratch and therefore
// must not run concurrently with any other operation on the same Core.
func (c *Core) BuildSelectedStore(roots []SubtreeID, policy SelectedStorePolicy, source []byte, poll func() error) (*SelectedStore, error) {
	return c.buildSelectedStoreOnePass(roots, policy, source, poll)
}
