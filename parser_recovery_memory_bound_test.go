package gotreesitter_test

import (
	"os"
	"runtime"
	"runtime/debug"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// recoveryMemoryBoundWitnesses are four adversarial inputs, all reachable
// through production's C-recovery missing-token search
// (cHandleError/cDoAllPotentialReductions, parser_recover_c.go), which used
// to clone the whole GSS stack (gssScratch, glr_gss.go) without bound:
//
//   - erlang (4 bytes) and jsdoc (56 bytes), from
//     spore.2026-08-02.walnut-e.memory-exhaustion: 2.4-2.6 GB RSS in 12-17
//     seconds on origin/main, reproducing identically regardless of the
//     admission candidate route (compact declined both fast, before any heavy
//     allocation, every time -- the spore's section 3).
//   - erlang_pfx_008_73b and erlang_pfx_017_71b (testdata/
//     recovery_memory_bound_witnesses), found by adversarial review of the
//     first landed fix: truncated C-style block comments (a copyright-header
//     shape) with one injected control byte each, fed to the erlang grammar.
//     Pre-ceiling-fix measurements: 73 bytes -> 40.5s / 1.1 GB (arena stop
//     source); 71 bytes -> 52.0s / 695 MB.
//
// The shipped fix is two complementary layers: the always-armed soft memory
// budget also sees parserScratch's (dominated by gssScratch's) allocated
// bytes (Parser.budgetScratch, parser.go; resultMaterializationStopReason,
// parser_result.go), and two Go-side backstop ceilings bound the
// missing-token search's own total work directly
// (cRecoverMaxReductionCandidateAttempts, cRecoverMaxMissingTokenTrials,
// parser_recover_c.go) -- the ceilings are what actually stop all four
// witnesses (CRecoverReductionCandidateCeilingHits > 0 on every one); the
// budget feed is defense in depth for a growth shape the ceilings do not
// cover.
//
// A source-length-independent absolute hard ceiling (removing
// runtimeMemoryHardCeilingEnabled's floor) was tried and dropped: it
// unintentionally selected the tight (1-in-16) runtime-memory poll mask for
// every small-source parse instead of the loose (1-in-256) one, since a
// disabled soft budget (0) satisfied the same "small budget" comparison the
// tight-mask selection used, and it reopened the issue #454 determinism
// symptom class for sub-64KiB input (a 500-byte Go file returned seven
// distinct truncated trees under concurrent heap churn). See
// runtimeMemoryHardCeilingEnabled's doc comment (parser_memory_budget_runtime.go).
var recoveryMemoryBoundWitnesses = []struct {
	name string
	lang func() *gotreesitter.Language
	src  []byte
}{
	{
		name: "erlang",
		lang: grammars.ErlangLanguage,
		src:  []byte{0x6b, 0x1c, 0x03, 0x21},
	},
	{
		name: "jsdoc",
		lang: grammars.JsdocLanguage,
		src:  []byte("/**\n * @param {string} name\n * #  @returns {number}\n */\n"),
	},
	{
		name: "erlang_pfx_008_73b",
		lang: grammars.ErlangLanguage,
		src:  mustReadRecoveryWitnessFile("erlang_pfx_008_73b.bin"),
	},
	{
		name: "erlang_pfx_017_71b",
		lang: grammars.ErlangLanguage,
		src:  mustReadRecoveryWitnessFile("erlang_pfx_017_71b.bin"),
	},
}

func mustReadRecoveryWitnessFile(name string) []byte {
	src, err := os.ReadFile("testdata/recovery_memory_bound_witnesses/" + name)
	if err != nil {
		panic("mustReadRecoveryWitnessFile: " + name + ": " + err.Error())
	}
	return src
}

// recoveryMemoryBoundMaxHeapGrowthBytes generously bounds per-parse heap
// growth for the witnesses above. Measured behavior after the fix: 12-40MB
// peak heap across both witnesses and both admission routes. This ceiling
// sits an order of magnitude above that (and two orders of magnitude below
// the pre-fix 2.4-2.6 GB) so ordinary run-to-run allocator variance can never
// make this test flaky, while a genuine regression back toward unbounded
// growth still fails it long before it could threaten the host running the
// test.
const recoveryMemoryBoundMaxHeapGrowthBytes = 200 << 20 // 200 MiB

// recoveryMemoryBoundTimeoutMicros is the parser's own cooperative wall-clock
// backstop (SetTimeoutMicros), checked at the same
// resultMaterializationStopReason poll sites the memory-budget fix threads
// through (parser_recover_c.go's cDoAllPotentialReductions/cHandleError).
// Measured completion time for both witnesses after the fix is well under
// 400ms; 5 seconds is generous headroom for a slow or loaded CI host while
// still finishing an order of magnitude before the pre-fix witnesses'
// 12-17 second run to a multi-GB OOM kill. This is what makes the harness
// GOMEMLIMIT-safe without needing an actual GOMEMLIMIT/subprocess: a
// still-pathological input gets cut off by this cooperative check long
// before it could grow to a dangerous size, so this test cannot OOM the host
// even if the fix regresses.
const recoveryMemoryBoundTimeoutMicros = 5_000_000

func TestRecoveryMissingTokenSearchStaysMemoryBoundedProductionRoute(t *testing.T) {
	for _, w := range recoveryMemoryBoundWitnesses {
		w := w
		t.Run(w.name, func(t *testing.T) {
			assertRecoveryMemoryBoundedParse(t, w.name, w.lang(), w.src, false)
		})
	}
}

func TestRecoveryMissingTokenSearchStaysMemoryBoundedCompactRoute(t *testing.T) {
	for _, w := range recoveryMemoryBoundWitnesses {
		w := w
		t.Run(w.name, func(t *testing.T) {
			assertRecoveryMemoryBoundedParse(t, w.name, w.lang(), w.src, true)
		})
	}
}

func assertRecoveryMemoryBoundedParse(t *testing.T, name string, lang *gotreesitter.Language, src []byte, candidateRoute bool) {
	t.Helper()
	if raceEnabled {
		// Same rationale as TestCSharpDesignerStyleBlockStaysBounded
		// (parser_csharp_boundedness_test.go): -race's ~10x instrumentation
		// slowdown competes with the wall-clock timeout for an unrelated
		// reason. Skip the whole test rather than let the timeout mask the
		// memory-growth assertion, which is this test's actual point.
		t.Skip("wall-clock timeout contract is not meaningful under -race instrumentation")
	}

	parser := gotreesitter.NewParser(lang)
	parser.SetAdmissionCandidateRoute(candidateRoute)
	parser.SetTimeoutMicros(recoveryMemoryBoundTimeoutMicros)

	debug.SetGCPercent(-1)
	defer debug.SetGCPercent(100)
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	tree, err := parser.Parse(src)

	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	if err != nil {
		t.Fatalf("[%s candidateRoute=%t] Parse() error = %v", name, candidateRoute, err)
	}
	if tree == nil {
		t.Fatalf("[%s candidateRoute=%t] Parse() returned a nil tree with no error", name, candidateRoute)
	}
	defer tree.Release()

	rt := tree.ParseRuntime()
	if got, want := rt.StopReason, gotreesitter.ParseStopAccepted; got != want {
		t.Fatalf("[%s candidateRoute=%t] StopReason = %s, want %s -- did not resolve within the timeout backstop (%s)",
			name, candidateRoute, got, want, rt.Summary())
	}
	if tree.ParseStoppedEarly() {
		t.Fatalf("[%s candidateRoute=%t] ParseStoppedEarly = true (%s)", name, candidateRoute, rt.Summary())
	}

	heapGrowth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	if heapGrowth > recoveryMemoryBoundMaxHeapGrowthBytes {
		t.Fatalf("[%s candidateRoute=%t] heap growth = %d bytes, want <= %d bytes -- memory-exhaustion regression (spore.2026-08-02.walnut-e.memory-exhaustion) (%s)",
			name, candidateRoute, heapGrowth, recoveryMemoryBoundMaxHeapGrowthBytes, rt.Summary())
	}
	t.Logf("[%s candidateRoute=%t] heap growth = %d bytes, reductionCeilingHits=%d missingTokenCeilingHits=%d (%s)",
		name, candidateRoute, heapGrowth, rt.CRecoverReductionCandidateCeilingHits, rt.CRecoverMissingTokenCeilingHits, rt.Summary())
}

// recoveryFuzzLanguages backs FuzzRecoveryMissingTokenSearchStaysBounded. It
// is deliberately separate from FuzzAdmissionRouteEquality's
// routeEqualityFuzzLanguages (fuzz_admission_route_equality_test.go): that
// fuzzer's language list is a route-EQUALITY corpus contract ("every listed
// language passes the compact admission smoke ratchet",
// admissionScorecardRequiredCompactPasses) that jsdoc does not currently
// meet (it FALLBACKs on compact, by design -- see
// TestAdmissionCandidateScorecard206's ratchet comment). This fuzzer asserts
// a different, route-agnostic property instead: Parse must return, and must
// not allocate past a generous ceiling, regardless of which admission route
// serves the input -- exactly what spore.2026-08-02.walnut-e.memory-
// exhaustion's four-combination reproduction (two witnesses times
// compact-forced/production-forced) established was broken.
var recoveryFuzzLanguages = []struct {
	name string
	lang func() *gotreesitter.Language
}{
	{"erlang", grammars.ErlangLanguage},
	{"jsdoc", grammars.JsdocLanguage},
}

// recoveryFuzzMaxInputBytes caps fuzzed input size. The bug class this
// fuzzer targets is a small-input recovery-search explosion (both seed
// witnesses are 4 and 56 bytes), not general large-input containment, which
// FuzzGoParseDoesNotPanic and the memory-budget suite
// (parser_memory_budget_test.go) already cover; keeping the cap small keeps
// recoveryFuzzMaxHeapGrowthBytes below meaningful for every input this
// fuzzer explores.
const recoveryFuzzMaxInputBytes = 4096

// recoveryFuzzMaxHeapGrowthBytes is looser than
// recoveryMemoryBoundMaxHeapGrowthBytes above: live fuzzing explores inputs
// up to recoveryFuzzMaxInputBytes (1024x the larger seed witness), not just
// the two exact 4- and 56-byte seeds, so this bound trades some precision for
// robustness against false positives on legitimately larger fuzzed inputs.
// It still sits three orders of magnitude below the pre-fix 2.4-2.6 GB
// witnesses.
const recoveryFuzzMaxHeapGrowthBytes = 512 << 20 // 512 MiB

// FuzzRecoveryMissingTokenSearchStaysBounded is the targeted regression
// fuzzer for spore.2026-08-02.walnut-e.memory-exhaustion, seeded with both
// witnesses. Committed seeds run as ordinary regression cases under plain
// `go test` (Go's native fuzzing converts f.Add corpus entries into subtests
// without -fuzz); live mutation is opt-in via `go test -fuzz`.
func FuzzRecoveryMissingTokenSearchStaysBounded(f *testing.F) {
	f.Add([]byte{0x6b, 0x1c, 0x03, 0x21}, byte(0))                                         // erlang witness (index 0: erlang)
	f.Add([]byte("/**\n * @param {string} name\n * #  @returns {number}\n */\n"), byte(1)) // jsdoc witness (index 1: jsdoc)
	// Cross-pair each witness with the other's language too: the underlying
	// bug is in language-agnostic core GLR error recovery (the spore's
	// section 5), not either grammar's own scanner, so a byte sequence one
	// grammar finds pathological is worth trying against the other.
	f.Add([]byte{0x6b, 0x1c, 0x03, 0x21}, byte(1))
	f.Add([]byte("/**\n * @param {string} name\n * #  @returns {number}\n */\n"), byte(0))

	f.Fuzz(func(t *testing.T, src []byte, langSel byte) {
		if len(src) >= recoveryFuzzMaxInputBytes {
			t.Skip("input at or above the fuzz latency/memory safety cap")
		}
		if raceEnabled {
			t.Skip("wall-clock timeout contract is not meaningful under -race instrumentation")
		}
		lp := recoveryFuzzLanguages[int(langSel)%len(recoveryFuzzLanguages)]

		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic during recovery-boundedness fuzz (lang=%s bytes=%d): %v", lp.name, len(src), r)
			}
		}()

		parser := gotreesitter.NewParser(lp.lang())
		parser.SetTimeoutMicros(recoveryMemoryBoundTimeoutMicros)

		debug.SetGCPercent(-1)
		defer debug.SetGCPercent(100)
		var before runtime.MemStats
		runtime.ReadMemStats(&before)

		tree, err := parser.Parse(src)

		var after runtime.MemStats
		runtime.ReadMemStats(&after)

		if err != nil {
			t.Fatalf("lang=%s Parse: %v", lp.name, err)
		}
		if tree == nil {
			t.Fatalf("lang=%s Parse returned a nil tree with no error", lp.name)
		}
		defer tree.Release()

		heapGrowth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
		if heapGrowth > recoveryFuzzMaxHeapGrowthBytes {
			t.Fatalf("lang=%s heap growth = %d bytes, want <= %d bytes (%s)",
				lp.name, heapGrowth, recoveryFuzzMaxHeapGrowthBytes, tree.ParseRuntime().Summary())
		}
	})
}

// TestRecoveryCeilingsNeverTripOnGreenCorpus is the corpus assertion for
// cRecoverMaxReductionCandidateAttempts and cRecoverMaxMissingTokenTrials
// (parser_recover_c.go): both ceilings must stay unreached on every green
// (non-adversarial) input, or they would change a currently-correct recovery
// outcome instead of only bounding a pathological search. Scope: the
// smoke-sample corpus (one representative fixture per each of the 206
// registered languages, grammars.ParseSmokeSample) plus a small curated set
// of malformed-but-plausible recovery-stress snippets across widely used
// grammars -- every fixture here is embedded in the repo, so the gate runs
// identically in CI and on any contributor's machine.
func TestRecoveryCeilingsNeverTripOnGreenCorpus(t *testing.T) {
	checkOne := func(t *testing.T, label string, lang *gotreesitter.Language, source []byte) {
		t.Helper()
		if lang == nil || len(source) == 0 {
			return
		}
		p := gotreesitter.NewParser(lang)
		p.SetAdmissionCandidateRoute(false) // production: cHandleError's missing-token search only runs here
		tree, err := p.Parse(source)
		if err != nil {
			t.Fatalf("%s: Parse error: %v", label, err)
		}
		if tree == nil {
			t.Fatalf("%s: Parse returned a nil tree", label)
		}
		defer tree.Release()
		rt := tree.ParseRuntime()
		if rt.CRecoverReductionCandidateCeilingHits != 0 {
			t.Fatalf("%s: CRecoverReductionCandidateCeilingHits = %d, want 0 -- the ceiling tripped on green input, "+
				"which changes a currently-correct recovery outcome", label, rt.CRecoverReductionCandidateCeilingHits)
		}
		if rt.CRecoverMissingTokenCeilingHits != 0 {
			t.Fatalf("%s: CRecoverMissingTokenCeilingHits = %d, want 0 -- the ceiling tripped on green input, "+
				"which changes a currently-correct recovery outcome", label, rt.CRecoverMissingTokenCeilingHits)
		}
	}

	t.Run("smoke", func(t *testing.T) {
		for _, entry := range grammars.AllLanguages() {
			entry := entry
			sample := grammars.ParseSmokeSample(entry.Name)
			if sample == "" {
				continue
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("%s: panic: %v", entry.Name, r)
					}
				}()
				checkOne(t, entry.Name, entry.Language(), []byte(sample))
			}()
		}
	})

	t.Run("curated", func(t *testing.T) {
		for _, tc := range curatedRecoveryStressFixtures() {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				checkOne(t, tc.name, tc.lang(), tc.source)
			})
		}
	})
}

type curatedRecoveryStressFixture struct {
	name   string
	lang   func() *gotreesitter.Language
	source []byte
}

// curatedRecoveryStressFixtures are small, embedded (not downloaded)
// snippets picked to exercise C-recovery (cHandleError) on grammars/shapes
// known from this codebase's own history to stress it: malformed/incomplete
// input in the two languages from the 2026-08-02 memory-exhaustion witnesses'
// grammar families (erlang, a JS/jsdoc-style comment) plus a couple of
// generically malformed snippets in widely used grammars. Deliberately NOT
// the raw witness bytes themselves: those are already covered directly by
// TestRecoveryMissingTokenSearchStaysMemoryBoundedProductionRoute/CompactRoute
// above, with tighter, witness-specific bounds.
func curatedRecoveryStressFixtures() []curatedRecoveryStressFixture {
	return []curatedRecoveryStressFixture{
		{name: "go-truncated-func", lang: grammarLanguageByName("go"), source: []byte("package p\nfunc f(x int) {\nif x {\n")},
		{name: "go-stray-token", lang: grammarLanguageByName("go"), source: []byte("package p\nvar x = ) + (\n")},
		{name: "javascript-unbalanced", lang: grammarLanguageByName("javascript"), source: []byte("function f(a, b) { return a +\n")},
		{name: "javascript-jsdoc-broken-comment", lang: grammarLanguageByName("javascript"), source: []byte("/**\n * @param {string} name\n * #  @returns {number}\n */\nfunction f(name) {}\n")},
		{name: "erlang-garbage", lang: grammarLanguageByName("erlang"), source: []byte("-module(x).\n-export([f/0]).\nf() -> ) ( .\n")},
		{name: "python-dedent-mismatch", lang: grammarLanguageByName("python"), source: []byte("def f():\n    if True:\n  x = 1\n")},
		{name: "json-trailing-comma", lang: grammarLanguageByName("json"), source: []byte("{\"a\": 1, \"b\": [1, 2, ,]}")},
		{name: "c-unbalanced-brace", lang: grammarLanguageByName("c"), source: []byte("int f(int x) {\n  if (x) {\n    return 1;\n")},
	}
}

func grammarLanguageByName(name string) func() *gotreesitter.Language {
	return func() *gotreesitter.Language {
		entry := grammars.DetectLanguageByName(name)
		if entry == nil {
			return nil
		}
		return entry.Language()
	}
}
