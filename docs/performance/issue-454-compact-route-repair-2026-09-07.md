# Issue 454 compact route repair

Date: 2026-09-07.

Route note, same day: the production engine is the default fresh route after
this report. See the
[production route decision](issue-454-production-route-decision-2026-09-07.md).
The repairs below stand on the candidate lane.

## Summary

Issue [#454](https://github.com/odvcencio/gotreesitter/issues/454) compared
v0.52.0 with v0.48.1 on 137 KiB editor fixtures. Clean full parses ran 2 to 3
times slower on 41 of 48 grammars. Seven grammars lost most incremental reuse.
A single-byte Go delete did not terminate. All three items trace to the compact
candidate route, which serves fresh full parses by default. Pull request #631
removed the 64 KiB size decline, so files above 64 KiB reached that route for
the first time in v0.49.0.

This change keeps the compact route as the default and repairs the route.
It has five parts:

1. Remove avoidable per-token work from the compact scheduler.
2. Bound the recovery cost memo so compact error recovery stays linear.
3. Restore incremental reuse for compact-materialized old trees.
4. Return transient-error keystrokes to v0.48.1 parity.
5. Cut the production engine's own drift since v0.48.1, which both routes
   inherit.

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

## Part 4: keystroke parity with v0.48.1

### Transient-error keystrokes

The first repair left a transient-error keystroke on the compact route at one
fresh compact recovery parse (178 ms at 137 KiB for Go against 11 ms at
v0.48.1). The measurement that decided the next step compared, for a delete
that leaves a syntax error, the production incremental tree on a compact old
tree against a fresh compact parse of the edited bytes:

- Mid-file errors, 14 grammars at 137 KiB and 32 KiB: identical S-expressions
  for 13. Scala differed because production's own fresh parse disagrees with
  its incremental parse on that input, and both take 2.4 s.
- Unbalanced mid-file edits (a deleted closing bracket): identical for all 12
  measured grammars at 137 KiB.
- Edits at end of file: identical for 13 of 14. JavaScript differed: the
  compact route's certified end-of-file recovery keeps the trailing function
  that production wraps in ERROR.
- The compact recovery route never produced the tree for these inputs. It
  declined after running to end of file, and production served the edit after
  the discarded pass.

The incremental entry point now serves a recovery-declined borrow attempt with
production reuse, which is the v0.48.1 mechanism, unless the edit reaches
within 256 bytes of the end of the source or the old tree cannot be reused.
Those cases keep the fresh compact recovery route.

| Go single-byte delete, compact route | before | after | v0.48.1 |
| --- | ---: | ---: | ---: |
| 4 KiB | 7 ms | 1.0 ms | 1.5 ms |
| 16 KiB | 21 ms | 3.1 ms | 3.0 ms |
| 64 KiB | 87 ms | 8.7 ms | |
| 137 KiB | 178 ms | 15.4 ms | 11.4 ms |

`TestCompactOldTreeTransientErrorEditsMatchFreshParse` pins the mid-file,
unbalanced, and end-of-file classes for nine grammars against a fresh parse
on the default route, and documents the two pre-existing divergences above.

### The fail-closed reparse after a local error

Pull request #613 extended the fresh-parse accepted-error retry ladder to the
plain incremental entry points. Its wide-stack condition fires on ordinary
GLR ambiguity (TypeScript runs four stacks), so every transient-error
keystroke on such a grammar paid one whole-file production reparse that the
ladder then discarded as quality-tied. The ladder now skips a tree that came
from old-tree reuse when its errors sit inside top-level items covering at
most a quarter of the source. Degenerate results still retry: an ERROR root,
a single-child root, a tree without reuse, or wide error coverage.

| 137 KiB single-byte delete | v0.48.1 | before | after |
| --- | ---: | ---: | ---: |
| typescript | 78 ms | 296 ms | 75 ms |
| javascript | 169 ms | 161 ms | 164 ms |
| go | 15 ms | 18 ms | 15 ms |

Every tree still equals the fresh default-route parse.

### Bounded compact incremental attempts

The compact incremental attempt now also counts a single tiny borrowed subtree
as reusing nothing (below 4 KiB), so it cannot commit a whole-file compact
reparse that production reuse would serve in a fraction of the time.

### Further scheduler cuts

- `Token` shrinks from 88 to 80 bytes by packing its flags and 16-bit symbol
  into one word. Tokens are copied by value at every election and dispatch on
  both routes.
- The scanner identity fingerprint, a SHA-256 of the scanner and grammar
  identifiers, is memoized per parse instead of recomputed at every election.
- The per-election current-checkpoint receipts, two SHA-256 digests of the
  scanner payload, are computed only when full receipts are retained.

| Grammar, 137 KiB full parse | compact before this part | compact after | production after | ratio |
| --- | ---: | ---: | ---: | ---: |
| go | 123.4 | 118.4 | 77.6 | 1.53 |
| scala | 153.7 | 134.0 | 76.2 | 1.76 |
| cmake | 184.2 | 162.5 | 97.4 | 1.67 |
| hcl | 155.9 | 139.6 | 70.4 | 1.98 |

### Fresh parses of error-bearing sources

A fresh compact parse of a source with a syntax error declined only at
materialization, after a whole-file pass, because a lexical error region
never starts the recovery version turns that an owned-recovery language needs
to publish. The route now declines when that region commits, when a shared
region opens with turns inactive, and when recovery versions rejoin the shared
lexer before end of file without an EOF accept. The fresh compact parse of the
137 KiB Go fixture with a mid-file error drops from about 424 ms to 275 ms;
the production parse alone costs 240 ms, so the discarded compact work fell
from about 180 ms to about 35 ms. The decline message keeps the publication
phrase that receipts and tests read. End-of-file errors still cost the clean
prefix before the decline; the error is not known earlier.

### Keystroke parity table

Single-byte insert and delete at the first near-top identifier, 137 KiB, one
process per row. v0.48.1 served every row on the production route; the repair
serves the old tree on the compact route and the edit on production reuse.
Reuse is reused bytes over the edited source.

| Grammar | insert v0.48.1 | insert repair | delete v0.48.1 | delete repair |
| --- | --- | --- | --- | --- |
| go | 5.5 ms, 97% | 10.3 ms, 98% | 14.5 ms | 18.2 ms |
| rust | 65.0 ms, 0% | 77.0 ms, 28% | 68.6 ms | 73.2 ms |
| scala | 74.1 ms, 0% | 94.3 ms, 0% | 66.6 ms | 90.9 ms |
| cmake | 17.5 ms, 88% | 31.1 ms, 88% | 20.9 ms | 21.4 ms |
| toml | 3.3 ms, 100% | 3.4 ms, 100% | 2.4 ms | 4.6 ms |
| make | 4.3 ms, 100% | 12.7 ms, 100% | 7.0 ms | 9.4 ms |
| css | 3.3 ms, 95% | 6.4 ms, 95% | 3.8 ms | 4.8 ms |
| scss | 3.0 ms, 95% | 4.4 ms, 95% | 3.8 ms | 4.2 ms |
| typescript | 4.5 ms, 98% | 6.4 ms, 98% | 63.8 ms | 75.3 ms |
| tsx | 4.1 ms, 98% | 8.0 ms, 98% | 70.0 ms | 86.6 ms |
| ini | 9.6 ms, 97% | 13.2 ms, 97% | 10.5 ms | 13.9 ms |
| diff | 2.2 ms, 100% | 2.5 ms, 100% | 1.5 ms | 3.4 ms |
| json | 49.1 ms, 69% | 76.9 ms, 69% | 48.0 ms | 73.0 ms |
| hcl | 91.3 ms, 0% | 98.7 ms, 0% | 80.1 ms | 77.3 ms |
| javascript | 3.5 ms, 98% | 4.4 ms, 98% | 163.4 ms | 171.3 ms |
| c | 5.3 ms, 98% | 8.8 ms, 98% | 1,891 ms | 3,023 ms |

Reuse matches or exceeds v0.48.1 on every row. The remaining time gap on the
small rows, 1 to 5 ms, is the production engine's own drift since v0.48.1,
which the incremental path inherits; a compact old tree adds about one
millisecond of admission work, and the compact borrow attempt under one. The
C delete is the memory-budget retry that the C fallback attribution work
owns. Every row matched its fresh default-route parse by node count and
S-expression.

## Part 5: production engine drift

### Attribution

The production route (`GTS_ADMISSION_CANDIDATE=0`) ran 1.1 to 1.4 times
v0.48.1 on the 137 KiB fixtures before this part. A bisect on hcl found a
gradual drift rather than one commit. A CPU profile diff against a v0.48.1
build of the same harness attributes the gap to four sources:

- Struct growth and by-value token passing. `Token` grew from 56 to 80
  bytes, `Lexer` from 160 to 208, and `dfaTokenSource` from 1320 to 1568.
  The new fields carry missing-node dependencies, the lookahead frontier,
  the skipped-prefix start, and error-mode proofs. The token source passed
  `Token` by value through a chain of per-token helpers, so every token paid
  about ten 80-byte copies. `runtime.duffcopy` and `runtime.duffzero` were
  the two largest positive entries in the diff: +110 ms of 1.7 s on Go and
  +280 ms of 1.6 s on hcl.
- Per-token lookahead-frontier bookkeeping from `e91b944f` (preserve
  missing-node edit dependencies). Both lexers decoded a rune at the
  frontier position for every token.
- The contextual close-angle probe, which compared symbol names before it
  looked at the token bytes.
- Two interface assertions per external scan on the retry path, which asked
  the scanner for its failure-mode capabilities every time.

### Changes

- Pass tokens by pointer through the per-token chain: `promoteKeyword`,
  `promoteActiveLiteralForCurrentState`, `normalizeDFAToken`,
  `splitCompactCloseAngleToken`, the three zero-width sentinel preferences,
  `trackZeroWidthExternalToken`, `preferDFASemicolonOverJSXText`, and the
  contextual close-angle helpers. `scanDFATokenForState`,
  `scanPreferredTokenForState`, and `nextDFAToken` gain `Into` forms that
  write the caller's slot. The by-value forms remain as wrappers for the
  other callers.
- Decode the frontier rune only for non-ASCII bytes in
  `Lexer.lookaheadEndByteAt` and `ExternalLexer.lookaheadEndByteAtCursor`.
- Check the token bytes before the symbol-name comparison in
  `deferContextualCloseAngleAction`.
- Answer the two external scanner failure-mode probes once per language.
- Guard the Swift member-keyword demotion and the Swift wide close-angle
  split with the cached language flag, so other grammars skip the calls.

### Results, 137 KiB full parse, production route, milliseconds

Minimum of 54 parses per cell, taken as paired runs of the three binaries
on a quiet host. `85843f84` is the state before this part.

| language | v0.48.1 | 85843f84 | this part | ratio before | ratio after |
| --- | ---: | ---: | ---: | ---: | ---: |
| go | 63.0 | 70.6 | 67.0 | 1.12 | 1.06 |
| rust | 38.9 | 53.0 | 51.8 | 1.36 | 1.33 |
| hcl | 54.9 | 65.7 | 62.8 | 1.20 | 1.14 |
| toml | 34.4 | 39.2 | 38.7 | 1.14 | 1.12 |
| cmake | 80.8 | 91.5 | 87.3 | 1.13 | 1.08 |
| json | 39.4 | 45.1 | 43.1 | 1.14 | 1.09 |
| css | 28.9 | 33.3 | 32.3 | 1.15 | 1.12 |
| scala | 55.4 | 66.7 | 65.8 | 1.20 | 1.19 |
| typescript | 40.8 | 50.0 | 47.8 | 1.22 | 1.17 |

The same binaries on the default compact route, measured in the same
session, give 1.7 to 2.2 times the v0.48.1 production numbers: Go 108.8 ms,
Rust 87.6 ms, hcl 124.5 ms, TypeScript 77.9 ms. The compact route inherits
the production cuts through the shared token source, and it gains about 3
percent from this part.

### What remains on the production route

- Rust runs 1.33 times v0.48.1. Its remaining profile diff is the GLR merge
  check `cStackEntryExternalScannerEndState`, which binary-searches the
  external scanner checkpoint set for every merge candidate that ends in an
  external leaf. v0.48.1 did not run that check in the merge.
- hcl runs 1.14 times. Its external token path takes 13 percent of samples
  against 7 percent at v0.48.1: the scanner itself, the retry wrapper's
  state capture, and per-language guards that run for every grammar.
- `Token` stays at 80 bytes. A 64-byte layout needs the missing-stack point
  out of the token or the unexported flags packed into one byte. Both touch
  the neighbors of exported fields, so they wait for a separate change.
- Raw-shape capture and the transient scratch checkpoint run per reduction
  and per iteration. Both serve the compact route's proofs and are new since
  v0.48.1.

## Part 6: compact scheduler per-token cuts

The compact scheduler profile after Part 5 (Go, 137 KiB, default route)
splits as: scheduler run 76 percent, of which dispatch 53, reduction 24,
election 16, shifts 15; materialization 21 percent, of which the postorder
visit 10, the derivation replay 5, and the result build 3. Four per-token
costs inside the scheduler were avoidable:

- The cap-pressure poll called `Core.Stats` on every dispatch loop, which
  validated the head node to report a node count. It now reads the node
  count directly.
- The per-state relex probe recomputed the scanner contract (reflection and
  a fingerprint) on every call, re-fingerprinted the checkpoint identity, and
  allocated four slices for its two transient snapshots. The scheduler now
  caches the contract and the identity per language, and the two snapshots
  use scheduler-owned scratch.
- The election re-asked the order adapter for the checkpoint identity on
  every token, and the adapter allocated two slices per answer. The election
  reads the cached identity.
- The reuse-proof invalidation copied a 104-byte lineage record at eight call
  sites. It takes a pointer.

### Results, 137 KiB full parse, default compact route, milliseconds

Minimum of 54 parses per cell, paired runs against the Part 5 commit
`0cf69ff3`.

| language | 0cf69ff3 | this part | ratio |
| --- | ---: | ---: | ---: |
| go | 105.3 | 97.5 | 0.93 |
| hcl | 123.1 | 123.3 | 1.00 |
| rust | 87.2 | 87.2 | 1.00 |
| typescript | 77.9 | 77.5 | 1.00 |
| scala | 119.0 | 116.7 | 0.98 |
| json | 71.4 | 70.9 | 0.99 |
| toml | 64.5 | 63.4 | 0.98 |
| cmake | 143.1 | 142.8 | 1.00 |
| css | 57.0 | 57.1 | 1.00 |

Go gains because its GLR stretches with an external scanner reach the relex
probe. The other grammars rarely reach it, and their remaining cost sits in
the core's reduction, condense, and materialization paths, which these cuts
do not touch. The production route is unchanged.

## Graduation status

The compact route serves fresh full parses by default, but it is not close
to graduating as the only engine. The cost envelope specification
(`spec.parser-cost-envelope.v1`) gates graduation on correctness against
locked C, deterministic work, wall time, allocated bytes and count, live
storage, retained heap, resident set size, incremental reuse and fallback
rate, and static storage. The current evidence:

- Correctness. The 206-grammar scorecard reports 201 PASS, 0 DIVERGE, 0
  FALLBACK, and 5 SKIP. Fallback keeps output correct through the
  production engine, so correctness is not the blocker.
- Coverage. The real-corpus matrix reports 64 PASS, 25 FALLBACK, and 8 SKIP.
  Markdown serves 15 files directly and falls back on 348. Every fallback
  pays a discarded compact pass before the production parse.
- Cost parity. A clean compact full parse costs 1.7 to 2.2 times the v0.48.1
  production parse and 1.5 to 1.9 times the current production parse. The
  wall-time and allocation gates fail by that margin.
- Incremental. Compact-materialized old trees reuse through the production
  incremental path at v0.48.1 rates. The compact borrow route itself reuses
  nothing on the parity table rows, and transient-error keystrokes depend on
  production reuse.
- Recovery. Owned recovery publication requires an executed end-of-file
  turn, and unpublishable shared recoveries decline to production.

The remaining work is structural rather than a list of small cuts: the
materialization double work, the condense and reduction path, lineage
persistence, recovery coverage, and the compact incremental borrow route.

## Remaining items outside this change

- Transient-error keystrokes on the compact route cost one compact recovery
  parse (178 ms incremental at 137 KiB for Go) against 11 ms at v0.48.1 on
  production reuse. Borrowing clean old-tree subtrees into compact recovery is
  the next step.
- TypeScript transient-error delete costs 300 ms on either route since pull
  request #613 added the memory-budget fail-closed retry; 64 ms at v0.48.1.
- C transient-error delete at 137 KiB still explores about 3.2 million nodes
  before the memory-budget retry (2.9 s). The C fallback attribution work owns it.
- Compact clean full parse stays 1.5 to 1.9 times production, and 1.7 to 2.2
  times v0.48.1 production. The remaining cost is the compact core's condense
  and reduction path plus materialization.
- The production route runs 1.06 to 1.19 times v0.48.1 after Part 5, and
  Rust 1.33 times. Part 5 lists the remaining attributed costs.
- Eight tagged scheduler tests fail identically on origin/main
  (`TestDiagnosticParserCoreGenericScheduler*`, `TestDiagnosticParserCoreConflict*`,
  `TestDiagnosticParserCoreStateDependentRelexKeepsExactSpanBranch`), as does
  `TestResultCompatibilityRetiredCommitProvenance`. This change does not touch them.
