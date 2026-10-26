# Native publication receipts without the raw-node buffer

This bucket removes a second node-pointer buffer from native publication. The existing occurrence records now supply the raw source ID and published node directly to the receipt writers. On 20 matched pairs, compact Go full-parse allocation fell from 819,960 to 429,568 B/op (−47.61%), and from 71 to 36 allocations. The LR workload fell from 3,715,744 to 2,675,360 B/op (−28.00%).

The baseline is the [held quiescent-native-publication prototype](../20261023-quiescent-native-publication/README.md), not main. The combined engine remains unpromoted. The [runtime source at `154bac17b`](runtime-source.md) matches this tested snapshot. It remains on the draft branch, with the existing merge and graduation holds.

## Paired results

All comparisons use the repository randomized runner, 20 explicit shuffle seeds, matched case order, alternating revision order, GOMAXPROCS=1, count=1, 750 ms, and benchmem. Seeds are 190001–190020 for YAML, 191001–191020 for the primary Go trio, and 192001–192020 for compact Go. All 300 rows per revision were retained.

| Compact Go workload | Median B/op before → after | Median allocs/op before → after | Median time change |
| --- | ---: | ---: | ---: |
| Full DFA | 819,960 → 429,568 | 71 → 36 | −3.87%, inconclusive |
| LR real file | 3,715,744 → 2,675,360 | 1,808 → 1,807 | −4.08%, p=0.030 |
| Fresh 1 MiB | 300,349,808 → 292,706,672 | 907.5 → 889.5 | −1.06%, inconclusive |
| Query source | 4,681 → 4,681 | 120 → 120 | −1.50%, inconclusive |
| Single-byte edit | 405 → 404.5 | 5 → 5 | −2.65%, inconclusive |
| No edit | 0 → 0 | 0 → 0 | −2.83%, inconclusive |

Unrelated host CPU load was substantial. The allocation reduction is the strongest result; these samples do not establish a general throughput improvement. The primary Go full-parse control moved +7.20% directionally, also inconclusively, with allocations unchanged. YAML allocation counts and bytes were unchanged. Its ordinary EOF timing moved +2.95%, owned EOF +4.71%, ordinary R4 −2.12%, and owned R4 −1.10%; all were inconclusive. Full results, including legacy controls, are in the benchstat files and qualification JSON.

Cold RSS uses 20 additional matched, alternating pairs per fixture, seeds 193001–193020, with one grammar per process. Median RSS per source byte moved from 231.866 to 225.344 for Go (−2.81%), 69.686 to 68.856 for ordinary YAML (−1.19%), and 83.130 to 82.270 for owned YAML (−1.04%). All 120 matched samples and six initial probes passed the 400-byte ceiling. These cold probes do not clear the earlier warmed forced-legacy Go RSS failure.

## Correctness and counters

All 206 before/after language ledgers are identical: no work increase, reuse loss, or route metadata change. Go R4 candidate full nodes remain 893 → 893. Large Go public nodes remain 593,457 → 593,457, tokens remain 425,742, and forced-legacy nodes remain 993,391. Every timed work and route metric is identical across each pair.

The candidate passed 412 invariant sessions (29,664 edit steps, zero no-edit allocations), 1,236 standard locked-C checks, 12 large-Go locked-C checks across warm rounds and routes, 20 memory checks, focused race tests, actual 386 execution, YAML ownership/EOF cases, and publication/receipt adversarial tests. The large-Go digest remains `1464b53d73dd7d3ad9ff2b7e064235a4a68442ec7e1379d4c2c263fc6a9eef6c`.

Receipt tests exercise reversed record order, duplicate pointers, aliases whose symbols changed, foreign arenas, unknown provenance, cancellation, and panic cleanup. A source receipt verifies that the reused focused C binary has identical production and C test sources; only a root adversarial test file differs.

The exact [Cognociente report](https://github.com/odvcencio/gotreesitter/issues/454#issuecomment-6311551850) still passes all 12 independent incremental/fresh comparisons and eight clean C comparisons. The same four malformed-delete C shape failures remain at `/source_file/function_declaration[2]/block[4]/ERROR[1]` (Go one child, C two). Native Windows and Mac were not qualified for this delta.

## Storage diagnosis and next target

An untimed, instrumented census confirms that the removed raw-node buffer occupied 7,637,512 bytes on the 1 MiB Go fixture and 1,037,328 bytes on the LR file. Total native scratch fell from 18,939,977 to 11,302,441 bytes and from 2,369,546 to 1,332,194 bytes respectively. The full-DFA workspace fell below the unchanged 256 KiB retention limit, explaining its warm allocation reduction.

Publication records remain the largest native scratch allocation: 9,511,672 bytes on the large fixture, allocated in roughly 580 separate pages. A bounded page-reservation experiment is the next target. The census is not timing or allocation-comparison evidence. Its first attempt failed because the required shuffle seed was absent; that log is preserved alongside the corrected successful run.

## Remaining holds and reproduction

This delta passes its baseline-relative allocation and cold-RSS ratchets. It does not qualify the combined prototype against main: earlier Go LR and large-file allocation regressions, ordinary YAML throughput gaps, the warmed legacy RSS failure, and the malformed C cases remain. No graduation or merge is claimed.

Apply `source.patch.gz` after the preceding native-publication patch. Its SHA-256 is `2ecbc5b4283d75298a6f2690d6126f42846b57308c05bcf0e81fd8b82b769c4a`. `receipts.tar.gz` includes source hashes, controls, raw successful and failed outputs, per-language counters, timing order, RSS samples, and a verified per-member manifest. Container runs used pinned Go 1.26.6 on linux/amd64, one CPU, 4 GiB memory/swap, GOMEMLIMIT=3GiB, network disabled, and an in-container shared workload lock. Source hashes match before and after every completed run.

- [Machine-readable qualification](qualification.json)
- [Compact Go benchstat](benchstat-compact-go.txt), [primary Go benchstat](benchstat-go.txt), [YAML benchstat](benchstat-yaml.txt)
- [Source patch](source.patch.gz) and [raw receipts](receipts.tar.gz)
