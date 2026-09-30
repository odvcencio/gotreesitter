# Issue #1358 receipts

`source-identity.json` pins the production files and identical benchmark test
used by both worktrees. The baseline is `2e93c61b`; the final engine is
`591f74eb`. `c-reference.json` records the exact C runtime and grammar sources.
The corpus lock is not included.

`counter-changes.json` lists every changed counter row. `invariant-summary.json`
lists the results of all 206 grammars on both admission routes.
`final-root-parser.json` records the successful final parser-package rerun.
`root-failure-comparison.json` identifies the four full-suite failures that also
occur on the baseline. `real-fixtures.json` records five pinned AWK inputs and
fresh C equality. `memory-fixture-parity.json` records exact fresh C equality for
the 1 MiB memory input.

Timing logs use the repository's randomized wrapper, 20 seeds, one process per
seed and revision, `GOMAXPROCS=1`, `GOWORK=off`, `-count=1`, `-benchtime=750ms`,
and `-benchmem`. Baseline/fixed ordering alternates by seed. The AWK benchmark
runs Go-C-C-Go cases on identical input. The ratio summary averages the two Go
and two C samples within a seed, then takes the median of the 20 seed ratios.
`benchstat` retains the individual benchmark cases and their significance tests.
The initial `# status: incomplete` header is closed by `# status: complete`.
Repository-directory and dirty-status comments are removed from the public
copies; revision and all benchmark results remain. Source hashes are recorded
separately because the wrapper's metadata alone does not authenticate source.

For exact runtime timing, copy the verified `v0.25.0` Go binding into an external
scratch directory, then replace its `src` and `include` contents with `lib/src`
and `lib/include` from runtime commit
`f5afe475deb7c0bae6407fb776c76824f717bb61`. Copy `cgo_harness/go.mod` and
`go.sum` to `issue1358-go.mod` and `issue1358-go.sum` in each worktree. In each
harness directory, set the alternate modfile's replacement:

```sh
GOWORK=off go mod edit -modfile=issue1358-go.mod \
  -replace="github.com/tree-sitter/go-tree-sitter=$binding_scratch"
```

Copy the checked-in AWK benchmark test into the baseline harness. From the
fixed harness directory, run:

```sh
GOFLAGS='-p=1 -modfile=issue1358-go.mod' GOWORK=off \
  bash ../scripts/run_randomized_benchmarks.sh \
  --output "$results/awk-head.txt" \
  --baseline-root "$baseline/cgo_harness" \
  --baseline-output "$results/awk-base.txt" \
  --tags treesitter_c_parity --package . \
  --bench-regex '^BenchmarkAWKIssue1358$' \
  --require-benchmarks 'BenchmarkAWKIssue1358/Full/GoFirst,BenchmarkAWKIssue1358/Full/CFirst,BenchmarkAWKIssue1358/Full/CSecond,BenchmarkAWKIssue1358/Full/GoSecond,BenchmarkAWKIssue1358/Insert/GoFirst,BenchmarkAWKIssue1358/Insert/CFirst,BenchmarkAWKIssue1358/Insert/CSecond,BenchmarkAWKIssue1358/Insert/GoSecond,BenchmarkAWKIssue1358/NoEdit/GoFirst,BenchmarkAWKIssue1358/NoEdit/CFirst,BenchmarkAWKIssue1358/NoEdit/CSecond,BenchmarkAWKIssue1358/NoEdit/GoSecond'
```

Run the primary Go benchmark trio from the fixed repository root:

```sh
GOWORK=off bash scripts/run_randomized_benchmarks.sh \
  --output "$results/go-head.txt" --baseline-root "$baseline" \
  --baseline-output "$results/go-base.txt" \
  --bench-regex '^BenchmarkGoParse(FullDFA|IncrementalSingleByteEditDFA|IncrementalNoEditDFA)$' \
  --require-benchmarks 'BenchmarkGoParseFullDFA,BenchmarkGoParseIncrementalSingleByteEditDFA,BenchmarkGoParseIncrementalNoEditDFA'
```

For RSS, use `make-rss-fixture.py` to create the 1 MiB witness padded with an AWK
comment. The `rss-go.go.txt` helper builds twice, against the baseline and fixed
root modules using external `replace` directives. Copy `rss-c.c.txt` to a
scratch `.c` file and build it directly with the pristine locked C runtime:

```sh
cc -O2 -DNDEBUG -std=c11 -D_POSIX_C_SOURCE=200112L -D_DEFAULT_SOURCE \
  -I"$runtime/lib/include" -I"$runtime/lib/src" \
  "$rss_c_source" "$runtime/lib/src/lib.c" -ldl -o "$native_c"
```

`rss-samples.sh` takes the fixture, baseline Go executable, fixed Go executable,
native C executable, exact AWK grammar shared object, and output directory.
It runs 20 alternating samples per mode with ten parses per process and
`/usr/bin/time -v`. The baseline helper records the known erroneous insertion;
the fixed helper rejects errors. This is a memory diagnostic, not a bypass of
a correctness gate. RSS includes the runtime, input buffers, initial tree, and
live results; native C uses no Go binding. The summary retains every sample and
both medians and maxima. `summarize-rss.py` accepts the samples directory.
`summarize-timing.py` accepts a directory containing `awk-base.txt` and
`awk-head.txt`; both scripts emit their JSON summary alongside the inputs.

`attribution.json` separates `Tree.Edit`, reuse-cursor setup, and reparse/rebuild
using 1,000 profiled edits. `profile-attribution.go.txt` contains that diagnostic.
Those profile times are not the randomized comparison evidence.

The timing and memory checks cover the reported input and a padded witness.
They do not constitute the complete language-graduation performance matrix of
three generated sizes, all real files, and 16 deletion sites.
