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

const wolframNextSplitInfixSource = "a + b\n"

const wolframNextCompoundInfixSource = "a + b + c\n"

const wolframNextMalformedInfixSource = "a +\n"

// TestWolframNextLiveArmProbe records every route before a possible retirement.
// It keeps split-infix recovery and unary-prefix behavior visible.
func TestWolframNextLiveArmProbe(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := grammars.WolframLanguage()
	cLanguage, err := COracleLanguage("wolfram")
	if err != nil {
		t.Fatal(err)
	}

	witnesses := []struct {
		name      string
		file      string
		source    []byte
		malformed bool
	}{
		{name: "a0-small-paclet-info", file: "small__PacletInfo.m"},
		{name: "a0-medium-output-handling", file: "medium__OutputHandlingUtilities.wl"},
		{name: "a0-large-evaluation-utilities", file: "large__EvaluationUtilities.wl"},
		{name: "split-infix", source: []byte(wolframNextSplitInfixSource)},
		{name: "compound-infix", source: []byte(wolframNextCompoundInfixSource)},
		{name: "unary-prefix-control", source: []byte("+b\n")},
		{name: "plain-symbol-control", source: []byte("a\n")},
		{name: "malformed-infix", source: []byte(wolframNextMalformedInfixSource), malformed: true},
	}

	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			source := witness.source
			if source == nil {
				var err error
				source, err = os.ReadFile(filepath.Join(
					"..", "testdata", "dispatcher_census_a0", "wolfram", witness.file,
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
			t.Logf("route=raw %s", wolframNextReceipt(raw, language, cTree, cDigest))

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			t.Logf("route=production %s", wolframNextReceipt(production, language, cTree, cDigest))

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
			t.Logf("route=compact mode=%s %s", compactMode, wolframNextReceipt(compact, language, cTree, cDigest))

			forestParser := gotreesitter.NewParser(language)
			forest, forestOK := forestParser.ParseForestExperimental(source)
			if forestOK && forest != nil {
				t.Cleanup(forest.Release)
				t.Logf("route=forest mode=accepted %s", wolframNextReceipt(forest, language, cTree, cDigest))
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
				StartPoint:  wolframNextPointAtByte(base),
				OldEndPoint: wolframNextPointAtByte(base),
				NewEndPoint: wolframNextPointAtByte(source),
			})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(incremental.Release)
			t.Logf("route=incremental reuse=%t unsupported=%t reason=%q reused_subtrees=%d reused_bytes=%d %s", profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason, profile.ReusedSubtrees, profile.ReusedBytes, wolframNextReceipt(incremental, language, cTree, cDigest))
		})
	}
}

// TestWolframNextLiveArmReceiptDocument guards the blocker receipt markers.
func TestWolframNextLiveArmReceiptDocument(t *testing.T) {
	raw, err := os.ReadFile("../docs/root-normalization-retirement.md")
	if err != nil {
		t.Fatal(err)
	}
	document := strings.Join(strings.Fields(string(raw)), " ")
	for _, marker := range []string{
		"The Wolfram arm remains live.",
		"A0 has three Wolfram files, six checked, six run, and zero rewrites.",
		"The split-infix parser route records zero dispatch.wolfram rewrites.",
		"The malformed infix witness remains a recovery blocker.",
		"Keep dispatch.wolfram live until derivation_election_selection emits the locked-C infix tree.",
	} {
		marker = strings.Join(strings.Fields(marker), " ")
		if !strings.Contains(document, marker) {
			t.Fatalf("Wolfram blocker receipt lacks marker %q", marker)
		}
	}
}

func wolframNextReceipt(tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, cDigest string) string {
	if tree == nil || tree.RootNode() == nil {
		return "tree=nil"
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		return fmt.Sprintf("inspect_error=%v", err)
	}
	diff := FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode())
	return fmt.Sprintf("error_root=%t digest=%s c_digest=%s exact=%t rewrites=%d passes=%s divergence=%+v", tree.RootNode().HasError(), inspection.SHA256, cDigest, diff == nil && inspection.SHA256 == cDigest, tree.ParseRuntime().NormalizationNodesRewritten, wolframNextPasses(tree), diff)
}

func wolframNextPasses(tree *gotreesitter.Tree) string {
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

func wolframNextPointAtByte(source []byte) gotreesitter.Point {
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
