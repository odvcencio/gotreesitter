# Ordered compact receipt publication, 2026-10-21


## Integrated adaptive variant

The PR now contains the qualified adaptive publication change. Publications with at most 64 candidates keep the original direct map path; larger publications seed the highest valid arena slot and publish the rest in physical slot order. This bounds copying on large fresh sparse stores without scanning an entire arena for a small publication. No buffer, Node field, language-name check, GC setting, pool cap, memory budget, route or allowlist changes.

Twenty new matched native Linux ARM64 pairs compare the qualified primitive-DFA source with this adaptive change. Go seeds are 144001..144020 and YAML seeds 145001..145020. The comparison is separate from the primitive-DFA/D64 measurements. All thirteen timing, byte and allocation-count median ratios stay inside the 10% ratchet. The no-edit path retains zero bytes and allocations.

| Go compact workload | Before | After | Result |
| --- | ---: | ---: | --- |
| Generated 3,500 functions | 110.30 ms | 96.19 ms | -12.79%, p<.001 |
| Generated 500 functions | 13.58 ms | 13.28 ms | -2.19%, p=.049 |
| Single-byte edit | 116.3 us | 115.8 us | inconclusive, p=.314 |
| No edit | 6.158 ns | 6.146 ns | inconclusive, p=.380 |
| Report-shaped 1 MiB | 978.7 ms | 988.9 ms | inconclusive, p=.081 |
| Human-authored query_compile | 13.79 ms | 13.96 ms | inconclusive, p=.355 |
| Human-authored grammargen_lr | 143.8 ms | 144.6 ms | inconclusive, p=.253 |

All six YAML timing differences remain inconclusive. Their complete values, directional changes and allocation metrics appear in [v3-yaml-benchstat.txt](v3-yaml-benchstat.txt); the corresponding Go table is [v3-go-benchstat.txt](v3-go-benchstat.txt). These results do not establish a human-authored Go or YAML throughput win. The primitive edit can bypass publication, so the sample does not establish publication ordering as the cause of the earlier small-edit difference.

The adaptive runtime preserves all 412 ledger rows and eighteen receipt digests. It passes 1,236 locked-C checks (206 grammars, both routes), 412 native ARM64 edit sessions / 29,664 steps with zero no-edit allocations, twenty memory contracts and 27 focused race cases. The overflow regression now uses 65 candidates so it exercises the ordered branch. All three integrated Go files match the qualified paired-source hashes. The adjacent v3 audits and archives retain source identities, controls and complete logs.

The existing parser-only warm RSS failure remains unresolved. Keep this PR draft and do not merge or graduate from this receipt. The earlier variant, its edit regression and its failed predecessor remain below as experiment evidence.

## Earlier measured variant, not integrated

Status: qualified prototype archived for review. The runtime patch is archived for review; this receipt does not add it to the engine, graduate a language, change ordinary YAML routing, or change a ledger baseline. Keep the existing primitive-DFA PR draft and its RSS hold. A small-publication fast path is the next experiment because this prototype makes the measured edit slightly slower.

The comparison starts from the qualified primitive-DFA source in PR #1509, before the publication patch. Its fresh-parser runtime is D64. The candidate changes only `publishCompactReuseDependencies`: it seeds the highest valid arena slot, then publishes remaining candidates in physical slot order. It preserves the full dependency range for density decisions and avoids random insertion into a sorted sparse receipt array. No buffer, Node field, GC setting, pool cap, or memory budget changes.

## Measured throughput and allocation limits

Twenty matched alternating pairs per grammar use the repository randomized runner, `GOMAXPROCS=1`, count 1, 750 ms and benchmem. The host is M1 Max, explicitly native Linux arm64 Docker, Go 1.25.14, CPUs 2,3, 4 GiB memory and a 3 GiB Go soft limit. Correctness ran separately. The runner preserves identical child order for each pair; Go seeds are 142001..142020 and YAML seeds 143001..143020.

| Go compact workload | Before | After | Result |
| --- | ---: | ---: | --- |
| Generated 3,500 functions, 141,294 bytes | 112.59 ms | 97.05 ms | -13.81%, p<.001 |
| Generated 500 functions, 19,294 bytes | 13.89 ms | 13.60 ms | inconclusive, p=.142 |
| Single-byte edit | 116.3 us | 118.4 us | +1.82%, p=.044 |
| No edit | 6.178 ns | 6.172 ns | inconclusive, p=.867 |
| Report-shaped 1 MiB | 1.010 s | 1.004 s | inconclusive, p=.947 |
| Human-authored query_compile | 14.12 ms | 14.14 ms | inconclusive, p=.968 |
| Human-authored grammargen_lr | 145.3 ms | 145.7 ms | inconclusive, p=.565 |

The edit regression remains visible. This is a measured large-generated-file win, not a general real-file speed claim or a fix for Cogno's small-document verifier report. The exact Cogno fixture has no retained receipts through this path in the geometry probe.

| YAML workload | Before | After | Result |
| --- | ---: | ---: | --- |
| R4 legacy | 204.6 us | 199.6 us | inconclusive, p=.461 |
| R4 ordinary | 273.0 us | 270.6 us | inconclusive, p=.478 |
| R4 diagnostic owned | 550.3 us | 543.4 us | inconclusive, p=.201 |
| EOF 8 KiB legacy | 741.2 us | 744.0 us | inconclusive, p=.718 |
| EOF 8 KiB ordinary | 855.8 us | 866.1 us | inconclusive, p=.925 |
| EOF 8 KiB diagnostic owned | 2.102 ms | 2.098 ms | inconclusive, p=.678 |

All thirteen timing median ratios remain within the 10% ratchet. No-edit retains zero bytes and allocations in all twenty pairs. The 1 MiB Go allocation median moves 237 to 238 with substantial calibration/pool variation; the raw allocation result is retained in the complete tables. Other allocation-count medians are unchanged. Small significant B/op rounding differences are not separate allocation wins. All public node, token and route metrics present in benchmark output agree in every matched pair. The complete [Go](go-benchstat.txt) and [YAML](yaml-benchstat.txt) tables retain every metric, including directional regressions.

## Deterministic work and correctness

All 412 selected ledger rows equal the qualified baseline byte-for-byte as JSON values. They include the same 92 declined/unavailable full rows; unavailable values are not zeroes. The full candidate ledger is inside `receipt-geometry.tar.gz` and equals the [primitive-DFA candidate ledger](../20261021-primitive-dfa-snapshots/candidate-ledger.json.gz). No owner counter exception is requested for this prototype.

The diagnostic overlay compares eighteen fresh compact parses per variant across six Go fixtures. Every returned runtime record and coordinate-authenticated receipt SHA-256 matches. Retained receipt capacity is unchanged. At 500 functions, sparse insertion copying falls from a 17,956,800-byte median to 48,000 bytes while retaining 65,536 bytes. At 3,500 functions it falls from 880,501,504 to 336,000 bytes while retaining 524,288 bytes. Cogno's exact fixture, the 1 MiB report shape, query_compile and grammargen_lr record no receipts through this path; no copy reduction is claimed for them. Instrumented geometry is not timing evidence.

The candidate passes 1,236 locked-C fixture checks: 206 languages, both routes, fresh structure, incremental structure and clean error status. The C gate runs Linux amd64 under emulation on the Mac and supplies correctness evidence only. All 218 supplied cached C shared objects and dependency inputs retain their original hashes. The cache key includes compiler identity, grammar lock and patches. No language is skipped.

An independent native Linux arm64 gate passes 412 canonical edit sessions, 29,664 steps, and zero no-edit allocations in every session; twenty memory contracts; and 26 receipt-focused race cases. Before/after focused units pass, and the final benchmark inputs pass another 27 units each, including the overflow regression. These checks do not resolve the existing large-file RSS failure or establish compact graduation.

The first private prototype treated the arena's global used-node count as the primary slab's length. The warmed 3,500-function test caught an index panic. Its patch and failed receipt are retained, with no timing result. The corrected prototype bounds the primary scan by its slice length and visits each overflow slab's used prefix. The added synthetic regression passes the unchanged baseline and corrected candidate and makes that rejected prototype fail specifically with the same bounds panic.

## Reproduction and provenance

Start with PR #1509's primitive-DFA source at `00fbed6a66531a3e2e0f1897d4b85c08d02b4cff`. Apply the existing [baseline benchmark harness](../20261021-trusted-small-source-results/baseline-benchmark-harness.patch.gz) and the two full test-file replacements `benchmark-wrapper.go.txt` and `overflow-regression.go.txt` to both inputs. Apply `runtime.patch` only to the candidate. Replace files atomically when using hardlinked snapshots. The full before manifest inside `paired-receipts.tar.gz` pins all 3,542 Go files in the original, baseline and candidate closures. Only the publication runtime file differs between the paired inputs. All sources and the user's Mac checkout remain unchanged after execution.

`paired-receipts.tar.gz` contains the unmodified shared runner, launch controls, raw output, complete original benchstat inputs, source manifests, regression output and terminal container audit. The shortened readable benchstat labels/padding above change no values. The geometry, C and invariant archives retain their separate commands, source pins and logs; their adjacent archive JSON pins bytes and SHA-256. Private commit logs, executables and the C shared objects are excluded.

Next work addresses the existing Go RSS ceiling failure and tests a fast path for small publications to avoid the measured edit slowdown. This receipt neither weakens correctness nor authorizes the separate trusted-small-source counter trade.
