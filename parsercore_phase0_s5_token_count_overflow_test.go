//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"strings"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// newS5RunOwnedAdmittedScheduler builds the minimal scheduler state that
// clears every s5RunOwned guard before the token-count check. The caller
// still supplies tokenCount so both the overflow and non-overflow cases
// share one setup path.
func newS5RunOwnedAdmittedScheduler(tokenCount uint32) *diagnosticParserCoreGenericScheduler {
	return &diagnosticParserCoreGenericScheduler{
		headers:     []diagnosticParserCoreHeader{{}},
		token:       Token{Symbol: 1},
		tokenSource: &dfaTokenSource{language: &Language{TokenCount: tokenCount}},
		options: DiagnosticParserCorePrefixOptions{
			Recovery:                             true,
			allowCompactMissingTokenInsertion:    true,
			allowCompactStrategy2ErrorRegion:      true,
			allowCompactRecoveryLineageSelection: true,
		},
	}
}

// TestS5RunOwnedDeclinesTokenCountAboveUint16Range pins issue: a
// core.Symbol(uint32) conversion truncates before a > math.MaxUint16
// check can ever see the overflow. s5RunOwned must read the raw uint32
// TokenCount first, so a 70000-token language declines instead of
// silently wrapping into a valid-looking uint16 count.
func TestS5RunOwnedDeclinesTokenCountAboveUint16Range(t *testing.T) {
	scheduler := newS5RunOwnedAdmittedScheduler(70000)
	ran, err := scheduler.s5RunOwned(core.SchedulerTransactionToken{}, 0, &diagnosticParserCoreS5Work{})
	if err != nil {
		t.Fatalf("s5RunOwned returned an error: %v", err)
	}
	if ran {
		t.Fatal("s5RunOwned ran with a token count above the uint16 range")
	}
}

// TestS5RunOwnedTokenCountAtUint16LimitIsNotDeclinedByTheGuard confirms
// the guard's boundary sits at math.MaxUint16, not below it: a language
// with exactly that many tokens must clear the token-count check. The
// scheduler has no seeded compact head, so it declines just past the
// guard with a "boundary" error from compact.Boundary instead of the
// silent (false, nil) the token-count guard itself returns.
func TestS5RunOwnedTokenCountAtUint16LimitIsNotDeclinedByTheGuard(t *testing.T) {
	scheduler := newS5RunOwnedAdmittedScheduler(65535)
	_, err := scheduler.s5RunOwned(core.SchedulerTransactionToken{}, 0, &diagnosticParserCoreS5Work{})
	if err == nil || !strings.Contains(err.Error(), "invalid node id") {
		t.Fatalf("s5RunOwned did not reach past the token-count guard for 65535 tokens: err=%v", err)
	}
}
