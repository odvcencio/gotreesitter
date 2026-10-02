package graduation

import (
	"testing"

	"github.com/odvcencio/gotreesitter/internal/sched"
)

func TestFrontierPeakRejectsDiscardedSchedulerWork(t *testing.T) {
	primary := sched.Work{Attempts: 1, Tokens: 10, Nodes: 20, Iterations: 30, Bytes: 40}
	work := sched.OperationWork{Compact: primary, Verification: sched.Work{Attempts: 1}}
	work.Total.Add(work.Compact)
	work.Total.Add(work.Verification)
	if !CompleteFrontierPeak(work, 2) {
		t.Fatal("zero-work verification hid a complete scheduler peak")
	}
	if CompleteFrontierPeak(work, 0) {
		t.Fatal("missing peak was accepted")
	}
	work.Verification.Nodes = 1
	work.Total.Nodes++
	if CompleteFrontierPeak(work, 2) {
		t.Fatal("discarded verification tree lacked a frontier peak")
	}
	work = sched.OperationWork{Compact: primary, Total: primary}
	work.Compact.Attempts = 2
	work.Total.Attempts = 2
	if CompleteFrontierPeak(work, 2) {
		t.Fatal("two attempts shared an unauthenticated selected peak")
	}
	work = sched.OperationWork{Compact: primary, Total: primary}
	work.Total.Tokens++
	if CompleteFrontierPeak(work, 2) {
		t.Fatal("phase sum did not authenticate the operation total")
	}
}
