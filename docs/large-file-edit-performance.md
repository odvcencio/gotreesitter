Large uncertified incremental edits reserve a frontier that successful fresh
verification discards. The engine change in `545ebb06a` selects that same fresh
result before rebuilding the frontier. It shares the existing 512 KiB cutoff
with verification and keeps the configured work limits, memory budget, included
ranges, and cancellation flag.

The completed comparisons use main revision `9148a96db` as the
baseline. Complete-edit Go/C falls from 63.21x to 54.16x. Median edit allocation
falls from 273,799,184 to 163,448,316 B/op (40.3%), and median peak RSS falls from
684.9 to 595.2 bytes per input byte (13.1%). Go and Java comparisons also have
20 paired seeds. All five languages have three candidate RSS measurements.
C# and Python timing comparisons have provisional samples; their planned
20-seed performance gates remain incomplete at the run time limit.

| Language | Paired seeds | Complete-edit Go/C, before → after | Go seconds/edit, before → after | Peak RSS/input byte, before → after |
| --- | --- | --- | --- | --- |
| C# | 4 (provisional) | 123.65 → 129.54 | 6.335 → 6.117 | 983.8 → 915.8 |
| Go | 20 | 13.30 → 13.92 | 0.683 → 0.640 | 270.3 → 273.9 |
| Java | 20 | 13.61 → 12.49 | 1.202 → 1.306 | 251.9 → 246.2 |
| TypeScript | 20 | 63.21 → 54.16 | 5.761 → 5.041 | 684.9 → 595.2 |
| Python | 3 (provisional) | 5.17 → 5.46 | 7.642 → 8.582 | 2244.6 → 2198.7 |

| Language | Go/C range before | Go/C range after | RSS/input range before | RSS/input range after |
| --- | --- | --- | --- | --- |
| C# | 122.66–197.63 | 94.29–147.92 | 914.04–1090.48 | 912.25–1100.18 |
| Go | 8.47–25.74 | 8.36–36.38 | 266.84–320.42 | 265.45–275.55 |
| Java | 2.41–29.53 | 6.43–20.96 | 251.63–253.19 | 246.13–246.32 |
| TypeScript | 44.10–69.28 | 44.39–73.18 | 684.59–696.70 | 554.85–643.48 |
| Python | 4.42–5.88 | 4.41–5.60 | 2213.84–2276.42 | 2194.82–2251.04 |

The provisional C# ratio change is below 5%, and does not establish a
code speedup. C# and Python keep identical B/op, allocs/op, deterministic work,
and fallback code paths. Their incomplete comparisons cannot establish a
performance-gate result.

Go's ratio moves in the guarded direction by 4.7%, below 5%; neither Go timing
cycle is significant in benchstat. Its B/op rises 6.3%, with varying calibrated
iteration counts and unchanged deterministic work and arena counters. Java's
complete Go edit median rises 8.6% while C also slows; neither Go timing cycle
is significant, and work and allocation counts are unchanged. Java's lower ratio
does not establish a code speedup. Both stay within the existing 10% ratchets.

The initial current-main diagnosis used three randomized seeds per language.
These baseline measurements guided profiling and are separate from comparison
gate evidence:

| Language | Main Go/C median | Go/C range | Go seconds/edit | Peak RSS/input byte |
| --- | ---: | --- | ---: | ---: |
| C# | 124.50 | 112.59–149.58 | 12.108 | 983.8 |
| Go | 12.80 | 8.83–14.84 | 1.030 | 270.3 |
| Java | 7.89 | 5.52–9.08 | 1.391 | 251.9 |
| TypeScript | 90.32 | 64.68–97.76 | 4.803 | 684.9 |
| Python | 4.24 | 3.52–4.34 | 12.636 | 2244.6 |

The VM is busy. The TypeScript Go/C sample ranges are 44.10–69.28 before and
44.39–73.18 after. The three peak-RSS ranges are 684.59–696.70 before and
554.85–643.48 after. All three candidate RSS peaks are below all three baseline
peaks. Benchstat finds the GoSecond time change significant (−12.48%, p=0.035),
while GoFirst narrowly misses significance (p=0.052). Allocation changes are
significant in both Go cycles. The JSON receipt keeps all samples, including
timing outliers.

TypeScript comparisons ran on CPU 0. CPU 0 became heavily contended during the
first Go campaign; recent Go/C operations both slowed by about 10x. That mixed
affinity campaign remains a diagnostic, and the complete Go comparison was
repeated with uniform scheduler access to CPUs 0–7 and GOMAXPROCS=1. Java and
the later languages use those same scheduler settings. Baseline RSS ran on CPU
0; candidate RSS outside TypeScript uses CPUs 0–7. The JSON records this detail.

[The JSON receipt](large-file-edit-performance.json) records settings, source
hashes, fixture hashes, four-step work counters, and per-seed samples.
[Baseline output](large-file-edit-performance-before.txt) and
[candidate output](large-file-edit-performance-after.txt) retain the benchmark
rows for benchstat. They omit machine paths. The benchmark body is identical
between baseline and candidate; the baseline uses the harness from `3a308a4d5`.

Each language runs alone in a process, with GOWORK=off, GOMAXPROCS=1, count=1,
750 ms benchtime, and benchmem. The planned comparison has 20 shuffle seeds;
Go, Java and TypeScript completed all 20. C# and Python have only the paired
samples shown above. Their samples are diagnostic and do not satisfy the
20-seed performance gate. No performance threshold or gate was reduced. Baseline and candidate process
order alternates each seed. Within each process the order is Go-C-C-Go. Each
timed operation includes Tree.Edit, incremental parsing, and release of the
previous tree. Parser creation and the initial parse are outside the timer.
Each seed's Go/C value divides the mean of its two Go cycles by the mean of its
two C cycles; the table reports the median of the completed paired ratios, with the sample
count stated explicitly. The RSS table
reports the median of three process peaks. Each RSS process runs only GoFirst
and four edits, including the initial parse in its process RSS.

Fixtures come from benchfixtures.GeneratedSource in issue454_shapes.go, grown
to at least 1 MiB. The edit changes the first marker byte between x and y,
keeping byte width and point unchanged. Actual lengths are 1,048,645 bytes for
C#, 1,048,591 for Go, 1,048,602 for Java, and 1,048,617 for TypeScript and Python.
These are generated cliff fixtures. They do not represent every edit position
or a distribution of real projects.

The C-oracle preflight passed. C uses the locked runtime 0.25.1 at
`f5afe475deb7c0bae6407fb776c76824f717bb61`, through go-tree-sitter v0.25.0 at
`adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3`. Grammars use the commits in
grammars/languages.lock and `-std=c11 -fPIC -O2 -I .`. The runtime links into the
test binary; grammar libraries load through dlopen. The timed C binding includes
its input-copy cost. The external corpus lock was fetched and its digest matched
the existing SHA-256 pin; the lock is outside the repository.

CPU and allocation profiles were collected with GOMAXPROCS=1 for C#, Python,
TypeScript, and Go. Profiles include benchmark calibration and initial parses.
C# spends 93.3% of profiled CPU in parseInternal; node reservation and node growth
account for 51.0% of allocation volume. Python spends 92.9% of profiled CPU in
parseInternal; scanner checkpoint upsert accounts for 33.3% of allocation volume.
Instrumented baseline phase clocks for four edits separate Tree.Edit, reuse
selection, and reparse work. Their medians are 0.016 ms / 0 / 13.421 s for C#
and 0.010 ms / 0 / 13.771 s for Python. The JSON retains each phase sample.
These clocks diagnose attribution; randomized whole-operation measurements
provide timing comparison evidence.

TypeScript node reservation accounts for 76.0% of allocation volume. Its large
edit performs two parse attempts before this change, even though only the fresh
attempt is published. The change removes the discarded attempt and its reserved
arena. C# and Python still take their established fresh fallback routes.

TypeScript's deterministic work changes are:

| Counter | Before | After |
| --- | ---: | ---: |
| Parse attempts | 2 | 1 |
| Shifts | 359,764 | 359,746 |
| Reductions | 587,010 | 558,553 |
| Action lookups | 1,922,292 | 1,827,423 |
| Lexer calls | 359,766 | 359,747 |
| Allocated nodes | 946,775 | 918,299 |
| Arena bytes | 269,500,752 | 159,602,728 |
| Reused subtrees / bytes | 0 / 0 | 0 / 0 |

The other four languages' work and reuse counters are unchanged. Every language
passes four alternating 1 MiB edits with incremental = fresh Go = locked C deep
digests, accepted completion, clean error flags, full coverage, and zero
allocations on reparses without edits. The 206-grammar smoke fleet passes all 48
steps per grammar, including syntax-breaking edits and the design invariants.
The 412-row, 206-language counter ledger passes unchanged. Focused memory-budget,
scratch-budget, dependency, borrowed-arena, and profile tests pass. Large
TypeScript node limits, iteration limits, and cancellation return the same deep
tree and stop reason as fresh parsing; those stop-control tests also pass under
the race detector. The four-step large TypeScript invariant also passes under
the race detector.

All five target languages also pass the 72-step R4 corpus sessions. The broader
R4 audit passed 202 of 206 languages and timed out in Kotlin, Haskell, PowerShell,
and Elsa. Kotlin reaches the same 20-minute
timeout in baseline and candidate Tree.Edit dependency recursion on the locked
2,051-byte fixture. Haskell also reproduces the 20-minute timeout on baseline.
PowerShell and Elsa also reproduce the same 20-minute timeouts on baseline. The largest
R4 sample is 28,828 bytes, below the unchanged 512 KiB selection cutoff. No gate,
threshold, allowlist, pin, or test was relaxed. These findings belong to the
remaining full-session correctness work.

The existing large-file speed and memory cliffs remain. TypeScript still
exceeds 10x C and 400 RSS bytes per input byte. C# and Python retain expensive
full reparses. This receipt supports a measured increment, and does not graduate
any language or change the default engine route.

To reproduce one language, create a baseline worktree at `9148a96db`, copy
cgo_harness/large_file_edit_test.go from `3a308a4d5` into it, and mount both that
worktree and an output directory into the existing Docker harness. Run each
language separately:

```sh
export GOWORK=off
bash cgo_harness/docker/run_parity_in_docker.sh --no-build \
  --mount "$GTS_BASELINE_DIR:/baseline" \
  --mount "$GTS_EVIDENCE_DIR:/evidence" --cpuset-cpus 0 -- \
  "cd /workspace/cgo_harness && GOWORK=off bash /workspace/scripts/run_randomized_benchmarks.sh \
    --output /evidence/after-typescript.txt \
    --baseline-root /baseline/cgo_harness \
    --baseline-output /evidence/before-typescript.txt \
    --runs 20 --tags treesitter_c_parity \
    --bench-regex '^BenchmarkLargeFileEdit/typescript$' --package ."
```

Use the same Docker harness to run TestLargeFileEditInvariant/typescript with
tags treesitter_c_parity,gts_workcount before timing. TestLargeFileEditStopControls
covers the configured stop controls. The production route is selected explicitly
in these tests and benchmarks.

For profiles and repeated RSS, compile the cgo harness test binary with tags
treesitter_c_parity and run one grammar per process with these arguments:

```sh
GOWORK=off GOMAXPROCS=1 /usr/bin/time -v ./large-edit.test \
  -test.run '^$' -test.bench '^BenchmarkLargeFileEdit/python/GoFirst$' \
  -test.count=1 -test.benchtime=4x -test.benchmem \
  -test.cpuprofile=python.cpu -test.memprofile=python.mem
benchstat docs/large-file-edit-performance-before.txt \
  docs/large-file-edit-performance-after.txt
```

The primary Go trio completed 20 alternating randomized seeds. Full parse
median time is 33.05 → 31.90 ms (p=0.925), single-byte edit is 643.7 → 613.1 µs
(p=0.820), and no-edit is 30.46 → 30.15 ns (p=0.441). Every timing delta is
below 5% and statistically insignificant. No-edit remains 0 B/op and
0 allocs/op in every sample. The raw [baseline](large-file-edit-primary-go-before.txt)
and [candidate](large-file-edit-primary-go-after.txt) trio receipts are retained.
The JSON includes medians and full ranges. No pin refresh requires approval.
