# Reproducing the zero-reuse receipt

The baseline is `9148a96db0dcf95594d72671f48862d72bfee343`. The measured
implementation is `f230db4a140240fb54f7e1eac313efacb357af19`, including
dependency fix `d10ce302f4163faad82b30a6818f7a848245185c`. The receipt commit
adds evidence only. Existing pins, grammar locks, and gate thresholds stay
unchanged.

`summary.json` contains paired per-seed medians, ranges, deterministic counter
changes, and RSS measurements. `census.json` records all 206 pinned first
edits, scanner support, fallback reasons, source hashes, and fresh-tree
digests. Raw benchmark files retain all 20 samples and completion markers.
The filtered test logs retain test names, results, and deterministic metrics;
their original log hashes are recorded in the summary.

Run every heavy command inside `cgo_harness/docker/run_parity_in_docker.sh`,
with `GOWORK=off`, `GOMAXPROCS=1`, and one grammar per process. Set
`GTS_PARITY_MODE=exhaustive` for C comparisons. The original runs used the
Go 1.25 harness image, CPU 6, and its default memory budget. The VM was
shared with other work. Do not interpret changes under 5% as speed gains.

Build the census command once, then run one language per process from the
repository root:

```sh
export GOWORK=off GOMAXPROCS=1
go build -o /tmp/zeroreuse ./cmd/zeroreuse
/tmp/zeroreuse --list > /tmp/zero-reuse-languages.txt
while read -r grammar; do
  /tmp/zeroreuse --language "$grammar"
done < /tmp/zero-reuse-languages.txt
```

The command checks the committed sample and editing-session hashes and fails
when incremental parsing differs from fresh Go parsing. For the baseline,
copy the diagnostic command to a detached baseline checkout without changing
its engine. The command also checks `ERROR` root flags, coverage or an
explanatory stop reason, and zero allocations on a no-edit reparse. The
deterministic default/candidate ledger is a separate gate: run
`go run ./cmd/perfcounterledger --langs all` inside Docker against its
unchanged checked-in expectations. Its children use one grammar per process.

Run these focused correctness checks separately for `lua`, `nickel`, and
`starlark`, replacing the environment value each time:

```sh
export GOWORK=off GOMAXPROCS=1 GTS_ZERO_REUSE_LANGUAGE=lua
go test ./internal/incr -count=1
go test ./grammars/runtime -run '^TestLengthNeutralScannerCertificate$' -count=1
GOTREESITTER_V1_INVARIANT_LANGUAGE=lua go test . \
  -run '^TestV1InvariantGateR4EditSession$' -count=1
cd cgo_harness
go test -tags treesitter_c_parity . \
  -run '^TestLengthNeutralScannerReusePinnedSample/lua$' -count=1
go test -tags treesitter_c_parity . \
  -run '^TestLengthNeutralScannerInverseEdits$' -count=1
GTS_ZERO_REUSE_BYTES=140288 go test -tags treesitter_c_parity . \
  -run '^TestLengthNeutralScannerGenerated$' -count=1
```

Repeat the generated check with 32768 and 1048576 bytes. Run the budget and
nonterminal fallback tests without increasing proof budgets. Scanner
certificates and `internal/incr` also passed container race checks. Full
external corpora, broad race sweeps, and language graduation were not run.

For comparison, create a detached baseline checkout and copy only
`cgo_harness/zero_reuse_scanner_test.go` into its harness. The benchmark calls
the existing public API on both revisions. Mount the baseline as `/baseline`
and an evidence directory as `/evidence`, then run from the changed harness:

```sh
export GOWORK=off GOMAXPROCS=1 GTS_ZERO_REUSE_LANGUAGE=lua
unset GTS_ZERO_REUSE_BYTES
bash ../scripts/run_randomized_benchmarks.sh \
  --package . --tags treesitter_c_parity \
  --bench-regex '^BenchmarkLengthNeutralScanner(CompleteEdit|Full|NoEdit)$' \
  --require-benchmarks 'BenchmarkLengthNeutralScannerCompleteEdit/Go1,BenchmarkLengthNeutralScannerCompleteEdit/C1,BenchmarkLengthNeutralScannerCompleteEdit/C2,BenchmarkLengthNeutralScannerCompleteEdit/Go2,BenchmarkLengthNeutralScannerFull,BenchmarkLengthNeutralScannerNoEdit' \
  --baseline-root /baseline/cgo_harness \
  --baseline-output /evidence/before.txt --output /evidence/after.txt
```

For 137 KiB, set `GTS_ZERO_REUSE_BYTES=140288`, select only
`^BenchmarkLengthNeutralScannerCompleteEdit$`, and require its four
subbenchmarks. Complete operations include editing, reparsing, and release.
For each seed, average Go1/Go2 and C1/C2 separately, then divide; report the
median and spread of those 20 paired ratios. Use `benchstat` on the raw files
to inspect each subbenchmark separately.

The dependency benchmark uses the same wrapper on the root package with
`--bench-regex '^BenchmarkNestedErrorDependency$'`. Copy its benchmark-only
function into the baseline's existing test file. It exercises the original
dependency implementation, without the new regression test.

For RSS, compile each harness outside `/usr/bin/time -v`, then execute only
`BenchmarkLengthNeutralScannerCompleteEdit/Go1` with
`GTS_ZERO_REUSE_BYTES=1048576`, `-benchtime=1x`, and `-count=1`. These are
memory probes, not timing comparisons. The process includes initial parsing
and the complete first edit. Correctness checks retain additional fresh Go
and C trees and deep digests, so their harness RSS measures a different
workload.
