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

type hlslNextWitness struct {
	name                 string
	source               []byte
	wantError            bool
	wantForest           bool
	wantRewrite          bool
	wantRawDigest        string
	wantNormalizedDigest string
	wantDivergence       *DumpV1Divergence
	wantNormalizedDiff   *DumpV1Divergence
}

// TestHLSLNextLiveArmLockedCRoutes records the HLSL arm on all five routes.
// It keeps the producer gap and malformed recovery gap visible.
func TestHLSLNextLiveArmLockedCRoutes(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	t.Setenv("GOT_PARSE_PHASE_TIMING", "1")
	goLanguage := grammars.HlslLanguage()
	cLanguage, err := COracleLanguage("hlsl")
	if err != nil {
		t.Fatal(err)
	}

	witnesses := []hlslNextWitness{
		{
			name:                 "cast-negative-number",
			source:               []byte("float4 main() : SV_Target { return (float4)-1; }\n"),
			wantForest:           true,
			wantRewrite:          true,
			wantRawDigest:        "e5d64d87ea862f2e28d3f53dd6bf53dd21936ee79496c9137d2fa944171eb1ca",
			wantNormalizedDigest: "c6895007d3b8523d87aa77eadf1123f1e96c60def279c9ce77f506609f74334f",
			wantDivergence: &DumpV1Divergence{
				Path:     "/translation_unit/function_definition[0]/compound_statement[2]/return_statement[1]/cast_expression[1]",
				Category: "type",
				GoValue:  "cast_expression",
				CValue:   "binary_expression",
			},
			wantNormalizedDiff: &DumpV1Divergence{
				Path:     "/translation_unit/function_definition[0]/compound_statement[2]/return_statement[1]/binary_expression[1]/parenthesized_expression[0]",
				Category: "field",
				GoValue:  "",
				CValue:   "left",
			},
		},
		{
			name:                 "cast-negative-number-malformed",
			source:               []byte("float4 main() : SV_Target { return (float4)-; }\n"),
			wantError:            true,
			wantRawDigest:        "4dd53dee79720e7209730e1eacd5038021517caa5548f0b25fac9189ecbcf97e",
			wantNormalizedDigest: "4dd53dee79720e7209730e1eacd5038021517caa5548f0b25fac9189ecbcf97e",
			wantDivergence: &DumpV1Divergence{
				Path:     "/translation_unit/function_definition[0]/compound_statement[2]/return_statement[1]/cast_expression[1]",
				Category: "type",
				GoValue:  "cast_expression",
				CValue:   "binary_expression",
			},
			wantNormalizedDiff: &DumpV1Divergence{
				Path:     "/translation_unit/function_definition[0]/compound_statement[2]/return_statement[1]/cast_expression[1]",
				Category: "type",
				GoValue:  "cast_expression",
				CValue:   "binary_expression",
			},
		},
		{
			name:                 "unorm-buffer-control",
			source:               []byte("buffer<unorm::float4> value...;\n"),
			wantRawDigest:        "8cbb8f76171423dfab0b254e1602c4619a85b1f0ab7184edfadbad3b886ad647",
			wantNormalizedDigest: "8cbb8f76171423dfab0b254e1602c4619a85b1f0ab7184edfadbad3b886ad647",
		},
		{
			name:                 "unorm-buffer-malformed",
			source:               []byte("buffer<unorm float4> value...;\n"),
			wantError:            true,
			wantRawDigest:        "05b2bf7a42e5c0ebbcf90d1ed35fddff902e995f97ce767404941a6299d337d9",
			wantNormalizedDigest: "05b2bf7a42e5c0ebbcf90d1ed35fddff902e995f97ce767404941a6299d337d9",
			wantDivergence: &DumpV1Divergence{
				Path:     "/translation_unit/expression_statement[0]",
				Category: "type",
				GoValue:  "expression_statement",
				CValue:   "declaration",
			},
			wantNormalizedDiff: &DumpV1Divergence{
				Path:     "/translation_unit/expression_statement[0]",
				Category: "type",
				GoValue:  "expression_statement",
				CValue:   "declaration",
			},
		},
		{
			name:                 "subscript-assignment-control",
			source:               []byte("void f() { Foo[bar] = value; }\n"),
			wantForest:           true,
			wantRawDigest:        "688445ba93476a6948684073b044cc8cf3dd13cbf4fbe889681221e12b774737",
			wantNormalizedDigest: "688445ba93476a6948684073b044cc8cf3dd13cbf4fbe889681221e12b774737",
		},
		{
			name:                 "subscript-assignment-top-level-control",
			source:               []byte("Foo[bar] = value;\n"),
			wantRawDigest:        "55f06b3603d46533fa00b5bdb6b586df5f6999aad99829764c05140bb1335266",
			wantNormalizedDigest: "55f06b3603d46533fa00b5bdb6b586df5f6999aad99829764c05140bb1335266",
		},
	}

	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
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
			raw := hlslParseRoute(t, goLanguage, witness.source, "raw", func(p *gotreesitter.Parser, source []byte) (*gotreesitter.Tree, error) {
				return p.ParseNoResultCompatibilityBenchmarkOnly(source)
			})
			production := hlslParseRoute(t, goLanguage, witness.source, "production", func(p *gotreesitter.Parser, source []byte) (*gotreesitter.Tree, error) {
				return p.Parse(source)
			})

			assertHLSLNextRoute(t, "raw", raw, goLanguage, cTree, cDigest, witness.wantError, witness.wantRawDigest, witness.wantDivergence, witness.wantRewrite, false)
			assertHLSLNextRoute(t, "production", production, goLanguage, cTree, cDigest, witness.wantError, witness.wantNormalizedDigest, witness.wantNormalizedDiff, witness.wantRewrite, true)

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(goLanguage)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(witness.source)
			if err != nil {
				t.Fatalf("compact parse: %v", err)
			}
			t.Cleanup(compact.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			compactRoute := "accepted"
			if fallbackAfter > fallbackBefore {
				compactRoute = "fallback:" + gotreesitter.AdmissionCandidateLastFallbackReason()
			}
			assertHLSLNextRoute(t, "compact", compact, goLanguage, cTree, cDigest, witness.wantError, witness.wantNormalizedDigest, witness.wantNormalizedDiff, witness.wantRewrite, true)

			forestParser := gotreesitter.NewParser(goLanguage)
			forest, forestOK := forestParser.ParseForestExperimental(witness.source)
			forestRoute := "declined"
			if forestOK && forest != nil {
				forestRoute = "accepted"
				t.Cleanup(forest.Release)
				assertHLSLNextRoute(t, "forest", forest, goLanguage, cTree, cDigest, witness.wantError, witness.wantNormalizedDigest, witness.wantNormalizedDiff, witness.wantRewrite, true)
			} else if witness.wantForest {
				t.Fatal("forest route declined for a required witness")
			}

			base := bytes.TrimSuffix(witness.source, []byte{'\n'})
			incrementalParser := gotreesitter.NewParser(goLanguage)
			oldTree, err := incrementalParser.Parse(base)
			if err != nil {
				t.Fatalf("incremental base parse: %v", err)
			}
			t.Cleanup(oldTree.Release)
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(base)),
				OldEndByte:  uint32(len(base)),
				NewEndByte:  uint32(len(witness.source)),
				StartPoint:  hlslPointAtByte(base),
				OldEndPoint: hlslPointAtByte(base),
				NewEndPoint: hlslPointAtByte(witness.source),
			})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(witness.source, oldTree)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			t.Cleanup(incremental.Release)
			assertHLSLNextRoute(t, "incremental", incremental, goLanguage, cTree, cDigest, witness.wantError, witness.wantNormalizedDigest, witness.wantNormalizedDiff, witness.wantRewrite, true)

			t.Logf("witness=%s bytes=%d source_sha256=%x c_digest=%s compact=%s counters=%d/%d->%d/%d forest=%s incremental_reuse=%t incremental_fallback=%t reason=%q raw_rewrite=%t production_rewrite=%t compact_rewrite=%t forest_rewrite=%t incremental_rewrite=%t", witness.name, len(witness.source), sha256.Sum256(witness.source), cDigest, compactRoute, routedBefore, fallbackBefore, routedAfter, fallbackAfter, forestRoute, profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason, witness.wantRewrite && hasHLSLCastRewrite(raw.RootNode(), goLanguage), witness.wantRewrite && hasHLSLCastRewrite(production.RootNode(), goLanguage), witness.wantRewrite && hasHLSLCastRewrite(compact.RootNode(), goLanguage), witness.wantRewrite && forest != nil && hasHLSLCastRewrite(forest.RootNode(), goLanguage), witness.wantRewrite && hasHLSLCastRewrite(incremental.RootNode(), goLanguage))
		})
	}
}

// TestHLSLNextLiveArmReceiptDocument guards the blocker receipt markers.
func TestHLSLNextLiveArmReceiptDocument(t *testing.T) {
	raw, err := os.ReadFile("../docs/root-normalization-retirement.md")
	if err != nil {
		t.Fatal(err)
	}
	document := strings.Join(strings.Fields(string(raw)), " ")
	for _, marker := range []string{
		"The HLSL arm remains live.",
		"A0 has three HLSL files, three checked, three run, and zero rewrites.",
		"The clean cast witness rewrites one cast_expression node on production, compact, forest, and incremental routes.",
		"The normalized routes diverge from locked C at the missing left field.",
		"Keep dispatch.hlsl live until scheduler_action_semantics emits the C field.",
	} {
		marker = strings.Join(strings.Fields(marker), " ")
		if !strings.Contains(document, marker) {
			t.Fatalf("HLSL blocker receipt lacks marker %q", marker)
		}
	}
}

func hlslParseRoute(t *testing.T, language *gotreesitter.Language, source []byte, route string, parse func(*gotreesitter.Parser, []byte) (*gotreesitter.Tree, error)) *gotreesitter.Tree {
	t.Helper()
	parser := gotreesitter.NewParser(language)
	tree, err := parse(parser, source)
	if err != nil {
		t.Fatalf("%s parse: %v", route, err)
	}
	t.Cleanup(tree.Release)
	return tree
}

func assertHLSLNextRoute(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, cDigest string, wantError bool, wantDigest string, wantDiff *DumpV1Divergence, wantRewrite bool, normalized bool) {
	t.Helper()
	root := tree.RootNode()
	if root == nil || root.HasError() != wantError {
		t.Fatalf("%s root error=%t, want %t", route, root != nil && root.HasError(), wantError)
	}
	inspection, err := benchfixtures.InspectGoTree(root, language)
	if err != nil {
		t.Fatalf("%s inspect Go tree: %v", route, err)
	}
	if inspection.SHA256 != wantDigest {
		t.Fatalf("%s Go digest=%s, want %s", route, inspection.SHA256, wantDigest)
	}
	diff := FirstDivergenceDumpV1(root, language, cTree.RootNode())
	if wantDiff == nil {
		if diff != nil {
			t.Fatalf("%s unexpected locked-C divergence: %+v", route, diff)
		}
		if inspection.SHA256 != cDigest {
			t.Fatalf("%s exact tree digest=%s, C=%s", route, inspection.SHA256, cDigest)
		}
	} else if diff == nil || *diff != *wantDiff {
		t.Fatalf("%s divergence=%+v, want %+v", route, diff, wantDiff)
	}
	castCount := countHLSLNodes(root, language, "cast_expression")
	binaryCount := countHLSLNodes(root, language, "binary_expression")
	if wantRewrite {
		if normalized && castCount != 0 {
			t.Fatalf("%s retained %d cast_expression nodes after the live rewrite", route, castCount)
		}
		if !normalized && castCount == 0 {
			t.Fatalf("%s lost the raw cast_expression before the live rewrite", route)
		}
		if normalized && binaryCount == 0 {
			t.Fatalf("%s lacks the normalized binary_expression", route)
		}
	}
	t.Logf("route=%s error=%t digest=%s c_digest=%s divergence=%v cast_nodes=%d binary_nodes=%d dispatch=%s", route, root.HasError(), inspection.SHA256, cDigest, diff, castCount, binaryCount, hlslDispatchReceipt(tree))
}

func countHLSLNodes(root *gotreesitter.Node, language *gotreesitter.Language, typ string) int {
	if root == nil {
		return 0
	}
	count := 0
	var walk func(*gotreesitter.Node)
	walk = func(node *gotreesitter.Node) {
		if node == nil {
			return
		}
		if node.Type(language) == typ {
			count++
		}
		for i := 0; i < node.ChildCount(); i++ {
			walk(node.Child(i))
		}
	}
	walk(root)
	return count
}

func hasHLSLCastRewrite(root *gotreesitter.Node, language *gotreesitter.Language) bool {
	return countHLSLNodes(root, language, "cast_expression") == 0 && countHLSLNodes(root, language, "binary_expression") > 0
}

func hlslDispatchReceipt(tree *gotreesitter.Tree) string {
	if tree == nil {
		return "none"
	}
	runtime := tree.ParseRuntime()
	if runtime.NormalizationPasses == nil {
		return "none"
	}
	for _, pass := range *runtime.NormalizationPasses {
		if pass.Name == "dispatch.hlsl" {
			return fmt.Sprintf("%d/%d/%d/%d", pass.Checked, pass.Run, pass.NodesVisited, pass.NodesRewritten)
		}
	}
	return "none"
}

func hlslPointAtByte(source []byte) gotreesitter.Point {
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
