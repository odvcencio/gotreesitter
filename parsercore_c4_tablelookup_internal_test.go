//go:build gts_parsercorephase0 && gts_workcount

package gotreesitter

import "testing"

// TestParserCoreCorridorTableLookupProxyIdentity proves the corridor keeps the
// published raw table-lookup proxy identical, so no counter under contract
// gts-work-count/v2 drifts when the lane is on.
func TestParserCoreCorridorTableLookupProxyIdentity(t *testing.T) {
	measure := func(on bool) uint64 {
		restore := SetParserCoreCorridorEnabledForTest(on)
		defer restore()
		fixture := loadDiagnosticParserCoreCanonicalFixture(t, "query_compile")
		runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
		if err != nil {
			t.Fatal(err)
		}
		BeginDiagnosticWorkCount()
		scheduler, tokenSource, err := runner.executeSchedulerOpen(fixture.Source, runner.compact, true)
		if err != nil {
			t.Fatal(err)
		}
		defer tokenSource.Close()
		counts := EndDiagnosticWorkCount()
		t.Logf("corridor=%v table_lookups_proxy=%d corridor_passes=%d", on, counts.TableLookupsProxy, scheduler.work.CorridorPasses)
		return counts.TableLookupsProxy
	}
	off := measure(false)
	on := measure(true)
	if off != on {
		t.Fatalf("TableLookupsProxy drifted: off=%d on=%d (delta=%d)", off, on, int64(on)-int64(off))
	}
}
