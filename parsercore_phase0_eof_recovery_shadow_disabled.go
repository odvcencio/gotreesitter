//go:build !gts_no_parsercorephase0 && gts_eof_history_census && !gts_eof_recovery_shadow

package gotreesitter

import core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"

func (s *diagnosticParserCoreGenericScheduler) censusEOFRecoveryShadow(
	_ int,
	_ []core.Derivation,
	_ *EOFAcceptHistoryHead,
) {
}
