# Combined incremental overhead qualification, 2026-10-22 UTC

Status: the combined runtime is integrated into draft PR #1509. All correctness, counter and relative performance gates pass. The absolute peak-RSS gate fails and the existing merge hold remains. No routing, allowlist, ledger baseline or graduation changes.

## Main to final candidate

The baseline is main `afd16867b666dfec05f3eb28e9eee86036474532`. The candidate combines three changes: return six DFA coordinates instead of copying a full lexer; publish large compact receipt sets in physical arena order; grow large read histories in grouped pages instead of repeatedly copying their prefix. The small history allocation, retained 64 KiB flat prefix, failed probes, wide promotion, budget abstention and ownership lifetime remain intact. The native read header stays 64 bytes, actual 386 stays 44 bytes, and public Node is unchanged. Root Go file count stays 756.

The baseline's 3,541 Go files match authenticated main apart from two benchmark/regression test files shared identically with the candidate. All 3,544 candidate Go files match the integrated PR, including the three read-history files, with no private Cogno helper additions. [Main authentication](main-source-audit.json), [integration audit](integration-audit.json) and full before/after manifests in the archive pin the source closure. The [complete Go patch](combined-go.patch.gz) is relative to that main.

## Counters and correctness before timing

All **412 selected ledger rows are exactly equal** before and after; no work increase, reuse decrease or ledger exception is requested. The same 92 unavailable full rows remain unavailable. [Counter equality](counter-equality.json); complete ledgers are in the archive.

| Route / workload | Tokens before→after | Nodes before→after | Max versions before→after | Reused bytes before→after | Block splices before→after |
| --- | ---: | ---: | ---: | ---: | ---: |
| Go default full | 589→589 | 1922→1922 | 8→8 | 0→0 | 0→0 |
| Go default first edit | 572→572 | 3810→3810 | 12→12 | 1166→1166 | 296→296 |
| Go candidate full | 589→589 | 1251→1251 | 3→3 | 0→0 | 0→0 |
| Go candidate first edit | 374→374 | 1553→1553 | 8→8 | 1784→1784 | 266→266 |
| YAML default full | 107→107 | 288→288 | 1→1 | 0→0 | 0→0 |
| YAML default first edit | 107→107 | 480→480 | 1→1 | 0→0 | 0→0 |

The combined candidate passes **1,236 locked-C fixture checks**, **412 edit sessions / 29,664 edit steps**, zero no-edit allocations in all sessions, **20 memory contracts** and **60 focused root race cases**. Focused internal tests and race checks pass. Native and actual 386 header assertions compile; the 386 binary is not executed on ARM64. Correctness containers finish before performance starts. These passes do not establish peak-RSS qualification.

The [latest Cogno report check](cognociente-latest-check.json) confirms the October 21 [v0.56.0 report](https://github.com/odvcencio/gotreesitter/issues/454#issuecomment-6311551850). Its exact Go fixture has 140,368 bytes, SHA-256 `5434575bb249338937331c3756e10dcec34237d107f523ea315996fb6de0ef8f`. Each variant passes eight clean C leaves and all twelve independent incremental/fresh Go and incremental/fresh C leaves; six selected profiled counter pairs agree across variants. Twelve numeric replacement checks pass on the candidate. Four malformed-delete leaves per variant still fail strict fresh-Go/fresh-C shape comparison at `/source_file/function_declaration[2]/block[4]/ERROR[1]`: Go has one child and C two. These failures stay failures. The small-document verifier experiment remains separate; #1511 is a documentation-only owner counter-approval proposal, with its runtime unpromoted. Windows ARM64, the version-to-version release comparison and the broader EOF/directive battery are not requalified here.

## Twenty matched native ARM64 pairs per language

M1 Max Docker Linux ARM64, Go 1.25.14, GOMAXPROCS=1, count=1 per process, 750 ms, benchmem. The shared randomized runner uses Go seeds 152001..152020 and YAML seeds 153001..153020, identical child order per pair and alternating revision order. All 260 rows per variant, source post-audits and median ratchets pass. Raw compressed outputs and complete [Go](go-benchstat.txt) / [YAML](yaml-benchstat.txt) tables retain every metric and directional result.

| Go workload | Before | After | Result |
| --- | ---: | ---: | --- |
| Generated 3,500 functions | 113.08 ms | 97.15 ms | -14.09%, p=.001 |
| Single-byte edit | 176.1 us | 117.1 us | -33.52%, p<.001 |
| No edit | 6.218 ns | 6.183 ns | inconclusive, p=.857 |
| Generated 500 functions | 13.52 ms | 13.54 ms | inconclusive, p=.583 |
| Human-authored query_compile | 16.55 ms | 14.32 ms | inconclusive, p=.659 |
| Human-authored grammargen_lr | 144.9 ms | 149.7 ms | inconclusive, p=.114 |
| Report-shaped 1 MiB | 1.051 s | 1.017 s | inconclusive, p=.925 |

| Allocation metric | Before | After | Result |
| --- | ---: | ---: | --- |
| grammargen_lr B/op | 1,079.5 KiB | 567.8 KiB | -47.40%, p<.001 |
| Large generated Go B/op | 3.281 MiB | 2.657 MiB | -19.04%, p<.001 |
| Report-shaped 1 MiB B/op | 344.7 MiB | 339.3 MiB | inconclusive, p=.056 |
| Standard edit B/op | 406 | 401 | -1.23%, p<.001 |
| grammargen_lr allocs/op | 1,706 | 1,709 | +0.18%, p<.001 |
| Large generated Go allocs/op | 74 | 76 | +2.70%, p<.001 |
| Report-shaped 1 MiB allocs/op | 264.5 | 286.5 | +8.32%, inconclusive p=.116 |

Allocation counts remain 5/edit, 37/generated-500 and 0/no-edit. All no-edit samples use zero bytes. These are combined results, not proof that a particular component caused each difference. Real-file Go throughput remains inconclusive; allocated bytes improve at the cost of additional small allocations. Timing variation is broad on the Go workloads, as retained in the full tables.

YAML has **no demonstrated throughput improvement**. Ordinary R4 moves **245.9→250.9 us (+2.03%, p=.049)** and legacy R4 **184.3→188.6 us (+2.33%, p=.005)**. Owned R4 and all three 8 KiB EOF cases are inconclusive. All YAML byte/allocation medians are unchanged and reported work/route metrics match. The small YAML timing regressions stay visible as the tradeoff for the Go wins; every median stays within the 10% ratchet. Ordinary YAML ownership and compact graduation remain blocked.

## Parser-only peak RSS: hold remains

The memory benchmark and a direct-child rusage wrapper compile before measurement. Twenty alternating pairs use seeds 154001..154020, route 0, `BenchmarkFreshMemory/go/1024KiB`, exactly two measured iterations and no manual GC. Source size is 1,048,591 bytes, SHA-256 `76c70b62127085d3c791ba98a24cba9f22bd343e40afb43522bbb5041cede524`. The compiler, Docker client and wrapper parent are outside measured parser RSS. Every sample accepts the complete clean input and has exactly **993,391 nodes, 425,742 tokens and two maximum stacks**; before/after work counters match in all twenty pairs.

Median peak RSS moves **390,512,640→392,632,320 bytes (+0.54%)**, inside the relative 10% ratchet. Maximum peak RSS is **468,299,776→461,205,504 bytes**. The 400-bytes/source-byte ceiling is **419,436,400 bytes**: seven baseline and **eight candidate samples exceed it**. The absolute gate fails; a lower median does not qualify the runtime. [Independent RSS/work audit](rss-work-audit.json). These fixed-iteration measurements do not supply throughput claims. No GC, pool-cap, memory-budget or retention-contract changes are made.

## Reproduction and receipts

The [raw archive](receipts.tar.gz) has SHA-256 `1fdbcdbd38cb1fd5b6b2fe0b566f0438013366624787d1b9f7003f0b30a448bd`, 2,624,177 bytes and 1,065 members. Its internal manifest pins all 1,064 evidence members: controls, full logs, report probes including expected failures, raw outputs, forty RSS records/logs, source manifests and terminal container audits. [Archive pin](receipt-archive.json), [collection audit](collection-audit.json). All sources and the user's Mac checkout remain unchanged; all containers exit normally with no OOM.

Extract the archive into `mac-retrieved/` next to this README, run `python3 audit-pairs.py mac-retrieved/paired-perf/perf go` and the same command with `yaml`, then `python3 verify-receipts.py .`. These scripts check hashes, paired seed/order/work metrics, counters, terminal states and RSS work equality. `benchstat` over the archived before/after files regenerates the tables.

For workload reproduction, start both inputs at the pinned main, decompress/apply `combined-go.patch.gz` to the candidate, and copy its `benchmark_admission_candidate_route_test.go` and `parsercore_phase0_reuse_dependency_test.go` to the baseline. Authenticate the resulting Go maps against archived `before.json`. The archive's `qualify.sh`, `full-gates/{cgo,native}.sh`, `paired-perf/{cgo,perf}.sh`, shared randomized runner, header overlay and locked-C dependency manifest retain the exact commands and bind mounts. The pinned image is `golang@sha256:3b4a11519ad929d1e1d261a12cff056f0c85b735253d7d861346b9c6f8b36437`. Performance uses explicit native `linux/arm64`; C uses emulated `linux/amd64` and supplies correctness evidence only. Correctness runs one grammar per process; performance runs separately under the shared compute lock.

Exact-head CI remains required. PR #1509 stays draft and held on RSS. This receipt does not close Cogno's adoption blocker or graduate Go or YAML.

## RSS follow-up: attribution and an unpromoted experiment

A [profile-backed reservation experiment](rss-attribution-followup.md) identifies the public-node primary array as the largest allocation site. Accepting a smaller learned capacity hint lowers median RSS by 7.35% in a separate twenty-pair native ARM64 run, but 9/20 candidate processes still breach the hard ceiling. The experiment is rejected for promotion; this PR's runtime and original qualification scope are unchanged, and its absolute RSS hold remains.

## Numeric-run proof prototype

A [separate numeric-run experiment](../20261022-numeric-run-projection/README.md) reduces a generated 137 KiB edit session by about 98% on default and compact routes in twenty shuffled pairs. It is not promoted: the primary single-byte edit regresses 9.67%, the targeted block-splice trade is unapproved, and the absolute RSS hold remains. The exact Cogno report is replayed against the prototype with the same four malformed-delete C failures.

## Segmented reservation follow-up

Two [segmented node-reservation prototypes](segmented-reservation-followup.md) pass correctness but remain unpromoted. Separate pieces meet the sampled RSS ceiling while adding 27.6% allocations; sharing metadata reduces that overhead but four of twenty refined runs still breach the RSS ceiling. All 206 refined counter exports remain identical. The runtime and existing hold remain unchanged.
