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

type templNextWitness struct {
	name   string
	source []byte
}

// TestTemplNextLiveArmLockedCRoutes records Templ on raw, production, compact,
// forest, and incremental routes. It keeps rewrite and recovery evidence visible.
func TestTemplNextLiveArmLockedCRoutes(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	t.Setenv("GOT_PARSE_PHASE_TIMING", "1")
	goLanguage := grammars.TemplLanguage()
	cLanguage, err := COracleLanguage("templ")
	if err != nil {
		t.Fatal(err)
	}

	witnesses := []templNextWitness{
		{name: "a0-medium-main", source: templNextReadA0(t, "medium__main.templ")},
		{name: "a0-medium-template", source: templNextReadA0(t, "medium__template.templ")},
		{name: "a0-small-template", source: templNextReadA0(t, "small__template.templ")},
		{name: "positive-no-op-control", source: []byte("templ T() { <div>ok</div> }\n")},
		{name: "qualified-component-import", source: []byte("package main\n\ntempl T() {\n\t@templ.JSONScript(\"scriptData\", scriptData)\n}\n")},
		{name: "malformed-dangling-attribute-quote", source: []byte("templ Broken() {\n\t<a class=\"external-link\"\n\t\t>\n}\n")},
		{name: "malformed-component-import", source: []byte("package main\n\ntempl Broken() {\n\t@counts(global,\n}\n")},
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
			if want := templNextExpectedCDigest(witness.name); want != "" && cDigest != want {
				t.Fatalf("%s locked-C digest=%s, want %s", witness.name, cDigest, want)
			}

			raw := templNextParseRoute(t, goLanguage, witness.source, "raw", func(p *gotreesitter.Parser, source []byte) (*gotreesitter.Tree, error) {
				p.SetAdmissionCandidateRoute(false)
				return p.ParseNoResultCompatibilityBenchmarkOnly(source)
			})
			production := templNextParseRoute(t, goLanguage, witness.source, "production", func(p *gotreesitter.Parser, source []byte) (*gotreesitter.Tree, error) {
				p.SetAdmissionCandidateRoute(false)
				return p.Parse(source)
			})

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(goLanguage)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(witness.source)
			if err != nil {
				t.Fatalf("compact parse: %v", err)
			}
			t.Cleanup(compact.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			compactRoute := "accepted"
			if fallbackAfter > fallbackBefore {
				compactRoute = "fallback:" + gotreesitter.AdmissionCandidateLastFallbackReason()
			}

			forestParser := gotreesitter.NewParser(goLanguage)
			forest, forestOK := forestParser.ParseForestExperimental(witness.source)
			forestRoute := "declined"
			forestDetail := ""
			if forestOK && forest != nil {
				forestRoute = "accepted"
				t.Cleanup(forest.Release)
			} else {
				offset, symbol, reason, _ := forestParser.ForestDeclineInfo()
				forestDetail = fmt.Sprintf("offset=%d symbol=%d reason=%q", offset, symbol, reason)
			}

			incremental, profile := templNextIncrementalRoute(t, goLanguage, witness.source)

			templNextLogRoute(t, "raw", raw, goLanguage, cTree, cDigest, "")
			templNextAssertKnownRoute(t, witness.name, "raw", raw, goLanguage, cTree)
			templNextLogRoute(t, "production", production, goLanguage, cTree, cDigest, "")
			templNextAssertKnownRoute(t, witness.name, "production", production, goLanguage, cTree)
			templNextLogRoute(t, "compact", compact, goLanguage, cTree, cDigest, compactRoute)
			templNextAssertKnownRoute(t, witness.name, "compact", compact, goLanguage, cTree)
			if forest != nil {
				templNextLogRoute(t, "forest", forest, goLanguage, cTree, cDigest, forestRoute)
				templNextAssertKnownRoute(t, witness.name, "forest", forest, goLanguage, cTree)
			} else {
				t.Logf("route=forest witness=%s result=declined %s", witness.name, forestDetail)
			}
			templNextLogRoute(t, "incremental", incremental, goLanguage, cTree, cDigest, fmt.Sprintf("reuse=%t unsupported=%t reason=%q reused_subtrees=%d reused_bytes=%d", profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason, profile.ReusedSubtrees, profile.ReusedBytes))
			templNextAssertKnownRoute(t, witness.name, "incremental", incremental, goLanguage, cTree)
			if profile.ReuseUnsupportedReason != "external_scanner_unsupported" || profile.OldTreeReuseRoute || !profile.ReuseUnsupported {
				t.Fatalf("incremental route profile = reuse=%t unsupported=%t reason=%q", profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason)
			}
			templNextAssertForestRoute(t, witness.name, forestParser, forestOK)
			t.Logf("witness=%s bytes=%d source_sha256=%x c_digest=%s compact=%s counters=%d/%d->%d/%d forest=%s %s incremental_reuse=%t incremental_unsupported=%t incremental_reason=%q", witness.name, len(witness.source), sha256.Sum256(witness.source), cDigest, compactRoute, routedBefore, fallbackBefore, routedAfter, fallbackAfter, forestRoute, forestDetail, profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason)
			templNextAssertCompactRoute(t, witness.name, compactRoute, routedAfter-routedBefore, fallbackAfter-fallbackBefore)
		})
	}
}

func templNextReadA0(t *testing.T, name string) []byte {
	t.Helper()
	source, err := os.ReadFile(filepath.Join("..", "testdata", "dispatcher_census_a0", "templ", name))
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func templNextParseRoute(t *testing.T, language *gotreesitter.Language, source []byte, route string, parse func(*gotreesitter.Parser, []byte) (*gotreesitter.Tree, error)) *gotreesitter.Tree {
	t.Helper()
	parser := gotreesitter.NewParser(language)
	tree, err := parse(parser, source)
	if err != nil {
		t.Fatalf("%s parse: %v", route, err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("%s returned no tree", route)
	}
	t.Cleanup(tree.Release)
	return tree
}

func templNextIncrementalRoute(t *testing.T, language *gotreesitter.Language, source []byte) (*gotreesitter.Tree, gotreesitter.IncrementalParseProfile) {
	t.Helper()
	if !bytes.HasSuffix(source, []byte{'\n'}) {
		t.Fatalf("incremental witness does not end with newline")
	}
	base := bytes.TrimSuffix(source, []byte{'\n'})
	parser := gotreesitter.NewParser(language)
	oldTree, err := parser.Parse(base)
	if err != nil {
		t.Fatalf("incremental base parse: %v", err)
	}
	t.Cleanup(oldTree.Release)
	point := templNextPointAtByte(base)
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte:   uint32(len(base)),
		OldEndByte:  uint32(len(base)),
		NewEndByte:  uint32(len(source)),
		StartPoint:  point,
		OldEndPoint: point,
		NewEndPoint: templNextPointAtByte(source),
	})
	tree, profile, err := parser.ParseIncrementalProfiled(source, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("incremental returned no tree")
	}
	t.Cleanup(tree.Release)
	return tree, profile
}

func templNextPointAtByte(source []byte) gotreesitter.Point {
	var point gotreesitter.Point
	for _, b := range source {
		if b == '\n' {
			point.Row++
			point.Column = 0
			continue
		}
		point.Column++
	}
	return point
}

func templNextLogRoute(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, cDigest, detail string) {
	t.Helper()
	root := tree.RootNode()
	inspection, err := benchfixtures.InspectGoTree(root, language)
	if err != nil {
		t.Fatalf("%s inspect Go tree: %v", route, err)
	}
	diff := FirstDivergenceDumpV1(root, language, cTree.RootNode())
	checked, run, visited, rewritten := templNextDispatchStats(tree)
	if detail != "" {
		detail = " " + detail
	}
	t.Logf("route=%s error=%t digest=%s c_digest=%s divergence=%+v dispatch_checked=%d dispatch_run=%d dispatch_visited=%d dispatch_rewritten=%d%s", route, root.HasError(), inspection.SHA256, cDigest, diff, checked, run, visited, rewritten, detail)
}

func templNextDispatchStats(tree *gotreesitter.Tree) (checked, run, visited, rewritten uint64) {
	runtime := tree.ParseRuntime()
	if runtime.NormalizationPasses == nil {
		return 0, 0, 0, 0
	}
	for _, pass := range *runtime.NormalizationPasses {
		if pass.Name != "dispatch.templ" {
			continue
		}
		checked += pass.Checked
		run += pass.Run
		visited += pass.NodesVisited
		rewritten += pass.NodesRewritten
	}
	return checked, run, visited, rewritten
}

type templNextExpectedRoute struct {
	digest  string
	error   bool
	rewrite uint64
	diff    *DumpV1Divergence
}

func templNextExpectedDiff(path, category, goValue, cValue string) *DumpV1Divergence {
	return &DumpV1Divergence{Path: path, Category: category, GoValue: goValue, CValue: cValue}
}

func templNextExpectedCDigest(witness string) string {
	switch witness {
	case "a0-medium-main":
		return "efab90f3a4a75a4deba8c94d67c741dd842a7c8c6708bed3f59e37e0a994a11f"
	case "a0-medium-template":
		return "7de9788750436a485bee98ec6200da09d5062700368333fe380562d71f171891"
	case "a0-small-template":
		return "cb81fe10587416eae568216d16d2f7258bda32d00136030d8a4fcd2198e12594"
	case "positive-no-op-control":
		return "ac11fb7f49572a2132e31c3a46328e17a103c4ae73fd75745427a188c5987e11"
	case "qualified-component-import":
		return "631955c93c466e736f59aa22039f2558b7685d3b4641b71b874cac52b5e70a23"
	case "malformed-dangling-attribute-quote":
		return "3e6eb2d96ca843d122d8f1e952fcd465feee7f67d289c8de118c08436f1b4491"
	case "malformed-component-import":
		return "922cf68ffcfd8e890f27d0cb8fb995b5a5fdfb10c3853bc661bd8c722bbd34cb"
	default:
		return ""
	}
}

func templNextExpectedRouteFor(witness, route string) (templNextExpectedRoute, bool) {
	shapeRaw := func(digest, path, goValue, cValue string) templNextExpectedRoute {
		return templNextExpectedRoute{digest: digest, diff: templNextExpectedDiff(path, "shape", goValue, cValue)}
	}
	shapeNormalized := func(digest string) templNextExpectedRoute {
		return shapeRaw(digest, "/source_file/component_declaration[26]/component_block[3]", "children=12", "children=11")
	}
	errorRoot := templNextExpectedDiff("/source_file", "error", "true", "false")
	malformedRoot := templNextExpectedDiff("/source_file", "error", "false", "true")
	switch witness {
	case "a0-medium-main":
		return templNextExpectedRoute{digest: "895657a1c4978896653cf968b2dedddf7badd40f464d558e97dd95a9d9675595", error: true, diff: errorRoot}, true
	case "a0-medium-template":
		if route == "raw" {
			return shapeRaw("33a54940b5da62255e5a03056b2ed7935994773b53a746b9a7e706b60a1a8dcb", "/source_file/component_declaration[26]/component_block[3]", "children=20", "children=11"), true
		}
		want := shapeNormalized("2499953c81a152ca9db474f121b1a8a9de0c888c6f00a25125301c157bcb0b0e")
		want.rewrite = 53
		return want, true
	case "a0-small-template":
		if route == "raw" {
			return shapeRaw("80e67baee0a78d252f4621c42b4eab3e1334bc919bdcba000d17034d04f954f3", "/source_file/component_declaration[3]/component_block[3]/element[2]/element[1]", "children=4", "children=3"), true
		}
		return templNextExpectedRoute{digest: "cb81fe10587416eae568216d16d2f7258bda32d00136030d8a4fcd2198e12594", rewrite: 23}, true
	case "positive-no-op-control":
		return templNextExpectedRoute{digest: "ac11fb7f49572a2132e31c3a46328e17a103c4ae73fd75745427a188c5987e11"}, true
	case "qualified-component-import":
		if route == "raw" {
			return shapeRaw("ed7d7f4155a6bdc6153038f7637dd2a14878abfcb26d27bbfc4a30529a4e0da7", "/source_file/component_declaration[1]/component_block[3]", "children=4", "children=3"), true
		}
		want := templNextExpectedRoute{digest: "631955c93c466e736f59aa22039f2558b7685d3b4641b71b874cac52b5e70a23"}
		if route == "production" || route == "forest" || route == "incremental" {
			want.rewrite = 15
		}
		return want, true
	case "malformed-dangling-attribute-quote":
		return templNextExpectedRoute{digest: "3e6eb2d96ca843d122d8f1e952fcd465feee7f67d289c8de118c08436f1b4491", error: true}, true
	case "malformed-component-import":
		return templNextExpectedRoute{digest: "8954432deb8e607319a63b17b665508434a8d87fac8c6fe9923ad1bbe062760f", diff: malformedRoot}, true
	default:
		return templNextExpectedRoute{}, false
	}
}

func templNextAssertKnownRoute(t *testing.T, witness, route string, tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree) {
	t.Helper()
	want, ok := templNextExpectedRouteFor(witness, route)
	if !ok {
		return
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatalf("%s/%s returned no root", witness, route)
	}
	if root.HasError() != want.error {
		t.Fatalf("%s/%s error_root=%t, want %t", witness, route, root.HasError(), want.error)
	}
	inspection, err := benchfixtures.InspectGoTree(root, language)
	if err != nil {
		t.Fatalf("%s/%s inspect: %v", witness, route, err)
	}
	if inspection.SHA256 != want.digest {
		t.Fatalf("%s/%s digest=%s, want %s", witness, route, inspection.SHA256, want.digest)
	}
	diff := FirstDivergenceDumpV1(root, language, cTree.RootNode())
	if want.diff == nil {
		if diff != nil {
			t.Fatalf("%s/%s divergence=%+v, want exact locked-C parity", witness, route, diff)
		}
	} else if diff == nil || *diff != *want.diff {
		t.Fatalf("%s/%s divergence=%+v, want %+v", witness, route, diff, want.diff)
	}
	_, _, _, rewritten := templNextDispatchStats(tree)
	if rewritten != want.rewrite {
		t.Fatalf("%s/%s dispatch.templ rewrites=%d, want %d", witness, route, rewritten, want.rewrite)
	}
}

func templNextAssertCompactRoute(t *testing.T, witness, route string, routedDelta, fallbackDelta uint64) {
	t.Helper()
	if routedDelta+fallbackDelta != 1 {
		t.Fatalf("%s compact counters routed_delta=%d fallback_delta=%d", witness, routedDelta, fallbackDelta)
	}
	wantFallback := witness == "a0-medium-main" || witness == "a0-medium-template" || witness == "a0-small-template" || witness == "malformed-dangling-attribute-quote"
	if wantFallback && (route == "accepted" || routedDelta != 0 || fallbackDelta != 1) {
		t.Fatalf("%s compact route=%q counters=%d/%d, want fallback", witness, route, routedDelta, fallbackDelta)
	}
	if !wantFallback && (route != "accepted" || routedDelta != 1 || fallbackDelta != 0) {
		t.Fatalf("%s compact route=%q counters=%d/%d, want accepted", witness, route, routedDelta, fallbackDelta)
	}
}

func templNextAssertForestRoute(t *testing.T, witness string, parser *gotreesitter.Parser, accepted bool) {
	t.Helper()
	wantDeclined := witness == "a0-medium-main" || witness == "malformed-dangling-attribute-quote"
	if wantDeclined != !accepted {
		t.Fatalf("%s forest accepted=%t, want %t", witness, accepted, !wantDeclined)
	}
	if wantDeclined {
		offset, symbol, reason, _ := parser.ForestDeclineInfo()
		if witness == "a0-medium-main" && (offset != 1025 || symbol != 74 || reason != "dead_end") {
			t.Fatalf("%s forest decline=%d/%d/%q, want 1025/74/dead_end", witness, offset, symbol, reason)
		}
		if witness == "malformed-dangling-attribute-quote" && (offset != 47 || symbol != 24 || reason != "dead_end") {
			t.Fatalf("%s forest decline=%d/%d/%q, want 47/24/dead_end", witness, offset, symbol, reason)
		}
	}
}

// TestTemplNextLiveArmReceiptDocument guards the final blocker receipt markers.
func TestTemplNextLiveArmReceiptDocument(t *testing.T) {
	raw, err := os.ReadFile("../docs/root-normalization-retirement.md")
	if err != nil {
		t.Fatal(err)
	}
	document := strings.ReplaceAll(strings.Join(strings.Fields(string(raw)), " "), "`", "")
	for _, marker := range []string{
		"Status: NO-GO. KEEP LIVE: dispatch.templ.",
		"The authenticated Templ A0 receipt records three files, three checks, three runs, 1138 visited nodes, 76 rewritten nodes, one error root, and zero parse errors.",
		"The medium template witness rewrites 53 nodes on production, compact, forest, and incremental routes.",
		"The full real-corpus census is unavailable because cgo_harness/corpus_real is absent.",
		"Do not change the registry or production state.",
	} {
		marker = strings.Join(strings.Fields(marker), " ")
		if !strings.Contains(document, marker) {
			t.Fatalf("Templ blocker receipt lacks marker %q", marker)
		}
	}
}
