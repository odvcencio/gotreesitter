# Incremental proof cost, 2026-09-30

The first change reduces lexical proof work without changing reuse admission.
It replaces repeated linear searches for duplicate lexer modes with bounded
scratch that preserves the original order and 1,024-mode capacity. DFA probes
return the six cursor fields the proof compares, rather than copying the entire
lexer back to the caller. The 32,768-byte and 2,048-scan budgets stay unchanged.

## First paired campaign

Baseline: `f9828512c`. Both versions ran 20 alternating paired shuffle seeds,
one process per seed, `GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`, and
`-benchmem`, through `scripts/run_randomized_benchmarks.sh` inside Docker.
The CPU was pinned, but other campaigns shared the host. These are local smoke
measurements, not a sealed quiet-host receipt. The requested isolated runner
could not be started because the available cloud credentials lacked the
required authentication scope.

| Control workload | Before | After | Change |
| --- | ---: | ---: | ---: |
| Go single-byte edit | 618.0 us | 345.1 us | -44.16%, p < 0.001 |
| Go full parse | 30.23 ms | 31.23 ms | +3.3%, p = 0.820 |
| Go no-edit reparse | 28.68 ns | 29.59 ns | +3.2%, p = 0.565 |
| Edit bytes allocated | 398 B | 392 B | -1.51% |
| Edit allocations | 5 | 5 | unchanged |
| No-edit allocations | 0 | 0 | unchanged |

The edit samples have a 45% baseline and 15% candidate confidence interval.
The full-parse and no-edit directional increases are recorded above; neither
was statistically significant. Those paths do not execute the changed proof.
An earlier prototype campaign measured 291.7 us to 164.4 us (-43.64%), with
even wider variation. Only the final campaign is retained as comparison data.

Raw records: [before](benchmarks/verify-cost-modes-before.txt) and
[after](benchmarks/verify-cost-modes-after.txt). Container source metadata was
informational; the baseline revision and source manifests below identify the
code used. This campaign is not an authenticated release receipt.

Candidate source SHA-256:

```text
229e120813048e0f9912eda89ef2967c8e3f064ec31f49691f9857f88251f981  token_invariant_lexical_proof.go
74b23d12f8a8db494e6505f8fccf3bffe6c1fa55da1505604e78139603894a51  internal/incr/lexproof/modes.go
a1416c93bfbc7091c2ecf81ebb3ee8b1a0d2251e078081f263de7f7a15427cf7  internal/incr/lexproof/cursor.go
1c12ad157c75483b092259f6426dd00449b60a5f39d55540b3e022929f49b245  cgo_harness/incremental_verify_cost_test.go
```

## Correctness and deterministic work

Docker checks passed for primitive proof budgets, keyword and whitespace
dependencies, UTF-8 rollback, compact reuse, edit coordinate handling, scanner
certification, memory budgets, and the pinned hot layouts. Parser size remains
2,328 bytes. Locked C checks passed for Go numeric repairs and reuse,
TypeScript numeric edits and syntax changes, Julia comment edits, and
PowerShell EOF commands. Each grammar ran in a separate test process.

The complete counter ledger passed all 412 rows for 206 languages. No pins or
thresholds changed. The generated 140,368-byte Go edit session also preserved
its exact work and reuse counters:

| First edit counter | Before | After |
| --- | ---: | ---: |
| Parse attempts | 2 | 2 |
| Shifts | 72,200 | 72,200 |
| Reductions | 75,729 | 75,729 |
| Action lookups | 359,605 | 359,605 |
| Raw lexer calls | 59,875 | 59,875 |
| New nodes | 3,599 | 3,599 |
| Reused subtrees | 1,770 | 1,770 |
| Reused bytes | 136,788 | 136,788 |

Two 206-language invariant sweeps, before and after memoization, have the same
198 passes and eight failures. The awk, JavaScript, meson, and twig digest
mismatches reproduce exactly on the unmodified baseline. Elsa, Haskell,
Kotlin, and PowerShell time out after ten minutes on both revisions. No gate
was changed. Do not read the focused green checks as fleet graduation.

## Requested large workloads

`TestIncrementalVerifyCost` compares every session step with fresh Go and the
locked C deep digest, checks root coverage and the ERROR-root invariant, and
requires zero allocations for an unchanged reparse. The fixtures are generated
and identify the intended size and edit location, rather than claiming to be
the owner's original real-code files.

* PowerShell: 15,390 bytes, unquoted `Write-Output x` typed at EOF, using the
  compact admission route. Every intermediate step passes Go/fresh/C parity.
  The route still falls back with `eof_append_fresh` and reuses zero bytes.
* C#: 1,048,645 bytes, alternating one-byte identifier edits. All four steps
  pass Go/fresh/C parity. Both versions consume 310,666 tokens, allocate
  1,978,947 nodes, and reuse zero bytes, with
  `external_scanner_unsupported`. A baseline profiled first edit took 4.35 s;
  that single observation is attribution evidence, not a timing comparison.
* Go: 140,368 bytes, alternating identifier edits. All four steps pass
  Go/fresh/C parity. This case still runs a fresh legacy verifier. The
  neighboring C reuse lane owns retiring that verifier.

These fallback paths do not execute the optimized lexical proof. This change
does not establish an O(edit) bound for those full edit operations.

## Rejected candidates and limitations

An exact unchanged-prefix/suffix cache reduced overlapping byte comparisons,
but enlarged the pinned Parser from 2,328 to 2,376 bytes. It was dropped. An
earlier form also accepted a replaced source buffer; binding the proof to
buffer identity fixed that test before the layout rejection.

The unmodified baseline's default PowerShell admission route fails the first
EOF typing step. Under compact admission, typing an opening quote also exposes
a fresh-Go versus C malformed-input recovery mismatch. The committed typing
witness uses an unquoted command; the existing EOF regressions remain enabled.
No gate, expectation, or pin was relaxed to accept these cases.

To reproduce the new workload checks, enter `cgo_harness` inside the parity
Docker runner and run one language at a time:

```sh
GOWORK=off GOMAXPROCS=1 go test -tags treesitter_c_parity . \
  -run '^TestIncrementalVerifyCost/powershell$' -count=1 -v
```

Use `c_sharp` or `go` for the other witnesses. Add `gts_workcount` to the build
tags to print deterministic parser work. Use the randomized benchmark wrapper
with package `.` from `cgo_harness`, tags `treesitter_c_parity`, and benchmark
`^BenchmarkIncrementalVerifyCost/<language>/(Go|C)$` for paired measurements.
