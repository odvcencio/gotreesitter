# AWK incremental insertion: issue #1358

The reported insertion now produces the same complete tree as fresh Go and
the locked C runtime. The fix preserves subtree reuse and adds no grammar-name
comparisons. The corrected counter row requires owner review before integration.

The baseline is `2e93c61b4ed5ef32d82eb07561e709960e2b478f`, the fetched
`origin/main` when this investigation began. No open pull request covered
[#1358](https://github.com/odvcencio/gotreesitter/issues/1358). The engine and
regressions are in `d4842aa1934a5ccc35aa6ea489c490ad67234e9e`; the separate
pin refresh is `d836f232a4666d9e669bed855cf1afb30519c7ce`. The follow-up
`591f74eb68a1372cc4c2872f5ce399d868b5c461` distinguishes a proven leaf replay
from uncertain top-level ownership, preserving the existing fresh verifier for
the latter.

The 579-byte reported input is the checked-in AWK real fixture. Its SHA-256 is
`9e2a236e15d67f3d9397ee99f101be4a3289f76128f1de60fd90dce4c151a944`.
Insert `x` at byte 438, row 20, column 42. The 72-step R4 session SHA-256 is
`f12685110d0d0bda18597feb4d13f33a307bf20a6a9ed3b82a61462be8f6aea4`.
The fetched corpus lock matched the mounted corpus lock and its expected
SHA-256, `41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`.
The lock was kept outside the repository.

A readable 23-byte witness reproduces the same bug on the baseline:

```awk
{ a[(1),(2),""]=value }
```

Insert `x` at byte 18, producing `{ a[(1),(2),""]=vxalue }`. The reused
quoted leaf occurs before the edit. Fresh parsing reduces the hidden expression
list before shifting that leaf. Incremental reuse previously accepted a matching
shift state without performing that reduction. Eager default reductions can
expose the conflict immediately before the second reuse attempt, so checking
only the first attempt leaves the bug intact.

The parser now checks ownership at both reuse attempts. A read-only proof in
`internal/incr` follows certified reductions to the leaf's recorded pre-goto
state. The proof allocates nothing and rejects ambiguous ancestry, extras,
missing gotos, and bounded cycles. The existing dispatcher performs the actual
reductions. A proven leaf replay follows normal dispatch and does not require
the extra fresh parse used to verify uncertain top-level ownership. The regression also
checks that its profile does not claim an unproven top-level mismatch.

The new leaf proof applies to external scanners without checkpoint
support; checkpointed scanners already authenticate this state. Existing
DFA-only reuse behavior is preserved.

The C reference is runtime `0.25.1` at
`f5afe475deb7c0bae6407fb776c76824f717bb61`, with Go binding `v0.25.0` at
`adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3`. The AWK grammar is
[`Beaglefoot/tree-sitter-awk`](https://github.com/Beaglefoot/tree-sitter-awk)
at `34bbdc7cce8e803096f47b625979e34c1be38127`; its C artifact SHA-256 is
`460ddc8d1bf00f5ebbf61d1a8e4d83692e311218592adf52b453e2bdbd039784`.
The oracle compares fresh C trees, including root spans, symbols, fields,
points, missing/error flags, and the canonical deep digest. The C incremental
tree used for edit timing also matches fresh C exactly on the reported input,
with the corrected digest below, no errors, and a 580-byte root. Standalone
pristine-runtime dumps also match all four initial/edited witness digests.

For final timing, both worktrees use the same external binding replacement with
all runtime sources copied from the pristine locked upstream commit. The
released binding omits one reset assignment in `parser.c`; its module checksum
verified, and both versions produce the same fresh trees here. The receipt
records the exact upstream source hashes. The final RSS helper compiles that
pristine runtime directly without a Go binding.

| Reported insertion | Baseline | Fixed / fresh Go / fresh C |
| --- | --- | --- |
| Deep digest | `e549aa0ec096e67ad30e1a209fc4dbd1b26cd92ca352d1cda03a45f88d4d6ce0` | `0b8fd6b6c468a255f7308ecad3f0a94d72458df3c6a604284f4d53fbf339aa44` |
| HasError | true | false |
| Root end / input bytes | 580 / 580 | 580 / 580 |
| Stop | accepted | accepted |

The shrunk edited witness has fresh Go, incremental Go, and fresh C digest
`b8e3f3d06be55b5abd49247dea00848397bb3e5a144717e5a7f9e431b1971493`.
Both admission routes and both normal/profiled incremental APIs match.

Deterministic counters ran before randomized timing. Only four of the 412
language/route rows changed, all in the guarded direction. The strict 2% ledger
gate passes for all 206 languages. No threshold changed.

| AWK default-route first insertion | Baseline | Fixed |
| --- | ---: | ---: |
| Tokens | 114 | 86 |
| New nodes | 407 | 284 |
| Maximum live versions | 5 | 1 |
| Multi-version token share, ppm | 166,666 | 0 |
| Reused bytes | 314 | 363 |
| Block splices | 85 | 108 |
| No-edit allocations | 0 | 0 |

The other changes are LESS new nodes 1,373 → 1,370; Move new nodes 1,092 →
1,089; and WGSL tokens 184 → 183, nodes 336 → 329, reused bytes 1,521 →
1,522, block splices 86 → 87, and multi-version token share 21,739 →
16,393 ppm. Every full-parse ledger row and every candidate-route counter remains
unchanged.

Broader candidates were rejected before choosing this fix:

| Candidate | Counter rows changed | Guarded metric failures | Reason rejected |
| --- | ---: | ---: | --- |
| Reject any conflicting matching leaf shift | 83 | 245 | Broke established reuse trajectories. |
| Preemptively replay every conflicting frontier | 54 | 58 | Forced unrelated reductions. |
| Restrict replay to certified repetition conflicts | 53 | 56 | Still preempted successful reuse. |
| Replay only after reuse fails | 53 | 56 | Did not distinguish unreachable leaf ownership. |
| Ignore unknown recorded frontiers | 53 | 56 | Same counter failures. |
| Respect existing repetition exceptions | 52 | 52 | Insufficient reachability proof. |
| External-scanner bridge without exact proof | 19 | 21 | Perturbed other scanner grammars. |
| One-step proof at the first attempt | 14 | 2 | AWK reuse fell from 314 bytes to zero; missed the retry. |
| One-step proof including linear GSS ancestry | 14 | 2 | Still missed the retry. |
| Bounded multi-reduction proof at the first attempt | 12 | 2 | Still missed the retry. |
| Proof at both attempts for every leaf class | 13 | 1 | Chatito block splices fell 114 → 44 despite other improvements. |
| Scanner bridge, proof at both attempts, counted as uncertain ownership | 4 | 0 | Correct trees, but an unnecessary fresh verifier regressed insertion time by 42.2% and allocations by 114.9%. |
| Same proof, distinguish proven leaf ownership | 4 | 0 | Selected; preserves the counter rows without invoking that verifier. |

Earlier shift-only restrictions also increased JSON tokens 75 → 97 and nodes
331 → 353 while reducing its block splices 276 → 254. The broadest restriction
increased Go nodes 3,804 → 3,899. Those candidates were not committed.

All heavy validation used one persistent Docker container with image
`gotreesitter/cgo-harness:go1.25-local`, Go `1.25.14`, `GOWORK=off`,
`GOMAXPROCS=1`, an 8 GiB container limit, `GOMEMLIMIT=6GiB`, and `GOFLAGS=-p=1`.
The image SHA-256 is
`cd86a0f13610081cbce2c079f3c9fb19dbb4531452ce0f47186c19b1b98995dd`.

| Correctness check | Result |
| --- | --- |
| Generic frontier and existing reuse/settle unit tests | Pass; proof allocates zero times. |
| AWK insertion, replacement, and undo regressions | Pass for two fixtures, two routes, and both APIs; positive reuse and zero no-edit allocations. |
| AWK R4 invariant gate | All 72 steps pass on both routes. |
| Exact locked-C AWK insertion regressions | Pass on both fixtures, routes, and APIs. |
| Standard fresh/incremental/error/highlight parity | Pass for AWK, JavaScript, LESS, Move, and WGSL, one grammar per process. |
| Locked AWK real files | All five digest-verified files match fresh C before and after the first insertion, including the 26,493-byte largest file. |
| All-language R4 audit | Baseline 389/412 pass; fixed 394/412 pass; no new failures. |
| Counter ledger | Pass: 412 rows, 206 languages. |
| Final root parser package `GOWORK=off go test -timeout=25m .` | Pass after the proven-leaf follow-up. |
| Root `GOWORK=off go test -timeout=75m ./...` | Completed; four grammar-generation failures reproduce on baseline with identical assertions and failing subtests. All other packages pass. |

The complete root run tested `d836f232`; the subsequent proven-leaf follow-up
was checked with focused units, memory-budget and existing top-level ownership
tests, affected parity suites, the full counter ledger, and the R4 audit,
followed by the entire root parser package. The complete root run was not
repeated after that follow-up.

The complete root run took 50 minutes 41.66 seconds and peaked at 4,879,008 KiB
RSS without an OOM. Its four baseline failures are
`TestImportedKotlinSwiftGrammarConstructors` (306 Swift rules versus 299
expected), `TestMarkdownGrammarCommonMarkParity`,
`TestMarkdownFencedCodeBlockContentParity`, and
`TestShippedBlobsNonTerminalAliasMapInventory` (ten uncertified languages).
The JSON receipt records the matching baseline and fixed failure lists.
No expectation was changed for these failures. The first root run hit Go's
default ten-minute grammar-generation timeout and a missing Docker Git-history
setup; the completed rerun supplied the repository history and increased only
the execution timeout.

The R4 audit still has six baseline invariant failures: Meson and Twig on both
routes, plus candidate Djot and HTML. It also has twelve baseline timeouts:
Elsa, Godot Resource, Haskell, Kotlin, Nickel, and PowerShell on both routes.
AWK and JavaScript now pass both routes. Baseline F# timed out and the fixed
run passed; that timing variation is not attributed to this fix.

An additional audit found exact fresh-C equality through AWK R4 step 7. Step 8
has a pre-existing malformed-input fresh Go/C difference: the block has 35 Go
children and 31 C children. The baseline reproduces the same difference.
Incremental equals fresh Go throughout all 72 steps; this insertion fix does
not claim to close that separate fresh recovery gap.

The complete 20-seed comparison uses the same pristine C runtime in both
worktrees. Each seed averages the two Go and two C samples; the table takes the
median across those seed pairs. Tree copying and cleanup are outside insertion
timing for both runtimes. `Tree.Edit` and incremental parsing are timed.

| Reported input | Go time before → after | Go/C before → after | Bytes/op before → after | Allocations/op before → after |
| --- | ---: | ---: | ---: | ---: |
| Full parse | 963.4 → 1,011.8 µs | 2.928 → 3.040 | 9,288 → 9,288 | 470 → 470 |
| Insertion | 1,249.9 → 675.9 µs | 20.087 → 11.098 | 12,449.5 → 1,939 | 242 → 39 |
| No edit | 21.69 → 22.01 ns | 0.0038322 → 0.0038328 | 0 → 0 | 0 → 0 |

Insertion improves 45.9% in Go time and 44.7% in Go/C, while errors/op changes
from one to zero. Full Go time moves up 5.0%, and full Go/C moves up 3.8%, below
the 10% ratio ratchet. `benchstat` finds no significant change in the first Go
full-parse position (`p=0.718`), but the second position increases 10.9%
(`p=0.002`). A separate 20-seed full-parse recheck follows below. No-edit timing
has no significant change, and its allocation count remains zero.

The insertion ratio is still above the design's 10× hard ceiling; the baseline
already exceeded it and produced the wrong tree. This fix does not claim AWK
performance graduation or change that ceiling. The full graduation matrix was
not run. The corrected insertion and unchanged full work counters are recorded
alongside the timing tradeoff.

The profiled attribution has 1,000 repetitions. Mean `Tree.Edit` time is
6.24 → 5.19 µs; reuse-cursor setup is 88.36 → 98.25 µs; reparse/rebuild is
1,129.28 → 554.43 µs. These diagnostic times include instrumentation and are
not comparison evidence. They identify reparse and materialization as the
remaining insertion cost. The first selected implementation's allocation
profile exposed the extra full forest verification; the follow-up removes that
unnecessary parse for proven leaves while retaining uncertain ownership checks.

The primary Go controls also completed 20 shuffled seed pairs. Full parsing is
15.20 → 15.16 ms (`p=0.989`), a single-byte edit is 312.4 → 313.3 µs
(`p=0.883`), and no edit is 14.71 → 15.03 ns (`p=0.753`). None has a
significant timing change. Allocations stay at eight, five, and zero respectively;
edit bytes/op stay at 391, and no-edit bytes/op stay at zero.

The 1 MiB memory fixture pads the shrunk witness with one AWK comment. Its
initial and edited canonical digests match pristine fresh C exactly; the edited
root covers all 1,048,577 bytes without errors. Twenty alternating process
samples per mode each perform ten parses. Every process completed without a
crash or OOM. RSS includes the runtime, initial tree, result trees, and source
buffers; native C has no Go binding.

| 1 MiB probe | Median Go RSS before → after, KiB | Median Go/C RSS before → after | Maximum Go RSS before → after, KiB |
| --- | ---: | ---: | ---: |
| Full parse | 36,148 → 36,656 | 8.628 → 8.910 | 41,168 → 42,640 |
| Insertion | 461,452 → 261,446 | 111.057 → 63.704 | 570,448 → 475,276 |
| No edit | 36,064 → 35,256 | 8.652 → 8.650 | 40,448 → 42,448 |

Insertion median RSS improves 43.3%; maximum RSS falls 16.7%, from 557.1 to
464.1 MiB. The maximum remains above the design's 400 bytes per source byte ceiling at 1 MiB. The baseline also
exceeds that ceiling. The lower median does not establish a memory-gate
pass. Full-parse median RSS rises 1.4%, and its Go/C RSS rises 3.3%; no-edit
median RSS falls 2.2%. Full and no-edit maxima rise 3.6% and 4.9%, respectively,
while staying below the 1 MiB ceiling. The receipt preserves all samples and
maxima. No parser memory-budget contract or gate threshold changed.

The independent full-parse recheck also completed 20 shuffled seed pairs.
Combined Go time is 1063.6 → 1044.2 µs, and Go/C is
3.027 → 3.024 (-0.1%). Both full-parse experiments are retained in
the receipts; the second run does not replace the first result. The recheck
finds no significant timing change in either Go position (`p=0.583` and
`p=0.904`). The full-parse increase is not consistent across the two
experiments, and the deterministic full work counters remain unchanged.

Run the focused checks inside the harness container with `GOWORK=off`:

```sh
go test ./internal/incr -count=1
go test . -run '^(TestLeafReuse|TestReuseTargetState|Test.*Reduce.*Reuse|TestSettleDeterministic|TestParser.*Conflict)' -count=1
go test ./grammars -run '^TestAWKIncrementalInsertMatchesFresh$' -count=1
GOTREESITTER_V1_INVARIANT_LANGUAGE=awk go test . -run '^TestV1InvariantGateR4EditSession$' -count=1
GTS_ADMISSION_CANDIDATE=1 GOTREESITTER_V1_INVARIANT_LANGUAGE=awk go test . -run '^TestV1InvariantGateR4EditSession$' -count=1
go run ./cmd/perfcounterledger
cd cgo_harness
go test -tags treesitter_c_parity . -run '^TestAWKIncrementalInsertLockedCFresh$' -count=1
GTS_PARITY_MODE=exhaustive go test -tags treesitter_c_parity . -run '^TestParity(FreshParse|IncrementalParse|HasNoErrors|Highlight)$/^awk$' -count=1
```

Repeat the final parity command one grammar per process for JavaScript, LESS,
Move, and WGSL. The complete R4 comparison repeats the invariant command for
each registered grammar and both routes, with an 80-second timeout per process;
the JSON receipt lists every nonzero result.
