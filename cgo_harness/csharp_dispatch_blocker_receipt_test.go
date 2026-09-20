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

type csharpDispatchWitness struct {
	name              string
	src               func(*testing.T) []byte
	wantSourceSHA     string
	wantRawDigest     string
	wantGoDigest      string
	wantCDigest       string
	wantRawDiff       *DumpV1Divergence
	wantRouteDiff     *DumpV1Divergence
	wantCompactMode   string
	wantForest        bool
	wantCompactPass   bool
	wantForestPass    bool
	wantIncremental   bool
	wantPassVisited   uint64
	wantPassRewritten uint64
	wantNative        bool
}

// TestCSharpDispatchBlockerRoutes records the live C# arm on raw, production,
// compact, forest, incremental, and locked-C routes.
func TestCSharpDispatchBlockerRoutes(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	witnesses := []csharpDispatchWitness{
		{
			name: "a0-jsontextreader", src: func(t *testing.T) []byte {
				source, err := os.ReadFile("../testdata/parser_result/csharp/jsontextreader_excerpt.cs")
				if err != nil {
					t.Fatal(err)
				}
				return source
			},
			wantSourceSHA:     "d76fd62cfc90076c11d86cb7d7a0058df181231aa3b34f30e549f650b5294d4a",
			wantRawDigest:     "849c28fff8597795f35cf012d256537489e12e0495df5e9901fca701c8c24f6b",
			wantGoDigest:      "fe8b4c540427c4864e168a494f3ae432b55f2ef81156aca6daf0f5471ff76448",
			wantCDigest:       "e55a76c08df5ab3cd9b5906b18ce1904e580f27712a760b4b965a3f979444978",
			wantRawDiff:       csharpExpectedDivergence("/compilation_unit", "shape", "children=5", "children=6"),
			wantRouteDiff:     csharpExpectedDivergence("/compilation_unit", "error", "false", "true"),
			wantCompactMode:   "fallback",
			wantCompactPass:   true,
			wantIncremental:   true,
			wantPassVisited:   2088,
			wantPassRewritten: 2080,
		},
		{
			name: "positive-simple", src: func(*testing.T) []byte {
				return []byte("class C { public int M() { return 1; } }\n")
			},
			wantSourceSHA:   "a6946abc5b7086a1ba0d6cd585882b3b8a20d96b6e578351cf07ecf993964362",
			wantRawDigest:   "58cdc6772314e0e82478a1d5811db6c8c09d939b5ac690ced9fb9695e0946ae7",
			wantGoDigest:    "58cdc6772314e0e82478a1d5811db6c8c09d939b5ac690ced9fb9695e0946ae7",
			wantCDigest:     "58cdc6772314e0e82478a1d5811db6c8c09d939b5ac690ced9fb9695e0946ae7",
			wantCompactMode: "accepted",
			wantForest:      true,
			wantForestPass:  true,
			wantIncremental: true,
			wantPassVisited: 22,
		},
		{
			// tree-sitter-c-sharp@9150f7d56bb4 removes this witness's last Go/C
			// divergence: every route now reproduces the C oracle digest
			// d9ca44d4… exactly, so wantRawDiff and wantRouteDiff are nil and
			// the Go digests equal the C digest.
			name: "historical-issue454", src: csharpDispatchIssue454Source,
			wantSourceSHA:   "a0de6cfb0e98995f41f1bac3931a4d0300ab8d34f68dd30843afecd9ee984711",
			wantRawDigest:   "d9ca44d4b6d5d7d555e5066a2c45fa329afb0fa237791746abe855fd31494ae4",
			wantGoDigest:    "d9ca44d4b6d5d7d555e5066a2c45fa329afb0fa237791746abe855fd31494ae4",
			wantCDigest:     "d9ca44d4b6d5d7d555e5066a2c45fa329afb0fa237791746abe855fd31494ae4",
			wantCompactMode: "fallback",
			wantCompactPass: true,
			wantIncremental: true,
			wantPassVisited: 57067,
			wantNative:      true,
		},
		{
			name: "malformed-missing-body", src: func(*testing.T) []byte {
				return []byte("class C { void M() { int x = ;\n")
			},
			wantSourceSHA:   "86a8c9f0a2ea38797add255cbbffcbe748af3c6f465d3db989b9dffd182d4ce8",
			wantRawDigest:   "83e10a3898a8af4e8f152e853e531013b18c0a1a2c801ef16bfd2fe1b9c7a4a5",
			wantGoDigest:    "83e10a3898a8af4e8f152e853e531013b18c0a1a2c801ef16bfd2fe1b9c7a4a5",
			wantCDigest:     "b252b21dc16f944cda8457956f65879a0222792efd936673448083a3b678aabc",
			wantRawDiff:     csharpExpectedDivergence("/compilation_unit/ERROR[0]", "extra", "false", "true"),
			wantRouteDiff:   csharpExpectedDivergence("/compilation_unit/ERROR[0]", "extra", "false", "true"),
			wantCompactMode: "fallback",
			wantCompactPass: true,
			wantIncremental: true,
			wantPassVisited: 19,
		},
	}

	language := grammars.CSharpLanguage()
	if language.ExternalScanner == nil {
		t.Fatal("C# language has no registered external scanner")
	}
	if reusable, ok := language.ExternalScanner.(gotreesitter.IncrementalReuseExternalScanner); ok && reusable.SupportsIncrementalReuse() {
		t.Fatal("C# external scanner unexpectedly supports incremental reuse")
	}
	cLanguage, err := COracleLanguage("c_sharp")
	if err != nil {
		t.Fatal(err)
	}
	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			source := witness.src(t)
			if len(source) == 0 || source[len(source)-1] != '\n' {
				source = append(source, '\n')
			}
			cTree := csharpDispatchCTree(t, cLanguage, source)
			defer cTree.Close()
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			sourceSHA := fmt.Sprintf("%x", sha256.Sum256(source))
			if sourceSHA != witness.wantSourceSHA {
				t.Fatalf("source SHA-256=%s, want %s", sourceSHA, witness.wantSourceSHA)
			}
			if cDigest != witness.wantCDigest {
				t.Fatalf("locked-C digest=%s, want %s", cDigest, witness.wantCDigest)
			}
			t.Logf("witness=%s bytes=%d source_sha256=%s c_digest=%s c_error=%t", witness.name, len(source), sourceSHA, cDigest, cTree.RootNode().HasError())

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatal(err)
			}
			defer raw.Release()
			rawDiff := csharpDispatchAssertReceipt(t, "raw", raw, language, cTree, cDigest)
			csharpDispatchCheckDigest(t, "raw", raw, language, witness.wantRawDigest)
			csharpDispatchCheckDivergence(t, "raw", rawDiff, witness.wantRawDiff)

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer production.Release()
			productionDiff := csharpDispatchAssertReceipt(t, "production", production, language, cTree, cDigest)
			productionPass := csharpDispatchPass(production)
			if productionPass == nil {
				t.Fatalf("production did not record dispatch.c_sharp")
			}
			csharpDispatchCheckDigest(t, "production", production, language, witness.wantGoDigest)
			csharpDispatchCheckDivergence(t, "production", productionDiff, witness.wantRouteDiff)
			csharpDispatchCheckPass(t, "production", productionPass, true, witness.wantPassVisited, witness.wantPassRewritten)
			if production.ParseRuntime().NativeRecoveredStructureAuthoritative != witness.wantNative {
				t.Fatalf("production native authority=%t, want %t", production.ParseRuntime().NativeRecoveredStructureAuthoritative, witness.wantNative)
			}

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer compact.Release()
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			compactMode := csharpDispatchCompactMode(routedBefore, fallbackBefore, routedAfter, fallbackAfter)
			compactDiff := csharpDispatchAssertReceipt(t, "compact", compact, language, cTree, cDigest)
			compactPass := csharpDispatchPass(compact)
			if compactMode != witness.wantCompactMode {
				t.Fatalf("compact mode=%s, want %s", compactMode, witness.wantCompactMode)
			}
			csharpDispatchCheckDigest(t, "compact", compact, language, witness.wantGoDigest)
			csharpDispatchCheckDivergence(t, "compact", compactDiff, witness.wantRouteDiff)
			csharpDispatchCheckPass(t, "compact", compactPass, witness.wantCompactPass, witness.wantPassVisited, witness.wantPassRewritten)

			forestParser := gotreesitter.NewParser(language)
			forest, forestOK := forestParser.ParseForestExperimental(source)
			forestMode := "declined"
			var forestDiff *DumpV1Divergence
			var forestPass *gotreesitter.NormalizationPassRuntime
			if forestOK && forest != nil {
				defer forest.Release()
				forestMode = "accepted"
				forestDiff = csharpDispatchAssertReceipt(t, "forest", forest, language, cTree, cDigest)
				forestPass = csharpDispatchPass(forest)
				csharpDispatchCheckDigest(t, "forest", forest, language, witness.wantGoDigest)
				csharpDispatchCheckDivergence(t, "forest", forestDiff, witness.wantRouteDiff)
				csharpDispatchCheckPass(t, "forest", forestPass, witness.wantForestPass, witness.wantPassVisited, witness.wantPassRewritten)
			} else if witness.wantForest {
				t.Fatal("forest route declined")
			}

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
				StartPoint:  csharpDispatchPoint(base),
				OldEndPoint: csharpDispatchPoint(base),
				NewEndPoint: csharpDispatchPoint(source),
			})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			defer incremental.Release()
			incrementalDiff := csharpDispatchAssertReceipt(t, "incremental", incremental, language, cTree, cDigest)
			incrementalPass := csharpDispatchPass(incremental)
			csharpDispatchCheckDigest(t, "incremental", incremental, language, witness.wantGoDigest)
			csharpDispatchCheckDivergence(t, "incremental", incrementalDiff, witness.wantRouteDiff)
			csharpDispatchCheckPass(t, "incremental", incrementalPass, witness.wantIncremental, witness.wantPassVisited, witness.wantPassRewritten)
			if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "external_scanner_unsupported" || profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
				t.Fatalf("incremental reuse changed: unsupported=%t reason=%q old_tree=%t subtrees=%d bytes=%d", profile.ReuseUnsupported, profile.ReuseUnsupportedReason, profile.OldTreeReuseRoute, profile.ReusedSubtrees, profile.ReusedBytes)
			}

			switch witness.name {
			case "a0-jsontextreader":
				if productionPass.NodesRewritten != 2080 || productionPass.NodesVisited != 2088 {
					t.Fatalf("A0 dispatch pass = visited:%d rewritten:%d, want 2088/2080", productionPass.NodesVisited, productionPass.NodesRewritten)
				}
				if rawDiff == nil || rawDiff.Category != "shape" || productionDiff == nil || productionDiff.Category != "error" || compactMode != "fallback" || forestOK || compactPass == nil || incrementalPass == nil || compactPass.NodesVisited != 2088 || compactPass.NodesRewritten != 2080 || incrementalPass.NodesVisited != 2088 || incrementalPass.NodesRewritten != 2080 {
					t.Fatalf("A0 evidence changed: raw=%+v production=%+v compact=%s forest=%t", rawDiff, productionDiff, compactMode, forestOK)
				}
			case "positive-simple":
				if productionPass.NodesRewritten != 0 || productionPass.NodesVisited != 22 {
					t.Fatalf("positive dispatch pass = visited:%d rewritten:%d, want 22/0", productionPass.NodesVisited, productionPass.NodesRewritten)
				}
				if rawDiff != nil || productionDiff != nil || compactDiff != nil || forestDiff != nil || incrementalDiff != nil || !forestOK || compactMode != "accepted" || (compactPass != nil && compactPass.NodesRewritten != 0) || (incrementalPass != nil && incrementalPass.NodesRewritten != 0) {
					t.Fatalf("positive control lost exact route parity: raw=%+v production=%+v compact=%+v forest=%+v incremental=%+v compact_mode=%s", rawDiff, productionDiff, compactDiff, forestDiff, incrementalDiff, compactMode)
				}
			case "historical-issue454":
				if productionPass.NodesRewritten != 0 || productionPass.NodesVisited != 57067 {
					t.Fatalf("issue-454 dispatch pass = visited:%d rewritten:%d, want 57067/0", productionPass.NodesVisited, productionPass.NodesRewritten)
				}
				// The 9150f7d56bb4 grammar reaches C-exact recovery on this
				// witness, so the route divergence that used to be required
				// here must now be absent.
				if production.ParseRuntime().NativeRecoveredStructureAuthoritative != true || productionDiff != nil || compactMode != "fallback" || forestOK {
					t.Fatalf("issue-454 evidence changed: native=%t production=%+v compact=%s forest=%t", production.ParseRuntime().NativeRecoveredStructureAuthoritative, productionDiff, compactMode, forestOK)
				}
			case "malformed-missing-body":
				if productionPass.NodesRewritten != 0 || productionPass.NodesVisited != 19 {
					t.Fatalf("malformed dispatch pass = visited:%d rewritten:%d, want 19/0", productionPass.NodesVisited, productionPass.NodesRewritten)
				}
				if productionDiff == nil || productionDiff.Category != "extra" || compactMode != "fallback" || forestOK {
					t.Fatalf("malformed evidence changed: production=%+v compact=%s forest=%t", productionDiff, compactMode, forestOK)
				}
			}
			forestDispatch := csharpDispatchPassSummary(forestPass)
			t.Logf("route-summary compact=%s forest=%s incremental_reuse=%t raw=%+v production=%+v compact_diff=%+v forest_diff=%+v incremental_diff=%+v production_dispatch=%s compact_dispatch=%s forest_dispatch=%s incremental_dispatch=%s", compactMode, forestMode, profile.OldTreeReuseRoute, rawDiff, productionDiff, compactDiff, forestDiff, incrementalDiff, csharpDispatchPassSummary(productionPass), csharpDispatchPassSummary(compactPass), forestDispatch, csharpDispatchPassSummary(incrementalPass))
		})
	}
}

func TestCSharpDispatchBlockerReceiptDocument(t *testing.T) {
	doc, err := os.ReadFile("../docs/root-normalization-retirement.md")
	if err != nil {
		t.Fatal(err)
	}
	changelog, err := os.ReadFile("../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	docMarkers := []string{
		"## 2026-08-24 C# dispatcher blocker receipt",
		"Status: `NO-GO`. Keep `dispatch.c_sharp` live.",
		"Base commit: `ef57c9d1b73bac046ef40f2a111bb76db643ebfd`.",
		"88366631d598ce6595ec655ce1591b315cffb14c",
		"The A0 (initial dispatcher census) manifest has 14",
		"d76fd62cfc90076c11d86cb7d7a0058df181231aa3b34f30e549f650b5294d4a",
		"6e5eb91f5577569ca2adebf26056095af492a5921ed09c72b05cba045dca57dc",
		"17a882ecc47150a396236512827eb2dd077ff2d65d9923d79d4ba98cb0b66abf",
		"a6946abc5b7086a1ba0d6cd585882b3b8a20d96b6e578351cf07ecf993964362",
		"58cdc6772314e0e82478a1d5811db6c8c09d939b5ac690ced9fb9695e0946ae7",
		"a0de6cfb0e98995f41f1bac3931a4d0300ab8d34f68dd30843afecd9ee984711",
		"4e6e7e9f33ca204763aff7a4d3e8ab4aee089ad057a9515cbc37a7c9a35f49aa",
		"d9ca44d4b6d5d7d555e5066a2c45fa329afb0fa237791746abe855fd31494ae4",
		"86a8c9f0a2ea38797add255cbbffcbe748af3c6f465d3db989b9dffd182d4ce8",
		"5140aac5a98ce1a8fa774400df978fa57b87ffcbe1d66fb159a82fe0de6553e2",
		"b252b21dc16f944cda8457956f65879a0222792efd936673448083a3b678aabc",
		"external_scanner_unsupported",
		"2,085 dispatcher rewrites.",
		"full authenticated corpus is unavailable",
		"20260823T030038Z-csharp-audit-route-final-v2",
		"## 2026-08-24 C# current-main follow-up",
		"83e0cfbc30ad82e2f327d58e35eea9f438a0ffda",
		"b97171a6ac31b12a268b6879f5a6bb89e7d1f268177144f6463f3d24cff5279b",
	}
	for _, marker := range docMarkers {
		if !strings.Contains(string(doc), marker) {
			t.Fatalf("retirement document lacks marker %q", marker)
		}
	}
	for _, marker := range []string{
		"Record the `dispatch.c_sharp` blocker at main commit",
		"ef57c9d1b73bac046ef40f2a111bb76db643ebfd",
		"Keep the arm live.",
		"Rechecked the `dispatch.c_sharp` blocker at current main commit",
	} {
		if !strings.Contains(string(changelog), marker) {
			t.Fatalf("changelog lacks marker %q", marker)
		}
	}
}

func csharpExpectedDivergence(path, category, goValue, cValue string) *DumpV1Divergence {
	return &DumpV1Divergence{Path: path, Category: category, GoValue: goValue, CValue: cValue}
}

func csharpDispatchCTree(t *testing.T, language *sitter.Language, source []byte) *sitter.Tree {
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

func csharpDispatchAssertReceipt(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, cDigest string) *DumpV1Divergence {
	t.Helper()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	diff := FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode())
	t.Logf("route=%s error_root=%t digest=%s c_digest=%s exact=%t rewrites=%d native_authoritative=%t divergence=%+v", route, tree.RootNode().HasError(), inspection.SHA256, cDigest, diff == nil && inspection.SHA256 == cDigest, tree.ParseRuntime().NormalizationNodesRewritten, tree.ParseRuntime().NativeRecoveredStructureAuthoritative, diff)
	return diff
}

func csharpDispatchPass(tree *gotreesitter.Tree) *gotreesitter.NormalizationPassRuntime {
	runtime := tree.ParseRuntime()
	if runtime.NormalizationPasses == nil {
		return nil
	}
	for i := range *runtime.NormalizationPasses {
		pass := &(*runtime.NormalizationPasses)[i]
		if pass.Name == "dispatch.c_sharp" {
			return pass
		}
	}
	return nil
}

func csharpDispatchPassSummary(pass *gotreesitter.NormalizationPassRuntime) string {
	if pass == nil {
		return "not-recorded"
	}
	return fmt.Sprintf("%d/%d", pass.NodesVisited, pass.NodesRewritten)
}

func csharpDispatchCheckDigest(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language, want string) {
	t.Helper()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatalf("%s inspect: %v", route, err)
	}
	if inspection.SHA256 != want {
		t.Fatalf("%s digest=%s, want %s", route, inspection.SHA256, want)
	}
}

func csharpDispatchCheckDivergence(t *testing.T, route string, got, want *DumpV1Divergence) {
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

func csharpDispatchCheckPass(t *testing.T, route string, got *gotreesitter.NormalizationPassRuntime, recorded bool, wantVisited, wantRewritten uint64) {
	t.Helper()
	if !recorded {
		if got != nil {
			t.Fatalf("%s recorded dispatch pass=%+v, want none", route, got)
		}
		return
	}
	if got == nil {
		t.Fatalf("%s did not record dispatch.c_sharp", route)
	}
	if got.NodesVisited != wantVisited || got.NodesRewritten != wantRewritten {
		t.Fatalf("%s dispatch pass=%d/%d, want %d/%d", route, got.NodesVisited, got.NodesRewritten, wantVisited, wantRewritten)
	}
}

func csharpDispatchCompactMode(routedBefore, fallbackBefore, routedAfter, fallbackAfter uint64) string {
	if routedAfter == routedBefore+1 && fallbackAfter == fallbackBefore {
		return "accepted"
	}
	if routedAfter == routedBefore && fallbackAfter == fallbackBefore+1 {
		return "fallback"
	}
	return fmt.Sprintf("counters=%d/%d->%d/%d", routedBefore, fallbackBefore, routedAfter, fallbackAfter)
}

func csharpDispatchPoint(source []byte) gotreesitter.Point {
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

func csharpDispatchIssue454Source(t *testing.T) []byte {
	t.Helper()
	var source strings.Builder
	source.Grow(137*1024 + 256)
	source.WriteString("namespace Bench {\n")
	for i := 0; source.Len() < 137*1024; i++ {
		fmt.Fprintf(&source, "public static class C%d { public static int F%d() { var x%d = %d; return x%d; } }\n", i, i, i, i, i)
	}
	source.WriteString("}\n")
	result := []byte(source.String())
	site := bytes.Index(result, []byte("x0"))
	if site < 0 {
		t.Fatal("C# edit marker is absent")
	}
	return append(append([]byte(nil), result[:site]...), result[site+1:]...)
}
