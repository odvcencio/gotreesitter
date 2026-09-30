//go:build !gts_incr_census

package gotreesitter

// Compile-time guards erase every census call, including defers and arguments.
const incrCensusEnabled = false

func incrCensusEnter(string) int               { return 0 }
func incrCensusLeave(int)                      {}
func incrCensusDecision(*Node, string, string) {}
func incrCensusFallback(string)                {}
func incrCensusSwitch(string)                  {}

// This identity function inlines to its argument in production.
func incrCensusRejectIf(_ *Node, _ string, reject bool) bool { return reject }
func incrCensusSkipIf(_ *Node, _ string, skip bool) bool     { return skip }
func incrCensusBlockedAt(uint32, string)                     {}

func incrCensusDispatchReady() {}

func incrCensusDependencyResult(_ *Node, _ string, unaffected bool) bool { return unaffected }
