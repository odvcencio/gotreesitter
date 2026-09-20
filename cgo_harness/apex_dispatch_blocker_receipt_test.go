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

type apexExpectedDivergence struct {
	path     string
	category string
	goValue  string
	cValue   string
}

// TestApexDispatchBlockerLockedCRoutes records all five routes for Apex
// witnesses. It requires zero rewrites only on raw, production, compact, and incremental routes.
func TestApexDispatchBlockerLockedCRoutes(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := grammars.ApexLanguage()
	cLanguage, err := COracleLanguage("apex")
	if err != nil {
		t.Fatal(err)
	}

	witnesses := []struct {
		name               string
		source             []byte
		wantForestRewrite  bool
		wantForestRewrites uint64
		wantError          bool
		expected           map[string]apexExpectedDivergence
	}{
		{
			name: "registered-class-literal-alias",
			source: []byte("public class C {\n" +
				"  void m() {\n" +
				"    Object t = RecordPage.class;\n" +
				"  }\n" +
				"}"),
			wantForestRewrite:  true,
			wantForestRewrites: 3,
			expected: apexExpectedRoutes(apexExpectedDivergence{
				path:     "/parser_output/class_declaration[0]/class_body[3]/method_declaration[1]/block[3]/local_variable_declaration[1]/variable_declarator[1]/field_access[2]/identifier[0]",
				category: "field",
				goValue:  "",
				cValue:   "object",
			}, "forest"),
		},
		{
			name: "registered-qualified-class-literal-alias",
			source: []byte("public class C {\n" +
				"  void m() {\n" +
				"    Object t = Outer.Inner.class;\n" +
				"  }\n" +
				"}"),
			expected: apexExpectedRoutes(apexExpectedDivergence{
				path:     "/parser_output/class_declaration[0]/class_body[3]/method_declaration[1]/block[3]/local_variable_declaration[1]/variable_declarator[1]/class_literal[2]",
				category: "type",
				goValue:  "class_literal",
				cValue:   "field_access",
			}, "forest"),
		},
		{
			name: "registered-nested-generic-local",
			source: []byte("public class C {\n" +
				"  void m() {\n" +
				"    List<List<SObject>> searchResults = [FIND :keyword IN ALL FIELDS];\n" +
				"  }\n" +
				"}"),
		},
		{
			name: "registered-right-shift",
			source: []byte("public class C {\n" +
				"  Integer m(Integer value) {\n" +
				"    return value >> 1;\n" +
				"  }\n" +
				"}"),
		},
		{
			name: "positive-plain-field-access",
			source: []byte("public class C {\n" +
				"  void m() {\n" +
				"    Object t = RecordPage.someField;\n" +
				"  }\n" +
				"}"),
		},
		{
			// The tree-sitter-sfapex da568eee bump (blob 28fc59c4) adds the
			// multi_line_string_literal rule and widens line_comment. That
			// reshapes recovery on this truncated source, and the recorded
			// /parser_output/ERROR[0]/void_type[4] field divergence stopped
			// occurring: all four routes now match the locked C oracle here.
			name:      "malformed-missing-class-body",
			source:    []byte("public class C { void m() { Object t = RecordPage.class;"),
			wantError: true,
		},
		{
			name:      "malformed-class-literal-dot",
			source:    []byte("public class C { void m() { Object t = RecordPage.; } }"),
			wantError: true,
			expected: apexExpectedRoutes(apexExpectedDivergence{
				path:     "/parser_output/class_declaration[0]/class_body[3]/method_declaration[1]/block[3]/local_variable_declaration[1]",
				category: "shape",
				goValue:  "children=3",
				cValue:   "children=4",
			}, "raw", "production", "compact", "incremental"),
		},
		{
			// The same bump reshapes recovery on this truncated source. The
			// recorded divergence stays at /parser_output/ERROR[0], but its
			// class moved from a child-count difference (10 against 12) to
			// an extra-node difference. The path and the routes do not move.
			name:      "malformed-class-literal-close",
			source:    []byte("public class C { void m() { Object t = RecordPage.class"),
			wantError: true,
			expected: apexExpectedRoutes(apexExpectedDivergence{
				path:     "/parser_output/ERROR[0]",
				category: "extra",
				goValue:  "false",
				cValue:   "true",
			}, "raw", "production", "compact", "incremental"),
		},
	}

	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			target := append([]byte(nil), witness.source...)
			if len(target) == 0 || target[len(target)-1] != '\n' {
				target = append(target, '\n')
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(target)); got == "" {
				t.Fatal("source SHA-256 is empty")
			}
			cTree := apexLockedCTree(t, cLanguage, target)
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatalf("inspect locked C tree: %v", err)
			}
			if got := cTree.RootNode().HasError(); got != witness.wantError {
				t.Fatalf("locked C root error=%t, want %t", got, witness.wantError)
			}

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(target)
			if err != nil {
				t.Fatalf("raw parse: %v", err)
			}
			t.Cleanup(raw.Release)
			assertApexBlockerRouteExact(t, "raw", raw, language, cTree, cDigest, witness.wantError, apexExpectedRoute(witness.expected, "raw"))
			if got := apexDispatchRewriteCount(raw); got != 0 {
				t.Fatalf("raw dispatch.apex rewrites=%d, want 0", got)
			}

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(target)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			t.Cleanup(production.Release)
			assertApexBlockerRouteExact(t, "production", production, language, cTree, cDigest, witness.wantError, apexExpectedRoute(witness.expected, "production"))
			if got := apexDispatchRewriteCount(production); got != 0 {
				t.Fatalf("production dispatch.apex rewrites=%d, want 0", got)
			}

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(target)
			if err != nil {
				t.Fatalf("compact parse: %v", err)
			}
			t.Cleanup(compact.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			compactRoute := "accepted"
			if routedAfter == routedBefore && fallbackAfter == fallbackBefore+1 {
				compactRoute = "fallback:" + gotreesitter.AdmissionCandidateLastFallbackReason()
			} else if routedAfter != routedBefore+1 || fallbackAfter != fallbackBefore {
				t.Fatalf("compact counters before=(%d,%d) after=(%d,%d)", routedBefore, fallbackBefore, routedAfter, fallbackAfter)
			}
			assertApexBlockerRouteExact(t, "compact", compact, language, cTree, cDigest, witness.wantError, apexExpectedRoute(witness.expected, "compact"))
			if got := apexDispatchRewriteCount(compact); got != 0 {
				t.Fatalf("compact dispatch.apex rewrites=%d, want 0", got)
			}

			forestParser := gotreesitter.NewParser(language)
			forest, forestOK := forestParser.ParseForestExperimental(target)
			forestRoute := "declined"
			forestRewrite := uint64(0)
			if forestOK && forest != nil {
				t.Cleanup(forest.Release)
				forestRoute = "accepted"
				assertApexBlockerRouteExact(t, "forest", forest, language, cTree, cDigest, witness.wantError, apexExpectedRoute(witness.expected, "forest"))
				forestRewrite = apexDispatchRewriteCount(forest)
				if forestRewrite != witness.wantForestRewrites {
					t.Fatalf("forest dispatch.apex rewrites=%d, want %d", forestRewrite, witness.wantForestRewrites)
				}
			} else if witness.wantForestRewrite {
				t.Fatalf("forest route declined for the live positive control")
			}

			incrementalParser := gotreesitter.NewParser(language)
			incrementalParser.SetAdmissionCandidateRoute(false)
			base := bytes.TrimSuffix(target, []byte{'\n'})
			oldTree, err := incrementalParser.Parse(base)
			if err != nil {
				t.Fatalf("incremental base parse: %v", err)
			}
			t.Cleanup(oldTree.Release)
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(base)),
				OldEndByte:  uint32(len(base)),
				NewEndByte:  uint32(len(target)),
				StartPoint:  apexPointAtByte(base, len(base)),
				OldEndPoint: apexPointAtByte(base, len(base)),
				NewEndPoint: apexPointAtByte(target, len(target)),
			})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(target, oldTree)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			t.Cleanup(incremental.Release)
			assertApexBlockerRouteExact(t, "incremental", incremental, language, cTree, cDigest, witness.wantError, apexExpectedRoute(witness.expected, "incremental"))
			if got := apexDispatchRewriteCount(incremental); got != 0 {
				t.Fatalf("incremental dispatch.apex rewrites=%d, want 0", got)
			}

			t.Logf("witness=%s bytes=%d source_sha256=%x c_digest=%s compact=%s forest=%s forest_rewrites=%d incremental_reuse=%t reuse_unsupported=%t reuse_reason=%s reused_subtrees=%d reused_bytes=%d raw_rewrites=%d production_rewrites=%d compact_rewrites=%d incremental_rewrites=%d error_root=%t", witness.name, len(target), sha256.Sum256(target), cDigest, compactRoute, forestRoute, forestRewrite, profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason, profile.ReusedSubtrees, profile.ReusedBytes, raw.ParseRuntime().NormalizationNodesRewritten, production.ParseRuntime().NormalizationNodesRewritten, compact.ParseRuntime().NormalizationNodesRewritten, incremental.ParseRuntime().NormalizationNodesRewritten, witness.wantError)
		})
	}
}

// TestApexDispatchBlockerReceiptDocument guards the A0 and route scope markers.
func TestApexDispatchBlockerReceiptDocument(t *testing.T) {
	raw, err := os.ReadFile("../docs/root-normalization-retirement.md")
	if err != nil {
		t.Fatalf("read Apex blocker receipt: %v", err)
	}
	document := strings.Join(strings.Fields(string(raw)), " ")
	for _, marker := range []string{
		"The A0 manifest has 14 languages and 14 receipts. It includes Apex with three files, three checked, three run, and zero rewrites. Only the seven-fixture tracked census excludes Apex.",
		"The raw, production, compact, and incremental routes report zero `dispatch.apex` rewrites for every clean witness.",
		"`registered-class-literal-alias`: `dispatch.apex` rewrites 3 nodes.",
	} {
		marker = strings.Join(strings.Fields(marker), " ")
		if !strings.Contains(document, marker) {
			t.Fatalf("Apex blocker receipt lacks marker %q", marker)
		}
	}
	for _, marker := range []string{
		"The A0 manifest has 14 languages and 14 receipts. It excludes Apex.",
		"Each clean route reports zero `dispatch.apex` rewrites.",
	} {
		if strings.Contains(document, marker) {
			t.Fatalf("Apex blocker receipt retains stale marker %q", marker)
		}
	}
}

func apexLockedCTree(t *testing.T, language *sitter.Language, source []byte) *sitter.Tree {
	t.Helper()
	parser := sitter.NewParser()
	t.Cleanup(parser.Close)
	if err := parser.SetLanguage(language); err != nil {
		t.Fatal(err)
	}
	tree := parser.Parse(source, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("locked C parse returned no root")
	}
	return tree
}

func assertApexBlockerRouteExact(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, cDigest string, wantError bool, expected *apexExpectedDivergence) {
	t.Helper()
	root := tree.RootNode()
	if root == nil || root.HasError() != wantError {
		t.Fatalf("%s root error=%t, want %t", route, root != nil && root.HasError(), wantError)
	}
	if diff := FirstDivergenceDumpV1(root, language, cTree.RootNode()); diff != nil {
		if expected == nil {
			t.Fatalf("%s differs from locked C: %+v", route, diff)
		}
		if diff.Path != expected.path || diff.Category != expected.category || diff.GoValue != expected.goValue || diff.CValue != expected.cValue {
			t.Fatalf("%s divergence changed: got=%+v want=%+v", route, diff, expected)
		}
		inspection, err := benchfixtures.InspectGoTree(root, language)
		if err != nil {
			t.Fatalf("%s inspect divergent Go tree: %v", route, err)
		}
		t.Logf("%s locked-C divergence=%+v go_digest=%s c_digest=%s", route, diff, inspection.SHA256, cDigest)
		return
	}
	if expected != nil {
		t.Fatalf("%s no longer reports the recorded locked-C divergence: want=%+v", route, expected)
	}
	inspection, err := benchfixtures.InspectGoTree(root, language)
	if err != nil {
		t.Fatalf("%s inspect Go tree: %v", route, err)
	}
	if inspection.SHA256 != cDigest {
		t.Fatalf("%s deep digest=%s, locked C=%s", route, inspection.SHA256, cDigest)
	}
}

func apexExpectedRoutes(divergence apexExpectedDivergence, routes ...string) map[string]apexExpectedDivergence {
	expected := make(map[string]apexExpectedDivergence, len(routes))
	for _, route := range routes {
		expected[route] = divergence
	}
	return expected
}

func apexExpectedRoute(expected map[string]apexExpectedDivergence, route string) *apexExpectedDivergence {
	divergence, ok := expected[route]
	if !ok {
		return nil
	}
	return &divergence
}

func apexDispatchRewriteCount(tree *gotreesitter.Tree) uint64 {
	if tree == nil || tree.ParseRuntime().NormalizationPasses == nil {
		return 0
	}
	for _, pass := range *tree.ParseRuntime().NormalizationPasses {
		if pass.Name == "dispatch.apex.class-literal-alias" {
			return pass.NodesRewritten
		}
	}
	return 0
}

func apexPointAtByte(source []byte, offset int) gotreesitter.Point {
	var point gotreesitter.Point
	for index, value := range source {
		if index >= offset {
			break
		}
		if value == '\n' {
			point.Row++
			point.Column = 0
		} else {
			point.Column++
		}
	}
	return point
}
