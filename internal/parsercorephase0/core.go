// Package parsercorephase0 contains the admitted compact parser core.
//
// The root package routes every eligible fresh full parse through this
// engine by default, regardless of source size. The admission switch in
// admission_switch.go controls this routing; a scheduler stop-control poll
// (memory budget, deadline, cancellation) bounds a large input instead of a
// source-length eligibility decline.
//
// The engine consumes a dependency-neutral TableView. It owns compact stack
// advancement and the admitted error-region recovery stages. It does not own
// lexer selection, external-scanner election, retry policy, or included
// ranges. Incremental parsing stays on the production engine.
//
// The engine fails closed. A decline at any eligibility check, or during
// the engine run, sends the parse to the production lane. This package
// never substitutes partial work.
//
// Build with -tags gts_no_parsercorephase0 as the emergency opt-out. This
// tag removes the engine. Every full parse then runs on the production
// lane.
package parsercorephase0

import (
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"sync"
	"unsafe"
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
	kind              ActionRowKind
	hasShift          bool
	hasReduce         bool
	dispatchSupported bool
}

func (d ActionRowDescriptor) Kind() ActionRowKind { return d.kind }
func (d ActionRowDescriptor) HasShift() bool      { return d.hasShift }
func (d ActionRowDescriptor) HasReduce() bool     { return d.hasReduce }

// DispatchSupported reports whether the root package's generic scheduler
// dispatch loop can prove this row is not an unsupported cell without
// reading the current token. It holds exactly for ActionRowShift,
// ActionRowReduce, and ActionRowConflict: parsercore_phase0_driver.go's
// diagnosticParserCoreGenericUnsupportedCellDescriptor returns "supported"
// (a nil decline) for those three kinds unconditionally, never consulting
// its token argument, while ActionRowExtraShift and ActionRowAccept still
// need the token's width/EOF shape and ActionRowEmpty/ActionRowUnsupported
// are never supported. This is table-derived and immutable once the row is
// decoded (describeActionRow computes it once per distinct action row, not
// once per dispatch pass), so the dispatch loop's per-cell, per-pass
// unsupported check can read this field instead of re-deriving the same
// kind-only fact on every pass a cell with this shape is dispatched
// (spec.campaign.v7 tranche C0 item 4, the "cell-array and descriptor
// validation" L1 sub-item). Keep this in sync with that function's switch.
func (d ActionRowDescriptor) DispatchSupported() bool { return d.dispatchSupported }

type actionRowData struct {
	actions    []Action
	descriptor ActionRowDescriptor
	reusable   bool
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

// NewActionRow snapshots actions into an immutable row. reusable carries the
// language table's per-entry token-reuse eligibility bit (Language.
// ParseActions[i].Reusable) through to the decoded row unchanged; it is not
// derived from actions and does not affect Descriptor's dispatch shape.
func NewActionRow(actions []Action, reusable bool) ActionRow {
	if len(actions) == 0 {
		return ActionRow{}
	}
	snapshot := append([]Action(nil), actions...)
	return ActionRow{data: &actionRowData{
		actions: snapshot, descriptor: describeActionRow(snapshot), reusable: reusable,
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

// Reusable reports the language table's token-reuse eligibility bit for this
// row, unchanged from Language.ParseActions[i].Reusable. Nothing reads it
// yet; it is substrate for a later forced-reuse tranche.
func (r ActionRow) Reusable() bool {
	if r.data == nil {
		return false
	}
	return r.data.reusable
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
		descriptor.dispatchSupported = true
		return descriptor
	}
	switch actions[0].Type {
	case ActionShift:
		if actions[0].Extra {
			descriptor.kind = ActionRowExtraShift
		} else {
			descriptor.kind = ActionRowShift
			descriptor.dispatchSupported = true
		}
	case ActionReduce:
		descriptor.kind = ActionRowReduce
		descriptor.dispatchSupported = true
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

// TableIdentityProvider supplies the immutable identity of the table producer.
// A compact core must not replay parser states after its producer changes.
// Adapters that do not provide an identity cannot prove replay compatibility.
type TableIdentityProvider interface {
	TableIdentity() ([32]byte, bool)
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
	// MaxDropCohortRefs bounds the shared spill arena for drop-cohort
	// references. Inline references remain part of their owning records.
	MaxDropCohortRefs     uint32
	MaxDropCohortRefBytes uint64
	// MaxDropCohorts bounds one scheduler session. MaxDropCohortMembers
	// bounds the stable branch set of one cohort.
	MaxDropCohorts       uint32
	MaxDropCohortMembers uint16
	MaxDropCohortBytes   uint64
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
	if l.MaxDropCohortRefs == 0 {
		// Keep enough room for the bounded reference history of every node.
		// The set itself remains capped by dropCohortRefHardCap.
		maxRefs := uint64(l.MaxNodes) * uint64(dropCohortRefHardCap)
		if maxRefs > math.MaxUint32 {
			l.MaxDropCohortRefs = math.MaxUint32
		} else {
			l.MaxDropCohortRefs = uint32(maxRefs)
		}
		if l.MaxDropCohortRefs == 0 {
			l.MaxDropCohortRefs = dropCohortRefHardCap
		}
	}
	if l.MaxDropCohortRefBytes == 0 {
		l.MaxDropCohortRefBytes = uint64(l.MaxDropCohortRefs) * uint64(unsafe.Sizeof(DropCohortRef{}))
	}
	if l.MaxDropCohorts == 0 {
		l.MaxDropCohorts = 4096
	}
	if l.MaxDropCohortMembers == 0 {
		l.MaxDropCohortMembers = dropCohortRefHardCap
	}
	if l.MaxDropCohortBytes == 0 {
		// Keep the certificate arena bounded without coupling its derivation
		// budget to the reference spill reserve. The latter is sized for node
		// lineage history and is too small for a complete parser derivation.
		l.MaxDropCohortBytes = 128 << 20
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
	state         StateID
	byteOffset    uint32
	firstLink     uint32
	linkCount     uint32
	pathCount     uint64
	precedenceMax int64
}

// precedenceCandidate is a computed precedence value. It carries no
// verification of its own; the fold rules in precedenceMaximumWitness and
// their C sources (stack.c stack_node_add_link) are the model it comes from.
type precedenceCandidate struct {
	value int64
}

// precedenceMaximumWitness folds C's per-node dynamic_precedence running
// value across one bounded merge transaction. seed carries the mutated
// node's stored value (C's self->dynamic_precedence before the transaction);
// replacement models the C same-pair assignment, which overwrites the
// running value and can lower it; discarded and postReplacement each hold
// the highest max-rule contribution observed before and after the last
// assignment. Publication folds seed, assignment, and contributions in that
// order; it never recomputes from the final link array, because C never
// does.
type precedenceMaximumWitness struct {
	seed               int64
	hasSeed            bool
	discarded          linkRecord
	hasDiscarded       bool
	replacement        linkRecord
	hasReplacement     bool
	postReplacement    linkRecord
	hasPostReplacement bool
}

type nodeLineageRecord struct {
	owner          uint32
	dropCohortRefs DropCohortRefSet
	set            AlternativeSet
	// transition only, deleted at stage 3 cleanup (spec.b4b-alternative-set.v1
	// section 3.2):
	lineage   uint16
	rank      CleanPathRankSelection
	converged bool
	// blended records whether this record's set was ever produced by folding
	// two incomparable recorded sets together (spec.b4b-alternative-set.v2
	// section 3.4). A blended record can never serve as a v2 containment
	// witness (section 5); it may still safely be dropped itself.
	blended bool
}

// Field order groups the two uint32 members (node, owner, setSpillRef)
// before the trailing byte/uint16-sized fields: setSpillRef needs 4-byte
// alignment, so declaring it after the 1-byte setCount/setFlags pair (as an
// earlier revision did) forced 2 bytes of mid-struct padding that this order
// avoids, matching journal-append-site field order 1:1 (every
// nodeLineageJournal append already names every field, so this reorder is
// layout-only and touches no call site).
type nodeLineageMutation struct {
	node           NodeID
	owner          uint32
	dropCohortRefs DropCohortRefSet
	setSpillRef    uint32
	setCount       uint8
	setFlags       uint8
	lineage        uint16
	rank           CleanPathRankSelection
	converged      bool
	blended        bool
}

// alternativeSetInlineCapacity is the fixed inline member width of
// AlternativeSet before it spills into Core.alternativeSpillArena.
// alternativeSetHardCap is the total recorded-member ceiling; a set that
// would exceed it stops recording new members and reports Overflowed.
// spec.b4b-alternative-set.v1 section 3.2, open question 5.
//
// v2 widened one packed member from uint16 (event only) to uint32 (event,
// branch), doubling AlternativeSet's inline array from 8 to 16 bytes and,
// with it, every by-value container that embeds a set (nodeLineageRecord,
// ReductionOutput, and diagnosticParserCoreHeader's two copies) -- paid on
// every header canonicalize double-buffer copy, rollback scratch snapshot,
// and reduction-output propagation, whether or not that particular set is
// ever populated (b4b-width-repair audit, 2026-08). 4 was never a load-
// bearing width: the canonical-fixture census (four fixtures, 8537 converged
// -split elections) shows 8519/8537 (99.8%) already touch a spilled set by
// election time and 8424/8537 (98.7%) are already Overflowed at the hard
// cap, so the inline/spill boundary is nearly vestigial in steady state.
// Lowering it to 2 -- matching "canonical reductions produce one output 95%
// of the time and at most two observed" (packAlternativeSetMember's doc
// comment; census maxBranch=1 on all four fixtures) so a fresh establishment
// plus one sibling union still never spills -- restores AlternativeSet's
// inline array to its pre-v2 8 bytes (2 members x 4 bytes, vs. the original
// 4 members x 2 bytes) at the same total 16-byte struct size, with no change
// to member encoding, hard cap, or any observable membership: spilled
// storage is exact and zero-alloc once warm (alternativeSetInsert/
// alternativeSetMembers already treat inline and spilled uniformly).
const (
	alternativeSetInlineCapacity = 2
	alternativeSetHardCap        = 32
)

const (
	alternativeSetFlagOverflowed uint8 = 1 << iota
	alternativeSetFlagSpilled
)

// alternativeSetBranchCap is the exclusive ceiling on a ReductionOutput's
// establishment ordinal within one dispatch (spec.b4b-alternative-set.v2
// section 3.2). A branch is a uint16, so it has headroom to 65535, but that
// top value stays reserved -- mirroring the lineage-id wraparound decline
// (nextDiagnosticParserCoreCleanPathLineage, parsercore_phase0_driver.go) --
// so establishment can decline before overflow instead of silently wrapping.
// Canonical reductions produce one output 95% of the time and at most two
// observed; this cap is unreachable on the certified corpora and exists only
// as a defensive decline.
const alternativeSetBranchCap = 65535

// errAlternativeSetBranchOrdinalCap is returned by RecordReductionLineageOwned
// when a single dispatch produces alternativeSetBranchCap or more outputs.
var errAlternativeSetBranchOrdinalCap = errors.New("parser-core phase zero: reduction output branch ordinal cap")

// AlternativeSet is a sorted, deduplicated set of (event, branch) converged-
// split-resolution identities ("members"; spec.b4b-alternative-set.v2 section
// 3.1). A member packs a uint16 event (the multi-pop reduce dispatch's
// lineage id, allocated exactly as v1's event-only identity was) and a
// uint16 branch (the ReductionOutput's ordinal within that one dispatch, in
// its stable first-boundary order) into one uint32:
// uint32(event)<<16 | uint32(branch). A member is established once, at
// multi-pop reduction time, and the set only ever grows by insertion or
// union -- it is never invalidated. Beyond alternativeSetInlineCapacity
// members it spills into Core's shared arena; beyond alternativeSetHardCap
// members it stops recording new members and sets Overflowed, so the
// recorded set stays a genuine subset of the true membership. The zero value
// is the empty set. Ascending uint32 order sorts event-major, branch-minor,
// so every branch of one event sits adjacent.
type AlternativeSet struct {
	inline   [alternativeSetInlineCapacity]uint32
	count    uint8
	flags    uint8
	spillRef uint32 // 1-based start index into Core.alternativeSpillArena
}

// packAlternativeSetMember packs one (event, branch) pair into the single
// uint32 member identity (spec.b4b-alternative-set.v2 section 3.1).
func packAlternativeSetMember(event, branch uint16) uint32 {
	return uint32(event)<<16 | uint32(branch)
}

// AlternativeSetMemberEvent unpacks the event half of a packed member.
func AlternativeSetMemberEvent(member uint32) uint16 { return uint16(member >> 16) }

// AlternativeSetMemberBranch unpacks the branch half of a packed member.
func AlternativeSetMemberBranch(member uint32) uint16 { return uint16(member) }

// alternativeSetRecordingEnabledOnce and alternativeSetRecordingEnabledVal
// cache the recording gate. Stage 1 kept recording shadow-only, off by
// default, behind GTS_B4B_SHADOW_CENSUS. Stage 2b promotes the v2
// containment predicate (diagnosticParserCoreConvergedCoverageDropsV2) to
// the deciding proof for uncertified languages, so recording must be
// unconditional: a header whose set was never populated fails closed on
// every drop it is asked to prove, which would silently turn every
// converged-split drop into a decline. Recording is allocation-free by
// construction (spec.b4b-alternative-set.v2 section 3.4), so turning it on
// always costs real CPU, never allocation.
var (
	alternativeSetRecordingEnabledOnce sync.Once
	alternativeSetRecordingEnabledVal  bool
)

func alternativeSetRecordingEnabled() bool {
	alternativeSetRecordingEnabledOnce.Do(func() {
		alternativeSetRecordingEnabledVal = true
	})
	return alternativeSetRecordingEnabledVal
}

// SetAlternativeSetRecordingEnabledForTest overrides the recording gate for
// one test process. Restore the previous value (the returned func) when
// done. Production never disables recording; this exists so a test can
// exercise the disabled-gate code path in isolation. Reads through
// alternativeSetRecordingEnabled first (rather than forcing the sync.Once
// with an empty closure) so the captured "previous" is always the gate's
// real value -- true on its first-ever call in a process -- never an
// artifact of which caller happened to fire the Once first.
func SetAlternativeSetRecordingEnabledForTest(on bool) func() {
	previous := alternativeSetRecordingEnabled()
	alternativeSetRecordingEnabledVal = on
	return func() { alternativeSetRecordingEnabledVal = previous }
}

// cleanPathRankWalkEnabledOnce and cleanPathRankWalkEnabledVal cache
// GTS_B4B_SHADOW_CENSUS, independently of the outer package's own copy of the
// same switch (parsercorephase0 has no dependency on the package that
// defines the census) -- the pattern alternativeSetRecordingEnabled used
// before stage 2b made set recording unconditional. Once the converged-
// coverage v2 predicate became the deciding proof, nothing on the routing
// path reads markCleanProductionRank's scalar (rank, lineage) output except
// the three-proof census's scalar comparison, so the DAG walk itself now
// runs only when that census is requested. Off by default, multi-pop paths
// are marked Unknown instead (markCleanPathRankUnknown): a single field
// write per path, no derivation walk. Unknown still marks
// nodeLineageRecord.converged=true exactly as Selected/Unselected would
// (recordNodeLineage), so HistoricalBoundaryConverged classification and
// alternative-set historical import (spec section 4, "Dead-node historical
// import") are unaffected -- only the now-unread scalar rank/lineage values
// degrade to Unknown/0.
var (
	cleanPathRankWalkEnabledOnce sync.Once
	cleanPathRankWalkEnabledVal  bool
)

func cleanPathRankWalkEnabled() bool {
	cleanPathRankWalkEnabledOnce.Do(func() {
		switch os.Getenv("GTS_B4B_SHADOW_CENSUS") {
		case "1", "true", "TRUE", "True", "on", "ON", "yes", "YES":
			cleanPathRankWalkEnabledVal = true
		}
	})
	return cleanPathRankWalkEnabledVal
}

// SetCleanPathRankWalkEnabledForTest overrides the rank-walk gate for one
// test process, mirroring SetAlternativeSetRecordingEnabledForTest's
// override pattern. Restore the previous value (the returned func) when
// done.
func SetCleanPathRankWalkEnabledForTest(on bool) func() {
	previous := cleanPathRankWalkEnabled()
	cleanPathRankWalkEnabledVal = on
	return func() { cleanPathRankWalkEnabledVal = previous }
}

// NewAlternativeSetMember returns the singleton set {pack(event, branch)},
// or the empty set when event==0 (the reserved "no event" identity) or when
// the recording gate is off (test-only; see
// SetAlternativeSetRecordingEnabledForTest). branch is the ReductionOutput's
// ordinal within the establishing dispatch (spec.b4b-alternative-set.v2
// section 3.1); it is not independently reserved -- only event==0 makes the
// packed value zero, since establishment always guards event!=0 first.
// Every driver-side
// alternative-set value not copied or unioned from an existing header/node
// set originates here, so gating this one construction point -- together
// with recordNodeLineageMember, the only other place a member is ever
// inserted -- keeps every set in the system at its zero value for the life
// of the parse when recording is disabled: every downstream union or insert
// then sees an empty source and no-ops through its own existing zero-cost
// guard, with no further gating needed at any of those call sites. Never
// allocates: a single fresh member always fits inline.
func NewAlternativeSetMember(event, branch uint16) AlternativeSet {
	var set AlternativeSet
	if event == 0 || !alternativeSetRecordingEnabled() {
		return set
	}
	set.inline[0] = packAlternativeSetMember(event, branch)
	set.count = 1
	return set
}

func (s AlternativeSet) spilled() bool { return s.flags&alternativeSetFlagSpilled != 0 }

// Overflowed reports whether this set declined at least one member at the
// hard cap. An overflowed set's recorded members remain a genuine subset of
// the true membership; only completeness is lost, so containment proofs
// against an overflowed dropped-side set must fail closed.
func (s AlternativeSet) Overflowed() bool { return s.flags&alternativeSetFlagOverflowed != 0 }

// Len reports the recorded member count (inline plus spilled), which may be
// less than the true member count when Overflowed.
func (s AlternativeSet) Len() int { return int(s.count) }

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

func (c *Core) nodePrecedenceMaximum(id NodeID) (precedenceCandidate, error) {
	node, err := c.node(id)
	if err != nil {
		return precedenceCandidate{}, err
	}
	return precedenceCandidate{value: node.precedenceMax}, nil
}

func (c *Core) linkPrecedenceMaximum(link linkRecord) (precedenceCandidate, error) {
	predecessor, err := c.nodePrecedenceMaximum(link.prev)
	if err != nil {
		return precedenceCandidate{}, err
	}
	contribution, err := c.effectivePayloadPrecedence(link.payload, link.scoreDelta)
	if err != nil {
		return precedenceCandidate{}, err
	}
	value, err := checkedAddScore(predecessor.value, contribution)
	if err != nil {
		return precedenceCandidate{}, errors.New("parser-core phase zero: precedence maximum overflow")
	}
	return precedenceCandidate{value: value}, nil
}

func (c *Core) observeDiscardedLink(w *precedenceMaximumWitness, link linkRecord) error {
	if w.hasReplacement {
		return c.observePostReplacementLink(w, link)
	}
	candidate, err := c.linkPrecedenceMaximum(link)
	if err != nil {
		return err
	}
	if !w.hasDiscarded {
		w.discarded = link
		w.hasDiscarded = true
		return nil
	}
	previous, err := c.linkPrecedenceMaximum(w.discarded)
	if err != nil {
		return err
	}
	if candidate.value > previous.value {
		w.discarded = link
	}
	return nil
}

func (c *Core) observePostReplacementLink(w *precedenceMaximumWitness, link linkRecord) error {
	candidate, err := c.linkPrecedenceMaximum(link)
	if err != nil {
		return err
	}
	if !w.hasPostReplacement {
		w.postReplacement = link
		w.hasPostReplacement = true
		return nil
	}
	previous, err := c.linkPrecedenceMaximum(w.postReplacement)
	if err != nil {
		return err
	}
	if candidate.value > previous.value {
		w.postReplacement = link
	}
	return nil
}

func (c *Core) discardedPrecedenceMaximum(w precedenceMaximumWitness) (precedenceCandidate, bool, error) {
	if !w.hasDiscarded {
		return precedenceCandidate{}, false, nil
	}
	candidate, err := c.linkPrecedenceMaximum(w.discarded)
	if err != nil {
		return precedenceCandidate{}, false, err
	}
	return candidate, true, nil
}

func (c *Core) computePrecedenceMaximumFromNode(r nodeRecord, folded precedenceMaximumWitness) (precedenceCandidate, error) {
	if folded.hasReplacement {
		return precedenceCandidate{}, errors.New("parser-core phase zero: replacement witness requires final adjacency")
	}
	id := LinkID(r.firstLink)
	var maximum precedenceCandidate
	haveMaximum := false
	for remaining := r.linkCount; remaining > 0; remaining-- {
		if id == 0 || uint64(id) > uint64(len(c.links)) {
			return precedenceCandidate{}, errors.New("parser-core phase zero: adjacency shorter than recorded link count")
		}
		candidate, err := c.linkPrecedenceMaximum(c.links[id-1])
		if err != nil {
			return precedenceCandidate{}, err
		}
		if !haveMaximum || candidate.value > maximum.value {
			maximum = candidate
			haveMaximum = true
		}
		id = c.links[id-1].next
	}
	if id != 0 {
		return precedenceCandidate{}, errors.New("parser-core phase zero: adjacency exceeds recorded link count or cycles")
	}
	if discarded, ok, err := c.discardedPrecedenceMaximum(folded); err != nil {
		return precedenceCandidate{}, err
	} else if ok && (!haveMaximum || discarded.value > maximum.value) {
		maximum = discarded
		haveMaximum = true
	}
	if !haveMaximum {
		return precedenceCandidate{}, errors.New("parser-core phase zero: missing precedence maximum certificate")
	}
	return maximum, nil
}

func (c *Core) computePrecedenceMaximum(links []linkRecord, folded precedenceMaximumWitness) (precedenceCandidate, error) {
	// A seeded witness folds C's stored running value: start from the mutated
	// node's stored maximum, let the last same-pair assignment overwrite it,
	// and max in the contributions observed around that assignment. The final
	// link array plays no part, matching stack_node_add_link, which never
	// recomputes self->dynamic_precedence from the link set.
	if folded.hasSeed {
		maximum := precedenceCandidate{value: folded.seed}
		if folded.hasReplacement {
			replacement, err := c.linkPrecedenceMaximum(folded.replacement)
			if err != nil {
				return precedenceCandidate{}, err
			}
			matched := false
			for _, link := range links {
				if link.prev == folded.replacement.prev && c.linkEdgesEqual(link, folded.replacement) {
					matched = true
					break
				}
			}
			if !matched {
				return precedenceCandidate{}, errors.New("parser-core phase zero: replacement witness is not present in final adjacency")
			}
			maximum = replacement
			if folded.hasPostReplacement {
				postReplacement, err := c.linkPrecedenceMaximum(folded.postReplacement)
				if err != nil {
					return precedenceCandidate{}, err
				}
				if postReplacement.value > maximum.value {
					maximum = postReplacement
				}
			}
			return maximum, nil
		}
		if discarded, ok, err := c.discardedPrecedenceMaximum(folded); err != nil {
			return precedenceCandidate{}, err
		} else if ok && discarded.value > maximum.value {
			maximum = discarded
		}
		return maximum, nil
	}
	var maximum precedenceCandidate
	haveMaximum := false
	for _, link := range links {
		candidate, err := c.linkPrecedenceMaximum(link)
		if err != nil {
			return precedenceCandidate{}, err
		}
		if !haveMaximum || candidate.value > maximum.value {
			maximum = candidate
			haveMaximum = true
		}
	}
	if folded.hasReplacement {
		matched := false
		for _, link := range links {
			if link.prev == folded.replacement.prev && c.linkEdgesEqual(link, folded.replacement) {
				matched = true
				break
			}
		}
		if !matched {
			return precedenceCandidate{}, errors.New("parser-core phase zero: replacement witness is not present in final adjacency")
		}
		replacement, err := c.linkPrecedenceMaximum(folded.replacement)
		if err != nil {
			return precedenceCandidate{}, err
		}
		maximum = replacement
		haveMaximum = true
		if folded.hasPostReplacement {
			postReplacement, err := c.linkPrecedenceMaximum(folded.postReplacement)
			if err != nil {
				return precedenceCandidate{}, err
			}
			if postReplacement.value > maximum.value {
				maximum = postReplacement
			}
		}
	} else if discarded, ok, err := c.discardedPrecedenceMaximum(folded); err != nil {
		return precedenceCandidate{}, err
	} else if ok && (!haveMaximum || discarded.value > maximum.value) {
		maximum = discarded
		haveMaximum = true
	}
	if !haveMaximum {
		return precedenceCandidate{}, errors.New("parser-core phase zero: missing precedence maximum certificate")
	}
	return maximum, nil
}

type subtreeRecord struct {
	symbol            Symbol
	productionID      uint16
	dynamicPrecedence int16
	// externalProvenanceState is derived metadata, not subtree identity. It
	// sits in the padding before startByte, so it costs no record size.
	// Normal publication computes it from child records.
	//
	// THAT PADDING IS NOW FULL. startByte needs four-byte alignment, which
	// reserves exactly two bytes after dynamicPrecedence, and both are taken:
	// externalProvenanceState and missing. The record measures 44 bytes and a
	// test pins that. A third single-byte field here has nowhere free to go,
	// so it rounds every compact subtree in every parse up to 48.
	externalProvenanceState subtreeExternalProvenanceState
	// missing mirrors C's ts_subtree_new_missing_leaf (subtree.c:534-546): a
	// zero-width terminal the parser inserted during recovery because the
	// grammar required it and the input did not supply it. C reads the bit
	// back through ts_subtree_missing, and it is load-bearing for cost, not
	// only for display: ts_subtree_error_cost (subtree.h:331-337) returns
	// ERROR_COST_PER_MISSING_TREE + ERROR_COST_PER_RECOVERY (610) for a
	// missing subtree instead of the stored error cost.
	//
	// The field occupies the SECOND alignment byte before startByte, which
	// startByte's own four-byte alignment already reserved, so the record
	// stays 44 bytes (pinned by core_test.go). Placing it anywhere after
	// startByte would round the record up to 48 and grow every compact
	// subtree in every parse.
	//
	// The compact S5 recovery stage sets this through MissingLeaf after its
	// artifact gate admits the complete recovery competition.
	missing    bool
	startByte  uint32
	endByte    uint32
	firstChild uint32
	childCount uint32
	firstField uint32
	fieldCount uint32
	firstAlias uint32
	aliasCount uint32
	extra      bool
	external   bool
	terminal   bool
	// fragile mirrors gotreesitter.Node's fragileLeft/fragileRight bits
	// (tree.go), collapsed to one conservative flag here: set whenever the
	// record was produced by a reduce/conflict-arm decision that ran under
	// ambiguity (see the conflict executor, parsercore_phase0_driver.go,
	// and Core.Reduce/ReduceOutputs, core.go). It is monotone set-only (only
	// ever flipped false->true), which is safe on shared/deduped records:
	// a record reachable via both a clean and an ambiguous derivation must
	// be treated as fragile, since the ambiguous derivation proves its
	// shape was not uniquely determined. Not yet consumed by any reuse gate
	// in this package -- laid down for a later lane, mirroring
	// gotreesitter's own production fragility metadata (see PHASE-3 LANE 1,
	// tree.go/parser_reduce.go).
	fragile bool
}

// externalPayloadProvenance stores the exact scanner states for one
// authenticated terminal. External-token identity and compact leaf
// reauthentication share this sparse sidecar so subtree records stay small.
// The historical type name remains internal.
type externalPayloadProvenance struct {
	payload SubtreeID
	start   CheckpointID
	end     CheckpointID
}

// lexerSkippedPrefixProvenance stores the exact start of one prefix that the
// internal DFA skipped before it produced a terminal. Sparse storage keeps the
// 44-byte subtree record unchanged for terminals that have no skipped prefix.
type lexerSkippedPrefixProvenance struct {
	payload SubtreeID
	start   uint32
}

type subtreeExternalProvenanceState uint8

const (
	subtreeExternalProvenanceUnknown subtreeExternalProvenanceState = iota
	subtreeExternalProvenanceExactNoExternal
	subtreeExternalProvenanceExactHasExternal
	subtreeExternalProvenanceInexactHasExternal
)

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
	// LexerSkippedPrefixLength is the exact internal-DFA prefix length.
	// Zero means that no bounded proof accompanies this terminal.
	LexerSkippedPrefixLength uint16
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

// CondenseCandidate identifies one active scheduler version that a new action
// output can merge into. The scheduler excludes the source version.
type CondenseCandidate struct {
	Head           Head
	Checkpoint     CheckpointID
	DropCohortRefs DropCohortRefSet
	// ErrorCost is zero for the clean Stage 2 merge route. The production
	// driver preserves a permanent recovery-costed marker and excludes those
	// headers. Recovery versions stay separate until a later stage can compare
	// their complete C costs.
	ErrorCost     uint32
	MergeIdentity uint16
	Shifted       bool
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
	// Missing mirrors subtreeRecord.missing: a recovery-inserted zero-width
	// terminal (C ts_subtree_missing).
	Missing bool
	// LexerSkippedPrefix proves that the internal DFA skipped the byte range
	// [LexerSkippedPrefixStart, StartByte) before it produced this terminal.
	LexerSkippedPrefix      bool
	LexerSkippedPrefixStart uint32
}

// MaterializationSubtreeView is a callback-scoped borrowed view of one compact
// subtree. Children and Aliases reference compact arena storage. A visitor
// must not retain or mutate them. The copying Subtree diagnostic API remains
// the stable inspection surface.
type MaterializationSubtreeView struct {
	Symbol            Symbol
	ProductionID      uint16
	DynamicPrecedence int16
	StartByte         uint32
	EndByte           uint32
	Children          []SubtreeID
	Aliases           []Symbol
	Extra             bool
	External          bool
	Terminal          bool
	// ExternalScannerCheckpointStart and ExternalScannerCheckpointEnd identify
	// the serialized scanner states before and after this terminal. They are
	// zero when the subtree has no authenticated scanner provenance. The
	// materializer copies the bytes into its own arena.
	ExternalScannerCheckpointStart CheckpointID
	ExternalScannerCheckpointEnd   CheckpointID
	ExternalScannerCheckpointExact bool
	// Fragile mirrors subtreeRecord.fragile: the record was produced by a
	// reduce/conflict-arm decision that ran under ambiguity (multiPop or an
	// authenticated >=2-action conflict). Threaded to the public materializer so
	// Lane-1 fragility metadata reaches compact-materialized public nodes.
	Fragile bool
	// Missing mirrors subtreeRecord.missing: a recovery-inserted zero-width
	// terminal (C ts_subtree_missing). Threaded to the public materializer so
	// it can set the public node's own missing and has-error bits.
	Missing bool
	// LexerSkippedPrefix is exact terminal provenance from the internal DFA.
	// It is false for reductions, external tokens, and synthetic terminals.
	LexerSkippedPrefix      bool
	LexerSkippedPrefixStart uint32
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
	PhysicalHeadMergeAttempts              uint64
	PhysicalHeadMergeSuccesses             uint64
	PhysicalHeadMergeInputLinks            uint64
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
	tables             TableView
	tableIdentity      [32]byte
	tableIdentityValid bool
	plans              ReductionPlanProvider
	selectedProvider   SelectedStorePolicyProvider
	selectedPolicy     *SelectedStorePolicy
	limits             Limits
	diagnostics        diagnosticOptions
	nodes              []nodeRecord
	nodeLineages       []nodeLineageRecord
	nodeCheckpoints    []CheckpointID
	links              []linkRecord

	// dropCohortLinkRefIndexes is an optional, LinkID-indexed sidecar. A zero
	// entry means that no finalized drop-cohort reference is bound.
	dropCohortLinkRefIndexes []uint32
	dropCohortLinkRefJournal []dropCohortLinkRefMutation

	subtrees             []subtreeRecord
	externalProvenance   []externalPayloadProvenance
	lexerSkippedPrefixes []lexerSkippedPrefixProvenance
	children             []SubtreeID
	fields               []FieldMapEntry
	aliases              []Symbol
	// eofRecoveryRoots records synthetic non-extra ERROR roots published by
	// RecoverEOFAcceptOwned. Keep this provenance outside subtreeRecord: that
	// record is size-pinned at 44 bytes for every compact payload.
	eofRecoveryRoots   []SubtreeID
	frontier           uint64
	checkpoint         CheckpointID
	checkpoints        checkpointInterner
	boundaries         boundaryIndex
	boundaryJournal    []boundaryMutation
	nodeLineageJournal []nodeLineageMutation
	// alternativeSpillArena backs every AlternativeSet beyond
	// alternativeSetInlineCapacity members, shared by nodeLineages and every
	// diagnosticParserCoreHeader.altSet (parsercore_phase0_driver.go). It
	// resets to length zero (capacity retained) only on Reset, matching
	// nodeLineageJournal; a rolled-back or superseded segment leaks arena
	// space until then, bounded by alternativeSetHardCap per record.
	alternativeSpillArena          []uint32
	dropCohortRefSpill             []DropCohortRef
	dropCohortActions              []dropCohortActionIdentity
	dropCohortRecords              []dropCohortRecord
	dropCohortMembers              []dropCohortMember
	dropCohortDerivations          []dropCohortDerivationRecord
	dropCohortDerivationIntern     []dropCohortDerivationInternEntry
	dropCohortDerivationBytes      []byte
	dropCohortCertificateRefs      []DropCohortRef
	dropCohortMapStore             []dropCohortMapEntry
	dropCohortJournalStore         []dropCohortJournalStoreEntry
	dropCohortFrontiers            []dropCohortFrontierRecord
	dropCohortFrontierParticipants []dropCohortFrontierParticipant
	dropCohortFrontierMembers      []dropCohortFrontierMember
	dropCohortFrontierJournal      []dropCohortFrontierMutation
	dropCohortDerivationScratch    []byte
	dropCohortPathScratch          []dropCohortPathStep
	dropCohortEphemeralBytes       uint64
	dropCohortEphemeralPeak        uint64
	dropCohortJournal              []dropCohortMutation
	dropCohortOwner                uint64
	dropCohortEpoch                uint64
	dropCohortNextSequence         uint64
	dropCohortFrontierNextSequence uint64
	dropCohortProducerWrites       [dropCohortProducerCount]uint64
	dropCohortAuthenticatedHistory uint64
	dropCohortUnprovedHistory      uint64
	dropCohortOwnerCheckedLookups  uint64
	dropCohortVerifierElections    uint64
	dropCohortVerifierProofs       uint64
	dropCohortVerifierDeclines     uint64
	dropCohortActionDeclines       uint64
	dropCohortDerivationDeclines   uint64
	dropCohortDeclineReasons       [dropCohortVerifierReasonCount]uint64
	dropCohortInlineReads          uint64
	dropCohortSpillReads           uint64
	dropCohortMapReads             uint64
	dropCohortInternerReads        uint64
	dropCohortReservations         []dropCohortReservation
	dropCohortReserved             [7]uint64
	dropCohortReservedBytes        uint64
	condenseCandidates             []CondenseCandidate
	condenseNewNode                NodeID
	condenseScopeActive            bool
	reductionSourceOwner           uint32
	transactions                   []uint64
	nextTransaction                uint64
	// resetGeneration identifies the retained arena lifetime. It changes only
	// when Reset clears the core, so phase checkpoints can advance independently.
	resetGeneration       uint64
	classificationPhase   uint64
	work                  Work
	popScratch            popEnumerationScratch
	reductionScratch      reductionOutputScratch
	historicalNodeScratch []NodeID
	cohortHeadScratch     []Head
	factorLinkScratch     []linkRecord
	selectedBuild         selectedStoreBuildScratch
	selectedPoolMu        sync.Mutex
	selectedPool          selectedStoreBacking
	schedulerFrame        schedulerTransactionFrame
	// metadataConstructionAuthenticated remains true only while every compact
	// subtree was published through the authenticated shift/reduction seams.
	// Diagnostic generic publication clears it monotonically until Reset.
	metadataConstructionAuthenticated bool
	// reduceConflictContext is a transient, non-sticky fragility signal: the
	// conflict executor (executeDiagnosticParserCoreGenericConflictDetailed,
	// parsercore_phase0_driver.go) sets it for the duration of applying every
	// arm of a >=2-action conflict (both the fork.Present secondaries and the
	// fork.Present==false primary), mirroring how markReduceFragility's
	// action-conflict trigger works in the production Go parser
	// (parser_reduce.go). Combined with the local len(paths) > 1 (multi-pop)
	// signal in reduceOutputsClassifiedIntoActive, this feeds subtreeRecord's
	// fragile bit. Core is used through a single-owner
	// (SchedulerTransactionToken) access-control model, not concurrently,
	// so a plain field is safe here -- see Parser.reduceActionConflict
	// (parser.go) for the identical pattern and its safety argument.
	reduceConflictContext bool
	// reduceNoLookaheadContext marks a reduction triggered by the parser's
	// synthetic EOF token. A transparent goto completes an extra in place.
	// The scheduler sets this only around one authenticated reduction.
	reduceNoLookaheadContext bool
	// dropCohortSelectionContext carries the authenticated scheduler selection
	// class into reduction-owned historical certificate checks.
	dropCohortSelectionContext DropCohortSelectionClass
	// historicalCertificateAuthentication keeps D2 certificate checks inert
	// until an explicit producer fixture enables them.
	historicalCertificateAuthentication bool
	// externalPayloadsQuiescent is a one-way language capability. The caller
	// certifies that external payload identity does not depend on scanner state.
	// Reset retains it because the property is stable for this core's tables.
	externalPayloadsQuiescent bool
	// terminalScannerCheckpointProvenance is a one-way language capability.
	// It records the scanner pair for every shifted terminal, including a DFA
	// terminal. Compact materialization needs both states to reauthenticate an
	// edited leaf without trusting a fresh scanner payload.
	terminalScannerCheckpointProvenance bool
	// externalTokenScannerStart and externalTokenScannerEnd authenticate one
	// elected token. A checkpoint-capable language copies this pair into each
	// immutable terminal payload.
	externalTokenScannerStart CheckpointID
	externalTokenScannerEnd   CheckpointID
	externalTokenScannerExact bool
}

// SetReduceConflictContext sets/clears the transient conflict-context
// fragility signal; see the reduceConflictContext field doc comment. Callers
// outside this package (the conflict executor) must always pair a true set
// with a deferred false reset, even on early-return error paths.
func (c *Core) SetReduceConflictContext(v bool) {
	if c == nil {
		return
	}
	c.reduceConflictContext = v
}

// SetReduceNoLookaheadContext sets or clears synthetic-EOF reduction context.
// Callers must pair a true set with a deferred false reset.
func (c *Core) SetReduceNoLookaheadContext(v bool) {
	if c == nil {
		return
	}
	c.reduceNoLookaheadContext = v
}

// SetDropCohortSelectionContextOwned sets the transient selection class under
// the active scheduler owner used by exact certificate authentication.
func (c *Core) SetDropCohortSelectionContextOwned(owner SchedulerTransactionToken, v DropCohortSelectionClass) error {
	if c == nil {
		return errors.New("parser-core phase zero: nil core selection context")
	}
	if err := c.validateSchedulerTransaction(owner); err != nil {
		return err
	}
	c.dropCohortSelectionContext = v
	return nil
}

// CertifyExternalPayloadsQuiescent permits recursive insertion to compare and
// retain external payloads. Call this only when scanner state cannot change
// the identity of any external payload produced by this core's language.
//
// The certificate is permanent for the Core. Reset retains it because Reset
// also retains the authenticated language tables.
func (c *Core) CertifyExternalPayloadsQuiescent() {
	if c == nil {
		return
	}
	c.externalPayloadsQuiescent = true
}

// EnableTerminalScannerCheckpointProvenance records the exact scanner pair on
// every authenticated terminal. Call this only for a language that declares
// complete external-scanner checkpoints.
//
// The capability is permanent for the Core. Reset retains it because Reset
// also retains the authenticated language tables.
func (c *Core) EnableTerminalScannerCheckpointProvenance() {
	if c == nil {
		return
	}
	c.terminalScannerCheckpointProvenance = true
}

// inlineAdjacencyCapacity covers the production default without forcing a
// fixed fan-out contract on callers that deliberately configure a wider
// boundary. nodeLinksInto spills to a bounded slice only above this width.
const inlineAdjacencyCapacity = 8

type diagnosticOptions struct {
	foldSamePredecessorShallowPayloads bool
}

type checkpoint struct {
	nodes, nodeLineages, nodeCheckpoints, links, subtrees, externalProvenance int
	lexerSkippedPrefixes                                                      int
	children, fields, aliases                                                 int
	eofRecoveryRoots                                                          int
	dropCohortLinkRefIndexes                                                  int
	dropCohortLinkRefJournal                                                  int
	frontier                                                                  uint64
	checkpoint                                                                CheckpointID
	boundaryIndex                                                             boundaryIndexSnapshot
	journal                                                                   int
	nodeLineageJournal                                                        int
	dropCohortRefSpill                                                        int
	dropCohortActions                                                         int
	dropCohortRecords                                                         int
	dropCohortMembers                                                         int
	dropCohortDerivations                                                     int
	dropCohortDerivationIntern                                                int
	dropCohortDerivationBytes                                                 int
	dropCohortCertificateRefs                                                 int
	dropCohortMapStore                                                        int
	dropCohortJournalStore                                                    int
	dropCohortFrontiers                                                       int
	dropCohortFrontierParticipants                                            int
	dropCohortFrontierMembers                                                 int
	dropCohortFrontierJournal                                                 int
	dropCohortEphemeralBytes                                                  uint64
	dropCohortJournal                                                         int
	dropCohortNextSequence                                                    uint64
	dropCohortFrontierNextSequence                                            uint64
	dropCohortProducerWrites                                                  [dropCohortProducerCount]uint64
	dropCohortAuthenticatedHistory                                            uint64
	dropCohortUnprovedHistory                                                 uint64
	dropCohortOwnerCheckedLookups                                             uint64
	dropCohortVerifierElections                                               uint64
	dropCohortVerifierProofs                                                  uint64
	dropCohortVerifierDeclines                                                uint64
	dropCohortActionDeclines                                                  uint64
	dropCohortDerivationDeclines                                              uint64
	dropCohortDeclineReasons                                                  [dropCohortVerifierReasonCount]uint64
	dropCohortInlineReads                                                     uint64
	dropCohortSpillReads                                                      uint64
	dropCohortMapReads                                                        uint64
	dropCohortInternerReads                                                   uint64
	dropCohortReservations                                                    int
	dropCohortReserved                                                        [7]uint64
	dropCohortReservedBytes                                                   uint64
	dropCohortRefSpillCap                                                     int
	dropCohortActionsCap                                                      int
	dropCohortRecordsCap                                                      int
	dropCohortMembersCap                                                      int
	dropCohortDerivationsCap                                                  int
	dropCohortDerivationInternCap                                             int
	dropCohortDerivationBytesCap                                              int
	dropCohortCertificateRefsCap                                              int
	dropCohortMapStoreCap                                                     int
	dropCohortJournalStoreCap                                                 int
	dropCohortJournalCap                                                      int
	dropCohortFrontierJournalCap                                              int
	dropCohortReservationsCap                                                 int
	dropCohortLinkRefIndexesHeader                                            []uint32
	dropCohortLinkRefJournalHeader                                            []dropCohortLinkRefMutation
	dropCohortRefSpillHeader                                                  []DropCohortRef
	dropCohortActionsHeader                                                   []dropCohortActionIdentity
	dropCohortRecordsHeader                                                   []dropCohortRecord
	dropCohortMembersHeader                                                   []dropCohortMember
	dropCohortDerivationsHeader                                               []dropCohortDerivationRecord
	dropCohortDerivationInternHeader                                          []dropCohortDerivationInternEntry
	dropCohortDerivationBytesHeader                                           []byte
	dropCohortCertificateRefsHeader                                           []DropCohortRef
	dropCohortMapStoreHeader                                                  []dropCohortMapEntry
	dropCohortJournalStoreHeader                                              []dropCohortJournalStoreEntry
	dropCohortFrontiersHeader                                                 []dropCohortFrontierRecord
	dropCohortFrontierParticipantsHeader                                      []dropCohortFrontierParticipant
	dropCohortFrontierMembersHeader                                           []dropCohortFrontierMember
	dropCohortFrontierJournalHeader                                           []dropCohortFrontierMutation
	dropCohortJournalHeader                                                   []dropCohortMutation
	dropCohortReservationsHeader                                              []dropCohortReservation
	transaction                                                               uint64
	work                                                                      Work
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
		nodes: len(c.nodes), nodeLineages: len(c.nodeLineages),
		links: len(c.links), subtrees: len(c.subtrees),
		nodeCheckpoints:      len(c.nodeCheckpoints),
		externalProvenance:   len(c.externalProvenance),
		lexerSkippedPrefixes: len(c.lexerSkippedPrefixes),
		children:             len(c.children), fields: len(c.fields), aliases: len(c.aliases),
		eofRecoveryRoots: len(c.eofRecoveryRoots),

		dropCohortLinkRefIndexes: len(c.dropCohortLinkRefIndexes),
		dropCohortLinkRefJournal: len(c.dropCohortLinkRefJournal),

		frontier: c.frontier, checkpoint: c.checkpoint,
		boundaryIndex:                        c.boundaries.snapshot(),
		journal:                              len(c.boundaryJournal),
		nodeLineageJournal:                   len(c.nodeLineageJournal),
		dropCohortRefSpill:                   len(c.dropCohortRefSpill),
		dropCohortActions:                    len(c.dropCohortActions),
		dropCohortRecords:                    len(c.dropCohortRecords),
		dropCohortMembers:                    len(c.dropCohortMembers),
		dropCohortDerivations:                len(c.dropCohortDerivations),
		dropCohortDerivationIntern:           len(c.dropCohortDerivationIntern),
		dropCohortDerivationBytes:            len(c.dropCohortDerivationBytes),
		dropCohortCertificateRefs:            len(c.dropCohortCertificateRefs),
		dropCohortMapStore:                   len(c.dropCohortMapStore),
		dropCohortJournalStore:               len(c.dropCohortJournalStore),
		dropCohortFrontiers:                  len(c.dropCohortFrontiers),
		dropCohortFrontierParticipants:       len(c.dropCohortFrontierParticipants),
		dropCohortFrontierMembers:            len(c.dropCohortFrontierMembers),
		dropCohortFrontierJournal:            len(c.dropCohortFrontierJournal),
		dropCohortEphemeralBytes:             c.dropCohortEphemeralBytes,
		dropCohortJournal:                    len(c.dropCohortJournal),
		dropCohortNextSequence:               c.dropCohortNextSequence,
		dropCohortFrontierNextSequence:       c.dropCohortFrontierNextSequence,
		dropCohortProducerWrites:             c.dropCohortProducerWrites,
		dropCohortAuthenticatedHistory:       c.dropCohortAuthenticatedHistory,
		dropCohortUnprovedHistory:            c.dropCohortUnprovedHistory,
		dropCohortOwnerCheckedLookups:        c.dropCohortOwnerCheckedLookups,
		dropCohortVerifierElections:          c.dropCohortVerifierElections,
		dropCohortVerifierProofs:             c.dropCohortVerifierProofs,
		dropCohortVerifierDeclines:           c.dropCohortVerifierDeclines,
		dropCohortActionDeclines:             c.dropCohortActionDeclines,
		dropCohortDerivationDeclines:         c.dropCohortDerivationDeclines,
		dropCohortDeclineReasons:             c.dropCohortDeclineReasons,
		dropCohortInlineReads:                c.dropCohortInlineReads,
		dropCohortSpillReads:                 c.dropCohortSpillReads,
		dropCohortMapReads:                   c.dropCohortMapReads,
		dropCohortInternerReads:              c.dropCohortInternerReads,
		dropCohortReservations:               len(c.dropCohortReservations),
		dropCohortReserved:                   c.dropCohortReserved,
		dropCohortReservedBytes:              c.dropCohortReservedBytes,
		dropCohortRefSpillCap:                cap(c.dropCohortRefSpill),
		dropCohortActionsCap:                 cap(c.dropCohortActions),
		dropCohortRecordsCap:                 cap(c.dropCohortRecords),
		dropCohortMembersCap:                 cap(c.dropCohortMembers),
		dropCohortDerivationsCap:             cap(c.dropCohortDerivations),
		dropCohortDerivationInternCap:        cap(c.dropCohortDerivationIntern),
		dropCohortDerivationBytesCap:         cap(c.dropCohortDerivationBytes),
		dropCohortCertificateRefsCap:         cap(c.dropCohortCertificateRefs),
		dropCohortMapStoreCap:                cap(c.dropCohortMapStore),
		dropCohortJournalStoreCap:            cap(c.dropCohortJournalStore),
		dropCohortJournalCap:                 cap(c.dropCohortJournal),
		dropCohortFrontierJournalCap:         cap(c.dropCohortFrontierJournal),
		dropCohortReservationsCap:            cap(c.dropCohortReservations),
		dropCohortLinkRefIndexesHeader:       c.dropCohortLinkRefIndexes,
		dropCohortLinkRefJournalHeader:       c.dropCohortLinkRefJournal,
		dropCohortRefSpillHeader:             c.dropCohortRefSpill,
		dropCohortActionsHeader:              c.dropCohortActions,
		dropCohortRecordsHeader:              c.dropCohortRecords,
		dropCohortMembersHeader:              c.dropCohortMembers,
		dropCohortDerivationsHeader:          c.dropCohortDerivations,
		dropCohortDerivationInternHeader:     c.dropCohortDerivationIntern,
		dropCohortDerivationBytesHeader:      c.dropCohortDerivationBytes,
		dropCohortCertificateRefsHeader:      c.dropCohortCertificateRefs,
		dropCohortMapStoreHeader:             c.dropCohortMapStore,
		dropCohortJournalStoreHeader:         c.dropCohortJournalStore,
		dropCohortFrontiersHeader:            c.dropCohortFrontiers,
		dropCohortFrontierParticipantsHeader: c.dropCohortFrontierParticipants,
		dropCohortFrontierMembersHeader:      c.dropCohortFrontierMembers,
		dropCohortFrontierJournalHeader:      c.dropCohortFrontierJournal,
		dropCohortJournalHeader:              c.dropCohortJournal,
		dropCohortReservationsHeader:         c.dropCohortReservations,
		transaction:                          c.nextTransaction,
		work:                                 c.work,
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
	c.nodeLineages = c.nodeLineages[:mark.nodeLineages]
	c.nodeCheckpoints = c.nodeCheckpoints[:mark.nodeCheckpoints]
	c.links = c.links[:mark.links]
	c.subtrees = c.subtrees[:mark.subtrees]
	c.eofRecoveryRoots = c.eofRecoveryRoots[:mark.eofRecoveryRoots]
	c.externalProvenance = c.externalProvenance[:mark.externalProvenance]
	c.lexerSkippedPrefixes = c.lexerSkippedPrefixes[:mark.lexerSkippedPrefixes]
	c.children = c.children[:mark.children]
	c.fields = c.fields[:mark.fields]
	c.aliases = c.aliases[:mark.aliases]
	c.frontier = mark.frontier
	c.checkpoint = mark.checkpoint
	c.work = mark.work
	for index := len(c.dropCohortLinkRefJournal) - 1; index >= mark.dropCohortLinkRefJournal; index-- {
		mutation := c.dropCohortLinkRefJournal[index]
		if mutation.index >= 0 && mutation.index < len(c.dropCohortLinkRefIndexes) {
			c.dropCohortLinkRefIndexes[mutation.index] = mutation.previous
		}
	}
	c.dropCohortLinkRefIndexes = mark.dropCohortLinkRefIndexesHeader
	for index := len(c.boundaryJournal) - 1; index >= mark.journal; index-- {
		mutation := c.boundaryJournal[index]
		mutation.slots[mutation.index] = mutation.previous
	}
	for index := len(c.nodeLineageJournal) - 1; index >= mark.nodeLineageJournal; index-- {
		mutation := c.nodeLineageJournal[index]
		nodeIndex := int(mutation.node) - 1
		if nodeIndex < 0 || nodeIndex >= len(c.nodes) {
			continue
		}
		c.nodeLineages[nodeIndex].owner = mutation.owner
		c.nodeLineages[nodeIndex].dropCohortRefs = mutation.dropCohortRefs
		c.nodeLineages[nodeIndex].set.count = mutation.setCount
		c.nodeLineages[nodeIndex].set.flags = mutation.setFlags
		c.nodeLineages[nodeIndex].set.spillRef = mutation.setSpillRef
		c.nodeLineages[nodeIndex].lineage = mutation.lineage
		c.nodeLineages[nodeIndex].rank = mutation.rank
		c.nodeLineages[nodeIndex].converged = mutation.converged
		c.nodeLineages[nodeIndex].blended = mutation.blended
	}
	c.boundaries.restore(mark.boundaryIndex)
	c.dropCohortRefSpill = mark.dropCohortRefSpillHeader
	for index := len(c.dropCohortJournal) - 1; index >= mark.dropCohortJournal; index-- {
		mutation := c.dropCohortJournal[index]
		if mutation.index >= 0 && mutation.index < len(c.dropCohortRecords) {
			c.dropCohortRecords[mutation.index] = mutation.before
		}
	}
	for index := len(c.dropCohortFrontierJournal) - 1; index >= mark.dropCohortFrontierJournal; index-- {
		mutation := c.dropCohortFrontierJournal[index]
		if uint64(mutation.index) < uint64(len(c.dropCohortFrontiers)) {
			c.dropCohortFrontiers[mutation.index].state = mutation.before
		}
	}
	c.dropCohortActions = mark.dropCohortActionsHeader
	c.dropCohortRecords = mark.dropCohortRecordsHeader
	c.dropCohortMembers = mark.dropCohortMembersHeader
	c.dropCohortDerivations = mark.dropCohortDerivationsHeader
	c.dropCohortDerivationIntern = mark.dropCohortDerivationInternHeader
	c.dropCohortDerivationBytes = mark.dropCohortDerivationBytesHeader
	c.dropCohortCertificateRefs = mark.dropCohortCertificateRefsHeader
	c.dropCohortMapStore = mark.dropCohortMapStoreHeader
	c.dropCohortJournalStore = mark.dropCohortJournalStoreHeader
	c.dropCohortFrontiers = mark.dropCohortFrontiersHeader
	c.dropCohortFrontierParticipants = mark.dropCohortFrontierParticipantsHeader
	c.dropCohortFrontierMembers = mark.dropCohortFrontierMembersHeader
	c.dropCohortFrontierJournal = mark.dropCohortFrontierJournalHeader
	c.dropCohortDerivationScratch = c.dropCohortDerivationScratch[:0]
	c.dropCohortPathScratch = c.dropCohortPathScratch[:0]
	c.dropCohortEphemeralBytes = mark.dropCohortEphemeralBytes
	c.dropCohortJournal = mark.dropCohortJournalHeader
	c.dropCohortNextSequence = mark.dropCohortNextSequence
	c.dropCohortFrontierNextSequence = mark.dropCohortFrontierNextSequence
	c.dropCohortProducerWrites = mark.dropCohortProducerWrites
	c.dropCohortAuthenticatedHistory = mark.dropCohortAuthenticatedHistory
	c.dropCohortUnprovedHistory = mark.dropCohortUnprovedHistory
	c.dropCohortOwnerCheckedLookups = mark.dropCohortOwnerCheckedLookups
	c.dropCohortVerifierElections = mark.dropCohortVerifierElections
	c.dropCohortVerifierProofs = mark.dropCohortVerifierProofs
	c.dropCohortVerifierDeclines = mark.dropCohortVerifierDeclines
	c.dropCohortActionDeclines = mark.dropCohortActionDeclines
	c.dropCohortDerivationDeclines = mark.dropCohortDerivationDeclines
	c.dropCohortDeclineReasons = mark.dropCohortDeclineReasons
	c.dropCohortInlineReads = mark.dropCohortInlineReads
	c.dropCohortSpillReads = mark.dropCohortSpillReads
	c.dropCohortMapReads = mark.dropCohortMapReads
	c.dropCohortInternerReads = mark.dropCohortInternerReads
	c.dropCohortReservations = mark.dropCohortReservationsHeader
	c.dropCohortReserved = mark.dropCohortReserved
	c.dropCohortReservedBytes = mark.dropCohortReservedBytes
	clear(c.boundaryJournal[mark.journal:])
	c.boundaryJournal = c.boundaryJournal[:mark.journal]
	clear(c.nodeLineageJournal[mark.nodeLineageJournal:])
	c.nodeLineageJournal = c.nodeLineageJournal[:mark.nodeLineageJournal]
	c.dropCohortLinkRefJournal = mark.dropCohortLinkRefJournalHeader
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
		clear(c.nodeLineageJournal)
		c.nodeLineageJournal = c.nodeLineageJournal[:0]
		clear(c.dropCohortLinkRefJournal)
		c.dropCohortLinkRefJournal = c.dropCohortLinkRefJournal[:0]
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

// SetHistoricalCertificateAuthenticationOwned arms the D2 import checks for
// the current scheduler session. Require the active owner for every change.
func (c *Core) SetHistoricalCertificateAuthenticationOwned(token SchedulerTransactionToken, enabled bool) error {
	if c == nil {
		return errors.New("parser-core phase zero: historical authentication on nil core")
	}
	if err := c.validateSchedulerTransaction(token); err != nil {
		return err
	}
	c.historicalCertificateAuthentication = enabled
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
	// A fresh scheduler session receives a new certificate epoch, even when
	// the session later commits without calling Reset.
	if err := c.advanceDropCohortEpoch(); err != nil {
		return err
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
		// Historical certificate authentication is scoped to this fresh session.
		// Clear the policy before invalidating the owner frame on every exit.
		c.historicalCertificateAuthentication = false
		if recovered != nil {
			frame.clearInactive()
			_ = c.ResetReleasingRetention()
			panic(recovered)
		}
		if err == nil && frame.poisoned != nil {
			err = fmt.Errorf("parser-core phase zero: poisoned fresh scheduler session: %w", frame.poisoned)
		}
		frame.clearInactive()
		if err != nil {
			// ResetReleasingRetention, not plain Reset: a declined session
			// leaves no future value in whatever capacity this run grew, and
			// this is the automatic reset every stop-control trip and every
			// mid-run hard decline (recovery, extra-chain, cap hits, and
			// friends) already goes through, so it is also the natural place
			// to release retention on the class of decline this covers
			// (tranche B9 retention-cap gate).
			if resetErr := c.ResetReleasingRetention(); resetErr != nil {
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
	if err = c.beginSchedulerOwned(token); err != nil {
		return err
	}
	defer c.recoverSchedulerOwnedPanic(token)
	return c.finishSchedulerOwned(token, fn())
}

// beginSchedulerOwned validates token ownership and poisons the owner on
// failure. It is the entry half of RunSchedulerOwned, factored out so the hot
// shift/reduce/cohort dispatch paths in scheduler_owned.go can call it
// directly instead of threading a fn func() error parameter through an extra
// wrapper layer.
func (c *Core) beginSchedulerOwned(token SchedulerTransactionToken) error {
	if err := c.validateSchedulerTransaction(token); err != nil {
		err = c.poisonSchedulerTransaction(token, err)
		phase0AObserveSchedulerPoison(c, token, Phase0APoisonReturnedError)
		return err
	}
	return nil
}

// finishSchedulerOwned poisons the owner on a non-nil error, mirroring the
// exit half of RunSchedulerOwned. It returns the (possibly rewritten) error
// so callers can fold it directly into a return statement.
func (c *Core) finishSchedulerOwned(token SchedulerTransactionToken, err error) error {
	if err != nil {
		err = c.poisonSchedulerTransaction(token, err)
		phase0AObserveSchedulerPoison(c, token, Phase0APoisonReturnedError)
	}
	return err
}

// recoverSchedulerOwnedPanic mirrors RunSchedulerOwned's deferred panic
// handler. Deferring this bound method directly -- instead of a deferred
// closure literal that calls back into RunSchedulerOwned -- keeps every hot
// dispatch call a direct, inlinable call with no func-value indirection.
func (c *Core) recoverSchedulerOwnedPanic(token SchedulerTransactionToken) {
	if recovered := recover(); recovered != nil {
		c.poisonSchedulerTransaction(token, fmt.Errorf("parser-core phase zero: scheduler-owned operation panicked: %v", recovered))
		phase0AObserveSchedulerPoison(c, token, Phase0APoisonPanic)
		panic(recovered)
	}
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
	owner, err := allocateDropCohortOwner()
	if err != nil {
		return nil, err
	}
	limits = limits.withDefaults()
	boundaries, err := newBoundaryIndex(limits.MaxNodes)
	if err != nil {
		return nil, err
	}
	core := &Core{
		tables: tables, limits: limits, frontier: 1, boundaries: boundaries,
		checkpoints:                       newCheckpointInterner(limits.MaxCheckpoints, limits.MaxCheckpointBytes),
		resetGeneration:                   1,
		classificationPhase:               1,
		dropCohortOwner:                   owner,
		dropCohortEpoch:                   1,
		diagnostics:                       diagnosticOptions{foldSamePredecessorShallowPayloads: true},
		metadataConstructionAuthenticated: true,
	}
	if provider, ok := tables.(TableIdentityProvider); ok {
		core.tableIdentity, core.tableIdentityValid = provider.TableIdentity()
	}
	core.plans, _ = tables.(ReductionPlanProvider)
	core.selectedProvider, _ = tables.(SelectedStorePolicyProvider)
	return core, nil
}

// TableIdentityMatches reports whether the table producer still matches the
// identity captured when the compact core was created. Missing identity fails
// closed because parser-state replay needs exact table provenance.
func (c *Core) TableIdentityMatches() bool {
	if c == nil || !c.tableIdentityValid {
		return false
	}
	provider, ok := c.tables.(TableIdentityProvider)
	if !ok {
		return false
	}
	identity, valid := provider.TableIdentity()
	return valid && identity == c.tableIdentity
}

// AuthenticationGeneration identifies the current Core capability phase.
// It changes before an operation can reuse an invalidated arena handle.
func (c *Core) AuthenticationGeneration() uint64 {
	if c == nil {
		return 0
	}
	return c.classificationPhase
}

// PhaseScannerCheckpoints returns the current scanner checkpoint binding.
// The exact flag is true only when start and end authenticate one token.
func (c *Core) PhaseScannerCheckpoints() (checkpoint, start, end CheckpointID, exact bool) {
	if c == nil {
		return 0, 0, 0, false
	}
	return c.checkpoint, c.externalTokenScannerStart, c.externalTokenScannerEnd, c.externalTokenScannerExact
}

// ResetGeneration identifies the current retained Core arena lifetime. It
// changes only when Reset clears the arena, not when a frontier or checkpoint
// advances its authentication phase.
func (c *Core) ResetGeneration() uint64 {
	if c == nil {
		return 0
	}
	return c.resetGeneration
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
	if c.resetGeneration == math.MaxUint64 {
		return errors.New("parser-core phase zero: reset generation overflow")
	}
	if c.dropCohortEpoch == math.MaxUint64 {
		return errors.New("parser-core phase zero: drop-cohort epoch overflow")
	}
	if err := phase0AInvalidateCore(c); err != nil {
		return err
	}
	c.classificationPhase++
	c.resetGeneration++
	if err := c.advanceDropCohortEpoch(); err != nil {
		return err
	}
	c.nodes = c.nodes[:0]
	c.nodeLineages = c.nodeLineages[:0]
	c.nodeCheckpoints = c.nodeCheckpoints[:0]
	c.links = c.links[:0]
	clear(c.dropCohortLinkRefIndexes)
	c.dropCohortLinkRefIndexes = c.dropCohortLinkRefIndexes[:0]
	c.subtrees = c.subtrees[:0]
	c.eofRecoveryRoots = c.eofRecoveryRoots[:0]
	c.externalProvenance = c.externalProvenance[:0]
	c.lexerSkippedPrefixes = c.lexerSkippedPrefixes[:0]
	c.children = c.children[:0]
	c.fields = c.fields[:0]
	c.aliases = c.aliases[:0]
	c.frontier = 1
	c.checkpoint = 0
	c.checkpoints.reset()
	c.boundaries.reset()
	clear(c.boundaryJournal)
	c.boundaryJournal = c.boundaryJournal[:0]
	clear(c.nodeLineageJournal)
	c.nodeLineageJournal = c.nodeLineageJournal[:0]
	clear(c.dropCohortLinkRefJournal)
	c.dropCohortLinkRefJournal = c.dropCohortLinkRefJournal[:0]
	c.alternativeSpillArena = c.alternativeSpillArena[:0]
	c.dropCohortRefSpill = c.dropCohortRefSpill[:0]
	c.dropCohortActions = c.dropCohortActions[:0]
	c.dropCohortRecords = c.dropCohortRecords[:0]
	c.dropCohortMembers = c.dropCohortMembers[:0]
	c.dropCohortDerivations = c.dropCohortDerivations[:0]
	c.dropCohortDerivationIntern = c.dropCohortDerivationIntern[:0]
	c.dropCohortDerivationBytes = c.dropCohortDerivationBytes[:0]
	c.dropCohortCertificateRefs = c.dropCohortCertificateRefs[:0]
	c.dropCohortMapStore = c.dropCohortMapStore[:0]
	c.dropCohortJournalStore = c.dropCohortJournalStore[:0]
	c.dropCohortFrontiers = c.dropCohortFrontiers[:0]
	c.dropCohortFrontierParticipants = c.dropCohortFrontierParticipants[:0]
	c.dropCohortFrontierMembers = c.dropCohortFrontierMembers[:0]
	c.dropCohortFrontierJournal = c.dropCohortFrontierJournal[:0]
	c.dropCohortDerivationScratch = c.dropCohortDerivationScratch[:0]
	c.dropCohortPathScratch = c.dropCohortPathScratch[:0]
	c.dropCohortEphemeralBytes = 0
	c.dropCohortEphemeralPeak = 0
	c.dropCohortJournal = c.dropCohortJournal[:0]
	c.dropCohortNextSequence = 0
	c.dropCohortFrontierNextSequence = 0
	clear(c.dropCohortProducerWrites[:])
	c.dropCohortAuthenticatedHistory = 0
	c.dropCohortUnprovedHistory = 0
	c.dropCohortOwnerCheckedLookups = 0
	c.dropCohortVerifierElections = 0
	c.dropCohortVerifierProofs = 0
	c.dropCohortVerifierDeclines = 0
	c.dropCohortActionDeclines = 0
	c.dropCohortDerivationDeclines = 0
	clear(c.dropCohortDeclineReasons[:])
	c.dropCohortInlineReads = 0
	c.dropCohortSpillReads = 0
	c.dropCohortMapReads = 0
	c.dropCohortInternerReads = 0
	c.dropCohortReservations = nil
	clear(c.dropCohortReserved[:])
	c.dropCohortReservedBytes = 0
	c.clearLiveCondenseCandidates()
	c.reductionSourceOwner = 0
	c.transactions = c.transactions[:0]
	c.nextTransaction = 0
	c.schedulerFrame.clearInactive()
	c.work = Work{}
	c.popScratch.resetLogical()
	c.reductionScratch.finish()
	c.historicalNodeScratch = c.historicalNodeScratch[:0]
	c.metadataConstructionAuthenticated = true
	c.reduceConflictContext = false
	c.reduceNoLookaheadContext = false
	c.dropCohortSelectionContext = DropCohortSelectionNone
	c.historicalCertificateAuthentication = false
	c.externalTokenScannerStart = 0
	c.externalTokenScannerEnd = 0
	c.externalTokenScannerExact = false
	return nil
}

// ResetReleasingRetention performs the same truncation as Reset, then drops
// retained capacity in two steps: every record arena ReserveRecordArenas
// reserves goes unconditionally (releaseRecordArenaReserve), and every other
// growable family goes when the combined FootprintBytes that remains still
// exceeds coreRetentionCapBytes (releaseOversizedRetention). Reset alone
// preserves capacity for legitimate reuse across repeated large-file parses
// -- the routine "clear the slate before the next attempt" call every fresh
// parse makes through the admission-candidate runner. This variant is for
// decline paths specifically: a just-declined attempt's retained capacity has
// no future value, and keeping it would otherwise stay billed to every later
// unrelated parse on the same cached runner regardless of that parse's own
// size (tranche B9 retention-cap gate). The record arenas need the stronger
// unconditional rule because a reserve is sized below the retention cap by
// construction, so the size gate alone can never see it.
func (c *Core) ResetReleasingRetention() error {
	if err := c.Reset(); err != nil {
		return err
	}
	c.condenseCandidates = nil
	c.releaseRecordArenaReserve()
	c.releaseOversizedRetention()
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
	c.externalTokenScannerStart = 0
	c.externalTokenScannerEnd = 0
	c.externalTokenScannerExact = false
	return nil
}

// SetPhaseCheckpoint binds subsequent condensation to the exact scanner
// checkpoint for the current lookahead epoch. A changed checkpoint advances
// classified-boundary authentication without advancing the frontier epoch.
func (c *Core) SetPhaseCheckpoint(checkpoint CheckpointID) error {
	if len(c.transactions) != 0 {
		return errors.New("parser-core phase zero: set checkpoint during active transaction")
	}
	c.externalTokenScannerStart = 0
	c.externalTokenScannerEnd = 0
	c.externalTokenScannerExact = false
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

// SetPhaseExternalTokenScannerCheckpoints binds one election to its exact
// scanner states. Validation is atomic. A changed pair advances boundary
// authentication even when its end checkpoint is unchanged.
func (c *Core) SetPhaseExternalTokenScannerCheckpoints(start, end CheckpointID) error {
	if len(c.transactions) != 0 {
		return errors.New("parser-core phase zero: set checkpoint during active transaction")
	}
	if end != 0 {
		if _, ok := c.checkpoints.record(end); !ok {
			return errors.New("parser-core phase zero: unknown checkpoint identity")
		}
	}
	if start != 0 {
		if _, ok := c.checkpoints.record(start); !ok {
			return errors.New("parser-core phase zero: unknown external token start checkpoint identity")
		}
	}
	if c.checkpoint == end && c.externalTokenScannerExact &&
		c.externalTokenScannerStart == start && c.externalTokenScannerEnd == end {
		return nil
	}
	if c.classificationPhase == math.MaxUint64 {
		return errors.New("parser-core phase zero: classification phase overflow")
	}
	c.classificationPhase++
	c.checkpoint = end
	c.externalTokenScannerStart = start
	c.externalTokenScannerEnd = end
	c.externalTokenScannerExact = true
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
	id, err := c.appendNodeAt(nodeRecord{
		state: state, byteOffset: byteOffset, pathCount: 1,
	}, key.checkpoint)
	if err != nil {
		return Head{}, err
	}
	if err := c.publishBoundary(probe, id); err != nil {
		c.nodes = c.nodes[:len(c.nodes)-1]
		c.nodeLineages = c.nodeLineages[:len(c.nodeLineages)-1]
		if !c.externalPayloadsQuiescent {
			c.nodeCheckpoints = c.nodeCheckpoints[:len(c.nodeCheckpoints)-1]
		}
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
	if probe.found && !c.condenseNodeIsLive(id) {
		return Head{}, false
	}
	return Head{Node: id}, probe.found
}

func (c *Core) condenseNodeIsLive(id NodeID) bool {
	if !c.condenseScopeActive {
		return true
	}
	if id >= c.condenseNewNode {
		return true
	}
	for _, candidate := range c.condenseCandidates {
		if candidate.Head.Node == id {
			return true
		}
	}
	return false
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

// CheckpointReceiptOwned resolves one checkpoint only under the active
// scheduler token. Callers use it before reading checkpoint interner state.
func (c *Core) CheckpointReceiptOwned(owner SchedulerTransactionToken, id CheckpointID) (uint32, [32]byte, bool) {
	if c == nil || c.validateSchedulerTransaction(owner) != nil {
		return 0, [32]byte{}, false
	}
	return c.checkpoints.receipt(id)
}

// CopyCheckpointBytes copies one exact serialized checkpoint into dst. It
// allows reads during transactions, reuses dst capacity, and returns no
// retained interner storage to the caller.
func (c *Core) CopyCheckpointBytes(id CheckpointID, dst []byte) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	return c.checkpoints.copyBytes(id, dst)
}

// CopyCheckpointBytesOwned copies one checkpoint under the active scheduler
// owner. It follows CheckpointReceiptOwned's stale-owner failure contract.
func (c *Core) CopyCheckpointBytesOwned(owner SchedulerTransactionToken, id CheckpointID, dst []byte) ([]byte, bool) {
	if c == nil || c.validateSchedulerTransaction(owner) != nil {
		return nil, false
	}
	return c.checkpoints.copyBytes(id, dst)
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

// ClassifyBoundaryWithRow builds one authenticated classification from an
// action row the caller already resolved for this head's state and lookahead.
// It exists for the compiled-corridor lane: that lane resolves the cell
// through its own validated instruction stream, whose row index is a
// projection of the same TableView this core reads, so consulting the parse
// table a second time would be pure duplicate work
// (spec.c4-bytecode-isa.v1 section 6.1: the win is removing the
// state-to-row indirection chain).
//
// The caller owns the obligation that actions is exactly what
// Actions(state, lookahead) returns for this head. The corridor discharges it
// statically: its per-state decode-back test walks every (state, symbol) cell
// and requires the compiled row index to equal the table's own action index,
// and its stream validator runs once at build so the interpreter can trust
// the stream (obligations S2 and S3). Every other caller must use
// ClassifyBoundary, which resolves the row itself.
func (c *Core) ClassifyBoundaryWithRow(head Head, lookahead Symbol, actions ActionRow) (ClassifiedBoundary, error) {
	node, err := c.node(head.Node)
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
// one undecorated non-extra shift. The token may have zero width: parser-state
// movement and scanner-checkpoint identity provide scheduler progress even
// when the byte offset is unchanged.
// Missing, no-lookahead, reused, and scanner-checkpoint identity remain
// caller-visible concerns. External provenance is one compact identity bit;
// scanner state remains outside this layer.
// The returned slice is transient scheduler-owned scratch. It stays valid
// only until the next cohort shift on this Core; clone it to retain it.
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
// The returned slice is transient scheduler-owned scratch. It stays valid
// only until the next cohort shift on this Core; clone it to retain it.
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

// ShiftExtraCohort applies one extra terminal election to distinct scheduler
// heads while allocating exactly one immutable terminal payload. An external
// terminal can have zero width when its scheduler proves scanner or parser
// state progress. Every selected cell must contain one undecorated extra shift.
// A target state of zero retains that head's current state, matching production
// extraShiftTargetState semantics.
// The returned slice is transient scheduler-owned scratch. It stays valid
// only until the next cohort shift on this Core; clone it to retain it.
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
// The returned slice is transient scheduler-owned scratch. It stays valid
// only until the next cohort shift on this Core; clone it to retain it.
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
	}, token.LexerSkippedPrefixLength)
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

// CleanPathRankSelection reports how one output relates to the unique clean
// path selected by production's same-boundary stack rank. The compact core
// does not apply this result yet. The scheduler can inspect it in a later
// admission tranche.
type CleanPathRankSelection uint8

const (
	CleanPathRankNotApplicable CleanPathRankSelection = iota
	CleanPathRankUnselected
	CleanPathRankSelected
	CleanPathRankUnknown
)

// HistoricalBoundaryProvenance classifies one retired boundary version.
// Deterministic versions contain no fragile or converged forest. Converged
// versions retain selected-lineage provenance. Unproved versions fail closed.
type HistoricalBoundaryProvenance uint8

const (
	HistoricalBoundaryNone HistoricalBoundaryProvenance = iota
	HistoricalBoundaryDeterministic
	HistoricalBoundaryConverged
	HistoricalBoundaryUnproved
)

// ReductionOutput is one final canonical boundary and its aggregate freshness
// relative to the boundary map at entry to ReduceOutputs. The Links field
// carries the authenticated graph-chain reference published for Head. The
// reference is metadata only; no current route consumes it. Every construction
// site uses keyed fields (reduceOutputsClassifiedIntoActive, scheduler_owned.go).
type ReductionOutput struct {
	Head Head
	// Links is the bounded graph-chain reference produced for Head. Its Count
	// follows link.next and does not imply physically adjacent LinkID values.
	// This value does not enable the default-off sidecar or allocate storage.
	Links          LinkChainRef
	DropCohortRefs DropCohortRefSet
	// HistoricalAlternativeSet is the union of every dead predecessor's
	// recorded alternative set discovered while producing this boundary
	// (spec.b4b-alternative-set.v1 section 4, "Dead-node historical
	// import"). Unlike HistoricalCleanPathRank/Lineage, multiple historical
	// versions union here instead of poisoning to Unknown/0, because
	// membership is never invalidated.
	HistoricalAlternativeSet     AlternativeSet
	HistoricalCleanPathLineage   uint16
	Freshness                    ReductionFreshness
	CleanPathRank                CleanPathRankSelection
	MultiplePopPaths             bool
	HistoricalBoundaryProvenance HistoricalBoundaryProvenance
	HistoricalCleanPathRank      CleanPathRankSelection
	// HistoricalBlended carries forward the blended mark accumulated while
	// building HistoricalAlternativeSet: true when an imported dead record
	// was itself blended, or when two dead sets unioned into this one
	// boundary were incomparable under containment (spec.b4b-alternative-
	// set.v2 section 3.4).
	HistoricalBlended bool
}

const inlineReductionBoundaryOutputs = 2

// Field order groups key, head, links, and the value-owned provenance sets up
// front. The sole construction site uses keyed fields.
type reductionBoundaryOutput struct {
	key                           boundaryKey
	head                          Head
	links                         LinkChainRef
	dropCohortRefs                DropCohortRefSet
	historicalSet                 AlternativeSet
	historicalLineage             uint16
	freshness                     ReductionFreshness
	cleanPathRank                 CleanPathRankSelection
	historicalBoundarySplit       bool
	historicalConvergedSplit      bool
	historicalForestDeterministic bool
	historicalCleanPathRank       CleanPathRankSelection
	historicalBlended             bool
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
	return c.reduceOutputsClassifiedIntoUncheckpointed(SchedulerTransactionToken{}, dst, boundary, actionOrdinal, fork)
}

func (c *Core) reductionParentForPath(
	act Action,
	plan *ReductionPlan,
	path popPath,
	key boundaryKey,
	fork ForkOrder,
	multiPop bool,
	scratch *reductionOutputScratch,
) (SubtreeID, int64, ForkOrder, error) {
	fields, aliases, err := c.remapReductionPlan(path.children, plan, scratch)
	if err != nil {
		return 0, 0, ForkOrder{}, err
	}
	// fragile mirrors markReduceFragility's non-propagation trigger in the
	// production Go parser (parser_reduce.go): this reduce is fragile when
	// more than one pop path was retained for it (multiPop, the pop.size > 1
	// condition -- see reduceOutputsClassifiedIntoActive) or it is one arm of
	// an authenticated >=2-action conflict (c.reduceConflictContext, set by
	// the conflict executor for every arm including the primary). See the
	// subtreeRecord.fragile field doc comment (above) for the monotone
	// set-only contract on dedup below.
	fragile := multiPop || c.reduceConflictContext
	parent := subtreeRecord{
		symbol: act.Symbol, productionID: act.ProductionID,
		dynamicPrecedence: act.DynamicPrecedence,
		startByte:         path.startByte, endByte: path.structuralEnd,
		fragile: fragile,
	}
	if c.reduceNoLookaheadContext {
		predecessor, err := c.node(path.prev)
		if err != nil {
			return 0, 0, ForkOrder{}, err
		}
		parent.extra = key.state == predecessor.state
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
	} else if fragile {
		// found reused an existing (deduped) record instead of publishing
		// parent above; its fragile bit must still be OR'd in -- a record
		// reachable via both a clean and an ambiguous/multi-pop derivation
		// is fragile, since the ambiguous derivation proves the shape was
		// not uniquely determined. Monotone set-only, safe on a shared
		// record (see the field doc comment).
		if rec, err := c.subtree(payload); err == nil && rec != nil && !rec.fragile {
			rec.fragile = true
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
	// fragile is derivation history, not structural identity: two
	// structurally-identical records must still dedup to one record even if
	// one arrived via an ambiguous/multi-pop derivation and the other did
	// not. The caller (reductionParentForPath) is responsible for OR-ing the
	// bit into whichever record survives the dedup -- see its doc comment.
	left.fragile, right.fragile = false, false
	// External provenance state is derived from immutable children and scanner
	// proofs. It is not part of structural identity.
	left.externalProvenanceState, right.externalProvenanceState =
		subtreeExternalProvenanceUnknown, subtreeExternalProvenanceUnknown
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
	maximum, err := c.linkPrecedenceMaximum(linkRecord{
		prev: in.prev, payload: in.payload, scoreDelta: in.scoreDelta,
	})
	if err != nil {
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
	linkID := c.appendGraphLink(linkRecord{
		prev: in.prev, payload: in.payload, scoreDelta: in.scoreDelta,
		order: in.order.Value, flags: flags,
	})
	c.addWork(&c.work.GraphLinkAdditionsProxy, 1)
	id, err := c.appendNodeAtWithMaximum(nodeRecord{
		state: state, byteOffset: byteOffset,
		firstLink: uint32(linkID), linkCount: 1, pathCount: prev.pathCount,
	}, c.checkpoint, maximum.value)
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
	head                          Head
	change                        condenseChange
	historicalDropCohortRefs      DropCohortRefSet
	historicalBoundarySplit       bool
	historicalConvergedSplit      bool
	historicalForestDeterministic bool
	historicalCleanPathRank       CleanPathRankSelection
	historicalLineage             uint16
	// historicalNode is the dead predecessor's NodeID, captured before
	// condenseWithOutcomeAtomic clears oldID below. Its nodeLineage record
	// (and alternative set) persists for the rest of the parse, so callers
	// can read it back through NodeLineageAlternativeSet for dead-node
	// import (spec.b4b-alternative-set.v1 section 4). Populated whenever
	// historicalBoundarySplit is true, regardless of which branch below
	// computed the scalar historical fields.
	historicalNode NodeID
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
	if !probe.found {
		// No incumbent has ever published this boundary in the current
		// frontier: exactly one candidate exists (the incoming link), so no
		// fold comparison against another link is possible, and there is no
		// historical predecessor to retire. This is the L1
		// deterministic-frontier direct-append fast path (spec.campaign.v7
		// tranche C0 item 4; ginkgo's open question 3, "can
		// condenseWithOutcomeAtomic prove a single pop path and take a
		// direct append"). condenseDirectAppend produces the exact bytes the
		// general path below would: every historical* field stays zero,
		// oldID stays 0, and the ~200-line fold-comparison block is
		// unreachable for this shape either way. This is a restructuring of
		// already-dead branches for a proven condition, not a behavior
		// change. publishBoundary still gates journal writes on
		// len(c.transactions) unchanged, so the rollback contract this
		// function depends on is untouched -- this function still cannot
		// prove a caller can never roll back past this append, so it does
		// not weaken that contract.
		return c.condenseDirectAppend(key, probe, prev.pathCount, in)
	}
	historicalBoundarySplit := false
	var historicalCleanPathRank CleanPathRankSelection
	var historicalLineage uint16
	var historicalNode NodeID
	var historicalDropCohortRefs DropCohortRefSet
	historicalConvergedSplit := false
	historicalForestDeterministic := false
	if probe.found && !c.condenseNodeIsLive(oldID) {
		historicalBoundarySplit = true
		historicalNode = oldID
		old, oldErr := c.nodeLineage(oldID)
		if oldErr != nil {
			return condenseOutcome{}, oldErr
		}
		if old.owner != 0 && old.owner == c.reductionSourceOwner {
			historicalForestDeterministic = true
		} else {
			deterministic, deterministicErr := c.historicalForestIsDeterministic(oldID, in)
			if deterministicErr != nil {
				return condenseOutcome{}, deterministicErr
			}
			historicalForestDeterministic = deterministic
			historicalCleanPathRank = old.rank
			historicalLineage = old.lineage
			historicalConvergedSplit = old.converged
		}
		historicalDropCohortRefs = old.dropCohortRefs
		oldID = 0
	}
	// buildOutcome stamps a returned condenseOutcome with the historical
	// provenance snapshot captured above. Every return below resolves the
	// same boundary key, so a historical split discovered above belongs on
	// whichever path this call actually takes, not only on the path that
	// happens to run first. This applies the duplicate-drop path's existing
	// propagation uniformly instead of letting the other early returns
	// silently drop it.
	buildOutcome := func(head Head, change condenseChange) condenseOutcome {
		return condenseOutcome{
			head: head, change: change,
			historicalDropCohortRefs:      historicalDropCohortRefs,
			historicalBoundarySplit:       historicalBoundarySplit,
			historicalConvergedSplit:      historicalConvergedSplit,
			historicalForestDeterministic: historicalForestDeterministic,
			historicalCleanPathRank:       historicalCleanPathRank,
			historicalLineage:             historicalLineage,
			historicalNode:                historicalNode,
		}
	}
	var old nodeRecord
	var oldLinks []linkRecord
	if oldID != 0 {
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
				return buildOutcome(Head{Node: oldID}, condenseUnchanged), nil
			}
		}
		if c.diagnostics.foldSamePredecessorShallowPayloads {
			// A shallow-class-equal incumbent shares the incoming payload's symbol,
			// span, and child count. Under ordinary reductions a boundary holds at
			// most one such incumbent, but a declined precedence tie (below) can
			// leave two structurally distinct alternates coexisting, so this search
			// tolerates several and also finds the structurally identical one.
			shallowCount := 0
			firstShallow := -1
			structuralMatch := -1
			for index, link := range oldLinks {
				equal, err := c.shallowPayloadClassEqual(link, in)
				if err != nil {
					return condenseOutcome{}, err
				}
				if !equal {
					continue
				}
				_, incumbentExact, err := c.subtreeExternalProvenance(link.payload)
				if err != nil {
					return condenseOutcome{}, err
				}
				_, incomingExact, err := c.subtreeExternalProvenance(in.payload)
				if err != nil {
					return condenseOutcome{}, err
				}
				if !incumbentExact || !incomingExact {
					return condenseOutcome{}, errors.New("parser-core phase zero: shallow fold declined inexact external payload provenance")
				}
				shallowCount++
				if firstShallow < 0 {
					firstShallow = index
				}
				structural, err := c.subtreesStructurallyEqual(link.payload, in.payload)
				if err != nil {
					return condenseOutcome{}, err
				}
				if structural {
					if structuralMatch >= 0 {
						return condenseOutcome{}, errors.New("parser-core phase zero: multiple structural-fold incumbents")
					}
					structuralMatch = index
				}
			}
			foldDeclined := false
			switch {
			case structuralMatch >= 0:
				// The incoming payload reproduces an existing alternate exactly, so
				// it is a redundant GLR path. Structural equality implies equal
				// dynamic precedence, so this is the duplicate drop that keeps one
				// subtree per (symbol, span), matching production.
				c.recordLinkUnionDuplicateNoop()
				if phase0AEnabled {
					phase0AObserveCandidateDrop(c, key, in, oldID, structuralMatch, phase0ATransitionPrecedenceDrop)
				}
				return buildOutcome(Head{Node: oldID}, condenseUnchanged), nil
			case shallowCount == 1:
				// Exactly one shallow-class incumbent, structurally different. Rank
				// the two by dynamic precedence, production's primary disambiguation
				// for the clean parses this route admits (error cost is zero here, so
				// it never outranks precedence).
				incumbent := firstShallow
				incumbentPrecedence, err := c.effectivePayloadPrecedence(oldLinks[incumbent].payload, oldLinks[incumbent].scoreDelta)
				if err != nil {
					return condenseOutcome{}, err
				}
				incomingPrecedence, err := c.effectivePayloadPrecedence(in.payload, in.scoreDelta)
				if err != nil {
					return condenseOutcome{}, err
				}
				switch {
				case incomingPrecedence < incumbentPrecedence:
					// The incumbent strictly dominates; drop the dominated incoming
					// payload, as production keeps the higher-precedence subtree.
					c.recordLinkUnionDuplicateNoop()
					if phase0AEnabled {
						phase0AObserveCandidateDrop(c, key, in, oldID, incumbent, phase0ATransitionPrecedenceDrop)
					}
					return buildOutcome(Head{Node: oldID}, condenseUnchanged), nil
				case incomingPrecedence > incumbentPrecedence:
					// The incoming payload strictly dominates; replace the incumbent.
					if phase0AEnabled {
						phase0ABeginReplacement(c, key, in, oldID, incumbent)
					}
					head, err := c.replaceBoundaryLink(key, probe, old, oldLinks, incumbent, in)
					if err != nil {
						c.recordLinkUnionRejected()
					} else {
						c.recordLinkUnionPrecedenceReplaced()
					}
					return buildOutcome(head, condenseUpdated), err
				default:
					// A precedence tie between two structurally different same-span
					// payloads. Dynamic precedence cannot rank them, and the compact
					// route does not own production's error-cost and structural
					// tie-breaks. Choosing one here by insertion order is the silent
					// wrong-tree divergence the admission flip surfaced (Go's
					// call_expression(index_expression) versus
					// type_conversion_expression(generic_type) for `Foo[int](a)`).
					// Decline the fold: keep both links as coexisting alternates and
					// let them race to EOF. A local ambiguity collapses when one arm
					// dies; a genuine whole-parse ambiguity keeps more than one
					// accepted derivation, and the sole-exact acceptance gate then
					// fails closed to production.
					foldDeclined = true
				}
			case shallowCount >= 2:
				// The boundary already carries coexisting structural alternates from
				// an earlier declined tie. Do not precedence-collapse across an
				// already ambiguous boundary; append the incoming payload as a
				// further distinct alternate for the sole-exact gate to resolve.
				foldDeclined = true
			}
			// A declined fold skips the exact-predecessor factor (which merges
			// same-boundary predecessors) and appends the incoming link as a
			// surviving alternate below. With no shallow-class incumbent the factor
			// still runs, preserving the recursive-insertion path unchanged.
			if !foldDeclined && shallowCount == 0 {
				folded := precedenceMaximumWitness{}
				outcome, handled, err := c.factorExactPredecessor(key, probe, oldID, oldLinks, in, &folded)
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
	var finalMaximum precedenceCandidate
	haveFinalMaximum := false
	if oldID != 0 {
		oldMaximum, err := c.nodePrecedenceMaximum(oldID)
		if err != nil {
			return condenseOutcome{}, err
		}
		finalMaximum = oldMaximum
		haveFinalMaximum = true
	}
	incomingMaximum, err := c.linkPrecedenceMaximum(linkRecord{
		prev: in.prev, payload: in.payload, scoreDelta: in.scoreDelta,
	})
	if err != nil {
		return condenseOutcome{}, err
	}
	if !haveFinalMaximum || incomingMaximum.value > finalMaximum.value {
		finalMaximum = incomingMaximum
		haveFinalMaximum = true
	}
	flags := uint32(0)
	if in.order.Present {
		flags |= linkFlagHasOrder
	}
	linkID := c.appendGraphLink(linkRecord{
		prev: in.prev, payload: in.payload, scoreDelta: in.scoreDelta,
		order: in.order.Value, flags: flags, next: LinkID(old.firstLink),
	})
	c.addWork(&c.work.GraphLinkAdditionsProxy, 1)
	id, err := c.appendNodeAtWithMaximum(nodeRecord{
		state: key.state, byteOffset: key.byteOffset,
		firstLink: uint32(linkID), linkCount: linkCount, pathCount: newPathCount,
	}, key.checkpoint, finalMaximum.value)
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
	return buildOutcome(Head{Node: id}, change), nil
}

// condenseDirectAppend is condenseWithOutcomeAtomic's single-candidate fast
// path: probe.found is false, so this frontier has never published key
// before, in is the sole candidate, and no fold comparison or historical
// retirement applies. Every argument and every write below matches exactly
// what the general path performs when oldID stays 0 throughout -- the same
// arena-cap checks, the same unlinked (next: 0) single-link node, the same
// publishBoundary call against the same probe, and the same zero-valued
// condenseOutcome shape (change: condenseNew, every historical* field at its
// zero value). publishBoundary keeps deciding journal writes from
// len(c.transactions) unchanged; this helper does not touch that contract.
func (c *Core) condenseDirectAppend(key boundaryKey, probe boundaryProbe, prevPathCount uint64, in linkInput) (condenseOutcome, error) {
	if uint64(len(c.links))+1 > uint64(c.limits.MaxLinks) || uint64(len(c.links)) >= math.MaxUint32 {
		return condenseOutcome{}, errors.New("parser-core phase zero: link arena cap")
	}
	if uint64(len(c.nodes))+1 > uint64(c.limits.MaxNodes) || uint64(len(c.nodes)) >= math.MaxUint32 {
		return condenseOutcome{}, errors.New("parser-core phase zero: node arena cap")
	}
	maximum, err := c.linkPrecedenceMaximum(linkRecord{
		prev: in.prev, payload: in.payload, scoreDelta: in.scoreDelta,
	})
	if err != nil {
		return condenseOutcome{}, err
	}
	flags := uint32(0)
	if in.order.Present {
		flags |= linkFlagHasOrder
	}
	linkID := c.appendGraphLink(linkRecord{
		prev: in.prev, payload: in.payload, scoreDelta: in.scoreDelta,
		order: in.order.Value, flags: flags,
	})
	c.addWork(&c.work.GraphLinkAdditionsProxy, 1)
	id, err := c.appendNodeAtWithMaximum(nodeRecord{
		state: key.state, byteOffset: key.byteOffset,
		firstLink: uint32(linkID), linkCount: 1, pathCount: prevPathCount,
	}, key.checkpoint, maximum.value)
	if err != nil {
		return condenseOutcome{}, err
	}
	if err := c.publishBoundary(probe, id); err != nil {
		return condenseOutcome{}, err
	}
	if phase0AEnabled {
		phase0AObserveDirectPublication(c, key, in, linkID, id, 0)
	}
	return condenseOutcome{head: Head{Node: id}, change: condenseNew}, nil
}

func (c *Core) linkEqualInput(link linkRecord, in linkInput) (bool, error) {
	return link.prev == in.prev && link.payload == in.payload && link.scoreDelta == in.scoreDelta &&
		link.hasOrder() == in.order.Present && (!link.hasOrder() || link.order == in.order.Value), nil
}

func (c *Core) historicalForestIsDeterministic(oldID NodeID, in linkInput) (bool, error) {
	incomingPayload, err := c.subtree(in.payload)
	if err != nil {
		return false, err
	}
	if incomingPayload.fragile {
		return false, nil
	}
	oldDeterministic, err := c.graphVersionIsDeterministic(oldID)
	if err != nil || !oldDeterministic {
		return oldDeterministic, err
	}
	return c.graphVersionIsDeterministic(in.prev)
}

func (c *Core) graphVersionIsDeterministic(root NodeID) (bool, error) {
	stack := c.historicalNodeScratch[:0]
	stack = append(stack, root)
	defer func() {
		clear(stack)
		c.historicalNodeScratch = stack[:0]
	}()
	var visits uint64
	for len(stack) != 0 {
		if visits >= uint64(c.limits.MaxNodes) {
			return false, nil
		}
		visits++
		last := len(stack) - 1
		id := stack[last]
		stack = stack[:last]
		node, err := c.node(id)
		if err != nil {
			return false, err
		}
		provenance, err := c.nodeLineage(id)
		if err != nil {
			return false, err
		}
		if provenance.converged {
			return false, nil
		}
		linkID := LinkID(node.firstLink)
		for count := uint32(0); count < node.linkCount; count++ {
			if linkID == 0 || uint64(linkID) > uint64(len(c.links)) {
				return false, errors.New("parser-core phase zero: historical forest has an invalid link")
			}
			link := c.links[linkID-1]
			payload, err := c.subtree(link.payload)
			if err != nil {
				return false, err
			}
			if payload.fragile {
				return false, nil
			}
			if link.prev >= id {
				return false, errors.New("parser-core phase zero: historical forest predecessor is not earlier than its node")
			}
			stack = append(stack, link.prev)
			linkID = link.next
		}
		if linkID != 0 {
			return false, errors.New("parser-core phase zero: historical forest has excess links")
		}
		if node.linkCount == 0 && node.firstLink != 0 {
			return false, errors.New("parser-core phase zero: empty historical forest node has a link")
		}
	}
	return true, nil
}

type shallowPayloadClass struct {
	symbol     Symbol
	padding    uint32
	size       uint32
	childCount uint32
	extra      bool
	external   bool
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
// outer edge must match in every stored field except predecessor identity, or
// belong to the same shallow class. The private folded maximum records the
// node-level precedence that C updates for the shallow non-exact case.
func (c *Core) factorExactPredecessor(key boundaryKey, probe boundaryProbe, oldID NodeID, oldLinks []linkRecord, in linkInput, folded *precedenceMaximumWitness) (out condenseOutcome, handled bool, err error) {
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
		_, leftExact, provenanceErr := c.subtreeExternalProvenance(incumbent.payload)
		if provenanceErr != nil {
			return condenseOutcome{}, true, provenanceErr
		}
		_, rightExact, provenanceErr := c.subtreeExternalProvenance(in.payload)
		if provenanceErr != nil {
			return condenseOutcome{}, true, provenanceErr
		}
		if !leftExact || !rightExact {
			return condenseOutcome{}, true, errors.New("parser-core phase zero: recursive insertion declined inexact external payload provenance")
		}
		if !exactEdge {
			pairsEqual, pairErr := c.subtreeScannerStatePairsEqual(incumbent.payload, in.payload)
			if pairErr != nil {
				return condenseOutcome{}, true, pairErr
			}
			if !pairsEqual {
				return condenseOutcome{}, true, errors.New("parser-core phase zero: recursive insertion declined scanner-state pair mismatch")
			}
		}
		if !shallow {
			return condenseOutcome{}, true, errors.New("parser-core phase zero: exact recursive edge is not a clean shallow payload")
		}

		// The merge-and-republish body owns a bounded transaction. It lives in a
		// dedicated method so its completion defer is open-coded, not heap-
		// allocated once per exact-predecessor factor by a loop-scoped defer.
		return c.factorExactPredecessorMerge(key, probe, oldID, oldLinks, index, incumbent, in, folded)
	}
	return condenseOutcome{}, false, nil
}

// factorExactPredecessorMerge merges one incumbent predecessor with the
// incoming edge and republishes the boundary. Callers pass the matched
// incumbent link and its index. The transaction completion runs through a
// single function-scoped defer, so it is open-coded and does not allocate.
func (c *Core) factorExactPredecessorMerge(key boundaryKey, probe boundaryProbe, oldID NodeID, oldLinks []linkRecord, index int, incumbent linkRecord, in linkInput, folded *precedenceMaximumWitness) (out condenseOutcome, handled bool, err error) {
	handled = true
	mark := c.mark()
	defer c.completeTransaction(mark, &err)
	if phase0AEnabled {
		phase0ABeginPredecessorMerge(c, incumbent.prev, in.prev)
	}
	merged, changed, mergeErr := c.mergePredecessorsBounded(incumbent.prev, in.prev, 0, folded)
	if mergeErr != nil {
		if phase0AEnabled {
			phase0AAbortPredecessorMerge(c)
		}
		return condenseOutcome{}, true, mergeErr
	}
	// The nested merge already published its own maximum on the merged
	// predecessor. The boundary node folds separately: C rule 3 maxes the
	// boundary's stored value with the INCOMING predecessor's stored value
	// plus the payload contribution (stack.c:237-243 reads link.node, which
	// the recursion never mutates). Do not carry nested discarded or
	// replacement records into the outer adjacency.
	oldMaximum, mergeErr := c.nodePrecedenceMaximum(oldID)
	if mergeErr != nil {
		return condenseOutcome{}, true, mergeErr
	}
	*folded = precedenceMaximumWitness{seed: oldMaximum.value, hasSeed: true}
	outerDiscarded := linkRecord{
		prev: in.prev, payload: in.payload, scoreDelta: in.scoreDelta,
	}
	if in.order.Present {
		outerDiscarded.order = in.order.Value
		outerDiscarded.flags |= linkFlagHasOrder
	}
	if mergeErr := c.observeDiscardedLink(folded, outerDiscarded); mergeErr != nil {
		return condenseOutcome{}, true, mergeErr
	}
	outerMaximum, mergeErr := c.linkPrecedenceMaximum(outerDiscarded)
	if mergeErr != nil {
		return condenseOutcome{}, true, mergeErr
	}
	if !changed && outerMaximum.value <= oldMaximum.value {
		if phase0AEnabled {
			phase0AAbortPredecessorMerge(c)
			phase0AObserveFactorNoChange(c, key, in, oldID, index)
		}
		return condenseOutcome{head: Head{Node: oldID}, change: condenseUnchanged}, true, nil
	}
	if phase0AEnabled {
		phase0AObserveAdjacencyPublished(c, merged)
	}
	// appendAdjacencyNode copies every link into the arena, so the rebuilt set
	// is transient and reuses a scheduler-owned scratch buffer instead of
	// cloning oldLinks on every exact-predecessor factor.
	rebuilt := append(c.factorLinkScratch[:0], oldLinks...)
	c.factorLinkScratch = rebuilt
	rebuilt[index].prev = merged
	if phase0AEnabled {
		phase0APrepareFactorOuter(c, key, in, oldID, index, merged)
	}
	id, appendErr := c.appendAdjacencyNodeAtWithPrecedence(key.state, key.byteOffset, key.checkpoint, rebuilt, *folded)
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

// maxRecursiveInsertDepth bounds the persistent counterpart of C's recursive
// stack-link insertion. Sixteen levels cover the measured clean corpus
// family while keeping pathological graphs on a small, fixed stack budget.
const maxRecursiveInsertDepth = 16

func (c *Core) mergePredecessorsBounded(leftID, rightID NodeID, depth int, folded *precedenceMaximumWitness) (NodeID, bool, error) {
	if depth > maxRecursiveInsertDepth {
		return 0, false, errors.New("parser-core phase zero: recursive insertion depth limit")
	}
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
	if folded == nil {
		return 0, false, errors.New("parser-core phase zero: missing folded precedence maximum witness")
	}
	leftMaximum, err := c.nodePrecedenceMaximum(leftID)
	if err != nil {
		return 0, false, err
	}
	// The merge mutates the left node in C terms, so the fold starts from the
	// left node's stored running value (stack_node_add_link's self).
	*folded = precedenceMaximumWitness{seed: leftMaximum.value, hasSeed: true}
	leftCheckpoint, leftExact := c.nodeScannerCheckpoint(leftID)
	rightCheckpoint, rightExact := c.nodeScannerCheckpoint(rightID)
	if left.state != right.state || left.byteOffset != right.byteOffset ||
		!leftExact || !rightExact || leftCheckpoint != rightCheckpoint {
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
		links, inserted, err = c.insertLinkBounded(left.state, left.byteOffset, links, incoming, depth, folded)
		if err != nil {
			return 0, false, err
		}
		changed = changed || inserted
	}
	if len(links) == 0 && !changed {
		if err := c.mergeNodeLineageMetadata(leftID, rightID, leftID); err != nil {
			return 0, false, err
		}
		return leftID, false, nil
	}
	// C updates the containing node's maximum even when the incoming edge is
	// shallow-equivalent and the link set stays unchanged. Fold the stored
	// seed with the observed rule contributions before making that decision.
	maximum, err := c.computePrecedenceMaximum(links, *folded)
	if err != nil {
		return 0, false, err
	}
	if !changed && maximum.value > leftMaximum.value {
		changed = true
	}
	if !changed {
		if err := c.mergeNodeLineageMetadata(leftID, rightID, leftID); err != nil {
			return 0, false, err
		}
		return leftID, false, nil
	}
	merged, err := c.appendAdjacencyNodeAtWithPrecedence(left.state, left.byteOffset, leftCheckpoint, links, *folded)
	if err != nil {
		return 0, false, err
	}
	if err := c.mergeNodeLineageMetadata(leftID, rightID, merged); err != nil {
		return 0, false, err
	}
	return merged, true, nil
}

// insertLinkBounded mirrors the measured lower-adjacency decisions of
// stack_node_add_link without mutating an existing adjacency. Same-pair
// shallow payloads select the higher effective subtree precedence; every
// other clean class remains in stable incumbent-first order. Boundary-equal
// predecessors recurse only when their complete outer edges are exact.

func (c *Core) insertLinkBounded(state StateID, byteOffset uint32, links []linkRecord, incoming linkRecord, depth int, folded *precedenceMaximumWitness) ([]linkRecord, bool, error) {
	if folded == nil {
		c.recordLinkUnionRejected()
		return nil, false, errors.New("parser-core phase zero: missing folded precedence maximum witness")
	}
	_, err := c.linkPrecedenceMaximum(incoming)
	if err != nil {
		c.recordLinkUnionRejected()
		return nil, false, err
	}
	if len(links) == 0 {
		if err := c.observeDiscardedLink(folded, incoming); err != nil {
			c.recordLinkUnionRejected()
			return nil, false, err
		}
		return append(slices.Clone(links), incoming), true, nil
	}
	c.recordLinkUnionAttempt()
	_, incomingExact, err := c.subtreeExternalProvenance(incoming.payload)
	if err != nil {
		c.recordLinkUnionRejected()
		return nil, false, err
	}
	if !incomingExact {
		c.recordLinkUnionRejected()
		return nil, false, errors.New("parser-core phase zero: recursive insertion declined inexact external payload provenance")
	}
	for index, incumbent := range links {
		_, incumbentExact, err := c.subtreeExternalProvenance(incumbent.payload)
		if err != nil {
			c.recordLinkUnionRejected()
			return nil, false, err
		}
		if !incumbentExact {
			c.recordLinkUnionRejected()
			return nil, false, errors.New("parser-core phase zero: recursive insertion declined inexact external payload provenance")
		}
		if c.linkRecordsEqual(incumbent, incoming) {
			// C rule 2: an exact duplicate performs no update at all
			// (stack.c:225), so the fold observes nothing.
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
				// C rule 2: a same-pair link that does not raise the subtree
				// precedence performs no update (stack.c:225).
				c.recordLinkUnionDuplicateNoop()
				if phase0AEnabled {
					phase0AMergeDecision(c, index, phase0ATransitionPrecedenceDrop)
				}
				return links, false, nil
			}
			// C rule 1: the assignment overwrites the running value
			// (stack.c:222-223), so the incumbent's value is discarded, not
			// recorded, and any post-assignment contributions start fresh.
			updated := slices.Clone(links)
			updated[index] = incoming
			folded.replacement = incoming
			folded.hasReplacement = true
			folded.hasPostReplacement = false
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
		if !c.linkEdgesEqual(incumbent, incoming) {
			c.recordLinkUnionRejected()
			return nil, false, errors.New("parser-core phase zero: recursive insertion declined non-exact nested edge")
		}
		if depth >= maxRecursiveInsertDepth {
			c.recordLinkUnionRejected()
			return nil, false, errors.New("parser-core phase zero: recursive insertion depth limit")
		}
		if phase0AEnabled {
			phase0ABeginPredecessorMerge(c, incumbent.prev, incoming.prev)
		}
		nestedFolded := precedenceMaximumWitness{}
		merged, changed, err := c.mergePredecessorsBounded(incumbent.prev, incoming.prev, depth+1, &nestedFolded)
		if err != nil {
			if phase0AEnabled {
				phase0AAbortPredecessorMerge(c)
			}
			c.recordLinkUnionRejected()
			return nil, false, err
		}
		if !changed {
			if err := c.observeDiscardedLink(folded, incoming); err != nil {
				c.recordLinkUnionRejected()
				return nil, false, err
			}
			if phase0AEnabled {
				phase0AAbortPredecessorMerge(c)
				phase0AMergeDecision(c, index, phase0ATransitionDuplicateDrop)
			}
			c.recordLinkUnionDuplicateNoop()
			return links, false, nil
		}
		if phase0AEnabled {
			phase0AObserveAdjacencyPublished(c, merged)
			phase0AMergeRecursiveDecision(c, index, merged)
		}
		// C rule 3 runs whether or not the recursion changed the incumbent
		// predecessor: it maxes the containing node with the INCOMING
		// predecessor's stored value plus the payload contribution
		// (stack.c:237-243 reads link.node, which the recursion never
		// mutates). The published link points at the merged predecessor, but
		// the fold contribution does not.
		if err := c.observeDiscardedLink(folded, incoming); err != nil {
			c.recordLinkUnionRejected()
			return nil, false, err
		}
		updated := slices.Clone(links)
		updated[index].prev = merged
		c.recordLinkUnionRecursiveChanged()
		return updated, true, nil
	}
	if uint32(len(links)) >= c.limits.MaxLinksPerBoundary {
		c.recordLinkUnionRejected()
		return nil, false, &LiveLinkCapacityError{State: state, ByteOffset: byteOffset, ObservedLinks: uint64(len(links)) + 1, Limit: c.limits.MaxLinksPerBoundary}
	}
	// C rule 5: an appended link maxes its contribution into the running
	// value (stack.c:253-263). The seeded fold no longer recomputes from the
	// final adjacency, so every append observes its contribution here.
	if err := c.observeDiscardedLink(folded, incoming); err != nil {
		c.recordLinkUnionRejected()
		return nil, false, err
	}
	c.recordLinkUnionAlternateAppended()
	if phase0AEnabled {
		phase0AMergeDecision(c, -1, phase0ATransitionAlternateAppend)
	}
	return append(slices.Clone(links), incoming), true, nil
}

// subtreeExternalProvenance reports whether a payload contains an external
// terminal and whether every such terminal has an exact scanner-state pair.
func (c *Core) subtreeExternalProvenance(root SubtreeID) (hasExternal, exact bool, err error) {
	record, err := c.subtree(root)
	if err != nil {
		return false, false, err
	}
	if c.externalPayloadsQuiescent {
		return false, true, nil
	}
	if has, exact, cached := record.externalProvenanceState.result(); cached {
		return has, exact, nil
	}
	var walk func(SubtreeID) (bool, bool, error)
	walk = func(id SubtreeID) (bool, bool, error) {
		record, err := c.subtree(id)
		if err != nil {
			return false, false, err
		}
		if has, exact, cached := record.externalProvenanceState.result(); cached {
			return has, exact, nil
		}
		if record.external {
			provenance, ok := c.externalPayloadScannerProvenance(id)
			if !record.terminal || !ok {
				record.externalProvenanceState = subtreeExternalProvenanceInexactHasExternal
				return true, false, nil
			}
			for _, checkpoint := range [...]CheckpointID{provenance.start, provenance.end} {
				if checkpoint == 0 {
					continue
				}
				if _, ok := c.checkpoints.record(checkpoint); !ok {
					record.externalProvenanceState = subtreeExternalProvenanceInexactHasExternal
					return true, false, nil
				}
			}
			record.externalProvenanceState = subtreeExternalProvenanceExactHasExternal
			return true, true, nil
		}
		has := false
		for _, child := range c.children[record.firstChild : record.firstChild+record.childCount] {
			if child >= id {
				return false, false, errors.New("parser-core phase zero: compact subtree child does not precede its parent")
			}
			childHas, childExact, err := walk(child)
			if err != nil || !childExact {
				if err == nil {
					record.externalProvenanceState = subtreeExternalProvenanceInexactHasExternal
				}
				return has || childHas, childExact, err
			}
			has = has || childHas
		}
		record.externalProvenanceState = subtreeExternalProvenanceExactNoExternal
		if has {
			record.externalProvenanceState = subtreeExternalProvenanceExactHasExternal
		}
		return has, true, nil
	}
	return walk(root)
}

// subtreeScannerStatePairsEqual compares the root post-scan state used by the
// locked C route. Nonterminal roots carry empty scanner state. The helper
// reads only interned checkpoint identities and allocates no scratch storage.
func (c *Core) subtreeScannerStatePairsEqual(left, right SubtreeID) (bool, error) {
	if c.externalPayloadsQuiescent {
		return true, nil
	}
	rootEnd := func(id SubtreeID) (CheckpointID, error) {
		record, err := c.subtree(id)
		if err != nil {
			return 0, err
		}
		if !record.external || !record.terminal {
			return 0, nil
		}
		provenance, ok := c.externalPayloadScannerProvenance(id)
		if !ok {
			return 0, errors.New("parser-core phase zero: scanner-state pair proof is unavailable")
		}
		if provenance.end != 0 {
			if _, ok := c.checkpoints.record(provenance.end); !ok {
				return 0, errors.New("parser-core phase zero: scanner-state pair checkpoint is unavailable")
			}
		}
		return provenance.end, nil
	}
	leftEnd, err := rootEnd(left)
	if err != nil {
		return false, err
	}
	rightEnd, err := rootEnd(right)
	if err != nil {
		return false, err
	}
	return leftEnd == rightEnd, nil
}

func (state subtreeExternalProvenanceState) result() (hasExternal, exact, cached bool) {
	switch state {
	case subtreeExternalProvenanceExactNoExternal:
		return false, true, true
	case subtreeExternalProvenanceExactHasExternal:
		return true, true, true
	case subtreeExternalProvenanceInexactHasExternal:
		return true, false, true
	default:
		return false, false, false
	}
}

func (c *Core) deriveSubtreeExternalProvenanceState(r subtreeRecord, children []SubtreeID) subtreeExternalProvenanceState {
	if c.externalPayloadsQuiescent {
		return subtreeExternalProvenanceExactNoExternal
	}
	if r.external {
		return subtreeExternalProvenanceInexactHasExternal
	}
	state := subtreeExternalProvenanceExactNoExternal
	next := SubtreeID(uint64(len(c.subtrees)) + 1)
	for _, child := range children {
		if child == 0 || child >= next {
			return subtreeExternalProvenanceUnknown
		}
		hasExternal, exact, cached := c.subtrees[child-1].externalProvenanceState.result()
		if !cached {
			return subtreeExternalProvenanceUnknown
		}
		if !exact {
			return subtreeExternalProvenanceInexactHasExternal
		}
		if hasExternal {
			state = subtreeExternalProvenanceExactHasExternal
		}
	}
	return state
}

func (c *Core) externalPayloadScannerProvenance(payload SubtreeID) (externalPayloadProvenance, bool) {
	low, high := 0, len(c.externalProvenance)
	for low < high {
		mid := low + (high-low)/2
		candidate := c.externalProvenance[mid]
		if candidate.payload < payload {
			low = mid + 1
			continue
		}
		high = mid
	}
	if low >= len(c.externalProvenance) || c.externalProvenance[low].payload != payload {
		return externalPayloadProvenance{}, false
	}
	return c.externalProvenance[low], true
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
	leftCheckpoint, leftExact := c.nodeScannerCheckpoint(leftID)
	rightCheckpoint, rightExact := c.nodeScannerCheckpoint(rightID)
	return left.state == right.state &&
		left.byteOffset == right.byteOffset &&
		leftExact && rightExact &&
		leftCheckpoint == rightCheckpoint, nil
}

func (c *Core) nodeScannerCheckpoint(id NodeID) (CheckpointID, bool) {
	if c.externalPayloadsQuiescent {
		return 0, true
	}
	if id == 0 || uint64(id) > uint64(len(c.nodeCheckpoints)) {
		return 0, false
	}
	checkpoint := c.nodeCheckpoints[id-1]
	if checkpoint != 0 {
		if _, ok := c.checkpoints.record(checkpoint); !ok {
			return 0, false
		}
	}
	return checkpoint, true
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
	return c.appendAdjacencyNodeAt(state, byteOffset, c.checkpoint, links)
}

func (c *Core) appendAdjacencyNodeAt(state StateID, byteOffset uint32, checkpoint CheckpointID, links []linkRecord) (NodeID, error) {
	return c.appendAdjacencyNodeAtWithPrecedence(state, byteOffset, checkpoint, links, precedenceMaximumWitness{})
}

// appendAdjacencyNodeAtWithPrecedence publishes an adjacency with its folded
// precedence maximum. A seeded witness carries C's stored running value
// through a merge transaction; an unseeded witness computes a fresh node's
// value from its link contributions, matching stack_node_new plus appends.
// Neither shape verifies the value against outside evidence; the C rule
// table in the precedence rule-table tests is the behavioral contract.
func (c *Core) appendAdjacencyNodeAtWithPrecedence(state StateID, byteOffset uint32, checkpoint CheckpointID, links []linkRecord, folded precedenceMaximumWitness) (NodeID, error) {
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
	maximum, err := c.computePrecedenceMaximum(links, folded)
	if err != nil {
		return 0, err
	}
	var first LinkID
	for _, stored := range links {
		copy := stored
		copy.next = first
		c.appendGraphLink(copy)
		c.addWork(&c.work.GraphLinkAdditionsProxy, 1)
		first = LinkID(len(c.links))
	}
	return c.appendNodeAtWithMaximum(nodeRecord{
		state: state, byteOffset: byteOffset, firstLink: uint32(first),
		linkCount: uint32(len(links)), pathCount: pathCount,
	}, checkpoint, maximum.value)
}

// subtreesStructurallyEqual reports whether two compact payload subtrees are the
// same parse: identical symbol, span, production, precedence, extra/external
// flags, field and alias vectors, and recursively identical children. It is the
// authorization test for a same-predecessor shallow-payload fold. Two links that
// reach one boundary with structurally-equal payloads are a redundant GLR path
// (the compact route and production both collapse them). Two links whose
// payloads are only shallow-class-equal but structurally DIFFERENT are a genuine
// structural ambiguity (for example Go's call_expression(index_expression) versus
// type_conversion_expression(generic_type) for `Foo[int](a)`); the compact route
// must not silently pick one by a local precedence comparison, so the fold is
// declined and both links survive as alternates. The surviving alternates raise
// the accepted-head derivation count above one, and the sole-exact acceptance
// gate then fails closed to production instead of routing a wrong tree.
//
// Payloads are only compared after a shallow-class match (same symbol, span, and
// child count), so the walk short-circuits on the first structural difference and
// stays bounded by the matched subtree's size. Compact payloads are acyclic by
// construction: appendSubtreeRecord appends every child before its parent, so a
// child SubtreeID is strictly less than its parent's. The walk enforces that
// order and returns a fail-closed error on a violation, so the recursion also
// terminates on a corrupted arena instead of overflowing the stack.
func (c *Core) subtreesStructurallyEqual(left, right SubtreeID) (bool, error) {
	if left == right {
		return true, nil
	}
	l, err := c.subtree(left)
	if err != nil {
		return false, err
	}
	r, err := c.subtree(right)
	if err != nil {
		return false, err
	}
	if l.symbol != r.symbol || l.productionID != r.productionID || l.dynamicPrecedence != r.dynamicPrecedence ||
		l.startByte != r.startByte || l.endByte != r.endByte || l.childCount != r.childCount ||
		l.fieldCount != r.fieldCount || l.aliasCount != r.aliasCount ||
		l.extra != r.extra || l.external != r.external || l.terminal != r.terminal ||
		l.missing != r.missing {
		// missing participates deliberately. This predicate authorizes a
		// duplicate DROP, so omitting the bit would let a recovery-inserted
		// MISSING payload be discarded in favour of a clean zero-width
		// payload with the same symbol and span, losing the error entirely.
		// Including it is strictly fail-closed: it can only keep two records
		// apart that would otherwise have been folded.
		return false, nil
	}
	if l.external {
		leftProvenance, leftExact := c.externalPayloadScannerProvenance(left)
		rightProvenance, rightExact := c.externalPayloadScannerProvenance(right)
		if leftExact != rightExact ||
			leftExact && (leftProvenance.start != rightProvenance.start ||
				leftProvenance.end != rightProvenance.end) {
			return false, nil
		}
	}
	if !slices.Equal(
		c.fields[l.firstField:l.firstField+l.fieldCount],
		c.fields[r.firstField:r.firstField+r.fieldCount],
	) {
		return false, nil
	}
	if !slices.Equal(
		c.aliases[l.firstAlias:l.firstAlias+l.aliasCount],
		c.aliases[r.firstAlias:r.firstAlias+r.aliasCount],
	) {
		return false, nil
	}
	leftChildren := c.children[l.firstChild : l.firstChild+l.childCount]
	rightChildren := c.children[r.firstChild : r.firstChild+r.childCount]
	for index := range leftChildren {
		if leftChildren[index] >= left || rightChildren[index] >= right {
			return false, errors.New("parser-core phase zero: compact subtree child identifier out of order during structural comparison")
		}
		equal, err := c.subtreesStructurallyEqual(leftChildren[index], rightChildren[index])
		if err != nil || !equal {
			return equal, err
		}
	}
	return true, nil
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
	// This class is the compact port of C's stack__subtree_is_equivalent
	// (stack.c:181-197), MINUS one clause. C tests, in order: same pointer;
	// equal symbol; a BOTH-HAVE-ERRORS shortcut ("if both have errors, don't
	// bother keeping both", stack.c:189, taken when ts_subtree_error_cost is
	// positive on both sides); and only then the field comparison this class
	// represents (padding, size, child count, extra, external scanner state).
	// The shortcut is deliberately not ported -- see
	// spec.derivation-set-equivalence.v1, which scopes it out until error-path
	// equivalence lands.
	//
	// Error payloads can now reach this class through S3 and S5. The omitted
	// shortcut keeps alternatives that C can merge. That difference is
	// fail-closed: recovery pricing rejects an ambiguous compact head instead
	// of publishing one guessed alternative. Exact error-path equivalence
	// remains outside spec.derivation-set-equivalence.v1.
	//
	// External payloads separately require exact per-token scanner provenance
	// or a stable language certificate.
	if payload.external && !c.externalPayloadsQuiescent {
		_, exact, err := c.subtreeExternalProvenance(payloadID)
		if err != nil || !exact {
			return shallowPayloadClass{}, false, err
		}
	}
	if payload.startByte < prev.byteOffset || payload.endByte < payload.startByte {
		return shallowPayloadClass{}, false, errors.New("parser-core phase zero: invalid shallow payload extent")
	}
	return shallowPayloadClass{
		symbol: payload.symbol, padding: payload.startByte - prev.byteOffset,
		size: payload.endByte - payload.startByte, childCount: payload.childCount,
		extra: payload.extra, external: payload.external,
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
	rebuilt := slices.Clone(oldLinks)
	rebuilt[candidate].prev = in.prev
	rebuilt[candidate].payload = in.payload
	rebuilt[candidate].scoreDelta = in.scoreDelta
	rebuilt[candidate].order = in.order.Value
	rebuilt[candidate].flags &^= linkFlagHasOrder
	if in.order.Present {
		rebuilt[candidate].flags |= linkFlagHasOrder
	}
	replacementWitness := precedenceMaximumWitness{
		replacement: rebuilt[candidate], hasReplacement: true,
	}
	maximum, err := c.computePrecedenceMaximum(rebuilt, replacementWitness)
	if err != nil {
		return Head{}, err
	}

	linkMark := len(c.links)
	linkRefMark := len(c.dropCohortLinkRefIndexes)
	var first LinkID
	for _, stored := range rebuilt {
		copy := stored
		copy.next = first
		c.appendGraphLink(copy)
		c.addWork(&c.work.GraphLinkAdditionsProxy, 1)
		first = LinkID(len(c.links))
	}
	id, err := c.appendNodeAtWithMaximum(nodeRecord{
		state: key.state, byteOffset: key.byteOffset,
		firstLink: uint32(first), linkCount: uint32(len(rebuilt)), pathCount: old.pathCount,
	}, key.checkpoint, maximum.value)
	if err != nil {
		c.links = c.links[:linkMark]
		if c.dropCohortLinkRefIndexes != nil {
			c.dropCohortLinkRefIndexes = c.dropCohortLinkRefIndexes[:linkRefMark]
		}
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
	cleanPathRank CleanPathRankSelection
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
	external   []SubtreeID
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
	s.external = s.external[:0]
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
	s.external = s.external[:0]
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

type cleanPathRank struct {
	score int64
	depth uint64
}

type cleanPathRankAccumulator struct {
	best       cleanPathRank
	winner     int
	found      bool
	crossTie   bool
	pathIndex  int
	windowRank cleanPathRank
}

func compareCleanPathRank(left, right cleanPathRank) int {
	if left.score != right.score {
		if left.score > right.score {
			return 1
		}
		return -1
	}
	if left.depth != right.depth {
		if left.depth > right.depth {
			return 1
		}
		return -1
	}
	return 0
}

func (a *cleanPathRankAccumulator) observe(prefixScore int64, prefixDepth uint64) bool {
	score, err := checkedAddScore(prefixScore, a.windowRank.score)
	if err != nil {
		return false
	}
	candidate := cleanPathRank{
		score: score,
		depth: prefixDepth + a.windowRank.depth,
	}
	switch {
	case !a.found || compareCleanPathRank(candidate, a.best) > 0:
		a.best = candidate
		a.winner = a.pathIndex
		a.found = true
		a.crossTie = false
	case compareCleanPathRank(candidate, a.best) == 0 && a.winner != a.pathIndex:
		a.crossTie = true
	}
	return true
}

// markCleanProductionRank selects one clean multi-pop path by production's
// same-boundary rank: higher cumulative dynamic precedence, then greater
// physical stack depth. Accepted and shifted status are equal for paths in one
// classified reduction. An exact cross-path tie stays unknown.
//
// The walk reuses popScratch's existing traversal storage. It does not publish
// arena data or allocate selector-owned storage.
func (c *Core) markCleanProductionRank(paths []popPath) {
	if len(paths) < 2 {
		return
	}
	for index := range paths {
		paths[index].cleanPathRank = CleanPathRankUnselected
	}
	scratch := &c.popScratch
	scratch.finishTraversal()
	defer scratch.finishTraversal()

	var rank cleanPathRankAccumulator
	for index := range paths {
		path := &paths[index]
		for _, payload := range path.children {
			hasExternal, err := c.cleanPathPayloadHasExternal(payload)
			if err != nil || hasExternal {
				markCleanPathRankUnknown(paths)
				return
			}
		}
		for _, trailing := range path.trailing {
			hasExternal, err := c.cleanPathPayloadHasExternal(trailing.payload)
			if err != nil || hasExternal {
				markCleanPathRankUnknown(paths)
				return
			}
		}
		prefix, err := c.node(path.prev)
		if err != nil || prefix.pathCount == math.MaxUint64 ||
			prefix.pathCount > c.limits.MaxDerivations {
			markCleanPathRankUnknown(paths)
			return
		}
		rank.pathIndex = index
		rank.windowRank = cleanPathRank{
			score: path.score,
			depth: uint64(len(path.children) + len(path.trailing)),
		}
		ok, err := c.walkCleanPrefixRanks(path.prev, &rank)
		if err != nil || !ok {
			markCleanPathRankUnknown(paths)
			return
		}
	}
	if !rank.found || rank.crossTie {
		markCleanPathRankUnknown(paths)
		return
	}
	paths[rank.winner].cleanPathRank = CleanPathRankSelected
}

func markCleanPathRankUnknown(paths []popPath) {
	for index := range paths {
		paths[index].cleanPathRank = CleanPathRankUnknown
	}
}

func (c *Core) cleanPathPayloadHasExternal(root SubtreeID) (bool, error) {
	if c.externalPayloadsQuiescent {
		return false, nil
	}
	if root == 0 {
		return false, errors.New("parser-core phase zero: clean path has no payload")
	}
	stack := c.popScratch.external[:0]
	stack = append(stack, root)
	c.popScratch.external = stack
	visited := uint64(0)
	for len(stack) != 0 {
		last := len(stack) - 1
		id := stack[last]
		stack = stack[:last]
		record, err := c.subtree(id)
		if err != nil {
			c.popScratch.external = stack[:0]
			return false, err
		}
		visited++
		if visited > uint64(c.limits.MaxSubtrees) {
			c.popScratch.external = stack[:0]
			return false, errors.New("parser-core phase zero: clean path external payload walk cap")
		}
		if record.external {
			c.popScratch.external = stack[:0]
			return true, nil
		}
		for _, child := range c.children[record.firstChild : record.firstChild+record.childCount] {
			if child == 0 || child >= id {
				c.popScratch.external = stack[:0]
				return false, errors.New("parser-core phase zero: clean path subtree order is invalid")
			}
			stack = append(stack, child)
		}
		c.popScratch.external = stack
	}
	c.popScratch.external = stack[:0]
	return false, nil
}

// walkCleanPrefixRanks visits each retained prefix derivation without a map.
// The persistent graph is an append-only directed acyclic graph. Existing
// link-frame and score scratch provide the iterative traversal stack.
func (c *Core) walkCleanPrefixRanks(root NodeID, rank *cleanPathRankAccumulator) (bool, error) {
	scratch := &c.popScratch
	scratch.finishTraversal()
	id := root
	score := int64(0)
	depth := uint64(0)
	active := 0

descend:
	for {
		node, err := c.node(id)
		if err != nil {
			return false, err
		}
		switch node.linkCount {
		case 0:
			if !rank.observe(score, depth) {
				return false, nil
			}
			break descend
		case 1:
			if node.firstLink == 0 || uint64(node.firstLink) > uint64(len(c.links)) {
				return false, errors.New("parser-core phase zero: clean path link is out of range")
			}
			link := c.links[node.firstLink-1]
			if link.next != 0 {
				return false, errors.New("parser-core phase zero: clean path single link has a successor")
			}
			hasExternal, err := c.cleanPathPayloadHasExternal(link.payload)
			if err != nil || hasExternal {
				return false, err
			}
			score, err = checkedAddScore(score, link.scoreDelta)
			if err != nil {
				return false, nil
			}
			depth++
			id = link.prev
		default:
			if len(scratch.linkFrames) <= active {
				scratch.linkFrames = append(scratch.linkFrames, nil)
			}
			links, err := c.nodeLinksInto(scratch.linkFrames[active], *node)
			if err != nil {
				return false, err
			}
			scratch.linkFrames[active] = links
			if len(scratch.revOrders) == active {
				scratch.revOrders = append(scratch.revOrders, ForkOrder{})
			} else {
				scratch.revOrders[active] = ForkOrder{}
				scratch.revOrders = scratch.revOrders[:active+1]
			}
			scoreIndex := active * 2
			if len(scratch.revScores) == scoreIndex {
				scratch.revScores = append(scratch.revScores, score, int64(depth))
			} else {
				scratch.revScores[scoreIndex] = score
				scratch.revScores[scoreIndex+1] = int64(depth)
				scratch.revScores = scratch.revScores[:scoreIndex+2]
			}
			active++
			break descend
		}
	}

	for active != 0 {
		frameIndex := active - 1
		frame := scratch.linkFrames[frameIndex]
		cursor := int(scratch.revOrders[frameIndex].Value)
		if cursor >= len(frame) {
			scratch.linkFrames[frameIndex] = frame[:0]
			scratch.revScores = scratch.revScores[:frameIndex*2]
			scratch.revOrders = scratch.revOrders[:frameIndex]
			active--
			continue
		}
		link := frame[cursor]
		scratch.revOrders[frameIndex].Value++
		var err error
		hasExternal, err := c.cleanPathPayloadHasExternal(link.payload)
		if err != nil || hasExternal {
			return false, err
		}
		score, err = checkedAddScore(scratch.revScores[frameIndex*2], link.scoreDelta)
		if err != nil {
			return false, nil
		}
		depth = uint64(scratch.revScores[frameIndex*2+1]) + 1
		id = link.prev
		goto descend
	}
	return true, nil
}

func mergeCleanPathRank(left, right CleanPathRankSelection) CleanPathRankSelection {
	switch {
	case left == CleanPathRankUnknown || right == CleanPathRankUnknown:
		return CleanPathRankUnknown
	case left == CleanPathRankSelected || right == CleanPathRankSelected:
		return CleanPathRankSelected
	case left == CleanPathRankUnselected || right == CleanPathRankUnselected:
		return CleanPathRankUnselected
	default:
		return CleanPathRankNotApplicable
	}
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
	n, err := c.node(head.Node)
	if err != nil {
		return nil, err
	}
	if n.pathCount == 1 {
		path, exact, err := c.singleDerivation(head.Node)
		if err != nil {
			return nil, err
		}
		if exact {
			return []Derivation{path}, nil
		}
	}

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

// singleDerivation walks a certified single-path graph without recursion.
// The returned Boolean is false when malformed path telemetry requires the
// general enumerator to reproduce its fail-closed result.
func (c *Core) singleDerivation(id NodeID) (Derivation, bool, error) {
	reverseLinks := make([]LinkID, 0, 64)
	for {
		n, err := c.node(id)
		if err != nil {
			return Derivation{}, true, err
		}
		if n.linkCount == 0 {
			if n.pathCount != 1 {
				return Derivation{}, true, errors.New("parser-core phase zero: malformed seed path count")
			}
			break
		}
		if n.pathCount != 1 || n.linkCount != 1 {
			return Derivation{}, false, nil
		}

		linkID := LinkID(n.firstLink)
		if linkID == 0 {
			return Derivation{}, true, errors.New("parser-core phase zero: adjacency shorter than recorded link count")
		}
		if uint64(linkID) > uint64(len(c.links)) {
			return Derivation{}, true, errors.New("parser-core phase zero: link adjacency out of range")
		}
		link := c.links[linkID-1]
		if link.next != 0 {
			if link.next == linkID {
				return Derivation{}, true, errors.New("parser-core phase zero: adjacency cycle")
			}
			if uint64(link.next) > uint64(len(c.links)) {
				return Derivation{}, true, errors.New("parser-core phase zero: link adjacency out of range")
			}
			return Derivation{}, true, errors.New("parser-core phase zero: adjacency exceeds recorded link count")
		}
		reverseLinks = append(reverseLinks, linkID)
		if len(reverseLinks) >= len(c.nodes) {
			return Derivation{}, true, errors.New("parser-core phase zero: graph cycle")
		}
		id = link.prev
	}

	path := Derivation{Payloads: make([]SubtreeID, len(reverseLinks))}
	for reverseIndex := len(reverseLinks) - 1; reverseIndex >= 0; reverseIndex-- {
		link := c.links[reverseLinks[reverseIndex]-1]
		score, err := checkedAddScore(path.Score, link.scoreDelta)
		if err != nil {
			return Derivation{}, true, err
		}
		path.Score = score
		path.Payloads[len(reverseLinks)-1-reverseIndex] = link.payload
		if link.hasOrder() {
			path.BranchOrder = link.order
			path.HasBranchOrder = true
		}
	}
	return path, true, nil
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
		Missing: r.missing,
	}
	view.LexerSkippedPrefixStart, view.LexerSkippedPrefix = c.lexerSkippedPrefix(id)
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
	var scratch MaterializationPostorderScratch
	return c.VisitMaterializationPostorderWithScratch(roots, poll, &scratch, visit)
}

// MaterializationView returns the borrowed materialization view for one
// subtree id. It is the random-access companion to VisitMaterializationPostorder
// used by ParseState-by-table-replay (Phase-3 Lane 2): the driver walks the
// derivation top-down to reconstruct per-node parser states before the
// postorder pass materializes public nodes. The returned Children and Aliases
// slices reference compact arena storage and must not be retained or mutated.
func (c *Core) MaterializationView(id SubtreeID) (MaterializationSubtreeView, error) {
	record, err := c.subtree(id)
	if err != nil {
		return MaterializationSubtreeView{}, err
	}
	view := MaterializationSubtreeView{
		Symbol:            record.symbol,
		ProductionID:      record.productionID,
		DynamicPrecedence: record.dynamicPrecedence,
		StartByte:         record.startByte,
		EndByte:           record.endByte,
		Children:          c.children[record.firstChild : record.firstChild+record.childCount],
		Aliases:           c.aliases[record.firstAlias : record.firstAlias+record.aliasCount],
		Extra:             record.extra,
		External:          record.external,
		Terminal:          record.terminal,
		Fragile:           record.fragile,
		Missing:           record.missing,
	}
	if record.terminal {
		if provenance, ok := c.externalPayloadScannerProvenance(id); ok {
			view.ExternalScannerCheckpointStart = provenance.start
			view.ExternalScannerCheckpointEnd = provenance.end
			view.ExternalScannerCheckpointExact = true
		}
	}
	view.LexerSkippedPrefixStart, view.LexerSkippedPrefix = c.lexerSkippedPrefix(id)
	return view, nil
}

// SubtreeArenaLen returns the number of subtree records currently allocated,
// used to size a per-id replay-state array (ids are 1-based, so callers size
// SubtreeArenaLen()+1).
func (c *Core) SubtreeArenaLen() int {
	if c == nil {
		return 0
	}
	return len(c.subtrees)
}

func (c *Core) validateMaterializationMetadata(id SubtreeID, record subtreeRecord) error {
	// Production construction authenticates every terminal and reduction before
	// publication. Compact arenas are immutable, so repeating the table remap at
	// materialization adds no evidence. The flag is Core-scoped so subtreeRecord
	// remains byte-for-byte unchanged on the scheduler's hot equality/copy path.
	if c.metadataConstructionAuthenticated {
		return nil
	}
	if c.isRecoverEOFAcceptRoot(id) {
		if record.symbol != ErrorRegionSymbol || record.extra || record.terminal ||
			record.fieldCount != 0 || record.aliasCount != 0 {
			return errors.New("parser-core phase zero: malformed recover_eof root provenance")
		}
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
	return c.appendNodeAt(r, c.checkpoint)
}

func (c *Core) appendNodeAt(r nodeRecord, checkpoint CheckpointID) (NodeID, error) {
	if r.linkCount == 0 {
		r.precedenceMax = 0
	} else {
		maximum, err := c.computePrecedenceMaximumFromNode(r, precedenceMaximumWitness{})
		if err != nil {
			return 0, err
		}
		r.precedenceMax = maximum.value
	}
	return c.appendNodeRecord(r, checkpoint)
}

func (c *Core) appendNodeAtWithMaximum(r nodeRecord, checkpoint CheckpointID, maximum int64) (NodeID, error) {
	r.precedenceMax = maximum
	return c.appendNodeRecord(r, checkpoint)
}

func (c *Core) appendNodeRecord(r nodeRecord, checkpoint CheckpointID) (NodeID, error) {
	if uint64(len(c.nodes))+1 > uint64(c.limits.MaxNodes) || uint64(len(c.nodes)) >= math.MaxUint32 {
		return 0, errors.New("parser-core phase zero: node arena cap")
	}
	if !c.externalPayloadsQuiescent && checkpoint != 0 {
		if _, ok := c.checkpoints.record(checkpoint); !ok {
			return 0, errors.New("parser-core phase zero: node scanner checkpoint is unavailable")
		}
	}
	next := NodeID(uint64(len(c.nodes)) + 1)
	if err := c.validatePublishedNodeDAG(r, next); err != nil {
		return 0, err
	}
	c.nodes = append(c.nodes, r)
	c.nodeLineages = append(c.nodeLineages, nodeLineageRecord{})
	if !c.externalPayloadsQuiescent {
		c.nodeCheckpoints = append(c.nodeCheckpoints, checkpoint)
	}
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

func (c *Core) appendAuthenticatedTerminal(
	r subtreeRecord,
	lexerSkippedPrefixLength uint16,
) (SubtreeID, error) {
	// The AST provenance ratchet requires every caller to pass a subtreeRecord
	// literal with terminal:true. Publish the terminal once. Then append its
	// sparse scanner and lexer proofs when the current election supplied them.
	if !r.terminal {
		return 0, errors.New("parser-core phase zero: authenticated terminal is not terminal")
	}
	if lexerSkippedPrefixLength != 0 {
		if r.external || uint32(lexerSkippedPrefixLength) > r.startByte {
			return 0, errors.New("parser-core phase zero: invalid lexer skipped-prefix provenance")
		}
	}
	payload, err := c.appendSubtreeRecord(r, nil, nil, nil)
	if err != nil {
		return 0, err
	}
	if (r.external || c.terminalScannerCheckpointProvenance) && c.externalTokenScannerExact {
		c.externalProvenance = append(c.externalProvenance, externalPayloadProvenance{
			payload: payload,
			start:   c.externalTokenScannerStart,
			end:     c.externalTokenScannerEnd,
		})
		if r.external && !c.externalPayloadsQuiescent {
			c.subtrees[payload-1].externalProvenanceState = subtreeExternalProvenanceExactHasExternal
		}
	}
	if lexerSkippedPrefixLength != 0 {
		c.lexerSkippedPrefixes = append(c.lexerSkippedPrefixes, lexerSkippedPrefixProvenance{
			payload: payload,
			start:   r.startByte - uint32(lexerSkippedPrefixLength),
		})
	}
	return payload, nil
}

func (c *Core) lexerSkippedPrefix(payload SubtreeID) (uint32, bool) {
	low, high := 0, len(c.lexerSkippedPrefixes)
	for low < high {
		mid := low + (high-low)/2
		if c.lexerSkippedPrefixes[mid].payload < payload {
			low = mid + 1
		} else {
			high = mid
		}
	}
	if low >= len(c.lexerSkippedPrefixes) || c.lexerSkippedPrefixes[low].payload != payload {
		return 0, false
	}
	return c.lexerSkippedPrefixes[low].start, true
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
	r.externalProvenanceState = c.deriveSubtreeExternalProvenanceState(r, children)
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

func (c *Core) nodeLineage(id NodeID) (*nodeLineageRecord, error) {
	if id == 0 || uint64(id) > uint64(len(c.nodeLineages)) {
		return nil, fmt.Errorf("parser-core phase zero: invalid node lineage id %d", id)
	}
	return &c.nodeLineages[id-1], nil
}

// NodeLineageAlternativeSet returns the currently recorded alternative set
// for id. A dead (superseded) node keeps its lineage record for the rest of
// the parse, so this also resolves historical membership for dead-node
// import (spec.b4b-alternative-set.v1 section 4, "Dead-node historical
// import").
func (c *Core) NodeLineageAlternativeSet(id NodeID) (AlternativeSet, error) {
	record, err := c.nodeLineage(id)
	if err != nil {
		return AlternativeSet{}, err
	}
	return record.set, nil
}

// alternativeSetMembers resolves set's sorted member slice, reading through
// the shared spill arena when set has spilled. The returned slice aliases
// Core-owned storage and is invalidated by the next mutation to any set; it
// never allocates and never writes.
func (c *Core) alternativeSetMembers(set AlternativeSet) ([]uint32, bool) {
	if !set.spilled() {
		return set.inline[:set.count], true
	}
	if set.spillRef == 0 {
		return nil, set.count == 0
	}
	start := int(set.spillRef) - 1
	end := start + int(set.count)
	if start < 0 || end > len(c.alternativeSpillArena) {
		return nil, false
	}
	return c.alternativeSpillArena[start:end], true
}

// AlternativeSetMembers is the exported form of alternativeSetMembers for
// cross-package readers (the shadow predicate and census in
// parsercore_phase0_driver.go). ok is false only when set's spill reference
// is out of range for the current arena; every recording path in this
// package keeps that from happening, so callers treat !ok as a fail-closed
// signal, not a recoverable case.
func (c *Core) AlternativeSetMembers(set AlternativeSet) ([]uint32, bool) {
	return c.alternativeSetMembers(set)
}

// searchAlternativeSetMembers returns the sorted insertion position for
// member within the already-sorted, deduplicated members, and whether member
// is already present. Linear scan is deliberate: members is bounded by
// alternativeSetHardCap (32), where a branch-predictable scan outperforms a
// callback-based binary search and never allocates.
func searchAlternativeSetMembers(members []uint32, member uint32) (int, bool) {
	for index, existing := range members {
		if existing == member {
			return index, true
		}
		if existing > member {
			return index, false
		}
	}
	return len(members), false
}

// alternativeSetInsert inserts one member into set in place and reports
// whether the set changed. It is a no-op for member==0 (reserved) or an
// already-present member. Every mutation is append-only at the byte level:
// positions below the pre-call count are never rewritten. Three cases, from
// cheapest to most expensive:
//
//  1. Pure ascending tail append while still inline: writes only the new
//     inline slot.
//  2. Pure ascending tail append while spilled, and this set's segment is
//     still the live tail of Core.alternativeSpillArena (nothing else has
//     grown the arena since): extends the segment and the arena together
//     with one appended element, O(1) amortized. This is the common case --
//     a header or node accumulating its own establishment/union history in
//     temporal (hence ascending) order, spec.b4b-alternative-set.v1 section
//     3.1's monotonic lineage-id allocation -- and is what keeps repeated
//     per-dispatch persistence (persistHeaderLineageOwned) cheap on a long
//     parse instead of re-copying a growing segment on every call.
//  3. Anything else (inline growth beyond capacity, an insertion that is not
//     the new maximum, or a spilled segment that is no longer the arena's
//     tail): writes a complete fresh sorted copy to a new segment at the
//     current end of the arena, leaving prior storage untouched and unread
//     from then on.
//
// Case 3's untouched-prior-storage property (shared with cases 1 and 2) is
// what lets the journal restore an exact prior set by truncating
// count/flags/spillRef alone (section 3.3); a superseded segment simply
// leaks arena space until the next Reset, bounded by alternativeSetHardCap
// per record.
//
// Overflowed is frozen, uniformly. Once alternativeSetFlagOverflowed is set
// -- whether by this function's own count reaching alternativeSetHardCap,
// or by alternativeSetUnion propagating an overflowed source's incomplete
// knowledge onto set (below) -- no further insert can ever change set
// again: the check runs first, before any member resolution or scan, so a
// set that stays overflowed for the rest of a parse (the canonical corpus
// census found this true for 98.7% of elections at (event, branch) member
// width) costs one flag test per call instead of a spill-arena read and a
// linear scan that would always end in the identical no-op.
func (c *Core) alternativeSetInsert(set *AlternativeSet, member uint32) bool {
	if member == 0 {
		return false
	}
	if set.flags&alternativeSetFlagOverflowed != 0 {
		return false
	}
	current, ok := c.alternativeSetMembers(*set)
	if !ok {
		return false
	}
	position, found := searchAlternativeSetMembers(current, member)
	if found {
		return false
	}
	if int(set.count) >= alternativeSetHardCap {
		set.flags |= alternativeSetFlagOverflowed
		return true
	}
	if position == len(current) {
		switch {
		case !set.spilled() && len(current) < alternativeSetInlineCapacity:
			set.inline[len(current)] = member
			set.count++
			return true
		case set.spilled() && int(set.spillRef)-1+len(current) == len(c.alternativeSpillArena):
			c.alternativeSpillArena = append(c.alternativeSpillArena, member)
			set.count++
			return true
		}
	}
	start := len(c.alternativeSpillArena)
	c.alternativeSpillArena = append(c.alternativeSpillArena, current[:position]...)
	c.alternativeSpillArena = append(c.alternativeSpillArena, member)
	c.alternativeSpillArena = append(c.alternativeSpillArena, current[position:]...)
	set.spillRef = uint32(start) + 1
	set.count = uint8(len(current) + 1)
	set.flags |= alternativeSetFlagSpilled
	return true
}

// alternativeSetUnion unions src's recorded members into *dst in place and
// reports whether dst changed. Overflowed is frozen, uniformly (see
// alternativeSetInsert's doc comment): this function never changes an
// already-overflowed dst, and never partially merges an overflowed src's
// known members before freezing dst -- an overflowed source's recorded
// members are an incomplete view of its true membership (spec.b4b-
// alternative-set.v1 section 3.2), so the only sound conclusion is that
// dst's own true union is unknowable beyond what dst already has recorded.
// dst becomes overflowed-and-done at exactly its pre-union state, rather
// than spending a merge loop whose result is discarded the moment the flag
// is set regardless. Checked first, in both directions, before any member
// resolution: on the hot, already-saturated path (98.7% of canonical-corpus
// elections touch an overflowed set), this turns a spill-arena read plus a
// linear scan into one flag test.
//
// Header-to-node persistence (persistHeaderLineageOwned,
// parsercore_phase0_driver.go) re-unions every convergedReductionSplit
// header's accumulated set into its node on every dispatch, mirroring the
// existing scalar RecordHeadLineageOwned call at the same frequency, and
// header.head moves to a freshly allocated node on most dispatches -- so the
// destination is empty far more often than it already equals src. Two more
// fast paths keep that pattern cheap once both sides are known unfrozen:
//
//  1. dst is empty: *dst = src, an O(1) value copy. This aliases src's spill
//     segment (if any) rather than copying it, which is safe: every further
//     mutation to either set only ever appends past its own recorded count
//     (alternativeSetInsert's append-only invariant), so neither set's
//     existing view is ever disturbed by growth on the other.
//  2. dst already contains every member of src (the steady state once a
//     header's set stops growing): one O(len(src)+len(dst)) merge-scan
//     (containsAll) instead of insert's O(len(src)) x O(len(dst))
//     member-by-member scan.
func (c *Core) alternativeSetUnion(dst *AlternativeSet, src AlternativeSet) bool {
	if dst.flags&alternativeSetFlagOverflowed != 0 {
		return false
	}
	if src.Overflowed() {
		dst.flags |= alternativeSetFlagOverflowed
		return true
	}
	if src.count == 0 {
		return false
	}
	if dst.count == 0 {
		*dst = src
		return true
	}
	srcMembers, ok := c.alternativeSetMembers(src)
	if !ok {
		return false
	}
	if dstMembers, dstOK := c.alternativeSetMembers(*dst); dstOK &&
		alternativeSetSortedContainsAll(srcMembers, dstMembers) {
		return false
	}
	changed := false
	for _, member := range srcMembers {
		if c.alternativeSetInsert(dst, member) {
			changed = true
		}
	}
	return changed
}

// alternativeSetSortedContainsAll reports whether every member of needle is
// present in haystack. Both slices are sorted ascending (AlternativeSet's
// section 3.2 invariant), so this is a single merge-scan, never allocating.
// A cheap O(1) range check on the first and last elements proves
// non-containment (and so skips the O(len(needle)+len(haystack)) scan
// entirely) whenever needle reaches outside haystack's covered range --
// sound in both directions, since haystack sorted implies every member of
// needle must fall within [haystack[0], haystack[len-1]] to be contained.
func alternativeSetSortedContainsAll(needle, haystack []uint32) bool {
	if len(needle) > len(haystack) {
		return false
	}
	if len(needle) == 0 {
		return true
	}
	if needle[0] < haystack[0] || needle[len(needle)-1] > haystack[len(haystack)-1] {
		return false
	}
	haystackIndex := 0
	for _, member := range needle {
		for haystackIndex < len(haystack) && haystack[haystackIndex] < member {
			haystackIndex++
		}
		if haystackIndex >= len(haystack) || haystack[haystackIndex] != member {
			return false
		}
		haystackIndex++
	}
	return true
}

// UnionAlternativeSet is the exported form of alternativeSetUnion for
// cross-package writers (parsercore_phase0_driver.go's header-scratch
// propagation sites: canonicalize fold, sibling adoption, dead-node import).
// Zero-alloc once c's shared spill arena has warmed to the parse's
// high-water mark, matching nodeLineageJournal and popScratch.
func (c *Core) UnionAlternativeSet(dst *AlternativeSet, src AlternativeSet) bool {
	return c.alternativeSetUnion(dst, src)
}

// AlternativeSetIncomparable reports whether a and b are incomparable under
// containment -- neither's recorded members are a subset of the other's
// (spec.b4b-alternative-set.v2 section 3.4). Every fold-class union site
// (every AlternativeSet union except establishment's own first insert into a
// fresh output) calls this before the union to decide whether the
// destination's blended mark must become true. It costs one extra
// merge-scan beyond the union itself, bounded by alternativeSetHardCap, paid
// only on the already-cold fold path -- extend (establishment) sites never
// call it. An unresolvable spill reference on either side reads as an empty
// member slice, which is always comparable (subset of anything): blended
// computation fails toward "not blended" on that unreachable case, since the
// predicate's own fail-closed containment check (not this helper) is what
// guards soundness against an unresolvable set.
func (c *Core) AlternativeSetIncomparable(a, b AlternativeSet) bool {
	aMembers, _ := c.alternativeSetMembers(a)
	bMembers, _ := c.alternativeSetMembers(b)
	if alternativeSetSortedContainsAll(aMembers, bMembers) {
		return false
	}
	if alternativeSetSortedContainsAll(bMembers, aMembers) {
		return false
	}
	return true
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
