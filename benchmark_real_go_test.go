package gotreesitter_test

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

//go:embed grammars/languages.lock
var benchmarkLanguagesLock string

// BenchmarkGoParseWarmRealDFA measures warm full-parse lifecycle cost on
// authenticated snapshots of human-authored Go source. Fixture decompression,
// grammar loading, parser construction, arena-pool draining, and one explicit
// warm-up parse all happen outside the timed region. Each timed operation owns
// and releases exactly one fully validated tree.
func BenchmarkGoParseWarmRealDFA(b *testing.B) {
	if err := verifyRealGoBenchmarkGrammarIdentity(); err != nil {
		b.Fatal(err)
	}
	statsEnabled, err := validateRealGoBenchmarkEnvironment(os.Environ())
	if err != nil {
		b.Fatal(err)
	}
	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		b.Fatal(err)
	}
	lang := grammars.GoLanguage()
	if statsEnabled {
		b.Log("GOT_STATS enabled: diagnostic/non-publication benchmark lane")
		gotreesitter.EnableRuntimeAudit(true)
		defer gotreesitter.EnableRuntimeAudit(false)
	}

	for _, fixture := range fixtures {
		fixture := fixture
		name := fixture.Fixture.ID
		if statsEnabled {
			name += "_diagnostic_stats_nonpublication"
		}
		b.Run(name, func(b *testing.B) {
			benchmarkWarmRealGoDFA(b, fixture, lang)
		})
	}
}

func admitRealGoBenchmarkFixture(tb testing.TB, fixture benchfixtures.LoadedFixture, lang *gotreesitter.Language) benchfixtures.NodeKindCoverage {
	tb.Helper()
	parser := gotreesitter.NewParser(lang)
	// Pin to production: this admission fixture asserts production-engine GLR
	// internals (MaxStacksSeen forking) alongside the deep-tree digest. The
	// compact candidate route does not report those internals; its digest
	// exactness is proven by the tagged scorecard suite.
	parser.SetAdmissionCandidateRoute(false)
	tree, err := parser.Parse(fixture.Source)
	if err != nil {
		releaseBenchmarkTree(tree)
		tb.Fatalf("%s admission parse: %v", fixture.Fixture.ID, err)
	}
	if err := validateRealGoBenchmarkTree(tree, fixture.Source, lang); err != nil {
		releaseBenchmarkTree(tree)
		tb.Fatalf("%s admission parse: %v", fixture.Fixture.ID, err)
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
	if err != nil {
		releaseBenchmarkTree(tree)
		tb.Fatalf("%s admission parse inspection: %v", fixture.Fixture.ID, err)
	}
	if err := fixture.Fixture.VerifyDeepTreeDigest(inspection.SHA256); err != nil {
		releaseBenchmarkTree(tree)
		tb.Fatalf("%s admission parse digest: %v", fixture.Fixture.ID, err)
	}
	if err := fixture.Fixture.VerifyWorkloadIdentity(tree.ParseRuntime(), inspection.NodeKinds); err != nil {
		releaseBenchmarkTree(tree)
		tb.Fatalf("%s admission workload identity: %v", fixture.Fixture.ID, err)
	}
	tree.Release()
	return inspection.NodeKinds
}

func benchmarkWarmRealGoDFA(b *testing.B, fixture benchfixtures.LoadedFixture, lang *gotreesitter.Language) {
	b.Helper()
	if err := fixture.Fixture.VerifySource(fixture.Source); err != nil {
		b.Fatal(err)
	}

	// Deep fixture/tree admission intentionally precedes the measured warm
	// state. Its full traversal can materialize lazy tree state and grow arena
	// pools, so discard that state before the single shallow warm parse.
	gotreesitter.DrainArenaPools()
	admitRealGoBenchmarkFixture(b, fixture, lang)
	gotreesitter.DrainArenaPools()
	parser := gotreesitter.NewParser(lang)
	// Pin to production, mirroring admitRealGoBenchmarkFixture's own parser
	// above: this benchmark reports and asserts on GLR-only runtime counters
	// (max_stacks, peak_depth, nodes/op below), which the compact candidate
	// route never populates. Phase-3 admission defaults the candidate route
	// on, and every one of this benchmark's fixtures is DFA-eligible, so an
	// unpinned parser here silently measures the compact route on both sides
	// of an A/B comparison instead of production (b4b-width-repair audit,
	// 2026-08; oak-b's v9 decomposition first caught this reproducing
	// unmodified on origin/main). Tranche B9 removed the source-length
	// eligibility ceiling that used to additionally exempt the largest
	// fixture from this risk; every fixture now needs this explicit pin. The
	// guard assertion after the timed loop below fails loudly if this pin is
	// ever lost again, instead of silently re-conflating the two routes.
	parser.SetAdmissionCandidateRoute(false)
	warmTree, err := parser.Parse(fixture.Source)
	if err != nil {
		releaseBenchmarkTree(warmTree)
		b.Fatalf("%s warm parse: %v", fixture.Fixture.ID, err)
	}
	if err := validateRealGoBenchmarkTree(warmTree, fixture.Source, lang); err != nil {
		releaseBenchmarkTree(warmTree)
		b.Fatalf("%s warm parse: %v", fixture.Fixture.ID, err)
	}
	warmTree.Release()

	routedBefore, _ := gotreesitter.AdmissionCandidateCounters()

	b.ReportAllocs()
	b.SetBytes(int64(len(fixture.Source)))
	b.ResetTimer()

	var lastRuntime gotreesitter.ParseRuntime
	for i := 0; i < b.N; i++ {
		tree, err := parser.Parse(fixture.Source)
		if err != nil {
			releaseBenchmarkTree(tree)
			b.Fatalf("%s timed parse: %v", fixture.Fixture.ID, err)
		}
		if err := validateRealGoBenchmarkTree(tree, fixture.Source, lang); err != nil {
			releaseBenchmarkTree(tree)
			b.Fatalf("%s timed parse: %v", fixture.Fixture.ID, err)
		}
		if i == b.N-1 {
			lastRuntime = tree.ParseRuntime()
		}
		tree.Release()
	}
	b.StopTimer()
	verifyRealGoBenchmarkMeasuredProduction(b, fixture.Fixture.ID, routedBefore, lastRuntime)
	reportRealGoRuntime(b, lastRuntime)
}

// verifyRealGoBenchmarkMeasuredProduction guards against silently re-
// conflating the compact candidate route into a benchmark that pins and
// reports on the production route (b4b-width-repair audit, 2026-08; see
// benchmarkWarmRealGoDFA's SetAdmissionCandidateRoute comment above). It
// fails closed on either signal alone: the process-wide candidate-route
// counter must not have moved across the timed loop (routedBefore is
// sampled after the pinned warm parse, so this isolates the b.N loop), and
// the final timed parse's GLR-only runtime counters -- which the compact
// route never populates -- must be nonzero. A genuine production route parse
// always seeds at least one GSS stack, so MaxStacksSeen/PeakStackDepth are
// never legitimately zero on success.
func verifyRealGoBenchmarkMeasuredProduction(b *testing.B, fixtureID string, routedBefore uint64, rt gotreesitter.ParseRuntime) {
	b.Helper()
	routedAfter, _ := gotreesitter.AdmissionCandidateCounters()
	if routedAfter != routedBefore {
		b.Fatalf("%s: compact candidate route counter moved (%d -> %d) during the pinned timed loop: this benchmark is measuring the compact route, not production", fixtureID, routedBefore, routedAfter)
	}
	if rt.MaxStacksSeen == 0 && rt.PeakStackDepth == 0 {
		b.Fatalf("%s: timed parse reported MaxStacksSeen=0 and PeakStackDepth=0 -- GLR-only counters the compact route never populates; this benchmark is not measuring production", fixtureID)
	}
}

func verifyRealGoBenchmarkGrammarIdentity() error {
	commit, err := lockedGrammarCommit(benchmarkLanguagesLock, "go")
	if err != nil {
		return err
	}
	blob := grammars.BlobByName("go")
	if len(blob) == 0 {
		return fmt.Errorf("Go benchmark grammar blob is empty")
	}
	blobSHA256 := fmt.Sprintf("%x", sha256.Sum256(blob))
	return benchfixtures.VerifyGoGrammarIdentity(commit, blobSHA256)
}

func lockedGrammarCommit(lock, language string) (string, error) {
	for _, line := range strings.Split(lock, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == language {
			if len(fields) < 3 {
				return "", fmt.Errorf("languages.lock row for %q has %d fields, want at least 3", language, len(fields))
			}
			return fields[2], nil
		}
	}
	return "", fmt.Errorf("languages.lock has no row for %q", language)
}

func validateRealGoBenchmarkEnvironment(environ []string) (bool, error) {
	statsEnabled := false
	var forbidden []string
	for _, entry := range environ {
		name, value, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(name, "GOT_") {
			continue
		}
		if name == "GOT_STATS" {
			statsEnabled = strings.TrimSpace(value) != ""
			continue
		}
		forbidden = append(forbidden, name)
	}
	if len(forbidden) > 0 {
		sort.Strings(forbidden)
		return false, fmt.Errorf("publication benchmark refuses GOT_* overrides; unset %s", strings.Join(forbidden, ", "))
	}
	return statsEnabled, nil
}

func validateRealGoBenchmarkTree(tree *gotreesitter.Tree, source []byte, lang *gotreesitter.Language) error {
	if tree == nil {
		return fmt.Errorf("parse returned nil tree")
	}
	root := tree.RootNode()
	if root == nil {
		return fmt.Errorf("parse returned nil root")
	}
	if got, want := root.StartByte(), uint32(0); got != want {
		return fmt.Errorf("root.StartByte=%d want=%d", got, want)
	}
	if got, want := root.EndByte(), uint32(len(source)); got != want {
		return fmt.Errorf("root.EndByte=%d want=%d (%s)", got, want, tree.ParseRuntime().Summary())
	}
	if root.HasError() {
		return fmt.Errorf("root %q has errors (%s)", root.Type(lang), tree.ParseRuntime().Summary())
	}
	if tree.ParseStoppedEarly() {
		return fmt.Errorf("parse stopped early (%s)", tree.ParseRuntime().Summary())
	}
	return nil
}

func releaseBenchmarkTree(tree *gotreesitter.Tree) {
	if tree != nil {
		tree.Release()
	}
}

func reportRealGoRuntime(b *testing.B, rt gotreesitter.ParseRuntime) {
	b.Helper()
	b.ReportMetric(float64(rt.MaxStacksSeen), "max_stacks")
	b.ReportMetric(float64(rt.PeakStackDepth), "peak_depth")
	b.ReportMetric(float64(rt.TokensConsumed), "tokens/op")
	b.ReportMetric(float64(rt.MultiStackIterations), "multi_iters/op")
	b.ReportMetric(float64(rt.MultiStackTokens), "multi_tokens/op")
	b.ReportMetric(float64(rt.NodesAllocated), "nodes/op")
	b.ReportMetric(float64(rt.ArenaBytesAllocated), "arena_B/op")
	b.ReportMetric(float64(rt.NormalizationPassesRun), "normalization_runs/op")
	if rt.ForestFastPath {
		b.ReportMetric(1, "forest_fast_path")
	} else {
		b.ReportMetric(0, "forest_fast_path")
	}
	constructed := rt.LeafNodesConstructed + rt.ParentNodesConstructed
	if rt.FinalNodes > 0 && constructed > 0 {
		b.ReportMetric(float64(constructed)/float64(rt.FinalNodes), "constructed/final")
	}
}

func TestGoFullParseBenchmarkFixturesParseClean(t *testing.T) {
	gotreesitter.DrainArenaPools()
	t.Cleanup(gotreesitter.DrainArenaPools)
	if err := verifyRealGoBenchmarkGrammarIdentity(); err != nil {
		t.Fatal(err)
	}
	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		t.Fatal(err)
	}
	lang := grammars.GoLanguage()
	suiteCoverage := make(benchfixtures.NodeKindCoverage)
	for _, fixture := range fixtures {
		suiteCoverage.Merge(admitRealGoBenchmarkFixture(t, fixture, lang))
	}
	if err := benchfixtures.VerifyGoFullParseSuiteNodeKindCoverage(suiteCoverage); err != nil {
		t.Fatalf("admission workload identity: %v", err)
	}
}

func TestRealGoBenchmarkPublicationEnvironment(t *testing.T) {
	stats, err := validateRealGoBenchmarkEnvironment([]string{"PATH=/bin", "GOT_STATS=1"})
	if err != nil || !stats {
		t.Fatalf("diagnostic stats lane: stats=%v err=%v", stats, err)
	}
	if _, err := validateRealGoBenchmarkEnvironment([]string{"GOT_GLR_FOREST=0", "GOT_PARSE_NODE_LIMIT_SCALE=3"}); err == nil {
		t.Fatal("publication environment unexpectedly admitted parser overrides")
	}
}

func TestLockedGrammarCommit(t *testing.T) {
	commit, err := lockedGrammarCommit("go https://example.test/go abc123 src .go\n", "go")
	if err != nil || commit != "abc123" {
		t.Fatalf("commit=%q err=%v", commit, err)
	}
	if _, err := lockedGrammarCommit("go too-short\n", "go"); err == nil {
		t.Fatal("malformed lock row unexpectedly admitted")
	}
}
