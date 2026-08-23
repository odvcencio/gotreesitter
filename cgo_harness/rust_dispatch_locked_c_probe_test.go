//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type rustDispatchLockedCWitness struct {
	name string
	src  []byte
}

// TestRustDispatchRegisteredWitnessLockedCDeepParity checks every Rust
// witness named by the ownership registry against the locked C oracle.
// It also checks the production, compact, forest, and incremental routes.
func TestRustDispatchRegisteredWitnessLockedCDeepParity(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	fixtures := []rustDispatchLockedCWitness{
		{name: "ownership-registry-smoke", src: []byte(grammars.ParseSmokeSample("rust"))},
		{name: "tracked-census-rust_ast.rs", src: rustDispatchReadFixture(t, "../testdata/incremental_gate/rust_ast.rs")},
	}
	goLanguage := grammars.RustLanguage()
	cLanguage, err := ParityCLanguage("rust")
	if err != nil {
		t.Fatalf("load locked Rust C oracle: %v", err)
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			sourceHash := sha256.Sum256(fixture.src)
			t.Logf("witness=%s bytes=%d source_sha256=%s", fixture.name, len(fixture.src), hex.EncodeToString(sourceHash[:]))

			raw := rustDispatchParseRaw(t, goLanguage, fixture.src)
			defer raw.Release()
			production := rustDispatchParseProduction(t, goLanguage, fixture.src)
			defer production.Release()
			rawDigest := rustDispatchGoDigest(t, raw, goLanguage)
			productionDigest := rustDispatchGoDigest(t, production, goLanguage)
			if rawDigest != productionDigest {
				t.Fatalf("raw/production deep digest mismatch: raw=%s production=%s", rawDigest, productionDigest)
			}
			cDigest := rustDispatchCDigest(t, cLanguage, fixture.src)
			if rawDigest != cDigest {
				t.Fatalf("raw/locked-C deep digest mismatch: go=%s c=%s", rawDigest, cDigest)
			}
			t.Logf("source route=raw production raw_digest=%s c_digest=%s raw_rewrites=%d production_rewrites=%d", rawDigest, cDigest, raw.ParseRuntime().NormalizationNodesRewritten, production.ParseRuntime().NormalizationNodesRewritten)
			rLogRustDispatchLockedC(t, "production", production.ParseRuntime())

			routeSource := append(append([]byte(nil), fixture.src...), '\n')
			routeCDigest := rustDispatchCDigest(t, cLanguage, routeSource)
			productionRoute := rustDispatchParseProduction(t, goLanguage, routeSource)
			defer productionRoute.Release()
			rustDispatchAssertGoDigest(t, "production-route", productionRoute, goLanguage, routeCDigest)
			rLogRustDispatchLockedC(t, "production-route", productionRoute.ParseRuntime())

			beforeCandidate, beforeFallback := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(goLanguage)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(routeSource)
			if err != nil {
				t.Fatalf("compact route: %v", err)
			}
			defer compact.Release()
			rustDispatchAssertGoDigest(t, "compact", compact, goLanguage, routeCDigest)
			afterCandidate, afterFallback := gotreesitter.AdmissionCandidateCounters()
			t.Logf("route=compact candidate_routed_delta=%d candidate_fallback_delta=%d digest=%s", afterCandidate-beforeCandidate, afterFallback-beforeFallback, rustDispatchGoDigest(t, compact, goLanguage))
			if afterCandidate <= beforeCandidate {
				t.Fatalf("compact route did not enter the admission candidate")
			}
			if afterFallback != beforeFallback {
				t.Fatalf("compact route fell back: before=%d after=%d reason=%q", beforeFallback, afterFallback, gotreesitter.AdmissionCandidateLastFallbackReason())
			}

			forestParser := gotreesitter.NewParser(goLanguage)
			forest, ok := forestParser.ParseForestExperimental(routeSource)
			if !ok || forest == nil {
				offset, symbol, reason, states := forestParser.ForestDeclineInfo()
				t.Fatalf("forest route declined: offset=%d symbol=%d reason=%q states=%v", offset, symbol, reason, states)
			}
			defer forest.Release()
			rustDispatchAssertGoDigest(t, "forest", forest, goLanguage, routeCDigest)
			t.Logf("route=forest outcome=accepted digest=%s forest_fast_path=%t", rustDispatchGoDigest(t, forest, goLanguage), forest.ParseRuntime().ForestFastPath)
			rLogRustDispatchLockedC(t, "forest", forest.ParseRuntime())

			oldTree := rustDispatchParseProduction(t, goLanguage, fixture.src)
			defer oldTree.Release()
			oldEnd := rustDispatchPointAtByte(fixture.src, len(fixture.src))
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(fixture.src)),
				OldEndByte:  uint32(len(fixture.src)),
				NewEndByte:  uint32(len(routeSource)),
				StartPoint:  oldEnd,
				OldEndPoint: oldEnd,
				NewEndPoint: rustDispatchPointAtByte(routeSource, len(routeSource)),
			})
			incrementalParser := gotreesitter.NewParser(goLanguage)
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(routeSource, oldTree)
			if err != nil {
				t.Fatalf("incremental route: %v", err)
			}
			defer incremental.Release()
			rustDispatchAssertGoDigest(t, "incremental", incremental, goLanguage, routeCDigest)
			t.Logf("route=incremental digest=%s reused_subtrees=%d reused_bytes=%d reuse_unsupported=%t reason=%q", rustDispatchGoDigest(t, incremental, goLanguage), profile.ReusedSubtrees, profile.ReusedBytes, profile.ReuseUnsupported, profile.ReuseUnsupportedReason)
			rLogRustDispatchLockedC(t, "incremental", incremental.ParseRuntime())
		})
	}
}

func rustDispatchReadFixture(t *testing.T, path string) []byte {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Rust fixture %s: %v", path, err)
	}
	return source
}

func rustDispatchParseRaw(t *testing.T, language *gotreesitter.Language, source []byte) *gotreesitter.Tree {
	t.Helper()
	parser := gotreesitter.NewParser(language)
	tree, err := parser.ParseNoResultCompatibilityBenchmarkOnly(source)
	if err != nil {
		t.Fatalf("raw parse: %v", err)
	}
	return tree
}

func rustDispatchParseProduction(t *testing.T, language *gotreesitter.Language, source []byte) *gotreesitter.Tree {
	t.Helper()
	parser := gotreesitter.NewParser(language)
	parser.SetAdmissionCandidateRoute(false)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	return tree
}

func rustDispatchGoDigest(t *testing.T, tree *gotreesitter.Tree, language *gotreesitter.Language) string {
	t.Helper()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatalf("inspect Go deep tree: %v", err)
	}
	return inspection.SHA256
}

func rustDispatchCDigest(t *testing.T, language *sitter.Language, source []byte) string {
	t.Helper()
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(language); err != nil {
		t.Fatalf("set C oracle language: %v", err)
	}
	tree := parser.Parse(source, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("C oracle returned a nil tree")
	}
	defer tree.Close()
	digest, err := COracleDeepDigest(tree)
	if err != nil {
		t.Fatalf("inspect C deep tree: %v", err)
	}
	return digest
}

func rustDispatchAssertGoDigest(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language, want string) {
	t.Helper()
	got := rustDispatchGoDigest(t, tree, language)
	if got != want {
		t.Fatalf("route=%s deep digest Go=%s, want locked C %s", route, got, want)
	}
	if tree.RootNode().HasError() {
		t.Fatalf("route=%s produced an error root", route)
	}
}

func rLogRustDispatchLockedC(t *testing.T, route string, runtime gotreesitter.ParseRuntime) {
	t.Helper()
	if runtime.NormalizationPasses == nil {
		t.Logf("route=%s dispatch.rust=absent", route)
		return
	}
	for _, pass := range *runtime.NormalizationPasses {
		if pass.Name == "dispatch.rust" {
			t.Logf("route=%s pass=%s checked=%d run=%d visited=%d rewritten=%d", route, pass.Name, pass.Checked, pass.Run, pass.NodesVisited, pass.NodesRewritten)
		}
	}
}

func rustDispatchPointAtByte(source []byte, offset int) gotreesitter.Point {
	if offset < 0 || offset > len(source) {
		panic(fmt.Sprintf("point offset %d outside source length %d", offset, len(source)))
	}
	var point gotreesitter.Point
	for _, value := range source[:offset] {
		if value == '\n' {
			point.Row++
			point.Column = 0
		} else {
			point.Column++
		}
	}
	return point
}
