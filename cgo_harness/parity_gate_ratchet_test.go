//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bufio"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

const (
	// Ratchets: these should only move in the "stricter" direction over time.
	minCuratedStructuralLanguages = 206
	minCuratedHighlightLanguages  = 200
	maxKnownDegradedStructural    = 0
	maxKnownDegradedNoErrorClean  = 0
	maxKnownDegradedHighlight     = 49
	maxParitySkips                = 0
)

// allowedKnownDegradedStructural is the membership half of the structural
// skip-list ratchet. The numeric ceiling above prevents growth, while this
// allow-list prevents replacing an approved exemption with a newly degraded
// language at the same count. When a verified fix removes an exemption, the
// same tightening change must remove it here too; additions or renames require
// an explicit policy change in both places.
var allowedKnownDegradedStructural = map[string]struct{}{
	// Empty: Norg's alias-target divergence was the final approved structural
	// exemption, and PR #214 removed it from knownDegradedStructural.
}

var javascriptParityRegressionCITests = []string{
	"TestParityJavaScriptAutomaticSemicolonBeforeTrailingComment",
	"TestParityJavaScriptAdjacentBlockCommentsDoNotConsumeFollowingToken",
}

// TestParityGateCoverageRatchet prevents silent narrowing of correctness gates.
// Update these thresholds only when intentionally tightening/loosening policy.
func TestParityGateCoverageRatchet(t *testing.T) {
	assertJavaScriptRegressionWorkflowCoverage(t)
	assertRequiredBuildFreshness(t)

	if got := len(curatedStructuralLanguages); got < minCuratedStructuralLanguages {
		t.Fatalf("curatedStructuralLanguages shrank: got=%d min=%d", got, minCuratedStructuralLanguages)
	}
	if got := len(curatedHighlightLanguages); got < minCuratedHighlightLanguages {
		t.Fatalf("curatedHighlightLanguages shrank: got=%d min=%d", got, minCuratedHighlightLanguages)
	}
	if got := len(knownDegradedStructural); got > maxKnownDegradedStructural {
		t.Fatalf("knownDegradedStructural grew: got=%d max=%d", got, maxKnownDegradedStructural)
	}
	for name := range knownDegradedStructural {
		if _, ok := allowedKnownDegradedStructural[name]; !ok {
			t.Fatalf("knownDegradedStructural added or renamed unapproved entry %q; this ratchet permits removals only", name)
		}
	}
	for name := range allowedKnownDegradedStructural {
		if _, ok := knownDegradedStructural[name]; !ok {
			t.Fatalf("allowedKnownDegradedStructural contains stale entry %q; remove it with the verified skip-list tightening", name)
		}
	}
	if got := len(knownDegradedNoErrorClean); got > maxKnownDegradedNoErrorClean {
		t.Fatalf("knownDegradedNoErrorClean grew: got=%d max=%d", got, maxKnownDegradedNoErrorClean)
	}
	for name := range knownDegradedNoErrorClean {
		if _, ok := knownDegradedStructural[name]; !ok {
			t.Fatalf("knownDegradedNoErrorClean[%q] is not in knownDegradedStructural", name)
		}
	}
	if got := len(knownDegradedHighlight); got > maxKnownDegradedHighlight {
		t.Fatalf("knownDegradedHighlight grew: got=%d max=%d", got, maxKnownDegradedHighlight)
	}
	if got := len(paritySkips); got > maxParitySkips {
		t.Fatalf("paritySkips grew: got=%d max=%d", got, maxParitySkips)
	}
}

func assertRequiredBuildFreshness(t *testing.T) {
	t.Helper()

	workflow, err := os.ReadFile("../.github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("read CI workflow: %v", err)
	}
	build := workflowJobBlock(string(workflow), "build")
	if build == "" {
		t.Fatal("CI workflow is missing the build job")
	}

	for _, required := range []string{
		"      - freshness",
		"          FRESHNESS_RESULT: ${{ needs.freshness.result }}",
		`          require_success "freshness" "$FRESHNESS_RESULT"`,
		"      - compact-route-lifecycle-history",
		"          COMPACT_ROUTE_LIFECYCLE_HISTORY_RESULT: ${{ needs.compact-route-lifecycle-history.result }}",
		`          require_success "compact-route-lifecycle-history" "$COMPACT_ROUTE_LIFECYCLE_HISTORY_RESULT"`,
	} {
		if !strings.Contains(build, required) {
			t.Errorf("CI build aggregate is missing required gate contract %q", required)
		}
	}

	history := workflowJobBlock(string(workflow), "compact-route-lifecycle-history")
	if history == "" {
		t.Fatal("CI workflow is missing the compact-route-lifecycle-history job")
	}
	for _, required := range []string{
		"          fetch-depth: 0",
		"          GTS_REQUIRE_HISTORICAL_RECEIPT_PROOF: \"1\"",
		"go test . -run '^TestCompactRouteCampaignLifecycleRegistry$' -count=1",
	} {
		if !strings.Contains(history, required) {
			t.Errorf("CI historical lifecycle job is missing contract %q", required)
		}
	}
}

func workflowJobBlock(workflow, job string) string {
	lines := strings.Split(workflow, "\n")
	header := "  " + job + ":"
	start := -1
	for i, line := range lines {
		if line == header {
			start = i
			break
		}
	}
	if start < 0 {
		return ""
	}

	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(line, ":") {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}

func assertJavaScriptRegressionWorkflowCoverage(t *testing.T) {
	t.Helper()

	workflow, err := os.Open("../.github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("open CI workflow: %v", err)
	}
	defer workflow.Close()

	selectors := make(map[string]string)
	var label string
	scanner := bufio.NewScanner(workflow)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "--label ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				label = strings.TrimSuffix(fields[1], "\\")
			}
			continue
		}
		if label == "" || !strings.HasPrefix(line, "--run ") {
			continue
		}
		selector := strings.TrimSpace(strings.TrimPrefix(line, "--run "))
		selector = strings.TrimSuffix(selector, "\\")
		selectors[label] = strings.Trim(strings.TrimSpace(selector), "'")
		label = ""
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan CI workflow: %v", err)
	}

	for _, label := range []string{"ci-parity-cgo-smoke", "ci-parity-cgo-exhaustive-fresh"} {
		selector, ok := selectors[label]
		if !ok {
			t.Errorf("CI workflow is missing --run selector for --label %s", label)
			continue
		}
		re, err := regexp.Compile(selector)
		if err != nil {
			t.Errorf("CI workflow selector for %s is invalid: %v", label, err)
			continue
		}
		for _, testName := range javascriptParityRegressionCITests {
			if !re.MatchString(testName) {
				t.Errorf("CI workflow selector for %s does not run %s", label, testName)
			}
		}
	}

	for _, label := range []string{
		"ci-parity-cgo-exhaustive-incremental",
		"ci-parity-cgo-exhaustive-corpus",
		"ci-parity-cgo-exhaustive-highlight-yaml",
	} {
		selector, ok := selectors[label]
		if !ok {
			t.Errorf("CI workflow is missing --run selector for --label %s", label)
			continue
		}
		re, err := regexp.Compile(selector)
		if err != nil {
			t.Errorf("CI workflow selector for %s is invalid: %v", label, err)
			continue
		}
		for _, testName := range javascriptParityRegressionCITests {
			if re.MatchString(testName) {
				t.Errorf("CI workflow selector for %s redundantly runs %s", label, testName)
			}
		}
	}
}

// parityCaseByName looks up a parityCases entry by language name. Returns
// ok=false if the language has no registry/smoke-sample entry at all, which
// itself indicates a dead knownDegradedStructural entry.
func parityCaseByName(name string) (parityCase, bool) {
	for _, tc := range parityCases {
		if tc.name == name {
			return tc, true
		}
	}
	return parityCase{}, false
}

// TestParityKnownDegradedStructuralStillDiverges is the "stale skip" ratchet.
//
// TestParityGateCoverageRatchet above only stops knownDegradedStructural from
// GROWING past its ceiling; it never confirms the entries it already
// contains still describe a real divergence. That one-directional check is
// exactly how the list went stale in the first place: agda, apex, doxygen,
// hare, jsdoc, and rst all sat skipped long after their fresh-parse output
// matched (or came to match) the C reference exactly, because nothing ever
// re-verified them.
//
// For every remaining knownDegradedStructural entry, this test bypasses the
// skip and re-parses the language's smoke sample with both gotreesitter and
// the pinned C reference parser (the same comparison TestParityFreshParse
// performs), then requires at least one real node divergence. A language
// that now parses byte-for-byte identically to the C reference must be
// removed from knownDegradedStructural — this test fails loudly instead of
// letting the skip rot silently.
//
// Requires the C reference toolchain/cache, so — like the rest of the
// structural gate — it only runs in the exhaustive parity lane:
//
//	GOWORK=off GTS_PARITY_ALLOW_HOST=1 GTS_PARITY_MODE=exhaustive \
//	GTS_PARITY_C_REF_BUILD_CACHE=<cache dir> \
//	go test . -tags treesitter_c_parity -run TestParityKnownDegradedStructuralStillDiverges -v
func TestParityKnownDegradedStructuralStillDiverges(t *testing.T) {
	parityRequireExhaustive(t, "TestParityKnownDegradedStructuralStillDiverges")

	names := make([]string, 0, len(knownDegradedStructural))
	for name := range knownDegradedStructural {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			if parityLanguageExcluded(name) {
				t.Skipf("[%s] excluded via GTS_PARITY_SKIP_LANGS", name)
			}
			parityMaybeParallel(t)
			scheduleParityMemoryScavenge(t)

			tc, ok := parityCaseByName(name)
			if !ok {
				t.Fatalf("knownDegradedStructural[%q] has no matching language registry entry (dead skip-list entry, remove it)", name)
			}

			cLang, err := ParityCLanguage(tc.name)
			if err != nil {
				if skipReason := parityReferenceSkipReason(err); skipReason != "" {
					t.Skipf("[%s] skip C reference parser: %s", tc.name, skipReason)
				}
				t.Fatalf("[%s] load C parser from languages.lock: %v", tc.name, err)
			}

			src := normalizedSource(tc.name, tc.source)
			goTree, goLang, err := parseWithGo(tc, src, nil)
			if err != nil {
				t.Fatalf("[%s] gotreesitter parse error: %v", tc.name, err)
			}
			defer releaseGoTree(goTree)

			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				if skipReason := parityReferenceSkipReason(err); skipReason != "" {
					t.Skipf("[%s] skip C reference parser SetLanguage: %s", tc.name, skipReason)
				}
				t.Fatalf("[%s] C parser SetLanguage error: %v", tc.name, err)
			}
			cTree := cParser.Parse(src, nil)
			if cTree == nil {
				t.Fatalf("[%s] C reference parser returned nil tree", tc.name)
			}
			defer cTree.Close()
			cRoot := cTree.RootNode()
			if cRoot == nil {
				t.Fatalf("[%s] C reference parser returned nil root node", tc.name)
			}

			var errs []string
			compareNodes(goTree.RootNode(), goLang, cRoot, "root", &errs)
			if len(errs) == 0 {
				t.Fatalf("stale skip: %q is listed in knownDegradedStructural but its fresh parse now matches the C reference exactly (0 node divergences) — remove %q from knownDegradedStructural", name, name)
			}
			t.Logf("[%s] confirmed live divergence: %d node mismatch(es)", name, len(errs))
		})
	}
}
