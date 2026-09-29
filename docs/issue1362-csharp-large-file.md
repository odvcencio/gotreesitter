# C# large-file condensation (issue 1362)

The default parser retained redundant clean versions across reduction rounds.
Their packed GSS histories forked again on the same lookahead. Main reached
107 live versions on the repetitive 512 KiB compilation unit, exhausted its
memory budget, and returned an error tree. C condenses versions after dispatch
and uses a maximum version count of six.

The fix extends condensation to certified clean full parses. It merges only
equivalent payloads through the existing GSS merge operation; distinct clean
interpretations retain their order and population policy. Recovery rounds,
incremental reuse, and nested recovery compatibility probes retain their
existing behavior. No grammar-name checks, runtime fields, or public API were
added.

## Measurements

Baseline: `5ff20d90`. Both revisions used the same generated source,
`GOMAXPROCS=1`, and one fresh process per size. Parse times exclude process
startup; RSS is the process maximum from `/usr/bin/time -v`. These four runs
are diagnostic measurements, not the randomized benchmark comparison.

| Target KiB | Source bytes | Main seconds | Fixed seconds | Main RSS KiB | Fixed RSS KiB | Main peak versions | Fixed peak versions |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 64 | 65,608 | 3.335 | 0.159 | 144,372 | 57,088 | 97 | 4 |
| 128 | 131,074 | 6.381 | 0.509 | 254,340 | 81,392 | 97 | 4 |
| 256 | 262,205 | 17.902 | 0.742 | 500,200 | 130,884 | 102 | 4 |
| 512 | 524,376 | 889.424 | 1.675 | 829,620 | 228,544 | 107 | 4 |

Main accepts 64–256 KiB without errors; 512 KiB stops with `memory_budget` and
an error tree. The fixed parser accepts every size without errors and covers
the complete input. The 512 KiB GSS allocation falls from 235,528,344 to
52,106,280 bytes; allocated nodes fall from 3,114,175 to 999,407. Its node arena
falls from 580,000,768 bytes (over the 536,870,912-byte budget) to 162,596,200.
The smallest regression witness is 2,050 bytes: main retains 65 versions,
while the fix retains four. The regression checks exact locked-C tree digests
at 2, 64, 128, 256, and 512 KiB.

Build and reproduce each revision from the repository root:

```sh
GOWORK=off go build -o /tmp/issue1362 ./cmd/issue454bench
for size in 64 128 256 512; do
  GOMAXPROCS=1 ISSUE454_FULLPROFILE=1 /usr/bin/time -v \
    /tmp/issue1362 c_sharp "$size" full 1
done
```

The unchanged counter ledger passes all 412 rows across 206 languages.
For the C# real-sample default row, full-parse nodes fall from 1,936 to 1,903;
edit nodes fall from 2,078 to 2,029; both peak-version counters fall from 23 to
21. No pinned expectation was changed.

## Verification scope

The C# parity regression and all runnable `TestCSharp` harness tests pass,
including the pinned dispatch digests. Two existing C# tests skip because
their optional grammar-source and recovered-corpus inputs are absent.
Receipt checks preserve every previously passing sample for C#, Elixir, PHP,
Markdown, and Kconfig. Existing failing receipt samples remain failing.

The edit-session invariant gate passes separately for all five certified
profiles affected by this change. The pinned incremental invariant ledger,
G6 merge-event census, recovery telemetry layout, narrow runtime profiles,
focused root/runtime/grammar tests, root `go vet ./...`, and API/root-file
budget checks pass.

An additional tagged harness `go vet` check fails identically on main and the
fixed revision: an existing corpus fixture references an undefined type, and
five existing tests copy lock-bearing values. This change does not resolve
those pre-existing diagnostics.

Randomized comparisons use `scripts/run_randomized_benchmarks.sh`, 20 seeds,
one process per seed, `GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`, and
`-benchmem`. The Go full/edit/no-edit trio and C# 64 KiB benchmark are paired
against the baseline checkout. Larger C# sizes use the fresh-process probes
above and exact C parity checks.

The final paired 64 KiB C# comparison drops from 2,746.1 to 124.9 ms/op
(-95.45%, p<0.001, 20 samples each). Allocated bytes fall from 135.277 to
1.453 MiB/op (-98.93%), and allocations fall from 195.46k to 11.43k/op
(-94.15%). These are steady benchmark measurements with parser reuse between
iterations, unlike the fresh-process probes above.

The final Go comparison reports full parse at 8.984 ms versus 9.159 ms
(+1.94%, p=0.021), single-byte edit at 203.2 versus 207.5 microseconds
(+2.12%, not significant, p=0.102), and no-edit at 8.868 versus 8.597 ns
(-3.06%, p=0.014). Allocation counts remain 8, 5, and 0 respectively;
no-edit allocates zero bytes. The small directional full/edit regressions
remain below the 10% timing ratchet. The tradeoff is bounded version retention
and a clean large-file result on the certified convergence profiles.

```sh
bash scripts/run_randomized_benchmarks.sh \
  --output /tmp/go-after.txt --baseline-root "$BASELINE_CHECKOUT" \
  --baseline-output /tmp/go-before.txt \
  --bench-regex '^BenchmarkGoParse(FullDFA|IncrementalSingleByteEditDFA|IncrementalNoEditDFA)$' \
  --require-benchmarks BenchmarkGoParseFullDFA,BenchmarkGoParseIncrementalSingleByteEditDFA,BenchmarkGoParseIncrementalNoEditDFA
cd cgo_harness
bash ../scripts/run_randomized_benchmarks.sh \
  --output /tmp/csharp-after.txt --baseline-root "$BASELINE_CHECKOUT/cgo_harness" \
  --baseline-output /tmp/csharp-before.txt --tags treesitter_c_parity \
  --package . --bench-regex '^BenchmarkCSharpLargeFileFull/^64KiB$'
```

Copy the new benchmark file into the baseline checkout before the comparison;
do not copy the engine changes. Run correctness and parity commands in the
project's isolated harness, one grammar at a time.
