# Deferred terminal publication screen — 2026-10-17

**Rejected before timing.** The private prototype preserves all five Go deep-tree
digests and passes focused Go/YAML correctness checks. It doubles total leaf
construction work after full consumption. No engine source, test, allowlist,
ledger baseline or default route is promoted.

The frozen runtime is `5e52cf2a2644f3d38785513eebc39f8cf8a506b2`.
[Source pins](benchmarks/20261017-deferred-publication/source-pins.json) and the
[private runtime patch](benchmarks/20261017-deferred-publication/runtime.patch)
retain the rejected source. The patch applies to that qualified runtime's Go
files with `git apply --unidiff-zero`; it is evidence, not an active change.

## What the prototype establishes

A typed pointer table distinguishes public nodes from deferred 68-byte leaves
without two pointer arrays. The private adapter preserves replay proofs,
lexer lookahead and elected-keyword receipts, direct field ordinals, extra
children, and stable public child identity. An arena observer updates the
publication table when projection requests a view; it is detached before the
tree escapes. Existing aliases, hidden flattening, inherited fields, unary
ownership transformations and scanner checkpoints retain eager construction.

The investigation found three eager consumers: diagnostic node census, nested
reuse-dependency publication, and parent-link wiring. Reading only eager child
slices can undercount nodes or remove reuse receipts. The private walkers now
read typed child references. The proof walk uses the existing node predicates
on a scalar leaf view, including the distinction between a sibling error and
an explicit recovery ancestor. Parent wiring preserves terminal error summaries
and leaves view creation to child access.

## Construction work

These are deterministic counts, not throughput results. The separate parse and
full-consumption census prevents work moved to an API call from appearing as a
saving. Every full-consumption digest equals the frozen baseline.

| Go fixture | Public arena nodes after parse: before → candidate | Candidate nodes after consumption | Leaf work: baseline → candidate parse + consumption |
| --- | ---: | ---: | ---: |
| real_sample | 1,251 → 1,172 | 1,251 | 588 → 1,097 + 79 |
| rewrite | 2,247 → 2,247 | 2,247 | 1,035 → 2,070 + 0 |
| query_compile | 11,326 → 11,326 | 11,326 | 5,125 → 10,250 + 0 |
| language | 10,917 → 10,917 | 10,917 | 5,057 → 10,114 + 0 |
| grammargen_lr | 107,964 → 107,964 | 107,964 | 49,911 → 99,822 + 0 |

The small Go fixture leaves 79 nodes deferred, or 6.31% of the baseline public
arena nodes. The other four fixtures retain no such saving. Every fixture
performs twice the baseline leaf construction work in total. This is a failed
work gate, with no owner exception requested or applied. No timing or memory
benefit is claimed.

[Stage 3](benchmarks/20261017-deferred-publication/stage3-work.json) retains the
first correct post-reuse-fix census. All leaves materialize before parse return.
[Stage 4](benchmarks/20261017-deferred-publication/stage4-work.json) additionally
avoids construction in the proof walk, but still saves no public nodes.
[Stage 5](benchmarks/20261017-deferred-publication/stage5-work.json) also preserves
deferred references during parent wiring and produces the table above.

## Correctness and limits

Focused publication/reuse/scheduler/lifecycle tests, Go and YAML 72-step edit
invariants, and each language's locked-C fresh/incremental/error suites pass in
Docker. The 412-row ordinary ledger completes; all changed rows and their full
before/after values are in the [ledger diff](benchmarks/20261017-deferred-publication/ledger-changes.json).
Both diagnostic-owned YAML ledger rows remain exact. Reduced public-node
counts in other languages do not establish correctness or throughput wins.

The original diagnostic census undercount and an incompletely initialized
field-test fixture are retained in the private lab's attempt archives. The
first mixed-reference ledger also caught a DOT incremental fallback regression;
reading both candidate depths through typed child references removes it. No
failure was accepted as a new baseline.

The final prototype has no all-language invariant, full C-fleet, strict C-census,
RSS, new root race/386 or randomized timing qualification. The source stops at
the work gate. The [result](benchmarks/20261017-deferred-publication/result.json),
raw logs, census source and scripts retain the check scope and environment.

The next architectural target is deciding terminal ownership before constructing
a leaf. Converting an already constructed compact leaf into a public leaf adds
work; deferring only the final conversion does not remove it. Any next candidate
must account for parse-only, full consumption and edits before claiming a win.

## Reproduce the construction comparison

Use two isolated checkouts of the baseline commit in Docker. Install
`work-census.go.txt` as `deferred_publication_work_census_test.go` in both.
Apply `runtime.patch` with `git apply --unidiff-zero` only in the candidate.
From each checkout, run:

```sh
GOWORK=off GOMAXPROCS=1 GOFLAGS='-p=1 -buildvcs=false' \
  go test . -tags gts_parsercorephase0,gts_workcount \
  -run '^TestDeferredPublicationWorkCensus$' -count=1 -v -timeout=5m
```

Compare the `PUBLICATION_WORK_CENSUS` JSON rows against the archived
[baseline](benchmarks/20261017-deferred-publication/base-publication-work.log.txt)
and [candidate](benchmarks/20261017-deferred-publication/head-publication-work.log.txt)
logs. The archived `check-*.sh` scripts describe the original frozen-overlay lab;
the clean-checkout comparison above needs no lab overlay. The two `.go.txt`
owned-YAML shims retain the separate diagnostic producer used for its ledger
control; they are not runtime changes.
