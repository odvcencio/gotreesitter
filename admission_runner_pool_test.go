//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"sync"
	"testing"
)

func TestAdmissionRunnerPoolOwnershipAndReset(t *testing.T) {
	lang := buildArithmeticLanguage()
	first, second := NewParser(lang), NewParser(lang)
	runner, pooled, err := first.borrowAdmissionCandidateRunner(0)
	if err != nil || !pooled {
		t.Fatalf("first borrow: pooled=%t err=%v", pooled, err)
	}
	other, otherPooled, err := second.borrowAdmissionCandidateRunner(0)
	if err != nil || !otherPooled || other == runner || other.compact == runner.compact {
		t.Fatalf("concurrent borrow shared mutable state: pooled=%t err=%v", otherPooled, err)
	}
	runner.scannerScratch = []byte("previous scanner state")
	first.returnAdmissionCandidateRunner(runner, pooled)
	if first.admissionCandidateRunner != nil || runner.parser != nil || runner.tables.parser != nil ||
		runner.options.stopControlParser != nil || runner.scheduler.options.materializationParser != nil ||
		runner.scheduler.tokenSource != nil || runner.scratch.materializer.source != nil ||
		runner.scratch.incrementalReuse != nil || runner.compact.StorageBytes() != 0 || len(runner.scannerScratch) != 0 {
		t.Fatal("returned runner retained parse state")
	}
	for _, value := range runner.scannerScratch[:cap(runner.scannerScratch)] {
		if value != 0 {
			t.Fatal("returned runner retained scanner bytes")
		}
	}
	third := NewParser(lang)
	reused, reusedPooled, err := third.borrowAdmissionCandidateRunner(0)
	if err != nil || !reusedPooled || reused != runner || reused.parser != third || reused.tables.parser != third ||
		reused.options.stopControlParser != third {
		t.Fatalf("runner was not rebound to the next parser: pooled=%t err=%v", reusedPooled, err)
	}
	third.returnAdmissionCandidateRunner(reused, reusedPooled)
	second.returnAdmissionCandidateRunner(other, otherPooled)
	// A concurrent burst retains only one idle runner.
	pool := lang.admissionRunnerPool()
	if pool.Get() == nil || pool.Get() != nil {
		t.Fatal("pool did not retain exactly one idle runner")
	}
}

func TestAdmissionRunnerPoolCopiedLanguageIsolation(t *testing.T) {
	lang := buildArithmeticLanguage()
	first := NewParser(lang)
	runner, pooled, err := first.borrowAdmissionCandidateRunner(0)
	if err != nil {
		t.Fatal(err)
	}
	first.returnAdmissionCandidateRunner(runner, pooled)
	copyLanguage := *lang
	second := NewParser(&copyLanguage)
	other, otherPooled, err := second.borrowAdmissionCandidateRunner(0)
	if err != nil || other == runner || other.lang != &copyLanguage {
		t.Fatalf("copied language reused the original language's core: %v", err)
	}
	second.returnAdmissionCandidateRunner(other, otherPooled)
}

func TestAdmissionRunnerPoolSmallerMemoryBudget(t *testing.T) {
	lang := buildArithmeticLanguage()
	first := NewParser(lang)
	runner, pooled, err := first.borrowAdmissionCandidateRunner(0)
	if err != nil {
		t.Fatal(err)
	}
	runner.compact.ReserveRecordArenas(32768, 16<<20)
	first.returnAdmissionCandidateRunner(runner, pooled)
	second := NewParser(lang)
	second.SetMemoryBudgetBytes(4 << 20)
	other, otherPooled, err := second.borrowAdmissionCandidateRunner(0)
	if err != nil || other == runner {
		t.Fatalf("small-budget parser inherited oversized arenas: %v", err)
	}
	second.returnAdmissionCandidateRunner(other, otherPooled)
	second.SetAdmissionCandidateRoute(true)
	tree, err := second.Parse([]byte("1+2+3"))
	if err != nil || tree == nil || !tree.compactMaterialized || tree.RootNode().HasError() {
		t.Fatalf("small-budget compact parse: %v; fallback=%q runtime=%s", err, AdmissionCandidateLastFallbackReason(), tree.ParseRuntime().Summary())
	}
	tree.Release()
}

func TestAdmissionRunnerPoolPreservesSuccessfulReserve(t *testing.T) {
	lang := buildArithmeticLanguage()
	first := NewParser(lang)
	first.SetMemoryBudgetBytes(1 << 30)
	runner, pooled, err := first.borrowAdmissionCandidateRunner(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	runner.compact.ReserveRecordArenas(1<<20, 1<<30)
	before := runner.compact.FootprintBytes()
	first.returnAdmissionCandidateRunner(runner, pooled)
	second := NewParser(lang)
	second.SetMemoryBudgetBytes(1 << 30)
	reused, reusedPooled, err := second.borrowAdmissionCandidateRunner(1 << 20)
	if err != nil || reused != runner || reused.compact.FootprintBytes() != before {
		t.Fatalf("successful reserve was not retained for the next large parse: %v", err)
	}
	second.returnAdmissionCandidateRunner(reused, reusedPooled)
}

func TestAdmissionRunnerPoolPreservesLiveTrees(t *testing.T) {
	lang := buildArithmeticLanguage()
	first := NewParser(lang)
	first.SetAdmissionCandidateRoute(true)
	tree, err := first.Parse([]byte("1+2+3"))
	if err != nil || tree == nil || !tree.compactMaterialized {
		t.Fatalf("initial compact parse: %v", err)
	}
	defer tree.Release()
	want := tree.RootNode().SExpr(lang)
	for _, source := range []string{"4+5", "6", "7+8+9"} {
		parser := NewParser(lang)
		parser.SetAdmissionCandidateRoute(true)
		next, err := parser.Parse([]byte(source))
		if err != nil || next == nil || !next.compactMaterialized {
			t.Fatalf("compact parse %q: %v", source, err)
		}
		next.Release()
		if parser.admissionCandidateRunner != nil || tree.RootNode().SExpr(lang) != want {
			t.Fatal("pool reuse retained a runner or changed an earlier live tree")
		}
	}
}

func TestAdmissionRunnerPoolConcurrentParsers(t *testing.T) {
	lang := buildArithmeticLanguage()
	var wg sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for attempt := 0; attempt < 10; attempt++ {
				parser := NewParser(lang)
				parser.SetAdmissionCandidateRoute(true)
				tree, err := parser.Parse([]byte("1+2+3"))
				if err != nil || tree == nil || !tree.compactMaterialized {
					t.Errorf("concurrent compact parse: %v", err)
					return
				}
				if tree.RootNode().HasError() || tree.RootNode().EndByte() != 5 || parser.admissionCandidateRunner != nil {
					t.Error("concurrent parse leaked state or returned an invalid tree")
				}
				tree.Release()
			}
		}()
	}
	wg.Wait()
}
