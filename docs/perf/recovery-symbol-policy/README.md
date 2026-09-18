# Recovery symbol allocation, 18 September 2026

Reuse one symbol table for recovery during each compact parse.
The change applies to shared scheduler code without language-specific conditions.
Nine call sites previously allocated the same projection repeatedly.
The scheduler now builds the projection on first use and discards it at reset.
Memory accounting includes its capacity. Symbol defaults and recovery costs do not change.

The baseline is `19c3fdbb88c4f56bd47bda37df5f97f6d4a10fa3`.
[Source hashes](source-manifest.json) identify the candidate code and regression tests.
Both revisions use identical benchmark functions.

## Results

The real Go query compiler fixture allocates 81.58 percent fewer bytes and 37.22 percent fewer objects.
Both changes have `p<0.001` across 20 samples per revision.
The timing difference is not significant.

| Metric | Baseline median | Candidate median |
| --- | ---: | ---: |
| Time per parse | 13,380,876 ns | 13,403,781 ns |
| Bytes per parse | 69,428 | 12,788 |
| Allocations per parse | 317 | 199 |

Every measured query compiler parse uses the compact route without a fallback.
The clean JavaScript control also preserves its compact route.

| Timing control | Baseline median | Candidate median | p-value |
| --- | ---: | ---: | ---: |
| Go full parse | 10.18 ms | 10.08 ms | 0.242 |
| Go incremental byte edit | 132.3 us | 130.8 us | 0.512 |
| Go incremental without edits | 2.722 ns | 2.744 ns | 0.062 |
| Clean JavaScript | 58.32 us | 57.38 us | 0.268 |

A separate 20-pair Python full-parse comparison measures 11.48 ms before and 11.49 ms after, with `p=0.883`.
Python allocation metrics also show no significant change.
No control shows a significant change in timing or allocation metrics.
These results establish an allocation improvement on one real source fixture.
They do not establish a timing improvement for all languages.

The raw results and comparison are available here:

- [Baseline samples](go-javascript-baseline.txt)
- [Candidate samples](go-javascript-candidate.txt)
- [Benchstat comparison](go-javascript-benchstat.txt)
- [Python baseline samples](python-baseline.txt)
- [Python candidate samples](python-candidate.txt)
- [Python comparison](python-benchstat.txt)

## Protocol

Work ran locally on chi-1 with these settings:

- Intel Core Ultra 9 285, Linux amd64, and Go 1.25.14.
- Docker image `gotreesitter/cgo-harness:go1.25-local`.
- Image identifier `sha256:e717b652af60b717ce74dc77433d97b1db23ccd8368dea07ed24af67426b8593`.
- Affinity to logical processor 2 and a one-processor quota.
- A 4 GiB container limit and `GOMEMLIMIT=3GiB`.
- `GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`, and `-benchmem`.
- Twenty explicit shuffle seeds with alternating baseline and candidate order.

The container cannot resolve host worktree metadata. The source manifest supplies the baseline revision and candidate hashes.

Create separate baseline and candidate checkouts. Mount the baseline at `/baseline` and an output directory at `/evidence`.
Run this command from the candidate checkout inside the bounded container:

```sh
bash scripts/run_randomized_benchmarks.sh \
  --output /evidence/candidate.txt \
  --baseline-root /baseline \
  --baseline-output /evidence/baseline.txt \
  --runs 20 --seed-start 1 --benchtime 750ms \
  --bench-regex '^(BenchmarkAdmissionCandidateGoQueryCompileWarmRoute|BenchmarkAdmissionCandidateJavaScriptCleanWarmRoute|BenchmarkGoParseFullDFA|BenchmarkGoParseIncrementalSingleByteEditDFA|BenchmarkGoParseIncrementalNoEditDFA)$' \
  --require-benchmarks BenchmarkAdmissionCandidateGoQueryCompileWarmRoute,BenchmarkAdmissionCandidateJavaScriptCleanWarmRoute,BenchmarkGoParseFullDFA,BenchmarkGoParseIncrementalSingleByteEditDFA,BenchmarkGoParseIncrementalNoEditDFA
```

Repeat the protocol with fresh output paths for Python.
Set `--bench-regex '^BenchmarkPythonParseFullDFA$'` and `--require-benchmarks BenchmarkPythonParseFullDFA`.

## Correctness

Focused recovery and memory-accounting tests pass before and after the change.
New tests cover projection width, visibility defaults, repeated allocation, reset cleanup, and changed metadata between parses.

Separate bounded containers pass these C-reference checks on both revisions:

- Go fresh and incremental parsing.
- JavaScript recovery selection, incremental recovery, and fallback selection.
- PHP issue 454, including the complete tree digest and fallback behavior.
- Python fresh and incremental parsing.
- TypeScript fresh and incremental parsing.

The focused recovery tests also pass with the race detector on both revisions.
They cover recovery-cost comparisons, version condensation, missing-token insertion, and scheduler footprint accounting.

Run the focused tests inside a bounded container:

```sh
go test -race -tags gts_parsercorephase0 . \
  -run '^Test(Recovery|Stage3Recovery|S5|DiagnosticParserCore.*(Recovery|Footprint)|CompactMemoryBudget|AdmissionSwitchCompactMemoryBudget)' \
  -count=1 -timeout=8m
```

Memory polls keep their existing frequency. The new table counts toward the scheduler footprint while it remains live.
A parse near its memory limit can therefore stop earlier by the size of that table.

## Large-source memory limits

Both large Go table-literal tests pass on both revisions with a 19,877,871-byte source.
Both revisions stop at byte 3,385,678 with the memory-budget reason.
The shipped-route test records identical heap growth of 530,237,728 bytes.
This preserves the existing containment contract; the configured budget is not a strict process-memory limit.

Maximum resident set size (RSS) is 579,504 KiB for the baseline and 580,412 KiB for the candidate.
The candidate observation is 908 KiB higher. One process per revision does not establish an RSS improvement or regression.
Neither container reports an out-of-memory termination or timeout.
The logs are [baseline](large-go-baseline.txt) and [candidate](large-go-candidate.txt).

Compile outside the measured interval. Run the same command for each revision inside a bounded container:

```sh
go test -c -o /evidence/memory.test ./roottest/parse
/usr/bin/time -v /evidence/memory.test \
  -test.run='^TestGoParseGiantTableLiteral(StopsWithinMemoryBudget|ShippedRouteStaysWithinAchievedBound)$' \
  -test.count=1 -test.v -test.timeout=8m
```
