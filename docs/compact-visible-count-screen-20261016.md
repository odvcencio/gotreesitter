# Cached-child shortcut: correctness passes, compact throughput does not improve

Evidence date: 2026-10-16 UTC. Baseline:
`d98e047064408324a1bead75fd3bc29fd17ef477`. All main merges remain held for
0.56.0. The runtime shortcut is rejected and stays private. The independent
correctness test is committed separately as
`8e4fe35380423f94db6c03f42c917e85666ba6aa` through Buckley.

The runtime experiment reads a valid cached child's base visible-node total
directly instead of calling the recursive counter again. It retains the
occurrence's alias adjustment, extra-node behavior, root-only ERROR_REPEAT
rule, zero IDs, predecessor checks, overflow errors and cache invalidation.
It changes no production field, arena, ownership check, limit or signature.

Before timing, full native/386 core suites, races, phase-zero-A, focused
Go/YAML locked-C cases, lifecycle and memory-budget checks pass. Both
48-step edit sessions pass with zero no-edit allocations. All **414 ledger
rows**, canonical deep-tree/Core.Work checks and raw21 fields stay exact.
The proper standalone owned export accepts with full/edit **484→484** nodes.
No counter exception or reset is used. Buckley's completed bounded
source-only review finds no concrete defect; it does not execute verification
or approve a merge.

The repository runner uses 20 alternating shuffled pairs per language:
Go **24701–24720**, YAML **24801–24820**; GOMAXPROCS=1, count 1, 750 ms,
benchmem, CPU 6, 6 GiB Docker and both locks. Source/data hashes, all samples,
alternating orders and timed work columns verify.

| Path | Before → after | Time change | p |
| --- | ---: | ---: | ---: |
| Compact Go rewrite | 1.765361 → 1.762826 ms | −0.14% | .478 |
| Compact Go query | 8.4814205 → 8.5041685 ms | +0.27% | .779 |
| Compact Go language | 8.7281195 → 8.739234 ms | +0.13% | .369 |
| Compact Go large | 90.531356 → 90.3731155 ms | −0.17% | .883 |
| Default Go full | 3.8485315 → 3.8338835 ms | −0.38% | .046 |
| Default Go edit | 99.029 → 99.279 µs | +0.25% | .659 |
| Default Go no-edit | 3.0585 → 3.077 ns | +0.60% | .394 |
| Ordinary YAML attempt/fallback | 194.182 → 194.421 µs | +0.12% | .429 |
| Owned YAML R4 | 349.554 → 349.7345 µs | +0.05% | .925 |
| Owned YAML 8 KiB stream | 1.3664865 → 1.370469 ms | +0.29% | .149 |

No compact Go or YAML change is significant. The small default-full result
does not satisfy this experiment's compact-throughput acceptance policy.
Every direction remains recorded. Object counts stay exact: compact Go
**21 / 129 / 243 / 1,716**, YAML **37 / 109 / 336**. Byte medians stay exact
except query 138,726→138,725 and large Go 3,251,741.5→3,251,743 B/op; no memory
improvement is claimed. No-edit remains zero. Full-fleet, strict-census and
RSS qualification are not run for the rejected runtime.

The retained test checks 24 deterministic shared graphs with 48 nodes each,
mixed aliases, extras, missing nodes and ERROR/ERROR_REPEAT symbols. It
alternates warm-child and parent-first orders, then changes visibility policy
and revisits roots in shuffled order. Expected counts come from independent
materialization accounting. All visible-count tests pass on the unchanged
production counter on native, race and 386 builds before the test is copied.
Only that 63-line internal test is committed. The shortcut is not copied.
Ordinary YAML ownership, routing, allowlists and graduation gates stay intact.

## Next probe target: preserve the read dependency

A separate post-warmup diagnostic uses the unchanged production runtime.
It observes actual uncached stateless external-probe failures, excluding
instrumented time and allocations. Trees, Core.Work and raw21 stay exact.

| Go file | Actual failed scans | Complete token equals shared token | Probe span range | Prior span range | Widens prior span |
| --- | ---: | ---: | ---: | ---: | ---: |
| Rewrite | 135 | 81 | 2–12 | 83–92 | 0 |
| Query | 547 | 235 | 2–31 | 26–94 | 0 |
| Language | 394 | 109 | 2–12 | 78–85 | 0 |
| Large | 6,214 | 2,718 | 2–84 | 81–92 | 0 |

All **7,290** probes have nonzero read spans and zero external-token production.
None extends the prior recorded span on these witnesses. Only 3,143 return
the identical complete token; the others show why token equality cannot
justify skipping a probe. The three YAML paths contain no eligible probes.
Complete-token equality is computed in Go, including unexported fields;
serialized Token objects omit those fields and are not a full token oracle.

The scanner's newline/zero-lookahead rule offers a possible negative
preflight, but the surrounding probe also reruns the DFA and records its read
dependency. A shortcut needs a source-backed read-bound proof, including
fallback and included-range behavior. Observed span containment is not that
proof. No probe shortcut or new scanner capability is implemented here.

Frozen sources, independent tests, completed gates/reviews, ledger exports,
shuffled samples, benchstat and rejection/test-only promotion audits remain
in local `gotreesitter-evidence/20261016-visible-child-cache`. Probe observers,
traces, logs and hash/raw-vector audits remain in `20261016-relex-span-census`.
Reproduce timing from d98; the physical checkout now adds only the separately
qualified test. Production runtime remains byte-identical to f7.
