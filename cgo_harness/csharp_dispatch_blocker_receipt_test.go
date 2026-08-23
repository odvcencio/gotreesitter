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

// TestCSharpDispatchBlockerRoutes records the live C# arm on raw, production,
// compact, forest, incremental, and locked-C routes.
func TestCSharpDispatchBlockerRoutes(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	witnesses := []struct {
		name string
		src  func(*testing.T) []byte
	}{
		{name: "a0-jsontextreader", src: func(t *testing.T) []byte {
			source, err := os.ReadFile("../testdata/parser_result/csharp/jsontextreader_excerpt.cs")
			if err != nil {
				t.Fatal(err)
			}
			return source
		}},
		{name: "positive-simple", src: func(*testing.T) []byte {
			return []byte("class C { public int M() { return 1; } }\n")
		}},
		{name: "historical-issue454", src: csharpDispatchIssue454Source},
		{name: "malformed-missing-body", src: func(*testing.T) []byte {
			return []byte("class C { void M() { int x = ;\n")
		}},
	}

	language := grammars.CSharpLanguage()
	cLanguage, err := COracleLanguage("c_sharp")
	if err != nil {
		t.Fatal(err)
	}
	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			source := witness.src(t)
			if len(source) == 0 || source[len(source)-1] != '\n' {
				source = append(source, '\n')
			}
			cTree := csharpDispatchCTree(t, cLanguage, source)
			defer cTree.Close()
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("witness=%s bytes=%d source_sha256=%x c_digest=%s c_error=%t", witness.name, len(source), sha256.Sum256(source), cDigest, cTree.RootNode().HasError())

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatal(err)
			}
			defer raw.Release()
			rawDiff := csharpDispatchAssertReceipt(t, "raw", raw, language, cTree, cDigest)

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer production.Release()
			productionDiff := csharpDispatchAssertReceipt(t, "production", production, language, cTree, cDigest)
			productionPass := csharpDispatchPass(production)
			if productionPass == nil {
				t.Fatalf("production did not record dispatch.c_sharp")
			}

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer compact.Release()
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			compactMode := csharpDispatchCompactMode(routedBefore, fallbackBefore, routedAfter, fallbackAfter)
			compactDiff := csharpDispatchAssertReceipt(t, "compact", compact, language, cTree, cDigest)

			forestParser := gotreesitter.NewParser(language)
			forest, forestOK := forestParser.ParseForestExperimental(source)
			forestMode := "declined"
			var forestDiff *DumpV1Divergence
			if forestOK && forest != nil {
				defer forest.Release()
				forestMode = "accepted"
				forestDiff = csharpDispatchAssertReceipt(t, "forest", forest, language, cTree, cDigest)
			}

			incrementalParser := gotreesitter.NewParser(language)
			incrementalParser.SetAdmissionCandidateRoute(false)
			base := bytes.TrimSuffix(source, []byte{'\n'})
			oldTree, err := incrementalParser.Parse(base)
			if err != nil {
				t.Fatal(err)
			}
			defer oldTree.Release()
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(base)),
				OldEndByte:  uint32(len(base)),
				NewEndByte:  uint32(len(source)),
				StartPoint:  csharpDispatchPoint(base),
				OldEndPoint: csharpDispatchPoint(base),
				NewEndPoint: csharpDispatchPoint(source),
			})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			defer incremental.Release()
			incrementalDiff := csharpDispatchAssertReceipt(t, "incremental", incremental, language, cTree, cDigest)
			if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "external_scanner_unsupported" || profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
				t.Fatalf("incremental reuse changed: unsupported=%t reason=%q old_tree=%t subtrees=%d bytes=%d", profile.ReuseUnsupported, profile.ReuseUnsupportedReason, profile.OldTreeReuseRoute, profile.ReusedSubtrees, profile.ReusedBytes)
			}

			switch witness.name {
			case "a0-jsontextreader":
				if productionPass.NodesRewritten != 2085 || productionPass.NodesVisited != 2093 {
					t.Fatalf("A0 dispatch pass = visited:%d rewritten:%d, want 2093/2085", productionPass.NodesVisited, productionPass.NodesRewritten)
				}
				if rawDiff == nil || rawDiff.Category != "shape" || productionDiff == nil || productionDiff.Category != "error" || forestOK {
					t.Fatalf("A0 evidence changed: raw=%+v production=%+v forest=%t", rawDiff, productionDiff, forestOK)
				}
			case "positive-simple":
				if productionPass.NodesRewritten != 0 || productionPass.NodesVisited != 22 {
					t.Fatalf("positive dispatch pass = visited:%d rewritten:%d, want 22/0", productionPass.NodesVisited, productionPass.NodesRewritten)
				}
				if rawDiff != nil || productionDiff != nil || compactDiff != nil || forestDiff != nil || incrementalDiff != nil || !forestOK || compactMode != "accepted" {
					t.Fatalf("positive control lost exact route parity: raw=%+v production=%+v compact=%+v forest=%+v incremental=%+v compact_mode=%s", rawDiff, productionDiff, compactDiff, forestDiff, incrementalDiff, compactMode)
				}
			case "historical-issue454":
				if productionPass.NodesRewritten != 0 || productionPass.NodesVisited != 57067 {
					t.Fatalf("issue-454 dispatch pass = visited:%d rewritten:%d, want 57067/0", productionPass.NodesVisited, productionPass.NodesRewritten)
				}
				if production.ParseRuntime().NativeRecoveredStructureAuthoritative != true || productionDiff == nil || productionDiff.Category != "error" || forestOK {
					t.Fatalf("issue-454 evidence changed: native=%t production=%+v forest=%t", production.ParseRuntime().NativeRecoveredStructureAuthoritative, productionDiff, forestOK)
				}
			case "malformed-missing-body":
				if productionPass.NodesRewritten != 0 || productionPass.NodesVisited != 19 {
					t.Fatalf("malformed dispatch pass = visited:%d rewritten:%d, want 19/0", productionPass.NodesVisited, productionPass.NodesRewritten)
				}
				if productionDiff == nil || productionDiff.Category != "extra" || forestOK {
					t.Fatalf("malformed evidence changed: production=%+v forest=%t", productionDiff, forestOK)
				}
			}
			t.Logf("route-summary compact=%s forest=%s incremental_reuse=%t raw=%+v production=%+v compact_diff=%+v forest_diff=%+v incremental_diff=%+v dispatch=%d/%d", compactMode, forestMode, profile.OldTreeReuseRoute, rawDiff, productionDiff, compactDiff, forestDiff, incrementalDiff, productionPass.NodesVisited, productionPass.NodesRewritten)
		})
	}
}

func csharpDispatchCTree(t *testing.T, language *sitter.Language, source []byte) *sitter.Tree {
	t.Helper()
	parser := sitter.NewParser()
	t.Cleanup(parser.Close)
	if err := parser.SetLanguage(language); err != nil {
		t.Fatal(err)
	}
	tree := parser.Parse(source, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("C parser returned no root")
	}
	return tree
}

func csharpDispatchAssertReceipt(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, cDigest string) *DumpV1Divergence {
	t.Helper()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	diff := FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode())
	t.Logf("route=%s error_root=%t digest=%s c_digest=%s exact=%t rewrites=%d native_authoritative=%t divergence=%+v", route, tree.RootNode().HasError(), inspection.SHA256, cDigest, diff == nil && inspection.SHA256 == cDigest, tree.ParseRuntime().NormalizationNodesRewritten, tree.ParseRuntime().NativeRecoveredStructureAuthoritative, diff)
	return diff
}

func csharpDispatchPass(tree *gotreesitter.Tree) *gotreesitter.NormalizationPassRuntime {
	runtime := tree.ParseRuntime()
	if runtime.NormalizationPasses == nil {
		return nil
	}
	for i := range *runtime.NormalizationPasses {
		pass := &(*runtime.NormalizationPasses)[i]
		if pass.Name == "dispatch.c_sharp" {
			return pass
		}
	}
	return nil
}

func csharpDispatchCompactMode(routedBefore, fallbackBefore, routedAfter, fallbackAfter uint64) string {
	if routedAfter == routedBefore+1 && fallbackAfter == fallbackBefore {
		return "accepted"
	}
	if routedAfter == routedBefore && fallbackAfter == fallbackBefore+1 {
		return "fallback"
	}
	return fmt.Sprintf("counters=%d/%d->%d/%d", routedBefore, fallbackBefore, routedAfter, fallbackAfter)
}

func csharpDispatchPoint(source []byte) gotreesitter.Point {
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

func csharpDispatchIssue454Source(t *testing.T) []byte {
	t.Helper()
	var source strings.Builder
	source.Grow(137*1024 + 256)
	source.WriteString("namespace Bench {\n")
	for i := 0; source.Len() < 137*1024; i++ {
		fmt.Fprintf(&source, "public static class C%d { public static int F%d() { var x%d = %d; return x%d; } }\n", i, i, i, i, i)
	}
	source.WriteString("}\n")
	result := []byte(source.String())
	site := bytes.Index(result, []byte("x0"))
	if site < 0 {
		t.Fatal("C# edit marker is absent")
	}
	return append(append([]byte(nil), result[:site]...), result[site+1:]...)
}
