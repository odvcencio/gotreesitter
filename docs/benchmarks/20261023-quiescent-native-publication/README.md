# Native publication for quiescent scanners, 2026-10-23

Native publication avoids constructing hidden wrappers and reduces large-Go memory and time, but **this prototype fails Go allocation ratchets and remains unpromoted**. This receipt changes no runtime or graduation allowlist. Its [source patch](source.patch.gz) applies to the frozen 77-file [ordinary-YAML prototype](../20261023-ordinary-yaml-link16/README.md), not directly to current main. The patch includes the earlier narrow value-replay interface; do not apply that separate patch first.

The generic admission gate now permits proven quiescent scanners, including Go, while actual recovery and unsupported inherited or multiple-field projections retain the reference publisher. The stream preserves hidden punctuation fields, hidden extras, unary production ownership and exact alias receipts. A cold field-profile flag prevents unsupported projections from silently losing compact admission. The 65,540-child test exercises public indices beyond the former uint16 limit. Seven production files and three test files change, including two new internal files and no new root production file or engine language-name comparison.

All **206 strict ledger comparisons pass**. Fifty-eight candidate full-parse rows allocate fewer public nodes; every edit, reuse and default-route metric remains exact. Complete changed rows are in [qualification.json](qualification.json).

| Selected public-node work | Before | After |
| --- | ---: | ---: |
| Go candidate full, R4 | 1,251 | 893 |
| Go candidate full, 1 MiB | 903,086 | 593,457 |
| Go forced legacy, 1 MiB | 993,391 | 993,391 |
| Foam candidate full, R4 | 1,299 | 459 |
| Faust candidate full, R4 | 701 | 380 |
| INI candidate full, R4 | 121 | 74 |

The large-Go count falls **34.29%** while twelve independent locked-C comparisons across both roles, both routes and three warm rounds preserve the exact deep-tree digest `1464b53d73dd7d3ad9ff2b7e064235a4a68442ec7e1379d4c2c263fc6a9eef6c`. The benchmark and C probe previously pinned the old node count. Their identical test-only overlays now require a positive count no greater than that baseline, stable across iterations, while preserving token, coverage, route and exact-tree checks. The original assertion failures remain in the archive.

Twenty paired shuffled timing samples per cell give these compact-Go results:

| Workload | Time before → after | Change | B/op change | Allocations before → after |
| --- | ---: | ---: | ---: | ---: |
| Fresh 1 MiB | 837.8 → 809.3 ms | −3.40%, p<.001 | −8.46% | 262 → 896 **fails** |
| Full DFA | 9.221 → 9.827 ms | +6.58%, p<.001 | +90.96% **fails** | 37 → 71 **fails** |
| Grammar LR source | 107.3 → 111.2 ms | +3.57%, p<.001 | +236.15% **fails** | 1,706 → 1,808 |
| Query source | 10.27 → 10.52 ms | +2.48%, p<.001 | +4.09% | 121 → 120 |

Compact edit and no-edit timing changes are nonsignificant, with allocations unchanged at five and zero. All three primary-Go controls are nonsignificant with unchanged allocations. The four native YAML cells improve 1.18–1.97% and drop one allocation / 16 bytes each through the included value-replay change. Legacy YAML controls are nonsignificant. [Compact Go](benchstat-compact-go.txt), [primary Go](benchstat-go.txt), and [YAML](benchstat-yaml.txt) retain all directional changes. This is a mixed result, not a general Go throughput win.

Twenty matched cold-RSS pairs per case, plus six initial safety probes, pass the memory ceilings and median ratchets. Go falls **10.12%**, from 260.19 to 233.85 RSS bytes per source byte; ordinary YAML rises **0.51%**, from 69.36 to 69.71; diagnostic-owned YAML rises **0.18%**, from 83.42 to 83.57. These are cold-process results. The previously documented warmed forced-legacy Go absolute ceiling failure remains unresolved.

Correctness completes **412 invariant sessions / 29,664 edits**, all no-edit allocations zero, and **1,236 standard locked-C checks**. Core, shadow, focused race, actual 386, twenty memory-contract tests, large YAML and exact Go metadata/receipt comparisons pass. The exact latest Cognociente source completes twelve independent incremental/fresh checks and eight clean C comparisons; its four malformed-delete C shape failures remain unchanged. Native Windows and Mac have not qualified this prototype.

Timing uses Go 1.26.6 linux/amd64 on an Intel Core Ultra 9 285, one CPU, GOMAXPROCS=1, count=1 per process, 750 ms and benchmem through the repository randomized runner. YAML seeds 184001–184020, primary Go 188001–188020 and compact Go 189001–189020 provide **300 valid rows per role**, with matching case order and alternating role order independently checked. RSS seeds are 187001–187020.

The original primary-Go suite 185001–185020 and partial compact-Go 186001 run are **invalidated in full** because an outer launcher exited while Docker kept running and another workload acquired the host-owned lock. YAML had already finished before the overlap and remains valid. Replacement runs own the exclusive lock inside Docker and record workload boundaries. The failed resume with conflicting benchmark overlays produced no timing samples. All invalid data, failures, source hashes, reproduction controls and corrected runs remain in [receipts.tar.gz](receipts.tar.gz).

Earlier prototypes exposed real failures in hidden fields, invisible extras, unsupported-field admission, unary parse-state ownership and alias provenance. Their failed tests and source patches remain alongside the final checks. Further work targets the dense raw-node pointer map and paged publication buffers; no future improvement is assumed in this receipt. All main-based throughput, memory and graduation holds remain in force.

Archive: 12160143 bytes; 7168 regular members; SHA-256 `e6e4fa008c02601f0dea3bacbadeeab15311d0ab9143a39303e6ddfc5b9e7bd1`. Every member hash was verified after archive creation.
