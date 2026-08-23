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

type wgslNextWitness struct {
	name   string
	source []byte
	class  string
}

type wgslNextRouteExpectation struct {
	digest       string
	receipt      string
	diff         *DumpV1Divergence
	rootError    bool
	rootErrorSet bool
}

type wgslNextExpectation struct {
	cDigest       string
	wantError     bool
	compactRoute  string
	forestRoute   string
	reuseSubtrees uint64
	reuseBytes    uint64
	routes        map[string]wgslNextRouteExpectation
	forest        *wgslNextRouteExpectation
}

// TestWGSLNextLiveArmLockedCRoutes records every WGSL route for A0 and controls.
func TestWGSLNextLiveArmLockedCRoutes(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	t.Setenv("GOT_PARSE_PHASE_TIMING", "1")
	goLanguage := grammars.WgslLanguage()
	cLanguage, err := COracleLanguage("wgsl")
	if err != nil {
		t.Fatal(err)
	}

	witnesses := []wgslNextWitness{
		{name: "a0-small-fragmentTextureQuad", source: wgslReadA0(t, "small__fragmentTextureQuad.wgsl"), class: "a0"},
		{name: "a0-medium-normalMap", source: wgslReadA0(t, "medium__normalMap.wgsl"), class: "a0"},
		{name: "a0-medium-radiosity", source: wgslReadA0(t, "medium__radiosity.wgsl"), class: "a0"},
		{name: "recovery-empty-return", source: []byte("fn malformed() { return; }\n"), class: "recovery"},
		{name: "malformed-missing-expression", source: []byte("fn malformed() { let value: f32 = ; }\n"), class: "malformed"},
		{name: "malformed-argument-list", source: []byte("fn malformed() { textureLoad(texture, coord,); }\n"), class: "malformed"},
		{name: "positive-control", source: []byte("fn identity(value: f32) -> f32 { return value; }\n"), class: "control"},
	}

	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(witness.source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parser returned no tree")
			}
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			want := wgslNextWant(witness.name)
			if cDigest != want.cDigest {
				t.Fatalf("locked C digest=%s, want %s", cDigest, want.cDigest)
			}

			raw := wgslParseRoute(t, goLanguage, witness.source, "raw", func(p *gotreesitter.Parser, source []byte) (*gotreesitter.Tree, error) {
				return p.ParseNoResultCompatibilityBenchmarkOnly(source)
			})
			production := wgslParseRoute(t, goLanguage, witness.source, "production", func(p *gotreesitter.Parser, source []byte) (*gotreesitter.Tree, error) {
				return p.Parse(source)
			})
			compactParser := gotreesitter.NewParser(goLanguage)
			compactParser.SetAdmissionCandidateRoute(true)
			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compact := wgslParseRoute(t, goLanguage, witness.source, "compact", func(_ *gotreesitter.Parser, source []byte) (*gotreesitter.Tree, error) {
				return compactParser.Parse(source)
			})
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			compactRoute := "accepted"
			if fallbackAfter > fallbackBefore {
				compactRoute = "fallback:" + gotreesitter.AdmissionCandidateLastFallbackReason()
			}

			forestParser := gotreesitter.NewParser(goLanguage)
			forest, forestOK := forestParser.ParseForestExperimental(witness.source)
			forestRoute := "declined"
			if forestOK && forest != nil {
				forestRoute = "accepted"
				t.Cleanup(forest.Release)
			} else {
				offset, symbol, reason, _ := forestParser.ForestDeclineInfo()
				forestRoute = fmt.Sprintf("declined:%s@%d/%d", reason, offset, symbol)
			}

			base := bytes.TrimSuffix(witness.source, []byte{'\n'})
			incrementalParser := gotreesitter.NewParser(goLanguage)
			oldTree, err := incrementalParser.Parse(base)
			if err != nil {
				t.Fatalf("incremental base parse: %v", err)
			}
			t.Cleanup(oldTree.Release)
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(base)),
				OldEndByte:  uint32(len(base)),
				NewEndByte:  uint32(len(witness.source)),
				StartPoint:  wgslPointAtByte(base),
				OldEndPoint: wgslPointAtByte(base),
				NewEndPoint: wgslPointAtByte(witness.source),
			})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(witness.source, oldTree)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			t.Cleanup(incremental.Release)

			for _, route := range []struct {
				name string
				tree *gotreesitter.Tree
			}{
				{name: "raw", tree: raw},
				{name: "production", tree: production},
				{name: "compact", tree: compact},
				{name: "incremental", tree: incremental},
			} {
				if route.tree == nil || route.tree.RootNode() == nil {
					t.Fatalf("%s route returned no root", route.name)
				}
				inspection, err := benchfixtures.InspectGoTree(route.tree.RootNode(), goLanguage)
				if err != nil {
					t.Fatalf("%s inspect Go tree: %v", route.name, err)
				}
				diff := FirstDivergenceDumpV1(route.tree.RootNode(), goLanguage, cTree.RootNode())
				expectation, ok := want.routes[route.name]
				if !ok {
					t.Fatalf("missing %s expectation", route.name)
				}
				wantError := want.wantError
				if expectation.rootErrorSet {
					wantError = expectation.rootError
				}
				if route.tree.RootNode().HasError() != wantError {
					t.Fatalf("%s root error=%t, want %t", route.name, route.tree.RootNode().HasError(), wantError)
				}
				if inspection.SHA256 != expectation.digest {
					t.Fatalf("%s digest=%s, want %s", route.name, inspection.SHA256, expectation.digest)
				}
				if got := wgslDispatchReceipt(route.tree); got != expectation.receipt {
					t.Fatalf("%s dispatch receipt=%s, want %s", route.name, got, expectation.receipt)
				}
				wgslRequireDivergence(t, route.name, diff, expectation.diff)
				t.Logf("witness=%s class=%s route=%s bytes=%d source_sha256=%x root_error=%t digest=%s c_digest=%s divergence=%+v dispatch=%s", witness.name, witness.class, route.name, len(witness.source), sha256.Sum256(witness.source), route.tree.RootNode().HasError(), inspection.SHA256, cDigest, diff, wgslDispatchReceipt(route.tree))
			}
			if forest != nil {
				inspection, err := benchfixtures.InspectGoTree(forest.RootNode(), goLanguage)
				if err != nil {
					t.Fatalf("forest inspect Go tree: %v", err)
				}
				diff := FirstDivergenceDumpV1(forest.RootNode(), goLanguage, cTree.RootNode())
				if want.forest == nil {
					t.Fatalf("forest returned a tree, want decline %s", want.forestRoute)
				}
				if forest.RootNode().HasError() != want.wantError {
					t.Fatalf("forest root error=%t, want %t", forest.RootNode().HasError(), want.wantError)
				}
				if inspection.SHA256 != want.forest.digest {
					t.Fatalf("forest digest=%s, want %s", inspection.SHA256, want.forest.digest)
				}
				if got := wgslDispatchReceipt(forest); got != want.forest.receipt {
					t.Fatalf("forest dispatch receipt=%s, want %s", got, want.forest.receipt)
				}
				wgslRequireDivergence(t, "forest", diff, want.forest.diff)
				t.Logf("witness=%s class=%s route=forest bytes=%d source_sha256=%x root_error=%t digest=%s c_digest=%s divergence=%+v dispatch=%s", witness.name, witness.class, len(witness.source), sha256.Sum256(witness.source), forest.RootNode().HasError(), inspection.SHA256, cDigest, diff, wgslDispatchReceipt(forest))
			} else if want.forest != nil {
				t.Fatalf("forest declined, want digest %s", want.forest.digest)
			}
			if compactRoute != want.compactRoute {
				t.Fatalf("compact route=%q, want %q", compactRoute, want.compactRoute)
			}
			if forestRoute != want.forestRoute {
				t.Fatalf("forest route=%q, want %q", forestRoute, want.forestRoute)
			}
			if profile.OldTreeReuseRoute != true || profile.ReuseUnsupported {
				t.Fatalf("incremental reuse route=%t unsupported=%t reason=%q, want reuse without fallback", profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason)
			}
			if profile.ReusedSubtrees != want.reuseSubtrees || profile.ReusedBytes != want.reuseBytes {
				t.Fatalf("incremental reuse=%d subtrees/%d bytes, want %d/%d", profile.ReusedSubtrees, profile.ReusedBytes, want.reuseSubtrees, want.reuseBytes)
			}
			t.Logf("witness=%s compact=%s counters=%d/%d->%d/%d forest=%s incremental_reuse=%t reuse_unsupported=%t reuse_reason=%q reused_subtrees=%d reused_bytes=%d", witness.name, compactRoute, routedBefore, fallbackBefore, routedAfter, fallbackAfter, forestRoute, profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason, profile.ReusedSubtrees, profile.ReusedBytes)
		})
	}
}

func wgslRequireDivergence(t *testing.T, route string, got, want *DumpV1Divergence) {
	t.Helper()
	if got == nil || want == nil {
		if got != want {
			t.Fatalf("%s divergence=%+v, want %+v", route, got, want)
		}
		return
	}
	if *got != *want {
		t.Fatalf("%s divergence=%+v, want %+v", route, got, want)
	}
}

func wgslDiff(path, category, goValue, cValue string) *DumpV1Divergence {
	return &DumpV1Divergence{Path: path, Category: category, GoValue: goValue, CValue: cValue}
}

func wgslNextWant(name string) wgslNextExpectation {
	switch name {
	case "a0-small-fragmentTextureQuad":
		const digest = "d3e58954c750ed560edd3177a165bbf701c159467a1b4677996bec620c377804"
		return wgslNextExpectation{
			cDigest:       digest,
			compactRoute:  "accepted",
			forestRoute:   "accepted",
			reuseSubtrees: 49,
			reuseBytes:    192,
			routes: map[string]wgslNextRouteExpectation{
				"raw":         {digest: digest, receipt: "none"},
				"production":  {digest: digest, receipt: "none"},
				"compact":     {digest: digest, receipt: "none"},
				"incremental": {digest: digest, receipt: "1/1/112/51"},
			},
			forest: &wgslNextRouteExpectation{digest: digest, receipt: "1/1/111/0"},
		}
	case "a0-medium-normalMap":
		return wgslNextExpectation{
			cDigest:       "231e10ca2215945a5fb51670620c9f5ba2ea1ca7d445cb2c9443fb51b8e0e18a",
			wantError:     true,
			compactRoute:  "fallback:compact route declined at recovery [mechanism=recovery-entered]: did not accept EOF: generic scheduler has no table action for the elected token",
			forestRoute:   "declined:dead_end@0/1",
			reuseSubtrees: 452,
			reuseBytes:    2318,
			routes: map[string]wgslNextRouteExpectation{
				"raw":         {digest: "77fdfd002d6937e6f5784fc19e21a6f63ab8f2280ca8ba0dcfc1ee5b1d3d42cc", receipt: "none", diff: wgslDiff("/source_file", "shape", "children=70", "children=79")},
				"production":  {digest: "9d802a0e9af71176c0520496ae99425406aa76dc8560f8c7e939e366f9fbbd44", receipt: "1/1/1289/120", diff: wgslDiff("/source_file", "shape", "children=70", "children=79")},
				"compact":     {digest: "9d802a0e9af71176c0520496ae99425406aa76dc8560f8c7e939e366f9fbbd44", receipt: "1/1/1289/120", diff: wgslDiff("/source_file", "shape", "children=70", "children=79")},
				"incremental": {digest: "fe0cb9f758eaace140619c81a9ef3347d89633580b18492e46dcfae6fb8f57c7", receipt: "1/1/1287/684", diff: wgslDiff("/source_file", "shape", "children=69", "children=79")},
			},
		}
	case "a0-medium-radiosity":
		return wgslNextExpectation{
			cDigest:       "22b9d004c33c6a8229b56876282125e04efddf59deef6224eddd61f38c9952b2",
			wantError:     true,
			compactRoute:  "fallback:compact route declined at recovery [mechanism=recovery-entered]: did not accept EOF: generic scheduler has no table action for the elected token",
			forestRoute:   "declined:dead_end@247/43",
			reuseSubtrees: 185,
			reuseBytes:    1197,
			routes: map[string]wgslNextRouteExpectation{
				"raw":         {digest: "c591b9329ad2fc946b6b8b7c4bc80adb7305f41934dd51d4505e4f606787b127", receipt: "none", diff: wgslDiff("/source_file/global_variable_declaration[3]/variable_declaration[2]/variable_identifier_declaration[2]/type_declaration[2]/ERROR[1]", "type", "ERROR", "<")},
				"production":  {digest: "592abfad21a9a3170c11fd2e18888a9c5ecac7f681af5cf81e2d1c352873df63", receipt: "1/1/1230/51", diff: wgslDiff("/source_file/function_declaration[29]", "error", "false", "true")},
				"compact":     {digest: "592abfad21a9a3170c11fd2e18888a9c5ecac7f681af5cf81e2d1c352873df63", receipt: "1/1/1230/51", diff: wgslDiff("/source_file/function_declaration[29]", "error", "false", "true")},
				"incremental": {digest: "9e0113134b748e66ba22390bdf09e6e083a7f09dc0fc3b1dec67a407fed979cd", receipt: "1/1/1230/56", diff: wgslDiff("/source_file", "shape", "children=46", "children=48")},
			},
		}
	case "recovery-empty-return":
		const digest = "909c5e3b5efd2201372daf83848ea91cc459c1069537f89fcebf1f7c49cd58f4"
		return wgslNextExpectation{
			cDigest:       digest,
			compactRoute:  "accepted",
			forestRoute:   "accepted",
			reuseSubtrees: 6,
			reuseBytes:    20,
			routes: map[string]wgslNextRouteExpectation{
				"raw":         {digest: digest, receipt: "none"},
				"production":  {digest: digest, receipt: "none"},
				"compact":     {digest: digest, receipt: "none"},
				"incremental": {digest: digest, receipt: "1/1/12/0"},
			},
			forest: &wgslNextRouteExpectation{digest: digest, receipt: "1/1/12/0"},
		}
	case "malformed-missing-expression":
		const digest = "dcdd782dbf759ef627faf475817ba14b68ae5b0783e431fc06a1bee5a879e73b"
		const normalizedDigest = "88373bc4d0bc650c42c159273d5cc2a6915b2c69334adc3d5e8923a421fce02b"
		return wgslNextExpectation{
			cDigest:       digest,
			wantError:     true,
			compactRoute:  "fallback:compact route declined at recovery [mechanism=recovery-entered]: did not accept EOF: generic scheduler has no table action for the elected token",
			forestRoute:   "declined:dead_end@34/3",
			reuseSubtrees: 9,
			reuseBytes:    26,
			routes: map[string]wgslNextRouteExpectation{
				"raw":         {digest: digest, receipt: "none"},
				"production":  {digest: normalizedDigest, receipt: "1/1/19/5", diff: wgslDiff("/source_file", "error", "false", "true"), rootErrorSet: true, rootError: false},
				"compact":     {digest: normalizedDigest, receipt: "1/1/19/5", diff: wgslDiff("/source_file", "error", "false", "true"), rootErrorSet: true, rootError: false},
				"incremental": {digest: normalizedDigest, receipt: "1/1/19/5", diff: wgslDiff("/source_file", "error", "false", "true"), rootErrorSet: true, rootError: false},
			},
		}
	case "malformed-argument-list":
		return wgslNextExpectation{
			cDigest:       "4a43e477628bba014be5e5863dc87ab460c14c7232c96aa737414f299a407e81",
			wantError:     true,
			compactRoute:  "fallback:compact route declined at recovery [mechanism=recovery-entered]: did not accept EOF: generic scheduler has no table action for the elected token",
			forestRoute:   "declined:dead_end@28/7",
			reuseSubtrees: 6,
			reuseBytes:    25,
			routes: map[string]wgslNextRouteExpectation{
				"raw":         {digest: "40a24a35095668102b40bd1bd92f5576211eaf92b3aac01f24b5e81a6ab755a9", receipt: "none", diff: wgslDiff("/source_file/function_declaration[0]/compound_statement[4]/assignment_statement[1]/parenthesized_expression[2]/ERROR[2]/,[0]", "error", "true", "false")},
				"production":  {digest: "2555f0b34dfb5e295bc72d8d5532ceadfd6807686edc2e417a5e8ae911aa9df7", receipt: "1/1/23/5", diff: wgslDiff("/source_file/function_declaration[0]/compound_statement[4]/assignment_statement[1]/compound_assignment_operator[1]", "error", "false", "true")},
				"compact":     {digest: "2555f0b34dfb5e295bc72d8d5532ceadfd6807686edc2e417a5e8ae911aa9df7", receipt: "1/1/23/5", diff: wgslDiff("/source_file/function_declaration[0]/compound_statement[4]/assignment_statement[1]/compound_assignment_operator[1]", "error", "false", "true")},
				"incremental": {digest: "2555f0b34dfb5e295bc72d8d5532ceadfd6807686edc2e417a5e8ae911aa9df7", receipt: "1/1/23/5", diff: wgslDiff("/source_file/function_declaration[0]/compound_statement[4]/assignment_statement[1]/compound_assignment_operator[1]", "error", "false", "true")},
			},
		}
	case "positive-control":
		const digest = "7868a6b43efc8d14b73746e2472a596c9eeb3cd215f734021528aaa3668f05ea"
		return wgslNextExpectation{
			cDigest:       digest,
			compactRoute:  "accepted",
			forestRoute:   "accepted",
			reuseSubtrees: 11,
			reuseBytes:    37,
			routes: map[string]wgslNextRouteExpectation{
				"raw":         {digest: digest, receipt: "none"},
				"production":  {digest: digest, receipt: "none"},
				"compact":     {digest: digest, receipt: "none"},
				"incremental": {digest: digest, receipt: "1/1/24/0"},
			},
			forest: &wgslNextRouteExpectation{digest: digest, receipt: "1/1/24/0"},
		}
	default:
		panic("missing WGSL expectation for " + name)
	}
}

func wgslReadA0(t *testing.T, name string) []byte {
	t.Helper()
	source, err := os.ReadFile(filepath.Join("..", "testdata", "dispatcher_census_a0", "wgsl", name))
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func wgslParseRoute(t *testing.T, language *gotreesitter.Language, source []byte, route string, parse func(*gotreesitter.Parser, []byte) (*gotreesitter.Tree, error)) *gotreesitter.Tree {
	t.Helper()
	parser := gotreesitter.NewParser(language)
	tree, err := parse(parser, source)
	if err != nil {
		t.Fatalf("%s parse: %v", route, err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("%s parse returned no tree", route)
	}
	t.Cleanup(tree.Release)
	return tree
}

func wgslDispatchReceipt(tree *gotreesitter.Tree) string {
	if tree == nil || tree.ParseRuntime().NormalizationPasses == nil {
		return "none"
	}
	for _, pass := range *tree.ParseRuntime().NormalizationPasses {
		if pass.Name == "dispatch.wgsl" {
			return fmt.Sprintf("%d/%d/%d/%d", pass.Checked, pass.Run, pass.NodesVisited, pass.NodesRewritten)
		}
	}
	return "none"
}

func wgslPointAtByte(source []byte) gotreesitter.Point {
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

// TestWGSLNextLiveArmReceiptDocument guards the blocker markers.
func TestWGSLNextLiveArmReceiptDocument(t *testing.T) {
	raw, err := os.ReadFile("../docs/root-normalization-retirement.md")
	if err != nil {
		t.Fatal(err)
	}
	document := strings.Join(strings.Fields(string(raw)), " ")
	for _, marker := range []string{
		"Status: NO-GO. KEEP LIVE: `dispatch.wgsl`.",
		"The WGSL A0 receipt contains three files, three checks, three runs, 2,630 visited nodes, 171 rewrites, two error roots, and zero parse errors.",
		"The tracked census has seven fixtures across six languages. It excludes WGSL.",
		"The focused receipt uses seven witnesses.",
		"The `normalMap` A0 witness exposes the first live blocker.",
		"The malformed missing-expression control matches locked C on its raw route.",
		"The malformed argument-list control keeps a live rewrite.",
		"The two malformed controls each add five rewrites.",
		"The probe changes only the receipt test, changelog, and retirement ledger.",
		"Keep `dispatch.wgsl` live until the producer emits the locked-C trees for all registered witnesses and all listed routes.",
	} {
		marker = strings.Join(strings.Fields(marker), " ")
		if !strings.Contains(document, marker) {
			t.Fatalf("WGSL blocker receipt lacks marker %q", marker)
		}
	}
}

// TestWGSLNextLiveChangelogReceipt guards the changelog marker.
func TestWGSLNextLiveChangelogReceipt(t *testing.T) {
	raw, err := os.ReadFile("../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	document := strings.Join(strings.Fields(string(raw)), " ")
	for _, marker := range []string{
		"Record the `dispatch.wgsl` blocker receipt at base `7498a678c52029a82f312e9637ecb66b15defa0b`.",
		"The A0 manifest records three WGSL files, three checks, three runs, and 171 rewrites.",
		"Both malformed controls rewrite five nodes on normalized routes. The production, compact, and incremental routes for both controls diverge from locked C.",
		"No registry or production code changes are included.",
	} {
		marker = strings.Join(strings.Fields(marker), " ")
		if !strings.Contains(document, marker) {
			t.Fatalf("WGSL changelog lacks marker %q", marker)
		}
	}
}
