## Working in gotreesitter

This file is for everyone who changes this repository: contributors, coding agents, reviewers, and the owner.
It says where the project is going, how to make a change safely, what a pull request (PR) must show, and which decisions belong to the owner.
[CONTRIBUTING.md](CONTRIBUTING.md) covers setup and first steps. [docs/repository-map.md](docs/repository-map.md) says where things live.

The correctness and performance requirements apply to everyone.
The Canopy, Buckley, and Hyphae steps apply to maintainers who have those tools configured. Other contributors may use `rg`, direct source reads, and signed plain-Git commits, and put their results in the PR body; maintainers archive raw evidence as section 6 describes.

### Where the project is going

gotreesitter is a tree-sitter runtime in pure Go, with no CGo. It loads the same grammars as the C runtime and aims for the same trees, on every target Go builds for.

The owner's [v1 design](docs/v1-design.md) governs engine work and supersedes conflicting campaign guidance.
The full design is `hypha://m31labs/gotreesitter/specs/spec.gotreesitter-v1-design`.
The owner's [decision of 2026-11-10](docs/v1-design.md#decision-of-2026-11-10-a-c-faithful-core) sets the engine plan:

- `internal/ccore`, the C-faithful core, becomes the only engine. Put new engine work there.
- The compact scheduler (`internal/parsercorephase0` and the root `parsercore_phase0*.go` files) is frozen. Do not extend it.
- The legacy engine gets Q fixes only.
- Every grammar that the legacy engine supports graduates to the core. Then the legacy engine and the compact scheduler are deleted.

[docs/roadmap.md](docs/roadmap.md) says what is done and what comes next. Put status there and in PRs, not in this file.

### Rules that always hold

- For parser performance changes, pass focused correctness and pinned-C parity checks before collecting performance evidence, both before and after the change.
- Add no language-name comparisons in engine files (D6). `go run ./cmd/v1guard` checks this.
- Start new engine code in `internal/` (D9). New root Go files must be named in `adapterFiles` in `cmd/v1layout/main.go`. Run `go run ./cmd/v1layout -base <base>` with the PR base to check additions against that base; an intentional move or adapter addition must also update the snapshots and the budget.
- Require incremental parsing to equal fresh Go parsing (D8). Use a fresh parse wherever this invariant remains unproven.
- Check deterministic counters before randomized timing (D10). Work counters must not rise; reuse counters must not fall.
- Put before-and-after counters in every engine PR body. Apply the design's 2% ledger threshold.
- Do not suppress a failing gate, a required check, or a required signature to make a PR pass. Follow only the exception processes that the governing policy allows, and record their approval and scope.
- Keep evidence out of the repository (section 6).

Run the invariant gate for every language that an engine PR can affect:

- Incremental parsing equals fresh parsing at every edit-session step.
- An `ERROR` root implies `HasError()`.
- The root covers the input, or the stop reason explains the gap.
- A reparse without edits allocates nothing.

Graduate each language through the allowlist after the E-A exit gate.
Flip the global default only after all top-50 languages graduate, and only in a release candidate (D7).
The owner decided O-Q1 through O-Q7 on 2026-09-26; see the owner decisions in [the v1 design](docs/v1-design.md#owner-decisions). Every grammar that the legacy engine supports graduates before legacy is deleted. Section 8 implements O11 under organization decision 0012.

### Lanes

Keep one active PR per lane:

- Core: `internal/ccore` and the root adapters `ccore_*.go`.
- Grammars: `grammargen`, `cmd/ts2go`, grammar blobs, and scanner ports.
- Legacy fixes: Q items.

PRs that change only graduation allowlists and receipts do not occupy the core lane.

### Kinds of change, and when each is done

| Change | Where | Done when |
| --- | --- | --- |
| Core engine | `internal/ccore`, root `ccore_*.go` | The core preset (section 7) passes, including the package tests with `-race`. Work counters do not rise and reuse counters do not fall, under the 2% ledger threshold and the R3 exception process. |
| Scanner port | scanner files under `grammars/` (`grammars/*_scanner*.go`, `grammars/runtime/`, `grammars/internal/`) | The port does what the pinned C scanner does, and a comment cites the C `file:line`. A test without cgo pins the behaviour. The grammar's parity suite does not regress. |
| Grammar generation | `grammargen`, `cmd/ts2go`, grammar blobs | The smallest Docker regression test passes, and so do the one-language real-corpus and C parity focus targets that cover the change. The PR states any unsupported target and anything not verified. No blob changes unless the PR says why. |
| Graduation | allowlists and receipts | The PR body has the results of the [graduation gate](docs/v1-design.md#graduation-gate-one-language) for each grammar. |
| Legacy fix | root parser files | The grammar's parity suite and the invariant gate pass. Counters and timing follow sections 2 and 3. |
| Documentation | `docs/`, root `*.md` | After you stage the intended files, `bash scripts/check_evidence_paths.sh` passes, and the PR's `evidence paths` check passes in CI. Each claim has a source a reader can check. |
| Release | `CHANGELOG.md`, `README.md` | [docs/releasing.md](docs/releasing.md) is followed step by step (section 10). |

Facts that a first change often misses:

- A core change follows the function of the pinned C tree-sitter 0.27.1 runtime that it corresponds to, and cites `C/<file>:<line>` in the function comment. `internal/ccore` imports only the standard library.
- In a Go scanner, index `validSymbols` by external-token index, but pass `lang.ExternalSymbols[i]` to `SetResultSymbol`: the result takes a language symbol ID. See [docs/external-scanners.md](docs/external-scanners.md).
- A scanner port must preserve the pinned C scanner's behaviour and its decisions about when two states are equal. Reuse checkpoints must encode all state that can affect later scans, restore it exactly, and fail closed when the complete state cannot be represented.
- Enable `SupportsIncrementalReuse` only after you certify the scanner's reuse behaviour. Enable `UsesExternalScannerCheckpoints` only when every non-empty checkpoint encodes all state that affects later scans. Otherwise keep the fresh-parse fallback.
- Before you regenerate a shipped blob, check who owns it and its recipe (`grammars/grammar_ownership.go`, [docs/grammar-ownership.md](docs/grammar-ownership.md)). A blob that `grammargen` owns is built with `cmd/grammargen` from the documented recipe, including its LR-splitting setting, and must pass its case of `TestGrammargenOwnedBlobsAreReproducible`.

### What a PR must show

1. What changed and why, in the first lines.
2. Testing: each command and its result. For a regression fix, verify that the regression test fails on the base commit and passes with the fix; report both results.
3. Evidence: numbers with their source. An engine PR gives counters before and after.
4. Risk, and what was not verified.
5. Signed commits, a branch that is up to date with `main`, and the `build` check green on the exact head.

Reviewers check the same five points. Review the claim against the code, not against the PR text.

### Decisions that belong to the owner

Ask; do not decide these yourself.

- An exception to the counter ledger (R3, [v1 design](docs/v1-design.md#r3-counter-ledger-changes)). Agents never approve one.
- A known difference from C for a grammar (O-Q1). It requires proof that the grammar cannot graduate, a witness, the blocking mechanism, the reason no generic mechanism serves it, and the owner's approval. It does not authorize intentional divergence from C.
- A change to the approved plan for the default switch and the engine deletions. The approved plan flips the default in `v1.0.0-rc.1` after the top 50 graduate, and deletes legacy and the compact scheduler after every grammar that legacy supports has graduated to the core.
- Publishing a release. It requires the owner's approval of the protected `release` environment. While the tag-creation rule gap remains without a standing owner decision, it also requires the owner's separate authorization of that version's tag-rule exception ([docs/releasing.md](docs/releasing.md)).
- A change to the v1 design's gates or decisions.

### 1) Non-negotiables
- Do not run repository-wide `go test ./...`, `go test ./... -race`, or `go build ./...` on the host. Broad host sweeps can OOM developer machines and make attribution harder.
- For heavy correctness, parity, or race coverage, use Docker isolation only and keep runs to one language/grammar at a time.
- Keep correctness gating separate from performance gating.
- Prefer reproducible runs over ad-hoc spot checks.
- When chasing an OOM, keep narrowing the workload until one language and one suite remain.
- Use `rg` for literal text. Plain `canopy search grep` patterns start structural parsing.
- Use `scripts/canopy_query.sh` for cached structural searches and analyses.
- Use direct `canopy ... --no-cache` commands for narrow searches of changed files.
- Scope structural Canopy searches to the smallest useful path.
- Apply `--limit` to structural searches.
- Do not run concurrent repository-wide Canopy searches.
- The query wrapper detects changed, missing, and new source files.
- The query wrapper uses a fresh scoped query when the cache is stale.
- Run `scripts/refresh_canopy_index.sh` before a broad search when the source set changed.
- The refresh script promotes its temporary cache only after a successful build and validation.

### 2) Correctness Gate (must stay green)
Run before and after performance changes:
- Focused package/unit tests inside Docker, scoped with `-run` whenever possible.
- Parity-focused Docker suites under `cgo_harness`, one language at a time.
- For a change to `internal/ccore` or the root adapters: the core preset (section 7).

When you change GLR/incremental logic, validate parity first, then validate performance.

### 3) Standard Perf Loop
Use this loop for optimization work:
1. Baseline with stable settings.
2. Make one focused change.
3. Pass correctness and invariant gates, then compare deterministic counters.
4. Re-run the same randomized benchmarks and compare results with `benchstat`.
5. Apply the [v1 hard gates and ratchets](docs/v1-design.md#performance-gate-one-language). Record directional regressions and their tradeoffs.

Stable settings:
- `GOMAXPROCS=1`
- `-count=1` per process
- one process per explicit shuffle seed
- 20 shuffle seeds by default
- `-benchtime=750ms`
- `-benchmem`

Use `scripts/run_randomized_benchmarks.sh` for every before-and-after
comparison. Do not use a fixed-order benchmark run as comparison evidence.

Primary bench trio:
- `BenchmarkGoParseFullDFA`
- `BenchmarkGoParseIncrementalSingleByteEditDFA`
- `BenchmarkGoParseIncrementalNoEditDFA`

These benchmarks measure the existing public routes. Until those routes reach `internal/ccore`, performance evidence for the core must select the `BenchmarkCcore` benchmarks in the `cgo_harness` module, set `CCORE_LANG`, use the tags `cgo treesitter_c_parity`, and run through the randomized driver inside Docker.

### 4) Metrics and Targets
Track at minimum:
- `ns/op`
- `B/op`
- `allocs/op`
- Max RSS on large-file runs (`/usr/bin/time -v`)

Release-blocking performance safety requirements:
- Do not permit a crash, an out-of-memory failure, or runaway memory retention.
- Preserve the parser memory-budget contracts.
- Complete the benchmark suite and publish reproducible evidence (in the PR body and the `evidence` release; see Commit Discipline).

Use the [v1 performance gate](docs/v1-design.md#performance-gate-one-language)
for hard failures and per-language ratchets.
Track work counters and reuse counters before timing.
D13 defines three target layers:

- An engine floor for all 206 grammars.
- Tuned targets for the top 50.
- Stretch targets for the top 20.

The `2x` full-parse goal and edits at or below C apply to the top-20 stretch layer.
The owner decided O-Q3: the v1.0 speed targets block the v1.0 release only.
Hard failures and ratchets remain mandatory. Record every directional regression and explain its tradeoff.

### 5) Attribution for Incremental Hot Path
When profiling incremental edits, split attribution into:
- `Tree.Edit(edit)`
- reuse-cursor/reuse-selection work
- reparse/rebuild work

Use profiled runs to decide whether the next win comes from:
- reuse/invalidation scope,
- GLR/recovery/materialization cost,
- or allocator sizing/retention.

### 6) Commit Discipline
- Keep commits scoped and bisectable.
- Drop scratch/debug artifacts before commit.
- Sign every commit. Use squash merges when landing PRs on `main`.
- Use project commit flow:
  - `git add ...`
  - `buckley commit --yes --minimal-output`

Replace `...` in `git add ...` with the paths you intend to commit, and `<file>` in the upload command below with the path of the local file.

#### Evidence stays out of the repository
Do not commit receipts, screens, censuses, certificates, raw logs, profiles, or benchmark archives.

1. Put the numbers and the verdict in the pull request (PR) body.
2. Attach raw files to the GitHub release named `evidence`:
   `gh release upload evidence <file> --repo odvcencio/gotreesitter`.
   Pack a folder of files into one `.tar.gz` first.
3. Store written receipts in the Hyphae space `hypha://m31labs/gotreesitter` under `reports/receipts/`.
4. Link to a release asset or a pinned commit instead of copying its content into a doc.

The repository keeps user-facing docs, test inputs, and generated files that a test checks.
CI (`.github/workflows/repository-hygiene.yml`) runs `scripts/check_evidence_paths.sh`.
It rejects new `docs/benchmarks/` files, dated or receipt-named docs, archives and raw logs under `docs/`, and `docs/` files over 200,000 bytes (except `docs/changelog/`).
`scripts/evidence_paths_allowlist.txt` lists the few existing files that wait for retirement. It may only shrink.

### 7) Gate Presets
The commands below contain placeholders; do not type the angle brackets.
Replace `TestName` with the name of an existing regression test, and confirm that it ran.
Replace `<grammar>` with a name that `bash cgo_harness/docker/run_single_grammar_parity.sh --list` prints; that runner uses the names `c_lang` and `go_lang`.
Replace `<language>` with a target that the focus runner supports; it accepts `c`, and `go` is not a supported target. Fortran supports the real-corpus mode only.

Correctness preset:
- `bash cgo_harness/docker/run_parity_in_docker.sh -- "cd /workspace && go test ./grammargen -run '^TestName$' -count=1"` for the smallest regression that covers the change
- `bash cgo_harness/docker/run_single_grammar_parity.sh <grammar>`
- `bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode real-corpus --langs <language>`
- `bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode cgo --langs <language>`
- Do not batch multiple languages together while diagnosing regressions or OOMs.

Core preset, for `internal/ccore` and the root adapters:
- Run the equality lane of each supported language that the change can affect, one container per language. This JSON example runs as written; replace `json` in both places with `python`, `c`, or `typescript` as needed:
  `bash cgo_harness/docker/run_parity_in_docker.sh --label ccore-json --wall-timeout 45m -- "bash /workspace/cgo_harness/docker/ccore_equality.sh json"`
  The script compares parse traces line by line, visible-tree digests, and subtree-record hashes with pinned C. Comparisons for the languages other than JSON normalize the spelling of symbol names. It runs incremental chains for JSON, Python, and TypeScript; the C edit-session inputs are parsed fresh. It also checks the core's table lookups against the root's, scanner isolation, and the JSON goldens. The CI job `ccore-equality-cgo` runs all four languages.
- `bash cgo_harness/docker/ccore_equality.sh --self-test` proves that a skipped or misspelled test fails the script.
- `bash cgo_harness/docker/run_parity_in_docker.sh -- "cd /workspace && go test -race ./internal/ccore/... -count=1 -timeout=10m"` runs the core's package tests with the race detector.
- `go run ./cmd/v1guard` checks language-name uses and environment reads against checked-in baselines. `go run ./cmd/v1layout` checks the count of root Go files, the file and API snapshots, and build-tag coverage. For a PR, pass the PR base through `-base` to both commands. `go run ./cmd/citestplan` checks that each test package of the root module that the host's race build can see has a declared CI lane; run `go run ./cmd/citestplan -stale-run-names` as well to check the test names that workflows refer to. Run `citestplan` on a host that can build with the race detector and has the C toolchain, or rely on CI's `compile` job.
- For a code PR, CI uses the `.github/scripts/editor_latency_scope.sh` of the PR base. While `.github/ccore-unrouted` exists, CI skips the `editor_latency` job only for a non-empty diff that lies entirely in the script's skip set: `internal/ccore/`, the `cgo_harness/ccore_*_test.go` files, the files directly in `cgo_harness/internal/corecords/`, `cgo_harness/docker/ccore_equality.sh`, `docs/`, root Markdown files, and `LICENSE`. The root adapters and harness fixtures are outside that set. A change to the marker, to the scope script, or to `ci.yml` runs the job. Delete the marker in the PR that first routes a grammar to the core by default.

Race preset:
- CI or dedicated-container only.
- Do not run host-side repo-wide race sweeps while diagnosing OOMs.

Perf preset (stable settings):
- `bash scripts/run_randomized_benchmarks.sh --output /tmp/gotreesitter-bench.txt`

Full-parse non-truncation probe:
- `GOT_PARSE_NODE_LIMIT_SCALE=3` may be used for diagnostic full-parse runs when default node budget truncates benchmark/parity cases.
- For diagnostic runs, set `GOT_GLR_MAX_STACKS` to a positive integer in the command's environment, for example `GOT_GLR_MAX_STACKS=16`, to override the default GLR stack cap of 8.

### 8) Prose Standard: Plain language

Write plainly: lead with the point, use common words and the active voice, keep each term consistent, back claims with evidence (numbers, links, test output), and say what you did not verify. M31 agents: see decision 0012 and the `writing-plainly` skill in hypha://m31labs/hyphae.

Use numbers from pinned benchmarks or CI runs as receipts. Cite them in the PR body; do not commit them.

### 9) Lessons that are now rules

1. For a core change, compare parse traces, visible trees, subtree records, and the supported incremental sessions with pinned C. These checks complement each other; passing one does not replace the others.
2. Give every new lexer, scanner, or recovery rule a test with a timeout. Include empty input and input without a final newline. A count of differing states does not show a hang.
3. A rule that infers what a generated C lexer does from the grammar blob is a heuristic. Mark it `HEURISTIC`. Say in the PR how many pinned lex states it was checked on and how many differ.
4. A gate must fail if a test that its verdict needs is skipped, matches no test, or compares no inputs. Inputs may be committed fixtures, literals, or deterministic generated inputs. List file fixtures by name, and fail if a listed file cannot be read.
5. Do not weaken a gate to make the same PR pass. A change that repairs or strengthens a gate must test its passing and its failing cases. Record any exception that the governing policy authorizes.

### 10) Releases

Follow [docs/releasing.md](docs/releasing.md), including these checks:

1. The date in the candidate's changelog heading must equal the `America/Los_Angeles` date when verification and when publication each collect their evidence. If the date changes before publication, update the candidate documents and repeat the required validation.
2. Nothing may merge to `main` between the full manual CI run on the candidate commit and the tag.
3. Publication requires the owner's approval of the protected `release` environment, and the version's tag-rule exception while `docs/releasing.md` requires one.
