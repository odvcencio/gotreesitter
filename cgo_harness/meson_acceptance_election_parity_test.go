//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestMesonAcceptanceElectionLockedCParity(t *testing.T) {
	language := grammars.MesonLanguage()
	if language == nil {
		t.Fatal("Meson Go language is nil")
	}
	if !language.CompactAcceptanceStructuralElectionCertified {
		t.Fatal("Meson structural acceptance election is not certified")
	}

	cLanguage, err := COracleLanguage("meson")
	if err != nil {
		t.Fatalf("load locked Meson language: %v", err)
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatalf("set locked Meson language: %v", err)
	}

	// These sources cover command, assignment, nesting, collection, control,
	// project, and ternary families from the exact grammar's corpus. Every
	// compact-routed result must match locked C. Fallback results keep production.
	tests := []struct {
		name                string
		source              string
		wantRouted          bool
		wantProductionExact bool
		wantSExpr           string
	}{
		{
			name: "command", source: "message('hello')\n", wantRouted: true, wantProductionExact: true,
			wantSExpr: "(source_file (normal_command (identifier) (variableunit (string))))",
		},
		{name: "string assignment", source: "a = 'ssss'\n", wantProductionExact: true},
		{name: "nested command", source: "libpam = cc.find_library('pam', required: get_option('pam'))\n"},
		{name: "list", source: "command = [sh, '-c', output]\n"},
		{name: "dictionary", source: "prefix = {a: true, 'b': abcd}\n"},
		{name: "if condition", source: "if 1 in my_array\n# true\nendif\n", wantRouted: true, wantProductionExact: true},
		{name: "project", source: "project('wayvnc', 'c', version: '0.5.0')\n"},
		{name: "ternary", source: "x = condition ? true_value : false_value\n", wantRouted: true, wantProductionExact: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked Meson parser returned no tree")
			}
			defer cTree.Close()

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			productionTree, err := productionParser.Parse(source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			defer productionTree.Release()
			if test.wantProductionExact {
				t.Run("production", func(t *testing.T) {
					assertLockedCTreeExact(t, "Meson production", productionTree, language, cTree)
				})
			}

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			candidateParser := gotreesitter.NewParser(language)
			candidateParser.SetAdmissionCandidateRoute(true)
			candidateTree, err := candidateParser.Parse(source)
			if err != nil {
				t.Fatalf("compact candidate parse: %v", err)
			}
			defer candidateTree.Release()
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			routed, fallback := routedAfter-routedBefore, fallbackAfter-fallbackBefore
			if test.wantRouted && (routed != 1 || fallback != 0) {
				t.Fatalf("compact route delta=%d/%d reason=%q, want 1/0", routed, fallback, gotreesitter.AdmissionCandidateLastFallbackReason())
			}
			if routed+fallback != 1 || routed > 1 || fallback > 1 {
				t.Fatalf("compact route delta=%d/%d, want one routed or fallback parse", routed, fallback)
			}
			if routed == 1 {
				assertLockedCTreeExact(t, "Meson compact candidate", candidateTree, language, cTree)
			}

			if test.wantSExpr != "" {
				root := candidateTree.RootNode()
				if root == nil || root.ChildCount() == 0 {
					t.Fatal("Meson compact result has no command")
				}
				if got := root.SExpr(language); got != test.wantSExpr {
					t.Fatalf("Meson compact result=%s, want %s", got, test.wantSExpr)
				}
			}
		})
	}
}
