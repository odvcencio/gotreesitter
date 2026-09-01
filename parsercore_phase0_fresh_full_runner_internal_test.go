//go:build gts_parsercorephase0

package gotreesitter

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func parserCoreFreshFullCanonicalOptions() DiagnosticParserCorePrefixOptions {
	return DiagnosticParserCorePrefixOptions{
		ReceiptMode:   DiagnosticParserCoreReceiptSummary,
		MaxTokens:     300000,
		MaxDispatches: 600000,
		Limits:        diagnosticParserCoreCanonicalLimits(),
	}
}

func TestParserCoreFreshFullRunnerRepeatedCanonicalLifecycle(t *testing.T) {
	runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	for pass := 0; pass < 2; pass++ {
		for _, row := range diagnosticParserCoreCanonicalAdmissions {
			fixture := loadDiagnosticParserCoreCanonicalFixture(t, row.id)
			requireDiagnosticParserCoreCanonicalFixtureIdentity(t, fixture, row)
			tree, err := runner.parse(fixture.Source)
			if err != nil {
				t.Fatalf("pass %d fixture %s: %v", pass, row.id, err)
			}
			requireDiagnosticParserCoreCanonicalEOF(t, tree, len(fixture.Source))
			if got := requireDiagnosticParserCoreCanonicalTreeDigest(t, tree, runner.lang); got != row.deepTreeSHA256 {
				tree.Release()
				t.Fatalf("pass %d fixture %s deep-tree digest=%s want=%s", pass, row.id, got, row.deepTreeSHA256)
			}
			if work := runner.compact.Work(); work != row.work || work.Overflow {
				tree.Release()
				t.Fatalf("pass %d fixture %s compact work=%+v want=%+v", pass, row.id, work, row.work)
			}
			tree.Release()
		}
	}
}

func TestParserCoreFreshFullRunnerReusesSchedulerStorage(t *testing.T) {
	runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	fixture := loadDiagnosticParserCoreCanonicalFixture(t, "rewrite")
	first, tokenSource, err := runner.executeSchedulerOpen(fixture.Source, runner.compact, true)
	if err != nil {
		t.Fatal(err)
	}
	tokenSource.Close()
	if first != &runner.scheduler {
		t.Fatal("fresh-full runner did not use its scheduler storage")
	}
	second, tokenSource, err := runner.executeSchedulerOpen(fixture.Source, runner.compact, true)
	if err != nil {
		t.Fatal(err)
	}
	tokenSource.Close()
	if second != first {
		t.Fatal("fresh-full runner replaced its scheduler storage")
	}
}

func TestResetDiagnosticParserCoreRetainedSliceBoundsAndClears(t *testing.T) {
	value := 1
	small := make([]*int, 4)
	for index := range small {
		small[index] = &value
	}
	retained := resetDiagnosticParserCoreRetainedSlice(small)
	if len(retained) != 0 || cap(retained) != 4 {
		t.Fatalf("small retained slice len=%d cap=%d", len(retained), cap(retained))
	}
	for index, item := range retained[:cap(retained)] {
		if item != nil {
			t.Fatalf("small retained slice item %d was not cleared", index)
		}
	}

	large := make([]*int, diagnosticParserCoreRetainedScratchCapacity+1)
	for index := range large {
		large[index] = &value
	}
	if retained := resetDiagnosticParserCoreRetainedSlice(large); retained != nil {
		t.Fatalf("oversized retained slice cap=%d", cap(retained))
	}
}

func TestResetDiagnosticParserCoreGenericSchedulerRejectsActiveScratch(t *testing.T) {
	for _, test := range []struct {
		name      string
		scheduler diagnosticParserCoreGenericScheduler
	}{
		{name: "dispatch", scheduler: diagnosticParserCoreGenericScheduler{
			dispatchScratch: diagnosticParserCoreDispatchScratch{busy: true},
		}},
		{name: "conflict", scheduler: diagnosticParserCoreGenericScheduler{
			conflictScratch: diagnosticParserCoreConflictScratch{busy: true},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := resetDiagnosticParserCoreGenericScheduler(&test.scheduler); err == nil || !strings.Contains(err.Error(), "scratch is active") {
				t.Fatalf("active scratch error=%v", err)
			}
		})
	}
}

func TestResetDiagnosticParserCoreGenericSchedulerClearsRecoveryState(t *testing.T) {
	scheduler := diagnosticParserCoreGenericScheduler{
		s5MissingInsertions:      7,
		s3RegionOpened:           true,
		s3ResumeState:            11,
		s3ResumeSymbol:           12,
		s3ResumeCount:            1,
		recoveryIsolation:        true,
		acceptedRootFinalization: diagnosticParserCoreFinalizeRecoverEOF,
	}
	if err := resetDiagnosticParserCoreGenericScheduler(&scheduler); err != nil {
		t.Fatal(err)
	}
	if scheduler.s5MissingInsertions != 0 {
		t.Fatalf("missing insertion count=%d, want zero", scheduler.s5MissingInsertions)
	}
	if scheduler.s3RegionOpened {
		t.Fatal("the error region marker remained set after reset")
	}
	if scheduler.s3ResumeState != 0 || scheduler.s3ResumeSymbol != 0 || scheduler.s3ResumeCount != 0 {
		t.Fatalf("the recovery resume remained set after reset: %d/%d/%d",
			scheduler.s3ResumeState, scheduler.s3ResumeSymbol, scheduler.s3ResumeCount)
	}
	if scheduler.recoveryIsolation {
		t.Fatal("recovery isolation remained active after reset")
	}
	if scheduler.acceptedRootFinalization != diagnosticParserCoreFinalizeDefault {
		t.Fatalf("accepted root finalization=%v, want default after reset", scheduler.acceptedRootFinalization)
	}
}

func TestParserCoreFreshFullRunnerClearsRecoverEOFFinalizationBeforeOrdinaryAcceptance(t *testing.T) {
	runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
	if err != nil {
		t.Fatal(err)
	}

	runner.scheduler.acceptedRootFinalization = diagnosticParserCoreFinalizeRecoverEOF
	ordinary, err := runner.parse([]byte("package p\n"))
	if err != nil {
		t.Fatalf("ordinary parse after stale recover-eof finalization: %v", err)
	}
	if ordinary == nil || ordinary.RootNode() == nil {
		if ordinary != nil {
			ordinary.Release()
		}
		t.Fatal("ordinary parse returned no root")
	}
	defer ordinary.Release()
	if ordinary.RootNode().IsError() || ordinary.RootNode().HasError() {
		t.Fatalf("ordinary parse retained stale error state: type=%s", ordinary.RootNode().Type(runner.lang))
	}
	if runner.scheduler.acceptedRootFinalization != diagnosticParserCoreFinalizeDefault {
		t.Fatalf("ordinary finalization=%v, want default", runner.scheduler.acceptedRootFinalization)
	}
}

func TestParserCoreFreshFullRunnerScratchDoesNotAliasLiveTree(t *testing.T) {
	runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	firstRow := diagnosticParserCoreCanonicalAdmissions[0]
	firstFixture := loadDiagnosticParserCoreCanonicalFixture(t, firstRow.id)
	firstTree, err := runner.parse(firstFixture.Source)
	if err != nil {
		t.Fatal(err)
	}
	defer firstTree.Release()
	firstDigest := requireDiagnosticParserCoreCanonicalTreeDigest(t, firstTree, runner.lang)

	secondRow := diagnosticParserCoreCanonicalAdmissions[1]
	secondFixture := loadDiagnosticParserCoreCanonicalFixture(t, secondRow.id)
	secondTree, err := runner.parse(secondFixture.Source)
	if err != nil {
		t.Fatal(err)
	}
	defer secondTree.Release()
	if got := requireDiagnosticParserCoreCanonicalTreeDigest(t, secondTree, runner.lang); got != secondRow.deepTreeSHA256 {
		t.Fatalf("second tree digest=%s want=%s", got, secondRow.deepTreeSHA256)
	}
	if got := requireDiagnosticParserCoreCanonicalTreeDigest(t, firstTree, runner.lang); got != firstDigest {
		t.Fatalf("first live tree changed after scratch reuse: digest=%s want=%s", got, firstDigest)
	}
}

func TestParserCoreFreshFullRunnerResetsAfterCap(t *testing.T) {
	options := parserCoreFreshFullCanonicalOptions()
	runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, options)
	if err != nil {
		t.Fatal(err)
	}
	fixture := loadDiagnosticParserCoreCanonicalFixture(t, "rewrite")

	runner.options.MaxDispatches = 1
	if tree, err := runner.parse(fixture.Source); err == nil || tree != nil || !strings.Contains(err.Error(), "dispatch cap") {
		if tree != nil {
			tree.Release()
		}
		t.Fatalf("capped parse tree=%v err=%v, want fail-closed dispatch cap", tree != nil, err)
	}

	runner.options.MaxDispatches = options.MaxDispatches
	tree, err := runner.parse(fixture.Source)
	if err != nil {
		t.Fatalf("parse after cap did not reset reusable state: %v", err)
	}
	defer tree.Release()
	requireDiagnosticParserCoreCanonicalEOF(t, tree, len(fixture.Source))
	if work := runner.compact.Work(); work != diagnosticParserCoreCanonicalAdmissions[0].work || work.Overflow {
		t.Fatalf("parse after cap work=%+v want=%+v", work, diagnosticParserCoreCanonicalAdmissions[0].work)
	}
}

func TestParserCoreFreshFullPublicRouteIsSelectedStoreFree(t *testing.T) {
	options := parserCoreFreshFullCanonicalOptions()
	options.Limits.MaxSelectedOccurrences = 1
	options.Limits.MaxSelectedBytes = 1
	runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, options)
	if err != nil {
		t.Fatal(err)
	}
	fixture := loadDiagnosticParserCoreCanonicalFixture(t, "rewrite")
	tree, err := runner.parse(fixture.Source)
	if err != nil {
		t.Fatalf("public compatibility route paid selected-store cap: %v", err)
	}
	tree.Release()
	if store, err := runner.parseSelectedStore(fixture.Source); err == nil || store != nil || !strings.Contains(err.Error(), "occurrence cap") {
		t.Fatalf("direct selected-store cap store=%v err=%v", store, err)
	}
}

func TestParserCoreFreshFullSelectedStorePollsCancellation(t *testing.T) {
	runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	var cancelled uint32
	runner.parser.SetCancellationFlag(&cancelled)
	fixture := loadDiagnosticParserCoreCanonicalFixture(t, "rewrite")
	atomic.StoreUint32(&cancelled, 1)
	store, err := runner.parseSelectedStore(fixture.Source)
	if err == nil || store != nil || !strings.Contains(err.Error(), "selected-store sealing stopped: cancelled") {
		t.Fatalf("cancelled direct store=%v err=%v", store, err)
	}
	atomic.StoreUint32(&cancelled, 0)
	store, err = runner.parseSelectedStore(fixture.Source)
	if err != nil || store == nil {
		t.Fatalf("selected store after cancellation reset=%v err=%v", store, err)
	}
}

func TestParserCoreFreshFullRunnerFailsClosedAtConstruction(t *testing.T) {
	base := parserCoreFreshFullCanonicalOptions()
	if _, err := newParserCoreFreshFullRunner(nil, base); err == nil || !strings.Contains(err.Error(), "external scanner identity mismatch") {
		t.Fatalf("unauthenticated scanner error=%v", err)
	}

	tests := []struct {
		name   string
		mutate func(*DiagnosticParserCorePrefixOptions)
	}{
		{name: "recovery", mutate: func(o *DiagnosticParserCorePrefixOptions) { o.Recovery = true }},
		{name: "retry", mutate: func(o *DiagnosticParserCorePrefixOptions) { o.Retry = true }},
		{name: "incremental", mutate: func(o *DiagnosticParserCorePrefixOptions) { o.Incremental = true }},
		{name: "included-ranges", mutate: func(o *DiagnosticParserCorePrefixOptions) { o.IncludedRanges = true }},
		{name: "full-receipts", mutate: func(o *DiagnosticParserCorePrefixOptions) { o.ReceiptMode = DiagnosticParserCoreReceiptFull }},
		{name: "closed-prefix", mutate: func(o *DiagnosticParserCorePrefixOptions) {
			closed := uint32(1)
			o.GenericStopAtClosedByte = &closed
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := base
			test.mutate(&options)
			if runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, options); err == nil || runner != nil {
				t.Fatalf("unsupported route runner=%v err=%v", runner != nil, err)
			}
		})
	}
}

func TestParserCoreFreshFullAcceptedTailRequiresParserPadding(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		headByte uint32
		want     bool
	}{
		{name: "exact EOF", source: "value", headByte: 5, want: true},
		{name: "newline", source: "value\n", headByte: 5, want: true},
		{name: "mixed padding", source: "value \t\r\n", headByte: 5, want: true},
		{name: "real tail", source: "value x", headByte: 5},
		{name: "past EOF", source: "value", headByte: 6},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := parserCoreFreshFullAcceptedTailIsClean([]byte(test.source), test.headByte, 0); got != test.want {
				t.Fatalf("clean accepted tail=%t, want %t", got, test.want)
			}
		})
	}
}

// BenchmarkParserCoreFreshFullCanonical measures the authenticated compact
// candidate's complete fresh materialized lifecycle. Fixture identity, deep
// tree equality, exact compact work, arena draining, runner construction, and
// one warm parse all remain outside the timed region. The timed region is only
// runner.parse, shallow completeness validation, and Tree.Release.
func BenchmarkParserCoreFreshFullCanonical(b *testing.B) {
	for _, row := range diagnosticParserCoreCanonicalAdmissions {
		row := row
		b.Run(row.id, func(b *testing.B) {
			fixture := loadDiagnosticParserCoreCanonicalFixture(b, row.id)
			requireDiagnosticParserCoreCanonicalFixtureIdentity(b, fixture, row)
			preflightParserCoreFreshFullCanonical(b, fixture, row)

			// Collect the now-unreachable compact preflight arenas before
			// constructing the measured runner so max RSS has no overlap.
			runtime.GC()
			DrainArenaPools()
			b.Cleanup(DrainArenaPools)
			runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
			if err != nil {
				b.Fatal(err)
			}
			warmTree, err := runner.parse(fixture.Source)
			if err != nil {
				if warmTree != nil {
					warmTree.Release()
				}
				b.Fatalf("%s candidate warm parse: %v", row.id, err)
			}
			if err := validateParserCoreFreshFullCanonicalTree(warmTree, len(fixture.Source)); err != nil {
				if warmTree != nil {
					warmTree.Release()
				}
				b.Fatalf("%s candidate warm parse: %v", row.id, err)
			}
			if work := runner.compact.Work(); work != row.work || work.Overflow {
				warmTree.Release()
				b.Fatalf("%s candidate warm work=%+v want=%+v", row.id, work, row.work)
			}
			warmTree.Release()

			b.ReportAllocs()
			b.SetBytes(int64(len(fixture.Source)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tree, err := runner.parse(fixture.Source)
				if err != nil {
					if tree != nil {
						tree.Release()
					}
					b.Fatalf("%s candidate timed parse: %v", row.id, err)
				}
				if err := validateParserCoreFreshFullCanonicalTree(tree, len(fixture.Source)); err != nil {
					if tree != nil {
						tree.Release()
					}
					b.Fatalf("%s candidate timed parse: %v", row.id, err)
				}
				tree.Release()
			}
			b.StopTimer()

			work := runner.compact.Work()
			if work != row.work || work.Overflow {
				b.Fatalf("%s candidate timed work=%+v want=%+v", row.id, work, row.work)
			}
			reportParserCoreFreshFullWork(b, work)
		})
	}
}

// BenchmarkParserCoreFreshFullSelectedStoreCanonical measures the same
// authenticated scheduler lifecycle but consumes the sealed store directly.
// The compatibility benchmark above remains the public-Node fallback/control.
func BenchmarkParserCoreFreshFullSelectedStoreCanonical(b *testing.B) {
	for _, row := range diagnosticParserCoreCanonicalAdmissions {
		row := row
		b.Run(row.id, func(b *testing.B) {
			fixture := loadDiagnosticParserCoreCanonicalFixture(b, row.id)
			runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
			if err != nil {
				b.Fatal(err)
			}
			store, err := runner.parseSelectedStore(fixture.Source)
			if err != nil {
				b.Fatal(err)
			}
			if got := store.NodeCount(); got != row.selectedNodes {
				b.Fatalf("selected nodes=%d want=%d", got, row.selectedNodes)
			}
			if got := diagnosticParserCoreSelectedStoreDeepDigest(b, store, runner.lang, fixture.Source); got != row.deepTreeSHA256 {
				b.Fatalf("selected digest=%s want=%s", got, row.deepTreeSHA256)
			}
			if checksum := diagnosticParserCoreTraverseSelectedStore(store); checksum == 0 {
				b.Fatal("selected traversal preflight failed")
			}
			retainedBytes := store.RetainedBytes()
			store.Release()
			runtime.GC()
			b.ReportAllocs()
			b.SetBytes(int64(len(fixture.Source)))
			b.ResetTimer()
			var checksum uint64
			for index := 0; index < b.N; index++ {
				store, err = runner.parseSelectedStore(fixture.Source)
				if err != nil {
					b.Fatal(err)
				}
				checksum ^= diagnosticParserCoreTraverseSelectedStore(store)
				store.Release()
			}
			b.StopTimer()
			if checksum == ^uint64(0) {
				b.Fatal("unreachable selected traversal checksum")
			}
			if work := runner.compact.Work(); work != row.work || work.Overflow {
				b.Fatalf("selected work=%+v want=%+v", work, row.work)
			} else {
				reportParserCoreFreshFullWork(b, work)
			}
			b.ReportMetric(float64(retainedBytes), "selected_retained_B/op")
		})
	}
}

func diagnosticParserCoreTraverseSelectedStore(store *core.SelectedStore) uint64 {
	if store == nil || store.Root() == 0 {
		return 0
	}
	cursor := store.Cursor()
	var checksum uint64
	for {
		record, ok := cursor.Record()
		if !ok {
			return 0
		}
		checksum += uint64(cursor.ID()) + uint64(record.Symbol) + uint64(record.StartByte) + uint64(record.EndByte) + uint64(record.Field)
		if child, ok := cursor.Child(0); ok {
			cursor = child
			continue
		}
		for {
			if sibling, ok := cursor.NextSibling(); ok {
				cursor = sibling
				break
			}
			parent, ok := cursor.Parent()
			if !ok {
				return checksum
			}
			cursor = parent
		}
	}
}

// Keep the deep admission in a separate frame. Returning drops the only
// reference to its compact arenas before the measured runner is constructed,
// so DrainArenaPools cannot overlap the preflight and measured lifecycles.
func preflightParserCoreFreshFullCanonical(
	b *testing.B,
	fixture DiagnosticParserCoreCanonicalFixtureForTest,
	row diagnosticParserCoreCanonicalAdmission,
) {
	b.Helper()
	runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
	if err != nil {
		b.Fatal(err)
	}
	tree, err := runner.parse(fixture.Source)
	if err != nil {
		if tree != nil {
			tree.Release()
		}
		b.Fatalf("%s candidate preflight: %v", row.id, err)
	}
	if err := validateParserCoreFreshFullCanonicalTree(tree, len(fixture.Source)); err != nil {
		b.Fatalf("%s candidate preflight: %v", row.id, err)
	}
	defer tree.Release()
	if got := requireDiagnosticParserCoreCanonicalTreeDigest(b, tree, runner.lang); got != row.deepTreeSHA256 {
		b.Fatalf("%s candidate preflight digest=%s want=%s", row.id, got, row.deepTreeSHA256)
	}
	if work := runner.compact.Work(); work != row.work || work.Overflow {
		b.Fatalf("%s candidate preflight work=%+v want=%+v", row.id, work, row.work)
	}
}

func validateParserCoreFreshFullCanonicalTree(tree *Tree, sourceLen int) error {
	if tree == nil || tree.root == nil {
		return errors.New("candidate parse returned no tree or root")
	}
	root := tree.root
	runtime := tree.ParseRuntime()
	if root.startByte != 0 || root.endByte != uint32(sourceLen) || root.IsError() || root.HasError() ||
		runtime.StopReason != ParseStopAccepted || runtime.Truncated || runtime.TokenSourceEOFEarly ||
		runtime.SourceLen != uint32(sourceLen) || runtime.ExpectedEOFByte != uint32(sourceLen) ||
		runtime.RootEndByte != uint32(sourceLen) || !runtime.LastTokenWasEOF {
		return fmt.Errorf("candidate tree is incomplete: root=%d..%d error=%t runtime=%s",
			root.startByte, root.endByte, root.HasError(), runtime.Summary())
	}
	return nil
}

func reportParserCoreFreshFullWork(b *testing.B, work core.Work) {
	b.Helper()
	b.ReportMetric(float64(work.Shifts), "compact_shifts/op")
	b.ReportMetric(float64(work.Reductions), "compact_reductions/op")
	b.ReportMetric(float64(work.ReductionPopRequests), "compact_pop_requests/op")
	b.ReportMetric(float64(work.EmittedPopPaths), "compact_pop_paths/op")
	b.ReportMetric(float64(work.EmittedPopPayloads), "compact_pop_payloads/op")
	b.ReportMetric(float64(work.GraphLinkAdditionsProxy), "compact_link_additions/op")
	b.ReportMetric(float64(work.LeafConstructionsProxy), "compact_leaf_constructions/op")
	b.ReportMetric(float64(work.ParentConstructionsProxy), "compact_parent_constructions/op")
	b.ReportMetric(0, "candidate_fallback/op")
}
