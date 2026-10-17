# Ordinary-link validation throughput receipt

Evidence date: 2026-10-16 UTC. Baseline:
`14f255bac37e64670b90b3120534b505ffeb6a1e`. Qualified runtime:
[`0808dfd5fbfb1367bf4d8b80ff0b5ceb897fda3c`](https://github.com/odvcencio/gotreesitter/commit/0808dfd5fbfb1367bf4d8b80ff0b5ceb897fda3c),
in [core PR #1485](https://github.com/odvcencio/gotreesitter/pull/1485).
All merges to main remain held for 0.56.0.

## Change and correctness boundary

Ordinary links with a nonzero payload and flags 0 or `HasOrder` now return
directly from shape validation. Every other shape calls the original validator,
whose body and error order remain unchanged. Scores, orders and identities
retain their full widths. A nonzero order without `HasOrder` remains valid;
identity bounds remain the responsibility of the existing callers.

The original method costs 101 compiler inlining units, above the budget of 80.
The common method now costs 74 and inlines at its callers. This changes no
record layout, storage budget, parse action or routing policy. The new test
compares 4,275 field combinations against the unchanged validator, including
each unknown flag bit, recovery shapes and scalar extrema. It checks both
acceptance and exact error text, rather than treating malformed links as
ordinary links.

Buckley's completed supplied-source review finds no concrete defect. Its first
attempt could not read evidence outside the workspace; the retry reads the
same frozen bundle inside the workspace. That temporary copy is removed.
The review does not independently verify source hashes, execute tests, or
certify timing. The runtime and evidence audits provide those separate checks.

## Completed twenty-pair measurements

The repository randomized runner alternates baseline and candidate across
Go seeds 20401–20420 and YAML seeds 20501–20520. Each process uses count 1,
750 ms, `-benchmem`, `GOMAXPROCS=1`, Docker CPU 6 with a 6 GiB limit and both
repository locks. All samples, seed order, frozen source hashes and work
columns are audited before `benchstat`. These comparisons have their own
baseline and must not be added to earlier wins.

| Case | Before | Qualified source | Parse-time change |
| --- | ---: | ---: | ---: |
| Go rewrite | 1.832104 ms | 1.814277 ms | −0.97%, p<.001 |
| Go query | 8.802520 ms | 8.694512 ms | −1.23%, p<.001 |
| Go language | 9.136778 ms | 9.054039 ms | −0.91%, p<.001 |
| Large Go | 94.297120 ms | 92.817029 ms | −1.57%, p<.001 |
| Ordinary YAML attempt and fallback | 194.4885 µs | 194.5205 µs | +0.02%, p=.461, inconclusive |
| Diagnostic-owned YAML R4 | 353.2365 µs | 349.3590 µs | −1.10%, p=.021 |
| Diagnostic-owned YAML 8 KiB stream | 1.368785 ms | 1.359043 ms | −0.71%, p=.007 |

Allocation counts stay 21 / 129 / 243 / 1,716 for Go and 37 / 109 / 336
for YAML. Go bytes stay 1,776 / 138,725 on rewrite/query; language rounds
284,629→284,630 and large Go moves 3,251,743→3,251,712. YAML bytes stay
16,760 / 12,872 on fallback/owned R4; stream bytes round 44,312.5→44,313.

The default full/edit/no-edit controls move −0.71% / −0.73% / +1.90%
(p<.001/.002/<.001). They bypass this validator, so these directions receive
no causal attribution. The no-edit direction is retained: 3.0785→3.1370 ns/op,
still 0 B/op and 0 allocs/op. Full/edit bytes stay 1,159 / 394 and allocations
stay 6 / 5.

## Counter, invariant and C gates

Fresh native and candidate ledgers match in all 412 ordinary rows and both
diagnostic-owned YAML rows, including routes, reuse and stops. No counter
exception or reset is requested. Owned YAML full/first-edit nodes remain
484→484 each and tokens remain 102→102 each. The owned first-edit row still
uses `legacy_fallback`; these measurements establish no compact-edit win.

All four canonical Go deep-tree and complete `Core.Work` pins stay exact.
All 21 raw work-count fields match. Raw lookups remain
7,582 / 37,662 / 37,652 / 400,160 for rewrite/query/language/large Go.
All benchmark work and route columns stay exact as well.

All 206 R4 edit-session invariant processes pass, plus an additional owned-YAML
session. They check incremental=fresh, root/error/stop consistency and zero
no-edit allocations. Focused Go and YAML locked-C checks and 48-step candidate
edit sessions also pass. The 618 C fixture checks pass without skips.

Fresh baseline and candidate strict censuses match in all 204 real-file rows:
159 pass and 45 retain their existing fallback/custom-source failures. All 120
actual compact acceptances match C. This is not grammar-wide C certification,
and the existing failures are not waived.

The complete 1,049,572-byte owned-YAML probe remains compact-routed,
error-free, full-span and C-exact. Its Go+C process peak RSS is 146,076 KiB,
below the 400 B/source-byte safety limit. The canonical large-Go process peak
is 120,804 KiB. These are single safety probes, not evidence of memory savings.

Full parser-core, EOF-shadow, race, phase-zero-A, 386 and lifecycle/memory-stop
checks pass. The actual copied source also passes selected-store race, a 386
library build, L0 (756 root Go files and 12 API sets), R6 (454 language-name uses
and 101 environment reads), and 64 CI selectors against 7,687 functions.
Post-copy and post-commit hashes match the timed files exactly.

## Scope and retained evidence

This qualifies a small Go and diagnostic-owned YAML throughput win. Ordinary
YAML routing, the allowlist and the global default remain unchanged. E-A2 and
E-A7 remain open; the compact graduation gate is not satisfied by this receipt.
The [larger packed-link experiment](compact-link-storage-screens-20261016.md)
is rejected under its own completed comparison.

Frozen sources, review attempts, raw pairs, `benchstat` reports, 414-row
comparisons, per-language logs, strict censuses, memory probes and source audits
are retained under `20261016-link-shape-fast-path` in
`/home/draco/.local/state/gotreesitter-evidence/`. Timing uses the committed
`BenchmarkParserCoreFreshFullCanonical` and primary Go trio, followed by the
three `BenchmarkAdmission{R4Compact,R4OwnedExternalLexer,YAMLDocumentStreamOwned}`
cases with `GTS_ADMISSION_REAL_CORPUS_LANGS=yaml`.
