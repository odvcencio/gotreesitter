package parsercorephase0

import (
	"errors"
	"math"
	"unsafe"
)

// NativeSelectedScratch holds only replay facts used while constructing a
// compact result. Readers consume the sealed SelectedStore directly. No Node
// tree or second projection is built, and no replay state is inferred from a
// renamed/aliased public symbol.
type NativeSelectedScratch struct {
	replay                  []nativeSelectedReplay
	store                   *SelectedStore
	core                    *Core
	poll                    func() error
	reservePublicOutput     bool
	reserveObservedRawCount int
}
type nativeSelectedReplay struct {
	pre, state StateID
	flags      uint16
	symbol     Symbol
}

func nativeSelectedReplayBytes(capacity int) uint64 {
	return uint64(capacity) * uint64(unsafe.Sizeof(nativeSelectedReplay{}))
}

func (s *NativeSelectedScratch) RetainedBytes() uint64 {
	if s == nil {
		return 0
	}
	total := uint64(cap(s.replay)) * uint64(unsafe.Sizeof(nativeSelectedReplay{}))
	if s.store != nil {
		total += s.store.RetainedBytes()
	}
	return total
}

// BuildNativeSelectedOutput shares the authenticated native projector with the
// public adapter, but writes pointer-free records directly to the reader's
// backing. Clean grammar roots and their extra siblings are admitted. Recovery
// retains its existing publication path.
func (c *Core) BuildNativeSelectedOutput(roots []SubtreeID, source []byte, replay NativeProjectionReplay, scratch *NativeSelectedScratch, poll func() error) (result *SelectedStore, err error) {
	if c == nil || scratch == nil || scratch.store != nil || !c.TableIdentityMatches() {
		return nil, errors.New("parser-core phase zero: invalid native selected owner")
	}
	if poll == nil {
		poll = func() error { return nil }
	}
	store := c.acquireSelectedStoreWithSources(0, 0, true)
	scratch.store, scratch.core, scratch.poll = store, c, poll
	scratch.replay = scratch.replay[:0]
	defer func() {
		scratch.store, scratch.core, scratch.poll = nil, nil, nil
		scratch.replay = scratch.replay[:0]
		if err != nil {
			store.Release()
		}
	}()
	if scratch.reservePublicOutput && c.subtrees.Len() >= 1<<16 && c.subtrees.Len() > scratch.reserveObservedRawCount {
		if err = scratch.reservePublicCapacity(c, poll); err != nil {
			return nil, err
		}
		scratch.reserveObservedRawCount = c.subtrees.Len()
	}
	ids, err := c.StreamNativeProjection(roots, source, scratch, replay, poll)
	if err != nil {
		return nil, err
	}
	if c.selectedPolicy == nil {
		return nil, errors.New("parser-core phase zero: native selected policy missing")
	}
	// Root framing uses the same field-aware synthetic-root rule as the
	// selected reader. Native projection already computed total precedence;
	// only a newly framed synthetic root needs a new sum.
	var deltas []int32
	if len(ids) > 1 {
		// Only sibling-root framing can rewrite a precedence delta. A normal
		// root already carries the authenticated total from native projection.
		deltas = resizeSelectedScratch(c.selectedBuild.precedence, len(store.records))
		c.selectedBuild.precedence = deltas
	}
	extra := uint64(cap(scratch.replay)) * uint64(unsafe.Sizeof(nativeSelectedReplay{}))
	if extra > c.limits.MaxSelectedBytes {
		return nil, errors.New("parser-core phase zero: native selected replay cap")
	}
	if err = store.finishRoots(ids, *c.selectedPolicy, deltas, c.limits.MaxSelectedBytes-extra, poll); err != nil {
		return nil, err
	}
	root := &store.records[store.root-1]
	if root.HasError() {
		return nil, errors.New("parser-core phase zero: native selected output requires a clean root")
	}
	if store.payloadSources[store.root-1] == 0 {
		root.DynamicPrecedence = 0
		for _, id := range store.children[root.FirstChild : root.FirstChild+uint32(root.ChildCount)] {
			root.DynamicPrecedence, err = checkedSelectedPrecedence(root.DynamicPrecedence, store.records[id-1].DynamicPrecedence)
			if err != nil {
				return nil, err
			}
		}
	}
	if err = store.normalizeGoCompatibility(*c.selectedPolicy, source, poll); err != nil {
		return nil, err
	}
	if err = store.normalizeResultRootSourceStart(*c.selectedPolicy, source); err != nil {
		return nil, err
	}
	if err = poll(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *NativeSelectedScratch) Emit(r NativeProjectionRecord, children []SelectedNodeID) (SelectedNodeID, error) {
	store := s.store
	if r.ChildCount != uint32(len(children)) || len(children) > math.MaxUint16 || uint64(len(store.records))+1 > uint64(s.core.limits.MaxSelectedOccurrences) || uint64(len(store.children))+uint64(len(children)) > math.MaxUint32 {
		return 0, errors.New("parser-core phase zero: native selected occurrence cap")
	}
	need := len(store.records) + 1
	// Charge replay scratch beside the output reserve, including geometric
	// growth, before either allocation. Tight budgets use exact capacity.
	capacity := selectedOnePassGrowth(cap(s.replay), need)
	extra := uint64(capacity) * uint64(unsafe.Sizeof(nativeSelectedReplay{}))
	if extra > s.core.limits.MaxSelectedBytes {
		return 0, errors.New("parser-core phase zero: native selected replay cap")
	}
	if err := ensureSelectedOnePassCapacity(store, need, len(store.children)+len(children), s.core.limits.MaxSelectedBytes-extra, s.poll); err != nil {
		return 0, err
	}
	if capacity > cap(s.replay) {
		next := make([]nativeSelectedReplay, len(s.replay), capacity)
		if err := copySelectedStoreChunks(next, s.replay, s.poll); err != nil {
			return 0, err
		}
		s.replay = next
	}
	id := SelectedNodeID(need)
	for i, child := range children {
		if child == 0 || uint64(child) > uint64(len(store.records)) {
			return 0, errors.New("parser-core phase zero: invalid native selected child")
		}
		record := &store.records[child-1]
		if record.Parent != 0 {
			return 0, errors.New("parser-core phase zero: native selected occurrence has two parents")
		}
		record.Parent, record.ChildIndex = id, uint16(i)
	}
	store.records = append(store.records, SelectedNodeRecord{FirstChild: uint32(len(store.children)), ChildCount: uint16(len(children))})
	store.children = append(store.children, children...)
	store.payloadSources = append(store.payloadSources, r.Source)
	s.replay = append(s.replay, nativeSelectedReplay{})
	if err := s.Rewrite(id, r); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *NativeSelectedScratch) Read(id SelectedNodeID) (NativeProjectionRecord, error) {
	r, ok := s.store.record(id)
	if !ok {
		return NativeProjectionRecord{}, errors.New("parser-core phase zero: invalid native selected handle")
	}
	replay := s.replay[id-1]
	return NativeProjectionRecord{
		ReplayPre: replay.pre, ReplayState: replay.state, Source: s.store.payloadSources[id-1],
		StartByte: r.StartByte, EndByte: r.EndByte, DynamicPrecedence: r.DynamicPrecedence,
		SupertypeMask: r.SupertypeMask, ChildCount: uint32(r.ChildCount), Symbol: r.Symbol,
		Field: r.Field, ProductionID: r.ProductionID, Flags: replay.flags,
	}, nil
}

func (s *NativeSelectedScratch) Rewrite(id SelectedNodeID, r NativeProjectionRecord) error {
	node, ok := s.store.record(id)
	if !ok || uint32(node.ChildCount) != r.ChildCount {
		return errors.New("parser-core phase zero: native selected rewrite changed children")
	}
	node.StartByte, node.EndByte = r.StartByte, r.EndByte
	node.DynamicPrecedence, node.SupertypeMask = r.DynamicPrecedence, r.SupertypeMask
	node.Symbol, node.Field, node.ProductionID = r.Symbol, r.Field, r.ProductionID
	node.flags = uint8(r.Flags) & (selectedNodeFlagNamed | selectedNodeFlagExtra | selectedNodeFlagExternal | selectedNodeFlagTerminal | selectedNodeFlagMissing | selectedNodeFlagHasError)
	if r.Fragile() {
		node.flags |= selectedNodeFlagFragile
	}
	s.store.payloadSources[id-1] = r.Source
	s.replay[id-1] = nativeSelectedReplay{pre: r.ReplayPre, state: r.ReplayState, flags: r.Flags, symbol: r.Symbol}
	return nil
}
