//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

const (
	parserCoreQueryCompileSourceBytes   = 20168
	parserCoreQueryCompileSourceSHA256  = "b788ee19b0075f0b9b567a9f93ea657e715bc8a6a40a99d3ca5c761404e71894"
	parserCoreQueryCompileGrammarSHA256 = "9cf914d26d962d1a62e7954f8b20b302337a44cb7d4a07218eec482c45a57a08"
	parserCoreQueryCompileTreeSHA256    = "ecc090a83a4343a1c7c2afbad63277f5b4d60c42d8d94a2af2a9b16e46f2ccb5"
)

type parserCoreQueryCompileComparison struct {
	shifts, reductions, paths, payloads, links, leaves, parents uint64
}

var (
	parserCoreQueryCompileDirectC = parserCoreQueryCompileComparison{
		shifts: 6823, reductions: 7745, paths: 8290, payloads: 14893,
		links: 15734, leaves: 5654, parents: 8291,
	}
	parserCoreQueryCompileContinue = parserCoreQueryCompileComparison{
		shifts: 8187, reductions: 9294, paths: 9948, payloads: 17871,
		links: 18880, leaves: 6784, parents: 9949,
	}
	parserCoreQueryCompileKill = parserCoreQueryCompileComparison{
		shifts: 10235, reductions: 11618, paths: 12435, payloads: 22340,
		links: 23601, leaves: 8481, parents: 12437,
	}
)

func TestDiagnosticParserCoreSummaryAcceptsExactQueryCompile(t *testing.T) {
	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		t.Fatal(err)
	}
	var fixture benchfixtures.LoadedFixture
	for _, candidate := range fixtures {
		if candidate.Fixture.ID == "query_compile" {
			fixture = candidate
			break
		}
	}
	if fixture.Fixture.ID == "" {
		t.Fatal("query_compile fixture missing")
	}
	if fixture.Fixture.UncompressedBytes != parserCoreQueryCompileSourceBytes || fixture.Fixture.SHA256 != parserCoreQueryCompileSourceSHA256 || fixture.Fixture.DeepTreeSHA256 != parserCoreQueryCompileTreeSHA256 {
		t.Fatalf("query_compile fixture identity drifted: %+v", fixture.Fixture)
	}
	if len(fixture.Source) != parserCoreQueryCompileSourceBytes || fmt.Sprintf("%x", sha256.Sum256(fixture.Source)) != parserCoreQueryCompileSourceSHA256 {
		t.Fatalf("query_compile source identity drifted: bytes=%d sha256=%x", len(fixture.Source), sha256.Sum256(fixture.Source))
	}
	if err := fixture.Fixture.VerifySource(fixture.Source); err != nil {
		t.Fatal(err)
	}
	blobHash := sha256.Sum256(grammars.BlobByName("go"))
	if fmt.Sprintf("%x", blobHash) != parserCoreQueryCompileGrammarSHA256 {
		t.Fatalf("Go grammar identity drifted: %x", blobHash)
	}

	first := runDiagnosticParserCoreQueryCompile(t, fixture)
	wantWork := core.Work{
		Shifts: 6685, Reductions: 7440, ReductionPopRequests: 7440,
		EmittedPopPaths: 8103, EmittedPopPayloads: 14703,
		GraphLinkAdditionsProxy: 14749, LeafConstructionsProxy: 5546,
		ParentConstructionsProxy: 7537,
	}
	if first.Acceptance.CoreWork != wantWork {
		t.Fatalf("query_compile parser-core work drifted: got=%+v want=%+v", first.Acceptance.CoreWork, wantWork)
	}
	observed := parserCoreQueryCompileComparisonFromWork(first.Acceptance.CoreWork)
	classification := "between_continue_and_kill"
	if parserCoreQueryCompileComparisonWithin(observed, parserCoreQueryCompileContinue) {
		classification = "continue"
	} else if parserCoreQueryCompileComparisonAtOrAbove(observed, parserCoreQueryCompileKill) {
		classification = "kill"
	}
	t.Logf("query_compile parser-core work=%+v direct_c=%+v continue_1.2x=%+v kill_1.5x=%+v classification=%s selected=%d/%d/%d", observed, parserCoreQueryCompileDirectC, parserCoreQueryCompileContinue, parserCoreQueryCompileKill, classification, first.Acceptance.SelectedNodes, first.Acceptance.SelectedParents, first.Acceptance.SelectedLeaves)
	if classification != "continue" {
		t.Fatalf("query_compile parser-core work exceeded the 1.2x continue vector: observed=%+v classification=%s", observed, classification)
	}
	for run := 1; run < 3; run++ {
		next := runDiagnosticParserCoreQueryCompile(t, fixture)
		if !reflect.DeepEqual(first, next) {
			t.Fatalf("query_compile summary run %d drifted\nfirst=%+v\nnext=%+v", run, first, next)
		}
	}
}

func runDiagnosticParserCoreQueryCompile(t *testing.T, fixture benchfixtures.LoadedFixture) *gotreesitter.DiagnosticParserCoreGenericScheduler {
	t.Helper()
	result, routeErr := gotreesitter.DiagnosticParseParserCorePrefix(
		grammars.GoExternalScanner{}, fixture.Source,
		gotreesitter.DiagnosticParserCorePrefixOptions{
			ReceiptMode:   gotreesitter.DiagnosticParserCoreReceiptSummary,
			MaxTokens:     25000,
			MaxDispatches: 50000,
			Limits: core.Limits{
				MaxNodes: 65536, MaxLinks: 65536, MaxSubtrees: 65536,
				MaxChildren: 262144, MaxMetadata: 131072,
				MaxLinksPerBoundary: 8, MaxPopPaths: 4096, MaxDerivations: 4096,
			},
		},
	)
	if routeErr != nil {
		t.Fatalf("query_compile parser-core boundary=%s detail=%q tokens=%d dispatches=%d: %v", result.Boundary, result.Detail, result.Tokens, result.Dispatches, routeErr)
	}
	if result.MaterializedTree != nil {
		t.Cleanup(result.MaterializedTree.Release)
	}
	if !result.Completed || !result.Materialized || result.MaterializedTree == nil || result.GenericScheduler == nil || result.GenericScheduler.Acceptance == nil {
		t.Fatalf("query_compile did not publish an accepted tree: %+v", result)
	}
	generic := result.GenericScheduler
	acceptance := generic.Acceptance
	emptyCheckpoint := gotreesitter.DiagnosticParserCoreScannerCheckpoint{SHA256: sha256.Sum256(nil)}
	if generic.ReceiptMode != gotreesitter.DiagnosticParserCoreReceiptSummary || generic.StartCheckpoint != emptyCheckpoint || generic.StartHeaders != nil || generic.Rounds != nil || generic.Conflicts != nil || generic.ExternalShifts != nil || generic.Elections != nil || generic.NoActionDrops != nil || generic.Completion != nil || !reflect.DeepEqual(generic.Stop, gotreesitter.DiagnosticParserCoreGenericStop{}) || result.Elections != nil || acceptance.Header.Derivations != nil || acceptance.Header.DerivationsTruncated || acceptance.Payloads != nil {
		t.Fatalf("query_compile summary retained detailed collections: %+v", generic)
	}
	if result.SourceSHA256 != sha256.Sum256(fixture.Source) || result.Grammar != "go" || !result.ExactRootDFA || fmt.Sprintf("%x", result.GrammarBlobSHA256) != parserCoreQueryCompileGrammarSHA256 ||
		result.Boundary != gotreesitter.DiagnosticParserCoreGenericClosed || result.State != 2 || result.Lookahead.Symbol != 0 || result.Lookahead.Text != "" || result.Lookahead.StartByte != uint32(len(fixture.Source)) || result.Lookahead.EndByte != uint32(len(fixture.Source)) || result.Lookahead.Missing || result.Lookahead.NoLookahead || result.Lookahead.ExternalScannerToken ||
		acceptance.Token != result.Lookahead || acceptance.Header.Header.State != 2 || acceptance.Header.Header.ByteOffset != uint32(len(fixture.Source)) || acceptance.Header.Header.Shifted || !acceptance.Header.Header.Accepted || acceptance.Header.Header.Paused || acceptance.Header.Header.ExactPaths != 1 || acceptance.Accepts != 1 || acceptance.Work.Accepts != 1 || acceptance.CoreWork.Overflow {
		t.Fatalf("query_compile identity/work acceptance drifted: source=%x grammar=%s/%x exact_dfa=%v accepts=%d/%d work=%+v", result.SourceSHA256, result.Grammar, result.GrammarBlobSHA256, result.ExactRootDFA, acceptance.Accepts, acceptance.Work.Accepts, acceptance.CoreWork)
	}
	if acceptance.SelectedNodes != 7524 || acceptance.SelectedParents != 2853 || acceptance.SelectedLeaves != 4671 || acceptance.SelectedParents+acceptance.SelectedLeaves != acceptance.SelectedNodes {
		t.Fatalf("query_compile selected census drifted: total=%d parents=%d leaves=%d", acceptance.SelectedNodes, acceptance.SelectedParents, acceptance.SelectedLeaves)
	}
	root := result.MaterializedTree.RootNode()
	runtime := result.MaterializedTree.ParseRuntime()
	inspection, inspectErr := benchfixtures.InspectGoTree(root, grammars.GoLanguage())
	if inspectErr != nil || inspection.SHA256 != parserCoreQueryCompileTreeSHA256 || root.StartByte() != 0 || root.EndByte() != uint32(len(fixture.Source)) || root.HasError() || runtime.StopReason != gotreesitter.ParseStopAccepted || runtime.Truncated || runtime.SourceLen != uint32(len(fixture.Source)) || runtime.ExpectedEOFByte != uint32(len(fixture.Source)) || runtime.RootEndByte != uint32(len(fixture.Source)) || !runtime.LastTokenWasEOF {
		t.Fatalf("query_compile exactness failed: digest=%s root=%d..%d error=%v runtime=%s inspect_err=%v", inspection.SHA256, root.StartByte(), root.EndByte(), root.HasError(), runtime.Summary(), inspectErr)
	}
	return generic
}

func parserCoreQueryCompileComparisonFromWork(work core.Work) parserCoreQueryCompileComparison {
	return parserCoreQueryCompileComparison{
		shifts: work.Shifts, reductions: work.Reductions, paths: work.EmittedPopPaths,
		payloads: work.EmittedPopPayloads, links: work.GraphLinkAdditionsProxy,
		leaves: work.LeafConstructionsProxy, parents: work.ParentConstructionsProxy,
	}
}

func parserCoreQueryCompileComparisonWithin(got, limit parserCoreQueryCompileComparison) bool {
	return got.shifts <= limit.shifts && got.reductions <= limit.reductions && got.paths <= limit.paths &&
		got.payloads <= limit.payloads && got.links <= limit.links && got.leaves <= limit.leaves && got.parents <= limit.parents
}

func parserCoreQueryCompileComparisonAtOrAbove(got, limit parserCoreQueryCompileComparison) bool {
	return got.shifts >= limit.shifts || got.reductions >= limit.reductions || got.paths >= limit.paths ||
		got.payloads >= limit.payloads || got.links >= limit.links || got.leaves >= limit.leaves || got.parents >= limit.parents
}
