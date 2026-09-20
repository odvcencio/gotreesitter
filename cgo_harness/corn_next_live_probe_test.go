//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
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

// TestCornNextLiveArmProbe records every route before a possible retirement.
// It keeps the quoted-path rewrite and malformed recovery visible.
func TestCornNextLiveArmProbe(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := grammars.CornLanguage()
	cLanguage, err := COracleLanguage("corn")
	if err != nil {
		t.Fatal(err)
	}

	witnesses := []struct {
		name      string
		file      string
		source    []byte
		malformed bool
	}{
		{name: "a0-compact", file: "small__compact.corn"},
		{name: "a0-complex", file: "small__complex.corn"},
		{name: "a0-readme-example", file: "small__readme_example.corn"},
		{
			name: "quoted-keys-trigger",
			source: []byte("{\n" +
				"    'foo.bar' = 42\n" +
				"    'green.eggs'.and.ham = \"hello world\"\n" +
				"    'with spaces' = true\n" +
				"    'escaped\\'quote' = false\n" +
				"    'escaped=equals' = -3\n" +
				"}\n"),
		},
		{
			name:   "plain-path-control",
			source: []byte("{\n    foo.bar = 42\n}\n"),
		},
		{
			name:      "malformed-quoted-path",
			source:    []byte("{\n    'foo.bar' = 42\n"),
			malformed: true,
		},
	}

	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			source := witness.source
			if source == nil {
				var err error
				source, err = os.ReadFile(filepath.Join(
					"..", "testdata", "dispatcher_census_a0", "corn", witness.file,
				))
				if err != nil {
					t.Fatal(err)
				}
			}
			t.Logf("witness=%s malformed=%t bytes=%d source_sha256=%x", witness.name, witness.malformed, len(source), sha256.Sum256(source))

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

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(raw.Release)
			t.Logf("route=raw %s", cornNextReceipt(raw, language, cTree, cDigest))

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			t.Logf("route=production %s", cornNextReceipt(production, language, cTree, cDigest))

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(compact.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			compactMode := fmt.Sprintf("counters=%d/%d->%d/%d", routedBefore, fallbackBefore, routedAfter, fallbackAfter)
			if routedAfter == routedBefore && fallbackAfter == fallbackBefore+1 {
				compactMode += " fallback:" + gotreesitter.AdmissionCandidateLastFallbackReason()
			} else if routedAfter == routedBefore+1 && fallbackAfter == fallbackBefore {
				compactMode += " accepted"
			}
			t.Logf("route=compact mode=%s %s", compactMode, cornNextReceipt(compact, language, cTree, cDigest))

			forestParser := gotreesitter.NewParser(language)
			forest, forestOK := forestParser.ParseForestExperimental(source)
			if forestOK && forest != nil {
				t.Cleanup(forest.Release)
				t.Logf("route=forest mode=accepted %s", cornNextReceipt(forest, language, cTree, cDigest))
			} else {
				t.Log("route=forest mode=declined")
			}

			incrementalParser := gotreesitter.NewParser(language)
			incrementalParser.SetAdmissionCandidateRoute(false)
			base := bytes.TrimSuffix(source, []byte{'\n'})
			oldTree, err := incrementalParser.Parse(base)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(oldTree.Release)
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(base)),
				OldEndByte:  uint32(len(base)),
				NewEndByte:  uint32(len(source)),
				StartPoint:  cornNextPointAtByte(base),
				OldEndPoint: cornNextPointAtByte(base),
				NewEndPoint: cornNextPointAtByte(source),
			})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(incremental.Release)
			t.Logf("route=incremental reuse=%t unsupported=%t reason=%q reused_subtrees=%d reused_bytes=%d %s", profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason, profile.ReusedSubtrees, profile.ReusedBytes, cornNextReceipt(incremental, language, cTree, cDigest))
		})
	}
}

// TestCornNextLiveArmReceiptDocument guards the blocker receipt markers.
func TestCornNextLiveArmReceiptDocument(t *testing.T) {
	raw, err := os.ReadFile("../docs/root-normalization-retirement.md")
	if err != nil {
		t.Fatal(err)
	}
	document := strings.Join(strings.Fields(string(raw)), " ")
	for _, marker := range []string{
		"The Corn arm remains live.",
		"A0 has three Corn files, three checked, three run, and zero rewrites.",
		"The quoted-path trigger rewrites four nodes and still differs from locked C.",
		"The malformed quoted-path witness differs from locked C at the error flag.",
		"Keep dispatch.corn live until scheduler_action_semantics emits the locked-C quoted-path tree.",
	} {
		marker = strings.Join(strings.Fields(marker), " ")
		if !strings.Contains(document, marker) {
			t.Fatalf("Corn blocker receipt lacks marker %q", marker)
		}
	}
}

func cornNextReceipt(tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, cDigest string) string {
	if tree == nil || tree.RootNode() == nil {
		return "tree=nil"
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		return fmt.Sprintf("inspect_error=%v", err)
	}
	diff := FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode())
	return fmt.Sprintf("error_root=%t digest=%s c_digest=%s exact=%t rewrites=%d passes=%s divergence=%+v", tree.RootNode().HasError(), inspection.SHA256, cDigest, diff == nil && inspection.SHA256 == cDigest, tree.ParseRuntime().NormalizationNodesRewritten, cornNextPasses(tree), diff)
}

func cornNextPasses(tree *gotreesitter.Tree) string {
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

func cornNextPointAtByte(source []byte) gotreesitter.Point {
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
