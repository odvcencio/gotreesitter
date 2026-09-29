# Recovery live-stack receipt

Baseline: `5ff20d90`. Engine changes: `74f48854` and `ae7c3b58`.
Edit-session regression checks: `db502753`.

The lexer skipped invalid bytes before a lookahead that also needed recovery.
Attachment-gap guards then retired the live stack. Recovery now preserves the
gap after other recovery strategies decline the token, or while a structural
recovery retry absorbs its failed suffix. At EOF, existing stack coverage is
checked before adding skipped-byte errors so an empty reduction does not cause
duplicate ERROR coverage.

The F# inputs `|` and `|>` now accept as `(file (ERROR))`, matching the locked C
reference. The sampled `Decimal.fs` file now accepts and is recovery-equivalent.
Ordinary resynchronization still rejects unexplained comment gaps.

## Recovery equivalence

These are C-error files from the requested grammar receipts. A file passes when
root HasError agrees, error spans overlap, and named nodes outside error regions
are equal. None of the baseline passing files regressed.

| Grammar | C-error files | Baseline passes | Branch passes |
| --- | ---: | ---: | ---: |
| angular | 3 | 2 | 2 |
| asm | 4 | 3 | 3 |
| blade | 1 | 0 | 0 |
| caddy | 3 | 3 | 3 |
| cairo | 3 | 2 | 2 |
| crystal | 4 | 0 | 0 |
| ebnf | 4 | 4 | 4 |
| fsharp | 2 | 0 | 1 |
| glsl | 2 | 0 | 0 |
| haxe | 4 | 3 | 3 |
| hlsl | 1 | 1 | 1 |
| hurl | 4 | 3 | 3 |
| jinja2 | 4 | 3 | 3 |
| kdl | 3 | 1 | 1 |
| less | 2 | 0 | 0 |
| Total | 44 | 25 | 26 |

All 60 sampled files across these receipts accept on the branch. The stop scan
before the second engine fix found one `no_stacks_alive` result, `Decimal.fs`;
the final scan found none. This scan does not cover the entire corpus or every
file in the original 30-grammar triage cluster.

Run the following separately on the baseline and branch, using the same locked
C cache, receipts, and corpus. Run one grammar at a time in the parity container.
`RECEIPTS` and `CORPUS` denote the mounted receipt and corpus directories.

```sh
go build -tags treesitter_c_parity -o /tmp/gm ./cmd/gts_mismatch
for g in angular asm blade caddy cairo crystal ebnf fsharp glsl haxe hlsl hurl jinja2 kdl less; do
  /tmp/gm -mode recovery-equiv -grammar "$g" \
    -receipt "$RECEIPTS/$g.json" -corpus "$CORPUS"
done
```

Count rows where `has_error_agrees`, `errors_overlap`, and
`clean_subtrees_equal` are all true. Compare by grammar and source path to check
for individual regressions rather than relying on the total alone.

## Gates

- PASS: `GOMAXPROCS=1 GOWORK=off go run ./cmd/perfcounterledger`;
  412 rows across 206 languages, without changing the pinned ledger.
- PASS: `TestMergeEventCensusBaseline`, with `treesitter_c_parity gts_merge_census`.
- PASS: `TestIncrementalInvariantProbeLedger`, separately for JavaScript, TOML,
  Python, Go, and TypeScript, and `TestIncrementalNoEditAllocations`.
- PASS: `TestRecoveryMemoTelemetryPreservesAMD64HotLayouts` and
  `TestBuiltinRuntimeProfilesStayNarrow`.
- PASS: focused gap, resynchronization, memory-budget, and accepted-root tests.
- PASS: F# recovery regression and edit-session tests. Every edit step checks
  equality with fresh parsing, input coverage, and allocation-free no-edit parsing.
- PASS: `GOWORK=off go vet ./...`.

An earlier, broader attempt increased Wolfram full-parse tokens from 10 to 14.
That attempt was narrowed before shipping. Final Wolfram full-parse counters are
10 tokens and 12 new nodes, matching the pinned values of 10 and 12.

Randomized timing, maximum RSS measurements, and exhaustive edit sessions for
all 206 grammars were not run. Accepted-tree recovery differences remain outside
the fixed stack-loss cases; this receipt does not claim exact parity for all files.
