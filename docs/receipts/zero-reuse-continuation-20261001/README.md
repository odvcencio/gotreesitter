# Reproducing the continuation receipt

The baseline engine is main `9148a96db0dcf95594d72671f48862d72bfee343`.
The final measured code is `2e96280330d44735fb37b1361f17dcf3d5e096be`.
The baseline checkout has only the benchmark test copied from the final
revision; its engine is unchanged. Raw files record the baseline as dirty
for that reason, and the measured final revision as clean. The earlier VM
receipt is historical; this receipt remeasures all five fixed languages
after the hot-layout repair on the workstation.

`summary.json` contains every paired sample, medians and ranges, allocations,
deterministic counter changes, protocol, and memory results. `census.json`
records all 206 pinned first edits, before/after counters, fallback reasons,
fixture and fresh-tree hashes, and invariant results. It is evidence, not
a replacement for existing ledger expectations or gate thresholds.
`correctness.txt` retains test names and results; original log hashes are
in `validation-log-hashes.json`. `tested-source.sha256` identifies the tested
engine, scanner, harness, diagnostic, and benchmark wrapper source.

Run heavy commands inside the Docker parity harness, serialized with the
workstation's shared heavy-work lock. Set GOWORK=off, GOMAXPROCS=1, and
GTS_PARITY_MODE=exhaustive. This campaign used the Go 1.25 image, one selected
CPU, 4 GiB container memory, and GOMEMLIMIT=3GiB. Timing uses 20 seeds,
750 ms per subbenchmark, count=1, and benchmem. The workstation is shared.
Changes under 5% are not claimed as speed improvements.

Create a detached baseline checkout at the hash above and copy only
`cgo_harness/zero_reuse_scanner_test.go` into its harness. Mount it at
`/baseline` and mount the final checkout at `/workspace`. If running guards
in a Git worktree, also make its Git common directory available inside
the container. Use an evidence mount at `/evidence` for the commands below.

Run correctness before timing, separately for lua, nickel, starlark,
properties, and firrtl. These commands execute inside Docker; replace the
language value each time:

```sh
export GOWORK=off GOMAXPROCS=1 GTS_PARITY_MODE=exhaustive
export GTS_ZERO_REUSE_LANGUAGE=properties
export GOTREESITTER_V1_INVARIANT_LANGUAGE=properties
cd /workspace
go test ./internal/incr -count=1
go test ./grammars/runtime -run '^TestLengthNeutralScannerCertificate$' -count=1
go test . -run '^TestV1InvariantGateR4EditSession$' -count=1
cd cgo_harness
go test -tags treesitter_c_parity . \
  -run '^TestLengthNeutralScannerReusePinnedSample/properties$|^TestLengthNeutralScannerInverseEdits$|^TestLengthNeutralScannerBudgetFallback$' -count=1
GTS_ZERO_REUSE_BYTES=140288 go test -tags treesitter_c_parity . \
  -run '^TestLengthNeutralScannerGenerated$' -count=1
```

Repeat generated correctness at 32768 and 1048576 bytes. At the root, run
`go run ./cmd/perfcounterledger --langs all` against unchanged expectations.
Build `./cmd/zeroreuse` once, get its `--list`, and invoke `--language` once
per listed grammar in sequence. Compare all fresh hashes against the earlier
receipt. The command verifies its committed sample/edit-session hashes,
incremental equality, root flags, coverage or explanatory stops, and no-edit
allocations. Unset the selected scanner language before the default package
race check:

```sh
cd /workspace
unset GTS_ZERO_REUSE_LANGUAGE
go test -race ./grammars/runtime -run '^TestLengthNeutralScannerCertificate$' -count=1
go test . -run '^Test(RecoveryMemoTelemetryPreservesAMD64HotLayouts|TokenInvariant)' -count=1
go run ./cmd/v1layout -base origin/main
go run ./cmd/v1guard -base origin/main
```

For pinned before/after comparisons, repeat this command once per language
from the final harness, with the language environment value set:

```sh
cd /workspace/cgo_harness
export GTS_ZERO_REUSE_LANGUAGE=properties
unset GTS_ZERO_REUSE_BYTES
bash ../scripts/run_randomized_benchmarks.sh \
  --package . --tags treesitter_c_parity \
  --bench-regex '^BenchmarkLengthNeutralScanner(CompleteEdit|Full|NoEdit)$' \
  --require-benchmarks 'BenchmarkLengthNeutralScannerCompleteEdit/Go1,BenchmarkLengthNeutralScannerCompleteEdit/C1,BenchmarkLengthNeutralScannerCompleteEdit/C2,BenchmarkLengthNeutralScannerCompleteEdit/Go2,BenchmarkLengthNeutralScannerFull,BenchmarkLengthNeutralScannerNoEdit' \
  --baseline-root /baseline/cgo_harness \
  --baseline-output /evidence/before.txt --output /evidence/after.txt
```

For 137 KiB, set GTS_ZERO_REUSE_BYTES=140288, select only
`^BenchmarkLengthNeutralScannerCompleteEdit$`, and require its four
subbenchmarks. Complete operations include Tree.Edit, parsing, and release.
For each seed average Go1/Go2 and C1/C2 separately, then divide and report
the median and range of the 20 paired ratios. Do not divide independently
pooled medians. Benchstat tables inspect each subbenchmark separately.

For the new languages' RSS probes, compile both cgo harness binaries outside
the measurement. Set GTS_ZERO_REUSE_BYTES=1048576 and invoke the selected
binary with `/usr/bin/time -v`, `-test.run '^$'`,
`-test.bench '^BenchmarkLengthNeutralScannerCompleteEdit$/^Go1$'`,
`-test.benchtime 1x`, and `-test.count 1`. Each process includes initial
parsing and the complete first edit. These are memory probes, not timing
comparison evidence. The earlier receipt retains the three carried
languages' VM memory results.

Broader real corpora, all edit sites and operation types, full per-language
graduation, and broad race sweeps are outside this receipt's validation.
