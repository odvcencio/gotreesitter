# Canonical Go incremental performance and repeated-route audit

Runtime/test commit: 85822a85d3013a1ee6b7a6dbcbcaa8b08c60ce86.
Graduation remains open. No runtime repair is claimed.

## Measurement

Six Go fixtures passed the existing four-way admission checks. Then the
standard randomized runner collected 20 seeds, 750 ms per benchmark,
one fresh process per seed, and both engines per process. All 240 rows
completed. The runner required all 12 exact benchmark names per seed.
All 2,462 recorded source hashes remained unchanged through measurement.

Docker used Go 1.25.14, one CPU pinned to CPU 18, GOMAXPROCS=1, and an
8 GiB limit. Build-once mode compiled the harness from its own Go module.
Two setup attempts failed before measurement: the first selected the
nested module from the repository root; the second refused to overwrite
that attempt's output. The successful campaign used a new output path.

The complete command took 400.14 seconds: 374.73 user CPU seconds and
11.87 system CPU seconds. Maximum RSS was 363,300 KiB. The container
reported no timeout or out-of-memory kill.

## Instrumented medians

The Go path uses ParseIncrementalProfiled. Both engines include edit,
parse, result validation, and old-tree release in the timed operation.
Initial full parse is outside timing. Forward and reverse edits alternate.

| Workload | Go ns/op | C ns/op | Go B/op | Go allocs/op |
| --- | ---: | ---: | ---: | ---: |
| unchanged_snapshot_identity | 495.5 | 30,861.0 | 48.0 | 1 |
| same_length_leaf_validation | 21,608.0 | 7,985.0 | 1,048.0 | 4 |
| token_class_change | 44,290,504.0 | 101,910.0 | 8,925,397.5 | 336 |
| same_line_length_change | 45,705,404.5 | 117,216.0 | 8,492,564.5 | 588 |
| early_newline | 743,085,073.0 | 807,129.5 | 142,314,364.0 | 65,577 |
| recovery_deletion | 9,973,339.0 | 75,459.5 | 1,754,724.0 | 559 |

These are instrumented workload observations, not an uninstrumented
engine-speed ratio or a before/after optimization comparison. The
samples use the existing harness's within-process engine order.
Go allocation counters do not include C's native allocations. Raw rows
retain C wrapper allocations and detailed Go work counters.

## Repeated-edit admission gap

Admission starts each direction from a fresh parse. Timing instead
feeds each returned tree into the next operation. These are different
histories. A passing one-step admission does not establish sustained
compact execution or repeated reuse.

A four-cycle diagnostic tested each representative fixture, comparing
each returned tree with a fresh C digest. Those comparisons passed.
The first representative edit declined native compact reuse with:

    compact incremental reuse requires one clean shared-lexer version

The early-newline reverse edit then reported ReuseUnsupported with
reason forest_recovery_fallback. It rebuilt about 837,000 nodes and
reported zero reused bytes. Later edits resumed reuse.

A six-cycle newline control omitted intermediate deep-tree inspection.
It reproduced the same first-reverse fallback, then reused 222,341 bytes
and allocated 1,162 nodes on each of cycles 2 through 5. This rules out
deep inspection as the cause of the observed return to reuse in that
control. The uninspected control is a route diagnostic, not a per-cycle
C parity claim.

All twenty newline benchmark samples had b.N=2. Consequently the single
first-reverse fallback accounts for the reported 0.5 unsupported
operations per operation. The 743 ms median is startup-sensitive; it
does not describe the later repeated-edit state. Do not hide this
first-edit penalty by replacing it with a warmed-only number.

## Required next work

- Fix or account for the first reverse newline recovery fallback.
- Validate the same edit history before admitting repeated timing.
- Report initial forward/reverse costs separately from later edits.
- Extend native compact reuse beyond its present clean-version limit
  only with correctness, deterministic-work, and memory evidence.
- Preserve the representative rebuild workloads alongside identity and
  leaf controls. The earlier fast leaf benchmark does not describe them.

Diagnostic tests intentionally failed on unsupported reuse. Their exact
sources and outputs were saved, then removed from the compiled test
package. No failing test or debug instrumentation remains in the checkout.

## Evidence

Remote directory: harness_out/canonical-incremental-current.
measured-bench.txt contains 240 validated rows. source-manifest.json,
summary.json, process accounting, Docker receipts, and both repeated-edit
probe sources preserve provenance. No source comparison or statistically
significant optimization claim is made.
