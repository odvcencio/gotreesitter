# Sixteen-slot stateless probe memo: qualified Go improvement

Evidence date: 2026-10-16 UTC. Baseline: `e4243c77ad9ede0aea152331111106e3cc417c28`.
Core commit: `86879374e37ef0028443121d1bf12214377653a6`, in [PR #1485](https://github.com/odvcencio/gotreesitter/pull/1485).

Increasing the negative stateless probe memo from eight to sixteen slots reduces
canonical Go language parse time **1.91%** and large Go time **0.80%**. The other
two Go files and all three YAML timings are inconclusive. This comparison starts
after the [eight-slot improvement](compact-relex-memo-receipt-20261016.md);
independent deltas are not added. Ordinary YAML activation and compact graduation
remain gated. All main merges remain held until the owner lifts the 0.56.0 hold.

## Change and deterministic qualification

The index changes from `state & 7` to `state & 15`, and the valid mask grows
from uint8 to uint16. Full uint32 state/start equality, scalar metadata,
checkpoint authentication, stateless-only eligibility, read-span replay and
capture/reset/S5 invalidation remain unchanged. No route or allowlist changes.

Refining a direct-index bucket retains every eight-slot hit for the same
operation sequence. A 100,000-step reference test covers high states, differing
start bytes, skipped recordings and resets. This is a conditional property of
the memo policy; it does not replace parser transition checks. Typed storage
grows **132→260 bytes** on amd64, still lazily allocated, bounded and fully
charged by the existing footprint calculation. There is no extra heap object.

Before timing, all **414 ledger rows** remain exact: 412 ordinary and two owned
YAML rows. Go R4 full tokens/nodes/max versions stay **589/1,251/3**; first-edit
tokens/nodes/reused bytes/splices stay **374/1,553/1,784/266**. Owned YAML full
and first-edit public constructions stay **484/484**. No counter exception.

| Canonical Go fixture | Raw table-lookup proxy: before → after |
| --- | ---: |
| rewrite | 7,568 → 7,553 |
| query | 37,640 → 37,640 |
| language | 37,601 → 37,479 |
| large | 398,566 → 397,227 |

All other 20 raw fields, deep tree digests and complete `Core.Work()` vectors
remain exact. Timed parse-work and completion columns also remain exact.

## Twenty randomized pairs per language

| Case | Median before → after | Time direction | p |
| --- | ---: | ---: | ---: |
| Go rewrite | 1.774587 → 1.771902 ms | -0.15% | .512 |
| Go query | 8.552899 → 8.515456 ms | -0.44% | .114 |
| Go language | 9.009200 → 8.836960 ms | -1.91% | <.001 |
| Large Go | 90.873847 → 90.144241 ms | -0.80% | .005 |
| Ordinary YAML attempt/fallback | 194.909 → 193.066 µs | -0.95% | .068 |
| Owned YAML R4 | 352.7425 → 349.2085 µs | -1.00% | .369 |
| Owned YAML 8 KiB stream | 1.367322 → 1.363975 ms | -0.24% | .221 |

YAML's stateful scanner bypasses this memo. All YAML timing differences are
inconclusive; this change supplies no YAML speed claim. Default Go full/edit/
no-edit controls move **−0.11% / +0.07% / −0.13%**, p=.947/.414/.857, also
inconclusive. No-edit remains **0 B/op / 0 allocations**.

Go allocation counts remain **21/129/243/1,716**. Bytes stay **1,776/138,725/
284,629**, while large Go moves **3,251,712→3,251,743 B/op** (+31 bytes,
+0.00095%). YAML bytes/objects remain **16,760/37**, **12,872/109** and
**44,313/336**. Warm benchmark medians do not eliminate the first lazy allocation
or the 128-byte increase in retained typed storage.

Runs use the repository randomized runner, 20 alternating shuffled pairs,
one process per seed, `GOMAXPROCS=1`, `-count=1`, 750 ms and `-benchmem`.
Go seeds: **21601–21620**; YAML seeds: **21701–21720**. Both locks, CPU 6,
6 GiB Docker; both complete without OOM or timeout. Source hashes, pair orders,
samples and work columns pass the audit.

## Correctness, safety and review

The exact timed source passes **206 invariant sessions plus owned YAML**,
zero-allocation no-edit checks, **618 C fixture checks**, focused canonical Go
and YAML C checks and 48-step edit sessions. The strict 204-row real census
matches baseline exactly: **159 PASS / 45 known FAIL**, including all **120
actual compact acceptances C-exact**. Existing failures are preserved, not waived.
Core/root races, phase-zero-A, EOF shadow, 386, lifecycle and memory gates pass.

The **1,049,572-byte / 524-document** owned YAML Go+C safety probe is C-exact,
error-free, full-span and compact-only; process RSS is **145,932 KiB**, below
400 bytes per source byte. Canonical 235,626-byte Go peaks at **120,984 KiB**.
These are single safety probes, not evidence of an RSS improvement.

Post-copy hashes match both timed files; the other 3,691 pinned native inputs
are unchanged. Selected-store one-pass core race checks, root focused checks
and a 386 library build pass. L0 stays **756 root files / 12 API tag sets**;
R6 stays **454 language names / 101 environment reads**. All **64 CI selectors**
resolve across seven workflows and 7,692 defined test functions.

Buckley's bounded source-only review establishes no concrete defect in the
supplied packet. Its limits include identical-operation assumptions and caller
accounting/aggregate lifetime outside the packet; it does not certify execution.
The ignored workspace review copy is removed. The two-file change is committed
through Buckley. CI status belongs to its exact head and is checked separately.

Evidence: `gotreesitter-evidence/20261016-probe-memo-sixteen`, especially
`pins.json`, `perf-audit.json`, `fleet/audit.json`, `promotion-audit.json` and
`commit-audit.json`. The earlier observer-only trace and offline policy replay
live in `20261016-probe-slot-trace`; they are not timing evidence. After promotion,
the physical `/baseline` checkout contains the candidate. `reproduce-native`
preserves both e4243 files and their baseline overlay; use fresh output paths.
