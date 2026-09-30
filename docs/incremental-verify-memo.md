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

Baseline: `f9828512c`. Candidate code is identified by the SHA-256 manifest in
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

The paired 19,458-byte numeric witness measures Go 178.200 to
2.175 us (-98.78%); C 756.702 to 751.619 us (-0.67%).
The median paired Go/C ratio is 0.234275x to 0.002902x.
This many-function synthetic case already favors Go over C before optimization;
it is not the owner's real-code speed ratio. Both differences have p < 0.001.
Raw records: [before](benchmarks/verify-memo-inline-numeric-before.txt) and
[after](benchmarks/verify-memo-inline-numeric-after.txt).

METRIC: go numeric edit Go/C | 0.234275x -> 0.002902x | f9828512c -> source manifest | 19458 B, alternating numeric replacement

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

The first increment's completed 20-seed campaigns use the generated witnesses
documented in [the earlier receipt](incremental-verify-cost.md). These paths do
not execute the lexical memo:

| Input / edit | Go before | Go after mode optimization | C before | C after |
| --- | ---: | ---: | ---: | ---: |
| PowerShell, 15,390 B EOF append | 60.60 ms | 60.19 ms | 1.006 ms | 1.015 ms |
| Go, 140,368 B identifier replacement | 220.2 ms | 218.8 ms | 2.785 ms | 2.755 ms |
| C#, 1,048,645 B identifier replacement | 2.861 s | 2.783 s | 26.73 ms | 27.11 ms |

PowerShell and C# timing changes are not statistically significant. The Go
change is -0.63%, p = 0.028. Allocations and reuse stay unchanged. PowerShell
uses `eof_append_fresh`; C# uses `external_scanner_unsupported`; both reuse zero
bytes. The neighboring reuse-certification lane owns retiring the Go legacy
fresh verifier. Its aggregate lexical shortcut still uses this proof.

The C# already-built benchmark process measured peak RSS of 696,792 to 697,668
KiB (+0.126%) under `/usr/bin/time -v`. It includes grammar loading and initial
parsing and excludes compilation. Both one-edit runs allocate 398,943,896 bytes
in 172,808 allocations. This is one RSS run per revision, not a timing campaign
or an RSS median. Both process peaks exceed the existing 400-byte-per-input-byte
gate. No threshold was changed to accept the fallback. Raw RSS records:
[before](benchmarks/verify-rss-csharp-before.txt),
[after](benchmarks/verify-rss-csharp-after.txt).

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
