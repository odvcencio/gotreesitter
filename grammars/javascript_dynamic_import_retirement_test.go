//go:build !grammar_subset

package grammars

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

const (
	javaScriptDynamicImportPositiveControlCommit = "143936b2a62b44eb779dda835b09abe0c26cc6d5"
	javaScriptDynamicImportProducerFixCommit     = "eee20b8a54a1608b47cce0ab7fb934651e204d66"
	javaScriptDynamicImportBlobSHA256            = "6706f93890f24d8ea90d6a140df5dde29c02ec8a3213bae16e8cc4df37e33ee0"
)

func TestJavaScriptDynamicImportRetirementRoutes(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	tests := []struct {
		name       string
		source     []byte
		sha256     string
		deepDigest string
		baseSource []byte
	}{
		{
			name:       "call",
			source:     []byte("import(\"foo\");\n"),
			sha256:     "d8df58099982cfa08eb81abba34eeff697541da9a833e24201212a446c3dfc32",
			deepDigest: "b1901d0041baf9c91907ae0662130263192c1f4007152f176c0a14095b03a99c",
			baseSource: []byte("fetch(\"foo\");\n"),
		},
		{
			name:       "awaited_call",
			source:     []byte("async function f() {\n  const m = await import(\"foo\");\n  return m;\n}\n"),
			sha256:     "ce37afad67db99de9253674850fd9f8ae5819ac22171535ba282e097e61e03f8",
			deepDigest: "e48a5956112fb1cec0f0de5b5121beca17795ab99b29e8ad8b8c9548737bd24a",
			baseSource: []byte("async function f() {\n  const m = await fetch(\"foo\");\n  return m;\n}\n"),
		},
	}
	blob, err := os.ReadFile("grammar_blobs/javascript.bin")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(blob)); got != javaScriptDynamicImportBlobSHA256 {
		t.Fatalf("JavaScript blob digest = %s, want %s", got, javaScriptDynamicImportBlobSHA256)
	}
	language := JavascriptLanguage()
	reusable, ok := language.ExternalScanner.(gotreesitter.IncrementalReuseExternalScanner)
	if !ok || !reusable.SupportsIncrementalReuse() {
		t.Fatalf("JavaScript scanner does not certify incremental reuse: %T", language.ExternalScanner)
	}
	stateless, ok := language.ExternalScanner.(gotreesitter.StatelessExternalScanner)
	if !ok || !stateless.ExternalScannerIsStateless() {
		t.Fatalf("JavaScript scanner does not certify stateless operation: %T", language.ExternalScanner)
	}
	preserving, ok := language.ExternalScanner.(gotreesitter.FailurePreservingExternalScanner)
	if !ok || !preserving.PreservesStateOnScanFailure() {
		t.Fatalf("JavaScript scanner does not preserve state after a failed scan: %T", language.ExternalScanner)
	}
	for _, symbol := range language.ExternalSymbols {
		index := int(symbol)
		if index >= 0 && index < len(language.SymbolNames) && language.SymbolNames[index] == "import" {
			t.Fatalf("dynamic import uses external scanner symbol %d", symbol)
		}
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := fmt.Sprintf("%x", sha256.Sum256(test.source)); got != test.sha256 {
				t.Fatalf("source digest = %s, want %s", got, test.sha256)
			}
			for _, forestEnabled := range []bool{true, false} {
				name := "forest_enabled"
				if !forestEnabled {
					name = "forest_disabled"
				}
				t.Run(name, func(t *testing.T) {
					gotreesitter.SetGLRForestEnabled(forestEnabled)
					t.Cleanup(func() { gotreesitter.SetGLRForestEnabled(true) })
					rawParser := gotreesitter.NewParser(language)
					rawParser.SetAdmissionCandidateRoute(false)
					raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(test.source)
					if err != nil {
						t.Fatal(err)
					}
					defer raw.Release()
					productionParser := gotreesitter.NewParser(language)
					productionParser.SetAdmissionCandidateRoute(false)
					production, err := productionParser.Parse(test.source)
					if err != nil {
						t.Fatal(err)
					}
					defer production.Release()
					if got := requireJavaScriptDynamicImportRetirementTree(t, "raw", raw, language); got != test.deepDigest {
						t.Fatalf("raw digest = %s, want %s", got, test.deepDigest)
					}
					if got := requireJavaScriptDynamicImportRetirementTree(t, "production", production, language); got != test.deepDigest {
						t.Fatalf("production digest = %s, want %s", got, test.deepDigest)
					}
				})
			}

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(test.source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(compact.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			if routedAfter != routedBefore+1 || fallbackAfter != fallbackBefore {
				t.Fatalf("compact route counters routed=%d/%d fallback=%d/%d reason=%s", routedBefore, routedAfter, fallbackBefore, fallbackAfter, gotreesitter.AdmissionCandidateLastFallbackReason())
			}
			if got := requireJavaScriptDynamicImportRetirementTree(t, "compact", compact, language); got != test.deepDigest {
				t.Fatalf("compact digest = %s, want %s", got, test.deepDigest)
			}

			forestParser := gotreesitter.NewParser(language)
			forest, ok := forestParser.ParseForestExperimental(test.source)
			if !ok || forest == nil {
				offset, symbol, reason, _ := forestParser.ForestDeclineInfo()
				t.Fatalf("forest declined at %d symbol=%d reason=%s", offset, symbol, reason)
			}
			t.Cleanup(forest.Release)
			if !forest.ParseRuntime().ForestFastPath {
				t.Fatal("strict forest parse did not report the forest route")
			}
			if got := requireJavaScriptDynamicImportRetirementTree(t, "strict-forest", forest, language); got != test.deepDigest {
				t.Fatalf("strict forest digest = %s, want %s", got, test.deepDigest)
			}

			incrementalParser := gotreesitter.NewParser(language)
			incrementalParser.SetAdmissionCandidateRoute(false)
			oldTree, err := incrementalParser.Parse(test.baseSource)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(oldTree.Release)
			start := bytes.Index(test.baseSource, []byte("fetch"))
			if start < 0 {
				t.Fatal("incremental base has no fetch token")
			}
			startPoint := javaScriptDynamicImportPointAtByte(test.baseSource, start)
			oldEndPoint := javaScriptDynamicImportPointAtByte(test.baseSource, start+len("fetch"))
			newEndPoint := startPoint
			newEndPoint.Column += uint32(len("import"))
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(start),
				OldEndByte:  uint32(start + len("fetch")),
				NewEndByte:  uint32(start + len("import")),
				StartPoint:  startPoint,
				OldEndPoint: oldEndPoint,
				NewEndPoint: newEndPoint,
			})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(test.source, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(incremental.Release)
			if !profile.OldTreeReuseRoute || profile.ReuseUnsupported || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
				t.Fatalf("incremental route did not reuse the old tree: %+v", profile)
			}
			if got := requireJavaScriptDynamicImportRetirementTree(t, "incremental", incremental, language); got != test.deepDigest {
				t.Fatalf("incremental digest = %s, want %s", got, test.deepDigest)
			}
		})
	}
}

func TestJavaScriptDynamicImportRetirementSurvivorCensus(t *testing.T) {
	const source = "function foo() {\n  //      ^ definition.function\n}\n\nfoo()\n// <- reference.call\n\n{ source: $ => repeat($._expression) }\n// ^ definition.function\n//              ^ reference.call\n\nlet plus1 = x => x + 1\n//   ^ definition.function\n\nlet plus2 = function(x) { return x + 2 }\n//   ^ definition.function\n\nfunction *gen() { }\n//         ^ definition.function\n\nasync function* foo() { yield 1; }\n//               ^ definition.function\n"
	const sourceSHA256 = "0bbd2cdb0a0492055e442c44b533797386ec9c8aeb7ce8a4d0f5f5a4681e3b90"
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(source))); got != sourceSHA256 {
		t.Fatalf("survivor source digest = %s, want %s", got, sourceSHA256)
	}
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	gotreesitter.SetGLRForestEnabled(true)
	t.Cleanup(func() { gotreesitter.SetGLRForestEnabled(true) })
	parser := gotreesitter.NewParser(JavascriptLanguage())
	parser.SetAdmissionCandidateRoute(false)
	tree, err := parser.Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tree.Release)
	if tree.ParseRuntime().NormalizationPasses == nil {
		t.Fatal("JavaScript survivor census has no pass records")
	}
	for _, pass := range *tree.ParseRuntime().NormalizationPasses {
		if pass.Name != "dispatch.javascript" {
			continue
		}
		if pass.Checked == 0 || pass.Run == 0 || pass.NodesVisited == 0 {
			t.Fatalf("JavaScript survivor pass did not run: %+v", pass)
		}
		if pass.NodesRewritten != 7 {
			t.Fatalf("JavaScript survivor rewrites = %d, want 7", pass.NodesRewritten)
		}
		return
	}
	t.Fatal("JavaScript survivor census has no dispatch.javascript record")
}

func requireJavaScriptDynamicImportRetirementTree(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language) string {
	t.Helper()
	if tree.RootNode().HasError() {
		t.Fatalf("%s tree has an error node", route)
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	imports := javaScriptDynamicImportNodes(tree.RootNode(), language)
	if len(imports) != 1 {
		t.Fatalf("%s dynamic import count = %d", route, len(imports))
	}
	runtime := tree.ParseRuntime()
	if imports[0].ChildCount() != 1 {
		t.Fatalf("%s import children = %d, want 1", route, imports[0].ChildCount())
	}
	child := imports[0].Child(0)
	if child == nil || child.IsNamed() || child.Type(language) != "import" || child.StartByte() != imports[0].StartByte() || child.EndByte() != imports[0].EndByte() {
		t.Fatalf("%s import child is not the native anonymous full-span token: parent=%v child=%v", route, imports[0], child)
	}
	t.Logf("%s digest=%s import_children=%d forest=%t rewrites=%d",
		route,
		inspection.SHA256,
		imports[0].ChildCount(),
		runtime.ForestFastPath,
		runtime.NormalizationNodesRewritten,
	)
	if runtime.NormalizationPasses != nil {
		for _, pass := range *runtime.NormalizationPasses {
			if pass.Name == "dispatch.javascript" {
				if pass.NodesRewritten != 0 {
					t.Fatalf("%s dispatch.javascript rewrites = %d, want 0", route, pass.NodesRewritten)
				}
				t.Logf("%s pass=%s checked=%d run=%d visited=%d rewritten=%d", route, pass.Name, pass.Checked, pass.Run, pass.NodesVisited, pass.NodesRewritten)
			}
		}
	}
	return inspection.SHA256
}

func javaScriptDynamicImportPointAtByte(source []byte, offset int) gotreesitter.Point {
	var point gotreesitter.Point
	for index, value := range source {
		if index == offset {
			break
		}
		if value == '\n' {
			point.Row++
			point.Column = 0
			continue
		}
		point.Column++
	}
	return point
}

func javaScriptDynamicImportNodes(root *gotreesitter.Node, language *gotreesitter.Language) []*gotreesitter.Node {
	var nodes []*gotreesitter.Node
	gotreesitter.Walk(root, func(node *gotreesitter.Node, _ int) gotreesitter.WalkAction {
		if node.IsNamed() && node.Type(language) == "import" {
			nodes = append(nodes, node)
		}
		return gotreesitter.WalkContinue
	})
	return nodes
}
