Legacy unit election receipts, 2026-09-30

Convergent legacy unit reductions now elect their children before head link union. Python head merges fall from 9 to 0 and over-merge sources from 3 to 0. The locked C runtime still records 2 Python merges and 150 merges across 108 constructed sources. The repaired Perl witness `{(p())if@;}` matches the canonical fresh C tree exactly.

The repair applies to a flat primary and a GSS sibling with the same linear prefix, exact one-child raw parents of the same symbol and production, and zero dynamic version scores. It replaces an incumbent only when C's parent election selects the candidate. It preserves the existing conflict dispatch and version budgets. Multi-child reductions and parallel histories require additional pop-slice provenance and retain their existing behavior.

The main baseline is `b8a3e2a8`; the starting link-union branch is `d4e9416b`; the repair is `c1445bd7`. All comparisons use the same grammar artifacts and canonical fresh C trees. The corpus lock digest was verified as `41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`; the lock itself is external to the repository.

| Measurement | Main | Starting branch | Repaired branch | Locked C |
| --- | ---: | ---: | ---: | ---: |
| Python head merges | 0 | 9 | 0 | 2 |
| Constructed head merges | 5 | 14 | 4 | 150 |
| Over-merge sources | 0 | 3 | 0 | 0 |
| Source count | 108 | 108 | 108 | 108 |
| Exact fresh receipt matches, eight task languages | 18/32 | 21/32 | 22/32 | 32/32 |

The repair preserves every exact receipt match from the starting branch and gains Dart's `_url_web.dart` receipt. A further 36 paired fresh checks across 18 languages lose no exact matches. Existing mismatches remain, including the Apex bind-variable query witness; its deep tree digest is unchanged by the repair even though its head-merge count falls from 1 to 0. Head-merge counts exclude convergent-pop child elections, as the C instrumentation does.

The census pin refresh records Apex merges 1 to 0, no-head refusals 53 to 24, shape refusals 53 to 12, and sources where C merges and Go does not 5 to 6; Perl no-head refusals 46 to 34; Python no-head refusals 5 to 0; and the aggregate 5 to 4. Ada's shallow-would-accept count 0 to 4 was already present on the starting link-union branch and comes from the deeper comparison refusing distinct completed payloads. C counts and the zero-over-merge gate remain the reference measurements.

The 412-row deterministic counter ledger passes with its existing pins and 2% tolerance. The election leaves the task's C# and Meson work counters unchanged from the starting branch:

| Counter | Main | Starting branch | Repaired branch |
| --- | ---: | ---: | ---: |
| C# full/edit peak versions | 23/23 | 21/21 | 21/21 |
| C# full/edit new nodes | 1936/2078 | 1962/2264 | 1962/2264 |
| Meson full tokens | 487 | 487 | 487 |
| Meson edit tokens | 365 | 703 | 703 |
| Meson edit reused bytes | 309 | 0 | 0 |
| Meson edit block splices | 122 | 271 | 271 |

The Meson reuse loss is inherited from the link-union branch. The new election adds no further loss on that fixture. It preserves incremental/fresh equality and zero no-edit allocations on `y('',e,d:e)` with a leading-byte insertion. That source still has a pre-existing fresh C mismatch (`variableunit` versus `identifier`).

Other counter movements from the starting branch are recorded below. These are directional changes, including changes below the gate tolerance; the ledger pins were not reset.

| Language/route | Phase | Counter changes |
| --- | --- | --- |
| agda/default | full | `tokens` 682 → 692; `new_nodes` 3297 → 3223; `multi_version_token_share_ppm` 57184 → 50578 |
| agda/default | edit 1 | `tokens` 683 → 693; `new_nodes` 3763 → 3657; `multi_version_token_share_ppm` 57101 → 50505 |
| elixir/default | full | `new_nodes` 1627 → 1119; `max_live_versions` 8 → 5; `multi_version_token_share_ppm` 428969 → 116991 |
| fsharp/default | full | `new_nodes` 9726 → 9777; `multi_version_token_share_ppm` 859683 → 863636 |
| fsharp/default | edit 1 | `new_nodes` 9786 → 9837; `multi_version_token_share_ppm` 859683 → 863636 |
| haskell/default | full | `tokens` 475 → 476; `new_nodes` 1274 → 1275; `multi_version_token_share_ppm` 71578 → 71428 |
| haskell/default | edit 1 | `tokens` 475 → 476; `new_nodes` 1334 → 1335; `multi_version_token_share_ppm` 71578 → 71428 |
| html/candidate | edit 1 | `tokens` 115 → 117; `new_nodes` 367 → 370; `reused_bytes` 749 → 742; `block_splices` 336 → 337 |
| html/default | edit 1 | `tokens` 115 → 117; `new_nodes` 367 → 370; `reused_bytes` 749 → 742; `block_splices` 336 → 337 |
| markdown/default | full | `new_nodes` 3416 → 1859; `max_live_versions` 4 → 2; `multi_version_token_share_ppm` 900246 → 105911 |
| php/default | full | `new_nodes` 363 → 304; `max_live_versions` 8 → 4; `multi_version_token_share_ppm` 318681 → 87912 |
| php/default | edit 1 | `new_nodes` 363 → 330; `max_live_versions` 8 → 4; `multi_version_token_share_ppm` 318681 → 87912 |
| verilog/default | full | `new_nodes` 3541 → 3533 |

HTML's first edit reuses 7 fewer bytes (749 to 742), scans 2 more tokens, and builds 3 more nodes. Agda scans 10 more tokens while building 74 fewer full-parse nodes and 106 fewer edit nodes. F# builds 51 more nodes, and Haskell scans one more token and builds one more node. All changes remain within the existing ledger's guarded tolerance. Elixir, Markdown, and PHP substantially reduce full-parse nodes and peak versions.

The broad incremental gate ran one language per process in Docker: 197 of 206 passed; Awk, JavaScript, and Twig failed assertions; Elsa, Godot Resource, Haskell, Kotlin, Nickel, and PowerShell timed out in tree editing. The same nine failures occurred in the earlier repair experiment. All six timeouts also reproduce on main; the three assertion failures were reproduced on main separately. The five pinned probe-ledger suites (JavaScript, TOML, Python, Go, TypeScript) pass. Focused Docker GSS/recovery, hot-layout and memory-budget tests pass. The narrow runtime-profile check, root `go vet ./...`, and the 756-file/12-tag API guardrails pass. The additional tagged C-harness vet fails on test-only-type and five `sync.Once` copy warnings; main produces identical diagnostics.

Randomized timing receipts follow. Each campaign uses `scripts/run_randomized_benchmarks.sh`, one process per explicit seed 1–20, `GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`, and `-benchmem`. Inputs are authenticated committed samples from `internal/benchfixtures/real_corpus.json`; edits use the first `benchfixtures.EditingSession` step. Timing excludes the old-tree setup and release for edit benchmarks and includes `Tree.Edit`, reuse selection, and rebuilding. Before/after comparisons use `benchstat`. The eight task-language main campaigns and repaired campaigns ran separately; additional changed languages and the Go trio alternate paired runs. Timing has substantial variance, including movement on the unchanged no-edit path. Allocation counts provide a steadier attribution check.


Twenty-seed median metrics (ns/op, B/op, allocs/op). Raw seed records and benchstat results are in [the receipt directory](receipts/legacy-unit-election/).

| Language | Benchmark | ns/op main → repaired | B/op main → repaired | allocs/op main → repaired |
| --- | --- | ---: | ---: | ---: |
| bash | full | 349,355.5 → 418,552 | 1,769 → 1,769 | 13 → 13 |
| bash | edit | 368,952 → 438,602 | 1,832 → 1,832 | 15 → 15 |
| bash | no_edit | 10.34 → 13.07 | 0 → 0 | 0 → 0 |
| c_sharp | full | 8,086,020.5 → 12,008,496.5 | 1,103,154 → 1,438,679.5 | 722 → 3,147 |
| c_sharp | edit | 8,237,585.5 → 12,253,620.5 | 1,103,484.5 → 1,440,622 | 726 → 3,162 |
| c_sharp | no_edit | 12.46 → 13.77 | 0 → 0 | 0 → 0 |
| d | full | 473,181.5 → 513,348.5 | 1,258 → 1,258 | 9 → 9 |
| d | edit | 472,432.5 → 521,280.5 | 1,329 → 1,329 | 14 → 14 |
| d | no_edit | 12.88 → 13.7 | 0 → 0 | 0 → 0 |
| dart | full | 17,556,968.5 → 17,696,933.5 | 45,117.5 → 45,622 | 200 → 344 |
| dart | edit | 27,451,264.5 → 28,489,651.5 | 928,713.5 → 924,463.5 | 447 → 493 |
| dart | no_edit | 12.61 → 13.79 | 0 → 0 | 0 → 0 |
| solidity | full | 2,569,977 → 2,922,081.5 | 6,585 → 89,318 | 21 → 802 |
| solidity | edit | 5,259,822 → 6,272,899.5 | 20,818 → 193,758 | 74 → 1,642 |
| solidity | no_edit | 13.59 → 13.22 | 0 → 0 | 0 → 0 |
| meson | full | 4,974,666.5 → 5,882,956.5 | 6,343 → 441,298 | 22 → 3,221 |
| meson | edit | 5,889,830.5 → 11,094,688 | 856,681.5 → 771,806.5 | 32 → 5,637 |
| meson | no_edit | 12.84 → 13.68 | 0 → 0 | 0 → 0 |
| python | full | 3,078,003.5 → 3,357,243.5 | 8,500 → 13,654 | 41 → 85 |
| python | edit | 7,214,223 → 7,848,656.5 | 603,224 → 612,261.5 | 138 → 228 |
| python | no_edit | 11.79 → 12.97 | 0 → 0 | 0 → 0 |
| perl | full | 435,112 → 463,271 | 1,337 → 1,337 | 10 → 10 |
| perl | edit | 442,763 → 481,089 | 1,544 → 1,544 | 14 → 14 |
| perl | no_edit | 13.21 → 14.09 | 0 → 0 | 0 → 0 |
| agda | full | 5,479,180.5 → 5,326,696.5 | 31,627 → 31,534.5 | 340 → 340 |
| agda | edit | 6,383,170.5 → 6,125,076 | 30,184 → 30,184 | 346 → 346 |
| agda | no_edit | 12.98 → 13.26 | 0 → 0 | 0 → 0 |
| elixir | full | 2,493,287.5 → 2,457,688.5 | 1,062,214.5 → 1,062,218.5 | 234 → 234 |
| elixir | edit | 5,674,088 → 5,548,506.5 | 1,078,478.5 → 1,078,457.5 | 293 → 293 |
| elixir | no_edit | 13.19 → 13.45 | 0 → 0 | 0 → 0 |
| fsharp | full | 318,820,619 → 312,989,014.5 | 378,674 → 378,578 | 1,076 → 1,074 |
| fsharp | edit | 470,511,324.5 → 460,923,120 | 566,752 → 566,608 | 1,582 → 1,579 |
| fsharp | no_edit | 13.04 → 13.55 | 0 → 0 | 0 → 0 |
| haskell | full | 2,814,150 → 2,847,049.5 | 1,059,545.5 → 1,059,536 | 151.5 → 151 |
| haskell | edit | 2,863,788 → 2,880,522 | 1,060,070.5 → 1,060,066 | 157.5 → 157 |
| haskell | no_edit | 13.09 → 13.31 | 0 → 0 | 0 → 0 |
| html | full | 1,161,222.5 → 1,133,097 | 5,824 → 5,824 | 693 → 693 |
| html | edit | 1,229,591 → 1,222,768.5 | 6,282 → 6,282 | 703 → 703 |
| html | no_edit | 13.19 → 13.1 | 0 → 0 | 0 → 0 |
| markdown | full | 2,923,837.5 → 3,158,647 | 1,289 → 1,289 | 11 → 11 |
| markdown | edit | 6,504,803 → 6,637,520.5 | 656,358 → 656,371.5 | 76 → 76 |
| markdown | no_edit | 13.26 → 13.56 | 0 → 0 | 0 → 0 |
| php | full | 478,479.5 → 759,513 | 5,418 → 45,547.5 | 24 → 282 |
| php | edit | 520,219.5 → 775,747 | 5,520 → 45,648 | 26 → 284 |
| php | no_edit | 13.56 → 13.34 | 0 → 0 | 0 → 0 |
| verilog | full | 54,801,362.5 → 55,995,551.5 | 166,305 → 166,302 | 2,029 → 2,029 |
| verilog | edit | 133,287,790.5 → 134,139,871.5 | 1,305,409 → 1,305,420 | 6,795 → 6,795 |
| verilog | no_edit | 13.33 → 13.6 | 0 → 0 | 0 → 0 |
| go | IncrementalSingleByteEditDFA | 254,383.5 → 285,462 | 390 → 390 | 5 → 5 |
| go | FullDFA | 15,581,176 → 15,744,520 | 1,302.5 → 1,304 | 8 → 8 |
| go | IncrementalNoEditDFA | 14.71 → 14.59 | 0 → 0 | 0 → 0 |

Rejected broader candidates also reached Python M_p=0. Universal clean-version condensation capped the measured C# versions at 6 instead of 21, but lost previously correct Solidity and Nushell trees; it is not a valid performance improvement. Deferring cost competition repaired an EBNF loss, but did not repair those two losses. Forcing the flat primary onto a shared GSS prefix was isolated as a Solidity regression even with child election disabled. A shared-prefix leaf-reuse guard raised Meson's edit tokens from 703 to 731 without restoring reused bytes; a universal leaf guard reduced that example to 248 tokens and 787 reused bytes but changed 71 language rows. The selected unit-only candidate avoids those changes. The previous owner-reported election attempt's C# increase from 21 to 23 is also avoided.

Large-file probes used deterministic valid source generators, with GOMAXPROCS=1 and `/usr/bin/time -v`. Python contains 33,000 functions (`def fN():\n    return N\n`), 1,000,780 bytes; every version covers the input, accepts, and reports no error. C# contains 30,000 classes (`class CN { int x = N; }\n`), 937,780 bytes; main, the starting branch, and the repaired branch all cover the input but stop at the existing memory budget and report an error. This C# probe is an existing failing workload and does not certify accepted completion.

| Large probe | Main peak RSS, KiB | Starting branch peak RSS, KiB | Repaired peak RSS, KiB |
| --- | ---: | ---: | ---: |
| Python | 167916 | not measured | 158584 |
| C# | 834972 | 1480512 | 1458376 |

The C# RSS increase over main is inherited from the starting link-union branch; the repair reduces that branch's measured peak by 1.5%. Existing parser memory-budget contracts pass their focused tests. The full v1 multi-size, C-paired latency and cliff campaign was not run here, so these receipts do not graduate a language or certify release performance.

The final G6 gate passes after the separately recorded census pin refresh. The counter ledger, incremental invariant limits, grammar artifacts and runtime-profile certificates keep their existing pins.

All 17 main/repaired campaigns (16 grammars and the primary Go trio) completed 20 seeds each. The main-relative allocation ratchet fails for C#, Meson, Python, Solidity, Dart and PHP: their full-parse allocation counts increase by more than 10%. These costs need review before integration. Paired campaigns against the starting branch isolate the election's contribution for Python, C#, Meson, Solidity and PHP. No performance threshold or exemption was changed.

To reproduce a grammar campaign, place the benchmark file in the baseline worktree, set `GTS_LEGACY_ELECTION_BENCH_LANG` to one grammar, and run:

```sh
GOWORK=off GOMAXPROCS=1 GTS_LEGACY_ELECTION_BENCH_LANG=python \
  bash scripts/run_randomized_benchmarks.sh \
  --output repaired-python.txt \
  --baseline-root ../baseline --baseline-output baseline-python.txt \
  --package ./grammars --tags '' \
  --bench-regex '^BenchmarkLegacyMergeElection$' \
  --require-benchmarks 'BenchmarkLegacyMergeElection/python/full,BenchmarkLegacyMergeElection/python/edit,BenchmarkLegacyMergeElection/python/no_edit' \
  --runs 20 --seed-start 1 --benchtime 750ms
```

The main Go trio uses the same protocol with package `.` and regex `^BenchmarkGoParse(FullDFA|IncrementalSingleByteEditDFA|IncrementalNoEditDFA)$`. Run correctness separately in the C-parity container, with `GOWORK=off go test -tags 'treesitter_c_parity gts_merge_census' -run '^(TestMergeEventCensusBaseline|TestLegacyMergeElection.*)$' -count=1` from `cgo_harness`.

The paired allocation counts for Python, C#, Meson and Solidity are unchanged from the starting branch. Full/edit timing differences are statistically insignificant in Python, C#, Meson and Solidity. Meson median full/edit times decrease by 2.6%/2.4%. Solidity medians move upward by 1.9%/2.5%, below statistical significance; this directional movement remains recorded. No-edit allocations remain zero in every measured grammar and the Go trio.

PHP full/edit medians move by +0.7%/−0.8%, neither statistically significant; allocation counts decrease by one per operation in each phase (282/284 to 281/283). The paired campaigns confirm that the substantial main-relative allocation costs in Python, C#, Meson, Solidity and PHP preceded this election. Dart was measured against main but did not receive a separate starting-branch campaign.

Election-only paired twenty-seed medians, starting branch to repaired branch:

| Language | Benchmark | ns/op starting → repaired | B/op starting → repaired | allocs/op starting → repaired |
| --- | --- | ---: | ---: | ---: |
| python | full | 3,497,545.5 → 3,422,638.5 | 13,654 → 13,655 | 85 → 85 |
| python | edit | 8,227,581.5 → 8,129,140.5 | 612,225.5 → 612,211 | 228 → 228 |
| python | no_edit | 13.2 → 13.62 | 0 → 0 | 0 → 0 |
| c_sharp | full | 11,615,956.5 → 11,598,937.5 | 1,438,689.5 → 1,438,675 | 3,146 → 3,146 |
| c_sharp | edit | 11,739,632.5 → 11,645,270.5 | 1,440,636 → 1,440,599 | 3,161.5 → 3,161 |
| c_sharp | no_edit | 13.39 → 13.51 | 0 → 0 | 0 → 0 |
| meson | full | 5,678,816.5 → 5,530,013 | 441,298.5 → 441,283.5 | 3,221 → 3,221 |
| meson | edit | 10,699,757.5 → 10,443,776 | 771,903 → 771,930 | 5,637 → 5,637 |
| meson | no_edit | 12.96 → 13.3 | 0 → 0 | 0 → 0 |
| solidity | full | 2,922,751 → 2,978,001 | 89,318.5 → 89,318.5 | 802 → 802 |
| solidity | edit | 6,057,336 → 6,210,620 | 193,757 → 193,758 | 1,642 → 1,642 |
| solidity | no_edit | 13.18 → 13.38 | 0 → 0 | 0 → 0 |
| php | full | 726,852.5 → 732,045.5 | 45,547.5 → 45,524 | 282 → 281 |
| php | edit | 748,519 → 742,342 | 45,648 → 45,624.5 | 284 → 283 |
| php | no_edit | 13.11 → 13.26 | 0 → 0 | 0 → 0 |
