//go:build gts_parsercorephase0

package gotreesitter

import (
	"reflect"
	"strings"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestDiagnosticParserCoreSiblingAdoptionInheritedAlternatives(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	for _, test := range []struct {
		name                           string
		source, destination, output    [][2]uint16
		sourceBlended, destBlended     bool
		outputBlended, wantBlended     bool
		overflowSource, overflowOutput bool
		want                           []uint32
	}{
		{
			name:   "typescript-inherited-nested-member-without-output-set",
			source: [][2]uint16{{1, 0}, {2, 0}, {2, 1}}, destination: [][2]uint16{{1, 0}, {2, 0}},
			want: []uint32{65536, 131072, 131073},
		},
		{
			name:   "output-extension-preserves-existing-blended-veto",
			source: [][2]uint16{{1, 0}, {2, 0}}, destination: [][2]uint16{{1, 0}}, output: [][2]uint16{{3, 0}},
			want: []uint32{65536, 131072, 196608}, wantBlended: true,
		},
		{
			name:   "incomparable-source-and-sibling-blend",
			source: [][2]uint16{{2, 0}}, destination: [][2]uint16{{1, 0}},
			want: []uint32{65536, 131072}, wantBlended: true,
		},
		{
			name:   "source-blended-survives",
			source: [][2]uint16{{1, 0}}, destination: [][2]uint16{{1, 0}},
			sourceBlended: true, wantBlended: true, want: []uint32{65536},
		},
		{
			name:   "destination-blended-survives",
			source: [][2]uint16{{1, 0}}, destination: [][2]uint16{{1, 0}},
			destBlended: true, wantBlended: true, want: []uint32{65536},
		},
		{
			name:   "output-blended-survives",
			source: [][2]uint16{{1, 0}}, destination: [][2]uint16{{1, 0}}, output: [][2]uint16{{1, 0}},
			outputBlended: true, wantBlended: true, want: []uint32{65536},
		},
		{
			name:   "source-overflow-fails-closed",
			source: [][2]uint16{{1, 0}}, destination: [][2]uint16{{1, 0}}, overflowSource: true,
		},
		{
			name:   "output-overflow-fails-closed",
			source: [][2]uint16{{1, 0}}, destination: [][2]uint16{{1, 0}}, overflowOutput: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
			makeSet := func(members [][2]uint16, overflow bool) core.AlternativeSet {
				var set core.AlternativeSet
				for _, member := range members {
					compact.UnionAlternativeSet(&set, core.NewAlternativeSetMember(member[0], member[1]))
				}
				if overflow {
					for event := uint16(1); event <= 40; event++ {
						compact.UnionAlternativeSet(&set, core.NewAlternativeSetMember(event, 0))
					}
					if !set.Overflowed() {
						t.Fatal("fixture did not overflow")
					}
				}
				return set
			}
			scheduler := &diagnosticParserCoreGenericScheduler{
				compact: compact,
				headers: []diagnosticParserCoreHeader{
					{head: head, altSet: makeSet(test.source, test.overflowSource), blended: test.sourceBlended, convergedReductionSplit: true},
					{head: head, altSet: makeSet(test.destination, false), blended: test.destBlended, paused: true, creationSeq: 19},
				},
			}
			sourceBefore := scheduler.headers[0]
			adopted, err := scheduler.adoptUpdatedReductionSibling(0, head, core.CleanPathRankUnknown, 0,
				makeSet(test.output, test.overflowOutput), test.outputBlended, false, false)
			if err != nil || !adopted {
				t.Fatalf("adoption = %t, %v", adopted, err)
			}
			if !reflect.DeepEqual(scheduler.headers[0], sourceBefore) {
				t.Fatal("adoption changed the source header")
			}
			destination := scheduler.headers[1]
			if destination.paused || destination.creationSeq != 19 {
				t.Fatalf("destination lifecycle changed: %+v", destination)
			}
			wantOverflow := test.overflowSource || test.overflowOutput
			if destination.altSet.Overflowed() != wantOverflow {
				t.Fatalf("destination overflow = %t, want %t", destination.altSet.Overflowed(), wantOverflow)
			}
			if !wantOverflow {
				members, ok := compact.AlternativeSetMembers(destination.altSet)
				if !ok || !reflect.DeepEqual(members, test.want) {
					t.Fatalf("destination members = %v/%t, want %v", members, ok, test.want)
				}
				if destination.blended != test.wantBlended {
					t.Fatalf("destination blended = %t, want %t", destination.blended, test.wantBlended)
				}
			}
			dropIndices := []int{0}
			if wantOverflow {
				// A later drop must reject the inherited incomplete history.
				scheduler.headers[1].convergedReductionSplit = true
				dropIndices = []int{1}
			}
			count, proved := scheduler.diagnosticParserCoreConvergedCoverageDropsV2(dropIndices)
			wantProof := !wantOverflow && !test.wantBlended
			if proved != wantProof || (proved && count != 1) {
				t.Fatalf("drop proof = %d/%t, want proved=%t", count, proved, wantProof)
			}
		})
	}
}

func TestDiagnosticParserCoreSiblingAdoptionTwoStepDropLifecycle(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	compact, head, unrelated := newDiagnosticParserCoreCanonicalTestCore(t)
	member := core.NewAlternativeSetMember(2, 1)
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		receipt: &DiagnosticParserCoreGenericScheduler{},
		headers: []diagnosticParserCoreHeader{
			{head: head, altSet: member, convergedReductionSplit: true, creationSeq: 1},
			{head: head, paused: true, creationSeq: 2},
			{head: unrelated, altSet: core.NewAlternativeSetMember(3, 0), creationSeq: 3},
		},
	}
	adopted, err := scheduler.adoptUpdatedReductionSibling(0, head, core.CleanPathRankUnknown, 0,
		core.AlternativeSet{}, false, false, false)
	if err != nil || !adopted {
		t.Fatalf("adoption = %t, %v", adopted, err)
	}
	if !scheduler.headers[1].convergedReductionSplit {
		t.Fatal("destination lost the source convergence obligation")
	}
	if err := scheduler.dropGenericNoActionHeads([]int{0}); err != nil {
		t.Fatalf("covered source drop: %v", err)
	}
	if len(scheduler.headers) != 2 || scheduler.headers[0].creationSeq != 2 {
		t.Fatalf("source drop did not retain the destination: %+v", scheduler.headers)
	}
	before := append([]diagnosticParserCoreHeader(nil), scheduler.headers...)
	if err := scheduler.dropGenericNoActionHeads([]int{0}); err == nil ||
		!strings.Contains(err.Error(), "lacks alternative-set coverage") {
		t.Fatalf("uncovered destination drop = %v", err)
	}
	if !reflect.DeepEqual(scheduler.headers, before) {
		t.Fatal("rejected destination drop changed the frontier")
	}
}

func TestDiagnosticParserCoreSiblingAdoptionInheritedResurrectionVeto(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	compact, head, unrelated := newDiagnosticParserCoreCanonicalTestCore(t)
	member := core.NewAlternativeSetMember(2, 1)
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		headers: []diagnosticParserCoreHeader{
			{head: head, altSet: member, convergedReductionSplit: true, resurrectionUnproved: true, blended: true},
			{head: head, paused: true},
			{head: unrelated, altSet: member},
		},
	}
	adopted, err := scheduler.adoptUpdatedReductionSibling(0, head, core.CleanPathRankUnknown, 0,
		core.AlternativeSet{}, false, false, false)
	if err != nil || !adopted {
		t.Fatalf("adoption = %t, %v", adopted, err)
	}
	destination := scheduler.headers[1]
	if !destination.resurrectionUnproved || !destination.convergedReductionSplit || !destination.blended {
		t.Fatalf("destination lost inherited veto metadata: %+v", destination)
	}
	// The unrelated header supplies coverage, but cannot waive resurrection.
	if count, proved := scheduler.diagnosticParserCoreConvergedCoverageDropsV2([]int{1}); !proved || count != 1 {
		t.Fatalf("fixture coverage = %d/%t", count, proved)
	}
	before := append([]diagnosticParserCoreHeader(nil), scheduler.headers...)
	if err := scheduler.dropGenericNoActionHeads([]int{1}); err == nil ||
		!strings.Contains(err.Error(), "unproved historical boundary resurrection") {
		t.Fatalf("inherited resurrection drop = %v", err)
	}
	if !reflect.DeepEqual(scheduler.headers, before) {
		t.Fatal("rejected resurrection drop changed the frontier")
	}
}

func TestDiagnosticParserCoreSiblingAdoptionSpilledSourceRemainsUnchanged(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	var source core.AlternativeSet
	for event := uint16(1); event <= 12; event++ {
		compact.UnionAlternativeSet(&source, core.NewAlternativeSetMember(event, 0))
	}
	members, ok := compact.AlternativeSetMembers(source)
	if !ok || len(members) != 12 || source.Overflowed() {
		t.Fatalf("spilled source fixture = %v/%t", members, ok)
	}
	beforeMembers := append([]uint32(nil), members...)
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		headers: []diagnosticParserCoreHeader{
			{head: head, altSet: source, convergedReductionSplit: true},
			{head: head, altSet: core.NewAlternativeSetMember(1, 0), paused: true},
		},
	}
	beforeHeader := scheduler.headers[0]
	adopted, err := scheduler.adoptUpdatedReductionSibling(0, head, core.CleanPathRankUnknown, 0,
		core.NewAlternativeSetMember(13, 0), false, false, false)
	if err != nil || !adopted {
		t.Fatalf("adoption = %t, %v", adopted, err)
	}
	afterMembers, ok := compact.AlternativeSetMembers(scheduler.headers[0].altSet)
	if !ok || !reflect.DeepEqual(afterMembers, beforeMembers) || !reflect.DeepEqual(scheduler.headers[0], beforeHeader) {
		t.Fatalf("adoption mutated spilled source: members=%v, want %v", afterMembers, beforeMembers)
	}
	destinationMembers, ok := compact.AlternativeSetMembers(scheduler.headers[1].altSet)
	wantDestination := append(append([]uint32(nil), beforeMembers...), uint32(13)<<16)
	if !ok || !reflect.DeepEqual(destinationMembers, wantDestination) || scheduler.headers[1].altSet.Overflowed() {
		t.Fatalf("destination growth = %v/%t, want %v", destinationMembers, ok, wantDestination)
	}
}

func TestDiagnosticParserCoreSiblingAdoptionMismatchedVersionDoesNotTransferAlternatives(t *testing.T) {
	defer core.SetAlternativeSetRecordingEnabledForTest(true)()
	compact, head, _ := newDiagnosticParserCoreCanonicalTestCore(t)
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		headers: []diagnosticParserCoreHeader{
			{head: head, altSet: core.NewAlternativeSetMember(2, 1), blended: true,
				versionState: &diagnosticParserCoreVersionState{relexSnapshot: &diagnosticParserCoreVersionLexerSnapshot{}}},
			{head: head, altSet: core.NewAlternativeSetMember(1, 0), paused: true,
				versionState: &diagnosticParserCoreVersionState{relexSnapshot: &diagnosticParserCoreVersionLexerSnapshot{}}},
		},
	}
	before := append([]diagnosticParserCoreHeader(nil), scheduler.headers...)
	adopted, err := scheduler.adoptUpdatedReductionSibling(0, head, core.CleanPathRankUnknown, 0,
		core.NewAlternativeSetMember(3, 0), true, false, false)
	if err != nil || adopted {
		t.Fatalf("mismatched adoption = %t, %v", adopted, err)
	}
	if !reflect.DeepEqual(scheduler.headers, before) {
		t.Fatal("mismatched adoption transferred metadata or changed a header")
	}
}
