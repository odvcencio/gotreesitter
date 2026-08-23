//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
	"unsafe"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

const (
	g18ReferenceMaxCohorts  = 8
	g18ReferenceMaxMembers  = 8
	g18ReferenceInlineLimit = 2
)

type g18ReferenceCohortID struct {
	ArenaOwner     uint64
	ArenaEpoch     uint64
	CohortSequence uint64
}

type g18ReferenceCertificateState uint8

const (
	g18ReferenceCertificateBuilding g18ReferenceCertificateState = iota + 1
	g18ReferenceCertificateComplete
	g18ReferenceCertificateOverflowed
	g18ReferenceCertificateBlended
	g18ReferenceCertificateUnproved
)

type g18ReferenceActionIdentity struct {
	State             core.StateID
	Lookahead         core.Symbol
	Ordinal           uint16
	Type              core.ActionType
	TargetState       core.StateID
	Symbol            core.Symbol
	ProductionID      uint16
	ChildCount        uint8
	DynamicPrecedence int16
	Extra             bool
	ExtraChain        bool
	Repetition        bool
	NoLookahead       bool
	SelectionClass    uint8
}

type g18ReferenceDerivationIdentity struct {
	InternedRecord uint32
	RootSymbol     core.Symbol
	StackDepth     uint32
	Digest         [sha256.Size]byte
	Record         [64]byte
	RecordLength   uint8
}

type g18ReferenceCohortMember struct {
	Branch     uint16
	Action     g18ReferenceActionIdentity
	Derivation g18ReferenceDerivationIdentity
}

type g18ReferenceCertificate struct {
	ID              g18ReferenceCohortID
	State           g18ReferenceCertificateState
	ExpectedMembers uint8
	WrittenMembers  uint8
	Spilled         bool
	Members         [g18ReferenceMaxMembers]g18ReferenceCohortMember
}

type g18ReferenceAccounting struct {
	ActionRecords         uint16
	DerivationRecords     uint16
	CertificateReferences uint16
	MapEntries            uint16
	InternerEntries       uint16
	JournalEntries        uint16
	StorageBytes          uint64
}

type g18ReferenceLimits struct {
	MaxActionRecords         uint16
	MaxDerivationRecords     uint16
	MaxCertificateReferences uint16
	MaxMapEntries            uint16
	MaxInternerEntries       uint16
	MaxJournalEntries        uint16
	MaxStorageBytes          uint64
}

type g18ReferenceCertificateHandle struct {
	ID    g18ReferenceCohortID
	Index uint8
}

type g18ReferenceHeadProof struct {
	Head        core.Head
	Certificate g18ReferenceCertificateHandle
	Member      g18ReferenceCohortMember
}

type g18ReferenceCertificateArena struct {
	Owner        uint64
	Epoch        uint64
	NextSequence uint64
	Count        uint8
	// OwnerCheckedLookups counts every owner-check attempt, including a
	// rejected foreign or stale handle. It is not a storage-read counter.
	OwnerCheckedLookups uint64
	Used                g18ReferenceAccounting
	Limits              g18ReferenceLimits
	Certificates        [g18ReferenceMaxCohorts]g18ReferenceCertificate
}

func g18ReferenceDefaultLimits() g18ReferenceLimits {
	return g18ReferenceLimits{
		MaxActionRecords: 64, MaxDerivationRecords: 64, MaxCertificateReferences: 64,
		MaxMapEntries: 64, MaxInternerEntries: 64, MaxJournalEntries: 96,
		MaxStorageBytes: 1 << 20,
	}
}

func g18ReferencePlan(expected uint8) g18ReferenceAccounting {
	return g18ReferenceAccounting{
		ActionRecords: uint16(expected), DerivationRecords: uint16(expected),
		CertificateReferences: uint16(expected) + 1, MapEntries: uint16(expected),
		InternerEntries: uint16(expected), JournalEntries: uint16(expected) + 2,
		StorageBytes: uint64(unsafe.Sizeof(g18ReferenceCertificate{})) +
			uint64(expected)*uint64(unsafe.Sizeof(g18ReferenceCohortMember{})),
	}
}

func g18ReferenceNewArena(owner uint64) g18ReferenceCertificateArena {
	return g18ReferenceCertificateArena{Owner: owner, Limits: g18ReferenceDefaultLimits()}
}

func g18ReferenceAllocateOwner(counter *uint64) (uint64, error) {
	if counter == nil || *counter == math.MaxUint64 {
		return 0, errors.New("arena owner overflow")
	}
	*counter = *counter + 1
	if *counter == 0 {
		return 0, errors.New("arena owner is zero")
	}
	return *counter, nil
}

func (arena *g18ReferenceCertificateArena) beginSession() error {
	if arena.Owner == 0 {
		return errors.New("arena owner is zero")
	}
	if arena.Epoch == math.MaxUint64 {
		return errors.New("arena epoch overflow")
	}
	arena.Epoch++
	if arena.Epoch == 0 {
		return errors.New("arena epoch is zero")
	}
	arena.NextSequence = 0
	arena.Count = 0
	arena.Used = g18ReferenceAccounting{}
	return nil
}

func (arena *g18ReferenceCertificateArena) newCertificate(
	expected uint8,
	plan g18ReferenceAccounting,
) (g18ReferenceCertificateHandle, error) {
	var zero g18ReferenceCertificateHandle
	if arena.Epoch == 0 {
		return zero, errors.New("certificate arena is inactive")
	}
	if arena.NextSequence == math.MaxUint64 {
		return zero, errors.New("cohort sequence overflow")
	}
	if int(arena.Count) == len(arena.Certificates) {
		return zero, errors.New("certificate cohort cap")
	}
	if expected == 0 || int(expected) > g18ReferenceMaxMembers {
		return zero, errors.New("certificate member cap")
	}
	if plan.ActionRecords < uint16(expected) || plan.DerivationRecords < uint16(expected) ||
		plan.CertificateReferences < uint16(expected)+1 || plan.MapEntries < uint16(expected) ||
		plan.InternerEntries < uint16(expected) || plan.JournalEntries < uint16(expected)+2 ||
		plan.StorageBytes < g18ReferencePlan(expected).StorageBytes {
		return zero, errors.New("certificate preflight is incomplete")
	}
	if !arena.fits(plan) {
		return zero, errors.New("certificate preflight exceeds a limit")
	}
	arena.NextSequence++
	index := arena.Count
	certificate := &arena.Certificates[index]
	*certificate = g18ReferenceCertificate{
		ID: g18ReferenceCohortID{
			ArenaOwner: arena.Owner, ArenaEpoch: arena.Epoch, CohortSequence: arena.NextSequence,
		},
		State: g18ReferenceCertificateBuilding, ExpectedMembers: expected,
		Spilled: expected > g18ReferenceInlineLimit,
	}
	arena.Count++
	arena.Used = arena.Used.plus(plan)
	return g18ReferenceCertificateHandle{ID: certificate.ID, Index: index}, nil
}

func (arena *g18ReferenceCertificateArena) storageBytes() uint64 {
	return arena.Used.StorageBytes
}

func (arena *g18ReferenceCertificateArena) footprintBytes() uint64 {
	if arena == nil {
		return 0
	}
	return uint64(unsafe.Sizeof(*arena))
}

func (value g18ReferenceAccounting) plus(other g18ReferenceAccounting) g18ReferenceAccounting {
	return g18ReferenceAccounting{
		ActionRecords:         value.ActionRecords + other.ActionRecords,
		DerivationRecords:     value.DerivationRecords + other.DerivationRecords,
		CertificateReferences: value.CertificateReferences + other.CertificateReferences,
		MapEntries:            value.MapEntries + other.MapEntries,
		InternerEntries:       value.InternerEntries + other.InternerEntries,
		JournalEntries:        value.JournalEntries + other.JournalEntries,
		StorageBytes:          value.StorageBytes + other.StorageBytes,
	}
}

func (arena *g18ReferenceCertificateArena) fits(plan g18ReferenceAccounting) bool {
	next := arena.Used.plus(plan)
	return next.ActionRecords >= arena.Used.ActionRecords && next.ActionRecords <= arena.Limits.MaxActionRecords &&
		next.DerivationRecords >= arena.Used.DerivationRecords && next.DerivationRecords <= arena.Limits.MaxDerivationRecords &&
		next.CertificateReferences >= arena.Used.CertificateReferences && next.CertificateReferences <= arena.Limits.MaxCertificateReferences &&
		next.MapEntries >= arena.Used.MapEntries && next.MapEntries <= arena.Limits.MaxMapEntries &&
		next.InternerEntries >= arena.Used.InternerEntries && next.InternerEntries <= arena.Limits.MaxInternerEntries &&
		next.JournalEntries >= arena.Used.JournalEntries && next.JournalEntries <= arena.Limits.MaxJournalEntries &&
		next.StorageBytes >= arena.Used.StorageBytes && next.StorageBytes <= arena.Limits.MaxStorageBytes
}

func (arena *g18ReferenceCertificateArena) lookup(
	handle g18ReferenceCertificateHandle,
	expose bool,
) (*g18ReferenceCertificate, error) {
	// Count every owner-check attempt before comparing the owner.
	arena.OwnerCheckedLookups++
	// The owner check must occur before an index, epoch, or sequence lookup.
	if handle.ID.ArenaOwner == 0 || handle.ID.ArenaOwner != arena.Owner {
		return nil, errors.New("foreign arena owner")
	}
	if handle.ID.ArenaEpoch == 0 || handle.ID.ArenaEpoch != arena.Epoch {
		return nil, errors.New("stale arena epoch")
	}
	if handle.ID.CohortSequence == 0 || int(handle.Index) >= int(arena.Count) {
		return nil, errors.New("invalid cohort reference")
	}
	certificate := &arena.Certificates[handle.Index]
	if certificate.ID != handle.ID {
		return nil, errors.New("cohort identity mismatch")
	}
	if expose && certificate.State != g18ReferenceCertificateComplete {
		return nil, errors.New("certificate is not complete")
	}
	return certificate, nil
}

func (arena *g18ReferenceCertificateArena) write(
	handle g18ReferenceCertificateHandle,
	member g18ReferenceCohortMember,
) error {
	certificate, err := arena.lookup(handle, false)
	if err != nil {
		return err
	}
	if certificate.State != g18ReferenceCertificateBuilding {
		return errors.New("certificate is not building")
	}
	for index := 0; index < int(certificate.WrittenMembers); index++ {
		current := certificate.Members[index]
		if current == member {
			return errors.New("duplicate certificate member")
		}
		if current.Branch == member.Branch {
			certificate.State = g18ReferenceCertificateBlended
			return errors.New("conflicting certificate branch")
		}
	}
	if certificate.WrittenMembers >= certificate.ExpectedMembers || int(certificate.WrittenMembers) == len(certificate.Members) {
		certificate.State = g18ReferenceCertificateOverflowed
		return errors.New("certificate member overflow")
	}
	certificate.Members[certificate.WrittenMembers] = member
	certificate.WrittenMembers++
	return nil
}

func (arena *g18ReferenceCertificateArena) finalize(handle g18ReferenceCertificateHandle) error {
	certificate, err := arena.lookup(handle, false)
	if err != nil {
		return err
	}
	if certificate.State != g18ReferenceCertificateBuilding ||
		certificate.WrittenMembers != certificate.ExpectedMembers {
		return errors.New("certificate is partial or terminal")
	}
	certificate.State = g18ReferenceCertificateComplete
	return nil
}

func (arena *g18ReferenceCertificateArena) markUnproved(handle g18ReferenceCertificateHandle) error {
	certificate, err := arena.lookup(handle, false)
	if err != nil {
		return err
	}
	certificate.State = g18ReferenceCertificateUnproved
	return nil
}

func (certificate *g18ReferenceCertificate) compatibleMerge(source g18ReferenceCertificate) bool {
	if certificate.ID != source.ID || certificate.State != g18ReferenceCertificateBuilding ||
		source.State != g18ReferenceCertificateBuilding || source.WrittenMembers == 0 ||
		source.WrittenMembers != source.ExpectedMembers {
		certificate.State = g18ReferenceCertificateBlended
		return false
	}
	before := *certificate
	for index := 0; index < int(source.WrittenMembers); index++ {
		member := source.Members[index]
		for existing := 0; existing < int(certificate.WrittenMembers); existing++ {
			if certificate.Members[existing] == member || certificate.Members[existing].Branch == member.Branch {
				*certificate = before
				certificate.State = g18ReferenceCertificateBlended
				return false
			}
		}
		if certificate.WrittenMembers >= certificate.ExpectedMembers {
			*certificate = before
			certificate.State = g18ReferenceCertificateOverflowed
			return false
		}
		certificate.Members[certificate.WrittenMembers] = member
		certificate.WrittenMembers++
	}
	return true
}

func (certificate *g18ReferenceCertificate) contains(member g18ReferenceCohortMember) bool {
	for index := 0; index < int(certificate.WrittenMembers); index++ {
		if certificate.Members[index] == member {
			return true
		}
	}
	return false
}

func g18ReferenceVerifyDrop(
	arena *g18ReferenceCertificateArena,
	survivor g18ReferenceHeadProof,
	dropped ...g18ReferenceHeadProof,
) bool {
	if arena == nil || len(dropped) == 0 {
		return false
	}
	if survivor.Head.Node == 0 {
		return false
	}
	survivorCertificate, err := arena.lookup(survivor.Certificate, true)
	if err != nil || !survivorCertificate.contains(survivor.Member) {
		return false
	}
	for candidateIndex, candidate := range dropped {
		if candidate.Head.Node == 0 || candidate.Head == survivor.Head {
			return false
		}
		for prior := 0; prior < candidateIndex; prior++ {
			if dropped[prior].Head == candidate.Head {
				return false
			}
		}
		candidateCertificate, candidateErr := arena.lookup(candidate.Certificate, true)
		if candidateErr != nil || candidate.Certificate.ID != survivor.Certificate.ID ||
			!candidateCertificate.contains(candidate.Member) ||
			candidate.Member.Action != survivor.Member.Action ||
			candidate.Member.Derivation != survivor.Member.Derivation {
			return false
		}
	}
	return true
}

func g18ReferenceMember(branch, actionOrdinal uint16, derivation uint32) g18ReferenceCohortMember {
	record := [64]byte{
		byte(derivation), byte(derivation >> 8), byte(derivation >> 16), byte(derivation >> 24),
		byte(actionOrdinal), 2, 4, 1,
	}
	return g18ReferenceCohortMember{
		Branch: branch,
		Action: g18ReferenceActionIdentity{
			State: 3, Lookahead: 9, Ordinal: actionOrdinal, Type: core.ActionReduce,
			Symbol: 2, ProductionID: 5, ChildCount: 1, DynamicPrecedence: 2,
		},
		Derivation: g18ReferenceDerivationIdentity{
			InternedRecord: derivation, RootSymbol: 2, StackDepth: 4,
			Digest: sha256.Sum256(record[:8]), Record: record, RecordLength: 8,
		},
	}
}

func g18ValidDropScheduler(t *testing.T) (*diagnosticParserCoreGenericScheduler, core.Head, core.Head) {
	t.Helper()
	compact, err := core.New(&genericConflictTable{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := compact.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		options: DiagnosticParserCorePrefixOptions{ReceiptMode: DiagnosticParserCoreReceiptSummary},
		receipt: &DiagnosticParserCoreGenericScheduler{ReceiptMode: DiagnosticParserCoreReceiptSummary},
	}
	return scheduler, first, second
}

func g18RequireValidSummaryHeads(t *testing.T, scheduler *diagnosticParserCoreGenericScheduler) {
	t.Helper()
	for index, header := range scheduler.headers {
		if _, err := scheduler.headerReceipt(header); err != nil {
			t.Fatalf("header %d summary receipt: %v", index, err)
		}
	}
}

func TestG18DropPathRejectsBlendedSurvivor(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	scheduler, first, second := g18ValidDropScheduler(t)
	member := alternativeSetPinBranchMember(t, scheduler.compact, [2]uint16{7, 0})
	scheduler.headers = []diagnosticParserCoreHeader{
		{head: first, convergedReductionSplit: true, altSet: member, blended: true},
		{head: second, convergedReductionSplit: true, altSet: member},
	}
	g18RequireValidSummaryHeads(t, scheduler)
	if err := scheduler.dropGenericNoActionHeads([]int{1}); err == nil ||
		!strings.Contains(err.Error(), "lacks alternative-set coverage by one non-blended survivor") {
		t.Fatalf("blended-survivor drop error = %v", err)
	}
}

func TestG18DropPathRejectsUnprovedHistoricalImport(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	scheduler, first, second := g18ValidDropScheduler(t)
	member := alternativeSetPinBranchMember(t, scheduler.compact, [2]uint16{11, 0})
	scheduler.headers = []diagnosticParserCoreHeader{
		{head: first, convergedReductionSplit: true, altSet: member},
		{head: second, convergedReductionSplit: true, altSet: member, resurrectionUnproved: true},
	}
	g18RequireValidSummaryHeads(t, scheduler)
	if err := scheduler.dropGenericNoActionHeads([]int{1}); err == nil ||
		!strings.Contains(err.Error(), "unproved historical boundary resurrection") {
		t.Fatalf("unproved-history drop error = %v", err)
	}
}

func TestG18DropPathRejectsForeignSpill(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	scheduler, first, second := g18ValidDropScheduler(t)
	foreign := newAlternativeSetPinScheduler(t)
	foreignSpill := alternativeSetPinBranchMember(
		t, foreign.compact, [2]uint16{17, 0}, [2]uint16{19, 0}, [2]uint16{23, 0},
	)
	if members, ok := foreign.compact.AlternativeSetMembers(foreignSpill); !ok || len(members) != 3 {
		t.Fatalf("foreign spill setup = %d/%t, want 3/true", len(members), ok)
	}
	localInline := alternativeSetPinBranchMember(t, scheduler.compact, [2]uint16{17, 0})
	scheduler.headers = []diagnosticParserCoreHeader{
		{head: first, convergedReductionSplit: true, altSet: localInline},
		{head: second, convergedReductionSplit: true, altSet: foreignSpill},
	}
	g18RequireValidSummaryHeads(t, scheduler)
	if err := scheduler.dropGenericNoActionHeads([]int{1}); err == nil ||
		!strings.Contains(err.Error(), "lacks alternative-set coverage") {
		t.Fatalf("foreign-spill drop error = %v", err)
	}
}

func TestG18DropPathRejectsResetStaleSpill(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	scheduler, _, _ := g18ValidDropScheduler(t)
	stale := alternativeSetPinBranchMember(
		t, scheduler.compact, [2]uint16{29, 0}, [2]uint16{31, 0}, [2]uint16{37, 0},
	)
	if err := scheduler.compact.Reset(); err != nil {
		t.Fatal(err)
	}
	first, err := scheduler.compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := scheduler.compact.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	local := alternativeSetPinBranchMember(t, scheduler.compact, [2]uint16{29, 0})
	scheduler.headers = []diagnosticParserCoreHeader{
		{head: first, convergedReductionSplit: true, altSet: local},
		{head: second, convergedReductionSplit: true, altSet: stale},
	}
	g18RequireValidSummaryHeads(t, scheduler)
	if err := scheduler.dropGenericNoActionHeads([]int{1}); err == nil ||
		!strings.Contains(err.Error(), "lacks alternative-set coverage") {
		t.Fatalf("reset-stale spill drop error = %v", err)
	}
}

// TestG18DropPathRejectsForeignInlineRED is intentional RED. The current
// inline set has no arena identity, so a foreign inline value can authorize
// a current-arena drop. The future certificate must reject this drop.
func TestG18DropPathRejectsForeignInlineRED(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	scheduler, first, second := g18ValidDropScheduler(t)
	foreign := newAlternativeSetPinScheduler(t)
	foreignInline := alternativeSetPinBranchMember(t, foreign.compact, [2]uint16{41, 0})
	localInline := alternativeSetPinBranchMember(t, scheduler.compact, [2]uint16{41, 0})
	scheduler.headers = []diagnosticParserCoreHeader{
		{head: first, convergedReductionSplit: true, altSet: localInline},
		{head: second, convergedReductionSplit: true, altSet: foreignInline},
	}
	g18RequireValidSummaryHeads(t, scheduler)
	if err := scheduler.dropGenericNoActionHeads([]int{1}); err != nil {
		t.Fatalf("current foreign-inline characterization changed: %v", err)
	}
	t.Fatal("RED: foreign inline lineage certified a current-arena drop")
}

func TestG18CurrentCanonicalizerProducerPaths(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	compact, err := core.New(&genericConflictTable{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	left := alternativeSetPinBranchMember(t, compact, [2]uint16{47, 0})
	right := alternativeSetPinBranchMember(t, compact, [2]uint16{47, 1})

	linear := diagnosticParserCoreCanonicalScratch{}
	linearOut, err := linear.canonicalize(compact, []diagnosticParserCoreHeader{
		{head: head, altSet: left},
		{head: head, altSet: right},
	})
	if err != nil || len(linearOut) != 1 || !linearOut[0].blended || linearOut[0].altSet.Len() != 2 {
		t.Fatalf("linear canonicalizer output=%+v err=%v", linearOut, err)
	}

	mappedHeaders := make([]diagnosticParserCoreHeader, diagnosticParserCoreLinearCanonicalLimit+1)
	for index := range mappedHeaders {
		mappedHeaders[index] = diagnosticParserCoreHeader{head: head, altSet: left}
	}
	mappedHeaders[1].altSet = right
	mapped := diagnosticParserCoreCanonicalScratch{}
	mappedOut, err := mapped.canonicalize(compact, mappedHeaders)
	if err != nil || len(mappedOut) != 1 || !mappedOut[0].blended || mappedOut[0].altSet.Len() != 2 {
		t.Fatalf("mapped canonicalizer output=%+v err=%v", mappedOut, err)
	}
}

func g18AdoptionFixture(t *testing.T) (*core.Core, core.Head, core.Head) {
	t.Helper()
	table := &genericConflictTable{
		cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 7}: {{Type: core.ActionShift, State: 5}},
			{state: 1, symbol: 8}: {{Type: core.ActionShift, State: 3}},
			{state: 5, symbol: 9}: {{Type: core.ActionReduce, Symbol: 2, ChildCount: 1, DynamicPrecedence: 1}},
			{state: 3, symbol: 9}: {{Type: core.ActionReduce, Symbol: 2, ChildCount: 1, DynamicPrecedence: 2}},
		},
		gotos: map[genericConflictCell]core.StateID{{state: 1, symbol: 2}: 4},
	}
	compact, err := core.New(table, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	low, err := compact.Shift(seed, 7, 0, core.Token{Symbol: 7, EndByte: 1}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	high, err := compact.Shift(seed, 8, 0, core.Token{Symbol: 8, EndByte: 1}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := compact.ReduceOutputs(low, 9, 0, core.ForkOrder{})
	if err != nil || len(outputs) != 1 {
		t.Fatalf("low reduction output=%+v err=%v", outputs, err)
	}
	return compact, high, outputs[0].Head
}

func TestG18CurrentAdoptionAndConflictProducerPaths(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	for _, test := range []struct {
		name      string
		reconcile bool
	}{
		{name: "sibling_adoption"},
		{name: "conflict_reconciliation", reconcile: true},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			compact, high, active := g18AdoptionFixture(t)
			left := alternativeSetPinBranchMember(t, compact, [2]uint16{53, 0})
			right := alternativeSetPinBranchMember(t, compact, [2]uint16{53, 1})
			scheduler := &diagnosticParserCoreGenericScheduler{
				compact: compact,
				headers: []diagnosticParserCoreHeader{
					{head: high, creationSeq: 3, altSet: left},
					{head: active, creationSeq: 11, altSet: left},
				},
			}
			if !test.reconcile {
				adopted, err := scheduler.adoptUpdatedReductionSibling(
					0, active, core.CleanPathRankUnknown, 0, right, false, true, false,
				)
				if err != nil || !adopted {
					t.Fatalf("sibling adoption=%t err=%v", adopted, err)
				}
			} else {
				outputs := []diagnosticParserCoreHeader{{
					head: active, freshness: core.ReductionUpdated, altSet: right, cleanPathLineage: 53,
				}}
				kept, adopted, err := scheduler.reconcileGenericConflictOutputs(0, outputs)
				if err != nil || len(kept) != 0 || adopted != 1 {
					t.Fatalf("conflict reconciliation kept=%+v adopted=%d err=%v", kept, adopted, err)
				}
			}
			got := scheduler.headers[1]
			if got.altSet.Len() != 2 || !got.blended || !got.convergedReductionSplit {
				t.Fatalf("adopted producer state=%+v", got)
			}
		})
	}
}

const (
	g18FutureStoreActionRecords = iota
	g18FutureStoreDerivationRecords
	g18FutureStoreCertificateReferences
	g18FutureStoreMapEntries
	g18FutureStoreInternerEntries
	g18FutureStoreJournalEntries
	g18FutureStoreCohortRecords
	g18FutureStoreCount

	g18FutureActionFieldCount = 14
)

var g18FutureProducerNames = []string{
	"reduction_establishment",
	"linear_canonicalizer",
	"mapped_canonicalizer",
	"sibling_adoption",
	"conflict_reconciliation",
	"dead_history_import",
}

// These aliases are fixed-width protocol primitives. They keep the future
// API independent of root-package test types, so internal Core can implement
// the methods without importing this package.
type g18FutureStoreVector = [g18FutureStoreCount]uint64
type g18FutureCohortHandle = [3]uint64
type g18FutureActionVector = [g18FutureActionFieldCount]int64

type g18FutureActionRecordLayout struct {
	Fields g18FutureActionVector
}

type g18FutureDerivationRecordLayout struct {
	Digest [sha256.Size]byte
	Offset uint32
	Length uint32
}

type g18FutureCertificateReferenceLayout struct {
	Handle g18FutureCohortHandle
	Head   core.Head
	Branch uint16
}

type g18FutureMapEntryLayout struct {
	Hash  uint64
	Index uint32
	Used  bool
}

type g18FutureInternerEntryLayout struct {
	Digest [sha256.Size]byte
	Index  uint32
	Used   bool
}

type g18FutureJournalEntryLayout struct {
	Store uint8
	Index uint32
	Value uint64
}

type g18FutureCohortRecordLayout struct {
	Handle   g18FutureCohortHandle
	State    uint8
	Expected uint16
	Written  uint16
	Spilled  bool
}

type g18FutureCoreCertificateBehavior interface {
	DiagnosticDropCohortArenaIdentityForTest() (uint64, uint64)
	DiagnosticDropCohortSnapshotForTest() []byte
	// The wrap probes use the production atomic counters with a scoped test
	// value. They return the last accepted value and the rejected wrapped value.
	DiagnosticDropCohortOwnerWrapProbeForTest() (uint64, uint64, error)
	DiagnosticDropCohortEpochWrapProbeForTest() (uint64, uint64, error)
	DiagnosticDropCohortSequenceWrapProbeForTest() (uint64, uint64, error)
	DiagnosticDropCohortLimitsForTest() (uint16, uint16)
	DiagnosticDropCohortSetLimitsForTest(uint16, uint16) error
	DiagnosticDropCohortBeginForTest(uint16, uint32, [g18FutureStoreCount]uint64) ([3]uint64, error)
	DiagnosticDropCohortWriteForTest(
		[3]uint64,
		core.Head,
		uint16,
		[g18FutureActionFieldCount]int64,
		[sha256.Size]byte,
		[]byte,
	) error
	DiagnosticDropCohortFinalizeForTest([3]uint64) error
	DiagnosticDropCohortMarkUnprovedForTest([3]uint64) error
	DiagnosticDropCohortRollbackForTest([3]uint64) error
}

// g18FutureCompileContractProvider proves that an internal-owned provider can
// satisfy the root adapter with only fixed-width primitives and core.Head.
type g18FutureCompileContractProvider struct{}

var _ g18FutureCoreCertificateBehavior = (*g18FutureCompileContractProvider)(nil)

func TestG18FutureInternalAPICompileContract(t *testing.T) {
	var provider g18FutureCoreCertificateBehavior = &g18FutureCompileContractProvider{}
	if provider == nil {
		t.Fatal("internal API compile contract provider is nil")
	}
}

func (*g18FutureCompileContractProvider) DiagnosticDropCohortArenaIdentityForTest() (uint64, uint64) {
	return 0, 0
}

func (*g18FutureCompileContractProvider) DiagnosticDropCohortSnapshotForTest() []byte {
	return nil
}

func (*g18FutureCompileContractProvider) DiagnosticDropCohortOwnerWrapProbeForTest() (uint64, uint64, error) {
	return 0, 0, nil
}

func (*g18FutureCompileContractProvider) DiagnosticDropCohortEpochWrapProbeForTest() (uint64, uint64, error) {
	return 0, 0, nil
}

func (*g18FutureCompileContractProvider) DiagnosticDropCohortSequenceWrapProbeForTest() (uint64, uint64, error) {
	return 0, 0, nil
}

func (*g18FutureCompileContractProvider) DiagnosticDropCohortLimitsForTest() (uint16, uint16) {
	return 0, 0
}

func (*g18FutureCompileContractProvider) DiagnosticDropCohortSetLimitsForTest(uint16, uint16) error {
	return nil
}

func (*g18FutureCompileContractProvider) DiagnosticDropCohortBeginForTest(uint16, uint32, [g18FutureStoreCount]uint64) ([3]uint64, error) {
	return [3]uint64{}, nil
}

func (*g18FutureCompileContractProvider) DiagnosticDropCohortWriteForTest([3]uint64, core.Head, uint16, [g18FutureActionFieldCount]int64, [sha256.Size]byte, []byte) error {
	return nil
}

func (*g18FutureCompileContractProvider) DiagnosticDropCohortFinalizeForTest([3]uint64) error {
	return nil
}

func (*g18FutureCompileContractProvider) DiagnosticDropCohortMarkUnprovedForTest([3]uint64) error {
	return nil
}

func (*g18FutureCompileContractProvider) DiagnosticDropCohortRollbackForTest([3]uint64) error {
	return nil
}

type g18FutureSchedulerDropBehavior interface {
	DiagnosticBindDropCohortReferencesForTest([]g18FutureCohortHandle, []uint16) error
	DiagnosticDropGenericNoActionHeadsForTest([]int) (string, error)
	DiagnosticDropGenericNoActionHeadsNonDestructiveForTest([]int) (string, uint64, [sha256.Size]byte, error)
	DiagnosticDropGenericNoActionHeadsVerifierStateDigestForTest() [sha256.Size]byte
}

// G18DropCohortProducerMutationClass identifies one authenticated producer
// transition in the test-only Slice C compatibility surface.
type G18DropCohortProducerMutationClass uint8

const (
	g18FutureProducerLinearCanonicalization G18DropCohortProducerMutationClass = iota + 1
	g18FutureProducerMappedCanonicalization
	g18FutureProducerSiblingAdoption
	g18FutureProducerConflictReconciliation
)

// These exported, test-only interfaces are the exact Slice C compatibility
// contract. Their unique names prevent accidental satisfaction by legacy APIs.
type g18FutureSiblingAdoptionCompatibility interface {
	G18AdoptUpdatedReductionSiblingOwned(
		core.SchedulerTransactionToken,
		int,
		core.Head,
		core.CleanPathRankSelection,
		uint16,
		core.AlternativeSet,
		bool,
		bool,
		bool,
		G18DropCohortProducerMutationClass,
	) (bool, error)
}

type g18FutureConflictReconciliationCompatibility interface {
	G18ReconcileGenericConflictOutputsOwned(
		core.SchedulerTransactionToken,
		int,
		[]diagnosticParserCoreHeader,
		G18DropCohortProducerMutationClass,
	) ([]diagnosticParserCoreHeader, int, error)
}

type g18FutureCanonicalizerCompatibility interface {
	G18CanonicalizeOwned(core.SchedulerTransactionToken, G18DropCohortProducerMutationClass) error
}

type g18FutureOwnedSchedulerAdapter struct {
	scheduler *diagnosticParserCoreGenericScheduler
}

func (adapter g18FutureOwnedSchedulerAdapter) g18AdoptUpdatedReductionSiblingOwned(
	owner core.SchedulerTransactionToken,
	source int,
	head core.Head,
	rank core.CleanPathRankSelection,
	lineage uint16,
	set core.AlternativeSet,
	setBlended bool,
	converged bool,
	resurrection bool,
	mutation G18DropCohortProducerMutationClass,
) (bool, error) {
	if mutation != g18FutureProducerSiblingAdoption {
		return false, errors.New("RED: invalid authenticated sibling-adoption mutation class")
	}
	wrapper, ok := any(adapter.scheduler).(g18FutureSiblingAdoptionCompatibility)
	if !ok {
		return false, errors.New("RED: missing exported Slice C sibling-adoption compatibility method")
	}
	return wrapper.G18AdoptUpdatedReductionSiblingOwned(
		owner, source, head, rank, lineage, set, setBlended, converged, resurrection, mutation,
	)
}

func (adapter g18FutureOwnedSchedulerAdapter) g18ReconcileGenericConflictOutputsOwned(
	owner core.SchedulerTransactionToken,
	source int,
	outputs []diagnosticParserCoreHeader,
	mutation G18DropCohortProducerMutationClass,
) ([]diagnosticParserCoreHeader, int, error) {
	if mutation != g18FutureProducerConflictReconciliation {
		return nil, 0, errors.New("RED: invalid authenticated conflict-reconciliation mutation class")
	}
	wrapper, ok := any(adapter.scheduler).(g18FutureConflictReconciliationCompatibility)
	if !ok {
		return nil, 0, errors.New("RED: missing exported Slice C conflict-reconciliation compatibility method")
	}
	return wrapper.G18ReconcileGenericConflictOutputsOwned(owner, source, outputs, mutation)
}

func g18FutureAdoptUpdatedReductionSiblingRED(
	t *testing.T,
	scheduler *diagnosticParserCoreGenericScheduler,
	source int,
	head core.Head,
	rank core.CleanPathRankSelection,
	lineage uint16,
	set core.AlternativeSet,
	setBlended bool,
	converged bool,
	resurrection bool,
	mutation G18DropCohortProducerMutationClass,
) (adopted bool, err error) {
	t.Helper()
	adapter := g18FutureOwnedSchedulerAdapter{scheduler: scheduler}
	err = scheduler.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		adopted, err = adapter.g18AdoptUpdatedReductionSiblingOwned(
			owner, source, head, rank, lineage, set, setBlended, converged, resurrection, mutation,
		)
		return err
	})
	return adopted, err
}

func g18FutureReconcileGenericConflictOutputsRED(
	t *testing.T,
	scheduler *diagnosticParserCoreGenericScheduler,
	source int,
	outputs []diagnosticParserCoreHeader,
	mutation G18DropCohortProducerMutationClass,
) (kept []diagnosticParserCoreHeader, adopted int, err error) {
	t.Helper()
	adapter := g18FutureOwnedSchedulerAdapter{scheduler: scheduler}
	err = scheduler.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		kept, adopted, err = adapter.g18ReconcileGenericConflictOutputsOwned(owner, source, outputs, mutation)
		return err
	})
	return kept, adopted, err
}

func (adapter g18FutureOwnedSchedulerAdapter) g18CanonicalizeOwned(
	owner core.SchedulerTransactionToken,
	mutation G18DropCohortProducerMutationClass,
) error {
	if mutation != g18FutureProducerLinearCanonicalization && mutation != g18FutureProducerMappedCanonicalization {
		return errors.New("RED: invalid authenticated canonicalizer mutation class")
	}
	wrapper, ok := any(adapter.scheduler).(g18FutureCanonicalizerCompatibility)
	if !ok {
		return errors.New("RED: missing exported Slice C canonicalizer compatibility method")
	}
	return wrapper.G18CanonicalizeOwned(owner, mutation)
}

// g18CertificateAdmissionOptionAdapter is a private, test-only seam. It
// keeps certificate admission outside runtime profile flags and returns an
// exact restore function for cached parser reuse.
type g18CertificateAdmissionOptionAdapter interface {
	DiagnosticEnableDropCohortCertificateAdmissionForTest() func()
}

func TestG18CertificateAdmissionOptionAdapterCompileContract(t *testing.T) {
	var parser *Parser
	if parser != nil {
		_, _ = any(parser).(g18CertificateAdmissionOptionAdapter)
	}
}

type g18FutureCohortSnapshot struct {
	Handle   g18FutureCohortHandle `json:"handle"`
	State    string                `json:"state"`
	Expected uint16                `json:"expected"`
	Written  uint16                `json:"written"`
	Spilled  bool                  `json:"spilled"`
}

type g18FutureCoreSnapshot struct {
	Schema               string                    `json:"schema"`
	ArenaOwner           uint64                    `json:"arena_owner"`
	ArenaEpoch           uint64                    `json:"arena_epoch"`
	Cohorts              []g18FutureCohortSnapshot `json:"cohorts"`
	Storage              g18FutureStoreVector      `json:"storage"`
	Footprint            g18FutureStoreVector      `json:"footprint"`
	StorageBytes         uint64                    `json:"storage_bytes"`
	FootprintBytes       uint64                    `json:"footprint_bytes"`
	ProducerWrites       map[string]uint64         `json:"producer_writes"`
	VerifierElections    uint64                    `json:"verifier_elections"`
	VerifierProofs       uint64                    `json:"verifier_proofs"`
	VerifierDeclines     uint64                    `json:"verifier_declines"`
	ActionDeclines       uint64                    `json:"action_identity_declines"`
	DerivationDeclines   uint64                    `json:"derivation_identity_declines"`
	AuthenticatedHistory uint64                    `json:"authenticated_history_imports"`
	UnprovedHistory      uint64                    `json:"unproved_history_imports"`
	DeclineReasons       map[string]uint64         `json:"decline_reasons"`
	// OwnerCheckedLookups counts every owner-check attempt, including a
	// rejected foreign or stale handle. It does not count certificate reads.
	OwnerCheckedLookups uint64 `json:"owner_checked_lookups"`
	InlineReads         uint64 `json:"inline_reads"`
	SpillReads          uint64 `json:"spill_reads"`
	MapReads            uint64 `json:"map_reads"`
	InternerReads       uint64 `json:"interner_reads"`
}

func g18RequireFutureCoreBehaviorRED(
	t *testing.T,
	compact *core.Core,
) g18FutureCoreCertificateBehavior {
	t.Helper()
	provider, ok := any(compact).(g18FutureCoreCertificateBehavior)
	if !ok {
		t.Fatal("RED: real Core does not implement the drop-cohort behavior API")
	}
	return provider
}

func g18FutureDecodeCoreSnapshot(
	t *testing.T,
	provider g18FutureCoreCertificateBehavior,
) g18FutureCoreSnapshot {
	t.Helper()
	var snapshot g18FutureCoreSnapshot
	if err := json.Unmarshal(provider.DiagnosticDropCohortSnapshotForTest(), &snapshot); err != nil {
		t.Fatalf("decode certificate arena snapshot: %v", err)
	}
	owner, epoch := provider.DiagnosticDropCohortArenaIdentityForTest()
	if snapshot.Schema != "gts-drop-cohort-certificate-arena/v2" || owner == 0 || epoch == 0 ||
		snapshot.ArenaOwner != owner || snapshot.ArenaEpoch != epoch {
		t.Fatalf("certificate arena identity = %d/%d snapshot=%+v", owner, epoch, snapshot)
	}
	for _, producer := range g18FutureProducerNames {
		if _, ok := snapshot.ProducerWrites[producer]; !ok {
			t.Fatalf("certificate arena snapshot omits producer counter %q", producer)
		}
	}
	if snapshot.StorageBytes != g18FutureSumStores(snapshot.Storage) ||
		snapshot.FootprintBytes != g18FutureSumStores(snapshot.Footprint) {
		t.Fatalf("certificate arena byte totals = %+v", snapshot)
	}
	return snapshot
}

func g18FutureSumStores(stores g18FutureStoreVector) uint64 {
	var total uint64
	for _, value := range stores {
		total += value
	}
	return total
}

func g18FutureNextPowerOfTwo(value uint64) uint64 {
	if value <= 1 {
		return 1
	}
	value--
	value |= value >> 1
	value |= value >> 2
	value |= value >> 4
	value |= value >> 8
	value |= value >> 16
	value |= value >> 32
	return value + 1
}

func g18FutureAccountingPlan(expected uint16, derivationBytes uint32) (
	g18FutureStoreVector,
	g18FutureStoreVector,
) {
	members := uint64(expected)
	mapCapacity := g18FutureNextPowerOfTwo(members * 2)
	storage := g18FutureStoreVector{
		g18FutureStoreActionRecords:         members * uint64(unsafe.Sizeof(g18FutureActionRecordLayout{})),
		g18FutureStoreDerivationRecords:     members * (uint64(unsafe.Sizeof(g18FutureDerivationRecordLayout{})) + uint64(derivationBytes)),
		g18FutureStoreCertificateReferences: members * uint64(unsafe.Sizeof(g18FutureCertificateReferenceLayout{})),
		g18FutureStoreMapEntries:            members * uint64(unsafe.Sizeof(g18FutureMapEntryLayout{})),
		g18FutureStoreInternerEntries:       members * uint64(unsafe.Sizeof(g18FutureInternerEntryLayout{})),
		g18FutureStoreJournalEntries:        (members + 2) * uint64(unsafe.Sizeof(g18FutureJournalEntryLayout{})),
		g18FutureStoreCohortRecords:         uint64(unsafe.Sizeof(g18FutureCohortRecordLayout{})),
	}
	footprint := storage
	footprint[g18FutureStoreMapEntries] = mapCapacity * uint64(unsafe.Sizeof(g18FutureMapEntryLayout{}))
	footprint[g18FutureStoreInternerEntries] = mapCapacity * uint64(unsafe.Sizeof(g18FutureInternerEntryLayout{}))
	return storage, footprint
}

func g18RequireOnlyProducerDelta(
	t *testing.T,
	before g18FutureCoreSnapshot,
	after g18FutureCoreSnapshot,
	producer string,
) {
	t.Helper()
	known := make(map[string]bool, len(g18FutureProducerNames))
	for _, name := range g18FutureProducerNames {
		known[name] = true
	}
	for name, count := range after.ProducerWrites {
		if !known[name] && count != before.ProducerWrites[name] {
			t.Fatalf("unknown producer counter %s changed from %d to %d", name, before.ProducerWrites[name], count)
		}
	}
	for _, name := range g18FutureProducerNames {
		want := uint64(0)
		if name == producer {
			want = 1
		}
		if after.ProducerWrites[name] < before.ProducerWrites[name] {
			t.Fatalf("producer counter %s decreased", name)
		}
		if got := after.ProducerWrites[name] - before.ProducerWrites[name]; got != want {
			t.Fatalf("producer delta %s=%d, want %d; before=%+v after=%+v", name, got, want, before.ProducerWrites, after.ProducerWrites)
		}
	}
}

// TestG18FutureCanonicalizerCertificateTelemetryRED binds both real
// canonicalizers to the future arena. Current main does not publish it.
func TestG18FutureCanonicalizerCertificateTelemetryRED(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	for _, mapped := range []bool{false, true} {
		mapped := mapped
		name := "linear_canonicalizer"
		if mapped {
			name = "mapped_canonicalizer"
		}
		t.Run(name, func(t *testing.T) {
			compact, err := core.New(&genericConflictTable{}, core.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			head, err := compact.Seed(1, 0)
			if err != nil {
				t.Fatal(err)
			}
			left := alternativeSetPinBranchMember(t, compact, [2]uint16{61, 0})
			right := alternativeSetPinBranchMember(t, compact, [2]uint16{61, 1})
			headers := []diagnosticParserCoreHeader{{head: head, altSet: left}, {head: head, altSet: right}}
			if mapped {
				headers = make([]diagnosticParserCoreHeader, diagnosticParserCoreLinearCanonicalLimit+1)
				for index := range headers {
					headers[index] = diagnosticParserCoreHeader{head: head, altSet: left}
				}
				headers[1].altSet = right
			}
			provider := g18RequireFutureCoreBehaviorRED(t, compact)
			before := g18FutureDecodeCoreSnapshot(t, provider)
			scheduler := &diagnosticParserCoreGenericScheduler{
				compact: compact,
				headers: headers,
			}
			mutation := g18FutureProducerLinearCanonicalization
			if mapped {
				mutation = g18FutureProducerMappedCanonicalization
			}
			adapter := g18FutureOwnedSchedulerAdapter{scheduler: scheduler}
			err = compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
				return adapter.g18CanonicalizeOwned(owner, mutation)
			})
			if err != nil || len(scheduler.headers) != 1 {
				t.Fatalf("canonicalizer outputs=%+v err=%v", scheduler.headers, err)
			}
			after := g18FutureDecodeCoreSnapshot(t, provider)
			g18RequireOnlyProducerDelta(t, before, after, name)
		})
	}
}

// TestG18FutureAdoptionCertificateTelemetryRED binds both real adoption
// producers to the future arena. Current main does not publish it.
func TestG18FutureAdoptionCertificateTelemetryRED(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	for _, test := range []struct {
		name      string
		reconcile bool
	}{
		{name: "sibling_adoption"},
		{name: "conflict_reconciliation", reconcile: true},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			compact, high, active := g18AdoptionFixture(t)
			left := alternativeSetPinBranchMember(t, compact, [2]uint16{67, 0})
			right := alternativeSetPinBranchMember(t, compact, [2]uint16{67, 1})
			scheduler := &diagnosticParserCoreGenericScheduler{
				compact: compact,
				headers: []diagnosticParserCoreHeader{
					{head: high, creationSeq: 3, altSet: left},
					{head: active, creationSeq: 11, altSet: left},
				},
			}
			provider := g18RequireFutureCoreBehaviorRED(t, compact)
			before := g18FutureDecodeCoreSnapshot(t, provider)
			if test.reconcile {
				outputs := []diagnosticParserCoreHeader{{
					head: active, freshness: core.ReductionUpdated, altSet: right, cleanPathLineage: 67,
				}}
				if _, adopted, err := g18FutureReconcileGenericConflictOutputsRED(t, scheduler, 0, outputs, g18FutureProducerConflictReconciliation); err != nil || adopted != 1 {
					t.Fatalf("conflict reconciliation adopted=%d err=%v", adopted, err)
				}
			} else if adopted, err := g18FutureAdoptUpdatedReductionSiblingRED(
				t, scheduler, 0, active, core.CleanPathRankUnknown, 0, right, false, true, false,
				g18FutureProducerSiblingAdoption,
			); err != nil || !adopted {
				t.Fatalf("sibling adoption=%t err=%v", adopted, err)
			}
			after := g18FutureDecodeCoreSnapshot(t, provider)
			g18RequireOnlyProducerDelta(t, before, after, test.name)
		})
	}
}

func TestG18FutureReductionEstablishmentTelemetryRED(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	compact, err := core.New(&genericConflictTable{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	provider := g18RequireFutureCoreBehaviorRED(t, compact)
	before := g18FutureDecodeCoreSnapshot(t, provider)
	err = compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		return compact.RecordReductionLineageOwned(owner, []core.ReductionOutput{{
			Head: head, CleanPathRank: core.CleanPathRankSelected, MultiplePopPaths: true,
		}}, 71)
	})
	if err != nil {
		t.Fatal(err)
	}
	after := g18FutureDecodeCoreSnapshot(t, provider)
	g18RequireOnlyProducerDelta(t, before, after, "reduction_establishment")
}

func g18FutureBehaviorFixture(
	t *testing.T,
) (*diagnosticParserCoreGenericScheduler, g18FutureCoreCertificateBehavior, [3]core.Head) {
	t.Helper()
	scheduler, first, second := g18ValidDropScheduler(t)
	compact := scheduler.compact
	third, err := compact.Seed(3, 0)
	if err != nil {
		t.Fatal(err)
	}
	provider := g18RequireFutureCoreBehaviorRED(t, compact)
	return scheduler, provider, [3]core.Head{first, second, third}
}

func g18FutureUnlimitedCaps() g18FutureStoreVector {
	var caps g18FutureStoreVector
	for index := range caps {
		caps[index] = math.MaxUint64
	}
	return caps
}

func g18FutureActionIdentity() g18FutureActionVector {
	return g18FutureActionVector{
		3, 9, 3, int64(core.ActionReduce), 0, 2, 5, 1, 2, 0, 0, 0, 0, 1,
	}
}

func g18FutureDerivationRecord() ([]byte, [sha256.Size]byte) {
	record := []byte{2, 4, 5, 1, 0, 8, 13, 21}
	return record, sha256.Sum256(record)
}

func g18FutureBeginCohort(
	t *testing.T,
	provider g18FutureCoreCertificateBehavior,
	expected uint16,
	recordBytes uint32,
) g18FutureCohortHandle {
	t.Helper()
	handle, err := provider.DiagnosticDropCohortBeginForTest(
		expected,
		recordBytes,
		g18FutureUnlimitedCaps(),
	)
	if err != nil {
		t.Fatal(err)
	}
	owner, epoch := provider.DiagnosticDropCohortArenaIdentityForTest()
	if handle[0] != owner || handle[1] != epoch || handle[2] == 0 {
		t.Fatalf("new cohort handle=%v arena=%d/%d", handle, owner, epoch)
	}
	return handle
}

func g18FutureWriteMember(
	t *testing.T,
	provider g18FutureCoreCertificateBehavior,
	handle g18FutureCohortHandle,
	head core.Head,
	branch uint16,
	action g18FutureActionVector,
	digest [sha256.Size]byte,
	record []byte,
) {
	t.Helper()
	if err := provider.DiagnosticDropCohortWriteForTest(
		handle,
		head,
		branch,
		action,
		digest,
		record,
	); err != nil {
		t.Fatal(err)
	}
}

func g18FutureFindCohort(
	t *testing.T,
	snapshot g18FutureCoreSnapshot,
	handle g18FutureCohortHandle,
) g18FutureCohortSnapshot {
	t.Helper()
	for _, cohort := range snapshot.Cohorts {
		if cohort.Handle == handle {
			return cohort
		}
	}
	t.Fatalf("cohort %v is absent from snapshot %+v", handle, snapshot.Cohorts)
	return g18FutureCohortSnapshot{}
}

func g18FutureRequireCohortState(
	t *testing.T,
	provider g18FutureCoreCertificateBehavior,
	handle g18FutureCohortHandle,
	state string,
	expected uint16,
	written uint16,
) g18FutureCoreSnapshot {
	t.Helper()
	snapshot := g18FutureDecodeCoreSnapshot(t, provider)
	cohort := g18FutureFindCohort(t, snapshot, handle)
	if cohort.State != state || cohort.Expected != expected || cohort.Written != written {
		t.Fatalf("cohort state=%+v, want %s %d/%d", cohort, state, expected, written)
	}
	return snapshot
}

func g18FutureSchedulerVerify(
	t *testing.T,
	scheduler *diagnosticParserCoreGenericScheduler,
	provider g18FutureCoreCertificateBehavior,
	heads []core.Head,
	handles []g18FutureCohortHandle,
	branches []uint16,
	drops ...int,
) (bool, string) {
	t.Helper()
	dropper, ok := any(scheduler).(g18FutureSchedulerDropBehavior)
	if !ok {
		return false, "RED: scheduler does not implement the real drop-path behavior API"
	}
	scheduler.headers = make([]diagnosticParserCoreHeader, len(heads))
	for index, head := range heads {
		scheduler.headers[index] = diagnosticParserCoreHeader{head: head}
	}
	g18RequireValidSummaryHeads(t, scheduler)
	if err := dropper.DiagnosticBindDropCohortReferencesForTest(handles, branches); err != nil {
		return false, "bind_drop_cohort_references"
	}
	var before g18FutureCoreSnapshot
	if err := json.Unmarshal(provider.DiagnosticDropCohortSnapshotForTest(), &before); err != nil {
		return false, "telemetry_decode_before"
	}
	reason, err := dropper.DiagnosticDropGenericNoActionHeadsForTest(drops)
	admitted := err == nil
	var after g18FutureCoreSnapshot
	if decodeErr := json.Unmarshal(provider.DiagnosticDropCohortSnapshotForTest(), &after); decodeErr != nil {
		return false, "telemetry_decode_after"
	}
	if after.VerifierElections != before.VerifierElections+1 {
		return false, "telemetry_election_delta"
	}
	if admitted {
		if reason != "proved" || after.VerifierProofs != before.VerifierProofs+1 ||
			after.VerifierDeclines != before.VerifierDeclines ||
			g18FutureReasonDelta(t, before.DeclineReasons, after.DeclineReasons) != 0 {
			return false, "telemetry_proof_delta"
		}
		return true, reason
	}
	if g18FutureReasonDelta(t, before.DeclineReasons, after.DeclineReasons) != 1 {
		return false, "telemetry_reason_delta"
	}
	if after.VerifierProofs != before.VerifierProofs ||
		after.VerifierDeclines != before.VerifierDeclines+1 ||
		after.DeclineReasons[reason] != before.DeclineReasons[reason]+1 {
		return false, "telemetry_decline_delta"
	}
	wantAction := before.ActionDeclines
	wantDerivation := before.DerivationDeclines
	if reason == "action_identity_mismatch" {
		wantAction++
	}
	if reason == "derivation_identity_mismatch" {
		wantDerivation++
	}
	if after.ActionDeclines != wantAction || after.DerivationDeclines != wantDerivation {
		return false, "telemetry_identity_delta"
	}
	return false, reason
}

func g18FutureReasonDelta(
	t *testing.T,
	before, after map[string]uint64,
) uint64 {
	t.Helper()
	var delta uint64
	for reason, count := range after {
		if count < before[reason] {
			t.Fatalf("decline reason %s decreased from %d to %d", reason, before[reason], count)
		}
		delta += count - before[reason]
	}
	for reason, count := range before {
		if _, ok := after[reason]; !ok && count != 0 {
			t.Fatalf("decline reason %s disappeared from %d", reason, count)
		}
	}
	return delta
}

func g18FutureRequireNoCertificateReads(
	t *testing.T,
	before, after g18FutureCoreSnapshot,
) {
	t.Helper()
	if after.InlineReads != before.InlineReads ||
		after.SpillReads != before.SpillReads ||
		after.MapReads != before.MapReads ||
		after.InternerReads != before.InternerReads {
		t.Fatalf(
			"foreign or stale handle reached certificate storage: before inline/spill/map/interner=%d/%d/%d/%d after=%d/%d/%d/%d",
			before.InlineReads,
			before.SpillReads,
			before.MapReads,
			before.InternerReads,
			after.InlineReads,
			after.SpillReads,
			after.MapReads,
			after.InternerReads,
		)
	}
}

func g18FutureSubtractStores(
	t *testing.T,
	after g18FutureStoreVector,
	before g18FutureStoreVector,
) g18FutureStoreVector {
	t.Helper()
	var difference g18FutureStoreVector
	for index := range difference {
		if after[index] < before[index] {
			t.Fatalf("store %d decreased from %d to %d", index, before[index], after[index])
		}
		difference[index] = after[index] - before[index]
	}
	return difference
}

func TestG18FutureConcurrentArenaOwnerAllocationRED(t *testing.T) {
	const instances = 32
	start := make(chan struct{})
	results := make(chan struct {
		owner uint64
		epoch uint64
		err   error
	}, instances)
	var wait sync.WaitGroup
	wait.Add(instances)
	for index := 0; index < instances; index++ {
		go func() {
			defer wait.Done()
			<-start
			compact, err := core.New(&genericConflictTable{}, core.Limits{})
			if err != nil {
				results <- struct {
					owner uint64
					epoch uint64
					err   error
				}{err: err}
				return
			}
			provider, ok := any(compact).(g18FutureCoreCertificateBehavior)
			if !ok {
				results <- struct {
					owner uint64
					epoch uint64
					err   error
				}{err: errors.New("RED: real Core does not implement the behavior API")}
				return
			}
			owner, epoch := provider.DiagnosticDropCohortArenaIdentityForTest()
			results <- struct {
				owner uint64
				epoch uint64
				err   error
			}{owner: owner, epoch: epoch}
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	owners := make(map[uint64]bool, instances)
	var firstErr error
	failed := 0
	for result := range results {
		if result.err != nil {
			failed++
			if firstErr == nil {
				firstErr = result.err
			}
			continue
		}
		if result.owner == 0 || result.epoch == 0 || owners[result.owner] {
			t.Errorf("concurrent arena identity owner=%d epoch=%d duplicate=%t", result.owner, result.epoch, owners[result.owner])
		}
		owners[result.owner] = true
	}
	if failed != 0 {
		t.Fatalf("%d concurrent real Core instances failed: %v", failed, firstErr)
	}
	if len(owners) != instances {
		t.Fatalf("concurrent unique owners=%d, want %d", len(owners), instances)
	}
}

func TestG18FutureRealVerifierArenaIdentityRED(t *testing.T) {
	schedulerA, firstA, secondA := g18ValidDropScheduler(t)
	compactA := schedulerA.compact
	providerA := g18RequireFutureCoreBehaviorRED(t, compactA)
	thirdA, err := compactA.Seed(3, 0)
	if err != nil {
		t.Fatal(err)
	}
	headsA := [3]core.Head{firstA, secondA, thirdA}
	schedulerB, firstB, secondB := g18ValidDropScheduler(t)
	providerB := g18RequireFutureCoreBehaviorRED(t, schedulerB.compact)
	thirdB, err := schedulerB.compact.Seed(3, 0)
	if err != nil {
		t.Fatal(err)
	}
	headsB := [3]core.Head{firstB, secondB, thirdB}
	ownerA, epochA := providerA.DiagnosticDropCohortArenaIdentityForTest()
	ownerB, epochB := providerB.DiagnosticDropCohortArenaIdentityForTest()
	if ownerA == ownerB || epochA != epochB {
		t.Fatalf("arena identities A=%d/%d B=%d/%d", ownerA, epochA, ownerB, epochB)
	}
	action := g18FutureActionIdentity()
	record, digest := g18FutureDerivationRecord()

	inlineA := g18FutureBeginCohort(t, providerA, 2, uint32(len(record)))
	inlineB := g18FutureBeginCohort(t, providerB, 2, uint32(len(record)))
	for index := 0; index < 2; index++ {
		g18FutureWriteMember(t, providerA, inlineA, headsA[index], uint16(index), action, digest, record)
		g18FutureWriteMember(t, providerB, inlineB, headsB[index], uint16(index), action, digest, record)
	}
	if inlineA[1] != inlineB[1] || inlineA[2] != inlineB[2] {
		t.Fatalf("inline equal-session handles A=%v B=%v", inlineA, inlineB)
	}
	if err := providerA.DiagnosticDropCohortFinalizeForTest(inlineA); err != nil {
		t.Fatal(err)
	}
	if err := providerB.DiagnosticDropCohortFinalizeForTest(inlineB); err != nil {
		t.Fatal(err)
	}
	beforeLocalInline := g18FutureDecodeCoreSnapshot(t, providerA)
	if admitted, reason := g18FutureSchedulerVerify(
		t,
		schedulerA,
		providerA,
		headsA[:2],
		[]g18FutureCohortHandle{inlineA, inlineA},
		[]uint16{0, 1},
		1,
	); !admitted || reason != "proved" {
		t.Fatalf("arena A inline verification=%t reason=%q", admitted, reason)
	}
	afterLocalInline := g18FutureDecodeCoreSnapshot(t, providerA)
	if afterLocalInline.OwnerCheckedLookups != beforeLocalInline.OwnerCheckedLookups+2 {
		t.Fatalf("local inline owner-check attempts: before=%d after=%d, want two attempts", beforeLocalInline.OwnerCheckedLookups, afterLocalInline.OwnerCheckedLookups)
	}
	// Snapshot after the valid local proof. The owner counter measures attempts.
	beforeForeignInline := afterLocalInline
	if admitted, reason := g18FutureSchedulerVerify(
		t,
		schedulerA,
		providerA,
		headsA[:2],
		[]g18FutureCohortHandle{inlineB, inlineA},
		[]uint16{1, 0},
		1,
	); admitted || reason != "foreign_arena_owner" {
		t.Fatalf("foreign inline verification=%t reason=%q", admitted, reason)
	}
	afterForeignInline := g18FutureDecodeCoreSnapshot(t, providerA)
	if afterForeignInline.OwnerCheckedLookups != beforeForeignInline.OwnerCheckedLookups+1 {
		t.Fatalf("foreign inline owner-check attempts: before=%d after=%d, want one rejected attempt", beforeForeignInline.OwnerCheckedLookups, afterForeignInline.OwnerCheckedLookups)
	}
	g18FutureRequireNoCertificateReads(t, beforeForeignInline, afterForeignInline)

	spillA := g18FutureBeginCohort(t, providerA, 3, uint32(len(record)))
	spillB := g18FutureBeginCohort(t, providerB, 3, uint32(len(record)))
	for index := 0; index < 3; index++ {
		g18FutureWriteMember(t, providerA, spillA, headsA[index], uint16(index), action, digest, record)
		g18FutureWriteMember(t, providerB, spillB, headsB[index], uint16(index), action, digest, record)
	}
	if spillA[1] != spillB[1] || spillA[2] != spillB[2] {
		t.Fatalf("spill equal-session handles A=%v B=%v", spillA, spillB)
	}
	if err := providerA.DiagnosticDropCohortFinalizeForTest(spillA); err != nil {
		t.Fatal(err)
	}
	if err := providerB.DiagnosticDropCohortFinalizeForTest(spillB); err != nil {
		t.Fatal(err)
	}
	if cohort := g18FutureFindCohort(t, g18FutureDecodeCoreSnapshot(t, providerA), spillA); !cohort.Spilled {
		t.Fatalf("arena A spill cohort=%+v", cohort)
	}
	beforeLocalSpill := g18FutureDecodeCoreSnapshot(t, providerA)
	if admitted, reason := g18FutureSchedulerVerify(
		t,
		schedulerA,
		providerA,
		headsA[:],
		[]g18FutureCohortHandle{spillA, spillA, spillA},
		[]uint16{0, 1, 2},
		1,
		2,
	); !admitted || reason != "proved" {
		t.Fatalf("arena A spill verification=%t reason=%q", admitted, reason)
	}
	afterLocalSpill := g18FutureDecodeCoreSnapshot(t, providerA)
	if afterLocalSpill.OwnerCheckedLookups != beforeLocalSpill.OwnerCheckedLookups+3 {
		t.Fatalf("local spill owner-check attempts: before=%d after=%d, want three attempts", beforeLocalSpill.OwnerCheckedLookups, afterLocalSpill.OwnerCheckedLookups)
	}
	beforeForeignSpill := afterLocalSpill
	if admitted, reason := g18FutureSchedulerVerify(
		t,
		schedulerA,
		providerA,
		headsA[:],
		[]g18FutureCohortHandle{spillB, spillA, spillA},
		[]uint16{1, 0, 0},
		1,
		2,
	); admitted || reason != "foreign_arena_owner" {
		t.Fatalf("foreign spill verification=%t reason=%q", admitted, reason)
	}
	afterForeignSpill := g18FutureDecodeCoreSnapshot(t, providerA)
	if afterForeignSpill.OwnerCheckedLookups != beforeForeignSpill.OwnerCheckedLookups+1 {
		t.Fatalf("foreign spill owner-check attempts: before=%d after=%d, want one rejected attempt", beforeForeignSpill.OwnerCheckedLookups, afterForeignSpill.OwnerCheckedLookups)
	}
	g18FutureRequireNoCertificateReads(t, beforeForeignSpill, afterForeignSpill)

	stale := spillA
	if err := compactA.Reset(); err != nil {
		t.Fatal(err)
	}
	resetOwner, resetEpoch := providerA.DiagnosticDropCohortArenaIdentityForTest()
	if resetOwner != ownerA {
		t.Fatalf("reset owner=%d, old=%d", resetOwner, ownerA)
	}
	if resetEpoch == epochA {
		t.Fatalf("reset epoch=%d, old=%d", resetEpoch, epochA)
	}
	for index := range headsA {
		head, err := compactA.Seed(core.StateID(index+1), 0)
		if err != nil {
			t.Fatal(err)
		}
		headsA[index] = head
	}
	fresh := g18FutureBeginCohort(t, providerA, 2, uint32(len(record)))
	for index := 0; index < 2; index++ {
		g18FutureWriteMember(t, providerA, fresh, headsA[index], uint16(index), action, digest, record)
	}
	if err := providerA.DiagnosticDropCohortFinalizeForTest(fresh); err != nil {
		t.Fatal(err)
	}
	beforeFreshProof := g18FutureDecodeCoreSnapshot(t, providerA)
	if admitted, reason := g18FutureSchedulerVerify(
		t,
		schedulerA,
		providerA,
		headsA[:2],
		[]g18FutureCohortHandle{fresh, fresh},
		[]uint16{0, 1},
		1,
	); !admitted || reason != "proved" {
		t.Fatalf("fresh post-reset verification=%t reason=%q", admitted, reason)
	}
	afterFreshProof := g18FutureDecodeCoreSnapshot(t, providerA)
	if afterFreshProof.OwnerCheckedLookups != beforeFreshProof.OwnerCheckedLookups+2 {
		t.Fatalf("fresh post-reset owner-check attempts: before=%d after=%d, want two attempts", beforeFreshProof.OwnerCheckedLookups, afterFreshProof.OwnerCheckedLookups)
	}
	// Snapshot after the valid local proof before testing the stale foreign epoch.
	beforeResetStale := afterFreshProof
	if admitted, reason := g18FutureSchedulerVerify(
		t,
		schedulerA,
		providerA,
		headsA[:2],
		[]g18FutureCohortHandle{stale, fresh},
		[]uint16{1, 0},
		1,
	); admitted || reason != "stale_arena_epoch" {
		t.Fatalf("reset-stale verification=%t reason=%q", admitted, reason)
	}
	resetAfter := g18FutureDecodeCoreSnapshot(t, providerA)
	if resetAfter.OwnerCheckedLookups != beforeResetStale.OwnerCheckedLookups+1 {
		t.Fatalf("reset-stale owner-check attempts=%d, want %d", resetAfter.OwnerCheckedLookups, beforeResetStale.OwnerCheckedLookups+1)
	}
	g18FutureRequireNoCertificateReads(t, beforeResetStale, resetAfter)
}

func TestG18FutureRealVerifierAllocationsRED(t *testing.T) {
	scheduler, provider, heads := g18FutureBehaviorFixture(t)
	action := g18FutureActionIdentity()
	record, digest := g18FutureDerivationRecord()
	handle := g18FutureBeginCohort(t, provider, 2, uint32(len(record)))
	g18FutureWriteMember(t, provider, handle, heads[0], 0, action, digest, record)
	g18FutureWriteMember(t, provider, handle, heads[1], 1, action, digest, record)
	if err := provider.DiagnosticDropCohortFinalizeForTest(handle); err != nil {
		t.Fatal(err)
	}
	scheduler.headers = []diagnosticParserCoreHeader{
		{head: heads[0]},
		{head: heads[1]},
	}
	g18RequireValidSummaryHeads(t, scheduler)
	dropper, ok := any(scheduler).(g18FutureSchedulerDropBehavior)
	if !ok {
		t.Fatal("RED: scheduler does not implement the real verifier API")
	}
	if err := dropper.DiagnosticBindDropCohortReferencesForTest(
		[]g18FutureCohortHandle{handle, handle}, []uint16{0, 1},
	); err != nil {
		t.Fatal(err)
	}
	boundHeaders := append([]diagnosticParserCoreHeader(nil), scheduler.headers...)
	if len(boundHeaders) != 2 {
		t.Fatalf("bound verifier headers=%d, want exactly two", len(boundHeaders))
	}
	beforeSnapshot := g18FutureDecodeCoreSnapshot(t, provider)
	beforeStateDigest := dropper.DiagnosticDropGenericNoActionHeadsVerifierStateDigestForTest()
	drops := [...]int{1}
	warmReason, warmInvocation, warmStateDigest, warmErr := dropper.DiagnosticDropGenericNoActionHeadsNonDestructiveForTest(drops[:])
	if warmErr != nil || warmReason != "proved" || warmStateDigest != beforeStateDigest {
		t.Fatalf("real verifier warm-up reason=%q invocation=%d state=%x err=%v", warmReason, warmInvocation, warmStateDigest, warmErr)
	}
	if !reflect.DeepEqual(scheduler.headers, boundHeaders) {
		t.Fatal("non-destructive warm-up changed the bound headers")
	}
	if afterWarmup := g18FutureDecodeCoreSnapshot(t, provider); !reflect.DeepEqual(afterWarmup, beforeSnapshot) {
		t.Fatalf("non-destructive warm-up changed certificate state: before=%+v after=%+v", beforeSnapshot, afterWarmup)
	}
	var reason string
	var invocation uint64
	var stateDigest [sha256.Size]byte
	var err error
	var invalidCalls uint64
	nextInvocation := warmInvocation + 1
	if got := testing.AllocsPerRun(1000, func() {
		reason, invocation, stateDigest, err = dropper.DiagnosticDropGenericNoActionHeadsNonDestructiveForTest(drops[:])
		if err != nil || reason != "proved" || invocation != nextInvocation || stateDigest != beforeStateDigest {
			invalidCalls++
		}
		nextInvocation++
	}); got != 0 {
		t.Fatalf("real future verifier allocations=%v, want 0 (last reason=%q invocation=%d state=%x err=%v)", got, reason, invocation, stateDigest, err)
	}
	if invalidCalls != 0 || invocation != warmInvocation+1001 || nextInvocation != warmInvocation+1002 {
		t.Fatalf("real future verifier calls=%d invocation=%d next=%d, want 1001 valid calls after warm-up invocation %d", invalidCalls, invocation, nextInvocation, warmInvocation)
	}
	if !reflect.DeepEqual(scheduler.headers, boundHeaders) {
		t.Fatal("measured non-destructive verifier calls changed the bound headers")
	}
	if afterMeasured := g18FutureDecodeCoreSnapshot(t, provider); !reflect.DeepEqual(afterMeasured, beforeSnapshot) {
		t.Fatalf("measured non-destructive verifier calls changed certificate state: before=%+v after=%+v", beforeSnapshot, afterMeasured)
	}
	if afterStateDigest := dropper.DiagnosticDropGenericNoActionHeadsVerifierStateDigestForTest(); afterStateDigest != beforeStateDigest {
		t.Fatalf("measured non-destructive verifier calls changed state digest: before=%x after=%x", beforeStateDigest, afterStateDigest)
	}
}

func g18FutureRequireCounterWrapRED(
	t *testing.T,
	name string,
	probe func(g18FutureCoreCertificateBehavior) (uint64, uint64, error),
) {
	t.Helper()
	compact, err := core.New(&genericConflictTable{}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	provider := g18RequireFutureCoreBehaviorRED(t, compact)
	before := g18FutureDecodeCoreSnapshot(t, provider)
	lastAccepted, rejected, probeErr := probe(provider)
	if probeErr == nil || !strings.Contains(probeErr.Error(), "overflow") {
		t.Fatalf("%s wrap error=%v, want overflow", name, probeErr)
	}
	if lastAccepted != math.MaxUint64 || rejected != 0 {
		t.Fatalf("%s wrap values accepted=%d rejected=%d, want max/zero", name, lastAccepted, rejected)
	}
	after := g18FutureDecodeCoreSnapshot(t, provider)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("%s wrap probe changed the real arena: before=%+v after=%+v", name, before, after)
	}
}

func TestG18FutureRealAtomicOwnerWrapRED(t *testing.T) {
	g18FutureRequireCounterWrapRED(t, "owner", func(provider g18FutureCoreCertificateBehavior) (uint64, uint64, error) {
		return provider.DiagnosticDropCohortOwnerWrapProbeForTest()
	})
}

func TestG18FutureRealAtomicEpochWrapRED(t *testing.T) {
	g18FutureRequireCounterWrapRED(t, "epoch", func(provider g18FutureCoreCertificateBehavior) (uint64, uint64, error) {
		return provider.DiagnosticDropCohortEpochWrapProbeForTest()
	})
}

func TestG18FutureRealAtomicSequenceWrapRED(t *testing.T) {
	g18FutureRequireCounterWrapRED(t, "sequence", func(provider g18FutureCoreCertificateBehavior) (uint64, uint64, error) {
		return provider.DiagnosticDropCohortSequenceWrapProbeForTest()
	})
}

func TestG18FutureRealDefaultDropCohortMembersRED(t *testing.T) {
	_, provider, heads := g18FutureBehaviorFixture(t)
	maxCohorts, maxMembers := provider.DiagnosticDropCohortLimitsForTest()
	if maxCohorts != 4096 || maxMembers != 32 {
		t.Fatalf("real drop-cohort defaults=%d/%d, want MaxDropCohorts=4096 MaxDropCohortMembers=32", maxCohorts, maxMembers)
	}
	action := g18FutureActionIdentity()
	record, digest := g18FutureDerivationRecord()
	handle := g18FutureBeginCohort(t, provider, 32, uint32(len(record)))
	for index := 0; index < int(maxMembers); index++ {
		g18FutureWriteMember(t, provider, handle, heads[index%len(heads)], uint16(index), action, digest, record)
	}
	if err := provider.DiagnosticDropCohortFinalizeForTest(handle); err != nil {
		t.Fatal(err)
	}
	g18FutureRequireCohortState(t, provider, handle, "complete", 32, 32)
}

func TestG18FutureRealMaxDropCohortsCapOverflowRED(t *testing.T) {
	_, provider, _ := g18FutureBehaviorFixture(t)
	if err := provider.DiagnosticDropCohortSetLimitsForTest(2, 32); err != nil {
		t.Fatal(err)
	}
	if maxCohorts, maxMembers := provider.DiagnosticDropCohortLimitsForTest(); maxCohorts != 2 || maxMembers != 32 {
		t.Fatalf("real cohort limits after setter=%d/%d, want 2/32", maxCohorts, maxMembers)
	}
	record, _ := g18FutureDerivationRecord()
	for index := 0; index < 2; index++ {
		g18FutureBeginCohort(t, provider, 1, uint32(len(record)))
	}
	beforeOverflow := g18FutureDecodeCoreSnapshot(t, provider)
	if _, err := provider.DiagnosticDropCohortBeginForTest(1, uint32(len(record)), g18FutureUnlimitedCaps()); err == nil {
		t.Fatal("MaxDropCohorts overflow was accepted")
	}
	afterOverflow := g18FutureDecodeCoreSnapshot(t, provider)
	if !reflect.DeepEqual(afterOverflow, beforeOverflow) {
		t.Fatalf("cohort-cap preflight changed real arena: before=%+v after=%+v", beforeOverflow, afterOverflow)
	}
}

func TestG18FutureRealMaxDropCohortMembersCapOverflowRED(t *testing.T) {
	_, provider, heads := g18FutureBehaviorFixture(t)
	if err := provider.DiagnosticDropCohortSetLimitsForTest(4, 2); err != nil {
		t.Fatal(err)
	}
	if maxCohorts, maxMembers := provider.DiagnosticDropCohortLimitsForTest(); maxCohorts != 4 || maxMembers != 2 {
		t.Fatalf("real cohort limits after setter=%d/%d, want 4/2", maxCohorts, maxMembers)
	}
	record, digest := g18FutureDerivationRecord()
	before := g18FutureDecodeCoreSnapshot(t, provider)
	if _, err := provider.DiagnosticDropCohortBeginForTest(3, uint32(len(record)), g18FutureUnlimitedCaps()); err == nil {
		t.Fatal("MaxDropCohortMembers overflow was accepted")
	}
	afterRejected := g18FutureDecodeCoreSnapshot(t, provider)
	if !reflect.DeepEqual(afterRejected, before) {
		t.Fatalf("member-cap preflight changed real arena: before=%+v after=%+v", before, afterRejected)
	}
	handle := g18FutureBeginCohort(t, provider, 2, uint32(len(record)))
	g18FutureWriteMember(t, provider, handle, heads[0], 0, g18FutureActionIdentity(), digest, record)
	if err := provider.DiagnosticDropCohortWriteForTest(handle, heads[1], 1, g18FutureActionIdentity(), digest, record); err != nil {
		t.Fatal(err)
	}
	if err := provider.DiagnosticDropCohortWriteForTest(handle, heads[2], 2, g18FutureActionIdentity(), digest, record); err == nil {
		t.Fatal("member-cap overflow after reservation was accepted")
	}
	g18FutureRequireCohortState(t, provider, handle, "overflowed", 2, 2)
}

func TestG18FutureRealCertificateLifecycleRED(t *testing.T) {
	scheduler, provider, heads := g18FutureBehaviorFixture(t)
	action := g18FutureActionIdentity()
	record, digest := g18FutureDerivationRecord()
	before := g18FutureDecodeCoreSnapshot(t, provider)
	building := g18FutureBeginCohort(t, provider, 2, uint32(len(record)))
	afterBegin := g18FutureRequireCohortState(t, provider, building, "building", 2, 0)
	wantStorage, wantFootprint := g18FutureAccountingPlan(2, uint32(len(record)))
	if got := g18FutureSubtractStores(t, afterBegin.Storage, before.Storage); got != wantStorage {
		t.Fatalf("reserved storage=%v, want %v", got, wantStorage)
	}
	if got := g18FutureSubtractStores(t, afterBegin.Footprint, before.Footprint); got != wantFootprint {
		t.Fatalf("reserved footprint=%v, want %v", got, wantFootprint)
	}
	g18FutureWriteMember(t, provider, building, heads[0], 0, action, digest, record)
	g18FutureRequireCohortState(t, provider, building, "building", 2, 1)
	if admitted, reason := g18FutureSchedulerVerify(
		t,
		scheduler,
		provider,
		heads[:2],
		[]g18FutureCohortHandle{building, building},
		[]uint16{0, 1},
		1,
	); admitted || reason != "certificate_building" {
		t.Fatalf("partial exposure verification=%t reason=%q", admitted, reason)
	}
	if err := provider.DiagnosticDropCohortFinalizeForTest(building); err == nil {
		t.Fatal("partial certificate finalized")
	}
	if err := provider.DiagnosticDropCohortWriteForTest(building, heads[0], 0, action, digest, record); err == nil {
		t.Fatal("duplicate certificate member was accepted")
	}
	g18FutureRequireCohortState(t, provider, building, "building", 2, 1)
	g18FutureWriteMember(t, provider, building, heads[1], 1, action, digest, record)
	if err := provider.DiagnosticDropCohortFinalizeForTest(building); err != nil {
		t.Fatal(err)
	}
	complete := g18FutureRequireCohortState(t, provider, building, "complete", 2, 2)
	if complete.Storage != afterBegin.Storage || complete.Footprint != afterBegin.Footprint {
		t.Fatalf("producer writes changed reserved bytes: begin=%+v complete=%+v", afterBegin, complete)
	}
	if admitted, reason := g18FutureSchedulerVerify(
		t,
		scheduler,
		provider,
		heads[:2],
		[]g18FutureCohortHandle{building, building},
		[]uint16{0, 1},
		1,
	); !admitted || reason != "proved" {
		t.Fatalf("complete verification=%t reason=%q", admitted, reason)
	}

	overflowed := g18FutureBeginCohort(t, provider, 1, uint32(len(record)))
	g18FutureWriteMember(t, provider, overflowed, heads[0], 0, action, digest, record)
	if err := provider.DiagnosticDropCohortWriteForTest(overflowed, heads[1], 1, action, digest, record); err == nil {
		t.Fatal("certificate overflow was accepted")
	}
	g18FutureRequireCohortState(t, provider, overflowed, "overflowed", 1, 1)
	if err := provider.DiagnosticDropCohortFinalizeForTest(overflowed); err == nil {
		t.Fatal("overflowed certificate finalized")
	}

	blended := g18FutureBeginCohort(t, provider, 2, uint32(len(record)))
	g18FutureWriteMember(t, provider, blended, heads[0], 0, action, digest, record)
	conflict := action
	conflict[2]++
	if err := provider.DiagnosticDropCohortWriteForTest(blended, heads[1], 0, conflict, digest, record); err == nil {
		t.Fatal("conflicting branch was accepted")
	}
	g18FutureRequireCohortState(t, provider, blended, "blended", 2, 1)
	if err := provider.DiagnosticDropCohortFinalizeForTest(blended); err == nil {
		t.Fatal("blended certificate finalized")
	}

	unproved := g18FutureBeginCohort(t, provider, 2, uint32(len(record)))
	for index := 0; index < 2; index++ {
		g18FutureWriteMember(t, provider, unproved, heads[index], uint16(index), action, digest, record)
	}
	if err := provider.DiagnosticDropCohortMarkUnprovedForTest(unproved); err != nil {
		t.Fatal(err)
	}
	g18FutureRequireCohortState(t, provider, unproved, "unproved", 2, 2)
	if err := provider.DiagnosticDropCohortWriteForTest(unproved, heads[0], 0, action, digest, record); err == nil {
		t.Fatal("unproved certificate accepted a later write")
	}
	if admitted, reason := g18FutureSchedulerVerify(
		t,
		scheduler,
		provider,
		heads[:2],
		[]g18FutureCohortHandle{unproved, unproved},
		[]uint16{0, 1},
		1,
	); admitted || reason != "certificate_unproved" {
		t.Fatalf("unproved verification=%t reason=%q", admitted, reason)
	}

	rollbackBefore := g18FutureDecodeCoreSnapshot(t, provider)
	rolledBack := g18FutureBeginCohort(t, provider, 1, uint32(len(record)))
	g18FutureWriteMember(t, provider, rolledBack, heads[0], 0, action, digest, record)
	if err := provider.DiagnosticDropCohortRollbackForTest(rolledBack); err != nil {
		t.Fatal(err)
	}
	rollbackAfter := g18FutureDecodeCoreSnapshot(t, provider)
	if !reflect.DeepEqual(rollbackAfter, rollbackBefore) {
		t.Fatalf("rollback snapshot differs: before=%+v after=%+v", rollbackBefore, rollbackAfter)
	}
	if admitted, reason := g18FutureSchedulerVerify(
		t,
		scheduler,
		provider,
		heads[:2],
		[]g18FutureCohortHandle{rolledBack, rolledBack},
		[]uint16{0, 0},
		1,
	); admitted || reason != "unknown_cohort" {
		t.Fatalf("rolled-back verification=%t reason=%q", admitted, reason)
	}
}

func TestG18FutureSequentialAndNestedCohortsRED(t *testing.T) {
	scheduler, provider, heads := g18FutureBehaviorFixture(t)
	action := g18FutureActionIdentity()
	record, digest := g18FutureDerivationRecord()
	complete := func(handle g18FutureCohortHandle, first, second int) {
		g18FutureWriteMember(t, provider, handle, heads[first], 0, action, digest, record)
		g18FutureWriteMember(t, provider, handle, heads[second], 1, action, digest, record)
		if err := provider.DiagnosticDropCohortFinalizeForTest(handle); err != nil {
			t.Fatal(err)
		}
	}
	first := g18FutureBeginCohort(t, provider, 2, uint32(len(record)))
	complete(first, 0, 1)
	second := g18FutureBeginCohort(t, provider, 2, uint32(len(record)))
	complete(second, 0, 1)
	if second[2] != first[2]+1 {
		t.Fatalf("sequential handles first=%v second=%v", first, second)
	}
	if admitted, reason := g18FutureSchedulerVerify(
		t,
		scheduler,
		provider,
		heads[:2],
		[]g18FutureCohortHandle{first, second},
		[]uint16{0, 1},
		1,
	); admitted || reason != "foreign_cohort_sequence" {
		t.Fatalf("sequential blend verification=%t reason=%q", admitted, reason)
	}

	outer := g18FutureBeginCohort(t, provider, 2, uint32(len(record)))
	g18FutureWriteMember(t, provider, outer, heads[0], 0, action, digest, record)
	inner := g18FutureBeginCohort(t, provider, 2, uint32(len(record)))
	complete(inner, 0, 1)
	g18FutureWriteMember(t, provider, outer, heads[1], 1, action, digest, record)
	if err := provider.DiagnosticDropCohortFinalizeForTest(outer); err != nil {
		t.Fatal(err)
	}
	if inner[2] != outer[2]+1 {
		t.Fatalf("nested handles outer=%v inner=%v", outer, inner)
	}
	for name, handle := range map[string]g18FutureCohortHandle{"outer": outer, "inner": inner} {
		if admitted, reason := g18FutureSchedulerVerify(
			t,
			scheduler,
			provider,
			heads[:2],
			[]g18FutureCohortHandle{handle, handle},
			[]uint16{0, 1},
			1,
		); !admitted || reason != "proved" {
			t.Fatalf("%s nested verification=%t reason=%q", name, admitted, reason)
		}
	}
}

func TestG18FutureAtomicPreflightForEveryStoreRED(t *testing.T) {
	storeNames := []string{
		"action_records",
		"derivation_records",
		"certificate_references",
		"map_entries",
		"interner_entries",
		"journal_entries",
		"cohort_records",
	}
	record, _ := g18FutureDerivationRecord()
	_, footprint := g18FutureAccountingPlan(2, uint32(len(record)))
	for store, name := range storeNames {
		store := store
		name := name
		t.Run(name, func(t *testing.T) {
			_, provider, _ := g18FutureBehaviorFixture(t)
			before := g18FutureDecodeCoreSnapshot(t, provider)
			caps := g18FutureUnlimitedCaps()
			caps[store] = footprint[store] - 1
			if _, err := provider.DiagnosticDropCohortBeginForTest(2, uint32(len(record)), caps); err == nil {
				t.Fatal("over-limit preflight was accepted")
			}
			after := g18FutureDecodeCoreSnapshot(t, provider)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("failed preflight changed state: before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestG18FutureExactStorageAndFootprintRED(t *testing.T) {
	record, _ := g18FutureDerivationRecord()
	for _, expected := range []uint16{1, 2, 3} {
		expected := expected
		t.Run(fmt.Sprintf("members_%d", expected), func(t *testing.T) {
			_, provider, _ := g18FutureBehaviorFixture(t)
			before := g18FutureDecodeCoreSnapshot(t, provider)
			handle := g18FutureBeginCohort(t, provider, expected, uint32(len(record)))
			after := g18FutureRequireCohortState(t, provider, handle, "building", expected, 0)
			wantStorage, wantFootprint := g18FutureAccountingPlan(expected, uint32(len(record)))
			if got := g18FutureSubtractStores(t, after.Storage, before.Storage); got != wantStorage {
				t.Fatalf("storage delta=%v, want %v", got, wantStorage)
			}
			if got := g18FutureSubtractStores(t, after.Footprint, before.Footprint); got != wantFootprint {
				t.Fatalf("footprint delta=%v, want %v", got, wantFootprint)
			}
			if after.StorageBytes-before.StorageBytes != g18FutureSumStores(wantStorage) ||
				after.FootprintBytes-before.FootprintBytes != g18FutureSumStores(wantFootprint) {
				t.Fatalf("byte totals before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestG18FutureEveryActionIdentityFieldRED(t *testing.T) {
	fieldNames := []string{
		"state",
		"lookahead",
		"ordinal",
		"type",
		"target_state",
		"symbol",
		"production_id",
		"child_count",
		"dynamic_precedence",
		"extra",
		"extra_chain",
		"repetition",
		"no_lookahead",
		"selection_class",
	}
	record, digest := g18FutureDerivationRecord()
	for field, name := range fieldNames {
		field := field
		name := name
		t.Run(name, func(t *testing.T) {
			scheduler, provider, heads := g18FutureBehaviorFixture(t)
			handle := g18FutureBeginCohort(t, provider, 2, uint32(len(record)))
			base := g18FutureActionIdentity()
			candidate := base
			if field >= 9 && field <= 12 {
				candidate[field] = 1 - candidate[field]
			} else {
				candidate[field]++
			}
			g18FutureWriteMember(t, provider, handle, heads[0], 0, base, digest, record)
			g18FutureWriteMember(t, provider, handle, heads[1], 1, candidate, digest, record)
			if err := provider.DiagnosticDropCohortFinalizeForTest(handle); err != nil {
				t.Fatal(err)
			}
			if admitted, reason := g18FutureSchedulerVerify(
				t,
				scheduler,
				provider,
				heads[:2],
				[]g18FutureCohortHandle{handle, handle},
				[]uint16{0, 1},
				1,
			); admitted || reason != "action_identity_mismatch" {
				t.Fatalf("field %s verification=%t reason=%q", name, admitted, reason)
			}
		})
	}
}

func TestG18FutureDerivationFullRecordEqualityRED(t *testing.T) {
	scheduler, provider, heads := g18FutureBehaviorFixture(t)
	action := g18FutureActionIdentity()
	digest := sha256.Sum256([]byte("forced equal digest"))
	recordA := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	recordB := []byte{1, 2, 3, 4, 5, 6, 7, 9}
	collision := g18FutureBeginCohort(t, provider, 2, uint32(len(recordA)))
	g18FutureWriteMember(t, provider, collision, heads[0], 0, action, digest, recordA)
	g18FutureWriteMember(t, provider, collision, heads[1], 1, action, digest, recordB)
	if err := provider.DiagnosticDropCohortFinalizeForTest(collision); err != nil {
		t.Fatal(err)
	}
	if admitted, reason := g18FutureSchedulerVerify(
		t,
		scheduler,
		provider,
		heads[:2],
		[]g18FutureCohortHandle{collision, collision},
		[]uint16{0, 1},
		1,
	); admitted || reason != "derivation_identity_mismatch" {
		t.Fatalf("equal-digest unequal-record verification=%t reason=%q", admitted, reason)
	}

	equal := g18FutureBeginCohort(t, provider, 2, uint32(len(recordA)))
	g18FutureWriteMember(t, provider, equal, heads[0], 0, action, digest, recordA)
	g18FutureWriteMember(t, provider, equal, heads[1], 1, action, digest, recordA)
	if err := provider.DiagnosticDropCohortFinalizeForTest(equal); err != nil {
		t.Fatal(err)
	}
	if admitted, reason := g18FutureSchedulerVerify(
		t,
		scheduler,
		provider,
		heads[:2],
		[]g18FutureCohortHandle{equal, equal},
		[]uint16{0, 1},
		1,
	); !admitted || reason != "proved" {
		t.Fatalf("full-record equality verification=%t reason=%q", admitted, reason)
	}
}

// The reference oracle below checks the predicate shape only. The RED tests
// above bind every future behavior to a real Core instance.
func g18ReferenceBuildCertificate(
	t *testing.T,
	arena *g18ReferenceCertificateArena,
	members ...g18ReferenceCohortMember,
) g18ReferenceCertificateHandle {
	t.Helper()
	handle, err := arena.newCertificate(uint8(len(members)), g18ReferencePlan(uint8(len(members))))
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range members {
		if err := arena.write(handle, member); err != nil {
			t.Fatal(err)
		}
	}
	if err := arena.finalize(handle); err != nil {
		t.Fatal(err)
	}
	return handle
}

func g18ReferenceRequireValidHeads(t *testing.T) (core.Head, core.Head) {
	t.Helper()
	scheduler, first, second := g18ValidDropScheduler(t)
	scheduler.headers = []diagnosticParserCoreHeader{{head: first}, {head: second}}
	g18RequireValidSummaryHeads(t, scheduler)
	return first, second
}

func g18ReferenceProof(
	head core.Head,
	handle g18ReferenceCertificateHandle,
	member g18ReferenceCohortMember,
) g18ReferenceHeadProof {
	return g18ReferenceHeadProof{Head: head, Certificate: handle, Member: member}
}

func g18ReferenceSamePayload(left, right g18ReferenceCertificate) bool {
	left.ID = g18ReferenceCohortID{}
	right.ID = g18ReferenceCohortID{}
	return left == right
}

func TestG18ReferenceArenaOwnerIdentityInlineAndSpill(t *testing.T) {
	firstHead, secondHead := g18ReferenceRequireValidHeads(t)
	arenaA := g18ReferenceNewArena(101)
	arenaB := g18ReferenceNewArena(202)
	if err := arenaA.beginSession(); err != nil {
		t.Fatal(err)
	}
	if err := arenaB.beginSession(); err != nil {
		t.Fatal(err)
	}
	inline := []g18ReferenceCohortMember{
		g18ReferenceMember(0, 3, 71), g18ReferenceMember(1, 3, 71),
	}
	inlineA := g18ReferenceBuildCertificate(t, &arenaA, inline...)
	inlineB := g18ReferenceBuildCertificate(t, &arenaB, inline...)
	if inlineA.ID.ArenaEpoch != inlineB.ID.ArenaEpoch ||
		inlineA.ID.CohortSequence != inlineB.ID.CohortSequence {
		t.Fatalf("equal-session identities differ: A=%+v B=%+v", inlineA.ID, inlineB.ID)
	}
	if !g18ReferenceSamePayload(
		arenaA.Certificates[inlineA.Index], arenaB.Certificates[inlineB.Index],
	) {
		t.Fatal("equal-session inline certificate payloads differ")
	}
	lookupsBefore := arenaA.OwnerCheckedLookups
	if !g18ReferenceVerifyDrop(
		&arenaA,
		g18ReferenceProof(firstHead, inlineA, inline[0]),
		g18ReferenceProof(secondHead, inlineA, inline[1]),
	) {
		t.Fatal("arena A rejected its valid inline certificate")
	}
	if got := arenaA.OwnerCheckedLookups - lookupsBefore; got != 2 {
		t.Fatalf("local inline owner-check attempts=%d, want 2", got)
	}
	lookupsBefore = arenaA.OwnerCheckedLookups
	if g18ReferenceVerifyDrop(
		&arenaA,
		g18ReferenceProof(firstHead, inlineB, inline[0]),
		g18ReferenceProof(secondHead, inlineA, inline[1]),
	) {
		t.Fatal("arena A accepted arena B inline identity")
	}
	if got := arenaA.OwnerCheckedLookups - lookupsBefore; got != 1 {
		t.Fatalf("foreign owner reached lookup: checked lookups=%d, want 1 survivor lookup", got)
	}

	spill := []g18ReferenceCohortMember{
		g18ReferenceMember(0, 5, 73), g18ReferenceMember(1, 5, 73),
		g18ReferenceMember(2, 5, 73),
	}
	spillA := g18ReferenceBuildCertificate(t, &arenaA, spill...)
	spillB := g18ReferenceBuildCertificate(t, &arenaB, spill...)
	if !arenaA.Certificates[spillA.Index].Spilled || !arenaB.Certificates[spillB.Index].Spilled {
		t.Fatal("three-member certificates did not use spill storage")
	}
	if spillA.ID.ArenaEpoch != spillB.ID.ArenaEpoch ||
		spillA.ID.CohortSequence != spillB.ID.CohortSequence ||
		!g18ReferenceSamePayload(
			arenaA.Certificates[spillA.Index], arenaB.Certificates[spillB.Index],
		) {
		t.Fatalf("equal-session spill identities or payloads differ: A=%+v B=%+v", spillA.ID, spillB.ID)
	}
	lookupsBefore = arenaA.OwnerCheckedLookups
	if !g18ReferenceVerifyDrop(
		&arenaA,
		g18ReferenceProof(firstHead, spillA, spill[0]),
		g18ReferenceProof(secondHead, spillA, spill[1]),
	) {
		t.Fatal("arena A rejected its valid spilled certificate")
	}
	if got := arenaA.OwnerCheckedLookups - lookupsBefore; got != 2 {
		t.Fatalf("local spill owner-check attempts=%d, want 2", got)
	}
	lookupsBefore = arenaA.OwnerCheckedLookups
	if g18ReferenceVerifyDrop(
		&arenaA,
		g18ReferenceProof(firstHead, spillB, spill[0]),
		g18ReferenceProof(secondHead, spillA, spill[1]),
	) {
		t.Fatal("arena A accepted arena B spilled identity")
	}
	if got := arenaA.OwnerCheckedLookups - lookupsBefore; got != 1 {
		t.Fatalf("foreign spilled owner reached lookup: checked lookups=%d, want 1", got)
	}

	staleA := spillA
	if err := arenaA.beginSession(); err != nil {
		t.Fatal(err)
	}
	fresh := g18ReferenceBuildCertificate(t, &arenaA, spill...)
	lookupsBefore = arenaA.OwnerCheckedLookups
	if g18ReferenceVerifyDrop(
		&arenaA,
		g18ReferenceProof(firstHead, staleA, spill[0]),
		g18ReferenceProof(secondHead, fresh, spill[1]),
	) {
		t.Fatal("reset arena A accepted its stale certificate")
	}
	if got := arenaA.OwnerCheckedLookups - lookupsBefore; got != 1 {
		t.Fatalf("reset-stale owner-check attempts=%d, want 1", got)
	}
}

func TestG18ReferenceFinalizationAndTerminalStates(t *testing.T) {
	arena := g18ReferenceNewArena(303)
	if err := arena.beginSession(); err != nil {
		t.Fatal(err)
	}
	partial, err := arena.newCertificate(2, g18ReferencePlan(2))
	if err != nil {
		t.Fatal(err)
	}
	first := g18ReferenceMember(0, 3, 71)
	if err := arena.write(partial, first); err != nil {
		t.Fatal(err)
	}
	if _, err := arena.lookup(partial, true); err == nil {
		t.Fatal("building certificate was exposed")
	}
	if err := arena.finalize(partial); err == nil {
		t.Fatal("partial certificate finalized")
	}
	if err := arena.write(partial, first); err == nil ||
		arena.Certificates[partial.Index].WrittenMembers != 1 {
		t.Fatalf("duplicate write error=%v certificate=%+v", err, arena.Certificates[partial.Index])
	}
	if err := arena.write(partial, g18ReferenceMember(1, 3, 71)); err != nil {
		t.Fatal(err)
	}
	if err := arena.finalize(partial); err != nil {
		t.Fatal(err)
	}
	if _, err := arena.lookup(partial, true); err != nil {
		t.Fatalf("complete certificate was not exposed: %v", err)
	}

	blended, err := arena.newCertificate(2, g18ReferencePlan(2))
	if err != nil {
		t.Fatal(err)
	}
	if err := arena.write(blended, first); err != nil {
		t.Fatal(err)
	}
	if err := arena.write(blended, g18ReferenceMember(0, 4, 72)); err == nil ||
		arena.Certificates[blended.Index].State != g18ReferenceCertificateBlended {
		t.Fatalf("blended write error=%v certificate=%+v", err, arena.Certificates[blended.Index])
	}

	overflowed, err := arena.newCertificate(1, g18ReferencePlan(1))
	if err != nil {
		t.Fatal(err)
	}
	if err := arena.write(overflowed, first); err != nil {
		t.Fatal(err)
	}
	if err := arena.write(overflowed, g18ReferenceMember(1, 3, 71)); err == nil ||
		arena.Certificates[overflowed.Index].State != g18ReferenceCertificateOverflowed {
		t.Fatalf("overflow write error=%v certificate=%+v", err, arena.Certificates[overflowed.Index])
	}

	unproved, err := arena.newCertificate(1, g18ReferencePlan(1))
	if err != nil {
		t.Fatal(err)
	}
	if err := arena.write(unproved, first); err != nil {
		t.Fatal(err)
	}
	if err := arena.markUnproved(unproved); err != nil {
		t.Fatal(err)
	}
	if _, err := arena.lookup(unproved, true); err == nil ||
		arena.Certificates[unproved.Index].State != g18ReferenceCertificateUnproved {
		t.Fatalf("unproved exposure error=%v certificate=%+v", err, arena.Certificates[unproved.Index])
	}
}

func TestG18ReferenceVerifierIdentityAndImportRules(t *testing.T) {
	firstHead, secondHead := g18ReferenceRequireValidHeads(t)
	arena := g18ReferenceNewArena(404)
	if err := arena.beginSession(); err != nil {
		t.Fatal(err)
	}
	members := []g18ReferenceCohortMember{
		g18ReferenceMember(0, 3, 71), g18ReferenceMember(1, 3, 71),
	}
	handle := g18ReferenceBuildCertificate(t, &arena, members...)
	survivor := g18ReferenceProof(firstHead, handle, members[0])
	dropped := g18ReferenceProof(secondHead, handle, members[1])
	if !g18ReferenceVerifyDrop(&arena, survivor, dropped) {
		t.Fatal("authenticated Ada import did not prove the exact derivation")
	}
	if g18ReferenceVerifyDrop(&arena, survivor) {
		t.Fatal("empty drop request was accepted")
	}
	zeroHead := dropped
	zeroHead.Head = core.Head{}
	if g18ReferenceVerifyDrop(&arena, survivor, zeroHead) {
		t.Fatal("zero drop head was accepted")
	}
	if g18ReferenceVerifyDrop(&arena, survivor, dropped, dropped) {
		t.Fatal("duplicate drop head was accepted")
	}

	actionMutations := []struct {
		name   string
		mutate func(*g18ReferenceActionIdentity)
	}{
		{name: "state", mutate: func(value *g18ReferenceActionIdentity) { value.State++ }},
		{name: "lookahead", mutate: func(value *g18ReferenceActionIdentity) { value.Lookahead++ }},
		{name: "ordinal", mutate: func(value *g18ReferenceActionIdentity) { value.Ordinal++ }},
		{name: "type", mutate: func(value *g18ReferenceActionIdentity) { value.Type++ }},
		{name: "target_state", mutate: func(value *g18ReferenceActionIdentity) { value.TargetState++ }},
		{name: "symbol", mutate: func(value *g18ReferenceActionIdentity) { value.Symbol++ }},
		{name: "production_id", mutate: func(value *g18ReferenceActionIdentity) { value.ProductionID++ }},
		{name: "child_count", mutate: func(value *g18ReferenceActionIdentity) { value.ChildCount++ }},
		{name: "dynamic_precedence", mutate: func(value *g18ReferenceActionIdentity) { value.DynamicPrecedence++ }},
		{name: "extra", mutate: func(value *g18ReferenceActionIdentity) { value.Extra = !value.Extra }},
		{name: "extra_chain", mutate: func(value *g18ReferenceActionIdentity) { value.ExtraChain = !value.ExtraChain }},
		{name: "repetition", mutate: func(value *g18ReferenceActionIdentity) { value.Repetition = !value.Repetition }},
		{name: "no_lookahead", mutate: func(value *g18ReferenceActionIdentity) { value.NoLookahead = !value.NoLookahead }},
		{name: "selection_class", mutate: func(value *g18ReferenceActionIdentity) { value.SelectionClass++ }},
	}
	for _, mutation := range actionMutations {
		mutation := mutation
		t.Run("action_"+mutation.name, func(t *testing.T) {
			actionMismatch := dropped
			mutation.mutate(&actionMismatch.Member.Action)
			if g18ReferenceVerifyDrop(&arena, survivor, actionMismatch) {
				t.Fatal("action identity mismatch was accepted")
			}
		})
	}
	derivationMismatch := dropped
	derivationMismatch.Member.Derivation.InternedRecord++
	if g18ReferenceVerifyDrop(&arena, survivor, derivationMismatch) {
		t.Fatal("Kotlin derivation mismatch was accepted")
	}
	digestCollision := dropped
	digestCollision.Member.Derivation.Record[0]++
	if digestCollision.Member.Derivation.Digest != dropped.Member.Derivation.Digest {
		t.Fatal("equal-digest collision setup changed the digest")
	}
	if g18ReferenceVerifyDrop(&arena, survivor, digestCollision) {
		t.Fatal("equal-digest unequal derivation record was accepted")
	}
	for _, mutate := range []func(*g18ReferenceCertificateHandle){
		func(value *g18ReferenceCertificateHandle) { value.ID.ArenaOwner++ },
		func(value *g18ReferenceCertificateHandle) { value.ID.ArenaEpoch++ },
		func(value *g18ReferenceCertificateHandle) { value.ID.ArenaEpoch = 0 },
		func(value *g18ReferenceCertificateHandle) { value.ID.CohortSequence++ },
	} {
		invalid := dropped
		mutate(&invalid.Certificate)
		if g18ReferenceVerifyDrop(&arena, survivor, invalid) {
			t.Fatal("reference verifier accepted an invalid identity")
		}
	}
	for _, state := range []g18ReferenceCertificateState{
		g18ReferenceCertificateBuilding,
		g18ReferenceCertificateOverflowed,
		g18ReferenceCertificateBlended,
		g18ReferenceCertificateUnproved,
	} {
		copyArena := arena
		copyArena.Certificates[handle.Index].State = state
		if g18ReferenceVerifyDrop(&copyArena, survivor, dropped) {
			t.Fatalf("reference verifier accepted state %d", state)
		}
	}
}

func TestG18ReferenceCompatibleCohortMerge(t *testing.T) {
	id := g18ReferenceCohortID{ArenaOwner: 505, ArenaEpoch: 1, CohortSequence: 1}
	left := g18ReferenceCertificate{
		ID: id, State: g18ReferenceCertificateBuilding, ExpectedMembers: 2, WrittenMembers: 1,
		Members: [g18ReferenceMaxMembers]g18ReferenceCohortMember{g18ReferenceMember(0, 3, 71)},
	}
	right := g18ReferenceCertificate{
		ID: id, State: g18ReferenceCertificateBuilding, ExpectedMembers: 1, WrittenMembers: 1,
		Members: [g18ReferenceMaxMembers]g18ReferenceCohortMember{g18ReferenceMember(1, 4, 73)},
	}
	if !left.compatibleMerge(right) || left.WrittenMembers != 2 ||
		left.State != g18ReferenceCertificateBuilding {
		t.Fatalf("compatible cohort merge=%+v", left)
	}

	conflictTarget := g18ReferenceCertificate{
		ID: id, State: g18ReferenceCertificateBuilding, ExpectedMembers: 2, WrittenMembers: 1,
		Members: [g18ReferenceMaxMembers]g18ReferenceCohortMember{g18ReferenceMember(0, 3, 71)},
	}
	conflict := g18ReferenceCertificate{
		ID: id, State: g18ReferenceCertificateBuilding, ExpectedMembers: 1, WrittenMembers: 1,
		Members: [g18ReferenceMaxMembers]g18ReferenceCohortMember{g18ReferenceMember(0, 5, 79)},
	}
	if conflictTarget.compatibleMerge(conflict) ||
		conflictTarget.State != g18ReferenceCertificateBlended || conflictTarget.WrittenMembers != 1 {
		t.Fatalf("conflicting cohort merge=%+v", conflictTarget)
	}

	partialTarget := g18ReferenceCertificate{
		ID: id, State: g18ReferenceCertificateBuilding, ExpectedMembers: 2, WrittenMembers: 1,
		Members: [g18ReferenceMaxMembers]g18ReferenceCohortMember{g18ReferenceMember(0, 3, 71)},
	}
	partialSource := g18ReferenceCertificate{
		ID: id, State: g18ReferenceCertificateBuilding, ExpectedMembers: 2, WrittenMembers: 1,
		Members: [g18ReferenceMaxMembers]g18ReferenceCohortMember{g18ReferenceMember(1, 4, 73)},
	}
	if partialTarget.compatibleMerge(partialSource) ||
		partialTarget.State != g18ReferenceCertificateBlended || partialTarget.WrittenMembers != 1 {
		t.Fatalf("partial cohort merge=%+v", partialTarget)
	}
}

func TestG18ReferenceSequentialAndNestedCohorts(t *testing.T) {
	firstHead, secondHead := g18ReferenceRequireValidHeads(t)
	arena := g18ReferenceNewArena(606)
	if err := arena.beginSession(); err != nil {
		t.Fatal(err)
	}
	firstMembers := []g18ReferenceCohortMember{
		g18ReferenceMember(0, 3, 71), g18ReferenceMember(1, 3, 71),
	}
	first := g18ReferenceBuildCertificate(t, &arena, firstMembers...)
	secondMembers := []g18ReferenceCohortMember{
		g18ReferenceMember(0, 4, 73), g18ReferenceMember(1, 4, 73),
	}
	second := g18ReferenceBuildCertificate(t, &arena, secondMembers...)
	if second.ID.CohortSequence != first.ID.CohortSequence+1 {
		t.Fatalf("sequential cohort identities: first=%+v second=%+v", first.ID, second.ID)
	}
	if g18ReferenceVerifyDrop(
		&arena,
		g18ReferenceProof(firstHead, first, firstMembers[0]),
		g18ReferenceProof(secondHead, second, secondMembers[1]),
	) {
		t.Fatal("sequential cohorts blended")
	}

	outer, err := arena.newCertificate(2, g18ReferencePlan(2))
	if err != nil {
		t.Fatal(err)
	}
	outerMembers := []g18ReferenceCohortMember{
		g18ReferenceMember(0, 5, 79), g18ReferenceMember(1, 5, 79),
	}
	if err := arena.write(outer, outerMembers[0]); err != nil {
		t.Fatal(err)
	}
	innerMembers := []g18ReferenceCohortMember{
		g18ReferenceMember(0, 6, 83), g18ReferenceMember(1, 6, 83),
	}
	inner := g18ReferenceBuildCertificate(t, &arena, innerMembers...)
	if err := arena.write(outer, outerMembers[1]); err != nil {
		t.Fatal(err)
	}
	if err := arena.finalize(outer); err != nil {
		t.Fatal(err)
	}
	if inner.ID.CohortSequence != outer.ID.CohortSequence+1 ||
		!g18ReferenceVerifyDrop(
			&arena,
			g18ReferenceProof(firstHead, outer, outerMembers[0]),
			g18ReferenceProof(secondHead, outer, outerMembers[1]),
		) ||
		!g18ReferenceVerifyDrop(
			&arena,
			g18ReferenceProof(firstHead, inner, innerMembers[0]),
			g18ReferenceProof(secondHead, inner, innerMembers[1]),
		) {
		t.Fatalf("nested cohort identities: outer=%+v inner=%+v", outer.ID, inner.ID)
	}
}

func TestG18ReferenceArenaLifecycleAndAccounting(t *testing.T) {
	firstHead, secondHead := g18ReferenceRequireValidHeads(t)
	var ownerCounter uint64
	owner, err := g18ReferenceAllocateOwner(&ownerCounter)
	if err != nil || owner == 0 {
		t.Fatalf("allocate arena owner=%d err=%v", owner, err)
	}
	ownerOverflow := uint64(math.MaxUint64)
	if _, err := g18ReferenceAllocateOwner(&ownerOverflow); err == nil ||
		!strings.Contains(err.Error(), "overflow") {
		t.Fatalf("owner wrap error=%v", err)
	}
	zeroOwner := g18ReferenceNewArena(0)
	if err := zeroOwner.beginSession(); err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("zero-owner session error=%v", err)
	}

	arena := g18ReferenceNewArena(707)
	if err := arena.beginSession(); err != nil {
		t.Fatal(err)
	}
	plan := g18ReferencePlan(2)
	old := g18ReferenceBuildCertificate(
		t, &arena, g18ReferenceMember(0, 3, 71), g18ReferenceMember(1, 3, 71),
	)
	if got := arena.storageBytes(); got != plan.StorageBytes {
		t.Fatalf("storage bytes=%d, want %d", got, plan.StorageBytes)
	}
	wantFootprint := uint64(unsafe.Sizeof(arena))
	if got := arena.footprintBytes(); got != wantFootprint {
		t.Fatalf("footprint bytes=%d, want %d", got, wantFootprint)
	}

	transactionBefore := arena
	temporary, err := arena.newCertificate(1, g18ReferencePlan(1))
	if err != nil {
		t.Fatal(err)
	}
	if err := arena.write(temporary, g18ReferenceMember(0, 4, 73)); err != nil {
		t.Fatal(err)
	}
	arena = transactionBefore
	if arena != transactionBefore {
		t.Fatal("rollback did not restore all arena values")
	}
	if _, err := arena.lookup(temporary, true); err == nil {
		t.Fatal("rollback left a certificate reference visible")
	}

	if err := arena.beginSession(); err != nil {
		t.Fatal(err)
	}
	if got := arena.footprintBytes(); got != wantFootprint {
		t.Fatalf("reset footprint bytes=%d, want retained %d", got, wantFootprint)
	}
	freshMembers := []g18ReferenceCohortMember{
		g18ReferenceMember(0, 3, 71), g18ReferenceMember(1, 3, 71),
	}
	fresh := g18ReferenceBuildCertificate(t, &arena, freshMembers...)
	if g18ReferenceVerifyDrop(
		&arena,
		g18ReferenceProof(firstHead, fresh, freshMembers[0]),
		g18ReferenceProof(secondHead, old, freshMembers[1]),
	) {
		t.Fatal("reset and reuse accepted a stale certificate")
	}

	for int(arena.Count) < len(arena.Certificates) {
		if _, err := arena.newCertificate(1, g18ReferencePlan(1)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := arena.newCertificate(1, g18ReferencePlan(1)); err == nil || !strings.Contains(err.Error(), "cap") {
		t.Fatalf("cohort cap error=%v", err)
	}

	epochOverflow := g18ReferenceNewArena(808)
	epochOverflow.Epoch = math.MaxUint64
	if err := epochOverflow.beginSession(); err == nil || !strings.Contains(err.Error(), "overflow") {
		t.Fatalf("epoch wrap error=%v", err)
	}
	sequenceOverflow := g18ReferenceNewArena(909)
	sequenceOverflow.Epoch = 1
	sequenceOverflow.NextSequence = math.MaxUint64
	if _, err := sequenceOverflow.newCertificate(1, g18ReferencePlan(1)); err == nil || !strings.Contains(err.Error(), "overflow") {
		t.Fatalf("sequence wrap error=%v", err)
	}
}

func TestG18ReferencePreflightIsAtomicForEveryStore(t *testing.T) {
	base := g18ReferenceNewArena(1001)
	if err := base.beginSession(); err != nil {
		t.Fatal(err)
	}
	plan := g18ReferencePlan(2)
	tests := []struct {
		name  string
		limit func(*g18ReferenceLimits)
	}{
		{name: "action_records", limit: func(value *g18ReferenceLimits) { value.MaxActionRecords = plan.ActionRecords - 1 }},
		{name: "derivation_records", limit: func(value *g18ReferenceLimits) { value.MaxDerivationRecords = plan.DerivationRecords - 1 }},
		{name: "certificate_references", limit: func(value *g18ReferenceLimits) { value.MaxCertificateReferences = plan.CertificateReferences - 1 }},
		{name: "map_entries", limit: func(value *g18ReferenceLimits) { value.MaxMapEntries = plan.MapEntries - 1 }},
		{name: "interner_entries", limit: func(value *g18ReferenceLimits) { value.MaxInternerEntries = plan.InternerEntries - 1 }},
		{name: "journal_entries", limit: func(value *g18ReferenceLimits) { value.MaxJournalEntries = plan.JournalEntries - 1 }},
		{name: "storage_bytes", limit: func(value *g18ReferenceLimits) { value.MaxStorageBytes = plan.StorageBytes - 1 }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			arena := base
			test.limit(&arena.Limits)
			before := arena
			if _, err := arena.newCertificate(2, plan); err == nil {
				t.Fatal("preflight accepted an over-limit reservation")
			}
			if arena != before {
				t.Fatalf("failed preflight wrote arena state: before=%+v after=%+v", before, arena)
			}
		})
	}

	incomplete := plan
	incomplete.JournalEntries = 0
	before := base
	if _, err := base.newCertificate(2, incomplete); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("incomplete preflight error=%v", err)
	}
	if base != before {
		t.Fatal("incomplete preflight wrote arena state")
	}
}

func TestG18ReferenceVerifierAllocations(t *testing.T) {
	firstHead, secondHead := g18ReferenceRequireValidHeads(t)
	arena := g18ReferenceNewArena(1111)
	if err := arena.beginSession(); err != nil {
		t.Fatal(err)
	}
	members := []g18ReferenceCohortMember{
		g18ReferenceMember(0, 3, 71), g18ReferenceMember(1, 3, 71),
	}
	handle := g18ReferenceBuildCertificate(t, &arena, members...)
	survivor := g18ReferenceProof(firstHead, handle, members[0])
	dropped := g18ReferenceProof(secondHead, handle, members[1])
	if got := testing.AllocsPerRun(1000, func() {
		if !g18ReferenceVerifyDrop(&arena, survivor, dropped) {
			panic("reference verifier declined")
		}
	}); got != 0 {
		t.Fatalf("reference verifier allocations=%v, want 0", got)
	}
}
