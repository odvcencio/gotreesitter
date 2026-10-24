# Bounded native-publication page reservation

Reserving publication pages in a bounded block reduces the 1 MiB Go fixture from **888 to 345 allocations per parse (−61.15%)**. The LR file drops from 1,807 to 1,759 allocations and from 2,675,360 to 2,576,800 B/op (−3.68%). None of the timing comparisons establishes a throughput improvement.

The baseline is the [native-record runtime](../20261023-native-record-receipts/runtime-source.md) installed at `154bac17b`, not main. This bucket preserves a separate four-file experiment; its source patch is not installed in the PR runtime. It changes neither language graduation nor the existing merge holds.

After at least 16,384 publication records and one sixteenth of the input, a one-shot estimate can reserve remaining pages together. The estimate has 6.25% headroom and a 24 MiB total page limit. Existing record addresses remain stable. Exact page and pointer-table growth is checked against both memory limits before allocation; refusal keeps normal paging. Workspace retention stays capped at 256 KiB.

| Compact Go workload | Median allocations before → after | Median B/op before → after | Median time change |
| --- | ---: | ---: | ---: |
| Fresh 1 MiB | 888 → 345 | 292,619,664 → 292,422,032 | −6.13%, inconclusive |
| LR real file | 1,807 → 1,759 | 2,675,360 → 2,576,800 | −4.74%, inconclusive |
| Full DFA | 36 → 36 | 429,572.5 → 429,571.5 | −3.63%, inconclusive |
| Query source | 120 → 120 | 4,681 → 4,681 | −2.73%, inconclusive |
| Single-byte edit | 5 → 5 | 404 → 404 | −0.79%, inconclusive |
| No edit | 0 → 0 | 0 → 0 | −2.06%, inconclusive |

Primary Go and all YAML timing changes are also inconclusive. Ordinary YAML R4 moves +1.32%, ordinary EOF +0.54%, owned R4 +0.92%, and owned EOF −0.21%. YAML allocation counts are unchanged. All directional results are retained in the [qualification JSON](qualification.json) and benchstat files; no median exceeds its 10% ratchet. Host timing variance remains substantial.

All **206 strict counter comparisons are identical**, including routing and reuse. R4 Go candidate full nodes stay 893 → 893; large Go public nodes stay 593,457 → 593,457. YAML ordinary and owned R4 stay 232 → 232; EOF stays 925 → 925. All timed work vectors also match.

Correctness passes 412 invariant sessions / 29,664 edit steps with zero no-edit allocations, 1,236 standard locked-C checks, 12 large-Go C comparisons, 20 memory checks, focused races, actual 386 execution, and YAML ownership/EOF probes. Page tests cover stable addresses, clearing all reserved slots, zero warm allocation, exact growth accounting, overflow/limit refusal, both runtime memory limits, and retention release. The exact Cognociente witness passes 12 independent incremental/fresh checks and eight clean C comparisons; four previously recorded malformed-delete C shape failures remain unchanged.

Twenty matched cold-RSS pairs per fixture plus six initial probes all pass the 400-byte/source-byte ceiling. Median RSS changes are Go −0.62%, ordinary YAML +0.83%, and owned YAML +0.18%. These cold results do not clear the existing warmed forced-legacy Go RSS failure.

Timing uses the repository randomized runner, 20 explicit seeds per suite, identical case order within each pair, alternating revision order, GOMAXPROCS=1, count=1, 750 ms and benchmem. YAML seeds are 195001–195020, primary Go 196001–196020, compact Go 197001–197020; RSS uses 201001–201020. All 300 timing rows per revision and all 126 RSS processes are retained. Runs use pinned Go 1.26.6 linux/amd64 on an Intel Core Ultra 9 285, one CPU, 4 GiB memory/swap, GOMEMLIMIT=3GiB, disabled networking, and the shared in-container workload lock. Source hashes are unchanged, exits are zero, and Docker reports no OOM.

The combined runtime still lacks main-based throughput/allocation qualification. Ordinary YAML throughput, the earlier Go allocation regressions, warmed legacy RSS, and the four malformed C cases remain open. Native Windows and Mac have not qualified this delta.

Apply [source.patch.gz](source.patch.gz) to the native-record runtime. Patch SHA-256: `80dbdb4a8fca4d18c8de48ab92fa67fb7e8d3451d017531f9c486b93ba4c1899`. [receipts.tar.gz](receipts.tar.gz) retains reproducible controls, raw logs, counters, source hashes and a verified per-member manifest: 2,472 files, 2,981,436 bytes, SHA-256 `67ccb3096ff11c4c938ff179d5beffed2fcbd2b62924d01c297b06b1b64d671e`.

[Compact Go benchstat](benchstat-compact-go.txt), [primary Go benchstat](benchstat-go.txt), [YAML benchstat](benchstat-yaml.txt).
