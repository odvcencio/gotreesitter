//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// TestCompactArenaReserveBytesStaysUnderBudget proves the record-arena reserve
// can never take a share of the caller's soft memory budget large enough to
// decline a parse that would otherwise complete. Core.FootprintBytes gauges
// capacity, so the reserve raises the scheduler's budget gauge immediately, at
// construction; this is the bound that makes that safe.
func TestCompactArenaReserveBytesStaysUnderBudget(t *testing.T) {
	limits := []int64{
		1 << 20,   // 1 MiB
		16 << 20,  // 16 MiB
		40 << 20,  // below the absolute cap, so the divisor governs
		96 << 20,  // the tranche B9 witness budget
		512 << 20, // the shipped default soft budget
		2048 << 20,
	}
	// Every armed limit constrains the reserve, and each independently: the
	// scheduler polls one FootprintBytes gauge against BOTH the soft budget
	// and the hard ceiling, and parseMemoryHardCeilingBytes stays armed even
	// when a caller disables the soft budget.
	for _, budget := range limits {
		for _, ceiling := range limits {
			reserve := compactArenaReserveBytes(budget, ceiling)
			for name, limit := range map[string]int64{"budget": budget, "ceiling": ceiling} {
				if reserve > uint64(limit)/compactArenaReserveBudgetDivisor {
					t.Fatalf("budget=%d ceiling=%d reserve=%d, want at most one %dth of the %s %d",
						budget, ceiling, reserve, compactArenaReserveBudgetDivisor, name, limit)
				}
			}
			if reserve > compactArenaReserveCapBytes {
				t.Fatalf("budget=%d ceiling=%d reserve=%d, want at most the absolute cap %d",
					budget, ceiling, reserve, uint64(compactArenaReserveCapBytes))
			}
		}
	}
	// A hard ceiling alone constrains the reserve even when no soft budget is
	// armed. This is the case a soft-budget-only ceiling used to miss: it let
	// the reserve trip the hard-ceiling poll before the seed published one
	// record, which declined a parse the compact route served before.
	for _, ceiling := range []int64{16 << 20, 32 << 20, 40 << 20, 48 << 20} {
		got := compactArenaReserveBytes(0, ceiling)
		if want := uint64(ceiling) / compactArenaReserveBudgetDivisor; got != min(want, uint64(compactArenaReserveCapBytes)) {
			t.Fatalf("ceiling-only reserve at %d = %d, want %d", ceiling, got, want)
		}
	}
	// The shipped defaults: the absolute cap binds before either limit.
	if got := compactArenaReserveBytes(512<<20, 2048<<20); got != compactArenaReserveCapBytes {
		t.Fatalf("shipped-default reserve=%d, want the absolute cap %d", got, uint64(compactArenaReserveCapBytes))
	}
	// Neither limit armed (the diagnostic and benchmark runners arm neither):
	// only the absolute cap applies.
	for _, limit := range []int64{0, -1} {
		if got := compactArenaReserveBytes(limit, limit); got != compactArenaReserveCapBytes {
			t.Fatalf("unarmed reserve at %d = %d, want the absolute cap %d", limit, got, uint64(compactArenaReserveCapBytes))
		}
	}
}

// TestCompactArenaReserveStaysBelowCoreRetentionCap proves the reserve cannot
// on its own make a core oversized by the compact core's own retention
// standard. releaseOversizedRetention compares TOTAL FootprintBytes against
// coreRetentionCapBytes, so a reserve at or above that cap would make every
// maximum-size core born oversized and would make retention non-monotonic in
// source length.
func TestCompactArenaReserveStaysBelowCoreRetentionCap(t *testing.T) {
	if compactArenaReserveCapBytes >= core.RetentionCapBytesForTest() {
		t.Fatalf("reserve cap=%d, want strictly below the core retention cap %d",
			uint64(compactArenaReserveCapBytes), core.RetentionCapBytesForTest())
	}
}

// TestCompactArenaReserveNeverExceedsCeilingAtAnySourceLength walks the source
// lengths a caller can actually present, from one byte to a hundred megabytes,
// and proves the reserve the compact core would take stays inside the ceiling
// at every one of them.
func TestCompactArenaReserveNeverExceedsCeilingAtAnySourceLength(t *testing.T) {
	lang, err := LoadLanguage(parserCoreCertifiedGoBlob)
	if err != nil {
		t.Skip(err)
	}
	tables, err := newParserCoreRootTables(NewParser(lang))
	if err != nil {
		t.Fatal(err)
	}
	compact, err := core.New(tables, admissionCandidateLimits())
	if err != nil {
		t.Fatal(err)
	}
	const defaultBudget = int64(512) << 20
	const defaultHardCeiling = int64(2048) << 20
	ceiling := compactArenaReserveBytes(defaultBudget, defaultHardCeiling)
	lengths := []int{
		1, 1 << 10, 5116, 20168, 41387, 235626, 484021,
		1 << 20, 8 << 20, 100 << 20,
	}
	for _, sourceLen := range lengths {
		reserve := compact.ReserveRecordArenaBytes(sourceLen, ceiling)
		if reserve > ceiling {
			t.Fatalf("sourceLen=%d reserve=%d, want at most the ceiling %d", sourceLen, reserve, ceiling)
		}
		if int64(reserve) >= defaultBudget {
			t.Fatalf("sourceLen=%d reserve=%d clears the default budget %d", sourceLen, reserve, defaultBudget)
		}
	}
	// The same sweep against a lowered hard ceiling with the soft budget left
	// at its default. The ceiling arms independently, so it must still bound
	// the reserve on its own.
	for _, hardCeiling := range []int64{16 << 20, 32 << 20, 40 << 20, 48 << 20} {
		lowered := compactArenaReserveBytes(defaultBudget, hardCeiling)
		for _, sourceLen := range lengths {
			if reserve := compact.ReserveRecordArenaBytes(sourceLen, lowered); int64(reserve)*compactArenaReserveBudgetDivisor > hardCeiling {
				t.Fatalf("hardCeiling=%d sourceLen=%d reserve=%d, want at most one %dth of the ceiling",
					hardCeiling, sourceLen, reserve, compactArenaReserveBudgetDivisor)
			}
		}
	}
	// The largest file in the project's own curated real corpus is 484,021
	// bytes. Its reserve must stay well inside the default budget, with room
	// for the parse the reserve exists to serve.
	if reserve := compact.ReserveRecordArenaBytes(484021, ceiling); reserve*4 > uint64(defaultBudget) {
		t.Fatalf("largest real-corpus reserve=%d, want under a quarter of the default budget %d", reserve, defaultBudget)
	}
}

// TestDFATokenSourceSourceLength proves the reserve estimator's only input
// reads the real source length and stays safe on the degenerate receivers.
func TestDFATokenSourceSourceLength(t *testing.T) {
	var nilSource *dfaTokenSource
	if got := nilSource.sourceLength(); got != 0 {
		t.Fatalf("nil token source length=%d, want 0", got)
	}
	if got := (&dfaTokenSource{}).sourceLength(); got != 0 {
		t.Fatalf("lexer-free token source length=%d, want 0", got)
	}
	lang, err := LoadLanguage(parserCoreCertifiedGoBlob)
	if err != nil {
		t.Skip(err)
	}
	source := []byte("package p\n\nfunc main() {}\n")
	parser := NewParser(lang)
	tokenSource := parser.acquireParserDFATokenSource(source)
	if tokenSource == nil {
		t.Fatal("production DFA token source unavailable")
	}
	defer tokenSource.Close()
	if got := tokenSource.sourceLength(); got != len(source) {
		t.Fatalf("token source length=%d, want %d", got, len(source))
	}
}
