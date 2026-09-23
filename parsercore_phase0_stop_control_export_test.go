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
