//go:build gts_parsercorephase0

package gotreesitter

import (
	"runtime"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// loadCertifiedGoLanguageForTest decodes the embedded certified Go blob into a
// full *Language without importing the grammars package (which would form an
// import cycle for this internal test package).
func loadCertifiedGoLanguageForTest(t *testing.T) *Language {
	t.Helper()
	lang, err := LoadLanguage(parserCoreCertifiedGoBlob)
	if err != nil {
		t.Fatalf("decode embedded Go blob: %v", err)
	}
	lang.Name = "go"
	return lang
}

// TestParserCoreLanguageTablesFootprint measures the retained heap footprint of
// one language's converted compact tables. It backs the retention decision
// documented on acquireParserCoreLanguageTables: the per-language footprint is
// small and fixed, and the cache lives on the Language itself, so it pins
// nothing beyond the Language it describes.
func TestParserCoreLanguageTablesFootprint(t *testing.T) {
	lang := loadCertifiedGoLanguageForTest(t)
	parser := NewParser(lang)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&before)
	tables, err := buildParserCoreLanguageTables(parser)
	if err != nil {
		t.Fatalf("build language tables: %v", err)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	footprint := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	runtime.KeepAlive(tables)
	runtime.KeepAlive(parser)

	t.Logf("go compact language-table footprint: %d bytes (%.1f KiB); actionRows=%d reductionPlans=%d reductionPlanIndex=%d stride=%d",
		footprint, float64(footprint)/1024,
		len(tables.actionRows), len(tables.reductionPlans), len(tables.reductionPlanIndex), tables.reductionPlanStride)

	// Guard the retention assumption: one language table set stays small. A large
	// regression here would invalidate the unbounded-cache decision.
	if footprint > 4<<20 {
		t.Fatalf("go compact language-table footprint %d bytes exceeds the 4 MiB retention guard", footprint)
	}
}

// TestParserCoreLanguageTablesCacheSharesAcrossParsers proves the per-language
// cache returns the same immutable table set for every Parser of one *Language,
// so a fresh Parser per file does not rebuild the action-table conversion.
func TestParserCoreLanguageTablesCacheSharesAcrossParsers(t *testing.T) {
	lang := loadCertifiedGoLanguageForTest(t)

	first, err := newParserCoreRootTables(NewParser(lang))
	if err != nil {
		t.Fatalf("first tables: %v", err)
	}
	second, err := newParserCoreRootTables(NewParser(lang))
	if err != nil {
		t.Fatalf("second tables: %v", err)
	}

	if first == second {
		t.Fatal("expected distinct per-Parser wrappers")
	}
	if first.parser == second.parser {
		t.Fatal("expected distinct Parser back-references")
	}
	// The heavy immutable tables must be the exact shared instance.
	if first.parserCoreLanguageTables != second.parserCoreLanguageTables {
		t.Fatal("expected the shared language tables to be reused across Parsers")
	}
	if &first.actionRows[0] != &second.actionRows[0] {
		t.Fatal("expected the action-row backing array to be shared")
	}

	// Building a wrapper for an already-cached language must not rebuild the
	// converted action rows: only the tiny per-Parser wrapper allocates.
	wrapperParser := NewParser(lang)
	if allocs := testing.AllocsPerRun(100, func() {
		wrapper, buildErr := newParserCoreRootTables(wrapperParser)
		if buildErr != nil || wrapper == nil {
			t.Fatalf("cached wrapper build: %v", buildErr)
		}
	}); allocs > 1 {
		t.Fatalf("cached wrapper build allocated %v objects, expected one tiny fixed wrapper", allocs)
	}

	// A distinct *Language builds and caches its own table set.
	other := loadCertifiedGoLanguageForTest(t)
	otherTables, err := newParserCoreRootTables(NewParser(other))
	if err != nil {
		t.Fatalf("other-language tables: %v", err)
	}
	if otherTables.parserCoreLanguageTables == first.parserCoreLanguageTables {
		t.Fatal("distinct languages must not share one converted table set")
	}

	// The shared tables answer runtime lookups identically through either wrapper.
	var wantRow core.ActionRow
	for state := core.StateID(0); state < core.StateID(len(lang.ParseTable)); state++ {
		row, lookupErr := first.Actions(state, 0)
		if lookupErr == nil && row.Len() > 0 {
			wantRow = row
			otherRow, otherErr := second.Actions(state, 0)
			if otherErr != nil || otherRow.Len() != wantRow.Len() {
				t.Fatalf("shared tables disagreed at state %d: %d vs %d err=%v", state, wantRow.Len(), otherRow.Len(), otherErr)
			}
			break
		}
	}
}

func TestParserCoreRootTablesIdentityUsesLanguageBlobHash(t *testing.T) {
	firstLanguage := loadCertifiedGoLanguageForTest(t)
	secondLanguage := loadCertifiedGoLanguageForTest(t)
	first, err := newParserCoreRootTables(NewParser(firstLanguage))
	if err != nil {
		t.Fatal(err)
	}
	second, err := newParserCoreRootTables(NewParser(secondLanguage))
	if err != nil {
		t.Fatal(err)
	}
	want, ok := firstLanguage.GrammarBlobSHA256()
	if !ok {
		t.Fatal("certified language has no blob identity")
	}
	got, ok := first.TableIdentity()
	if !ok || got != want {
		t.Fatalf("first table identity=%x/%t, want blob=%x", got, ok, want)
	}
	other, ok := second.TableIdentity()
	if !ok || other != want {
		t.Fatalf("second table identity=%x/%t, want blob=%x", other, ok, want)
	}
}
