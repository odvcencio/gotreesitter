# Compact resource accounting, 18 September 2026

This change separates compact resource checks from scheduling and shares symbol metadata projection.
It also fixes an omitted recovery-memo charge in the scheduler memory footprint.
The regression test reported zero growth for a 320-byte memo before the correction.
The corrected calculation includes both backing arrays and retains the charge after reset.

The baseline is `dc055fe07652293aac5d0d9967dd5fe7a14a7f58`, the reviewed recovery-symbol allocation change.
[Source hashes](source-manifest.json) identify the measured candidate.
Two source comments received updated file references after measurement. Their final hashes are recorded separately.
The compared benchmark functions are identical.

## Performance tradeoff

| Benchmark | Baseline median | Candidate median | Result |
| --- | ---: | ---: | --- |
| Real Go query compiler | 13.104 ms | 13.283 ms | 1.37 percent slower, p=0.035 |
| Go full parse | 10.12 ms | 10.03 ms | No significant change, p=0.640 |
| Go incremental byte edit | 130.3 us | 130.3 us | No significant change, p=0.640 |
| Go incremental without edits | 2.712 ns | 2.716 ns | No significant change, p=0.262 |

Allocated bytes and allocation counts show no significant change on all four benchmarks.
The real Go fixture allocates 12,788 bytes and 199 allocations per parse.
Every measured real-source parse uses the compact route without fallback.

Keep the timing regression visible. The change corrects memory-limit enforcement and establishes source ownership; it does not claim a speed improvement.
The comparison measures the combined change. It does not isolate the cost of the added memory charge.

Read the [baseline samples](baseline.txt), [candidate samples](candidate.txt), and [Benchstat comparison](benchstat.txt).

## Reproduce the comparison

Use separate baseline and candidate checkouts inside the Docker harness.
Mount the baseline at `/baseline` and an output directory at `/evidence`.
Use these settings:

- Go 1.25.14 on Linux amd64 and Intel Core Ultra 9 285.
- Image `gotreesitter/cgo-harness:go1.25-local`.
- Image identifier `sha256:e717b652af60b717ce74dc77433d97b1db23ccd8368dea07ed24af67426b8593`.
- Affinity to logical processor 2 and a one-processor quota.
- A 4 GiB container limit and `GOMEMLIMIT=3GiB`.
- `GOMAXPROCS=1`, one sample per process, and alternating revision order.

Run from the candidate checkout:

```sh
bash scripts/run_randomized_benchmarks.sh \
  --output /evidence/candidate.txt \
  --baseline-root /baseline \
  --baseline-output /evidence/baseline.txt \
  --runs 20 --seed-start 1 --benchtime 750ms \
  --bench-regex '^(BenchmarkAdmissionCandidateGoQueryCompileWarmRoute|BenchmarkGoParseFullDFA|BenchmarkGoParseIncrementalSingleByteEditDFA|BenchmarkGoParseIncrementalNoEditDFA)$' \
  --require-benchmarks BenchmarkAdmissionCandidateGoQueryCompileWarmRoute,BenchmarkGoParseFullDFA,BenchmarkGoParseIncrementalSingleByteEditDFA,BenchmarkGoParseIncrementalNoEditDFA
```

## Correctness and memory

The moved stop-control declarations were verified byte-for-byte before the accounting correction.
Focused tests pass for recovery, symbol projection, memory limits, cancellation, and deadlines, including race detection.
Both selected-store build modes and the emergency parser build pass.
Go, JavaScript, and PHP pass focused pinned-C parity checks in separate containers.

Both large Go memory regressions pass on the 19,877,871-byte source.
The shipped-route heap growth remains 530,237,728 bytes and preserves the existing containment contract.
The configured budget is not a strict process-memory limit.
The candidate's maximum resident set size is 579,888 KiB in one isolated resource probe.
The container reports no out-of-memory termination or timeout.
The [resource log](large-go.txt) includes the command and observations.
