//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

const compactJavaScriptErrorModeKeywordSource = "const f = (a) => a + 1;&\nclass A { m() { return 1 } }\n\n"

func TestCompactJavaScriptErrorModeKeywordCaptureSelectsAbsorbLineage(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	lang := grammars.JavascriptLanguage()
	if !lang.CompactRecoveryErrorModeKeywordCaptureCertified {
		t.Fatal("the JavaScript artifact lacks error-mode keyword certification")
	}
	source := []byte(compactJavaScriptErrorModeKeywordSource)

	token, ok := gts.ErrorModeRelexForTest(lang, source, 24)
	if !ok || token.Symbol != 54 || token.StartByte != 25 || token.EndByte != 30 {
		t.Fatalf("error-mode token=%+v ok=%t, want class/25..30", token, ok)
	}
	receipt, provenance, err := gts.RunStateDependentRecoveryRelexProvenanceForTest(lang, source)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Acceptance == nil || receipt.Acceptance.Accepts != 2 ||
		receipt.Acceptance.Work.RecoveryLineageSelections != 1 {
		t.Fatalf("acceptance=%+v stop=%+v, want two accepts and one selection", receipt.Acceptance, receipt.Stop)
	}
	if provenance.ResumeState != 20 || provenance.ResumeSymbol != 54 || provenance.ResumeCount != 1 ||
		provenance.MissingInsertions != 1 || provenance.LineageSelections != 1 ||
		provenance.LineageRetirements != 0 || !provenance.SelectedAbsorb || provenance.NoActionDrops != 1 {
		t.Fatalf("provenance=%+v, want resume=20/54, one insertion, and one selection", provenance)
	}

	gts.ResetAdmissionCandidateCountersForTest()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	routed, fallback := gts.AdmissionCandidateCounters()
	if routed != 1 || fallback != 0 {
		t.Fatalf("route counters=%d/%d, want 1/0; reason=%q", routed, fallback, gts.AdmissionCandidateLastFallbackReason())
	}
	want := "(program (lexical_declaration (variable_declarator (identifier) (arrow_function (formal_parameters (identifier)) (binary_expression (identifier) (number))))) (ERROR) (class_declaration (identifier) (class_body (method_definition (property_identifier) (formal_parameters) (statement_block (return_statement (number)))))))"
	if got := tree.RootNode().SExpr(lang); got != want {
		t.Fatalf("tree=%s, want %s", got, want)
	}
}

func TestCompactJavaScriptErrorModeKeywordCaptureRequiresArtifact(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	lang := *grammars.JavascriptLanguage()
	lang.CompactRecoveryErrorModeKeywordCaptureCertified = false
	source := []byte(compactJavaScriptErrorModeKeywordSource)

	token, ok := gts.ErrorModeRelexForTest(&lang, source, 24)
	if !ok || token.Symbol != lang.KeywordCaptureToken {
		t.Fatalf("uncertified error-mode token=%+v ok=%t, want the raw capture token", token, ok)
	}
	receipt, _, err := gts.RunStateDependentRecoveryRelexProvenanceForTest(&lang, source)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Acceptance != nil || !strings.Contains(receipt.Stop.Detail, "error-mode lex disagrees") {
		t.Fatalf("acceptance=%+v stop=%+v, want the prior fail-closed election", receipt.Acceptance, receipt.Stop)
	}
}

func TestCompactJavaScriptSelectedAbsorbAliasUsesProductionProof(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	lang := *grammars.JavascriptLanguage()
	lang.CompactRecoveryTerminalAliasRules = nil

	gts.ResetAdmissionCandidateCountersForTest()
	parser := gts.NewParser(&lang)
	parser.SetAdmissionCandidateRoute(true)
	tree, err := parser.Parse([]byte(compactJavaScriptErrorModeKeywordSource))
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	routed, fallback := gts.AdmissionCandidateCounters()
	if routed != 1 || fallback != 0 {
		t.Fatalf("route counters=%d/%d, want 1/0; reason=%q", routed, fallback, gts.AdmissionCandidateLastFallbackReason())
	}
}
