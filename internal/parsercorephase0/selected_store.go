package parsercorephase0

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"unsafe"
)

// SelectedSymbolPolicy is immutable materialization metadata for one grammar
// symbol. It is built once with the authenticated table adapter, outside the
// parse and selected-store timing boundaries.
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

// SelectedStorePolicy is the fail-closed compact-to-consumer contract. Unary
// is a dense parent-major matrix with stride len(Symbols).
type SelectedStorePolicy struct {
	Symbols             []SelectedSymbolPolicy
	Unary               []SelectedUnaryRule
	ExpectedRoot        Symbol
	Semicolon           Symbol
	SemicolonNUL        Symbol
	SemicolonContainers []bool
	Cases               []bool
	StatementLists      []bool
}

// SelectedStorePolicyProvider lets an authenticated table adapter install the
// compact-to-consumer policy once when a Core is constructed.
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

func (p SelectedStorePolicy) validate() error {
	if len(p.Symbols) == 0 || int(p.ExpectedRoot) >= len(p.Symbols) {
		return errors.New("parser-core phase zero: selected-store policy has no authenticated root")
	}
	width := len(p.Symbols)
	if width > math.MaxInt/width || len(p.Unary) != width*width {
		return errors.New("parser-core phase zero: selected-store unary policy width drifted")
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
)

// SelectedNodeRecord is one immutable logical occurrence after hidden-parent
// elision and unary collapse. Field belongs to the incoming logical edge.
type SelectedNodeRecord struct {
	Parent            SelectedNodeID
	FirstChild        uint32
	StartByte         uint32
	EndByte           uint32
	Payload           SubtreeID
	Symbol            Symbol
	Field             FieldID
	ProductionID      uint16
	DynamicPrecedence int16
	ChildCount        uint16
	ChildIndex        uint16
	flags             uint8
}

func (r SelectedNodeRecord) Named() bool    { return r.flags&selectedNodeFlagNamed != 0 }
func (r SelectedNodeRecord) Extra() bool    { return r.flags&selectedNodeFlagExtra != 0 }
func (r SelectedNodeRecord) External() bool { return r.flags&selectedNodeFlagExternal != 0 }
func (r SelectedNodeRecord) Terminal() bool { return r.flags&selectedNodeFlagTerminal != 0 }

// SelectedStore is the sealed, pointer-free selected syntax backing store.
// IDs are occurrence identities; repeated compact SubtreeIDs remain distinct.
type SelectedStore struct {
	root     SelectedNodeID
	records  []SelectedNodeRecord
	children []SelectedNodeID
}

func (s *SelectedStore) Root() SelectedNodeID {
	if s == nil {
		return 0
	}
	return s.root
}

func (s *SelectedStore) NodeCount() uint64 {
	if s == nil {
		return 0
	}
	return uint64(len(s.records))
}

func (s *SelectedStore) ChildCount() uint64 {
	if s == nil {
		return 0
	}
	return uint64(len(s.children))
}

// RetainedBytes reports exact backing-array retention, excluding the small
// store header itself so capacity changes remain visible.
func (s *SelectedStore) RetainedBytes() uint64 {
	if s == nil {
		return 0
	}
	return uint64(cap(s.records))*uint64(unsafe.Sizeof(SelectedNodeRecord{})) +
		uint64(cap(s.children))*uint64(unsafe.Sizeof(SelectedNodeID(0)))
}

func (s *SelectedStore) Record(id SelectedNodeID) (SelectedNodeRecord, bool) {
	if s == nil || id == 0 || uint64(id) > uint64(len(s.records)) {
		return SelectedNodeRecord{}, false
	}
	return s.records[id-1], true
}

func (s *SelectedStore) Child(record SelectedNodeRecord, index uint32) (SelectedNodeID, bool) {
	if s == nil || index >= uint32(record.ChildCount) {
		return 0, false
	}
	slot := uint64(record.FirstChild) + uint64(index)
	if slot >= uint64(len(s.children)) {
		return 0, false
	}
	return s.children[slot], true
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

// BuildSelectedStore seals accepted roots directly from compact immutable
// payload arenas. It does not enumerate a head, construct public Nodes, or
// invoke a per-occurrence callback.
func (c *Core) BuildSelectedStore(roots []SubtreeID, policy SelectedStorePolicy, source []byte, poll func() error) (*SelectedStore, error) {
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
	results := make([]SelectedNodeID, len(raw))
	store := &SelectedStore{
		records:  make([]SelectedNodeRecord, 0, len(raw)),
		children: make([]SelectedNodeID, 0, len(raw)-len(roots)),
	}
	collect := make([]uint32, 0, 32)
	logical := make([]SelectedNodeID, 0, 32)

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
		symbol := record.symbol
		if occ.alias != 0 {
			symbol = occ.alias
		}
		meta, ok := policy.symbol(symbol)
		if !ok {
			return nil, fmt.Errorf("parser-core phase zero: selected symbol %d is outside policy", symbol)
		}

		logical = logical[:0]
		for childIndex := uint32(0); childIndex < uint32(occ.childCount); childIndex++ {
			rawChild := rawChildren[occ.firstChild+childIndex]
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
				if rule == SelectedUnaryRenameLeaf {
					child.Symbol = record.symbol
					child.flags = selectedFlags(policy.Symbols[record.symbol], child.Extra(), child.External(), child.Terminal())
				}
				if occ.alias != 0 {
					child.Symbol = occ.alias
					child.flags = selectedFlags(meta, child.Extra(), child.External(), child.Terminal())
				}
				child.ProductionID = record.productionID
				child.DynamicPrecedence += record.dynamicPrecedence
				if occ.field != 0 {
					child.Field = occ.field
				}
				results[index] = childID
				continue
			}
		}

		if !meta.Visible && !record.extra {
			continue
		}
		if len(logical) > math.MaxUint16 || len(store.children)+len(logical) > math.MaxUint32 || len(store.records) >= math.MaxUint32 {
			return nil, errors.New("parser-core phase zero: selected store exceeded uint32 arena")
		}
		id := SelectedNodeID(len(store.records) + 1)
		first := uint32(len(store.children))
		store.children = append(store.children, logical...)
		flags := selectedFlags(meta, record.extra, record.external, record.terminal)
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
		store.records = append(store.records, SelectedNodeRecord{
			FirstChild: first, StartByte: startByte, EndByte: endByte,
			Payload: occ.payload, Symbol: symbol, Field: occ.field,
			ProductionID: record.productionID, DynamicPrecedence: record.dynamicPrecedence,
			ChildCount: uint16(len(logical)), flags: flags,
		})
		for childIndex, childID := range logical {
			store.records[childID-1].Parent = id
			store.records[childID-1].ChildIndex = uint16(childIndex)
		}
		results[index] = id
	}

	rootIDs := make([]SelectedNodeID, 0, len(roots))
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
	if err := store.finishRoots(rootIDs, policy); err != nil {
		return nil, err
	}
	if err := store.normalizeGoCompatibility(policy, source, poll); err != nil {
		return nil, err
	}
	if err := poll(); err != nil {
		return nil, err
	}
	return store, nil
}

// BuildAuthenticatedSelectedStore uses the immutable policy captured from the
// Core's exact table adapter.
func (c *Core) BuildAuthenticatedSelectedStore(roots []SubtreeID, source []byte, poll func() error) (*SelectedStore, error) {
	if c == nil || c.selectedPolicy == nil {
		return nil, errors.New("parser-core phase zero: selected-store policy is unavailable")
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
	children := make([]SelectedNodeID, 0, len(s.children))
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
		if tail.EndByte > target || tail.EndByte < target && selectedTrivia(source, tail.EndByte, target) {
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

func selectedFlags(meta SelectedSymbolPolicy, extra, external, terminal bool) uint8 {
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
	return flags
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

func (s *SelectedStore) finishRoots(roots []SelectedNodeID, policy SelectedStorePolicy) error {
	if len(roots) == 0 {
		return errors.New("parser-core phase zero: selected store sealed no logical roots")
	}
	if len(roots) == 1 {
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
		}
	}
	if realIndex < 0 {
		return errors.New("parser-core phase zero: selected store cannot identify authenticated root")
	}
	realID := roots[realIndex]
	real := &s.records[realID-1]
	oldChildren := append([]SelectedNodeID(nil), s.children[real.FirstChild:real.FirstChild+uint32(real.ChildCount)]...)
	merged := make([]SelectedNodeID, 0, len(roots)-1+len(oldChildren))
	merged = append(merged, roots[:realIndex]...)
	merged = append(merged, oldChildren...)
	merged = append(merged, roots[realIndex+1:]...)
	if len(merged) > math.MaxUint16 || len(s.children)+len(merged) > math.MaxUint32 {
		return errors.New("parser-core phase zero: selected root extras exceed arena")
	}
	real.FirstChild = uint32(len(s.children))
	real.ChildCount = uint16(len(merged))
	real.StartByte = 0
	s.children = append(s.children, merged...)
	for childIndex, child := range merged {
		s.records[child-1].Parent = realID
		s.records[child-1].ChildIndex = uint16(childIndex)
	}
	s.root = realID
	return nil
}

func (c *Core) selectedRawOccurrences(roots []SubtreeID, poll func() error) ([]selectedRawOccurrence, []uint32, []uint32, error) {
	raw := make([]selectedRawOccurrence, 0, len(c.subtrees))
	children := make([]uint32, 0, len(c.children))
	rootOccurrences := make([]uint32, 0, len(roots))
	type frame struct {
		occurrence uint32
		next       uint32
	}
	stack := make([]frame, 0, 64)
	var work uint64
	appendOccurrence := func(payload SubtreeID, alias Symbol, field FieldID) (uint32, error) {
		record, err := c.subtree(payload)
		if err != nil {
			return 0, err
		}
		if record.childCount > math.MaxUint16 || len(raw) >= math.MaxUint32 || len(children)+int(record.childCount) > math.MaxUint32 {
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
		stack = append(stack, frame{occurrence: rootID})
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
			stack = append(stack, frame{occurrence: childID})
		}
	}
	return raw, children, rootOccurrences, nil
}
