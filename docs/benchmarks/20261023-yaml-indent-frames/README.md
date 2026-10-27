# YAML indentation-frame allocation receipt, 2026-10-23

A single stack stores each YAML indentation type and width together. This replaces two independently growing arrays while preserving the checkpoint wire format, signed values, root sentinel and scanner behavior. The two-file source change starts from main `c5695bc4c3a155cc1af490e18c6a0684c8317945`. It contains no compact-ownership, graph-layout, generated-lexer or held incremental prototype.

Twenty alternating shuffled pairs per suite establish lower allocation counts on all six YAML cases. All nine YAML/Go time differences are nonsignificant. **This is an allocation improvement; no throughput win or compact graduation is claimed.** Ordinary YAML keeps its existing fallback route.

| YAML workload | Allocations before → after | B/op before → after |
| --- | ---: | ---: |
| R4 legacy | 16 → 14 (−12.50%) | 1,440 → 1,432 |
| EOF 8 KiB legacy | 13 → 11 (−15.38%) | 1,264 → 1,256 |
| R4 ordinary | 37 → 34 (−8.11%) | 14,040 → 14,024 |
| EOF 8 KiB ordinary | 34 → 31 (−8.82%) | 45,736 → 45,720 |
| R4 diagnostic-owned | 72 → 70 (−2.78%) | 11,848 → 11,840 |
| EOF 8 KiB diagnostic-owned | 191 → 189 (−1.05%) | 41,048 → 41,040 |

The complete [YAML statistics](benchstat-yaml.txt) retain every timing and allocation direction, including controls. Time medians move −0.60% to +0.32%; none is significant. The rounded throughput metric for owned EOF has p=.049 while its time metric has p=.068; that does not establish a speedup. [Go controls](benchstat-go.txt) show no significant time change, unchanged allocations and zero bytes/allocations on no-edit reparses. Timing and RSS measurements are separate.

## Correctness and work

YAML scanner tests pass natively, under race and on actual 386. Existing signed-checkpoint, independent-history, binding and zero-allocation restore tests pass. The new regression grows and pops 257 signed frames, verifies parent/type/width pairing, restores into an independent payload, mutates the serialized source and checks every output-buffer capacity against the complete-entry prefix contract.

Both Go and YAML pass their two-route 72-step edit sessions: **four sessions / 288 edits**, with zero no-edit allocations. The twelve standard Go/YAML C checks and large YAML document-stream, EOF and malformed-input fail-closed probes pass. Focused root checkpoint tests and twenty memory-budget/retention cases pass in Docker. The scanner is the only affected language implementation; other grammar suites were not rerun.

Both selected Go/YAML ledgers are byte-identical. All **fifty** full/edit/attempt work and reuse comparisons are exact, including the work spent before ordinary fallback. Representative YAML rows:

| Node work | Before | After |
| --- | ---: | ---: |
| Default R4 full | 288 | 288 |
| Default R4 edit | 480 | 480 |
| Ordinary R4 full | 288 | 288 |
| Diagnostic-owned R4 full | 484 | 484 |
| Ordinary EOF 8 KiB full | 1,156 | 1,156 |
| Diagnostic-owned EOF 8 KiB full | 1,936 | 1,936 |

Tokens, maximum versions, reuse bytes and splices remain unchanged. No ledger exception is requested.

## Large-file memory

The 1,049,572-byte YAML fixture uses twenty alternating shuffled cold-process pairs per route, seeds 180001–180020, with all eighty matched probes and four initial probes retained. No forced GC is added. Every probe accepts, covers the complete source and remains below 400 RSS bytes per source byte.

| Route | Median RSS bytes/source byte, before → after | Ratio | Maximum, before → after |
| --- | ---: | ---: | ---: |
| Ordinary fallback | 68.103 → 68.778 | 1.00991 (+0.99%) | 70.070 → 70.511 |
| Diagnostic-owned | 105.474 → 101.778 | 0.96496 (−3.50%) | 108.659 → 110.130 |

Both median ratchets pass. The candidate owned maximum is higher than the baseline maximum; no samples are excluded. These are memory measurements and do not establish a throughput win. Native Windows and Mac were not rerun for this source change. Cognociente's separate Go malformed-delete C mismatch remains open.

## Reproduction

Pinned Docker image: `golang@sha256:116d58cbd88c1297624acc6e967a060012422bacf9930927e23fb719189c6f36`; Go 1.26.6 linux/amd64 on Intel Core Ultra 9 285, one CPU, 4 GiB memory/swap limit and GOMEMLIMIT=3GiB. Randomized timing uses the repository runner, GOMAXPROCS=1, count=1 per explicit seed, 750 ms and benchmem. YAML seeds 178001–178020 and Go seeds 179001–179020 give **180 rows per role**, with paired workload order and alternating role order independently checked. Every median time, byte, allocation and RSS ratchet passes.

The first attempt used unsupported subset build tags, so unrelated untagged package tests could not compile. It produced no benchmark rows. The second attempt removes only those invocation flags and preserves identical runtime source. Both the failure and the repair are retained. Source hashes are unchanged across correctness, timing and RSS runs, and the PR's complete Go source set matches the measured source.

[qualification.json](qualification.json) pins both changed sources, complete measurements, counters and archive members. [receipts.tar.gz](receipts.tar.gz) retains controls, source patch, raw output and all samples. Every regular member was extracted and checked against its SHA-256 manifest.

Archive: 1341684 bytes; 284 regular members; SHA-256 `07a9059543992c8ba6738c75d61d44c6864ef3cc4eba7ce41f64e229fe4fc6f0`.
