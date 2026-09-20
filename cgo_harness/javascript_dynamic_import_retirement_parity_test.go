//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestJavaScriptDynamicImportRetirementLockedCRoutes(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := grammars.JavascriptLanguage()
	cLanguage, err := COracleLanguage("javascript")
	if err != nil {
		t.Fatal(err)
	}
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

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := fmt.Sprintf("%x", sha256.Sum256(test.source)); got != test.sha256 {
				t.Fatalf("source digest = %s, want %s", got, test.sha256)
			}
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(test.source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parse returned a nil tree")
			}
			t.Cleanup(cTree.Close)

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(test.source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(raw.Release)
			assertJavaScriptDynamicImportLockedCExact(t, "JavaScript dynamic import raw", raw, language, cTree, test.deepDigest)

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(test.source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			assertJavaScriptDynamicImportLockedCExact(t, "JavaScript dynamic import production", production, language, cTree, test.deepDigest)

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
			assertJavaScriptDynamicImportLockedCExact(t, "JavaScript dynamic import compact", compact, language, cTree, test.deepDigest)

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
			assertJavaScriptDynamicImportLockedCExact(t, "JavaScript dynamic import forest", forest, language, cTree, test.deepDigest)

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
			startPoint := javaScriptDynamicImportRetirementPointAtByte(test.baseSource, start)
			oldEndPoint := javaScriptDynamicImportRetirementPointAtByte(test.baseSource, start+len("fetch"))
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
			assertJavaScriptDynamicImportLockedCExact(t, "JavaScript dynamic import incremental", incremental, language, cTree, test.deepDigest)
		})
	}
}

func assertJavaScriptDynamicImportLockedCExact(t *testing.T, label string, tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, wantDigest string) {
	t.Helper()
	assertLockedCTreeExact(t, label, tree, language, cTree)
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.SHA256 != wantDigest {
		t.Fatalf("%s deep digest = %s, want %s", label, inspection.SHA256, wantDigest)
	}
}

func javaScriptDynamicImportRetirementPointAtByte(source []byte, offset int) gotreesitter.Point {
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
