package parsercorephase0

// reservePublicCapacity replaces repeated large result-array copies with one
// initial reserve. This is only a capacity hint: projection still authenticates
// every occurrence and retains its normal checked growth when the hint is short.
// Cached output backing and the largest observed raw count avoid this census on
// repeated parses. Aliases and discarded derivations may overestimate the hint.
func (s *NativeSelectedScratch) reservePublicCapacity(c *Core, poll func() error) error {
	if c.selectedPolicy == nil {
		if c.selectedProvider == nil {
			return nil
		}
		policy, err := c.selectedProvider.SelectedStorePolicy()
		if err != nil {
			return err
		}
		c.selectedPolicy = &policy
	}
	count := uint64(1) // Leave room for an extra-sibling root.
	for i := 0; i < c.subtrees.Len(); i++ {
		if i&255 == 0 {
			if err := poll(); err != nil {
				return err
			}
		}
		r := c.subtrees.At(i)
		policy, valid := c.selectedPolicy.symbol(r.symbol)
		if valid && policy.Visible {
			count++
		}
		if uint64(r.firstAlias)+uint64(r.aliasCount) > uint64(len(c.aliases)) {
			// Projection reports malformed metadata; sizing must not index it.
			return nil
		}
		if r.aliasCount != 0 && (r.aliasCount != r.childCount || uint64(r.firstChild)+uint64(r.childCount) > uint64(len(c.children))) {
			return nil
		}
		for ordinal, alias := range c.aliases[r.firstAlias : r.firstAlias+r.aliasCount] {
			meta, valid := c.selectedPolicy.symbol(alias)
			if alias == 0 || !valid || !meta.Visible {
				continue
			}
			child, err := c.subtree(c.children[int(r.firstChild)+ordinal])
			if err != nil {
				return nil
			}
			original, valid := c.selectedPolicy.symbol(child.symbol)
			// Renaming a visible child adds no occurrence. Aliasing a hidden
			// child, or retaining the original beneath a wrapper, can add one.
			if !valid || !original.Visible || c.selectedPolicy.retainsAliasChild(alias, child.symbol) {
				count++
			}
		}
		if count >= uint64(c.limits.MaxSelectedOccurrences) {
			count = uint64(c.limits.MaxSelectedOccurrences)
			break
		}
	}
	if count > uint64(^uint(0)>>1) {
		return nil
	}
	store := s.store
	records, children, replay := max(cap(store.records), int(count)), max(cap(store.children), int(count)), max(cap(s.replay), int(count))
	extra := nativeSelectedReplayBytes(replay)
	// A conservative hint cannot make an otherwise valid tight budget fail.
	// Let the existing exact-on-pressure growth path decide such cases.
	if extra > c.limits.MaxSelectedBytes || store.retainedBytesForCapacity(records, children) > c.limits.MaxSelectedBytes-extra {
		return nil
	}
	if err := rightSizeSelectedOnePassStore(store, records, children, c.limits.MaxSelectedBytes-extra, poll); err != nil {
		return err
	}
	if replay > cap(s.replay) {
		next := make([]nativeSelectedReplay, len(s.replay), replay)
		if err := copySelectedStoreChunks(next, s.replay, poll); err != nil {
			return err
		}
		s.replay = next
	}
	return poll()
}
