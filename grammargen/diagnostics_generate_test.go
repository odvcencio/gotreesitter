package grammargen

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestGenerateWithReportCtxSkipsDiagnosticsWhenNotRequested(t *testing.T) {
	report, err := generateWithReportCtx(context.Background(), CalcGrammar(), reportBuildOptions{
		includeLanguage: true,
	})
	if err != nil {
		t.Fatalf("generateWithReportCtx: %v", err)
	}
	if report.Language == nil {
		t.Fatal("report.Language is nil")
	}
	if len(report.Conflicts) != 0 {
		t.Fatalf("report.Conflicts = %d, want 0", len(report.Conflicts))
	}
	if len(report.SplitCandidates) != 0 {
		t.Fatalf("report.SplitCandidates = %d, want 0", len(report.SplitCandidates))
	}
	if report.SplitResult != nil {
		t.Fatalf("report.SplitResult = %#v, want nil", report.SplitResult)
	}
	if len(report.Blob) != 0 {
		t.Fatalf("report.Blob len = %d, want 0", len(report.Blob))
	}
}

func TestComputeLexModesWithContextHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, _, err := computeLexModesWithContext(
		ctx,
		1,
		2,
		func(state, sym int) bool { return false },
		nil,
		nil,
		-1,
		nil,
		nil,
		0,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("computeLexModesWithContext error = %v, want context.Canceled", err)
	}
}

// TestGenerateWithReportContextLanguageMatchesGenerateLanguage proves that
// GenerateWithReportContext's single LR pass produces exactly the same
// compiled Language as a standalone GenerateLanguage call -- a caller that
// needs both the diagnostic report and the compiled Language can read
// report.Language instead of calling GenerateLanguage a second time.
func TestGenerateWithReportContextLanguageMatchesGenerateLanguage(t *testing.T) {
	grammars := []struct {
		name string
		make func() *Grammar
	}{
		{"calc", CalcGrammar},
		{"json", JSONGrammar},
		{"lox", LoxGrammar},
	}

	for _, tc := range grammars {
		t.Run(tc.name, func(t *testing.T) {
			report, err := GenerateWithReportContext(context.Background(), tc.make())
			if err != nil {
				t.Fatalf("GenerateWithReportContext: %v", err)
			}
			if report.Language == nil {
				t.Fatal("report.Language is nil")
			}

			wantLang, err := GenerateLanguage(tc.make())
			if err != nil {
				t.Fatalf("GenerateLanguage: %v", err)
			}

			gotBlob, err := encodeLanguageBlob(report.Language)
			if err != nil {
				t.Fatalf("encode report.Language: %v", err)
			}
			wantBlob, err := encodeLanguageBlob(wantLang)
			if err != nil {
				t.Fatalf("encode GenerateLanguage(g) output: %v", err)
			}
			if !bytes.Equal(gotBlob, wantBlob) {
				t.Fatalf("report.Language from GenerateWithReportContext differs from GenerateLanguage(g) output (encoded lengths %d vs %d)",
					len(gotBlob), len(wantBlob))
			}
		})
	}
}

// TestGenerateWithReportContextSinglePassIsFasterThanDoubleGen measures that
// a single GenerateWithReportContext call is meaningfully cheaper than the
// naive pattern of calling GenerateLanguage and GenerateWithReport
// separately when a caller wants both the compiled Language and the
// diagnostic report -- the naive pattern runs two full LR generation
// passes (state construction, conflict resolution, lex DFA construction),
// while GenerateWithReportContext does the work once.
func TestGenerateWithReportContextSinglePassIsFasterThanDoubleGen(t *testing.T) {
	if testing.Short() {
		t.Skip("timing-sensitive; skipped in -short mode")
	}

	naiveStart := time.Now()
	if _, err := GenerateLanguage(LoxGrammar()); err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	if _, err := GenerateWithReport(LoxGrammar()); err != nil {
		t.Fatalf("GenerateWithReport: %v", err)
	}
	naiveElapsed := time.Since(naiveStart)

	singleStart := time.Now()
	report, err := GenerateWithReportContext(context.Background(), LoxGrammar())
	if err != nil {
		t.Fatalf("GenerateWithReportContext: %v", err)
	}
	if report.Language == nil {
		t.Fatal("report.Language is nil")
	}
	singleElapsed := time.Since(singleStart)

	t.Logf("naive double-gen (GenerateLanguage + GenerateWithReport): %v", naiveElapsed)
	t.Logf("single-pass GenerateWithReportContext:                   %v", singleElapsed)

	// Single-pass generation should land near half the naive double-gen
	// time. Allow generous slack (75% of naive) for machine noise while
	// still catching a regression back to double generation, which would
	// land at ~100% of naive.
	if singleElapsed > naiveElapsed*3/4 {
		t.Fatalf("single-pass GenerateWithReportContext (%v) is not meaningfully faster than the naive double-gen pattern (%v); expected roughly half",
			singleElapsed, naiveElapsed)
	}
}

// TestGenerateWithReportContextAbortsPromptlyOnTinyDeadline proves that a
// context with a near-immediate deadline aborts LR generation for a heavy
// grammar well short of the several seconds a full, uncancelled generation
// takes.
func TestGenerateWithReportContextAbortsPromptlyOnTinyDeadline(t *testing.T) {
	if testing.Short() {
		t.Skip("timing-sensitive; skipped in -short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := GenerateWithReportContext(ctx, TypeScriptGrammar())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error from a context with a near-immediate deadline, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want an error wrapping context.DeadlineExceeded", err)
	}

	const maxAbortTime = 5 * time.Second
	if elapsed > maxAbortTime {
		t.Fatalf("cancellation did not abort promptly: took %v (want < %v); an uncancelled TypeScriptGrammar generation takes several seconds longer than this",
			elapsed, maxAbortTime)
	}
}

// TestGenerateWithReportContextCancelMidGenerationAbortsPromptly proves that
// cancelling the context WHILE a heavy grammar's LR generation is actively
// running (not merely pre-expired before the call starts) aborts the
// in-flight pass promptly, rather than running generation to completion
// regardless of cancellation.
func TestGenerateWithReportContextCancelMidGenerationAbortsPromptly(t *testing.T) {
	if testing.Short() {
		t.Skip("timing-sensitive; skipped in -short mode")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := GenerateWithReportContext(ctx, TypeScriptGrammar())
		done <- err
	}()

	// Let generation get underway (a full TypeScriptGrammar pass takes
	// several seconds) before cancelling mid-flight.
	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		elapsed := time.Since(start)
		if err == nil {
			t.Fatal("expected an error after mid-generation cancellation, got nil")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want an error wrapping context.Canceled", err)
		}
		const maxAbortTime = 5 * time.Second
		if elapsed > maxAbortTime {
			t.Fatalf("cancellation did not abort promptly: took %v total (cancelled at ~150ms), want < %v", elapsed, maxAbortTime)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("generation did not return within 15s of mid-generation cancellation")
	}
}
