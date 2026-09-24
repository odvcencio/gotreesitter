# Compact parser certification: issue 1064

Status on 2026-09-24: **open**. These focused receipts cover JSON and CSV. They do not certify every supported grammar.

The tests use the locked C runtime and compare each deep tree. The comparison includes symbols, child order, ranges, points, fields, and flags.

The source manifest format is one line per source: `<name> <source SHA-256>\n`. The manifest digest is the SHA-256 of those lines.

Peak counts describe live scheduler boundaries. The derivation peak sums exact paths across live headers. Discarded transient forks are outside this measure.

The count tables use these meanings:

- **Exact** means an equal deep tree.
- **Mismatch** means an unequal deep tree.
- **Fallback** means the compact admission route declined and the production parser completed the parse.
- **Skip** means the test skipped a selected input.
- **Error** means the parser call failed. A matching recovery tree counts as exact.

## Locked oracle

Both receipts use the following C oracle:

| Component | Identity |
| --- | --- |
| Binding | `github.com/tree-sitter/go-tree-sitter` `v0.25.0`, commit `adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3` |
| Runtime | Tree-sitter C `0.25.1`, commit `f5afe475deb7c0bae6407fb776c76824f717bb61` |
| Linkage | Static C runtime in the test binary; grammar loaded from a shared object |
| Compiler | Debian `/usr/bin/cc` `12.2.0`, flags `-std=c11 -fPIC -O2 -I .` |
| Source revision | `170ee57a53f2edc9e83dcea7a0f165769a6fe3bc` plus this change |

## JSON focused receipt

The locked JSON grammar uses commit `254c42a6476413b776221e03982ac8ae159eeb72` from `tree-sitter/tree-sitter-json`.

- C grammar artifact: `harness_out/parity_c_ref_cache/linux_amd64/json-cc4c46f8c09599a8.so`.
- C grammar artifact SHA-256: `2d8f8227048bcb50d887c2a8b1d26243236552eafcc241a27c2f14b8890832fa`.
- Go grammar blob SHA-256: `ad656555b6909bc8efbf40eceb2ac98121f7abdf707c829c9dc03ebe6d83bfbf`.
- Source manifest SHA-256: `592806b195730d12401a93e9f8b9e2fe34d1ff2d1f5ed658c7d3dcb7af5807bb`.

| Source | Source SHA-256 | Deep tree SHA-256 | Route |
| --- | --- | --- | --- |
| Smoke | `e8c628edc9968ef0c668f54e0ba2636b35503357eb1aca0ddc828aeace432f67` | `a6a9ea2b18c6a1299d3b6c92150ac65bb6bdb4d3e5471835bf6a8ceabe37e36e` | Compact |
| Nested | `450d651642cac0ced405936ee7f5cedc1c5fda7d711a2de0210b40d31db75199` | `dc9eadbad3129297e497b447a4ab43ba4013d27b609b7918d672f3e8e8d98d55` | Compact |
| Recovery | `39ad37a9ca0506df7fd7b58b2fbf6a255186dda73ce1b67c8b4f92390b45cf6f` | `6ac46497ddfa5b2fe49e302679317a7866fff762792cff4dc9691dc423968585` | Production fallback |
| Edited smoke | `4356524800e99cb1d0f41e9376cc484ece80e8ccef54d6152ea3a7a89a160c33` | `a6a9ea2b18c6a1299d3b6c92150ac65bb6bdb4d3e5471835bf6a8ceabe37e36e` | Incremental reuse |

| Parse | Exact | Mismatch | Fallback | Skip | Error | Compact route | Peak live headers | Peak live derivations |
| --- | ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: |
| Fresh | 3 | 0 | 1 | 0 | 0 | 2 accepted, 1 declined | 1 | 1 |
| Incremental | 1 | 0 | 0 | 0 | 0 | 0 accepted, 0 declined | Not sampled | Not sampled |

The recovery input has a matching error tree. Its compact route declines when recovery begins. The production parser then matches C exactly.

The incremental edit replaces one digit. Go reuses one subtree and nine bytes. The C incremental tree equals the C fresh tree.

No JSON divergence or accepted exception occurred in this focused corpus. The exhaustive-mode JSON smoke parity trio also passed.

## CSV blocker receipt

The locked CSV grammar uses commit `f6bf6e35eb0b95fbadea4bb39cb9709507fcb181` from `amaanq/tree-sitter-csv`.

- C grammar artifact: `harness_out/parity_c_ref_cache/linux_amd64/csv-ca937bb85faa7d14.so`.
- C grammar artifact SHA-256: `323915c8e205b84f1e9601626986cc848e3c3933c89f3cc3f125cedc5d124379`.
- Go grammar blob SHA-256: `53912a51bae53b1470e19e8f8e9401e0b0dbde58cd0e83baf2d0105a2ba50bbd`.
- Source manifest SHA-256: `c16df28d86737383a58de9977f98027eca88efb52a0347a880a24fe8ae728ecf`.

The manifest includes seven end-of-file witnesses, the one-byte source `,`, the four-byte source `a,b,`, and one edited source.

The empty source matches C. Therefore, `,` is a minimum-length mismatch. The reduction probe also checked shorter prefixes and nearby comma forms.

| Parse | Exact | Mismatch | Fallback | Skip | Error | Compact route | Peak live headers | Peak live derivations |
| --- | ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: |
| Fresh, production | 7 | 2 | 0 | 0 | 0 | Disabled | Not sampled | Not sampled |
| Fresh, compact requested | 7 | 2 | 2 | 0 | 0 | 7 accepted, 2 declined | 1 | 1 |
| Incremental deep tree | 1 | 0 | 0 | 0 | 0 | 0 accepted, 0 declined | Not sampled | Not sampled |

Both mismatches first diverge at `/document`: Go `hasError=false`, C `hasError=true`.

For `,`, C emits an `ERROR` node. For `a,b,`, C emits a missing `number_token1` node.

The production and compact-requested parses return clean Go trees. Both compact requests fall back. The strict gate fails on both routes.

The clean edit replaces `2` with `4` in `a,b,c\n1,2,3\n`. Go reuses one subtree and five bytes. The Go and C incremental trees match exactly. The C incremental tree also matches the C fresh tree. The edited source SHA-256 is `107f9bb038f4d1a2d36ab6f26be1e57a89720b38b670a5d3c356b23f43481241`.

CSV remains **uncertified**. The zero-width `number` node is clean in Go. Locked C instead publishes an error or a missing leaf. A prior broad missing-node visibility change failed COBOL, Chatito, and Bitbake parity. Reopen the correction when recovery-candidate provenance distinguishes those cases. Require `GTS_STAGE6_CSV_STRICT=1` and those three parity tests to pass.

The exhaustive-mode CSV smoke parity trio passed. It does not cover the trailing-field mismatch.

## Docker evidence

All listed runs used the `gts-docker.lock` host lock, `GOWORK=off`, one grammar per container, and an 8 GiB memory limit.

| Grammar and gate | Artifact directory under `harness_out/docker` | `container.log` SHA-256 | Result |
| --- | --- | --- | --- |
| JSON deep tree | `20260924T183016Z-1064-verify-json` | `99f120cef6d3ca03b0f8ca95dd8f5acdfe12d506fdf105bb03e5cc55fce55832` | Pass |
| JSON telemetry reset | `20260924T183122Z-1064-verify-telemetry` | `66db685e3fac7d3760c92788f368a5ed3d85c3e923f1b722c77d9fc7280843cb` | Pass |
| JSON parity trio | `20260924T175804Z-1064-json-exhaustive` | `34657b59fa8a09aa2541d0d2074769b7b96efdf602bcdacbf70153f37e1d317b` | Pass |
| CSV reduction probe | `20260924T180742Z-1064-csv-reduction-probe` | `e895c4a2685829a506c96384cb4036f3cd4696192c2555b846ddeeb9d64be4f2` | Found the one-byte mismatch |
| CSV deep trees and incremental edit | `20260924T184306Z-1064-csv-verified` | `49047f57f006a53ef36716d2040bbcb05b1faa743ccf241176ddefb2a66466fa` | Seven exact fresh witnesses, two open mismatches, one exact edit |
| CSV strict gate | `20260924T181039Z-1064-csv-minimal-strict` | `fb75a2ec4567d5542098eaa605c22cee170dcd306af420c2a811b7686a98dd9d` | Expected failure for both sources and routes |
| CSV parity trio | `20260924T180058Z-1064-csv-exhaustive` | `eaa16e0d577c82bb1023714061113f385658634c55ad3b5c1d2962cf983b8eb5` | Pass |
| Swift helper regression | `20260924T181417Z-1064-swift-helper` | `96b1fecacf412b4f4ef51b44e7fa7e65434cf67b67ea32af2cc7c68111599557` | Pass |

The Docker runner removed each test container. The runner kept its logs and metadata outside Git.

To reproduce the JSON receipt, run this command from the repository root:

```sh
GOWORK=off flock /home/draco/.local/state/nightwatch/gts-docker.lock bash cgo_harness/docker/run_parity_in_docker.sh --label 1064-json-recheck --no-build -- 'cd /workspace/cgo_harness && GOWORK=off go test . -tags "treesitter_c_parity gts_parsercorephase0" -run "^TestStage6JSONCompactCertification$" -count=1 -v'
```

Replace the test name with `^Test(CsvEOFAcceptLockedC|CSVTrailingFieldOpenGap|CSVIncrementalLockedC)$` for the CSV receipt.

## Performance comparison

The paired benchmark gate used 20 shuffle seeds, one process per seed, `GOMAXPROCS=1`, `-count=1`, and `-benchtime=750ms`. The baseline is clean main at `170ee57a5`. The candidate includes the opt-in telemetry change with telemetry disabled.

| Benchmark | Main ns/op | Candidate ns/op | Main B/op | Candidate B/op | Allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: |
| Full Go parse | 4,490,000 | 4,557,000 | 1.228 KiB | 1.229 KiB | 8 on both |
| Single-byte incremental edit | 119,000 | 120,000 | 386 | 386 | 5 on both |
| Incremental no edit | 4.123 | 4.127 | 0 | 0 | 0 on both |

`benchstat` found no significant timing or allocation change. Another lane ran a host benchmark during this comparison, so the timing estimates include shared host load. No large-file maximum resident set size was measured for this focused telemetry change.

- Main output: `/tmp/gts1064-main-bench.txt`, SHA-256 `11f42b593641d54000555950f4f6515213127faa6cfad4dc8abf04c22aaea43a`.
- Candidate output: `/tmp/gts1064-head-bench.txt`, SHA-256 `fe94a6578af8fb9af9351745502fa6e3cb4d01aafebb0aa405260467bd381977`.

## Remaining gate

Issue 1064 remains open. JSON needs a wider locked corpus. The remaining supported grammars need complete receipts.

CSV needs exact corrections for both comma witnesses.
