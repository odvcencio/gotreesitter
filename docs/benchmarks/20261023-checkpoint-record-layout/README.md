# Twelve-byte checkpoint records: qualified building block

The private prototype shrinks each immutable scanner checkpoint record from 44 to 12 bytes. A four-entry cache retains full SHA-256 receipts; evicted receipts are recomputed from the independently retained immutable bytes. Exact byte comparison, hash-collision chains, reset generation, owner validation and full SHA comparisons remain. Receipt readers never mutate the cache. Its lazy allocation and retained capacity are charged, and detached recovery clones copy and charge their own cache.

This is an **unpromoted memory building block**, not a throughput win. It does not enable ordinary YAML ownership or graduate a language. No runtime code or ledger baseline is changed by this documentation bucket.

The baseline is PR #1509 runtime `dba13461f0df4dc8eaf626a80d143a7cf0e8e5ab`. The archive authenticates every unchanged Go file and the complete prototype patch. New engine code remains in `internal/parsercorephase0`; public Node and root file count are unchanged.

Before timing, the final source passes all **412 R4 sessions / 29,664 edits**, zero-allocation no-edit reparses, **1,236 C fixture checks without skips**, and **206 byte-identical counter exports**. Full core and shadow suites, focused races, snapshot ownership/immutability guards and twenty memory contracts pass. A one-MiB owned-YAML document stream independently matches C in both revisions. Earlier build failures and the pre-accounting prototype remain visible; they are not treated as final qualification.

For example, Go default full remains 1,922 nodes / 589 tokens, default first edit 3,810 nodes / 572 tokens / 1,166 reused bytes / 296 splices, and candidate first edit 1,553 nodes / 374 tokens / 1,784 reused bytes / 266 splices. YAML default full remains 288 nodes / 107 tokens. All selected counter rows stay unchanged; no exception is requested.

## Local Linux amd64 memory screen

Go 1.26.6, GOMAXPROCS=1, four-GiB Docker, precompiled parser binaries, direct-child rusage, no manual GC. Twenty alternating pairs per mode use seeds 164001–164020 and two measured iterations. Every measured parse checks completion, work and route against the warm parse. The 1,049,572-byte YAML fixture has SHA-256 `8d615e96c0efe207dd1bfc4f54f601580d61dd0afc8f051ac3aaa92a2b27e397`.

| Route | Median RSS before→after | Maximum before→after | Median bytes/op before→after | Median allocs/op before→after |
| --- | ---: | ---: | ---: | ---: |
| Ordinary, existing fallback | 176,635,904→176,844,800 B | 234,586,112→234,790,912 B | 114,437,312→114,437,288 | 35.5→35 |
| Diagnostic-owned compact | 115,800,064→113,983,488 B | 123,158,528→121,331,712 B | 6,613,320→6,631,784 | 24,778→24,780 |

Ordinary median RSS changes +0.12%; diagnostic-owned median falls 1.57%. Both pass relative and absolute screens. Owned byte/allocation increases remain visible and within 10%. These fixed-iteration samples establish no throughput comparison. This screen does not replace the separately pinned cold-parse memory protocol required for a future ordinary-ownership candidate.

## Randomized throughput

Twenty matched local amd64 pairs use the shared randomized runner, GOMAXPROCS=1, count=1, 750 ms and benchmem. YAML seeds are 165001–165020 and the primary Go trio uses 166001–166020. Revision order alternates, and each pair has the same shuffled workload order. All 120 YAML and 60 Go rows per revision complete. Sources stay unchanged.

All nine timing differences are statistically inconclusive. Directional YAML changes include diagnostic-owned R4 **654.1→686.0 µs (+4.88%, p=.231)**, ordinary EOF8K **1.040→1.091 ms (+4.90%, p=.289)**, owned EOF8K **2.481→2.565 ms (+3.39%, p=.289)** and ordinary R4 **349.3→357.7 µs (+2.40%, p=.327)**. Every median remains within 10%; bytes and allocation medians on these six cells are unchanged. Go full is **6.731→6.635 ms (p=.383)**, edit **96.64→96.94 µs (p=.989)** and no edit **4.535→4.505 ns (p=.425)**; allocations remain 6/5/0, with zero no-edit bytes. See complete [YAML](benchstat-yaml.txt) and [Go](benchstat-go.txt) tables. None of these data demonstrate a throughput improvement.

Cogno's exact [October 21 report](https://github.com/odvcencio/gotreesitter/issues/454#issuecomment-6311551850) is replayed: all twelve independent Go incremental/fresh comparisons and eight clean C cases pass; the same four malformed-delete C shape comparisons fail. Windows ARM64/x64 is not rerun. The owner approved the exact Mac source overlays and benchmark controls on October 23. The completed [Mac qualification](mac-screen.md) authenticates the same source: owned YAML median RSS falls 2.51%, while two owned timings regress 2.63% and 3.38%. Its limits and raw evidence are separate from this local receipt.

The [raw archive](receipts.tar.gz), [hashes and qualification](qualification.json), and [prototype patch](prototype.patch.gz) retain commands, source authentication, complete gate logs, all memory pairs, randomized outputs and expected report failures. A separate prototype combines this storage change with the earlier ordinary-YAML ownership algorithm on current block storage. Its focused correctness and fifty whole-request work-counter comparisons pass, but the matched cold-parse ordinary-YAML RSS ratio is 1.2026, above 1.10. It remains private and unpromoted. None of this receipt's broader qualification transfers to that integration automatically. PR #1509 remains held on its existing Go RSS failure.
