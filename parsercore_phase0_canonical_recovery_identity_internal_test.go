//go:build gts_parsercorephase0

package gotreesitter

import (
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestDiagnosticParserCoreCanonicalizePreservesRecoveryPhysicalIdentity(t *testing.T) {
	for _, kind := range []string{"costed", "marked", "region"} {
		for _, count := range []int{1, 2} {
			name := "sole"
			if count == 2 {
				name = "multiple"
			}
			t.Run(kind+"/"+name, func(t *testing.T) {
				compact, heads := canonicalRecoveryIdentityFixture(t)
				headers := make([]diagnosticParserCoreHeader, count)
				for i := range headers {
					headers[i] = diagnosticParserCoreHeader{head: heads[i], shifted: true, creationSeq: uint64(i + 1)}
					switch kind {
					case "costed":
						headers[i].markRecoveryCosted()
					case "marked":
						headers[i].markRecoveryLineage()
					case "region":
						headers[i].setRecoveryRegion(&diagnosticParserCoreS3Region{})
					}
				}
				// Recovery headers remain live, but ordinary condensation excludes them.
				scheduler := diagnosticParserCoreGenericScheduler{compact: compact, headers: headers}
				if candidates := scheduler.collectCondenseCandidates(0); len(candidates) != 0 {
					t.Fatalf("recovery condense candidates=%+v", candidates)
				}
				var scratch diagnosticParserCoreCanonicalScratch
				got, err := scratch.canonicalize(compact, headers)
				if err != nil {
					t.Fatal(err)
				}
				if len(got) != count {
					t.Fatalf("canonical headers=%+v, want %d physical versions", got, count)
				}
				for i := range got {
					if got[i].head != heads[i] || got[i].creationSeq != headers[i].creationSeq || got[i].versionState != headers[i].versionState || got[i].recoveryFlags != headers[i].recoveryFlags {
						t.Fatalf("header %d lost physical identity or recovery ownership: got=%+v want=%+v", i, got[i], headers[i])
					}
				}
			})
		}
	}
}

func TestDiagnosticParserCoreCanonicalizeStillRemapsCleanPhysicalIdentity(t *testing.T) {
	for _, count := range []int{1, 2} {
		compact, heads := canonicalRecoveryIdentityFixture(t)
		headers := make([]diagnosticParserCoreHeader, count)
		for i := range headers {
			headers[i] = diagnosticParserCoreHeader{head: heads[i], shifted: true, creationSeq: uint64(i + 1)}
		}
		var scratch diagnosticParserCoreCanonicalScratch
		got, err := scratch.canonicalize(compact, headers)
		if err != nil || len(got) != 1 || got[0].head != heads[2] {
			t.Fatalf("clean count=%d canonical headers=%+v err=%v, want newest head=%+v", count, got, err, heads[2])
		}
	}
}

// Each frontier publishes a distinct payload at the same boundary.
// The scheduler retains older physical heads independently of the boundary index.
func canonicalRecoveryIdentityFixture(t *testing.T) (*core.Core, []core.Head) {
	t.Helper()
	table := &genericConflictTable{cells: map[genericConflictCell][]core.Action{
		{state: 1, symbol: 3}: {{Type: core.ActionShift, State: 2}},
		{state: 1, symbol: 4}: {{Type: core.ActionShift, State: 2}},
		{state: 1, symbol: 5}: {{Type: core.ActionShift, State: 2}},
	}}
	compact, err := core.New(table, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	var heads []core.Head
	for _, symbol := range []core.Symbol{3, 4, 5} {
		if err := compact.BeginFrontier(); err != nil {
			t.Fatal(err)
		}
		seed, err := compact.Seed(1, 0)
		if err != nil {
			t.Fatal(err)
		}
		head, err := compact.Shift(seed, symbol, 0, core.Token{Symbol: symbol, EndByte: 1}, core.ForkOrder{})
		if err != nil {
			t.Fatal(err)
		}
		heads = append(heads, head)
	}
	canonical, ok := compact.CanonicalBoundary(2, 1, true, 0)
	if !ok || canonical != heads[2] || heads[0] == heads[1] || heads[1] == heads[2] {
		t.Fatalf("invalid physical-head fixture: heads=%+v canonical=%+v/%t", heads, canonical, ok)
	}
	return compact, heads
}
