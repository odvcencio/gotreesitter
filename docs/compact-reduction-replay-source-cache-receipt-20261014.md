# Compact reduction replay source cache: discarded

The parent GOTO cache establishes no throughput gain and is **discarded**. With
parse-state replay explicitly enabled, the 235 KiB Go fixture is 0.87% slower
(p=.035). No runtime code from this experiment is integrated. The previously
qualified identity-reduction win remains separate in PR #1485.

## Candidate and source identity

Baseline: `a32d87b6d23b090fef2559380b44bfb2c86a800e`. Ten candidate files
implement an optional eight-byte `(predecessor, result state)` sidecar per raw
subtree. The scheduler captures actual nonterminal GOTO results; publication
uses a pair only for the exact producer parser and predecessor. Terminals,
extras, recovery, reused subtrees and mismatched contexts retain ordinary
replay. Reset and pool return revoke the parser binding. Recycled subtree IDs
clear their slots. The cache holder stays after the hot Core prefix, and
footprint and speculative reserve estimators charge retained backing.

All 3,477 baseline and effective candidate Go/module inputs, all ten candidate
files, the overlay and the randomized runner remain pinned outside the repo.
Verification after timing confirms the baseline checkout and effective source
contents. The workspace overlay includes the baseline's separately qualified
five-file dependency-storage change, so it is not attributed to this cache.

## Correctness and deterministic counters, before timing

The full core suite, core/root races, four canonical Go replay shadows,
72-step Go and YAML edit sessions, owned-YAML invariants, Go C fixtures, strict
Go R4 locked-C structure, owned-YAML document streams and recover-EOF C checks
pass. All **412 ordinary ledger rows** are identical; owned-YAML full/edit
nodes remain **484/484**, with every other owned work/reuse counter unchanged.
The four canonical shadows authenticate every cache hit against ordinary
replay and preserve their pinned deep tree digests. Their parent hits/misses
are 1,128/63, 5,806/273, 5,288/365 and 55,377/2,927.

The native-state differential remains exactly the baseline: **87,898 nodes,
87,661 exact, 237 existing parse-state differences, zero pre-GOTO differences,
847 pre-state abstentions**. This does not establish complete native-state
parity. Six adversarial/wide memory-budget processes pass. The largest sampled
adversarial peak is 1,072,928 bytes within its 1,153,433-byte sampling cap.
The recovery shadow preserves the existing incomplete-sequence decline;
accepted recovery uses no cache hits. A clean trailing-comment case checks
11 hits against original replay. The malformed fused-replay subcase remains
an existing skip and is not a new correctness claim.

## Twenty shuffled paired processes

Every benchmark has twenty before/after samples and a complete runner footer.
The four sequential timing families use the repository randomized runner,
alternating baseline/candidate seed order, `GOMAXPROCS=1`, `-count=1`, `750ms`
and `-benchmem`. Docker is limited to 6 GiB, one CPU (CPU 6), and a 5 GiB Go
memory limit. Both shared host lock inodes are held inside the running
container. The final run exits zero without OOM or timeout.

| Benchmark / context | Before | After | Direction | p |
| --- | ---: | ---: | ---: | ---: |
| Owned YAML R4 | 459.7 µs | 457.2 µs | −0.54% | .841 |
| Compact Go R4 | 1.108 ms | 1.108 ms | +0.04% | .968 |
| Ordinary 20 KiB canonical Go | 10.51 ms | 10.07 ms | −4.18% | .495 |
| Ordinary 235 KiB canonical Go | 115.0 ms | 108.8 ms | −5.38% | .478 |
| Replay-enabled 20 KiB canonical Go | 10.61 ms | 10.69 ms | +0.73% | .068 |
| Replay-enabled 235 KiB canonical Go | 114.3 ms | 115.3 ms | **+0.87%** | **.035** |
| Legacy Go R4 control | 807.0 µs | 770.2 µs | −4.56% | .989 |
| Go full DFA control | 4.867 ms | 4.729 ms | −2.84% | .512 |
| Go one-byte edit DFA control | 136.2 µs | 114.6 µs | −15.88% | .529 |
| Go no-edit DFA control | 3.942 ns | 3.849 ns | −2.36% | .952 |

Replay is disabled in the ordinary canonical benchmarks; the separate
`GTS_REPLAY_PARSESTATE=1` family measures the mechanism's active context.
Neither family is pooled with the other. Inconclusive decreases, including the
edit control, are not wins. No-edit stays **0 B/op / 0 allocations**.
Owned YAML uses **12,936→12,944 B/op** with **111→111 allocations**; compact Go
uses **1,696→1,704 B/op**, **25→25 allocations**. Replay-enabled canonical Go
uses **141,139→141,147 / 3,295,768→3,295,776 B/op**, with **205/3,092** allocation
counts unchanged. Ordinary canonical allocation counts remain **204/3,091**;
its small byte variations are not memory gains. Legacy bytes/allocations stay
**2,712/11**; full/edit control bytes change **1,162.5→1,162 / 395→394**, with
**6/5** allocations unchanged. All directional changes remain recorded.

## Exclusions and limits

The initial launch after the host reboot failed before starting Docker because
the shared lock file was absent; it produced no timing evidence. Early
qualification attempts before pool detachment/reserve accounting and an
incorrect inactive footprint charge are excluded. The final counters,
correctness, memory and timing use the same pinned ten-file implementation.

This is a scoped rejected experiment, not a fleet qualification, 1 MiB RSS
certificate, YAML admission or compact graduation. Full fleet qualification and
runtime integration are withheld because the measured mechanism has no win.
The result redirects work toward publication projection, whose independently
pinned construction census identifies 23–24% hidden-parent node allocation.

Evidence: `/home/draco/.local/state/gotreesitter-evidence/20261014-reduction-replay-source-cache`.
The directory retains source pins, overlays, launch/exit records, exact
counter comparisons, correctness logs, all paired raw runs, four benchstat
reports and `qualification-summary.json`.
