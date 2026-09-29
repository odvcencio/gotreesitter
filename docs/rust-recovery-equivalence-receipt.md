# Rust recovery equivalence

The baseline is main commit `5ff20d9`. The runtime fix is `bf1f931e`;
`da7123fd` and `785218ea` strengthen its regression coverage. Measurements
use the pinned recovery corpus and locked C reference runtime.

A file passes when `has_error_agrees`, `errors_overlap`, and
`clean_subtrees_equal` all hold.

| Grammar | Baseline passing files | Fixed passing files |
| --- | ---: | ---: |
| Rust | 19/26 | 20/26 |
| C | 9/21 | 9/21 |
| Go | 5/7 | 5/7 |
| JavaScript | 25/84 | 25/84 |
| Python | 0/4 | 0/4 |
| TypeScript | 5/5 | 5/5 |

Only `large__out-of-bounds-arguments.rs` changes its result. It now matches
the C tree exactly, with eight error spans on each side. No previously
passing criterion regresses in Rust or the five control languages.

The compatibility pass rebuilt errored macro token trees from source and
discarded parser-owned recovery boundaries. Removing that rewrite preserves
the recovered tree. The regression test fails on the baseline and passes
with the fix; it also checks full input coverage, error reporting, zero
allocations for no-edit reparsing, and incremental equality with a fresh
parse after an edit.

To reproduce in the C parity container, run from `cgo_harness` with
`PINNED_CORPUS` set to the pinned corpus directory:

```sh
go build -tags treesitter_c_parity -o /tmp/gm ./cmd/gts_mismatch
/tmp/gm -mode recovery-equiv -grammar rust -in "$PINNED_CORPUS/rust/*"
go test -tags treesitter_c_parity . -run '^TestRustRecoveryPreservesTokenTreeErrors$' -count=1
```

Repeat the measurement on the baseline and fixed revisions. Measure each
control grammar separately with the same corpus and locked reference.

## Verification

- Passed: deterministic counter ledger before and after, with 412 unchanged
  rows across 206 languages. No pinned ledger values were edited.
- Passed: focused Rust tests, Rust incremental invariant gate, recovery
  hot-layout check, narrow runtime profile check, and G6 merge census.
- Passed: repository-root `go vet ./...` and diff checks.
- Failed: additional C harness module vet, on unchanged structural fixture
  and lock-copy diagnostics. Those files were outside this change.
- Timing benchmarks and race tests were not run; this receipt makes no
  timing or race-safety claim.

Six Rust files remain outside recovery equivalence. Keyword-promotion,
skip-cost padding, and conflict-policy experiments did not improve the
score. One remaining structural mismatch shrinks to `struct F{et x:i`
(15 bytes): Go and C place the error differently, and C inserts a missing
closing brace.
