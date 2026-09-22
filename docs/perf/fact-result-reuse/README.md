# Fact result reuse

Measured on 2026-09-22, from base commit `e65d88a27` with the accompanying changes.
The existing `Extract` method keeps its behavior.
The new `ExtractInto` method reuses caller-owned result slices and clears previous entries.
Callers must consume or clone results before they reuse the destination.
Assign an empty `FactSet` to the destination to release its retained storage.

## Results

The fixture contains 500 Go functions and one package declaration in 19,294 bytes.
Both methods extract all supported fact kinds from the same existing tree.
This fixture exercises definitions and package imports. It contains no calls or inheritance.
The measurements exclude parsing and program compilation.

| Metric | `Extract` | `ExtractInto` | Change |
| --- | ---: | ---: | ---: |
| Time per extraction | 100.61 microseconds | 72.90 microseconds | -27.54% |
| Allocated bytes per extraction | 99,944 | 1,919 | -98.08% |
| Allocations per extraction | 512 | 501 | -2.15% |

All differences have `p < 0.001` across 20 samples.
String extraction still allocates. Result reuse does not eliminate those allocations.
The destination retains its largest slice capacities until the caller releases them.

A separate probe parsed 327,793 bytes and extracted 10,000 definitions 100 times.
The managed heap stayed near 35.5 megabytes after garbage collection.
Maximum resident set size was 137,792 kibibytes. The process exited successfully without a memory limit failure.
The [probe source](rss_probe.go.txt) and [output](memory-and-api.txt) preserve this bounded retention check.
This observation does not establish a resident-memory improvement over `Extract`.

The same run measured the primary Go parser benchmarks:

| Operation | Median time | Allocations |
| --- | ---: | ---: |
| Full parse | 9.986 milliseconds | 37 |
| Single-byte incremental edit | 95.15 microseconds | 5 |
| Unchanged incremental parse | 3.522 nanoseconds | 0 |

These parser measurements describe the current build. They do not establish a parser speed change.
This run is a focused measurement, not a complete release benchmark suite.

## Reproduction

The run used an Intel Core Ultra 9 285 and the existing Docker harness image.
Docker pinned processor 0 and limited the container to one processor and 8 gibibytes of memory.
The script used `GOMAXPROCS=1`, seeds 1 through 20, and one process per seed.
Each process used `-count=1`, `-benchtime=750ms`, and `-benchmem`.

Run this command from the repository root inside the harness container:

```sh
bash scripts/run_randomized_benchmarks.sh \
  --output /evidence/benchmarks.txt \
  --runs 20 \
  --benchtime 750ms \
  --bench-regex '^(BenchmarkFactProgramAllTreeGo(Reuse)?|BenchmarkGoParseFullDFA|BenchmarkGoParseIncrementalSingleByteEditDFA|BenchmarkGoParseIncrementalNoEditDFA)$' \
  --require-benchmarks BenchmarkFactProgramAllTreeGo,BenchmarkFactProgramAllTreeGoReuse,BenchmarkGoParseFullDFA,BenchmarkGoParseIncrementalSingleByteEditDFA,BenchmarkGoParseIncrementalNoEditDFA
```

The [raw output](benchmarks.txt) preserves both method names and all parser measurements.
The [comparison](benchstat.txt) separates the two method series and gives them the same benchmark name for `benchstat`.
Both methods run in each randomized process. This comparison measures two methods in one build, not two repository revisions.

The source files have these Secure Hash Algorithm 256-bit (SHA-256) hashes:

| File | SHA-256 |
| --- | --- |
| `fact_program.go` | `628115f851e67c8c7acf5a20b02f8ab3df602e0b6ab0f3cc4d28241eaf71b8ca` |
| `benchmark_tagger_test.go` | `fdf6b6630f4b0f05c90944988ca1e4f6847c0163c1b835db22d5d428a07d6f52` |

Run the retention probe from the repository root inside the same container:

```sh
cp docs/perf/fact-result-reuse/rss_probe.go.txt /tmp/fact-rss.go
GOMAXPROCS=1 go build -o /tmp/fact-rss /tmp/fact-rss.go
/usr/bin/time -v env GOMAXPROCS=1 /tmp/fact-rss
```

## Correctness

Docker tests passed for each language separately:

- Go
- JavaScript
- TypeScript
- Python
- Java
- Starlark

Each language compares repeated extraction with the legacy extractors.
Additional tests check storage reuse, stale entries, and nil inputs.
They also check language mismatches and changes to the selected fact kinds.

The focused assessment also passed these existing checks:

- Canonical Go full-parse preflight against the pinned C grammar.
- Go fresh and incremental parity against C.
- Scala scanner recovery parity against C for both current witnesses.
- Scanner proof tampering and injected probe failures.
- Go tree ownership and parser memory budgets.
- Nil accessors and changed-range coordinates.
- Public work limits and strict incremental stops.
- COBOL column dependencies across copies and repeated incremental edits.

The C# trailing-comma divergence remains documented in `csharp_grammargen_cgo_regression_test.go`.
This work does not change parser routing or grammar tables.
