# A1 — Live result-compatibility arm inventory

Campaign prong A, artifact 1. Evidence-only inventory; this file changes no
parser, registry, or test behavior.

Sources (read-only):

- Mechanical source of truth:
  `testdata/result_compat_ownership_v1.json` (schema
  `gotreesitter/result-compat-ownership/v1`).
- Readable guide: `docs/compat-tier.md`.
- Receipt vocabulary and blocker receipts:
  `docs/root-normalization-retirement.md`.

## Denominator

Per `testdata/result_compat_ownership_v1.json` `denominator` block, frozen at
exact main commit `f5adfc7091bad6f8adb5088a32f3b2912561fc72` (see
`docs/compat-tier.md` "Current denominator"):

| Field | Value |
| --- | ---: |
| dispatcher_arms | 31 |
| dispatcher_languages | 33 |
| dispatcher_predicates | 1 |
| generic_passes | 0 |
| post_finalization_arms | 0 |
| post_finalization_languages | 0 |
| live_entries | **32** |
| retired_entries | 56 |

The 32 live entries name 35 language labels (`tsx`+`typescript` share one arm;
`c`+`cpp` share one arm); the predicate matches exactly `cobol` or `COBOL`.
Every live entry carries route coverage over production, compact, forest,
incremental (`shared_result_compatibility_tail`) plus
`c_oracle: curated_single_grammar_parity`, and
`evidence_scope: baseline_corpus_wide_only`.

## Blocker-receipt status legend

- **Receipt: NO-GO** — a dated dispatcher blocker receipt in
  `docs/root-normalization-retirement.md` ends `KEEP LIVE`; retirement is
  blocked by named, receipted conditions.
- **No dedicated receipt** — no dated dispatcher blocker receipt for this
  entry was found in `docs/root-normalization-retirement.md` as of this
  inventory. Baseline coverage only (`baseline_corpus_wide_only`: the
  dispatcher census `parser_result_test/dispatcher_census_test.go` and the
  C-oracle structural parity harness `cgo_harness/parity_cgo_test.go`, with
  the documented limitations of `live_evidence_baseline.limitations`).

## Inventory — all 32 live registry entries

### Dispatcher arms (31)

| # | Entry ID | Languages | Function(s) | File(s) | Owner | Registered repairs | Blocker receipt status |
| -- | --- | --- | --- | --- | --- | --- | --- |
| 1 | `dispatch.ada` | ada | `normalizeAdaCompatibilityWithCensus` | `parser_result_ada.go` | derivation_election_selection | subpasses `dispatch.ada.constraint-kind-election`, `dispatch.ada.aggregate-kind-election` | **Receipt: NO-GO** — "2026-08-22 Ada blocker receipt": raw route elects wrong derivation on seven clean witnesses; production pass rewrites seven witnesses and fails exact parity on two; two malformed witnesses fail parity. Reopen only when derivation election emits exact trees, zero rewrites on five routes, malformed witnesses match locked C. |
| 2 | `dispatch.apex` | apex | `normalizeApexClassLiteralAccess` | `parser_result_apex.go` | derivation_election_selection | subpass `dispatch.apex.class-literal-alias` | **Receipt: NO-GO** — "2026-08-22 Apex blocker receipt" (`KEEP LIVE`). Four witnesses incl. A3 certification sweep; classic-route tests report no clean-route divergence, but full-route native ownership unproven. |
| 3 | `dispatch.authzed` | authzed | `normalizeAuthzedCompatibility` | `parser_result_authzed.go` | scheduler_action_semantics | none (arm-level walk) | **Receipt: NO-GO** — "2026-08-24 Authzed dispatcher blocker receipt": `a0-small-localimport` needs 17 production rewrites, `recovery-use-directive` needs 11; root-shape mismatch uses different producer invariants; no safe shared producer invariant identified. |
| 4 | `dispatch.awk` | awk | `normalizeAwkCompatibility` | `parser_result_awk.go` | scheduler_action_semantics | none | **Receipt: NO-GO** — "2026-08-24 AWK dispatcher blocker receipt" (`KEEP LIVE / NO-GO`): adds one focused test; retirement requires exact production, compact, forest, incremental, and locked-C output for every registered witness. |
| 5 | `dispatch.bitbake` | bitbake | `normalizeBitbakeCompatibility` | `parser_result_bitbake.go` | scheduler_action_semantics | none | **Receipt: NO-GO** — "2026-08-24 BitBake blocker receipt" (`KEEP LIVE`): 40,358 A0 visited nodes, zero rewrites, but two error roots; selected after Corn. |
| 6 | `dispatch.c_cpp` | c, cpp | `normalizeCCompatibilityWithParser` | `parser_result_c.go` | derivation_election_selection | C++ subpass calls `normalizeCppMalformedClassFunctionDefinition` (per receipt text) | **Receipt: NO-GO** — "2026-08-24 C and C++ dispatcher blocker receipt" (`KEEP LIVE / NO-GO`): C recovery gap and token-source incremental divergence separate blockers; C++ field repair language-specific and incomplete; C/C++ absent from A0 manifest and tracked census; `corpus_sources.lock` absent from worktree. Six named reopen conditions. |
| 7 | `dispatch.c_sharp` | c_sharp | `normalizeCSharpCompatibility` | `parser_result_csharp.go` | scheduler_action_semantics | none | **Receipt: NO-GO** — "2026-08-24 C# dispatcher blocker receipt": probe does not prove the generic scheduler emits the C tree; authenticated corpus unavailable; recovery helpers own top-level chunks, namespaces, type declarations. |
| 8 | `dispatch.cooklang` | cooklang | `normalizeCooklangCompatibilityWithCensus` | `parser_result_cooklang.go` | scheduler_action_semantics | subpass `dispatch.cooklang.recovered-recipe` | **Receipt: NO-GO** — "2026-08-24 Cooklang dispatcher blocker receipt" (`KEEP LIVE / NO-GO`): changes only receipt doc, CHANGELOG, focused test; full-route native ownership unproven. |
| 9 | `dispatch.corn` | corn | `normalizeCornCompatibility` | `parser_result_corn.go` | scheduler_action_semantics | none | **Receipt: NO-GO** — "2026-08-23 Corn blocker receipt" (merged as PR #795 per Wolfram receipt cross-reference): cleanest A0 profile among receipted arms — 792 visited nodes, zero error roots, zero rewrites — but no complete six-route locked-C receipt yet. See A2/A3. |
| 10 | `dispatch.dart` | dart | `normalizeDartCompatibility` | `parser_result_dart.go` | derivation_election_selection | none | **Receipt: NO-GO** — "2026-08-23 Dart dispatcher blocker receipt" (`KEEP LIVE / NO-GO`): route receipts exist but "do not prove native ownership"; authenticated Dart corpus absent; six named reopen conditions incl. bounded scanner-reuse receipt through 256 KiB. |
| 11 | `dispatch.doxygen` | doxygen | `normalizeDoxygenCompatibility` | `parser_result_doxygen.go` | scheduler_action_semantics | none | **Receipts: NO-GO x2** — "2026-08-22 Doxygen blocker receipt" and "2026-08-23 Doxygen dispatcher blocker receipt" (`KEEP LIVE / NO-GO`); known locked-C divergences documented in the latter. |
| 12 | `dispatch.dtd` | dtd | `normalizeDTDCompatibility` | `parser_result_dtd.go` | scheduler_action_semantics | none | **Receipt: NO-GO** — "2026-08-22 document type definition (DTD) blocker receipt"; also held live by the "2026-08-22 normalization checkpoint". Two known divergences (`parser-produced-pe-reference-trigger`; historical sources) must close before reopening. |
| 13 | `dispatch.go` | go | `normalizeGoReturnedTreeCompatibilityWithCensus` | `parser_result_go.go`, `parser_result_go_compat.go` | derivation_election_selection | subpasses `dispatch.go.source-file-root`, `dispatch.go.compat-walk`, `dispatch.go.new-make-type` | **Receipt: NO-GO** — "2026-08-24 Go dispatcher blocker receipt" (`KEEP LIVE / NO-GO`); three live subpasses enumerated there. |
| 14 | `dispatch.hlsl` | hlsl | `normalizeHLSLCompatibility` | `parser_result_hlsl.go` | scheduler_action_semantics | negative-number cast and unorm-buffer members remain live (subscript-assignment declarator member already retired; see compat-tier R3 note) | **Receipt: NO-GO** — "2026-08-22 HLSL blocker receipt" (`KEEP LIVE`): two known live members. |
| 15 | `dispatch.javascript` | javascript | `normalizeJavaScriptCompatibility` | `parser_result_javascript_typescript.go` | derivation_election_selection | none registered at arm level; dynamic-import token child retired separately (generic collapsed-child reducer retains the token) | **No dedicated receipt.** Progress ledger records prior partial retirements (returned-tree fixpoint arm via PR #459; dynamic-import subpass); arm remains live for its remaining registered repairs. |
| 16 | `dispatch.julia` | julia | `normalizeJuliaCompatibility` | `parser_result_julia.go` | scheduler_action_semantics | none | **No dedicated receipt.** |
| 17 | `dispatch.kotlin` | kotlin | `normalizeKotlinCompatibilityWithCensus` | `parser_result_kotlin.go` | derivation_election_selection | subpass `dispatch.kotlin.recovered-source-file-root`; interpolated-call subpass retired (generic reduction preserves the hidden call wrapper) | **No dedicated receipt** for the remaining live arm. |
| 18 | `dispatch.perl` | perl | `normalizePerlCompatibility` | `parser_result_perl.go` | scheduler_action_semantics | none | **No dedicated receipt.** |
| 19 | `dispatch.php` | php | `normalizePHPCompatibility` | `parser_result_php.go` | scheduler_action_semantics | none | **No dedicated receipt.** |
| 20 | `dispatch.powershell` | powershell | `normalizePowerShellProgramShape`, `normalizePowerShellErrorProgramRoot`, `normalizePowerShellPathCommandNameVariables`, `normalizePowerShellEnumStatementKeywordSpans` | `parser_result_powershell.go` | scheduler_action_semantics | four local repair walks (the listed functions) | **No dedicated receipt.** |
| 21 | `dispatch.python` | python | `normalizePythonCompatibilityWithParser` | `parser_result_python.go` | scheduler_action_semantics | none | **Receipt: NO-GO** — "2026-08-24 Python dispatcher blocker receipt": both f-string gaps open; skipped tests not run; authenticated corpus and lock absent; A0 denominator excludes Python; scanner limitation kept as separate incremental receipt. |
| 22 | `dispatch.rust` | rust | `normalizeRustCompatibility` | `parser_result_rust_recovery.go` | derivation_election_selection | none (dot-range repair retired via PR #470) | **Receipt: NO-GO** — "`dispatch.rust` blocker receipt — 2026-08-22": 23 witnesses, every one zero production rewrites with matching raw/production deep digests, but route-complete native ownership unproven. |
| 23 | `dispatch.scala` | scala | `normalizeScalaCompatibility` | `parser_result_scala_compilation.go`, `parser_result_misc_spans.go` | derivation_election_selection | none at arm level (span/duplicate-call repairs retired per progress ledger) | **No dedicated dispatcher blocker receipt** for the residual arm. |
| 24 | `dispatch.solidity` | solidity | `normalizeSolidityMemberObjectWrappers`, `normalizeSolidityCallExpressionAliases` | `parser_result_solidity.go` | derivation_election_selection | two local repairs (member-object wrappers; call-expression aliases) | **Receipt: NO-GO** — "2026-08-23 Solidity dispatcher blocker receipt": authenticated Solidity corpus lock and directory absent; listed divergences incl. malformed controls open; five named reopen conditions. |
| 25 | `dispatch.sql` | sql | `normalizeSQLRecoveredSelectRoot`, `normalizeSQLTrailingSelectListError`, `normalizeSQLRecoveredTopLevelSelectStatements` | `parser_result_sql.go` | scheduler_action_semantics | three local repairs (recovered select root; trailing select-list error; recovered top-level selects); field-projection repairs retired via PR #522 | **No dedicated receipt** for the residual arm. |
| 26 | `dispatch.swift` | swift | `normalizeSwiftCompatibilityWithCensus` | `parser_result_swift.go` | scheduler_action_semantics | none at arm level (ternary source-reparse subpass retired; see progress ledger) | **No dedicated receipt** for the residual arm. |
| 27 | `dispatch.templ` | templ | `normalizeTemplCompatibility` | `parser_result_templ.go` | scheduler_action_semantics | none | **Receipt: NO-GO** — "2026-08-24 Templ dispatcher blocker receipt" (`KEEP LIVE / NO-GO`): authenticated corpus lock absent; main error-root route diverges from locked C; medium template retains shape divergence after 53 rewrites; compact falls back on every witness; incremental reuse unsupported. |
| 28 | `dispatch.typescript` | tsx, typescript | `normalizeTypeScriptTreeCompatibilityWithParser` | `parser_result_javascript_typescript.go` | derivation_election_selection | none | **Receipt: NO-GO** — "2026-08-24 TypeScript dispatcher blocker receipt" (`KEEP LIVE / NO-GO`): adds one focused route test; retirement requires exact output for every registered witness on all routes. |
| 29 | `dispatch.wgsl` | wgsl | `normalizeWGSLCompatibility` | `parser_result_wgsl.go` | scheduler_action_semantics | none; grammar attaches `grammars.WgslExternalScanner` | **Receipt: NO-GO** — "2026-08-24 WGSL dispatcher blocker receipt" (`KEEP LIVE / NO-GO`; investigation worktree `/tmp/gotreesitter-n31r-next-arm.1787516501`, the N31R "next arm" probe): production route needs 171 A0 rewrites; compact falls back on both large witnesses; locked-C route has shape, type, and error-state divergences; authenticated corpus lock absent (sidecar hash only). |
| 30 | `dispatch.wolfram` | wolfram | `normalizeWolframCompatibility` | `parser_result_wolfram.go` | derivation_election_selection | none | **Receipt: NO-GO** — "2026-08-24 Wolfram blocker receipt": three authenticated A0 fixtures, 77 visited nodes, three error roots, zero rewrites; producer must close A0 error-root divergences, prove scanner-aware incremental reuse, supply authenticated real corpus, repeat all six route receipts. |
| 31 | `dispatch.yaml` | yaml | `normalizeYAMLRecoveredRoot` | `parser_result_yaml.go` | scheduler_action_semantics | one local repair (recovered root) | **No dedicated receipt.** |

### Dispatcher predicate (1)

| # | Entry ID | Languages | Function(s) | File(s) | Owner | Registered repairs | Blocker receipt status |
| -- | --- | --- | --- | --- | --- | --- | --- |
| 32 | `predicate.cobol-exact` | cobol, COBOL | `normalizeCobolCompatibility` | `parser_result_cobol.go`, `parser_result_helpers.go` | scheduler_action_semantics | none; predicate matches exactly `cobol` or `COBOL` | **No dedicated receipt.** |

## Cross-checks

- Count check: 31 dispatcher arms + 1 predicate = 32 live entries; matches
  `denominator.live_entries = 32`.
- Language-label check: 33 dispatcher languages across arms + 2 predicate
  labels (`cobol`, `COBOL`) = 35 live labels, matching
  `docs/compat-tier.md` "The live entries name 35 language labels."
- Every entry above carries the shared route tuple
  (`production`, `compact fallback`, `forest`, `incremental reuse` =
  `shared_result_compatibility_tail`; `c_oracle` =
  `curated_single_grammar_parity`) and `evidence_scope`
  `baseline_corpus_wide_only`.
- Retired history (56 retired entries, including Swift ternary, JavaScript
  dynamic-import, Kotlin interpolated-call, Bash generated-command assignment,
  Ninja recovery, Ledger recovery, JSDoc recovery arms) stays in the registry;
  deleting historical records is not retirement (`docs/compat-tier.md`).
- A0 runtime probe baseline: 41 of 41 live arms passed with `checked >= 1`,
  `run >= 1`, `nodes_visited > 0` at commit
  `65c9472806bdaa9f98d7eff0e19c0b2d53ef84d5`
  (`harness_out/docker/20260811T223206Z-a0-all-arm-probe/container.log`).
