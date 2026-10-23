# Read-history storage experiments, 2026-10-21/22

Status: V4 passes the component correctness, counter and performance ratchets and is integrated into draft PR #1509. The [combined main-to-final receipt](../20261022-combined-incremental-overhead/README.md) qualifies the composition and retains the failed absolute peak-RSS gate and merge hold. No route, allowlist, ledger baseline or graduation decision changes. The earlier V3 allocation-count failure remains below.

## V4: grouped pages pass the component ratchets

Coallocating suffix pages in pairs reduces allocator calls while preserving the initial 64 KiB flat prefix and existing limits. An added regression rejects storage-size overflow before budget charging; it fails the earlier prototype and passes V4. The full internal test and race suites pass, including the retained-prefix and differential probe-history tests.

Twenty new pairs use Go seeds 150001..150020 and YAML seeds 151001..151020 with the same native platform, shared runner and stable settings as V3. All thirteen time, byte and allocation-count median ratios stay within 10%; all reported work and route metrics match. All timing differences remain inconclusive. In particular, grammargen_lr moves 144.6→146.6 ms (p=.383), large generated Go 112.2→114.1 ms (p=.989), and the report-shaped 1 MiB case 998.5→1006.9 ms (p=.529). The complete [Go](v4-go-benchstat.txt) and [YAML](v4-yaml-benchstat.txt) tables retain every directional change.

| V4 allocation metric | Before | After | Result |
| --- | ---: | ---: | --- |
| grammargen_lr B/op | 1,079.5 KiB | 567.8 KiB | -47.40%, p<.001 |
| Large generated Go B/op | 3.281 MiB | 2.657 MiB | -19.04%, p<.001 |
| Report-shaped 1 MiB B/op | 337.3 MiB | 331.9 MiB | -1.59%, p=.039 |
| grammargen_lr allocs/op | 1,706 | 1,709 | +0.18%, p<.001 |
| Large generated Go allocs/op | 74 | 76 | +2.70%, p<.001 |
| Report-shaped 1 MiB allocs/op | 236.5 | 256.5 | +8.46%, inconclusive p=.097 |

The remaining allocation-count trade stays visible. All YAML byte and allocation medians remain unchanged; no-edit uses zero bytes and allocations in every pair. V4 again preserves all 412 ledger rows and passes 1,236 standard C checks, 29,664 invariant edit steps, twenty memory contracts and 59 focused root race cases. The exact report probes retain twelve D8 leaves per variant, eight clean C leaves per variant and the same four malformed-delete C failures per variant. Candidate numeric checks pass twelve replacement steps. Header assertions compile for native 64-bit and actual 386 layouts; the latter binary is not executed on ARM64.

The [V4 archive](v4-receipts.tar.gz) contains 973 verified members. The [collection audit](v4-collection-audit.json) records component qualification and the outstanding combined/RSS checks. Its original and paired Go manifests authenticate all sources; the user's checkout remains unchanged. Reproduction follows the V3 instructions with the V4 archive's shared benchmark wrapper and [V4 runtime patch](v4-runtime.patch.gz). This is an allocation-byte improvement with additional small allocations, not a throughput or RSS claim.

## Earlier V3: twenty matched pairs per language

Native Linux ARM64 Docker on M1 Max, Go 1.25.14, GOMAXPROCS=1, count=1 per process, 750 ms and benchmem. The shared randomized runner uses Go seeds 147001..147020 and YAML seeds 148001..148020. Every pair has the same child order and alternates revision order. All 260 rows per variant are retained and verified; source manifests and post-run hashes authenticate the inputs independently of the runner's informational metadata.

The baseline is the qualified primitive-DFA source, before ordered publication. The candidate replaces the contiguous read-history suffix with 64 KiB pages, keeps the original small allocation and retained flat prefix, and packs private flags into the existing 64-byte native / 44-byte 386 header. Failed lexer probes, prefix bounds, wide promotion and budget abstention remain intact. The initial prefix remains bounded; suffix growth avoids copying earlier pages. This comparison is separate from the publication improvement in PR #1509.

| Go workload | Before | After | Result |
| --- | ---: | ---: | --- |
| Generated 500 functions | 13.90 ms | 13.80 ms | inconclusive, p=.968 |
| Generated 3,500 functions | 114.3 ms | 113.0 ms | inconclusive, p=.602 |
| Human-authored grammargen_lr | 146.5 ms | 148.0 ms | inconclusive, p=.620 |
| Human-authored query_compile | 14.18 ms | 14.46 ms | inconclusive, p=.512 |
| Single-byte edit | 117.9 us | 116.1 us | inconclusive, p=.108 |
| No edit | 6.258 ns | 6.156 ns | inconclusive, p=.199 |
| Report-shaped 1 MiB | 1.010 s | 1.030 s | inconclusive, p=.265 |

| Allocation metric | Before | After | Result |
| --- | ---: | ---: | --- |
| grammargen_lr B/op | 1,079.5 KiB | 503.8 KiB | -53.33%, p<.001 |
| Generated 3,500 functions B/op | 3.281 MiB | 2.657 MiB | -19.04%, p<.001 |
| Report-shaped 1 MiB B/op | 329.6 MiB | 324.2 MiB | -1.65%, p=.008 |
| grammargen_lr allocs/op | 1,706 | 1,712 | +0.35%, p<.001 |
| Generated 3,500 functions allocs/op | 74 | 79 | +6.76%, p<.001 |
| Report-shaped 1 MiB allocs/op | 230 | 270 | **+17.39%, p<.001; ratchet failure** |

The [complete Go table](v3-go-benchstat.txt) retains every metric and directional change. All six YAML timing differences are inconclusive; their byte and allocation-count medians remain unchanged in the [complete YAML table](v3-yaml-benchstat.txt). No-edit bytes and allocations stay zero in every pair. All reported node, token and route metrics match. V4 above coallocates suffix pages in pairs and passes the component allocation-count ratchet; its results do not retroactively qualify V3.

## Correctness and rejected variants

All 412 selected ledger rows equal the qualified DFA ledger, including the same 92 unavailable full rows. The prototype passes 1,236 standard locked-C checks across 206 grammars/two routes, 412 invariant sessions with 29,664 edit steps and zero no-edit allocations, twenty memory contracts, and 59 focused root race cases. Focused internal tests and race checks pass. A compile-time assertion verifies the actual 386 header size; those 386 tests were compiled, not executed on the ARM64 host. These gates do not qualify peak RSS.

The exact Cognociente fixture has 140,368 bytes and SHA-256 `5434575bb249338937331c3756e10dcec34237d107f523ea315996fb6de0ef8f`. Eight clean C leaves pass per variant, covering both routes and public APIs. All twelve report edit leaves per variant match independent fresh Go; independent incremental/fresh C digests also match. Reused-subtree and new-node counts agree in six profiled pairs between variants. Twelve candidate numeric replacement steps pass C comparisons.

Four malformed-delete leaves per variant still fail the strict Go/C comparison at `/source_file/function_declaration[2]/block[4]/ERROR[1]`: Go has one child and C has two. These eight failures remain failures. The harness now checks each engine's incremental/fresh digest before comparing the two engines, so the existing C gap cannot hide a new D8 failure. The broader Windows ARM64 and EOF/directive battery remains outside this receipt.

V1 failed existing small-budget and wide-promotion tests before timing. V2 passed correctness, but storage review found that page descriptors put a formerly retained 64 KiB flat history over its retention allowance. V3 preserves that flat prefix. Its regression passes the unchanged baseline and V3 and fails V2 with 65,680 retained bytes instead of 65,600. The failed receipt is retained. V3's later allocation-count failure prevents promotion despite its lower byte counts.

## Warm CPU attribution

The [warm profiles](warm-profiles.tar.gz) capture the qualified DFA runtime after parser construction and eleven warm-up parses, with no manual GC. They are attribution evidence, not before/after performance measurements. Go `grammargen_lr` has 31 measured iterations; ordinary and diagnostic-owned YAML have 21,504 and 9,433. All sources and the user's Mac checkout remain unchanged.

On ordinary YAML, external-lexer read-frontier accounting accounts for 8.05% cumulative CPU. On diagnostic-owned YAML, snapshot destination validation accounts for 5.23%, including scanner-contract validation. Go's compact reduction condensation accounts for 8.57%. These cumulative percentages overlap with their callers and must not be added. The adjacent `warm-*-top.txt` and `warm-*-cum.txt` files retain the full attribution. They identify the next investigations without establishing a speedup.

## Reproduction and provenance

Start from the qualified DFA source at `00fbed6a66531a3e2e0f1897d4b85c08d02b4cff` and its [shared baseline harness](../20261021-trusted-small-source-results/baseline-benchmark-harness.patch.gz). The archive pins every original, candidate and benchmark Go file. Its `paired-perf/benchmark-wrapper.go.txt` replaces the same test file in both benchmark inputs; decompress and apply [v3-runtime.patch.gz](v3-runtime.patch.gz) only to the candidate. Use atomic replacements with hardlinked snapshots. No production Go file outside `internal/incr` differs in this comparison.

The [V3 archive](v3-receipts.tar.gz) contains 969 verified members: controls, complete logs, raw benchmark output, source manifests, the canonical ledger, report probes, known C failures, retention regression and terminal container audits. C runs explicitly emulated Linux amd64 on the Mac and supply correctness evidence only. Native gates and timing explicitly use Linux arm64. Executables, source checkouts and C shared objects are excluded. The adjacent archive JSON files pin byte counts and SHA-256; internal manifests pin every member.

The [collection audit](v3-collection-audit.json) records the rejected allocation ratchet. Passing memory-contract tests and reducing B/op do not resolve the existing warm-parser RSS failure. V3 remains rejected; V4 integration and its outstanding RSS hold are recorded in the combined receipt linked above. No ledger exception or compact graduation follows.
