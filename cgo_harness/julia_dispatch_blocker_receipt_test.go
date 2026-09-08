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

type juliaDispatchWitness struct {
	name   string
	source []byte
	want   juliaDispatchExpected
}

type juliaDispatchExpected struct {
	sourceSHA       string
	cDigest         string
	rawDigest       string
	goDigest        string
	rawDiff         *DumpV1Divergence
	routeDiff       *DumpV1Divergence
	forestDiff      *DumpV1Divergence
	production      string
	compact         string
	compactRouted   uint64
	compactFallback uint64
	compactReason   string
	forestAccepted  bool
	forestDigest    string
	forestDispatch  string
	incremental     string
	reuseSubtrees   uint64
	reuseBytes      uint64
	profileRetry    string
}

const (
	juliaGrammarLockSHA256     = "9ddb6324afd014f6ecdd1cae3dd1ba238f1e62ce03d126e6d8b267ce34d72ecb"
	juliaGrammarBlobSHA256     = "c716f2b9ee3852cc25a26107b7a1c78b9f76585fa77774d8f8a1b47ad590134f"
	juliaGrammarRepo           = "https://github.com/tree-sitter/tree-sitter-julia"
	juliaGrammarCommit         = "e0f9dcd180fdcfcfa8d79a3531e11d99e79321d3"
	juliaCArtifactSHA256       = "ac44385e88e2f5dc8c78dafef4eaf28f89bd2f922cc6a68e282b1b7b21f7eb8c"
	juliaA0ManifestSHA256      = "999117555688ff15089c02f926a077e809ab4bd221daf622e3b2ba118d3adbba"
	juliaTrackedManifestSHA256 = "be584a0a4a26f0ca5268a7845cf3f04247e6b57259b9c7057e8eb2c9af26f839"
	juliaCorpusLockSHA256      = "41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea"
)

func TestJuliaDispatchBlockerReceiptRoutes(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	goLanguage := grammars.JuliaLanguage()
	if goLanguage == nil {
		t.Fatal("Julia language is unavailable")
	}
	if goLanguage.Name != "julia" {
		t.Fatalf("Julia language name=%q, want julia", goLanguage.Name)
	}
	if goLanguage.ExternalScanner == nil {
		t.Fatal("Julia external scanner is unavailable")
	}
	if reusable, ok := goLanguage.ExternalScanner.(gotreesitter.IncrementalReuseExternalScanner); !ok || !reusable.SupportsIncrementalReuse() {
		t.Fatal("Julia external scanner does not advertise incremental reuse")
	}
	if stateless, ok := goLanguage.ExternalScanner.(gotreesitter.StatelessExternalScanner); !ok || !stateless.ExternalScannerIsStateless() {
		t.Fatal("Julia external scanner does not advertise stateless operation")
	}
	if preserving, ok := goLanguage.ExternalScanner.(gotreesitter.FailurePreservingExternalScanner); !ok || !preserving.PreservesStateOnScanFailure() {
		t.Fatal("Julia external scanner does not advertise failure preservation")
	}
	identity, err := COracleIdentity("julia")
	if err != nil {
		t.Fatal(err)
	}
	juliaDispatchCheckStaticEvidence(t, identity)
	cLanguage, err := COracleLanguage("julia")
	if err != nil {
		t.Fatal(err)
	}
	realSource, err := os.ReadFile("../testdata/compact_selected_lineage/julia_utils.jl")
	if err != nil {
		t.Fatal(err)
	}
	witnesses := []juliaDispatchWitness{
		{name: "recovered-return-range", source: []byte("function f()\n    return 1:(2 + 3)\nend\n"), want: juliaDispatchExpected{
			sourceSHA: "d58ebe735ae8ffdbeee398abe6d5a2f52276cf9e265cdd97a0dcbbfb005fdf3d", cDigest: "e3e63066738c5bec861d489d05ae4dfb391a61b1d9d4ef7790f33a2af53fe4a6", rawDigest: "e3e63066738c5bec861d489d05ae4dfb391a61b1d9d4ef7790f33a2af53fe4a6", goDigest: "67734bc3f92b669f6fbdff0ac12f61954d831afaa47efc85e0e75e6d9d321772", production: "1/1/23/12", compact: "fallback", compactFallback: 1, compactReason: "compact route error: parser-core phase zero: accepted compact root is incomplete or erroneous: span=0..38 expected=0..38 error=true allowErrorRoot=false", forestAccepted: true, forestDigest: "67734bc3f92b669f6fbdff0ac12f61954d831afaa47efc85e0e75e6d9d321772", forestDispatch: "1/1/23/12", incremental: "1/1/23/12", reuseSubtrees: 4, reuseBytes: 18,
			rawDiff: nil, routeDiff: &DumpV1Divergence{Path: "/source_file", Category: "error", GoValue: "true", CValue: "false"}, forestDiff: &DumpV1Divergence{Path: "/source_file", Category: "error", GoValue: "true", CValue: "false"},
		}},
		{name: "clean-program", source: []byte("module M\nexport f\nf(x) = x + 1\nend\n"), want: juliaDispatchExpected{
			sourceSHA: "1ed7b08ce391d766efbaba852ca75c1fc1e3dab68d381919bc6f411a8ec160cd", cDigest: "f71944da957be91e8f455b6bf50cbce30e87a2cee3fdf26c4eaccc5a32772d30", rawDigest: "f71944da957be91e8f455b6bf50cbce30e87a2cee3fdf26c4eaccc5a32772d30", goDigest: "f71944da957be91e8f455b6bf50cbce30e87a2cee3fdf26c4eaccc5a32772d30", production: "1/1/21/0", compact: "accepted", compactRouted: 1, forestAccepted: true, forestDigest: "f71944da957be91e8f455b6bf50cbce30e87a2cee3fdf26c4eaccc5a32772d30", forestDispatch: "1/1/21/0", incremental: "1/1/21/0", reuseSubtrees: 3, reuseBytes: 24,
		}},
		{name: "real-julia-utils", source: realSource, want: juliaDispatchExpected{
			sourceSHA: "d81017a2d640f6c84f2ca2a7030687049b7334bdb75ad5c50302b29052ecf79c", cDigest: "d26ec18b262399c0ddef69b993cc2e6b063a22008b6882068fa172b1c4ec99b2", rawDigest: "0557559a3ec5dc431793b04a92e7c83666a53da670cb33b0d1a19cb2d84cc6f3", goDigest: "0557559a3ec5dc431793b04a92e7c83666a53da670cb33b0d1a19cb2d84cc6f3", production: "1/1/343/0", compact: "fallback", compactFallback: 1, compactReason: "compact route declined at no_action [mechanism=scheduler-frontier-shape]: converged-path reduction split no-action drop descends from an unproved historical boundary resurrection", forestAccepted: true, forestDigest: "d26ec18b262399c0ddef69b993cc2e6b063a22008b6882068fa172b1c4ec99b2", forestDispatch: "1/1/342/0", incremental: "1/1/343/0", reuseSubtrees: 3, reuseBytes: 18, profileRetry: "incremental_parse_full_retry",
			rawDiff: &DumpV1Divergence{Path: "/source_file/function_definition[4]/block[2]/for_statement[4]/block[2]/while_statement[2]/block[2]/assignment[0]/parenthesized_expression[2]/binary_expression[1]/parenthesized_expression[0]/binary_expression[1]/binary_expression[0]/parenthesized_expression[2]/juxtaposition_expression[1]", Category: "type", GoValue: "juxtaposition_expression", CValue: "binary_expression"}, routeDiff: &DumpV1Divergence{Path: "/source_file/function_definition[4]/block[2]/for_statement[4]/block[2]/while_statement[2]/block[2]/assignment[0]/parenthesized_expression[2]/binary_expression[1]/parenthesized_expression[0]/binary_expression[1]/binary_expression[0]/parenthesized_expression[2]/juxtaposition_expression[1]", Category: "type", GoValue: "juxtaposition_expression", CValue: "binary_expression"}, forestDiff: nil,
		}},
	}
	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			source := witness.source
			if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != witness.want.sourceSHA {
				t.Fatalf("source SHA-256=%s, want %s", got, witness.want.sourceSHA)
			}
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parse returned no tree")
			}
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			if cDigest != witness.want.cDigest {
				t.Fatalf("locked-C digest=%s, want %s", cDigest, witness.want.cDigest)
			}

			rawParser := gotreesitter.NewParser(goLanguage)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(raw.Release)
			productionParser := gotreesitter.NewParser(goLanguage)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(goLanguage)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(compact.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			forestParser := gotreesitter.NewParser(goLanguage)
			forestParser.SetAdmissionCandidateRoute(false)
			forest, forestOK := forestParser.ParseForestExperimental(source)
			if forestOK && forest != nil {
				t.Cleanup(forest.Release)
			}

			base := bytes.TrimSuffix(source, []byte{'\n'})
			incrementalParser := gotreesitter.NewParser(goLanguage)
			incrementalParser.SetAdmissionCandidateRoute(false)
			oldTree, err := incrementalParser.Parse(base)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(oldTree.Release)
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(base)),
				OldEndByte:  uint32(len(base)),
				NewEndByte:  uint32(len(source)),
				StartPoint:  juliaPoint(base),
				OldEndPoint: juliaPoint(base),
				NewEndPoint: juliaPoint(source),
			})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(incremental.Release)

			for _, route := range []struct {
				name string
				tree *gotreesitter.Tree
			}{
				{name: "raw", tree: raw},
				{name: "production", tree: production},
				{name: "compact", tree: compact},
				{name: "incremental", tree: incremental},
			} {
				inspection, err := benchfixtures.InspectGoTree(route.tree.RootNode(), goLanguage)
				if err != nil {
					t.Fatal(err)
				}
				diff := FirstDivergenceDumpV1(route.tree.RootNode(), goLanguage, cTree.RootNode())
				wantDigest := witness.want.goDigest
				if route.name == "raw" {
					wantDigest = witness.want.rawDigest
				}
				if inspection.SHA256 != wantDigest {
					t.Fatalf("%s digest=%s, want %s", route.name, inspection.SHA256, wantDigest)
				}
				wantDiff := witness.want.routeDiff
				if route.name == "raw" {
					wantDiff = witness.want.rawDiff
				}
				juliaDispatchAssertDivergence(t, route.name, diff, wantDiff)
				if route.name == "raw" && juliaDispatchSummary(route.tree) != "none" {
					t.Fatalf("raw dispatch=%s, want none", juliaDispatchSummary(route.tree))
				}
				if route.name == "production" && juliaDispatchSummary(route.tree) != witness.want.production {
					t.Fatalf("production dispatch=%s, want %s", juliaDispatchSummary(route.tree), witness.want.production)
				}
				if route.name == "compact" {
					if witness.want.compact == "accepted" && juliaDispatchSummary(route.tree) != "none" {
						t.Fatalf("compact dispatch=%s, want none", juliaDispatchSummary(route.tree))
					}
					if witness.want.compact == "fallback" && juliaDispatchSummary(route.tree) != witness.want.production {
						t.Fatalf("compact dispatch=%s, want %s", juliaDispatchSummary(route.tree), witness.want.production)
					}
				}
				if route.name == "incremental" && juliaDispatchSummary(route.tree) != witness.want.incremental {
					t.Fatalf("incremental dispatch=%s, want %s", juliaDispatchSummary(route.tree), witness.want.incremental)
				}
				t.Logf("route=%s digest=%s error=%t divergence=%+v dispatch=%s rewrites=%d", route.name, inspection.SHA256, route.tree.RootNode().HasError(), diff, juliaDispatchSummary(route.tree), route.tree.ParseRuntime().NormalizationNodesRewritten)
			}
			if forestOK && forest != nil {
				inspection, err := benchfixtures.InspectGoTree(forest.RootNode(), goLanguage)
				if err != nil {
					t.Fatal(err)
				}
				if !witness.want.forestAccepted || inspection.SHA256 != witness.want.forestDigest {
					t.Fatalf("forest digest=%s, want %s", inspection.SHA256, witness.want.forestDigest)
				}
				juliaDispatchAssertDivergence(t, "forest", FirstDivergenceDumpV1(forest.RootNode(), goLanguage, cTree.RootNode()), witness.want.forestDiff)
				if juliaDispatchSummary(forest) != witness.want.forestDispatch {
					t.Fatalf("forest dispatch=%s, want %s", juliaDispatchSummary(forest), witness.want.forestDispatch)
				}
				t.Logf("route=forest accepted=true digest=%s error=%t divergence=%+v dispatch=%s rewrites=%d", inspection.SHA256, forest.RootNode().HasError(), FirstDivergenceDumpV1(forest.RootNode(), goLanguage, cTree.RootNode()), juliaDispatchSummary(forest), forest.ParseRuntime().NormalizationNodesRewritten)
			} else {
				if witness.want.forestAccepted {
					t.Fatal("forest route declined")
				}
				t.Logf("route=forest accepted=false")
			}
			if routedAfter-routedBefore != witness.want.compactRouted || fallbackAfter-fallbackBefore != witness.want.compactFallback {
				t.Fatalf("compact counters=%d/%d, want %d/%d", routedAfter-routedBefore, fallbackAfter-fallbackBefore, witness.want.compactRouted, witness.want.compactFallback)
			}
			if witness.want.compactFallback == 1 && gotreesitter.AdmissionCandidateLastFallbackReason() != witness.want.compactReason {
				t.Fatalf("compact reason=%q, want %q", gotreesitter.AdmissionCandidateLastFallbackReason(), witness.want.compactReason)
			}
			if profile.ReusedSubtrees != witness.want.reuseSubtrees || profile.ReusedBytes != witness.want.reuseBytes {
				t.Fatalf("incremental reuse=%d/%d, want %d/%d", profile.ReusedSubtrees, profile.ReusedBytes, witness.want.reuseSubtrees, witness.want.reuseBytes)
			}
			if witness.want.profileRetry != "" && profile.ReuseUnsupportedReason != witness.want.profileRetry {
				t.Fatalf("incremental reuse reason=%q, want %q", profile.ReuseUnsupportedReason, witness.want.profileRetry)
			}
			t.Logf("compact routed_delta=%d fallback_delta=%d reason=%q", routedAfter-routedBefore, fallbackAfter-fallbackBefore, gotreesitter.AdmissionCandidateLastFallbackReason())
			t.Logf("incremental profile reuse=%t unsupported=%t reason=%q reused_subtrees=%d reused_bytes=%d", profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason, profile.ReusedSubtrees, profile.ReusedBytes)
		})
	}
}

func TestJuliaDispatchBlockerReceiptDocument(t *testing.T) {
	doc, err := os.ReadFile("../docs/root-normalization-retirement.md")
	if err != nil {
		t.Fatal(err)
	}
	changelog, err := os.ReadFile("../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"## 2026-08-24 Julia dispatcher blocker receipt",
		"Status: `KEEP LIVE / NO-GO`. Keep `dispatch.julia` live.",
		"The recovered witness flips the root error flag from false to true.",
		"Keep `dispatch.julia` live.",
	} {
		if !strings.Contains(string(doc), marker) {
			t.Fatalf("retirement document lacks marker %q", marker)
		}
	}
	if !strings.Contains(string(changelog), "Recorded the unreceipted `dispatch.julia` blocker") {
		t.Fatal("changelog lacks Julia blocker entry")
	}
}

func juliaDispatchCheckStaticEvidence(t *testing.T, identity COracleBuildIdentity) {
	t.Helper()
	if identity.Contract != COracleContractVersion || identity.Transport != "cgo_parity_binding" || identity.BindingModule != COracleBindingModule || identity.BindingVersion != COracleBindingVersion || identity.BindingCommit != COracleBindingCommit || identity.RuntimeVersion != COracleRuntimeVersion || identity.RuntimeCommit != COracleRuntimeCommit || identity.RuntimeLinkage != "static_cgo_test_binary" || identity.Language != "julia" || identity.GrammarRepo != juliaGrammarRepo || identity.GrammarCommit != juliaGrammarCommit || identity.GrammarLinkage != "shared_dlopen" || identity.GrammarCompileFlags != COracleGrammarCFlags || identity.CompilerPath != "/usr/bin/cc" || identity.CompilerVersion != "cc (Debian 12.2.0-14+deb12u1) 12.2.0" || identity.GrammarArtifactSHA256 != juliaCArtifactSHA256 {
		t.Fatalf("locked-C identity changed: %+v", identity)
	}
	for path, want := range map[string]string{
		"../grammars/languages.lock":                        juliaGrammarLockSHA256,
		"../grammars/grammar_blobs/julia.bin":               juliaGrammarBlobSHA256,
		"../testdata/dispatcher_census_a0_manifest_v1.json": juliaA0ManifestSHA256,
		"../testdata/dispatcher_census_tracked_v1.json":     juliaTrackedManifestSHA256,
		"perf_scan/corpus_sources.lock.sha256":              "2b2209597d1701ccc813bd35d1685b5b13730e6ebd285e66485ce812e35877cf",
	} {
		got, err := fileSHA256(path)
		if err != nil {
			t.Fatalf("hash %s: %v", path, err)
		}
		if got != want {
			t.Fatalf("%s SHA-256=%s, want %s", path, got, want)
		}
	}
	sidecar, err := os.ReadFile("perf_scan/corpus_sources.lock.sha256")
	if err != nil || strings.TrimSpace(string(sidecar)) != juliaCorpusLockSHA256+"  corpus_sources.lock" {
		t.Fatalf("corpus sidecar=%q, want %q", sidecar, juliaCorpusLockSHA256+"  corpus_sources.lock")
	}
	for _, absent := range []string{"../corpus_sources.lock", "perf_scan/corpus_sources.lock", "corpus_real"} {
		if _, err := os.Stat(absent); err == nil {
			t.Fatalf("unexpected authenticated corpus path %s", absent)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", absent, err)
		}
	}
	if identity.GrammarArtifactPath == "" {
		t.Fatal("C artifact path is empty")
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(grammars.BlobByName("julia"))); got != juliaGrammarBlobSHA256 {
		t.Fatalf("embedded Julia blob SHA-256=%s, want %s", got, juliaGrammarBlobSHA256)
	}
}

func juliaDispatchAssertDivergence(t *testing.T, route string, got, want *DumpV1Divergence) {
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

func juliaPoint(source []byte) gotreesitter.Point {
	return gotreesitter.Point{Row: uint32(bytes.Count(source, []byte{'\n'})), Column: func() uint32 {
		if i := bytes.LastIndexByte(source, '\n'); i >= 0 {
			return uint32(len(source) - i - 1)
		}
		return uint32(len(source))
	}()}
}

func juliaDispatchSummary(tree *gotreesitter.Tree) string {
	if tree == nil || tree.ParseRuntime().NormalizationPasses == nil {
		return "none"
	}
	for _, pass := range *tree.ParseRuntime().NormalizationPasses {
		if pass.Name == "dispatch.julia" {
			return fmt.Sprintf("%d/%d/%d/%d", pass.Checked, pass.Run, pass.NodesVisited, pass.NodesRewritten)
		}
	}
	return "none"
}
