# Compact-lane perf attribution board

This document defines one boundary-exact attribution tree over the compact
parser core's fresh-full-parse CPU. It states the disambiguation rule for
every function that more than one component could otherwise claim. It
records the noise floor of the local measurement host, and the first
published receipt.

This is measurement infrastructure only. It changes no parser code, no
routing, and no shipped behavior.

## Why this document exists

Three prior estimates put the compact scheduler's share of full-parse CPU at
77-79%, at approximately 50%, and at 78-82%. Each estimate used a different,
undocumented attribution boundary: one drew the scheduler boundary wide
(covering shift, reduce, and election together), one drew it narrow, and one
used a different profiling method. The three numbers do not contradict each
other once you know their boundaries, but no one had written the boundaries
down, so the campaign could not tell whether a proposed perf lever targeted
77% of parse CPU or 50% of it.

Campaign v7 tranche C0 (`spec.campaign.v7`, `hypha://m31labs/gotreesitter`)
blocks new perf-lever bets until one receipt exists with a named boundary for
every component, a documented noise floor, and a cost-per-event model. This
document is that receipt's home. The attached tool
(`cgo_harness/attribution`) regenerates the receipt from one command.

## Scope

The receipt covers the compact parser core's fresh, non-incremental full
parse: the default route for eligible full parses (`admission_switch.go`),
and the diagnostic runner path (`parserCoreFreshFullRunner`) that exercises
the same scheduler and materialization code directly. Campaign v7 tranche B9
retired the compact admission switch's 64 KiB source-length eligibility
decline; the shipped route now attempts every eligible full parse regardless
of size, with a scheduler stop-control poll (tranche B8) bounding a large or
pathological input instead.

It does not cover: incremental reparse, error recovery, retry passes, the
production (non-compact) GLR engine, or any language other than the four
pinned Go canonical fixtures.

## The compact lane, by file

| Area | File(s) | Key entry points |
|---|---|---|
| Scheduler driver (root package) | `parsercore_phase0_driver.go` | `run`, `dispatchPass`, `dispatchPassActive`, `elect`, `canonicalize` |
| Core structures (`internal/parsercorephase0`) | `core.go`, `scheduler_owned.go`, `boundary_index.go`, `checkpoint_interner.go` | `ApplySchedulerAtomic`, `condenseWithOutcomeAtomic`, `popPaths`, `Derivations`, `MaterializationOrder` |
| Lexing | `parser_dfa_token_source.go`, `lexer.go` | `(*dfaTokenSource).Next` |
| Runner glue | `parsercore_phase0_fresh_full_runner.go` | `executeSchedulerOpen`, `parse`, `materialize` |
| Shipped-route fidelity tail | `admission_switch.go`, `admission_switch_candidate.go` | `normalizeReturnedTreeForParse`, `resolveCRecoverySwallowedError`, `maybeCompactReturnedFullTree` |

The four canonical fixtures are SHA-256-pinned in
`internal/benchfixtures/testdata` and authenticated the same way
`cgo_harness/pure_c/run_canonical_go_full_parse.sh` and
`diagnosticParserCoreCanonicalAdmissions` authenticate them:

| Fixture | Bytes |
|---|---:|
| `rewrite.go` | 5,116 |
| `query_compile.go` | 20,168 |
| `language.go` | 41,387 |
| `grammargen/lr.go` | 235,626 |

Before tranche B9, `grammargen/lr.go` sat above the compact admission
switch's 64 KiB source-length eligibility floor and never took the shipped
`Parser.Parse` route (diagnostic-lane only; see the first receipt below,
which predates B9). B9 retired that floor, so all four fixtures now run
through both lanes; see the B9 addendum.

## Two lanes, one methodology

Every fixture profiles under one or both of two lanes. Both lanes time an
identical, already-committed lifecycle; the tool never adds new parser calls.

- **Diagnostic lane.** `runner.parse`, shallow completeness validation, and
  `Tree.Release` — the exact timed region of
  `BenchmarkParserCoreFreshFullCanonical` (build tag
  `gts_parsercorephase0`). Runs for all four fixtures.
- **Shipped route.** `Parser.Parse` and `Tree.Release` — the exact timed
  region of `BenchmarkDiagnosticParserCoreWarmProductionParseQueryCompile`,
  generalized to all four fixtures (tranche B9 retired the 64 KiB eligibility
  floor that used to keep `grammargen/lr.go` diagnostic-lane only). Every
  sample is verified, by the admission-candidate routed/fallback counters, to
  have actually taken the compact route rather than falling back to
  production.

The tool labels every row with its lane so a reader never mixes shipped-route
public-API overhead into the diagnostic lane's scheduler-only numbers, or
vice versa.

## The attribution tree

Nine components, mutually exclusive and collectively exhaustive. Every
CPU-profile sample lands in exactly one.

1. **scheduler-dispatch** — the scheduler loop, per-token boundary
   classification, and the mechanical application of shift and accept
   actions. Owning functions: `run`, `dispatchPass`, `dispatchPassActive`,
   `applyGenericShifts(Owned)`, `applyGenericExtraShifts(Owned)`,
   `applyGenericAccept`, `finish`, `Core.Shift*`, `Core.ClassifyBoundary`,
   `Core.Actions`, scheduler construction and seeding, and the accepted-tail
   cleanliness check (`requireParserCoreFreshFullAcceptance`,
   `parserCoreFreshFullAcceptedTailIsClean` — see the naming-collision note
   below).
2. **elections** — choosing among competing alternatives: which GSS heads
   advance each round (frontier election), and which action wins at an
   ambiguous cell (GLR conflict resolution / dynamic precedence). Owning
   functions: `elect`, `executeDiagnosticParserCoreGenericConflictDetailed`,
   `applyGenericConflict(Owned)`, `diagnosticParserCoreConflictPolicyOrdinal`,
   `diagnosticParserCoreRepetitionFoldOrdinal`.
3. **reductions-and-pops** — applying a reduce action: enumerating GSS pop
   paths back to a production's children, building the parent subtree, and
   merging (condensing) the result into the derivation graph. Owning
   functions: `applyGenericReduction(Owned)`, `Core.Reduce`,
   `Core.ReduceOutputs*`, `Core.popPaths`, `Core.popSingleLinkPath`,
   `Core.factorExactPredecessor`, `Core.mergePredecessorsBounded`,
   `Core.insertLinkBounded`, `Core.appendNode`, `Core.appendSubtree`,
   `Core.walkCleanPrefixRanks`.
4. **canonicalization** — post-step header-lineage deduplication: merging
   headers that reconverge to the same `(state, byte-offset)` boundary after
   any dispatch action. Owning functions: `canonicalize`,
   `canonicalizeDiagnosticParserCoreHeaders`,
   `diagnosticParserCoreCanonicalScratch.canonicalize(Linear|Mapped)`,
   `mergeDiagnosticParserCoreCleanPathLineage`,
   `persistHeaderLineageOwned`, `RecordReductionLineageOwned`,
   `RecordHeadLineageOwned`.
5. **lexing** — producing the next token from source bytes. Owning function:
   `(*dfaTokenSource).Next`, and every function defined in
   `parser_dfa_token_source.go` or `lexer.go` (a whole-file rule, since both
   files exist for exactly this one job).
6. **materialization** — turning the accepted derivation into the returned
   public `*Tree`: selecting the canonical derivation when more than one
   exists, building nodes, attaching parent/sibling links, replaying parser
   states, and finalizing the root span. Owning functions:
   `completeAcceptance`, `selectCompactAcceptanceDerivation`,
   `Core.Derivations`, `materializeDiagnosticParserCoreAcceptedTree`,
   `materializeDiagnosticParserCoreAcceptedSelection`,
   `finalizeDiagnosticParserCoreAcceptedRootSpan`, `Core.MaterializationOrder`,
   `Core.VisitMaterializationPostorder`, `(*Parser).replayCompactDerivation`.
7. **compat-tail** — the shipped-route-only public-API fidelity tail that
   runs after the compact tree exists, so every `Parser.Parse` return matches
   production's API surface exactly. Not reachable from the diagnostic
   runner path. Owning functions: `normalizeReturnedTreeForParse`,
   `resolveCRecoverySwallowedError`, `maybeCompactReturnedFullTree`,
   `tryCompactFullParseRoute`, `attemptAdmissionCandidateFullParse`,
   `admissionCandidateFullParseEligible` (the whole of `admission_switch.go`
   and `admission_switch_candidate.go` — a whole-file rule).
8. **recovery** — the compact core's native locked-C recovery mechanism
   (error cost, missing-token insertion, retry, and recovery election over
   compact subtrees; campaign v7 tranche B3). Added in tranche B3 stage S1,
   before any recovery engine code exists, per gate G5 ("classifier
   first... so recovery cost can never hide in `other`"). It owns no
   functions yet — compact has no recovery implementation (B3 stages S2-S5
   add error-cost, absorb/condense-resume, election, and missing-token code
   class by class) — so it reads 0.0% on every clean canonical fixture
   today, the same way `compat-tail`'s conditional work reads 0.0% below.
   The whole-file rule for `internal/parsercorephase0/recovery_cost.go`
   (stage S2's planned file) is forward-declared now so landing that file
   does not also require touching this table.
9. **other** — every sample the walk cannot attribute to a named component:
   Go runtime, garbage collection, and goroutine-scheduling frames with no
   gotreesitter-domain ancestor, plus any genuinely unclassified function.
   The tool separately reports the largest unclassified, non-runtime
   functions it saw; a nonzero entry there is a signal this table has a gap,
   not a license to guess.

### A naming collision to avoid

`parserCoreFreshFullAcceptedTailIsClean` checks that no real source byte
follows the accepted head (only parser padding may remain) before the
scheduler declares acceptance. This is **not** the compat-tail component. It
is a scheduler-dispatch acceptance check that runs on both lanes. The English
word "tail" names two unrelated things here: an accepted-frontier byte-range
check (scheduler-dispatch), and a post-parse public-API fidelity pass
(compat-tail, shipped route only). The classifier and this document both use
the full function name, never the bare word "tail", to keep them apart.

## The disambiguation rule

Every named function belongs to at most one component's owned set, chosen by
its primary job, not by which caller happens to invoke it. A handful of
low-level primitives exist purely to serve more than one component
(transaction bookkeeping, checkpoint identity, and pure counters). Those
primitives are explicitly marked shared and are **not** classified directly.

A CPU-profile sample is a full call stack, leaf (currently executing frame)
to root. Attribution walks the stack from the leaf toward the root and
assigns the sample's entire value to the first frame the table recognizes.
A shared primitive is transparent to this walk: the sample "sees through" it
to the nearer ancestor frame, which is always the specific call site that
decided to use the primitive. A sample with no recognized frame anywhere on
its stack is `other`.

This single rule resolves every case where the same function serves two
components:

- **`condense` / `condenseWithOutcomeAtomic`** run from both `Core.Shift*`
  (scheduler-dispatch, appending a shifted GSS node) and reduction-output
  application (reductions-and-pops, appending a reduced parent node). Marked
  shared; the walk finds whichever caller reached it.
- **`ApplySchedulerAtomic` / `RunSchedulerOwned`** wrap the shift, reduce,
  conflict, and extra-shift appliers uniformly (four call sites in
  `parsercore_phase0_driver.go`). Marked shared for the same reason.
- **Checkpoint interning and external-scanner-state capture**
  (`diagnosticParserCoreInternCheckpoint`,
  `captureExternalScannerStateInto`) run both nested inside a live call to
  `dfaTokenSource.Next` (lexing) and directly from `elect` for the
  election's checkpoint-continuity proof (elections). Marked shared; the
  walk finds `Next` when the capture is nested inside token production, and
  finds `elect` when it is not, without any special case.
- **`canonicalize`** is called from inside every applier (accept, shift,
  reduce, conflict, extra-shift) as a post-step. It is deliberately **not**
  marked shared: its job (header-lineage deduplication) is the same
  regardless of caller, so it is classified directly as canonicalization.
  The walk finds `canonicalize`'s own frame before it would reach the
  calling applier's frame, so canonicalization is correctly separated from
  whichever action triggered it.

## The tool

`cgo_harness/attribution` is a small, dependency-free Go program (it lives in
the `cgo_harness` module and imports nothing beyond the standard library,
including its own minimal pprof-profile reader, so this measurement
infrastructure does not widen either module's dependency footprint). It:

1. Extracts and SHA-256-verifies the four canonical fixtures.
2. Builds two test binaries: one tagged `gts_parsercorephase0` (capture and
   noise floor), one tagged `gts_parsercorephase0,gts_workcount` (event
   counts). Building happens once, before any timed measurement, so
   compilation never contends with the host for CPU during a timed sample.
3. Runs `TestParserCoreAttributionCapture` (added in
   `parsercore_phase0_attribution_capture_internal_test.go`, inert unless
   `GTS_ATTRIBUTION_OUT` is set) to collect one gzip'd pprof CPU profile per
   lane per fixture. Every one-time warm pass, digest check, and GC/arena
   drain happens outside `pprof.StartCPUProfile`/`StopCPUProfile`, so the
   profile is not diluted by non-representative setup cost.
4. Runs the existing `TestParserCoreWorkCountChild` (already committed,
   `work_count_parsercore_child_internal_test.go`) once per fixture for
   event counts.
5. Runs the noise floor (see below).
6. Decodes every profile with a minimal, read-only pprof protobuf reader,
   classifies every sample by the rule above, and emits `receipt.json` and
   `receipt.md`.

Reproduce the whole receipt with one command from the repository root:

```sh
(cd cgo_harness && go run ./attribution -repo .. -duration 800ms -noise-samples 10 -noise-benchtime 750ms)
```

Add `-out <dir>` to choose the output directory (default:
`harness_out/perf_attribution/<UTC timestamp>`, already git-ignored). Add
`-core <cpu>` to pin every subprocess with `taskset`; the published receipt
below did not pin a core (see host class, next section).

## Noise floor

**Protocol.** Using the pinned local proxy benchmark
`BenchmarkParserCoreFreshFullCanonical`, the tool builds one test binary and
invokes that identical binary twice per pair, labeled A and B, interleaved
A,B,A,B,... for n pairs (n >= 10). Each invocation runs with `GOMAXPROCS=1`
and times all four fixtures in one process. For each fixture and pair `i`,
`delta_i = |A_i - B_i|`. The noise floor is the 95th percentile (linear
interpolation over the sorted deltas) of `delta_i`, reported both in
nanoseconds and as a percentage of the fixture's median ns/op across the
combined A and B samples.

**Host class.** Shared WSL2 development host, background load uncontrolled.
This is the local-development floor, not the quiet-host or enclave floor
that `BENCH.md`'s sealed epoch uses. Treat every ns/op number in this
document as wall-clock on a busy, shared machine (other agents were active
on the same host during this run), not as a publication-grade performance
claim. The relative component-share percentages are far less sensitive to
this noise than the absolute ns/op and ns-per-event numbers are, because Go's
CPU profiler samples on-CPU (SIGPROF/`ITIMER_PROF`) time, not wall time: host
contention stretches wall-clock ns/op without changing which component a
given on-CPU sample belongs to.

## Cost-per-event join

For each fixture, the tool divides the diagnostic lane's measured wall
ns/op by the `gts_workcount` build's authenticated event counts for that
same fixture and lifecycle (`TestParserCoreWorkCountChild`, schema
`gts-work-count-parsercore-child/v3`): shifts, reductions, emitted pop
paths, emitted pop payloads, canonicalizations, elections, and action
lookups.

This is an **average attribution, not a causal model**. Dividing total wall
time by an event count says nothing about which event class actually causes
the most CPU per occurrence; it says how the average work per fixture spread
across event classes on this run, on this host. Two events of the same class
can cost very different amounts (a pop path over a clean linear GSS chain is
far cheaper than one over a branching, ambiguous chain), and the classes are
not independent (an election's checkpoint-continuity proof is not free, but
it has no "count" of its own in this join).

## Results — first receipt

Generated 2026-08-01T11:12:22Z. Host: shared WSL2 development host,
background load uncontrolled. Other agents were active on the same host
throughout this session (22 registered at session start). Git commit
`f91e9c8cc7d2c8830c973cfa28b658c70dd0d0ba`. Full machine-readable receipt:
`docs/perf-attribution-receipt.json`. Regenerate both (and the larger raw
pprof profiles, which are not committed) with the one command above.

Component shares are computed to sum to exactly 100% of attributed samples
by construction (`other` absorbs the remainder); the displayed 1-decimal
rows are checked to sum to 100% within +/-0.2 percentage points of rounding
tolerance. Every row below is within that tolerance.

### Attribution shares

| lane | fixture | wall ns/op | coverage % | scheduler-dispatch | elections | reductions-and-pops | canonicalization | lexing | materialization | compat-tail | other | sum % |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| diagnostic lane | rewrite | 2.674ms | 105.0 | 22.9 | 9.5 | 33.3 | 4.8 | 14.3 | 13.3 | 0.0 | 1.9 | 100.0 |
| diagnostic lane | query_compile | 12.340ms | 104.8 | 20.8 | 4.7 | 30.2 | 8.5 | 19.8 | 16.0 | 0.0 | 0.0 | 100.0 |
| diagnostic lane | language | 13.195ms | 105.7 | 22.6 | 7.5 | 33.0 | 7.5 | 13.2 | 16.0 | 0.0 | 0.0 | 99.8 |
| diagnostic lane | grammargen_lr | 126.019ms | 106.1 | 25.2 | 7.5 | 22.4 | 5.6 | 19.6 | 17.8 | 0.0 | 1.9 | 100.0 |
| shipped route | rewrite | 2.707ms | 104.8 | 21.9 | 7.6 | 28.6 | 9.5 | 19.0 | 11.4 | 0.0 | 1.9 | 99.9 |
| shipped route | query_compile | 13.026ms | 104.7 | 30.5 | 3.8 | 21.9 | 1.0 | 20.0 | 22.9 | 0.0 | 0.0 | 100.1 |
| shipped route | language | 11.753ms | 105.9 | 25.2 | 4.7 | 30.8 | 6.5 | 15.9 | 16.8 | 0.0 | 0.0 | 99.9 |

`coverage %` is profiled CPU time divided by measured wall time on this
single-core-bound (`GOMAXPROCS=1`) run. It runs 105-106% here, not 100%,
because the Go runtime's background workers (GC assist, sweep) can add a
small amount of concurrent on-CPU time beyond the profiled goroutine's own
wall-clock window. A coverage figure far below 100% would mean too few
samples landed to trust the percentages; every row here is comfortably
above that concern.

`grammargen_lr` has no shipped-route row: it is above the 64 KiB floor, so
`Parser.Parse` never routes it through the compact candidate. `compat-tail`
measures zero on every shipped-route row: for these four fixtures every
parse is clean (no error nodes, no C-recovery-swallowed error to resolve),
so the shipped-route fidelity tail's conditional work never fires. This is
a genuine finding, not a classifier gap: the compat-tail functions exist for
correctness parity on error/recovery paths this receipt does not exercise,
not for cost on the clean path.

`other` stays at 0.0-1.9% on every row. Where it is nonzero, the largest
non-runtime contributors are `(*nodeArena).reset` (arena-pool bookkeeping
inside `Tree.Release`), `hiddenTreeHasFieldIDs`, and
`maxRetainedNodeCapacityForClass` — each under 1% of one profile's samples.
None of them changes the qualitative picture; they are recorded as a known
small residual rather than folded into a component that would overstate its
weight.

### B3 stage S1 addendum: the `recovery` component

The table above predates the `recovery` component (added to
`cgo_harness/attribution/classify.go` in campaign v7 tranche B3 stage S1, per
gate G5) and so has no `recovery` column. A local reproduction of the same
command on stage S1's branch, after the classifier change and before any
recovery engine code, confirmed `recovery` reads exactly 0.0% on every lane
and fixture — diagnostic lane (`rewrite`, `query_compile`, `language`,
`grammargen_lr`) and shipped route (`rewrite`, `query_compile`, `language`)
alike — matching `compat-tail`'s 0.0% pattern above. This is expected: the
component owns no functions yet (B3 stages S2-S5 add error-cost,
absorb/condense-resume, election, and missing-token code class by class), so
no sample can land there. This local run is a verification check, not a new
sealed or C0-authoritative epoch; it does not replace the receipt above or
`docs/perf-attribution-receipt.json`. The next full regeneration (any future
tranche that re-seals this board) will show `recovery` alongside the other
eight components by construction, with no further classifier change needed.

### B3 stage S2 addendum: the inert error-cost model

Stage S2 added `internal/parsercorephase0/recovery_cost.go`: compact
equivalents of `cNodeErrorCostLang`, `cSymbolVisibleLang`, and
`cVersionStatus`, callable on demand but wired into nothing (no call site
exists outside the file's own tests; a compile-time AST ratchet,
`TestRecoveryCostNoOutsideCallSitesRatchet`, enforces this). A local
reproduction of the documented command on stage S2's branch, with the file
present, again confirmed `recovery` reads exactly 0.0% on every lane and
fixture — diagnostic lane and shipped route alike. Every other component
share stayed within this host's published noise floor of the stage-S1-era
receipt above; the local noise floor measured on this run (8.2%-11.9% of
median ns/op) is consistent with that same shared, uncontrolled-host
variance, not a new finding. As with the S1 addendum, this is a
verification check, not a new sealed epoch.

### 2026-08-02 addendum: the provenance-predicate flip receipt

The compact scheduler's no-action head drop now decides with the
alternative-set containment predicate by default. The scalar rank-walk
(`markCleanProductionRank`) still exists in the source. It now runs only
under a diagnostic flag (`GTS_B4B_SHADOW_CENSUS`), not on the default
path. Alternative-set recording used to run only under that same flag. It
now runs unconditionally, because the containment predicate needs a
populated set to decide.

This addendum re-runs the documented command. It adds one targeted
profile of the changed functions. The goal is one question: how much CPU
does the changed provenance machinery cost today, and where should the
next performance lever land.

#### Component shares (regenerated receipt)

Generated 2026-08-02T04:38:36Z. Host: shared WSL2 development host,
background load uncontrolled. Git commit
`193c44b340538b78770da68e4246abd253c81710`. This run's noise floor (see
below) sits far above the first receipt's 7.4%-10.8%. Read every share
below as a rough reading, not an exact one. This run's raw JSON output was
not committed, matching the existing `harness_out/` git-ignore rule;
rerun the documented command to reproduce it.

| lane | fixture | wall ns/op | scheduler-dispatch | elections | reductions-and-pops | canonicalization | lexing | materialization | compat-tail | recovery | other |
|---|---|---|---|---|---|---|---|---|---|---|---|
| diagnostic lane | rewrite | 2.389ms | 21.6 | 8.0 | 22.7 | 10.2 | 21.6 | 14.8 | 0.0 | 0.0 | 1.1 |
| diagnostic lane | query_compile | 11.201ms | 26.1 | 8.0 | 20.5 | 8.0 | 22.7 | 13.6 | 0.0 | 0.0 | 1.1 |
| diagnostic lane | language | 12.511ms | 14.8 | 6.8 | 25.0 | 15.9 | 15.9 | 21.6 | 0.0 | 0.0 | 0.0 |
| diagnostic lane | grammargen_lr | 125.275ms | 22.9 | 7.3 | 20.8 | 15.6 | 15.6 | 16.7 | 0.0 | 0.0 | 1.0 |
| shipped route | rewrite | 2.452ms | 21.6 | 6.8 | 31.8 | 11.4 | 10.2 | 15.9 | 1.1 | 0.0 | 1.1 |
| shipped route | query_compile | 11.824ms | 21.6 | 4.5 | 25.0 | 10.2 | 18.2 | 20.5 | 0.0 | 0.0 | 0.0 |
| shipped route | language | 12.202ms | 10.2 | 4.5 | 33.0 | 12.5 | 20.5 | 19.3 | 0.0 | 0.0 | 0.0 |

`recovery` reads 0.0% on every row. This matches every prior receipt: the
inert cost model from stage S2 still has no call site. The `compat-tail`
reading for shipped-route `rewrite` (1.1%) is one profiled sample out of
roughly ninety. Treat it as noise, not a new conditional-cost finding,
given this run's wide noise floor.

Scheduler-dispatch, elections, reductions-and-pops, and canonicalization
together still cover 60.2%-71.6% of wall time across every fixture above.
No lever in the ranked list below reaches that scale on its own; see item
5.

#### Noise floor (this run)

| fixture | pairs | median ns/op | p95 \|delta\| | p95 \|delta\| as % of median |
|---|---:|---:|---:|---:|
| `grammargen_lr` | 10 | 122.945ms | 68.439ms | 55.7% |
| `rewrite` | 10 | 2.448ms | 1.038ms | 42.4% |
| `query_compile` | 10 | 11.862ms | 4.626ms | 39.0% |
| `language` | 10 | 12.123ms | 3.725ms | 30.7% |

Other automated workers ran concurrent CPU-bound jobs on this host
throughout this run. Read every ns/op number above as directional, not
exact. Component shares still rest on firmer ground than raw ns/op does:
the CPU profiler samples on-CPU time, so host contention stretches
wall-clock duration without moving which component a sample belongs to
(see "Noise floor," above).

#### Targeted profile: the provenance-predicate family

The component table above answers where compact wall time goes, across
nine wide boundaries. It does not answer a narrower question: how much of
the reductions-and-pops and canonicalization shares still belongs to the
mechanism this flip changed. This section profiles that mechanism by
function name.

Method: `go test -tags gts_parsercorephase0 -bench
BenchmarkParserCoreFreshFullCanonical/query_compile -benchtime 6s
-cpuprofile`, `GOMAXPROCS=1`, pinned to one core with `taskset -c 0`,
`GTS_B4B_SHADOW_CENSUS` unset (the shipped default). 630 iterations, 9.55
seconds of profiled samples. `query_compile` matches the fixture the
original regression bisect used, so the reading below is
fixture-consistent with that bisect. The bisect's own numbers predate
this tool's interleaved-capture methodology, so treat the comparison as
fixture-consistent, not protocol-identical.

| function | flat % | cum % |
|---|---:|---:|
| `condenseWithOutcomeAtomic` | 5.1% | 9.2% |
| `persistHeaderLineageOwned` | 1.5% | 7.3% |
| `RecordHeadLineageOwned` | 0.2% | 4.0% |
| `RecordHeadOwnerOwned` | 0.2% | 1.9% |
| `recordNodeLineageSet` | 0.5% | 2.1% |
| `recordNodeLineage` | 0.5% | 0.7% |
| `RecordReductionLineageOwned` | 0.0% | 0.4% |
| `alternativeSetInsert` | 0.1% | 0.3% |
| `alternativeSetUnion` | 0.0% | 0.3% |
| `alternativeSetSortedContainsAll` | 0.0% | 0.0% |
| the v2 containment predicate (`dropGenericNoActionHeads`'s decider) | 0.0% | 0.1% |
| `enterLiveCondenseCandidates` | 0.3% | 0.3% |
| `clearLiveCondenseCandidates` | 0.1% | 0.1% |
| `condenseNodeIsLive` | 0.2% | 0.2% |
| `runLiveCondenseCandidates` (old closure-wrapper form) | 0.0% | 0.0% |
| `markCleanProductionRank` | 0.0% | 0.0% |
| `walkCleanPrefixRanks` | 0.0% | 0.0% |

`condenseWithOutcomeAtomic` is a shared GSS merge primitive that predates
the regression; it is not itself a provenance-family member, and it is
the single largest function this profile named. `condenseNodeIsLive` runs
nested inside it. `RecordHeadLineageOwned` and `RecordHeadOwnerOwned` run
nested inside `persistHeaderLineageOwned`, as two separate transactions,
not one inside the other; `recordNodeLineageSet`, `recordNodeLineage`,
`alternativeSetInsert`, and `alternativeSetUnion` run nested inside
`RecordHeadLineageOwned` in turn. None of these nested cum numbers add on
top of their parent row.

Pre/post comparison, rolled up to the mechanism level (query_compile;
pre-flip numbers from the regression bisect, at the commit that
introduced them):

| mechanism | pre-flip cumulative CPU | today's cumulative CPU |
|---|---|---|
| condense live-scoping and liveness check (`enterLiveCondenseCandidates`, `clearLiveCondenseCandidates`, `condenseNodeIsLive`) | 26%-30% (as the old closure-wrapper form, `runLiveCondenseCandidates`) | 0.6% |
| lineage-and-set recording (`persistHeaderLineageOwned` and everything nested below it, plus `RecordReductionLineageOwned`) | canonicalization's whole component share grew by +3.8 percentage points (3.0% to 6.8%) when this family was introduced, per the bisect's own before/after reading; the family did not yet have a separate cum number | 7.7% |
| scalar rank-walk (`markCleanProductionRank`) | part of the two rows above; not separated at bisect time | 0.0% (retired) |
| v2 containment predicate (decider) | did not exist | 0.1% |

This run's own component table (above) reads canonicalization at 8.0% for
`query_compile`, diagnostic lane. Lineage-and-set recording alone (7.7%)
now accounts for nearly all of that share. Canonicalization today is
almost entirely lineage-and-set recording, not the header-merge mechanics
the component predates.

Two readings follow.

The retirement holds. `markCleanProductionRank` and `walkCleanPrefixRanks`
carry zero samples on the default path. `runLiveCondenseCandidates`'s
closure-wrapper form also carries zero samples: an earlier, separate fix
replaced its call sites with direct calls before this predicate flip
landed. Both retirements match their design intent.

The residual moved; it did not vanish. Live-condense scoping fell from
26%-30% to 0.6%, but lineage-and-set recording rose from a gated-off 0%
to 7.7%, because recording is now unconditional. The family's live cost
today concentrates almost entirely in `persistHeaderLineageOwned` and its
two callees (7.3% cumulative), not in `alternativeSetInsert` or
`alternativeSetUnion` (0.3% cumulative, combined). An overflow fast path
already made the set math cheap: on this corpus, most sets saturate their
cap early, and both functions return immediately once a set carries the
overflow flag. The remaining cost sits above that math, in the per-header
transaction wrapper that runs whether or not the math underneath does
anything.

#### Ranked residual levers

Five candidate levers, ranked by addressable share:

1. **Further live-condense scoping reduction.** Addressable share: under
   1% cumulative CPU (0.6% combined, `query_compile`, the three-function
   row above). An earlier de-indirection fix already captured this
   lever's addressable share. The remainder is the scoping check's own
   cost, not avoidable call overhead. Not recommended: too little room
   left.
2. **Stage-3 scalar-field deletion.** `nodeLineageRecord` carries 36 bytes
   today, measured directly (`unsafe.Sizeof`). Two fields marked
   "transition only" in their own source comment, a 2-byte lineage ID and
   a 1-byte rank, could shrink it by roughly 11%. Both fields sit inside a
   component that already costs under 8% of wall time, so their removal
   has a ceiling near or under 1% of wall time. Worth doing for code
   hygiene; not a performance lever on its own.
3. **Recording-overhead reduction.** The set math is now cheap. The
   transaction wrapper around it, for each header, is not. Today,
   `persistHeaderLineageOwned` opens one scheduler-owned transaction for
   owner persistence on every header (`RecordHeadOwnerOwned`, 1.9%
   cumulative), and a second, separate transaction for lineage-and-set
   persistence on headers with a converged reduction split
   (`RecordHeadLineageOwned`, 4.0% cumulative). One source comment already
   states the merge goal for the second transaction's own two halves:
   halve the per-header token-validation cost. Folding the owner write
   into the same transaction is not free: the owner write fires on every
   header, and the lineage write fires only on converged ones, so a naive
   merge would change which headers get an owner write. A follow-up needs
   to preserve that distinction inside one transaction. Addressable
   share, before subtracting the work that must stay: up to 7.3%
   cumulative CPU on `query_compile`.
4. **The remaining L1 deterministic-frontier follow-ons.** An earlier
   lever selection modeled five single-head-frontier fast-path sub-items.
   A shipped change captured two of them: a checkpoint-digest skip inside
   election, and a canonical-group-key skip inside single-head
   canonicalization (roughly 4.3 modeled percentage points of an 8.1
   modeled total). Two more never landed: a direct-append path for
   `condenseWithOutcomeAtomic` on an exact single pop path, and a
   validation skip inside the dispatch loop's per-cell checks. Modeled
   addressable share for the two open sub-items: roughly 3.8% combined,
   against a `condenseWithOutcomeAtomic` share this run measured at 9.2%
   cumulative, the single largest function this profile named. The model
   predates this run by several days of intervening change and needs
   re-validation before a bet. The target function is still large and
   still un-optimized.
5. **The bytecode-scheduler structural bet.** Scheduler-dispatch,
   elections, reductions-and-pops, and canonicalization together still
   cover 60.2%-71.6% of wall time on every fixture in the component table
   above. None of the other four levers reach that scale; this is a
   separate, larger, and longer-horizon bet.

Recommendation: pursue item 4 next, ahead of item 3. Item 4 targets one
named function this run confirmed still costs 9.2% cumulative CPU. It
already has a capture model and two concrete, previously scoped
sub-items. Item 3 is real, but smaller: an overhead layer on an
already-cheap operation, entangled with a correctness distinction a
follow-up must preserve. It needs its own follow-up profile, to separate
wrapper cost from the work that must stay, before a precise size. Land
items 1 and 2 opportunistically. Neither earns a dedicated lever slot.
Item 5 stays the standing structural option, for whenever items 3 and 4
stop paying.

### Noise floor (interleaved A/A, identical binary, 12 pairs per fixture)

| fixture | median ns/op | p95 \|delta\| | p95 \|delta\| as % of median |
|---|---:|---:|---:|
| `grammargen_lr` | 123.099ms | 9.069ms | 7.4% |
| `language` | 12.020ms | 1.299ms | 10.8% |
| `query_compile` | 11.772ms | 1.165ms | 9.9% |
| `rewrite` | 2.401ms | 0.256ms | 10.7% |

Read this as: on this shared, uncontrolled host, two runs of the literal
same binary can disagree by roughly 7-11% before you have measured any real
difference. Any comparison this receipt states below that floor is not a
finding; it is noise.

### Cost per event (diagnostic lane; average, not causal)

| fixture | wall ns/op | ns/shift | ns/reduction | ns/pop path | ns/pop payload | ns/canonicalization | ns/election | ns/action lookup |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| `rewrite` | 2.674ms | 1984 | 1778 | 1625 | 894 | 1093 | 2581 | 753 |
| `query_compile` | 12.340ms | 1846 | 1643 | 1522 | 838 | 998 | 2407 | 708 |
| `language` | 13.195ms | 2026 | 1745 | 1575 | 834 | 1071 | 2609 | 707 |
| `grammargen_lr` | 126.019ms | 1906 | 1651 | 1506 | 830 | 1033 | 2525 | 684 |

The top-3 most expensive classes, by ns/event and consistent across all four
fixtures, are elections (2,407-2,609 ns), shifts (1,846-2,026 ns), and
reductions (1,643-1,778 ns). Elections costing more per event than shifts or
reductions is consistent with the shares table: on the diagnostic lane,
elections is the smallest of the four scheduler-family CPU shares
(4.7-9.5%) but fires the fewest times per fixture (1,036 on `rewrite`
versus 3,551 action lookups), so each election carries relatively more
weight. Full JSON: `docs/perf-attribution-receipt.json`.

### Which historical figure this receipt speaks to

Sum the four diagnostic-lane scheduler-family components
(scheduler-dispatch + elections + reductions-and-pops + canonicalization)
without lexing: 70.5% (`rewrite`), 64.2% (`query_compile`), 70.8%
(`language`), 60.7% (`grammargen_lr`) — average 66.5%. That number, on its
own, matches none of the three historical figures well.

Add lexing to that sum (scheduler-dispatch + elections +
reductions-and-pops + canonicalization + lexing): 84.8%, 84.0%, 84.0%,
80.3% — average 83.3%. This closely reproduces the **77-79% and 78-82%**
figures. Read literally, this means those two historical estimates most
likely counted token production as part of "scheduler cost per event" —
a defensible reading, since a shift cannot happen without first lexing the
token it shifts, but one this document did not previously state.

Sum only scheduler-dispatch and reductions-and-pops, the two components that
directly apply a table-driven action (excluding election/conflict overhead,
canonicalization bookkeeping, and lexing): 56.2%, 50.9%, 55.7%, 47.7% —
average 52.6%. This closely reproduces the **approximately-50%** figure. That
estimate most likely measured only the two dominant mechanical appliers and
implicitly folded election, canonicalization, and lexing cost into
"overhead" it did not separately name.

**Call: this receipt retires all three historical figures as decision
inputs**, not because any of them was numerically wrong, but because none of
them stated a boundary a later reader could reproduce or falsify. Each is a
reasonable reading of a different implicit boundary that this receipt now
makes explicit. From this tranche forward, campaign v7 should cite the
eight-component table above — with its stated boundary and disambiguation
rule — instead of any of the three legacy percentages.

### B9 addendum: the wall-removal receipt

Campaign v7 tranche B9 removed the compact admission switch's 64 KiB
source-length eligibility decline (`admission_switch.go`). The shipped route
now attempts every eligible full parse regardless of size; a scheduler
stop-control poll (tranche B8) bounds a large or pathological input instead
of a size wall. This addendum re-runs the documented command with
`grammargen_lr` added to the shipped-route lane (diagnostic-lane only in
every receipt above) and reports what changed. Per doctrine 10 (board
honesty), every row below still names its lane; nothing here merges a
diagnostic-lane number into a shipped-route claim or vice versa.

Generated 2026-08-02T11:02:14Z. Host: shared WSL2 development host,
background load uncontrolled -- the same local-development floor as every
prior receipt in this document, not a quiet-host or enclave measurement.
Git identity: `6fab988d30d42d931d42625eaa0cfa167d4877ba` (the capture-time
base commit; the wall-removal change is a working-tree diff on top of it at
capture time, so the tool's `git rev-parse HEAD` names the base, not a
B9-specific commit). This run's raw JSON output was not committed, matching
the existing `harness_out/` git-ignore rule; rerun the documented command to
reproduce it.

#### Attribution shares (regenerated; shipped route now covers all four fixtures)

| lane | fixture | wall ns/op | coverage % | scheduler-dispatch | elections | reductions-and-pops | canonicalization | lexing | materialization | compat-tail | recovery | other |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| diagnostic lane | rewrite | 2.381ms | 110.9 | 23.6 | 6.7 | 22.5 | 11.2 | 11.2 | 23.6 | 0.0 | 0.0 | 1.1 |
| diagnostic lane | query_compile | 11.548ms | 110.1 | 21.3 | 6.7 | 28.1 | 11.2 | 14.6 | 18.0 | 0.0 | 0.0 | 0.0 |
| diagnostic lane | language | 12.156ms | 109.7 | 26.1 | 2.3 | 28.4 | 12.5 | 15.9 | 13.6 | 0.0 | 0.0 | 1.1 |
| diagnostic lane | grammargen_lr | 126.493ms | 110.7 | 27.6 | 5.1 | 23.5 | 12.2 | 12.2 | 17.3 | 0.0 | 0.0 | 2.0 |
| shipped route | rewrite | 2.480ms | 109.9 | 15.9 | 3.4 | 30.7 | 12.5 | 21.6 | 15.9 | 0.0 | 0.0 | 0.0 |
| shipped route | query_compile | 11.661ms | 110.6 | 15.7 | 4.5 | 30.3 | 15.7 | 13.5 | 20.2 | 0.0 | 0.0 | 0.0 |
| shipped route | language | 12.782ms | 110.5 | 16.9 | 4.5 | 30.3 | 9.0 | 18.0 | 20.2 | 0.0 | 0.0 | 1.1 |
| **shipped route** | **grammargen_lr** | **140.751ms** | 111.3 | 11.7 | 10.6 | 29.8 | 8.5 | 19.1 | 20.2 | 0.0 | 0.0 | 0.0 |

`grammargen_lr` has a shipped-route row for the first time. The public
`Parser.Parse` route, with no per-Parser pin and no diagnostic flag, admitted
it at 140.751ms against the diagnostic lane's 126.493ms for the same
fixture -- a wall-time gap of 11.3% (not to be confused with the unrelated
111.3 coverage % figure in that same table row above; coverage % is
profiled-CPU-time over wall-time, see "Attribution shares" earlier in this
document). Read this 11.3% gap against the noise floor below before treating
it as a finding. A same-shape gap would be consistent with the compat-tail
and admission-switch bookkeeping the shipped route pays and the diagnostic
runner does not (`normalizeReturnedTreeForParse`,
`resolveCRecoverySwallowedError`, `maybeCompactReturnedFullTree`, plus
`Parser.Parse`'s own dispatch); `compat-tail` itself still reads 0.0% because
this run's four fixtures stay clean (no error nodes, no C-recovery-swallowed
error to resolve), so any such cost here is admission-switch dispatch
overhead, not compat-tail's own conditional work. Every shipped-route sample
was verified, by the admission-candidate routed/fallback counters, to have
actually taken the compact route rather than falling back to production.

#### Noise floor (this run, interleaved A/A, 10 pairs per fixture)

| fixture | pairs | median ns/op | p95 \|delta\| | p95 \|delta\| as % of median |
|---|---:|---:|---:|---:|
| `grammargen_lr` | 10 | 120.831ms | 27.151ms | 22.47% |
| `language` | 10 | 12.164ms | 2.994ms | 24.61% |
| `query_compile` | 10 | 11.770ms | 4.986ms | 42.36% |
| `rewrite` | 10 | 2.458ms | 378.273us | 15.39% |

This run's noise floor is wide (15.4%-42.4% of median), consistent with a
busy shared host (other agents were active throughout this session). The
11.3% shipped-route/diagnostic-lane gap on `grammargen_lr` above sits BELOW
every one of these four floors, including `grammargen_lr`'s own (22.47%) and
`rewrite`'s (the narrowest, at 15.39%). This run's noise cannot resolve that
gap from zero: it is not a finding, directional or otherwise, until a
quieter run repeats the measurement with a floor tighter than 11.3%.

#### Cost per event (diagnostic lane; average, not causal)

| fixture | wall ns/op | ns/shift | ns/reduction | ns/pop path | ns/pop payload | ns/canonicalization | ns/election |
|---|---:|---:|---:|---:|---:|---:|---:|
| `rewrite` | 2.381ms | 1766 | 1583 | 1446 | 795 | 973 | 2298 |
| `query_compile` | 11.548ms | 1728 | 1538 | 1424 | 784 | 934 | 2253 |
| `language` | 12.156ms | 1867 | 1607 | 1451 | 769 | 987 | 2403 |
| `grammargen_lr` | 126.493ms | 1913 | 1658 | 1512 | 833 | 1036 | 2534 |

Every per-event reading sits within this document's established noise
floors from prior receipts. The wall-removal change moved routing, not the
compact engine's per-event cost.

This is a local, one-run, noisy-host receipt, not a sealed epoch. It does
not replace or invalidate the first receipt or its addenda above (all
measured before tranche B9 landed, correctly showing `grammargen_lr`
diagnostic-lane only at that time); it extends the same methodology to the
newly shipped-route-eligible fixture.

## Caveats

- This is one run, on one noisy shared host, not a sealed-epoch,
  quiet-host, or enclave receipt. Treat the noise floor as a floor, not a
  ceiling: a quiet or enclave host would very likely show tighter deltas.
- The classifier's function table
  (`cgo_harness/attribution/classify.go`) is the single source of truth for
  every boundary rule in this document. If the two ever disagree, the code
  is authoritative; file a follow-up to reconcile the prose.
- `other` includes Go runtime and GC frames by design (a legitimate,
  expected component), not only classification gaps. The tool reports the
  largest non-runtime unclassified functions separately so a real gap is
  visible rather than silently absorbed.
- Cost-per-event numbers are an average, not a causal attribution (see
  above). Do not use them alone to justify a specific optimization target;
  pair them with the component shares.
