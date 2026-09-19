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

const (
	kotlinInterpolatedCallSourceSHA256 = "2ab0943ca4d948edded0764c76ad9a30a923c1b5e8ecfb331b912a9d4aca2df1"
	kotlinInterpolatedCallDeepDigest   = "1d39bfec6c6290c0b3440ee99ba8b05e4f4c9cc17ff233f950d2c62de73981dd"
)

func TestKotlinInterpolatedCallRetirementLockedCRoutes(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := grammars.KotlinLanguage()
	source := []byte("package demo\n\nfun f() {\n  val time = if (true) \"${Instant.now()} \" else \"\"\n}\n")
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != kotlinInterpolatedCallSourceSHA256 {
		t.Fatalf("source digest=%s, want %s", got, kotlinInterpolatedCallSourceSHA256)
	}
	cLanguage, err := COracleLanguage("kotlin")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("locked C parse returned a nil tree")
	}
	t.Cleanup(cTree.Close)

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
	assertKotlinInterpolatedCallLockedCExact(t, "raw", raw, language, cTree)

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
	assertKotlinInterpolatedCallLockedCExact(t, "production", production, language, cTree)

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
	assertKotlinInterpolatedCallLockedCExact(t, "compact", compact, language, cTree)

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
	assertKotlinInterpolatedCallLockedCExact(t, "forest", forest, language, cTree)

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
	point := kotlinInterpolatedCallParityPointAtByte(baseSource, start)
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
		t.Fatalf("incremental fallback receipt changed: %+v", profile)
	}
	assertKotlinInterpolatedCallLockedCExact(t, "incremental-fresh-fallback", incremental, language, cTree)
}

func assertKotlinInterpolatedCallLockedCExact(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree) {
	t.Helper()
	assertLockedCTreeExact(t, "Kotlin interpolated call "+route, tree, language, cTree)
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.SHA256 != kotlinInterpolatedCallDeepDigest {
		t.Fatalf("%s deep digest=%s, want %s", route, inspection.SHA256, kotlinInterpolatedCallDeepDigest)
	}
	if tree.ParseRuntime().NormalizationNodesRewritten != 0 {
		t.Fatalf("%s reports %d normalization rewrites", route, tree.ParseRuntime().NormalizationNodesRewritten)
	}
}

func kotlinInterpolatedCallParityPointAtByte(source []byte, offset int) gotreesitter.Point {
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
