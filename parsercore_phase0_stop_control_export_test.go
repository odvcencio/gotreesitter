//go:build !gts_no_parsercorephase0

package gotreesitter

// FootprintPollStrideForTest exposes footprintPollStride
// (parsercore_phase0_stop_control.go), the throttle lever 2 applies to
// pollStopControl's memory-footprint recompute, so the external test package
// can size an adversarial witness against the real stride instead of
// hardcoding a copy that could silently drift out of sync.
func FootprintPollStrideForTest() int {
	return footprintPollStride
}

// SetStopControlExactFootprintObserverForTest installs fn as
// stopControlExactFootprintObserverForTest (parsercore_phase0_stop_control.go):
// it is called with every EXACT scheduler-footprint value computed during a
// parse, whether or not that value trips the configured budget, so a
// differential test can record the peak value actually observed and assert
// it against the configured budget. Pass nil to remove it.
func SetStopControlExactFootprintObserverForTest(fn func(exact uint64)) {
	stopControlExactFootprintObserverForTest = fn
}
