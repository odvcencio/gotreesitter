# C# scanner reuse receipt

C# equal-width lowercase substitutions inside ASCII identifiers ending in a
digit now reuse the complete tree after the existing bounded lexer dependency
proof succeeds. This covers the headline `x0` to `y0` edit. Every scanner string
and comment path treats those letters alike. Every suffix the lambda modifier
recognizer can read still ends in a digit, so none can become `ref`, `out`, `in`,
`readonly`, or `scoped`. Scanner cursor, marks, symbol, read frontier, and
outgoing serialized state remain equal for every incoming payload.

The scanner implements a grammar-owned substitution proof. The engine uses
that proof and its existing identifier source-semantics check without requiring
a forest-built old tree. Keyword checks remain in place. Internal lexer probes,
discarded lookahead, and keyword selection still pass the bounded verifier.
Unproved edits retain fresh fallback. General scanner checkpoint reuse remains
disabled; the stateful scanner still declines compact full admission.

## Fixed workload

Baseline: `f9828512c`. Candidate: the accompanying C# proof change, on top of
`253a21b0c` and the independent recovery-walk fix. Generated source requests
1 MiB and produces 1,048,645 bytes. The first `x0` identifier alternates with
`y0`. Edited SHA-256:
`51cca2396943765c15ba7a9ee5840493e544a4abe653f1b8fbb8466bd33786f5`.
Each timed operation includes `Tree.Edit`, incremental parsing, and release of
the previous tree. Initial parsing is outside both engines' timers.

METRIC: C# edit tokens | 310666 -> 0 | f9828512c -> accompanying change | generated 1048645-byte x0/y0 edit
METRIC: C# edit nodes | 1978947 -> 0 | f9828512c -> accompanying change | generated 1048645-byte x0/y0 edit
METRIC: C# reused bytes | 0 -> 1048645 | f9828512c -> accompanying change | generated 1048645-byte x0/y0 edit
METRIC: C# complete edit ns/op | 4447412300 -> 186600 | f9828512c -> accompanying change | generated 1048645-byte x0/y0 edit, 20 shuffle seeds
METRIC: C# complete-operation Go/C | 92.7383x -> 0.0038501x | f9828512c -> accompanying change | generated 1048645-byte x0/y0 edit, paired 20-seed ratios

| Measurement | Before | After |
| --- | ---: | ---: |
| Go complete edit, first group | 4.4474 s | 186.6 us |
| Go complete edit, second group | 4.4819 s | 187.2 us |
| Go complete edit, B/op | 398,943,896 | 408 |
| Go complete edit, allocs/op | 172,808 | 5 |
| C complete edit, first group | 47.22 ms | 47.61 ms |
| C complete edit, second group | 47.38 ms | 48.63 ms |
| C# full-parse control, 140,356 bytes | 1.129 s | 1.175 s |
| C# full-parse control, B/op | 1.942 MiB | 1.942 MiB |
| C# full-parse control, allocs/op | 23,910 | 23,910 |
| Go full-parse control | 8.966 ms | 8.924 ms |
| Go single-byte edit control | 200.5 us | 200.3 us |
| Go no-edit control | 8.741 ns | 8.889 ns |

The complete edit, C# full parse, and Go controls each use 20 seeds through
`scripts/run_randomized_benchmarks.sh`: paired alternating baseline/head
processes, `GOWORK=off`, `GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`,
`-benchmem`, and pinned CPUs in Docker. Go/C is the median of the per-seed
paired Go/C group averages. C uses the locked 0.27 runtime revision `6070dbfe`
through the oracle pin introduced in `3a0e7df5`; no runtime pin changes here.
Timing varies across the shared machine, so the owner's earlier 194x ratio is
not substituted for the newly measured baseline. C controls show no significant
change. The C# full-parse control rises about 4.1% (p=0.076), below the 10% ratchet;
record this directional result. The final Go controls have no significant timing
change and preserve 8/5/0 allocation counts for full/edit/no-edit operations.

An already-built, cold one-operation RSS probe falls from 792,664 KiB to
430,956 KiB. This includes initial load. The latter is about 421 bytes per
source byte and still exceeds the 400-byte floor; the baseline already exceeds
that floor. This work does not graduate C# or claim the initial-load memory
failure is resolved. Cold first-proof allocations fall from 172,808 to 471 and
bytes from 398,943,896 to 32,768. The warmed randomized row is the comparison
receipt; the cold probe is separate RSS evidence.

## Validation and scope

Docker checks pass, one grammar per process:

- All 8,192 external-symbol masks over six valid incoming serialized states,
  six string/comment/lambda contexts, and every origin up to the edited byte.
  Reject punctuation, changed identifier tails, width changes, uppercase
  substitutions, and adjacent non-ASCII identifiers.
- Four alternating edits at 4 KiB, 137 KiB, and 1 MiB, both requested parser
  routes, with incremental/fresh/locked-C deep digest equality, complete coverage,
  ERROR/HasError consistency, and zero-allocation no-edit reparses.
- Existing token-invariant, scanner fault, compact dependency, borrowed-tree,
  parser-layout, and arena ownership tests.
- Seventy-two-step R4 gates for C#, Properties, PowerShell, Go, TypeScript,
  and Julia, each in a separate process.
- Isolated race coverage for the scanner proof, token-invariant paths,
  Properties proof, compact dependency publication, and borrowed trees.

No gate, pin, expectation, node layout, parser layout, or language graduation
allowlist changes. The fleet census has not been rerun. Bash and Blade have
replay certification but still need an admitted incremental boundary proof.
