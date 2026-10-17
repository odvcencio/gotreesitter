# A smaller condensation frame does not improve throughput

Evidence date: 2026-10-16 UTC. Baseline:
`f7ec67210486be4398d962fa0cf7e73b02da1c41`. All main merges remain held for
0.56.0. The runtime candidate is rejected and remains private.

The experiment moves the existing-boundary continuation into a separate
internal method. Its complete body matches the original byte for byte after
stripping outer whitespace; the original method changes only by delegating
to it after the existing direct-append cases. Merge, history, rollback,
validation and publication order stay unchanged. On linux/amd64, assembly
shows the common function's stack frame falling **1,032→272 bytes**; the new
continuation uses 928 bytes. That frame reduction does not establish a speed
or heap-memory improvement.

The full core suite passes on native and 386 builds, with races and
phase-zero-A checks. Focused Go/YAML locked-C cases, canonical/lifecycle and
memory-budget tests, and 48-step Go/YAML edit sessions pass before timing.
No-edit reparses allocate nothing. All **412 ordinary plus two actual owned
YAML ledger rows** match exactly, as do canonical deep-tree/Core.Work and
all 21 raw Go fields. The standalone owned export proves compact acceptance
with full/edit **484→484** nodes. No counter exception is used.

The repository runner executes 20 alternating shuffled pairs per language:
Go **24501–24520**, YAML **24601–24620**; GOMAXPROCS=1, count 1, 750 ms,
benchmem, CPU 6, 6 GiB Docker and both locks. Source/data hashes, every sample,
alternating orders and timed work columns verify.

| Path | Before → after | Time change | p |
| --- | ---: | ---: | ---: |
| Compact Go rewrite | 1.7601815 → 1.769611 ms | +0.54% | .021 |
| Compact Go query | 8.451962 → 8.511778 ms | +0.71% | .014 |
| Compact Go language | 8.7231205 → 8.779692 ms | +0.65% | .060 |
| Compact Go large | 90.0113205 → 89.833585 ms | −0.20% | .602 |
| Default Go full | 3.838996 → 3.828432 ms | −0.28% | .157 |
| Default Go edit | 99.275 → 99.3315 µs | +0.06% | .495 |
| Default Go no-edit | 3.068 → 3.121 ns | +1.73% | <.001 |
| Ordinary YAML attempt/fallback | 193.188 → 193.9775 µs | +0.41% | .301 |
| Owned YAML R4 | 349.211 → 350.3665 µs | +0.33% | .602 |
| Owned YAML 8 KiB stream | 1.3635205 → 1.362255 ms | −0.09% | .968 |

Rewrite and query regress significantly. No compact Go or YAML path improves
significantly. No-edit bypasses this runtime operation; its control movement
does not establish an intrinsic cost change in that operation. Every direction
remains recorded. Heap-object counts stay exact: compact Go **21 / 129 / 243 /
1,716**, YAML **37 / 109 / 336**. Most byte medians stay exact; large Go moves
3,251,743→3,251,741.5 B/op and YAML stream 44,313→44,312.5 B/op. These tiny
byte differences do not qualify a memory improvement. No-edit stays zero.

Buckley's completed bounded source-only review finds no concrete defect; it
does not execute verification or approve a merge. Full-fleet, strict-census,
RSS and runtime promotion gates are not run for this rejected candidate.
Preparing their scripts is not qualification. Ordinary YAML routing,
allowlists and the global default stay unchanged.

The 386 gate exposes existing ABI-specific size assertions. The unmodified
baseline reproduces those failures. Separate Buckley commit
**d98e047064408324a1bead75fd3bc29fd17ef477** repairs four test files while
preserving exact expected sizes and a stored-reference budget exactly one byte
short of the actual record. Full native and 386 core suites pass with only
these test files overlaid on the unchanged production runtime. Production
record sizes and counters do not change. The repair is committed separately;
the condensation extraction is not copied into the checkout.

Frozen source, test-only baseline reproduction, completed scoped gates,
proper ledger exports, source-only review, assembly, shuffled samples,
benchstat, decision and portability-promotion audits remain in local
`gotreesitter-evidence/20261016-condense-hot-cold`. Reproduce the rejected
comparison from f7, or restore the four native test files from its saved
manifest; the physical checkout now contains the separate portability repair.
