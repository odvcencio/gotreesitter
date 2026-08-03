//go:build gts_parsercorephase0

package gotreesitter

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// DiagnosticParserCoreCanonicalFixtureForTest is the import-cycle-free value
// supplied by the external tagged test package. Keeping the loader here avoids
// embedding a second copy of the canonical sources in the root test binary.
type DiagnosticParserCoreCanonicalFixtureForTest struct {
	ID                string
	Source            []byte
	SourceSHA256      string
	DeepTreeSHA256    string
	UncompressedBytes int
}

var diagnosticParserCoreCanonicalFixtureLoaderForTest func(string) (DiagnosticParserCoreCanonicalFixtureForTest, error)
var diagnosticParserCoreCanonicalTreeDigestForTest func(*Tree, *Language) (string, error)

// SetDiagnosticParserCoreCanonicalFixtureLoaderForTest installs the lazy
// canonical-fixture bridge from package gotreesitter_test. It is compiled only
// in the diagnostic parser-core test lane.
func SetDiagnosticParserCoreCanonicalFixtureLoaderForTest(loader func(string) (DiagnosticParserCoreCanonicalFixtureForTest, error)) {
	diagnosticParserCoreCanonicalFixtureLoaderForTest = loader
}

// SetDiagnosticParserCoreCanonicalTreeDigestForTest installs the canonical
// gts-deep-tree-v1 implementation from the import-cycle-safe external package.
func SetDiagnosticParserCoreCanonicalTreeDigestForTest(digest func(*Tree, *Language) (string, error)) {
	diagnosticParserCoreCanonicalTreeDigestForTest = digest
}

type diagnosticParserCoreCanonicalAdmission struct {
	id              string
	bytes           int
	sourceSHA256    string
	deepTreeSHA256  string
	selectedNodes   uint64
	selectedParents uint64
	selectedLeaves  uint64
	conflictArms    uint64
	causalForks     uint64
	rawSelected     core.RawSelectedCensus
	work            core.Work
}

var diagnosticParserCoreCanonicalAdmissions = []diagnosticParserCoreCanonicalAdmission{
	{
		id: "rewrite", bytes: 5116,
		sourceSHA256:   "74c0705f8729670559492fb5460a01b2a1a2a109928e1aeb52736e485e8ff097",
		deepTreeSHA256: "b3f9814b65763642d4eac58b9065018048ea13e6f10d56afb28a0479bf5a68a1",
		selectedNodes:  1524, selectedParents: 572, selectedLeaves: 952,
		conflictArms: 328, causalForks: 168,
		rawSelected: core.RawSelectedCensus{Nodes: 2237, Parents: 1202, Leaves: 1035},
		work: core.Work{
			Shifts: 1348, Reductions: 1504, ReductionPopRequests: 1504,
			EmittedPopPaths: 1646, EmittedPopPayloads: 2993,
			PredecessorLinkUnionAttempts: 174, PredecessorLinkUnionDuplicateNoop: 4,
			PredecessorLinkUnionPrecedenceReplaced: 25, PredecessorLinkUnionAlternateAppended: 145,
			GraphLinkAdditionsProxy: 3006, LeafConstructionsProxy: 1109,
			ParentConstructionsProxy: 1515,
		},
	},
	{
		id: "query_compile", bytes: 20168,
		sourceSHA256:   "b788ee19b0075f0b9b567a9f93ea657e715bc8a6a40a99d3ca5c761404e71894",
		deepTreeSHA256: "ecc090a83a4343a1c7c2afbad63277f5b4d60c42d8d94a2af2a9b16e46f2ccb5",
		selectedNodes:  7524, selectedParents: 2853, selectedLeaves: 4671,
		conflictArms: 1365, causalForks: 685,
		rawSelected: core.RawSelectedCensus{Nodes: 11331, Parents: 6206, Leaves: 5125},
		work: core.Work{
			Shifts: 6685, Reductions: 7509, ReductionPopRequests: 7509,
			EmittedPopPaths: 8108, EmittedPopPayloads: 14730,
			PredecessorLinkUnionAttempts: 722, PredecessorLinkUnionDuplicateNoop: 36,
			PredecessorLinkUnionPrecedenceReplaced: 75, PredecessorLinkUnionAlternateAppended: 611,
			GraphLinkAdditionsProxy: 14789, LeafConstructionsProxy: 5546,
			ParentConstructionsProxy: 7542,
		},
	},
	{
		id: "language", bytes: 41387,
		sourceSHA256:   "009aa9fd5352c712f3839670c7df8a9b00ae878ee20dc88131a438b2d5edfd9a",
		deepTreeSHA256: "583df223904fe414c33bba3b474c6557ecdb20e7f47e304b9a09bfcc2da44539",
		selectedNodes:  7082, selectedParents: 2631, selectedLeaves: 4451,
		conflictArms: 1216, causalForks: 618,
		rawSelected: core.RawSelectedCensus{Nodes: 10761, Parents: 5704, Leaves: 5057},
		work: core.Work{
			Shifts: 6512, Reductions: 7564, ReductionPopRequests: 7564,
			EmittedPopPaths: 8377, EmittedPopPayloads: 15816,
			PredecessorLinkUnionAttempts: 1036, PredecessorLinkUnionDuplicateNoop: 18,
			PredecessorLinkUnionPrecedenceReplaced: 207, PredecessorLinkUnionRecursiveChanged: 1,
			PredecessorLinkUnionAlternateAppended: 810,
			GraphLinkAdditionsProxy:               15124, LeafConstructionsProxy: 5375,
			ParentConstructionsProxy: 7674,
		},
	},
	{
		id: "grammargen_lr", bytes: 235626,
		sourceSHA256:   "a7e4a1a64b25a60aea36183b9d6d53dcd9240942cdb10e67a3cf9e6ce30f95b2",
		deepTreeSHA256: "1472cfd9a014d4034dbc1456afd12c282630ef787c3543cf0cecb73619883ad2",
		selectedNodes:  71768, selectedParents: 26371, selectedLeaves: 45397,
		conflictArms: 16043, causalForks: 8155,
		rawSelected: core.RawSelectedCensus{Nodes: 109614, Parents: 59703, Leaves: 49911},
		work: core.Work{
			Shifts: 66115, Reductions: 76310, ReductionPopRequests: 76310,
			EmittedPopPaths: 83658, EmittedPopPayloads: 151917,
			PredecessorLinkUnionAttempts: 9137, PredecessorLinkUnionDuplicateNoop: 325,
			PredecessorLinkUnionPrecedenceReplaced: 1376, PredecessorLinkUnionRecursiveChanged: 1,
			PredecessorLinkUnionAlternateAppended: 7435,
			GraphLinkAdditionsProxy:               149768, LeafConstructionsProxy: 53896,
			ParentConstructionsProxy: 77168,
		},
	},
}

func TestDiagnosticParserCoreCanonicalAdmissions(t *testing.T) {
	for _, row := range diagnosticParserCoreCanonicalAdmissions {
		row := row
		t.Run(row.id, func(t *testing.T) {
			fixture := loadDiagnosticParserCoreCanonicalFixture(t, row.id)
			requireDiagnosticParserCoreCanonicalFixtureIdentity(t, fixture, row)

			result, routeErr := DiagnosticParseParserCorePrefix(
				parserCoreWarmGoScanner, fixture.Source,
				DiagnosticParserCorePrefixOptions{
					ReceiptMode: DiagnosticParserCoreReceiptSummary,
					MaxTokens:   300000, MaxDispatches: 600000,
					Limits: diagnosticParserCoreCanonicalLimits(),
				},
			)
			if routeErr != nil {
				t.Fatalf("canonical compact admission declined: boundary=%s detail=%q err=%v", result.Boundary, result.Detail, routeErr)
			}
			if result.MaterializedTree != nil {
				t.Cleanup(result.MaterializedTree.Release)
			}
			if !result.Completed || !result.Materialized || result.MaterializedTree == nil || result.GenericScheduler == nil || result.GenericScheduler.Acceptance == nil {
				t.Fatalf("canonical compact admission did not publish an exact tree: %+v", result)
			}
			acceptance := result.GenericScheduler.Acceptance
			if result.Boundary != DiagnosticParserCoreGenericClosed || result.State != 2 || result.Lookahead.Symbol != 0 || result.Lookahead.Text != "" || result.Lookahead.StartByte != uint32(len(fixture.Source)) || result.Lookahead.EndByte != uint32(len(fixture.Source)) || result.Lookahead.Missing || result.Lookahead.NoLookahead || result.Lookahead.ExternalScannerToken || acceptance.Header.Header.State != 2 || acceptance.Header.Header.ByteOffset != uint32(len(fixture.Source)) || !acceptance.Header.Header.Accepted || acceptance.Header.Header.Paused || acceptance.Header.Header.ExactPaths != 1 || acceptance.Accepts != 1 || acceptance.Work.Accepts != 1 {
				t.Fatalf("canonical compact EOF acceptance drifted: result=%+v acceptance=%+v", result, acceptance)
			}
			if acceptance.CoreWork != row.work || acceptance.CoreWork.Overflow {
				t.Fatalf("canonical compact work drifted: got=%+v want=%+v", acceptance.CoreWork, row.work)
			}
			if acceptance.Work.Overflow || acceptance.Work.ConflictActionArmsAdmitted != row.conflictArms || acceptance.Work.CausalConflictForks != row.causalForks {
				t.Fatalf("canonical compact causal fanout drifted: got=%+v want=%d/%d", acceptance.Work, row.conflictArms, row.causalForks)
			}
			if acceptance.SelectedNodes != row.selectedNodes || acceptance.SelectedParents != row.selectedParents || acceptance.SelectedLeaves != row.selectedLeaves || acceptance.SelectedParents+acceptance.SelectedLeaves != acceptance.SelectedNodes {
				t.Fatalf("canonical selected census drifted: got=%d/%d/%d want=%d/%d/%d", acceptance.SelectedNodes, acceptance.SelectedParents, acceptance.SelectedLeaves, row.selectedNodes, row.selectedParents, row.selectedLeaves)
			}
			requireDiagnosticParserCoreCanonicalEOF(t, result.MaterializedTree, len(fixture.Source))

			lang, err := authenticatedParserCoreGoLanguage(parserCoreWarmGoScanner)
			if err != nil {
				t.Fatal(err)
			}
			production, err := NewParser(lang).Parse(fixture.Source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			requireDiagnosticParserCoreCanonicalEOF(t, production, len(fixture.Source))
			if got := requireDiagnosticParserCoreCanonicalTreeDigest(t, result.MaterializedTree, lang); got != fixture.DeepTreeSHA256 {
				t.Fatalf("compact canonical deep-tree digest=%s want=%s", got, fixture.DeepTreeSHA256)
			}
			if got := requireDiagnosticParserCoreCanonicalTreeDigest(t, production, lang); got != fixture.DeepTreeSHA256 {
				t.Fatalf("production canonical deep-tree digest=%s want=%s", got, fixture.DeepTreeSHA256)
			}
			parserCoreWarmRequireDeepEqual(t, result.MaterializedTree, production, lang)
		})
	}
}

func diagnosticParserCorePointCacheCensus(t testing.TB, compact *core.Core, head core.Head, source []byte) (views, hits, misses int) {
	t.Helper()
	derivations, err := compactDerivationsForAcceptance(compact, head)
	if err != nil {
		t.Fatal(err)
	}
	if len(derivations) != 1 {
		t.Fatalf("compact derivations=%d, want one", len(derivations))
	}
	index, err := newDiagnosticParserCorePointIndex(source, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	record := func(offset uint32) {
		if _, hit := index.pointCached(offset); hit {
			hits++
		} else {
			misses++
		}
	}
	err = compact.VisitMaterializationPostorder(derivations[0].Payloads, func() error { return nil }, func(_ core.SubtreeID, view core.MaterializationSubtreeView) error {
		views++
		record(view.StartByte)
		record(view.EndByte)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if hits+misses != 2*views {
		t.Fatalf("point-cache census views=%d hits=%d misses=%d", views, hits, misses)
	}
	return views, hits, misses
}

func TestDiagnosticParserCoreBoundaryIndexCensus(t *testing.T) {
	for _, row := range diagnosticParserCoreCanonicalAdmissions {
		row := row
		t.Run(row.id, func(t *testing.T) {
			fixture := loadDiagnosticParserCoreCanonicalFixture(t, row.id)
			requireDiagnosticParserCoreCanonicalFixtureIdentity(t, fixture, row)
			lang, err := authenticatedParserCoreGoLanguage(parserCoreWarmGoScanner)
			if err != nil {
				t.Fatal(err)
			}
			parser := NewParser(lang)
			tables, err := newParserCoreRootTables(parser)
			if err != nil {
				t.Fatal(err)
			}
			compact, err := core.New(tables, diagnosticParserCoreCanonicalLimits())
			if err != nil {
				t.Fatal(err)
			}
			tokenSource := parser.acquireParserDFATokenSource(fixture.Source)
			if tokenSource == nil {
				t.Fatal("canonical boundary census could not acquire DFA token source")
			}
			defer tokenSource.Close()
			var elections, peakCurrent, peakRetained uint64
			maxCheckpointBytes := 0
			observer := diagnosticParserCoreSeedObserver{beforeElection: func(s *diagnosticParserCoreGenericScheduler) error {
				stats := s.compact.BoundaryIndexStats()
				elections++
				if stats.CurrentEntries > peakCurrent {
					peakCurrent = stats.CurrentEntries
				}
				if stats.RetainedEntries > peakRetained {
					peakRetained = stats.RetainedEntries
				}
				if s.checkpoint.Length > maxCheckpointBytes {
					maxCheckpointBytes = s.checkpoint.Length
				}
				return nil
			}}
			var scannerScratch []byte
			scheduler, err := executeDiagnosticParserCoreGenericSchedulerFromSeed(
				compact, tokenSource, &scannerScratch, lang.InitialState,
				diagnosticParserCoreCanonicalSeedOptions(lang),
				observer,
			)
			if err != nil {
				t.Fatal(err)
			}
			if scheduler == nil || scheduler.acceptedHead.Node == 0 || compact.Work() != row.work {
				t.Fatalf("canonical boundary census did not preserve acceptance: scheduler=%v work=%+v", scheduler != nil, compact.Work())
			}
			views, pointHits, pointMisses := diagnosticParserCorePointCacheCensus(t, compact, scheduler.acceptedHead, fixture.Source)
			if views != int(row.rawSelected.Nodes) || pointHits <= pointMisses {
				t.Fatalf("canonical point-cache census views=%d hits=%d misses=%d", views, pointHits, pointMisses)
			}
			final := compact.BoundaryIndexStats()
			t.Logf("boundary-index fixture=%s elections=%d peak_current=%d peak_retained=%d final_current=%d final_retained=%d max_checkpoint_bytes=%d point_hits=%d point_misses=%d point_hit_rate=%.2f%%",
				row.id, elections, peakCurrent, peakRetained, final.CurrentEntries, final.RetainedEntries, maxCheckpointBytes, pointHits, pointMisses, 100*float64(pointHits)/float64(pointHits+pointMisses))
		})
	}
}

func BenchmarkDiagnosticParserCoreCanonicalSchedulerCold(b *testing.B) {
	for _, row := range diagnosticParserCoreCanonicalAdmissions {
		row := row
		b.Run(row.id, func(b *testing.B) {
			fixture := loadDiagnosticParserCoreCanonicalFixture(b, row.id)
			requireDiagnosticParserCoreCanonicalFixtureIdentity(b, fixture, row)
			lang, err := authenticatedParserCoreGoLanguage(parserCoreWarmGoScanner)
			if err != nil {
				b.Fatal(err)
			}
			parser := NewParser(lang)
			tables, err := newParserCoreRootTables(parser)
			if err != nil {
				b.Fatal(err)
			}
			options := diagnosticParserCoreCanonicalSeedOptions(lang)
			var scannerScratch []byte
			b.ReportAllocs()
			b.SetBytes(int64(len(fixture.Source)))
			b.ResetTimer()
			for range b.N {
				compact, err := core.New(tables, diagnosticParserCoreCanonicalLimits())
				if err != nil {
					b.Fatal(err)
				}
				tokenSource := parser.acquireParserDFATokenSource(fixture.Source)
				if tokenSource == nil {
					b.Fatal("canonical cold scheduler could not acquire DFA token source")
				}
				scheduler, runErr := executeDiagnosticParserCoreGenericSchedulerFromSeed(
					compact, tokenSource, &scannerScratch, lang.InitialState, options,
					diagnosticParserCoreSeedObserver{},
				)
				tokenSource.Close()
				if runErr != nil {
					b.Fatal(runErr)
				}
				if scheduler == nil || scheduler.acceptedHead.Node == 0 || compact.Work() != row.work {
					b.Fatalf("canonical cold scheduler acceptance drifted: scheduler=%v work=%+v", scheduler != nil, compact.Work())
				}
			}
		})
	}
}

func BenchmarkDiagnosticParserCoreCanonicalTotal(b *testing.B) {
	for _, row := range diagnosticParserCoreCanonicalAdmissions {
		row := row
		b.Run(row.id, func(b *testing.B) {
			fixture := loadDiagnosticParserCoreCanonicalFixture(b, row.id)
			requireDiagnosticParserCoreCanonicalFixtureIdentity(b, fixture, row)
			runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, DiagnosticParserCorePrefixOptions{
				ReceiptMode: DiagnosticParserCoreReceiptSummary,
				MaxTokens:   300000, MaxDispatches: 600000,
				Limits: diagnosticParserCoreCanonicalLimits(),
			})
			if err != nil {
				b.Fatal(err)
			}
			tree, err := runner.parse(fixture.Source)
			if err != nil {
				b.Fatal(err)
			}
			requireDiagnosticParserCoreCanonicalEOF(b, tree, len(fixture.Source))
			if got := diagnosticParserCoreSelectedNodeCensus(tree.root); got.total != row.selectedNodes || got.parents != row.selectedParents || got.leaves != row.selectedLeaves {
				b.Fatalf("canonical total selected census=%+v want=%d/%d/%d", got, row.selectedNodes, row.selectedParents, row.selectedLeaves)
			}
			tree.Release()
			b.ReportAllocs()
			b.SetBytes(int64(len(fixture.Source)))
			b.ResetTimer()
			for range b.N {
				tree, err := runner.parse(fixture.Source)
				if err != nil {
					b.Fatal(err)
				}
				tree.Release()
			}
		})
	}
}

func diagnosticParserCoreCanonicalLimits() core.Limits {
	return core.Limits{
		MaxNodes: 1 << 20, MaxLinks: 1 << 20, MaxSubtrees: 1 << 20,
		MaxChildren: 4 << 20, MaxMetadata: 2 << 20,
		MaxLinksPerBoundary: 8, MaxPopPaths: 1 << 16, MaxDerivations: 1 << 16,
	}
}

// diagnosticParserCoreCanonicalSeedOptions builds the prefix options for the
// canonical instruments that seed the generic scheduler themselves rather
// than call DiagnosticParseParserCorePrefix.
//
// allowConvergedSplitDropArtifact is the load-bearing field. The driver sets
// it from the language's own certification record
// (lang.CompactConvergedReductionSplitDropsCertified,
// parsercore_phase0_driver.go), so every route that goes through
// DiagnosticParseParserCorePrefix or the admission-candidate runner carries
// it. An instrument that builds its own option literal and seeds
// executeDiagnosticParserCoreGenericSchedulerFromSeed directly bypasses that
// seam. Without this field the certified Go grammar's converged-path
// reduction split no-action drop fails the alternative-set coverage proof
// (dropGenericNoActionHeads) and every canonical fixture declines, so the
// instrument measures nothing at all.
func diagnosticParserCoreCanonicalSeedOptions(lang *Language) DiagnosticParserCorePrefixOptions {
	options := DiagnosticParserCorePrefixOptions{
		ReceiptMode: DiagnosticParserCoreReceiptSummary,
		MaxTokens:   300000, MaxDispatches: 600000,
		Limits: diagnosticParserCoreCanonicalLimits(),
	}
	if lang != nil {
		options.allowConvergedSplitDropArtifact = lang.CompactConvergedReductionSplitDropsCertified
	}
	return options
}

func loadDiagnosticParserCoreCanonicalFixture(t testing.TB, id string) DiagnosticParserCoreCanonicalFixtureForTest {
	t.Helper()
	if diagnosticParserCoreCanonicalFixtureLoaderForTest == nil {
		t.Fatal("canonical fixture loader was not installed")
	}
	fixture, err := diagnosticParserCoreCanonicalFixtureLoaderForTest(id)
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

func requireDiagnosticParserCoreCanonicalFixtureIdentity(t testing.TB, fixture DiagnosticParserCoreCanonicalFixtureForTest, row diagnosticParserCoreCanonicalAdmission) {
	t.Helper()
	if fixture.ID != row.id || fixture.UncompressedBytes != row.bytes || len(fixture.Source) != row.bytes || fixture.SourceSHA256 != row.sourceSHA256 || fixture.DeepTreeSHA256 != row.deepTreeSHA256 || fmt.Sprintf("%x", sha256.Sum256(fixture.Source)) != row.sourceSHA256 {
		t.Fatalf("canonical fixture identity drifted: got=%+v bytes=%d sha256=%x want=%+v", fixture, len(fixture.Source), sha256.Sum256(fixture.Source), row)
	}
}

func requireDiagnosticParserCoreCanonicalEOF(t testing.TB, tree *Tree, sourceLen int) {
	t.Helper()
	if tree == nil || tree.root == nil {
		t.Fatal("canonical compact route returned no tree")
	}
	root := tree.root
	runtime := tree.ParseRuntime()
	if root.startByte != 0 || root.endByte != uint32(sourceLen) || root.IsError() || root.HasError() || runtime.StopReason != ParseStopAccepted || runtime.Truncated || runtime.TokenSourceEOFEarly || runtime.SourceLen != uint32(sourceLen) || runtime.ExpectedEOFByte != uint32(sourceLen) || runtime.RootEndByte != uint32(sourceLen) || !runtime.LastTokenWasEOF || strings.TrimSpace(runtime.Summary()) == "" {
		t.Fatalf("canonical compact EOF exactness drifted: root=%d..%d error=%v runtime=%s", root.startByte, root.endByte, root.HasError(), runtime.Summary())
	}
}

func requireDiagnosticParserCoreCanonicalTreeDigest(t testing.TB, tree *Tree, lang *Language) string {
	t.Helper()
	if diagnosticParserCoreCanonicalTreeDigestForTest == nil {
		t.Fatal("canonical tree digest helper was not installed")
	}
	digest, err := diagnosticParserCoreCanonicalTreeDigestForTest(tree, lang)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
