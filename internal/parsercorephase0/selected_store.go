package parsercorephase0

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"unsafe"
)

// SelectedSymbolPolicy is immutable materialization metadata for one grammar
// symbol. The authenticated table adapter builds it lazily only when the
// selected-store boundary is requested; the Core then caches it.
type SelectedSymbolPolicy struct {
	Visible bool
	Named   bool
}

// SelectedUnaryRule is the exact clean-tree unary reduction decision for one
// parent/child symbol pair.
type SelectedUnaryRule uint8

const (
	SelectedUnaryKeep SelectedUnaryRule = iota
	SelectedUnaryPass
	SelectedUnaryRenameLeaf
)

// SelectedAliasChildPair is one compiled occurrence fact requiring an alias to
// retain its raw child instead of relabeling that child.
type SelectedAliasChildPair struct {
	Alias Symbol
	Child Symbol
}

// SelectedStorePolicy is the fail-closed compact-to-consumer contract. Unary
// is a dense parent-major matrix with stride len(Symbols).
type SelectedStorePolicy struct {
	Symbols               []SelectedSymbolPolicy
	Unary                 []SelectedUnaryRule
	RetainedAliasChildren []SelectedAliasChildPair
	ExpectedRoot          Symbol
	Semicolon             Symbol
	SemicolonNUL          Symbol
	SemicolonContainers   []bool
	Cases                 []bool
	StatementLists        []bool
}

// SetRetainedAliasChildren installs parser-compiled symbol facts. The compact
// core deliberately receives no language names or source-text policy.
func (p *SelectedStorePolicy) SetRetainedAliasChildren(edges []SelectedAliasChildPair) {
	if p == nil {
		return
	}
	p.RetainedAliasChildren = append(p.RetainedAliasChildren[:0], edges...)
}

// SelectedStorePolicyProvider lets an authenticated table adapter build the
// compact-to-consumer policy lazily on first selected-store use.
type SelectedStorePolicyProvider interface {
	SelectedStorePolicy() (SelectedStorePolicy, error)
}

func NewSelectedStorePolicy(symbols []SelectedSymbolPolicy, unary []SelectedUnaryRule, expectedRoot Symbol) (SelectedStorePolicy, error) {
	if len(symbols) == 0 || int(expectedRoot) >= len(symbols) {
		return SelectedStorePolicy{}, errors.New("parser-core phase zero: selected-store policy has no authenticated root")
	}
	want := len(symbols) * len(symbols)
	if (len(symbols) != 0 && want/len(symbols) != len(symbols)) || len(unary) != want {
		return SelectedStorePolicy{}, errors.New("parser-core phase zero: selected-store unary policy width drifted")
	}
	return SelectedStorePolicy{
		Symbols:      append([]SelectedSymbolPolicy(nil), symbols...),
		Unary:        append([]SelectedUnaryRule(nil), unary...),
		ExpectedRoot: expectedRoot,
	}, nil
}

func (p *SelectedStorePolicy) SetGoCompatibility(semicolon, semicolonNUL Symbol, containers, cases, statementLists []bool) error {
	if p == nil || len(p.Symbols) == 0 || len(containers) != len(p.Symbols) || len(cases) != len(p.Symbols) || len(statementLists) != len(p.Symbols) {
		return errors.New("parser-core phase zero: selected-store Go compatibility width drifted")
	}
	p.Semicolon, p.SemicolonNUL = semicolon, semicolonNUL
	p.SemicolonContainers = append([]bool(nil), containers...)
	p.Cases = append([]bool(nil), cases...)
	p.StatementLists = append([]bool(nil), statementLists...)
	return nil
}

func (p SelectedStorePolicy) symbol(symbol Symbol) (SelectedSymbolPolicy, bool) {
	if int(symbol) >= len(p.Symbols) {
		return SelectedSymbolPolicy{}, false
	}
	return p.Symbols[symbol], true
}

func (p SelectedStorePolicy) unary(parent, child Symbol) SelectedUnaryRule {
	stride := len(p.Symbols)
	if int(parent) >= stride || int(child) >= stride {
		return SelectedUnaryKeep
	}
	slot := int(parent)*stride + int(child)
	if slot >= len(p.Unary) {
		return SelectedUnaryKeep
	}
	return p.Unary[slot]
}

func (p SelectedStorePolicy) retainsAliasChild(alias, child Symbol) bool {
	if alias == 0 {
		return false
	}
	for _, edge := range p.RetainedAliasChildren {
		if edge.Alias == alias && edge.Child == child {
			return true
		}
	}
	return false
}

func (p SelectedStorePolicy) validate() error {
	if len(p.Symbols) == 0 || int(p.ExpectedRoot) >= len(p.Symbols) {
		return errors.New("parser-core phase zero: selected-store policy has no authenticated root")
	}
	width := len(p.Symbols)
	if width > math.MaxInt/width || len(p.Unary) != width*width {
		return errors.New("parser-core phase zero: selected-store unary policy width drifted")
	}
	for _, pair := range p.RetainedAliasChildren {
		if pair.Alias == 0 || int(pair.Alias) >= width || int(pair.Child) >= width || !p.Symbols[pair.Alias].Visible {
			return errors.New("parser-core phase zero: selected-store retained-alias policy is invalid")
		}
	}
	compatWidth := len(p.SemicolonContainers)
	if compatWidth == 0 {
		if len(p.Cases) != 0 || len(p.StatementLists) != 0 {
			return errors.New("parser-core phase zero: selected-store compatibility policy is partial")
		}
		return nil
	}
	if compatWidth != width || len(p.Cases) != width || len(p.StatementLists) != width {
		return errors.New("parser-core phase zero: selected-store compatibility width drifted")
	}
	return nil
}

type SelectedNodeID uint32

const (
	selectedNodeFlagNamed uint8 = 1 << iota
	selectedNodeFlagExtra
	selectedNodeFlagExternal
	selectedNodeFlagTerminal
	// selectedNodeFlagMissing mirrors subtreeRecord.missing (B3 stage S5
	// substrate). Without it the SelectedStore projection would answer
	// differently from the postorder public tree for the same parse: the
	// public tree sets the node's missing and has-error bits, the projection
	// would report an ordinary zero-width token.
	selectedNodeFlagMissing
	selectedNodeFlagHasError
)

// SelectedNodeRecord is one immutable logical occurrence after hidden-parent
// elision and unary collapse. Field belongs to the incoming logical edge.
type SelectedNodeRecord struct {
	Parent            SelectedNodeID
	FirstChild        uint32
	StartByte         uint32
	EndByte           uint32
	Symbol            Symbol
	Field             FieldID
	ProductionID      uint16
	DynamicPrecedence int32
	ChildCount        uint16
	ChildIndex        uint16
	flags             uint8
}

func (r SelectedNodeRecord) Named() bool    { return r.flags&selectedNodeFlagNamed != 0 }
func (r SelectedNodeRecord) Extra() bool    { return r.flags&selectedNodeFlagExtra != 0 }
func (r SelectedNodeRecord) External() bool { return r.flags&selectedNodeFlagExternal != 0 }
func (r SelectedNodeRecord) Terminal() bool { return r.flags&selectedNodeFlagTerminal != 0 }

// Missing reports a recovery-inserted zero-width terminal (C ts_subtree_missing).
func (r SelectedNodeRecord) Missing() bool { return r.flags&selectedNodeFlagMissing != 0 }

// HasError reports recovery content on this occurrence or its descendants.
func (r SelectedNodeRecord) HasError() bool { return r.flags&selectedNodeFlagHasError != 0 }

// SelectedStore is the sealed, pointer-free selected syntax backing store.
// IDs are occurrence identities; repeated compact SubtreeIDs remain distinct.
type SelectedStore struct {
	root     SelectedNodeID
	records  []SelectedNodeRecord
	children []SelectedNodeID
	owner    *Core
	released bool
}

type selectedStoreBacking struct {
	records  []SelectedNodeRecord
	children []SelectedNodeID
}

type selectedStoreBuildScratch struct {
	raw            []selectedRawOccurrence
	rawChildren    []uint32
	rawRoots       []uint32
	results        []SelectedNodeID
	resultHasError []bool
	precedence     []int32
	collect        []uint32
	logical        []SelectedNodeID
	rootIDs        []SelectedNodeID
	stack          []selectedOccurrenceFrame
}

type selectedOccurrenceFrame struct {
	occurrence uint32
	next       uint32
	hasError   bool
}

func (s *SelectedStore) Root() SelectedNodeID {
	if s == nil || s.released {
		return 0
	}
	return s.root
}

func (s *SelectedStore) NodeCount() uint64 {
	if s == nil || s.released {
		return 0
	}
	return uint64(len(s.records))
}

func (s *SelectedStore) ChildCount() uint64 {
	if s == nil || s.released {
		return 0
	}
	return uint64(len(s.children))
}

// RetainedBytes reports exact backing-array retention, excluding the small
// store header itself so capacity changes remain visible.
func (s *SelectedStore) RetainedBytes() uint64 {
	if s == nil || s.released {
		return 0
	}
	return selectedStoreRetainedBytes(cap(s.records), cap(s.children))
}

func selectedStoreRetainedBytes(recordCapacity, childCapacity int) uint64 {
	return uint64(recordCapacity)*uint64(unsafe.Sizeof(SelectedNodeRecord{})) +
		uint64(childCapacity)*uint64(unsafe.Sizeof(SelectedNodeID(0)))
}

func (s *SelectedStore) record(id SelectedNodeID) (*SelectedNodeRecord, bool) {
	if s == nil || s.released || id == 0 || uint64(id) > uint64(len(s.records)) {
		return nil, false
	}
	return &s.records[id-1], true
}

func (s *SelectedStore) Record(id SelectedNodeID) (SelectedNodeRecord, bool) {
	record, ok := s.record(id)
	if !ok {
		return SelectedNodeRecord{}, false
	}
	return *record, true
}

func (s *SelectedStore) NodeSymbol(id SelectedNodeID) (Symbol, bool) {
	record, ok := s.record(id)
	if !ok {
		return 0, false
	}
	return record.Symbol, true
}

func (s *SelectedStore) NodeNamed(id SelectedNodeID) (bool, bool) {
	record, ok := s.record(id)
	if !ok {
		return false, false
	}
	return record.Named(), true
}

func (s *SelectedStore) NodeRange(id SelectedNodeID) (uint32, uint32, bool) {
	record, ok := s.record(id)
	if !ok {
		return 0, 0, false
	}
	return record.StartByte, record.EndByte, true
}

func (s *SelectedStore) NodeParent(id SelectedNodeID) (SelectedNodeID, bool) {
	record, ok := s.record(id)
	if !ok || record.Parent == 0 {
		return 0, false
	}
	return record.Parent, true
}

func (s *SelectedStore) NodeChildCount(id SelectedNodeID) (uint16, bool) {
	record, ok := s.record(id)
	if !ok {
		return 0, false
	}
	return record.ChildCount, true
}

func (s *SelectedStore) NodeField(id SelectedNodeID) (FieldID, bool) {
	record, ok := s.record(id)
	if !ok {
		return 0, false
	}
	return record.Field, true
}

func (s *SelectedStore) ChildID(parent SelectedNodeID, index uint32) (SelectedNodeID, bool) {
	record, ok := s.record(parent)
	if !ok {
		return 0, false
	}
	return s.child(record, index)
}

func (s *SelectedStore) Child(record SelectedNodeRecord, index uint32) (SelectedNodeID, bool) {
	return s.child(&record, index)
}

func (s *SelectedStore) child(record *SelectedNodeRecord, index uint32) (SelectedNodeID, bool) {
	if s == nil || s.released || record == nil || index >= uint32(record.ChildCount) {
		return 0, false
	}
	slot := uint64(record.FirstChild) + uint64(index)
	if slot >= uint64(len(s.children)) {
		return 0, false
	}
	return s.children[slot], true
}

// Release invalidates the selected snapshot and returns its pointer-free
// backing to the owning compact core. It is idempotent when called serially,
// but must not race with any operation on the same store. Releasing distinct
// sibling stores is synchronized. The store remains independent of Core.Reset
// until Release.
func (s *SelectedStore) Release() {
	if s == nil || s.released {
		return
	}
	s.released = true
	owner := s.owner
	records, children := s.records, s.children
	s.root, s.owner, s.records, s.children = 0, nil, nil, nil
	if owner != nil {
		owner.recycleSelectedStore(records, children)
	}
}

func (c *Core) acquireSelectedStore(recordCapacity, childCapacity int) *SelectedStore {
	c.selectedPoolMu.Lock()
	records := c.selectedPool.records
	children := c.selectedPool.children
	c.selectedPool = selectedStoreBacking{}
	c.selectedPoolMu.Unlock()
	prospectiveRecords := max(cap(records), recordCapacity)
	prospectiveChildren := max(cap(children), childCapacity)
	if selectedStoreRetainedBytes(prospectiveRecords, prospectiveChildren) > c.limits.MaxSelectedBytes {
		records, children = nil, nil
	}
	if cap(records) < recordCapacity {
		records = make([]SelectedNodeRecord, 0, recordCapacity)
	} else {
		clear(records)
		records = records[:0]
	}
	if cap(children) < childCapacity {
		children = make([]SelectedNodeID, 0, childCapacity)
	} else {
		clear(children)
		children = children[:0]
	}
	return &SelectedStore{records: records, children: children, owner: c}
}

func (c *Core) recycleSelectedStore(records []SelectedNodeRecord, children []SelectedNodeID) {
	if c == nil {
		return
	}
	clear(records)
	clear(children)
	incoming := selectedStoreBacking{records: records[:0], children: children[:0]}
	incomingBytes := selectedStoreRetainedBytes(cap(records), cap(children))
	if incomingBytes > c.limits.MaxSelectedBytes {
		return
	}
	c.selectedPoolMu.Lock()
	defer c.selectedPoolMu.Unlock()
	pooledBytes := selectedStoreRetainedBytes(cap(c.selectedPool.records), cap(c.selectedPool.children))
	if incomingBytes >= pooledBytes {
		c.selectedPool = incoming
	}
}

// SelectedCursor is a direct indexed cursor over a sealed store. It contains
// no interface, callback, or public Node pointer.
type SelectedCursor struct {
	store *SelectedStore
	id    SelectedNodeID
}

func (s *SelectedStore) Cursor() SelectedCursor             { return SelectedCursor{store: s, id: s.Root()} }
func (c SelectedCursor) ID() SelectedNodeID                 { return c.id }
func (c SelectedCursor) Record() (SelectedNodeRecord, bool) { return c.store.Record(c.id) }
func (c SelectedCursor) Parent() (SelectedCursor, bool) {
	r, ok := c.Record()
	if !ok || r.Parent == 0 {
		return SelectedCursor{}, false
	}
	return SelectedCursor{store: c.store, id: r.Parent}, true
}
func (c SelectedCursor) Child(index uint32) (SelectedCursor, bool) {
	r, ok := c.Record()
	if !ok {
		return SelectedCursor{}, false
	}
	id, ok := c.store.Child(r, index)
	if !ok {
		return SelectedCursor{}, false
	}
	return SelectedCursor{store: c.store, id: id}, true
}
func (c SelectedCursor) NextSibling() (SelectedCursor, bool) {
	record, ok := c.Record()
	if !ok || record.Parent == 0 {
		return SelectedCursor{}, false
	}
	parent, ok := c.store.Record(record.Parent)
	if !ok || uint32(record.ChildIndex)+1 >= uint32(parent.ChildCount) {
		return SelectedCursor{}, false
	}
	id, ok := c.store.Child(parent, uint32(record.ChildIndex)+1)
	if !ok {
		return SelectedCursor{}, false
	}
	return SelectedCursor{store: c.store, id: id}, true
}

type selectedRawOccurrence struct {
	payload    SubtreeID
	firstChild uint32
	childCount uint16
	alias      Symbol
	field      FieldID
}

func (c *Core) buildSelectedStoreStaged(roots []SubtreeID, policy SelectedStorePolicy, source []byte, poll func() error) (*SelectedStore, error) {
	if c == nil || len(roots) == 0 {
		return nil, errors.New("parser-core phase zero: selected store requires accepted roots")
	}
	if err := policy.validate(); err != nil {
		return nil, err
	}
	if poll == nil {
		poll = func() error { return nil }
	}
	if err := poll(); err != nil {
		return nil, err
	}

	raw, rawChildren, rawRoots, err := c.selectedRawOccurrences(roots, poll)
	if err != nil {
		return nil, err
	}
	defer c.finishSelectedBuildScratch()
	retainedAliasCount := 0
	for _, occurrence := range raw {
		record, recordErr := c.subtree(occurrence.payload)
		if recordErr != nil {
			return nil, recordErr
		}
		if !record.extra && policy.retainsAliasChild(occurrence.alias, record.symbol) {
			retainedAliasCount++
		}
	}
	results := resizeSelectedScratch(c.selectedBuild.results, len(raw))
	c.selectedBuild.results = results
	resultHasError := resizeSelectedScratch(c.selectedBuild.resultHasError, len(raw))
	c.selectedBuild.resultHasError = resultHasError
	wantRetained := uint64(len(raw)+retainedAliasCount)*uint64(unsafe.Sizeof(SelectedNodeRecord{})) +
		uint64(len(raw)-len(roots)+retainedAliasCount)*uint64(unsafe.Sizeof(SelectedNodeID(0)))
	if wantRetained > c.limits.MaxSelectedBytes {
		return nil, errors.New("parser-core phase zero: selected store retained-byte cap")
	}
	store := c.acquireSelectedStore(len(raw)+retainedAliasCount, len(raw)-len(roots)+retainedAliasCount)
	sealed := false
	defer func() {
		if !sealed {
			store.Release()
		}
	}()
	precedenceDeltas := resizeSelectedScratch(c.selectedBuild.precedence, 0)
	if cap(precedenceDeltas) < len(raw) {
		precedenceDeltas = make([]int32, 0, len(raw))
	}
	c.selectedBuild.precedence = precedenceDeltas
	if store.RetainedBytes() > c.limits.MaxSelectedBytes {
		return nil, errors.New("parser-core phase zero: selected store retained-byte cap")
	}
	collect := c.selectedBuild.collect[:0]
	logical := c.selectedBuild.logical[:0]

	for index := len(raw) - 1; index >= 0; index-- {
		if index&255 == 0 {
			if err := poll(); err != nil {
				return nil, err
			}
		}
		occ := raw[index]
		record, err := c.subtree(occ.payload)
		if err != nil {
			return nil, err
		}
		retainAliasChild := !record.extra && policy.retainsAliasChild(occ.alias, record.symbol)
		symbol := record.symbol
		if occ.alias != 0 && !retainAliasChild {
			symbol = occ.alias
		}
		meta, ok := policy.symbol(symbol)
		if !ok {
			return nil, fmt.Errorf("parser-core phase zero: selected symbol %d is outside policy", symbol)
		}
		hasError := record.missing || record.symbol == ErrorRegionSymbol
		logical = logical[:0]
		for childIndex := uint32(0); childIndex < uint32(occ.childCount); childIndex++ {
			rawChild := rawChildren[occ.firstChild+childIndex]
			if resultHasError[rawChild] {
				hasError = true
			}
			start := len(logical)
			collect = append(collect[:0], rawChild)
			for len(collect) != 0 {
				last := len(collect) - 1
				candidate := collect[last]
				collect = collect[:last]
				if id := results[candidate]; id != 0 {
					logical = append(logical, id)
					continue
				}
				hidden := raw[candidate]
				for nested := int(hidden.childCount) - 1; nested >= 0; nested-- {
					collect = append(collect, rawChildren[hidden.firstChild+uint32(nested)])
				}
			}
			if field := raw[rawChild].field; field != 0 {
				store.applyDirectField(logical[start:], field)
			}
		}

		if !record.terminal && record.childCount == 1 && record.fieldCount == 0 && record.aliasCount == 0 && len(logical) == 1 {
			childID := logical[0]
			child := &store.records[childID-1]
			rule := policy.unary(record.symbol, child.Symbol)
			if !child.Extra() && (rule == SelectedUnaryPass || rule == SelectedUnaryRenameLeaf && child.ChildCount == 0) {
				if hasError {
					child.flags |= selectedNodeFlagHasError
				}
				if rule == SelectedUnaryRenameLeaf {
					child.Symbol = record.symbol
					child.flags = selectedFlags(policy.Symbols[record.symbol], child.Extra(), child.External(), child.Terminal(), child.Missing(), child.HasError())
				}
				if occ.alias != 0 && !retainAliasChild {
					child.Symbol = occ.alias
					child.flags = selectedFlags(meta, child.Extra(), child.External(), child.Terminal(), child.Missing(), child.HasError())
				}
				child.ProductionID = record.productionID
				delta, err := checkedSelectedPrecedence(precedenceDeltas[childID-1], int32(record.dynamicPrecedence))
				if err != nil {
					return nil, err
				}
				precedenceDeltas[childID-1] = delta
				if occ.field != 0 && !retainAliasChild {
					child.Field = occ.field
				}
				if retainAliasChild {
					childID, precedenceDeltas, err = appendSelectedRetainedAliasWrapper(store, policy, childID, occ.alias, occ.field, record.productionID, precedenceDeltas)
					if err != nil {
						return nil, err
					}
				}
				results[index] = childID
				resultHasError[index] = hasError
				continue
			}
		}

		if !meta.Visible && !record.extra {
			resultHasError[index] = hasError
			continue
		}
		if len(logical) > math.MaxUint16 || uint64(len(store.children))+uint64(len(logical)) > math.MaxUint32 || uint64(len(store.records)) >= math.MaxUint32 {
			return nil, errors.New("parser-core phase zero: selected store exceeded uint32 arena")
		}
		id := SelectedNodeID(uint64(len(store.records)) + 1)
		first := uint32(len(store.children))
		store.children = append(store.children, logical...)
		flags := selectedFlags(meta, record.extra, record.external, record.terminal, record.missing, hasError)
		startByte, endByte := record.startByte, record.endByte
		if len(logical) != 0 {
			firstChild := store.records[logical[0]-1]
			lastChild := store.records[logical[len(logical)-1]-1]
			if firstChild.StartByte < startByte {
				startByte = firstChild.StartByte
			}
			if lastChild.EndByte > endByte {
				endByte = lastChild.EndByte
			}
		}
		incomingField := occ.field
		if retainAliasChild {
			incomingField = 0
		}
		store.records = append(store.records, SelectedNodeRecord{
			FirstChild: first, StartByte: startByte, EndByte: endByte,
			Symbol: symbol, Field: incomingField,
			ProductionID: record.productionID, DynamicPrecedence: int32(record.dynamicPrecedence),
			ChildCount: uint16(len(logical)), flags: flags,
		})
		precedenceDeltas = append(precedenceDeltas, int32(record.dynamicPrecedence))
		for childIndex, childID := range logical {
			store.records[childID-1].Parent = id
			store.records[childID-1].ChildIndex = uint16(childIndex)
		}
		if retainAliasChild {
			id, precedenceDeltas, err = appendSelectedRetainedAliasWrapper(store, policy, id, occ.alias, occ.field, record.productionID, precedenceDeltas)
			if err != nil {
				return nil, err
			}
		}
		results[index] = id
		resultHasError[index] = hasError
	}

	rootIDs := c.selectedBuild.rootIDs[:0]
	for _, index := range rawRoots {
		if id := results[index]; id != 0 {
			rootIDs = append(rootIDs, id)
			continue
		}
		collect = append(collect[:0], index)
		for len(collect) != 0 {
			last := len(collect) - 1
			candidate := collect[last]
			collect = collect[:last]
			if id := results[candidate]; id != 0 {
				rootIDs = append(rootIDs, id)
				continue
			}
			hidden := raw[candidate]
			for nested := int(hidden.childCount) - 1; nested >= 0; nested-- {
				collect = append(collect, rawChildren[hidden.firstChild+uint32(nested)])
			}
		}
	}
	if err := store.finishRoots(rootIDs, policy, c.limits.MaxSelectedBytes, poll); err != nil {
		return nil, err
	}
	if err := store.normalizeGoCompatibility(policy, source, poll); err != nil {
		return nil, err
	}
	if err := store.recomputePrecedence(precedenceDeltas); err != nil {
		return nil, err
	}
	if store.RetainedBytes() > c.limits.MaxSelectedBytes {
		return nil, errors.New("parser-core phase zero: selected store retained-byte cap")
	}
	if err := poll(); err != nil {
		return nil, err
	}
	sealed = true
	return store, nil
}

func resizeSelectedScratch[T any](items []T, length int) []T {
	if cap(items) < length {
		return make([]T, length)
	}
	items = items[:length]
	clear(items)
	return items
}

func (c *Core) finishSelectedBuildScratch() {
	if c == nil {
		return
	}
	c.selectedBuild.raw = c.selectedBuild.raw[:0]
	c.selectedBuild.rawChildren = c.selectedBuild.rawChildren[:0]
	c.selectedBuild.rawRoots = c.selectedBuild.rawRoots[:0]
	clear(c.selectedBuild.results)
	c.selectedBuild.results = c.selectedBuild.results[:0]
	clear(c.selectedBuild.resultHasError)
	c.selectedBuild.resultHasError = c.selectedBuild.resultHasError[:0]
	c.selectedBuild.precedence = c.selectedBuild.precedence[:0]
	c.selectedBuild.collect = c.selectedBuild.collect[:0]
	c.selectedBuild.logical = c.selectedBuild.logical[:0]
	c.selectedBuild.rootIDs = c.selectedBuild.rootIDs[:0]
	c.selectedBuild.stack = c.selectedBuild.stack[:0]
}

func checkedSelectedPrecedence(left, right int32) (int32, error) {
	value := int64(left) + int64(right)
	if value < math.MinInt32 || value > math.MaxInt32 {
		return 0, errors.New("parser-core phase zero: selected precedence overflow")
	}
	return int32(value), nil
}

func (s *SelectedStore) recomputePrecedence(deltas []int32) error {
	if s == nil || s.root == 0 || len(deltas) != len(s.records) {
		return errors.New("parser-core phase zero: selected precedence sidecar drifted")
	}
	recompute := func(id SelectedNodeID) error {
		record := &s.records[id-1]
		value := deltas[id-1]
		for index := uint32(0); index < uint32(record.ChildCount); index++ {
			child := s.children[record.FirstChild+index]
			var err error
			value, err = checkedSelectedPrecedence(value, s.records[child-1].DynamicPrecedence)
			if err != nil {
				return err
			}
		}
		record.DynamicPrecedence = value
		return nil
	}
	for index := range s.records {
		if err := recompute(SelectedNodeID(index + 1)); err != nil {
			return err
		}
	}
	return recompute(s.root)
}

// BuildAuthenticatedSelectedStore lazily requests the immutable policy from
// the Core's exact table adapter. Generic/public compact-parser controls do not
// pay the quadratic unary-policy initialization or retention cost.
func (c *Core) BuildAuthenticatedSelectedStore(roots []SubtreeID, source []byte, poll func() error) (*SelectedStore, error) {
	if c == nil || c.selectedProvider == nil {
		return nil, errors.New("parser-core phase zero: selected-store policy is unavailable")
	}
	if poll != nil {
		if err := poll(); err != nil {
			return nil, err
		}
	}
	if c.selectedPolicy == nil {
		policy, err := c.selectedProvider.SelectedStorePolicy()
		if err != nil {
			return nil, err
		}
		if len(policy.Symbols) == 0 {
			return nil, errors.New("parser-core phase zero: selected-store policy is unavailable")
		}
		c.selectedPolicy = &policy
	}
	return c.BuildSelectedStore(roots, *c.selectedPolicy, source, poll)
}

func selectedMarked(table []bool, symbol Symbol) bool {
	return int(symbol) < len(table) && table[symbol]
}

func (s *SelectedStore) normalizeGoCompatibility(policy SelectedStorePolicy, source []byte, poll func() error) error {
	if s == nil || len(source) == 0 || len(policy.SemicolonContainers) == 0 {
		return nil
	}
	// First drop implicit semicolon transport nodes. Compact the child arena in
	// one pass so dead windows do not survive in retained bytes.
	children := s.children[:0]
	for index := range s.records {
		if index&255 == 0 {
			if err := poll(); err != nil {
				return err
			}
		}
		record := &s.records[index]
		old := s.children[record.FirstChild : record.FirstChild+uint32(record.ChildCount)]
		record.FirstChild = uint32(len(children))
		for _, childID := range old {
			child := s.records[childID-1]
			drop := selectedMarked(policy.SemicolonContainers, record.Symbol) &&
				(child.Symbol == policy.Semicolon || child.Symbol == policy.SemicolonNUL) &&
				selectedDropSemicolon(child.StartByte, child.EndByte, source)
			if !drop {
				s.records[childID-1].ChildIndex = uint16(len(children) - int(record.FirstChild))
				children = append(children, childID)
			}
		}
		record.ChildCount = uint16(len(children) - int(record.FirstChild))
	}
	s.children = children

	// Public Go compatibility applies sibling boundary ownership before the
	// trailing statement-list adjustment. The selected store has unique parent
	// ownership, so a dense record pass is sufficient.
	for index := range s.records {
		record := &s.records[index]
		for childIndex := uint32(0); childIndex+1 < uint32(record.ChildCount); childIndex++ {
			leftID := s.children[record.FirstChild+childIndex]
			rightID := s.children[record.FirstChild+childIndex+1]
			left, right := &s.records[leftID-1], s.records[rightID-1]
			if selectedMarked(policy.StatementLists, left.Symbol) && left.EndByte < right.StartByte && selectedTrivia(source, left.EndByte, right.StartByte) {
				if target := selectedTrailingNewline(left.EndByte, right.StartByte, source); target > left.EndByte {
					left.EndByte = target
				}
			}
			if selectedMarked(policy.Cases, left.Symbol) {
				s.normalizeCaseBoundary(left, right.StartByte, policy, source)
			}
		}
	}
	for index := range s.records {
		record := &s.records[index]
		if !selectedMarked(policy.StatementLists, record.Symbol) || record.ChildCount == 0 {
			continue
		}
		last := &s.records[s.children[record.FirstChild+uint32(record.ChildCount)-1]-1]
		if last.EndByte >= record.EndByte {
			continue
		}
		if target := selectedTriviaBeforeExtra(last.EndByte, record.EndByte, source); target > last.EndByte && target < record.EndByte {
			record.EndByte = target
		}
	}
	root := &s.records[s.root-1]
	if root.EndByte < uint32(len(source)) && selectedTrivia(source, root.EndByte, uint32(len(source))) {
		root.EndByte = uint32(len(source))
	}
	return nil
}

func selectedDropSemicolon(start, end uint32, source []byte) bool {
	if start >= end || int(end) > len(source) {
		return true
	}
	span := source[start:end]
	return bytes.IndexByte(span, ';') < 0 && (bytes.IndexByte(span, '\n') >= 0 || bytes.IndexByte(span, '\r') >= 0)
}

func selectedTrivia(source []byte, start, end uint32) bool {
	if start > end || int(end) > len(source) {
		return false
	}
	for _, b := range source[start:end] {
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			return false
		}
	}
	return true
}

func selectedTrailingNewline(start, end uint32, source []byte) uint32 {
	if !selectedTrivia(source, start, end) {
		return start
	}
	if index := bytes.LastIndexByte(source[start:end], '\n'); index >= 0 {
		return start + uint32(index) + 1
	}
	return start
}

func selectedTriviaBeforeExtra(start, end uint32, source []byte) uint32 {
	if start >= end || int(end) > len(source) {
		return start
	}
	for cursor := start; cursor < end; cursor++ {
		switch source[cursor] {
		case ' ', '\t', '\n', '\r':
			continue
		}
		for newline := start; newline < cursor; newline++ {
			if source[newline] == '\n' {
				return newline + 1
			}
		}
		return cursor
	}
	return end
}

func selectedTrailingTriviaBoundaryBefore(end uint32, source []byte) (uint32, bool) {
	if end == 0 || int(end) > len(source) {
		return end, false
	}
	start := int(end)
	for start > 0 {
		switch source[start-1] {
		case ' ', '\t', '\r', '\n':
			start--
		default:
			goto ready
		}
	}
ready:
	gap := source[start:int(end)]
	if newline := bytes.LastIndexByte(gap, '\n'); newline >= 0 {
		return uint32(start + newline + 1), true
	}
	return end, false
}

func (s *SelectedStore) normalizeCaseBoundary(current *SelectedNodeRecord, nextStart uint32, policy SelectedStorePolicy, source []byte) {
	if current.ChildCount == 0 || int(nextStart) > len(source) {
		return
	}
	tail := &s.records[s.children[current.FirstChild+uint32(current.ChildCount)-1]-1]
	if !selectedMarked(policy.StatementLists, tail.Symbol) {
		return
	}
	target, newline := selectedTrailingTriviaBoundaryBefore(nextStart, source)
	if newline {
		current.EndByte = target
		if tail.EndByte > target || (tail.EndByte < target && selectedTrivia(source, tail.EndByte, target)) {
			tail.EndByte = target
		}
		return
	}
	if current.EndByte > nextStart {
		current.EndByte = nextStart
	}
	if tail.EndByte > nextStart {
		tail.EndByte = nextStart
	}
}

func selectedFlags(meta SelectedSymbolPolicy, extra, external, terminal, missing, hasError bool) uint8 {
	var flags uint8
	if meta.Named {
		flags |= selectedNodeFlagNamed
	}
	if extra {
		flags |= selectedNodeFlagExtra
	}
	if external {
		flags |= selectedNodeFlagExternal
	}
	if terminal {
		flags |= selectedNodeFlagTerminal
	}
	if missing {
		flags |= selectedNodeFlagMissing
	}
	if hasError {
		flags |= selectedNodeFlagHasError
	}
	return flags
}

func appendSelectedRetainedAliasWrapper(
	store *SelectedStore,
	policy SelectedStorePolicy,
	childID SelectedNodeID,
	alias Symbol,
	field FieldID,
	productionID uint16,
	precedenceDeltas []int32,
) (SelectedNodeID, []int32, error) {
	if store == nil || childID == 0 || int(childID) > len(store.records) {
		return 0, precedenceDeltas, errors.New("parser-core phase zero: retained alias child is invalid")
	}
	meta, ok := policy.symbol(alias)
	if !ok || !meta.Visible {
		return 0, precedenceDeltas, errors.New("parser-core phase zero: retained alias wrapper is not visible")
	}
	if uint64(len(store.records)) >= math.MaxUint32 || uint64(len(store.children)) >= math.MaxUint32 {
		return 0, precedenceDeltas, errors.New("parser-core phase zero: retained alias wrapper exceeded arena")
	}
	child := &store.records[childID-1]
	id := SelectedNodeID(uint64(len(store.records)) + 1)
	first := uint32(len(store.children))
	store.children = append(store.children, childID)
	store.records = append(store.records, SelectedNodeRecord{
		FirstChild: first, StartByte: child.StartByte, EndByte: child.EndByte,
		Symbol: alias, Field: field, ProductionID: productionID, ChildCount: 1,
		flags: selectedFlags(meta, false, false, false, false, child.HasError()),
	})
	precedenceDeltas = append(precedenceDeltas, 0)
	child = &store.records[childID-1]
	child.Parent = id
	child.ChildIndex = 0
	return id, precedenceDeltas, nil
}

func (s *SelectedStore) applyDirectField(ids []SelectedNodeID, field FieldID) {
	if len(ids) == 0 || field == 0 {
		return
	}
	named := 0
	for _, id := range ids {
		if s.records[id-1].Named() {
			named++
		}
	}
	for _, id := range ids {
		r := &s.records[id-1]
		if r.Field != 0 {
			continue
		}
		if named > 0 && !r.Named() {
			continue
		}
		r.Field = field
		if named == 1 {
			return
		}
	}
}

func selectedRootMergeCounts(childCount, rootCount int, realChildCount uint16) (int, int, error) {
	if childCount < 0 || rootCount < 1 {
		return 0, 0, errors.New("parser-core phase zero: invalid selected root counts")
	}
	extraCount := uint64(rootCount - 1)
	mergedCount := extraCount + uint64(realChildCount)
	finalChildren := uint64(childCount) + extraCount
	maxInt := uint64(^uint(0) >> 1)
	if mergedCount > math.MaxUint16 || finalChildren > math.MaxUint32 || finalChildren > maxInt {
		return 0, 0, errors.New("parser-core phase zero: selected root extras exceed arena")
	}
	return int(mergedCount), int(finalChildren), nil
}

const selectedStoreCopyPollElements = 1024

func copySelectedStoreChunks[T any](destination, source []T, poll func() error) error {
	if len(destination) < len(source) {
		return errors.New("parser-core phase zero: selected store copy destination is short")
	}
	for start := 0; start < len(source); start += selectedStoreCopyPollElements {
		if poll != nil {
			if err := poll(); err != nil {
				return err
			}
		}
		end := min(start+selectedStoreCopyPollElements, len(source))
		copy(destination[start:end], source[start:end])
	}
	return nil
}

func moveSelectedStoreChildren(children []SelectedNodeID, destination, source, count int, poll func() error) error {
	if count < 0 || destination < 0 || source < 0 || destination+count > len(children) || source+count > len(children) {
		return errors.New("parser-core phase zero: selected child move is outside arena")
	}
	if count == 0 || destination == source {
		return nil
	}
	if destination < source {
		for offset := 0; offset < count; offset += selectedStoreCopyPollElements {
			if poll != nil {
				if err := poll(); err != nil {
					return err
				}
			}
			width := min(selectedStoreCopyPollElements, count-offset)
			copy(children[destination+offset:destination+offset+width], children[source+offset:source+offset+width])
		}
		return nil
	}
	for remaining := count; remaining > 0; {
		if poll != nil {
			if err := poll(); err != nil {
				return err
			}
		}
		width := min(selectedStoreCopyPollElements, remaining)
		remaining -= width
		copy(children[destination+remaining:destination+remaining+width], children[source+remaining:source+remaining+width])
	}
	return nil
}

func (s *SelectedStore) finishRoots(roots []SelectedNodeID, policy SelectedStorePolicy, retainedCap uint64, poll func() error) error {
	if len(roots) == 0 {
		return errors.New("parser-core phase zero: selected store sealed no logical roots")
	}
	if len(roots) == 1 {
		root := s.records[roots[0]-1]
		if root.Extra() || root.Symbol != policy.ExpectedRoot {
			return errors.New("parser-core phase zero: selected store root is not authenticated")
		}
		s.root = roots[0]
		return nil
	}
	realIndex := -1
	for index, id := range roots {
		record := s.records[id-1]
		if !record.Extra() && record.Symbol == policy.ExpectedRoot {
			if realIndex != -1 {
				return errors.New("parser-core phase zero: selected store has multiple authenticated roots")
			}
			realIndex = index
			continue
		}
		if !record.Extra() {
			return errors.New("parser-core phase zero: selected store has a non-extra sibling root")
		}
	}
	if realIndex < 0 {
		return errors.New("parser-core phase zero: selected store cannot identify authenticated root")
	}
	realID := roots[realIndex]
	real := &s.records[realID-1]
	oldStart := real.FirstChild
	oldEnd := oldStart + uint32(real.ChildCount)
	extraCount := len(roots) - 1
	mergedCount, finalChildren, err := selectedRootMergeCounts(len(s.children), len(roots), real.ChildCount)
	if err != nil {
		return err
	}
	childCapacity := cap(s.children)
	if childCapacity < finalChildren {
		childCapacity = finalChildren
	}
	if selectedStoreRetainedBytes(cap(s.records), childCapacity) > retainedCap {
		return errors.New("parser-core phase zero: selected store retained-byte cap")
	}
	oldLength := len(s.children)
	if cap(s.children) < finalChildren {
		children := make([]SelectedNodeID, oldLength, finalChildren)
		if err := copySelectedStoreChunks(children, s.children, poll); err != nil {
			return err
		}
		s.children = children
	}
	s.children = s.children[:finalChildren]
	if err := moveSelectedStoreChildren(s.children, int(oldEnd)+extraCount, int(oldEnd), oldLength-int(oldEnd), poll); err != nil {
		return err
	}
	before := uint32(realIndex)
	if err := moveSelectedStoreChildren(s.children, int(oldStart+before), int(oldStart), int(real.ChildCount), poll); err != nil {
		return err
	}
	if err := copySelectedStoreChunks(s.children[oldStart:oldStart+before], roots[:realIndex], poll); err != nil {
		return err
	}
	if err := copySelectedStoreChunks(s.children[oldEnd+before:oldEnd+uint32(extraCount)], roots[realIndex+1:], poll); err != nil {
		return err
	}
	for index := range s.records {
		record := &s.records[index]
		if SelectedNodeID(index+1) != realID && record.ChildCount != 0 && record.FirstChild >= oldEnd {
			record.FirstChild += uint32(extraCount)
		}
	}
	real.ChildCount = uint16(mergedCount)
	real.StartByte = 0
	merged := s.children[oldStart : oldStart+uint32(mergedCount)]
	for childIndex, child := range merged {
		s.records[child-1].Parent = realID
		s.records[child-1].ChildIndex = uint16(childIndex)
	}
	s.root = realID
	return nil
}

func (c *Core) selectedRawOccurrences(roots []SubtreeID, poll func() error) ([]selectedRawOccurrence, []uint32, []uint32, error) {
	rawCapacity := min(len(c.subtrees), int(c.limits.MaxSelectedOccurrences))
	raw := c.selectedBuild.raw[:0]
	if cap(raw) < rawCapacity {
		raw = make([]selectedRawOccurrence, 0, rawCapacity)
	}
	children := c.selectedBuild.rawChildren[:0]
	childCapacity := min(len(c.children), int(c.limits.MaxSelectedOccurrences))
	if cap(children) < childCapacity {
		children = make([]uint32, 0, childCapacity)
	}
	rootOccurrences := c.selectedBuild.rawRoots[:0]
	if cap(rootOccurrences) < len(roots) {
		rootOccurrences = make([]uint32, 0, len(roots))
	}
	stack := c.selectedBuild.stack[:0]
	if cap(stack) < 64 {
		stack = make([]selectedOccurrenceFrame, 0, 64)
	}
	c.selectedBuild.raw, c.selectedBuild.rawChildren = raw, children
	c.selectedBuild.rawRoots, c.selectedBuild.stack = rootOccurrences, stack
	var work uint64
	appendOccurrence := func(payload SubtreeID, alias Symbol, field FieldID) (uint32, error) {
		record, err := c.subtree(payload)
		if err != nil {
			return 0, err
		}
		if record.childCount > math.MaxUint16 || uint64(len(raw))+1 > uint64(c.limits.MaxSelectedOccurrences) ||
			uint64(len(children))+uint64(record.childCount) > uint64(c.limits.MaxSelectedOccurrences) {
			return 0, errors.New("parser-core phase zero: raw selected occurrence cap")
		}
		if uint64(len(raw)) >= math.MaxUint32 || uint64(len(children))+uint64(record.childCount) > math.MaxUint32 {
			return 0, errors.New("parser-core phase zero: raw selected occurrence exceeds arena")
		}
		id := uint32(len(raw))
		raw = append(raw, selectedRawOccurrence{payload: payload, firstChild: uint32(len(children)), childCount: uint16(record.childCount), alias: alias, field: field})
		children = append(children, make([]uint32, record.childCount)...)
		return id, nil
	}
	for _, root := range roots {
		rootID, err := appendOccurrence(root, 0, 0)
		if err != nil {
			return nil, nil, nil, err
		}
		rootOccurrences = append(rootOccurrences, rootID)
		stack = append(stack, selectedOccurrenceFrame{occurrence: rootID})
		for len(stack) != 0 {
			work++
			if work&255 == 0 {
				if err := poll(); err != nil {
					return nil, nil, nil, err
				}
			}
			top := &stack[len(stack)-1]
			parent := raw[top.occurrence]
			if top.next >= uint32(parent.childCount) {
				stack = stack[:len(stack)-1]
				continue
			}
			ordinal := top.next
			top.next++
			parentRecord, err := c.subtree(parent.payload)
			if err != nil {
				return nil, nil, nil, err
			}
			payload := c.children[parentRecord.firstChild+ordinal]
			var alias Symbol
			if parentRecord.aliasCount != 0 {
				if parentRecord.aliasCount != parentRecord.childCount {
					return nil, nil, nil, errors.New("parser-core phase zero: selected alias width drifted")
				}
				alias = c.aliases[parentRecord.firstAlias+ordinal]
			}
			var field FieldID
			for _, entry := range c.fields[parentRecord.firstField : parentRecord.firstField+parentRecord.fieldCount] {
				if uint32(entry.ChildIndex) != ordinal {
					continue
				}
				if entry.Inherited || field != 0 {
					return nil, nil, nil, errors.New("parser-core phase zero: selected field profile is outside admitted direct single-field scope")
				}
				field = entry.FieldID
			}
			childID, err := appendOccurrence(payload, alias, field)
			if err != nil {
				return nil, nil, nil, err
			}
			children[parent.firstChild+ordinal] = childID
			stack = append(stack, selectedOccurrenceFrame{occurrence: childID})
		}
	}
	c.selectedBuild.raw, c.selectedBuild.rawChildren = raw, children
	c.selectedBuild.rawRoots, c.selectedBuild.stack = rootOccurrences, stack
	return raw, children, rootOccurrences, nil
}
