//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestCompactJavaScriptRetiresTrailingRecoveryLineage(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	lang := grammars.JavascriptLanguage()
	for name, test := range map[string]struct {
		source        []byte
		resumeState   gts.StateID
		resumeSymbol  gts.Symbol
		noActionDrops uint64
	}{
		"js_log_5": {
			source:      []byte("const f = (a) =. + + 1;\nclass A { m() { return 1 } }\n\n"),
			resumeState: 231, resumeSymbol: 85, noActionDrops: 1,
		},
		"js_log_8": {
			source:      []byte("function f() { return 1; ^}\ncon\nconst x = () => x + 1;\n"),
			resumeState: 22, resumeSymbol: 9,
		},
	} {
		t.Run(name, func(t *testing.T) {
			receipt, provenance, err := gts.RunStateDependentRecoveryRelexProvenanceForTest(lang, test.source)
			if err != nil {
				t.Fatal(err)
			}
			if receipt.Acceptance == nil {
				t.Fatalf("stop=%+v, want acceptance", receipt.Stop)
			}
			if got := receipt.Acceptance.Work.RecoveryLineageRetirements; got != 1 {
				t.Fatalf("recovery lineage retirements=%d, want 1", got)
			}
			if provenance.ResumeState != test.resumeState || provenance.ResumeSymbol != test.resumeSymbol ||
				provenance.ResumeCount != 1 || provenance.MissingInsertions != 1 ||
				provenance.LineageSelections != 0 || provenance.LineageRetirements != 1 ||
				provenance.NoActionDrops != test.noActionDrops {
				t.Fatalf("provenance=%+v, want resume=%d/%d count=1 missing=1 selection=0 retirement=1 drops=%d",
					provenance, test.resumeState, test.resumeSymbol, test.noActionDrops)
			}
		})
	}
}

func TestCompactJavaScriptTrailingRecoveryRetirementRequiresArtifact(t *testing.T) {
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })
	lang := *grammars.JavascriptLanguage()
	lang.CompactRecoveryTrailingLineageRetirementCertified = false
	source := []byte("function f() { return 1; ^}\ncon\nconst x = () => x + 1;\n")
	receipt, _, err := gts.RunStateDependentRecoveryRelexProvenanceForTest(&lang, source)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Acceptance != nil || receipt.Stop.Detail != gts.DiagnosticParserCoreNoTableActionDetailForTest() {
		t.Fatalf("acceptance=%+v stop=%+v, want the prior fail-closed boundary", receipt.Acceptance, receipt.Stop)
	}
}
