# Native selected projection: architectural screen, not publication qualification

The transient projection removes about a third of Go node constructions, but
the timed variants fail the allocation ratchet and omit required reuse metadata.
They stay outside the runtime. These timings establish neither a qualified
native-parser win nor a compact-graduation result.

Baseline: `22554e6bafe29422f0e7fed4b10cdb3e10db13ec` (output PR #1484).
The diagnostic runs a fresh scheduler for every iteration, forces state replay,
and compares ordinary native materialization with native nodes reconstructed
from the one-pass selected records. Both roles share experimental 40-byte
record instrumentation. The driver also lacks the ordinary parse entrypoint's
acceptance-materialization hook. The selected-store output win in #1484 and
the independently qualified reduction win in #1485 remain separate receipts.

## Deterministic construction and metadata evidence

These six fresh, clean derivations match the existing native public tree's
shape, fields, spans, points, production IDs, dynamic precedence and supertype
masks. The diagnostic separately finds zero parse-state, proof-bit, fragility,
field-source and column-dependency differences. Committed core work stays
unchanged. This is comparison with native Go, not an independent C certificate.

| Fixture | Native node constructions | Projected constructions |
| --- | ---: | ---: |
| Go `rewrite` | 2,247 | 1,524 |
| Go `query_compile` | 11,326 | 7,524 |
| Go `language` | 10,917 | 7,082 |
| Go `grammargen_lr` | 107,964 | 71,768 |
| Diagnostic-owned YAML R4 | 484 | 231 |
| Diagnostic-owned YAML 8 KiB EOF witness | 1,936 | 925 |

YAML scanner-checkpoint counts are zero in both trees on these witnesses. Zero
counts do not certify scanner provenance or ordinary YAML ownership.

## Timed variants: allocation failures

Each comparison completed 20 alternating, shuffled pairs with `GOMAXPROCS=1`,
one process and `-count=1` per seed, `-benchtime=750ms`, and `-benchmem` through
the repository randomized runner. Both runs exited zero without an OOM or
timeout; their completion footers and source hashes were checked. All timing
directions below have p<.001. They are diagnostic results with incomplete
publication, not production performance receipts.

| Variant and fixture | Before time | After time | Time direction | Allocated bytes direction | Allocations direction |
| --- | ---: | ---: | ---: | ---: | ---: |
| Reused node/point scratch, Go 20 KiB | 10.73 ms | 10.06 ms | −6.25% | +4.33% | −5.53% |
| Reused node/point scratch, Go 235 KiB | 116.8 ms | 108.2 ms | −7.32% | **+28.05%** | −0.75% |
| Reserved transient plan, Go 20 KiB | 10.701 ms | 9.991 ms | −6.63% | +2.01% | −5.62% |
| Reserved transient plan, Go 235 KiB | 116.7 ms | 108.8 ms | −6.81% | **+13.45%** | −0.78% |

The allocation profile attributes the extra cost to growing and shrinking the
temporary selected-record arrays. A native-only reserved plan avoids repeated
growth and final shrinking while retaining the exact-sized ordinary snapshot
path. It reduces the allocation regression but still exceeds the 10% ratchet.
Neither direction resets a ratchet or requests a counter exception.

## Publication blocker: lexer-read receipts

The broader comparison found omitted legacy lexer-read receipts on 1,393 /
6,811 / 6,286 / 66,459 public Go nodes for `rewrite` / `query_compile` /
`language` / `grammargen_lr`. Matching tree shape and replay states did not
preserve this metadata.

An experiment that feeds only outer payload provenance into the existing read
publisher restores most receipts, but still differs on 131 / 703 / 795 / 5,309
nodes. All remaining differences add a receipt where native has none; none
merely remove an existing receipt. Many witnesses are aliased field/type
identifiers. This transfer is rejected for publication: a public alias cannot
inherit the original occurrence's read certificate without the complete
projection proof. The fixture logs retain concrete spans, words and symbols.

A strict source-symbol guard eliminates those extra receipts, but leaves 67 /
421 / 389 / 4,216 native receipts missing on the same fixtures, including unary
statement wrappers. That version also stays unqualified: discarding valid
receipts would lower reuse.

The narrower transfer recognizes a hidden unary occurrence separately from an
alias copy. On the four Go fixtures it closes every receipt difference: zero
extra, missing, count or keyword/leaf-flag differences, with all other checked
metadata unchanged. The subsequent diagnostic makes receipt and intrinsic
metadata differences fail the comparison. This is a six-fixture qualification
step, not a proof for arbitrary aliases, shared payloads, recovery or edits.

A later prototype moves payload provenance into an optional capacity-charged
plane: ordinary selected records stay **36 bytes**, and public `Node` stays
**104 bytes**. The six comparisons and existing selected-store cap/cancellation
tests pass. The new projection's full live-memory accounting, lifecycle/race
coverage, all-language invariants, C parity, incremental reuse and large-file
RSS gates remain outstanding. This prototype has no qualified timing receipt.

After the strict six-fixture metadata gate, a third 20-pair screen includes
receipt transfer and the 36-byte optional-plane layout. Both files complete,
all source hashes match, and the Docker run exits zero without OOM/timeout.
The small-Go direction is inconclusive; large Go improves **2.40% (p=.001)**.
Allocated bytes still rise **11.33%** on large Go, so this version also fails
the ratchet. Closing metadata gaps reduces the earlier timing advantage.

| Receipt-preserving screen | Before | After | Direction |
| --- | ---: | ---: | --- |
| Go 20 KiB time | 10.98 ms | 10.87 ms | Inconclusive, p=.231 |
| Go 235 KiB time | 120.4 ms | 117.5 ms | −2.40%, p=.001 |
| Go 20 KiB bytes | 228.5 KiB | 237.5 KiB | +3.94%, p<.001 |
| Go 235 KiB bytes | 8.383 MiB | 9.333 MiB | **+11.33%, p<.001** |
| Go 20 KiB allocations | 1,217 | 1,206 | Inconclusive, p=.110 |
| Go 235 KiB allocations | 14,668 | 14,555 | −0.77%, p<.001 |

Construction counts remain 11,326→7,524 and 107,964→71,768. This screen does
not include ordinary-entrypoint acceptance trials or establish YAML throughput.

## Reproduction and exclusions

Evidence directories under `/home/draco/.local/state/gotreesitter-evidence/`:

- `20261014-native-selected-projection`: fresh-reset timings, allocation
  profiles, `source-pins.json`, `perf-pins.json`, overlays and launcher scripts.
- `20261014-native-projection-plan`: reserved-plan timings, construction
  comparisons, exact source hashes and completion logs.
- `20261014-native-projection-plane`: optional-plane sources, layout checks,
  selected cap tests, missing-receipt and transfer diagnostics with separate
  source pins for each variant.

`perf-wrapper.sh` runs the Docker-isolated paired benchmark; `wrapper.sh` runs
the scoped correctness comparison. The image, CPU/memory limits, immutable
baseline checkout and inside-container shared locks are pinned in those
scripts. `parse-before.txt`, `parse-after.txt` and `parse-benchstat.txt` retain
every measured direction. Allocation profiles are attribution, not timing
comparison evidence.

The wrong-API compile attempts, repeated parses without `reset=true`,
an earlier unpooled screen and a terminal launcher-path setup failure are
excluded. The no-reset timings must not be used. No runtime prototype is
committed, no route/default/allowlist changes, and no graduation claim follows.
