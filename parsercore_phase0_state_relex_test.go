//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestDiagnosticParserCoreStateDependentRelexKeepsExactSpanBranch(t *testing.T) {
	source := []byte(`object X {
  val EOL = "\n"
  def f(t: TraversableOnce[String], c: Boolean): String = {
    val space = " "
    val sep = if (c) EOL + space * 2 else EOL
    val (a, b) = if (c) (space + "{", EOL + "}") else ("", "")
    t.mkString(a + sep, sep, b + EOL)
  }
}`)
	var entry grammars.LangEntry
	found := false
	for _, candidate := range grammars.AllLanguages() {
		if candidate.Name == "scala" {
			entry = candidate
			found = true
			break
		}
	}
	if !found {
		t.Fatal("scala is absent from the language registry")
	}
	lang := entry.Language()

	production, err := gts.NewParser(lang).Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer production.Release()
	if production.RootNode().HasError() {
		t.Fatal("production witness has an error")
	}

	receipt, err := gts.RunStateDependentRelexSchedulerForTest(lang, source)
	if err != nil {
		t.Fatalf("compact scheduler: %v", err)
	}
	if receipt.Acceptance == nil {
		t.Fatalf("compact scheduler stopped at %+v", receipt.Stop)
	}

	certifiedLang := *lang
	certifiedLang.CompactConvergedReductionSplitDropsCertified = true
	entry.Language = func() *gts.Language {
		return &certifiedLang
	}
	row := runAdmissionScorecardSource(entry, source)
	if row.status != scorecardPass {
		t.Fatalf("certified compact result = %s: %s", row.status, row.detail)
	}
}
