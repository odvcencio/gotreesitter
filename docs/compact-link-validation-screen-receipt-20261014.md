# Reduction copies and link validation: no qualified throughput win

Neither trial enters the runtime. Borrowing a reduction path preserves the
focused correctness checks, but its copy has too little profile attribution to
prioritize. Splitting ordinary graph-link validation from its recovery checks
makes the ordinary predicate inline, but twenty shuffled pairs establish no Go
or owned-YAML throughput improvement. The Go edit control increases 0.39%
(p=.024). Existing qualified work in [core PR #1485](https://github.com/odvcencio/gotreesitter/pull/1485)
remains separate.

Both trials start at `70b27618117e60eecd6e1aaa26552aa81b37f22a`, including the
qualified identity-reduction metadata change. Neither changes routes, caps,
allowlists, ownership proofs or graduation status.

## Correctness before timing

The borrowed-path trial passes the full core unit suite, focused root,
recovery, footprint and canonical lifecycle tests, Go/YAML edit invariants,
the owned-YAML invariant session, Go C fixtures and strict R4 comparison, and
the YAML document-stream, version-ownership and recovery C checks. Its first
build missed a test caller of the changed private signature; the corrected
test passes and the failed build is excluded. No comparative timing was run
for this trial, and no speed claim follows from it.

The validation trial passes the same focused checks. Its original full
recovery/malformed-link validator body is unchanged apart from its name. The
ordinary predicate accepts exactly a nonzero payload with no flags except
optional branch order. All other shapes retain the original errors and checks.
The compiler reports inline cost **101 → 74**, below its budget of 80, and
confirms inlining at the callers. This is a compiler receipt, not a runtime win.

| Deterministic counter | Before → after, both trials |
| --- | --- |
| Ordinary work/reuse ledger | All 412 rows identical |
| Candidate Go full tokens / nodes / versions | 589 / 1,251 / 3 → identical |
| Candidate Go edit tokens / nodes / reused bytes / splices | 374 / 1,553 / 1,784 / 266 → identical |
| Diagnostic-owned YAML full/edit nodes | 484 / 484 → identical |
| Default YAML full/edit nodes | 288 / 480 → identical |
| Ordinary YAML ownership decline | Unchanged |

## Validation trial: twenty paired samples

Go and YAML run separately through the repository randomized runner, with
alternating pair order, one process per shuffle seed, `GOMAXPROCS=1`,
`-count=1`, `-benchtime=750ms`, and `-benchmem`. Both campaigns finish all
twenty pairs and source hashes remain exact. Docker exits zero
without OOM or timeout.

The large-Go benchmark calls the ordinary public `Parser.Parse` candidate
route and checks successful compact routing, complete roots and error status.
It uses the normal runner pool; this differs from the explicitly bound private
runner in the earlier native-projection screen. Results across those baselines
are not combined. The YAML ownership probe remains diagnostic.

| Benchmark | Before | After | Result |
| --- | ---: | ---: | --- |
| Public Go 20 KiB | 10.027 ms | 9.968 ms | Inconclusive, p=.277 |
| Public Go 235 KiB | 108.6 ms | 107.1 ms | Inconclusive, p=.289 |
| Compact Go R4 | 846.1 µs | 845.8 µs | Inconclusive, p=.904 |
| Go full-DFA control | 3.921 ms | 3.893 ms | Inconclusive, p=.968 |
| Go edit-DFA control | 99.04 µs | 99.43 µs | **+0.39%, p=.024** |
| Go no-edit control | 3.142 ns | 3.148 ns | Inconclusive, p=.324 |
| Owned YAML R4 | 381.5 µs | 380.1 µs | Inconclusive, p=.289 |
| Owned YAML 8 KiB | 1.470 ms | 1.469 ms | Inconclusive, p=.355 |

Allocated-byte and allocation medians stay unchanged in all eight rows.
Public Go stays at **141,139 B / 205 allocations** and
**3,295,728 B / 3,092 allocations**. Owned YAML stays at
**12,936 B / 111 allocations** and **44,601 B / 345 allocations**.
No-edit stays at zero bytes and allocations. All route metrics remain one
compact parse and zero fallbacks. Public Go construction counts remain
11,326 / 107,964.

The measured medians stay within the 10% ratchets, but there is no supported
throughput benefit to justify integration. Full fleet/C-corpus, race and large
RSS qualification are not run after this failed screen; prepared scripts do
not constitute completed evidence. No counter exception is requested.

## Public-path attribution and evidence

A separate baseline public-parse CPU profile attributes **40.43% cumulative
CPU** to generic dispatch, **24.93%** to reduction application, **16.56%** to
election and **10.69%** to the postorder materialization visitor. These shares
overlap and must not be added. The profile guides the next dispatch/reduction
investigation; it supplies no comparative timing evidence.

A separate census rules out synthetic no-lookahead elections as the cause of
these Go corridor exits: all four canonical fixtures have **zero** such
elections. The production-configured admission runner already executes most
single-header passes through C4:

| Go fixture | All passes | Single-header passes | Corridor passes | Multiple-header passes |
| --- | ---: | ---: | ---: | ---: |
| `rewrite` | 2,589 | 1,556 | 1,409 | 1,033 |
| `query_compile` | 12,874 | 7,874 | 7,257 | 5,000 |
| `language` | 12,803 | 7,422 | 6,935 | 5,381 |
| `grammargen_lr` | 127,874 | 74,701 | 68,032 | 53,173 |

Large Go therefore spends **41.58% of passes with multiple headers**. C4
already covers **91.07% of its single-header passes**; uncovered single-header
passes are only **5.22% of all passes**. This ranks multiple-version dispatch
and reduction bookkeeping above a no-lookahead corridor extension for these
witnesses. It does not establish a speedup or prove a new pruning policy.

The census records pass mix from an unobserved admission-runner parse. A second
parse observes election tokens; the observer disables C4's election loop, so
its timings are not comparable. Core work, pass/election counts, public tree
structure and reuse eligibility match between the two runs. No scheduling or
ownership rule is changed. The census exits zero without OOM or timeout.

A second read-only census accounts for every multiple-header pass and preserves
the full scheduler work record, core work, public tree and reuse eligibility.
On large Go, 18,402 passes have only shift cells; 3,166 have one reduction cell,
but only 871 of those have every sibling already shifted. Only 208 passes have
multiple reduction cells with the same action. These counts do not justify a
general same-action reduction batch or a broad single-runnable-head shortcut.

A final census counts repeated clean classifications between immediately
adjacent generic passes. It requires the same election, frontier width and
head ID, with no owned lexer, recovery, version state, relexed symbol or selected
conflict policy, and only ordinary shift/reduce/extra-shift cells:

| Go fixture | Classified cells in wide passes | Eligible clean cells | Repeated unchanged-head cells |
| --- | ---: | ---: | ---: |
| `rewrite` | 1,610 | 1,339 | 291 |
| `query_compile` | 7,900 | 6,797 | 1,652 |
| `language` | 9,669 | 6,236 | 1,562 |
| `grammargen_lr` | 88,853 | 77,303 | 19,917 |

Those 19,917 cells are **22.42% of classified large-Go wide-frontier cells**.
They identify a possible reuse seam for immutable table classification, not a
speed estimate or authority to retain a stale boundary. Any cache still needs
exact table/session/election identity, fresh ownership authentication, retained
lexer-read dependencies and correct invalidation when the frontier changes.
No classification is skipped in this census. All four trees and complete work
records remain equal, and both census Docker runs exit zero.

Evidence directories under `/home/draco/.local/state/gotreesitter-evidence/`:

- `20261014-borrowed-reduction-path`: pinned overlay, corrected signature,
  focused test results, exact ledgers and disposition.
- `20261014-inline-link-shape`: pinned runtime and benchmark sources, compiler
  reports, complete raw pairs, benchstat, metric medians, focused checks and
  the public-entrypoint CPU profile.
- `20261014-corridor-no-lookahead-census`: pinned observation source and exact
  per-fixture work/pass mix and tree-equivalence checks.
- `20261014-wide-frontier-census`: complete per-pass shape buckets.
- `20261014-stable-sibling-classification-census`: conservative unchanged-head
  classification counts, full work/tree checks and pinned observation sources.

The runtime patches stay external. Ordinary YAML still needs its ownership
proof, the Go throughput gap remains open, and no merge occurs in this bucket.
