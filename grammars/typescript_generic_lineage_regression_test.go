package grammars

import (
	"os"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestTypeScriptGenericLineageRegressions(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	stress, err := os.ReadFile("testdata/typescript_issue_544.ts")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		source     []byte
		wantShapes []string
	}{
		{
			name:   "issue_541_nested_union_call",
			source: []byte("f<Record<string, never> | Array<Record<string, never>>>();"),
			wantShapes: []string{
				"call_expression",
				"type_arguments",
				"union_type",
			},
		},
		{
			name:   "issue_544_zod_union",
			source: stress,
			wantShapes: []string{
				"export_statement",
				"class_declaration",
				"class_heritage",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lang := TypescriptLanguage()
			production := gotreesitter.NewParser(lang)
			production.SetAdmissionCandidateRoute(false)
			productionTree, err := production.Parse(test.source)
			if err != nil {
				t.Fatal(err)
			}
			defer productionTree.Release()
			assertTypeScriptGenericLineageTree(t, "production", productionTree, lang, test.source, test.wantShapes)

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compact := gotreesitter.NewParser(lang)
			compact.SetAdmissionCandidateRoute(true)
			compactTree, err := compact.Parse(test.source)
			if err != nil {
				t.Fatal(err)
			}
			defer compactTree.Release()
			assertTypeScriptGenericLineageTree(t, "compact fallback", compactTree, lang, test.source, test.wantShapes)

			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			if routedAfter != routedBefore || fallbackAfter != fallbackBefore+1 {
				t.Fatalf(
					"compact counters routed=%d/%d fallback=%d/%d, want one safe fallback",
					routedBefore,
					routedAfter,
					fallbackBefore,
					fallbackAfter,
				)
			}
			if got, want := compactTree.RootNode().SExpr(lang), productionTree.RootNode().SExpr(lang); got != want {
				t.Fatalf("compact fallback tree differs from production\ncompact: %s\nproduction: %s", got, want)
			}
		})
	}
}

func assertTypeScriptGenericLineageTree(
	t *testing.T,
	route string,
	tree *gotreesitter.Tree,
	lang *gotreesitter.Language,
	source []byte,
	wantShapes []string,
) {
	t.Helper()
	root := tree.RootNode()
	if root == nil {
		t.Fatalf("%s returned a nil root", route)
	}
	if root.HasError() {
		t.Fatalf("%s returned an error tree: %s", route, root.SExpr(lang))
	}
	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("%s root end byte = %d, want %d", route, got, want)
	}
	if got := tree.ParseRuntime().NormalizationNanos; got != 0 {
		t.Fatalf("%s normalization time = %d, want native parse result", route, got)
	}
	sexpr := root.SExpr(lang)
	for _, shape := range wantShapes {
		if !strings.Contains(sexpr, "("+shape) {
			t.Fatalf("%s tree lacks %s: %s", route, shape, sexpr)
		}
	}
}
