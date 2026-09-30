# Certified scanner reuse receipt

The Properties scanner now authenticates equal-width ASCII substitutions and
reuses the whole tree when the bounded lexer proof also succeeds. Its only
input distinction is EOF versus a nonzero rune; the scanner consumes no bytes.
The proof covers both incoming EOF-latch states, both valid-symbol masks, every
nonzero ASCII byte, the emitted symbol, cursor, marks, read frontier, and outgoing
serialized state. EOF, NUL, and non-ASCII bytes do not receive this proof.

The existing dependency verifier still authenticates internal lexer probes and
discarded lookahead. Source-sensitive compatibility rewrites still abstain.
General scanner subtree reuse remains disabled. This avoids a fresh verification
parse without broadening replay beyond the proven substitution.

The scanner certification harness comes from commits `6a27079c2` and
`0f2c26dd6`. It tests Bash, Blade, Properties, C#, and PowerShell replay and
rejects deliberately faulty serialization, hidden state, and failure behavior.

## Fixed workload and results

Baseline: `f9828512c`. Candidate: the accompanying Properties proof change.
Generated Properties input: 140,298 bytes; one equal-width ASCII replacement
at the midpoint. Original SHA-256:
`782512614f6a080a1f6ab89e68b5d1d5ce2a9df857f5312999f7320f5dc26468`.
Edited SHA-256:
`9b11b908d5b6f64339e2734923be3797a0d8fd7bbe7cef30d3c296bced58cceb`.
The complete edit operation includes `Tree.Edit`, incremental parsing, and
release of the old tree. The initial parse is outside the timer for both engines.

METRIC: Properties edit tokens | 140300 -> 0 | f9828512c -> accompanying change | generated 140298-byte midpoint ASCII edit
METRIC: Properties edit nodes | 288454 -> 0 | f9828512c -> accompanying change | generated 140298-byte midpoint ASCII edit
METRIC: Properties reused bytes | 0 -> 140298 | f9828512c -> accompanying change | generated 140298-byte midpoint ASCII edit
METRIC: Properties complete edit ns/op | 206396292 -> 37695 | f9828512c -> accompanying change | generated 140298-byte midpoint ASCII edit, 20 shuffle seeds

| Measurement | Before | After |
| --- | ---: | ---: |
| Complete Go edit, first group, ns/op | 206,396,292 | 37,695 |
| Complete Go edit, second group, ns/op | 205,358,892 | 37,492 |
| Complete Go edit, B/op | 1,332 | 384 |
| Complete Go edit, allocs/op | 9 | 5 |
| C edit, first group, ns/op | 138,516,227 | 137,223,838 |
| C edit, second group, ns/op | 138,003,358 | 137,254,262 |
| Properties full parse, ns/op | 202,911,378 | 202,776,112 |
| Properties full parse, B/op / allocs/op | 1,256 / 9 | 1,256 / 9 |
| Go full-parse control, ns/op | 9,009,000 | 8,878,000 |
| Go single-byte edit control, ns/op | 199,200 | 186,900 |
| Go no-edit control, ns/op | 8.543 | 8.976 |
| Go no-edit control, B/op / allocs/op | 0 / 0 | 0 / 0 |

All timings use `scripts/run_randomized_benchmarks.sh`, paired alternating
baseline/head processes, seeds 1 through 20, `GOWORK=off`, `GOMAXPROCS=1`,
`-count=1`, `-benchtime=750ms`, `-benchmem`, and one pinned CPU in Docker.
The complete-operation Go/C ratio moves from about 1.49 to 0.000274.
Properties and Go full parses show no significant timing change. The Go edit
control improves 6.18%; the no-edit control rises 5.07% (0.433 ns). No-edit
executes the unchanged early-return path; its work and allocation counters stay
zero. Record this directional timing change rather than hiding it.

The C module uses the locked 0.27 runtime revision `6070dbfe`, from the oracle
pin introduced in `3a0e7df5`, through an external module replacement. The runtime
pin and corpus lock are not changed by this work.

The generated 1 MiB Properties input is 1,048,577 bytes. A cold single-operation
RSS probe using an already-built test binary records 563,456 KiB before and
326,204 KiB after. Cold edit bytes fall from 289,100,352 to 32,752, while cold
allocations rise from 80 to 471 because the first dependency proof builds shared
lexer tables. These cold observations are separate from the warmed randomized
comparison and are not used as timing evidence.

## Validation

All checks ran in Docker, one grammar per process:

- Exhaustive Properties ASCII scanner proof and certification fault contracts.
- Properties incremental/fresh deep equality at 4 KiB, 137 KiB, and 1 MiB,
  legacy and compact routes, including locked-C deep digest equality over four
  edits, complete root coverage, ERROR/HasError consistency, and zero-allocation
  no-edit reparses.
- Existing token-invariant, incremental-leaf, scanner-policy, arena-release,
  and parser-layout tests.
- Seventy-two-step edit-session gates for Properties, Mojo, Starlark, Julia,
  and TypeScript, each in a separate process.
- Isolated race coverage for the Properties proof, 4 KiB reuse witness,
  token-invariant tests, and parser layout.
- Twenty-seed Properties full/edit and standard Go benchmark controls.

## Rejected candidate and remaining work

Enabling checkpoint-based Properties subtree reuse alone passes small parity
cases but retains a fresh frontier verification parse. The first 20-seed
candidate worsened the complete edit from 207,212,401 to 258,457,638 ns/op
(24.73%), bytes from 1,332 to 4,707,272, and allocations from 9 to 87. It was
rejected. The shipped proof leaves checkpoint collection disabled.

The C# 1 MiB witness still declines compact admission and reparses with zero
reuse because its stateful scanner has no accepted incremental capability.
Bash and Blade have replay certification but no admitted reuse proof yet.
PowerShell EOF appends remain the next boundary to authenticate. The original
overseer receipt directory was absent, so the baseline was regenerated from the
recorded revision. The fleet census was not rerun; this receipt proves one
previously zero-reuse workload now reuses, and does not claim a new 206-grammar
census total. No pins, expectations, thresholds, or tests were removed or relaxed.
