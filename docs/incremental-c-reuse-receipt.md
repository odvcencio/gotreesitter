# Incremental reuse conditions: first legacy increment

The legacy parser now records lexer and external-scanner read frontiers, including failed probes, rollback, and EOF. Edits mark subtrees whose recorded lookahead reaches the edit, even when their visible text ends earlier. The recorded bound includes probes that start at the subtree end: the legacy projection eagerly splices reductions and must preserve that boundary dependency.

New engine logic starts in `internal/incr`. The legacy adapter stores one dependency word per node in arena sidecars; the pinned 104-byte Node layout stays intact. Histories and sidecars obey the arena memory budget and retention caps. Unknown history abstains. Borrowed subtrees forward their bounds, and an incomplete borrowed history cannot certify a new ancestor. A whole-tree lexical shortcut invalidates the old per-node certificates because it does not rebuild their new frontiers. The existing aggregate lexical proof remains in place for that shortcut.

Certified reuse uses incremental arena sizing at 1 MiB and above. The previous size selector reserved one Node per source byte for these incremental reparses, although only the dirty frontier needed rebuilding. This change preserves the existing allocation-budget checks. Missing-token and error-descendant dependency handlers run before native lookahead decisions; a boundary insertion regression checks that missing-token padding coordinates still move correctly.

C's first-leaf conditions authenticate lex modes, reusable table entries, keyword provenance, empty tokens, and external lexical modes. Existing fragility, column dependencies, and serialized scanner checkpoint checks still apply. For accepted, clean, same-width legacy parses with complete history and certified scanner behavior, those conditions replace the fresh parse previously triggered by pre-goto state mismatches. Unproven reuse and recovery keep the fresh proof. Moving byte/point boundaries, stateful-scanner admission, included ranges, contextual close-angle probes, and compact/forest projections remain outside this increment's certification.

The baseline is `f9828512cc58f1bc77b856c2b3451e85266c38b1`. Both revisions use the same benchmark harness. The companion receipt records source hashes and every timing sample. The 137 KiB fixtures are deterministic `benchfixtures.GeneratedSource` reconstructions, rather than the owner's unpublished real-code samples. Go contains 140,368 bytes; Java contains 140,327 bytes. Each operation edits the first marker byte between `x` and `y`, reparses the returned tree, and releases the preceding handle.

The deterministic counters cover the whole operation, including verification attempts. All four alternating edit steps produce these counts:

| Counter | Go before | Go after | Java before | Java after |
| --- | ---: | ---: | ---: | ---: |
| Parse attempts | 2 | 1 | 2 | 1 |
| Shifts | 72,200 | 1,792 | 52,788 | 14 |
| Reductions | 75,729 | 1,803 | 88,698 | 6,369 |
| Action lookups | 359,481 | 16,076 | 298,041 | 27,601 |
| Lexer calls | 59,875 | 1,786 | 52,790 | 15 |
| Leaf constructions | 72,200 | 1,792 | 52,788 | 14 |
| Parent constructions | 45,804 | 1,798 | 88,698 | 6,369 |

Selected-pass reuse stays at 1,770 subtrees / 136,788 bytes for Go. Java changes from 2,122 / 138,184 to 2,121 / 138,181: one boundary token is re-lexed under the first-leaf rule. This is a directional reuse decrease of 0.047% / 0.0022%; it accompanies the elimination of the complete fresh verification pass. No pinned counter, digest, census, expectation, allowlist, or threshold was changed.

Correctness evidence:

- 206 grammars: 48 insertion, undo, and same-width replacement session steps per grammar, incremental equals fresh Go at every step; ERROR-root flags and coverage/stop reasons checked; no-edit allocations zero.
- 206 grammars: fresh and incremental smoke parity against the locked C runtime, one grammar per process, without skips.
- Go and Java: four 137 KiB edit steps match fresh Go and C by the full deep digest, including fields, flags, points, and spans. Generated 32 KiB and 1 MiB fixtures pass the same comparison and no-edit allocation check.
- The eight existing real-source invariant gates pass: Go, Python, JavaScript, JSON, Rust, CSS, Bash, C; all report zero fresh-clean divergences and zero unlisted divergences.
- The reconstructed 72-step sessions pass on both legacy and compact routes for Go, Java, JavaScript, TOML, TSX, TypeScript, Diff, CSS, SCSS, Less, Objective-C, Proto, Zig. The five existing probe ledgers also pass.
- Original longest-match, failed-probe, Unicode-continuation, and release fallback witnesses pass. Arena lifecycle, overflow, retention, Node layout, and memory-budget tests pass. Focused Docker race runs pass.

The replacement sweep exposed existing SQL and Julia bugs. The SQL witness replaces the space in `FROM users` with `x`; incremental and fresh Go now match fresh C. The Julia witness replaces the final newline in `module M\nx = 1\nend\n` with `x`; incremental now equals fresh Go. Fresh Go's malformed recovery tree still differs from C on this witness: Go returns `(source_file (ERROR) (identifier) (assignment (identifier) (operator) (integer_literal)) (identifier))`, while C returns `(ERROR (identifier) (block (assignment (identifier) (operator) (integer_literal)) (identifier)))`. The 48-step fleet regression retains that D8 witness. This increment does not claim malformed Julia C parity.

Rejected candidates:

- Dense byte-indexed history: Go edit bytes/op rose from 3.391 MiB to 3.790 MiB (+11.76%), failing the allocation ratchet despite a timing improvement.
- Unpooled sparse history: the standard full-parse fixture rose from 8 to 17 allocations/op and 1.271 KiB to 256.343 KiB/op. Bounded retention replaces it.
- Certification for moving boundaries: the 72-step Diff ledger rose from zero to five mismatches on each route. Moving boundaries keep their established proof.
- Requiring an immediate first-leaf shift before every splice: Java action lookups rose from 298,041 to 375,589 (+26.0%) and reused bytes fell from 138,184 to 64,366. The boundary dependency fixes the witnesses without that regression.
- Keeping the full-parse arena for 1 MiB certified reuse: parser-only peak RSS rose from 333,136 to 418,204 KiB for Go and 333,928 to 438,384 KiB for Java. Both exceeded 400 bytes per source byte. Incremental arena sizing replaces this candidate; the final paired RSS measurements below pass the limit.

The measured engine revision is `c38930557fe3ad0d81978d39dc1f4088f2a47521`. The engine-file SHA-256 is `dc61af807814e5b360664749931f095adac339111074142240a185da8d11aacf`; the receipt authenticates the source independently of container Git metadata. All timing rows below are medians of 20 completed seeds, using the required randomized runner. Go/C is the median of the per-seed paired ratios.

| Generated fixture | Edit before | Edit after | Go/C before | Go/C after | B/op before → after | Allocs/op before → after |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Go 137 KiB | 215.61 ms | 22.59 ms | 79.02x | 8.29x | 3,547,648 → 3,543,124 | 49.5 → 18 |
| Java 137 KiB | 169.52 ms | 22.93 ms | 50.89x | 6.75x | 3,641,262 → 3,591,746 | 57 → 24 |
| Go 1024KiB | 1707.48 ms | 197.13 ms | 57.68x | 7.05x | 261,198,064 → 8,982,580 | 123 → 37 |
| Go 32KiB | 45.77 ms | 4.19 ms | 78.48x | 7.26x | 880,761 → 886,286 | 41 → 18 |

METRIC: go 137 KiB edit Go/C | 79.02x -> 8.29x | f9828512c -> c38930557 | GeneratedSource, alternating one-byte replacement

METRIC: java 137 KiB edit Go/C | 50.89x -> 6.75x | f9828512c -> c38930557 | GeneratedSource, alternating one-byte replacement

The standard Go trio records the overhead of retaining the new read history and preserving existing lexical proof on the control edit:

| Benchmark | Before | After | B/op before → after | Allocs/op before → after |
| --- | ---: | ---: | ---: | ---: |
| Full parse | 14.02 ms | 14.41 ms | 1298 → 1298 | 8 → 8 |
| Control edit | 303.13 µs | 314.40 µs | 391 → 391 | 5 → 5 |
| No edit | 15.34 ns | 15.35 ns | 0 → 0 | 0 → 0 |

Full-parse time rises 2.74% and control-edit time rises 3.72% in this campaign; both differences are statistically significant under benchstat. The tradeoff is the parse-time read bookkeeping that eliminates the complete verification pass on the larger certified fixtures. Full-parse and control-edit allocation medians stay unchanged. No-edit latency changes 0.065%, without a significant difference; it remains zero bytes and zero allocations. These rows stay within the 10% timing ratchet. Java C latency rises 1.63%, although its runtime and fixture are unchanged.

Parser-only 1 MiB peak RSS is 331,900 → 316,064 KiB for Go (-4.77%) and 329,600 → 245,280 KiB for Java (-25.58%). The final peaks are 308.65 and 239.53 bytes per source byte, below 400. The measurement invokes the already-built benchmark binary under `/usr/bin/time -v`, with `GOMAXPROCS=1`, `-count=1`, and `-benchtime=750ms`; it excludes compilation. Each RSS row is one run, rather than a 20-run RSS median.

CPU sample attribution within Tree.Edit and incremental parsing is Go: 3.71% editing, 12.00% reuse selection/splicing, 84.29% reparse/rebuild; Java: 3.94%, 14.34%, 81.72%. Classification uses complete sampled stacks and excludes initial parsing, grammar loading, release, and background runtime frames. Error-rank computation alone accounts for about 19–20% of all captured samples, and result/error-tree walks add further cost. Profiles guide the next increment; their timings are not comparison evidence.

The comparisons ran in Docker on a shared local Intel Xeon Platinum 8481C CPU, pinned to one CPU, with `GOWORK=off`, `GOMAXPROCS=1`, `-count=1` per process, seeds 1–20, `-benchtime=750ms`, and `-benchmem`. Baseline/head order alternates by seed. Go and C run on identical input in each language process. Dedicated benchmark-runner discovery failed because the available cloud credentials lack authorization scopes. No remote machine was started. An earlier completed 20-seed campaign measured Go 297.67 → 47.09 ms and Java 378.65 → 56.42 ms on the same 137 KiB fixtures, illustrating the shared-machine variation.

To reproduce, mount the baseline and candidate in Docker, copy the five new `cgo_harness/incremental_c_reuse*_test.go` files into the baseline, and run `scripts/run_randomized_benchmarks.sh` with `--baseline-root`, `--baseline-output`, `--output`, `--package ./cgo_harness`, `--tags treesitter_c_parity`, and `--bench-regex '^BenchmarkIncrementalCReuse137K$/^go$'`. Use `java` for the other language and `BenchmarkIncrementalCReuseSizes` for the 32 KiB / 1 MiB set. Use the three named Go benchmarks from the table for the standard trio. Retain the default 20 seeds and 750 ms duration. The correctness harness runs each registry language separately in Docker.

The under-5-ms edit target is not achieved. This is a first legacy increment, and the full performance gate remains incomplete: the three median real files and largest locked real file, 16 fixed deletion sites, insertion/deletion latency percentiles, C Go-C-C-Go cycles, version/cliff attribution, initial load/NewParser cost, and repeated RSS/retained-memory measurements remain unverified. The corpus lock digest was verified without adding the lock to the repository. Complete stateful-scanner admission, cumulative C error-cost metadata, moved boundaries, included ranges, and compact/forest certification remain follow-up work. The expensive whole-tree lexical shortcut remains for the control fixture; its allocation count is still five, rather than the historical zero. No graduation or default-route switch is included.

Incomplete size campaigns at receipt time: sizes-java. The receipt includes only completed 20-seed sets.

Go 32 KiB bytes/op rises 0.63%, from 880,761 to 886,286.5, for the dependency metadata; allocations fall 41 → 18. At 1 MiB, bytes/op falls 96.56%, from 261,198,064 to 8,982,580. The Go size timing distributions are broad: benchstat reports about ±35% at 32 KiB and ±36% at 1 MiB for the new engine, versus ±35% and ±31% before. C size timings also vary widely, although the C runtime is unchanged.
