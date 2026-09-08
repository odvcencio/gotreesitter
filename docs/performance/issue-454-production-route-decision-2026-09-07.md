# Issue 454: route decision

Date: 2026-09-07, revised 2026-09-08.

## Decision

The owner's direction on 2026-09-08: the production generalized-LR (GLR)
engine retires, and the compact parser core must perform better than any
other route. The compact route stays the default fresh full-parse route. The
compact core is the engine to invest in; the production engine receives
only the fixes that keep it safe while it still serves incremental,
injection, included-range, and fallback parses.

An earlier revision of this record proposed the opposite, fixing the
production engine first, on the effort evidence below. The owner rejected
that proposal. The evidence stays here because it measures the gap the
compact core must close.

## Evidence

The measurements come from the
[repair report](issue-454-compact-route-repair-2026-09-07.md), the
[real-corpus matrix](../compact-route-real-corpus-matrix.md), and the
campaign v7 baseline. Fixtures are the 137 KiB issue #454 editor files.

| Dimension | Production engine | Compact core |
| --- | --- | --- |
| Clean full parse against v0.48.1 | 1.06 to 1.19 times, Rust 1.33 | 1.7 to 2.2 times |
| Routes served | fresh, incremental, injections, included ranges, sub-parsers, recovery, all 206 grammars | fresh full parses; the compact borrow route reuses nothing; sub-parsers pin production at three sites; included ranges decline |
| Real-corpus matrix | every row | 64 PASS, 25 FALLBACK, 8 SKIP; Markdown 15 direct and 348 fallback |
| False-clean divergence class, 98 witnesses | the C oracle sides with production on all 98 | publishes clean trees |
| Remaining cost, named | `Token` at 80 bytes, the external token path share, one merge check | the scheduler run alone exceeds a whole production parse; materialization replays the derivation |
| Known deficits | 2.0 to 2.4 times C core work on GLR-heavy canonical fixtures (decision 0006); recovery cliffs, for example the C transient delete at 2.9 s | per-event cost, coverage, recovery ownership, incremental ownership |

## The gap the compact core must close

Closing the compact gap needs all of these:

- a 40 to 50 percent cut in scheduler cost, after per-token cuts returned 7
  percent on Go and under 2 percent elsewhere;
- a coverage burn-down over 25 real-corpus fallbacks and the Markdown
  corpus;
- recovery ownership, so owned recovery publishes without an executed
  end-of-file turn;
- a compact incremental route, so incremental parses stop depending on the
  production engine.

The compact repairs in pull request #1101 stand: the linear recovery memo,
restored incremental reuse for compact trees, the early declines, and the
parity gate. They keep the candidate lane healthy for graduation work. The
production drift cuts in the same pull request stand on both routes.

## Interim measurements

With production selected (`GTS_ADMISSION_CANDIDATE=0`) the 137 KiB clean
full parse sits at v0.48.1 numbers in the same session: Go 62.8 ms against
63.0, hcl 59.5 against 54.9, TypeScript 44.8 against 40.8. The compact route
on the same binary parses Go in 99.1 ms. That 1.6 times is the first
target; the compact core must then pass production and keep going.

## Production engine fixes kept until retirement

These three fixes stay because production still serves the routes the
compact core does not. Two of them, the scratch trim and the smaller token,
also run on the compact route through the shared token source and scratch.

### Scratch lifetime isolation

Scratch lifetime isolation, the open finding in the cost envelope. A pooled
parser scratch kept the transient parent and child slabs of the largest
earlier parse, up to 512K elements, and billed them to every later parse in
the process. A 4 KiB SQL parse after a 315 KiB parse reported 35 MB of
inherited scratch and tripped its certification ceiling once the route
default moved. Each parse now drops inherited transient slabs above four
times its own initial arena estimate before it starts.
`TestParseScratchIsolationSmallLargeSmall` runs the envelope's
small-large-small sequence and fails without the trim.

### Token 80 to 64 bytes

`Token` shrinks from 80 to 64 bytes. The five unexported provenance bits
pack into one flag byte, and the stack position behind a synthetic missing
token moves to a parser-owned anchor table that the token indexes with one
`uint32`. A missing token is shifted in the same step that creates it, so an
anchor never outlives its parse; nested parses append after the outer
entries and truncate back on return. The public fields are unchanged.

### Results, 137 KiB full parse, production route, milliseconds

Minimum of 54 parses per cell, paired runs of the three binaries on a quiet
host, measured in one session.

| language | v0.48.1 | Token 80 bytes | Token 64 bytes | ratio before | ratio after |
| --- | ---: | ---: | ---: | ---: | ---: |
| go | 56.4 | 61.5 | 60.8 | 1.09 | 1.08 |
| rust | 37.2 | 45.5 | 45.6 | 1.23 | 1.23 |
| hcl | 51.5 | 59.0 | 56.5 | 1.15 | 1.10 |
| toml | 31.6 | 35.6 | 32.2 | 1.13 | 1.02 |
| cmake | 73.9 | 81.2 | 77.5 | 1.10 | 1.05 |
| json | 37.6 | 39.5 | 37.6 | 1.05 | 1.00 |
| css | 25.8 | 30.4 | 28.5 | 1.18 | 1.10 |
| scala | 51.4 | 61.5 | 61.4 | 1.20 | 1.19 |
| typescript | 39.7 | 44.3 | 41.0 | 1.12 | 1.03 |

Seven of nine grammars now run within 1.10 times v0.48.1. Rust and Scala
do not move; their remaining cost is the GLR merge work listed below.

### Incremental reuse budget

The incremental reuse budget. The issue #454 C single-byte delete turns
`x0` into `0`, old-tree reuse resynchronizes nowhere, and the incremental
attempt built 3.2 million nodes for a 68 thousand node tree before the
memory budget stopped it and the parser ran a plain full parse. The
attempt now stops with `ParseStopReuseBudget` once it has built four times
the larger of the old tree's nodes and the fresh-parse arena estimate while
reusing under one eighth of the source. The same plain full parse follows.
`TestIncrementalReuseBudgetDeclinesReuseHostileEdit` replays the delete and
requires the fresh-parse tree with under 800 thousand nodes built. Ordinary
keystrokes never reach the budget: they reuse most of the source long
before the node count grows.

## Compact cost, round two

Measured on the Go fixture in one process: the compact scheduler run takes
84.8 ms and materialization 15.6 ms against a 59.3 ms production parse. Passes
are 82 percent single-header, and the existing C4 corridor lane covers 97
percent of those without changing time, because its reduce still runs the
generic apply. On the real route the largest flat cost was large-record
copies at 12 percent, then link validation at 5 percent.

The cuts in this round: the fused replay (one full-derivation pass fewer),
the dead election record, narrow reuse-dependency accessors, in-place
election and header updates, pointer reads for headers, reduction outputs,
pop paths, boundary outputs, and canonical groups, and the direct-append
condense reading the predecessor it already holds. (An in-place
single-header canonicalization was tried and reverted for the double-buffer
copy; it is not part of the result.) Result: about 6 percent on every
measured grammar. The gap is structural from here.

## Eager materialization lane

The materializer is now a struct the scheduler can drive during the run.
With `GTS_COMPACT_EAGER=1` every single-header shift and every in-place
reduction builds its public node at once, and the postorder pass skips the
subtrees that already own a node. On every Go witness the lane builds the
whole tree before acceptance and publishes the same tree, the same replay
stamps, and the same work as the postorder pass.

Measured on the Go 137 KiB witness in four interleaved rounds (minimum of
nine parses each, milliseconds): previous build 88.5 to 93.5, extraction
with the lane off 88.3 to 93.4, lane on 97.8 to 104.8. The extraction is
neutral. The lane alone costs about ten percent: construction interleaved
with dispatch loses the locality of the batch pass, and the compact core
still writes every subtree, link, and lineage record. The lane therefore
stays off. It is the construction half of program item 3, and the gain
arrives with item 1: once a single-header stretch builds public nodes
directly, the core must stop writing the records those nodes replace.

## Corridor default and the canonical probe

The compact scheduler ran a canonical-boundary probe after every dispatch.
A single header that holds a node the dispatch just published gains
nothing from the probe: a fresh node is the latest node of its phase
identity, so the probe returns the head the header already holds. The
generic shift, the in-place reduction, and the corridor direct shift now
skip the probe on that shape and keep the barrier count.

With the skip in place the C4 bytecode corridor (default off since stage
2) runs faster than the generic pass on 14 of 15 bench grammars at 137 KiB
(minimum of nine parses, three rounds, corridor on over corridor off):

| Grammar | Off, ms | On, ms | Ratio |
| --- | --- | --- | --- |
| Go | 89.8 | 86.6 | 0.96 |
| C | 68.9 | 65.6 | 0.95 |
| hcl | 110.4 | 103.7 | 0.94 |
| TypeScript | 66.7 | 62.9 | 0.94 |
| JSON | 63.8 | 52.7 | 0.83 |
| TOML | 57.6 | 51.1 | 0.89 |
| Python | 123.5 | 115.4 | 0.94 |
| Rust | 75.0 | 69.7 | 0.93 |
| INI | 42.2 | 38.2 | 0.91 |
| Scala | 105.7 | 99.7 | 0.94 |
| CSS | 50.2 | 41.3 | 0.82 |
| Make | 54.7 | 44.6 | 0.82 |
| CMake | 127.4 | 120.1 | 0.94 |
| Haskell | 86.2 | 86.5 | 1.00 |
| diff | 29.7 | 24.3 | 0.82 |

These exploratory results do not satisfy the required randomized benchmark
comparison. They predate the review fixes and the benchmark lifetime fix.
A JavaScript recovery mutation also exposed a corridor tree mismatch.
The corridor remains opt-in through `GTS_C4_CORRIDOR=1` pending broader
correctness validation and new performance measurements.

## Recorded parse states: an open finding

Program item 2 proposes that the core record the parse state it pushes
each subtree into, so materialization stops replaying the tables. A trial
recorded the shift target and the reduction goto per subtree, gave every
trailing extra a reduction migrates that reduction's goto state again, and
compared the result with the fused replay on every canonical Go fixture.
Terminals and extras then agree. Reductions inside condensed diamonds do
not: on `startByte, endByte uint32` the accepted derivation's
`parameter_declaration` carries the goto state of the branch that reduced
it, while the tree-position replay computes the goto from the state after
the previous sibling, and the two branches reached that sibling in
different states. The grammargen fixture shows 71 such visible
non-terminal differences. The reuse gate reads those stamps, so the
recorded state is not a drop-in replacement. Item 2 needs a decision on
which state a node inside a merge should carry before it can land; the
trial is not in the tree.

## Historical exploratory result

The 137 KiB full parse, minimum of nine parses over two rounds, on the
same host and load: the compact route at commit 88d3f926, the compact route
at the earlier measurement head f4cf343b, and production in that binary.
The table does not describe the current reviewed implementation.

| Grammar | Start, ms | Now, ms | Production, ms | Now / start | Now / production |
| --- | --- | --- | --- | --- | --- |
| Go | 88.4 | 79.5 | 63.6 | 0.90 | 1.25 |
| C | 69.3 | 64.2 | 99.7 | 0.93 | 0.64 |
| hcl | 113.6 | 103.0 | 60.4 | 0.91 | 1.71 |
| TypeScript | 71.8 | 62.9 | 44.3 | 0.88 | 1.42 |
| JSON | 65.9 | 57.6 | 41.7 | 0.87 | 1.38 |
| TOML | 60.8 | 53.6 | 33.6 | 0.88 | 1.59 |
| Python | 130.1 | 113.5 | 157.2 | 0.87 | 0.72 |
| Rust | 75.6 | 67.7 | 49.4 | 0.90 | 1.37 |
| INI | 45.1 | 37.0 | 28.5 | 0.82 | 1.30 |
| Scala | 111.4 | 101.3 | 62.4 | 0.91 | 1.63 |
| CSS | 52.3 | 46.4 | 30.0 | 0.89 | 1.54 |
| Make | 56.5 | 48.7 | 33.7 | 0.86 | 1.45 |
| CMake | 141.4 | 120.0 | 83.6 | 0.85 | 1.44 |
| Haskell | 86.2 | 80.8 | 77.8 | 0.94 | 1.04 |
| diff | 31.6 | 25.6 | 17.6 | 0.81 | 1.45 |

The earlier sample suggested improvements of 6 to 19 percent.
It does not establish gains after the correctness and benchmark fixes.
Run `scripts/run_randomized_benchmarks.sh` on the corrected baseline and
candidate before making retention or release decisions from performance.

## Compact program

The compact scheduler profile on Go at 137 KiB splits as: scheduler run 76
percent (dispatch 53, of which reduction 24, election 16, shifts 15;
stop-control poll 6, canonicalization 4, lineage 4) and materialization 21
percent (postorder visit 10, derivation replay 5, result build 3). The
scheduler run alone exceeds a whole production parse. The program, in
order of expected return:

1. A single-header advance path. Deterministic stretches are most of every
   parse. When one header holds one plain shift or reduce, the scheduler
   must not pay election receipts, checkpoint interning, canonicalization,
   lineage journaling, or condense. The records it writes must stay
   byte-identical to the general path.
2. Parse states captured during the run, so materialization stops replaying
   the derivation.
3. Materialization straight into result nodes as reductions commit, so the
   postorder visit disappears.
4. Coverage, recovery ownership, and a compact incremental route, so
   production can retire from the routes it still serves.

## Relation to campaign v7

Campaign v7 names the compact core as the sole engine at its end state.
This decision affirms it. The production engine retires when the compact
core serves every route and outperforms it on the sealed and field boards.
