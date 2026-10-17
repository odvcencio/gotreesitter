# Reusing failed stateless lexer probes: qualified Go improvement

Evidence date: 2026-10-16 UTC. Baseline: `0808dfd5fbfb1367bf4d8b80ff0b5ceb897fda3c`.
Core commit: `e4243c77ad9ede0aea152331111106e3cc417c28`, in [PR #1485](https://github.com/odvcencio/gotreesitter/pull/1485).

Canonical Go parse time falls **0.52–2.40%** on all four measured files with
unchanged allocation medians and parse-work counters. This qualifies a Go
throughput improvement. Ordinary YAML routing and compact graduation remain gated.
All merges to main remain held until the owner lifts the 0.56.0 hold.

## Runtime and correctness boundary

Reduction passes can ask the same stateless scanner/state request to relex from
one unchanged election snapshot repeatedly. An eight-slot internal memo now
reuses only the early result that no usable external candidate exists. It does
not cache positive candidates or the comparison against the shared after-state.
Every hit retains scanner/checkpoint checks and exact read-span/external-token
metadata. Equality uses the full 32-bit state and start byte; a slot collision
only discards reuse. No input, token, tree or scanner-payload reference is stored.

The cache clears when the frozen snapshot changes, on scheduler reset and on S5
restore. Reset retains the bounded allocation; the footprint charges its 132-byte
typed storage on amd64. Allocation is lazy. Read-span capture merges earlier
reads back with `max`, including on panic. New engine code starts in `internal/`;
regressions extend an existing root test file, preserving the 756-file budget.

A before/after S5 regression fails on the earlier private implementation with
`S5 restore retained speculative negative lexer result`; the final source clears
speculative entries through nested restores while retaining bounded storage.
High-state tests distinguish 65,536 / 65,537 and the maximum uint32 state from
low-bit collisions. Span, cursor/scanner, GLR state, reset and footprint checks pass.

## Twenty pairs on the final source

| Case | Median before → after | Parse-time direction | p |
| --- | ---: | ---: | ---: |
| Go rewrite | 1.814990 → 1.774150 ms | -2.25% | <.001 |
| Go query | 8.706752 → 8.497532 ms | -2.40% | <.001 |
| Go language | 9.033046 → 8.986336 ms | -0.52% | .001 |
| Large Go | 92.821352 → 90.859937 ms | -2.11% | <.001 |
| Ordinary YAML attempt/fallback | 194.163500 → 192.413000 µs | -0.90% | .002 |
| Owned YAML R4 | 350.983500 → 348.888500 µs | -0.60% | .098 |
| Owned YAML 8 KiB stream | 1.357103 → 1.359126 ms | +0.15% | .512 |

The YAML owned R4/stream directions are inconclusive. YAML's stateful scanner
bypasses this memo, so the ordinary fallback's measured reduction is not attributed
to cached probes. No YAML speed improvement from this mechanism is claimed.
The +0.15% stream direction remains in the receipt.

Go bytes/objects stay **1,776/21**, **138,725/129**, **284,629/243**, and
**3,251,743/1,716**. YAML stays **16,760/37**, **12,872/109**, and **336** stream
objects; stream byte medians round **44,313→44,312**. These are steady-state
benchmark medians, not a claim that the first lazy allocation is free.

Default full/edit/no-edit controls move **−0.07% / +0.20% / −1.10%**, with
p=.678/.968/<.001. The first two are inconclusive. These controls bypass the
new memo; their movements receive no causal attribution. No-edit stays at
**0 B/op / 0 allocations**. All other timed work/completion columns remain exact.

Both languages use the repository randomized runner, 20 alternating shuffled
pairs, one process per seed, `GOMAXPROCS=1`, `-count=1`, 750 ms and `-benchmem`.
Go seeds are **21401–21420**; YAML seeds are **21501–21520**. Runs are serialized
under both locks in 6 GiB Docker on CPU 6. Both containers exit zero without
OOM or timeout. All six promoted source hashes equal the timed files.

## Deterministic counters before timing

All **414 rows** are exactly equal: 412 ordinary work/reuse rows and two
owned-YAML rows. Go R4 full tokens/nodes/maximum versions stay **589/1,251/3**;
first-edit tokens/nodes/reused bytes/splices stay **374/1,553/1,784/266**.
Owned YAML full/edit nodes stay **484/484**. No ledger exception is used.

| Canonical Go fixture | Raw table-lookup proxy: before → after |
| --- | ---: |
| rewrite | 7,582 → 7,568 |
| query | 37,662 → 37,640 |
| language | 37,652 → 37,601 |
| large | 400,160 → 398,566 |

All other 20 raw fields, pinned deep tree digests and complete `Core.Work()`
vectors remain exact. Public-node construction and compact work stay unchanged.

## Full qualification and limits

The exact final runtime passes **206 invariant sessions plus owned YAML**,
allocation-free no-edit reparses, **618 C fixture checks**, focused Go/YAML C
witnesses, and Go/YAML 48-step edit checks. A fresh **204-row** strict census
matches native exactly: **159 PASS / 45 known FAIL**, including all **120 actual
compact acceptances C-exact**. Existing failures are preserved, not waived.
Core/root races, phase-zero-A, EOF-shadow, 386, lifecycle and memory gates pass.
Post-copy source, root/API, environment and CI-selector checks pass.

A **1,049,572-byte / 524-document** owned YAML Go+C probe is C-exact, error-free,
full-span and compact-only. Its process peaks at **151,544 KiB**, below 400 bytes
per source byte. Canonical 235,626-byte Go peaks at **120,332 KiB**. These are
single safety probes, not paired RSS improvement measurements.

Buckley's final source-only review completes without a tool-limit stop and
establishes no concrete defect in the supplied packet. It does not independently
certify execution and retains limits about the full transition call graph and
underlying checkpoint/scanner proof. An earlier review that hit 16 reads is
explicitly not treated as complete. Frozen packets and reports remain in evidence;
the ignored workspace copies are removed.

## Diagnostic counts and discarded attempts

A source-pinned observer sees repeated election/state/token requests on Go:
239/374, 983/1,530, 1,138/1,532 and 13,120/19,334. There are no within-pass repeats.
Those are upper bounds on reuse. A separate observer of the final algorithm
records actual hits **224/374**, **920/1,530**, **510/1,532**, **9,691/19,334**.
Observed/unobserved digests, `Core.Work()` and all 21 raw fields match.
These diagnostics are not timing samples. Collisions remain a future target.

The first private prototype incorrectly narrowed states to uint16. The next
preserved width but shared mutable entries across S5 rollback. Both are excluded;
their deliberately stopped partial timings (exit 143) establish no speed result.
A separate runner exit 127 reflects missing container `rg` after reproducing the
S5 failure; the repaired runner uses Python and completes. None is an OOM or a
parser crash. The corrected preliminary screen's 3.03% large-Go direction is
superseded by the final 2.11% comparison above; independent deltas are not added.

Evidence: `gotreesitter-evidence/20261016-negative-relex-memo`, especially
`promotion-ready/{pins.json,perf-audit.json,fleet/audit.json,promotion-audit.json}`.
The native/request census is `20261016-relex-request-census`; actual-hit evidence
is `rollback-safe/census`. Native source/module/grammar hashes cover 3,691 files.
After promotion, the original `/baseline` mount contains candidate source: do not
rerun old wrappers unchanged. `promotion-ready/reproduce-native` preserves the
four native root files and an overlay masking both new internal files for a
fresh reproduction with new output paths.

Ordinary YAML ownership/cost, E-A and per-language graduation gates remain open.
Admission allowlists and defaults remain unchanged. No main merge occurs here.
