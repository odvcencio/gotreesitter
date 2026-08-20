//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestDiagnosticParserCoreStateDependentRelexFailsClosedWithoutConvergedCoverage(t *testing.T) {
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

	const coverageDetail = "lacks alternative-set coverage by one non-blended survivor"
	receipt, err := gts.RunStateDependentRelexSchedulerForTest(lang, source)
	if err == nil || !strings.Contains(err.Error(), coverageDetail) {
		t.Fatalf("compact scheduler err=%v, want %q", err, coverageDetail)
	}
	if receipt.Acceptance != nil || receipt.Completion != nil {
		t.Fatalf("failed compact transaction published acceptance/completion: acceptance=%v completion=%v", receipt.Acceptance, receipt.Completion)
	}
	if receipt.ReceiptMode != gts.DiagnosticParserCoreReceiptFull || len(receipt.StartHeaders) == 0 ||
		len(receipt.Rounds) == 0 || len(receipt.Elections) == 0 {
		t.Fatalf("failed compact transaction lost diagnostic trace: mode=%v headers=%d rounds=%d elections=%d", receipt.ReceiptMode, len(receipt.StartHeaders), len(receipt.Rounds), len(receipt.Elections))
	}
	if receipt.Tokens != 0 || receipt.Dispatches != 0 || receipt.Stop.Boundary != "" {
		t.Fatalf("failed compact transaction finalized a public summary: tokens=%d dispatches=%d stop=%q", receipt.Tokens, receipt.Dispatches, receipt.Stop.Boundary)
	}

	// The legacy split-drop certificate cannot bypass the newer alternative-set
	// coverage proof; the public candidate route must preserve production via
	// fallback rather than publish an unproved tree.
	certifiedLang := *lang
	certifiedLang.CompactConvergedReductionSplitDropsCertified = true
	entry.Language = func() *gts.Language {
		return &certifiedLang
	}
	row := runAdmissionScorecardSource(entry, source)
	// The default public route intentionally collapses pre-acceptance scheduler
	// declines to this coarser reason; the raw diagnostic assertion above binds
	// the underlying coverage failure precisely.
	const publicDetail = "fresh-full runner did not accept EOF"
	if row.status != scorecardFallback || !strings.Contains(row.detail, publicDetail) {
		t.Fatalf("certified compact result = %s: %s, want fallback %q", row.status, row.detail, publicDetail)
	}
}
