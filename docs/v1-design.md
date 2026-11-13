# Engine requirements and gates

This document states the engine rules and gates that bind contributors and reviewers.
Where it differs from older text elsewhere in the repository, this document governs.
New engine work goes in `internal/ccore`.
Do not extend the compact scheduler.
Change the legacy engine only to fix defects.
A rule or gate below that names a candidate engine applies to `internal/ccore`.
The functions of `internal/ccore` follow pinned C.
Apply the rules D6 to D13 and all correctness, performance, graduation, and release gates to the core.

## Rules

| ID | Rule |
| --- | --- |
| D6 | Add no language-name comparisons in engine files. Use grammar-owned hooks, conflict policies, and compatibility passes keyed by artifact identity. |
| D7 | Graduate each language through the allowlist. Flip the global default only after all top-50 languages graduate, and only in `v1.0.0-rc.1`. |
| D8 | Require incremental results to equal fresh Go results. Use a fresh parse wherever this invariant remains unproven. |
| D9 | Move the engine into `internal/`. Keep a generated facade of defined types at the root. Put scanner-facing types in a leaf package. |
| D10 | Gate performance with deterministic counters first, then randomized timing. Work counters must not rise; reuse counters must not fall. |
| D11 | Keep the v1 API small. Move diagnostics into `internal/diag`; the sibling `cgo_harness` module can import that path. |
| D12 | Version the grammar format independently from engine releases. |
| D13 | Apply an engine floor to all 206 grammars, tuned targets to the top 50, and stretch targets to the top 20. |

Three more rules have no ID:

- A known difference from C for a grammar needs four things: proof that the grammar cannot graduate, a witness, the blocking mechanism, and an explanation of why no generic mechanism serves it. It also needs the repository owner's approval. A known difference does not authorize intentional divergence from C.
- The legacy engine is not deleted until every grammar it supports has graduated.
- The runtime module keeps zero third-party dependencies.

### Top-50 membership

The top 50 are the existing top-50 list with six replacements.
Dockerfile, Protobuf, Groovy, Vue, Solidity, and MATLAB replace json5, gomod, ini, D, awk, and Elm, which move to positions 51 to 56.
This membership defines the top 50 for D7, D13, and the v1.0 release gate.
`grammars/update_tier1_top50.txt` is the updater's list; it keeps the six replaced languages until the file is updated, and it does not define the top 50 for these rules.

### D9 supporting requirements

- The root facade uses defined types and generated forwarding methods. `facadegen` generates them, and continuous integration (CI) checks its output. The facade preserves the public application programming interface (API).
- `ExternalScanner`, `ExternalLexer`, and `Symbol` live in a leaf package below the root and the engine. The root aliases them.
- Provide an internal helper for scanner tests that use `go:linkname` before moving `ExternalLexer`.
- Lazy `*Node` views preserve node identity within each tree and keep a reparse without edits allocation-free.
- The root-file limits are in the [repository map](repository-map.md#root-file-limits).

### Project rules

- `grammargen` may use an embedded JavaScript engine only in a separate module. The runtime module stays free of third-party dependencies.
- Do not split the repository into more modules before v1.0.
- The automated review is a workflow rule, not a required check. No second reviewer is required.
- No cap limits the number or cadence of prereleases before v1.0.
- Do not build another legacy merge. Limit legacy work to Q fixes.
- A legacy fix of item Q2 or Q6 needs a receipt for each language it affects.

`internal/ccore` implements the C-faithful parser core and imports only the standard library.
The root adapters build its tables and scanner bindings from a `*Language`.
No public API reaches the core; only tests parse through it.

The lanes and the rule of one active PR per lane are in [AGENTS.md](../AGENTS.md#lanes).

## Gates

Run correctness before performance checks. Check deterministic counters before randomized timing.
Publish before-and-after counters in every engine PR body.
Keep the runtime pure Go and preserve strict locked-C parity.
Keep the collapse-class table-validity and safe-reuse requirements.
Known differences describe unresolved evidence; they do not authorize intentional divergence from C.

### Graduation gate: one language

Require all of these results on the candidate commit:

1. Generate a receipt with zero unexplained fresh mismatches against the C commit pinned at M1.
2. Pass the invariant gate for the 72-step session and 16 sites per edit class.
3. Match or exceed legacy reuse. Require positive reuse on clean steps once scanner certification covers the scanner class.
4. Record zero corpus declines, or give every decline an artifact deny entry and a reopening condition.
5. Keep tokens, new nodes, and maximum live versions at or below legacy on every fixture.
6. Keep the 20-seed median time at or below legacy at 32 KiB, 137 KiB, and 1 MiB.
7. Keep peak resident set size (RSS) at 1 MiB at or below legacy.
8. Pass crash, memory-budget, out-of-memory, and race checks in the dedicated container.
9. Preserve highlight and tags parity, including supertype and `MISSING` patterns.

Add the language to the allowlist in the PR that publishes its receipt.

Comparison baselines. Every comparison with C uses the C commit pinned at M1.
Every comparison with the legacy engine uses the default route of the M1 release tag.
M1 (Slim core) is the milestone that releases the Q1 to Q6 legacy fixes, completes E-A0 to E-A8 and E-C, passes the E-A exit gate, pins C 0.27.x by commit, and generates receipts for all 206 grammars.
Keep that baseline fixed when later fixes improve the legacy engine.
The engine exit gate (E-A) uses the same two baselines.

Graduation prerequisites:

- A grammar with an external scanner graduates only after scanner certification covers its scanner class. A stateless scanner needs scanner certification. A stateful or unconfirmed scanner needs scanner certification of its class (capability `scanner_state_reuse`, E-E1, in [v1-capability-flags.md](v1-capability-flags.md)).
- A language whose factory uses a custom token source graduates only after the candidate engine supports a caller-supplied `TokenSource` or `TokenSourceFactory` (`token_source`, E-D1).
- A language that defaults to the GSS-forest route graduates only after the candidate engine serves the forest route and `ParseForestExperimental` (`forest_route`, E-D6).
- Included-range parses move to the candidate engine only after it applies included ranges (`included_ranges`, E-D2).

Prerequisites by grammar group. A grammar graduates only after the item in its row is complete:

| Grammar group | Needs first |
| --- | --- |
| Go | The E-A exit gate |
| C#, Elixir, HTML, Markdown, PHP, Python | E-A3; E-B1 (HTML); E-E1 (Python, HTML) |
| The rest of the top 50 | E-D1, E-D6, E-E1, E-A3, or E-B6, depending on the grammar |
| Grammars that the compact route accepts and that have no scanner | The graduation gate only |
| Grammars with a stateless scanner | Scanner certification |
| Grammars with a stateful or unconfirmed scanner | E-E1 |
| Grammars where the compact route declines on a table shape | E-B6, and E-A3 for proof-gate declines |
| Ada, Apex, JSDoc, Meson | E-B1 and E-B4 (they depend on certified grants) |
| Grammars on the forest route | E-D6 |
| Lean 4 (opt-in) | An admission census run |

### Performance gate: one language

Measure generated fixtures at 32 KiB, 137 KiB, and 1 MiB.
Include three median real files and the largest locked real file.
Test a one-byte delete at 16 fixed sites.

- Record full-parse time per byte and token, allocations per KiB, and bytes per operation.
- Record arena bytes per source byte, peak RSS, and Go/C time on identical input.
- Compare maximum live versions and the token share with multiple versions against the C logger.
- Require accepted completion, full root coverage, and `HasError` agreement with C on clean fixtures.
- Record insertion, deletion, and replacement latency at the 50th and 99th percentiles for 137 KiB and 1 MiB inputs.
- Record reuse and separate time for `Tree.Edit`, reuse selection, and reparse.
- Record initial load time, retained memory, and `NewParser` time.

Use `scripts/run_randomized_benchmarks.sh` for before-and-after comparisons.
Run one language per process with these settings:

- `GOMAXPROCS=1`.
- 20 shuffled seeds, with one process per seed.
- `-count=1`.
- `-benchtime=750ms`.
- `-benchmem`.

Measure C in paired Go-C-C-Go cycles.

Hard failures block merges for graduated languages and for every language/fixture pair that passes today:

- A crash or out-of-memory failure.
- A non-accepted stop on a clean fixture.
- A `HasError` difference from C.
- Go/C above 10x on any file.
- A cliff-detector failure.
- Peak RSS above 400 bytes per source byte at 1 MiB.

The cliff detector fails above `2 × C versions + 2` Go stacks.
It also fails when Go's multi-stack token share exceeds C's share by more than 15 percentage points.
The detector is mandatory for a graduated language.

Keep existing failures in a checked-in exemption list. The list can only shrink.
It holds no entry for a language of cohort 1 (Go, C#, Elixir, HTML, Markdown, PHP, and Python) once those languages have graduated.
It holds no entry for a top-50 language once the top 50 have graduated.
It is empty at v1.0.
Ratchet Go/C, allocations per KiB, RSS, and incremental 99th-percentile latency at 10% above the 20-seed median.
The deterministic ledger rejects work increases or reuse decreases above 2% on any fixture.
The 2% threshold defines the ledger's failure tolerance; it does not replace D10's improvement direction.

**Engine exit gate (E-A).** Before any language graduates, the candidate engine must meet this gate against legacy.
Across the typical-file corpus for all 206 grammars, require median candidate/legacy time at most 1.0 and worst at most 1.15.
Include declined files. Preserve the sealed Go results and reduce the route sanity test's `maxRatio` from 6.0 to 1.15.

| D13 layer | Scope | Directional targets for v1.0 |
| --- | --- | --- |
| Engine floor | All 206 | Median at most 3x C; no file above 5x; no cliff failures. Generic scanner certification supplies reuse. |
| Tuned | Top 50 | Median at most 2.5x C; no language above 4x; 99th-percentile edit latency at most 20 ms at 137 KiB. |
| Stretch | Top 20 | Full parse at most 2x C; edit latency at or below C. |

The engine-floor and tuned targets block the v1.0 release. Targets after v1.0 stay directional.
The existing hard failures and ratchets remain mandatory.

#### R3: Counter-ledger changes

Keep the 2% ratchet strict unless the repository owner explicitly approves an exception PR.
An exception may reset ledger rows or let a ratcheted counter move in its guarded direction.
The PR body must list every changed row, its old and new values, and the reason for the change.
The reason must be a correctness change or a measured trade that improves the result overall.
Agents do not approve ledger exceptions.

### Invariant gate: every engine PR

Run this gate for every language the change can affect:

- Incremental parsing equals fresh Go parsing at every edit-session step.
- An `ERROR` root implies `HasError()`.
- The root covers the input, or the stop reason explains the gap.
- A reparse without edits allocates nothing.

These four checks apply to the public parse API. For a language that `internal/ccore` serves, run them through the public route that serves it.
The core's own `ParseWith` hands out a new tree handle on every call, also for an unedited tree; `TestNoEditReparseAllocations` in `internal/ccore` pins how many objects that allocates.

Use a fresh parse where incremental equivalence remains unproven. This requirement does not wait for malformed-input C parity.
Run heavy parity suites in Docker, one language at a time.
Serialize local runs with `flock` on one shared lock file, for example `flock /tmp/gts-docker.lock <command>`.

### v1.0 release gate

- Graduate every top-50 language and delete legacy.
- Graduate every other grammar that the legacy engine supports. Record a known difference only for a grammar that is proven infeasible, with a witness, the blocking mechanism, the reason no generic mechanism serves it, and the repository owner's approval.
- Meet the D13 engine-floor and tuned speed targets. They block v1.0.
- Require subtree reuse for every top-50 language and 99th-percentile edit latency at most 20 ms at 137 KiB.
- Reach recovery parity on at least 60 of 79 cases. Always match C's `HasError` result.
- Publish the v1 API, versioning policy, and grammar format v1.
- Publish the dashboard for all 206 grammars.
- Pass tests on all six platform targets.

The six targets are Linux amd64/arm64, macOS arm64, Windows amd64/arm64, and `wasip1`.
