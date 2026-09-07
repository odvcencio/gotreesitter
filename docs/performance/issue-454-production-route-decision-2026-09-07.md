# Issue 454: production route decision

Date: 2026-09-07.

## Decision

Fresh full parses use the production generalized-LR (GLR) engine by default.
The compact core stays available as the graduation candidate lane. A process
selects it with `GTS_ADMISSION_CANDIDATE=1`, with
`SetAdmissionCandidateRouteDefault(true)`, or with the per-Parser override.
The package test binaries opt in to the compact route when the variable is
unset, so the certification and parity suites keep their coverage.

The owner asked for the smaller of two jobs: graduate the compact core, or
fix the production engine. The evidence below selects the production engine.

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

## Effort comparison

Closing the production gap needs three named cuts and the recovery-cliff
work. Each cut has a profile line behind it.

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

## Result

On the default route, the 137 KiB clean full parse returns to v0.48.1
numbers in the same session: Go 62.8 ms against 63.0, hcl 59.5 against
54.9, TypeScript 44.8 against 40.8. The compact route on the same binary
parses Go in 99.1 ms.

## First production tranche in this change

Scratch lifetime isolation, the open finding in the cost envelope. A pooled
parser scratch kept the transient parent and child slabs of the largest
earlier parse, up to 512K elements, and billed them to every later parse in
the process. A 4 KiB SQL parse after a 315 KiB parse reported 35 MB of
inherited scratch and tripped its certification ceiling once the route
default moved. Each parse now drops inherited transient slabs above four
times its own initial arena estimate before it starts.
`TestParseScratchIsolationSmallLargeSmall` runs the envelope's
small-large-small sequence and fails without the trim.

## Second production tranche in this change

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

## Third production tranche in this change

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

## Next production tranches

1. Rust: replace the per-candidate binary search in the merge-time external
   scanner checkpoint check.
2. hcl: cut the external token path, which takes 13 percent of samples
   against 7 percent at v0.48.1.
3. Recovery cliffs: the C transient-error delete at 137 KiB explores about
   3.2 million nodes before the memory-budget retry.
4. Redundant GLR work on GLR-heavy fixtures, under
   `spec.merge-time-election.v1`.

## Relation to campaign v7

Campaign v7 names the compact core as the sole engine at its end state. This
decision does not close that campaign. It changes the shipped default while
the candidate lane stays behind its gates. The execution queue's technical
direction, one C-order advance kernel extracted from the production
per-version path with compact storage behind it, is consistent with fixing
the production engine first. Owner ratification travels through a decision
spore.
