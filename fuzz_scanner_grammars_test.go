package gotreesitter_test

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// This file fuzzes every one of gotreesitter's 119 hand-ported external
// scanners with arbitrary bytes (hardening/fuzz-blob-safety finding #3).
// Only the Go grammar had parser fuzzing before this file existed
// (fuzz_parser_test.go); every other scanner-backed grammar had none.
//
// Go's native fuzzer requires one top-level FuzzXxx function per target, so
// this file is table-driven in the sense that matters: fuzzScannerGrammar
// below is the one place that knows how to fuzz a grammar by name (seed
// corpus, parser setup, safety budget, and the pass/fail/finding decision),
// and every FuzzScanner_<Name> function underneath is a one-line call into
// it. Regenerate the list of thin wrappers from
// grammars/runtime/builtin_scanners_gen.go's RegisterXxxSupport calls if the
// scanner roster changes; nothing else here needs to change per grammar.
//
// Run one target directly, e.g.:
//
//	go test . -run '^$' -fuzz 'FuzzScanner_Python$' -fuzztime 60s
//
// scannerFuzzTop20 (below) lists the 20 most-used scanner grammars this
// slice actually ran on buildbox; every other target exists and works the
// same way but was not part of that specific sweep.

const (
	// scannerFuzzMaxInputBytes bounds the input size fuzzed per call. Larger
	// inputs are skipped rather than fuzzed: this harness is hunting for
	// crashes and pathological-cost shapes in small, adversarial inputs
	// (mirroring how a real external scanner sees a handful of bytes of
	// lookahead at a time), not doing a throughput benchmark.
	scannerFuzzMaxInputBytes = 1 << 12 // 4 KiB

	// scannerFuzzTimeoutMicros bounds each fuzzed parse's wall-clock time via
	// the parser's own timeout mechanism (see README.md "Strict parsing and
	// partial trees"), so a pathological input returns a partial tree
	// instead of hanging the fuzz worker.
	scannerFuzzTimeoutMicros = 250_000 // 250ms

	// scannerFuzzMemoryBudgetBytes bounds each fuzzed parse's own memory
	// budget (Parser.SetMemoryBudgetBytes / WithParserPoolMemoryBudgetBytes),
	// so a pathological input stops with a partial tree instead of growing
	// without bound.
	scannerFuzzMemoryBudgetBytes = 64 << 20 // 64 MiB

	// scannerFuzzAllocationBudgetBytes is the "sane budget" allocations must
	// stay under per fuzzed call, measured via runtime.MemStats.TotalAlloc
	// delta. It is an observability threshold, not a hard resource limit
	// (scannerFuzzMemoryBudgetBytes is the hard limit): exceeding it on an
	// unlisted grammar fails the fuzz run so a new pathological-allocation
	// shape gets triaged instead of silently shipped; a grammar already
	// listed in scannerFuzzAllocationKnownExceptions only logs.
	scannerFuzzAllocationBudgetBytes = 16 << 20 // 16 MiB
)

// scannerFuzzAllocationKnownExceptions documents scanner-backed grammars
// whose Parse call is already known to blow scannerFuzzAllocationBudgetBytes
// on some malformed input, with the finding. This mirrors
// grammars/language_memory_ceiling_test.go's
// languageMemoryCeilingKnownExceptions: do not add an entry casually, and
// record why. A grammar's actual worst case can be far larger than
// scannerFuzzMemoryBudgetBytes would suggest, because
// scannerFuzzMemoryBudgetBytes bounds retained parse-tree memory, not every
// transient allocation along the way (e.g. an internal parser retry or a
// scanner building and discarding large scratch structures per call).
//
// Every entry here should have a committed regression corpus file under
// testdata/fuzz/FuzzScanner_<Name>/ (see TestFuzzScannerKnownExceptionsHaveCorpusEntries)
// -- an exception with no reproducer is unverifiable and easy to go stale.
var scannerFuzzAllocationKnownExceptions = map[string]string{
	// Recorded from hardening/fuzz-blob-safety's scanner sweep, 2026-09-23,
	// buildbox (Intel Xeon D-2141I) and confirmed locally: a 10-byte
	// malformed input (testdata/fuzz/FuzzScanner_Swift/b577283e9c68616e,
	// bytes \xbbH4 ?A0""$) drives a Swift Parse call to allocate roughly
	// 17-38MB (host-dependent), consistently on every call with this input,
	// not just a first-touch cache-warming cost -- see the harness's
	// warm-up loop above, which already primes the common lazy caches and
	// still does not absorb this one. Swift is one of the largest and most
	// structurally complex shipped grammars (see
	// grammars/language_memory_ceiling_test.go); this reads as GLR
	// conflict/error-recovery exploration cost scaling with table size on
	// adversarial input, not an unbounded or attacker-amplifiable blowup
	// (scannerFuzzMemoryBudgetBytes still bounds the retained tree). Root
	// cause not isolated in this slice.
	"swift": "10-byte input allocates ~17-38MB per call (host-dependent); see testdata/fuzz/FuzzScanner_Swift/b577283e9c68616e",
}

// scannerFuzzTop20 are the 20 most-used scanner-backed grammars, the ones
// this slice ran an actual fuzzing sweep against on buildbox (about 60s
// each). Every FuzzScanner_<Name> function below works identically for the
// other 99 grammars; this set only records which ones already got a sweep.
var scannerFuzzTop20 = map[string]bool{
	"python": true, "javascript": true, "typescript": true, "tsx": true,
	"html": true, "ruby": true, "php": true, "bash": true, "yaml": true,
	"toml": true, "markdown": true, "kotlin": true, "swift": true,
	"scala": true, "lua": true, "rust": true, "perl": true, "sql": true,
	"scss": true, "dart": true,
}

// TestScannerFuzzTop20NamesAreRegistered catches a typo in scannerFuzzTop20
// (a name that silently fuzzes nothing, since fuzzScannerGrammar just skips
// an unregistered name) as a normal test failure instead of a silent gap in
// the recorded sweep coverage.
func TestScannerFuzzTop20NamesAreRegistered(t *testing.T) {
	for name := range scannerFuzzTop20 {
		if grammars.DetectLanguageByName(name) == nil {
			t.Errorf("scannerFuzzTop20 contains %q, which is not a registered grammar", name)
		}
	}
}

// TestFuzzScannerKnownExceptionsHaveCorpusEntries keeps
// scannerFuzzAllocationKnownExceptions honest: every excepted grammar must
// have at least one committed regression corpus file under
// testdata/fuzz/FuzzScanner_<Name>/, so the exception is a verified,
// reproducible finding (replayed on every `go test .`) rather than a
// standing, unverifiable waiver that quietly suppresses future findings too.
func TestFuzzScannerKnownExceptionsHaveCorpusEntries(t *testing.T) {
	for name := range scannerFuzzAllocationKnownExceptions {
		entry := grammars.DetectLanguageByName(name)
		if entry == nil {
			t.Errorf("scannerFuzzAllocationKnownExceptions contains %q, which is not a registered grammar", name)
			continue
		}
		dir := "testdata/fuzz/FuzzScanner_" + scannerFuzzFuncSuffix(entry.Name)
		files, err := os.ReadDir(dir)
		if err != nil || len(files) == 0 {
			t.Errorf("scannerFuzzAllocationKnownExceptions[%q] has no committed corpus entries under %s", name, dir)
		}
	}
}

// scannerFuzzFuncSuffix maps a grammar's registry name to its
// FuzzScanner_<Suffix> function-name suffix, matching
// fuzz_scanner_grammars_wrappers_test.go's generated names (e.g. "c_sharp"
// -> "C_sharp"): the first letter capitalized, everything else unchanged.
func scannerFuzzFuncSuffix(name string) string {
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// scannerFuzzSeeds supplements grammars.ParseSmokeSamples with inputs shaped
// to exercise a scanner's edge handling (empty input, a lone byte, deeply
// nested brackets, and an unterminated multi-byte UTF-8 sequence), which a
// short valid-code sample does not reach.
func scannerFuzzSeeds() [][]byte {
	return [][]byte{
		[]byte(""),
		[]byte("\x00"),
		[]byte("\n"),
		[]byte("((((((((((((((((\n"),
		[]byte("\"\"\"\"\"\"\"\"\n"),
		{0xE2, 0x82}, // truncated 3-byte UTF-8 sequence (would be U+20AC)
		[]byte("\xff\xfe\xfd"),
	}
}

// fuzzScannerGrammar fuzzes name's Parse path: no panic, the parse
// terminates (bounded by scannerFuzzTimeoutMicros /
// scannerFuzzMemoryBudgetBytes), and allocations stay under
// scannerFuzzAllocationBudgetBytes unless name is a known exception.
func fuzzScannerGrammar(f *testing.F, name string) {
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		f.Skipf("grammar %q is not registered", name)
	}
	lang := entry.Language()
	if lang == nil {
		f.Skipf("grammar %q has no Language", name)
	}

	if sample, ok := grammars.ParseSmokeSamples[entry.Name]; ok {
		f.Add([]byte(sample))
	}
	for _, seed := range scannerFuzzSeeds() {
		f.Add(seed)
	}

	pool := gotreesitter.NewParserPool(lang,
		gotreesitter.WithParserPoolMemoryBudgetBytes(scannerFuzzMemoryBudgetBytes),
		gotreesitter.WithParserPoolTimeoutMicros(scannerFuzzTimeoutMicros),
	)

	// Warm the Language's lazily built, sync.Once-guarded caches (ASCII DFA
	// transition table, compact parser tables, parser-derived tables, ...)
	// before fuzzing starts. Each of those is a one-time, often multi-MB
	// allocation on whichever call happens to check out this Language's
	// Parser first; without this warm-up, that one-time setup cost lands on
	// the first fuzzed input and swamps scannerFuzzAllocationBudgetBytes
	// regardless of input content, making the allocation check meaningless.
	//
	// One warm-up call is not always enough: some of this lazy setup is keyed
	// by which lexical category a call actually visits (e.g. an identifier
	// path stays cold until something identifier-shaped is parsed, even
	// after a whitespace-only warm-up), so several small, lexically varied
	// inputs are warmed here instead of just one.
	for _, w := range [][]byte{[]byte(" "), []byte("x"), []byte("1"), []byte("_"), []byte("\"a\""), []byte("//"), []byte("(")} {
		if warm, err := pool.Parse(w); err == nil {
			warm.Release()
		}
	}

	f.Fuzz(func(t *testing.T, src []byte) {
		if len(src) > scannerFuzzMaxInputBytes {
			t.Skip()
		}
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("%s: panic while parsing fuzz input (%d bytes): %v", name, len(src), r)
			}
		}()

		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		start := time.Now()

		var tree *gotreesitter.Tree
		var err error
		if entry.TokenSourceFactory != nil {
			ts := entry.TokenSourceFactory(src, lang)
			tree, err = pool.ParseWithTokenSource(src, ts)
		} else {
			tree, err = pool.Parse(src)
		}
		elapsed := time.Since(start)
		runtime.ReadMemStats(&after)
		if tree != nil {
			tree.Release()
		}
		// A non-nil error or a partial (early-stopped) tree is expected and
		// fine for malformed input -- ParseStopReason, not err, is how the
		// parser reports a timeout/budget stop; err staying nil for a
		// partial tree is documented behavior (see README.md "Strict
		// parsing and partial trees"). Only panics, non-termination, and
		// runaway allocation are findings here.
		_ = err

		if elapsed > 2*time.Second {
			t.Fatalf("%s: parsing %d bytes took %s; the parser's own timeout (%dus) should have stopped it well before this", name, len(src), elapsed, scannerFuzzTimeoutMicros)
		}

		if after.TotalAlloc < before.TotalAlloc {
			return // wrapped counter (effectively never, but stay defensive)
		}
		allocDelta := after.TotalAlloc - before.TotalAlloc
		if allocDelta <= scannerFuzzAllocationBudgetBytes {
			return
		}
		if reason, known := scannerFuzzAllocationKnownExceptions[name]; known {
			t.Logf("%s: parsing %d bytes allocated %d bytes (> %d budget), known exception: %s", name, len(src), allocDelta, scannerFuzzAllocationBudgetBytes, reason)
			return
		}
		t.Fatalf("%s: parsing %d bytes allocated %d bytes, exceeding the %d byte budget and not in scannerFuzzAllocationKnownExceptions -- this is a new finding", name, len(src), allocDelta, scannerFuzzAllocationBudgetBytes)
	})
}
