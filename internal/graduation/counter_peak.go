package graduation

import "github.com/odvcencio/gotreesitter/internal/sched"

// CompleteFrontierPeak authenticates a selected peak when exactly one attempt
// did scheduler work. A zero-work verification can count as an attempt without
// creating another frontier. A retry or a second scheduler attempt requires its
// own peak receipt, even when its result was discarded.
func CompleteFrontierPeak(work sched.OperationWork, peak uint64) bool {
	if peak == 0 {
		return false
	}
	var sum sched.Work
	active := 0
	for index, phase := range []sched.Work{work.Initial, work.Compact, work.Retry, work.Fallback, work.Verification, work.Recovery, work.Forest} {
		sum.Add(phase)
		if phase.Tokens != 0 || phase.Nodes != 0 || phase.Iterations != 0 || phase.Bytes != 0 {
			if phase.Attempts != 1 {
				return false
			}
			active++
		} else if phase.Attempts != 0 && (index != int(sched.Verification) || phase.Attempts != 1) {
			return false
		}
	}
	return active == 1 && sum == work.Total
}
