//go:build cgo && treesitter_c_parity

package cgoharness

import (
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

const (
	doxygenNextGrammarCommit   = "ccd998f378c3f9345ea4eeb223f56d7b84d16687"
	doxygenNextGrammarLockSHA  = "9ddb6324afd014f6ecdd1cae3dd1ba238f1e62ce03d126e6d8b267ce34d72ecb"
	doxygenNextCGrammarRepo    = "https://github.com/amaanq/tree-sitter-doxygen"
	doxygenNextCArtifactSHA256 = "1fe84dfe69da98a5860f2261fc8deb2cf250aa4ae07c2ecf3bace5dfe396d11e"
	doxygenNextCContract       = "tree-sitter-c-v1"
	doxygenNextCRuntimeVersion = "0.25.1"
	doxygenNextCRuntimeCommit  = "f5afe475deb7c0bae6407fb776c76824f717bb61"
)

type doxygenNextWitness struct {
	name, path, source                                         string
	sourceSHA, rawDigest, goDigest, cDigest                    string
	wantDivergence                                             *normalizationKnownDivergence
	wantDispatch, wantCompactDispatch, wantIncrementalDispatch bool
	wantCompactFallback                                        bool
	wantCompactRoutedDelta, wantCompactFallbackDelta           uint64
	wantForestReason                                           string
	wantDispatchRewrites                                       uint64
	wantDispatchChecked, wantDispatchRun, wantDispatchVisited  uint64
}

func TestDoxygenNextLiveArmProbe(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := grammars.DoxygenLanguage()
	if language == nil || language.Name != "doxygen" {
		t.Fatalf("Doxygen language=%v, want doxygen", language)
	}
	if language.ExternalScanner == nil {
		t.Fatal("Doxygen language has no external scanner")
	}
	if got := doxygenNextFileSHA(t, "../grammars/languages.lock"); got != doxygenNextGrammarLockSHA {
		t.Fatalf("grammar lock SHA-256=%s, want %s", got, doxygenNextGrammarLockSHA)
	}
	cLanguage, err := COracleLanguage("doxygen")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := COracleIdentity("doxygen")
	if err != nil {
		t.Fatal(err)
	}
	if identity.Contract != doxygenNextCContract || identity.RuntimeVersion != doxygenNextCRuntimeVersion || identity.RuntimeCommit != doxygenNextCRuntimeCommit {
		t.Fatalf("C identity contract=%s runtime=%s@%s, want %s runtime=%s@%s", identity.Contract, identity.RuntimeVersion, identity.RuntimeCommit, doxygenNextCContract, doxygenNextCRuntimeVersion, doxygenNextCRuntimeCommit)
	}
	if identity.GrammarRepo != doxygenNextCGrammarRepo || identity.GrammarCommit != doxygenNextGrammarCommit || identity.GrammarArtifactSHA256 != doxygenNextCArtifactSHA256 {
		t.Fatalf("C grammar identity=%+v, want repo=%s commit=%s artifact_sha256=%s", identity, doxygenNextCGrammarRepo, doxygenNextGrammarCommit, doxygenNextCArtifactSHA256)
	}
	t.Logf("grammar=doxygen grammar_lock_sha256=%s c_contract=%s c_runtime=%s@%s c_grammar=%s@%s c_artifact=%s c_artifact_sha256=%s", doxygenNextGrammarLockSHA, identity.Contract, identity.RuntimeVersion, identity.RuntimeCommit, identity.GrammarRepo, identity.GrammarCommit, identity.GrammarArtifactPath, identity.GrammarArtifactSHA256)

	witnesses := []doxygenNextWitness{
		{name: "a0_CMakeLists", path: "../testdata/dispatcher_census_a0/doxygen/medium__CMakeLists.txt", sourceSHA: "66408d6539b27d7c49b1e51777605c38c91b6d924267db5109ee00e2a1cfcf41", goDigest: "a206903ee351591886014cb963d527769fc710d513af7b84c6dba9d9cc77cd2b", cDigest: "d6f623d2b87344001e98de5528b44e38b102e564491871a9ffb64c1b73d193c5", wantDivergence: &normalizationKnownDivergence{Path: "/document", Category: "type", GoValue: "document", CValue: "ERROR", Reason: "the C oracle keeps the whole A0 source under an ERROR root"}, wantDispatch: true, wantCompactDispatch: true, wantIncrementalDispatch: true, wantCompactFallback: true, wantCompactRoutedDelta: 0, wantCompactFallbackDelta: 1, wantForestReason: "dead_end", wantDispatchChecked: 1, wantDispatchRun: 1, wantDispatchVisited: 2},
		{name: "a0_metrics", path: "../testdata/dispatcher_census_a0/doxygen/medium__metrics.py", sourceSHA: "31622a6c075ffa6f78a16af6e379f517213d42ff67729bbd0d10551c5fca9702", goDigest: "5adbacb1ec949237a802a56a5c95c3c7a1ce17fe9c8db5423b63f083da62d5d1", cDigest: "6660931c2bf1bf1e0f909a1cac1e4cd8446853ae4466781c943e28fbcc61e860", wantDivergence: &normalizationKnownDivergence{Path: "/ERROR", Category: "shape", GoValue: "children=0", CValue: "children=279", Reason: "the C oracle retains the recovered ERROR children that Go currently drops"}, wantCompactFallback: true, wantCompactRoutedDelta: 0, wantCompactFallbackDelta: 1, wantForestReason: "dead_end"},
		// goDigest and wantDispatchVisited moved: the absorbed tag_name token
		// no longer carries its own error bit (pushOrExtendErrorNode change),
		// so expectedRootCanFrameRecoveredFragments no longer treats it as
		// recovery debris it can elide, and the "document" root keeps the
		// ERROR wrapper around it (one more node than before). The Go/C root
		// type divergence documented below is unrelated and still holds.
		{name: "a0_example_cfg", path: "../testdata/dispatcher_census_a0/doxygen/small__example.cfg", sourceSHA: "86998161914382f8152e4984db091e7bf486799c1091fc6c57db4e704eee4a3b", goDigest: "b9bb1f5701ae912a89cde155e0b18ba39bbd6db5619b7d1b5d52a4b079e6219b", cDigest: "f1938d5c7bc544856a5df6c204af75af10a5395bd1f89f560c74caef5acf191f", wantDivergence: &normalizationKnownDivergence{Path: "/document", Category: "type", GoValue: "document", CValue: "ERROR", Reason: "the C oracle keeps the whole A0 source under an ERROR root"}, wantDispatch: true, wantCompactDispatch: true, wantIncrementalDispatch: true, wantCompactFallback: true, wantCompactRoutedDelta: 0, wantCompactFallbackDelta: 1, wantForestReason: "dead_end", wantDispatchChecked: 1, wantDispatchRun: 1, wantDispatchVisited: 3},
		{name: "historical_childless_error", source: "/** Adds all words in \\a s to document \\a doc with weight \\a wfd */", sourceSHA: "ff90d209911d0d32bf44ebff0742e6f42ff40a6f4978860a00ec3f7228b2af24", rawDigest: "6c16ff1b99a3b116d575f90aa0fe5456381b442a58af021dac36e6954345ce4c", goDigest: "0e1129b2130636e62dd05b2494c22a9a2b5b6ec044aea2eeb4dc836380e38b38", cDigest: "0e1129b2130636e62dd05b2494c22a9a2b5b6ec044aea2eeb4dc836380e38b38", wantDispatch: true, wantCompactDispatch: true, wantIncrementalDispatch: true, wantCompactFallback: true, wantCompactRoutedDelta: 0, wantCompactFallbackDelta: 1, wantForestReason: "eof_no_root", wantDispatchRewrites: 3, wantDispatchChecked: 2, wantDispatchRun: 2, wantDispatchVisited: 4},
		// rawDigest and goDigest moved: one of the recovered @-tag spans
		// absorbs a bare tag_name token under a nested ERROR (the same
		// pushOrExtendErrorNode shape as a0_example_cfg above); its absorbed
		// leaf no longer carries an error bit. wantDispatchVisited is
		// unaffected: the arm walks the same node count either way.
		{name: "historical_recovered_document", source: "/**\n * @param {int} value\n * @brief Example\n */", sourceSHA: "f6deae068bcf0fe684f8623d671ee5dfbfab47c93d7827ec03c3b4b5330f8309", rawDigest: "131d2425cc4bbd59336c92a6f728d15142cf72940dc0b92426e26476fcb5d1c1", goDigest: "df23e9820913fced85dd38bd693bbb6bc746b0a65c9b4dd2e3812e83ff9df5f0", cDigest: "05813d8b13788902a7f9b9322ca16127ecf5e9c3694d60726acc7a511be622fe", wantDivergence: &normalizationKnownDivergence{Path: "/document/tag_name[0]", Category: "type", GoValue: "tag_name", CValue: "ERROR", Reason: "pushOrExtendErrorNode now keeps the absorbed @brief tag_name as a real child instead of dropping it; the C oracle still reports an ERROR at this position"}, wantDispatch: true, wantCompactDispatch: true, wantIncrementalDispatch: true, wantCompactFallback: true, wantCompactRoutedDelta: 0, wantCompactFallbackDelta: 1, wantForestReason: "dead_end", wantDispatchRewrites: 16, wantDispatchChecked: 1, wantDispatchRun: 1, wantDispatchVisited: 16},
		{name: "registered_smoke", source: grammars.ParseSmokeSample("doxygen"), sourceSHA: "e2d564b999c40b0a53450771ffa82adf7880375449e8628fefd118aae21056d7", goDigest: "1ae089a98760be594f06d0820951e01714097e99621cc2cd4428ce09ba867083", cDigest: "1ae089a98760be594f06d0820951e01714097e99621cc2cd4428ce09ba867083", wantDispatch: true, wantCompactDispatch: false, wantIncrementalDispatch: true, wantCompactFallback: false, wantCompactRoutedDelta: 1, wantCompactFallbackDelta: 0, wantForestReason: "nolook_relex_empty", wantDispatchChecked: 1, wantDispatchRun: 1, wantDispatchVisited: 6},
	}
	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			source := []byte(witness.source)
			if witness.path != "" {
				var err error
				source, err = os.ReadFile(filepath.Join(".", witness.path))
				if err != nil {
					t.Fatal(err)
				}
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != witness.sourceSHA {
				t.Fatalf("source SHA-256=%s, want %s", got, witness.sourceSHA)
			}
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C oracle returned no root")
			}
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			if witness.cDigest == "" {
				t.Fatal("empty pinned C digest")
			}
			if cDigest != witness.cDigest {
				t.Fatalf("C digest=%s, want %s", cDigest, witness.cDigest)
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
			if witness.rawDigest != "" {
				assertDoxygenNextRoute(t, "raw", raw, language, witness.rawDigest, 0, false, 0, 0, 0, 0)
			} else {
				assertDoxygenNextRoute(t, "raw", raw, language, witness.goDigest, 0, false, 0, 0, 0, 0)
			}
			assertDoxygenNextRoute(t, "production", production, language, witness.goDigest, 0, witness.wantDispatch, witness.wantDispatchRewrites, witness.wantDispatchChecked, witness.wantDispatchRun, witness.wantDispatchVisited)
			if diff := FirstDivergenceDumpV1(production.RootNode(), language, cTree.RootNode()); witness.wantDivergence != nil {
				if diff == nil {
					t.Fatalf("%s expected locked-C divergence %+v, got exact match", witness.name, witness.wantDivergence)
				}
				if diff.Path != witness.wantDivergence.Path || diff.Category != witness.wantDivergence.Category || diff.GoValue != witness.wantDivergence.GoValue || diff.CValue != witness.wantDivergence.CValue {
					t.Fatalf("%s locked-C divergence=%+v, want %+v", witness.name, diff, witness.wantDivergence)
				}
				t.Logf("%s known locked-C divergence: %s", witness.name, witness.wantDivergence.Reason)
			} else if diff != nil {
				t.Fatalf("unexpected locked-C divergence: %+v", diff)
			} else if witness.goDigest != cDigest {
				t.Fatalf("exact locked-C witness Go digest=%s, C digest=%s", witness.goDigest, cDigest)
			}

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(compact.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			t.Logf("compact counters=%d/%d->%d/%d reason=%q", routedBefore, fallbackBefore, routedAfter, fallbackAfter, gotreesitter.AdmissionCandidateLastFallbackReason())
			if routedAfter-routedBefore != witness.wantCompactRoutedDelta || fallbackAfter-fallbackBefore != witness.wantCompactFallbackDelta {
				t.Fatalf("compact counter delta=%d/%d, want %d/%d", routedAfter-routedBefore, fallbackAfter-fallbackBefore, witness.wantCompactRoutedDelta, witness.wantCompactFallbackDelta)
			}
			if gotFallback := fallbackAfter > fallbackBefore; gotFallback != witness.wantCompactFallback {
				t.Fatalf("compact fallback=%t, want %t", gotFallback, witness.wantCompactFallback)
			}
			assertDoxygenNextRoute(t, "compact", compact, language, witness.goDigest, 0, witness.wantCompactDispatch, witness.wantDispatchRewrites, witness.wantDispatchChecked, witness.wantDispatchRun, witness.wantDispatchVisited)

			forestParser := gotreesitter.NewParser(language)
			forest, forestOK := forestParser.ParseForestExperimental(source)
			t.Logf("forest accepted=%t", forestOK && forest != nil)
			if forestOK || forest != nil {
				t.Fatalf("forest route accepted unexpectedly: ok=%t tree=%v", forestOK, forest != nil)
			}
			if _, _, reason, _ := forestParser.ForestDeclineInfo(); reason != witness.wantForestReason {
				t.Fatalf("forest decline reason=%q, want %s", reason, witness.wantForestReason)
			}
			if forest != nil {
				t.Cleanup(forest.Release)
			}

			incrementalParser := gotreesitter.NewParser(language)
			incrementalParser.SetAdmissionCandidateRoute(false)
			base := append(append([]byte(nil), source...), ' ')
			oldTree, err := incrementalParser.Parse(base)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(oldTree.Release)
			sourceEnd := doxygenNextPoint(source)
			oldTree.Edit(gotreesitter.InputEdit{StartByte: uint32(len(source)), OldEndByte: uint32(len(base)), NewEndByte: uint32(len(source)), StartPoint: sourceEnd, OldEndPoint: doxygenNextPoint(base), NewEndPoint: sourceEnd})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(incremental.Release)
			t.Logf("incremental profile=%+v", profile)
			if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "external_scanner_unsupported" || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
				t.Fatalf("incremental scanner fallback profile=%+v", profile)
			}
			assertDoxygenNextRoute(t, "incremental", incremental, language, witness.goDigest, 0, witness.wantIncrementalDispatch, witness.wantDispatchRewrites, witness.wantDispatchChecked, witness.wantDispatchRun, witness.wantDispatchVisited)
			t.Logf("witness=%s bytes=%d source_sha256=%s c_digest=%s compact_fallback=%t forest=%s incremental_edit=delete_trailing_space incremental=external_scanner_unsupported raw_digest=%s production_digest=%s dispatch_rewrites=%d", witness.name, len(source), witness.sourceSHA, cDigest, witness.wantCompactFallback, witness.wantForestReason, witness.goDigest, witness.goDigest, witness.wantDispatchRewrites)
		})
	}
}

func assertDoxygenNextRoute(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language, wantDigest string, wantTotalRewrites uint64, wantDispatch bool, wantDispatchRewrites, wantDispatchChecked, wantDispatchRun, wantDispatchVisited uint64) {
	t.Helper()
	if !wantDispatch {
		wantDispatchRewrites = 0
		wantDispatchChecked = 0
		wantDispatchRun = 0
		wantDispatchVisited = 0
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatalf("inspect %s: %v", route, err)
	}
	if inspection.SHA256 != wantDigest {
		t.Fatalf("%s digest=%s, want %s", route, inspection.SHA256, wantDigest)
	}
	runtime := tree.ParseRuntime()
	if runtime.NormalizationNodesRewritten != wantTotalRewrites {
		t.Fatalf("%s total rewrites=%d, want %d", route, runtime.NormalizationNodesRewritten, wantTotalRewrites)
	}
	var dispatchPass *gotreesitter.NormalizationPassRuntime
	if runtime.NormalizationPasses != nil {
		for index := range *runtime.NormalizationPasses {
			candidate := &(*runtime.NormalizationPasses)[index]
			if candidate.Name == "dispatch.doxygen" {
				dispatchPass = candidate
				break
			}
		}
	}
	if wantDispatch != (dispatchPass != nil) {
		t.Fatalf("%s dispatch.doxygen presence=%t, want %t", route, dispatchPass != nil, wantDispatch)
	}
	if dispatchPass != nil && dispatchPass.NodesRewritten != wantDispatchRewrites {
		t.Fatalf("%s dispatch.doxygen rewrites=%d, want %d", route, dispatchPass.NodesRewritten, wantDispatchRewrites)
	}
	if dispatchPass != nil {
		if dispatchPass.Checked != wantDispatchChecked || dispatchPass.Run != wantDispatchRun || dispatchPass.NodesVisited != wantDispatchVisited {
			t.Fatalf("%s dispatch.doxygen counters=%d/%d/%d, want %d/%d/%d", route, dispatchPass.Checked, dispatchPass.Run, dispatchPass.NodesVisited, wantDispatchChecked, wantDispatchRun, wantDispatchVisited)
		}
	} else if wantDispatchChecked != 0 || wantDispatchRun != 0 || wantDispatchVisited != 0 {
		t.Fatalf("%s has no dispatch.doxygen pass but expected counters=%d/%d/%d", route, wantDispatchChecked, wantDispatchRun, wantDispatchVisited)
	}
	t.Logf("route=%s digest=%s error=%t rewrites=%d dispatch=%+v", route, inspection.SHA256, tree.RootNode().HasError(), runtime.NormalizationNodesRewritten, dispatchPass)
}

func doxygenNextFileSHA(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func doxygenNextPoint(source []byte) gotreesitter.Point {
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

// TestDoxygenNextLiveArmReceiptDocument guards the current blocker receipt.
func TestDoxygenNextLiveArmReceiptDocument(t *testing.T) {
	doc, err := os.ReadFile("../docs/root-normalization-retirement.md")
	if err != nil {
		t.Fatal(err)
	}
	documentText := string(doc)
	sectionStart := strings.Index(documentText, "## 2026-08-23 Doxygen dispatcher blocker receipt")
	if sectionStart < 0 {
		t.Fatal("Doxygen N31k section is missing")
	}
	sectionEndRelative := strings.Index(documentText[sectionStart:], "\n### R3 — move materialization invariants upstream")
	if sectionEndRelative < 0 {
		t.Fatal("Doxygen N31k section end is missing")
	}
	document := strings.Join(strings.Fields(documentText[sectionStart:sectionStart+sectionEndRelative]), " ")
	for _, marker := range []string{
		"Status: `KEEP LIVE / NO-GO`. Keep `dispatch.doxygen` live.",
		"The focused Doxygen probe covers raw, production, compact, forest, incremental, and locked-C routes.",
		"Included-ranges coverage is not applicable to Doxygen.",
		"Go and C both report digest `1ae089a98760be594f06d0820951e01714097e99621cc2cd4428ce09ba867083`.",
		"The A0 Doxygen fixtures have zero rewrites.",
		"The A0 production passes report checked/run/rewritten values of `1/1/0` for `CMakeLists.txt` and `example.cfg`.",
		"The compact counter deltas are exact.",
		"Forest decline reasons are witness-specific.",
		"The successful route receipt is under `/tmp/gts-n31k-doxygen-artifacts/20260823T152330Z-n31k-route-repair-v2`.",
		"Its `container.log` SHA-256 is `5c9e3f3173bb43294152bf2555413cdc9e95e63d6e2821e841c3bd79e7216626`.",
		"The first document guard receipt is under `/tmp/gts-n31k-doxygen-artifacts/20260823T152357Z-n31k-document-first-repair-v1`.",
		"Its `container.log` SHA-256 is `3bcc26bfbdb52b8e98e001f74f204c34e60e85e387fb6f2950f79a8cc7c7aa77`.",
		"The final document guard receipt is under `/tmp/gts-n31k-doxygen-artifacts/20260823T152453Z-n31k-document-final-repair-v1`.",
		"Its `container.log` SHA-256 is `cea994a81199e3f792ffc1865f6405dcc891fa9a35d277b954441490575843c3`.",
		"The final marker-validation artifact is under `/tmp/gts-n31k-doxygen-artifacts/20260823T152530Z-n31k-document-final-repair-v2`.",
		"Its `container.log` SHA-256 is `42dadbaefc2af5496b8cb2bcaeda7ab0e90dcfdc8e3c74751a3d50ac9185f241`.",
		"The next document guard is the terminal external verifier. Do not self-pin its path or hash because self-pinning creates a circular receipt.",
		"Each incremental check parses the pinned source with one trailing space.",
		"The incremental probe deletes one deterministic trailing space from each parsed source, then checks the original source digest and profile.",
	} {
		marker = strings.Join(strings.Fields(marker), " ")
		if !strings.Contains(document, marker) {
			t.Fatalf("Doxygen receipt lacks marker %q", marker)
		}
		t.Logf("checked marker=%s", marker)
	}
}
