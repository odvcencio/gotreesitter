# Repository map

Decision D9 in the [v1 design](v1-design.md) supersedes the one-root-package rule.
The engine moves into `internal/`. New engine code starts there.
The root keeps a generated facade of defined types, with forwarding methods from `facadegen`.
Continuous integration (CI) checks the generated facade.
Scanner-facing `ExternalScanner`, `ExternalLexer`, and `Symbol` move into a leaf package below the root and engine.
The root aliases those scanner-facing types.
The facade preserves the public application programming interface (API).
Use one pull request (PR) per lane, as the design requires.

The tables below describe current ownership. They do not claim that the planned moves have landed.
Use [package-layout.md](package-layout.md) to find current files by subsystem.

## Layout phases and root budget

Workstream L ratchets the number of root `.go` files through these phases:

| Phase | Milestone | Work | Root budget |
| --- | --- | --- | --- |
| L0 | M0 | Add the file budget and allowlist. Snapshot the API for each build-tag set. Check test selectors and discover fuzz tests across packages. Record test inventories and ignored blame revisions. | 756 |
| L1 | M1 | Move non-Go files to their owning directories. Reorganize documentation and move test directories. | 756 |
| L2 | M1 | Move 107 external tests into `tests/api/`. Move benchmark helpers into `internal/benchfixtures` to unblock the 49-file cluster. | 650 |
| L3 | M2 | Build `tests/regress/` and a corpus store. Convert one language per PR; remove converted codename tests. | 550 |
| L4 | M2 | Merge generated grammar shims by build constraint. Generate subset registration files and relocate public grammar tests. | 550; at most 60 top-level files in `grammars/` |
| L5 | M3–M4 | Move each existing engine subsystem into `internal/` after its engine phase closes. Move its supporting root files with it. | 500 at M3 |
| L6 | M4 | Delete legacy files and their tests through E-H. | 120 |
| L7 | M4 | Keep the generated facade, examples, and a contract test. Move diagnostics into `internal/diag`. | 25 |

The design records 756 root Go files at baseline commit `e436c82`.
The budgets are phase limits, rather than a claim about the current file count.

For each move:

1. Use `git mv` in a commit that changes only paths and package clauses.
2. Add that commit to `.git-blame-ignore-revs`.
3. Land large moves after a release tag and provide active lanes with a rebase script.
4. Preserve the test inventory and update package-aware selectors.
5. Lower the budget in the same PR.

Copy benchmark inputs into `internal/benchfixtures/testdata` before moving source files that benchmarks read.
Provide internal helpers for scanner tests that use `go:linkname` before moving `ExternalLexer`.

## Current subsystem ownership

| Area | Primary files | Owns |
| --- | --- | --- |
| Public parse API | `parser_api.go`, `parser_pool.go`, `language.go`, `tree.go`, `cursor.go` | Stable caller-facing parsing, language, tree, node, and cursor behavior |
| Parse scheduler | `parser.go`, `parser_retry.go`, `parser_runtime*.go`, `parser_stop*.go` | Parse orchestration, retries, stop reasons, caps, and runtime accounting |
| Reductions and recovery | `parser_reduce*.go`, `parser_recover*.go`, `parser_recovery.go` | Reduction ownership, recovery election, error costs, spans, and aliases |
| GLR and forest | `glr*.go`, `glr_forest*.go`, `glr_gss.go`, `pending_parent.go`, `transient_*.go` | Stack/forest identity, merging, materialization, and transient ownership |
| Lexing and token sources | `lexer.go`, `parser_dfa_token_source.go`, `external*.go`, `external_scanner*.go` | DFA lexing, external-token integration, scanner state, and checkpoints |
| Incremental parsing | `incremental*.go`, `parser_incremental_support.go`, `parser_relex*.go`, `w1*.go` tests | Edit application, reuse admission, splice/settle paths, and correctness gates |
| Result compatibility | `parser_result*.go` | Oracle-gated language compatibility that remains after engine-level selection |
| Queries and analysis | `query*.go`, `highlight*.go`, `tagger*.go`, `injection*.go`, `imports.go`, `understanding.go` | Query compilation/execution and higher-level syntax services |
| Compact parser candidate | `internal/parsercorephase0/`, root `parsercore_phase0*.go` files | Compact scheduler, selected store, admission, replay, and parity ratchets |
| Allocation and storage | `arena*.go`, `raw_shape*.go`, `no_tree_node.go`, `node_field_metadata.go` | Arena lifetime, compact/raw node storage, and memory accounting |

The result-compatibility tier has its own retirement rules in
[compat-tier.md](compat-tier.md), with the ordered root-cleanup program in
[root-normalization-retirement.md](root-normalization-retirement.md). External
scanner certification and fallback policy live in
[external-scanners.md](external-scanners.md). Compact admission breadth is
tracked in [compact-route-coverage-census.md](compact-route-coverage-census.md).

## Package and tool directories

| Path | Purpose |
| --- | --- |
| `grammars/` | Embedded grammar registry, generated blobs, scanners, and runtime profiles |
| `grammargen/` | Grammar import, table construction, minimization, and blob generation |
| `internal/parsercorephase0/` | Internal compact-parser implementation |
| `cgo_harness/` | C oracle, parity, race, corpus, work-count, and certified timing harnesses |
| `cmd/` | Maintainer and user CLIs such as `ts2go`, `tsquery`, `benchgate`, and `parity_report` |
| `taproot/`, `grep/` | Higher-level consumers and helper packages |
| `roottest/` | Root-package black-box test packages (`bench`, `highlight`, `parse`, `query`) split out so `go test ./...` and CI race lanes can target them independently of the root package |
| `parser_result_test/` | Black-box tests (`package parserresult_test`) for the result-compatibility tier; kept separate from the root package so census and dispatcher tests cannot depend on unexported internals |
| `pgo/` | Current profile-guided optimization (PGO) input: `default.pgo`. CI uses `-pgo=pgo/default.pgo`; L1 moves the profile beside its consuming commands. |
| `policy/` | The `arbiter`-checked release policy (`release.arb`, `release.test.arb`) that `.github/workflows/release.yml` runs directly by path |
| `wasm/` | Browser runtimes and grammargen WebAssembly targets |
| `scripts/` | Bounded host-side maintenance helpers; heavy correctness work stays in Docker or CI |
| `testdata/` | Checked-in regression fixtures and ratchet manifests |

## Review-sensitive seams

Canopy's churn/complexity/centrality analysis identifies the parser scheduler,
reduction engine, DFA token source, recovery election, tree cloning, and GLR
merge paths as the highest-risk shared seams. In practice, changes around
`parseInternal`, `completeConflictReduceFrontier`, `mergeStacksWithScratch`,
`DFATokenSource.Next`, `cRecoverStrategy1Election`, or tree/arena cloning need
the smallest relevant correctness gate first, followed by the appropriate
single-grammar parity lane. Performance evidence comes after correctness.

Put new behavior in the subsystem that owns the invariant, under `internal/`.
Use grammar-owned hooks for language rules (D6 and E-A8).
Add no language-name comparisons in engine files.
The graduation allowlist controls routing; it does not authorize language-specific engine rules.

## Test placement

- Put focused tests beside their owning subsystem. Put new engine tests under `internal/`.
- Use `parser_result_<language>_test.go` only for compatibility behavior that
  cannot yet be expressed upstream.
- Put grammar generation tests under `grammargen/` and registry/scanner tests
  under `grammars/`.
- Put C-oracle, corpus, and certification tests under `cgo_harness/`.
- Keep broad race and parity coverage in CI or one-language Docker lanes; do
  not use host-wide `go test ./...` as a maintenance shortcut.

## Local artifacts and receipts

Ignored data is local state, not repository structure:

- Root `*.test`, `*.log`, `*.prof`, `benchgate`, `parity_report`, `scannertest`,
  `ts2go`, `tsquery`, and accidental Windows `nul` files are reproducible build
  artifacts.
- `.parity_seed/`, generated corpus directories, and grammar seeds are caches.
- `harness_out/`, parity outputs, reports, and benchmark-run directories can
  contain durable correctness, performance, or certified-run receipts.
- `.gts/`, `.tiller/`, and other private agent state are never cleanup targets.

Run `scripts/prune_harness_artifacts.sh` for a size-labelled dry run.
`--delete` removes only root build artifacts and reproducible caches. Receipt
directories require the explicit `--delete-receipts` option so evidence is not
discarded during ordinary cleanup.

## Maintainer entry points

1. Read `AGENTS.md` for correctness/performance gate discipline.
2. Use this map to locate the owning subsystem.
3. Use Canopy for symbol references, impact, and hotspot analysis.
4. Run the smallest Docker correctness gate that covers the change.
5. Keep commits scoped and use the repository's Buckley commit flow.
