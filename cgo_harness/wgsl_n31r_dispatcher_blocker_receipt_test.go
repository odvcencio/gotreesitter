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

	gots "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const (
	wgslN31rGrammarLockSHA256     = "9ddb6324afd014f6ecdd1cae3dd1ba238f1e62ce03d126e6d8b267ce34d72ecb"
	wgslN31rGrammarBlobSHA256     = "bed4620b51ac8e6dde6ea1ed0d14465f8b17ab11c2487a190650ef15abe392eb"
	wgslN31rA0ManifestSHA256      = "215df59aa56d28caa403f799733ef915db1c4ac07eb2bc96a9402f80cf67f80a"
	wgslN31rTrackedManifestSHA256 = "be584a0a4a26f0ca5268a7845cf3f04247e6b57259b9c7057e8eb2c9af26f839"
	wgslN31rCorpusLockSHA256      = "41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea"
	wgslN31rGrammarRepo           = "https://github.com/szebniok/tree-sitter-wgsl"
	wgslN31rGrammarCommit         = "40259f3c77ea856841a4e0c4c807705f3e4a2b65"
	wgslN31rCArtifactSHA256       = "67e5190b02afea88cfd9ced8866be93a6ab083922bdb73328c8d481d54907f0c"
	wgslN31rCompactFallback       = "compact route declined at recovery [mechanism=recovery-entered]: did not accept EOF: generic scheduler has no table action for the elected token"
)

type wgslN31rWitness struct {
	name, path                                                                                      string
	source                                                                                          []byte
	sourceSHA, cDigest, rawDigest, productionDigest, compactDigest, forestDigest, incrementalDigest string
	rawError, productionError, compactError, forestError, incrementalError                          bool
	rawDiff, productionDiff, compactDiff, forestDiff, incrementalDiff                               *DumpV1Divergence
	rawDispatch, productionDispatch, compactDispatch, forestDispatch, incrementalDispatch           string
	compactRouted, compactFallback                                                                  uint64
	compactReason                                                                                   string
	forestOK                                                                                        bool
	reusedSubtrees, reusedBytes                                                                     uint64
}

// TestWGSLN31rDispatcherBlockerReceipt records every required route for the
// WGSL compatibility arm. It does not read the retirement documentation.
func TestWGSLN31rDispatcherBlockerReceipt(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	for path, want := range map[string]string{
		"../grammars/languages.lock":                        wgslN31rGrammarLockSHA256,
		"../grammars/grammar_blobs/wgsl.bin":                wgslN31rGrammarBlobSHA256,
		"../testdata/dispatcher_census_a0_manifest_v1.json": wgslN31rA0ManifestSHA256,
		"../testdata/dispatcher_census_tracked_v1.json":     wgslN31rTrackedManifestSHA256,
	} {
		if got := wgslN31rHashFile(t, path); got != want {
			t.Fatalf("%s SHA-256=%s, want %s", path, got, want)
		}
	}
	if got := strings.TrimSpace(string(wgslN31rReadFile(t, "perf_scan/corpus_sources.lock.sha256"))); got != wgslN31rCorpusLockSHA256+"  corpus_sources.lock" {
		t.Fatalf("corpus lock sidecar=%q", got)
	}
	for _, path := range []string{"../corpus_sources.lock", "perf_scan/corpus_sources.lock", "corpus_real"} {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("authenticated corpus evidence unexpectedly exists at %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("check absent authenticated corpus evidence at %s: %v", path, err)
		}
	}
	goLanguage := grammars.WgslLanguage()
	if goLanguage == nil || goLanguage.Name != "wgsl" {
		t.Fatalf("WGSL language=%v, want wgsl", goLanguage)
	}
	if goLanguage.ExternalScanner == nil {
		t.Fatal("WGSL scanner is absent")
	}
	reusable, ok := goLanguage.ExternalScanner.(gots.IncrementalReuseExternalScanner)
	if !ok || !reusable.SupportsIncrementalReuse() {
		t.Fatalf("WGSL scanner does not certify incremental reuse: %T", goLanguage.ExternalScanner)
	}
	stateless, ok := goLanguage.ExternalScanner.(gots.StatelessExternalScanner)
	if !ok || !stateless.ExternalScannerIsStateless() {
		t.Fatalf("WGSL scanner does not certify stateless operation: %T", goLanguage.ExternalScanner)
	}
	preserving, ok := goLanguage.ExternalScanner.(gots.FailurePreservingExternalScanner)
	if !ok || !preserving.PreservesStateOnScanFailure() {
		t.Fatalf("WGSL scanner does not certify failure preservation: %T", goLanguage.ExternalScanner)
	}
	identity, err := COracleIdentity("wgsl")
	if err != nil {
		t.Fatal(err)
	}
	if identity.Contract != COracleContractVersion || identity.Transport != "cgo_parity_binding" || identity.BindingModule != COracleBindingModule || identity.BindingVersion != COracleBindingVersion || identity.BindingCommit != COracleBindingCommit || identity.RuntimeVersion != COracleRuntimeVersion || identity.RuntimeCommit != COracleRuntimeCommit || identity.RuntimeLinkage != "static_cgo_test_binary" || identity.Language != "wgsl" || identity.GrammarRepo != wgslN31rGrammarRepo || identity.GrammarCommit != wgslN31rGrammarCommit || identity.GrammarLinkage != "shared_dlopen" || identity.GrammarCompileFlags != COracleGrammarCFlags || identity.CompilerPath != "/usr/bin/cc" || identity.CompilerVersion != "cc (Debian 12.2.0-14+deb12u1) 12.2.0" || identity.GrammarArtifactPath == "" || identity.GrammarArtifactSHA256 != wgslN31rCArtifactSHA256 {
		t.Fatalf("locked-C identity is incomplete or changed: %+v", identity)
	}
	t.Logf("identity=%+v scanner=%T incremental_reuse=true stateless=true failure_preserving=true", identity, goLanguage.ExternalScanner)
	rootDiff := &DumpV1Divergence{Path: "/source_file", Category: "shape", GoValue: "children=70", CValue: "children=79"}
	radioDiff := &DumpV1Divergence{Path: "/source_file/global_variable_declaration[3]/variable_declaration[2]/variable_identifier_declaration[2]/type_declaration[2]/ERROR[1]", Category: "type", GoValue: "ERROR", CValue: "<"}
	radioIncrementalDiff := &DumpV1Divergence{Path: "/source_file", Category: "shape", GoValue: "children=46", CValue: "children=48"}
	malformedDiff := &DumpV1Divergence{Path: "/source_file", Category: "error", GoValue: "false", CValue: "true"}
	witnesses := []wgslN31rWitness{
		{name: "a0-normalMap", path: "../testdata/dispatcher_census_a0/wgsl/medium__normalMap.wgsl", sourceSHA: "999d93539ed738ab5041d75fe28e7b9d4da7e7ef25c345f2a1ef52239320268b", cDigest: "231e10ca2215945a5fb51670620c9f5ba2ea1ca7d445cb2c9443fb51b8e0e18a", rawDigest: "77fdfd002d6937e6f5784fc19e21a6f63ab8f2280ca8ba0dcfc1ee5b1d3d42cc", productionDigest: "9d802a0e9af71176c0520496ae99425406aa76dc8560f8c7e939e366f9fbbd44", compactDigest: "9d802a0e9af71176c0520496ae99425406aa76dc8560f8c7e939e366f9fbbd44", incrementalDigest: "fe0cb9f758eaace140619c81a9ef3347d89633580b18492e46dcfae6fb8f57c7", rawError: true, productionError: true, compactError: true, incrementalError: true, rawDiff: rootDiff, productionDiff: rootDiff, compactDiff: rootDiff, incrementalDiff: &DumpV1Divergence{Path: "/source_file", Category: "shape", GoValue: "children=69", CValue: "children=79"}, rawDispatch: "none", productionDispatch: "1/1/1289/120", compactDispatch: "1/1/1289/120", incrementalDispatch: "1/1/1287/684", compactRouted: 0, compactFallback: 1, compactReason: wgslN31rCompactFallback, forestOK: false, reusedSubtrees: 452, reusedBytes: 2318},
		{name: "a0-radiosity", path: "../testdata/dispatcher_census_a0/wgsl/medium__radiosity.wgsl", sourceSHA: "2d5630364c6667404abeb05ead49255f6538998ec66bf20cd5337a0eb5a26783", cDigest: "22b9d004c33c6a8229b56876282125e04efddf59deef6224eddd61f38c9952b2", rawDigest: "c591b9329ad2fc946b6b8b7c4bc80adb7305f41934dd51d4505e4f606787b127", productionDigest: "592abfad21a9a3170c11fd2e18888a9c5ecac7f681af5cf81e2d1c352873df63", compactDigest: "592abfad21a9a3170c11fd2e18888a9c5ecac7f681af5cf81e2d1c352873df63", incrementalDigest: "9e0113134b748e66ba22390bdf09e6e083a7f09dc0fc3b1dec67a407fed979cd", rawError: true, productionError: true, compactError: true, incrementalError: true, rawDiff: radioDiff, productionDiff: &DumpV1Divergence{Path: "/source_file/function_declaration[29]", Category: "error", GoValue: "false", CValue: "true"}, compactDiff: &DumpV1Divergence{Path: "/source_file/function_declaration[29]", Category: "error", GoValue: "false", CValue: "true"}, incrementalDiff: radioIncrementalDiff, rawDispatch: "none", productionDispatch: "1/1/1230/51", compactDispatch: "1/1/1230/51", incrementalDispatch: "1/1/1230/56", compactRouted: 0, compactFallback: 1, compactReason: wgslN31rCompactFallback, forestOK: false, reusedSubtrees: 185, reusedBytes: 1197},
		{name: "a0-fragmentTextureQuad", path: "../testdata/dispatcher_census_a0/wgsl/small__fragmentTextureQuad.wgsl", sourceSHA: "7af39dd8fd0e00f911fafaef4e2b40e2b0314f586049acef74c0fc451dd73286", cDigest: "d3e58954c750ed560edd3177a165bbf701c159467a1b4677996bec620c377804", rawDigest: "d3e58954c750ed560edd3177a165bbf701c1594677996bec620c377804", productionDigest: "d3e58954c750ed560edd3177a165bbf701c1594677996bec620c377804", compactDigest: "d3e58954c750ed560edd3177a165bbf701c1594677996bec620c377804", forestDigest: "d3e58954c750ed560edd3177a165bbf701c1594677996bec620c377804", incrementalDigest: "d3e58954c750ed560edd3177a165bbf701c1594677996bec620c377804", productionError: false, compactError: false, forestError: false, incrementalError: false, rawDispatch: "none", productionDispatch: "1/1/111/0", compactDispatch: "none", forestDispatch: "1/1/111/0", incrementalDispatch: "1/1/112/51", compactRouted: 1, compactFallback: 0, forestOK: true, reusedSubtrees: 49, reusedBytes: 192},
		{name: "clean-control", source: []byte("fn main() {}\n"), sourceSHA: "536e506bb90914c243a12b397b9a998f85ae2cbd9ba02dfd03a9e155ca5ca0f4", cDigest: "14614e2c6311dffe5c29ab57a2aa210e46d51280ed82d8886a46e61e2f9e5f23", rawDigest: "14614e2c6311dffe5c29ab57a2aa210e46d51280ed82d8886a46e61e2f9e5f23", productionDigest: "14614e2c6311dffe5c29ab57a2aa210e46d51280ed82d8886a46e61e2f9e5f23", compactDigest: "14614e2c6311dffe5c29ab57a2aa210e46d51280ed82d8886a46e61e2f9e5f23", forestDigest: "14614e2c6311dffe5c29ab57a2aa210e46d51280ed82d8886a46e61e2f9e5f23", incrementalDigest: "14614e2c6311dffe5c29ab57a2aa210e46d51280ed82d8886a46e61e2f9e5f23", productionError: false, compactError: false, forestError: false, incrementalError: false, rawDispatch: "none", productionDispatch: "1/1/9/0", compactDispatch: "none", forestDispatch: "1/1/9/0", incrementalDispatch: "1/1/9/0", compactRouted: 1, compactFallback: 0, forestOK: true, reusedSubtrees: 6, reusedBytes: 10},
		{name: "malformed-control", source: []byte("fn main() { let x: i32 = ; }\n"), sourceSHA: "fbc60f288807227888346e9f067f4753ce389799e612135e890db08598edb60e", cDigest: "95ceb782bbf18d9b5f3658c798aaefcd010bc694205abbf6ba1e793642db14d8", rawDigest: "95ceb782bbf18d9b5f3658c798aaefcd010bc694205abbf6ba1e793642db14d8", productionDigest: "34650fa98c42f84b32954caa9d371cc4e75f2a3d32da379cfda1de0736a27f11", compactDigest: "34650fa98c42f84b32954caa9d371cc4e75f2a3d32da379cfda1de0736a27f11", incrementalDigest: "34650fa98c42f84b32954caa9d371cc4e75f2a3d32da379cfda1de0736a27f11", rawError: true, productionError: false, compactError: false, incrementalError: false, rawDiff: nil, productionDiff: malformedDiff, compactDiff: malformedDiff, incrementalDiff: malformedDiff, rawDispatch: "none", productionDispatch: "1/1/19/5", compactDispatch: "1/1/19/5", incrementalDispatch: "1/1/19/5", compactRouted: 0, compactFallback: 1, compactReason: wgslN31rCompactFallback, forestOK: false, reusedSubtrees: 9, reusedBytes: 17},
	}
	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			if witness.name == "a0-fragmentTextureQuad" {
				const digest = "d3e58954c750ed560edd3177a165bbf701c159467a1b4677996bec620c377804"
				witness.rawDigest, witness.productionDigest, witness.compactDigest, witness.forestDigest, witness.incrementalDigest = digest, digest, digest, digest, digest
			}
			if witness.source == nil {
				var err error
				witness.source, err = os.ReadFile(filepath.Clean(witness.path))
				if err != nil {
					t.Fatal(err)
				}
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(witness.source)); got != witness.sourceSHA {
				t.Fatalf("source SHA-256=%s, want %s", got, witness.sourceSHA)
			}
			t.Logf("source_sha256=%s bytes=%d", witness.sourceSHA, len(witness.source))
			cLanguage, err := COracleLanguage("wgsl")
			if err != nil {
				t.Fatal(err)
			}
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(witness.source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C parser returned no tree")
			}
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			if cDigest != witness.cDigest {
				t.Fatalf("locked C digest=%s, want %s", cDigest, witness.cDigest)
			}
			t.Logf("locked_c_digest=%s c_error=%t", cDigest, cTree.RootNode().HasError())
			parse := func(route string, candidate bool, wantDigest string, wantError bool, wantDiff *DumpV1Divergence, wantDispatch string, fn func(*gots.Parser, []byte) (*gots.Tree, error)) *gots.Tree {
				p := gots.NewParser(goLanguage)
				p.SetAdmissionCandidateRoute(candidate)
				tree, err := fn(p, witness.source)
				if err != nil {
					t.Fatal(err)
				}
				wgslN31rAssertRoute(t, route, tree, goLanguage, cTree, wantDigest, wantError, wantDiff, wantDispatch)
				if tree == nil || tree.RootNode() == nil {
					t.Fatalf("%s returned no tree", route)
				}
				t.Cleanup(tree.Release)
				inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), goLanguage)
				if err != nil {
					t.Fatal(err)
				}
				diff := FirstDivergenceDumpV1(tree.RootNode(), goLanguage, cTree.RootNode())
				t.Logf("route=%s digest=%s error=%t divergence=%+v dispatch=%s rewrites=%d", route, inspection.SHA256, tree.RootNode().HasError(), diff, wgslN31rDispatch(tree, "dispatch.wgsl"), tree.ParseRuntime().NormalizationNodesRewritten)
				return tree
			}
			parse("raw", false, witness.rawDigest, witness.rawError, witness.rawDiff, witness.rawDispatch, func(p *gots.Parser, s []byte) (*gots.Tree, error) {
				return p.ParseNoResultCompatibilityBenchmarkOnly(s)
			})
			parse("production", false, witness.productionDigest, witness.productionError, witness.productionDiff, witness.productionDispatch, func(p *gots.Parser, s []byte) (*gots.Tree, error) { return p.Parse(s) })
			routedBefore, fallbackBefore := gots.AdmissionCandidateCounters()
			parse("compact", true, witness.compactDigest, witness.compactError, witness.compactDiff, witness.compactDispatch, func(p *gots.Parser, s []byte) (*gots.Tree, error) { return p.Parse(s) })
			routedAfter, fallbackAfter := gots.AdmissionCandidateCounters()
			if routedAfter-routedBefore != witness.compactRouted || fallbackAfter-fallbackBefore != witness.compactFallback {
				t.Fatalf("compact counter delta=%d/%d, want %d/%d", routedAfter-routedBefore, fallbackAfter-fallbackBefore, witness.compactRouted, witness.compactFallback)
			}
			if witness.compactReason != "" && gots.AdmissionCandidateLastFallbackReason() != witness.compactReason {
				t.Fatalf("compact fallback reason=%q, want %q", gots.AdmissionCandidateLastFallbackReason(), witness.compactReason)
			}
			t.Logf("compact routed_delta=%d fallback_delta=%d reason=%q", routedAfter-routedBefore, fallbackAfter-fallbackBefore, gots.AdmissionCandidateLastFallbackReason())
			forestParser := gots.NewParser(goLanguage)
			forestParser.SetAdmissionCandidateRoute(false)
			forest, forestOK := forestParser.ParseForestExperimental(witness.source)
			if forestOK != witness.forestOK {
				t.Fatalf("forest accepted=%t, want %t", forestOK, witness.forestOK)
			}
			t.Logf("forest accepted=%t", forestOK)
			if forestOK {
				t.Cleanup(forest.Release)
				inspection, err := benchfixtures.InspectGoTree(forest.RootNode(), goLanguage)
				if err != nil {
					t.Fatal(err)
				}
				if inspection.SHA256 != witness.forestDigest {
					t.Fatalf("forest digest=%s, want %s", inspection.SHA256, witness.forestDigest)
				}
				if forest.RootNode().HasError() != witness.forestError || FirstDivergenceDumpV1(forest.RootNode(), goLanguage, cTree.RootNode()) != nil {
					t.Fatalf("forest shape/error differs: error=%t divergence=%+v", forest.RootNode().HasError(), FirstDivergenceDumpV1(forest.RootNode(), goLanguage, cTree.RootNode()))
				}
				if got := wgslN31rDispatch(forest, "dispatch.wgsl"); got != witness.forestDispatch {
					t.Fatalf("forest dispatch=%q, want %q", got, witness.forestDispatch)
				}
				t.Logf("route=forest digest=%s divergence=%+v dispatch=%s rewrites=%d", inspection.SHA256, FirstDivergenceDumpV1(forest.RootNode(), goLanguage, cTree.RootNode()), wgslN31rDispatch(forest, "dispatch.wgsl"), forest.ParseRuntime().NormalizationNodesRewritten)
			}
			base := bytes.TrimSuffix(witness.source, []byte{'\n'})
			incParser := gots.NewParser(goLanguage)
			incParser.SetAdmissionCandidateRoute(false)
			oldTree, err := incParser.Parse(base)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(oldTree.Release)
			oldTree.Edit(gots.InputEdit{StartByte: uint32(len(base)), OldEndByte: uint32(len(base)), NewEndByte: uint32(len(witness.source)), StartPoint: wgslN31rPoint(base), OldEndPoint: wgslN31rPoint(base), NewEndPoint: wgslN31rPoint(witness.source)})
			inc, profile, err := incParser.ParseIncrementalProfiled(witness.source, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			wgslN31rAssertRoute(t, "incremental", inc, goLanguage, cTree, witness.incrementalDigest, witness.incrementalError, witness.incrementalDiff, witness.incrementalDispatch)
			if !profile.OldTreeReuseRoute || profile.ReuseUnsupported || profile.ReusedSubtrees != witness.reusedSubtrees || profile.ReusedBytes != witness.reusedBytes {
				t.Fatalf("incremental reuse profile=%+v, want old-tree reuse=%d subtrees/%d bytes", profile, witness.reusedSubtrees, witness.reusedBytes)
			}
			t.Cleanup(inc.Release)
			inspection, err := benchfixtures.InspectGoTree(inc.RootNode(), goLanguage)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("route=incremental digest=%s divergence=%+v dispatch=%s rewrites=%d profile=%+v", inspection.SHA256, FirstDivergenceDumpV1(inc.RootNode(), goLanguage, cTree.RootNode()), wgslN31rDispatch(inc, "dispatch.wgsl"), inc.ParseRuntime().NormalizationNodesRewritten, profile)
		})
	}
}

func wgslN31rAssertRoute(t *testing.T, route string, tree *gots.Tree, language *gots.Language, cTree *sitter.Tree, wantDigest string, wantError bool, wantDiff *DumpV1Divergence, wantDispatch string) {
	t.Helper()
	if tree.RootNode().HasError() != wantError {
		t.Fatalf("%s error=%t, want %t", route, tree.RootNode().HasError(), wantError)
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.SHA256 != wantDigest {
		t.Fatalf("%s digest=%s, want %s", route, inspection.SHA256, wantDigest)
	}
	diff := FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode())
	if (diff == nil) != (wantDiff == nil) || (diff != nil && *diff != *wantDiff) {
		t.Fatalf("%s divergence=%+v, want %+v", route, diff, wantDiff)
	}
	if got := wgslN31rDispatch(tree, "dispatch.wgsl"); got != wantDispatch {
		t.Fatalf("%s dispatch=%q, want %q", route, got, wantDispatch)
	}
	t.Logf("route=%s digest=%s error=%t divergence=%+v dispatch=%s rewrites=%d", route, inspection.SHA256, tree.RootNode().HasError(), diff, wantDispatch, tree.ParseRuntime().NormalizationNodesRewritten)
}

func wgslN31rReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if len(b) == 0 {
		t.Fatalf("evidence file %s is empty", path)
	}
	return b
}

func wgslN31rHashFile(t *testing.T, path string) string {
	return fmt.Sprintf("%x", sha256.Sum256(wgslN31rReadFile(t, path)))
}

func wgslN31rDispatch(tree *gots.Tree, name string) string {
	if tree == nil || tree.ParseRuntime().NormalizationPasses == nil {
		return "none"
	}
	for _, pass := range *tree.ParseRuntime().NormalizationPasses {
		if pass.Name == name {
			return fmt.Sprintf("%d/%d/%d/%d", pass.Checked, pass.Run, pass.NodesVisited, pass.NodesRewritten)
		}
	}
	return "none"
}

func wgslN31rPoint(source []byte) gots.Point {
	var point gots.Point
	for _, b := range source {
		if b == '\n' {
			point.Row++
			point.Column = 0
		} else {
			point.Column++
		}
	}
	return point
}
