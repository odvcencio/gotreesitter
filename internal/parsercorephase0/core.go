// Package parsercorephase0 contains a diagnostic-only parser-core prototype.
//
// It deliberately is not imported by the production parser. The prototype
// consumes a dependency-neutral TableView, but it does not own a lexer, an
// external-scanner election, recovery, retries, included ranges, or
// incremental parsing. A future build-tagged diagnostic driver in the root
// package may adapt canonical production tables while independently scheduling
// against the exact root lexer/scanner election; the ordinary production
// parser does not import this package. Differential replay is debugging
// evidence, not the execution route. Exact scanner/election integration remains
// required before any full-parse timing claim. Callers must treat a decline as
// a request to use the production parser; this package never silently
// substitutes partial work.
package parsercorephase0

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

// Symbol and StateID are grammar-table identifiers. They intentionally use
// the same widths as tree-sitter language blobs without depending on the
// public gotreesitter package.
type Symbol uint16
type StateID uint32
type FieldID uint16

// ActionType identifies a decoded parse-table action.
type ActionType uint8

const (
	ActionShift ActionType = iota
	ActionReduce
	ActionAccept
	ActionRecover
)

// Action is the lex-neutral action record consumed by the compact core.
type Action struct {
	Type              ActionType
	State             StateID
	Symbol            Symbol
	ChildCount        uint8
	DynamicPrecedence int16
	ProductionID      uint16
	Extra             bool
	ExtraChain        bool
	Repetition        bool
}

// FieldMapEntry is the production metadata required while materializing a
// compact reduction payload.
type FieldMapEntry struct {
	FieldID    FieldID
	ChildIndex uint8
	Inherited  bool
}

// TableView is the dependency-neutral parser-table boundary. The adapter owns
// table decoding and grammar authentication; the compact core only requests
// semantic cells and reduction metadata.
type TableView interface {
	Actions(StateID, Symbol) ([]Action, error)
	Goto(StateID, Symbol) (StateID, error)
	ProductionFields(uint16, int) ([]FieldMapEntry, error)
	ProductionAliases(uint16, int) ([]Symbol, error)
}

// Decline identifies a feature that phase zero cannot execute faithfully.
type Decline string

const (
	DeclineExtras Decline = "reduction_extras"
)

// DeclineError is returned instead of weakening a phase-zero boundary.
type DeclineError struct {
	Feature Decline
	Detail  string
}

func (e *DeclineError) Error() string {
	if e == nil || e.Detail == "" {
		return "parser-core phase zero declined " + string(e.Feature)
	}
	return "parser-core phase zero declined " + string(e.Feature) + ": " + e.Detail
}

// IsDecline reports whether err is a phase-zero fail-closed decline.
func IsDecline(err error, feature Decline) bool {
	var decline *DeclineError
	return errors.As(err, &decline) && decline.Feature == feature
}

// Limits bound every pointer-free arena and the exact derivation fanout.
type Limits struct {
	MaxNodes            uint32
	MaxLinks            uint32
	MaxSubtrees         uint32
	MaxChildren         uint32
	MaxMetadata         uint32
	MaxPathsPerBoundary uint64
	MaxEnumeration      uint64
}

func (l Limits) withDefaults() Limits {
	if l.MaxNodes == 0 {
		l.MaxNodes = 4096
	}
	if l.MaxLinks == 0 {
		l.MaxLinks = 8192
	}
	if l.MaxSubtrees == 0 {
		l.MaxSubtrees = 8192
	}
	if l.MaxChildren == 0 {
		l.MaxChildren = 32768
	}
	if l.MaxMetadata == 0 {
		l.MaxMetadata = 32768
	}
	if l.MaxPathsPerBoundary == 0 {
		l.MaxPathsPerBoundary = 6
	}
	if l.MaxEnumeration == 0 {
		l.MaxEnumeration = l.MaxPathsPerBoundary
	}
	return l
}

// NodeID, LinkID, and SubtreeID are one-based indexes into pointer-free arenas.
type NodeID uint32
type LinkID uint32
type SubtreeID uint32

type boundaryKey struct {
	frontier   uint64
	state      StateID
	byteOffset uint32
	shifted    bool
	checkpoint [32]byte
}

type nodeRecord struct {
	state      StateID
	byteOffset uint32
	firstLink  uint32
	linkCount  uint32
	pathCount  uint64
}

type linkRecord struct {
	scoreDelta int64
	order      uint64
	prev       NodeID
	payload    SubtreeID
	next       LinkID
	flags      uint32
}

const linkFlagHasOrder uint32 = 1 << iota

func (l linkRecord) hasOrder() bool { return l.flags&linkFlagHasOrder != 0 }

type subtreeRecord struct {
	symbol            Symbol
	productionID      uint16
	dynamicPrecedence int16
	startByte         uint32
	endByte           uint32
	firstChild        uint32
	childCount        uint32
	firstField        uint32
	fieldCount        uint32
	firstAlias        uint32
	aliasCount        uint32
	extra             bool
	external          bool
	terminal          bool
}

// pathMeta is stored on a graph link. ScoreDelta includes the contributions
// collapsed into that payload; BranchOrder optionally overrides the current
// path-local order when an authenticated dispatch event created a fork.
type pathMeta struct {
	ScoreDelta  int64
	BranchOrder ForkOrder
}

// ForkOrder is an externally authenticated current-order override produced by
// a real dispatch fork event. Table cardinality and action ordinal are not
// sufficient to infer fork creation, so phase zero never invents one.
type ForkOrder struct {
	Value   uint64
	Present bool
}

// Token describes a parser-core terminal payload. Lexing is intentionally out
// of scope; the symbol and span must already be authenticated by the caller.
type Token struct {
	Symbol    Symbol
	StartByte uint32
	EndByte   uint32
	Extra     bool
	External  bool
}

// OrdinaryCohortShiftInput identifies one distinct scheduler head and its
// authenticated single shift action. The operation deliberately accepts no
// payload ID: sharing is confined to one validated cohort election.
type OrdinaryCohortShiftInput struct {
	Head          Head
	ActionOrdinal int
}

// Head is a compact parse-version head referencing a persistent graph node.
type Head struct {
	Node NodeID
}

// Derivation is one exact root-to-head path. No score/order selection occurs
// while alternatives are condensed.
type Derivation struct {
	Payloads       []SubtreeID
	Score          int64
	BranchOrder    uint64
	HasBranchOrder bool
}

// SubtreeView exposes immutable reduction identity for tests and diagnostics.
type SubtreeView struct {
	Symbol            Symbol
	ProductionID      uint16
	DynamicPrecedence int16
	StartByte         uint32
	EndByte           uint32
	Children          []SubtreeID
	Fields            []FieldMapEntry
	Aliases           []Symbol
	Extra             bool
	External          bool
	Terminal          bool
}

// Stats reports physical storage separately from semantic path counts. It is
// not a replacement for production work-count emissions.
type Stats struct {
	Nodes             uint32
	Links             uint32
	Subtrees          uint32
	Children          uint32
	CurrentExactPaths uint64
}

// Core is the compact, persistent diagnostic graph. All records are indexes
// into pointer-free slices; the production parser is unaffected.
type Core struct {
	tables          TableView
	limits          Limits
	nodes           []nodeRecord
	links           []linkRecord
	subtrees        []subtreeRecord
	children        []SubtreeID
	fields          []FieldMapEntry
	aliases         []Symbol
	frontier        uint64
	checkpoint      [32]byte
	boundaries      map[boundaryKey]NodeID
	boundaryJournal []boundaryMutation
	transactions    []uint64
	nextTransaction uint64
}

type checkpoint struct {
	nodes, links, subtrees, children, fields, aliases int
	frontier                                          uint64
	checkpoint                                        [32]byte
	journal                                           int
	transaction                                       uint64
}

func (c *Core) mark() checkpoint {
	if c.nextTransaction == math.MaxUint64 {
		panic("parser-core phase zero: transaction identity overflow")
	}
	c.nextTransaction++
	mark := checkpoint{
		nodes: len(c.nodes), links: len(c.links), subtrees: len(c.subtrees),
		children: len(c.children), fields: len(c.fields), aliases: len(c.aliases),
		frontier: c.frontier, checkpoint: c.checkpoint,
		journal: len(c.boundaryJournal), transaction: c.nextTransaction,
	}
	c.transactions = append(c.transactions, mark.transaction)
	return mark
}

func (c *Core) restore(mark checkpoint) {
	c.assertTopTransaction(mark)
	c.nodes = c.nodes[:mark.nodes]
	c.links = c.links[:mark.links]
	c.subtrees = c.subtrees[:mark.subtrees]
	c.children = c.children[:mark.children]
	c.fields = c.fields[:mark.fields]
	c.aliases = c.aliases[:mark.aliases]
	c.frontier = mark.frontier
	c.checkpoint = mark.checkpoint
	for index := len(c.boundaryJournal) - 1; index >= mark.journal; index-- {
		mutation := c.boundaryJournal[index]
		if mutation.existed {
			c.boundaries[mutation.key] = mutation.previous
		} else {
			delete(c.boundaries, mutation.key)
		}
	}
	c.boundaryJournal = c.boundaryJournal[:mark.journal]
	c.finishTransaction()
}

func (c *Core) commit(mark checkpoint) {
	c.assertTopTransaction(mark)
	c.finishTransaction()
}

func (c *Core) assertTopTransaction(mark checkpoint) {
	if len(c.transactions) == 0 || c.transactions[len(c.transactions)-1] != mark.transaction {
		panic("parser-core phase zero: transaction checkpoint used out of LIFO order")
	}
}

func (c *Core) finishTransaction() {
	c.transactions = c.transactions[:len(c.transactions)-1]
	if len(c.transactions) == 0 {
		c.boundaryJournal = c.boundaryJournal[:0]
	}
}

func (c *Core) completeTransaction(mark checkpoint, err *error) {
	if recovered := recover(); recovered != nil {
		c.restore(mark)
		panic(recovered)
	}
	if *err != nil {
		c.restore(mark)
	} else {
		c.commit(mark)
	}
}

type boundaryMutation struct {
	key      boundaryKey
	previous NodeID
	existed  bool
}

func (c *Core) writeBoundary(key boundaryKey, id NodeID) {
	previous, existed := c.boundaries[key]
	if len(c.transactions) != 0 {
		c.boundaryJournal = append(c.boundaryJournal, boundaryMutation{key: key, previous: previous, existed: existed})
	}
	c.boundaries[key] = id
}

func New(tables TableView, limits Limits) (*Core, error) {
	if tables == nil {
		return nil, errors.New("parser-core phase zero: nil table view")
	}
	limits = limits.withDefaults()
	if limits.MaxPathsPerBoundary > limits.MaxEnumeration {
		return nil, fmt.Errorf("parser-core phase zero: path cap %d exceeds enumeration cap %d", limits.MaxPathsPerBoundary, limits.MaxEnumeration)
	}
	return &Core{tables: tables, limits: limits, frontier: 1, boundaries: make(map[boundaryKey]NodeID)}, nil
}

// BeginFrontier starts one authenticated election/dispatch generation.
// Boundary condensation never crosses generations, even when state and byte
// position repeat. Existing heads remain valid persistent predecessors.
func (c *Core) BeginFrontier() error {
	if c.frontier == math.MaxUint64 {
		return errors.New("parser-core phase zero: frontier epoch overflow")
	}
	c.frontier++
	c.checkpoint = [32]byte{}
	return nil
}

// SetPhaseCheckpoint binds subsequent condensation to the exact scanner
// checkpoint for the current lookahead epoch. It does not advance the epoch.
func (c *Core) SetPhaseCheckpoint(checkpoint [32]byte) { c.checkpoint = checkpoint }

func (c *Core) boundaryKey(state StateID, byteOffset uint32) boundaryKey {
	return boundaryKey{frontier: c.frontier, state: state, byteOffset: byteOffset, checkpoint: c.checkpoint}
}

func (c *Core) shiftedBoundaryKey(state StateID, byteOffset uint32) boundaryKey {
	return boundaryKey{frontier: c.frontier, state: state, byteOffset: byteOffset, shifted: true, checkpoint: c.checkpoint}
}

// Seed creates one empty derivation at a parser boundary.
func (c *Core) Seed(state StateID, byteOffset uint32) (Head, error) {
	key := c.boundaryKey(state, byteOffset)
	if id := c.boundaries[key]; id != 0 {
		return Head{Node: id}, nil
	}
	id, err := c.appendNode(nodeRecord{state: state, byteOffset: byteOffset, pathCount: 1})
	if err != nil {
		return Head{}, err
	}
	c.writeBoundary(key, id)
	return Head{Node: id}, nil
}

// Boundary returns the parser state and byte offset represented by head.
// The tagged root scheduler uses it to continue independently after a reduce
// returns one or more condensed boundaries.
func (c *Core) Boundary(head Head) (StateID, uint32, error) {
	node, err := c.node(head.Node)
	if err != nil {
		return 0, 0, err
	}
	return node.state, node.byteOffset, nil
}

// CanonicalBoundary returns the latest condensed head for one complete
// same-lookahead scheduler phase identity. Headers use it at pass barriers to
// replace stale immutable NodeIDs without changing their first-slot order.
func (c *Core) CanonicalBoundary(state StateID, byteOffset uint32, consumed bool, checkpoint [32]byte) (Head, bool) {
	id := c.boundaries[boundaryKey{
		frontier: c.frontier, state: state, byteOffset: byteOffset,
		shifted: consumed, checkpoint: checkpoint,
	}]
	return Head{Node: id}, id != 0
}

// ApplyAtomic rolls back every compact arena and boundary mutation if fn
// fails. Scheduler conflict cells use it because per-action rollback is not
// sufficient after an earlier secondary action succeeded.
func (c *Core) ApplyAtomic(fn func() error) (err error) {
	if fn == nil {
		return errors.New("parser-core phase zero: nil atomic operation")
	}
	mark := c.mark()
	defer c.completeTransaction(mark, &err)
	err = fn()
	return err
}

// Actions returns the authentic decoded action entry for (state, lookahead).
func (c *Core) Actions(state StateID, lookahead Symbol) ([]Action, error) {
	return c.tables.Actions(state, lookahead)
}

// Shift applies one authentic decoded shift action and condenses the resulting
// exact path at its (state, byte) boundary.
func (c *Core) Shift(head Head, lookahead Symbol, actionOrdinal int, token Token, fork ForkOrder) (out Head, err error) {
	mark := c.mark()
	defer c.completeTransaction(mark, &err)
	act, err := c.action(head, lookahead, actionOrdinal)
	if err != nil {
		return Head{}, err
	}
	if act.Type != ActionShift {
		return Head{}, fmt.Errorf("parser-core phase zero: action %d is %v, not shift", actionOrdinal, act.Type)
	}
	if token.Symbol != lookahead {
		return Head{}, fmt.Errorf("parser-core phase zero: token symbol %d != lookahead %d", token.Symbol, lookahead)
	}
	if token.Extra != act.Extra {
		return Head{}, fmt.Errorf("parser-core phase zero: token extra=%t disagrees with decoded action extra=%t", token.Extra, act.Extra)
	}
	current, err := c.node(head.Node)
	if err != nil {
		return Head{}, err
	}
	targetState := act.State
	if act.Extra && targetState == 0 {
		// Match production extraShiftTargetState: an extra shift with target
		// zero leaves the LR state unchanged.
		targetState = current.state
	}
	payload, err := c.appendSubtree(subtreeRecord{
		symbol: token.Symbol, startByte: token.StartByte, endByte: token.EndByte,
		extra: act.Extra, external: token.External, terminal: true,
	}, nil, nil, nil)
	if err != nil {
		return Head{}, err
	}
	return c.condense(c.shiftedBoundaryKey(targetState, token.EndByte), linkInput{
		prev: head.Node, payload: payload, order: fork,
	})
}

// ShiftOrdinaryCohort applies one ordinary terminal election to distinct
// scheduler heads while allocating exactly one immutable terminal payload.
// It is intentionally narrower than Shift: every cell must contain exactly
// one undecorated non-extra shift and the token must have positive width.
// Missing, no-lookahead, reused, and scanner-checkpoint identity remain
// caller-visible concerns. External provenance is one compact identity bit;
// scanner state remains outside this layer.
func (c *Core) ShiftOrdinaryCohort(inputs []OrdinaryCohortShiftInput, lookahead Symbol, token Token) (out []Head, err error) {
	if len(inputs) == 0 {
		return nil, errors.New("parser-core phase zero: empty ordinary shift cohort")
	}
	if token.Symbol != lookahead {
		return nil, fmt.Errorf("parser-core phase zero: token symbol %d != lookahead %d", token.Symbol, lookahead)
	}
	if token.Extra || token.EndByte <= token.StartByte {
		return nil, errors.New("parser-core phase zero: cohort token is not an ordinary positive-width terminal")
	}
	targets := make([]StateID, len(inputs))
	seen := make(map[NodeID]struct{}, len(inputs))
	for index, input := range inputs {
		if _, duplicate := seen[input.Head.Node]; duplicate {
			return nil, fmt.Errorf("parser-core phase zero: duplicate ordinary cohort head %d", input.Head.Node)
		}
		seen[input.Head.Node] = struct{}{}
		target, err := c.ordinaryCohortShiftTarget(input, lookahead)
		if err != nil {
			return nil, err
		}
		targets[index] = target
	}

	err = c.ApplyAtomic(func() error {
		payload, err := c.appendSubtree(subtreeRecord{
			symbol: token.Symbol, startByte: token.StartByte, endByte: token.EndByte,
			external: token.External, terminal: true,
		}, nil, nil, nil)
		if err != nil {
			return err
		}
		out = make([]Head, len(inputs))
		for index, input := range inputs {
			out[index], err = c.condense(c.shiftedBoundaryKey(targets[index], token.EndByte), linkInput{
				prev: input.Head.Node, payload: payload,
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Core) ordinaryCohortShiftTarget(input OrdinaryCohortShiftInput, lookahead Symbol) (StateID, error) {
	current, err := c.node(input.Head.Node)
	if err != nil {
		return 0, err
	}
	actions, err := c.Actions(current.state, lookahead)
	if err != nil {
		return 0, err
	}
	if len(actions) != 1 || input.ActionOrdinal != 0 {
		return 0, fmt.Errorf("parser-core phase zero: head %d does not select one ordinary shift", input.Head.Node)
	}
	action := actions[0]
	if action.State == 0 || action != (Action{Type: ActionShift, State: action.State}) {
		return 0, fmt.Errorf("parser-core phase zero: head %d does not select one ordinary shift", input.Head.Node)
	}
	return action.State, nil
}

// appendDiagnosticPayload adds an already-authenticated terminal payload. It
// is only a setup seam for exercising real reductions before lexer/election
// integration; it is not a parse action and is deliberately named as such.
func (c *Core) appendDiagnosticPayload(head Head, state StateID, token Token, meta pathMeta) (out Head, err error) {
	mark := c.mark()
	defer c.completeTransaction(mark, &err)
	if _, err := c.node(head.Node); err != nil {
		return Head{}, err
	}
	payload, err := c.appendSubtree(subtreeRecord{
		symbol: token.Symbol, startByte: token.StartByte, endByte: token.EndByte,
		extra: token.Extra, external: token.External, terminal: true,
	}, nil, nil, nil)
	if err != nil {
		return Head{}, err
	}
	return c.condense(c.boundaryKey(state, token.EndByte), linkInput{
		prev: head.Node, payload: payload, scoreDelta: meta.ScoreDelta, order: meta.BranchOrder,
	})
}

// Reduce applies one authentic decoded reduction to every exact pop path in
// head. Equivalent boundaries are condensed without discarding derivations.
func (c *Core) Reduce(head Head, lookahead Symbol, actionOrdinal int, fork ForkOrder) (frontier []Head, err error) {
	mark := c.mark()
	defer c.completeTransaction(mark, &err)
	act, err := c.action(head, lookahead, actionOrdinal)
	if err != nil {
		return nil, err
	}
	if act.Type != ActionReduce {
		return nil, fmt.Errorf("parser-core phase zero: action %d is %v, not reduce", actionOrdinal, act.Type)
	}
	if _, err := c.node(head.Node); err != nil {
		return nil, err
	}
	paths, err := c.popPaths(head.Node, int(act.ChildCount))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, errors.New("parser-core phase zero: reduction has no exact pop path")
	}
	frontierByBoundary := make(map[boundaryKey]Head)
	var boundaryOrder []boundaryKey
	var batchParents []SubtreeID
	for _, path := range paths {
		prev, err := c.node(path.prev)
		if err != nil {
			return nil, err
		}
		gotoState, err := c.tables.Goto(prev.state, act.Symbol)
		if err != nil {
			return nil, err
		}
		if gotoState == 0 {
			return nil, fmt.Errorf("parser-core phase zero: no goto from state %d for reduced symbol %d", prev.state, act.Symbol)
		}
		key := c.boundaryKey(gotoState, path.structuralEnd)
		payload, scoreDelta, order, err := c.reductionParentForPath(act, path, key, fork, &batchParents)
		if err != nil {
			return nil, err
		}
		parentLink := linkInput{
			prev: path.prev, payload: payload,
			scoreDelta: scoreDelta, order: order,
		}
		var out Head
		if len(path.trailing) == 0 {
			out, err = c.condense(key, parentLink)
		} else {
			out, err = c.appendPrivate(gotoState, path.structuralEnd, parentLink)
		}
		if err != nil {
			return nil, err
		}
		for index, trailing := range path.trailing {
			extra, err := c.subtree(trailing.payload)
			if err != nil {
				return nil, err
			}
			key = c.boundaryKey(gotoState, extra.endByte)
			extraLink := linkInput{prev: out.Node, payload: trailing.payload, scoreDelta: trailing.scoreDelta}
			if index == len(path.trailing)-1 {
				out, err = c.condense(key, extraLink)
			} else {
				out, err = c.appendPrivate(gotoState, extra.endByte, extraLink)
			}
			if err != nil {
				return nil, err
			}
		}
		if _, seen := frontierByBoundary[key]; !seen {
			boundaryOrder = append(boundaryOrder, key)
		}
		frontierByBoundary[key] = out
	}
	frontier = make([]Head, 0, len(boundaryOrder))
	for _, key := range boundaryOrder {
		frontier = append(frontier, frontierByBoundary[key])
	}
	return frontier, nil
}

func (c *Core) reductionParentForPath(
	act Action,
	path popPath,
	key boundaryKey,
	fork ForkOrder,
	batchParents *[]SubtreeID,
) (SubtreeID, int64, ForkOrder, error) {
	fields, err := c.tables.ProductionFields(act.ProductionID, int(act.ChildCount))
	if err != nil {
		return 0, 0, ForkOrder{}, err
	}
	aliases, err := c.tables.ProductionAliases(act.ProductionID, int(act.ChildCount))
	if err != nil {
		return 0, 0, ForkOrder{}, err
	}
	fields, aliases, err = c.remapProductionMetadata(path.children, int(act.ChildCount), fields, aliases)
	if err != nil {
		return 0, 0, ForkOrder{}, err
	}
	parent := subtreeRecord{
		symbol: act.Symbol, productionID: act.ProductionID,
		dynamicPrecedence: act.DynamicPrecedence,
		startByte:         path.startByte, endByte: path.structuralEnd,
	}
	order := path.order
	if fork.Present {
		order = fork
	}
	scoreDelta, err := checkedAddScore(path.score, int64(act.DynamicPrecedence))
	if err != nil {
		return 0, 0, ForkOrder{}, err
	}
	var payload SubtreeID
	var found bool
	if len(path.trailing) == 0 {
		payload, found, err = c.findReductionBatchParent(*batchParents, parent, path.children, fields, aliases)
		if err != nil {
			return 0, 0, ForkOrder{}, err
		}
		if found {
			collision, err := c.condenseInputExists(key, linkInput{
				prev: path.prev, payload: payload, scoreDelta: scoreDelta, order: order,
			})
			if err != nil {
				return 0, 0, ForkOrder{}, err
			}
			found = !collision
		}
	}
	if !found {
		payload, err = c.appendSubtree(parent, path.children, fields, aliases)
		if err != nil {
			return 0, 0, ForkOrder{}, err
		}
		if len(path.trailing) == 0 {
			*batchParents = append(*batchParents, payload)
		}
	}
	return payload, scoreDelta, order, nil
}

func (c *Core) condenseInputExists(key boundaryKey, in linkInput) (bool, error) {
	id := c.boundaries[key]
	if id == 0 {
		return false, nil
	}
	node, err := c.node(id)
	if err != nil {
		return false, err
	}
	links, err := c.nodeLinks(*node)
	if err != nil {
		return false, err
	}
	for _, link := range links {
		equal, err := c.linkEqualInput(link, in)
		if err != nil {
			return false, err
		}
		if equal {
			return true, nil
		}
	}
	return false, nil
}

func (c *Core) findReductionBatchParent(
	batch []SubtreeID,
	record subtreeRecord,
	children []SubtreeID,
	fields []FieldMapEntry,
	aliases []Symbol,
) (SubtreeID, bool, error) {
	for _, id := range batch {
		stored, err := c.subtree(id)
		if err != nil {
			return 0, false, err
		}
		if reductionParentIdentityEqual(
			*stored,
			c.children[stored.firstChild:stored.firstChild+stored.childCount],
			c.fields[stored.firstField:stored.firstField+stored.fieldCount],
			c.aliases[stored.firstAlias:stored.firstAlias+stored.aliasCount],
			record, children, fields, aliases,
		) {
			return id, true, nil
		}
	}
	return 0, false, nil
}

func reductionParentIdentityEqual(
	left subtreeRecord,
	leftChildren []SubtreeID,
	leftFields []FieldMapEntry,
	leftAliases []Symbol,
	right subtreeRecord,
	rightChildren []SubtreeID,
	rightFields []FieldMapEntry,
	rightAliases []Symbol,
) bool {
	// Arena offsets are storage locations, not subtree identity. Normalize
	// them while deriving each count from the complete ordered sidecar.
	left.firstChild, left.childCount = 0, uint32(len(leftChildren))
	left.firstField, left.fieldCount = 0, uint32(len(leftFields))
	left.firstAlias, left.aliasCount = 0, uint32(len(leftAliases))
	right.firstChild, right.childCount = 0, uint32(len(rightChildren))
	right.firstField, right.fieldCount = 0, uint32(len(rightFields))
	right.firstAlias, right.aliasCount = 0, uint32(len(rightAliases))
	return left == right && slices.Equal(leftChildren, rightChildren) && slices.Equal(leftFields, rightFields) && slices.Equal(leftAliases, rightAliases)
}

func (c *Core) remapProductionMetadata(children []SubtreeID, structuralCount int, fields []FieldMapEntry, aliases []Symbol) ([]FieldMapEntry, []Symbol, error) {
	structuralPositions := make([]int, 0, structuralCount)
	for index, child := range children {
		payload, err := c.subtree(child)
		if err != nil {
			return nil, nil, err
		}
		if !payload.extra {
			structuralPositions = append(structuralPositions, index)
		}
	}
	if len(structuralPositions) != structuralCount {
		return nil, nil, fmt.Errorf("parser-core phase zero: reduction stored %d structural children, want %d", len(structuralPositions), structuralCount)
	}
	remappedFields := make([]FieldMapEntry, len(fields))
	for index, field := range fields {
		if int(field.ChildIndex) >= len(structuralPositions) {
			return nil, nil, fmt.Errorf("parser-core phase zero: field child index %d exceeds structural child count %d", field.ChildIndex, structuralCount)
		}
		actual := structuralPositions[field.ChildIndex]
		if actual > math.MaxUint8 {
			return nil, nil, errors.New("parser-core phase zero: remapped field child index exceeds uint8")
		}
		field.ChildIndex = uint8(actual)
		remappedFields[index] = field
	}
	if len(aliases) == 0 {
		return remappedFields, nil, nil
	}
	if len(aliases) > structuralCount {
		return nil, nil, fmt.Errorf("parser-core phase zero: alias count %d exceeds structural child count %d", len(aliases), structuralCount)
	}
	remappedAliases := make([]Symbol, len(children))
	for index, alias := range aliases {
		remappedAliases[structuralPositions[index]] = alias
	}
	return remappedFields, remappedAliases, nil
}

// appendPrivate adds one exact single-link node without publishing an
// intermediate boundary for cross-path condensation. Reduction paths with
// trailing extras use it so only their final (state, byte) boundary merges.
func (c *Core) appendPrivate(state StateID, byteOffset uint32, in linkInput) (Head, error) {
	prev, err := c.node(in.prev)
	if err != nil {
		return Head{}, err
	}
	if _, err := c.subtree(in.payload); err != nil {
		return Head{}, err
	}
	if prev.pathCount > c.limits.MaxPathsPerBoundary {
		return Head{}, fmt.Errorf("parser-core phase zero: private exact-path cap exceeded: %d > %d", prev.pathCount, c.limits.MaxPathsPerBoundary)
	}
	if uint64(len(c.links))+1 > uint64(c.limits.MaxLinks) || len(c.links) >= math.MaxUint32 {
		return Head{}, errors.New("parser-core phase zero: link arena cap")
	}
	if uint64(len(c.nodes))+1 > uint64(c.limits.MaxNodes) || len(c.nodes) >= math.MaxUint32 {
		return Head{}, errors.New("parser-core phase zero: node arena cap")
	}
	flags := uint32(0)
	if in.order.Present {
		flags |= linkFlagHasOrder
	}
	linkID := LinkID(len(c.links) + 1)
	c.links = append(c.links, linkRecord{
		prev: in.prev, payload: in.payload, scoreDelta: in.scoreDelta,
		order: in.order.Value, flags: flags,
	})
	id, err := c.appendNode(nodeRecord{
		state: state, byteOffset: byteOffset,
		firstLink: uint32(linkID), linkCount: 1, pathCount: prev.pathCount,
	})
	if err != nil {
		return Head{}, err
	}
	return Head{Node: id}, nil
}

func (c *Core) action(head Head, lookahead Symbol, ordinal int) (Action, error) {
	n, err := c.node(head.Node)
	if err != nil {
		return Action{}, err
	}
	actions, err := c.Actions(n.state, lookahead)
	if err != nil {
		return Action{}, err
	}
	if ordinal < 0 || ordinal >= len(actions) {
		return Action{}, fmt.Errorf("parser-core phase zero: action ordinal %d out of range", ordinal)
	}
	return actions[ordinal], nil
}

type linkInput struct {
	prev       NodeID
	payload    SubtreeID
	scoreDelta int64
	order      ForkOrder
}

func (c *Core) condense(key boundaryKey, in linkInput) (Head, error) {
	if key.frontier != c.frontier {
		return Head{}, errors.New("parser-core phase zero: boundary frontier mismatch")
	}
	prev, err := c.node(in.prev)
	if err != nil {
		return Head{}, err
	}
	if _, err := c.subtree(in.payload); err != nil {
		return Head{}, err
	}
	var oldID NodeID
	var old nodeRecord
	var oldLinks []linkRecord
	oldID = c.boundaries[key]
	if oldID != 0 {
		oldRecord, err := c.node(oldID)
		if err != nil {
			return Head{}, err
		}
		old = *oldRecord
		oldLinks, err = c.nodeLinks(old)
		if err != nil {
			return Head{}, err
		}
		for _, link := range oldLinks {
			equal, err := c.linkEqualInput(link, in)
			if err != nil {
				return Head{}, err
			}
			if equal {
				return Head{Node: oldID}, nil
			}
		}
	}
	newPathCount := prev.pathCount
	if oldID != 0 {
		if math.MaxUint64-newPathCount < old.pathCount {
			return Head{}, errors.New("parser-core phase zero: exact path count overflow")
		}
		newPathCount += old.pathCount
	}
	if newPathCount > c.limits.MaxPathsPerBoundary {
		return Head{}, fmt.Errorf("parser-core phase zero: shared (%d,%d) exact-path cap exceeded: %d > %d", key.state, key.byteOffset, newPathCount, c.limits.MaxPathsPerBoundary)
	}
	if uint64(len(c.links))+1 > uint64(c.limits.MaxLinks) || len(c.links) >= math.MaxUint32 {
		return Head{}, errors.New("parser-core phase zero: link arena cap")
	}
	if uint64(len(c.nodes))+1 > uint64(c.limits.MaxNodes) || len(c.nodes) >= math.MaxUint32 {
		return Head{}, errors.New("parser-core phase zero: node arena cap")
	}
	linkCount := uint32(1)
	if oldID != 0 {
		if old.linkCount == math.MaxUint32 {
			return Head{}, errors.New("parser-core phase zero: boundary link count overflow")
		}
		linkCount += old.linkCount
	}
	linkID := LinkID(len(c.links) + 1)
	flags := uint32(0)
	if in.order.Present {
		flags |= linkFlagHasOrder
	}
	c.links = append(c.links, linkRecord{
		prev: in.prev, payload: in.payload, scoreDelta: in.scoreDelta,
		order: in.order.Value, flags: flags, next: LinkID(old.firstLink),
	})
	id, err := c.appendNode(nodeRecord{
		state: key.state, byteOffset: key.byteOffset,
		firstLink: uint32(linkID), linkCount: linkCount, pathCount: newPathCount,
	})
	if err != nil {
		return Head{}, err
	}
	c.writeBoundary(key, id)
	return Head{Node: id}, nil
}

func (c *Core) linkEqualInput(link linkRecord, in linkInput) (bool, error) {
	return link.prev == in.prev && link.payload == in.payload && link.scoreDelta == in.scoreDelta &&
		link.hasOrder() == in.order.Present && (!link.hasOrder() || link.order == in.order.Value), nil
}

type popPath struct {
	prev          NodeID
	children      []SubtreeID
	trailing      []pathPayload
	score         int64
	order         ForkOrder
	startByte     uint32
	structuralEnd uint32
}

type pathPayload struct {
	payload    SubtreeID
	scoreDelta int64
	order      ForkOrder
}

func (c *Core) popPaths(head NodeID, childCount int) ([]popPath, error) {
	if childCount == 0 {
		n, err := c.node(head)
		if err != nil {
			return nil, err
		}
		return []popPath{{prev: head, startByte: n.byteOffset, structuralEnd: n.byteOffset}}, nil
	}
	var out []popPath
	visiting := make(map[NodeID]bool)
	var rev []SubtreeID
	var revScores []int64
	var revOrders []ForkOrder
	var trailingRev []pathPayload
	var walk func(NodeID, int, bool, uint32) error
	walk = func(id NodeID, remaining int, peelingTrailing bool, structuralEnd uint32) error {
		if visiting[id] {
			return errors.New("parser-core phase zero: graph cycle")
		}
		n, err := c.node(id)
		if err != nil {
			return err
		}
		visiting[id] = true
		defer delete(visiting, id)
		links, err := c.nodeLinks(*n)
		if err != nil {
			return err
		}
		for _, link := range links {
			payload, err := c.subtree(link.payload)
			if err != nil {
				return err
			}
			linkOrder := ForkOrder{Value: link.order, Present: link.hasOrder()}
			if payload.extra && peelingTrailing {
				trailingRev = append(trailingRev, pathPayload{payload: link.payload, scoreDelta: link.scoreDelta, order: linkOrder})
				if err := walk(link.prev, remaining, true, structuralEnd); err != nil {
					return err
				}
				trailingRev = trailingRev[:len(trailingRev)-1]
				continue
			}
			if payload.extra {
				rev = append(rev, link.payload)
				revScores = append(revScores, link.scoreDelta)
				revOrders = append(revOrders, linkOrder)
				if err := walk(link.prev, remaining, false, structuralEnd); err != nil {
					return err
				}
				rev = rev[:len(rev)-1]
				revScores = revScores[:len(revScores)-1]
				revOrders = revOrders[:len(revOrders)-1]
				continue
			}
			nextStructuralEnd := structuralEnd
			if peelingTrailing {
				nextStructuralEnd = payload.endByte
			}
			rev = append(rev, link.payload)
			revScores = append(revScores, link.scoreDelta)
			revOrders = append(revOrders, linkOrder)
			next := remaining - 1
			if next == 0 {
				if uint64(len(out)) >= c.limits.MaxEnumeration {
					return errors.New("parser-core phase zero: pop enumeration cap")
				}
				path := popPath{prev: link.prev, startByte: payload.startByte, structuralEnd: nextStructuralEnd}
				for i := len(rev) - 1; i >= 0; i-- {
					path.children = append(path.children, rev[i])
					path.score, err = checkedAddScore(path.score, revScores[i])
					if err != nil {
						return err
					}
					if revOrders[i].Present {
						path.order = revOrders[i]
					}
				}
				for i := len(trailingRev) - 1; i >= 0; i-- {
					path.trailing = append(path.trailing, trailingRev[i])
					if trailingRev[i].order.Present {
						path.order = trailingRev[i].order
					}
				}
				out = append(out, path)
			} else if err := walk(link.prev, next, false, nextStructuralEnd); err != nil {
				return err
			}
			rev = rev[:len(rev)-1]
			revScores = revScores[:len(revScores)-1]
			revOrders = revOrders[:len(revOrders)-1]
		}
		return nil
	}
	if err := walk(head, childCount, true, 0); err != nil {
		return nil, err
	}
	return out, nil
}

// Derivations enumerates the exact alternatives represented by head.
func (c *Core) Derivations(head Head) ([]Derivation, error) {
	var out []Derivation
	visiting := make(map[NodeID]bool)
	var walk func(NodeID) ([]Derivation, error)
	walk = func(id NodeID) ([]Derivation, error) {
		if visiting[id] {
			return nil, errors.New("parser-core phase zero: graph cycle")
		}
		n, err := c.node(id)
		if err != nil {
			return nil, err
		}
		if n.linkCount == 0 {
			if n.pathCount != 1 {
				return nil, errors.New("parser-core phase zero: malformed seed path count")
			}
			return []Derivation{{}}, nil
		}
		visiting[id] = true
		defer delete(visiting, id)
		var paths []Derivation
		links, err := c.nodeLinks(*n)
		if err != nil {
			return nil, err
		}
		for _, link := range links {
			prefixes, err := walk(link.prev)
			if err != nil {
				return nil, err
			}
			for _, prefix := range prefixes {
				if uint64(len(paths)) >= c.limits.MaxEnumeration {
					return nil, errors.New("parser-core phase zero: derivation enumeration cap")
				}
				score, err := checkedAddScore(prefix.Score, link.scoreDelta)
				if err != nil {
					return nil, err
				}
				path := Derivation{Score: score}
				path.Payloads = append(path.Payloads, prefix.Payloads...)
				path.Payloads = append(path.Payloads, link.payload)
				path.BranchOrder = prefix.BranchOrder
				path.HasBranchOrder = prefix.HasBranchOrder
				if link.hasOrder() {
					path.BranchOrder = link.order
					path.HasBranchOrder = true
				}
				paths = append(paths, path)
			}
		}
		if uint64(len(paths)) != n.pathCount {
			return nil, fmt.Errorf("parser-core phase zero: path-count mismatch: enumerated %d, recorded %d", len(paths), n.pathCount)
		}
		return paths, nil
	}
	paths, err := walk(head.Node)
	if err != nil {
		return nil, err
	}
	out = append(out, paths...)
	return out, nil
}

func (c *Core) Subtree(id SubtreeID) (SubtreeView, error) {
	r, err := c.subtree(id)
	if err != nil {
		return SubtreeView{}, err
	}
	view := SubtreeView{
		Symbol: r.symbol, ProductionID: r.productionID, DynamicPrecedence: r.dynamicPrecedence,
		StartByte: r.startByte, EndByte: r.endByte, Extra: r.extra, External: r.external, Terminal: r.terminal,
	}
	view.Children = append(view.Children, c.children[r.firstChild:r.firstChild+r.childCount]...)
	view.Fields = append(view.Fields, c.fields[r.firstField:r.firstField+r.fieldCount]...)
	view.Aliases = append(view.Aliases, c.aliases[r.firstAlias:r.firstAlias+r.aliasCount]...)
	return view, nil
}

func (c *Core) Stats(head Head) (Stats, error) {
	n, err := c.node(head.Node)
	if err != nil {
		return Stats{}, err
	}
	return Stats{
		Nodes: uint32(len(c.nodes)), Links: uint32(len(c.links)), Subtrees: uint32(len(c.subtrees)), Children: uint32(len(c.children)),
		CurrentExactPaths: n.pathCount,
	}, nil
}

func (c *Core) appendNode(r nodeRecord) (NodeID, error) {
	if uint64(len(c.nodes))+1 > uint64(c.limits.MaxNodes) || len(c.nodes) >= math.MaxUint32 {
		return 0, errors.New("parser-core phase zero: node arena cap")
	}
	c.nodes = append(c.nodes, r)
	return NodeID(len(c.nodes)), nil
}

func (c *Core) appendSubtree(r subtreeRecord, children []SubtreeID, fields []FieldMapEntry, aliases []Symbol) (SubtreeID, error) {
	if uint64(len(c.subtrees))+1 > uint64(c.limits.MaxSubtrees) || len(c.subtrees) >= math.MaxUint32 {
		return 0, errors.New("parser-core phase zero: subtree arena cap")
	}
	if uint64(len(c.children))+uint64(len(children)) > uint64(c.limits.MaxChildren) {
		return 0, errors.New("parser-core phase zero: child arena cap")
	}
	if uint64(len(c.fields))+uint64(len(fields)) > uint64(c.limits.MaxMetadata) || uint64(len(c.aliases))+uint64(len(aliases)) > uint64(c.limits.MaxMetadata) {
		return 0, errors.New("parser-core phase zero: metadata arena cap")
	}
	r.firstChild, r.childCount = uint32(len(c.children)), uint32(len(children))
	r.firstField, r.fieldCount = uint32(len(c.fields)), uint32(len(fields))
	r.firstAlias, r.aliasCount = uint32(len(c.aliases)), uint32(len(aliases))
	c.children = append(c.children, children...)
	c.fields = append(c.fields, fields...)
	c.aliases = append(c.aliases, aliases...)
	c.subtrees = append(c.subtrees, r)
	return SubtreeID(len(c.subtrees)), nil
}

func (c *Core) node(id NodeID) (*nodeRecord, error) {
	if id == 0 || uint64(id) > uint64(len(c.nodes)) {
		return nil, fmt.Errorf("parser-core phase zero: invalid node id %d", id)
	}
	return &c.nodes[id-1], nil
}

func (c *Core) subtree(id SubtreeID) (*subtreeRecord, error) {
	if id == 0 || uint64(id) > uint64(len(c.subtrees)) {
		return nil, fmt.Errorf("parser-core phase zero: invalid subtree id %d", id)
	}
	return &c.subtrees[id-1], nil
}

func (c *Core) nodeLinks(n nodeRecord) ([]linkRecord, error) {
	links := make([]linkRecord, 0, n.linkCount)
	id := LinkID(n.firstLink)
	seen := make(map[LinkID]bool, n.linkCount)
	for id != 0 {
		if seen[id] {
			return nil, errors.New("parser-core phase zero: adjacency cycle")
		}
		seen[id] = true
		if uint64(id) > uint64(len(c.links)) {
			return nil, errors.New("parser-core phase zero: link adjacency out of range")
		}
		link := c.links[id-1]
		links = append(links, link)
		if uint64(len(links)) > uint64(n.linkCount) {
			return nil, errors.New("parser-core phase zero: adjacency exceeds recorded link count")
		}
		id = link.next
	}
	if uint32(len(links)) != n.linkCount {
		return nil, errors.New("parser-core phase zero: adjacency shorter than recorded link count")
	}
	// The chain prepends in O(1); callers observe stable insertion order.
	for i, j := 0, len(links)-1; i < j; i, j = i+1, j-1 {
		links[i], links[j] = links[j], links[i]
	}
	return links, nil
}

func checkedAddScore(a, b int64) (int64, error) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, errors.New("parser-core phase zero: score overflow")
	}
	return a + b, nil
}
