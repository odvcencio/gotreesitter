# Zero-reuse continuation receipt, 2026-10-01

The pinned first edit now has zero subtree reuse in **78/206** grammars,
down from **83/206** at main `9148a96db` after #1402. The carried VM work
fixed Lua, Nickel, and Starlark. This continuation adds Properties and FIRRTL,
preserves the original hot layouts, and remeasures all five against main.
The historical 81/206 count used a different revision; both sides here use
the same committed samples and the first insertion from edit-session seed 4242.

| Grammar | Reused bytes | Parsed tokens | New nodes |
| --- | ---: | ---: | ---: |
| Lua | 0 → 2,025 | 385 → 0 | 956 → 0 |
| Nickel | 0 → 2,088 | 296 → 0 | 982 → 0 |
| Starlark | 0 → 2,049 | 199 → 0 | 456 → 0 |
| Properties | 0 → 2,050 | 1,176 → 0 | 2,361 → 0 |
| FIRRTL | 0 → 1,780 | 544 → 0 | 1,095 → 0 |

Only those five census rows change. Every fresh-tree digest remains the same
across all 206 grammars. Work counters fall, reuse counters rise, and the
unchanged 412-row default/candidate ledger passes.

The general mechanism authenticates an interior ASCII run-length edit in an
existing terminal. Scanner-owned classes certify that scanner results and
payloads remain equivalent after byte and point projection. The existing
bounded lexer proof still compares raw tokens and keyword selection. The new
Properties and FIRRTL hooks certify alphanumeric runs; they exclude whitespace,
escapes, indentation changes, and comment delimiters. Unknown cases still use
fresh parsing. Ordinary stateful scanner reuse remains disabled.

The carried implementation put a read-bound anchor in every Tree header.
That changed its size from 208 to 232 bytes and broke the unchanged layout
gate. The continuation moves the anchor into the existing cold result metadata
and keeps scanner-only metadata from triggering deferred normalization.
Tree, Parser, and ParseRuntime remain 208, 2,336, and 3,120 bytes. No API pins,
ledger expectations, grammar or corpus locks, graduation allowlists, or gate
thresholds change. The default scanner-certificate test now runs all five
grammars in separate child processes, including under the race detector.

The following tables measure complete operations: Tree.Edit, incremental
parsing, and release. Each of 20 independent seeds uses Go-C-C-Go; the ratio
is computed from each seed's Go and C pair averages before taking the median.
The revisions alternate first position. Runs use GOMAXPROCS=1, count=1,
750 ms per subbenchmark, benchmem, and the randomized benchmark wrapper.
Docker confines the process to one CPU and 4 GiB, with a 3 GiB Go memory limit.
The workstation is shared; the raw samples include both before and after
ranges. Locked C uses runtime f5afe475deb7c0bae6407fb776c76824f717bb61
(0.25.1), binding v0.25.0, and the unchanged grammar lock.

Pinned samples:

| Grammar | Complete Go edit, median | Median paired Go/C | After Go/C range | Go B/op |
| --- | ---: | ---: | ---: | ---: |
| Lua | 0.338816 → 0.079469 ms | 12.7578× → 3.0043× | 2.6902–3.0610× | 1,570 → 728 |
| Nickel | 0.518522 → 0.149874 ms | 33.5856× → 8.9600× | 6.9513–10.1525× | 2,352 → 1,176 |
| Starlark | 0.221000 → 0.098420 ms | 6.6806× → 3.0384× | 2.8716–3.2886× | 2,168 → 672 |
| Properties | 0.748313 → 0.018490 ms | 1.3040× → 0.0321× | 0.0288–0.0363× | 1,305 → 464 |
| FIRRTL | 0.347568 → 0.015199 ms | 97.2553× → 4.2533× | 4.0599–4.3656× | 3,464 → 2,544 |

Generated 137 KiB fixtures contain short tokens. Lua and Nickel edit string
content; Starlark edits an identifier; Properties and FIRRTL edit comment
content. Initial, incremental, and fresh trees match locked C exactly.
These results cover those fixtures, rather than an entire language corpus.

| Grammar | Complete Go edit, median | Median paired Go/C | After Go/C range | Go B/op |
| --- | ---: | ---: | ---: | ---: |
| Lua | 24.670586 → 0.250123 ms | 13.2247× → 0.1310× | 0.1200–0.1611× | 1,408 → 552 |
| Nickel | 28.068333 → 0.553154 ms | 11.2213× → 0.2235× | 0.1812–0.2360× | 1,344 → 480 |
| Starlark | 27.990429 → 0.268711 ms | 2.3613× → 0.0241× | 0.0193–0.0276× | 1,448 → 576 |
| Properties | 42.408578 → 0.215810 ms | 1.2822× → 0.0064× | 0.0062–0.0070× | 1,320 → 464 |
| FIRRTL | 12.330012 → 0.233578 ms | 6.8908× → 0.1322× | 0.1284–0.1340× | 1,432 → 560 |

Full and no-edit measurements use the pinned fixtures:

| Grammar | Full parse ns/op change | Full parse B/op before → after | No-edit ns/op change |
| --- | ---: | ---: | ---: |
| Lua | -0.71% | 1,243 → 1,243 | -0.55% |
| Nickel | -7.25% | 1,594 → 1,594 | -6.58% |
| Starlark | -2.15% | 1,960 → 1,960 | -10.75% |
| Properties | -6.30% | 1,241 → 1,241 | -0.01% |
| FIRRTL | -0.86% | 1,345 → 1,345 | -1.04% |

Changes under 5% are not claimed as speed improvements. The accompanying
benchstat tables report significance and spread; no full-parse speed gain
is claimed from noise. Full-parse allocation counts stay unchanged; byte
estimates differ by at most 1 B/op.
No-edit reparses allocate zero bytes and objects.

Properties and FIRRTL memory probes compile outside the measured process,
then parse a generated 1 MiB file and perform its complete first edit.
These processes retain the old and incremental Go trees. They exclude the
additional fresh Go/C trees and deep digests used by correctness tests.
The single-operation probes are memory evidence, not timing comparisons.

| Grammar | Peak RSS before → after | After bytes per source byte |
| --- | ---: | ---: |
| Properties | 307.80 → 170.09 MiB | 170.09 |
| FIRRTL | 127.08 → 77.12 MiB | 77.12 |

All memory probes exit successfully, remain below 400 bytes per source byte,
and report no crash or OOM. The earlier VM receipt retains the corresponding
memory probes for Lua, Nickel, and Starlark.

Correctness passed in Docker, one grammar per process: exact locked C initial,
incremental, and fresh trees for all five pinned samples; 96 alternating
insertions and inverses; malformed edits; generated 32 KiB, 137 KiB, and
1 MiB fixtures; and 72-step fresh-Go invariant sessions. Exhaustive valid-symbol
masks cover scanner state witnesses, positive and negative length changes,
token endpoints, and actual byte and point cursor projections. Oversized
tokens exhaust the unchanged proof budget and fall back. The R fixed-closer
counterexample remains a negative test. Targeted internal, dependency,
token-invariance, hot-layout, API, engine-policy, and scanner race checks pass.
The 206-language census also passes ERROR-root flags, coverage or explanatory
stops, fresh equality, and zero-allocation no-edit reparses. The carried
dependency fix and its measured adversarial case are documented in the
[earlier receipt](zero-reuse-20261001.md).

Alphanumeric scanner hooks for Gitcommit, Mojo, and GDScript were tried and
reverted because their pinned first edits still had no reuse. Their edited
trees matched C, but work remained 155 tokens / 407 nodes, 235 / 564, and
327 / 860, respectively. These are no-gain candidates. The VM's separate
checkpoint/prefix candidates regressed JSONnet edits 19.8% and Properties
85.6%; its Gitcommit stateless opt-in raised tokens 155 → 300 and nodes
407 → 792. Those candidates remain rejected and are not this implementation.

The remaining 78 zero rows group by reported fallback reason: 65 unsupported
scanners (61 without checkpoints and four checkpointed opt-outs), eight
unproven recovery frontiers, one forest fallback, one explicit Groovy opt-out,
and three grammar/fixture cases (HEEx, HTTP, and Todo.txt). Thirty-one also
have error trees, so these are overlapping underlying causes. No pinned
first-edit zero row stops because of a budget. Existing Norg and Wolfram
coverage gaps have unchanged no_stacks_alive stops and digests. Broader
structural edits, additional real corpora, full per-language performance
graduation, and certification of the remaining scanners are not verified here.


METRIC: zero-reuse rows | 83/206 -> 78/206 | 9148a96db..2e9628033 | pinned first edit, seed 4242

METRIC: lua reused bytes | 0 -> 2025 | 9148a96db..2e9628033 | pinned first edit, seed 4242

METRIC: nickel reused bytes | 0 -> 2088 | 9148a96db..2e9628033 | pinned first edit, seed 4242

METRIC: starlark reused bytes | 0 -> 2049 | 9148a96db..2e9628033 | pinned first edit, seed 4242

METRIC: properties reused bytes | 0 -> 2050 | 9148a96db..2e9628033 | pinned first edit, seed 4242

METRIC: firrtl reused bytes | 0 -> 1780 | 9148a96db..2e9628033 | pinned first edit, seed 4242

METRIC: lua complete edit Go/C | 12.757761 -> 3.004284 | 9148a96db..2e9628033 | pinned insertion and inverse, 20 paired seeds

METRIC: nickel complete edit Go/C | 33.585608 -> 8.960045 | 9148a96db..2e9628033 | pinned insertion and inverse, 20 paired seeds

METRIC: starlark complete edit Go/C | 6.680572 -> 3.038376 | 9148a96db..2e9628033 | pinned insertion and inverse, 20 paired seeds

METRIC: properties complete edit Go/C | 1.303964 -> 0.032103 | 9148a96db..2e9628033 | pinned insertion and inverse, 20 paired seeds

METRIC: firrtl complete edit Go/C | 97.255261 -> 4.253288 | 9148a96db..2e9628033 | pinned insertion and inverse, 20 paired seeds

METRIC: lua complete edit Go/C | 13.224727 -> 0.131010 | 9148a96db..2e9628033 | generated 137 KiB insertion and inverse, 20 paired seeds

METRIC: nickel complete edit Go/C | 11.221265 -> 0.223496 | 9148a96db..2e9628033 | generated 137 KiB insertion and inverse, 20 paired seeds

METRIC: starlark complete edit Go/C | 2.361294 -> 0.024118 | 9148a96db..2e9628033 | generated 137 KiB insertion and inverse, 20 paired seeds

METRIC: properties complete edit Go/C | 1.282161 -> 0.006438 | 9148a96db..2e9628033 | generated 137 KiB insertion and inverse, 20 paired seeds

METRIC: firrtl complete edit Go/C | 6.890765 -> 0.132172 | 9148a96db..2e9628033 | generated 137 KiB insertion and inverse, 20 paired seeds

Raw runs, per-seed summaries, the census, source hashes, filtered validation
logs, and reproduction instructions are in the
[receipt directory](receipts/zero-reuse-continuation-20261001/README.md).
