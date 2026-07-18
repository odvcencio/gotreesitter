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
	"sync"
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

// ActionRowKind is the immutable dispatch shape of one decoded parse-table
// cell. It classifies only row-intrinsic properties; token-width and EOF
// authentication remain scheduler responsibilities.
type ActionRowKind uint8

const (
	ActionRowEmpty ActionRowKind = iota
	ActionRowShift
	ActionRowExtraShift
	ActionRowReduce
	ActionRowAccept
	ActionRowConflict
	ActionRowUnsupported
)

// ActionRowDescriptor is the precompiled dispatch shape of an ActionRow.
// Keeping it separate from token-dependent validation lets the scheduler avoid
// repeatedly interpreting immutable action records without weakening the
// existing ordering and authentication gates.
type ActionRowDescriptor struct {
	kind      ActionRowKind
	hasShift  bool
	hasReduce bool
}

func (d ActionRowDescriptor) Kind() ActionRowKind { return d.kind }
func (d ActionRowDescriptor) HasShift() bool      { return d.hasShift }
func (d ActionRowDescriptor) HasReduce() bool     { return d.hasReduce }

type actionRowData struct {
	actions    []Action
	descriptor ActionRowDescriptor
}

// ActionRow is an immutable decoded parse-table cell. Its backing storage is
// deliberately hidden so a cached row can be shared across parser-core
// lookups without allowing callers to corrupt later dispatches.
type ActionRow struct {
	data *actionRowData
}

// ClassifiedBoundary is one authenticated action-table classification for a
// compact head in the current scheduler phase. Its owner and monotonically
// advancing phase prevent a cached classification from being replayed against
// another core or after Reset/BeginFrontier has invalidated its arena/frontier
// identity. The action row remains immutable.
type ClassifiedBoundary struct {
	owner      *Core
	actions    ActionRow
	phase      uint64
	head       Head
	state      StateID
	byteOffset uint32
	lookahead  Symbol
}

// Head returns the compact head authenticated by this classification.
func (b ClassifiedBoundary) Head() Head { return b.head }

// State returns the LR state authenticated by this classification.
func (b ClassifiedBoundary) State() StateID { return b.state }

// ByteOffset returns the source byte boundary authenticated by this classification.
func (b ClassifiedBoundary) ByteOffset() uint32 { return b.byteOffset }

// Actions returns the immutable decoded action row for this classification.
func (b ClassifiedBoundary) Actions() ActionRow { return b.actions }

// NewActionRow snapshots actions into an immutable row.
func NewActionRow(actions []Action) ActionRow {
	if len(actions) == 0 {
		return ActionRow{}
	}
	snapshot := append([]Action(nil), actions...)
	return ActionRow{data: &actionRowData{
		actions: snapshot, descriptor: describeActionRow(snapshot),
	}}
}

// Len reports the number of actions in the row.
func (r ActionRow) Len() int {
	if r.data == nil {
		return 0
	}
	return len(r.data.actions)
}

// At returns one action by value, keeping the cached row immutable. Like a
// slice index, it panics when index is outside [0, Len()).
func (r ActionRow) At(index int) Action { return r.data.actions[index] }

// Descriptor returns the immutable row-intrinsic dispatch classification.
func (r ActionRow) Descriptor() ActionRowDescriptor {
	if r.data == nil {
		return ActionRowDescriptor{kind: ActionRowEmpty}
	}
	return r.data.descriptor
}

func (r ActionRow) actionRef(index int) *Action { return &r.data.actions[index] }

func describeActionRow(actions []Action) ActionRowDescriptor {
	descriptor := ActionRowDescriptor{kind: ActionRowUnsupported}
	if len(actions) == 0 {
		descriptor.kind = ActionRowEmpty
		return descriptor
	}
	for _, action := range actions {
		if action.Repetition || action.ExtraChain || action.Type > ActionRecover ||
			action.Type == ActionRecover || action.Extra && (len(actions) != 1 || action.Type != ActionShift) ||
			action.Type == ActionAccept && len(actions) != 1 {
			return descriptor
		}
		descriptor.hasShift = descriptor.hasShift || action.Type == ActionShift
		descriptor.hasReduce = descriptor.hasReduce || action.Type == ActionReduce
	}
	if len(actions) > 1 {
		descriptor.kind = ActionRowConflict
		return descriptor
	}
	switch actions[0].Type {
	case ActionShift:
		if actions[0].Extra {
			descriptor.kind = ActionRowExtraShift
		} else {
			descriptor.kind = ActionRowShift
		}
	case ActionReduce:
		descriptor.kind = ActionRowReduce
	case ActionAccept:
		descriptor.kind = ActionRowAccept
	default:
		descriptor.kind = ActionRowUnsupported
	}
	return descriptor
}

// FieldMapEntry is the production metadata required while materializing a
// compact reduction payload.
type FieldMapEntry struct {
	FieldID    FieldID
	ChildIndex uint8
	Inherited  bool
}

// ReductionPlan is immutable production metadata authenticated for one exact
// (production ID, structural child count) pair. It intentionally is not stored
// on Action: adapters may share one plan across many decoded action cells.
type ReductionPlan struct {
	fields       []FieldMapEntry
	aliases      []Symbol
	productionID uint16
	childCount   uint8
}

// NewReductionPlan snapshots one exact production/child-count pair.
func NewReductionPlan(productionID uint16, childCount int, fields []FieldMapEntry, aliases []Symbol) (ReductionPlan, error) {
	if childCount < 0 || childCount > math.MaxUint8 {
		return ReductionPlan{}, errors.New("parser-core phase zero: reduction plan child count exceeds uint8")
	}
	for _, field := range fields {
		if int(field.ChildIndex) >= childCount {
			return ReductionPlan{}, fmt.Errorf("parser-core phase zero: reduction plan field child index %d exceeds structural child count %d", field.ChildIndex, childCount)
		}
	}
	if len(aliases) > childCount {
		return ReductionPlan{}, fmt.Errorf("parser-core phase zero: reduction plan alias count %d exceeds structural child count %d", len(aliases), childCount)
	}
	return ReductionPlan{
		fields: append([]FieldMapEntry(nil), fields...), aliases: append([]Symbol(nil), aliases...),
		productionID: productionID, childCount: uint8(childCount),
	}, nil
}

// ReductionPlanProvider optionally supplies pair-indexed immutable plans.
// TableView remains source-compatible for small diagnostic adapters.
type ReductionPlanProvider interface {
	ReductionPlan(uint16, int) (ReductionPlan, error)
}

// TableView is the dependency-neutral parser-table boundary. The adapter owns
// table decoding and grammar authentication; the compact core only requests
// semantic cells and reduction metadata.
type TableView interface {
	Actions(StateID, Symbol) (ActionRow, error)
	Goto(StateID, Symbol) (StateID, error)
	ProductionFields(uint16, int) ([]FieldMapEntry, error)
	ProductionAliases(uint16, int) ([]Symbol, error)
}

// Decline identifies a feature that phase zero cannot execute faithfully.
type Decline string

const (
	DeclineExtras Decline = "reduction_extras"
)

// ErrDerivationEnumerationCap marks a diagnostic-only observation limit. It
// must never be used to stop compact syntax execution.
var ErrDerivationEnumerationCap = errors.New("parser-core phase zero: derivation enumeration cap")

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

// LiveLinkCapacityError reports the exact shared boundary whose next distinct
// live link would exceed the configured local fan-out limit.
type LiveLinkCapacityError struct {
	State         StateID
	ByteOffset    uint32
	ObservedLinks uint64
	Limit         uint32
}

func (e *LiveLinkCapacityError) Error() string {
	return fmt.Sprintf("parser-core phase zero: shared (%d,%d) live-link cap exceeded: %d > %d", e.State, e.ByteOffset, e.ObservedLinks, e.Limit)
}

// Limits bound every pointer-free arena and bounded diagnostic traversal.
// Packed root-path multiplicity is telemetry, not an execution limit.
type Limits struct {
	MaxNodes               uint32
	MaxLinks               uint32
	MaxSubtrees            uint32
	MaxChildren            uint32
	MaxMetadata            uint32
	MaxLinksPerBoundary    uint32
	MaxPopPaths            uint64
	MaxDerivations         uint64
	MaxCheckpoints         uint32
	MaxCheckpointBytes     uint64
	MaxSelectedOccurrences uint32
	MaxSelectedBytes       uint64
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
	if l.MaxLinksPerBoundary == 0 {
		l.MaxLinksPerBoundary = 8
	}
	if l.MaxPopPaths == 0 {
		l.MaxPopPaths = 64
	}
	if l.MaxDerivations == 0 {
		l.MaxDerivations = 64
	}
	if l.MaxCheckpoints == 0 {
		l.MaxCheckpoints = 100000
	}
	if l.MaxCheckpointBytes == 0 {
		l.MaxCheckpointBytes = 16 << 20
	}
	if l.MaxSelectedOccurrences == 0 {
		l.MaxSelectedOccurrences = l.MaxSubtrees
	}
	if l.MaxSelectedBytes == 0 {
		l.MaxSelectedBytes = uint64(l.MaxSelectedOccurrences) * 96
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
	checkpoint CheckpointID
	shifted    bool
}

// BoundaryIndexStats reports the diagnostic canonical-boundary index shape.
// CurrentEntries is the active frontier's live width; RetainedEntries includes
// historical frontier entries still held by the index. Reading these counters
// is diagnostic-only and deliberately scans the map rather than taxing the hot
// mutation path.
type BoundaryIndexStats struct {
	Frontier        uint64
	CurrentEntries  uint64
	RetainedEntries uint64
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

// ExtraCohortShiftInput identifies one distinct scheduler head and its
// authenticated single extra shift action. As with ordinary cohorts, payload
// sharing is confined to one completely validated election.
type ExtraCohortShiftInput struct {
	Head          Head
	ActionOrdinal int
}

// Head is a compact parse-version head referencing a persistent graph node.
type Head struct {
	Node NodeID
}

// Derivation is one retained exact root-to-head path after local shallow-link
// precedence selection.
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

// MaterializationSubtreeView is a callback-scoped borrowed view of one compact
// subtree. Children aliases compact arena storage and must not be retained or
// mutated by the visitor. The copying Subtree diagnostic API remains the
// stable inspection surface.
type MaterializationSubtreeView struct {
	Symbol            Symbol
	ProductionID      uint16
	DynamicPrecedence int16
	StartByte         uint32
	EndByte           uint32
	Children          []SubtreeID
	Extra             bool
	External          bool
	Terminal          bool
}

// Stats reports physical storage separately from semantic path counts. It is
// not a replacement for production work-count emissions.
type Stats struct {
	Nodes    uint32
	Links    uint32
	Subtrees uint32
	Children uint32
	// CurrentExactPaths saturates at math.MaxUint64. It is observation only and
	// never drops or selects a syntax alternative.
	CurrentExactPaths uint64
}

// Work reports committed compact-core operations in units that can be
// compared with an instrumented tree-sitter runtime. Every field is
// transactional: failed operations contribute nothing. Counters saturate at
// math.MaxUint64 and set Overflow instead of wrapping.
type Work struct {
	Shifts                                 uint64
	Reductions                             uint64
	ReductionPopRequests                   uint64
	EmittedPopPaths                        uint64
	EmittedPopPayloads                     uint64
	PredecessorLinkUnionAttempts           uint64
	PredecessorLinkUnionDuplicateNoop      uint64
	PredecessorLinkUnionPrecedenceReplaced uint64
	PredecessorLinkUnionRecursiveChanged   uint64
	PredecessorLinkUnionAlternateAppended  uint64
	PredecessorLinkUnionRejected           uint64
	GraphLinkAdditionsProxy                uint64
	LeafConstructionsProxy                 uint64
	ParentConstructionsProxy               uint64
	Overflow                               bool
}

// RawSelectedCensus reports occurrences in the accepted compact syntax graph
// before public-node visibility/collapse rules are applied. Accepted compact
// payload roots do not contain the terminal EOF transport sentinel, matching
// the paired contract's explicit EOF exclusion. It counts occurrences rather
// than unique SubtreeIDs because one immutable compact record may be referenced
// from more than one selected position.
type RawSelectedCensus struct {
	Nodes    uint64
	Parents  uint64
	Leaves   uint64
	Overflow bool
}

// Core is the compact, persistent diagnostic graph. All records are indexes
// into pointer-free slices; the production parser is unaffected.
type Core struct {
	tables              TableView
	plans               ReductionPlanProvider
	selectedProvider    SelectedStorePolicyProvider
	selectedPolicy      *SelectedStorePolicy
	limits              Limits
	diagnostics         diagnosticOptions
	nodes               []nodeRecord
	links               []linkRecord
	subtrees            []subtreeRecord
	children            []SubtreeID
	fields              []FieldMapEntry
	aliases             []Symbol
	frontier            uint64
	checkpoint          CheckpointID
	checkpoints         checkpointInterner
	boundaries          boundaryIndex
	boundaryJournal     []boundaryMutation
	transactions        []uint64
	nextTransaction     uint64
	classificationPhase uint64
	work                Work
	popScratch          popEnumerationScratch
	reductionScratch    reductionOutputScratch
	selectedBuild       selectedStoreBuildScratch
	selectedPoolMu      sync.Mutex
	selectedPool        selectedStoreBacking
	schedulerFrame      schedulerTransactionFrame
	// metadataConstructionAuthenticated remains true only while every compact
	// subtree was published through the authenticated shift/reduction seams.
	// Diagnostic generic publication clears it monotonically until Reset.
	metadataConstructionAuthenticated bool
}

// inlineAdjacencyCapacity covers the production default without forcing a
// fixed fan-out contract on callers that deliberately configure a wider
// boundary. nodeLinksInto spills to a bounded slice only above this width.
const inlineAdjacencyCapacity = 8

type diagnosticOptions struct {
	foldSamePredecessorShallowPayloads bool
}

type checkpoint struct {
	nodes, links, subtrees, children, fields, aliases int
	frontier                                          uint64
	checkpoint                                        CheckpointID
	boundaryIndex                                     boundaryIndexSnapshot
	journal                                           int
	transaction                                       uint64
	work                                              Work
}

// SchedulerTransactionToken is an opaque capability for one active
// scheduler-owned transaction. Callers may pass it only to the authenticated
// Owned methods below; core identity, epoch, transaction identity, and
// top-of-stack ownership are validated on every use.
type SchedulerTransactionToken struct {
	owner       *Core
	epoch       uint64
	transaction uint64
}

type schedulerTransactionFrame struct {
	mark     checkpoint
	epoch    uint64
	poisoned error
	active   bool
	fresh    bool
}

// clearInactive releases every completed checkpoint reference, including the
// boundary-index snapshot backing array, while preserving the monotonic epoch
// that prevents Reset from making an escaped token current again.
func (f *schedulerTransactionFrame) clearInactive() {
	epoch := f.epoch
	*f = schedulerTransactionFrame{epoch: epoch}
}

func (c *Core) markInto(mark *checkpoint) {
	if mark == nil {
		panic("parser-core phase zero: nil transaction checkpoint")
	}
	if c.nextTransaction == math.MaxUint64 {
		panic("parser-core phase zero: transaction identity overflow")
	}
	c.nextTransaction++
	*mark = checkpoint{
		nodes: len(c.nodes), links: len(c.links), subtrees: len(c.subtrees),
		children: len(c.children), fields: len(c.fields), aliases: len(c.aliases),
		frontier: c.frontier, checkpoint: c.checkpoint,
		boundaryIndex: c.boundaries.snapshot(),
		journal:       len(c.boundaryJournal), transaction: c.nextTransaction,
		work: c.work,
	}
	c.transactions = append(c.transactions, mark.transaction)
	parent := uint64(0)
	if len(c.transactions) > 1 {
		parent = c.transactions[len(c.transactions)-2]
	}
	phase0AObserveMark(c, mark.transaction, parent)
}

func (c *Core) mark() checkpoint {
	var mark checkpoint
	c.markInto(&mark)
	return mark
}

func (c *Core) restoreCheckpoint(mark *checkpoint) {
	c.assertTopTransactionCheckpoint(mark)
	// A classification may have escaped from any nested operation that
	// published arena records after this mark. Rollback makes those NodeIDs
	// reusable, so invalidate every outstanding capability before truncating
	// storage. Successful commits deliberately keep the phase stable.
	if c.classificationPhase == math.MaxUint64 {
		panic("parser-core phase zero: classification phase overflow during rollback")
	}
	c.classificationPhase++
	c.nodes = c.nodes[:mark.nodes]
	c.links = c.links[:mark.links]
	c.subtrees = c.subtrees[:mark.subtrees]
	c.children = c.children[:mark.children]
	c.fields = c.fields[:mark.fields]
	c.aliases = c.aliases[:mark.aliases]
	c.frontier = mark.frontier
	c.checkpoint = mark.checkpoint
	c.work = mark.work
	for index := len(c.boundaryJournal) - 1; index >= mark.journal; index-- {
		mutation := c.boundaryJournal[index]
		mutation.slots[mutation.index] = mutation.previous
	}
	c.boundaries.restore(mark.boundaryIndex)
	clear(c.boundaryJournal[mark.journal:])
	c.boundaryJournal = c.boundaryJournal[:mark.journal]
	phase0AObserveRollback(c, mark.transaction, phase0ATakeRollbackCause(c))
	c.finishTransaction()
}

func (c *Core) restore(mark checkpoint) {
	c.restoreCheckpoint(&mark)
}

func (c *Core) commit(mark checkpoint) {
	c.assertTopTransactionCheckpoint(&mark)
	phase0AObserveCommit(c, mark.transaction)
	c.finishTransaction()
}

func (c *Core) assertTopTransaction(mark checkpoint) {
	c.assertTopTransactionCheckpoint(&mark)
}

func (c *Core) assertTopTransactionCheckpoint(mark *checkpoint) {
	if mark == nil || len(c.transactions) == 0 || c.transactions[len(c.transactions)-1] != mark.transaction {
		panic("parser-core phase zero: transaction checkpoint used out of LIFO order")
	}
}

func (c *Core) finishTransaction() {
	c.transactions = c.transactions[:len(c.transactions)-1]
	if len(c.transactions) == 0 {
		clear(c.boundaryJournal)
		c.boundaryJournal = c.boundaryJournal[:0]
	}
}

func (c *Core) completeTransaction(mark checkpoint, err *error) {
	if recovered := recover(); recovered != nil {
		phase0ASetRollbackCause(c, Phase0ARollbackPanic)
		c.restore(mark)
		panic(recovered)
	}
	if *err != nil {
		phase0ASetRollbackCause(c, Phase0ARollbackReturnedError)
		c.restore(mark)
	} else {
		c.commit(mark)
	}
}

func (c *Core) validateSchedulerTransaction(token SchedulerTransactionToken) error {
	frame := &c.schedulerFrame
	if token.owner != c {
		return errors.New("parser-core phase zero: scheduler transaction token belongs to a different core")
	}
	if !frame.active || token.epoch == 0 || token.epoch != frame.epoch || token.transaction != frame.mark.transaction {
		return errors.New("parser-core phase zero: stale scheduler transaction token")
	}
	if frame.fresh {
		return nil
	}
	if len(c.transactions) == 0 || c.transactions[len(c.transactions)-1] != token.transaction {
		return errors.New("parser-core phase zero: scheduler transaction token is not top-of-stack owner")
	}
	return nil
}

// RunFreshSchedulerSession authenticates one scheduler-owned run without
// taking per-operation arena checkpoints. It is restricted to a core with no
// active transaction. Successful runs keep their compact graph; any error or
// panic resets the whole fresh core after invalidating the capability. The
// session still poisons ignored owned-operation failures.
func (c *Core) RunFreshSchedulerSession(fn func(SchedulerTransactionToken) error) (err error) {
	frame := &c.schedulerFrame
	if fn == nil {
		err := errors.New("parser-core phase zero: nil fresh scheduler session")
		if frame.active && frame.poisoned == nil {
			frame.poisoned = err
		}
		return err
	}
	if frame.active {
		err := errors.New("parser-core phase zero: nested fresh scheduler session")
		if frame.poisoned == nil {
			frame.poisoned = err
		}
		return err
	}
	if len(c.transactions) != 0 {
		return errors.New("parser-core phase zero: fresh scheduler session inside active transaction")
	}
	if frame.epoch == math.MaxUint64 || c.nextTransaction == math.MaxUint64 {
		return errors.New("parser-core phase zero: fresh scheduler session identity overflow")
	}
	frame.clearInactive()
	frame.epoch++
	c.nextTransaction++
	frame.mark.transaction = c.nextTransaction
	frame.active = true
	frame.fresh = true
	token := SchedulerTransactionToken{owner: c, epoch: frame.epoch, transaction: frame.mark.transaction}
	defer func() {
		recovered := recover()
		if recovered != nil {
			frame.clearInactive()
			_ = c.Reset()
			panic(recovered)
		}
		if err == nil && frame.poisoned != nil {
			err = fmt.Errorf("parser-core phase zero: poisoned fresh scheduler session: %w", frame.poisoned)
		}
		frame.clearInactive()
		if err != nil {
			if resetErr := c.Reset(); resetErr != nil {
				err = errors.Join(err, fmt.Errorf("parser-core phase zero: reset failed after fresh scheduler error: %w", resetErr))
			}
		}
	}()
	err = fn(token)
	return err
}

func (c *Core) poisonSchedulerTransaction(token SchedulerTransactionToken, cause error) error {
	if cause == nil {
		return nil
	}
	if err := c.validateSchedulerTransaction(token); err != nil {
		if c.schedulerFrame.active && c.schedulerFrame.poisoned == nil {
			c.schedulerFrame.poisoned = err
		}
		return err
	}
	if c.schedulerFrame.poisoned == nil {
		c.schedulerFrame.poisoned = cause
	}
	return cause
}

// RunSchedulerOwned validates token ownership, executes one uncheckpointed
// inner scheduler mutation, and poisons the outer owner on every returned
// error. Ignoring the returned error therefore cannot commit partial state.
func (c *Core) RunSchedulerOwned(token SchedulerTransactionToken, fn func() error) (err error) {
	if fn == nil {
		err := c.poisonSchedulerTransaction(token, errors.New("parser-core phase zero: nil scheduler-owned operation"))
		phase0AObserveSchedulerPoison(c, token, Phase0APoisonReturnedError)
		return err
	}
	if err := c.validateSchedulerTransaction(token); err != nil {
		err = c.poisonSchedulerTransaction(token, err)
		phase0AObserveSchedulerPoison(c, token, Phase0APoisonReturnedError)
		return err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			c.poisonSchedulerTransaction(token, fmt.Errorf("parser-core phase zero: scheduler-owned operation panicked: %v", recovered))
			phase0AObserveSchedulerPoison(c, token, Phase0APoisonPanic)
			panic(recovered)
		}
	}()
	if err = fn(); err != nil {
		err = c.poisonSchedulerTransaction(token, err)
		phase0AObserveSchedulerPoison(c, token, Phase0APoisonReturnedError)
		return err
	}
	return nil
}

// ApplySchedulerAtomic owns one retained checkpoint for an authenticated
// scheduler operation. Standalone public wrappers continue to own their
// ordinary checkpoints; only methods explicitly passed the returned opaque
// token may execute without another frame.
func (c *Core) ApplySchedulerAtomic(fn func(SchedulerTransactionToken) error) (err error) {
	if fn == nil {
		err := errors.New("parser-core phase zero: nil scheduler atomic operation")
		if c.schedulerFrame.active && c.schedulerFrame.poisoned == nil {
			c.schedulerFrame.poisoned = err
			phase0AObserveFirstPoison(c, c.schedulerFrame.mark.transaction, Phase0APoisonReturnedError)
		}
		return err
	}
	frame := &c.schedulerFrame
	if frame.active && frame.fresh {
		token := SchedulerTransactionToken{owner: c, epoch: frame.epoch, transaction: frame.mark.transaction}
		if err = fn(token); err != nil {
			return c.poisonSchedulerTransaction(token, err)
		}
		return nil
	}
	if frame.active {
		err := errors.New("parser-core phase zero: nested scheduler-owned transaction")
		if frame.poisoned == nil {
			frame.poisoned = err
			phase0AObserveFirstPoison(c, frame.mark.transaction, Phase0APoisonNestedScheduler)
		}
		return err
	}
	if frame.epoch == math.MaxUint64 {
		return errors.New("parser-core phase zero: scheduler transaction epoch overflow")
	}
	frame.clearInactive()
	frame.epoch++
	c.markInto(&frame.mark)
	frame.active = true
	token := SchedulerTransactionToken{owner: c, epoch: frame.epoch, transaction: frame.mark.transaction}
	defer func() {
		recovered := recover()
		if recovered != nil {
			phase0ASetRollbackCause(c, Phase0ARollbackPanic)
			c.restoreCheckpoint(&frame.mark)
			frame.clearInactive()
			panic(recovered)
		}
		if err == nil && frame.poisoned != nil {
			err = fmt.Errorf("parser-core phase zero: poisoned scheduler transaction: %w", frame.poisoned)
		}
		if err != nil {
			cause := Phase0ARollbackReturnedError
			if frame.poisoned != nil {
				cause = Phase0ARollbackSchedulerPoison
			}
			phase0ASetRollbackCause(c, cause)
			c.restoreCheckpoint(&frame.mark)
		} else {
			c.commit(frame.mark)
		}
		frame.clearInactive()
	}()
	err = fn(token)
	return err
}

func (c *Core) writeBoundary(key boundaryKey, id NodeID) error {
	if key.frontier != c.frontier {
		return errors.New("parser-core phase zero: boundary frontier mismatch")
	}
	probe, _ := c.boundaries.probe(boundaryIdentityFromKey(key))
	return c.publishBoundary(probe, id)
}

func (c *Core) publishBoundary(probe boundaryProbe, id NodeID) error {
	return c.boundaries.publish(probe, id, &c.boundaryJournal, len(c.transactions) != 0)
}

func New(tables TableView, limits Limits) (*Core, error) {
	if tables == nil {
		return nil, errors.New("parser-core phase zero: nil table view")
	}
	limits = limits.withDefaults()
	boundaries, err := newBoundaryIndex(limits.MaxNodes)
	if err != nil {
		return nil, err
	}
	core := &Core{
		tables: tables, limits: limits, frontier: 1, boundaries: boundaries,
		checkpoints:                       newCheckpointInterner(limits.MaxCheckpoints, limits.MaxCheckpointBytes),
		classificationPhase:               1,
		diagnostics:                       diagnosticOptions{foldSamePredecessorShallowPayloads: true},
		metadataConstructionAuthenticated: true,
	}
	core.plans, _ = tables.(ReductionPlanProvider)
	core.selectedProvider, _ = tables.(SelectedStorePolicyProvider)
	return core, nil
}

// Reset returns the compact core to the same empty frontier created by New
// while retaining its authenticated tables, limits, diagnostic policy, and
// arena capacities. Reset is deliberately unavailable during an active
// transaction because clearing the rollback journal would weaken the atomic
// publication contract. Reset invalidates every Head, SubtreeID, and other
// arena handle previously returned by the core; callers must discard them
// because the retained arenas deliberately reuse their one-based IDs.
func (c *Core) Reset() error {
	if c == nil {
		return errors.New("parser-core phase zero: reset nil core")
	}
	if len(c.transactions) != 0 || c.schedulerFrame.active {
		return errors.New("parser-core phase zero: reset during active transaction")
	}
	if c.classificationPhase == math.MaxUint64 {
		return errors.New("parser-core phase zero: classification phase overflow")
	}
	if err := phase0AInvalidateCore(c); err != nil {
		return err
	}
	c.classificationPhase++
	c.nodes = c.nodes[:0]
	c.links = c.links[:0]
	c.subtrees = c.subtrees[:0]
	c.children = c.children[:0]
	c.fields = c.fields[:0]
	c.aliases = c.aliases[:0]
	c.frontier = 1
	c.checkpoint = 0
	c.checkpoints.reset()
	c.boundaries.reset()
	clear(c.boundaryJournal)
	c.boundaryJournal = c.boundaryJournal[:0]
	c.transactions = c.transactions[:0]
	c.nextTransaction = 0
	c.schedulerFrame.clearInactive()
	c.work = Work{}
	c.popScratch.resetLogical()
	c.reductionScratch.finish()
	c.metadataConstructionAuthenticated = true
	return nil
}

// BeginFrontier starts one authenticated election/dispatch generation.
// Boundary condensation never crosses generations, even when state and byte
// position repeat. Existing heads remain valid persistent predecessors.
func (c *Core) BeginFrontier() error {
	if len(c.transactions) != 0 {
		return errors.New("parser-core phase zero: begin frontier during active transaction")
	}
	if c.frontier == math.MaxUint64 {
		return errors.New("parser-core phase zero: frontier epoch overflow")
	}
	if c.classificationPhase == math.MaxUint64 {
		return errors.New("parser-core phase zero: classification phase overflow")
	}
	c.boundaries.advanceGeneration()
	c.frontier++
	c.classificationPhase++
	c.checkpoint = 0
	return nil
}

// SetPhaseCheckpoint binds subsequent condensation to the exact scanner
// checkpoint for the current lookahead epoch. A changed checkpoint advances
// classified-boundary authentication without advancing the frontier epoch.
func (c *Core) SetPhaseCheckpoint(checkpoint CheckpointID) error {
	if len(c.transactions) != 0 {
		return errors.New("parser-core phase zero: set checkpoint during active transaction")
	}
	if checkpoint != 0 {
		if _, ok := c.checkpoints.record(checkpoint); !ok {
			return errors.New("parser-core phase zero: unknown checkpoint identity")
		}
	}
	if checkpoint == c.checkpoint {
		return nil
	}
	if c.classificationPhase == math.MaxUint64 {
		return errors.New("parser-core phase zero: classification phase overflow")
	}
	c.classificationPhase++
	c.checkpoint = checkpoint
	return nil
}

func (c *Core) boundaryKey(state StateID, byteOffset uint32) boundaryKey {
	return boundaryKey{frontier: c.frontier, state: state, byteOffset: byteOffset, checkpoint: c.checkpoint}
}

func (c *Core) shiftedBoundaryKey(state StateID, byteOffset uint32) boundaryKey {
	return boundaryKey{frontier: c.frontier, state: state, byteOffset: byteOffset, shifted: true, checkpoint: c.checkpoint}
}

// BoundaryIndexStats returns the live and retained canonical lookup census.
func (c *Core) BoundaryIndexStats() BoundaryIndexStats {
	if c == nil {
		return BoundaryIndexStats{}
	}
	count := uint64(c.boundaries.count)
	return BoundaryIndexStats{Frontier: c.frontier, CurrentEntries: count, RetainedEntries: count}
}

// Seed creates one empty derivation at a parser boundary.
func (c *Core) Seed(state StateID, byteOffset uint32) (Head, error) {
	key := c.boundaryKey(state, byteOffset)
	probe, id := c.boundaries.probe(boundaryIdentityFromKey(key))
	if probe.found {
		return Head{Node: id}, nil
	}
	id, err := c.appendNode(nodeRecord{state: state, byteOffset: byteOffset, pathCount: 1})
	if err != nil {
		return Head{}, err
	}
	if err := c.publishBoundary(probe, id); err != nil {
		c.nodes = c.nodes[:len(c.nodes)-1]
		return Head{}, err
	}
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
func (c *Core) CanonicalBoundary(state StateID, byteOffset uint32, consumed bool, checkpoint CheckpointID) (Head, bool) {
	probe, id := c.boundaries.probe(boundaryIdentityFromKey(boundaryKey{
		frontier: c.frontier, state: state, byteOffset: byteOffset,
		shifted: consumed, checkpoint: checkpoint,
	}))
	return Head{Node: id}, probe.found
}

// InternCheckpoint returns the core-local identity of an exact serialized
// scanner state. Digest bucketing is only an index; equality confirms all
// bytes. Interner mutation is rejected during compact transactions; existing
// immutable identities survive rollback, while Reset clears logical contents.
func (c *Core) InternCheckpoint(serialized []byte) (CheckpointID, error) {
	if c == nil {
		return 0, errors.New("parser-core phase zero: intern checkpoint on nil core")
	}
	if len(c.transactions) != 0 {
		return 0, errors.New("parser-core phase zero: intern checkpoint during active transaction")
	}
	return c.checkpoints.intern(serialized)
}

// CheckpointReceipt resolves receipt metadata without exposing retained
// serialized storage.
func (c *Core) CheckpointReceipt(id CheckpointID) (uint32, [32]byte, bool) {
	if c == nil {
		return 0, [32]byte{}, false
	}
	return c.checkpoints.receipt(id)
}

// CheckpointInternerStats reports bounded logical scanner-state retention.
func (c *Core) CheckpointInternerStats() CheckpointInternerStats {
	if c == nil {
		return CheckpointInternerStats{}
	}
	return c.checkpoints.stats()
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
func (c *Core) Actions(state StateID, lookahead Symbol) (ActionRow, error) {
	return c.tables.Actions(state, lookahead)
}

// ClassifyBoundary resolves one compact head and its immutable action row once
// for the current scheduler phase. Classified execution methods validate this
// capability before consuming it, avoiding a second table lookup per action.
func (c *Core) ClassifyBoundary(head Head, lookahead Symbol) (ClassifiedBoundary, error) {
	node, err := c.node(head.Node)
	if err != nil {
		return ClassifiedBoundary{}, err
	}
	actions, err := c.tables.Actions(node.state, lookahead)
	if err != nil {
		return ClassifiedBoundary{}, err
	}
	return ClassifiedBoundary{
		owner: c, actions: actions, phase: c.classificationPhase,
		head: head, state: node.state, byteOffset: node.byteOffset, lookahead: lookahead,
	}, nil
}

func (c *Core) validateClassification(boundary ClassifiedBoundary) error {
	if boundary.owner != c {
		return errors.New("parser-core phase zero: classified boundary belongs to another core")
	}
	if boundary.phase == 0 || boundary.phase != c.classificationPhase {
		return errors.New("parser-core phase zero: classified boundary is stale")
	}
	return nil
}

func (c *Core) classifiedActionRef(boundary ClassifiedBoundary, ordinal int) (*Action, error) {
	if err := c.validateClassification(boundary); err != nil {
		return nil, err
	}
	if ordinal < 0 || ordinal >= boundary.actions.Len() {
		return nil, fmt.Errorf("parser-core phase zero: action ordinal %d out of range", ordinal)
	}
	return boundary.actions.actionRef(ordinal), nil
}

// Shift applies one authentic decoded shift action and condenses the resulting
// exact path at its (state, byte) boundary.
func (c *Core) Shift(head Head, lookahead Symbol, actionOrdinal int, token Token, fork ForkOrder) (Head, error) {
	boundary, err := c.ClassifyBoundary(head, lookahead)
	if err != nil {
		return Head{}, err
	}
	return c.ShiftClassified(boundary, actionOrdinal, token, fork)
}

// ShiftClassified applies one shift using a current owner-authenticated
// classification, avoiding a duplicate action-table lookup.
func (c *Core) ShiftClassified(boundary ClassifiedBoundary, actionOrdinal int, token Token, fork ForkOrder) (out Head, err error) {
	mark := c.mark()
	defer c.completeTransaction(mark, &err)
	return c.shiftClassifiedUncheckpointed(boundary, actionOrdinal, token, fork)
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
	boundaries := make([]ClassifiedBoundary, len(inputs))
	for index, input := range inputs {
		if input.ActionOrdinal != 0 {
			return nil, fmt.Errorf("parser-core phase zero: head %d does not select one ordinary shift", input.Head.Node)
		}
		boundary, err := c.ClassifyBoundary(input.Head, lookahead)
		if err != nil {
			return nil, err
		}
		boundaries[index] = boundary
	}
	return c.ShiftOrdinaryClassifiedCohort(boundaries, token)
}

// ShiftOrdinaryClassifiedCohort is the classified scheduler form of
// ShiftOrdinaryCohort. Every boundary must select one ordinary shift.
func (c *Core) ShiftOrdinaryClassifiedCohort(boundaries []ClassifiedBoundary, token Token) (out []Head, err error) {
	var inlineTargets [inlineSchedulerCohortTargets]StateID
	targets := inlineTargets[:]
	if len(boundaries) > len(targets) {
		targets = make([]StateID, len(boundaries))
	} else {
		targets = targets[:len(boundaries)]
	}
	if err := c.prepareOrdinaryClassifiedCohortInto(boundaries, token, targets); err != nil {
		return nil, err
	}
	err = c.ApplyAtomic(func() error {
		var innerErr error
		out, innerErr = c.shiftOrdinaryClassifiedCohortUncheckpointed(boundaries, targets, token)
		return innerErr
	})
	return out, err
}

// ShiftExtraCohort applies one positive-width extra terminal election to
// distinct scheduler heads while allocating exactly one immutable terminal
// payload. Every selected cell must contain one undecorated extra shift. A
// target state of zero retains that head's current state, matching production
// extraShiftTargetState semantics.
func (c *Core) ShiftExtraCohort(inputs []ExtraCohortShiftInput, lookahead Symbol, token Token) (out []Head, err error) {
	if len(inputs) == 0 {
		return nil, errors.New("parser-core phase zero: empty extra shift cohort")
	}
	boundaries := make([]ClassifiedBoundary, len(inputs))
	for index, input := range inputs {
		if input.ActionOrdinal != 0 {
			return nil, fmt.Errorf("parser-core phase zero: head %d does not select one extra shift", input.Head.Node)
		}
		boundary, err := c.ClassifyBoundary(input.Head, lookahead)
		if err != nil {
			return nil, err
		}
		boundaries[index] = boundary
	}
	return c.ShiftExtraClassifiedCohort(boundaries, token)
}

// ShiftExtraClassifiedCohort is the classified scheduler form of
// ShiftExtraCohort. Every boundary must select one extra shift.
func (c *Core) ShiftExtraClassifiedCohort(boundaries []ClassifiedBoundary, token Token) (out []Head, err error) {
	var inlineTargets [inlineSchedulerCohortTargets]StateID
	targets := inlineTargets[:]
	if len(boundaries) > len(targets) {
		targets = make([]StateID, len(boundaries))
	} else {
		targets = targets[:len(boundaries)]
	}
	if err := c.prepareExtraClassifiedCohortInto(boundaries, token, targets); err != nil {
		return nil, err
	}
	err = c.ApplyAtomic(func() error {
		var innerErr error
		out, innerErr = c.shiftExtraClassifiedCohortUncheckpointed(boundaries, targets, token)
		return innerErr
	})
	return out, err
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
	payload, err := c.appendAuthenticatedTerminal(subtreeRecord{
		symbol: token.Symbol, startByte: token.StartByte, endByte: token.EndByte,
		extra: token.Extra, external: token.External, terminal: true,
	})
	if err != nil {
		return Head{}, err
	}
	return c.condense(c.boundaryKey(state, token.EndByte), linkInput{
		prev: head.Node, payload: payload, scoreDelta: meta.ScoreDelta, order: meta.BranchOrder,
	})
}

// ReductionFreshness describes how one final canonical boundary changed over
// a complete ReduceOutputs call.
type ReductionFreshness uint8

const (
	ReductionUnchanged ReductionFreshness = iota + 1
	ReductionNew
	ReductionUpdated
)

// ReductionOutput is one final canonical boundary and its aggregate freshness
// relative to the boundary map at entry to ReduceOutputs.
type ReductionOutput struct {
	Head      Head
	Freshness ReductionFreshness
}

const inlineReductionBoundaryOutputs = 2

type reductionBoundaryOutput struct {
	key       boundaryKey
	head      Head
	freshness ReductionFreshness
}

// reductionOutputScratch owns the ephemeral aggregation state for one
// reduction. Canonical query_compile reductions produce one output 95% of the
// time and never more than two, so the ordered inline slice handles that path
// without hashing. Unusual wider reductions spill to an indexed map while the
// slice remains the authoritative stable output order.
type reductionOutputScratch struct {
	boundaries          []reductionBoundaryOutput
	boundaryByKey       map[boundaryKey]int
	batchParents        []SubtreeID
	structuralPositions []uint16
	remappedFields      []FieldMapEntry
	remappedAliases     []Symbol
	spilled             bool
}

func (s *reductionOutputScratch) begin() {
	s.boundaries = s.boundaries[:0]
	s.batchParents = s.batchParents[:0]
	s.structuralPositions = s.structuralPositions[:0]
	s.remappedFields = s.remappedFields[:0]
	s.remappedAliases = s.remappedAliases[:0]
	s.spilled = false
}

func (s *reductionOutputScratch) finish() {
	s.boundaries = s.boundaries[:0]
	s.batchParents = s.batchParents[:0]
	s.structuralPositions = s.structuralPositions[:0]
	s.remappedFields = s.remappedFields[:0]
	s.remappedAliases = s.remappedAliases[:0]
	if len(s.boundaryByKey) != 0 {
		clear(s.boundaryByKey)
	}
	s.spilled = false
}

func (s *reductionOutputScratch) boundary(key boundaryKey) (int, bool) {
	if s.spilled {
		index, ok := s.boundaryByKey[key]
		if !ok {
			return len(s.boundaries), false
		}
		return index, ok
	}
	for index := range s.boundaries {
		if s.boundaries[index].key == key {
			return index, true
		}
	}
	if len(s.boundaries) < inlineReductionBoundaryOutputs {
		return len(s.boundaries), false
	}
	if s.boundaryByKey == nil {
		s.boundaryByKey = make(map[boundaryKey]int, inlineReductionBoundaryOutputs*2)
	} else {
		clear(s.boundaryByKey)
	}
	for index := range s.boundaries {
		s.boundaryByKey[s.boundaries[index].key] = index
	}
	s.spilled = true
	index, ok := s.boundaryByKey[key]
	if !ok {
		return len(s.boundaries), false
	}
	return index, ok
}

func (s *reductionOutputScratch) store(index int, seen bool, output reductionBoundaryOutput) {
	if seen {
		s.boundaries[index] = output
		return
	}
	s.boundaries = append(s.boundaries, output)
	if s.spilled {
		s.boundaryByKey[output.key] = index
	}
}

// Reduce preserves the compatibility surface used by earlier phase-zero
// diagnostics: it returns every canonical output, including unchanged ones.
// Worklist schedulers must use ReduceOutputs and inspect Freshness explicitly.
func (c *Core) Reduce(head Head, lookahead Symbol, actionOrdinal int, fork ForkOrder) ([]Head, error) {
	outputs, err := c.ReduceOutputs(head, lookahead, actionOrdinal, fork)
	if err != nil {
		return nil, err
	}
	frontier := make([]Head, len(outputs))
	for index, output := range outputs {
		frontier[index] = output.Head
	}
	return frontier, nil
}

// ReduceOutputs applies one authentic decoded reduction to every retained pop
// path and reports aggregate boundary freshness. Condensation selects only
// C-shallow clean links from the same exact predecessor; other derivations
// remain distinct.
func (c *Core) ReduceOutputs(head Head, lookahead Symbol, actionOrdinal int, fork ForkOrder) (frontier []ReductionOutput, err error) {
	// A nil destination gives the compatibility caller its own stable result.
	// Hot schedulers use ReduceOutputsInto with retained destination storage.
	return c.ReduceOutputsInto(nil, head, lookahead, actionOrdinal, fork)
}

// ReduceOutputsInto is ReduceOutputs with caller-owned destination storage.
// It resets dst's logical length and appends outputs in the same stable first-
// boundary order as ReduceOutputs. Core-owned aggregation scratch is ephemeral
// and is always cleared on success, error, or panic; dst remains caller-owned.
func (c *Core) ReduceOutputsInto(dst []ReductionOutput, head Head, lookahead Symbol, actionOrdinal int, fork ForkOrder) (frontier []ReductionOutput, err error) {
	boundary, err := c.ClassifyBoundary(head, lookahead)
	if err != nil {
		return nil, err
	}
	return c.ReduceOutputsClassifiedInto(dst, boundary, actionOrdinal, fork)
}

// ReduceOutputsClassifiedInto applies one reduction using a current
// owner-authenticated classification and caller-owned destination storage.
func (c *Core) ReduceOutputsClassifiedInto(dst []ReductionOutput, boundary ClassifiedBoundary, actionOrdinal int, fork ForkOrder) (frontier []ReductionOutput, err error) {
	mark := c.mark()
	defer c.completeTransaction(mark, &err)
	return c.reduceOutputsClassifiedIntoUncheckpointed(dst, boundary, actionOrdinal, fork)
}

func (c *Core) reductionParentForPath(
	act Action,
	plan *ReductionPlan,
	path popPath,
	key boundaryKey,
	fork ForkOrder,
	scratch *reductionOutputScratch,
) (SubtreeID, int64, ForkOrder, error) {
	fields, aliases, err := c.remapReductionPlan(path.children, plan, scratch)
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
		payload, found, err = c.findReductionBatchParent(scratch.batchParents, parent, path.children, fields, aliases)
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
		// The remap above authenticated the exact production, child sequence, and
		// ordered sidecars published here. Keep raw publication in this lexical
		// seam so no reusable authority reaches another caller.
		payload, err = c.appendSubtreeRecord(parent, path.children, fields, aliases)
		if err != nil {
			return 0, 0, ForkOrder{}, err
		}
		if len(path.trailing) == 0 {
			scratch.batchParents = append(scratch.batchParents, payload)
		}
	}
	return payload, scoreDelta, order, nil
}

func (c *Core) reductionPlan(act Action) (ReductionPlan, error) {
	return c.reductionPlanForPair(act.ProductionID, int(act.ChildCount))
}

func (c *Core) reductionPlanForPair(productionID uint16, childCount int) (ReductionPlan, error) {
	if c.plans != nil {
		plan, err := c.plans.ReductionPlan(productionID, childCount)
		if err != nil {
			return ReductionPlan{}, err
		}
		if plan.productionID != productionID || int(plan.childCount) != childCount {
			return ReductionPlan{}, errors.New("parser-core phase zero: reduction plan pair identity mismatch")
		}
		return plan, nil
	}
	fields, err := c.tables.ProductionFields(productionID, childCount)
	if err != nil {
		return ReductionPlan{}, err
	}
	aliases, err := c.tables.ProductionAliases(productionID, childCount)
	if err != nil {
		return ReductionPlan{}, err
	}
	return NewReductionPlan(productionID, childCount, fields, aliases)
}

func (c *Core) remapReductionPlan(children []SubtreeID, plan *ReductionPlan, scratch *reductionOutputScratch) ([]FieldMapEntry, []Symbol, error) {
	if plan == nil {
		return nil, nil, errors.New("parser-core phase zero: nil reduction plan")
	}
	structuralCount := int(plan.childCount)
	positions := scratch.structuralPositions[:0]
	for index, child := range children {
		payload, err := c.subtree(child)
		if err != nil {
			return nil, nil, err
		}
		if !payload.extra {
			if index > math.MaxUint16 {
				return nil, nil, errors.New("parser-core phase zero: structural child position exceeds uint16")
			}
			positions = append(positions, uint16(index))
		}
	}
	scratch.structuralPositions = positions
	if len(positions) != structuralCount {
		return nil, nil, fmt.Errorf("parser-core phase zero: reduction stored %d structural children, want %d", len(positions), structuralCount)
	}
	if cap(scratch.remappedFields) < len(plan.fields) {
		scratch.remappedFields = make([]FieldMapEntry, len(plan.fields))
	}
	fields := scratch.remappedFields[:len(plan.fields)]
	for index, field := range plan.fields {
		actual := positions[field.ChildIndex]
		if actual > math.MaxUint8 {
			return nil, nil, errors.New("parser-core phase zero: remapped field child index exceeds uint8")
		}
		field.ChildIndex = uint8(actual)
		fields[index] = field
	}
	scratch.remappedFields = fields
	if len(plan.aliases) == 0 {
		scratch.remappedAliases = scratch.remappedAliases[:0]
		return fields, nil, nil
	}
	if cap(scratch.remappedAliases) < len(children) {
		scratch.remappedAliases = make([]Symbol, len(children))
	}
	aliases := scratch.remappedAliases[:len(children)]
	clear(aliases)
	for index, alias := range plan.aliases {
		aliases[positions[index]] = alias
	}
	scratch.remappedAliases = aliases
	return fields, aliases, nil
}

func (c *Core) condenseInputExists(key boundaryKey, in linkInput) (bool, error) {
	if key.frontier != c.frontier {
		return false, errors.New("parser-core phase zero: boundary frontier mismatch")
	}
	probe, id := c.boundaries.probe(boundaryIdentityFromKey(key))
	if !probe.found {
		return false, nil
	}
	node, err := c.node(id)
	if err != nil {
		return false, err
	}
	var inline [inlineAdjacencyCapacity]linkRecord
	links, err := c.publishedNodeLinksInto(inline[:0], *node)
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
	if uint64(len(c.links))+1 > uint64(c.limits.MaxLinks) || uint64(len(c.links)) >= math.MaxUint32 {
		return Head{}, errors.New("parser-core phase zero: link arena cap")
	}
	if uint64(len(c.nodes))+1 > uint64(c.limits.MaxNodes) || uint64(len(c.nodes)) >= math.MaxUint32 {
		return Head{}, errors.New("parser-core phase zero: node arena cap")
	}
	flags := uint32(0)
	if in.order.Present {
		flags |= linkFlagHasOrder
	}
	linkID := LinkID(uint64(len(c.links)) + 1)
	c.links = append(c.links, linkRecord{
		prev: in.prev, payload: in.payload, scoreDelta: in.scoreDelta,
		order: in.order.Value, flags: flags,
	})
	c.addWork(&c.work.GraphLinkAdditionsProxy, 1)
	id, err := c.appendNode(nodeRecord{
		state: state, byteOffset: byteOffset,
		firstLink: uint32(linkID), linkCount: 1, pathCount: prev.pathCount,
	})
	if err != nil {
		return Head{}, err
	}
	if phase0AEnabled {
		phase0AObservePrivatePublication(c, state, byteOffset, in, linkID, id)
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
	if ordinal < 0 || ordinal >= actions.Len() {
		return Action{}, fmt.Errorf("parser-core phase zero: action ordinal %d out of range", ordinal)
	}
	return actions.At(ordinal), nil
}

type linkInput struct {
	prev       NodeID
	payload    SubtreeID
	scoreDelta int64
	order      ForkOrder
}

type condenseChange uint8

const (
	condenseUnchanged condenseChange = iota
	condenseNew
	condenseUpdated
)

type condenseOutcome struct {
	head   Head
	change condenseChange
}

func (c *Core) condense(key boundaryKey, in linkInput) (Head, error) {
	outcome, err := c.condenseWithOutcome(key, in)
	return outcome.head, err
}

// condenseWithOutcome distinguishes a newly published boundary, a material
// update to an existing canonical boundary, and an input already represented
// by that canonical boundary. The reduction worklist uses this monotone
// freshness signal; shifts only need the resulting head.
func (c *Core) condenseWithOutcome(key boundaryKey, in linkInput) (condenseOutcome, error) {
	var out condenseOutcome
	err := c.ApplyAtomic(func() error {
		var err error
		out, err = c.condenseWithOutcomeAtomic(key, in)
		return err
	})
	return out, err
}

func (c *Core) condenseWithOutcomeAtomic(key boundaryKey, in linkInput) (condenseOutcome, error) {
	if key.frontier != c.frontier {
		return condenseOutcome{}, errors.New("parser-core phase zero: boundary frontier mismatch")
	}
	prev, err := c.node(in.prev)
	if err != nil {
		return condenseOutcome{}, err
	}
	if _, err := c.subtree(in.payload); err != nil {
		return condenseOutcome{}, err
	}
	probe, oldID := c.boundaries.probe(boundaryIdentityFromKey(key))
	var old nodeRecord
	var oldLinks []linkRecord
	if probe.found {
		oldRecord, err := c.node(oldID)
		if err != nil {
			return condenseOutcome{}, err
		}
		old = *oldRecord
		var inline [inlineAdjacencyCapacity]linkRecord
		oldLinks, err = c.publishedNodeLinksInto(inline[:0], old)
		if err != nil {
			return condenseOutcome{}, err
		}
		c.recordLinkUnionAttempt()
		for index, link := range oldLinks {
			equal, err := c.linkEqualInput(link, in)
			if err != nil {
				return condenseOutcome{}, err
			}
			if equal {
				c.recordLinkUnionDuplicateNoop()
				if phase0AEnabled {
					phase0AObserveCandidateDrop(c, key, in, oldID, index, phase0ATransitionDuplicateDrop)
				}
				return condenseOutcome{head: Head{Node: oldID}, change: condenseUnchanged}, nil
			}
		}
		if c.diagnostics.foldSamePredecessorShallowPayloads {
			candidate := -1
			for index, link := range oldLinks {
				equal, err := c.shallowPayloadClassEqual(link, in)
				if err != nil {
					return condenseOutcome{}, err
				}
				if !equal {
					continue
				}
				if candidate >= 0 {
					return condenseOutcome{}, errors.New("parser-core phase zero: multiple shallow-fold incumbents")
				}
				candidate = index
			}
			if candidate >= 0 {
				incumbentPrecedence, err := c.effectivePayloadPrecedence(oldLinks[candidate].payload, oldLinks[candidate].scoreDelta)
				if err != nil {
					return condenseOutcome{}, err
				}
				incomingPrecedence, err := c.effectivePayloadPrecedence(in.payload, in.scoreDelta)
				if err != nil {
					return condenseOutcome{}, err
				}
				if incomingPrecedence <= incumbentPrecedence {
					c.recordLinkUnionDuplicateNoop()
					if phase0AEnabled {
						phase0AObserveCandidateDrop(c, key, in, oldID, candidate, phase0ATransitionPrecedenceDrop)
					}
					return condenseOutcome{head: Head{Node: oldID}, change: condenseUnchanged}, nil
				}
				if phase0AEnabled {
					phase0ABeginReplacement(c, key, in, oldID, candidate)
				}
				head, err := c.replaceBoundaryLink(key, probe, old, oldLinks, candidate, in)
				if err != nil {
					c.recordLinkUnionRejected()
				} else {
					c.recordLinkUnionPrecedenceReplaced()
				}
				return condenseOutcome{head: head, change: condenseUpdated}, err
			}
			outcome, handled, err := c.factorExactPredecessor(key, probe, oldID, oldLinks, in)
			if err != nil || handled {
				if handled {
					if err != nil {
						c.recordLinkUnionRejected()
					} else if outcome.change == condenseUpdated {
						c.recordLinkUnionRecursiveChanged()
					} else {
						c.recordLinkUnionDuplicateNoop()
					}
				}
				return outcome, err
			}
		}
	}
	newPathCount := prev.pathCount
	if oldID != 0 {
		newPathCount = saturatingAddPaths(newPathCount, old.pathCount)
	}
	if uint64(len(c.links))+1 > uint64(c.limits.MaxLinks) || uint64(len(c.links)) >= math.MaxUint32 {
		if oldID != 0 {
			c.recordLinkUnionRejected()
		}
		return condenseOutcome{}, errors.New("parser-core phase zero: link arena cap")
	}
	if uint64(len(c.nodes))+1 > uint64(c.limits.MaxNodes) || uint64(len(c.nodes)) >= math.MaxUint32 {
		if oldID != 0 {
			c.recordLinkUnionRejected()
		}
		return condenseOutcome{}, errors.New("parser-core phase zero: node arena cap")
	}
	linkCount := uint32(1)
	if oldID != 0 {
		if old.linkCount == math.MaxUint32 {
			return condenseOutcome{}, errors.New("parser-core phase zero: boundary link count overflow")
		}
		if old.linkCount >= c.limits.MaxLinksPerBoundary {
			c.recordLinkUnionRejected()
			return condenseOutcome{}, &LiveLinkCapacityError{
				State: key.state, ByteOffset: key.byteOffset,
				ObservedLinks: uint64(old.linkCount) + 1, Limit: c.limits.MaxLinksPerBoundary,
			}
		}
		linkCount += old.linkCount
	}
	linkID := LinkID(uint64(len(c.links)) + 1)
	flags := uint32(0)
	if in.order.Present {
		flags |= linkFlagHasOrder
	}
	c.links = append(c.links, linkRecord{
		prev: in.prev, payload: in.payload, scoreDelta: in.scoreDelta,
		order: in.order.Value, flags: flags, next: LinkID(old.firstLink),
	})
	c.addWork(&c.work.GraphLinkAdditionsProxy, 1)
	id, err := c.appendNode(nodeRecord{
		state: key.state, byteOffset: key.byteOffset,
		firstLink: uint32(linkID), linkCount: linkCount, pathCount: newPathCount,
	})
	if err != nil {
		return condenseOutcome{}, err
	}
	if err := c.publishBoundary(probe, id); err != nil {
		if oldID != 0 {
			c.recordLinkUnionRejected()
		}
		return condenseOutcome{}, err
	}
	if phase0AEnabled {
		phase0AObserveDirectPublication(c, key, in, linkID, id, oldID)
	}
	change := condenseNew
	if oldID != 0 {
		change = condenseUpdated
		c.recordLinkUnionAlternateAppended()
	}
	return condenseOutcome{head: Head{Node: id}, change: change}, nil
}

func (c *Core) linkEqualInput(link linkRecord, in linkInput) (bool, error) {
	return link.prev == in.prev && link.payload == in.payload && link.scoreDelta == in.scoreDelta &&
		link.hasOrder() == in.order.Present && (!link.hasOrder() || link.order == in.order.Value), nil
}

type shallowPayloadClass struct {
	symbol     Symbol
	padding    uint32
	size       uint32
	childCount uint32
	extra      bool
}

func (c *Core) shallowPayloadClassEqual(link linkRecord, in linkInput) (bool, error) {
	if link.prev != in.prev {
		return false, nil
	}
	left, leftOK, err := c.shallowPayloadClass(link.prev, link.payload)
	if err != nil || !leftOK {
		return false, err
	}
	right, rightOK, err := c.shallowPayloadClass(in.prev, in.payload)
	if err != nil || !rightOK {
		return false, err
	}
	return left == right, nil
}

func (c *Core) effectivePayloadPrecedence(payloadID SubtreeID, aggregate int64) (int64, error) {
	payload, err := c.subtree(payloadID)
	if err != nil {
		return 0, err
	}
	if payload.childCount == 0 {
		return 0, nil
	}
	return aggregate, nil
}

// factorExactPredecessor applies the narrow persistent one-layer form of C's
// recursive stack-link insertion that phase zero can represent exactly. The
// outer edge must match in every stored field except predecessor identity. A merely
// shallow-equivalent outer edge is deliberately declined: tree-sitter also
// updates a node-level maximum precedence in that case, and nodeRecord has no
// authenticated equivalent yet.
func (c *Core) factorExactPredecessor(key boundaryKey, probe boundaryProbe, oldID NodeID, oldLinks []linkRecord, in linkInput) (out condenseOutcome, handled bool, err error) {
	for index, incumbent := range oldLinks {
		if incumbent.prev == in.prev {
			continue
		}
		mergeable, matchErr := c.predecessorBoundariesMatch(incumbent.prev, in.prev)
		if matchErr != nil {
			return condenseOutcome{}, true, matchErr
		}
		if !mergeable {
			continue
		}
		exactEdge := c.linkEdgeEqualInput(incumbent, in)
		shallow, classErr := c.shallowPayloadsEqual(incumbent.prev, incumbent.payload, in.prev, in.payload)
		if classErr != nil {
			return condenseOutcome{}, true, classErr
		}
		if !exactEdge && !shallow {
			continue
		}
		leftClean, cleanErr := c.subtreeHasNoExternalDescendant(incumbent.payload)
		if cleanErr != nil {
			return condenseOutcome{}, true, cleanErr
		}
		rightClean, cleanErr := c.subtreeHasNoExternalDescendant(in.payload)
		if cleanErr != nil {
			return condenseOutcome{}, true, cleanErr
		}
		if !leftClean || !rightClean {
			return condenseOutcome{}, true, errors.New("parser-core phase zero: recursive insertion declined external payload")
		}
		if !exactEdge {
			return condenseOutcome{}, true, errors.New("parser-core phase zero: recursive insertion declined shallow non-exact outer edge")
		}
		if !shallow {
			return condenseOutcome{}, true, errors.New("parser-core phase zero: exact recursive edge is not a clean shallow payload")
		}

		handled = true
		mark := c.mark()
		defer c.completeTransaction(mark, &err)
		if phase0AEnabled {
			phase0ABeginPredecessorMerge(c, incumbent.prev, in.prev)
		}
		merged, changed, mergeErr := c.mergePredecessorsOneLayer(incumbent.prev, in.prev)
		if mergeErr != nil {
			if phase0AEnabled {
				phase0AAbortPredecessorMerge(c)
			}
			return condenseOutcome{}, true, mergeErr
		}
		if !changed {
			if phase0AEnabled {
				phase0AAbortPredecessorMerge(c)
				phase0AObserveFactorNoChange(c, key, in, oldID, index)
			}
			return condenseOutcome{head: Head{Node: oldID}, change: condenseUnchanged}, true, nil
		}
		if phase0AEnabled {
			phase0AObserveAdjacencyPublished(c, merged)
		}
		rebuilt := slices.Clone(oldLinks)
		rebuilt[index].prev = merged
		if phase0AEnabled {
			phase0APrepareFactorOuter(c, key, in, oldID, index, merged)
		}
		id, appendErr := c.appendAdjacencyNode(key.state, key.byteOffset, rebuilt)
		if appendErr != nil {
			return condenseOutcome{}, true, appendErr
		}
		if phase0AEnabled {
			phase0AObserveAdjacencyPublished(c, id)
		}
		if err := c.publishBoundary(probe, id); err != nil {
			return condenseOutcome{}, true, err
		}
		if phase0AEnabled {
			phase0AObserveFactorPublished(c, oldID, id)
		}
		return condenseOutcome{head: Head{Node: id}, change: condenseUpdated}, true, nil
	}
	return condenseOutcome{}, false, nil
}

func (c *Core) mergePredecessorsOneLayer(leftID, rightID NodeID) (NodeID, bool, error) {
	if leftID == rightID {
		return 0, false, errors.New("parser-core phase zero: recursive insertion self-merge")
	}
	left, err := c.node(leftID)
	if err != nil {
		return 0, false, err
	}
	right, err := c.node(rightID)
	if err != nil {
		return 0, false, err
	}
	if left.state != right.state || left.byteOffset != right.byteOffset {
		return 0, false, errors.New("parser-core phase zero: recursive predecessors are not boundary-equivalent")
	}
	related, err := c.nodesAncestryRelated(leftID, rightID)
	if err != nil {
		return 0, false, err
	}
	if related {
		return 0, false, errors.New("parser-core phase zero: recursive predecessors are ancestry-related")
	}

	var leftInline [inlineAdjacencyCapacity]linkRecord
	links, err := c.publishedNodeLinksInto(leftInline[:0], *left)
	if err != nil {
		return 0, false, err
	}
	var rightInline [inlineAdjacencyCapacity]linkRecord
	rightLinks, err := c.publishedNodeLinksInto(rightInline[:0], *right)
	if err != nil {
		return 0, false, err
	}
	changed := false
	for _, incoming := range rightLinks {
		var inserted bool
		links, inserted, err = c.insertLinkOneLayer(left.state, left.byteOffset, links, incoming)
		if err != nil {
			return 0, false, err
		}
		changed = changed || inserted
	}
	if !changed {
		return leftID, false, nil
	}
	merged, err := c.appendAdjacencyNode(left.state, left.byteOffset, links)
	if err != nil {
		return 0, false, err
	}
	return merged, true, nil
}

// insertLinkOneLayer mirrors the measured lower-adjacency decisions of
// stack_node_add_link without mutating an existing adjacency. Same-pair
// shallow payloads select the higher effective subtree precedence; every
// other clean class remains in stable incumbent-first order. A second
// different-predecessor merge is outside this tranche and declines.
func (c *Core) insertLinkOneLayer(state StateID, byteOffset uint32, links []linkRecord, incoming linkRecord) ([]linkRecord, bool, error) {
	if len(links) == 0 {
		return append(slices.Clone(links), incoming), true, nil
	}
	c.recordLinkUnionAttempt()
	clean, err := c.subtreeHasNoExternalDescendant(incoming.payload)
	if err != nil {
		c.recordLinkUnionRejected()
		return nil, false, err
	}
	if !clean {
		c.recordLinkUnionRejected()
		return nil, false, errors.New("parser-core phase zero: recursive insertion declined external payload")
	}
	for index, incumbent := range links {
		clean, err := c.subtreeHasNoExternalDescendant(incumbent.payload)
		if err != nil {
			c.recordLinkUnionRejected()
			return nil, false, err
		}
		if !clean {
			c.recordLinkUnionRejected()
			return nil, false, errors.New("parser-core phase zero: recursive insertion declined external payload")
		}
		if c.linkRecordsEqual(incumbent, incoming) {
			c.recordLinkUnionDuplicateNoop()
			if phase0AEnabled {
				phase0AMergeDecision(c, index, phase0ATransitionDuplicateDrop)
			}
			return links, false, nil
		}
		shallow, err := c.shallowPayloadsEqual(incumbent.prev, incumbent.payload, incoming.prev, incoming.payload)
		if err != nil {
			c.recordLinkUnionRejected()
			return nil, false, err
		}
		if !shallow {
			continue
		}
		if incumbent.prev == incoming.prev {
			incumbentPrecedence, err := c.effectivePayloadPrecedence(incumbent.payload, incumbent.scoreDelta)
			if err != nil {
				c.recordLinkUnionRejected()
				return nil, false, err
			}
			incomingPrecedence, err := c.effectivePayloadPrecedence(incoming.payload, incoming.scoreDelta)
			if err != nil {
				c.recordLinkUnionRejected()
				return nil, false, err
			}
			if incomingPrecedence <= incumbentPrecedence {
				c.recordLinkUnionDuplicateNoop()
				if phase0AEnabled {
					phase0AMergeDecision(c, index, phase0ATransitionPrecedenceDrop)
				}
				return links, false, nil
			}
			updated := slices.Clone(links)
			updated[index] = incoming
			c.recordLinkUnionPrecedenceReplaced()
			if phase0AEnabled {
				phase0AMergeDecision(c, index, phase0ATransitionPrecedenceReplacement)
			}
			return updated, true, nil
		}
		mergeable, err := c.predecessorBoundariesMatch(incumbent.prev, incoming.prev)
		if err != nil {
			c.recordLinkUnionRejected()
			return nil, false, err
		}
		if !mergeable {
			continue
		}
		c.recordLinkUnionRejected()
		return nil, false, errors.New("parser-core phase zero: recursive insertion declined beyond one predecessor layer")
	}
	if uint32(len(links)) >= c.limits.MaxLinksPerBoundary {
		c.recordLinkUnionRejected()
		return nil, false, &LiveLinkCapacityError{State: state, ByteOffset: byteOffset, ObservedLinks: uint64(len(links)) + 1, Limit: c.limits.MaxLinksPerBoundary}
	}
	c.recordLinkUnionAlternateAppended()
	if phase0AEnabled {
		phase0AMergeDecision(c, -1, phase0ATransitionAlternateAppend)
	}
	return append(slices.Clone(links), incoming), true, nil
}

func (c *Core) subtreeHasNoExternalDescendant(root SubtreeID) (bool, error) {
	seen := make(map[SubtreeID]bool)
	visiting := make(map[SubtreeID]bool)
	var walk func(SubtreeID) (bool, error)
	walk = func(id SubtreeID) (bool, error) {
		if visiting[id] {
			return false, errors.New("parser-core phase zero: compact subtree cycle during recursive insertion")
		}
		if seen[id] {
			return true, nil
		}
		record, err := c.subtree(id)
		if err != nil {
			return false, err
		}
		if record.external {
			return false, nil
		}
		seen[id] = true
		visiting[id] = true
		defer delete(visiting, id)
		for _, child := range c.children[record.firstChild : record.firstChild+record.childCount] {
			clean, err := walk(child)
			if err != nil || !clean {
				return clean, err
			}
		}
		return true, nil
	}
	return walk(root)
}

func (c *Core) predecessorBoundariesMatch(leftID, rightID NodeID) (bool, error) {
	left, err := c.node(leftID)
	if err != nil {
		return false, err
	}
	right, err := c.node(rightID)
	if err != nil {
		return false, err
	}
	return left.state == right.state && left.byteOffset == right.byteOffset, nil
}

func (c *Core) shallowPayloadsEqual(leftPrev NodeID, leftPayload SubtreeID, rightPrev NodeID, rightPayload SubtreeID) (bool, error) {
	left, leftOK, err := c.shallowPayloadClass(leftPrev, leftPayload)
	if err != nil || !leftOK {
		return false, err
	}
	right, rightOK, err := c.shallowPayloadClass(rightPrev, rightPayload)
	if err != nil || !rightOK {
		return false, err
	}
	return left == right, nil
}

func (c *Core) linkEdgeEqualInput(link linkRecord, in linkInput) bool {
	return link.payload == in.payload && link.scoreDelta == in.scoreDelta &&
		link.hasOrder() == in.order.Present && (!link.hasOrder() || link.order == in.order.Value)
}

func (c *Core) linkEdgesEqual(left, right linkRecord) bool {
	return left.payload == right.payload && left.scoreDelta == right.scoreDelta &&
		left.hasOrder() == right.hasOrder() && (!left.hasOrder() || left.order == right.order)
}

func (c *Core) linkRecordsEqual(left, right linkRecord) bool {
	return left.prev == right.prev && c.linkEdgesEqual(left, right)
}

func (c *Core) nodesAncestryRelated(left, right NodeID) (bool, error) {
	if left == right {
		return true, nil
	}
	// Published edges strictly decrease, so only the newer ID can reach the
	// older one. Avoid a second traversal that is impossible by construction.
	if left > right {
		return c.nodeReaches(left, right)
	}
	return c.nodeReaches(right, left)
}

func (c *Core) nodeReaches(start, target NodeID) (bool, error) {
	seen := make(map[NodeID]bool)
	var walk func(NodeID) (bool, error)
	walk = func(id NodeID) (bool, error) {
		if id == target {
			return true, nil
		}
		if id < target {
			return false, nil
		}
		if seen[id] {
			return false, nil
		}
		seen[id] = true
		node, err := c.node(id)
		if err != nil {
			return false, err
		}
		var inline [inlineAdjacencyCapacity]linkRecord
		links, err := c.publishedNodeLinksInto(inline[:0], *node)
		if err != nil {
			return false, err
		}
		for _, link := range links {
			if link.prev == 0 || link.prev >= id {
				return false, errors.New("parser-core phase zero: graph predecessor does not decrease during recursive insertion")
			}
			reaches, err := walk(link.prev)
			if err != nil || reaches {
				return reaches, err
			}
		}
		return false, nil
	}
	return walk(start)
}

func (c *Core) appendAdjacencyNode(state StateID, byteOffset uint32, links []linkRecord) (NodeID, error) {
	if len(links) == 0 {
		return 0, errors.New("parser-core phase zero: recursive insertion produced empty adjacency")
	}
	if uint64(len(links)) > uint64(c.limits.MaxLinksPerBoundary) {
		return 0, &LiveLinkCapacityError{State: state, ByteOffset: byteOffset, ObservedLinks: uint64(len(links)), Limit: c.limits.MaxLinksPerBoundary}
	}
	if uint64(len(c.links))+uint64(len(links)) > uint64(c.limits.MaxLinks) || uint64(len(c.links))+uint64(len(links)) > math.MaxUint32 {
		return 0, errors.New("parser-core phase zero: link arena cap")
	}
	if uint64(len(c.nodes))+1 > uint64(c.limits.MaxNodes) || uint64(len(c.nodes)) >= math.MaxUint32 {
		return 0, errors.New("parser-core phase zero: node arena cap")
	}
	next := NodeID(uint64(len(c.nodes)) + 1)
	pathCount := uint64(0)
	for _, link := range links {
		if link.prev == 0 || link.prev >= next {
			return 0, fmt.Errorf("parser-core phase zero: graph predecessor %d must be lower than new node %d", link.prev, next)
		}
		prev, err := c.node(link.prev)
		if err != nil {
			return 0, err
		}
		pathCount = saturatingAddPaths(pathCount, prev.pathCount)
	}
	var first LinkID
	for _, stored := range links {
		copy := stored
		copy.next = first
		c.links = append(c.links, copy)
		c.addWork(&c.work.GraphLinkAdditionsProxy, 1)
		first = LinkID(len(c.links))
	}
	return c.appendNode(nodeRecord{
		state: state, byteOffset: byteOffset, firstLink: uint32(first),
		linkCount: uint32(len(links)), pathCount: pathCount,
	})
}

func (c *Core) shallowPayloadClass(prevID NodeID, payloadID SubtreeID) (shallowPayloadClass, bool, error) {
	prev, err := c.node(prevID)
	if err != nil {
		return shallowPayloadClass{}, false, err
	}
	payload, err := c.subtree(payloadID)
	if err != nil {
		return shallowPayloadClass{}, false, err
	}
	// The compact phase-zero core cannot represent recovery/error subtrees yet,
	// so every resident non-external payload is clean by construction. External
	// scanner payloads remain ineligible until scanner-state equality is part of
	// the compact graph.
	if payload.external {
		return shallowPayloadClass{}, false, nil
	}
	if payload.startByte < prev.byteOffset || payload.endByte < payload.startByte {
		return shallowPayloadClass{}, false, errors.New("parser-core phase zero: invalid shallow payload extent")
	}
	return shallowPayloadClass{
		symbol: payload.symbol, padding: payload.startByte - prev.byteOffset,
		size: payload.endByte - payload.startByte, childCount: payload.childCount,
		extra: payload.extra,
	}, true, nil
}

// replaceBoundaryLink publishes a new canonical adjacency while leaving the
// historical head and every link reachable from it immutable. oldLinks are in
// stable insertion order, so rebuilding them through prepends preserves the
// order observed by nodeLinks.
func (c *Core) replaceBoundaryLink(key boundaryKey, probe boundaryProbe, old nodeRecord, oldLinks []linkRecord, candidate int, in linkInput) (Head, error) {
	if candidate < 0 || candidate >= len(oldLinks) || uint32(len(oldLinks)) != old.linkCount {
		return Head{}, errors.New("parser-core phase zero: invalid shallow-fold candidate")
	}
	if uint64(len(c.links))+uint64(len(oldLinks)) > uint64(c.limits.MaxLinks) || uint64(len(c.links))+uint64(len(oldLinks)) > math.MaxUint32 {
		return Head{}, errors.New("parser-core phase zero: link arena cap")
	}
	if uint64(len(c.nodes))+1 > uint64(c.limits.MaxNodes) || uint64(len(c.nodes)) >= math.MaxUint32 {
		return Head{}, errors.New("parser-core phase zero: node arena cap")
	}

	linkMark := len(c.links)
	var first LinkID
	for index, stored := range oldLinks {
		copy := stored
		if index == candidate {
			copy.prev = in.prev
			copy.payload = in.payload
			copy.scoreDelta = in.scoreDelta
			copy.order = in.order.Value
			copy.flags &^= linkFlagHasOrder
			if in.order.Present {
				copy.flags |= linkFlagHasOrder
			}
		}
		copy.next = first
		c.links = append(c.links, copy)
		c.addWork(&c.work.GraphLinkAdditionsProxy, 1)
		first = LinkID(len(c.links))
	}
	id, err := c.appendNode(nodeRecord{
		state: key.state, byteOffset: key.byteOffset,
		firstLink: uint32(first), linkCount: old.linkCount, pathCount: old.pathCount,
	})
	if err != nil {
		c.links = c.links[:linkMark]
		return Head{}, err
	}
	if err := c.publishBoundary(probe, id); err != nil {
		return Head{}, err
	}
	if phase0AEnabled {
		phase0AObserveReplacementPublished(c, key, in, id, LinkID(linkMark+1), len(oldLinks), candidate)
	}
	return Head{Node: id}, nil
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

type popEnumerationScratch struct {
	busy       bool
	linkFrames [][]linkRecord
	rev        []SubtreeID
	revScores  []int64
	revOrders  []ForkOrder
	trailing   []pathPayload
	paths      []popPath
}

func (s *popEnumerationScratch) begin() {
	for index := range s.linkFrames {
		s.linkFrames[index] = s.linkFrames[index][:0]
	}
	s.rev = s.rev[:0]
	s.revScores = s.revScores[:0]
	s.revOrders = s.revOrders[:0]
	s.trailing = s.trailing[:0]
	s.paths = s.paths[:0]
}

func (s *popEnumerationScratch) finishTraversal() {
	for index := range s.linkFrames {
		s.linkFrames[index] = s.linkFrames[index][:0]
	}
	s.rev = s.rev[:0]
	s.revScores = s.revScores[:0]
	s.revOrders = s.revOrders[:0]
	s.trailing = s.trailing[:0]
}

func (s *popEnumerationScratch) resetLogical() {
	s.finishTraversal()
	s.paths = s.paths[:0]
	s.busy = false
}

func (s *popEnumerationScratch) nextPath() *popPath {
	index := len(s.paths)
	if index == cap(s.paths) {
		s.paths = append(s.paths, popPath{})
	} else {
		s.paths = s.paths[:index+1]
		previous := s.paths[index]
		s.paths[index] = popPath{
			children: previous.children[:0],
			trailing: previous.trailing[:0],
		}
	}
	return &s.paths[index]
}

// popPaths returns Core-owned ephemeral storage. ReduceOutputs consumes the
// complete result before any later popPaths call, so serial calls may reuse
// path slots and their child/trailing buffers. Paths within one result remain
// independent and retain authentic enumeration order.
func (c *Core) popPaths(head NodeID, childCount int) (out []popPath, err error) {
	scratch := &c.popScratch
	scratch.begin()
	defer func() {
		scratch.finishTraversal()
		if err != nil {
			scratch.paths = scratch.paths[:0]
		}
	}()
	if childCount == 0 {
		n, err := c.node(head)
		if err != nil {
			return nil, err
		}
		path := scratch.nextPath()
		path.prev = head
		path.startByte = n.byteOffset
		path.structuralEnd = n.byteOffset
		return scratch.paths, nil
	}
	if complete, err := c.popSingleLinkPath(head, childCount, scratch); err != nil {
		return nil, err
	} else if complete {
		return scratch.paths, nil
	}
	// The fast probe may have accumulated a partial reverse path before it met
	// an ambiguous node. Restart the authenticated generic traversal from the
	// original head so enumeration order and every cap remain unchanged.
	scratch.begin()
	var walk func(NodeID, int, bool, uint32, int) error
	walk = func(id NodeID, remaining int, peelingTrailing bool, structuralEnd uint32, depth int) error {
		n, err := c.node(id)
		if err != nil {
			return err
		}
		for len(scratch.linkFrames) <= depth {
			scratch.linkFrames = append(scratch.linkFrames, nil)
		}
		links, err := c.nodeLinksInto(scratch.linkFrames[depth], *n)
		scratch.linkFrames[depth] = links
		if err != nil {
			return err
		}
		for _, link := range links {
			// Every published graph edge points to an older node. The strict local
			// decrease proves acyclicity without a traversal map while still
			// rejecting corrupted diagnostic arena records before recursion.
			if link.prev == 0 || link.prev >= id {
				return errors.New("parser-core phase zero: graph predecessor does not decrease")
			}
			payload, err := c.subtree(link.payload)
			if err != nil {
				return err
			}
			linkOrder := ForkOrder{Value: link.order, Present: link.hasOrder()}
			if payload.extra && peelingTrailing {
				scratch.trailing = append(scratch.trailing, pathPayload{payload: link.payload, scoreDelta: link.scoreDelta, order: linkOrder})
				if err := walk(link.prev, remaining, true, structuralEnd, depth+1); err != nil {
					return err
				}
				scratch.trailing = scratch.trailing[:len(scratch.trailing)-1]
				continue
			}
			if payload.extra {
				scratch.rev = append(scratch.rev, link.payload)
				scratch.revScores = append(scratch.revScores, link.scoreDelta)
				scratch.revOrders = append(scratch.revOrders, linkOrder)
				if err := walk(link.prev, remaining, false, structuralEnd, depth+1); err != nil {
					return err
				}
				scratch.rev = scratch.rev[:len(scratch.rev)-1]
				scratch.revScores = scratch.revScores[:len(scratch.revScores)-1]
				scratch.revOrders = scratch.revOrders[:len(scratch.revOrders)-1]
				continue
			}
			nextStructuralEnd := structuralEnd
			if peelingTrailing {
				nextStructuralEnd = payload.endByte
			}
			scratch.rev = append(scratch.rev, link.payload)
			scratch.revScores = append(scratch.revScores, link.scoreDelta)
			scratch.revOrders = append(scratch.revOrders, linkOrder)
			next := remaining - 1
			if next == 0 {
				if uint64(len(scratch.paths)) >= c.limits.MaxPopPaths {
					return errors.New("parser-core phase zero: pop enumeration cap")
				}
				path := scratch.nextPath()
				path.prev = link.prev
				path.startByte = payload.startByte
				path.structuralEnd = nextStructuralEnd
				for i := len(scratch.rev) - 1; i >= 0; i-- {
					path.children = append(path.children, scratch.rev[i])
					path.score, err = checkedAddScore(path.score, scratch.revScores[i])
					if err != nil {
						return err
					}
					if scratch.revOrders[i].Present {
						path.order = scratch.revOrders[i]
					}
				}
				for i := len(scratch.trailing) - 1; i >= 0; i-- {
					path.trailing = append(path.trailing, scratch.trailing[i])
					if scratch.trailing[i].order.Present {
						path.order = scratch.trailing[i].order
					}
				}
			} else if err := walk(link.prev, next, false, nextStructuralEnd, depth+1); err != nil {
				return err
			}
			scratch.rev = scratch.rev[:len(scratch.rev)-1]
			scratch.revScores = scratch.revScores[:len(scratch.revScores)-1]
			scratch.revOrders = scratch.revOrders[:len(scratch.revOrders)-1]
		}
		return nil
	}
	if err := walk(head, childCount, true, 0, 0); err != nil {
		return nil, err
	}
	return scratch.paths, nil
}

// popSingleLinkPath handles the common clean GSS shape without recursive
// dispatch, adjacency copies, or depth-indexed link-frame traffic. It returns
// complete=false without publishing a path when any visited node branches;
// the caller then restarts the existing generic enumerator from the original
// head. Scratch may contain a partial reverse path on that decline.
func (c *Core) popSingleLinkPath(head NodeID, childCount int, scratch *popEnumerationScratch) (complete bool, err error) {
	id := head
	remaining := childCount
	peelingTrailing := true
	structuralEnd := uint32(0)
	for {
		n, err := c.node(id)
		if err != nil {
			return false, err
		}
		count := uint64(n.linkCount)
		if count > uint64(c.limits.MaxLinks) || count > uint64(c.limits.MaxLinksPerBoundary) {
			return false, errors.New("parser-core phase zero: recorded link count exceeds configured limit")
		}
		if count > uint64(len(c.links)) {
			return false, errors.New("parser-core phase zero: recorded link count exceeds link arena")
		}
		if n.linkCount != 1 {
			return false, nil
		}
		linkID := LinkID(n.firstLink)
		if linkID == 0 {
			return false, errors.New("parser-core phase zero: adjacency shorter than recorded link count")
		}
		if uint64(linkID) > uint64(len(c.links)) {
			return false, errors.New("parser-core phase zero: link adjacency out of range")
		}
		link := c.links[linkID-1]
		if link.next != 0 {
			return false, errors.New("parser-core phase zero: adjacency exceeds recorded link count or cycles")
		}
		if link.prev == 0 || link.prev >= id {
			return false, errors.New("parser-core phase zero: graph predecessor does not decrease")
		}
		payload, err := c.subtree(link.payload)
		if err != nil {
			return false, err
		}
		linkOrder := ForkOrder{Value: link.order, Present: link.hasOrder()}
		if payload.extra && peelingTrailing {
			scratch.trailing = append(scratch.trailing, pathPayload{payload: link.payload, scoreDelta: link.scoreDelta, order: linkOrder})
			id = link.prev
			continue
		}
		scratch.rev = append(scratch.rev, link.payload)
		scratch.revScores = append(scratch.revScores, link.scoreDelta)
		scratch.revOrders = append(scratch.revOrders, linkOrder)
		if payload.extra {
			id = link.prev
			continue
		}
		if peelingTrailing {
			structuralEnd = payload.endByte
			peelingTrailing = false
		}
		remaining--
		if remaining != 0 {
			id = link.prev
			continue
		}
		if uint64(len(scratch.paths)) >= c.limits.MaxPopPaths {
			return false, errors.New("parser-core phase zero: pop enumeration cap")
		}
		path := scratch.nextPath()
		path.prev = link.prev
		path.startByte = payload.startByte
		path.structuralEnd = structuralEnd
		for index := len(scratch.rev) - 1; index >= 0; index-- {
			path.children = append(path.children, scratch.rev[index])
			path.score, err = checkedAddScore(path.score, scratch.revScores[index])
			if err != nil {
				return false, err
			}
			if scratch.revOrders[index].Present {
				path.order = scratch.revOrders[index]
			}
		}
		for index := len(scratch.trailing) - 1; index >= 0; index-- {
			path.trailing = append(path.trailing, scratch.trailing[index])
			if scratch.trailing[index].order.Present {
				path.order = scratch.trailing[index].order
			}
		}
		return true, nil
	}
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
				if uint64(len(paths)) >= c.limits.MaxDerivations {
					return nil, ErrDerivationEnumerationCap
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
		if n.pathCount != math.MaxUint64 && uint64(len(paths)) != n.pathCount {
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

func saturatingAddPaths(left, right uint64) uint64 {
	if math.MaxUint64-left < right {
		return math.MaxUint64
	}
	return left + right
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

// MaterializationOrder authenticates roots as one ownership tree and returns
// its subtree IDs in child-before-parent order. Compact subtrees can be shared
// safely while they remain immutable parser payloads, but public gotreesitter
// Nodes carry one mutable parent/index pair. A repeated compact ID therefore
// cannot be materialized by pointer memoization without corrupting navigation.
//
// The walk is bounded by the already-capped subtree and child arenas. poll is
// called at a coarse cadence and before success so callers can enforce their
// cancellation and memory contracts during diagnostic materialization.
func (c *Core) MaterializationOrder(roots []SubtreeID, poll func() error) ([]SubtreeID, error) {
	if c == nil || len(roots) == 0 {
		return nil, errors.New("parser-core phase zero: materialization requires at least one compact root")
	}
	if poll == nil {
		poll = func() error { return nil }
	}
	if err := poll(); err != nil {
		return nil, err
	}

	// IDs are one-based and dense within the capped arena.
	colors := make([]uint8, len(c.subtrees)+1)
	incoming := make([]uint8, len(c.subtrees)+1)
	if err := poll(); err != nil {
		return nil, err
	}
	type frame struct {
		id   SubtreeID
		next uint32
	}
	order := make([]SubtreeID, 0, len(c.subtrees))
	var work uint64
	pollWork := func() error {
		work++
		if work&255 == 0 {
			return poll()
		}
		return nil
	}

	for _, root := range roots {
		if _, err := c.subtree(root); err != nil {
			return nil, err
		}
		if incoming[root] != 0 {
			return nil, errors.New("parser-core phase zero: compact subtree has repeated public-tree ownership")
		}
		incoming[root] = 1
		if colors[root] != 0 {
			return nil, errors.New("parser-core phase zero: compact subtree has repeated public-tree ownership")
		}
		colors[root] = 1
		stack := []frame{{id: root}}
		for len(stack) != 0 {
			if err := pollWork(); err != nil {
				return nil, err
			}
			top := &stack[len(stack)-1]
			record, err := c.subtree(top.id)
			if err != nil {
				return nil, err
			}
			if top.next < record.childCount {
				child := c.children[record.firstChild+top.next]
				top.next++
				if _, err := c.subtree(child); err != nil {
					return nil, err
				}
				if colors[child] == 1 {
					return nil, errors.New("parser-core phase zero: compact subtree cycle during materialization")
				}
				if incoming[child] != 0 || colors[child] == 2 {
					return nil, errors.New("parser-core phase zero: compact subtree has repeated public-tree ownership")
				}
				incoming[child] = 1
				colors[child] = 1
				stack = append(stack, frame{id: child})
				continue
			}
			if err := c.validateMaterializationMetadata(top.id, *record); err != nil {
				return nil, err
			}
			colors[top.id] = 2
			order = append(order, top.id)
			stack = stack[:len(stack)-1]
		}
	}
	if len(order) > len(c.subtrees) {
		return nil, errors.New("parser-core phase zero: materialization exceeded compact subtree arena")
	}
	if err := poll(); err != nil {
		return nil, err
	}
	return order, nil
}

// VisitMaterializationPostorder authenticates roots as one ownership tree and
// invokes visit exactly once per subtree after all of its children. It fuses
// unique-ownership validation, iterative postorder traversal, metadata
// authentication, and borrowed compact access so production materialization
// does not allocate an order or copy subtree sidecars.
func (c *Core) VisitMaterializationPostorder(
	roots []SubtreeID,
	poll func() error,
	visit func(SubtreeID, MaterializationSubtreeView) error,
) error {
	if c == nil || len(roots) == 0 {
		return errors.New("parser-core phase zero: materialization requires at least one compact root")
	}
	if visit == nil {
		return errors.New("parser-core phase zero: materialization requires a visitor")
	}
	if poll == nil {
		poll = func() error { return nil }
	}
	if err := poll(); err != nil {
		return err
	}

	// Zero is unseen, one is the active path, and two is already owned. This
	// single state vector proves both acyclicity and unique public ownership.
	colors := make([]uint8, len(c.subtrees)+1)
	type frame struct {
		id   SubtreeID
		next uint32
	}
	stack := make([]frame, 0, 64)
	var visited, work uint64
	for _, root := range roots {
		if _, err := c.subtree(root); err != nil {
			return err
		}
		if colors[root] != 0 {
			return errors.New("parser-core phase zero: compact subtree has repeated public-tree ownership")
		}
		colors[root] = 1
		stack = append(stack, frame{id: root})
		for len(stack) != 0 {
			work++
			if work&255 == 0 {
				if err := poll(); err != nil {
					return err
				}
			}
			top := &stack[len(stack)-1]
			record, err := c.subtree(top.id)
			if err != nil {
				return err
			}
			if top.next < record.childCount {
				child := c.children[record.firstChild+top.next]
				top.next++
				if _, err := c.subtree(child); err != nil {
					return err
				}
				switch colors[child] {
				case 0:
					colors[child] = 1
					stack = append(stack, frame{id: child})
					continue
				case 1:
					return errors.New("parser-core phase zero: compact subtree cycle during materialization")
				default:
					return errors.New("parser-core phase zero: compact subtree has repeated public-tree ownership")
				}
			}
			if err := c.validateMaterializationMetadata(top.id, *record); err != nil {
				return err
			}
			view := MaterializationSubtreeView{
				Symbol: record.symbol, ProductionID: record.productionID,
				DynamicPrecedence: record.dynamicPrecedence,
				StartByte:         record.startByte, EndByte: record.endByte,
				Children: c.children[record.firstChild : record.firstChild+record.childCount],
				Extra:    record.extra, External: record.external, Terminal: record.terminal,
			}
			if err := visit(top.id, view); err != nil {
				return err
			}
			colors[top.id] = 2
			visited++
			stack = stack[:len(stack)-1]
		}
	}
	if visited > uint64(len(c.subtrees)) {
		return errors.New("parser-core phase zero: materialization exceeded compact subtree arena")
	}
	return poll()
}

func (c *Core) validateMaterializationMetadata(id SubtreeID, record subtreeRecord) error {
	// Production construction authenticates every terminal and reduction before
	// publication. Compact arenas are immutable, so repeating the table remap at
	// materialization adds no evidence. The flag is Core-scoped so subtreeRecord
	// remains byte-for-byte unchanged on the scheduler's hot equality/copy path.
	if c.metadataConstructionAuthenticated {
		return nil
	}
	return c.validateGenericMaterializationMetadata(id, record)
}

func (c *Core) validateGenericMaterializationMetadata(id SubtreeID, record subtreeRecord) error {
	children := c.children[record.firstChild : record.firstChild+record.childCount]
	storedFields := c.fields[record.firstField : record.firstField+record.fieldCount]
	storedAliases := c.aliases[record.firstAlias : record.firstAlias+record.aliasCount]
	if record.terminal {
		if len(children) != 0 || len(storedFields) != 0 || len(storedAliases) != 0 {
			return fmt.Errorf("parser-core phase zero: terminal subtree %d carries reduction metadata", id)
		}
		return nil
	}
	structuralCount := 0
	for _, child := range children {
		payload, err := c.subtree(child)
		if err != nil {
			return err
		}
		if !payload.extra {
			structuralCount++
		}
	}
	plan, err := c.reductionPlanForPair(record.productionID, structuralCount)
	if err != nil {
		return err
	}
	fields, aliases, err := c.remapReductionPlan(children, &plan, &c.reductionScratch)
	if err != nil {
		return err
	}
	if !slices.Equal(fields, storedFields) || !slices.Equal(aliases, storedAliases) {
		return fmt.Errorf("parser-core phase zero: compact subtree %d production metadata does not match authenticated tables", id)
	}
	return nil
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

// Work returns a value copy of the committed compact-core work counters.
func (c *Core) Work() Work {
	if c == nil {
		return Work{}
	}
	return c.work
}

// RawSelectedSubtreeCensus walks accepted compact payload roots before any
// public visibility or unary-collapse rule. The traversal is occurrence based
// and fail-closed on invalid IDs, cycles, or counter overflow.
func (c *Core) RawSelectedSubtreeCensus(roots []SubtreeID) (RawSelectedCensus, error) {
	if c == nil || len(roots) == 0 {
		return RawSelectedCensus{}, errors.New("parser-core phase zero: raw selected census requires roots")
	}
	type frame struct {
		id   SubtreeID
		exit bool
	}
	stack := make([]frame, 0, len(roots))
	for index := len(roots) - 1; index >= 0; index-- {
		stack = append(stack, frame{id: roots[index]})
	}
	active := make([]bool, len(c.subtrees)+1)
	var census RawSelectedCensus
	add := func(slot *uint64) error {
		if *slot == math.MaxUint64 {
			census.Overflow = true
			return errors.New("parser-core phase zero: raw selected census overflow")
		}
		*slot++
		return nil
	}
	for len(stack) != 0 {
		item := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if item.id == 0 || uint64(item.id) > uint64(len(c.subtrees)) {
			return RawSelectedCensus{}, fmt.Errorf("parser-core phase zero: raw selected census invalid subtree %d", item.id)
		}
		if item.exit {
			active[item.id] = false
			continue
		}
		if active[item.id] {
			return RawSelectedCensus{}, errors.New("parser-core phase zero: raw selected census cycle")
		}
		active[item.id] = true
		stack = append(stack, frame{id: item.id, exit: true})
		record := c.subtrees[item.id-1]
		if err := add(&census.Nodes); err != nil {
			return census, err
		}
		if record.childCount == 0 {
			if err := add(&census.Leaves); err != nil {
				return census, err
			}
		} else {
			if err := add(&census.Parents); err != nil {
				return census, err
			}
		}
		children := c.children[record.firstChild : record.firstChild+record.childCount]
		for index := len(children) - 1; index >= 0; index-- {
			stack = append(stack, frame{id: children[index]})
		}
	}
	return census, nil
}

func (c *Core) appendNode(r nodeRecord) (NodeID, error) {
	if uint64(len(c.nodes))+1 > uint64(c.limits.MaxNodes) || uint64(len(c.nodes)) >= math.MaxUint32 {
		return 0, errors.New("parser-core phase zero: node arena cap")
	}
	next := NodeID(uint64(len(c.nodes)) + 1)
	if err := c.validatePublishedNodeDAG(r, next); err != nil {
		return 0, err
	}
	c.nodes = append(c.nodes, r)
	return next, nil
}

// validatePublishedNodeDAG authenticates the append-only persistent graph at
// its single publication point. Since NodeIDs are dense arena ordinals, a
// strict predecessor decrease is both necessary and sufficient to rule out a
// cycle. The bounded adjacency walk also keeps malformed internal/diagnostic
// records fail closed without a hash table.
func (c *Core) validatePublishedNodeDAG(r nodeRecord, next NodeID) error {
	count := uint64(r.linkCount)
	if count == 0 {
		if r.firstLink != 0 {
			return errors.New("parser-core phase zero: empty adjacency has nonzero first link")
		}
		return nil
	}
	if count > uint64(c.limits.MaxLinks) || count > uint64(c.limits.MaxLinksPerBoundary) {
		return errors.New("parser-core phase zero: recorded link count exceeds configured limit")
	}
	if count > uint64(len(c.links)) {
		return errors.New("parser-core phase zero: recorded link count exceeds link arena")
	}
	id := LinkID(r.firstLink)
	for remaining := r.linkCount; remaining > 0; remaining-- {
		if id == 0 {
			return errors.New("parser-core phase zero: adjacency shorter than recorded link count")
		}
		if uint64(id) > uint64(len(c.links)) {
			return errors.New("parser-core phase zero: link adjacency out of range")
		}
		link := c.links[id-1]
		if link.prev == 0 || link.prev >= next {
			return fmt.Errorf("parser-core phase zero: graph predecessor %d must be lower than new node %d", link.prev, next)
		}
		id = link.next
	}
	if id != 0 {
		return errors.New("parser-core phase zero: adjacency exceeds recorded link count or cycles")
	}
	return nil
}

func (c *Core) appendSubtree(r subtreeRecord, children []SubtreeID, fields []FieldMapEntry, aliases []Symbol) (SubtreeID, error) {
	// Generic diagnostic/test publication is deliberately unauthenticated.
	// Clearing this Core-scoped invariant is monotonic until Reset, including
	// failed or rolled-back diagnostic publication, which is conservative.
	c.metadataConstructionAuthenticated = false
	return c.appendSubtreeRecord(r, children, fields, aliases)
}

func (c *Core) appendAuthenticatedTerminal(r subtreeRecord) (SubtreeID, error) {
	// The AST provenance ratchet requires every caller to pass a subtreeRecord
	// literal with terminal:true. Keep this seam as pure forwarding so terminal
	// construction does not add a partial store before the hot record copy.
	return c.appendSubtreeRecord(r, nil, nil, nil)
}

func (c *Core) appendSubtreeRecord(r subtreeRecord, children []SubtreeID, fields []FieldMapEntry, aliases []Symbol) (SubtreeID, error) {
	if uint64(len(c.subtrees))+1 > uint64(c.limits.MaxSubtrees) || uint64(len(c.subtrees)) >= math.MaxUint32 {
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
	if r.terminal {
		c.addWork(&c.work.LeafConstructionsProxy, 1)
	} else {
		c.addWork(&c.work.ParentConstructionsProxy, 1)
	}
	return SubtreeID(len(c.subtrees)), nil
}

func (c *Core) addWork(counter *uint64, delta uint64) {
	if math.MaxUint64-*counter < delta {
		*counter = math.MaxUint64
		c.work.Overflow = true
		return
	}
	*counter += delta
}

func (c *Core) recordLinkUnionAttempt() {
	c.addWork(&c.work.PredecessorLinkUnionAttempts, 1)
}

func (c *Core) recordLinkUnionDuplicateNoop() {
	c.addWork(&c.work.PredecessorLinkUnionDuplicateNoop, 1)
}

func (c *Core) recordLinkUnionPrecedenceReplaced() {
	c.addWork(&c.work.PredecessorLinkUnionPrecedenceReplaced, 1)
}

func (c *Core) recordLinkUnionRecursiveChanged() {
	c.addWork(&c.work.PredecessorLinkUnionRecursiveChanged, 1)
}

func (c *Core) recordLinkUnionAlternateAppended() {
	c.addWork(&c.work.PredecessorLinkUnionAlternateAppended, 1)
}

func (c *Core) recordLinkUnionRejected() {
	c.addWork(&c.work.PredecessorLinkUnionRejected, 1)
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

// nodeLinksInto copies one immutable adjacency chain into caller-owned
// storage in stable insertion order. The chain itself is newest-first because
// links prepend in O(1), so filling the destination backwards avoids both a
// second pass and per-enumeration allocation. The recorded count bounds the
// traversal: an early zero is short, while a nonzero successor after exactly
// that many records is either overlong or cyclic and is rejected fail closed.
func (c *Core) nodeLinksInto(dst []linkRecord, n nodeRecord) ([]linkRecord, error) {
	return c.nodeLinksIntoBounded(dst, n, c.limits.MaxLinksPerBoundary)
}

// publishedNodeLinksInto reads an already-authenticated immutable node. A
// caller may lower MaxLinksPerBoundary after publication to ratchet subsequent
// writes, so an older legal node remains readable up to its recorded width.
// appendNode authenticated that width against the then-current configured cap.
func (c *Core) publishedNodeLinksInto(dst []linkRecord, n nodeRecord) ([]linkRecord, error) {
	return c.nodeLinksIntoBounded(dst, n, n.linkCount)
}

func (c *Core) nodeLinksIntoBounded(dst []linkRecord, n nodeRecord, maxCount uint32) ([]linkRecord, error) {
	count := uint64(n.linkCount)
	if count > uint64(c.limits.MaxLinks) || count > uint64(maxCount) {
		return dst[:0], errors.New("parser-core phase zero: recorded link count exceeds configured limit")
	}
	if count > uint64(len(c.links)) {
		return dst[:0], errors.New("parser-core phase zero: recorded link count exceeds link arena")
	}
	if cap(dst) < int(n.linkCount) {
		dst = make([]linkRecord, n.linkCount)
	} else {
		dst = dst[:n.linkCount]
	}
	id := LinkID(n.firstLink)
	for index := len(dst) - 1; index >= 0; index-- {
		if id == 0 {
			return dst, errors.New("parser-core phase zero: adjacency shorter than recorded link count")
		}
		if uint64(id) > uint64(len(c.links)) {
			return dst, errors.New("parser-core phase zero: link adjacency out of range")
		}
		link := c.links[id-1]
		dst[index] = link
		id = link.next
	}
	if id != 0 {
		return dst, errors.New("parser-core phase zero: adjacency exceeds recorded link count or cycles")
	}
	return dst, nil
}

func checkedAddScore(a, b int64) (int64, error) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, errors.New("parser-core phase zero: score overflow")
	}
	return a + b, nil
}
