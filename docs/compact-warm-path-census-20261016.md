# Warm-path measurements before the next compact change

Evidence date: 2026-10-16 UTC. All measurements use runtime
`f7ec67210486be4398d962fa0cf7e73b02da1c41`. All main merges remain held for
0.56.0. These are diagnostic observations, not throughput comparisons or
graduation evidence. Instrumented time and allocations are excluded.

The four canonical Go files and three YAML paths run separately in Docker,
with GOMAXPROCS=1, one CPU, a 6 GiB limit and both repository locks. Observers
start after warmup. Source hashes verify; canonical deep-tree and Core.Work
checks pass, and all 21 raw Go counters match the uninstrumented baseline.

## SKIP loops have no opportunity on these files

None of the seven paths enters an explicit same-state ASCII SKIP loop.
Extending the existing bulk loop to SKIP would add code without helping these
witnesses. The existing ordinary ASCII loop does encounter useful runs:

| Go file | Entries | Additional same-state bytes | Longest additional run |
| --- | ---: | ---: | ---: |
| Rewrite | 494 | 3,489 | 88 |
| Query | 2,233 | 13,563 | 91 |
| Language | 2,336 | 32,421 | 81 |
| Large | 23,646 | 156,657 | 88 |

All three YAML paths report zero ordinary-loop entries too. This rules out
this specific loop extension on these witnesses; it does not generalize to
every grammar or input.

## A larger external-probe memo removes no misses

The existing 16-slot memo already eliminates these repeated external scans:

| Go file | Eligible requests | Memo hits | Actual scans, all invalid |
| --- | ---: | ---: | ---: |
| Rewrite | 374 | 239 | 135 |
| Query | 1,530 | 983 | 547 |
| Language | 1,532 | 1,138 | 394 |
| Large | 19,334 | 13,120 | 6,214 |

No eligible scan succeeds, so caching successful results cannot help these
traces. Replaying every lookup, invalid record and clear reproduces the
actual direct-16 hits exactly. Direct-32/64, two-way/four-way 16-slot and
fully associative 16-slot simulations eliminate no additional misses. A
four-bit XOR index makes misses worse: 156 / 603 / 1,016 / 9,415. The three
YAML paths contain no eligible stateless external-probe requests.

No larger memo or positive-result cache is implemented. This is trace
simulation, without a new runtime or randomized timing claim.

## Checkpoint digest caching is inactive here

The scanner/grammar checkpoint digest seam records zero digest requests,
memo requests and unique identity pairs on all seven paths. Scanner pointer
identity already has a direct representation; it is not repeatedly hashed
at this seam. A digest-cache change has no measured opportunity here.

## Warm CPU profiles point back to graph construction

Separate profiles start after fixture validation and parser warmup, and stop
immediately after the measured loop. Production code remains unchanged.
The runs use language Go 1,000 iterations, large Go 100, ordinary/owned R4
YAML 30,000 each and the 8 KiB YAML stream 10,000. Sampled cumulative CPU for
reduction output construction is 16.96% on large Go, 15.58% on owned R4 YAML
and 16.62% on the YAML stream. Shared graph construction is a better supported
next target than the three inactive or saturated cache/loop proposals above.
Profile percentages locate work; they do not establish a speedup.

Frozen overlays, observers, native manifests, completed logs and audits remain
in local `gotreesitter-evidence/20261016-ascii-skip-census`,
`20261016-relex-outcome-census`, `20261016-checkpoint-digest-census` and
`20261016-warm-runtime-profile`. The initial relex observer compile error is
archived and excluded. No runtime, counter exception, allowlist or default
changes result from these censuses. Issues #1319 (single-version throughput)
and #1321 (declined-file overhead) remain open; ordinary YAML ownership and
the E-A exit gate remain unresolved.
