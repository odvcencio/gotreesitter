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
	kotlinInterpolatedCallPositiveControlCommit = "a321147042b7374a52570865d6ec44e1771669a4"
	kotlinInterpolatedCallProducerFixCommit     = "b06804219dc0b27a0804d769a5cc24626568387d"
	kotlinInterpolatedCallBlobSHA256            = "8c618126dbd4ed6cdda93b922e574cef500a8d5c9a250d3fc1a345c15b4c7f89"
	kotlinInterpolatedCallSourceSHA256          = "2ab0943ca4d948edded0764c76ad9a30a923c1b5e8ecfb331b912a9d4aca2df1"
	kotlinInterpolatedCallDeepDigest            = "1d39bfec6c6290c0b3440ee99ba8b05e4f4c9cc17ff233f950d2c62de73981dd"
)

func TestKotlinInterpolatedCallRetirementRoutes(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	blob, err := os.ReadFile("grammar_blobs/kotlin.bin")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(blob)); got != kotlinInterpolatedCallBlobSHA256 {
		t.Fatalf("Kotlin blob digest=%s, want %s", got, kotlinInterpolatedCallBlobSHA256)
	}
	language := KotlinLanguage()
	source := []byte("package demo\n\nfun f() {\n  val time = if (true) \"${Instant.now()} \" else \"\"\n}\n")
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != kotlinInterpolatedCallSourceSHA256 {
		t.Fatalf("source digest=%s, want %s", got, kotlinInterpolatedCallSourceSHA256)
	}

	rawParser := gotreesitter.NewParser(language)
	rawParser.SetAdmissionCandidateRoute(false)
	raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(raw.Release)
	if raw.ParseRuntime().ForestFastPath {
		t.Fatal("raw production-pinned parse reported the forest route")
	}
	requireKotlinInterpolatedCallRetirementTree(t, "raw", raw, language)

	productionParser := gotreesitter.NewParser(language)
	productionParser.SetAdmissionCandidateRoute(false)
	production, err := productionParser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(production.Release)
	if production.ParseRuntime().ForestFastPath {
		t.Fatal("production-pinned parse reported the forest route")
	}
	requireKotlinInterpolatedCallRetirementTree(t, "production", production, language)

	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	compactParser := gotreesitter.NewParser(language)
	compactParser.SetAdmissionCandidateRoute(true)
	compact, err := compactParser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(compact.Release)
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter != routedBefore+1 || fallbackAfter != fallbackBefore {
		t.Fatalf("compact route counters routed=%d/%d fallback=%d/%d reason=%s", routedBefore, routedAfter, fallbackBefore, fallbackAfter, gotreesitter.AdmissionCandidateLastFallbackReason())
	}
	requireKotlinInterpolatedCallRetirementTree(t, "compact", compact, language)

	forestParser := gotreesitter.NewParser(language)
	forest, ok := forestParser.ParseForestExperimental(source)
	if !ok || forest == nil {
		offset, symbol, reason, _ := forestParser.ForestDeclineInfo()
		t.Fatalf("forest declined at %d symbol=%d reason=%s", offset, symbol, reason)
	}
	t.Cleanup(forest.Release)
	if !forest.ParseRuntime().ForestFastPath {
		t.Fatal("strict forest did not report the forest route")
	}
	requireKotlinInterpolatedCallRetirementTree(t, "forest", forest, language)

	baseSource := bytes.Replace(source, []byte("Instant"), []byte("Moments"), 1)
	incrementalParser := gotreesitter.NewParser(language)
	incrementalParser.SetAdmissionCandidateRoute(false)
	oldTree, err := incrementalParser.Parse(baseSource)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(oldTree.Release)
	start := bytes.Index(baseSource, []byte("Moments"))
	if start < 0 {
		t.Fatal("incremental base has no Moments token")
	}
	point := kotlinInterpolatedCallPointAtByte(baseSource, start)
	oldEnd, newEnd := point, point
	oldEnd.Column += uint32(len("Moments"))
	newEnd.Column += uint32(len("Instant"))
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte: uint32(start), OldEndByte: uint32(start + len("Moments")), NewEndByte: uint32(start + len("Instant")),
		StartPoint: point, OldEndPoint: oldEnd, NewEndPoint: newEnd,
	})
	incremental, profile, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(incremental.Release)
	if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "external_scanner_unsupported" || profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
		t.Fatalf("incremental route-extinction receipt changed: %+v", profile)
	}
	requireKotlinInterpolatedCallRetirementTree(t, "incremental-fresh-fallback", incremental, language)
}

func TestKotlinInterpolatedCallRetirementNegativeControls(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	tests := []struct {
		name   string
		source []byte
		sha256 string
	}{
		{
			name:   "identifier_interpolation",
			source: []byte("package demo\n\nfun f() {\n  val time = \"$value\"\n}\n"),
			sha256: "3daf6da36aeb0744626526bfe40e65481fd976a807fddaa65fcbe8ee8f36bedc",
		},
		{
			name:   "call_outside_interpolation",
			source: []byte("package demo\n\nfun f() {\n  val time = Instant.now()\n}\n"),
			sha256: "783d4aff9596d45dbb46a53935cdd72fbeb51a935a2d0d8b44153ac223e1aadf",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := fmt.Sprintf("%x", sha256.Sum256(test.source)); got != test.sha256 {
				t.Fatalf("source digest=%s, want %s", got, test.sha256)
			}
			language := KotlinLanguage()
			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(test.source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(raw.Release)
			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(test.source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			rawInspection, err := benchfixtures.InspectGoTree(raw.RootNode(), language)
			if err != nil {
				t.Fatal(err)
			}
			productionInspection, err := benchfixtures.InspectGoTree(production.RootNode(), language)
			if err != nil {
				t.Fatal(err)
			}
			if rawInspection.SHA256 != productionInspection.SHA256 {
				t.Fatalf("raw/production mismatch: raw=%s production=%s", rawInspection.SHA256, productionInspection.SHA256)
			}
			requireNoKotlinInterpolatedCallRewrite(t, "production", production)
		})
	}
}

func requireKotlinInterpolatedCallRetirementTree(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language) {
	t.Helper()
	if tree.RootNode() == nil || tree.RootNode().HasError() {
		t.Fatalf("%s returned an invalid tree", route)
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.SHA256 != kotlinInterpolatedCallDeepDigest {
		t.Fatalf("%s deep digest=%s, want %s", route, inspection.SHA256, kotlinInterpolatedCallDeepDigest)
	}
	interpolated := kotlinInterpolatedCallFirstNode(tree.RootNode(), language, "interpolated_expression")
	if interpolated == nil || interpolated.ChildCount() != 1 {
		t.Fatalf("%s has no single-child interpolated expression: %s", route, tree.RootNode().SExpr(language))
	}
	call := interpolated.Child(0)
	if call == nil || call.Type(language) != "call_expression" || call.ChildCount() != 2 {
		t.Fatalf("%s does not contain the native call expression: %s", route, interpolated.SExpr(language))
	}
	requireNoKotlinInterpolatedCallRewrite(t, route, tree)
}

func kotlinInterpolatedCallFirstNode(root *gotreesitter.Node, language *gotreesitter.Language, nodeType string) *gotreesitter.Node {
	var match *gotreesitter.Node
	gotreesitter.Walk(root, func(node *gotreesitter.Node, _ int) gotreesitter.WalkAction {
		if node.Type(language) == nodeType {
			match = node
			return gotreesitter.WalkStop
		}
		return gotreesitter.WalkContinue
	})
	return match
}

func requireNoKotlinInterpolatedCallRewrite(t *testing.T, route string, tree *gotreesitter.Tree) {
	t.Helper()
	runtime := tree.ParseRuntime()
	if runtime.NormalizationNodesRewritten != 0 {
		t.Fatalf("%s reports %d normalization rewrites", route, runtime.NormalizationNodesRewritten)
	}
	if runtime.NormalizationPasses == nil {
		return
	}
	for _, pass := range *runtime.NormalizationPasses {
		if pass.Name != "dispatch.kotlin" {
			continue
		}
		if pass.Checked != 1 || pass.Run != 1 || pass.NodesRewritten != 0 {
			t.Fatalf("%s Kotlin census=%+v, want one run and zero rewrites", route, pass)
		}
		return
	}
	t.Fatalf("%s has no dispatch.kotlin census row", route)
}

func kotlinInterpolatedCallPointAtByte(source []byte, offset int) gotreesitter.Point {
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
