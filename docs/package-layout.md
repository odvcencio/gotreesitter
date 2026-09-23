# Root package layout

The root `gotreesitter` package holds 226 production Go files and 502 test
files (728 total) at this snapshot. This page maps the file-name prefixes to
the subsystem each group owns, so a reader can find the right file by name
alone. It complements [docs/repository-map.md](repository-map.md), which
covers ownership, review-sensitive seams, and non-Go directories.

This is orientation, not a package boundary: every file below still compiles
into the single `gotreesitter` package. See the phase 2 package-split
proposal (tracked separately, not in this repository) for how these groups
would map onto `internal/` packages.

## Lexer and token sources

| Prefix | Files | Owns |
| --- | ---: | --- |
| `lexer*` | 9 | The generated DFA lexer core |
| `token*` | 16 | Token identity, `TokenSource` glue, invariant checks |
| `external*` | 17 | External-scanner adapters, checkpoints, symbol resolution, VM |
| `scanner*` | 2 | Scanner-facing shared helpers |
| `utf16*` | 3 | UTF-16 code-unit input and coordinate conversion |

## Parser core (scheduler, reductions, retries)

| Prefix | Files | Owns |
| --- | ---: | --- |
| `parser_api*`, `parser_pool*`, `parser_config*` | 7 | Public parser construction, pooling, and configuration |
| `parser_reduce*` | 10 | Reduction application and ownership |
| `parser_recover*`, `parser_recovery*` | 15 | Recovery election, costs, spans, aliases, cycle-debug tracing |
| `parser_stop*`, `parser_timeout*`, `parser_limits*`, `parser_max*` | 9 | Stop reasons, timeouts, and safety caps |
| `parser_retry*` | 2 | Full-parse retry and certified accepted-error retry |
| `parser_relex*`, `parser_dfa*` | 9 | Relexing and the DFA token source binding |
| `parser_incremental*`, `parser_reuse*` | 5 | Incremental-support glue used by `incremental*.go` |
| `parser_scratch*`, `parser_memory*`, `parser_tables*` | 8 | Parser-lifetime scratch buffers, memory accounting, table caches |
| `parser_<language>*` (`parser_go`, `parser_typescript`, `parser_csharp`, `parser_powershell`, `parser_scala`, `parser_sql`, `parser_objc`, `parser_markdown`, `parser_html`, `parser_cpp`) | ~20 | Parser-core-level language-specific fixes that are not part of the `parser_result_*` compat tier (keyword promotion, lexer state, and similar upstream-owned repairs) |
| `parsercore_phase0*` | 125 | The default compact scheduler (`internal/parsercorephase0`-backed), admission, replay, and parity ratchets, gated by `!gts_no_parsercorephase0` |
| `parsercore_c4*` | 6 | The C4 stage-2 table-shape analyzer and program/VM |
| `parsestate_*` | 7 | Parse-state replay and compact-differential support |
| `work_*` | 19 | GLR/GSS work-count instrumentation, gated by `gts_workcount` (see [docs/perf-attribution.md](perf-attribution.md)) |
| `admission_switch*` | 22 | The compact-vs-production admission decision and candidate scoring |
| `admission_census*`, `admission_scorecard*` | 5 | Admission census counters and scorecard receipts |
| `admission_route*` | 5 | Route-equality regression coverage between engines |
| other `admission_*` | 8 | Targeted admission regressions (corpus, recovery, zero-width, and so on) |

## GLR and forest

| Prefix | Files | Owns |
| --- | ---: | --- |
| `glr*` | 15 | GLR stack, GSS (`glr_gss.go`), merge, and forest identity |
| `forest*` | 8 | Forest materialization and publication |
| `transient_*` | 3 | Transient reduce-parent ownership before materialization |
| `pending_parent*`, `no_tree_node*` | 2 | Non-materialized payload representations shared by reduce-time code |
| `conflict_*` | 3 | Conflict-policy resolution |
| `ambiguity_profile*` | 1 | Ambiguity profiling |

## Incremental parsing

| Prefix | Files | Owns |
| --- | ---: | --- |
| `incremental*` | 11 | Edit application, reuse admission, splice/settle paths |
| `w1*` | 5 | Incremental splice correctness gates (block, leading, sibling) |
| `compact_incremental*`, `compact_nested_reuse*`, `compact_reuse*`, `compact_recovery*` | ~10 | Compact-route incremental reuse and dependency proofs |
| `oedit_*` | 3 | Old-tree edit coordinate maintenance |
| `included_ranges*` | 2 | Injection-aware included-range tracking |

## Result compatibility tier

| Prefix | Files | Owns |
| --- | ---: | --- |
| `parser_result*` | 112 | The C-faithful post-parse compatibility tier; see [docs/compat-tier.md](compat-tier.md) and the machine-checked registry at `testdata/result_compat_ownership_v1.json` |
| `compat_ownership*` | 2 | Registry provenance helpers used by the compat-tier test |

## Query, highlight, tags, and understanding

| Prefix | Files | Owns |
| --- | ---: | --- |
| `query*` | 21 | Query compilation, execution, predicates, and directives |
| `highlight*` | 8 | Highlight query execution and incremental highlight support |
| `tagger*` | 3 | Tags-query execution |
| `outline*` | 7 | `Outliner` symbol-outline projection |
| `understanding*`, `fact_*` | 4 | `FactProgram` code-understanding extraction |
| `injection*` | 2 | Multi-language injection orchestration |
| `imports.go` | 1 | Import-fact extraction shared by understanding and tags |

## Language and blob loading

| Prefix | Files | Owns |
| --- | ---: | --- |
| `language*` | 8 | `Language`, symbol metadata, and blob-identity caching |
| `load_language.go`, `language_blob_envelope.go` | 2 | Blob deserialization and envelope parsing |
| `raw_shape*` | 6 | Compact/raw node storage shape |
| `intern_*` | 2 | String and transition interning |

## Tree, node, cursor, and rewriting

| Prefix | Files | Owns |
| --- | ---: | --- |
| `tree.go`, `tree_*`, `bound_tree*` | 5 | `Tree`, released-tree safety, and the pooled `BoundTree` |
| `node.go`, `node_field_metadata.go` | 2 | `Node` and field metadata |
| `cursor*` | 2 | `TreeCursor` |
| `rewrite*` | 2 | Source-level edit collection (`Rewriter`) |
| `pointer_*` | 2 | Pointer/handle safety helpers |
| `missing_*` | 2 | Missing-token synthesis |

## Allocation and storage

| Prefix | Files | Owns |
| --- | ---: | --- |
| `arena*` | 6 | Slab arena allocation and lifetime |
| `merge_event*` | 3 | GSS merge-event census |
| `reference_atlas*` | 3 | `gts_workcount`-gated reference-atlas tracing |

## Test, benchmark, and census support

| Prefix | Files | Owns |
| --- | ---: | --- |
| `benchmark*` | 17 | In-package benchmarks (see [BENCH.md](../BENCH.md) for the canonical results) |
| `fuzz_*` | 3 | Fuzz targets (`go test -fuzz`) |
| `race_*` | 2 | Race-lane build-tag stubs |
| `perf_*` | 3 | Perf-counter and perf-metrics instrumentation |
| `issue380_*` | 3 | Regression coverage for a specific numbered issue |
| `compact_route_campaign*` | 2 | The compact-route lifecycle/campaign registry test |
| `large_*` | 2 | Large-input regression fixtures |
| `walk_*`, `rust_*`, `runtime_*` | 6 | Small focused helper/regression groups |

For file-level ownership disputes, prefer `hypha analyze refs <Symbol>` or
`git log -p -- <file>` over guessing from the prefix table above; prefixes
group by naming convention, not by a machine-checked package boundary.
