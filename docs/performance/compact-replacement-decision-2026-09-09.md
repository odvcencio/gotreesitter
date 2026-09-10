# Compact parser replacement decision — historical milestone

The later [suffix-reuse pass](compact-suffix-reuse-2026-09-09.md) closes the required lifecycle cost gap.
It also adds native length changes. General replacement remains on HOLD for the remaining route and API gates.
The later [parent-publication follow-up](compact-parent-publication-2026-09-10.md) fixes the deletion-history parent-link defect. The historical measurements below remain unchanged.

The measurements below describe the earlier milestone candidate.

**1. Milestone assessment: HOLD.**

The bounded certificate passes its focused checks and enables three native histories.
The final candidate still fails every required wall-time gate against repaired legacy.
Certificate validation consumed less than 0.5% of sampled CPU time in the attribution profiles.
The remaining gap comes from additional parsing, rebuilding, materialization, and scheduler accounting.

The profiles show that certificate validation is no longer the main cost.
They do not prove that closing the remaining gap requires a substantial redesign.
The earlier NO-GO classification overstated that inference. Graduation remains blocked by measured costs and incomplete route proofs.

Start: 2026-09-09 08:43 UTC. Deadline: 12:43 UTC.
Starting revision: 90bdc698d898f7ebb5e41e6bb54844224e480f16.
Decision recorded: 09:44 UTC, 61 minutes after the start.
The milestone stops after preserving this decision. No automatic second run is authorized.

**2. What became native**

Token-class change, early newline, and prefix call/conversion each completed 20 alternating edits.
All 60 results matched fresh C after releasing the old tree.
Each result preserved public parent navigation and reported actual subtree reuse.
The initial trees came from compact. No legacy entry or fallback executed in these histories.

**3. What still enters legacy**

Length change and deletion each entered legacy for all 20 edits.
Length change fails the active-ancestry certificate.
Prior scoped tracing identified an active alternative set containing 28 entries.
Deletion reaches a no-action recovery boundary after borrowing.
Recovery cannot yet select through the opaque borrowed derivation.

**4. Correctness**

The core suite, bounded-work, mutation, rollback/ID-reuse, reset, cancellation, and memory-budget tests passed.
The small compact edit matrix passed.
All 447 broader Go test/subtest outcomes match the repaired baseline.
Existing forest, new/make, probe, and included-range failures remain.

The stronger navigation test found shared legacy defects: 20 length-change and 10 deletion results had incorrect parent links.
Both lanes reproduced those failures. All 100 edit results still matched C digests.
These shared defects do not affect the three required native histories.
Stateful scanners, complete recovery/range coverage, and the full public API remain unverified.

**5. Cost**

Each operation includes compact initial parsing, four edits, old-tree releases, and final release.
Results use 20 paired seeds and fresh processes on pinned CPU 18.
The container uses one CPU, an 8 GiB limit, and GOMAXPROCS=1.

| History | Legacy median ms | Candidate median ms | Paired time ratio [95% CI] |
| --- | ---: | ---: | ---: |
| Token-class change | 20.40 | 27.75 | 1.367 [1.276, 1.457] |
| Early newline | 215.73 | 255.06 | 1.160 [1.110, 1.219] |
| Prefix call/conversion | 52.54 | 67.69 | 1.352 [1.285, 1.435] |
| Length change | 20.68 | 26.93 | 1.309 [1.241, 1.418] |
| Deletion recovery | 10.07 | 10.55 | 1.102 [1.050, 1.168] |

The required ceiling is 1.10 at the upper interval bound.
CI means confidence interval. Ratios use paired geometric means; displayed lane times use sample medians.
Final-run variance widened the intervals. All three comparison rounds failed the required timing gates.

Allocated bytes and released heap are below legacy for the three native histories.
Incremental arena sizing reduced token-change released heap from about 15.4 MiB to 10.0 MiB.
Legacy retained about 13.6 MiB at that same checkpoint.
The deep-prefix allocation fixture improved from 4,155 allocations to 20, with unchanged derivations.
Token and deletion retention plateaued through 20 cycles.
Draining released pools reduced both lanes to approximately 3.28 MiB.

**6. Retirement implications**

No legacy entry point can be deleted.
The certificate removes repeated ancestry-map construction for the three demonstrated histories.
The route ledger still contains unresolved lineage, recovery, and scanner proofs.
The efficiency fixes are useful independently of retirement.

**7. Next action**

The original milestone ended with its evidence preserved.
The user subsequently authorized another optimization pass on 2026-09-09.
Investigate excess token consumption and node construction before choosing broader changes.
Retain the certificate, arena sizing, derivation allocation, and repaired legacy improvements.
Keep retirement blocked until the required correctness and cost gates pass.

See compact-replacement-evidence-2026-09-09.md and compact-retirement-boundary-2026-09-09.md.
The evidence archive contains remaining-gates.md and reproduction.md.
Raw results, source identities, binary hashes, patches, and failure controls accompany this report.
