# Q2 retry work budget

Issue [#1306](https://github.com/odvcencio/gotreesitter/issues/1306) is implemented
by `3af9ec2d82f33ef23a08922461ae31d6293ab001` and
`60ea58440794f7c3aa6fd4557405b6f72da73d4c`, based on
`8a78a7673e502b9d3eb1ae7b7f99328b951b3976`.

The pinned 75,688-byte Go parser cliff previously ran six full parse attempts.
It now runs two and returns the same selected tree. A 179-byte reduced witness
reproduces the old ladder and remains a cheap-input control. A separate
3,435-byte fixture still takes its required merge retry and matches the locked
C runtime's fresh tree exactly.

The first fresh pass grants two token-count × peak-stack-count work units when
its work reaches 65,536 token-stack units. Cheaper first passes retain the full
ladder: wide merge recovery can be necessary even when narrow retries fail.
Each completed retry debits its token-count × peak-stack-count work. An admitted
pass can exceed the remaining allowance; it then exhausts the ladder. A first
pass at its resolved stack cap grants no retry work. Saturating arithmetic
prevents overflow from renewing the allowance.

The existing 24-pass limit also belongs to the whole operation. Nested calls
and recovery snippet parsers share it. Independent operations reset it, and
returning a snippet parser to its pool clears its borrowed quota. The hot
`Parser` layout is unchanged: the quota lives in its lazy sidecar.

Only the exact Go artifact
`df63fc35604c4e4e7a484abde9eb2110b61640045601c23991723f323a48310d`
enables work budgeting. Explicit stack or merge overrides retain the caller's
existing policy. Reused incremental passes do not seed a work allowance: doing
so reduced Go reuse splices from 294 to 115 and failed the counter ledger.
That candidate was rejected. The existing pass limit still applies to
incremental fallbacks.

Budgeting cheap first passes was rejected after the excluded-file audit found
`go/build/constraint/expr.go`. Its 3,188 tokens and twelve peak stacks cost only
38,256 token-stack units. Reducing its six attempts to two returned an error
tree with digest
`0f1b483e…`; the original ladder and fresh C return the same clean tree
`2c8c6bd7…` after the final wide merge. The correction keeps cheap recovery
while retaining the costly-cliff saving. The fixture includes the upstream
BSD license alongside its source.

Skipping every accepted-error retry was also rejected: the cliff's selected
digest changed from `d68083e7…` to `3cf54cb8…` without a correctness proof.

## Deterministic work

Instrumentation uses `gts_workcount`. The counters below include all attempts,
not just the selected attempt.

| Pinned Go cliff | Before | After |
| --- | ---: | ---: |
| Full attempts | 6 | 2 |
| Retry passes | 5 | 1 |
| Shifts | 748,193 | 248,331 |
| Reductions | 823,889 | 273,337 |
| Lexer calls (proxy) | 104,766 | 34,922 |
| Parent constructions (proxy) | 449,391 | 149,072 |
| Leaf constructions (proxy) | 749,522 | 248,774 |

Both selected-tree digests are
`d68083e7d0b99b493f4c78faceb11c191a5edf6bdba34a379df99eb67e10816e`.
The 15,254-byte build-constraint regression remains a clean six-attempt parse
with digest `2c8c6bd7c59f189454f6b6902d2fc13b721dee3caffb03d9e42a94966dcee8fc`.
The required merge fixture remains
`b63efd2aca1fcb7655cd7b6c30c6c76e8a30fcda3fe56896eca73630ca2c4b36`.
The straight-LR control remains one attempt with digest
`e229ccb60d58e50687a55f87b6f2520924091f143096193f5c572a00770cdd6b`.

The 2% counter ledger passes all 412 rows across 206 grammars. Work did not
increase and reuse did not fall. The checked-in ledger and other existing pins
are unchanged; its SHA-256 is
`057cba93bd791d995f814e404516c1a24b232f2b8450fa3cb4546dae337c6615`.
[Full counter captures](receipts/q2-retry-budget/counter-comparison.json) from
the original and corrected engines are byte-for-byte identical, SHA-256
`cc32935db65906abdae75112c6df065a7607a25a3606f21de368497ad69104ce`.
The [measured table](receipts/q2-retry-budget/counters.json.gz) is a separate
receipt and does not replace the checked-in pins.

## Locked Go corpus

[The compressed receipt](receipts/q2-retry-budget/go-corpus.json.gz) records
11,397 files and 98,302,153 bytes. It authenticates the clean implementation
revision, pure-Go child, exact grammar, corpus lock, source digests, selected
trees, attempt sequences, and locked static C oracle. Parse order alternates:
5,699 files run baseline first and 5,698 run candidate first. Local paths are
replaced with environment variable names in the published copy; use a newly
generated journal for resume.

| Corpus diagnostic | Before | After |
| --- | ---: | ---: |
| Full attempts | 12,085 | 12,071 |
| Recorded parse time | 222.419 s | 217.201 s |
| Allocated bytes | 143,177,413,552 | 141,132,523,176 |
| Exact C matches | 10,437 | 10,437 |
| Existing C mismatches | 960 | 960 |
| C crashes or unavailable results | 0 | 0 |

The budget eliminates 14 attempts on eight files. Every selected tree and C parity
result is unchanged. All 11,250 files whose selected Go tree is clean retain
their attempt sequence. Corpus wall times use instrumentation and are diagnostic;
randomized benchmarks below supply performance evidence.

The corpus lock SHA-256 is
`41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`.
The selected source manifest SHA-256 is
`b45df134a15d17b217413d2e4726ad70fb35a0f020ee126ebef52e837b3eb1d9`.
The Go source revision is `d00c67f297ef6f2cb2cd0e9aae59fa3936bb7eca`.
The standard corpus walker excludes directories named `build`, which excludes
forty additional Go files. Their [separate audit](receipts/q2-retry-budget/excluded-build-audit.json)
passes on the final authenticated child: all forty selected trees are unchanged,
with 36 exact C matches and four existing mismatches. The late wide-merge
regression belongs to these forty files.

The reference is fresh C from runtime `0.25.1`, commit
`f5afe475deb7c0bae6407fb776c76824f717bb61`, and Go grammar commit
`2346a3ab1bb3857b48b29d779a1ef9799a248cd7`. The static oracle artifact SHA-256 is
`1781b277bc48e9da928cd8ecc8a0fb9d666cf61ef41f9d1040745ade3529cd07`.

The pinned cliff remains an existing C mismatch. C returns a clean tree with
digest `9ed06ed630d8e18a6f191271d9da9339765ad1bd8ca15df2829d9b58cf3cd0f1`;
Go returns the same error tree before and after. Both roots span `0..75688`.
This change does not graduate Go or resolve the existing C mismatches.

## Reproduction

Use `GOWORK=off`, the Go 1.25 Docker harness image, and one grammar per process.
Run correctness and deterministic counters before timing. Reuse persistent
containers for the commands below when container launches are limited.

```sh
GOWORK=off go test ./internal/retrybudget ./grammars/runtime -count=1
GOWORK=off go test . -run 'Test(NestedParsePreservesRetryPasses|RecoverySnippetSharesRetryBudget|InheritedRetryBudgetChargesParentAndRestoresPool|StackCapSkipsEveryRetryRung)' -count=1
GOWORK=off go test ./grammars -run '^TestGoRetryBudget' -count=1
GOWORK=off go test . -tags gts_workcount -run '^TestDiagnosticRetryBudgetCliffCounters$' -count=1 -v
GOWORK=off go run -tags gts_workcount ./cmd/perfcounterledger
GOWORK=off GOTREESITTER_V1_INVARIANT_LANGUAGE=go go test . -run '^TestV1InvariantGateR4EditSession$' -count=1 -v
GOWORK=off go test ./... -count=1 -timeout=60m
```

For corpus certification, use a clean checkout with a `.git` directory. The
container's Go toolchain does not stamp VCS metadata for a worktree `.git` file.
The certification gate remains enabled and rejects unstamped or dirty children.

```sh
cd cgo_harness
GOWORK=off \
GTS_RETRY_PROFILE_CERT=1 \
GTS_RETRY_PROFILE_CERT_LANG=go \
GTS_RETRY_PROFILE_CERT_MODE=first_pass_work_budget \
GTS_REAL_CORPUS_BENCH_ROOT="$CORPUS_ROOT" \
GTS_REAL_CORPUS_BENCH_LOCK="$CORPUS_LOCK" \
GTS_REAL_CORPUS_BENCH_LOCK_FILTER=1 \
GTS_C_ORACLE_CACHE="$C_ORACLE_CACHE" \
GTS_RETRY_PROFILE_CERT_OUT="$RECEIPT_OUT" \
go test . -buildvcs=true -tags treesitter_c_parity,treesitter_c_perfscan,gts_workcount \
  -run '^TestRetryProfileCorpusCertification$' -count=1 -timeout=45m -v
```

## Invariant and correctness results

The 72-step Go R4 session passes with zero allocations for a reparse without
edits. The full 75,688-byte cliff also passes all 72 steps on the legacy route:
incremental and fresh Go digests agree at every step, ERROR roots report
`HasError`, accepted roots cover the input, and no-edit reparsing allocates
nothing. That run took 79.43 seconds and peaked at 1,130,752 KiB RSS.
To reproduce it, place [the standalone invariant probe](receipts/q2-retry-budget/cliff-invariant-probe.go.txt)
in a temporary command directory under the checkout and pass the decompressed
Go cliff fixture as its first argument. Run it inside Docker with `GOWORK=off`.

[The 206-grammar sweep](receipts/q2-retry-budget/invariants.json) passes 198
sessions. [Baseline replay](receipts/q2-retry-budget/invariant-baseline-failures.json)
reproduces all eight remaining failures. AWK, JavaScript, Meson, and Twig have
identical failing-step digest pairs before and after. Elsa, Haskell, Kotlin,
and PowerShell exceed the same three-minute session timeout in edit-dependency
traversal. These failures remain; no allowlist or expectation was changed.

The original engine was also replayed on every one of the 514 locked corpus
files that runs more than one full attempt. All selected trees are unchanged.
The replay uses the unchanged original parser and grammar with a
[CLI-only receipt shim](receipts/q2-retry-budget/origin-cli-shim.patch.gz) to emit
one parse. Its binary SHA-256 is
`bc097e7750ec466872c0b4aa64dc6e1c3bf6d1e76c0673b737fa94eeaa745991`.
This supplements the clean-revision budget-switch certification above.

Focused race tests pass for the operation quota, snippet inheritance, pool
cleanup, stack ceiling, saturation, and first-pass classification.

Fresh, incremental, and error-status C smoke tests pass individually for Go,
C, C++, C#, Rust, PHP, Scala, Swift, SQL, and HTML. The Go GLR canary and both
clean merge-recovery fixtures also match fresh C. These smoke tests cover
operation-wide quota changes; the full artifact certificate covers Go's work
policy.

The requested root `GOWORK=off go test ./... -count=1 -timeout=60m` ran to
completion inside Docker: 34:09.97 elapsed and 5,254,124 KiB peak RSS. The root
parser, grammar, and internal packages pass. Four grammar-generator tests fail:
`TestImportedKotlinSwiftGrammarConstructors`,
`TestMarkdownGrammarCommonMarkParity`,
`TestMarkdownFencedCodeBlockContentParity`, and
`TestShippedBlobsNonTerminalAliasMapInventory`.
[Focused baseline replay](receipts/q2-retry-budget/root-test-baseline-replay.json)
reproduces the entire failure output exactly after removing elapsed times.
The broad command exits with status 1; these pre-existing failures remain.

## Randomized performance

Twenty paired shuffle seeds ran on an Intel Xeon Platinum 8481C with Go
1.25.14. [Raw measurements](receipts/q2-retry-budget/cliff-before.txt) and
[their comparison](receipts/q2-retry-budget/cliff-benchstat.txt) show:

| Legacy Go cliff | Before | After | Change |
| --- | ---: | ---: | ---: |
| Time/op | 1.30038 s | 0.431023 s | -66.85% |
| B/op | 228,501,416 | 48,939,832 | -78.58% |
| Allocs/op | 18,150 | 5,845 | -67.80% |

All three cliff changes have `p < 0.001`, with twenty samples per revision.

| Primary Go benchmark | Before | After | Time comparison |
| --- | ---: | ---: | --- |
| Full DFA | 8.76173 ms | 8.74651 ms | unchanged (`p=0.063`) |
| Single-byte edit DFA | 185.703 µs | 186.237 µs | +0.29% (`p=0.003`) |
| No-edit DFA | 9.5575 ns | 8.7075 ns | -8.89% (`p<0.001`) |

The [trio comparison](receipts/q2-retry-budget/trio-benchstat.txt) keeps edit
allocations at 388 B/op and five allocations, and no-edit at zero bytes and
zero allocations. Full parse changes from 1,277 to 1,275 B/op and retains eight
allocations. The 0.29% edit-time increase is the recorded tradeoff for bounding
costly retry work and sharing the operation quota. It stays within the unchanged
2% gate; all deterministic work and reuse gates pass.

Benchmark headers mark both checkouts dirty because the candidate has new
documentation and the original checkout has only the added cliff benchmark.
The parser code used for timing is exactly the implementation and baseline
revisions listed above. Benchmark headers are informational, as stated by the
runner; the corpus receipt separately authenticates a clean revision.

The [native Go/C receipt](receipts/q2-retry-budget/ratios.json) uses the original
engine binary before and the corrected implementation after, with the same
locked static C artifact throughout. These are warmed, parse-only measurements,
separate from the Go benchmark's parse-and-release loop.

| Native cliff measurement | Before | After |
| --- | ---: | ---: |
| Go median parse time | 1.21222 s | 0.394212 s |
| C median parse time | 11.4584 ms | 11.4653 ms |
| Median paired Go/C time ratio | 105.91× | 34.38× |
| Go maximum process RSS | 325,868 KiB | 216,760 KiB |
| C maximum process RSS | 3,316 KiB | 3,316 KiB |
| Maximum-RSS Go/C ratio | 98.27× | 65.37× |

The RSS ratio divides the largest Go process peak by the largest C process peak
across twenty seeds. It includes grammar and runtime setup, unlike the parse
timer. The reduction does not establish a 2× Go/C target or fresh-C tree parity
for this existing cliff mismatch.

## Randomized performance reproduction

The benchmark comparison uses twenty paired shuffle seeds, one process per
seed, `GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`, and `-benchmem`. Copy only
`grammars/go_retry_budget_bench_test.go` to the original checkout so it exposes
the cliff benchmark without changing its parser. Run both commands inside the
same Docker image after correctness jobs finish:

```sh
GOWORK=off bash scripts/run_randomized_benchmarks.sh \
  --output "$RECEIPT_DIR/trio-after.txt" \
  --baseline-root "$BASELINE_ROOT" --baseline-output "$RECEIPT_DIR/trio-before.txt" \
  --runs 20 --benchtime 750ms --tags gts_parsercorephase0 \
  --package . --bench-regex '^BenchmarkGoParse(FullDFA|IncrementalSingleByteEditDFA|IncrementalNoEditDFA)$' \
  --require-benchmarks BenchmarkGoParseFullDFA,BenchmarkGoParseIncrementalSingleByteEditDFA,BenchmarkGoParseIncrementalNoEditDFA
GOWORK=off bash scripts/run_randomized_benchmarks.sh \
  --output "$RECEIPT_DIR/cliff-after.txt" \
  --baseline-root "$BASELINE_ROOT" --baseline-output "$RECEIPT_DIR/cliff-before.txt" \
  --runs 20 --benchtime 750ms --tags gts_parsercorephase0 \
  --package ./grammars --bench-regex '^BenchmarkGoRetryBudgetCliff$' \
  --require-benchmarks BenchmarkGoRetryBudgetCliff
benchstat "$RECEIPT_DIR/trio-before.txt" "$RECEIPT_DIR/trio-after.txt"
benchstat "$RECEIPT_DIR/cliff-before.txt" "$RECEIPT_DIR/cliff-after.txt"
```

For native Go/C ratios, build the [standalone Go probe](receipts/q2-retry-budget/memory-probe.go.txt)
as a temporary `.go` file in the candidate checkout. Build a second copy in the
original checkout after removing only the assignment to
`lang.FullParseRetryWorkBudgetEnabled`, which does not exist there.
Set `GO_RETRY_PROBE`, `GO_RETRY_BASELINE_PROBE`, `GO_CLIFF_INPUT`,
`C_ORACLE_ARTIFACT`, and `Q2_RECEIPT_DIR`, then run with
`GOWORK=off GOMAXPROCS=1 python3`
[the ratio driver](receipts/q2-retry-budget/measure-ratios.py).
It alternates Go-C-C-Go and C-Go-Go-C windows across twenty seeds, shuffles
budget settings, warms once, and measures three fresh parses per process.
Tree deletion runs outside each timed parse. Process peak RSS comes from
`/usr/bin/time -v`; it includes runtime and grammar setup. C allocation counts
are not available from this native oracle, so Go allocation counts are reported
separately rather than presented as a Go/C allocation ratio.

## Run assumptions and remaining work

The existing digest-verified corpus lock defines the C reference for this run,
including runtime 0.25.1. Two persistent Docker containers handled all heavy
commands. The shared Docker lock remained occupied by another long-running
lane, so this lane used its own lock and serialized its workloads. Correctness
finished before randomized timing began.

Work budgeting is certified and enabled only for the recorded Go artifact.
Other artifacts retain their retry-work policy; all artifacts share the repaired
operation-wide pass quota. The eight existing invariant failures, four existing
grammar-generator test failures, and existing C mismatches remain separate work.
No pins, allowlists, or gate thresholds changed. Integration and issue closure
belong to the lead; the owner's autonomy instruction requests a pushed branch
without opening a PR.
