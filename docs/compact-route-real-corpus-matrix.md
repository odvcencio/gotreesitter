# Compact route real-corpus matrix

Current evidence date: 2026-10-25 (UTC).
Parent PR #1486 merged to main as `88dc1d9c3ff15cc2b8b5c53864455fc3a24a9d34`
after all 78 checks passed. Branch integration `b876c5b5` preserves all 414
ledger rows and passes focused TypeScript/TSX C checks. The final whole-PR matched-order audit passes all 414 rows and 480 timings;
[PR #1487](https://github.com/odvcencio/gotreesitter/pull/1487) merged as
`a8ecfc80cac24b7921f464386bfefa8db3840426` after 74 successful checks and
four skips, including the completed editor-latency gate. Integrating that
squash leaves all 3524 Go files in the qualified wrapper assembly unchanged.
Historical receipts below retain their own source scope.
Release update, 2026-10-17 UTC: v0.56.0 was published on October 14. The
release condition behind the merge hold is fulfilled; current reviews and CI
still govern landing. Historical hold statements below retain their original
qualification context.
Latest core link-comparison baseline: `fb4c8acc4de8cbd14da37c3dff13fa6b2bc4b054`.
The latest stateless-probe capacity comparison uses core parent `e4243c77ad9ede0aea152331111106e3cc417c28`.
The read-history comparison uses incremental-lane parent `a977238f9eb1548ea671dedecb6750f58906bc43`.
The earlier leaf-allocation comparison uses incremental-lane baseline `a32d87b6d23b090fef2559380b44bfb2c86a800e`.
Earlier comparisons below name their own baselines. Canonical Go files
`query_compile` and `grammargen_lr` contain 20,168 and 235,626 bytes respectively.
Scope: the 204 committed real `sample` files in the R4 manifest. CSV and Enforce
have only synthetic samples and are outside this real-file census.

## Current compact-graduation matrix

Current owner instruction: **hold all merges to main**. The earlier release condition was fulfilled, but the renewed hold remains active independently of that release. PR #1512 stays draft; branch-relative experiments do not graduate a language.

### Projection follow-up: saved-source correctness and counter gates pass

The [CI follow-up receipt](benchmarks/20261025-projection-ci-followup/README.md) preserves selected MISSING history for Go grouped declarations, repairs COBOL's source-backed EXEC/AND/CICS header handling, and fixes the EXEC error's pre-existing `Extra` mismatch. The permanent COBOL gate now checks all flags and independent C digests. Both Go incremental APIs pass the separator delete/restore cycles on both routes with zero no-edit allocations. Cooklang retains all 19 previously C-exact results; only HasError bits change in its existing complex-witness diagnostic, whose C gap remains explicit.

Saved files pass **412 sessions / 29,664 edits**, **618 C fixture checks**, the root/API guard at **756/756 files**, and all **412 ledger / 24 complete Go work comparisons unchanged**. The sessions include 408 real R4 and four CSV/Enforce grammar-smoke cases. The four C/EDS owner exceptions were approved on 2026-10-25; the earlier Go assignment exception remains unapproved. No timing, RSS, native-recovery admission or graduation improvement is claimed by this follow-up. All merges remain held.

### Go graduation: reserved words and retained missing-token errors qualified

The [reserved-word and missing-token receipt](benchmarks/20261025-reserved-missing/README.md) repairs the original frozen Go `}func` boundary and its 31-byte reduction on both routes, including two delete/restore cycles, without suppressing C's errors. Shared missing-token retention also repairs a C first edit and EDS section error flags. All **412 × 72-step invariant sessions**, **618 C fixture checks**, focused memory/race/publication gates and saved-source checks pass. All **24 complete Go operation records** remain identical. Of 412 fleet ledger rows, 410 remain identical; the owner approved the [four C/EDS first-edit exceptions](benchmarks/20261025-reserved-missing/counter-review.md) on 2026-10-25. Only their four guarded ledger values are updated; thresholds remain unchanged. The earlier Go assignment exception remains separate and unapproved.

Eighty matched before/after pairs show default Go full parsing **4.017→3.897 ms (−2.98%, p<.001)**, but no significant compact Go/YAML throughput improvement. Current R4 paired compact/legacy median ratios are **1.1678 / 1.4083**. Both still exceed the E-A worst-file limit; these diagnostics do not authenticate frozen M1. Native non-EOF recovery and complete included-range C equality remain open. No language graduates, no reuse or default switch changes, and all merges remain held.

### Earlier Go recovery repair: counters and graduation remain held

The [Go graduation receipt](benchmarks/20261025-go-graduation-recovery/README.md) repairs the remaining `= a + b` recovery election with Go's exact-artifact missing-version scheduling grant. All 96 assignment-target edits (16 sites per class, both routes) match independent fresh/incremental Go and C; both 72-step invariant sessions, focused memory/race/publication checks and the repaired canonical controls pass. The newer receipt above completes broader final-source Go C qualification. The standard Go ledger and original declaration-witness work stay identical, but the new malformed-assignment fixture raises tokens, nodes, stacks and merge work. Its [R3 exception](benchmarks/20261025-go-graduation-recovery/counter-review.md) remains unapproved. The full version-order alternative is rejected for adverse work.

Forty before/after pairs across both Go routes find no significant timing change from the correctness fix; allocations stay default 6/5/0 and candidate 22/5/0. A separate frozen **all-206** current-runtime census was deferred after 139 complete languages and a partial Org block, including declines and locally authenticated CSV/Enforce samples; it is incomplete. Its completed Go/YAML 20-pair snapshots have median paired compact/legacy ratios **1.1720 / 1.4317**, both executing compact with zero fallbacks. These exceed E-A's worst-file 1.15 limit, and the frozen M1 baseline is not authenticated. Upstream C 0.27.1 is resolved by commit, but its compatibility preflight is deferred and unqualified. Go remains ungraduated, V4 stays deferred, and all merges stay held.

### Public compact trees: Go publisher enabled within the candidate route

The [public-output receipt](benchmarks/20261024-public-compact-views/README.md) connects owned compact snapshots to the public tree, query, copy, and edit APIs. A parse creates one stable root header; child reads create views on demand, and edits restore the complete mutable representation with replay/checkpoint/reuse receipts. The full fresh verifier remains enabled. Go uses this publisher inside its existing candidate route; YAML requires the experimental output tag. Admission allowlists and the global route default are unchanged.

Twenty paired runs measure Go full-DFA time **−12.25%**, bytes **−39.11%**, and allocations **36→22**. Public 235 KiB Go parse time falls **8.31%**, with **59.09% fewer allocated bytes**. Legacy Go/YAML traversal improves **21.14% / 25.25%**. All **1,040** before/after rows pass the relative ratchets. YAML parse speed is inconclusive; its traversal and edit slowdowns keep it experimental. Go query/edit allocation increases, legacy query slowdowns, and no-edit timing increases remain explicit in the receipt.

All **160** large-file memory processes are retained, including initial screening pairs. Root-only total live heap falls about **24%** for both languages. Fully traversed trees retain both representations and increase live heap **14.64% / 6.46%**; peak RSS rises **8.63–9.27% / 3.79–3.90%**, within the unchanged relative and absolute gates. Correctness covers **412 invariant sessions**, **29,664 edits**, **1,236 standard C checks**, race/386/emergency checks, and 20 memory contracts. The 824 work rows and 206 selected ledgers have no adverse changes. The latest Cognociente replay preserves twelve independent fresh agreements and eight clean C passes, with the same four malformed-delete C mismatches. The 640-row locked native-C diagnostic measures **2.63–2.82× C for Go** and **1.90–2.02× C for YAML**; it is not the standalone-static graduation protocol. Main-based net, platform, C-correctness, graduation, and merge holds remain.

### Compact result ownership: retained graph removed

The [ownership receipt](benchmarks/20261024-compact-output-ownership/README.md) separates result recycling from the producing core. Across twenty matched pairs per language, keeping a compact result after discarding its parser retains **152.21→46.14 MB for 1 MiB Go (−69.69%)** and **27.67→5.77 MB for 1 MiB YAML (−79.16%)**, with identical output and work. This earlier checkpoint uses the internal read-only route; public integration is qualified separately above. Peak RSS changes only −1.91% / −0.05%; all thirteen timing comparisons are inconclusive, so this is a lifetime/memory bucket with no throughput-win claim.

All **824 complete-operation rows**, **206 selected ledgers**, **412 invariant sessions**, **1,236 standard C checks**, and **20 memory-contract tests** pass. All 520 timing rows pass the configured ratchets, and no-edit remains allocation-free. The exact Cognociente replay retains twelve independent fresh agreements, eight clean C passes, and the same four malformed-delete C failures. Main-based net, public-output, platform, correctness, graduation, and merge holds remain.

### Single-version loop and direct compact output

The [new qualification bucket](benchmarks/20261024-single-version-output/README.md) builds the owned deterministic loop and an internal read-only compact output route. Twenty paired runs measure public compact Go full DFA time **8.51% lower**, fresh-parser 1 MiB time **3.06% lower**, and real Go files **1.08–1.52% lower**. The direct read-only route cuts real Go/YAML parse time **11.61–13.48%**; generated 1 MiB Go takes **16.42% less parse time**, allocates **65.61% fewer bytes**, and falls **673→30 allocations**. Traversing every output node retains the gain. These read-only numbers are not public `*Node` API improvements.

All **824 complete-operation work rows**, **206 selected ledgers**, **412 invariant sessions**, and **1,236 standard C checks** pass unchanged. The known Cognociente malformed-delete C mismatch remains, as do the fresh verifier, public-output integration, memory/net-PR qualification, and graduation holds. The receipt separates actual public routing, read-only consumption, controls, and directional regressions.

### Trusted small-source verification: measured Go edits, held counters

The [qualification receipt](benchmarks/20261021-trusted-small-source-results/README.md) records 20 matched Mac Docker ARM64 pairs on Cogno’s exact Go fixture: plain insert **107.981→5.705 ms** and replace **95.971→5.697 ms** (both p<.001). Tree.Edit becomes slower; delete timing is inconclusive. The 178 adverse selected counter changes and 158 complete-operation event increases remain unapproved. Two YAML screens are rejected, and ordinary ownership/graduation stay unchanged. This receipt promotes no runtime or ledger baseline.

### Fused scanner checkpoint receipts: preliminary screen held

The [Mac ARM64 screen](compact-fused-checkpoint-screen-20261021.md) retains all **480 rows / 80 processes** and exact route/node/token counters. All twelve timing differences are nonsignificant, and all median ratchets pass. Focused correctness checks preserve **414 ledger rows**, four Go vectors and actual C/race/386 checks; full-fleet and RSS qualification were not run. E stays held, the existing native-YAML net hold remains, and no runtime or graduation change follows.

### Sixteen-byte graph nodes: qualified allocation and capacity reduction

The [lossless count-storage receipt](compact-node16-path-count-receipt-20261020.md) preserves all **414 ledger rows**, four canonical vectors and full correctness gates. One-MiB Go allocated bytes fall **6.79%**, ordinary YAML **5.19% / 6.78%** (all p<.001), while the pinned Go node arena sheds **8 MiB**. All 480 timing/allocation ratchets and 126 RSS processes pass. All twelve timing differences are inconclusive; ordinary YAML routing, known Go C failures and graduation remain unchanged.

### YAML storage growth: remaining relative RSS gap

The [reserve and checkpoint screens](compact-yaml-storage-growth-screen-20261020.md) retain three private sources and all **378** RSS processes. Corrected complete-lexicon density and paged immutable checkpoint records preserve focused C output and all 50 work/reuse comparisons; their ordinary YAML median ratios are **1.239112 / 1.186330**, still above 1.10. The earlier activation-only screen misses the intended reserve branch and remains recorded separately. No runtime, full-fleet qualification, timing, pin or graduation change follows these failed early screens.

### YAML graph retirement: capacity falls, peak-memory gate still fails

The [completed-graph screen](compact-yaml-graph-retirement-screen-20261020.md) releases Core graph storage before native publication while preserving all correctness/work gates and the 121,045-node large YAML tree. Core capacity falls **31,840,112→15,454,184 bytes** across publication. Ordinary YAML RSS still rises **29.24%** against the existing route (twenty matched pairs, all initial probes retained), exceeding the 10% relative gate. No timing, runtime promotion, pin, approval or graduation change follows. Storage growth before publication remains the next target.

### Full-width precedence: remove the unused array prefix

The [suffix-storage receipt](compact-wide-precedence-suffix-receipt-20261020.md) confirms **8,388,608→1,398,336 capacity bytes** on the pinned 1 MiB Go witness, preserving full int64 scores, all **414 ledger rows** and **four canonical vectors**. All correctness, budget, RSS and 480 benchmark-row ratchets pass. Timing differences are inconclusive; the fresh-Go B/op median falls 1.35% with borderline p=.051. This is a capacity and memory-budget improvement, with no graduation or speed claim.

### Go election fill: compiler improvement, no measured throughput win

The [direct-fill receipt](compact-go-election-fill-receipt-20261020.md) preserves all **414 ledger rows** and **four canonical Go vectors**. Twenty matched pairs per language pass all 480 timing/allocation rows and RSS limits, but all twelve timing differences are inconclusive. The **1,120→856-byte** election stack change stays held. A separately scoped capacity census identifies an unused **873,784-node prefix** in Go's full-width precedence storage; no performance claim is made for the proposed suffix representation.

### Ordinary YAML ownership: current-layout memory holds

The [ownership and publication receipt](compact-yaml-ordinary-memory-receipt-20261020.md) records two private sources that pass correctness and work gates. Both preserve C output, pass 206 × 72-step invariant sessions and 618 C fixture checks, and reduce ordinary YAML node construction **288→232 / 1,156→925**. They remain held: twenty matched RSS pairs show ordinary YAML median ratios **1.337192 / 1.301437** against the existing fallback route. Every initial and matched probe is retained. No throughput timing, runtime promotion, allowlist change or counter reset is claimed.

### Matched workload order and final PR audit

The [order correction](compact-benchmark-order-receipt-20261020.md) rejects
pairs with different workload orders and shuffles YAML's six mode/fixture pairs
explicitly. The original failed allocation screen remains visible. Go's test
registration had changed all twenty original paired orders; corrected smoke
preserves order across the two different test lists.

The [whole-PR audit](benchmarks/20261020-pr1487-net/README.md) compares
`88dc1d9c` with `bdc45318` using twenty distinct matched orders per language.
All **414 complete rows** stay exact and all **480 rows** pass the ratchets.
Large Go time falls **5.92% (p=.001)** and allocated bytes **34.96%**.
Ordinary YAML allocated bytes fall **11.46% / 13.33%**. Owned YAML objects
fall **33.94% / 43.15%**, trading **1.24% / 2.74%** more allocated bytes.
Other than no-edit time (−6.65%, p=.005), timing differences are inconclusive.
This audit excludes the new wrapper and supplements prior correctness/RSS gates.

### Qualified 32-byte immutable version state

PR #1488 merged to main as `2769aea23c6c8621db5368b6d9c3257bcb3c9b25`
after 74 successful checks and four skips, including editor latency. Every
runtime and benchmark source hash matches the qualified assembly below.

The [wrapper receipt](compact-version-state32-receipt-20261020.md) keeps rare
recovery metadata in a shared immutable holder and preserves full-width IDs,
rollback and closed-group identity. All **414 rows**, four canonical vectors,
**206 × 72-step sessions**, owned YAML, **618 C checks**, memory/race and native
guards pass. The final assembly also passes focused amd64/386 and API/R6 guards.

The corrected **480-row** comparison passes all ratchets with every object
median unchanged, including large Go **260→260**. Query Go allocated bytes fall
**44.58%**, grammargen **4.34%**, owned YAML **9.09% / 9.84%**. All timings are
inconclusive. All **126 RSS processes** pass; including initial samples, Go's
maximum is **319.230 B/source byte**. Ordinary YAML still falls back, the same
39 expanded Go failures remain, and no allowlist or work exception changes.

### Prepared YAML scanner binding: measured R4 throughput win

The [prepared-binding receipt](compact-yaml-prepared-binding-receipt-20261020.md)
shares immutable scanner metadata and proves the complete identity mask at
binding. Twenty matched alternating pairs measure ordinary R4 YAML **2.60%
faster (p=.028)** and legacy R4 **2.07% faster (p=.017)**. The other four YAML
and all six Go timings are inconclusive; every warm YAML byte/object median
stays exact. All **414 ledger rows**, four canonical vectors, YAML correctness
checks, **480 timings** and **126 RSS processes** pass. Supplemental C results
retain 417/723 passes and the same 306 failures. No ordinary YAML ownership,
allowlist or graduation change. An existing Kotlin test-order failure is fixed
by isolating forced-profile language instances; every route/tree assertion stays.

### Closed acceptance-group identity: qualified correctness repair

The [three-file identity repair](compact-acceptance-group-identity-receipt-20261019.md)
preserves retained recovery-election groups during immutable interning and
lexer-owner equality. Both new regressions fail before and pass after,
including race and 386 coverage. All **414 complete ledger rows**, four canonical
trees/work vectors, **206 × 72-step** ordinary sessions, explicit owned YAML,
**618 C checks**, unchanged strict census, **306 Go C outcomes / 300 routes**,
large witnesses, memory/race and API/layout/R6/selector gates pass.

All **480 shuffled rows** finish, all twelve timings are inconclusive and all
object medians stay exact; no time/object median fails its ratchet. Directional
movements include Go grammargen **+6.88%**, owned YAML R4 **+7.68%** and ordinary
YAML 8 KiB **+7.52%**; no speedup is claimed. All **126 RSS processes** pass,
maximum **297.914 B/source byte**. All 3518 Go source hashes match the qualified
candidate. No pin, exception, route or graduation changes.

The corrected malformed-YAML oracle retains **417/723 passing checks** on both
revisions, the same 306 failures and exact public vectors. The initial
clean-helper error and its replay remain available. This repair proves internal
identity preservation; remaining public C mismatches stay open.


### Graph links: fewer allocated bytes, higher RSS

The [29-file split-link candidate](compact-link-hot-cold-allocation-receipt-20261019.md)
keeps identities in **16-byte** records and full-width immutable metadata in
a shared sidecar. On runtime b704, all **414 ledger rows**, four canonical
trees/work vectors, **206 × 72-step** sessions, owned YAML, **618 C checks**,
unchanged strict census, **306 Go C outcomes / 300 routes**, large witnesses,
memory/race/386 and API/layout/R6/selector gates pass.

All **480 shuffled rows** finish. One-MiB Go allocated bytes fall **6.89%**
(**539585416→502397808**), ordinary YAML R4 **10.19%** (**14840→13328**), and
ordinary YAML 8 KiB **12.47%** (**49097→42976**), all p<.001. Every timing
difference is inconclusive. Ordinary YAML 8 KiB time moves **+7.53% (p=.060)**
and query Go **+5.32% (p=.820)**. No throughput improvement is established.

The costs remain explicit: ordinary YAML adds **one object/op**, and large-Go
object medians move **328→344 (+4.88%, p=.124)**. All **126 RSS processes**
pass, maximum **308.558 B/source byte**, but median RSS rises **6.02% Go /
1.50% ordinary YAML / 4.47% owned YAML**. No time/object or RSS median fails
its ratchet. No-edit always allocates nothing. The prototype remains unpromoted
while current-main qualification is prepared; these measurements precede
main 88dc1d9c. No route, work pin, exception or graduation changes.

The earlier [retained-link census](benchmarks/20261019-link-metadata-census/README.md)
remains geometry evidence; its 45.31% live-link estimate is not an RSS claim.
The complete new replay preserves the compiler setup failures and all samples.


### Version-state publication census: common node baselines stay inline

The [six-case publication census](benchmarks/20261019-version-state-publication-census/README.md)
counts newly created immutable wrappers on runtime b704. Their size is **64
bytes**. Go query / grammargen create **113 / 1569** wrappers carrying a node
baseline, and none carrying region/group metadata. Owned YAML R4 / 8 KiB /
one-MiB create **37 / 140 / 19380** wrappers; **1 / 4 / 524** carry node
baselines, and none carry region/group metadata. Large Go creates none at
this publication site. All six compact parses complete with exact input pins,
full root coverage and no errors.

The independent twelve-file prototype keeps the full-width node baseline in
a **32-byte** wrapper and moves only rare region/group fields into an optional
immutable holder. Moving all recovery data out would add allocations on clean
Go. Focused storage, immutability, 386 and race checks pass before main
integration; current-main qualification is running. These are constructor counts and layout
targets, **not measured allocation, RSS or throughput wins**. No production
source, counter pin, route, exception or graduation setting changes.


### Borrowed dispatch cells: no measured throughput win

The [six-file private screen](compact-borrowed-dispatch-cell-screen-20261020.md)
preserves **414 complete rows**, four canonical vectors, **206 × 72-step**
sessions, **618 C fixtures**, unchanged strict/expanded C behavior and all safety
guards. All **480 matched rows** complete, but all twelve timings are inconclusive
and all six Go medians rise **1.00–3.13%**. The runtime stays held. All **126 RSS
probes** pass, maximum **297.293 B/source byte**. The independent public fix only
pins the existing scheduler-size test to measured amd64 and 386 layouts; no
runtime record, budget, counter, route or graduation change.

### Native 16-byte actions: safe, throughput win unproven

The [two-file direct-layout screen](compact-action16-native-layout-screen-20261019.md)
preserves complete fields in a native sixteen-byte Action. All **414 ledger
rows**, four canonical trees/work vectors, **206 × 72-step** sessions, owned
YAML, **618 C checks**, unchanged strict census, **306 Go C outcomes / 300
routes**, large witnesses, Core/386/race, memory and API/layout/R6/selector
gates pass. All **480 shuffled rows** finish; all twelve timing differences
are inconclusive and every object median stays exact. Go full moves **−2.02%
(p=.414)**, one-MiB **−3.75% (p=.265)** and query **+0.66% (p=.799)**. No
throughput win is established. All **126 RSS processes** pass, maximum
**297.617 B/source byte**. The source remains held; the evidence preserves all
samples and the prerequisite-monitor timeout before timing began. No runtime,
route, pin, exception or graduation changes.


### Stable YAML request window: rejected by the allocation ratchet

The [six-file request-window screen](compact-yaml-stable-request-window-screen-20261019.md)
preserves all **414 ledger rows**, four canonical trees/work vectors, **206 ×
72-step** ordinary sessions, explicit owned YAML, **618 C checks**, the strict
census, all **306 Go C outcomes / 300 routes**, large witnesses, memory/race/386
and API/layout/R6/selector gates. All **126 RSS processes** pass, maximum
**318.949 B/source byte**. Recovery pruning, rollback and namespace exhaustion
also pass dedicated checks; the original race-selector typo and corrected
replay remain available.

All **480 shuffled rows** finish. Owned-YAML allocations stay **72 / 191**,
with no timing improvement established. One-MiB Go allocation medians move
**278→325 (+16.91%)**, failing the 10% ratchet; the high-variance distribution
is inconclusive (**p=.930**), which does not waive that gate. Large-Go byte
medians rise **2.16% (p=.109)**. YAML legacy R4 time rises **7.66% (p=.007)**;
all other timings are inconclusive. **No runtime source is promoted.**

Filtering the earlier b513 allocation profile puts **54.30% of sampled objects**
under equivalent-state installation through request publication and shifts.
No sampled allocation stack includes the removed election-compaction loop.
These overlapping sampled shares are attribution, not a speed prediction.
The next YAML target is those actual publication callers. The independent Go
screen tests native 16-byte actions with direct field reads; it is not yet
qualified and does not close E-A7. No pin, exception, route or graduation changes.


### Packed decoder follow-up: throughput unproven; direct consumers next

The [six-file decoder follow-up](compact-packed-action-decoder-screen-20261019.md)
preserves eight-byte canonical actions and lossless exceptional values, but all
**twelve timing differences are inconclusive** across **480 shuffled rows**.
Go full time moves **+1.77%**, query **+2.86%**, grammargen **+3.59%**, and
one-MiB **+3.54%**; owned YAML moves **−2.79% / −0.60%**. No time/object median
exceeds its ratchet. All directions remain in the receipt. **No runtime source
is promoted**. The next Go screen keeps direct field reads in native sixteen-
byte Action values; encoded consumers remain an unqualified follow-up.

All **414 ledger rows**, four canonical trees/work vectors, **206 × 72-step**
ordinary sessions, explicit owned YAML, **618 C checks**, unchanged strict census,
all **306 Go C outcomes / 300 routes**, large Go/YAML witnesses, memory/race/386
and API/layout/R6/selector gates pass. All **126 RSS processes** pass, maximum
**295.695 B/source byte**; ratios **0.994932 Go / 0.967289 ordinary YAML /
0.968543 owned YAML**. No pin, exception, route or graduation changes.

The [fresh owned-YAML attribution](benchmarks/20261019-yaml-owned-attribution/README.md)
uses runtime b513 on the 8012-byte witness with exact routes and work. Header
publication accounts for **63.35% of sampled allocation objects**; snapshot
validation has **8.61% cumulative CPU**, and acceptance materialization **13.52%**.
Sampled allocation profiles include setup/calibration, and cumulative shares
overlap. These are targets, not speedup predictions. A separate private stable-
request-window proposal removes ordinary-election renumbering while retaining
rollback, stale-reference checks and recovery compaction; it is not certified by
this profile or by the decoder's gates. Its completed rejection is recorded above.


### Packed action rows: smaller tables, Go throughput rejected

The [six-file packed-action screen](compact-packed-action-rows-screen-20261019.md)
preserves all fields in eight-byte canonical / sixteen-byte exceptional entries,
with unchanged row-header widths. All **414 complete ledger rows**, four canonical
trees/work vectors, **206 × 72-step** ordinary sessions, explicit owned YAML,
**618 C checks**, unchanged strict census, all **306 Go C statuses / 300 routes**,
large Go/YAML witnesses, races, memory and API/layout/R6/selector guards pass.
All **126 RSS processes** pass, maximum **296.914 B/source byte**.

All **480 shuffled timing rows** complete. Go full DFA time rises **5.29%
(p<.001)**, query_compile **4.17% (p=.028)** and grammargen_lr **5.61%
(p=.008)**. Other timings are inconclusive; allocation-count medians stay exact.
A separate retained-table probe reports **95424→77056 bytes**. The storage
saving does not justify these throughput regressions: **the runtime stays
private**. The follow-up targets decoding and full-Action stack copies.
No production pin, route, exception or graduation changes.


### Compact graph nodes: qualified memory improvement

The [24-byte graph-node receipt](compact-node24-lineage-precedence-receipt-20261019.md)
preserves full state/path/precedence widths, exact lineage sets and all **414
complete ledger rows**. The one-MiB Go comparison reduces allocated bytes
**579888608→528111072 (−8.93%, p<.001)**. Ordinary-YAML bytes fall **4.92% /
5.42%**. All **480 benchmark rows** finish; every timing difference is
inconclusive, and no time/object median exceeds its ratchet.

All **206 × 72-step** ordinary sessions, explicit owned YAML, **618 C checks**,
unchanged strict **120 / 39 / 45** census, all **306 Go C statuses / 300 routes**,
large Go/YAML C witnesses, memory/pooling contracts, races, 386 and native
guards pass. All **126 RSS processes** pass: median reductions are **7.13% Go /
2.94% ordinary YAML / 6.11% owned YAML**, maximum **297.879 B/source byte**.
The pending Go correctness repair remains separate. Routing, work pins,
exceptions and graduation stay unchanged; no throughput improvement is claimed.

### Paired immutable snapshots: qualified allocation reduction

The [paired-snapshot receipt](compact-scanner-snapshot-pairs-receipt-20261019.md)
reduces owned-YAML R4 allocations **79→72 (−8.86%)** and 8 KiB allocations
**218→191 (−12.39%)**. R4 bytes stay exact; 8 KiB bytes rise **1.28%**.
All **440 timing rows** finish with no conclusive throughput difference and
no time/object median above the ratchet. Ordinary routing stays unchanged.

All **414 complete ledger rows**, six public full/edit rows and canonical Go
work pins stay exact. The **206 × 72-step** sessions, explicit owned YAML,
**618 C checks**, unchanged strict **120 / 39 / 45** census, memory contracts,
races, native/386 regressions and API/layout/selector guards pass. All **126
RSS processes** pass, maximum **329.136 B/source byte**; owned-YAML median RSS
rises **3.93%**, within the ratchet. The pending Go correctness repair remains
separate; no production pin, owner exception or graduation change follows.

### Immutable snapshot pages: allocation reduction, byte trade rejected

The [five-file allocation screen](compact-scanner-snapshot-pages-screen-20261019.md)
keeps six complete public full/edit work rows exact and passes focused ownership,
memory, C/edit and Go/YAML invariant gates. Fresh pages never overwrite published
records; the allocator's native/386/race tests pass.

All **440 shuffled-pair rows** finish. Owned 8 KiB allocations fall **218→181**,
but R4 falls only **79→76** while bytes rise **13032→18392 (+41.13%)**.
Every timing comparison is inconclusive. The five-file source remains private:
unused page capacity must be reduced before this can be a runtime improvement.
It includes neither ordinary ownership activation nor the pending Go repair.

### Ordinary YAML streamed ownership: smaller output, timing rejected

The [latest private screen](compact-yaml-streamed-prefix-screen-20261019.md)
completes ordinary YAML on compact through both 524-document C witnesses.
Fifty work/reuse comparisons pass; public constructions fall **288→232** on
R4 and **1156→925** on 8 KiB, with whole-request peaks unchanged at **2→2**.
Both 72-step YAML sessions and metadata/memory checks pass.

All **440 timing rows** finish, but ordinary time rises **16.66% / 42.63%** and
allocation counts rise **37→43 / 34→83**. Go full time rises **10.81%**, above
the median ratchet despite p=.134. Owned timings are inconclusive. The thirteen-
file source remains private; no pin, routing or graduation change follows.
The allocation profile instead identifies immutable scanner snapshots as the
next target (**75.32% of allocated bytes**); this share is not a speed estimate.

### Go correctness repair: requalified on current storage

The [current-layout receipt](compact-go-correctness-rebase-receipt-20261020.md)
passes **306/306 identical C cases**, repairing all 39 baseline failures. All
300 expanded cases route compact; 24 six-step edits remain fresh=C. Current
24-byte nodes / 32-byte version state also pass **206 × 72-step sessions**,
**618 C fixtures**, unchanged strict outcomes, API/R6, root race and **126 RSS
probes** (maximum **307.125 B/source byte**, Go median **+2.27%**). All 414
numeric rows and four canonical trees stay exact. The same **65 work increases
and 12 decreases** still require the pending R3 decision; the original guard
rejects them. No timing comparison or runtime promotion. Historical receipts
below retain their own source and measurement scope.

### Go correctness repair: memory gates pass; R3 owner review ready

The [counter-exception review](compact-go-correctness-counter-review-20261019.md)
contains the complete 17-file private proposal and every one of its 77 changed
canonical fields: **65 increases / 12 decreases**. Fresh published-runtime
controls reproduce **39/300** expanded C failures; the proposal passes all
**306 clean cases** and **144 generic edit steps**. All **206 × 72-step**
ordinary sessions, explicitly activated owned YAML, **618 C checks**, strict
204-row equality, memory contracts, true publication stop controls and races
pass. All 12 API tag sets and layout/R6/selector guards pass after moving the
adapter interface into `internal/`.

All **126 RSS processes** pass, including initial samples; candidate Go maximum
**324.343 B/source byte**. Median ratios are Go **1.009375**, ordinary YAML
**1.001307**, owned YAML **0.977019**. This qualifies the combined source's
memory safety, not throughput. No official timing comparison crosses its
unapproved work direction. No production pin, routing or graduation change
occurs; the runtime remains private pending R3 owner approval and later
performance qualification. Earlier failed memory/API/cache attempts remain
visible in their original scopes.


### Qualified progress-based capacity: large-Go allocation win

Runtime `54863cf280722c720bda81c2a8480f2398255730` adds a late capacity estimate
from actual record counts while retaining the early reserve and memory limits.
The [receipt](compact-progress-arena-projection-receipt-20261019.md) records
**827,364,832→592,422,760 B/op (−28.40%, p<.001)** on the one-MiB Go witness.
Timing **1.181→1.138 s, p=.277** is inconclusive. Allocation count medians
**305→326.5, p=.439** rise 7.05%, within the 10% ratchet; every direction and
all 480 benchmark rows remain visible. No throughput improvement is claimed.

All **414 complete work/reuse rows**, four canonical trees/work vectors and
the large-Go complete work stay exact. The **206 × 72-step invariants**,
explicitly activated owned-YAML 72-step session, **618 C checks**, unchanged
**120 / 39 / 45** strict census, races, memory contracts, native/386 checks
and layout/API/selector guards pass. All **126 RSS probes** pass; candidate
Go peaks at **331.515 B/source byte**, with matched median ratio **0.925638**.
Ordinary and owned YAML ratios are **0.997552 / 1.000859**. No new work exception,
routing change or graduation follows.

A separate earlier combined C-correct protocol/projection screen passes 42 Go
RSS processes (maximum **334.046**, ratio **0.937044**) but retains 65 unapproved
canonical increases. Its source has no full fleet/three-case RSS/timing
qualification. The independent production result does not certify that source.

**Owned-invariant evidence correction:** Prior extra jobs that set only
`GTS_LAYOUT_OWNED_PROBE` did not activate ownership in the invariant process.
Treat those jobs as ordinary-mode coverage. The current receipt's separate
wrapper explicitly activates ownership and proves compact=1/fallback=0 before
running all 72 steps. Explicit owned C/RSS cases retain their scope.


### Go correctness protocol: broad C screen passes; RSS still rejects promotion

The [combined correctness/memory screen](compact-go-correctness-memory-screen-20261019.md)
repairs all **39 / 300** expanded Go C failures and passes all **306 clean C
cases through compact**, 144 generic edit steps, **206 × 72-step invariants**,
**618 smoke-C checks**, the unchanged **120 / 39 / 45** strict census, races,
memory contracts and large-file C checks. Four canonical deep digests and
all 414 numeric fixture-ledger rows are unchanged. One C decline location
changes. **65 canonical work fields increase; 12 decrease.** Every old/new
value is retained; no owner exception is approved or requested here.

The flat origin-receipt store fails two Go RSS probes at **400.43 / 401.72
B/source B**. Paged storage removes **13,651,080 allocated bytes** on the
same one-MiB Go witness with complete work vectors unchanged. Its narrowed
42-probe Go screen passes (max **384.25**, median ratio **0.97693**), but
its complete 126-probe run still fails one Go sample at **404.16**. Median
ratios are Go **1.07590**, ordinary YAML **1.00221**, owned YAML **0.99459**.
The hard ceiling overrides the passing median ratchet. All failed probes
remain included. No timing claim transfers to the final paged candidate.

Both private sources remain unpromoted. That screen used the read-history
runtime `5c463e56`; the newer independent projection above reduces the large-Go
memory peak. Its qualification does not transfer to either private source.
Ordinary YAML ownership and compact graduation remain open.

### Qualified no-replay demand: canonical Go work without unused history

The [read-history demand receipt](compact-read-history-demand-receipt-20261019.md)
skips lexer histories only when publication explicitly disables replay.
Unknown-context and replay-enabled callers retain collection. Twenty shuffled
pairs support **−6.49% / −6.20%** parse time on canonical Go `rewrite` /
`query_compile`. Large canonical Go bytes fall **1,155,584 → 107,520
(−90.70%)**. These benchmarks disable replay; public Go controls and all YAML
timing changes are inconclusive. No-edit stays at zero bytes and allocations.

All **414 work/reuse rows**, four complete canonical vectors/deep digests,
**206 × 72-step invariants**, **618 C checks**, unchanged **120 / 39 / 45**
strict census, races, memory contracts, native/default/tagged/386 tests and
layout/API/selector checks pass. All **126 RSS probes** pass; median ratios
are Go **1.02408**, ordinary YAML **1.00023** and owned YAML **0.99864**, with
maximum candidate **373.06 B/source B**. These are safety measurements.

This bucket changes one runtime file and two existing tests. It includes none
of the private version-ordering protocol below. No counter exception is needed.
Routing, allowlists, ordinary YAML fallback and graduation remain unchanged.

### Go cohort frontier: private C screen repaired; work direction unresolved

The [cohort/frontier and restoration screen](compact-cohort-frontier-ordering-screen-20261019.md)
now passes **306/306 clean Go C checks and 144 edit steps** privately. An
ordinary shift cohort had omitted an already-shifted sibling from its live
frontier. A causal three-history test retains two on production and all
three with the repair. Combined with C-style version ordering and the
owner-bound DFA proof, it fixes the last 12 qualified-type failures below.

Replacing a per-dispatch restoration closure preserves all four complete
private canonical vectors and six selected ledger rows. Twenty shuffled
pairs measure Go R4 **−8.37%**, canonical `language` **−5.43%**, and
`grammargen_lr` **−6.07%** parse time against the same private protocol.
YAML allocations also fall; all three YAML timing changes are inconclusive.
This is a private closure/value comparison, not production throughput evidence.

Preserving the lost histories raises production work counters, including
`grammargen_lr` parents **75,769 → 76,345**, reduction pauses **1,963 → 2,117**
and no-action drops **8,172 → 8,443**. Every direction and all 440 timing rows
are archived. No R3 exception is approved; no full fleet, 414-row ledger or
RSS qualification ran for that screen. Its production reference was `b17ca367`
with all 3,512 Go files unchanged at the time. Ordinary YAML ownership and
graduation are unchanged.

### Earlier Go version-turn screen: 27 private repairs; 12 failures

A new 300-case locked-C screen finds **39 failures in the unchanged runtime**:
27 through fallback and 12 through compact. A private C-style pairwise version
permutation plus authenticated publication union fixes those 27 fallback
failures and adds none in this screen. It still fails the same 12 qualified-type
cases, so it is **not promoted**. All 300 prototype cases use compact; the
unchanged runtime routes 90 and falls back on 210. These are correctness and
admission observations, not throughput or work-ratchet qualification.

The six original generic-call witnesses now pass through compact with no
fallback in the private ordering probe, with the eight-link cap unchanged.
Seven selected existing Go certification, incremental, reuse and recovery
suites pass. The larger screen rejects a one-best-version ordering shortcut:
252/300 exact cases become **288/300** only with C's pairwise slot permutation.

Direct payload sums rule out the earlier suspected precedence miscount.
The remaining qualified-type witness is C-exact on forced legacy, which keeps
the explicit-type-argument call at precedence 2; compact publishes the index
call. The [screen](compact-go-version-turn-order-screen-20261018.md) retains all
variants, unchanged-runtime controls, exact replay patches, failed C evidence
and the next branch-retention question. No new timing, ledger, fleet or RSS
qualification ran. Production stays `b17ca367`; graduation is unchanged.

### DFA origin proof: private cap removal; selection variant rejected

An owner/source/reset-bound proof for identical DFA terminal IDs removes the
**9 > 8** failure from all three combined-call Go witnesses, keeping the cap
at eight. They still decline at tied acceptance; all six public witnesses
remain C-exact on existing routes. Earlier selection alone does not finish
the five generic witnesses.

Extending receipts to copies of one actual elected read finishes every
runner witness, but **five of six public C checks fail**: compact chooses
`call_expression(index_expression)` where C and legacy choose
`type_conversion_expression(generic_type)`. That variant is rejected.
The existing unproven-history checks and 19 origin/context negatives pass,
but lexical ownership does not authorize the independent tree-selection rule.

A narrowed oracle trace finds equal effective precedence 1 and the same
relative call/conversion symbol order in both grammars. Simple renumbering
cannot fix the choice. The [screen](compact-dfa-terminal-origin-screen-20261018.md)
retains all variants, failed C evidence, exact source pins and the remaining
precedence/version-ranking question. No counter/timing/fleet/RSS qualification
was launched after rejection. Runtime stays `b17ca367`; routing and graduation
remain unchanged.

A separate legacy-only trace resolves the witness at aggregate precedence
**−3 versus 1**, while the compact local fold reports **1 versus 1**. The
comparison scopes differ, so this identifies a concrete precedence-attribution
check rather than proving a propagation bug. Its oracle reference passes.

### Scanner record coallocation: qualified YAML allocation win

Runtime `b17ca367a85d419514554e88f4477a339fd3ecba` freezes eligible snapshot
records, byte fields and trial masks in one typed allocation. Owned YAML
allocations fall **109 → 79 (−27.52%)** on R4 and **336 → 218 (−35.12%)**
on 8 KiB. Bytes rise **1.24% / 1.44%**; twenty shuffled pairs establish no
supported throughput difference. Disjoint field capacities retain complete
memory accounting, and all identity/reset/checkpoint checks remain intact.

All **414 counter rows**, five consumed and four canonical vectors,
**206 × 72-step invariants**, **618 C checks**, unchanged **120 / 39 / 45**
strict census, races, memory contracts and actual-source guards pass. All
126 RSS probes pass the ceiling and median ratchet. Go RSS median ratio
**1.03008**, initial **337.07 → 370.15**, and maximum **389.01 B/source B**
remain recorded. The [receipt](compact-scanner-record-coallocation-receipt-20261018.md)
retains all directions and replay evidence. Ordinary YAML still falls back;
routing, allowlists and graduation are unchanged.

### Go generic-call frontier: ownership proof remains the blocker

Three two-generic-call witnesses reach **9 > 8** live links sharing one graph
predecessor. Their terminal values match, but exact leaf identities form four
groups. The existing selector already activates for relevant comparisons;
it still correctly rejects unproven DFA histories. This isolates an ownership
proof target without changing the cap or weakening selection.

All six public witnesses remain C-exact on their current routes. Observed
and unobserved frontier records match. The
[attribution screen](compact-go-generic-call-frontier-screen-20261018.md)
retains source pins, complete traces and limits; it makes no runtime or
throughput claim.

### Combined shift and lineage publication: qualified Go win

Runtime `6d5b0b07e44a8f4fd1ab66d3246a485fde303909` combines eligible direct
shifts with checked owner and scalar/alternative lineage publication. It
retains owner conflicts, journals, rollback, condensed-head publication and
the guards for callbacks, drop references and creation-sequence overflow.
Twenty shuffled pairs support **1.27% less Go R4 time, p=.003**, and
**0.46% / 0.45%** less time on query/language fixtures. Other compact Go and
all YAML differences remain inconclusive. The **+0.58%, p=.011** default-Go
edit slowdown is retained without attribution.

All **414 counter rows**, five consumed vectors, four canonical vectors,
**206 × 72-step invariants**, **618 C fixture checks**, the unchanged strict
**120 / 39 / 45** census, races, memory contracts and actual-source guards
pass. All 126 standalone RSS probes pass 400 B/source B and the matched
medians pass the 10% ratchet. The initial Go RSS **342.07 → 355.67** remains
recorded. The [receipt](compact-shift-lineage-fusion-receipt-20261017.md)
retains the complete measurements and limits. Ordinary YAML still falls
back; this bucket does not graduate a language.

### Direct shared-request publication: private allocation win

Direct publication removes the producer's temporary before-cursor wrapper.
R4 publications fall **224 → 169** and allocations **1,253 → 1,198**; allocated
bytes fall **3.52%**. The 8 KiB stream removes **220** publications/allocations
and **3.50%** of bytes. Both variants pass **19 C fixtures / 57 unique parse
comparisons** and a **72-step YAML invariant session**. Complete Core,
scheduler and runtime values otherwise match on R4 and 8/32 KiB; both YAML
ledger rows match.

Twenty shuffled pairs establish **no supported throughput difference**, with
intervals up to 27%. All 120 rows and the legacy control remain in the
[screen](compact-shared-request-publication-screen-20261018.md). This compares
two unpromoted producers. R4 still exceeds the qualified diagnostic route's
102 tokens / 60 publications at **107 / 169**; no runtime, counter exception
or graduation is claimed. The same-start drop proof remains strict, and C4
removal, empty-action re-lex and selective-marker activation remain rejected.

### Actual shared lookahead ownership: private correctness pathway

Two private variants complete ordinary YAML compact parsing on R4 and the
8/32 KiB streams with the diagnostic probe and artifact exception disabled.
Each passes **19 fixtures / 57 fresh-edit-fresh locked-C comparisons** and
allocation-free no-edit reparses. Publication before the first reduction also
passes a **72-step YAML invariant session**. This does not establish compact
incremental reuse or graduation.

R4 reductions fall **209 → 183**, canonicalizations **326 → 293**, and
per-request re-lex restores **18 → 0**, but elections rise **102 → 107** and
snapshot publications **60 → 224**; nodes remain **484**. Shared-cursor rejoin
restores remain. Late fork publication and a lighter canonical-origin guard
still decline on all three stream witnesses. These counter/admission failures
block promotion, so no randomized timing or new runtime is claimed. The
[screen](compact-shared-lookahead-origin-screen-20261018.md) retains source
patches, the actual-origin trace, passing and failing controls, and replay
commands. That screen used runtime `6d5b0b07e44a8f4fd1ab66d3246a485fde303909`.

### Scanner byte-field packing: no measured gain, not promoted

Twenty shuffled pairs preserve all **414 counter rows** and five consumed
vectors but establish **no supported throughput or YAML allocation gain**.
Owned YAML stays at **109 / 336 allocations** on R4 / 8 KiB; ordinary YAML
attempt plus fallback stays at **37**. All 440 samples, every timing direction,
focused C/invariant/race/386 checks and full Core results remain in the
[screen](compact-scanner-byte-packing-screen-20261018.md).

An untimed field census explains the miss: these freezes have one populated
14–34-byte field and a separate **113-boolean trial mask**. Byte-only packing
cannot combine those copies. Full-fleet/RSS and actual-source qualification
were not run after rejection. Runtime, routes and graduation remain unchanged.

### Scanner fingerprint entry split: mixed result, not promoted

Isolating complete value binding encoding reduces the nil/pointer entry's
stack reservation **4,304 → 152 bytes**, without omitting identity or callback
checks. Twenty pairs improve owned YAML R4 **1.28%, p=.046**, and ordinary
YAML attempt plus fallback **1.09%, p=.011**, but compact Go R4 slows
**0.98%, p=.001**. The default-full **−0.93%, p=.011** control shift remains
unattributed. This runtime is not promoted.

All **414 counter rows**, five consumed vectors and allocation medians remain
exact. Focused C, invariant, ownership, race/386 and full Core checks pass;
full-fleet/RSS and actual-source qualification were not run after rejection.
The [screen](compact-scanner-fingerprint-entry-screen-20261018.md) retains all
440 samples, compiler evidence, the corrected metric-parser audit and limits.

### Combined reduction publication: inconclusive, not promoted

A private general/corridor variant reaches **69.39% of Go** and **66.55% of
owned-YAML reductions** in activation probes while retaining classification,
owner-conflict, history and rollback checks. All **414 ledger rows**, five
consumed vectors, focused C/invariant/race/386 gates and full Core suites pass.
Twenty shuffled pairs establish **no supported throughput or allocation gain**;
timing intervals reach 13%. Every timing direction and all 440 samples remain
in the [screen](compact-reduction-lineage-fusion-screen-20261018.md).

This runtime is not promoted. No additional seeds, full-fleet/RSS qualification,
actual-source gate, counter exception, route change or graduation is claimed.
The qualified shift-publication runtime above remains active.

### Paired terminal cursors: qualified owned-stream win

Runtime `5e52cf2a2644f3d38785513eebc39f8cf8a506b2` reuses a fully validated
terminal's proof within the current comparison pair and directly consumes a
matching right leaf while charging its logical visit. It adds no retained
storage and preserves distinct-terminal checks, error order and budgets.
Twenty pairs support **1.20% less time, p=.007**, on the owned YAML 8 KiB
stream. Compact Go, owned R4 and ordinary YAML timing differences remain
inconclusive. Default Go full parsing slows **1.02%, p<.001**, retained without
compact-work attribution.

All **414 counter rows**, four canonical vectors, **206 × 72-step invariants**,
**618 locked-C fixture checks**, strict **120 / 39 / 45** route census, races,
memory contracts and exact-source guards pass. All 126 standalone RSS probes
pass 400 B/source B; twenty matched pairs per Go/ordinary/owned case pass the
10% median ratchet. The initial Go RSS **341.85 → 395.31** remains recorded.
The [receipt](compact-paired-terminal-cursors-receipt-20261017.md) retains every
timing direction and gate limit. Ordinary YAML routing and graduation remain
open.

### Boundary-probe result transport and first-slot inlining: rejected

Two independent twenty-pair screens reject caller-owned probe results and an
inline first-slot variant. The first establishes no supported compact gain
and slows Go R4 **1.16%, p=.038**, with wide timing intervals. The second
confirms actual hot-path inlining (**cost 58**) and non-escaping scratch, yet
slows four Go workloads **0.89–1.42%** and both owned YAML workloads
**0.62–0.77%**. Compiler inlining does not establish a throughput win.

Both preserve all **414 counter rows**, allocation counts, exact full-key
checks, generation and journal behavior, with focused C/invariant/race/386
gates passing. The [receipt](compact-boundary-probe-screens-20261017.md)
retains all **880 timed rows**, source patches, compiler evidence, every
control direction and noise limits. No runtime promotion or full-fleet/RSS
qualification is claimed.

### Cached visible-count walk: rejected

Twenty pairs find no supported compact Go or YAML speedup from separating the
cold visible-node walk and reading cached child totals directly. All **414
counter rows** and allocation counts stay exact; focused core/C/invariant,
race and 386 checks pass. Supported default-full/no-edit control shifts
**−0.52% / −4.17%** remain unattributed. The
[screen](compact-visible-count-screen-20261017.md) retains all 440 timing rows
and the private source. No runtime promotion or full-fleet/RSS qualification
is claimed.

### Condensation entry split: smaller frame rejected on throughput

A byte-identical extraction of 325 fold/history lines reduces the amd64 entry
stack reservation **1,032 → 272 bytes**, yet twenty pairs slow four compact
Go workloads **0.79–1.64%** and owned YAML R4 **1.04%**. Large Go, the owned
stream and ordinary YAML differences remain inconclusive. All **414 counter
rows**, allocation counts and byte medians stay exact; focused correctness,
C, invariant, race and 386 gates pass. The
[screen](compact-condense-entry-screen-20261017.md) retains all 440 samples,
compiler/source evidence and control movements. This runtime is rejected;
no full-fleet or RSS qualification is claimed.

### Terminal-only provenance filter: rejected

Twenty pairs reject a cache-free own-pair filter: Go R4 slows **0.75%**,
ordinary YAML **0.89%**, and the owned YAML stream **1.12%**. Other compact
timing changes are inconclusive. All **414 counter rows** and allocation
counts stay exact; focused C, edit-invariant, race and 386 checks pass. The
source adds no retained storage but receives no runtime promotion or full
fleet/RSS qualification.

The [receipt](compact-provenance-lookup-screen-20261017.md#terminal-only-absence-screen)
retains all 440 samples and control directions. Baseline profiling locates
most provenance search in reduction-terminal comparisons, motivating a
separate pair-local shared-terminal validation experiment, qualified above.
The rejected filter is absent from that runtime.

### Provenance lookup hints: rejected, including inline control

Two independent twenty-pair comparisons reject a fixed 256-byte position-hint
table. Initial hints improve the owned YAML stream **0.68%**, but Go R4 and
large parsing slow **0.76% / 0.89%**. A controlled split restores getter
inlining (**cost 79**, versus **128** before and **71** in the baseline), yet
still slows three canonical Go fixtures **0.62–0.84%** and ordinary YAML
**1.26%**. Its owned-YAML changes are inconclusive. Inlining alone does not
resolve the measured regressions.

Both sources preserve all **414 counter rows** and allocation counts, with
focused C/invariant/race/386 gates passing. No runtime is promoted and no
full-fleet or RSS qualification is claimed. The
[receipt](compact-provenance-lookup-screen-20261017.md) retains all 880 timing
rows, every control direction, source patches and compiler evidence. A
separate terminal-only lookup alternative is outside these measured sources.

### Ownership no-op guard: rejected; fresh attribution retained

Twenty pairs reject an authenticated live-owner guard: compact Go R4 slows
**1.22%**, and rewrite/query/language slow **0.34–0.80%**. Large Go and all
three YAML comparisons are inconclusive. Allocation counts and all **414
counter rows** stay exact, and focused correctness, C, invariant, race and
386 checks pass. No runtime source or graduation change is promoted.

Fresh unchanged-baseline profiles put reduction construction near **20%
cumulative** in both Go and the owned YAML stream. YAML's external-provenance
binary search is **3.37% flat**, motivating a bounded live-record lookup hint.
The [receipt](compact-owner-publication-screen-20261017.md) retains all 440
timing rows, default-control shifts, attribution scope and source patch.

### Action-row storage: two measured rejections

Independent twenty-pair comparisons from `a03d0842994a85aa26cf494c8c67404e1e64e999`
reject packed and full inline singleton action rows. Packed storage slows all
five compact Go fixtures **3.12–3.84%** and the three YAML paths **2.62–4.18%**
(all p<.001). Keeping the full action inline still slows compact Go
**0.65–1.73%** and the owned YAML stream **0.75%**; its other YAML changes are
inconclusive. The packed cold-table reduction does not yield a warm-parse win.

Both candidates pass focused correctness, locked-C, edit invariants, race and
386 checks with all **414 ledger rows exact** and allocation counts unchanged.
They receive no full-fleet or RSS qualification and no runtime promotion. The
[receipt](compact-action-storage-screen-20261017.md) retains all 880 timed rows,
source patches, counter hashes, validation scope and default-control shifts.
Routing, allowlists and graduation stay unchanged.

### Scheduler checkpoint binding and reduction aggregation

Runtime `027a10d5a75eb59b45a7a0c3477e9254a7c8553e` binds scanner checkpoint IDs
without discarded receipt copies and moves general reduction-history work into
an internal helper. Authentication, callback-sensitive reads, output order and
rollback retain their behavior. The public election representation stays intact.

Twenty direct combined pairs support **−0.46% Go language / −0.80% Go R4** and
**−1.13% owned YAML R4 / −1.22% owned 8 KiB / −0.59% ordinary attempt plus
fallback**. Other compact Go differences remain inconclusive. Allocation counts
stay exact; default controls, including no-edit **+0.61%** at zero allocations,
are recorded without compact-work attribution. Component gains also repeat in
fresh YAML seeds; their separate percentages are not added to this result.

All **414 counter rows**, four canonical vectors/deep digests, **206 × 72-step
invariants**, **618 C checks**, races and memory contracts pass. The strict
census retains **120 compact C-exact / 39 passing legacy / 45 existing legacy
mismatches**. Actual source/API counts stay **756 / 12 / 454 / 101**, and all
65 selectors resolve against 7,732 discovered functions.

An initial unchanged-Go baseline RSS probe fails at **401.11 bytes/source
byte**. It remains in the evidence. All twenty matched Go pairs and all
initial candidate probes pass the unchanged 400 ceiling; candidate matched
maximum is **376.06**. Its matched median moves **+2.28%**, within the unchanged
10% ratchet; no RSS saving is claimed. The
[receipt](compact-scheduler-overhead-receipt-20261017.md) retains that failure,
the diagnostic/repeated measurements, the rejected 264→200-byte election
representation, every timing direction, and complete qualification scope.
Ordinary YAML ownership, routing, allowlists and graduation remain unchanged.

### Canonical head mask: qualified Go throughput bucket

Runtime `75507f35ec6caece2125157b192e7c54060573e3` folds an eight-byte head-ID
mask into the existing normalized-frontier scan. Distinct bits prove singleton
canonical groups; collisions retain exact grouping. Boundary remapping,
version-lexer authentication, metadata and rollback stay intact.

Twenty shuffled pairs measure **0.89–1.74% less parse time across all five
compact Go fixtures**, with allocation counts unchanged. All **forty** YAML
pairs are retained; their three pooled timing differences are inconclusive.
The first marginal stream result does not replicate and is not claimed as a
YAML win. The default no-edit control's −1.45% movement bypasses this operation
and remains unattributed, with zero allocations.

All **414 counter rows**, four complete canonical work vectors/deep digests,
**206 × 72-step invariants**, **618 C fixture checks**, races, one-MiB C checks,
and memory/source/API gates pass. The strict census keeps **120 compact
C-exact / 39 passing legacy / 45 existing legacy mismatches**, all 204 rows
exact. All standalone parser RSS probes stay below 400 bytes/source byte;
no statistical RSS win is claimed. Source/API counts remain **756 / 12 / 454 /
101**; all 65 CI selectors resolve against 7,728 discovered functions.

The [receipt](compact-canonical-head-mask-receipt-20261017.md) records every
direction, both YAML repetitions, the mixed generic-key precursor, and the
untimed array precursor. Their qualifications are not conflated. Ordinary
YAML routing, allowlists and the global default remain unchanged.


**Do not graduate yet.** The E-A exit gate requires median compact/legacy time
at most 1.0 and worst at most 1.15 across all 206 grammars, including declined
files. This census does not satisfy that gate or replace the per-language
edit, query, memory, and C-parity graduation obligations. The allowlist and
global default remain unchanged.

The existing candidate-versus-legacy matrix reports 119 PASS, 79 FALLBACK,
5 SKIP, and 1 DIVERGE on these real files. The divergence is Djot: locked C
agrees with compact, so requiring legacy equality would reject a correct
result. The five skipped custom-token-source cases are Authzed, C, C++, Java,
and JSON. These skips are not compact certifications.

The 79 declines comprise 45 recovery entries, 28 scheduler-frontier proof
shapes, one extra-shift shape, one accepted-leaf-tiling gap, one repetition
shift class, and three runtime diagnostics (Haskell live-link cap, Ledger
root span, and Robot scheduler ownership). These are real inputs; do not
remove proof checks to make the acceptance count rise.

The earlier byte/shape/field C comparison found 160 passes and 44 mismatches.
Among the 120 actual compact acceptances, Liquid was the only mismatch in
that comparison. That comparison did not check every point, extra flag, or
propagated error flag. `TestCompactR4StructuralLockedC` now checks those axes
unconditionally, reports the actual route, and verifies the sample digest.

The strengthened final-source check passes **159 of 204** samples. All **120
actual compact acceptances** match locked C on every tested structural axis.
Of the 79 fallback cases, 35 match and 44 differ; of the five custom-source
cases, four match and C++ differs. Chatito and EDS add propagated-error
mismatches that the earlier comparator missed. The [initial per-language receipt](compact-r4-readiness-20261010.json)
contains sample hashes, routes, decline reasons, and first differences. No
language was skipped in this strengthened check. This is one fresh real file
per language, not a grammar-wide parity certificate.

### Compact lineage storage: qualified retained-capacity reduction

The direct 16-byte node-provenance record keeps recovery costs and reference
indexes in a lazy sidecar. Retained lineage capacity falls **42.86%** on all
four canonical Go files; large Go falls **4,948,160 → 2,827,520 bytes**.
Twenty pairs establish small rewrite/owned-YAML-stream gains of **0.48% /
0.51%** (p=.040/.003), with unchanged allocation counts. Other Go and ordinary
YAML timing changes remain inconclusive. Ordinary YAML bytes fall
**16,760 → 15,608**, with 37 allocations.

All **414 ledger rows**, **206 invariant sessions**, **618 C fixture checks**,
and **204 strict real-file rows** retain their results. Race, 386, memory and
standalone 1 MiB RSS gates pass. A new callback regression guard prevents a
recovery-cost snapshot from crossing reduction-plan callbacks; baseline passes,
the first temporary sidecar fails, and the fixed source passes. The
[focused receipt](compact-lineage-storage-receipt-20261017.md) records rejected
sharing/plain-sidecar screens, every timing direction, the +4.21% default no-edit
control, exact counter pins, RSS scope, and existing strict-census failures.
No broad Go throughput, ownership, allowlist, or graduation claim follows.

### YAML scanner lookahead: qualified shipped-route throughput win

Against main `eb1312afdd82d98490e157e51391febd275f0432`, the YAML scanner
caches its current rune inside one scan. All four advance helpers and scan
initialization invalidate it. Failed scans, checkpoint restores and later
requests receive a fresh environment. Scanner state and read frontiers retain
their original meaning. The existing 184-byte frontier witness records
**933→409 observations**, with the same exact frontier maxima.

Twenty alternating shuffled pairs use the repository runner, one process per
seed, `GOMAXPROCS=1`, `-count=1`, `750ms`, `-benchmem`, CPU 6, serialized
6 GiB Docker and the pinned 1,998-byte R4 sample / 8,012-byte stream:

| Route | Before → after | p | Bucket |
| --- | --- | --- | --- |
| Legacy | 151.497→143.325 µs | .004 | **−5.39% time** |
| Ordinary compact attempt plus fallback | 198.183→191.215 µs | .007 | **−3.52% time** |
| Diagnostic owned compact | 356.901→352.296 µs | .265 | Inconclusive |
| Diagnostic owned stream | 1.384588→1.365058 ms | .127 | Inconclusive |

All byte and allocation medians remain exact: **1,440/16**, **16,760/37**,
**12,872/109**, and **44,313/336**, respectively. Ordinary and owned YAML
counter exports match their baselines; all **412 ordinary fleet rows** are also
identical on the actual candidate source. Owned full/edit nodes stay **484/484**;
default full/edit nodes stay **288/480**. No ledger exception is needed.
The [raw before](benchmarks/20261017-yaml-lookahead/before.txt),
[raw after](benchmarks/20261017-yaml-lookahead/after.txt),
[benchstat](benchmarks/20261017-yaml-lookahead/benchstat.txt),
[counter receipts](benchmarks/20261017-yaml-lookahead/counters.json) and
[measurement audit](benchmarks/20261017-yaml-lookahead/measurement.json)
preserve all twenty samples, source pins and route counters. Raw completion
footers confirm all seeds completed; no samples are removed.

Before timing, 16 cursor fixtures pass **144 exact C comparisons per revision**
across legacy/candidate/owned fresh, edited-fresh and incremental parsing.
All 22 YAML corpus samples pass structural and highlight checks on all three
routes. Ordinary/owned 72-step invariants, zero-allocation no-edit reparses,
frontier differentials, checkpoint tests, races and 386 checks pass. Four
separate **1,049,572-byte** C-exact safety probes cover original/cached scanners
and ordinary/owned routes. Cached RSS is **121,992/143,644 KiB**, below the
400-bytes-per-source-byte gate. These are single safety probes, not a
statistical RSS improvement claim. Set `GTS_YAML_LARGE_PROBE=1` and select
`TestCompactYAMLDocumentStreamLockedC/large_ordinary_documents` or
`/large_owned_documents` to reproduce them in separate processes.

Inside the same isolated, locked and CPU-pinned environment, reproduce timing
with baseline and candidate mounted at `/baseline` and `/workspace`:

```sh
GTS_ADMISSION_REAL_CORPUS_LANGS=yaml GOMAXPROCS=1 \
bash scripts/run_randomized_benchmarks.sh \
  --baseline-root /baseline --baseline-output /tmp/yaml-before.txt \
  --output /tmp/yaml-after.txt --runs 20 --benchtime 750ms \
  --bench-regex '^Benchmark(AdmissionR4Legacy|AdmissionR4Compact|AdmissionR4OwnedExternalLexer|AdmissionYAMLDocumentStreamOwned)$' \
  --require-benchmarks BenchmarkAdmissionR4Legacy,BenchmarkAdmissionR4Compact,BenchmarkAdmissionR4OwnedExternalLexer,BenchmarkAdmissionYAMLDocumentStreamOwned
```

Ordinary YAML still declines at its ownership proof. No Go throughput,
ordinary compact acceptance, allowlist change or graduation is claimed.
The separate used-adjacency-frame screen preserves all 414 ledger rows and
passes scoped correctness/race/386 checks, but twenty pairs establish no
supported Go or YAML speed gain (all time p≥.081); its runtime is discarded.
Local evidence is `gotreesitter-evidence/20261017-yaml-scanner-lookahead` and
`gotreesitter-evidence/20261017-compact-reduction-workspace`.

The separate Go parent-only receipt screen removes leaf-receipt scanning from
normal reductions. All 414 ledger rows and four raw leaf/keyword receipt digests
remain exact; scoped C/invariant/race/386 checks pass. Its twenty-pair comparison
uses the YAML cache as its predecessor. All four compact Go timings and all three
YAML timings are inconclusive (p≥.076), so the two-line runtime change and its
prototype test stay outside the checkout. No full-fleet invariant, strict-census
or graduation qualification is transferred to that rejected source. Evidence:
`gotreesitter-evidence/20261017-reduction-leaf-receipts`.

### Empty external modes: qualified Go win, YAML timing unchanged

Buckley runtime commit `959121c6311d45f743a24aad0ae55b021cb56a04` proves an
external-correction result constant when both explicit external rows reject
all symbols, retaining existing stateless/frozen-election/ownership/checkpoint
conditions. Internal DFA candidates are discarded by this helper, so this is
a static proof rather than an observed-span shortcut. A separate unsigned
bounds repair closes a reproduced 386 high-state panic. The [qualified
receipt](compact-empty-external-modes-receipt-20261016.md), against `8e4fe353`,
records twenty-pair compact Go time reductions **2.23% / 1.88% / 2.19% / 2.69%**
on rewrite/query/language/large. All are significant; all three YAML changes
are inconclusive. Default edit/no-edit controls regress **0.86% / 0.68%**
(p<.001/.002); these directions remain reported. Objects stay exact and
no-edit stays zero. All 414 rows and canonical trees/Core.Work remain exact;
only raw table lookups fall, **7,553→7,537 / 37,640→37,621 / 37,479→37,436 /
397,227→395,591**. No counter exception is needed. All 206 sessions plus owned
YAML, 618 C fixtures, strict 159/45 with all 120 compact C-exact, races, 386,
RSS and post-copy/API/CI gates pass. The existing deeper-S3 Scala test failure
is reproduced on unchanged baseline and retained. An earlier inlined source
regresses ordinary YAML 2.23% and is rejected; its timing is not transferred.
Ordinary YAML ownership and E-A graduation stay gated. All main merges and
auto-merge remain held for 0.56.0.

### Visible counts: retain the independent test, reject the runtime shortcut

The [cached-child screen](compact-visible-count-screen-20261016.md) on
`d98e0470` preserves all 414 rows, canonical trees/Core.Work and raw21, but
twenty pairs find no significant compact Go or YAML time gain. The runtime
shortcut is rejected. Separate Buckley commit `8e4fe353` adds a 24-graph
independent materialization oracle, passing visible-count tests on the
unchanged runtime under native, race and 386 builds. A diagnostic observes
7,290 failed Go probes with nonzero reads, none extending the prior span on
these witnesses. It motivates a read-bound proof before a probe shortcut;
it does not supply that proof or a new runtime. Ownership and graduation
remain gated, with all main merges held for 0.56.0.

### Condensation split: smaller frame, rejected on throughput

The [twenty-pair screen](compact-condense-split-screen-20261016.md) on
`f7ec6721` reduces the common condensation frame 1,032→272 bytes but improves
no compact Go or YAML path significantly. Rewrite/query regress **0.54% /
0.71%** (p=.021/.014); all 414 rows, canonical trees/Core.Work and raw21 stay
exact. The private runtime is rejected; no full-fleet or RSS qualification is
claimed. Separate test-only Buckley commit `d98e0470` repairs existing 386
layout assertions, retaining exact sizes and budget checks, with full native
and 386 core suites passing. Production runtime and graduation gates stay
unchanged. All main merges remain held for 0.56.0.

### Warm-path census: discard inactive and saturated targets

The [post-warmup census](compact-warm-path-census-20261016.md) on `f7ec6721`
finds zero eligible ASCII SKIP loops, zero successful stateless external probes
and zero checkpoint-digest requests on the four Go and three YAML witnesses.
Larger or associative probe memos remove no trace misses; the existing 16-slot
memo already captures the repetition. Trees, Core.Work and raw21 stay exact.
Separate warm CPU profiles put reduction output construction at 15.58–16.96%
on owned YAML and large Go. These are target-selection receipts, without a
runtime or throughput claim. Ordinary YAML ownership and graduation remain
gated; all main merges stay held for 0.56.0.

### Bounded ASCII runs: qualified Go improvement, YAML unchanged by timing

Buckley runtime commit `f7ec67210486be4398d962fa0cf7e73b02da1c41` adds a bounded
same-state ASCII scan and shared encoding constants, retaining scalar fallback
for SKIP, Unicode, newlines, other states and the reserved no-match value. The
[qualified receipt](compact-ascii-self-loop-receipt-20261016.md) compares with
`8701f7f9`: rewrite **−0.75%**, language **−1.90%**, default Go full **−0.68%**
(p=.021/<.001/.001). Query/large, editing and all three YAML changes are
inconclusive. Every byte/object median and all 414 valid ledger rows stay exact.
The proper owned export confirms full/edit **484→484**; earlier extra exports
used a test-only initializer and duplicated ordinary routing, so those exports
are excluded. The owned work unit already passed before timing. All 206
sessions plus owned YAML, 618 C fixtures, strict 159/45 with all 120 compact
C-exact, races, 386, RSS and post-copy/API/CI gates pass. Read-only Buckley review
catches and closes the private sentinel edge. Ordinary YAML routing and
all graduation gates stay unchanged. All main merges remain held for 0.56.0.

### Direct native action planes: no Go speed improvement

The [native-row screen](compact-native-row-screen-20261016.md) preserves all
414 ledger rows, all 21 raw Go fields, scoped C/edit checks and overridden
adapter behavior. Twenty pairs against `86879374e` find no significant compact
Go time improvement: rewrite/query/language/large +0.10% / −0.01% / +0.49% /
+0.35%. Default full rises 2.66% (p<.001). The private source is rejected;
no YAML timing, full-fleet, strict, RSS or runtime promotion is claimed.

### Immutable YAML retry flags: allocation savings rejected on time

The [four sharing screens](compact-shared-retry-screens-20261016.md) preserve
all 414 ledger rows and scoped C/edit checks against `86879374e`. Owned R4 /
8 KiB allocations fall 109→94 / 336→277 and bytes fall 14.92% / 17.04%, but
ordinary YAML slows 1.23–1.93% in twenty pairs per source. The final source
preserves all original hot-field offsets and raw DFA source, yet still costs
+1.93% (p<.001). A same-source control is inconclusive on all three paths.
All four candidates are rejected; no runtime source, counter exception or
routing change is promoted. Incomplete final reviews are not approval.
Go timing, fleet, strict-census and large-RSS qualification are not claimed.
All main merges remain held for 0.56.0.

### Single-prefix ordinary YAML: acceptance proved on witnesses, cost gate failed

The [single-prefix screen](compact-single-prefix-yaml-screen-20261015.md) on
`c498e889e` transfers the original shared lookahead into exact owned snapshots
before the first unowned fork. All seven focused C comparisons require compact
acceptance, including ordinary R4 and one/four/sixteen-document streams. Of 414
ledger rows, 413 remain exact; only ordinary YAML admission changes. A separate
fallback counter probe records 107→107 tokens and 288→484 public constructions
for both full and first-edit parsing; diagnostic-owned counters stay exact.
The first twenty-pair screen rejects activation: ordinary R4 is **+22.61%** and
the 8 KiB stream **+48.47%** slower than the current attempt and fallback. A
separate twenty-pair scratch-reuse screen cuts bytes **65.61% / 63.91%**, but
still regresses time **16.02% / 41.08%** and raises allocation counts. Both are
rejected. Review then exposes a historical scanner-binding gap in the private
prototype; a before/after regression, seven positive C cases and the 72-step
YAML invariant pass with the fix. Earlier timings do not transfer to that fixed
source. No runtime or counter exception is promoted, and no earlier owner
approval is extended. Ordinary YAML graduation remains blocked by cost.

### Packed graph links: storage projection rejected by throughput

The [graph-link screen](compact-link-storage-screens-20261016.md) starts at
`14f255bac`, after the qualified tagged alternative-set change. A 24-byte link
with full-width rare scalar pairs projects 20.71% less logical link storage on
large Go. Scoped correctness, C checks, edit invariants and deterministic
checks pass. Twenty alternating pairs nevertheless raise time 6.59–8.05% on
all four Go files and 2.20% / 4.12% / 4.69% on ordinary-fallback / owned-R4 /
owned-stream YAML (all p<.001). Allocation counts stay exact; fallback bytes
fall 4.58%. The prototype is rejected and remains outside the checkout. It
supplies no fleet certification, route activation or graduation evidence.

### Qualified stateless probe capacity increase

Core commit `86879374e` grows the qualified negative probe memo from eight to
sixteen slots. The [capacity receipt](compact-relex-capacity-receipt-20261016.md)
records **1.91% less Go language time / 0.80% less large-Go time** in twenty
shuffled pairs against `e4243c77a`. Other Go and all YAML timings are inconclusive;
YAML's stateful scanner bypasses the memo. Allocation counts remain exact,
large-Go bytes move +31 B/op (+0.00095%), and typed memo storage grows 132→260
bytes with full footprint charging. All 414 ledger rows stay exact; raw lookup
work falls or stays unchanged. The 206-session invariant gate plus owned YAML,
618 C checks, unchanged strict census, races and memory gates pass. No ordinary
YAML activation, graduation or main merge is implied. Independent deltas below
are not added.

### Qualified reuse of failed stateless lexer probes

Core commit `e4243c77a` reuses failed external-token probes only within the same
frozen stateless election, retaining checkpoint checks and exact read metadata.
The [qualified receipt](compact-relex-memo-receipt-20261016.md) records
**0.52–2.40% less time** on all four canonical Go files in twenty final-source
pairs. Allocation medians and parse-work counters stay exact; all 414 ledger
rows remain unchanged. Ordinary YAML's −0.90% direction bypasses this memo and
receives no causal attribution. Owned YAML directions are inconclusive, including
stream **+0.15%**. No YAML speed gain from cached probes is claimed.
All 206 invariant sessions plus owned YAML, 618 C fixture checks, the unchanged
204-row strict census, races and memory checks pass. Full state width and S5
restore invalidation reject the earlier private prototypes. Root/API and CI
selector checks pass. Ordinary YAML activation and graduation remain gated;
all main merges remain held.

### Qualified ordinary-link validation fast path

Core commit `0808dfd5f` accepts the existing ordinary-link shape through an
inlinable guard and delegates every other shape to the unchanged validator.
The [completed receipt](compact-link-validation-receipt-20261016.md) records
0.91–1.57% less time on all four canonical Go files and 1.10% / 0.71% less on
owned YAML R4 / 8 KiB stream. Ordinary YAML fallback is inconclusive. All 414
counter rows, 206 invariant sessions and 618 C fixture checks pass; the 204-row
strict census stays exact, including all 120 C-exact compact acceptances.
Allocations stay exact. The default no-edit control's +1.90% direction and
single-probe RSS limits are retained. This is a qualified throughput win;
ordinary YAML activation and compact graduation remain gated.

### Compiled Go action rows: throughput screen rejected

A production-options observer finds that all 115,952 prepared rows across four
Go fixtures match existing compiled rows. A private internal decoder/table
adapter preserves fresh classification, all 414 ledger rows, canonical
digests/Core.Work, raw counters and scoped C/edit/race/386/memory checks.
Nevertheless, twenty shuffled pairs make compact Go **1.40–1.87% slower**
(all p<.001); the prototype is rejected. No YAML timing or full-fleet promotion
gate is claimed. A separate warm YAML census finds **zero snapshot clones**
and one exact retry mask repeated in 15 / 59 owned captures, directing the next
experiment toward immutable sharing of ordinary boolean storage. Those counts
are diagnostic. The [receipt](compact-dispatch-row-screen-20261016.md) records
controls, review limits, source pins and the unchanged merge hold.

### Pointer fingerprint frame split: no measurable gain

The [pointer-frame screen](compact-scanner-frame-screen-20261016.md) reduces
the amd64 wrapper's stack allocation **4,304→152 bytes** with the generic body
unchanged. Scoped C/edit, race/386, memory and all 414 counter checks pass.
Twenty YAML pairs give **+0.18% / −0.02% / +0.01%** ordinary / owned R4 /
stream directions, all inconclusive (p=.092/.478/.904); bytes and objects stay
exact. The compiler result supplies no runtime win. The prototype is not
promoted; no Go timing, full-fleet, strict-census or RSS certificate is claimed.

### Numeric scanner fingerprint encoding: no measurable gain

The [fingerprint encoding screen](compact-scanner-encoding-screen-20261016.md)
preserves the old identity bytes and passes scoped C/edit, race/386, memory and
414-row counter checks. Twenty YAML pairs give **−0.30% / −0.30% / −0.23%**
ordinary / owned R4 / stream directions, all inconclusive (p=.525/.862/.086).
Allocation counts stay exact; stream bytes round +1. No runtime source is
promoted. A separate warm-loop census observes **1 / 675 / 2,522** pointer
fingerprints and zero numeric-encoding calls, redirecting work to the actual
pointer path. The census is diagnostic; no Go timing, full-fleet, strict-census
or RSS certificate is claimed.

### Owned-only retry flags: throughput screen rejected

The [owned-only mask screen](compact-owned-retry-screen-20261016.md) keeps live
DFA flags unchanged and places immutable packed flags only in owned snapshots.
All 414 ledger rows, scoped C/edit checks, races/386 and memory gates pass;
source-only review finds no concrete defect within its stated scope. Twenty
YAML pairs nevertheless increase ordinary / owned R4 / stream time **2.66% /
2.37% / 1.30%** (p<.001/.001/<.001). Owned bytes fall **11.19–12.78%** and
objects **13.76–17.56%**, including the 400→424-byte record cost. The candidate
is rejected. No Go timing, full-fleet, strict-census or RSS certification is
claimed; no runtime source, route or graduation change is promoted.

### Packed retry flags: memory savings rejected for ordinary throughput

The [packed retry-mask screen](compact-packed-retry-screen-20261016.md) starts
at `86879374e`. Twenty YAML pairs cut owned R4/stream bytes **17.90% / 17.91%**
and objects **16.51% / 18.45%**, but ordinary attempt/fallback takes **1.33% more
time** (p=.001). Owned R4 is inconclusive; stream time falls 0.69% (p=.033).
All 414 ledger rows, 176 scoped parser tests, races/386, focused YAML C checks,
Go/YAML edit sessions and budget gates pass on the initial source. The general
runtime change is rejected. Source review also finds a private oversized-backing
clearing gap; a before/fixed leaf regression and normal/race/386 checks pass.
That repair is untimed and has no parser certification. No Go timing, full-fleet
or large-RSS gate is claimed for either version. No runtime source or route is
promoted; the ordinary YAML cost blocker remains.

### Snapshot and retry-mask coallocation: discarded

The [owned snapshot screen](compact-snapshot-allocation-screen-20261016.md)
starts at `0808dfd5f`. A typed record/mask allocation preserves ownership,
validation, all 414 ledger rows and canonical Go tree/work pins. Twenty YAML
pairs reduce owned R4/stream objects 109→94 / 336→277 but raise bytes
12,872→13,352 / 44,313→46,201. Timing stays inconclusive: ordinary fallback
−0.62% (p=.052), owned R4 +0.17% (p=.820), stream −0.23% (p=.369).
The source stays outside the checkout and receives no Go timing, full-fleet
promotion or graduation qualification.

### Diagnostic scheduler test maintenance

Core commit `7b016214d6322540ffd52009ae5eb214ce8434ea` in
[PR #1485](https://github.com/odvcencio/gotreesitter/pull/1485) refreshes five
stale diagnostic scheduler expectations against unchanged production `c498e889e`.
It retains exact work, graph/state/round/scanner assertions, three repetitions
and the accepted deep digest. The cap helper now uses the public diagnostic's
existing grammar certificate; its before-accept dispatch cap is 2,673, one
before acceptance at 2,674. Every capped case still asserts no published result.

Both timing roles shared these identical test-only repairs. All 414 measured
runtime ledger rows remain unchanged; no engine counters or ratchets are reset.
Broadened diagnostic/context tests, the diagnostic race suite and four canonical
Go locked-C comparisons pass in Docker. Source and promotion audits pass in
`20261015-generic-scheduler-receipts`. Only two existing test files are committed;
production source remains unchanged and no throughput win is claimed. The
exact PR head's CI finishes with 74 successful and four skipped checks, no
failures, including editor latency. All merges remain held until the owner
lifts the 0.56.0 release hold.

### Ordinary YAML route gate: measured throughput gap

On baseline `70b27618117e60eecd6e1aaa26552aa81b37f22a`, twenty shuffled seeds
compare the three existing R4 entrypoints on the same pinned 1,998-byte YAML
file. Each seed runs in its own process with `GOMAXPROCS=1`, `-count=1`, 750 ms
and `-benchmem`, in serialized 6 GiB Docker on CPU 6.

| Route | Median time | B/op | Allocations/op | Compact / fallback per operation |
| --- | ---: | ---: | ---: | ---: |
| Legacy | 148.966 µs | 1,440 | 16 | 0 / 0 |
| Shipped compact attempt and fallback | 199.713 µs | 17,144 | 37 | 0 / 1 |
| Diagnostic owned compact | 387.804 µs | 12,936 | 111 | 1 / 0 |

Owned compact takes **2.603× legacy time (+160.33%, p<.001)**. The shipped
attempt-and-fallback entrypoint takes **1.341× (+34.07%, p<.001)**. Activating
owned parsing alone would therefore worsen ordinary throughput on this witness.
It needs about **61.6% less time** to equal legacy here. This is a single pinned
file, not the all-language E-A corpus or a C speed comparison. No runtime,
ownership, allowlist or graduation change follows. Evidence is local
`gotreesitter-evidence/20261015-yaml-owned-route-gate`: raw samples, verified
routes, source pins and separate benchstat comparisons. The comparison files
normalize benchmark names only; measurements are unchanged. All twenty seeds
complete and the container exits zero.

### Go lexer census and owned-YAML attribution

A raw GLR lexer census on the same baseline checks identical lex-state/cursor
requests within each union election. Observed and unobserved parses retain
identical core work, public structure and reuse eligibility on four canonical
Go files:

| Go witness | Union rounds | Raw scans | Repeated scans across parser states | Result/cursor mismatches |
| --- | ---: | ---: | ---: | ---: |
| rewrite | 171 | 359 | 17 | 0 |
| query_compile | 927 | 1,901 | 45 | 0 |
| language | 664 | 1,418 | 90 | 0 |
| grammargen_lr | 8,901 | 18,842 | 1,012 | 0 |

Only **5.37%** of large-Go raw union scans repeat. This limits the coverage of
a raw-result cache; it does not estimate total parse speed. The existing
preferred-result cache remains keyed by parser state because promotion and
sentinel handling depend on that state. No new cache is introduced. The census
passes with source pins and terminal exit zero; evidence is local
`gotreesitter-evidence/20261015-glr-raw-lex-census`.

A separate owned-YAML CPU profile attributes 84.25% cumulative CPU to scheduler
execution, 25.41% to owned reduction application, 19.41% to election and 14.33%
to materialization. Snapshot equality accounts for 6.10% cumulative CPU;
the by-value DFA snapshot comparison accounts for 2.54%. These shares overlap
and are not additive speed estimates. The owned profile contains 9.84 CPU
seconds of samples. Its separate legacy profile has a different total, so
percentage differences do not establish an absolute saving. Both profiling
processes exit zero on pinned source; evidence is local
`gotreesitter-evidence/20261015-owned-yaml-profile`.

### Borrowed lexer equality: qualified owned-YAML win

The next screen uses the same `70b27618117e60eecd6e1aaa26552aa81b37f22a`
baseline. DFA equality borrows two snapshots instead of passing two 232-byte
values. Identical immutable owned snapshots and metadata holders can return
equal immediately; distinct snapshots retain every prior value comparison.
Scheduler ownership still requires the original owner pointers. No scanner,
checkpoint, rollback, proof or table check is removed.

Twenty alternating shuffled pairs use the repository runner, one language per
process, `GOMAXPROCS=1`, 750 ms, `-count=1`, `-benchmem`, CPU 6 and serialized
6 GiB Docker. All samples and source pins verify; the container exits zero
without OOM or timeout.

| Workload | Before → after | p |
| --- | ---: | ---: |
| Owned YAML R4 | 395.132 → 373.650 µs (**−5.44%**) | .002 |
| Owned YAML 8 KiB | 1.505 → 1.433 ms (**−4.81%**) | .001 |
| Public compact Go, 20.2 kB | 10.098 → 10.020 ms | .495 |
| Public compact Go, 235.6 kB | 107.739 → 107.530 ms | .529 |
| Go R4 compact | 849.363 → 850.246 µs | .883 |
| Go full DFA control | 3.952 → 3.882 ms (**−1.79%**) | .001 |
| Go single-byte edit control | 99.658 → 99.702 µs | .883 |
| Go no-edit control | 3.1435 → 3.111 ns (**−1.03%**) | .007 |

All byte/allocation medians stay identical: YAML **12,936 B / 111 allocations**
and **44,601 B / 345 allocations**; public Go **141,139 B / 205 allocations**
and **3,295,728 B / 3,092 allocations**; Go R4 **1,696 B / 25 allocations**;
the default trio **1,159 B / 6**, **394 B / 5**, and **0 B / 0**. All public
node counts and route/fallback measurements remain exact. The compact Go and
edit timing directions are inconclusive. The no-edit result is a control
measurement, not evidence of work saved by snapshot comparison.

Before timing, all **412 ordinary work/reuse rows** and the diagnostic-owned
YAML rows remain exact. Final engine source passes **206 invariant sessions**,
**618 locked-C fixture checks**, both full core/tagged phase0A runs, focused
snapshot/ownership/lifecycle tests and core/root/YAML races. All 204 real-file
census rows remain exactly identical: **159 PASS / 45 known FAIL**, including
**120 actual compact/C-exact acceptances**. All six budget trials pass at
**1,072,860 / 13,060 bytes**. The **1,049,572-byte / 524-document YAML stream**
remains C-exact with complete coverage, no error/fallback and **156,508 KiB**
peak process RSS. This Go+C process probe is not a controlled before/after RSS
comparison or a graduation memory certificate.

L0 retains **756 root files / 12 API tag sets** and R6 retains **454 language-name
uses / 101 environment reads**. The regression assertions move into the existing
clone/ownership test, preserving the root file inventory; CI now explicitly
includes that test. Selector validation and its unit tests pass. Engine hashes
remain identical to the measured screen. Buckley commit `dbdf56887b37f769dad88f4563b01e6f5fd92dde`
promotes this win in [core PR #1485](https://github.com/odvcencio/gotreesitter/pull/1485).
All qualification and final-guard containers exit zero without OOM or timeout.
Evidence is local
`gotreesitter-evidence/20261015-borrowed-lexer-equality`, including raw samples,
benchstat, metric medians, both source-pin sets and the excluded initial failed
compile. The earlier reduction win is an independent comparison and is not
added to these percentages. Ordinary YAML activation and graduation remain
gated.

A fresh profile on this final source contains 5.56 CPU seconds of samples on
the 235.6 kB Go witness. Dispatch accounts for 39.75% cumulative CPU, owned
reduction for 24.64%, materialization for 20.68% and election for 18.71%; these
shares overlap. Copies account for 5.04% flat CPU. Copy callers include
canonicalization, external relex, snapshot capture, election and subtree-view
publication. This keeps dispatch/copy and publication work ahead of a raw
lexer cache as the next Go investigation. It is attribution, not an additional
throughput result, and its allocation output is not mixed into the twenty-pair
comparison.

### Direct snapshot capture: discarded

On baseline `dbdf56887b37f769dad88f4563b01e6f5fd92dde`, a separate six-file
trial changes the existing scratch-snapshot method to fill a caller-provided
destination. It preserves scanner serialization, scratch clearing and snapshot
restoration, and adapts all ordinary, owned and EOF-quiescence callers. An
expanded zero-payload regression checks stale destination slices as well as
the existing zero-allocation and independent before/after scratch contracts.
Focused lexer/rollback/ownership/EOF tests, Go/YAML edit invariants and locked-C
checks pass. All 412 ordinary work/reuse rows and the owned-YAML rows remain
identical before timing.

Twenty alternating shuffled pairs use the same runner and pinned settings as
the qualified equality comparison:

| Workload | Before → after | p |
| --- | ---: | ---: |
| Public compact Go, 20.2 kB | 10.193 → 10.076 ms | .211 |
| Public compact Go, 235.6 kB | 107.741 → 107.562 ms | .529 |
| Go R4 compact | 852.587 → 845.779 µs | .698 |
| Go full DFA control | 3.890 → 3.942 ms (**+1.34%**) | .009 |
| Go single-byte edit control | 99.693 → 99.487 µs | .995 |
| Go no-edit control | 3.1315 → 3.1275 ns | .794 |
| Owned YAML R4 | 362.671 → 360.558 µs | .149 |
| Owned YAML 8 KiB | 1.410 → 1.401 ms (**−0.65%**) | .014 |

Every compact Go direction is inconclusive. The small YAML gain does not
justify the full-DFA control regression, so the runtime trial is discarded.
All byte/allocation medians remain identical, including no-edit zero. The
8 KiB byte sample distributions differ (p=.008), while both medians remain
44,601 B/op. Public-node and route/fallback counts stay exact. All samples and
source pins verify, and the pipeline exits zero without OOM or timeout. Full
fleet/race/RSS qualification is withheld after this timing screen. The qualified
equality win and PR #1485 remain unchanged. Evidence is local
`gotreesitter-evidence/20261015-direct-snapshot-capture`, including the external
patch, raw samples, benchstat, exact medians and disposition.

### Retained subtree duplication: limited interning coverage

A read-only, post-parse census on the same `dbdf56887` baseline compares retained
compact subtree records, their field/alias vectors and either exact child IDs
or recursively canonicalized child-record identities:

| Go witness | Retained records | Nonterminal records | Exact-child duplicates | Recursive duplicates |
| --- | ---: | ---: | ---: | ---: |
| rewrite | 2,613 | 1,504 | 12 | 28 |
| query_compile | 12,961 | 7,415 | 21 | 64 |
| language | 12,998 | 7,623 | 80 | 231 |
| grammargen_lr | 129,665 | 75,769 | 728 | 1,857 |

For large Go, exact-child duplicates are **0.96% of nonterminal records**;
recursive duplicates are **2.45%**, or **1.43% of all retained records**. Every
reported duplicate nonterminal has a nonempty span. These are retained-record
counts, not a census of records removed by rollback or a CPU saving estimate.
The identity deliberately omits sparse provenance, ownership and dependency
receipts, so it cannot authorize interning or merging. Observation preserves
complete core/scheduler work, public structure and reuse eligibility on all four
witnesses. The container exits zero on verified source pins. This ranks a new
record-interning table below the measured dispatch/copy and publication targets;
no cache or merge rule is added. Evidence is local
`gotreesitter-evidence/20261015-subtree-duplication-census`, with source, raw
logs and parsed census JSON.

### Borrowed header accessors: qualified Go/YAML win

A separate one-file trial on `dbdf56887` borrows headers for seven read-only
version-state/recovery accessors. Their bodies and the 88-byte header layout
remain unchanged. A scan of all 756 explicitly named root Go files finds no
bound getter method values; the four same-name numeric field references are
reviewed separately. All 25 caller files' build constraints are covered by the
checked default and `gts_parsercorephase0` builds.

All 412 ordinary rows and owned-YAML rows remain identical. Focused root,
canonical/ownership/lifecycle, Go/YAML edit-invariant and locked-C checks pass.
Assembly confirms that the canonicalizer's static calls into `runtime.duffcopy`
decrease **11→4**; the seven removed sites copied complete headers for field
reads, including an inlined getter. The remaining four sites belong to the
generic ownership-remap adapter. This function's explicit stack reservation
decreases **408→400 bytes**. These are static compiler results, not executed
copy counts or a throughput gain.

Twenty alternating shuffled pairs measure the complete public entrypoint:

| Workload | Before → after | p |
| --- | ---: | ---: |
| Public compact Go, 20.2 kB | 10.051 → 9.897 ms (**−1.53%**) | <.001 |
| Public compact Go, 235.6 kB | 107.875 → 105.530 ms (**−2.17%**) | <.001 |
| Go R4 compact | 846.295 → 837.083 µs (**−1.09%**) | .002 |
| Owned YAML R4 | 364.713 → 354.156 µs (**−2.89%**) | <.001 |
| Owned YAML 8 KiB | 1.410 → 1.386 ms (**−1.72%**) | .001 |
| Go full DFA control | 3.901 → 3.906 ms | .301 |
| Go single-byte edit control | 99.430 → 99.738 µs | .081 |
| Go no-edit control | 3.116 → 3.1215 ns | .223 |

All default-control directions are inconclusive. All byte/allocation medians
remain identical: public Go **141,139 B / 205 allocations** and
**3,295,728 B / 3,092 allocations**; owned YAML **12,936 / 111** and
**44,601 / 345**; Go R4 **1,696 / 25**; the default trio **1,159 / 6**,
**394 / 5** and **0 / 0**. Public-node and route/fallback counts remain exact.
The runner uses twenty explicit seeds, one language/process per seed/role,
750 ms, `-count=1`, `-benchmem`, `GOMAXPROCS=1`, CPU 6 and serialized 6 GiB Docker.

Final source passes **206 invariant sessions**, **618 locked-C fixture checks**,
both full core/tagged phase0A suites, core/root/YAML races and six budget trials.
All **204 strict census rows remain exactly identical: 159 PASS / 45 known FAIL**,
including **120 actual compact/C-exact acceptances**. Four additional public
compact Go canonical files match locked C, including both timed large files.
The **1,049,572-byte / 524-document YAML stream** is C-exact, has complete
coverage and no error/fallback, and peaks at **156,876 KiB process RSS**. The
previous separately pinned probe reported 156,508 KiB (+0.24%); these single
Go+C process probes are not a controlled RSS comparison or a graduation memory
certificate. Budget peaks remain **1,072,860 / 13,060 bytes**.

L0 retains **756 files / 12 API tag sets**, R6 **454 language-name uses /
101 environment reads**; CI selector checks pass. All source pins and terminal
Docker exits verify without OOM or timeout. A byte comparison proves the runtime
change consists only of seven receiver signatures. Buckley commit
`fb4c8acc4de8cbd14da37c3dff13fa6b2bc4b054` promotes this win in
[core PR #1485](https://github.com/odvcencio/gotreesitter/pull/1485).

A separate final-source Go profile has 5.80 CPU seconds of samples and attributes
**2.93% flat CPU** to `duffcopy`, versus 5.04% in the earlier profile. This is
attribution, not an additional timing gain or completion of #1313's wider
checkpoint/recovery scope. Earlier snapshot/reduction percentages remain
separate and are not added to this comparison. Ordinary YAML activation and
the remaining graduation obligations stay gated. Evidence is local
`gotreesitter-evidence/20261015-borrowed-header-accessors`, including the exact
source pins, reference/build-tag audit, counter JSON and before/after assembly
with checked `duffcopy` address ranges.

### Current public routes: small Go and owned YAML remain behind

A fresh route comparison pins the qualified `fb4c8acc4` runtime. Twenty seeds
compare each route in its own shuffled process with the repository runner,
750 ms, `-count=1`, `-benchmem`, `GOMAXPROCS=1`, CPU 6 and serialized 6 GiB Docker.
Go route order alternates evenly; every route occupies each position ten times.
The three YAML routes occupy each position six or seven times. The initial
YAML order audit found an 8/6/6 imbalance before its timing medians were
inspected; those samples are excluded and YAML alone is rerun with balanced
order. Normalized comparison files change benchmark names only.

| Witness | Legacy time | Compact time | Compact / legacy | p |
| --- | ---: | ---: | ---: | ---: |
| Go R4 | 668.821 µs | 860.107 µs | **1.286× (+28.60%)** | <.001 |
| Go query_compile, 20.2 kB | 10.947 ms | 9.812 ms | **0.896× (−10.37%)** | <.001 |
| Go grammargen_lr, 235.6 kB | 128.251 ms | 104.678 ms | **0.816× (−18.38%)** | <.001 |
| YAML R4, ordinary attempt and fallback | 151.747 µs | 202.416 µs | **1.334× (+33.39%)** | <.001 |
| YAML R4, diagnostic owned compact | 151.747 µs | 366.641 µs | **2.416× (+141.61%)** | <.001 |

Go compact accepts every timed parse without fallback; all eight locked-C
comparisons on four canonical Go files and both routes pass. The ordinary YAML
attempt still falls back every time, while the owned route accepts every time.
The qualified runtime's earlier YAML correctness evidence remains separate
from this throughput measurement. Small Go and ordinary YAML remain graduation
blockers; the larger Go wins do not establish the all-language E-A gate.

Allocation medians show another gap. Go legacy/compact bytes and allocations
are **2,712 / 11 versus 1,696 / 25** for R4, **3,552 / 18 versus 141,136 / 205**
for 20.2 kB, and **16,632 / 146 versus 3,295,728 / 3,092** for 235.6 kB. YAML
legacy, ordinary attempt and owned compact retain **1,440 / 16**, **17,144 / 37**
and **12,936 / 111**. These are current-route comparisons, not accumulated
improvement percentages or C speed measurements. All processes exit zero
without OOM or timeout and all source pins verify. Evidence is local
`gotreesitter-evidence/20261015-public-route-gap`, with the accepted YAML samples
and order audit in `yaml-balanced` and the initial exclusion recorded separately.

### Steady Go allocations: dependency storage and immutable publication

Separate allocation profiles on `fb4c8acc4` use allocation sampling rate one
and 32 warm Parse/Release iterations per Go file. Fixture setup is outside a
unique profiled caller; collections run only after the loop to flush delayed
records without clearing warm pools before measurement. These diagnostic runs
allocate **4,516,680 bytes / 6,565 objects** for 20.2 kB and **105,463,904 bytes /
98,953 objects** for 235.6 kB. Their per-parse totals agree closely with the
unprofiled allocation medians above; profile timing is not throughput evidence.

For large Go, lexer `Reads.Record` allocates **2,096,128 bytes per parse** and
compact leaf-receipt storage allocates **1,048,064 bytes per parse**. Together
they account for about **95.4% of steady allocated bytes**. Immutable version
state accounts for **1,569 objects per parse**, principally through recovery
baseline publication; link-record clones account for another **1,376 objects**.
These are about **50.7% and 44.5% of steady objects** respectively. Percentages
use the isolated steady-loop totals, not pprof's whole-process denominator.
The lexer-read buffer is distinct from incremental PR #1480's arena dependency
index; that PR's full-parse allocation medians remain unchanged.

Separate small-Go CPU profiles put dispatch at **46.35% cumulative CPU**,
reduction at **17.49%**, election at **15.45%** and materialization at **15.28%**
on the compact route. Legacy-read publication accounts for **4.41% cumulative**
and copies for **3.57% flat**. The compact profile contains 5.89 CPU seconds;
legacy contains 6.65. Cumulative shares overlap and differing totals do not
establish speedups. No storage, ownership or proof rule changes follow from
this attribution alone. Both profiling processes and the allocation runs exit
zero on pinned source. Evidence is local
`gotreesitter-evidence/20261015-go-allocation-attribution`.

A separate publication census finds **zero exact repetitions among 1,905
immutable state publications** across four canonical Go files. All 1,569
publications on large Go carry only a known recovery baseline, as do all
publications on the other three files. None belongs to an accepted header.
This rules out a no-op publication guard on these witnesses and identifies
baseline storage as the next allocation target. Ownership comparisons include
that baseline's value and known/unknown state, so changing its storage must
preserve both. No runtime change follows from this census. All four parses
accept with complete coverage and no error/fallback; source pins and terminal
exit zero verify. Evidence is local
`gotreesitter-evidence/20261015-version-publication-census` and its `shape` run.

### Baseline-state batching: discarded

On core baseline `fb4c8acc4`, a separate trial allocates baseline-only immutable
version states in bounded pages. Every publication still receives a distinct
pointer, reset drops pages without overwriting old values, and complete page
capacity is charged before allocation. Focused lifetime, ownership, C and edit
checks pass; all 412 ordinary and owned-YAML ledger rows remain exact.

Twenty alternating shuffled pairs reduce large-Go objects **3,092→1,553**,
but raise bytes **3,295,728→3,316,464**. The smaller Go file changes
**205→99 objects / 141,139→143,187 bytes**; Go R4 changes **25→23 objects /
1,696→1,824 bytes**. Compact Go and YAML throughput directions are
inconclusive. The full-DFA control regresses **4.90% (p<.001)** and no-edit
**2.38% (p<.001)**; full-DFA bytes rise **1,159→1,160**. These controls bypass
the allocator, so no causal attribution follows. The trade is declined, rather
than reported as a hard 10% ratchet failure. No runtime source is promoted and
no full fleet/race/RSS qualification follows. Evidence is local
`gotreesitter-evidence/20261015-batched-baseline-states`.

### Single-link replacement: qualified Go throughput and allocation win

The next trial on the same `fb4c8acc4` baseline replaces one temporary heap
clone with a private one-element stack buffer. Larger adjacencies retain the
existing clone. All graph, capacity, precedence and provenance checks remain
unchanged; the surviving link is copied into the graph arena before the buffer
leaves scope. Restoring the single clone statement reproduces the entire
baseline `core.go` byte-for-byte. Compiler escape analysis, assembly and the
allocation measurements agree that the common buffer stays on the stack.

Twenty fresh alternating shuffled pairs use the repository runner and the
same pinned settings as above:

| Workload | Before → after | p |
| --- | ---: | ---: |
| Compact Go query_compile | 10.079 → 9.967 ms | .165 |
| Compact Go grammargen_lr | **107.274 → 105.821 ms (−1.35%)** | .040 |
| Go R4 compact | 841.982 → 839.197 µs | .925 |
| Owned YAML R4 | 361.700 → 364.085 µs | .165 |
| Owned YAML 8,012-byte stream | 1.404118 → 1.399334 ms | .925 |
| Go full DFA control | 3.955280 → 3.922137 ms | .242 |
| Go single-byte edit control | 100.240 → 100.514 µs | .820 |
| Go no-edit control | **3.140 → 3.109 ns (−0.99%)** | .021 |

Only the large-Go compact time has a supported improvement. Other compact and
edit/full-control directions are inconclusive. The no-edit control bypasses
replacement construction; its direction is recorded without attribution.

| Compact workload | B/op before → after | Allocations before → after |
| --- | ---: | ---: |
| Go query_compile | **141,139 → 138,739** | **205 → 130** |
| Go grammargen_lr | **3,295,728 → 3,251,728** | **3,092 → 1,717 (−44.47%)** |
| Go R4 | **1,696 → 1,600** | **25 → 22** |
| Owned YAML R4 | **12,936 → 12,872** | **111 → 109** |
| Owned YAML stream | **44,601 → 44,313** | **345 → 336** |

Default-control byte/allocation medians remain **1,159 / 6**, **394 / 5** and
**0 / 0**. Timed node-work and route/fallback counters remain exact; all 412
ordinary and owned-YAML ledger rows match before timing. No counter exception
or baseline reset is requested.

Final source passes **206 invariant sessions**, **618 locked-C fixture checks**,
full core/correct-tag phase0A suites, **all core race tests**, root/YAML races
and six bounded-stop budget trials. All **204 strict census rows are exactly
unchanged: 159 PASS / 45 known FAIL**, including **120 actual compact/C-exact
acceptances**. Four additional canonical Go/C comparisons pass. Budget peaks
stay **1,072,860 / 13,060 bytes**. The **1,049,572-byte / 524-document YAML
stream** remains C-exact with full coverage, no error/fallback and **156,836 KiB
process RSS**. The previous separate header probe measured 156,876 KiB; these
single Go+C process probes are not a controlled RSS comparison or a graduation
memory certificate.

L0 remains **756 root files / 12 API tag sets**; R6 **454 language-name uses /
101 environment reads**. Package-execution coverage, workflow selectors and
source pins pass. Core/phase0A suites rerun on the promoted source so the
source-inspection ratchets read the actual candidate file. The initial compiled
test-binary invocation used the wrong working directory for source inspection;
it is excluded and corrected without runtime changes. All final containers
exit zero without OOM or timeout. Buckley commit
`bb5022df4c950fa9881351cbd3896354ca714d61` promotes the win in
[core PR #1485](https://github.com/odvcencio/gotreesitter/pull/1485).
Earlier percentages are independent and are not added. Evidence is local
`gotreesitter-evidence/20261015-single-link-replacement`.

### Paired checkpoint authentication: rejected after callback review

A shared-destination validation prototype on `bb5022df4` measured owned-YAML
R4 **354.527→341.824 µs (−3.58%)** and the 8,012-byte stream
**1.373438→1.336439 ms (−2.69%)**, both p<.001 over twenty paired shuffled seeds.
Every allocation median and all **414 work/reuse rows** stayed exact. Compact
Go timings were inconclusive; full/edit/no-edit controls rose
**3.02% / 0.97% / 1.73%**, recorded without causal attribution to code they
bypass. All raw samples, production hashes and scope limits remain retained.

The prototype also passed 206 invariant sessions, 618 C fixture checks,
204 unchanged strict rows, races, six budgets and a 1 MiB YAML/C probe. Those
checks did not cover a callback binding change. A Buckley read-only source
review found that `ExternalScannerIsStateless()` can replace
`Language.ExternalScanner` while the first snapshot's contract is read.
Sharing that destination result could then accept the second snapshot against
the old binding. A new Docker regression reproduces the exact acceptance.

**The shortcut is rejected and is not promoted.** The request validator is
restored byte-for-byte to `bb5022df4`, preserving both live destination checks.
The new callback regression is retained with corruption and reset guards in
the explicit CI selector. The separately measured copy-only screen below also
fails to establish a throughput win. The apparent speed result cannot qualify
the unsafe batch or graduate YAML.

The dedicated Buckley review command timed out during broad host verification
without a verdict; its remaining session processes were stopped and its
incomplete artifact is excluded from qualification. The separate tool-limited
source pass produced the actionable finding above, without executing tests or
issuing merge approval. Evidence is local
`gotreesitter-evidence/20261015-shared-checkpoint-validation`, including the
frozen unsafe patch, review, failing witness and explicit disposition.

### Copy-only checkpoint validation: no supported throughput gain

A separate prototype retains both live destination checks and avoids only a
second validation of an identical private checkpoint copy, after checking its
ID, storage, length and receipt metadata. It passes focused core/phase-zero-A,
Go/YAML C checks and edit invariants; all **414 work/reuse rows**, timed node
and route counters, bytes and allocation medians stay exact. The callback
regression passes on both the unchanged baseline and this candidate.

Twenty alternating shuffled pairs on `bb5022df4` establish no affected-path
gain: owned YAML R4 **352.147→351.005 µs (−0.32%, p=.061)**; the 8,012-byte
stream **1.376199→1.366275 ms (−0.72%, p=.096)**; compact Go query/large/R4
**−0.34% / −0.52% / +0.61%**, all inconclusive. The full-DFA and no-edit
controls rise **3.40% / 2.74%** (both p<.001); the edit control is inconclusive
at **−0.36%**. The controls bypass this snapshot operation, so these directions
are recorded without causal attribution. No-edit stays **0 bytes / 0 allocations**.

**The runtime prototype and its helper are discarded.** Production returns
byte-for-byte to the qualified baseline. The adversarial callback, seventeen
second-snapshot corruption cases and between-call binding/reset guards remain
as tests with explicit CI coverage in Buckley commit `83463617e`. Native
Docker tests, L0 and all 64 workflow selectors pass. There is no transferred full-fleet, race,
RSS or graduation qualification for this screen. All twenty samples per role,
source hashes, complete container exit zero and no-OOM/no-timeout receipts are
verified. Evidence is local
`gotreesitter-evidence/20261015-checkpoint-copy-alias-validation`; the retained
test/CI patch is in `20261015-checkpoint-validation-guards`.

### Qualified borrowed election headers

Buckley commit `b7225ca0a` borrows each 88-byte scheduler header while election
reads its LR state and flags. The full diagnostic path keeps its existing
value snapshot. It preserves all state, checkpoint and callback ordering;
there is no new field or retained pointer. The measured production baseline
is `bb5022df4`; intervening `83463617e` adds only tests and CI.

All **414 work/reuse rows**, timed public-node counts, routing/fallbacks and
allocation medians remain exact. Twenty alternating shuffled pairs, using
the repository runner at one process/seed/role, `GOMAXPROCS=1`, 750 ms,
`-benchmem`, CPU 6 and serialized 6 GiB Docker, measure:

| Workload | Before → after | Direction | p |
| --- | ---: | ---: | ---: |
| Compact Go query_compile, 20,168 bytes | 9.801 → 9.631 ms | -1.74% | <.001 |
| Compact Go grammargen_lr, 235,626 bytes | 103.418 → 102.220 ms | -1.16% | .026 |
| Go R4 compact | 815.684 → 815.750 µs | +0.01% | .512 |
| Owned YAML R4 | 352.661 → 349.137 µs | -1.00% | .003 |
| Owned YAML 8,012-byte stream | 1.377 → 1.373 ms | -0.28% | .495 |
| Go full DFA control | 3.871 → 3.889 ms | +0.47% | .056 |
| Go single-byte edit control | 99.004 → 99.265 µs | +0.26% | .268 |
| Go no-edit control | 3.051 → 3.101 ns | +1.62% | <.001 |

The supported affected-path wins are Go query **−1.74%**, large Go **−1.16%**
and owned YAML R4 **−1.00%**. The other affected times and full/edit controls
are inconclusive. The no-edit control bypasses election; its **+1.62%**
direction is recorded without causal attribution, retaining **0 bytes /
0 allocations**. All other byte/allocation medians are unchanged: Go query
**138,739 / 130**, large Go **3,251,728 / 1,717**, Go R4 **1,600 / 22**, owned
YAML R4 **12,872 / 109** and YAML stream **44,313 / 336**. Earlier bucket gains
are independent and are not added.

Final source passes **206 invariant sessions**, **618 locked-C fixture
checks**, **204 exactly unchanged strict rows** (**159 PASS / 45 known FAIL**,
including **120 actual compact/C-exact acceptances**), full core/phase-zero-A,
core/root/YAML races and six bounded-stop trials. Peaks stay
**1,072,860 / 13,060 bytes**. The **1,049,572-byte / 524-document diagnostic-owned
YAML stream** remains C-exact with full coverage, no error/fallback and
**156,988 KiB Go+C process RSS**. This is a single safety probe, not a controlled
RSS comparison or a graduation memory certificate.

Promoted-source core/phase-zero-A checks and four additional canonical Go/C
comparisons pass, including both timed files. L0 remains **756 root files /
12 API tag sets**; R6 **454 language-name uses / 101 environment reads**;
package coverage and all **64 workflow selectors** pass. A supplemental 386
library build passes; full 32-bit root tests are not claimed. All timing and
qualification hashes, rows and completion markers verify. A disk-full event
was resolved by removing old rebuildable test executables; sources, raw
samples and all complete qualification receipts remain intact.

This is partial progress on E-A2/E-A7. Ordinary YAML ownership, throughput
gaps and graduation remain open; no route or allowlist changes. Evidence is
local `gotreesitter-evidence/20261015-borrowed-election-headers`. No merge to
main is performed.

### Qualified callback binding guard: correctness at a measured cost

Buckley commit `0cb7017e6` closes an existing snapshot-acceptance hole. A
capability callback could replace `Language.ExternalScanner` after its old
contract was read, allowing validation, publication or restore to succeed
against the replacement. Capture the entry interface and reject a changed
descriptor after all capability callbacks. The callback-free helper lives
under `internal/`; every existing contract, identity, generation, checkpoint
and independent request-snapshot check remains.

Twelve cases cover stateless, checkpoint-identity and non-comparable value
scanner replacement across destination validation, complete validation,
publication and restore. All twelve reproduce baseline acceptance and pass
with the fix; failed validation preserves the destination lexer/header.
Scalar descriptors can be shared under the existing type/value identity
rules, and equal reboxed values may conservatively reject. The helper makes
an additional rejection check and supplies no immutable-capability guarantee.

All **414 work/reuse rows** and timed route/public-node counters remain exact.
Twenty alternating shuffled pairs on `b7225ca0a`, with the same runner,
750 ms, process/seed/role, `GOMAXPROCS=1`, CPU 6 and serialized 6 GiB Docker,
measure this correctness trade:

| Workload | Before → after | Direction | p |
| --- | ---: | ---: | ---: |
| Compact Go query_compile | 9.636 → 9.627 ms | -0.10% | .369 |
| Compact Go grammargen_lr | 101.953 → 101.813 ms | -0.14% | .512 |
| Go R4 compact | 815.939 → 813.306 µs | -0.32% | .192 |
| Owned YAML R4 | 348.183 → 351.394 µs | +0.92% | .009 |
| Owned YAML 8,012-byte stream | 1.369 → 1.377 ms | +0.60% | .063 |
| Go full DFA control | 4.000 → 3.994 ms | -0.15% | .289 |
| Go single-byte edit control | 99.502 → 99.403 µs | -0.10% | .779 |
| Go no-edit control | 3.143 → 3.202 ns | +1.86% | <.001 |

Owned YAML R4 pays **0.92%** for the new check; the stream and affected Go
timings are inconclusive. The no-edit control bypasses this validation; its
**+1.86%** direction is recorded without causal attribution, retaining
**0 B/op / 0 allocations**. No throughput gain is claimed for this guard.

Every allocation-count median stays identical. Byte medians are unchanged
except query **138,738.5→138,739 (+0.00036%)**, still **130 allocations**.
The additional exact-byte screen stopped on that half-byte median; the
actual v1 10% performance ratchet passes. Every direction is retained, no
timing is resampled and no deterministic-ledger exception is requested.

Final combined source passes **206 invariant sessions**, **618 C fixture
checks**, full core/phase-zero-A, core/root/YAML races and six budget trials.
All **204 strict rows are exactly unchanged: 159 PASS / 45 known FAIL**,
including **120 actual compact/C-exact acceptances**. Peaks remain
**1,072,860 / 13,060 bytes**. The **1,049,572-byte / 524-document diagnostic-owned
YAML** probe stays C-exact, fully covered, without error/fallback, at
**152,104 KiB Go+C process RSS**. This is a single safety probe, not a controlled
RSS comparison or graduation certificate.

Promoted-source tests, four further canonical Go/C comparisons, L0
(**756 root files / 12 API tag sets**), R6 (**454 language-name uses /
101 environment reads**), package coverage and all **64 workflow selectors**
pass. The 386 helper tests and library build pass; full root 32-bit tests are
not claimed. Buckley's completed source-only review establishes no additional
finding and gives no merge approval. The scalar finding in its earlier pass
led to explicit descriptor semantics and added value/scalar cases. A blocked
outside-workspace pass and an empty exit receipt during disk exhaustion are
not treated as approval; the final readable-source review has terminal exit
zero and a completed report.

Two fixture compilation attempts and one build-cache cleanup startup failure
are excluded. Their corrected inputs pass; engine/timing source hashes remain
verified. After timing completed, old rebuildable project cache entries were
trimmed to restore disk space; source and raw samples remain retained.

The preceding election win retains its independent comparison and is not
combined numerically with this cost. Ordinary YAML ownership, throughput gaps
and graduation remain open, with routing and allowlists unchanged. No merge
to main is performed. Evidence is local
`gotreesitter-evidence/20261015-callback-binding-guard`.

### Link-reference reduction scratch: no parser throughput gain

A prototype on `0cb7017e6` replaces copied adjacency records with immutable
`LinkID` frames and parallel reverse payload/score/order columns with a
separate reverse-reference vector. It retains stable insertion/fold order,
preflight/shape/cardinality checks, marker semantics and overflow behavior.
Child selection keeps its separately typed `SubtreeID` scratch. The added
reverse vector is accounted and cleared on reset/trim. The existing nested
adjacency-frame accounting gap is unchanged; no complete-memory claim is made.

Full core/phase-zero-A tests, 24 independent physical-route comparisons,
focused Go/YAML C checks and edit invariants pass. All **414 work/reuse rows**
and timed route/public-node counts remain exact. Buckley's source-only review
establishes no concrete finding under the immutable-link premise.

Twenty alternating shuffled pairs establish no compact throughput gain:
Go query **+0.23% (p=.398)**, large Go **+0.17% (p=.301)**, Go R4
**−0.16% (p=.947)**, owned YAML R4 **−0.05% (p=.820)** and the 8,012-byte stream
**−0.10% (p=.529)**. Every allocation-count median stays unchanged. Query bytes
move **138,739→138,738**; every other byte median is identical. Full-DFA is
inconclusive at **−0.23% (p=.883)**; edit/no-edit controls improve
**0.98% / 0.97%** (p<.001/.002), bypassing compact pop scratch and recorded
without causal attribution. No-edit remains allocation-free.

**The runtime prototype is discarded.** There is no transferred full-fleet,
race, RSS or graduation qualification. All source pins, twenty samples per
role, complete container exits and no-OOM/no-timeout receipts verify. One
initial compile attempt exposed shared use of subtree scratch; the corrected
candidate separates ID families and passes before timing. Evidence is local
`gotreesitter-evidence/20261015-pop-link-reference-scratch`.

### Reduce-then-shift fusion: default activation rejected

Baseline is `0cb7017e6b3809b238283b740d178ea98045c7e7`. The first default-on
screen fails the deterministic gate before timing: previously accepted CMake,
HTML and Python samples fail to complete. The executable compiler uses immediate
predecessors even for zero-child and multi-child reductions. A synthetic witness
reproduces all three unsafe arities on the baseline. Restricting the proof to
unary reductions restores the three complete ledger rows exactly.

The corrected experiment also records and checks the actual post-reduction
landing before using the fused shift row. It retains the intermediate stop poll,
token gate and contextual-close-angle check. An independent baseline regression
finds 31 / 104 / 98 / 822 missing polls on the four canonical Go files; corrected
fusion matches all unfused poll and semantic-work counts. Forced landing-mismatch
fallback and invalid-landing validation tests pass. The incremental receipt now
compares each route to its own fresh parse, replacing a legacy-materialization
assertion that also fails on untouched baseline with fusion enabled.

All **414 work/reuse rows** remain exact after correction. Scoped C4, five-language
edit invariants and locked-C comparisons, owned-YAML checks, and regenerated
206-grammar static receipts pass. Buckley's completed source-only review finds
no established defect, while explicitly leaving full compiler and runtime
qualification to the gates. No full-fleet invariant, race, budget or RSS receipt
is transferred to this experiment.

Twenty alternating shuffled pairs use the standard Docker/CPU-6 settings,
`GOMAXPROCS=1`, `750ms`, `-count=1` and `-benchmem`:

| Workload | Before → after | Direction | p |
| --- | ---: | ---: | ---: |
| Go 20,168 bytes | 9,699.272 → 9,676.291 µs | -0.24% | .369 |
| Go 235,626 bytes | 103,374.777 → 102,698.303 µs | -0.65% | .253 |
| Go R4 | 837.054 → 839.086 µs | +0.24% | .738 |
| Owned YAML R4 | 376.302 → 378.103 µs | +0.48% | .904 |
| Owned YAML 8 KiB | 1,466.824 → 1,453.550 µs | -0.90% | .445 |

Every affected timing direction is inconclusive. Allocation-count medians stay
identical. Query bytes rise **138,738→138,739**; other compact bytes stay exact.
The bypassed full/edit/no-edit controls move **+2.01%** (p=.698), **−0.42%**
(p=.056), and **−2.91%** (p=.005); full-control bytes rise **1,159→1,160** and
no-edit stays allocation-free. These controls do not establish a fusion win.

Retained bytecode rises **155,201,856→155,539,164 bytes (+0.22%)** across the
fleet; Go rises **0.58%**, YAML **4.63%**, and OCaml stays exact. This comparison
uses the regenerated, untouched `0cb7017e6` baseline on the same current blobs.
The first calculation used a stale committed fleet receipt for 18 grammars and
overstated the fusion cost; its **+1.67% / +71.12% OCaml** figures are withdrawn.
The untouched baseline reproduces that stale-receipt failure, and its regenerated
206 rows exactly equal the repaired default-off candidate. The receipt refresh
changes no default compiled table. This is static program storage, not per-parse
allocation or measured RSS. With no supported throughput gain or allocation
savings, default activation is rejected. The separately qualified opt-in repair is recorded below. Routing and graduation do not change. Frozen source,
failed witnesses, all forty samples per workload and the completed audit remain
in local `gotreesitter-evidence/20261015-c4-reduce-shift-promotion`.

### Qualified opt-in reduce-shift correctness

Buckley commit `c498e889ec2d86382a3a0baad58020d7df7145b3` in #1485 keeps
reduce-shift fusion opt-in and repairs its compiler proof and VM boundary. It
requires a unary reduction, records the expected landing in a fourth word,
validates its range and checks the actual post-reduction state before shifting.
A mismatch resumes ordinary dispatch. Intermediate stop control, token gating
and contextual deferral remain intact. Four explicit CI regressions cover arity,
invalid landing, forced fallback and exact polls. Incremental receipts compare
both routes to fresh parsing. The completed source-only Buckley review is
qualification evidence with no merge approval.

Against `0cb7017e6`, default and promoted opt-in ledgers each retain **all 414
rows exactly**. Each mode independently passes **206 invariant sessions**,
**618 C fixture checks**, and **204 exactly unchanged strict rows: 159 PASS /
45 known FAIL**, including **120 actual compact/C-exact** samples. Full core,
phase-zero-A, core/root/YAML and explicit opt-in C4 races pass. Six memory-budget
trials per mode preserve **1,072,860 / 13,060 bytes**. The C-exact 1,049,572-byte
owned-YAML safety probes record **164,212 / 157,092 KiB Go+C process RSS** for
default/opt-in mode; these are single probes, not controlled RSS comparisons.
No root 32-bit test-suite pass is claimed.

The refreshed default fleet artifact fixes a stale receipt for 18 grammars,
reproduced on untouched baseline. Baseline and repaired-default tables match
exactly on all 206 current blobs: **155,201,856 bytes**, with no default compiled
table change. Native source checks, four further canonical Go/C comparisons,
386 helper tests and library build pass. L0 stays **756 / 12**; R6 **454 / 101**;
all **64** CI selectors resolve against **7,681** defined functions.

The final source completes twenty alternating shuffled pairs with the standard
serialized Docker/CPU-6 settings. Every affected timing is inconclusive:

| Workload | Before → after | Direction | p |
| --- | ---: | ---: | ---: |
| Go 20,168 bytes | 9,719.624 → 9,687.507 µs | -0.33% | .904 |
| Go 235,626 bytes | 103,160.799 → 102,952.433 µs | -0.20% | .758 |
| Go R4 | 827.835 → 824.152 µs | -0.44% | 1.000 |
| Owned YAML R4 | 362.393 → 365.041 µs | +0.73% | .602 |
| Owned YAML 8 KiB | 1,395.058 → 1,408.569 µs | +0.97% | .883 |

Allocation-count medians stay exact. Query bytes move **138,739→138,738**;
other compact byte medians stay exact. Full/edit/no-edit controls move **+0.27% /
−0.22% / −0.48%**, all inconclusive; no-edit stays allocation-free. This is a
correctness bucket with **no new throughput claim** and no ledger exception.
Source, native promotion and sample audits pass. Evidence is local
`gotreesitter-evidence/20261015-c4-reduce-shift-correctness`.

### Cold reduction-boundary frame split: discarded

The independent prototype extracts historical/certificate aggregation from the
common reducer without removing a proof or changing publication order. Native
assembly shrinks its active stack frame **1,064→752 bytes (−29.32%)**. Full core,
phase-zero-A, 414 exact rows, focused Go/YAML invariant/C checks and the completed
source-only Buckley review pass. An unused import in the first compilation is
fixed before the frozen gate and excluded from timing.

Twenty pairs establish no public parser gain: Go query **−0.27%** (p=.114),
large Go **+0.12%** (p=.529), Go R4 **−0.48%** (p=.414), owned YAML R4 **+0.63%**
(p=.231), and its stream **+0.21%** (p=.157). Allocation counts stay exact;
query bytes move **138,739→138,738.5** and other compact bytes stay exact.
Full/no-edit controls move **+0.18% / −0.59%**, inconclusively. The edit control
improves **0.36%** (p=.009) but bypasses the compact reducer and supplies no
causal win. The runtime change is discarded; no full-fleet, race, budget or RSS
certificate transfers to it. Raw samples and pins remain in local
`gotreesitter-evidence/20261015-reduction-boundary-frame-split`.

### Actual YAML shift-origin replay and checkpoint convergence

A read-only diagnostic replays actual shared shifts from their pre-shift parser
states, restoring the original lexer after every probe. Both the 1,998-byte R4
fixture and four-document 8 KiB witness retain their route, tree, root span,
error status, token/node work and scheduler/core work. Frozen inputs verify.

The preceding zero-width token **111**, shifted from state **176**, reproduces
the complete token but **not** the shared post-scan snapshot. Thus token equality
does not establish snapshot equality. At the actual subsequent shift from state
**819**, independent replay reproduces token **93** and the complete shared
after-snapshot. Carrying the divergent token-111 replay snapshot into that next
scan also converges: token and after-snapshot both match. R4 observes bytes
**346–350**; the document-stream witness observes **350–354**.

This is a two-step lexical witness, not a version-ownership or alternative-set
certificate. It does not authorize borrowing the request made from state 1756,
dropping an alternative, or changing admission. The next proof must bind actual
shift history and all participants. No runtime/routing or timing change follows.
Evidence is local `gotreesitter-evidence/20261015-yaml-shift-origin-replay/chain`;
the initial wrong election filter and the passing direct/chained probes remain
separate. Ordinary YAML and compact graduation stay gated.

### Layout and dispatch follow-ups: eight rejected screens

Eight independent screens on `c498e889e` production source preserve all 414 deterministic rows
and pass focused correctness checks. Packing lineage flags shrinks each dense
record 32→28 bytes but establishes no compact throughput win; its owned YAML
stream direction is +0.67% (p=.040). Exact immutable wrapper reuse leaves public
allocation counts unchanged and slows the two affected YAML benchmarks
0.84% / 0.81%. Compiled pure-reduction cohorts slow canonical query and large
Go about 1.67% each (p<.001). Direct built-in row inspection removes that
regression but establishes no gain (query −0.16%, p=.583; large Go −0.05%,
p=.620). Moving the existing conflict-policy rejection guards before token
projection also establishes no gain (query −0.22%, p=.461; large Go −0.99%,
p=.076). Extracting the unchanged cold condensation body regresses large Go
0.70% (p=.043), with other compact times inconclusive and unchanged allocations.
Reusing a classified cell for one unchanged owned header also establishes no
YAML gain (R4 −0.43%, p=.183; stream −0.24%, p=.314), with unchanged allocations;
unfinished Go samples are deliberately excluded after rejection.
State-indexed slot hints preserve full-key/generation validation but establish
no compact gain across twenty pairs (query Go +0.09%, p=.779; large Go +0.25%,
p=.640; YAML −0.58% / −0.09%, p=.478/.383). All affected allocation medians
remain exact. The final lazy, accounted cache passes six focused budget trials;
the earlier inline, reservation and build attempts are excluded. It is rejected.
All are rejected. The
[focused receipt](compact-layout-dispatch-screens-20261015.md) records the paired
measurements, controls, discarded partial run and qualification limits. No
runtime or graduation change follows; all merges to main remain held. Separate
read-only censuses find exact duplicate parents in only 0.58% of large-Go
subtrees and none in the two YAML witnesses; first-entry hits account for only
about 11% of boundary lookups. No broad cache follows from those observations.
The newer state/frontier census finds that all 129,327 successful large-Go
lookups (44.94% of 287,787 probes) revisit the first published key for that
state. Four observed/unobserved work comparisons and pinned Go digests pass.
This motivated the bounded slot-hint screen above, which preserves full-key
and generation checks but establishes no supported compact throughput gain.

### Go linear suffix: a larger publication target, not a runtime win

The [linear-suffix receipt](compact-linear-suffix-census-20261015.md) compares
four observed/unobserved canonical Go parses with exact work and deep digests.
After retaining live heads, frontier history and predecessor closure at every
C4 exit, plus checked later node reads, 49,246 of 148,368 large-Go states remain
candidates for a different representation (33.19%; other fixtures 33–37%).
Direct array reads are not exhaustively hooked, so this is an upper bound,
not authority to omit states or a speed/memory estimate.

A private transition prototype matches separate GSS pop/reduction behavior on
61 fixture shapes through arity 255, including extras and epsilon reductions,
with equal abstract link work and no graph publication. The continuation adds
private parent construction: 80 seeded shapes and epsilon cases match the
actual scheduler's metadata, next pops, head facts and abstract counters.
Actual-scheduler witnesses correct migrated-extra order and bind commits to
saved pop facts; callback/cap/failure checks and full core/phase-zero-A pass.
A further private handoff layer retains active references and every current
frontier key plus predecessor closure, then serializes through existing DAG
checks and transaction rollback. Forty 65-operation traces match all current
frontier derivations and lineage records, including checkpoint modes and
explicit owners. Six failure cases, ambiguous-prefix flush and a review-driven
batch guard pass with full core/phase-zero-A coverage. This is implementation
evidence, not throughput savings. Selected real-Go replay then adds deferred
scalar/set lineage persistence: 150 chosen corridors/prefixes match full native
work vectors, pinned trees and frontier graph/lineage semantics. Replay extends
727→1,621 operations; 106 handoffs now reach the opaque-prefix boundary. The
[focused receipt](compact-linear-suffix-census-20261015.md#real-go-replay-and-deferred-header-lineage)
records command/overflow regressions, diagnostic review fixes and the limits of
the selected coverage. Lazy deterministic-prefix import then reuses original
graph identities without new work or budget charges. In the same 150 selected
regions, completion rises 44→129 and replay 1,621→9,378 operations; native
counters/pinned trees and model frontier semantics remain exact. Remaining
handoffs are 16 canonical collisions and five ambiguous prefixes. Error-base
and logical-membership retry regressions pass after correction, alongside full
core/phase-zero-A tests. Runtime dispatch, terminal production,
certificate/recovery handling and memory integration remain open; no route,
throughput or graduation change follows. All merges stay held.

The [first real C4 runtime screen](compact-linear-suffix-census-20261015.md#first-real-c4-runtime-integration-correctness-passes-throughput-rejected)
then builds terminals and parents through actual parser dispatch. All four Go
pinned trees, core/scheduler work, reuse arrays/frontiers and ordering match.
Four locked-C Go cases, Go/YAML 48-step sessions with zero no-edit allocations,
and focused Go lifecycle/memory gates pass. Twenty pairs reject this version:
time rises **10.41–16.58%**, bytes **4.42–1,071.68%**, and allocations
**4.49–214.29%**, despite 5.4–7.0% fewer graph nodes. A separate exit trace finds
roughly 60% empty entries, mainly fork probes. Lazy acquisition cuts entries
11,449→4,780 on the large Go file but a loop-local cleanup defer escapes to the
heap. Compiler diagnostics support moving it before the loop. With that fix
and a bounded 64 KiB retained handle, three allocation counts match native;
the large file adds ten (+0.58%) and 0.99% bytes. A third 20-pair screen still
rejects CPU time (**+8.91–14.08%**). All complete observations and focused
C/edit/memory gates pass. The runtime source remains private/default-off;
a separate `gts_workcount` query test fails raw lookup identity
**37,662→39,188 (+4.05%)**, despite the earlier equal core/scheduler vectors.
CPU attribution puts imported-origin validation at 5.53% cumulative sampled
CPU. Compiled-row reuse and validation need qualified proofs; no counter
exception or runtime promotion follows. Full-fleet, race, RSS, on-disk source
guards and graduation qualification remain open.

The [compiled-row/handoff continuation](compact-linear-suffix-census-20261015.md#compiled-row-reuse-and-one-read-goto-handoff)
closes that measured lookup gap: all four complete diagnostic work vectors are
exact (raw lookups **7,582 / 37,662 / 37,652 / 400,160**). One-read pure GOTO
replay removes speculative fallback rereads, and its actual **24 bytes** are
charged even inactive. The focused C/edit/memory gates pass; the memory witness
rises exactly 24 bytes within its unchanged bound. Two further 20-pair screens
still reject promotion: accounted replay has three significant regressions and
one inconclusive case; borrowed validation retains every check but also shows
regressing times. Variance reaches ±12–15%, so no component speed gain is inferred.
A native Go/owned-YAML occupancy census finds **86.52–95.10%** nonempty sets,
rejecting a sparse-set layout before implementation. Error-cost/reference fields
are zero on these clean witnesses. The subsequent [native cold-plane screen](compact-linear-suffix-census-20261015.md#native-cold-errorreference-plane-qualified-screen-throughput-rejected)
implements exact 24-byte hot records with checked cold allocation, nested undo,
recovery copies and memory accounting. Four Go diagnostic work vectors and tree
pins, focused C/edit/memory checks, a 386 regression and backend race pass.
Twenty pairs reject throughput: Go is **1.15–2.56% slower**; owned YAML directions
are **+2.42% / +5.09%**, both inconclusive. Ordinary YAML fallback bytes fall
**4.48%**, with no confirmed time gain. No cold-plane runtime is promoted.
The rejected cold-plane source is not promoted; the separate qualified tagged
layout follows below. Ordinary YAML ownership, allowlists and graduation status
remain unchanged, and all main merges stay held.

### Qualified tagged alternative sets: smaller storage and small Go gains

[Core PR #1485](https://github.com/odvcencio/gotreesitter/pull/1485), Buckley commit
`b45a0192520f873edf0eb2fdcca169dbc153a748`, shares the exact inline-member and
spill-offset word under the existing flag. Sets shrink **12→8 bytes**, graph
lineage **32→28**, dispatch headers **88→80**, canonical groups **48→40**, and
reduction handoffs **56→48**, preserving widths, flags,
cap, membership and rollback. A checked offset prevents wrap before append.
The single accessor stays within the compiler budget; an observed two-allocation
regression is eliminated before promotion and verified by a failing control.

The [qualified receipt](compact-linear-suffix-census-20261015.md#qualified-tagged-alternative-set-storage)
records **414 exact rows / 206 invariant sessions / 618 C checks / 204 exactly
unchanged strict rows**, native source/race and CI-selector guards, tighter layout assertions, and focused
opt-in fusion checks. Twenty final pairs support **0.78% / 0.65% / 1.24%** less
compact Go time on rewrite/language/large Go; query is inconclusive. All final
YAML timing directions are inconclusive, including **+0.42%** on the owned
8 KiB witness. Ordinary YAML bytes fall **2.24%** with unchanged allocation
counts. The earlier wrapped-source YAML timing does not transfer to this source.

Accounted large-Go core storage falls **26,056,865→25,349,985 bytes (−2.71%)**;
total scheduler storage falls **2.56%**. The 1 MiB owned YAML stream remains
C-exact without fallback at **144,484 KiB** Go+C process RSS, a single safety
probe. Ordinary YAML ownership and the larger throughput gaps remain; no route,
allowlist or graduation change follows. The qualified change stays unmerged
under the owner's 0.56.0 main-merge hold.

### Lineage proof sharing: smaller storage, rejected throughput

On `c498e889e`, an exact immutable interner reduces retained large-Go lineage
storage **5,655,040 → 2,364,032 bytes (−58.20%)** while preserving all metadata,
rollback and memory accounting. Twenty shuffled pairs reject it: query/large
Go time rises **9.12% / 10.60%**, Go R4 **9.06%**, and owned YAML R4/stream
**6.31% / 5.65%** (all p<.001). All 414 rows are exact, and allocation counts
are unchanged. The private runtime is discarded; broader qualification is not
transferred. The [focused receipt](compact-lineage-sharing-screen-20261015.md)
records the complete samples, scope, controls and storage census.

A separate current-source YAML replay probe narrows the preceding same-token
snapshot mismatch to **`extZeroState`**. State-819 replay still converges from
that divergent marker snapshot. This identifies an active retry-cache key;
it does not authorize an ownership bypass or ordinary-YAML routing change.

### Exact classification recurrence: rejected cache

Baseline is `c498e889ec2d86382a3a0baad58020d7df7145b3`. A read-only census
preserves all four canonical Go digests and deterministic work. Repeated
(owner, authentication generation, head, lookahead) keys always reproduce the
same complete capability:

| Go fixture | Generic classifications | Exact repeats | Simulated 16-slot hits |
| --- | ---: | ---: | ---: |
| rewrite | 2,131 | 697 | 696 |
| query_compile | 10,047 | 3,289 | 3,283 |
| language | 11,688 | 4,569 | 4,542 |
| grammargen_lr | 114,856 | 41,905 | 41,566 |

The prototype caches only tables declaring immutable action rows, validates the
existing owner/phase/transaction capability on every hit, and clears all Core
and row references at scheduler-run entry/exit. Mutable tables use ordinary
classification. Contextual deferral, selection and action authentication remain.
Logical lookup counters retain the corridor's convention. Tests cover phase,
reset, rollback/ordinal reuse, ended transactions, foreign owners, symbols and
invalid heads. A pointer-dereference compilation failure is corrected before
the frozen gate and excluded from timing.

Core and phase-zero-A, default/opt-in C4, focused Go/YAML/CMake/HTML/Python
invariant/C checks and **414 exact ledger rows** pass. The completed source-only
Buckley review establishes no defect in the stated private lane, with explicit
limits on the immutability and invalidation assumptions. It gives no merge
approval or unseen-path qualification.

Twenty paired shuffled runs reject the implementation: public query Go is
**+1.08%** (p=.010), large Go **+1.47%** (p=.020), and Go R4 **+0.74%** (p=.289).
Owned YAML R4 **+0.13%** (p=.947) and stream **−0.46%** (p=.301) are inconclusive.
Every compact byte/allocation median remains exact. Bypassed full/edit/no-edit
controls move **+0.78%** (p=.046), **−0.40%** (p=.355) and **+1.18%** (p<.001);
full bytes rise **1,159→1,160**, and no-edit remains allocation-free. No cache,
marker interface or scheduler field is promoted. No full-fleet/race/RSS/budget
certificate transfers. Pins, failed compilation and all raw samples remain in
local `gotreesitter-evidence/20261015-classification-cache`; the recurrence
census is `gotreesitter-evidence/20261015-classification-recurrence`.

### Current Go attribution and provenance storage target

A four-second current-source large-Go public-entrypoint CPU profile attributes
**79.07%** cumulatively to the scheduler, **36.82%** to generic dispatch,
**19.72%** to election and **9.66%** to condensation. Borrowed postorder
materialization is **10.87%** cumulatively; replay shift lookup alone is **1.01%**
flat. The standalone replay helper is inactive on this path: replay is already
folded into materialization. The point index already caches the current/next
line. These findings reject plans that assume another replay traversal or an
uncached coordinate search. Profiling is attribution, not randomized speed
evidence. Binary, CPU samples and source hashes remain in local
`gotreesitter-evidence/20261015-go-publication-profile`.

A separate clean-cohort census preserves canonical digests/work and finds
**17,484** large-Go multi-header passes whose observed rows are ordinary
shift/reduce cells with at least one reduction, among **51,764** observed
multi-header passes. Those eligible passes contain **37,881 / 108,187** cells.
The counts identify a bounded corridor-extension opportunity; they do not prove
that its full dispatch policy may be skipped. Evidence is local
`gotreesitter-evidence/20261015-classification-recurrence/cohort-coverage`.

A read-only graph census identifies a larger representation target:

| Go fixture | Graph lineage records | Distinct final records | Distinct observed construction records | Distinct observed with owner removed |
| --- | ---: | ---: | ---: | ---: |
| rewrite | 2,996 | 445 | 1,282 | 450 |
| query_compile | 14,663 | 1,763 | 5,232 | 1,797 |
| language | 15,074 | 2,043 | 6,041 | 2,431 |
| grammargen_lr | 148,368 | 21,104 | 63,268 | 21,765 |

Each current lineage record is **32 bytes**. Large Go retains **4,747,776 live
bytes / 5,655,040 capacity bytes** in that array. **141,100** records carry an
alternative set; only **7,268** have no fields beyond owner. A sparse zero-only
scheme therefore has little coverage. Separating the owner from shared immutable
proof records is a concrete candidate, but no new representation, allocation or
throughput saving is claimed. The construction observer samples reads, owner
writes, proof invalidations and final values; it is not an exhaustive mutation
proof or an intern-table sizing bound. Final-state uniqueness alone understates
the observed value stream by about three times. Repeated immutable values also
do not eliminate the need for exact rollback and reference-lifetime handling.

Both censuses preserve pinned canonical digests and scheduler/core work. Fresh
sessions already keep the transaction stack empty, so ordinary journal elision
is not a fresh-path optimization. Runtime and routing remain unchanged.
Evidence is local `gotreesitter-evidence/20261015-lineage-storage-census` and its
`observed-values` subdirectory. Compact graduation remains gated.

### Ordinary YAML stop census: the first decline is near byte 350

A test-only inspection on `fb4c8acc4` holds the existing runner lease until
after the production compact entrypoint returns. Engine code and options are
unchanged. Standalone and inspected attempts have identical acceptance and
decline reasons; accepted trees also match S-expressions, end spans and error
flags. The pinned R4 source and its four-/sixteen-document wrappers report:

| Source bytes | Ordinary tokens before decline | Current token byte span | Diagnostic-owned tokens / requests | Owned result |
| ---: | ---: | ---: | ---: | --- |
| 1,998 | 17 | 346–350 | 102 / 18 | Accepted |
| 8,012 | 18 | 350–354 | 403 / 68 | Accepted |
| 32,048 | 18 | 350–354 | 1,603 / 284 | Accepted |

All ordinary attempts decline with two headers because a converged-path
no-action drop lacks alternative-set coverage by one non-blended survivor.
This identifies the current first blocker independently of the later EOF
diagnostics. Keep the coverage proof mandatory and investigate how owned relex
supplies valid viability decisions. The post-run ownership flag is false even
for successful owned parses; the request counters above establish actual owned
execution. This census is diagnostic evidence, not throughput or graduation
evidence. All six cases, pins and terminal exit zero verify. Evidence is local
`gotreesitter-evidence/20261015-yaml-stop-census`.

### YAML live frontier: identify the missing lexer certificate

A follow-up on `bb5022df4` observes the live frontier immediately before the
first drop decision. The observer preserves complete scheduler/core work and
header values; observed and unobserved parses retain identical routes, public
node/token counts, S-expressions, spans and error flags on all six cases above.
Ordinary and owned parsing reach the same two states (**696 / 1748**), header
sequences (**0 / 2**), scanner checkpoints and alternative sets. The survivor
is blended in both modes. Ordinary parsing therefore still lacks the required
non-blended coverage witness.

Owned parsing instead authenticates one request per header: originating states
**1756 / 1748**, equal before/after checkpoint IDs, equal complete before/after
snapshots, identical tokens and no ragged spans. Both before snapshots equal
the shared election's saved DFA/scanner state. The shifted survivor consumes
its own sequence-matched request; the paused dropped header retains its own.
The ordinary shared election lists state **809**, which is not proof that
either eventual header independently requested this token. Token equality or
checkpoint equality alone cannot replace that missing provenance.

This narrows the next design question to the cost of authenticating each
header's lexical viability. It authorizes no coverage waiver, owner shortcut,
ordinary-route change or graduation. All six cases and terminal exit zero
verify; observation is separate from performance measurements. Evidence is
local `gotreesitter-evidence/20261015-yaml-frontier-census` and its `requests`
subdirectory, including complete request comparisons and pinned overlays.

Two further probes on the same `bb5022df4` source compare each owned request's
complete after-DFA/scanner snapshot with the shared election's saved checkpoint
payload and current cursor. Both requests match the shared **before and after**
values on all three witnesses. Six observed/unobserved cases preserve work,
headers, routes and trees; the observers neither re-lex nor publish ownership.

The ordinary survivor's shared shift originates in state **819**, at byte
**345 / 349 / 349** for R4/four/sixteen documents. It uses exactly the shared
token and begins at the shared before-cursor. The owned request retained for
that sequence instead originates in **1756**. Reductions can separate a
lookahead request from its later shift state; the last shift cannot stand in
for the original request's identity. A cheaper certificate must preserve the
actual request state and sequence through those reductions. These are
read-only diagnostics, not an admitted proof or throughput improvement.
Evidence adds `shared-after` and `shared-shift` beneath the frontier census.

A read-only grammar-table probe further narrows that state relationship.
The ordinary election state **809**, owned request origins **1756 / 1748**,
and ordinary shift state **819** have identical complete lexer modes: DFA
state 0, external state 9, and the same 17-symbol external validity row. The
resulting survivor state **696** instead uses external state **44**, whose row
contains only symbols 20 and 112. The original lexer request must therefore
remain tied to its origin through reduction and shift; the resulting head's
state cannot reconstruct it. Equal lexer modes alone do not authenticate
request ownership or all state-dependent token-source behavior. No certificate,
route change, or speed result follows. The probe passes on pinned `bb5022df4`
with terminal exit zero; evidence is
`gotreesitter-evidence/20261015-yaml-frontier-census/lex-modes`.

### Go read-history census: exact bounds need less storage

A diagnostic on `bb5022df4` records the history immediately before sealing.
Observed and unobserved compact parses have identical public trees, spans,
error flags and actual routes on four canonical Go files:

| File | Recorded entries | Distinct origins | Prefix-bound increases | Largest read width |
| --- | ---: | ---: | ---: | ---: |
| rewrite | 1,676 | 1,667 | 1,665 | 92 bytes |
| query_compile | 8,177 | 8,137 | 8,112 | 94 bytes |
| language | 8,332 | 8,307 | 8,303 | 85 bytes |
| grammargen_lr | 85,028 | 84,740 | 84,622 | 92 bytes |

Large Go retains nearly every bound, so deduplication has little coverage.
Its backing array grows to **131,072 eight-byte entries**, while its narrow
read widths suggest a smaller exact representation. This is storage
attribution, not a throughput result or permission to weaken dependency
bounds. All four cases pass and the pinned container exits zero. Evidence is
local `gotreesitter-evidence/20261015-read-history-census`.

### Packed leaf receipts: qualified Go allocation win

On incremental-lane baseline `a32d87b6d`, pack the two leaf-provenance flags
into sixteen receipts per word. Terminal, symbol, span, missing-token and state
checks remain intact. Allocation charging still precedes growth, and all
dependency scratch retains the existing shared **64 KiB** ceiling. Unused tail
slots are cleared after truncation to preserve the old exact-ID bound. A
regression fails on the initial packing revision because truncated slot 7
retains provenance; the corrected source passes. That initial screen is
excluded and contributes no samples or qualification to this result.

Twenty fresh alternating shuffled pairs use the repository runner, one
language/process/seed/role, `GOMAXPROCS=1`, 750 ms, `-count=1`, `-benchmem`, CPU 6
and serialized 6 GiB Docker:

| Compact Go file | B/op before → after | Allocations before → after |
| --- | ---: | ---: |
| query_compile, 20,168 bytes | **141,139 → 140,563 (−0.41%)** | **205 → 203** |
| grammargen_lr, 235,626 bytes | **3,295,728 → 2,247,664 (−31.80%)** | **3,092 → 3,081** |

All other byte/allocation medians stay identical: Go R4 **1,696 / 25**; owned
YAML **12,936 / 111** and **44,601 / 345**; default full/edit/no-edit controls
**1,159 / 6**, **394 / 5** and **0 / 0**. Timed node-work and route/fallback
counters stay exact. All 412 ordinary ledger rows and the two owned-YAML rows
are identical before timing.

| Workload | Before → after | p |
| --- | ---: | ---: |
| Compact Go query_compile | 10.100 → 10.109 ms | .445 |
| Compact Go grammargen_lr | 107.458 → 107.588 ms | .841 |
| Go R4 compact | 854.086 → 847.223 µs | .547 |
| Owned YAML R4 | 386.681 → 385.771 µs | .989 |
| Owned YAML document stream | 1.489940 → 1.487732 ms | .429 |
| Go full DFA control | 3.869754 → 3.892795 ms | .183 |
| Go single-byte edit control | 99.123 → 99.449 µs | .134 |
| Go no-edit control | 3.112 → 3.144 ns (**+1.03%**) | <.001 |

This is an allocation win. Go and YAML throughput directions remain
inconclusive. The no-edit control bypasses receipt construction and remains
allocation-free; its measured 0.032 ns increase is retained without causal
attribution. The substantial byte reduction is the accepted trade. No counter
exception or baseline reset is requested.

Final source passes **206 invariant sessions**, **618 locked-C fixture checks**,
full incremental/core suites, correct-tag phase0A, incremental/core/root/YAML
races and six bounded-stop budget trials. All **204 strict census rows are
exactly unchanged: 159 PASS / 45 known FAIL**, including **120 actual
compact/C-exact acceptances**. Four further canonical Go/C comparisons pass,
including both timed files. Budget peaks are **1,081,840 / 13,068 bytes** within
the existing bounds. The **1,049,572-byte / 524-document YAML stream** remains
C-exact with complete coverage, no error/fallback and **157,132 KiB process
RSS**. The prior separate storage receipt measured 150,112 KiB (+4.68%); these
single Go+C process probes are not a controlled RSS comparison or a graduation
memory certificate.

L0 retains **756 root files / 12 API tag sets**; R6 **454 language-name uses /
101 environment reads**. Test-package execution coverage and all 64 workflow
selectors pass. All pins, promoted source files and terminal exits verify
without OOM or timeout. Buckley commit
`a977238f9eb1548ea671dedecb6750f58906bc43` promotes this allocation win in
[incremental PR #1480](https://github.com/odvcencio/gotreesitter/pull/1480).
It is independent of the core/header timing comparison above. Ordinary YAML
activation, compact graduation and #1317's mutex removal remain open. Evidence
is local `gotreesitter-evidence/20261015-packed-leaf-receipts-bounded`; the
excluded initial comparison is `20261015-packed-leaf-receipts-incr`.

### Packed read history: qualified Go allocation win

On incremental-lane parent `a977238f9`, encode each lexer scan origin and
exact examined width in one 32-bit word when they fit. Source size determines
the split; a wider read promotes the history to two words per record. Charge
storage before growth or promotion, and abstain when unfunded. Sealing retains
all failed reads and the same prefix maxima, including separate parent and
leaf boundary rules. An earlier probe's width bounds its contribution at any
later origin, so sealed prefix maxima still fit the packed form. Reset clears
authorization; the shared **64 KiB** retention ceiling stays unchanged.

Twenty alternating shuffled pairs use the repository runner, one
language/process/seed/role, `GOMAXPROCS=1`, 750 ms, `-count=1`, `-benchmem`, CPU 6
and serialized 6 GiB Docker. All 412 ordinary rows and two owned-YAML rows
remain exactly identical before timing. All measured node counts and routes
stay exact.

| Compact Go file | B/op before → after | Allocations before → after |
| --- | ---: | ---: |
| query_compile, 20,168 bytes | **140,563 → 10,512 (−92.52%)** | **203 → 196 (−3.45%)** |
| grammargen_lr, 235,626 bytes | **2,247,664 → 1,199,600 (−46.63%)** | **3,081 → 3,081** |

The smaller file's complete history now fits within the existing retained
allowance, avoiding repeated buffer growth. Larger histories still drop
oversized capacity. Other byte/allocation medians remain identical: Go R4
**1,696 / 25**, owned YAML **12,936 / 111** and **44,601 / 345**, and default
full/edit/no-edit controls **1,159 / 6**, **394 / 5**, **0 / 0**.

| Workload | Before → after | p |
| --- | ---: | ---: |
| Compact Go query_compile | 10.222411 → 10.102711 ms | .127 |
| Compact Go grammargen_lr | 108.562575 → 107.897197 ms | .698 |
| Go R4 compact | 861.4215 → 867.2055 µs | .429 |
| Owned YAML R4 | 415.106 → 418.064 µs | .779 |
| Owned YAML 8 KiB | 1.545531 → 1.570377 ms | .718 |
| Go full DFA control | 3.919085 → 3.916125 ms | .327 |
| Go single-byte edit control | 100.471 → 99.1855 µs | .091 |
| Go no-edit control | 3.1235 → 3.1565 ns (**+1.06%**) | .048 |

This is an allocation win; all affected throughput directions are
inconclusive. The no-edit control bypasses history construction and retains
zero allocation. Record its 0.033 ns increase without attributing it to this
change. No deterministic-counter exception or baseline reset is requested.

Final engine source passes **206 invariant sessions**, **618 locked-C fixture
checks**, full incremental/core suites, correct-tag phase0A,
incremental/core/root/YAML races and six memory-budget trials. All **204 strict
census rows are exactly identical: 159 PASS / 45 known FAIL**, including
**120 actual compact/C-exact acceptances**. Four canonical Go/C comparisons
pass. Long-comment and raw-string Go witnesses force wide storage after short
probes and match C on initial, incremental-edited and fresh-edited trees (six
further comparisons). Reference-model tests exercise both cursor directions,
source-width limits, restores, promotion, budget denial and reset. The initial
screen's old eight-byte budget expectation is corrected while retaining its
one-growth/next-growth-refusal assertions; that screen is excluded. A launcher
race refused timing before the correctness exit file existed and produced no
samples; the unchanged runner starts only after observed correctness exit zero.

Native promoted source repeats incremental units and root retention/budget
tests. Budget peaks are **1,049,072 / 12,556 bytes**. The **1,049,572-byte /
524-document YAML stream** stays C-exact with complete coverage, no
error/fallback and **156,396 KiB process RSS**. This single Go+C probe is not a
controlled RSS comparison or graduation memory certificate. L0 stays
**756 root files / 12 API tag sets** and R6 **454 language-name uses / 101
environment reads**. Package execution coverage, 64 workflow selectors and
selector units pass. All pins and terminal exits verify without OOM/timeout.
Buckley `16c4519298cc41027b4dff47c0f2821b837ee293` promotes the win in
[incremental PR #1480](https://github.com/odvcencio/gotreesitter/pull/1480).
Evidence is local `gotreesitter-evidence/20261015-packed-read-history/bounded`.
Earlier allocation comparisons remain independent and are not added.

The final-source Go CPU profile has **6.63 seconds** of samples: dispatch
**41.78%**, reduction application **26.70%**, election **17.50%** and
materialization **19.61%** cumulative CPU, with **4.22%** flat copies. These
shares overlap. They identify dispatch/reduction work for the next throughput
experiment; they do not establish a further speedup or close #1313/#1319.
Ordinary YAML activation and compact graduation remain gated.

### Fresh reduction publication split: discarded

On core parent `bb5022df4`, a two-file overlay separates the existing fresh
single-output publication from the general frontier builder. The internal
eligibility predicate retains all original freshness/history/pop/reference
conditions. The adapter restricts the small publisher to fresh, single-header,
non-recovery, shared-lexer, ordinary-token rows without an action-selection
override or full receipts. Core still performs the same authenticated reduction. Other cases
retain the general publisher. Native assembly changes the reduction function's
reserved frame from **1,400 to 800 bytes**, plus **112 bytes** in the publisher;
these static reservations are not total stack usage or a speed measurement.

Core units, focused C4/root ownership/rollback tests, Go/YAML edit invariants
and focused locked-C comparisons pass. All **412 ordinary rows** and **two
owned-YAML rows** are exactly unchanged before timing. Twenty alternating
shuffled pairs use the repository runner, one language/process/seed/role,
`GOMAXPROCS=1`, 750 ms, `-count=1`, `-benchmem`, CPU 6 and serialized 6 GiB Docker:

| Workload | Before → after | p |
| --- | ---: | ---: |
| Compact Go query_compile | 9.814483 → 9.786957 ms | .841 |
| Compact Go grammargen_lr | 104.989754 → 104.506568 ms | .461 |
| Go R4 compact | 826.524 → 822.353 µs | .314 |
| Owned YAML R4 | 354.9105 → 356.623 µs | .779 |
| Owned YAML 8 KiB | 1.379104 → 1.383649 ms | .547 |
| Go full DFA control | 3.881011 → 4.004763 ms (**+3.19%**) | <.001 |
| Go single-byte edit control | 99.172 → 99.2015 µs | .826 |
| Go no-edit control | 3.069 → 3.1685 ns (**+3.24%**) | <.001 |

All byte/allocation medians remain identical, including no-edit zero. Timed
node-work and routes stay exact. There is no supported affected-path speed or
allocation benefit. The full/no-edit controls bypass the changed publication
path; retain their directions without causal attribution. Reject this trade
and keep runtime unchanged. It is not a hard 10% ratchet failure. No full-fleet,
race or RSS qualification follows this timing screen, and no earlier
qualification transfers. All samples, source pins and terminal exits verify
without OOM/timeout. Evidence is local
`gotreesitter-evidence/20261015-fresh-reduction-publication`, including the
external patch, assembly, complete medians, benchstat and disposition.

### Graph-link storage: a larger representation target

A separate read-only census on `bb5022df4` inspects completed Go graph arenas.
Observed and unobserved parses retain identical complete core/scheduler work,
public structure and reuse eligibility. The current node and link records each
occupy **32 bytes**. Counts include retained history, not rolled-back records:

| File | Node / link records | Single-link nodes | Single-path nodes with one link | Unique zero-score/order-free links | Largest branch order |
| --- | ---: | ---: | ---: | ---: | ---: |
| rewrite | 2,996 / 2,995 | 2,850 | 2,499 | 1,513 | 168 |
| query_compile | 14,663 / 14,662 | 14,051 | 13,608 | 8,956 | 685 |
| language | 15,074 / 15,073 | 14,263 | 13,097 | 7,311 | 618 |
| grammargen_lr | 148,368 / 148,367 | 140,932 | 121,312 | 72,699 | 8,155 |

Large Go retains **176,720 slots / 5,655,040 bytes in each arena**. Among the
72,699 uniquely referenced zero-score/order-free single-path links, their
nodes retain nonzero running precedence in **72,638** cases. That 64-bit
precedence remains required. All recorded branch-order values fit fourteen
bits on these witnesses; the general runtime still needs the full 64-bit
range, including a wide form. No absent-order record has a nonzero stored
order on these inputs.

This ranks physical link storage as a larger target than another scheduler
frame split. A representation change must preserve exact scores, full order
values, stable LinkIDs, adjacency, ownership, rollback and checked budgets.
The census omits external LinkID references and rollback lifetimes and cannot
authorize link removal or merging. It establishes neither an RSS saving nor a
throughput result. All four cases pass with pinned source and terminal exit
zero. Evidence is local `gotreesitter-evidence/20261015-gss-link-storage-census`
and its `orders` subdirectory. No runtime or graduation policy changes.

### Packed graph links: storage saving, rejected throughput screen

On core parent `bb5022df4`, an external overlay stores physical graph links in
24 bytes instead of 32. It preserves LinkIDs, full signed score deltas, and
full unsigned 64-bit orders. Common orders share the flags word; wide orders
use a sorted sparse history keyed by LinkID. Rollback truncates that history
with the graph, and EOF recovery shadows copy it independently. A regression
first reproduces an aliased wide-order shadow, then passes after the copy fix.
Unknown flags and malformed or missing wide orders fail closed.

A deterministic retained-capacity comparison preserves node/link counts and
capacities on all four canonical Go files. Large Go's link arena falls from
**5,655,040 to 4,241,280 bytes (−25%)**, and recorded total core footprint falls
from **26,056,865 to 24,643,105 bytes (−5.43%)**. No wide orders occur on these
four files. These are core capacity measurements, not process RSS or per-parse
allocations. A wide record instead needs 40 bytes on amd64, versus 32 before;
the common-case compression does not bound the general order domain.

The complete standard, phase0A, and EOF-shadow core suites, focused Go/YAML
72-step incremental invariants, no-edit zero allocations, and focused locked-C
checks pass. All **412 ordinary plus two diagnostic-owned ledger rows are
exactly unchanged** before timing. Twenty paired shuffled seeds then use the
repository randomized runner, alternating roles, 750 ms, `-count=1`, `-benchmem`,
`GOMAXPROCS=1`, CPU 6, and serialized 6 GiB Docker:

| Witness | Before | Packed links | Change | p |
| --- | ---: | ---: | ---: | ---: |
| Go query_compile, 20.2 kB | 9.872462 ms | 10.336749 ms | **+4.70%** | <.001 |
| Go grammargen_lr, 235.6 kB | 106.417349 ms | 109.173475 ms | **+2.59%** | .009 |
| Go R4 | 835.763 µs | 866.577 µs | **+3.69%** | <.001 |
| Owned YAML R4 | 352.574 µs | 363.330 µs | **+3.05%** | <.001 |
| Owned YAML stream, 8,012 bytes | 1.370737 ms | 1.422958 ms | **+3.81%** | <.001 |
| Full-DFA control | 3.951727 ms | 3.924229 ms | −0.70%, inconclusive | .678 |
| Single-byte-edit control | 99.719 µs | 99.310 µs | −0.41%, inconclusive | .277 |
| No-edit control | 3.098 ns | 3.188 ns | **+2.91%** | <.001 |

All five compact witnesses retain identical allocation medians: respectively
138,739 / 130, 3,251,728 / 1,717, 1,600 / 22, 12,872 / 109, and 44,313 / 336
bytes / allocations. The no-edit control bypasses the link codec and remains
zero bytes / zero allocations; its change is recorded without causal
attribution. The smaller physical layout does not qualify as a throughput win.
A tagged CPU profile identifies link expansion during reduction and publication
validation, plus the new packing call, for a separate follow-up. An initial
profile omitted the required tag, executed no benchmark, and is excluded.

The measured revision is not promoted. No full 206-language invariant fleet,
618-check C fleet, strict corpus sweep, race qualification, or RSS gate is
claimed for it. All focused, capacity, and randomized jobs exit zero without
OOM or timeout, and source pins verify. Evidence is local
`gotreesitter-evidence/20261015-packed-graph-links`, including the frozen source,
414-row comparison, capacity records, raw samples, medians, profiles, and the
first-screen disposition.

### Packed-link hot readers: rejected follow-up

A second frozen overlay keeps the same representation but reads packed links
directly in single-link reduction and node publication validation. Common
appends pack their fields locally; wide-order and invalid-flag cases retain
the checked fallback. Additional tests preserve wide orders through the hot
pop path, including trailing extras, and reject missing, malformed, or reserved
storage in both hot readers. These hot-reader and default codec tests pass on
amd64 and 386. The standard, phase0A, and EOF-shadow core suites, focused
Go/YAML invariants and C checks pass again; all **414 ledger rows remain exact**.

Twenty new paired shuffled seeds use the same runner and settings. These are
separate comparisons against `bb5022df4`, not paired with the first screen:

| Witness | Before | Packed hot readers | Change | p |
| --- | ---: | ---: | ---: | ---: |
| Go query_compile | 9.745135 ms | 9.899591 ms | **+1.58%** | <.001 |
| Go grammargen_lr | 102.708431 ms | 104.682848 ms | **+1.92%** | <.001 |
| Go R4 | 819.702 µs | 828.331 µs | **+1.05%** | <.001 |
| Owned YAML R4 | 352.158 µs | 354.520 µs | +0.67%, inconclusive | .096 |
| Owned YAML stream, 8,012 bytes | 1.373524 ms | 1.381539 ms | +0.58%, inconclusive | .221 |
| Full-DFA control | 3.862722 ms | 3.853248 ms | −0.25%, inconclusive | .583 |
| Single-byte-edit control | 98.594 µs | 99.052 µs | **+0.46%** | .011 |
| No-edit control | 3.0665 ns | 3.0925 ns | **+0.85%** | .001 |

Every allocation median is unchanged, including zero-byte, zero-allocation
no-edit parsing. The edit and no-edit controls bypass the packed-link code;
their changes are recorded without causal attribution. This revision also
remains outside the runtime PR. It has no full fleet, race, or RSS qualification.
All gate and randomized jobs exit zero without OOM or timeout; frozen source
pins verify. Evidence is the `lazy-view` subdirectory of
`gotreesitter-evidence/20261015-packed-graph-links`.

### Packed-link scalar views: rejected final screen

A third frozen overlay extends direct packed-record access to the remaining
scalar-only readers. Full logical records are expanded where scratch or an API
actually requires one; those common decodes no longer return a Boolean tuple.
The same full-width fallback, storage validation, rollback, clone isolation,
and error checks remain. Standard, phase0A, and EOF-shadow core suites, focused
Go/YAML invariants and locked-C checks, and hot/default codec tests on amd64
and 386 pass. All **414 work/reuse rows are exactly unchanged before timing**.

Twenty independent paired seeds against `bb5022df4` use the same randomized
runner and stable settings:

| Witness | Before | Packed scalar views | Change | p |
| --- | ---: | ---: | ---: | ---: |
| Go query_compile | 9.753381 ms | 9.859785 ms | **+1.09%** | <.001 |
| Go grammargen_lr | 103.421419 ms | 105.471250 ms | **+1.98%** | <.001 |
| Go R4 | 822.777 µs | 831.296 µs | **+1.04%** | .012 |
| Owned YAML R4 | 352.152 µs | 352.462 µs | +0.09%, inconclusive | .883 |
| Owned YAML stream, 8,012 bytes | 1.368129 ms | 1.374845 ms | +0.49%, inconclusive | .063 |
| Full-DFA control | 3.878085 ms | 3.862955 ms | −0.39%, inconclusive | .134 |
| Single-byte-edit control | 99.156 µs | 99.149 µs | −0.01%, inconclusive | .512 |
| No-edit control | 3.0565 ns | 3.0750 ns | **+0.61%** | .001 |

Every allocation median remains unchanged; no-edit parsing stays at zero
bytes and allocations. The no-edit control bypasses the codec, so its direction
is recorded without causal attribution. All jobs exit zero without OOM or
timeout, and all frozen source pins verify. This revision is also rejected
without full-fleet, race, or RSS qualification. The 24-byte layout has not
produced a throughput win across these three independently paired revisions;
none enters the runtime PR. Evidence is the `scalar-views` subdirectory of
`gotreesitter-evidence/20261015-packed-graph-links`, including its unchanged
ledger, source, regressions, raw samples, medians, and disposition.

### One-pass selected projection: qualified output win

Buckley commits `24c4f142` and `4c124dfa` in
[output PR #1484](https://github.com/odvcencio/gotreesitter/pull/1484) fix
synthetic/padded root metadata and promote the qualified one-pass folder.
Twenty paired samples measure selected Go **−2.08% / −3.06%** at 20.2 / 235.6 kB,
with **−32.85% / −33.68% retained selected backing**. Owned YAML selected
projection is **−0.83%**, with 103→101 allocations and −20.79% store backing.
All 412 rows, reported work, node counts and pinned digests stay exact; 206
invariants, 618 C fixtures, races, caps, layout and final L0/R6 checks pass.
Native Go/YAML controls remain inconclusive and allocation medians unchanged.
The [one-pass receipt](https://github.com/odvcencio/gotreesitter/blob/codex/selected-supertype-projection/docs/compact-selected-onepass-receipt-20261014.md)
and [root-framing receipt](https://github.com/odvcencio/gotreesitter/blob/codex/selected-supertype-projection/docs/compact-selected-root-framing-receipt-20261014.md)
keep independent timings, every direction and qualification limits. Root
framing's separate +4.49% full-DFA control direction remains recorded.
This promotes an output builder; ordinary YAML ownership, native-parser
throughput, the compact default/allowlist and graduation remain open.

### Native selected projection: promising construction reduction, unqualified

A native-publication prototype removes 33.57% / 33.53% of Go node
constructions and matches six clean Go/YAML trees on public structure, replay
states and fragility. Its two 20-pair screens still fail the large-Go allocation
ratchet (+28.05%, then +13.45%). The broader comparison also catches omitted
lexer-read receipts. The corrected transfer matches all checked metadata on
six fixtures; its third 20-pair screen measures large Go −2.40% (p=.001) but
still fails the allocation ratchet at +11.33%. The prototype stays outside the
runtime. The
[architectural screen receipt](compact-native-projection-screen-receipt-20261014.md)
records these limits, every measured direction and the optional-plane layout
work. These are diagnostic screens, not qualified native-parser speed wins.
Ordinary YAML ownership and compact graduation remain open.

### Direct replay into native projection: component screen

The [direct-replay receipt](compact-native-direct-replay-receipt-20261014.md)
records exact metadata on seventeen accepted witnesses and a six-step edit
session with unchanged reuse and zero no-edit allocations. Separate 20-pair
screens measure large Go −2.49% (p=.043), owned YAML R4 −3.28% (p=.002), and
the owned 8 KiB witness −1.22% (p=.030). Smaller Go timing is inconclusive.
Large-Go bytes rise +9.60%, narrowly clearing the earlier allocation failure;
YAML bytes rise +1.23% / +2.09%, and the EOF witness adds two allocations.
The driver omits ordinary-entrypoint acceptance hooks. Full memory, C/fleet,
race/lifecycle, borrowed-tree and publication qualification remain open. This
prototype stays outside the runtime and supplies no graduation or ordinary
YAML ownership claim.

### Native projection public entrypoint: unqualified integration

The [public-entrypoint receipt](compact-native-entrypoint-receipt-20261014.md)
supersedes any inference that the direct-replay component timing is a runtime
win. All four public Go/YAML timing directions are inconclusive across separate
20-pair screens; large-Go bytes rise +10.43%, failing the ratchet. The
construction reductions remain, public shape and error/coverage checks pass,
and a forced live-working-set budget declines correctly. Owned YAML R4 saves
ten allocations, but the integration remains outside the runtime and advances
neither ordinary YAML ownership nor graduation. Full C/fleet/RSS and lifecycle
qualification is withheld after the failed Go screen.

### Streaming native construction: scoped YAML pathway

The [streaming receipt](compact-native-streaming-screen-receipt-20261015.md)
removes the selected plan and fuses raw replay with native construction. After
fixing pointer cleanup and publication-purpose gating, twenty pairs measure
owned YAML **−3.70% / −3.71%** on R4 / 8 KiB, with one extra allocation and
248 B/op (**+1.92% / +0.56%**). The same generic source regresses public Go
**+2.57% / +1.17%**, so it is not promoted. Native construction counts decrease
roughly 33.5% for Go and 52% for YAML, but those counts are not speed estimates.
Seventeen checked witnesses, six Go edits and zero-allocation no-edit reparsing
pass. The receipt separates earlier noisy screens, direct C/memory qualification
and final-source limits. Ordinary YAML ownership, all-language graduation and
runtime integration remain open; the qualified reduction/output PRs are unchanged.

### Reduction copying and link validation: not promoted

Borrowing a reduction path preserves all 412 work/reuse rows and focused Go,
YAML and locked-C checks, but its profile contribution is too small to prioritize.
An ordinary-link validation split then reduces compiler inline cost 101 → 74;
twenty paired samples still show no supported Go or owned-YAML throughput gain.
The Go edit control increases 0.39% (p=.024); all allocation medians stay exact.
The [screen receipt](compact-link-validation-screen-receipt-20261014.md) keeps
these rejected trials separate from #1485's qualified reduction win. No runtime,
route or graduation change follows. A public `Parser.Parse` profile points back
to generic dispatch and reduction application; its overlapping CPU shares are
attribution only. The follow-up census finds zero no-lookahead elections on all
four Go fixtures. Large Go already runs 91.07% of single-header passes through
C4, while 41.58% of all passes have multiple headers. That ranks multiple-version
bookkeeping above a no-lookahead corridor extension for these witnesses.
The complete wide-frontier census also limits simple shortcuts: only 871 large-Go
passes reduce one runnable cell with every sibling shifted, and only 208 have
multiple same-action reduction cells. A conservative adjacent-pass census finds
19,917 repeated clean head classifications (22.42% of wide-frontier classified
cells). This identifies a possible authenticated classification-reuse seam;
no cache, stale-boundary reuse or speed claim is introduced.

### Cohort lineage publication: not promoted

Batching ordered owner and provenance publication preserves all 412 work/reuse
rows and focused Go/YAML/C checks. Twenty pairs measure large public Go
−1.18% / −1.00% in two independent variants; neither gain is combined with #1485.
The full-DFA control regresses +3.95% / +4.38%, and a separate twenty-pair
control-only run confirms +3.68%. The [scoped receipt](compact-cohort-lineage-screen-receipt-20261014.md)
records all directions, the first version's YAML regression, exact pins and
unrun qualification. This trade stays outside the runtime. The owner-only
precursor has negligible coverage and its partial timing is excluded. No
ownership rule, route, allowlist or graduation status changes.

### Condensation cold split: discarded

Baseline `70b27618117e60eecd6e1aaa26552aa81b37f22a`. Moving the existing-boundary
fold/history body into `internal/parsercorephase0/condense_existing.go` reduces
the common condensation stack frame **1,032→280 bytes**. All 412 work/reuse rows
and owned-YAML rows remain exact. Both full core unit runs, canonical Go/lifecycle
checks, Go/YAML/owned-YAML edit invariants, Go C fixtures/R4 and YAML document,
version-ownership and recovery C checks pass. No language predicate or proof rule
changes. Twenty alternating shuffled pairs establish no throughput gain:

| Workload | Before → after | p |
| --- | ---: | ---: |
| Public Go, 20.2 kB | 10.136 → 10.180 ms | .640 |
| Public Go, 235.6 kB | 108.122 → 108.007 ms | .904 |
| Go R4 compact | 852.773 → 854.679 µs | .369 |
| Go full DFA | 3.933 → 3.921 ms | .369 |
| Go single-byte edit | 99.989 → 99.822 µs | .398 |
| Go no edit | 3.149 → 3.1505 ns | .250 |
| Owned YAML R4 | 388.429 → 388.570 µs | .883 |
| Owned YAML 8 KiB | 1.497 → 1.504 ms | .640 |

Every time direction is inconclusive; byte/allocation medians remain identical,
and no-edit allocation remains zero. The randomized runner uses twenty seeds,
750 ms, `-count=1`, `-benchmem`, `GOMAXPROCS=1`, CPU 6 and serialized 6 GiB Docker.
All roles complete, source hashes remain exact and containers exit zero.
Evidence: local `gotreesitter-evidence/20261015-condense-cold-split`, including
raw samples, counter JSON, before/after assembly and the exact external patch.
The runtime split is discarded; full fleet/race/RSS qualification is not run
after this failed timing screen. The smaller frame is not counted as a speed win.

### ASCII self-loop coalescing: discarded

An independent trial on `70b27618117e60eecd6e1aaa26552aa81b37f22a` groups repeated
non-skipping ASCII DFA transitions to the same state. Unicode, newline and SKIP
handling remain scalar. The new `internal/lexrun` helper is inlineable at compiler
cost 34 and its inputs do not escape. A frozen scalar scanner matches all 4,000
seeded cases and explicit long-run cases, including tokens, private lookahead
flags, accepted cursors and failure extents. All 412 ordinary work/reuse rows and
owned-YAML rows stay exact. Focused Go/YAML parser, invariant and C checks pass.

Twenty shuffled alternating pairs still establish no compact Go/YAML time win:

| Workload | Before → after | p |
| --- | ---: | ---: |
| Public Go, 20.2 kB | 10.214 → 10.221 ms | .799 |
| Public Go, 235.6 kB | 109.716 → 109.608 ms | .758 |
| Go R4 compact | 850.350 → 862.893 µs | .301 |
| Go full DFA | 3.932 → 4.199 ms (**+6.80%**) | .015 |
| Go single-byte edit | 99.732 → 101.703 µs (**+1.98%**) | .012 |
| Go no edit | 3.162 → 3.2605 ns | .126 |
| Owned YAML R4 | 385.482 → 384.508 µs | .678 |
| Owned YAML 8 KiB | 1.495 → 1.506 ms | .698 |

Full-DFA bytes rise 1,159→1,160 (+0.09%, p=.015); all other byte medians and all
allocation medians remain unchanged. No-edit stays 0 B/op and 0 allocations.
The run uses the same pinned Docker/CPU/settings and randomized runner as the
condensation split. All roles complete with exact source hashes and exit zero;
Go timing has wide distributions, retained in the raw evidence. An initial run's
differential tests pass but vet rejects an overlay-only package directory; that
wrapper is excluded. The complete gate creates/removes the empty directory and
passes with vet enabled. Evidence: `gotreesitter-evidence/20261015-ascii-self-loop`.
The runtime patch is discarded, with full fleet/race/RSS qualification withheld.
No ownership, proof, route, allowlist or graduation change follows either screen.

### Reduction replay source cache: discarded

Twenty pairs establish no Go or owned-YAML throughput gain from capturing
parent GOTO states. With replay enabled, 235.6 kB Go regresses 0.87% (p=.035).
All 412 work/reuse rows and the existing native-state differential stay exact;
allocation counts are unchanged, while active contexts add 8 B/op. The
[scoped receipt](compact-reduction-replay-source-cache-receipt-20261014.md)
records every direction, final source identity, qualified witnesses and
excluded attempts. The ten-file runtime prototype stays outside the repo.
The independent reduction win in #1485 and publication-construction census
remain separate buckets; this rejected cache advances no graduation claim.

### TSX conflict priority and runtime follow-up

The exact TSX artifact now preserves C's conflict-action source-version priority,
fixing [issue #1443](https://github.com/odvcencio/gotreesitter/issues/1443) when
appending a typed function would change an earlier generic call into a binary
expression. All six fresh/append regressions pass, and a surrounding-context
probe improves 124/128 → 128/128 C-exact checks without changing any node,
token, or maximum-stack counter. All 412 ordinary ledger rows remain
byte-identical; TSX's 72-step session, container races, C fixtures and real-file
oracle pass. Shared memory gates pass, and L0/R6 snapshots stay unchanged.

Twenty pairs against `b9705970a208f0fc83ee676dc667669d715e3776` measure legacy
528.6 → 539.8 µs (p = 0.583) and compact 631.5 → 656.2 µs (p = 0.221), with
unchanged bytes and allocations. Both increases are inconclusive and below the
10% ratchet; this establishes no speed win. Go's smaller native publication
frame and YAML's snapshot/checkpoint reuse experiments were also inconclusive
and are discarded. The [scoped receipt](runtime-tsx-conflict-order-20261012.md)
records counters, timings, source identity, limits and raw evidence.

### Owned YAML early selection: approved measured trade

Baseline: `624b2a60c8f402f6c2adb24846b192a547a918ce`. This proposal starts the
existing bounded child-selection proof before link pressure only when the
**diagnostic external-lexer ownership probe** is active. Every terminal must
still authenticate its exact external scanner transition. Ordinary activation,
recovery, caps, the allowlist and the global default keep their current behavior.
The owner approved the two diagnostic node-work increases below on 2026-10-12.
The committed change uses the reviewed predicate and work guard.

Twenty shuffled, alternating baseline/candidate pairs on CPU 6, in a 6 GiB
container with `GOMAXPROCS=1`, `-count=1`, `750ms` and `-benchmem`, measured the
committed 1,998-byte YAML sample through `BenchmarkAdmissionR4OwnedExternalLexer`:

| Metric | Before | After | Change |
| --- | ---: | ---: | ---: |
| Median parse time | 667.2 µs | 487.5 µs | −26.93%, p < 0.001 |
| Median allocated bytes | 16,593 | 15,144 | −8.73%, p < 0.001 |
| Allocations | 172 | 141 | −31, p < 0.001 |
| Compact acceptance / fallback | 1 / 0 | 1 / 0 | Unchanged |
| Diagnostic candidate full new nodes | 482 | 484 | +2 (+0.41%) |
| Diagnostic candidate first-edit new nodes | 482 | 484 | +2 (+0.41%) |

The first edit still uses the established fresh-parse fallback. The last two
rows are the owner-approved measured trade under
[R3](v1-design.md#r3-counter-ledger-changes). Only the diagnostic 482-node guard
rises to 484; the ordinary ledger keeps every numerical ratchet.

The reviewed candidate passes core/phase-zero-A and focused root tests, all 206
separate 72-step invariant sessions, 618 locked-C fixture checks, the unchanged
strict census (159 PASS / 45 known FAIL; all 120 compact acceptances C-exact),
focused container races, the 335-pattern CI selection and six peak-budget
processes. The 1,049,572-byte, 524-document stream completes on compact and
matches locked C on all checked structural axes (SHA-256
`8d615e96c0efe207dd1bfc4f54f601580d61dd0afc8f051ac3aaa92a2b27e397`).
Both owned YAML 72-step sessions pass with zero no-edit allocations.
The large C-oracle process peaks at 157,004 KiB RSS (153.18 bytes/source byte),
under the 400-byte requirement, with no OOM or timeout.
All 412 ordinary numerical work/reuse ledger rows remain unchanged; only the
two owned node counters above change.

A broader experiment also resolved ordinary YAML ownership and passed the
original and four-document C oracles, but **failed the throughput ratchet**.
Across twenty pairs, ordinary candidate time rose **311.7 → 609.4 µs (+95.47%,
p < 0.001)** while fallback became compact acceptance. Allocations rose
**37 → 141**. Legacy control time was **202.4 → 220.3 µs**, statistically
inconclusive (p = 0.758). This activation is discarded. Passing ownership and
C correctness is necessary but does not justify this regression or graduation.

A Go prototype split sole fresh-reduction publication out of the cold output
handling. Its ledger and focused C/invariant checks passed, but twenty pairs
showed **1.213 → 1.201 ms**, an inconclusive change (p = 0.583), with unchanged
1,696 bytes and 25 allocations. That prototype is discarded too.

A second Go prototype combined owner and lineage publication under one
capability check while preserving write order, failure-stage diagnostics and
rollback. Core/phase-zero-A and focused root/C checks passed, all 412 ledger
rows were byte-identical, and Go's 72-step invariant session allocated nothing
on a no-edit reparse. Twenty pairs against `0a3bb30af0470526dd781d4a2657887ac05b011e`
measured compact **1.679 → 1.575 ms** (p = 0.862) and legacy control
**1.263 → 1.328 ms** (p = 0.659), with unchanged bytes and allocations.
This is inconclusive, so the API and driver prototype are discarded.
An initial prototype test incorrectly compared unused inline integer scratch
slots after rollback; published membership, ownership, rank and flags were
restored. Correcting that assertion preceded the successful gate and timing run.

Fresh CPU profiles on retained commit `0a3bb30a` used the fixed R4 sources,
one language per process, CPU 6 and a 6-second profiling benchmark. These
attribute work and are not before/after timing evidence. Go spends 27.75% of
sampled CPU time under reduction dispatch, 16.75% in its native reduction body,
21.44% under materialization and 6.55% under lineage persistence. Owned YAML
spends 18.84% under election, 14.79% under lexer requests and 13.42% under
materialization. Cumulative shares overlap and must not be added. The next
architectural investigations should test a proven linear reduction path for
Go and reduce election/request work for YAML while keeping exact lexer-state
and owner authentication. The profiles do not prove either change will win.

The retained change also completed twenty shuffled pairs of the standard Go
DFA trio against `624b2a60`, with the same Docker/CPU settings. Timing variation
was wide; these results establish no Go speed win. Every median stays inside
the 10% ratchet and every allocation count is unchanged:

| Ordinary Go benchmark | Before | After | Direction / p | Bytes before → after | Allocations |
| --- | ---: | ---: | --- | ---: | ---: |
| Full DFA | 5.990 ms | 6.110 ms | +2.00% / 0.904 | 1,168 → 1,167 | 6 → 6 |
| Single-byte edit DFA | 140.7 µs | 145.4 µs | +3.32% / 0.862 | 395 → 394.5 | 5 → 5 |
| No-edit DFA | 4.124 ns | 4.385 ns | +6.32% / 1.000 | 0 → 0 | 0 → 0 |

The ordinary YAML control also completed twenty alternating pairs against
`624b2a60`. Legacy time is **164.7 → 163.5 µs (-0.74%, p = 0.301)**;
candidate-attempt time is **222.6 → 227.4 µs (+2.16%, p = 0.620)**.
Both are inconclusive and within the timing ratchet. Legacy keeps 1,440 bytes /
16 allocations; the candidate attempt keeps 17,144 bytes / 37 allocations,
zero compact acceptances and one fallback per parse. This validates the retained
ordinary route separately from the discarded activation experiment.

Raw overlay sources, before/after logs, benchstat summaries and gate logs are in
`/home/draco/.local/state/gotreesitter-evidence/20261012-go-corridor-yaml-ownership`.
The exact YAML proposal uses `yaml-diagnostic-early-overlay.json` and
`yaml-diagnostic-early-{before,after}.txt`; the ordinary rejection uses
`yaml-ordinary-{before,after}.txt`. These are separate measurements and are not
claims about the shipped ordinary YAML route.

### Scanner-history correction and single-version follow-up

The broader CI run found a nested Go generic-call regression in the previous
reduction child-selection change. `Foo[int](Foo[int](a))` selected nested indexed
calls, while locked C selects nested type conversions. The new
`TestGoNestedGenericSelectionLockedC` checks both routes on every structural axis;
the existing nineteen-case generic-conflict gate remains unchanged.

Child selection now requires **every terminal to carry an exact external scanner
transition**. DFA-only and mixed terminal histories do not authenticate the owned
lexer requests needed by this proof and keep both alternatives. This tightens the
existing predicate without adding a language-name branch or changing a cap.
The nested Go case again falls back and matches C. Owned YAML's original sample,
four-document and sixteen-document witnesses keep C-correct compact acceptance.
Native regressions cover both wholly unowned and mixed histories and cursor
cleanup. The clean-checkout Solidity sequence also regains its pinned fallback.

All **412 numerical work/reuse rows** remain unchanged against `396f4cbcc5f641a06bb8f60c73c0f82cfdeaa523`.
Only C's fallback boundary diagnostic returns **(445,389) → (450,365)**, with the
same `9 > 8` limit failure and unchanged fixture identities and route. The canonical
ledger changes only that string; no numeric ratchet or exception is reset.

The corrected source passes **206 separate 72-step invariant sessions**,
**618 locked-C fixture checks**, the unchanged strict census (**159 PASS / 45
known FAIL**, all **120 compact acceptances** C-exact), core/phase-zero-A tests,
focused root and container races including the generic-conflict family, the exact
335-pattern CI selection, and six peak-budget processes. Owned YAML's full/edit
ledger remains byte-identical at 482 full-parse nodes. The full run stopped at the
strict-census step because its evidence manifest had not been copied; resuming
there completed the remaining checks. No complete root-suite certificate is claimed.

Raw correction evidence is in
`/home/draco/.local/state/gotreesitter-evidence/20261012-go-corridor-yaml-ownership`.
`final-source.json` pins 3,470 Go/module files (SHA-256
`0eaaa81688c0c928326a0c2085b15611836348bf369c6d6775ce409a42297dcd`).
`correction-full-wrapper.log`, `correction-resume-wrapper.log`,
`scanner-selection-wrapper.log` and `solidity-full-wrapper.log` distinguish the
full fixture sweeps from the focused regression checks. The first local Solidity
probe stopped at an existing ignored corpus directory; the clean owned checkout
preserved that user data and passed the complete Solidity fixture sequence.

### Reconverged YAML children and EOF acceptance

The diagnostic-owned YAML EOF witness now completes on compact at **8,012 bytes**
(four documents, SHA-256
`778d1dec01bef4da1cef26acff6b8012c9701a23af67bdb9511b419912f04400`).
The 32,048-byte sixteen-document witness also completes. Both match locked C on
all structural axes in `TestCompactYAMLDocumentStreamLockedC`, including points,
fields, extra and missing flags, propagated errors, and complete input coverage.
The same sources previously reached the unchanged eight-link limit at EOF.

The new internal helper implements a bounded, clean subset of C's reduction
child selection. It compares parents from one reduction batch at one identical
graph predecessor, after a fresh forest first exceeds live-link capacity. The
parents must share their production, precedence and span, and their ordered
terminal histories must match exactly, including external scanner transitions
and skipped-prefix proofs. C's raw child ordering chooses the survivor. Missing
nodes, recovery, opaque reused records, corrupt histories and exhausted
comparison budgets retain alternatives or fail closed. No language-name branch,
scanner ownership bypass, larger cap, allowlist entry, or default-route change is
introduced.

Keep the narrow selection active through the rest of that fresh session, then
clear its pressure flag on entry, success, failure, panic and reset. A diagnostic
probe shows why the lifetime matters: limiting selection to each pressured
operation left eight derivations at acceptance and repeatedly materialized public
trees. Continuing the same proven selection removes that acceptance election on
the four- and sixteen-document witnesses. The discarded per-operation version
was **18.69% slower** in twenty pairs and failed the ratchet despite reducing
allocation bytes; it is not the committed candidate.

All **412 rows / 206 languages** preserve every numerical work and reuse counter.
Only C's existing fallback diagnostic changes from boundary `(450,365)` to
`(445,389)`; both hit `9 > 8`, and C remains a fallback. The canonical ledger updates
only this diagnostic string; every numeric ratchet, fixture identity, route and
other field stays pinned. The original owned YAML
full/edit ledger is byte-identical, including **482 full-parse nodes**. A parser
reuse regression parses the pressured stream before this sample and verifies the
same node budget. Only the four approved Liquid exceptions applied to this earlier receipt.

| Route and phase | Tokens before → after | Nodes before → after | Maximum versions before → after | Reused bytes before → after | Splices before → after |
| --- | ---: | ---: | ---: | ---: | ---: |
| Go candidate full | 589 → 589 | 1,251 → 1,251 | 3 → 3 | 0 → 0 | 0 → 0 |
| Go candidate first edit | 374 → 374 | 1,553 → 1,553 | 8 → 8 | 1,784 → 1,784 | 266 → 266 |
| YAML default full | 107 → 107 | 288 → 288 | 1 → 1 | 0 → 0 | 0 → 0 |
| YAML diagnostic owned full | 102 → 102 | 482 → 482 | 2 → 2 | 0 → 0 | 0 → 0 |

The final-source pipeline passes all 206 separate 72-step invariant sessions,
618 locked-C fixture checks, and the strict census of **159 PASS / 45 known FAIL**,
with **120 / 120 actual compact acceptances** matching C. Focused core and root
checks, phase-zero-A checks, scanner/recovery/reuse checks, container races, the
exact CI 335-pattern tagged selection, and six fresh-process peak-budget probes
pass. R6 stays 454 language-name uses / 101 environment reads; L0 stays 756 root
Go files / 12 API tag sets. No API or layout snapshots changed. This is not a
complete grammar-wide C-parity certificate.

The final comparison uses **twenty alternating, shuffled before/after pairs**
per workload, count=1, 750ms, benchmem and GOMAXPROCS=1 through the randomized
wrapper, in one serialized 6 GiB Docker container pinned to CPU 6. The stream
benchmark includes the baseline's failed compact attempt and legacy fallback;
it counts the actual route during timing. This is an end-to-end route comparison,
not a comparison with pure legacy or a C-relative speed certificate.

| Workload | Before ns/op | After ns/op | Time direction | p | B/op before → after | allocs/op before → after |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Go real file, compact | 1,115,522 | 1,084,187.5 | -2.81% | .108 | 1,696 → 1,696 | 25 → 25 |
| Go real file, legacy | 772,621.5 | 796,024.5 | +3.03% | .289 | 2,712 → 2,712 | 11 → 11 |
| Go generated full | 9,989,183 | 9,536,764 | -4.53% | .142 | 538,080 → 538,079 | 50 → 50 |
| Go one-byte edit | 119,996.5 | 111,630 | -6.97% | .429 | 401.5 → 401 | 5 → 5 |
| Go no edit | 3.393 | 3.623 | +6.78% | .127 | 0 → 0 | 0 → 0 |
| YAML original diagnostic owned | 709,581.5 | 705,275 | -0.61% | .620 | 16,592 → 16,592 | 172 → 172 |
| YAML ordinary attempt + legacy | 263,343.5 | 270,376 | +2.67% | .529 | 17,144 → 17,144 | 37 → 37 |
| YAML legacy | 191,798 | 199,056 | +3.78% | .678 | 1,440 → 1,440 | 16 → 16 |
| YAML 8 KiB diagnostic owned stream | 4,934,423 | 3,488,693 | -29.30% | < .001 | 961,540 → 53,081 | 479 → 460 |

Bucket the supported win as **compact EOF completion**, **29.30% lower end-to-end
time** (p < .001), **908,459 fewer allocated bytes (-94.48%)**, and **19 fewer
allocations (-3.97%)** on the four-document witness. Fallbacks change **1 → 0** per
parse and compact completions **0 → 1**. The original owned sample retains 172
allocations and its existing node budget. Ordinary YAML remains a fallback.

The direct-fill DFA snapshot and sole-pop native publication changes preserve
ownership, cursor state and provenance, but **no Go throughput improvement is
established**. Go's compact direction is -2.81% (p=.108); all allocation counts
are unchanged. Treat these as neutral architectural experiments. Record Go
no-edit time +6.78%, Go legacy time +3.03%, and ordinary YAML times +2.67% / +3.78%
as directional regressions; none is statistically supported. All measured 10%
time, bytes and allocation ratchets pass. Current compact/legacy ratios are
**1.362× Go / 1.358× ordinary YAML**; the Go gap remains open.

Twenty fresh-process Go pairs complete exactly 1 MiB on compact with no fallback.
RSS medians **316,336 → 318,630 KiB (+0.73%)** remain within the ratchet, with
ranges 299,584..345,568 / 293,836..342,824 KiB. Retained heap medians change
214,686,920 → 214,687,240 bytes; HeapInuse stays 216,801,280 bytes. The 400-bytes-per-
source-byte limit passes. These are not retention wins. Ordinary YAML's
1,049,572-byte probe retains complete, error-free fallback; RSS changes
62,304 → 63,080 KiB and retained heap 131,424,840 → 131,430,240 bytes. These small
increases are recorded, not claimed as improvements.

A separate **524-document / 1,049,572-byte owned YAML** probe now accepts on
compact, covers every byte, has no error, and matches locked C on every tested
axis (SHA-256
`8d615e96c0efe207dd1bfc4f54f601580d61dd0afc8f051ac3aaa92a2b27e397`).
The combined Go-and-C oracle process reaches **155,020.2 kB RSS**, below the hard
limit. Its 0.97-second wall time is a completion/memory receipt, not comparative
throughput evidence. No crash, OOM or timeout occurs in the final pipeline.

The final-source Go R4 profile attributes **44.91% cumulative CPU** to corridor
dispatch, **17.84%** to election and **21.19%** to final selection materialization.
These shares overlap and must not be added. The next larger targets are the
single-version dispatch/reduction path in #1319 and final publication. The profile
ranks targets; it does not establish a speedup or complete E-A7.

Ordinary YAML still declines at the no-action alternative-set coverage proof;
the accepted ownership path remains diagnostic. Go's throughput target, ordinary
scanner ownership, incremental scanner reuse, the indexed-header target, the
full E-A distribution and C-relative performance remain open. Neither language
graduates from these receipts.

Raw receipts and reproduction scripts are in
`/home/draco/.local/state/gotreesitter-evidence/20261012-yaml-eof-go-throughput`:
`final-source.json` (3,470 files; SHA-256
`5ce8987bb864a34378b93ac2256f83497cd0ef373fbd6898d593db2267cdbb55`),
`final-check.sh`, `final-measure.sh`, `final-perf.sh`, `rss-repeat.sh`,
`large-yaml.sh`, `guard-snapshot.py`, and their logs. Earlier rejected or exploratory
receipts have `p1-`, `p6-`, or `p8-` prefixes. The final correctness pipeline exits
zero without resuming. The timing runner's preflight refused an existing output
file before measuring; archived outputs and fresh paths preserve both versions. CI then caught the
canonical C diagnostic mismatch; a diagnostic-only follow-up and the exact
canonical ledger command address it. The first pushed head's grammar-lock job
failed before tests when the pinned CLI download returned HTTP 504; no oracle or
CLI pin changes are made.

### Scanner restore buffers and projection publication

This earlier receipt compares against `d22b66a1ffbd5c34608f7953deb263e282602619`.
The EOF frontier status and latest measurements appear above.

YAML checkpoint restoration reuses its private indentation buffers instead of
allocating root sentinels on every restore. Serialized checkpoints still own
independent bytes. Tests cover deep/shallow transitions, nil and short
checkpoints, incomplete trailing entries, signed state, root sentinels, input
mutation, and zero-allocation warm restores. Scanner binding and grammar blobs
remain unchanged.

Compact publication reconciles every compact ID that projects onto one public
node before granting any legacy reuse receipt. Replace its transient pointer
map with a temporary flag on the private arena's nodes. Any mismatched outer
projection still invalidates aligned inner projections. Borrowed nodes remain
untouched; cleanup runs on success, both polling failure phases, and panic.
The flag uses the last available uint16 bit without growing Node. A reservation
check prevents overlap with existing flags. Geometry, point, state and keyword
checks and polling intervals remain intact.

This also removes the map's conservative budget reservation, rather than
weakening a limit. A paired, cold-run bisection on the 2,047-byte Go R4 fixture
finds the minimum accepted configured budget changes **3,402,742 → 3,320,310
bytes**, freeing **82,432 bytes (2.42%)** of request headroom. Both runs create
1,283 compact records. The difference exactly matches the former
`(records + 1) * 64 + 256` reservation. This measures budget headroom, not a
reduction in retained heap or a throughput improvement. Arena memory remains
charged; the existing peak bounds pass in three fresh processes per witness.

All **412 ledger rows / 206 languages** are byte-identical to baseline
`d22b66a1ffbd5c34608f7953deb263e282602619`. The separate diagnostic-owned YAML
ledger also matches. No counters, exemptions, allowlists, defaults or caps reset.
Only the four approved Liquid exceptions applied to this earlier receipt.

| Route and phase | Tokens before → after | Nodes before → after | Maximum versions before → after | Reused bytes before → after | Splices before → after |
| --- | ---: | ---: | ---: | ---: | ---: |
| Go candidate full | 589 → 589 | 1,251 → 1,251 | 3 → 3 | 0 → 0 | 0 → 0 |
| Go candidate first edit | 374 → 374 | 1,553 → 1,553 | 8 → 8 | 1,784 → 1,784 | 266 → 266 |
| YAML default full | 107 → 107 | 288 → 288 | 1 → 1 | 0 → 0 | 0 → 0 |
| YAML diagnostic owned full | 102 → 102 | 482 → 482 | 2 → 2 | 0 → 0 | 0 → 0 |

All 206 separate 72-step invariant sessions pass, with zero no-edit allocations,
as do 618 locked-C fixture checks. The strict census stays **159 PASS / 45 known
FAIL**, with **120 / 120 actual compact acceptances** matching C. Focused core,
root, recovery, reuse, scanner and container race checks pass, including the
independent fused replay comparison and CI's exact 335-pattern tagged selection.
R6 remains 454 language-name uses / 101 environment reads; L0 remains 756 root
Go files / 12 API tag sets. No snapshots changed. No full root-suite or complete
C-parity certificate is claimed.

The final paired comparisons use one process per shuffle seed, 20 alternating
before/after seeds, GOMAXPROCS=1, count=1, 750ms and benchmem, through the
randomized wrapper in a serialized 6 GiB Docker container pinned to CPU 6.

| Workload | Before ns/op | After ns/op | Time direction | p | B/op before → after | allocs/op before → after |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Go real file, compact | 1,267,969.5 | 1,239,597 | -2.24% | .989 | 1,696 → 1,696 | 25 → 25 |
| Go real file, legacy | 958,091.5 | 888,462 | -7.27% | .565 | 2,712 → 2,712 | 11 → 11 |
| Go generated full | 11,887,901.5 | 11,793,093 | -0.80% | .698 | 538,082 → 538,083.5 | 50 → 50 |
| Go one-byte edit | 145,886 | 134,669 | -7.69% | .369 | 403 → 402 | 5 → 5 |
| Go no edit | 4.0325 | 3.9725 | -1.49% | .640 | 0 → 0 | 0 → 0 |
| YAML ordinary attempt + legacy | 411,254 | 391,356.5 | -4.84% | .820 | 17,144 → 17,144 | 39 → 37 |
| YAML legacy | 292,073.5 | 283,809 | -2.83% | .620 | 1,440 → 1,440 | 18 → 16 |
| YAML diagnostic owned | 879,086 | 864,327 | -1.68% | .301 | 18,065 → 16,593 | 366 → 172 |

**No throughput improvement is established.** Go's timing ranges are broad,
and its unchanged legacy control also moves. Bucket the supported wins as
**194 fewer owned-YAML allocations (-53.01%) / 1,472 fewer bytes (-8.15%)**, two
fewer ordinary-YAML allocations, and Go's budget headroom. Record the tiny Go
full-parse B/op increase and Go compact/legacy ratio increase as directional
regressions. All measured 10% ratchets pass. The earlier borrowed-pop-path
prototype had no supported timing or allocation gain and is discarded; its
noisy Go full/control increases are not retained-candidate evidence.

Twenty fresh-process Go pairs complete exactly 1 MiB on compact with no
fallback. RSS medians **330,576 → 330,586 KiB** are effectively unchanged, with
ranges 298,656..344,724 / 302,656..355,032 KiB. Retained heap medians change
214,686,832 → 214,686,984 bytes; HeapInuse stays 216,801,280 bytes. The RSS ratchet
and 400-bytes-per-source-byte limit pass. Ordinary YAML's 1,049,572-byte probe
keeps complete, error-free fallback; RSS stays 62,976 KiB and retained heap
changes 131,424,824 → 131,425,136 bytes. These are not retention wins.

The owned YAML EOF failure now has a bounded **four-document / 8,012-byte**
witness (SHA-256 `778d1dec01bef4da1cef26acff6b8012c9701a23af67bdb9511b419912f04400`).
At state 1507 and byte 8012, reductions combine four left alternatives with
multiple right alternatives; the ninth distinct live link reaches the unchanged
cap of eight. This is a live reduction batch, not a dead-boundary remap.
`TestCompactYAMLDocumentStreamLockedC` compares one owned document, four ordinary
documents, and four owned documents against C on every structural axis, counts
fallback explicitly, and permits a future C-correct compact completion. The
one-document owned parse must stay accepted. The four-document owned route
still falls back at the cap; the ordinary route still declines at no-action.
Neither ownership checks nor alternative-set coverage are relaxed.

Current ratios remain **1.395× Go / 1.379× ordinary YAML**; the owned route remains
diagnostic. E-A2's indexed-header target, ordinary YAML ownership, its live-link
frontier, incremental scanner reuse, the full E-A distribution and C-relative
performance remain unresolved. Neither language graduates.

Raw receipts and reproduction scripts are in
`/home/draco/.local/state/gotreesitter-evidence/20261012-yaml-route-go-hotpath`:
`final-source.json` (3,468 files; SHA-256
`63459ba1324b03d6fa8ce27f49b54e6d16a0119005d3438df9d1f7f9cdaa538b`),
`final-check.sh`, `final-resume.sh`, `final-remaining.sh`, `final-perf.sh`,
`final-memory.sh`, `final-bounds.sh`, `budget-probe.sh`, `performance-summary.json`
and the benchmark/census logs. The first full run stopped on test-line-only
census differences; moving the new regression to its own file restored the
byte-identical census. The next run stopped on container VCS stamping during CI
package discovery; buildvcs=false resolved that tooling failure. Remaining
correctness, memory, bounds and performance runs exit zero. These resumes do
not stand in for an uninterrupted successful full pipeline.

### Narrower headers and borrowed publication replay

Keep singleton alternative-set membership inline and store larger sets in the
existing exact, append-only arena. The set view narrows 16 → 12 bytes; graph
lineage records narrow 36 → 32 bytes; action-output records narrow 48 → 40 bytes.
Reorder the header's checkpoint beside
its head to fill alignment padding: scheduler headers narrow **96 → 88 bytes**.
Membership, sorted order, the 32-member ceiling, overflow conservatism and
rollback remain unchanged. The out-of-order spill regression also holds an
earlier inline snapshot alive and checks its exact membership after growth.
CI exposed an old 48-byte action-output size assertion. The corrected 40-byte
pin and CI's complete 335-pattern tagged-root selection pass in Docker.
Runtime code and performance measurements remain unchanged by this correction.
This is progress toward #1314, not completion of its 32-byte indexed-header
requirement; optional version state still uses the existing immutable wrappers.

Publication now borrows one scratch-owned replay view per transition. The
value-based traversal API remains available through an adapter. Every root
and child still receives the complete replay transition and metadata/ownership
validation. Frames carry the validated subtree ID and resolve records from the
unchanged arena; removing record pointers narrows frames **32 → 24 bytes** and
makes them pointer-free. Tests pin that layout, verify complete cleanup, and
compare production borrowed replay against independent top-down states on Go
code with extras and malformed input. Borrowed reuse transitions still reject
incompatible states. Aliases, table identity, scanner provenance, cancellation,
deadlines and memory limits retain their checks.

The pointer callback initially made a fallback local materializer escape to
the heap even when reusable scratch was supplied. Construct it only in the
branch that needs it. The discarded prototype added one allocation and 640
bytes to Go's real compact parse and failed the byte-allocation ratchet; those
results are not the retained candidate. The narrower records also exposed one
intermittent tiny-budget peak failure. Extend the cheap capacity-growth signal
to include cold lineage and membership arrays, which it previously omitted;
the complete footprint remains the authority. The existing peak bounds pass
in three fresh processes, and all final memory checks pass without relaxing
budgets or test expectations.

**Work and correctness bucket.** Against
`ef3ab2ba734609bc6d1a72cf28cc339e2875d502`, all **412 ledger rows for 206
languages** and the separate owned-YAML ledger remain byte-identical. No work
counter rises, no reuse counter falls, and no exception or reset is added:

| Route | Full node work | First edit node work | First edit reused bytes | First edit block splices |
|---|---:|---:|---:|---:|
| Go candidate | 1,251 → 1,251 | 1,553 → 1,553 | 1,784 → 1,784 | 266 → 266 |
| Go default | 1,922 → 1,922 | 3,810 → 3,810 | 1,166 → 1,166 | 296 → 296 |
| YAML default | 288 → 288 | 480 → 480 | 0 → 0 | 0 → 0 |
| YAML diagnostic owned candidate | 482 → 482 | 482 → 482 | 0 → 0 | 0 → 0 |

All 206 separate 72-step sessions and 618 C fixture checks pass, without
invariant skips and with zero no-edit allocations. The strict real-file census
is byte-identical: 159 PASS / 45 known FAIL, including 120 / 120 actual compact
acceptances. Full core, phase-zero-A, focused root/budget/replay checks, focused
container races, owned-YAML/recover-EOF and Swift C oracles, and the CI planner
pass. The supplemental Perl fixture still has 19 C differences on baseline and
candidate, with its eight reported differences identical; its existing reach
pin is unchanged. These scoped checks do not establish a complete root-suite
pass or eliminate the known correctness debt. The C4 proxy remains 37,662
lookups in each mode and 7,257 corridor passes when enabled.

**Throughput and allocation buckets.** The final source uses 20 alternating
paired shuffled seeds per set through `scripts/run_randomized_benchmarks.sh`,
one process per seed, `GOMAXPROCS=1`, `750ms`, `-count=1` and `-benchmem`,
serialized 6 GiB Docker pinned to CPU 6. Medians and benchstat results are:

| Workload | Before ns/op | After ns/op | Direction | p | B/op | allocs/op |
|---|---:|---:|---:|---:|---:|---:|
| Go real file, compact | 927,123 | 901,621 | -2.75% | .005 | 1,696 → 1,696 | 25 → 25 |
| Go real file, legacy | 682,347.5 | 673,922.5 | -1.23% | .020 | 2,712 → 2,712 | 11 → 11 |
| Go generated, full | 9,003,122 | 8,770,215 | -2.59% | .010 | 538,079.5 → 538,079 | 50 → 50 |
| Go generated, single-byte edit | 101,720.5 | 100,814 | -0.89% | .713 | 400 → 400 | 5 → 5 |
| Go generated, no edit | 3.057 | 3.0515 | -0.18% | .499 | 0 → 0 | 0 → 0 |
| YAML real file, declined + legacy | 208,920.5 | 210,596 | +0.80% | .192 | 17,528 → 17,144 | 39 → 39 |
| YAML real file, legacy | 153,506.5 | 154,703 | +0.78% | .341 | 1,440 → 1,440 | 18 → 18 |
| YAML diagnostic owned route | 609,427 | 599,500.5 | -1.63% | .012 | 18,065 → 18,065 | 366 → 366 |

The final Go compact/full and diagnostic owned-YAML timing improvements are
statistically supported in this set. Legacy Go also improves; these results
do not isolate each component's contribution. Incremental and ordinary-YAML
timing differences remain inconclusive. Record both positive ordinary-YAML
directions as regressions. All allocation counts stay unchanged. The ordinary
YAML declined attempt saves 384 allocated bytes (-2.19%). Accepted Go and
owned-YAML timing samples have zero fallback. Every measured ratchet passes.

Earlier isolated header-read/no-op-wrapper trials establish no allocation or
timing benefit and are discarded. The layout-only comparison also has unchanged
warm allocation counts and inconclusive timing: Go compact 1,084,656.5 →
1,054,213.5 ns/op (-2.81%, p=.414), Go legacy 863,662 → 810,081.5
(-6.20%, p=.242), owned YAML 650,568.5 → 659,251.5 (+1.33%, p=.989).
Retain the final combination for its measured throughput and smaller storage;
do not attribute the combined timing wins to the layout-only trial.

**Memory bucket.** Twenty alternating fresh-process pairs accept the exact
1 MiB Go input on compact without errors, truncation or fallback. Median
retained heap falls **218,570,032 → 214,687,168 bytes (-3,882,864, -1.78%)**;
heap-in-use falls 220,643,328 → 216,801,280 bytes. Median RSS falls
340,096 → 322,980 KiB (-5.03%), with overlapping ranges 315,136–358,920
and 302,660–349,380 KiB. The direction supports the storage result, but the
overlap does not establish an RSS effect by itself. The 10% RSS ratchet and
400-bytes-per-source-byte hard gate pass. Ordinary YAML's 1,049,572-byte probe
keeps its complete, error-free legacy tree and existing no-action decline;
single-process RSS rises 60,832 → 62,980 KiB. Record that +3.53% direction;
this single pair is not a randomized RSS ratchet.

**Graduation bucket.** Final compact/legacy medians remain **1.338× Go /
1.361× ordinary YAML**. Neither graduates. No all-206 timed distribution or
C-relative throughput certificate was produced. Owned YAML remains diagnostic;
its large-input EOF frontier failure (state 1507, 9 live links exceeding 8)
precedes publication and was not rerun. Existing edit fallbacks, the allowlist
and global default remain unchanged. #1314 and #1319 remain open.

R6 retains 454 language-name uses and 101 environment reads. L0 retains 756
root Go files and all 12 API tag sets, without snapshot changes. The frozen
Go/module manifest covers 3,465 files with SHA-256
`98c8dcd4abfa6bc4bc39027f5356410080081d486a29ee37be2da93ecf60490d`.
Raw receipts and reproduction scripts are in
`/home/draco/.local/state/gotreesitter-evidence/20261012-hot-header-replay`:
`layout-perf.sh`, `perl-oracle.sh`, `final-check.sh`, `final-perf-resume.sh`,
`perf.sh`, `rss-repeat.sh`, `analyze.py` and `guard-snapshot.py`. The completed
final correctness gates and resumed final performance run have no OOM or
timeout; the performance container exits 0. Failed/prototype receipts are
separate. All historical receipts and four owner-approved Liquid correctness
counter increases below remain in force.

### Reusable acceptance derivation enumeration

Fresh profiles put exact derivation enumeration at about 20% of allocated
objects on the diagnostic owned-YAML route. Reuse its temporary adjacency,
path, payload and cycle-check storage through an exclusive runner lease.
The implementation starts in `internal/parsercorephase0`; the existing copying
API remains available. Borrowed results expire after acceptance selection and
publication have copied their keys. Multiple recovered groups retain the
copying path. Scratch retains IDs and values, with no core, parser, source,
arena or public-tree references.

Keep every alternative and preserve payloads, forward score arithmetic,
insertion order, branch order, saturated-count validation, corruption errors
and cap error precedence. Narrow adjacency checks use a bounded scan; wider
frontiers retain the existing linear map validator. All backing capacities,
including inactive frame storage, enter scheduler memory accounting.
Enumeration polls stop control, and reset drops the complete workspace above
256 KiB. Ineligible runner attempts also drop the lease before accounting.
Warm sole-path and ambiguous-path unit probes allocate nothing. Generated
graph comparisons, cancellation, reentry and retention tests pass.

**Deterministic work bucket.** Against
`ef360102d322ac6f553529f74989c2b927621a06`, all 412 ledger rows across 206
languages remain byte-identical, including work, reuse, routing, coverage and
stop reasons. The separate owned-YAML ledger also matches. No ledger reset or
new counter exception is required. Selected before-and-after counters are:

| Route | Full node work | First edit node work | First edit reused bytes | First edit block splices |
|---|---:|---:|---:|---:|
| Go candidate | 1,251 → 1,251 | 1,553 → 1,553 | 1,784 → 1,784 | 266 → 266 |
| Go default | 1,922 → 1,922 | 3,810 → 3,810 | 1,166 → 1,166 | 296 → 296 |
| YAML default | 288 → 288 | 480 → 480 | 0 → 0 | 0 → 0 |
| YAML diagnostic owned candidate | 482 → 482 | 482 → 482 | 0 → 0 | 0 → 0 |

The C4 identity check again records 37,662 lookups in each mode and 7,257
corridor passes when enabled. These are implementation-specific proxies.

**Allocation and throughput buckets.** All comparisons use 20 paired,
alternating shuffled seeds through `scripts/run_randomized_benchmarks.sh`, one
process per seed, `GOMAXPROCS=1`, `750ms`, `-count=1` and `-benchmem`, in
serialized 6 GiB Docker pinned to CPU 6. The completed medians are:

| Workload | Before ns/op | After ns/op | Direction | p | B/op | allocs/op |
|---|---:|---:|---:|---:|---:|---:|
| Go real file, compact | 946,066 | 945,760.5 | -0.03% | .820 | 1,744 → 1,696 | 26 → 25 |
| Go real file, legacy | 691,650 | 700,786.5 | +1.32% | .009 | 2,712 → 2,712 | 11 → 11 |
| Go generated, full | 9,052,334 | 8,994,446.5 | -0.64% | .211 | 538,115.5 → 538,080 | 51 → 50 |
| Go generated, single-byte edit | 103,255 | 103,750.5 | +0.48% | .298 | 400 → 400 | 5 → 5 |
| Go generated, no edit | 3.2765 | 3.2815 | +0.15% | .410 | 0 → 0 | 0 → 0 |
| YAML real file, declined + legacy | 224,576 | 223,529.5 | -0.47% | .779 | 17,528 → 17,528 | 39 → 39 |
| YAML real file, legacy | 162,882.5 | 172,844 | +6.12% | .547 | 1,440 → 1,440 | 18 → 18 |
| YAML diagnostic owned route | 647,735.5 | 650,896.5 | +0.49% | .862 | 20,601 → 18,065 | 434 → 366 |

Retain this change for the owned-YAML savings of **68 allocations (-15.67%)
and 2,536 bytes (-12.31%)** per warm parse, plus one allocation (-3.85%) and
48 bytes (-2.75%) for Go's real compact file. Both accepted routes have zero
fallback in every timing sample. **No throughput improvement is established.**
Go legacy's +1.32% is statistically supported in this set; all other timing
differences are inconclusive. Record the Go edit/no-edit, owned YAML and
ordinary YAML legacy increases as directional regressions. Every measured
timing, allocation and RSS ratchet passes. The tradeoff is less allocation
churn with added reusable temporary storage, without a proven speed benefit.

**Memory bucket.** Twenty alternating fresh-process pairs accept the exact
1 MiB Go input on compact without errors, truncation or fallback. RSS medians
are 340,078 → 355,132 KiB (+4.43%); ranges are 310,124–359,852 and
320,256–360,096 KiB. Retained heap medians are 218,569,968 → 218,570,112
bytes; heap-in-use is 220,635,136 → 220,643,328 bytes. Record the RSS increase;
the overlapping ranges do not establish attribution. The RSS ratchet and
400-bytes-per-source-byte hard limit pass. The 1,049,572-byte ordinary YAML
probe keeps its complete, error-free legacy result and existing no-action
decline; single-process RSS is 60,812 → 60,864 KiB.

**Correctness bucket.** All 206 separate 72-step edit-session gates and 618 C
fixture checks pass, with no invariant skips and zero no-edit allocations.
The strict 204-file census remains byte-identical: 159 PASS / 45 known FAIL,
including 120 / 120 actual compact acceptances. Full tagged core tests,
focused acceptance, corridor, memory and stop-control checks, phase-zero-A
tests, owned-YAML/recover-EOF and Swift C oracles, focused container races and
the CI planner pass.

A broader supplemental root run exposed an existing failing assertion:
`TestAdmissionCandidatePerlExternalPayloadPinsLiveLinkCapReach` expects a
live-link-cap fallback, while both untouched baseline and candidate accept
its 3,479-byte fixture on compact. A supplemental locked-C oracle reports
19 structural differences on each, with the first eight reported differences
identical. The fixture SHA-256 is
`84b468672c82a73ba88d62a47591e85d02f9e35952a7ce45a494db71d1fa3ad4`.
This fixture is outside the 204-file R4 census. Its expectation is unchanged;
it is additional unfixed baseline correctness debt, not a passing C result.
The required scoped gates pass; a complete root-suite pass is not claimed.

**Graduation bucket.** Compact/legacy medians in this set remain **1.350× Go
/ 1.293× ordinary YAML**. Neither graduates, and the lower ratios do not
establish compact acceleration when legacy timing also rises. No all-206
timed distribution or C-relative throughput certificate was produced. Owned
YAML remains diagnostic; its existing large-input EOF frontier failure
(state 1507, 9 live links exceeding the cap of 8) precedes acceptance and was
not rerun. Go edit fallback, ordinary YAML's no-action decline and owned-YAML
edit fallback remain. No allowlist, default, proof, cap or milestone changes
are part of this receipt. #1319 remains open; the hot-header and publication
replay costs remain architectural targets.

R6 retains 454 language-name uses and 101 environment reads. L0 retains 756
root Go files and all 12 API tag sets without snapshot changes. The frozen
Go/module manifest covers 3,465 files with SHA-256
`f2c786244f6e03628232f8326c7aa4a1f6547a628a57b7e4b7d9ea1ede1ddfd7`.
Raw receipts and reproduction scripts are in
`/home/draco/.local/state/gotreesitter-evidence/20261011-runtime-cost-buckets`:
`profile.sh`, `perl-oracle.sh`, `final-check.sh`, `measure.sh`, `perf.sh`,
`rss-repeat.sh`, `analyze.py` and `guard-snapshot.py`. The final serialized
Docker pipeline exited 0 without OOM or timeout. Earlier receipts and the
four owner-approved Liquid correctness increases below remain in force.

### States-free acceptance alternatives

Temporary alternatives now use the existing states-free comparison traversal.
The retained reference still receives full parser-state replay and scanner
provenance. Every alternative still builds its own public tree and passes the
same complete public-tree comparator. That comparator excludes replay states;
alternatives cannot be published or reused. Table identity, metadata,
exclusive ownership, source tiling, cancellation, deadline and memory checks
remain active. Remove the unused replay-comparison purpose.

The alias regression now uses initialized parser tables and an immutable table
identity. It checks authoritative states through a retained alias, rejects a
different alias, compares independent publications, holds an earlier tree alive
across warm workspace reuse, checks cleared references, and exercises the
scheduler-to-publication transfer. Existing comparison tests still check every
attribute against an independent states-free trial and reject missing coverage.

Against `3fe73a53e5309032851f01bb73c545d1678f8999`, all 412 ledger rows across
206 languages remain byte-identical, including work, reuse, routing, coverage
and stop reasons. The separate owned-YAML ledger also matches. No new counter
exception or ledger reset is required. Selected before-and-after receipts are:

| Route | Full node work | First edit node work | First edit reused bytes | First edit block splices |
|---|---:|---:|---:|---:|
| Go candidate | 1,251 → 1,251 | 1,553 → 1,553 | 1,784 → 1,784 | 266 → 266 |
| Go default | 1,922 → 1,922 | 3,810 → 3,810 | 1,166 → 1,166 | 296 → 296 |
| YAML default | 288 → 288 | 480 → 480 | 0 → 0 | 0 → 0 |
| YAML diagnostic owned candidate | 482 → 482 | 482 → 482 | 0 → 0 | 0 → 0 |

Before timing, separate `gts_workcount` processes measure the same warmed real
files on the candidate route. Owned-YAML's table-lookup proxy falls
**1,872 → 998 (-46.69%)**; its leaf/parent construction proxies remain
420 → 420 / 748 → 748. Go's lookup proxy remains 3,688 → 3,688, with
588 → 588 / 567 → 567 constructions. All other recorded proxy fields are
unchanged. These implementation-specific proxies are not C work comparisons.
The separate C4 identity check still records 37,662 → 37,662 lookups with
7,257 corridor passes.

The candidate passes all 206 separate 72-step edit-session gates and 618 C
fixture checks, with no invariant skips and zero no-edit allocations. The
strict 204-file census is byte-identical: 159 PASS / 45 known FAIL, including
120 / 120 actual compact acceptances. Full core tests, focused root checks,
owned-YAML/recover-EOF and Swift C oracles, focused container races and the CI
planner pass. Baseline fleet evidence at the same immutable commit remains
valid from the preceding corridor experiments.

All timing sets use 20 paired alternating shuffled seeds through
`scripts/run_randomized_benchmarks.sh`, one process per seed, `GOMAXPROCS=1`,
`750ms`, `-count=1` and `-benchmem`, in serialized 6 GiB Docker pinned to CPU 6.
The completed medians are:

| Workload | Before ns/op | After ns/op | Direction | p | B/op | allocs/op |
|---|---:|---:|---:|---:|---:|---:|
| Go real file, compact | 1,221,724 | 1,210,805 | -0.89% | .512 | 1,744 → 1,744 | 26 → 26 |
| Go real file, legacy | 896,872 | 890,329.5 | -0.73% | .478 | 2,712 → 2,712 | 11 → 11 |
| Go generated, full | 11,869,043 | 11,364,790.5 | -4.25% | .414 | 538,119 → 538,119 | 51 → 51 |
| Go generated, single-byte edit | 133,024 | 129,666 | -2.52% | .718 | 403 → 402 | 5 → 5 |
| Go generated, no edit | 4.176 | 4.1165 | -1.42% | .733 | 0 → 0 | 0 → 0 |
| YAML real file, declined + legacy | 281,164 | 279,935 | -0.44% | .698 | 17,528 → 17,528 | 39 → 39 |
| YAML real file, legacy | 206,302 | 208,855 | +1.24% | .547 | 1,440 → 1,440 | 18 → 18 |
| YAML diagnostic owned route | 848,461.5 | 842,930.5 | -0.65% | .820 | 20,649 → 20,601 | 437 → 434 |

Owned YAML saves three allocations (-0.69%) and 48 bytes per parse (-0.23%),
with zero fallback in every sample. Every timing difference is inconclusive;
**no throughput improvement is established**. Record the ordinary YAML legacy
+1.24% direction as a regression. Every measured timing, allocation and RSS
ratchet passes. Retain this smaller change because it removes duplicate work
and allocations while simplifying the comparison modes; the earlier direct
reduction and childless-traversal prototypes had no reliable throughput gain
and were discarded.

Compact/legacy medians remain **1.360× Go / 1.340× ordinary YAML**. Neither
graduates. No all-206 timed distribution or C-relative throughput certificate
was produced. Owned YAML remains diagnostic. Its existing large-input EOF
frontier failure (state 1507, 9 live links exceeding the cap of 8) occurs before
acceptance materialization and was not rerun. Go's existing edit fallback,
ordinary YAML's no-action decline and owned-YAML's edit fallback remain intact.
The allowlist and global default remain unchanged; #1319 stays open.

Twenty alternating fresh-process pairs accept the exact 1 MiB Go input on
compact without fallback, errors or truncation. RSS medians are 344,734 →
337,392 KiB; ranges are 305,996–356,876 and 306,188–356,412 KiB. Retained heap
medians are 218,569,568 → 218,569,816 bytes; heap-in-use remains 220,635,136
bytes. The overlapping ranges do not prove an RSS improvement. The RSS ratchet
and 400-bytes-per-source-byte hard limit pass. The 1,049,572-byte ordinary YAML
probe retains its complete, error-free legacy result and unchanged decline;
single-process RSS is 60,812 → 60,812 KiB.

R6 retains 454 language-name uses and 101 environment reads. L0 retains 756
root Go files and all 12 API tag sets without snapshot changes. The frozen
Go/module pin covers 3,463 files with SHA-256
`4399f8496176e0348f13226831f618db6af667540451b2b3a21fb48137ce9a01`.
Raw receipts and reproduction scripts are in
`/home/draco/.local/state/gotreesitter-evidence/20261011-comparison-replay`:
`final-check.sh`, `work.sh`, `compare-work.py`, `measure.sh`, `perf.sh`,
`rss-repeat.sh` and `analyze.py`. Baseline and discarded-prototype receipts
remain in `20261011-corridor-session`. The final Docker run exited 0 without
OOM or timeout. Earlier receipts and the four owner-approved Liquid correctness
increases below remain in force.

### Cleared acceptance-comparison workspace

The next optimization retains the secondary construction workspace across
exclusive runner leases. Every alternative still builds an independent public
tree and passes the complete existing comparator. The workspace stores cleared
construction buffers; it retains no public tree, parser, source, core, or arena.
This extends the allocation work toward #1319 without changing its completion
criterion or certifying YAML ownership for production routing.

The entire secondary workspace has a 256 KiB retention limit, including its
backing-array capacities. Oversized comparisons remain permitted within the
parser budget, but their storage leaves the lease after use. Scheduler polling
charges the complete workspace alongside both live tree arenas. Coverage and
checkpoint buffers are charged once. An ineligible attempt drops the workspace
before starting a scheduler without its accounting hook. Memory-budget,
cancellation, deadline, acceptance and ownership checks remain active.

New regressions keep an earlier publication alive across three warm comparisons,
check exact metadata and separate nodes, inspect cleared references, enforce the
retention limit, and exercise the exact budget boundary. CI now selects these
acceptance/materialization checks. The [Swift contribution review](swift-directive-contribution-review.md)
incorporates PR #1442's credited regressions and 420 exact C comparisons, keeps
the current scanner, and adds the Swift package and oracle to CI execution.
Current main is integrated at `4f6c2429b`.

The baseline is `481d8cfbc27f16030aaf324b81d75ed5725d2284`. Before and after
correctness checks run in bounded Docker, one language per process. All 206
72-step edit-session gates and 618 C fixture checks pass on both builds. The
strict 204-file census is byte-identical: 159 PASS / 45 known FAIL, including
120 / 120 actual compact acceptances. The owned-YAML and recover-EOF oracles,
Swift oracle with the CI tags, full core package, focused root checks and
container races pass. Every no-edit invariant remains allocation-free.

All 412 ledger rows across 206 languages are byte-identical. The separate
owned-YAML ledger and 72-step invariant session also match. No new counter
exception or ledger reset is required. Selected before-and-after node-work
receipts are:

| Route | Full node work | First edit node work | First edit reused bytes | First edit block splices |
|---|---:|---:|---:|---:|
| Go candidate | 1,251 → 1,251 | 1,553 → 1,553 | 1,784 → 1,784 | 266 → 266 |
| Go default | 1,922 → 1,922 | 3,810 → 3,810 | 1,166 → 1,166 | 296 → 296 |
| YAML default | 288 → 288 | 480 → 480 | 0 → 0 | 0 → 0 |
| YAML diagnostic owned candidate | 482 → 482 | 482 → 482 | 0 → 0 | 0 → 0 |

The ordinary YAML candidate retains its existing no-action decline. The Go
candidate edit and diagnostic owned-YAML edit retain their existing fallback
paths. Every other work, reuse, routing and completeness field is unchanged.

Each timing set uses 20 paired alternating shuffled seeds through
`scripts/run_randomized_benchmarks.sh`, one process per seed, `GOMAXPROCS=1`,
`750ms`, `-count=1` and `-benchmem`, in serialized 6 GiB Docker pinned to CPU 6.
The completed medians are:

| Workload | Before ns/op | After ns/op | Direction | p | B/op | allocs/op |
|---|---:|---:|---:|---:|---:|---:|
| Go real file, compact | 1,033,959 | 1,034,136 | +0.02% | .620 | 1,744 → 1,744 | 26 → 26 |
| Go real file, legacy | 742,760 | 745,146.5 | +0.32% | .640 | 2,712 → 2,712 | 11 → 11 |
| Go generated, full | 9,795,102.5 | 9,845,628.5 | +0.52% | .583 | 538,117 → 538,116 | 51 → 51 |
| Go generated, single-byte edit | 114,140.5 | 113,117.5 | -0.90% | .947 | 401 → 401 | 5 → 5 |
| Go generated, no edit | 3.56 | 3.5305 | -0.83% | .857 | 0 → 0 | 0 → 0 |
| YAML real file, declined + legacy | 226,627 | 224,687.5 | -0.86% | .265 | 17,528 → 17,528 | 39 → 39 |
| YAML real file, legacy | 165,856.5 | 165,832.5 | -0.01% | .398 | 1,440 → 1,440 | 18 → 18 |
| YAML diagnostic owned route | 749,816.5 | 729,865.5 | -2.66% | .398 | 30,089 → 20,649 | 461 → 437 |

Owned-YAML allocation volume falls 31.37%, with 24 fewer allocations per parse
(-5.21%). Every sample completes on compact with zero fallback. All timing
differences are inconclusive, including its -2.66% time direction. Record the
Go compact (+0.02%), legacy (+0.32%), and generated full (+0.52%) directions as
regressions; none is statistically supported. Every measured 10% ratchet passes.
No Go or ordinary-YAML allocation-count improvement is claimed.

The compact/legacy medians remain 1.388× for Go and 1.355× for ordinary YAML.
Both still block graduation. Diagnostic owned-YAML activation is not production
certification; the large owned-YAML EOF frontier failure remains unresolved.
This change executes after that boundary, so the costly failing diagnostic was
not rerun. The existing allowlist and default route remain unchanged.

Twenty alternating fresh-process pairs accept the exact 1 MiB Go input on
compact without fallback, errors or truncation. RSS medians are 348,784 →
332,476 KiB; ranges are 319,436–356,848 and 305,508–357,036 KiB. Retained heap
medians are 218,569,712 → 218,569,624 bytes, with heap-in-use unchanged at
220,635,136 bytes. The RSS ratchet and 400-bytes-per-source-byte hard limit pass.
The overlapping ranges do not prove an RSS improvement. The ordinary
1,049,572-byte YAML probe keeps its existing decline and complete legacy result;
single-process RSS is 61,004 → 60,812 KiB. That is a safety probe, not a timing
or memory-improvement claim.

R6 retains 454 language-name uses and 101 environment reads. L0 retains 756
root Go files and all 12 API tag sets without snapshot changes. The Go/module
source pin covers 3,463 files and has SHA-256
`126a9228a8fa0083bb73c1cc07903e3acd453fdacbc4b919533e1187f455b2bb`.
The initial correctness run's CI inventory command needed `-buildvcs=false`
because the container cannot read the host worktree's Git metadata. All test
commands had passed; the corrected inventory and resumed performance run exit
0 without OOM or timeout. The final Swift check under CI tags also exits 0.

Exact source hashes, counter ledgers, full per-language logs, paired timings,
`benchstat` output, fresh-process RSS pairs and runnable gate scripts are under
`/home/draco/.local/state/gotreesitter-evidence/20261011-acceptance-workspace`.
Use `perf.sh` and `rss-repeat.sh` there inside the same bounded parity container,
mounting the baseline as `/baseline` and the receipt directory as `/evidence`.
Earlier receipts below retain their original baselines and claims.

### Retained acceptance publication

An eligible clean fresh parse now retains the selected publication while the
tied-acceptance gate checks the alternatives. After all comparisons pass, the
caller transfers that tree instead of constructing the selected tree again.
The reference includes normal replay and scanner provenance. Alternative trees
use the publication traversal, remain private, and omit incremental checkpoint
copies. Each owns a separate arena; the full public-tree comparator remains.

The one-shot internal owner binds the Core pointer, reset and authentication
generations, accepted head, copied payload sequence, and replay context. The
caller also checks language identity and source bytes. The fixed eight-payload
key uses no per-parse allocation; wider selections use the original path.
Recovery, borrowed reuse, and eager materialization retain their existing paths.
No acceptance, graph, memory, or dispatch cap changes.

Publication rechecks cancellation and the deadline without traversal throttling,
then charges the transferred arena and retained scratch to the normal memory
check. The regressions reproduce cancellation, timeout, and budget changes after
retention, source changes before publication, stale generations, failed
alternatives, and the exact scheduler-to-publication transfer. Failed checks
release the tree. Comparison memory also charges the live reference arena.

The final source passes the full core package, focused root checks, 206 separate
72-step edit sessions, 618 locked-C fixture checks, and focused container race
checks. The strict 204-file census remains 159 PASS / 45 known FAIL, including
120 / 120 actual compact acceptances. The owned-YAML and recover-EOF C oracles
pass. All 412 ledger rows and the separate owned-YAML before/after receipts are
byte-identical to baseline `0c9e53580e401a95152eaa7ac60b58bf23574a25`. All
no-edit checks allocate zero. No ledger exception or baseline reset is added.

The final owned-YAML timing uses 20 paired alternating shuffle seeds, one process
per seed, `GOMAXPROCS=1`, `750ms`, and `-benchmem`, in serialized 6 GiB Docker
pinned to CPU 6. It completes on compact with zero fallback in every sample:
1,219,860.5 → 1,180,129.5 ns/op (-3.26%, p=.040), 30,545 → 30,089 B/op,
and 469 → 461 allocations/op. This diagnostic activation is still uncertified
for default routing and is not a YAML graduation certificate.

The fused version-persistence experiment was dropped after its completed paired
benchmarks found no throughput improvement. An initial four-payload retention
bound excluded the five-payload YAML witness; those inactive-path timings were
discarded. The active prototype's initial gain was repeated on the final source
after restoring the publication stop-control poll. Only the final-source
measurements above and below are used as receipts.

The same protocol covers the five Go benchmarks and ordinary YAML, one
language per process. All timing sets contain exactly 20 samples per benchmark
and build. The completed final-source medians are:

| Workload | Before ns/op | After ns/op | Direction | p | B/op | allocs/op |
|---|---:|---:|---:|---:|---:|---:|
| Go real file, compact | 1,068,762 | 1,033,219.5 | -3.33% | .678 | 1,744 → 1,744 | 26 → 26 |
| Go real file, legacy | 757,811.5 | 745,748 | -1.59% | .904 | 2,712 → 2,712 | 11 → 11 |
| Go generated, full | 9,860,028 | 10,375,834.5 | +5.23% | .659 | 538,115 → 538,115.5 | 51 → 51 |
| Go generated, single-byte edit | 110,566 | 115,703.5 | +4.65% | .758 | 401 → 401 | 5 → 5 |
| Go generated, no edit | 3.5985 | 3.478 | -3.35% | .180 | 0 → 0 | 0 → 0 |
| YAML real file, declined + legacy | 216,679.5 | 221,221 | +2.10% | .003 | 17,528 → 17,528 | 39 → 39 |
| YAML real file, legacy | 163,205.5 | 162,842.5 | -0.22% | .369 | 1,440 → 1,440 | 18 → 18 |
| YAML diagnostic owned route | 1,219,860.5 | 1,180,129.5 | -3.26% | .040 | 30,545 → 30,089 | 469 → 461 |

The ordinary YAML attempt-plus-fallback regression is statistically supported
(+2.10%, p=.003). The owned-route improvement is also supported (-3.26%,
p=.040). Both remain within the timing ratchet. Every other timing difference
is inconclusive. The Go generated full-parse (+5.23%) and edit (+4.65%) directions
are recorded as regressions, with unchanged allocation counts. Go no-edit
remains allocation-free. This change saves tied-acceptance construction; it
does not demonstrate a Go throughput win.

The final compact/legacy medians remain 1.385× for Go and 1.358× for ordinary
YAML, including its unchanged decline and fallback. Both block graduation.
The owned-YAML timing is diagnostic and does not change that judgment.

Twenty alternating fresh-process pairs on the exact 1 MiB Go source measure
RSS medians of 346,322 → 331,176 KiB. The observed ranges are 319,696–356,932
and 307,400–359,088 KiB. Retained heap medians are 218,463,800 → 218,463,632
bytes, with heap-in-use unchanged at 220,512,256 bytes. The RSS ratchet and
400-bytes-per-source-byte hard limit pass; the overlapping ranges do not prove
an RSS improvement. Every process accepts on compact without fallback and
covers all 1,048,576 bytes without errors.

The 1,049,572-byte ordinary YAML memory probe retains its existing no-action
decline and complete legacy result. Its single-process RSS values are
59,404 → 60,196 KiB. The earlier large owned-YAML EOF live-link failure remains
a blocker: this acceptance optimization runs after that frontier boundary and
does not alter its caps. That costly diagnostic was not rerun.

R6 retains 454 language-name uses and 101 environment reads. L0 retains all
756 root Go files and checks 12 API tag sets; its snapshots now record the
private retention hook in the existing diagnostic-options struct. No callable
API is added. The source pin covers 3,444 Go/module files and has SHA-256
`d8f224b92c3e86106e8d0e672ea2c2f35a852c0b8bd686e8b8be76617f6f771c`.
The complete bounded Docker run exits 0 without OOM or timeout.

Local receipts are under
`/home/draco/.local/state/gotreesitter-evidence/20261011-acceptance-publication-final`.
The baseline checkout is pinned to `0c9e53580e401a95152eaa7ac60b58bf23574a25`.
Reproduce each timing set with `scripts/run_randomized_benchmarks.sh`, using
`--baseline-root`, `--baseline-output`, `--runs 20`, and `--benchtime 750ms`,
inside the same bounded parity container. Select one language with
`GTS_ADMISSION_REAL_CORPUS_LANGS`, set `GTS_ADMISSION_CANDIDATE=1`, and use the
benchmark names in the table. For the Go memory pairs, build the unmodified
probe from `scripts/compact_runtime_memory_probe.sh` once per checkout and run
`1 go 24902` in 20 fresh alternating process pairs under `/usr/bin/time -v`.
The allowlist, default, existing C-failure census, and ledger baselines stay
unchanged.

### Acceptance-trial materialization

Tied acceptance still materializes and compares every distinct candidate,
within the existing eight-candidate cap. The candidates now share transient
construction buffers: traversal frames, node lookup tables, child-building
scratch, and source-point storage. Each tree owns a separate arena. Clearing
the buffers between candidates cannot mutate the reference tree.

Comparison trees preserve the original nil-scratch trials' traversal without
parse-state replay. The public-tree comparator excludes those stamps. The
caller's replay setting still controls the table-identity precondition, and
publication keeps its replay and provenance checks. Temporary trees also omit
scanner checkpoint copies and remain ineligible for incremental reuse. Source
coverage, ownership, metadata, aliases, fields, missing nodes, and error checks
remain active. The regression compares all node attributes with an independent
original-path trial and checks that publication retains distinct replay
metadata. YAML's oracle stores no checkpoint records, so that workload measures
construction overhead rather than checkpoint-copy savings.

Raw derivation equality was insufficient: YAML alternatives can differ in
hidden productions and aliases while publishing identical trees. The full
public-tree comparison remains. A per-materialization replay cache was also
discarded: its completed 20-seed Go experiment added eight bytes per parse and
1,536 bytes of retained storage without a measurable throughput improvement
(`p = .820`). The final implementation contains no replay cache.

#### Acceptance correctness and counters

The final source passes the complete core package, focused materialization
tests, 206 separate 72-step edit sessions with zero no-edit allocations, and
618 locked-C fixture checks. The strict 204-file C census retains 159 passes
and 45 known failures; all 120 actual compact acceptances pass. Both diagnostic
YAML C oracles pass. The owned-YAML before/after builds also pass their separate
72-step sessions. Focused materialization and runtime-pool race checks pass.

All 412 ordinary counter rows are byte-identical to baseline
`7a1a14dde2a3e73370d5f031c2e17c080feaacb9`. The strict ledger passes without a
reset; only the four approved Liquid exceptions applied at that stage. The
owned-YAML counter receipts are also byte-identical:

| Route and phase | Tokens before → after | Nodes before → after | Maximum versions before → after | Reused bytes before → after | Splices before → after |
| --- | ---: | ---: | ---: | ---: | ---: |
| Go candidate full | 589 → 589 | 1,251 → 1,251 | 3 → 3 | 0 → 0 | 0 → 0 |
| Go candidate first edit | 374 → 374 | 1,553 → 1,553 | 8 → 8 | 1,784 → 1,784 | 266 → 266 |
| Go default full | 589 → 589 | 1,922 → 1,922 | 8 → 8 | 0 → 0 | 0 → 0 |
| Go default first edit | 572 → 572 | 3,810 → 3,810 | 12 → 12 | 1,166 → 1,166 | 296 → 296 |
| YAML default full | 107 → 107 | 288 → 288 | 1 → 1 | 0 → 0 | 0 → 0 |
| YAML default first edit | 107 → 107 | 480 → 480 | 1 → 1 | 0 → 0 | 0 → 0 |
| YAML diagnostic owned full | 102 → 102 | 482 → 482 | 2 → 2 | 0 → 0 | 0 → 0 |
| YAML diagnostic candidate first edit | 102 → 102 | 482 → 482 | 0 → 0 | 0 → 0 |

Go's first edit falls back with the existing shared-lexer-version decline.
The diagnostic YAML edit remains `candidate_incremental_not_routed`.
Ordinary YAML retains its existing no-action decline. These receipts do not
certify new incremental routes.

Clean-checkout static checks pass with 454 language-name uses, 101 environment
reads, 756 root Go files, and 12 public-API tag sets. The 3,442 Go/module source
hashes remain fixed through final validation and measurement.

Fresh-process 1 MiB probes complete with full coverage and error-free roots.
Go uses compact without fallback; ordinary YAML uses its unchanged fallback.
Peak RSS is 324,800 → 339,780 KiB for Go and 60,316 → 60,320.2 kB for YAML.
Retained heap is 218,463,960 → 218,464,752 bytes and
131,319,672 → 131,319,664 bytes, respectively. These are single-process
completion and memory probes, not 20-seed RSS or timing ratchets. The earlier
large owned-YAML EOF live-link failure remains a graduation blocker.

#### Acceptance performance receipt

All comparisons below use clean baseline `7a1a14dde2a3e73370d5f031c2e17c080feaacb9`.
Each workload uses 20 paired alternating shuffled seeds, one process per seed,
`GOMAXPROCS=1`, one CPU, `-count=1`, 750 ms, and `-benchmem`, through
`scripts/run_randomized_benchmarks.sh` in serialized 6 GiB Docker. Go, ordinary
YAML, and diagnostic owned YAML run separately. Both owned builds complete
every timed parse on compact with zero fallbacks.

| Workload | Before ns/op | After ns/op | Change | p | B/op before → after | Allocs/op before → after |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Go real file, compact | 1,157,333 | 1,146,933.5 | -0.90% | .478 | 1,744 → 1,744 | 26 → 26 |
| Go real file, legacy | 826,405.5 | 807,727 | -2.26% | .925 | 2,712 → 2,712 | 11 → 11 |
| Go generated, full | 11,151,275.5 | 10,949,298 | -1.81% | .659 | 538,118.5 → 538,117 | 51 → 51 |
| Go generated, single-byte edit | 124,823.5 | 125,344 | +0.42% | .738 | 401.5 → 402 | 5 → 5 |
| Go generated, no edit | 3.8065 | 3.8425 | +0.95% | .602 | 0 → 0 | 0 → 0 |
| YAML real file, declined + legacy | 248,070.5 | 252,667.5 | +1.85% | .327 | 17,528 → 17,528 | 39 → 39 |
| YAML real file, legacy | 183,018 | 189,110.5 | +3.33% | .192 | 1,440 → 1,440 | 18 → 18 |
| YAML diagnostic owned full | 820,959 | 779,872 | -5.00% | .127 | 55,282 → 30,545 | 549 → 469 |

Owned YAML allocates **44.75% fewer bytes** and performs **14.57% fewer
allocations**, saving 80 allocations per parse; both have `p < .001`.
Its first timing set is inconclusive. One additional independent set of
20 paired seeds (21–40) measures 817,061.5 → 784,209.5 ns/op (-4.02%,
`p = .327`). Combining the two sets gives 820,959 → 784,209.5 ns/op
(-4.48%, `p = .067`, 40 samples per build), with the same allocation reductions.
The throughput improvement remains unproven. No further sampling was added.
The replicated allocation savings concern the diagnostic owned route, not
ordinary YAML's declined route. Go's timing changes are also inconclusive.

Candidate/legacy ratios change **1.400 → 1.420** for Go and **1.355 → 1.336**
for ordinary YAML. The directional increases in Go's ratio, edit/no-edit time,
and YAML's route/control time remain in the receipt. Neither route clears
E-A's 1.15 ceiling, and YAML does not clear E-C's 1.10 declined-route ceiling.
This subset does not measure the all-206 timing distribution or Go/C speed.
No graduation allowlist, parser default, cap, or ledger baseline changes.

The final owned-YAML profile still attributes 37.88% cumulatively to
materialization, including 31.12% under acceptance vacuity; these percentages
overlap. A further win needs to avoid repeated public-tree construction while
proving exactly the same visible syntax, including hidden productions and
aliases. Raw forest-record equality did not supply that proof. Go's scheduler
accounts for 76.85% cumulatively, including 46.64% in corridor dispatch;
[the single-version path, #1319](https://github.com/odvcencio/gotreesitter/issues/1319),
remains a larger throughput target. Profiles are attribution, not timing
comparison evidence.

Raw samples, source hashes, counter receipts, profiles, and `benchstat` output
are under `/home/draco/.local/state/gotreesitter-evidence/20261011-acceptance-materialization`.
The rejected replay cache and raw-equality experiments have separate archives
there. The final candidate source pin contains 3,442 Go/module files.

Reproduce by mounting clean `7a1a14dde` at `/baseline`, this candidate at
`/workspace`, and an output directory at `/evidence` in the serialized Docker
harness (`--memory 6g --cpus 1 --gomemlimit 5GiB`). Run each language separately:

```sh
GOMAXPROCS=1 GTS_ADMISSION_CANDIDATE=1 GTS_ADMISSION_REAL_CORPUS_LANGS=yaml \
  bash scripts/run_randomized_benchmarks.sh \
  --output /evidence/yaml-after.txt --baseline-root /baseline \
  --baseline-output /evidence/yaml-before.txt \
  --bench-regex '^BenchmarkAdmissionR4(Legacy|Compact)$' \
  --require-benchmarks BenchmarkAdmissionR4Legacy,BenchmarkAdmissionR4Compact \
  --runs 20 --benchtime 750ms
```

For Go, select `go` and include the primary full/edit/no-edit trio alongside
the two R4 benchmarks, as recorded in `perf.sh`. For owned YAML, use
`^BenchmarkAdmissionR4OwnedExternalLexer$` and require that benchmark. Repeat
only that owned command with `--seed-start 21` and separate output files for
the independent replication. The benchmark enables and restores its diagnostic
probe explicitly.

### Preserve independent YAML lexer histories

The early-owned YAML prototype exposed a canonicalization error. At byte 517,
heads 94 and 95 had the same parser state, shifted state, and scanner checkpoint,
but different pending lexer requests. Boundary lookup remapped both versions
onto head 95. The unchanged ownership guard then rejected two scheduler owners
on that physical head.

Under the diagnostic probe, an owned remap needs a live version with matching immutable lexer history
to authenticate its destination. Incompatible condensation candidates remain
live for freshness and provenance checks, while their graph histories stay
separate. They cannot be treated as dead historical boundaries or merged into
one another. The new `Independent` bit fits existing padding; a condensation
candidate remains 32 bytes. Tests cover equal and unequal requests, unwitnessed
boundaries, separate owners, provenance, and rollback.

Continuing every newly separated history was too expensive for a route that
still falls back. The rejected variant raised Robot's complete route from
986,767 to 2,546,686 ns/op (+158.08%, `p < .001`) and from 59,482 to 281,262 B/op.
Independent-history expansion and owned canonicalization now stay behind the
diagnostic probe along with early ownership. Ordinary routes retain their
established canonicalization, condensation, and ownership checks. Tests prove
that the ordinary route still rejects conflicting owners before publication.
The existing Swift production-activation test retains its acceptance and exact
lexer counters. No ownership or alternative-set check is removed.

The diagnostic probe can continue with independent histories. It activates
early ownership only when external symbols exactly cover every non-EOF
terminal, before either clean pending version shifts. It defaults to false,
has no environment switch, and is unavailable in an ordinary build.
`TestCompactYAMLVersionOwnershipCOracle` requires actual compact acceptance
without fallback and checks the hash-locked 1,998-byte YAML sample against C
on every structural axis. The ordinary YAML route retains its existing
blended-survivor decline. The probe does not graduate YAML or change routing
allowlists.

Scanner validation now avoids repeated formatting and hashing of immutable
bindings. YAML gives its immutable language binding a stable pointer identity;
per-parse state remains in the scanner payload. Value bindings use typed binary
encoding that still detects changes to array and slice contents. Checkpoint
validation compares receipts and exact bytes against the interner instead of
rehashing every private copy. Request classification reuses validation already
completed in the same call. Transient before/after state uses existing bounded
scratch; published snapshots retain independent copies. Corruption, stale
generation, foreign destination, binding mutation, and pointer replacement
checks remain active.

### Ownership correctness and work counters

All **412 counter rows are byte-identical** to the measured `26dcc65c9` baseline,
including Robot's original decline. The checked-in ledger is unchanged, and its
strict 2% check passes. The four previously approved Liquid exceptions remain
the only exceptions. The diagnostic owned-YAML build and its correctness-only
baseline also produce byte-identical counter receipts:

| Route and phase | Tokens before → after | Nodes before → after | Maximum versions before → after | Reused bytes before → after | Splices before → after |
| --- | ---: | ---: | ---: | ---: | ---: |
| Go candidate full | 589 → 589 | 1,251 → 1,251 | 3 → 3 | 0 → 0 | 0 → 0 |
| Go candidate first edit | 374 → 374 | 1,553 → 1,553 | 8 → 8 | 1,784 → 1,784 | 266 → 266 |
| YAML default full | 107 → 107 | 288 → 288 | 1 → 1 | 0 → 0 | 0 → 0 |
| YAML default first edit | 107 → 107 | 480 → 480 | 1 → 1 | 0 → 0 | 0 → 0 |
| YAML diagnostic owned full | 102 → 102 | 482 → 482 | 2 → 2 | 0 → 0 | 0 → 0 |
| YAML diagnostic candidate first edit | 102 → 102 | 482 → 482 | 0 → 0 | 0 → 0 | 0 → 0 |

The ordinary YAML candidate still records its no-action decline instead of
full/edit work. The diagnostic candidate's edit receipt records
`candidate_incremental_not_routed`; this does not certify compact subtree reuse.
Owned full parsing still exceeds legacy's node and version counts, so its
correctness success alone cannot satisfy graduation.

All 206 separate 72-step edit sessions pass, with zero no-edit allocations,
as do 618 locked-C fixture checks. The complete core package and focused
canonicalization, ownership, checkpoint, snapshot, recovery, lifecycle, and
memory-budget checks pass. The strict 204-file C census retains the same
159 passes and 45 known failures; all 120 ordinary compact acceptances pass.
The separate diagnostic YAML oracle requires compact acceptance without
fallback. Existing failure expectations and graduation gates are unchanged.

### Scanner validation performance receipt

The shipped-route comparison uses clean baseline `26dcc65c9`. The diagnostic
comparison uses correctness-only commit `219ab216be310015ff2aa1c4e91bb74ec1944364`
with the ownership repair and probe,
but the original scanner binding, formatting-based identity checks, checkpoint
rehashing, and temporary snapshot allocation. Both diagnostic builds complete
on compact, match C, and have identical work counters. This comparison does
not measure a failing parser against a successful one.

Each workload uses 20 paired alternating shuffled seeds, one process per seed,
`GOMAXPROCS=1`, one CPU, `-count=1`, 750 ms, and `-benchmem`, through
`scripts/run_randomized_benchmarks.sh` in serialized 6 GiB Docker. All Go source
hashes remain fixed through the final correctness and timing runs. The Go run
includes the primary full/edit/no-edit trio. Robot is included because its
fallback cost exposed the rejected expansion policy.

| Workload | Before ns/op | After ns/op | Change | p | B/op before → after | Allocs/op before → after |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Go real file, compact | 1,384,970 | 1,408,631.5 | +1.71% | .925 | 2,032 → 1,744 | 28 → 26 |
| Go real file, legacy | 1,002,473 | 1,000,898 | -0.16% | .512 | 2,712 → 2,712 | 11 → 11 |
| Go generated, full | 13,243,647.5 | 12,654,164.5 | -4.45% | .620 | 538,412 → 538,120 | 53 → 51 |
| Go generated, single-byte edit | 148,020.5 | 152,182 | +2.81% | .738 | 404 → 404 | 5 → 5 |
| Go generated, no edit | 4.3885 | 4.382 | -0.15% | .512 | 0 → 0 | 0 → 0 |
| YAML real file, declined + legacy | 423,723.5 | 421,148 | -0.61% | .968 | 20,088 → 17,528 | 41 → 39 |
| YAML real file, legacy | 304,045.5 | 302,723 | -0.43% | .718 | 1,440 → 1,440 | 18 → 18 |
| Robot real file, declined + legacy | 749,987 | 677,136.5 | -9.71% | .023 | 59,482 → 59,482 | 132 → 132 |
| Robot real file, legacy | 425,358 | 428,830.5 | +0.82% | .578 | 1,144 → 1,144 | 6 → 6 |
| YAML diagnostic owned full | 15,127,465.5 | 1,140,997.5 | -92.46% | <.001 | 5,101,023 → 55,282 | 4,440 → 549 |

The completed diagnostic YAML path is **13.26 times faster**, allocates
**98.92% fewer bytes**, and performs **87.64% fewer allocations**; all three
comparisons have `p < .001`. Every timed parse reports one compact route and
zero fallbacks. This improvement is relative to the correctness-only owned
parser, not to legacy or today's shipped fallback route.

Go real-file allocation volume falls **14.17%** and YAML's complete shipped
route falls **12.74%**, each saving two allocations (`p < .001`). Their final
timing changes are inconclusive. Robot's complete route measures **9.71% faster**
without changing its allocation volume, allocation count, or decline. Its
legacy control rises 0.82%, an inconclusive change. The rejected Robot regression
is not present in the final code.

Candidate/legacy ratios change **1.382 → 1.407** for Go, **1.394 → 1.391** for
YAML, and **1.763 → 1.579** for Robot. Go's directional ratio regression is
retained alongside its allocation improvement. All three remain above E-A's
1.15 ceiling; YAML and Robot remain above E-C's 1.10 declined-route ceiling.
This subset does not establish the all-206 timing distribution or C-relative
speed. The owned route remains diagnostic-only, and graduation stays blocked.

Focused core and scheduler race checks pass. Both diagnostic-owned builds pass
the 72-step YAML edit session with zero no-edit allocations. Clean-checkout
`v1guard` and `v1layout` pass: 454 language-name uses, 101 environment reads,
756 root Go files, and 12 public-API tag sets remain within existing registries.

A fresh-process memory probe repeats complete YAML documents to **1,049,572
bytes** (SHA-256 `8d615e96c0efe207dd1bfc4f54f601580d61dd0afc8f051ac3aaa92a2b27e397`).
Both ordinary builds fall back, cover the input, and return an error-free tree.
Peak RSS is 60,120 → 60,316 KiB (+0.33%); retained heap is
131,318,712 → 131,317,744 bytes. Reserved heap is
154,894,336 → 142,311,424 bytes. Wall time is 0.24 → 0.30 seconds. These are
single-process completion and memory probes, not 20-seed timing or RSS ratchets.

The diagnostic owned build also returns a complete error-free tree, but only
after **61.50 seconds and legacy fallback**: at EOF it exceeds the existing
live-link cap, **9 > 8**, at state 1507 and byte 1,049,572. Peak RSS is
279,612 KiB (272.80 bytes per source byte), and retained heap is 139,243,944
bytes. There is no crash, OOM, or timeout. This result does not certify compact
completion or C parity on the large input. It is a concrete graduation blocker;
neither the cap nor the admission default changes.

On the small owned sample, the final CPU profile attributes 38.53% cumulatively
to materialization and root finalization, including 31.49% under
`compactAcceptanceElectionIsVacuous`. That points to answering acceptance
vacuity from authenticated compact records without constructing a temporary
public tree. Large-document ownership convergence and the EOF live-link limit
also need investigation. Profile attribution does not replace the timing gate.

Raw samples, source hashes, `benchstat` output, profiles, and counter receipts
are under `/home/draco/.local/state/gotreesitter-evidence/20261011-yaml-ownership`.
The rejected variants have separate source archives and receipts in
`rejected-unbounded-owner` and `rejected-canonical-only` below that directory.

Reproduce the ordinary comparison by mounting clean `26dcc65c9` at `/baseline`
and this candidate at `/workspace`, then running the following inside the
serialized Docker harness. Select Go, YAML, and Robot in separate invocations;
the Go run also includes the primary trio as recorded in `perf.sh`.

```sh
GOMAXPROCS=1 GTS_ADMISSION_CANDIDATE=1 GTS_ADMISSION_REAL_CORPUS_LANGS=yaml \
  bash scripts/run_randomized_benchmarks.sh \
  --output /evidence/yaml-after.txt --baseline-root /baseline \
  --baseline-output /evidence/yaml-before.txt \
  --bench-regex '^BenchmarkAdmissionR4(Legacy|Compact)$' \
  --require-benchmarks BenchmarkAdmissionR4Legacy,BenchmarkAdmissionR4Compact \
  --runs 20 --benchtime 750ms
```

For the owned comparison, mount **`219ab216b`** at `/baseline`, select YAML,
and use `^BenchmarkAdmissionR4OwnedExternalLexer$` for `--bench-regex` and
`BenchmarkAdmissionR4OwnedExternalLexer` for `--require-benchmarks`.
That benchmark explicitly enables and restores the diagnostic probe.

### Move cohort references out of hot scheduler records

This change advances [E-A2, #1314](https://github.com/odvcencio/gotreesitter/issues/1314).
The baseline Go profile spends 6.34% of sampled CPU in `runtime.duffcopy`,
including copies of scheduler headers, reduction outputs, and canonicalization
records. Every record previously carried two inline 32-byte cohort references,
even when neither was used. References now live in bounded core arena storage;
the hot records carry a 16-byte view with an index, count, flags, and identity.

| Record | Before bytes | After bytes |
| --- | ---: | ---: |
| Cohort reference set | 72 | 16 |
| Scheduler header | 152 | 96 |
| Reduction output | 112 | 56 |
| Condensation candidate | 88 | 32 |
| Canonical group | 104 | 48 |
| Action output | 104 | 48 |

Copies retain their exact reference membership. A sorted tail extension can
share its prefix, and publishing an existing immutable set needs no duplicate
storage. The final-slot identity rejects foreign cores, reset arenas, and
rolled-back suffixes whose indices have been reused. Identity exhaustion fails
closed. Stored references occupy 40 bytes, including identity; storage,
retention, shadow estimates, and preflight checks charge all 40 bytes against
the existing byte budgets. Budget defaults are not increased. Node records,
node lineage records, and compact subtrees retain their existing layouts.
Owner, checkpoint, alternative-set, and blended-survivor checks remain active.
The 96-byte header still exceeds E-A2's 32-byte target, so this does not close
that issue.

All 206 separate 72-step edit sessions and 618 locked-C fixture checks pass.
The strict 204-file C census is byte-identical to baseline: 159 passes, the
same 45 fallback/custom-source failures, and all 120 actual compact
acceptances passing. All 412 deterministic counter rows are byte-identical;
the strict 2% ledger passes without another exception. The complete core
package, focused scheduler/lifecycle/budget checks, and focused core and
scheduler race checks pass. New regressions cover immutable membership,
foreign/reset/rollback rejection, identity exhaustion, and byte-budget
atomicity. Existing warmed enumeration and union checks still allocate nothing.

Five generic-scheduler pinned snapshot tests fail on both this source and the
unchanged `3a76b2df4` baseline: `ClosesAtRequestedByte`, `CapsPublishNothing`,
`ContinuesToNextRequestedClosedByte`, `CrossesReductionFreshnessCycle`, and
`CrossesMultiHeadExtraCohort` (each prefixed
`TestDiagnosticParserCoreGenericScheduler`). Their expectations are unchanged;
the focused passes above do not claim that entire suite passes.
The separate Kotlin controls pass for the shipped decline, platform-modifier
split, and non-candidate admission. Its forced-certification object-declaration
test fails identically on baseline and candidate: it expects compact acceptance
but the blended-survivor check declines. That expectation is also unchanged.

During the reference-view work, an external YAML prototype gave each competing
version independent lexer
ownership before any sibling shifted. It passed the earlier blended-survivor
decline, then failed because two scheduler owners reached the same physical
compact head. That was not a valid acceptance, so it was excluded from that
change. YAML retained its original decline at `spec`, bytes 346–350. The next ownership
change needed to represent or separate these owners correctly through graph
canonicalization and rollback. Removing either ownership or alternative-set
checks would not establish C correctness. The ownership repair above supersedes
that rejected prototype; early activation remains diagnostic-only.

### Reference-view performance receipt

This comparison uses the clean `3a76b2df4` baseline and the final reference-view
implementation above. Go and YAML each use 20 paired alternating shuffled
seeds, one process per seed, `GOMAXPROCS=1`, one CPU, `-count=1`, 750 ms, and
`-benchmem` through `scripts/run_randomized_benchmarks.sh` in serialized 6 GiB
Docker. The Go run includes the primary full/edit/no-edit trio. Runtime and test
source hashes stayed unchanged throughout the gates and measurements.

| Workload | Before ns/op | After ns/op | Change | p | B/op before → after | Allocs/op before → after |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Go real file, compact | 1,322,317 | 1,281,251.5 | -3.11% | .265 | 2,032 → 2,032 | 28 → 28 |
| Go real file, legacy | 950,788 | 916,885 | -3.57% | .547 | 2,712 → 2,712 | 11 → 11 |
| Go generated, full | 12,143,879.5 | 12,280,755 | +1.13% | .779 | 538,412 → 538,412.5 | 53 → 53 |
| Go generated, single-byte edit | 136,160.5 | 139,444.5 | +2.41% | .925 | 403 → 403 | 5 → 5 |
| Go generated, no edit | 4.143 | 4.1905 | +1.15% | .495 | 0 → 0 | 0 → 0 |
| YAML real file, declined + legacy | 295,700 | 301,866 | +2.09% | .904 | 20,152 → 20,088 | 41 → 41 |
| YAML real file, legacy | 215,356 | 218,962.5 | +1.67% | .495 | 1,440 → 1,440 | 18 → 18 |

**This run does not demonstrate a Go or YAML throughput improvement.** All
timing differences are inconclusive. Go's legacy control improves slightly
more than compact; YAML's complete route rises slightly more than legacy.
Candidate/legacy ratios rise **1.391 → 1.397** for Go and **1.373 → 1.379**
for YAML. Both remain above the E-A ceiling of 1.15, and YAML remains above
E-C's 1.10 declined-route ceiling. YAML allocates 64 fewer bytes per operation
(-0.32%, `p < .001`); every allocation count is unchanged. This is progress on
E-A2's record layout, with no graduation, all-206 timing, or C-relative speed
claim. The allowlist and default remain unchanged.

The final 10-second Go attribution profile reports `runtime.duffcopy` at
5.40% of sampled CPU, versus 6.34% in the baseline profile. Election remains
17.74%, including 12.74% in token production. These separate profile runs
locate remaining work; their timings do not replace the randomized comparison.

Twenty alternating fresh-process pairs parse exactly 1,048,576 bytes of Go.
All 40 accept compact, cover the input, and finish without errors, fallback,
crash, OOM, or timeout. Median RSS falls **342,900 → 337,916 KiB (-1.45%)**;
maximum RSS falls **373,428 → 357,436 KiB (-4.28%)**. Candidate maximum RSS
is **349.06 bytes per source byte**, below the 400-byte ceiling on this probe.
Median retained heap falls **218,464,712 → 218,463,808 bytes (-904 bytes)**.
Median runtime-reserved heap rises **343,703,552 → 352,059,392 bytes (+2.43%)**;
that directional increase is retained alongside the lower RSS and live heap.
These measurements cover one generated Go input, not every workload.

Final-source `v1guard` and `v1layout` checks pass in a clean checkout: 454
language-name uses, 101 environment reads, 756 root Go files, and 12 public-API
tag sets remain within the existing registries.

Raw logs, source hashes, paired samples, `benchstat` output, profiles, and
counter receipts are under
`/home/draco/.local/state/gotreesitter-evidence/20261011-compact-throughput`.
The baseline C census and counter receipts are the pinned final-source
receipts from the earlier allocation run, now committed as `3a76b2df4`; the
candidate sweeps were rerun. Reproduce using the Go/YAML and memory commands
under [the allocation receipt](#final-allocation-performance-receipt), with a
clean **`3a76b2df4`** checkout at `/baseline` and this candidate at `/workspace`.

### Go lexer scratch and YAML speculative reservation

Two allocation changes preserve the existing route decisions. Scheduler reset
now keeps the independent before/after relex buffers through the existing
clearing and capacity checks. Go reuses a 4,096-byte scanner buffer instead of
allocating it on every parse. Both scratch records now contribute their full
capacity to the scheduler's soft-budget and hard-ceiling checks, including
when their lengths are zero. The existing limits permit at most 24,704
additional retained bytes per pooled runtime; the observed Go case retains
4,096 bytes. This scratch remains separate from the 64 KiB dependency-history
allowance. Tests cover independent snapshots, cleared scanner bytes and retry
masks, repeated resets, oversized-buffer rejection, exact budget boundaries,
and the stability of a still-live tree across pooled parses.

The initial record-arena reservation now uses a source-proportional estimate
with a 128-byte minimum, while the full reservation still waits for at least
1 KiB of source progress. Inputs at most 1 KiB and at least 16 KiB retain their
previous reservation plan. On the 1,998-byte YAML sample, the initial node
capacity falls from 768 to 96. Dense prefixes can still grow normally, and
accepted parses still reserve for the whole source after the progress
threshold. Declined attempts still release record-arena retention. Tests
cover small accepted and declined attempts, dense prefixes, reservation
limits, and release behavior.

The final source passes all 206 separate 72-step edit sessions and 618
locked-C fixture checks. The strict 204-file C census is byte-identical to
baseline: 159 passes, 45 known fallback/custom-source failures, and all 120
actual compact acceptances passing. Focused lifecycle and race checks pass.
All 412 counter rows are byte-identical before and after; the strict 2%
ledger passes without another exception. Representative counters are:

| Route and phase | Tokens before → after | Nodes before → after | Max versions before → after | Reused bytes before → after | Splices before → after |
| --- | ---: | ---: | ---: | ---: | ---: |
| Go candidate full | 589 → 589 | 1,251 → 1,251 | 3 → 3 | 0 → 0 | 0 → 0 |
| Go candidate first edit, legacy fallback | 374 → 374 | 1,553 → 1,553 | 8 → 8 | 1,784 → 1,784 | 266 → 266 |
| YAML default full | 107 → 107 | 288 → 288 | 1 → 1 | 0 → 0 | 0 → 0 |
| YAML default first edit | 107 → 107 | 480 → 480 | 1 → 1 | 0 → 0 | 0 → 0 |

The YAML candidate ledger row records the same no-action decline, rather than
full/edit work counters. The separate frontier diagnostic locates that decline
at the external token `spec`, bytes 346–350, after 17 elections, 45 dispatches,
and 27 reductions. The survivor contains the dropped alternatives but is
blended, and authenticated certificates describe different derivations.
Removing that veto would not establish correctness.

### Final allocation performance receipt

This comparison uses baseline `bcca83fa1` and the eight final source files,
with the inconclusive reduction/publication experiment removed. Each language
uses 20 paired alternating shuffled seeds, `GOMAXPROCS=1`, one CPU, `-count=1`,
750 ms, and `-benchmem` through `scripts/run_randomized_benchmarks.sh` in the
serialized 6 GiB Docker harness. Host source receipts pin the clean baseline
and candidate hashes; runtime source stayed frozen through correctness,
counters, and timing. The Go run includes the primary full/edit/no-edit trio
and both real-file routes.

| Workload | Before ns/op | After ns/op | Change | p | B/op before → after | Allocs/op before → after |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Go real file, compact | 1,009,016 | 1,005,365.5 | -0.36% | .183 | 6,128 → 2,032 | 29 → 28 |
| Go real file, legacy | 731,846.5 | 712,428.5 | -2.65% | .211 | 2,712 → 2,712 | 11 → 11 |
| Go generated, full | 9,652,701.5 | 9,371,894 | -2.91% | .038 | 538,414 → 538,409 | 53 → 53 |
| Go generated, single-byte edit | 109,451.5 | 107,198 | -2.06% | .030 | 401 → 400 | 5 → 5 |
| Go generated, no edit | 3.4325 | 3.286 | -4.27% | <.001 | 0 → 0 | 0 → 0 |
| YAML real file, declined + legacy | 285,742.5 | 235,919.5 | -17.44% | <.001 | 119,739 → 20,152 | 41 → 41 |
| YAML real file, legacy | 167,088.5 | 167,146 | +0.03% | .758 | 1,440 → 1,440 | 18 → 18 |

Go real-file allocation volume falls **66.84%**, with one fewer allocation
(**3.45%**); both allocation comparisons have `p < .001`. Its timing change is
inconclusive. Generated Go full/edit/no-edit timing improves in this run, but
the unchanged no-edit path and the legacy control also move. These small
shifts do not isolate a throughput gain caused by relex retention. The clear
Go result is lower allocation volume; no-edit parsing still allocates nothing.

YAML's complete declined-plus-legacy route is **17.44% faster**, with
**83.17% fewer allocated bytes**; both have `p < .001`. Allocation count stays
at 41. Its legacy timing rises **0.03%**, an inconclusive change (`p=.758`).
The candidate/legacy ratio falls **1.710 → 1.411**. Go's ratio rises
**1.379 → 1.411**, because its legacy control improves more than the compact
sample; this directional change is retained in the receipt. Both remain above
the E-A worst-file ceiling of 1.15, and YAML remains above E-C's 1.10 ceiling.
This two-language subset does not establish the all-206 timing distribution
or a C-relative speed claim. The allowlist and default stay unchanged.

The four existing Go memory-budget regressions pass on baseline and candidate:
matching production stop receipts, deterministic stopping, throttled polling,
and bounded peak footprint on the 50,000-line adversarial witness.

Twenty alternating fresh-process pairs parse exactly 1,048,576 bytes of Go.
All 40 accept compact, cover the input, and finish without errors, fallback,
crash, OOM, or timeout. Median RSS rises **338,420 → 339,078 KiB (+0.19%)**;
maximum RSS rises **358,316 → 364,204 KiB (+1.64%)**. The candidate maximum is
**355.67 bytes per source byte**, below the 400-byte limit on this probe.
Median retained heap rises **218,465,112 → 218,465,496 bytes (+384 bytes)**.
These small memory increases accompany the lower warm-parse allocation
volume; this single generated input does not certify every Go workload.

Final-source `v1guard` and `v1layout` checks pass in a clean checkout: 454
language-name uses, 101 environment reads, 756 root Go files, and 12 public-API
tag sets remain within their existing registries. No public API, graduation
allowlist, or default changes are included.

Raw logs, 20-seed samples, `benchstat` output, source hashes, counter receipts,
and frontier diagnostics are under
`/home/draco/.local/state/gotreesitter-evidence/20261011-compact-go-yaml`.
Final measurements use the `final-` prefix; the earlier unprefixed Go run
belongs to the rejected reduction experiment. To reproduce, mount a clean
`bcca83fa1` checkout at `/baseline` and the candidate at `/workspace` in the
serialized Docker harness:

```sh
export GOWORK=off GOMAXPROCS=1 GTS_ADMISSION_CANDIDATE=1
GTS_ADMISSION_REAL_CORPUS_LANGS=go bash scripts/run_randomized_benchmarks.sh \
  --output /tmp/go-after.txt --baseline-root /baseline \
  --baseline-output /tmp/go-before.txt --runs 20 --benchtime 750ms \
  --bench-regex '^(BenchmarkAdmissionR4(Legacy|Compact)|BenchmarkGoParseFullDFA|BenchmarkGoParseIncrementalSingleByteEditDFA|BenchmarkGoParseIncrementalNoEditDFA)$' \
  --require-benchmarks BenchmarkAdmissionR4Legacy,BenchmarkAdmissionR4Compact,BenchmarkGoParseFullDFA,BenchmarkGoParseIncrementalSingleByteEditDFA,BenchmarkGoParseIncrementalNoEditDFA
GTS_ADMISSION_REAL_CORPUS_LANGS=yaml bash scripts/run_randomized_benchmarks.sh \
  --output /tmp/yaml-after.txt --baseline-root /baseline \
  --baseline-output /tmp/yaml-before.txt --runs 20 --benchtime 750ms \
  --bench-regex '^BenchmarkAdmissionR4(Legacy|Compact)$' \
  --require-benchmarks BenchmarkAdmissionR4Legacy,BenchmarkAdmissionR4Compact
# Repeat in 20 alternating fresh-process pairs from each checkout:
bash scripts/compact_runtime_memory_probe.sh 1 go 24902
```

### Liquid alias metadata repair

The shipped blob lacked the locked C grammar's nonterminal alias map. A
plain-text body inside `{% if %}` or `{% for %}` consequently disappeared
from the public `block` child. Restore the five alias rows from
`hankthetank27/tree-sitter-liquid` at
`fa11c7ba45038b61e03a8a00ad667fb5f3d72088`.

`ts2go -alias-map-only` checks symbol identities, productions, and parse
tables before applying source indices to an existing blob. It preserves
the lexer, ABI, scanner state tables, and runtime certifications. Full
regeneration would change unrelated metadata. Tests cover rejection without
writing, encode/decode, preservation, and idempotence. Grammargen still omits
four hidden-repeat alias rows; Liquid remains explicitly uncertified in the
alias-map generator inventory. The inventory test also fails for the same six
pre-existing uncertified maps on the baseline and candidate: Blade, Dart,
Markdown, PureScript, Scala, and Teal. Those failures are not suppressed.

The 72-step Liquid edit session passes, including incremental/fresh equality,
root/error invariants, and zero allocations for no-edit reparses. The owner
approved the four correctness-related `new_nodes` increases on 2026-10-10
under [R3](v1-design.md#r3-counter-ledger-changes). The measured before/after
comparison changes only these four counters in two Liquid rows; the other
**410 of 412 rows** are byte-identical:

| Route and phase | Before | After | Reason |
| --- | ---: | ---: | --- |
| Candidate full | 179 | 203 | Preserve aliasable wrappers |
| Candidate first edit (legacy fallback) | 41 | 54 | Preserve aliasable wrappers |
| Default full | 255 | 289 | Preserve aliasable wrappers |
| Default first edit | 202 | 236 | Preserve aliasable wrappers |

Tokens, live versions, multiversion share, reuse bytes, splices, coverage,
stop reason, and error flags remain unchanged between the measured before/after
runs. The checked-in default first-edit baseline was 203, rather than the
measured 202; its approved target is 236. Only these four `new_nodes` baseline
values are updated. All other ledger fields and the strict 2% threshold are
preserved. The strict Docker ledger gate passes all **412 rows / 206 languages**
after these updates. The current symbol-wide preservation may build more
wrappers than each parent actually needs; removing that work requires a separate change that tracks alias use for each selected occurrence
and preserves fields, extras, scanner state, and reuse ownership.

### Liquid timing cost

The paired comparison uses 20 alternating shuffled seeds, 750 ms per benchmark,
`-count=1`, `-benchmem`, and the standard randomized script. Timings characterize
the correctness exception; the baseline update relies on explicit owner approval.

| R4 route | Before ns/op | After ns/op | Change | Mann–Whitney p | B/op before → after | Allocs/op before → after |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Compact | 292,656.5 | 290,516 | -0.73% | .841 | 5,956 → 5,956 | 12 → 12 |
| Legacy | 259,990.5 | 268,576 | +3.30% | .529 | 1,144 → 1,144 | 6 → 6 |

Neither timing difference is significant. The extra node work does not increase heap allocation counts on this sample.
This small-file run supplies no large-file RSS or C-relative speed claim.

### Bounded compact read-history reuse

Compact used to allocate and grow a new lexer-read history on every full
parse. It now retains an idle history on the exclusive runner and resets it
before the next eligible parse. Active capture and idle capacity remain
separate, so an ineligible parse cannot acquire old authorization. Completed
trees own copied receipt values, not this scratch history.

The history shares the existing 64 KiB dependency-scratch allowance with
node and leaf receipts. Reset invalidates the prior source and clears its
budget pointer. Footprint accounting includes idle capacity; a smaller next
budget drops excess capacity before the first scheduler poll. Ineligible
requests discard the idle history. After an active capture, indexed buffers
larger than twice the observed demand are discarded without allocating a
replacement. Idle initialization does not treat zero lengths as demand.
This prevents a large file from occupying the entire allowance indefinitely
and forcing every later small parse to allocate its history again.

A pooled Go regression parses a larger file and then the committed R4 sample
twice. It observes leaf storage shrink from 16,384 to 2,048 entries for 1,284
used entries, verifies the 64 KiB limit and invalidated history, and preserves
a still-live tree. Warm parses use 29 allocations versus 34 with history
explicitly discarded; randomized timing below supplies the performance evidence.

Focused reset, budget, ownership, live-tree, and concurrency tests pass,
including under the race detector. Warm read-history capture allocates zero
times in the focused regression. All 206 separate 72-step edit sessions and
618 locked-C fixture checks pass. The 204 deep-C real-file results are exactly
unchanged, including the 45 recorded fallback/custom-source mismatches. All
412 counter rows are byte-identical to the Liquid-repaired baseline. The four
Liquid counter increases above are the only differences from `1a2c1eb0f`.

### Read-history performance and memory

The isolated retention comparison uses runtime baseline `84dc9203e` (the
Liquid repair) with the identical benchmark harness copied in. It uses 20
paired alternating shuffled seeds per language, 750 ms, `-count=1`,
`-benchmem`, and `GOMAXPROCS=1` through the standard randomized script.
The Go run mixes the three primary benchmarks with both real-file routes,
so a preceding larger parse exercises the shared runner's size transition.
Production source hashes stayed unchanged across correctness, counters,
and timing. These are isolated patch measurements, not cumulative gains.

| Workload | Before ns/op | After ns/op | Change | p | B/op before → after | Allocs/op before → after |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Go real file, compact | 1,507,557 | 1,466,793 | -2.70% | .221 | 22,064 → 6,128 | 35 → 29 |
| Go real file, legacy | 1,075,681.5 | 1,077,988.5 | +0.21% | .738 | 2,712 → 2,712 | 11 → 11 |
| Go generated, full | 13,525,373.5 | 14,550,402.5 | +7.58% | .253 | 538,418 → 538,418 | 53 → 53 |
| Go generated, single-byte edit | 154,949.5 | 148,729 | -4.01% | .445 | 404.5 → 404 | 5 → 5 |
| Go generated, no edit | 4.5815 | 4.4215 | -3.49% | .565 | 0 → 0 | 0 → 0 |
| Liquid real file, compact | 192,750 | 194,910 | +1.12% | .820 | 5,956 → 4,868 | 12 → 10 |
| Liquid real file, legacy | 171,570 | 172,642.5 | +0.63% | .698 | 1,144 → 1,144 | 6 → 6 |
| YAML real file, declined + legacy | 343,704 | 343,519.5 | -0.05% | .602 | 119,739 → 119,739 | 41 → 41 |
| YAML real file, legacy | 202,046 | 197,468 | -2.27% | .512 | 1,440 → 1,440 | 18 → 18 |

Go real-file allocated bytes fall **72.23%** and allocation count falls
**17.14%**. Liquid falls **18.27%** in allocated bytes and **16.67%** in
allocation count. Each of these allocation differences has `p < .001`.
Every timing comparison is inconclusive. Directional timing increases are
recorded above, including generated Go full parsing at **+7.58%**. The
tradeoff is lower allocation volume on small accepted files; this does not
establish a throughput improvement for larger inputs. No-edit parsing still
allocates zero bytes and zero objects. YAML allocation metrics do not change.

For that read-history revision, candidate/legacy real-file timing ratios are
**1.361 for Go**,
**1.129 for Liquid**, and **1.740 for YAML**. YAML still declines, so its ratio
includes both attempts. Go and YAML remain above the E-A worst-file ceiling;
YAML remains above the E-C declined-route ceiling. This subset cannot establish
the required all-206 distribution or a C-relative speed claim.

Twenty alternating before/after fresh-process pairs parse a 1,048,612-byte
Python input. All 40 runs accept compact, cover the input, and finish without
crash, OOM, timeout, or fallback. Median RSS rises **323,472 → 324,422 KiB
(+0.29%)**; maximum RSS is **350,780 → 346,320.2 kB**. The candidate maximum is
338.19 bytes per input byte, below the 400-byte floor on this probe. Median
retained heap is **237,642,272 → 237,642,032 bytes**. The 64 KiB scratch cap
is unchanged. This one large-file workload does not certify all languages.

### Route cost before read-scratch retention

Each row uses 20 shuffled seeds after the Liquid repair and before read-scratch
retention, with legacy and compact as
separate top-level benchmarks. These measurements include all declined work.
They measure this branch against its legacy route, not the eventual frozen
M1 release baseline or C.

| Real sample | Legacy ns/op | Candidate ns/op | Ratio | Candidate route | Legacy → candidate B/op | Legacy → candidate allocs/op |
| --- | ---: | ---: | ---: | --- | ---: | ---: |
| Go, 2,047 bytes | 1,309,409 | 1,796,361 | 1.372 | compact | 2,712 → 21,553 | 11 → 34 |
| Liquid, 1,431 bytes | 268,576 | 290,516 | 1.082 | compact | 1,144 → 5,956 | 6 → 12 |
| YAML | 226,036 | 385,376.5 | 1.705 | declined + legacy | 1,440 → 119,739 | 18 → 41 |

Go and YAML differences have `p < .001`; both exceed the E-A worst-file
ceiling of 1.15. YAML also exceeds E-C's 1.10 declined-route ceiling. This is
sufficient evidence to withhold graduation; it does not measure the complete
206-grammar timing distribution.

A separate five-second Go CPU profile attributes 24.95% cumulative time to
`dispatchPassActive`, 15.20% to `reduceOutputsClassifiedIntoActive`, 15.01% to
postorder materialization, 17.15% to election, and 3.90% to read publication;
these overlapping totals must not be added. The allocation profile attributes
54.16% to `internal/incr.Reads.Record`. It includes benchmark setup, unlike
the reported timed-loop B/op. These identify scheduler/reduction work and
receipt allocation as targets; the profile itself is not speedup evidence.

### Architectural work that remains

The open [E-C issue #1321](https://github.com/odvcencio/gotreesitter/issues/1321)
requires declined-route overhead at most 10%. The public candidate entry
already checks unsupported API modes, and dispatch already rejects unsupported
action rows. The YAML R4 decline instead needs a dynamic proof: the exact
alternatives in a dropped head and a surviving head that covers them.
Rejecting all conflicted grammars would regress certified compact inputs.

The closed-frontier diagnostic establishes that containment alone is
insufficient for YAML: the surviving head is blended and its derivation differs
from the dropped head. The [ownership prototype](#move-cohort-references-out-of-hot-scheduler-records)
gets past that decline but exposes a second conflict: independently owned
lexer versions can converge on one physical graph node. Ownership through
canonicalization and rollback must be resolved before this can change
acceptance, with locked-C validation and the Kotlin unsafe-drop controls.
Do not remove
alternative-set proof state under [#1315](https://github.com/odvcencio/gotreesitter/issues/1315)
without a correct replacement.

The [E-A7 issue #1319](https://github.com/odvcencio/gotreesitter/issues/1319)
remains a Go throughput target, alongside the remaining E-A2 hot/cold record
split. In the earlier graduation profile, election accounts
for 17.26% of CPU, including 11.68% in token production and 3.19% in the
pre-election snapshot. A sole header can fork while reducing the same token,
so that snapshot must preserve the original scanner cursor and retry history.
Its removal is not a safe shortcut. Allocation reductions alone do not close
this throughput gap or supply the locked-C graduation receipts in
[#1064](https://github.com/odvcencio/gotreesitter/issues/1064).

### Reproduce

Run inside the repository's serialized, bounded Docker harness, one language
per process. Use `GOMAXPROCS=1`, one CPU, 6 GiB RAM, and `GOMEMLIMIT=5GiB`.
Raw receipts are under
`/home/draco/.local/state/gotreesitter-evidence/20261010-compact-graduation`.

```sh
# Run from cgo_harness; repeat with one language name per process.
GTS_ADMISSION_REAL_CORPUS_LANGS=liquid go test . \
  -tags treesitter_c_parity,gts_parsercorephase0 \
  -run '^(TestCompactR4StructuralLockedC|TestCompactLiquidBlockTextLockedC)$' \
  -count=1 -v

# Run from the repository root.
GTS_ADMISSION_CANDIDATE=1 GOTREESITTER_V1_INVARIANT_LANGUAGE=liquid \
  go test . -run '^TestV1InvariantGateR4EditSession$' -count=1

go run ./cmd/perfcounterledger # Strict gate against the four approved baseline updates.

# These separate top-level benchmarks include compact decline + legacy retry.
GTS_ADMISSION_REAL_CORPUS_LANGS=liquid \
  bash scripts/run_randomized_benchmarks.sh --output /evidence/after.txt \
  --baseline-root /baseline --baseline-output /evidence/before.txt \
  --bench-regex '^BenchmarkAdmissionR4(Legacy|Compact)$' \
  --require-benchmarks BenchmarkAdmissionR4Legacy,BenchmarkAdmissionR4Compact \
  --runs 20 --benchtime 750ms
```

For the paired metadata comparison, `/baseline` is `1a2c1eb0f` with the
identical new benchmark file copied in. For the isolated read-history comparison,
use `84dc9203e` with that same benchmark file copied in. Run Go and YAML route comparisons
without the two baseline arguments. Validate route counters alongside timing;
a fast fallback is not a compact acceptance.

Historical evidence date: 2026-08-25.
Historical base commit: `6dab5c9516c1372c87fa4ea08da1e155f9599dec` from `main`.
Historical candidate base commit: `6dab5c9516c1372c87fa4ea08da1e155f9599dec`.

## Historical 2026-08-25 compact matrix

Evidence date: 2026-08-25.

- Matrix totals: 64 PASS, 25 FALLBACK, and 8 SKIP.
- Divergence and error rows: zero.
- Execution: 49 sequential processes.
- Corpus manifest SHA-256: `14e811c4c278570e795a4a79f387dd15c61ff20718e3430f2091fe386e35c92b`.
- Haskell `small__Main.hs`: 260 bytes; source SHA-256
  `c60b55de99836dacb00a0f2808835895132996a95d52304cefab548b8cbdef65`;
  route `1/0`; one compact acceptance; recursive link-union work is non-zero;
  production, compact, and locked-C deep SHA-256
  `6e3806c54c39af93701157f87ac8ee9ef947108d66b7d6069d6908a4d5c71e9f`.
- The `nodeRecord` is 32 bytes. The eight-link cap is unchanged.

## Latest Swift issue #576 parser recovery candidate

Receipt base: `da6f71471aaaa835503accaa1bc2083ced90b4e6`.

Status: **GO FOR REVIEW / KEEP ISSUE #576 OPEN**. This candidate changes the
generic C-style recovery path. It does not change compact-route admission.

During recovery, the active DFA source replays from the exact skipped-prefix
offset. It verifies the stack offset, token span, points, token identity, and
scanner state. It resynchronizes the source before it emits `errorSymbol`.
Recovery replay requires a non-empty scanner checkpoint and live state. A
checkpointless scanner rejects recovery replay without changing the scanner.
Generic relex still accepts stateless scanners with empty serialization. The
Swift scanner now serializes its complete state. The parser records error-mode
lexing only after the DFA produces the token. The recovery path excludes tokens
from an external scanner. Each outer parse operation resets the recovery memo
size. Nested retries keep the larger memo. Temporary growth retains an existing
16,384-entry standard slab. Cleanup restores that slab without an allocation.
A shorter retained slab remains short. The parser returns a rejected recovery
probe before the legacy retry. Merge scratch drops its preflight state at a pool
boundary.

Entry scratch also enforces the source-sized reservation after pool reuse. It
moves an adequate retained slab to the front when possible. Otherwise, it adds
the required slab and keeps smaller slabs for later growth. This policy stops a
small control parse from changing the next large parse.

Swift no longer claims that failed scans preserve scanner state. The token
source restores failed scans by default. Swift explicitly retains its required
failed-scan state change. Retention takes precedence over failure preservation.
The source records actual start and end checkpoints for an internal token.
Each alternate retry starts from the original snapshot. After terminal
failure, only the last failed attempt remains live. Recovery replay still
requires equal checkpoints.
Incremental fast-forward restores the recorded end checkpoint. Generic relex
rejects a synthetic end-of-file lookahead that would replace a real token.
Swift defers `>?` to the DFA only when an unmatched `<` exists.

The 20-byte witness now matches locked C. The two corpus witnesses still
differ:

| Witness | Go deep SHA-256 | Locked C deep SHA-256 | Result |
|---|---|---|---|
| `let x = unsafe bar()` | `c64b894edc4a20e15f2b4127bad4223f698c8996dba091c06c34aa89386d3c68` | `c64b894edc4a20e15f2b4127bad4223f698c8996dba091c06c34aa89386d3c68` | exact |
| `stdlib_FloatingPointToString.swift` | `7cb588c1f7b44cf490d8fcddd11adb0cc56238e891156687c26660568a7f7447` | `ab96dddf088487acc700d72af9342c338901504dcf1d32b9644e9f6f6638190d` | mismatch |
| `stdlib_CollectionAlgorithms.swift` | `a3e737087be92518dbe1f8481a2b5169529b4f557b7f00033ab9da21d7aa32c9` | `132d332f511f12735d80e846f52ec1fddf5f3d0dcd7a097779640a7710497487` | mismatch |

Focused Docker gates passed with one Swift grammar, one CPU, and one test
worker. The artifacts are:

- Failed-scan mutation: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T104928Z-swift576-second-review-failed-scan`
- Incremental fast-forward: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T105001Z-swift576-second-review-fast-forward`
- Swift checkpoint grammar tests: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T105029Z-swift576-second-review-swift-checkpoints-v2`
- Generic relex contract: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T105035Z-swift576-second-review-generic-relex`
- Parser and scanner tests: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T105047Z-swift576-second-review-focused`
- Repeated clean-to-large sequence: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T105132Z-swift576-second-review-clean-large`
- Memory contract: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T105237Z-swift576-second-review-memory-contract`
- AWK recovery control: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T105100Z-swift576-second-review-awk`
- Both Swift corpus witnesses: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T105216Z-swift576-second-review-large-telemetry`

The TypeScript receipt changed only documentation and a focused test. The
Swift production inputs stayed unchanged during the rebase. These identity
gates passed on `da6f71471aaaa835503accaa1bc2083ced90b4e6`:

- Transition and generic relex tests: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T110309Z-swift576-da6-identity-root`
- Swift checkpoint grammar tests: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T110323Z-swift576-da6-swift-checkpoints`
- AWK recovery control: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T110332Z-swift576-da6-awk`
- Pooled minimal Swift parity: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T110349Z-swift576-da6-pooled-minimal`

The scanner repair benchmark used 20 seeds and a 750 millisecond duration.
No timing or allocation result regressed. The geometric mean time decreased
3.57 percent. Bytes per operation stayed unchanged. The warmed large-witness
maximum resident set size had one 594240 KiB before sample and one 597160 KiB
after sample. The observed increase is 0.491384 percent, or about 0.49 percent.
The raw benchmark files are
`/tmp/swift576-scanner-checkpoint-before.txt` and
`/tmp/swift576-scanner-checkpoint-after.txt`.

PR #967 exposed Templ, Swift operator, recovery memo, and Markdown reuse
regressions. The repair restores default failed-scan rollback. Swift keeps an
explicit failure-retention capability. The final scanner, Swift, Templ, and
Markdown artifacts are:

- `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T121348Z-swift967-repair-final-scanner`
- `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T121422Z-swift967-repair-final-swift`
- `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T121436Z-swift967-repair-final-templ`
- `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T121446Z-swift967-repair-final-markdown-race`

The 20-seed primary trio changed by 1.242879 percent in the median geomean.
No primary timing lane changed significantly. Bytes and allocations per
operation stayed unchanged. One warmed large-witness sample changed from
565920.2 kB to 565280 KiB. The observed decrease is 0.113090 percent.

The large witnesses still fail correctness. Keep issue #576 open until both
corpus witnesses match locked C.

## C26a Swift issue #576 token-production blocker

Receipt base: `18d63b6f7802b28a0ddb889327fcd4ebebb99426`.

Status: **NO-GO / KEEP LIVE**. This receipt does not change compact route
admission, D6 status, or the bounded matrix counts. Keep issue #576 open.

The smallest existing witness is the 20-byte source `let x = unsafe bar()`.
Its source SHA-256 digest is
`b511d81ace2a89b05e8e5e0ca6730c10f2ac9295111dae013097c7c6be8861fe`.
The Go deep-tree digest is
`860b79483c37e217690deae43036bada15b259bed77713606124fa851702e62f`.
The locked C deep-tree digest is
`c64b894edc4a20e15f2b4127bad4223f698c8996dba091c06c34aa89386d3c68`.

The first divergence is
`/source_file/property_declaration[0]/call_expression[3]/ERROR[1]`, bytes
15..18. Go emits a childless `ERROR` node. Locked C emits an `ERROR` node with
the `bar` child. The two existing Swift corpus witnesses share this unsafe
expression-prefix cause.

The locked C reference parser uses grammar version `0.7.2`, grammar commit
`41d6e5fe811ec94229ee71771174a8cce558dfee`, runtime `0.25.1`, and runtime
commit `f5afe475deb7c0bae6407fb776c76824f717bb61`. Its lexer skips `bar`,
emits an error token of size four, detects the error, and resumes through the
previous recovery point. Go emits identifier symbol `160`, reaches state 47
without a parser action, and enters generic recovery. Go therefore does not
receive the C error token before it materializes the childless `ERROR` node.

The exact minimal-witness C logger trace is at
`/tmp/gts-c26a-correction-artifacts/20260823T024242Z-swift576-c-trace-exact`.
It records the skipped characters, the four-byte `ERROR` token, `detect_error`,
`resume`, and `recover_to_previous state:2543, depth:2`. The trace also records
the 20-byte C and Go trees. The Go state trace is at
`/tmp/gts-c26a-artifacts/20260823T022437Z-swift576-predicate-trace-min`.

Canopy attributes the generic C error-mode acquisition to
`cRecoverAcquireToken` in `parser_recover_c.go`. The
`cRecoverResumeLookahead` path serves custom source fallback. The
`pushLexErrorRunLeaf` path owns the C-shaped error wrapper. Those paths cannot
restore a token that the deterministic finite automaton (DFA) token source did
not produce. The divergence is therefore in token production and grammar-table
behavior, before generic error materialization. The direct structural queries
were:

- `scripts/canopy_query.sh search symbols parser_recover_c.go --limit 120 --no-cache`
- `canopy graph calls 'pushLexErrorRunLeaf' parser_reduce.go --no-cache --depth 3`
- `canopy graph calls 'cRecoverAcquireToken' parser_recover_c.go --no-cache --depth 3`
- `canopy graph calls --reverse 'cRecoverResumeLookahead' parser_recover_c.go --no-cache --depth 2`

The isolated experiment tried a grammar-agnostic predicate. When normal
lookahead had no parser action, it relexed from broad error mode before
recovery. The prototype touched only `parser.go` and
`parser_dfa_token_source.go`. The focused Swift run kept both deep-tree
digests and the first divergence unchanged. The prototype was removed. No
production or test change survives.

The focused Docker artifacts are:

- `/tmp/gts-c26a-artifacts/20260823T021728Z-swift576-minimal`
- `/tmp/gts-c26a-artifacts/20260823T021757Z-swift576-corpus`
- `/tmp/gts-c26a-artifacts/20260823T022322Z-swift576-predicate2`
- `/tmp/gts-c26a-correction-artifacts/20260823T024140Z-swift576-minimal-current`
- `/tmp/gts-c26a-correction-artifacts/20260823T024242Z-swift576-c-trace-exact`

An upstream regeneration also left the witness unchanged. It produced blob
digest `be5cd0bf8df7077804fe4b54ee47d76005c9a85c7c33b857ef6d2aff34461286`.
The shipped Go Swift blob is
`be4575bc0acc3c60324aab635d067f940ac5f0557b80a8e3565d1e7d02d53582`.
The upstream probe used revision
`172ada1cc4117d0260d9340680b4134adba2bc2c`, package version `0.7.3`, and
these artifacts:

- `/tmp/gts-swift-upstream-probe-artifacts/20260823T000638Z-swift-upstream-issue576-range`
- `/tmp/gts-swift-upstream-probe-artifacts/20260823T000433Z-swift-upstream-issue576-parity`

Reopen implementation work after a grammar or lexer revision changes token
production for the `unsafe` expression prefix. Reopen it after a generic
runtime change produces the locked C error token without a Swift-specific
rule. Then run the 20-byte witness and one corpus witness in separate Docker
parity runs. Keep issue #576 open until both witnesses pass.

## C26b Swift issue #576 conformance-list recovery blocker

Receipt base: `838aba943038248529429a572c4d6d98359bd87e`.

Status: **NO-GO / KEEP LIVE**. No production or test change survives. Keep
issue #576 open.

The 66-byte witness is:

```swift
protocol P {
  associatedtype Stride: SignedNumeric, Comparable
}
```

The source SHA-256 digest is
`9cbe9baf046f1ec48eef0c3fb6e4ceadf2e38fa24e6fc23a3c9bf77d2b7b03a9`.
The Go deep-tree digest is
`8146289d41f1937be85d27f1c0b03ce84eb2d5e7f27b0ba7300bba11c19f274e`.
The locked C deep-tree digest is
`596a0910e6111ff5db146630797aea26722d830aa6ea06f4933d0b7062dd4bed`.

Both roots span `0..66` and report an error. The first divergent node is
`/source_file/protocol_declaration[0]/protocol_body[2]/ERROR[2]`.
Go emits `ERROR[51:64]` with one child, the comma at `51..52`.
Locked C emits `ERROR[51:63]` with two children:

- The comma at `51..52`.
- A nested `ERROR` node named `Comparable` at `53..63`.

The Go recovery trace identifies the first event. At byte `51`, the comma has
no parser action in state `2811`. Three recovery stacks pause. Recovery then
resumes at state `1584`. Condense chooses the ordinary lineage over the error
group at byte `63`.

At byte `64`, Go receives a zero-width `_implicit_semi` token.
`cRecoverDispatchInError` passes this token to `cAbsorbTokenIntoError`.
This extends the open Go error to byte `64`. Locked C ends the error at byte
`63` and keeps `Comparable` as a nested error.

Direct no-cache Canopy queries trace token acquisition through
`cRecoverAcquireToken`. They trace recovery through `cRecoverResumeLookahead`,
`cRecoverToState`, and `cCondenseAndResume`. They trace materialization through
`cAppendVisibleSplice` and `cSetNodeSpan`. They trace zero-width absorption
through `cRecoverDispatchInError` and `cAbsorbTokenIntoError`.
No Swift-specific parser-result helper participates in this path.

The corpus witness is `grammars/testdata/swift_corpus/stdlib_Stride.swift`.
It contains 26,410 bytes. Its source SHA-256 digest is
`4890e76decb629479ada497343e93f6c979d8f438c72b9ef0769b7b61e5e6fe1`.
The Go deep-tree digest is
`42d6eb25ba0da35d3e45e4b41cbb5d1f86f06beee67ba1f0f5c12f477c510073`.
The locked C deep-tree digest is
`992c548f12780cae0ca9eb7f456a32dd27b9f625e4748a898e22cf2cc3075a7b`.
Go emits one whole-file `ERROR` node at `0..26410`. Locked C emits the local
target error at `3991..4003` with two children. The first full-tree difference
is the existing root shape divergence. The known-failing corpus ratchet passes.

Existing generic recovery tests require zero-width skipped tokens to advance
and to update error spans. A generic change that drops this behavior could
change other grammars. A condense-ranking change also lacks cross-language
proof. Do not add a Swift rule, source-hash exception, blob exception, or
witness repair. The audit found no safe grammar-agnostic correction.

The focused Docker artifacts are:

- Minimal probe: `/tmp/gts-c26b-artifacts/20260823T031009Z-swift576-associatedtype-probe-base`.
- Corpus probe: `/tmp/gts-c26b-artifacts/20260823T031024Z-swift576-associatedtype-probe-corpus-base`.
- Corpus ratchet: `/tmp/gts-c26b-artifacts/20260823T031211Z-swift576-associatedtype-corpus-ratchet-base`.
- Go recovery trace: `/tmp/gts-c26b-artifacts/20260823T031320Z-swift576-associatedtype-go-trace-base`.
- Field projection guard: `/tmp/gts-c26b-artifacts/20260823T030838Z-swift576-associatedtype-minimal-base`.

Each run used one Swift workload, one CPU, and `GOMAXPROCS=1`. Every run
passed without an out-of-memory failure or timeout.

Reopen implementation work after an upstream grammar or table change, or
after a generic recovery change gains cross-language proof. Require the
minimal witness, the corpus ratchet, locked-C parity, field projection, and
generic zero-width tests to pass. Keep issue #576 open until both witnesses
match locked C.

## C26c Swift issue #576 masking-shift recovery blocker

Receipt base: `77ec4288a115e4ddb1969d2945e8507dad92f1af`.

This masking-shift item originated as closed issue #574.
Issue #576 is the open umbrella for this Swift recovery family.

Status: **NO-GO / KEEP LIVE**. No production or test change survives. Keep
issue #576 open.

The canonical witness includes its trailing newline:

```swift
let x = 1&<<7
```

It has 14 bytes. Its secure hash algorithm 256 (SHA-256) source digest is
`25d8869d28d13d391a46d2afcab12662c4a9c62b3c9555be966767caf27bc720`.
The Go tree digest is
`0a080f094102d27305084a234d22396a1c4b64cad5be9ab55a9969249f2a67aa`.
The locked C tree digest is
`14b99aace77ea88a972e0d1bbefcdef9f226bb764aeba12bb41c2cf1509610e9`.

Both roots span `0:14` and report an error. Go has one root child. Its
`property_declaration` spans `0:13`, and its `infix_expression` spans `8:13`.
That expression contains `integer_literal[8:9]`, `_custom_operator[9:11]`,
`ERROR[11:12]` with `<`, and `integer_literal[12:13]`.

Locked C has two root children. Its `property_declaration` spans `0:12`, and
its `infix_expression` spans `8:12`. The second root child is
`ERROR[12:13]`, with `integer_literal[12:13]` as its child. The first
divergence is `/source_file`, where Go has `children=1` and C has
`children=2`.

The 13-byte control has no trailing newline. Its source digest is
`82b80d62359b54747565d50be90f55f6177299dde3c43237b1efddc417339809`.
Its Go tree digest is
`1b4e24b4b42e2f38df92cf0a7d9ffd2345373dad5b3766f120a61b3419284740`.
Its locked C tree digest is
`b226b4ce49ab883efb4614f0cb05ad92a34ba39262beb80f8bd502c38130ffff`.
Both roots span `0:13` and have two children. Go reports no error. Locked C
reports an error. This control is not the tracked root-child absorption case.

The Go and C traces share these symbols through the final integer:

- `let[0:3]`
- `identifier x[4:5]`
- `=[6:7]`
- `integer_literal[8:9]`
- `_custom_operator[9:11]`
- `<[11:12]`
- `integer_literal[12:13]`

The implicit-semicolon spans differ:

- Go logs `_implicit_semi[14:14]`.
- C logs a one-byte `_implicit_semi` at byte `13`.

Both traces reach end of input at byte `14`. Go labels this token `end`.
C also labels this token `end`.

The first Go recovery event occurs at byte `12`. The integer literal has no
parser action in state `130`. Go calls `cRecoverToState` at state `291` with
depth `2`. Recovery creates ten stacks. Condense rejects the missing-group
lineage because the ordinary lineage has lower cost. The final Go tree keeps
`7` inside the infix expression.

The first locked-C recovery event occurs in state `1459` at byte `12`. C
lexes an integer literal of size one, detects an error, and resumes version
zero. It recovers with missing `custom_operator_token1` in state `432`, then
recovers to previous state `623` at depth `2`. It skips the integer literal.
The final locked-C tree keeps `7` in a sibling `ERROR` node.

Canopy assigns the ownership path to generic recovery and materialization:

- `parser.go:4526` `parseInternal` reaches `parser_recover_c.go:776`
  `cRecoverAcquireToken`.
- `parser_recover_c.go:4110` `cRecoverToState` uses
  `cAppendVisibleSplice:4064` and `cSetNodeSpan:4099`.
- `parser_recover_c.go:4462` `cCondenseAndResume` uses the recovery version
  comparison and lookahead paths.
- `parser.go:3876` `materializeTransientChildrenForReturnedTree` finalizes
  transient nodes.

The token sequence does not differ first. A Swift-specific lexer rule that
joins `&<<` is rejected. A generic condense change that prefers an error group
is rejected because existing tests require the lower-cost clean stack. A
generic operator or materialization heuristic lacks cross-language proof. A
source-hash exception, blob exception, or witness repair is also rejected.

The corpus witness is `grammars/testdata/swift_corpus/stdlib_ASCII.swift`.
It has 3,115 bytes. Its source digest is
`0db11184c2f5e94ae43dd5349cc37ae55fc9acca4dd2eb82b0311daefca72540`.
Its Go tree digest is
`e4cc31fc7b2c14ad9c466cabc4614cb002ac3fad8cb671eaba22a4d1661b1572`.
Its locked C tree digest is
`4721985dfa027f12728eb183afd782e77bb5a44fff22a0e4aa002e2495f648f3`.
Both roots span `0:3115`, have 14 children, and report an error. The target
`1&<<7` occurrence starts at byte `1513`. The first full-corpus difference is
the existing `user_type` versus `simple_identifier` difference at
`/source_file/class_declaration[12]/class_body[4]/function_declaration[5]/function_body[8]/statements[1]/control_transfer_statement[0]/call_expression[1]/navigation_expression[0]/user_type[0]`.
The known-failing `stdlib_ASCII.swift` corpus ratchet passes.

The focused Docker artifacts are:

- `/tmp/gts-c26c-artifacts/20260823T033300Z-swift576-masking-shift-probe-minimal-canonical`
- `/tmp/gts-c26c-artifacts/20260823T033224Z-swift576-masking-shift-probe-minimal-exact`
- `/tmp/gts-c26c-artifacts/20260823T033433Z-swift576-masking-shift-tree-dump`
- `/tmp/gts-c26c-artifacts/20260823T033312Z-swift576-masking-shift-probe-corpus`
- `/tmp/gts-c26c-artifacts/20260823T033324Z-swift576-masking-shift-corpus-ratchet`
- `/tmp/gts-c26c-artifacts/20260823T033347Z-swift576-masking-shift-go-trace-exact`
- `/tmp/gts-c26c-artifacts/20260823T033508Z-swift576-masking-shift-c-logger`

The missing-fixture trace failed and is excluded:

- `/tmp/gts-c26c-artifacts/20260823T033336Z-swift576-masking-shift-go-trace`

The Canopy audit artifacts are:

- `/tmp/c26c-canopy-corrected-parse.json`
- `/tmp/c26c-canopy-corrected-acquire.json`
- `/tmp/c26c-canopy-corrected-recover.json`
- `/tmp/c26c-canopy-corrected-condense.json`
- `/tmp/c26c-canopy-corrected-materialize.json`

Reopen implementation work only after a generic recovery or materialization
change produces the locked-C sibling `ERROR` without a Swift-specific rule.
Require the 14-byte witness, the 13-byte control, the corpus ratchet, focused
generic recovery tests, and locked-C parity to pass. Keep issue #576 open.

## C26d Swift issue #576 optional-binding recovery blocker

Receipt base: `11d9aec70eaef0c0d65c3cd14b8f594d64869c7b`.

Evidence was collected on `731f8a9d9440a006b2cc6b56ef5b31c0ff3b5ce7`.
The probe artifacts use `issue-590` labels. Issue #576 remains the open
umbrella for this Swift recovery family.

Status: **NO-GO / KEEP ISSUE #576 OPEN**. No production or test change
survives.

The focused probe uses two witnesses. The first witness has the label
`issue-590-chunked-minimal`. It has 118 bytes. Its source SHA-256 digest is
`66a0ffea75dcba15e0fe65f8c1e14221421a888e3f80b3809ba195ce88a5c0ef`.
Locked C reports a clean `source_file` root over `0..118`. Its tree digest is
`0cdb8862939d6d21cbb9b6792939381144e980ee5e45ee9dcab97a3e0930e9e7`.

The Go raw, production, compact, incremental-production, and
incremental-compact routes all report `has_error=true`. Each route reports a
`source_file` root over `0..118` with digest
`3a90ba023387b89a692aa8e9c755ee9d54008437486b26e90a77552ffec03c3b`.
The first error is `/source_file/ERROR[0]` over `0..118`. The nested error is
`/source_file/ERROR[0]/statements[8]/if_statement[0]/ERROR[5]` over `98..114`.

The Go forest route reports a clean root over `0..118` with digest
`e44be93f73b5bf5b669aba5dbc9d65e7ac065798e4a33471a70ac91df16c286e`.
It still differs from locked C at
`/source_file/class_declaration[0]/class_body[2]/function_declaration[1]/function_body[4]/statements[1]/if_statement[0]/statements[6]/if_statement[0]/call_expression[1]/call_suffix[1]/value_arguments[0]/value_argument[3]/navigation_expression[0]/user_type[0]`.
Go emits `user_type`. Locked C emits `simple_identifier`.

The second witness has the label `issue-590-chunked-corpus`. It is
`grammars/testdata/swift_corpus/swift-algorithms_Chunked.swift`. It has 27,814
bytes. Its source SHA-256 digest is
`78edea4a5ca6c8bf26b1cdf9c4f40b3f38cca0782cd45ec5ccddd14c02f7d38c`.
Locked C reports a clean `source_file` root over `0..27814`. Its tree digest
is `48b7dc83a6cfb41c5a9a631abf91b71b24e5295fb4dc35c9052ede79d984bbf8`.

The Go raw, production, compact, incremental-production, and
incremental-compact routes all report `has_error=true`. Each route reports a
`source_file` root over `0..27814` with digest
`27f732bd09e3676f54b637f1dfd2fbb87762f2b447e3a87d352e513df47c9bfc`.
The corpus forest route declines with `forest route declined`. It produces no
forest digest. The Go corpus result therefore does not establish C parity.

The runtime records no concrete recovery action for either witness. It
reports `action=false` with zero state, byte, token, type, and result fields.
It also reports `normalize=0/0`, `rewritten=0`, and `retry=0`.

Fresh production and compact routes report one `swift_legacy` entry for each
witness. Raw routes report zero. The minimal forest route reports zero. The
incremental logs omit this field. Do not infer a complete legacy-pass total.

Both incremental-production and incremental-compact routes report
`external_scanner_unsupported`. They report zero reused subtrees, zero reused
bytes, and `OldTreeReuseRoute=false`. They do not establish incremental reuse.

The focused condition-family controls passed fresh production, incremental
production, fresh compact, and incremental compact routes. The controls are
`issue-590-condition-list` with digest
`f9b26a3b9b7a881e53dbe70a512733c98b6c77f86ab7550a04924b370035df63` and
`issue-591-else-if-binding` with digest
`018cecb4cb76ff3c7bbecb40546c1832fdfcaa644debd1c489c634a762b91c84`.
These controls do not establish parity for the optional-binding witness.

The Swift detector boundaries are narrow:

- `swiftIfStatementSwallowedThenBlockKeywordEnd` requires a first child of
  type `if`. It accepts only an `ERROR` child whose first child is `else`.
- `swiftFindElseIfKeywordEnd` skips trivia. It accepts only word-boundaried
  `else` followed by `if`.
- `swiftFindOptionalBindingRHSEnd` stops at the first depth-zero comma. It
  ignores commas inside parentheses or brackets, comments, and strings.
- `swiftFindConditionBodyBrace` finds the first depth-zero `{`. It skips
  comments and strings and rejects a depth-zero `}` or `;`.

Direct no-cache Canopy queries traced this ownership path:

- `normalizeResultCompatibility` in `parser_result_compat.go:28` calls
  `applyResultCompatibility` at line 41.
- `applyResultCompatibility` calls `runLanguageResultCompatibility` at line
  103.
- `runLanguageResultCompatibility` calls `dispatcherArmSubpassCensus` at line
  408 for `dispatch.swift`.
- The Swift arm calls `normalizeSwiftCompatibilityWithCensus` at
  `parser_result_swift.go:22`.
- That path calls `normalizeSwiftRecoveredTrailingClosureConditions` at
  `parser_result_swift_conditions.go:40`.
- The condition helper calls `swiftCollectConditionParenInserts` at line 114,
  then `parseSwiftCleanFullSourceRecovery` at `parser_result_swift.go:62`.
- Recovery calls `parseForRecoveryWithMode` at `parser_api.go:568`, then
  `Parse` at line 1064.

No safe generic parser-core correction remains. The generic swallowed-error
hook returns unchanged when the returned root has an error. The compact error
region hook fails closed for deeper resume, missing insertion, or end of file.
Neither hook can synthesize the missing Swift structure.

The successful Docker runs used one Swift workload at a time, one CPU,
`GOMAXPROCS=1`, `-p=1`, and test parallelism one. They completed without an
out-of-memory failure or timeout:

- Probe: `/tmp/gts-c26d-artifacts/20260823T043130Z-swift-590-probe-v2`.
- Corpus ratchet: `/tmp/gts-c26d-artifacts/20260823T043210Z-swift-chunked-ratchet`.
- Condition controls: `/tmp/gts-c26d-artifacts/20260823T042421Z-swift-condition-family`.

The corpus manifest test and `TestSwiftCorpusProbeMatchesLegacy` passed. The
legacy comparison proves Go probe and legacy equality only. It does not prove
Go and locked-C parity.

Reopen implementation work after a grammar, table, or generic parser change.
Require Go and locked C equality on all six minimal routes. Run raw,
production, compact, forest, incremental-production, and incremental-compact
routes. Then run the chunked corpus controls on every available route. Require
the corpus forest route to produce a certified comparison instead of
declining. Require the condition-family controls to pass. Keep issue #576 open
until these controls pass.

## C26e Swift issue #576 CollectionAlgorithms recovery blocker

Receipt base: `f0904533b6398775d5df5e01bc34d32feee34900`.

Evidence base: `14f6692fac65eab817f65af8cc6072e423ca6563`.

Status: **NO-GO / KEEP ISSUE #576 OPEN**. No production or test change
survives.

The locked C identity is:

- Grammar commit `41d6e5fe811ec94229ee71771174a8cce558dfee`.
- Runtime `0.25.1`, commit `f5afe475deb7c0bae6407fb776c76824f717bb61`.
- Grammar artifact SHA-256
  `2a9f14046d4ca88b6db1316ee5f48b876aea1700e3c09811b3c87257fe827c5c`.

The 16,871-byte witness is the exact prefix of
`grammars/testdata/swift_corpus/stdlib_CollectionAlgorithms.swift`.
Its source SHA-256 digest is
`39591b90ee9164a0bc594f2206945946ea27c0ee24149a23ed9755f9466c703d`.
The tracked source has 24,056 bytes. Its source SHA-256 digest is
`1aae0051b0bfb50e17c7ac94961ee7cab7332367dcc16e827d2482be7a2dc5a`.
The prefix is therefore a byte-exact part of the tracked full source.

The prefix locked-C digest is
`030878796edce87bdae03b7fb51be6e92c52e81482a9a94edc822248c3aad9d1`.
The Go digest on raw, production, compact, and incremental routes is
`cd95cd60eb4cac2ad3d1dd7652eec9b5c188c98338e9d4fe22c9223dacedba03`.
Both roots report an error over `0..16871`. Locked C has 37 root children.
Go has 39 root children.

The prefix first-error records are:

- Go: `/source_file/class_declaration[35]/class_body[2]/function_declaration[48]/function_body[9]/ERROR[2]@13078..13223/children=2`.
- Locked C: `/source_file/class_declaration[36]/class_body[3]/function_declaration[43]/function_body[9]/statements[1]/property_declaration[0]/try_expression[3]/call_expression[1]/ERROR[1]@15495..15534/children=5`.

Go reports eight errors. Locked C reports seven errors. The first root
difference is the child count. Do not apply these prefix error paths to the
full corpus.

The full-corpus locked-C digest is
`132d332f511f12735d80e846f52ec1fddf5f3d0dcd7a097779640a7710497487`.
The Go digest on raw, production, compact, and incremental routes is
`23035510c7f709e0cf029509c1d54aef62fefe27535e12e10a7bd874c0479fe2`.
Both roots report an error over `0..24056`. Locked C has 50 root children.
Go has 39 root children. The full-corpus record reports only this root shape
difference. It does not claim a full-corpus first-error path.

All four Go routes retain their Go digest for both witnesses. Each route
reports `stop=accepted`, `c_recovery=true`, `dropped=false`, `retries=0`,
`retry_reason='\x00'`, `normalization=0/0`, `rewritten=0`, and
`authority=false`. The prefix profile reports `stacks=15`, `depth=120`,
`nodes=10511`, and `error_ceiling=0/0`, `error_peak=9/11`. The full-corpus
profile reports `stacks=16`, `depth=144`, `nodes=16483`, and the same error
ceiling and peak.

The compact route falls back for both witnesses. Its counters move from
`before=0/0` to `after=0/1`. The reason is:

```text
compact route declined at recovery [mechanism=recovery-entered]: did not accept EOF: generic scheduler has no table action for the elected token
```

The forest route declines for both witnesses:
`accepted=false reason="dead_end" states=[10 524 46 646]`.

The incremental route reports
`reuse=false unsupported=true reason="external_scanner_unsupported"` and
`old_reuse=false`. It reports zero reused subtrees, zero reused bytes, zero
accepted retries, no adopted retry, and zero recovery searches and hits.
The prefix reports `new_nodes=9105` and `max_stacks=15`. The full corpus
reports `new_nodes=14490` and `max_stacks=16`.

Canopy v0.18.0-19-g01a5f95 attributes the generic path to
`cHandleError` in `parser_recover_c.go` and
`nextGLRUnionDFAToken` in `parser_dfa_token_source.go`. The recovery graph is
at `/tmp/gts-c26e-correct-canopy-recovery.json`. The token graph is at
`/tmp/gts-c26e-correct-canopy-token.json`. Direct no-cache queries confirmed
the generic recovery and token-preference paths. No safe grammar-agnostic
correction is proven. The Swift normalization path did not run a repair:
`normalization=0/0`, `rewritten=0`.

The successful Docker artifacts are:

- `/tmp/gts-c26e-correct/harness_out/docker/20260823T053216Z-c26e-correct-swift576-collection-final2`
- `/tmp/gts-c26e-correct/harness_out/docker/20260823T052929Z-c26e-correct-swift576-collection-full`

Each run used one Swift workload, one CPU, 4 GiB of container memory,
`GOMAXPROCS=1`, `-p=1`, test parallelism one, and a 3 GiB Go memory limit.
Each run used a 20-minute test timeout. Both focused Docker tests passed with
exit code zero. Neither run timed out or hit an out-of-memory failure.

The authenticated corpus is unavailable. The repository has no
`cgo_harness/corpus_real` directory and no
`cgo_harness/perf_scan/corpus_sources.lock` file. The sidecar
`cgo_harness/perf_scan/corpus_sources.lock.sha256` does not supply that lock.
The tracked full source is a control, not an authenticated release corpus.

The shared 20-byte unsafe control is `let x = unsafe bar()`.
Its source SHA-256 digest is
`b511d81ace2a89b05e8e5e0ca6730c10f2ac9295111dae013097c7c6be8861fe`.
Its Go digest is
`860b79483c37e217690deae43036bada15b259bed77713606124fa851702e62f`.
Its locked-C digest is
`c64b894edc4a20e15f2b4127bad4223f698c8996dba091c06c34aa89386d3c68`.
The first divergence is
`/source_file/property_declaration[0]/call_expression[3]/ERROR[1]`, bytes
`15..18`. Go emits a childless error. Locked C includes the `bar` child.
This control shares the unsafe-expression-prefix family. It does not prove
parity for either CollectionAlgorithms witness.

The 645-byte construction at
`/tmp/gts-c26e-correct/harness_out/docker/20260823T052725Z-c26e-correct-swift576-collection`
is excluded. It used the wrong source and is not a valid C26e witness.

Reopen implementation work only after a grammar, lexer, or generic
token-recovery change supplies a safe proof. Require locked-C equality for the
20-byte control, the 16,871-byte prefix, and the 24,056-byte full source on
raw, production, compact, forest, and incremental routes. Require compact
and forest routes to produce certified comparisons. Require the authenticated
corpus and its source lock before release retirement. Keep issue #576 open
until these conditions pass.

## C26f Swift issue #576 FloatingPointToString recovery blocker

Receipt base: `1c30650814ec6e65cbf31184301bf4776f3e5f41`.

Evidence base: `5648911ecf509df8ec870a1214917d9e95cf54f1`.

Status: **NO-GO / KEEP ISSUE #576 OPEN**. No production or test change
survives.

This receipt covers one tracked Swift source and one faithful prefix.
It does not claim an authoritative first-token trace.

### Grammar identities

The source is
`grammars/testdata/swift_corpus/stdlib_FloatingPointToString.swift`.
The manifest names `swiftlang/swift` at `swift-6.3-RELEASE`.
The manifest path is `stdlib/public/core/FloatingPointToString.swift`.

The Go Swift identity is:

| Field | Value |
|---|---|
| `go_swift_blob_sha256` | `be4575bc0acc3c60324aab635d067f940ac5f0557b80a8e3565d1e7d02d53582` |
| `swift_grammar_commit` | `41d6e5fe811ec94229ee71771174a8cce558dfee` |

The locked C identity is:

| Field | Value |
|---|---|
| `c_contract` | `tree-sitter-c-v1` |
| `c_transport` | `cgo_parity_binding` |
| `c_binding` | `v0.25.0@adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3` |
| `c_runtime` | `0.25.1@f5afe475deb7c0bae6407fb776c76824f717bb61` |
| `c_grammar_repo` | [tree-sitter-swift](https://github.com/alex-pinkus/tree-sitter-swift) |
| `c_grammar_commit` | `41d6e5fe811ec94229ee71771174a8cce558dfee` |
| `c_grammar_artifact` | `/workspace/harness_out/parity_c_ref_cache/linux_amd64/swift-8d91e4446b76c6a7.so` |
| `c_grammar_artifact_sha256` | `2a9f14046d4ca88b6db1316ee5f48b876aea1700e3c09811b3c87257fe827c5c` |
| `compiler` | `/usr/bin/cc`, Debian 12.2.0-14+deb12u1 |

Two C26f route logs contain an identity defect:

| Artifact | Incorrect field values |
|---|---|
| `/tmp/gts-c26f-artifacts/20260823T061320Z-c26f-routes/container.log:2` | `go_grammar_commit=2346a3ab1bb3857b48b29d779a1ef9799a248cd7`; `go_grammar_blob_sha256=9cf914d26d962d1a62e7954f8b20b302337a44cb7d4a07218eec482c45a57a08` |
| `/tmp/gts-c26f-artifacts/20260823T061947Z-c26f-routes-gomax1/container.log:2` | `go:2346a3ab1bb3857b48b29d779a1ef9799a248cd7/9cf914d26d962d1a62e7954f8b20b302337a44cb7d4a07218eec482c45a57a08` |

These fields identify tree-sitter-Go. They do not identify Swift.
Treat them as artifact defects. Use the corrected Swift identity above.

### Witness bytes and tree digests

The full source has 104,681 bytes.
Its SHA-256 digest is
`ec96801e5237dff8da773f617a8a2f36e95b6a0a7c94b581855a451cd6507fdc`.
The faithful prefix has 7,316 bytes.
Its SHA-256 digest is
`6ebf3b26112a3df3611eafe82cd16ffa3f639b7b0f57608d9fdc422ccde78e72`.
The prefix ends after the complete `_float16ToStringImpl` function.

| Witness | Go deep SHA-256 | C deep SHA-256 | Go root | C root | First difference |
|---|---|---|---|---|---|
| Full, 104,681 bytes | `ec51c633a3f99515cc0cd1c0cff435a44ddc7db8e83705977d28f78bdfb0fc0e` | `ab96dddf088487acc700d72af9342c338901504dcf1d32b9644e9f6f6638190d` | `0..104681`, 1 child | `0..104681`, 353 children | `/source_file`, shape, Go children 1; C children 353 |
| Prefix, 7,316 bytes | `1460dfcc13dee3135ca5f8368cb58bdd374c203a06c45b19a23e56cca179cf36` | `2b72cea53a148ffc499ce3f8a817cfb8a27cf2a08efc158c2e011150426704d8` | `0..7316`, 130 children | `0..7316`, 130 children | `/source_file/function_declaration[129]/function_body[14]`, shape, Go children 5; C children 4 |

The full first errors are:

| Tree | First error |
|---|---|
| Go | `ERROR[0..104680] children=719` |
| Locked C | `ERROR[6828..6839] children=3` |

The prefix first errors are:

| Tree | First error |
|---|---|
| Go | `ERROR[6827..7314] children=6` |
| Locked C | `ERROR[6828..6839] children=3` |

The first full-source difference is a root shape difference.
The first prefix difference is a function-body child count.
The artifacts do not prove a first-token cause.

### Route results

Raw, production, compact, and incremental Go routes retain the Go digest.
Each route differs from locked C for both witnesses.

| Route | Full result | Prefix result |
|---|---|---|
| Raw | `exact=false`; Go deep digest above; stop accepted | `exact=false`; Go deep digest above; stop accepted |
| Production | `exact=false`; Go deep digest above; stop accepted | `exact=false`; Go deep digest above; stop accepted |
| Compact | `0/0 -> 0/1`; fallback at recovery | `0/1 -> 0/2`; fallback at recovery |
| Forest | `accepted=false`; offset 6254; symbol 175; `dead_end`; states `[10 2814]` | Same decline and state set |
| Incremental | `exact=false`; zero reuse; 124,563 new nodes | `exact=false`; zero reuse; 1,877 new nodes |

The compact decline is:

```text
compact route declined at recovery [mechanism=recovery-entered]: did not accept EOF: generic scheduler has no table action for the elected token
```

The forest route declines at offset 6254 with symbol 175.
It reports `dead_end` and states `[10 2814]` for both witnesses.

Incremental reuse is unsupported because of
`external_scanner_unsupported`.
It reports zero reused subtrees and zero reused bytes.
The full incremental profile reports `new_nodes=124563`.
The prefix profile reports `new_nodes=1877`.

### Recovery and pass telemetry

The full route reports these recovery values:

| Entries | Strategy 1 | Competitions | Walks | Error nodes | Error span | Retry passes | Retry attempts | Selected |
|---:|---:|---:|---:|---:|---:|---:|---:|---|
| 390 | 1,621 | 193,358 | 386,716 | 372 | 104,680 | 4 | 4 | `initial_merge` |

The full retry reason is `initial_result_requires_merge_width`.
The incremental route reports eight retry passes in its final fallback ladder.
The prefix route reports 13 entries, 36 strategy-one actions, 917
competitions, 1,834 walks, 6 error nodes, and error span 487.
The prefix route reports zero retry passes and zero retry attempts.

The full route reports `max_stacks=28` and `peak_depth=637`.
The prefix route reports `max_stacks=14` and `peak_depth=153`.

Normalization performs no rewrites.
Full pass counters are `1/1/19290/0` for each Swift pass.
Prefix pass counters are `1/1/447/0` for each Swift pass.
The pass visits do not establish parity.

### Memory and run controls

Resident set size (RSS) values are workload observations.
They are not a performance comparison.

| Artifact | Workload | RSS | Controls |
|---|---|---:|---|
| `/tmp/gts-c26f-artifacts/20260823T060621Z-c26f-prefix-raw` | Raw probe | 445,268 KiB | One CPU; the log does not state `GOMAXPROCS=1` |
| `/tmp/gts-c26f-artifacts/20260823T061320Z-c26f-routes` | Full and prefix routes | 1,210,240 KiB | One CPU; the log does not state `GOMAXPROCS=1` |
| `/tmp/gts-c26f-artifacts/20260823T061947Z-c26f-routes-gomax1` | Full and prefix routes | 988,640 KiB | One CPU; `GOMAXPROCS=1` |
| `/tmp/gts-c26f-artifacts/20260823T062159Z-c26f-reduction-min` | Incomplete reductions | 297,728 KiB | One CPU; `GOMAXPROCS=1` |

The Docker runs used 4 GiB of memory, a 3 GiB Go memory limit, `-p=1`,
test parallelism one, and a 20-minute timeout.
The retained logs report exit code zero.
No run timed out or hit an out-of-memory failure.
Do not compare these RSS values as a stable performance result.

### Reduction scope and exclusions

The raw prefix artifact includes two excluded, different-family slices:

| Bytes | Source SHA-256 | Exclusion |
|---:|---|---|
| 781 | `24bd84e42e555749833bc8652e3ca6893297593ca532af6faeef689db3b21008` | `/source_file/comparison_expression[3]`; Go has `comparison_expression`, C has `constructor_expression` |
| 711 | `9dae29100a640d762fa8eca8cec13a336d46778e4c0126e1f9d5c7dbe6a34eac` | `/source_file/comparison_expression[3]`; Go has `comparison_expression`, C has `constructor_expression` |

These slices are not C26f evidence.
They use a different divergence family.

The minimum reduction artifact is
`/tmp/gts-c26f-artifacts/20260823T062159Z-c26f-reduction-min`.
It contains incomplete prefixes at 6805, 6855, 6883, 6912, and 6913 bytes.
It does not contain the canonical 7,316-byte prefix.

The full reduction sweep is
`/tmp/gts-c26f-artifacts/20260823T062139Z-c26f-reduction-sweep`.
It contains endpoints 6913, 6978, 7042, 7088, 7121, 7266, and 7316.
Only endpoint 7316 is the complete canonical prefix.
Do not use the other endpoints as complete witnesses.

### Canopy reachability

Canopy version: `v0.18.0-19-g01a5f95-dirty`.

Scoped no-cache queries confirmed these reachable generic paths:

- `cHandleError`: `parser_recover_c.go:3290-3566`; call at line 4670.
- `cCondenseAndResume`: `parser.go:6713` and `parser.go:6879`.
- `parseForRecoveryWithMode`: `parser_api.go:568`; wrappers at lines 540 and 546.
- `nextGLRUnionDFAToken`: `parser_dfa_token_source.go:1113-1279`; calls at lines 539 and 746.
- `dispatchPass`: `parsercore_phase0_driver.go:5521`; called by `run` at line 5503.
- `dropGenericNoActionHeads`: `parsercore_phase0_driver.go:7203-7304`; calls at lines 5856 and 5976.

These paths show reachability only.
The retained artifacts contain tree shapes and counters.
They do not contain an authoritative first-token event trace.
Therefore, this receipt does not assign the first divergence to one token.

### Decision and reopening condition

Keep issue #576 open.
Do not ship parser, grammar, or test changes.
Do not add a Swift rule, source hash exception, or blob exception.

Reopen implementation work only after a generic grammar, lexer, or parser
producer change supplies an authoritative trace and a safe proof.
Require the corrected Swift identities and an authenticated source lock.
Require Go and locked C equality on the full and 7,316-byte witnesses.
Run raw, production, compact, forest, and incremental routes.
Require compact and forest routes to produce certified comparisons.
Require the same proof on the authenticated corpus.

## C26h Swift issue #576 MutableSpan producer-boundary blocker

Receipt base: `41d0b9de133de777aeba9c1dca091903da052a7f`.

Evidence base: `54c7f521505e23b7a32c84c2a14d3bd3175c09dd`.

Status: **NO-GO / KEEP ISSUE #576 OPEN**. No production or test change
survives.

This receipt extends C26f with a durable producer trace.
It does not alter compact admission or D6 status.

### Identities and prefixes

The source is
`grammars/testdata/swift_corpus/stdlib_FloatingPointToString.swift`.
It has 104,681 bytes.
Its source SHA-256 digest is
`ec96801e5237dff8da773f617a8a2f36e95b6a0a7c94b581855a451cd6507fdc`.

The Go Swift blob is
`be4575bc0acc3c60324aab635d067f940ac5f0557b80a8e3565d1e7d02d53582`.
The Swift grammar commit is
`41d6e5fe811ec94229ee71771174a8cce558dfee`.

The locked C grammar artifact is
`2a9f14046d4ca88b6db1316ee5f48b876aea1700e3c09811b3c87257fe827c5c`.
The locked C runtime is `0.25.1` at commit
`f5afe475deb7c0bae6407fb776c76824f717bb61`.

The full-source Go deep digest is
`ec51c633a3f99515cc0cd1c0cff435a44ddc7db8e83705977d28f78bdfb0fc0e`.
The full-source locked-C deep digest is
`ab96dddf088487acc700d72af9342c338901504dcf1d32b9644e9f6f6638190d`.

The six focused prefixes have these identities and digests:

| Bytes | Source SHA-256 | Go deep SHA-256 | Locked-C deep SHA-256 | First difference |
|---:|---|---|---|---|
| 6,805 | `525674aacceaeddb55fddef838cbb6b167db636dc0a171df3a8351ca15248c30` | `205b67cae26d6d42733c0bc1223e27406d2a435c25a162ed994a6571303ef688` | `47d1f1c78f541925f9deebf02d38bb78a68860057905e251aad4c99890fe3c99` | `/source_file`, type, Go `source_file`, C `ERROR` |
| 6,855 | `d3e340b8ef6c87769933cbb000557eeb87eff04ae8fbc27568ccefeecb0f44f7` | `1b8299a2c6ce8c6f7aafed2468fb426b191941e5966e0fa84bf162739c807498` | `3f3ea8f5fcdfba0ae9ac2e8d1b8672f2e1dfcd34c3d6625781bb0c54724ab8c4` | `function_declaration[129]/function_body[14]`, shape, 5 versus 4 children |
| 6,883 | `1d067da11cccfb64db6cb1025748994cb53cfdfb00a761b06e8b31b93af411ae` | `ac5aa2b8ff258c40c1b9405189fcea61d7d93b609fa018bb65ff40840eb37834` | `5e1bb7b780e5dedb4df2017895e203da0fb4d4e00c0eac6769788c430593494c` | `function_declaration[129]/function_body[14]`, shape, 5 versus 4 children |
| 6,912 | `c65a11b82e34178bb7189140f85e81b3c632ee4ed292b34b6257f294f25de690` | `73496d47059acfd101e1e3ee752cd8709e11432fbe0656510e8016f7fcb8ad07` | `90c282b8896a39a531e789084e80383fc48f2bffb23bb1fc7a39dce51d2950b9` | `/source_file`, type, Go `source_file`, C `ERROR` |
| 6,913 | `7f7870469f8b9e345b4295aa98d067235b0fee476617ee1f9379d54d96d7053a` | `06868a0ba3857e49cf34aba018f31e2ee8b1339153913503c4fc8468ebb2905b` | `9f43c73bc5858107e510b2455060e3f315ee17a6526bc4001fe106dce454b09a` | `function_declaration[129]/function_body[14]`, shape, 5 versus 4 children |
| 7,316 | `6ebf3b26112a3df3611eafe82cd16ffa3f639b7b0f57608d9fdc422ccde78e72` | `1460dfcc13dee3135ca5f8368cb58bdd374c203a06c45b19a23e56cca179cf36` | `2b72cea53a148ffc499ce3f8a817cfb8a27cf2a08efc158c2e011150426704d8` | `function_declaration[129]/function_body[14]`, shape, 5 versus 4 children |

Raw, production, compact, and incremental Go routes retain each Go digest.
Each route differs from locked C for every prefix.

### Durable producer trace

The event-origin artifacts are:

- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074540Z-c26h-swift-event-origin/container.log`
- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074540Z-c26h-swift-event-origin/metadata.txt`
- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074540Z-c26h-swift-event-origin/inspect.json`

The trace records this Go event:

```text
DFA tok 160 ... 6828 6839 MutableSpan state=10
```

Go then emits token 35 from byte 6839.

Locked C enters external state 1 and internal state 0 at row 156, column 23.
It emits `ERROR` with size 2.
It calls `detect_error`, resumes version 0, and skips the `ERROR` token.

The C trace later skips unrecognized characters before later recovery events.
The first mismatch is therefore producer-visible.
It occurs before recovery election, condense, or tree materialization.

### Route and resource results

The route artifacts are:

- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074338Z-c26h-swift-probe-cgo/container.log`
- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074338Z-c26h-swift-probe-cgo/metadata.txt`
- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074338Z-c26h-swift-probe-cgo/inspect.json`

The timed route artifacts are:

- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074507Z-c26h-swift-routes-time/container.log`
- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074507Z-c26h-swift-routes-time/metadata.txt`
- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074507Z-c26h-swift-routes-time/inspect.json`

The compact route declines every prefix.
Each focused counter delta records zero admissions and one fallback.

The forest route declines every prefix.

The incremental route reports `external_scanner_unsupported`.
It reuses zero subtrees and zero bytes.
At 7,316 bytes, it allocates 1,877 new nodes and sees 14 stacks.

The timed run observes 232,640 KiB maximum resident set size (RSS).
This value is a workload observation, not a performance comparison.

The explicit controls are one Swift grammar workload, one CPU, 4 GiB,
`-parallel=1`, `GOFLAGS=-p=1`, and `GOMEMLIMIT=3GiB`.
The wrapper metadata does not record `GOMAXPROCS=1`.
This receipt makes no claim about that setting.

The route and event runs report exit code zero.
They report no out-of-memory failure and no timeout.

### Canopy ownership and candidate boundary

Canopy version: `v0.18.0-19-g01a5f95-dirty`.

Scoped no-cache queries trace these generic paths:

- `dfaTokenSource.Next` at `parser_dfa_token_source.go:466`.
- `nextExternalToken` at `parser_dfa_token_source.go:2967`.
- `nextTokenForLexState` at `parser_dfa_token_source.go:945`.
- `cRecoverAcquireToken` at `parser_recover_c.go:776`.
- `cRecoverInternalErrorModeToken` at `parser_recover_c.go:926`.
- `cHandleError` at `parser_recover_c.go:3290`.
- `cRecover` at `parser_recover_c.go:3579`.
- `cCondenseAndResume` at `parser_recover_c.go:4462`.
- `updateParserStateTokenSource` at `parser.go:7823`.

The internal error-mode helper declines languages with an external scanner.
The Swift scanner therefore keeps its own error-mode semantics.

Do not convert identifier token 160 to `ERROR`.
That change could reject valid identifiers in other states and grammars.

Do not force Go to skip the identifier.
That change could discard a valid producer token.

Do not use a Swift rule, source hash, grammar blob, or artifact policy.
No grammar-agnostic correction is proven.

### Artifacts and reopening condition

The local C26h report is
`/tmp/gts-c26h-artifacts/20260823T0745Z-c26h-swift-boundary/report.md`.
Its SHA-256 digest is
`86f7d0aac31462ad8bce98d56b360eeb324a7137b910b6603541c997479b2a6e`.

The local report records the corrected controls and all six route digests.
The prior C26g route and event logs remain retained:

- `/tmp/gts-c26g-swift-20260824/harness_out/docker/20260823T071059Z-c26g-swift-prefix-routes/container.log`
- `/tmp/gts-c26g-swift-20260824/harness_out/docker/20260823T071125Z-c26g-swift-event-trace/container.log`

Reopen implementation work only after a scanner-aware, grammar-agnostic
error-mode token-frontier contract gains proof.
Require matching token identity, span, and skip behavior on this boundary.
Require clean controls and locked-C parity on raw, production, compact,
forest, and incremental routes.
Require the authenticated corpus and its source lock before retirement.
Keep issue #576 open until these conditions pass.

## C26i Swift issue #576 scanner-aware token-frontier blocker

Evidence base: `137860ebd80921094e5a8069007d49188dcb5e50`.
Publication base: `7b6f40fe089283674f5d0d19408d2380f77caf68`.

Status: **NO-GO / KEEP ISSUE #576 OPEN**. No production or test change
survives.

This receipt audits the C26h producer boundary from the evidence base.
The evidence base changes no parser, lexer, scanner, or grammar code.

### Witness identities and results

The source is
`grammars/testdata/swift_corpus/stdlib_FloatingPointToString.swift`.
It has 104,681 bytes.
Its source SHA-256 digest is
`ec96801e5237dff8da773f617a8a2f36e95b6a0a7c94b581855a451cd6507fdc`.

The Go Swift blob is
`be4575bc0acc3c60324aab635d067f940ac5f0557b80a8e3565d1e7d02d53582`.
The Swift grammar commit is
`41d6e5fe811ec94229ee71771174a8cce558dfee`.
The locked C grammar artifact SHA-256 digest is
`2a9f14046d4ca88b6db1316ee5f48b876aea1700e3c09811b3c87257fe827c5c`.
The locked C runtime is `0.25.1` at commit
`f5afe475deb7c0bae6407fb776c76824f717bb61`.

The full-source Go deep digest is
`ec51c633a3f99515cc0cd1c0cff435a44ddc7db8e83705977d28f78bdfb0fc0e`.
The full-source locked-C deep digest is
`ab96dddf088487acc700d72af9342c338901504dcf1d32b9644e9f6f6638190d`.
Go has one root child. Locked C has 353 root children.
Go's first error spans `0..104680`. C's first error spans `6828..6839`.

The six prefix witnesses retain the C26h identities and results.

| Bytes | Source SHA-256 | Go deep SHA-256 | Locked-C deep SHA-256 |
|---:|---|---|---|
| 6,805 | `525674aacceaeddb55fddef838cbb6b167db636dc0a171df3a8351ca15248c30` | `205b67cae26d6d42733c0bc1223e27406d2a435c25a162ed994a6571303ef688` | `47d1f1c78f541925f9deebf02d38bb78a68860057905e251aad4c99890fe3c99` |
| 6,855 | `d3e340b8ef6c87769933cbb000557eeb87eff04ae8fbc27568ccefeecb0f44f7` | `1b8299a2c6ce8c6f7aafed2468fb426b191941e5966e0fa84bf162739c807498` | `3f3ea8f5fcdfba0ae9ac2e8d1b8672f2e1dfcd34c3d6625781bb0c54724ab8c4` |
| 6,883 | `1d067da11cccfb64db6cb1025748994cb53cfdfb00a761b06e8b31b93af411ae` | `ac5aa2b8ff258c40c1b9405189fcea61d7d93b609fa018bb65ff40840eb37834` | `5e1bb7b780e5dedb4df2017895e203da0fb4d4e00c0eac6769788c430593494c` |
| 6,912 | `c65a11b82e34178bb7189140f85e81b3c632ee4ed292b34b6257f294f25de690` | `73496d47059acfd101e1e3ee752cd8709e11432fbe0656510e8016f7fcb8ad07` | `90c282b8896a39a531e789084e80383fc48f2bffb23bb1fc7a39dce51d2950b9` |
| 6,913 | `7f7870469f8b9e345b4295aa98d067235b0fee476617ee1f9379d54d96d7053a` | `06868a0ba3857e49cf34aba018f31e2ee8b1339153913503c4fc8468ebb2905b` | `9f43c73bc5858107e510b2455060e3f315ee17a6526bc4001fe106dce454b09a` |
| 7,316 | `6ebf3b26112a3df3611eafe82cd16ffa3f639b7b0f57608d9fdc422ccde78e72` | `1460dfcc13dee3135ca5f8368cb58bdd374c203a06c45b19a23e56cca179cf36` | `2b72cea53a148ffc499ce3f8a817cfb8a27cf2a08efc158c2e011150426704d8` |

Raw, production, compact, and incremental routes retain the Go digest.
Each route differs from locked C on every prefix.
Compact declines every prefix with one fallback and zero admissions.
Forest declines every prefix.
Incremental reports `external_scanner_unsupported` and zero reuse.
The 7,316-byte run allocates 1,877 new nodes and sees 14 stacks.

### Focused validation

The full-witness Docker artifact is
`/tmp/gts-c26i-swift-20260824/harness_out/docker/20260823T081122Z-c26i-swift-full-base`.
Its command runs the full and minimal Swift mismatch guards.
The run passed with exit code zero.
It used one Swift grammar, one CPU, 4 GiB, `-parallel=1`, `GOFLAGS=-p=1`,
and `GOMEMLIMIT=3GiB`.
The command did not set `GOMAXPROCS`.
The run had no out-of-memory kill and no wall timeout.

The exact six-prefix route artifacts remain at:

- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074338Z-c26h-swift-probe-cgo`
- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074507Z-c26h-swift-routes-time`

The timed C26h route observed 232,640 KiB maximum resident set size (RSS).
This value is a workload observation, not a performance comparison.

### Producer trace and Canopy ownership

The durable event trace is
`/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074540Z-c26h-swift-event-origin/container.log`.
It records this Go event:

```text
DFA tok 160 ... 6828 6839 MutableSpan state=10
```

Go then emits token 35 from byte 6839.
Locked C reaches row 156, column 21, with parser state 2408.
It tries external state 25 and internal state 716.
Both normal attempts fail.
It retries external state 1 and internal state 0.
It skips one unrecognized character and emits `ERROR` with size 2 at column 23.
It calls `detect_error`, resumes version 0, and skips the `ERROR` token.

The C and Go traces use different parser-state number spaces.
Do not equate C state 2408 with Go state 198 or state 10.
The first difference occurs before recovery election, condense, or materialization.

Canopy version `v0.18.0-19-g01a5f95-dirty` traced these generic paths:

- `dfaTokenSource.Next` to `nextExternalToken` and `nextDFAToken`.
- `nextDFAToken` to `scanPreferredTokenForState` and `nextTokenForLexState`.
- `cRecoverAcquireToken` to `cRecoverInternalErrorModeToken`.
- `updateParserStateTokenSource` to the shared parser-state frontier.
- `cHandleError`, `cRecover`, and `cCondenseAndResume` after token production.

The scoped commands were:

```text
scripts/canopy_query.sh search symbols parser_dfa_token_source.go --limit 120
scripts/canopy_query.sh search symbols parser_recover_c.go --limit 120
canopy graph calls 'nextExternalToken' parser_dfa_token_source.go --no-cache --reverse --depth 3
canopy graph calls 'scanPreferredTokenForState' parser_dfa_token_source.go --no-cache --reverse --depth 3
canopy graph calls 'cRecoverAcquireToken' parser_recover_c.go --no-cache --depth 3
```

### Generic contract boundary

The reusable invariant is this:

> A parser version in recovery error mode must retry its scanner-aware error
> frontier before it accepts a normal token. Apply this retry only after its
> normal external and internal lex attempts fail.

The contract must bind the parser version, scanner state, external lex state,
internal lex state, token span, and skip result.
The current token source shares one scanner payload across live versions.
The internal error-mode helper rejects languages with an external scanner.
Swift serializes its raw-string hash only.
Its carried previous-rune state is not serialized.
No generic scanner-state transfer proof exists.

Changing identifier symbol 160 to `ERROR` can reject valid identifiers.
Forcing Go to skip the identifier can discard valid input.
No grammar-agnostic candidate diff exists.
The temporary inspection test was removed.

D6a and D6b authenticate compact drop frontiers after token production.
They cannot create C's missing token or alter scanner state.
D6 frontier work does not change this blocker.

### C26j per-version scanner ownership follow-up

Evidence base: `137860ebd80921094e5a8069007d49188dcb5e50`.

The C26j audit found one mutable `externalPayload` in each
`dfaTokenSource`. A GLR stack clone copies parser and recovery data, but it
does not copy an external scanner payload or checkpoint.

`updateParserStateTokenSource` selects one primary parser state. It sends the
union of active GLR states to the shared token source. Retry snapshots save and
restore that same shared payload.

`relexTokenForStackLexState` uses the internal DFA for a parser version. It
does not re-enter the external scanner. `cCondenseAndResume` selects and
resumes a version, but it does not transfer scanner state to that version.

Swift `Serialize` writes its raw-string hash count. `Deserialize` clears
`carriedPreviousRune` and `carriedPreviousValid`. Serialized bytes alone
cannot restore every value that can affect the next scanner call.

The default-off, opt-in complete checkpoint capability must meet seven requirements:

1. Clone complete scanner state for every parser version.
2. Bind the checkpoint to scanner identity, grammar blob, parser-version
   lineage, source byte and point, external lex state, and serialized state.
3. Record state before and after successful and failed scans.
4. Copy the checkpoint when GLR creates a parser version.
5. Transfer the selected checkpoint during recovery relex, condense, and
   resume, and discard checkpoints for dead versions.
6. Require exact checkpoint identity before versions merge or share a token.
7. Revalidate the checkpoint before edit reuse, then fail closed when the
   scanner does not support the contract.

The root controls passed
`TestNextExternalTokenRetriesScannerAfterFailedPreferredCandidate`,
`TestCRecoveryGateValidatesParseTableActionAndGotoBounds`, and
`TestParserExternalScannerToken`. The scanner-free control
`TestParserIncrementalArithmeticEditMatchesFreshParse` also passed.
The Swift controls passed
`TestParitySwiftCleanRecoveryProbeControls` and both known-mismatch guards.

The root-control artifact is
`/tmp/gts-c26j-scanner-contract-20260824/harness_out/docker/20260823T082443Z-c26j-controls`.
The Swift-control artifact is
`/tmp/gts-c26j-scanner-contract-20260824/harness_out/docker/20260823T082454Z-c26j-swift`.
The Swift run used one Swift grammar. Both runs used one CPU, 4 GiB,
`-parallel=1`, `GOFLAGS=-p=1`, and `GOMEMLIMIT=3GiB`.
Neither metadata record sets `GOMAXPROCS`.
Both runs passed without an out-of-memory kill or wall timeout.

The next proof must exercise two parser versions through a failed scan, GLR
fork, C recovery retry, condense, resume, merge, and edit reuse. Run one
external-scanner control and one scanner-free control. Require locked-C parity
before testing compact admission.

### C26k conclusion

Evidence base: `5d39d9658f5071c5c0f476eaadc6ae067e6c77e1`.

The existing `CheckpointedExternalScanner` contract proves incremental
checkpoint boundaries only. It does not assign scanner state to a parser
version. `Language` has no generic grammar-blob identity. A record-only slice
cannot prove ownership across GLR forks, merges, or recovery resumes.

The integration blast radius includes the token source, GLR stack and
scheduler, merge paths, C recovery, and incremental reuse. Focused controls
passed. The exact artifacts are:

- `/tmp/gts-c26k-checkpoint-contract-20260824/harness_out/docker/20260823T084110Z-c26k-root-controls`
- `/tmp/gts-c26k-checkpoint-contract-20260824/harness_out/docker/20260823T084134Z-c26k-external-control`
- `/tmp/gts-c26k-checkpoint-contract-20260824/harness_out/docker/20260823T084144Z-c26k-scanner-free-control`
- `/tmp/gts-c26k-checkpoint-contract-20260824/harness_out/docker/20260823T084153Z-c26k-offmode-bench`
- `/tmp/gts-c26k-checkpoint-contract-20260824/harness_out/docker/20260823T084251Z-c26k-checkpoint-boundary-controls`

The runs used one CPU, 4 GiB, `GOMEMLIMIT=3GiB`, `GOFLAGS=-p=1`, and
`-parallel=1`. The metadata does not set `GOMAXPROCS`. No candidate exists, so
the off-mode benchmark is observational only. It recorded 13.69 ms/op,
14.03 MB/op, and 3,742 allocations per operation.

The narrower prerequisite is a complete checkpoint API with identity. Add
synthetic tests for independent fork copies, exact merge identity, elected
state transfer during recovery, failed-scan restoration, an external-scanner
control, and a scanner-free control. Keep the capability default off and fail
closed when identity or completeness is unavailable. No code or test change
was made.

## C26l Swift issue #576 checkpoint foundation

Evidence and publication base:
`929609ccde78b0c9f4e57cf2225e0ae1204149cb`.

Status: **ACCEPTED DEFAULT-OFF FOUNDATION**. Keep issue #576 open.

C26l adds an opt-in identity-bearing checkpoint capability. It does not add a
parser production call site. It does not change Swift, grammar, generalized
LR (GLR), recovery, merge, or incremental paths. It does not change behavior
when the capability is absent.

### API and safety boundary

`ExternalScannerCheckpointIdentityProvider` embeds the existing
`CheckpointedExternalScanner` contract. It adds `CheckpointIdentity`, which
must return stable, non-empty scanner and grammar identifiers. Each identifier
must be at most 256 bytes. The existing contract requires complete
serialization. C26l also requires non-empty serialized bytes from `Serialize`.

The internal owned record stores:

- scanner and grammar identifiers;
- source byte and source point;
- external lexer state;
- token start and end bytes;
- serialized scanner bytes.

Capture fails closed when the capability is absent, disabled, or incomplete.
It also fails closed when identity is missing or too large, serialization is
empty or too large, or the token span is inverted. Capture deep-copies identity
and serialized bytes. Fork cloning makes independent copies. Share and merge
require exact identity, source position, lexer state, token span, and
serialized bytes. Restore checks exact identity before it calls `Deserialize`.
It serializes the restored payload into the bounded buffer and requires exact
length and byte equality. It returns false on any mismatch. Callers must
discard a payload after failed verification. Restore passes a copied byte
slice and rejects an inverted token span.

The synthetic lifecycle tests cover:

- absent capability, incomplete identity, oversized identity, or invalid
  serialization;
- owned identity and serialized state bytes;
- independent fork copies;
- exact merge equality and mismatch;
- elected recovery-state transfer;
- restore after a failed scan;
- an internally overlong record and restore verification mismatch;
- identity-mismatch restore failure;
- an external-scanner control;
- scanner-free off mode.

### Structural proof and focused validation

The new files are
`external_scanner_checkpoint_capability.go` and
`external_scanner_checkpoint_capability_test.go`. Scoped Canopy searches found
the new definitions and test references. Direct no-cache Canopy impact for
`captureExternalScannerCheckpointRecord` returned only 14 affected test
functions in `external_scanner_checkpoint_capability_test.go`. It returned no
affected production file. A literal search found no production caller outside
the new API file. This proves zero production call sites. Therefore the
absent-capability path has no parser runtime or allocation change. This is a
structural proof, not a performance benchmark.

The focused Docker artifact is
`/tmp/gts-c26l-checkpoint-foundation-rebase/harness_out/docker/20260823T102036Z-c26l-checkpoint-foundation-rebase`.
The run executed the C26l tests in the root package. It used one central
processing unit (CPU), 4 GiB, `GOMEMLIMIT=3GiB`, `GOFLAGS=-p=1`, and
`-parallel=1`. The metadata does not set `GOMAXPROCS`. The run passed with
exit code zero. It had no out-of-memory kill and no wall timeout. This focused
validation did not collect maximum resident set size (RSS).

This foundation does not prove scanner ownership across real parser versions.
It does not prove GLR scheduling, recovery, merge, or edit-reuse parity. The
next proof must connect the capability to those paths without enabling it for
Swift or any grammar. Keep issue #576 open until that proof passes against
locked C.

### C26i decision and reopening condition

Keep issue #576 open.
Ship no parser, grammar, or test change.

Reopen implementation work only after a generic scanner-aware frontier
capability proves all of the following:

- Preserve per-version scanner state or an authenticated scanner checkpoint.
- Match normal and error token identity, span, and skip behavior.
- Pass clean controls and all six prefixes in locked-C parity.
- Pass the full witness on raw, production, compact, forest, and incremental routes.
- Use an authenticated source lock and corpus evidence.


## C26q SQL scanner identity gate

Publication base: `a62b9db306bcb983852cbf0043852546e864e856`.

Status: **GO FOR THE BOUNDED IDENTITY GATE.**

C26q binds an external scanner checkpoint to its scanner and grammar identity.
The gate runs at incremental checkpoint reuse. It does not change GLR
scheduling, recovery election, condense, merge, or forest ownership.

### Identity inputs

The native SQL scanner uses these authenticated inputs:

- Upstream repository: [tree-sitter-sql](https://github.com/m-novikov/tree-sitter-sql).
- Upstream commit: `587f30d184b058450be2a2330878210c5f33b3f9`.
- `src/grammar.json`: 319,142 bytes,
  SHA-256 `42f011860137175a5a0cb820d1a694e5ccca1d17f226729ff6a4e886910cde1c`.
- `src/scanner.cc`: 4,333 bytes,
  SHA-256 `d437ad9f517d7a1f4248ccd05abe58370b5040c0037c877dab1f0aefeaa04af6`.
- External names: `_dollar_quoted_string_tag`,
  `_dollar_quoted_string_content`, and `_dollar_quoted_string_end_tag`.
- Scanner semantics: tagged state, zero-length empty state, bounded tags,
  content-end scanning, state preservation on failure, and a 4,095-byte tag
  limit.

The scanner identity is the raw SHA-256 value
`7e493677411a501e6d8592c6b9cc158e21a1bfed44c72ca914e2d81e4e34861d`.
It includes the local Go port hash
`588328cd27eea49e88b704b9bd8e46958046564187a5db1a70f6622308a7fff8`.
The focused test hashes the marked local source region. A semantics change
fails the test until the scanner identity is updated with review.

The checked-in SQL blob is `grammars/grammar_blobs/sql.bin`. It has 581,443
bytes and SHA-256
`e21421cbab52b54cf5ba15c8f78a2bb4729bf4e8c0da14368069e897de451268`.
The trusted exact-byte loader records this hash before it adapts the SQL
scanner. The public `Language` API has no grammar identity setter. Callers
cannot relabel a loaded language after adaptation.

`gotreesitter.LoadLanguage` is the trusted construction path for this identity.
It hashes the exact input bytes after envelope validation. A manually created
`Language` has no trusted grammar identity and fails closed at reuse. The
focused tests reject scanner and grammar drift when raw checkpoint bytes remain
equal. The SQL source guard uses test-only file reads. Production code does not
read its source file. Identity set and inherit paths charge only the exact
state delta. Conflict state removes the charge, and reset clears it.

The override loader reads the exact override bytes and hashes them before
scanner adaptation. It never copies the checked-in grammar identity.
Each SQL identity contains two raw 32-byte SHA-256 values. The arena accounts
for 64 identity bytes in the parser memory budget. Arena reset clears them.
Tree copies and checkpoint copies preserve them. Legacy scanners without the
capability keep their previous reuse behavior.

### Native SQL certification

The native SQL language loaded the checked-in blob identity above. The focused
tests passed for identity stability, local-port source binding, checkpoint
round trips, scanner failure restore, native blob identity, cross-scanner
rejection, exact arena accounting, and SQL incremental certification.

The route probe reported this digest for every accepted route:
`2c78089192b45b7aa6f2870fa8d2e492f00ce403c6b4b26fd59d6dfa28572982`.
Production, compact, included-ranges, and incremental routes matched.
Forest admission was true. The route artifact used one CPU, 4 GiB,
`GOMEMLIMIT=3GiB`, and `GOFLAGS=-p=1`.

`/tmp/gts-c26q-hardening-artifacts/20260823T141907Z-c26q-hardening-sql-route`

Its `container.log` SHA-256 is
`269d63d598e8ffbcf9590412af73e2f18469846dc9108e29d11e8eb15b662ce4`.
Its `metadata.txt` SHA-256 is
`d40840ee4aa12992707fd8344e73ab2b540e6abb10e17b21fcf2beb6293d5cb4`.

The SQL incremental certification artifact is:

`/tmp/gts-c26q-hardening-artifacts/20260823T141430Z-c26q-hardening-sql-incremental`

Its `container.log` SHA-256 is
`a9c95e8b30bea5437c8f0e39116a08b70565b5ccada0280ec97073546261156f`.
Its `metadata.txt` SHA-256 is
`a607c51e6cceef40f86a9a8ecb5ea6666b504c00b64d518840b05b8675d68c79`.

The post-hardening raw and range gate covered SQL scanner recovery, the
no-tree retry, SQL incremental certification, and the included-range parser
probe. Its artifact is:

`/tmp/gts-c26q-hardening-artifacts/20260823T143124Z-c26q-hardening-sql-raw-ranges`

Its `container.log` SHA-256 is
`8d475defa727ff4e5c07647a2aadf68520c9e4892e8ab6efd024b76ced7042f2`.
Its `metadata.txt` SHA-256 is
`2b4fadadb9726a4d00d6cb32039daaebff9fd322e9151045c41fb5226a2677ac`.

The SQL real-corpus probe passed 25 of 25 no-error cases, 25 of 25
S-expression comparisons, and 25 of 25 deep comparisons. It saw 28 of 186
available samples. Maximum resident set size was 1,087,976 KiB. The run used
one CPU, 4 GiB, `GOMAXPROCS=1`, and `GOFLAGS=-p=1`.
The Docker artifact is:

`/tmp/gts-c26q-production-proof-20260823/harness_out/docker/20260823T142119Z-diag-sql`

Its `container.log` SHA-256 is
`c397c45908dfbe46bc8375d9f1e33fe44056d7b7ee9da70b78f1eb008e15acff`.
Its `metadata.txt` SHA-256 is
`995ff318234b4e2325904a24339b677ef9825787f8fd6eacacf5268654afc10a`.

### Generated SQL identity gate

The direct grammargen route now generates and reloads its exact blob through
`LoadLanguage`. SQL scanner adaptation then receives the generated blob hash.
The focused regression checks scanner and language identities for equality.
Missing or changed identities fail closed before checkpoint reuse.

This proof uses publication base
`d54147516440a91b8eda6983251c7cd6c4be2707` and the authenticated SQL source
under `/tmp/grammar_parity/sql`. The final focused Docker artifact is:

`/tmp/gts-c26r-artifacts/20260823T155352Z-c26r-sql-rebase-d5414751`

Its `container.log` SHA-256 is
`a70b0473a0a59f3cfcec52dfef99c9dc88e58a2c6fc8be952c717f7dc597fe7b`.
Its `metadata.txt` SHA-256 is
`1d86b47e74bd620424e46407ede358a9eef30d89afb27f1b5198e8e2f19b894e`.
The test logged scanner identity
`7e493677411a501e6d8592c6b9cc158e21a1bfed44c72ca914e2d81e4e34861d` and
generated blob identity
`4ffb2a6d09e2000126f10101db9028d28e0752ac3e4f83e401f045c3b028ca7c`.
The old parse recorded 13 checkpoint records, 4 checkpoint leaves, and 4
snapshots. The edited parse
reused 1 subtree and 16 bytes. Fresh and incremental trees matched.
The reference SQL scanner supplied stale grammar identity
`e21421cbab52b54cf5ba15c8f78a2bb4729bf4e8c0da14368069e897de451268`.
The stale checkpoint reused zero subtrees and zero bytes.

The full regression artifact recorded the existing dollar-quote divergence:

`/tmp/gts-c26r-artifacts/20260823T154425Z-c26r-sql-full-final`

Its `container.log` SHA-256 is
`7b77af8d87280b5e1de43b1234504ab7ec110f68385530d223a3562e41dbecd2`.
Its `metadata.txt` SHA-256 is
`ccede21b768050fcd6b08cfe8b4e735c3e584da63a85e7ff0bc1d818f773507e`.

The route remains blocked by four generated SQL locked-C divergences. Keep
generated SQL compact parity gated until those trees match node for node.

### Generated SQL override proof

The generated override tests use the checked-in SQL blob as input. They
change only the language name before gzip and gob encoding. The positive
override is 582,159 bytes with SHA-256
`2e484c002ffc7773fe02944901c5b2e87194932ca32025c0e3d9dac597540149`.
Its scanner receives that override hash, not the checked-in hash. Equal
override identities preserve incremental reuse and fresh-tree equality.

The drift test uses these two generated blobs:

- First: 582,165 bytes,
  SHA-256 `ec701f2ea2286db40bb0b6fa09957aab6f901962bfa5f7c103db6a0459ae2fb6`.
- Second: 582,165 bytes,
  SHA-256 `493e63f1a5645be6c6948244abb0be5654b624e98f9aed97fcff848e0fa4fe3f`.

The new language rejects the old tree after this drift. It reuses zero
subtrees and zero bytes. The fresh and incremental trees remain equal.
Adaptation without a target grammar identity keeps the scanner usable but
returns no checkpoint identity. The identity gate then fails closed.

The post-hardening SQL and override gate artifact is:

`/tmp/gts-c26q-hardening-artifacts/20260823T141319Z-c26q-hardening-sql`

Its `container.log` SHA-256 is
`2e125602ce5fb7ff33d8b9464b8edd37784ab46459cdebcd55485b72ec182cd5`.
Its `metadata.txt` SHA-256 is
`1d461d8740c49d8d18a79b5ef9ff5e922ca6891d1b7b9530ba56be652c29d241`.

The post-hardening race artifact covers the native scanner and generated
override tests. It used one CPU, 4 GiB, `GOMAXPROCS=1`, and `GOFLAGS=-p=1`.
Its artifact is:

`/tmp/gts-c26q-hardening-artifacts/20260823T141349Z-c26q-hardening-sql-race`

Its `container.log` SHA-256 is
`bd6d48591e98924cd49ba82725b5b864a98076d9553b69f2b808646f49adbed3`.
Its `metadata.txt` SHA-256 is
`915f4559d2a01d13a6fd99bd99187a7a764ff062281d0aee340ba31a5e35e847`.

### Generated SQL locked-C boundary

The generated SQL route is not the checked-in native SQL blob route. The
locked-C probe used 25 eligible samples. It reported 23 of 25 no-error
results, 21 of 25 tree matches, and four divergences.

The four known divergences are:

- `SELECT $$hey$$;`: generated SQL adds an `ERROR` at bytes 12 to 14.
- `SELECT $$(a + b)$$;`: generated SQL adds an `ERROR` at bytes 16 to 18.
- `CREATE DOMAIN test;`: generated `CREATE_DOMAIN` has one child; C has zero.
- `CREATE DOMAIN test AS text;`: generated `CREATE_DOMAIN` has one child; C has zero.

The checked-in SQL blob matches C for these witnesses. These four generated
grammar divergences remain outside the bounded native identity gate.

The post-hardening primary locked-C artifact is:

`/tmp/gts-c26q-production-proof-20260823/harness_out/grammargen_cparity/20260823_071725-c26q-hardening-sql-locked-c`

Its `container.log` SHA-256 is
`3da06f65285aa598d7b2f931a1b7e519314dcf7be9ac833e5b2977ae6d21e2c8`.
Its `metadata.txt` SHA-256 is
`7e28e8013623b9f3664621b1d34739f27393f14615af1eaf50db18118c947bad`.

### Post-hardening validation

The final hardening reran the root, SQL, race, incremental, route,
real-corpus, and locked-C gates. Each run used one container and one CPU.
No run reported an out-of-memory kill or a wall timeout.

The final root gate artifact is:

`/tmp/gts-c26q-hardening-artifacts/20260823T143340Z-c26q-hardening-root-final`

Its `container.log` SHA-256 is
`9cf08c045266c069239c06b10a6e134fe7e6eb7c797d9804aa35ec5bd5aa77f9`.
Its `metadata.txt` SHA-256 is
`1ce71c4845672333cf1fdcf2b2291d3ed0e4a8c703ac27a227ecf395d6dd9ddf`.

The root race artifact is:

`/tmp/gts-c26q-hardening-artifacts/20260823T141242Z-c26q-hardening-root-race`

Its `container.log` SHA-256 is
`cb69e6ec92349fa703a93e0fa45dccdfe087b7b838412cc23eed17b8f3daca6a`.
Its `metadata.txt` SHA-256 is
`8fcf380350708abaf4337d3d5b9e952da3128c67a807351e88dd9d4bb5e86d39`.

The SQL race, incremental, route, and diagnostic screen artifacts are:

- SQL race: `/tmp/gts-c26q-hardening-artifacts/20260823T141349Z-c26q-hardening-sql-race`.
- Incremental: `/tmp/gts-c26q-hardening-artifacts/20260823T141430Z-c26q-hardening-sql-incremental`.
- Routes: `/tmp/gts-c26q-hardening-artifacts/20260823T141907Z-c26q-hardening-sql-route`.
- Diagnostic screen, candidate: `/tmp/gts-c26q-hardening-artifacts/20260823T142008Z-c26q-hardening-sql-perf-candidate`.
- Diagnostic screen, base: `/tmp/gts-c26q-hardening-artifacts/20260823T142021Z-c26q-hardening-sql-perf-base`.

The diagnostic screen is not performance evidence. It used a temporary
benchmark and one sample. Treat the accounting change as correctness-only.

The real-corpus artifact is:

`/tmp/gts-c26q-production-proof-20260823/harness_out/docker/20260823T142119Z-diag-sql`

The locked-C artifact is:

`/tmp/gts-c26q-production-proof-20260823/harness_out/grammargen_cparity/20260823_071725-c26q-hardening-sql-locked-c`

The SQL race metadata records `GOMAXPROCS=1` and `GOFLAGS=-p=1`.
The route metadata records `GOMEMLIMIT=3GiB` and `GOFLAGS=-p=1`.
The real-corpus metadata records `GOMAXPROCS=1` and `GOFLAGS=-p=1`.

### Focused Docker gates and scope

The root identity tests passed. The root race tests passed. SQL scanner,
override, incremental, raw, included-ranges, real-corpus, and locked-C gates
also passed their process exit checks. No run reported an out-of-memory kill
or wall timeout. The focused runs used one CPU and 4 GiB. SQL race and
locked-C runs set `GOMAXPROCS=1` and `GOFLAGS=-p=1`. The real-corpus metadata
records `GOMAXPROCS=1` and `GOFLAGS=-p=1`.

The proof excludes Swift, other grammar changes, GLR scheduler changes,
recovery election changes, condense or merge changes, forest ownership
changes, and performance claims. It does not close the generated SQL parity
gap.

The code and test patch SHA-256 against the publication base is
`cdc8d0d83204a7ea613442cafa461d961f27b81d3ad7fd3e3e85aebe3906ee48`.
The patch contains these files:

- `arena.go`
- `external_scanner_checkpoint_capability.go`
- `external_scanner_checkpoint_identity_test.go`
- `external_scanner_checkpoints.go`
- `grammars/embedded_loader.go`
- `grammars/runtime/grammargen_blob_override.go`
- `grammars/grammargen_blob_override_test.go`
- `grammars/sql_scanner.go`
- `grammars/sql_scanner_test.go`
- `language.go`
- `load_language.go`
- `parser.go`
- `tree.go`

The receipt adds only `CHANGELOG.md` and this document. No C26q document
guard test exists. Therefore, no two-stage document-guard chain is needed.
The diagnostic C parity log change was restored before this receipt.

Several commands were quarantined. The root command `go test ./cgo_harness ...`
was invalid because `cgo_harness` is a separate module. A later command used
`cd /workspace/cgo_harness && go test . ...`. A trial passed the unsupported
`--gomaxprocs` option to `run_parity_in_docker.sh`; the wrapper printed usage.
Later probes passed unsupported `--out-root` and `--src-dir` options, and one
sent SQL to a wrapper that excludes SQL. Those probes produced no test
evidence. The final direct runner commands above are the evidence.

### C26q decision and reopening condition

GO applies only to the bounded native SQL identity gate and generated override
fail-closed behavior. This gate does not prove generated SQL parity with C.
Keep generated SQL compact parity gated.

Reopen the generated SQL path only after all of the following conditions hold:

- Explain and fix the four generated SQL locked-C divergences.
- Prove generated source and grammar identities from an authenticated lock.
- Pass equal-identity reuse and changed-identity rejection on generated SQL.
- Pass raw, production, compact, forest, included-ranges, and incremental
  parity for native and generated SQL.
- Avoid source-hash, blob-exception, witness-repair, and language-specific
  policy.

Keep the generated SQL route gated until it passes the full locked-C proof.

## Current bounded result

The bounded matrix completed with no silent divergence.

| Status | Files |
|---|---:|
| PASS | 63 |
| FALLBACK | 26 |
| SKIP | 8 |
| DIVERGE | 0 |
| ERROR | 0 |
| Total | 97 |

The compact route now admits no-primary multi-derivation end-of-file (EOF)
frontiers through a bounded materiality comparison. It selects a deterministic
provisional candidate only for that comparison. It requires exact public-tree
equality for every live candidate and caps the comparison at eight candidates.
The route fails closed when the cap, context, materialization, or equality
proof fails.

The C# 642-byte `variableDeclarations.cs` row now routes. Production, compact,
and locked-C report the exact deep digest
`005b39bd9a68ff9775129d3fb793b9d7a58b9f56812bb9ca9bd0eb753465dd86`.
The C# collapsed-token regression fixture and three Dart constructor probes
(class, private, and enum) route without a dispatch pass and with exact
locked-C trees. These focused probes are outside the matrix and do not change
its counts.
The change grants no language profile or digest grant.

The 97-row matrix moves from 62 PASS, 27 FALLBACK, and 8 SKIP to 63 PASS,
26 FALLBACK, and 8 SKIP. It has zero DIVERGE and ERROR rows.

The corpus manifest contains 149 verified files across 50 languages.
Its SHA-256 digest is
`14e811c4c278570e795a4a79f387dd15c61ff20718e3430f2091fe386e35c92b`.
This run selected files at or below 16,383 bytes and excluded AWK.
The AWK medium file needs a separate slow-path budget.

The selected HTML rows are:

- `small__index.html`, 258 bytes, source SHA-256
  `d6255f90847aca0286078542cd8ab6a9da0687069222bb868ab0315b05396a86`.
- `medium__a00355.html`, 9,218 bytes, source SHA-256
  `c166899fb1aeceb0a3967731a6a37cad628133f82fb17bb3c282b4afe7bf85bd`.

The compact route now collects raw terminal spans and compact subtree
identities during authenticated materialization. It preserves hidden terminal
bytes and exact public `ERROR` provenance. Caller-owned scratch bounds retained
storage. Bounded polling preserves cancellation response. The change uses no
language-specific rule.

The direct route served 64 percent of the selected files.
Production served every fallback and every ineligible file.

The C# `small__variableDeclarations.cs` row changed from fallback to direct
admission. It has 642 bytes and the exact production, compact, and locked-C
deep digest recorded above.

## D6a producer evidence

D6a publishes an authenticated drop-cohort frontier with the feature disabled
by default. It does not enable route admission, frontier history, or frontier
verification. The bounded matrix counts above remain unchanged because D6a is
producer-only.

The isolated Docker targets accepted these published and member counts:

| Grammar | Target | Published/member | Route | Natural fallback |
|---|---|---:|---:|---|
| Go | `query_compile` | 2/6 | 0/1 | Unchanged |
| Go | `rewrite` | 2/6 | 0/1 | Unchanged |
| Go | `language` | 2/2 | 0/1 | Unchanged |
| Go | `grammargen_lr` | 2/2 | 0/1 | Unchanged |
| Erlang | `macro_function_clauses` | 8/26 | 0/1 | Unchanged |
| Erlang | `macro_expanded_top_level_function` | 3/6 | 0/1 | Unchanged |
| Haskell | `smoke` | 2/2 | 0/1 | Unchanged |
| JavaScript | `functions` | 2/8 | 0/1 | Unchanged |
| Bash | `converged_split` | 4/24 | 0/1 | Unchanged |

The gate fixed complete frontier byte accounting and stale-token transaction
poisoning.
It ran 10 internal tests. The root and no-core gates passed.
The final Docker artifacts span `20260822T065000Z` through
`20260822T065433Z`.

### Interleaved performance receipt

The accepted receipt uses 20 interleaved seeds and the primary benchmark trio.

| Benchmark | Candidate versus base | Probability |
|---|---:|---:|
| Full parse | -3.01% | `p=0.678` |
| Incremental single-byte edit | -1.09% | `p=0.758` |
| Incremental no-edit | +7.34% | `p=0.910` |
| Geomean | +0.97% | — |

Full-parse bytes per operation changed by -3.10 percent.
Allocation counts remained unchanged.
Discard the earlier sequential full-set comparison because unrelated host
tests overlapped the candidate run.

This receipt does not graduate the route and does not verify D6b.

## D6b internal-consumer evidence

D6b adds a default-off authenticated internal consumer for complete frontiers.
It does not activate the route driver, admission, or production drop wiring.

The driver verifier collects exact current heads and references. It requires
one common nonzero frontier sequence, rebuilds the frontier token, and consumes
the authenticated frontier immediately before each of the three drop sites.
Zero or mixed sequences fail closed. The verifier remains default-off, and the
route remains ungraduated.

The consumer enforces these contracts:

- Authenticate the owner, epoch, election, token, frontier seal, and ordered records.
- Reject producer-cap violations, blended references, malformed offsets, and mismatched reference sets.
- Require one survivor member to match every dropped participant within one common cohort.
- Match the exact survivor reference and compare action and full derivation identity, metadata, and bytes.
- Journal the consumed state only after proof succeeds, then restore it through checkpoint rollback.

The focused Docker gate results are:

| Target | Result | Artifact |
|---|---|---|
| `TestG18D6b` | PASS, exit 0 | `harness_out/docker/20260822T101332Z-d6b-proof-tests-v4` |
| `TestG18DropCohortFrontier` plus `TestG18D6b` | PASS, exit 0 | `harness_out/docker/20260822T101347Z-d6b-frontier-regression-v4` |
| `TestG18D6bDriver` | PASS, exit 0, no out-of-memory (OOM), no timeout | `harness_out/docker/20260822T123532Z-d6b-driver-refresh-v19` |
| `TestG18D6aProducerTelemetry/go/language` | PASS, exit 0, no OOM, no timeout | `harness_out/docker/20260822T123627Z-d6a-go-language-refresh-v21` |
| `TestG18D6aProducerTelemetry/erlang/macro_expanded_top_level_function` | PASS, exit 0, no OOM, no timeout | `harness_out/docker/20260822T123634Z-d6a-erlang-refresh-v22` |
| Default no-tag compile | PASS, exit 0, no OOM, no timeout | `harness_out/docker/20260822T123642Z-d6b-no-tag-refresh-v23` |
| Combined D6a/D6b gate | FAIL, exit 1, no OOM, no timeout | `harness_out/docker/20260822T123547Z-d6b-d6a-combined-refresh-v20` |
| Combined D6a/D6b gate | FAIL, exit 1, no OOM, no timeout | `harness_out/docker/20260822T123655Z-d6b-d6a-combined-refresh-v24` |

Both combined gates failed only in the documented process-global pool telemetry comparison.
The affected subtests changed between runs. The isolated targets passed. No OOM or timeout occurred.

### Safe D6b decline

D6a authenticates each frontier member independently. D6b also requires one common action and exact derivation.

An authenticated frontier can place every dropped participant in one cohort with the survivor candidate.
If it has no common proof, the consumer returns a typed decline.
The scheduler then runs the existing alternative-set proof.

A mixed-cohort frontier is not a safe decline. The consumer returns a fatal error and poisons the transaction.

Keep every malformed, stale, foreign, resealed, blended, different-cohort, or otherwise unauthenticated frontier as a fatal error.
Do not weaken exact derivation comparison.

The `grammargen_lr` witness demonstrates this boundary:

- D6a publishes two complete frontiers.
- The target frontier has two members with equal actions and different derivation digests.
- D6b declines the frontier without changing its state or journal.
- The existing alternative-set proof declines, so production fallback serves the tree.
- Candidate and production trees have equal locked-C deep digests.

The focused Docker receipts are:

- `harness_out/docker/20260822T184936Z-d6b-core-typed-decline-refresh-review7`
- `harness_out/docker/20260822T184953Z-d6b-grammargen-lr-safe-decline-refresh-review8`
- `harness_out/docker/20260822T185008Z-d6b-driver-positive-fatal-refresh-review9`
- `harness_out/docker/20260822T185021Z-d6b-layout-budgets-refresh-review10`

These receipts do not graduate direct D6b admission.

The matrix counts and performance counts remain unchanged.
This evidence does not graduate the route or wire production drops.

### D6c blocker receipt: `grammargen_lr`

The receipt branch uses base commit `ed0568d9`. The focused Docker reproduction
ran at original probe commit `0c34a681` and records the authenticated frontier
used by the D6b decline. The frontier drops participant `1`.

| Member | Derivation digest | Length | Continuation state | Head byte |
|---|---|---:|---:|---:|
| Survivor | `9b1c3a249bec15d4b74a7462f701c491e022be80f7a51a5590f1520a76fd2c06` | 5,254 | 1,141 | 1,046 |
| Dropped | `d72a6fe90ca3aec9883bd00494eb8ca7110ede90d5f09fb5000fdc6441a79e8f` | 5,324 | 680 | 1,046 |

Both members use the same authenticated scanner checkpoint at source span
`1046..1047`. The first continuation-state difference is `1141` versus `680`.

The first public projection difference is path `[0,4]`, span `1030..1037`,
source text `prodIdx`:

- The survivor materializes terminal `identifier` symbol `86`, production `0`.
- The dropped member materializes `parameter_declaration` symbol `113`,
  production `36`, with one `_simple_type` child under field `type`.

The focused regression test records a typed `no_action` D6b decline. The
existing `grammargen_lr` fallback gate also passes. Candidate and production
trees retain locked-C deep digest
`1472cfd9a014d4034dbc1456afd12c282630ef787c3543cf0cecb73619883ad2`.

The labeled Docker artifacts are:

- `/tmp/gts-d6c-nogo-receipt-20260822/harness_out/docker/20260822T202359Z-d6c-frontier-state-public-shape`
- `/tmp/gts-d6c-nogo-receipt-20260822/harness_out/docker/20260822T202427Z-d6c-grammargen-lr-fallback`

The D6c decision is NO-GO. A drop may proceed only when the authenticated
continuation state and the canonical public projection match. Any state or
public-shape mismatch must decline and preserve the production fallback.

## Current fallback taxonomy

The 30 fallbacks divide into 15 clean production trees and 15 production error trees.

| Class | Clean | Error tree | Total | Exact trigger |
|---|---:|---:|---:|---|
| Recovery handoff | 0 | 13 | 13 | The elected token has no table action at end-of-file. |
| Selected-lineage ownership | 11 | 1 | 12 | A converged split drop lacks one selected-lineage proof. |
| Certified repetition conflicts | 3 | 0 | 3 | The generic scheduler declines a repetition shift. |
| Acceptance-frontier ownership | 1 | 1 | 2 | The end-of-file frontier has more than one active head. |
| Total | 15 | 15 | 30 | |

The recovery handoff class contains these witnesses:

- Dart
- Go module
- INI, two files
- Make
- Objective-C
- PHP, two files
- PowerShell
- SQL, two files
- Swift
- TypeScript

The selected-lineage class contains these witnesses:

- C#
- D
- Dart
- Elixir
- Julia
- Kotlin, two files
- Perl, two files
- Scala, two files
- TSX

The repetition class contains one C# file and two Haskell files.
The acceptance class contains one Bash file and one Markdown file.

The scheduler now scopes each condense action to live headers.
It no longer counts removed versions against the shared link cap.
This retires the prior Go module, OCaml, and Rust shared-cap witnesses.
The live scope retains discarded boundary history as split provenance.
Both Perl witnesses remain on the production route without an exact lineage proof.

## Class prerequisites

The recovery handoff must preserve production error-tree ownership.
Do not convert a failed compact acceptance into a clean result.

Selected-lineage ownership must identify the exact surviving reduction path.
An artifact certificate can authorize only a pinned grammar with C-oracle evidence.

Certified repetition conflicts need one reusable conflict rule or exact artifact evidence.
Do not add a grammar-name branch to the scheduler.

Post-accept continuation must preserve an accepted result while live end-of-file reductions finish.
The Markdown continuation reaches score 160 and branch 8.
It materializes the exact production digest `a411b8648c76`.
The path still needs one selected-lineage proof for an intervening converged split drop.

The Bash frontier has one accept head and one table-dead head.
Production returns an error tree for that file.
Keep this file on the production route.

## Historical bounded result at `382080a3`

Commit `963fae08` recorded this receipt on 2026-07-29.
The run used `382080a3` as its base.
It is not the current matrix result.
It enabled ratchet mode, excluded AWK, and set the maximum file size to 16,383 bytes.

| Status | Files |
|---|---:|
| PASS | 67 |
| FALLBACK | 33 |
| SKIP | 10 |
| DIVERGE | 0 |
| ERROR | 0 |
| Total | 110 |

That corpus manifest contained 147 verified files across 50 languages.
The run selected files smaller than 16,384 bytes and excluded AWK.
Its manifest included one additional Elm highlight file.
The current manifest includes that generated file.

That receipt also recorded this 206-language smoke scorecard:

| Status | Languages |
|---|---:|
| PASS | 200 |
| FALLBACK | 1 |
| SKIP | 5 |
| DIVERGE | 0 |
| ERROR | 0 |

## Earlier campaign receipts

The following sections preserve earlier certification, fixture, corpus, rejection, and performance evidence.
They do not replace the current bounded result.

### Certified acceptance frontiers

Exact artifact profiles now enable three generic selection mechanisms.

| Language | Mechanism | Pinned C commit |
|---|---|---|
| HTTP | Accept one EOF head and drop no-action siblings | `db8b4398de90b6d0b6c780aba96aaa2cd8e9202c` |
| Robot | Accept one EOF head and drop no-action siblings | `278958ff2fc44732833f717ee864c9fe4dae6e11` |
| Meson | Select the sole primary accepted derivation | `c84f3540624b81fc44067030afce2ff78d6ede05` |
| Bash | Allow converged-path reduction split drops | `a06c2e4415e9bc0346c6b86d401879ffb44058f7` |
| Erlang | Allow converged-path reduction split drops | `1d78195c4fbb1fc027eb3e4220427f1eb8bfc89e` |
| Haskell | Allow converged-path reduction split drops | `0975ef72fc3c47b530309ca93937d7d143523628` |
| JavaScript | Allow converged-path reduction split drops | `58404d8cf191d69f2674a8fd507bd5776f46cb11` |
| Python | Allow converged-path reduction split drops | `bffb65a8cfe4e46290331dfef0dbf0ef3679de11` |

Separate Docker runs compared each selected Go tree with its pinned C parser.
All eight passed field-aware exhaustive comparison.

The evidence is in these artifact directories:

- `harness_out/docker/20260728T050445Z-compact-frontier-http-c-oracle-fields`
- `harness_out/docker/20260728T050509Z-compact-frontier-robot-c-oracle-fields`
- `harness_out/docker/20260728T050512Z-compact-frontier-meson-c-oracle-fields`
- `harness_out/docker/20260728T054141Z-compact-split-bash-c-oracle-fields`
- `harness_out/docker/20260728T054151Z-compact-split-erlang-c-oracle-fields`
- `harness_out/docker/20260728T054158Z-compact-split-haskell-c-oracle-fields`
- `harness_out/docker/20260728T054206Z-compact-split-javascript-c-oracle-fields`
- `harness_out/docker/20260728T104725Z-pr491-python-cert-final`

Custom, adapted, stale, and same-name grammars retain conservative defaults.

### Bounded no-lookahead reductions

The generic scheduler now supports one authenticated synthetic-EOF shape.
One runnable head can apply one reduction and re-elect at the same byte.
A transparent goto marks the reduced node as an extra.
A root reduction must meet authenticated EOF on the next election.
The scheduler declines wider frontiers, scanner changes, and runaway re-election.

This mechanism routes the Doxygen, JSDoc, and VHDL smoke fixtures.
Field-aware C-oracle runs passed for all three grammars:

- `harness_out/docker/20260728T061644Z-compact-nolookahead-doxygen-c-oracle-fields`
- `harness_out/docker/20260728T061726Z-compact-nolookahead-jsdoc-c-oracle-fields`
- `harness_out/docker/20260728T061735Z-compact-nolookahead-vhdl-c-oracle-fields`

### Zero-width extras with byte progress

The generic scheduler now recognizes progress from a parser boundary to the
end of a zero-width extra token.
It still declines when the byte, parser state, and scanner state do not change.

This mechanism routes the COBOL smoke fixture.
Its field-aware C-oracle run passed:

- `harness_out/docker/20260728T064111Z-compact-zero-width-cobol-c-oracle-fields`

### Cooklang smoke fixture

The prior Cooklang smoke source ended an ingredient instruction with a period.
Production recovery discarded that period.
The fixture now uses a valid ingredient instruction without the period.
The corrected source routes directly.
The old dotted source remains a required compact fallback.

The Cooklang field-aware C-oracle run passed:

- `harness_out/docker/20260728T064513Z-compact-clean-smoke-cooklang-c-oracle-fields`

### Lock-pinned corpus refresh

The previous 109-file matrix used a stale generated corpus directory.
The builder at `382080a3` selected one additional Elm highlight source.
Two independent rebuilds produced the same normalized manifest hash.
After volatile fields are removed, the hash is
`1e9998f1e4282c3c3397f518638a8779e016a0038064903f8de90f48b781661e`.

| Field | Value |
|---|---|
| Repository | `elm-tooling/tree-sitter-elm` |
| Commit | `6d9511c28181db66daee4e883f811f6251220943` |
| Source path | `test/highlight/basic.elm` |
| Bytes | 1,231 |
| SHA-256 | `8fca87bd8cc2735e83704acd8d06ffbc6cf04e386505de45596218d7fb72642c` |
| Compact result | PASS |

The tracked [Elm fixture](../testdata/admission_direct/elm_highlight_basic.elm)
protects this source when the generated corpus directory is absent.

The canonical ratchet enforces these bounds:

- At least 110 selected rows.
- At least 67 direct PASS rows.
- At most 33 FALLBACK rows.
- Exactly 10 SKIP rows.
- No DIVERGE or ERROR rows.

Ratchet mode rejects noncanonical language, bucket, AWK, and byte filters.
Manifest order does not affect the aggregate bounds.

### Rejected C# convergence candidate

The cap-one GSS convergence candidate did not preserve C# parity.
Commit `3204480b` lost field attributes and a modifier in this 65-byte witness:

```csharp
[System.Serializable]
struct S
{
    [System.Obsolete]
    int x;
}
```

Setting `GOT_GLR_MAX_MERGE_PER_KEY=16` restored the expected tree.
The 25-case Docker suite also found a net correctness and memory loss.

| Metric | `main` | Candidate |
|---|---:|---:|
| No-error outcomes | 24/25 | 24/25 |
| Deep tree matches | 20/25 | 19/25 |
| Maximum RSS | 1,382,676 KiB | 1,601,596 KiB |

The campaign rejected this candidate.

### Performance gate

The stable benchmark trio used:

- `GOMAXPROCS=1`
- `-count=10`
- `-benchtime=750ms`
- `-benchmem`

The comparison uses `f639fbaa` as the base.

| Benchmark | Time | Bytes | Allocations |
|---|---:|---:|---:|
| Full DFA parse | -8.01% | -12.06% | unchanged |
| Incremental single-byte edit | -5.71% | unchanged | unchanged |
| Incremental no-edit | +1.48% | unchanged | unchanged |

The large 5,000-function probe completed without an out-of-memory failure.
A balanced rerun measured 83,240 KiB for the base and 83,360 KiB for the head.

Compact reduction outputs now carry the multi-pop fact directly.
This avoids copying the complete compact work record twice per reduction.

The compact scheduler now stores its seed frontier in its own allocation.
The warm full-parse benchmark moves from 20,352 to 20,328 bytes per operation.
Allocation count moves from 66 to 65 per operation.
Time remains statistically unchanged (`p=0.853`).

## Reproduce the run

Run this command from the repository root:

```sh
GTS_ADMISSION_REAL_CORPUS=1 \
GTS_ADMISSION_REAL_CORPUS_EXCLUDE_LANGS=awk \
GTS_ADMISSION_REAL_CORPUS_MAX_BYTES=16383 \
GOMAXPROCS=1 go test . \
  -tags gts_parsercorephase0 \
  -run '^TestAdmissionCandidateRealCorpusMatrix$' \
  -count=1 \
  -v
```

The test reads `cgo_harness/corpus_real/manifest.json` by default.
Use `GTS_ADMISSION_REAL_CORPUS_MANIFEST` to select another manifest.
This command reproduces the current 97-row bounded result.
Ratchet mode applies only to the historical 110-row receipt.

Use these optional filters:

- `GTS_ADMISSION_REAL_CORPUS_LANGS`
- `GTS_ADMISSION_REAL_CORPUS_EXCLUDE_LANGS`
- `GTS_ADMISSION_REAL_CORPUS_BUCKETS`
- `GTS_ADMISSION_REAL_CORPUS_MAX_BYTES`

## Per-language matrix

| Language | PASS | FALLBACK | SKIP |
|---|---:|---:|---:|
| bash | 2 | 1 | 0 |
| c | 0 | 0 | 2 |
| c_sharp | 0 | 2 | 0 |
| clojure | 2 | 0 | 0 |
| cmake | 2 | 0 | 0 |
| cpp | 0 | 0 | 3 |
| css | 1 | 0 | 0 |
| d | 1 | 1 | 0 |
| dart | 0 | 2 | 0 |
| elixir | 2 | 1 | 0 |
| elm | 3 | 0 | 0 |
| erlang | 2 | 0 | 0 |
| go | 2 | 0 | 0 |
| gomod | 2 | 1 | 0 |
| graphql | 3 | 0 | 0 |
| haskell | 0 | 2 | 0 |
| hcl | 2 | 0 | 0 |
| html | 3 | 0 | 0 |
| ini | 0 | 2 | 0 |
| java | 0 | 0 | 2 |
| javascript | 1 | 0 | 0 |
| json | 0 | 0 | 3 |
| json5 | 2 | 0 | 0 |
| julia | 1 | 1 | 0 |
| kotlin | 0 | 2 | 0 |
| lua | 2 | 0 | 0 |
| make | 1 | 1 | 0 |
| markdown | 1 | 1 | 0 |
| nix | 3 | 0 | 0 |
| objc | 1 | 1 | 0 |
| ocaml | 2 | 0 | 0 |
| perl | 1 | 2 | 0 |
| php | 1 | 2 | 0 |
| powershell | 1 | 1 | 0 |
| python | 2 | 0 | 0 |
| r | 3 | 0 | 0 |
| ruby | 2 | 0 | 0 |
| rust | 1 | 0 | 0 |
| scala | 0 | 2 | 0 |
| scss | 2 | 0 | 0 |
| sql | 1 | 2 | 0 |
| svelte | 2 | 0 | 0 |
| swift | 1 | 1 | 0 |
| toml | 3 | 0 | 0 |
| tsx | 1 | 1 | 0 |
| typescript | 1 | 1 | 0 |
| xml | 2 | 0 | 0 |
| yaml | 3 | 0 | 0 |
| zig | 2 | 0 | 0 |

## Historical safety boundary receipt

The refreshed Kotlin fixture exposed one silent compact divergence.
The 38-byte reduction is:

```kotlin
internal actual fun f(): String = "x"
```

Production Go and tree-sitter C return the same error-bearing tree.
The compact parser previously returned a clean function declaration.

The compact scheduler merged two conflict paths.
A later reduction split those paths into separate heads.
The scheduler then dropped one head and accepted the other.

Admission fails closed after this unproved frontier shape.
Exact built-in profiles can certify the behavior against the C oracle.
Custom, adapted, stale, and unproved built-in grammars stay conservative.

The boundary originally moved four files to FALLBACK:

- Bash medium
- Erlang medium
- JavaScript small
- Kotlin small

Focused field-aware C-oracle receipts now certify Bash, Erlang, and JavaScript.
Those files route directly again. Kotlin remains fail-closed.

## C26y SQL compact checkpoint trace

C26y used base
`e24ccf5a87bbd7febc21f67f014c2d5301d229d0`.

Status: NO-GO.

Restore the pinned source with:

```sh
bash scripts/seed_real_corpus_from_lock.sh /tmp/gts-c26y-pinned-grammar-20260824 sql
```

The source used commit
`587f30d184b058450be2a2330878210c5f33b3f9`.

- `src/grammar.json`: `42f011860137175a5a0cb820d1a694e5ccca1d17f226729ff6a4e886910cde1c`
- `src/scanner.cc`: `d437ad9f517d7a1f4248ccd05abe58370b5040c0037c877dab1f0aefeaa04af6`

The direct production and locked-C parity artifact is
`/tmp/gts-c26y-artifacts-prod2/20260823T174027Z-c26y-production-direct-trace`.
Its log SHA-256 is
`9db58d347835f6c2fede671ebce47c89c840f8651bed93fb109c6be24ea3cbc5`.
Its metadata SHA-256 is
`17b8ca563d6e41000490371aff422500fea4857d9551e9bdffc458e0b9e2b012`.

The direct generated route recorded 13 records, 4 leaves, and 4 snapshots,
but it returned an error tree. The locked blob returned the correct tree and
recorded 14 records, 5 leaves, and 5 snapshots. The locked C comparison in
the same parity suite matched the locked blob tree. The C binding does not
expose scanner serialization bytes.

The target-symbol wrapper production artifact is
`/tmp/gts-c26y-artifacts-wrapperprod2/20260823T174239Z-c26y-wrapper-production-trace2`.
Its log SHA-256 is
`87658133020bc5232ce5e2b67b12fb61b6d253a39f3af56137894ef937b575a0`.
Its metadata SHA-256 is
`5fe266fdab999e4c2b825ff6ed81e54c4503c487a4779595cc567f95b36e0207`.
The wrapper returned the correct tree. It recorded 14 records, 5 leaves, and
5 snapshots. Its target symbols differed, but its scanner bytes matched the
locked blob at every checkpoint boundary.

The successful compact-admission artifact is
`/tmp/gts-c26y-artifacts-compact2/20260823T174539Z-c26y-compact-wrapper-trace2`.
Its log SHA-256 is
`81ff5dee3f401dd15e11b0bc3a6bee5f15e86aed18c8e177c015af5228a7ba09`.
Its metadata SHA-256 is
`4a6329873e021ad0db5a4d28c968971e629669a58fee81baccf8d5e0e1314563`.
The compact scheduler captured the same four current checkpoint pairs:

- `7-9`: `00` to `242400`
- `9-12`: `242400` to `242400`
- `12-14`: `242400` to `00`
- `14-15`: `00` to `00`

The compact tree returned zero records, zero leaves, and zero snapshots. Its
incremental probe reused zero subtrees and zero bytes. The route therefore
has a semantic scanner trace but no tree-owned sidecar proof. Do not enable
checkpointed compact reuse from this trace alone.

The 13/4/4 values are path-specific. They come from the direct generated
route's failed external-token mapping and error recovery. The invariant for a
correct SQL route is the four byte-span and serialized-state pairs above,
plus a complete tree with no error. A future candidate must materialize these
sidecars, preserve scanner and grammar identity, and prove nonzero reuse.
Keep SQL compact admission gated. Do not require the path-specific 13/4/4
receipt for a semantically correct route.

## C26z SQL compact checkpoint sidecars

C26z used base
`f8b9d718ee19f65598e274035f5481a899ab2b72`.

Status: NO-GO.

The diagnostic candidate attached the scheduler's authenticated checkpoint
pairs to compact terminal nodes before parent construction. It also bound
adapted scanners to the target grammar blob identity. Duplicate spans with
different states failed closed. The candidate was reverted after the C26aa
screen because compact replay did not prove production-equivalent reuse.

The pinned SQL source remains commit
`587f30d184b058450be2a2330878210c5f33b3f9` with grammar hash
`42f011860137175a5a0cb820d1a694e5ccca1d17f226729ff6a4e886910cde1c` and
scanner hash
`d437ad9f517d7a1f4248ccd05abe58370b5040c0037c877dab1f0aefeaa04af6`.

The generated SQL witness records 14 checkpoint records, 5 leaf nodes, and
5 snapshot payloads. It uses these scanner transitions:

- `7-9`: `00` to `242400`
- `9-12`: `242400` to `242400`
- `12-14`: `242400` to `00`
- `14-15`: `00` to `00`

The generated tree matches the locked blob and locked C trees for all three
focused inputs. Incremental parsing reuses 1 subtree and 6 bytes. A stale
grammar identity reuses 0 subtrees and 0 bytes. The focused Docker run passed:

```text
/tmp/gts-c26z-artifacts/20260823T181038Z-c26z-final-gate
```

The container log SHA-256 is
`1bb984c67b55adaaf6ded853f31473c0eeb38bc844c9c046af6ba2b9b2c99c4c`.
The metadata SHA-256 is
`d866055e83db3d9e17b45c5007b6acc2d46cca28bed28363d7951ce18a159e8d`.

The unit gate passed in
`/tmp/gts-c26z-artifacts/20260823T181230Z-c26z-unit-final`, and the grammar
binding gate passed in
`/tmp/gts-c26z-artifacts/20260823T181309Z-c26z-grammars-final`.
Their container log and metadata hashes are
`2bea71d9e86e304da3da8c7e4b04c16549c0d867408423bd79ca4f0e75d8fda6` and
`a89df6d2adb725d9cfefb55b04dc6d8b6f338155c142fa8d7d51f8bf05829215`, and
`e24f45dcc56d9960e364a9288f64731806b5a9156da9ad6196c30fe0f6589424` and
`8acd3e1d407a44fea29389ba25097d2a14191edd5fe494dfc482e93e549884ae`.

The diagnostic compact route had tree-owned sidecars and safe nonzero reuse.
The observed 6-byte reuse was lower than the earlier 16-byte production
receipt. C26aa traced the difference to parse-state replay, not span
selection. The generated compact content leaf covered bytes `9-12` with
replay state `180`; checkpoint scanning then returned an error span `9-14`.
The locked production leaf used state `16613` and returned the expected
`9-12` span. No generic state remap passed the equality and stale-identity
guards. The production and test changes were reverted. Keep SQL compact
admission gated until a generic replay contract proves production-equivalent
reuse.

## C26ad SQL compact replay state provenance

C26ad used evidence base
`cf58fba517ed4fa6a8f5d1328ac2f850d48a8c75`.
This receipt is rebased onto publication base
`515df769b9b4e2f8e3ea715e78b75a44faa3b6d6` after PR #868 merged.

Status: NO-GO. Keep SQL compact replay gated.

The pinned SQL source remains commit
`587f30d184b058450be2a2330878210c5f33b3f9`.

- `src/grammar.json`: `42f011860137175a5a0cb820d1a694e5ccca1d17f226729ff6a4e886910cde1c`
- `src/scanner.cc`: `d437ad9f517d7a1f4248ccd05abe58370b5040c0037c877dab1f0aefeaa04af6`

The source was mounted read-only from
`/tmp/gts-c26ad-grammar_parity`.

### Clean baseline

Run the focused generated SQL parity test with this command:

```sh
bash cgo_harness/docker/run_parity_in_docker.sh \
  --repo-root /tmp/gts-c26ad-sql-state-provenance-20260824 \
  --out-root /tmp/gts-c26ad-artifacts \
  --label c26ad-clean-baseline --no-build --memory 4g --cpus 1 \
  --goflags -p=1 --test-parallel 1 --timeout 10m \
  --mount /tmp/gts-c26ad-grammar_parity:/tmp/grammar_parity:ro -- \
  'export PATH=/usr/local/go/bin:$PATH; cd /workspace/cgo_harness && \
   go test -tags "cgo treesitter_c_parity" . \
   -run "^TestSQLGrammargenCGORegressionCases$" -count=1 -v'
```

The artifact is
`/tmp/gts-c26ad-artifacts/20260823T201241Z-c26ad-clean-baseline`.

- `container.log`: `d00a60bdabbcb837f69972c60020cd0fa77816ec1737957941d0b30cc79e8441`
- `metadata.txt`: `54dddcb902722f22553947592c221c0fb9a1c5fae9e3b7d0f0c5f3b135c4bc42`
- `inspect.json`: `98deafbaf286b9cb219d17517cde873c5e18913b3f6829376edfd68a303bb90e`

The run used one central processing unit, 4 GiB, `GOMEMLIMIT=6GiB`,
`GOFLAGS=-p=1`, and test parallelism one. It failed only the generated
dollar-quoted witness. The run passed the identifier and parenthesized
witnesses. The generated tree has an `ERROR` root child, while the locked blob
and locked C trees match.

The generated route recorded 13 old checkpoints, 4 old leaves, 4 old snapshots,
1 reused subtree, and 16 reused bytes. The stale grammar route recorded zero
reused subtrees and zero bytes. These counters do not certify the failed
dollar-quoted route.

### State provenance

Generation copies normalized external symbol IDs into `Language.ExternalSymbols`
(`grammargen/assemble.go:179-188`). The blob encoder serializes that language
(`grammargen/encode.go:102-106`). `LoadLanguage` restores the language tables
and records the exact blob hash (`load_language.go:34-59`).

The embedded loader binds a registered scanner to the target language before
it falls back to external-order adaptation (`grammars/embedded_loader.go:437-499`).
The SQL scanner currently binds grammar identity only. Its `Scan` method reads
symbols from the reference `SqlLanguage()` (`grammars/sql_scanner.go:74-86`,
`grammars/sql_scanner.go:155-160`).

The narrow SQL source repair is to bind the target language's external symbols
inside `ExternalScannerForLanguage` and use them in `Scan`. The temporary probe
used that repair and passed the generated production and compact witnesses.
This repair is SQL-specific. It does not preserve parser-state identity across
different language tables, so C26ad reverted it and did not ship it.

The generated and locked identities are:

| Route | Grammar or blob identity | External symbols | Content state | Scanner span |
| --- | --- | --- | --- | --- |
| Generated production | `4ffb2a6d09e2000126f10101db9028d28e0752ac3e4f83e401f045c3b028ca7c` | `[286,287,288]` | `pre=180`, `parse=449` | `[9,12)` |
| Generated compact, target symbols | `4ffb2a6d09e2000126f10101db9028d28e0752ac3e4f83e401f045c3b028ca7c` | `[286,287,288]` | `pre=180`, `parse=449` | `[9,12)` |
| Locked production | `e21421cbab52b54cf5ba15c8f78a2bb4729bf4e8c0da14368069e897de451268` | `[285,286,287]` | `pre=16613`, `parse=16652` | `[9,12)` |

The generated scanner identity is
`7e493677411a501e6d8592c6b9cc158e21a1bfed44c72ca914e2d81e4e34861d`.
The locked identity uses the same scanner implementation and a different
grammar blob identity.

Temporary target-symbol binding produced the generated rows above. Its
diagnostic artifact is
`/tmp/gts-c26ad-artifacts/20260823T200735Z-temp-state-three-languages`.
It passed its temporary probe. Its container log, metadata, and inspection
hashes are:

- `container.log`: `ed3141558aaf211d66608ed3ad08122177c51277ad5ccfb9b8ce5f6400e3f8a6`
- `metadata.txt`: `cd6cecaa55441901200004709e2918cbc2a9d6b46615d8a786045979ef7ed91c`
- `inspect.json`: `20fe89ad5fc3f5a82802b5983e23e699150dfebf0565083eddab234058c3204c`

The target-symbol scan at state `180` used generated content symbol `287` and
returned action `309` with span `[9,12)`. The compact route reconstructed the
same generated states and returned the correct tree. The temporary files and
test changes were removed after this probe.

The earlier C26aa mixed-table witness remains at
`/tmp/gts-c26z-artifacts/20260823T182021Z-c26aa-state-debug`.
Its container log hash is
`ac3f991ed44055fd93a1fb3a689adcd21a40eabd0937546fa6e1981001974737`.
It shows generated state `180` scanning `[9,14)` and locked state `16613`
scanning `[9,12)`. Its generated probe also reports `want_sym=277`, which is
not the generated symbol table in the target-symbol probe. This confirms that
the C26aa comparison crossed scanner or language-table identity.

### Replay boundary

Compact records retain symbol, production, span, child, alias, and external
flags. They retain sparse scanner checkpoint references for authenticated
external terminals (`internal/parsercorephase0/core.go:691-731`,
`internal/parsercorephase0/core.go:5115-5133`).

`MaterializationSubtreeView` does not expose parser states or checkpoint
references (`internal/parsercorephase0/core.go:822-841`). The materializer
replays each compact derivation against the parser's current language table and
stamps the resulting states (`parsercore_phase0_driver.go:4786-4828`).
`replayTransition` selects that current table's shift or goto action
(`parsestate_replay.go:109-126`). Production leaves receive their states from
the current shift action (`parser_reduce.go:2542-2582`).

The incremental leaf path restores `preGotoState`, reauthenticates the scanner,
and requires an exact token symbol and span (`incremental_leaf_fastpath.go:60-86`,
`incremental_leaf_fastpath.go:848-860`).

The reuse cursor walks old nodes in source order. `tryReuseSubtree` checks the
current parser state, the stored node state, and the scanner checkpoint gate
before it reuses a node (`incremental.go:10-85`, `incremental.go:609-657`).
`reuseTargetState` looks up the current language table and rejects a leaf when
its stored parse state does not match a current shift (`incremental.go:1028-1065`).

State `180` and state `16613` therefore have local meaning. State `180` proves
the generated table transition. State `16613` proves the locked table
transition. A numeric remap cannot prove equal shift, goto, reduction, hidden
derivation, or scanner-validity behavior. Span equality and scanner identity do
not add that proof. The compact record has no generic cross-table state map.

The current origin route remains fail-closed when compact scanner sidecars are
absent. The temporary generated compact route had no runtime parse work and
reconstructed the generated states only after target-symbol binding. No generic
state remap passed the equality and stale-identity guards. The C26z candidate
was reverted. No code or test change survives C26ad.

The focused scanner unit gate passed:

```sh
go test ./grammars -run '^TestSQLScanner' -count=1
```

### C26ad decision and reopening condition

Reject the generic state-remap fix. Keep compact replay disabled for this
scanner route.

Reopen only after a replay contract binds all of these values to one language
identity:

- parser action and goto tables;
- external symbol order and scanner identity;
- scanner checkpoint bytes and source spans;
- compact derivation identity, including hidden symbols and aliases; and
- a parity test that proves fresh, compact, incremental, and locked-C trees.

The contract must reject a missing or changed identity before it attempts
scanner replay. A numeric state translation alone is not sufficient.

## C26ag generic compact table identity guard

C26ag used publication base
`ae6e49448be249dd52dca5a95ba187fdd3000fe6`.
The isolated worktree was `/tmp/gts-c26ag-table-identity-20260824`.

Status: ACCEPTED guard. Keep the SQL compact route gated for the remaining
parity gap.

The guard adds a dependency-neutral `TableIdentityProvider` contract at
`internal/parsercorephase0/core.go:277-282`.
`Core` stores the producer identity at construction and compares it before
parser-state replay (`internal/parsercorephase0/core.go:898-900,1690-1708`).
The root adapter supplies the identity from its current `Language`
(`parsercore_phase0_driver.go:552-562`).
Replay returns an identity decline before it allocates or walks replay states
when the identity differs (`parsestate_replay_compact.go:145-155`).

Loaded languages use their exact compressed grammar blob SHA-256.
In-memory generated languages use one process-local producer token.
The contract does not translate numeric parser states.

The existing reuse cursor requires the same `*Language` pointer
(`parser.go:3450-3452`). Scanner checkpoint reuse requires scanner and grammar
identity (`external_scanner_checkpoint_capability.go:153-159`). These gates
remain unchanged. The new core guard covers the earlier replay boundary.

### Identity evidence

The focused tests cover matching identity, producer drift, and missing identity.
The blob test proves that two loaded copies use the same exact blob SHA-256.
The language-swap test proves that replay returns
`DiagnosticParserCoreIdentity` before state reconstruction.

The test files are:

- `internal/parsercorephase0/core_test.go`
- `parsercore_phase0_action_test.go`
- `parsercore_phase0_language_tables_internal_test.go`

### Docker validation

The SQL scanner unit gate ran in the combined focused command below.

Run the generated SQL, locked-C, fresh-tree, incremental-reuse, and
stale-identity gate:

```sh
bash cgo_harness/docker/run_parity_in_docker.sh \
  --repo-root /tmp/gts-c26ag-table-identity-20260824 \
  --out-root /tmp/gts-c26ag-ae6e-artifacts \
  --label c26ag-sql-generated-locked-c --no-build --memory 4g --cpus 1 \
  --goflags -p=1 --test-parallel 1 --timeout 10m \
  --mount /tmp/gts-c26ad-grammar_parity:/tmp/grammar_parity:ro -- \
  'export PATH=/usr/local/go/bin:$PATH; cd /workspace/cgo_harness && \
   go test -tags "cgo treesitter_c_parity" . \
   -run "^TestSQLGrammargenCGORegressionCases$" -count=1 -v'
```

Artifact:
`/tmp/gts-c26ag-ae6e-artifacts/20260824T010956Z-c26ag-ae6e-sql-generated-locked-c`.

- `container.log`: `34f6ef038f16fa22dfb0ac9edb7633e889b76a231c317f5788a4c7f7a3d03ed9`
- `metadata.txt`: `05132d52f87ef1fcf24d9759d2a97653764faa334db10350162df9524c907b01`
- `inspect.json`: `c61c48886aa2f6ea56af080fc5b742d464689b623dae4497fe946a2c14afc05e`

The generated identity matched the generated blob:
`4ffb2a6d09e2000126f10101db9028d28e0752ac3e4f83e401f045c3b028ca7c`.
The route reused one subtree and 16 bytes.
The stale identity route reused zero subtrees and zero bytes.
The identifier and parenthesized Boolean cases passed.
The dollar-quoted case failed with the known generated-vs-locked-C error
tree difference. The guard did not change that result.

Run the focused identity tests:

```sh
bash cgo_harness/docker/run_parity_in_docker.sh \
  --repo-root /tmp/gts-c26ag-table-identity-20260824 \
  --out-root /tmp/gts-c26ag-ae6e-artifacts \
  --label c26ag-ae6e-focused --no-build --memory 4g --cpus 1 \
  --goflags -p=1 --test-parallel 1 --timeout 10m -- \
  'export PATH=/usr/local/go/bin:$PATH; cd /workspace && \
   go test ./internal/parsercorephase0 \
   -run "^TestCoreTableIdentityCapturesAndRejectsProducerDrift$" -count=1 && \
   go test -tags gts_parsercorephase0 . \
   -run "^TestParserCore(ReplayRejectsLanguageTableSwap|RootTablesIdentityUsesLanguageBlobHash)$" -count=1 && \
   go test -race -tags gts_parsercorephase0 . \
   -run "^TestParserCoreReplayRejectsLanguageTableSwap$" -count=1 && \
   go test -tags gts_no_parsercorephase0 . -run "^$" -count=1'
```

Artifact:
`/tmp/gts-c26ag-ae6e-artifacts/20260824T010926Z-c26ag-ae6e-focused`.

- `container.log`: `2c5455352c780dfd865d2c38c112a7fd44c11912094ffe7abcd909a291121002`
- `metadata.txt`: `decaddbe79940108b3f7d14d2e31f942a364a78f7376fbb936482bf586719a39`
- `inspect.json`: `4832627711918dda32dd69b40c38bcf8f34b3a09a3571624c0c2084ac1fbeaf9`

The focused core and root tests passed.

The loaded-blob identity, race, and compile-out tests ran in the combined
focused command above. They passed in this artifact:
`/tmp/gts-c26ag-ae6e-artifacts/20260824T010926Z-c26ag-ae6e-focused`.

### C26ag decision and reopening condition

Accept the generic table-identity guard. Do not add SQL-specific symbol maps.
Keep the SQL compact route gated until generated SQL matches locked C on the
dollar-quoted witness and the replay contract covers all derivation metadata.

Reopen the SQL route only after a Docker gate proves all of these conditions:

- equal table identity permits replay;
- changed or missing identity declines before replay;
- scanner identity and checkpoint bytes remain exact;
- fresh, compact, incremental, and locked-C trees match; and
- the generated SQL dollar-quoted witness passes without a state remap.

## C26ah generated SQL dollar-quote producer blocker

C26ah used publication base
`d72987b44b76cf39aa4ad0f5fff03860eed7cd0d`.
The isolated worktree was `/tmp/gotreesitter-compact-next.oFLjU5`.

Status: **NO-GO / KEEP LIVE**. Keep SQL compact admission gated.
No parser, scanner, grammar, or compact admission change ships.
The receipt adds one focused probe, one changelog entry, and this section.

### Producer and witness

The pinned SQL producer is `m-novikov/tree-sitter-sql` at commit
`587f30d184b058450be2a2330878210c5f33b3f9`.
The source hashes are:

- `src/grammar.json`: `42f011860137175a5a0cb820d1a694e5ccca1d17f226729ff6a4e886910cde1c`.
- `src/scanner.cc`: `d437ad9f517d7a1f4248ccd05abe58370b5040c0037c877dab1f0aefeaa04af6`.

The generated language uses blob identity
`4ffb2a6d09e2000126f10101db9028d28e0752ac3e4f83e401f045c3b028ca7c`.
Its scanner identity is
`7e493677411a501e6d8592c6b9cc158e21a1bfed44c72ca914e2d81e4e34861d`.
The checked-in SQL blob identity is
`e21421cbab52b54cf5ba15c8f78a2bb4729bf4e8c0da14368069e897de451268`.

The witness is the 16-byte source `SELECT $$hey$$;\n`.
Its source SHA-256 is
`c65f30545110c37897fb7fe364af31ff572b35796963ec8fc59b37b76d319912`.
The generated tree digest is
`7ae91c65aebee3a10eeef68804af8b74bbf27620b200dfe842916898fc90dd46`.
The checked-in tree digest is
`f093882f4f27897036dd245c3e17f1dad2d7cd72e470e8594b5492150a2c451e`.
The locked-C tree digest is the same checked-in digest.

The first difference is `/source_file` child count.
The generated tree has three children. The checked-in and locked-C trees have
two children. The generated tree has an error. The other two trees are clean.
This difference starts in the generated producer output, before compact replay.

### Compact route evidence

The focused probe checks the generated identity before it parses the witness.
It checks the checked-in tree against the locked-C tree.
It also checks the generated tree against both references.
The probe is `cgo_harness/sql_c26ah_generated_dollar_quote_blocker_test.go`.

The compact candidate recorded `routed=0` and `fallback=1`.
Its exact fallback reason was:

```text
compact route declined at recovery [mechanism=recovery-entered]: did not accept EOF: generic scheduler has no table action for the elected token
```

The compact fallback tree equals the generated production tree.
It does not claim compact parity. It preserves the current fail-closed route.

### Focused Docker gate

Run one SQL grammar with one CPU and one Go test worker:

```sh
GOMAXPROCS=1 bash cgo_harness/docker/run_parity_in_docker.sh \
  --repo-root /tmp/gotreesitter-compact-next.oFLjU5 \
  --out-root /tmp/gotreesitter-c26ah-artifacts \
  --label c26ah-sql-blocker-gomax1 --no-build --memory 4g --cpus 1 \
  --gomemlimit 3GiB --goflags -p=1 --test-parallel 1 --timeout 10m \
  --mount /tmp/gotreesitter-sql-seed.ada3CT:/tmp/grammar_parity:ro -- \
  'cd /workspace/cgo_harness && \
   go test -tags "cgo treesitter_c_parity" . \
   -run "^TestSQLC26ahGeneratedDollarQuoteBlocker$" -count=1 \
   -parallel=1 -timeout=10m -v'
```

The run passed with exit code zero.
It used one SQL grammar, one CPU, `GOMAXPROCS=1`, and `GOFLAGS=-p=1`.
It reported no out-of-memory kill and no wall timeout.

The artifact is
`/tmp/gotreesitter-c26ah-artifacts/20260824T015950Z-c26ah-sql-blocker-final2`.

- `container.log`: `40e12556fdbf0c806e1d54b71e59a0566f8e6eab09f62d9ceeb37742f7ce4d88`.
- `metadata.txt`: `cd0c53681a3ae278e5ad6d2b26609952a868efa0550715803c9ed5dfe49eb01c`.
- `inspect.json`: `f61f7fe393588d334610829acee2d3cfa34d892f7ed098c0be2f02c96e4f908b`.

The existing strict grammargen regression still fails on this witness.
The failure is the expected generated-versus-locked-C producer difference.
The new receipt makes that blocker and the compact fallback explicit.

### Rebase applicability

Remote main added the Perl dispatcher receipt and related documentation after
the original C26ah base.
It did not change the SQL grammar lock, SQL blob, SQL scanner, grammargen
inputs, parser core, or SQL parity test.
The current hashes match the `6eed698a13e7371fa978adb893e8b89ad1cd81ba`
hashes for every SQL executable input.
The original focused artifact therefore remains applicable after this rebase.
No Docker rerun was required.

### C26ah decision and reopening condition

Keep the SQL route live and gated. Do not add a parser-state remap.
Do not add a SQL-specific symbol map.

Reopen only after a producer or grammar revision makes the generated tree
match the checked-in and locked-C trees for this witness.
Then rerun the strict grammargen parity test and the compact route gate.
Require equal table identity, scanner identity, fresh-tree equality, compact
equality, incremental equality, and locked-C equality.

No performance gate ran for C26ah. This receipt makes no performance claim.

## C26ai generated SQL tagged dollar-quote producer witness

C26ai used publication base
`7a14a701eb2a5d623ce128e792ee67820a734c8b`.
It follows the C26ah untagged dollar-quote receipt.

Status: **NO-GO / KEEP LIVE**. Keep SQL compact admission gated.
This slice adds one tagged producer witness, one focused Docker probe, one
changelog entry, and this section. It adds no parser or route change.

### Producer and witness

The pinned SQL producer is `m-novikov/tree-sitter-sql` at commit
`587f30d184b058450be2a2330878210c5f33b3f9`.
The grammar source SHA-256 is
`42f011860137175a5a0cb820d1a694e5ccca1d17f226729ff6a4e886910cde1c`.
The scanner source SHA-256 is
`d437ad9f517d7a1f4248ccd05abe58370b5040c0037c877dab1f0aefeaa04af6`.

The generated blob identity is
`4ffb2a6d09e2000126f10101db9028d28e0752ac3e4f83e401f045c3b028ca7c`.
The scanner identity is
`7e493677411a501e6d8592c6b9cc158e21a1bfed44c72ca914e2d81e4e34861d`.
The checked-in SQL blob identity is
`e21421cbab52b54cf5ba15c8f78a2bb4729bf4e8c0da14368069e897de451268`.

The witness is the 22-byte source `SELECT $tag$hey$tag$;\n`.
Its source SHA-256 is
`1279d93f715690fee6c8af53fa774d0108c19846d9418d86d53edec0d743bc88`.
The generated tree digest is
`ba05964c2f2c62e56a3ee9470c76dc55098f7c2ea4f656607da30c5f8af212d4`.
The checked-in and locked-C tree digest is
`824c8bdf7107be3632bd0a43d89e0324ebc0802dcd8c4dca014130224f930ef6`.

The generated tree has three root children and an error.
The checked-in and locked-C trees have two root children and no error.
The first divergence is:

```text
root: ChildCount go=3 c=2 (goType="source_file" cType="source_file" goBytes=[0-22] cBytes=[0-22])
```

The difference starts in generated producer output, before compact replay.
The tagged witness extends the same producer boundary shown by C26ah.

### Focused Docker gate

The probe is
`cgo_harness/sql_c26ai_generated_tagged_dollar_quote_test.go`.
It checks generated identity, checked-in parity, and locked-C parity.
Run one SQL grammar with one CPU and one Go test worker:

```sh
GOMAXPROCS=1 bash cgo_harness/docker/run_parity_in_docker.sh \
  --repo-root /tmp/gotreesitter-graduation-next.VP39zH \
  --out-root /tmp/gotreesitter-c26ai-artifacts \
  --label c26ai-sql-tagged-dollar-quote-final --no-build --memory 4g --cpus 1 \
  --gomemlimit 3GiB --goflags -p=1 --test-parallel 1 --timeout 10m \
  --mount /tmp/gotreesitter-sql-seed.ada3CT:/tmp/grammar_parity:ro -- \
  'cd /workspace/cgo_harness && \
   go test -tags "cgo treesitter_c_parity" . \
   -run "^TestSQLC26aiGeneratedTaggedDollarQuote$" -count=1 \
   -parallel=1 -timeout=10m -v'
```

The run passed with exit code zero.
It used one SQL grammar, one CPU, `GOMAXPROCS=1`, and `GOFLAGS=-p=1`.
It reported no out-of-memory kill and no wall timeout.

The artifact is
`/tmp/gotreesitter-c26ai-artifacts/20260824T024111Z-c26ai-sql-tagged-dollar-quote-final`.

- `container.log`: `94ebf803c12c2381c67467de3d4041c61b2513e420335983a22bdab74ebd5fce`.
- `metadata.txt`: `496d6ff1881e87f6c09f571f9995cdf6aee25d9260c7581fae1c1087dece92d3`.
- `inspect.json`: `69210f60086dd2c2510c374d6bc928b7ce90206d523f0ce248895ba806583978`.

The generated tree remains different from locked C.
The probe records a producer blocker, not a compact parity claim.

### Rebase applicability

The C26ah receipt and the intervening Julia, highlighting, and recovery
receipts are present on this base.
The SQL grammar lock, SQL blob, SQL scanner, parser, and grammargen inputs
did not change between `d72987b44b76cf39aa4ad0f5fff03860eed7cd0d` and this base.
The C26ai artifact therefore remains applicable after this rebase.
No Docker rerun was required.

### C26ai decision and reopening condition

Keep SQL compact admission gated. Do not add a parser-state remap.
Do not add a SQL-specific symbol map.

Reopen only after a producer or grammar revision makes both dollar-quote
witnesses match the checked-in and locked-C trees.
Then rerun the C26ah and C26ai Docker probes and the strict grammargen gate.
Require equal table identity, scanner identity, fresh-tree equality, compact
equality, incremental equality, and locked-C equality.

No performance gate ran for C26ai. This receipt makes no performance claim.

## C26aj generated SQL `CREATE DOMAIN` producer witness

C26aj used publication base
`83e0cfbc30ad82e2f327d58e35eea9f438a0ffda`.
It follows the C26ai tagged dollar-quote receipt.

Status: **NO-GO / KEEP LIVE**. Keep SQL compact admission gated.
This slice adds one producer witness, one focused Docker probe, one changelog
entry, and this section. It adds no parser or route change.

### Producer and witness

The pinned SQL producer is `m-novikov/tree-sitter-sql` at commit
`587f30d184b058450be2a2330878210c5f33b3f9`.
The grammar source SHA-256 is
`42f011860137175a5a0cb820d1a694e5ccca1d17f226729ff6a4e886910cde1c`.
The scanner source SHA-256 is
`d437ad9f517d7a1f4248ccd05abe58370b5040c0037c877dab1f0aefeaa04af6`.

The generated blob identity is
`4ffb2a6d09e2000126f10101db9028d28e0752ac3e4f83e401f045c3b028ca7c`.
The scanner identity is
`7e493677411a501e6d8592c6b9cc158e21a1bfed44c72ca914e2d81e4e34861d`.
The checked-in SQL blob identity is
`e21421cbab52b54cf5ba15c8f78a2bb4729bf4e8c0da14368069e897de451268`.

The witness is the 19-byte source `CREATE DOMAIN test;`.
Its source SHA-256 is
`94c5d360b7205e2bd6e84fa28efc2cb3ee2cbc89aa6b759dd3349e578dd133c8`.
The generated tree digest is
`f08d628fa30d83cf92352dbfcab4885b7422ac0fadde9c756e747e6c116dc044`.
The checked-in and locked-C tree digest is
`e72fb9fb57180d5db5be9b649969c7e722d20e1ea4040075260258ee293ce630`.

Both generated and checked-in trees are clean with two root children.
The generated `CREATE_DOMAIN` node has one child. Locked C has no child.
The first divergence is:

```text
root[0][1]: ChildCount go=1 c=0 (goType="CREATE_DOMAIN" cType="CREATE_DOMAIN" goBytes=[7-13] cBytes=[7-13])
```

The difference starts in generated producer output, before compact replay.
The witness is the next smallest remaining SQL producer gap after C26ai.

### Focused Docker gate

The probe is
`cgo_harness/sql_c26aj_generated_create_domain_test.go`.
It checks generated identity, checked-in parity, and locked-C parity.
Run one SQL grammar with one CPU and one Go test worker:

```sh
GOMAXPROCS=1 bash cgo_harness/docker/run_parity_in_docker.sh \
  --repo-root /tmp/gotreesitter-graduation-next.EaYNNn \
  --out-root /tmp/gotreesitter-c26aj-artifacts \
  --label c26aj-sql-create-domain-final --no-build --memory 4g --cpus 1 \
  --gomemlimit 3GiB --goflags -p=1 --test-parallel 1 --timeout 10m \
  --mount /tmp/gotreesitter-sql-seed.ada3CT:/tmp/grammar_parity:ro -- \
  'cd /workspace/cgo_harness && \
   go test -tags "cgo treesitter_c_parity" . \
   -run "^TestSQLC26ajGeneratedCreateDomain$" -count=1 \
   -parallel=1 -timeout=10m -v'
```

The run passed with exit code zero.
It used one SQL grammar, one CPU, `GOMAXPROCS=1`, and `GOFLAGS=-p=1`.
It reported no out-of-memory kill and no wall timeout.

The artifact is
`/tmp/gotreesitter-c26aj-artifacts/20260824T053231Z-c26aj-sql-create-domain-final`.

- `container.log`: `d2aa7a24577775a055d838fd619f17fd7f559395ad76f89b078bd6dcd94b62cf`.
- `metadata.txt`: `7b1bdbc8fc7d1ac21e1692419a4eda2b066ec1003895b70f04e54824d8197ab0`.
- `inspect.json`: `3db73a6ff767692081b99beed02ccd9d93b37a78427c8f001a7ebbaa28a743a0`.

The generated tree remains different from locked C.
The probe records a producer blocker, not a compact parity claim.

### C26aj decision and reopening condition

Keep SQL compact admission gated. Do not add a parser-state remap.
Do not add a SQL-specific symbol map.

Reopen only after the SQL producer or grammar revision makes both
`CREATE DOMAIN` witnesses match the checked-in and locked-C trees.
Then rerun the C26aj gate with C26ah and C26ai coverage.
Require equal table identity, scanner identity, fresh-tree equality, compact
equality, incremental equality, and locked-C equality.

No performance gate ran for C26aj. This receipt makes no performance claim.

## C26ak generated SQL `CREATE DOMAIN AS` producer witness

C26ak used publication base
`83e0cfbc30ad82e2f327d58e35eea9f438a0ffda`.
It follows the queued C26aj `CREATE DOMAIN` receipt.

Status: **NO-GO / KEEP LIVE**. Keep SQL compact admission gated.
This slice adds one producer witness, one focused Docker probe, one changelog
entry, and this section. It adds no parser or route change.

### Producer and witness

The pinned SQL producer is `m-novikov/tree-sitter-sql` at commit
`587f30d184b058450be2a2330878210c5f33b3f9`.
The grammar source SHA-256 is
`42f011860137175a5a0cb820d1a694e5ccca1d17f226729ff6a4e886910cde1c`.
The scanner source SHA-256 is
`d437ad9f517d7a1f4248ccd05abe58370b5040c0037c877dab1f0aefeaa04af6`.

The generated blob identity is
`4ffb2a6d09e2000126f10101db9028d28e0752ac3e4f83e401f045c3b028ca7c`.
The scanner identity is
`7e493677411a501e6d8592c6b9cc158e21a1bfed44c72ca914e2d81e4e34861d`.
The checked-in SQL blob identity is
`e21421cbab52b54cf5ba15c8f78a2bb4729bf4e8c0da14368069e897de451268`.

The witness is the 27-byte source `CREATE DOMAIN test AS text;`.
Its source SHA-256 is
`0cad93f1b70ffa192008df866587c05b206ee404cedd5a0025f542c39c6a504b`.
The generated tree digest is
`67366d68529459613040eb60c5eef47b371fd3091dbe3283c82d7bcff287ea9c`.
The checked-in and locked-C tree digest is
`d0daa4279f00eb8c7278992cff6a870c98bf3deee2292b28f997adbe2f434916`.

Both generated and checked-in trees are clean with two root children.
The generated `CREATE_DOMAIN` node has one child. Locked C has no child.
The first divergence is:

```text
root[0][1]: ChildCount go=1 c=0 (goType="CREATE_DOMAIN" cType="CREATE_DOMAIN" goBytes=[7-13] cBytes=[7-13])
```

The difference starts in generated producer output, before compact replay.
This witness is distinct from C26ah, C26ai, and the queued C26aj witness.

### Focused Docker gate

The probe is
`cgo_harness/sql_c26ak_generated_create_domain_as_test.go`.
It checks generated identity, checked-in parity, and locked-C parity.
Run one SQL grammar with one CPU and one Go test worker:

```sh
GOMAXPROCS=1 bash cgo_harness/docker/run_parity_in_docker.sh \
  --repo-root /tmp/gotreesitter-graduation-next.e5Kf8Z \
  --out-root /tmp/gotreesitter-c26ak-artifacts \
  --label c26ak-sql-create-domain-as-final --no-build --memory 4g --cpus 1 \
  --gomemlimit 3GiB --goflags -p=1 --test-parallel 1 --timeout 10m \
  --mount /tmp/gotreesitter-sql-seed.ada3CT:/tmp/grammar_parity:ro -- \
  'cd /workspace/cgo_harness && \
   go test -tags "cgo treesitter_c_parity" . \
   -run "^TestSQLC26akGeneratedCreateDomainAs$" -count=1 \
   -parallel=1 -timeout=10m -v'
```

The run passed with exit code zero.
It used one SQL grammar, one CPU, `GOMAXPROCS=1`, and `GOFLAGS=-p=1`.
It reported no out-of-memory kill and no wall timeout.

The artifact is
`/tmp/gotreesitter-c26ak-artifacts/20260824T054920Z-c26ak-sql-create-domain-as-final`.

- `container.log`: `50452ea4d162b689cf5fcf59233f055b6715f8acdc168ef28cd993aae694ef08`.
- `metadata.txt`: `44da1ae400911935701d1507342454873b6ecad7f1a9dd0b8d3c695f8e159f00`.
- `inspect.json`: `9faae0b5768c81811cbc843bcc02c1d3b01f664a4a86bd8446cd664557e31b05`.

The generated tree remains different from locked C.
The probe records a producer blocker, not a compact parity claim.

### C26ak decision and reopening condition

Keep SQL compact admission gated. Do not add a parser-state remap.
Do not add a SQL-specific symbol map.

Reopen only after the SQL producer or grammar revision makes both
`CREATE DOMAIN` witnesses match the checked-in and locked-C trees.
Then rerun the C26ak gate with C26ah, C26ai, and the queued C26aj coverage.
Require equal table identity, scanner identity, fresh-tree equality, compact
equality, incremental equality, and locked-C equality.

No performance gate ran for C26ak. This receipt makes no performance claim.

## Corpus state

The current manifest has these properties:

- 50 declared languages
- 50 languages with selected files
- 147 files
- No missing files
- No size mismatches
- No SHA-256 mismatches
- No stale source commits
- No absolute output paths

The focused quality gate passes:

```sh
cd cgo_harness
GTS_REAL_CORPUS_MANIFEST=corpus_real/manifest.json \
  go test . -run '^TestRealCorpusManifestQuality$' -count=1
```

The corpus directory is ignored by Git.
Rebuild it from the exact profile before a new campaign.

Ordinary YAML with 16-byte graph nodes remains held: [three memory screens and Go graph-lifetime attribution](compact-yaml-node16-memory-screen-20261020.md) retain all 378 RSS processes. Ordinary RSS ratios 1.16443, 1.14617, and 1.16318 each fail the 1.10 ratchet despite 50 passing work comparisons per source; no timing or allowlist promotion follows. A separate Go census identifies retained GSS history as an architectural target, with other ownership roots still unproven.
