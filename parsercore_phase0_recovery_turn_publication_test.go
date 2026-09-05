//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import (
	"reflect"
	"strings"
	"testing"
)

func TestAdmissionOwnedEOFRecoveryBundle(t *testing.T) {
	p := newAdmissionCandidateGoParser(t)
	for _, certified := range []bool{false, true} {
		language := *p.language
		language.CompactOwnedEOFRecoveryCertified = certified
		runner, err := newAdmissionCandidateRunner(NewParser(&language))
		if err != nil {
			t.Fatal(err)
		}
		options := runner.options
		for name, enabled := range map[string]bool{
			"recovery": options.Recovery, "strategy2": options.allowCompactStrategy2ErrorRegion,
			"missing": options.allowCompactMissingTokenInsertion, "faithful": options.allowCompactFaithfulS5Recovery,
			"lineage": options.allowCompactRecoveryLineageSelection, "turns": options.allowCompactRecoveryVersionTurns,
			"plain_first": runner.recoveryPlainFirst,
		} {
			if enabled != certified {
				t.Fatalf("bundle=%t mechanism %s=%t", certified, name, enabled)
			}
		}
		if options.allowCompactStackSummaryRecovery || options.allowCompactS5EOFMissingInsertion || options.allowCompactRecoverEOF ||
			options.allowCompactRecoveryTrailingLineageRetirement || options.allowCompactRecoveryErrorModeKeywordCapture {
			t.Fatal("the owned EOF bundle enabled an unrelated recovery route")
		}
	}
}

func TestCompactRecoveryVersionTurnRejectsPriorSharedHistory(t *testing.T) {
	for _, history := range []string{"opened", "resumed", "dropped"} {
		t.Run(history, func(t *testing.T) {
			scheduler := newRecoveryLineageForkScheduler(t, true)
			scheduler.options.allowCompactRecoveryVersionTurns = true
			switch history {
			case "opened":
				scheduler.s3RegionOpened = true
			case "resumed":
				scheduler.s3ResumeCount = 1
			case "dropped":
				scheduler.work.NoActionDrops = 1
			}
			headers := append([]diagnosticParserCoreHeader(nil), scheduler.headers...)
			work, coreWork, turns := scheduler.work, scheduler.compact.Work(), scheduler.recoveryTurns
			handled, err := scheduler.s5TryMissingTokenInsertion(0)
			if handled || err == nil || !strings.Contains(err.Error(), "no prior shared recovery or no-action drops") {
				t.Fatalf("prior history handled=%t err=%v", handled, err)
			}
			if !reflect.DeepEqual(headers, scheduler.headers) || scheduler.work != work ||
				scheduler.compact.Work() != coreWork || scheduler.recoveryTurns != turns {
				t.Fatal("prior-history decline mutated the recovery frontier")
			}
		})
	}
}

func TestCompactRecoveryVersionTurnRejectsSharedPublication(t *testing.T) {
	for _, source := range []string{
		"package/p\nfunc a() { aa := 12; _ = aa + 1 }\nfunc b() { _ = 2 }\n",
		"package p\nfunc a() { aa := 12; _ = aa + 1'}\nfunc b() { _ = 2 }\n",
		"package p\nfunc a() { aa := 12; _ = aa + 1 }\nfunc b() { _ = 2'}\n",
	} {
		p := newCompactRecoveryVersionTurnGoParser(t)
		tree, routed, reason := p.tryCompactFullParseRoute([]byte(source))
		if tree != nil {
			tree.Release()
		}
		if routed || !strings.Contains(reason, "requires an executed EOF turn") {
			t.Fatalf("shared recovery route=%t reason=%q", routed, reason)
		}
		runner := p.admissionCandidateRunner.(*parserCoreFreshFullRunner)
		scheduler := &runner.scheduler
		if !scheduler.s3RegionOpened || scheduler.recoveryTurns.active || scheduler.work.RecoverEOFAccepts != 0 {
			t.Fatal("fixture did not exercise shared recovery without owned EOF")
		}
		// A fork alone cannot certify an EOF advance.
		scheduler.recoveryTurns.active = true
		if _, err := runner.materializeSelection([]byte(source), nil, scheduler); err == nil ||
			!strings.Contains(err.Error(), "requires an executed EOF turn") {
			t.Fatalf("unexecuted fork publication err=%v", err)
		}
	}
}

func TestCompactRecoveryVersionTurnSelectedStoreScope(t *testing.T) {
	for _, recovery := range []bool{false, true} {
		p := newCompactRecoveryVersionTurnGoParser(t)
		source := []byte("package p\nfunc f(){}\nfunc g(){}\n")
		if recovery {
			source[len(source)-1] = '+'
		}
		tree, routed, reason := p.tryCompactFullParseRoute(source)
		if !routed || tree == nil {
			t.Fatalf("public route recovery=%t reason=%q", recovery, reason)
		}
		tree.Release()
		runner := p.admissionCandidateRunner.(*parserCoreFreshFullRunner)
		store, err := runner.materializeSelectedStoreSelection(source, runner.compact, &runner.scheduler, nil)
		if store != nil {
			store.Release()
		}
		if recovery {
			if store != nil || err == nil || !strings.Contains(err.Error(), "does not support owned recovery roots") {
				t.Fatalf("owned recovery selected store=%v err=%v", store != nil, err)
			}
		} else if store == nil || err != nil {
			t.Fatalf("clean selected store=%v err=%v", store != nil, err)
		}
	}
}
