# Cache recovery visible-subtree counts

Date: 2026-09-05.
Baseline: `7db6e20ad88ac7f79228979899d5c55578818f49`.
Candidate: `bc34a57e549da5a077a5045927ae65bda8f285e2`.
Benchmark driver: `4e8d780c0dfecfff2b14850bc750e22b8ab15fa0`, from pull request #1089.

The pull request includes a later correction to the core dependency boundary.
It renames the cache API to `CachedVisibleSubtreeCount` and uses existing core symbol constants.
Malformed identifiers receive the core invalid-subtree error.
Valid-input counting remains unchanged. The recorded timing identities still refer to the measured candidate above.

## Result

The cache reduces time across all four frozen Go fixtures.
The largest fixture falls from 5.687 seconds to 203.5 milliseconds, a 27.94-times speedup.
Each timing change has twenty samples per side and a significance below 0.001.
C timing changes are not significant.

| Fixture | Baseline time | Candidate time | Time reduction | Baseline Go/C | Candidate Go/C |
| --- | ---: | ---: | ---: | ---: | ---: |
| `query_compile` | 56.30 ms | 19.13 ms | 66.02% | 10.84 | 3.77 |
| `rewrite` | 5.197 ms | 4.155 ms | 20.05% | 4.87 | 3.99 |
| `language` | 109.01 ms | 20.08 ms | 81.58% | 20.04 | 3.71 |
| `grammargen_lr` | 5687.3 ms | 203.5 ms | 96.42% | 101.35 | 3.65 |

Times are medians. Go/C values are median ratios within each process.
The C endpoint uses the locked Go grammar through the existing cgo harness.
These results do not establish standalone C performance or a universal language certificate.

| Fixture | Baseline bytes per parse | Candidate bytes per parse | Baseline allocations | Candidate allocations |
| --- | ---: | ---: | ---: | ---: |
| `query_compile` | 3,413,681 | 317,042 | 388,966 | 13,731 |
| `rewrite` | 301,622 | 210,411 | 16,319 | 5,349 |
| `language` | 7,767,901 | 700,860 | 885,523 | 19,720 |
| `grammargen_lr` | 434,011,048 | 6,257,339 | 52,741,156 | 186,185 |

The separate public-route comparison improves `query_compile` from 57.29 ms to 18.67 ms.
Every timed parse in that comparison reports one compact route and zero fallbacks.
The required generated Go trio shows no significant timing change.
Both incremental controls retain zero allocations.

## Cause and implementation

Recovery pauses record a cumulative visible-node baseline, including pauses on clean input.
The previous implementation repeatedly traverses completed subtrees and copies their child arrays.
Its query-compile profile assigns 63.47 percent cumulative CPU time to that traversal.
The pause-baseline path entered the repository on September 1 through `59bbd375`.

Cache exact subtree totals in the compact core.
Adjust the cached total for each occurrence's alias and root ERROR_REPEAT context.
Retain the original recovery baseline values and ownership behavior.

The implementation preserves:

- Extra nodes, aliases, missing nodes, and ERROR_REPEAT rules.
- Reused subtree materialization semantics.
- Rollback and identifier reuse.
- Reset and visibility-policy changes.
- Memory accounting and retention release.
- Separate cache storage for the diagnostic recovery clone.

The cache uses eight bytes per allocated count slot plus a copied visibility policy.
The slot capacity follows the compact subtree arena.
Storage and footprint accounting include both allocations.
The first traversal remains recursive.

## Correctness

Focused Docker controls pass before and after the runtime change.
Each grammar runs separately with one CPU and a four GiB memory limit.

- All four canonical Go fixtures preserve their exact locked-C tree digests.
- Go certification preserves three direct cases and one explicit recovery fallback.
- Python certification preserves four direct cases and one explicit recovery fallback.
- JavaScript certification preserves three direct cases and no fallback.
- All three grammar smoke edits preserve exact trees and subtree reuse.
- Count tests cover occurrence context, overflow, rollback, reset, reused nodes, and malformed metadata.
- The diagnostic recovery clone test confirms separate cache storage.
- Existing recovery and scheduler memory-budget controls pass.

Independent source review found no actionable correctness defect.
These controls do not constitute complete grammar certification or a broad race sweep.

Continuous integration subsequently found a reference-model dependency violation in the new cache.
The correction preserves the existing architecture check and removes those references.
Both complete compact-core package variants pass race tests in Docker after the correction.
The diagnostic clone and count controls also pass.
Independent review confirms unchanged counting behavior for valid input.

A broader historical diagnostic selection fails seven tests on unchanged `7db6e20a`.
Those failures concern conflict sequence overflow and old scheduler receipts.
The focused baseline controls used here pass.

Run the core regression controls:

```sh
bash cgo_harness/docker/run_parity_in_docker.sh -- \
  "cd /workspace && go test ./internal/parsercorephase0 -run '^TestCoreRecoveryVisibleCount' -count=1"
bash cgo_harness/docker/run_parity_in_docker.sh -- \
  "cd /workspace && go test ./internal/parsercorephase0 -tags gts_eof_recovery_shadow -run '^TestDiagnosticEOFRecoveryCloneDetachesVisibleCountMemo$' -count=1"
bash cgo_harness/docker/run_parity_in_docker.sh -- \
  "cd /workspace/cgo_harness && go test . -tags 'treesitter_c_parity gts_parsercorephase0' -run '^(TestCanonicalGoBenchmarkPreflight|TestStage6GoCompactCertification)$' -count=1"
```

## Measurement procedure

Use the paired driver from #1089 for each comparison.
Each pair uses the same shuffle seed and reverses checkout order between successive seeds.
One lock covers the complete campaign.

The measurements use:

- `GOMAXPROCS=1` and one process per seed.
- Twenty seeds, starting at one.
- `-count=1`, `-benchtime=750ms`, and `-benchmem`.
- Docker with four GiB memory and `GOMEMLIMIT=3GiB`.
- One CPU, pinned to CPU 2 on an Intel Core Ultra 9 285.
- Go 1.25.14 on Linux amd64.

Mount the clean baseline at `/baseline` and the candidate at `/workspace`.
Mount the paired driver at `/paired-driver.sh` and an empty output directory at `/evidence`.
Run this command inside the candidate container:

```sh
cd /workspace/cgo_harness
bash /paired-driver.sh \
  --baseline-root /baseline/cgo_harness \
  --baseline-output /evidence/canonical-base.txt \
  --output /evidence/canonical-head.txt \
  --runs 20 --seed-start 1 --benchtime 750ms \
  --tags 'treesitter_c_parity,gts_parsercorephase0' \
  --bench-regex '^BenchmarkParityGoCanonicalFull$' \
  --require-benchmarks 'BenchmarkParityGoCanonicalFull/query_compile/gotreesitter,BenchmarkParityGoCanonicalFull/query_compile/tree-sitter-c,BenchmarkParityGoCanonicalFull/rewrite/gotreesitter,BenchmarkParityGoCanonicalFull/rewrite/tree-sitter-c,BenchmarkParityGoCanonicalFull/language/gotreesitter,BenchmarkParityGoCanonicalFull/language/tree-sitter-c,BenchmarkParityGoCanonicalFull/grammargen_lr/gotreesitter,BenchmarkParityGoCanonicalFull/grammargen_lr/tree-sitter-c'
benchstat /evidence/canonical-base.txt /evidence/canonical-head.txt
```

Require `# status: complete` as the final line in both outputs.
The fixture preflight authenticates source hashes, grammar identity, deep trees, and compact routing.
The work-count instrumentation tag is absent from timing builds.
Zero-valued legacy scheduler metrics do not describe compact scheduler work.

The host is a shared development machine.
Paired order and CPU pinning reduce bias but do not establish an isolated release measurement environment.
Container Git metadata was unavailable. Companion host receipts record the clean source identities.
The first trio comparison used the exact staged runtime patch before its commit.
The canonical comparison used the clean candidate commit.

## Memory and residual profile

A bounded largest-fixture probe excludes compilation from its maximum resident set size measurement.
The probe includes the canonical preflight, warmup, one parse, and tree release.
Baseline peak memory is 178,316 KiB. Candidate peak memory is 149,920 KiB.
Both processes exit successfully without a container memory failure or wall timeout.
This single probe establishes a bounded observation, not a statistical memory improvement.
Its elapsed times are not comparison evidence.

The new query-compile profile assigns 0.71 percent CPU time to cached subtree traversal.
Residual observations include:

| Component | CPU share |
| --- | ---: |
| Active dispatch, cumulative | 55.34% |
| Generic reduction, cumulative | 28.89% |
| Structure copying through `runtime.duffcopy`, flat | 17.27% |
| Scheduler footprint accounting, cumulative | 10.52% |
| DFA token scan, cumulative | 8.79% |
| Canonicalization with its scheduler wrapper, cumulative | 5.89% |

These shares overlap. Do not add them.
The allocation profile includes setup and assigns 48.83 percent of sampled bytes to canonicalization.
Use the profile to select the next experiment; do not use profiled timing as speed evidence.

Candidate Go/C ratios still require approximately 45 to 50 percent runtime removal to pass below two.
Canonicalization elision alone cannot close that gap on this profile.
Measure repeated scheduler work, structure copying, and footprint scans before selecting another runtime change.
Preserve exact memory budgets and ownership publication in any execution-region experiment.

## Receipts

The [receipt directory](recovery-visible-count-2026-09-05/) contains raw samples, source identities, container settings, memory output, and the residual profile.
Verify its files with `sha256sum -c SHA256SUMS` from that directory.
Read the [canonical comparison](recovery-visible-count-2026-09-05/canonical-benchstat.txt) and the [required trio comparison](recovery-visible-count-2026-09-05/trio-benchstat.txt).
