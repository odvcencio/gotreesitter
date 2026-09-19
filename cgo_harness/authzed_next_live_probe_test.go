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

const authzedNextCleanSource = "caveat foo(someParam int) {\n\tsomeParam == 42\n}\n\n" +
	"definition document {\n" +
	"\trelation viewer: user | user:*\n" +
	"\trelation editor: user | group#member with foo\n" +
	"\trelation parent: organization\n" +
	"\tpermission edit = editor\n" +
	"\tpermission view = viewer + edit + parent->view\n" +
	"\tpermission other = viewer - edit\n" +
	"\tpermission intersect = viewer & edit\n" +
	"\tpermission with_nil = (viewer - edit) & parent->view & nil\n" +
	"}\n"

type authzedNextWitness struct {
	name                       string
	file                       string
	source                     []byte
	malformed                  bool
	wantSourceSHA              string
	wantRawDigest              string
	wantGoDigest               string
	wantCDigest                string
	wantRawDiff                *DumpV1Divergence
	wantRouteDiff              *DumpV1Divergence
	wantCompactMode            string
	wantForest                 bool
	wantIncrementalReuse       bool
	wantIncrementalUnsupported bool
	wantIncrementalReason      string
	wantProductionRewrites     uint64
	wantIncrementalRewrites    uint64
}

// TestAuthzedNextLiveArmProbe records all parser routes for A0, clean, and
// recovery witnesses before any retirement decision.
func TestAuthzedNextLiveArmProbe(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := grammars.AuthzedLanguage()
	if language == nil {
		t.Fatal("Authzed language is nil")
	}
	if language.ExternalScanner != nil {
		t.Fatalf("Authzed unexpectedly has an external scanner: %T", language.ExternalScanner)
	}
	cLanguage, err := COracleLanguage("authzed")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := COracleIdentity("authzed")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("grammar=authzed external_scanner=false grammar_lock=83e5c26a8687eb4688fe91d690c735cc3d21ad81 c_oracle_runtime=%s@%s c_oracle_grammar=%s@%s c_oracle_artifact=%s c_oracle_artifact_sha256=%s", identity.RuntimeVersion, identity.RuntimeCommit, identity.GrammarRepo, identity.GrammarCommit, identity.GrammarArtifactPath, identity.GrammarArtifactSHA256)

	witnesses := []authzedNextWitness{
		{
			name: "a0-large-superlarge", file: "large__superlarge.zed",
			wantSourceSHA:   "86921255ed996dbf67a519adf1e33d5353aa873f025f77e2150ded76ac81197c",
			wantRawDigest:   "d9c96edd4ddc6b69484fb1fc71b1f5da15386509b91d3b7a797ef1d849e08345",
			wantGoDigest:    "d9c96edd4ddc6b69484fb1fc71b1f5da15386509b91d3b7a797ef1d849e08345",
			wantCDigest:     "d9c96edd4ddc6b69484fb1fc71b1f5da15386509b91d3b7a797ef1d849e08345",
			wantCompactMode: "accepted", wantIncrementalReuse: true,
		},
		{
			name: "a0-small-doccomments", file: "small__doccomments.zed",
			wantSourceSHA:   "fac84d359ebf4b628e54f567b30657792550d0c58441c42fb10a2a044b379574",
			wantRawDigest:   "d40e29166c95e851e9c4e924cb588bde163b118c2f336df7cd631e09638551ce",
			wantGoDigest:    "d40e29166c95e851e9c4e924cb588bde163b118c2f336df7cd631e09638551ce",
			wantCDigest:     "3106320fc4b5649e232fc96f368a1bb8d25d3eae3baa3b30e4042692176981fd",
			wantRawDiff:     authzedNextExpectedDivergence("/source_file", "shape", "children=16", "children=6"),
			wantRouteDiff:   authzedNextExpectedDivergence("/source_file", "shape", "children=16", "children=6"),
			wantCompactMode: "fallback", wantForest: true,
			wantIncrementalUnsupported: true, wantIncrementalReason: "forest_recovery_fallback",
		},
		{
			name: "a0-small-localimport", file: "small__localimport_with_quotes_in_quotes.zed",
			wantSourceSHA:   "a78131bee5849e2ce1b002605896a090a0003e052574e76ea469c3b895c534e0",
			wantRawDigest:   "febf866c4e81009c7dc7be7abca2716bb7dba6a59696c8bae222a13f65c8c069",
			wantGoDigest:    "442465537f4e5ee65298701ef8832b6c326a48c6523c40abeb25eaa105601d25",
			wantCDigest:     "442465537f4e5ee65298701ef8832b6c326a48c6523c40abeb25eaa105601d25",
			wantRawDiff:     authzedNextExpectedDivergence("/source_file/ERROR[2]/\n[0]", "error", "true", "false"),
			wantCompactMode: "fallback", wantIncrementalReuse: true,
			wantProductionRewrites: 17, wantIncrementalRewrites: 17,
		},
		{
			name: "positive-clean-schema", source: []byte(authzedNextCleanSource),
			wantSourceSHA:   "32ac093bcf4c41596f2d6ace463df54bdd2d8ad3880ad0241c0993612a56b502",
			wantRawDigest:   "e5a70c33ef46081de56bd282ed8b314a17577fed33fe18390afcd2626da3a15c",
			wantGoDigest:    "e5a70c33ef46081de56bd282ed8b314a17577fed33fe18390afcd2626da3a15c",
			wantCDigest:     "e5a70c33ef46081de56bd282ed8b314a17577fed33fe18390afcd2626da3a15c",
			wantCompactMode: "accepted", wantForest: false, wantIncrementalReuse: true,
		},
		{
			name: "positive-empty-definition", source: []byte("definition user {}\n"),
			wantSourceSHA:   "5b622d08904a68f8dc95905b1807d0792434f203aa3962084aa8c7ff60606e71",
			wantRawDigest:   "9314a3abd9b53ae58dd026fc30d8929a07c4fdbb72857a63d1a650b15844bb6c",
			wantGoDigest:    "9314a3abd9b53ae58dd026fc30d8929a07c4fdbb72857a63d1a650b15844bb6c",
			wantCDigest:     "9314a3abd9b53ae58dd026fc30d8929a07c4fdbb72857a63d1a650b15844bb6c",
			wantCompactMode: "accepted", wantForest: true, wantIncrementalReuse: true,
		},
		{
			name: "recovery-single-quoted-caveat",
			source: []byte("definition another {}\n\n" +
				"caveat somecaveat(somecondition uint, somebool bool, somestring string) {\n" +
				"  somecondition == 42 && somebool && somestring == 'hello'\n" +
				"}\n\n" + "definition user {}"),
			malformed:       true,
			wantSourceSHA:   "9bc941c8a15cdc634cc27e8b0694b1e2d7f42c76f32c3eabb06eaa4556d3077b",
			wantRawDigest:   "320b6bec5f40133b859fefe9ab71b6e9365d43daff175deba61f9e8ec90bbfeb",
			wantGoDigest:    "320b6bec5f40133b859fefe9ab71b6e9365d43daff175deba61f9e8ec90bbfeb",
			wantCDigest:     "1e725f32692398293473557052f21ded90b7b638e8a26367b8fcbe3243978078",
			wantRawDiff:     authzedNextExpectedDivergence("/source_file/\n[5]", "type", "", "}"),
			wantRouteDiff:   authzedNextExpectedDivergence("/source_file/\n[5]", "type", "", "}"),
			wantCompactMode: "fallback", wantIncrementalReuse: true,
		},
		{
			name: "recovery-stray-caveat-tail",
			source: []byte("definition user {}\n\n" +
				"caveat somecaveat(somecondition int) {\n" +
				"  somecondition == 42 `\n" + "}"),
			malformed:       true,
			wantSourceSHA:   "a15377a7a655c1543f7146b0667f9862cae28a288d04a09902709348b0ff2149",
			wantRawDigest:   "40cb42f2872b8e27b26adf238c56ba1ead92e12c035d84588a47605b05df7c8c",
			wantGoDigest:    "40cb42f2872b8e27b26adf238c56ba1ead92e12c035d84588a47605b05df7c8c",
			wantCDigest:     "50d04110529fd05ab2d699c5fea863bb9a90e6c485339bb83979f851664ad68b",
			wantRawDiff:     authzedNextExpectedDivergence("/source_file/\n[3]", "type", "", "}"),
			wantRouteDiff:   authzedNextExpectedDivergence("/source_file/\n[3]", "type", "", "}"),
			wantCompactMode: "fallback", wantIncrementalReuse: true,
		},
		{
			name: "recovery-unclosed-caveat",
			source: []byte("definition another {}\n\n" +
				"caveat somecaveat(somecondition uint, somebool bool) {\n" +
				"  somemap{\n\n" + "  \n" + "}\n\n" + "definition user {}"),
			malformed:       true,
			wantSourceSHA:   "e8d2fcc0194cf928e66ba9c5f9c7db189766848513fcec7683f9d4c69a6ea6ac",
			wantRawDigest:   "21fc05856ec3e6188a3d37e9763067750f6ca5bc3a6114560033e2897ade1ecd",
			wantGoDigest:    "21fc05856ec3e6188a3d37e9763067750f6ca5bc3a6114560033e2897ade1ecd",
			wantCDigest:     "281c359752fd28f8e51bae5329e70d9edfd78be0ba115ae022e502b38cbca93b",
			wantRawDiff:     authzedNextExpectedDivergence("/source_file/caveat[2]/block_c[3]/ERROR[2]/{[0]", "error", "true", "false"),
			wantRouteDiff:   authzedNextExpectedDivergence("/source_file/caveat[2]/block_c[3]/ERROR[2]/{[0]", "error", "true", "false"),
			wantCompactMode: "fallback", wantIncrementalReuse: true,
		},
		{
			name: "recovery-use-directive",
			source: []byte("use import\n\n" + "import \"subjects.zed\"\n\n" +
				"definition resource {\n" + "  relation viewer: user\n" +
				"  permission view = viewer\n" + "}\n"),
			malformed:       true,
			wantSourceSHA:   "54f2f51dd7c0a9d03c312e346224eb56144f84c1d8abf143af11f4578aee18f9",
			wantRawDigest:   "f62b682b3119d3e50336e18ec258f6ceca80a9aaac89cc264a7941a75448581e",
			wantGoDigest:    "e4eef8bc22e9f86a8aec754a2020a503e48ca5d70ffca10d5ce80fc358a8acdf",
			wantCDigest:     "e4eef8bc22e9f86a8aec754a2020a503e48ca5d70ffca10d5ce80fc358a8acdf",
			wantRawDiff:     authzedNextExpectedDivergence("/source_file/ERROR[0]/\n[0]", "error", "true", "false"),
			wantCompactMode: "fallback", wantIncrementalReuse: true,
			wantProductionRewrites: 11, wantIncrementalRewrites: 11,
		},
		{
			name:            "recovery-missing-permission-expression",
			source:          []byte("definition resource {\n  permission view =\n}\n"),
			malformed:       true,
			wantSourceSHA:   "c70345ec2be6e59fee29a9913ae4983cac0f775d4f69bad97b11c001e5c771ab",
			wantRawDigest:   "d92cae8ce3fe890414a86663d628ed38ad44ad9440427d0b74224bc275d3bbb7",
			wantGoDigest:    "d92cae8ce3fe890414a86663d628ed38ad44ad9440427d0b74224bc275d3bbb7",
			wantCDigest:     "d92cae8ce3fe890414a86663d628ed38ad44ad9440427d0b74224bc275d3bbb7",
			wantCompactMode: "fallback", wantIncrementalReuse: true,
		},
		{
			name:            "recovery-malformed-relation",
			source:          []byte("definition resource {\n  relation viewer:\n}\n"),
			malformed:       true,
			wantSourceSHA:   "7e36b3bf0f6f2be9c578aa5393ca82f2d5e3ba9768ada6ebfa1884c09fe5e2cb",
			wantRawDigest:   "19e021d8e44fef573b634a370fd3bba4e6175f315e041cfcbcb802d48a8a6cb3",
			wantGoDigest:    "19e021d8e44fef573b634a370fd3bba4e6175f315e041cfcbcb802d48a8a6cb3",
			wantCDigest:     "fdbf594f12c71c8924e103838bc96ce963008f897bfed7a4355c2dff7bda3cf2",
			wantRawDiff:     authzedNextExpectedDivergence("/source_file/ERROR[0]/\n[5]", "error", "true", "false"),
			wantRouteDiff:   authzedNextExpectedDivergence("/source_file/ERROR[0]/\n[5]", "error", "true", "false"),
			wantCompactMode: "fallback", wantIncrementalReuse: true,
		},
	}

	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			source := witness.source
			if source == nil {
				source, err = os.ReadFile(filepath.Join("..", "testdata", "dispatcher_census_a0", "authzed", witness.file))
				if err != nil {
					t.Fatal(err)
				}
			}
			sourceSHA := fmt.Sprintf("%x", sha256.Sum256(source))
			if sourceSHA != witness.wantSourceSHA {
				t.Fatalf("source SHA-256=%s, want %s", sourceSHA, witness.wantSourceSHA)
			}
			t.Logf("witness=%s malformed=%t bytes=%d source_sha256=%s", witness.name, witness.malformed, len(source), sourceSHA)

			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C parser returned no root")
			}
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			if cDigest != witness.wantCDigest {
				t.Fatalf("locked-C digest=%s, want %s", cDigest, witness.wantCDigest)
			}

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(raw.Release)
			authzedNextCheckReceipt(t, "raw", raw, language, cTree, cDigest, witness.wantRawDigest, witness.wantRawDiff)
			authzedNextCheckNoPass(t, "raw", raw)
			t.Logf("route=raw %s", authzedNextReceipt(raw, language, cTree, cDigest))

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			authzedNextCheckReceipt(t, "production", production, language, cTree, cDigest, witness.wantGoDigest, witness.wantRouteDiff)
			authzedNextCheckPass(t, "production", production, witness.wantProductionRewrites)
			t.Logf("route=production %s", authzedNextReceipt(production, language, cTree, cDigest))

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(compact.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			compactMode := authzedNextCompactMode(routedBefore, fallbackBefore, routedAfter, fallbackAfter)
			if compactMode != witness.wantCompactMode {
				t.Fatalf("compact mode=%s, want %s (counters=%d/%d->%d/%d)", compactMode, witness.wantCompactMode, routedBefore, fallbackBefore, routedAfter, fallbackAfter)
			}
			if compactMode == "fallback" && !strings.Contains(gotreesitter.AdmissionCandidateLastFallbackReason(), "generic scheduler has no table action") {
				t.Fatalf("compact fallback reason=%q, want generic scheduler EOF fallback", gotreesitter.AdmissionCandidateLastFallbackReason())
			}
			authzedNextCheckReceipt(t, "compact", compact, language, cTree, cDigest, witness.wantGoDigest, witness.wantRouteDiff)
			if compactMode == "fallback" {
				authzedNextCheckPass(t, "compact", compact, witness.wantProductionRewrites)
			} else {
				authzedNextCheckNoPass(t, "compact", compact)
			}
			t.Logf("route=compact mode=%s %s", compactMode, authzedNextReceipt(compact, language, cTree, cDigest))

			forestParser := gotreesitter.NewParser(language)
			forest, forestOK := forestParser.ParseForestExperimental(source)
			if forestOK && forest != nil {
				if !witness.wantForest {
					t.Fatal("forest route accepted, want decline")
				}
				t.Cleanup(forest.Release)
				authzedNextCheckReceipt(t, "forest", forest, language, cTree, cDigest, witness.wantGoDigest, witness.wantRouteDiff)
				authzedNextCheckOptionalPass(t, "forest", forest, 0)
				t.Logf("route=forest mode=accepted %s", authzedNextReceipt(forest, language, cTree, cDigest))
			} else {
				if witness.wantForest {
					t.Fatal("forest route declined, want acceptance")
				}
				t.Log("route=forest mode=declined")
			}

			incrementalParser := gotreesitter.NewParser(language)
			incrementalParser.SetAdmissionCandidateRoute(false)
			base := append(append([]byte(nil), source...), ' ')
			oldTree, err := incrementalParser.Parse(base)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(oldTree.Release)
			end := authzedNextPointAtByte(source)
			oldEnd := end
			oldEnd.Column++
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(source)),
				OldEndByte:  uint32(len(base)),
				NewEndByte:  uint32(len(source)),
				StartPoint:  end,
				OldEndPoint: oldEnd,
				NewEndPoint: end,
			})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(incremental.Release)
			authzedNextCheckReceipt(t, "incremental", incremental, language, cTree, cDigest, witness.wantGoDigest, witness.wantRouteDiff)
			authzedNextCheckPass(t, "incremental", incremental, witness.wantIncrementalRewrites)
			if profile.OldTreeReuseRoute != witness.wantIncrementalReuse || profile.ReuseUnsupported != witness.wantIncrementalUnsupported || profile.ReuseUnsupportedReason != witness.wantIncrementalReason {
				t.Fatalf("incremental reuse=%t unsupported=%t reason=%q, want reuse=%t unsupported=%t reason=%q", profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason, witness.wantIncrementalReuse, witness.wantIncrementalUnsupported, witness.wantIncrementalReason)
			}
			t.Logf("route=incremental reuse=%t unsupported=%t reason=%q reused_subtrees=%d reused_bytes=%d %s", profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason, profile.ReusedSubtrees, profile.ReusedBytes, authzedNextReceipt(incremental, language, cTree, cDigest))
		})
	}
}

func authzedNextExpectedDivergence(path, category, goValue, cValue string) *DumpV1Divergence {
	return &DumpV1Divergence{Path: path, Category: category, GoValue: goValue, CValue: cValue}
}

func authzedNextCheckReceipt(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, cDigest, wantDigest string, wantDiff *DumpV1Divergence) {
	t.Helper()
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("%s returned no tree", route)
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatalf("%s inspect: %v", route, err)
	}
	if inspection.SHA256 != wantDigest {
		t.Fatalf("%s digest=%s, want %s", route, inspection.SHA256, wantDigest)
	}
	gotDiff := FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode())
	if gotDiff == nil || wantDiff == nil {
		if gotDiff != wantDiff {
			t.Fatalf("%s divergence=%+v, want %+v", route, gotDiff, wantDiff)
		}
	} else if gotDiff.Path != wantDiff.Path || gotDiff.Category != wantDiff.Category {
		t.Fatalf("%s divergence path/category=%s/%s, want %s/%s (values got=%q/%q want=%q/%q)", route, gotDiff.Path, gotDiff.Category, wantDiff.Path, wantDiff.Category, gotDiff.GoValue, gotDiff.CValue, wantDiff.GoValue, wantDiff.CValue)
	}
	if cDigest == "" {
		t.Fatal("locked-C digest is empty")
	}
	if tree.ParseRuntime().NativeRecoveredStructureAuthoritative {
		t.Fatalf("%s native_authoritative=true, want false", route)
	}
}

func authzedNextCheckPass(t *testing.T, route string, tree *gotreesitter.Tree, wantRewritten uint64) {
	t.Helper()
	pass := authzedNextPass(tree)
	if pass == nil {
		t.Fatalf("%s did not record dispatch.authzed", route)
	}
	if pass.Checked != 1 || pass.Run != 1 || pass.NodesRewritten != wantRewritten {
		t.Fatalf("%s dispatch.authzed pass=%d/%d/%d/%d, want checked/run=1/1 and rewrites=%d", route, pass.Checked, pass.Run, pass.NodesVisited, pass.NodesRewritten, wantRewritten)
	}
}

func authzedNextCheckNoPass(t *testing.T, route string, tree *gotreesitter.Tree) {
	t.Helper()
	if pass := authzedNextPass(tree); pass != nil {
		t.Fatalf("%s recorded dispatch.authzed pass=%+v, want none", route, pass)
	}
}

func authzedNextCheckOptionalPass(t *testing.T, route string, tree *gotreesitter.Tree, wantRewritten uint64) {
	t.Helper()
	if pass := authzedNextPass(tree); pass != nil && pass.NodesRewritten != wantRewritten {
		t.Fatalf("%s dispatch.authzed rewrites=%d, want %d", route, pass.NodesRewritten, wantRewritten)
	}
}

func authzedNextPass(tree *gotreesitter.Tree) *gotreesitter.NormalizationPassRuntime {
	if tree == nil || tree.ParseRuntime().NormalizationPasses == nil {
		return nil
	}
	for i := range *tree.ParseRuntime().NormalizationPasses {
		pass := &(*tree.ParseRuntime().NormalizationPasses)[i]
		if pass.Name == "dispatch.authzed" {
			return pass
		}
	}
	return nil
}

func authzedNextCompactMode(routedBefore, fallbackBefore, routedAfter, fallbackAfter uint64) string {
	if routedAfter == routedBefore+1 && fallbackAfter == fallbackBefore {
		return "accepted"
	}
	if routedAfter == routedBefore && fallbackAfter == fallbackBefore+1 {
		return "fallback"
	}
	return fmt.Sprintf("counters=%d/%d->%d/%d", routedBefore, fallbackBefore, routedAfter, fallbackAfter)
}

func authzedNextReceipt(tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, cDigest string) string {
	if tree == nil || tree.RootNode() == nil {
		return "tree=nil"
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		return fmt.Sprintf("inspect_error=%v", err)
	}
	diff := FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode())
	return fmt.Sprintf("error_root=%t digest=%s c_digest=%s exact=%t rewrites=%d native_authoritative=%t passes=%s divergence=%+v", tree.RootNode().HasError(), inspection.SHA256, cDigest, diff == nil && inspection.SHA256 == cDigest, tree.ParseRuntime().NormalizationNodesRewritten, tree.ParseRuntime().NativeRecoveredStructureAuthoritative, authzedNextPasses(tree), diff)
}

func authzedNextPasses(tree *gotreesitter.Tree) string {
	runtime := tree.ParseRuntime()
	if runtime.NormalizationPasses == nil {
		return "none"
	}
	parts := make([]string, 0, len(*runtime.NormalizationPasses))
	for _, pass := range *runtime.NormalizationPasses {
		parts = append(parts, fmt.Sprintf("%s:%d/%d/%d/%d", pass.Name, pass.Checked, pass.Run, pass.NodesVisited, pass.NodesRewritten))
	}
	return fmt.Sprintf("%v", parts)
}

func authzedNextPointAtByte(source []byte) gotreesitter.Point {
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

// TestAuthzedNextLiveArmReceiptDocument guards the durable blocker receipt.
func TestAuthzedNextLiveArmReceiptDocument(t *testing.T) {
	documents := map[string][]string{
		"../docs/root-normalization-retirement.md": {
			"## 2026-08-24 Authzed dispatcher blocker receipt",
			"Status: NO-GO. Keep `dispatch.authzed` live.",
			"A0 means the initial dispatcher census.",
			"The authenticated corpus lock is unavailable.",
			"The A0 receipt records 8,326 visited nodes, 18 rewrites",
			"The refreshed A0 production routes visited 8,322 nodes and rewrote 17 nodes.",
			"The `a0-small-localimport` witness needs 17 production rewrites.",
			"The `recovery-use-directive` witness needs 11 production rewrites.",
			"The forest route accepted two witnesses and declined nine witnesses.",
			"Keep `dispatch.authzed` live until a producer change closes every recorded divergence.",
			"Require the authenticated Authzed corpus and its source lock.",
		},
		"../CHANGELOG.md": {
			"Recorded the N31f Authzed dispatcher blocker at evidence base",
			"and publication base",
			"Keep `dispatch.authzed` live.",
			"A0 lists three Authzed files, three checked files, three run files",
			"The refreshed route probe reports 17 rewrites",
			"The authenticated corpus and its source lock are unavailable.",
		},
	}
	for path, markers := range documents {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, marker := range markers {
			if !strings.Contains(strings.Join(strings.Fields(text), " "), strings.Join(strings.Fields(marker), " ")) {
				t.Fatalf("%s is missing receipt marker %q", path, marker)
			}
		}
	}
}
