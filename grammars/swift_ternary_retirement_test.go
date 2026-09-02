//go:build !grammar_subset

package grammars

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

const (
	swiftTernaryRetirementManifestDigest = "49f57837686d0ea7e070cd08e14ab05dc1b7c128a540ae5aa6c155435a6e18e9"
	swiftTernaryPositiveControlCommit    = "685ef7f885aa697e8a842de56de414d2a25cf0d8"
	swiftTernaryProducerFixCommit        = "71180718521aa6cf53fa4122a50998a7a2ef8020"
)

type swiftTernaryRetirementManifest struct {
	Schema string                       `json:"schema"`
	Cases  []swiftTernaryRetirementCase `json:"cases"`
}

type swiftTernaryRetirementCase struct {
	Name   string `json:"name"`
	Origin string `json:"origin"`
	Source string `json:"source"`
}

func TestSwiftTernaryRetirementRoutes(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := SwiftLanguage()
	manifest := loadSwiftTernaryRetirementManifest(t)

	for _, test := range manifest.Cases {
		t.Run(test.Name, func(t *testing.T) {
			source := []byte(test.Source)

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatalf("raw parse: %v", err)
			}
			t.Cleanup(raw.Release)

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			t.Cleanup(production.Release)
			rawInspection, err := benchfixtures.InspectGoTree(raw.RootNode(), language)
			if err != nil {
				t.Fatal(err)
			}
			wantDigest := requireSwiftTernaryRetirementTree(t, "production", production, language, source)
			requireSwiftTernaryRetiredCensus(t, "production", production, true)
			if rawInspection.SHA256 != wantDigest {
				if test.Name != "compat-as-if-condition" {
					t.Fatalf("raw digest = %s, want production %s", rawInspection.SHA256, wantDigest)
				}
				requireSwiftTernaryConditionsRewrite(t, production)
			} else if test.Name == "compat-as-if-condition" {
				t.Fatal("condition-recovery control no longer differs before compatibility")
			}

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(source)
			if err != nil {
				t.Fatalf("compact parse: %v", err)
			}
			t.Cleanup(compact.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			direct := routedAfter == routedBefore+1 && fallbackAfter == fallbackBefore
			fallback := routedAfter == routedBefore && fallbackAfter == fallbackBefore+1
			if !direct && !fallback {
				t.Fatalf(
					"compact route counters routed=%d/%d fallback=%d/%d: %s",
					routedBefore,
					routedAfter,
					fallbackBefore,
					fallbackAfter,
					gotreesitter.AdmissionCandidateLastFallbackReason(),
				)
			}
			if got := requireSwiftTernaryRetirementTree(t, "compact", compact, language, source); got != wantDigest {
				t.Fatalf("compact digest = %s, want raw %s", got, wantDigest)
			}
			requireSwiftTernaryRetiredCensus(t, "compact", compact, fallback)

			forestParser := gotreesitter.NewParser(language)
			forest, ok := forestParser.ParseForestExperimental(source)
			if !ok || forest == nil {
				offset, symbol, reason, _ := forestParser.ForestDeclineInfo()
				if test.Name != "compat-as-if-condition" || offset != 22 || symbol != 194 || reason != "dead_end" {
					t.Fatalf("forest declined at %d symbol=%d reason=%s", offset, symbol, reason)
				}
			} else {
				t.Cleanup(forest.Release)
				if !forest.ParseRuntime().ForestFastPath {
					t.Fatal("forest parse did not report the forest route")
				}
				if got := requireSwiftTernaryRetirementTree(t, "forest", forest, language, source); got != wantDigest {
					t.Fatalf("forest digest = %s, want raw %s", got, wantDigest)
				}
				requireSwiftTernaryRetiredCensus(t, "forest", forest, true)
			}

			baseSource, edit := swiftTernaryIncrementalBase(source)
			incrementalParser := gotreesitter.NewParser(language)
			incrementalParser.SetAdmissionCandidateRoute(false)
			oldTree, err := incrementalParser.Parse(baseSource)
			if err != nil {
				t.Fatalf("incremental base parse: %v", err)
			}
			t.Cleanup(oldTree.Release)
			oldTree.Edit(edit)
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			t.Cleanup(incremental.Release)
			if !profile.OldTreeReuseRoute && (!profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "external_scanner_unsupported") {
				t.Fatalf("incremental route receipt is not classified: %+v", profile)
			}
			if got := requireSwiftTernaryRetirementTree(t, "incremental", incremental, language, source); got != wantDigest {
				t.Fatalf("incremental digest = %s, want raw %s", got, wantDigest)
			}
			requireSwiftTernaryRetiredCensus(t, "incremental", incremental, true)
		})
	}
}

func loadSwiftTernaryRetirementManifest(t *testing.T) swiftTernaryRetirementManifest {
	t.Helper()
	path := filepath.Join("..", "testdata", "swift_ternary_retirement_cases_v1.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if got := fmt.Sprintf("%x", sum); got != swiftTernaryRetirementManifestDigest {
		t.Fatalf("manifest digest = %s, want %s", got, swiftTernaryRetirementManifestDigest)
	}
	var manifest swiftTernaryRetirementManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != "swift-ternary-retirement-cases-v1" || len(manifest.Cases) != 16 {
		t.Fatalf("manifest schema/cases = %q/%d", manifest.Schema, len(manifest.Cases))
	}
	requireSwiftTernaryManifestIdentity(t, manifest)
	return manifest
}

func requireSwiftTernaryManifestIdentity(t *testing.T, manifest swiftTernaryRetirementManifest) {
	t.Helper()
	names := make(map[string]bool, len(manifest.Cases))
	sources := make(map[string]bool, len(manifest.Cases))
	origins := make(map[string]int, 2)
	for _, test := range manifest.Cases {
		if test.Name == "" || names[test.Name] {
			t.Fatalf("manifest has an empty or duplicate name %q", test.Name)
		}
		if test.Source == "" || sources[test.Source] {
			t.Fatalf("manifest has an empty or duplicate source for %q", test.Name)
		}
		names[test.Name] = true
		sources[test.Source] = true
		origins[test.Origin]++
	}
	if len(origins) != 2 || origins[swiftTernaryPositiveControlCommit] != 11 || origins[swiftTernaryProducerFixCommit] != 5 {
		t.Fatalf("manifest origin counts = %+v, want 11 positive controls and 5 producer controls", origins)
	}
}

func requireSwiftTernaryRetirementTree(
	t *testing.T,
	route string,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	source []byte,
) string {
	t.Helper()
	root := tree.RootNode()
	if root == nil || root.HasError() || root.StartByte() != 0 || root.EndByte() != uint32(len(source)) {
		t.Fatalf("%s root is not clean and full-span: %v", route, root)
	}
	if countSwiftNodeType(language, root, "ternary_expression") == 0 {
		t.Fatalf("%s tree has no ternary expression: %s", route, root.SExpr(language))
	}
	inspection, err := benchfixtures.InspectGoTree(root, language)
	if err != nil {
		t.Fatal(err)
	}
	return inspection.SHA256
}

func requireSwiftTernaryRetiredCensus(
	t *testing.T,
	route string,
	tree *gotreesitter.Tree,
	required bool,
) {
	t.Helper()
	runtime := tree.ParseRuntime()
	if runtime.NormalizationPasses == nil {
		if required {
			t.Fatalf("%s route has no normalization census", route)
		}
		return
	}
	wantLive := map[string]bool{
		"dispatch.swift.conditions": false,
		"dispatch.swift.top-level":  false,
		"dispatch.swift.control":    false,
	}
	for _, pass := range *runtime.NormalizationPasses {
		if pass.Name == "dispatch.swift.ternary" {
			t.Fatalf("%s route still reports the retired Swift ternary pass", route)
		}
		if _, ok := wantLive[pass.Name]; ok {
			wantLive[pass.Name] = true
		}
	}
	for name, found := range wantLive {
		if !found {
			t.Fatalf("%s route has no %s census", route, name)
		}
	}
}

func requireSwiftTernaryConditionsRewrite(t *testing.T, tree *gotreesitter.Tree) {
	t.Helper()
	runtime := tree.ParseRuntime()
	if runtime.NormalizationPasses == nil {
		t.Fatal("raw and production differ without a normalization census")
	}
	for _, pass := range *runtime.NormalizationPasses {
		if pass.Name == "dispatch.swift.conditions" && pass.NodesRewritten > 0 {
			return
		}
	}
	t.Fatal("raw and production differ without a Swift conditions rewrite")
}

func swiftTernaryIncrementalBase(source []byte) ([]byte, gotreesitter.InputEdit) {
	if len(source) > 0 && source[len(source)-1] == '\n' {
		base := append([]byte(nil), source[:len(source)-1]...)
		start := retiredDispatchPointAtByte(base, len(base))
		end := retiredDispatchPointAtByte(source, len(source))
		return base, gotreesitter.InputEdit{
			StartByte:   uint32(len(base)),
			OldEndByte:  uint32(len(base)),
			NewEndByte:  uint32(len(source)),
			StartPoint:  start,
			OldEndPoint: start,
			NewEndPoint: end,
		}
	}
	base := append(append([]byte(nil), source...), '\n')
	start := retiredDispatchPointAtByte(source, len(source))
	oldEnd := retiredDispatchPointAtByte(base, len(base))
	return base, gotreesitter.InputEdit{
		StartByte:   uint32(len(source)),
		OldEndByte:  uint32(len(base)),
		NewEndByte:  uint32(len(source)),
		StartPoint:  start,
		OldEndPoint: oldEnd,
		NewEndPoint: start,
	}
}
