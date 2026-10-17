# Empty external modes: Go throughput win with unchanged YAML timing

Evidence date: 2026-10-16 UTC. Baseline:
`8e4fe35380423f94db6c03f42c917e85666ba6aa`. Buckley runtime commit:
`959121c6311d45f743a24aad0ae55b021cb56a04`.
All main merges and auto-merge remain held for 0.56.0.

The external-correction helper discards every internal DFA candidate. When
both the requested state's external row and ERROR_STATE's external row are
explicitly present and all false, no input can produce an external candidate.
Single-state scanning and its recovery retry therefore have a constant negative
result. The new internal predicate proves this before rerunning the DFA. It
retains the existing stateless, fresh-session, frozen-election, ownership and
checkpoint checks, and records the zero-read negative in the existing memo.
There is no input-prefix assumption or scanner-specific comparison.

This is a static table proof. The earlier observation that 7,290 failed probes
never widened an existing span is not the justification for removing reads.
A separate unchanged-runtime census finds both rows empty on 114/135,
405/547, 341/394 and 5,258/6,214 actual uncached failures in the four Go files.
The three YAML paths have no eligible probes. Invalid row IDs fail closed;
mode 65,535 stays representable. No new fields, table cache, allocation or
language-name check is added.

Review also exposes a separate 386 correctness bug: converting a high-bit
uint32 parser state to int can make the range check negative. The baseline
panics at state 0x80000000. The unsigned comparison rejects states 65,536,
0x80000000 and 0xffffffff without narrowing valid IDs. A baseline EOF
regression separately demonstrates the unnecessary speculative read. Both
new regressions pass natively and on 386 in the candidate, including cursor
preservation and failed checkpoint authentication.

## Deterministic gate before timing

All **414 valid ledger rows** stay exact, including all work/reuse counters.
The proper main-package owned export confirms compact acceptance with full /
first-edit nodes **484→484 / 484→484**. Canonical deep trees and Core.Work stay
exact. Of the 21 raw fields, only table-lookup work falls:

| Go file | Before | After | Change |
| --- | ---: | ---: | ---: |
| Rewrite | 7,553 | 7,537 | −0.212% |
| Query | 37,640 | 37,621 | −0.050% |
| Language | 37,479 | 37,436 | −0.115% |
| Large | 397,227 | 395,591 | −0.412% |

The other 20 raw fields stay exact. Focused Go/YAML locked-C comparisons,
48-step incremental=fresh sessions, no-edit zero-allocation checks, full core
native/race/phase-zero-A/386 suites, root lexer/relex, canonical, lifecycle and
memory-budget checks pass before timing. No ledger reset or owner exception
is needed.

## Twenty-pair timing

The repository runner uses Go seeds **25101–25120** and YAML seeds
**25201–25220**, alternating role order, one process per role/seed, count 1,
750 ms and benchmem, GOMAXPROCS=1, CPU 6, 6 GiB Docker and both shared locks.
All source/data hashes, sample counts, alternating orders and timed work
columns verify. The predicate stays out of line; both probe methods retain
the same 1,608-byte stack frame. No frame-size saving is claimed.

| Path | Before → after | Time change | p |
| --- | ---: | ---: | ---: |
| Compact Go rewrite | 1.765029 → 1.7256855 ms | **−2.23%** | <.001 |
| Compact Go query | 8.4527075 → 8.2935325 ms | **−1.88%** | .001 |
| Compact Go language | 8.736698 → 8.5451655 ms | **−2.19%** | <.001 |
| Compact Go large | 90.039699 → 87.614192 ms | **−2.69%** | <.001 |
| Default Go full | 3.844692 → 3.835370 ms | −0.24% | .738 |
| Default Go edit | 99.223 → 100.080 µs | **+0.86%** | <.001 |
| Default Go no-edit | 3.0745 → 3.0955 ns | **+0.68%** | .002 |
| Ordinary YAML attempt/fallback | 194.2815 → 194.3485 µs | +0.03% | .941 |
| Owned YAML R4 | 351.5015 → 352.0470 µs | +0.16% | .640 |
| Owned YAML 8 KiB stream | 1.369904 → 1.371902 ms | +0.15% | .529 |

All four compact Go gains are significant. Every YAML change and default Go
full change is inconclusive. The significant default edit/no-edit control
regressions remain reported; they stay within the timing ratchet and neither
path uses the new guard. There is no control-path speed claim. Allocated
objects remain **21 / 129 / 243 / 1,716** on compact Go, **6 / 5 / 0** on the
default trio and **37 / 109 / 336** on YAML. Byte medians stay exact except
large Go 3,251,743→3,251,726 B/op; that tiny difference is not a memory win.
No-edit remains **0 B/op / 0 allocations**. Earlier bucket percentages are
independent comparisons and must not be added to these results.

An earlier inlined form, measured independently with Go seeds 24901–24920
and YAML 25001–25020, improves compact Go 1.49–2.02% but regresses ordinary
YAML **2.23% (p<.001)**. It is rejected. The final source factors the immutable
eligibility check once and keeps the pure predicate out of line. Exact source
reconstruction verifies the same logic, and both regression tests are byte
identical. No timing result is transferred from the rejected source.

## Final qualification and limits

Final source passes all 206 language invariant sessions plus the owned-YAML session, zero no-edit allocations, 618 C fixture checks and focused canonical Go/YAML C/edit checks. The 204-row strict census exactly matches fresh baseline: 159 PASS / 45 existing FAIL, with all 120 actual compact acceptances C-exact. Core/root ownership races, phase-zero-A, EOF shadow, full core 386 and root 386 regression tests pass. The 1,049,572-byte / 524-document owned-YAML Go+C safety probe is compact-only, complete, error-free and C-exact at 143,300 KiB process RSS, below 400 B/source byte. Large canonical Go peaks at 121,056 KiB. These single probes do not establish an RSS improvement.

Post-copy source hashes exactly match timing and fleet inputs; all 3,546 other native and 296 data inputs stay unchanged. Lexrun and one-pass selected-store races, canonical/owned484/lifecycle/root-386 tests, full core 386 and library 386 build pass. L0 stays 756 root files / 12 API tag sets; R6 stays 454 language-name uses / 101 environment reads. CI package coverage passes and all 64 selectors resolve across seven workflows and 7,698 defined functions.

The widened root test selection exposes an existing Scala limitation in
`TestDiagnosticParserCoreStateDependentRelexKeepsExactSpanBranch`: deeper S3
recovery at election 50, byte 145. Unchanged 8e4 and the first candidate fail
with byte-identical stop/token/header/stats/work evidence. This test is not
changed, removed or exempted. The focused affected-family gates and the fleet
gates are reported separately; no all-root-suite pass is claimed.

Two completed bounded Buckley source-only reviews cover the full Next/external
retry proof and bounds. The first packet omitted the ERROR_STATE declaration;
a follow-up verifies that it is zero, so any valid unsigned state proves that
index present. The audit independently catches the real 386 conversion bug.
The final compiler-outline/eligibility factoring is source-equivalent to the
reviewed logic. These reviews do not execute verification or approve a merge.

This is partial progress on #1319. Ordinary YAML routing/ownership, compact
allowlists and the E-A graduation gate stay unchanged. Source, counter exports,
completed checks, shuffled samples, benchstat, review/equivalence and memory
audits remain in local `gotreesitter-evidence/20261016-empty-external-modes-outlined`.
The rejected source and baseline red-test evidence remain in
`20261016-empty-external-modes`; the static eligibility census remains in
`20261016-empty-external-mode-census`. Reproduce against 8e4, before copying
the candidate over the physical checkout.
