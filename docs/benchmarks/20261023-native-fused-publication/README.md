# Fused native publication traversal

One traversal now owns projection, replay state, and raw byte-coverage validation. It replaces four parallel traversal arrays with one frame stack, removes the second raw-child coverage scan, and constructs full materialization views only for callbacks that need them. The same ownership, source-extent, trivia-gap, scanner, replay, cancellation, and memory checks remain enforced.

The change is retained in the PR runtime. Twenty paired runs on **Linux ARM64 in Docker on the Mac** establish Go and YAML throughput improvements. A separate Linux amd64 confirmation also establishes compact Go and ordinary YAML gains; the initial, inconclusive Linux batch remains recorded below. These comparisons use the frozen `native-record-receipts-v2` runtime, source-equivalent to `362db75a2`, not main or C. Native Darwin and Windows remain unqualified.

| ARM64 workload | Before | After | Parse-time change |
| --- | ---: | ---: | --- |
| Compact Go, full DFA | 17.21 ms | 16.29 ms | −5.35%, p=0.030 |
| Compact Go, fresh 1 MiB | 1.178 s | 1.129 s | −4.12%, p=0.003 |
| Compact Go, query source | 17.72 ms | 16.97 ms | −4.24%, p=0.001 |
| Ordinary YAML, R4 | 383.5 µs | 367.5 µs | −4.17%, p<0.001 |
| Ordinary YAML, 8 KiB EOF witness | 1.434 ms | 1.369 ms | −4.54%, p<0.001 |
| Owned-lexer YAML, R4 | 620.4 µs | 604.7 µs | −2.53%, p=0.001 |
| Owned-lexer YAML, 8 KiB EOF witness | 2.316 ms | 2.258 ms | −2.48%, p<0.001 |

ARM64 LR timing remains inconclusive (p=0.056), as do both edit cases. The legacy YAML R4 control also moves −0.94% (p=0.013); the complete tables retain it. LR allocated bytes rise **0.48% on both architectures**, while allocation counts stay unchanged. Smaller YAML and query-source byte savings are recorded in the full tables. All 10% median ratchets pass; no deterministic counter exception is introduced.

The initial Linux amd64 batch established no significant throughput improvement under substantial host load. After later measurements showed lower variance, one independent confirmation batch was declared for the unchanged source, using new seeds and the same paired protocol. All initial samples remain in the receipts; the confirmation is reported separately rather than pooled with them.

| Linux amd64 confirmation workload | Before | After | Parse-time change |
| --- | ---: | ---: | --- |
| Compact Go, full DFA | 10.040 ms | 9.803 ms | −2.36%, p=0.017 |
| Compact Go, fresh 1 MiB | 848.6 ms | 804.8 ms | −5.16%, p=0.001 |
| Compact Go, query source | 10.97 ms | 10.72 ms | −2.26%, p=0.001 |
| Compact Go, LR source | 115.3 ms | 113.4 ms | −1.63%, p=0.017 |
| Ordinary YAML, R4 | 237.8 µs | 229.9 µs | −3.34%, p=0.024 |
| Ordinary YAML, 8 KiB EOF witness | 881.8 µs | 860.9 µs | −2.37%, p=0.007 |

The primary Go trio, compact edit/no-edit cases, owned YAML, and both legacy YAML controls remain inconclusive in this confirmation. Fresh 1 MiB Go allocations fall 875→848 (−3.09%, p=0.039); other allocation counts stay unchanged. LR bytes again rise 0.48%, while query-source bytes fall 1.03%. All timed work/route metrics remain exact, no-edit allocation stays zero, and all median ratchets pass.

| Deterministic work | Before | After |
| --- | ---: | ---: |
| Go candidate full, R4 public nodes | 893 | 893 |
| Go compact full, 1 MiB public nodes | 593,457 | 593,457 |
| YAML ordinary and owned R4 public nodes | 232 | 232 |
| YAML ordinary and owned EOF public nodes | 925 | 925 |
| No-edit allocated bytes / allocations | 0 / 0 | 0 / 0 |

Every counter, reuse, and routing row across all 206 languages is identical. Correctness and counters ran before timing. Linux amd64 passes 412 invariant sessions / 29,664 edit steps, 1,236 standard locked-C checks, 12 large-Go C comparisons, YAML ownership/EOF probes, 20 memory checks, focused races, and actual 386 execution. Added tests compare fused coverage with the previous algorithm across gaps, overlapping and invalid extents, and root exceptions; they also cover deep frame growth, retained memory, terminal callbacks, cancellation, and retry.

The exact [Cognociente report](https://github.com/odvcencio/gotreesitter/issues/454#issuecomment-6311551850), rechecked as the latest author report on October 23, retains all 12 independent incremental/fresh agreements and eight clean C agreements. The same four malformed-delete C shape failures remain unresolved. This witness does not cover every issue in that report.

All 126 Linux cold-RSS probes pass the 400 bytes/source-byte ceiling. Candidate median RSS changes are Go +0.17%, ordinary YAML +1.04%, and owned YAML +0.10%. The existing warmed forced-legacy Go RSS failure remains a separate hold. ARM64 supplements the Linux gates with the complete core unit suite, publication/reuse units, four Go/YAML invariant sessions, 20 memory checks, and exact Go/YAML ledgers. It does not repeat fleet C parity, races, or RSS.

Both architectures use pinned Go 1.26.6, one CPU, 4 GiB container memory, GOMEMLIMIT=3GiB, no network, and an exclusive workload lock. The repository randomized driver runs 20 explicit shuffle seeds per suite, alternating baseline/candidate order with identical workload order, GOMAXPROCS=1, count=1, 750 ms, and benchmem. Linux retains 300 timing rows per revision; ARM64 retains 240. ARM64 uses the exact same 3,559 baseline and 3,561 candidate Go source hashes through targeted overlays over existing source directories. Both runs verify unchanged source hashes at completion and exit zero without OOM. Containers were removed after audit.

The Linux confirmation adds 300 rows per revision with seeds 217001–217020 for YAML, 218001–218020 for primary Go, and 219001–219020 for compact Go. Before timing it authenticates the source against the completed correctness run and verifies all 206 strict counter receipts. It reuses those source-identical correctness, race, C, and RSS results. Completion hashes remain identical, and the confirmation exits zero without OOM. It does not establish a separate correctness result or a main-based comparison.

[Qualification and all directional results](qualification.json), [complete receipts](receipts.tar.gz), and [source patch](source.patch.gz) preserve source pins, controls, raw samples, counter comparisons, tests, RSS, and separate CPU profiles. Linux tables: [compact Go](benchstat-compact-go.txt), [primary Go](benchstat-go.txt), [YAML](benchstat-yaml.txt). ARM64 tables: [compact Go](benchstat-arm64-compact-go.txt), [YAML](benchstat-arm64-yaml.txt).

Linux confirmation tables: [compact Go](benchstat-confirmation-compact-go.txt), [primary Go](benchstat-confirmation-go.txt), [YAML](benchstat-confirmation-yaml.txt). The archive includes the confirmation protocol, its declared seeds, raw samples, and source checks alongside the initial Linux results.

Separate profiles of the qualified ARM64 binary put accepted publication at 36.21% of Go CPU samples, including 23.13% in the native projection traversal. Reduction construction accounts for 14.94%. On ordinary YAML, native traversal accounts for 17.28% and reduction construction for 14.72%. These cumulative shares overlap and are attribution, not benchmark comparisons. The [Go](arm64-go-cpu-top.txt) and [YAML](arm64-yaml-cpu-top.txt) profiles identify remaining node construction, child-summary reads, and reduction staging costs. The receipts include all overlay backing files and raw profiles; the archive builder checks every source-overlay reference before publication.

The combined runtime still needs main-based throughput/allocation qualification, resolution of existing correctness and warmed-RSS holds, and the remaining platform gates. This commit changes no graduation allowlist and authorizes no merge. The separate [reduction construction experiment](../20261023-reduction-pop-construction/README.md) passed correctness but established no throughput improvement; its patch is archived and not installed.
