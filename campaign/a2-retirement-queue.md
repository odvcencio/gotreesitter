# A2 — Retirement queue: retirable-now ranking and next-3 queue

Campaign prong A, artifact 2. Evidence-only; changes no parser, registry, or
test behavior. Companion to `campaign/a1-live-arm-inventory.md`.

## Ranking basis

Every one of the 32 live registry entries is currently **blocked**:

- 21 entries have a dated dispatcher blocker receipt in
  `docs/root-normalization-retirement.md`, each ending `KEEP LIVE / NO-GO`.
- 11 entries (`javascript`, `julia`, `kotlin`, `perl`, `php`, `powershell`,
  `scala`, `sql`, `swift`, `yaml`, `predicate.cobol-exact`) have no dedicated
  receipt and therefore cannot be retired under the program's rules ("Rules
  for every retirement PR", `docs/root-normalization-retirement.md`): a
  current firing witness or a filed defect is required for every live arm,
  plus exact route receipts before any arm deletion.

"Retirable-now" below therefore means *closest to retirement*: the smallest
remaining gap measured against the standard receipt checklist, ranked by the
evidence profile stated in each entry's own blocker receipt.

## Full classification — all 32 live entries

Classification rule applied per entry: **retirable-now** requires either (a)
a current firing witness plus exact route receipts under the "Rules for every
retirement PR", or (b) no blocker receipt but a completed focused probe
proving native ownership. Neither condition holds for any live entry today,
so **zero entries are retirable-now and all 32 are blocked**, split by block
type:

### Blocked A — dated blocker receipt ends NO-GO / KEEP LIVE (21 entries)

| # | Entry ID | Block (citation: receipt in `docs/root-normalization-retirement.md`) |
| -- | --- | --- |
| 1 | `dispatch.ada` | Derivation election wrong on seven clean witnesses; production rewrites seven, fails parity on two malformed ("2026-08-22 Ada blocker receipt") |
| 2 | `dispatch.apex` | Full-route native ownership unproven despite four-witness A3 sweep ("2026-08-22 Apex blocker receipt") |
| 3 | `dispatch.authzed` | 17 + 11 production rewrites needed; no safe shared producer invariant ("2026-08-24 Authzed dispatcher blocker receipt") |
| 4 | `dispatch.awk` | Retirement gated on exact five-route output for every witness ("2026-08-24 AWK dispatcher blocker receipt") |
| 5 | `dispatch.bitbake` | Two error roots remain despite clean zero-rewrite A0 profile ("2026-08-24 BitBake blocker receipt") |
| 6 | `dispatch.c_cpp` | C recovery gap + incremental token-source divergence + absent corpus lock; six reopen conditions ("2026-08-24 C and C++ dispatcher blocker receipt") |
| 7 | `dispatch.c_sharp` | Generic-scheduler emission unproven; authenticated corpus unavailable ("2026-08-24 C# dispatcher blocker receipt") |
| 8 | `dispatch.cooklang` | Full-route native ownership unproven ("2026-08-24 Cooklang dispatcher blocker receipt") |
| 9 | `dispatch.corn` | No complete six-route locked-C receipt yet ("2026-08-23 Corn blocker receipt") |
| 10 | `dispatch.dart` | Route receipts don't prove native ownership; corpus absent; scanner-reuse receipt open ("2026-08-23 Dart dispatcher blocker receipt") |
| 11 | `dispatch.doxygen` | Known locked-C divergences documented ("2026-08-22 Doxygen blocker receipt"; "2026-08-23 Doxygen dispatcher blocker receipt") |
| 12 | `dispatch.dtd` | Two named divergences must close ("2026-08-22 document type definition (DTD) blocker receipt") |
| 13 | `dispatch.go` | Three live subpasses enumerated ("2026-08-24 Go dispatcher blocker receipt") |
| 14 | `dispatch.hlsl` | Two named live members remain ("2026-08-22 HLSL blocker receipt") |
| 15 | `dispatch.python` | Both f-string gaps open; corpus and lock absent; excluded from A0 denominator ("2026-08-24 Python dispatcher blocker receipt") |
| 16 | `dispatch.rust` | Zero-rewrite on all 23 witnesses but route-complete ownership unproven ("`dispatch.rust` blocker receipt — 2026-08-22") |
| 17 | `dispatch.solidity` | Corpus lock/directory absent; listed divergences incl. malformed controls open; five reopen conditions ("2026-08-23 Solidity dispatcher blocker receipt") |
| 18 | `dispatch.templ` | Error-root route diverges from locked C after 53 rewrites; compact falls back everywhere ("2026-08-24 Templ dispatcher blocker receipt") |
| 19 | `dispatch.typescript` | Exact all-route output unproven ("2026-08-24 TypeScript dispatcher blocker receipt") |
| 20 | `dispatch.wgsl` | 171 A0 rewrites; locked-C shape/type/error divergences; corpus lock absent (N31R probe receipt, "2026-08-24 WGSL dispatcher blocker receipt") |
| 21 | `dispatch.wolfram` | Three error roots; producer, incremental reuse, and corpus work all outstanding ("2026-08-24 Wolfram blocker receipt") |

### Blocked B — no dedicated receipt, so retirement cannot even be attempted (11 entries)

Under "Rules for every retirement PR", every live arm needs a current firing
witness or filed defect plus exact route receipts before any deletion; with no
receipt, there is not even a measured gap to close. Baseline coverage only
(`evidence_scope: baseline_corpus_wide_only`, per A1):

| # | Entry ID | Note |
| -- | --- | --- |
| 22 | `dispatch.javascript` | Residual arm after partial retirements (PR #459; dynamic-import subpass); needs fresh focused probes to rank |
| 23 | `dispatch.julia` | No receipt; baseline coverage only |
| 24 | `dispatch.kotlin` | Residual arm after interpolated-call subpass retirement |
| 25 | `dispatch.perl` | No receipt; baseline coverage only |
| 26 | `dispatch.php` | No receipt; baseline coverage only |
| 27 | `dispatch.powershell` | Four local repair walks live; no receipt |
| 28 | `dispatch.scala` | Residual arm after span/duplicate-call repairs retired |
| 29 | `dispatch.sql` | Three local repairs live; no receipt for residual arm |
| 30 | `dispatch.swift` | Residual arm after ternary subpass retirement |
| 31 | `dispatch.yaml` | One local repair (recovered root); no receipt |
| 32 | `predicate.cobol-exact` | Predicate entry; no receipt; baseline coverage only |

Cross-check: Blocked A (21) + Blocked B (11) = 32 =
`denominator.live_entries`. Every A1 row is classified exactly once.

Within Blocked A/B above, "closest to retirement" ranking is below; no entry
crosses into retirable-now until its full checklist closes.

## Ranked queue (top candidates)

| Rank | Entry | A0/census profile (from its receipt) | Remaining gap |
| -- | --- | --- | --- |
| 1 | `dispatch.corn` | 792 visited nodes, zero error roots, zero rewrites — cleanest zero-rewrite A0 profile among receipted arms ("2026-08-23 Corn blocker receipt") | No complete six-route locked-C receipt yet; authenticated corpus coverage unproven |
| 2 | `dispatch.bitbake` | 40,358 visited nodes, zero rewrites, two error roots ("2026-08-24 BitBake blocker receipt") | Two error-root witnesses must close; full-route receipts missing |
| 3 | `dispatch.apex` | Four witnesses incl. A3 certification sweep; "strongest remaining focused route evidence" at selection time; no clean-route divergence in existing classic-route tests ("2026-08-22 Apex blocker receipt") | Native ownership not proven across all routes; focused locked-C route receipt incomplete |

Excluded from the top 3, with reason:

- `dispatch.wolfram` — three error roots and only 77 visited nodes; producer
  work required before routes can even be measured.
- `dispatch.templ`, `dispatch.wgsl` (N31R), `dispatch.cooklang`,
  `dispatch.python`, `dispatch.c_sharp`, `dispatch.dart`, `dispatch.solidity`
  — all carry hard external blockers: absent authenticated corpus lock and/or
  open locked-C divergences with substantial rewrite counts.
- `dispatch.c_cpp`, `dispatch.dtd`, `dispatch.hlsl` — multiple named live
  members/divergences; largest remaining repair surface.
- The 11 unreceipted arms — a blocker receipt must exist first; several are
  already partially retired (Swift ternary, JavaScript dynamic-import, Kotlin
  interpolated-call subpasses are retired history), so their residual arms
  need fresh focused probes before they can rank.

## Next-3 queue with per-entry receipt checklists

Receipt vocabulary follows `docs/root-normalization-retirement.md` exactly:
**compatibility-free producer**, **production**, **compact fallback**,
**forest**, **incremental reuse**, **isolated C-oracle parity**.

### Queue 1 — `dispatch.corn`

Arm to retire: `normalizeCornCompatibility` in `parser_result_corn.go`,
called from the `runLanguageResultCompatibility` dispatcher.
Registry ID: `dispatch.corn`. Owner: `scheduler_action_semantics`.

Checklist:

1. [ ] Compatibility-free producer: prove native reduction emits the locked-C
       tree for every registered witness with zero `dispatch.corn` rewrites
       (extend the existing zero-rewrite A0 census).
2. [ ] Production route receipt: exact raw/production digests per witness.
3. [ ] Compact fallback receipt: record compact admission or documented
       fallback for every witness.
4. [ ] Forest receipt: exact forest output or documented decline per witness.
5. [ ] Incremental reuse receipt: nonzero reuse or documented unsupported
       reason, per witness, fresh and edited.
6. [ ] Isolated C-oracle parity: locked-C digest equality for every witness on
       every covered route.
7. [ ] Authenticated corpus coverage: add corn fixtures to the tracked census
       denominator (`testdata/dispatcher_census_tracked_v1.json`) or supply an
       authenticated corn receipt.
8. [ ] Delete the switch arm + `parser_result_corn.go`; update
       `testdata/result_compat_ownership_v1.json` (move entry to retired with
       commit + receipt references), the dispatcher-census ratchet test, and
       `TestResultCompatibilityOwnershipRegistry`.

### Queue 2 — `dispatch.bitbake`

Arm to retire: `normalizeBitbakeCompatibility` in
`parser_result_bitbake.go`. Registry ID: `dispatch.bitbake`.
Owner: `scheduler_action_semantics`.

Checklist:

1. [ ] Compatibility-free producer: close the two error-root witnesses so
       native reduction emits the locked-C tree with zero rewrites.
2. [ ] Production receipt at the pinned grammar revision for every registered
       witness.
3. [ ] Compact fallback receipt per witness.
4. [ ] Forest receipt per witness.
5. [ ] Incremental reuse receipt per witness.
6. [ ] Isolated C-oracle parity for every witness and route.
7. [ ] Authenticated corpus lock/coverage receipt.
8. [ ] Registry retirement update + census ratchet + ownership test.

### Queue 3 — `dispatch.apex`

Arm to retire: `normalizeApexClassLiteralAccess` in
`parser_result_apex.go` (subpass `dispatch.apex.class-literal-alias`).
Registry ID: `dispatch.apex`. Owner: `derivation_election_selection`.

Checklist:

1. [ ] Compatibility-free producer: derivation election emits the class-literal
       alias reading natively for every registered witness (zero rewrites).
2. [ ] Production receipt extending the existing A3 certification sweep to all
       five route classes per witness.
3. [ ] Compact fallback receipt per witness.
4. [ ] Forest receipt per witness.
5. [ ] Incremental reuse receipt per witness.
6. [ ] Isolated C-oracle parity per witness and route.
7. [ ] Authenticated corpus coverage receipt.
8. [ ] Registry retirement update (entry has four witnesses:
       `cgo_harness/parity_cgo_test.go`,
       `cgo_harness/apex_generic_local_parity_test.go`,
       `cgo_harness/apex_a3_certification_sweep_test.go`,
       `grammars/apex_class_literal_election_native_regression_test.go`)
       + census ratchet + ownership test.

## Standing rule

No candidate leaves the queue until every checklist box is checked with a
dated receipt; a single divergent node, flag, digest, or route result reopens
the candidate (`docs/root-normalization-retirement.md`, "Rules for every
retirement PR").
