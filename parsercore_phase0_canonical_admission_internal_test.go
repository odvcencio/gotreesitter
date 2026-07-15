//go:build gts_parsercorephase0

package gotreesitter

import (
	"crypto/sha256"
	"errors"
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
	work            core.Work
}

func TestDiagnosticParserCoreCanonicalAdmissions(t *testing.T) {
	rows := []diagnosticParserCoreCanonicalAdmission{
		{
			id: "rewrite", bytes: 5116,
			sourceSHA256:   "74c0705f8729670559492fb5460a01b2a1a2a109928e1aeb52736e485e8ff097",
			deepTreeSHA256: "b3f9814b65763642d4eac58b9065018048ea13e6f10d56afb28a0479bf5a68a1",
			selectedNodes:  1524, selectedParents: 572, selectedLeaves: 952,
			work: core.Work{
				Shifts: 1348, Reductions: 1501, ReductionPopRequests: 1501,
				EmittedPopPaths: 1646, EmittedPopPayloads: 2995,
				GraphLinkAdditionsProxy: 3004, LeafConstructionsProxy: 1109,
				ParentConstructionsProxy: 1515,
			},
		},
		{
			id: "query_compile", bytes: 20168,
			sourceSHA256:   "b788ee19b0075f0b9b567a9f93ea657e715bc8a6a40a99d3ca5c761404e71894",
			deepTreeSHA256: "ecc090a83a4343a1c7c2afbad63277f5b4d60c42d8d94a2af2a9b16e46f2ccb5",
			selectedNodes:  7524, selectedParents: 2853, selectedLeaves: 4671,
			work: core.Work{
				Shifts: 6685, Reductions: 7440, ReductionPopRequests: 7440,
				EmittedPopPaths: 8103, EmittedPopPayloads: 14703,
				GraphLinkAdditionsProxy: 14749, LeafConstructionsProxy: 5546,
				ParentConstructionsProxy: 7537,
			},
		},
	}
	for _, row := range rows {
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

type diagnosticParserCoreCanonicalDecline struct {
	id             string
	bytes          int
	sourceSHA256   string
	deepTreeSHA256 string
	state          uint32
	byteOffset     uint32
}

func TestDiagnosticParserCoreCanonicalExpectedCap8Declines(t *testing.T) {
	rows := []diagnosticParserCoreCanonicalDecline{
		{
			id: "language", bytes: 41387,
			sourceSHA256:   "009aa9fd5352c712f3839670c7df8a9b00ae878ee20dc88131a438b2d5edfd9a",
			deepTreeSHA256: "583df223904fe414c33bba3b474c6557ecdb20e7f47e304b9a09bfcc2da44539",
			state:          217, byteOffset: 15513,
		},
		{
			id: "grammargen_lr", bytes: 235626,
			sourceSHA256:   "a7e4a1a64b25a60aea36183b9d6d53dcd9240942cdb10e67a3cf9e6ce30f95b2",
			deepTreeSHA256: "1472cfd9a014d4034dbc1456afd12c282630ef787c3543cf0cecb73619883ad2",
			state:          218, byteOffset: 8559,
		},
	}
	for _, row := range rows {
		row := row
		t.Run(row.id, func(t *testing.T) {
			fixture := loadDiagnosticParserCoreCanonicalFixture(t, row.id)
			requireDiagnosticParserCoreCanonicalFixtureIdentity(t, fixture, diagnosticParserCoreCanonicalAdmission{
				id: row.id, bytes: row.bytes, sourceSHA256: row.sourceSHA256, deepTreeSHA256: row.deepTreeSHA256,
			})
			result, routeErr := DiagnosticParseParserCorePrefix(
				parserCoreWarmGoScanner, fixture.Source,
				DiagnosticParserCorePrefixOptions{
					ReceiptMode: DiagnosticParserCoreReceiptSummary,
					MaxTokens:   300000, MaxDispatches: 600000,
					Limits: diagnosticParserCoreCanonicalLimits(),
				},
			)
			var capacity *core.LiveLinkCapacityError
			if !errors.As(routeErr, &capacity) {
				t.Fatalf("cap8 decline=%v, want *parsercorephase0.LiveLinkCapacityError; result=%+v", routeErr, result)
			}
			if capacity.State != core.StateID(row.state) || capacity.ByteOffset != row.byteOffset || capacity.ObservedLinks != 9 || capacity.Limit != 8 {
				t.Fatalf("cap8 decline fields=%+v, want state=%d byte=%d links=9 limit=8", capacity, row.state, row.byteOffset)
			}
			if result.Boundary != "" || result.GenericScheduler != nil || result.MaterializedTree != nil || result.Materialized || result.Completed || result.Tokens != 0 || result.Dispatches != 0 || len(result.Elections) != 0 || result.State != 0 || result.Lookahead != (Token{}) {
				t.Fatalf("expected cap8 decline leaked publication: %+v", result)
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
