# gotreesitter v1 design

The owner's design, dated 2026-09-25, governs engine work.
The full design is the Hyphae object
`hypha://m31labs/gotreesitter/specs/spec.gotreesitter-v1-design`.
This document summarizes that design. It does not replace it.
The design supersedes Campaign v7 where they conflict.
It fully supersedes `specs/spec.compact-graduation-rider.v1.md`.

## Goal and baseline

Make compact the only engine, then delete legacy.
Reduce parsing cost and repository size while preserving strict parity with the locked C oracle.
Publish a stable version 1 application programming interface (API) and an independently versioned grammar format.

The design's baseline is commit `e436c82` at v0.55.0.
Its measurements describe that baseline, rather than current performance claims.
Compact is opt-in since #1264. The admission allowlist is empty at this document's introduction.
`admission_switch.go` gives a parser override priority over explicit process settings, then the allowlist, then the implicit off default.

The diagnosis identifies duplicated engines and language-specific engine rules as the common cause.
Legacy rejects useful graph merges. Compact carries proof state through parsing and builds a second tree.
Incremental parsing lacks sufficient reuse and can return trees that differ from fresh parses.
The reset phase establishes reproducible evidence before optimization.

## Decisions

| ID | Decision |
| --- | --- |
| D1 | Make compact the only engine. Keep its index arenas and C-style head merge; rebuild the remaining mechanisms. |
| D2 | Do not build another legacy merge. Limit legacy work to Q fixes; Q0 requires owner decision O-Q4. |
| D3 | Move proofs and detailed telemetry into tests and the `gts_diag` build. Remove them from the hot path. |
| D4 | Use one representation per phase: an index graph, then a structure-of-arrays tree. Create stable `*Node` views lazily. |
| D5 | Enable recovery and acceptance mechanisms for every grammar. Key deny entries by artifact identity and specify reopening conditions. |
| D6 | Add no language-name comparisons in engine files. Use grammar-owned hooks, conflict policies, and compatibility passes keyed by artifact identity. |
| D7 | Graduate each language through the allowlist. Flip the global default only after all top-50 languages graduate, in a release candidate. |
| D8 | Require incremental results to equal fresh Go results. Use a fresh parse wherever this invariant remains unproven. |
| D9 | Move the engine into `internal/`. Keep a generated facade of defined types at the root. Put scanner-facing types in a leaf package. |
| D10 | Gate performance with deterministic counters first, then randomized timing. Work counters must not rise; reuse counters must not fall. |
| D11 | Keep the v1 API small. Move diagnostics into `internal/diag`; the sibling `cgo_harness` module can import that path. |
| D12 | Version the grammar format independently from engine releases. |
| D13 | Apply an engine floor to all 206 grammars, tuned targets to the top 50, and stretch targets to the top 20. |

## Target architecture

Every public parse method builds one `internal/sched.Request` and calls `sched.Parse`.
`Supports` and a capability table identify unsupported modes before parsing starts.
All capability flags must close before legacy deletion.

The scheduler uses these packages:

- `lex` supplies tokens with a cache for each version and lexer mode.
- `gss` stores the graph and merges heads.
- `recover` performs generic recovery.
- `incr` selects reusable subtrees.
- `tree` stores output in parallel arrays.
- `compat` reads the same output through a shared reader interface.

Transactions mark arena lengths and truncate on rollback.
Hot version headers occupy at most 32 bytes; cold state uses indexed sidecars.
Dependency records use dense arrays. At most five counters remain always on.
A single live version uses direct lexing, shifts, and reductions.
Only a budgeted recovery rung can parse the same bytes twice within one request.

The root facade uses defined types and generated forwarding methods.
`facadegen` generates those methods; continuous integration (CI) checks its output.
Scanner-facing `ExternalScanner`, `ExternalLexer`, and `Symbol` move into a leaf package below the root and engine.
The root aliases those scanner-facing types.
Lazy `*Node` views preserve identity within each tree and allocation-free reparses without edits.
The [repository map](repository-map.md) defines the layout phases and budgets.

## Milestones

| Milestone | Planned weeks | Exit criteria |
| --- | --- | --- |
| [M0 Reset](https://github.com/odvcencio/gotreesitter/issues/1293) | 0–2 | Complete R1–R4, R5a, R6, and R7. Bisect the edit regression and publish one baseline. Put counter and invariant ledgers in CI. Measure compact on cliff fixtures. Enable L0 guardrails. |
| [M1 Slim core](https://github.com/odvcencio/gotreesitter/issues/1294) | 2–8 | Release Q1–Q6; require receipts for each language affected by Q2 or Q6. Complete E-A0–E-A8 and E-C; pass the E-A exit gate. Pin C 0.27.x by commit and regenerate receipts. Generate O4 receipts for all 206 grammars. Reduce the root to 650 Go files. Q0 remains blocked by O-Q4; if authorized, keep or revert each language's experiment by week 4. |
| [M2 Cohort 1](https://github.com/odvcencio/gotreesitter/issues/1295) | 8–16 | Complete E-B, E-E1, and E-E7. Graduate cohorts 1a and 1b; remove every cohort-1 cliff. Reach zero top-50 invariant mismatches through R5b. Draft grammar format v1. Reduce the root to 550 Go files. |
| [M3 Top 50](https://github.com/odvcencio/gotreesitter/issues/1296) | 16–26 | Complete E-D, E-E2–E-E6, and E-F. Graduate cohorts 2 and 3. Ship D-A1–D-A3 deprecations. Obtain the owner's O-Q1 decision. Reduce the root to at most 500 Go files. |
| [M4 One engine](https://github.com/odvcencio/gotreesitter/issues/1297) | 26–32 | Complete cohort 4 through graduation or recorded known differences, subject to O-Q1. Complete E-H and delete legacy. Reduce the root to 25 Go files. Flip the global default in `v1.0.0-rc.1`. |

M1 also includes these work items:

- O1 and O2: build gates and test discovery.
- D-D1 and D-D3: the oracle upgrade and platform tests.
- L1 and L2: initial layout changes.

M2 adds tiered gates, the corpus store, and initial query work.
M3 adds API deprecations, grammar tooling, and the public dashboard.
M4 closes the remaining API and release requirements.
D-B compatibility adapters and D-E editor tooling follow v1.0.

### Lanes and sequence

Keep one active pull request (PR) per lane:

- Core: `sched`, `gss`, and `recover`.
- Incremental: `incr`.
- Output: `tree` and `compat`.
- Legacy fixes: Q items.

The design calls for a CODEOWNERS path check to enforce these lanes.
PRs that change only graduation allowlists and receipts do not occupy the core lane.

1. Pass the E-A exit gate before starting E-G graduation.
2. Freeze the comparison baseline at the default route from the M1 release tag.
3. Keep that baseline fixed when later Q fixes improve legacy.
4. Move an existing subsystem into `internal/` only after its engine phase closes under L5.
5. Start new engine code in `internal/` immediately.
6. Graduate stateful scanners only after E-E1.
7. Complete E-D1 before graduating languages whose factories use custom token sources.
8. Complete E-D6 for forest-default languages and E-D2 before moving included-range parses.
9. Flip the global default only after the top 50 graduate, in a release candidate.

R7 confirms or revises these cohorts:

- Cohort 1a: Go and Elixir.
- Cohort 1b: C#, PHP, Python, Markdown, and HTML, after E-E1.
- Cohort 2: the remaining top-20 languages.
- Cohort 3: the remaining top-50 languages.
- Cohort 4: the other 156 grammars.

Use the design's P1 membership, including its six replacements for the existing top-50 list.
Those replacements add Dockerfile, Protobuf, Groovy, Vue, Solidity, and MATLAB.
They move json5, gomod, ini, D, awk, and Elm to positions 51–56.

## Four gates

Run correctness before performance checks. Check deterministic counters before randomized timing.
Publish before-and-after counters in every engine PR body.
Keep the runtime pure Go and preserve strict locked-C parity under decisions 0001 and 0007.
Decision 0008 retains its collapse-class table-validity and safe-reuse requirements.
Known differences describe unresolved evidence; they do not authorize intentional divergence from C.

### Graduation gate: one language

Require all of these results on the candidate commit:

1. Generate a receipt with zero unexplained fresh mismatches against the C commit pinned at M1.
2. Pass the invariant gate for the 72-step session and 16 sites per edit class.
3. Match or exceed legacy reuse. Require positive reuse on clean steps once E-E1 covers the scanner class.
4. Record zero corpus declines, or give every decline an artifact deny entry and a reopening condition.
5. Keep tokens, new nodes, and maximum live versions at or below legacy on every fixture.
6. Keep the 20-seed median time at or below legacy at 32 KiB, 137 KiB, and 1 MiB.
7. Keep peak resident set size (RSS) at 1 MiB at or below legacy.
8. Pass crash, memory-budget, out-of-memory, and race checks in the dedicated container.
9. Preserve highlight and tags parity, including supertype and `MISSING` patterns.

Add the language to the allowlist in the PR that publishes its receipt.
This gate replaces the global certification sequence in #1064 and #1065.

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
R7 reports cliffs without blocking until graduation makes the detector mandatory for each language.

Keep existing failures in a checked-in exemption list that can only shrink.
Empty that list for cohort 1 at M2, the top 50 at M3, and all 206 at v1.0.
Ratchet Go/C, allocations per KiB, RSS, and incremental 99th-percentile latency at 10% above the 20-seed median.
The deterministic ledger rejects work increases or reuse decreases above 2% on any fixture.
The 2% threshold defines the ledger's failure tolerance; it does not replace D10's improvement direction.

The E-A exit gate precedes graduation.
Across the typical-file corpus for all 206 grammars, require median compact/legacy time at most 1.0 and worst at most 1.15.
Include declined files. Preserve the sealed Go results and reduce the route sanity test's `maxRatio` from 6.0 to 1.15.

| D13 layer | Scope | Directional targets for v1.0 |
| --- | --- | --- |
| Engine floor | All 206 | Median at most 3x C; no file above 5x; no cliff failures. Generic scanner certification supplies reuse. |
| Tuned | Top 50 | Median at most 2.5x C; no language above 4x; 99th-percentile edit latency at most 20 ms at 137 KiB. |
| Stretch | Top 20 | Full parse at most 2x C; edit latency at or below C. |

O-Q3 remains open. Do not turn the proposed v1.0 speed targets into release blockers without that decision.
The existing hard failures and ratchets remain mandatory.

### Invariant gate: every engine PR

Run this gate for every language the change can affect:

- Incremental parsing equals fresh Go parsing at every edit-session step.
- An `ERROR` root implies `HasError()`.
- The root covers the input, or the stop reason explains the gap.
- A reparse without edits allocates nothing.

Use a fresh parse where incremental equivalence remains unproven. This requirement does not wait for malformed-input C parity.
Run heavy parity suites in Docker, one language at a time.
Serialize local runs with `flock /home/draco/.local/state/nightwatch/gts-docker.lock <command>`.

### v1.0 release gate

- Graduate every top-50 language and delete legacy.
- Graduate every other grammar or record its known difference, subject to O-Q1.
- Check the D13 engine-floor and tuned speed targets; their release-blocking status awaits O-Q3.
- Require subtree reuse for every top-50 language and 99th-percentile edit latency at most 20 ms at 137 KiB.
- Reach recovery parity on at least 60 of 79 cases. Always match C's `HasError` result.
- Publish the v1 API, versioning policy, and grammar format v1.
- Publish the dashboard for all 206 grammars.
- Pass tests on all six D-D3 targets.

The six targets are Linux amd64/arm64, macOS arm64, Windows amd64/arm64, and `wasip1`.

## Open owner decisions

These decisions remain OPEN. Recommendations in the full design do not constitute approval.

| ID | Status | Owner question |
| --- | --- | --- |
| O-Q1 | OPEN | Delete legacy if some long-tail grammars do not graduate? Decide at M3 before authorizing that M4 outcome. |
| O-Q2 | OPEN | Add a second required reviewer? |
| O-Q3 | OPEN | Make the v1.0 speed targets release-blocking? |
| O-Q4 | OPEN | Authorize a two-week Q0 experiment on legacy merge admission? Q0 implementation remains blocked. |
| O-Q5 | OPEN | Accept an embedded JavaScript engine for `grammargen`, in a separate module? |
| O-Q6 | OPEN | Split grammars into separate modules? |
| O-Q7 | OPEN | Cap release cadence before v1.0? |

O11 also awaits the owner because it conflicts with the organization's prose decision 0011.
Keep `AGENTS.md` section 8 unchanged. Write plainly: lead with the point, use common words and the active voice, keep each term consistent, back claims with evidence (numbers, links, test output), and say what you did not verify.
This process change authorizes no release, tag, or release-note section.
