//go:build gts_parsercorephase0_selectedstore_onepass

package parsercorephase0

func (c *Core) BuildSelectedStore(roots []SubtreeID, policy SelectedStorePolicy, source []byte, poll func() error) (*SelectedStore, error) {
	return c.buildSelectedStoreOnePass(roots, policy, source, poll)
}
