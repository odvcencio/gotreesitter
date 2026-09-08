//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const (
	templN31qBaseCommit             = "3c2a2106102769bab891047174dbcfec15045e74"
	templN31qGrammarRepo            = "https://github.com/vrischmann/tree-sitter-templ"
	templN31qGrammarCommit          = "1c6db04effbcd7773c826bded9783cbc3061bd55"
	templN31qGrammarLockSHA256      = "9ddb6324afd014f6ecdd1cae3dd1ba238f1e62ce03d126e6d8b267ce34d72ecb"
	templN31qBlobSHA256             = "78f20ce45f9a4df12c458aadfbe9a98c80572bb13e0e2d01ffc43060e8d04701"
	templN31qA0ManifestSHA256       = "999117555688ff15089c02f926a077e809ab4bd221daf622e3b2ba118d3adbba"
	templN31qCorpusSidecarSHA256    = "2b2209597d1701ccc813bd35d1685b5b13730e6ebd285e66485ce812e35877cf"
	templN31qCorpusLockSHA256       = "41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea"
	templN31qCArtifactSHA256        = "91e455a6392a736912a481f0322c67bf571896c067ad0c7fba4ce4e9a7038081"
	templN31qRecoveryFallback       = "compact route declined at recovery [mechanism=recovery-entered]: did not accept EOF: generic scheduler has no table action for the elected token"
	templN31qNoActionFallback       = "compact route declined at no_action [mechanism=scheduler-frontier-shape]: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor"
	templN31qIncrementalUnsupported = "external_scanner_unsupported"
)

type templN31qDispatch struct {
	present            bool
	checked, run       uint64
	visited, rewritten uint64
}

type templN31qWitness struct {
	name, path, source       string
	sourceSHA256             string
	wantError                bool
	wantCDigest              string
	wantRawDigest            string
	wantNormalizedDigest     string
	wantRawDiff              *DumpV1Divergence
	wantNormalizedDiff       *DumpV1Divergence
	wantCompactReason        string
	wantForest               bool
	wantForestReason         string
	wantDispatch             templN31qDispatch
	wantCompactRoutedDelta   uint64
	wantCompactFallbackDelta uint64
}

func TestTemplN31qLiveArmLockedCRoutes(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := grammars.TemplLanguage()
	if language == nil || language.Name != "templ" {
		t.Fatalf("language=%v, want templ", language)
	}
	if language.ExternalScanner == nil {
		t.Fatal("Templ external scanner is absent")
	}
	if got := fmt.Sprintf("%T", language.ExternalScanner); got != "grammars.TemplExternalScanner" {
		t.Fatalf("Templ scanner type=%s, want grammars.TemplExternalScanner", got)
	}
	if _, ok := language.ExternalScanner.(gotreesitter.IncrementalReuseExternalScanner); ok {
		t.Fatal("Templ external scanner unexpectedly advertises incremental reuse")
	}
	t.Logf("base=%s arm=dispatch.templ scanner_type=%T scanner_incremental_reuse=false included_ranges=not_applicable", templN31qBaseCommit, language.ExternalScanner)

	if got := templN31qHashFile(t, "../grammars/languages.lock"); got != templN31qGrammarLockSHA256 {
		t.Fatalf("grammar lock SHA-256=%s, want %s", got, templN31qGrammarLockSHA256)
	}
	blob := grammars.BlobByName("templ")
	if len(blob) == 0 {
		t.Fatal("embedded Templ grammar blob is empty")
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(blob)); got != templN31qBlobSHA256 {
		t.Fatalf("embedded Templ grammar blob SHA-256=%s, want %s", got, templN31qBlobSHA256)
	}
	if got := templN31qHashFile(t, "../grammars/grammar_blobs/templ.bin"); got != templN31qBlobSHA256 {
		t.Fatalf("Templ grammar blob file SHA-256=%s, want %s", got, templN31qBlobSHA256)
	}
	if got := templN31qHashFile(t, "../testdata/dispatcher_census_a0_manifest_v1.json"); got != templN31qA0ManifestSHA256 {
		t.Fatalf("A0 manifest SHA-256=%s, want %s", got, templN31qA0ManifestSHA256)
	}
	if got := templN31qHashFile(t, "perf_scan/corpus_sources.lock.sha256"); got != templN31qCorpusSidecarSHA256 {
		t.Fatalf("corpus sidecar SHA-256=%s, want %s", got, templN31qCorpusSidecarSHA256)
	}
	sidecar, err := os.ReadFile("perf_scan/corpus_sources.lock.sha256")
	if err != nil {
		t.Fatal(err)
	}
	const wantSidecar = templN31qCorpusLockSHA256 + "  corpus_sources.lock\n"
	if string(sidecar) != wantSidecar {
		t.Fatalf("corpus sidecar=%q, want %q", string(sidecar), wantSidecar)
	}
	for _, absent := range []string{"../corpus_sources.lock", "perf_scan/corpus_sources.lock"} {
		if _, err := os.Stat(absent); !os.IsNotExist(err) {
			t.Fatalf("unauthenticated corpus lock %s is present or unreadable: %v", absent, err)
		}
	}

	cLanguage, err := COracleLanguage("templ")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := COracleIdentity("templ")
	if err != nil {
		t.Fatal(err)
	}
	if identity.Contract != COracleContractVersion ||
		identity.Transport != "cgo_parity_binding" ||
		identity.BindingModule != COracleBindingModule ||
		identity.BindingVersion != COracleBindingVersion ||
		identity.BindingCommit != COracleBindingCommit ||
		identity.RuntimeVersion != COracleRuntimeVersion ||
		identity.RuntimeCommit != COracleRuntimeCommit ||
		identity.RuntimeLinkage != "static_cgo_test_binary" ||
		identity.Language != "templ" ||
		identity.GrammarRepo != templN31qGrammarRepo ||
		identity.GrammarCommit != templN31qGrammarCommit ||
		identity.GrammarLinkage != "shared_dlopen" ||
		identity.GrammarCompileFlags != COracleGrammarCFlags ||
		identity.CompilerPath != "/usr/bin/cc" ||
		identity.CompilerVersion != "cc (Debian 12.2.0-14+deb12u1) 12.2.0" ||
		identity.GrammarArtifactPath == "" ||
		identity.GrammarArtifactSHA256 != templN31qCArtifactSHA256 {
		t.Fatalf("locked-C identity is incomplete or changed: %+v", identity)
	}
	t.Logf("locked-C identity contract=%s transport=%s binding=%s@%s/%s runtime=%s@%s grammar=%s@%s compiler=%s %s artifact=%s sha256=%s", identity.Contract, identity.Transport, identity.BindingModule, identity.BindingVersion, identity.BindingCommit, identity.RuntimeVersion, identity.RuntimeCommit, identity.GrammarRepo, identity.GrammarCommit, identity.CompilerPath, identity.CompilerVersion, identity.GrammarArtifactPath, identity.GrammarArtifactSHA256)

	witnesses := []templN31qWitness{
		{
			name: "a0-medium-main", path: "../testdata/dispatcher_census_a0/templ/medium__main.templ",
			sourceSHA256: "4415618a310cc880cb67fcd902bb7e9f82e91b9d0f461349e0cfb5cd0b1fa007", wantError: true,
			wantCDigest:   "efab90f3a4a75a4deba8c94d67c741dd842a7c8c6708bed3f59e37e0a994a11f",
			wantRawDigest: "895657a1c4978896653cf968b2dedddf7badd40f464d558e97dd95a9d9675595", wantNormalizedDigest: "895657a1c4978896653cf968b2dedddf7badd40f464d558e97dd95a9d9675595",
			wantRawDiff: templN31qDiff("/source_file", "error", "true", "false"), wantNormalizedDiff: templN31qDiff("/source_file", "error", "true", "false"),
			wantCompactReason: templN31qRecoveryFallback, wantForestReason: "dead_end", wantDispatch: templN31qDispatch{present: true, checked: 1, run: 1, visited: 317, rewritten: 0},
		},
		{
			name: "a0-medium-template", path: "../testdata/dispatcher_census_a0/templ/medium__template.templ",
			sourceSHA256:  "e4a5934ad709206e1c5ca82ab9bc86cd20467df61484357096e6378b5dbb7791",
			wantCDigest:   "7de9788750436a485bee98ec6200da09d5062700368333fe380562d71f171891",
			wantRawDigest: "33a54940b5da62255e5a03056b2ed7935994773b53a746b9a7e706b60a1a8dcb", wantNormalizedDigest: "2499953c81a152ca9db474f121b1a8a9de0c888c6f00a25125301c157bcb0b0e",
			wantRawDiff: templN31qDiff("/source_file/component_declaration[26]/component_block[3]", "shape", "children=20", "children=11"), wantNormalizedDiff: templN31qDiff("/source_file/component_declaration[26]/component_block[3]", "shape", "children=12", "children=11"),
			wantCompactReason: templN31qNoActionFallback, wantForest: true, wantDispatch: templN31qDispatch{present: true, checked: 1, run: 1, visited: 737, rewritten: 53},
		},
		{
			name: "a0-small-template", path: "../testdata/dispatcher_census_a0/templ/small__template.templ",
			sourceSHA256:  "bdc8798d13311d9f459108d3fad77f291dde4156fe68295671706684b8dd3eb3",
			wantCDigest:   "cb81fe10587416eae568216d16d2f7258bda32d00136030d8a4fcd2198e12594",
			wantRawDigest: "80e67baee0a78d252f4621c42b4eab3e1334bc919bdcba000d17034d04f954f3", wantNormalizedDigest: "cb81fe10587416eae568216d16d2f7258bda32d00136030d8a4fcd2198e12594",
			wantRawDiff:       templN31qDiff("/source_file/component_declaration[3]/component_block[3]/element[2]/element[1]", "shape", "children=4", "children=3"),
			wantCompactReason: templN31qNoActionFallback, wantForest: true, wantDispatch: templN31qDispatch{present: true, checked: 1, run: 1, visited: 84, rewritten: 23},
		},
		{
			name: "clean-component-import", source: "package p\n\n@templ.JSONScript(\"scriptData\", scriptData)\n", sourceSHA256: "54d6f6d873afb7b4155c87b08e3c70dedede40dfec36a0fc583d7129cbafc2e0",
			wantError: true, wantCDigest: "be90e19cd2f34f20303a530e3af710dae87e72d681d336aadcf7e5b605050cb6", wantRawDigest: "ba6bdf62e3dd580ad78d0bd2f6ec2b4aeef7cad28c8d4c05b9ee76107aa55301", wantNormalizedDigest: "ba6bdf62e3dd580ad78d0bd2f6ec2b4aeef7cad28c8d4c05b9ee76107aa55301",
			wantRawDiff: templN31qDiff("/source_file/ERROR[1]", "range", "2:0-3:0 @11..55", "2:0-2:43 @11..54"), wantNormalizedDiff: templN31qDiff("/source_file/ERROR[1]", "range", "2:0-3:0 @11..55", "2:0-2:43 @11..54"),
			wantCompactReason: templN31qRecoveryFallback, wantForestReason: "dead_end", wantDispatch: templN31qDispatch{present: true, checked: 1, run: 1, visited: 7, rewritten: 0},
		},
		{
			name: "malformed-dangling-quote", source: "package p\n\n<div title=\" >\n", sourceSHA256: "d5f89458e03f1f63c9b42ab2a7b76ddd3d4c7117b4b8c9b579374e631cd5cafc",
			wantError: true, wantCDigest: "6d468842ea01aeb519c3e7cf49e000862800c3b5324e4b2a6429616788c4cd42", wantRawDigest: "43b2a79e97c3f8f5d426e889610a2eef8c8256cc4b770333ef18088e9949c173", wantNormalizedDigest: "43b2a79e97c3f8f5d426e889610a2eef8c8256cc4b770333ef18088e9949c173",
			wantRawDiff: templN31qDiff("/source_file", "shape", "children=4", "children=2"), wantNormalizedDiff: templN31qDiff("/source_file", "shape", "children=4", "children=2"),
			wantCompactReason: templN31qRecoveryFallback, wantForestReason: "dead_end", wantDispatch: templN31qDispatch{present: true, checked: 1, run: 1, visited: 14, rewritten: 0},
		},
	}

	for _, witness := range witnesses {
		witness := witness
		witness.wantCompactFallbackDelta = 1
		t.Run(witness.name, func(t *testing.T) {
			source := []byte(witness.source)
			if witness.path != "" {
				var err error
				source, err = os.ReadFile(filepath.Clean(witness.path))
				if err != nil {
					t.Fatal(err)
				}
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != witness.sourceSHA256 {
				t.Fatalf("source SHA-256=%s, want %s", got, witness.sourceSHA256)
			}
			t.Logf("witness=%s bytes=%d source_sha256=%s", witness.name, len(source), witness.sourceSHA256)

			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parser returned no root")
			}
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			if cDigest != witness.wantCDigest {
				t.Fatalf("locked C digest=%s, want %s", cDigest, witness.wantCDigest)
			}

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(raw.Release)
			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			templN31qAssertRoute(t, "raw", raw, language, cTree.RootNode(), witness.wantRawDigest, witness.wantRawDiff, witness.wantError, templN31qDispatch{})
			templN31qAssertRoute(t, "production", production, language, cTree.RootNode(), witness.wantNormalizedDigest, witness.wantNormalizedDiff, witness.wantError, witness.wantDispatch)

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(compact.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			if routedAfter-routedBefore != witness.wantCompactRoutedDelta || fallbackAfter-fallbackBefore != witness.wantCompactFallbackDelta {
				t.Fatalf("compact counter delta=%d/%d, want %d/%d", routedAfter-routedBefore, fallbackAfter-fallbackBefore, witness.wantCompactRoutedDelta, witness.wantCompactFallbackDelta)
			}
			if got := gotreesitter.AdmissionCandidateLastFallbackReason(); got != witness.wantCompactReason {
				t.Fatalf("compact fallback reason=%q, want %q", got, witness.wantCompactReason)
			}
			t.Logf("compact counters=%d/%d->%d/%d reason=%q", routedBefore, fallbackBefore, routedAfter, fallbackAfter, witness.wantCompactReason)
			templN31qAssertRoute(t, "compact", compact, language, cTree.RootNode(), witness.wantNormalizedDigest, witness.wantNormalizedDiff, witness.wantError, witness.wantDispatch)

			forestParser := gotreesitter.NewParser(language)
			forest, forestOK := forestParser.ParseForestExperimental(source)
			forestAccepted := forestOK && forest != nil
			if forest != nil {
				t.Cleanup(forest.Release)
			}
			if forestAccepted != witness.wantForest {
				t.Fatalf("forest accepted=%t, want %t", forestAccepted, witness.wantForest)
			}
			_, _, forestReason, _ := forestParser.ForestDeclineInfo()
			if witness.wantForest {
				if forestReason != "" {
					t.Fatalf("accepted forest reason=%q, want empty", forestReason)
				}
				templN31qAssertRoute(t, "forest", forest, language, cTree.RootNode(), witness.wantNormalizedDigest, witness.wantNormalizedDiff, witness.wantError, witness.wantDispatch)
			} else if forestReason != witness.wantForestReason {
				t.Fatalf("forest decline reason=%q, want %q", forestReason, witness.wantForestReason)
			}
			t.Logf("forest accepted=%t reason=%q", forestAccepted, forestReason)

			incrementalParser := gotreesitter.NewParser(language)
			incrementalParser.SetAdmissionCandidateRoute(false)
			base := append(append([]byte(nil), source...), ' ')
			oldTree, err := incrementalParser.Parse(base)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(oldTree.Release)
			end := templN31qPointAtByte(source)
			oldTree.Edit(gotreesitter.InputEdit{StartByte: uint32(len(source)), OldEndByte: uint32(len(base)), NewEndByte: uint32(len(source)), StartPoint: end, OldEndPoint: templN31qPointAtByte(base), NewEndPoint: end})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(incremental.Release)
			if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != templN31qIncrementalUnsupported || profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
				t.Fatalf("incremental reuse profile=%+v", profile)
			}
			t.Logf("incremental edit=delete_trailing_space reuse_unsupported=%t reason=%q old_tree_route=%t reused_subtrees=%d reused_bytes=%d", profile.ReuseUnsupported, profile.ReuseUnsupportedReason, profile.OldTreeReuseRoute, profile.ReusedSubtrees, profile.ReusedBytes)
			templN31qAssertRoute(t, "incremental", incremental, language, cTree.RootNode(), witness.wantNormalizedDigest, witness.wantNormalizedDiff, witness.wantError, witness.wantDispatch)
		})
	}
}

func templN31qAssertRoute(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language, cRoot *sitter.Node, wantDigest string, wantDiff *DumpV1Divergence, wantError bool, wantDispatch templN31qDispatch) {
	t.Helper()
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("%s returned no root", route)
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatalf("inspect %s: %v", route, err)
	}
	if inspection.SHA256 != wantDigest {
		t.Fatalf("%s digest=%s, want %s", route, inspection.SHA256, wantDigest)
	}
	if got := tree.RootNode().HasError(); got != wantError {
		t.Fatalf("%s root error=%t, want %t", route, got, wantError)
	}
	if got := FirstDivergenceDumpV1(tree.RootNode(), language, cRoot); !templN31qSameDiff(got, wantDiff) {
		t.Fatalf("%s divergence=%+v, want %+v", route, got, wantDiff)
	}
	var pass *gotreesitter.NormalizationPassRuntime
	if tree.ParseRuntime().NormalizationPasses != nil {
		for i := range *tree.ParseRuntime().NormalizationPasses {
			candidate := &(*tree.ParseRuntime().NormalizationPasses)[i]
			if candidate.Name == "dispatch.templ" {
				pass = candidate
				break
			}
		}
	}
	if (pass != nil) != wantDispatch.present {
		t.Fatalf("%s dispatch.templ present=%t, want %t", route, pass != nil, wantDispatch.present)
	}
	if pass != nil && (pass.Checked != wantDispatch.checked || pass.Run != wantDispatch.run || pass.NodesVisited != wantDispatch.visited || pass.NodesRewritten != wantDispatch.rewritten) {
		t.Fatalf("%s dispatch.templ=%+v, want checked/run/visited/rewritten=%d/%d/%d/%d", route, pass, wantDispatch.checked, wantDispatch.run, wantDispatch.visited, wantDispatch.rewritten)
	}
	t.Logf("route=%s digest=%s c_comparison=receipt root_error=%t dispatch=%s/%d/%d/%d/%d", route, inspection.SHA256, wantError, route, wantDispatch.checked, wantDispatch.run, wantDispatch.visited, wantDispatch.rewritten)
}

func templN31qSameDiff(got, want *DumpV1Divergence) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return got.Path == want.Path && got.Category == want.Category && got.GoValue == want.GoValue && got.CValue == want.CValue
}

func templN31qDiff(path, category, goValue, cValue string) *DumpV1Divergence {
	return &DumpV1Divergence{Path: path, Category: category, GoValue: goValue, CValue: cValue}
}

func templN31qHashFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatalf("empty evidence file %s", path)
	}
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func templN31qPointAtByte(source []byte) gotreesitter.Point {
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
