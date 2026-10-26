# Preserve hidden newline ownership before a generated NUL sentinel

The generic zero-width start-accept override could replace an already lexed,
parser-valid hidden newline with an empty NUL token. The statement list then
stopped one byte before its C boundary. NUL tokens now use the existing
boundary-aware sentinel rule, preserving the consuming token, its DFA
provenance and its dependency frontier.

This is an output/compat correctness change. All 206 registered grammars were
inspected; none can enter the changed branch. It fixes generated grammars that
use NUL terminators and enables further work on Go's internal semicolon lexing.
The shipped Go grammar continues to use its existing semicolon scanner.

Baseline: `afd16867b666dfec05f3eb28e9eee86036474532`.

## Correctness

The regression derives an internal-terminator variant from the owned Go DSL,
replacing its external semicolon with a newline-or-NUL alternative. The locked
C Go grammar is `2346a3ab1bb3857b48b29d779a1ef9799a248cd7`.
For the 65-byte function fixture, the statement list ended at byte 62 before
the fix. C and the corrected runtime end it at byte 63, including the newline.
Both new regression tests fail on unchanged main and pass with the fix.

- Ten new complete Go/C tree comparisons pass in default and candidate
  configurations, including edited incremental and independent fresh trees.
  Six generated-grammar no-edit cases measure zero allocations.
- Existing Go compact certification, recovery, reuse and numeric parity checks,
  and standard fresh/incremental/error parity in both configurations, pass.
- Linux amd64 and native ARM64 each pass four Go/YAML R4 sessions: 288 edits per
  architecture, incremental equals fresh at every step, no-edit allocates zero.
  Both architectures pass 20 memory-budget tests.
- The [206-language sentinel inventory](sentinel-inventory.jsonl) finds zero
  shipped grammars entering the new branch.

See [exact-tree evidence](correctness.txt) and
[qualification metadata](qualification.json). Correctness used isolated
one-grammar Docker workloads. Linux used Go 1.26.6; the Mac used native
linux/arm64 Go 1.25.14. These are separate validation environments.

## Deterministic work

The complete Go/YAML ledgers match exactly before and after on both machines.
These values apply to both revisions. Multi-version shares, stop reasons,
coverage and error flags also match in the raw [before](before-ledger.json)
and [after](after-ledger.json) receipts.

| Language | Configured route | Operation | Tokens | New nodes | Max versions | Reused bytes | Block splices |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: |
| go | candidate | full | 589 | 1251 | 3 | 0 | 0 |
| go | candidate | first edit | 374 | 1553 | 8 | 1784 | 266 |
| go | default | full | 589 | 1922 | 8 | 0 | 0 |
| go | default | first edit | 572 | 3810 | 12 | 1166 | 296 |
| yaml | default | full | 107 | 288 | 1 | 0 | 0 |
| yaml | default | first edit | 107 | 480 | 1 | 0 | 0 |

The YAML candidate row retains its existing alternative-set coverage decline.
Its unchanged routing receipt is included in both JSON files.

These are selected-route counters. They omit the small-source fresh verifier
identified in [Cogno's October 21 report](https://github.com/odvcencio/gotreesitter/issues/454#issuecomment-6311551850).
The primary trio below is a performance safety check; that report's
span-changing edit throughput blocker remains open.

## Randomized performance safety

Work counters passed before timing. The M1 Max ran 20 matched seeds through
`scripts/run_randomized_benchmarks.sh --primary-go-suite`, alternating execution
order, with `GOMAXPROCS=1`, one process per seed, `-count=1`,
`-benchtime=750ms` and `-benchmem`.

| Benchmark | Before | After | Time significance |
| --- | ---: | ---: | --- |
| Full parse | 5.864 ms | 5.797 ms | p=0.429, unchanged |
| Single-byte edit | 208.2 µs | 206.5 µs | p=0.659, unchanged |
| No edit | 6.163 ns | 6.175 ns | p=0.482, unchanged |

Allocated bytes and allocations are statistically unchanged. No edit remains
0 B/op and 0 allocs/op. Full parse remains 6 allocs/op; single-byte edit remains
5. Timing variation is substantial, so these samples support no throughput claim.

Raw samples: [before](baseline-primary-arm64.txt),
[after](candidate-primary-arm64.txt), [benchstat](benchstat-arm64.txt).
Source manifests stayed unchanged during the Mac workloads; containers
completed without an OOM kill. This receipt does not clear the separate memory
or CPU holds on the other optimization PRs.

## Reproduce

Run the focused tests through the repository's Docker harness:

```sh
bash cgo_harness/docker/run_parity_in_docker.sh -- \
  "cd /workspace && go test . -tags gts_parsercorephase0 -run '^TestNextDFAToken.*(ZeroWidth|NUL|HiddenNewline)' -count=1"
bash cgo_harness/docker/run_parity_in_docker.sh -- \
  "cd /workspace/cgo_harness && go test . -tags treesitter_c_parity,gts_parsercorephase0 -run '^TestGoGeneratedNULPreservesHiddenNewlineLockedC$' -count=1"
```

Use separate baseline and candidate source checkouts in the same isolated
environment for the comparison:

```sh
GOMAXPROCS=1 bash scripts/run_randomized_benchmarks.sh \
  --primary-go-suite --baseline-root "$BASELINE_ROOT" \
  --baseline-output baseline-primary.txt --output candidate-primary.txt \
  --runs 20 --benchtime 750ms
benchstat baseline-primary.txt candidate-primary.txt
```
