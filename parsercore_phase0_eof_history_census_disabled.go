//go:build !gts_no_parsercorephase0 && !gts_eof_history_census

package gotreesitter

// The shipped build omits the G2 EOF-history census. The inliner removes this
// empty call from the compact scheduler.
func (s *diagnosticParserCoreGenericScheduler) censusEOFAcceptHistoryFrontier(_ int, _ []int) {}

// EOFAcceptHistoryCensusBuilt reports whether this binary includes the G2
// diagnostic census.
//
// Deprecated: this name belongs to the compact scheduler and will be removed together with it. It has no replacement.
func EOFAcceptHistoryCensusBuilt() bool { return false }
