package parsercorephase0

import (
	"errors"
	"fmt"
	"math"
)

// NativeProjectionRecord is transient construction metadata. The sink owns the
// public representation; the core keeps only the active logical frontier.
type NativeProjectionRecord struct {
	ReplayPre, ReplayState StateID
	Source                 SubtreeID
	StartByte, EndByte     uint32
	DynamicPrecedence      int32
	SupertypeMask          uint32
	ChildCount             uint32
	Symbol                 Symbol
	Field                  FieldID
	ProductionID           uint16
	Flags                  uint16
}

const nativeProjectionFragile uint16 = 1 << 8
const nativeProjectionPreKnown uint16 = 1 << 9
const nativeProjectionStateKnown uint16 = 1 << 10

func (r NativeProjectionRecord) ReplayPreKnown() bool { return r.Flags&nativeProjectionPreKnown != 0 }
func (r NativeProjectionRecord) ReplayStateKnown() bool {
	return r.Flags&nativeProjectionStateKnown != 0
}

// NativeProjectionTransition replays fresh native records from their real
// grammar symbol and terminal flag. StreamNativeProjection rejects borrowed
// subtrees before using this narrower contract; borrowed validation remains
// available through the full-view Transition callback.
type NativeProjectionTransition interface {
	ReplayNativeTransition(StateID, Symbol, bool) (StateID, bool)
}

type NativeProjectionReplay struct {
	CheckpointsRequired bool
	Scratch             *MaterializationPostorderScratch
	RootPre             StateID
	Transition          MaterializationReplayBorrowedTransition
	NativeTransition    NativeProjectionTransition
	VisitRaw            func(SubtreeID, *MaterializationSubtreeView) error
	VisitTerminal       func(SubtreeID, *MaterializationSubtreeView) error
	Tiling              *NativeProjectionTiling
}

func (r NativeProjectionRecord) Named() bool    { return r.Flags&uint16(selectedNodeFlagNamed) != 0 }
func (r NativeProjectionRecord) Extra() bool    { return r.Flags&uint16(selectedNodeFlagExtra) != 0 }
func (r NativeProjectionRecord) External() bool { return r.Flags&uint16(selectedNodeFlagExternal) != 0 }
func (r NativeProjectionRecord) Terminal() bool { return r.Flags&uint16(selectedNodeFlagTerminal) != 0 }
func (r NativeProjectionRecord) Missing() bool  { return r.Flags&uint16(selectedNodeFlagMissing) != 0 }
func (r NativeProjectionRecord) HasError() bool { return r.Flags&uint16(selectedNodeFlagHasError) != 0 }
func (r NativeProjectionRecord) Fragile() bool  { return r.Flags&nativeProjectionFragile != 0 }

// NativeProjectionSink must copy borrowed children before Emit returns. Read
// and Rewrite address only handles emitted during this synchronous call.
type NativeProjectionSink interface {
	Emit(NativeProjectionRecord, []SelectedNodeID) (SelectedNodeID, error)
	Read(SelectedNodeID) (NativeProjectionRecord, error)
	Rewrite(SelectedNodeID, NativeProjectionRecord) error
}

// NativeProjectionSinkAdapter keeps the implementation's methods internal.
// The caller owns a stable context and supplies method expressions, not closures.
type NativeProjectionSinkAdapter[T any] struct {
	Context       *T
	EmitRecord    func(*T, NativeProjectionRecord, []SelectedNodeID) (SelectedNodeID, error)
	ReadRecord    func(*T, SelectedNodeID) (NativeProjectionRecord, error)
	RewriteRecord func(*T, SelectedNodeID, NativeProjectionRecord) error
}

func (a *NativeProjectionSinkAdapter[T]) Emit(r NativeProjectionRecord, ids []SelectedNodeID) (SelectedNodeID, error) {
	return a.EmitRecord(a.Context, r, ids)
}
func (a *NativeProjectionSinkAdapter[T]) Read(id SelectedNodeID) (NativeProjectionRecord, error) {
	return a.ReadRecord(a.Context, id)
}
func (a *NativeProjectionSinkAdapter[T]) Rewrite(id SelectedNodeID, r NativeProjectionRecord) error {
	return a.RewriteRecord(a.Context, id, r)
}

// StreamNativeProjection emits admitted clean occurrences without allocating a
// selected-record arena or selected-child arena. Returned roots borrow Core
// scratch until the next selected build. Native root finalization remains the
// consumer's responsibility, as do replay, dependencies and byte-tiling checks.
func (c *Core) StreamNativeProjection(roots []SubtreeID, source []byte, sink NativeProjectionSink, replay NativeProjectionReplay, poll func() error) ([]SelectedNodeID, error) {
	if c == nil || c.selectedProvider == nil || sink == nil || len(roots) == 0 {
		return nil, errors.New("parser-core phase zero: incomplete native projection")
	}
	if len(c.reusedSubtrees) != 0 {
		return nil, errors.New("parser-core phase zero: native projection cannot expand reused subtrees")
	}
	if poll == nil {
		poll = func() error { return nil }
	}
	if err := poll(); err != nil {
		return nil, err
	}
	if c.selectedPolicy == nil {
		p, err := c.selectedProvider.SelectedStorePolicy()
		if err != nil {
			return nil, err
		}
		c.selectedPolicy = &p
	}
	policy := *c.selectedPolicy
	if err := policy.validate(); err != nil {
		return nil, err
	}
	if replay.Scratch == nil || (replay.Transition == nil) == (replay.NativeTransition == nil) {
		return nil, errors.New("parser-core phase zero: native projection requires authenticated replay")
	}
	rs := replay.Scratch
	if err := rs.prepareNative(c.subtrees.Len()); err != nil {
		return nil, err
	}
	defer rs.Reset()
	colors := rs.colors
	// A hidden span projecting to one visible child does not mean the raw
	// child materializes as that node. Only an emitted node or an actual
	// unary collapse may receive its parent's replay stamp.
	const rawNodeIsProjected uint8 = 1 << 2
	defer c.finishSelectedBuildScratch()
	logical := c.selectedBuild.logical[:0]
	rootIDs := c.selectedBuild.rootIDs[:0]
	stack := rs.nativeFrames[:0]
	var occurrences, rawChildren, work, emitted uint64
	save := func() {
		c.selectedBuild.logical = logical
		c.selectedBuild.rootIDs = rootIDs
		rs.nativeFrames = stack
	}
	defer save()
	push := func(id SubtreeID, alias Symbol, field FieldID) error {
		r, err := c.subtree(id)
		if err != nil {
			return err
		}
		if r.childCount > math.MaxInt32 || occurrences+1 > uint64(c.limits.MaxSelectedOccurrences) || rawChildren+uint64(r.childCount) > uint64(c.limits.MaxSelectedOccurrences) || occurrences >= math.MaxUint32 || rawChildren+uint64(r.childCount) > math.MaxUint32 {
			return errors.New("parser-core phase zero: native projection occurrence cap")
		}
		if err := c.validateMaterializationMetadata(id, r); err != nil {
			return err
		}
		if r.missing || r.symbol == ErrorRegionSymbol {
			return errors.New("parser-core phase zero: native projection requires a clean derivation")
		}
		if uint64(len(logical)) > math.MaxUint32 {
			return errors.New("parser-core phase zero: native logical frontier exceeds arena")
		}
		switch colors[id] {
		case 0:
		case 1:
			return errors.New("parser-core phase zero: compact subtree cycle during materialization")
		default:
			return errors.New("parser-core phase zero: compact subtree has repeated public-tree ownership")
		}
		colors[id] = 1
		pre := replay.RootPre
		if replay.Tiling != nil {
			if err := replay.Tiling.validateExtent(source, r.startByte, r.endByte); err != nil {
				return err
			}
		}
		if len(stack) != 0 {
			parent := &stack[len(stack)-1]
			pre = parent.cursor
			if replay.Tiling != nil {
				if !parent.root {
					if err := replay.Tiling.validateGap(source, parent.lastEnd, r.startByte); err != nil {
						return err
					}
				}
				parent.lastEnd = max(parent.lastEnd, r.endByte)
			}
		}
		var state StateID
		var known bool
		if replay.NativeTransition != nil {
			state, known = replay.NativeTransition.ReplayNativeTransition(pre, r.symbol, r.terminal)
		} else {
			c.fillMaterializationReplayViewForRecord(id, r, &rs.replayView)
			var err error
			state, known, err = replay.Transition(pre, &rs.replayView)
			if err != nil {
				return err
			}
		}
		if len(stack) != 0 {
			stack[len(stack)-1].cursor = state
		}
		occurrences++
		rawChildren += uint64(r.childCount)
		stack = append(stack, nativeProjectionFrame{
			id: uint32(id), logicalStart: uint32(len(logical)), lastEnd: r.startByte,
			rawPrecedence: int32(r.dynamicPrecedence),
			pre:           pre, cursor: pre, state: state, stateKnown: known, preKnown: known && !r.extra,
			alias: alias, field: field,
			root: replay.Tiling != nil && replay.Tiling.HasRootSymbol && r.symbol == replay.Tiling.RootSymbol,
		})
		return nil
	}
	emit := func(r NativeProjectionRecord, children []SelectedNodeID) (SelectedNodeID, error) {
		if emitted+1 > uint64(c.limits.MaxSelectedOccurrences) || emitted >= math.MaxUint32 {
			return 0, errors.New("parser-core phase zero: native projected node cap")
		}
		id, err := sink.Emit(r, children)
		if err != nil {
			return 0, err
		}
		emitted++
		if uint64(id) != emitted {
			return 0, errors.New("parser-core phase zero: native projection sink handle drift")
		}
		return id, nil
	}
	applyField := func(ids []SelectedNodeID, field FieldID) error {
		// A deeper direct entry owns this field when it already occurs in
		// the span. Otherwise the enclosing direct entry carries through
		// every unassigned child of the hidden slot, including punctuation.
		// Extras do not receive fields. Inherited entries remain outside
		// this publisher's admitted field profile.
		for _, id := range ids {
			r, err := sink.Read(id)
			if err != nil {
				return err
			}
			if r.Field == field {
				return nil
			}
		}
		for _, id := range ids {
			r, err := sink.Read(id)
			if err != nil {
				return err
			}
			if r.Field != 0 || r.Extra() {
				continue
			}
			r.Field = field
			if err := sink.Rewrite(id, r); err != nil {
				return err
			}
		}
		return nil
	}
	wrap := func(childID SelectedNodeID, alias Symbol, field FieldID, production uint16, source SubtreeID) (SelectedNodeID, error) {
		child, err := sink.Read(childID)
		if err != nil {
			return 0, err
		}
		meta, ok := policy.symbol(alias)
		if !ok || !meta.Visible {
			return 0, errors.New("parser-core phase zero: native alias wrapper is not visible")
		}
		r := NativeProjectionRecord{Source: source, StartByte: child.StartByte, EndByte: child.EndByte, Symbol: alias, Field: field, ProductionID: production, ChildCount: 1, DynamicPrecedence: child.DynamicPrecedence, ReplayPre: child.ReplayPre, ReplayState: child.ReplayState, Flags: uint16(selectedFlags(meta, false, false, false, false, child.HasError()))}
		r.Flags |= child.Flags & (nativeProjectionPreKnown | nativeProjectionStateKnown)
		var one [1]SelectedNodeID
		one[0] = childID
		return emit(r, one[:])
	}
	for _, root := range roots {
		begin := len(logical)
		if err := push(root, 0, 0); err != nil {
			return nil, err
		}
		for len(stack) != 0 {
			work++
			if work&255 == 0 {
				save()
				if err := poll(); err != nil {
					return nil, err
				}
			}
			index := len(stack) - 1
			frame := &stack[index]
			payload := SubtreeID(frame.id)
			raw, err := c.subtree(payload)
			if err != nil {
				return nil, err
			}
			if frame.next < raw.childCount {
				ordinal := frame.next
				frame.next++
				var alias Symbol
				if raw.aliasCount != 0 {
					if raw.aliasCount != raw.childCount {
						return nil, errors.New("parser-core phase zero: native alias width drifted")
					}
					alias = c.aliases[raw.firstAlias+ordinal]
				}
				var field FieldID
				for _, f := range c.fields[raw.firstField : raw.firstField+raw.fieldCount] {
					if uint32(f.ChildIndex) == ordinal {
						if f.Inherited || field != 0 {
							return nil, errors.New("parser-core phase zero: native projection requires direct single fields")
						}
						field = f.FieldID
					}
				}
				if err := push(c.children[raw.firstChild+ordinal], alias, field); err != nil {
					return nil, err
				}
				continue
			}
			rf := *frame
			if replay.Tiling != nil && !raw.terminal && !rf.root {
				if err := replay.Tiling.validateGap(source, rf.lastEnd, raw.endByte); err != nil {
					return nil, err
				}
			}
			view := &rs.view
			if replay.VisitRaw != nil || raw.terminal && (replay.CheckpointsRequired || replay.VisitTerminal != nil) {
				c.fillMaterializationSubtreeView(payload, raw, view)
				view.ReplayPreGotoState, view.ReplayParseState = rf.pre, rf.state
				view.ReplayPreGotoKnown, view.ReplayParseStateKnown = rf.preKnown, rf.stateKnown
				if replay.VisitRaw != nil {
					if err := replay.VisitRaw(payload, view); err != nil {
						return nil, err
					}
				}
				if raw.terminal && replay.VisitTerminal != nil {
					if err := replay.VisitTerminal(payload, view); err != nil {
						return nil, err
					}
				}
			}
			replayFlags := uint16(0)
			if rf.preKnown {
				replayFlags |= nativeProjectionPreKnown
			}
			if rf.stateKnown {
				replayFlags |= nativeProjectionStateKnown
			}
			start := int(rf.logicalStart)
			alias, field := rf.alias, rf.field
			retain := !raw.extra && policy.retainsAliasChild(alias, raw.symbol)
			symbol := raw.symbol
			if alias != 0 && !retain {
				symbol = alias
			}
			meta, ok := policy.symbol(symbol)
			if !ok {
				return nil, fmt.Errorf("parser-core phase zero: native symbol %d outside policy", symbol)
			}
			folded := false
			if !raw.terminal && raw.childCount == 1 && raw.fieldCount == 0 && raw.aliasCount == 0 && len(logical)-start == 1 && colors[c.children[raw.firstChild]]&rawNodeIsProjected != 0 {
				id := logical[start]
				child, err := sink.Read(id)
				if err != nil {
					return nil, err
				}
				rule := policy.unary(raw.symbol, child.Symbol)
				if !child.Extra() && (rule == SelectedUnaryPass || rule == SelectedUnaryRenameLeaf && child.ChildCount == 0) {
					if child.StartByte != raw.startByte || child.EndByte != raw.endByte {
						return nil, errors.New("parser-core phase zero: native unary span differs from raw geometry")
					}
					if rule == SelectedUnaryPass && raw.symbol != child.Symbol {
						child.SupertypeMask |= policy.supertypeMask(raw.symbol)
					}
					if rule == SelectedUnaryRenameLeaf {
						child.Symbol = raw.symbol
						child.Flags = uint16(selectedFlags(policy.Symbols[raw.symbol], child.Extra(), child.External(), child.Terminal(), child.Missing(), child.HasError())) | child.Flags&nativeProjectionFragile
					}
					if alias != 0 && !retain {
						child.Symbol = alias
						child.Flags = uint16(selectedFlags(meta, child.Extra(), child.External(), child.Terminal(), child.Missing(), child.HasError())) | child.Flags&nativeProjectionFragile
					}
					child.ReplayPre, child.ReplayState = rf.pre, rf.state
					child.Flags = child.Flags&^(nativeProjectionPreKnown|nativeProjectionStateKnown) | replayFlags
					child.ProductionID = raw.productionID
					child.Source = payload
					child.DynamicPrecedence = rf.rawPrecedence
					if raw.fragile {
						child.Flags |= nativeProjectionFragile
					}
					if field != 0 && !retain {
						child.Field = field
					}
					if err := sink.Rewrite(id, child); err != nil {
						return nil, err
					}
					folded = true
				}
			}
			if !folded && !meta.Visible {
				if raw.terminal && replay.CheckpointsRequired && view.ExternalScannerCheckpointExact && view.ExternalScannerCheckpointStart != 0 && view.ExternalScannerCheckpointEnd != 0 {
					return nil, errors.New("parser-core phase zero: native projection would hide a scanner checkpoint")
				}

				// Hidden wrappers do not contribute their own precedence after
				// elision, matching buildSelectedStoreOnePass and native flattening.
				if mask := policy.supertypeMask(raw.symbol); mask != 0 {
					for _, id := range logical[start:] {
						r, err := sink.Read(id)
						if err != nil {
							return nil, err
						}
						r.SupertypeMask |= mask
						if err := sink.Rewrite(id, r); err != nil {
							return nil, err
						}
					}
				}
			}
			if !folded && meta.Visible {
				children := logical[start:]
				if len(children) > math.MaxInt32 {
					return nil, errors.New("parser-core phase zero: native projected child count exceeds int32")
				}
				r := NativeProjectionRecord{Source: payload, Symbol: symbol, Field: field, ProductionID: raw.productionID, StartByte: raw.startByte, EndByte: raw.endByte, DynamicPrecedence: rf.rawPrecedence, ChildCount: uint32(len(children)), Flags: uint16(selectedFlags(meta, raw.extra, raw.external, raw.terminal, false, false))}
				r.ReplayPre, r.ReplayState = rf.pre, rf.state
				r.Flags |= replayFlags
				if retain {
					r.Field = 0
				}
				if raw.fragile {
					r.Flags |= nativeProjectionFragile
				}
				for _, id := range children {
					child, err := sink.Read(id)
					if err != nil {
						return nil, err
					}
					if child.StartByte < raw.startByte || child.EndByte > raw.endByte {
						return nil, errors.New("parser-core phase zero: native child extends outside raw parent")
					}
					if child.StartByte < r.StartByte {
						r.StartByte = child.StartByte
					}
					if child.EndByte > r.EndByte {
						r.EndByte = child.EndByte
					}

				}
				id, err := emit(r, children)
				if err != nil {
					return nil, err
				}
				logical = logical[:start]
				logical = append(logical, id)
				folded = true
			}
			if folded && retain {
				id, err := wrap(logical[start], alias, field, raw.productionID, payload)
				if err != nil {
					return nil, err
				}
				logical[start] = id
			}
			if field != 0 && !retain {
				if err := applyField(logical[start:], field); err != nil {
					return nil, err
				}
			}
			colors[payload] = 2
			if folded {
				colors[payload] |= rawNodeIsProjected
			}
			if index > 0 {
				parent := &stack[index-1]
				parent.rawPrecedence, err = checkedSelectedPrecedence(parent.rawPrecedence, rf.rawPrecedence)
				if err != nil {
					return nil, err
				}
			}
			stack = stack[:index]
		}
		rootIDs = append(rootIDs, logical[begin:]...)
		logical = logical[:begin]
	}
	save()
	if err := poll(); err != nil {
		return nil, err
	}
	return rootIDs, nil
}
