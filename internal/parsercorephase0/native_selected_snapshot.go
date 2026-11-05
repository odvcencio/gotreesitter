package parsercorephase0

// NativeSelectedSnapshot owns both the public occurrence tree and the replay
// facts that authenticate those occurrences. It has no reference to Core or
// to the scratch used to produce it. A public adapter can therefore construct
// node views after the producing parser has been reset or collected.
type NativeSelectedSnapshot struct {
	store  *SelectedStore
	replay []nativeSelectedReplay
	owner  *selectedStorePool
}

// BuildNativeSelectedSnapshot transfers replay storage along with the syntax
// records. BuildNativeSelectedOutput remains the smaller read-only interface.
func (c *Core) BuildNativeSelectedSnapshot(roots []SubtreeID, source []byte, replay NativeProjectionReplay, scratch *NativeSelectedScratch, poll func() error) (NativeSelectedSnapshot, error) {
	if scratch != nil {
		previous := scratch.reservePublicOutput
		scratch.reservePublicOutput = true
		defer func() { scratch.reservePublicOutput = previous }()
	}
	if c != nil && scratch != nil && scratch.store == nil && cap(scratch.replay) == 0 {
		scratch.replay = c.selectedStoreOwner().takeReplay()
	}
	store, err := c.BuildNativeSelectedOutput(roots, source, replay, scratch, poll)
	if err != nil {
		return NativeSelectedSnapshot{}, err
	}
	facts := scratch.replay[:len(store.records)]
	scratch.replay = nil
	return NativeSelectedSnapshot{store: store, replay: facts, owner: store.owner}, nil
}

func (s *NativeSelectedSnapshot) Store() *SelectedStore {
	if s == nil {
		return nil
	}
	return s.store
}

// Record combines final public geometry with the original replay facts.
// Synthetic roots deliberately have no replay proof. Compatibility changes
// retain their original projection symbol for receipt authentication.
func (s *NativeSelectedSnapshot) Record(id SelectedNodeID) (NativeProjectionRecord, Symbol, bool) {
	if s == nil || s.store == nil || id == 0 || uint64(id) > uint64(len(s.replay)) {
		return NativeProjectionRecord{}, 0, false
	}
	r, ok := s.store.Record(id)
	if !ok {
		return NativeProjectionRecord{}, 0, false
	}
	f := s.replay[id-1]
	source, _ := s.store.PayloadSource(id)
	if source == 0 {
		f = nativeSelectedReplay{}
	}
	flags := uint16(r.flags &^ selectedNodeFlagFragile)
	flags |= f.flags & (nativeProjectionPreKnown | nativeProjectionStateKnown)
	if r.Fragile() {
		flags |= nativeProjectionFragile
	}
	return NativeProjectionRecord{Source: source, Symbol: r.Symbol, Field: r.Field,
		ProductionID: r.ProductionID, StartByte: r.StartByte, EndByte: r.EndByte,
		ChildCount: uint32(r.ChildCount), DynamicPrecedence: r.DynamicPrecedence,
		SupertypeMask: r.SupertypeMask, ReplayPre: f.pre, ReplayState: f.state, Flags: flags}, f.symbol, true
}

func (s *NativeSelectedSnapshot) RetainedBytes() uint64 {
	if s == nil || s.store == nil || s.store.released {
		return 0
	}
	return s.store.RetainedBytes() + nativeSelectedReplayBytes(cap(s.replay))
}

func (s *NativeSelectedSnapshot) Release() {
	if s == nil {
		return
	}
	owned := s.store != nil && !s.store.released
	s.store.Release()
	if owned {
		s.owner.recycleReplay(s.replay)
	}
	s.store, s.replay = nil, nil
	s.owner = nil
}
