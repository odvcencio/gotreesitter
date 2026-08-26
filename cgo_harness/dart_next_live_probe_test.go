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

type dartNextWitness struct {
	name                     string
	source                   []byte
	sourceSHA256             string
	cDigest                  string
	rawDigest                string
	productionDigest         string
	compactDigest            string
	forestDigest             string
	incrementalDigest        string
	rawRewrites              uint64
	productionRewrites       uint64
	compactRewrites          uint64
	forestRewrites           uint64
	incrementalRewrites      uint64
	rawDivergencePath        string
	rawDivergenceCategory    string
	productionDivergencePath string
	productionDivergenceCat  string
	compactFallback          bool
	compactDispatchPass      bool
	forestAccepted           bool
	incrementalReuse         bool
	incrementalUnsupported   bool
	malformed                bool
}

var dartNextWitnesses = []dartNextWitness{
	{
		name:                     "single-type-free-call",
		source:                   []byte("class CancelToken {\n  final _token = calloc<Size>(1);\n}\n"),
		sourceSHA256:             "09d754270caf75b1ff52126fcbe2ed8cccaf34cb05fbda8ba24b042f01297e51",
		cDigest:                  "4d1969523ca9d897b2d5154740cc3b34372ee6e8b100384cbec327eef80e5f26",
		rawDigest:                "4592ebedc702c08c6694e1631a31acdb1a9aedd068110c2961194f897d28de58",
		productionDigest:         "4592ebedc702c08c6694e1631a31acdb1a9aedd068110c2961194f897d28de58",
		compactDigest:            "4592ebedc702c08c6694e1631a31acdb1a9aedd068110c2961194f897d28de58",
		forestDigest:             "4d1969523ca9d897b2d5154740cc3b34372ee6e8b100384cbec327eef80e5f26",
		incrementalDigest:        "4592ebedc702c08c6694e1631a31acdb1a9aedd068110c2961194f897d28de58",
		forestRewrites:           10,
		rawDivergencePath:        "/program/class_definition[0]/class_body[2]/declaration[1]/initialized_identifier_list[1]/initialized_identifier[0]/relational_expression[2]/identifier[0]",
		rawDivergenceCategory:    "type",
		productionDivergencePath: "/program/class_definition[0]/class_body[2]/declaration[1]/initialized_identifier_list[1]/initialized_identifier[0]/relational_expression[2]/identifier[0]",
		productionDivergenceCat:  "type",
		compactFallback:          true,
		compactDispatchPass:      true,
		forestAccepted:           true,
		incrementalReuse:         true,
	},
	{
		name:                  "complex-void-function-type-call",
		source:                []byte("base class Parser implements Finalizable {\n  late final p = _lookup<ffi.NativeFunction<ffi.Void Function(ffi.Pointer<TSParser>, TSLogger)>>('ts_parser_set_logger');\n}\n"),
		sourceSHA256:          "98073341a8cdc5dfb052f667072fa968141cd1538852c9143db2619991971210",
		cDigest:               "f06b1f0088eead7eb346506de36942dfaea6d9a1fd2089d23ac683cb620eddac",
		rawDigest:             "c2865b8258531e7d5caaa5b79ea036ab1cfd64518328f627f72713b1707f41ab",
		productionDigest:      "f06b1f0088eead7eb346506de36942dfaea6d9a1fd2089d23ac683cb620eddac",
		compactDigest:         "f06b1f0088eead7eb346506de36942dfaea6d9a1fd2089d23ac683cb620eddac",
		forestDigest:          "f06b1f0088eead7eb346506de36942dfaea6d9a1fd2089d23ac683cb620eddac",
		incrementalDigest:     "f06b1f0088eead7eb346506de36942dfaea6d9a1fd2089d23ac683cb620eddac",
		productionRewrites:    4,
		compactRewrites:       4,
		incrementalRewrites:   4,
		rawDivergencePath:     "/program/class_definition[0]/class_body[4]/declaration[1]/initialized_identifier_list[2]/initialized_identifier[0]/relational_expression[2]/identifier[0]",
		rawDivergenceCategory: "type",
		compactFallback:       true,
		compactDispatchPass:   true,
		forestAccepted:        true,
		incrementalReuse:      true,
	},
	{
		name:                  "nested-function-type-call",
		source:                []byte("base class Parser implements Finalizable {\n  late final p = _lookup<ffi.NativeFunction<TSSymbol Function(ffi.Pointer<TSLanguage>, ffi.Pointer<ffi.Char>, ffi.Uint32, ffi.Bool)>>('ts_language_symbol_for_name');\n}\n"),
		sourceSHA256:          "9fc3f24b921fa53d0e66ebe1b7ba216a7e4ac7e4b5bfb23688c3b4ca442bbb0f",
		cDigest:               "fe747a4e459dd83d2c02d7d21d4e3d608367fd03e2609986464a1fcaf2f94760",
		rawDigest:             "adae85234671a704c4b9efd5a6c3fcfc6f445db4cdfca6f505b8a3f58357a84c",
		productionDigest:      "fe747a4e459dd83d2c02d7d21d4e3d608367fd03e2609986464a1fcaf2f94760",
		compactDigest:         "fe747a4e459dd83d2c02d7d21d4e3d608367fd03e2609986464a1fcaf2f94760",
		forestDigest:          "fe747a4e459dd83d2c02d7d21d4e3d608367fd03e2609986464a1fcaf2f94760",
		incrementalDigest:     "fe747a4e459dd83d2c02d7d21d4e3d608367fd03e2609986464a1fcaf2f94760",
		productionRewrites:    4,
		compactRewrites:       4,
		incrementalRewrites:   4,
		rawDivergencePath:     "/program/class_definition[0]/class_body[4]/declaration[1]/initialized_identifier_list[2]/initialized_identifier[0]/relational_expression[2]/identifier[0]",
		rawDivergenceCategory: "type",
		compactFallback:       true,
		compactDispatchPass:   true,
		forestAccepted:        true,
		incrementalReuse:      true,
	},
	{
		name:                     "complex-generic-return-call",
		source:                   []byte("base class Parser implements Finalizable {\n  late final p = _lookup<ffi.NativeFunction<ffi.Pointer<TSLanguage> Function(ffi.Pointer<TSParser>)>>('ts_parser_language');\n}\n"),
		sourceSHA256:             "a40a05fc1cdace3bd60fa334475c12ecfe1e804c1fca55612483addf9cb3059a",
		cDigest:                  "03aac429467894dc376a66896c792e92a5236ec403cffa8517cffb8c929b1dfc",
		rawDigest:                "b424eb39c84e034d2dd3acdf3522f452e0517cccea06da36c0ba4b39dbd7e0ae",
		productionDigest:         "b424eb39c84e034d2dd3acdf3522f452e0517cccea06da36c0ba4b39dbd7e0ae",
		compactDigest:            "b424eb39c84e034d2dd3acdf3522f452e0517cccea06da36c0ba4b39dbd7e0ae",
		forestDigest:             "03aac429467894dc376a66896c792e92a5236ec403cffa8517cffb8c929b1dfc",
		incrementalDigest:        "b424eb39c84e034d2dd3acdf3522f452e0517cccea06da36c0ba4b39dbd7e0ae",
		forestRewrites:           44,
		rawDivergencePath:        "/program/class_definition[0]/class_body[4]/declaration[1]/initialized_identifier_list[2]/initialized_identifier[0]",
		rawDivergenceCategory:    "shape",
		productionDivergencePath: "/program/class_definition[0]/class_body[4]/declaration[1]/initialized_identifier_list[2]/initialized_identifier[0]",
		productionDivergenceCat:  "shape",
		compactFallback:          true,
		compactDispatchPass:      true,
		forestAccepted:           true,
		incrementalReuse:         true,
	},
	{
		name:              "class-constructor",
		source:            []byte("base class QueryCursor {\n  QueryCursor() {}\n}\n"),
		sourceSHA256:      "af8089b2a7696122fc2d2a8197469c9b484a0dded579f7b3bd4275d5d9b2b164",
		cDigest:           "4f0ac97990578f9eb695a85db58d310c9674ca58c702a22937bcb224ea95f0b1",
		rawDigest:         "4f0ac97990578f9eb695a85db58d310c9674ca58c702a22937bcb224ea95f0b1",
		productionDigest:  "4f0ac97990578f9eb695a85db58d310c9674ca58c702a22937bcb224ea95f0b1",
		compactDigest:     "4f0ac97990578f9eb695a85db58d310c9674ca58c702a22937bcb224ea95f0b1",
		forestDigest:      "4f0ac97990578f9eb695a85db58d310c9674ca58c702a22937bcb224ea95f0b1",
		incrementalDigest: "4f0ac97990578f9eb695a85db58d310c9674ca58c702a22937bcb224ea95f0b1",
		forestAccepted:    true,
		incrementalReuse:  true,
	},
	{
		name:              "private-constructor",
		source:            []byte("class _SymbolAddresses {\n  final TreeSitter _library;\n  _SymbolAddresses(this._library);\n}\n"),
		sourceSHA256:      "c9fbb9bddc9aa395934e00a3a45df2bab56b1285584876cca563e17f8e94197d",
		cDigest:           "b5f6fcdde084e50cfc8dd4fce75514f5a632e52130e52a70c411bf42c89366d8",
		rawDigest:         "b5f6fcdde084e50cfc8dd4fce75514f5a632e52130e52a70c411bf42c89366d8",
		productionDigest:  "b5f6fcdde084e50cfc8dd4fce75514f5a632e52130e52a70c411bf42c89366d8",
		compactDigest:     "b5f6fcdde084e50cfc8dd4fce75514f5a632e52130e52a70c411bf42c89366d8",
		forestDigest:      "b5f6fcdde084e50cfc8dd4fce75514f5a632e52130e52a70c411bf42c89366d8",
		incrementalDigest: "b5f6fcdde084e50cfc8dd4fce75514f5a632e52130e52a70c411bf42c89366d8",
		forestAccepted:    true,
		incrementalReuse:  true,
	},
	{
		name:              "enum-constructor",
		source:            []byte("enum LogPriority { warning; LogPriority(this.priority, this.prefix); final int priority; final String prefix; }\n"),
		sourceSHA256:      "5cf4ddaf26fd35854bcd5b41a5bbf0a6ed0bb1717cf98bed6825adda041f3f7c",
		cDigest:           "ac3f1a92f0ec55fe7862e61bde3b92cfab1a4f393dbfab9280c10d5243902285",
		rawDigest:         "ac3f1a92f0ec55fe7862e61bde3b92cfab1a4f393dbfab9280c10d5243902285",
		productionDigest:  "ac3f1a92f0ec55fe7862e61bde3b92cfab1a4f393dbfab9280c10d5243902285",
		compactDigest:     "ac3f1a92f0ec55fe7862e61bde3b92cfab1a4f393dbfab9280c10d5243902285",
		forestDigest:      "ac3f1a92f0ec55fe7862e61bde3b92cfab1a4f393dbfab9280c10d5243902285",
		incrementalDigest: "ac3f1a92f0ec55fe7862e61bde3b92cfab1a4f393dbfab9280c10d5243902285",
		forestAccepted:    true,
		incrementalReuse:  true,
	},
	{
		name:              "relational-control",
		source:            []byte("class C {\n  final b = 1 < 2;\n}\n"),
		sourceSHA256:      "54b13169d4e0b73846307abcd87184c19e4c0304e9ce077e461d7e1d9460a8a3",
		cDigest:           "8545e51b499fd18fe3aa153fcdeef2873a2637567ad4fe6a87a9ec85adc186b0",
		rawDigest:         "8545e51b499fd18fe3aa153fcdeef2873a2637567ad4fe6a87a9ec85adc186b0",
		productionDigest:  "8545e51b499fd18fe3aa153fcdeef2873a2637567ad4fe6a87a9ec85adc186b0",
		compactDigest:     "8545e51b499fd18fe3aa153fcdeef2873a2637567ad4fe6a87a9ec85adc186b0",
		forestDigest:      "8545e51b499fd18fe3aa153fcdeef2873a2637567ad4fe6a87a9ec85adc186b0",
		incrementalDigest: "8545e51b499fd18fe3aa153fcdeef2873a2637567ad4fe6a87a9ec85adc186b0",
		forestAccepted:    true,
		incrementalReuse:  true,
	},
	{
		name:                "library-recovery",
		source:              []byte("library;\n"),
		sourceSHA256:        "09c63fb57f8540f571c1defda4fbdc59ec9ec1cdfe3c3e23a1613c083abc04e7",
		cDigest:             "324de0a5a06943713b7a9346b4029a8d35f796a8144f2f748c78afcd7662e5f9",
		rawDigest:           "324de0a5a06943713b7a9346b4029a8d35f796a8144f2f748c78afcd7662e5f9",
		productionDigest:    "324de0a5a06943713b7a9346b4029a8d35f796a8144f2f748c78afcd7662e5f9",
		compactDigest:       "324de0a5a06943713b7a9346b4029a8d35f796a8144f2f748c78afcd7662e5f9",
		incrementalDigest:   "324de0a5a06943713b7a9346b4029a8d35f796a8144f2f748c78afcd7662e5f9",
		compactFallback:     true,
		compactDispatchPass: true,
		incrementalReuse:    true,
		malformed:           true,
	},
}

// TestDartNextLiveArmProbe records Dart normalization on every required route.
func TestDartNextLiveArmProbe(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := grammars.DartLanguage()
	if language.ExternalScanner == nil {
		t.Fatal("Dart language has no registered external scanner")
	}
	reusable, ok := language.ExternalScanner.(gotreesitter.IncrementalReuseExternalScanner)
	if !ok || !reusable.SupportsIncrementalReuse() {
		t.Fatal("Dart external scanner does not support incremental reuse")
	}
	cLanguage, err := COracleLanguage("dart")
	if err != nil {
		t.Fatal(err)
	}

	for _, witness := range dartNextWitnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			sourceSHA := fmt.Sprintf("%x", sha256.Sum256(witness.source))
			if sourceSHA != witness.sourceSHA256 {
				t.Fatalf("source SHA-256=%s, want %s", sourceSHA, witness.sourceSHA256)
			}
			t.Logf("witness=%s malformed=%t bytes=%d source_sha256=%s", witness.name, witness.malformed, len(witness.source), sourceSHA)
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(witness.source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parser returned no root")
			}
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			if cDigest != witness.cDigest {
				t.Fatalf("locked C digest=%s, want %s", cDigest, witness.cDigest)
			}

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(witness.source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(raw.Release)
			assertDartNextRoute(t, "raw", raw, language, cTree, cDigest, witness.rawDigest, witness.rawRewrites, witness.rawDivergencePath, witness.rawDivergenceCategory, false)
			t.Logf("route=raw %s", dartNextReceipt(raw, language, cTree, cDigest))

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(witness.source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			assertDartNextRoute(t, "production", production, language, cTree, cDigest, witness.productionDigest, witness.productionRewrites, witness.productionDivergencePath, witness.productionDivergenceCat, true)
			t.Logf("route=production %s", dartNextReceipt(production, language, cTree, cDigest))

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(witness.source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(compact.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			if witness.compactFallback {
				if routedAfter != routedBefore || fallbackAfter != fallbackBefore+1 {
					t.Fatalf("compact fallback transition=%d/%d->%d/%d, want routed unchanged and fallback +1", routedBefore, fallbackBefore, routedAfter, fallbackAfter)
				}
			} else if routedAfter != routedBefore+1 || fallbackAfter != fallbackBefore {
				t.Fatalf("compact accepted transition=%d/%d->%d/%d, want routed +1 and fallback unchanged", routedBefore, fallbackBefore, routedAfter, fallbackAfter)
			}
			assertDartNextRoute(t, "compact", compact, language, cTree, cDigest, witness.compactDigest, witness.compactRewrites, witness.productionDivergencePath, witness.productionDivergenceCat, witness.compactDispatchPass)
			compactMode := fmt.Sprintf("counters=%d/%d->%d/%d", routedBefore, fallbackBefore, routedAfter, fallbackAfter)
			if routedAfter == routedBefore && fallbackAfter == fallbackBefore+1 {
				compactMode += " fallback:" + gotreesitter.AdmissionCandidateLastFallbackReason()
			} else if routedAfter == routedBefore+1 && fallbackAfter == fallbackBefore {
				compactMode += " accepted"
			}
			t.Logf("route=compact mode=%s %s", compactMode, dartNextReceipt(compact, language, cTree, cDigest))

			forestParser := gotreesitter.NewParser(language)
			forest, forestOK := forestParser.ParseForestExperimental(witness.source)
			if forestOK != witness.forestAccepted {
				if forest != nil {
					forest.Release()
				}
				t.Fatalf("forest accepted=%t, want %t", forestOK, witness.forestAccepted)
			}
			if forestOK {
				if forest == nil {
					t.Fatal("forest accepted=true returned nil tree")
				}
				t.Cleanup(forest.Release)
				assertDartNextRoute(t, "forest", forest, language, cTree, cDigest, witness.forestDigest, witness.forestRewrites, "", "", true)
				t.Logf("route=forest mode=accepted %s", dartNextReceipt(forest, language, cTree, cDigest))
			} else {
				if forest != nil {
					forest.Release()
					t.Fatal("forest accepted=false returned a non-nil tree")
				}
				t.Log("route=forest mode=declined")
			}

			incrementalParser := gotreesitter.NewParser(language)
			incrementalParser.SetAdmissionCandidateRoute(false)
			base := bytes.TrimSuffix(witness.source, []byte{'\n'})
			oldTree, err := incrementalParser.Parse(base)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(oldTree.Release)
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(base)),
				OldEndByte:  uint32(len(base)),
				NewEndByte:  uint32(len(witness.source)),
				StartPoint:  dartNextPointAtByte(base),
				OldEndPoint: dartNextPointAtByte(base),
				NewEndPoint: dartNextPointAtByte(witness.source),
			})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(witness.source, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(incremental.Release)
			if profile.OldTreeReuseRoute != witness.incrementalReuse || profile.ReuseUnsupported != witness.incrementalUnsupported {
				t.Fatalf("incremental reuse=%t unsupported=%t, want reuse=%t unsupported=%t (reason=%q)", profile.OldTreeReuseRoute, profile.ReuseUnsupported, witness.incrementalReuse, witness.incrementalUnsupported, profile.ReuseUnsupportedReason)
			}
			if profile.OldTreeReuseRoute && (profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0) {
				t.Fatalf("incremental reuse route reported no material reuse: subtrees=%d bytes=%d", profile.ReusedSubtrees, profile.ReusedBytes)
			}
			assertDartNextRoute(t, "incremental", incremental, language, cTree, cDigest, witness.incrementalDigest, witness.incrementalRewrites, witness.productionDivergencePath, witness.productionDivergenceCat, true)
			t.Logf("route=incremental reuse=%t unsupported=%t reason=%q reused_subtrees=%d reused_bytes=%d %s", profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason, profile.ReusedSubtrees, profile.ReusedBytes, dartNextReceipt(incremental, language, cTree, cDigest))
		})
	}
}

func assertDartNextRoute(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, cDigest, wantDigest string, wantRewrites uint64, wantDivergencePath, wantDivergenceCategory string, wantDispatchPass bool) {
	t.Helper()
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("%s returned no tree", route)
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatalf("%s inspect Go tree: %v", route, err)
	}
	if inspection.SHA256 != wantDigest {
		t.Fatalf("%s digest=%s, want %s", route, inspection.SHA256, wantDigest)
	}
	if tree.ParseRuntime().NativeRecoveredStructureAuthoritative {
		t.Fatalf("%s native recovered structure authoritative=true, want false", route)
	}
	assertDartNextPass(t, route, tree, wantDispatchPass, wantRewrites)
	diff := FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode())
	if wantDivergencePath == "" {
		if diff != nil {
			t.Fatalf("%s unexpected locked-C divergence: %+v", route, diff)
		}
		if inspection.SHA256 != cDigest {
			t.Fatalf("%s digest=%s, C=%s", route, inspection.SHA256, cDigest)
		}
		return
	}
	if diff == nil {
		t.Fatalf("%s unexpectedly matches locked C", route)
	}
	if diff.Path != wantDivergencePath || diff.Category != wantDivergenceCategory {
		t.Fatalf("%s divergence path/category=%s/%s, want %s/%s", route, diff.Path, diff.Category, wantDivergencePath, wantDivergenceCategory)
	}
}

func assertDartNextPass(t *testing.T, route string, tree *gotreesitter.Tree, wantPass bool, wantRewrites uint64) {
	t.Helper()
	runtime := tree.ParseRuntime()
	var found *gotreesitter.NormalizationPassRuntime
	if runtime.NormalizationPasses != nil {
		for i := range *runtime.NormalizationPasses {
			pass := &(*runtime.NormalizationPasses)[i]
			if pass.Name == "dispatch.dart" {
				if found != nil {
					t.Fatalf("%s recorded duplicate dispatch.dart passes", route)
				}
				found = pass
			}
		}
	}
	if !wantPass {
		if found != nil {
			t.Fatalf("%s recorded dispatch.dart pass=%d/%d/%d/%d, want no pass", route, found.Checked, found.Run, found.NodesVisited, found.NodesRewritten)
		}
		return
	}
	if found == nil {
		t.Fatalf("%s did not record dispatch.dart", route)
	}
	if found.Checked != 1 || found.Run != 1 || found.NodesRewritten != wantRewrites {
		t.Fatalf("%s dispatch.dart pass checked/run/visited/rewritten=%d/%d/%d/%d, want 1/1/diagnostic/%d", route, found.Checked, found.Run, found.NodesVisited, found.NodesRewritten, wantRewrites)
	}
}

func dartNextReceipt(tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, cDigest string) string {
	if tree == nil || tree.RootNode() == nil {
		return "tree=nil"
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		return fmt.Sprintf("inspect_error=%v", err)
	}
	diff := FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode())
	return fmt.Sprintf("error_root=%t digest=%s c_digest=%s exact=%t rewrites=%d native_authoritative=%t passes=%s divergence=%+v", tree.RootNode().HasError(), inspection.SHA256, cDigest, diff == nil && inspection.SHA256 == cDigest, tree.ParseRuntime().NormalizationNodesRewritten, tree.ParseRuntime().NativeRecoveredStructureAuthoritative, dartNextPasses(tree), diff)
}

func dartNextPasses(tree *gotreesitter.Tree) string {
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

func dartNextPointAtByte(source []byte) gotreesitter.Point {
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

// TestDartNextLiveArmReceiptDocument guards the Dart blocker receipt.
func TestDartNextLiveArmReceiptDocument(t *testing.T) {
	raw, err := os.ReadFile("../docs/root-normalization-retirement.md")
	if err != nil {
		t.Fatal(err)
	}
	document := strings.Join(strings.Fields(string(raw)), " ")
	markers := []string{
		"The Dart arm remains live.",
		"Evidence base: `7b6f40fe089283674f5d0d19408d2380f77caf68`.",
		"Publication base: `09cb5faa41af35a6bc84fefccbab1a17850d38cc`.",
		"The grammar lock SHA-256 is `9ddb6324afd014f6ecdd1cae3dd1ba238f1e62ce03d126e6d8b267ce34d72ecb`.",
		"A0 means the initial dispatcher census.",
		"A0 has 14 languages and 42 files.",
		"The tracked census has seven fixtures across six languages.",
		"The authenticated corpus is unavailable because `cgo_harness/corpus_real` and `cgo_harness/perf_scan/corpus_sources.lock` are absent.",
		"The focused probe uses eight clean witnesses and one malformed recovery witness.",
		"The single-type witness differs from locked C on raw, production, compact, and incremental routes.",
		"The forest route rewrites 10 nodes and matches C.",
		"The generic-return witness differs from C on raw, production, compact, and incremental routes.",
		"The forest route rewrites 44 nodes and matches C.",
		"The void and nested function-type witnesses differ from C on raw only.",
		"The relational control compact route is accepted without a `dispatch.dart` pass.",
		"The malformed `library;` witness matches C on raw, production, compact, and incremental routes.",
		"The Dart external scanner is present and supports incremental reuse.",
		"Keep `dispatch.dart` live.",
		"Do not add a Dart-specific parser branch.",
	}
	for _, marker := range markers {
		marker = strings.Join(strings.Fields(marker), " ")
		if !strings.Contains(document, marker) {
			t.Errorf("document is missing marker %q", marker)
		}
	}
	changelogRaw, err := os.ReadFile("../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	changelog := strings.Join(strings.Fields(string(changelogRaw)), " ")
	for _, marker := range []string{
		"Recorded the N31h Dart dispatcher blocker at evidence base `7b6f40fe089283674f5d0d19408d2380f77caf68` and publication base `09cb5faa41af35a6bc84fefccbab1a17850d38cc`.",
		"Keep `dispatch.dart` live.",
		"No safe shared producer invariant was identified.",
	} {
		marker = strings.Join(strings.Fields(marker), " ")
		if !strings.Contains(changelog, marker) {
			t.Errorf("changelog is missing marker %q", marker)
		}
	}
}
