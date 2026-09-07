# Issue 454 compact route repair

Date: 2026-09-07.

## Summary

Issue [#454](https://github.com/odvcencio/gotreesitter/issues/454) compared
v0.52.0 with v0.48.1 on 137 KiB editor fixtures. Clean full parses ran 2 to 3
times slower on 41 of 48 grammars. Seven grammars lost most incremental reuse.
A single-byte Go delete did not terminate. All three items trace to the compact
candidate route, which serves fresh full parses by default. Pull request #631
removed the 64 KiB size decline, so files above 64 KiB reached that route for
the first time in v0.49.0.

This change keeps the compact route as the default and repairs the route.
It has three parts:

1. Remove avoidable per-token work from the compact scheduler.
2. Bound the recovery cost memo so compact error recovery stays linear.
3. Restore incremental reuse for compact-materialized old trees.

It also halts a production GLR stack at a no-action point when a sibling stack
accepts the lookahead. That rule fixes a C++ regression from pull request #709
that the compact default had masked. See the [route default report](issue-454-compact-route-default-2026-09-07.md)
for the original attribution and the downstream numbers.

## Environment and method

- linux/amd64 under Windows Subsystem for Linux, 20 logical CPUs, go1.25.1.
- Shared development host. Test lanes ran during some samples.
- Harness: `go run ./cmd/issue454bench <lang> 137 <full|replace|insert|delete>`.
  `GTS_ADMISSION_CANDIDATE=0` forces the production route; `=1` forces compact.
  `ISSUE454_CPUPROFILE=<path>` writes a CPU profile of the measured parses.
- Full parse: median of seven parses in one process.
- Every row is one process run. These are attribution samples, not paired benchmark evidence.

## Part 1: compact full parse

### Profile before the change

A CPU profile of twenty compact full parses of the 137 KiB Go fixture at
`a50f1532` attributed the time as follows.

| Cost center | Share | Avoidable |
| --- | ---: | --- |
| Memory-budget poll: exact footprint walk on every dispatch loop | 11.7% | yes |
| SHA-256 per external-scanner checkpoint intern and per relex probe (Scala, CMake) | 5% to 7% | yes |
| Large struct copies (`runtime.duffcopy`) | 16.7% | partly |
| Election, including lexing | 18.8% | no; lexing cost equals production |
| Reduction, condense, and pop paths in the compact core | 27% | no; core design |
| Materialization of the public tree | 14.5% | no; the compact route records then builds |
| Per-version relex on no-action headers | 6% | no; per-version lexing semantics |

### Changes

- The memory-budget poll now caches the last exact footprint. It reuses that
  value for up to 64 polls while the value, plus the caller's additional bytes,
  stays below half of the smallest armed threshold. From half the threshold
  upward every poll runs the exact walk, so the trip point near a budget does
  not move. The gauge resets at scheduler run start.
- The checkpoint interner compares a new serialization against the most
  recently interned record before hashing. Scanner state rarely changes
  between tokens, so most interns return without a digest.
- The per-version relex probe authenticates the election-start payload with a
  byte-exact comparison against its interned checkpoint instead of a fresh
  SHA-256 of the payload.
- The materialization walk passes each subtree record to its validator by
  pointer. The authenticated fast path returns before it reads the record.

### Results, 137 KiB full parse, median milliseconds

| Grammar | compact before | compact after | production | after / production |
| --- | ---: | ---: | ---: | ---: |
| go | 136.4 | 123.4 | 77.8 | 1.59 |
| rust | 140.4 | 115.1 | 67.8 | 1.70 |
| scala | 189.0 | 153.7 | 81.4 | 1.89 |
| cmake | 237.2 | 184.2 | 105.5 | 1.75 |
| toml | 94.5 | 78.9 | 51.1 | 1.54 |
| make | 91.4 | 72.6 | 51.9 | 1.40 |
| css | 79.2 | 74.7 | 51.5 | 1.45 |
| scss | 75.7 | 70.0 | 40.6 | 1.72 |
| typescript | 116.3 | 88.5 | 58.9 | 1.50 |
| ini | 74.2 | 61.0 | 36.6 | 1.67 |
| diff | 54.0 | 46.8 | 26.6 | 1.76 |
| json | 105.5 | 84.4 | 53.5 | 1.58 |
| hcl | 192.8 | 155.9 | 92.1 | 1.69 |

The compact route moves from 2.0 to 2.9 times production to 1.4 to 1.9 times.
The remaining gap is structural. The compact core records a derivation and
then materializes the public tree, so it pays for node construction twice,
and its reduction path condenses a boundary graph that production does not
keep. Closing that gap is campaign work, not a per-token cut.

Every row above matched the production tree by node count.

## Part 2: compact error recovery

### Root cause

A fresh compact parse of a Go file with one syntax error grew superlinearly
in file size. Two allocation defects in the recovery cost path compounded:

1. `RecoveryCostMemo.store` grew its two backing slices to the exact size the
   new `SubtreeID` needed. A compact parse stores a monotonically increasing
   id on almost every token, so nearly every store reallocated and copied the
   whole table: O(table size) per store and O(n^2) per parse.
2. `recoveryOutputCostFunc` and `s5RecoveryOutputCostFunc` allocated a new
   empty memo on every call, and every caller reset it immediately after. The
   memo never accumulated a hit, so each token re-priced the subtree from
   scratch.

### Changes

- `store` grows the memo geometrically with a floor of 64 slots.
- The scheduler owns one `RecoveryCostMemo` for the life of a parse. A
  published subtree's cost is immutable for the rest of the parse, so the
  shared memo preserves every returned cost. The per-reduction resets are
  removed.

### Results, Go single-byte delete on the compact route, milliseconds

| Size | fresh before | fresh after | incremental after | production incremental |
| --- | ---: | ---: | ---: | ---: |
| 4 KiB | 137 | 15 | 7 | 1.2 |
| 8 KiB | 698 | 27 | 14 | 2.1 |
| 16 KiB | 4,881 | 49 | 21 | 2.2 |
| 32 KiB | about 34,700 (extrapolated) | 93 | 41 | |
| 64 KiB | no termination | 179 | 87 | |
| 137 KiB | no termination in 420 s (downstream) | 438 | 178 | 15 |

The compact route now scales linearly, and every incremental tree equals the
fresh tree by S-expression. The remaining gap to production is structural:
the compact incremental recovery route runs a fresh compact recovery parse
and does not borrow old-tree nodes, so a transient-error keystroke costs one
compact recovery parse. Production reuses the unchanged siblings and reparses
only the local region. The fresh number also includes the accepted-error retry
ladder, which both routes pay.

## Part 3: incremental reuse on compact-materialized trees

### Root causes

1. `tryReuseSubtree` rejected every compact-materialized top-level candidate
   whose recorded pre-goto state differed from the live state, while
   production candidates continue under the established compatible-goto
   contract. Compact materialization records the same pre-goto and parse
   states production records for the same span, and the recovery-bearing and
   unproven cases are already excluded earlier in the same loop. The extra
   rejection left compact old trees with leaf-only reuse: 10,447 root
   non-leaf rejects on the 137 KiB TOML fixture.
2. `compactTreeIncrementalReuseProven` required the root to carry compact
   proof flags. When trailing extras follow the accepted root payload, result
   building synthesizes a fresh root with no compact flags, so every INI tree
   that ends in a blank line disabled reuse for the whole tree and reported a
   scanner-quiescence reason for a grammar with no external scanner.
3. The compact incremental route committed to a whole-file compact parse
   before it knew whether any subtree could be reused, then declined. INI and
   JSON paid a full compact parse per keystroke before the production path
   ran.

### Changes

- Compact-materialized candidates use the compatible-goto contract.
  Error-bearing compact trees stay strict through `strictTopLevelOwnership`.
- The root is exempt from the whole-tree proof. The reuse cursor reuses the
  root only on a byte-identical undo, which needs no state proof.
- The unsupported-reuse reason names the clause that failed: table-replay
  proof, unproven visible node, or the scanner gate.
- The compact incremental attempt declines after eight clean in-scope
  candidates that it cannot authenticate, or once the shared token has
  advanced 32 KiB past the last edit with zero reuse. Both are bounds on
  wasted work, not reuse gates.

### Results, 137 KiB near-top insert, compact old tree

| Grammar | before | after | production old tree |
| --- | --- | --- | --- |
| toml | 62.5%, 61.1 ms | 100.0%, 3.5 ms | 100.0%, 3.3 ms |
| ini | 0.0%, 58.2 ms | 97.3%, 14.0 ms | 97.3%, 17.1 ms |
| make | 29.9%, 56.1 ms | 100.0%, 11.5 ms | 100.0%, 8.2 ms |
| typescript | 54.3%, 67.9 ms | 98.1%, 5.8 ms | 98.1%, 5.5 ms |
| tsx | 54.3%, 71.3 ms | 98.1%, 6.6 ms | |
| css (delete) | 68.0%, 26.2 ms | 95.1%, 4.9 ms | 95.1%, 3.9 ms |
| scss | 56.5%, 26.6 ms | 95.1%, 4.4 ms | 95.1%, 8.0 ms |
| diff | 25.1%, 36.4 ms | 100.0%, 2.2 ms | 99.9%, 2.6 ms |
| json | 69.2%, 203.9 ms | 69.2%, 75.8 ms | 69.2%, 70.2 ms |
| go | 97.5%, 10.1 ms | 97.5%, 9.6 ms | 97.5%, 6.1 ms |

Every row matches the fresh parse by S-expression.
`TestCompactOldTreeKeepsTopLevelIncrementalReuse` pins toml, ini, make, diff,
and typescript on the compact route at 48 KiB.

## Remaining items outside this change

- Transient-error keystrokes on the compact route cost one compact recovery
  parse (178 ms incremental at 137 KiB for Go) against 11 ms at v0.48.1 on
  production reuse. Borrowing clean old-tree subtrees into compact recovery is
  the next step.
- TypeScript transient-error delete costs 300 ms on either route since pull
  request #613 added the memory-budget fail-closed retry; 64 ms at v0.48.1.
- C transient-error delete at 137 KiB still explores about 3.2 million nodes
  before the memory-budget retry (2.9 s). The C fallback attribution work owns it.
- Compact clean full parse stays 1.4 to 1.9 times production. The remaining
  cost is the compact core's condense and reduction path plus materialization.
- Eight tagged scheduler tests fail identically on origin/main
  (`TestDiagnosticParserCoreGenericScheduler*`, `TestDiagnosticParserCoreConflict*`,
  `TestDiagnosticParserCoreStateDependentRelexKeepsExactSpanBranch`), as does
  `TestResultCompatibilityRetiredCommitProvenance`. This change does not touch them.
