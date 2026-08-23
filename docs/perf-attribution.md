# Compact-lane perf attribution board

This document defines one boundary-exact attribution tree over the compact
parser core's fresh-full-parse central processing unit (CPU) time. It states
the disambiguation rule for
every function that more than one component could otherwise claim. It
records the noise floor of the local measurement host, and the first
published receipt.

This is measurement infrastructure only. It changes no parser code, no
routing, and no shipped behavior.

## 2026-08-23 P25aw Swift incremental capacity witness

Status: **NO-GO / NO WITNESS**. Keep the Swift external scanner reuse gate
closed. Do not change `nodeArena.ensureNodeCapacity` for Swift.

P25aw used clean base commit
`af9ded2b77b7828b12b1d2da7c9fff8dd5ca053b`. The route test manifest is
`testdata/swift_ternary_retirement_cases_v1.json`. Its SHA-256 is
`49f57837686d0ea7e070cd08e14ab05dc1b7c128a540ae5aa6c155435a6e18e9`.
The authenticated Swift corpus manifest is
`grammars/testdata/swift_corpus/MANIFEST.md`. Its SHA-256 is
`1a48d83c37f3596fecb53ec34e277aa62d1e41d36749db8b97a0468a31e2a4ec`.
The corpus pins Swift 6.3 and Swift Algorithms 1.2.1 sources.

Each case parsed an old source, applied one final-newline edit, and called
`ParseIncrementalProfiled` with that edited old tree. The probe covered four
large pinned Swift files.

| File | Old bytes | New bytes | Reuse status | Reused subtrees | Reused bytes |
| --- | ---: | ---: | --- | ---: | ---: |
| `stdlib_Collection.swift` | 66,940 | 66,941 | unsupported: external scanner | 0 | 0 |
| `stdlib_CollectionAlgorithms.swift` | 24,055 | 24,056 | unsupported: external scanner | 0 | 0 |
| `stdlib_FloatingPointToString.swift` | 104,680 | 104,681 | unsupported: external scanner | 0 | 0 |
| `stdlib_Optional.swift` | 33,866 | 33,867 | unsupported: external scanner | 0 | 0 |

All four cases reported `OldTreeReuseRoute=false`. All four cases reported
`ReuseUnsupported=true` and `ReuseUnsupportedReason=external_scanner_unsupported`.
All four cases reported `valid_witness=false`. The edited roots had errors.

The Swift scanner type has no `SupportsIncrementalReuse` method. The parser
therefore refuses incremental reuse with reason `external_scanner_unsupported`.
`parseIncrementalInternal` calls `incrementalTokenSourceFreshFullParse` after
this refusal. That helper selects `arenaClassFull`, which uses
`ensureExactNodeCapacity`. It does not set `Tree.incrementalReuseDisabled` or
call `Parse`.

The route proof gives zero calls to the hotspot. The four cases made zero
reuse progress. They are route controls, not allocator witnesses. The existing
16-case Swift ternary test also passed its route classification check.

The P25aw gate requires a valid old-tree edit, nonzero reuse, and nonzero
`ensureNodeCapacity` calls. No case met these requirements. Do not run a
six-seed screen, the required 20-seed campaign, or a maximum resident set
size (RSS) campaign. No sizing candidate was tested.

Source identities are:

- Swift scanner: `219941b1ff0bcb0defa9e475075d13cf929a2554acd0d19a5e76cacdd8f97147`.
- Route test manifest (`testdata/swift_ternary_retirement_cases_v1.json`): `49f57837686d0ea7e070cd08e14ab05dc1b7c128a540ae5aa6c155435a6e18e9`.
- Swift corpus manifest (`grammars/testdata/swift_corpus/MANIFEST.md`): `1a48d83c37f3596fecb53ec34e277aa62d1e41d36749db8b97a0468a31e2a4ec`.
- `stdlib_Collection.swift`: `7ccfed56194d956b47724c935ea3a6e1abcdddbfa23f440a9c19a3e5eab03c6c`.
- `stdlib_CollectionAlgorithms.swift`: `1aae0051b0bfb50e17c7ac94961ee7cab7332367dcc16e827d2482be7a2dc5a1`.
- `stdlib_FloatingPointToString.swift`: `ec96801e5237dff8da773f617a8a2f36e95b6a0a7c94b581855a451cd6507fdc`.
- `stdlib_Optional.swift`: `06e7d03986b079ced51e8f918c2ef63ecb1f1e33b1dfb20744ea61aa6e699eb6`.

The focused Docker receipts are:

- Existing Swift ternary route test: `/tmp/gts-p25aw-artifacts-aw/20260823T204343Z-swift-existing-ternary/container.log`, SHA-256 `909f0cd5e8a00c9a8df7503b3564ae2d5dff386e849b144aa8333a4c60a44054`.
- Existing test metadata: `/tmp/gts-p25aw-artifacts-aw/20260823T204343Z-swift-existing-ternary/metadata.txt`, SHA-256 `66db6fb229833632eed5ee18164dcafaadf40b30a82a0f480084c12ed20cd11a`.
- Four-file probe: `/tmp/gts-p25aw-artifacts-aw/20260823T204617Z-swift-incremental-probe/container.log`, SHA-256 `169a82520d75631081eb02d5351772591e72afe3e364d63f525279f8a59711d1`.
- Probe metadata: `/tmp/gts-p25aw-artifacts-aw/20260823T204617Z-swift-incremental-probe/metadata.txt`, SHA-256 `b5ae4abbf8385e2883668d33726cf97fe6ba9553314c1bd011d0679c2c80d42c`.
- Probe inspection: `/tmp/gts-p25aw-artifacts-aw/20260823T204617Z-swift-incremental-probe/inspect.json`, SHA-256 `a6a6e7667d0a042709eea653d2b341df84a0a8811909806a8ba145721fbc738`.

P25aw used these structural commands:

```text
scripts/canopy_query.sh search symbols cgo_harness --name 'Swift|swift|Incremental|incremental|Reuse|reuse|Edit|edit' --limit 200 --json
scripts/canopy_query.sh search refs ParseIncrementalProfiled cgo_harness --limit 200 --json
scripts/canopy_query.sh search refs ParseIncremental cgo_harness --limit 300 --json
scripts/canopy_query.sh search refs SupportsIncrementalReuse grammars --limit 200 --json
canopy search refs SupportsIncrementalReuse grammars/swift_scanner.go --no-cache --limit 20 --json
canopy search symbols parser.go --name 'incrementalTokenSourceFreshFullParse|parseIncrementalInternal' --no-cache --limit 20 --json
canopy search refs incrementalTokenSourceFreshFullParse parser.go --no-cache --limit 20 --json
canopy search refs ensureExactNodeCapacity parser.go --no-cache --limit 20 --json
canopy search refs ensureNodeCapacity parser.go --no-cache --limit 20 --json
```

The correctness commands were:

```text
bash cgo_harness/docker/run_parity_in_docker.sh --repo-root /tmp/gts-p25aw-swift-capacity-20260823 --out-root /tmp/gts-p25aw-artifacts-aw --label swift-existing-ternary --no-build --memory 8g --cpus 1 --goflags '-p=1' --test-parallel 1 --timeout 30m -- "cd /workspace && GOMAXPROCS=1 go test ./grammars -run '^TestSwiftTernaryRetirementRoutes$' -count=1 -parallel 1 -timeout 20m -v"
bash cgo_harness/docker/run_parity_in_docker.sh --repo-root /tmp/gts-p25aw-swift-capacity-20260823 --out-root /tmp/gts-p25aw-artifacts-aw --label swift-incremental-probe --no-build --memory 8g --cpus 1 --goflags '-p=1' --test-parallel 1 --timeout 30m -- "cd /workspace && GOMAXPROCS=1 go test . -run '^TestP25awSwiftIncrementalProbe$' -count=1 -parallel 1 -timeout 20m -v"
```

The document checks were:

```text
mdpp parse --json docs/perf-attribution.md
mdpp lint --json docs/perf-attribution.md
mdpp fmt --check docs/perf-attribution.md
mdpp parse --json CHANGELOG.md
mdpp lint --json CHANGELOG.md
mdpp fmt --check CHANGELOG.md
git diff --check
```

Both parse checks passed. Lint and format checks returned the same existing
baseline warnings and formatting status. The final diff check passed.

The temporary probe and test file were removed. No parser or test change
remains. No benchmark or RSS artifact exists because no valid witness exists.

Reopen P25aw only after the Swift scanner certifies incremental reuse, an old
tree edit reports nonzero reuse, and the parser preserves parity.

## 2026-08-23 P25at `nodeArena.ensureNodeCapacity` screen

Status: **NO-GO / REVERTED**. Keep the geometric growth loop in
`nodeArena.ensureNodeCapacity`.

P25at publishes against clean base commit
`b65a9c235915edc3198851cf07b0257e3caed6d6`. The measurements used clean base
`3c2a2106102769bab891047174dbcfec15045e74`. The relevant source files are
byte-identical between these bases. Canopy traced the only parser caller at
`parser.go:7169`. The caller performs incremental initial sizing. Full parses
use `ensureExactNodeCapacity`.

The current function allocates a new primary node slice before node use. It
copies zero bytes. The temporary probe recorded requested nodes, capacity
growth, allocation bytes, live capacity, waste, and retained pool capacity.

| Lane | Calls | Growths | Requested nodes | Old/new capacity | Allocated bytes | Copied bytes | Live/capacity | Retained capacity/waste |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Primary full parse | 0 | 0 | 0 | 0/0 | 0 | 0 | 14,506/20,164 | 20,164/20,164 |
| Primary single-byte edit | 0 | 0 | 0 | 0/0 | 0 | 0 | 14,506/20,164 | 20,164/20,164 |
| Primary no-edit | 0 | 0 | 0 | 0/0 | 0 | 0 | 14,506/20,164 | 20,164/20,164 |
| Authenticated recovery deletion | 2 | 2 | 8,863 | 314/11,304 | 1,175,616 | 0 | 7,311/11,147 | 2,355/2,355 |
| Swift high-hit edit | 0 | 0 | 0 | 0/0 | 0 | 0 | 25,596/40,328 | 40,328/40,328 |
| JavaScript control | 0 | 0 | 0 | 0/0 | 0 | 0 | 2,240/20,164 | 20,164/20,164 |

The source identities are:

- Primary source: `2e0f9d4676549c8560cebc3c53877932f921ab9d77c88518227f3018573af4b7`.
- Primary edited source: `63d6743bdfcf50d94242b6c85923570d38b4ed8a24b2e9b636e91526874b71cd`.
- Recovery source: `74c0705f8729670559492fb5460a01b2a1a2a109928e1aeb52736e485e8ff097`.
- Recovery edited source: `81543368d0ec807dd951edfdb04fb66ee9ef14ca32b0bb5c53fc8a4d0f2e7b07`.
- Swift source: `7ccfed56194d956b47724c935ea3a6e1abcdddbfa23f440a9c19a3e5eab03c6c`.
- Swift edited source: `c5bd1ee190dde2a3c3d227563a281b31b938b43fadf7f58eccacf011dcafc9ac`.
- JavaScript source: `d84deac86ad03c7f23ef4a257aba49ca1ad40e4fd89a5a501e461b45bd099faa`.

The recovery lane entered the hotspot twice. Geometric growth requested
8,863 nodes and allocated 1,175,616 bytes. The final tree used 7,311 nodes.
Reset retained 2,355 nodes after the retention trim.

The rejected candidate used `newCap = max(min, minArenaNodeCap)`.
Its counters were calls `2`, growths `2`, requested nodes `8,863`, and old
capacity `314`. Its new capacity was `8,863`.
It allocated `921,752` bytes and discarded `32,656` bytes. It copied `0`
bytes. It retained `7,311` live nodes, `10,530` capacity nodes, and `3,219`
waste nodes. Its retained capacity and waste were `1,738/1,738`.
Both implementations copied zero bytes.

Focused candidate tests passed. Authenticated recovery parity passed in Docker.
The candidate failed the performance gate.

The gate required at least five percent improvement in target bytes per
operation (B/op) and allocations per operation (allocs/op). Controls could not
regress by more than one percent. The six-seed medians were:

| Lane | Base median | Candidate median | Change | Gate result |
| --- | ---: | ---: | ---: | --- |
| Recovery time | 6.057196 ms/op | 4.9153135 ms/op | -18.85% | Pass |
| Recovery B/op | 1,331,057.5 | 1,206,127.5 | -9.39% | Pass |
| Recovery allocs/op | 61 | 64 | +4.92% | Fail |
| Primary full parse time | 9.8366225 ms/op | 10.6181605 ms/op | +7.95% | Fail |
| Primary full parse B/op | 75,844.5 | 81,819.5 | +7.88% | Fail |
| Primary edit time | 1,025.65 ns/op | 1,153.5 ns/op | +12.47% | Fail |
| Primary no-edit time | 3.394 ns/op | 3.432 ns/op | +1.12% | Fail |

Primary edit, primary no-edit, and primary full allocations stayed at zero,
zero, and five, respectively. Primary edit and no-edit B/op stayed at zero.
The candidate failed the allocation target and the control threshold. Do not
run the required 20-seed campaign. Do not run a maximum resident set size
(RSS) campaign.

The Swift edit returned a full arena with an error-bearing root. It did not
exercise the incremental allocator. Treat it as a route control, not as a
Swift incremental witness.

The temporary probe, benchmark functions, wrappers, and candidate code were
removed. No parser or test change remains.

P25at used these structural commands:

```text
scripts/canopy_query.sh search refs ensureNodeCapacity . --limit 200 --json
scripts/canopy_query.sh graph calls --reverse ensureNodeCapacity . --json --depth 4
scripts/canopy_query.sh search symbols . --name 'ensure(Node|Exact)NodeCapacity|nodeArena|parse.*Arena.*Capacity|maxRetainedNodeCapacityForClass' --limit 300 --json
canopy search refs ensureNodeCapacity parser.go --no-cache --limit 50 --json
```

The telemetry command ran one workload per Docker container. The workload
values were `primary_full`, `primary_edit`, `primary_no_edit`,
`recovery_deletion`, `swift_collection`, and `javascript_control`.

```text
bash cgo_harness/docker/run_parity_in_docker.sh --repo-root /tmp/gts-p25at-capacity-20260823 --out-root /tmp/gts-p25at-artifacts --label telemetry-<workload> --no-build --memory 8g --cpus 1 --goflags '-p=1' --test-parallel 1 --timeout 20m -- "cd /workspace && GOMAXPROCS=1 P25AT_WORKLOAD=<workload> go test . -run '^TestP25ATNodeCapacityTelemetry$' -count=1 -parallel 1 -timeout 20m -v"
```

The candidate telemetry used the same command with
`P25AT_WORKLOAD=recovery_deletion` and the candidate source.

The correctness commands were:

```text
bash cgo_harness/docker/run_parity_in_docker.sh --repo-root /tmp/gts-p25at-capacity-20260823 --out-root /tmp/gts-p25at-artifacts --label candidate-correctness-arena --no-build --memory 8g --cpus 1 --goflags '-p=1' --test-parallel 1 --timeout 30m -- "cd /workspace && GOMAXPROCS=1 go test . -run '^(TestEnsureNodeCapacity|TestEnsureExactNodeCapacity|TestArenaResetRetainsOverflow|TestNodeRetentionCapRespectsByteLimit|TestArenaByteBreakdown)' -count=1 -parallel 1 -timeout 20m -v"
bash cgo_harness/docker/run_parity_in_docker.sh --repo-root /tmp/gts-p25at-capacity-20260823 --out-root /tmp/gts-p25at-artifacts --label candidate-correctness-recovery-parity --no-build --memory 8g --cpus 1 --goflags '-p=1' --test-parallel 1 --timeout 30m -- "cd /workspace/cgo_harness && GOMAXPROCS=1 GTS_CANONICAL_GO_INCREMENTAL=1 go test . -tags treesitter_c_parity -run '^TestCanonicalGoIncrementalParity/recovery_deletion$' -count=1 -parallel 1 -timeout 20m -v"
```

The benchmark runner used `GOMAXPROCS=1`, `-count=1`, `-benchmem`, one process
per shuffle seed, `-benchtime=750ms`, Docker CPU limit 1, and `GOFLAGS=-p=1`.
It ran seeds 1 through 6. The exact inner commands were:

```text
scripts/run_randomized_benchmarks.sh --output /artifacts/baseline-recovery-6.txt --runs 6 --seed-start 1 --benchtime 750ms --bench-regex '^BenchmarkP25ATRecoveryDeletion$'
scripts/run_randomized_benchmarks.sh --output /artifacts/candidate-recovery-6.txt --runs 6 --seed-start 1 --benchtime 750ms --bench-regex '^BenchmarkP25ATRecoveryDeletion$'
scripts/run_randomized_benchmarks.sh --output /artifacts/baseline-primary-6.txt --runs 6 --seed-start 1 --benchtime 750ms --bench-regex '^BenchmarkGoParse(FullDFA|IncrementalSingleByteEditDFA|IncrementalNoEditDFA)$'
scripts/run_randomized_benchmarks.sh --output /artifacts/candidate-primary-6.txt --runs 6 --seed-start 1 --benchtime 750ms --bench-regex '^BenchmarkGoParse(FullDFA|IncrementalSingleByteEditDFA|IncrementalNoEditDFA)$'
benchstat -format csv /tmp/gts-p25at-artifacts/baseline-recovery-6.txt /tmp/gts-p25at-artifacts/candidate-recovery-6.txt
benchstat -format csv /tmp/gts-p25at-artifacts/baseline-primary-6.txt /tmp/gts-p25at-artifacts/candidate-primary-6.txt
```

P25at artifacts and SHA-256 values are:

- Primary full telemetry: `/tmp/gts-p25at-artifacts/20260823T193944Z-telemetry-primary-full-v3/container.log`, `054267246c3fa299ecb1dca0c6d32c793df37f2b50bacf3f62d5591002c88270`.
- Primary edit telemetry: `/tmp/gts-p25at-artifacts/20260823T194014Z-telemetry-primary-edit/container.log`, `213e2e34d1d0b156ab9410940dbcfd6de33d5f8edc97fd60343c4edfc4314ae0`.
- Primary no-edit telemetry: `/tmp/gts-p25at-artifacts/20260823T194031Z-telemetry-primary-no-edit/container.log`, `3f78c18499223dd014833c149d2dabf83b4664e2c0dcc2bd6208ba49738ac98a`.
- Recovery telemetry: `/tmp/gts-p25at-artifacts/20260823T194056Z-telemetry-recovery-deletion/container.log`, `f3a9b7e901c229899001b38addb9a7cd49bc2bddf538a0ca7c124f87a182a7a7`.
- Swift telemetry: `/tmp/gts-p25at-artifacts/20260823T194153Z-telemetry-swift-collection-incremental/container.log`, `39dd529617431148d89f5ac4beb9dd29b4b77a47168e459fcfd66d69d305a37f`.
- JavaScript telemetry: `/tmp/gts-p25at-artifacts/20260823T194143Z-telemetry-javascript-control/container.log`, `2c15e5d368d3a427e1a6fd4c65566a7b469255c63b229fb4bdd2911357aee267`.
- Candidate recovery telemetry: `/tmp/gts-p25at-artifacts/20260823T194553Z-candidate-telemetry-recovery/container.log`, `1c5663f3fb23abc07bfdf15088d3507ac313bf8aa5f5a0b9af0189892824c32f`.
- Baseline recovery benchmark: `/tmp/gts-p25at-artifacts/baseline-recovery-6.txt`, `c45911b3ca83cbb2ec1319e0a5f8a59a6b23f28f004fb8ef159ef5393e7e5bfd`.
- Candidate recovery benchmark: `/tmp/gts-p25at-artifacts/candidate-recovery-6.txt`, `11063c61cec9f2415ac616ff6ca3113341c151cb9206217e846eb8134e7e09db`.
- Baseline primary benchmark: `/tmp/gts-p25at-artifacts/baseline-primary-6.txt`, `de657e266dff4ad318e7f23a0f85238e6a3c51fe224fb0da929613b4bc15539e`.
- Candidate primary benchmark: `/tmp/gts-p25at-artifacts/candidate-primary-6.txt`, `05d44d0470db8d3b8ef2656a0b81a1f623637494883b70cebfc7633345c7b740`.
- Arena correctness: `/tmp/gts-p25at-artifacts/20260823T194635Z-candidate-correctness-arena/container.log`, `1ff62ea02937c4afe94a67c74d7de9d19ca0120ed90f17ad520135e589e51f9a`.
- Recovery parity: `/tmp/gts-p25at-artifacts/20260823T194648Z-candidate-correctness-recovery-parity/container.log`, `cc99c4245b1a42f6c923c9b7a8a63f6e511da8e1c9776f384e1aa25a613fabbf`.

Residual risk remains. The Swift lane lacks a valid incremental witness. The
current receipt must not claim Swift allocator coverage. Reopen the screen only
when an authenticated Swift edit enters the incremental arena, preserves parser
parity, and meets both target allocation gates.

Checkpoint candidate: exact-fit sizing. Decision: **NO-GO**. Next action:
retain geometric growth and obtain the missing Swift incremental witness.

## 2026-08-23 P25aq node-equivalence depth telemetry

Status: **NO-GO / NO CANDIDATE**. Keep `glrNodeEquivCacheSize` at `16384`.

P25aq used clean worktrees at base
`e24ccf5a87bbd7febc21f67f014c2d5301d229d0`. Canopy traced the node lookup
and store paths in `glr.go`. The temporary probe kept the full key, version,
depth, epoch, collision, and eviction checks. The probe counted lookups,
hits, misses, stores, and depth-zero results by depth.

| Workload | Lookups | Hits | Misses | Stores | Depth-zero lookups | Depth-zero hits |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Authenticated recovery deletion | 3,431 | 49 | 3,382 | 3,382 | 5 | 0 |
| Swift high-hit control | 20,549 | 9,072 | 11,477 | 11,477 | 91 | 0 |
| JavaScript control (focused rerun) | 6 | 2 | 4 | 4 | 0 | 0 |
| Primary full parse | 0 | 0 | 0 | 0 | 0 | 0 |
| Primary single-byte edit | 0 | 0 | 0 | 0 | 0 | 0 |
| Primary no-edit | 0 | 0 | 0 | 0 | 0 | 0 |

The standalone P25aq telemetry log at
`/tmp/gts-p25aq-artifacts/20260823T172421Z-telemetry/container.log` recorded
zero JavaScript lookups and zero hits. A later focused-cache rerun at
`/tmp/gts-p25aq-artifacts/20260823T173419Z-correctness-cache/container.log`
recorded six lookups and two hits. This receipt uses the focused rerun value
in the table. It does not infer a cause for the difference.

Depth-zero probes were `0.15%` of recovery lookups and `0.44%` of Swift
lookups. They produced no hits. The probe did not show material cost, so no
depth-zero bypass candidate was tested. Do not infer a candidate from profile
samples alone.

Each base benchmark used 20 randomized seeds, `GOMAXPROCS=1`, one process per
seed, `-count=1`, `-benchtime=750ms`, and `-benchmem` in Docker. The base-only
reference medians were:

| Lane | Time | Bytes | Allocs |
| --- | ---: | ---: | ---: |
| Full parse | 7.261 ms/op | 55.42 KiB/op | 4 |
| Single-byte edit | 788.4 ns/op | 0 B/op | 0 |
| No edit | 2.620 ns/op | 0 B/op | 0 |
| Swift | 89.85 ms/op | 11.20 MiB/op | 5,178 |
| JavaScript | 1.658 ms/op | 188.9 KiB/op | 471 |
| Recovery deletion | 9.080 ms/op | 4.540 MiB/op | 111 |

Focused cache tests and the telemetry test passed. Authenticated recovery
parity passed. The full canonical parity run reached recovery deletion, then
stopped at the existing Python external-scanner fallback case. No candidate
passed the allocation or time gate. Therefore, no RSS run was needed.

P25aq artifacts and SHA-256 values are:

- Telemetry log: `/tmp/gts-p25aq-artifacts/20260823T172421Z-telemetry/container.log`, `88d5e8a179d41cea798ef64fd77ee041755c36bfff63f36168c068c983f64472`.
- Focused correctness log: `/tmp/gts-p25aq-artifacts/20260823T173419Z-correctness-cache/container.log`, `46c788ac277008caccbe32f7284e8a6a46a6e563cb207dda59ade48a867861b6`.
- Canonical parity log: `/tmp/gts-p25aq-artifacts/20260823T173433Z-correctness-canonical/container.log`, `ed304ac03a8dd542d41625c8d17539741b10081ed2dc401366bf1db960e35c02`.
- Recovery parity log: `/tmp/gts-p25aq-artifacts/20260823T173531Z-correctness-recovery-enabled/container.log`, `a650c0cd8021b49dd28b237c9f37d43b5602d8fa9e20890ab6d6879700e12b85`.
- Primary benchmark output: `/tmp/gts-p25aq-base-20260824/harness_out/p25aq/base/primary.txt`, `3773bb1cc5a303befc8d6b2acdf5e247135a6758e730fceb09eb3ab3d02cb248`.
- Swift benchmark output: `/tmp/gts-p25aq-base-20260824/harness_out/p25aq/base/swift.txt`, `4d8574735a787d063f63c726345bb3478ff670d9a61724904677035e8fe486af`.
- JavaScript benchmark output: `/tmp/gts-p25aq-base-20260824/harness_out/p25aq/base/js.txt`, `89979a97fa53de8a9baa214d3a09b2022591473aae797ee574c9427f510e56c8`.
- Recovery benchmark output: `/tmp/gts-p25aq-base-20260824/harness_out/p25aq/base/recovery.txt`, `02d3259192dc4243e989f71bc516a2dcf258f8bedfaac74ea5c27dc67cdfe766`.

The temporary telemetry, test, and benchmark files were removed. No P25aq
production or test change remains.

## 2026-08-23 P25ar lazy node-equivalence table allocation

Status: **NO-GO / REVERTED**. Keep the eager full-table behavior at
`glrNodeEquivCacheSize = 16384`.

P25ar used the clean base
`e24ccf5a87bbd7febc21f67f014c2d5301d229d0`. Canopy traced construction in
`beginEquivEpoch`, lookup guards, store paths, and `allocatedBytes`.
The candidate removed allocation from `beginEquivEpoch`. The first valid store
allocated the existing full table. Zero-probe epochs kept a nil table and zero
node-cache bytes. Full key, version, depth, epoch, collision, and eviction
semantics stayed unchanged in the candidate.

The deterministic candidate tests covered these cases:

- A zero-probe epoch allocated no node-equivalence table.
- The first store allocated all `16384` entries.
- Two colliding entries retained their separate results.
- A new epoch rejected entries from the previous epoch.

Focused cache tests passed. Authenticated `recovery_deletion` parity passed.
Each benchmark used 20 randomized seeds, `GOMAXPROCS=1`, one process per
seed, `-count=1`, `-benchtime=750ms`, and `-benchmem` in Docker.

| Lane | Time change | Bytes change | Allocs change | Decision |
| --- | ---: | ---: | ---: | --- |
| Primary full parse | `+0.64%` | `+0.55%` | unchanged | Reject |
| Primary single-byte edit | `+2.29%` | `0%` | unchanged | Reject |
| Primary no-edit | `-0.27%` | `0%` | unchanged | Control |
| Swift high-hit | `-9.36%` | `0%` | `0%` | No allocation win |
| JavaScript control | `-5.38%` | `0%` | `0%` | Pass |
| Recovery deletion | `+6.10%` | `+0.03%` | `0%` | Reject |

The candidate did not improve bytes or allocations by at least five percent on
any target lane. It also regressed the primary edit control by more than one
percent. No RSS run was needed because the allocation and time gates failed.
The candidate diff for `glr.go` and `glr_test.go` has SHA-256
`c250f6297adc23371de2282ce132dc99fa20167d937cc8616d08af8a8f3eb605`.

P25ar artifacts and SHA-256 values are:

- Lazy-cache correctness log: `/tmp/gts-p25ar-artifacts/20260823T174052Z-correctness-lazy-node-cache/container.log`, `006d52c81cf97d36cfc63a30cf256ad4de10fef9b1a46079a50c7ba523a2d264`.
- Recovery parity log: `/tmp/gts-p25ar-artifacts/20260823T174127Z-correctness-recovery-lazy-node-cache/container.log`, `0b5ebb6f1851be877653f101d6c26808cd6148281aaca8b3bf2a661daadf8d15`.
- Primary base output: `/tmp/gts-p25ar-artifacts/benchmark-outputs/base-primary.txt`, `56bf71e79676f447d09924dda1de77fc19ec105e08d12204f9f89c63802383cb`.
- Primary candidate output: `/tmp/gts-p25ar-artifacts/benchmark-outputs/candidate-primary.txt`, `7153c2b149a965b90c17585ff02e270dc0d391680b1da5b9b1572c04b7b4a500`.
- Swift base output: `/tmp/gts-p25ar-artifacts/benchmark-outputs/base-swift.txt`, `2e4359c91e2af28f44193687adf383b97d66dd419b4f51232b20d4f9cca7ff8a`.
- Swift candidate output: `/tmp/gts-p25ar-artifacts/benchmark-outputs/candidate-swift.txt`, `84d7f55726472f50f9dc917e88056a7898b382cefdec32027c7e1e11bd5ea2f9`.
- JavaScript base output: `/tmp/gts-p25ar-artifacts/benchmark-outputs/base-javascript.txt`, `20071f85a4ae1657cb36bb521c04f7705ffa4ea60e1e5062ae3f2b0b8418e9fa`.
- JavaScript candidate output: `/tmp/gts-p25ar-artifacts/benchmark-outputs/candidate-javascript.txt`, `5427417d4007987655620d5a7c91199cfd868508eda5525f2a442e89fb4a2352`.
- Recovery base output: `/tmp/gts-p25ar-artifacts/benchmark-outputs/base-recovery.txt`, `48ea11b07c9fbf8fcff56bb9231d437db4d42d55c7efd6618f41380ec50f2d11`.
- Recovery candidate output: `/tmp/gts-p25ar-artifacts/benchmark-outputs/candidate-recovery.txt`, `4e54a2b5461fb5385406d8527eecd74c3b383714fe5295d1124b7666639fe531`.

The candidate production and test changes were reverted. No P25ar code or
test change remains.

## 2026-08-23 P25ao adaptive node-equivalence cache screen

Status: **NO-GO / REVERTED**. Keep `glrNodeEquivCacheSize` at `16384`.

P25ao used clean worktrees from base
`d54147516440a91b8eda6983251c7cd6c4be2707`. The candidate started the
node-equivalence cache at two entries. It doubled the table after a mixed
miss and hit signal. It stopped at the existing `16384` entry limit.
Growth discarded old entries. This preserved fail-closed semantics, but it
added table allocation and lookup overhead.

The current base audit reported these merge-heavy controls:

| Workload | Lookups | Hits | Hit share |
| --- | ---: | ---: | ---: |
| Swift `stdlib_Collection.swift` | 7,694 | 5,123 | 66.6% |
| JavaScript control | 6 | 2 | 33.3% |

The deterministic Docker test passed collision, growth, and epoch-reset
checks. The test verified safe misses after growth and preserved full key,
version, depth, and epoch checks. The candidate did not change parse results.
The correctness log is
`/home/draco/work/gotreesitter/harness_out/docker/20260823T160419Z-p25ao-cache-correctness/container.log`,
SHA-256
`2f43dcfa5925f5cc4c2a2e4334c90efcc202872896ab788bfadfb15a3344d60d`.

Each benchmark used 20 randomized seeds, `GOMAXPROCS=1`, one process per
seed, `-count=1`, `-benchtime=750ms`, and `-benchmem` in Docker.

| Lane | Time change | Bytes change | Allocs change | Decision |
| --- | ---: | ---: | ---: | --- |
| Swift | `+1.72%` | unchanged | unchanged | Reject |
| JavaScript | approximately unchanged | unchanged | unchanged | Pass |
| Recovery deletion | `+26.34%` | `+0.17%` | `+0.90%` | Reject |

The recovery benchmark included the initial parse inside the timed loop. It
does not replace the authenticated pooled and drained receipt. It still
shows a decisive time regression. No RSS run was needed after the time gate
failed. The primary trio showed lower measured time, but its no-edit lane
reported near-zero work and did not override the failed target lanes.

P25ao benchmark outputs are:

- Swift base: `/tmp/gts-p25ao-base-20260824/p25ao-swift-base.txt`, SHA-256 `aa52fba3be698259b3cd7469611d86cb6049968ce263a8369efff0a54e707a9d`.
- Swift candidate: `/tmp/p25ao-rejected-artifacts/p25ao-swift-candidate.txt` (moved after cleanup), SHA-256 `569f974853d5febc7708690398313606883f7d451faac631c3c0d16c4d8b3fc6`.
- JavaScript base: `/tmp/gts-p25ao-base-20260824/p25ao-js-base.txt`, SHA-256 `47e82bc438df89ab05d15e512e43c8166a4f3b1a305cd22064ca39e5b7bc1579`.
- JavaScript candidate: `/tmp/p25ao-rejected-artifacts/p25ao-js-candidate.txt` (moved after cleanup), SHA-256 `daf3c4d371683a85484eb8f99c56b6d62857652e52e47cf4efb95ec225664f6e`.
- Primary trio base: `/tmp/gts-p25ao-base-20260824/p25ao-primary-base.txt`, SHA-256 `8c3f9ce3e30843da0aad237d7aa168e6a9b26d9cac3ef5af5728e241f884d2a7`.
- Primary trio candidate: `/tmp/p25ao-rejected-artifacts/p25ao-primary-candidate.txt` (moved after cleanup), SHA-256 `7fdf1d96b36dab18a2e27811a0994d3a7977dfaf1e55b4c0d9f3d443583bf7d0`.
- Recovery base: `/tmp/gts-p25ao-base-20260824/p25ao-recovery-base.txt`, SHA-256 `d3ae96bf7ff110e0dcd071c1cb5f5dc6ff5f390ab45f9593da9b63c210d1fc74`.
- Recovery candidate: `/tmp/p25ao-rejected-artifacts/p25ao-recovery-candidate.txt` (moved after cleanup), SHA-256 `873d93e416f6180eb26f1fb103373c38952de8f6f5aac506f1c9c71a699131e5`.

The last two recovery hashes are retained for traceability. The candidate
worktree is clean after reverting `glr.go`, `glr_test.go`, and the temporary
benchmark file. No production or test change ships.

The recovery profile is `/tmp/gts-p25ab-profile/p25ab-recovery.pprof`,
SHA-256 `d718da2a0cb18f586df1b97c609469a340e1ddaa9fcb276121df1154f01b02f2`.

### Next signal decision

Canopy maps the node-equivalence lookup to the frontier comparison path and
the exact comparison path. The existing recovery profile records no sampled
node-equivalence lookup or store frames. Its focused cache view records only
`3.25%` of samples across equivalence helpers, with `lookupSpineEquivCache` at
`1.30%` flat time. The profile does not support another node-cache candidate.

The smallest future node-cache signal is the existing depth key. A diagnostic
count should first report the depth-zero share and the hit rate by depth. A
future candidate could bypass depth-zero table probes only if that count shows
material probe cost and stable reuse at deeper depths. No production candidate
was tested. Do not infer an allocation win from this proposal.

## 2026-08-23 P25an collision hardening

Status: **NO-GO / REVERTED**. Keep `glrNodeEquivCacheSize` at `16384`.

P25an tested the candidate from base
`5eaa38e536e54530b3d795c6c0a56d927d3d0e0e` with code-only SHA-256
`9c8a68735bcfca0c3c7971957d631bb5beb8d639aeeb216a98228d0f6e7b84fd`.
The candidate changed the node-equivalence cache from `16384` entries to
`2` entries. The deterministic test covered full keys, versions, depth, epoch,
eviction, and parse-result stability. Swift regressed time by `45.38%`.
JavaScript improved time by `8.45%`. Swift exceeded the one-percent limit.

P25an used 20 randomized seeds, `GOMAXPROCS=1`, one process per seed,
`-count=1`, `-benchtime=750ms`, and `-benchmem` in Docker.

| Lane | Time change | Bytes change | Allocs change | Decision |
| --- | ---: | ---: | ---: | --- |
| Swift | `+45.38%` | `+0.12%` | `+0.04%` | Reject |
| JavaScript | `-8.45%` | unchanged | unchanged | Pass |

P25an artifacts are:

- Correctness log: `/home/draco/work/gotreesitter/harness_out/docker/20260823T155519Z-p25an-collision-correctness/container.log`, SHA-256 `f8b701422f118969189208595b6ee29cf9373d317778e29751793cafc27f1ab4`.
- Swift base: `/tmp/gts-p25al-base-20260824/p25an-swift-base.txt`, SHA-256 `c5cf3294ba250ef28634268ab1caa205fda71a38b70d23f4a4bc27e900d39238`.
- Swift candidate: `/tmp/gts-p25aj-screen-20260824-aj/p25an-swift-candidate-v2.txt` (removed after cleanup), SHA-256 `af8feb9106a8171937d68cf9d6568a29891e86f7701883944e726236be7fb571`.
- JavaScript base: `/tmp/gts-p25al-base-20260824/p25an-js-base.txt`, SHA-256 `ad83319ad36c5ee1c46e97a14bc0384d907818a09e8896a5314a79314626ebcf`.
- JavaScript candidate: `/tmp/gts-p25aj-screen-20260824-aj/p25an-js-candidate.txt` (removed after cleanup), SHA-256 `c61ff75df97fbc6db0cec3042cd151cc303d67da7352502c35cfd6bf1631921f`.

Cleanup removed the P25an candidate output files. Their hashes remain for
traceability. The candidate was reverted. No production or test change
remains.

## 2026-08-23 P25ak-P25al node-equivalence cache screen

Status: **HISTORICAL SCREEN / SUPERSEDED BY P25an NO-GO**. P25ak-P25al
passed their scoped gates. P25an rejected the candidate on a Swift time
regression.

The base was
`5eaa38e536e54530b3d795c6c0a56d927d3d0e0e`. The candidate changed
`glrNodeEquivCacheSize` from `16384` to `2`. The code-only diff SHA-256 was
`9c8a68735bcfca0c3c7971957d631bb5beb8d639aeeb216a98228d0f6e7b84fd`.

P25ak telemetry identified the node cache as the lowest-hit fixed merge cache:

| Cache | Initial bytes | Lookups | Hits | Drained construction |
| --- | ---: | ---: | ---: | ---: |
| Node equivalence | 512 KiB | 3,482 | 81 | 2 |
| Stack equivalence | 96 KiB | 1,321 | 85 | 1 |
| Spine equivalence | 640 KiB | 2,876 | 1,145 | 2 |
| Shape prefix | 512 KiB | 3,210 | 1,146 | 2 |

The accepted P25al candidate reduced drained bytes per operation by `11.41%`
and allocations by `0.75%`. Pooled time improved by `13.40%`. The primary
trio improved time by `6.64%` to `7.76%`. Only full parse reduced bytes, by
`7.56%`; other trio allocation metrics stayed unchanged. A known JavaScript
control failed in both baseline controls. Three focused JavaScript
regressions passed in both trees. Do not claim a broad allocation win.

The P25ak and P25al artifact paths and hashes remain:

- Pooled telemetry: `/home/draco/work/gotreesitter/harness_out/docker/20260823T151230Z-p25ak-telemetry-pooled/container.log`, SHA-256 `2abc13026c624620f98a2b0099202f2019d19cc105f27d823469f5efc2674c2a`.
- Drained telemetry: `/home/draco/work/gotreesitter/harness_out/docker/20260823T151300Z-p25ak-telemetry-drained/container.log`, SHA-256 `821c01d44ee5cdafffda8254c88cdd399e987bbdaeece8a57886213a591e8ecb`.
- Base primary: `/tmp/gts-p25al-artifacts/p25al-base-primary.txt`, SHA-256 `ffecaf146c1dcc9238a79676109fb81d5d77e0e563aa0e473a2b6dde84093592`.
- Candidate primary: `/tmp/gts-p25al-artifacts/p25al-candidate-primary.txt`, SHA-256 `d23323f0d18d61b11f6f1be4ab00230ae2bdd8d962caa5d21eb487230203764e`.
- Base large-file RSS: `/tmp/gts-p25al-artifacts/p25al-base-large-rss.rss`, SHA-256 `de5e642a1aa8a4669496ed1f6cad63f9ce7e0f42f3f2b166a42553fdba6fe73b`.
- Candidate large-file RSS: `/tmp/gts-p25al-artifacts/p25al-candidate-large-rss.rss`, SHA-256 `01892345b764f36dcd3bc0d5bcf01be68d7285b6844ac0831edb0d3a77faccf1`.

P25al focused Docker logs passed for Go, JavaScript, Python, and Rust. Keep
issue #454 live. No production or test change from P25ak-P25al remains.

## 2026-08-23 P25x-P25ab field and reduction performance blocker

Status: **KEEP ISSUE #454 LIVE / NO-GO**. Ship no code.

The publication base is main commit
`8c80a46e450d906fc9ce1665c189497b02483a3e`.

P25x used evidence base
`3d6cd2628f7a42c348f51dce0a0ed9b92b183c6a`. P25y and P25z used evidence base
`675697a1144fad306489c5142aedaae0825545d9`. P25aa and P25ab used evidence
base `2c533f5c19f5f7ab9b586cd8454f0cdc4ece014b`.

No production or test change survives this receipt. No twenty-seed campaign
was warranted because no candidate passed the proof boundary.

### P25x condense and resume guard

P25x audited `cCondenseAndResume` for one redundant resume relex operation.
Canopy traced the call through C recovery lookahead, shared-token state, and
custom-source state. The temporary candidate added a guard before
`cRecoverResumeLookahead`.

The one-file candidate diff changed `parser_recover_c.go`. Its raw diff
SHA-256 was
`4ac139b25516f7856bba0ed7967ba4fb777a00bcd60bbfde6802f80257a40c6e`.

Focused condense tests passed. The authenticated forward
`recovery_deletion` parity test passed. Go fresh, Go incremental, C fresh, and
C incremental used digest
`b03cf98e18bdddbd5ee3d2a0bc39410c51e21c41fbde89f7c5f3107c63276502`.

The candidate did not prove a grammar-agnostic skip. P25x therefore stopped
before publication. The candidate remains uncommitted in the isolated
`/tmp/gts-p25x-condense` worktree; no candidate code ships.

P25x artifacts are:

- Final trace: `/tmp/gts-p25x-artifacts/20260823T105447Z-p25x-condense-trace-final/container.log`, SHA-256 `abf435f20497da1455019be66d45188800f6e8bfd6b4cbb41117451bebf56958`.
- Candidate recovery metadata: `/tmp/gts-p25x-artifacts/20260823T110154Z-p25x-condense-candidate-recovery-enabled/metadata.txt`, SHA-256 `45d6b7fd839d6441148e30243410ffa1d46f1261fe14644e711a2bbf4750ff3e`.
- Base benchmark: `/tmp/gts-p25x-artifacts/20260823T110343Z-p25x-condense-base-bench-2/container.log`.
- Candidate benchmark: `/tmp/gts-p25x-artifacts/20260823T110330Z-p25x-condense-candidate-bench-2/container.log`.

### P25y six-seed rejection

P25y re-applied only the P25x guard at evidence base
`675697a1144fad306489c5142aedaae0825545d9`.

The artifact set contains six base and six candidate runs, paired by shuffle
seed. Its metadata records one central processing unit (CPU), four GiB of
memory, `GOFLAGS=-p=1`, `-count=1`, `-benchtime=750ms`, `-benchmem`, and
`-parallel=1`. It does not record `GOMAXPROCS=1` or an alternating process
order protocol.

The table reports per-lane geometric means over the six cited base files and
the six cited candidate files. The change is candidate minus base.

| Lane | Base | Candidate | Change |
| --- | ---: | ---: | ---: |
| Full parse | `7,160,818 ns/op` | `7,663,006 ns/op` | `+7.01%` |
| Single-byte edit | `788.657 ns/op` | `818.586 ns/op` | `+3.79%` |
| No edit | `2.527652 ns/op` | `2.584016 ns/op` | `+2.23%` |
| `recovery_deletion` | `4,131,437 ns/op` | `4,231,133 ns/op` | `+2.41%` |

The candidate also raised full-parse allocations at seed four and seed six.
The candidate failed the screen. No twenty-seed campaign ran.

P25y artifacts are:

- Root controls: `/tmp/gts-p25y-artifacts/20260823T110859Z-p25y-condense-root` and `/tmp/gts-p25y-artifacts/20260823T110908Z-p25y-condense-recovery`.
- Base seeds: `/tmp/gts-p25y-artifacts/20260823T110931Z-p25y-base-s01` through `/tmp/gts-p25y-artifacts/20260823T111236Z-p25y-base-s06`.
- Candidate seeds: `/tmp/gts-p25y-artifacts/20260823T110950Z-p25y-candidate-s01` through `/tmp/gts-p25y-artifacts/20260823T111255Z-p25y-candidate-s06`.
- Candidate seed-six log SHA-256: `31aaad9ea03b18ff58015c69267561465568c1f06feee759a698c13aea1fa287`.

The candidate worktree was restored clean. No parser code ships.

### P25z reduction-path audit

P25z audited `applyReduceActionFromGSS` and `applyReduceActionDispatch`.
Canopy traced reduction lookup, field propagation, dynamic precedence, GSS
mutation, and rollback. It found no distinct duplicate operation with a safe
generic invariant.

The focused reduction unit and suite passed. The authenticated
`recovery_deletion` parity test passed. A short six-run recovery screen showed
`4,076,831 ns/op` for base and `4,027,230 ns/op` for the temporary candidate,
or `-1.22%`. The screen lacked a primary-trio result, confidence interval,
and a proven invariant. It did not justify a twenty-seed run.

P25z artifacts are:

- Reduction unit: `/tmp/gts-p25z-artifacts/20260823T112225Z-p25z-reduce-unit`.
- Reduction suite: `/tmp/gts-p25z-artifacts/20260823T112315Z-p25z-reduce-suite`.
- Recovery parity: `/tmp/gts-p25z-artifacts/20260823T112349Z-p25z-reduce-recovery-parity/metadata.txt`, SHA-256 `47f223530f35b524e215b3477853455db49108b73763f0ae444ad53ad0e78a82`.
- Base six-run screen: `/tmp/gts-p25z-bench/20260823T112644Z-p25z-base-recovery-bench-count6`.
- Candidate six-run screen: `/tmp/gts-p25z-bench/20260823T112701Z-p25z-candidate-recovery-bench-count6/container.log`, SHA-256 `35397216dc0e9351975fdbd77d1eb463b87638af2da549bf4dc22c464e6a33f1`.

P25z removed its temporary candidate. Treat the short improvement as
diagnostic noise, not as a performance result.

### P25aa hidden-field scan rejection

P25aa tested one `parser_reduce.go` change. It attempted to skip a duplicate
visibility scan in the hidden-field reduction path.

The candidate diff SHA-256 was
`749b9748010a818c2524305a48709347eb22274a7d46f2d0906d9f2ff9debbc6`.

All focused field tests passed. The authenticated forward
`recovery_deletion` parity test passed. The six-seed recovery screen reported
`4,053,945 ns/op` for base and `4,122,966 ns/op` for the candidate.
The candidate rose `+1.70%` by geometric mean. No twenty-seed campaign ran.

Two baseline setup attempts were quarantined. The first used the wrong package
path. The second used a missing script path. The third baseline run passed.

P25aa artifacts are:

- Field tests: `/tmp/gts-p25aa-artifacts/20260823T113505Z-p25aa-fields` and `/tmp/gts-p25aa-artifacts/20260823T113541Z-p25aa-reduce-tests`.
- Recovery parity: `/tmp/gts-p25aa-artifacts/20260823T113559Z-p25aa-recovery-parity/metadata.txt`, SHA-256 `b2200bf0bb11ed176a420d0faf4368acdc9458f7db81ae58e289cf9dcbdc5ff9`.
- Baseline six-seed output: `/tmp/gts-p25aa-bench/p25aa-base-3.txt`.
- Candidate six-seed output: `/tmp/gts-p25aa-bench/p25aa-candidate.txt`, SHA-256 `ab826a40ffc30e91c0bd82031a8b1080ac626021295e79617d687fb71f4e865d`.
- Failed baseline metadata: `/tmp/gts-p25aa-artifacts/20260823T113707Z-p25aa-base-bench/metadata.txt` and `/tmp/gts-p25aa-artifacts/20260823T113717Z-p25aa-base-bench2/metadata.txt`.

P25aa removed the candidate. No parser or test change ships.

Earlier P25y figures `+1.40%`, `+2.95%`, `+0.44%`, and `+2.62%` are
unsupported by this artifact set. Earlier P25aa figures `+1.00%` and
`+0.86%` are also unsupported. The authenticated raw files supersede both
sets of figures.

### P25ab field-path telemetry

P25ab added temporary performance counters for three field paths:

- Hidden-field scratch fallback.
- `appendFlattenedHiddenChildrenWithFieldScratch`.
- `applyParentFieldToFlattenedHiddenSpan`.

The smallest field control executed one fallback, two flatten calls, seven
hidden nodes, 11 source bytes, four output nodes, and one parent-field call.
It used three parent-field span nodes. Its diagnostic benchmark reported
`2,656 ns/op`, `5,008 B/op`, and `32 allocs/op`.

The authenticated forward recovery control matched all four deep digests:
Go fresh, Go incremental, C fresh, and C incremental. Each digest was
`b03cf98e18bdddbd5ee3d2a0bc39410c51e21c41fbde89f7c5f3107c63276502`.

The recovery counters reported 1,183 fallbacks, 2,363 flatten calls, 7,065
hidden nodes, 188,613 source bytes, 3,932 output nodes, and 3,275 parent-field
calls. The diagnostic benchmark reported `1,972,669 B/op` and `112 allocs/op`.

The CPU profile placed `29.22%` cumulative time in
`retryIncrementalAcceptedErrorWithBaseMergeCap`, `27.92%` in
`applyReduceActionFromGSS`, and `1.30%` flat time in
`appendFlattenedHiddenChildrenWithFieldScratch`. The temporary timer added
`time.Now` overhead. Do not read its absolute helper time as production cost.

The allocation profile placed `74.98%` of allocated bytes in
`nodeArena.ensureNodeCapacity`. The field helpers did not appear as allocation
sources. P25ab collected no separate RSS sample.

P25ab artifacts are:

- Field control: `/tmp/gts-p25ab-artifacts/20260823T115059Z-p25ab-field-control-v2`.
- Field benchmark: `/tmp/gts-p25ab-artifacts/20260823T115134Z-p25ab-field-bench`.
- Recovery control: `/tmp/gts-p25ab-artifacts/20260823T115143Z-p25ab-recovery-control`.
- Recovery benchmark: `/tmp/gts-p25ab-artifacts/20260823T115222Z-p25ab-recovery-bench`.
- CPU profile: `/tmp/gts-p25ab-profile/p25ab-recovery.pprof`, SHA-256 `d718da2a0cb18f586df1b97c609469a340e1ddaa9fcb276121df1154f01b02f2`.
- Allocation profile: `/tmp/gts-p25ab-profile/p25ab-recovery.mprof`, SHA-256 `7025b862af5934db0dfcb1a31746c451ef899cca2de8bc87482bd782c3419a1d`.
- Normal compile: `/tmp/gts-p25ab-artifacts/20260823T114626Z-p25ab-compile-normal`.
- Performance compile: `/tmp/gts-p25ab-artifacts/20260823T114626Z-p25ab-compile-perf`.

All P25ab Docker metadata used one CPU, four GiB, `GOMEMLIMIT=3GiB`,
`GOFLAGS=-p=1`, and test parallelism one. Seven of the nine receipts explicitly
set `GOMAXPROCS=1`; the normal and performance compile receipts do not record
it. Every run passed without timeout or out-of-memory failure. The telemetry
was removed.

### P25x-P25ab decision and reopening condition

Reject every P25x through P25ab candidate. No production or test change
survives. Keep issue #454 open.

The accepted-error retry bypass is correctness-invalid. P25w found a root
`ERROR` with 196 children and a different first error span when the bypass ran.

The field helpers execute materially, but they are not the dominant source.
The next source-owned investigation is `nodeArena.ensureNodeCapacity`, with
separate attribution for arena growth and parser work. Keep correctness and
performance gates separate.

Reopen this lane only when a fresh profile identifies one bounded,
grammar-agnostic operation. Prove fresh, incremental, and locked-C equality.
Run six-seed screens before any twenty-seed campaign. Require neutral bytes,
allocations, RSS, and parser-work counters before publication.

## 2026-08-23 P25k-P25w incremental and source-hotspot blocker

Status: **KEEP ISSUE #454 LIVE / NO-GO**. Ship no code.

The publication base is main commit
`3d6cd2628f7a42c348f51dce0a0ed9b92b183c6a`. P25k through P25q used evidence
base `09cb5faa41af35a6bc84fefccbab1a17850d38cc`. Every investigation
worktree remained code-clean.

### P25k fresh profile and remaining source hotspots

P25k collected a quiet Docker profile for the authenticated Go
`grammargen_lr` workload. The container used one central processing unit
(CPU), four GiB of memory, `GOMEMLIMIT=3GiB`, `GOMAXPROCS=1`, and
`GOFLAGS=-p=1`. The three-second benchmark reported `96,500,616 ns/op`,
`8,630 B/op`, and `123 allocs/op` over 32 iterations.

The profile ranked source-owned work outside the exhausted copy, dispatch,
token-source, lexer, generalized LR (GLR) ranking, checkpoint, and
graph-structured stack (GSS) hash lanes:

| Function | Flat time | Cumulative time | Result |
|---|---:|---:|---|
| `VisitMaterializationPostorderWithScratch` | `110 ms` | `370 ms` | Required traversal |
| `reduceOutputsClassifiedIntoActive` | `110 ms` | `880 ms` | Required reduction and condensation |
| `appendSubtreeRecord` | `100 ms` | `140 ms` | Required provenance append |
| `lookupActionIndexSmall` | `70 ms` | — | State and symbol dependent |
| `applyGenericShiftsOwned` | `60 ms` | `540 ms` | Required authenticated shift |
| `condenseDirectAppend` | `50 ms` | `180 ms` | Required graph publication |
| `persistHeaderLineageOwned` | `40 ms` | `290 ms` | Already skips clean state |
| `appendNodeAt` | `40 ms` | `120 ms` | Required graph validation |
| `popPaths` | `10 ms` | `130 ms` | Required ambiguous traversal |

Canopy and source review found no duplicate operation with a generic
invariant. The profile and symbol binary are:

- CPU profile: `/tmp/gts-p25k-investigation/harness_out/p25k-profile/grammargen_lr.cpu.pprof`, SHA-256 `45b04d62b726bb9683eca458fade8c238cb18ff8b0f8f098e8d3823d46b8b1ef`.
- Allocation profile: `/tmp/gts-p25k-investigation/harness_out/p25k-profile/grammargen_lr.alloc.pprof`, SHA-256 `9148278e831f4b48c801bbdb206e83ee46d3b310f1cc61ebe28a55733ac07bb9`.
- Test binary: `/tmp/gts-p25k-investigation/harness_out/p25k-profile/gotreesitter.test`, SHA-256 `f92306de048603a8029efbf7603d9429b3f1da62b97a45be98394694d9bb0405`.
- Docker metadata: `/tmp/gts-p25k-investigation/harness_out/docker/20260823T091826Z-p25k-fresh-profile/metadata.txt`, SHA-256 `f0b8e34630730b6ab896977908d8a846d9df29e4fa9035e4917112a17837fb30`.

### P25l through P25p source attribution

P25l traced `dispatchPassActive` and `elect`. Canopy mapped their callers,
state ownership, rollback points, and parity-sensitive operations. No
provably duplicate lookup or operation remained.

P25m traced the `tokenSource.Next` cost. The existing profile placed about
`460 ms` of flat time in that function. Checkpoint and scanner sites each
used about `20` to `40 ms`. The trace found no safe serialization or lookup
to remove.

P25n separated the lexer and GLR token paths. `Lexer.scan` used about
`110 ms` flat time. `scanPreferredTokenForState` used about `90 ms` flat
time. Rune decoding, table access, state restoration, and candidate ranking
remain semantics-sensitive. No generic redundant operation was proven.

P25o attempted dynamic branch evidence. Docker installed `perf`, but the
kernel rejected branch events with `No permission to enable branches event`.
The permission context reported `perf_event_paranoid=2`. The blocker
artifacts are:

- Install probe: `/tmp/gts-p25o-investigation/harness_out/docker/20260823T093535Z-p25o-perf-install-probe`, metadata SHA-256 `b75d25658cb815a64385b20c478d69a865efaaefb8088cec9c15fa8b55aad`.
- Permission probe: `/tmp/gts-p25o-investigation/harness_out/docker/20260823T093722Z-p25o-perf-permission-context`, metadata SHA-256 `3c857661b771725fd1d1a442b729b706d15016074f01223457ebea4679f08669`.

P25p reused the P25k profile to rank the remaining source-owned functions.
It found no exact redundant operation and created no code or new benchmark
artifact. P25l through P25p did not run correctness or parity gates because
no candidate passed the proof boundary.

### P25q incremental attribution

P25q collected separate edit and no-edit profiles with one CPU,
`GOMAXPROCS=1`, `GOFLAGS=-p=1`, `-count=1`, `-benchmem`, and two-second
Docker runs. Both runs passed without an out-of-memory event or timeout.

The edit benchmark reported `791.5 ns/op`, `0 B/op`, and `0 allocs/op`.
The edit profile placed `Tree.Edit` at `8.51%` flat and `16.41%`
cumulative CPU. `tryTokenInvariantLeafEdit` used `43.47%` cumulative CPU,
and `parseIncrementalChanged` used `58.36%` cumulative CPU. The edit
telemetry reported `88.8 ns` in `Tree.Edit`, `420.0 ns` in reuse, and zero
reparse or rebuild time per iteration. It reused one subtree and 19,294
bytes. It created no new nodes and performed no recovery work.

The no-edit benchmark reported `2.625 ns/op`, `0 B/op`, and `0 allocs/op`.
It is a same-source identity path, not a reparse workload. Its profile placed
`checkLanguageCompatible` at `16.91%` flat CPU,
`ensureResultCompatibility` at `10.79%`, and
`canReuseUnchangedTree` at `9.35%`. Compatibility caching is unsafe without
an immutable-language invariant and the existing telemetry reset.

P25q artifacts are:

- Edit CPU profile: `/tmp/gts-p25q-investigation/harness_out/p25q-edit/edit.cpu.pprof`, SHA-256 `a47c9c712b29e639c0a965ae239ca9f9977742b9f883429e1de6cd8d8bd88141`.
- Edit allocation profile: `/tmp/gts-p25q-investigation/harness_out/p25q-edit/edit.alloc.pprof`, SHA-256 `b85eba69409d1e24003967ee94a3d2d7760c75c9c4086662405578e8b4951492`.
- No-edit CPU profile: `/tmp/gts-p25q-investigation/harness_out/p25q-noedit/noedit.cpu.pprof`, SHA-256 `e9d53a47367b877ec5126425e75dfae4d7f8e67ded76494d636c2b88586ccbb7`.
- No-edit allocation profile: `/tmp/gts-p25q-investigation/harness_out/p25q-noedit/noedit.alloc.pprof`, SHA-256 `a671e53fad75cc7d47a022b8f86e4606d051d0872261006526bace67e949aca9`.
- Edit metadata: `/tmp/gts-p25q-investigation/harness_out/docker/20260823T094337Z-p25q-incremental-edit-profile-final/metadata.txt`, SHA-256 `cd5607f50c9fa887d90f1484d76b319bac200039a2afc4058771fb20be65fd80`.
- No-edit metadata: `/tmp/gts-p25q-investigation/harness_out/docker/20260823T094349Z-p25q-incremental-noedit-profile-final/metadata.txt`, SHA-256 `1907b4a2563a77a5c799643ef5785f2e70428403688c74544141802043ee8afd`.
- Edit telemetry metadata: `/tmp/gts-p25q-investigation/harness_out/docker/20260823T094511Z-p25q-incremental-edit-stats/metadata.txt`, SHA-256 `871f72defab4f08178c6bad5d93f9879ed290e5ebc98572cbf2c07dfb7943ade`.

### P25r through P25u recovery-forward investigation

P25r through P25t used evidence base
`f9a141c57c6588a63d51301820867730079ce87b`. P25u and P25v used the later
clean evidence base `929609ccde78b0c9f4e57cf2225e0ae1204149cb`. The receipt
publication base is the current main commit stated above.

P25r measured the authenticated Go `recovery_deletion` edit from the pinned
canonical incremental manifest. The source uses the Go grammar and the
`rewrite` fixture. The Docker container used four GiB of memory, one CPU,
`GOMEMLIMIT=3GiB`, `GOMAXPROCS=1`, `GOFLAGS=-p=1`, and a 15-minute test limit.
The 750-millisecond run completed 193 iterations at `4,351,195 ns/op`,
`1,331,622 B/op`, and 63 allocations per operation.

P25s's first one-iteration profile recorded `5,696,921 ns` parse wall time,
`5,530,209 ns` parser-loop time, `4,979,592 ns` reparse time, 123 reused
subtrees, 2,745 reused bytes, 11,564 new nodes, 1,300 tokens, 15 maximum
stacks, 257 single-stack iterations, and 1,294 multi-stack iterations. It
recorded one accepted-error retry attempt and did not adopt that retry.

P25s isolated the same forward path. Its one-iteration benchmark reported
`6,155,260 ns/op`, `3,102,704 B/op`, and 120 allocations. The profile used
the same Docker limits. P25t repeated the forward path for 20 iterations.
The stable counters stayed at 123 reused subtrees, 2,745 reused bytes,
11,564 new nodes, 1,300 tokens, 15 maximum stacks, 257 single-stack
iterations, and 1,294 multi-stack iterations. The stable three-run mean was
`6,165,450 ns/op`. The run-mean range was `5,863,544` to `6,500,572 ns/op`,
with a coefficient of variation (CV) of `4.24%`. Within-run iteration
extremes ranged from `4,854,679` to `9,648,997 ns`; do not treat these
extremes as run means. No timeout or out-of-memory event occurred.
The three warm RSS samples were `236,960`, `236,640`, and `236,320 KiB`.

P25u performed a read-only structural audit of the forward-only construction.
It rejected the construction as a performance candidate. The path proved
correctness for the pinned Go/C control, but it did not prove a generic
optimization invariant. No P25u standalone artifact exists. The focused
correctness and locked-C gates therefore remained separate from benchmark
evidence. No production or test change survived.

P25r through P25t artifacts are:

- P25r Docker metadata: `/tmp/gts-p25r-investigation/harness_out/docker/20260823T100544Z-p25r-recovery-deletion-profile/metadata.txt`, SHA-256 `a25a2ba79e7dbd5595a2f18e6983bf7edc07ff7129b7c6e054e44c93938544dd`.
- P25s first profile CPU: `/tmp/gts-p25r-investigation/harness_out/docker/p25s-forward-cpu.pprof`, SHA-256 `aa7672bbc9239674dfe25b8171f79c80893bd5b00135030c7cdc8bd8bbea1f22`.
- P25s first profile memory: `/tmp/gts-p25r-investigation/harness_out/docker/p25s-forward-mem.pprof`, SHA-256 `7cc9aa0a50f576492442db6bc7861f979f5a324a24fa9db386b5bfb151df52d3`.
- P25s first Docker metadata: `/tmp/gts-p25r-investigation/harness_out/docker/20260823T101146Z-p25s-forward-profile/metadata.txt`, SHA-256 `e8d995bc08e5c8b1c0042f7ecfc8878a83dd23c1f67a3987fcbc51ce1cbe3710`.
- P25s forward-only CPU: `/tmp/gts-p25s-forward-harness/harness_out/docker/p25s-forward-only-cpu.pprof`, SHA-256 `6ff678d4c9bc56f52a3fa0a8b354e6c49598ac1c1f6056312dc43b0cf55bc8e1`.
- P25s forward-only memory: `/tmp/gts-p25s-forward-harness/harness_out/docker/p25s-forward-only-mem.pprof`, SHA-256 `9d8a91841fc0a7814f9c88cd95a6c8f033d8126d7504e59e25753b511b93cdcc`.
- P25s forward-only Docker metadata: `/tmp/gts-p25s-forward-harness/harness_out/docker/20260823T101441Z-p25s-forward-only-profile/metadata.txt`, SHA-256 `bae57770b0e99a246e39295377bdc8f58d3a575410524826c7fe4e338d0e16c9`.
- P25t CPU: `/tmp/gts-p25t-forward-harness/harness_out/docker/p25t-forward-cpu.pprof`, SHA-256 `731815fd36ee20ee0cd70a09967a912795c47d8dcc147f23f76f062e16f16081`.
- P25t memory: `/tmp/gts-p25t-forward-harness/harness_out/docker/p25t-forward-mem.pprof`, SHA-256 `4f14c7aec62ebf0763745628b7a49c828983b2d195762478e7f3fee005e671cb`.
- P25t iteration log: `/tmp/gts-p25t-forward-harness/harness_out/docker/p25t-forward-iterations.tsv`, SHA-256 `23a41ff57766edda5b01617315d20d3ff68b9e0f33eeb2ba84af44755401407e`.
- P25t Docker metadata: `/tmp/gts-p25t-forward-harness/harness_out/docker/20260823T101908Z-p25t-forward-profile/metadata.txt`, SHA-256 `df72dfd0b4ebbc52949e4751568ec85fd9e9e7e74a410b921eb873519d29561c`.

### P25v accepted-error retry diagnostic

P25v tested one bounded hypothesis: skip the accepted-error base-merge retry
when `GOT_DIAGNOSTIC_SKIP_ACCEPTED_ERROR_BASE_MERGE_RETRY=1`. The test used
the Go `recovery_deletion` case, a clean same-length leaf control, and an
accepted-error same-line-length control. The source and edited-source hashes
are pinned in both JSON reports. Recovery uses
`74c0705f8729670559492fb5460a01b2a1a2a109928e1aeb52736e485e8ff097` to
`81543368d0ec807dd951edfdb04fb66ee9ef14ca32b0bb5c53fc8a4d0f2e7b07`.
The leaf control uses `938253936e03230126ab1aa1f78ccd919a51e20f7207f0199704144636fe5b9a`
to `f6993788c45b23a3584d971316d7493ec20cf063e393629a215e13c6322d1820`.
The same-line control uses
`009aa9fd5352c712f3839670c7df8a9b00ae878ee20dc88131a438b2d5edfd9a` to
`3e8bcd61b37b35bc8af614e6bf6e547d8909de39b8850b2d1ac955b13678962e`.
The Go and locked-C roots matched in every case. Fresh, incremental, and
cross-route deep digests were equal.

The successful Docker runs used four GiB of memory, one CPU, `GOMEMLIMIT=3GiB`,
`GOMAXPROCS=1`, `GOFLAGS=-p=1`, one test worker, and a five-minute test limit.
The final enabled run used 314,560 KiB maximum RSS. The bypassed run used
322,408 KiB maximum RSS. The first enabled construction failed during compile:
`ParseStopReason.String` was undefined. It exited with code one and is
quarantined. The enabled rerun and final enabled run passed. The bypassed run
passed. These single-run enabled or bypassed wall and RSS differences are
diagnostic, not performance evidence.

The enabled route recorded one retry attempt for `recovery_deletion`, with
merge cap three and no retry adoption. It recorded no retry for the leaf
control. The same-line control recorded one adopted retry. The bypass route
recorded zero retry attempts for all three cases. These changed retry counters
show why the toggle cannot serve as a performance candidate. They do not show
that the retry is redundant.

P25v artifacts are:

- Enabled JSON: `/tmp/gts-p25v-artifacts/p25v-enabled-final.json`, SHA-256 `5c69a5e648d54424ab972a1cd35d32a66d91aeb054004b4f895b7cc4e19754e4`.
- Bypassed JSON: `/tmp/gts-p25v-artifacts/p25v-bypassed.json`, SHA-256 `3350b1b6d3dc33ababceecb30ba150fd36c7e8072fb7841aeeb6f246e185e894`.
- Paired test source: `/tmp/gts-p25v-paired-harness/cgo_harness/p25v_retry_paired_test.go`, SHA-256 `103d43cbd613d3b2a948a01bbad449839c1a2f3bef020da766c48a9a0fa8bd7d`.
- Final enabled metadata: `/tmp/gts-p25v-artifacts/20260823T103537Z-p25v-enabled-final/metadata.txt`, SHA-256 `dcc7acfa59333567635e21099d28cf3b3c89440ee186548395fb1a065edd5195`.
- Bypassed metadata: `/tmp/gts-p25v-artifacts/20260823T103501Z-p25v-bypassed/metadata.txt`, SHA-256 `8a961b6c58c8fb6b7f9f496d9959a1e37f6e23697b6cdca207466e83ff8e1ee8`.
- Failed construction metadata: `/tmp/gts-p25v-artifacts/20260823T103346Z-p25v-enabled/metadata.txt`, SHA-256 `60d3c00a5f60dcb2a65f1a5d3b2c3580fea7b4c33326494b900f7904353d5ad`; quarantine this artifact.
- Failed construction log: `/tmp/gts-p25v-artifacts/20260823T103346Z-p25v-enabled/container.log`, SHA-256 `fe804510ba96f52ff2e66877ea9ead3c440c4331adfb143bc7e25cb581ccbce1`; it contains the compile error.

P25v was a candidate hypothesis only. P25w superseded it. No production or
test change ships, and no randomized performance gate ran.

### P25w independent exact-route defect

P25w reran the P25v bypass hypothesis with an independent parser test. The
enabled route produced equal fresh, incremental Go, fresh C, and incremental
C digests: `d5af3b86c49049bf95e1ebf5a098a4194a3d17d08b233c5fb9e5bfd90e37c801`.
The bypassed Go incremental digest was
`a8b8e0eeb01df1f70d692f1099bd038a9b092dc8bf4baec3791ed156c7444fb7`.
The bypassed route produced a root `ERROR` with 196 children and a first error
span `[39332,39426)`. Fresh Go and both C routes stayed clean with 195
children. The independent route therefore exposed a correctness defect.

The route used case `language/same_line_length_change`. Its source SHA-256 is
`009aa9fd5352c712f3839670c7df8a9b00ae878ee20dc88131a438b2d5edfd9a`.
Its edited-source SHA-256 is
`3e8bcd61b37b35bc8af614e6bf6e547d8909de39b8850b2d1ac955b13678962e`.
The enabled JSON is `/tmp/gts-p25w-artifacts/results/p25w-exact-enabled.json`,
SHA-256 `65cdca44c94895cb1cee8e12d343fc3823a4be474ede80cec50bfb5dfd1a1fad`.
The bypassed JSON is `/tmp/gts-p25w-artifacts/results/p25w-exact-bypassed.json`,
SHA-256 `d4ec9de7014725338850d401ac4af1a2b2367b60bfc173138310ce2d907e75d4`.
The independent test source is
`/tmp/gts-p25w-canonical/cgo_harness/p25w_exact_language_route_test.go`,
SHA-256 `f6f14ed2a935abb846b10c98ee775c0591c3ae0b20ef3bb91fecb852ce1190a3`.

The enabled exact-route Docker run used four GiB of memory, one CPU,
`GOMEMLIMIT=3GiB`, `GOMAXPROCS=1`, `GOFLAGS=-p=1`, one test worker, and a
five-minute limit. It passed with maximum RSS `318,240 KiB`. The bypassed
run used the same limits, reported maximum RSS `233,600 KiB`, and failed the
deep-digest assertion. Neither run timed out or exhausted memory. The exact
enabled metadata is `/tmp/gts-p25w-artifacts/20260823T104558Z-p25w-exact-enabled/metadata.txt`,
SHA-256 `9dea0f1e490103d6697cb7946b5c4de4006bab358e0da069b351ec3468c3638d`.
The bypassed metadata is `/tmp/gts-p25w-artifacts/20260823T104610Z-p25w-exact-bypassed/metadata.txt`,
SHA-256 `0b8c2aec33a495e12b8d107b69b4088fcd7bde3c0f3ed56279f897b30f3aba71`.

The canonical suite also reports its existing Python scanner limitation:
`python_scanner_dedent` uses the expected `external_scanner_unsupported`
fallback. Treat this as a known suite limitation, not as a P25w candidate
finding. The bypass hypothesis is discarded. P25v's shared paired diagnostic
was insufficient because it did not isolate this exact route.

### Decision and reopening condition

Reject every P25k through P25w candidate or speculative lever. No production
or test change survives. Keep issue #454 open. Do not publish a 20-seed
campaign because no candidate passed correctness and performance screening.

The Docker profile runs are performance evidence only. They do not replace
focused correctness tests or locked-C parity. Run those gates before any
future candidate benchmark.

Reopen this lane only when a fresh profile identifies one bounded,
grammar-agnostic operation, including a condense operation. Preserve the
operation's ownership, rollback, and output invariants. For an accepted-error
retry predicate, first prove full authenticated canonical correctness and
locked-C correctness. Include fresh, incremental, recovery, field, raw-shape,
and memory-safety checks. Then run six-seed screens.
Run the 20-seed primary and authenticated GLR campaigns only after a
significant target win with neutral bytes, allocations, maximum resident set
size (RSS), and work counters.

## 2026-08-23 P25h-P25j parser-core dispatch copy blocker

Status: **KEEP ISSUE #454 LIVE / NO-GO**. Ship no code.

The publication base is main commit
`526815a853713ef8af114170f87e94eed6438e85`.

P25h uses evidence base
`41d0b9de133de777aeba9c1dca091903da052a7f`. P25i uses evidence base
`137860ebd80921094e5a8069007d49188dcb5e50`. P25j uses evidence base
`5d39d9658f5071c5c0f476eaadc6ae067e6c77e1`.

### Fresh profile and bounded candidate

The fresh profile used one quiet Docker CPU, `GOMAXPROCS=1`, and a three-second
capture. It used the authenticated Go `grammargen_lr` workload.

The profile placed `13.25%` flat CPU time in `runtime.duffcopy` and `3.31%`
in `dispatchPassActive`. The latter had `58.94%` cumulative CPU time. The
profile also showed `2.65%` flat time in materialization.

The fresh profile is at
`/tmp/gts-p25h-investigation/harness_out/docker/20260823T075254Z-p25h-fresh-attribution`.
Its shipped `grammargen_lr` profile SHA-256 is
`fc8187d35fcb1dc99717193e9a689bf3567719c58e675bceb1507a9a64db2590`.
The profile manifest SHA-256 is
`a881ae0263b988490408cdb7b3241e3a19d54e09e11f3b7249d92abf58dd6899`.

The candidate changed only `parsercore_phase0_driver.go`. It introduced a
value snapshot with five fields: `head`, `s3Region`, `shifted`, `accepted`,
and `paused`. The loop read and wrote these fields through the existing header
array. The resume path rebuilt the same value snapshot after each mutation.
This design avoided pointer aliasing.

The candidate preserved the loop's value-snapshot semantics. It did not retry
the rejected indexed pointer approach. The raw diff SHA-256 is
`1a21f58236d65b6f2c91d74c6a9be322c4d0e095a929452218b60ebfcfeae776`.
The candidate source SHA-256 was
`420ebab3f073e140571c7e0fe99a8c8dfe52d57e3fcb27778218c15b0b5def3a`.
The baseline source SHA-256 was
`dd711d956b3174a1281acc121766ef31d2d1b8b86a191230589200456e0f3211`.

Canopy traced `dispatchPassActive` and the new snapshot type in the changed
file. The trace covered every header field read, write, and resume reload.
The trace also covered boundary classification, recovery, cell construction,
and downstream materialization calls.

- Canopy symbol trace: direct `canopy search symbols` with `--no-cache`.
- Impact graph: `/tmp/p25h-impact.json`.
- Call graph: `/tmp/p25h-calls.json`.
- Dispatch field trace: `/tmp/p25h-dispatch.txt`.

The candidate profile is at
`/tmp/gts-p25h-investigation/harness_out/docker/20260823T075754Z-p25h-candidate-attribution`.
Its shipped `grammargen_lr` profile SHA-256 is
`a00a9e360f19b4312584a4623013ddcca0f0b9bb8a6e6e031154188a39db6328`.
The candidate profile manifest SHA-256 is
`c857f2bfc3bf7c179a17a6c3f7e3af1a8646e46e309f3feb76c91a02a4cb9093`.

### Correctness and parity

The action conversion test passed. The exact rewrite and materialization test
passed. The candidate attribution capture passed. The focused scheduler test
failed with `ConvergedReductionSplitDrops=6`; the existing test expects `5`.

The baseline focused run reported the same failure and the same value of `6`.
Treat this result as a baseline-reproduced test limitation, not a candidate
finding.

- Candidate focused tests: `/tmp/gts-p25h-investigation/harness_out/docker/20260823T075521Z-p25h-candidate-focused`
- Baseline focused tests: `/tmp/gts-p25h-baseline/harness_out/docker/20260823T080901Z-p25h-baseline-focused`
- Candidate attribution: `/tmp/gts-p25h-investigation/harness_out/docker/20260823T075754Z-p25h-candidate-attribution`

The one-language Go parity run reproduced the known locked-C result. It
reported `24/25` deep-parity samples. The first difference remained the
`declarations.txt` parameter span. The candidate added no divergence.

- Candidate parity: `/tmp/gts-p25h-investigation/harness_out/docker/20260823T075557Z-diag-go_lang`

The Docker metadata recorded one CPU, four GiB memory, `GOMEMLIMIT=3GiB`,
`GOMAXPROCS=1`, and `GOFLAGS=-p=1`. The focused and parity runs reported no
timeout and no out-of-memory event.

### Randomized performance result

The primary protocol used 20 seeds. Each seed used one process,
`GOMAXPROCS=1`, `GOFLAGS=-p=1`, `-count=1`, `-benchtime=750ms`, `-benchmem`,
and one shuffle seed.

| Benchmark | Candidate minus baseline | p-value | Result |
|---|---:|---:|---|
| Full parse | `-1.34%` | `<0.001` | Improvement |
| Single-byte edit | `-0.29%` | `0.147` | Neutral |
| No edit | `-0.75%` | `<0.001` | Improvement |
| Trio geometric mean (geomean) | `-0.79%` | — | Improvement |

Full-parse bytes per operation fell `2.36%` (`p=0.001`). Incremental bytes
per operation stayed at zero. Allocation counts stayed unchanged at four for
full parsing and zero for both incremental lanes.

- Baseline primary output: `/tmp/gts-p25h-baseline/harness_out/p25h-baseline-primary.txt`
- Candidate primary output: `/tmp/gts-p25h-investigation/harness_out/p25h-candidate-primary.txt`
- Baseline primary SHA-256: `7236e7f49e146029002584942efb00a4b5a396e8634642f094059b4a4e95c5b9`
- Candidate primary SHA-256: `f4dca94d250374bd3ce2c6aa893b5af0df9b585d9de6402a2717b68eee22e557`

The authenticated `grammargen_lr` control used the same 20-seed protocol.
Generalized LR (GLR) parsing means generalized parsing with
multiple active stacks.

| Metric | Candidate minus baseline | p-value |
|---|---:|---:|
| Time per operation | `+11.03%` | `<0.001` |
| Bytes per operation | `+16.61%` | `<0.001` |
| Allocations per operation | neutral at `156` | `0.231` |

Structural counters stayed equal. Both runs reported `56.43M` arena bytes,
zero forest fast-path uses, `18` maximum stacks, `53.12k` multi-iterations,
`47.98k` multi-tokens, `330.5k` nodes, zero normalization runs, `58` peak
depth, and `49.91k` tokens.

- Baseline GLR output: `/tmp/gts-p25h-baseline/harness_out/p25h-baseline-glr.txt`
- Candidate GLR output: `/tmp/gts-p25h-investigation/harness_out/p25h-candidate-glr.txt`
- Baseline GLR SHA-256: `7e790b96b94f1a22288a77f4f1abbeaa13ed138c6875bf93003e48958ee99aae`
- Candidate GLR SHA-256: `5b542313530087333a78ac069da295bcd2e324c0717bd2930dca0507a945c0ca`

The first cold RSS run included a candidate compile outlier. The controlled
warm run built each test binary before measurement. Its three samples were:

| Run | Samples in KiB | Median |
|---|---:|---:|
| Baseline | `602880`, `600000`, `580000` | `600000` |
| Candidate | `600960`, `610240`, `612160` | `610240` |

The warm candidate median increased `1.71%`. The warm RSS artifacts are:

- Baseline: `/tmp/gts-p25h-baseline/harness_out/docker/20260823T081017Z-p25h-baseline-rss-warm`
- Candidate: `/tmp/gts-p25h-investigation/harness_out/docker/20260823T081040Z-p25h-candidate-rss-warm`

The candidate and baseline runs reported no crash, out-of-memory event, or
wall timeout. The candidate did not retain a production change.

### P25i scalar follow-up diagnostic

P25i used evidence base
`137860ebd80921094e5a8069007d49188dcb5e50` and re-applied the P25h candidate
temporarily.
It compared compiler reports, assembly, and allocation-space profiles. It did
not create a production change or a 20-seed publication.

The compiler inline costs were:

| Variant | Inline cost | Escape result |
|---|---:|---|
| Baseline | `3572` | Scheduler receiver leaks to heap |
| P25h aggregate | `3631` | Same result |
| P25i scalar | `3613` | Same result |

The assembly comparison was:

| Variant | Lines | `MOVUPS` | `MOVQ` | Calls | Stack frame |
|---|---:|---:|---:|---:|---:|
| Baseline | `2019` | `342` | `644` | `79` | `0x538` |
| P25h aggregate | `2057` | `346` | `645` | `85` | `0x510` |
| P25i scalar | `1997` | `342` | `616` | `77` | `0x500` |

The P25h aggregate added stack copy and spill sequences. The compiler did not
report a new header allocation. The assembly and profile evidence support
copy and spill cost as the cause of the P25h regression.

The P25i scalar form read `shifted`, `accepted`, and `paused` into scalars.
It kept value snapshots for `head` and `s3Region`. It reloaded both values
after an error-region resume. Its raw diff SHA-256 is
`63eb4b41af5b601dd5105887918ab1024fbe2d96dacbf56e15ccd3ae84ec0cd7`.

The focused action, rewrite, and materialization tests passed. The scheduler
test failed on both baseline and scalar runs with `ConvergedReductionSplitDrops=6`;
the existing test expects `5`. The scalar Go parity run preserved the known
`24/25` deep-parity result and the same `declarations.txt` span divergence.

- Scalar focused test: `/tmp/gts-p25i-investigation/harness_out/docker/20260823T082832Z-p25i-scalar-focused`
- Scalar parity: `/tmp/gts-p25i-investigation/harness_out/docker/20260823T082839Z-diag-go_lang`
- Baseline escape report: `/tmp/gts-p25i-baseline/harness_out/p25i-baseline-escape.txt` (SHA-256 `12f03d67c2a16c5fcd7f2c29ff96a3e181118ecafc939a390a6ad9119ca7736b`)
- Aggregate escape report: `/tmp/gts-p25i-investigation/harness_out/p25i-candidate-escape.txt` (SHA-256 `8b7f3838c99c88b09d60526c5d319fffe9181c48106b88d4249688cacbe6401d`)
- Scalar escape report: `/tmp/gts-p25i-investigation/harness_out/p25i-scalar-escape.txt` (SHA-256 `5525db79bc8042036a8216d46797368cccc38accc58d1e29f25b9e836d5ac5ac`)
- Baseline assembly: `/tmp/gts-p25i-baseline/harness_out/p25i-baseline-objdump.txt` (SHA-256 `9c31c466f60ea7a6718ecce8faca4e63ae39024eee43aa4760f32da0f8db22be`)
- Aggregate assembly: `/tmp/gts-p25i-investigation/harness_out/p25i-candidate-objdump.txt` (SHA-256 `9c218c616710e642be26c147ec49de401a12950b462309e33c35fb52005b949e`)
- Scalar assembly: `/tmp/gts-p25i-investigation/harness_out/p25i-scalar-objdump.txt` (SHA-256 `a21c31cb1384a2e31c868233615608c09fe83180058f7a55d921a2f593030d7c`)
- Baseline allocation profile: `/tmp/gts-p25i-baseline/harness_out/p25i-baseline-alloc.pprof` (SHA-256 `ca586b976602d05f6e596dbe09d1a567edafd5d677dc768e598cd8451d3ca673`)
- Aggregate allocation profile: `/tmp/gts-p25i-investigation/harness_out/p25i-candidate-alloc.pprof` (SHA-256 `e610c5abc43468fc9173a1269dec39110339b670e0839e92e8b324c3da4a7d1b`)

The six-seed screen used alternating baseline-first and scalar-first order.
Each run used one CPU, `GOMAXPROCS=1`, `GOFLAGS=-p=1`, `-count=1`,
`-benchtime=750ms`, and `-benchmem`. Treat this screen as diagnostic only.

Primary trio screen:

| Benchmark | Scalar minus baseline | p-value |
|---|---:|---:|
| Full parse | `-1.90%` | `0.026` |
| Single-byte edit | neutral | `0.331` |
| No edit | neutral | `1.000` |
| Trio geometric mean (geomean) | `-0.18%` | — |

Full-parse bytes per operation increased `1.85%` (`p=0.022`). Allocation
counts stayed unchanged in the screen.

Authenticated generalized LR (GLR) screen:

| Metric | Baseline | Scalar | p-value |
|---|---:|---:|---:|
| Time per operation | `133.8 ms` | `132.4 ms` | `0.937` |
| Bytes per operation | `6.251 MiB` | `6.256 MiB` | `0.513` |
| Allocations per operation | `156.5` | `157.0` | `0.859` |

Arena bytes, forest fast-path uses, maximum stacks, multi-iterations,
multi-tokens, nodes, normalization runs, peak depth, and tokens stayed equal.
The screen did not show a GLR improvement. Do not run the 20-seed campaign.

Warm three-sample maximum resident set size (RSS) was:

| Variant | Samples in KiB | Median |
|---|---:|---:|
| Baseline | `621120`, `588800`, `590720` | `590720` |
| Scalar | `621080`, `596320`, `587840` | `596320` |

The scalar median increased `0.95%`.

- Canopy symbols: `/tmp/p25i-symbols.json` (SHA-256 `daae9f3e6b7ffb00be6e7ec040a16b66526d02cedb59c1bc7a14fded5487274e`)
- Canopy calls: `/tmp/p25i-calls.json` (SHA-256 `a641a797a23c0ef571ee3582414b018934484e73e1492513e7f94073db53bdba`)
- Canopy impact: `/tmp/p25i-impact.json` (SHA-256 `bc7d6b0d274b58cee95f9e63e588ec51f2cc24355a68eb26e3ce9259ba4632d5`)
- Alternating screen artifacts: `/tmp/gts-p25i-investigation/harness_out/p25i-screen`
- Warm RSS baseline: `/tmp/gts-p25i-baseline/harness_out/docker/20260823T083306Z-p25i-baseline-rss-warm`
- Warm RSS scalar: `/tmp/gts-p25i-investigation/harness_out/docker/20260823T083324Z-p25i-scalar-rss-warm`

### P25j distinct-copy follow-up diagnostic

P25j used evidence base
`5d39d9658f5071c5c0f476eaadc6ae067e6c77e1` and a clean isolated worktree at
`/tmp/gts-p25j-investigation`. The baseline worktree was
`/tmp/gts-p25j-baseline` at the same commit.

The publication base was
`526815a853713ef8af114170f87e94eed6438e85`.

The fresh authenticated profile used one quiet Docker CPU,
`GOMAXPROCS=1`, `GOFLAGS=-p=1`, a three-second benchmark, and
`BenchmarkParserCoreFreshFullCanonical/grammargen_lr`.

The profile measured `11.82%` flat CPU time in `runtime.duffcopy`. The total
was `0.48` seconds in `runtime.duffcopy`. Canonical linear grouping accounted
for `0.07` seconds of that total. Keep these two values separate.

The assembly showed one distinct 224-byte copy at the `linearGroup` literal
assignment in `canonicalizeLinearCheckedWithMutation`. The candidate changed
only `parsercore_phase0_driver.go`. Its raw diff SHA-256 is
`a055e19160f401b266c6afb43a4be8e24bd058434c05ba6b3050b3629b27404b`.

Each inline group slot starts zeroed and is used once per call. Field-wise
initialization therefore preserves the literal's zero-value fields. It removes
the targeted 224-byte assembly copy without changing group ownership or values.
The assembly still contains the existing dispatch header copies. The candidate
did not retry the rejected indexed header pointer.

The fresh profile artifacts are:

- CPU profile: `/tmp/gts-p25j-investigation/harness_out/p25j-profile/grammargen_lr.cpu.pprof` (SHA-256 `8c6d0e5e3869bf1d9da41b3abea49fdc9722ccccf789938290241578445d11c5`)
- Allocation profile: `/tmp/gts-p25j-investigation/harness_out/p25j-profile/grammargen_lr.alloc.pprof` (SHA-256 `341ef74b8d622e5c6e3626f20b03acec0de107db63f3eb492ef70720f13a834`)
- Docker metadata: `/tmp/gts-p25j-investigation/harness_out/docker/20260823T084133Z-p25j-authenticated-profile/metadata.txt` (SHA-256 `99cc9af90cb4db78e4e361cf4465032eb46c46614d38f739e84d976fa8002b5e`)

The benchmark reported `89.588 ms/op`, `8,276 B/op`, and `123 allocs/op`.
The allocation profile is process-wide. Use benchmark B/op and allocs/op for
the timed operation values.

The focused Docker correctness run passed all nine canonical-scratch tests.
It used one CPU, four GiB memory, `GOMEMLIMIT=3GiB`, `GOMAXPROCS=1`, and
`GOFLAGS=-p=1`. It reported no timeout or out-of-memory event.

- Correctness artifact: `/tmp/gts-p25j-investigation/harness_out/docker/20260823T084901Z-p25j-canonical-correctness-tags`
- Correctness metadata SHA-256: `50db1ffa0d44e94f854688943f0d43868edef89c83e5b21c4c9ab10ea39c25b7`

The first six-seed screen used one process per seed, one CPU,
`GOMAXPROCS=1`, `GOFLAGS=-p=1`, `-count=1`, `-benchtime=750ms`, and
`-benchmem`. It included the primary deterministic finite automaton (DFA)
trio and the authenticated `grammargen_lr` generalized LR (GLR) control.
The combined screen geomean changed by `-0.21%` (`p=0.589`, `n=6`).

- Baseline output: `/tmp/gts-p25j-baseline/harness_out/p25j-baseline-screen.txt` (SHA-256 `65dd23929f7b3a8429904720278e4a7b5fa1e2838f29d1125b2b2036e0abcdd8`)
- Candidate output: `/tmp/gts-p25j-investigation/harness_out/p25j-candidate-screen.txt` (SHA-256 `06c41f63e1d93c854143f971389dfa17c192f54c86b5cee4b7ce0776961ce69d`)
- Docker artifact: `/tmp/gts-p25j-investigation/harness_out/docker/20260823T084946Z-p25j-candidate-screen`

The order-balanced screen alternated baseline-first and candidate-first
containers for seeds 1 through 6. Its geomean changed by `-0.07%`.
The primary lanes were neutral: full parse changed `+0.34%`, single-byte
edit changed `-0.06%`, and no-edit changed `-0.20%`. The authenticated
`grammargen_lr` control changed `-1.16%` (`p=0.394`, `n=6`). These results
did not establish a target improvement.

The parser-core B/op and allocs/op values stayed equal. Full-DFA B/op noise
changed sign between the two screen orders. The compact work counters stayed
identical for every authenticated parser-core sub-benchmark.

- Order-balanced benchstat: `/tmp/p25j-order-benchstat.txt` (SHA-256 `48033a28d28a5638a00682662941977a440a8c7d817f120b3225dc584571dcb5`)
- Candidate order artifacts: `/tmp/gts-p25j-investigation/harness_out/p25j-order-candidate-seed1.txt` through `p25j-order-candidate-seed6.txt`
- Baseline order artifacts: `/tmp/gts-p25j-baseline/harness_out/p25j-order-baseline-seed1.txt` through `p25j-order-baseline-seed6.txt`

The candidate failed the six-seed improvement gate. Do not run a 20-seed
campaign or an RSS campaign for this rejected candidate. No production or
test change survives. Both P25j worktrees are clean.

### Decision and reopening condition

Reject the aggregate, scalar, and linear-group value projections. The aggregate regresses
authenticated GLR time, bytes per operation, and RSS. The scalar screen is
neutral on GLR and nearly neutral on the primary trio. The P25j screen is
neutral on both the primary trio and authenticated GLR.

Keep issue #454 open. Ship no parser or test change. P25h completed the
20-seed primary and authenticated GLR campaigns. P25i and P25j stopped after
six-seed diagnostic screens. Reopen this lane only after a fresh quiet
profile identifies a distinct 224-byte or larger copy outside the rejected
header snapshots, and an order-balanced screen improves the target without
changing B/op, allocs/op, RSS, or parser-work counters.

Preserve value-snapshot semantics without pointer aliasing. Prove equal work,
raw shape, fields, fragility, recovery, materialization, incremental behavior,
and locked-C parity. Repeat the 20-seed primary trio, the 20-seed authenticated
GLR control, and three warm RSS runs. Reject any result with a crash,
out-of-memory event, timeout, or retention regression.

The isolated worktrees are code-clean at publication base
`526815a853713ef8af114170f87e94eed6438e85`.

## 2026-08-23 P25g parser-core dispatch blocker

Status: **KEEP ISSUE #454 LIVE / NO-GO**. Ship no code.

The publication base is main commit
`54c7f521505e23b7a32c84c2a14d3bd3175c09dd`. The performance evidence uses
base commit `1c30650814ec6e65cbf31184301bf4776f3e5f41`.

### Fresh profile and hypothesis

The fresh profile used one quiet Docker CPU, `GOMAXPROCS=1`, and a three-second
capture. It used the authenticated Go `grammargen_lr` workload.

The profile placed `12.29%` flat CPU in `runtime.duffcopy` and `5.98%` flat CPU
in `dispatchPassActive`. It placed `1.33%` flat CPU in
`applyGenericReductionOwned` and `2.99%` in materialization.

The profile artifact is
`/tmp/gts-p25g-investigation/harness_out/docker/20260823T072006Z-p25g-fresh-attribution`.
The shipped `grammargen_lr` profile SHA-256 is
`b1e190976237e4d344555042468a05faf44843aa1433d86fcb839d0ccb900bb5`.

The rejected hypothesis changed only `dispatchPassActive`. It replaced one
224-byte header value copy with indexed access. The raw diff SHA-256 is
`7c9b90937af20c0d5ffb5768e224fe718201a2592f2f4cf1469e50da40fbac65`.
The rejected source change was removed from its isolated worktree. No code
ships.

### Correctness and parity

The candidate attribution capture passed at
`/tmp/gts-p25g-investigation/harness_out/docker/20260823T072827Z-p25g-candidate-attribution`.
The acceptance and materialization focused test passed within
`20260823T074121Z-p25g-candidate-scheduler-tests`.

The scheduler boundary test failed on both candidate and baseline. Both runs
reported `ConvergedReductionSplitDrops=6`; the existing test expects `5`.
Treat this as a baseline-reproduced test limitation, not a candidate finding.

The Go locked-C run reproduced the same known divergence on both sides. It
reported `24/25` deep-parity samples. The first difference was the same
`declarations.txt` parameter span. Candidate and baseline produced no new
divergence.

- Candidate parity: `/tmp/gts-p25g-investigation/harness_out/docker/20260823T072904Z-diag-go_lang`
- Baseline parity: `/tmp/gts-p25g-baseline/harness_out/docker/20260823T073012Z-diag-go_lang`
- Candidate focused tests: `/tmp/gts-p25g-investigation/harness_out/docker/20260823T074121Z-p25g-candidate-scheduler-tests`
- Baseline focused tests: `/tmp/gts-p25g-baseline/harness_out/docker/20260823T074142Z-p25g-baseline-scheduler-tests`

### Performance result

The primary protocol used 20 seeds. Each seed used one process,
`GOMAXPROCS=1`, `GOFLAGS=-p=1`, `-count=1`, `-benchtime=750ms`,
`-benchmem`, and one shuffle seed.

| Benchmark | Candidate minus baseline | p-value | Result |
|---|---:|---:|---|
| Full parse | `+6.29%` | `0.043` | Regression |
| Single-byte edit | `+7.99%` | `0.038` | Regression |
| No edit | Neutral | `0.073` | No change |
| Trio geomean | `+7.02%` | — | Regression |

Full-parse bytes per operation rose `+9.49%` (`p=0.028`). Allocation counts
stayed unchanged in the primary trio.

- Baseline primary output: `/tmp/gts-p25g-investigation/harness_out/p25g-baseline-primary.txt`
- Candidate primary output: `/tmp/gts-p25g-investigation/harness_out/p25g-candidate-primary.txt`
- Primary benchstat: `/tmp/gts-p25g-investigation/benchstat-p25g-primary.txt`

The authenticated `grammargen_lr` control used the same 20-seed protocol.

| Metric | Candidate minus baseline | p-value |
|---|---:|---:|
| Time per operation | `+54.28%` | `<0.001` |
| Bytes per operation | `+74.57%` | `<0.001` |
| Allocations per operation | `+0.64%` | `0.005` |

Structural counters stayed equal. Both runs reported `330,478` nodes,
`49,912` tokens, `18` maximum stacks, and `58` peak depth.

- Baseline GLR output: `/tmp/gts-p25g-baseline/harness_out/p25g-baseline-glr.txt`
- Candidate GLR output: `/tmp/gts-p25g-investigation/harness_out/p25g-candidate-glr.txt`
- GLR benchstat: `/tmp/gts-p25g-baseline/benchstat-p25g-glr.txt`

The three-sample maximum resident set size (RSS) medians were `607040 KiB`
for baseline and `618720 KiB` for candidate. The median change was `+1.92%`.

- Baseline RSS: `/tmp/gts-p25g-baseline/harness_out/docker/20260823T073737Z-p25g-baseline-rss`
- Candidate RSS: `/tmp/gts-p25g-investigation/harness_out/docker/20260823T073712Z-p25g-candidate-rss2`

All accepted Docker runs reported no out-of-memory event and no wall timeout.
The first RSS construction attempt expanded its shell variable incorrectly.
Exclude `/tmp/gts-p25g-investigation/harness_out/docker/20260823T073705Z-p25g-candidate-rss`.

### Decision and reopening condition

Reject the indexed header access. It regresses the primary trio, the
authenticated generalized LR (GLR) control, bytes, and RSS.

Keep issue #454 open. Do not change parser code, tests, or registry state.

Reopen this lane only after a fresh quiet profile identifies a field-level
header projection that avoids the 224-byte value copy without pointer aliasing.
Prove equal parser work, raw shape, fields, fragility, recovery, materialization,
incremental behavior, and locked-C parity before another benchmark campaign.

The receipt records no accepted candidate. It records no production code.

## 2026-08-23 P25f graph-structured stack (GSS) entry hash candidate

Status: **CANDIDATE ACCEPTED FOR REVIEW**. Keep issue #454 open.

The publication base is main commit
`5e307cb2509fa5b4c6b0e50aba7454f5ec6748a3`.
The performance evidence uses base commit
`f0904533b6398775d5df5e01bc34d32feee34900`.

The candidate changes only `glr_gss.go`. It removes duplicate dynamic-
precedence dispatch in `gssEntryHash`. The switch reads the same field for
`Node`, `noTreeNode`, `compactFullLeaf`, and `pendingParent` entries. Nil and
unknown entries keep the existing sentinel path. The raw diff SHA-256 is
`7ba71f7d82f3273e7c1028ea23034bfffb18ba1fb2e42714713e8e1566edb5c3`.
Use this command to reproduce the digest:

```sh
git -c core.abbrev=8 diff --no-ext-diff --no-color --binary 5133fcd1^ 5133fcd1 -- glr_gss.go | sha256sum
```

This command produces the recorded
`7ba71f7d82f3273e7c1028ea23034bfffb18ba1fb2e42714713e8e1566edb5c3` digest.

### Performance receipt

The primary trio used one Go grammar and 20 sequential seeds. Each seed used
one process, `GOMAXPROCS=1`, `GOFLAGS=-p=1`, `-count=1`, `-benchtime=750ms`,
`-benchmem`, and its own shuffle seed. Docker used one CPU, four GiB of
memory, and `GOMEMLIMIT=3GiB`.

| Benchmark | Candidate change | Result |
|---|---:|---|
| Full parse | `+0.16%` | neutral (`p=0.718`, `n=20`) |
| Incremental single-byte edit | `+0.21%` | neutral (`p=0.473`, `n=20`) |
| Incremental no-edit | `-3.86%` | significant (`p=0.000`, `n=20`) |
| Trio geometric mean (geomean) | `-1.18%` | — |

Bytes per operation and allocations per operation stayed unchanged. Full
parses used `53.67 KiB/op` and four allocations. Both incremental lanes used
zero bytes and zero allocations. The primary raw outputs are:

- `/tmp/gts-p25f-baseline/p25f-primary-baseline-final.txt`
- `/tmp/gts-p25f-candidate/p25f-primary-candidate-final.txt`

The real `grammargen_lr` run improved by `-3.13%` (`p=0.004`, `n=20`).
Bytes per operation were effectively neutral. The candidate reported `157`
allocations in two samples. Its other samples reported `156` allocations.

The three-sample maximum resident set size (RSS) medians were `615840 KiB` for
the baseline and `593760 KiB` for the candidate. The median change was
`-3.59%`.

- Baseline RSS: `/tmp/gts-p25f-baseline/harness_out/docker/20260823T063727Z-p25f-rss-baseline-repeat`
- Candidate RSS: `/tmp/gts-p25f-candidate/harness_out/docker/20260823T063750Z-p25f-rss-candidate-repeat`

The one-sample P25f values were `602720 KiB` for the baseline and `595040 KiB`
for the candidate. No accepted run timed out or exhausted memory.

### Attribution and correctness

The P25e CPU profile supplies the candidate attribution. It was collected at
base commit `14f6692fac65eab817f65af8cc6072e423ca6563`. Its SHA-256 is
`96bead7a8c448a597e4986839b000b602a08bf72b5ffaa5685fa72dcc432715c`.
The profile path is
`/tmp/gts-p25e-investigation/harness_out/p25e-real-go-grammargen.pprof`.
The profile reports `gssEntryHash` at `3.67%` flat CPU and `7.35%` cumulative
CPU. P25f did not create a new CPU profile. Keep this provenance explicit.

The accepted correctness artifacts are:

- `/tmp/gts-p25f-artifacts/20260823T061027Z-p25f-candidate-correctness`
- `/tmp/gts-p25f-artifacts/20260823T061054Z-p25f-candidate-locked-c`
- `/tmp/gts-p25f-artifacts/20260823T061605Z-p25f-candidate-path-tests`

The current-main reruns passed at:

- `/tmp/gts-p25f-publication-20260824/harness_out/docker/20260823T064341Z-p25f-publication-go-correctness`
- `/tmp/gts-p25f-publication-20260824/harness_out/docker/20260823T064348Z-p25f-publication-go-locked-c`
- `/tmp/gts-p25f-publication-20260824/harness_out/docker/20260823T064403Z-p25f-publication-path-tests`

The focused path tests cover hash semantics, dynamic precedence, linear and
forked GSS paths, materialization, raw-shape handling, forest parsing, and
incremental parsing. The candidate and baseline runtime probes report the
same accepted parse, budget, retry, recovery, stack, node, arena, scratch,
and GSS telemetry.

The initial candidate runtime probe failed because it referenced unavailable
`ParseRuntime` fields. Exclude its artifact:
`/tmp/gts-p25f-artifacts/20260823T061724Z-p25f-candidate-runtime`.
The corrected candidate probe passed at
`/tmp/gts-p25f-artifacts/20260823T061757Z-p25f-candidate-runtime`.

Keep issue #454 open. Review the candidate on current main before merge.

## 2026-08-23 P25e one-Go-grammar fresh-profile evidence receipt

Status: **KEEP ISSUE #454 LIVE / NO-GO**. Ship no candidate code.

The evidence was collected at main commit
`14f6692fac65eab817f65af8cc6072e423ca6563`.

This publication is based on main commit
`2b755f744aef8dd253a4415ca4a5816fa85b0dbb`.

The workload used one Go grammar. It used four authenticated Go source
fixtures. It measured fresh parsing only.

### Fixture identity

The fixture registry records these identities:

| Fixture | Bytes | Source SHA-256 | Deep tree SHA-256 |
|---|---:|---|---|
| `query_compile` | 20,168 | `b788ee19b0075f0b9b567a9f93ea657e715bc8a6a40a99d3ca5c761404e71894` | `ecc090a83a4343a1c7c2afbad63277f5b4d60c42d8d94a2af2a9b16e46f2ccb5` |
| `rewrite` | 5,116 | `74c0705f8729670559492fb5460a01b2a1a2a109928e1aeb52736e485e8ff097` | `b3f9814b65763642d4eac58b9065018048ea13e6f10d56afb28a0479bf5a68a1` |
| `language` | 41,387 | `009aa9fd5352c712f3839670c7df8a9b00ae878ee20dc88131a438b2d5edfd9a` | `583df223904fe414c33bba3b474c6557ecdb20e7f47e304b9a09bfcc2da44539` |
| `grammargen_lr` | 235,626 | `a7e4a1a64b25a60aea36183b9d6d53dcd9240942cdb10e67a3cf9e6ce30f95b2` | `1472cfd9a014d4034dbc1456afd12c282630ef787c3543cf0cecb73619883ad2` |

The fixture source revision is
`a5df0aa5b3c5b20ce12bb21250bd166b9f47bd68`.

The locked Go grammar commit is
`2346a3ab1bb3857b48b29d779a1ef9799a248cd7`.

The embedded Go grammar blob SHA-256 is
`9cf914d26d962d1a62e7954f8b20b302337a44cb7d4a07218eec482c45a57a08`.

The production admission test verifies source bytes, source SHA-256, deep tree
SHA-256, grammar identity, clean roots, and production runtime identity.

### Benchmark protocol

The Docker run used 20 sequential seeds from 1 through 20. Each seed used:

- one test process;
- `GOMAXPROCS=1`;
- `GOFLAGS=-p=1`;
- `-count=1`;
- `-benchtime=750ms`;
- `-benchmem`;
- `-shuffle=<seed>`.

The benchmark container exposed four CPUs. It did not pin a host CPU.

The log contains 20 seed lines, 80 benchmark lines, and 20 successful
package results. The four sub-benchmarks are all Go fixtures.

| Fixture | ns/op mean [range] | Bytes per operation (B/op) mean [range] | Allocations per operation (allocs/op) mean [range] |
|---|---:|---:|---:|
| `query_compile` | 9,622,952.45 [9,447,310–9,815,779] | 50,662.75 [46,747–55,918] | 13 [13–13] |
| `rewrite` | 1,766,833.55 [1,744,667–1,797,896] | 2,024 [2,024–2,024] | 13 [13–13] |
| `language` | 9,186,618.25 [9,052,142–9,386,829] | 59,781.10 [58,503–65,605] | 25 [25–25] |
| `grammargen_lr` | 116,920,537 [114,068,642–121,898,438] | 5,621,264.05 [5,618,872–5,629,496] | 156.05 [156–157] |

The benchmark output is in
`/tmp/gts-p25e-investigation/harness_out/docker/20260823T050959Z/container.log`.

The run metadata is in
`/tmp/gts-p25e-investigation/harness_out/docker/20260823T050959Z/metadata.txt`.

### Resident set size

Resident set size (RSS) is process memory held in RAM.

The RSS run measured one `grammargen_lr` benchmark. It used
`GOMAXPROCS=1` and `/usr/bin/time -v`.

The Docker metadata exposed four CPUs and no CPU pin.

The maximum RSS was `601600 KiB`. This result is one sample. It is not a
20-seed RSS campaign.

The RSS log is in
`/tmp/gts-p25e-investigation/harness_out/docker/20260823T051343Z/container.log`.

### Runtime telemetry

The runtime probe reported these common values for all four fixtures:

`stop=accepted budget=536870912 source="" heap=0 sys=0 retry_initial=0
retry_legacy=0 swift_retry=0 recovery=false dropped=false fallback=false
ceilings=0/0 peaks=0/0`.

The zero heap and system fields describe budget-stop telemetry. They do not
describe total heap use.

| Fixture | Stack range | Nodes | Arena bytes | Scratch bytes | GSS bytes |
|---|---:|---:|---:|---:|---:|
| `query_compile` | 12/5,216 | 31,859 | 9,996,480 | 7,101,760 | 2,248,736 |
| `rewrite` | 12/937 | 5,569 | 9,996,480 | 7,101,760 | 2,248,736 |
| `language` | 18/4,955 | 31,697 | 11,403,912 | 7,105,200 | 2,248,816 |
| `grammargen_lr` | 18/53,125 | 330,478 | 56,654,840 | 27,813,904 | 6,705,264 |

The runtime log is in
`/tmp/gts-p25e-investigation/harness_out/docker/20260823T051725Z/container.log`.

### Profile and Canopy attribution

The profile is a CPU profile. It contains 2.45 seconds of samples from a
2.59-second run. The profile SHA-256 is
`96bead7a8c448a597e4986839b000b602a08bf72b5ffaa5685fa72dcc432715c`.

The profile path is
`/tmp/gts-p25e-investigation/harness_out/p25e-real-go-grammargen.pprof`.

The profile includes the untimed
`admitRealGoBenchmarkFixture` path. That path accounts for 18.78 percent
cumulative CPU in the profile.

| Frame | Flat CPU | Cumulative CPU |
|---|---:|---:|
| `Parser.parseInternal` | 6.94% | 89.80% |
| `Parser.applyReduceActionDispatch` | 1.22% | 29.80% |
| `Parser.applyReduceActionFromGSSTransientParents` | 2.45% | 28.57% |
| `gssEntryHash` | 3.67% | 7.35% |
| `Parser.captureRawShape` | 1.63% | 6.53% |
| `normalizeGoNewMakeTypeArgument.func1` | 1.63% | 2.04% |

Canopy found reduction dispatch callers in
`parser_default_reduce.go` and `parser_reduce.go`.

Canopy found raw-shape capture in parser reduction, recovery, and forest
paths. It found field-source parent construction in reduction, pending-parent,
and transient-parent paths.

The Go normalizer has one Go-specific caller. The broad parser frames cross
many grammars and parser paths. No profile frame identifies a safe,
grammar-agnostic candidate.

### Parity and freshness limits

The production fixture test ran in Docker at:

`/tmp/gts-p25e-investigation/harness_out/docker/20260823T050750Z`.

The Go/C preflight ran in Docker at:

`/tmp/gts-p25e-investigation/harness_out/docker/20260823T050833Z`.

The Go/C preflight used the compact candidate backend. It does not prove
production-route parity.

The fresh benchmark does not call `Tree.Edit`. It does not measure reuse,
incremental parsing, retry selection, or recovery.

The campaign does not test the 137 KiB C# witness. It does not provide
issue #454 locked-C recovery proof.

### Excluded and ancillary artifacts

The `050742Z` run used the wrong package. It reported `[no tests to run]`.

The `050817Z` run failed to compile with the disabled parser-core tag. It
reported undefined diagnostic census symbols. It is excluded.

The `050801Z` run used no backend tag. It failed the backend preflight.

The `050941Z` run used one benchmark iteration. It is not campaign evidence.

The `051713Z` runtime probe used a zero budget and reported zero work. It is
not runtime evidence.

All accepted cited runs reported exit code zero, no out-of-memory event, and
no wall-clock timeout. The accepted benchmark ran sequentially. The metadata
uses the same Docker image for each cited run.

### Decision and reopening condition

Keep issue #454 live. Ship no parser or test change from P25e.

Reopen P25e only when a fresh profile identifies one bounded helper above
the measured noise floor. Prove the helper on production and compact routes,
fresh and incremental trees, and locked-C recovery.

Then run the exact 20-seed, 40-process protocol with one CPU, alternating
process order, `GOMAXPROCS=1`, count one, 750 milliseconds, benchmark memory
reporting, and per-process RSS.

Reopen the issue #454 correctness work only when all conditions pass:

1. Keep the 1 KiB known-divergence ratchet.
2. Derive a new grammar-agnostic semantic predicate for the leaf error flag.
3. Clear that flag only when the predicate proves C recovery semantics.
4. Preserve all unrelated grammar census and digest results.
5. Match locked C at 1, 4, 16, 64, and 137 KiB for fresh and incremental trees.
6. Pass the replace, insert, delete, parser-result, and C recovery gates.
7. Preserve the memory-budget fallback as a separate correctness concern.

The P25e evidence does not meet these conditions.

## 2026-08-23 P25a fresh-profile performance blocker receipt

Status: **KEEP LIVE / NO-GO**. Ship no candidate code.

This receipt is based on main commit
`0448715e9a80305556b687b6ecaf041da42e9d9d`.

P25a used the fresh profile settings required by P24i. The artifacts prove the
recorded commands, but they do not prove a quiet host before or during the run.
The run used one central processing unit (CPU), `GOMAXPROCS=1`, one process per
benchmark, and sequential benchmark execution. The process was pinned to CPU
19 with `taskset`. The benchmark used one count, one test CPU, benchmark memory
reporting, and a five-second profile window. The benchmark invocation used
`GOFLAGS=-p=1`.

The artifact set records benchmark process logs only. It does not measure
unrelated host activity or prove the absence of a concurrent workload.

The exact benchmark command was:

```text
taskset -c 19 env GOMAXPROCS=1 GOFLAGS=-p=1 GOT_BENCH_FUNC_COUNT=500 GOT_GLR_MAX_STACKS=6 /tmp/p25a-profile-20260824-artifacts/gotreesitter.test -test.run '^$' -test.bench '^BenchmarkName$' -test.benchtime 5s -test.count 1 -test.cpu 1 -test.benchmem -test.cpuprofile /tmp/p25a-profile-20260824-artifacts/profiles/BenchmarkName.pprof
```

P25a substituted each name from the primary trio. It ran the benchmarks
sequentially. It did not run the 20-seed publication protocol. One profile
ran for each lane. This receipt does not establish a formal noise floor.

### P25a fresh profile results

The primary trio is a historical generated straight-LR control. It does not
prove generalized LR (GLR) behavior or a real-GLR performance improvement.

| Benchmark | Time | Bytes per operation (B/op) | Allocations per operation (allocs/op) | Maximum resident set size (RSS) |
|---|---:|---:|---:|---:|
| `BenchmarkGoParseFullDFA` | 9.763 ms | 12,453 | 4 | 49,872 KiB |
| `BenchmarkGoParseIncrementalSingleByteEditDFA` | 882.5 ns | 0 | 0 | 50,668 KiB |
| `BenchmarkGoParseIncrementalNoEditDFA` | 3.212 ns | 0 | 0 | 49,868 KiB |

Full parsing had distributed cost. `runtime.duffcopy` used 9.99% flat CPU.
`Core.reduceOutputsClassifiedIntoActive` used 4.70% flat and 23.35%
cumulative CPU. The generic scheduler dispatch used 3.96% flat and 55.65%
cumulative CPU. Materialization used 2.35% flat and 10.43% cumulative CPU.
No single full-parse helper formed a safe local candidate.

The single-byte edit profile supplied the strongest target region:

- `runtime.memmove` used 21.78% flat CPU.
- `Tree.Edit` used 8.08% flat and 11.37% cumulative CPU.
- `tryTokenInvariantLeafEdit` used 3.56% flat and 47.95% cumulative CPU.
- `reuseTreeWithNewSource` used 2.05% flat and 13.15% cumulative CPU.
- `newTreeWithUniqueArenas` used 6.16% flat and 11.10% cumulative CPU.

The no-edit lane is an identity-reuse control. `Parser.ParseIncremental` used
32.98% flat and 63.03% cumulative CPU. Compatibility checks and root reuse
accounted for most remaining samples. It is not a useful local target.

### P25a candidate and proof boundary

P25a considered the `Tree.Edit` single-byte replacement and reuse path. The
path marks the affected tree path. When final child references exist, it uses
the general delta path. That path shifts child entries, stack entries, pending
parents, and points.

The token-invariant fast path checks the language, edit range, leaf range,
error and missing flags, scanner checkpoints, token symbol, and token span.
The reuse path retains arenas, clears dirty paths, and carries forest, compact,
reuse, result, and compatibility state into the new tree.

Any candidate in this region must prove all of these properties:

- Preserve linear and forked graph-structured stack (GSS) paths.
- Preserve pending-parent ownership, child references, and materialization.
- Preserve raw shape, hashes, precedence, and result selection.
- Preserve field identifiers, field sources, hidden flattening, and field order.
- Preserve fragility flags and fragile non-leaf reuse rejection.
- Preserve locked-C recovery, retries, missing tokens, and fallback decisions.
- Prove incremental trees equal fresh parses after clean and malformed edits.

The fresh profiles identify a region, but they do not identify one helper whose
edit can satisfy this proof boundary. The primary trio also lacks forked GLR
coverage. P25a therefore found no defensible candidate.

### P25a artifacts and decision

The isolated profile worktree was `/tmp/gts-p25a-profile-20260824`. The
benchmark binary SHA-256 is
`8f05f5c338ab95050dcb91c8944dd9c51995765ca12a727359b1de564fd40ac2`.

The profile artifacts are in
`/tmp/p25a-profile-20260824-artifacts/`. The `profiles` directory contains
these files:

- `profiles/BenchmarkGoParseFullDFA.pprof`
- `profiles/BenchmarkGoParseIncrementalSingleByteEditDFA.pprof`
- `profiles/BenchmarkGoParseIncrementalNoEditDFA.pprof`

The `top` directory contains the matching top reports. The same directory
contains benchmark output and `/usr/bin/time -v` logs.

P25a changed no parser or test code. It ran no Docker correctness or parity
test because no candidate survived the profile proof boundary. It ran no
20-seed, 40-process publication. The accepted decision is **KEEP LIVE / NO-GO**.

### P25a reopening condition

Reopen P25a only when a fresh quiet-host profile names one bounded helper
above a measured noise floor. Use one CPU, `GOMAXPROCS=1`, and no concurrent
CPU work. Make one local candidate edit only after the profile identifies it.
Prove linear and forked GSS, pending parents, raw shape, fields, fragility,
recovery, and incremental equality on production and compact routes. Run
focused Docker correctness and parity before any performance publication.
Then run the exact 20-seed, 40-process protocol with alternating order,
`GOMAXPROCS=1`, count one, 750 milliseconds, benchmark memory reporting, the
primary trio, and per-process RSS.

## 2026-08-22 P24i final performance blocker receipt

Status: **KEEP LIVE / NO-GO**. Ship no candidate code.

This receipt is based on main commit
`603f64155651888d46937e6b5df461873283b9a1`.

P24i reviewed the bounded performance candidates from P24a through P24h. It
found no safe bounded candidate for the next performance slot. The remaining
targets cross graph-structured stack (GSS) ownership, raw-shape selection,
field metadata, fragility, recovery, or incremental reuse boundaries.

P24i made no production or test-code change. It ran no 20-seed campaign. It
records a blocker and keeps the performance arm live.

### P24i focused baseline control

The focused Docker baseline passed at
`/tmp/gotreesitter-p24i-investigation/harness_out/docker/20260822T234737Z`.
The artifact runs these controls:

- `TestParseGoIncrementalRepeatedSingleByteEdit`
- `TestParserIncrementalArithmeticEditMatchesFreshParse`
- `TestRawShapeElisionDifferentialRecoveryFromSingleStackPrefix`

The Docker command exited zero. The metadata records no timeout and no
out-of-memory event. This artifact is a baseline control. It is not a
candidate result.

The artifact used commit `86bc8ae4641a884d94bf28dd2bf8e309367252ee`, the first
parent of the P24i experiment main. Later main commits added documentation and
one parity test. They did not change parser production code.

No P24i publication directory exists. No 20-seed, 40-process, or benchstat
receipt exists for P24i.

### P24i remaining profile attribution

The prior receipts leave these named profile ranges:

| Target | Observed profile range | Receipt |
|---|---:|---|
| `rawShapeComputeContentHash` | 47.25% cumulative CPU | P24e |
| `rawShapeChild.entry` | 24.71% cumulative CPU | P24e |
| `stackEntryNodeParseState` | 24.55% flat, 24.67% cumulative CPU | P24f |
| `prepareParseStacksForIteration` | 7.54% to 15.58% cumulative CPU | P24g |
| `applyReduceActionDispatch` | 20.00% to 28.64% cumulative CPU | P24h |
| `condenseWithOutcomeAtomic` | 9.2% cumulative CPU | Attribution board |

P24a through P24d supplied no fresh quiet-host profile range for a new
decision. These ranges do not prove that a safe local edit exists. They show
where a future profile must start.

### P24i proof obligations

Require every future bounded candidate to prove all of these properties:

- Prove equal results on linear and forked graph-structured stack (GSS) paths.
- Preserve child order, byte ranges, parser states, and selected lineage.
- Preserve pending-parent ownership, child references, flags, and materialization.
- Preserve raw-shape availability, hashes, precedence, and result selection.
- Preserve field identifiers, field sources, hidden flattening, and field order.
- Preserve fragility flags and fragile non-leaf reuse rejection.
- Preserve locked-C recovery, retries, missing tokens, and fallback decisions.
- Prove incremental trees equal fresh parses after clean and malformed edits.

The proof must cover both production and compact routes. It must cover clean,
forked, pending-parent, raw-shape, field, fragile, recovery, and incremental
fixtures. A focused unit result cannot replace these route proofs.

### P24i decision

**KEEP LIVE / NO-GO.** Do not ship a candidate or change production code. Do
not start a performance publication without a new candidate and a fresh
quiet-host profile.

### P24i reopening condition

Reopen P24i only when a fresh quiet-host profile identifies one bounded target.
The profile must use one CPU, `GOMAXPROCS=1`, and no concurrent CPU work. It
must name a function, show a range above the measured noise floor, and support
one local candidate edit.

After the proof obligations pass, run focused Docker correctness and parity.
Then run the full 20-seed, 40-process campaign with per-process RSS. Reject
the candidate when any route, proof, parity result, or required lane fails.

## 2026-08-22 P24h transient-parent dispatch predicate receipt

Status: **REJECT / NO-GO**. Ship no candidate code.

This receipt is based on main commit
`30f470f5c2bf18540f7a18b2b22a7e33b88d4e10`.

P24h tested one bounded predicate in `applyReduceActionDispatch`. The
candidate changed exactly one file:

- `parser_reduce.go`

The candidate diff SHA-256 is
`2a57aef7c0eac33b802439506ac01b92e29d3286ee5a78fc482f26d4b7630c8c`.

### P24h profiler attribution and proof boundary

The P19 attribution packet is at `/tmp/gts-p19-evidence`. It used one central
processing unit (CPU), `GOMAXPROCS=1`, and stable benchmark settings. The warm
profiles place `applyReduceActionDispatch` at 20.00% to 28.64% cumulative CPU
across the early-newline, same-line, and token scenarios.

The candidate evaluates the transient-parent predicate once per dispatch. The
old code evaluated the same predicate at both dispatch sites. Production code
sets `p.reduceScratch.transientParents` once at parse setup and does not replace
the pointer during this dispatch. The candidate therefore selects the same
transient-parent or ordinary reduction path. It changes no reduction inputs,
children, fields, parser state, or tree output.

### P24h focused correctness and parity evidence

The focused Docker tests passed:

- `/tmp/gotreesitter-p24h-investigation.xb2SWG/harness_out/docker/20260822T231855Z`

The tests covered transient-parent reduction state and temporary entry reuse.
The run exited zero without an out-of-memory event or timeout.

The candidate Go real-corpus run reported 25 of 25 no-error cases, 24 of 25
S-expression matches, and 24 of 25 deep matches. It reported the known Go
range divergence. Its artifact is
`/tmp/gotreesitter-p24h-investigation.xb2SWG/harness_out/docker/20260822T231910Z-diag-go_lang`.

The candidate Go-to-C run reported 20 of 20 no-error cases, 7 of 20 tree
matches, and 15 known divergences. Its artifact is
`/tmp/gotreesitter-p24h-investigation.xb2SWG/harness_out/grammargen_cparity/20260822_161943/container.log`.
These counts match the baseline run from the same base commit.

### P24h randomized publication

The publication is at `/tmp/gts-p24h-publication.X0MEre`.

It used 20 shuffle seeds and 40 isolated processes. Odd seeds ran the baseline
first. Even seeds ran the candidate first. Each process used:

- `GOMAXPROCS=1`
- `-count=1`
- `-benchtime=750ms`
- `-benchmem`
- one process for each seed and variant

The primary trio was:

- `BenchmarkGoParseFullDFA`
- `BenchmarkGoParseIncrementalSingleByteEditDFA`
- `BenchmarkGoParseIncrementalNoEditDFA`

The runner metadata is at
`/tmp/gts-p24h-publication.X0MEre/runner-metadata.txt`. All 40 processes
exited zero. The logs contain no failure, out-of-memory, timeout, panic, or
contamination marker.

For each seed, compute the geometric mean (geomean) across the three lanes.
For each order, take the geomean of the ten baseline seed geomeans and the ten
candidate seed geomeans. Report the candidate-to-baseline ratio minus one.

The paired analysis uses equal seeds. It uses the candidate-minus-baseline
relative change for each pair and a Student-t 95% interval over 20 pairs.

### P24h performance result

The benchstat result is at
`/tmp/gts-p24h-publication.X0MEre/benchstat.txt`.

| Benchmark | Baseline | Candidate | Result | p-value |
|---|---:|---:|---:|---:|
| Full parse | 7.036ms | 7.061ms | +0.36% | 0.620 |
| Single-byte edit | 768.5ns | 771.2ns | +0.35% | 0.172 |
| No edit | 2.537ns | 2.534ns | −0.12% | 0.909 |
| Geomean | 2.394µs | 2.399µs | +0.20% | — |

The paired intervals are in
`/tmp/gts-p24h-publication.X0MEre/paired-stats.txt`.

| Benchmark | Paired mean | 95% paired interval |
|---|---:|---:|
| Full parse | +0.124143% | [−0.532215%, +0.780502%] |
| Single-byte edit | +0.275082% | [−0.286294%, +0.836458%] |
| No edit | +0.122733% | [−0.406059%, +0.651525%] |
| Geomean | +0.170048% | [−0.231804%, +0.571901%] |

The order geomean formula reports +0.214736% for baseline-first and +0.118408%
for candidate-first. Both order results include zero in their paired
intervals.

Raw FullDFA bytes per operation (B/op) averaged 54,885.9 for the baseline and
55,190.4 for the candidate. The candidate-to-baseline ratio was +0.554696%.
Benchstat rounded both medians to 53.67 KiB. Both variants used four
allocations per operation (allocs/op) in every sample. Both incremental lanes
used zero B/op and zero allocs/op.

An auxiliary exact-setting run measured maximum resident set size (RSS) at
602,400 KB for the baseline and 601,440 KB for the candidate. The run used one
process for each variant. It does not replace the 40-process campaign.

The candidate has no significant timing improvement. Its raw FullDFA byte
mean increased. The accepted result is **REJECT / NO-GO**. No code shipped.

### P24h reopening condition

Reopen P24h only with a different profiler-backed target. The new candidate
must show a clear primary-trio opportunity and improve a measured target.
Require focused correctness, Go and C parity, the same 20-seed campaign, and
per-process RSS before publication.

## 2026-08-22 P24g conditional recovery-scratch reset receipt

Status: **REJECT / NO-GO**. Ship no candidate code.

This receipt is based on main commit
`30f470f5c2bf18540f7a18b2b22a7e33b88d4e10`.

P24g tested one bounded branch in
`prepareParseStacksForIteration`. The candidate changed exactly one file:

- `parser.go`

The candidate diff SHA-256 is
`f1bf93c698c41d06c77a123ee6ab255e7caae61f6406d7171cbeee4355bdaa01`.

### P24g profiler attribution and proof boundary

The P19 attribution packet is at `/tmp/gts-p19-evidence`. It used one central
processing unit (CPU), `GOMAXPROCS=1`, and stable benchmark settings. The warm
early-newline profile attributes 7.54% cumulative CPU to
`prepareParseStacksForIteration`. The same-line profile attributes 15.58%.

The candidate guards `resetCRecoveryMergeScratch`. The guard checks the same
four flags that the reset function clears. It calls the reset when any flag is
true. It skips four false stores when every flag is already false.

The single-stack recovery test proves that the early return leaves all four
flags false. The multi-stack path synchronizes the flags before merge work.
The candidate does not change parser state or merge decisions at either
boundary.

### P24g focused correctness and parity evidence

The focused Docker tests passed:

- `/tmp/gotreesitter-p24g-investigation/harness_out/docker/20260822T230243Z`
- `/tmp/gotreesitter-p24g-investigation/harness_out/docker/20260822T230305Z`

The tests covered the single-stack recovery reset, the one-cycle cost walk,
and recovery retry state reset. Both runs exited zero without an out-of-memory
event or timeout.

The candidate Go real-corpus run reported 25 of 25 no-error cases, 24 of 25
S-expression matches, and 24 of 25 deep matches. It reported one known range
divergence. The baseline reported the same result.

- Candidate: `/tmp/gotreesitter-p24g-investigation/harness_out/docker/20260822T230325Z-diag-go_lang`
- Baseline: `/tmp/gotreesitter-p24g-baseline.1Yk6zZ/harness_out/docker/20260822T231339Z-diag-go_lang`

The candidate and baseline Go-to-C runs each reported 20 of 20 no-error cases,
7 of 20 tree matches, and 15 known divergences. Their artifacts are:

- Candidate: `/tmp/gotreesitter-p24g-investigation/harness_out/grammargen_cparity/20260822_160406/container.log`
- Baseline: `/tmp/gotreesitter-p24g-baseline.1Yk6zZ/harness_out/grammargen_cparity/20260822_161349/container.log`

### P24g randomized publication

The publication is at `/tmp/gts-p24g-publication.5AaoUM`.

It used 20 shuffle seeds and 40 isolated processes. Odd seeds ran the baseline
first. Even seeds ran the candidate first. Each process used:

- `GOMAXPROCS=1`
- `-count=1`
- `-benchtime=750ms`
- `-benchmem`
- one process for each seed and variant

The primary trio was:

- `BenchmarkGoParseFullDFA`
- `BenchmarkGoParseIncrementalSingleByteEditDFA`
- `BenchmarkGoParseIncrementalNoEditDFA`

The runner metadata is at
`/tmp/gts-p24g-publication.5AaoUM/runner-metadata.txt`. All 40 processes
exited zero. The logs contain no failure, out-of-memory, timeout, panic, or
contamination marker.

For each seed, compute the geometric mean (geomean) across the three lanes.
For each order, take the geomean of the ten baseline seed geomeans and the ten
candidate seed geomeans. Report the candidate-to-baseline ratio minus one.

The paired analysis uses equal seeds. It uses the candidate-minus-baseline
relative change for each pair and a Student-t 95% interval over 20 pairs.

### P24g performance result

The benchstat result is at
`/tmp/gts-p24g-publication.5AaoUM/benchstat.txt`.

| Benchmark | Baseline | Candidate | Result | p-value |
|---|---:|---:|---:|---:|
| Full parse | 7.062ms | 7.129ms | +0.94% | 0.004 |
| Single-byte edit | 768.9ns | 767.3ns | no significant change | 0.606 |
| No edit | 2.554ns | 2.623ns | +2.70% | <0.001 |
| Geomean | 2.402µs | 2.430µs | +1.14% | — |

The paired intervals are in
`/tmp/gts-p24g-publication.5AaoUM/paired-stats.txt`.

| Benchmark | Paired mean | 95% paired interval |
|---|---:|---:|
| Full parse | +1.115890% | [+0.228433%, +2.003347%] |
| Single-byte edit | +0.662063% | [−0.789332%, +2.113459%] |
| No edit | +2.685050% | [+2.438164%, +2.931937%] |
| Geomean | +1.476205% | [+0.700682%, +2.251728%] |

The order split reports the following paired means:

| Benchmark | Baseline first | Candidate first |
|---|---:|---:|
| Full parse | +0.701348% | +1.530433% |
| No edit | +2.869775% | +2.500325% |
| Geomean | +1.200969% | +1.751441% |

The order geomean formula reports +1.200411% for baseline-first and +1.727965%
for candidate-first.

Benchstat reports a median of 55,399 bytes per operation (B/op) and four
allocations per operation (allocs/op) for FullDFA in both variants. Raw means
show 55,404.3 B/op for the baseline and 56,386.3 B/op for the candidate. Their
ratio is +1.772335%. The paired relative mean is +1.782949%. Candidate seed 2
reported 69,776 B/op and 5 allocs/op. Every other candidate FullDFA sample
reported 4 allocs/op.
Both incremental lanes reported zero B/op and zero allocs/op.

An auxiliary exact-setting run measured maximum resident set size (RSS) at
619,520 KB for the baseline and 589,920 KB for the candidate. The run was a
single process for each variant. It does not replace the 40-process campaign.

The candidate has significant FullDFA and no-edit regressions. The accepted
result is **REJECT / NO-GO**. No code shipped.

### P24g reopening condition

Reopen P24g only with a different profiler-backed hot path. The new candidate
must show material cost in the primary trio and must not repeat P24a through
P24f. Require focused correctness, Go and C parity, the same 20-seed campaign,
and per-process RSS before publication.

## 2026-08-22 P24f stack-entry state receipt

Status: **REJECT / NO-GO**. Ship no candidate code.

This receipt is based on main commit
`f42b88ac9014537d20d3edd76e2c9caa4330a579`.

The candidate experiment used base commit
`48e844d9a73863cb92367c4db10f02bc5c09375d`.

P24f tested one bounded hot-path change in stack-entry state extraction. The
candidate rewrote `stackEntryNodeParseState` in `no_tree_node.go`. It checks
the payload pointer once, then selects the payload type from the stack-entry
kind. It does not change parser state, reuse selection, tree construction, or
recovery decisions.

The candidate changed exactly one file:

- `no_tree_node.go`

The candidate diff SHA-256 is
`3f930b8109c3a2be5b8aabfa4161c71148aa9a15277e2407dab32976b8c8e7d1`.

### P24f profiler attribution

The P19 packet at `/tmp/gts-p19-evidence` used one central processing unit
(CPU), `GOMAXPROCS=1`, and 20 samples per scenario. Its warm early-newline
profile used 25 samples. It named `stackEntryNodeParseState` at 24.55% flat
CPU and 24.67% cumulative CPU.

P24f targets state extraction inside the reparse and rebuild component. It does
not repeat the rejected P24a through P24e candidates. Its proof boundary is
the existing stack-entry representation. Each supported payload type stores
`parseState` at the field used by the old accessor path. A nil payload and an
unknown kind still return zero.

### P24f correctness evidence

The baseline focused Docker gate passed at
`/tmp/gts-p24f-artifacts/20260822T222719Z-p24f-baseline-stack-entry-state`.
The candidate focused Docker gate passed at
`/tmp/gts-p24f-artifacts/20260822T222738Z-p24f-candidate-stack-entry-state`.
The tests covered raw-shape behavior, raw-shape reclamation, cache eviction,
and incremental equality with a fresh parse. Both runs exited zero without an
out-of-memory event or timeout.

The Go and C parity Docker gate passed at
`/tmp/gts-p24f-artifacts/20260822T222826Z-p24f-go-c-parity`. It passed the Go
generalized LR (GLR) canary and three multiline `Tree.Edit` coordinate cases.

### P24f publication protocol

The publication used 20 shuffle seeds and 40 isolated processes. Odd seeds ran
the baseline first. Even seeds ran the candidate first. Each process used
`GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`, and `-benchmem`.

The primary trio was:

- `BenchmarkGoParseFullDFA`
- `BenchmarkGoParseIncrementalSingleByteEditDFA`
- `BenchmarkGoParseIncrementalNoEditDFA`

The raw publication files are in `/tmp/gts-p24f-publication`. The aggregate
files are:

- `/tmp/gts-p24f-publication/baseline-all.txt`
- `/tmp/gts-p24f-publication/candidate-all.txt`
- `/tmp/gts-p24f-publication/benchstat.txt`
- `/tmp/gts-p24f-publication/paired-stats.txt`

All 40 processes emitted three valid benchmark rows. They reported no failure,
out-of-memory event, timeout, panic, or contamination marker.

### P24f performance result

The benchstat result compares the candidate with the baseline. The geometric
mean (geomean) combines the three primary benchmark lanes.

| Benchmark | Baseline | Candidate | Result | p-value |
|---|---:|---:|---:|---:|
| Full parse | 7.181ms | 7.189ms | +0.11% | 0.862 |
| Single-byte edit | 770.4ns | 770.5ns | +0.01% | 0.616 |
| No edit | 2.549ns | 2.549ns | +0.00% | 0.909 |
| Geomean | 2.416µs | 2.417µs | +0.04% | — |

For each seed, compute the geomean across the trio. For each order, take the
geometric mean of the ten baseline seed geomeans and the ten candidate seed
geomeans. Report the candidate-to-baseline ratio minus one. The order formula
reports −0.167% when baseline ran first and −0.317% when candidate ran first.

The paired analysis uses equal seeds. For each pair, compute the
candidate-minus-baseline relative change. Use all 20 pairs and a Student-t
95% interval.

| Benchmark | Paired mean | 95% paired interval |
|---|---:|---:|
| Full parse | −0.359% | [−1.405%, +0.688%] |
| Single-byte edit | −0.262% | [−0.804%, +0.280%] |
| No edit | −0.070% | [−0.518%, +0.378%] |
| Geomean | −0.236% | [−0.772%, +0.300%] |

All incremental lanes used zero bytes and zero allocations. Full parsing used
54.97 KiB and four allocations per operation in both aggregate variants.

An auxiliary exact-setting run measured maximum resident set size at 591,840
KiB for the baseline and 608,000 KiB for the candidate. The publication runner
did not record resident set size for each process.

The candidate has no significant lane improvement. The paired geomean interval
crosses zero, and the auxiliary candidate run used more resident memory. The
accepted result is **REJECT / NO-GO**. No code shipped.

### P24f reopening condition

Reopen P24f only if a new profile confirms this helper as a material cost. Use
a real incremental or generalized LR workload. Require focused correctness and
parity first. Then repeat the 20-seed, 40-process protocol and require an
interval that excludes zero without a resident-memory regression.

## 2026-08-22 P24e raw-shape hashing receipt

Status: **REJECT / NO-GO**. Ship no candidate code.

This receipt is based on main commit
`56b97e092d0fb034bddb9e65cc617ebb933cc718`.

P24e tested one bounded hot-path change in raw-shape result selection. The
candidate read the packed child entry directly inside
`rawShapeComputeContentHash`. It did not restore parser state that hashing
does not inspect. This change differs from the rejected P24a through P24d
scheduler and memo candidates.

The experiment used base commit
`098620ad5e39d7b69b258239d1059a1e33bea892` and changed exactly one file:

- `raw_shape.go`

The candidate diff SHA-256 is
`e92471c690c7d6fed16d593044a449b0557b8b78380718173731a70a62842996`.

### P24e profiler attribution

The P19 attribution packet is at `/tmp/gts-p19-evidence`. It used Go 1.25.12,
`GOMAXPROCS=1`, and 20 samples per scenario. The warm early-newline profile
used 25 samples. It named `rawShapeComputeContentHash` at 47.25% cumulative
CPU. It named
`rawShapeChild.entry` at 24.71% cumulative CPU.

The same packet attributes incremental parse time as follows:

| Component | Observed share |
|---|---:|
| `Tree.Edit` | 0.16%–0.95% |
| Reuse cursor | 0.03%–0.98% |
| Reuse selection | 0.00%–2.79% |
| Reparse and rebuild | 96.98%–99.96% |

The candidate therefore targets a named reparse and result-selection cost.
It does not change `Tree.Edit`, reuse admission, parser decisions, or tree
shape rules. The proof boundary is the existing packed-entry representation:
the hash reads node payload fields and never reads the packed parser state.

### P24e correctness evidence

The focused Docker unit gate passed at
`/tmp/gotreesitter-p24e-candidate/harness_out/docker/20260822T214450Z`.
It covered raw-shape differential tests, cache eviction, hidden-shape
comparison, and the C result-selection control. The run exited zero without
an out-of-memory event or timeout.

The Go real-corpus smoke gate passed 25 of 25 cases for no-error,
S-expression, and deep parity. Its artifact is
`/tmp/gotreesitter-p24e-candidate/harness_out/docker/20260822T213032Z-diag-go_lang`.

The Go grammargen-versus-C gate passed five of five cases with two tree
matches and three known baseline divergences. Its artifact is
`/tmp/gotreesitter-p24e-candidate/harness_out/grammargen_cparity/20260822_143121-p24e-raw-shape-go`.

### P24e accepted publication

The accepted publication used 20 shuffle seeds and 40 isolated processes.
Odd seeds ran baseline first. Even seeds ran candidate first. Each process
used `GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`, and `-benchmem`.

The primary trio was:

- `BenchmarkGoParseFullDFA`
- `BenchmarkGoParseIncrementalSingleByteEditDFA`
- `BenchmarkGoParseIncrementalNoEditDFA`

This trio is a historical generated straight-LR control. It does not prove a
real generalized LR (GLR) improvement.

The accepted aggregate files are:

- Baseline: `/tmp/gts-p24e-publication/alternating-baseline-20.txt`
- Candidate: `/tmp/gts-p24e-publication/alternating-candidate-20.txt`

All 40 accepted processes emitted three valid benchmark rows. They reported
no failure, out-of-memory event, timeout, panic, or contamination marker.

The first seed-1 baseline construction was interrupted and is excluded from
the aggregate. Its quarantined file is
`/tmp/gts-p24e-publication/alternating-baseline/seed1.txt`. Clean seed-1
retries are recorded in the `seed1-retry.txt` files for both variants.

### P24e performance result

The combined benchstat result reports candidate versus baseline. The geometric
mean (geomean) combines the three primary benchmark lanes.

| Benchmark | Baseline | Candidate | Result | p-value |
|---|---:|---:|---:|---:|
| Full parse | 7.101ms | 7.128ms | +0.38% | 0.529 |
| Single-byte edit | 771.5ns | 772.6ns | +0.14% | 0.516 |
| No edit | 2.541ns | 2.539ns | −0.08% | 0.605 |
| Geomean | 2.405µs | 2.409µs | +0.16% | — |

For each seed, compute the geomean across the trio. Each order then compares
ten seed geomeans. For each order, take the geometric mean of the ten baseline
seed geomeans and the ten candidate seed geomeans. Report the
candidate-to-baseline ratio minus one. The order split reports changes of +0.278% when baseline ran
first and −0.228% when candidate ran first. The direction reverses.

The paired analysis uses equal seeds and the clean seed-1 retries. For each
pair, compute the candidate-minus-baseline relative change. Use all 20 pairs
and a Student-t 95% interval. The result is:

| Benchmark | Paired mean | 95% paired interval |
|---|---:|---:|
| Full parse | +0.056% | [−0.634%, +0.746%] |
| Single-byte edit | +0.300% | [−0.707%, +1.307%] |
| No edit | −0.240% | [−0.937%, +0.457%] |
| Geomean | +0.034% | [−0.639%, +0.708%] |

Full parsing used 54.10 KiB and four allocations per operation in both
variants. Both incremental lanes used zero bytes and zero allocations.

An auxiliary exact-setting run measured maximum resident set size at 603,200
KB for baseline and 609,120 KB for candidate. The publication runner did not
record per-process resident set size.

The candidate has no significant lane improvement. The accepted result is
**REJECT / NO-GO**. No code shipped.

### P24e reopening condition

Reopen P24e only with a dedicated real generalized LR (GLR) benchmark that
exercises raw-shape result selection. Use the same 20-seed, 40-process protocol
and record per-process resident set size. Require correctness parity before
performance.

## 2026-08-22 P24d authenticated single-head shift receipt

Status: **REJECT / NO-GO**. Ship no candidate code.

P24d tested one authenticated proof that a generic scheduler cell is an
ordinary shift on an exact single-head frontier. The proof lets the apply
path skip one repeated action-shape check. The experiment used base commit
`ed0568d9a822b1c83e7bb7e69b0e0f4b8ad529cf` and changed exactly:

- `parsercore_phase0_driver.go`
- `parsercore_phase0_generic_conflict_internal_test.go`

The combined candidate diff SHA-256 is
`5181912ec91f0bc1ecd504d06db488d3dd9145fdc5d3d5486300756661a4fc59`.
The per-file patch SHA-256 values are:

- Driver: `038ad2cf59286f93ecfdd36928cf0157022d6eb6fd414e01d14b95c4622f2fa6`
- Tests: `cfd9646fd8cf257718b0fb83eb8e735527335827a6bcf8a6429c7adef0aa9d1e`

The candidate source hashes are:

- Driver: `f3e279ea9fc3fece75b5e1b2b8ee760c4d8a26e2a5adb55af1d0c3e805f21a33`
- Tests: `a97472e2fbad888198fea03a01b0d59ce576b6572a221b9873d0b89982de85f7`

### Proof and validation

The proof requires one header, one elected state, header index zero, no
relexed symbol, matching checkpoint and parser state, and an unaccepted,
unshifted, unpaused, non-S3 header. It also requires an allowed selection,
an ordinary shift row, a valid action ordinal, and a non-extra shift action.

The focused tests cover the positive proof, a second header, a shifted header,
a checkpoint mismatch, a parser-state mismatch, a relexed symbol, an extra
shift, and a forged reduce action. Existing descriptor and unsupported-arm
checks remain active. Core owner, epoch, boundary, transaction, and rollback
validation remains active after the proof skips the repeated shape check.

The final focused Docker gate passed at
`/tmp/gts-p24d-docker-candidate-final/20260822T204456Z-p24d-unit-driver-final`.
The baseline control passed at
`/tmp/gts-p24d-docker-baseline/20260822T203123Z-p24d-unit-driver`.
Both runs used the local Go 1.25 image, one CPU, `GOMAXPROCS=1`, and `-p=1`.
Both runs exited zero without an out-of-memory event or timeout.

### Real-corpus and C parity

The Go real-corpus smoke gate passed five no-error, five S-expression, and
five deep checks for both variants. The reports are:

- Candidate: `/tmp/gts-p24d-real-candidate-final/diag_go_lang.log`
- Baseline: `/tmp/gts-p24d-real-baseline/diag_go_lang.log`

The grammargen-versus-C gate passed with five eligible cases and five no-error
cases for both variants. Both sides reported two tree-parity cases and three
known divergences. The candidate artifact is
`/tmp/gotreesitter-p24d-candidate/harness_out/grammargen_cparity/20260822_135328-p24d-candidate-final`.
The baseline artifact is
`/tmp/gotreesitter-p24d-baseline/harness_out/grammargen_cparity/20260822_133343-p24d-baseline`.

The known divergences are the blob root child-count difference and the two
statement-list end-byte differences. The candidate reproduces the baseline.

### Accepted publication

The final publication root is
`/tmp/gts-p24d-publication-final`.
The runner metadata is
`/tmp/gts-p24d-publication-final/runner-metadata.txt`.
It records 20 seeds, 40 isolated processes, and one process per seed.
Odd seeds run baseline first. Even seeds run candidate first. Each process
uses `GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`, and `-benchmem`.
The primary trio is:

- `BenchmarkGoParseFullDFA`
- `BenchmarkGoParseIncrementalSingleByteEditDFA`
- `BenchmarkGoParseIncrementalNoEditDFA`

All 40 status rows have exit code zero. The raw output and log files contain
no failure, out-of-memory, timeout, panic, or contamination marker.
The runner records Go 1.25.1 on host `Chi` and candidate primary-trio maximum
resident set size of 596,800 KB. The comparable baseline run used 605,120 KB.

### Performance result

The combined benchstat result reports candidate versus baseline:

| Benchmark | Baseline | Candidate | Result | p-value |
|---|---:|---:|---:|---:|
| Full parse | 6.635ms | 6.682ms | +0.71% | 0.096 |
| Single-byte edit | 732.2ns | 736.5ns | +0.59% | 0.256 |
| No edit | 2.344ns | 2.365ns | +0.87% | 0.031 |
| Geomean | 2.250µs | 2.266µs | +0.72% | — |

The order split reports these geomeans:

| Order | Baseline | Candidate | Result |
|---|---:|---:|---:|
| Baseline first | 2.252µs | 2.260µs | +0.37% |
| Candidate first | 2.247µs | 2.268µs | +0.93% |

The paired relative timing analysis reports candidate minus baseline:

| Benchmark | Paired mean | 95% paired interval |
|---|---:|---:|
| Full parse | +4.088% | [-0.049%, +8.225%] |
| Single-byte edit | +3.426% | [-1.301%, +8.154%] |
| No edit | +3.243% | [+0.063%, +6.423%] |
| Geomean | +3.496% | [+0.179%, +6.813%] |

Full parsing used 50.50 KiB/op for baseline and 50.88 KiB/op for candidate.
The byte comparison was +0.74% with p=0.024. Incremental lanes used 0 B/op.
Median full-parse allocations were 4 on both sides with p=0.231. The
candidate used 5 allocations on seeds 11, 12, and 13; baseline used 4 on
every seed. Incremental lanes used 0 allocations on every seed.

The candidate is slower in the combined geomean and both order splits. The
no-edit timing and full-parse byte regressions are statistically significant.
The result is **REJECT / NO-GO**. No code shipped.

## 2026-08-22 P24c exact-single-pop helper-inlining receipt

Status: **REJECT / NO-GO**. Ship no candidate code.

The original direct-append path already shipped in ancestor
`b9106e78635244b59dc9c9b75aa4863b89c99630` (`b9106e78`). P24c tested only
helper inlining inside `condenseWithOutcomeAtomic`. The audited candidate used
base commit `c46c9597005faf57ea810e3268e2cf55239da7e4` and changed exactly:

- `internal/parsercorephase0/core.go`
- `internal/parsercorephase0/condense_direct_append_test.go`

The audit recorded these three patch SHA-256 values:

- Core patch: `2dbcf54b185214e609d0cade7bf53ea6408150e6b0857ac104f24c3fee759438`
- Focused test patch: `418f60f9d20948d161adfe510fc068cc622ca4660f95aeef7476cde6ff05b0af`
- Combined two-file patch: `48a5ab605ff49373017aa5a5a31b423692619a3cecf71d9be9fd9f46a37190c3`

The focused tests directly cover direct publication shape, ordering, rollback,
and arena-cap behavior.

### Correctness evidence

The focused Docker unit gate passed at
`/tmp/gts-p24c-correctness/20260822T185424Z-p24c-direct-candidate-unit`.
The final parser-core package gate passed at
`/tmp/gts-p24c-correctness/20260822T185528Z-p24c-direct-candidate-package-r2`.

Exclude the stale pre-final package attempt at
`/tmp/gts-p24c-correctness/20260822T185452Z-p24c-direct-candidate-package`.
Its initial inline composite literal failed
`TestCondenseWithOutcomeAtomicProvenanceRatchet`. The final candidate fixed
that provenance-ratchet issue before the accepted package gate.

The generic driver failures reproduced on baseline. The candidate artifact is
`/tmp/gts-p24c-correctness/20260822T185553Z-p24c-direct-candidate-driver`.
The baseline artifact is
`/tmp/gts-p24c-correctness/20260822T185626Z-p24c-direct-baseline-driver`.
The shared failures are baseline-reproduced fixture and counter failures.
Do not attribute them to helper inlining.

The JSON real-corpus parity gate passed 5/5 on baseline and 5/5 on candidate.
Each side reported five no-error, five S-expression, and five deep checks.
The candidate report is
`/tmp/gts-p24c-reports/candidate-json/diag_json.log`.
The baseline report is
`/tmp/gts-p24c-reports/baseline-json/diag_json.log`.
The CSS grammargen-versus-C parity gate passed 5/5 on baseline and 5/5 on
candidate, with zero tree divergences on each side. The candidate artifact is
`/tmp/gts-p24c-single-pop-candidate/harness_out/grammargen_cparity/20260822_115824-p24c-direct-candidate-cgo`.
The baseline artifact is
`/tmp/gts-p24c-single-pop-baseline/harness_out/grammargen_cparity/20260822_115856-p24c-direct-baseline-cgo`.

### Accepted publication

The accepted publication root is
`/tmp/gts-p24c-publication-p24c-20260822-single-pop`.
It used 20 shuffle seeds, 40 isolated processes, and 120 benchmark rows.
Odd seeds ran baseline first. Even seeds ran candidate first. Each process
used `GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`, and `-benchmem`.
The primary trio was:

- `BenchmarkGoParseFullDFA`
- `BenchmarkGoParseIncrementalSingleByteEditDFA`
- `BenchmarkGoParseIncrementalNoEditDFA`

All 40 processes passed. The publication recorded no failures, out-of-memory
events, timeouts, or contamination. Raw headers prove the benchmark expression,
build tag, shuffle seed, and 750ms benchtime. The publication directory does
not itself encode GOMAXPROCS, count, benchmem, or isolated-process settings.
Those settings rely on external runner records.

### Performance result

The combined benchstat result reports candidate minus baseline:

| Benchmark | Baseline | Candidate | Result | p-value |
|---|---:|---:|---:|---:|
| Full parse | 6.512ms | 6.624ms | +1.72% | 0.925 |
| Single-byte edit | 725.2ns | 727.9ns | +0.37% | 0.805 |
| No edit | 2.347ns | 2.420ns | +3.11% | 0.129 |
| Geomean | 2.230µs | 2.268µs | +1.72% | — |

The order split reports these geomeans:

| Order | Baseline | Candidate | Result |
|---|---:|---:|---:|
| Baseline first | 2.246µs | 2.304µs | +2.60% |
| Candidate first | 2.229µs | 2.257µs | +1.26% |

The paired timing analysis reports candidate minus baseline:

| Benchmark | Paired mean | 95% paired interval |
|---|---:|---:|
| Full parse | +0.4707% | [-1.6206%, +2.5620%] |
| Single-byte edit | +0.4155% | [-1.6074%, +2.4385%] |
| No edit | +0.3220% | [-2.8235%, +3.4676%] |
| Geomean | +0.3163% | [-1.1488%, +1.7814%] |

Full parsing used 50.13 KiB/op for baseline and 51.07 KiB/op for candidate.
The byte comparison had p=0.370. Incremental lanes used 0 B/op in both
variants. Full parsing used 4 allocations per operation in both variants.
Incremental lanes used 0 allocations per operation in both variants. Every
allocation comparison had p=1.000.

The geomean rose in the combined publication. Each paired interval includes
zero. The order split did not show a credible directional win. The accepted
result is **REJECT / NO-GO**. No code shipped.

## P24b owner-plus-lineage transaction receipt

P24b tested one scheduler transaction for owner and lineage state.
The hypothesis was that one transaction would reduce repeated scheduler work.
The candidate combined owner and lineage writes for converged, reference-bearing heads.
It preserved owner-only calls, rollback, journaling, and token validation.
It kept production defaults and profile grants unchanged.

The base commit was
a01f8037319c9f8f0ea12ae3c96523112656ce22.
The candidate changed these three files:

- internal/parsercorephase0/scheduler_owned.go
- parsercore_phase0_driver.go
- internal/parsercorephase0/scheduler_owned_owner_lineage_test.go

The full candidate diff had SHA-256
c064480db960885558cce4eff8b22679a3dffed996d908f938fa5e480f92dee.

Focused correctness gates passed. The corrected unit gate used
`/tmp/gts-p24b-owner-lineage-20260822/harness_out/docker/20260822T184141Z-p24b-owner-lineage-unit-corrected-20260822`.
Its Docker command selected these eight tests:

- `TestRecordHeadOwnerAndLineageOwnedCommitsAllState`
- `TestRecordHeadOwnerAndLineageOwnedRollsBackAllState`
- `TestRecordHeadOwnerAndLineageOwnedRejectsInvalidOwnerLineage`
- `TestRecordHeadOwnerAndLineageOwnedRejectsOwnerConflict`
- `TestRecordHeadOwnerAndLineageOwnedRejectsStaleToken`
- `TestRecordHeadOwnerAndLineageOwnedRollsBackReferenceFailure`
- `TestRecordHeadOwnerAndLineageOwnedHonorsSetDirtyFalse`
- `TestHeadOwnerRollsBackWithSchedulerTransaction`

The earlier unit receipt
`/tmp/gts-p24b-owner-lineage-20260822/harness_out/docker/20260822T180959Z-p24b-owner-lineage-unit-final2`
is superseded and excluded from this unit claim. Its anchored expression
matched no combined owner-lineage test, so it did not cover the new combined
operation.
The driver gate used
`/tmp/gts-p24b-owner-lineage-20260822/harness_out/docker/20260822T180751Z-p24b-owner-lineage-driver-final`.
The real-corpus smoke gate used
`/tmp/gts-p24b-owner-lineage-20260822/harness_out/docker/20260822T180806Z-diag-go_lang`.
The candidate C parity gate used
`/tmp/gts-p24b-owner-lineage-20260822/harness_out/grammargen_cparity/20260822_110830-p24b-owner-lineage-cgo-final`.
The baseline C parity gate used
`/tmp/gts-p24b-baseline-20260822/harness_out/grammargen_cparity/20260822_110533-p24b-owner-lineage-baseline-cgo`.
The candidate and baseline C receipts showed the same three pre-existing
grammar divergences. They showed two EndByte differences and one root
child-count difference.

### Publication protocol

The accepted publication is
`/tmp/gts-p24b-publication-20260822T181351Z`.
It used the three required benchmarks:
`BenchmarkGoParseFullDFA`,
`BenchmarkGoParseIncrementalSingleByteEditDFA`, and
`BenchmarkGoParseIncrementalNoEditDFA`.
It used 20 seeds and 40 isolated processes.
Each process used `GOMAXPROCS=1`, `-count=1`,
`-benchtime=750ms`, `-benchmem`, and `-test.shuffle=<seed>`.
Odd seeds ran the baseline first. Even seeds ran the candidate first.
The run produced 120 accepted benchmark rows.
The run reported no out-of-memory event, timeout, or contamination.

The first attempt was quarantined at
`/tmp/gts-p24b-publication-20260822T181256Z-QUARANTINED`.
Its validator expected a hyphenated benchmark suffix.
Go emitted whitespace after the benchmark name.
The attempt contained only three seed-one baseline rows.
The analysis excluded that attempt from every aggregate and decision.

### Benchstat result

The analysis used
`/tmp/gts-p24b-analysis-20260822T182318Z/benchstat.txt`.
The table reports candidate minus baseline.

| Benchmark | Baseline median | Candidate median | Change | p-value |
|---|---:|---:|---:|---:|
| Full parse | 6.616ms | 6.585ms | -0.47% | 0.512 |
| Single-byte edit | 721.1ns | 722.0ns | +0.12% | 0.644 |
| No edit | 2.350ns | 2.347ns | -0.13% | 0.784 |
| Geomean | 2.238µs | 2.234µs | -0.17% | — |

Memory stayed neutral.
Full parse used 50.50 KiB and 4 allocations per operation in both variants.
Incremental benchmarks used zero bytes and zero allocations in both variants.
The full-parse allocation comparison had p=1.000.
The full-parse byte comparison had p=0.757.

### Paired and order results

The paired analysis is in
`/tmp/gts-p24b-analysis-20260822T182318Z/paired-summary.txt`.
Each interval uses 20 matched seeds and reports candidate minus baseline.

| Benchmark | Paired mean | 95% paired interval |
|---|---:|---:|
| Full parse | +0.389% | [-1.376%, +2.154%] |
| Single-byte edit | +1.779% | [-0.198%, +3.756%] |
| No edit | +0.452% | [-0.555%, +1.459%] |

The order split is in
`/tmp/gts-p24b-analysis-20260822T182318Z/order-split.txt`.

| Order | Full parse | Single-byte edit | No edit |
|---|---:|---:|---:|
| Baseline first | +0.643% | +1.417% | +0.282% |
| Candidate first | +0.135% | +2.140% | +0.622% |

### Decision

**REJECT / NO-GO.**

The keep rule requires a target improvement without a correctness regression.
No benchmark lane improved with statistical significance.
The paired mean rose in all three lanes.
The memory result stayed neutral.
The candidate code does not ship.

## Why this document exists

Three prior estimates put the compact scheduler's share of full-parse CPU at
77-79%, at approximately 50%, and at 78-82%. Each estimate used a different,
undocumented attribution boundary: one drew the scheduler boundary wide
(covering shift, reduce, and election together), one drew it narrow, and one
used a different profiling method. The three numbers do not contradict each
other once you know their boundaries, but no one had written the boundaries
down, so the campaign could not tell whether a proposed perf lever targeted
77% of parse CPU or 50% of it.

Campaign v7 tranche C0 (`spec.campaign.v7`, `hypha://m31labs/gotreesitter`)
blocks new perf-lever bets until one receipt exists with a named boundary for
every component, a documented noise floor, and a cost-per-event model. This
document is that receipt's home. The attached tool
(`cgo_harness/attribution`) regenerates the receipt from one command.

## Scope

The receipt covers the compact parser core's fresh, non-incremental full
parse: the default route for eligible full parses (`admission_switch.go`),
and the diagnostic runner path (`parserCoreFreshFullRunner`) that exercises
the same scheduler and materialization code directly. Campaign v7 tranche B9
retired the compact admission switch's 64 KiB source-length eligibility
decline; the shipped route now attempts every eligible full parse regardless
of size, with a scheduler stop-control poll (tranche B8) bounding a large or
pathological input instead.

It does not cover: incremental reparse, error recovery, retry passes, the
production (non-compact) GLR engine, or any language other than the four
pinned Go canonical fixtures.

## The compact lane, by file

| Area | File(s) | Key entry points |
|---|---|---|
| Scheduler driver (root package) | `parsercore_phase0_driver.go` | `run`, `dispatchPass`, `dispatchPassActive`, `elect`, `canonicalize` |
| Core structures (`internal/parsercorephase0`) | `core.go`, `scheduler_owned.go`, `boundary_index.go`, `checkpoint_interner.go` | `ApplySchedulerAtomic`, `condenseWithOutcomeAtomic`, `popPaths`, `Derivations`, `MaterializationOrder` |
| Lexing | `parser_dfa_token_source.go`, `lexer.go` | `(*dfaTokenSource).Next` |
| Runner glue | `parsercore_phase0_fresh_full_runner.go` | `executeSchedulerOpen`, `parse`, `materialize` |
| Shipped-route fidelity tail | `admission_switch.go`, `admission_switch_candidate.go` | `normalizeReturnedTreeForParse`, `resolveCRecoverySwallowedError`, `maybeCompactReturnedFullTree` |

The four canonical fixtures are SHA-256-pinned in
`internal/benchfixtures/testdata` and authenticated the same way
`cgo_harness/pure_c/run_canonical_go_full_parse.sh` and
`diagnosticParserCoreCanonicalAdmissions` authenticate them:

| Fixture | Bytes |
|---|---:|
| `rewrite.go` | 5,116 |
| `query_compile.go` | 20,168 |
| `language.go` | 41,387 |
| `grammargen/lr.go` | 235,626 |

Before tranche B9, `grammargen/lr.go` sat above the compact admission
switch's 64 KiB source-length eligibility floor and never took the shipped
`Parser.Parse` route (diagnostic-lane only; see the first receipt below,
which predates B9). B9 retired that floor, so all four fixtures now run
through both lanes; see the B9 addendum.

## Two lanes, one methodology

Every fixture profiles under one or both of two lanes. Both lanes time an
identical, already-committed lifecycle; the tool never adds new parser calls.

- **Diagnostic lane.** `runner.parse`, shallow completeness validation, and
  `Tree.Release` — the exact timed region of
  `BenchmarkParserCoreFreshFullCanonical` (build tag
  `gts_parsercorephase0`). Runs for all four fixtures.
- **Shipped route.** `Parser.Parse` and `Tree.Release` — the exact timed
  region of `BenchmarkDiagnosticParserCoreWarmProductionParseQueryCompile`,
  generalized to all four fixtures (tranche B9 retired the 64 KiB eligibility
  floor that used to keep `grammargen/lr.go` diagnostic-lane only). Every
  sample is verified, by the admission-candidate routed/fallback counters, to
  have actually taken the compact route rather than falling back to
  production.

The tool labels every row with its lane so a reader never mixes shipped-route
public-API overhead into the diagnostic lane's scheduler-only numbers, or
vice versa.

## The attribution tree

Nine components, mutually exclusive and collectively exhaustive. Every
CPU-profile sample lands in exactly one.

1. **scheduler-dispatch** — the scheduler loop, per-token boundary
   classification, and the mechanical application of shift and accept
   actions. Owning functions: `run`, `dispatchPass`, `dispatchPassActive`,
   `applyGenericShifts(Owned)`, `applyGenericExtraShifts(Owned)`,
   `applyGenericAccept`, `finish`, `Core.Shift*`, `Core.ClassifyBoundary`,
   `Core.Actions`, scheduler construction and seeding, and the accepted-tail
   cleanliness check (`requireParserCoreFreshFullAcceptance`,
   `parserCoreFreshFullAcceptedTailIsClean` — see the naming-collision note
   below).
2. **elections** — choosing among competing alternatives: which GSS heads
   advance each round (frontier election), and which action wins at an
   ambiguous cell (GLR conflict resolution / dynamic precedence). Owning
   functions: `elect`, `executeDiagnosticParserCoreGenericConflictDetailed`,
   `applyGenericConflict(Owned)`, `diagnosticParserCoreConflictPolicyOrdinal`,
   `diagnosticParserCoreRepetitionFoldOrdinal`.
3. **reductions-and-pops** — applying a reduce action: enumerating GSS pop
   paths back to a production's children, building the parent subtree, and
   merging (condensing) the result into the derivation graph. Owning
   functions: `applyGenericReduction(Owned)`, `Core.Reduce`,
   `Core.ReduceOutputs*`, `Core.popPaths`, `Core.popSingleLinkPath`,
   `Core.factorExactPredecessor`, `Core.mergePredecessorsBounded`,
   `Core.insertLinkBounded`, `Core.appendNode`, `Core.appendSubtree`,
   `Core.walkCleanPrefixRanks`.
4. **canonicalization** — post-step header-lineage deduplication: merging
   headers that reconverge to the same `(state, byte-offset)` boundary after
   any dispatch action. Owning functions: `canonicalize`,
   `canonicalizeDiagnosticParserCoreHeaders`,
   `diagnosticParserCoreCanonicalScratch.canonicalize(Linear|Mapped)`,
   `mergeDiagnosticParserCoreCleanPathLineage`,
   `persistHeaderLineageOwned`, `RecordReductionLineageOwned`,
   `RecordHeadLineageOwned`.
5. **lexing** — producing the next token from source bytes. Owning function:
   `(*dfaTokenSource).Next`, and every function defined in
   `parser_dfa_token_source.go` or `lexer.go` (a whole-file rule, since both
   files exist for exactly this one job).
6. **materialization** — turning the accepted derivation into the returned
   public `*Tree`: selecting the canonical derivation when more than one
   exists, building nodes, attaching parent/sibling links, replaying parser
   states, and finalizing the root span. Owning functions:
   `completeAcceptance`, `selectCompactAcceptanceDerivation`,
   `Core.Derivations`, `materializeDiagnosticParserCoreAcceptedTree`,
   `materializeDiagnosticParserCoreAcceptedSelection`,
   `finalizeDiagnosticParserCoreAcceptedRootSpan`, `Core.MaterializationOrder`,
   `Core.VisitMaterializationPostorder`, `(*Parser).replayCompactDerivation`.
7. **compat-tail** — the shipped-route-only public-API fidelity tail that
   runs after the compact tree exists, so every `Parser.Parse` return matches
   production's API surface exactly. Not reachable from the diagnostic
   runner path. Owning functions: `normalizeReturnedTreeForParse`,
   `resolveCRecoverySwallowedError`, `maybeCompactReturnedFullTree`,
   `tryCompactFullParseRoute`, `attemptAdmissionCandidateFullParse`,
   `admissionCandidateFullParseEligible` (the whole of `admission_switch.go`
   and `admission_switch_candidate.go` — a whole-file rule).
8. **recovery** — the compact core's native locked-C recovery mechanism
   (error cost, missing-token insertion, retry, and recovery election over
   compact subtrees; campaign v7 tranche B3). Added in tranche B3 stage S1,
   before any recovery engine code exists, per gate G5 ("classifier
   first... so recovery cost can never hide in `other`"). It owns no
   functions yet — compact has no recovery implementation (B3 stages S2-S5
   add error-cost, absorb/condense-resume, election, and missing-token code
   class by class) — so it reads 0.0% on every clean canonical fixture
   today, the same way `compat-tail`'s conditional work reads 0.0% below.
   The whole-file rule for `internal/parsercorephase0/recovery_cost.go`
   (stage S2's planned file) is forward-declared now so landing that file
   does not also require touching this table.
9. **other** — every sample the walk cannot attribute to a named component:
   Go runtime, garbage collection, and goroutine-scheduling frames with no
   gotreesitter-domain ancestor, plus any genuinely unclassified function.
   The tool separately reports the largest unclassified, non-runtime
   functions it saw; a nonzero entry there is a signal this table has a gap,
   not a license to guess.

### A naming collision to avoid

`parserCoreFreshFullAcceptedTailIsClean` checks that no real source byte
follows the accepted head (only parser padding may remain) before the
scheduler declares acceptance. This is **not** the compat-tail component. It
is a scheduler-dispatch acceptance check that runs on both lanes. The English
word "tail" names two unrelated things here: an accepted-frontier byte-range
check (scheduler-dispatch), and a post-parse public-API fidelity pass
(compat-tail, shipped route only). The classifier and this document both use
the full function name, never the bare word "tail", to keep them apart.

## The disambiguation rule

Every named function belongs to at most one component's owned set, chosen by
its primary job, not by which caller happens to invoke it. A handful of
low-level primitives exist purely to serve more than one component
(transaction bookkeeping, checkpoint identity, and pure counters). Those
primitives are explicitly marked shared and are **not** classified directly.

A CPU-profile sample is a full call stack, leaf (currently executing frame)
to root. Attribution walks the stack from the leaf toward the root and
assigns the sample's entire value to the first frame the table recognizes.
A shared primitive is transparent to this walk: the sample "sees through" it
to the nearer ancestor frame, which is always the specific call site that
decided to use the primitive. A sample with no recognized frame anywhere on
its stack is `other`.

This single rule resolves every case where the same function serves two
components:

- **`condense` / `condenseWithOutcomeAtomic`** run from both `Core.Shift*`
  (scheduler-dispatch, appending a shifted GSS node) and reduction-output
  application (reductions-and-pops, appending a reduced parent node). Marked
  shared; the walk finds whichever caller reached it.
- **`ApplySchedulerAtomic` / `RunSchedulerOwned`** wrap the shift, reduce,
  conflict, and extra-shift appliers uniformly (four call sites in
  `parsercore_phase0_driver.go`). Marked shared for the same reason.
- **Checkpoint interning and external-scanner-state capture**
  (`diagnosticParserCoreInternCheckpoint`,
  `captureExternalScannerStateInto`) run both nested inside a live call to
  `dfaTokenSource.Next` (lexing) and directly from `elect` for the
  election's checkpoint-continuity proof (elections). Marked shared; the
  walk finds `Next` when the capture is nested inside token production, and
  finds `elect` when it is not, without any special case.
- **`canonicalize`** is called from inside every applier (accept, shift,
  reduce, conflict, extra-shift) as a post-step. It is deliberately **not**
  marked shared: its job (header-lineage deduplication) is the same
  regardless of caller, so it is classified directly as canonicalization.
  The walk finds `canonicalize`'s own frame before it would reach the
  calling applier's frame, so canonicalization is correctly separated from
  whichever action triggered it.

## The tool

`cgo_harness/attribution` is a small, dependency-free Go program (it lives in
the `cgo_harness` module and imports nothing beyond the standard library,
including its own minimal pprof-profile reader, so this measurement
infrastructure does not widen either module's dependency footprint). It:

1. Extracts and SHA-256-verifies the four canonical fixtures.
2. Builds two test binaries: one tagged `gts_parsercorephase0` (capture and
   noise floor), one tagged `gts_parsercorephase0,gts_workcount` (event
   counts). Building happens once, before any timed measurement, so
   compilation never contends with the host for CPU during a timed sample.
3. Runs `TestParserCoreAttributionCapture` (added in
   `parsercore_phase0_attribution_capture_internal_test.go`, inert unless
   `GTS_ATTRIBUTION_OUT` is set) to collect one gzip'd pprof CPU profile per
   lane per fixture. Every one-time warm pass, digest check, and GC/arena
   drain happens outside `pprof.StartCPUProfile`/`StopCPUProfile`, so the
   profile is not diluted by non-representative setup cost.
4. Runs the existing `TestParserCoreWorkCountChild` (already committed,
   `work_count_parsercore_child_internal_test.go`) once per fixture for
   event counts.
5. Runs the noise floor (see below).
6. Decodes every profile with a minimal, read-only pprof protobuf reader,
   classifies every sample by the rule above, and emits `receipt.json` and
   `receipt.md`.

Reproduce the whole receipt with one command from the repository root:

```sh
(cd cgo_harness && go run ./attribution -repo .. -duration 800ms -noise-samples 10 -noise-benchtime 750ms)
```

Add `-out <dir>` to choose the output directory (default:
`harness_out/perf_attribution/<UTC timestamp>`, already git-ignored). Add
`-core <cpu>` to pin every subprocess with `taskset`; the published receipt
below did not pin a core (see host class, next section).

## Noise floor

**Protocol.** Using the pinned local proxy benchmark
`BenchmarkParserCoreFreshFullCanonical`, the tool builds one test binary and
invokes that identical binary twice per pair, labeled A and B, interleaved
A,B,A,B,... for n pairs (n >= 10). Each invocation runs with `GOMAXPROCS=1`
and times all four fixtures in one process. For each fixture and pair `i`,
`delta_i = |A_i - B_i|`. The noise floor is the 95th percentile (linear
interpolation over the sorted deltas) of `delta_i`, reported both in
nanoseconds and as a percentage of the fixture's median ns/op across the
combined A and B samples.

**Host class.** Shared WSL2 development host, background load uncontrolled.
This is the local-development floor, not the quiet-host or enclave floor
that `BENCH.md`'s sealed epoch uses. Treat every ns/op number in this
document as wall-clock on a busy, shared machine (other agents were active
on the same host during this run), not as a publication-grade performance
claim. The relative component-share percentages are far less sensitive to
this noise than the absolute ns/op and ns-per-event numbers are, because Go's
CPU profiler samples on-CPU (SIGPROF/`ITIMER_PROF`) time, not wall time: host
contention stretches wall-clock ns/op without changing which component a
given on-CPU sample belongs to.

## Cost-per-event join

For each fixture, the tool divides the diagnostic lane's measured wall
ns/op by the `gts_workcount` build's authenticated event counts for that
same fixture and lifecycle (`TestParserCoreWorkCountChild`, schema
`gts-work-count-parsercore-child/v3`): shifts, reductions, emitted pop
paths, emitted pop payloads, canonicalizations, elections, and action
lookups.

This is an **average attribution, not a causal model**. Dividing total wall
time by an event count says nothing about which event class actually causes
the most CPU per occurrence; it says how the average work per fixture spread
across event classes on this run, on this host. Two events of the same class
can cost very different amounts (a pop path over a clean linear GSS chain is
far cheaper than one over a branching, ambiguous chain), and the classes are
not independent (an election's checkpoint-continuity proof is not free, but
it has no "count" of its own in this join).

## Results — first receipt

Generated 2026-08-01T11:12:22Z. Host: shared WSL2 development host,
background load uncontrolled. Other agents were active on the same host
throughout this session (22 registered at session start). Git commit
`f91e9c8cc7d2c8830c973cfa28b658c70dd0d0ba`. Full machine-readable receipt:
`docs/perf-attribution-receipt.json`. Regenerate both (and the larger raw
pprof profiles, which are not committed) with the one command above.

Component shares are computed to sum to exactly 100% of attributed samples
by construction (`other` absorbs the remainder); the displayed 1-decimal
rows are checked to sum to 100% within +/-0.2 percentage points of rounding
tolerance. Every row below is within that tolerance.

### Attribution shares

| lane | fixture | wall ns/op | coverage % | scheduler-dispatch | elections | reductions-and-pops | canonicalization | lexing | materialization | compat-tail | other | sum % |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| diagnostic lane | rewrite | 2.674ms | 105.0 | 22.9 | 9.5 | 33.3 | 4.8 | 14.3 | 13.3 | 0.0 | 1.9 | 100.0 |
| diagnostic lane | query_compile | 12.340ms | 104.8 | 20.8 | 4.7 | 30.2 | 8.5 | 19.8 | 16.0 | 0.0 | 0.0 | 100.0 |
| diagnostic lane | language | 13.195ms | 105.7 | 22.6 | 7.5 | 33.0 | 7.5 | 13.2 | 16.0 | 0.0 | 0.0 | 99.8 |
| diagnostic lane | grammargen_lr | 126.019ms | 106.1 | 25.2 | 7.5 | 22.4 | 5.6 | 19.6 | 17.8 | 0.0 | 1.9 | 100.0 |
| shipped route | rewrite | 2.707ms | 104.8 | 21.9 | 7.6 | 28.6 | 9.5 | 19.0 | 11.4 | 0.0 | 1.9 | 99.9 |
| shipped route | query_compile | 13.026ms | 104.7 | 30.5 | 3.8 | 21.9 | 1.0 | 20.0 | 22.9 | 0.0 | 0.0 | 100.1 |
| shipped route | language | 11.753ms | 105.9 | 25.2 | 4.7 | 30.8 | 6.5 | 15.9 | 16.8 | 0.0 | 0.0 | 99.9 |

`coverage %` is profiled CPU time divided by measured wall time on this
single-core-bound (`GOMAXPROCS=1`) run. It runs 105-106% here, not 100%,
because the Go runtime's background workers (GC assist, sweep) can add a
small amount of concurrent on-CPU time beyond the profiled goroutine's own
wall-clock window. A coverage figure far below 100% would mean too few
samples landed to trust the percentages; every row here is comfortably
above that concern.

`grammargen_lr` has no shipped-route row: it is above the 64 KiB floor, so
`Parser.Parse` never routes it through the compact candidate. `compat-tail`
measures zero on every shipped-route row: for these four fixtures every
parse is clean (no error nodes, no C-recovery-swallowed error to resolve),
so the shipped-route fidelity tail's conditional work never fires. This is
a genuine finding, not a classifier gap: the compat-tail functions exist for
correctness parity on error/recovery paths this receipt does not exercise,
not for cost on the clean path.

`other` stays at 0.0-1.9% on every row. Where it is nonzero, the largest
non-runtime contributors are `(*nodeArena).reset` (arena-pool bookkeeping
inside `Tree.Release`), `hiddenTreeHasFieldIDs`, and
`maxRetainedNodeCapacityForClass` — each under 1% of one profile's samples.
None of them changes the qualitative picture; they are recorded as a known
small residual rather than folded into a component that would overstate its
weight.

### B3 stage S1 addendum: the `recovery` component

The table above predates the `recovery` component (added to
`cgo_harness/attribution/classify.go` in campaign v7 tranche B3 stage S1, per
gate G5) and so has no `recovery` column. A local reproduction of the same
command on stage S1's branch, after the classifier change and before any
recovery engine code, confirmed `recovery` reads exactly 0.0% on every lane
and fixture — diagnostic lane (`rewrite`, `query_compile`, `language`,
`grammargen_lr`) and shipped route (`rewrite`, `query_compile`, `language`)
alike — matching `compat-tail`'s 0.0% pattern above. This is expected: the
component owns no functions yet (B3 stages S2-S5 add error-cost,
absorb/condense-resume, election, and missing-token code class by class), so
no sample can land there. This local run is a verification check, not a new
sealed or C0-authoritative epoch; it does not replace the receipt above or
`docs/perf-attribution-receipt.json`. The next full regeneration (any future
tranche that re-seals this board) will show `recovery` alongside the other
eight components by construction, with no further classifier change needed.

### B3 stage S2 addendum: the inert error-cost model

Stage S2 added `internal/parsercorephase0/recovery_cost.go`: compact
equivalents of `cNodeErrorCostLang`, `cSymbolVisibleLang`, and
`cVersionStatus`, callable on demand but wired into nothing (no call site
exists outside the file's own tests; a compile-time AST ratchet,
`TestRecoveryCostNoOutsideCallSitesRatchet`, enforces this). A local
reproduction of the documented command on stage S2's branch, with the file
present, again confirmed `recovery` reads exactly 0.0% on every lane and
fixture — diagnostic lane and shipped route alike. Every other component
share stayed within this host's published noise floor of the stage-S1-era
receipt above; the local noise floor measured on this run (8.2%-11.9% of
median ns/op) is consistent with that same shared, uncontrolled-host
variance, not a new finding. As with the S1 addendum, this is a
verification check, not a new sealed epoch.

### 2026-08-02 addendum: the provenance-predicate flip receipt

The compact scheduler's no-action head drop now decides with the
alternative-set containment predicate by default. The scalar rank-walk
(`markCleanProductionRank`) still exists in the source. It now runs only
under a diagnostic flag (`GTS_B4B_SHADOW_CENSUS`), not on the default
path. Alternative-set recording used to run only under that same flag. It
now runs unconditionally, because the containment predicate needs a
populated set to decide.

This addendum re-runs the documented command. It adds one targeted
profile of the changed functions. The goal is one question: how much CPU
does the changed provenance machinery cost today, and where should the
next performance lever land.

#### Component shares (regenerated receipt)

Generated 2026-08-02T04:38:36Z. Host: shared WSL2 development host,
background load uncontrolled. Git commit
`193c44b340538b78770da68e4246abd253c81710`. This run's noise floor (see
below) sits far above the first receipt's 7.4%-10.8%. Read every share
below as a rough reading, not an exact one. This run's raw JSON output was
not committed, matching the existing `harness_out/` git-ignore rule;
rerun the documented command to reproduce it.

| lane | fixture | wall ns/op | scheduler-dispatch | elections | reductions-and-pops | canonicalization | lexing | materialization | compat-tail | recovery | other |
|---|---|---|---|---|---|---|---|---|---|---|---|
| diagnostic lane | rewrite | 2.389ms | 21.6 | 8.0 | 22.7 | 10.2 | 21.6 | 14.8 | 0.0 | 0.0 | 1.1 |
| diagnostic lane | query_compile | 11.201ms | 26.1 | 8.0 | 20.5 | 8.0 | 22.7 | 13.6 | 0.0 | 0.0 | 1.1 |
| diagnostic lane | language | 12.511ms | 14.8 | 6.8 | 25.0 | 15.9 | 15.9 | 21.6 | 0.0 | 0.0 | 0.0 |
| diagnostic lane | grammargen_lr | 125.275ms | 22.9 | 7.3 | 20.8 | 15.6 | 15.6 | 16.7 | 0.0 | 0.0 | 1.0 |
| shipped route | rewrite | 2.452ms | 21.6 | 6.8 | 31.8 | 11.4 | 10.2 | 15.9 | 1.1 | 0.0 | 1.1 |
| shipped route | query_compile | 11.824ms | 21.6 | 4.5 | 25.0 | 10.2 | 18.2 | 20.5 | 0.0 | 0.0 | 0.0 |
| shipped route | language | 12.202ms | 10.2 | 4.5 | 33.0 | 12.5 | 20.5 | 19.3 | 0.0 | 0.0 | 0.0 |

`recovery` reads 0.0% on every row. This matches every prior receipt: the
inert cost model from stage S2 still has no call site. The `compat-tail`
reading for shipped-route `rewrite` (1.1%) is one profiled sample out of
roughly ninety. Treat it as noise, not a new conditional-cost finding,
given this run's wide noise floor.

Scheduler-dispatch, elections, reductions-and-pops, and canonicalization
together still cover 60.2%-71.6% of wall time across every fixture above.
No lever in the ranked list below reaches that scale on its own; see item
5.

#### Noise floor (this run)

| fixture | pairs | median ns/op | p95 \|delta\| | p95 \|delta\| as % of median |
|---|---:|---:|---:|---:|
| `grammargen_lr` | 10 | 122.945ms | 68.439ms | 55.7% |
| `rewrite` | 10 | 2.448ms | 1.038ms | 42.4% |
| `query_compile` | 10 | 11.862ms | 4.626ms | 39.0% |
| `language` | 10 | 12.123ms | 3.725ms | 30.7% |

Other automated workers ran concurrent CPU-bound jobs on this host
throughout this run. Read every ns/op number above as directional, not
exact. Component shares still rest on firmer ground than raw ns/op does:
the CPU profiler samples on-CPU time, so host contention stretches
wall-clock duration without moving which component a sample belongs to
(see "Noise floor," above).

#### Targeted profile: the provenance-predicate family

The component table above answers where compact wall time goes, across
nine wide boundaries. It does not answer a narrower question: how much of
the reductions-and-pops and canonicalization shares still belongs to the
mechanism this flip changed. This section profiles that mechanism by
function name.

Method: `go test -tags gts_parsercorephase0 -bench
BenchmarkParserCoreFreshFullCanonical/query_compile -benchtime 6s
-cpuprofile`, `GOMAXPROCS=1`, pinned to one core with `taskset -c 0`,
`GTS_B4B_SHADOW_CENSUS` unset (the shipped default). 630 iterations, 9.55
seconds of profiled samples. `query_compile` matches the fixture the
original regression bisect used, so the reading below is
fixture-consistent with that bisect. The bisect's own numbers predate
this tool's interleaved-capture methodology, so treat the comparison as
fixture-consistent, not protocol-identical.

| function | flat % | cum % |
|---|---:|---:|
| `condenseWithOutcomeAtomic` | 5.1% | 9.2% |
| `persistHeaderLineageOwned` | 1.5% | 7.3% |
| `RecordHeadLineageOwned` | 0.2% | 4.0% |
| `RecordHeadOwnerOwned` | 0.2% | 1.9% |
| `recordNodeLineageSet` | 0.5% | 2.1% |
| `recordNodeLineage` | 0.5% | 0.7% |
| `RecordReductionLineageOwned` | 0.0% | 0.4% |
| `alternativeSetInsert` | 0.1% | 0.3% |
| `alternativeSetUnion` | 0.0% | 0.3% |
| `alternativeSetSortedContainsAll` | 0.0% | 0.0% |
| the v2 containment predicate (`dropGenericNoActionHeads`'s decider) | 0.0% | 0.1% |
| `enterLiveCondenseCandidates` | 0.3% | 0.3% |
| `clearLiveCondenseCandidates` | 0.1% | 0.1% |
| `condenseNodeIsLive` | 0.2% | 0.2% |
| `runLiveCondenseCandidates` (old closure-wrapper form) | 0.0% | 0.0% |
| `markCleanProductionRank` | 0.0% | 0.0% |
| `walkCleanPrefixRanks` | 0.0% | 0.0% |

`condenseWithOutcomeAtomic` is a shared GSS merge primitive that predates
the regression; it is not itself a provenance-family member, and it is
the single largest function this profile named. `condenseNodeIsLive` runs
nested inside it. `RecordHeadLineageOwned` and `RecordHeadOwnerOwned` run
nested inside `persistHeaderLineageOwned`, as two separate transactions,
not one inside the other; `recordNodeLineageSet`, `recordNodeLineage`,
`alternativeSetInsert`, and `alternativeSetUnion` run nested inside
`RecordHeadLineageOwned` in turn. None of these nested cum numbers add on
top of their parent row.

Pre/post comparison, rolled up to the mechanism level (query_compile;
pre-flip numbers from the regression bisect, at the commit that
introduced them):

| mechanism | pre-flip cumulative CPU | today's cumulative CPU |
|---|---|---|
| condense live-scoping and liveness check (`enterLiveCondenseCandidates`, `clearLiveCondenseCandidates`, `condenseNodeIsLive`) | 26%-30% (as the old closure-wrapper form, `runLiveCondenseCandidates`) | 0.6% |
| lineage-and-set recording (`persistHeaderLineageOwned` and everything nested below it, plus `RecordReductionLineageOwned`) | canonicalization's whole component share grew by +3.8 percentage points (3.0% to 6.8%) when this family was introduced, per the bisect's own before/after reading; the family did not yet have a separate cum number | 7.7% |
| scalar rank-walk (`markCleanProductionRank`) | part of the two rows above; not separated at bisect time | 0.0% (retired) |
| v2 containment predicate (decider) | did not exist | 0.1% |

This run's own component table (above) reads canonicalization at 8.0% for
`query_compile`, diagnostic lane. Lineage-and-set recording alone (7.7%)
now accounts for nearly all of that share. Canonicalization today is
almost entirely lineage-and-set recording, not the header-merge mechanics
the component predates.

Two readings follow.

The retirement holds. `markCleanProductionRank` and `walkCleanPrefixRanks`
carry zero samples on the default path. `runLiveCondenseCandidates`'s
closure-wrapper form also carries zero samples: an earlier, separate fix
replaced its call sites with direct calls before this predicate flip
landed. Both retirements match their design intent.

The residual moved; it did not vanish. Live-condense scoping fell from
26%-30% to 0.6%, but lineage-and-set recording rose from a gated-off 0%
to 7.7%, because recording is now unconditional. The family's live cost
today concentrates almost entirely in `persistHeaderLineageOwned` and its
two callees (7.3% cumulative), not in `alternativeSetInsert` or
`alternativeSetUnion` (0.3% cumulative, combined). An overflow fast path
already made the set math cheap: on this corpus, most sets saturate their
cap early, and both functions return immediately once a set carries the
overflow flag. The remaining cost sits above that math, in the per-header
transaction wrapper that runs whether or not the math underneath does
anything.

#### Ranked residual levers

Five candidate levers, ranked by addressable share:

1. **Further live-condense scoping reduction.** Addressable share: under
   1% cumulative CPU (0.6% combined, `query_compile`, the three-function
   row above). An earlier de-indirection fix already captured this
   lever's addressable share. The remainder is the scoping check's own
   cost, not avoidable call overhead. Not recommended: too little room
   left.
2. **Stage-3 scalar-field deletion.** `nodeLineageRecord` carries 36 bytes
   today, measured directly (`unsafe.Sizeof`). Two fields marked
   "transition only" in their own source comment, a 2-byte lineage ID and
   a 1-byte rank, could shrink it by roughly 11%. Both fields sit inside a
   component that already costs under 8% of wall time, so their removal
   has a ceiling near or under 1% of wall time. Worth doing for code
   hygiene; not a performance lever on its own.
3. **Recording-overhead reduction.** The set math is now cheap. The
   transaction wrapper around it, for each header, is not. Today,
   `persistHeaderLineageOwned` opens one scheduler-owned transaction for
   owner persistence on every header (`RecordHeadOwnerOwned`, 1.9%
   cumulative), and a second, separate transaction for lineage-and-set
   persistence on headers with a converged reduction split
   (`RecordHeadLineageOwned`, 4.0% cumulative). One source comment already
   states the merge goal for the second transaction's own two halves:
   halve the per-header token-validation cost. Folding the owner write
   into the same transaction is not free: the owner write fires on every
   header, and the lineage write fires only on converged ones, so a naive
   merge would change which headers get an owner write. A follow-up needs
   to preserve that distinction inside one transaction. Addressable
   share, before subtracting the work that must stay: up to 7.3%
   cumulative CPU on `query_compile`.
4. **The remaining L1 deterministic-frontier follow-ons.** An earlier
   lever selection modeled five single-head-frontier fast-path sub-items.
   A shipped change captured two of them: a checkpoint-digest skip inside
   election, and a canonical-group-key skip inside single-head
   canonicalization (roughly 4.3 modeled percentage points of an 8.1
   modeled total). Two more never landed: a direct-append path for
   `condenseWithOutcomeAtomic` on an exact single pop path, and a
   validation skip inside the dispatch loop's per-cell checks. Modeled
   addressable share for the two open sub-items: roughly 3.8% combined,
   against a `condenseWithOutcomeAtomic` share this run measured at 9.2%
   cumulative, the single largest function this profile named. The model
   predates this run by several days of intervening change and needs
   re-validation before a bet. The target function is still large and
   still un-optimized.
5. **The bytecode-scheduler structural bet.** Scheduler-dispatch,
   elections, reductions-and-pops, and canonicalization together still
   cover 60.2%-71.6% of wall time on every fixture in the component table
   above. None of the other four levers reach that scale; this is a
   separate, larger, and longer-horizon bet.

Recommendation: pursue item 4 next, ahead of item 3. Item 4 targets one
named function this run confirmed still costs 9.2% cumulative CPU. It
already has a capture model and two concrete, previously scoped
sub-items. Item 3 is real, but smaller: an overhead layer on an
already-cheap operation, entangled with a correctness distinction a
follow-up must preserve. It needs its own follow-up profile, to separate
wrapper cost from the work that must stay, before a precise size. Land
items 1 and 2 opportunistically. Neither earns a dedicated lever slot.
Item 5 stays the standing structural option, for whenever items 3 and 4
stop paying.

### Noise floor (interleaved A/A, identical binary, 12 pairs per fixture)

| fixture | median ns/op | p95 \|delta\| | p95 \|delta\| as % of median |
|---|---:|---:|---:|
| `grammargen_lr` | 123.099ms | 9.069ms | 7.4% |
| `language` | 12.020ms | 1.299ms | 10.8% |
| `query_compile` | 11.772ms | 1.165ms | 9.9% |
| `rewrite` | 2.401ms | 0.256ms | 10.7% |

Read this as: on this shared, uncontrolled host, two runs of the literal
same binary can disagree by roughly 7-11% before you have measured any real
difference. Any comparison this receipt states below that floor is not a
finding; it is noise.

### Cost per event (diagnostic lane; average, not causal)

| fixture | wall ns/op | ns/shift | ns/reduction | ns/pop path | ns/pop payload | ns/canonicalization | ns/election | ns/action lookup |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| `rewrite` | 2.674ms | 1984 | 1778 | 1625 | 894 | 1093 | 2581 | 753 |
| `query_compile` | 12.340ms | 1846 | 1643 | 1522 | 838 | 998 | 2407 | 708 |
| `language` | 13.195ms | 2026 | 1745 | 1575 | 834 | 1071 | 2609 | 707 |
| `grammargen_lr` | 126.019ms | 1906 | 1651 | 1506 | 830 | 1033 | 2525 | 684 |

The top-3 most expensive classes, by ns/event and consistent across all four
fixtures, are elections (2,407-2,609 ns), shifts (1,846-2,026 ns), and
reductions (1,643-1,778 ns). Elections costing more per event than shifts or
reductions is consistent with the shares table: on the diagnostic lane,
elections is the smallest of the four scheduler-family CPU shares
(4.7-9.5%) but fires the fewest times per fixture (1,036 on `rewrite`
versus 3,551 action lookups), so each election carries relatively more
weight. Full JSON: `docs/perf-attribution-receipt.json`.

### Which historical figure this receipt speaks to

Sum the four diagnostic-lane scheduler-family components
(scheduler-dispatch + elections + reductions-and-pops + canonicalization)
without lexing: 70.5% (`rewrite`), 64.2% (`query_compile`), 70.8%
(`language`), 60.7% (`grammargen_lr`) — average 66.5%. That number, on its
own, matches none of the three historical figures well.

Add lexing to that sum (scheduler-dispatch + elections +
reductions-and-pops + canonicalization + lexing): 84.8%, 84.0%, 84.0%,
80.3% — average 83.3%. This closely reproduces the **77-79% and 78-82%**
figures. Read literally, this means those two historical estimates most
likely counted token production as part of "scheduler cost per event" —
a defensible reading, since a shift cannot happen without first lexing the
token it shifts, but one this document did not previously state.

Sum only scheduler-dispatch and reductions-and-pops, the two components that
directly apply a table-driven action (excluding election/conflict overhead,
canonicalization bookkeeping, and lexing): 56.2%, 50.9%, 55.7%, 47.7% —
average 52.6%. This closely reproduces the **approximately-50%** figure. That
estimate most likely measured only the two dominant mechanical appliers and
implicitly folded election, canonicalization, and lexing cost into
"overhead" it did not separately name.

**Call: this receipt retires all three historical figures as decision
inputs**, not because any of them was numerically wrong, but because none of
them stated a boundary a later reader could reproduce or falsify. Each is a
reasonable reading of a different implicit boundary that this receipt now
makes explicit. From this tranche forward, campaign v7 should cite the
eight-component table above — with its stated boundary and disambiguation
rule — instead of any of the three legacy percentages.

### B9 addendum: the wall-removal receipt

Campaign v7 tranche B9 removed the compact admission switch's 64 KiB
source-length eligibility decline (`admission_switch.go`). The shipped route
now attempts every eligible full parse regardless of size; a scheduler
stop-control poll (tranche B8) bounds a large or pathological input instead
of a size wall. This addendum re-runs the documented command with
`grammargen_lr` added to the shipped-route lane (diagnostic-lane only in
every receipt above) and reports what changed. Per doctrine 10 (board
honesty), every row below still names its lane; nothing here merges a
diagnostic-lane number into a shipped-route claim or vice versa.

Generated 2026-08-02T11:02:14Z. Host: shared WSL2 development host,
background load uncontrolled -- the same local-development floor as every
prior receipt in this document, not a quiet-host or enclave measurement.
Git identity: `6fab988d30d42d931d42625eaa0cfa167d4877ba` (the capture-time
base commit; the wall-removal change is a working-tree diff on top of it at
capture time, so the tool's `git rev-parse HEAD` names the base, not a
B9-specific commit). This run's raw JSON output was not committed, matching
the existing `harness_out/` git-ignore rule; rerun the documented command to
reproduce it.

#### Attribution shares (regenerated; shipped route now covers all four fixtures)

| lane | fixture | wall ns/op | coverage % | scheduler-dispatch | elections | reductions-and-pops | canonicalization | lexing | materialization | compat-tail | recovery | other |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| diagnostic lane | rewrite | 2.381ms | 110.9 | 23.6 | 6.7 | 22.5 | 11.2 | 11.2 | 23.6 | 0.0 | 0.0 | 1.1 |
| diagnostic lane | query_compile | 11.548ms | 110.1 | 21.3 | 6.7 | 28.1 | 11.2 | 14.6 | 18.0 | 0.0 | 0.0 | 0.0 |
| diagnostic lane | language | 12.156ms | 109.7 | 26.1 | 2.3 | 28.4 | 12.5 | 15.9 | 13.6 | 0.0 | 0.0 | 1.1 |
| diagnostic lane | grammargen_lr | 126.493ms | 110.7 | 27.6 | 5.1 | 23.5 | 12.2 | 12.2 | 17.3 | 0.0 | 0.0 | 2.0 |
| shipped route | rewrite | 2.480ms | 109.9 | 15.9 | 3.4 | 30.7 | 12.5 | 21.6 | 15.9 | 0.0 | 0.0 | 0.0 |
| shipped route | query_compile | 11.661ms | 110.6 | 15.7 | 4.5 | 30.3 | 15.7 | 13.5 | 20.2 | 0.0 | 0.0 | 0.0 |
| shipped route | language | 12.782ms | 110.5 | 16.9 | 4.5 | 30.3 | 9.0 | 18.0 | 20.2 | 0.0 | 0.0 | 1.1 |
| **shipped route** | **grammargen_lr** | **140.751ms** | 111.3 | 11.7 | 10.6 | 29.8 | 8.5 | 19.1 | 20.2 | 0.0 | 0.0 | 0.0 |

`grammargen_lr` has a shipped-route row for the first time. The public
`Parser.Parse` route, with no per-Parser pin and no diagnostic flag, admitted
it at 140.751ms against the diagnostic lane's 126.493ms for the same
fixture -- a wall-time gap of 11.3% (not to be confused with the unrelated
111.3 coverage % figure in that same table row above; coverage % is
profiled-CPU-time over wall-time, see "Attribution shares" earlier in this
document). Read this 11.3% gap against the noise floor below before treating
it as a finding. A same-shape gap would be consistent with the compat-tail
and admission-switch bookkeeping the shipped route pays and the diagnostic
runner does not (`normalizeReturnedTreeForParse`,
`resolveCRecoverySwallowedError`, `maybeCompactReturnedFullTree`, plus
`Parser.Parse`'s own dispatch); `compat-tail` itself still reads 0.0% because
this run's four fixtures stay clean (no error nodes, no C-recovery-swallowed
error to resolve), so any such cost here is admission-switch dispatch
overhead, not compat-tail's own conditional work. Every shipped-route sample
was verified, by the admission-candidate routed/fallback counters, to have
actually taken the compact route rather than falling back to production.

#### Noise floor (this run, interleaved A/A, 10 pairs per fixture)

| fixture | pairs | median ns/op | p95 \|delta\| | p95 \|delta\| as % of median |
|---|---:|---:|---:|---:|
| `grammargen_lr` | 10 | 120.831ms | 27.151ms | 22.47% |
| `language` | 10 | 12.164ms | 2.994ms | 24.61% |
| `query_compile` | 10 | 11.770ms | 4.986ms | 42.36% |
| `rewrite` | 10 | 2.458ms | 378.273us | 15.39% |

This run's noise floor is wide (15.4%-42.4% of median), consistent with a
busy shared host (other agents were active throughout this session). The
11.3% shipped-route/diagnostic-lane gap on `grammargen_lr` above sits BELOW
every one of these four floors, including `grammargen_lr`'s own (22.47%) and
`rewrite`'s (the narrowest, at 15.39%). This run's noise cannot resolve that
gap from zero: it is not a finding, directional or otherwise, until a
quieter run repeats the measurement with a floor tighter than 11.3%.

#### Cost per event (diagnostic lane; average, not causal)

| fixture | wall ns/op | ns/shift | ns/reduction | ns/pop path | ns/pop payload | ns/canonicalization | ns/election |
|---|---:|---:|---:|---:|---:|---:|---:|
| `rewrite` | 2.381ms | 1766 | 1583 | 1446 | 795 | 973 | 2298 |
| `query_compile` | 11.548ms | 1728 | 1538 | 1424 | 784 | 934 | 2253 |
| `language` | 12.156ms | 1867 | 1607 | 1451 | 769 | 987 | 2403 |
| `grammargen_lr` | 126.493ms | 1913 | 1658 | 1512 | 833 | 1036 | 2534 |

Every per-event reading sits within this document's established noise
floors from prior receipts. The wall-removal change moved routing, not the
compact engine's per-event cost.

This is a local, one-run, noisy-host receipt, not a sealed epoch. It does
not replace or invalidate the first receipt or its addenda above (all
measured before tranche B9 landed, correctly showing `grammargen_lr`
diagnostic-lane only at that time); it extends the same methodology to the
newly shipped-route-eligible fixture.

## 2026-08-22 P24a memo-probe performance receipt

Status: **NO-GO**. Do not ship the candidate code.

The receipt uses base commit `6c94426b69f5aa106873b0d667dd4112acc138f9`.
The candidate changes only these files:

- `parser_recover_c.go`
- `parser_recover_c_test.go`

The full candidate diff SHA-256 is
`d56566edb8d6587a13935dc13bc173db892b099950fb612f5c95c688f93cdeea`.
Compute it with:

```text
git diff --no-ext-diff --binary 6c94426b69f5aa106873b0d667dd4112acc138f9 -- parser_recover_c.go parser_recover_c_test.go | sha256sum
```

### Accepted comparison

The accepted publication used 20 seeds and 40 isolated benchmark processes.
Each seed ran one baseline process and one candidate process.
Odd seeds ran baseline first. Even seeds ran candidate first.
Each process used `GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`, and
`-benchmem`. The benchmark expression selected the three canonical controls.

All 40 benchmark processes passed and emitted 120 benchmark rows.
No run reported an out-of-memory event, timeout, panic, or contamination
marker.

The focused Docker gate passed at
`/tmp/gts-p24a-cnode-memo-probe/harness_out/docker/20260822T170841Z-p24a-refresh-6c94426b-focused15-corrected-20260822`.
The historical `focused15` label is not a test count. The command ran 17
top-level tests and 20 tests when its three subtests were counted.

Exclude both prior attempts from this receipt.
Exclude the earlier focused attempt at
`/tmp/gts-p24a-cnode-memo-probe/harness_out/docker/20260822T170828Z-p24a-refresh-6c94426b-focused15-20260822`.
Its expression ran only five non-memo tests.
Exclude the quarantined publication at
`/tmp/gts-p24a-publication-20260822T170915Z`.
It contains the `QUARANTINED` marker and no accepted benchmark rows.

### Benchstat results

The following values are benchstat medians from
`/tmp/gts-p24a-analysis-20260822T171127Z/benchstat.txt`.

| Benchmark | Baseline | Candidate | p-value | Result |
|---|---:|---:|---:|---|
| `BenchmarkGoParseIncrementalNoEditDFA` | 2.342n | 2.349n | 0.401 | No significant change |
| `BenchmarkGoParseIncrementalSingleByteEditDFA` | 721.2n | 727.9n | p<0.001 | **+0.93% regression** |
| `BenchmarkGoParseFullDFA` | 6.604m | 6.620m | 0.242 | No significant change |

The geomean change is +0.48%. The single-byte edit regression is significant.

### Paired results

The paired means use one baseline and one candidate value for each seed.
Each interval is a 95% paired confidence interval.

| Benchmark | Mean delta | 95% confidence interval | Relative mean |
|---|---:|---:|---:|
| `BenchmarkGoParseIncrementalNoEditDFA` | +0.00345 ns/op | [-0.001566, +0.008466] ns/op | +0.1486% |
| `BenchmarkGoParseIncrementalSingleByteEditDFA` | +5.79 ns/op | [+3.607, +7.973] ns/op | +0.8031% |
| `BenchmarkGoParseFullDFA` | +11,140.35 ns/op | [-18,442, +40,722] ns/op | +0.1718% |

The order split reports candidate versus baseline means:

| Benchmark | Odd seeds, baseline first | Even seeds, candidate first |
|---|---:|---:|
| `BenchmarkGoParseIncrementalNoEditDFA` | +0.077% | +0.218% |
| `BenchmarkGoParseIncrementalSingleByteEditDFA` | +0.769% | +0.834% |
| `BenchmarkGoParseFullDFA` | +0.099% | +0.238% |

The candidate remains slower in both orders. The order split does not invert.

### Memory and decision

Incremental benchmarks use 0 B/op and 0 allocations per operation in both
variants. Full parsing uses 50.50 KiB/op in both variants and 4 allocations
per operation in both variants. Benchstat reports p=0.066 for full B/op and
p=1.000 for every allocation comparison.

Use this reject rule: treat the first statistically significant positive
delta as the regression point. Reject a candidate when a required lane has
such a delta. Also reject an out-of-memory event, timeout, contamination
marker, or correctness failure.

P24a fails this rule because the single-byte edit lane is +0.93% slower at
p<0.001. Its paired interval is wholly positive. No code ships from this
candidate.

### Receipt sources

- Publication: `/tmp/gts-p24a-publication-20260822T171127Z`
- Analysis: `/tmp/gts-p24a-analysis-20260822T171127Z/benchstat.txt`
- Focused Docker gate: `/tmp/gts-p24a-cnode-memo-probe/harness_out/docker/20260822T170841Z-p24a-refresh-6c94426b-focused15-corrected-20260822`

## 2026-08-24 P25b issue #454 performance blocker receipt

Status: **NO-GO**. Keep issue #454 and the performance arm live.

This receipt records the P25b baseline investigation. It does not record a
candidate code change. The source worktree was at commit
`18d63b6f7802b28a0ddb889327fcd4ebebb99426`. This docs-only receipt is based on
main commit `b800161da38ce04ed66f29095d74818ae918e4d8`.

### Witness construction

Build the source with `benchfixtures.Issue454CSource`.
The constructor writes repeated C functions and pads the result with spaces.
The base source length is exactly 137 KiB, or 140,288 bytes.
The edit deletes the first `x` in `x0` at byte offset 19.
The edited source length is 140,287 bytes.

The base source SHA-256 is
`8c4f63057e4b28fdcbbc7e08763078d9aa23dee7486f6fca56247e107ff7a98d`.
The edited source SHA-256 is
`600bf905d3c001a25546c33d230e4e56016b4ffabdd0197ce4ea988f57b1be72`.

The phrase “exact 137 KiB Go/C parity witness” is not correct for the edited
tree. The Go and C base trees match exactly. The edited trees do not.
Use “137 KiB base-source witness with a known edited-tree divergence”.

### Fresh and incremental profiles

The profile used one CPU, `GOMAXPROCS=1`, three runs, and one test process.
The fresh lane parsed the 140,287-byte edited source.
Each fresh Go tree had 205,934 nodes, reached byte 140,287, and had an error.
All three fresh Go digests were
`9c979bb436f92e7f96885454de81d9d95d2befff242145b1026addf5d9395c4d`.

| Lane | Wall time | Go allocations | Maximum RSS | Result |
|---|---:|---:|---:|---|
| Fresh, three runs | 186.7 to 293.8 ms | 31.8 to 121.9 MB; 734,803 to 742,605 mallocs | 575,520 KiB | Accepted error tree |
| Incremental, three runs | 13.47 to 17.80 s | 747.2 to 819.1 MB; 9,085 to 9,287 mallocs | 921,456 KiB | Replaced by fresh full parse |

`Tree.Edit` took 1.704 to 1.974 ms and allocated 224 bytes in five mallocs.
The reuse cursor took 181 to 360 microseconds.
The profile recorded three reused subtrees and ten reused bytes.
It rejected one dirty node, five dirty ancestors, and four changed root
non-leaf nodes. It recorded no scanner, fragile-node, or block-splice rejects.

The incremental profile recorded 3,165,354 to 3,213,594 new nodes.
These nodes belong to the discarded memory-budget attempt.
The final runtime recorded 205,934 nodes, 39,558,912 arena bytes,
22,812,236 scratch bytes, and 16,810,028 GSS bytes.
The final tree reached byte 140,287 and had an error.

The exact retry reason was
`incremental_parse_memory_budget_full_retry`.
The fallback released the failed tree and selected one default-budget full
parse. The profile reports zero ordinary retry passes because this fallback
does not use the widening retry ladder.

The detailed attempt receipt resets when the fallback starts.
It therefore retains the selected full parse, not the discarded attempt.
The selected attempt took 145.0 to 308.4 ms.
Subtracting it from the measured incremental wall time infers
13.32 to 17.49 seconds for the discarded attempt.
Treat that interval as an inference, not a direct attempt timestamp.

Recovery telemetry recorded one recovery entry and 44,532 cost competitions
per run. Recovery cost walks took 22.13 to 35.26 ms.
The final runtime marked C recovery as entered and marked a clean result as
dropped before the fallback replaced it.

### Correctness and locked-C result

The focused Go Docker test passed all three edit classes.
Replace, insert, and delete all matched their fresh Go trees.
The delete case required the exact memory-budget fallback reason.
The test also confirmed a clean, deterministic base tree.

The one KiB locked-C test passed its known-divergence ratchet.
It recorded this first difference:

```text
/translation_unit/function_definition[0]/compound_statement[2]/ERROR[2]/number_literal[0]
category=error Go=true C=false
```

The 137 KiB locked-C test also passed.
Its base digests matched:

```text
Go base = 3f51a3e4c10824342dd1a92eee794c94548d214c4cbbf295bec07ef734b9fbf3
C base  = 3f51a3e4c10824342dd1a92eee794c94548d214c4cbbf295bec07ef734b9fbf3
```

The edited digests did not match:

```text
Go fresh       = 9c979bb436f92e7f96885454de81d9d95d2befff242145b1026addf5d9395c4d
Go incremental = 9c979bb436f92e7f96885454de81d9d95d2befff242145b1026addf5d9395c4d
C fresh        = 8fe04819317a4f225b5c298a71acdceb5aa965abb3a397e35eb48c5849888d5c
```

Go incremental and Go fresh are equal.
Both differ from C at the error flag shown above.
The locked-C test accepts this known divergence and checks Go self-consistency.
It does not prove exact Go/C parity.

### Canopy call path

The narrow, no-cache Canopy query recorded this path:

```text
ParseIncrementalProfiled
  -> parseIncrementalChangedProfiled
  -> retryIncrementalMemoryBudgetAsPlainFullWithDFA
  -> parseInternal with no old tree and the default budget
```

The changed-path call is at `parser_api.go:1741`.
The fallback implementation is at `parser_retry.go:2137`.
The profile test calls `Tree.Edit` at `p25b_issue454_profile_test.go:105`.

### Decision and reopening condition

Do not publish a performance candidate from this receipt.
Do not call the edited witness C-exact.
Keep the performance arm live until a producer fix removes the first error
divergence and preserves the memory-budget fallback.

Reopen a bounded performance candidate only after all of these conditions hold:

- Match the 137 KiB edited witness against the locked C tree by deep digest.
- Preserve equality between fresh Go and incremental Go trees.
- Preserve the fallback reason and final root span.
- Report the discarded attempt and the selected runtime separately.
- Repeat the focused Docker correctness and locked-C tests.
- Run a quiet, reproducible performance comparison with the same settings.

### P25b artifact paths

- Fresh profile: `/tmp/gts-p25b-artifacts/20260823T025558Z-c137-fresh-base-v2`
- Incremental profile: `/tmp/gts-p25b-artifacts/20260823T025620Z-c137-incremental-delete-base`
- Go correctness: `/tmp/gts-p25b-artifacts/20260823T025804Z-c137-correctness-go-base`
- Locked-C one KiB: `/tmp/gts-p25b-artifacts/20260823T025832Z-c137-locked-c-1k-base`
- Locked-C 137 KiB: `/tmp/gts-p25b-artifacts/20260823T030032Z-c137-locked-c-137k-base-v2`
- Canopy profile path: `/tmp/p25b-canopy-profiled.json`
- Canopy retry callers: `/tmp/p25b-canopy-retry2.json`
- Canopy `Tree.Edit` callers: `/tmp/p25b-canopy-edit-all.json`

Exclude the failed construction attempts from this receipt:

- `/tmp/gts-p25b-artifacts/20260823T025456Z-c137-fresh-base`
- `/tmp/gts-p25b-artifacts/20260823T025935Z-c137-locked-c-137k-base`

## 2026-08-24 P25c issue #454 Objective-C transient-delete blocker receipt

Status: **NO-GO**. Keep issue #454 and the performance arm live.

This receipt records the P25c Objective-C transient-delete investigation.
It does not record a candidate code change. The source worktree used commit
`838aba943038248529429a572c4d6d98359bd87e`. This docs-only receipt is based on
main commit `db3105ce9f253b9a4b17ca83c4d61c6ccf6fe0fd`.
No production or test change survives.

### Witness and method

The fixture reconstructs repeated Objective-C methods and pads each source.
The base edit deletes the `x` in the first `x0` identifier.
The insert control adds one `x` before that identifier.
The delete control creates the transient-error path.

This synthetic fixture is not an authenticated issue #454 corpus.
Do not call this synthetic witness C-exact.

The sweep covered 4, 64, 256, 512, 640, 768, 896, and 1,024 KiB.
Each size ran in one Docker process. The run used one CPU and `GOMAXPROCS=1`.
The run used `GOFLAGS=-p=1`, one test worker, and `GOMEMLIMIT=6GiB`.
The container memory limit was 8 GiB. No workload was active before the sweep.

The table gives wall time in milliseconds. RSS means maximum resident set size.

| Base | Full | Fresh insert | Incremental insert | Fresh delete | Incremental delete | RSS KiB |
|---:|---:|---:|---:|---:|---:|---:|
| 4 KiB | 3.658 | 1.167 | 1.293 | 13.478 | 13.797 | 609,120 |
| 64 KiB | 18.988 | 19.921 | 18.253 | 240.219 | 221.642 | 592,160 |
| 256 KiB | 91.708 | 67.277 | 80.331 | 990.174 | 976.860 | 603,360 |
| 512 KiB | 152.709 | 132.829 | 153.948 | 2,129.643 | 2,021.328 | 593,920 |
| 640 KiB | 214.215 | 165.593 | 189.911 | 2,842.216 | 2,996.034 | 984,112 |
| 768 KiB | 264.178 | 186.887 | 227.263 | 3,401.347 | 3,521.943 | 1,120,356 |
| 896 KiB | 296.456 | 267.544 | 267.031 | 3,998.901 | 3,992.069 | 1,121,428 |
| 1,024 KiB | 353.429 | 258.753 | 381.133 | 4,562.481 | 4,477.760 | 1,255,104 |

The sweep shows no discrete threshold between 512 KiB and 1 MiB.
Delete time grows with source size and shows normal run variation.
The 640 KiB RSS increase does not change the retry or recovery structure.

### Attribution and resources

The profile separates edit, reuse selection, and reparse work.
The table gives the incremental timings in milliseconds.

| Base | Insert edit | Insert reuse | Insert reparse | Delete edit | Delete reuse | Delete reparse |
|---:|---:|---:|---:|---:|---:|---:|
| 4 KiB | 0.020 | 0.209 | 1.050 | 0.020 | 0.011 | 3.434 |
| 64 KiB | 0.273 | 2.741 | 15.347 | 0.272 | 0.015 | 51.469 |
| 256 KiB | 2.233 | 9.735 | 69.306 | 1.959 | 0.021 | 205.875 |
| 512 KiB | 5.527 | 19.990 | 130.415 | 5.436 | 0.020 | 413.120 |
| 640 KiB | 7.114 | 24.409 | 161.095 | 7.266 | 0.021 | 641.736 |
| 768 KiB | 8.872 | 29.604 | 191.340 | 8.599 | 0.018 | 665.690 |
| 896 KiB | 10.483 | 34.164 | 225.275 | 9.909 | 0.018 | 818.165 |
| 1,024 KiB | 11.236 | 43.374 | 329.556 | 11.767 | 0.023 | 867.905 |

At 1,024 KiB, the incremental delete took 4,477.760 ms.
`Tree.Edit` took 11.767 ms. Reuse selection took 0.023 ms.
Reparse took 867.905 ms. The parser loop took 767.362 ms.
Result selection took 59.762 ms. Result-tree build took 0.017 ms.

The 1,024 KiB delete allocated 848,689,760 heap bytes.
It allocated 1,608,685,968 total bytes in 3,857 mallocs.
It created 1,515,998 new nodes. Arena, scratch, and GSS bytes were
217,695,656, 103,489,980, and 100,715,300.
The maximum RSS was 1,255,104 KiB.

The incremental-delete allocation receipts were:

| Base | Heap bytes | Total bytes | Mallocs | New nodes | Recovery competitions |
|---:|---:|---:|---:|---:|---:|
| 4 KiB | 2,986,752 | 2,986,752 | 296 | 6,956 | 3,400 |
| 64 KiB | 30,779,992 | 30,779,992 | 502 | 103,620 | 50,608 |
| 256 KiB | 164,929,600 | 192,926,672 | 1,095 | 395,160 | 192,988 |
| 512 KiB | 290,100,032 | 347,076,816 | 1,892 | 783,794 | 382,786 |
| 640 KiB | 828,444,512 | 1,003,922,912 | 2,352 | 970,586 | 474,010 |
| 768 KiB | 1,038,118,424 | 1,214,357,984 | 2,677 | 1,152,390 | 562,798 |
| 896 KiB | 1,008,104,296 | 1,402,656,736 | 2,943 | 1,334,194 | 651,586 |
| 1,024 KiB | 848,689,760 | 1,608,685,968 | 3,857 | 1,515,998 | 740,374 |

Every insert used the old-tree reuse route and one stack.
Every delete reused 53 bytes and rejected one dirty node.
Each delete rejected seven dirty ancestors and five changed root non-leaves.
No scanner or fragile-node rejection occurred.
Every delete used five attempt entries:

- `initial`
- `initial_merge`
- `clean_wide`
- `recovery_wide_or_node`
- `final_merge`

Each delete used four retry passes and selected `initial`.
The retry reason was empty. Accepted-error retry attempts remained zero.
Each delete entered C recovery and recorded one recovery entry.
Each delete recorded `CRecoveryDroppedErrorForClean=true`.
The swallowed-error fallback did not run.

Recovery competitions ranged from 3,400 at 4 KiB to 740,374 at 1,024 KiB.
The memory budget was 536,870,912 bytes for every incremental delete.
No run stopped for that budget. No run truncated its source.
Normalization ran zero passes and rewrote zero nodes.

Every fresh and incremental insert digest matched.
Every fresh and incremental delete digest matched.
The 512 KiB correctness receipt records these delete digests:

```text
fresh       = 346ff2142b2069c7a59b6f3f9ba6a06c0cac4401a97ba70cd83dceb85f186bc2
incremental = 346ff2142b2069c7a59b6f3f9ba6a06c0cac4401a97ba70cd83dceb85f186bc2
```

### Correctness and locked-C result

The focused Go Docker correctness test passed.
It checked fresh and incremental roots, spans, errors, and digests.
The focused Objective-C locked-C tests also passed.
Those tests cover nearby Objective-C witnesses, not this synthetic source.
They do not prove exact locked-C parity for the transient delete.

### Canopy path and helper decision

The narrow, no-cache Canopy query recorded this path:

```text
ParseIncrementalProfiled
  -> parseIncrementalChangedProfiled
  -> parseIncrementalInternal
  -> retryIncrementalAcceptedErrorWithDFA
  -> normalizeReturnedIncrementalTree
```

The retry caller query found both changed incremental callers.
The stack-cap query found `effectiveFullParseInitialMaxStacks`.
That helper caps default Objective-C full parses at two stacks.
The transient incremental delete reached five stacks.
The helper is grammar-specific. Reject it as a grammar-agnostic candidate.

### Decision and reopening condition

Do not publish a performance candidate from this receipt.
Do not change production or registry state.
Keep issue #454 and the performance arm live.

Reopen a bounded candidate only after all conditions hold:

- Use an authenticated Objective-C issue #454 corpus.
- Repeat a quiet sweep around 512, 640, 768, 896, and 1,024 KiB.
- Use at least three samples per size or an approved 20-seed campaign.
- Preserve fresh and incremental deep-digest equality.
- Prove exact locked-C parity for the edited witness.
- Record the first divergence when parity fails.
- Bound recovery retries and memory use.
- Consider a generic helper only after a profile names a safe helper.
- Repeat the focused correctness and locked-C Docker gates.

### P25c artifact paths

- Correctness: `/tmp/gts-p25c-artifacts/20260823T034301Z-objc-correctness-512k-v2`
- Locked-C: `/tmp/gts-p25c-artifacts/20260823T034551Z-objc-locked-c-focused`
- 4 KiB: `/tmp/gts-p25c-artifacts/20260823T034351Z-objc-4k-baseline-v2`
- 64 KiB: `/tmp/gts-p25c-artifacts/20260823T034402Z-objc-64k-sweep-v2`
- 256 KiB: `/tmp/gts-p25c-artifacts/20260823T034411Z-objc-256k-sweep-v2`
- 512 KiB: `/tmp/gts-p25c-artifacts/20260823T034023Z-objc-512k-sweep-v2`
- 640 KiB: `/tmp/gts-p25c-artifacts/20260823T033951Z-objc-640k-sweep-v2`
- 768 KiB: `/tmp/gts-p25c-artifacts/20260823T034055Z-objc-768k-sweep-v2`
- 896 KiB: `/tmp/gts-p25c-artifacts/20260823T034203Z-objc-896k-sweep-v2`
- 1,024 KiB: `/tmp/gts-p25c-artifacts/20260823T034124Z-objc-1m-sweep-v2`
- Incremental Canopy: `/tmp/p25c-canopy-incremental.json`
- Retry Canopy: `/tmp/p25c-canopy-retry.json`
- Stack-cap Canopy: `/tmp/p25c-canopy-maxstacks.json`

## 2026-08-24 P25d issue #454 C# fresh-parse performance blocker receipt

Status: **KEEP LIVE / NO-GO**. Keep issue #454 live. Ship no candidate code.

This receipt records a fresh-parse C# performance investigation. It does not
measure incremental C# performance. The temporary probe changed no production
or test code in the repository. Its source worktree used base commit
`731f8a9d9440a006b2cc6b56ef5b31c0ff3b5ce7`.

The probe generated synthetic C# source. It deleted the first `x` from `x0`
in the edited source. The edit produced `var 0`, which enters recovery. The
source is not an authenticated issue #454 corpus. It has no direct locked-C
parity proof. Treat every result as a diagnostic performance witness.

### Source sizes and hashes

The probe requested 2, 4, 8, 16, and 32 kibibytes (KiB). The generated source
lengths differ from those targets because the constructor adds C# text.

| Requested | Clean bytes | Clean SHA-256 | Edited bytes | Edited SHA-256 |
|---:|---:|---|---:|---|
| 2 KiB | 2,102 | `9c9f78d2043c8d27e3e8012d8243f6684c295935a3e5ec9852083189fc64ccac` | 2,101 | `deba24ea52600d712a86dacded0d2371525930b440598f7746ca0bc0cfd450f2` |
| 4 KiB | 4,152 | `bc47d64777684a917aad73c5fc69cfb3e0fec778e3a5624e8eeb241f7fbff0e0` | 4,151 | `3cb49ea0a1697e6474c2a06072e2cb89c3e068a7f4154a2b7364c82741bb8ed7` |
| 8 KiB | 8,257 | `399145dcd25b8d4fe973d3d4176885d7ffb8b3e56f79676c6a109a8f1871d66f` | 8,256 | `7ff5a2e0e6669d92637136e45e11f2f4d21a8fe5d24599a11dd6f9115bf2c131` |
| 16 KiB | 16,435 | `60720c7371d1bdc14dee2c462fd1a6d9c71e71ee8a839b84bf6278823d39f3c9` | 16,434 | `bfa97fc71f5fdbce2c3240b548e34713f1902343c95c6d0310d2c8aa14a0079a` |
| 32 KiB | 32,791 | `7d179877f56356777aa9861afda11db0517eb8226c2dfdc610192180a30b25cd` | 32,790 | `abab13f6f776074a408b97661c57c30224a1fc866d8f3d647d51a29537f3e4a6` |

### Fresh production results

The production lane materialized a tree from each edited source. The wall
times below are approximate values from corrected production runs.

| Requested | Edited bytes | Wall time | Nodes | Maximum stacks | Peak depth | Recovery cost walks | Heap delta | Mallocs | Selected attempt |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| 2 KiB | 2,101 | 60.2 ms | 2,861 | 10 | 14 | 2,485 | 55,433,064 | 25,097 | `initial` |
| 4 KiB | 4,151 | 28.0 ms | 25,638 | 16 | 65 | 48,262 | 31,630,600 | 742 | `initial_merge` |
| 8 KiB | 8,256 | 60.3 ms | 52,038 | 16 | 115 | 100,862 | 40,700,552 | 860 | `initial_merge` |
| 16 KiB | 16,434 | 113.9 ms | 101,670 | 16 | 209 | 199,750 | 60,607,464 | 1,066 | `initial_merge` |
| 32 KiB | 32,790 | 243.3 ms | 200,154 | 16 | 397 | 397,528 | 87,286,120 | 1,468 | `initial_merge` |

The 2 KiB production record reports about 60.2 milliseconds. A separate
instrumented 2 KiB run reports 71.1 milliseconds. Do not treat 71.1
milliseconds as the production timing. Its artifact is
`/tmp/gts-p25d-csharp-20260824.gMbLCq/artifacts/docker/20260823T042419Z-p25d-csharp-production-2k-timing`.

The first 2 KiB attempt was clean. The selected `initial` attempt therefore
has `root_error=false`, `c_recovery_entered=true`, and
`c_recovery_dropped_error=false`. The 4 KiB through 32 KiB records selected
`initial_merge`, set `root_error=true`, entered C recovery, and dropped the
clean result. Each record reports one retry pass. The 4 KiB through 32 KiB
retry reason is `initial_result_requires_merge_width`.

The final 32 KiB record is also confirmed by
`/tmp/gts-p25d-csharp-20260824.gMbLCq/artifacts/docker/20260823T043142Z-p25d-csharp-production-32k-final`.
It reports about 244.5 milliseconds.

### Clean, raw, and no-tree controls

The clean control parsed the unedited source. The raw control parsed the
edited source without the production wrapper. The no-tree control parsed the
edited source without tree materialization.

| Requested | Clean production | Raw edited | No-tree edited |
|---:|---:|---:|---:|
| 2 KiB | 2.86 ms, root error false | 15.14 ms, root error true | 2.56 ms, root error false |
| 4 KiB | 4.60 ms, root error false | 28.99 ms, root error true | 3.82 ms, root error false |
| 8 KiB | 7.30 ms, root error false | 61.00 ms, root error true | 7.72 ms, root error false |
| 16 KiB | 13.20 ms, root error false | 105.36 ms, root error true | 7.68 ms, root error false |
| 32 KiB | 29.19 ms, root error false | 232.43 ms, root error true | 12.73 ms, root error false |

The 32 KiB production record reports 200,154 nodes, depth 397, 397,528
recovery cost walks, 87,286,120 heap bytes, and 1,468 mallocs. The clean
32 KiB control reports 25,289 nodes. The no-tree edited control reports
28,347 nodes and does not materialize a tree.

### RSS, scanner, and retry limits

RSS means maximum resident set size. The production RSS sequence was:

| Requested | RSS |
|---:|---:|
| 2 KiB | 44,968 KiB |
| 4 KiB | 39,796 KiB |
| 8 KiB | 51,012 KiB |
| 16 KiB | 62,528 KiB |
| 32 KiB | 89,984 KiB |

The probe records an external scanner. The C# scanner does not support
incremental reuse. Locked-C route logs report `incremental_reuse=false` and
`external_scanner_unsupported`. P25d therefore reports no `Tree.Edit`, reuse,
or incremental reparse timing.

Every final production record reports one retry pass. The 2 KiB record
selected `initial`. The larger records selected `initial_merge`. The final
2 KiB root is clean. The larger edited roots report errors. All edited
production records entered C recovery.

Every one of the 32 Docker metadata records reports
`wall_timed_out=false` and `oom_killed=false`. Three records failed before a
valid run. Quarantine them instead of counting them as performance results.

### Coarse pprof and Canopy attribution

The 32 KiB production CPU profile is
`/tmp/gts-p25d-csharp-20260824.gMbLCq/artifacts/profiles/p25d-csharp-production-32k.pprof`.
Its SHA-256 is
`98b3e8d16a123273c14eefdb6035f7817b815a49e218e6d6ee64185b55cdabb2`.
The text report is
`/tmp/gts-p25d-csharp-20260824.gMbLCq/artifacts/profiles/p25d-csharp-production-32k.pprof.txt`.
Its SHA-256 is
`16da805a8e5a77e88104ce7d0ed3a35d339ee1ed5c7b4d1cc8a32445e2c9a523`.

The profile ran for 433.96 milliseconds and sampled 280 milliseconds.
`parseInternal` accounts for 82.14% cumulative CPU. Other broad frames
include `applyShiftAction`, `cStackPrefixCostForMerge`, `stackCompareMerge`,
`stackHash`, graph-structured stack (GSS) hash and merge, and arena node
creation. Sparse samples support only coarse attribution. They do not prove a
safe local optimization.

The Canopy artifacts are:

- `/tmp/gts-p25d-csharp-20260824.gMbLCq/artifacts/canopy/dispatcher-census-depth2.json`
- `/tmp/gts-p25d-csharp-20260824.gMbLCq/artifacts/canopy/normalize-csharp-depth2.json`
- `/tmp/gts-p25d-csharp-20260824.gMbLCq/artifacts/canopy/parse-internal-depth2.json`
- `/tmp/gts-p25d-csharp-20260824.gMbLCq/artifacts/canopy/recovered-top-level-chunks-depth2.json`
- `/tmp/gts-p25d-csharp-20260824.gMbLCq/artifacts/canopy/stack-error-cost-depth2.json`
- `/tmp/gts-p25d-csharp-20260824.gMbLCq/artifacts/canopy/stack-prefix-cost-depth2.json`

The narrow query traces incremental parsing through
`parseIncrementalChangedProfiled`, `parseIncrementalInternal`, and
`retryIncrementalMemoryBudgetAsPlainFullWithDFA`. P25d did not measure that
path. The stack-cost query reaches
`cStackPrefixCostForMerge` and generic node visibility and error-cost helpers.

### Locked-C routes

The corrected route test passed at
`/tmp/gts-p25d-csharp-20260824.gMbLCq/artifacts/docker/20260823T043105Z-p25d-csharp-locked-c-routes-v2`.
It used one CPU, one test worker, `GOMAXPROCS=1`, and a 30-minute timeout.
The metadata reports exit code zero, no timeout, and no out-of-memory event.

The four route witnesses produced these results:

- The JSON-reader witness diverged in raw shape, with five Go children and six C children. Production, compact, and incremental error flags differed.
- The simple positive witness matched all routes.
- The historical issue #454 witness differed in an error flag at `ERROR[1]/integer_literal[0]`. Compact fallback and forest decline matched the route logs.
- The malformed missing-body witness differed by one extra error node. Compact fallback and forest decline matched the route logs.

These route results do not establish direct parity for the synthetic sources.
They show known route behavior and a positive control only.

### Quarantined artifacts

Quarantine these failed artifacts:

- `/tmp/gts-p25d-csharp-20260824.gMbLCq/artifacts/docker/20260823T042231Z-p25d-csharp-production-2k` — constructor field type errors.
- `/tmp/gts-p25d-csharp-20260824.gMbLCq/artifacts/docker/20260823T042311Z-p25d-csharp-production-2k-v2` — remaining constructor field type errors.
- `/tmp/gts-p25d-csharp-20260824.gMbLCq/artifacts/docker/20260823T043054Z-p25d-csharp-locked-c-routes` — wrong package path.

Use these corrected replacements:

- `/tmp/gts-p25d-csharp-20260824.gMbLCq/artifacts/docker/20260823T042327Z-p25d-csharp-production-2k-v3`
- `/tmp/gts-p25d-csharp-20260824.gMbLCq/artifacts/docker/20260823T043105Z-p25d-csharp-locked-c-routes-v2`

### Decision and reopening condition

Do not publish a performance candidate from this receipt. Keep issue #454
live. The synthetic source is fresh, not incremental. It has no direct
locked-C parity proof. The scanner limitation blocks an incremental result.

Reopen a bounded performance candidate only after all conditions hold:

- Use an authenticated issue #454 C# source.
- Match fresh and incremental Go trees to the locked C tree by deep digest.
- Run separate clean and malformed lanes.
- Measure an incremental lane only when scanner reuse is supported.
- Preserve retry selection, error flags, recovery behavior, and memory-budget contracts.
- Record `Tree.Edit`, reuse selection, and reparse or rebuild attribution separately.
- Repeat focused Docker correctness and locked-C route gates.
- Use a quiet reproducible profile before proposing one generic candidate.

The P25d evidence supports **KEEP LIVE / NO-GO**. No parser or test change
survives.

### P25d artifact root

The audited artifact root is
`/tmp/gts-p25d-csharp-20260824.gMbLCq/`. The temporary probe is
`/tmp/gts-p25d-csharp-20260824.gMbLCq/repo/grammars/p25d_csharp_probe_test.go`.
The probe remains outside this documentation patch.

## 2026-08-24 P25ac-P25ae incremental arena retention receipt

Status: **NO-GO**. Keep issue #454 open. Ship no candidate code.

This receipt supersedes the P25ac primary-route screen. P25ac measured a
same-length edit that used the token-invariant leaf fast path. P25ad traced
the route and selected the authenticated Go `recovery_deletion` edit.
P25ae then measured a temporary incremental arena retention ceiling.

The evidence base for P25ac is
`8c80a46e450d906fc9ce1665c189497b02483a3e`.
The evidence and candidate base for P25ad and P25ae is
`af8a9a5bdb5bd1ac03762bc9a4f1a89f42463682`.
This docs-only receipt uses publication base
`6a22fcf82c4d84ab613c68084ad356eb52bb4eac`.

### P25ac route attribution

P25ac used the generated Go incremental single-byte edit benchmark. The edit
had equal old and new lengths. The route entered
`tryTokenInvariantLeafEdit` and returned through `reuseTreeWithNewSource`.
It did not enter `parseInternal` or acquire an incremental arena.

The primary receipt recorded these counters:

- `new_nodes=0`
- `parse_ns=0`
- `arena_inc_acquire=0`
- `arena_inc_new=0`

The route reused the old tree arena. The P25ac arena tests and locked-C
recovery parity gate passed. The temporary candidate was fully reverted.

### P25ad arena trace

Canopy traced the actual incremental path as follows:

```text
ParseIncrementalProfiled
  -> parseIncrementalChangedProfiled
  -> parseIncrementalInternal
  -> parseInternal
  -> acquireNodeArena
  -> incrementalArenaPool.acquire
```

`nodeArena.Release` resets the primary array and overflow slabs before it
returns the arena to the eight-entry incremental pool. `DrainArenaPools`
clears that pool. `recordIncrementalArenaUsage` updates the capacity hint with
12.5% headroom. `ensureParseInitialCapacity` applies that hint before parsing.

The authenticated workload is the Go `recovery_deletion` edit from the
canonical incremental manifest:

| Item | Identity |
|---|---|
| Source SHA-256 | `74c0705f8729670559492fb5460a01b2a1a2a109928e1aeb52736e485e8ff097` |
| Edited SHA-256 | `81543368d0ec807dd951edfdb04fb66ee9ef14ca32b0bb5c53fc8a4d0f2e7b07` |
| Edit | Delete `)` at byte `2076` |
| Forward new nodes | `12,598` |
| Reverse new nodes | `8,301` |
| Forward route | `recovery_incremental` |
| Reverse route | `genuine_incremental_glr` |
| Locked-C result | Fresh and incremental Go and C digests match |

The workload bypasses the leaf fast path because its source length changes.
Its source length also selects the incremental arena class. The existing
profile records forward and reverse parity, accepted roots, stack peaks, and
retry facts. It does not claim a new grammar or corpus lock.

### P25ae bounded screen

The temporary candidate changed `maxRetainedIncrementalNodeBytes` from 256
kibibytes (KiB) to 2 mebibytes (MiB). It changed no retry, recovery, parser,
or test logic. The
candidate diff SHA-256 was
`d2506e03d97cb7161667096c5b2108dbd4f7caa6c29f402ce2c5355120ec1c8f`.

Both worktrees ran one Go grammar in Docker. The limits were one CPU, 4 GiB,
512 process IDs (PIDs), `GOMEMLIMIT=3GiB`, `GOMAXPROCS=1`, `GOFLAGS=-p=1`,
`-parallel=1`, `-count=1`, and `-benchtime=750ms`.
The six-seed screen used seeds 1 through 6. The locked-C gate ran before the
performance screen.

The pooled six-seed result follows. The table reports bytes per operation
(B/op) and allocations per operation (allocations/op):

| Metric | 256 KiB | 2 MiB | Change |
|---|---:|---:|---:|
| Time | 4.182 ms | 3.660 ms | −12.49%, p=0.002 |
| B/op | 1297.98 KiB | 44.03 KiB | −96.61%, p=0.002 |
| Allocations/op | 61.00 | 60.00 | −1.64%, p=0.002 |
| Arena acquire/op | 1.500 | 1.502 | unchanged |
| Arena new/op | 0 | 0 | unchanged |
| New nodes/op | 9,355 | 9,362 | unchanged |
| Retry attempts/op | 0.5000 | 0.5021 | unchanged |
| Maximum stacks | 18 | 18 | unchanged |
| Memory-budget stops/op | 0 | 0 | unchanged |

The pooled one-seed maximum resident set size (RSS) was `234560 KiB` for 256 KiB and
`234880 KiB` for 2 MiB. The change was `+0.14%`.

The drained six-seed control produced these results:

| Metric | 256 KiB | 2 MiB | Change |
|---|---:|---:|---:|
| Time | 5.419 ms | 5.549 ms | not significant |
| B/op | 2.731 MiB | 2.714 MiB | −0.61% |
| Allocations/op | 114.0 | 113.5 | not significant |
| Arena acquire/op | 1.502 | 1.502 | unchanged |
| Arena new/op | 1.490 | 1.490 | unchanged |
| New nodes/op | 9,361 | 9,360 | unchanged |
| Retry attempts/op | 0.5013 | 0.5013 | unchanged |
| Maximum stacks | 18 | 18 | unchanged |
| Memory-budget stops/op | 0 | 0 | unchanged |

The drained six-seed maximum RSS was `306028 KiB` for 256 KiB and
`328640 KiB` for 2 MiB. The change was `+7.39%`.

The pooled lane met the B/op target. The drained lane did not meet the 5%
allocation target and exceeded the 1% RSS regression limit. The candidate
therefore failed the release gate and was fully reverted.

### Correctness and decision

The focused arena tests passed in both worktrees. The authenticated Go
`recovery_deletion` locked-C parity test passed in both worktrees. No run
timed out or exhausted memory. No production or test change remains.

Do not accept a pooled-only improvement. Reopen this candidate only when the
drained lane also meets these conditions:

- Reduce B/op or allocations by at least 5%.
- Keep target time, primary benchmarks, bytes, allocations, RSS, counters,
  and memory-budget outcomes within 1% of baseline.
- Preserve fresh Go, incremental Go, fresh C, and incremental C digest parity.
- Preserve retry, recovery, stack, and arena ownership telemetry.
- Repeat the focused arena tests and the one-Go-grammar locked-C Docker gate.

### P25ac and P25ad artifact paths

P25ac successful artifact roots:

- `/tmp/gts-p25ac-artifacts/20260823T122843Z-baseline-arena-correctness/metadata.txt` — SHA-256 `5a7b9f6ae577a91851dcec4bab61ca17860d2bbf0a606ef11e92e685fa5899ba`
- `/tmp/gts-p25ac-artifacts/20260823T123328Z-candidate-arena-correctness/metadata.txt` — SHA-256 `766fa4f24f6be5654243be1f853782a36d92dcace61a0179355c4a48eb5fd603`
- `/tmp/gts-p25ac-artifacts/20260823T123222Z-baseline-go-recovery-parity-v3/metadata.txt` — SHA-256 `f601ea6a7caf8390857d67e4cad26dc648140d1e22211c0622e0f1a70c44e581`
- `/tmp/gts-p25ac-artifacts/20260823T123400Z-candidate-go-recovery-parity/metadata.txt` — SHA-256 `14ed67900a4f982ee5a220bbb018ad1e5b201d1bc9bf8967e698ced0edcaba2a`
- `/tmp/gts-p25ac-artifacts/baseline-pool-six.txt` — SHA-256 `1fd1501f63904c7ccd8b66a6685ab4ec0bfacbab93852ba6c77c14ba900bd431`
- `/tmp/gts-p25ac-artifacts/candidate-pool-six.txt` — SHA-256 `136d5c6a096a140243cf8ff6b516ec358d8b06d3220c08191d734e1f87c2f3d7`
- `/tmp/gts-p25ac-artifacts/baseline-drained-six.txt` — SHA-256 `b44398da034b11614d7468b421d3fc4f33c1b3de0247821485e8cf84926bb042`
- `/tmp/gts-p25ac-artifacts/candidate-drained-six.txt` — SHA-256 `7295a8d898f76739ed9907f46dcde19403b7c3789ed9f57309333d4d84f3b4f5`

P25ac supplementary metric and RSS artifacts:

- `/tmp/gts-p25ac-artifacts/baseline-pool-metrics.txt` — SHA-256 `fba67e380897294d686fa90358f9afe9738b04a0c491eab8467c375b31eeaac5`
- `/tmp/gts-p25ac-artifacts/candidate-pool-metrics.txt` — SHA-256 `08b28b06e6f8ac315982e4793bd8fe62ce0fb333180cb0d70ef0bd77873ddd96`
- `/tmp/gts-p25ac-artifacts/baseline-drained-metrics.txt` — SHA-256 `1f585943bfc0102cb280c9722605f5138dc852491dab7b43ce96739c81fc2129`
- `/tmp/gts-p25ac-artifacts/candidate-drained-metrics.txt` — SHA-256 `d01336a6ca19de5ca6f9943783ba230139e41917f0a990d9221ee217313ad388`
- `/tmp/gts-p25ac-artifacts/20260823T123605Z-baseline-six-pool/metadata.txt` — SHA-256 `a08b8247a8a5b8748de96b10db5878e74044276d0423a4e0c8ca9195c042665d`
- `/tmp/gts-p25ac-artifacts/20260823T124015Z-candidate-six-pool/metadata.txt` — SHA-256 `bc719a367d6d6cd32cae0743ef8d0d94525ba4cf287392a7a1bd655bf1be9595`
- `/tmp/gts-p25ac-artifacts/20260823T124428Z-baseline-six-drained/metadata.txt` — SHA-256 `1eea46406b178fdea6fde4b3a986f99a936e32b3b0f0fca33f7d162626715c2f`
- `/tmp/gts-p25ac-artifacts/20260823T124832Z-candidate-six-drained/metadata.txt` — SHA-256 `3ca96f19792a605a8b411fa8d5b19ee5663d2a57507c3671894c4e5d6c5f9fb9`
- `/tmp/gts-p25ac-artifacts/20260823T125352Z-baseline-rss-pool/metadata.txt` — SHA-256 `ce179124088e44c881f3838944a67880502d264d31b709d9f4d05c38ac211b8c`
- `/tmp/gts-p25ac-artifacts/20260823T125442Z-candidate-rss-pool/metadata.txt` — SHA-256 `727ea001f918f6b3f09f6a79107faa1aec7b7d1aa6a9fdb957c0ed33d86957c7`
- `/tmp/gts-p25ac-artifacts/20260823T125529Z-baseline-rss-drained/metadata.txt` — SHA-256 `f75eb3b4a3b08979be117157f3f43f9e468d7b18817ed56d886e87a23ff2064b`
- `/tmp/gts-p25ac-artifacts/20260823T125616Z-candidate-rss-drained/metadata.txt` — SHA-256 `b69aee0a816c2446c549c3022487243a836ea5894ff78e5fde9ebcdbc68f0c9b`

P25ad structural artifacts:

- `/tmp/p25ad-acquire-reverse.json` — SHA-256 `de3786e1db8e81f10e8663cc653dfe10b92708e7781774353ba5bc38ecc07aa6`
- `/tmp/p25ad-leaf-edit.json` — SHA-256 `ea7603229a482134725ee87825cc63a494e9f626393a1bbd909da529bb93a941`
- `/tmp/p25ad-incremental-calls.json` — SHA-256 `951ad60486fffb485bd6c4b327138b58519602807d399c0179414665141ce327`
- `/tmp/p25ad-capacity-calls.json` — SHA-256 `541b7fd7aa2b0741a093a54d0f7f5f8c81940b2af71c3b6b09db3f844bcd7ab6`
- `/tmp/gts-p25ad-arena-attribution-20260824/cgo_harness/testdata/canonical_incremental_before_state_receipt_v1.json` — SHA-256 `281cc43c515dbccd0c8eeb2900c68426a85761c3825cf85c2e8abff69d02aa0d`

P25ae durable aggregate and Docker run artifacts:

The six-seed benchmark stdout files were written inside the containers at
the paths below. Container cleanup removed those files. The durable aggregate
source is `p25ae-results.txt`; do not treat the P25ac benchmark files above as
P25ae raw evidence.

- Pooled baseline stdout: `/workspace/cgo_harness/p25ae-baseline-six.txt`
- Pooled candidate stdout: `/workspace/cgo_harness/p25ae-candidate-six.txt`
- Drained baseline stdout: `/workspace/cgo_harness/p25ae-baseline-drained-six.txt`
- Drained candidate stdout: `/workspace/cgo_harness/p25ae-candidate-drained-six.txt`

The host-side raw Docker logs have these paths and hashes:

- Pooled baseline log: `/tmp/gts-p25ae-artifacts/20260823T131639Z-baseline-recovery-six/container.log` — SHA-256 `7a56f08a69b23d6d45ffdb442efb821da6b3a0447df2b17c165d5e71db246bc5`
- Pooled candidate log: `/tmp/gts-p25ae-artifacts/20260823T131709Z-candidate-recovery-six/container.log` — SHA-256 `c2bd45e95a2499aaba8bfee2cb0853e18ac6a927cb5acf16bfbe8ee80e192384`
- Drained baseline log: `/tmp/gts-p25ae-artifacts/20260823T131758Z-baseline-recovery-drained-six/container.log` — SHA-256 `59ae755984d28384d9911140cc5d3ea6c71659fc8dff68d9a3116aaa6fea178c`
- Drained candidate log: `/tmp/gts-p25ae-artifacts/20260823T131832Z-candidate-recovery-drained-six/container.log` — SHA-256 `a78ad1f9050ae756e46355abb25b67484d67ec4a4c5b9bcf3e7b384f543cbb1f`

The one-seed RSS logs are:

- Baseline RSS log: `/tmp/gts-p25ae-artifacts/20260823T131925Z-baseline-recovery-pooled-rss/container.log` — SHA-256 `f83a72d7afd3c9cb12a806c1bf4165463431a7d3325548f4b30b6a9a015c8b48`
- Candidate RSS log: `/tmp/gts-p25ae-artifacts/20260823T131936Z-candidate-recovery-pooled-rss/container.log` — SHA-256 `5f12e0820c59f4c190232f136129b258a1e1c9d1d7e894e91ac850a11ae8dbd6`

P25ae durable aggregate:

- `/tmp/gts-p25ae-artifacts/20260823T131222Z-baseline-arena-correctness/metadata.txt` — SHA-256 `e269ed046d78bd9c11ce130676feb3661fc1cf229fe1e3793c17bcc3de6fc056`
- `/tmp/gts-p25ae-artifacts/20260823T131239Z-candidate-arena-correctness/metadata.txt` — SHA-256 `e23c43e0b1020f6c1c322a52c4d3f35af4a49fc542657ac28ea18fd07d841127`
- `/tmp/gts-p25ae-artifacts/20260823T131323Z-baseline-go-recovery-parity/metadata.txt` — SHA-256 `4e3bfe20aac3fb381550c8c5e0563dab1dd1abe18a92555dde36070684627f71`
- `/tmp/gts-p25ae-artifacts/20260823T131334Z-candidate-go-recovery-parity/metadata.txt` — SHA-256 `71c01015dd1cd0f693fd1b6c9f76d299c4d7ec5aaaa7bcb5cf46cfd937fe7b7c`
- `/tmp/gts-p25ae-artifacts/20260823T131639Z-baseline-recovery-six/metadata.txt` — SHA-256 `3ea47e8356fde2b5d7e09cb456d4a87f71fb530fa35e933844fc285476895ee6`
- `/tmp/gts-p25ae-artifacts/20260823T131709Z-candidate-recovery-six/metadata.txt` — SHA-256 `383eb5328fc7c897d25b0439a7fc3ca37aa65c2e8db3b05ddaa386b02e6de188`
- `/tmp/gts-p25ae-artifacts/20260823T131758Z-baseline-recovery-drained-six/metadata.txt` — SHA-256 `5e0d0d2c8eee8ea1b7f4bd31dd5cf28681d139b119f19e7bc9e961a6ee454c9c`
- `/tmp/gts-p25ae-artifacts/20260823T131832Z-candidate-recovery-drained-six/metadata.txt` — SHA-256 `a2fdafd0f7de08e7fd3f4b100b4459a637d95d533b0109cce473572949b93ee9`
- `/tmp/gts-p25ae-artifacts/p25ae-results.txt` — SHA-256 `9ede57e007e753d1ecd2d67f9587497578166fa91c99fcfc8d3f7ffb967b3e39`

## Caveats

- This is one run, on one noisy shared host, not a sealed-epoch,
  quiet-host, or enclave receipt. Treat the noise floor as a floor, not a
  ceiling: a quiet or enclave host would very likely show tighter deltas.
- The classifier's function table
  (`cgo_harness/attribution/classify.go`) is the single source of truth for
  every boundary rule in this document. If the two ever disagree, the code
  is authoritative; file a follow-up to reconcile the prose.
- `other` includes Go runtime and GC frames by design (a legitimate,
  expected component), not only classification gaps. The tool reports the
  largest non-runtime unclassified functions separately so a real gap is
  visible rather than silently absorbed.
- Cost-per-event numbers are an average, not a causal attribution (see
  above). Do not use them alone to justify a specific optimization target;
  pair them with the component shares.
