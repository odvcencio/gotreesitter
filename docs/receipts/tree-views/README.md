# Tree-view evidence

The [receipt](../../tree-views-receipt.md) defines operation scope, revisions,
source hashes, native allocation accounting, and remaining failures.

- `complete-before.txt` and `complete-after.txt`: 20 paired shuffled processes
  for the first view implementation and stable slabs, including native C.
- `final-complete-before.txt` and `final-complete-after.txt`: the final cache
  compared with the first slab layout, including native C.
- `sparse-before.txt` and `sparse-after.txt`: the final sparse API comparison
  against the first correct implementation.
- `rejected-sparse-before.txt` and `rejected-sparse-after.txt`: the measured
  sparse regression that caused the first slab layout to be superseded.
- `default-before.txt` and `default-after.txt`: the separate default Go trio.
- `final-default-before.txt` and `final-default-after.txt`: its final repeat
  against the completed cache implementation.
- `navigation.txt`: generated old/new graph witnesses at three sizes.
- `smoke.txt`: all 206 registered grammars, one test process per grammar.
- `rss.csv`: cold full-operation RSS and allocation probes for three revisions.

The samples retain benchmark values and seed order. Temporary paths and build
output are removed. They are measurement receipts, not refreshed gate pins.

```sh
benchstat docs/receipts/tree-views/final-complete-before.txt \
  docs/receipts/tree-views/final-complete-after.txt
benchstat docs/receipts/tree-views/sparse-before.txt \
  docs/receipts/tree-views/sparse-after.txt
benchstat docs/receipts/tree-views/final-default-before.txt \
  docs/receipts/tree-views/final-default-after.txt
```

To reproduce complete-operation timing, check out the source revisions in the
receipt and use the wrapper. Build the standalone driver against the oracle
checkout at `3a0e7df5`, using its runtime headers and source, and obtain the
locked grammar artifacts from that checkout's harness. Verify their hashes
against the receipt. The variables below refer to local measurement checkouts
and artifacts; they contain no fixed machine paths.

```sh
cc -O2 -DNDEBUG -std=c11 -D_POSIX_C_SOURCE=200112L -D_DEFAULT_SOURCE -rdynamic \
  -I"$task_oracle_checkout/cgo_harness/internal/coracle/include" \
  -I"$task_oracle_checkout/cgo_harness/internal/coracle/src" \
  cgo_harness/pure_c/tree_navigation_benchmark.c \
  "$task_oracle_checkout/cgo_harness/internal/coracle/src/lib.c" \
  -ldl -o "$task_c_driver"

GOWORK=off GTS_TREE_NAV_C_DRIVER="$task_c_driver" \
  GTS_TREE_NAV_C_GO="$task_go_artifact" \
  GTS_TREE_NAV_C_C_SHARP="$task_csharp_artifact" \
  bash scripts/run_randomized_benchmarks.sh \
    --output "$task_receipt_dir/after.txt" \
    --baseline-root "$task_baseline_checkout" \
    --baseline-output "$task_receipt_dir/before.txt" \
    --bench-regex '^Benchmark(TreeNavigationComplete(Payload|Views)|LockedCTreeNavigationComplete)$'
```

The fixture target defaults to 32,768 bytes. `GTS_TREE_VIEW_BYTES=140288` or
`1048576` selects the larger fixtures for navigation, benchmark, and C parity
tests. A final default-parser comparison uses the explicit regex
`^BenchmarkGoParse(FullDFA|IncrementalSingleByteEditDFA|IncrementalNoEditDFA)$`.

Correctness runs use Docker and one grammar per test process. The focused root
tests are `TestTreeViewNavigationWitness`, `TestTreeViewEditSessions`,
`TestTreeViewAdaptersAndNoEdit`, `TestTreeViewConcurrentNavigation`,
`TestTreeViewReleaseDropsPayloadReferences`, and `TestTreeViewLanguageSmoke`.
Set `GTS_TREE_VIEW_LANGUAGE` to one registered grammar for its smoke test.
The broad smoke pass builds a test binary once, then starts it separately for
each remaining grammar; no parser state survives between grammar processes.

`TestTreeViewNavigationLockedC` lives in `cgo_harness` and uses
`-tags treesitter_c_parity`. Run its `go` and `c_sharp` subtests separately.
For runtime 0.27 parity, copy that test into the pinned oracle measurement
checkout and point its module replacement at the view source under test.
This procedure changes the measurement checkout, rather than the public
repository's current oracle pin.

RSS probes compile the Go test binary before `/usr/bin/time -v`, then run only
`BenchmarkTreeNavigationCompleteViews/<grammar>/Full` with `GOMAXPROCS=1`,
`-test.benchtime 1x`, and `-test.benchmem`. They are memory probes; randomized
timing claims come only from the complete 20-seed comparisons above.
