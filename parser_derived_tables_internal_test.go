//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"reflect"
	"sync"
	"testing"
	"unsafe"
)

// The per-language derived parser tables are built once and shared by every
// Parser of that Language, where NewParser previously rebuilt them per call.
// These tests prove the shared tables are the SAME tables, that the build is
// safe under concurrent first use, and that the inputs the builders read are
// disjoint from the fields callers mutate after load.

// derivedTablesTestLanguage decodes a fresh Language from the certified Go
// blob for each test. A fresh decode matters: the memo is per-Language, so
// sharing one instance across tests would let an earlier test's build satisfy
// a later test's first-use assertion.
//
// A decode failure is FATAL, never a skip. The blob is embedded at compile
// time and its SHA is pinned, so failing to decode it is a defect in the blob
// or the decoder, not an environment condition. Skipping would turn every test
// in this file -- including the only race-lane coverage of the memo -- green
// with zero assertions executed.
//
// It mirrors loadCertifiedGoLanguageForTest, which lives behind the positive
// gts_parsercorephase0 tag and so is not visible here. Name is set for the
// same reason that helper sets it: buildSmallTokenLookup branches on it.
func derivedTablesTestLanguage(t *testing.T) *Language {
	t.Helper()
	lang, err := LoadLanguage(parserCoreCertifiedGoBlob)
	if err != nil {
		t.Fatalf("decode embedded Go blob: %v", err)
	}
	lang.Name = "go"
	return lang
}

// TestParserDerivedTablesMatchFreshBuilds is the correctness claim the whole
// change rests on: memoizing must not change WHAT is built, only how often.
// It rebuilds each table directly from the Language and compares against the
// memoized instance the Parser now receives.
func TestParserDerivedTablesMatchFreshBuilds(t *testing.T) {
	lang := derivedTablesTestLanguage(t)
	derived := lang.acquireParserDerivedTables()

	freshSmallTokenLookup := buildSmallTokenLookup(lang)
	if !reflect.DeepEqual(derived.smallTokenLookup, freshSmallTokenLookup) {
		t.Fatal("memoized smallTokenLookup differs from a fresh build")
	}
	if !reflect.DeepEqual(derived.smallLookup, buildSmallLookup(lang, freshSmallTokenLookup)) {
		t.Fatal("memoized smallLookup differs from a fresh build")
	}
	if !reflect.DeepEqual(derived.classifiedActions, buildClassifiedParseActions(lang)) {
		t.Fatal("memoized classifiedActions differs from a fresh build")
	}
	if !reflect.DeepEqual(derived.keepSameNamedAnonChildSymbol, buildKeepSameNamedAnonChildSymbols(lang)) {
		t.Fatal("memoized keepSameNamedAnonChildSymbol differs from a fresh build")
	}
	if !reflect.DeepEqual(derived.sharedAnonymousTokenSymbol, buildSharedAnonymousTokenSymbols(lang)) {
		t.Fatal("memoized sharedAnonymousTokenSymbol differs from a fresh build")
	}

	// eagerDefaultReduces is built through the explicit action-table view.
	// Build it the way a real Parser would and require the memoized copy to
	// match, which cross-checks the view the memo assembles against the one a
	// constructed Parser projects. denseLimit cannot differ between them --
	// both call languageDenseLimit -- so what this genuinely covers is
	// smallBase and the three table fields.
	reference := NewParser(lang)
	if !reflect.DeepEqual(derived.eagerDefaultReduces, buildEagerDefaultReduceActions(reference.actionTableView())) {
		t.Fatal("memoized eagerDefaultReduces differs from one built through a real Parser")
	}
}

// TestParserDerivedTablesAreSharedNotCopied proves the memo actually shares:
// two Parsers of one Language must receive the identical backing arrays, which
// is where the allocation saving comes from.
//
// It checks EVERY memoized table, including eagerDefaultReduces -- the largest
// one, and the only one built through the explicit action-table view rather
// than directly from the Language. A regression that copied just that table
// per Parser would otherwise pass while undoing most of the win.
func TestParserDerivedTablesAreSharedNotCopied(t *testing.T) {
	lang := derivedTablesTestLanguage(t)
	first, second := NewParser(lang), NewParser(lang)

	// Assert on every table, and require each to be non-empty rather than
	// skipping it: a guard that tolerates an empty table lets this test go
	// green having compared nothing.
	for _, probe := range []struct {
		name        string
		left, right func() (uintptr, int)
	}{
		{"classifiedActions",
			func() (uintptr, int) { return sliceHead(first.classifiedActions) },
			func() (uintptr, int) { return sliceHead(second.classifiedActions) }},
		{"eagerDefaultReduces",
			func() (uintptr, int) { return sliceHead(first.eagerDefaultReduces) },
			func() (uintptr, int) { return sliceHead(second.eagerDefaultReduces) }},
		{"smallTokenLookup",
			func() (uintptr, int) { return sliceHead(first.smallTokenLookup) },
			func() (uintptr, int) { return sliceHead(second.smallTokenLookup) }},
		{"smallLookup",
			func() (uintptr, int) { return sliceHead(first.smallLookup) },
			func() (uintptr, int) { return sliceHead(second.smallLookup) }},
		{"sharedAnonymousTokenSymbol",
			func() (uintptr, int) { return sliceHead(first.sharedAnonymousTokenSymbol) },
			func() (uintptr, int) { return sliceHead(second.sharedAnonymousTokenSymbol) }},
	} {
		leftPtr, leftLen := probe.left()
		rightPtr, rightLen := probe.right()
		if leftLen == 0 {
			t.Fatalf("%s is empty for the certified Go blob; the fixture cannot prove sharing", probe.name)
		}
		if leftPtr != rightPtr || leftLen != rightLen {
			t.Fatalf("two Parsers of one Language hold different %s arrays", probe.name)
		}
	}
}

// sliceHead returns a slice's backing-array address and length, which together
// identify the exact allocation two Parsers must be sharing.
func sliceHead[T any](s []T) (uintptr, int) {
	if len(s) == 0 {
		return 0, 0
	}
	return uintptr(unsafe.Pointer(&s[0])), len(s)
}

// TestParserDerivedTablesConcurrentFirstUse covers the reason the build is
// lazy rather than eager. A cached *Language is served to every goroutine
// (grammars' embedded loader), and ParserPool constructs Parsers concurrently
// against it, so first use races by construction. Run with -race.
func TestParserDerivedTablesConcurrentFirstUse(t *testing.T) {
	lang := derivedTablesTestLanguage(t)

	const goroutines = 16
	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	results := make([]*parserDerivedTables, goroutines)
	for i := range results {
		done.Add(1)
		go func(index int) {
			defer done.Done()
			start.Wait()
			results[index] = lang.acquireParserDerivedTables()
		}(i)
	}
	start.Done()
	done.Wait()

	for i, got := range results {
		if got == nil {
			t.Fatalf("goroutine %d received no derived tables", i)
		}
		if got != results[0] {
			t.Fatalf("goroutine %d received a different derived-table instance; the build ran more than once", i)
		}
	}
}

// TestParserDerivedTablesReadOnlyPostLoadMutableFields is the scoping guard.
// Callers DO mutate a *Language after load: runtime profiles set the compact
// certification flags, and scanner attach swaps ExternalScanner. Memoizing is
// only safe because the memoized builders read none of those fields.
//
// It deliberately does NOT assert that the memo returns the same pointer after
// a mutation. sync.Once guarantees that outcome for any input whatsoever, so
// such an assertion can never fail and proves nothing. Instead it mutates the
// post-load-mutable fields and then rebuilds every memoized table from the
// mutated Language, requiring each to be unchanged. That fails loudly if a
// future change memoizes a table which reads one of these fields.
func TestParserDerivedTablesReadOnlyPostLoadMutableFields(t *testing.T) {
	lang := derivedTablesTestLanguage(t)
	derived := lang.acquireParserDerivedTables()

	restoreErrorRegion := lang.CompactStrategy2ErrorRegionCertified
	restoreRecoverEOF := lang.CompactRecoverEOFCertified
	restoreRecoverEOFReceipt := lang.CompactRecoverEOFArtifactReceipt
	restoreStackSummary := lang.CompactStackSummaryRecoveryCertified
	restoreStructuralElection := lang.CompactAcceptanceStructuralElectionCertified
	restoreLexerSkippedPrefix := lang.CompactLexerSkippedPrefixTilingCertified
	restoreMissingInsertion := lang.CompactMissingTokenInsertionCertified
	restoreS5EOFInsertion := lang.CompactS5EOFMissingInsertionCertified
	restoreErrorModeKeyword := lang.CompactRecoveryErrorModeKeywordCaptureCertified
	restoreSplitDrops := lang.CompactConvergedReductionSplitDropsCertified
	restoreScanner := lang.ExternalScanner
	t.Cleanup(func() {
		lang.CompactStrategy2ErrorRegionCertified = restoreErrorRegion
		lang.CompactRecoverEOFCertified = restoreRecoverEOF
		lang.CompactRecoverEOFArtifactReceipt = restoreRecoverEOFReceipt
		lang.CompactStackSummaryRecoveryCertified = restoreStackSummary
		lang.CompactAcceptanceStructuralElectionCertified = restoreStructuralElection
		lang.CompactLexerSkippedPrefixTilingCertified = restoreLexerSkippedPrefix
		lang.CompactMissingTokenInsertionCertified = restoreMissingInsertion
		lang.CompactS5EOFMissingInsertionCertified = restoreS5EOFInsertion
		lang.CompactRecoveryErrorModeKeywordCaptureCertified = restoreErrorModeKeyword
		lang.CompactConvergedReductionSplitDropsCertified = restoreSplitDrops
		lang.ExternalScanner = restoreScanner
	})
	lang.CompactStrategy2ErrorRegionCertified = !restoreErrorRegion
	lang.CompactRecoverEOFCertified = !restoreRecoverEOF
	lang.CompactRecoverEOFArtifactReceipt.EOFByteOffset++
	lang.CompactStackSummaryRecoveryCertified = !restoreStackSummary
	lang.CompactAcceptanceStructuralElectionCertified = !restoreStructuralElection
	lang.CompactLexerSkippedPrefixTilingCertified = !restoreLexerSkippedPrefix
	lang.CompactMissingTokenInsertionCertified = !restoreMissingInsertion
	lang.CompactS5EOFMissingInsertionCertified = !restoreS5EOFInsertion
	lang.CompactRecoveryErrorModeKeywordCaptureCertified = !restoreErrorModeKeyword
	lang.CompactConvergedReductionSplitDropsCertified = !restoreSplitDrops
	lang.ExternalScanner = nil

	rebuilt := buildParserDerivedTables(lang)
	if !reflect.DeepEqual(derived.classifiedActions, rebuilt.classifiedActions) {
		t.Fatal("classifiedActions changed after a post-load mutation")
	}
	if !reflect.DeepEqual(derived.smallTokenLookup, rebuilt.smallTokenLookup) {
		t.Fatal("smallTokenLookup changed after a post-load mutation")
	}
	if !reflect.DeepEqual(derived.smallLookup, rebuilt.smallLookup) {
		t.Fatal("smallLookup changed after a post-load mutation")
	}
	if !reflect.DeepEqual(derived.eagerDefaultReduces, rebuilt.eagerDefaultReduces) {
		t.Fatal("eagerDefaultReduces changed after a post-load mutation")
	}
	if !reflect.DeepEqual(derived.keepSameNamedAnonChildSymbol, rebuilt.keepSameNamedAnonChildSymbol) {
		t.Fatal("keepSameNamedAnonChildSymbol changed after a post-load mutation")
	}
	if !reflect.DeepEqual(derived.sharedAnonymousTokenSymbol, rebuilt.sharedAnonymousTokenSymbol) {
		t.Fatal("sharedAnonymousTokenSymbol changed after a post-load mutation")
	}
}

// TestParserDerivedTablesFootprint pins the retained size of the memo.
//
// This change MOVES memory rather than only saving it. The tables used to be
// per-Parser and short-lived; they are now per-Language and live as long as
// the Language does, which for an embedded grammar is the process lifetime
// (the grammars cache is unbounded by default). A consumer that touches many
// grammars retains this for each one, after every Parser is gone.
//
// The sibling memo this design follows pins its own footprint the same way
// (TestParserCoreLanguageTablesFootprint). Without a pin, a builder that
// started allocating per state rather than per used state would raise the
// permanent cost of every language with no signal.
//
// It measures the STRUCTURE rather than reading MemStats around the build.
// A MemStats delta here is dominated by the Language decode's own garbage and
// reads negative as often as not; counting the retained slices is exact and
// deterministic.
func TestParserDerivedTablesFootprint(t *testing.T) {
	lang := derivedTablesTestLanguage(t)
	derived := lang.acquireParserDerivedTables()

	var retained uintptr
	for _, row := range derived.smallTokenLookup {
		retained += unsafe.Sizeof(row) + uintptr(len(row))*unsafe.Sizeof(uint16(0))
	}
	for _, row := range derived.smallLookup {
		retained += unsafe.Sizeof(row) + uintptr(len(row))*unsafe.Sizeof(smallActionPair{})
	}
	retained += uintptr(len(derived.classifiedActions)) * unsafe.Sizeof(classifiedParseAction{})
	retained += uintptr(len(derived.eagerDefaultReduces)) * unsafe.Sizeof(eagerDefaultReduceAction{})
	retained += uintptr(len(derived.keepSameNamedAnonChildSymbol))
	retained += uintptr(len(derived.sharedAnonymousTokenSymbol))

	t.Logf("derived tables retain %d bytes per Language (go grammar): "+
		"smallTokenLookup=%d smallLookup=%d classifiedActions=%d eagerDefaultReduces=%d",
		retained, len(derived.smallTokenLookup), len(derived.smallLookup),
		len(derived.classifiedActions), len(derived.eagerDefaultReduces))

	if retained == 0 {
		t.Fatal("measured no retained derived tables; the fixture proves nothing")
	}
	const ceiling = 4 << 20 // 4 MiB, an order-of-magnitude guard, not a tight bound
	if retained > ceiling {
		t.Fatalf("derived tables retain %d bytes per Language, ceiling %d", retained, ceiling)
	}
}
