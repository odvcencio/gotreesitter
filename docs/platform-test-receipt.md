# D-D3 weekly platform tests

Issue [#1326](https://github.com/odvcencio/gotreesitter/issues/1326) requires weekly test execution and published results on six targets. At the starting revision, `8a78a7673e502b9d3eb1ae7b7f99328b951b3976`, CI built WASI but had no weekly six-target test matrix. The workflow coverage regression fails when the new workflow is absent.

`.github/workflows/platform-weekly.yml` runs every Monday at 06:23 UTC. It also supports manual dispatch and runs when its workflow or tests change. The schedule becomes active after integration into the default branch. It builds all root-module packages for each target, then executes a small Go-grammar runtime suite. It checks fresh parse acceptance and root coverage, recovery error reporting and coverage, complete incremental/fresh tree equality after an edit, and allocation-free reparsing without edits.

Five targets execute on native GitHub-hosted runners. WASI executes through Go's `go_wasip1_wasm_exec` and Wasmtime 36.0.2, whose archive digest is checked before installation. Each job verifies the runtime target, checks that the test actually passed, and publishes a summary and a 30-day artifact containing toolchain, build, and test output. Failures do not cancel the other targets. The matrix permits three concurrent jobs and caps each job at 15 minutes.

Runner labels follow the [GitHub-hosted runner reference](https://docs.github.com/en/actions/reference/runners/github-hosted-runners). WASI execution uses the [Go WASI test wrapper](https://go.dev/lib/wasm/go_wasip1_wasm_exec).

## Hosted verification

Candidate: `06883319cf6e7715ec5a9ec1bb625b52b78db664`.

[Run 36662049347](https://github.com/odvcencio/gotreesitter/actions/runs/36662049347) passed all six targets with Go 1.25.0. Each artifact contains the runtime test and its three passing subtests, with no test skips. Weekly coverage changes from **0/6 to 6/6 targets**.

| Target | Runner | Result | Job seconds | Test package seconds |
| --- | --- | --- | ---: | ---: |
| Linux amd64 | ubuntu-24.04 | pass | 57 | 0.025 |
| Linux arm64 | ubuntu-24.04-arm | pass | 23 | 0.021 |
| macOS arm64 | macos-14 | pass | 51 | 0.023 |
| Windows amd64 | windows-2022 | pass | 49 | 0.039 |
| Windows arm64 | windows-11-arm | pass | 109 | 0.057 |
| wasip1 wasm | ubuntu-24.04 / Wasmtime | pass | 64 | 7.150 |

Total job time was 353 runner-seconds (5.88 minutes), before any billing rounding or platform multiplier. These test times include assertions and, for WASI, runtime startup. They are not parser performance benchmarks.

The [first run](https://github.com/odvcencio/gotreesitter/actions/runs/36661816938), at `06888ad90bcc1015f2af8c50823c53e5847c18bb`, passed 5/6 targets. macOS Bash 3.2 rejected expansion of an empty argument array under `set -u` before tests executed. The follow-up commit uses positional arguments for the optional WASI executor. The revised run passed macOS and all five other targets.

## Local gates and limits

Passed:

- Native Linux and Wasmtime execution of `TestPlatformRuntime`.
- `GOWORK=off go test ./cmd/citestplan -count=1`.
- `GOWORK=off go run ./cmd/citestplan` and `-stale-run-names` (59 selectors, no stale names).
- `actionlint` 1.7.12, repository hygiene, and `git diff --check`.
- Docker `TestParityGLRCanaryGo` against the currently locked C runtime, 0.25.1 at `f5afe475deb7c0bae6407fb776c76824f717bb61`.
- Docker Go counter collection: both route rows exactly match the checked-in ledger, including fixture/session identities, completion, root spans, and error state.

| Route / operation | Tokens before → after | New nodes before → after | Max versions before → after | Reused bytes before → after | Splices before → after |
| --- | ---: | ---: | ---: | ---: | ---: |
| Default / full | 589 → 589 | 1,922 → 1,922 | 8 → 8 | 0 → 0 | 0 → 0 |
| Default / edit | 574 → 574 | 3,804 → 3,804 | 12 → 12 | 1,162 → 1,162 | 294 → 294 |
| Candidate / full | 589 → 589 | 1,251 → 1,251 | 3 → 3 | 0 → 0 | 0 → 0 |
| Candidate / edit (legacy fallback) | 374 → 374 | 1,545 → 1,545 | 8 → 8 | 1,784 → 1,784 | 266 → 266 |

The before values are the checked-in ledger; the after values are the collected candidate rows. All counters and pins remain unchanged. No production runtime file changes, so no Go/C time or memory effect is claimed and no randomized performance comparison was needed.

The requested Docker `GOWORK=off go test ./... -count=1 -timeout=20m` did **not** pass or complete. Its root package ran for 579.467 seconds and reported one failing assertion: `TestCompactRouteLifecycleAllowsStaleReceiptOutsideCurrentLineage` could not access Git metadata referenced by the mounted worktree. The root batch reported no invariant-ledger, no-edit-allocation, or `ERROR`-root assertion failures. It also reported an existing optional real-corpus gate as unprovisioned. Later package builds ran out of disk space, and the wrapper exited 141 without an OOM or timeout.

The second and final permitted local Docker run passed the Go canary and counter collection. Disk exhaustion blocked separate builds for the Go invariant/no-edit tests, the `ERROR`-root test, and the Go fresh/incremental/error-state parity tests. Those focused reruns are not claimed as passes. A clean broad gate still needs sufficient temporary disk space, the worktree's Git metadata mounted at the path referenced by its `.git` file, and provisioning for the optional real-corpus gate. Neither local run changed a pin, threshold, test expectation, or runtime file.

The six-target CI requirement is verified. Integration into the default branch remains necessary to activate its weekly schedule. No PR was opened because the owner's autonomy rules assign integration to the lead.
