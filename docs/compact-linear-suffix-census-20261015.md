# Go linear-suffix feasibility and transition evidence

The current Go census identifies a larger architectural target than another
boundary-lookup cache: about one third of graph-state publications are not
required by the observed corridor exits or later checked node reads. This is
an **upper bound on a possible representation change**, not measured speed or
memory savings. No parser route or default changes, and all merges remain held.

Evidence date: 2026-10-15 UTC. Baseline:
`7b016214d6322540ffd52009ae5eb214ce8434ea`, whose production source remains
`c498e889ec2d86382a3a0baad58020d7df7145b3`.

## What the census retains

The observer tags new single-link graph states produced inside C4 regions,
excluding retained multi-pop reduction construction and historical-boundary
cases. At every region exit it retains every live head and current frontier
index entry, plus their complete predecessor closure. Reads through `Core.node`
outside the region also force retention and predecessor closure.

Four separately executed observed/unobserved parses preserve both scheduler
work vectors and all four pinned deep tree digests. Invalid references remain
zero. Source pins and terminal exit zero pass the audit in local evidence
`gotreesitter-evidence/20261015-linear-suffix-retention-census`.

| Go fixture | Actual graph states | Tagged new states | Tagged states retained at exits | Remaining candidates | Fraction of all graph states |
| --- | ---: | ---: | ---: | ---: | ---: |
| `rewrite` | 2,996 | 1,395 | 399 | 996 | 33.24% |
| `query_compile` | 14,663 | 7,127 | 1,745 | 5,382 | 36.70% |
| `language` | 15,074 | 6,852 | 1,424 | 5,428 | 36.01% |
| `grammargen_lr` | 148,368 | 66,853 | 17,607 | 49,246 | 33.19% |

Large Go executes 11,890 observed regions. Its 97,524 later reads of tagged
states require no additional states beyond the conservative exit closures.
The largest region produces 1,192 tagged states. None of these counts estimates
how much graph import, prefix metadata or stack storage a replacement needs.

Direct array reads are not exhaustively intercepted. A real implementation
must account for those reads, immutable-row authentication, frontier history,
multiple pop paths, recovery, checkpoint state, hooks, metadata and rollback.
The census supplies no ownership certificate and does not authorize deleting
the counted states from the existing arena.

## Private transition-layer prototype

The next prototype represents an ordinary vector suffix above a frozen GSS
prefix under a fresh scheduler owner. Its operations construct no graph nodes
or links. Borrowed pop plans carry a revision and classification phase; graph
growth, prefix node/lineage/checkpoint mutation, owner completion and reset
invalidate the relevant authority. Buffer and object footprints are explicit.

Sixty-one seeded fixture shapes compare vector pops and one reduction commit
with a separate GSS reference. They cover arities zero through 255, interstitial
and trailing extras, child order, byte spans, score/order folds, predecessor
facts and reattached extras. Abstract stack-link work remains equal. Additional
tests reject stale/foreign/checkpoint leases, prefix mutations, overflow,
invalid payloads and logical node-cap violations without partial frame/work
changes. Focused transition, full core and phase-zero-A suites pass in Docker.
An ambiguous-prefix case preserves both GSS paths, matches a local suffix pop,
and requests GSS handling when a pop crosses into the diamond.

The initial overflow fixture used a childless payload, whose aggregate
precedence the existing C-aligned core correctly ignores. Its failed assertion
is archived; the corrected fixture uses a nonempty parent. Earlier source
without prefix freezing has a separate receipt. Final source and tests are
pinned in `20261015-linear-suffix-stack-prototype`.

Buckley's first broad review ends incomplete and supplies no approval. Its
completed bounded comparison flags a conditional zero-end difference: GSS
uses the first forward structural child's end when the reverse peel leaves
the structural end at zero. A direct regression reproduces vector end 0 versus
GSS end 5 before the correction and passes after it. The source follow-up
establishes no remaining mismatch in that narrow change. Both completed reviews
are supplied-source comparisons, not independent execution or parser approval.
A missing container utility interrupts the first post-fix runner; that attempt
is excluded, and the portable runner completes all final gates.

At this checkpoint parent construction, frontier-history preservation, GSS
import/flush and parser dispatch integration are absent. The next receipt below
adds private parent construction; it supplies no whole-parser qualification.

## Parent construction against the actual scheduler

The continuation resolves the actual vector state's sole ordinary reduction
cell, production plan and predecessor goto. It rechecks owner, prefix, revision,
classification phase and ordinary context after table callbacks. Parent fields
and aliases use the existing remapper and raw-store seam; no graph predecessor
is invented. All frame precedence and logical capacity checks precede parent
publication, and arena caps fail before any store mutation. Session pressure
history survives temporary scratch reset.

The actual scheduler exposes a gap in the earlier hand-built reference:
migrated extras clear branch order. A focused witness fails before correction
and passes afterward. Both private vector reduction operations now follow that
rule; this is a prototype correction, not a production bug fix. The initial
test's link-order type error is archived separately and supplies no evidence.

Eighty seeded shapes and an empty-stack epsilon case compare actual parent
metadata, migrated extras, subsequent pops, head facts and parent/pop/abstract
link counters against `ReduceOutputs`. Fifteen callback, context, plan, goto,
arena-cap and overflow cases reject without publishing a parent, frames or work;
a reentrant goto callback invalidates the borrowed pop. Existing transition
tests and full core/phase-zero-A suites pass in Docker. Frozen sources and
terminal exits are in `20261015-linear-suffix-parent-prototype`.

The parent review's epsilon concern is contradicted by the original GSS
zero-child branch and an explicit actual-scheduler witness above two extras,
with distinct gotos per predecessor state. The witness passes without changing
epsilon behavior. Its mutable-pop concern produces failing keep/span/trailing
witnesses. Commit now treats the returned description as a lease, consumes
saved scalar facts and reads migrated extras from original frames. All six
description-mutation cases pass afterward, along with the state-specific goto
comparisons and full core/phase-zero-A suites. Borrowed child descriptions keep
the existing read-only scratch contract; this does not make arbitrary writes
within the trusted package safe.

Buckley's completed follow-up considers the lease finding addressed and
establishes no remaining epsilon mismatch in the supplied source. It does not
independently execute the witnesses or certify parser integration/performance.

The parent checkpoint remains outside the checkout and parser routing. The
next receipt adds private frontier preservation and graph flush. It supplies
no whole-parser qualification.

## Frontier preservation and atomic graph handoff

The continuation assigns publications typed virtual ordinals, separate from
graph `NodeID`s. Each journal record retains its predecessor. The current
frontier keeps complete state/byte/checkpoint/shifted keys, including popped
states that are no longer active frames. A duplicate virtual or existing graph
key requests GSS handling before a shift or parent publication. A changed
frontier expires the old key list; active frames and their predecessor closure
still retain any necessary older records.

Flush marks active references and every current virtual frontier entry, follows
their predecessor closure, and serializes retained records in DAG order through
the existing graph publication checks. It preserves every preexisting graph and
index entry. The existing core transaction restores graph/index/work after a
failed flush; successful flush closes the vector capability. Serialization adds
no abstract parse work: virtual stack-link work was already counted when the
operation occurred. Logical node/link caps still include every publication.

Scheduler owners are explicit per virtual head, rather than inferred from its
predecessor. Zero/conflicting owners and stale pop plans are rejected. Scanner
quiescence is frozen at entry; both recorded-checkpoint and quiescent modes are
covered. Unsupported recovery/conflict/certificate contexts and active reduction
scratch decline. Journal, reference, history, retention and remap buffers are
included in the private object's footprint; this is not a parser budget gate.

Forty seeded traces execute 65 operations each against actual GSS reductions.
Twenty use each scanner capability mode; 26 persist explicit scheduler owners.
Heads and **every current frontier entry** match reference derivations, scanner
facts and lineage records after flush. Parent payloads, parent constructions,
reductions, emitted pop paths/payloads and abstract stack-link counters match.
Six flush-failure cases
preserve logical graph/index/work state, including a late index-cap failure
after partial writes; repaired cases retry. A direct history witness retains
four graph states in the original frontier and three after expiry, with equal
abstract work. Ambiguous-prefix handoff preserves both branches.

Buckley identifies a missing active-batch guard. Its witness fails before the
correction and passes afterward; the completed bounded follow-up considers the
guard addressed and establishes no defect in that correction. Neither review
executes the tests or certifies the surrounding parser. Final focused tests,
full core and phase-zero-A suites pass in Docker with frozen source pins in
`20261015-linear-suffix-handoff-prototype`.

This checkpoint implements a private handoff layer outside the checkout.
The next receipt adds selected real-Go replay and deferred scalar/set header
persistence; certificate handling, recovery and parser dispatch remain open.
There is no whole-parser qualification or graduation claim.
It advances the implementation boundary of [E-A7](https://github.com/odvcencio/gotreesitter/issues/1319),
whose corpus-throughput exit requirement remains unproven. All merges stay held.

## Real Go replay and deferred header lineage

Four separate Go processes compare unobserved, profile-only and replay parses.
Each preserves the full native core and scheduler work vectors and its pinned
deep tree digest. The profile chooses the 24 largest graph-publication regions
plus fixed region ordinals. These are selected traces, not all-region coverage.
Detached graph/metadata snapshots prevent model appends from sharing live arena
backing. The replay imports the actual authenticated terminals and independently
builds reduction parents and stack state; it does not replace token production.

The owner-only model stops mainly on inherited lineage. Deferred header
persistence preserves the actual successful scheduler-call inputs, including
the dirty bit and original alternative sets, then applies the existing GSS
scalar/set helpers inside the graph-flush transaction. A populated overflowed
input can produce a zero-member node set with overflow/blended flags; recording
only that final node would lose the input needed to reproduce it. Focused
inline, spill, overflow and repeated-update cases compare against GSS. Logical
command caps, unresolved membership and malformed chains reject without a
partial publication. Certificate references remain unsupported.

| Go fixture | Selected regions | Owner-only replayed operations | Lineage replayed operations | Regions reaching their end | Prefix handoffs |
| --- | ---: | ---: | ---: | ---: | ---: |
| `rewrite` | 25 | 125 | 193 | 2 | 23 |
| `query_compile` | 28 | 80 | 176 | 2 | 26 |
| `language` | 27 | 415 | 457 | 4 | 23 |
| `grammargen_lr` | 70 | 107 | 795 | 36 | 34 |

All 150 selected regions start successfully. The lineage model replays 900
shifts and 721 reductions, up from 727 total operations with owners alone.
Heads and every current frontier entry match graph/link/scanner facts and
lineage membership; spill positions are compared by members in their separate
arenas. Full model core work matches native work at every checked handoff.
The model serializes 166 new graph states versus 1,695 native states **within
those matched prefixes**. This is not a whole-parse memory or speed saving:
the diagnostic also allocates snapshots and membership imports.

At this checkpoint 106 handoffs reach below the deferred suffix into its opaque
GSS prefix. The next receipt adds deterministic-prefix import; the observed
header-lineage stops are already eliminated here.

The broad Buckley report ends truncated and supplies no approval. Its partial
evidence motivates a pending-command witness that fails before correction.
Capture now queues only successful persistence calls; finish rejects native
errors and unconsumed commands. The adapter checks snapshot ownership against
the live node, and repeated-owner tests reject completed/reset sessions. A
completed bounded review confirms those corrections and finds an uncapped
waiting queue. Its before witness fails; the corrected queue stops diagnostic
qualification at the logical cap while returning the native result unchanged.
A completed guard-only follow-up confirms that correction; the source audit
finds one production capture append path, with queues initialized empty.
Review scopes and incomplete output are preserved with the raw evidence.

Final focused diagnostic/vector tests, full core and phase-zero-A suites use
the explicit detached-clone tag `gts_eof_recovery_shadow`; each Go fixture runs
separately. Evidence is `20261015-linear-suffix-real-go-trace` and
`20261015-linear-suffix-lineage-trace`, with frozen sources, archived attempts
and terminal exits. No prior fleet/race/budget/RSS/timing certificate transfers.
This code remains outside the checkout and parser routes. No allowlist,
default, throughput or graduation change follows; all merges remain held.

## Lazy deterministic-prefix import

The continuation imports only the existing single-link frames needed to
complete a pop. It validates the complete required chain before changing the
representation. Original graph IDs, checkpoints, owner/lineage, score/order,
path counts and recorded precedence maxima remain intact. A stored single-link
maximum can include an earlier fold, so import does not recompute it from the
link. Imported frames consume neither new-publication work nor new node/link
budget; flush reuses their existing graph identities.

Deferred publications retain fixed graph predecessors. Moving the opaque base
therefore cannot reinterpret older frontier history, and no existing journal
entry acquires a forward virtual predecessor. Imported snapshots are checked
before operations and flush. Validation reads bounded raw links without an
adjacency allocation; a 12-origin check measures zero allocations. It still
scans the origins, so this is no throughput estimate.

| Go fixture | Selected regions | Previous replay operations | Imported-prefix operations | Regions reaching their end | Handoffs | Imported original frames |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `rewrite` | 25 | 193 | 866 | 23 | 2 | 308 |
| `query_compile` | 28 | 176 | 1,650 | 25 | 3 | 404 |
| `language` | 27 | 457 | 2,469 | 21 | 6 | 316 |
| `grammargen_lr` | 70 | 795 | 4,393 | 60 | 10 | 403 |

All 150 selected regions start. Completion rises 44→129; replay rises
1,621→9,378 operations (4,663 shifts and 4,715 reductions). All native work
vectors/pinned trees and model work/frontier semantics remain exact. The 21
handoffs now distinguish **16 canonical-boundary collisions and five ambiguous
prefixes**. Import totals 1,431 original frames, with a region peak of 37.
The model adds 958 graph states versus 9,664 native states within the matched
prefixes. Existing graph storage, diagnostic snapshots and membership imports
remain present; these counts are not whole-parse memory or timing savings.

Thirty-two seeded prefix shapes cover all available arities, extras, both
checkpoint modes and origin-only flush preserving IDs/work. Focused tests
cover base movement with older history, folded maxima, exhausted budgets,
partial/capped imports, revision limits and node/link/lineage/checkpoint
mutation. Original-node metadata changes roll back and retry.

Buckley finds that the loop rejects ERROR-state nodes, but initially accepts
a zero-cost ERROR-state final predecessor. A real recovery-discontinuity
witness fails before correction and passes after the guard; its bounded
follow-up confirms that guard. The metadata rollback test then exposes an
over-strict private snapshot check: core rollback restores logical membership
but can leave bytes in unused inline capacity. Detailed evidence confirms
owner, flags, counts, graph and work were restored. Before witnesses reject a
legitimate retry and unused-tail change. The corrected comparison preserves
all header fields and recorded inline members; active-member mutation still
rejects, and rollback/retry passes. Spilled membership relies on the core's
append-only arena contract. A completed bounded comparison covers this fix;
no shared-core rollback change or bug is claimed.

Final focused/import/diagnostic tests, full core and phase-zero-A suites plus
four separately executed Go trace checks pass with frozen pins in
`20261015-linear-suffix-prefix-import`. Reviews inspect supplied source only.
Runtime dispatch, terminal production, certificate/recovery handling and
memory integration remain open. There is no whole-parser C, incremental,
fleet, race, RSS or shuffled-timing qualification, no route/default change,
and no graduation claim. All merges remain held.


## First real C4 runtime integration: correctness passes, throughput rejected

The next private overlay replaces actual C4 shift/reduction publications with
vector operations. It builds terminals through the existing authenticated
terminal helper, preserves the existing parent builder and lexer, reads virtual
state during elections, and flushes real graph heads before generic dispatch
or acceptance. Work is no longer a shadow replay. Default routing is unchanged.
Full receipts, eager materialization, certification peaks, version lexing and
recovery contexts stay outside this experiment's entry domain.

The initial deep-tree/work comparison passes, but a stronger observation finds
that a path-count query also asks for a sole payload. The empty seed has one
path and no payload, so this wrongly invalidates reuse tracking. Separating
those queries fixes the failing observation and a focused seed regression.
Final four-fixture checks compare complete native core/scheduler work, pinned
deep trees, reuse arrays, lexer frontiers, branch/creation ordering, token and
dispatch totals, and election counts.

| Go fixture | Virtual operations | Graph nodes: native → vector | Entries / flushes |
| --- | ---: | ---: | ---: |
| `rewrite` | 547 | 2,996 → 2,786 | 246 |
| `query_compile` | 2,444 | 14,663 → 13,675 | 1,023 |
| `language` | 1,993 | 15,074 → 14,256 | 865 |
| `grammargen_lr` | 26,231 | 148,368 → 138,105 | 11,449 |

The full backend suite and core package pass in Docker. Repeated Go parsing,
cap/reset and live-tree ownership checks pass. The fused reduce/shift edit
receipt passes with its enabling flag. Four canonical Go files match locked C
through the candidate public route with no fallback. Go and YAML each pass
48 edit-session steps and allocate zero on no-edit reparses. Three Go memory
checks pass; the adversarial 1 MiB witness stops with a 1,074,099-byte observed
peak, below its existing 1,153,433-byte bound. No bound is raised.

The first harness command uses the wrong module directory and fails at setup;
its corrected run completes. A Go subtest selector also runs `rego` incidentally;
a separate anchored Go-only process passes. Both attempts are preserved. The
completed bounded Buckley review establishes no concrete defect in the supplied
runtime glue; it does not execute tests, re-review the backend, or approve the
excluded contexts.

Twenty alternating shuffled pairs compare this overlay with `7b016214d`, whose
production engine remains `c498e889e`. Each fixture has 20 samples per variant,
with `GOMAXPROCS=1`, one process per seed, `-count=1`, 750 ms, `-benchmem`, CPU 6,
6 GiB Docker and both shared locks. All recorded work metrics remain exact.

| Fixture | Median parse time: native → vector | Time change | Median B/op: native → vector | Allocations: native → vector |
| --- | ---: | ---: | ---: | ---: |
| `rewrite` | 1.835 → 2.139 ms | +16.58% | 1,776 → 20,809 | 21 → 66 |
| `query_compile` | 8.785 → 9.903 ms | +12.74% | 138,723.5 → 159,659 | 129 → 186 |
| `language` | 9.138 → 10.089 ms | +10.41% | 284,629 → 312,654 | 243 → 308 |
| `grammargen_lr` | 94.259 → 105.350 ms | +11.77% | 3,251,743 → 3,395,595 | 1,716 → 1,793 |

All timing and allocation differences have p<.001. Fewer graph nodes do not
establish a speed or whole-parse memory win: this version is rejected. The
source pins, raw samples, benchstat output, complete metric audit, failed
attempts and final correctness logs are in `20261015-linear-suffix-runtime`.
This is a focused Go screen under [#1065](https://github.com/odvcencio/gotreesitter/issues/1065), not fleet, race, RSS or graduation
qualification. No experimental runtime source is promoted.

A separate exit-frequency run preserves the complete observations and identifies
why the entry count is high:

| Fixture | Entries | Empty entries | Fork exits | External-token exits | Supported-action handoffs |
| --- | ---: | ---: | ---: | ---: | ---: |
| `rewrite` | 246 | 150 | 213 | 24 | 9 |
| `query_compile` | 1,023 | 623 | 824 | 124 | 75 |
| `language` | 865 | 515 | 701 | 108 | 56 |
| `grammargen_lr` | 11,449 | 6,774 | 9,398 | 1,320 | 731 |

Empty entries are a subset of the exit counts. No traced exit ends on a
no-lookahead token; immediate fork probes dominate. The trace is diagnostic
only, with separate source pins
in `20261015-linear-suffix-runtime-exits`; its instrumented code supplies no
timing evidence. All main merges remain held.


## Lazy runtime acquisition and cleanup allocation

Acquiring the backend only on the first supported operation cuts entries and
flushes **246→99, 1,023→406, 865→378 and 11,449→4,780** in fixture order. The
same 31,215 total vector operations and physical graph counts remain. Complete
observations, repeated/reset parsing, fused edit, memory, four locked-C Go
cases and the separate 48-step Go/YAML sessions pass. The bounded diff review
finds no concrete defect in named-return cleanup or pending-suffix handling.

Twenty new alternating pairs at the same settings still reject this variant:

| Fixture | Median parse time: native → lazy | Time change | Median B/op: native → lazy | Allocations: native → lazy |
| --- | ---: | ---: | ---: | ---: |
| `rewrite` | 1.837 → 2.127 ms | +15.79% | 1,776 → 27,361 | 21 → 426 |
| `query_compile` | 8.831 → 9.829 ms | +11.30% | 138,722 → 187,054 | 129 → 1,695 |
| `language` | 9.188 → 9.934 ms | +8.11% | 284,629 → 335,950 | 243 → 1,575 |
| `grammargen_lr` | 94.768 → 103.975 ms | +9.72% | 3,251,743 → 3,700,555 | 1,716 → 18,463 |

All differences have p<.001. Source pins, raw samples, work/seed audits and final
checks are in `20261015-linear-suffix-runtime-lazy-entry`. These are separate
comparisons with the unchanged engine, not paired evidence of improvement over
the first private variant.

Compiler diagnostics then identify the allocation mechanism: registering the
cleanup defer inside the VM loop makes both its closure and named `err` escape
to the heap. Registering it before the loop keeps the closure from escaping,
while backend acquisition remains lazy. Separate diagnostics preserve both
source mappings and complete compiler output in
`20261015-linear-suffix-runtime-retained-static-flush/escape-excerpts.json`.

An independent lifetime correction retains the scheduler's backend handle,
clears its Core/owner/prepared authority, preserves the monotone revision, and
retains at most **64 KiB of slice capacity plus the accounted object**. Retained
slices are cleared through capacity; oversized families are dropped. Reset
rejects an open suffix under a valid owner and allows disposal after the owner
ends. Active-reset rejection, bounded capacity, clearing, rebinding, ended-owner
disposal and repeated scheduler-handle tests pass. The completed bounded source
review finds no concrete defect in that supplied reset diff. These source/test
receipts are separate in `20261015-linear-suffix-runtime-retained`; that variant
inherits the loop-local defer and supplies no timing result.


## Retained backend with static cleanup: allocation corrected, CPU rejected

The combined candidate keeps lazy acquisition, the bounded handle, and one
cleanup registration before the loop. All four complete Go observations and
physical graph counts remain exact. Final bounded reset/scheduler-handle,
repeated/reset parsing, fused edit, memory, four locked-C Go cases and separate
48-step Go/YAML checks pass. The observed adversarial memory peak stays
1,074,099 bytes under the existing bound. Source/seed/sample audits pass.

The bounded cleanup diff review finds no concrete defect, but lacks inactive
helper context. A source audit confirms the unchanged `flushLinearCorridor`
returns immediately when inactive, without dereferencing a backend or owner;
error cleanup only clears the active flag. The first integration review
included that helper. This resolves the narrow context question and supplies
no broader parser approval.

Twenty further alternating pairs use the same settings and unchanged native
engine. They produce the following independent comparison:

| Fixture | Median parse time: native → retained/static | Time change | Median B/op: native → retained/static | Allocations: native → retained/static |
| --- | ---: | ---: | ---: | ---: |
| `rewrite` | 1.830 → 2.087 ms | +14.08% | 1,776 → 1,776 | 21 → 21 |
| `query_compile` | 8.833 → 9.773 ms | +10.64% | 138,722.5 → 138,723 | 129 → 129 |
| `language` | 9.162 → 9.978 ms | +8.91% | 284,629 → 284,630 | 243 → 243 |
| `grammargen_lr` | 94.140 → 104.194 ms | +10.68% | 3,251,747 → 3,283,987 | 1,716 → 1,726 |

All timing differences have p<.001. `rewrite` bytes and the first three allocation
counts are identical in every sample. Query bytes are statistically unchanged
(p=.261); the language witness adds one median byte (p<.001). The large file
adds 32,240 median bytes (+0.99%) and ten allocations (+0.58%, both p<.001).
Those directions remain recorded. The CPU regression rejects promotion.

Evidence is in `20261015-linear-suffix-runtime-retained-static-flush`, continuing
through 2026-10-16 UTC. The incomplete setup directory for the reset experiment
contains no run or qualification. No private runtime code is promoted; the
existing qualified production buckets and owner-approved YAML exceptions are
unchanged. Full fleet, race, RSS, on-disk source-walking guards,
certificate/recovery/eager coverage and graduation qualification remain open.
All merges to main stay held until the owner lifts the 0.56.0 hold.


## Instrumented blocker and CPU attribution

A separate `gts_workcount` query-fixture probe fails the existing corridor
lookup-identity test: **TableLookupsProxy 37,662→39,188 (+1,526, +4.05%)**.
The earlier matching core/scheduler vectors and benchmark metrics do not cover
this counter. This is an additional R3 blocker, not a waiver or a passing
ledger. The private reduction API resolves its action row again after C4 has
already dispatched the compiled row; reusing the authenticated compiled row
needs a qualified interface before another promotion attempt.

A separate five-second large-Go CPU profile records 6.51 sampled seconds,
including setup/preflight. Imported-origin validation accounts for **5.53%
cumulative / 3.07% flat CPU**, and all suffix validity checks account for
**7.37% cumulative**. These nested totals must not be added. The profile is
attribution only; its fixed-order timing and heap values are not comparison
evidence. Source pins, the failed counter probe, profiles, summary tables and
separate exit codes are in `20261015-linear-suffix-runtime-profile`.

Validation cannot be removed on an ownership assumption: the current
`Core.beginSchedulerOwned` validates the token and poisons errors, but supplies
no mutation-exclusion guard. Reducing repeated snapshot checks therefore needs
an explicit invalidation/ownership proof. No check or counter is weakened.
The runtime remains private; graduation and all main merges remain held.


## Compiled-row reuse and one-read GOTO handoff

The continuation removes the duplicate reduction-row lookup under the existing
C4 S2 decode-back/S3 stream-validation contract used by
`ClassifyBoundaryWithRow`. The compiled caller supplies its dispatched row;
other callers still resolve their row and recheck callback authority. Both
entrypoints use the same parent builder, production-plan/goto rechecks,
canonical/cap preflight and raw-store seams. The no-repeat-lookup and
fields/aliases/goto invalidation tests pass. The initial test-field compile
error is archived and supplies no qualification.

Query lookups fall **39,188→37,687**, leaving 25 above native. A diagnostic-only
site trace isolates them: actions 10,047, classification 7,257 and other reads
12,377 remain exact, while GOTO reads rise **7,981→8,006**. These are repeated
reads after a speculative reduction hands off before publication.

The final bridge captures one pure immutable-root-table GOTO result during a
reduction and arms it only for the immediately resumed GSS path. Its key binds
the language, predecessor state and symbol; hit or mismatch consumes it.
Native GSS cannot populate it, an unprimed handoff disables it, and every exit
clears its scope. A broader first replay variant passed the query/tree checks;
the final one-consumption form prevents it becoming a general fallback cache.
Zero-target, foreign-key, empty/nested/ended scope, one-consumption and stale
repopulation checks pass. An incorrect 16-byte size assumption in the first
test is archived; `StateID` is 32 bits, and the actual slot is **24 bytes**.
The state domain is not narrowed.

The root wrapper's actual slot size is added once to `Core.FootprintBytes`,
even inactive; overflow saturates. The accounting test and full core package
pass. The adversarial 1 MiB Go witness now observes **1,074,123 bytes**, exactly
24 above the preceding source and below the unchanged **1,153,433-byte** bound.
Final lifecycle/cap/live-tree, fused edit and memory checks pass. Four Go cases
match locked C without fallback; Go and YAML each pass 48 edit-session steps
with zero no-edit allocations.

| Fixture | Raw table lookups: native → handoff | Full diagnostic work vector |
| --- | ---: | --- |
| `rewrite` | 7,582 → 7,582 | exact |
| `query_compile` | 37,662 → 37,662 | exact |
| `language` | 37,652 → 37,652 | exact |
| `grammargen_lr` | 400,160 → 400,160 | exact |

Complete native core/scheduler vectors, pinned trees, dependency arrays/frontiers,
ordering and token/dispatch/election totals also remain exact. The bounded
compiled-row review finds no concrete defect in its supplied contract/diff.
The replay review's missing call-flow/counter context is supplied in a completed
follow-up: generic apply occurs only after `Arm`; private failure flushes and
returns first. The supplied lookup implementation and unchanged counter hook
support the replay's physical-read accounting. Reviews inspect supplied source,
not full parser/performance qualification. No exception or relaxed gate is used.

The final sources and separate intermediate attempts are pinned in
`20261016-linear-suffix-compiled-row`, `20261016-linear-suffix-goto-handoff`,
`20261016-linear-suffix-goto-once` and
`20261016-linear-suffix-goto-accounted`; the site trace is separately pinned in
`20261016-linear-suffix-compiled-row-lookup-trace`.

Twenty alternating native-vs-accounted pairs complete at the established
750 ms/one-CPU/6 GiB settings. All sample, source, seed and work audits pass.
Timing variance is higher than earlier screens (up to ±12%):

| Fixture | Median parse time: native → handoff | Direction | Benchstat p |
| --- | ---: | ---: | ---: |
| `rewrite` | 2.049 → 2.300 ms | +12.30% | .001 |
| `query_compile` | 9.963 → 11.192 ms | +12.33% | .017 |
| `language` | 10.352 → 11.504 ms | +11.13% | .076 |
| `grammargen_lr` | 107.090 → 116.545 ms | +8.83% | .009 |

The language difference is inconclusive; the other three regress significantly.
Allocation counts remain **21/129/243/1,716→21/129/243/1,726**. Large-Go median
bytes rise **3,251,747→3,283,992 (+0.99%)**; the other byte directions remain in
the raw audit. This closes the measured four-fixture lookup blocker but rejects
throughput promotion. It does not establish all-language R3 qualification.

## Borrowed validation preserves checks, not a demonstrated speed win

The next source traverses frozen origins by pointer and compares the seven
non-set lineage scalars directly, removing whole-origin/argument copies and
zeroed temporary sets. It still checks every origin, record, link, checkpoint,
owner/lifetime and logical membership. It adds no validation cache or mutation
exclusion assumption. The copied old predicate is the oracle; reflected scalar
mutations cover future header fields, and count/flag/spill/active-inline/unused-
tail combinations agree. The oracle, existing mutation/rollback/import tests,
full core, all four complete work/observation vectors and the same C/edit/memory
gates pass. The bounded source comparison finds no missing field or pointer
lifetime defect in its supplied scope.

Twenty new alternating pairs against the unchanged native engine complete;
timing variance reaches ±15%:

| Fixture | Median parse time: native → borrowed | Direction | Benchstat p |
| --- | ---: | ---: | ---: |
| `rewrite` | 1.964 → 2.097 ms | +6.77% | .006 |
| `query_compile` | 9.066 → 11.052 ms | +21.90% | .001 |
| `language` | 9.832 → 11.114 ms | +13.03% | .026 |
| `grammargen_lr` | 108.158 → 112.166 ms | +3.71% | .040 |

All four timing directions regress in this screen. The first three allocation
counts remain native; large Go remains +10 (+0.58%) and +0.99% median bytes.
The other byte differences are statistically unchanged. These are independent
native comparisons, not evidence that the copy change improves its parent.
Source/work/seed/sample audits pass in
`20261016-linear-suffix-borrowed-validation`. The runtime stays private and
unpromoted; on-disk source guards, full-fleet/race/RSS and graduation gates
remain open. All main merges stay held.

## Native lineage occupancy rules out a sparse-set layout

A separate tagged native-only census uses production engine `c498e889e` at
checkout `7b016214d`, with no private kernel source. Four Go fixture processes
retain pinned trees/work. A tagged read-only post-session observer then checks
the owned YAML R4 sample: one compact route, zero fallback, complete/error-free
root and the same deep tree as fresh production. These are diagnostic runs,
not performance comparisons.

The actual node-lineage record is **32 bytes** and its alternative set is
**12 bytes**. Sets are dense:

| Fixture | Graph nodes | Logical nonempty sets | Set occupancy | Nonzero error-cost or reference records |
| --- | ---: | ---: | ---: | ---: |
| `rewrite` | 2,996 | 2,647 | 88.35% | 0 |
| `query_compile` | 14,663 | 13,917 | 94.91% | 0 |
| `language` | 15,074 | 13,600 | 90.22% | 0 |
| `grammargen_lr` | 148,368 | 141,100 | 95.10% | 0 |
| `owned_yaml` | 356 | 308 | 86.52% | 0 |

A 24-byte header plus a 12-byte sparse set slot would raise logical storage
on every witness: large Go **4,747,776→5,254,032 bytes (+10.66%)**, owned YAML
**11,392→12,240 (+7.44%)**. That layout is rejected before implementation.
The alternative measured target is the two scalar fields that are zero here:
error cost and certificate-reference index. A hypothetical 24-byte hot record
with eight-byte cold pairs projects **3,560,832** logical bytes for large Go
and **8,544** for owned YAML (25% below current). At this census stage it is only a projection, excluding
mutation/rollback, capacity and transient costs. The native implementation screen
below subsequently rejects its throughput; the projection is not a measured
memory, RSS or speed saving. Final occupancy alone does not bound transient usage. The census source/route/tree audits are in
`20261016-lineage-occupancy-census` and `20261016-lineage-occupancy-yaml`.
No language graduates and no merge hold is lifted.


## Native cold error/reference plane: qualified screen, throughput rejected

Evidence: local `gotreesitter-evidence/20261016-lineage-cold-plane`, based on
native checkout `7b016214d6322540ffd52009ae5eb214ce8434ea` (production engine
`c498e889e`). This source contains no private linear-suffix kernel. It keeps
alternative sets dense and changes the node-lineage hot record from 32 to 24
bytes by moving exact error costs and immutable certificate-reference indices
into an optional NodeID-indexed eight-byte pair. This is a 25% record-width
reduction, not a measured total-memory or RSS reduction.

Missing cold indices read as zero. The store checks gross allocation before
changing storage, including replacement-buffer overlap; a scheduler checker
uses the existing memory-budget stop. Its journal preserves nested checkpoint
commit/restore, and the footprint includes buffers retained by older marks.
Reset clears stale slots and the budget checker. Recovery markers copy both
hot and cold history; EOF recovery shadows own separate cold buffers, included
in copy planning and equality/disjointness checks. No cost/reference identity,
rank, membership, language routing, allowlist or budget limit is narrowed.

Qualification passes before timing:

- Full compact-core tests, including a 1,200-operation nested reference model,
  allocation rejection before mutation, zero-store allocation checks, gap reuse,
  reset and stale/non-LIFO checkpoint rejection.
- Four pinned Go deep trees and complete Core.Work records; all 21 fields in
  DiagnosticWorkCountValues equal native. Raw lookup counts remain
  **7,582 / 37,662 / 37,652 / 400,160**. The diagnostic canonical admissions,
  lifecycle/cap/live-tree checks and fused-edit receipt also pass.
- Four canonical Go candidate parses match locked C without fallback. Focused
  YAML ownership, document-stream and EOF cases match locked C, including the
  owned 8 KiB witness. The original owned sample's work/coverage guard passes.
- Separate Go/YAML standard-candidate invariant sessions each complete 48 edit
  steps with zero no-edit allocations. These do not certify owned YAML
  incremental activation. The Go memory witness peaks at **1,124,784 bytes**,
  below the unchanged **1,153,433-byte** bound.

The first bounded Buckley helper review is incomplete and supplies no approval.
Its completed retry finds an unchecked NodeID-to-int conversion on 32-bit Go.
The timed source rejects unrepresentable IDs before reservation or mutation;
all store tests, including that regression, pass under `GOARCH=386`. The bounded
integration review suggests checker retention in release helpers. Actual
production calls go through `ResetReleasingRetention`, which calls `Reset`
first; Reset already clears that checker. The reported production retention
path is absent. The review report and this source-based assessment are saved.
These reviews do not approve the whole runtime.

The representation change also requires full semantic rollback assertions.
A subsequent test-only overlay compares hot records and cold pairs in the
existing sibling/discontinuity/reference tests, preserving their former field
coverage. The full core suite and focused backend race gate pass. Engine source
hashes are identical to the timed source.

Twenty alternating pairs per language use the repository randomized runner,
GOMAXPROCS=1, count=1, 750 ms, benchmem, CPU 6 and serialized 6 GiB Docker. Go
seeds are 18601–18620; YAML seeds are 18701–18720:

| Witness | Median native → cold plane | Time direction | Benchstat p |
| --- | ---: | ---: | ---: |
| Go `rewrite` | 1.841 → 1.864 ms | +1.22% | .020 |
| Go `query_compile` | 8.837 → 8.938 ms | +1.15% | .001 |
| Go `language` | 9.187 → 9.311 ms | +1.35% | .001 |
| Go `grammargen_lr` | 94.672 → 97.096 ms | +2.56% | <.001 |
| YAML R4 attempt/fallback | 206.413 → 201.189 µs | −2.53%, inconclusive | .108 |
| YAML owned R4 | 361.497 → 370.251 µs | +2.42%, inconclusive | .277 |
| YAML owned 8 KiB stream | 1.408 → 1.479 ms | +5.09%, inconclusive | .183 |

All allocation counts remain exact: Go **21 / 129 / 243 / 1,716** and the
three YAML routes **37 / 109 / 336**. Go allocated-byte medians differ by at
most 0.002%; owned YAML bytes remain **12,872 / 44,313**. The ordinary YAML
attempt/fallback allocates **17,144→16,376 bytes (−4.48%)**, with no confirmed
time gain. Benchmark route/work metrics, all sample counts, alternating seed
order and source hashes pass the audit. YAML timing variance reaches ±8%.

**Reject this source as a throughput improvement.** No runtime is promoted.
Full 206-language qualification, parser-level race coverage, large-file RSS
comparison and on-disk guards for overlay-added source remain open; they are
not borrowed from earlier experiments. The separate native tagged-word set
prototype targets another storage direction and supplies no throughput claim
here. All main merges remain held pending the owner's explicit release.


## Qualified tagged alternative-set storage

Buckley commit `b45a0192520f873edf0eb2fdcca169dbc153a748` promotes the qualified
implementation into the held [core PR #1485](https://github.com/odvcencio/gotreesitter/pull/1485).
Baseline is native `7b016214d` (engine `c498e889e`). The prior cold plane remains
rejected. This change has no private suffix kernel, interner or sparse store.
It shares the exact inline member and exact one-based spill offset in one word,
selected by the existing spilled flag. Member/offset widths, count/flags, the
32-member cap and pruning rules remain unchanged. Sizes fall **12→8 bytes** for
AlternativeSet, **32→28** for graph lineage and **56→48** for reduction output.
The dispatch header also shrinks **88→80** and canonical groups **48→40**;
this does not meet E-A2's at-most-32-byte dispatch-header target.
Every journal snapshot restores that active word, including a singleton spilled
inside a nested commit and restored by the outer rollback.

The bounded Buckley representation review finds a pre-existing unchecked spill
offset conversion. The final source checks representability before append;
on exhaustion it preserves every recorded member and marks overflow, preventing
an incomplete set from proving containment. A narrow repair review reports no
concrete defect. Tests cover the final representable uint32 offset, rejection
beyond it, full-width members and nested rollback. The reports supply no broader
parser approval.

The first tagged source passes focused/full-fleet correctness but has a compiler
regression. Twenty pairs leave compact Go timing inconclusive; ordinary YAML
attempt/fallback time falls 1.57% (p=.006), but allocations rise **37→39**.
Those results belong to that source. Allocation profiling, not fixed-order timing,
attributes the new allocations to member reads. Compiler diagnostics show the
public forwarding accessor at cost **83**, above budget **80**; native is **79**.
Caching the word locally still costs 85 and is not timed or promoted.

The final implementation removes the forwarding wrapper and calls the single
public implementation directly. Its body is byte-for-byte identical to the
previous private body. The public inline cost is **70**. An allocation regression
fails for empty, inline and spilled consumers on the wrapped source and passes
on the final source, including race coverage. The initial fixed-count profile
returns ordinary YAML to **37 allocations / 16,760 bytes**; this profile is
attribution evidence, not a timing comparison. Final randomized measurements
below independently confirm those counts.

### Final source qualification

Before timing, the corrected source passes **all 414 rows exactly**: 412 ordinary
and two diagnostic-owned YAML rows. No counter exception or ratchet reset occurs.
Native baseline captures are reused only where source and helper identity remain
unchanged. Four Go pins retain complete Core.Work and deep trees; all 21 fields
in DiagnosticWorkCountValues agree. Raw lookups remain **7,582 / 37,662 / 37,652 /
400,160**. Representative ledger counters remain:

| Counter | Before → after |
| --- | --- |
| Go R4 full tokens / nodes / max versions | 589 / 1,251 / 3 → identical |
| Go first edit tokens / nodes / reused bytes / splices | 374 / 1,553 / 1,784 / 266 → identical |
| Owned YAML full / first-edit nodes | 484 / 484 → identical |
| Ordinary YAML ownership decline | unchanged |

The final default source passes **206 R4 invariant sessions**, a separate owned
YAML R4 session and **618 C fixture checks**, with no skips. The strict 204-file
census is exactly unchanged: **159 PASS / 45 known FAIL**, including all **120
actual compact/C-exact acceptances**. This preserves the known fallback failures;
it is not a 204-language C certificate. All no-edit invariant observations stay
zero. The standard Go/YAML 48-step sessions and focused C/lifecycle/memory gates
also pass.

After copying exactly the eight frozen files into the core checkout, native
full-core and one-pass races, phase-zero-A, root C4/version-lexer checks and all
206 static table shapes pass. Focused opt-in fusion Go/YAML C checks and its edit
receipt pass; no new full opt-in fleet is claimed. The 386 helper tests and library
build pass, not the full root 386 suite. L0 remains **756 root files / 12 API tag
sets**, R6 **454 language-name uses / 101 environment reads**. All **64** CI
selectors resolve against **7,686** defined functions. Source hashes match the
measured engine and actual on-disk guards pass. CI subsequently exposes four
stale 88-byte header assertions and an associated canonical-group assertion.
Buckley test-only follow-up `14f255bac37e64670b90b3120534b505ffeb6a1e` tightens
them to **80 / 40** rather than loosening a bound.
The exact tagged CI step and all four layout checks under race pass locally;
the earlier race-shard failure is the same layout mismatch, not a reported data race. Action-output width remains 40.
The eight measured engine/regression files remain byte-identical throughout.

### Final twenty-pair comparison

Each language uses its own 20 alternating pairs through the repository runner:
GOMAXPROCS=1, count=1, 750 ms, benchmem, CPU 6 and serialized 6 GiB Docker. Go seeds
are 19001–19020 and YAML seeds 19101–19120. All samples, role order, source hashes,
benchmark work metrics and allocation counts pass the audit.

| Workload | Median native → final | Time direction | p |
| --- | ---: | ---: | ---: |
| Go `rewrite` | 1.848 → 1.834 ms | −0.78% | .030 |
| Go `query_compile` | 8.865 → 8.800 ms | −0.72%, inconclusive | .445 |
| Go `language` | 9.205 → 9.146 ms | −0.65% | .004 |
| Go `grammargen_lr` | 95.271 → 94.091 ms | −1.24% | .003 |
| YAML R4 attempt/fallback | 200.204 → 196.966 µs | −1.62%, inconclusive | .142 |
| YAML owned R4 | 358.491 → 354.748 µs | −1.04%, inconclusive | .445 |
| YAML owned 8 KiB | 1.392 → 1.398 ms | +0.42%, inconclusive | .862 |

All allocation counts equal native: Go **21 / 129 / 243 / 1,716**, YAML
**37 / 109 / 336**. Ordinary YAML bytes fall **17,144→16,760 (−2.24%)**.
Owned R4 stays **12,872 bytes**; stream medians round **44,312.5→44,313**
(+0.00113%). Compact Go byte medians stay equal except large Go
**3,251,747→3,251,727.5** (less than 0.001%). No extra allocation is promoted.
The independent default full/edit/no-edit controls move **−3.26% / +0.11% /
−1.13%** (p<.001/.883/.015), with **6 / 5 / 0** allocations; full bytes round
1,159.5→1,159 and the others stay 394 / 0. These controls bypass this compact
representation and receive no causal attribution. Neither independent screen
isolates a component speed effect against its private parent.

Final accounted footprints use one fresh process per role/fixture, pinned trees
and work, the actual qualified engine and explicitly restored native files:

| Go fixture | Core retained bytes: native → final | Direction |
| --- | ---: | ---: |
| `rewrite` | 621,785 → 606,437 | −2.47% |
| `query_compile` | 2,288,425 → 2,227,921 | −2.64% |
| `language` | 4,526,021 → 4,401,857 | −2.74% |
| `grammargen_lr` | 26,056,865 → 25,349,985 | −2.71% |

Large-Go total accounted scheduler storage falls **27,644,891→26,937,587 bytes
(−2.56%)**. Per-process Max RSS observations are archived without a statistical
savings claim. A separate **1,049,572-byte / 524-document** owned YAML probe is
complete, error-free, compact-only and C-exact; its Go+C process peaks at
**144,484 KiB**, below the unchanged 400-bytes/source-byte bound. This is a single
safety probe, not a paired RSS comparison.

Evidence is local `gotreesitter-evidence/20261016-alternative-set-tagged-word`:
final source and pairs in `direct-members`, `final-fleet`, `native-guards`,
`final-footprint`, `optin-checks`, and `large-yaml`. Initial unit size/nested-owner
expectations, runner quoting/container-tool errors and the rejected generated
commit message remain preserved; final assertions/gates are not skipped. The
message retry passes the same safety check without changing staged code.

This qualifies a small compact Go throughput and storage improvement. **It does
not establish a YAML speed improvement or graduate a language.** Ordinary YAML
ownership/cost and the larger Go throughput gap remain. Route/default/allowlists
and all owner ratchets stay unchanged. No C speed ratio is measured here.
All main merges remain held until the owner explicitly releases the 0.56.0 hold.
