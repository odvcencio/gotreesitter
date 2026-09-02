# Repository map

gotreesitter deliberately keeps the public runtime in one root Go package.
That makes the import surface simple, but it also means the repository root is
wide: at this snapshot it contains 214 production Go files and 308 root-package
test files. Moving those files into cosmetic subdirectories would create new Go
packages and change ownership or API boundaries, so navigation should follow
subsystem names rather than directory depth.

## Root-package ownership

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
tracked in the compact-route campaign lifecycle registry under `testdata/`.

## Package and tool directories

| Path | Purpose |
| --- | --- |
| `grammars/` | Embedded grammar registry, generated blobs, scanners, and runtime profiles |
| `grammargen/` | Grammar import, table construction, minimization, and blob generation |
| `internal/parsercorephase0/` | Internal compact-parser implementation |
| `cgo_harness/` | C oracle, parity, race, corpus, work-count, and certified timing harnesses |
| `cmd/` | Maintainer and user CLIs such as `ts2go`, `tsquery`, `benchgate`, and `parity_report` |
| `taproot/`, `grep/` | Higher-level consumers and helper packages |
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

New behavior should live with the subsystem that owns the invariant. Avoid
adding source-text detectors, language-name allowlists, or new result patches
when scheduler, recovery, scanner, span, alias, or materialization ownership can
express the rule directly.

## Test placement

- Put focused unit/regression tests beside the owning root subsystem.
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
