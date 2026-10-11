//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"sync"
	"testing"
)

// Tests inspecting an accepted graph need explicit ownership between parses.
func pinAdmissionCandidateRunnerForTest(t testing.TB, p *Parser) {
	t.Helper()
	if _, err := p.acquireAdmissionCandidateRunner(); err != nil {
		t.Fatal(err)
	}
}

// TestParserPoolKeepsWarmCompactRunner checks that a pooled parser keeps its
// compact runner across checkouts. A cold runner allocates its arenas again
// on the next parse.
func TestParserPoolKeepsWarmCompactRunner(t *testing.T) {
	lang := buildArithmeticLanguage()
	pool := NewParserPool(lang)
	p := NewParser(lang)
	pinAdmissionCandidateRunnerForTest(t, p)
	tree, err := p.Parse([]byte("1+2+3"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tree.Release()
	if p.admissionCandidateRunner == nil {
		t.Skip("the compact route did not run for this grammar")
	}
	runner := p.admissionCandidateRunner
	pool.applyDefaults(p)
	if p.admissionCandidateRunner != runner {
		t.Fatal("applyDefaults dropped the warm compact runner")
	}
	p.SetParseWorkLimits(ParseWorkLimits{NodeLimit: 10})
	if p.admissionCandidateRunner != nil {
		t.Fatal("changed work limits must drop the runner")
	}
}

func TestCompactRuntimePoolPreservesLiveTrees(t *testing.T) {
	lang := buildArithmeticLanguage()
	p, q := NewParser(lang), NewParser(lang)
	first, ok, reason := p.tryCompactFullParseRoute([]byte("1+2+3"))
	if !ok {
		t.Fatalf("first compact parse declined: %s", reason)
	}
	defer first.Release()
	want := first.RootNode().SExpr(lang)
	if p.admissionCandidateRunner != nil {
		t.Fatal("idle Parser retained the runtime")
	}
	idle, ok := lang.compactRunnerPool.Take().(*parserCoreFreshFullRunner)
	if !ok || idle.parser != nil || idle.tables.parser != nil || idle.options.stopControlParser != nil || idle.scheduler.tokenSource != nil {
		t.Fatal("pool did not detach the caller and scanner")
	}
	lang.compactRunnerPool.Put(idle)
	second, ok, reason := q.tryCompactFullParseRoute([]byte("4+5"))
	if !ok {
		t.Fatalf("second compact parse declined: %s", reason)
	}
	defer second.Release()
	if got := first.RootNode().SExpr(lang); got != want {
		t.Fatalf("reusing the runtime changed a live tree: %s -> %s", want, got)
	}
	reused := lang.compactRunnerPool.Take()
	if reused != idle {
		t.Fatal("sequential Parsers did not share the runtime")
	}
	lang.compactRunnerPool.Put(reused)
}

func TestCompactRuntimePoolExclusiveAndBounded(t *testing.T) {
	lang := buildArithmeticLanguage()
	const workers = 8
	ready := make(chan *parserCoreFreshFullRunner, workers)
	resume := make(chan struct{})
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			p := NewParser(lang)
			runner, borrowed, err := p.borrowAdmissionCandidateRunner()
			if err != nil {
				t.Error(err)
				ready <- nil
				return
			}
			ready <- runner
			<-resume
			tree, ok, reason := p.tryCompactFullParseRoute([]byte("1+2+3"))
			if !ok {
				t.Errorf("concurrent compact parse declined: %s", reason)
			} else {
				if tree.RootNode().HasError() || tree.RootNode().EndByte() != 5 {
					t.Error("concurrent parse returned an incomplete tree")
				}
				tree.Release()
			}
			p.returnAdmissionCandidateRunner(runner, borrowed)
		}()
	}
	seen := make(map[*parserCoreFreshFullRunner]bool)
	for range workers {
		runner := <-ready
		if runner == nil || seen[runner] {
			t.Error("concurrent requests shared a runtime")
		}
		seen[runner] = true
	}
	close(resume)
	wait.Wait()
	if lang.compactRunnerPool.Take() == nil || lang.compactRunnerPool.Take() != nil {
		t.Fatal("the pool must retain exactly one idle runtime after a burst")
	}
}

func TestCompactRuntimePoolRebindsCallerAndRejectsForeignRuntime(t *testing.T) {
	lang := buildArithmeticLanguage()
	p, q := NewParser(lang), NewParser(lang)
	first, borrowed, err := p.borrowAdmissionCandidateRunner()
	if err != nil {
		t.Fatal(err)
	}
	p.returnAdmissionCandidateRunner(first, borrowed)
	// Exercise the foreign-runtime guard without copying Language's locks.
	clone := buildArithmeticLanguage()
	clone.compactRunnerPool.Put(first)
	other := NewParser(clone)
	cloned, clonedBorrowed, err := other.borrowAdmissionCandidateRunner()
	if err != nil {
		t.Fatal(err)
	}
	if cloned == first || first.parser != nil {
		t.Fatal("a foreign Language stole the original pool's runtime")
	}
	other.returnAdmissionCandidateRunner(cloned, clonedBorrowed)
	second, secondBorrowed, err := q.borrowAdmissionCandidateRunner()
	if err != nil {
		t.Fatal(err)
	}
	defer q.returnAdmissionCandidateRunner(second, secondBorrowed)
	if second != first || second.parser != q || second.tables.parser != q || second.options.stopControlParser != q {
		t.Fatal("the pooled runtime retained the previous caller's controls")
	}
}

func TestCompactRuntimePoolDiscardsPanickedLease(t *testing.T) {
	lang := buildArithmeticLanguage()
	p := NewParser(lang)
	const failure = "lease interrupted"
	func() {
		defer func() {
			if got := recover(); got != failure {
				t.Errorf("panic=%v, want original failure", got)
			}
		}()
		runner, borrowed, err := p.borrowAdmissionCandidateRunner()
		if err != nil {
			t.Fatal(err)
		}
		defer p.returnAdmissionCandidateRunner(runner, borrowed)
		panic(failure)
	}()
	if p.admissionCandidateRunner != nil || lang.compactRunnerPool.Take() != nil {
		t.Fatal("panicked request retained or pooled its runtime")
	}
}
