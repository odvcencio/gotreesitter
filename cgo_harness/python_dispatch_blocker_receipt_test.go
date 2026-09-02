//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type pythonDispatchPassExpectation struct {
	recorded  bool
	checked   uint64
	run       uint64
	visited   uint64
	rewritten uint64
}

type pythonDispatchBlockerWitness struct {
	name               string
	source             []byte
	wantSourceSHA      string
	wantRawDigest      string
	wantGoDigest       string
	wantCDigest        string
	wantRawDiff        *DumpV1Divergence
	wantRouteDiff      *DumpV1Divergence
	wantCompactMode    string
	wantRoutedBefore   uint64
	wantFallbackBefore uint64
	wantRoutedAfter    uint64
	wantFallbackAfter  uint64
	wantProduction     pythonDispatchPassExpectation
	wantCompact        pythonDispatchPassExpectation
	wantForest         pythonDispatchPassExpectation
	wantIncremental    pythonDispatchPassExpectation
}

// TestPythonDispatchBlockerReceiptRoutes records the live Python arm on all
// required routes before a possible retirement.
func TestPythonDispatchBlockerReceiptRoutes(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := grammars.PythonLanguage()
	if language.ExternalScanner == nil {
		t.Fatal("Python language has no registered external scanner")
	}
	if reusable, ok := language.ExternalScanner.(gotreesitter.IncrementalReuseExternalScanner); !ok || !reusable.SupportsIncrementalReuse() {
		t.Fatal("Python external scanner does not expose authenticated incremental reuse")
	}
	cLanguage, err := COracleLanguage("python")
	if err != nil {
		t.Fatal(err)
	}
	startRouted, startFallback := gotreesitter.AdmissionCandidateCounters()

	witnesses := []pythonDispatchBlockerWitness{
		{
			name:          "assignment_bare_tuple_positive",
			source:        []byte("x, y, z = 1, 2, 3\nxyz = x, y, z\n"),
			wantSourceSHA: "6a1661337725eea3d5f3e26c38c3c3536f2c9fbfb66e04ae73f2dcc1a1afdd03",
			wantRawDigest: "1ee859d4c1d2489f24dd57e0671a1832480b1c43afb960c10f798ce9f71f9759",
			wantGoDigest:  "577a8b7b9281fa12c48dfa239a977c82f3a94e3d248253663c4a6fafc9121622",
			wantCDigest:   "577a8b7b9281fa12c48dfa239a977c82f3a94e3d248253663c4a6fafc9121622",
			wantRawDiff: pythonDispatchExpectedDivergence(
				"/module/assignment[1]/pattern_list[2]", "type", "pattern_list", "expression_list",
			),
			wantCompactMode:    "accepted",
			wantRoutedBefore:   0,
			wantFallbackBefore: 0,
			wantRoutedAfter:    1,
			wantFallbackAfter:  0,
			wantProduction:     pythonDispatchPassExpectation{recorded: true, checked: 1, run: 1, visited: 24, rewritten: 1},
			wantCompact:        pythonDispatchPassExpectation{},
			wantForest:         pythonDispatchPassExpectation{recorded: true, checked: 1, run: 1, visited: 24},
			wantIncremental:    pythonDispatchPassExpectation{recorded: true, checked: 1, run: 1, visited: 24, rewritten: 1},
		},
		{
			name:          "fstring_interpolation_bare_tuple_recovery_gap",
			source:        []byte("x = 1\ny = 2\nz = f\"{x, y}\"\n"),
			wantSourceSHA: "7d0029944fcffb700144302da9b1b80b03da8f89d716772b3a207dca9ba543a7",
			wantRawDigest: "89ca835ae4fb5cf19d38e40b6ae4f09c99c66987ca77d8b4921aa9f593aaa641",
			wantGoDigest:  "89ca835ae4fb5cf19d38e40b6ae4f09c99c66987ca77d8b4921aa9f593aaa641",
			wantCDigest:   "84c987ddc73cc06bcc63e0cc860ecaa58560a46db882c775815ecb8867f95c07",
			wantRawDiff: pythonDispatchExpectedDivergence(
				"/module/assignment[2]/string[2]/interpolation[1]/pattern_list[1]", "type", "pattern_list", "expression_list",
			),
			wantRouteDiff: pythonDispatchExpectedDivergence(
				"/module/assignment[2]/string[2]/interpolation[1]/pattern_list[1]", "type", "pattern_list", "expression_list",
			),
			wantCompactMode:    "accepted",
			wantRoutedBefore:   1,
			wantFallbackBefore: 0,
			wantRoutedAfter:    2,
			wantFallbackAfter:  0,
			wantProduction:     pythonDispatchPassExpectation{recorded: true, checked: 1, run: 1, visited: 22},
			wantCompact:        pythonDispatchPassExpectation{},
			wantForest:         pythonDispatchPassExpectation{recorded: true, checked: 1, run: 1, visited: 22, rewritten: 1},
			wantIncremental:    pythonDispatchPassExpectation{recorded: true, checked: 1, run: 1, visited: 22},
		},
		{
			name:          "fstring_interpolation_splat_recovery_gap",
			source:        []byte("xs = [1, 2]\nz = f\"{*xs,}\"\n"),
			wantSourceSHA: "660a9ed55b63e6b98cfc70db1776895ec9046a16c906c33b3d273bee496a121d",
			wantRawDigest: "102ebedd10a3864a2640cb293f541e42f63b4f1ce3d60c9f219d7088b4f484c6",
			wantGoDigest:  "102ebedd10a3864a2640cb293f541e42f63b4f1ce3d60c9f219d7088b4f484c6",
			wantCDigest:   "e646688923780dab15e472c1754d89e87ebfdb669fafeda109d4a2d630b4a4c9",
			wantRawDiff: pythonDispatchExpectedDivergence(
				"/module/assignment[1]/string[2]/interpolation[1]/pattern_list[1]", "type", "pattern_list", "expression_list",
			),
			wantRouteDiff: pythonDispatchExpectedDivergence(
				"/module/assignment[1]/string[2]/interpolation[1]/pattern_list[1]", "type", "pattern_list", "expression_list",
			),
			wantCompactMode:    "accepted",
			wantRoutedBefore:   2,
			wantFallbackBefore: 0,
			wantRoutedAfter:    3,
			wantFallbackAfter:  0,
			wantProduction:     pythonDispatchPassExpectation{recorded: true, checked: 1, run: 1, visited: 24},
			wantCompact:        pythonDispatchPassExpectation{},
			wantForest:         pythonDispatchPassExpectation{recorded: true, checked: 1, run: 1, visited: 24},
			wantIncremental:    pythonDispatchPassExpectation{recorded: true, checked: 1, run: 1, visited: 24},
		},
	}

	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			source := append([]byte(nil), witness.source...)
			sourceSHA := fmt.Sprintf("%x", sha256.Sum256(source))
			if sourceSHA != witness.wantSourceSHA {
				t.Fatalf("source SHA-256=%s, want %s", sourceSHA, witness.wantSourceSHA)
			}

			cTree := pythonDispatchCTree(t, cLanguage, source)
			defer cTree.Close()
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			if cDigest != witness.wantCDigest {
				t.Fatalf("locked-C digest=%s, want %s", cDigest, witness.wantCDigest)
			}
			if cTree.RootNode().HasError() {
				t.Fatal("locked-C witness unexpectedly has an error root")
			}

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatal(err)
			}
			defer raw.Release()
			rawDiff := pythonDispatchAssertReceipt(t, "raw", raw, language, cTree, cDigest)
			pythonDispatchCheckDigest(t, "raw", raw, language, witness.wantRawDigest)
			pythonDispatchCheckDivergence(t, "raw", rawDiff, witness.wantRawDiff)
			pythonDispatchCheckPass(t, "raw", raw, pythonDispatchPassExpectation{})
			pythonDispatchCheckNativeAuthority(t, "raw", raw)

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer production.Release()
			productionDiff := pythonDispatchAssertReceipt(t, "production", production, language, cTree, cDigest)
			pythonDispatchCheckDigest(t, "production", production, language, witness.wantGoDigest)
			pythonDispatchCheckDivergence(t, "production", productionDiff, witness.wantRouteDiff)
			pythonDispatchCheckPass(t, "production", production, witness.wantProduction)
			pythonDispatchCheckNativeAuthority(t, "production", production)

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			wantRoutedBefore := startRouted + witness.wantRoutedBefore
			wantFallbackBefore := startFallback + witness.wantFallbackBefore
			wantRoutedAfter := startRouted + witness.wantRoutedAfter
			wantFallbackAfter := startFallback + witness.wantFallbackAfter
			if routedBefore != wantRoutedBefore || fallbackBefore != wantFallbackBefore {
				t.Fatalf("compact counters before=%d/%d, want %d/%d", routedBefore, fallbackBefore, wantRoutedBefore, wantFallbackBefore)
			}
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer compact.Release()
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			compactMode := pythonDispatchCompactMode(routedBefore, fallbackBefore, routedAfter, fallbackAfter)
			if compactMode != witness.wantCompactMode || routedAfter != wantRoutedAfter || fallbackAfter != wantFallbackAfter {
				t.Fatalf("compact mode=%s counters=%d/%d->%d/%d, want %s %d/%d->%d/%d", compactMode, routedBefore, fallbackBefore, routedAfter, fallbackAfter, witness.wantCompactMode, wantRoutedBefore, wantFallbackBefore, wantRoutedAfter, wantFallbackAfter)
			}
			compactDiff := pythonDispatchAssertReceipt(t, "compact", compact, language, cTree, cDigest)
			pythonDispatchCheckDigest(t, "compact", compact, language, witness.wantGoDigest)
			pythonDispatchCheckDivergence(t, "compact", compactDiff, witness.wantRouteDiff)
			pythonDispatchCheckPass(t, "compact", compact, witness.wantCompact)
			pythonDispatchCheckNativeAuthority(t, "compact", compact)

			forestParser := gotreesitter.NewParser(language)
			forest, forestOK := forestParser.ParseForestExperimental(source)
			if !forestOK || forest == nil {
				t.Fatal("forest route declined")
			}
			defer forest.Release()
			forestDiff := pythonDispatchAssertReceipt(t, "forest", forest, language, cTree, cDigest)
			pythonDispatchCheckDigest(t, "forest", forest, language, witness.wantGoDigest)
			pythonDispatchCheckDivergence(t, "forest", forestDiff, witness.wantRouteDiff)
			pythonDispatchCheckPass(t, "forest", forest, witness.wantForest)
			pythonDispatchCheckNativeAuthority(t, "forest", forest)

			incrementalParser := gotreesitter.NewParser(language)
			incrementalParser.SetAdmissionCandidateRoute(false)
			base := bytes.TrimSuffix(source, []byte{'\n'})
			oldTree, err := incrementalParser.Parse(base)
			if err != nil {
				t.Fatal(err)
			}
			defer oldTree.Release()
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(base)),
				OldEndByte:  uint32(len(base)),
				NewEndByte:  uint32(len(source)),
				StartPoint:  pythonDispatchPoint(base),
				OldEndPoint: pythonDispatchPoint(base),
				NewEndPoint: pythonDispatchPoint(source),
			})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			defer incremental.Release()
			incrementalDiff := pythonDispatchAssertReceipt(t, "incremental", incremental, language, cTree, cDigest)
			pythonDispatchCheckDigest(t, "incremental", incremental, language, witness.wantGoDigest)
			pythonDispatchCheckDivergence(t, "incremental", incrementalDiff, witness.wantRouteDiff)
			pythonDispatchCheckPass(t, "incremental", incremental, witness.wantIncremental)
			pythonDispatchCheckNativeAuthority(t, "incremental", incremental)
			if profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "" {
				t.Fatalf("incremental authenticated scanner reuse declined: unsupported=%t reason=%q old_tree=%t subtrees=%d bytes=%d", profile.ReuseUnsupported, profile.ReuseUnsupportedReason, profile.OldTreeReuseRoute, profile.ReusedSubtrees, profile.ReusedBytes)
			}

			t.Logf("route-summary compact=%s raw=%s production=%s forest=%s incremental=%s reuse=%t unsupported=%t reason=%q", compactMode, pythonDispatchPassSummary(raw), pythonDispatchPassSummary(production), pythonDispatchPassSummary(forest), pythonDispatchPassSummary(incremental), profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason)
		})
	}
}

// TestPythonDispatchBlockerReceiptDocument guards the durable receipt markers.
func TestPythonDispatchBlockerReceiptDocument(t *testing.T) {
	doc, err := os.ReadFile("../docs/root-normalization-retirement.md")
	if err != nil {
		t.Fatal(err)
	}
	changelog, err := os.ReadFile("../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	document := strings.Join(strings.Fields(string(doc)), " ")
	for _, marker := range []string{
		"## 2026-08-24 Python dispatcher blocker receipt",
		"Status: `NO-GO`. Keep `dispatch.python` live.",
		"Base commit: `14f6692fac65eab817f65af8cc6072e423ca6563`.",
		"The raw positive witness differs from locked C, but production, compact, forest, and incremental routes match locked C.",
		"The two f-string witnesses differ from locked C on every route.",
		"external_scanner_unsupported",
		"native_authoritative=false",
		"The authenticated Python corpus and corpus lock are unavailable.",
		"The sidecar records the expected corpus lock SHA, but it does not provide the corpus.",
		"The A0 (initial dispatcher census) manifest has 14 languages, 42 files, and 14 receipts. It excludes Python.",
		"The tracked census has seven fixtures. Its Python fixture is",
		"20260823T052051Z-n31c-python-blocker-final5",
		"20260823T052108Z-n31c-python-document-final6",
		"20260823T051420Z-n31c-python-receipts-final",
		"20260823T051431Z-n31c-python-census-final",
		"20260823T051442Z-n31c-python-a3-corpus-absence",
		"Canopy traces `runLanguageResultCompatibility` to `dispatcherArmCensus` and then to `normalizePythonCompatibilityWithParser`.",
		"Keep dispatch.python live until scheduler_action_semantics emits the locked-C tree for every witness and route.",
	} {
		marker = strings.Join(strings.Fields(marker), " ")
		if !strings.Contains(document, marker) {
			t.Fatalf("retirement document lacks marker %q", marker)
		}
	}
	for _, marker := range []string{
		"Record the N31c Python dispatcher blocker at main commit",
		"14f6692fac65eab817f65af8cc6072e423ca6563",
		"Keep `dispatch.python` live.",
	} {
		if !strings.Contains(string(changelog), marker) {
			t.Fatalf("changelog lacks marker %q", marker)
		}
	}
}

func pythonDispatchExpectedDivergence(path, category, goValue, cValue string) *DumpV1Divergence {
	return &DumpV1Divergence{Path: path, Category: category, GoValue: goValue, CValue: cValue}
}

func pythonDispatchCTree(t *testing.T, language *sitter.Language, source []byte) *sitter.Tree {
	t.Helper()
	parser := sitter.NewParser()
	t.Cleanup(parser.Close)
	if err := parser.SetLanguage(language); err != nil {
		t.Fatal(err)
	}
	tree := parser.Parse(source, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("C parser returned no root")
	}
	return tree
}

func pythonDispatchAssertReceipt(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, cDigest string) *DumpV1Divergence {
	t.Helper()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	diff := FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode())
	t.Logf("route=%s bytes=%d error_root=%t digest=%s c_digest=%s exact=%t rewrites=%d native_authoritative=%t divergence=%+v", route, len(tree.Source()), tree.RootNode().HasError(), inspection.SHA256, cDigest, diff == nil && inspection.SHA256 == cDigest, tree.ParseRuntime().NormalizationNodesRewritten, tree.ParseRuntime().NativeRecoveredStructureAuthoritative, diff)
	return diff
}

func pythonDispatchCheckDigest(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language, want string) {
	t.Helper()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatalf("%s inspect: %v", route, err)
	}
	if inspection.SHA256 != want {
		t.Fatalf("%s digest=%s, want %s", route, inspection.SHA256, want)
	}
}

func pythonDispatchCheckDivergence(t *testing.T, route string, got, want *DumpV1Divergence) {
	t.Helper()
	if got == nil || want == nil {
		if got != want {
			t.Fatalf("%s divergence=%+v, want %+v", route, got, want)
		}
		return
	}
	if *got != *want {
		t.Fatalf("%s divergence=%+v, want %+v", route, got, want)
	}
}

func pythonDispatchCheckPass(t *testing.T, route string, tree *gotreesitter.Tree, want pythonDispatchPassExpectation) {
	t.Helper()
	pass := pythonDispatchPass(tree)
	if !want.recorded {
		if pass != nil {
			t.Fatalf("%s recorded dispatch.python pass=%+v, want none", route, pass)
		}
		return
	}
	if pass == nil {
		t.Fatalf("%s did not record dispatch.python", route)
	}
	if pass.Checked != want.checked || pass.Run != want.run || pass.NodesVisited != want.visited || pass.NodesRewritten != want.rewritten {
		t.Fatalf("%s dispatch.python pass=%d/%d/%d/%d, want %d/%d/%d/%d", route, pass.Checked, pass.Run, pass.NodesVisited, pass.NodesRewritten, want.checked, want.run, want.visited, want.rewritten)
	}
}

func pythonDispatchCheckNativeAuthority(t *testing.T, route string, tree *gotreesitter.Tree) {
	t.Helper()
	if tree.ParseRuntime().NativeRecoveredStructureAuthoritative {
		t.Fatalf("%s native_authoritative=true, want false", route)
	}
}

func pythonDispatchPass(tree *gotreesitter.Tree) *gotreesitter.NormalizationPassRuntime {
	if tree == nil || tree.ParseRuntime().NormalizationPasses == nil {
		return nil
	}
	for i := range *tree.ParseRuntime().NormalizationPasses {
		pass := &(*tree.ParseRuntime().NormalizationPasses)[i]
		if pass.Name == "dispatch.python" {
			return pass
		}
	}
	return nil
}

func pythonDispatchPassSummary(tree *gotreesitter.Tree) string {
	pass := pythonDispatchPass(tree)
	if pass == nil {
		return "none"
	}
	return fmt.Sprintf("%d/%d/%d/%d", pass.Checked, pass.Run, pass.NodesVisited, pass.NodesRewritten)
}

func pythonDispatchCompactMode(routedBefore, fallbackBefore, routedAfter, fallbackAfter uint64) string {
	if routedAfter == routedBefore+1 && fallbackAfter == fallbackBefore {
		return "accepted"
	}
	if routedAfter == routedBefore && fallbackAfter == fallbackBefore+1 {
		return "fallback"
	}
	return fmt.Sprintf("counters=%d/%d->%d/%d", routedBefore, fallbackBefore, routedAfter, fallbackAfter)
}

func pythonDispatchPoint(source []byte) gotreesitter.Point {
	var point gotreesitter.Point
	for _, value := range source {
		if value == '\n' {
			point.Row++
			point.Column = 0
		} else {
			point.Column++
		}
	}
	return point
}
