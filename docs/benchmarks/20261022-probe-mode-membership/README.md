# Lexer-mode membership: 36.15% less Go edit time

Replace repeated linear lexer-mode membership searches with a fixed 512-byte bitmap for IDs below 4,096. Wide IDs retain the original ordered-slice lookup. Preserve first-seen mode order, fallback state zero, no-lookahead and after-whitespace rules, invalid-state rejection, the 1,024-mode and 32,768-row limits, external-mask checks and all proof byte/scan budgets. The bitmap allocates nothing. New engine code lives in `internal/incr`; Node and routing are unchanged.

This addition builds on PR #1509 runtime `4de229c4b1718989917c6212f92cf74df28cf229`. A native profile of the standard numeric edit attributed about a fifth of samples to linear membership searches. The change accelerates those searches without removing any lexer probe or fresh-result verifier. The separate numeric-run projection and arena-reservation experiments are not included.

## Counters and correctness before timing

All **206 counter exports / 412 selected rows are byte-identical**. For example:

| Go workload | Nodes before→after | Tokens before→after | Reused bytes before→after | Block splices before→after |
| --- | ---: | ---: | ---: | ---: |
| Default full | 1922→1922 | 589→589 | 0→0 | 0→0 |
| Default first edit | 3810→3810 | 572→572 | 1166→1166 | 296→296 |
| Candidate full | 1251→1251 | 589→589 | 0→0 | 0→0 |
| Candidate first edit | 1553→1553 | 374→374 | 1784→1784 | 266→266 |

The frozen source passes **412 R4 sessions / 29,664 edit steps**, zero no-edit allocations, **1,236 locked-C fixture checks with no skips**, focused proof and memory tests, and native internal/root race checks. Tests compare bitmap membership with the original algorithm across duplicate, boundary and wide IDs, and pin mode/row limits and invalid primary/after-whitespace states. The installed production files match this frozen source exactly. The identical root regression is appended to its existing test file, keeping root file count unchanged; the installed unit/invariant checks and all 206 counter exports pass again.

## Twenty paired native ARM64 seeds

M1 Max Docker Linux ARM64, Go 1.25.14, GOMAXPROCS=1, count=1, 750 ms, benchmem. The shared randomized runner uses seeds 162001–162020, alternates revision order, and runs the unchanged Go trio in identical shuffled workload order per pair. All sixty rows per revision, terminal audits and before/after source manifests are authenticated. Correctness and deterministic gates precede timing.

| Benchmark | Held runtime | With bitmap | Result |
| --- | ---: | ---: | --- |
| Standard single-byte edit | 130.73 µs | 83.48 µs | −36.15%, p<.001 |
| Full parse | 5.913 ms | 5.902 ms | inconclusive, p=.547 |
| No edit | 6.117 ns | 6.111 ns | inconclusive, p=.417 |

Allocations remain 5/edit, 6/full and 0/no-edit. Edit bytes move 395→394 B/op; all no-edit samples use zero bytes. These results compare this addition with the held three-change runtime; the complete four-change corpus comparison against main has not been rerun. [Full benchstat](benchstat-primary.txt).

## Latest Cogno report and remaining holds

The exact [October 21 v0.56.0 report](https://github.com/odvcencio/gotreesitter/issues/454#issuecomment-6311551850) is replayed on the new source. Its 140,368-byte Go fixture has SHA-256 `5434575bb249338937331c3756e10dcec34237d107f523ea315996fb6de0ef8f`. Both routes and both APIs pass all twelve independent incremental/fresh Go comparisons and eight clean C cases. All four malformed-delete C comparisons still fail: the ERROR has one child in Go and two in C. Failures remain failures. The Windows ARM64/x64 matrix is not rerun.

The timed edit is the standard same-width numeric replacement, not Cogno's identifier edit. This win does not close the small-document verifier adoption blocker, ordinary YAML ownership, the existing absolute 1 MiB RSS hold, or compact graduation. PR #1509 remains draft; no merge or gate exception is requested by this receipt.

The [archive](receipts.tar.gz), [member hashes and qualification](qualification.json), and [frozen prototype patch](prototype.patch.gz) retain full language gates, source authentication, exact-report failures, native raw timings/races, controls and installed-source checks. The prototype patch keeps the regression as its original standalone test; the committed version places the same function in the existing test file. No forced GC or memory-policy change is included.
