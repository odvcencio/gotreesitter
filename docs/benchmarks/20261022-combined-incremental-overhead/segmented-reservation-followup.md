# Segmented node reservation: memory gate remains blocked

The allocator prototypes are not promoted. Splitting an oversized primary node reservation improves median peak RSS, but neither tested layout clears all memory and allocation gates. PR #1509 remains draft; its runtime is unchanged by this receipt.

Both prototypes start from held runtime `4de229c4b1718989917c6212f92cf74df28cf229`. They preserve total reserved node capacity, public Node layout, budgets, pool ceilings and the primary-reservation discard policy. The second uses bounded backing groups and lazy shared metadata arrays to reduce the first prototype's allocation overhead; it also restores the original first-overflow growth rule. Source files for both experiments are archived. No manual GC is used.

The refined prototype passes all 206 languages: **412 R4 edit sessions, 1,236 locked-C fixture checks with no skips, and 206 byte-identical counter exports**. Focused arena, pointer stability, metadata ownership, budget, retention, overflow-growth and twenty memory-contract tests pass. The explicit metadata test is rerun successfully with the source directory mounted as its working directory; an earlier fixture-directory harness failure is retained. These correctness passes do not qualify the memory result.

The same 1,048,591-byte generated Go fixture has SHA-256 `76c70b62127085d3c791ba98a24cba9f22bd343e40afb43522bbb5041cede524`. Native Linux ARM64 on the M1 Max uses Go 1.25.14, GOMAXPROCS=1, precompiled parser processes and direct-child rusage. Each experiment has twenty matched, alternating pairs and exactly two measured benchmark iterations. Compilers remain outside measured RSS; source manifests stay unchanged, every parse passes completion assertions, and no container OOM occurs. Warm-parse counters remain 993,391 nodes, 425,742 tokens and two maximum stacks; the benchmark does not report each measured iteration's work separately. Separate C preflights validate complete warm parses.

| Experiment | Held median / maximum RSS | Prototype median / maximum RSS | Held / prototype ceiling failures | Held / prototype allocs per operation |
| --- | ---: | ---: | ---: | ---: |
| Separate 8 MiB pieces; seeds 160001–160020 | 456,495,104 / 462,602,240 B | 406,546,432 / 408,977,408 B | 13 / 0 | 116 / 148 |
| Grouped pieces and shared metadata; seeds 161001–161020 | 458,194,944 / 461,078,528 B | 395,821,056 / 507,138,048 B | 12 / 4 | 116 / 124–128 |

The absolute ceiling is **419,436,400 bytes**. The first layout stays below it in this sample but adds 27.6% allocations. The refinement reduces median allocation overhead to 6.90%, within the 10% median ratchet, yet four runs breach the RSS ceiling. The 124–128 range stays visible; the ratchet applies to the twenty-seed median. A lower median does not qualify either layout. These fixed-iteration RSS processes provide **no randomized throughput claim**.

The [archive](segmented-reservation-followup.tar.gz) and [member hashes and qualification](segmented-reservation-followup.json) retain controls, raw samples, source manifests, both prototypes, expected harness failure and successful rerun, and full language gates. The earlier [allocation profile](rss-attribution-followup.md) still supports targeting node representation and allocation lifetime. This experiment does not fix Cogno's small-document verifier blocker or malformed-delete C mismatch, and it does not graduate Go or YAML.
