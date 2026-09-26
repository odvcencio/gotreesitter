//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"unsafe"
)

// The per-language derived parser tables are built once and shared by every
// Parser of that Language, where NewParser previously rebuilt them per call.
// These tests prove the shared tables are the SAME tables, that the build is
// safe under concurrent first use, and that supported post-load changes do
// not change the tables unexpectedly.

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

func derivedTablesTestLanguageFromBlob(t *testing.T, blobName string) *Language {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("grammars", "grammar_blobs", blobName))
	if err != nil {
		t.Fatalf("read %s: %v", blobName, err)
	}
	lang, err := LoadLanguage(data)
	if err != nil {
		t.Fatalf("decode %s: %v", blobName, err)
	}
	lang.Name = strings.TrimSuffix(filepath.Base(blobName), ".bin")
	return lang
}

// TestParserDerivedTablesMatchFreshBuilds is the correctness claim the whole
// change rests on: memoizing must not change WHAT is built, only how often.
// It rebuilds each table directly from the Language and compares against the
// memo, field by field. This catches both stale inputs and builder drift.
func TestParserDerivedTablesMatchFreshBuilds(t *testing.T) {
	assertParserDerivedTablesMatchFreshBuilds(t, derivedTablesTestLanguage(t))
}

func TestParserDerivedTablesMatchFreshBuildsCSharp(t *testing.T) {
	assertParserDerivedTablesMatchFreshBuilds(t, derivedTablesTestLanguageFromBlob(t, "c_sharp.bin"))
}

func TestParserDerivedTablesMatchFreshBuildsCpp(t *testing.T) {
	assertParserDerivedTablesMatchFreshBuilds(t, derivedTablesTestLanguageFromBlob(t, "cpp.bin"))
}

func TestParserDerivedTablesMatchFreshBuildsKotlin(t *testing.T) {
	assertParserDerivedTablesMatchFreshBuilds(t, derivedTablesTestLanguageFromBlob(t, "kotlin.bin"))
}

func TestParserDerivedTablesMatchFreshBuildsSql(t *testing.T) {
	assertParserDerivedTablesMatchFreshBuilds(t, derivedTablesTestLanguageFromBlob(t, "sql.bin"))
}

func assertParserDerivedTablesMatchFreshBuilds(t *testing.T, lang *Language) {
	t.Helper()
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
	if !reflect.DeepEqual(derived.reduceChainHints, buildReduceChainHints(lang)) {
		t.Fatal("memoized reduceChainHints differ from a fresh build")
	}
	if !reflect.DeepEqual(derived.reduceChainHintByState, buildReduceChainHintIndex(buildReduceChainHints(lang))) {
		t.Fatal("memoized reduceChainHintByState differs from a fresh build")
	}
	if !reflect.DeepEqual(derived.reduceAliasSeq, buildReduceAliasSequences(lang)) {
		t.Fatal("memoized reduceAliasSeq differs from a fresh build")
	}
	if !reflect.DeepEqual(derived.aliasTargetSymbol, buildAliasTargetSymbols(lang)) {
		t.Fatal("memoized aliasTargetSymbol differs from a fresh build")
	}
	if !reflect.DeepEqual(derived.reduceHasFields, buildReduceFieldPresence(lang)) {
		t.Fatal("memoized reduceHasFields differs from a fresh build")
	}
	if !reflect.DeepEqual(derived.reduceFieldPlans, buildReduceFieldPlans(lang)) {
		t.Fatal("memoized reduceFieldPlans differ from a fresh build")
	}
	freshRecoverByState, freshHasRecoverState, freshHasRecoverSymbol := buildRecoverActionsByState(lang)
	if !reflect.DeepEqual(derived.recoverByState, freshRecoverByState) {
		t.Fatal("memoized recoverByState differs from a fresh build")
	}
	if !reflect.DeepEqual(derived.hasRecoverState, freshHasRecoverState) {
		t.Fatal("memoized hasRecoverState differs from a fresh build")
	}
	if !reflect.DeepEqual(derived.hasRecoverSymbol, freshHasRecoverSymbol) {
		t.Fatal("memoized hasRecoverSymbol differs from a fresh build")
	}
	if !reflect.DeepEqual(derived.hasKeywordState, buildKeywordStates(lang)) {
		t.Fatal("memoized hasKeywordState differs from a fresh build")
	}
	freshLookup := &Parser{
		language:         lang,
		denseLimit:       languageDenseLimit(lang),
		smallBase:        int(lang.LargeStateCount),
		smallTokenLookup: buildSmallTokenLookup(lang),
	}
	freshLookup.smallLookup = buildSmallLookup(lang, freshLookup.smallTokenLookup)
	freshExternalValid := freshLookup.buildExternalValidByState()
	if !reflect.DeepEqual(derived.externalValidByState, freshExternalValid) {
		t.Fatal("memoized externalValidByState differs from a fresh build")
	}
	if !reflect.DeepEqual(derived.externalValidMaskByState, buildExternalValidMaskByState(freshExternalValid, len(lang.ExternalSymbols))) {
		t.Fatal("memoized externalValidMaskByState differs from a fresh build")
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

// TestParserDerivedTablesAreSharedNotCopied proves the original memo tables
// are shared: two Parsers of one Language must receive the same arrays.
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

func TestParserDerivedQ3TablesAreSharedNotCopied(t *testing.T) {
	lang := derivedTablesTestLanguageFromBlob(t, "c_sharp.bin")
	lang.ExternalLexStates = nil
	first, second := NewParser(lang), NewParser(lang)

	if len(first.externalValidByState) == 0 {
		t.Fatal("C# fixture produced no external-valid rows; it cannot prove sharing")
	}
	if len(first.reduceFieldPlans) == 0 {
		t.Fatal("C# fixture produced no reduce field plans; it cannot prove sharing")
	}
	if len(first.recoverByState) == 0 {
		t.Fatal("C# fixture produced no recovery actions; it cannot prove sharing")
	}
	if len(first.hasKeywordState) == 0 {
		t.Fatal("C# fixture produced no keyword states; it cannot prove sharing")
	}
	for _, probe := range []struct {
		name        string
		left, right func() (uintptr, int)
	}{
		{"externalValidByState",
			func() (uintptr, int) { return sliceHead(first.externalValidByState) },
			func() (uintptr, int) { return sliceHead(second.externalValidByState) }},
		{"externalValidMaskByState",
			func() (uintptr, int) { return sliceHead(first.externalValidMaskByState) },
			func() (uintptr, int) { return sliceHead(second.externalValidMaskByState) }},
		{"reduceChainHints",
			func() (uintptr, int) { return sliceHead(first.reduceChainHints) },
			func() (uintptr, int) { return sliceHead(second.reduceChainHints) }},
		{"reduceChainHintByState",
			func() (uintptr, int) { return sliceHead(first.reduceChainHintByState) },
			func() (uintptr, int) { return sliceHead(second.reduceChainHintByState) }},
		{"reduceAliasSeq",
			func() (uintptr, int) { return sliceHead(first.reduceAliasSeq) },
			func() (uintptr, int) { return sliceHead(second.reduceAliasSeq) }},
		{"aliasTargetSymbol",
			func() (uintptr, int) { return sliceHead(first.aliasTargetSymbol) },
			func() (uintptr, int) { return sliceHead(second.aliasTargetSymbol) }},
		{"reduceHasFields",
			func() (uintptr, int) { return sliceHead(first.reduceHasFields) },
			func() (uintptr, int) { return sliceHead(second.reduceHasFields) }},
		{"reduceFieldPlans",
			func() (uintptr, int) { return sliceHead(first.reduceFieldPlans) },
			func() (uintptr, int) { return sliceHead(second.reduceFieldPlans) }},
		{"recoverByState",
			func() (uintptr, int) { return sliceHead(first.recoverByState) },
			func() (uintptr, int) { return sliceHead(second.recoverByState) }},
		{"hasRecoverState",
			func() (uintptr, int) { return sliceHead(first.hasRecoverState) },
			func() (uintptr, int) { return sliceHead(second.hasRecoverState) }},
		{"hasRecoverSymbol",
			func() (uintptr, int) { return sliceHead(first.hasRecoverSymbol) },
			func() (uintptr, int) { return sliceHead(second.hasRecoverSymbol) }},
		{"hasKeywordState",
			func() (uintptr, int) { return sliceHead(first.hasKeywordState) },
			func() (uintptr, int) { return sliceHead(second.hasKeywordState) }},
	} {
		leftPtr, leftLen := probe.left()
		rightPtr, rightLen := probe.right()
		if leftLen != rightLen || leftPtr != rightPtr {
			t.Fatalf("two Parsers of one Language hold different %s arrays", probe.name)
		}
	}
}

func TestParserDerivedTablesHonorExternalLexStateAttachment(t *testing.T) {
	lang := derivedTablesTestLanguageFromBlob(t, "c_sharp.bin")
	lang.ExternalLexStates = nil
	first := NewParser(lang)
	if len(first.externalValidByState) == 0 {
		t.Fatal("C# fixture produced no external-valid rows before scanner-state attachment")
	}

	lang.ExternalLexStates = [][]bool{make([]bool, len(lang.ExternalSymbols))}
	second := NewParser(lang)
	if len(second.externalValidByState) != 0 || len(second.externalValidMaskByState) != 0 {
		t.Fatal("NewParser installed fallback tables after scanner-state attachment")
	}
}

func TestParserDerivedReduceChainHintsAreShared(t *testing.T) {
	lang := derivedTablesTestLanguageFromBlob(t, "python.bin")
	first, second := NewParser(lang), NewParser(lang)
	if parseReduceChainHintsEnabled() && len(first.reduceChainHints) == 0 {
		t.Fatal("enabled Python reduce-chain hints are missing from the memo")
	}
	if len(first.reduceChainHints) == 0 {
		return
	}
	firstHintsPtr, firstHintsLen := sliceHead(first.reduceChainHints)
	secondHintsPtr, secondHintsLen := sliceHead(second.reduceChainHints)
	if firstHintsPtr != secondHintsPtr || firstHintsLen != secondHintsLen {
		t.Fatal("two Parsers of one Language hold different reduceChainHints arrays")
	}
	firstIndexPtr, firstIndexLen := sliceHead(first.reduceChainHintByState)
	secondIndexPtr, secondIndexLen := sliceHead(second.reduceChainHintByState)
	if firstIndexPtr != secondIndexPtr || firstIndexLen != secondIndexLen {
		t.Fatal("two Parsers of one Language hold different reduceChainHintByState arrays")
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
	results := make([]*Parser, goroutines)
	for i := range results {
		done.Add(1)
		go func(index int) {
			defer done.Done()
			start.Wait()
			results[index] = NewParser(lang)
		}(i)
	}
	start.Done()
	done.Wait()

	for i, got := range results {
		if got == nil {
			t.Fatalf("goroutine %d received no Parser", i)
		}
		if got.language.parserDerived != results[0].language.parserDerived {
			t.Fatalf("goroutine %d received a different derived-table instance; the build ran more than once", i)
		}
	}
}

// TestParserDerivedTablesReadOnlyPostLoadMutableFields is the scoping guard.
// Callers DO mutate a *Language after load: runtime profiles set the compact
// certification flags, and scanner attach swaps ExternalScanner. The memo
// does not depend on those fields. ExternalLexStates is handled separately:
// NewParser checks its current value before installing fallback tables.
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
	if !reflect.DeepEqual(derived, rebuilt) {
		t.Fatal("derived parser tables changed after a supported post-load mutation")
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

	retained := unsafe.Sizeof(*derived)
	for _, row := range derived.smallTokenLookup {
		retained += unsafe.Sizeof(row) + uintptr(len(row))*unsafe.Sizeof(uint16(0))
	}
	for _, row := range derived.smallLookup {
		retained += unsafe.Sizeof(row) + uintptr(len(row))*unsafe.Sizeof(smallActionPair{})
	}
	for _, row := range derived.externalValidByState {
		retained += unsafe.Sizeof(row) + uintptr(len(row))*unsafe.Sizeof(uint16(0))
	}
	retained += uintptr(len(derived.externalValidMaskByState)) * unsafe.Sizeof(uint64(0))
	retained += uintptr(len(derived.classifiedActions)) * unsafe.Sizeof(classifiedParseAction{})
	retained += uintptr(len(derived.eagerDefaultReduces)) * unsafe.Sizeof(eagerDefaultReduceAction{})
	for _, hint := range derived.reduceChainHints {
		retained += unsafe.Sizeof(hint) + uintptr(len(hint.terminalStates))*unsafe.Sizeof(StateID(0))
	}
	retained += uintptr(len(derived.reduceChainHintByState)) * unsafe.Sizeof(int(0))
	retained += uintptr(len(derived.reduceAliasSeq)) * unsafe.Sizeof([]Symbol(nil))
	retained += uintptr(len(derived.aliasTargetSymbol))
	retained += uintptr(len(derived.keepSameNamedAnonChildSymbol))
	retained += uintptr(len(derived.sharedAnonymousTokenSymbol))
	retained += uintptr(len(derived.reduceHasFields))
	retained += uintptr(len(derived.reduceFieldPlans)) * unsafe.Sizeof(reduceFieldPlan{})
	for _, plan := range derived.reduceFieldPlans {
		retained += uintptr(len(plan.fieldIDs)) * unsafe.Sizeof(FieldID(0))
		retained += uintptr(len(plan.inherited))
		retained += uintptr(len(plan.conflictedInherited))
	}
	for _, row := range derived.recoverByState {
		retained += unsafe.Sizeof(row) + uintptr(len(row))*unsafe.Sizeof(recoverSymbolAction{})
	}
	retained += uintptr(len(derived.hasRecoverState))
	retained += uintptr(len(derived.hasRecoverSymbol))
	retained += uintptr(len(derived.hasKeywordState))

	t.Logf("derived tables retain %d bytes per Language (go grammar): "+
		"smallTokenLookup=%d smallLookup=%d externalValidByState=%d reduceChainHints=%d "+
		"reduceAliasSeq=%d reduceFieldPlans=%d recoverByState=%d hasKeywordState=%d",
		retained, len(derived.smallTokenLookup), len(derived.smallLookup),
		len(derived.externalValidByState), len(derived.reduceChainHints), len(derived.reduceAliasSeq),
		len(derived.reduceFieldPlans), len(derived.recoverByState), len(derived.hasKeywordState))

	if retained == 0 {
		t.Fatal("measured no retained derived tables; the fixture proves nothing")
	}
	const ceiling = 4 << 20 // 4 MiB, an order-of-magnitude guard, not a tight bound
	if retained > ceiling {
		t.Fatalf("derived tables retain %d bytes per Language, ceiling %d", retained, ceiling)
	}
}
