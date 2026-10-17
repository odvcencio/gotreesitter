# Native direct replay: component screen and qualification limits

Direct replay removes the separate per-payload state array from the native
projection prototype. It builds logical nodes first, stamps their states during
the existing authenticated replay traversal, and transfers only state/proof
metadata between occurrences with the same source payload. Parent, field,
alias and supertype identity stay local. A raw-header capacity estimate avoids
running replay merely to size the temporary plan.

This is an external runtime prototype, not a production route change.
Baseline: `22554e6bafe29422f0e7fed4b10cdb3e10db13ec` (output PR #1484).
It follows the [earlier projection screens](compact-native-projection-screen-receipt-20261014.md).
The manual driver lacks ordinary-entrypoint acceptance-materialization hooks;
these results do not establish an ordinary parse or graduation performance win.

## Counters and correctness before timing

Four pinned Go files, eleven added accepted Go witnesses and two
diagnostic-owned YAML witnesses match the native tree on public structure,
fields, spans, points, production IDs, precedence and supertype masks. All
checked replay states, proof bits, fragility, field sources, column flags and
legacy lexer-read words match. Small Go witnesses also publish compact reuse
dependencies: the prototype invokes the existing conservative publisher and
compares each node's dependency presence and byte bound with native.
Committed core work remains unchanged. Selected records stay 36 bytes; Node
stays 104 bytes. Existing selected-store cap/cancellation tests pass.

| Witness | Native constructions | Projected constructions |
| --- | ---: | ---: |
| Go 20 KiB `query_compile` | 11,326 | 7,524 |
| Go 235 KiB `grammargen_lr` | 107,964 | 71,768 |
| Owned YAML R4 | 484 | 231 |
| Owned YAML 8 KiB EOF witness | 1,936 | 925 |

The large-Go temporary plan falls from 5,705,260 to 3,864,212 backing bytes
in the same frozen derivation, before removing the separate replay-state array.
This plan is transient; that number is not total tree retention or peak RSS.

The combined generic-call witness `F[int](x); p.G[T](x)` still declines before
projection with the existing scheduler live-link cap: `9 > 8`. It is recorded
separately from the seventeen accepted witnesses; neither the cap nor the
decline proof is relaxed.

## Twenty shuffled pairs per language

Go and owned YAML run separately through the repository randomized runner.
Each comparison uses alternating pair order, 20 shuffle seeds, one process per
seed, `GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms` and `-benchmem`. All four
raw timing files have complete footers, every pinned source hash matches, and
both Docker runs exit zero without OOM or timeout.

| Fixture | Native time | Projected time | Time direction | B/op direction | Allocation direction |
| --- | ---: | ---: | --- | ---: | --- |
| Go 20 KiB | 10.37 ms | 10.06 ms | Inconclusive, p=.086 | +2.58%, p=.009 | Inconclusive, p=.095 |
| Go 235 KiB | 111.8 ms | 109.0 ms | −2.49%, p=.043 | **+9.60%, p=.009** | −0.77%, p<.001 |
| Owned YAML R4 | 386.6 µs | 373.9 µs | −3.28%, p=.002 | +1.23%, p<.001 | 112→106, p<.001 |
| Owned YAML 8 KiB | 1.505 ms | 1.487 ms | −1.22%, p=.030 | +2.09%, p<.001 | 346→348, p<.001 |

The large-Go allocation direction improves from the earlier +11.33% failure
to +9.60%, narrowly within the 10% ratchet. Both Go time distributions are
noisy; the large case has a statistically supported direction, not a strong
end-to-end speed claim. The extra two allocations on the YAML EOF witness
remain a regression. These results are not added to the independent reduction
win in #1485 or the selected-store output win in #1484.

## Incremental session

An 8,938-byte Go witness starts once with the existing native tree and once
with the projected tree. Both go through six public incremental replacements
and are compared with fresh parsing using the runtime's full structural
comparator. No-edit parsing allocates zero in both initial trees.

| Step | Native reused bytes | Projected reused bytes |
| --- | ---: | ---: |
| 0 | 8,712 | 8,712 |
| 1 | 8,718 | 8,718 |
| 2 | 8,719 | 8,719 |
| 3 | 8,712 | 8,712 |
| 4 | 8,724 | 8,724 |
| 5 | 8,724 | 8,724 |

The first projected-session attempt disabled reuse because its diagnostic
wrapper passed nil scratch to the strict tree-proof helper. That helper
requires a real workspace. Fixing the caller preserves the proof check and
restores the exact reuse counts above. The failed attempt and the subsequent
diagnostic-format compile failure are retained and excluded from qualification.
The initial corpus also exposed omitted compact dependency publication; that
omission was fixed before both timing comparisons.

## Evidence and remaining gates

Separate CPU profiles are attribution only. In the candidate, active dispatch
accounts for 44.35% cumulative CPU and reductions for 18.31% within that
dispatch work. Selected-plan construction accounts for 4.99%; the projection
driver for 12.86%, including read publication. Copying (`runtime.duffcopy`) is
6.04% flat CPU. These overlapping call stacks must not be added together or
used as a timing comparison. They guide the next reduction/publication work.

Evidence: `/home/draco/.local/state/gotreesitter-evidence/20261014-native-projection-direct-replay`.
It holds frozen overlay sources, correctness logs, `source-pins.json`,
`perf-pins.json`, Go/YAML raw timing and benchstat files, Docker launchers,
and the incremental overlay, source pins and session output.

This does not certify arbitrary alias/shared-payload graphs, recovered inputs,
borrowed compact subtrees, included ranges, scanner checkpoints or every edit
site. Independent locked-C parity, full live-memory/pool accounting, lifecycle
and race gates, fleet invariants, large-file RSS, actual-entrypoint throughput
and qualification after the core reduction is integrated remain required.
Ordinary YAML ownership, the allowlist/default and compact graduation remain
unchanged. No counter exception or ratchet reset is requested.
