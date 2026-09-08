//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type wolframNextWitness struct {
	name, file                                                                                                string
	source                                                                                                    []byte
	sourceSHA256                                                                                              string
	wantError, wantForest                                                                                     bool
	wantC, wantRaw, wantProduction, wantCompactDigest, wantForestDigest, wantIncremental                      string
	wantRawDiff, wantProductionDiff, wantCompactDiff, wantForestDiff, wantIncrementalDiff                     *DumpV1Divergence
	wantCompact                                                                                               string
	wantRawDispatch, wantProductionDispatch, wantCompactDispatch, wantForestDispatch, wantIncrementalDispatch string
	wantCompactRoutedDelta, wantCompactFallbackDelta                                                          uint64
	wantCompactFallback                                                                                       string
	wantReuseUnsupported                                                                                      string
}

const (
	wolframNextGrammarLockSHA256     = "9ddb6324afd014f6ecdd1cae3dd1ba238f1e62ce03d126e6d8b267ce34d72ecb"
	wolframNextGrammarBlobSHA256     = "049223fe9382f88405b2758c21811af85cb0a7d771de71970817198ff703c169"
	wolframNextGrammarRepo           = "https://github.com/bostick/tree-sitter-wolfram"
	wolframNextGrammarCommit         = "63ebdac6f040d9082d3d8fa88be96ce24549adc5"
	wolframNextCArtifactSHA256       = "3dce4fc1569d56ec22a3f4beee18d1268d643916d635281764743275ce8bc463"
	wolframNextA0ManifestSHA256      = "999117555688ff15089c02f926a077e809ab4bd221daf622e3b2ba118d3adbba"
	wolframNextTrackedManifestSHA256 = "be584a0a4a26f0ca5268a7845cf3f04247e6b57259b9c7057e8eb2c9af26f839"
	wolframNextCorpusLockSHA256      = "41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea"
	wolframNextRecoveryFallback      = "fallback:compact route declined at recovery [mechanism=recovery-entered]: did not accept EOF: generic scheduler has no table action for the elected token"
	wolframNextScannerUnsupported    = "external_scanner_unsupported"
)

// TestWolframNextLiveArmLockedCRoutes records all routes before retirement.
func TestWolframNextLiveArmLockedCRoutes(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	wolframNextCheckStaticEvidence(t)
	goLanguage := grammars.WolframLanguage()
	if goLanguage == nil {
		t.Fatal("Wolfram language is unavailable")
	}
	cLanguage, err := COracleLanguage("wolfram")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := COracleIdentity("wolfram")
	if err != nil {
		t.Fatal(err)
	}
	if identity.Contract != COracleContractVersion || identity.Transport != "cgo_parity_binding" || identity.BindingModule != COracleBindingModule || identity.BindingVersion != COracleBindingVersion || identity.BindingCommit != COracleBindingCommit || identity.RuntimeVersion != COracleRuntimeVersion || identity.RuntimeCommit != COracleRuntimeCommit || identity.RuntimeLinkage != "static_cgo_test_binary" || identity.Language != "wolfram" || identity.GrammarRepo != wolframNextGrammarRepo || identity.GrammarCommit != wolframNextGrammarCommit || identity.GrammarLinkage != "shared_dlopen" || identity.GrammarCompileFlags != COracleGrammarCFlags || identity.CompilerPath != "/usr/bin/cc" || identity.CompilerVersion != "cc (Debian 12.2.0-14+deb12u1) 12.2.0" || identity.GrammarArtifactPath == "" || identity.GrammarArtifactSHA256 != wolframNextCArtifactSHA256 {
		t.Fatalf("locked-C identity is incomplete or changed: %+v", identity)
	}
	if goLanguage.ExternalScanner == nil {
		t.Fatal("Wolfram external scanner is unavailable")
	}
	if _, ok := goLanguage.ExternalScanner.(gotreesitter.IncrementalReuseExternalScanner); ok {
		t.Fatal("Wolfram external scanner unexpectedly advertises incremental reuse")
	}
	t.Logf("identity contract=%s transport=%s binding=%s@%s/%s runtime=%s/%s scanner_present=true scanner_incremental_reuse_api=false grammar=%s@%s artifact_sha256=%s", identity.Contract, identity.Transport, identity.BindingVersion, identity.BindingModule, identity.BindingCommit, identity.RuntimeVersion, identity.RuntimeCommit, identity.GrammarRepo, identity.GrammarCommit, identity.GrammarArtifactSHA256)
	rootDiff := &DumpV1Divergence{Path: "/source_file", Category: "type", GoValue: "source_file", CValue: "ERROR"}
	rootShape := &DumpV1Divergence{Path: "/source_file", Category: "shape", GoValue: "children=5", CValue: "children=4"}
	malformedDiff := &DumpV1Divergence{Path: "/source_file", Category: "shape", GoValue: "children=2", CValue: "children=1"}
	witnesses := []wolframNextWitness{
		{name: "a0-large-EvaluationUtilities", file: "large__EvaluationUtilities.wl", sourceSHA256: "e03c8588214ce3a0a5ba48d1f1335276c1826356052c33df1f3184a6d6303a53", wantError: true, wantC: "d5ed73a998ea3abb1778b3882b31824db86b1b18428e00176b3cd8cd72e685e1", wantRaw: "fe5f88dd4b103ced493354d2bf9161964eb5d59333a633178f2629b5cf293af1", wantProduction: "fe5f88dd4b103ced493354d2bf9161964eb5d59333a633178f2629b5cf293af1", wantCompactDigest: "fe5f88dd4b103ced493354d2bf9161964eb5d59333a633178f2629b5cf293af1", wantIncremental: "fe5f88dd4b103ced493354d2bf9161964eb5d59333a633178f2629b5cf293af1", wantRawDiff: rootDiff, wantProductionDiff: rootDiff, wantCompactDiff: rootDiff, wantIncrementalDiff: rootDiff, wantCompact: wolframNextRecoveryFallback, wantCompactFallback: wolframNextRecoveryFallback, wantCompactFallbackDelta: 1, wantReuseUnsupported: wolframNextScannerUnsupported},
		{name: "a0-medium-OutputHandlingUtilities", file: "medium__OutputHandlingUtilities.wl", sourceSHA256: "45a6287c3c8ad5f4f37298d4915d1bfb29e6e91ee0eccde1c842efb7c90e3dec", wantError: true, wantC: "5cc3a3615d9f5e1113e43ebc10ede08cefefc293bce9fb6306621cd0c1b106c1", wantRaw: "756d9dea72eca24759de158fa88d5779c1b8cf02d6b908327468ff9b3e443d56", wantProduction: "756d9dea72eca24759de158fa88d5779c1b8cf02d6b908327468ff9b3e443d56", wantCompactDigest: "756d9dea72eca24759de158fa88d5779c1b8cf02d6b908327468ff9b3e443d56", wantIncremental: "756d9dea72eca24759de158fa88d5779c1b8cf02d6b908327468ff9b3e443d56", wantRawDiff: rootDiff, wantProductionDiff: rootDiff, wantCompactDiff: rootDiff, wantIncrementalDiff: rootDiff, wantCompact: wolframNextRecoveryFallback, wantCompactFallback: wolframNextRecoveryFallback, wantCompactFallbackDelta: 1, wantReuseUnsupported: wolframNextScannerUnsupported},
		{name: "a0-small-PacletInfo", file: "small__PacletInfo.m", sourceSHA256: "55be9b6143e5dd68ddb433bb9c95c0388a505b65c452fb6036e064d537e3f602", wantError: true, wantC: "8e9966945e16f3a6fa173cc41ed6a171821f4a196bb4198057786ba4edbd464d", wantRaw: "a800797037893e3541b1265a179b09ab8795c26c9b174bbee7fe23d9c0814b6d", wantProduction: "a800797037893e3541b1265a179b09ab8795c26c9b174bbee7fe23d9c0814b6d", wantCompactDigest: "a800797037893e3541b1265a179b09ab8795c26c9b174bbee7fe23d9c0814b6d", wantIncremental: "a800797037893e3541b1265a179b09ab8795c26c9b174bbee7fe23d9c0814b6d", wantRawDiff: rootShape, wantProductionDiff: rootShape, wantCompactDiff: rootShape, wantIncrementalDiff: rootShape, wantCompact: wolframNextRecoveryFallback, wantCompactFallback: wolframNextRecoveryFallback, wantCompactFallbackDelta: 1, wantReuseUnsupported: wolframNextScannerUnsupported},
		{name: "split-infix", source: []byte("a + b\n"), sourceSHA256: "2ff5f5b73b79baf991a11cf00ca251c8c436a9e4c768b907cd6c589211a87dec", wantC: "f55efd4d7590d69c7a0cf938c4276db5a91c67fe864951e58fd6e222bb9dc3e8", wantRaw: "f55efd4d7590d69c7a0cf938c4276db5a91c67fe864951e58fd6e222bb9dc3e8", wantProduction: "f55efd4d7590d69c7a0cf938c4276db5a91c67fe864951e58fd6e222bb9dc3e8", wantCompactDigest: "f55efd4d7590d69c7a0cf938c4276db5a91c67fe864951e58fd6e222bb9dc3e8", wantForest: true, wantForestDigest: "f55efd4d7590d69c7a0cf938c4276db5a91c67fe864951e58fd6e222bb9dc3e8", wantIncremental: "f55efd4d7590d69c7a0cf938c4276db5a91c67fe864951e58fd6e222bb9dc3e8", wantCompact: "accepted", wantCompactRoutedDelta: 1, wantReuseUnsupported: wolframNextScannerUnsupported},
		{name: "plain-symbol", source: []byte("a\n"), sourceSHA256: "87428fc522803d31065e7bce3cf03fe475096631e5e07bbd7a0fde60c4cf25c7", wantC: "d417ec327539473cae7cc3baffce13f6391e31563ee60835f2f289baf5c1dcba", wantRaw: "d417ec327539473cae7cc3baffce13f6391e31563ee60835f2f289baf5c1dcba", wantProduction: "d417ec327539473cae7cc3baffce13f6391e31563ee60835f2f289baf5c1dcba", wantCompactDigest: "d417ec327539473cae7cc3baffce13f6391e31563ee60835f2f289baf5c1dcba", wantForest: true, wantForestDigest: "d417ec327539473cae7cc3baffce13f6391e31563ee60835f2f289baf5c1dcba", wantIncremental: "d417ec327539473cae7cc3baffce13f6391e31563ee60835f2f289baf5c1dcba", wantCompact: "accepted", wantCompactRoutedDelta: 1, wantReuseUnsupported: wolframNextScannerUnsupported},
		{name: "malformed", source: []byte("a +\n"), sourceSHA256: "0743fffb28bab9c8a803d05f63061f6e9578b01cdcac165608c71392395fd5af", wantError: true, wantC: "0239eda45e2781be67dce8d9a1cf61168e8b41e46c67f5d2be17795830b9c99a", wantRaw: "647d0ecc7542b5eeab97d968c582b031258772a0d694f654a2662a88bdf96c3e", wantProduction: "647d0ecc7542b5eeab97d968c582b031258772a0d694f654a2662a88bdf96c3e", wantCompactDigest: "647d0ecc7542b5eeab97d968c582b031258772a0d694f654a2662a88bdf96c3e", wantIncremental: "647d0ecc7542b5eeab97d968c582b031258772a0d694f654a2662a88bdf96c3e", wantRawDiff: malformedDiff, wantProductionDiff: malformedDiff, wantCompactDiff: malformedDiff, wantIncrementalDiff: malformedDiff, wantCompact: wolframNextRecoveryFallback, wantCompactFallback: wolframNextRecoveryFallback, wantCompactFallbackDelta: 1, wantReuseUnsupported: wolframNextScannerUnsupported},
	}
	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			witness.wantRawDispatch = "none"
			dispatch := map[string]string{
				"a0-large-EvaluationUtilities":      "2/2/16/0",
				"a0-medium-OutputHandlingUtilities": "2/2/16/0",
				"a0-small-PacletInfo":               "2/2/45/0",
				"split-infix":                       "1/1/5/0",
				"plain-symbol":                      "1/1/2/0",
				"malformed":                         "2/2/5/0",
			}[witness.name]
			witness.wantProductionDispatch = dispatch
			witness.wantCompactDispatch = dispatch
			if witness.name == "split-infix" || witness.name == "plain-symbol" {
				witness.wantCompactDispatch = "none"
			}
			witness.wantForestDispatch = dispatch
			witness.wantIncrementalDispatch = dispatch
			if witness.source == nil {
				witness.source = wolframNextRead(t, filepath.Join("../testdata/dispatcher_census_a0/wolfram", witness.file))
			}
			wolframNextRequire(t, witness)
			t.Logf("witness=%s source_sha256=%s bytes=%d want_error=%t", witness.name, witness.sourceSHA256, len(witness.source), witness.wantError)
			t.Logf("witness=%s identity grammar=%s@%s artifact_sha256=%s runtime=%s@%s binding=%s@%s", witness.name, identity.GrammarRepo, identity.GrammarCommit, identity.GrammarArtifactSHA256, identity.RuntimeVersion, identity.RuntimeCommit, identity.BindingModule, identity.BindingVersion)
			if got := fmt.Sprintf("%x", sha256.Sum256(witness.source)); got != witness.sourceSHA256 {
				t.Fatalf("source SHA-256=%s, want %s", got, witness.sourceSHA256)
			}
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(witness.source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parser returned no tree")
			}
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			if cDigest != witness.wantC {
				t.Fatalf("locked C digest=%s, want %s", cDigest, witness.wantC)
			}
			t.Logf("witness=%s locked_c_digest=%s", witness.name, cDigest)
			raw := wolframNextParse(t, goLanguage, witness.source, func(p *gotreesitter.Parser, s []byte) (*gotreesitter.Tree, error) {
				return p.ParseNoResultCompatibilityBenchmarkOnly(s)
			})
			production := wolframNextParse(t, goLanguage, witness.source, func(p *gotreesitter.Parser, s []byte) (*gotreesitter.Tree, error) { return p.Parse(s) })
			wolframNextAssert(t, "raw", raw, goLanguage, cTree, cDigest, witness.wantError, witness.wantRaw, witness.wantRawDiff, witness.wantRawDispatch)
			wolframNextAssert(t, "production", production, goLanguage, cTree, cDigest, witness.wantError, witness.wantProduction, witness.wantProductionDiff, witness.wantProductionDispatch)
			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(goLanguage)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(witness.source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(compact.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			routedDelta, fallbackDelta := routedAfter-routedBefore, fallbackAfter-fallbackBefore
			if routedDelta != witness.wantCompactRoutedDelta || fallbackDelta != witness.wantCompactFallbackDelta || routedDelta+fallbackDelta != 1 {
				t.Fatalf("compact counter delta=%d/%d, want %d/%d", routedDelta, fallbackDelta, witness.wantCompactRoutedDelta, witness.wantCompactFallbackDelta)
			}
			compactRoute := "accepted"
			if fallbackDelta == 1 {
				compactRoute = "fallback:" + gotreesitter.AdmissionCandidateLastFallbackReason()
			}
			if compactRoute != witness.wantCompact {
				t.Fatalf("compact route=%q, want %q", compactRoute, witness.wantCompact)
			}
			compactReason := ""
			if fallbackDelta == 1 {
				compactReason = gotreesitter.AdmissionCandidateLastFallbackReason()
			}
			t.Logf("witness=%s route=compact routed_delta=%d fallback_delta=%d reason=%q expected=%q scanner_present=true scanner_incremental_reuse_api=false", witness.name, routedDelta, fallbackDelta, compactReason, witness.wantCompact)
			wolframNextAssert(t, "compact", compact, goLanguage, cTree, cDigest, witness.wantError, witness.wantCompactDigest, witness.wantCompactDiff, witness.wantCompactDispatch)
			forestParser := gotreesitter.NewParser(goLanguage)
			forestParser.SetAdmissionCandidateRoute(false)
			forest, forestOK := forestParser.ParseForestExperimental(witness.source)
			if forestOK != witness.wantForest || (forestOK && forest == nil) || (!forestOK && forest != nil) {
				t.Fatalf("forest accepted=%t, want %t", forestOK, witness.wantForest)
			}
			t.Logf("witness=%s route=forest accepted=%t scanner_present=true scanner_incremental_reuse_api=false", witness.name, forestOK)
			if forestOK {
				t.Cleanup(forest.Release)
				wolframNextAssert(t, "forest", forest, goLanguage, cTree, cDigest, witness.wantError, witness.wantForestDigest, witness.wantForestDiff, witness.wantForestDispatch)
			}
			base := bytes.TrimSuffix(witness.source, []byte{'\n'})
			incrementalParser := gotreesitter.NewParser(goLanguage)
			incrementalParser.SetAdmissionCandidateRoute(false)
			oldTree, err := incrementalParser.Parse(base)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(oldTree.Release)
			oldTree.Edit(gotreesitter.InputEdit{StartByte: uint32(len(base)), OldEndByte: uint32(len(base)), NewEndByte: uint32(len(witness.source)), StartPoint: wolframNextPoint(base), OldEndPoint: wolframNextPoint(base), NewEndPoint: wolframNextPoint(witness.source)})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(witness.source, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(incremental.Release)
			if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != witness.wantReuseUnsupported || profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
				t.Fatalf("incremental profile=%+v, want scanner unsupported with zero reuse", profile)
			}
			t.Logf("witness=%s route=incremental reuse=%t unsupported=%t reason=%q reused_subtrees=%d reused_bytes=%d scanner_present=true scanner_incremental_reuse_api=false", witness.name, profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason, profile.ReusedSubtrees, profile.ReusedBytes)
			wolframNextAssert(t, "incremental", incremental, goLanguage, cTree, cDigest, witness.wantError, witness.wantIncremental, witness.wantIncrementalDiff, witness.wantIncrementalDispatch)
		})
	}
}

func wolframNextCheckStaticEvidence(t *testing.T) {
	if got := wolframNextHash(t, "../testdata/dispatcher_census_a0_manifest_v1.json"); got != wolframNextA0ManifestSHA256 {
		t.Fatalf("A0 manifest SHA-256=%s, want %s", got, wolframNextA0ManifestSHA256)
	}
	if got := wolframNextHash(t, "../testdata/dispatcher_census_tracked_v1.json"); got != wolframNextTrackedManifestSHA256 {
		t.Fatalf("tracked manifest SHA-256=%s, want %s", got, wolframNextTrackedManifestSHA256)
	}
	if got := strings.TrimSpace(string(wolframNextRead(t, "perf_scan/corpus_sources.lock.sha256"))); got != wolframNextCorpusLockSHA256+"  corpus_sources.lock" {
		t.Fatalf("corpus lock sidecar=%q", got)
	}
	for _, path := range []string{"../corpus_sources.lock", "perf_scan/corpus_sources.lock", "corpus_real"} {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("authenticated corpus evidence unexpectedly exists at %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("check corpus evidence %s: %v", path, err)
		}
	}
	if got := wolframNextHash(t, "../grammars/languages.lock"); got != wolframNextGrammarLockSHA256 {
		t.Fatalf("grammar lock SHA-256=%s, want %s", got, wolframNextGrammarLockSHA256)
	}
	blob := grammars.BlobByName("wolfram")
	if len(blob) == 0 {
		t.Fatal("Wolfram grammar blob is empty")
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(blob)); got != wolframNextGrammarBlobSHA256 {
		t.Fatalf("embedded grammar blob SHA-256=%s, want %s", got, wolframNextGrammarBlobSHA256)
	}
	if got := wolframNextHash(t, "../grammars/grammar_blobs/wolfram.bin"); got != wolframNextGrammarBlobSHA256 {
		t.Fatalf("grammar blob file SHA-256=%s, want %s", got, wolframNextGrammarBlobSHA256)
	}
}

func wolframNextRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if len(data) == 0 {
		t.Fatalf("evidence file %s is empty", path)
	}
	return data
}
func wolframNextHash(t *testing.T, path string) string {
	return fmt.Sprintf("%x", sha256.Sum256(wolframNextRead(t, path)))
}
func wolframNextRequire(t *testing.T, w wolframNextWitness) {
	t.Helper()
	if len(w.source) == 0 || w.sourceSHA256 == "" || w.wantC == "" || w.wantRaw == "" || w.wantProduction == "" || w.wantCompactDigest == "" || w.wantIncremental == "" {
		t.Fatal("witness has empty source or digest evidence")
	}
	for _, digest := range []string{w.sourceSHA256, w.wantC, w.wantRaw, w.wantProduction, w.wantCompactDigest, w.wantForestDigest, w.wantIncremental} {
		if digest != "" && len(digest) != sha256.Size*2 {
			t.Fatalf("invalid digest evidence %q", digest)
		}
	}
	if w.wantCompact == "" || w.wantRawDispatch == "" || w.wantProductionDispatch == "" || w.wantCompactDispatch == "" || w.wantIncrementalDispatch == "" || w.wantReuseUnsupported == "" {
		t.Fatal("witness has empty route evidence")
	}
	if w.wantForest && (w.wantForestDigest == "" || w.wantForestDispatch == "") {
		t.Fatal("accepted forest witness has incomplete evidence")
	}
	if w.wantCompactRoutedDelta+w.wantCompactFallbackDelta != 1 {
		t.Fatal("compact witness must pin one route outcome")
	}
	if w.wantCompactFallbackDelta == 1 && w.wantCompactFallback == "" {
		t.Fatal("fallback witness has empty reason")
	}
}
func wolframNextParse(t *testing.T, language *gotreesitter.Language, source []byte, parse func(*gotreesitter.Parser, []byte) (*gotreesitter.Tree, error)) *gotreesitter.Tree {
	t.Helper()
	parser := gotreesitter.NewParser(language)
	parser.SetAdmissionCandidateRoute(false)
	tree, err := parse(parser, source)
	if err != nil {
		t.Fatal(err)
	}
	if tree == nil {
		t.Fatal("route returned no tree")
	}
	t.Cleanup(tree.Release)
	return tree
}
func wolframNextAssert(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, cDigest string, wantError bool, wantDigest string, wantDiff *DumpV1Divergence, wantDispatch string) {
	t.Helper()
	if language.ExternalScanner == nil {
		t.Fatalf("%s scanner identity is absent", route)
	}
	if _, ok := language.ExternalScanner.(gotreesitter.IncrementalReuseExternalScanner); ok {
		t.Fatalf("%s scanner unexpectedly advertises incremental reuse", route)
	}
	root := tree.RootNode()
	if root == nil || root.HasError() != wantError {
		t.Fatalf("%s root error=%t, want %t", route, root != nil && root.HasError(), wantError)
	}
	inspection, err := benchfixtures.InspectGoTree(root, language)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.SHA256 != wantDigest {
		t.Fatalf("%s digest=%s, want %s", route, inspection.SHA256, wantDigest)
	}
	diff := FirstDivergenceDumpV1(root, language, cTree.RootNode())
	if (diff == nil) != (wantDiff == nil) || (diff != nil && *diff != *wantDiff) {
		t.Fatalf("%s divergence=%+v, want %+v", route, diff, wantDiff)
	}
	if diff == nil && inspection.SHA256 != cDigest {
		t.Fatalf("%s exact digest=%s, C=%s", route, inspection.SHA256, cDigest)
	}
	if got := wolframNextDispatch(tree); got != wantDispatch {
		t.Fatalf("%s dispatch=%q, want %q", route, got, wantDispatch)
	}
	t.Logf("witness=%s route=%s error_root=%t digest=%s locked_c_digest=%s first_divergence=%+v dispatch=%s rewrites=%d scanner_present=true scanner_incremental_reuse_api=false", t.Name(), route, root.HasError(), inspection.SHA256, cDigest, diff, wantDispatch, tree.ParseRuntime().NormalizationNodesRewritten)
}
func wolframNextDispatch(tree *gotreesitter.Tree) string {
	if tree == nil || tree.ParseRuntime().NormalizationPasses == nil {
		return "none"
	}
	for _, pass := range *tree.ParseRuntime().NormalizationPasses {
		if pass.Name == "dispatch.wolfram" {
			return fmt.Sprintf("%d/%d/%d/%d", pass.Checked, pass.Run, pass.NodesVisited, pass.NodesRewritten)
		}
	}
	return "none"
}
func wolframNextPoint(source []byte) gotreesitter.Point {
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
