# Bounded lexical proof memo, 2026-09-30

Repeated control edits now reuse completed lexical safety proofs. The final
20-seed paired campaign measures 197.660 us to 2.864 us per edit (-98.55%),
388 to 256 bytes, and five to two allocations. This increment follows
`38b4574a4`, which reduced mode enumeration and probe-copy overhead.

The memo holds two successful, directed comparisons. Each entry copies at most
512 bytes from each source version and retains no source buffers. Its window
includes the changed bytes, captured lookahead, whitespace and UTF-8 decoding
margins, and any previous-line bytes used to derive the starting point. The
key includes byte and point coordinates, input length, the captured read bound,
and leading BOM bytes. A hit compares that exact bounded window. It neither
infers a reverse proof nor walks lexer modes or untouched subtrees.

Every ordinary admission and source-sensitive semantic check still runs. Only
immutable loaded grammar tables can use the memo. Stateless external scanners
also need an exact, bounded snapshot of their receiver parameters. Unsupported
metadata, custom tables, wider windows, and explicit memory budgets use the
original proof. Failed proofs never enter the cache. The existing parser cold
sidecar owns the cache; the pinned Parser remains 2,328 bytes.

The initial memo candidate caused a repeatable no-edit timing increase:
8.825 to 9.704 ns (+9.96%) in a separate 20-seed campaign. The unchanged-source
assembly was otherwise identical. The final implementation adds an inlineable
same-buffer predicate to the public incremental entry point and retains the
original copied-buffer and included-range checks. The final paired campaign
improves no-edit latency and still allocates nothing.

## Final control campaign

Baseline: `f9828512c`. Engine revision: `c119366c0`. Candidate code is also identified by the SHA-256 manifest in
[the receipt](incremental-verify-memo-receipt.json). Docker runs used the
randomized wrapper, alternating baseline/head order, seeds 201–220,
`GOWORK=off`, `GOMAXPROCS=1`, one process per seed, `-count=1`, 750 ms, and
`-benchmem`. One CPU was pinned on a shared local Xeon Platinum 8481C host.
The requested dedicated runner could not be started because cloud credentials
lacked the required authorization scope. These are local smoke results; they
do not reproduce the owner's unpublished real-code inputs or establish a
quiet-host release receipt.

| Control workload | Before | After | Change |
| --- | ---: | ---: | ---: |
| Go single-byte edit | 197.660 us | 2.864 us | -98.55% |
| Go full parse | 8.850 ms | 8.742 ms | -1.21% |
| Go no-edit reparse | 8.764 ns | 7.731 ns | -11.78% |
| Edit bytes allocated | 388 B | 256 B | -34.02% |
| Edit allocations | 5 | 2 | -60% |
| Full-parse allocations | 8 | 8 | unchanged |
| No-edit allocations | 0 | 0 | unchanged |

All three timing differences have p < 0.001. Full parsing does not execute
the memo, so its small directional improvement is not attributed to the cache.
Raw records: [before](benchmarks/verify-memo-inline-trio-before.txt) and
[after](benchmarks/verify-memo-inline-trio-after.txt).

The corrected numeric campaign measures Go 176.351 to 2.141 us
(-98.79%). C is 755.515 to 754.207 us, p = 0.052. The median
paired Go/C ratio is 0.232598x to 0.002844x. This
many-function synthetic case already favors Go over C; it is not the owner's
real-code ratio. Raw records: [before](benchmarks/verify-final-numeric-before.txt),
[after](benchmarks/verify-final-numeric-after.txt).

The original numeric and large-workload sets kept subbenchmark order fixed.
Their raw records remain for the audit and are superseded as comparison evidence.
Harness `7f92332e9` applies each explicit seed to size and Go/C subbenchmarks.
All corrected sets use seeds 401–420; before/head permutations match exactly and
include both Go/C orders. They retain the control campaign's stable settings.

METRIC: go numeric edit Go/C | 0.232598x -> 0.002844x | f9828512c -> c119366c0, harness 7f92332e9 | 19458 B numeric replacement

## Size scaling

Every size passes six steps against fresh Go and locked C on both routes, plus
coverage/error and zero-allocation no-edit checks. First proofs use 1,388 raw
lexer calls; warmed steps use one, allocate no nodes, and reuse all input bytes.

| Input | Go before | Go after | C before | C after |
| --- | ---: | ---: | ---: | ---: |
| 32,779 B | 177.059 us | 2.072 us | 1.278 ms | 1.278 ms |
| 140,304 B | 177.035 us | 2.076 us | 5.553 ms | 5.573 ms |
| 1,048,596 B | 179.008 us | 2.159 us | 74.018 ms | 73.090 ms |

Go improves 98.79–98.83%, p < 0.001. At every size, Go bytes fall 384 to 256 and
allocations fall five to two. C allocations are unchanged. C at 137 KiB rises
0.36%, p = 0.008; other C size changes are not significant. C uses the same
runtime and input, so that small directional increase is not attributed to Go.
Raw records: [before](benchmarks/verify-final-sizes-before.txt),
[after](benchmarks/verify-final-sizes-after.txt). These flat many-function trees
make C rebuild a wide root; their favorable ratios do not predict real-code edits.

METRIC: go numeric 32KiB edit Go/C | 0.13862924x -> 0.00161951x | f9828512c -> c119366c0, harness 7f92332e9 | repeated functions

METRIC: go numeric 137KiB edit Go/C | 0.03189657x -> 0.00037191x | f9828512c -> c119366c0, harness 7f92332e9 | repeated functions

METRIC: go numeric 1MiB edit Go/C | 0.00241433x -> 0.00002867x | f9828512c -> c119366c0, harness 7f92332e9 | repeated functions

## Deterministic work and correctness

The 19,458-byte Go witness alternates a numeric byte six times. At every step,
both legacy and compact routes equal fresh Go and locked C by the full deep
digest. Root coverage, ERROR-root flags, and zero-allocation no-edit reparsing
also pass.

| Counter | Baseline, every step | Candidate, first two | Candidate, warmed steps |
| --- | ---: | ---: | ---: |
| Raw lexer calls | 1,388 | 1,388 | 1 |
| Parse attempts | 0 | 0 | 0 |
| Shifts / reductions / action lookups | 0 / 0 / 0 | 0 / 0 / 0 | 0 / 0 / 0 |
| New nodes | 0 | 0 | 0 |
| Reused bytes | 19,458 | 19,458 | 19,458 |
| Dependency checks | 1 | 1 | 1 |

The raw lexer counter matters: the front-door lexer counter is zero even when
the original safety proof performs 1,388 private DFA scans.

Focused Docker tests pass for proof budgets, keyword and whitespace dependencies,
scanner identity changes, compact reuse, edit coordinates, included ranges,
memory budgets, both cold-sidecar builds, and pinned hot layouts. Focused race
tests pass. A 60-second fuzz run compared the memo with the original proof over
64,785 executions, including changed windows, BOMs, Unicode, and starting points.
The complete deterministic ledger passes 412 rows for 206 grammars. No pin,
expectation, threshold, allowlist, or default route changed.

The original and memoized 206-language edit-session sweeps have identical
statuses: 198 pass; awk, JavaScript, meson, and twig have the same digest
mismatches; elsa, Haskell, Kotlin, and PowerShell time out after ten minutes.
All eight failures reproduce on the unmodified baseline. These are outstanding
gate failures, not graduation evidence. The final no-edit predicate is also
covered by the focused incremental, included-range, and allocation tests.

## Requested fallback workloads

The corrected generated witnesses use the inputs documented in
[the earlier receipt](incremental-verify-cost.md). These paths do not use the memo:

| Input / edit | Go before | Go after | C before | C after |
| --- | ---: | ---: | ---: | ---: |
| PowerShell 15,390 B EOF | 34.659 ms | 34.651 ms | 0.572 ms | 0.571 ms |
| Go 140,368 B identifier | 144.038 ms | 144.099 ms | 1.585 ms | 1.585 ms |
| C# 1,048,645 B identifier | 2.702 s | 2.705 s | 24.508 ms | 23.652 ms |

All Go changes are statistically insignificant. PowerShell C improves 0.23%,
p = 0.049; other C changes are not significant. Allocation and reuse counters
stay unchanged. PowerShell uses `eof_append_fresh`; C# uses
`external_scanner_unsupported`; both reuse zero bytes. C# also passes all four
fresh-Go/C comparisons with compact admission enabled, with the same fallback.
The neighboring lane owns retiring Go's legacy fresh verifier. Raw records are
`benchmarks/verify-final-{powershell,go,c_sharp}-{before,after}.txt`.

METRIC: powershell edit Go/C | 60.5233x -> 60.6948x | f9828512c -> c119366c0, harness 7f92332e9 | generated matched input

METRIC: go edit Go/C | 90.8212x -> 90.6404x | f9828512c -> c119366c0, harness 7f92332e9 | generated matched input

METRIC: c_sharp edit Go/C | 109.1917x -> 114.7710x | f9828512c -> c119366c0, harness 7f92332e9 | generated matched input

The C# already-built benchmark process measured peak RSS of 696,792 to 697,668
KiB (+0.126%) under `/usr/bin/time -v`. It includes grammar loading and initial
parsing and excludes compilation. Both one-edit runs allocate 398,943,896 bytes
in 172,808 allocations. That preliminary pair is one RSS run per revision, not a timing campaign
or an RSS median. Both process peaks exceed the existing 400-byte-per-input-byte
gate. No threshold was changed to accept the fallback. Raw RSS records:
[before](benchmarks/verify-rss-csharp-before.txt),
[after](benchmarks/verify-rss-csharp-after.txt).

A second single RSS pair measured 697,876 to 807,388 KiB (+15.69%). Five
additional alternating process pairs resolve the apparent regression: baseline
peaks are 796,536 / 802,884 / 794,244 / 682,332 / 803,080 KiB; candidate peaks
are 794,364 / 793,376 / 682,268 / 681,308 / 793,752 KiB. Median peak RSS is
796,536 to 793,376 KiB (-0.40%); the maximum is 803,080 to 794,364 KiB (-1.09%).
All operations allocate the same 398,943,896 bytes in 172,808 allocations. The
process peaks vary substantially, so the single outlier is retained rather than
used as evidence of persistent growth. Both versions still exceed the absolute
RSS ceiling. The receipt includes every pair, with raw records under
`benchmarks/verify-rss-repeat-*.txt`.

## Attribution and final-byte site

Single 1 MiB CPU profiles classify complete sampled stacks under Tree.Edit and
ParseIncremental, excluding initial parsing, release, and background runtime.
They guide attribution and do not serve as timing comparisons.

| Operation CPU share | Before | After |
| --- | ---: | ---: |
| Tree.Edit | 0.27% | 19.32% |
| Primitive verification | 99.00% | 14.24% |
| Reuse selection and entry | 0.74% | 66.44% |
| Reparse/rebuild | 0% | 0% |

A final-byte site at byte 1,048,592 passes six steps on both routes against
fresh Go and C. Warmed steps use one raw lexer call, zero new nodes, full reuse,
and no rebuild. Its profiled reuse takes 27–30 us, not comparison evidence.
The engine still contains whole-source bytes.Equal in reuseCursor.reset.
Source-consistency and leaf-admission costs need further attribution before an
O(edit) bound can be claimed for the whole operation.

## Scope and coordination

A memo hit bounds primitive-proof work by the retained dependency window.
Novel edits and uncacheable windows run the original bounded lexical proof.
This increment does not establish an O(edit) bound for every parse operation,
retire fresh fallback verification, or solve PowerShell EOF/C# scanner admission.
Tree editing, reuse selection, rebuilding, release, and result normalization
remain separate costs.

A combined checkout with the neighboring reuse-certification commit passed
focused lexical and legacy-reuse checks, but its Parser grew to 2,336 bytes and
failed the existing 2,328-byte layout pin. This lane changes no pin. The two
implementations apply without conflicts; integration must preserve or separately
justify that neighboring layout change. No pull request was opened.

The full performance gate remains incomplete: original real-code inputs, three
median real files and the largest locked file, 16 fixed deletion sites, latency
percentiles, Go-C-C-Go cycles, stack/cliff attribution, initial-load/NewParser
cost, and a sealed quiet-host campaign remain unverified. C#'s absolute RSS
and all three large-fixture Go/C failures remain. No language graduates.
