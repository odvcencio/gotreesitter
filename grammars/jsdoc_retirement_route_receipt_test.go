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
	jsdocProducerTrigger       = "/**\n * @param {string} name\n * @returns {number}\n */\n"
	jsdocProducerControl       = "/**\n * @param {string} name\n */\n"
	jsdocProducerTriggerSHA256 = "8a1683a43035994f3abf03f2f9556b96514a745018c5373ff77d3127fb27d201"
	jsdocProducerControlSHA256 = "0f4dbe6ca5d62b8c033c09ac26689c787a66298540c46b3af7a9760a7240b5ce"
	jsdocUncertifiedGapRoute   = "fallback:compact route declined at accept_without_materialization: accepted-leaf-tiling-gap: compact subtree symbol=38 span=7..48 has an unaccounted byte range 27..31 not covered by any child"
)

func TestJsdocLexerSkipProvenanceRoutes(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := JsdocLanguage()
	for _, test := range []struct {
		name             string
		source           []byte
		sha256           string
		compactRoute     string
		forestRoute      string
		incrementalRoute string
	}{
		{
			name:             "multi_tag_trigger",
			source:           []byte(jsdocProducerTrigger),
			sha256:           jsdocProducerTriggerSHA256,
			compactRoute:     "direct",
			forestRoute:      "fallback:51:22:dead_end",
			incrementalRoute: "fallback:external_scanner_unsupported",
		},
		{
			name:             "single_tag_control",
			source:           []byte(jsdocProducerControl),
			sha256:           jsdocProducerControlSHA256,
			compactRoute:     "direct",
			forestRoute:      "fallback:31:0:nolook_relex_empty",
			incrementalRoute: "fallback:external_scanner_unsupported",
		},
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
			jsdocRequireNoDispatchRewrites(t, raw, "raw")
			jsdocRequireNoDispatchRewrites(t, production, "production")
			if rawDigest != productionDigest {
				t.Fatalf("raw digest=%s production digest=%s", rawDigest, productionDigest)
			}
			t.Logf("witness=%s bytes=%d source_sha256=%s raw_digest=%s production_digest=%s raw_rewrites=%d production_rewrites=%d", test.name, len(test.source), test.sha256, rawDigest, productionDigest, raw.ParseRuntime().NormalizationNodesRewritten, production.ParseRuntime().NormalizationNodesRewritten)

			compactRoute, compactDigest := jsdocCompactRoute(t, language, test.source, productionDigest)
			if compactRoute != test.compactRoute {
				t.Fatalf("compact route=%q, want %q", compactRoute, test.compactRoute)
			}
			t.Logf("compact route=%s digest=%s", compactRoute, compactDigest)
			if test.name == "multi_tag_trigger" {
				uncertified, err := LoadLanguage("jsdoc", BlobByName("jsdoc"))
				if err != nil {
					t.Fatalf("load uncertified JSDoc language: %v", err)
				}
				uncertified.CompactLexerSkippedPrefixTilingCertified = false
				uncertifiedRoute, uncertifiedDigest := jsdocCompactRoute(t, uncertified, test.source, productionDigest)
				if uncertifiedRoute != jsdocUncertifiedGapRoute {
					t.Fatalf("uncertified compact route=%q, want %q", uncertifiedRoute, jsdocUncertifiedGapRoute)
				}
				t.Logf("uncertified compact route=%s digest=%s", uncertifiedRoute, uncertifiedDigest)
			}

			forestRoute, forestDigest := jsdocForestRoute(t, language, test.source, productionDigest)
			if forestRoute != test.forestRoute {
				t.Fatalf("forest route=%q, want %q", forestRoute, test.forestRoute)
			}
			t.Logf("forest route=%s digest=%s", forestRoute, forestDigest)

			incrementalRoute, incrementalDigest := jsdocIncrementalRoute(t, language, test.source, productionDigest)
			if incrementalRoute != test.incrementalRoute {
				t.Fatalf("incremental route=%q, want %q", incrementalRoute, test.incrementalRoute)
			}
			t.Logf("incremental route=%s digest=%s", incrementalRoute, incrementalDigest)
		})
	}
}

func jsdocTreeDigest(t *testing.T, tree *gotreesitter.Tree, language *gotreesitter.Language) string {
	t.Helper()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	return inspection.SHA256
}

func jsdocRequireNoDispatchRewrites(t *testing.T, tree *gotreesitter.Tree, route string) {
	t.Helper()
	runtime := tree.ParseRuntime()
	if runtime.NormalizationNodesRewritten != 0 {
		t.Fatalf("%s route rewrote %d JSDoc nodes", route, runtime.NormalizationNodesRewritten)
	}
	if runtime.NormalizationPasses != nil {
		for _, pass := range *runtime.NormalizationPasses {
			if pass.Name == "dispatch.jsdoc" {
				t.Fatalf("%s route unexpectedly recorded retired dispatch.jsdoc: %+v", route, pass)
			}
		}
	}
}

func jsdocRequireDirectBypass(t *testing.T, tree *gotreesitter.Tree, route string) {
	t.Helper()
	runtime := tree.ParseRuntime()
	if runtime.NormalizationPassesRun != 0 || runtime.NormalizationNodesRewritten != 0 {
		t.Fatalf("%s route ran normalization: passes=%d rewrites=%d", route, runtime.NormalizationPassesRun, runtime.NormalizationNodesRewritten)
	}
	if runtime.NormalizationPasses != nil {
		for _, pass := range *runtime.NormalizationPasses {
			if pass.Name == "dispatch.jsdoc" {
				t.Fatalf("%s route unexpectedly recorded dispatch.jsdoc: %+v", route, pass)
			}
		}
	}
}

func jsdocRequireNoDispatchRewritesOrBypass(t *testing.T, tree *gotreesitter.Tree, route string) {
	t.Helper()
	jsdocRequireNoDispatchRewrites(t, tree, route)
}

func jsdocCompactRoute(t *testing.T, language *gotreesitter.Language, source []byte, wantDigest string) (string, string) {
	t.Helper()
	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	parser := gotreesitter.NewParser(language)
	parser.SetAdmissionCandidateRoute(true)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("compact parse: %v", err)
	}
	t.Cleanup(tree.Release)
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	digest := jsdocTreeDigest(t, tree, language)
	if digest != wantDigest {
		t.Fatalf("compact digest=%s production digest=%s", digest, wantDigest)
	}
	switch {
	case routedAfter == routedBefore+1 && fallbackAfter == fallbackBefore:
		jsdocRequireDirectBypass(t, tree, "compact direct")
		return "direct", digest
	case routedAfter == routedBefore && fallbackAfter == fallbackBefore+1:
		reason := gotreesitter.AdmissionCandidateLastFallbackReason()
		if reason == "" {
			t.Fatal("compact fallback has no reason")
		}
		jsdocRequireNoDispatchRewrites(t, tree, "compact fallback")
		return "fallback:" + reason, digest
	default:
		t.Fatalf("compact counters routed=%d/%d fallback=%d/%d", routedBefore, routedAfter, fallbackBefore, fallbackAfter)
		return "", ""
	}
}

func jsdocForestRoute(t *testing.T, language *gotreesitter.Language, source []byte, wantDigest string) (string, string) {
	t.Helper()
	parser := gotreesitter.NewParser(language)
	tree, ok := parser.ParseForestExperimental(source)
	if !ok || tree == nil {
		offset, symbol, reason, _ := parser.ForestDeclineInfo()
		if reason == "" {
			t.Fatalf("forest declined without a reason at %d symbol=%d", offset, symbol)
		}
		route := fmt.Sprintf("forest fallback at %d:%d:%s", offset, symbol, reason)
		fallback, err := parser.Parse(source)
		if err != nil {
			t.Fatalf("%s production parse: %v", route, err)
		}
		t.Cleanup(fallback.Release)
		digest := jsdocTreeDigest(t, fallback, language)
		if digest != wantDigest {
			t.Fatalf("%s digest=%s production digest=%s", route, digest, wantDigest)
		}
		jsdocRequireNoDispatchRewritesOrBypass(t, fallback, route)
		return fmt.Sprintf("fallback:%d:%d:%s", offset, symbol, reason), digest
	}
	t.Cleanup(tree.Release)
	if !tree.ParseRuntime().ForestFastPath {
		t.Fatal("forest parse did not report the forest route")
	}
	digest := jsdocTreeDigest(t, tree, language)
	if digest != wantDigest {
		t.Fatalf("forest digest=%s production digest=%s", digest, wantDigest)
	}
	jsdocRequireDirectBypass(t, tree, "forest direct")
	return "direct", digest
}

func jsdocIncrementalRoute(t *testing.T, language *gotreesitter.Language, source []byte, wantDigest string) (string, string) {
	t.Helper()
	parser := gotreesitter.NewParser(language)
	parser.SetAdmissionCandidateRoute(false)
	variant := append(append([]byte(nil), source...), ' ')
	oldTree, err := parser.Parse(variant)
	if err != nil {
		t.Fatalf("incremental variant base parse: %v", err)
	}
	t.Cleanup(oldTree.Release)
	startPoint := pointAtByte(source, len(source))
	oldEndPoint := pointAtByte(variant, len(variant))
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte:   uint32(len(source)),
		OldEndByte:  uint32(len(variant)),
		NewEndByte:  uint32(len(source)),
		StartPoint:  startPoint,
		OldEndPoint: oldEndPoint,
		NewEndPoint: startPoint,
	})
	tree, profile, err := parser.ParseIncrementalProfiled(source, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	t.Cleanup(tree.Release)
	digest := jsdocTreeDigest(t, tree, language)
	if digest != wantDigest {
		t.Fatalf("incremental digest=%s production digest=%s profile=%+v", digest, wantDigest, profile)
	}
	if profile.ReuseUnsupported {
		if profile.ReuseUnsupportedReason == "" {
			t.Fatalf("incremental fallback has no reason: %+v", profile)
		}
		jsdocRequireNoDispatchRewrites(t, tree, "incremental fallback")
		return "fallback:" + profile.ReuseUnsupportedReason, digest
	}
	jsdocRequireDirectBypass(t, tree, "incremental reuse")
	if !profile.OldTreeReuseRoute || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
		t.Fatalf("incremental route did not reuse the old tree: %+v", profile)
	}
	return fmt.Sprintf("reuse:%d:%d", profile.ReusedSubtrees, profile.ReusedBytes), digest
}
