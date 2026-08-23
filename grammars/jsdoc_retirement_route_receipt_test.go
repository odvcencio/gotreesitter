//go:build !grammar_subset

package grammars

import (
	"crypto/sha256"
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

const (
	jsdocRetirementTrigger       = "/**\n * @param {string} name\n * @returns {number}\n */\n"
	jsdocRetirementControl       = "/**\n * @param {string} name\n */\n"
	jsdocRetirementTriggerSHA256 = "8a1683a43035994f3abf03f2f9556b96514a745018c5373ff77d3127fb27d201"
	jsdocRetirementControlSHA256 = "0f4dbe6ca5d62b8c033c09ac26689c787a66298540c46b3af7a9760a7240b5ce"
)

func TestJsdocDispatchRetirementRoutes(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := JsdocLanguage()
	for _, test := range []struct {
		name   string
		source []byte
		sha256 string
	}{
		{name: "multi_tag_trigger", source: []byte(jsdocRetirementTrigger), sha256: jsdocRetirementTriggerSHA256},
		{name: "single_tag_control", source: []byte(jsdocRetirementControl), sha256: jsdocRetirementControlSHA256},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := fmt.Sprintf("%x", sha256.Sum256(test.source)); got != test.sha256 {
				t.Fatalf("source SHA-256 = %s, want %s", got, test.sha256)
			}

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(test.source)
			if err != nil {
				t.Fatalf("raw parse: %v", err)
			}
			t.Cleanup(raw.Release)

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(test.source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			t.Cleanup(production.Release)

			rawDigest := jsdocTreeDigest(t, raw, language)
			productionDigest := jsdocTreeDigest(t, production, language)
			pass := jsdocDispatchPass(t, production)
			if test.name == "multi_tag_trigger" {
				if pass.NodesRewritten == 0 {
					t.Errorf("trigger did not fire dispatch.jsdoc: %+v", pass)
				}
				if rawDigest == productionDigest {
					t.Errorf("trigger raw and production digests match despite a rewrite: %s", rawDigest)
				}
			} else {
				if pass.NodesRewritten != 0 {
					t.Errorf("control rewrote %d nodes: %+v", pass.NodesRewritten, pass)
				}
				if rawDigest != productionDigest {
					t.Errorf("control raw digest=%s production digest=%s", rawDigest, productionDigest)
				}
			}
			t.Logf("raw_digest=%s production_digest=%s pass=%+v", rawDigest, productionDigest, pass)

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(test.source)
			if err != nil {
				t.Fatalf("compact parse: %v", err)
			}
			t.Cleanup(compact.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			compactDirect := routedAfter == routedBefore+1 && fallbackAfter == fallbackBefore
			compactFallback := routedAfter == routedBefore && fallbackAfter == fallbackBefore+1
			if !compactDirect && !compactFallback {
				t.Fatalf("compact counters routed=%d/%d fallback=%d/%d reason=%s", routedBefore, routedAfter, fallbackBefore, fallbackAfter, gotreesitter.AdmissionCandidateLastFallbackReason())
			}
			compactDigest := jsdocTreeDigest(t, compact, language)
			if compactDigest != productionDigest {
				t.Fatalf("compact digest=%s production digest=%s", compactDigest, productionDigest)
			}
			t.Logf("compact direct=%t fallback=%t digest=%s reason=%s", compactDirect, compactFallback, compactDigest, gotreesitter.AdmissionCandidateLastFallbackReason())

			forestParser := gotreesitter.NewParser(language)
			forest, ok := forestParser.ParseForestExperimental(test.source)
			if !ok || forest == nil {
				offset, symbol, reason, _ := forestParser.ForestDeclineInfo()
				if reason == "" {
					t.Fatalf("forest declined without a reason at %d symbol=%d", offset, symbol)
				}
				t.Logf("forest fallback offset=%d symbol=%d reason=%s production_digest=%s", offset, symbol, reason, productionDigest)
			} else {
				t.Cleanup(forest.Release)
				if !forest.ParseRuntime().ForestFastPath {
					t.Fatal("forest parse did not report the forest route")
				}
				forestDigest := jsdocTreeDigest(t, forest, language)
				if forestDigest != productionDigest {
					t.Fatalf("forest digest=%s production digest=%s", forestDigest, productionDigest)
				}
				t.Logf("forest direct digest=%s", forestDigest)
			}

			incrementalParser := gotreesitter.NewParser(language)
			incrementalParser.SetAdmissionCandidateRoute(false)
			oldTree, err := incrementalParser.Parse(test.source)
			if err != nil {
				t.Fatalf("incremental base parse: %v", err)
			}
			t.Cleanup(oldTree.Release)
			point := jsdocPointAtByte(test.source, len(test.source))
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(test.source)),
				OldEndByte:  uint32(len(test.source)),
				NewEndByte:  uint32(len(test.source)),
				StartPoint:  point,
				OldEndPoint: point,
				NewEndPoint: point,
			})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(test.source, oldTree)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			t.Cleanup(incremental.Release)
			incrementalDigest := jsdocTreeDigest(t, incremental, language)
			if incrementalDigest != productionDigest {
				t.Fatalf("incremental digest=%s production digest=%s profile=%+v", incrementalDigest, productionDigest, profile)
			}
			if profile.ReuseUnsupported {
				if profile.ReuseUnsupportedReason == "" {
					t.Fatalf("incremental fallback has no reason: %+v", profile)
				}
				t.Logf("incremental fallback reason=%s digest=%s", profile.ReuseUnsupportedReason, incrementalDigest)
			} else {
				if !profile.OldTreeReuseRoute || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
					t.Fatalf("incremental route did not reuse the old tree: %+v", profile)
				}
				t.Logf("incremental reuse subtrees=%d bytes=%d digest=%s", profile.ReusedSubtrees, profile.ReusedBytes, incrementalDigest)
			}
		})
	}
}

func jsdocDispatchPass(t *testing.T, tree *gotreesitter.Tree) gotreesitter.NormalizationPassRuntime {
	t.Helper()
	if tree.ParseRuntime().NormalizationPasses == nil {
		t.Fatal("missing normalization pass records")
	}
	for _, pass := range *tree.ParseRuntime().NormalizationPasses {
		if pass.Name == "dispatch.jsdoc" {
			return pass
		}
	}
	t.Fatal("missing dispatch.jsdoc pass record")
	return gotreesitter.NormalizationPassRuntime{}
}

func jsdocTreeDigest(t *testing.T, tree *gotreesitter.Tree, language *gotreesitter.Language) string {
	t.Helper()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	return inspection.SHA256
}

func jsdocPointAtByte(source []byte, offset int) gotreesitter.Point {
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
